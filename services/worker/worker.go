package jobotworker

import (
	"context"
	"log/slog"
	"os"

	"github.com/GuiFernandess7/jobot/services/worker/internal/jobs"
	"github.com/joho/godotenv"
)

func RunWorker(ctx context.Context) error {
	loadEnvFiles()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
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