package middleware

import (
	"fmt"
	"github.com/ton-connect/bridge/internal/utils"
	"net/http"
	"sync"
)

// ConnectionsLimiter is a middleware that limits the number of simultaneous connections per IP.
type ConnectionsLimiter struct {
	mu          sync.Mutex
	connections map[string]int
	max         int
	realIP      *utils.RealIPExtractor
}

func NewConnectionLimiter(i int, extractor *utils.RealIPExtractor) *ConnectionsLimiter {
	return &ConnectionsLimiter{
		connections: map[string]int{},
		max:         i,
		realIP:      extractor,
	}
}

// leaseConnection increases a number of connections per given token and
// returns a release function to be called once a request is finished.
// If the token reaches the limit of max simultaneous connections, leaseConnection returns an error.
func (auth *ConnectionsLimiter) LeaseConnection(request *http.Request) (release func(), err error) {
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
