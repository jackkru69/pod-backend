package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ReservationMetrics holds Prometheus metrics for game reservation operations (T049).
type ReservationMetrics struct {
	// ActiveReservations tracks the current number of active reservations
	ActiveReservations prometheus.Gauge

	// ReservationsCreated tracks total reservations created
	ReservationsCreated prometheus.Counter

	// ReservationsExpired tracks total reservations that expired
	ReservationsExpired prometheus.Counter

	// ReservationsCancelled tracks total reservations that were cancelled by user
	ReservationsCancelled prometheus.Counter

	// ReservationsJoined tracks total reservations that were released because user joined game
	ReservationsJoined prometheus.Counter

	// ReservationErrors tracks reservation operation errors by error type
	ReservationErrors *prometheus.CounterVec

	// ReservationsByWallet tracks active reservations per wallet (gauge)
	ReservationsByWallet *prometheus.GaugeVec
}

// NewReservationMetrics creates and registers reservation metrics (T049).
func NewReservationMetrics() *ReservationMetrics {
	return &ReservationMetrics{
		ActiveReservations: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "game_reservations_active",
				Help: "Current number of active game reservations",
			},
		),

		ReservationsCreated: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "game_reservations_created_total",
				Help: "Total number of game reservations created",
			},
		),

		ReservationsExpired: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "game_reservations_expired_total",
				Help: "Total number of game reservations that expired",
			},
		),

		ReservationsCancelled: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "game_reservations_cancelled_total",
				Help: "Total number of game reservations cancelled by user",
			},
		),

		ReservationsJoined: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "game_reservations_joined_total",
				Help: "Total number of reservations released because user joined game",
			},
		),

		ReservationErrors: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "game_reservation_errors_total",
				Help: "Total number of reservation operation errors by type",
			},
			[]string{"error_type"},
		),

		ReservationsByWallet: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "game_reservations_by_wallet",
				Help: "Active reservations per wallet",
			},
			[]string{"wallet"},
		),
	}
}

// RecordCreated records a successful reservation creation
func (m *ReservationMetrics) RecordCreated() {
	m.ReservationsCreated.Inc()
	m.ActiveReservations.Inc()
}

// RecordExpired records a reservation expiration
func (m *ReservationMetrics) RecordExpired() {
	m.ReservationsExpired.Inc()
	m.ActiveReservations.Dec()
}

// RecordCancelled records a user-initiated reservation cancellation
func (m *ReservationMetrics) RecordCancelled() {
	m.ReservationsCancelled.Inc()
	m.ActiveReservations.Dec()
}

// RecordJoined records a reservation release due to game join
func (m *ReservationMetrics) RecordJoined() {
	m.ReservationsJoined.Inc()
	m.ActiveReservations.Dec()
}

// RecordError records a reservation operation error
func (m *ReservationMetrics) RecordError(errorType string) {
	m.ReservationErrors.WithLabelValues(errorType).Inc()
}

// SetActiveCount sets the current active reservation count
func (m *ReservationMetrics) SetActiveCount(count int) {
	m.ActiveReservations.Set(float64(count))
}

// SetWalletReservations sets the reservation count for a specific wallet
func (m *ReservationMetrics) SetWalletReservations(wallet string, count int) {
	m.ReservationsByWallet.WithLabelValues(wallet).Set(float64(count))
}
