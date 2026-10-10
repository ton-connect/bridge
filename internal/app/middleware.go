package app

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/ton-connect/bridge/internal/config"
	bridge_middleware "github.com/ton-connect/bridge/internal/middleware"
	"github.com/ton-connect/bridge/internal/utils"
	"golang.org/x/exp/slices"
)

// SkipRateLimitsByToken checks if the request should bypass rate limits based on bearer token
func SkipRateLimitsByToken(request *http.Request) bool {
	if request == nil {
		return false
	}
	authorization := request.Header.Get("Authorization")
	if authorization == "" {
		return false
	}
	token := strings.TrimPrefix(authorization, "Bearer ")
	exist := slices.Contains(config.Config.RateLimitsByPassToken, token)
	if exist {
		TokenUsageMetric.WithLabelValues(token).Inc()
		return true
	}
	return false
}

// ConnectionsLimitMiddleware creates middleware for limiting concurrent connections
func ConnectionsLimitMiddleware(counter *bridge_middleware.ConnectionsLimiter, skipper func(c echo.Context) bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if skipper(c) {
				return next(c)
			}
			release, err := counter.LeaseConnection(c.Request())
			if err != nil {
				return c.JSON(utils.HttpResError(err.Error(), http.StatusTooManyRequests))
			}
			defer release()
			return next(c)
		}
	}
}

// BodyLimitMiddleware reads the body before the handler runs and answers 413 once it passes
// maxBodySize, so the status does not depend on Content-Length or on how a handler reads.
func BodyLimitMiddleware(maxBodySize int64) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			if req.ContentLength > maxBodySize {
				return echo.ErrStatusRequestEntityTooLarge
			}
			if req.Body == nil || req.Body == http.NoBody {
				return next(c)
			}
			body, err := io.ReadAll(io.LimitReader(req.Body, maxBodySize+1))
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err.Error())
			}
			if int64(len(body)) > maxBodySize {
				return echo.ErrStatusRequestEntityTooLarge
			}
			req.Body = io.NopCloser(bytes.NewReader(body))
			return next(c)
		}
	}
}
