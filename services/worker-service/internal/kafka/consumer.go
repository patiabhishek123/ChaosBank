package kafka

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chaosbank/chaosbank/pkg/chaos"
	"github.com/chaosbank/chaosbank/pkg/service"
	"github.com/segmentio/kafka-go"
)

const (
	defaultMaxRetries = 3
	defaultRetryDelay = 1 * time.Second
)

type HandlerFunc func(ctx context.Context, msg kafka.Message) error

type Consumer struct {
	reader     *kafka.Reader
	logger     *service.Logger
	chaos      *chaos.Injector
	maxRetries int
	retryDelay time.Duration
}

func NewConsumer(brokers, topic, groupID string, replayFromBeginning bool, logger *service.Logger) *Consumer {
	readerGroupID := groupID
	startOffset := int64(kafka.LastOffset)

	if replayFromBeginning {
		readerGroupID = replayGroupID(groupID)
		startOffset = kafka.FirstOffset
		logger.Warn("kafka.consumer.replay_mode_enabled", map[string]interface{}{
			"group_id":      readerGroupID,
			"start_offset":  startOffset,
			"replay_enabled": true,
		})
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        parseBrokers(brokers),
		Topic:          topic,
		GroupID:        readerGroupID,
		StartOffset:    startOffset,
		CommitInterval: 0,
		MinBytes:       1,
		MaxBytes:       10e6,
	})

	return &Consumer{
		reader:     reader,
		logger:     logger,
		chaos:      chaos.NewInjector(logger),
		maxRetries: defaultMaxRetries,
		retryDelay: defaultRetryDelay,
	}
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

func replayGroupID(base string) string {
	if strings.TrimSpace(base) == "" {
		base = "worker-group"
	}
	return fmt.Sprintf("%s-replay-%d", base, time.Now().UTC().Unix())
}

func (c *Consumer) Close() error {
	if c.reader == nil {
		return nil
	}
	return c.reader.Close()
}

func (c *Consumer) Consume(ctx context.Context, handler HandlerFunc) {
	defer c.Close()
	wrappedHandler := chaos.WrapHandler(c.chaos, handler)

	for {
		if err := c.chaos.InjectNetworkTimeout("kafka.consumer.fetch_message"); err != nil {
			c.logger.Warn("kafka.consumer.chaos_network_timeout", map[string]interface{}{
				"stage": "fetch",
				"error": err.Error(),
			})
			continue
		}

		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				c.logger.Info("kafka.consumer.shutdown", nil)
				return
			}
			c.logger.Error("kafka.consumer.fetch_error", map[string]interface{}{
				"error": err.Error(),
			})
			continue
		}

		if err := c.processWithRetry(ctx, msg, wrappedHandler); err != nil {
			c.logger.Error("kafka.consumer.process_failed", map[string]interface{}{
				"topic":     msg.Topic,
				"partition": msg.Partition,
				"offset":    msg.Offset,
				"error":     err.Error(),
			})
			continue
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			if errors.Is(err, context.Canceled) {
				c.logger.Info("kafka.consumer.shutdown", nil)
				return
			}
			c.logger.Error("kafka.consumer.commit_error", map[string]interface{}{
				"topic":     msg.Topic,
				"partition": msg.Partition,
				"offset":    msg.Offset,
				"error":     err.Error(),
			})
			continue
		}

		if err := c.chaos.InjectPartialFailure("kafka.consumer.after_commit"); err != nil {
			c.logger.Warn("kafka.consumer.chaos_partial_failure", map[string]interface{}{
				"topic":     msg.Topic,
				"partition": msg.Partition,
				"offset":    msg.Offset,
				"error":     err.Error(),
			})
			continue
		}

		c.logger.Info("kafka.consumer.committed", map[string]interface{}{
			"topic":     msg.Topic,
			"partition": msg.Partition,
			"offset":    msg.Offset,
		})
	}
}

func (c *Consumer) processWithRetry(ctx context.Context, msg kafka.Message, handler HandlerFunc) error {
	var lastErr error
	for attempt := 1; attempt <= c.maxRetries; attempt++ {
		if err := handler(ctx, msg); err == nil {
			return nil
		} else {
			lastErr = err
			c.logger.Warn("kafka.consumer.retry", map[string]interface{}{
				"topic":     msg.Topic,
				"partition": msg.Partition,
				"offset":    msg.Offset,
				"attempt":   attempt,
				"error":     err.Error(),
			})
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.retryDelay * time.Duration(attempt)):
		}
	}
	return lastErr
}
