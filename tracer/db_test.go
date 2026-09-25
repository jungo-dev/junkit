package tracer_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jungo-dev/junkit/tracer"
)

// fakeDB is a dbtx whose Query returns the configured rows (or error).
type fakeDB struct {
	cols     []string
	rows     [][]any
	queryErr error
}

func (f *fakeDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 3"), nil
}

func (f *fakeDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return &fakeRows{cols: f.cols, rows: f.rows, i: -1}, nil
}

func (f *fakeDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	panic("the tracing wrapper should route QueryRow through Query")
}

// fakeRows implements pgx.Rows over in-memory values; Scan supports *int, *string and *[]byte.
type fakeRows struct {
	cols []string
	rows [][]any
	i    int
}

func (r *fakeRows) Close()                        {}
func (r *fakeRows) Err() error                    { return nil }
func (r *fakeRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (r *fakeRows) Next() bool                    { r.i++; return r.i < len(r.rows) }
func (r *fakeRows) Values() ([]any, error)        { return r.rows[r.i], nil }
func (r *fakeRows) RawValues() [][]byte           { return nil }
func (r *fakeRows) Conn() *pgx.Conn               { return nil }

func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription {
	fds := make([]pgconn.FieldDescription, len(r.cols))
	for i, c := range r.cols {
		fds[i].Name = c
	}
	return fds
}

func (r *fakeRows) Scan(dest ...any) error {
	for i, d := range dest {
		switch p := d.(type) {
		case *int:
			*p = r.rows[r.i][i].(int)
		case *string:
			*p = r.rows[r.i][i].(string)
		case *[]byte:
			*p = r.rows[r.i][i].([]byte)
		}
	}
	return nil
}

// sqlEntries returns d's entries, asserting they are all SQL entries.
func sqlEntries(t *testing.T, d *tracer.Debugger) []tracer.LogEntry {
	t.Helper()
	logs := d.Logs()
	for _, e := range logs {
		if e.Type != tracer.TypeSQL {
			t.Fatalf("entry %+v is not a SQL entry", e)
		}
	}
	return logs
}

func TestDBWrapper_PassesThroughWithoutDebugger(t *testing.T) {
	db := tracer.NewDBWrapper(&fakeDB{cols: []string{"id"}, rows: [][]any{{1}}})

	rows, err := db.Query(context.Background(), "SELECT id FROM users")
	if err != nil {
		t.Fatal(err)
	}
	if _, traced := rows.(*fakeRows); !traced {
		t.Fatalf("Query without a Debugger returned %T, want the inner rows unwrapped", rows)
	}
}

func TestDBWrapper_Exec(t *testing.T) {
	ctx, d := newCtx()
	db := tracer.NewDBWrapper(&fakeDB{})

	if _, err := db.Exec(ctx, "-- name: TouchUsers :exec\nUPDATE users SET seen = $1", true); err != nil {
		t.Fatal(err)
	}

	logs := sqlEntries(t, d)
	if len(logs) != 1 || logs[0].Label != "TouchUsers" {
		t.Fatalf("Logs() = %+v, want one TouchUsers entry", logs)
	}
	data := logs[0].Data.(tracer.SQLData)
	if data.SQL != "UPDATE users SET seen = true" || data.Operation != "EXEC" {
		t.Errorf("SQLData = %+v, want the bound SQL and the EXEC operation", data)
	}
	if data.RowsAffected == nil || *data.RowsAffected != 3 {
		t.Errorf("RowsAffected = %v, want 3", data.RowsAffected)
	}
}

func TestDBWrapper_QueryRecordsRowsOnceOnClose(t *testing.T) {
	ctx, d := newCtx()
	db := tracer.NewDBWrapper(&fakeDB{
		cols: []string{"id", "meta"},
		rows: [][]any{{1, []byte(`{"a":1}`)}, {2, []byte{0xde, 0xad}}},
	})

	rows, err := db.Query(ctx, "SELECT id, meta FROM users")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int
		var meta []byte
		if err := rows.Scan(&id, &meta); err != nil {
			t.Fatal(err)
		}
	}
	rows.Close()
	rows.Close() // e.g. an explicit Close plus a deferred one

	logs := sqlEntries(t, d)
	if len(logs) != 1 {
		t.Fatalf("got %d SQL entries, want exactly 1 even when Close is called twice", len(logs))
	}

	data := logs[0].Data.(tracer.SQLData)
	if data.RowCount == nil || *data.RowCount != 2 {
		t.Fatalf("RowCount = %v, want 2", data.RowCount)
	}
	got, _ := json.Marshal(data.Rows)
	if want := `[{"id":1,"meta":{"a":1}},{"id":2,"meta":"dead"}]`; string(got) != want {
		t.Errorf("Rows = %s, want %s (JSON bytes kept as JSON, others as hex)", got, want)
	}
}

func TestDBWrapper_QueryRow(t *testing.T) {
	t.Run("records the scanned row", func(t *testing.T) {
		ctx, d := newCtx()
		db := tracer.NewDBWrapper(&fakeDB{cols: []string{"email"}, rows: [][]any{{"jane@example.com"}}})

		var email string
		if err := db.QueryRow(ctx, "SELECT email FROM users WHERE id = $1", 7).Scan(&email); err != nil {
			t.Fatal(err)
		}

		data := sqlEntries(t, d)[0].Data.(tracer.SQLData)
		if data.Result["email"] != "jane@example.com" {
			t.Errorf("Result = %v, want the scanned email", data.Result)
		}
		if data.SQL != "SELECT email FROM users WHERE id = 7" {
			t.Errorf("SQL = %q, want the bound SQL", data.SQL)
		}
	})

	t.Run("no rows is recorded as ErrNoRows", func(t *testing.T) {
		ctx, d := newCtx()
		db := tracer.NewDBWrapper(&fakeDB{cols: []string{"email"}})

		var email string
		if err := db.QueryRow(ctx, "SELECT email FROM users").Scan(&email); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("Scan error = %v, want pgx.ErrNoRows", err)
		}
		if data := sqlEntries(t, d)[0].Data.(tracer.SQLData); data.Error != pgx.ErrNoRows.Error() {
			t.Errorf("Error = %q, want %q", data.Error, pgx.ErrNoRows.Error())
		}
	})

	t.Run("a query error is returned from Scan and recorded", func(t *testing.T) {
		ctx, d := newCtx()
		boom := errors.New("connection refused")
		db := tracer.NewDBWrapper(&fakeDB{queryErr: boom})

		if err := db.QueryRow(ctx, "SELECT 1").Scan(); !errors.Is(err, boom) {
			t.Fatalf("Scan error = %v, want %v", err, boom)
		}
		data := sqlEntries(t, d)[0].Data.(tracer.SQLData)
		if data.Error != boom.Error() {
			t.Errorf("Error = %q, want %q", data.Error, boom.Error())
		}
		// The origin is the code that ran the query, not the tracer wrapper.
		if !strings.Contains(data.Location, "db_test.go:") || !strings.Contains(data.Function, "TestDBWrapper_QueryRow") {
			t.Errorf("Location, Function = %q, %q, want this test", data.Location, data.Function)
		}
	})

	t.Run("a successful query has no origin", func(t *testing.T) {
		ctx, d := newCtx()
		db := tracer.NewDBWrapper(&fakeDB{cols: []string{"email"}, rows: [][]any{{"a@b.c"}}})

		var email string
		_ = db.QueryRow(ctx, "SELECT email FROM users").Scan(&email)

		if data := sqlEntries(t, d)[0].Data.(tracer.SQLData); data.Location != "" {
			t.Errorf("Location = %q, want empty for a successful query", data.Location)
		}
	})
}
