package bridge_test

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	webhookMockPort    = 9091
	webhookWalletName  = "testwallet"
	webhookWaitTimeout = 15 * time.Second
)

// webhookMockAddr returns the address the bridge container uses to reach the mock.
// In Docker: "http://bridge-gointegration:9091". Locally: "http://localhost:9091".
func webhookMockAddr() string {
	if v := os.Getenv("WEBHOOK_MOCK_HOST"); v != "" {
		return fmt.Sprintf("http://%s:%d", v, webhookMockPort)
	}
	return fmt.Sprintf("http://localhost:%d", webhookMockPort)
}

// startWebhookMock starts the mock server and waits for the bridge to pick up
// the wallet list (via its refresh interval).
func startWebhookMock(t *testing.T) *webhookMockServer {
	t.Helper()

	externalAddr := webhookMockAddr()
	mock, err := newWebhookMockServer(
		fmt.Sprintf(":%d", webhookMockPort),
		webhookWalletName,
		externalAddr+"/", // webhook POST target
	)
	if err != nil {
		t.Fatalf("start webhook mock: %v", err)
	}

	// Fetch the bridge's public key for signature verification
	pubKey, err := fetchBridgePublicKey(BRIDGE_URL)
	if err != nil {
		t.Logf("WARNING: could not fetch bridge public key: %v (signature verification disabled)", err)
	} else {
		mock.SetPublicKey(pubKey)
	}

	// Wait for the bridge to refresh its wallet list and pick up our mock.
	// WALLET_LIST_REFRESH_INTERVAL is set to 5s in docker-compose.
	t.Log("Waiting for bridge to refresh wallet list...")
	time.Sleep(7 * time.Second)

	return mock
}

func sendMessage(t *testing.T, clientID, toID, payload string, extra map[string]string) {
	t.Helper()

	u, _ := url.Parse(BRIDGE_URL + "/message")
	q := u.Query()
	q.Set("client_id", clientID)
	q.Set("to", toID)
	q.Set("ttl", "60")
	for k, v := range extra {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	resp, err := http.Post(u.String(), "text/plain", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", u.String(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
}

func pollWebhooks(t *testing.T, mock *webhookMockServer, n int, timeout time.Duration) []webhookRecord {
	t.Helper()
	deadline := time.After(timeout)
	for {
		records := mock.getRecords()
		if len(records) >= n {
			return records
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d webhook(s) (got %d)", n, len(records))
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func TestBridge_WebhookSentOnMessage(t *testing.T) {
	mock := startWebhookMock(t)
	defer mock.Close()

	clientID := randomSessionID(t)
	toID := randomSessionID(t)
	payload := base64.StdEncoding.EncodeToString([]byte("hello webhook"))

	sendMessage(t, clientID, toID, payload, map[string]string{"wallet": webhookWalletName})

	records := pollWebhooks(t, mock, 1, webhookWaitTimeout)
	rec := records[0]

	if rec.ClientID != clientID {
		t.Errorf("client_id: got %q, want %q", rec.ClientID, clientID)
	}
	if rec.To != toID {
		t.Errorf("to: got %q, want %q", rec.To, toID)
	}
	if rec.Message != payload {
		t.Errorf("message: got %q, want %q", rec.Message, payload)
	}
	if rec.Signature == "" {
		t.Error("expected X-Webhook-Signature header")
	}
	if rec.SignatureOK != nil && !*rec.SignatureOK {
		t.Error("webhook signature is INVALID")
	}
}

func TestBridge_WebhookNoWebhookForUnknownWallet(t *testing.T) {
	mock := startWebhookMock(t)
	defer mock.Close()

	sendMessage(t, randomSessionID(t), randomSessionID(t), "test", map[string]string{"wallet": "unknownwallet"})

	time.Sleep(2 * time.Second)
	if len(mock.getRecords()) != 0 {
		t.Errorf("expected 0 webhooks for unknown wallet, got %d", len(mock.getRecords()))
	}
}

func TestBridge_WebhookNotSentWithoutWalletParam(t *testing.T) {
	mock := startWebhookMock(t)
	defer mock.Close()

	sendMessage(t, randomSessionID(t), randomSessionID(t), "test", nil)

	time.Sleep(2 * time.Second)
	if len(mock.getRecords()) != 0 {
		t.Errorf("expected 0 webhooks without wallet param, got %d", len(mock.getRecords()))
	}
}

func TestBridge_WebhookPublicKeyEndpoint(t *testing.T) {
	// BRIDGE_URL is like "http://bridge:8081/bridge"
	resp, err := http.Get(strings.TrimRight(BRIDGE_URL, "/") + "/webhook/public-key")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "BEGIN PUBLIC KEY") {
		t.Error("expected PEM public key in response")
	}

	pubKey, err := parseRSAPublicKeyPEM(body)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}
	if pubKey.N == nil {
		t.Error("parsed key has nil modulus")
	}
}

func TestBridge_WebhookMultipleMessages(t *testing.T) {
	mock := startWebhookMock(t)
	defer mock.Close()

	const count = 5
	clientID := randomSessionID(t)
	toID := randomSessionID(t)

	for i := 0; i < count; i++ {
		payload := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("msg-%d", i)))
		sendMessage(t, clientID, toID, payload, map[string]string{"wallet": webhookWalletName})
	}

	records := pollWebhooks(t, mock, count, webhookWaitTimeout)

	for i, rec := range records {
		if rec.SignatureOK != nil && !*rec.SignatureOK {
			t.Errorf("webhook #%d: invalid signature", i)
		}
		expected := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("msg-%d", i)))
		if rec.Message != expected {
			t.Errorf("webhook #%d: message got %q, want %q", i, rec.Message, expected)
		}
	}
}
