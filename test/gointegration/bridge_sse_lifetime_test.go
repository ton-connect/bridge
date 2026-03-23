package bridge_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/ton-connect/bridge/internal/config"
	"github.com/ton-connect/bridge/internal/ntp"
	handlerv3 "github.com/ton-connect/bridge/internal/v3/handler"
	storagev3 "github.com/ton-connect/bridge/internal/v3/storage"
)

// startLocalBridge starts a local bridge server with the given SSE lifetime config.
// Returns the server and a cleanup function.
func startLocalBridge(t *testing.T, maxLifetimeSec, jitterSec int) *httptest.Server {
	t.Helper()

	// Override global config for this test
	config.Config.SSEMaxLifetime = maxLifetimeSec
	config.Config.SSEMaxLifetimeJitter = jitterSec

	storage := storagev3.NewMemStorage(nil, nil)
	timeProvider := ntp.NewLocalTimeProvider()
	h := handlerv3.NewHandler(storage, 10*time.Second, nil, timeProvider, nil, nil)

	e := echo.New()
	e.GET("/bridge/events", h.EventRegistrationHandler)
	e.POST("/bridge/message", h.SendMessageHandler)

	srv := httptest.NewServer(e)
	return srv
}

func TestSSEMaxLifetime_ConnectionClosedAfterTimeout(t *testing.T) {
	srv := startLocalBridge(t, 2, 0) // 2s lifetime, no jitter
	defer srv.Close()

	bridgeURL := srv.URL + "/bridge"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session := randomSessionID(t)
	gw, err := OpenBridge(ctx, OpenOpts{
		BridgeURL: bridgeURL,
		SessionID: session,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = gw.Close() }()

	if !gw.IsReady() {
		t.Fatal("gateway not ready")
	}

	// Wait for connection to be closed by the server
	start := time.Now()
	_, err = gw.WaitMessage(ctx)
	elapsed := time.Since(start)

	// The connection should close after ~2 seconds (no jitter)
	if elapsed < 1500*time.Millisecond {
		t.Fatalf("connection closed too early: %v", elapsed)
	}
	if elapsed > 4*time.Second {
		t.Fatalf("connection stayed open too long: %v", elapsed)
	}

	t.Logf("connection closed after %v (expected ~2s)", elapsed)
}

func TestSSEMaxLifetime_ConnectionClosedWithJitter(t *testing.T) {
	srv := startLocalBridge(t, 2, 2) // 2s lifetime + up to 2s jitter = 2-4s
	defer srv.Close()

	bridgeURL := srv.URL + "/bridge"

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Open multiple connections and record when each closes
	const numConns = 5
	closeTimes := make(chan time.Duration, numConns)

	for i := 0; i < numConns; i++ {
		go func() {
			session := randomSessionID(t)
			gw, err := OpenBridge(ctx, OpenOpts{
				BridgeURL: bridgeURL,
				SessionID: session,
			})
			if err != nil {
				closeTimes <- 0
				return
			}
			defer func() { _ = gw.Close() }()

			start := time.Now()
			// Wait for the connection to be closed by server
			for {
				_, err := gw.WaitMessage(ctx)
				if err != nil {
					break
				}
			}
			closeTimes <- time.Since(start)
		}()
	}

	var durations []time.Duration
	for i := 0; i < numConns; i++ {
		d := <-closeTimes
		if d == 0 {
			t.Fatal("a connection failed to open")
		}
		durations = append(durations, d)
	}

	// All connections should close within the valid range: 2s to 4s (+tolerance)
	for i, d := range durations {
		if d < 1500*time.Millisecond || d > 6*time.Second {
			t.Fatalf("connection %d closed at unexpected time: %v (expected 2-4s)", i, d)
		}
	}

	// Check that not all connections closed at exactly the same time (jitter works)
	allSame := true
	for i := 1; i < len(durations); i++ {
		diff := durations[i] - durations[0]
		if diff < 0 {
			diff = -diff
		}
		if diff > 200*time.Millisecond {
			allSame = false
			break
		}
	}

	if allSame {
		t.Log("WARNING: all connections closed at nearly the same time, jitter may not be effective")
		t.Logf("durations: %v", durations)
	} else {
		t.Logf("jitter confirmed: durations spread across %v", durations)
	}
}

func TestSSEMaxLifetime_MessagesDeliveredBeforeTimeout(t *testing.T) {
	srv := startLocalBridge(t, 3, 0) // 3s lifetime, no jitter
	defer srv.Close()

	bridgeURL := srv.URL + "/bridge"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	senderSession := randomSessionID(t)
	sender, err := OpenBridge(ctx, OpenOpts{BridgeURL: bridgeURL, SessionID: senderSession})
	if err != nil {
		t.Fatalf("open sender: %v", err)
	}
	defer func() { _ = sender.Close() }()

	receiverSession := randomSessionID(t)
	receiver, err := OpenBridge(ctx, OpenOpts{BridgeURL: bridgeURL, SessionID: receiverSession})
	if err != nil {
		t.Fatalf("open receiver: %v", err)
	}
	defer func() { _ = receiver.Close() }()

	if err := receiver.WaitReady(ctx); err != nil {
		t.Fatalf("receiver not ready: %v", err)
	}

	// Send a message well before the lifetime expires
	if err := sender.Send(ctx, []byte("before-timeout"), senderSession, receiverSession, nil); err != nil {
		t.Fatalf("send: %v", err)
	}

	msgCtx, msgCancel := context.WithTimeout(ctx, 2*time.Second)
	defer msgCancel()

	ev, err := receiver.WaitMessage(msgCtx)
	if err != nil {
		t.Fatalf("expected to receive message before lifetime expires: %v", err)
	}

	if !strings.Contains(ev.Data, senderSession) {
		t.Fatalf("unexpected message data: %s", ev.Data)
	}
	t.Logf("message delivered successfully before lifetime timeout")
}

func TestSSEMaxLifetime_ClosedDuringContinuousMessages(t *testing.T) {
	srv := startLocalBridge(t, 2, 0) // 2s lifetime, no jitter
	defer srv.Close()

	bridgeURL := srv.URL + "/bridge"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	senderSession := randomSessionID(t)
	sender, err := OpenBridge(ctx, OpenOpts{BridgeURL: bridgeURL, SessionID: senderSession})
	if err != nil {
		t.Fatalf("open sender: %v", err)
	}
	defer func() { _ = sender.Close() }()

	receiverSession := randomSessionID(t)
	receiver, err := OpenBridge(ctx, OpenOpts{BridgeURL: bridgeURL, SessionID: receiverSession})
	if err != nil {
		t.Fatalf("open receiver: %v", err)
	}
	defer func() { _ = receiver.Close() }()

	if err := receiver.WaitReady(ctx); err != nil {
		t.Fatalf("receiver not ready: %v", err)
	}

	// Send messages continuously in a goroutine
	stopSending := make(chan struct{})
	go func() {
		i := 0
		for {
			select {
			case <-stopSending:
				return
			default:
				_ = sender.Send(ctx, []byte(fmt.Sprintf("msg-%d", i)), senderSession, receiverSession, nil)
				i++
				time.Sleep(50 * time.Millisecond) // ~20 msgs/sec
			}
		}
	}()

	// Read messages until the connection closes
	start := time.Now()
	received := 0
	for {
		msgCtx, msgCancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := receiver.WaitMessage(msgCtx)
		msgCancel()
		if err != nil {
			break
		}
		received++
	}
	elapsed := time.Since(start)
	close(stopSending)

	if elapsed < 1500*time.Millisecond {
		t.Fatalf("connection closed too early: %v (received %d msgs)", elapsed, received)
	}
	if elapsed > 4*time.Second {
		t.Fatalf("connection stayed open too long: %v (received %d msgs)", elapsed, received)
	}

	t.Logf("connection force-closed after %v despite receiving %d messages", elapsed, received)
}

func TestSSEMaxLifetime_MultipleMessagesThenClose(t *testing.T) {
	srv := startLocalBridge(t, 3, 0) // 3s lifetime, no jitter
	defer srv.Close()

	bridgeURL := srv.URL + "/bridge"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	senderSession := randomSessionID(t)
	sender, err := OpenBridge(ctx, OpenOpts{BridgeURL: bridgeURL, SessionID: senderSession})
	if err != nil {
		t.Fatalf("open sender: %v", err)
	}
	defer func() { _ = sender.Close() }()

	receiverSession := randomSessionID(t)
	receiver, err := OpenBridge(ctx, OpenOpts{BridgeURL: bridgeURL, SessionID: receiverSession})
	if err != nil {
		t.Fatalf("open receiver: %v", err)
	}
	defer func() { _ = receiver.Close() }()

	if err := receiver.WaitReady(ctx); err != nil {
		t.Fatalf("receiver not ready: %v", err)
	}

	// Send 3 messages quickly
	for i := 0; i < 3; i++ {
		msg := fmt.Sprintf("msg-%d", i)
		if err := sender.Send(ctx, []byte(msg), senderSession, receiverSession, nil); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}

	// Receive all 3
	for i := 0; i < 3; i++ {
		msgCtx, msgCancel := context.WithTimeout(ctx, 2*time.Second)
		_, err := receiver.WaitMessage(msgCtx)
		msgCancel()
		if err != nil {
			t.Fatalf("failed to receive message %d: %v", i, err)
		}
	}

	// Now wait for the connection to be closed by the server
	start := time.Now()
	_, err = receiver.WaitMessage(ctx)
	elapsed := time.Since(start)

	// Should close within the remaining lifetime (~3s from start, minus time already spent)
	if elapsed > 5*time.Second {
		t.Fatalf("connection stayed open too long after messages: %v", elapsed)
	}

	t.Logf("connection closed %v after last message (lifetime enforced)", elapsed)
}
