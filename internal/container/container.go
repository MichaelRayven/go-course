package container

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
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

type Container struct {
	Server *http.Server
	Logger *slog.Logger
	pool   *pgxpool.Pool
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

func New(ctx context.Context, cfg config.Config) (*Container, error) {

	logger := newLogger(cfg.LogLevel)

	poolConfig, err := pgxpool.ParseConfig(cfg.Database.URL)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL pool config: %w", err)
	}
	poolConfig.MaxConns = cfg.Database.MaxConns
	poolConfig.MinConns = cfg.Database.MinConns
	poolConfig.MaxConnLifetime = cfg.Database.MaxConnLifetime
	poolConfig.ConnConfig.ConnectTimeout = cfg.Database.ConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}

	pingCtx, cancelPing := context.WithTimeout(ctx, cfg.Database.QueryTimeout)
	err = pool.Ping(pingCtx)
	cancelPing()
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}

	transactionManager := transaction.NewTransactionManager(pool, cfg.Database.QueryTimeout)
	tripRepository := triprepo.New(pool, cfg.Database.QueryTimeout)
	now := func() time.Time { return time.Now().UTC() }

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

	return &Container{
		Server: server,
		Logger: logger,
		pool:   pool,
	}, nil
}

func (c *Container) Close() {
	c.pool.Close()
}
