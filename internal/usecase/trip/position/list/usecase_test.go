package list_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	"github.com/MichaelRayven/go-course/internal/usecase/trip/position/list"
)

var errDB = errors.New("db error")

func TestExecuteSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	uc := list.New(repo)
	ctx := context.Background()
	tripID := uuid.New()

	tr := &trip.Trip{
		ID:        tripID,
		Status:    trip.StatusActive,
		StartedAt: time.Now().UTC(),
	}

	wantPositions := []trip.Position{
		{
			ID:          1,
			TripID:      tripID,
			Coordinates: trip.Coordinates{Latitude: 59.93, Longitude: 30.33},
			RecordedAt:  time.Now().UTC().Add(-10 * time.Second),
		},
		{
			ID:          2,
			TripID:      tripID,
			Coordinates: trip.Coordinates{Latitude: 59.94, Longitude: 30.34},
			RecordedAt:  time.Now().UTC(),
		},
	}

	repo.EXPECT().Fetch(gomock.Any(), tripID).Return(tr, nil)
	repo.EXPECT().FetchPositions(gomock.Any(), tripID).Return(wantPositions, nil)

	got, err := uc.Execute(ctx, tripID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(got))
	}
	if got[0].ID != 1 || got[1].ID != 2 {
		t.Errorf("got %+v, want %+v", got, wantPositions)
	}
}

func TestExecuteTripNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	uc := list.New(repo)
	ctx := context.Background()
	tripID := uuid.New()

	repo.EXPECT().Fetch(gomock.Any(), tripID).Return(nil, trip.ErrTripNotFound)

	got, err := uc.Execute(ctx, tripID)
	if !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("expected ErrTripNotFound, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil positions, got %+v", got)
	}
}

func TestExecuteFetchPositionsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := NewMocktripRepository(ctrl)
	uc := list.New(repo)
	ctx := context.Background()
	tripID := uuid.New()

	tr := &trip.Trip{
		ID:        tripID,
		Status:    trip.StatusActive,
		StartedAt: time.Now().UTC(),
	}

	repo.EXPECT().Fetch(gomock.Any(), tripID).Return(tr, nil)
	repo.EXPECT().FetchPositions(gomock.Any(), tripID).Return(nil, errDB)

	got, err := uc.Execute(ctx, tripID)
	if !errors.Is(err, errDB) {
		t.Fatalf("expected errDB, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil positions, got %+v", got)
	}
}
