package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/chaosbank/chaosbank/pkg/service"
	"github.com/chaosbank/chaosbank/pkg/util"
	"github.com/chaosbank/chaosbank/services/worker-service/config"
	ckafka "github.com/chaosbank/chaosbank/services/worker-service/internal/kafka"
	skafka "github.com/segmentio/kafka-go"
)

type Worker struct {
	cfg      *config.Config
	consumer *ckafka.Consumer
	db       *sql.DB
	logger   *service.Logger
}

type TransferEvent struct {
	EventID   string  `json:"eventId"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	Amount    float64 `json:"amount"`
	Timestamp string  `json:"timestamp"`
}

func NewWorker(cfg *config.Config, logger *service.Logger, db *sql.DB) *Worker {
	consumer := ckafka.NewConsumer(cfg.KafkaBrokers, "transactions", "worker-group", logger)

	return &Worker{
		cfg:      cfg,
		consumer: consumer,
		db:       db,
		logger:   logger,
	}
}

func (w *Worker) Start(ctx context.Context) {
	if err := w.ensureProcessedEventsTable(ctx); err != nil {
		w.logger.Error("worker.ensure_processed_events_table_error", map[string]interface{}{"error": err.Error()})
		return
	}

	w.consumer.Consume(ctx, w.processTransaction)
}

func (w *Worker) processTransaction(ctx context.Context, msg skafka.Message) error {
	w.logger.Info("worker.message_received", map[string]interface{}{
		"topic":     msg.Topic,
		"partition": msg.Partition,
		"offset":    msg.Offset,
	})

	var event TransferEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		w.logger.Error("worker.invalid_event", map[string]interface{}{
			"error":   err.Error(),
			"payload": string(msg.Value),
		})
		// Invalid payload is non-retriable; acknowledge by returning nil.
		return nil
	}

	if event.EventID == "" {
		w.logger.Error("worker.missing_event_id", map[string]interface{}{
			"payload": string(msg.Value),
		})
		// Missing event ID cannot be deduplicated; treat as non-retriable poison message.
		return nil
	}

	if !util.ValidateRequestID(event.EventID) {
		w.logger.Error("worker.invalid_event_id", map[string]interface{}{
			"event_id": event.EventID,
		})
		// Invalid event ID cannot be deduplicated safely; acknowledge to avoid poison retries.
		return nil
	}

	if event.From == "" || event.To == "" || event.Amount <= 0 {
		w.logger.Error("worker.invalid_transfer_event", map[string]interface{}{
			"event_id": event.EventID,
			"from":     event.From,
			"to":       event.To,
			"amount":   event.Amount,
		})
		return nil
	}

	duplicate, err := w.applyTransferWithDedup(ctx, event, msg)
	if err != nil {
		return err
	}

	if duplicate {
		w.logger.Info("worker.event_skipped_duplicate", map[string]interface{}{
			"event_id":  event.EventID,
			"partition": msg.Partition,
			"offset":    msg.Offset,
		})
		return nil
	}

	w.logger.Info("worker.process_transaction", map[string]interface{}{
		"event_id":  event.EventID,
		"from":      event.From,
		"to":        event.To,
		"amount":    event.Amount,
		"partition": msg.Partition,
		"offset":    msg.Offset,
	})

	return nil
}

func (w *Worker) ensureProcessedEventsTable(ctx context.Context) error {
	_, err := w.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS processed_events (
			event_id UUID PRIMARY KEY,
			topic VARCHAR(255) NOT NULL,
			partition_id INT NOT NULL,
			message_offset BIGINT NOT NULL,
			processed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	return err
}

func (w *Worker) applyTransferWithDedup(ctx context.Context, event TransferEvent, msg skafka.Message) (bool, error) {
	tx, err := w.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var exists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM processed_events WHERE event_id = $1)`,
		event.EventID,
	).Scan(&exists); err != nil {
		return false, err
	}

	if exists {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	}

	insertResult, err := tx.ExecContext(ctx, `
		INSERT INTO processed_events (event_id, topic, partition_id, message_offset, processed_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		ON CONFLICT (event_id) DO NOTHING
	`, event.EventID, msg.Topic, msg.Partition, msg.Offset)
	if err != nil {
		return false, err
	}

	rows, err := insertResult.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 0 {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	}

	if err := w.applyTransferBalanceUpdates(ctx, tx, event); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}

	return false, nil
}

func (w *Worker) applyTransferBalanceUpdates(ctx context.Context, tx *sql.Tx, event TransferEvent) error {
	if event.From == event.To {
		return errors.New("from and to accounts must be different")
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT account_number
		FROM accounts
		WHERE account_number IN ($1, $2)
		ORDER BY account_number
		FOR UPDATE
	`, event.From, event.To)
	if err != nil {
		return err
	}
	defer rows.Close()

	lockedCount := 0
	for rows.Next() {
		lockedCount++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if lockedCount != 2 {
		return fmt.Errorf("accounts not found for transfer: from=%s to=%s", event.From, event.To)
	}

	debitResult, err := tx.ExecContext(ctx, `
		UPDATE accounts
		SET balance = balance - $1, version = version + 1
		WHERE account_number = $2
		  AND balance >= $1
	`, event.Amount, event.From)
	if err != nil {
		return err
	}

	debitRows, err := debitResult.RowsAffected()
	if err != nil {
		return err
	}
	if debitRows == 0 {
		return fmt.Errorf("insufficient funds or source account not found: from=%s", event.From)
	}

	creditResult, err := tx.ExecContext(ctx, `
		UPDATE accounts
		SET balance = balance + $1, version = version + 1
		WHERE account_number = $2
	`, event.Amount, event.To)
	if err != nil {
		return err
	}

	creditRows, err := creditResult.RowsAffected()
	if err != nil {
		return err
	}
	if creditRows == 0 {
		return fmt.Errorf("destination account not found: to=%s", event.To)
	}

	return nil
}
