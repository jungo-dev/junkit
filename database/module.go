package database

import "go.uber.org/fx"

// Module provides an Fx module that constructs a *DB instance from Options and *zap.Logger.
//
// Usage:
//
//	fx.New(
//	    fx.Supply(database.Options{DSN: dsn, MaxConnection: 20}),
//	    database.Module,
//	)
var Module = fx.Module("database",
	fx.Provide(NewDatabase),
)
