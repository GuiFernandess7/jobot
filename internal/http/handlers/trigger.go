package handlers

import (
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
)

type TriggerHandler struct {
	logger *slog.Logger
}

func NewTriggerHandler(logger *slog.Logger) TriggerHandler {
	return TriggerHandler{logger: logger}
}

func (h TriggerHandler) Handle(c echo.Context) error {
	requestID := c.Response().Header().Get(echo.HeaderXRequestID)
	h.logger.Info("trigger endpoint called", "request_id", requestID)

	return c.JSON(http.StatusOK, map[string]string{
		"message": "trigger received",
	})
}