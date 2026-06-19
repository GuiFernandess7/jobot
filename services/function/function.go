package jobotfunction

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"github.com/GuiFernandess7/jobot/services/function/internal/http/app"
	"github.com/joho/godotenv"
)

var (
	functionHandler http.Handler
	handlerOnce     sync.Once
)

func init() {
	functions.HTTP("Trigger", Trigger)
}

func Trigger(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/trigger" {
		r.URL.Path = "/trigger"
	}

	getFunctionHandler().ServeHTTP(w, r)
}

func getFunctionHandler() http.Handler {
	handlerOnce.Do(func() {
		loadEnvFiles()

		logger := newLogger()
		handler, err := app.NewHandler(logger)
		if err != nil {
			panic(err)
		}

		functionHandler = handler
	})

	return functionHandler
}

func loadEnvFiles() {
	_ = godotenv.Load()
	_ = godotenv.Load("../../.env")
}

func newLogger() *slog.Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			switch attr.Key {
			case slog.TimeKey:
				if timestamp, ok := attr.Value.Any().(time.Time); ok {
					attr.Value = slog.StringValue(timestamp.Format("2006-01-02 15:04:05"))
				}
			case slog.LevelKey:
				attr.Value = slog.StringValue(strings.ToUpper(attr.Value.String()))
			}

			return attr
		},
	})

	return slog.New(handler)
}