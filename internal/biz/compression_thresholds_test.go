package biz

import (
	"strings"
	"sync"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// warnCapturingLogger captures Warn calls so tests can assert the
// cross-semantic inversion warning path (logging must go through
// loggateway, never fmt/slog).
type warnCapturingLogger struct {
	loggateway.Logger
	mu  sync.Mutex
	n   int
	msg string
}

func (l *warnCapturingLogger) Warn(msg string, _ ...loggateway.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.n++
	l.msg = msg
}

func (l *warnCapturingLogger) warns() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.n
}

func TestValidateCompressionThresholds(t *testing.T) {
	noop := loggateway.NewNoop()

	t.Run("all unset (defaults) passes", func(t *testing.T) {
		if err := ValidateCompressionThresholds(&AgentRuntimeSettings{}, noop); err != nil {
			t.Fatalf("unset ratios should pass, got %v", err)
		}
	})

	t.Run("defaults effective (0.6 < 0.9) no warning", func(t *testing.T) {
		lg := &warnCapturingLogger{Logger: noop}
		if err := ValidateCompressionThresholds(&AgentRuntimeSettings{}, lg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if lg.warns() != 0 {
			t.Fatalf("unexpected warning on default thresholds")
		}
	})

	t.Run("boundary 0 and 1 pass", func(t *testing.T) {
		s := &AgentRuntimeSettings{SoftTriggerRatio: 0, HardTriggerRatio: 1, L0SummaryThreshold: 0}
		if err := ValidateCompressionThresholds(s, noop); err != nil {
			t.Fatalf("boundary ratios should pass, got %v", err)
		}
	})

	outOfRange := []struct {
		name string
		s    AgentRuntimeSettings
	}{
		{"soft negative", AgentRuntimeSettings{SoftTriggerRatio: -0.1}},
		{"soft above one", AgentRuntimeSettings{SoftTriggerRatio: 1.1}},
		{"hard negative", AgentRuntimeSettings{HardTriggerRatio: -0.01}},
		{"hard above one", AgentRuntimeSettings{HardTriggerRatio: 1.5}},
		{"l0 negative", AgentRuntimeSettings{L0SummaryThreshold: -1}},
		{"l0 above one", AgentRuntimeSettings{L0SummaryThreshold: 2}},
	}
	for _, tc := range outOfRange {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			if err := ValidateCompressionThresholds(&tc.s, noop); err == nil {
				t.Fatal("expected range validation error")
			}
		})
	}

	t.Run("rejects soft >= hard (hysteresis inversion)", func(t *testing.T) {
		for _, pair := range [][2]float64{{0.9, 0.9}, {0.95, 0.7}} {
			s := &AgentRuntimeSettings{SoftTriggerRatio: pair[0], HardTriggerRatio: pair[1]}
			err := ValidateCompressionThresholds(s, noop)
			if err == nil {
				t.Fatalf("soft=%v hard=%v should be rejected", pair[0], pair[1])
			}
			if !strings.Contains(err.Error(), "hysteresis") {
				t.Fatalf("error should mention hysteresis band, got: %v", err)
			}
		}
	})

	t.Run("soft < hard explicit passes", func(t *testing.T) {
		s := &AgentRuntimeSettings{SoftTriggerRatio: 0.5, HardTriggerRatio: 0.8}
		if err := ValidateCompressionThresholds(s, noop); err != nil {
			t.Fatalf("valid ordering rejected: %v", err)
		}
	})

	t.Run("L0 > hard warns but does not reject", func(t *testing.T) {
		lg := &warnCapturingLogger{Logger: noop}
		s := &AgentRuntimeSettings{L0SummaryThreshold: 0.95, HardTriggerRatio: 0.9}
		if err := ValidateCompressionThresholds(s, lg); err != nil {
			t.Fatalf("cross-semantic inversion must be warn-only, got %v", err)
		}
		if lg.warns() != 1 {
			t.Fatalf("expected exactly 1 warning, got %d", lg.warns())
		}
	})

	t.Run("nil settings and nil logger are safe", func(t *testing.T) {
		if err := ValidateCompressionThresholds(nil, nil); err != nil {
			t.Fatalf("nil settings should be a no-op, got %v", err)
		}
		// Also: settings set, logger nil → no panic on the warn path.
		s := &AgentRuntimeSettings{L0SummaryThreshold: 0.99, HardTriggerRatio: 0.1}
		if err := ValidateCompressionThresholds(s, nil); err != nil {
			t.Fatalf("nil logger should still validate, got %v", err)
		}
	})
}
