//nolint:dupl // This metric family intentionally mirrors the existing advisory reservation/claim metrics with cancel-specific names.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// CancelReservationMetrics holds Prometheus metrics for cancel-coordination
// operations. It mirrors the other advisory coordination metric families so the
// lifecycle can be observed independently.
type CancelReservationMetrics struct {
	ActiveReservations    prometheus.Gauge
	ReservationsCreated   prometheus.Counter
	ReservationsExpired   prometheus.Counter
	ReservationsCancelled prometheus.Counter
	ReservationsResolved  prometheus.Counter
	ReservationErrors     *prometheus.CounterVec
}

// NewCancelReservationMetrics creates and registers cancel-reservation metrics.
func NewCancelReservationMetrics() *CancelReservationMetrics {
	return &CancelReservationMetrics{
		ActiveReservations: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "cancel_reservations_active",
			Help: "Current number of active cancel reservations",
		}),
		ReservationsCreated: promauto.NewCounter(prometheus.CounterOpts{
			Name: "cancel_reservations_created_total",
			Help: "Total number of cancel reservations created",
		}),
		ReservationsExpired: promauto.NewCounter(prometheus.CounterOpts{
			Name: "cancel_reservations_expired_total",
			Help: "Total number of cancel reservations that expired",
		}),
		ReservationsCancelled: promauto.NewCounter(prometheus.CounterOpts{
			Name: "cancel_reservations_cancelled_total",
			Help: "Total number of cancel reservations cancelled manually",
		}),
		ReservationsResolved: promauto.NewCounter(prometheus.CounterOpts{
			Name: "cancel_reservations_resolved_total",
			Help: "Total number of cancel reservations released by authoritative game progress",
		}),
		ReservationErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "cancel_reservation_errors_total",
				Help: "Total number of cancel-reservation operation errors by type",
			},
			[]string{"error_type"},
		),
	}
}

// RecordCreated records a successful cancel-reservation creation.
func (m *CancelReservationMetrics) RecordCreated() {
	m.ReservationsCreated.Inc()
	m.ActiveReservations.Inc()
}

// RecordExpired records a cancel-reservation expiration.
func (m *CancelReservationMetrics) RecordExpired() {
	m.ReservationsExpired.Inc()
	m.ActiveReservations.Dec()
}

// RecordCancelled records a holder-initiated cancellation.
func (m *CancelReservationMetrics) RecordCancelled() {
	m.ReservationsCancelled.Inc()
	m.ActiveReservations.Dec()
}

// RecordResolved records an authoritative release.
func (m *CancelReservationMetrics) RecordResolved() {
	m.ReservationsResolved.Inc()
	m.ActiveReservations.Dec()
}

// RecordError records a cancel-reservation operation error by type.
func (m *CancelReservationMetrics) RecordError(errorType string) {
	m.ReservationErrors.WithLabelValues(errorType).Inc()
}

// SetActiveCount sets the current active cancel-reservation count.
func (m *CancelReservationMetrics) SetActiveCount(count int) {
	m.ActiveReservations.Set(float64(count))
}
