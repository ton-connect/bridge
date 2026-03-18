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
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

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

// Options configures the webhook service.
type Options struct {
	InlineConfigJSON string
	ConfigSource     string
	RefreshInterval  time.Duration
	PrivateKeyPath   string
}

// Service delivers signed webhook notifications to wallet endpoints.
// The webhook config map can be built from inline JSON and an optional file/URL source.
type Service struct {
	privateKey *rsa.PrivateKey

	mu             sync.RWMutex
	webhooks       map[string]WalletConfig // app_name → config
	inlineWebhooks map[string]WalletConfig
	configSource   string
	httpClient     *http.Client

	refreshInterval time.Duration
	stopCh          chan struct{}
	doneCh          chan struct{}
	closeOnce       sync.Once
}

// NewService creates the service, loads or generates the RSA private key,
// and parses the inline webhook config JSON.
func NewService(webhookConfigJSON string, privateKeyPath string) (*Service, error) {
	return NewServiceWithOptions(Options{
		InlineConfigJSON: webhookConfigJSON,
		PrivateKeyPath:   privateKeyPath,
	})
}

// NewServiceWithOptions creates the service from inline config plus an optional file/URL source.
func NewServiceWithOptions(opts Options) (*Service, error) {
	s := &Service{
		webhooks:       make(map[string]WalletConfig),
		inlineWebhooks: make(map[string]WalletConfig),
		configSource:   opts.ConfigSource,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		refreshInterval: opts.RefreshInterval,
	}

	if opts.PrivateKeyPath != "" {
		key, err := loadPrivateKey(opts.PrivateKeyPath)
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

	inlineWebhooks, err := parseWebhookConfigJSON(opts.InlineConfigJSON)
	if err != nil {
		return nil, fmt.Errorf("parse WEBHOOK_CONFIG: %w", err)
	}
	s.inlineWebhooks = copyWebhookConfigMap(inlineWebhooks)
	s.webhooks = copyWebhookConfigMap(inlineWebhooks)

	if opts.ConfigSource != "" {
		if opts.RefreshInterval <= 0 {
			return nil, fmt.Errorf("WEBHOOK_CONFIG_REFRESH_INTERVAL must be greater than 0 when WEBHOOK_CONFIG_SOURCE is set")
		}

		sourceWebhooks, err := s.loadSourceWebhooks()
		if err != nil {
			return nil, fmt.Errorf("load WEBHOOK_CONFIG_SOURCE: %w", err)
		}
		s.webhooks = mergeWebhookConfigs(s.inlineWebhooks, sourceWebhooks)
		s.startRefreshLoop()
	}

	log.WithFields(log.Fields{
		"count":          len(s.webhooks),
		"inline_count":   len(s.inlineWebhooks),
		"has_source":     opts.ConfigSource != "",
		"refresh_period": opts.RefreshInterval,
	}).Info("Wallet webhooks configured")
	return s, nil
}

// GetWalletConfig returns the webhook configuration for a given wallet app_name.
func (s *Service) GetWalletConfig(wallet string) (WalletConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg, ok := s.webhooks[wallet]
	return cfg, ok
}

// Close stops the background refresh loop, if one is running.
func (s *Service) Close() {
	s.closeOnce.Do(func() {
		if s.stopCh == nil {
			return
		}
		close(s.stopCh)
		<-s.doneCh
	})
}

// Send sends a signed webhook to the given wallet config with the provided data.
// The destination URL is the configured webhook URL with "/<clientID>" appended.
func (s *Service) Send(cfg WalletConfig, clientID string, data WebhookData) {
	if err := s.send(cfg, clientID, data); err != nil {
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

func (s *Service) send(cfg WalletConfig, clientID string, data WebhookData) error {
	body, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal webhook data: %w", err)
	}

	webhookURL, err := buildWebhookURL(cfg.URL, clientID)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(body))
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

func buildWebhookURL(baseURL, clientID string) (string, error) {
	if clientID == "" {
		return baseURL, nil
	}

	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse webhook URL: %w", err)
	}

	u.Path = strings.TrimRight(u.Path, "/") + "/" + clientID
	return u.String(), nil
}

func (s *Service) startRefreshLoop() {
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})

	go func() {
		ticker := time.NewTicker(s.refreshInterval)
		defer func() {
			ticker.Stop()
			close(s.doneCh)
		}()

		for {
			select {
			case <-ticker.C:
				if err := s.refreshSourceWebhooks(); err != nil {
					log.Errorf("failed to refresh wallet webhooks from '%s': %v", s.configSource, err)
				}
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *Service) refreshSourceWebhooks() error {
	sourceWebhooks, err := s.loadSourceWebhooks()
	if err != nil {
		return err
	}

	merged := mergeWebhookConfigs(s.inlineWebhooks, sourceWebhooks)

	s.mu.Lock()
	changed := !reflect.DeepEqual(s.webhooks, merged)
	s.webhooks = merged
	count := len(s.webhooks)
	s.mu.Unlock()

	if changed {
		log.WithFields(log.Fields{
			"count":  count,
			"source": s.configSource,
		}).Info("Wallet webhooks refreshed")
	}

	return nil
}

func (s *Service) loadSourceWebhooks() (map[string]WalletConfig, error) {
	data, err := loadWebhookConfigSource(s.httpClient, s.configSource)
	if err != nil {
		return nil, err
	}

	cfg, err := parseWebhookConfigJSON(string(data))
	if err != nil {
		return nil, fmt.Errorf("parse source config: %w", err)
	}

	return cfg, nil
}

func loadWebhookConfigSource(client *http.Client, source string) ([]byte, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse source: %w", err)
	}

	switch u.Scheme {
	case "":
		data, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("read source file: %w", err)
		}
		return data, nil
	case "file":
		path := u.Path
		if u.Host != "" && u.Host != "localhost" {
			path = "//" + u.Host + u.Path
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read source file: %w", err)
		}
		return data, nil
	case "http", "https":
		req, err := http.NewRequest(http.MethodGet, source, nil)
		if err != nil {
			return nil, fmt.Errorf("create source request: %w", err)
		}
		res, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch source URL: %w", err)
		}
		defer func() {
			if closeErr := res.Body.Close(); closeErr != nil {
				log.Errorf("failed to close source response body: %v", closeErr)
			}
		}()
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("bad source status code: %v", res.StatusCode)
		}
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, fmt.Errorf("read source response: %w", err)
		}
		return body, nil
	default:
		return nil, fmt.Errorf("unsupported source scheme: %q", u.Scheme)
	}
}

func parseWebhookConfigJSON(raw string) (map[string]WalletConfig, error) {
	cfg := make(map[string]WalletConfig)
	if raw == "" {
		return cfg, nil
	}

	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func copyWebhookConfigMap(src map[string]WalletConfig) map[string]WalletConfig {
	dst := make(map[string]WalletConfig, len(src))
	for wallet, cfg := range src {
		dst[wallet] = cfg
	}
	return dst
}

func mergeWebhookConfigs(base, overlay map[string]WalletConfig) map[string]WalletConfig {
	merged := copyWebhookConfigMap(base)
	for wallet, cfg := range overlay {
		merged[wallet] = cfg
	}
	return merged
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
