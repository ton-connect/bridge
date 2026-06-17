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
	webhookWalletAuth = "test-webhook-secret"
)

// Shared mock — started once, used by all webhook tests.
var sharedWebhookMock *webhookMockServer

func initWebhookMock(t *testing.T) {
	if sharedWebhookMock == nil {
		t.Skip("webhook mock not initialized (bridge may not support webhooks)")
	}
	if !webhookBridgeReady {
		t.Skip("bridge did not deliver probe webhook")
	}
	// Reset records between tests
	sharedWebhookMock.resetRecords()
}

// webhookBridgeReady is true once a probe webhook has been delivered successfully.
var webhookBridgeReady bool

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

	// Send probe messages until the bridge successfully delivers a webhook.
	// This is needed because in Docker the bridge container starts before the
	// gointegration container, so the bridge may not be able to reach the mock
	// immediately (DNS resolution, network setup).
	fmt.Fprintf(os.Stderr, "Waiting for bridge to deliver a probe webhook...\n")
	probeClientID := "a3f9c8e21d7b4a5e9c0f6b1d8e72c4fa9b0e1d5c7a6f84b2e93d0c1a5f7e8b42"
	probeToID := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		u, _ := url.Parse(BRIDGE_URL + "/message")
		q := u.Query()
		q.Set("client_id", probeClientID)
		q.Set("to", probeToID)
		q.Set("ttl", "60")
		q.Set("topic", "probe")
		q.Set("wallet", webhookWalletName)
		u.RawQuery = q.Encode()
		resp, err := http.Post(u.String(), "text/plain", strings.NewReader("probe"))
		if err == nil {
			_ = resp.Body.Close()
		}

		time.Sleep(2 * time.Second)

		if len(mock.getRecords()) > 0 {
			fmt.Fprintf(os.Stderr, "Webhook delivery confirmed\n")
			mock.resetRecords()
			webhookBridgeReady = true
			return
		}
	}
	fmt.Fprintf(os.Stderr, "WARNING: bridge did not deliver probe webhook within 60s, webhook tests will be skipped\n")
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

	sendMessage(t, clientID, toID, payload, map[string]string{
		"wallet": webhookWalletName,
		"topic":  "sendTransaction",
	})

	records := pollWebhooks(t, sharedWebhookMock, 1, 5*time.Second)
	rec := records[0]

	if rec.Topic != "sendTransaction" {
		t.Errorf("topic: got %q, want %q", rec.Topic, "sendTransaction")
	}
	if rec.Hash != payload {
		t.Errorf("hash: got %q, want %q", rec.Hash, payload)
	}
	if rec.Path != "/"+clientID {
		t.Errorf("path: got %q, want %q", rec.Path, "/"+clientID)
	}
	if rec.Signature == "" {
		t.Error("expected X-Webhook-Signature header")
	}
	if rec.SignatureOK != nil && !*rec.SignatureOK {
		t.Error("webhook signature is INVALID")
	}
	expectedAuth := "Bearer " + webhookWalletAuth
	if rec.Authorization != expectedAuth {
		t.Errorf("Authorization: got %q, want %q", rec.Authorization, expectedAuth)
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

	pubKey, err := parseEd25519PublicKeyPEM(body)
	if err != nil {
		t.Fatalf("parse public key: %v", err)
	}
	if len(pubKey) == 0 {
		t.Error("parsed key is empty")
	}
}

func TestBridge_WebhookMultipleMessages(t *testing.T) {
	initWebhookMock(t)

	const count = 5
	clientID := randomSessionID(t)
	toID := randomSessionID(t)

	for i := 0; i < count; i++ {
		payload := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("msg-%d", i)))
		sendMessage(t, clientID, toID, payload, map[string]string{
			"wallet": webhookWalletName,
			"topic":  "sendTransaction",
		})
	}

	records := pollWebhooks(t, sharedWebhookMock, count, 5*time.Second)

	// Collect received messages (order is not guaranteed for async webhooks)
	receivedMessages := make(map[string]bool)
	for i, rec := range records {
		if rec.SignatureOK != nil && !*rec.SignatureOK {
			t.Errorf("webhook #%d: invalid signature", i)
		}
		if rec.Topic != "sendTransaction" {
			t.Errorf("webhook #%d: topic got %q, want %q", i, rec.Topic, "sendTransaction")
		}
		receivedMessages[rec.Hash] = true
	}
	for i := 0; i < count; i++ {
		expected := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("msg-%d", i)))
		if !receivedMessages[expected] {
			t.Errorf("missing webhook for message %q", expected)
		}
	}
}
