package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/MichaelRayven/go-course/internal/db"
	api "github.com/MichaelRayven/go-course/internal/generated"
	"github.com/MichaelRayven/go-course/internal/repository"
)

type TripService struct {
	repository *repository.TripRepository
	txManager  db.TxManager
}

func NewTripService(repository *repository.TripRepository, txManager db.TxManager) *TripService {
	return &TripService{repository: repository, txManager: txManager}
}

func (s *TripService) Create(
	ctx context.Context,
	data api.TripData,
) (api.Trip, error) {
	now := time.Now().UTC()

	tripToCreate := api.Trip{
		Id:             uuid.New(),
		UserId:         data.UserId,
		DriverId:       data.DriverId,
		StartPoint:     data.StartPoint,
		EndPoint:       data.EndPoint,
		Price:          data.Price,
		Status:         api.Active,
		StartedAt:      now,
		FinishedAt:     nil,
		LastPositionAt: nil,
	}

	var createdTrip api.Trip

	err := s.txManager.Do(ctx, func(txCtx context.Context) error {
		trip, err := s.repository.Create(txCtx, tripToCreate)
		if err != nil {
			return fmt.Errorf("create trip: %w", err)
		}

		if err := s.repository.AddStatusHistory(
			txCtx,
			repository.StatusChange{
				TripID:     trip.Id,
				FromStatus: nil,
				ToStatus:   api.Active,
				Reason:     nil,
				ChangedAt:  now,
			},
		); err != nil {
			return fmt.Errorf("add initial trip status: %w", err)
		}

		createdTrip = trip
		return nil
	})
	if err != nil {
		return api.Trip{}, fmt.Errorf("create trip transaction: %w", err)
	}

	return createdTrip, nil
}

func (s *TripService) GetByID(ctx context.Context, id api.TripId) (api.Trip, error) {
	trip, err := s.repository.GetByID(ctx, id)
	if err != nil {
		return api.Trip{}, fmt.Errorf("get trip: %w", err)
	}
	return trip, nil
}

func (s *TripService) Finish(ctx context.Context, id api.TripId) (api.Trip, error) {
	now := time.Now().UTC()
	var finishedTrip api.Trip

	err := s.txManager.Do(ctx, func(txCtx context.Context) error {
		trip, err := s.repository.Finish(txCtx, id, now)
		if err != nil {
			return fmt.Errorf("finish trip: %w", err)
		}

		fromStatus := api.Active
		if err := s.repository.AddStatusHistory(txCtx, repository.StatusChange{
			TripID:     trip.Id,
			FromStatus: &fromStatus,
			ToStatus:   api.Completed,
			Reason:     nil,
			ChangedAt:  now,
		}); err != nil {
			return fmt.Errorf("add completed trip status: %w", err)
		}

		finishedTrip = trip
		return nil
	})
	if err != nil {
		return api.Trip{}, fmt.Errorf("finish trip transaction: %w", err)
	}

	return finishedTrip, nil
}
