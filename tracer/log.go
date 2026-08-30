package tracer

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	"go.uber.org/zap"
)

// globalZap is the fallback logger C/V/W/E use when no enabled Debugger is
// attached to the given context, set once via BindLogger at startup.
var globalZap *zap.Logger

// BindLogger sets the fallback *zap.Logger for tracer logging when no Debugger is enabled.
//
// Usage:
//
//	tracer.BindLogger(zapLogger)
func BindLogger(l *zap.Logger) {
	globalZap = l
}

// C (Comment) records a plain text note.
//
// Usage:
//
//	tracer.C(ctx, "starting user synchronization")
func C(c any, message string) {
	ctx := ToContext(c)
	if ctx == nil {
		return
	}

	if d := FromContext(ctx); d.IsEnabled() {
		d.Append(LogEntry{Type: "comment", Label: message})
		return
	}
	if globalZap != nil {
		globalZap.Info("🐞 [TRACE] " + message)
	}
}

// V (Variable) records one or more values under a shared label.
//
// Usage:
//
//	tracer.V(ctx, "user profile", user)
func V(c any, label string, values ...any) {
	ctx := ToContext(c)
	if ctx == nil {
		return
	}

	d := FromContext(ctx)
	enabled := d.IsEnabled()

	for _, val := range values {
		if enabled {
			d.Append(LogEntry{Type: "variable", Label: label, Data: val})
		} else if globalZap != nil {
			globalZap.Info("🐞 [TRACE] "+label, zap.Any("value", val))
		}
	}
}

// W (Warning) records a non-fatal warning message without a stack trace.
//
// Usage:
//
//	tracer.W(ctx, "user not found", err)
func W(c any, label string, err any) {
	ctx := ToContext(c)
	if ctx == nil {
		return
	}

	labelStr := label
	if labelStr == "" {
		labelStr = "WARNING"
	}

	msg := fmt.Sprintf("%v", err)
	file, line, fn := FindErrorOrigin()

	data := map[string]any{
		"error":     msg,
		"label":     labelStr,
		"location":  file,
		"line":      line,
		"function":  fn,
		"timestamp": time.Now().Format("2006-01-02 15:04:05.000 -0700"),
	}
	if info := GetLastQueryInfoCtx(ctx); info.FinalSQL != "" {
		data["sql"] = info.FinalSQL
		data["query_name"] = info.QueryName
		data["duration_ms"] = info.DurationMS
	}

	if d := FromContext(ctx); d.IsEnabled() {
		d.Append(LogEntry{Type: "warning", Label: labelStr, Data: data})
	}

	if globalZap != nil {
		globalZap.Warn(labelStr,
			zap.String("error", msg),
			zap.String("file", file),
			zap.Int("line", line),
			zap.String("function", fn),
		)
	}
}

// E (Error) records an error with source location and full stack trace.
//
// Usage:
//
//	if err != nil {
//	    tracer.E(ctx, "database query failed", err)
//	}
func E(c any, label string, err any) {
	ctx := ToContext(c)
	if ctx == nil {
		return
	}

	labelStr := label
	if labelStr == "" {
		labelStr = "ERROR"
	}

	msg := fmt.Sprintf("%v", err)
	file, line, fn := FindErrorOrigin()
	stack := debug.Stack()

	data := map[string]any{
		"error":     msg,
		"label":     labelStr,
		"location":  file,
		"line":      line,
		"function":  fn,
		"stack":     string(stack),
		"timestamp": time.Now().Format("2006-01-02 15:04:05.000 -0700"),
	}
	if info := GetLastQueryInfoCtx(ctx); info.FinalSQL != "" {
		data["sql"] = info.FinalSQL
		data["query_name"] = info.QueryName
		data["duration_ms"] = info.DurationMS
	}

	if d := FromContext(ctx); d.IsEnabled() {
		d.Append(LogEntry{Type: "error", Label: labelStr, Data: data})
	}

	if globalZap != nil {
		globalZap.Error(labelStr,
			zap.String("error", msg),
			zap.String("file", file),
			zap.Int("line", line),
			zap.String("function", fn),
			zap.ByteString("stack", stack),
		)
	}
}

// Stop records values and panics with BreakpointSignal when the Debugger is enabled.
//
// Usage:
//
//	tracer.Stop(ctx, "user before save", user)
func Stop(c any, label string, values ...any) {
	V(c, label, values...)

	ctx := ToContext(c)
	if ctx != nil && IsEnabledCtx(ctx) {
		panic(BreakpointSignal{})
	}
}

// SQL records the most recently executed query metadata and optional result.
func SQL(ctx context.Context, result any) {
	d := FromContext(ctx)
	if !d.IsEnabled() {
		return
	}

	info := d.GetLastQueryInfo()
	if info.QueryName == "" {
		return
	}

	label := fmt.Sprintf("%s | %.2fms", info.QueryName, info.DurationMS)
	if info.FinalSQL != "" {
		label += " | " + info.FinalSQL
	}
	d.Append(LogEntry{Type: "sql_result", Label: label, Data: result})
}
