//go:build integration

package trip_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MichaelRayven/go-course/internal/domain/trip"
	"github.com/MichaelRayven/go-course/internal/infra/postgres/transaction"
	triprepo "github.com/MichaelRayven/go-course/internal/repository/trip"
)

const queryTimeout = 3 * time.Second

// newPool connects to the database from `tripgoctl environment start`.
// The pool is closed when the test ends.
func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is not set: run `tripgoctl environment start` and `make test-integration`")
	}

	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

// newTrip builds a valid trip with unique IDs so tests
// never collide on the "one active trip per driver" index.
func newTrip(t *testing.T, pool *pgxpool.Pool) *trip.Trip {
	t.Helper()

	tr, err := trip.New(
		uuid.New(),
		uuid.New(),
		uuid.New(),
		trip.Coordinates{Latitude: 10, Longitude: 20},
		trip.Coordinates{Latitude: 30, Longitude: 40},
		100,
		time.Now().UTC().Truncate(time.Microsecond), // Postgres stores microseconds
	)
	if err != nil {
		t.Fatalf("trip.New: %v", err)
	}

	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM trips WHERE id = $1", tr.ID); err != nil {
			t.Errorf("cleanup trip %v: %v", tr.ID, err)
		}
	})

	return tr
}

func countHistory(t *testing.T, pool *pgxpool.Pool, tripID uuid.UUID) int {
	t.Helper()

	var n int
	err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM trip_status_history WHERE trip_id = $1", tripID).Scan(&n)
	if err != nil {
		t.Fatalf("count history: %v", err)
	}

	return n
}

func TestRollbackUndoesBothInserts(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)
	repo := triprepo.New(pool, queryTimeout)
	txm := transaction.NewTransactionManager(pool, queryTimeout)
	tr := newTrip(t, pool)

	err := txm.Do(ctx, func(txCtx context.Context) error {
		if err := repo.Save(txCtx, *tr); err != nil {
			t.Errorf("save trip inside transaction: %v", err)
			return err
		}

		// "teleported" violates CHECK (to_status IN ('active', 'completed'))
		err := repo.AddStatusHistory(txCtx, trip.StatusChange{
			TripID:    tr.ID,
			ToStatus:  trip.Status("teleported"),
			ChangedAt: tr.StartedAt,
		})
		if err == nil {
			t.Error("expected the history insert to be rejected by the database")
			return errors.New("history insert unexpectedly succeeded")
		}

		return err
	})
	if err == nil {
		t.Fatal("transaction succeeded, want an error")
	}

	// Outside the transaction: the first insert must be gone as well.
	if _, err := repo.Fetch(ctx, tr.ID); !errors.Is(err, trip.ErrTripNotFound) {
		t.Errorf("Fetch after rollback: error = %v, want ErrTripNotFound", err)
	}
	if n := countHistory(t, pool, tr.ID); n != 0 {
		t.Errorf("history rows after rollback = %d, want 0", n)
	}
}

func TestSaveAndFetch(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)
	repo := triprepo.New(pool, queryTimeout)
	tr := newTrip(t, pool)

	if err := repo.Save(ctx, *tr); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := repo.Fetch(ctx, tr.ID)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.ID != tr.ID || got.UserID != tr.UserID || got.DriverID != tr.DriverID ||
		got.StartPoint != tr.StartPoint || got.EndPoint != tr.EndPoint ||
		got.Price != tr.Price || got.Status != trip.StatusActive ||
		!got.StartedAt.Equal(tr.StartedAt) || got.FinishedAt != nil {
		t.Errorf("Fetch = %+v, want %+v", got, tr)
	}
}

func TestFetchNotFound(t *testing.T) {
	repo := triprepo.New(newPool(t), queryTimeout)

	if _, err := repo.Fetch(context.Background(), uuid.New()); !errors.Is(err, trip.ErrTripNotFound) {
		t.Errorf("error = %v, want ErrTripNotFound", err)
	}
}

func TestSaveDriverBusy(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)
	repo := triprepo.New(pool, queryTimeout)

	first := newTrip(t, pool)
	second := newTrip(t, pool)
	second.DriverID = first.DriverID

	if err := repo.Save(ctx, *first); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	if err := repo.Save(ctx, *second); !errors.Is(err, trip.ErrDriverBusy) {
		t.Errorf("Save second: error = %v, want ErrDriverBusy", err)
	}
}

func TestFinish(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)
	repo := triprepo.New(pool, queryTimeout)
	tr := newTrip(t, pool)

	if err := repo.Save(ctx, *tr); err != nil {
		t.Fatalf("Save: %v", err)
	}

	finishedAt := tr.StartedAt.Add(time.Hour)
	if err := tr.Finish(finishedAt); err != nil {
		t.Fatalf("domain Finish: %v", err)
	}
	if err := repo.Finish(ctx, *tr); err != nil {
		t.Fatalf("repo Finish: %v", err)
	}

	got, err := repo.Fetch(ctx, tr.ID)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.Status != trip.StatusCompleted || got.FinishedAt == nil || !got.FinishedAt.Equal(finishedAt) {
		t.Errorf("after Finish: status %q, finishedAt %v; want completed at %v", got.Status, got.FinishedAt, finishedAt)
	}

	if err := repo.Finish(ctx, *tr); !errors.Is(err, trip.ErrTripCompleted) {
		t.Errorf("second Finish: error = %v, want ErrTripCompleted", err)
	}
}

func TestDriverFreeAfterFinish(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)
	repo := triprepo.New(pool, queryTimeout)

	first := newTrip(t, pool)
	if err := repo.Save(ctx, *first); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	if err := first.Finish(first.StartedAt.Add(time.Minute)); err != nil {
		t.Fatalf("domain Finish: %v", err)
	}
	if err := repo.Finish(ctx, *first); err != nil {
		t.Fatalf("repo Finish: %v", err)
	}

	second := newTrip(t, pool)
	second.DriverID = first.DriverID
	if err := repo.Save(ctx, *second); err != nil {
		t.Errorf("Save second trip for the same driver: %v", err)
	}
}

func TestAddStatusHistory(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t)
	repo := triprepo.New(pool, queryTimeout)
	tr := newTrip(t, pool)

	if err := repo.Save(ctx, *tr); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := repo.AddStatusHistory(ctx, trip.StatusChange{
		TripID:    tr.ID,
		ToStatus:  trip.StatusActive,
		ChangedAt: tr.StartedAt,
	}); err != nil {
		t.Fatalf("AddStatusHistory: %v", err)
	}

	if n := countHistory(t, pool, tr.ID); n != 1 {
		t.Errorf("history rows = %d, want 1", n)
	}
}
