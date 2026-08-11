package client

import "testing"

// reachedLimit is List*Filtered's early-exit check — lookup data sources
// only need to know whether 0, exactly 1, or more than 1 record matches,
// so there's no reason to paginate an unbounded filter to exhaustion
// (per_page=200, loop until !HasMore) just to discard everything past the
// second match. Found via code review, docs/plans/paddle-provider-v4.md.

func TestReachedLimit(t *testing.T) {
	cases := []struct {
		name  string
		count int
		limit int
		want  bool
	}{
		{"limit disabled (0) never stops", 1000, 0, false},
		{"below limit", 1, 2, false},
		{"at limit", 2, 2, true},
		{"above limit", 3, 2, true},
		{"zero count, limit set", 0, 2, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := reachedLimit(c.count, c.limit); got != c.want {
				t.Errorf("reachedLimit(%d, %d) = %v, want %v", c.count, c.limit, got, c.want)
			}
		})
	}
}
