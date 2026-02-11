package objectstorage

import (
	"encoding/json"
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
	body := `{"object":"dGVzdCBvYmplY3Q="}`
	req := httptest.NewRequest(http.MethodPost, "/store?ttl=60", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler.StoreHandler(c); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Extract ID from get_url
	var resp storeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.GetURL == "" {
		t.Fatal("response should contain get_url")
	}

	parts := strings.Split(resp.GetURL, "/store/")
	if len(parts) < 2 {
		t.Fatalf("unexpected get_url format: %s", resp.GetURL)
	}
	id := parts[len(parts)-1]

	// Retrieve the object
	req2 := httptest.NewRequest(http.MethodGet, "/store/"+id, nil)
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
	if !strings.Contains(rec2.Body.String(), "dGVzdCBvYmplY3Q=") {
		t.Fatalf("response should contain the stored object: %s", rec2.Body.String())
	}
}

func TestStoreMissingTTL(t *testing.T) {
	handler, e := setupTestHandler()

	body := `{"object":"dGVzdA=="}`
	req := httptest.NewRequest(http.MethodPost, "/store", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
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

	body := `{"object":"dGVzdA=="}`
	req := httptest.NewRequest(http.MethodPost, "/store?ttl=9999", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler.StoreHandler(c); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "max_ttl") {
		t.Fatalf("response should contain max_ttl: %s", rec.Body.String())
	}
}

func TestStoreInvalidTTL(t *testing.T) {
	handler, e := setupTestHandler()

	body := `{"object":"dGVzdA=="}`
	req := httptest.NewRequest(http.MethodPost, "/store?ttl=abc", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
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

	body := `{"object":"dGVzdA=="}`
	req := httptest.NewRequest(http.MethodPost, "/store?ttl=-1", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler.StoreHandler(c); err != nil {
		t.Fatalf("StoreHandler returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestStoreMissingObject(t *testing.T) {
	handler, e := setupTestHandler()

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/store?ttl=60", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
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
	body := `{"object":"` + largeObject + `"}`
	req := httptest.NewRequest(http.MethodPost, "/store?ttl=60", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
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
	body := `{"object":"` + exactObject + `"}`
	req := httptest.NewRequest(http.MethodPost, "/store?ttl=60", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
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

	req := httptest.NewRequest(http.MethodGet, "/store/nonexistent", nil)
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

func TestBuildGetURLWithBaseURL(t *testing.T) {
	storage := NewMemObjectStorage()
	handler := NewHandler(storage, 300, testMaxSize, "https://bridge.example.com")
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	url := handler.buildGetURL(c, "abc123")
	expected := "https://bridge.example.com/store/abc123"
	if url != expected {
		t.Fatalf("expected %s, got %s", expected, url)
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

	url := handler.buildGetURL(c, "abc123")
	expected := "http://localhost:8081/store/abc123"
	if url != expected {
		t.Fatalf("expected %s, got %s", expected, url)
	}
}
