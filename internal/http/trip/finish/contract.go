package finish

import (
	"context"

	"github.com/google/uuid"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
)

//go:generate go tool mockgen -source=${GOFILE} -destination=mocks_test.go -package=${GOPACKAGE}_test

type usecase interface {
	Execute(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
}
