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
	UserID     uuid.UUID
	DriverID   uuid.UUID
	StartPoint trip.Coordinates
	EndPoint   trip.Coordinates
	Price      int64
}

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

func (uc *Usecase) Execute(ctx context.Context, i Input) (*trip.Trip, error) {
	now := uc.now()

	t, err := trip.New(
		uuid.New(),
		i.UserID,
		i.DriverID,
		i.StartPoint,
		i.EndPoint,
		i.Price,
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("new trip: %w", err)
	}

	err = uc.tx.Do(ctx, func(txCtx context.Context) error {
		if err := uc.repo.Save(txCtx, *t); err != nil {
			return fmt.Errorf("save trip: %w", err)
		}

		if err := uc.repo.AddStatusHistory(txCtx, trip.StatusChange{
			TripID:    t.ID,
			ToStatus:  trip.StatusActive,
			ChangedAt: now,
		}); err != nil {
			return fmt.Errorf("add initial trip status: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	uc.logger.InfoContext(ctx, "trip created", "trip_id", t.ID)
	return t, nil
}
