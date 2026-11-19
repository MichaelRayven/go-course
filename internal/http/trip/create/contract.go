//go:generate go tool mockgen -source=${GOFILE} -destination=mocks_test.go -package=${GOPACKAGE}_test

package create

import (
	"context"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	createuc "github.com/MichaelRayven/go-course/internal/usecase/trip/create"
)

type usecase interface {
	Execute(ctx context.Context, i createuc.Input) (*trip.Trip, error)
}
