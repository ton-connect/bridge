package webhook

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
)

// Record represents a single received webhook.
type Record struct {
	Payload       WebhookData
	Signature     string
	SignatureOK   *bool
	RawBody       []byte
	Authorization string
}

// Mock is an httptest-based webhook receiver for use in tests.
type Mock struct {
	Server *httptest.Server

	mu        sync.RWMutex
	records   []Record
	publicKey *rsa.PublicKey
}

// NewMock creates a mock webhook server. If publicKey is provided,
// signatures will be verified on each received webhook.
func NewMock(publicKey *rsa.PublicKey) *Mock {
	m := &Mock{
		records:   make([]Record, 0),
		publicKey: publicKey,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", m.handle)
	m.Server = httptest.NewServer(mux)
	return m
}

// Close shuts down the mock server.
func (m *Mock) Close() {
	m.Server.Close()
}

// URL returns the base URL of the mock server.
func (m *Mock) URL() string {
	return m.Server.URL
}

// Records returns a copy of all received webhook records.
func (m *Mock) Records() []Record {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Record, len(m.records))
	copy(out, m.records)
	return out
}

// SetPublicKey updates the public key used for signature verification.
func (m *Mock) SetPublicKey(key *rsa.PublicKey) {
	m.mu.Lock()
	m.publicKey = key
	m.mu.Unlock()
}

// Reset clears all recorded webhooks.
func (m *Mock) Reset() {
	m.mu.Lock()
	m.records = m.records[:0]
	m.mu.Unlock()
}

func (m *Mock) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var payload WebhookData
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	rec := Record{
		Payload:       payload,
		RawBody:       body,
		Authorization: r.Header.Get("Authorization"),
	}

	sig := r.Header.Get("X-Webhook-Signature")
	if sig != "" {
		rec.Signature = sig
		if m.publicKey != nil {
			ok := verifySignature(m.publicKey, body, sig)
			rec.SignatureOK = &ok
		}
	}

	m.mu.Lock()
	m.records = append(m.records, rec)
	m.mu.Unlock()

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, `{"status":"ok"}`)
}

func verifySignature(pubKey *rsa.PublicKey, body []byte, sigBase64 string) bool {
	sig, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		return false
	}
	hash := sha256.Sum256(body)
	return rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hash[:], sig) == nil
}

// ParsePublicKeyPEM parses a PEM-encoded public key.
func ParsePublicKeyPEM(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}
	return rsaPub, nil
}
