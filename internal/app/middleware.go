package app

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
	"github.com/tonkeeper/bridge/internal/config"
	bridge_middleware "github.com/tonkeeper/bridge/internal/middleware"
	"github.com/tonkeeper/bridge/internal/utils"
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

// LogrusLoggerMiddleware creates a middleware that logs HTTP requests using logrus
// This ensures the Echo framework logs match the same format as the bridge logger
func LogrusLoggerMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()

			req := c.Request()
			res := c.Response()

			err := next(c)

			stop := time.Now()

			fields := logrus.Fields{
				"remote_ip":  c.RealIP(),
				"host":       req.Host,
				"method":     req.Method,
				"uri":        req.RequestURI,
				"status":     res.Status,
				"latency":    stop.Sub(start).String(),
				"latency_ms": stop.Sub(start).Milliseconds(),
				"bytes_in":   req.Header.Get("Content-Length"),
				"bytes_out":  res.Size,
			}

			if ua := req.UserAgent(); ua != "" {
				fields["user_agent"] = ua
			}

			if referer := req.Referer(); referer != "" {
				fields["referer"] = referer
			}

			if id := req.Header.Get(echo.HeaderXRequestID); id != "" {
				fields["request_id"] = id
			}

			logrus.WithFields(fields).Info()

			return err
		}
	}
}
