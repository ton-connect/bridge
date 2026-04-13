package middleware

import (
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
	log "github.com/sirupsen/logrus"
)

// RequestLogger returns an echo middleware that logs HTTP requests via logrus.
// It replaces the deprecated middleware.Logger() from echo v4.15+.
func RequestLogger() echo.MiddlewareFunc {
	return echomw.RequestLoggerWithConfig(echomw.RequestLoggerConfig{
		LogLatency:       true,
		LogRemoteIP:      true,
		LogHost:          true,
		LogMethod:        true,
		LogURI:           true,
		LogRequestID:     true,
		LogUserAgent:     true,
		LogStatus:        true,
		LogError:         true,
		LogContentLength: true,
		LogResponseSize:  true,
		HandleError:      true,
		LogValuesFunc: func(_ echo.Context, v echomw.RequestLoggerValues) error {
			fields := log.Fields{
				"id":            v.RequestID,
				"remote_ip":     v.RemoteIP,
				"host":          v.Host,
				"method":        v.Method,
				"uri":           v.URI,
				"user_agent":    v.UserAgent,
				"status":        v.Status,
				"latency":       v.Latency.Nanoseconds(),
				"latency_human": v.Latency.String(),
				"bytes_in":      v.ContentLength,
				"bytes_out":     v.ResponseSize,
			}
			if v.Error != nil {
				log.WithFields(fields).WithError(v.Error).Error("request")
			} else {
				log.WithFields(fields).Info("request")
			}
			return nil
		},
	})
}
