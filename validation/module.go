package validation

import "go.uber.org/fx"

// Module provides an Fx provider for Validator.
//
// Usage:
//
//	fx.New(
//	    fx.Supply(validation.Options{Language: "en"}),
//	    i18n.Module,
//	    validation.Module,
//	)
var Module = fx.Module("validation",
	fx.Provide(NewValidator),
)
