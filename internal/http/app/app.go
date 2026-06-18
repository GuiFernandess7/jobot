package app

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/GuiFernandess7/jobot/internal/http/handlers"
	"github.com/GuiFernandess7/jobot/internal/http/routes"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func NewHandler(logger *slog.Logger) http.Handler {
	return newEcho(logger)
}

func newEcho(logger *slog.Logger) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Use(middleware.RequestID())
	e.Use(middleware.RemoveTrailingSlash())
	e.Use(middleware.Recover())
	e.Use(middleware.SecureWithConfig(middleware.SecureConfig{
		XSSProtection:         "1; mode=block",
		ContentTypeNosniff:    "nosniff",
		XFrameOptions:         "DENY",
		HSTSMaxAge:            31536000,
		HSTSExcludeSubdomains: false,
		ContentSecurityPolicy: "default-src 'self'",
		ReferrerPolicy:        "no-referrer",
	}))
	e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:    true,
		LogURI:       true,
		LogMethod:    true,
		LogLatency:   true,
		LogRemoteIP:  true,
		LogRequestID: true,
		LogValuesFunc: func(c echo.Context, values middleware.RequestLoggerValues) error {
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

	routes.Register(e, handlers.NewTriggerHandler(logger))

	return e
}