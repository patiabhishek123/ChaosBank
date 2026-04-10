package main

import (
	"context"
	"database/sql"
	"os"
	"os/signal"
	"syscall"

	"github.com/chaosbank/chaosbank/pkg/service"
	"github.com/chaosbank/chaosbank/services/transaction-service/config"
	"github.com/chaosbank/chaosbank/services/transaction-service/internal/handler"
	_ "github.com/lib/pq"
)

func main() {
	baseCfg := service.LoadConfig()
	appCfg := config.Load()
	logger := service.NewLogger(baseCfg.LogLevel, os.Stdout)

	// Connect to database
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
	h := handler.NewHandler(logger, db)
	router.Mount("/", h.Router())

	server := service.NewServer(baseCfg, router)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := service.Run(ctx, server, logger, baseCfg); err != nil {
		logger.Error("service.failed", map[string]interface{}{"error": err.Error()})
		os.Exit(1)
	}
}
