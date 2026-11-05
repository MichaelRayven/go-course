package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MichaelRayven/go-course/internal/config"
	"github.com/MichaelRayven/go-course/internal/http/health"
	"github.com/MichaelRayven/go-course/internal/http/router"
	createhttp "github.com/MichaelRayven/go-course/internal/http/trip/create"
	finishhttp "github.com/MichaelRayven/go-course/internal/http/trip/finish"
	gethttp "github.com/MichaelRayven/go-course/internal/http/trip/get"
	"github.com/MichaelRayven/go-course/internal/infra/postgres"
	"github.com/MichaelRayven/go-course/internal/infra/postgres/transaction"
	"github.com/MichaelRayven/go-course/internal/infra/telemetry"
	triprepo "github.com/MichaelRayven/go-course/internal/repository/trip"
	createuc "github.com/MichaelRayven/go-course/internal/usecase/trip/create"
	finishuc "github.com/MichaelRayven/go-course/internal/usecase/trip/finish"
	getuc "github.com/MichaelRayven/go-course/internal/usecase/trip/get"
)

type Container struct {
	Server    *http.Server
	Logger    *slog.Logger
	pool      *pgxpool.Pool
	telemetry *telemetry.Telemetry
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

	tel, err := telemetry.New(ctx, telemetry.Config{ServiceName: cfg.Telemetry.ServiceName}, logger)
	if err != nil {
		return nil, fmt.Errorf("set up telemetry: %w", err)
	}

	pool, err := newPool(ctx, cfg.Database, tel)
	if err != nil {
		return nil, errors.Join(err, tel.Shutdown(context.WithoutCancel(ctx)))
	}

	transactionManager := transaction.NewTransactionManager(pool, cfg.Database.QueryTimeout)
	tripRepository := triprepo.New(pool, cfg.Database.QueryTimeout)
	now := func() time.Time { return time.Now().UTC() }

	handler := router.New(router.Handlers{
		Create: createhttp.New(createuc.New(tripRepository, transactionManager, logger, now), logger),
		Get:    gethttp.New(getuc.New(tripRepository), logger),
		Finish: finishhttp.New(finishuc.New(tripRepository, transactionManager, logger, now), logger),
		Health: health.New(pool, cfg.Database.QueryTimeout),
	}, router.Observability{
		TracerProvider: tel.TracerProvider,
		MeterProvider:  tel.MeterProvider,
		Propagator:     tel.Propagator,
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
		Server:    server,
		Logger:    logger,
		pool:      pool,
		telemetry: tel,
	}, nil
}

// newPool creates the pgx pool with query tracing, and checks that the
// database answers.
func newPool(ctx context.Context, cfg config.DatabaseConfig, tel *telemetry.Telemetry) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL pool config: %w", err)
	}
	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	tracer, err := postgres.NewQueryTracer(tel.TracerProvider, tel.MeterProvider)
	if err != nil {
		return nil, fmt.Errorf("create query tracer: %w", err)
	}
	poolConfig.ConnConfig.Tracer = tracer

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}

	pingCtx, cancelPing := context.WithTimeout(ctx, cfg.QueryTimeout)
	defer cancelPing()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}

	return pool, nil
}

func (c *Container) Close(ctx context.Context) error {
	c.pool.Close()
	return c.telemetry.Shutdown(ctx)
}
