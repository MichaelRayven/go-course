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
		return fmt.Errorf("build container: %w", err)
	}
	defer func() {
		// Clean up on early abort/error if shutdown was not run
		closeCtx, cancelClose := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancelClose()
		if err := c.Close(closeCtx); err != nil {
			c.Logger.Warn("close dependencies", "error", err)
		}
	}()

	// Start background positions worker pool and scanner.
	// Pool uses Background so it can drain queued tasks during ShutdownTimeout after appCtx is cancelled.
	c.PositionsPool.Start(context.Background())
	c.Scanner.Start(appCtx)

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
		c.Logger.Info("initiating graceful shutdown")
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancelShutdown()

		if err := c.Server.Shutdown(shutdownCtx); err != nil {
			c.Logger.Error("HTTP server graceful shutdown timed out, forcing close", "error", err)
			_ = c.Server.Close()
		} else {
			c.Logger.Info("HTTP server stopped")
		}

		c.Scanner.Stop()
		c.Logger.Info("positions scanner stopped")

		if err := c.PositionsPool.Stop(shutdownCtx); err != nil {
			c.Logger.Error("positions worker pool drain failed", "error", err)
		} else {
			c.Logger.Info("positions worker pool stopped")
		}

		if err := c.Close(shutdownCtx); err != nil {
			c.Logger.Error("failed to close resources", "error", err)
			return fmt.Errorf("close resources: %w", err)
		}
		c.Logger.Info("shutdown complete")

		return nil
	}
}
