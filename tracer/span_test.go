package tracer_test

import (
	"context"
	"testing"
	"time"

	"github.com/jungo-dev/junkit/tracer"
)

func TestSpan_NoOpWithoutEnabledDebugger(t *testing.T) {
	t.Run("no context at all", func(t *testing.T) {
		stop := tracer.Span(42, "should not panic")
		stop() // must not panic
	})

	t.Run("context with no Debugger attached", func(t *testing.T) {
		stop := tracer.Span(context.Background(), "no debugger")
		stop()
	})

	t.Run("Debugger attached but disabled", func(t *testing.T) {
		d := tracer.New()
		ctx := tracer.WithContext(context.Background(), d)

		stop := tracer.Span(ctx, "disabled")
		stop()

		if logs := d.GetLogs(); logs != nil {
			t.Fatalf("GetLogs() = %v, want nil when the Debugger is disabled", logs)
		}
	})
}

func TestSpan_RecordsOnStop(t *testing.T) {
	d := tracer.New()
	d.Enable()
	ctx := tracer.WithContext(context.Background(), d)

	stop := tracer.Span(ctx, "Calculate Discount & Taxes")
	if logs := d.GetLogs(); len(logs) != 0 {
		t.Fatalf("GetLogs() = %v before stop() is called, want no entries yet", logs)
	}

	time.Sleep(time.Millisecond)
	stop()

	logs := d.GetLogs()
	if len(logs) != 1 {
		t.Fatalf("GetLogs() returned %d entries, want 1", len(logs))
	}

	entry := logs[0]
	if entry.Type != "span" {
		t.Fatalf("entry.Type = %q, want %q", entry.Type, "span")
	}

	data, ok := entry.Data.(tracer.SpanData)
	if !ok {
		t.Fatalf("entry.Data = %T, want tracer.SpanData", entry.Data)
	}
	if data.Label != "Calculate Discount & Taxes" {
		t.Fatalf("data.Label = %q, want %q", data.Label, "Calculate Discount & Taxes")
	}
	if data.Category != tracer.CategoryLogic {
		t.Fatalf("data.Category = %q, want default %q", data.Category, tracer.CategoryLogic)
	}
	if data.DurationMS <= 0 {
		t.Fatalf("data.DurationMS = %v, want > 0", data.DurationMS)
	}
	if data.Depth != 0 {
		t.Fatalf("data.Depth = %d, want 0 for a top-level span", data.Depth)
	}
}

func TestSpan_ExplicitCategory(t *testing.T) {
	d := tracer.New()
	d.Enable()
	ctx := tracer.WithContext(context.Background(), d)

	tracer.Span(ctx, "Call Payment Gateway", tracer.CategoryExternal)()

	data := logAt(t, d, 0)
	if data.Category != tracer.CategoryExternal {
		t.Fatalf("data.Category = %q, want %q", data.Category, tracer.CategoryExternal)
	}
}

func TestSpan_NestedDepth(t *testing.T) {
	d := tracer.New()
	d.Enable()
	ctx := tracer.WithContext(context.Background(), d)

	stopOuter := tracer.Span(ctx, "outer")
	stopInner := tracer.Span(ctx, "inner")
	stopInner()
	stopOuter()

	logs := d.GetLogs()
	if len(logs) != 2 {
		t.Fatalf("GetLogs() returned %d entries, want 2", len(logs))
	}

	inner := logs[0].Data.(tracer.SpanData)
	outer := logs[1].Data.(tracer.SpanData)

	if inner.Depth != 1 {
		t.Fatalf("inner span Depth = %d, want 1", inner.Depth)
	}
	if outer.Depth != 0 {
		t.Fatalf("outer span Depth = %d, want 0", outer.Depth)
	}
	if inner.StartMS < outer.StartMS {
		t.Fatalf("inner.StartMS (%v) should be >= outer.StartMS (%v)", inner.StartMS, outer.StartMS)
	}
}

func TestMeasure_RunsFnAndRecordsDuration(t *testing.T) {
	d := tracer.New()
	d.Enable()
	ctx := tracer.WithContext(context.Background(), d)

	called := false
	tracer.Measure(ctx, "Bcrypt Hashing", func() {
		called = true
		time.Sleep(time.Millisecond)
	}, tracer.CategoryLogic)

	if !called {
		t.Fatal("Measure did not run fn")
	}

	data := logAt(t, d, 0)
	if data.Label != "Bcrypt Hashing" {
		t.Fatalf("data.Label = %q, want %q", data.Label, "Bcrypt Hashing")
	}
	if data.DurationMS <= 0 {
		t.Fatalf("data.DurationMS = %v, want > 0", data.DurationMS)
	}
}

func TestMeasure_NoOpWhenDisabled(t *testing.T) {
	d := tracer.New()
	ctx := tracer.WithContext(context.Background(), d)

	called := false
	tracer.Measure(ctx, "skipped", func() { called = true })

	if !called {
		t.Fatal("Measure must still run fn even when the Debugger is disabled")
	}
	if logs := d.GetLogs(); logs != nil {
		t.Fatalf("GetLogs() = %v, want nil when the Debugger is disabled", logs)
	}
}

// logAt asserts that d has an entry at index i whose Data is a tracer.SpanData, returning it.
func logAt(t *testing.T, d *tracer.Debugger, i int) tracer.SpanData {
	t.Helper()
	logs := d.GetLogs()
	if i >= len(logs) {
		t.Fatalf("GetLogs() has %d entries, want at least %d", len(logs), i+1)
	}
	data, ok := logs[i].Data.(tracer.SpanData)
	if !ok {
		t.Fatalf("logs[%d].Data = %T, want tracer.SpanData", i, logs[i].Data)
	}
	return data
}
