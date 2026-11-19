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

	// Start background positions worker pool and scanner
	c.PositionsPool.Start(appCtx)
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
		c.Logger.Info("initiating graceful shutdown...")
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancelShutdown()

		// Step 1: HTTP server stops accepting new requests and awaits active ones
		if err := c.Server.Shutdown(shutdownCtx); err != nil {
			c.Logger.Error("step 1/4: HTTP server graceful shutdown timed out, forcing close", "error", err)
			_ = c.Server.Close()
		} else {
			c.Logger.Info("step 1/4: HTTP server stopped")
		}

		// Step 2: Scanner stops — no new tasks will be enqueued
		c.Scanner.Stop()
		c.Logger.Info("step 2/4: positions scanner stopped")

		// Step 3: Workers drain remaining tasks in queue within shutdown timeout
		if err := c.PositionsPool.Stop(shutdownCtx); err != nil {
			c.Logger.Error("step 3/4: positions worker pool drain failed", "error", err)
		} else {
			c.Logger.Info("step 3/4: positions worker pool drained and stopped")
		}

		// Step 4: Close Push client, database pool, and telemetry
		if err := c.Close(shutdownCtx); err != nil {
			c.Logger.Error("step 4/4: failed to close database and telemetry", "error", err)
			return fmt.Errorf("close resources: %w", err)
		}
		c.Logger.Info("step 4/4: resources closed, shutdown complete")

		return nil
	}
}
