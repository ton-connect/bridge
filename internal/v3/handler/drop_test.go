package handlerv3

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/ton-connect/bridge/internal/config"
	"github.com/ton-connect/bridge/internal/models"
	"github.com/ton-connect/bridge/internal/ntp"
	"github.com/ton-connect/bridge/internal/utils"
	storagev3 "github.com/ton-connect/bridge/internal/v3/storage"
)

type failingSSEWriter struct {
	*httptest.ResponseRecorder
	failWrite     bool
	failFlush     bool
	messageWrites int
}

func (w *failingSSEWriter) Write(p []byte) (int, error) {
	if bytes.HasPrefix(p, []byte("event: message")) {
		w.messageWrites++
		if w.failWrite {
			return 0, errors.New("connection reset")
		}
	}
	return w.ResponseRecorder.Write(p)
}

func (w *failingSSEWriter) FlushError() error {
	if w.failFlush && strings.Contains(w.Body.String(), "event: message") {
		return errors.New("connection reset")
	}
	w.Flush()
	return nil
}

func TestEventRegistrationHandler_CountsMessageLostToFailedWrite(t *testing.T) {
	for _, tc := range []struct {
		name      string
		failWrite bool
		failFlush bool
	}{
		{name: "write fails", failWrite: true},
		{name: "flush fails", failFlush: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// An unloaded config leaves the lifetime at zero, which closes the session before it starts.
			lifetime := config.Config.SSEMaxLifetime
			config.Config.SSEMaxLifetime = 60
			t.Cleanup(func() { config.Config.SSEMaxLifetime = lifetime })

			memStorage := storagev3.NewMemStorage(nil, nil)
			msg := models.SseMessage{EventId: 1, To: defaultClientID, Message: []byte(`{}`)}
			if err := memStorage.Pub(context.Background(), msg, 60); err != nil {
				t.Fatalf("Pub() error = %v", err)
			}
			extractor, err := utils.NewRealIPExtractor([]string{})
			if err != nil {
				t.Fatalf("failed to create RealIPExtractor: %v", err)
			}
			h := NewHandler(memStorage, 10*time.Second, extractor, ntp.NewLocalTimeProvider(), nil, nil)

			dropped := storagev3.MessagesDropped.WithLabelValues(storagev3.DropPathSSE, storagev3.DropReasonWriteError)
			before := testutil.ToFloat64(dropped)

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			req := httptest.NewRequest(http.MethodGet, "/bridge/events?client_id="+defaultClientID, nil).WithContext(ctx)
			w := &failingSSEWriter{ResponseRecorder: httptest.NewRecorder(), failWrite: tc.failWrite, failFlush: tc.failFlush}
			_ = h.EventRegistrationHandler(echo.New().NewContext(req, w))
			if w.messageWrites == 0 {
				t.Fatalf("the message never reached the writer: %q", w.Body.String())
			}

			if got := testutil.ToFloat64(dropped) - before; got != 1 {
				t.Errorf("sse write_error drops = %v, want 1", got)
			}
		})
	}
}
