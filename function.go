package jobot

import (
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"github.com/GuiFernandess7/jobot/internal/http/app"
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
		logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
		functionHandler = app.NewHandler(logger)
	})

	return functionHandler
}