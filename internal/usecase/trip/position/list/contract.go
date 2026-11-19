//go:generate go tool mockgen -source=${GOFILE} -destination=mocks_test.go -package=${GOPACKAGE}_test

package list

import (
	"context"

	"github.com/google/uuid"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
)

type tripRepository interface {
	Fetch(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
	FetchPositions(ctx context.Context, tripID uuid.UUID) ([]trip.Position, error)
}
