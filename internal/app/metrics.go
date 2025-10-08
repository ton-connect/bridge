package app

import (
	client_prometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tonkeeper/bridge/internal"
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

	BridgeEngineMetric = client_prometheus.NewGaugeVec(client_prometheus.GaugeOpts{
		Name: "bridge_info_engine",
		Help: "Bridge engine information",
	}, []string{"engine"})

	BridgeVersionMetric = client_prometheus.NewGaugeVec(client_prometheus.GaugeOpts{
		Name: "bridge_info_version",
		Help: "Bridge version information",
	}, []string{"version"})

	BridgeStorageMetric = client_prometheus.NewGaugeVec(client_prometheus.GaugeOpts{
		Name: "bridge_info_storage",
		Help: "Bridge storage backend information",
	}, []string{"storage"})
)

// InitMetrics registers all Prometheus metrics and sets version info
func InitMetrics() {
	client_prometheus.MustRegister(HealthMetric)
	client_prometheus.MustRegister(ReadyMetric)
	client_prometheus.MustRegister(BridgeEngineMetric)
	client_prometheus.MustRegister(BridgeVersionMetric)
	client_prometheus.MustRegister(BridgeStorageMetric)
}

// SetBridgeInfo sets the bridge_info metrics with engine, version, and storage labels
func SetBridgeInfo(engine, storage string) {
	BridgeEngineMetric.WithLabelValues(engine).Set(1)
	BridgeVersionMetric.WithLabelValues(internal.BridgeVersionRevision).Set(1)
	BridgeStorageMetric.WithLabelValues(storage).Set(1)
}
