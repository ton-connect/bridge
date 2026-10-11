package storagev3

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/ton-connect/bridge/internal/models"
)

func TestMemStorage_CountsMessagesDroppedOnFullSubscriber(t *testing.T) {
	assertSubscriberDropsCounted(t, NewMemStorage(nil, nil), "1")
}

func TestValkeyStorage_CountsMessagesDroppedOnFullSubscriber(t *testing.T) {
	s, err := NewValkeyStorage(getTestValkeyURI(t))
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	assertSubscriberDropsCounted(t, s, fmt.Sprintf("drop-test-%d", time.Now().UnixNano()))
}

func assertSubscriberDropsCounted(t *testing.T, s Storage, key string) {
	t.Helper()
	ctx := context.Background()
	for id := int64(1); id <= 3; id++ {
		if err := s.Pub(ctx, models.SseMessage{EventId: id, To: key, Message: []byte("m")}, 60); err != nil {
			t.Fatalf("Pub() error = %v", err)
		}
	}
	replay := MessagesDropped.WithLabelValues(DropPathReplay, DropReasonSubscriberFull)
	live := MessagesDropped.WithLabelValues(DropPathLive, DropReasonSubscriberFull)
	replayBefore, liveBefore := testutil.ToFloat64(replay), testutil.ToFloat64(live)

	ch := make(chan models.SseMessage, 1)
	if err := s.Sub(ctx, []string{key}, 0, ch); err != nil {
		t.Fatalf("Sub() error = %v", err)
	}
	defer func() { _ = s.Unsub(ctx, []string{key}, ch) }()
	if got := testutil.ToFloat64(replay) - replayBefore; got != 2 {
		t.Errorf("replay drops = %v, want 2", got)
	}

	// A live subscription may become active after Sub returns, so publishing repeats until one lands.
	deadline := time.Now().Add(5 * time.Second)
	for id := int64(4); testutil.ToFloat64(live) == liveBefore && time.Now().Before(deadline); id++ {
		if err := s.Pub(ctx, models.SseMessage{EventId: id, To: key, Message: []byte("m")}, 60); err != nil {
			t.Fatalf("Pub() error = %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if testutil.ToFloat64(live) == liveBefore {
		t.Error("live drops did not grow while the subscriber channel was full")
	}
}
