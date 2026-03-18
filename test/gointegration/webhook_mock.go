package bridge_test

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
	"net"
	"net/http"
	"strings"
	"sync"
)

// webhookRecord stores a single received webhook.
type webhookRecord struct {
	ClientID      string `json:"client_id"`
	To            string `json:"to"`
	Message       string `json:"message"`
	TraceID       string `json:"trace_id"`
	Signature     string `json:"signature,omitempty"`
	SignatureOK   *bool  `json:"signature_ok,omitempty"`
	Authorization string `json:"authorization,omitempty"`
}

// webhookMockServer is an HTTP server that receives webhooks.
// It runs on a fixed port so the bridge container can reach it by hostname.
type webhookMockServer struct {
	server    *http.Server
	listener  net.Listener
	mu        sync.RWMutex
	records   []webhookRecord
	publicKey *rsa.PublicKey
}

// newWebhookMockServer creates and starts the mock on the given addr (e.g. ":9091").
func newWebhookMockServer(addr string) (*webhookMockServer, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}

	m := &webhookMockServer{
		records: make([]webhookRecord, 0),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/records", m.handleRecords)
	mux.HandleFunc("/reset", m.handleReset)
	mux.HandleFunc("/", m.handleWebhook) // catch-all for POST webhooks

	m.server = &http.Server{Handler: mux}
	m.listener = ln
	go m.server.Serve(ln) //nolint:errcheck

	return m, nil
}

func (m *webhookMockServer) Close() {
	_ = m.server.Close()
}

func (m *webhookMockServer) Port() int {
	return m.listener.Addr().(*net.TCPAddr).Port
}

func (m *webhookMockServer) SetPublicKey(key *rsa.PublicKey) {
	m.mu.Lock()
	m.publicKey = key
	m.mu.Unlock()
}

func (m *webhookMockServer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var rec webhookRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	rec.Signature = r.Header.Get("X-Webhook-Signature")
	rec.Authorization = r.Header.Get("Authorization")

	m.mu.RLock()
	pubKey := m.publicKey
	m.mu.RUnlock()

	if rec.Signature != "" && pubKey != nil {
		ok := verifyRSASignature(pubKey, body, rec.Signature)
		rec.SignatureOK = &ok
	}

	m.mu.Lock()
	m.records = append(m.records, rec)
	m.mu.Unlock()

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, `{"status":"ok"}`)
}

func (m *webhookMockServer) handleRecords(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(m.records)
}

func (m *webhookMockServer) handleReset(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.records = m.records[:0]
	m.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

// resetRecords clears all recorded webhooks.
func (m *webhookMockServer) resetRecords() {
	m.mu.Lock()
	m.records = m.records[:0]
	m.mu.Unlock()
}

// getRecords returns a copy of all recorded webhooks.
func (m *webhookMockServer) getRecords() []webhookRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]webhookRecord, len(m.records))
	copy(out, m.records)
	return out
}

// verifyRSASignature checks an RSA-PKCS1v15-SHA256 signature.
func verifyRSASignature(pubKey *rsa.PublicKey, body []byte, sigBase64 string) bool {
	sig, err := base64.StdEncoding.DecodeString(sigBase64)
	if err != nil {
		return false
	}
	hash := sha256.Sum256(body)
	return rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hash[:], sig) == nil
}

// fetchBridgePublicKey fetches and parses the PEM public key from the bridge.
func fetchBridgePublicKey(bridgeBaseURL string) (*rsa.PublicKey, error) {
	// bridgeBaseURL is like "http://bridge:8081/bridge"
	// public key endpoint is at "/bridge/webhook/public-key"
	resp, err := http.Get(strings.TrimRight(bridgeBaseURL, "/") + "/webhook/public-key")
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	return parseRSAPublicKeyPEM(data)
}

func parseRSAPublicKeyPEM(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("decode PEM failed")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not RSA")
	}
	return rsaPub, nil
}
