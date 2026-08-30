package validation_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-playground/validator/v10"

	"github.com/jungo-dev/junkit/validation"
)

// newTestValidator returns a validator.Validate instance with custom rules registered.
func newTestValidator(t *testing.T) *validator.Validate {
	t.Helper()
	v := validator.New()
	validation.RegisterCustomValidation(v)
	return v
}

func TestPasswordStrongTag(t *testing.T) {
	v := newTestValidator(t)

	type form struct {
		Password string `validate:"password_strong"`
	}

	tests := []struct {
		name     string
		password string
		wantOK   bool
	}{
		{name: "meets every requirement", password: "Abcdef1!", wantOK: true},
		{name: "too short", password: "Ab1!", wantOK: false},
		{name: "missing uppercase", password: "abcdef1!", wantOK: false},
		{name: "missing lowercase", password: "ABCDEF1!", wantOK: false},
		{name: "missing digit", password: "Abcdefg!", wantOK: false},
		{name: "missing special character", password: "Abcdefg1", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(form{Password: tt.password})
			if (err == nil) != tt.wantOK {
				t.Fatalf("password %q: validate error = %v, want ok=%v", tt.password, err, tt.wantOK)
			}
		})
	}
}

func TestSlugTag(t *testing.T) {
	v := newTestValidator(t)

	type form struct {
		Slug string `validate:"slug"`
	}

	tests := []struct {
		name   string
		slug   string
		wantOK bool
	}{
		{name: "simple slug", slug: "hello-world", wantOK: true},
		{name: "dot separator is allowed", slug: "v1.2", wantOK: true},
		{name: "uppercase is rejected", slug: "Hello-World", wantOK: false},
		{name: "spaces are rejected", slug: "hello world", wantOK: false},
		{name: "leading dash is rejected", slug: "-hello", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(form{Slug: tt.slug})
			if (err == nil) != tt.wantOK {
				t.Fatalf("slug %q: validate error = %v, want ok=%v", tt.slug, err, tt.wantOK)
			}
		})
	}
}

func TestSearchTag(t *testing.T) {
	v := newTestValidator(t)

	type form struct {
		Search string `validate:"search"`
	}

	tests := []struct {
		name   string
		search string
		wantOK bool
	}{
		{name: "letters and digits", search: "John 2nd", wantOK: true},
		{name: "special characters are rejected", search: "John; DROP TABLE", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(form{Search: tt.search})
			if (err == nil) != tt.wantOK {
				t.Fatalf("search %q: validate error = %v, want ok=%v", tt.search, err, tt.wantOK)
			}
		})
	}
}

func TestMinIntMaxIntTags(t *testing.T) {
	v := newTestValidator(t)

	type form struct {
		Age int `validate:"min_int=1,max_int=130"`
	}

	tests := []struct {
		name   string
		age    int
		wantOK bool
	}{
		{name: "within range", age: 30, wantOK: true},
		{name: "at the minimum", age: 1, wantOK: true},
		{name: "at the maximum", age: 130, wantOK: true},
		{name: "below the minimum", age: 0, wantOK: false},
		{name: "above the maximum", age: 131, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(form{Age: tt.age})
			if (err == nil) != tt.wantOK {
				t.Fatalf("age %d: validate error = %v, want ok=%v", tt.age, err, tt.wantOK)
			}
		})
	}
}

func TestFileExtTag(t *testing.T) {
	v := newTestValidator(t)

	type form struct {
		Filename string `validate:"file_ext=jpg png"`
	}

	tests := []struct {
		name     string
		filename string
		wantOK   bool
	}{
		{name: "allowed extension", filename: "avatar.jpg", wantOK: true},
		{name: "allowed extension, case-insensitive", filename: "avatar.PNG", wantOK: true},
		{name: "disallowed extension", filename: "script.exe", wantOK: false},
		{name: "no extension", filename: "avatar", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(form{Filename: tt.filename})
			if (err == nil) != tt.wantOK {
				t.Fatalf("filename %q: validate error = %v, want ok=%v", tt.filename, err, tt.wantOK)
			}
		})
	}
}

func TestAlphanumDashTag(t *testing.T) {
	v := newTestValidator(t)

	type form struct {
		Code string `validate:"alphanum_dash"`
	}

	tests := []struct {
		name   string
		code   string
		wantOK bool
	}{
		{name: "letters, digits, dash, underscore", code: "SKU_123-a", wantOK: true},
		{name: "a dot is rejected", code: "SKU.123", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(form{Code: tt.code})
			if (err == nil) != tt.wantOK {
				t.Fatalf("code %q: validate error = %v, want ok=%v", tt.code, err, tt.wantOK)
			}
		})
	}
}

func TestDateTags(t *testing.T) {
	v := newTestValidator(t)

	type dateForm struct {
		Date string `validate:"date"`
	}
	type datePastForm struct {
		Date string `validate:"date_past"`
	}

	t.Run("date accepts YYYY-MM-DD", func(t *testing.T) {
		if err := v.Struct(dateForm{Date: "2026-01-15"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("date rejects other formats", func(t *testing.T) {
		if err := v.Struct(dateForm{Date: "15/01/2026"}); err == nil {
			t.Fatal("expected a validation error")
		}
	})

	t.Run("date_past accepts a date before now", func(t *testing.T) {
		if err := v.Struct(datePastForm{Date: "2020-01-01"}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("date_past rejects a future date", func(t *testing.T) {
		if err := v.Struct(datePastForm{Date: "2999-01-01"}); err == nil {
			t.Fatal("expected a validation error")
		}
	})
}

func TestPhoneAdvancedTag(t *testing.T) {
	v := newTestValidator(t)

	type form struct {
		Phone string `validate:"phone_advanced"`
	}

	tests := []struct {
		name   string
		phone  string
		wantOK bool
	}{
		{name: "with country code", phone: "+84912345678", wantOK: true},
		{name: "without country code", phone: "0912345678", wantOK: true},
		{name: "too short", phone: "12345", wantOK: false},
		{name: "contains letters", phone: "091234abcd", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(form{Phone: tt.phone})
			if (err == nil) != tt.wantOK {
				t.Fatalf("phone %q: validate error = %v, want ok=%v", tt.phone, err, tt.wantOK)
			}
		})
	}
}

// TestEmailAdvancedTag covers format and disposable-domain checks without network I/O.
func TestEmailAdvancedTag(t *testing.T) {
	v := newTestValidator(t)

	type form struct {
		Email string `validate:"email_advanced"`
	}

	tests := []struct {
		name   string
		email  string
		wantOK bool
	}{
		{name: "malformed address", email: "not-an-email", wantOK: false},
		{name: "disposable domain is rejected before any network lookup", email: "user@mailinator.com", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(form{Email: tt.email})
			if (err == nil) != tt.wantOK {
				t.Fatalf("email %q: validate error = %v, want ok=%v", tt.email, err, tt.wantOK)
			}
		})
	}
}

func TestIsDisposableEmail(t *testing.T) {
	if !validation.IsDisposableEmail("mailinator.com") {
		t.Error("mailinator.com should be in the hardcoded fallback list")
	}
	if !validation.IsDisposableEmail("MAILINATOR.COM") {
		t.Error("domain matching should be case-insensitive")
	}
	if validation.IsDisposableEmail("gmail.com") {
		t.Error("gmail.com should not be considered disposable")
	}
}

func TestLoadDisposableEmailsFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blocklist.conf")

	// Reset dynamicDisposableDomains after test completion.
	t.Cleanup(func() {
		empty := filepath.Join(dir, "empty.conf")
		_ = os.WriteFile(empty, nil, 0o644)
		_ = validation.LoadDisposableEmailsFromFile(empty)
	})

	content := "throwaway-test-domain.example\nANOTHER-TEST-DOMAIN.example\n\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test blocklist: %v", err)
	}

	if err := validation.LoadDisposableEmailsFromFile(path); err != nil {
		t.Fatalf("LoadDisposableEmailsFromFile() error = %v", err)
	}

	if !validation.IsDisposableEmail("throwaway-test-domain.example") {
		t.Error("domain loaded from the blocklist file should be reported as disposable")
	}
	if !validation.IsDisposableEmail("another-test-domain.example") {
		t.Error("domain matching against the loaded blocklist should be case-insensitive")
	}

	t.Run("missing file returns an error", func(t *testing.T) {
		if err := validation.LoadDisposableEmailsFromFile(filepath.Join(dir, "does-not-exist.conf")); err == nil {
			t.Fatal("expected an error for a nonexistent file")
		}
	})
}
