package handlerv3

import (
	"context"
	"errors"
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

// markRecorder is a storage that hands Sub one queued message and records every delivery mark.
type markRecorder struct {
	storagev3.Storage
	mu    sync.Mutex
	marks []int64
}

func (s *markRecorder) Sub(_ context.Context, keys []string, _ int64, messageCh chan<- models.SseMessage) error {
	messageCh <- models.SseMessage{EventId: 1, Message: []byte(`{"from":"sender","message":"m"}`), To: keys[0]}
	return nil
}

func (s *markRecorder) Unsub(context.Context, []string, chan<- models.SseMessage) error { return nil }

func (s *markRecorder) AddConnection(context.Context, storagev3.ConnectionInfo, time.Duration) error {
	return nil
}

func (s *markRecorder) MarkDelivered(_ context.Context, _ string, eventID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.marks = append(s.marks, eventID)
	return nil
}

// closedAfterFirstFlush is a response whose connection is gone after the opening frame: writes still
// land in the buffer, and only a flush reports the closed connection, as net/http does.
type closedAfterFirstFlush struct {
	*httptest.ResponseRecorder
	flushes int
}

func (w *closedAfterFirstFlush) FlushError() error {
	w.flushes++
	if w.flushes > 1 {
		return errors.New("use of closed network connection")
	}
	w.Flush()
	return nil
}

// TestEventRegistrationHandler_FailedFlushIsNotMarked delivers a message onto a connection that is
// already closed. A message whose flush failed never reached the client, so it must not be marked.
func TestEventRegistrationHandler_FailedFlushIsNotMarked(t *testing.T) {
	storage := &markRecorder{}
	extractor, err := utils.NewRealIPExtractor([]string{})
	if err != nil {
		t.Fatalf("NewRealIPExtractor failed: %v", err)
	}
	h := NewHandler(storage, time.Hour, extractor, ntp.NewLocalTimeProvider(), nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/bridge/events?client_id="+defaultClientID, nil).WithContext(ctx)
	w := &closedAfterFirstFlush{ResponseRecorder: httptest.NewRecorder()}
	c := echo.New().NewContext(req, w)

	done := make(chan error, 1)
	go func() { done <- h.EventRegistrationHandler(c) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the handler kept running after its flush failed")
	}

	storage.mu.Lock()
	defer storage.mu.Unlock()
	if len(storage.marks) != 0 {
		t.Errorf("messages %v were marked delivered although their flush failed", storage.marks)
	}
}

// TestWaitStreams_WaitsForRunningHandler keeps one event stream open. WaitStreams must give up at its
// deadline while the handler runs, and return once the handler has, with its marks handed over.
func TestWaitStreams_WaitsForRunningHandler(t *testing.T) {
	lifetime := config.Config.SSEMaxLifetime
	config.Config.SSEMaxLifetime = 3600
	defer func() { config.Config.SSEMaxLifetime = lifetime }()

	storage := &markRecorder{}
	extractor, err := utils.NewRealIPExtractor([]string{})
	if err != nil {
		t.Fatalf("NewRealIPExtractor failed: %v", err)
	}
	h := NewHandler(storage, time.Hour, extractor, ntp.NewLocalTimeProvider(), nil, nil)

	reqCtx, closeConn := context.WithCancel(context.Background())
	defer closeConn()
	req := httptest.NewRequest(http.MethodGet, "/bridge/events?client_id="+defaultClientID, nil).WithContext(reqCtx)
	c := echo.New().NewContext(req, httptest.NewRecorder())
	done := make(chan error, 1)
	go func() { done <- h.EventRegistrationHandler(c) }()

	deadline := time.Now().Add(5 * time.Second)
	for h.streams.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the handler did not start within 5s")
		}
		time.Sleep(time.Millisecond)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := h.WaitStreams(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitStreams returned %v with a handler still running, want its deadline", err)
	}
	if waited := time.Since(started); waited > time.Second {
		t.Errorf("WaitStreams took %v to give up on a 100ms deadline", waited)
	}

	closeConn()
	waitCtx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.WaitStreams(waitCtx); err != nil {
		t.Fatalf("WaitStreams failed after the connection closed: %v", err)
	}
	<-done
	storage.mu.Lock()
	defer storage.mu.Unlock()
	if len(storage.marks) != 1 {
		t.Errorf("the delivered message left %d marks by the time WaitStreams returned, want 1", len(storage.marks))
	}
}
