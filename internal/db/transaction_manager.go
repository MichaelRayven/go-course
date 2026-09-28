package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

var _ TxManager = (*TransactionManager)(nil)

type TransactionManager struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

func NewTransactionManager(pool *pgxpool.Pool, timeout time.Duration) *TransactionManager {
	return &TransactionManager{pool: pool, timeout: timeout}
}

func (m *TransactionManager) Do(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := transactionFromContext(ctx); ok {
		return fn(ctx)
	}

	txCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	tx, err := m.pool.BeginTx(txCtx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	txCtx = contextWithTransaction(txCtx, tx)
	defer func() {
		if recovered := recover(); recovered != nil {
			rollbackCtx, cancelRollback := context.WithTimeout(context.WithoutCancel(ctx), m.timeout)
			defer cancelRollback()
			_ = tx.Rollback(rollbackCtx)
			panic(recovered)
		}
	}()

	if err := fn(txCtx); err != nil {
		rollbackCtx, cancelRollback := context.WithTimeout(context.WithoutCancel(ctx), m.timeout)
		defer cancelRollback()

		rollbackErr := tx.Rollback(rollbackCtx)
		if rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return errors.Join(
				fmt.Errorf("run transaction: %w", err),
				fmt.Errorf("rollback transaction: %w", rollbackErr),
			)
		}

		return fmt.Errorf("run transaction: %w", err)
	}

	if err := tx.Commit(txCtx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}
