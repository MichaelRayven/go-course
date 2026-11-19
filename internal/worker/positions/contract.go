//go:generate go tool mockgen -source=${GOFILE} -destination=mocks_test.go -package=${GOPACKAGE}_test

package positions

import (
	"context"
	"time"

	"github.com/google/uuid"

	triprepo "github.com/MichaelRayven/go-course/internal/repository/trip"
)

type pushGateway interface {
	SendPositionRequest(ctx context.Context, recipientID, tripID uuid.UUID) error
}

type staleTripRepository interface {
	FetchStaleTrips(ctx context.Context, staleThreshold time.Time, limit int) ([]triprepo.StaleTrip, error)
}
