package get_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	"github.com/MichaelRayven/go-course/internal/usecase/trip/get"
)

func TestExecute(t *testing.T) {
	t.Run("returns the trip", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := NewMocktripRepository(ctrl)
		want := &trip.Trip{ID: uuid.UUID{15: 1}}

		repo.EXPECT().Fetch(gomock.Any(), want.ID).Return(want, nil)

		got, gotErr := get.New(repo).Execute(context.Background(), want.ID)
		if gotErr != nil {
			t.Fatalf("unexpected error: %v", gotErr)
		}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})

	t.Run("not found passes through", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := NewMocktripRepository(ctrl)
		id := uuid.UUID{15: 1}

		repo.EXPECT().Fetch(gomock.Any(), id).Return(nil, trip.ErrTripNotFound)

		got, gotErr := get.New(repo).Execute(context.Background(), id)
		if !errors.Is(gotErr, trip.ErrTripNotFound) {
			t.Errorf("error = %v, want ErrTripNotFound", gotErr)
		}
		if got != nil {
			t.Fatalf("unexpected return: wanted nil, got %+v", got)
		}
	})
}
