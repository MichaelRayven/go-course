//go:generate go tool mockgen -source=${GOFILE} -destination=mocks_test.go -package=${GOPACKAGE}_test

package create

import (
	"context"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
)

type tripRepository interface {
	Save(ctx context.Context, t trip.Trip) error
	AddStatusHistory(ctx context.Context, entry trip.StatusChange) error
}

type txManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type tripCounter interface {
	Inc(ctx context.Context)
}
