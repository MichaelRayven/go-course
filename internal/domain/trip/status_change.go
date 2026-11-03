package trip

import (
	"time"

	api "github.com/MichaelRayven/go-course/internal/generated"
)

type StatusChange struct {
	TripID     api.TripId
	FromStatus *api.TripStatus
	ToStatus   api.TripStatus
	Reason     *string
	ChangedAt  time.Time
}
