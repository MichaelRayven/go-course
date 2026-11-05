package create

import (
	"github.com/google/uuid"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	api "github.com/MichaelRayven/go-course/internal/generated"
	createuc "github.com/MichaelRayven/go-course/internal/usecase/trip/create"
)

// request mirrors api.TripData with pointer fields, so a missing field can be
// told apart from a zero value. Range checks are the domain's job (trip.New).
type request struct {
	UserID     *uuid.UUID   `json:"user_id"`
	DriverID   *uuid.UUID   `json:"driver_id"`
	StartPoint *coordinates `json:"start_point"`
	EndPoint   *coordinates `json:"end_point"`
	Price      *int64       `json:"price"`
}

type coordinates struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

// toInput checks that every required field is present and builds the usecase input.
func (req request) toInput() (createuc.Input, error) {
	switch {
	case req.UserID == nil:
		return createuc.Input{}, missing("user_id")
	case req.DriverID == nil:
		return createuc.Input{}, missing("driver_id")
	case req.Price == nil:
		return createuc.Input{}, missing("price")
	}

	start, err := req.StartPoint.toDomain("start_point")
	if err != nil {
		return createuc.Input{}, err
	}

	end, err := req.EndPoint.toDomain("end_point")
	if err != nil {
		return createuc.Input{}, err
	}

	return createuc.Input{
		UserID:     *req.UserID,
		DriverID:   *req.DriverID,
		StartPoint: start,
		EndPoint:   end,
		Price:      *req.Price,
	}, nil
}

func (c *coordinates) toDomain(field string) (trip.Coordinates, error) {
	switch {
	case c == nil:
		return trip.Coordinates{}, missing(field)
	case c.Latitude == nil:
		return trip.Coordinates{}, missing(field + ".latitude")
	case c.Longitude == nil:
		return trip.Coordinates{}, missing(field + ".longitude")
	}

	return trip.Coordinates{Latitude: *c.Latitude, Longitude: *c.Longitude}, nil
}

func missing(field string) error {
	return &trip.ValidationError{Field: field, Reason: "is required"}
}

func toAPI(t trip.Trip) api.Trip {
	return api.Trip{
		Id:             t.ID,
		UserId:         t.UserID,
		DriverId:       t.DriverID,
		Price:          t.Price,
		Status:         api.TripStatus(t.Status),
		StartPoint:     api.Coordinates{Latitude: t.StartPoint.Latitude, Longitude: t.StartPoint.Longitude},
		EndPoint:       api.Coordinates{Latitude: t.EndPoint.Latitude, Longitude: t.EndPoint.Longitude},
		StartedAt:      t.StartedAt,
		FinishedAt:     t.FinishedAt,
		LastPositionAt: t.LastPositionAt,
	}
}
