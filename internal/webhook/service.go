package webhook

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"

	log "github.com/sirupsen/logrus"
)

// WebhookData matches the legacy webhook payload used on master.
type WebhookData struct {
	Topic string `json:"topic"`
	Hash  string `json:"hash"`
}

// WalletConfig holds per-wallet webhook configuration.
type WalletConfig struct {
	URL  string `json:"url"`
	Auth string `json:"auth,omitempty"`
}

// Service delivers signed webhook notifications to wallet endpoints.
// The webhook config map is provided via a JSON string at construction time
// and is immutable for the lifetime of the service.
type Service struct {
	privateKey *rsa.PrivateKey
	webhooks   map[string]WalletConfig // app_name → config
}

// NewService creates the service, loads or generates the RSA private key,
// and parses the webhook config from a JSON string.
// webhookConfigJSON is a JSON object like {"wallet_name":{"url":"https://...","auth":"token"}}.
// An empty string means no webhooks are configured.
func NewService(webhookConfigJSON string, privateKeyPath string) (*Service, error) {
	s := &Service{
		webhooks: make(map[string]WalletConfig),
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

	if webhookConfigJSON != "" {
		if err := json.Unmarshal([]byte(webhookConfigJSON), &s.webhooks); err != nil {
			return nil, fmt.Errorf("parse WEBHOOK_CONFIG: %w", err)
		}
	}

	log.WithField("count", len(s.webhooks)).Info("Wallet webhooks configured")
	return s, nil
}

// GetWalletConfig returns the webhook configuration for a given wallet app_name.
func (s *Service) GetWalletConfig(wallet string) (WalletConfig, bool) {
	cfg, ok := s.webhooks[wallet]
	return cfg, ok
}

// Send sends a signed webhook to the given wallet config with the provided data.
func (s *Service) Send(cfg WalletConfig, data WebhookData) {
	if err := s.send(cfg, data); err != nil {
		log.Errorf("failed to send wallet webhook to '%s': %v", cfg.URL, err)
	}
}

// PublicKeyPEM returns the PEM-encoded public key for signature verification.
func (s *Service) PublicKeyPEM() ([]byte, error) {
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

func (s *Service) send(cfg WalletConfig, data WebhookData) error {
	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal webhook data: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if cfg.Auth != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Auth)
	}

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
