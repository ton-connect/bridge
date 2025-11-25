package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
)

type AnalyticsMock struct {
	mu             sync.RWMutex
	receivedEvents []map[string]interface{}
	totalEvents    int
}

func NewAnalyticsMock() *AnalyticsMock {
	return &AnalyticsMock{
		receivedEvents: make([]map[string]interface{}, 0),
	}
}

func (m *AnalyticsMock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/health" {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			log.Printf("Failed to write health response: %v", err)
		}
		return
	}

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
		log.Printf("Failed to read body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var events []map[string]interface{}
	if err := json.Unmarshal(body, &events); err != nil {
		log.Printf("Failed to unmarshal events: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	m.receivedEvents = append(m.receivedEvents, events...)
	m.totalEvents += len(events)
	total := m.totalEvents
	m.mu.Unlock()

	log.Printf("✅ Received batch of %d events (total: %d)", len(events), total)
	for _, event := range events {
		if eventName, ok := event["event_name"].(string); ok {
			log.Printf("   - %s", eventName)
		}
	}

	// Return 202 Accepted like the real analytics server
	w.WriteHeader(http.StatusAccepted)
}

func (m *AnalyticsMock) GetStats(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	eventTypes := make(map[string]int)
	for _, event := range m.receivedEvents {
		if eventName, ok := event["event_name"].(string); ok {
			eventTypes[eventName]++
		}
	}

	stats := map[string]interface{}{
		"total_events": m.totalEvents,
		"event_types":  eventTypes,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		log.Printf("Failed to encode stats: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func main() {
	mock := NewAnalyticsMock()

	http.HandleFunc("/events", mock.ServeHTTP)
	http.HandleFunc("/health", mock.ServeHTTP)
	http.HandleFunc("/stats", mock.GetStats)

	port := ":9090"
	log.Printf("🚀 Analytics Mock Server starting on %s", port)
	log.Printf("📊 Endpoints:")
	log.Printf("   POST /events - Receive analytics events")
	log.Printf("   GET  /health - Health check")
	log.Printf("   GET  /stats  - Get statistics")
	log.Printf("")
	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatal(err)
	}
}
