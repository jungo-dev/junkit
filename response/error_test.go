package response_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jungo-dev/junkit/response"
)

func TestNew(t *testing.T) {
	err := response.New(response.NotFound, "user_not_found")

	if err.Code != response.NotFound {
		t.Fatalf("Code = %q, want %q", err.Code, response.NotFound)
	}
	if err.Message != "user_not_found" {
		t.Fatalf("Message = %q, want %q", err.Message, "user_not_found")
	}
	if err.Cause != nil {
		t.Fatalf("Cause = %v, want nil", err.Cause)
	}
	if err.Error() != "user_not_found" {
		t.Fatalf("Error() = %q, want just the message with no cause", err.Error())
	}
}

func TestWrap(t *testing.T) {
	cause := errors.New("connection refused")
	err := response.Wrap(response.Internal, "failed_to_save_user", cause)

	if err.Error() != "failed_to_save_user: connection refused" {
		t.Fatalf("Error() = %q, want the message and cause both included", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatal("errors.Is(err, cause) should be true — Unwrap must expose the cause")
	}
}

func TestAppError_ErrorsAsThroughAWrappingLayer(t *testing.T) {
	original := response.New(response.Conflict, "email_already_exists")
	wrapped := fmt.Errorf("create user: %w", original)

	var target *response.AppError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should find the *AppError through a %w-wrapping layer")
	}
	if target.Code != response.Conflict {
		t.Fatalf("unwrapped Code = %q, want %q", target.Code, response.Conflict)
	}
}

func TestSentinelErrors(t *testing.T) {
	if response.ErrInternalServer.Code != response.Internal {
		t.Fatalf("ErrInternalServer.Code = %q, want %q", response.ErrInternalServer.Code, response.Internal)
	}
	if response.ErrDatabaseConnection.Code != response.Internal {
		t.Fatalf("ErrDatabaseConnection.Code = %q, want %q", response.ErrDatabaseConnection.Code, response.Internal)
	}
}
