package app

import (
	client_prometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/ton-connect/bridge/internal"
)

var (
	TokenUsageMetric = promauto.NewCounterVec(client_prometheus.CounterOpts{
		Name: "bridge_token_usage",
	}, []string{"token"})

	HealthMetric = client_prometheus.NewGauge(client_prometheus.GaugeOpts{
		Name: "bridge_health_status",
		Help: "Health status of the bridge (1 = healthy, 0 = unhealthy)",
	})

	ReadyMetric = client_prometheus.NewGauge(client_prometheus.GaugeOpts{
		Name: "bridge_ready_status",
		Help: "Ready status of the bridge (1 = ready, 0 = not ready)",
	})

	BridgeInfoMetric = client_prometheus.NewGaugeVec(client_prometheus.GaugeOpts{
		Name: "bridge_info",
		Help: "Bridge information with engine type, version, and storage backend",
	}, []string{"engine", "version", "storage"})
)

// InitMetrics registers all Prometheus metrics and sets version info
func InitMetrics() {
	client_prometheus.MustRegister(HealthMetric)
	client_prometheus.MustRegister(ReadyMetric)
	client_prometheus.MustRegister(BridgeInfoMetric)
}

// SetBridgeInfo sets the bridge_info metric with engine, version, and storage labels
func SetBridgeInfo(engine, storage string) {
	BridgeInfoMetric.With(client_prometheus.Labels{
		"engine":  engine,
		"version": internal.BridgeVersionRevision,
		"storage": storage,
	}).Set(1)
}
