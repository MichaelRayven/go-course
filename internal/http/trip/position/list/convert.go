package list

import (
	"github.com/MichaelRayven/go-course/internal/domain/trip"
	api "github.com/MichaelRayven/go-course/internal/generated"
)

func toAPI(positions []trip.Position) api.TripPositionsResponse {
	apiPositions := make([]api.TripPosition, 0, len(positions))
	for _, p := range positions {
		apiPositions = append(apiPositions, api.TripPosition{
			Id:     p.ID,
			TripId: p.TripID,
			Coordinates: api.Coordinates{
				Latitude:  p.Coordinates.Latitude,
				Longitude: p.Coordinates.Longitude,
			},
			RecordedAt: p.RecordedAt,
		})
	}

	return api.TripPositionsResponse{
		Positions: apiPositions,
	}
}
