package main

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func registerHandlers(e *echo.Echo, h *handler) {
	cors := middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST"},
	}
	bridge := e.Group("/bridge")
	bridge.GET("/events", h.EventRegistrationHandler)
	bridge.POST("/message", h.SendMessageHandler, middleware.CORSWithConfig(cors))
}
