package tracer

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// dbtx defines the common database query-execution interface shared by pool and transaction handles.
type dbtx interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// maxTracedRows caps the maximum number of rows captured per traced query for dashboard display.
const maxTracedRows = 20

// DBWrapper wraps a dbtx pool handle to record query execution data in Debugger.
type DBWrapper struct {
	inner dbtx
}

// NewDBWrapper creates a DBWrapper wrapping the provided dbtx handle.
//
// Usage:
//
//	database.Options{Decorate: func(inner database.DBTX) database.DBTX {
//	    return tracer.NewDBWrapper(inner)
//	}}
func NewDBWrapper(inner dbtx) *DBWrapper {
	return &DBWrapper{inner: inner}
}

// Exec implements dbtx, tracing the call (including rows affected) when debugging is enabled on ctx.
func (w *DBWrapper) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if !IsEnabledCtx(ctx) {
		return w.inner.Exec(ctx, sql, args...)
	}
	ts := beginTrace(ctx, sql, args)
	tag, err := w.inner.Exec(ctx, sql, args...)
	ts.end(ctx)
	recordQuery(ctx, ts, execResult(tag), err)
	return tag, err
}

// Query implements dbtx, wrapping rows to record query results when debugging is enabled.
func (w *DBWrapper) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if !IsEnabledCtx(ctx) {
		return w.inner.Query(ctx, sql, args...)
	}
	return tracedQuery(ctx, w.inner, sql, args)
}

// QueryRow implements dbtx, recording scanned row values when debugging is enabled.
func (w *DBWrapper) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if !IsEnabledCtx(ctx) {
		return w.inner.QueryRow(ctx, sql, args...)
	}
	return tracedQueryRow(ctx, w.inner, sql, args)
}

// TxWrapper wraps a transaction-scoped dbtx handle to trace query execution.
//
// Usage:
//
//	tx, _ := pool.Begin(ctx)
//	traced := tracer.NewTxWrapper(tx)
type TxWrapper struct {
	inner dbtx
}

// NewTxWrapper wraps inner, typically the pgx.Tx a transaction began with.
func NewTxWrapper(inner dbtx) *TxWrapper {
	return &TxWrapper{inner: inner}
}

// Exec implements dbtx, tracing the call (including rows affected) when debugging is enabled on ctx.
func (w *TxWrapper) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if !IsEnabledCtx(ctx) {
		return w.inner.Exec(ctx, sql, args...)
	}
	ts := beginTrace(ctx, sql, args)
	tag, err := w.inner.Exec(ctx, sql, args...)
	ts.end(ctx)
	recordQuery(ctx, ts, execResult(tag), err)
	return tag, err
}

// Query implements dbtx, tracing rows the same way DBWrapper.Query does.
func (w *TxWrapper) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if !IsEnabledCtx(ctx) {
		return w.inner.Query(ctx, sql, args...)
	}
	return tracedQuery(ctx, w.inner, sql, args)
}

// QueryRow implements dbtx, tracing the scanned row the same way DBWrapper.QueryRow does.
func (w *TxWrapper) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if !IsEnabledCtx(ctx) {
		return w.inner.QueryRow(ctx, sql, args...)
	}
	return tracedQueryRow(ctx, w.inner, sql, args)
}

// traceSpan captures a traced SQL call's starting state: its timing and
// timeline position (StartMS/depth), shared by Exec, Query, and QueryRow.
type traceSpan struct {
	start      time.Time
	startMS    float64
	depth      int
	info       queryInfo
	displaySQL string
}

// beginTrace starts query timing, reserves the query's slot on ctx's
// Debugger timeline, and returns parsed display metadata.
func beginTrace(ctx context.Context, sql string, args []any) traceSpan {
	info := getQueryInfo(sql)
	ts := traceSpan{
		start:      time.Now(),
		info:       info,
		displaySQL: BindArgsToSQL(info.CleanSQL, args),
	}
	if d := FromContext(ctx); d != nil {
		ts.startMS = d.elapsedMS()
		ts.depth = d.pushSpanDepth()
	}
	return ts
}

// end marks the traced query span as finished.
func (ts *traceSpan) end(ctx context.Context) {
	if d := FromContext(ctx); d != nil {
		d.popSpanDepth()
	}
}

// execResult builds the result payload for a traced Exec call.
func execResult(tag pgconn.CommandTag) sqlLogData {
	n := tag.RowsAffected()
	return sqlLogData{RowsAffected: &n}
}

// tracedQueryRow executes QueryRow and records scanned row values for tracing.
func tracedQueryRow(ctx context.Context, q dbtx, sql string, args []any) pgx.Row {
	ts := beginTrace(ctx, sql, args)

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		ts.end(ctx)
		recordQuery(ctx, ts, sqlLogData{}, err)
		return errRow{err}
	}

	return &tracedRow{ctx: ctx, ts: ts, rows: rows}
}

// errRow implements pgx.Row for deferred query errors.
type errRow struct{ err error }

// Scan implements pgx.Row.
func (r errRow) Scan(...any) error { return r.err }

// tracedRow wraps pgx.Row to record scanned row data for the debug dashboard.
type tracedRow struct {
	ctx  context.Context
	ts   traceSpan
	rows pgx.Rows
}

// Scan implements pgx.Row.
func (r *tracedRow) Scan(dest ...any) error {
	defer r.rows.Close()
	defer r.ts.end(r.ctx)

	if !r.rows.Next() {
		err := r.rows.Err()
		if err == nil {
			err = pgx.ErrNoRows
		}
		recordQuery(r.ctx, r.ts, sqlLogData{}, err)
		return err
	}

	err := r.rows.Scan(dest...)
	names := fieldNames(r.rows.FieldDescriptions())
	result := zipColumns(names, derefAll(dest))

	if err == nil {
		err = r.rows.Err()
	}
	recordQuery(r.ctx, r.ts, sqlLogData{Result: result}, err)
	return err
}

// tracedQuery wraps pgx.Query to record scanned rows for the debug dashboard.
func tracedQuery(ctx context.Context, q dbtx, sql string, args []any) (pgx.Rows, error) {
	ts := beginTrace(ctx, sql, args)

	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		ts.end(ctx)
		recordQuery(ctx, ts, sqlLogData{}, err)
		return rows, err
	}

	return &tracedRows{Rows: rows, ctx: ctx, ts: ts}, nil
}

// tracedRows wraps pgx.Rows to collect scanned rows and record them on Close.
type tracedRows struct {
	pgx.Rows
	ctx   context.Context
	ts    traceSpan
	rows  []map[string]any
	total int
}

// Scan implements pgx.Rows, collecting this row's values (up to maxTracedRows) alongside the real Scan.
func (r *tracedRows) Scan(dest ...any) error {
	err := r.Rows.Scan(dest...)
	if err != nil {
		return err
	}

	r.total++
	if len(r.rows) < maxTracedRows {
		names := fieldNames(r.Rows.FieldDescriptions())
		r.rows = append(r.rows, zipColumns(names, derefAll(dest)))
	}
	return nil
}

// Close implements pgx.Rows, recording the collected rows (and true total) once the caller is done.
func (r *tracedRows) Close() {
	r.Rows.Close()
	r.ts.end(r.ctx)

	data := sqlLogData{RowCount: &r.total, Rows: r.rows}
	if r.total > len(r.rows) {
		data.Truncated = fmt.Sprintf("showing first %d of %d rows", len(r.rows), r.total)
	}
	recordQuery(r.ctx, r.ts, data, r.Rows.Err())
}

// sqlLogData defines the dashboard entry payload for a traced SQL query.
type sqlLogData struct {
	SQL          string           `json:"sql"`
	QueryName    string           `json:"query_name"`
	Operation    string           `json:"operation"`
	DurationMS   float64          `json:"duration_ms"`
	StartMS      float64          `json:"start_ms"`
	Depth        int              `json:"depth"`
	Result       any              `json:"result,omitempty"`
	RowCount     *int             `json:"row_count,omitempty"`
	Rows         []map[string]any `json:"rows,omitempty"`
	Truncated    string           `json:"truncated,omitempty"`
	RowsAffected *int64           `json:"rows_affected,omitempty"`
	Error        string           `json:"error,omitempty"`
}

// recordQuery appends a traced SQL result log entry to the active Debugger.
func recordQuery(ctx context.Context, ts traceSpan, extra sqlLogData, err error) {
	d := FromContext(ctx)
	if d == nil {
		return
	}

	durMS := float64(time.Since(ts.start).Microseconds()) / 1000.0
	d.SetLastQueryInfo(ts.info.QueryName, ts.info.OperationType, ts.displaySQL, durMS)

	data := extra
	data.SQL = ts.displaySQL
	data.QueryName = ts.info.QueryName
	data.Operation = ts.info.OperationType
	data.DurationMS = durMS
	data.StartMS = ts.startMS
	data.Depth = ts.depth
	if err != nil {
		data.Error = err.Error()
	}

	d.Append(LogEntry{
		Type:  "sql_result",
		Label: fmt.Sprintf("%s | %.2fms", ts.info.QueryName, durMS),
		Data:  data,
	})
}

// derefAll dereferences pointer elements in dest by one level.
func derefAll(dest []any) []any {
	out := make([]any, len(dest))
	for i, d := range dest {
		v := reflect.ValueOf(d)
		if v.Kind() == reflect.Ptr && !v.IsNil() {
			out[i] = v.Elem().Interface()
		} else {
			out[i] = d
		}
	}
	return out
}

// fieldNames extracts column names from pgx field descriptions.
func fieldNames(fds []pgconn.FieldDescription) []string {
	names := make([]string, len(fds))
	for i, fd := range fds {
		names[i] = fd.Name
	}
	return names
}

// zipColumns pairs column names with values into a map.
func zipColumns(names []string, values []any) map[string]any {
	result := make(map[string]any, len(values))
	for i, v := range values {
		key := fmt.Sprintf("col_%d", i)
		if i < len(names) && names[i] != "" {
			key = names[i]
		}
		result[key] = v
	}
	return result
}
