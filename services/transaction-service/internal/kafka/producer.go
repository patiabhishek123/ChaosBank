package kafka

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/chaosbank/chaosbank/pkg/service"
	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

const (
	maxProducerRetries = 3
	retryDelay         = 2 * time.Second
)

type EventProducer interface {
	ProduceTransferEvent(ctx context.Context, event TransferEvent) error
	Close() error
}

type Producer struct {
	writer *kafka.Writer
	logger *service.Logger
	topic  string
}

type TransferEvent struct {
	EventID   string    `json:"eventId"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Amount    float64   `json:"amount"`
	Timestamp time.Time `json:"timestamp"`
}

func NewProducer(brokers, topic string, logger *service.Logger) EventProducer {
	brokerList := parseBrokers(brokers)

	return &Producer{
		writer: kafka.NewWriter(kafka.WriterConfig{
			Brokers:      brokerList,
			Topic:        topic,
			Balancer:     &kafka.LeastBytes{},
			RequiredAcks: int(kafka.RequireOne),
			Async:        false,
		}),
		logger: logger,
		topic:  topic,
	}
}

func parseBrokers(brokers string) []string {
	items := strings.Split(brokers, ",")
	var list []string
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			list = append(list, trimmed)
		}
	}
	return list
}

func (p *Producer) Close() error {
	if p.writer == nil {
		return nil
	}
	return p.writer.Close()
}

func (p *Producer) ProduceTransferEvent(ctx context.Context, event TransferEvent) error {
	if event.EventID == "" {
		event.EventID = uuid.NewString()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	message := kafka.Message{
		Key:   []byte(event.EventID),
		Value: payload,
		Time:  event.Timestamp,
	}

	var lastErr error
	for attempt := 1; attempt <= maxProducerRetries; attempt++ {
		lastErr = p.writer.WriteMessages(ctx, message)
		if lastErr == nil {
			p.logger.Info("kafka.producer.sent", map[string]interface{}{
				"event_id": event.EventID,
				"topic":    p.topic,
				"attempt":  attempt,
			})
			return nil
		}

		p.logger.Warn("kafka.producer.retry", map[string]interface{}{
			"event_id": event.EventID,
			"topic":    p.topic,
			"attempt":  attempt,
			"error":    lastErr.Error(),
		})

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryDelay * time.Duration(attempt)):
		}
	}

	p.logger.Error("kafka.producer.failed", map[string]interface{}{
		"event_id": event.EventID,
		"topic":    p.topic,
		"error":    lastErr.Error(),
	})
	return lastErr
}
