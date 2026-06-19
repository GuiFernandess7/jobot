package routes

import (
	"github.com/GuiFernandess7/jobot/services/function/internal/http/handlers"
	"github.com/labstack/echo/v4"
)

func Register(e *echo.Echo, triggerHandler handlers.TriggerHandler) {
	e.POST("/trigger", triggerHandler.Handle)
}