package metrics

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

type Registry struct {
	totalTransactions       atomic.Int64
	failedTransactions      atomic.Int64
	retries                 atomic.Int64
	latencyTotalNanoseconds atomic.Int64
	latencyCount            atomic.Int64
	lastLatencyNanoseconds  atomic.Int64
}

type Snapshot struct {
	TotalTransactions       int64
	FailedTransactions      int64
	Retries                 int64
	LatencyTotalNanoseconds int64
	LatencyCount            int64
	LastLatencyNanoseconds  int64
}

var defaultRegistry = &Registry{}

func Default() *Registry {
	return defaultRegistry
}

func (r *Registry) IncTotalTransactions() {
	r.totalTransactions.Add(1)
}

func (r *Registry) IncFailedTransactions() {
	r.failedTransactions.Add(1)
}

func (r *Registry) IncRetries() {
	r.retries.Add(1)
}

func (r *Registry) ObserveLatency(d time.Duration) {
	nanos := d.Nanoseconds()
	r.latencyTotalNanoseconds.Add(nanos)
	r.latencyCount.Add(1)
	r.lastLatencyNanoseconds.Store(nanos)
}

func (r *Registry) Snapshot() Snapshot {
	return Snapshot{
		TotalTransactions:       r.totalTransactions.Load(),
		FailedTransactions:      r.failedTransactions.Load(),
		Retries:                 r.retries.Load(),
		LatencyTotalNanoseconds: r.latencyTotalNanoseconds.Load(),
		LatencyCount:            r.latencyCount.Load(),
		LastLatencyNanoseconds:  r.lastLatencyNanoseconds.Load(),
	}
}

func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		s := Default().Snapshot()
		avgLatencySeconds := 0.0
		if s.LatencyCount > 0 {
			avgLatencySeconds = float64(s.LatencyTotalNanoseconds) / float64(s.LatencyCount) / float64(time.Second)
		}
		lastLatencySeconds := float64(s.LastLatencyNanoseconds) / float64(time.Second)
		totalLatencySeconds := float64(s.LatencyTotalNanoseconds) / float64(time.Second)

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = fmt.Fprintf(w, "# HELP chaosbank_total_transactions_total Total successfully processed transactions\n")
		_, _ = fmt.Fprintf(w, "# TYPE chaosbank_total_transactions_total counter\n")
		_, _ = fmt.Fprintf(w, "chaosbank_total_transactions_total %d\n", s.TotalTransactions)
		_, _ = fmt.Fprintf(w, "# HELP chaosbank_failed_transactions_total Total failed transaction processing attempts\n")
		_, _ = fmt.Fprintf(w, "# TYPE chaosbank_failed_transactions_total counter\n")
		_, _ = fmt.Fprintf(w, "chaosbank_failed_transactions_total %d\n", s.FailedTransactions)
		_, _ = fmt.Fprintf(w, "# HELP chaosbank_transaction_retries_total Total retry attempts while processing transactions\n")
		_, _ = fmt.Fprintf(w, "# TYPE chaosbank_transaction_retries_total counter\n")
		_, _ = fmt.Fprintf(w, "chaosbank_transaction_retries_total %d\n", s.Retries)
		_, _ = fmt.Fprintf(w, "# HELP chaosbank_processing_latency_seconds_total Sum of transaction processing latency in seconds\n")
		_, _ = fmt.Fprintf(w, "# TYPE chaosbank_processing_latency_seconds_total counter\n")
		_, _ = fmt.Fprintf(w, "chaosbank_processing_latency_seconds_total %.6f\n", totalLatencySeconds)
		_, _ = fmt.Fprintf(w, "# HELP chaosbank_processing_latency_seconds_count Number of latency observations\n")
		_, _ = fmt.Fprintf(w, "# TYPE chaosbank_processing_latency_seconds_count counter\n")
		_, _ = fmt.Fprintf(w, "chaosbank_processing_latency_seconds_count %d\n", s.LatencyCount)
		_, _ = fmt.Fprintf(w, "# HELP chaosbank_processing_latency_seconds_avg Average transaction processing latency in seconds\n")
		_, _ = fmt.Fprintf(w, "# TYPE chaosbank_processing_latency_seconds_avg gauge\n")
		_, _ = fmt.Fprintf(w, "chaosbank_processing_latency_seconds_avg %.6f\n", avgLatencySeconds)
		_, _ = fmt.Fprintf(w, "# HELP chaosbank_processing_latency_seconds_last Last observed transaction processing latency in seconds\n")
		_, _ = fmt.Fprintf(w, "# TYPE chaosbank_processing_latency_seconds_last gauge\n")
		_, _ = fmt.Fprintf(w, "chaosbank_processing_latency_seconds_last %.6f\n", lastLatencySeconds)
	})
}
