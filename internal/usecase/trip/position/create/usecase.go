package create

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
)

type Input struct {
	TripID      uuid.UUID
	Coordinates trip.Coordinates
	RecordedAt  time.Time
}

type Usecase struct {
	repo     tripRepository
	tx       txManager
	notifier cancellationNotifier
	counter  positionCounter
	logger   *slog.Logger
	now      func() time.Time
}

func New(
	repo tripRepository,
	tx txManager,
	notifier cancellationNotifier,
	counter positionCounter,
	logger *slog.Logger,
	now func() time.Time,
) *Usecase {
	return &Usecase{
		repo:     repo,
		tx:       tx,
		notifier: notifier,
		counter:  counter,
		logger:   logger,
		now:      now,
	}
}

func (uc *Usecase) Execute(ctx context.Context, input Input) (*trip.Position, error) {
	now := uc.now()

	pos, err := trip.NewPosition(input.TripID, input.Coordinates, input.RecordedAt, now)
	if err != nil {
		return nil, fmt.Errorf("validate position: %w", err)
	}

	var savedPos *trip.Position

	err = uc.tx.Do(ctx, func(txCtx context.Context) error {
		tr, err := uc.repo.Fetch(txCtx, input.TripID)
		if err != nil {
			return fmt.Errorf("fetch trip: %w", err)
		}

		if err := tr.CanAcceptPositions(); err != nil {
			return err
		}

		savedPos, err = uc.repo.AddPosition(txCtx, *pos)
		if err != nil {
			return fmt.Errorf("add position: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	if uc.notifier != nil {
		uc.notifier.Cancel(input.TripID)
	}

	if uc.counter != nil {
		uc.counter.Inc(ctx)
	}

	uc.logger.InfoContext(ctx, "trip position saved",
		"trip_id", input.TripID,
		"position_id", savedPos.ID,
	)

	return savedPos, nil
}
