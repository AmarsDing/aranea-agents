package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

type countingHealthAggregator struct {
	calls   atomic.Int32
	metrics *biz.SkillHealthMetrics
	fail    bool
}

func (c *countingHealthAggregator) GetHealthMetrics(_ context.Context, _ string, _ time.Time) (*biz.SkillHealthMetrics, error) {
	c.calls.Add(1)
	if c.fail {
		return nil, errors.New("boom")
	}
	return c.metrics, nil
}

func (c *countingHealthAggregator) GetFailureStats(_ context.Context, _ string, _ time.Time) (*biz.SkillFailureStats, error) {
	return nil, nil
}

func (c *countingHealthAggregator) GetFailureTagCounts(_ context.Context, _ string, _ time.Time) ([]biz.FailureTagCount, error) {
	return nil, nil
}

func TestSkillHealthMetricsAdapter_CachesWithinTTL(t *testing.T) {
	agg := &countingHealthAggregator{metrics: &biz.SkillHealthMetrics{SuccessRate: 0.9, AvgDurationMS: 120}}
	a := NewSkillHealthMetricsAdapter(agg)

	for i := 0; i < 3; i++ {
		rate, err := a.GetRecentSuccessRate(context.Background(), "skill-a", 30)
		if err != nil || rate != 0.9 {
			t.Fatalf("call %d = (%v, %v), want (0.9, nil)", i, rate, err)
		}
	}
	if got := agg.calls.Load(); got != 1 {
		t.Errorf("aggregator calls = %d, want 1 (cached within TTL)", got)
	}
}

func TestSkillHealthMetricsAdapter_SharesMetricsAcrossAccessors(t *testing.T) {
	agg := &countingHealthAggregator{metrics: &biz.SkillHealthMetrics{SuccessRate: 0.9, AvgDurationMS: 120}}
	a := NewSkillHealthMetricsAdapter(agg)

	if _, err := a.GetRecentSuccessRate(context.Background(), "skill-a", 30); err != nil {
		t.Fatal(err)
	}
	d, err := a.GetRecentAvgDuration(context.Background(), "skill-a", 30)
	if err != nil || d != 120 {
		t.Fatalf("avg duration = (%v, %v), want (120, nil)", d, err)
	}
	if got := agg.calls.Load(); got != 1 {
		t.Errorf("aggregator calls = %d, want 1 (two accessors share one cached lookup)", got)
	}
}

func TestSkillHealthMetricsAdapter_DifferentWindowsAreSeparateEntries(t *testing.T) {
	agg := &countingHealthAggregator{metrics: &biz.SkillHealthMetrics{SuccessRate: 0.9}}
	a := NewSkillHealthMetricsAdapter(agg)

	if _, err := a.GetRecentSuccessRate(context.Background(), "skill-a", 30); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GetRecentSuccessRate(context.Background(), "skill-a", 7); err != nil {
		t.Fatal(err)
	}
	if got := agg.calls.Load(); got != 2 {
		t.Errorf("aggregator calls = %d, want 2 (days is part of the cache key)", got)
	}
}

func TestSkillHealthMetricsAdapter_ErrorNotCached(t *testing.T) {
	agg := &countingHealthAggregator{fail: true}
	a := NewSkillHealthMetricsAdapter(agg)

	if _, err := a.GetRecentSuccessRate(context.Background(), "skill-a", 30); err == nil {
		t.Fatal("first call should fail")
	}
	agg.fail = false
	agg.metrics = &biz.SkillHealthMetrics{SuccessRate: 0.5}
	rate, err := a.GetRecentSuccessRate(context.Background(), "skill-a", 30)
	if err != nil || rate != 0.5 {
		t.Fatalf("second call = (%v, %v), want (0.5, nil)", rate, err)
	}
	if got := agg.calls.Load(); got != 2 {
		t.Errorf("aggregator calls = %d, want 2 (transient error must not be cached)", got)
	}
}

func TestSkillHealthMetricsAdapter_NilMetricsCached(t *testing.T) {
	agg := &countingHealthAggregator{metrics: nil}
	a := NewSkillHealthMetricsAdapter(agg)

	for i := 0; i < 2; i++ {
		rate, err := a.GetRecentSuccessRate(context.Background(), "skill-a", 30)
		if err != nil || rate != 0 {
			t.Fatalf("call %d = (%v, %v), want (0, nil)", i, rate, err)
		}
	}
	if got := agg.calls.Load(); got != 1 {
		t.Errorf("aggregator calls = %d, want 1 (nil metrics = no data, cache the miss)", got)
	}
}

func TestSkillHealthMetricsAdapter_ExpiresAfterTTL(t *testing.T) {
	agg := &countingHealthAggregator{metrics: &biz.SkillHealthMetrics{SuccessRate: 0.9}}
	a := NewSkillHealthMetricsAdapter(agg)
	now := time.Now()
	a.now = func() time.Time { return now }

	if _, err := a.GetRecentSuccessRate(context.Background(), "skill-a", 30); err != nil {
		t.Fatal(err)
	}
	now = now.Add(skillHealthCacheTTL + time.Second)
	if _, err := a.GetRecentSuccessRate(context.Background(), "skill-a", 30); err != nil {
		t.Fatal(err)
	}
	if got := agg.calls.Load(); got != 2 {
		t.Errorf("aggregator calls = %d, want 2 (entry expired after TTL)", got)
	}
}
