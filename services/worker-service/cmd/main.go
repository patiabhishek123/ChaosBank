package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/chaosbank/chaosbank/pkg/service"
	"github.com/chaosbank/chaosbank/services/worker-service/config"
	"github.com/chaosbank/chaosbank/services/worker-service/internal/worker"
)

func main() {
	baseCfg := service.LoadConfig()
	logger := service.NewLogger(baseCfg.LogLevel, os.Stdout)
	appCfg := config.Load()

	router := service.NewRouter()
	workerSvc := worker.NewWorker(appCfg, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go workerSvc.Start(ctx)

	server := service.NewServer(baseCfg, router)
	if err := service.Run(ctx, server, logger, baseCfg); err != nil {
		logger.Error("service.failed", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
}
