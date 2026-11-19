package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MichaelRayven/go-course/internal/config"
	"github.com/MichaelRayven/go-course/internal/gateway/push"
	"github.com/MichaelRayven/go-course/internal/http/health"
	"github.com/MichaelRayven/go-course/internal/http/router"
	createhttp "github.com/MichaelRayven/go-course/internal/http/trip/create"
	finishhttp "github.com/MichaelRayven/go-course/internal/http/trip/finish"
	gethttp "github.com/MichaelRayven/go-course/internal/http/trip/get"
	createposhttp "github.com/MichaelRayven/go-course/internal/http/trip/position/create"
	listposhttp "github.com/MichaelRayven/go-course/internal/http/trip/position/list"
	"github.com/MichaelRayven/go-course/internal/infra/postgres"
	"github.com/MichaelRayven/go-course/internal/infra/postgres/transaction"
	"github.com/MichaelRayven/go-course/internal/infra/telemetry"
	triprepo "github.com/MichaelRayven/go-course/internal/repository/trip"
	createuc "github.com/MichaelRayven/go-course/internal/usecase/trip/create"
	finishuc "github.com/MichaelRayven/go-course/internal/usecase/trip/finish"
	getuc "github.com/MichaelRayven/go-course/internal/usecase/trip/get"
	createposuc "github.com/MichaelRayven/go-course/internal/usecase/trip/position/create"
	listposuc "github.com/MichaelRayven/go-course/internal/usecase/trip/position/list"
	positions "github.com/MichaelRayven/go-course/internal/worker/positions"
)

type Container struct {
	Server        *http.Server
	Logger        *slog.Logger
	Scanner       *positions.Scanner
	PositionsPool *positions.Pool
	Queue         *positions.Queue
	PushGateway   *push.Gateway
	pool          *pgxpool.Pool
	telemetry     *telemetry.Telemetry
	closeOnce     sync.Once
}

func New(ctx context.Context, cfg config.Config) (*Container, error) {
	logger := telemetry.NewLogger(os.Stdout, cfg.LogLevel, cfg.Telemetry.ServiceName)

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

	tripsCreated, err := telemetry.NewCounter(tel.MeterProvider, "tripgo.trips.created", "Trips created.")
	if err != nil {
		return nil, closeAll(ctx, err, pool, tel)
	}
	tripsCompleted, err := telemetry.NewCounter(tel.MeterProvider, "tripgo.trips.completed", "Trips completed.")
	if err != nil {
		return nil, closeAll(ctx, err, pool, tel)
	}
	positionsSaved, err := telemetry.NewCounter(tel.MeterProvider, "tripgo.positions.saved", "Positions saved.")
	if err != nil {
		return nil, closeAll(ctx, err, pool, tel)
	}

	pushHTTPClient := &http.Client{
		Timeout: cfg.Push.RequestTimeout,
	}
	pushGateway, err := push.New(pushHTTPClient, cfg.Push.URL, tel.TracerProvider, tel.MeterProvider, tel.Propagator)
	if err != nil {
		return nil, closeAll(ctx, fmt.Errorf("create push gateway: %w", err), pool, tel)
	}

	queue, err := positions.NewQueue(cfg.Positions.QueueSize, tel.MeterProvider)
	if err != nil {
		return nil, closeAll(ctx, fmt.Errorf("create positions queue: %w", err), pool, tel)
	}

	positionsPool, err := positions.NewPool(
		cfg.Positions.Workers,
		cfg.Push.RequestTimeout,
		queue,
		pushGateway,
		tel.TracerProvider,
		tel.MeterProvider,
		logger,
	)
	if err != nil {
		return nil, closeAll(ctx, fmt.Errorf("create positions pool: %w", err), pool, tel)
	}

	ticker := positions.NewRealTicker(cfg.Positions.CheckInterval)
	scanner, err := positions.NewScanner(
		tripRepository,
		queue,
		cfg.Positions.StaleAfter,
		ticker,
		tel.TracerProvider,
		logger,
		now,
	)
	if err != nil {
		return nil, closeAll(ctx, fmt.Errorf("create positions scanner: %w", err), pool, tel)
	}

	handler, err := router.New(router.Handlers{
		Create:         createhttp.New(createuc.New(tripRepository, transactionManager, tripsCreated, logger, now), logger),
		Get:            gethttp.New(getuc.New(tripRepository), logger),
		Finish:         finishhttp.New(finishuc.New(tripRepository, transactionManager, tripsCompleted, logger, now), logger),
		Health:         health.New(pool, cfg.Database.QueryTimeout),
		CreatePosition: createposhttp.New(createposuc.New(tripRepository, transactionManager, queue, positionsSaved, logger, now), logger),
		ListPositions:  listposhttp.New(listposuc.New(tripRepository), logger),
	}, router.Observability{
		TracerProvider: tel.TracerProvider,
		MeterProvider:  tel.MeterProvider,
		Propagator:     tel.Propagator,
	}, logger)
	if err != nil {
		return nil, closeAll(ctx, fmt.Errorf("build router: %w", err), pool, tel)
	}

	server := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           handler,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	return &Container{
		Server:        server,
		Logger:        logger,
		Scanner:       scanner,
		PositionsPool: positionsPool,
		Queue:         queue,
		PushGateway:   pushGateway,
		pool:          pool,
		telemetry:     tel,
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
	var errs []error
	c.closeOnce.Do(func() {
		if c.PushGateway != nil {
			if err := c.PushGateway.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		if err := ClosePool(ctx, c.pool); err != nil {
			errs = append(errs, err)
		}
		if c.telemetry != nil {
			if err := c.telemetry.Shutdown(ctx); err != nil {
				errs = append(errs, err)
			}
		}
	})
	return errors.Join(errs...)
}

// ClosePool closes the PostgreSQL pool within the provided context budget.
func ClosePool(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return nil
	}
	poolClosed := make(chan struct{})
	go func() {
		pool.Close()
		close(poolClosed)
	}()
	select {
	case <-poolClosed:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("close database pool: %w", ctx.Err())
	}
}

func closeAll(ctx context.Context, err error, pool *pgxpool.Pool, tel *telemetry.Telemetry) error {
	pool.Close()
	return errors.Join(err, tel.Shutdown(context.WithoutCancel(ctx)))
}
