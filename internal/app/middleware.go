package app

import (
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
