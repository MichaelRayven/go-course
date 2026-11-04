package finish

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	"github.com/google/uuid"
)

type Usecase struct {
	repo   tripRepository
	tx     txManager
	logger *slog.Logger
	now    func() time.Time
}

func New(repo tripRepository, tx txManager, logger *slog.Logger, now func() time.Time) *Usecase {
	return &Usecase{
		repo:   repo,
		tx:     tx,
		logger: logger,
		now:    now,
	}
}

func (uc *Usecase) Execute(ctx context.Context, id uuid.UUID) (*trip.Trip, error) {
	var finished *trip.Trip

	err := uc.tx.Do(ctx, func(txCtx context.Context) error {
		t, txErr := uc.repo.Fetch(txCtx, id)
		if txErr != nil {
			return fmt.Errorf("fetch trip: %w", txErr)
		}

		now := uc.now()
		txErr = t.Finish(now)
		if txErr != nil {
			return fmt.Errorf("finish trip: %w", txErr)
		}

		txErr = uc.repo.Finish(txCtx, *t)
		if txErr != nil {
			return fmt.Errorf("persist finished trip: %w", txErr)
		}

		fromStatus := trip.StatusActive
		txErr = uc.repo.AddStatusHistory(txCtx, trip.StatusChange{
			TripID:     t.ID,
			FromStatus: &fromStatus,
			ToStatus:   trip.StatusCompleted,
			ChangedAt:  now,
		})
		if txErr != nil {
			return fmt.Errorf("add status change history: %w", txErr)
		}

		finished = t
		return nil
	})

	if err != nil {
		return nil, err
	}

	uc.logger.InfoContext(ctx, "trip finished", "trip_id", finished.ID)
	return finished, nil
}
