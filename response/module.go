package response

import "go.uber.org/fx"

// Module provides an Fx provider for Responder.
//
// Usage:
//
//	fx.New(
//	    fx.Supply(response.Options{Language: "en"}),
//	    i18n.Module,
//	    response.Module,
//	)
var Module = fx.Module("response",
	fx.Provide(NewResponder),
)
