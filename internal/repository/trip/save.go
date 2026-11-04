package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/MichaelRayven/go-course/internal/domain/trip"
	"github.com/MichaelRayven/go-course/internal/infra/postgres"
	"github.com/jackc/pgx/v5/pgconn"
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
