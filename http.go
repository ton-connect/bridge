package main

import (
	"github.com/labstack/echo/v4"
)

func registerHandlers(e *echo.Echo, h *handler) {
	bridge := e.Group("/bridge")

	bridge.GET("/events", h.EventRegistrationHandler)
	bridge.POST("/message", h.SendMessageHandler)
}
