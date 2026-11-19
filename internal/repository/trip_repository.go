package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/MichaelRayven/go-course/internal/db"
	api "github.com/MichaelRayven/go-course/internal/generated"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	activeDriverIndex       = "one_trip_active_per_driver_idx"
	postgresUniqueViolation = "23505"
)

var (
	ErrDriverBusy    = errors.New("driver already has an active trip")
	ErrTripNotFound  = errors.New("trip not found")
	ErrTripCompleted = errors.New("trip is already completed")
)

type StatusChange struct {
	TripID     api.TripId
	FromStatus *api.TripStatus
	ToStatus   api.TripStatus
	Reason     *string
	ChangedAt  time.Time
}

type TripRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewTripRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *TripRepository {
	return &TripRepository{
		pool:         pool,
		queryTimeout: queryTimeout,
	}
}

func (r *TripRepository) Create(ctx context.Context, trip api.Trip) (api.Trip, error) {
	query, args, err := squirrel.Insert("trips").
		Columns(
			"id",
			"user_id",
			"driver_id",
			"start_latitude",
			"start_longitude",
			"end_latitude",
			"end_longitude",
			"price",
			"status",
			"started_at",
			"finished_at",
		).
		Values(
			trip.Id,
			trip.UserId,
			trip.DriverId,
			trip.StartPoint.Latitude,
			trip.StartPoint.Longitude,
			trip.EndPoint.Latitude,
			trip.EndPoint.Longitude,
			trip.Price,
			trip.Status,
			trip.StartedAt,
			trip.FinishedAt,
		).
		Suffix("RETURNING " + tripColumns()).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return api.Trip{}, fmt.Errorf("build create trip query: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	created, err := scanTrip(db.ExecutorFromContext(queryCtx, r.pool).QueryRow(queryCtx, query, args...))
	if err == nil {
		return created, nil
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) &&
		pgErr.Code == postgresUniqueViolation &&
		pgErr.ConstraintName == activeDriverIndex {
		return api.Trip{}, ErrDriverBusy
	}

	return api.Trip{}, fmt.Errorf("create trip: %w", err)
}

func (r *TripRepository) AddStatusHistory(ctx context.Context, change StatusChange) error {
	query, args, err := squirrel.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason", "changed_at").
		Values(change.TripID, change.FromStatus, change.ToStatus, change.Reason, change.ChangedAt).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build add trip status history query: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err := db.ExecutorFromContext(queryCtx, r.pool).Exec(queryCtx, query, args...); err != nil {
		return fmt.Errorf("add trip status history: %w", err)
	}

	return nil
}

func (r *TripRepository) GetByID(ctx context.Context, id api.TripId) (api.Trip, error) {
	query, args, err := squirrel.Select(tripColumns()).
		From("trips").
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return api.Trip{}, fmt.Errorf("build get trip query: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	trip, err := scanTrip(db.ExecutorFromContext(queryCtx, r.pool).QueryRow(queryCtx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return api.Trip{}, ErrTripNotFound
	}
	if err != nil {
		return api.Trip{}, fmt.Errorf("get trip: %w", err)
	}

	return trip, nil
}

func (r *TripRepository) Finish(ctx context.Context, id api.TripId, finishedAt time.Time) (api.Trip, error) {
	query, args, err := squirrel.Update("trips").
		Set("status", api.Completed).
		Set("finished_at", finishedAt).
		Set("updated_at", finishedAt).
		Where(squirrel.Eq{
			"id":     id,
			"status": api.Active,
		}).
		Suffix("RETURNING " + tripColumns()).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return api.Trip{}, fmt.Errorf("build finish trip query: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	trip, err := scanTrip(db.ExecutorFromContext(queryCtx, r.pool).QueryRow(queryCtx, query, args...))
	if err == nil {
		return trip, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return api.Trip{}, fmt.Errorf("finish trip: %w", err)
	}

	if _, err := r.GetByID(ctx, id); err != nil {
		return api.Trip{}, err
	}

	return api.Trip{}, ErrTripCompleted
}

func tripColumns() string {
	return "id, user_id, driver_id, start_latitude, start_longitude, " +
		"end_latitude, end_longitude, price, status, started_at, finished_at, NULL::timestamptz"
}

func scanTrip(row pgx.Row) (api.Trip, error) {
	var trip api.Trip
	err := row.Scan(
		&trip.Id,
		&trip.UserId,
		&trip.DriverId,
		&trip.StartPoint.Latitude,
		&trip.StartPoint.Longitude,
		&trip.EndPoint.Latitude,
		&trip.EndPoint.Longitude,
		&trip.Price,
		&trip.Status,
		&trip.StartedAt,
		&trip.FinishedAt,
		&trip.LastPositionAt,
	)
	if err != nil {
		return api.Trip{}, err
	}

	return trip, nil
}
