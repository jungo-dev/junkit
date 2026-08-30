package tracer

import "testing"

func TestGuessQueryName(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{name: "select", sql: "SELECT * FROM users", want: "Select"},
		{name: "lowercase keyword", sql: "select * from users", want: "Select"},
		{name: "insert", sql: "INSERT INTO users (email) VALUES ($1)", want: "Insert"},
		{name: "update", sql: "UPDATE users SET email = $1", want: "Update"},
		{name: "delete", sql: "DELETE FROM users", want: "Delete"},
		{name: "with (CTE)", sql: "WITH recent AS (SELECT 1) SELECT * FROM recent", want: "With"},
		{name: "unrecognized keyword falls back to Query", sql: "BEGIN", want: "Query"},
		{name: "empty string falls back to Query", sql: "", want: "Query"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := guessQueryName(tt.sql); got != tt.want {
				t.Fatalf("guessQueryName(%q) = %q, want %q", tt.sql, got, tt.want)
			}
		})
	}
}

func TestParseQueryInfo(t *testing.T) {
	tests := []struct {
		name              string
		sql               string
		wantQueryName     string
		wantOperationType string
		wantCleanSQL      string
	}{
		{
			name:              "a sqlc name annotation is parsed",
			sql:               "-- name: GetUserByUUID :one\nSELECT * FROM users WHERE uuid = $1",
			wantQueryName:     "GetUserByUUID",
			wantOperationType: "ONE",
			wantCleanSQL:      "SELECT * FROM users WHERE uuid = $1",
		},
		{
			name:          "no annotation falls back to a guessed name",
			sql:           "SELECT * FROM users",
			wantQueryName: "Select",
			wantCleanSQL:  "SELECT * FROM users",
		},
		{
			name:          "comments and extra whitespace are stripped",
			sql:           "SELECT *\n  -- a comment\n  FROM   users\nWHERE  id = 1",
			wantQueryName: "Select",
			wantCleanSQL:  "SELECT * FROM users WHERE id = 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := parseQueryInfo(tt.sql)

			if info.QueryName != tt.wantQueryName {
				t.Errorf("QueryName = %q, want %q", info.QueryName, tt.wantQueryName)
			}
			if info.OperationType != tt.wantOperationType {
				t.Errorf("OperationType = %q, want %q", info.OperationType, tt.wantOperationType)
			}
			if info.CleanSQL != tt.wantCleanSQL {
				t.Errorf("CleanSQL = %q, want %q", info.CleanSQL, tt.wantCleanSQL)
			}
		})
	}
}

func TestGetQueryInfo_Caches(t *testing.T) {
	sql := "SELECT * FROM users WHERE uuid = $1"

	first := getQueryInfo(sql)

	cached, ok := queryInfoCache.Load(sql)
	if !ok {
		t.Fatal("getQueryInfo did not populate queryInfoCache")
	}
	if cached.(queryInfo) != first {
		t.Fatalf("cached entry = %+v, want %+v", cached, first)
	}

	if second := getQueryInfo(sql); second != first {
		t.Fatalf("second call returned %+v, want the same result %+v", second, first)
	}
}
