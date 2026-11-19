package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	"github.com/MichaelRayven/go-course/internal/infra/postgres"
)

func (r *Repository) Save(ctx context.Context, t trip.Trip) error {
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
			t.ID,
			t.UserID,
			t.DriverID,
			t.StartPoint.Latitude,
			t.StartPoint.Longitude,
			t.EndPoint.Latitude,
			t.EndPoint.Longitude,
			t.Price,
			t.Status,
			t.StartedAt,
			t.FinishedAt,
		).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build save trip query: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	_, err = postgres.ExecutorFromContext(queryCtx, r.pool).Exec(queryCtx, query, args...)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) &&
		pgErr.Code == postgresUniqueViolation &&
		pgErr.ConstraintName == activeDriverIndex {
		return trip.ErrDriverBusy
	}

	if err != nil {
		return fmt.Errorf("save trip: %w", err)
	}

	return nil
}

func (r *Repository) Finish(ctx context.Context, t trip.Trip) error {
	query, args, err := squirrel.Update("trips").
		Set("status", t.Status).
		Set("finished_at", t.FinishedAt).
		Set("updated_at", t.FinishedAt).
		Where(squirrel.Eq{
			"id":     t.ID,
			"status": trip.StatusActive,
		}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build finish trip query: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	tag, err := postgres.ExecutorFromContext(queryCtx, r.pool).Exec(queryCtx, query, args...)
	if err != nil {
		return fmt.Errorf("finish trip: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return trip.ErrTripCompleted
	}

	return nil
}

func (r *Repository) AddStatusHistory(ctx context.Context, change trip.StatusChange) error {
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

	if _, err := postgres.ExecutorFromContext(queryCtx, r.pool).Exec(queryCtx, query, args...); err != nil {
		return fmt.Errorf("add trip status history: %w", err)
	}

	return nil
}

func (r *Repository) AddPosition(ctx context.Context, pos trip.Position) (*trip.Position, error) {
	queryCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	executor := postgres.ExecutorFromContext(queryCtx, r.pool)

	insertQuery, args, err := squirrel.Insert("trip_positions").
		Columns("trip_id", "latitude", "longitude", "recorded_at", "created_at").
		Values(pos.TripID, pos.Coordinates.Latitude, pos.Coordinates.Longitude, pos.RecordedAt, pos.CreatedAt).
		Suffix("RETURNING id").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build insert trip position query: %w", err)
	}

	var id int64
	if err := executor.QueryRow(queryCtx, insertQuery, args...).Scan(&id); err != nil {
		return nil, fmt.Errorf("insert trip position: %w", err)
	}

	updateQuery, args, err := squirrel.Update("trips").
		Set("last_position_at", squirrel.Expr("CASE WHEN last_position_at IS NULL THEN ? ELSE GREATEST(last_position_at, ?) END", pos.RecordedAt, pos.RecordedAt)).
		Set("updated_at", squirrel.Expr("now()")).
		Where(squirrel.Eq{
			"id":     pos.TripID,
			"status": trip.StatusActive,
		}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build update trip last_position_at query: %w", err)
	}

	tag, err := executor.Exec(queryCtx, updateQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("update trip last_position_at: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, trip.ErrTripCompleted
	}

	pos.ID = id
	return &pos, nil
}
