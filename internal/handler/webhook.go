package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

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
				slog.Error("failed to trigger webhook", "webhook", webhook, "err", err)
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
			slog.Error("failed to close response body", "err", closeErr)
		}
	}()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code: %v", res.StatusCode)
	}
	return nil
}
