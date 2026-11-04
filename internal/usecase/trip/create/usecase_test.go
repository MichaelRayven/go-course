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
	"github.com/MichaelRayven/go-course/internal/usecase/trip/create"
)

var (
	fixed = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	errDB = errors.New("db down")
)

type mocks struct {
	repo *MocktripRepository
	tx   *MocktxManager
}

func setup(t *testing.T) (*create.Usecase, mocks) {
	t.Helper()

	ctrl := gomock.NewController(t)
	m := mocks{
		repo: NewMocktripRepository(ctrl),
		tx:   NewMocktxManager(ctrl),
	}
	now := func() time.Time { return fixed }

	return create.New(m.repo, m.tx, slog.New(slog.DiscardHandler), now), m
}

func (m mocks) runInTx() {
	m.tx.EXPECT().Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		})
}

func validInput() create.Input {
	return create.Input{
		UserID:     uuid.UUID{15: 1},
		DriverID:   uuid.UUID{15: 2},
		StartPoint: trip.Coordinates{Latitude: 10, Longitude: 20},
		EndPoint:   trip.Coordinates{Latitude: 30, Longitude: 40},
		Price:      100,
	}
}

func TestExecuteCreatesTrip(t *testing.T) {
	uc, m := setup(t)
	in := validInput()

	var saved trip.Trip
	m.runInTx()
	m.repo.EXPECT().Save(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, tr trip.Trip) error {
			saved = tr
			return nil
		})
	m.repo.EXPECT().AddStatusHistory(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, c trip.StatusChange) error {
			if c.TripID != saved.ID || c.FromStatus != nil || c.ToStatus != trip.StatusActive || !c.ChangedAt.Equal(fixed) {
				t.Errorf("status change = %+v, want initial active entry for %v at %v", c, saved.ID, fixed)
			}
			return nil
		})

	got, err := uc.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.ID == uuid.Nil {
		t.Error("trip ID was not generated")
	}
	if got.ID != saved.ID {
		t.Errorf("returned ID %v differs from saved ID %v", got.ID, saved.ID)
	}
	if got.UserID != in.UserID || got.DriverID != in.DriverID {
		t.Errorf("user/driver = %v/%v, want %v/%v", got.UserID, got.DriverID, in.UserID, in.DriverID)
	}
	if got.StartPoint != in.StartPoint || got.EndPoint != in.EndPoint || got.Price != in.Price {
		t.Errorf("got %+v, want points and price from %+v", got, in)
	}
	if got.Status != trip.StatusActive || !got.StartedAt.Equal(fixed) || got.FinishedAt != nil {
		t.Errorf("got status %q started %v finished %v; want active at %v", got.Status, got.StartedAt, got.FinishedAt, fixed)
	}
}

// Invalid input is rejected by the domain before a transaction is opened, so
// no mock call is expected at all.
func TestExecuteRejectsInvalidInput(t *testing.T) {
	uc, _ := setup(t)
	in := validInput()
	in.Price = -1

	got, err := uc.Execute(context.Background(), in)

	var vErr *trip.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("error = %v, want *trip.ValidationError", err)
	}
	if vErr.Field != "Price" {
		t.Errorf("field = %q, want %q", vErr.Field, "Price")
	}
	if got != nil {
		t.Errorf("got trip %+v, want nil", got)
	}
}

func TestExecuteErrors(t *testing.T) {
	tests := []struct {
		name    string
		expect  func(m mocks)
		wantErr error
	}{
		{
			name: "driver busy",
			expect: func(m mocks) {
				m.runInTx()
				m.repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(trip.ErrDriverBusy)
			},
			wantErr: trip.ErrDriverBusy,
		},
		{
			name: "save fails",
			expect: func(m mocks) {
				m.runInTx()
				m.repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(errDB)
			},
			wantErr: errDB,
		},
		{
			name: "status history insert fails",
			expect: func(m mocks) {
				m.runInTx()
				m.repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
				m.repo.EXPECT().AddStatusHistory(gomock.Any(), gomock.Any()).Return(errDB)
			},
			wantErr: errDB,
		},
		{
			name: "transaction cannot start",
			expect: func(m mocks) {
				m.tx.EXPECT().Do(gomock.Any(), gomock.Any()).Return(errDB)
			},
			wantErr: errDB,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc, m := setup(t)
			tt.expect(m)

			got, err := uc.Execute(context.Background(), validInput())

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
			if got != nil {
				t.Errorf("got trip %+v, want nil", got)
			}
		})
	}
}
