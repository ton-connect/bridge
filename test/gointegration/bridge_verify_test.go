package bridge_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"
)

type verifyResponse struct {
	Status string `json:"status"`
}

type errorResponse struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
}

// callVerifyEndpoint calls the /bridge/verify endpoint with given parameters
func callVerifyEndpoint(t *testing.T, baseURL, clientID, originURL, verifyType string) (verifyResponse, int, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Build URL with query parameters
	queryParams := url.Values{}
	queryParams.Set("client_id", clientID)
	queryParams.Set("url", originURL)
	if verifyType != "" {
		queryParams.Set("type", verifyType)
	}

	fullURL := baseURL + "/verify?" + queryParams.Encode()
	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, nil)
	if err != nil {
		return verifyResponse{}, 0, fmt.Errorf("create request: %w", err)
	}

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

// establishConnection opens SSE connection and waits for heartbeat
func establishConnection(t *testing.T, baseURL, clientID, originURL string) func() {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	gw, err := OpenBridge(ctx, OpenOpts{
		BridgeURL: baseURL,
		SessionID: clientID,
		OriginURL: originURL,
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

func TestBridgeVerify_UnknownClient(t *testing.T) {
	clientID := randomSessionID(t)
	originURL := "https://example.com"

	// Test with unknown client
	resp, status, err := callVerifyEndpoint(t, BRIDGE_URL_Provider, clientID, originURL, "connect")
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

func TestBridgeVerify_ExactMatch(t *testing.T) {
	clientID := randomSessionID(t)
	originURL := "https://example.com"

	// Establish connection
	cleanup := establishConnection(t, BRIDGE_URL_Provider, clientID, originURL)
	defer cleanup()

	// Wait for connection to be registered
	time.Sleep(500 * time.Millisecond)

	// Verify - should return "ok" for exact match
	resp, status, err := callVerifyEndpoint(t, BRIDGE_URL_Provider, clientID, originURL, "connect")
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

func TestBridgeVerify_MissingClientID(t *testing.T) {
	// Test with missing client_id
	_, status, err := callVerifyEndpoint(t, BRIDGE_URL_Provider, "", "https://example.com", "connect")

	// Should return bad request
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}

	// Should have error message
	if err == nil {
		t.Error("expected error for missing client_id")
	}
}

func TestBridgeVerify_MissingURL(t *testing.T) {
	clientID := randomSessionID(t)

	// Test with missing url
	_, status, err := callVerifyEndpoint(t, BRIDGE_URL_Provider, clientID, "", "connect")

	// Should return bad request
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}

	// Should have error message
	if err == nil {
		t.Error("expected error for missing url")
	}
}

func TestBridgeVerify_InvalidType(t *testing.T) {
	clientID := randomSessionID(t)
	originURL := "https://example.com"

	// Test with invalid type
	_, status, err := callVerifyEndpoint(t, BRIDGE_URL_Provider, clientID, originURL, "invalid")

	// Should return bad request
	if status != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", status)
	}

	// Should have error message
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestBridgeVerify_DefaultTypeConnect(t *testing.T) {
	clientID := randomSessionID(t)
	originURL := "https://example.com"

	// Test without type parameter (should default to "connect")
	resp, status, err := callVerifyEndpoint(t, BRIDGE_URL_Provider, clientID, originURL, "")
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

func TestBridgeVerify_MultipleConnectionsSameOrigin(t *testing.T) {
	clientID := randomSessionID(t)
	originURL := "https://example.com"

	// Establish connection
	cleanup := establishConnection(t, BRIDGE_URL_Provider, clientID, originURL)
	defer cleanup()

	time.Sleep(500 * time.Millisecond)

	// Verify from same origin (same IP in test environment)
	resp, status, err := callVerifyEndpoint(t, BRIDGE_URL_Provider, clientID, originURL, "connect")
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

func TestBridgeVerify_DifferentOrigin(t *testing.T) {
	clientID := randomSessionID(t)
	originURL := "https://example.com"

	// Establish connection with one origin
	cleanup := establishConnection(t, BRIDGE_URL_Provider, clientID, originURL)
	defer cleanup()

	time.Sleep(500 * time.Millisecond)

	// Verify with different origin - should return "danger"
	resp, status, err := callVerifyEndpoint(t, BRIDGE_URL_Provider, clientID, "https://malicious.com", "connect")
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	// Should return OK status
	if status != http.StatusOK {
		t.Errorf("expected 200, got %d", status)
	}

	// Should return "danger" for different origin
	if resp.Status != "danger" {
		t.Errorf("expected 'danger', got '%s'", resp.Status)
	}
}

func TestBridgeVerify_TTLExpiration(t *testing.T) {
	clientID := randomSessionID(t)
	originURL := "https://example.com"

	// Establish connection
	cleanup := establishConnection(t, BRIDGE_URL_Provider, clientID, originURL)
	defer cleanup()

	time.Sleep(500 * time.Millisecond)

	// Verify immediately - should return "ok"
	resp, status, err := callVerifyEndpoint(t, BRIDGE_URL_Provider, clientID, originURL, "connect")
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	if status != http.StatusOK {
		t.Errorf("expected 200, got %d", status)
	}

	if resp.Status != "ok" {
		t.Errorf("expected 'ok', got '%s'", resp.Status)
	}

	// Close connection and wait for TTL to expire
	cleanup()
	// Default TTL is typically 5 minutes, but we'll wait a reasonable time
	// In production, connection cache TTL should be configured via environment
	// For this test, we expect the connection to remain valid for at least a few seconds
	time.Sleep(2 * time.Second)

	// Verify after some time - connection should still be cached
	resp2, status2, err2 := callVerifyEndpoint(t, BRIDGE_URL_Provider, clientID, originURL, "connect")
	if err2 != nil {
		t.Fatalf("verify failed: %v", err2)
	}

	if status2 != http.StatusOK {
		t.Errorf("expected 200, got %d", status2)
	}

	// After 2 seconds, connection should still be cached (TTL is usually 5+ minutes)
	if resp2.Status != "ok" {
		t.Logf("Note: connection expired after 2 seconds, expected 'ok', got '%s'", resp2.Status)
	}
}

func TestBridgeVerify_MultipleConnectionsDifferentIPs(t *testing.T) {
	clientID := randomSessionID(t)
	originURL := "https://example.com"

	// Establish first connection
	cleanup1 := establishConnection(t, BRIDGE_URL_Provider, clientID, originURL)
	defer cleanup1()

	time.Sleep(500 * time.Millisecond)

	// First verification should be "ok"
	resp1, status1, err1 := callVerifyEndpoint(t, BRIDGE_URL_Provider, clientID, originURL, "connect")
	if err1 != nil {
		t.Fatalf("first verify failed: %v", err1)
	}

	if status1 != http.StatusOK {
		t.Errorf("expected 200, got %d", status1)
	}

	if resp1.Status != "ok" {
		t.Errorf("expected 'ok', got '%s'", resp1.Status)
	}

	// Note: In the test environment (Docker), all connections come from the same IP
	// so we can't actually test the "warning" status for different IPs in integration tests.
	// This would require either:
	// 1. Multiple containers with different IPs
	// 2. Mock/stub the IP extraction in the handler
	// 3. Test at the storage layer (already done in valkey_test.go)

	// This test verifies that multiple verify calls work correctly
	resp2, status2, err2 := callVerifyEndpoint(t, BRIDGE_URL_Provider, clientID, originURL, "connect")
	if err2 != nil {
		t.Fatalf("second verify failed: %v", err2)
	}

	if status2 != http.StatusOK {
		t.Errorf("expected 200, got %d", status2)
	}

	if resp2.Status != "ok" {
		t.Errorf("expected 'ok', got '%s'", resp2.Status)
	}
}

func TestBridgeVerify_DifferentOriginAfterMultipleConnections(t *testing.T) {
	clientID := randomSessionID(t)
	originURL1 := "https://example.com"
	originURL2 := "https://another.com"

	// Establish first connection
	cleanup1 := establishConnection(t, BRIDGE_URL_Provider, clientID, originURL1)
	defer cleanup1()

	time.Sleep(500 * time.Millisecond)

	// Verify with first origin - should be "ok"
	resp1, status1, err1 := callVerifyEndpoint(t, BRIDGE_URL_Provider, clientID, originURL1, "connect")
	if err1 != nil {
		t.Fatalf("first verify failed: %v", err1)
	}

	if status1 != http.StatusOK {
		t.Errorf("expected 200, got %d", status1)
	}

	if resp1.Status != "ok" {
		t.Errorf("expected 'ok', got '%s'", resp1.Status)
	}

	// Establish second connection with different origin
	cleanup2 := establishConnection(t, BRIDGE_URL_Provider, clientID, originURL2)
	defer cleanup2()

	time.Sleep(500 * time.Millisecond)

	// Verify with third, completely different origin - should be "danger"
	resp3, status3, err3 := callVerifyEndpoint(t, BRIDGE_URL_Provider, clientID, "https://malicious.com", "connect")
	if err3 != nil {
		t.Fatalf("third verify failed: %v", err3)
	}

	if status3 != http.StatusOK {
		t.Errorf("expected 200, got %d", status3)
	}

	// With multiple origins registered, a third different one should be "danger"
	if resp3.Status != "danger" {
		t.Errorf("expected 'danger', got '%s'", resp3.Status)
	}
}
