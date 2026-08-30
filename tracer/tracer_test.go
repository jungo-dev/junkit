package tracer_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/jungo-dev/junkit/tracer"
)

func TestDebugger_EnableIsEnabled(t *testing.T) {
	d := tracer.New()

	if d.IsEnabled() {
		t.Fatal("a new Debugger should start disabled")
	}

	d.Enable()
	if !d.IsEnabled() {
		t.Fatal("IsEnabled() should be true after Enable()")
	}
}

func TestDebugger_NilReceiverIsSafe(t *testing.T) {
	var d *tracer.Debugger

	if d.IsEnabled() {
		t.Fatal("a nil Debugger should report disabled")
	}
	d.Enable() // must not panic
	d.Reset()  // must not panic
}

func TestDebugger_Append(t *testing.T) {
	t.Run("entries are dropped while disabled", func(t *testing.T) {
		d := tracer.New()
		d.Append(tracer.LogEntry{Type: "comment", Label: "should be dropped"})

		if logs := d.GetLogs(); logs != nil {
			t.Fatalf("GetLogs() = %v, want nil while disabled", logs)
		}
	})

	t.Run("entries are recorded once enabled", func(t *testing.T) {
		d := tracer.New()
		d.Enable()
		d.Append(tracer.LogEntry{Type: "comment", Label: "first"})
		d.Append(tracer.LogEntry{Type: "warning", Label: "second"})

		logs := d.GetLogs()
		if len(logs) != 2 {
			t.Fatalf("GetLogs() returned %d entries, want 2", len(logs))
		}
		if logs[0].Label != "first" || logs[1].Label != "second" {
			t.Fatalf("GetLogs() = %+v, want entries in append order", logs)
		}
	})
}

func TestDebugger_GetLogsReturnsASnapshot(t *testing.T) {
	d := tracer.New()
	d.Enable()
	d.Append(tracer.LogEntry{Type: "comment", Label: "original"})

	logs := d.GetLogs()
	logs[0].Label = "mutated by caller"

	fresh := d.GetLogs()
	if fresh[0].Label != "original" {
		t.Fatalf("GetLogs() leaked internal state: got %q after an external mutation, want %q",
			fresh[0].Label, "original")
	}
}

func TestDebugger_Reset(t *testing.T) {
	d := tracer.New()
	d.Enable()
	d.Append(tracer.LogEntry{Type: "comment", Label: "will be cleared"})
	d.SetLastQueryInfo("GetUser", "ONE", "SELECT 1", 1.5)

	d.Reset()

	if d.IsEnabled() {
		t.Fatal("Reset() should disable the debugger")
	}
	if logs := d.GetLogs(); logs != nil {
		t.Fatalf("GetLogs() = %v after Reset(), want nil", logs)
	}
	if info := d.GetLastQueryInfo(); info != (tracer.LastQueryInfo{}) {
		t.Fatalf("GetLastQueryInfo() = %+v after Reset(), want the zero value", info)
	}
}

func TestDebugger_LastQueryInfo(t *testing.T) {
	d := tracer.New()

	if info := d.GetLastQueryInfo(); info != (tracer.LastQueryInfo{}) {
		t.Fatalf("GetLastQueryInfo() on a fresh Debugger = %+v, want the zero value", info)
	}

	d.SetLastQueryInfo("GetUserByUUID", "ONE", "SELECT * FROM users WHERE uuid = $1", 2.3)
	want := tracer.LastQueryInfo{QueryName: "GetUserByUUID", Operation: "ONE", FinalSQL: "SELECT * FROM users WHERE uuid = $1", DurationMS: 2.3}
	if got := d.GetLastQueryInfo(); got != want {
		t.Fatalf("GetLastQueryInfo() = %+v, want %+v", got, want)
	}

	d.ClearLastQueryInfo()
	if info := d.GetLastQueryInfo(); info != (tracer.LastQueryInfo{}) {
		t.Fatalf("GetLastQueryInfo() = %+v after ClearLastQueryInfo(), want the zero value", info)
	}
}

func TestWithContextAndFromContext(t *testing.T) {
	if got := tracer.FromContext(context.Background()); got != nil {
		t.Fatalf("FromContext() on a plain context = %v, want nil", got)
	}

	d := tracer.New()
	ctx := tracer.WithContext(context.Background(), d)

	if got := tracer.FromContext(ctx); got != d {
		t.Fatal("FromContext() did not return the Debugger passed to WithContext()")
	}
}

func TestIsEnabledCtx(t *testing.T) {
	if tracer.IsEnabledCtx(context.Background()) {
		t.Fatal("IsEnabledCtx() on a plain context should be false")
	}

	d := tracer.New()
	ctx := tracer.WithContext(context.Background(), d)
	if tracer.IsEnabledCtx(ctx) {
		t.Fatal("IsEnabledCtx() should be false before Enable()")
	}

	d.Enable()
	if !tracer.IsEnabledCtx(ctx) {
		t.Fatal("IsEnabledCtx() should be true after Enable()")
	}
}

func TestGetLastQueryInfoCtx(t *testing.T) {
	if got := tracer.GetLastQueryInfoCtx(context.Background()); got != (tracer.LastQueryInfo{}) {
		t.Fatalf("GetLastQueryInfoCtx() on a plain context = %+v, want the zero value", got)
	}

	d := tracer.New()
	d.SetLastQueryInfo("GetUser", "ONE", "SELECT 1", 0.5)
	ctx := tracer.WithContext(context.Background(), d)

	want := tracer.LastQueryInfo{QueryName: "GetUser", Operation: "ONE", FinalSQL: "SELECT 1", DurationMS: 0.5}
	if got := tracer.GetLastQueryInfoCtx(ctx); got != want {
		t.Fatalf("GetLastQueryInfoCtx() = %+v, want %+v", got, want)
	}
}

func TestToContext(t *testing.T) {
	t.Run("a context.Context is returned as-is", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), struct{}{}, "marker")
		if got := tracer.ToContext(ctx); got != ctx {
			t.Fatal("ToContext() did not return the same context.Context")
		}
	})

	t.Run("a *gin.Context yields its request context", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		req := httptest.NewRequest("GET", "/", nil)
		reqCtx := context.WithValue(req.Context(), struct{}{}, "from-request")
		req = req.WithContext(reqCtx)

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = req

		got := tracer.ToContext(c)
		if got.Value(struct{}{}) != "from-request" {
			t.Fatal("ToContext() did not return the *gin.Context's underlying request context")
		}
	})

	t.Run("an unrelated type returns nil", func(t *testing.T) {
		if got := tracer.ToContext(42); got != nil {
			t.Fatalf("ToContext(42) = %v, want nil", got)
		}
	})
}
