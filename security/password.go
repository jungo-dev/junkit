package security

import (
	"golang.org/x/crypto/bcrypt"
)

// maxPasswordBytes is bcrypt's input limit.
const maxPasswordBytes = 72

// PasswordHasher hashes and verifies passwords with bcrypt.
type PasswordHasher interface {
	// Hash returns the bcrypt hash of plain.
	Hash(plain string) (string, error)
	// Compare reports whether plain matches hash.
	Compare(hash, plain string) bool
	// CompareDummy spends the same time as Compare; call it when the user does not exist.
	CompareDummy(plain string)
}

// passwordHasher is the default bcrypt PasswordHasher.
type passwordHasher struct {
	cost      int
	dummyHash []byte
}

// NewPasswordHasher creates a PasswordHasher with opts.Password.Cost.
//
// Usage:
//
//	hasher, err := security.NewPasswordHasher(security.Options{Password: security.PasswordOptions{Cost: 12}})
//	hash, err := hasher.Hash("s3cret-pass")
//	ok := hasher.Compare(hash, "s3cret-pass")
func NewPasswordHasher(opts Options) (PasswordHasher, error) {
	cost := opts.Password.Cost
	if cost == 0 {
		cost = bcrypt.DefaultCost
	}
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return nil, ErrInvalidPasswordCost
	}

	dummy, err := bcrypt.GenerateFromPassword([]byte("security.dummy-password"), cost)
	if err != nil {
		return nil, err
	}
	return &passwordHasher{cost: cost, dummyHash: dummy}, nil
}

// Hash implements PasswordHasher. Passwords over 72 bytes return ErrPasswordTooLong.
func (h *passwordHasher) Hash(plain string) (string, error) {
	if len(plain) > maxPasswordBytes {
		return "", ErrPasswordTooLong
	}
	b, err := bcrypt.GenerateFromPassword([]byte(plain), h.cost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Compare implements PasswordHasher.
func (h *passwordHasher) Compare(hash, plain string) bool {
	if len(plain) > maxPasswordBytes {
		h.CompareDummy(plain)
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// CompareDummy implements PasswordHasher.
func (h *passwordHasher) CompareDummy(plain string) {
	if len(plain) > maxPasswordBytes {
		plain = plain[:maxPasswordBytes]
	}
	_ = bcrypt.CompareHashAndPassword(h.dummyHash, []byte(plain))
}
