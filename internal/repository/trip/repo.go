package trip

import (
	"time"

	"github.com/MichaelRayven/go-course/internal/infra/postgres"
)

const (
	activeDriverIndex       = "one_trip_active_per_driver_idx"
	postgresUniqueViolation = "23505"
)

type Repository struct {
	pool         postgres.DBTX
	queryTimeout time.Duration
}

func New(pool postgres.DBTX, queryTimeout time.Duration) *Repository {
	return &Repository{
		pool:         pool,
		queryTimeout: queryTimeout,
	}
}
