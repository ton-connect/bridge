package webhook

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

const (
	testClientID = "a3f9c8e21d7b4a5e9c0f6b1d8e72c4fa9b0e1d5c7a6f84b2e93d0c1a5f7e8b42"
	testToID     = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func walletListServer(t *testing.T, wallets []walletListEntry) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(wallets)
	if err != nil {
		t.Fatalf("marshal wallet list: %v", err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
}

func TestService_SendAndVerifySignature(t *testing.T) {
	svc, err := NewService("", "", 0)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	pubPEM, err := svc.PublicKeyPEM()
	if err != nil {
		t.Fatalf("PublicKeyPEM: %v", err)
	}
	pubKey, err := ParsePublicKeyPEM(pubPEM)
	if err != nil {
		t.Fatalf("ParsePublicKeyPEM: %v", err)
	}

	mock := NewMock(pubKey)
	defer mock.Close()

	data := Data{
		ClientID: testClientID,
		To:       testToID,
		Message:  "dGVzdCBtZXNzYWdl",
		TraceID:  "trace-123",
	}

	svc.Send(mock.URL(), data)

	// async send — give it a moment
	time.Sleep(50 * time.Millisecond)

	records := mock.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(records))
	}

	rec := records[0]
	if rec.Payload.ClientID != testClientID {
		t.Errorf("client_id: got %q, want %q", rec.Payload.ClientID, testClientID)
	}
	if rec.Payload.To != testToID {
		t.Errorf("to: got %q, want %q", rec.Payload.To, testToID)
	}
	if rec.Payload.Message != "dGVzdCBtZXNzYWdl" {
		t.Errorf("message: got %q, want %q", rec.Payload.Message, "dGVzdCBtZXNzYWdl")
	}
	if rec.Payload.TraceID != "trace-123" {
		t.Errorf("trace_id: got %q, want %q", rec.Payload.TraceID, "trace-123")
	}
	if rec.Signature == "" {
		t.Error("expected X-Webhook-Signature header")
	}
	if rec.SignatureOK == nil || !*rec.SignatureOK {
		t.Error("signature verification failed")
	}
}

func TestService_InvalidSignatureRejected(t *testing.T) {
	svc, err := NewService("", "", 0)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Create a different key pair for the mock — signatures will mismatch
	otherSvc, err := NewService("", "", 0)
	if err != nil {
		t.Fatalf("NewService (other): %v", err)
	}
	otherPEM, _ := otherSvc.PublicKeyPEM()
	otherPub, _ := ParsePublicKeyPEM(otherPEM)

	mock := NewMock(otherPub)
	defer mock.Close()

	svc.Send(mock.URL(), Data{
		ClientID: testClientID,
		To:       testToID,
		Message:  "msg",
		TraceID:  "t",
	})

	time.Sleep(50 * time.Millisecond)

	records := mock.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(records))
	}
	if records[0].SignatureOK == nil {
		t.Fatal("expected signature verification result")
	}
	if *records[0].SignatureOK {
		t.Error("expected signature to be INVALID with mismatched keys")
	}
}

func TestService_LoadWalletList(t *testing.T) {
	mockURL := "https://webhook.example.com"
	listSrv := walletListServer(t, []walletListEntry{
		{
			AppName: "testwallet",
			Bridge: []walletListBridge{
				{Type: "sse", URL: "https://bridge.example.com", Webhook: mockURL},
			},
		},
		{
			AppName: "nowebook",
			Bridge: []walletListBridge{
				{Type: "sse", URL: "https://bridge2.example.com"},
			},
		},
		{
			AppName: "jswallet",
			Bridge: []walletListBridge{
				{Type: "js", URL: "", Webhook: "https://should-be-ignored.com"},
			},
		},
	})
	defer listSrv.Close()

	svc, err := NewService(listSrv.URL, "", 0)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	url, ok := svc.GetWebhookURL("testwallet")
	if !ok || url != mockURL {
		t.Errorf("testwallet: got %q (ok=%v), want %q", url, ok, mockURL)
	}

	_, ok = svc.GetWebhookURL("nowebook")
	if ok {
		t.Error("nowebook should have no webhook (empty webhook field)")
	}

	_, ok = svc.GetWebhookURL("jswallet")
	if ok {
		t.Error("jswallet should have no webhook (type=js, not sse)")
	}

	_, ok = svc.GetWebhookURL("unknown")
	if ok {
		t.Error("unknown wallet should not have a webhook")
	}
}

func TestService_WalletListRefresh(t *testing.T) {
	calls := 0
	listSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var wallets []walletListEntry
		if calls >= 2 {
			wallets = []walletListEntry{
				{AppName: "newwallet", Bridge: []walletListBridge{
					{Type: "sse", URL: "https://b.com", Webhook: "https://hook.com"},
				}},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(wallets)
	}))
	defer listSrv.Close()

	svc, err := NewService(listSrv.URL, "", 1) // refresh every 1 second
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Initially empty
	_, ok := svc.GetWebhookURL("newwallet")
	if ok {
		t.Error("newwallet should not exist initially")
	}

	// Wait for refresh
	time.Sleep(1500 * time.Millisecond)

	url, ok := svc.GetWebhookURL("newwallet")
	if !ok || url != "https://hook.com" {
		t.Errorf("after refresh: got %q (ok=%v), want %q", url, ok, "https://hook.com")
	}
}

func TestService_PublicKeyPEM(t *testing.T) {
	svc, err := NewService("", "", 0)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	pemBytes, err := svc.PublicKeyPEM()
	if err != nil {
		t.Fatalf("PublicKeyPEM: %v", err)
	}

	pubKey, err := ParsePublicKeyPEM(pemBytes)
	if err != nil {
		t.Fatalf("ParsePublicKeyPEM: %v", err)
	}

	if pubKey.N == nil {
		t.Error("parsed public key has nil modulus")
	}
}

func TestService_LoadPrivateKeyFromFile(t *testing.T) {
	// Generate a temp key file
	tmpFile := t.TempDir() + "/test_private.pem"
	if err := generateTestKeyFile(tmpFile); err != nil {
		t.Fatalf("generate test key: %v", err)
	}

	svc, err := NewService("", tmpFile, 0)
	if err != nil {
		t.Fatalf("NewService with key file: %v", err)
	}

	pemBytes, err := svc.PublicKeyPEM()
	if err != nil {
		t.Fatalf("PublicKeyPEM: %v", err)
	}
	if len(pemBytes) == 0 {
		t.Error("expected non-empty PEM output")
	}
}

func TestService_EndToEnd(t *testing.T) {
	// 1. Create a mock without verification first (we need its URL for the wallet list)
	mock := NewMock(nil)
	defer mock.Close()

	// 2. Set up wallet list pointing to mock
	listSrv := walletListServer(t, []walletListEntry{
		{
			AppName: "testwallet",
			Bridge: []walletListBridge{
				{Type: "sse", URL: "https://bridge.example.com", Webhook: mock.URL()},
			},
		},
	})
	defer listSrv.Close()

	// 3. Create service with wallet list
	svc, err := NewService(listSrv.URL, "", 0)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// 4. Now set the mock's public key from the service
	pubPEM, _ := svc.PublicKeyPEM()
	pubKey, _ := ParsePublicKeyPEM(pubPEM)
	mock.mu.Lock()
	mock.publicKey = pubKey
	mock.mu.Unlock()

	// 3. Lookup and send
	webhookURL, ok := svc.GetWebhookURL("testwallet")
	if !ok {
		t.Fatal("testwallet webhook not found")
	}

	messages := []Data{
		{ClientID: testClientID, To: testToID, Message: "msg1", TraceID: "t1"},
		{ClientID: testClientID, To: testToID, Message: "msg2", TraceID: "t2"},
		{ClientID: testClientID, To: testToID, Message: "msg3", TraceID: "t3"},
	}
	for _, msg := range messages {
		svc.Send(webhookURL, msg)
	}

	time.Sleep(100 * time.Millisecond)

	// 4. Verify
	records := mock.Records()
	if len(records) != 3 {
		t.Fatalf("expected 3 webhooks, got %d", len(records))
	}

	for i, rec := range records {
		expected := messages[i]
		if rec.Payload.Message != expected.Message {
			t.Errorf("webhook %d: message got %q, want %q", i, rec.Payload.Message, expected.Message)
		}
		if rec.Payload.TraceID != expected.TraceID {
			t.Errorf("webhook %d: trace_id got %q, want %q", i, rec.Payload.TraceID, expected.TraceID)
		}
		if rec.SignatureOK == nil || !*rec.SignatureOK {
			t.Errorf("webhook %d: signature invalid", i)
		}
	}

	// 5. Verify unknown wallet returns false
	_, ok = svc.GetWebhookURL("unknown")
	if ok {
		t.Error("unknown wallet should not have a webhook")
	}
}

func TestMock_Reset(t *testing.T) {
	mock := NewMock(nil)
	defer mock.Close()

	svc, _ := NewService("", "", 0)
	svc.Send(mock.URL(), Data{ClientID: "c", To: "t", Message: "m", TraceID: "tr"})
	time.Sleep(50 * time.Millisecond)

	if len(mock.Records()) != 1 {
		t.Fatalf("expected 1 record, got %d", len(mock.Records()))
	}

	mock.Reset()
	if len(mock.Records()) != 0 {
		t.Fatalf("expected 0 records after reset, got %d", len(mock.Records()))
	}
}

func TestService_WebhookToDownServer(t *testing.T) {
	svc, _ := NewService("", "", 0)

	// Send to a closed server — should log error, not panic
	mock := NewMock(nil)
	url := mock.URL()
	mock.Close()

	svc.Send(url, Data{ClientID: "c", To: "t", Message: "m", TraceID: "tr"})
	// No panic = pass
}

func TestService_WalletListBadURL(t *testing.T) {
	svc, err := NewService("http://localhost:1/nonexistent", "", 0)
	if err != nil {
		t.Fatalf("NewService should not fail on bad wallet list URL: %v", err)
	}
	// Should have empty webhooks
	_, ok := svc.GetWebhookURL("anything")
	if ok {
		t.Error("should have no webhooks with bad wallet list URL")
	}
	_ = svc
}

func TestService_WalletListBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "not json")
	}))
	defer srv.Close()

	svc, err := NewService(srv.URL, "", 0)
	if err != nil {
		t.Fatalf("NewService should not fail on bad JSON: %v", err)
	}
	_, ok := svc.GetWebhookURL("anything")
	if ok {
		t.Error("should have no webhooks with bad JSON")
	}
	_ = svc
}

func TestMock_WithoutPublicKey(t *testing.T) {
	mock := NewMock(nil) // no signature verification
	defer mock.Close()

	svc, _ := NewService("", "", 0)
	svc.Send(mock.URL(), Data{ClientID: "c", To: "t", Message: "m", TraceID: "tr"})
	time.Sleep(50 * time.Millisecond)

	records := mock.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1, got %d", len(records))
	}
	if records[0].Signature == "" {
		t.Error("expected signature header to be present")
	}
	if records[0].SignatureOK != nil {
		t.Error("SignatureOK should be nil when mock has no public key")
	}
}

func generateTestKeyFile(path string) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return pem.Encode(f, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}
