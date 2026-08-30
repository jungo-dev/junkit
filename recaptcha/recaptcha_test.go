package recaptcha_test

import (
	"context"
	"testing"

	"github.com/jungo-dev/junkit/recaptcha"
)

func TestNewClient_ConstructsWithoutPanicking(t *testing.T) {
	if recaptcha.NewClient(recaptcha.Options{SecretKey: "x"}, nil) == nil {
		t.Fatal("NewClient() returned nil")
	}
}

func TestMockVerifier_AlwaysSucceeds(t *testing.T) {
	v := recaptcha.NewMockVerifier()

	if err := v.Verify(context.Background(), "any-token", "1.2.3.4"); err != nil {
		t.Fatalf("Verify() error = %v, want nil", err)
	}
	if err := v.Verify(context.Background(), "", ""); err != nil {
		t.Fatalf("Verify() with empty args error = %v, want nil", err)
	}
}
