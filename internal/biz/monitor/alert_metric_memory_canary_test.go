package monitor_test

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
)

type stubCanaryReader struct {
	failures int64
}

func (s *stubCanaryReader) ConsecutiveFailures() int64 { return s.failures }

func TestMemoryCanaryMetric_Evaluate(t *testing.T) {
	m := monitor.NewMemoryCanaryMetric(&stubCanaryReader{failures: 3})
	if m.Key() != "memory.canary_consecutive_failures" {
		t.Fatalf("Key() = %q", m.Key())
	}
	v, err := m.Evaluate(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v != 3 {
		t.Fatalf("Evaluate() = %v, want 3", v)
	}
}

func TestMemoryCanaryMetric_NilReader(t *testing.T) {
	m := monitor.NewMemoryCanaryMetric(nil)
	v, err := m.Evaluate(context.Background(), time.Minute)
	if err != nil || v != 0 {
		t.Fatalf("Evaluate() = %v, %v, want 0, nil", v, err)
	}
}

func TestMemoryCanaryMetric_Catalog(t *testing.T) {
	m := monitor.NewMemoryCanaryMetric(&stubCanaryReader{})
	c := m.Catalog()
	if c.Key != m.Key() || c.Unit != "count" || c.SuggestedThreshold < 1 {
		t.Fatalf("Catalog() = %+v", c)
	}
}
