package worker

import (
	"context"
	"errors"

	"github.com/chaosbank/chaosbank/pkg/service"
	"github.com/chaosbank/chaosbank/services/worker-service/config"
	"github.com/segmentio/kafka-go"
)

type Worker struct {
	cfg    *config.Config
	reader *kafka.Reader
	logger *service.Logger
}

func NewWorker(cfg *config.Config, logger *service.Logger) *Worker {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{cfg.KafkaBrokers},
		Topic:   "transactions",
		GroupID: "worker-group",
	})

	return &Worker{
		cfg:    cfg,
		reader: r,
		logger: logger,
	}
}

func (w *Worker) Start(ctx context.Context) {
	defer w.reader.Close()

	for {
		m, err := w.reader.ReadMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				w.logger.Info("worker.shutdown", nil)
				return
			}

			w.logger.Error("worker.read_error", map[string]interface{}{"error": err.Error()})
			continue
		}

		w.logger.Info("worker.message_received", map[string]interface{}{
			"topic":     m.Topic,
			"partition": m.Partition,
			"offset":    m.Offset,
		})
		w.processTransaction(m.Value)
	}
}

func (w *Worker) processTransaction(data []byte) {
	w.logger.Info("worker.process_transaction", map[string]interface{}{"payload": string(data)})
}
