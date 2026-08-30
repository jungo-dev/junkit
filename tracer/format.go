package tracer

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

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

// queryInfoCache memoizes parseQueryInfo results by SQL string.
var queryInfoCache sync.Map // sql string -> queryInfo

// getQueryInfo returns parsed queryInfo for sql, caching the result.
func getQueryInfo(sql string) queryInfo {
	if v, ok := queryInfoCache.Load(sql); ok {
		return v.(queryInfo)
	}
	info := parseQueryInfo(sql)
	queryInfoCache.Store(sql, info)
	return info
}

// parseQueryInfo parses SQL name annotations and cleans the SQL string.
func parseQueryInfo(sql string) queryInfo {
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
	return info
}

// guessQueryName infers a display name from the query's leading SQL keyword.
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

// FormatValue renders v as a Postgres SQL literal for display and logging.
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

// BindArgsToSQL binds argument literals into SQL placeholder positions ($1, $2, ...).
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

// formatPgtypeForSQL renders a pgx/v5/pgtype value as a SQL literal, or "" if unrecognized.
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

// NormalizeValue converts v into a JSON-serializable representation for debug rendering.
//
// Usage:
//
//	rendered := tracer.NormalizeValue(someStruct)
func NormalizeValue(v any) any {
	if v == nil {
		return nil
	}

	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
		v = rv.Interface()
	}

	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8 {
		return normalizeByteSlice(rv.Bytes())
	}
	if rv.Kind() == reflect.Array && rv.Len() == 16 && rv.Type().Elem().Kind() == reflect.Uint8 {
		var b [16]byte
		reflect.Copy(reflect.ValueOf(&b).Elem(), rv)
		if u, err := uuid.FromBytes(b[:]); err == nil {
			return u.String()
		}
		return hex.EncodeToString(b[:])
	}
	if rv.Type().PkgPath() == "github.com/jackc/pgx/v5/pgtype" {
		return normalizePgtype(rv)
	}
	if t, ok := v.(time.Time); ok {
		return t.Format(time.RFC3339Nano)
	}

	switch rv.Kind() {
	case reflect.Struct:
		return normalizeStruct(rv)
	case reflect.Map:
		return normalizeMap(rv)
	case reflect.Slice, reflect.Array:
		a := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			a[i] = NormalizeValue(rv.Index(i).Interface())
		}
		return a
	default:
		return v
	}
}

// normalizeByteSlice converts []byte to JSON, UUID string, or hex representation.
func normalizeByteSlice(b []byte) any {
	if len(b) > 0 {
		var parsed any
		if json.Unmarshal(b, &parsed) == nil {
			return parsed
		}
	}
	if len(b) == 16 {
		if u, err := uuid.FromBytes(b); err == nil {
			return u.String()
		}
	}
	return hex.EncodeToString(b)
}

// normalizeStruct converts a struct into an orderedMap of exported fields.
func normalizeStruct(rv reflect.Value) any {
	rt := rv.Type()
	m := make(orderedMap, 0, rv.NumField())

	for i := 0; i < rv.NumField(); i++ {
		f := rt.Field(i)
		if !f.IsExported() {
			continue
		}

		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		if tag == "" {
			tag = f.Tag.Get("db")
		}

		key := f.Name
		omitEmpty := false
		if tag != "" {
			parts := strings.Split(tag, ",")
			if parts[0] != "" {
				key = parts[0]
			}
			for _, p := range parts[1:] {
				if p == "omitempty" {
					omitEmpty = true
				}
			}
		}

		fieldVal := rv.Field(i)
		if omitEmpty && isEmptyValue(fieldVal) {
			continue
		}

		m = append(m, orderedPair{Key: key, Value: NormalizeValue(fieldVal.Interface())})
	}
	return m
}

// isEmptyValue checks whether v is its zero value for omitempty filtering.
func isEmptyValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return v.IsNil()
	default:
		return false
	}
}

// orderedPair is one key/value entry in an orderedMap.
type orderedPair struct {
	Key   string
	Value any
}

// orderedMap preserves struct field insertion order during JSON marshaling.
type orderedMap []orderedPair

// MarshalJSON implements json.Marshaler, writing m's entries in order.
func (m orderedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')

	for i, p := range m {
		if i > 0 {
			buf.WriteByte(',')
		}

		keyBytes, err := json.Marshal(p.Key)
		if err != nil {
			return nil, err
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')

		valBytes, err := json.Marshal(p.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(valBytes)
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// normalizeMap renders a map with string-formatted keys.
func normalizeMap(rv reflect.Value) any {
	m := make(map[string]any, rv.Len())
	for _, k := range rv.MapKeys() {
		key := fmt.Sprint(NormalizeValue(k.Interface()))
		m[key] = NormalizeValue(rv.MapIndex(k).Interface())
	}
	return m
}

// normalizePgtype converts a pgtype struct into its Go representation.
func normalizePgtype(rv reflect.Value) any {
	if valid := rv.FieldByName("Valid"); valid.IsValid() && valid.Kind() == reflect.Bool && !valid.Bool() {
		return nil
	}

	v := rv.Interface()
	switch x := v.(type) {
	case pgtype.UUID:
		return uuid.UUID(x.Bytes).String()
	case pgtype.Timestamptz:
		return x.Time.Format(time.RFC3339Nano)
	case pgtype.Timestamp:
		return x.Time.Format("2006-01-02 15:04:05")
	case pgtype.Date:
		return x.Time.Format("2006-01-02")
	case pgtype.Text:
		return x.String
	case pgtype.Int8:
		return x.Int64
	case pgtype.Int4:
		return x.Int32
	case pgtype.Int2:
		return x.Int16
	case pgtype.Bool:
		return x.Bool
	default:
		return fmt.Sprint(v)
	}
}

// StringifyArgs normalizes elements of args for logging and display.
func StringifyArgs(args []any) []any {
	out := make([]any, len(args))
	for i, a := range args {
		out[i] = NormalizeValue(a)
	}
	return out
}
