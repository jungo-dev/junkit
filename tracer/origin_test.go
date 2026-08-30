package tracer_test

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/jungo-dev/junkit/tracer"
)

// oneWrapperDeep helper calls FindErrorOrigin through one intermediate wrapper level.
func oneWrapperDeep() (string, int, string) {
	return tracer.FindErrorOrigin()
}

func TestFindErrorOrigin_ReportsTheCallerOfItsWrapper(t *testing.T) {
	file, line, fn := oneWrapperDeep()

	if !strings.HasSuffix(file, "origin_test.go") {
		t.Fatalf("file = %q, want it to point at this test file", file)
	}
	if line <= 0 {
		t.Fatalf("line = %d, want a positive line number", line)
	}
	if !strings.Contains(fn, "TestFindErrorOrigin_ReportsTheCallerOfItsWrapper") {
		t.Fatalf("function = %q, want it to mention this test function", fn)
	}
}

// TestFindErrorOrigin_ThroughW tests that tracer.W correctly attributes the line number of its caller.
func TestFindErrorOrigin_ThroughW(t *testing.T) {
	d := tracer.New()
	d.Enable()
	ctx := tracer.WithContext(context.Background(), d)

	_, _, callerLine, _ := runtime.Caller(0)
	tracer.W(ctx, "test warning", "boom") // <-- the next line after callerLine; must match wantLine below
	wantLine := callerLine + 1

	logs := d.GetLogs()
	if len(logs) != 1 {
		t.Fatalf("got %d log entries, want 1", len(logs))
	}

	data, ok := logs[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("log entry Data = %#v, want map[string]any", logs[0].Data)
	}

	if loc, _ := data["location"].(string); !strings.HasSuffix(loc, "origin_test.go") {
		t.Fatalf(`data["location"] = %v, want it to end in "origin_test.go"`, data["location"])
	}
	if line, _ := data["line"].(int); line != wantLine {
		t.Fatalf(`data["line"] = %v, want %d (the tracer.W call site, not one frame further up)`, data["line"], wantLine)
	}
	if fn, _ := data["function"].(string); !strings.Contains(fn, "TestFindErrorOrigin_ThroughW") {
		t.Fatalf(`data["function"] = %v, want it to mention this test function`, data["function"])
	}
}
