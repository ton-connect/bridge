package main

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/tonkeeper/bridge/config"
)

func registerHandlers(e *echo.Echo, h *handler) {
	bridge := e.Group("/bridge")

	if config.Config.CorsEnable {
		bridge.GET("/events", h.EventRegistrationHandler, middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: []string{"*"},
			AllowMethods: []string{"GET"},
		}))
		bridge.POST("/message", h.SendMessageHandler, middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: []string{"*"},
			AllowMethods: []string{"POST"},
		}))

	} else {
		bridge.GET("/events", h.EventRegistrationHandler)
		bridge.POST("/message", h.SendMessageHandler)
	}
}
