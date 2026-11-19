package create

import (
	"time"

	"github.com/google/uuid"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	api "github.com/MichaelRayven/go-course/internal/generated"
	createuc "github.com/MichaelRayven/go-course/internal/usecase/trip/position/create"
)

type coordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type request struct {
	Coordinates coordinates `json:"coordinates"`
	RecordedAt  time.Time   `json:"recorded_at"`
}

func (req request) toInput(tripID uuid.UUID) createuc.Input {
	return createuc.Input{
		TripID: tripID,
		Coordinates: trip.Coordinates{
			Latitude:  req.Coordinates.Latitude,
			Longitude: req.Coordinates.Longitude,
		},
		RecordedAt: req.RecordedAt,
	}
}

func toAPI(pos trip.Position) api.TripPosition {
	return api.TripPosition{
		Id:     pos.ID,
		TripId: pos.TripID,
		Coordinates: api.Coordinates{
			Latitude:  pos.Coordinates.Latitude,
			Longitude: pos.Coordinates.Longitude,
		},
		RecordedAt: pos.RecordedAt,
	}
}
