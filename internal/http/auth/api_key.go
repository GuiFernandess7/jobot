package auth

import (
	"crypto/subtle"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
)

const APIKeyHeader = "X-API-Key"

func APIKey(expectedKey string, logger *slog.Logger) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if expectedKey == "" {
				logger.Error("trigger api key is not configured")
				return c.JSON(http.StatusInternalServerError, map[string]string{
					"message": "trigger api key is not configured",
				})
			}

			providedKey := c.Request().Header.Get(APIKeyHeader)
			if subtle.ConstantTimeCompare([]byte(providedKey), []byte(expectedKey)) != 1 {
				return c.JSON(http.StatusUnauthorized, map[string]string{
					"message": "invalid api key",
				})
			}

			return next(c)
		}
	}
}