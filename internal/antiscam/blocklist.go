package antiscam

import (
	"bufio"
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// DomainChecker checks whether an origin should be blocked.
type DomainChecker interface {
	IsBlocked(origin string) bool
}

// Blocklist fetches a newline-separated domain list from a URL and refreshes it periodically.
type Blocklist struct {
	mu              sync.RWMutex
	domains         map[string]struct{}
	url             string
	refreshInterval time.Duration
	client          *http.Client
	stopCh          chan struct{}
}

// NewBlocklist creates a new Blocklist that will fetch domains from the given URL.
func NewBlocklist(url string, refreshInterval time.Duration) *Blocklist {
	return &Blocklist{
		domains:         make(map[string]struct{}),
		url:             url,
		refreshInterval: refreshInterval,
		client:          &http.Client{Timeout: 30 * time.Second},
		stopCh:          make(chan struct{}),
	}
}

// Start performs an initial fetch and starts a background goroutine that refreshes the blocklist.
func (b *Blocklist) Start(ctx context.Context) {
	b.refresh(ctx)

	go func() {
		ticker := time.NewTicker(b.refreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-b.stopCh:
				return
			case <-ticker.C:
				b.refresh(ctx)
			}
		}
	}()
}

// Stop signals the background goroutine to exit.
func (b *Blocklist) Stop() {
	close(b.stopCh)
}

// IsBlocked returns true if the host extracted from origin matches a blocked domain.
// It walks up the domain hierarchy (e.g. sub.evil.com → evil.com → com).
func (b *Blocklist) IsBlocked(origin string) bool {
	if origin == "" {
		return false
	}

	host := extractHost(origin)
	if host == "" {
		return false
	}

	host = strings.ToLower(host)

	b.mu.RLock()
	defer b.mu.RUnlock()

	parts := strings.Split(host, ".")
	for i := range parts {
		candidate := strings.Join(parts[i:], ".")
		if _, ok := b.domains[candidate]; ok {
			return true
		}
	}
	return false
}

func extractHost(origin string) string {
	u, err := url.Parse(origin)
	if err == nil && u.Host != "" {
		host := u.Hostname() // strips port
		return host
	}
	// fallback: treat as bare host, strip port
	host := origin
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		h := host[:idx]
		if h != "" {
			return h
		}
	}
	return host
}

func (b *Blocklist) refresh(ctx context.Context) {
	log := logrus.WithField("prefix", "antiscam.Blocklist")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.url, nil)
	if err != nil {
		BlocklistRefreshErrorsMetric.Inc()
		log.Warnf("failed to create request for blocklist: %v", err)
		return
	}

	resp, err := b.client.Do(req)
	if err != nil {
		BlocklistRefreshErrorsMetric.Inc()
		log.Warnf("failed to fetch blocklist: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		BlocklistRefreshErrorsMetric.Inc()
		log.Warnf("blocklist fetch returned status %d", resp.StatusCode)
		return
	}

	newDomains := make(map[string]struct{})
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		newDomains[strings.ToLower(line)] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		BlocklistRefreshErrorsMetric.Inc()
		log.Warnf("error reading blocklist body: %v", err)
		return
	}

	b.mu.Lock()
	b.domains = newDomains
	b.mu.Unlock()

	BlocklistSizeMetric.Set(float64(len(newDomains)))
	log.Infof("blocklist refreshed: %d domains loaded", len(newDomains))
}
