package antiscam

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	BlockedPushesMetric = promauto.NewCounter(prometheus.CounterOpts{
		Name: "antiscam_blocked_pushes_total",
		Help: "Total number of push messages silently dropped by antiscam filter",
	})
	PoisonedConnectionsMetric = promauto.NewCounter(prometheus.CounterOpts{
		Name: "antiscam_poisoned_connections_total",
		Help: "Total number of SSE connections poisoned by antiscam filter",
	})
	BlocklistSizeMetric = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "antiscam_blocklist_size",
		Help: "Current number of domains in the antiscam blocklist",
	})
	BlocklistRefreshErrorsMetric = promauto.NewCounter(prometheus.CounterOpts{
		Name: "antiscam_blocklist_refresh_errors_total",
		Help: "Total number of blocklist refresh failures",
	})
)
