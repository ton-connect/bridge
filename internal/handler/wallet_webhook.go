package handler

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// WalletWebhookData is the payload sent to wallet webhook endpoints.
type WalletWebhookData struct {
	ClientID string `json:"client_id"`
	To       string `json:"to"`
	Message  string `json:"message"`
	TraceID  string `json:"trace_id"`
}

// walletListEntry mirrors the relevant fields from wallets-v2.json.
type walletListEntry struct {
	AppName string             `json:"app_name"`
	Bridge  []walletListBridge `json:"bridge"`
}

type walletListBridge struct {
	Type    string `json:"type"`
	URL     string `json:"url"`
	Webhook string `json:"webhook"`
}

// WalletWebhookService fetches wallet webhook URLs from a remote wallet list
// and sends signed webhook notifications.
type WalletWebhookService struct {
	walletListURL string
	privateKey    *rsa.PrivateKey

	mu       sync.RWMutex
	webhooks map[string]string // app_name → webhook URL
}

// NewWalletWebhookService creates the service, loads the private key,
// fetches the wallet list, and optionally starts a background refresh loop.
func NewWalletWebhookService(walletListURL string, privateKeyPath string, refreshSec int) (*WalletWebhookService, error) {
	s := &WalletWebhookService{
		walletListURL: walletListURL,
		webhooks:      make(map[string]string),
	}

	if privateKeyPath != "" {
		key, err := loadPrivateKey(privateKeyPath)
		if err != nil {
			return nil, fmt.Errorf("load webhook private key: %w", err)
		}
		s.privateKey = key
		log.Info("Webhook RSA private key loaded from file")
	} else {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, fmt.Errorf("generate webhook private key: %w", err)
		}
		s.privateKey = key
		log.Info("Webhook RSA private key generated (2048-bit)")
	}

	if walletListURL != "" {
		if err := s.refresh(); err != nil {
			log.Errorf("initial wallet list fetch failed: %v", err)
		}
		if refreshSec > 0 {
			go s.refreshLoop(time.Duration(refreshSec) * time.Second)
		}
	}

	return s, nil
}

// GetWebhookURL returns the webhook URL for a given wallet app_name.
func (s *WalletWebhookService) GetWebhookURL(wallet string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	url, ok := s.webhooks[wallet]
	return url, ok
}

// Send sends a signed webhook to the given URL with the provided data.
func (s *WalletWebhookService) Send(webhookURL string, data WalletWebhookData) {
	if err := s.send(webhookURL, data); err != nil {
		log.Errorf("failed to send wallet webhook to '%s': %v", webhookURL, err)
	}
}

// PublicKeyPEM returns the PEM-encoded public key for signature verification.
func (s *WalletWebhookService) PublicKeyPEM() ([]byte, error) {
	if s.privateKey == nil {
		return nil, fmt.Errorf("no private key loaded")
	}
	pubBytes, err := x509.MarshalPKIXPublicKey(&s.privateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("marshal public key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}), nil
}

func (s *WalletWebhookService) refreshLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		if err := s.refresh(); err != nil {
			log.Errorf("failed to refresh wallet list: %v", err)
		}
	}
}

func (s *WalletWebhookService) refresh() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.walletListURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch wallet list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wallet list returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read wallet list body: %w", err)
	}

	var wallets []walletListEntry
	if err := json.Unmarshal(body, &wallets); err != nil {
		return fmt.Errorf("parse wallet list: %w", err)
	}

	newMap := make(map[string]string)
	for _, w := range wallets {
		for _, b := range w.Bridge {
			if b.Type == "sse" && b.Webhook != "" {
				newMap[w.AppName] = b.Webhook
				break
			}
		}
	}

	s.mu.Lock()
	s.webhooks = newMap
	s.mu.Unlock()

	log.WithField("count", len(newMap)).Info("Wallet webhooks loaded from wallet list")
	return nil
}

func (s *WalletWebhookService) send(webhookURL string, data WalletWebhookData) error {
	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal webhook data: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if s.privateKey != nil {
		hash := sha256.Sum256(body)
		signature, err := rsa.SignPKCS1v15(rand.Reader, s.privateKey, crypto.SHA256, hash[:])
		if err != nil {
			return fmt.Errorf("sign webhook body: %w", err)
		}
		req.Header.Set("X-Webhook-Signature", base64.StdEncoding.EncodeToString(signature))
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer func() {
		if closeErr := res.Body.Close(); closeErr != nil {
			log.Errorf("failed to close response body: %v", closeErr)
		}
	}()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code: %v", res.StatusCode)
	}
	return nil
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key file: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		keyIface, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("parse private key (PKCS1: %w, PKCS8: %w)", err, err2)
		}
		rsaKey, ok := keyIface.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA")
		}
		return rsaKey, nil
	}
	return key, nil
}
