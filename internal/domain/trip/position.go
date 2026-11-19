package trip

import (
	"time"

	"github.com/google/uuid"
)

type Position struct {
	ID          int64
	TripID      uuid.UUID
	Coordinates Coordinates
	RecordedAt  time.Time
	CreatedAt   time.Time
}

func NewPosition(tripID uuid.UUID, coords Coordinates, recordedAt time.Time, createdAt time.Time) (*Position, error) {
	if tripID == uuid.Nil {
		return nil, &ValidationError{Field: "TripID", Reason: reasonEmpty}
	}

	if coords.Latitude < -90 || coords.Latitude > 90 {
		return nil, &ValidationError{
			Field:  "Coordinates.Latitude",
			Reason: "must be between -90 and 90 degrees",
		}
	}
	if coords.Longitude < -180 || coords.Longitude > 180 {
		return nil, &ValidationError{
			Field:  "Coordinates.Longitude",
			Reason: "must be between -180 and 180 degrees",
		}
	}
	// Точка не может быть в будущем более чем на 1 минуту относительно текущего времени
	if recordedAt.After(createdAt.Add(1 * time.Minute)) {
		return nil, &ValidationError{
			Field:  "RecordedAt",
			Reason: "cannot be in the future by more than 1 minute",
		}
	}
	return &Position{
		TripID:      tripID,
		Coordinates: coords,
		RecordedAt:  recordedAt,
		CreatedAt:   createdAt,
	}, nil
}
