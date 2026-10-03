package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

// tokenVersion prefixes every sealed token.
const tokenVersion = "v1"

// maxTokenLength rejects oversized input before any crypto work.
const maxTokenLength = 4096

// b64 encodes token segments (URL-safe, no padding).
var b64 = base64.RawURLEncoding

// TokenSealer encrypts (AES-256-GCM) and signs (HMAC-SHA256) tokens for clients.
// Format: v1.<nonce+ciphertext>.<signature>.
type TokenSealer interface {
	// Seal encrypts and signs payload.
	Seal(payload string) (string, error)
	// Open verifies the signature, then decrypts.
	Open(token string) (string, error)
}

// tokenSealer is the default TokenSealer implementation.
type tokenSealer struct {
	secret []byte
	aead   cipher.AEAD
}

// NewTokenSealer creates a TokenSealer from opts.TokenHMACSecret and opts.TokenEncryptionKey.
//
// Usage:
//
//	sealer, err := security.NewTokenSealer(security.Options{TokenHMACSecret: hmacKey, TokenEncryptionKey: encKey})
//	token, _ := sealer.Seal(rawToken)
//	raw, err := sealer.Open(token)
func NewTokenSealer(opts Options) (TokenSealer, error) {
	if err := opts.validateTokenKeys(); err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(opts.TokenEncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidKey, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidKey, err)
	}

	return &tokenSealer{secret: append([]byte(nil), opts.TokenHMACSecret...), aead: aead}, nil
}

// Seal implements TokenSealer.
func (s *tokenSealer) Seal(payload string) (string, error) {
	nonce := RandomBytes(s.aead.NonceSize())
	ciphertext := s.aead.Seal(nonce, nonce, []byte(payload), []byte(tokenVersion))

	signed := tokenVersion + "." + b64.EncodeToString(ciphertext)
	return signed + "." + b64.EncodeToString(s.sign(signed)), nil
}

// Open implements TokenSealer.
func (s *tokenSealer) Open(token string) (string, error) {
	if len(token) == 0 || len(token) > maxTokenLength {
		return "", ErrMalformedToken
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != tokenVersion {
		return "", ErrMalformedToken
	}

	// Verify the signature over "v1.<ciphertext>" before decrypting.
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return "", ErrMalformedToken
	}
	if !hmac.Equal(sig, s.sign(parts[0]+"."+parts[1])) {
		return "", ErrBadSignature
	}

	data, err := b64.DecodeString(parts[1])
	if err != nil {
		return "", ErrMalformedToken
	}
	nonceSize := s.aead.NonceSize()
	if len(data) < nonceSize+s.aead.Overhead() {
		return "", ErrMalformedToken
	}
	plaintext, err := s.aead.Open(nil, data[:nonceSize], data[nonceSize:], []byte(tokenVersion))
	if err != nil {
		return "", ErrDecrypt
	}
	return string(plaintext), nil
}

// sign returns HMAC-SHA256 of msg.
func (s *tokenSealer) sign(msg string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(msg))
	return mac.Sum(nil)
}
