// Package repository provides a shared transaction helper used across all
// repository implementations. It is a pure infrastructure utility with zero
// business knowledge — safe to import from any internal/*/repository package.
//
// Rule (database-access.md §4): Transactions are owned by the service layer,
// executed via this helper, with context.Context propagated throughout.
package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InTx executes fn inside a single database transaction.
//
// The transaction is committed when fn returns nil; otherwise it is rolled back.
// The deferred Rollback is a no-op after a successful Commit (pgx ignores it).
//
// Usage (from the service layer):
//
//	err := repository.InTx(ctx, pool, func(tx pgx.Tx) error {
//	    if err := postRepo.CreateTx(ctx, tx, post); err != nil { return err }
//	    return nil
//	})
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // intentional: no-op after Commit

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
