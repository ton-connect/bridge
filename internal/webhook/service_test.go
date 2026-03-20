package webhook

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

const testClientID = "a3f9c8e21d7b4a5e9c0f6b1d8e72c4fa9b0e1d5c7a6f84b2e93d0c1a5f7e8b42"

func TestService_SendAndVerifySignature(t *testing.T) {
	svc, err := NewService("", "")
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

	data := WebhookData{
		Topic: "sendTransaction",
		Hash:  "dGVzdCBtZXNzYWdl",
	}

	svc.Send(WalletConfig{URL: mock.URL()}, testClientID, data)

	// async send — give it a moment
	time.Sleep(50 * time.Millisecond)

	records := mock.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 webhook, got %d", len(records))
	}

	rec := records[0]
	if rec.Payload.Topic != "sendTransaction" {
		t.Errorf("topic: got %q, want %q", rec.Payload.Topic, "sendTransaction")
	}
	if rec.Payload.Hash != "dGVzdCBtZXNzYWdl" {
		t.Errorf("hash: got %q, want %q", rec.Payload.Hash, "dGVzdCBtZXNzYWdl")
	}
	if rec.Path != "/"+testClientID {
		t.Errorf("path: got %q, want %q", rec.Path, "/"+testClientID)
	}
	if rec.Signature == "" {
		t.Error("expected X-Webhook-Signature header")
	}
	if rec.SignatureOK == nil || !*rec.SignatureOK {
		t.Error("signature verification failed")
	}
}

func TestService_InvalidSignatureRejected(t *testing.T) {
	svc, err := NewService("", "")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// Create a different key pair for the mock — signatures will mismatch
	otherSvc, err := NewService("", "")
	if err != nil {
		t.Fatalf("NewService (other): %v", err)
	}
	otherPEM, _ := otherSvc.PublicKeyPEM()
	otherPub, _ := ParsePublicKeyPEM(otherPEM)

	mock := NewMock(otherPub)
	defer mock.Close()

	svc.Send(WalletConfig{URL: mock.URL()}, testClientID, WebhookData{
		Topic: "test",
		Hash:  "msg",
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

func TestService_ParseWebhookConfig(t *testing.T) {
	configJSON := `{"wallets":{
		"testwallet":{"url":"https://webhook.example.com","auth":"secret-1"},
		"otherwallet":{"url":"https://other.example.com/hook"}
	}}`

	svc, err := NewService(configJSON, "")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	cfg, ok := svc.GetWalletConfig("testwallet")
	if !ok {
		t.Fatal("testwallet not found")
	}
	if cfg.URL != "https://webhook.example.com" {
		t.Errorf("testwallet URL: got %q", cfg.URL)
	}
	if cfg.Auth != "secret-1" {
		t.Errorf("testwallet Auth: got %q, want %q", cfg.Auth, "secret-1")
	}

	cfg, ok = svc.GetWalletConfig("otherwallet")
	if !ok {
		t.Fatal("otherwallet not found")
	}
	if cfg.URL != "https://other.example.com/hook" {
		t.Errorf("otherwallet URL: got %q", cfg.URL)
	}
	if cfg.Auth != "" {
		t.Errorf("otherwallet Auth: got %q, want empty", cfg.Auth)
	}

	_, ok = svc.GetWalletConfig("unknown")
	if ok {
		t.Error("unknown wallet should not have a config")
	}
}

func TestService_EmptyWebhookConfig(t *testing.T) {
	svc, err := NewService("", "")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, ok := svc.GetWalletConfig("anything")
	if ok {
		t.Error("should have no webhooks with empty config")
	}
}

func TestService_AuthTokenSent(t *testing.T) {
	mock := NewMock(nil)
	defer mock.Close()

	svc, err := NewService("", "")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	svc.Send(WalletConfig{URL: mock.URL(), Auth: "my-secret-token"}, testClientID, WebhookData{
		Topic: "test", Hash: "m",
	})
	time.Sleep(50 * time.Millisecond)

	records := mock.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Authorization != "Bearer my-secret-token" {
		t.Errorf("Authorization: got %q, want %q", records[0].Authorization, "Bearer my-secret-token")
	}
}

func TestService_NoAuthTokenWhenEmpty(t *testing.T) {
	mock := NewMock(nil)
	defer mock.Close()

	svc, err := NewService("", "")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	svc.Send(WalletConfig{URL: mock.URL()}, testClientID, WebhookData{
		Topic: "test", Hash: "m",
	})
	time.Sleep(50 * time.Millisecond)

	records := mock.Records()
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].Authorization != "" {
		t.Errorf("expected no Authorization header, got %q", records[0].Authorization)
	}
}

func TestService_InvalidWebhookConfigJSON(t *testing.T) {
	_, err := NewService("not json", "")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestService_PublicKeyPEM(t *testing.T) {
	svc, err := NewService("", "")
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
	tmpFile := t.TempDir() + "/test_private.pem"
	if err := generateTestKeyFile(tmpFile); err != nil {
		t.Fatalf("generate test key: %v", err)
	}

	svc, err := NewService("", tmpFile)
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

func TestService_LoadPrivateKeyFromInlinePEM(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pemData := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	svc, err := NewServiceWithOptions(Options{
		PrivateKeyPEM: string(pemData),
	})
	if err != nil {
		t.Fatalf("NewServiceWithOptions: %v", err)
	}

	pubPEM, err := svc.PublicKeyPEM()
	if err != nil {
		t.Fatalf("PublicKeyPEM: %v", err)
	}
	if len(pubPEM) == 0 {
		t.Error("expected non-empty PEM output")
	}

	// Verify it's the same key
	pubKey, _ := ParsePublicKeyPEM(pubPEM)
	if pubKey.N.Cmp(key.PublicKey.N) != 0 {
		t.Error("public key does not match the inline private key")
	}
}

func TestService_InlinePEMTakesPrecedenceOverFile(t *testing.T) {
	// Generate two different keys
	inlineKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	inlinePEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(inlineKey),
	})

	tmpFile := t.TempDir() + "/test_private.pem"
	if err := generateTestKeyFile(tmpFile); err != nil {
		t.Fatalf("generate test key: %v", err)
	}

	svc, err := NewServiceWithOptions(Options{
		PrivateKeyPEM:  string(inlinePEM),
		PrivateKeyPath: tmpFile,
	})
	if err != nil {
		t.Fatalf("NewServiceWithOptions: %v", err)
	}

	pubPEM, _ := svc.PublicKeyPEM()
	pubKey, _ := ParsePublicKeyPEM(pubPEM)
	if pubKey.N.Cmp(inlineKey.PublicKey.N) != 0 {
		t.Error("expected inline PEM key to take precedence over file")
	}
}

func TestService_EndToEnd(t *testing.T) {
	// 1. Create a mock without verification first (we need its URL for the config)
	mock := NewMock(nil)
	defer mock.Close()

	// 2. Create service with webhook config pointing to mock
	configJSON := fmt.Sprintf(`{"wallets":{"testwallet":{"url":"%s","auth":"e2e-token"}}}`, mock.URL())
	svc, err := NewService(configJSON, "")
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	// 3. Now set the mock's public key from the service
	pubPEM, _ := svc.PublicKeyPEM()
	pubKey, _ := ParsePublicKeyPEM(pubPEM)
	mock.mu.Lock()
	mock.publicKey = pubKey
	mock.mu.Unlock()

	// 4. Lookup and send
	walletCfg, ok := svc.GetWalletConfig("testwallet")
	if !ok {
		t.Fatal("testwallet config not found")
	}

	messages := []WebhookData{
		{Topic: "topic1", Hash: "msg1"},
		{Topic: "topic2", Hash: "msg2"},
		{Topic: "topic3", Hash: "msg3"},
	}
	for _, msg := range messages {
		svc.Send(walletCfg, testClientID, msg)
	}

	time.Sleep(100 * time.Millisecond)

	// 5. Verify
	records := mock.Records()
	if len(records) != 3 {
		t.Fatalf("expected 3 webhooks, got %d", len(records))
	}

	for i, rec := range records {
		expected := messages[i]
		if rec.Payload.Topic != expected.Topic {
			t.Errorf("webhook %d: topic got %q, want %q", i, rec.Payload.Topic, expected.Topic)
		}
		if rec.Payload.Hash != expected.Hash {
			t.Errorf("webhook %d: hash got %q, want %q", i, rec.Payload.Hash, expected.Hash)
		}
		if rec.SignatureOK == nil || !*rec.SignatureOK {
			t.Errorf("webhook %d: signature invalid", i)
		}
		if rec.Authorization != "Bearer e2e-token" {
			t.Errorf("webhook %d: Authorization got %q, want %q", i, rec.Authorization, "Bearer e2e-token")
		}
	}

	// 6. Verify unknown wallet returns false
	_, ok = svc.GetWalletConfig("unknown")
	if ok {
		t.Error("unknown wallet should not have a config")
	}
}

func TestMock_Reset(t *testing.T) {
	mock := NewMock(nil)
	defer mock.Close()

	svc, _ := NewService("", "")
	svc.Send(WalletConfig{URL: mock.URL()}, testClientID, WebhookData{Topic: "test", Hash: "m"})
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
	svc, _ := NewService("", "")

	// Send to a closed server — should log error, not panic
	mock := NewMock(nil)
	url := mock.URL()
	mock.Close()

	svc.Send(WalletConfig{URL: url}, testClientID, WebhookData{Topic: "test", Hash: "m"})
	// No panic = pass
}

func TestMock_WithoutPublicKey(t *testing.T) {
	mock := NewMock(nil) // no signature verification
	defer mock.Close()

	svc, _ := NewService("", "")
	svc.Send(WalletConfig{URL: mock.URL()}, testClientID, WebhookData{Topic: "test", Hash: "m"})
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

func TestService_LoadsWebhookConfigFromFileSource(t *testing.T) {
	path := t.TempDir() + "/webhooks.json"
	if err := os.WriteFile(path, []byte(`{"wallets":{"filewallet":{"url":"https://file.example.com/hook","auth":"file-token"}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	svc, err := NewServiceWithOptions(Options{
		ConfigSource:    path,
		RefreshInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("NewServiceWithOptions: %v", err)
	}
	defer svc.Close()

	cfg, ok := svc.GetWalletConfig("filewallet")
	if !ok {
		t.Fatal("filewallet not found")
	}
	if cfg.URL != "https://file.example.com/hook" {
		t.Fatalf("URL: got %q, want %q", cfg.URL, "https://file.example.com/hook")
	}
	if cfg.Auth != "file-token" {
		t.Fatalf("Auth: got %q, want %q", cfg.Auth, "file-token")
	}
}

func TestService_LoadsWebhookConfigFromHTTPSource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"wallets":{"httpwallet":{"url":"https://http.example.com/hook","auth":"http-token"}}}`)
	}))
	defer server.Close()

	svc, err := NewServiceWithOptions(Options{
		ConfigSource:    server.URL,
		RefreshInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("NewServiceWithOptions: %v", err)
	}
	defer svc.Close()

	cfg, ok := svc.GetWalletConfig("httpwallet")
	if !ok {
		t.Fatal("httpwallet not found")
	}
	if cfg.URL != "https://http.example.com/hook" {
		t.Fatalf("URL: got %q, want %q", cfg.URL, "https://http.example.com/hook")
	}
	if cfg.Auth != "http-token" {
		t.Fatalf("Auth: got %q, want %q", cfg.Auth, "http-token")
	}
}

func TestService_MergesInlineAndSourceWebhookConfig(t *testing.T) {
	path := t.TempDir() + "/webhooks.json"
	if err := os.WriteFile(path, []byte(`{"wallets":{
		"sourcewallet":{"url":"https://source.example.com/hook"},
		"shared":{"url":"https://source.example.com/shared","auth":"source-token"}
	}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	svc, err := NewServiceWithOptions(Options{
		InlineConfigJSON: `{"wallets":{
			"inlinewallet":{"url":"https://inline.example.com/hook","auth":"inline-token"},
			"shared":{"url":"https://inline.example.com/shared","auth":"inline-token"}
		}}`,
		ConfigSource:    path,
		RefreshInterval: time.Second,
	})
	if err != nil {
		t.Fatalf("NewServiceWithOptions: %v", err)
	}
	defer svc.Close()

	inlineCfg, ok := svc.GetWalletConfig("inlinewallet")
	if !ok || inlineCfg.URL != "https://inline.example.com/hook" {
		t.Fatalf("inlinewallet: got %+v, exists=%v", inlineCfg, ok)
	}

	sourceCfg, ok := svc.GetWalletConfig("sourcewallet")
	if !ok || sourceCfg.URL != "https://source.example.com/hook" {
		t.Fatalf("sourcewallet: got %+v, exists=%v", sourceCfg, ok)
	}

	sharedCfg, ok := svc.GetWalletConfig("shared")
	if !ok {
		t.Fatal("shared wallet not found")
	}
	if sharedCfg.URL != "https://source.example.com/shared" {
		t.Fatalf("shared URL: got %q, want %q", sharedCfg.URL, "https://source.example.com/shared")
	}
	if sharedCfg.Auth != "source-token" {
		t.Fatalf("shared Auth: got %q, want %q", sharedCfg.Auth, "source-token")
	}
}

func TestService_RefreshesWebhookConfigFromSource(t *testing.T) {
	path := t.TempDir() + "/webhooks.json"
	if err := os.WriteFile(path, []byte(`{"wallets":{"refreshwallet":{"url":"https://old.example.com/hook","auth":"old-token"}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	svc, err := NewServiceWithOptions(Options{
		ConfigSource:    path,
		RefreshInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewServiceWithOptions: %v", err)
	}
	defer svc.Close()

	cfg, ok := svc.GetWalletConfig("refreshwallet")
	if !ok || cfg.URL != "https://old.example.com/hook" {
		t.Fatalf("initial refreshwallet: got %+v, exists=%v", cfg, ok)
	}

	if err := os.WriteFile(path, []byte(`{"wallets":{"refreshwallet":{"url":"https://new.example.com/hook","auth":"new-token"}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile updated config: %v", err)
	}

	waitForWalletConfig(t, svc, "refreshwallet", "https://new.example.com/hook", "new-token", 2*time.Second)
}

func TestService_SourceRequiresPositiveRefreshInterval(t *testing.T) {
	path := t.TempDir() + "/webhooks.json"
	if err := os.WriteFile(path, []byte(`{"wallets":{"wallet":{"url":"https://example.com/hook"}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := NewServiceWithOptions(Options{
		ConfigSource:    path,
		RefreshInterval: 0,
	})
	if err == nil {
		t.Fatal("expected error for zero refresh interval with config source")
	}
}

func waitForWalletConfig(t *testing.T, svc *Service, wallet, wantURL, wantAuth string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		cfg, ok := svc.GetWalletConfig(wallet)
		if ok && cfg.URL == wantURL && cfg.Auth == wantAuth {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	cfg, ok := svc.GetWalletConfig(wallet)
	t.Fatalf("timed out waiting for wallet=%q url=%q auth=%q, got cfg=%+v exists=%v", wallet, wantURL, wantAuth, cfg, ok)
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
