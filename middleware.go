package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func connectionsLimitMiddleware(authenticator *Authenticator) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			release, err := authenticator.leaseConnection(c.Request())
			if err != nil {
				return c.JSON(HttpResError(err.Error(), http.StatusTooManyRequests))
			}
			defer release()
			return next(c)
		}
	}
}
