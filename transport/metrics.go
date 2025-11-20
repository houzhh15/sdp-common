package transport

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// tunnelTotal tracks the total number of tunnels by status
	// Labels: status (active, pending, failed)
	tunnelTotal = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tunnel_total",
			Help: "Total number of tunnels grouped by status",
		},
		[]string{"status"},
	)

	// tunnelBytesTransferred tracks the total bytes transferred through tunnels
	tunnelBytesTransferred = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "tunnel_bytes_transferred_total",
			Help: "Total bytes transferred through tunnels",
		},
	)

	// tunnelPairingDuration tracks the duration of tunnel pairing operations
	// Buckets: 0.01s, 0.05s, 0.1s, 0.5s, 1s, 5s, 10s
	tunnelPairingDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "tunnel_pairing_duration_seconds",
			Help:    "Duration of tunnel pairing operations in seconds",
			Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10},
		},
	)

	// tunnelRelayErrors tracks the total number of relay errors by reason
	// Labels: reason (pairing_timeout, read_error, write_error, validation_error, unknown)
	tunnelRelayErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tunnel_relay_errors_total",
			Help: "Total number of tunnel relay errors grouped by reason",
		},
		[]string{"reason"},
	)
)

// updateTunnelMetrics updates the tunnel total metrics based on current state
func (s *tunnelRelayServer) updateTunnelMetrics() {
	stats := s.GetStats()
	
	// Update gauge for active tunnels
	tunnelTotal.WithLabelValues("active").Set(float64(stats.ActiveTunnels))
	
	// Update gauge for pending connections
	tunnelTotal.WithLabelValues("pending").Set(float64(stats.PendingConnections))
}

// recordPairingDuration records the duration of a pairing operation
func recordPairingDuration(duration float64) {
	tunnelPairingDuration.Observe(duration)
}

// recordBytesTransferred records the number of bytes transferred
func recordBytesTransferred(bytes uint64) {
	tunnelBytesTransferred.Add(float64(bytes))
}

// recordRelayError records a relay error with the given reason
func recordRelayError(reason string) {
	tunnelRelayErrors.WithLabelValues(reason).Inc()
}
