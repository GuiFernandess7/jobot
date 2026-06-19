package jobotfunction

import (
	"log/slog"
	"net/http"
	"os"
	"sync"

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

		logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
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