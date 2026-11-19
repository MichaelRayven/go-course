package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX is the subset of pgx operations used by repositories. Both
// *pgxpool.Pool and pgx.Tx implement it.
type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type transactionContextKey struct{}

func contextWithTransaction(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, transactionContextKey{}, tx)
}

func transactionFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(transactionContextKey{}).(pgx.Tx)
	return tx, ok
}

func ExecutorFromContext(ctx context.Context, fallback DBTX) DBTX {
	if tx, ok := transactionFromContext(ctx); ok {
		return tx
	}

	return fallback
}
