package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chaosbank/chaosbank/pkg/chaos"
	"github.com/chaosbank/chaosbank/pkg/metrics"
	"github.com/chaosbank/chaosbank/pkg/service"
	"github.com/chaosbank/chaosbank/pkg/util"
	"github.com/chaosbank/chaosbank/services/worker-service/config"
	ckafka "github.com/chaosbank/chaosbank/services/worker-service/internal/kafka"
	skafka "github.com/segmentio/kafka-go"
)

type Worker struct {
	cfg       *config.Config
	consumer  *ckafka.Consumer
	db        *sql.DB
	logger    *service.Logger
	chaos     *chaos.Injector
	replayMu  sync.Mutex
	replaying atomic.Bool
}

type replayBypassKey struct{}

const replayTopic = "transactions"

type ReplayResult struct {
	StartedAt       time.Time `json:"started_at"`
	FinishedAt      time.Time `json:"finished_at"`
	Partitions      int       `json:"partitions"`
	EventsProcessed int64     `json:"events_processed"`
	Status          string    `json:"status"`
}

type TransactionLogItem struct {
	ID          string     `json:"id"`
	From        string     `json:"from"`
	To          string     `json:"to"`
	Amount      float64    `json:"amount"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type SystemStats struct {
	AccountsCount        int64 `json:"accounts_count"`
	TransactionsCount    int64 `json:"transactions_count"`
	ProcessedEventsCount int64 `json:"processed_events_count"`
	ReplayInProgress     bool  `json:"replay_in_progress"`
	ChaosEnabled         bool  `json:"chaos_enabled"`
}

type ChaosToggleRequest struct {
	Enabled *bool `json:"enabled"`
}

type ChaosToggleResponse struct {
	Enabled bool `json:"enabled"`
}

type TransferEvent struct {
	EventID   string  `json:"eventId"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	Amount    float64 `json:"amount"`
	Timestamp string  `json:"timestamp"`
}

func NewWorker(cfg *config.Config, logger *service.Logger, db *sql.DB) *Worker {
	consumer := ckafka.NewConsumer(cfg.KafkaBrokers, "transactions", cfg.KafkaGroupID, cfg.ReplayFromBeginning, logger)

	return &Worker{
		cfg:      cfg,
		consumer: consumer,
		db:       db,
		logger:   logger,
		chaos:    chaos.NewInjector(logger),
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
	start := time.Now()
	defer func() {
		metrics.Default().ObserveLatency(time.Since(start))
	}()

	if w.replaying.Load() {
		if bypass, ok := ctx.Value(replayBypassKey{}).(bool); !ok || !bypass {
			metrics.Default().IncFailedTransactions()
			return errors.New("replay in progress")
		}
	}

	w.logger.Info("worker.message_received", map[string]interface{}{
		"topic":     msg.Topic,
		"partition": msg.Partition,
		"offset":    msg.Offset,
	})

	var event TransferEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		metrics.Default().IncFailedTransactions()
		w.logger.Error("worker.invalid_event", map[string]interface{}{
			"error":   err.Error(),
			"payload": string(msg.Value),
		})
		// Invalid payload is non-retriable; acknowledge by returning nil.
		return nil
	}

	if event.EventID == "" {
		metrics.Default().IncFailedTransactions()
		w.logger.Error("worker.missing_event_id", map[string]interface{}{
			"payload": string(msg.Value),
		})
		// Missing event ID cannot be deduplicated; treat as non-retriable poison message.
		return nil
	}

	if !util.ValidateRequestID(event.EventID) {
		metrics.Default().IncFailedTransactions()
		w.logger.Error("worker.invalid_event_id", map[string]interface{}{
			"event_id": event.EventID,
		})
		// Invalid event ID cannot be deduplicated safely; acknowledge to avoid poison retries.
		return nil
	}

	if event.From == "" || event.To == "" || event.Amount <= 0 {
		metrics.Default().IncFailedTransactions()
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
		metrics.Default().IncFailedTransactions()
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
	metrics.Default().IncTotalTransactions()

	return nil
}

func (w *Worker) ReplayHTTPHandler() http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if !w.cfg.ReplayEnabled {
			w.logger.Warn("worker.replay.disabled", nil)
			http.Error(rw, "replay endpoint is disabled", http.StatusForbidden)
			return
		}

		if w.chaos.Enabled() {
			w.logger.Warn("worker.replay.blocked_chaos_mode", nil)
			http.Error(rw, "disable CHAOS_MODE before replay", http.StatusPreconditionFailed)
			return
		}

		confirm := strings.TrimSpace(r.Header.Get("X-Replay-Confirm"))
		if confirm == "" || confirm != w.cfg.ReplayConfirmToken {
			w.logger.Warn("worker.replay.confirmation_failed", nil)
			http.Error(rw, "missing or invalid replay confirmation token", http.StatusBadRequest)
			return
		}

		if !w.tryBeginReplay() {
			http.Error(rw, "replay already in progress", http.StatusConflict)
			return
		}
		defer w.endReplay()

		result, err := w.replayAllEvents(r.Context())
		if err != nil {
			w.logger.Error("worker.replay.failed", map[string]interface{}{"error": err.Error()})
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		rw.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(rw).Encode(result); err != nil {
			w.logger.Error("worker.replay.encode_response_failed", map[string]interface{}{"error": err.Error()})
		}
	}
}

func (w *Worker) ChaosHTTPHandler() http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			respondJSON(rw, http.StatusOK, ChaosToggleResponse{Enabled: chaos.CurrentMode()})
			return
		case http.MethodPost:
			var req ChaosToggleRequest
			if r.Body != nil {
				_ = json.NewDecoder(r.Body).Decode(&req)
			}

			next := !chaos.CurrentMode()
			if req.Enabled != nil {
				next = *req.Enabled
			}

			chaos.SetMode(next)
			w.logger.Warn("worker.chaos.mode_changed", map[string]interface{}{"enabled": next})
			respondJSON(rw, http.StatusOK, ChaosToggleResponse{Enabled: next})
			return
		default:
			rw.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (w *Worker) TransactionLogHTTPHandler() http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		limit := int64(50)
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if v, err := strconv.ParseInt(raw, 10, 64); err == nil && v > 0 {
				if v > 500 {
					v = 500
				}
				limit = v
			}
		}

		rows, err := w.db.QueryContext(r.Context(), `
			SELECT
				t.id,
				fa.account_number,
				ta.account_number,
				t.amount,
				t.status,
				t.created_at,
				t.completed_at
			FROM transactions t
			JOIN accounts fa ON fa.id = t.from_account_id
			JOIN accounts ta ON ta.id = t.to_account_id
			ORDER BY t.created_at DESC
			LIMIT $1
		`, limit)
		if err != nil {
			w.logger.Error("worker.transactions_log.query_error", map[string]interface{}{"error": err.Error()})
			http.Error(rw, "failed to query transaction log", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		items := make([]TransactionLogItem, 0)
		for rows.Next() {
			var item TransactionLogItem
			var completedAt sql.NullTime
			if err := rows.Scan(&item.ID, &item.From, &item.To, &item.Amount, &item.Status, &item.CreatedAt, &completedAt); err != nil {
				w.logger.Error("worker.transactions_log.scan_error", map[string]interface{}{"error": err.Error()})
				http.Error(rw, "failed to read transaction log", http.StatusInternalServerError)
				return
			}
			if completedAt.Valid {
				ts := completedAt.Time
				item.CompletedAt = &ts
			}
			items = append(items, item)
		}

		if err := rows.Err(); err != nil {
			w.logger.Error("worker.transactions_log.rows_error", map[string]interface{}{"error": err.Error()})
			http.Error(rw, "failed to read transaction log", http.StatusInternalServerError)
			return
		}

		respondJSON(rw, http.StatusOK, items)
	}
}

func (w *Worker) StatsHTTPHandler() http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		stats := SystemStats{
			ReplayInProgress: w.replaying.Load(),
			ChaosEnabled:     chaos.CurrentMode(),
		}

		if err := w.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM accounts`).Scan(&stats.AccountsCount); err != nil {
			w.logger.Error("worker.stats.accounts_error", map[string]interface{}{"error": err.Error()})
			http.Error(rw, "failed to query stats", http.StatusInternalServerError)
			return
		}
		if err := w.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM transactions`).Scan(&stats.TransactionsCount); err != nil {
			w.logger.Error("worker.stats.transactions_error", map[string]interface{}{"error": err.Error()})
			http.Error(rw, "failed to query stats", http.StatusInternalServerError)
			return
		}
		if err := w.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM processed_events`).Scan(&stats.ProcessedEventsCount); err != nil {
			w.logger.Error("worker.stats.processed_events_error", map[string]interface{}{"error": err.Error()})
			http.Error(rw, "failed to query stats", http.StatusInternalServerError)
			return
		}

		respondJSON(rw, http.StatusOK, stats)
	}
}

func (w *Worker) tryBeginReplay() bool {
	w.replayMu.Lock()
	defer w.replayMu.Unlock()

	if w.replaying.Load() {
		return false
	}
	w.replaying.Store(true)
	return true
}

func (w *Worker) endReplay() {
	w.replayMu.Lock()
	defer w.replayMu.Unlock()
	w.replaying.Store(false)
}

func (w *Worker) replayAllEvents(ctx context.Context) (*ReplayResult, error) {
	startedAt := time.Now().UTC()
	w.logger.Info("worker.replay.started", map[string]interface{}{"started_at": startedAt})

	if err := w.db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("replay safety check failed: database unreachable: %w", err)
	}

	if err := w.ensureProcessedEventsTable(ctx); err != nil {
		return nil, fmt.Errorf("replay safety check failed: processed_events not ready: %w", err)
	}

	if err := w.resetReplayTables(ctx); err != nil {
		return nil, err
	}

	brokers := parseBrokers(w.cfg.KafkaBrokers)
	if len(brokers) == 0 {
		return nil, errors.New("replay safety check failed: no Kafka brokers configured")
	}

	partitions, err := listTopicPartitions(brokers, replayTopic)
	if err != nil {
		return nil, err
	}
	if len(partitions) == 0 {
		return nil, fmt.Errorf("replay safety check failed: no partitions found for topic %s", replayTopic)
	}

	var processed int64
	replayCtx := context.WithValue(ctx, replayBypassKey{}, true)

	for _, partition := range partitions {
		firstOffset, lastOffset, err := fetchPartitionOffsets(ctx, brokers, replayTopic, partition)
		if err != nil {
			return nil, err
		}

		total := lastOffset - firstOffset
		w.logger.Info("worker.replay.partition_start", map[string]interface{}{
			"partition":     partition,
			"first_offset":  firstOffset,
			"last_offset":   lastOffset,
			"message_count": total,
		})

		if total <= 0 {
			continue
		}

		reader := skafka.NewReader(skafka.ReaderConfig{
			Brokers:     brokers,
			Topic:       replayTopic,
			Partition:   partition,
			StartOffset: firstOffset,
			MinBytes:    1,
			MaxBytes:    10e6,
		})

		if err := reader.SetOffset(firstOffset); err != nil {
			reader.Close()
			return nil, fmt.Errorf("failed to set replay offset for partition %d: %w", partition, err)
		}

		for idx := int64(0); idx < total; idx++ {
			msg, err := reader.FetchMessage(replayCtx)
			if err != nil {
				reader.Close()
				return nil, fmt.Errorf("failed to fetch replay message partition=%d index=%d: %w", partition, idx, err)
			}

			if err := w.processTransaction(replayCtx, msg); err != nil {
				reader.Close()
				return nil, fmt.Errorf("failed to replay message partition=%d offset=%d: %w", partition, msg.Offset, err)
			}

			processed++
			if processed%50 == 0 {
				w.logger.Info("worker.replay.progress", map[string]interface{}{
					"events_processed": processed,
					"partition":        partition,
					"offset":           msg.Offset,
				})
			}
		}

		if err := reader.Close(); err != nil {
			return nil, fmt.Errorf("failed closing replay reader for partition %d: %w", partition, err)
		}

		w.logger.Info("worker.replay.partition_done", map[string]interface{}{
			"partition": partition,
			"processed": total,
		})
	}

	finishedAt := time.Now().UTC()
	result := &ReplayResult{
		StartedAt:       startedAt,
		FinishedAt:      finishedAt,
		Partitions:      len(partitions),
		EventsProcessed: processed,
		Status:          "completed",
	}

	w.logger.Info("worker.replay.completed", map[string]interface{}{
		"events_processed": processed,
		"partitions":       len(partitions),
		"started_at":       startedAt,
		"finished_at":      finishedAt,
	})

	return result, nil
}

func (w *Worker) resetReplayTables(ctx context.Context) error {
	w.logger.Warn("worker.replay.db_reset_started", nil)

	tx, err := w.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("failed to begin replay reset transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		TRUNCATE TABLE
			ledger_entries,
			transactions,
			processed_events,
			account_locks
		RESTART IDENTITY CASCADE
	`); err != nil {
		return fmt.Errorf("failed to clear replay tables: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit replay table reset: %w", err)
	}

	w.logger.Warn("worker.replay.db_reset_completed", nil)
	return nil
}

func parseBrokers(raw string) []string {
	parts := strings.Split(raw, ",")
	brokers := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			brokers = append(brokers, trimmed)
		}
	}
	return brokers
}

func listTopicPartitions(brokers []string, topic string) ([]int, error) {
	conn, err := skafka.Dial("tcp", brokers[0])
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Kafka broker %s: %w", brokers[0], err)
	}
	defer conn.Close()

	partitions, err := conn.ReadPartitions(topic)
	if err != nil {
		return nil, fmt.Errorf("failed to read partitions for topic %s: %w", topic, err)
	}

	ids := make([]int, 0, len(partitions))
	seen := make(map[int]struct{}, len(partitions))
	for _, p := range partitions {
		if _, ok := seen[p.ID]; ok {
			continue
		}
		seen[p.ID] = struct{}{}
		ids = append(ids, p.ID)
	}
	sort.Ints(ids)
	return ids, nil
}

func fetchPartitionOffsets(ctx context.Context, brokers []string, topic string, partition int) (int64, int64, error) {
	if len(brokers) == 0 {
		return 0, 0, errors.New("no Kafka brokers configured")
	}

	address := brokers[0]
	if _, _, err := net.SplitHostPort(address); err != nil {
		address = net.JoinHostPort(address, "9092")
	}

	conn, err := skafka.DialLeader(ctx, "tcp", address, topic, partition)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to dial Kafka leader partition=%d: %w", partition, err)
	}
	defer conn.Close()

	first, err := conn.ReadFirstOffset()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read first offset partition=%d: %w", partition, err)
	}

	last, err := conn.ReadLastOffset()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read last offset partition=%d: %w", partition, err)
	}

	return first, last, nil
}

func (w *Worker) ensureProcessedEventsTable(ctx context.Context) error {
	if err := w.chaos.InjectDBFailure("worker.ensure_processed_events_table"); err != nil {
		return err
	}

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
	if err := w.chaos.InjectDBFailure("worker.begin_transaction"); err != nil {
		return false, err
	}

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

	if err := ensureReplayAccounts(ctx, tx, event); err != nil {
		return false, err
	}

	if err := w.applyTransferBalanceUpdates(ctx, tx, event); err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}

	if err := w.chaos.InjectPartialFailure("worker.after_commit"); err != nil {
		return false, err
	}

	return false, nil
}

func (w *Worker) applyTransferBalanceUpdates(ctx context.Context, tx *sql.Tx, event TransferEvent) error {
	if err := w.chaos.InjectDBFailure("worker.apply_transfer_balance_updates"); err != nil {
		return err
	}

	if event.From == event.To {
		return errors.New("from and to accounts must be different")
	}

	amount := math.Round(event.Amount*100) / 100
	if amount <= 0 {
		return errors.New("amount must be greater than 0")
	}

	type accountSnapshot struct {
		ID      string
		Number  string
		Balance float64
	}

	accounts := make(map[string]accountSnapshot, 2)

	rows, err := tx.QueryContext(ctx, `
		SELECT id, account_number, balance
		FROM accounts
		WHERE account_number IN ($1, $2)
		  AND status = 'active'
		ORDER BY account_number
		FOR UPDATE
	`, event.From, event.To)
	if err != nil {
		return err
	}
	defer rows.Close()

	lockedCount := 0
	for rows.Next() {
		var acc accountSnapshot
		if err := rows.Scan(&acc.ID, &acc.Number, &acc.Balance); err != nil {
			return err
		}
		accounts[acc.Number] = acc
		lockedCount++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if lockedCount != 2 {
		return fmt.Errorf("active accounts not found for transfer: from=%s to=%s", event.From, event.To)
	}

	fromAcc, ok := accounts[event.From]
	if !ok {
		return fmt.Errorf("source account not found: from=%s", event.From)
	}

	toAcc, ok := accounts[event.To]
	if !ok {
		return fmt.Errorf("destination account not found: to=%s", event.To)
	}

	if fromAcc.Balance < amount {
		return fmt.Errorf("insufficient funds: from=%s", event.From)
	}

	debitResult, err := tx.ExecContext(ctx, `
		UPDATE accounts
		SET balance = balance - $1, version = version + 1
		WHERE id = $2
		  AND balance >= $1
	`, amount, fromAcc.ID)
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
		WHERE id = $2
	`, amount, toAcc.ID)
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

	var transactionID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO transactions (from_account_id, to_account_id, amount, status, reference_code, completed_at)
		VALUES ($1, $2, $3, 'completed', $4, CURRENT_TIMESTAMP)
		RETURNING id
	`, fromAcc.ID, toAcc.ID, amount, event.EventID).Scan(&transactionID)
	if err != nil {
		return err
	}

	fromBalanceAfter := math.Round((fromAcc.Balance-amount)*100) / 100
	toBalanceAfter := math.Round((toAcc.Balance+amount)*100) / 100

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (
			transaction_id,
			account_id,
			entry_type,
			amount,
			balance_before,
			balance_after
		)
		VALUES
			($1, $2, 'debit', $3, $4, $5),
			($1, $6, 'credit', $3, $7, $8)
	`, transactionID, fromAcc.ID, amount, fromAcc.Balance, fromBalanceAfter, toAcc.ID, toAcc.Balance, toBalanceAfter); err != nil {
		return err
	}

	return nil
}

func ensureReplayAccounts(ctx context.Context, tx *sql.Tx, event TransferEvent) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO accounts (account_number, owner_name, balance, currency, status, version)
		VALUES ($1, $2, 0.00, 'USD', 'active', 1)
		ON CONFLICT (account_number) DO NOTHING
	`, event.From, event.From); err != nil {
		return fmt.Errorf("failed to upsert sender account for replay: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO accounts (account_number, owner_name, balance, currency, status, version)
		VALUES ($1, $2, 0.00, 'USD', 'active', 1)
		ON CONFLICT (account_number) DO NOTHING
	`, event.To, event.To); err != nil {
		return fmt.Errorf("failed to upsert receiver account for replay: %w", err)
	}

	return nil
}

func respondJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
