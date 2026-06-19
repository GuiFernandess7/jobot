package handlers

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/GuiFernandess7/jobot/services/function/internal/jobs"
	"github.com/labstack/echo/v4"
)

type TriggerHandler struct {
	logger  *slog.Logger
	service *jobs.Service
}

type TriggerRequest struct {
	Terms []string `json:"terms"`
}

func NewTriggerHandler(logger *slog.Logger, service *jobs.Service) TriggerHandler {
	return TriggerHandler{logger: logger, service: service}
}

func (h TriggerHandler) Handle(c echo.Context) error {
	var request TriggerRequest
	if c.Request().ContentLength != 0 {
		if err := c.Bind(&request); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{
				"message": "invalid request payload",
			})
		}
	}

	requestID := c.Response().Header().Get(echo.HeaderXRequestID)
	if err := h.service.StartCapture(requestID, request.Terms); err != nil {
		status := http.StatusInternalServerError
		message := "failed to trigger capture"
		if errors.Is(err, jobs.ErrNoSearchTerms) {
			status = http.StatusBadRequest
			message = err.Error()
		}

		h.logger.Error("trigger endpoint failed", "request_id", requestID, "error", err)
		return c.JSON(status, map[string]string{
			"message": message,
		})
	}

	h.logger.Info("trigger endpoint accepted", "request_id", requestID)

	return c.JSON(http.StatusAccepted, map[string]any{
		"message":    "capture started",
		"request_id": requestID,
		"terms":      h.service.NormalizeTerms(request.Terms),
	})
}