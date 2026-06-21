package app

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/ton-connect/bridge/internal"
)

// HealthChecker interface for storage health checking
type HealthChecker interface {
	HealthCheck() error
}

// HealthManager manages the health status of the bridge
type HealthManager struct {
	healthy  int64 // storage reachability; updated by StartHealthMonitoring
	draining int64 // 1 once SIGTERM shutdown has begun; flips /readyz to 503
}

// NewHealthManager creates a new health manager
func NewHealthManager() *HealthManager {
	return &HealthManager{healthy: 0}
}

// SetDraining marks the bridge as shutting down so ReadinessHandler starts returning 503, taking
// the pod out of the load balancer rotation while it is still listening, before the HTTP server
// actually stops accepting connections.
func (h *HealthManager) SetDraining() {
	atomic.StoreInt64(&h.draining, 1)
	ReadyMetric.Set(0)
}

// UpdateHealthStatus checks storage health and updates metrics
func (h *HealthManager) UpdateHealthStatus(storage HealthChecker) {
	var healthStatus int64 = 1
	if err := storage.HealthCheck(); err != nil {
		healthStatus = 0
	}

	atomic.StoreInt64(&h.healthy, healthStatus)
	HealthMetric.Set(float64(healthStatus))
	ReadyMetric.Set(float64(healthStatus))
}

// StartHealthMonitoring starts a background goroutine to monitor health
func (h *HealthManager) StartHealthMonitoring(storage HealthChecker) {
	// Initial health check
	h.UpdateHealthStatus(storage)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		h.UpdateHealthStatus(storage)
	}
}

// HealthHandler returns HTTP handler for health endpoints
func (h *HealthManager) HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Build-Commit", internal.BridgeVersionRevision)

	healthy := atomic.LoadInt64(&h.healthy)
	if healthy == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, err := fmt.Fprintf(w, `{"status":"unhealthy"}`+"\n")
		if err != nil {
			slog.Error("health response write error", "err", err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	_, err := fmt.Fprintf(w, `{"status":"ok"}`+"\n")
	if err != nil {
		slog.Error("health response write error", "err", err)
	}
}

// LivenessHandler answers /healthz: it reports only that the PROCESS is alive and intentionally does
// NOT check Valkey. Liveness drives the kubelet's restart decision, so coupling it to a shared
// dependency would let one Valkey blip restart every replica at once (correlated eviction).
func (h *HealthManager) LivenessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Build-Commit", internal.BridgeVersionRevision)
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, `{"status":"ok"}`+"\n"); err != nil {
		slog.Error("liveness response write error", "err", err)
	}
}

// ReadinessHandler answers /readyz, the load balancer health-check and readiness probe target. It
// returns 503 once draining so the load balancer stops routing during shutdown, otherwise 200. It
// is deliberately NOT Valkey-coupled: Valkey is shared by all replicas, so failing readiness on a
// Valkey outage would take the whole pool out of rotation and 503 every request, whereas staying in
// rotation lets the bridge return per-request errors that the client SDK retries. Storage health is
// still exported via HealthMetric for alerting.
func (h *HealthManager) ReadinessHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Build-Commit", internal.BridgeVersionRevision)
	if atomic.LoadInt64(&h.draining) != 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := fmt.Fprintf(w, `{"status":"draining"}`+"\n"); err != nil {
			slog.Error("readiness response write error", "err", err)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, `{"status":"ok"}`+"\n"); err != nil {
		slog.Error("readiness response write error", "err", err)
	}
}

// VersionHandler returns HTTP handler for version endpoint
func VersionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Build-Commit", internal.BridgeVersionRevision)

	w.WriteHeader(http.StatusOK)
	response := fmt.Sprintf(`{"version":"%s"}`, internal.BridgeVersionRevision)
	_, err := fmt.Fprintf(w, "%s", response+"\n")
	if err != nil {
		slog.Error("version response write error", "err", err)
	}
}
