package monitor

import (
	"context"
	"sync"
	"time"
)

type AlertMetric interface {
	Key() string
	Description() string
	Evaluate(ctx context.Context, window time.Duration) (value float64, err error)
}

type AlertMetricRegistry struct {
	mu sync.RWMutex
	m  map[string]AlertMetric
}

func NewAlertMetricRegistry() *AlertMetricRegistry {
	return &AlertMetricRegistry{m: make(map[string]AlertMetric)}
}

func (r *AlertMetricRegistry) Register(m AlertMetric) {
	if r == nil || m == nil {
		return
	}
	r.mu.Lock()
	r.m[m.Key()] = m
	r.mu.Unlock()
}

func (r *AlertMetricRegistry) Get(key string) (AlertMetric, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.m[key]
	return m, ok
}

func (r *AlertMetricRegistry) List() []AlertMetric {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]AlertMetric, 0, len(r.m))
	for _, m := range r.m {
		out = append(out, m)
	}
	return out
}

type RunnerErrorRateMetric struct {
	repo Repo
	rb   *MetricRingBuffer
}

func NewRunnerErrorRateMetric(repo Repo, rb *MetricRingBuffer) *RunnerErrorRateMetric {
	return &RunnerErrorRateMetric{repo: repo, rb: rb}
}

func (m *RunnerErrorRateMetric) Key() string        { return "runner.error_rate" }
func (m *RunnerErrorRateMetric) Description() string { return "Runner error rate within time window" }
func (m *RunnerErrorRateMetric) Evaluate(ctx context.Context, window time.Duration) (float64, error) {
	windowMin := int(window.Minutes())
	if windowMin <= 0 {
		windowMin = 60
	}
	if m.rb != nil {
		wr := m.rb.SumLastN(windowMin)
		if wr.Total == 0 {
			return 0, nil
		}
		return float64(wr.Errors) / float64(wr.Total), nil
	}
	since := time.Now().UTC().Add(-window).Format(time.RFC3339)
	total, err := m.repo.CountMonitorEventsSince(ctx, "runner.completion", "", since)
	if err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, nil
	}
	errors, err := m.repo.CountMonitorEventsSince(ctx, "runner.completion", "error", since)
	if err != nil {
		return 0, err
	}
	return float64(errors) / float64(total), nil
}

type SkillFilesystemMissingMetric struct {
	fsHealth FilesystemHealthReader
}

func NewSkillFilesystemMissingMetric(fsHealth FilesystemHealthReader) *SkillFilesystemMissingMetric {
	return &SkillFilesystemMissingMetric{fsHealth: fsHealth}
}

func (m *SkillFilesystemMissingMetric) Key() string        { return "skill.filesystem_missing_count" }
func (m *SkillFilesystemMissingMetric) Description() string { return "Number of skills with missing filesystem" }
func (m *SkillFilesystemMissingMetric) Evaluate(ctx context.Context, _ time.Duration) (float64, error) {
	if m.fsHealth == nil {
		return 0, nil
	}
	missing, _, err := m.fsHealth.FilesystemHealthStats(ctx)
	if err != nil {
		return 0, err
	}
	return float64(missing), nil
}
