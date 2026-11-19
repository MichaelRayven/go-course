package main

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TEST_DATABASE_URL=postgres://... go test -v ./cmd/trip-service -run Shutdown
func TestShutdown(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	tests := []struct {
		name    string
		budget  time.Duration
		wantErr bool
	}{
		{name: "budget is enough", budget: 3 * time.Second, wantErr: false},
		{name: "budget is short", budget: 200 * time.Millisecond, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := pgxpool.New(context.Background(), dbURL)
			if err != nil {
				t.Fatal(err)
			}
			if err := pool.Ping(context.Background()); err != nil {
				t.Fatal(err)
			}

			// Медленный запрос держит соединение 1 секунду.
			go pool.Exec(context.Background(), "SELECT pg_sleep(1)")
			for i := 0; pool.Stat().AcquiredConns() == 0; i++ {
				if i > 100 {
					t.Fatal("slow query did not start")
				}
				time.Sleep(10 * time.Millisecond)
			}

			ctx, cancel := context.WithTimeout(context.Background(), tt.budget)
			defer cancel()

			start := time.Now()
			err = shutdown(ctx, &http.Server{}, pool)
			elapsed := time.Since(start)

			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if elapsed > tt.budget+100*time.Millisecond {
				t.Fatalf("shutdown took %v, budget %v", elapsed, tt.budget)
			}
		})
	}
}
