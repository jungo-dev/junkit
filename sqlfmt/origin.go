package sqlfmt

import (
	"fmt"
	"runtime"
	"strings"
)

// originSkipped lists function prefixes that sit between application code
// and a query logger: pgx, and junkit's own database/tracer wrappers.
var originSkipped = []string{
	"github.com/jackc/pgx/",
	"github.com/jungo-dev/junkit/database.",
	"github.com/jungo-dev/junkit/tracer.",
	"github.com/jungo-dev/junkit/sqlfmt.",
}

// Origin returns the "file:line" and function of the application code that
// ran the current query. sqlc-generated files (*.sql.go) are skipped too, so
// it points at the repository/service calling the generated method.
func Origin() (location, function string) {
	pc := make([]uintptr, 32)
	n := runtime.Callers(2, pc) // skip runtime.Callers and Origin
	frames := runtime.CallersFrames(pc[:n])

	for {
		f, more := frames.Next()
		if !isOriginSkipped(f) {
			return fmt.Sprintf("%s:%d", f.File, f.Line), f.Function
		}
		if !more {
			return "unknown", "unknown"
		}
	}
}

// isOriginSkipped reports whether f is runtime, driver, wrapper, or generated code.
func isOriginSkipped(f runtime.Frame) bool {
	if strings.HasPrefix(f.Function, "runtime.") || strings.HasSuffix(f.File, ".sql.go") {
		return true
	}
	for _, p := range originSkipped {
		if strings.HasPrefix(f.Function, p) {
			return true
		}
	}
	return false
}
