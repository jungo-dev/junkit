package security

import "errors"

// Errors returned by TokenSealer.Open.
var (
	ErrMalformedToken = errors.New("security: malformed token")
	ErrBadSignature   = errors.New("security: invalid token signature")
	ErrDecrypt        = errors.New("security: token decryption failed")
)

// Errors returned by the constructors.
var (
	ErrInvalidKey          = errors.New("security: invalid token key")
	ErrInvalidPasswordCost = errors.New("security: invalid bcrypt cost")
)

// ErrPasswordTooLong is returned when a password exceeds bcrypt's 72-byte limit.
var ErrPasswordTooLong = errors.New("security: password exceeds 72 bytes")
