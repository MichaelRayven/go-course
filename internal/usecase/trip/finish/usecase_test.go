package finish_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	"github.com/MichaelRayven/go-course/internal/usecase/trip/finish"
)

var (
	startedAt = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	fixed     = time.Date(2026, 1, 1, 13, 0, 0, 0, time.UTC)
	errDB     = errors.New("db down")
)

type mocks struct {
	repo *MocktripRepository
	tx   *MocktxManager
}

// setup returns the usecase under test and its mocks. The clock always
// returns fixed, so FinishedAt and ChangedAt can be asserted exactly.
func setup(t *testing.T) (*finish.Usecase, mocks) {
	t.Helper()

	ctrl := gomock.NewController(t)
	m := mocks{
		repo: NewMocktripRepository(ctrl),
		tx:   NewMocktxManager(ctrl),
	}
	now := func() time.Time { return fixed }

	return finish.New(m.repo, m.tx, slog.New(slog.DiscardHandler), now), m
}

// runInTx makes the tx mock behave like the real manager: it runs the
// callback and returns its error.
func (m mocks) runInTx() {
	m.tx.EXPECT().Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		})
}

func activeTrip(t *testing.T) *trip.Trip {
	t.Helper()

	tr, err := trip.New(
		uuid.UUID{15: 1},
		uuid.UUID{15: 2},
		uuid.UUID{15: 3},
		trip.Coordinates{Latitude: 10, Longitude: 20},
		trip.Coordinates{Latitude: 30, Longitude: 40},
		100,
		startedAt,
	)
	if err != nil {
		t.Fatalf("trip.New: %v", err)
	}

	return tr
}

func TestExecuteFinishesActiveTrip(t *testing.T) {
	uc, m := setup(t)
	tr := activeTrip(t)

	m.runInTx()
	m.repo.EXPECT().Fetch(gomock.Any(), tr.ID).Return(tr, nil)
	m.repo.EXPECT().Finish(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, saved trip.Trip) error {
			if saved.Status != trip.StatusCompleted {
				t.Errorf("persisted status = %q, want %q", saved.Status, trip.StatusCompleted)
			}
			return nil
		})
	m.repo.EXPECT().AddStatusHistory(gomock.Any(), trip.StatusChange{
		TripID:     tr.ID,
		FromStatus: new(trip.StatusActive),
		ToStatus:   trip.StatusCompleted,
		ChangedAt:  fixed,
	}).Return(nil)

	got, err := uc.Execute(context.Background(), tr.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Status != trip.StatusCompleted || got.FinishedAt == nil || !got.FinishedAt.Equal(fixed) {
		t.Errorf("got status %q, finishedAt %v; want completed at %v", got.Status, got.FinishedAt, fixed)
	}
}

func TestExecuteErrors(t *testing.T) {
	id := uuid.UUID{15: 1}

	tests := []struct {
		name    string
		expect  func(t *testing.T, m mocks)
		wantErr error
	}{
		{
			name: "trip not found",
			expect: func(_ *testing.T, m mocks) {
				m.runInTx()
				m.repo.EXPECT().Fetch(gomock.Any(), id).Return(nil, trip.ErrTripNotFound)
			},
			wantErr: trip.ErrTripNotFound,
		},
		{
			// The domain rule rejects it before anything is written.
			name: "trip already completed",
			expect: func(t *testing.T, m mocks) {
				tr := activeTrip(t)
				if err := tr.Finish(startedAt.Add(time.Minute)); err != nil {
					t.Fatalf("setup finish: %v", err)
				}
				m.runInTx()
				m.repo.EXPECT().Fetch(gomock.Any(), id).Return(tr, nil)
			},
			wantErr: trip.ErrTripCompleted,
		},
		{
			// Another request completed the trip between Fetch and the UPDATE.
			name: "trip completed concurrently",
			expect: func(t *testing.T, m mocks) {
				m.runInTx()
				m.repo.EXPECT().Fetch(gomock.Any(), id).Return(activeTrip(t), nil)
				m.repo.EXPECT().Finish(gomock.Any(), gomock.Any()).Return(trip.ErrTripCompleted)
			},
			wantErr: trip.ErrTripCompleted,
		},
		{
			name: "repository fails to persist",
			expect: func(t *testing.T, m mocks) {
				m.runInTx()
				m.repo.EXPECT().Fetch(gomock.Any(), id).Return(activeTrip(t), nil)
				m.repo.EXPECT().Finish(gomock.Any(), gomock.Any()).Return(errDB)
			},
			wantErr: errDB,
		},
		{
			name: "status history insert fails",
			expect: func(t *testing.T, m mocks) {
				m.runInTx()
				m.repo.EXPECT().Fetch(gomock.Any(), id).Return(activeTrip(t), nil)
				m.repo.EXPECT().Finish(gomock.Any(), gomock.Any()).Return(nil)
				m.repo.EXPECT().AddStatusHistory(gomock.Any(), gomock.Any()).Return(errDB)
			},
			wantErr: errDB,
		},
		{
			// Nothing inside the transaction may run.
			name: "transaction cannot start",
			expect: func(_ *testing.T, m mocks) {
				m.tx.EXPECT().Do(gomock.Any(), gomock.Any()).Return(errDB)
			},
			wantErr: errDB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc, m := setup(t)
			tt.expect(t, m)

			got, err := uc.Execute(context.Background(), id)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
			if got != nil {
				t.Errorf("got trip %+v, want nil", got)
			}
		})
	}
}
