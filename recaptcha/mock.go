package recaptcha

import "context"

// MockVerifier is a Verifier that accepts every token unconditionally.
type MockVerifier struct{}

// NewMockVerifier creates a MockVerifier that accepts every token unconditionally.
//
// Usage:
//
//	verifier := recaptcha.NewMockVerifier()
func NewMockVerifier() *MockVerifier {
	return &MockVerifier{}
}

// Verify always returns nil.
func (m *MockVerifier) Verify(context.Context, string, string) error {
	return nil
}
