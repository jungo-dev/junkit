package tracer_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jungo-dev/junkit/tracer"
)

func TestFormatValue(t *testing.T) {
	sampleTime := time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)
	sampleUUID := uuid.MustParse("8040c036-b0a5-4178-8015-c4c4c6b37164")

	tests := []struct {
		name string
		v    any
		want string
	}{
		{name: "nil becomes NULL", v: nil, want: "NULL"},
		{name: "nil pointer becomes NULL", v: (*string)(nil), want: "NULL"},
		{name: "string is single-quoted", v: "hello", want: "'hello'"},
		{name: "a single quote is escaped by doubling", v: "O'Brien", want: "'O''Brien'"},
		{name: "bool true", v: true, want: "true"},
		{name: "time.Time is RFC3339", v: sampleTime, want: "'2026-01-15T10:30:00Z'"},
		{name: "uuid.UUID is quoted", v: sampleUUID, want: "'8040c036-b0a5-4178-8015-c4c4c6b37164'"},
		{name: "int", v: 42, want: "42"},
		{name: "float64", v: 3.5, want: "3.5"},
		{name: "empty slice becomes NULL", v: []int{}, want: "NULL"},
		{name: "int slice becomes a Postgres array literal", v: []int{1, 2, 3}, want: "'{1,2,3}'"},
		{name: "valid pgtype.UUID", v: pgtype.UUID{Bytes: sampleUUID, Valid: true}, want: "'8040c036-b0a5-4178-8015-c4c4c6b37164'"},
		{name: "invalid pgtype.Text is NULL", v: pgtype.Text{Valid: false}, want: "NULL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tracer.FormatValue(tt.v); got != tt.want {
				t.Fatalf("FormatValue(%#v) = %q, want %q", tt.v, got, tt.want)
			}
		})
	}
}

func TestBindArgsToSQL(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		args []any
		want string
	}{
		{name: "no args returns sql unchanged", sql: "SELECT * FROM users", args: nil, want: "SELECT * FROM users"},
		{
			name: "a placeholder is replaced with its formatted literal",
			sql:  "SELECT * FROM users WHERE email = $1",
			args: []any{"jane@example.com"},
			want: "SELECT * FROM users WHERE email = 'jane@example.com'",
		},
		{
			name: "double-digit placeholders are parsed correctly",
			sql:  "SELECT $10, $1",
			args: []any{"first", "s", "s", "s", "s", "s", "s", "s", "s", "tenth"},
			want: "SELECT 'tenth', 'first'",
		},
		{
			name: "an out-of-range placeholder is left as-is",
			sql:  "SELECT $1, $5",
			args: []any{"only one arg"},
			want: "SELECT 'only one arg', $5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tracer.BindArgsToSQL(tt.sql, tt.args); got != tt.want {
				t.Fatalf("BindArgsToSQL(%q, %v) = %q, want %q", tt.sql, tt.args, got, tt.want)
			}
		})
	}
}

// orderedFields extracts top-level JSON object keys in document order for testing.
func orderedFields(t *testing.T, data []byte) []string {
	t.Helper()

	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token() // consume '{'
	if err != nil || tok != json.Delim('{') {
		t.Fatalf("expected a JSON object, got token %v (err %v)", tok, err)
	}

	var keys []string
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			t.Fatalf("failed to read key token: %v", err)
		}
		keys = append(keys, keyTok.(string))

		var discard json.RawMessage
		if err := dec.Decode(&discard); err != nil {
			t.Fatalf("failed to read value token: %v", err)
		}
	}
	return keys
}

func TestNormalizeValue_StructFieldOrderIsPreserved(t *testing.T) {
	// Verify that normalizeStruct returns an order-preserving orderedMap to preserve struct field order.
	type sqlLogData struct {
		SQL        string  `json:"sql"`
		QueryName  string  `json:"query_name"`
		Operation  string  `json:"operation"`
		DurationMS float64 `json:"duration_ms"`
	}

	data := sqlLogData{SQL: "SELECT 1", QueryName: "GetUser", Operation: "ONE", DurationMS: 1.23}

	normalized := tracer.NormalizeValue(data)
	marshaled, err := json.Marshal(normalized)
	if err != nil {
		t.Fatalf("json.Marshal(NormalizeValue(...)) error = %v", err)
	}

	want := []string{"sql", "query_name", "operation", "duration_ms"}
	got := orderedFields(t, marshaled)
	if len(got) != len(want) {
		t.Fatalf("field order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("field order = %v, want %v (declared struct order)", got, want)
		}
	}
}

func TestNormalizeValue_OmitEmptyAndJSONTags(t *testing.T) {
	type row struct {
		ID       int    `json:"id"`
		Nickname string `json:"nickname,omitempty"`
		Ignored  string `json:"-"`
		hidden   string //nolint:unused // exercises the unexported-field skip
		DBOnly   int    `db:"legacy_col"`
	}

	marshaled, err := json.Marshal(tracer.NormalizeValue(row{ID: 1, DBOnly: 7}))
	if err != nil {
		t.Fatalf("json.Marshal error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(marshaled, &got); err != nil {
		t.Fatalf("json.Unmarshal error = %v", err)
	}

	if _, present := got["nickname"]; present {
		t.Error("an empty omitempty field should be dropped")
	}
	if _, present := got["Ignored"]; present {
		t.Error(`a field tagged json:"-" should never appear`)
	}
	if _, present := got["hidden"]; present {
		t.Error("an unexported field should never appear")
	}
	if got["legacy_col"] != float64(7) {
		t.Errorf(`DBOnly should appear under its "db" tag name "legacy_col", got %+v`, got)
	}
	if got["id"] != float64(1) {
		t.Errorf("id = %v, want 1", got["id"])
	}
}

func TestNormalizeValue_Pointer(t *testing.T) {
	if got := tracer.NormalizeValue((*string)(nil)); got != nil {
		t.Fatalf("NormalizeValue(nil pointer) = %v, want nil", got)
	}

	s := "hello"
	if got := tracer.NormalizeValue(&s); got != "hello" {
		t.Fatalf("NormalizeValue(&s) = %v, want %q", got, "hello")
	}
}

func TestNormalizeValue_UUIDBytes(t *testing.T) {
	id := uuid.MustParse("8040c036-b0a5-4178-8015-c4c4c6b37164")
	raw := [16]byte(id)

	got := tracer.NormalizeValue(raw)
	if got != id.String() {
		t.Fatalf("NormalizeValue([16]byte) = %v, want %q", got, id.String())
	}
}

func TestNormalizeValue_ByteSlice(t *testing.T) {
	t.Run("valid JSON bytes are parsed", func(t *testing.T) {
		got := tracer.NormalizeValue([]byte(`{"a":1}`))
		want := map[string]any{"a": float64(1)}
		gotMap, ok := got.(map[string]any)
		if !ok || gotMap["a"] != want["a"] {
			t.Fatalf("NormalizeValue(json bytes) = %#v, want %#v", got, want)
		}
	})

	t.Run("non-JSON, non-UUID-length bytes become hex", func(t *testing.T) {
		got := tracer.NormalizeValue([]byte{0xDE, 0xAD, 0xBE, 0xEF})
		if got != "deadbeef" {
			t.Fatalf("NormalizeValue(bytes) = %v, want %q", got, "deadbeef")
		}
	})
}

func TestNormalizeValue_PgtypeInvalidIsNil(t *testing.T) {
	got := tracer.NormalizeValue(pgtype.Text{Valid: false})
	if got != nil {
		t.Fatalf("NormalizeValue(invalid pgtype.Text) = %v, want nil", got)
	}

	got = tracer.NormalizeValue(pgtype.Text{String: "hi", Valid: true})
	if got != "hi" {
		t.Fatalf("NormalizeValue(valid pgtype.Text) = %v, want %q", got, "hi")
	}
}

func TestNormalizeValue_Map(t *testing.T) {
	got := tracer.NormalizeValue(map[string]int{"a": 1})
	m, ok := got.(map[string]any)
	if !ok || m["a"] != 1 {
		t.Fatalf("NormalizeValue(map) = %#v, want map[string]any{\"a\": 1}", got)
	}
}

func TestNormalizeValue_Slice(t *testing.T) {
	got := tracer.NormalizeValue([]int{1, 2, 3})
	s, ok := got.([]any)
	if !ok || len(s) != 3 || s[0] != 1 {
		t.Fatalf("NormalizeValue([]int{1,2,3}) = %#v, want []any{1, 2, 3}", got)
	}
}

func TestStringifyArgs(t *testing.T) {
	got := tracer.StringifyArgs([]any{1, "two", nil})
	want := []any{1, "two", nil}

	if len(got) != len(want) {
		t.Fatalf("StringifyArgs() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("StringifyArgs()[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
