package routes

import (
	"github.com/GuiFernandess7/jobot/internal/http/handlers"
	"github.com/labstack/echo/v4"
)

func Register(e *echo.Echo, triggerHandler handlers.TriggerHandler) {
	e.Any("/trigger", triggerHandler.Handle)
}