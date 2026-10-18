package handlerv3

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/ton-connect/bridge/internal/config"
	"github.com/ton-connect/bridge/internal/models"
	"github.com/ton-connect/bridge/internal/ntp"
	"github.com/ton-connect/bridge/internal/utils"
	storagev3 "github.com/ton-connect/bridge/internal/v3/storage"
)

func TestSession_StartAfterCloseDoesNotSubscribe(t *testing.T) {
	store := storagev3.NewMemStorage(nil, nil)
	session := NewSession(store, []string{defaultClientID}, 0)

	session.Close()
	session.Start()

	msg := models.SseMessage{EventId: 1, To: defaultClientID, Message: []byte(`{}`)}
	if err := store.Pub(context.Background(), msg, 60); err != nil {
		t.Fatalf("Pub() error = %v", err)
	}
	if got, ok := <-session.GetMessages(); ok {
		t.Fatalf("a closed session received %+v", got)
	}
}

// failingSubStorage registers the subscriber and then fails, the way a Valkey subscribe
// error leaves it.
type failingSubStorage struct {
	storagev3.Storage
}

func (s *failingSubStorage) Sub(ctx context.Context, keys []string, lastEventId int64, ch chan<- models.SseMessage) error {
	if err := s.Storage.Sub(ctx, keys, lastEventId, ch); err != nil {
		return err
	}
	return errors.New("subscribe failed")
}

func TestSession_FailedStartUnsubscribesBeforeClosing(t *testing.T) {
	store := &failingSubStorage{Storage: storagev3.NewMemStorage(nil, nil)}
	session := NewSession(store, []string{defaultClientID}, 0)

	session.Start()
	msg := models.SseMessage{EventId: 1, To: defaultClientID, Message: []byte(`{}`)}
	if err := store.Pub(context.Background(), msg, 60); err != nil {
		t.Fatalf("Pub() error = %v", err)
	}
	session.Close()
}

// A request cancelled before the handler reaches Start, the way one cancelled during
// AddConnection is, makes the session's close race its subscribe.
func TestEventRegistrationHandler_ClientGoneBeforeStart(t *testing.T) {
	lifetime := config.Config.SSEMaxLifetime
	config.Config.SSEMaxLifetime = 60
	t.Cleanup(func() { config.Config.SSEMaxLifetime = lifetime })

	store := storagev3.NewMemStorage(nil, nil)
	extractor, err := utils.NewRealIPExtractor([]string{})
	if err != nil {
		t.Fatalf("failed to create RealIPExtractor: %v", err)
	}
	h := NewHandler(store, 10*time.Second, extractor, ntp.NewLocalTimeProvider(), nil, nil)

	// A client id per request keeps the handler's per-client connection list out of the race.
	const workers, requests = 8, 500
	clientID := func(n int) string { return fmt.Sprintf("%064x", n) }

	e := echo.New()
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := range requests {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				req := httptest.NewRequest(http.MethodGet, "/bridge/events?client_id="+clientID(w*requests+i), nil).WithContext(ctx)
				_ = h.EventRegistrationHandler(e.NewContext(req, httptest.NewRecorder()))
			}
		})
	}
	wg.Wait()

	for n := range workers * requests {
		msg := models.SseMessage{EventId: 1, To: clientID(n), Message: []byte(`{}`)}
		if err := store.Pub(context.Background(), msg, 60); err != nil {
			t.Fatalf("Pub() error = %v", err)
		}
	}
}
