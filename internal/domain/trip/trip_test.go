package trip_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
)

type newArgs struct {
	id, userID, driverID uuid.UUID
	start, end           trip.Coordinates
	price                int64
	startedAt            time.Time
}

func validNewArgs() newArgs {
	return newArgs{
		id:        uuid.UUID{15: 1},
		userID:    uuid.UUID{15: 2},
		driverID:  uuid.UUID{15: 3},
		start:     trip.Coordinates{Latitude: 90, Longitude: -180},
		end:       trip.Coordinates{Latitude: -90, Longitude: 180},
		price:     0,
		startedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestNewValid(t *testing.T) {
	a := validNewArgs()

	got, gotErr := trip.New(a.id, a.userID, a.driverID, a.start, a.end, a.price, a.startedAt)
	if gotErr != nil {
		t.Fatalf("unexpected error: %v", gotErr)
	}

	want := trip.Trip{
		ID:         a.id,
		UserID:     a.userID,
		DriverID:   a.driverID,
		StartPoint: a.start,
		EndPoint:   a.end,
		Price:      a.price,
		Status:     trip.StatusActive,
		StartedAt:  a.startedAt,
		// FinishedAt and LastPositionAt stay nil
	}

	if !reflect.DeepEqual(*got, want) {
		t.Errorf("trip.New() = %+v, want %+v", *got, want)
	}
}

func TestNewInvalid(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(a *newArgs) // Breaks one field
		wantField string           // Expected ValidationError.Field
	}{
		{
			name: "zero id",
			modify: func(a *newArgs) {
				a.id = uuid.Nil
			},
			wantField: "ID",
		},
		{
			name: "zero user id",
			modify: func(a *newArgs) {
				a.userID = uuid.Nil
			},
			wantField: "UserID",
		},
		{
			name: "zero driver id",
			modify: func(a *newArgs) {
				a.driverID = uuid.Nil
			},
			wantField: "DriverID",
		},
		{
			name: "end point latitude too big",
			modify: func(a *newArgs) {
				a.end.Latitude = 90.0001
			},
			wantField: "EndPoint.Latitude",
		},
		{
			name: "end point latitude too small",
			modify: func(a *newArgs) {
				a.end.Latitude = -90.0001
			},
			wantField: "EndPoint.Latitude",
		},
		{
			name: "end point longitude too big",
			modify: func(a *newArgs) {
				a.end.Longitude = 180.0001
			},
			wantField: "EndPoint.Longitude",
		},
		{
			name: "end point longitude too small",
			modify: func(a *newArgs) {
				a.end.Longitude = -180.0001
			},
			wantField: "EndPoint.Longitude",
		},
		{
			name: "start point latitude too big",
			modify: func(a *newArgs) {
				a.start.Latitude = 90.0001
			},
			wantField: "StartPoint.Latitude",
		},
		{
			name: "start point latitude too small",
			modify: func(a *newArgs) {
				a.start.Latitude = -90.0001
			},
			wantField: "StartPoint.Latitude",
		},
		{
			name: "start point longitude too big",
			modify: func(a *newArgs) {
				a.start.Longitude = 180.0001
			},
			wantField: "StartPoint.Longitude",
		},
		{
			name: "start point longitude too small",
			modify: func(a *newArgs) {
				a.start.Longitude = -180.0001
			},
			wantField: "StartPoint.Longitude",
		},
		{
			name: "negative price",
			modify: func(a *newArgs) {
				a.price = -1
			},
			wantField: "Price",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := validNewArgs()
			tt.modify(&a)

			got, gotErr := trip.New(a.id, a.userID, a.driverID, a.start, a.end, a.price, a.startedAt)
			var vErr *trip.ValidationError

			if got != nil {
				t.Errorf("got trip %+v, want nil", got)
			}
			if !errors.As(gotErr, &vErr) {
				t.Fatalf("expected *ValidationError, got %v", gotErr)
				return
			}
			if vErr.Field != tt.wantField {
				t.Errorf("field = %q, want %q", vErr.Field, tt.wantField)
			}
		})
	}
}

func newTrip(t *testing.T) *trip.Trip {
	t.Helper()
	a := validNewArgs()
	tr, err := trip.New(a.id, a.userID, a.driverID, a.start, a.end, a.price, a.startedAt)
	if err != nil {
		t.Fatalf("trip.New: %v", err)
	}
	return tr
}

func TestFinish(t *testing.T) {
	t.Run("active trip is completed", func(t *testing.T) {
		tr := newTrip(t)
		at := tr.StartedAt.Add(time.Hour)

		if err := tr.Finish(at); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tr.Status != trip.StatusCompleted || tr.FinishedAt == nil || !tr.FinishedAt.Equal(at) {
			t.Errorf("got status %q, finishedAt %v; want completed at %v", tr.Status, tr.FinishedAt, at)
		}
	})

	t.Run("completed trip cannot be finished again", func(t *testing.T) {
		tr := newTrip(t)
		first := tr.StartedAt.Add(time.Hour)
		_ = tr.Finish(first)

		err := tr.Finish(first.Add(time.Hour))
		if !errors.Is(err, trip.ErrTripCompleted) {
			t.Fatalf("error = %v, want ErrTripCompleted", err)
		}
		if !tr.FinishedAt.Equal(first) {
			t.Errorf("finishedAt changed to %v, want %v", *tr.FinishedAt, first)
		}
	})

	t.Run("finish before start is rejected", func(t *testing.T) {
		tr := newTrip(t)

		err := tr.Finish(tr.StartedAt.Add(-time.Nanosecond))
		if !errors.Is(err, trip.ErrFinishBeforeStart) {
			t.Fatalf("error = %v, want ErrFinishBeforeStart", err)
		}
		if tr.Status != trip.StatusActive || tr.FinishedAt != nil {
			t.Errorf("trip was modified: status %q, finishedAt %v", tr.Status, tr.FinishedAt)
		}
	})
}
