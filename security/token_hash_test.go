package security_test

import (
	"regexp"
	"testing"

	"github.com/jungo-dev/junkit/security"
)

func TestHashToken(t *testing.T) {
	// sha256("abc")
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := security.HashToken("abc"); got != want {
		t.Errorf("HashToken(abc) = %s, want %s", got, want)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(security.HashToken(security.GenerateToken())) {
		t.Error("HashToken output is not 64 lowercase hex chars")
	}
}

func TestSecureEqual(t *testing.T) {
	if !security.SecureEqual("secret", "secret") {
		t.Error("equal strings reported unequal")
	}
	for _, other := range []string{"Secret", "secret2", "", "secre"} {
		if security.SecureEqual("secret", other) {
			t.Errorf("SecureEqual(secret, %q) = true", other)
		}
	}
}
