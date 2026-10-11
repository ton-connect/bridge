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
	e.POST("/bridge/message", func(c echo.Context) error {
		if _, err := io.ReadAll(c.Request().Body); err != nil {
			return err
		}
		return c.NoContent(http.StatusOK)
	})

	cases := []struct {
		name          string
		body          string
		contentLength int64
		want          int
	}{
		{"at the limit", "12345678", 8, http.StatusOK},
		{"over the limit", "123456789", 9, http.StatusRequestEntityTooLarge},
		{"over the limit without Content-Length", "123456789", -1, http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/bridge/message", strings.NewReader(tc.body))
			req.ContentLength = tc.contentLength
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}
