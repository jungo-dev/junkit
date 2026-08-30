// Package tracer provides request-scoped debug logging and fallback Zap logger integration.
package tracer

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// debuggerContextKey is the context key under which a *Debugger is stored.
type debuggerContextKey struct{}

// BreakpointSignal is panicked by Stop to halt request processing for debugging.
type BreakpointSignal struct{}

// LastQueryInfo captures metadata for the most recently executed SQL query.
type LastQueryInfo struct {
	QueryName  string
	Operation  string
	FinalSQL   string
	DurationMS float64
}

// LogEntry is one recorded unit of debug information.
type LogEntry struct {
	Type  string `json:"type"` // "comment", "variable", "warning", "error", "sql_result", "span"
	Label string `json:"label"`
	Data  any    `json:"data,omitempty"`
}

// Debugger accumulates LogEntry values for a single request in a thread-safe manner.
type Debugger struct {
	mu            sync.RWMutex
	enabled       bool
	logs          []LogEntry
	lastQueryMu   sync.RWMutex
	lastQueryInfo LastQueryInfo
	startedAt     time.Time
	activeSpans   atomic.Int32
}

// New creates an empty, disabled Debugger instance.
func New() *Debugger {
	return &Debugger{logs: make([]LogEntry, 0, 32), startedAt: time.Now()}
}

// elapsedMS returns the milliseconds elapsed since d was created, used to
// place spans and traced queries on the dashboard timeline.
func (d *Debugger) elapsedMS() float64 {
	return float64(time.Since(d.startedAt).Microseconds()) / 1000.0
}

// pushSpanDepth marks a span or query as active and returns its nesting depth (0 for outermost).
func (d *Debugger) pushSpanDepth() int {
	return int(d.activeSpans.Add(1)) - 1
}

// popSpanDepth marks one active span/query as finished.
func (d *Debugger) popSpanDepth() {
	d.activeSpans.Add(-1)
}

// WithContext returns a copy of ctx carrying d, retrievable via FromContext.
func WithContext(ctx context.Context, d *Debugger) context.Context {
	return context.WithValue(ctx, debuggerContextKey{}, d)
}

// FromContext retrieves the Debugger attached to ctx by WithContext, or nil if none is attached.
func FromContext(ctx context.Context) *Debugger {
	d, _ := ctx.Value(debuggerContextKey{}).(*Debugger)
	return d
}

// Enable activates log collection for the Debugger.
func (d *Debugger) Enable() {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.enabled = true
	d.mu.Unlock()
}

// IsEnabled reports whether log collection is active.
func (d *Debugger) IsEnabled() bool {
	if d == nil {
		return false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.enabled
}

// Reset clears recorded logs and disables log collection.
func (d *Debugger) Reset() {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.logs = d.logs[:0]
	d.enabled = false
	d.mu.Unlock()
	d.ClearLastQueryInfo()
}

// Append records a log entry if the Debugger is enabled.
func (d *Debugger) Append(entry LogEntry) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.enabled {
		return
	}
	d.logs = append(d.logs, entry)
}

// GetLogs returns a snapshot copy of every entry recorded so far.
func (d *Debugger) GetLogs() []LogEntry {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.logs) == 0 {
		return nil
	}
	cp := make([]LogEntry, len(d.logs))
	copy(cp, d.logs)
	return cp
}

// SetLastQueryInfo records metadata for the most recently executed query.
func (d *Debugger) SetLastQueryInfo(name, operation, finalSQL string, durationMS float64) {
	d.lastQueryMu.Lock()
	d.lastQueryInfo = LastQueryInfo{QueryName: name, Operation: operation, FinalSQL: finalSQL, DurationMS: durationMS}
	d.lastQueryMu.Unlock()
}

// GetLastQueryInfo returns the most recently recorded query metadata.
func (d *Debugger) GetLastQueryInfo() LastQueryInfo {
	d.lastQueryMu.RLock()
	defer d.lastQueryMu.RUnlock()
	return d.lastQueryInfo
}

// ClearLastQueryInfo resets the last-query metadata to its zero value.
func (d *Debugger) ClearLastQueryInfo() {
	d.lastQueryMu.Lock()
	d.lastQueryInfo = LastQueryInfo{}
	d.lastQueryMu.Unlock()
}

// GetLastQueryInfoCtx returns the last query metadata recorded on ctx's Debugger.
func GetLastQueryInfoCtx(ctx context.Context) LastQueryInfo {
	d := FromContext(ctx)
	if d == nil {
		return LastQueryInfo{}
	}
	return d.GetLastQueryInfo()
}

// ToContext extracts context.Context from either a *gin.Context or a context.Context.
func ToContext(c any) context.Context {
	switch v := c.(type) {
	case *gin.Context:
		return v.Request.Context()
	case context.Context:
		return v
	default:
		return nil
	}
}

// IsEnabledCtx reports whether ctx carries an enabled Debugger.
func IsEnabledCtx(ctx context.Context) bool {
	d := FromContext(ctx)
	return d != nil && d.IsEnabled()
}
