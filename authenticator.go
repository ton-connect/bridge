package main

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"golang.org/x/time/rate"
)

const (
	maxStreamingConnectionsPerIP = 100
)

// Authenticator authenticates and authorizes a request.
// Strictly speaking it should be AuthenticatorAndAuthorizer
type Authenticator struct {
	mu          sync.Mutex
	ipLimits    *rateLimiter
	connections map[string]int
}

type rateLimiter struct {
	m map[string]*rate.Limiter
	sync.RWMutex
}

func newAuthenticator() *Authenticator {
	return &Authenticator{
		ipLimits:    newRateLimiter(),
		connections: map[string]int{},
	}
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{m: map[string]*rate.Limiter{}}
}

// leaseConnection increases a number of connections per given token and
// returns a release function to be called once a request is finished.
// If the token reaches the limit of max simultaneous connections, leaseConnection returns an error.
func (auth *Authenticator) leaseConnection(request *http.Request) (release func(), err error) {
	key := fmt.Sprintf("ip-%v", realIP(request))
	max := maxStreamingConnectionsPerIP

	auth.mu.Lock()
	defer auth.mu.Unlock()

	if auth.connections[key] >= max {
		return nil, fmt.Errorf("you have reached the limit of streaming connections: %v max", max)
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

func realIP(request *http.Request) string {
	// Fall back to legacy behavior
	if ip := request.Header.Get("X-Forwarded-For"); ip != "" {
		i := strings.IndexAny(ip, ",")
		if i > 0 {
			return strings.Trim(ip[:i], "[] \t")
		}
		return ip
	}
	if ip := request.Header.Get("X-Real-Ip"); ip != "" {
		return strings.Trim(ip, "[]")
	}
	ra, _, _ := net.SplitHostPort(request.RemoteAddr)
	return ra
}
