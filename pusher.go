package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/config"
	"net/http"
)

type WebhookData struct {
	Topic string `json:"topic"`
	Hash  string `json:"hash"`
}

func SendWebhook(clientID string, body WebhookData) error {
	if config.Config.WebhookURL == "" {
		return nil
	}

	postBody, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, config.Config.WebhookURL+"/"+clientID, bytes.NewReader(postBody))
	if err != nil {
		log.Errorf("failed to init request: %v", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Errorf("failed send request: %v", err)
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		err = fmt.Errorf("bad status code: %v", res.StatusCode)
		return err
	}
	return nil
}
