package get

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
)

type Usecase struct {
	repo tripRepository
}

func New(repo tripRepository) *Usecase {
	return &Usecase{
		repo: repo,
	}
}

func (uc *Usecase) Execute(ctx context.Context, id uuid.UUID) (*trip.Trip, error) {
	t, err := uc.repo.Fetch(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get trip: %w", err)
	}

	return t, nil
}
