package bridge_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// ===== shared config (same BRIDGE_URL as before) =====

var (
	BRIDGE_URL_Provider = func() string {
		if v := os.Getenv("BRIDGE_URL"); v != "" {
			return strings.TrimRight(v, "/")
		}
		return "https://walletbot.me/tonconnect-bridge/bridge"
	}()
)

// ===== payload / event types =====

type JSONRPC struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
	ID     string   `json:"id"`
}

// What wallet/app listeners receive:
type BridgeAppEvent struct {
	Method      string   `json:"method"`
	Params      []string `json:"params"`
	ID          string   `json:"id"`
	From        string   `json:"from,omitempty"`
	LastEventID string   `json:"lastEventId,omitempty"`
}

// ===== BridgeProvider =====

type clientPair struct {
	SessionID string // our local session (SSE listen)
	ClientID  string // remote peer client_id (send target)
}

type BridgeProvider struct {
	baseURL string

	mu       sync.RWMutex
	clients  map[string]string // local session -> remote client
	gateways map[string]*BridgeGateway
	listener func(BridgeAppEvent)

	closed bool

	// for internal cancellation of readers
	ctx    context.Context
	cancel context.CancelFunc

	httpc *http.Client
}

type ProviderOpenOpts struct {
	BridgeURL string
	Clients   []clientPair
	Listener  func(BridgeAppEvent)
}

type RestoreOpts struct {
	LastEventID string // resume SSE for all sessions using this ID (matches your JS)
}

func OpenProvider(ctx context.Context, opts ProviderOpenOpts) (*BridgeProvider, error) {
	if opts.BridgeURL == "" || len(opts.Clients) == 0 {
		return nil, errors.New("BridgeURL and at least one client required")
	}
	pctx, cancel := context.WithCancel(ctx)
	p := &BridgeProvider{
		baseURL:  opts.BridgeURL,
		clients:  make(map[string]string),
		gateways: make(map[string]*BridgeGateway),
		listener: opts.Listener,
		ctx:      pctx,
		cancel:   cancel,
		httpc:    &http.Client{Timeout: 30 * time.Second},
	}

	for _, c := range opts.Clients {
		p.clients[c.SessionID] = c.ClientID
		if err := p.openSession(c.SessionID, ""); err != nil {
			if err = p.Close(); err != nil {
				log.Println("error during p.Close():", err)
			}
			return nil, err
		}
	}
	return p, nil
}

func (p *BridgeProvider) openSession(sessionID, lastEventID string) error {
	gw, err := OpenBridge(p.ctx, OpenOpts{
		BridgeURL:   p.baseURL,
		SessionID:   sessionID,
		LastEventID: lastEventID,
	})
	if err != nil {
		return fmt.Errorf("open sse for %s: %w", sessionID, err)
	}
	p.gateways[sessionID] = gw

	// reader goroutine
	go func(local string, g *BridgeGateway) {
		for {
			ev, err := g.WaitMessage(p.ctx)
			if err != nil {
				return
			}
			// decode bridge envelope: {"from": "<hex>", "message":"<b64>"}
			var env struct {
				From    string `json:"from"`
				Message string `json:"message"`
			}
			if jerr := json.Unmarshal([]byte(ev.Data), &env); jerr != nil {
				continue
			}
			raw, derr := base64.StdEncoding.DecodeString(env.Message)
			if derr != nil {
				continue
			}
			var msg BridgeAppEvent
			if jerr := json.Unmarshal(raw, &msg); jerr != nil {
				continue
			}
			msg.From = env.From
			msg.LastEventID = ev.ID

			p.mu.RLock()
			cb := p.listener
			p.mu.RUnlock()
			if cb != nil {
				cb(msg)
			}
		}
	}(sessionID, gw)

	return nil
}

func (p *BridgeProvider) Listen(fn func(BridgeAppEvent)) {
	p.mu.Lock()
	p.listener = fn
	p.mu.Unlock()
}

func (p *BridgeProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if p.cancel != nil {
		p.cancel()
	}
	for _, g := range p.gateways {
		_ = g.Close()
	}
	p.gateways = map[string]*BridgeGateway{}
	return nil
}

// RestoreConnection replaces/extends the session set.
// - If provider was previously closed, it reopens streams.
// - If LastEventID provided, it is applied to every (re)opened session.
func (p *BridgeProvider) RestoreConnection(ctx context.Context, clients []clientPair, ro RestoreOpts) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Re-enable if previously closed
	if p.closed {
		pctx, cancel := context.WithCancel(ctx)
		p.ctx = pctx
		p.cancel = cancel
		p.closed = false
	}

	desired := make(map[string]string, len(clients))
	for _, c := range clients {
		desired[c.SessionID] = c.ClientID
	}

	// Close streams that are no longer needed
	for sess, gw := range p.gateways {
		if _, ok := desired[sess]; !ok {
			_ = gw.Close()
			delete(p.gateways, sess)
		}
	}

	// Open new streams (or keep existing)
	for sess, peer := range desired {
		p.clients[sess] = peer
		if _, ok := p.gateways[sess]; !ok {
			if err := p.openSession(sess, ro.LastEventID); err != nil {
				return err
			}
		}
	}
	return nil
}

// Send posts a JSONRPC payload to the bridge (body = base64(JSON)).
type SendOpts struct {
	Attempts int
	TTL      *int // optional
}

func (p *BridgeProvider) Send(ctx context.Context, msg JSONRPC, fromSession, toClient string) error {
	b, _ := json.Marshal(msg)
	b64 := base64.StdEncoding.EncodeToString(b)

	u, _ := url.Parse(p.baseURL + "/message")
	q := u.Query()
	q.Set("client_id", fromSession)
	q.Set("to", toClient)
	q.Set("topic", "test")
	q.Set("ttl", "300")
	u.RawQuery = q.Encode()

	req, _ := http.NewRequestWithContext(ctx, "POST", u.String(), strings.NewReader(b64))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := p.httpc.Do(req)
	defer func() {
		if resp == nil || resp.Body == nil {
			return
		}
		if err = resp.Body.Close(); err != nil {
			log.Println("error during resp.Body.Close():", err)
		}
	}()

	if err != nil {
		return fmt.Errorf("send request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("send bad status: %s", resp.Status)
	}
	return nil
}

// ===== Tests (ported 1:1 from your Jest) =====

func TestBridgeProvider_SendAndRetrieve(t *testing.T) {
	// sessions
	appSession := randomSessionID(t)
	walletSession := randomSessionID(t)

	// app provider
	app, err := OpenProvider(context.Background(), ProviderOpenOpts{
		BridgeURL: BRIDGE_URL_Provider,
		Clients:   []clientPair{{SessionID: appSession, ClientID: walletSession}},
		Listener:  func(BridgeAppEvent) {},
	})
	if err != nil {
		t.Fatalf("open app: %v", err)
	}
	defer func() {
		if err = app.Close(); err != nil {
			log.Println("error during app.Close():", err)
		}
	}()

	// wallet provider with a resolver-like listener
	resCh := make(chan BridgeAppEvent, 1)
	wallet, err := OpenProvider(context.Background(), ProviderOpenOpts{
		BridgeURL: BRIDGE_URL_Provider,
		Clients:   []clientPair{{SessionID: walletSession, ClientID: appSession}},
		Listener:  func(e BridgeAppEvent) { resCh <- e },
	})
	if err != nil {
		t.Fatalf("open wallet: %v", err)
	}
	defer func() {
		if err = wallet.Close(); err != nil {
			log.Println("error during wallet.Close():", err)
		}
	}()

	// Give the server-side subscription time to be established
	time.Sleep(100 * time.Millisecond)

	// send
	if err := app.Send(context.Background(),
		JSONRPC{Method: "sendTransaction", Params: []string{""}, ID: "1"},
		appSession, walletSession,
	); err != nil {
		t.Fatalf("send: %v", err)
	}

	// expect
	select {
	case got := <-resCh:
		if got.Method != "sendTransaction" || got.ID != "1" || len(got.Params) != 1 || got.Params[0] != "" {
			t.Fatalf("payload mismatch: %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for wallet event")
	}
}

func TestBridgeProvider_ReconnectToAnotherWalletAndReceive(t *testing.T) {
	appSession := randomSessionID(t)
	walletSession := randomSessionID(t)

	app, err := OpenProvider(context.Background(), ProviderOpenOpts{
		BridgeURL: BRIDGE_URL_Provider,
		Clients:   []clientPair{{SessionID: appSession, ClientID: walletSession}},
		Listener:  func(BridgeAppEvent) {},
	})
	if err != nil {
		t.Fatalf("open app: %v", err)
	}
	defer func() {
		if err = app.Close(); err != nil {
			log.Println("error during app.Close():", err)
		}
	}()

	// wallet #1
	resCh1 := make(chan BridgeAppEvent, 1)
	wallet1, err := OpenProvider(context.Background(), ProviderOpenOpts{
		BridgeURL: BRIDGE_URL_Provider,
		Clients:   []clientPair{{SessionID: walletSession, ClientID: appSession}},
		Listener:  func(e BridgeAppEvent) { resCh1 <- e },
	})
	if err != nil {
		t.Fatalf("open wallet1: %v", err)
	}
	defer func() {
		if err = wallet1.Close(); err != nil {
			log.Println("error during wallet1.Close():", err)
		}
	}()

	// send to wallet1
	if err := app.Send(context.Background(),
		JSONRPC{Method: "sendTransaction", Params: []string{"abc"}, ID: "1"},
		appSession, walletSession,
	); err != nil {
		t.Fatalf("send #1: %v", err)
	}

	select {
	case got := <-resCh1:
		if got.Method != "sendTransaction" || got.ID != "1" || got.Params[0] != "abc" {
			t.Fatalf("payload mismatch: %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting event #1")
	}

	// wallet #2 sessions
	app2Session := randomSessionID(t)
	wallet2Session := randomSessionID(t)

	// wallet #2 provider/listener
	resCh2 := make(chan BridgeAppEvent, 1)
	wallet2, err := OpenProvider(context.Background(), ProviderOpenOpts{
		BridgeURL: BRIDGE_URL_Provider,
		Clients:   []clientPair{{SessionID: wallet2Session, ClientID: app2Session}},
		Listener:  func(e BridgeAppEvent) { resCh2 <- e },
	})
	if err != nil {
		t.Fatalf("open wallet2: %v", err)
	}
	defer func() {
		if err = wallet2.Close(); err != nil {
			log.Println("error during wallet2.Close():", err)
		}
	}()

	// app adds a new connection (app2 <-> wallet2) while keeping the old one
	if err := app.RestoreConnection(context.Background(), []clientPair{
		{SessionID: appSession, ClientID: walletSession},
		{SessionID: app2Session, ClientID: wallet2Session},
	}, RestoreOpts{}); err != nil {
		t.Fatalf("app restore: %v", err)
	}

	// send to wallet2 via app2Session
	if err := app.Send(context.Background(),
		JSONRPC{Method: "disconnect", Params: []string{}, ID: "2"},
		app2Session, wallet2Session,
	); err != nil {
		t.Fatalf("send #2: %v", err)
	}

	select {
	case got := <-resCh2:
		if got.Method != "disconnect" || got.ID != "2" || len(got.Params) != 0 {
			t.Fatalf("payload mismatch #2: %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting event #2")
	}
}

func TestBridgeProvider_ReceiveAfterReconnectWithLastEventID(t *testing.T) {
	appSession := randomSessionID(t)
	walletSession := randomSessionID(t)

	app, err := OpenProvider(context.Background(), ProviderOpenOpts{
		BridgeURL: BRIDGE_URL_Provider,
		Clients:   []clientPair{{SessionID: appSession, ClientID: walletSession}},
		Listener:  func(BridgeAppEvent) {},
	})
	if err != nil {
		t.Fatalf("open app: %v", err)
	}
	defer func() {
		if err = app.Close(); err != nil {
			log.Println("error during app.Close():", err)
		}
	}()

	// wallet online -> catch first message
	resCh := make(chan BridgeAppEvent, 1)
	wallet, err := OpenProvider(context.Background(), ProviderOpenOpts{
		BridgeURL: BRIDGE_URL_Provider,
		Clients:   []clientPair{{SessionID: walletSession, ClientID: appSession}},
		Listener:  func(e BridgeAppEvent) { resCh <- e },
	})
	if err != nil {
		t.Fatalf("open wallet: %v", err)
	}

	// send first
	if err := app.Send(context.Background(),
		JSONRPC{Method: "sendTransaction", Params: []string{"abc"}, ID: "1"},
		appSession, walletSession,
	); err != nil {
		t.Fatalf("send #1: %v", err)
	}
	var first BridgeAppEvent
	select {
	case first = <-resCh:
		if first.Method != "sendTransaction" || first.ID != "1" || first.Params[0] != "abc" {
			t.Fatalf("payload mismatch #1: %+v", first)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting event #1")
	}

	_ = wallet.Close()

	if err := wallet.RestoreConnection(context.Background(),
		[]clientPair{{SessionID: walletSession, ClientID: appSession}},
		RestoreOpts{LastEventID: first.LastEventID},
	); err != nil {
		t.Fatalf("wallet restore: %v", err)
	}

	// (optional) reattach or change listener; the existing one is still kept,
	// but it’s fine to set it again:
	wallet.Listen(func(e BridgeAppEvent) { resCh <- e })

	// send second
	if err := app.Send(context.Background(),
		JSONRPC{Method: "disconnect", Params: []string{}, ID: "2"},
		appSession, walletSession,
	); err != nil {
		t.Fatalf("send #2: %v", err)
	}

	select {
	case got := <-resCh:
		if got.Method != "disconnect" || got.ID != "2" {
			t.Fatalf("payload mismatch #2: %+v", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting event #2")
	}
	_ = wallet.Close()
}
