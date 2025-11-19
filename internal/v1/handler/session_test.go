package handlerv1

import (
	"context"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/ton-connect/bridge/internal/models"
)

// mockDB implements the db interface for testing
type mockDB struct{}

func (m *mockDB) GetMessages(ctx context.Context, clientIds []string, lastEventId int64) ([]models.SseMessage, error) {
	// Return empty slice for testing
	return []models.SseMessage{}, nil
}

func (m *mockDB) Add(ctx context.Context, mes models.SseMessage, ttl int64) error {
	// Mock implementation - just return nil
	return nil
}

func (m *mockDB) HealthCheck() error {
	return nil // Always healthy
}

// TestMultipleRuns runs the panic test multiple times to increase chances of hitting the race condition
func TestMultipleRuns(t *testing.T) {
	runs := 10 // Reduced runs for faster testing

	var wg sync.WaitGroup

	for i := 0; i < runs; i++ {
		log.Print("TestMultipleRuns run", i)

		wg.Add(1)
		go func(runNum int) {
			defer wg.Done()

			mockDb := &mockDB{}
			session := NewSession(mockDb, []string{"client1"}, 0)

			heartbeatInterval := 1 * time.Microsecond

			session.Start("heartbeat", false, heartbeatInterval)

			// Random small delay to vary timing
			time.Sleep(5 * time.Microsecond)

			close(session.Closer)
			time.Sleep(200 * time.Microsecond)
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
}
