package security

import "go.uber.org/fx"

// Module provides an Fx module for TokenSealer and PasswordHasher.
//
// Usage:
//
//	fx.New(
//	    fx.Supply(security.Options{TokenHMACSecret: hmacKey, TokenEncryptionKey: encKey}),
//	    security.Module,
//	)
var Module = fx.Module("security",
	fx.Provide(NewTokenSealer, NewPasswordHasher),
)
