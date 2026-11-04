package get

import (
	"context"

	"github.com/google/uuid"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
)

//go:generate go tool mockgen -source=${GOFILE} -destination=mocks_test.go -package=${GOPACKAGE}_test

type tripRepository interface {
	Fetch(ctx context.Context, id uuid.UUID) (*trip.Trip, error)
}
