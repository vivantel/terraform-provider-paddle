package provider

import (
	"testing"
	"time"
)

// tooRecentToSweep's own unit tests — pure logic, no real API needed.
// This is the guard added to sweepTestFixtureCustomers to close a real
// contention risk: sweep.yaml and ci.yaml's acceptance job both run
// against the same live sandbox account with no coordination between
// them, so a sweep that happens to run *during* an in-flight acceptance
// test could otherwise archive/cancel a fixture that test hasn't
// finished using yet. See tooRecentToSweep's own comment in
// sweep_test.go for the full account.

func TestTooRecentToSweep(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	minAge := 15 * time.Minute

	cases := []struct {
		name      string
		createdAt string
		want      bool
	}{
		{"created 1 minute ago — too recent, skip", now.Add(-1 * time.Minute).Format(time.RFC3339), true},
		{"created exactly at the threshold — not recent, safe to sweep", now.Add(-15 * time.Minute).Format(time.RFC3339), false},
		{"created 1 hour ago — well past threshold, safe to sweep", now.Add(-1 * time.Hour).Format(time.RFC3339), false},
		{"unparseable timestamp — fail open (not recent), so a bug here can't wedge sweeping forever", "not-a-real-timestamp", false},
		{"empty timestamp — same fail-open reasoning", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := tooRecentToSweep(c.createdAt, now, minAge); got != c.want {
				t.Errorf("tooRecentToSweep(%q, ...) = %v, want %v", c.createdAt, got, c.want)
			}
		})
	}
}
