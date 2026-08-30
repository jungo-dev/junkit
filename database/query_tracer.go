package database

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/tracelog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/jungo-dev/junkit/logger"
)

// queryTracer implements pgx tracelog.Logger using zap for query logging and slow query detection.
type queryTracer struct {
	log                *zap.Logger
	slowQueryThreshold time.Duration
	parseCache         sync.Map
}

// newQueryTracer creates a new queryTracer instance.
func newQueryTracer(log *zap.Logger, slowQueryThreshold time.Duration) *queryTracer {
	return &queryTracer{log: log, slowQueryThreshold: slowQueryThreshold}
}

// queryInfo holds metadata extracted from a SQL query string.
type queryInfo struct {
	QueryName     string
	OperationType string
	CleanSQL      string
}

var (
	sqlcNameRegex = regexp.MustCompile(`-- name:\s*(\w+)\s*:(\w+)`)
	spaceRegex    = regexp.MustCompile(`\s+`)
	commentRegex  = regexp.MustCompile(`-- [^\r\n]*`)
)

// getQueryInfo extracts and caches metadata (query name, operation type, clean SQL) from a SQL string.
func (t *queryTracer) getQueryInfo(sql string) queryInfo {
	if val, ok := t.parseCache.Load(sql); ok {
		return val.(queryInfo)
	}

	var info queryInfo
	if matches := sqlcNameRegex.FindStringSubmatch(sql); len(matches) == 3 {
		info.QueryName = matches[1]
		info.OperationType = strings.ToUpper(matches[2])
	}

	cleanSQL := commentRegex.ReplaceAllString(sql, "")
	cleanSQL = strings.TrimSpace(cleanSQL)
	cleanSQL = spaceRegex.ReplaceAllString(cleanSQL, " ")
	info.CleanSQL = cleanSQL

	if info.QueryName == "" {
		info.QueryName = guessQueryName(cleanSQL)
	}

	t.parseCache.Store(sql, info)
	return info
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
	info := t.getQueryInfo(sql)

	finalSQL := info.CleanSQL
	if len(rawArgs) > 0 {
		finalSQL = BindArgsToSQL(info.CleanSQL, rawArgs)
	}

	fields := make([]zap.Field, 0, 6)
	fields = append(fields,
		zap.String("trace_id", logger.GetTraceID(ctx)),
		zap.Duration("latency", duration),
		zap.String("query_name", info.QueryName),
		zap.String("operation", info.OperationType),
		zap.String("sql", finalSQL),
	)
	if err, ok := data["err"].(error); ok {
		fields = append(fields, zap.Error(err))
	}

	event := "SQL_EXECUTION"
	if isSlow {
		event = "SLOW_QUERY_DETECTION"
	}
	t.log.Log(zapLevel, event, fields...)
}

// guessQueryName infers a display name from the query's leading SQL keyword,
// used when a query has no sqlc name annotation.
func guessQueryName(sql string) string {
	firstWord, _, _ := strings.Cut(sql, " ")

	switch strings.ToUpper(firstWord) {
	case "SELECT":
		return "Select"
	case "INSERT":
		return "Insert"
	case "UPDATE":
		return "Update"
	case "DELETE":
		return "Delete"
	case "WITH":
		return "With"
	default:
		return "Query"
	}
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
	if v == nil {
		return "NULL"
	}

	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return "NULL"
		}
		rv = rv.Elem()
	}
	v = rv.Interface()

	if s := formatPgtypeForSQL(v); s != "" {
		return s
	}

	switch x := v.(type) {
	case string:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(x, "'", "''"))
	case bool:
		return fmt.Sprintf("%t", x)
	case time.Time:
		return fmt.Sprintf("'%s'", x.Format(time.RFC3339))
	case uuid.UUID:
		return fmt.Sprintf("'%s'", x.String())
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", x)
	case float32, float64:
		return fmt.Sprintf("%.15g", x)
	default:
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			return formatSliceForSQL(rv)
		}
		return fmt.Sprintf("'%s'", strings.ReplaceAll(fmt.Sprintf("%v", x), "'", "''"))
	}
}

// BindArgsToSQL replaces Postgres placeholders ($1, $2, ...) in sql with argument literals for logging.
func BindArgsToSQL(sql string, args []any) string {
	if len(args) == 0 {
		return sql
	}

	var sb strings.Builder
	sb.Grow(len(sql) + len(args)*16)

	for i := 0; i < len(sql); i++ {
		if sql[i] == '$' && i+1 < len(sql) && sql[i+1] >= '0' && sql[i+1] <= '9' {
			idx := 0
			j := i + 1
			for j < len(sql) && sql[j] >= '0' && sql[j] <= '9' {
				idx = idx*10 + int(sql[j]-'0')
				j++
			}

			if idx > 0 && idx <= len(args) {
				sb.WriteString(FormatValue(args[idx-1]))
				i = j - 1
				continue
			}
		}
		sb.WriteByte(sql[i])
	}

	return sb.String()
}

// formatSliceForSQL renders a Go slice/array as a Postgres array literal.
// An empty slice becomes NULL, matching how pgx encodes it over the wire.
func formatSliceForSQL(rv reflect.Value) string {
	if rv.Len() == 0 {
		return "NULL"
	}
	parts := make([]string, rv.Len())
	for i := range parts {
		parts[i] = FormatValue(rv.Index(i).Interface())
	}
	return fmt.Sprintf("'{%s}'", strings.Join(parts, ","))
}

// formatPgtypeForSQL renders a pgtype value as a SQL literal or returns "".
func formatPgtypeForSQL(v any) string {
	switch x := v.(type) {
	case pgtype.UUID:
		if x.Valid {
			return fmt.Sprintf("'%s'", uuid.UUID(x.Bytes).String())
		}
		return "NULL"
	case pgtype.Text:
		if x.Valid {
			return fmt.Sprintf("'%s'", strings.ReplaceAll(x.String, "'", "''"))
		}
		return "NULL"
	case pgtype.Int8:
		if x.Valid {
			return fmt.Sprintf("%d", x.Int64)
		}
		return "NULL"
	case pgtype.Int4:
		if x.Valid {
			return fmt.Sprintf("%d", x.Int32)
		}
		return "NULL"
	case pgtype.Int2:
		if x.Valid {
			return fmt.Sprintf("%d", x.Int16)
		}
		return "NULL"
	case pgtype.Bool:
		if x.Valid {
			return fmt.Sprintf("%t", x.Bool)
		}
		return "NULL"
	case pgtype.Timestamptz:
		if x.Valid {
			return fmt.Sprintf("'%s'", x.Time.Format(time.RFC3339))
		}
		return "NULL"
	case pgtype.Timestamp:
		if x.Valid {
			return fmt.Sprintf("'%s'", x.Time.Format("2006-01-02 15:04:05"))
		}
		return "NULL"
	case pgtype.Date:
		if x.Valid {
			return fmt.Sprintf("'%s'", x.Time.Format("2006-01-02"))
		}
		return "NULL"
	}
	return ""
}
