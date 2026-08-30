package tracer

import "go.uber.org/fx"

// Module binds *zap.Logger as the fallback logger for tracer functions.
//
// Usage:
//
//	fx.New(tracer.Module)
var Module = fx.Module("tracer",
	fx.Invoke(BindLogger),
)
