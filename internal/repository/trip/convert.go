package trip

import (
	"github.com/jackc/pgx/v5"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
)

func tripColumns() string {
	// `NULL::timestamptz` is a placeholder value for LastPositionAt field
	// It will be used later
	return "id, user_id, driver_id, start_latitude, start_longitude, " +
		"end_latitude, end_longitude, price, status, started_at, finished_at, NULL::timestamptz"
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
