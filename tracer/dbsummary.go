package tracer

import (
	"sort"
	"time"
)

// slowQueryThresholdMS is the duration in ms above which a query is flagged as slow (0 disables).
var slowQueryThresholdMS = 50.0

// nPlusOneThreshold is the query repetition threshold for N+1 detection.
var nPlusOneThreshold = 3

// SetSlowQueryThreshold sets the threshold for highlighting slow queries (default 50ms).
//
// Usage:
//
//	tracer.SetSlowQueryThreshold(100 * time.Millisecond)
func SetSlowQueryThreshold(d time.Duration) {
	slowQueryThresholdMS = float64(d.Microseconds()) / 1000.0
}

// SetNPlusOneThreshold sets the query repetition threshold for N+1 detection (default 3).
//
// Usage:
//
//	tracer.SetNPlusOneThreshold(5)
func SetNPlusOneThreshold(count int) {
	if count > 0 {
		nPlusOneThreshold = count
	}
}

// slowQueryTier returns "", "warning", or "critical" based on query duration.
func slowQueryTier(durMS float64) string {
	if slowQueryThresholdMS <= 0 {
		return ""
	}
	switch {
	case durMS > slowQueryThresholdMS*2:
		return "critical"
	case durMS > slowQueryThresholdMS:
		return "warning"
	default:
		return ""
	}
}

// dbQueryStat aggregates executions of a query within a request.
type dbQueryStat struct {
	QueryName string
	Operation string
	Count     int
	TotalMS   float64
}

// dbSummary aggregates traced SQL queries and N+1 query stats for a request.
type dbSummary struct {
	TotalQueries int
	TotalMS      float64
	Percent      float64
	SlowestMS    float64
	NPlusOne     []dbQueryStat // queries repeated more than nPlusOneThreshold times
}

// buildDBSummary aggregates a request's sql_result log entries into a dbSummary.
func buildDBSummary(logs []LogEntry, totalMS float64) dbSummary {
	var s dbSummary

	groups := make(map[string]*dbQueryStat)
	var order []string

	for _, entry := range logs {
		if entry.Type != "sql_result" {
			continue
		}
		q, ok := entry.Data.(sqlLogData)
		if !ok {
			continue
		}

		s.TotalQueries++
		s.TotalMS += q.DurationMS
		if q.DurationMS > s.SlowestMS {
			s.SlowestMS = q.DurationMS
		}

		g, seen := groups[q.QueryName]
		if !seen {
			g = &dbQueryStat{QueryName: q.QueryName, Operation: q.Operation}
			groups[q.QueryName] = g
			order = append(order, q.QueryName)
		}
		g.Count++
		g.TotalMS += q.DurationMS
	}

	s.Percent = percentOf(s.TotalMS, totalMS)

	for _, name := range order {
		if g := groups[name]; g.Count > nPlusOneThreshold {
			s.NPlusOne = append(s.NPlusOne, *g)
		}
	}
	sort.Slice(s.NPlusOne, func(i, j int) bool { return s.NPlusOne[i].Count > s.NPlusOne[j].Count })

	return s
}
