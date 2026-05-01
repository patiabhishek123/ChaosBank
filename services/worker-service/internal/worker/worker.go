package worker

import (
	"context"

	"github.com/chaosbank/chaosbank/pkg/service"
	"github.com/chaosbank/chaosbank/services/worker-service/config"
	ckafka "github.com/chaosbank/chaosbank/services/worker-service/internal/kafka"
	skafka "github.com/segmentio/kafka-go"
)

type Worker struct {
	cfg      *config.Config
	consumer *ckafka.Consumer
	logger   *service.Logger
}

func NewWorker(cfg *config.Config, logger *service.Logger) *Worker {
	consumer := ckafka.NewConsumer(cfg.KafkaBrokers, "transactions", "worker-group", logger)

	return &Worker{
		cfg:      cfg,
		consumer: consumer,
		logger:   logger,
	}
}

func (w *Worker) Start(ctx context.Context) {
	w.consumer.Consume(ctx, w.processTransaction)
}

func (w *Worker) processTransaction(_ context.Context, msg skafka.Message) error {
	w.logger.Info("worker.message_received", map[string]interface{}{
		"topic":     msg.Topic,
		"partition": msg.Partition,
		"offset":    msg.Offset,
	})

	w.logger.Info("worker.process_transaction", map[string]interface{}{"payload": string(msg.Value)})
	return nil
}
