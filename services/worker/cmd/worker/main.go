package main

import (
	"context"
	"log"

	worker "github.com/GuiFernandess7/jobot/services/worker"
)

func main() {
	if err := worker.RunWorker(context.Background()); err != nil {
		log.Fatalf("worker failed: %v", err)
	}
}