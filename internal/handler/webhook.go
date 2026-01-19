package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/ton-connect/bridge/internal/config"
)

type WebhookData struct {
	Topic string `json:"topic"`
	Hash  string `json:"hash"`
}

func SendWebhook(clientID string, body WebhookData) {
	if config.Config.WebhookURL == "" {
		return
	}
	webhooks := strings.Split(config.Config.WebhookURL, ",")
	for _, webhook := range webhooks {
		go func(webhook string) {
			err := sendWebhook(clientID, body, webhook)
			if err != nil {
				log.Errorf("failed to trigger webhook '%s': %v", webhook, err)
			}
		}(webhook)
	}
}

func sendWebhook(clientID string, body WebhookData, webhook string) error {
	postBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal body: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, webhook+"/"+clientID, bytes.NewReader(postBody))
	if err != nil {
		return fmt.Errorf("failed to init request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed send request: %w", err)
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
