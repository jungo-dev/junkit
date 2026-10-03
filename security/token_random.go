package security

// TokenSize is the number of random bytes in a generated token.
const TokenSize = 32

// GenerateToken returns TokenSize random bytes, base64url-encoded without padding.
//
// Usage:
//
//	raw := security.GenerateToken()
//	hash := security.HashToken(raw) // store this
//	sealed, _ := sealer.Seal(raw)   // return this to the client
func GenerateToken() string {
	return b64.EncodeToString(RandomBytes(TokenSize))
}
