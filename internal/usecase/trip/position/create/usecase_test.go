package create_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	"github.com/MichaelRayven/go-course/internal/usecase/trip/position/create"
)

var (
	fixed = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	errDB = errors.New("db error")
)

type mocks struct {
	repo     *MocktripRepository
	tx       *MocktxManager
	notifier *MockcancellationNotifier
	counter  *MockpositionCounter
}

func setup(t *testing.T) (*create.Usecase, mocks) {
	t.Helper()

	ctrl := gomock.NewController(t)
	m := mocks{
		repo:     NewMocktripRepository(ctrl),
		tx:       NewMocktxManager(ctrl),
		notifier: NewMockcancellationNotifier(ctrl),
		counter:  NewMockpositionCounter(ctrl),
	}
	now := func() time.Time { return fixed }

	return create.New(m.repo, m.tx, m.notifier, m.counter, slog.New(slog.DiscardHandler), now), m
}

func (m mocks) runInTx() {
	m.tx.EXPECT().Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		})
}

func validInput(tripID uuid.UUID) create.Input {
	return create.Input{
		TripID:      tripID,
		Coordinates: trip.Coordinates{Latitude: 59.9350, Longitude: 30.3350},
		RecordedAt:  fixed.Add(-10 * time.Second),
	}
}

func activeTrip(id uuid.UUID) *trip.Trip {
	return &trip.Trip{
		ID:        id,
		UserID:    uuid.New(),
		DriverID:  uuid.New(),
		Status:    trip.StatusActive,
		StartedAt: fixed.Add(-1 * time.Hour),
	}
}

func TestExecuteSuccess(t *testing.T) {
	uc, m := setup(t)
	ctx := context.Background()
	tripID := uuid.New()
	input := validInput(tripID)
	tr := activeTrip(tripID)

	savedPos := &trip.Position{
		ID:          42,
		TripID:      tripID,
		Coordinates: input.Coordinates,
		RecordedAt:  input.RecordedAt,
		CreatedAt:   fixed,
	}

	m.runInTx()
	m.repo.EXPECT().Fetch(gomock.Any(), tripID).Return(tr, nil)
	m.repo.EXPECT().AddPosition(gomock.Any(), gomock.Any()).Return(savedPos, nil)
	m.notifier.EXPECT().Cancel(tripID)
	m.counter.EXPECT().Inc(ctx)

	got, err := uc.Execute(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != 42 {
		t.Errorf("got position ID %d, want 42", got.ID)
	}
}

func TestExecuteTripNotFound(t *testing.T) {
	uc, m := setup(t)
	ctx := context.Background()
	tripID := uuid.New()
	input := validInput(tripID)

	m.runInTx()
	m.repo.EXPECT().Fetch(gomock.Any(), tripID).Return(nil, trip.ErrTripNotFound)

	got, err := uc.Execute(ctx, input)
	if !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("expected ErrTripNotFound, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil position, got %+v", got)
	}
}

func TestExecuteTripCompleted(t *testing.T) {
	uc, m := setup(t)
	ctx := context.Background()
	tripID := uuid.New()
	input := validInput(tripID)

	finishedAt := fixed.Add(-10 * time.Minute)
	completedTrip := &trip.Trip{
		ID:         tripID,
		Status:     trip.StatusCompleted,
		StartedAt:  fixed.Add(-1 * time.Hour),
		FinishedAt: &finishedAt,
	}

	m.runInTx()
	m.repo.EXPECT().Fetch(gomock.Any(), tripID).Return(completedTrip, nil)

	got, err := uc.Execute(ctx, input)
	if !errors.Is(err, trip.ErrTripCompleted) {
		t.Fatalf("expected ErrTripCompleted, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil position, got %+v", got)
	}
}

func TestExecuteAddPositionError(t *testing.T) {
	uc, m := setup(t)
	ctx := context.Background()
	tripID := uuid.New()
	input := validInput(tripID)
	tr := activeTrip(tripID)

	m.runInTx()
	m.repo.EXPECT().Fetch(gomock.Any(), tripID).Return(tr, nil)
	m.repo.EXPECT().AddPosition(gomock.Any(), gomock.Any()).Return(nil, errDB)

	got, err := uc.Execute(ctx, input)
	if !errors.Is(err, errDB) {
		t.Fatalf("expected errDB, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil position, got %+v", got)
	}
}
