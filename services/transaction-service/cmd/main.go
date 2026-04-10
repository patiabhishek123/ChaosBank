package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/chaosbank/chaosbank/pkg/service"
	"github.com/chaosbank/chaosbank/services/transaction-service/internal/handler"
)

func main() {
	baseCfg := service.LoadConfig()
	logger := service.NewLogger(baseCfg.LogLevel, os.Stdout)

	router := service.NewRouter()
	h := handler.NewHandler(logger)
	router.Mount("/", h.Router())

	server := service.NewServer(baseCfg, router)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := service.Run(ctx, server, logger, baseCfg); err != nil {
		logger.Error("service.failed", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
}
