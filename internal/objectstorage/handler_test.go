package objectstorage

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

const testMaxSize = 100 * 1024 // 100 KB

func setupTestHandler() (*Handler, *echo.Echo) {
	storage := NewMemObjectStorage()
	handler := NewHandler(storage, 300, testMaxSize, "")
	e := echo.New()
	return handler, e
}

func TestStoreAndRetrieve(t *testing.T) {
	handler, e := setupTestHandler()

	// Store an object
	req := httptest.NewRequest(http.MethodPost, "/objects?ttl=60", strings.NewReader("hello world"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler.StoreHandler(c); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Response is a plain text URL
	getURL := rec.Body.String()
	if !strings.Contains(getURL, "/objects/") {
		t.Fatalf("response should contain /objects/ path: %s", getURL)
	}

	// Extract ID from the URL
	parts := strings.Split(getURL, "/objects/")
	if len(parts) < 2 {
		t.Fatalf("unexpected URL format: %s", getURL)
	}
	id := parts[len(parts)-1]

	// Retrieve the object
	req2 := httptest.NewRequest(http.MethodGet, "/objects/"+id, nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("id")
	c2.SetParamValues(id)

	if err := handler.GetHandler(c2); err != nil {
		t.Fatalf("GetHandler returned error: %v", err)
	}
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	if rec2.Body.String() != "hello world" {
		t.Fatalf("expected 'hello world', got '%s'", rec2.Body.String())
	}
	gotCT := rec2.Header().Get("Content-Type")
	if !strings.HasPrefix(gotCT, "text/plain") {
		t.Fatalf("expected Content-Type text/plain, got %s", gotCT)
	}
}

func TestStoreDeduplication(t *testing.T) {
	handler, e := setupTestHandler()

	// Store same object twice
	req1 := httptest.NewRequest(http.MethodPost, "/objects?ttl=60", strings.NewReader("same content"))
	req1.Header.Set("Content-Type", "text/plain")
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	if err := handler.StoreHandler(c1); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	url1 := rec1.Body.String()

	req2 := httptest.NewRequest(http.MethodPost, "/objects?ttl=60", strings.NewReader("same content"))
	req2.Header.Set("Content-Type", "text/plain")
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	if err := handler.StoreHandler(c2); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	url2 := rec2.Body.String()

	if url1 != url2 {
		t.Fatalf("same content should produce same URL, got %s and %s", url1, url2)
	}
}

func TestStoreDifferentContent(t *testing.T) {
	handler, e := setupTestHandler()

	req1 := httptest.NewRequest(http.MethodPost, "/objects?ttl=60", strings.NewReader("content A"))
	req1.Header.Set("Content-Type", "text/plain")
	rec1 := httptest.NewRecorder()
	c1 := e.NewContext(req1, rec1)
	_ = handler.StoreHandler(c1)

	req2 := httptest.NewRequest(http.MethodPost, "/objects?ttl=60", strings.NewReader("content B"))
	req2.Header.Set("Content-Type", "text/plain")
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	_ = handler.StoreHandler(c2)

	if rec1.Body.String() == rec2.Body.String() {
		t.Fatal("different content should produce different URLs")
	}
}

func TestStoreMissingTTL(t *testing.T) {
	handler, e := setupTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/object", strings.NewReader("test"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler.StoreHandler(c); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestStoreTTLTooHigh(t *testing.T) {
	handler, e := setupTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/objects?ttl=9999", strings.NewReader("test"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler.StoreHandler(c); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestStoreInvalidTTL(t *testing.T) {
	handler, e := setupTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/objects?ttl=abc", strings.NewReader("test"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler.StoreHandler(c); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestStoreNegativeTTL(t *testing.T) {
	handler, e := setupTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/objects?ttl=-1", strings.NewReader("test"))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler.StoreHandler(c); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestStoreEmptyBody(t *testing.T) {
	handler, e := setupTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/objects?ttl=60", strings.NewReader(""))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler.StoreHandler(c); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestStoreObjectTooLarge(t *testing.T) {
	handler, e := setupTestHandler()

	largeObject := strings.Repeat("a", testMaxSize+1)
	req := httptest.NewRequest(http.MethodPost, "/objects?ttl=60", strings.NewReader(largeObject))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler.StoreHandler(c); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestStoreObjectExactlyAtLimit(t *testing.T) {
	handler, e := setupTestHandler()

	exactObject := strings.Repeat("a", testMaxSize)
	req := httptest.NewRequest(http.MethodPost, "/objects?ttl=60", strings.NewReader(exactObject))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler.StoreHandler(c); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetNonExistent(t *testing.T) {
	handler, e := setupTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/objects/nonexistent", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues("nonexistent")

	if err := handler.GetHandler(c); err != nil {
		t.Fatalf("GetHandler returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", rec.Code)
	}
}

func storeObject(t *testing.T, handler *Handler, e *echo.Echo, body string, contentType string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/objects?ttl=60", strings.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := handler.StoreHandler(c); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	parts := strings.Split(rec.Body.String(), "/objects/")
	return parts[len(parts)-1]
}

func TestGetReturnsStoredContentType(t *testing.T) {
	handler, e := setupTestHandler()

	tests := []struct {
		name        string
		body        string
		contentType string
	}{
		{"text/plain", "hello world", "text/plain"},
		{"application/json", `{"key":"value"}`, "application/json"},
		{"application/xml", "<root/>", "application/xml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := storeObject(t, handler, e, tt.body, tt.contentType)

			req := httptest.NewRequest(http.MethodGet, "/objects/"+id, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues(id)

			if err := handler.GetHandler(c); err != nil {
				t.Fatalf("GetHandler returned error: %v", err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
			}
			gotCT := rec.Header().Get("Content-Type")
			if !strings.HasPrefix(gotCT, tt.contentType) {
				t.Fatalf("expected Content-Type %s, got %s", tt.contentType, gotCT)
			}
			if rec.Body.String() != tt.body {
				t.Fatalf("expected body %q, got %q", tt.body, rec.Body.String())
			}
		})
	}
}

func TestStoreUnsupportedContentType(t *testing.T) {
	handler, e := setupTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/objects?ttl=60", strings.NewReader("hello"))
	req.Header.Set("Content-Type", "text/html")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler.StoreHandler(c); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported Content-Type") {
		t.Fatalf("expected unsupported Content-Type message, got %q", rec.Body.String())
	}
}

func TestStoreDefaultContentType(t *testing.T) {
	handler, e := setupTestHandler()

	// POST without Content-Type header should default to text/plain
	req := httptest.NewRequest(http.MethodPost, "/objects?ttl=60", strings.NewReader("hello"))
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler.StoreHandler(c); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Retrieve and verify content type defaults to text/plain
	parts := strings.Split(rec.Body.String(), "/objects/")
	id := parts[len(parts)-1]

	req2 := httptest.NewRequest(http.MethodGet, "/objects/"+id, nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetParamNames("id")
	c2.SetParamValues(id)

	if err := handler.GetHandler(c2); err != nil {
		t.Fatalf("GetHandler returned error: %v", err)
	}
	gotCT := rec2.Header().Get("Content-Type")
	if !strings.HasPrefix(gotCT, "text/plain") {
		t.Fatalf("expected Content-Type text/plain, got %s", gotCT)
	}
}

func TestBuildGetURLWithBaseURL(t *testing.T) {
	storage := NewMemObjectStorage()
	handler := NewHandler(storage, 300, testMaxSize, "https://bridge.example.com")
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	u := handler.buildGetURL(c, "abc123")
	expected := "https://bridge.example.com/objects/abc123"
	if u != expected {
		t.Fatalf("expected %s, got %s", expected, u)
	}
}

func TestBuildGetURLFromRequest(t *testing.T) {
	storage := NewMemObjectStorage()
	handler := NewHandler(storage, 300, testMaxSize, "")
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "localhost:8081"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	u := handler.buildGetURL(c, "abc123")
	expected := "http://localhost:8081/objects/abc123"
	if u != expected {
		t.Fatalf("expected %s, got %s", expected, u)
	}
}
