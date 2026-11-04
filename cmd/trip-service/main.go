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
	"time"

	"github.com/MichaelRayven/go-course/internal/config"
	"github.com/MichaelRayven/go-course/internal/http/health"
	"github.com/MichaelRayven/go-course/internal/http/router"
	createhttp "github.com/MichaelRayven/go-course/internal/http/trip/create"
	finishhttp "github.com/MichaelRayven/go-course/internal/http/trip/finish"
	gethttp "github.com/MichaelRayven/go-course/internal/http/trip/get"
	"github.com/MichaelRayven/go-course/internal/infra/postgres/transaction"
	triprepo "github.com/MichaelRayven/go-course/internal/repository/trip"
	createuc "github.com/MichaelRayven/go-course/internal/usecase/trip/create"
	finishuc "github.com/MichaelRayven/go-course/internal/usecase/trip/finish"
	getuc "github.com/MichaelRayven/go-course/internal/usecase/trip/get"
	"github.com/jackc/pgx/v5/pgxpool"
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

	logger := newLogger(cfg.LogLevel)

	appCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	poolConfig, err := pgxpool.ParseConfig(cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("parse PostgreSQL pool config: %w", err)
	}
	poolConfig.MaxConns = cfg.Database.MaxConns
	poolConfig.MinConns = cfg.Database.MinConns
	poolConfig.MaxConnLifetime = cfg.Database.MaxConnLifetime
	poolConfig.ConnConfig.ConnectTimeout = cfg.Database.ConnectTimeout

	pool, err := pgxpool.NewWithConfig(appCtx, poolConfig)
	if err != nil {
		return fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	defer pool.Close()

	pingCtx, cancelPing := context.WithTimeout(appCtx, cfg.Database.QueryTimeout)
	err = pool.Ping(pingCtx)
	cancelPing()
	if err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}

	transactionManager := transaction.NewTransactionManager(pool, cfg.Database.QueryTimeout)
	tripRepository := triprepo.New(pool, cfg.Database.QueryTimeout)
	now := func() time.Time { return time.Now().UTC() }

	// TODO(T9): this wiring moves into container.New(cfg).
	handler := router.New(router.Handlers{
		Create: createhttp.New(createuc.New(tripRepository, transactionManager, logger, now), logger),
		Get:    gethttp.New(getuc.New(tripRepository), logger),
		Finish: finishhttp.New(finishuc.New(tripRepository, transactionManager, logger, now), logger),
		Health: health.New(pool, cfg.Database.QueryTimeout),
	}, logger)

	server := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           handler,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("HTTP server is listening", "address", cfg.HTTP.Addr)
		serverErrors <- server.ListenAndServe()
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

		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		logger.Info("HTTP server stopped")
		return nil
	}
}

func newLogger(levelName string) *slog.Logger {
	level := slog.LevelInfo
	switch levelName {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler).With("service", "trip-service")
}
