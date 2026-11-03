package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/MichaelRayven/go-course/internal/domain/trip"
	"github.com/MichaelRayven/go-course/internal/infra/postgres"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
