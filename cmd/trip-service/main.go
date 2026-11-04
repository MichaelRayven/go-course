package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/MichaelRayven/go-course/internal/config"
	"github.com/MichaelRayven/go-course/internal/container"
)

func main() {
	if err := run(); err != nil {
		logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
		logger.Error("trip service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	appCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	c, err := container.New(appCtx, cfg)
	if err != nil {
		return fmt.Errorf("container error: %w", err)
	}
	defer c.Close()

	serverErrors := make(chan error, 1)
	go func() {
		c.Logger.Info("HTTP server is listening", "address", cfg.HTTP.Addr)
		serverErrors <- c.Server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-appCtx.Done():
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancelShutdown()

		if err := c.Server.Shutdown(shutdownCtx); err != nil {
			c.Logger.Error("graceful shutdown timed out, forcing close", "error", err)
			c.Server.Close()
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		c.Logger.Info("HTTP server stopped")
		return nil
	}
}
