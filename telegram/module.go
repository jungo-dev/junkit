package telegram

import "go.uber.org/fx"

// Module provides an Fx provider for Client.
//
// Usage:
//
//	fx.New(
//	    fx.Supply(telegram.Options{Token: cfg.Telegram.Token}),
//	    httpclient.Module,
//	    telegram.Module,
//	)
var Module = fx.Module("telegram",
	fx.Provide(NewClient),
)
