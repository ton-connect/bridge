package bridge_test

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ===== Test config =====

var (
	BRIDGE_URL = func() string {
		if v := os.Getenv("BRIDGE_URL"); v != "" {
			return strings.TrimRight(v, "/")
		}
		return "https://walletbot.me/tonconnect-bridge/bridge"
	}()
	testHTTPTimeout  = 30 * time.Second
	testSSETimeout   = 5 * time.Second
	testSilentWait   = 1500 * time.Millisecond
	testNoMsgTimeout = 1 * time.Second
)

// ===== Minimal SSE client + BridgeGateway =====

type SSEEvent struct {
	ID   string
	Data string
}

type BridgeGateway struct {
	baseURL   string
	sessionID string

	client *http.Client
	resp   *http.Response
	cancel context.CancelFunc
	msgs   chan SSEEvent
	errs   chan error
	ready  bool
	mu     sync.RWMutex
	closed bool
}

type OpenOpts struct {
	BridgeURL   string
	SessionID   string
	LastEventID string
	OriginURL   string
}

func OpenBridge(ctx context.Context, opts OpenOpts) (*BridgeGateway, error) {
	if opts.SessionID == "" || opts.BridgeURL == "" {
		return nil, errors.New("BridgeURL and SessionID are required")
	}
	cctx, cancel := context.WithCancel(ctx)

	gw := &BridgeGateway{
		baseURL:   strings.TrimRight(opts.BridgeURL, "/"),
		sessionID: opts.SessionID,
		client: &http.Client{
			Timeout: testHTTPTimeout,
		},
		cancel: cancel,
		msgs:   make(chan SSEEvent, 16),
		errs:   make(chan error, 1),
	}

	// Build SSE URL: GET /events?client_id=<sessionID>
	u, _ := url.Parse(gw.baseURL + "/events")
	q := u.Query()
	q.Add("client_id", opts.SessionID)
	u.RawQuery = q.Encode()

	req, _ := http.NewRequestWithContext(cctx, http.MethodGet, u.String(), nil)
	req.Header.Set("Accept", "text/event-stream")
	if opts.OriginURL != "" {
		req.Header.Set("Origin", opts.OriginURL)
	}
	if opts.LastEventID != "" {
		// Standard SSE resume header
		req.Header.Set("Last-Event-ID", opts.LastEventID)
	}

	resp, err := gw.client.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open SSE failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		_ = resp.Body.Close()
		return nil, fmt.Errorf("SSE status %d", resp.StatusCode)
	}
	gw.resp = resp
	gw.setReady(true)

	// Reader goroutine
	go func() {
		defer close(gw.msgs)
		defer close(gw.errs)
		// explicitly ignore Close() error to satisfy errcheck
		defer func() {
			if err = gw.Close(); err != nil {
				log.Println("error during gw.Close():", err)
			}
		}()

		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

		var curID string
		var dataBuilder strings.Builder

		flushEvent := func() {
			if dataBuilder.Len() == 0 && curID == "" {
				return
			}
			ev := SSEEvent{ID: curID, Data: dataBuilder.String()}
			select {
			case gw.msgs <- ev:
			case <-cctx.Done():
			}
			curID = ""
			dataBuilder.Reset()
		}

		for sc.Scan() {
			line := sc.Text()
			if line == "" {
				flushEvent()
				continue
			}
			if strings.HasPrefix(line, ":") {
				// comment/heartbeat
				continue
			}
			if strings.HasPrefix(line, "id:") {
				curID = strings.TrimSpace(line[3:])
				continue
			}
			if strings.HasPrefix(line, "data:") {
				if dataBuilder.Len() > 0 {
					dataBuilder.WriteByte('\n')
				}
				dataBuilder.WriteString(strings.TrimSpace(line[5:]))
			}
			// ignore event:, retry:, etc.
		}
		if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
			select {
			case gw.errs <- err:
			default:
			}
		}
	}()

	return gw, nil
}

func (g *BridgeGateway) setReady(v bool) {
	g.mu.Lock()
	g.ready = v
	g.mu.Unlock()
}

func (g *BridgeGateway) IsReady() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.ready
}

// WaitReady blocks until the gateway is ready for server-side subscription
// This replaces the need for arbitrary sleeps after OpenBridge
func (g *BridgeGateway) WaitReady(ctx context.Context) error {
	if g.IsReady() {
		// Give the server-side subscription time to be established
		// This is a known requirement of the bridge architecture
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if g.IsReady() {
				// Give the server-side subscription time to be established
				time.Sleep(100 * time.Millisecond)
				return nil
			}
		}
	}
}

func (g *BridgeGateway) IsClosed() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.closed
}

func (g *BridgeGateway) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return nil
	}
	g.closed = true
	if g.cancel != nil {
		g.cancel()
	}
	if g.resp != nil && g.resp.Body != nil {
		_ = g.resp.Body.Close()
	}
	return nil
}

func (g *BridgeGateway) Send(ctx context.Context, payload []byte, fromSession, toSession string, ttlSeconds *int) error {
	// POST /message?client_id=<from>&to=<to>&ttl=...&topic=sendTransaction
	// body: base64
	// (topic is not semantically checked by bridge for these tests)
	if ttlSeconds == nil {
		ttlSeconds = new(int)
		*ttlSeconds = 300 // default 5min
	}

	u, _ := url.Parse(g.baseURL + "/message")
	q := u.Query()
	q.Set("client_id", fromSession)
	q.Set("to", toSession)
	q.Set("ttl", fmt.Sprintf("%d", *ttlSeconds))
	q.Set("topic", "test")
	u.RawQuery = q.Encode()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u.String(),
		strings.NewReader(base64.StdEncoding.EncodeToString(payload)))
	req.Header.Set("Content-Type", "text/plain")

	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		if err = resp.Body.Close(); err != nil {
			log.Println("error during resp.Body.Close():", err)
		}
	}()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("send status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (g *BridgeGateway) WaitMessage(ctx context.Context) (SSEEvent, error) {
	select {
	case ev, ok := <-g.msgs:
		if !ok {
			return SSEEvent{}, io.EOF
		}
		return ev, nil
	case err := <-g.errs:
		if err != nil {
			return SSEEvent{}, err
		}
		return SSEEvent{}, io.EOF
	case <-ctx.Done():
		return SSEEvent{}, ctx.Err()
	}
}

// ===== Helpers =====

func randomSessionID(t *testing.T) string {
	t.Helper()
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("rand: %v", err)
	}
	// Bridge expects hex client_id; 32 bytes -> 64 hex chars.
	return hex.EncodeToString(b[:])
}

type bridgeMessage struct {
	From    string `json:"from"`
	Message string `json:"message"`
}

// getBridgeLastEventID spins up a throwaway receiver session to capture the last
// event id after sender pings an empty payload.
func getBridgeLastEventID(t *testing.T, sender *BridgeGateway, senderSession string) string {
	t.Helper()

	session := randomSessionID(t)
	ctx, cancel := context.WithTimeout(context.Background(), testSSETimeout)
	defer cancel()

	receiver, err := OpenBridge(ctx, OpenOpts{
		BridgeURL: BRIDGE_URL, SessionID: session,
	})
	if err != nil {
		t.Fatalf("open receiver: %v", err)
	}
	defer func() {
		if err = receiver.Close(); err != nil {
			log.Println("error during receiver.Close():", err)
		}
	}()

	if !receiver.IsReady() {
		t.Fatal("receiver not ready")
	}

	if err := sender.Send(ctx, make([]byte, 0), senderSession, session, nil); err != nil {
		t.Fatalf("send empty ping: %v", err)
	}

	ev, err := receiver.WaitMessage(ctx)
	if err != nil {
		t.Fatalf("wait sse: %v", err)
	}
	return ev.ID
}

// ===== Tests (ported 1:1 from your Jest suite) =====

func TestBridge_ConnectAndClose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testSSETimeout)
	defer cancel()

	session := randomSessionID(t)
	gw, err := OpenBridge(ctx, OpenOpts{
		BridgeURL: BRIDGE_URL, SessionID: session,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if !gw.IsReady() {
		t.Fatal("gateway not ready")
	}
	_ = gw.Close()
	if !gw.IsClosed() {
		t.Fatal("gateway not closed")
	}
}

func TestBridge_ReceiveMessageOverOpenConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testSSETimeout)
	defer cancel()

	senderSession := randomSessionID(t)
	sender, err := OpenBridge(ctx, OpenOpts{BridgeURL: BRIDGE_URL, SessionID: senderSession})
	if err != nil {
		t.Fatalf("open sender: %v", err)
	}
	defer func() {
		if err = sender.Close(); err != nil {
			log.Println("error during sender.Close():", err)
		}
	}()
	if !sender.IsReady() {
		t.Fatal("sender not ready")
	}

	receiverSession := randomSessionID(t)
	receiver, err := OpenBridge(ctx, OpenOpts{BridgeURL: BRIDGE_URL, SessionID: receiverSession})
	if err != nil {
		t.Fatalf("open receiver: %v", err)
	}
	defer func() { _ = receiver.Close() }()
	if err := receiver.WaitReady(ctx); err != nil {
		t.Fatalf("receiver not ready: %v", err)
	}

	if err := sender.Send(ctx, []byte("ping"), senderSession, receiverSession, nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	ev, err := receiver.WaitMessage(ctx)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}

	var bm bridgeMessage
	if err := json.Unmarshal([]byte(ev.Data), &bm); err != nil {
		t.Fatalf("decode: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(bm.Message)
	if err != nil {
		t.Fatalf("b64: %v", err)
	}
	if string(raw) != "ping" {
		t.Fatalf("expected 'ping', got %q", string(raw))
	}
	if bm.From != senderSession {
		t.Fatalf("expected from=%s, got %s", senderSession, bm.From)
	}
}

func TestBridge_NoMessageAfterReconnectWithUpdatedLastEventID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*testSSETimeout)
	defer cancel()

	senderSession := randomSessionID(t)
	sender, err := OpenBridge(ctx, OpenOpts{BridgeURL: BRIDGE_URL, SessionID: senderSession})
	if err != nil {
		t.Fatalf("open sender: %v", err)
	}
	defer func() {
		if err = sender.Close(); err != nil {
			log.Println("error during sender.Close():", err)
		}
	}()

	session := randomSessionID(t)

	// First connect and receive a message
	r1, err := OpenBridge(ctx, OpenOpts{BridgeURL: BRIDGE_URL, SessionID: session})
	if err != nil {
		t.Fatalf("open receiver1: %v", err)
	}
	if !r1.IsReady() {
		t.Fatal("receiver1 not ready")
	}
	if err := sender.Send(ctx, []byte("Hello!"), senderSession, session, nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	ev1, err := r1.WaitMessage(ctx)
	if err != nil {
		t.Fatalf("wait1: %v", err)
	}
	_ = r1.Close()

	// Reconnect with updated lastEventId -> expect no new messages in 1s
	r2, err := OpenBridge(ctx, OpenOpts{
		BridgeURL: BRIDGE_URL, SessionID: session, LastEventID: ev1.ID,
	})
	if err != nil {
		t.Fatalf("open receiver2: %v", err)
	}
	defer func() {
		if err = r2.Close(); err != nil {
			log.Println("error during r2.Close():", err)
		}
	}()

	waitCtx, cancelWait := context.WithTimeout(ctx, testNoMsgTimeout)
	defer cancelWait()
	_, err = r2.WaitMessage(waitCtx)
	if err == nil {
		t.Fatalf("expected no message after reconnect with exact Last-Event-ID")
	}
}

func TestBridge_ReceiveMessageAgainAfterReconnectWithValidLastEventID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*testSSETimeout)
	defer cancel()

	senderSession := randomSessionID(t)
	sender, err := OpenBridge(ctx, OpenOpts{BridgeURL: BRIDGE_URL, SessionID: senderSession})
	if err != nil {
		t.Fatalf("open sender: %v", err)
	}
	defer func() {
		if err = sender.Close(); err != nil {
			log.Println("error during sender.Close():", err)
		}
	}()

	session := randomSessionID(t)

	r1, err := OpenBridge(ctx, OpenOpts{BridgeURL: BRIDGE_URL, SessionID: session})
	if err != nil {
		t.Fatalf("open receiver1: %v", err)
	}
	if err := r1.WaitReady(ctx); err != nil {
		t.Fatalf("receiver1 not ready: %v", err)
	}

	if err := sender.Send(ctx, []byte("ping"), senderSession, session, nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	ev1, err := r1.WaitMessage(ctx)
	if err != nil {
		t.Fatalf("wait1: %v", err)
	}
	_ = r1.Close()

	// Reconnect with lastEventId-1 -> should re-deliver ev1
	// (Event IDs are numeric in practice; if not, the server may still accept "previous" semantics)
	// We’ll just pass ev1.ID-1 when it’s a number; otherwise, pass empty to skip this branch gracefully.
	last := ev1.ID
	// Best effort: parse as uint
	var lastMinusOne string
	if n, perr := parseUint(last); perr == nil && n > 0 {
		lastMinusOne = fmt.Sprintf("%d", n-1)
	}

	r2, err := OpenBridge(ctx, OpenOpts{
		BridgeURL: BRIDGE_URL, SessionID: session, LastEventID: lastMinusOne,
	})
	if err != nil {
		t.Fatalf("open receiver2: %v", err)
	}
	defer func() {
		if err = r2.Close(); err != nil {
			log.Println("error during r2.Close():", err)
		}
	}()
	if !r2.IsReady() {
		t.Fatal("receiver2 not ready")
	}

	ev2, err := r2.WaitMessage(ctx)
	if err != nil {
		t.Fatalf("wait2: %v", err)
	}
	if ev2.ID != ev1.ID {
		t.Fatalf("expected same lastEventId, got %s != %s", ev2.ID, ev1.ID)
	}
	var bm bridgeMessage
	_ = json.Unmarshal([]byte(ev2.Data), &bm)
	raw, _ := base64.StdEncoding.DecodeString(bm.Message)
	if string(raw) != "ping" || bm.From != senderSession {
		t.Fatalf("payload mismatch: from=%s, msg=%s", bm.From, string(raw))
	}
}

func TestBridge_NoDeliveryAfterReconnectWithFutureLastEventID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*testSSETimeout)
	defer cancel()

	senderSession := randomSessionID(t)
	sender, err := OpenBridge(ctx, OpenOpts{BridgeURL: BRIDGE_URL, SessionID: senderSession})
	if err != nil {
		t.Fatalf("open sender: %v", err)
	}
	defer func() {
		if err = sender.Close(); err != nil {
			log.Println("error during sender.Close():", err)
		}
	}()

	session := randomSessionID(t)

	// Send while disconnected
	if err := sender.Send(ctx, []byte("Offline message"), senderSession, session, nil); err != nil {
		t.Fatalf("send offline: %v", err)
	}

	// Obtain a "current" lastEventId from bridge
	lastID := getBridgeLastEventID(t, sender, senderSession)

	// Reconnect with lastEventId far in the future -> expect no message
	futureID := bumpID(lastID, 1_000_000_000)
	r, err := OpenBridge(ctx, OpenOpts{
		BridgeURL: BRIDGE_URL, SessionID: session, LastEventID: futureID,
	})
	if err != nil {
		t.Fatalf("open receiver: %v", err)
	}
	defer func() {
		if err = r.Close(); err != nil {
			log.Println("error during r.Close():", err)
		}
	}()

	waitCtx, cancelWait := context.WithTimeout(ctx, testNoMsgTimeout)
	defer cancelWait()
	if _, err := r.WaitMessage(waitCtx); err == nil {
		t.Fatalf("expected no message when reconnecting with future Last-Event-ID")
	}
}

func TestBridge_DeliveryAfterReconnectWithoutLastEventID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testSSETimeout)
	defer cancel()

	senderSession := randomSessionID(t)
	sender, err := OpenBridge(ctx, OpenOpts{BridgeURL: BRIDGE_URL, SessionID: senderSession})
	if err != nil {
		t.Fatalf("open sender: %v", err)
	}
	defer func() {
		if err = sender.Close(); err != nil {
			log.Println("error during sender.Close():", err)
		}
	}()

	session := randomSessionID(t)

	// Send while disconnected
	if err := sender.Send(ctx, []byte("Delivered later"), senderSession, session, nil); err != nil {
		t.Fatalf("send offline: %v", err)
	}

	// Now connect without Last-Event-ID -> should receive it
	r, err := OpenBridge(ctx, OpenOpts{BridgeURL: BRIDGE_URL, SessionID: session})
	if err != nil {
		t.Fatalf("open receiver: %v", err)
	}
	defer func() {
		if err = r.Close(); err != nil {
			log.Println("error during r.Close():", err)
		}
	}()

	ev, err := r.WaitMessage(ctx)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	var bm bridgeMessage
	if err := json.Unmarshal([]byte(ev.Data), &bm); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if bm.From != senderSession {
		t.Fatalf("from mismatch: %s", bm.From)
	}
	raw, _ := base64.StdEncoding.DecodeString(bm.Message)
	if string(raw) != "Delivered later" {
		t.Fatalf("payload mismatch, got %q", string(raw))
	}
}

func TestBridge_TTLExpires_NoDelivery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*testSSETimeout)
	defer cancel()

	senderSession := randomSessionID(t)
	sender, err := OpenBridge(ctx, OpenOpts{BridgeURL: BRIDGE_URL, SessionID: senderSession})
	if err != nil {
		t.Fatalf("open sender: %v", err)
	}
	defer func() {
		if err = sender.Close(); err != nil {
			log.Println("error during sender.Close():", err)
		}
	}()

	session := randomSessionID(t)
	ttl := 1
	if err := sender.Send(ctx, []byte("Expiring message"), senderSession, session, &ttl); err != nil {
		t.Fatalf("send with ttl: %v", err)
	}

	time.Sleep(testSilentWait) // wait > ttl

	r, err := OpenBridge(ctx, OpenOpts{BridgeURL: BRIDGE_URL, SessionID: session})
	if err != nil {
		t.Fatalf("open receiver: %v", err)
	}
	defer func() {
		if err = r.Close(); err != nil {
			log.Println("error during r.Close():", err)
		}
	}()

	waitCtx, cancelWait := context.WithTimeout(ctx, testNoMsgTimeout)
	defer cancelWait()
	if _, err := r.WaitMessage(waitCtx); err == nil {
		t.Fatalf("expected no message after ttl expired")
	}
}

func TestBridge_MultipleMessagesInOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testSSETimeout)
	defer cancel()

	senderSession := randomSessionID(t)
	sender, err := OpenBridge(ctx, OpenOpts{BridgeURL: BRIDGE_URL, SessionID: senderSession})
	if err != nil {
		t.Fatalf("open sender: %v", err)
	}
	defer func() {
		if err = sender.Close(); err != nil {
			log.Println("error during sender.Close():", err)
		}
	}()
	if !sender.IsReady() {
		t.Fatal("sender not ready")
	}

	receiverSession := randomSessionID(t)
	r, err := OpenBridge(ctx, OpenOpts{BridgeURL: BRIDGE_URL, SessionID: receiverSession})
	if err != nil {
		t.Fatalf("open receiver: %v", err)
	}
	defer func() {
		if err = r.Close(); err != nil {
			log.Println("error during r.Close():", err)
		}
	}()
	if err := r.WaitReady(ctx); err != nil {
		t.Fatalf("receiver not ready: %v", err)
	}

	// Send 3 messages
	for _, m := range []string{"1", "2", "3"} {
		if err := sender.Send(ctx, []byte(m), senderSession, receiverSession, nil); err != nil {
			t.Fatalf("send %s: %v", m, err)
		}
	}

	// Receive 3 in order
	got := make([]string, 0, 3)
	for len(got) < 3 {
		ev, err := r.WaitMessage(ctx)
		if err != nil {
			t.Fatalf("wait: %v", err)
		}
		var bm bridgeMessage
		if err := json.Unmarshal([]byte(ev.Data), &bm); err != nil {
			t.Fatalf("decode: %v", err)
		}
		raw, _ := base64.StdEncoding.DecodeString(bm.Message)
		got = append(got, string(raw))
		if bm.From != senderSession {
			t.Fatalf("unexpected from: %s", bm.From)
		}
	}
	for i, v := range got {
		exp := fmt.Sprintf("%d", i+1)
		if v != exp {
			t.Fatalf("order mismatch at %d: got %s want %s", i, v, exp)
		}
	}
}

// ===== small helpers =====

func parseUint(s string) (uint64, error) {
	var n uint64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func bumpID(s string, delta uint64) string {
	if n, err := parseUint(s); err == nil {
		return fmt.Sprintf("%d", n+delta)
	}
	// if non-numeric, just return the same (server will ignore resume semantics)
	return s
}
