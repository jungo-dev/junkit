package security_test

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/jungo-dev/junkit/security"
)

func newHasher(t *testing.T) security.PasswordHasher {
	t.Helper()
	h, err := security.NewPasswordHasher(security.Options{Password: security.PasswordOptions{Cost: bcrypt.MinCost}})
	if err != nil {
		t.Fatalf("NewPasswordHasher: %v", err)
	}
	return h
}

func TestPasswordHasher_HashAndCompare(t *testing.T) {
	h := newHasher(t)

	hash, err := h.Hash("correct horse")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if hash == "correct horse" {
		t.Fatal("hash equals plaintext")
	}
	if !h.Compare(hash, "correct horse") {
		t.Error("Compare(correct) = false")
	}
	if h.Compare(hash, "wrong horse") {
		t.Error("Compare(wrong) = true")
	}
	if h.Compare("not-a-bcrypt-hash", "correct horse") {
		t.Error("Compare(garbage hash) = true")
	}
}

func TestPasswordHasher_UsesConfiguredCost(t *testing.T) {
	h := newHasher(t)
	hash, _ := h.Hash("pw")
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil || cost != bcrypt.MinCost {
		t.Errorf("cost = %d, %v; want %d", cost, err, bcrypt.MinCost)
	}
}

func TestPasswordHasher_TooLong(t *testing.T) {
	h := newHasher(t)
	long := strings.Repeat("a", 73)

	if _, err := h.Hash(long); !errors.Is(err, security.ErrPasswordTooLong) {
		t.Errorf("Hash(73 bytes) err = %v, want ErrPasswordTooLong", err)
	}

	// A hash of the 72-byte prefix must not match the 73-byte input (no silent truncation).
	hash, _ := h.Hash(long[:72])
	if h.Compare(hash, long) {
		t.Error("Compare accepted a password longer than 72 bytes")
	}
}

func TestPasswordHasher_CompareDummyDoesNotPanic(t *testing.T) {
	h := newHasher(t)
	h.CompareDummy("")
	h.CompareDummy(strings.Repeat("x", 200))
}

func TestNewPasswordHasher_InvalidCost(t *testing.T) {
	for _, cost := range []int{1, 32, -1} {
		if _, err := security.NewPasswordHasher(security.Options{Password: security.PasswordOptions{Cost: cost}}); !errors.Is(err, security.ErrInvalidPasswordCost) {
			t.Errorf("cost %d: err = %v, want ErrInvalidPasswordCost", cost, err)
		}
	}
}
