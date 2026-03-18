package handlerv1

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/ton-connect/bridge/internal/utils"
	"github.com/ton-connect/bridge/internal/v1/storage"
	"github.com/ton-connect/bridge/internal/webhook"
)

const (
	defaultClientID = "a3f9c8e21d7b4a5e9c0f6b1d8e72c4fa9b0e1d5c7a6f84b2e93d0c1a5f7e8b42"
	defaultToID     = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func TestHandler(t *testing.T) {
	defaultBody := "test message payload"

	tCases := map[string]struct {
		expectedStatus int
		expectedBody   []string
		rqParams       map[string]string
		body           string
	}{
		"ok path": {
			expectedStatus: http.StatusOK,
			expectedBody:   []string{`"message":"OK"`, `"statusCode":200`},
			rqParams: map[string]string{
				"client_id":         defaultClientID,
				"to":                defaultToID,
				"ttl":               "60",
				"no_request_source": "true",
			},
			body: defaultBody,
		},
		"missing client_id": {
			expectedStatus: http.StatusBadRequest,
			expectedBody:   []string{`"message":"param \"client_id\" not present"`},
			rqParams: map[string]string{
				"to":  defaultToID,
				"ttl": "60",
			},
			body: defaultBody,
		},
		"missing to": {
			expectedStatus: http.StatusBadRequest,
			expectedBody:   []string{`"message":"param \"to\" not present"`},
			rqParams: map[string]string{
				"client_id": defaultClientID,
				"ttl":       "60",
			},
			body: defaultBody,
		},
		"missing ttl": {
			expectedStatus: http.StatusBadRequest,
			expectedBody:   []string{`"message":"param \"ttl\" not present"`},
			rqParams: map[string]string{
				"client_id": defaultClientID,
				"to":        defaultToID,
			},
			body: defaultBody,
		},
		"ttl too high": {
			expectedStatus: http.StatusBadRequest,
			expectedBody:   []string{`"message":"param \"ttl\" too high"`},
			rqParams: map[string]string{
				"client_id": defaultClientID,
				"to":        defaultToID,
				"ttl":       "500",
			},
			body: defaultBody,
		},
		"large toID": {
			expectedStatus: http.StatusBadRequest,
			expectedBody:   []string{utils.ErrInvalidPublicAddressLength.Error()},
			rqParams: map[string]string{
				"client_id":         defaultClientID,
				"to":                strings.Repeat("a", 2048*100),
				"ttl":               "60",
				"no_request_source": "true",
			},
			body: defaultBody,
		},
		"large clientID": {
			expectedStatus: http.StatusBadRequest,
			expectedBody:   []string{utils.ErrInvalidPublicAddressLength.Error()},
			rqParams: map[string]string{
				"client_id":         strings.Repeat("a", 2048*100),
				"to":                defaultToID,
				"ttl":               "60",
				"no_request_source": "true",
			},
			body: defaultBody,
		},
		"invalid clientID, length": {
			expectedStatus: http.StatusBadRequest,
			expectedBody:   []string{"failed to parse the", "client_id", utils.ErrInvalidPublicAddressLength.Error()},
			rqParams: map[string]string{
				"client_id":         defaultClientID[1:],
				"to":                defaultToID,
				"ttl":               "60",
				"no_request_source": "true",
			},
			body: defaultBody,
		},
		"invalid toID, length": {
			expectedStatus: http.StatusBadRequest,
			expectedBody:   []string{"failed to parse the", "to", utils.ErrInvalidPublicAddressLength.Error()},
			rqParams: map[string]string{
				"client_id":         defaultClientID,
				"to":                defaultToID[1:],
				"ttl":               "60",
				"no_request_source": "true",
			},
			body: defaultBody,
		},
		"invalid clientID, format": {
			expectedStatus: http.StatusBadRequest,
			expectedBody:   []string{"failed to parse the", "client_id", utils.ErrInvalidPublicAddressFormat.Error()},
			rqParams: map[string]string{
				"client_id":         "t" + defaultClientID[1:],
				"to":                defaultToID,
				"ttl":               "60",
				"no_request_source": "true",
			},
			body: defaultBody,
		},
		"invalid toID, format": {
			expectedStatus: http.StatusBadRequest,
			expectedBody:   []string{"failed to parse the", "to", utils.ErrInvalidPublicAddressFormat.Error()},
			rqParams: map[string]string{
				"client_id":         defaultClientID,
				"to":                "t" + defaultToID[1:],
				"ttl":               "60",
				"no_request_source": "true",
			},
			body: defaultBody,
		},
	}
	for name, tc := range tCases {
		t.Run(name, func(t *testing.T) {
			e := echo.New()

			values := url.Values{}
			for key, value := range tc.rqParams {
				values.Set(key, value)
			}

			reqURL := "/bridge/message"
			if len(values) > 0 {
				reqURL += "?" + values.Encode()
			}

			req := httptest.NewRequest(http.MethodPost, reqURL, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/octet-stream")

			memStorage := storage.NewMemStorage(nil, nil)
			extractor, err := utils.NewRealIPExtractor([]string{})
			if err != nil {
				t.Fatalf("failed to create RealIPExtractor: %v", err)
			}

			h := NewHandler(memStorage, 10*time.Second, extractor, nil, nil, nil)

			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			if err := h.SendMessageHandler(c); err != nil {
				t.Fatalf("SendMessageHandler returned error: %v", err)
			}

			if rec.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, rec.Code)
			}

			for _, expectedBody := range tc.expectedBody {
				if !strings.Contains(rec.Body.String(), expectedBody) {
					t.Errorf("expected body to contain %q, got %q", expectedBody, rec.Body.String())
				}
			}
		})
	}
}

func TestHandler_WalletWebhook(t *testing.T) {
	e := echo.New()

	mock := webhook.NewMock(nil)
	defer mock.Close()

	walletWebhookSvc, err := webhook.NewService(
		fmt.Sprintf(`{"testwallet":{"url":"%s"}}`, mock.URL()),
		"",
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	values := url.Values{}
	values.Set("client_id", defaultClientID)
	values.Set("to", defaultToID)
	values.Set("ttl", "60")
	values.Set("wallet", "testwallet")
	values.Set("topic", "sendTransaction")
	values.Set("no_request_source", "true")

	req := httptest.NewRequest(http.MethodPost, "/bridge/message?"+values.Encode(), strings.NewReader("payload"))
	req.Header.Set("Content-Type", "application/octet-stream")

	memStorage := storage.NewMemStorage(nil, nil)
	extractor, err := utils.NewRealIPExtractor([]string{})
	if err != nil {
		t.Fatalf("failed to create RealIPExtractor: %v", err)
	}

	h := NewHandler(memStorage, 10*time.Second, extractor, nil, nil, walletWebhookSvc)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := h.SendMessageHandler(c); err != nil {
		t.Fatalf("SendMessageHandler returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		records := mock.Records()
		if len(records) == 1 {
			if records[0].Payload.Topic != "sendTransaction" {
				t.Fatalf("topic: got %q, want %q", records[0].Payload.Topic, "sendTransaction")
			}
			if records[0].Payload.Hash != "payload" {
				t.Fatalf("hash: got %q, want %q", records[0].Payload.Hash, "payload")
			}
			if records[0].Path != "/"+defaultClientID {
				t.Fatalf("path: got %q, want %q", records[0].Path, "/"+defaultClientID)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for wallet webhook")
}

func TestHandler_WalletWebhookWithoutTopic(t *testing.T) {
	e := echo.New()

	mock := webhook.NewMock(nil)
	defer mock.Close()

	walletWebhookSvc, err := webhook.NewService(
		fmt.Sprintf(`{"testwallet":{"url":"%s"}}`, mock.URL()),
		"",
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	values := url.Values{}
	values.Set("client_id", defaultClientID)
	values.Set("to", defaultToID)
	values.Set("ttl", "60")
	values.Set("wallet", "testwallet")
	values.Set("no_request_source", "true")

	req := httptest.NewRequest(http.MethodPost, "/bridge/message?"+values.Encode(), strings.NewReader("payload"))
	req.Header.Set("Content-Type", "application/octet-stream")

	memStorage := storage.NewMemStorage(nil, nil)
	extractor, err := utils.NewRealIPExtractor([]string{})
	if err != nil {
		t.Fatalf("failed to create RealIPExtractor: %v", err)
	}

	h := NewHandler(memStorage, 10*time.Second, extractor, nil, nil, walletWebhookSvc)

	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := h.SendMessageHandler(c); err != nil {
		t.Fatalf("SendMessageHandler returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		records := mock.Records()
		if len(records) == 1 {
			if records[0].Payload.Topic != "" {
				t.Fatalf("topic: got %q, want empty string", records[0].Payload.Topic)
			}
			if records[0].Payload.Hash != "payload" {
				t.Fatalf("hash: got %q, want %q", records[0].Payload.Hash, "payload")
			}
			if records[0].Path != "/"+defaultClientID {
				t.Fatalf("path: got %q, want %q", records[0].Path, "/"+defaultClientID)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for wallet webhook")
}
