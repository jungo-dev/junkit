package tracer

import (
	"fmt"
	"time"
)

// SpanCategory classifies a span for the dashboard's timeline breakdown.
type SpanCategory string

const (
	CategoryLogic    SpanCategory = "logic"    // default: application/business logic
	CategoryExternal SpanCategory = "external" // calls to external APIs/services
	CategoryDB       SpanCategory = "db"       // reserved: also used to label traced SQL queries
)

// SpanData is the dashboard entry payload for a custom timed span.
type SpanData struct {
	Label      string       `json:"label"`
	Category   SpanCategory `json:"category"`
	StartMS    float64      `json:"start_ms"`
	DurationMS float64      `json:"duration_ms"`
	Depth      int          `json:"depth"`
}

// Span tracks execution time of a code block, typically used with `defer`.
//
// Usage:
//
//	defer tracer.Span(ctx, "Calculate Discount & Taxes")()
func Span(c any, label string, category ...SpanCategory) func() {
	ctx := ToContext(c)
	if ctx == nil {
		return func() {}
	}

	d := FromContext(ctx)
	if !d.IsEnabled() {
		return func() {}
	}

	cat := CategoryLogic
	if len(category) > 0 {
		cat = category[0]
	}

	start := time.Now()
	startMS := d.elapsedMS()
	depth := d.pushSpanDepth()

	return func() {
		d.popSpanDepth()
		recordSpan(d, label, cat, startMS, time.Since(start), depth)
	}
}

// Measure times fn's execution as a span.
//
// Usage:
//
//	tracer.Measure(ctx, "Bcrypt Hashing", func() {
//	    hash, err = bcrypt.GenerateFromPassword(pw, bcrypt.DefaultCost)
//	})
func Measure(c any, label string, fn func(), category ...SpanCategory) {
	stop := Span(c, label, category...)
	defer stop()
	fn()
}

// recordSpan appends a "span" log entry to d.
func recordSpan(d *Debugger, label string, cat SpanCategory, startMS float64, dur time.Duration, depth int) {
	durMS := float64(dur.Microseconds()) / 1000.0

	d.Append(LogEntry{
		Type:  "span",
		Label: fmt.Sprintf("%s | %.2fms", label, durMS),
		Data: SpanData{
			Label:      label,
			Category:   cat,
			StartMS:    startMS,
			DurationMS: durMS,
			Depth:      depth,
		},
	})
}
