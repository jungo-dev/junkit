// Package tracer records request-scoped debug information (notes, values,
// errors, spans and SQL queries) and renders it as a debug dashboard.
//
// Recording only happens when a Debugger is attached to the context, which
// middleware.TracerDebug does for requests carrying the debug query key.
// Without one, every function in this package is a cheap no-op.
package tracer

import (
	"context"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Entry types recorded by this package.
const (
	TypeComment  = "comment"
	TypeVariable = "variable"
	TypeWarning  = "warning"
	TypeError    = "error"
	TypeSpan     = "span"
	TypeSQL      = "sql"
)

// debuggerContextKey is the context key under which a *Debugger is stored.
type debuggerContextKey struct{}

// BreakpointSignal is panicked by Stop to halt request processing for debugging.
type BreakpointSignal struct{}

// LogEntry is one recorded unit of debug information.
type LogEntry struct {
	Type       string  `json:"type"`
	Label      string  `json:"label"`
	AtMS       float64 `json:"at_ms"`                 // offset from request start at which the entry began
	DurationMS float64 `json:"duration_ms,omitempty"` // set for spans and SQL queries
	Data       any     `json:"data,omitempty"`
}

// Debugger accumulates LogEntry values for a single request.
// All methods are safe for concurrent use and on a nil receiver.
type Debugger struct {
	mu        sync.Mutex
	startedAt time.Time
	logs      []LogEntry
}

// New creates an empty Debugger whose timeline starts now.
func New() *Debugger {
	return &Debugger{startedAt: time.Now(), logs: make([]LogEntry, 0, 32)}
}

// sinceStartMS returns the milliseconds elapsed since d was created.
func (d *Debugger) sinceStartMS() float64 {
	return msSince(d.startedAt)
}

// msSince returns the milliseconds elapsed since t, at microsecond precision.
func msSince(t time.Time) float64 {
	return float64(time.Since(t).Microseconds()) / 1000.0
}

// Append records a log entry.
func (d *Debugger) Append(entry LogEntry) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.logs = append(d.logs, entry)
	d.mu.Unlock()
}

// record appends an entry of the given type, stamped with the current offset.
func (d *Debugger) record(typ, label string, data any) {
	if d == nil {
		return
	}
	d.Append(LogEntry{Type: typ, Label: label, AtMS: d.sinceStartMS(), Data: data})
}

// Logs returns a snapshot copy of every entry recorded so far.
func (d *Debugger) Logs() []LogEntry {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.logs) == 0 {
		return nil
	}
	cp := make([]LogEntry, len(d.logs))
	copy(cp, d.logs)
	return cp
}

// WithContext returns a copy of ctx carrying d, retrievable via FromContext.
func WithContext(ctx context.Context, d *Debugger) context.Context {
	return context.WithValue(ctx, debuggerContextKey{}, d)
}

// FromContext retrieves the Debugger attached to ctx, or nil if none is attached.
// A *gin.Context is resolved through its request context, since gin only
// falls back to it when the engine enables ContextWithFallback.
func FromContext(ctx context.Context) *Debugger {
	if gc, ok := ctx.(*gin.Context); ok {
		if gc == nil || gc.Request == nil {
			return nil
		}
		ctx = gc.Request.Context()
	}
	if ctx == nil {
		return nil
	}
	d, _ := ctx.Value(debuggerContextKey{}).(*Debugger)
	return d
}

// Enabled reports whether ctx carries a Debugger, i.e. whether tracer calls
// on ctx record anything. Use it to skip expensive debug-only work.
func Enabled(ctx context.Context) bool {
	return FromContext(ctx) != nil
}
