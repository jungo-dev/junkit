// Package sqlfmt parses sqlc query annotations, renders SQL with bound
// arguments, and locates the code that ran a query, for logging and debugging.
// It is shared by database and tracer.
package sqlfmt

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Info holds metadata extracted from a SQL query string.
type Info struct {
	Name      string // sqlc query name, or a guess from the leading keyword
	Operation string // sqlc operation (ONE, MANY, EXEC, ...), empty if unannotated
	SQL       string // query with comments stripped and whitespace collapsed
}

var (
	sqlcNameRegex = regexp.MustCompile(`-- name:\s*(\w+)\s*:(\w+)`)
	spaceRegex    = regexp.MustCompile(`\s+`)
	commentRegex  = regexp.MustCompile(`-- [^\r\n]*`)
)

// maxCached bounds the parse cache so dynamically built SQL can't grow it forever.
const maxCached = 1024

var (
	cache     sync.Map // sql string -> Info
	cacheSize atomic.Int32
)

// Parse returns the Info for sql, caching up to maxCached distinct queries.
func Parse(sql string) Info {
	if v, ok := cache.Load(sql); ok {
		return v.(Info)
	}
	info := parse(sql)
	if cacheSize.Load() < maxCached {
		if _, loaded := cache.LoadOrStore(sql, info); !loaded {
			cacheSize.Add(1)
		}
	}
	return info
}

// parse extracts the sqlc annotation and cleans the SQL string.
func parse(sql string) Info {
	var info Info
	if m := sqlcNameRegex.FindStringSubmatch(sql); len(m) == 3 {
		info.Name = m[1]
		info.Operation = strings.ToUpper(m[2])
	}

	clean := commentRegex.ReplaceAllString(sql, "")
	clean = spaceRegex.ReplaceAllString(strings.TrimSpace(clean), " ")
	info.SQL = clean

	if info.Name == "" {
		info.Name = guessName(clean)
	}
	return info
}

// guessName infers a display name from the query's leading SQL keyword.
func guessName(sql string) string {
	first, _, _ := strings.Cut(sql, " ")
	switch strings.ToUpper(first) {
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

// Bind replaces Postgres placeholders ($1, $2, ...) in sql with argument literals.
func Bind(sql string, args []any) string {
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

// FormatValue renders v as a Postgres SQL literal.
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

	if s, ok := formatPgtype(v); ok {
		return s
	}

	switch x := v.(type) {
	case string:
		return quote(x)
	case bool:
		return fmt.Sprintf("%t", x)
	case time.Time:
		return quote(x.Format(time.RFC3339))
	case uuid.UUID:
		return quote(x.String())
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", x)
	case float32, float64:
		return fmt.Sprintf("%.15g", x)
	default:
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			return formatSlice(rv)
		}
		return quote(fmt.Sprintf("%v", x))
	}
}

// quote wraps s in single quotes, escaping embedded quotes by doubling them.
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// formatSlice renders a Go slice/array as a Postgres array literal.
// An empty slice becomes NULL, matching how pgx encodes it over the wire.
func formatSlice(rv reflect.Value) string {
	if rv.Len() == 0 {
		return "NULL"
	}
	parts := make([]string, rv.Len())
	for i := range parts {
		parts[i] = FormatValue(rv.Index(i).Interface())
	}
	return fmt.Sprintf("'{%s}'", strings.Join(parts, ","))
}

// formatPgtype renders a pgtype value as a SQL literal; ok is false for other types.
func formatPgtype(v any) (s string, ok bool) {
	null := func(valid bool, lit string) (string, bool) {
		if !valid {
			return "NULL", true
		}
		return lit, true
	}

	switch x := v.(type) {
	case pgtype.UUID:
		return null(x.Valid, quote(uuid.UUID(x.Bytes).String()))
	case pgtype.Text:
		return null(x.Valid, quote(x.String))
	case pgtype.Int8:
		return null(x.Valid, fmt.Sprintf("%d", x.Int64))
	case pgtype.Int4:
		return null(x.Valid, fmt.Sprintf("%d", x.Int32))
	case pgtype.Int2:
		return null(x.Valid, fmt.Sprintf("%d", x.Int16))
	case pgtype.Bool:
		return null(x.Valid, fmt.Sprintf("%t", x.Bool))
	case pgtype.Timestamptz:
		return null(x.Valid, quote(x.Time.Format(time.RFC3339)))
	case pgtype.Timestamp:
		return null(x.Valid, quote(x.Time.Format("2006-01-02 15:04:05")))
	case pgtype.Date:
		return null(x.Valid, quote(x.Time.Format("2006-01-02")))
	}
	return "", false
}
