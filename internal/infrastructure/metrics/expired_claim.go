package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ExpiredClaimMetrics holds Prometheus metrics for advisory expired-follow-up
// claim operations.
type ExpiredClaimMetrics struct {
	ActiveClaims    prometheus.Gauge
	ClaimsCreated   prometheus.Counter
	ClaimsExpired   prometheus.Counter
	ClaimsCancelled prometheus.Counter
	ClaimsResolved  prometheus.Counter
	ClaimErrors     *prometheus.CounterVec
}

// NewExpiredClaimMetrics creates and registers expired-claim metrics.
func NewExpiredClaimMetrics() *ExpiredClaimMetrics {
	return &ExpiredClaimMetrics{
		ActiveClaims: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "expired_claims_active",
			Help: "Current number of active expired follow-up claims",
		}),
		ClaimsCreated: promauto.NewCounter(prometheus.CounterOpts{
			Name: "expired_claims_created_total",
			Help: "Total number of expired follow-up claims created",
		}),
		ClaimsExpired: promauto.NewCounter(prometheus.CounterOpts{
			Name: "expired_claims_expired_total",
			Help: "Total number of expired follow-up claims that expired",
		}),
		ClaimsCancelled: promauto.NewCounter(prometheus.CounterOpts{
			Name: "expired_claims_cancelled_total",
			Help: "Total number of expired follow-up claims cancelled by the holder",
		}),
		ClaimsResolved: promauto.NewCounter(prometheus.CounterOpts{
			Name: "expired_claims_resolved_total",
			Help: "Total number of expired follow-up claims released because the game became resolved",
		}),
		ClaimErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "expired_claim_errors_total",
				Help: "Total number of expired-claim operation errors by type",
			},
			[]string{"error_type"},
		),
	}
}

// RecordCreated records a successful expired-claim creation.
func (m *ExpiredClaimMetrics) RecordCreated() {
	m.ClaimsCreated.Inc()
	m.ActiveClaims.Inc()
}

// RecordExpired records an expired-claim expiration.
func (m *ExpiredClaimMetrics) RecordExpired() {
	m.ClaimsExpired.Inc()
	m.ActiveClaims.Dec()
}

// RecordCancelled records a holder-initiated cancellation.
func (m *ExpiredClaimMetrics) RecordCancelled() {
	m.ClaimsCancelled.Inc()
	m.ActiveClaims.Dec()
}

// RecordResolved records a release triggered by a state change that made the
// follow-up no longer actionable.
func (m *ExpiredClaimMetrics) RecordResolved() {
	m.ClaimsResolved.Inc()
	m.ActiveClaims.Dec()
}

// RecordError records an expired-claim operation error by type.
func (m *ExpiredClaimMetrics) RecordError(errorType string) {
	m.ClaimErrors.WithLabelValues(errorType).Inc()
}

// SetActiveCount sets the current active expired-claim count.
func (m *ExpiredClaimMetrics) SetActiveCount(count int) {
	m.ActiveClaims.Set(float64(count))
}
