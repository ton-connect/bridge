package main

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/tonkeeper/bridge/config"
	"golang.org/x/exp/slices"
)

var tokenUsageMetric = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "bridge_token_usage",
}, []string{"token"})

// ConnectionsLimiter is a middleware that limits the number of simultaneous connections per IP.
type ConnectionsLimiter struct {
	mu          sync.Mutex
	connections map[string]int
	max         int
	realIP      *realIPExtractor
}

func newConnectionLimiter(i int, extractor *realIPExtractor) *ConnectionsLimiter {
	return &ConnectionsLimiter{
		connections: map[string]int{},
		max:         i,
		realIP:      extractor,
	}
}

// leaseConnection increases a number of connections per given token and
// returns a release function to be called once a request is finished.
// If the token reaches the limit of max simultaneous connections, leaseConnection returns an error.
func (auth *ConnectionsLimiter) leaseConnection(request *http.Request) (release func(), err error) {
	key := fmt.Sprintf("ip-%v", auth.realIP.Extract(request))
	auth.mu.Lock()
	defer auth.mu.Unlock()

	if auth.connections[key] >= auth.max {
		return nil, fmt.Errorf("you have reached the limit of streaming connections: %v max", auth.max)
	}
	auth.connections[key] += 1

	return func() {
		auth.mu.Lock()
		defer auth.mu.Unlock()
		auth.connections[key] -= 1
		if auth.connections[key] == 0 {
			delete(auth.connections, key)
		}
	}, nil
}

func skipRateLimitsByToken(request *http.Request) bool {
	if request == nil {
		return false
	}
	authorization := request.Header.Get("Authorization")
	if authorization == "" {
		return false
	}
	token := strings.TrimPrefix(authorization, "Bearer ")
	exist := slices.Contains(config.Config.RateLimitsByPassToken, token)
	if exist {
		tokenUsageMetric.WithLabelValues(token).Inc()
		return true
	}
	return false
}
