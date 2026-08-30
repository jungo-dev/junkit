package tracer

import (
	"testing"
	"time"
)

func TestBuildDBSummary(t *testing.T) {
	t.Cleanup(func() {
		slowQueryThresholdMS = 50.0
		nPlusOneThreshold = 3
	})

	logs := []LogEntry{
		{Type: "comment", Label: "ignored"},
	}
	for i := 0; i < 4; i++ {
		logs = append(logs, LogEntry{Type: "sql_result", Data: sqlLogData{QueryName: "GetRoleByID", DurationMS: 1.5}})
	}
	logs = append(logs, LogEntry{Type: "sql_result", Data: sqlLogData{QueryName: "ListUsers", DurationMS: 10}})

	s := buildDBSummary(logs, 100)

	if s.TotalQueries != 5 {
		t.Fatalf("TotalQueries = %d, want 5", s.TotalQueries)
	}
	wantTotalMS := 4*1.5 + 10.0
	if s.TotalMS != wantTotalMS {
		t.Fatalf("TotalMS = %v, want %v", s.TotalMS, wantTotalMS)
	}
	if s.SlowestMS != 10 {
		t.Fatalf("SlowestMS = %v, want 10", s.SlowestMS)
	}
	if len(s.NPlusOne) != 1 || s.NPlusOne[0].QueryName != "GetRoleByID" || s.NPlusOne[0].Count != 4 {
		t.Fatalf("NPlusOne = %+v, want single GetRoleByID group with count 4", s.NPlusOne)
	}
}

func TestBuildDBSummary_NoQueries(t *testing.T) {
	s := buildDBSummary([]LogEntry{{Type: "comment", Label: "hi"}}, 100)
	if s.TotalQueries != 0 || len(s.NPlusOne) != 0 {
		t.Fatalf("expected empty summary, got %+v", s)
	}
}

func TestBuildDBSummary_ThresholdIsExclusive(t *testing.T) {
	t.Cleanup(func() { nPlusOneThreshold = 3 })
	nPlusOneThreshold = 3

	logs := []LogEntry{
		{Type: "sql_result", Data: sqlLogData{QueryName: "GetX"}},
		{Type: "sql_result", Data: sqlLogData{QueryName: "GetX"}},
		{Type: "sql_result", Data: sqlLogData{QueryName: "GetX"}},
	}

	s := buildDBSummary(logs, 100)
	if len(s.NPlusOne) != 0 {
		t.Fatalf("count equal to threshold should not flag N+1, got %+v", s.NPlusOne)
	}

	logs = append(logs, LogEntry{Type: "sql_result", Data: sqlLogData{QueryName: "GetX"}})
	s = buildDBSummary(logs, 100)
	if len(s.NPlusOne) != 1 || s.NPlusOne[0].Count != 4 {
		t.Fatalf("count above threshold should flag N+1, got %+v", s.NPlusOne)
	}
}

func TestSlowQueryTier(t *testing.T) {
	t.Cleanup(func() { slowQueryThresholdMS = 50.0 })
	slowQueryThresholdMS = 50.0

	tests := []struct {
		durMS float64
		want  string
	}{
		{10, ""},
		{50, ""},
		{50.1, "warning"},
		{99, "warning"},
		{100.1, "critical"},
		{500, "critical"},
	}
	for _, tt := range tests {
		if got := slowQueryTier(tt.durMS); got != tt.want {
			t.Errorf("slowQueryTier(%v) = %q, want %q", tt.durMS, got, tt.want)
		}
	}
}

func TestSlowQueryTier_Disabled(t *testing.T) {
	t.Cleanup(func() { slowQueryThresholdMS = 50.0 })
	slowQueryThresholdMS = 0

	if got := slowQueryTier(100000); got != "" {
		t.Fatalf("slowQueryTier with threshold disabled = %q, want \"\"", got)
	}
}

func TestSetSlowQueryThreshold(t *testing.T) {
	t.Cleanup(func() { slowQueryThresholdMS = 50.0 })

	SetSlowQueryThreshold(100 * time.Millisecond)
	if slowQueryThresholdMS != 100 {
		t.Fatalf("slowQueryThresholdMS = %v, want 100", slowQueryThresholdMS)
	}
}

func TestSetNPlusOneThreshold(t *testing.T) {
	t.Cleanup(func() { nPlusOneThreshold = 3 })

	SetNPlusOneThreshold(5)
	if nPlusOneThreshold != 5 {
		t.Fatalf("nPlusOneThreshold = %d, want 5", nPlusOneThreshold)
	}

	SetNPlusOneThreshold(0) // ignored
	if nPlusOneThreshold != 5 {
		t.Fatalf("nPlusOneThreshold changed after ignored SetNPlusOneThreshold(0), got %d", nPlusOneThreshold)
	}
}
