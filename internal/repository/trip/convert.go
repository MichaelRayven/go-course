package trip

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
)

type StaleTrip struct {
	ID       uuid.UUID
	DriverID uuid.UUID
	LastSeen time.Time
}

func tripColumns() string {
	return "id, user_id, driver_id, start_latitude, start_longitude, " +
		"end_latitude, end_longitude, price, status, started_at, finished_at, last_position_at"
}

func positionColumns() string {
	return "id, trip_id, latitude, longitude, recorded_at, created_at"
}

func scanTrip(row pgx.Row) (*trip.Trip, error) {
	var t trip.Trip
	err := row.Scan(
		&t.ID,
		&t.UserID,
		&t.DriverID,
		&t.StartPoint.Latitude,
		&t.StartPoint.Longitude,
		&t.EndPoint.Latitude,
		&t.EndPoint.Longitude,
		&t.Price,
		&t.Status,
		&t.StartedAt,
		&t.FinishedAt,
		&t.LastPositionAt,
	)
	if err != nil {
		return nil, err
	}

	return &t, nil
}

func scanPosition(row pgx.Row) (*trip.Position, error) {
	var p trip.Position
	err := row.Scan(
		&p.ID,
		&p.TripID,
		&p.Coordinates.Latitude,
		&p.Coordinates.Longitude,
		&p.RecordedAt,
		&p.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	return &p, nil
}
