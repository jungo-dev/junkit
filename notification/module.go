package notification

import "go.uber.org/fx"

// Module provides an Fx provider for Notifier.
//
// Usage:
//
//	fx.New(
//	    fx.Supply(notification.Options{TelegramChatID: chatID, Environment: cfg.Environment}),
//	    telegram.Module,
//	    notification.Module,
//	)
var Module = fx.Module("notification",
	fx.Provide(NewNotifier),
)
