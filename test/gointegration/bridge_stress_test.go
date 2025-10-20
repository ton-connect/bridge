package bridge_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// A tiny threadsafe buffer for received wallet messages.
type recvBuf struct {
	mu   sync.Mutex
	msgs map[string]BridgeAppEvent
}

func (b *recvBuf) add(e BridgeAppEvent) {
	b.mu.Lock()
	b.msgs[e.ID] = e
	b.mu.Unlock()
}

func (b *recvBuf) len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.msgs)
}

func (b *recvBuf) contains(method, id string, params []string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, m := range b.msgs {
		if m.ID == id && m.Method == method {
			// params here are single string in tests, but compare anyway
			if len(m.Params) == len(params) {
				eq := true
				for i := range params {
					if m.Params[i] != params[i] {
						eq = false
						break
					}
				}
				if eq {
					return true
				}
			}
		}
	}
	return false
}

func TestBridgeStress_10x100(t *testing.T) {
	const (
		CLIENT_COUNT        = 10
		MESSAGES_PER_CLIENT = 100
	)

	type pair struct {
		app           *BridgeProvider
		wallet        *BridgeProvider
		appSession    string
		walletSession string
		received      *recvBuf
	}

	clients := make([]*pair, 0, CLIENT_COUNT)

	// Create CLIENT_COUNT client pairs (app+wallet)
	for i := 0; i < CLIENT_COUNT; i++ {
		appSession := randomSessionID(t)
		walletSession := randomSessionID(t)

		// app provider (listen noop)
		app, err := OpenProvider(context.Background(), ProviderOpenOpts{
			BridgeURL: BRIDGE_URL_Provider,
			Clients:   []clientPair{{SessionID: appSession, ClientID: walletSession}},
			Listener:  func(BridgeAppEvent) {},
		})
		if err != nil {
			t.Fatalf("open app #%d: %v", i, err)
		}

		rb := &recvBuf{
			msgs: map[string]BridgeAppEvent{},
		}

		// wallet provider (records messages)
		wallet, err := OpenProvider(context.Background(), ProviderOpenOpts{
			BridgeURL: BRIDGE_URL_Provider,
			Clients:   []clientPair{{SessionID: walletSession, ClientID: appSession}},
			Listener:  func(e BridgeAppEvent) { rb.add(e) },
		})
		if err != nil {
			t.Fatalf("open wallet #%d: %v", i, err)
		}

		clients = append(clients, &pair{
			app:           app,
			wallet:        wallet,
			appSession:    appSession,
			walletSession: walletSession,
			received:      rb,
		})
	}

	t.Logf("Created %d client pairs", CLIENT_COUNT)

	// Send messages from all clients in parallel
	var wg sync.WaitGroup
	totalMsgs := CLIENT_COUNT * MESSAGES_PER_CLIENT
	wg.Add(totalMsgs)

	for ci := 0; ci < CLIENT_COUNT; ci++ {
		c := clients[ci]
		for mi := 0; mi < MESSAGES_PER_CLIENT; mi++ {
			ci, mi := ci, mi // capture
			go func() {
				defer wg.Done()
				msg := JSONRPC{
					Method: "sendTransaction",
					Params: []string{fmt.Sprintf("client-%d-message-%d", ci, mi)},
					ID:     fmt.Sprintf("%d-%d", ci, mi),
				}
				if err := c.app.Send(context.Background(), msg, c.appSession, c.walletSession); err != nil {
					// Don’t fail the whole test immediately; record it and let the final checks catch any gaps.
					t.Errorf("send fail client=%d msg=%d: %v", ci, mi, err)
				}
			}()
		}
	}

	t.Logf("Sending %d messages...", totalMsgs)
	wg.Wait()
	t.Log("All messages sent, waiting for delivery...")

	// Wait for all messages to be received (with timeout)
	maxWait := 30 * time.Second
	start := time.Now()
	for {
		var totalReceived int
		for _, c := range clients {
			totalReceived += c.received.len()
		}
		t.Logf("Received %d/%d messages", totalReceived, totalMsgs)
		if totalReceived >= totalMsgs {
			break
		}
		if time.Since(start) > maxWait {
			t.Fatalf("timeout waiting for all messages: got %d/%d", totalReceived, totalMsgs)
		}
		time.Sleep(1 * time.Second)
	}

	// Verify all messages were received correctly per client
	for ci := 0; ci < CLIENT_COUNT; ci++ {
		c := clients[ci]
		if got := c.received.len(); got != MESSAGES_PER_CLIENT {
			t.Fatalf("client %d received %d/%d", ci, got, MESSAGES_PER_CLIENT)
		}
		for mi := 0; mi < MESSAGES_PER_CLIENT; mi++ {
			expParams := []string{fmt.Sprintf("client-%d-message-%d", ci, mi)}
			expID := fmt.Sprintf("%d-%d", ci, mi)
			if !c.received.contains("sendTransaction", expID, expParams) {
				t.Fatalf("client %d missing message id=%s params=%v", ci, expID, expParams)
			}
		}
	}

	// Cleanup
	for _, c := range clients {
		_ = c.app.Close()
		_ = c.wallet.Close()
	}
	t.Logf("Successfully processed %d messages across %d clients", totalMsgs, CLIENT_COUNT)
}
