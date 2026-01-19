package app

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge/internal"
)

// HealthChecker interface for storage health checking
type HealthChecker interface {
	HealthCheck() error
}

// HealthManager manages the health status of the bridge
type HealthManager struct {
	healthy int64 // Use atomic for thread-safe access
}

// NewHealthManager creates a new health manager
func NewHealthManager() *HealthManager {
	return &HealthManager{healthy: 0}
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
			log.Errorf("health response write error: %v", err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	_, err := fmt.Fprintf(w, `{"status":"ok"}`+"\n")
	if err != nil {
		log.Errorf("health response write error: %v", err)
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
		log.Errorf("version response write error: %v", err)
	}
}
