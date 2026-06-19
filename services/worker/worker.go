package jobotworker

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/GuiFernandess7/jobot/services/worker/internal/jobs"
	"github.com/joho/godotenv"
)

func RunWorker(ctx context.Context) error {
	loadEnvFiles()

	logger := newLogger()
	logger.Info("worker execution requested")

	processor, err := jobs.NewProcessor(logger)
	if err != nil {
		logger.Error("worker initialization failed", "error", err)
		return err
	}
	defer processor.Close()

	if err := processor.Run(ctx); err != nil {
		logger.Error("worker execution failed", "error", err)
		return err
	}

	logger.Info("worker execution completed")
	return nil
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