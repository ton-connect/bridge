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
	webhookMockPort   = 9091
	webhookWalletName = "testwallet"
)

// webhookMockAddr returns the address the bridge container uses to reach the mock.
// In Docker: "http://bridge-gointegration:9091". Locally: "http://localhost:9091".
func webhookMockAddr() string {
	if v := os.Getenv("WEBHOOK_MOCK_HOST"); v != "" {
		return fmt.Sprintf("http://%s:%d", v, webhookMockPort)
	}
	return fmt.Sprintf("http://localhost:%d", webhookMockPort)
}

// Shared mock — started once, used by all webhook tests.
var sharedWebhookMock *webhookMockServer

func initWebhookMock(t *testing.T) {
	if sharedWebhookMock == nil {
		t.Skip("webhook mock not initialized (bridge may not support webhooks)")
	}
	// Reset records between tests
	sharedWebhookMock.resetRecords()
}

func setupSharedWebhookMock() {
	mock, err := newWebhookMockServer(fmt.Sprintf(":%d", webhookMockPort))
	if err != nil {
		fmt.Fprintf(os.Stderr, "webhook mock: %v (webhook tests will be skipped)\n", err)
		return
	}

	// Check if bridge supports webhooks
	pubKey, err := fetchBridgePublicKey(BRIDGE_URL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bridge does not support webhooks: %v (webhook tests will be skipped)\n", err)
		mock.Close()
		return
	}
	mock.SetPublicKey(pubKey)
	sharedWebhookMock = mock
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
	defer func() { _ = resp.Body.Close() }()
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
	initWebhookMock(t)

	clientID := randomSessionID(t)
	toID := randomSessionID(t)
	payload := base64.StdEncoding.EncodeToString([]byte("hello webhook"))

	sendMessage(t, clientID, toID, payload, map[string]string{"wallet": webhookWalletName})

	records := pollWebhooks(t, sharedWebhookMock, 1, 5*time.Second)
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
	initWebhookMock(t)

	sendMessage(t, randomSessionID(t), randomSessionID(t), "test", map[string]string{"wallet": "unknownwallet"})

	time.Sleep(2 * time.Second)
	if len(sharedWebhookMock.getRecords()) != 0 {
		t.Errorf("expected 0 webhooks for unknown wallet, got %d", len(sharedWebhookMock.getRecords()))
	}
}

func TestBridge_WebhookNotSentWithoutWalletParam(t *testing.T) {
	initWebhookMock(t)

	sendMessage(t, randomSessionID(t), randomSessionID(t), "test", nil)

	time.Sleep(2 * time.Second)
	if len(sharedWebhookMock.getRecords()) != 0 {
		t.Errorf("expected 0 webhooks without wallet param, got %d", len(sharedWebhookMock.getRecords()))
	}
}

func TestBridge_WebhookPublicKeyEndpoint(t *testing.T) {
	resp, err := http.Get(strings.TrimRight(BRIDGE_URL, "/") + "/webhook/public-key")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Skipf("bridge does not support webhook public key endpoint (got %d), skipping", resp.StatusCode)
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
	initWebhookMock(t)

	const count = 5
	clientID := randomSessionID(t)
	toID := randomSessionID(t)

	for i := 0; i < count; i++ {
		payload := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("msg-%d", i)))
		sendMessage(t, clientID, toID, payload, map[string]string{"wallet": webhookWalletName})
	}

	records := pollWebhooks(t, sharedWebhookMock, count, 5*time.Second)

	// Collect received messages (order is not guaranteed for async webhooks)
	receivedMessages := make(map[string]bool)
	for i, rec := range records {
		if rec.SignatureOK != nil && !*rec.SignatureOK {
			t.Errorf("webhook #%d: invalid signature", i)
		}
		receivedMessages[rec.Message] = true
	}
	for i := 0; i < count; i++ {
		expected := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("msg-%d", i)))
		if !receivedMessages[expected] {
			t.Errorf("missing webhook for message %q", expected)
		}
	}
}
