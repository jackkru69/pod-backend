package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// RevealReservationMetrics holds Prometheus metrics for reveal-phase reservation
// operations (spec 005-reveal-reservation). Mirrors ReservationMetrics so the
// two reservation lifecycles can be observed independently.
type RevealReservationMetrics struct {
	ActiveReservations    prometheus.Gauge
	ReservationsCreated   prometheus.Counter
	ReservationsExpired   prometheus.Counter
	ReservationsCancelled prometheus.Counter
	ReservationsRevealed  prometheus.Counter
	ReservationErrors     *prometheus.CounterVec
}

// NewRevealReservationMetrics creates and registers reveal-reservation metrics.
func NewRevealReservationMetrics() *RevealReservationMetrics {
	return &RevealReservationMetrics{
		ActiveReservations: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "reveal_reservations_active",
			Help: "Current number of active reveal-phase reservations",
		}),
		ReservationsCreated: promauto.NewCounter(prometheus.CounterOpts{
			Name: "reveal_reservations_created_total",
			Help: "Total number of reveal-phase reservations created",
		}),
		ReservationsExpired: promauto.NewCounter(prometheus.CounterOpts{
			Name: "reveal_reservations_expired_total",
			Help: "Total number of reveal-phase reservations that expired",
		}),
		ReservationsCancelled: promauto.NewCounter(prometheus.CounterOpts{
			Name: "reveal_reservations_cancelled_total",
			Help: "Total number of reveal-phase reservations cancelled by the holder",
		}),
		ReservationsRevealed: promauto.NewCounter(prometheus.CounterOpts{
			Name: "reveal_reservations_revealed_total",
			Help: "Total number of reveal-phase reservations released because the game reached a terminal on-chain status",
		}),
		ReservationErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "reveal_reservation_errors_total",
				Help: "Total number of reveal-reservation operation errors by type",
			},
			[]string{"error_type"},
		),
	}
}

// RecordCreated records a successful reveal-reservation creation.
func (m *RevealReservationMetrics) RecordCreated() {
	m.ReservationsCreated.Inc()
	m.ActiveReservations.Inc()
}

// RecordExpired records a reveal-reservation expiration.
func (m *RevealReservationMetrics) RecordExpired() {
	m.ReservationsExpired.Inc()
	m.ActiveReservations.Dec()
}

// RecordCancelled records a holder-initiated cancellation.
func (m *RevealReservationMetrics) RecordCancelled() {
	m.ReservationsCancelled.Inc()
	m.ActiveReservations.Dec()
}

// RecordRevealed records a release triggered by an on-chain terminal status.
func (m *RevealReservationMetrics) RecordRevealed() {
	m.ReservationsRevealed.Inc()
	m.ActiveReservations.Dec()
}

// RecordError records a reveal-reservation operation error by type.
func (m *RevealReservationMetrics) RecordError(errorType string) {
	m.ReservationErrors.WithLabelValues(errorType).Inc()
}

// SetActiveCount sets the current active reveal-reservation count.
func (m *RevealReservationMetrics) SetActiveCount(count int) {
	m.ActiveReservations.Set(float64(count))
}
