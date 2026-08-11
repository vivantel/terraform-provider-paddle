package provider

import "time"

// sweepMinAge is how long a customer must have existed before
// sweepTestFixtureCustomers will touch it — see tooRecentToSweep's
// comment in sweep_test.go for why. Generously larger than this repo's
// own CI job durations (the acceptance job runs in ~1-2 minutes; sweep
// runs weekly or via a one-off workflow_dispatch), so no legitimate
// in-flight test object should ever be old enough to match.
const sweepMinAge = 15 * time.Minute

// tooRecentToSweep reports whether createdAt is too recent (within
// minAge of now) to safely sweep. Unparseable/empty createdAt fails
// open — returns false (safe to sweep) rather than getting permanently
// stuck skipping an object a parsing bug can't identify the age of;
// that failure mode already existed before this guard (sweep-only, no
// age awareness at all), so this guard can't make it worse, only better
// for the common case of a real, parseable timestamp.
func tooRecentToSweep(createdAt string, now time.Time, minAge time.Duration) bool {
	if createdAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return false
	}
	return now.Sub(t) < minAge
}
