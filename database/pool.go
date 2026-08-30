// Package database provides PostgreSQL connection pool management, transactions, and query logging.
package database

import (
	"context"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/jungo-dev/junkit/console"
)

// DBTX is the common query-execution interface shared by a database pool and a transaction (compatible with sqlc).
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DBPool defines the database pool interface.
type DBPool interface {
	DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
	Ping(ctx context.Context) error
	Close()
}

// Options configures NewDatabase.
type Options struct {
	// DSN is the PostgreSQL connection string (e.g. "postgres://user:pass@host:5432/db").
	DSN string
	// MaxConnection is the maximum number of pooled connections.
	MaxConnection int32
	// MinConnection is the minimum number of pooled connections kept open.
	MinConnection int32
	// MaxConnLifetime is the maximum age of a pooled connection before it is recycled.
	MaxConnLifetime time.Duration
	// MaxConnIdleTime is the maximum idle time before a pooled connection is closed.
	MaxConnIdleTime time.Duration
	// HealthCheckPeriod is how often the pool checks idle connections' health.
	HealthCheckPeriod time.Duration
	// QueryLogging enables structured per-query logging through the supplied
	// *zap.Logger (see NewDatabase). When false, no query tracer is attached.
	QueryLogging bool
	// SlowQueryThreshold escalates any query slower than this to a WARN log
	// entry when QueryLogging is true. Zero disables slow-query escalation.
	SlowQueryThreshold time.Duration
	// Decorate is an optional middleware wrapper applied to all DBTX instances (e.g. for tracing or metrics).
	Decorate func(DBTX) DBTX
}

// DB is the primary database handle supporting connection pooling and transactions.
type DB struct {
	pool     DBPool
	decorate func(DBTX) DBTX
	executor DBTX
}

// Params are NewDatabase's Fx-injected dependencies.
type Params struct {
	fx.In

	Lifecycle fx.Lifecycle
	Options   Options
	Logger    *zap.Logger
}

// NewDatabase opens a PostgreSQL connection pool and registers Fx lifecycle hooks.
//
// Usage:
//
//	fx.New(fx.Supply(database.Options{DSN: dsn, MaxConnection: 20}), database.Module)
func NewDatabase(p Params) (*DB, error) {
	poolConfig, err := pgxpool.ParseConfig(p.Options.DSN)
	if err != nil {
		return nil, console.NewError("failed to parse postgres dsn: %w", err)
	}

	poolConfig.MaxConns = p.Options.MaxConnection
	poolConfig.MinConns = p.Options.MinConnection
	poolConfig.MaxConnLifetime = p.Options.MaxConnLifetime
	poolConfig.MaxConnIdleTime = p.Options.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = p.Options.HealthCheckPeriod

	if p.Options.QueryLogging {
		poolConfig.ConnConfig.Tracer = &tracelog.TraceLog{
			Logger:   newQueryTracer(p.Logger, p.Options.SlowQueryThreshold),
			LogLevel: tracelog.LogLevelInfo,
		}
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, console.NewError("failed to create pgx pool: %w", err)
	}

	db := newDB(pool, p.Options.Decorate)
	registerLifecycleHooks(p.Lifecycle, pool, p.Logger, p.Options.DSN)

	return db, nil
}

// NewDB builds a DB from an existing DBPool instance.
func NewDB(pool DBPool) *DB {
	return newDB(pool, nil)
}

// newDB constructs a DB, pre-decorating the pool-level executor once if decorate is non-nil.
func newDB(pool DBPool, decorate func(DBTX) DBTX) *DB {
	var executor DBTX = pool
	if decorate != nil {
		executor = decorate(pool)
	}
	return &DB{pool: pool, decorate: decorate, executor: executor}
}

// registerLifecycleHooks registers start/stop lifecycle hooks for the database pool.
func registerLifecycleHooks(lc fx.Lifecycle, pool DBPool, log *zap.Logger, dsn string) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := pool.Ping(ctx); err != nil {
				return console.NewError("database unreachable: %w", err)
			}
			log.Info("connected to PostgreSQL", zap.String("dsn", MaskDSN(dsn)))
			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("closing database connections")
			pool.Close()
			return nil
		},
	})
}

// Executor returns the active transaction DBTX from ctx if available, otherwise the pool executor.
//
// Usage:
//
//	queries := sqlc.New(db.Executor(ctx))
func (db *DB) Executor(ctx context.Context) DBTX {
	if tx, ok := GetTx(ctx); ok {
		if db.decorate != nil {
			return db.decorate(tx)
		}
		return tx
	}
	return db.executor
}

// Pool returns the underlying DBPool instance.
func (db *DB) Pool() DBPool {
	return db.pool
}

// sensitiveRe matches "key=value" pairs whose key commonly holds a secret
// (password, token, api key, ...), for redacting connection strings in logs.
var sensitiveRe = regexp.MustCompile(
	`(?i)(\b(password|passwd|pwd|secret|token|api[_-]?key)\s*=\s*)(('[^']*')|("[^"]*")|(\S+))`,
)

// MaskDSN redacts secrets in a connection string with "********".
//
// Usage:
//
//	log.Info("connecting", zap.String("dsn", database.MaskDSN(dsn)))
func MaskDSN(dsn string) string {
	if dsn == "" {
		return dsn
	}
	return sensitiveRe.ReplaceAllString(dsn, `${1}********`)
}
