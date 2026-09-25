package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/tracelog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jungo-dev/junkit/logger"
	"github.com/jungo-dev/junkit/sqlfmt"
)

// queryTracer implements pgx tracelog.Logger using zap for query logging and slow query detection.
type queryTracer struct {
	log                *zap.Logger
	slowQueryThreshold time.Duration
}

// newQueryTracer creates a new queryTracer instance.
//
// Its logger drops zap's caller and error stack trace: the caller is always
// this file, and the stack is mostly pgx/gin frames. A failed query logs the
// application code location that ran it instead (see sqlfmt.Origin).
func newQueryTracer(log *zap.Logger, slowQueryThreshold time.Duration) *queryTracer {
	log = log.WithOptions(zap.WithCaller(false), zap.AddStacktrace(zapcore.FatalLevel))
	return &queryTracer{log: log, slowQueryThreshold: slowQueryThreshold}
}

// Log implements tracelog.Logger for query execution events and slow-query warnings.
func (t *queryTracer) Log(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]any) {
	if msg != "Query" {
		return
	}

	zapLevel := mapLogLevel(level)
	duration, _ := data["time"].(time.Duration)

	isSlow := t.slowQueryThreshold > 0 && duration > t.slowQueryThreshold
	if isSlow && zapLevel < zap.WarnLevel {
		zapLevel = zap.WarnLevel
	}

	if !t.log.Core().Enabled(zapLevel) {
		return
	}

	sql, _ := data["sql"].(string)
	rawArgs, _ := data["args"].([]any)
	info := sqlfmt.Parse(sql)
	finalSQL := sqlfmt.Bind(info.SQL, rawArgs)

	fields := make([]zap.Field, 0, 6)
	fields = append(fields,
		zap.String("trace_id", logger.GetTraceID(ctx)),
		zap.Duration("latency", duration),
		zap.String("query_name", info.Name),
		zap.String("operation", info.Operation),
		zap.String("sql", finalSQL),
	)
	if err, ok := data["err"].(error); ok {
		location, function := sqlfmt.Origin()
		fields = append(fields,
			zap.Error(err),
			zap.String("location", location),
			zap.String("function", function),
		)
	}

	event := "SQL_EXECUTION"
	if isSlow {
		event = "SLOW_QUERY_DETECTION"
	}
	t.log.Log(zapLevel, event, fields...)
}

// mapLogLevel converts a pgx tracelog level to the equivalent zap level.
func mapLogLevel(level tracelog.LogLevel) zapcore.Level {
	switch level {
	case tracelog.LogLevelError:
		return zap.ErrorLevel
	case tracelog.LogLevelWarn:
		return zap.WarnLevel
	case tracelog.LogLevelInfo:
		return zap.InfoLevel
	case tracelog.LogLevelDebug, tracelog.LogLevelTrace:
		return zap.DebugLevel
	default:
		return zap.InfoLevel
	}
}

// FormatValue renders v as a Postgres SQL literal (for query logging only).
//
// Usage:
//
//	database.FormatValue("O'Brien") // "'O''Brien'"
func FormatValue(v any) string {
	return sqlfmt.FormatValue(v)
}

// BindArgsToSQL replaces Postgres placeholders ($1, $2, ...) in sql with argument literals for logging.
func BindArgsToSQL(sql string, args []any) string {
	return sqlfmt.Bind(sql, args)
}
