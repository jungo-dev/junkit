package database

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/jungo-dev/junkit/console"
)

// txContextKey is the context key under which the active transaction is stored.
type txContextKey struct{}

// WithTx returns a copy of ctx carrying tx, retrievable via GetTx.
func WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

// GetTx retrieves the transaction attached to ctx by WithTx, if any.
func GetTx(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txContextKey{}).(pgx.Tx)
	return tx, ok
}

// WithTransaction executes fn inside a database transaction, committing on success or rolling back on error/panic.
//
// Usage:
//
//	err := db.WithTransaction(ctx, func(txCtx context.Context) error {
//	    if err := repo.Create(txCtx, user); err != nil {
//	        return err
//	    }
//	    return repo.CreateAuditLog(txCtx, entry)
//	})
func (db *DB) WithTransaction(ctx context.Context, fn func(context.Context) error) (err error) {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return console.NewError("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		} else if err != nil {
			_ = tx.Rollback(ctx)
		} else if commitErr := tx.Commit(ctx); commitErr != nil {
			err = console.NewError("failed to commit transaction: %w", commitErr)
		}
	}()

	err = fn(WithTx(ctx, tx))
	return err
}
