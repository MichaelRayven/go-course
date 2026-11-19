package trip_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
)

func TestNewPosition(t *testing.T) {
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	validTripID := uuid.New()

	tests := []struct {
		name       string
		tripID     uuid.UUID
		coords     trip.Coordinates
		recordedAt time.Time
		wantErr    bool
		errField   string
	}{
		{
			name:       "valid position in past",
			tripID:     validTripID,
			coords:     trip.Coordinates{Latitude: -90, Longitude: 180},
			recordedAt: now.Add(-10 * time.Minute),
			wantErr:    false,
		},
		{
			name:       "valid position within 1 minute future tolerance",
			tripID:     validTripID,
			coords:     trip.Coordinates{Latitude: 90, Longitude: -180},
			recordedAt: now.Add(60 * time.Second),
			wantErr:    false,
		},
		{
			name:       "empty trip id",
			tripID:     uuid.Nil,
			coords:     trip.Coordinates{Latitude: 90, Longitude: -180},
			recordedAt: now,
			wantErr:    true,
			errField:   "TripID",
		},
		{
			name:       "latitude too low",
			tripID:     validTripID,
			coords:     trip.Coordinates{Latitude: -90.0001, Longitude: 30.3350},
			recordedAt: now,
			wantErr:    true,
			errField:   "Coordinates.Latitude",
		},
		{
			name:       "latitude too high",
			tripID:     validTripID,
			coords:     trip.Coordinates{Latitude: 90.0001, Longitude: 30.3350},
			recordedAt: now,
			wantErr:    true,
			errField:   "Coordinates.Latitude",
		},
		{
			name:       "longitude too low",
			tripID:     validTripID,
			coords:     trip.Coordinates{Latitude: 59.9350, Longitude: -180.0001},
			recordedAt: now,
			wantErr:    true,
			errField:   "Coordinates.Longitude",
		},
		{
			name:       "longitude too high",
			tripID:     validTripID,
			coords:     trip.Coordinates{Latitude: 59.9350, Longitude: 180.0001},
			recordedAt: now,
			wantErr:    true,
			errField:   "Coordinates.Longitude",
		},
		{
			name:       "recorded at more than 1 minute in future",
			tripID:     validTripID,
			coords:     trip.Coordinates{Latitude: 59.9350, Longitude: 30.3350},
			recordedAt: now.Add(61 * time.Second),
			wantErr:    true,
			errField:   "RecordedAt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos, gotErr := trip.NewPosition(tt.tripID, tt.coords, tt.recordedAt, now)

			if tt.wantErr {
				var vErr *trip.ValidationError
				if !errors.As(gotErr, &vErr) {
					t.Fatalf("expected *ValidationError, got %v", gotErr)
				}
				if vErr.Field != tt.errField {
					t.Errorf("expected error field %q, got %q", tt.errField, vErr.Field)
				}
				return
			}

			// Валидация позиции для успешного кейса
			if gotErr != nil {
				t.Fatalf("unexpected error = %v, want = nil", gotErr)
			}
			if pos.TripID != tt.tripID {
				t.Errorf("expected TripID %s, got %s", tt.tripID, pos.TripID)
			}
			if pos.Coordinates != tt.coords {
				t.Errorf("expected Coordinates %+v, got %+v", tt.coords, pos.Coordinates)
			}
		})
	}
}
