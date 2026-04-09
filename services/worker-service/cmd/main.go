package main

import (
	"log"

	"github.com/chaosbank/chaosbank/services/worker-service/config"
	"github.com/chaosbank/chaosbank/services/worker-service/internal/worker"
)

func main() {
	cfg := config.Load()

	w := worker.NewWorker(cfg)

	log.Println("Worker Service starting")
	w.Start()
}