package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestBodyLimitMiddleware(t *testing.T) {
	e := echo.New()
	e.Use(BodyLimitMiddleware(8))
	var called bool
	var seen string
	e.Any("/ignores-body", func(c echo.Context) error {
		called = true
		return c.NoContent(http.StatusOK)
	})
	e.POST("/reads-body", func(c echo.Context) error {
		called = true
		body, err := io.ReadAll(c.Request().Body)
		seen = string(body)
		if err != nil {
			return err
		}
		return c.NoContent(http.StatusOK)
	})

	cases := []struct {
		name          string
		method        string
		path          string
		body          string
		contentLength int64
		want          int
		wantCalled    bool
	}{
		{"at the limit", http.MethodPost, "/reads-body", "12345678", 8, http.StatusOK, true},
		{"over the limit", http.MethodPost, "/ignores-body", "123456789", 9, http.StatusRequestEntityTooLarge, false},
		{"over the limit without Content-Length", http.MethodPost, "/ignores-body", "123456789", -1, http.StatusRequestEntityTooLarge, false},
		{"no body", http.MethodGet, "/ignores-body", "", 0, http.StatusOK, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called, seen = false, ""
			var body io.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			req.ContentLength = tc.contentLength
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if called != tc.wantCalled {
				t.Fatalf("handler called = %v, want %v", called, tc.wantCalled)
			}
			if tc.path == "/reads-body" && seen != tc.body {
				t.Fatalf("handler read %q, want %q", seen, tc.body)
			}
		})
	}
}
