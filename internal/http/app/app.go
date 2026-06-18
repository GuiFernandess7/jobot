package app

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/GuiFernandess7/jobot/internal/http/auth"
	"github.com/GuiFernandess7/jobot/internal/http/handlers"
	"github.com/GuiFernandess7/jobot/internal/http/routes"
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
)

func NewHandler(logger *slog.Logger, triggerAPIKey string) http.Handler {
	return newEcho(logger, triggerAPIKey)
}

func newEcho(logger *slog.Logger, triggerAPIKey string) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(echomiddleware.RequestID())
	e.Use(echomiddleware.RemoveTrailingSlash())
	e.Use(echomiddleware.Recover())
	e.Use(echomiddleware.SecureWithConfig(echomiddleware.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            31536000,
		HSTSExcludeSubdomains: false,
		ContentSecurityPolicy: "default-src 'self'",
		ReferrerPolicy:        "no-referrer",
	}))
	e.Use(echomiddleware.RequestLoggerWithConfig(echomiddleware.RequestLoggerConfig{
		LogStatus:    true,
		LogURI:       true,
		LogMethod:    true,
		LogLatency:   true,
		LogRemoteIP:  true,
		LogRequestID: true,
		LogValuesFunc: func(c echo.Context, values echomiddleware.RequestLoggerValues) error {
			logger.Info("http request",
				"request_id", values.RequestID,
				"method", values.Method,
				"uri", values.URI,
				"status", values.Status,
				"latency", values.Latency.Round(time.Microsecond).String(),
				"remote_ip", values.RemoteIP,
			)
			return nil
		},
	}))
	e.Use(auth.APIKey(triggerAPIKey, logger))

	routes.Register(e, handlers.NewTriggerHandler(logger))

	return e
}