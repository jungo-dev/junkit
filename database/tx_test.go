package database_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/jungo-dev/junkit/database"
)

// fakeTx mocks pgx.Tx for testing, overriding Commit and Rollback.
type fakeTx struct {
	pgx.Tx
	commitErr  error
	committed  bool
	rolledBack bool
}

func (f *fakeTx) Commit(context.Context) error {
	f.committed = true
	return f.commitErr
}

func (f *fakeTx) Rollback(context.Context) error {
	f.rolledBack = true
	return nil
}

// fakePool mocks DBPool for testing, overriding Begin.
type fakePool struct {
	database.DBPool
	tx       *fakeTx
	beginErr error
}

func (f *fakePool) Begin(context.Context) (pgx.Tx, error) {
	if f.beginErr != nil {
		return nil, f.beginErr
	}
	return f.tx, nil
}

func TestWithTxAndGetTx(t *testing.T) {
	ctx := context.Background()

	if _, ok := database.GetTx(ctx); ok {
		t.Fatal("GetTx() on a plain context reported a transaction present")
	}

	tx := &fakeTx{}
	ctxWithTx := database.WithTx(ctx, tx)

	got, ok := database.GetTx(ctxWithTx)
	if !ok {
		t.Fatal("GetTx() = _, false after WithTx, want true")
	}
	if got != tx {
		t.Fatal("GetTx() returned a different transaction than the one passed to WithTx")
	}
}

func TestDB_WithTransaction(t *testing.T) {
	t.Run("commits on success", func(t *testing.T) {
		tx := &fakeTx{}
		db := database.NewDB(&fakePool{tx: tx})

		err := db.WithTransaction(context.Background(), func(context.Context) error {
			return nil
		})

		if err != nil {
			t.Fatalf("WithTransaction() error = %v, want nil", err)
		}
		if !tx.committed {
			t.Fatal("expected the transaction to be committed")
		}
		if tx.rolledBack {
			t.Fatal("did not expect a rollback on success")
		}
	})

	t.Run("rolls back when fn returns an error", func(t *testing.T) {
		tx := &fakeTx{}
		db := database.NewDB(&fakePool{tx: tx})
		wantErr := errors.New("boom")

		err := db.WithTransaction(context.Background(), func(context.Context) error {
			return wantErr
		})

		if !errors.Is(err, wantErr) {
			t.Fatalf("WithTransaction() error = %v, want %v", err, wantErr)
		}
		if tx.committed {
			t.Fatal("did not expect a commit when fn returned an error")
		}
		if !tx.rolledBack {
			t.Fatal("expected a rollback when fn returned an error")
		}
	})

	t.Run("rolls back and re-panics when fn panics", func(t *testing.T) {
		tx := &fakeTx{}
		db := database.NewDB(&fakePool{tx: tx})

		defer func() {
			r := recover()
			if r != "fn panicked" {
				t.Fatalf("recover() = %v, want %q", r, "fn panicked")
			}
			if !tx.rolledBack {
				t.Fatal("expected a rollback after a panic")
			}
			if tx.committed {
				t.Fatal("did not expect a commit after a panic")
			}
		}()

		_ = db.WithTransaction(context.Background(), func(context.Context) error {
			panic("fn panicked")
		})
	})

	t.Run("propagates a begin error without touching the transaction", func(t *testing.T) {
		beginErr := errors.New("connection refused")
		db := database.NewDB(&fakePool{beginErr: beginErr})

		err := db.WithTransaction(context.Background(), func(context.Context) error {
			t.Fatal("fn should not run when Begin fails")
			return nil
		})

		if !errors.Is(err, beginErr) {
			t.Fatalf("WithTransaction() error = %v, want it to wrap %v", err, beginErr)
		}
	})

	t.Run("wraps a commit error", func(t *testing.T) {
		commitErr := errors.New("commit failed")
		tx := &fakeTx{commitErr: commitErr}
		db := database.NewDB(&fakePool{tx: tx})

		err := db.WithTransaction(context.Background(), func(context.Context) error {
			return nil
		})

		if !errors.Is(err, commitErr) {
			t.Fatalf("WithTransaction() error = %v, want it to wrap %v", err, commitErr)
		}
	})

	t.Run("fn's context carries the active transaction", func(t *testing.T) {
		tx := &fakeTx{}
		db := database.NewDB(&fakePool{tx: tx})

		err := db.WithTransaction(context.Background(), func(ctx context.Context) error {
			got, ok := database.GetTx(ctx)
			if !ok || got != tx {
				t.Fatal("fn's context does not carry the transaction WithTransaction began")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
