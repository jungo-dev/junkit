package tracer

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jungo-dev/junkit/sqlfmt"
)

// dbtx defines the common query-execution interface shared by pool and transaction handles.
type dbtx interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// maxTracedRows caps the number of rows captured per traced query.
const maxTracedRows = 20

// SQLData is the payload of a traced SQL query entry.
type SQLData struct {
	SQL          string           `json:"sql,omitempty"`
	Operation    string           `json:"operation,omitempty"`
	Result       map[string]any   `json:"result,omitempty"`    // QueryRow
	Rows         []map[string]any `json:"rows,omitempty"`      // Query, up to maxTracedRows
	RowCount     *int             `json:"row_count,omitempty"` // Query, true total
	RowsAffected *int64           `json:"rows_affected,omitempty"`
	Error        string           `json:"error,omitempty"`
	Location     string           `json:"location,omitempty"` // failed queries: app code that ran it
	Function     string           `json:"function,omitempty"`

	query string // SQL before binding args; groups repeated queries for N+1 detection
}

// DBWrapper wraps a pool or transaction handle to record queries on ctx's Debugger.
type DBWrapper struct {
	inner dbtx
}

// NewDBWrapper wraps inner, which may be a pool or a pgx.Tx.
//
// Usage:
//
//	database.Options{Decorate: func(inner database.DBTX) database.DBTX {
//	    return tracer.NewDBWrapper(inner)
//	}}
func NewDBWrapper(inner dbtx) *DBWrapper {
	return &DBWrapper{inner: inner}
}

// Exec implements dbtx, recording rows affected when debugging is enabled on ctx.
func (w *DBWrapper) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	q := beginQuery(ctx, sql, args)
	if q == nil {
		return w.inner.Exec(ctx, sql, args...)
	}
	tag, err := w.inner.Exec(ctx, sql, args...)
	n := tag.RowsAffected()
	q.finish(SQLData{RowsAffected: &n}, err)
	return tag, err
}

// Query implements dbtx, recording scanned rows when debugging is enabled on ctx.
func (w *DBWrapper) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	q := beginQuery(ctx, sql, args)
	if q == nil {
		return w.inner.Query(ctx, sql, args...)
	}
	rows, err := w.inner.Query(ctx, sql, args...)
	if err != nil {
		q.finish(SQLData{}, err)
		return rows, err
	}
	return &tracedRows{Rows: rows, q: q}, nil
}

// QueryRow implements dbtx, recording the scanned row when debugging is enabled on ctx.
func (w *DBWrapper) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	q := beginQuery(ctx, sql, args)
	if q == nil {
		return w.inner.QueryRow(ctx, sql, args...)
	}
	// Query instead of QueryRow so the column names are available at Scan time.
	rows, err := w.inner.Query(ctx, sql, args...)
	if err != nil {
		q.finish(SQLData{}, err)
		return errRow{err}
	}
	return &tracedRow{rows: rows, q: q}
}

// pendingQuery is a traced query that has started but not yet been recorded.
type pendingQuery struct {
	d     *Debugger
	start time.Time
	atMS  float64
	info  sqlfmt.Info
	sql   string // SQL with arguments bound, for display
}

// beginQuery starts tracing a query, or returns nil when ctx has no Debugger.
func beginQuery(ctx context.Context, sql string, args []any) *pendingQuery {
	d := FromContext(ctx)
	if d == nil {
		return nil
	}
	info := sqlfmt.Parse(sql)
	return &pendingQuery{
		d:     d,
		start: time.Now(),
		atMS:  d.sinceStartMS(),
		info:  info,
		sql:   sqlfmt.Bind(info.SQL, args),
	}
}

// finish records the query with its result payload and error.
func (q *pendingQuery) finish(data SQLData, err error) {
	data.SQL = q.sql
	data.query = q.info.SQL
	data.Operation = q.info.Operation
	if err != nil {
		data.Error = err.Error()
		data.Location, data.Function = sqlfmt.Origin()
	}
	q.d.Append(LogEntry{Type: TypeSQL, Label: q.info.Name, AtMS: q.atMS, DurationMS: msSince(q.start), Data: data})
}

// errRow implements pgx.Row for a query that failed before any row was read.
type errRow struct{ err error }

// Scan implements pgx.Row.
func (r errRow) Scan(...any) error { return r.err }

// tracedRow implements pgx.Row, recording the scanned row.
type tracedRow struct {
	rows pgx.Rows
	q    *pendingQuery
}

// Scan implements pgx.Row.
func (r *tracedRow) Scan(dest ...any) error {
	defer r.rows.Close()

	if !r.rows.Next() {
		err := r.rows.Err()
		if err == nil {
			err = pgx.ErrNoRows
		}
		r.q.finish(SQLData{}, err)
		return err
	}

	err := r.rows.Scan(dest...)
	if err == nil {
		err = r.rows.Err()
	}
	r.q.finish(SQLData{Result: rowMap(r.rows.FieldDescriptions(), dest)}, err)
	return err
}

// tracedRows wraps pgx.Rows, collecting scanned rows and recording them on Close.
type tracedRows struct {
	pgx.Rows
	q      *pendingQuery
	rows   []map[string]any
	total  int
	closed bool
}

// Scan implements pgx.Rows, capturing up to maxTracedRows rows.
func (r *tracedRows) Scan(dest ...any) error {
	if err := r.Rows.Scan(dest...); err != nil {
		return err
	}
	r.total++
	if len(r.rows) < maxTracedRows {
		r.rows = append(r.rows, rowMap(r.Rows.FieldDescriptions(), dest))
	}
	return nil
}

// Close implements pgx.Rows, recording the query once even if called repeatedly.
func (r *tracedRows) Close() {
	r.Rows.Close()
	if r.closed {
		return
	}
	r.closed = true
	r.q.finish(SQLData{Rows: r.rows, RowCount: &r.total}, r.Rows.Err())
}

// rowMap pairs column names with the scanned values in dest.
func rowMap(fds []pgconn.FieldDescription, dest []any) map[string]any {
	row := make(map[string]any, len(dest))
	for i, d := range dest {
		key := fmt.Sprintf("col_%d", i)
		if i < len(fds) && fds[i].Name != "" {
			key = fds[i].Name
		}
		row[key] = displayValue(d)
	}
	return row
}

// displayValue dereferences a Scan destination and makes raw bytes readable:
// JSON (e.g. a jsonb column) is kept as JSON, anything else becomes hex.
func displayValue(dest any) any {
	v := reflect.ValueOf(dest)
	if v.Kind() == reflect.Pointer && !v.IsNil() {
		dest = v.Elem().Interface()
	}
	if b, ok := dest.([]byte); ok {
		if json.Valid(b) {
			return json.RawMessage(b)
		}
		return hex.EncodeToString(b)
	}
	return dest
}
