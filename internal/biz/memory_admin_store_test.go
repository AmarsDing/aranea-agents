package biz

import "testing"

func TestDefaultFactBruteForceThreshold(t *testing.T) {
	if DefaultFactBruteForceThreshold != 5000 {
		t.Errorf("DefaultFactBruteForceThreshold = %d, want 5000", DefaultFactBruteForceThreshold)
	}
}
