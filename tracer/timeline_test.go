package tracer

import "testing"

func TestBuildTimeline(t *testing.T) {
	logs := []LogEntry{
		{Type: "comment", Label: "ignored"},
		{Type: "span", Label: "logic span", Data: SpanData{Label: "Business Logic", Category: CategoryLogic, StartMS: 1, DurationMS: 5, Depth: 0}},
		{Type: "sql_result", Label: "query", Data: sqlLogData{QueryName: "GetUser", StartMS: 6, DurationMS: 2, Depth: 0}},
		{Type: "warning", Label: "ignored too"},
	}

	got := buildTimeline(logs)
	if len(got) != 2 {
		t.Fatalf("buildTimeline() returned %d entries, want 2 (span + sql_result only)", len(got))
	}

	if got[0].Label != "Business Logic" || got[0].Category != CategoryLogic {
		t.Fatalf("got[0] = %+v, want the span entry with CategoryLogic", got[0])
	}

	if got[1].Label != "GetUser" || got[1].Category != CategoryDB {
		t.Fatalf("got[1] = %+v, want the sql_result entry mapped to CategoryDB", got[1])
	}
}

func TestSelfMS(t *testing.T) {
	tests := []struct {
		name    string
		entries []timelineEntry
		index   int
		want    float64
	}{
		{
			name:    "no children: self time equals own duration",
			entries: []timelineEntry{{StartMS: 0, DurationMS: 10, Depth: 0}},
			index:   0,
			want:    10,
		},
		{
			name: "one fully-contained direct child is subtracted",
			entries: []timelineEntry{
				{StartMS: 0, DurationMS: 10, Depth: 0},
				{StartMS: 2, DurationMS: 4, Depth: 1},
			},
			index: 0,
			want:  6,
		},
		{
			name: "a sibling at the same depth is not subtracted",
			entries: []timelineEntry{
				{StartMS: 0, DurationMS: 10, Depth: 0},
				{StartMS: 2, DurationMS: 4, Depth: 0},
			},
			index: 0,
			want:  10,
		},
		{
			name: "a grandchild (depth+2) is not subtracted directly",
			entries: []timelineEntry{
				{StartMS: 0, DurationMS: 10, Depth: 0},
				{StartMS: 2, DurationMS: 6, Depth: 1},
				{StartMS: 3, DurationMS: 2, Depth: 2},
			},
			index: 0,
			want:  4, // 10 - (child at depth 1) = 4; the grandchild is already inside the depth-1 child's own self time
		},
		{
			name: "multiple direct children are all subtracted",
			entries: []timelineEntry{
				{StartMS: 0, DurationMS: 10, Depth: 0},
				{StartMS: 1, DurationMS: 2, Depth: 1},
				{StartMS: 5, DurationMS: 3, Depth: 1},
			},
			index: 0,
			want:  5,
		},
		{
			name: "overlapping concurrent children summing past the parent's duration clamp to zero",
			entries: []timelineEntry{
				{StartMS: 0, DurationMS: 5, Depth: 0},
				{StartMS: 0, DurationMS: 5, Depth: 1},
				{StartMS: 0, DurationMS: 5, Depth: 1},
			},
			index: 0,
			want:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := selfMS(tt.entries, tt.index); got != tt.want {
				t.Fatalf("selfMS() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTopLevelMS(t *testing.T) {
	entries := []timelineEntry{
		{DurationMS: 10, Depth: 0},
		{DurationMS: 4, Depth: 1},
		{DurationMS: 3, Depth: 0},
	}
	if got := topLevelMS(entries); got != 13 {
		t.Fatalf("topLevelMS() = %v, want 13", got)
	}
}

func TestBreakdownByCategory(t *testing.T) {
	// 100ms request: 40ms DB query and 80ms logic span wrapping a 20ms external call.
	entries := []timelineEntry{
		{Label: "GetUser", Category: CategoryDB, StartMS: 0, DurationMS: 40, Depth: 0},
		{Label: "Process Order", Category: CategoryLogic, StartMS: 40, DurationMS: 80, Depth: 0},
		{Label: "Call Payment Gateway", Category: CategoryExternal, StartMS: 60, DurationMS: 20, Depth: 1},
	}
	totalMS := 120.0

	got := breakdownByCategory(entries, totalMS)

	byCategory := make(map[SpanCategory]CategoryBreakdown, len(got))
	for _, b := range got {
		byCategory[b.Category] = b
	}

	if b := byCategory[CategoryDB]; b.SelfMS != 40 {
		t.Fatalf("DB self time = %v, want 40", b.SelfMS)
	}
	if b := byCategory[CategoryExternal]; b.SelfMS != 20 {
		t.Fatalf("external self time = %v, want 20", b.SelfMS)
	}
	if b := byCategory[CategoryLogic]; b.SelfMS != 60 {
		t.Fatalf("logic self time = %v, want 60 (80 total - 20 nested external call)", b.SelfMS)
	}
	if _, ok := byCategory[categoryOther]; ok {
		t.Fatalf("breakdown = %+v, want no %q bucket since top-level time accounts for the full request", got, categoryOther)
	}

	var pctSum float64
	for _, b := range got {
		pctSum += b.Percent
	}
	if pctSum < 99.99 || pctSum > 100.01 {
		t.Fatalf("percentages sum to %v, want ~100", pctSum)
	}
}

func TestBreakdownByCategory_UntrackedTimeGoesToOther(t *testing.T) {
	entries := []timelineEntry{
		{Label: "GetUser", Category: CategoryDB, StartMS: 0, DurationMS: 30, Depth: 0},
	}
	totalMS := 100.0 // 70ms of the request wasn't inside any recorded span/query

	got := breakdownByCategory(entries, totalMS)

	var other *CategoryBreakdown
	for i := range got {
		if got[i].Category == categoryOther {
			other = &got[i]
		}
	}
	if other == nil {
		t.Fatalf("breakdown = %+v, want a %q bucket for the untracked 70ms", got, categoryOther)
	}
	if other.SelfMS != 70 {
		t.Fatalf("other.SelfMS = %v, want 70", other.SelfMS)
	}
	if other.Percent != 70 {
		t.Fatalf("other.Percent = %v, want 70", other.Percent)
	}
}

func TestPercentOf(t *testing.T) {
	if got := percentOf(25, 100); got != 25 {
		t.Fatalf("percentOf(25, 100) = %v, want 25", got)
	}
	if got := percentOf(10, 0); got != 0 {
		t.Fatalf("percentOf(10, 0) = %v, want 0 (avoid dividing by zero)", got)
	}
}

func TestBuildSegments(t *testing.T) {
	t.Run("no entries produce one full-width gap", func(t *testing.T) {
		got := buildSegments(nil, 50)
		want := []segment{{Category: categoryOther, StartMS: 0, DurationMS: 50}}
		if !equalSegments(got, want) {
			t.Fatalf("buildSegments(nil, 50) = %+v, want %+v", got, want)
		}
	})

	t.Run("two sequential top-level entries with gaps before, between, and after", func(t *testing.T) {
		entries := []timelineEntry{
			{Label: "Bcrypt Hashing", Category: CategoryLogic, StartMS: 10, DurationMS: 20, Depth: 0},
			{Label: "CreateUser", Category: CategoryDB, StartMS: 40, DurationMS: 5, Depth: 0},
		}
		got := buildSegments(entries, 50)
		want := []segment{
			{Category: categoryOther, StartMS: 0, DurationMS: 10},
			{Label: "Bcrypt Hashing", Category: CategoryLogic, StartMS: 10, DurationMS: 20},
			{Category: categoryOther, StartMS: 30, DurationMS: 10},
			{Label: "CreateUser", Category: CategoryDB, StartMS: 40, DurationMS: 5},
			{Category: categoryOther, StartMS: 45, DurationMS: 5},
		}
		if !equalSegments(got, want) {
			t.Fatalf("buildSegments() = %+v, want %+v", got, want)
		}
	})

	t.Run("a nested child is unpacked into its own segment", func(t *testing.T) {
		// Process Order (0-80, logic) wraps Call Payment Gateway (20-50, external).
		entries := []timelineEntry{
			{Label: "Process Order", Category: CategoryLogic, StartMS: 0, DurationMS: 80, Depth: 0},
			{Label: "Call Payment Gateway", Category: CategoryExternal, StartMS: 20, DurationMS: 30, Depth: 1},
		}
		got := buildSegments(entries, 80)
		want := []segment{
			{Label: "Process Order", Category: CategoryLogic, StartMS: 0, DurationMS: 20},
			{Label: "Call Payment Gateway", Category: CategoryExternal, StartMS: 20, DurationMS: 30},
			{Label: "Process Order", Category: CategoryLogic, StartMS: 50, DurationMS: 30},
		}
		if !equalSegments(got, want) {
			t.Fatalf("buildSegments() = %+v, want %+v", got, want)
		}
	})

	t.Run("a zero-duration leaf span still gets its own segment", func(t *testing.T) {
		// A 0ms span with no children should still be rendered.
		entries := []timelineEntry{
			{Label: "Bcrypt Hashing", Category: CategoryLogic, StartMS: 0, DurationMS: 20, Depth: 0},
			{Label: "Check error", Category: CategoryLogic, StartMS: 20, DurationMS: 0, Depth: 0},
		}
		got := buildSegments(entries, 20)
		want := []segment{
			{Label: "Bcrypt Hashing", Category: CategoryLogic, StartMS: 0, DurationMS: 20},
			{Label: "Check error", Category: CategoryLogic, StartMS: 20, DurationMS: 0},
		}
		if !equalSegments(got, want) {
			t.Fatalf("buildSegments() = %+v, want %+v (the zero-duration span must still be present)", got, want)
		}
	})

	t.Run("segments always sum to totalMS", func(t *testing.T) {
		entries := []timelineEntry{
			{Label: "GetUser", Category: CategoryDB, StartMS: 5, DurationMS: 7, Depth: 0},
			{Label: "Process Order", Category: CategoryLogic, StartMS: 20, DurationMS: 50, Depth: 0},
			{Label: "Call Payment Gateway", Category: CategoryExternal, StartMS: 30, DurationMS: 10, Depth: 1},
		}
		totalMS := 100.0
		got := buildSegments(entries, totalMS)

		var sum float64
		for _, s := range got {
			sum += s.DurationMS
		}
		if sum != totalMS {
			t.Fatalf("segment durations sum to %v, want totalMS %v (got %+v)", sum, totalMS, got)
		}
	})
}

// equalSegments compares two segment slices for equality (helper since segment has no comparable float precision concerns in these fixtures).
func equalSegments(a, b []segment) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
