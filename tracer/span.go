package tracer

import (
	"context"
	"time"
)

// Span times a code block, typically used with `defer`.
//
// Usage:
//
//	defer tracer.Span(ctx, "Calculate Discount & Taxes")()
func Span(ctx context.Context, label string) func() {
	d := FromContext(ctx)
	if d == nil {
		return func() {}
	}

	start := time.Now()
	atMS := d.sinceStartMS()

	return func() {
		d.Append(LogEntry{Type: TypeSpan, Label: label, AtMS: atMS, DurationMS: msSince(start)})
	}
}

// Measure times fn's execution as a span.
//
// Usage:
//
//	tracer.Measure(ctx, "Bcrypt Hashing", func() {
//	    hash, err = bcrypt.GenerateFromPassword(pw, bcrypt.DefaultCost)
//	})
func Measure(ctx context.Context, label string, fn func()) {
	defer Span(ctx, label)()
	fn()
}
