package main

import (
	"github.com/labstack/echo/v4"
)

func registerHandlers(e *echo.Echo, h *handler) {
	e.GET("/bridge/events", h.EventRegistrationHandler)
	e.POST("/bridge/message", h.SendMessageHandler)
	e.POST("/bridge/verify", h.ConnectVerifyHandler)
}
