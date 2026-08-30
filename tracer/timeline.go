package tracer

import "sort"

// timelineEntry represents a single span or query in the execution timeline.
type timelineEntry struct {
	Label      string
	Category   SpanCategory
	StartMS    float64
	DurationMS float64
	Depth      int
}

// endMS returns e's end offset, in milliseconds from request start.
func (e timelineEntry) endMS() float64 { return e.StartMS + e.DurationMS }

// buildTimeline extracts span and traced-SQL-query entries from logs, in the
// order they were recorded.
func buildTimeline(logs []LogEntry) []timelineEntry {
	var out []timelineEntry
	for _, entry := range logs {
		switch entry.Type {
		case "span":
			if sd, ok := entry.Data.(SpanData); ok {
				out = append(out, timelineEntry{
					Label:      sd.Label,
					Category:   sd.Category,
					StartMS:    sd.StartMS,
					DurationMS: sd.DurationMS,
					Depth:      sd.Depth,
				})
			}
		case "sql_result":
			if q, ok := entry.Data.(sqlLogData); ok {
				out = append(out, timelineEntry{
					Label:      q.QueryName,
					Category:   CategoryDB,
					StartMS:    q.StartMS,
					DurationMS: q.DurationMS,
					Depth:      q.Depth,
				})
			}
		}
	}
	return out
}

// selfMS returns the exclusive duration of entries[i], excluding its child spans.
func selfMS(entries []timelineEntry, i int) float64 {
	e := entries[i]
	self := e.DurationMS

	for j, c := range entries {
		if j == i || c.Depth != e.Depth+1 {
			continue
		}
		if c.StartMS >= e.StartMS && c.endMS() <= e.endMS() {
			self -= c.DurationMS
		}
	}

	if self < 0 {
		return 0
	}
	return self
}

// topLevelMS returns the total duration of top-level (depth-0) spans.
func topLevelMS(entries []timelineEntry) float64 {
	var sum float64
	for _, e := range entries {
		if e.Depth == 0 {
			sum += e.DurationMS
		}
	}
	return sum
}

// categoryOther labels untraced request time (framework overhead, untraced code, etc.).
const categoryOther SpanCategory = "other"

// CategoryBreakdown reports the exclusive time and percentage of totalMS
// spent in one span/query category.
type CategoryBreakdown struct {
	Category SpanCategory
	SelfMS   float64
	Percent  float64
}

// breakdownByCategory aggregates execution time by category as percentages of totalMS.
func breakdownByCategory(entries []timelineEntry, totalMS float64) []CategoryBreakdown {
	sums := make(map[SpanCategory]float64, 4)
	order := make([]SpanCategory, 0, 4)

	for i, e := range entries {
		if _, seen := sums[e.Category]; !seen {
			order = append(order, e.Category)
		}
		sums[e.Category] += selfMS(entries, i)
	}

	out := make([]CategoryBreakdown, 0, len(order)+1)
	for _, cat := range order {
		out = append(out, CategoryBreakdown{Category: cat, SelfMS: sums[cat], Percent: percentOf(sums[cat], totalMS)})
	}

	if untracked := totalMS - topLevelMS(entries); untracked > 0.005 {
		out = append(out, CategoryBreakdown{Category: categoryOther, SelfMS: untracked, Percent: percentOf(untracked, totalMS)})
	}

	return out
}

// percentOf returns ms as a percentage of totalMS, or 0 if totalMS isn't positive.
func percentOf(ms, totalMS float64) float64 {
	if totalMS <= 0 {
		return 0
	}
	return ms / totalMS * 100
}

// segment represents a contiguous time slice in the timeline view.
type segment struct {
	Label      string
	Category   SpanCategory
	StartMS    float64
	DurationMS float64
}

// buildSegments flattens entries into a single timeline bar of non-overlapping segments.
func buildSegments(entries []timelineEntry, totalMS float64) []segment {
	var roots []timelineEntry
	for _, e := range entries {
		if e.Depth == 0 {
			roots = append(roots, e)
		}
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i].StartMS < roots[j].StartMS })

	var segs []segment
	cursor := 0.0
	for _, r := range roots {
		if r.StartMS > cursor {
			segs = append(segs, segment{Category: categoryOther, StartMS: cursor, DurationMS: r.StartMS - cursor})
		}
		segs = append(segs, expandSegment(r, entries)...)
		cursor = r.endMS()
	}
	if totalMS > cursor {
		segs = append(segs, segment{Category: categoryOther, StartMS: cursor, DurationMS: totalMS - cursor})
	}
	return segs
}

// expandSegment recursively splits a timeline entry into segments for itself and its children.
func expandSegment(e timelineEntry, all []timelineEntry) []segment {
	var children []timelineEntry
	for _, c := range all {
		if c.Depth == e.Depth+1 && c.StartMS >= e.StartMS && c.endMS() <= e.endMS() {
			children = append(children, c)
		}
	}
	sort.Slice(children, func(i, j int) bool { return children[i].StartMS < children[j].StartMS })

	var out []segment
	cursor := e.StartMS
	for _, c := range children {
		if c.StartMS > cursor {
			out = append(out, segment{Label: e.Label, Category: e.Category, StartMS: cursor, DurationMS: c.StartMS - cursor})
		}
		out = append(out, expandSegment(c, all)...)
		cursor = c.endMS()
	}
	// Leaf entries always get a segment even at zero duration.
	// Parent entries get a trailing segment only if a gap remains after children.
	if len(children) == 0 || e.endMS() > cursor {
		out = append(out, segment{Label: e.Label, Category: e.Category, StartMS: cursor, DurationMS: e.endMS() - cursor})
	}
	return out
}
