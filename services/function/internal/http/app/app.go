package app

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/GuiFernandess7/jobot/services/function/internal/http/handlers"
	"github.com/GuiFernandess7/jobot/services/function/internal/http/routes"
	"github.com/GuiFernandess7/jobot/services/function/internal/jobs"
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
)

func NewHandler(logger *slog.Logger) (http.Handler, error) {
	service, err := jobs.NewService(logger)
	if err != nil {
		return nil, err
	}

	return newEcho(logger, service), nil
}

func newEcho(logger *slog.Logger, service *jobs.Service) *echo.Echo {
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

	routes.Register(e, handlers.NewTriggerHandler(logger, service))

	return e
}