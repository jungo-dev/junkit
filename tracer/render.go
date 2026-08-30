package tracer

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
)

// Constant HTML/CSS/JS assets for the debug dashboard, extracted to avoid
// re-allocating them on every render.
const (
	dashboardStyle = `
    <style>
        :root {
            --bg: #0a0a0b; --card: #16161a; --border: #2a2a2e;
            --text: #fafafa; --text-muted: #71717a;
            --primary: #3b82f6; --success: #22c55e; --warning: #f59e0b;
            --error: #ef4444; --code-bg: #000000;
            --max-width: 100%;
        }
        * { box-sizing: border-box; }
        body {
            font-family: 'Inter', system-ui, -apple-system, sans-serif;
            background: var(--bg); color: var(--text);
            margin: 0; line-height: 1.4; font-size: 13px;
            display: flex; flex-direction: column; min-height: 100vh;
        }

        .header-wrapper {
            position: sticky; top: 0; z-index: 1000;
            background: rgba(10, 10, 11, 0.95); backdrop-filter: blur(12px);
            border-bottom: 1px solid var(--border);
            padding: 10px 0;
        }
        .controls {
            max-width: var(--max-width); margin: 0 auto;
            padding: 0 16px;
            display: flex; justify-content: space-between; align-items: center;
        }
        .header-title { font-size: 16px; font-weight: 700; display: flex; align-items: center; gap: 8px; }

        .search-input {
            background: var(--card); border: 1px solid var(--border); color: white;
            padding: 6px 12px; border-radius: 6px; width: 200px; outline: none; font-size: 12px;
            transition: border-color 0.2s;
        }
        .search-input:focus { border-color: var(--primary); }

        .filter-group { display: flex; gap: 6px; }
        .btn-filter {
            background: var(--card); border: 1px solid var(--border); color: var(--text-muted);
            padding: 4px 10px; border-radius: 6px; cursor: pointer; font-size: 11px; transition: all 0.2s;
            font-weight: 500;
        }
        .btn-filter:hover, .btn-filter.active {
            background: var(--primary); color: white; border-color: var(--primary);
        }

        .main-content {
            padding: 16px; max-width: var(--max-width); margin: 0 auto;
            flex: 1; width: 100%;
        }

        .log-entry {
            background: var(--card); border: 1px solid var(--border);
            border-radius: 8px; margin-bottom: 10px; position: relative;
            overflow: hidden;
        }
        .log-entry.hidden { display: none; }
        .log-header {
            padding: 6px 12px; border-bottom: 1px solid var(--border);
            display: flex; justify-content: space-between; align-items: center;
            background: rgba(255,255,255,0.02);
            font-size: 12px;
        }
        .log-body { padding: 10px 12px; position: relative; }

        .comment-item {
            margin: 12px 0; color: var(--warning); font-style: italic;
            display: flex; align-items: center; gap: 8px; font-weight: 500;
            font-size: 13px; border-left: 3px solid var(--warning); padding-left: 10px;
            background: rgba(245, 158, 11, 0.05); padding: 6px 10px; border-radius: 0 6px 6px 0;
        }

        .badge {
            font-size: 10px; font-weight: 700; text-transform: uppercase;
            padding: 2px 6px; border-radius: 4px; margin-right: 8px; display: inline-block;
        }
        .badge-sql_result { background: rgba(59, 130, 246, 0.15); color: #60a5fa; }
        .badge-variable { background: rgba(34, 197, 94, 0.15); color: #4ade80; }
        .badge-warning { background: rgba(245, 158, 11, 0.15); color: #fbbf24; }
        .badge-error { background: rgba(239, 68, 68, 0.15); color: #f87171; }

        .sql-error-block {
            margin-top: 10px;
            background: rgba(239, 68, 68, 0.05);
            border: 1px solid rgba(239, 68, 68, 0.2);
            border-radius: 6px;
            position: relative; overflow: hidden;
        }
        .sql-error-header {
            padding: 8px 10px;
            font-size: 10px; font-weight: 600; text-transform: uppercase;
            color: var(--error);
            background: rgba(239, 68, 68, 0.1);
            border-bottom: 1px solid rgba(239, 68, 68, 0.2);
            letter-spacing: 0.5px;
        }
        .sql-error-block pre {
            background: transparent; border: none; padding: 10px; margin: 0;
            color: #fca5a5;
            font-size: 12px;
        }

        .sql-warning-block {
            margin-top: 10px;
            background: rgba(245, 158, 11, 0.05);
            border: 1px solid rgba(245, 158, 11, 0.2);
            border-radius: 6px;
            position: relative; overflow: hidden;
        }
        .sql-warning-header {
            padding: 8px 10px;
            font-size: 10px; font-weight: 600; text-transform: uppercase;
            color: var(--warning);
            background: rgba(245, 158, 11, 0.1);
            border-bottom: 1px solid rgba(245, 158, 11, 0.2);
            letter-spacing: 0.5px;
        }
        .sql-warning-block pre {
            background: transparent; border: none; padding: 10px; margin: 0;
            color: #fcd34d;
            font-size: 12px;
        }

        pre {
            font-family: 'JetBrains Mono', monospace; font-size: 12px;
            background: #0d0d0d; padding: 10px; border-radius: 6px;
            overflow-x: auto; color: #e2e8f0; border: 1px solid var(--border); margin: 0;
            line-height: 1.5;
        }

        .copy-btn {
            position: absolute; top: 11px; right: 12px; z-index: 10;
            background: var(--card); color: var(--text-muted); border: 1px solid var(--border);
            padding: 3px 6px; border-radius: 4px; font-size: 10px; cursor: pointer;
            transition: all 0.2s;
            display: flex; align-items: center; justify-content: center;
        }
        .copy-btn:hover { background: var(--border); color: var(--text); }
        .copy-btn.copied { border-color: var(--success); color: var(--success); }

        .json-key { color: #f97583; }
        .json-string { color: #9ece6a; }
        .json-number { color: #ff9d00; }
        .json-boolean { color: #79c0ff; }
        .json-null { color: #f87171; }

        footer {
            text-align: center; padding: 4px 12px 12px;
            color: var(--text-muted); font-size: 11px;
            margin-top: 0;
        }

        .stack-file { color: var(--primary); font-weight: 600; }
        .stack-line { color: var(--text-muted); font-size: 11px; }

        .timeline-section {
            background: var(--card); border: 1px solid var(--border);
            border-radius: 8px; padding: 12px 14px; margin-bottom: 16px;
        }
        .timeline-title-row {
            display: flex; justify-content: space-between; align-items: baseline;
            margin-bottom: 10px;
        }
        .timeline-title { font-weight: 700; font-size: 13px; }
        .timeline-total { color: var(--text-muted); font-size: 11px; font-variant-numeric: tabular-nums; }
        .timeline-legend {
            display: flex; gap: 14px; flex-wrap: wrap;
            margin-bottom: 12px; font-size: 11px; color: var(--text-muted);
        }
        .legend-dot { display: inline-block; width: 8px; height: 8px; border-radius: 2px; margin-right: 5px; }
        .legend-logic, .timeline-seg.logic { background: #4ade80; }
        .legend-db, .timeline-seg.db { background: #60a5fa; }
        .legend-external, .timeline-seg.external { background: #a78bfa; }
        .legend-other, .timeline-seg.other { background: #f59e0b; }

        .timeline-bar-track {
            display: flex; height: 20px; border-radius: 4px; overflow: hidden;
            background: rgba(255,255,255,0.03);
        }
        .timeline-seg {
            position: relative; height: 100%; min-width: 2px;
            flex-shrink: 0; cursor: default;
            border-right: 1px solid rgba(0,0,0,0.25);
        }
        /* Untracked gaps get no enforced minimum: a negligible gap should
           shrink away rather than appear as a visible sliver out of
           proportion to its actual (near-zero) share of the request. */
        .timeline-seg.other { min-width: 0; }
        .timeline-seg:last-child { border-right: none; }
        .timeline-seg:hover { filter: brightness(1.2); }
        .timeline-seg[data-tooltip]:hover::after {
            content: attr(data-tooltip);
            position: absolute; bottom: 130%; left: 50%; transform: translateX(-50%);
            background: #000; color: #fff; padding: 4px 8px; border-radius: 4px;
            font-size: 11px; white-space: nowrap; z-index: 20;
            box-shadow: 0 2px 8px rgba(0,0,0,0.4); pointer-events: none;
        }
        .timeline-seg[data-tooltip]:hover::before {
            content: ''; position: absolute; bottom: 120%; left: 50%; transform: translateX(-50%);
            border: 5px solid transparent; border-top-color: #000;
            z-index: 20; pointer-events: none;
        }

        .timeline-seg-list { margin-top: 10px; display: flex; flex-direction: column; gap: 4px; }
        .timeline-seg-list-item {
            display: flex; align-items: center; font-size: 11px; color: var(--text-muted);
        }
        .timeline-seg-list-label { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
        .timeline-seg-list-duration { margin-left: auto; padding-left: 12px; font-variant-numeric: tabular-nums; }

        .db-summary-card {
            background: var(--card); border: 1px solid var(--border);
            border-radius: 8px; padding: 12px 14px; margin-bottom: 16px;
        }
        .db-summary-stats { display: flex; gap: 28px; flex-wrap: wrap; }
        .db-stat { display: flex; flex-direction: column; gap: 2px; }
        .db-stat-value { font-size: 18px; font-weight: 700; font-variant-numeric: tabular-nums; }
        .db-stat-value.slow { color: var(--warning); }
        .db-stat-label { font-size: 11px; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.3px; }

        .n-plus-one-badge {
            margin-top: 10px; padding: 8px 10px; border-radius: 6px;
            background: rgba(245, 158, 11, 0.1); border: 1px solid rgba(245, 158, 11, 0.3);
            color: #fbbf24; font-size: 12px; font-weight: 600;
        }
        .n-plus-one-badge + .n-plus-one-badge { margin-top: 6px; }

        .log-entry.slow-query-warning { border-color: var(--warning); box-shadow: 0 0 0 1px rgba(245,158,11,0.25); }
        .log-entry.slow-query-critical { border-color: var(--error); box-shadow: 0 0 0 1px rgba(239,68,68,0.25); }
        .badge-slow-warning { background: rgba(245, 158, 11, 0.15); color: #fbbf24; }
        .badge-slow-critical { background: rgba(239, 68, 68, 0.15); color: #f87171; }
    </style>`

	dashboardScript = `
    <script>
       function highlightAll() {
            document.querySelectorAll('pre:not(.no-highlight)').forEach(el => {
                try {
                    const obj = JSON.parse(el.innerText);
                    el.innerHTML = syntaxHighlight(obj);
                } catch(e) {}
            });
        }

        function syntaxHighlight(json) {
            if (typeof json != 'string') json = JSON.stringify(json, undefined, 2);
            json = json.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
            return json.replace(/("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?)/g, function (match) {
                var cls = 'json-number';
                if (/^"/.test(match)) {
                    if (/:$/.test(match)) cls = 'json-key';
                    else cls = 'json-string';
                } else if (/true|false/.test(match)) cls = 'json-boolean';
                else if (/null/.test(match)) cls = 'json-null';
                return '<span class="' + cls + '">' + match + '</span>';
            });
        }

        function filterLogs(type, btn) {
            document.querySelectorAll('.btn-filter').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            document.querySelectorAll('.log-entry, .comment-item').forEach(entry => {
                const isMatch = type === 'all' || entry.getAttribute('data-type') === type;
                entry.style.display = isMatch ? '' : 'none';
            });
        }

        function searchLogs(query) {
            query = query.toLowerCase();
            document.querySelectorAll('.log-entry, .comment-item').forEach(entry => {
                entry.style.display = entry.innerText.toLowerCase().includes(query) ? '' : 'none';
            });
        }

        async function copyToClipboard(btn, textID) {
            const el = document.getElementById(textID);
            const text = el.innerText;
            const originalText = btn.innerText;

            const success = () => {
                btn.innerText = '✓ Copied!';
                btn.classList.add('copied');
                setTimeout(() => {
                    btn.innerText = originalText;
                    btn.classList.remove('copied');
                }, 1500);
            };

            const fail = () => {
                btn.innerText = '❌ Ctrl+C';
                btn.style.color = 'var(--error)';

                const range = document.createRange();
                range.selectNodeContents(el);
                const sel = window.getSelection();
                sel.removeAllRanges();
                sel.addRange(range);

                setTimeout(() => {
                    btn.innerText = originalText;
                    btn.style.color = '';
                }, 2000);
            };

            try {
                if (navigator.clipboard && navigator.clipboard.writeText) {
                    await navigator.clipboard.writeText(text);
                    success();
                } else {
                    throw new Error('Clipboard API unavailable');
                }
            } catch (err) {
                try {
                    const textArea = document.createElement("textarea");
                    textArea.value = text;
                    textArea.style.position = "fixed";
                    textArea.style.left = "-9999px";
                    document.body.appendChild(textArea);

                    textArea.focus();
                    textArea.select();

                    const successful = document.execCommand('copy');
                    document.body.removeChild(textArea);

                    if (successful) success();
                    else fail();
                } catch (fallbackErr) {
                    fail();
                }
            }
        }
       window.onload = highlightAll;
    </script>`

	dashboardHeader = `
        <div class="header-wrapper">
            <div class="controls">
                <div class="header-title">🚀 Tracer Debug Dashboard</div>
                <div style="display:flex; gap:12px; align-items:center;">
                    <input type="text" class="search-input" placeholder="Search logs..." onkeyup="searchLogs(this.value)">
                    <div class="filter-group">
                        <button class="btn-filter active" onclick="filterLogs('all', this)">All</button>
                        <button class="btn-filter" onclick="filterLogs('comment', this)">Comments</button>
                        <button class="btn-filter" onclick="filterLogs('warning', this)">Warnings</button>
                        <button class="btn-filter" onclick="filterLogs('error', this)">Errors</button>
                        <button class="btn-filter" onclick="filterLogs('sql_result', this)">SQL</button>
                        <button class="btn-filter" onclick="filterLogs('variable', this)">Variables</button>
                    </div>
                </div>
            </div>
        </div>`
)

// RenderHTML renders the debug logs from ctx as an interactive HTML dashboard.
//
// Usage:
//
//	c.String(http.StatusOK, tracer.RenderHTML(ctx))
func RenderHTML(ctx context.Context) string {
	d := FromContext(ctx)
	if d == nil {
		return `<div style="font-family:sans-serif;padding:40px;text-align:center;color:#666;">
                <h2>🚀 Tracer Debug Dashboard</h2>
                <p>Context not initialized. Make sure middleware.TracerDebug is active.</p>
             </div>`
	}

	logs := d.GetLogs()
	totalMS := d.elapsedMS()

	var sb strings.Builder
	sb.Grow(32 * 1024)

	sb.WriteString(`<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8">`)
	sb.WriteString(`<title>Tracer Debug Dashboard</title>`)
	sb.WriteString(dashboardStyle)
	sb.WriteString(`</head><body>`)
	sb.WriteString(dashboardHeader)
	sb.WriteString(`<div class="main-content">`)

	renderDBSummary(&sb, buildDBSummary(logs, totalMS))
	renderTimeline(&sb, totalMS, logs)

	for i, entry := range logs {
		renderLogEntry(&sb, i, entry)
	}

	sb.WriteString(`</div>`)
	sb.WriteString(`<footer>junkit/tracer debug dashboard</footer>`)
	sb.WriteString(dashboardScript)
	sb.WriteString(`</body></html>`)

	return sb.String()
}

// renderDBSummary renders the DB summary card and N+1 warning badges.
func renderDBSummary(sb *strings.Builder, s dbSummary) {
	if s.TotalQueries == 0 {
		return
	}

	slowClass := ""
	if slowQueryTier(s.SlowestMS) != "" {
		slowClass = " slow"
	}

	sb.WriteString(`<div class="db-summary-card"><div class="db-summary-stats">`)
	fmt.Fprintf(sb, `<div class="db-stat"><span class="db-stat-value">%d</span><span class="db-stat-label">Total Queries</span></div>`, s.TotalQueries)
	fmt.Fprintf(sb, `<div class="db-stat"><span class="db-stat-value%s">%.2fms</span><span class="db-stat-label">Total DB Time</span></div>`, slowClass, s.TotalMS)
	fmt.Fprintf(sb, `<div class="db-stat"><span class="db-stat-value">%.1f%%</span><span class="db-stat-label">of Request</span></div>`, s.Percent)
	sb.WriteString(`</div>`)

	for _, g := range s.NPlusOne {
		fmt.Fprintf(sb, `<div class="n-plus-one-badge">⚠️ N+1 Query Detected: Query <strong>%s</strong> executed %d times (total %.2fms)</div>`,
			html.EscapeString(g.QueryName), g.Count, g.TotalMS)
	}

	sb.WriteString(`</div>`)
}

// renderTimeline renders the execution timeline bar and segment list.
func renderTimeline(sb *strings.Builder, totalMS float64, logs []LogEntry) {
	entries := buildTimeline(logs)
	if len(entries) == 0 {
		return
	}

	fmt.Fprintf(sb, `<div class="timeline-section"><div class="timeline-title-row"><div class="timeline-title">⏱ Timeline</div><div class="timeline-total">Total: %.2fms</div></div>`, totalMS)

	sb.WriteString(`<div class="timeline-legend">`)
	for _, b := range breakdownByCategory(entries, totalMS) {
		fmt.Fprintf(sb, `<span><span class="legend-dot legend-%s"></span>%s %.1f%%</span>`,
			html.EscapeString(string(b.Category)), html.EscapeString(strings.ToUpper(string(b.Category))), b.Percent)
	}
	sb.WriteString(`</div>`)

	segs := buildSegments(entries, totalMS)

	sb.WriteString(`<div class="timeline-bar-track">`)
	for _, seg := range segs {
		width := 0.0
		if totalMS > 0 {
			width = seg.DurationMS / totalMS * 100
		}
		label := seg.Label
		if label == "" {
			label = "Untracked"
		}
		tooltip := fmt.Sprintf("%s — %.2fms", label, seg.DurationMS)
		fmt.Fprintf(sb, `<div class="timeline-seg %s" style="width:%.3f%%" title="%s" data-tooltip="%s"></div>`,
			html.EscapeString(string(seg.Category)), width, html.EscapeString(tooltip), html.EscapeString(tooltip))
	}
	sb.WriteString(`</div>`)

	sb.WriteString(`<div class="timeline-seg-list">`)
	for _, seg := range segs {
		if seg.Label == "" {
			continue // skip untracked gaps; their total is already in the OTHER legend badge
		}
		fmt.Fprintf(sb, `<div class="timeline-seg-list-item"><span class="legend-dot legend-%s"></span><span class="timeline-seg-list-label">%s</span><span class="timeline-seg-list-duration">%.2fms</span></div>`,
			html.EscapeString(string(seg.Category)), html.EscapeString(seg.Label), seg.DurationMS)
	}
	sb.WriteString(`</div>`)

	sb.WriteString(`</div>`)
}

// renderLogEntry appends one LogEntry's HTML to sb.
func renderLogEntry(sb *strings.Builder, index int, entry LogEntry) {
	if entry.Type == "comment" {
		fmt.Fprintf(sb, `<div class="comment-item" data-type="comment"><span>💬</span> %s</div>`, html.EscapeString(entry.Label))
		return
	}

	id := fmt.Sprintf("code-%d", index)

	entryClass := ""
	slowBadge := ""
	if entry.Type == "sql_result" {
		if q, ok := entry.Data.(sqlLogData); ok {
			if tier := slowQueryTier(q.DurationMS); tier != "" {
				entryClass = " slow-query-" + tier
				slowBadge = fmt.Sprintf(`<span class="badge badge-slow-%s">SLOW</span>`, tier)
			}
		}
	}

	fmt.Fprintf(sb, `<div class="log-entry%s" data-type="%s">`, entryClass, entry.Type)
	fmt.Fprintf(sb, `
            <div class="log-header">
                <div><span class="badge badge-%s">%s</span>%s<span style="font-weight:600;">%s</span></div>
                <div style="color:var(--text-muted); font-size:11px;">#%d</div>
            </div>`, entry.Type, entry.Type, slowBadge, html.EscapeString(entry.Label), index+1)

	sb.WriteString(`<div class="log-body">`)
	if entry.Type == "error" || entry.Type == "warning" {
		renderErrorOrWarning(sb, index, entry)
	} else {
		fmt.Fprintf(sb, `<button class="copy-btn" onclick="copyToClipboard(this, '%s')">Copy</button>`, id)
		fmt.Fprintf(sb, `<pre id="%s">%s</pre>`, id, html.EscapeString(PrettyPrint(entry.Data)))
	}
	sb.WriteString(`</div></div>`)
}

// renderErrorOrWarning renders an error or warning entry with stack trace if available.
func renderErrorOrWarning(sb *strings.Builder, index int, entry LogEntry) {
	data, _ := entry.Data.(map[string]any)
	msg, _ := data["error"].(string)
	loc, _ := data["location"].(string)
	line, _ := data["line"].(int)
	stack, _ := data["stack"].(string)
	sql, _ := data["sql"].(string)

	color := "var(--error)"
	if entry.Type == "warning" {
		color = "var(--warning)"
	}

	fmt.Fprintf(sb, `<div style="color:%s; font-weight:bold; margin-bottom:8px;">%s</div>`, color, html.EscapeString(msg))
	fmt.Fprintf(sb, `<div style="font-size:12px; color:var(--text-muted); margin-bottom:12px;">📍 %s:%d</div>`, html.EscapeString(loc), line)

	if sql != "" {
		sqlID := fmt.Sprintf("sql-err-%d", index)

		blockClass, headerClass, codeColor := "sql-error-block", "sql-error-header", "#fca5a5"
		if entry.Type == "warning" {
			blockClass, headerClass, codeColor = "sql-warning-block", "sql-warning-header", "#fcd34d"
		}

		fmt.Fprintf(sb, `<div class="%s">`, blockClass)
		fmt.Fprintf(sb, `<div class="%s">Related SQL Query</div>`, headerClass)
		fmt.Fprintf(sb, `<button class="copy-btn" style="top:6px; right:12px;" onclick="copyToClipboard(this, '%s')">Copy</button>`, sqlID)
		fmt.Fprintf(sb, `<pre id="%s" class="no-highlight" style="background:transparent; border:none; color:%s; padding: 12px; white-space: pre-wrap; word-break: break-word;">%s</pre>`, sqlID, codeColor, html.EscapeString(sql))
		sb.WriteString(`</div><br/>`)
	}

	if stack != "" {
		sb.WriteString(`<details><summary style="cursor:pointer; font-size:12px; color:var(--primary); margin-bottom:5px;">Stack Trace</summary>`)
		sb.WriteString(`<pre class="no-highlight" style="margin-top:10px; background:#0a0a0a; color:#f87171;">` + formatStack(stack) + `</pre></details>`)
	}
}

// RenderText renders the debug logs from ctx as ANSI-colored plain text.
//
// Usage:
//
//	fmt.Println(tracer.RenderText(ctx))
func RenderText(ctx context.Context) string {
	d := FromContext(ctx)
	if d == nil {
		return "tracer: not initialized\n"
	}
	logs := d.GetLogs()
	totalMS := d.elapsedMS()

	var sb strings.Builder
	sb.WriteString("\n" + colorize("38;5;238", strings.Repeat("━", 80)) + "\n")
	sb.WriteString(colorize("1;37", "  🚀 TRACER DEBUG DASHBOARD") + "\n")
	sb.WriteString(colorize("38;5;238", strings.Repeat("━", 80)) + "\n\n")

	renderDBSummaryText(&sb, buildDBSummary(logs, totalMS))
	renderTimelineText(&sb, totalMS, logs)

	for _, entry := range logs {
		if entry.Type == "comment" {
			sb.WriteString("  " + colorize("38;5;214;3", "💬 "+entry.Label) + "\n\n")
			continue
		}

		var icon, headColor string
		switch entry.Type {
		case "error":
			icon, headColor = "✘", "31;1"
		case "warning":
			icon, headColor = "⚠", "33;1"
		case "sql_result":
			icon, headColor = "🖴", "34;1"
			if q, ok := entry.Data.(sqlLogData); ok {
				switch slowQueryTier(q.DurationMS) {
				case "critical":
					icon, headColor = "🐢", "31;1"
				case "warning":
					icon, headColor = "🐢", "33;1"
				}
			}
		default:
			icon, headColor = "•", "32;1"
		}

		sb.WriteString(fmt.Sprintf("  %s %s\n", colorize(headColor, icon+" "+strings.ToUpper(entry.Type)), colorize("1;37", entry.Label)))
		for _, line := range strings.Split(PrettyPrint(entry.Data), "\n") {
			sb.WriteString("    " + colorize("38;5;244", line) + "\n")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// renderDBSummaryText renders a text-mode summary of database queries and N+1 warnings.
func renderDBSummaryText(sb *strings.Builder, s dbSummary) {
	if s.TotalQueries == 0 {
		return
	}

	sb.WriteString("  " + colorize("1;37", fmt.Sprintf("🖴 DB SUMMARY — Total Queries: %d | Total DB Time: %.2fms (%.1f%% of request)", s.TotalQueries, s.TotalMS, s.Percent)) + "\n")
	for _, g := range s.NPlusOne {
		sb.WriteString("    " + colorize("33;1", fmt.Sprintf("⚠ N+1 Query Detected: Query %s executed %d times (total %.2fms)", g.QueryName, g.Count, g.TotalMS)) + "\n")
	}
	sb.WriteString("\n")
}

// timelineBarWidth is the character width of each ASCII bar in RenderText's timeline.
const timelineBarWidth = 40

// ansiColorForCategory maps a span category to an ANSI color code.
func ansiColorForCategory(cat SpanCategory) string {
	switch cat {
	case CategoryLogic:
		return "38;5;114" // green
	case CategoryDB:
		return "38;5;75" // blue
	case CategoryExternal:
		return "38;5;141" // purple
	default: // categoryOther
		return "38;5;208" // orange
	}
}

// renderTimelineText renders a text-mode timeline bar and segment list.
func renderTimelineText(sb *strings.Builder, totalMS float64, logs []LogEntry) {
	entries := buildTimeline(logs)
	if len(entries) == 0 {
		return
	}

	sb.WriteString("  " + colorize("1;37", fmt.Sprintf("⏱ TIMELINE — Total: %.2fms", totalMS)) + "\n")
	for _, b := range breakdownByCategory(entries, totalMS) {
		sb.WriteString(fmt.Sprintf("    %-10s %5.1f%%\n", strings.ToUpper(string(b.Category)), b.Percent))
	}
	sb.WriteString("\n    ")

	segs := buildSegments(entries, totalMS)

	for _, seg := range segs {
		filled := 0
		if totalMS > 0 {
			filled = int(seg.DurationMS/totalMS*timelineBarWidth + 0.5)
		}
		if filled == 0 {
			continue
		}
		sb.WriteString(colorize(ansiColorForCategory(seg.Category), strings.Repeat("█", filled)))
	}
	sb.WriteString("\n\n")

	for _, seg := range segs {
		if seg.Label == "" {
			continue // skip untracked gaps; their total is already in the OTHER breakdown line
		}
		sb.WriteString(fmt.Sprintf("    %s %s (%.2fms)\n", colorize(ansiColorForCategory(seg.Category), "■"), seg.Label, seg.DurationMS))
	}
	sb.WriteString("\n")
}

// formatStack wraps each line of a stack trace in a span for HTML display,
// highlighting lines that reference a Go source file.
func formatStack(stack string) string {
	lines := strings.Split(stack, "\n")
	formatted := make([]string, len(lines))
	for i, line := range lines {
		if strings.Contains(line, ".go") {
			formatted[i] = `<span class="stack-file">` + html.EscapeString(line) + `</span>`
		} else {
			formatted[i] = `<span class="stack-line">` + html.EscapeString(line) + `</span>`
		}
	}
	return strings.Join(formatted, "\n")
}

// colorize wraps text in an ANSI escape code, for RenderText's terminal output.
func colorize(code, text string) string {
	return "\033[" + code + "m" + text + "\033[0m"
}

// PrettyPrint formats v as indented JSON after value normalization.
//
// Usage:
//
//	s := tracer.PrettyPrint(myStruct)
func PrettyPrint(v any) string {
	normalized := NormalizeValue(v)
	b, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return "JSON Error: " + err.Error()
	}
	return string(b)
}
