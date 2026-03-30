package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	bridge_middleware "github.com/ton-connect/bridge/internal/middleware"
)

func TestRecipientRateLimitMiddleware_AllowsFirstRequest(t *testing.T) {
	e := echo.New()
	limiter := bridge_middleware.NewRecipientRateLimiter(time.Second, 1)
	defer limiter.Stop()

	mw := RecipientRateLimitMiddleware(limiter, func(c echo.Context) bool { return false })
	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/bridge/message?to=abc123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}

func TestRecipientRateLimitMiddleware_BlocksSecondRequest(t *testing.T) {
	e := echo.New()
	limiter := bridge_middleware.NewRecipientRateLimiter(time.Second, 1)
	defer limiter.Stop()

	mw := RecipientRateLimitMiddleware(limiter, func(c echo.Context) bool { return false })
	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	// First request — allowed
	req := httptest.NewRequest(http.MethodPost, "/bridge/message?to=abc123", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second request — blocked
	req = httptest.NewRequest(http.MethodPost, "/bridge/message?to=abc123", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	if err := handler(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected status 429, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if body["error"] != "too many requests to this recipient" {
		t.Fatalf("unexpected error message: %s", body["error"])
	}
}

func TestRecipientRateLimitMiddleware_SkipperBypasses(t *testing.T) {
	e := echo.New()
	limiter := bridge_middleware.NewRecipientRateLimiter(time.Second, 1)
	defer limiter.Stop()

	mw := RecipientRateLimitMiddleware(limiter, func(c echo.Context) bool { return true })
	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/bridge/message?to=abc123", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		if err := handler(c); err != nil {
			t.Fatalf("unexpected error on request %d: %v", i, err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200 with skipper, got %d on request %d", rec.Code, i)
		}
	}
}

func TestRecipientRateLimitMiddleware_NoToParam(t *testing.T) {
	e := echo.New()
	limiter := bridge_middleware.NewRecipientRateLimiter(time.Second, 1)
	defer limiter.Stop()

	mw := RecipientRateLimitMiddleware(limiter, func(c echo.Context) bool { return false })
	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/bridge/message", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		if err := handler(c); err != nil {
			t.Fatalf("unexpected error on request %d: %v", i, err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200 without to param, got %d on request %d", rec.Code, i)
		}
	}
}

func TestRecipientRateLimitMiddleware_DifferentRecipients(t *testing.T) {
	e := echo.New()
	limiter := bridge_middleware.NewRecipientRateLimiter(time.Second, 1)
	defer limiter.Stop()

	mw := RecipientRateLimitMiddleware(limiter, func(c echo.Context) bool { return false })
	handler := mw(func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	recipients := []string{"aaa", "bbb", "ccc"}
	for _, to := range recipients {
		req := httptest.NewRequest(http.MethodPost, "/bridge/message?to="+to, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		if err := handler(c); err != nil {
			t.Fatalf("unexpected error for recipient %s: %v", to, err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200 for first request to %s, got %d", to, rec.Code)
		}
	}
}
