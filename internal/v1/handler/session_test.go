package handlerv1

import (
	"context"
	"log"
	"sync"
	"testing"
	"time"

	"github.com/tonkeeper/bridge/datatype"
)

// mockDB implements the db interface for testing
type mockDB struct{}

func (m *mockDB) GetMessages(ctx context.Context, clientIds []string, lastEventId int64) ([]datatype.SseMessage, error) {
	// Return empty slice for testing
	return []datatype.SseMessage{}, nil
}

func (m *mockDB) Add(ctx context.Context, mes datatype.SseMessage, ttl int64) error {
	// Mock implementation - just return nil
	return nil
}

// TestMultipleRuns runs the panic test multiple times to increase chances of hitting the race condition
func TestMultipleRuns(t *testing.T) {
	runs := 100 // Reduced runs for faster testing

	var wg sync.WaitGroup

	for i := 0; i < runs; i++ {
		log.Print("TestMultipleRuns run", i)

		wg.Add(1)
		go func(runNum int) {
			defer wg.Done()

			mockDb := &mockDB{}
			session := NewSession(mockDb, []string{"client1"}, 0)

			heartbeatInterval := 1 * time.Millisecond
			session.Start("heartbeat", false, heartbeatInterval)

			// Random small delay to vary timing
			time.Sleep(5 * time.Millisecond)

			close(session.Closer)
			time.Sleep(10 * time.Millisecond)
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
}
