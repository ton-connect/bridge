package bridge_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// hashObject mirrors the server-side content-addressable hash (SHA-256 of object + content type).
// Frontend uses the same algorithm to predict the object ID without waiting for the backend response.
func hashObject(object []byte, contentType string) string {
	h := sha256.New()
	h.Write(object)
	h.Write([]byte(contentType))
	return hex.EncodeToString(h.Sum(nil))
}

func objectsURL() string {
	return strings.TrimSuffix(BRIDGE_URL, "/bridge") + "/objects"
}

func storeObjectHTTP(t *testing.T, ctx context.Context, body []byte, contentType string, ttl int) (*http.Response, string) {
	t.Helper()
	u := fmt.Sprintf("%s?ttl=%d", objectsURL(), ttl)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /objects: %v", err)
	}
	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp, string(respBody)
}

func getObjectHTTP(t *testing.T, ctx context.Context, id string) (*http.Response, []byte) {
	t.Helper()
	u := fmt.Sprintf("%s/%s", objectsURL(), id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /objects/%s: %v", id, err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	return resp, body
}

// ===== Tests =====

func TestObjectStorage_StoreAndRetrieve(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testHTTPTimeout)
	defer cancel()

	body := []byte("hello object storage")
	resp, respBody := storeObjectHTTP(t, ctx, body, "text/plain", 60)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, respBody)
	}
	if !strings.Contains(respBody, "/objects/") {
		t.Fatalf("expected URL with /objects/, got %s", respBody)
	}

	// Extract ID from returned URL
	parts := strings.Split(respBody, "/objects/")
	id := parts[len(parts)-1]

	getResp, getBody := getObjectHTTP(t, ctx, id)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}
	if !bytes.Equal(getBody, body) {
		t.Fatalf("expected %q, got %q", body, getBody)
	}
	ct := getResp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected Content-Type text/plain, got %s", ct)
	}
}

func TestObjectStorage_ContentAddressableID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testHTTPTimeout)
	defer cancel()

	body := []byte("deterministic content")
	contentType := "text/plain"

	// Store twice, expect same URL
	resp1, url1 := storeObjectHTTP(t, ctx, body, contentType, 60)
	if resp1.StatusCode != http.StatusOK {
		t.Fatalf("store 1: expected 200, got %d", resp1.StatusCode)
	}

	resp2, url2 := storeObjectHTTP(t, ctx, body, contentType, 60)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("store 2: expected 200, got %d", resp2.StatusCode)
	}

	if url1 != url2 {
		t.Fatalf("same content should produce same URL, got %s and %s", url1, url2)
	}

	// Verify the ID matches client-side hash computation
	expectedID := hashObject(body, contentType)
	if !strings.HasSuffix(url1, expectedID) {
		t.Fatalf("URL should end with hash %s, got %s", expectedID, url1)
	}
}

func TestObjectStorage_DifferentContentDifferentIDs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testHTTPTimeout)
	defer cancel()

	_, url1 := storeObjectHTTP(t, ctx, []byte("content A"), "text/plain", 60)
	_, url2 := storeObjectHTTP(t, ctx, []byte("content B"), "text/plain", 60)

	if url1 == url2 {
		t.Fatal("different content should produce different URLs")
	}
}

func TestObjectStorage_DifferentContentTypeDifferentIDs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testHTTPTimeout)
	defer cancel()

	body := []byte("same body")
	_, url1 := storeObjectHTTP(t, ctx, body, "text/plain", 60)
	_, url2 := storeObjectHTTP(t, ctx, body, "application/json", 60)

	if url1 == url2 {
		t.Fatal("same body with different content type should produce different URLs")
	}
}

func TestObjectStorage_ContentTypePreserved(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testHTTPTimeout)
	defer cancel()

	tests := []struct {
		name        string
		body        []byte
		contentType string
	}{
		{"text/plain", []byte("hello"), "text/plain"},
		{"application/json", []byte(`{"key":"value"}`), "application/json"},
		{"application/xml", []byte("<root/>"), "application/xml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, respBody := storeObjectHTTP(t, ctx, tt.body, tt.contentType, 60)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("store: expected 200, got %d: %s", resp.StatusCode, respBody)
			}
			parts := strings.Split(respBody, "/objects/")
			id := parts[len(parts)-1]

			getResp, getBody := getObjectHTTP(t, ctx, id)
			if getResp.StatusCode != http.StatusOK {
				t.Fatalf("get: expected 200, got %d", getResp.StatusCode)
			}
			if !bytes.Equal(getBody, tt.body) {
				t.Fatalf("body mismatch: expected %q, got %q", tt.body, getBody)
			}
			ct := getResp.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, tt.contentType) {
				t.Fatalf("expected Content-Type %s, got %s", tt.contentType, ct)
			}
		})
	}
}

func TestObjectStorage_DefaultContentType(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testHTTPTimeout)
	defer cancel()

	// POST without Content-Type header
	resp, respBody := storeObjectHTTP(t, ctx, []byte("no content type"), "", 60)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, respBody)
	}

	parts := strings.Split(respBody, "/objects/")
	id := parts[len(parts)-1]

	getResp, _ := getObjectHTTP(t, ctx, id)
	ct := getResp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("expected default Content-Type text/plain, got %s", ct)
	}
}

func TestObjectStorage_TTLExpiration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testHTTPTimeout)
	defer cancel()

	resp, respBody := storeObjectHTTP(t, ctx, []byte("expiring soon"), "text/plain", 1)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("store: expected 200, got %d: %s", resp.StatusCode, respBody)
	}
	parts := strings.Split(respBody, "/objects/")
	id := parts[len(parts)-1]

	// Should exist immediately
	getResp, _ := getObjectHTTP(t, ctx, id)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 immediately, got %d", getResp.StatusCode)
	}

	// Wait for expiration
	time.Sleep(2 * time.Second)

	getResp2, _ := getObjectHTTP(t, ctx, id)
	if getResp2.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after expiration, got %d", getResp2.StatusCode)
	}
}

func TestObjectStorage_GetNonExistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testHTTPTimeout)
	defer cancel()

	getResp, _ := getObjectHTTP(t, ctx, "0000000000000000000000000000000000000000000000000000000000000000")
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", getResp.StatusCode)
	}
}

func TestObjectStorage_MissingTTL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testHTTPTimeout)
	defer cancel()

	u := objectsURL()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader("test"))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestObjectStorage_InvalidTTL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testHTTPTimeout)
	defer cancel()

	tests := []struct {
		name string
		ttl  string
	}{
		{"negative", "-1"},
		{"zero", "0"},
		{"non-numeric", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := fmt.Sprintf("%s?ttl=%s", objectsURL(), tt.ttl)
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader("test"))
			req.Header.Set("Content-Type", "text/plain")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", resp.StatusCode)
			}
		})
	}
}

func TestObjectStorage_EmptyBody(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testHTTPTimeout)
	defer cancel()

	resp, respBody := storeObjectHTTP(t, ctx, []byte(""), "text/plain", 60)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, respBody)
	}
}

func TestObjectStorage_UnsupportedContentType(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testHTTPTimeout)
	defer cancel()

	resp, respBody := storeObjectHTTP(t, ctx, []byte("hello"), "text/html", 60)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, respBody)
	}
	if !strings.Contains(respBody, "unsupported Content-Type") {
		t.Fatalf("expected unsupported Content-Type message, got %q", respBody)
	}
}

func TestObjectStorage_LargeObject(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testHTTPTimeout)
	defer cancel()

	// Store a 50KB object
	large := bytes.Repeat([]byte("x"), 50*1024)
	resp, respBody := storeObjectHTTP(t, ctx, large, "text/plain", 60)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("store: expected 200, got %d: %s", resp.StatusCode, respBody)
	}
	parts := strings.Split(respBody, "/objects/")
	id := parts[len(parts)-1]

	getResp, getBody := getObjectHTTP(t, ctx, id)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", getResp.StatusCode)
	}
	if !bytes.Equal(getBody, large) {
		t.Fatalf("retrieved size %d does not match stored size %d", len(getBody), len(large))
	}
}

func TestObjectStorage_ClientSideHashPrediction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testHTTPTimeout)
	defer cancel()

	body := []byte(`{"wallet":"EQ...","amount":"1000000000"}`)
	contentType := "application/json"

	// Client predicts the ID before storing
	predictedID := hashObject(body, contentType)

	// Store the object
	resp, respBody := storeObjectHTTP(t, ctx, body, contentType, 60)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("store: expected 200, got %d: %s", resp.StatusCode, respBody)
	}

	// Verify the predicted ID matches
	if !strings.HasSuffix(respBody, predictedID) {
		t.Fatalf("predicted ID %s doesn't match returned URL %s", predictedID, respBody)
	}

	// Client can retrieve using the predicted ID directly
	getResp, getBody := getObjectHTTP(t, ctx, predictedID)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get by predicted ID: expected 200, got %d", getResp.StatusCode)
	}
	if !bytes.Equal(getBody, body) {
		t.Fatalf("body mismatch")
	}
}
