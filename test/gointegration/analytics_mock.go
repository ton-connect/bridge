package bridge_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/sirupsen/logrus"
)

// AnalyticsMock is a mock analytics server for testing
type AnalyticsMock struct {
	Server         *httptest.Server
	mu             sync.RWMutex
	receivedEvents []map[string]interface{}
	totalEvents    int
}

// NewAnalyticsMock creates a new mock analytics server
func NewAnalyticsMock() *AnalyticsMock {
	mock := &AnalyticsMock{
		receivedEvents: make([]map[string]interface{}, 0),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		if r.URL.Path != "/events" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			logrus.Errorf("analytics mock: failed to read body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var events []map[string]interface{}
		if err := json.Unmarshal(body, &events); err != nil {
			logrus.Errorf("analytics mock: failed to unmarshal events: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		mock.mu.Lock()
		mock.receivedEvents = append(mock.receivedEvents, events...)
		mock.totalEvents += len(events)
		mock.mu.Unlock()

		logrus.Infof("analytics mock: received batch of %d events (total: %d)", len(events), mock.totalEvents)

		// Return 202 Accepted like the real analytics server
		w.WriteHeader(http.StatusAccepted)
	})

	mock.Server = httptest.NewServer(handler)
	return mock
}

// Close shuts down the mock server
func (m *AnalyticsMock) Close() {
	m.Server.Close()
}

// GetEvents returns all received events
func (m *AnalyticsMock) GetEvents() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	events := make([]map[string]interface{}, len(m.receivedEvents))
	copy(events, m.receivedEvents)
	return events
}

// GetEventCount returns the total number of events received
func (m *AnalyticsMock) GetEventCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.totalEvents
}

// GetEventsByName returns events filtered by event_name
func (m *AnalyticsMock) GetEventsByName(eventName string) []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	filtered := make([]map[string]interface{}, 0)
	for _, event := range m.receivedEvents {
		if name, ok := event["event_name"].(string); ok && name == eventName {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

// Reset clears all received events
func (m *AnalyticsMock) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.receivedEvents = make([]map[string]interface{}, 0)
	m.totalEvents = 0
}
