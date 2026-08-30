package database_test

import (
	"testing"

	"github.com/jungo-dev/junkit/database"
)

func TestMaskDSN(t *testing.T) {
	tests := []struct {
		name string
		dsn  string
		want string
	}{
		{name: "empty string is returned unchanged", dsn: "", want: ""},
		{
			name: "password is masked",
			dsn:  "host=localhost port=5432 user=postgres password=secret123 dbname=app",
			want: "host=localhost port=5432 user=postgres password=******** dbname=app",
		},
		{
			name: "key match is case-insensitive",
			dsn:  "user=admin PASSWORD=Secret123",
			want: "user=admin PASSWORD=********",
		},
		{
			name: "pwd, secret, token, and api_key are all recognized",
			dsn:  "pwd=a secret=b token=c api_key=d api-key=e",
			want: "pwd=******** secret=******** token=******** api_key=******** api-key=********",
		},
		{
			name: "single-quoted values are masked whole",
			dsn:  "password='p@ss word' host=localhost",
			want: "password=******** host=localhost",
		},
		{
			name: "a DSN with no recognized secret keys is unchanged",
			dsn:  "host=localhost port=5432 dbname=app sslmode=disable",
			want: "host=localhost port=5432 dbname=app sslmode=disable",
		},
		{
			name: "URL-style credentials are not masked by this function",
			dsn:  "postgres://user:pass@localhost:5432/app",
			want: "postgres://user:pass@localhost:5432/app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := database.MaskDSN(tt.dsn); got != tt.want {
				t.Fatalf("MaskDSN(%q) = %q, want %q", tt.dsn, got, tt.want)
			}
		})
	}
}
