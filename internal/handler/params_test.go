package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

const maxBodySize = 1048576 // 1 MB

func TestParamsStorage_URLParameters(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/test?client_id=test123&heartbeat=message", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	params, err := NewParamsStorage(c, maxBodySize)
	if err != nil {
		t.Error(err)
		return
	}

	clientID, ok := params.Get("client_id")
	if !ok {
		t.Error("Expected to find client_id parameter")
	}
	if clientID != "test123" {
		t.Errorf("Expected client_id=test123, got %s", clientID)
	}

	heartbeat, ok := params.Get("heartbeat")
	if !ok {
		t.Error("Expected to find heartbeat parameter")
	}
	if heartbeat != "message" {
		t.Errorf("Expected heartbeat=message, got %s", heartbeat)
	}
}

func TestParamsStorage_JSONBodyParameters(t *testing.T) {
	e := echo.New()

	jsonBody := `{"client_id": "test456", "to": "test123", "ttl": "300"}`
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	params, err := NewParamsStorage(c, maxBodySize)
	if err != nil {
		t.Error(err)
		return
	}

	clientID, ok := params.Get("client_id")
	if !ok {
		t.Error("Expected to find client_id parameter")
	}
	if clientID != "test456" {
		t.Errorf("Expected client_id=test456, got %s", clientID)
	}

	to, ok := params.Get("to")
	if !ok {
		t.Error("Expected to find to parameter")
	}
	if to != "test123" {
		t.Errorf("Expected to=test123, got %s", to)
	}
}

func TestParamsStorage_JSONBodyParametersWithWrongContentType(t *testing.T) {
	e := echo.New()

	jsonBody := `{"client_id": "test456", "to": "test123", "ttl": "300"}`
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "text/event-stream")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	params, err := NewParamsStorage(c, maxBodySize)
	if err != nil {
		t.Error(err)
		return
	}

	_, ok := params.Get("client_id")
	if ok {
		t.Error("Expected not to find client_id parameter")
	}
}

func TestParamsStorage_NonJSONBody(t *testing.T) {
	e := echo.New()

	body := "This is just a regular message, not JSON"
	req := httptest.NewRequest(http.MethodPost, "/test?client_id=test123", strings.NewReader(body))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	params, err := NewParamsStorage(c, maxBodySize)
	if err != nil {
		t.Error(err)
		return
	}

	clientID, ok := params.Get("client_id")
	if !ok {
		t.Error("Expected to find client_id parameter")
	}
	if clientID != "test123" {
		t.Errorf("Expected client_id=test123, got %s", clientID)
	}
}

func TestParamsStorage_JSONBodyIsTooLarge(t *testing.T) {
	e := echo.New()

	jsonBody := `{"client_id": "test456", "to": "test123", "ttl": "300"}`
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	_, err := NewParamsStorage(c, 10)
	if err == nil {
		t.Error("Expected error when body is too large")
	} else if !strings.Contains(err.Error(), "body too large") {
		t.Errorf("Expected 'body too large' error, got: %s", err.Error())
	}
}
