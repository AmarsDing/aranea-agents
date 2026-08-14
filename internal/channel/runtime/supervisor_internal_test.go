package runtime

import (
	"testing"
	"time"
)

// Backoff must only reset after a provably stable run; fast-fail loops (bad
// credentials, instant dial errors) must keep exponential growth (CH-R1).
func TestShouldResetRunBackoff(t *testing.T) {
	cases := []struct {
		name   string
		runDur time.Duration
		want   bool
	}{
		{"instant failure keeps backoff", 10 * time.Millisecond, false},
		{"short run keeps backoff", 30 * time.Second, false},
		{"stable run resets", runtimeStableRunThreshold, true},
		{"long run resets", 10 * time.Minute, true},
	}
	for _, tc := range cases {
		if got := shouldResetRunBackoff(tc.runDur); got != tc.want {
			t.Errorf("%s: shouldResetRunBackoff(%v) = %v, want %v", tc.name, tc.runDur, got, tc.want)
		}
	}
}
