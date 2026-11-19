package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	"github.com/MichaelRayven/go-course/internal/infra/postgres"
)

func (r *Repository) Fetch(ctx context.Context, id uuid.UUID) (*trip.Trip, error) {
	query, args, err := squirrel.Select(tripColumns()).
		From("trips").
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build get trip query: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	t, err := scanTrip(postgres.ExecutorFromContext(queryCtx, r.pool).QueryRow(queryCtx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, trip.ErrTripNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("fetch trip: %w", err)
	}

	return t, nil
}

func (r *Repository) FetchPositions(ctx context.Context, tripID uuid.UUID) ([]trip.Position, error) {
	query, args, err := squirrel.Select(positionColumns()).
		From("trip_positions").
		Where(squirrel.Eq{"trip_id": tripID}).
		OrderBy("recorded_at ASC", "id ASC").
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build fetch positions query: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	rows, err := postgres.ExecutorFromContext(queryCtx, r.pool).Query(queryCtx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch positions: %w", err)
	}
	defer rows.Close()

	positions := make([]trip.Position, 0)
	for rows.Next() {
		pos, err := scanPosition(rows)
		if err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}
		positions = append(positions, *pos)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate positions: %w", err)
	}

	return positions, nil
}

func (r *Repository) FetchStaleTrips(ctx context.Context, staleThreshold time.Time, limit int) ([]StaleTrip, error) {
	builder := squirrel.Select("id", "driver_id", "COALESCE(last_position_at, started_at) AS last_seen").
		From("trips").
		Where(squirrel.Eq{"status": trip.StatusActive}).
		Where(squirrel.Expr("COALESCE(last_position_at, started_at) < ?", staleThreshold)).
		// trips where position wasn't being updated the longest are first to get pinged
		OrderBy("COALESCE(last_position_at, started_at) ASC", "id ASC").
		PlaceholderFormat(squirrel.Dollar)

	if limit > 0 {
		builder = builder.Limit(uint64(limit))
	}

	query, args, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("build fetch stale trips query: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	rows, err := postgres.ExecutorFromContext(queryCtx, r.pool).Query(queryCtx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("fetch stale trips: %w", err)
	}
	defer rows.Close()

	staleTrips := make([]StaleTrip, 0)
	for rows.Next() {
		var st StaleTrip
		if err := rows.Scan(&st.ID, &st.DriverID, &st.LastSeen); err != nil {
			return nil, fmt.Errorf("scan stale trip: %w", err)
		}
		staleTrips = append(staleTrips, st)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stale trips: %w", err)
	}

	return staleTrips, nil
}
