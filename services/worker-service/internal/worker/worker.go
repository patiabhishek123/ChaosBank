package worker

import (
	"context"
	"log"

	"github.com/chaosbank/chaosbank/services/worker-service/config"
	"github.com/segmentio/kafka-go"
)

type Worker struct {
	cfg    *config.Config
	reader *kafka.Reader
}

func NewWorker(cfg *config.Config) *Worker {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{cfg.KafkaBrokers},
		Topic:   "transactions",
		GroupID: "worker-group",
	})

	return &Worker{
		cfg:    cfg,
		reader: r,
	}
}

func (w *Worker) Start() {
	defer w.reader.Close()

	for {
		m, err := w.reader.ReadMessage(context.Background())
		if err != nil {
			log.Printf("Error reading message: %v", err)
			continue
		}

		log.Printf("Processing message: %s", string(m.Value))
		// Process the message
		w.processTransaction(m.Value)
	}
}

func (w *Worker) processTransaction(data []byte) {
	// Implement transaction processing logic
	log.Printf("Processing transaction: %s", string(data))
}