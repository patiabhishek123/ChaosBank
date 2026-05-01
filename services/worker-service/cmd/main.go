package main

import (
	"context"
	"database/sql"
	"os"
	"os/signal"
	"syscall"

	"github.com/chaosbank/chaosbank/pkg/service"
	"github.com/chaosbank/chaosbank/services/worker-service/config"
	"github.com/chaosbank/chaosbank/services/worker-service/internal/worker"
	_ "github.com/lib/pq"
)

func main() {
	baseCfg := service.LoadConfig()
	logger := service.NewLogger(baseCfg.LogLevel, os.Stdout)
	appCfg := config.Load()

	db, err := sql.Open("postgres", appCfg.DatabaseURL)
	if err != nil {
		logger.Error("db.connection_error", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		logger.Error("db.ping_error", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}

	logger.Info("db.connected", nil)

	router := service.NewRouter()
	workerSvc := worker.NewWorker(appCfg, logger, db)
	router.Post("/replay", workerSvc.ReplayHTTPHandler())
	router.Get("/chaos", workerSvc.ChaosHTTPHandler())
	router.Post("/chaos", workerSvc.ChaosHTTPHandler())
	router.Get("/transactions/log", workerSvc.TransactionLogHTTPHandler())
	router.Get("/stats", workerSvc.StatsHTTPHandler())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go workerSvc.Start(ctx)

	server := service.NewServer(baseCfg, router)
	if err := service.Run(ctx, server, logger, baseCfg); err != nil {
		logger.Error("service.failed", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
}
