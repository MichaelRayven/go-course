//go:generate go tool mockgen -source=${GOFILE} -destination=mocks_test.go -package=${GOPACKAGE}_test

package create

import (
	"context"

	"github.com/google/uuid"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
)

type tripRepository interface {
	Fetch(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
	AddPosition(ctx context.Context, pos trip.Position) (*trip.Position, error)
}

type txManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type positionCounter interface {
	Inc(ctx context.Context)
}

type cancellationNotifier interface {
	Cancel(tripID uuid.UUID)
}
