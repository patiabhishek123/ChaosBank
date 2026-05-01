package chaos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/chaosbank/chaosbank/pkg/service"
)

const (
	maxDelayMillis        = 3000
	minFailureProbability = 0.10
	maxFailureProbability = 0.30
	duplicateProbability  = 0.15

	networkTimeoutDuration = 50 * time.Millisecond
)

type Injector struct {
	logger *service.Logger
}

var chaosModeOverride atomic.Int32

func init() {
	chaosModeOverride.Store(-1)
}

func NewInjector(logger *service.Logger) *Injector {
	return &Injector{logger: logger}
}

func (i *Injector) Enabled() bool {
	if i == nil {
		return false
	}
	return CurrentMode()
}

func CurrentMode() bool {
	override := chaosModeOverride.Load()
	if override == 0 {
		return false
	}
	if override == 1 {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(os.Getenv("CHAOS_MODE")), "true")
}

func SetMode(enabled bool) {
	if enabled {
		chaosModeOverride.Store(1)
		os.Setenv("CHAOS_MODE", "true")
		return
	}

	chaosModeOverride.Store(0)
	os.Setenv("CHAOS_MODE", "false")
}

func (i *Injector) InjectDBFailure(operation string) error {
	if !i.Enabled() {
		return nil
	}

	if !i.shouldFail() {
		return nil
	}

	err := fmt.Errorf("chaos injected db failure: %s", operation)
	i.logger.Warn("chaos.db.failure_injected", map[string]interface{}{
		"operation": operation,
		"error":     err.Error(),
	})
	return err
}

func (i *Injector) InjectNetworkTimeout(operation string) error {
	if !i.Enabled() {
		return nil
	}

	if !i.shouldFail() {
		return nil
	}

	err := fmt.Errorf("chaos injected network timeout: %s: %w", operation, context.DeadlineExceeded)
	i.logger.Warn("chaos.network.timeout_injected", map[string]interface{}{
		"operation": operation,
		"timeout":   networkTimeoutDuration.String(),
		"error":     err.Error(),
	})
	return err
}

func (i *Injector) InjectPartialFailure(operation string) error {
	if !i.Enabled() {
		return nil
	}

	if !i.shouldFail() {
		return nil
	}

	err := fmt.Errorf("chaos injected partial failure: %s", operation)
	i.logger.Warn("chaos.partial.failure_injected", map[string]interface{}{
		"operation": operation,
		"error":     err.Error(),
	})
	return err
}

func (i *Injector) HTTPMiddleware(next http.Handler) http.Handler {
	if !i.Enabled() {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := i.injectDelay(r.Context()); err != nil {
			http.Error(w, "request canceled", http.StatusRequestTimeout)
			return
		}

		if i.shouldFail() {
			i.logger.Warn("chaos.http.failure_injected", map[string]interface{}{
				"method": r.Method,
				"path":   r.URL.Path,
			})
			http.Error(w, "chaos injected failure", http.StatusServiceUnavailable)
			return
		}

		var body []byte
		if r.Body != nil {
			payload, err := io.ReadAll(r.Body)
			if err == nil {
				body = payload
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
		}

		next.ServeHTTP(w, r)

		if !i.shouldDuplicateRequest(r.Method) {
			return
		}

		duplicateReq := r.Clone(context.Background())
		if body != nil {
			duplicateReq.Body = io.NopCloser(bytes.NewReader(body))
		}

		i.logger.Warn("chaos.http.duplicate_simulated", map[string]interface{}{
			"method": r.Method,
			"path":   r.URL.Path,
		})

		dupWriter := &discardResponseWriter{header: make(http.Header)}
		next.ServeHTTP(dupWriter, duplicateReq)
	})
}

func WrapHandler[T any](injector *Injector, handler func(context.Context, T) error) func(context.Context, T) error {
	if injector == nil || !injector.Enabled() {
		return handler
	}

	return func(ctx context.Context, input T) error {
		if err := injector.injectDelay(ctx); err != nil {
			return err
		}

		if injector.shouldFail() {
			injector.logger.Warn("chaos.worker.failure_injected", nil)
			return errors.New("chaos injected worker failure")
		}

		if err := handler(ctx, input); err != nil {
			return err
		}

		if injector.shouldDuplicate() {
			injector.logger.Warn("chaos.worker.duplicate_simulated", nil)
			if err := handler(ctx, input); err != nil {
				injector.logger.Warn("chaos.worker.duplicate_handler_error", map[string]interface{}{"error": err.Error()})
			}
		}

		return nil
	}
}

func (i *Injector) injectDelay(ctx context.Context) error {
	delay := time.Duration(rand.Intn(maxDelayMillis+1)) * time.Millisecond
	if delay <= 0 {
		return nil
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		i.logger.Info("chaos.delay_injected", map[string]interface{}{"delay_ms": delay.Milliseconds()})
		return nil
	}
}

func (i *Injector) shouldFail() bool {
	p := minFailureProbability + rand.Float64()*(maxFailureProbability-minFailureProbability)
	return rand.Float64() < p
}

func (i *Injector) shouldDuplicate() bool {
	return rand.Float64() < duplicateProbability
}

func (i *Injector) shouldDuplicateRequest(method string) bool {
	if !i.shouldDuplicate() {
		return false
	}

	switch strings.ToUpper(method) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

type discardResponseWriter struct {
	header http.Header
	status int
}

func (w *discardResponseWriter) Header() http.Header {
	return w.header
}

func (w *discardResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return len(b), nil
}

func (w *discardResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}
