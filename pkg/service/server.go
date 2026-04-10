package service

import (
	"context"
	"net/http"
)

func NewServer(cfg *Config, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}
}

func Run(ctx context.Context, server *http.Server, logger *Logger, cfg *Config) error {
	errCh := make(chan error, 1)

	go func() {
		logger.Info("server.starting", map[string]interface{}{"addr": server.Addr})
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		logger.Info("server.shutting_down", nil)
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server.shutdown.failed", map[string]interface{}{"error": err.Error()})
			return err
		}
		logger.Info("server.shutdown.complete", nil)
		return nil
	case err := <-errCh:
		if err == nil || err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
