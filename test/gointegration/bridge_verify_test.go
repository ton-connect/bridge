package bridge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// ===== Test config =====

var (
	// For verify tests, we use the same BRIDGE_URL as other tests
	// and test either v1 or v3 depending on what's running
	BRIDGE_VERIFY_URL = func() string {
		// Try v3 URL first
		if v := os.Getenv("BRIDGE_V3_URL"); v != "" {
			return strings.TrimRight(v, "/")
		}
		// Try v1 URL
		if v := os.Getenv("BRIDGE_V1_URL"); v != "" {
			return strings.TrimRight(v, "/")
		}
		// Use default BRIDGE_URL (same as other tests)
		if v := os.Getenv("BRIDGE_URL"); v != "" {
			return strings.TrimRight(v, "/")
		}
		return "http://localhost:8081"
	}()
)

// ===== Response types =====

type verifyResponse struct {
	Status string `json:"status"`
}

type errorResponse struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

// ===== Helper functions =====

// callVerifyEndpoint calls the /bridge/verify endpoint with given parameters
func callVerifyEndpoint(t *testing.T, baseURL, clientID, originURL, verifyType string) (verifyResponse, int, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Build request body with form data
	formData := url.Values{}
	formData.Set("client_id", clientID)
	formData.Set("url", originURL)
	if verifyType != "" {
		formData.Set("type", verifyType)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/bridge/verify",
		bytes.NewBufferString(formData.Encode()))
	if err != nil {
		return verifyResponse{}, 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return verifyResponse{}, 0, fmt.Errorf("do request: %w", err)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Logf("failed to close response body: %v", cerr)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return verifyResponse{}, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if err := json.Unmarshal(body, &errResp); err != nil {
			return verifyResponse{}, resp.StatusCode, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
		}
		return verifyResponse{}, resp.StatusCode, fmt.Errorf("status %d: %s", resp.StatusCode, errResp.Message)
	}

	var vr verifyResponse
	if err := json.Unmarshal(body, &vr); err != nil {
		return verifyResponse{}, resp.StatusCode, fmt.Errorf("unmarshal response: %w", err)
	}

	return vr, resp.StatusCode, nil
}

// establishConnection opens an SSE connection to register a client with the bridge
func establishConnection(t *testing.T, baseURL, clientID, originURL string) func() {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	// Open SSE connection to register the client
	gw, err := OpenBridge(ctx, OpenOpts{
		BridgeURL: baseURL,
		SessionID: clientID,
	})
	if err != nil {
		cancel()
		t.Fatalf("failed to establish connection: %v", err)
	}

	if !gw.IsReady() {
		cancel()
		_ = gw.Close()
		t.Fatal("gateway not ready after connection")
	}

	// Return cleanup function
	return func() {
		_ = gw.Close()
		cancel()
	}
}

// ===== Tests =====

func TestVerify_UnknownClient(t *testing.T) {
	clientID := randomSessionID(t)
	originURL := "https://example.com"

	// Test with unknown client
	resp, status, err := callVerifyEndpoint(t, BRIDGE_VERIFY_URL, clientID, originURL, "connect")
	if err != nil {
		t.Logf("verify error: %v", err)
	}

	// Should return "unknown" status for non-existent client
	if status == http.StatusOK {
		if resp.Status != "unknown" {
			t.Errorf("expected 'unknown', got '%s'", resp.Status)
		}
	}
}

func TestVerify_ExactMatch(t *testing.T) {
	clientID := randomSessionID(t)
	originURL := "https://example.com"

	// Establish connection
	cleanup := establishConnection(t, BRIDGE_VERIFY_URL, clientID, originURL)
	defer cleanup()

	// Wait for connection to be registered
	time.Sleep(500 * time.Millisecond)

	// Verify - should return "ok" for exact match
	resp, status, err := callVerifyEndpoint(t, BRIDGE_VERIFY_URL, clientID, originURL, "connect")
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	// Should return 200 OK
	if status != http.StatusOK {
		t.Errorf("expected 200, got %d", status)
	}

	// Should return "ok" status
	if resp.Status != "ok" {
		t.Errorf("expected 'ok', got '%s'", resp.Status)
	}
}

func TestVerify_MissingClientID(t *testing.T) {
	// Test with missing client_id
	_, status, err := callVerifyEndpoint(t, BRIDGE_VERIFY_URL, "", "https://example.com", "connect")

	// Should return bad request
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}

	// Should have error message
	if err == nil {
		t.Error("expected error for missing client_id")
	}
}

func TestVerify_MissingURL(t *testing.T) {
	clientID := randomSessionID(t)

	// Test with missing url
	_, status, err := callVerifyEndpoint(t, BRIDGE_VERIFY_URL, clientID, "", "connect")

	// Should return bad request
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}

	// Should have error message
	if err == nil {
		t.Error("expected error for missing url")
	}
}

func TestVerify_InvalidType(t *testing.T) {
	clientID := randomSessionID(t)
	originURL := "https://example.com"

	// Test with invalid type
	_, status, err := callVerifyEndpoint(t, BRIDGE_VERIFY_URL, clientID, originURL, "invalid")

	// Should return bad request
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}

	// Should have error message
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestVerify_DefaultTypeConnect(t *testing.T) {
	clientID := randomSessionID(t)
	originURL := "https://example.com"

	// Test without type parameter (should default to "connect")
	resp, status, err := callVerifyEndpoint(t, BRIDGE_VERIFY_URL, clientID, originURL, "")
	if err != nil && status != http.StatusOK {
		t.Fatalf("verify failed: %v", err)
	}

	// Should accept missing type and default to "connect"
	if status != http.StatusOK {
		t.Errorf("expected 200, got %d", status)
	}

	// Should return some status (likely "unknown" for non-existent client)
	if resp.Status == "" {
		t.Error("expected non-empty status")
	}
}

func TestVerify_MultipleConnectionsSameOrigin(t *testing.T) {
	clientID := randomSessionID(t)
	originURL := "https://example.com"

	// Establish connection
	cleanup := establishConnection(t, BRIDGE_VERIFY_URL, clientID, originURL)
	defer cleanup()

	time.Sleep(500 * time.Millisecond)

	// Verify from same origin (same IP in test environment)
	resp, status, err := callVerifyEndpoint(t, BRIDGE_VERIFY_URL, clientID, originURL, "connect")
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	// Should return OK status
	if status != http.StatusOK {
		t.Errorf("expected 200, got %d", status)
	}

	// Should return "ok" for exact match (same IP and origin in test)
	if resp.Status != "ok" {
		t.Errorf("expected 'ok', got '%s'", resp.Status)
	}
}
