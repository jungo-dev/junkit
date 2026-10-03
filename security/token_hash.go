package security

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
)

// HashToken returns the hex SHA-256 of raw (64 chars), for storing instead of the raw token.
//
// Usage:
//
//	hash := security.HashToken(raw)
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// SecureEqual compares two secrets in constant time.
//
// Usage:
//
//	if !security.SecureEqual(header, secret) { ... }
func SecureEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
