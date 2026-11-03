// Package service is a temporary bridge between the old handler and the new
// domain/repository packages. It is replaced by internal/usecase in T6.
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	api "github.com/MichaelRayven/go-course/internal/generated"
	"github.com/MichaelRayven/go-course/internal/infra/postgres/transaction"
	triprepo "github.com/MichaelRayven/go-course/internal/repository/trip"
)

type TripService struct {
	repository *triprepo.Repository
	txManager  transaction.TxManager
}

func NewTripService(repository *triprepo.Repository, txManager transaction.TxManager) *TripService {
	return &TripService{repository: repository, txManager: txManager}
}

func (s *TripService) Create(ctx context.Context, data api.TripData) (api.Trip, error) {
	now := time.Now().UTC()

	t, err := trip.New(
		uuid.New(),
		data.UserId,
		data.DriverId,
		trip.Coordinates{Latitude: data.StartPoint.Latitude, Longitude: data.StartPoint.Longitude},
		trip.Coordinates{Latitude: data.EndPoint.Latitude, Longitude: data.EndPoint.Longitude},
		data.Price,
		now,
	)
	if err != nil {
		return api.Trip{}, fmt.Errorf("new trip: %w", err)
	}

	err = s.txManager.Do(ctx, func(txCtx context.Context) error {
		if err := s.repository.Save(txCtx, *t); err != nil {
			return fmt.Errorf("save trip: %w", err)
		}

		if err := s.repository.AddStatusHistory(txCtx, trip.StatusChange{
			TripID:    t.ID,
			ToStatus:  trip.StatusActive,
			ChangedAt: now,
		}); err != nil {
			return fmt.Errorf("add initial trip status: %w", err)
		}

		return nil
	})
	if err != nil {
		return api.Trip{}, fmt.Errorf("create trip transaction: %w", err)
	}

	return toAPI(t), nil
}

func (s *TripService) GetByID(ctx context.Context, id api.TripId) (api.Trip, error) {
	t, err := s.repository.Fetch(ctx, id)
	if err != nil {
		return api.Trip{}, fmt.Errorf("get trip: %w", err)
	}

	return toAPI(t), nil
}

func (s *TripService) Finish(ctx context.Context, id api.TripId) (api.Trip, error) {
	now := time.Now().UTC()

	var finished *trip.Trip

	err := s.txManager.Do(ctx, func(txCtx context.Context) error {
		t, err := s.repository.Fetch(txCtx, id)
		if err != nil {
			return fmt.Errorf("fetch trip: %w", err)
		}

		if err := t.Finish(now); err != nil {
			return fmt.Errorf("finish trip: %w", err)
		}

		if err := s.repository.Finish(txCtx, t); err != nil {
			return fmt.Errorf("persist finished trip: %w", err)
		}

		fromStatus := trip.StatusActive
		if err := s.repository.AddStatusHistory(txCtx, trip.StatusChange{
			TripID:     t.ID,
			FromStatus: &fromStatus,
			ToStatus:   trip.StatusCompleted,
			ChangedAt:  now,
		}); err != nil {
			return fmt.Errorf("add completed trip status: %w", err)
		}

		finished = t
		return nil
	})
	if err != nil {
		return api.Trip{}, fmt.Errorf("finish trip transaction: %w", err)
	}

	return toAPI(finished), nil
}

func toAPI(t *trip.Trip) api.Trip {
	return api.Trip{
		Id:             t.ID,
		UserId:         t.UserID,
		DriverId:       t.DriverID,
		StartPoint:     api.Coordinates{Latitude: t.StartPoint.Latitude, Longitude: t.StartPoint.Longitude},
		EndPoint:       api.Coordinates{Latitude: t.EndPoint.Latitude, Longitude: t.EndPoint.Longitude},
		Price:          t.Price,
		Status:         api.TripStatus(t.Status),
		StartedAt:      t.StartedAt,
		FinishedAt:     t.FinishedAt,
		LastPositionAt: t.LastPositionAt,
	}
}
