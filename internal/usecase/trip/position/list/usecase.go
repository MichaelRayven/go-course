package list

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
	return &Usecase{repo: repo}
}

func (uc *Usecase) Execute(ctx context.Context, tripID uuid.UUID) ([]trip.Position, error) {
	if _, err := uc.repo.Fetch(ctx, tripID); err != nil {
		return nil, fmt.Errorf("fetch trip: %w", err)
	}

	positions, err := uc.repo.FetchPositions(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("fetch positions: %w", err)
	}

	return positions, nil
}
