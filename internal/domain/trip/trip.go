package trip

import (
	"time"

	"github.com/google/uuid"
)

type Coordinates struct {
	Latitude  float64
	Longitude float64
}

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

type Trip struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	DriverID       uuid.UUID
	StartPoint     Coordinates
	EndPoint       Coordinates
	Price          int64
	Status         Status
	StartedAt      time.Time
	FinishedAt     *time.Time
	LastPositionAt *time.Time
}

func New(id uuid.UUID, userID uuid.UUID, driverID uuid.UUID, startPoint Coordinates, endPoint Coordinates, price int64, startedAt time.Time) (*Trip, error) {
	if id == uuid.Nil {
		return nil, &ValidationError{Field: "ID", Reason: "must not be empty"}
	}
	if userID == uuid.Nil {
		return nil, &ValidationError{Field: "UserID", Reason: "must not be empty"}
	}
	if driverID == uuid.Nil {
		return nil, &ValidationError{Field: "DriverID", Reason: "must not be empty"}
	}
	if endPoint.Latitude < -90 || endPoint.Latitude > 90 {
		return nil, &ValidationError{Field: "EndPoint.Latitude", Reason: "must be between -90 and 90 degrees"}
	}
	if startPoint.Latitude < -90 || startPoint.Latitude > 90 {
		return nil, &ValidationError{Field: "StartPoint.Latitude", Reason: "must be between -90 and 90 degrees"}
	}
	if endPoint.Longitude < -180 || endPoint.Longitude > 180 {
		return nil, &ValidationError{Field: "EndPoint.Longitude", Reason: "must be between -180 and 180 degrees"}
	}
	if startPoint.Longitude < -180 || startPoint.Longitude > 180 {
		return nil, &ValidationError{Field: "StartPoint.Longitude", Reason: "must be between -180 and 180 degrees"}
	}
	if price < 0 {
		return nil, &ValidationError{Field: "Price", Reason: "cannot be negative"}
	}

	return &Trip{
		ID:             id,
		UserID:         userID,
		DriverID:       driverID,
		StartPoint:     startPoint,
		EndPoint:       endPoint,
		Price:          price,
		Status:         StatusActive,
		StartedAt:      startedAt,
		FinishedAt:     nil,
		LastPositionAt: nil,
	}, nil
}

func (t *Trip) Finish(at time.Time) error {
	if t.Status != StatusActive {
		return ErrTripCompleted
	}
	if t.StartedAt.After(at) {
		return ErrFinishBeforeStart
	}

	t.Status = StatusCompleted
	t.FinishedAt = &at
	return nil
}
