package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// BlockchainMetrics holds Prometheus metrics for blockchain event processing (T097).
type BlockchainMetrics struct {
	// EventsReceived tracks total blockchain events received by event type
	EventsReceived *prometheus.CounterVec

	// EventsProcessed tracks successfully processed events by event type
	EventsProcessed *prometheus.CounterVec

	// EventsFailed tracks failed event processing by event type and reason
	EventsFailed *prometheus.CounterVec

	// DatabaseQueriesDuration tracks database query latency during event processing
	DatabaseQueriesDuration prometheus.Histogram

	// EventProcessingDuration tracks end-to-end event processing time
	EventProcessingDuration *prometheus.HistogramVec

	// LastProcessedBlock tracks the last successfully processed block number
	LastProcessedBlock prometheus.Gauge

	// CircuitBreakerState tracks TON Center circuit breaker state (0=closed, 1=open, 2=half-open)
	CircuitBreakerState prometheus.Gauge
}

// NewBlockchainMetrics creates and registers blockchain metrics (T097).
func NewBlockchainMetrics() *BlockchainMetrics {
	return &BlockchainMetrics{
		EventsReceived: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "blockchain_events_received_total",
				Help: "Total number of blockchain events received by event type",
			},
			[]string{"event_type"},
		),

		EventsProcessed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "blockchain_events_processed_total",
				Help: "Total number of blockchain events successfully processed by event type",
			},
			[]string{"event_type"},
		),

		EventsFailed: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "blockchain_events_failed_total",
				Help: "Total number of failed blockchain event processing attempts by event type and reason",
			},
			[]string{"event_type", "reason"},
		),

		DatabaseQueriesDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "database_queries_duration_seconds",
				Help:    "Duration of database queries during blockchain event processing",
				Buckets: prometheus.DefBuckets,
			},
		),

		EventProcessingDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "blockchain_event_processing_duration_seconds",
				Help:    "Duration of blockchain event processing by event type",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
			},
			[]string{"event_type"},
		),

		LastProcessedBlock: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "blockchain_last_processed_block",
				Help: "Last successfully processed blockchain block number",
			},
		),

		CircuitBreakerState: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "blockchain_circuit_breaker_state",
				Help: "TON Center circuit breaker state (0=closed/normal, 1=open/failing, 2=half-open/testing)",
			},
		),
	}
}

// RecordEventReceived increments the events received counter.
func (m *BlockchainMetrics) RecordEventReceived(eventType string) {
	m.EventsReceived.WithLabelValues(eventType).Inc()
}

// RecordEventProcessed increments the events processed counter and records processing duration.
func (m *BlockchainMetrics) RecordEventProcessed(eventType string, duration time.Duration) {
	m.EventsProcessed.WithLabelValues(eventType).Inc()
	m.EventProcessingDuration.WithLabelValues(eventType).Observe(duration.Seconds())
}

// RecordEventFailed increments the events failed counter.
func (m *BlockchainMetrics) RecordEventFailed(eventType, reason string) {
	m.EventsFailed.WithLabelValues(eventType, reason).Inc()
}

// RecordDatabaseQuery records database query duration.
func (m *BlockchainMetrics) RecordDatabaseQuery(duration time.Duration) {
	m.DatabaseQueriesDuration.Observe(duration.Seconds())
}

// UpdateLastProcessedBlock updates the last processed block gauge.
func (m *BlockchainMetrics) UpdateLastProcessedBlock(block int64) {
	m.LastProcessedBlock.Set(float64(block))
}

// UpdateCircuitBreakerState updates the circuit breaker state gauge.
// state: 0 = closed (normal), 1 = open (failing), 2 = half-open (testing)
func (m *BlockchainMetrics) UpdateCircuitBreakerState(state int) {
	m.CircuitBreakerState.Set(float64(state))
}
