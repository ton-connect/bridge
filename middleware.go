package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func connectionsLimitMiddleware(counter *ConnectionsLimiter, skipper func(c echo.Context) bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if skipper(c) {
				return next(c)
			}
			release, err := counter.leaseConnection(c.Request())
			if err != nil {
				return c.JSON(HttpResError(err.Error(), http.StatusTooManyRequests))
			}
			defer release()
			return next(c)
		}
	}
}
