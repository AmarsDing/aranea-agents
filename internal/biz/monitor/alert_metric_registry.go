package monitor

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"
)

type AlertMetric interface {
	Key() string
	Description() string
	Evaluate(ctx context.Context, window time.Duration) (value float64, err error)
}

// AlertMetricInfo is the human-readable catalog metadata for an alert
// metric, exposed via the ListAlertMetrics API so the Alerts page can render
// a metric directory instead of raw technical keys.
type AlertMetricInfo struct {
	// Key is the technical metric key (e.g. "runner.error_rate").
	Key string
	// Name is a short English display name; the frontend maps known keys to
	// localized names and falls back to this value.
	Name string
	// Description explains what the metric measures and when it fires.
	Description string
	// Unit is "ratio" (0..1) or "count".
	Unit string
	// DefaultWindowMinutes is the recommended evaluation window.
	DefaultWindowMinutes int32
	// SuggestedThreshold is the recommended alert threshold; the rule fires
	// when the metric value is >= threshold.
	SuggestedThreshold float64
}

// AlertMetricCatalogProvider is an optional interface an AlertMetric can
// implement to expose rich catalog metadata. Metrics without it fall back
// to Key()/Description() in ListCatalog.
type AlertMetricCatalogProvider interface {
	Catalog() AlertMetricInfo
}

// AlertBreachDetailer is an optional interface an AlertMetric can implement
// to attach a human-readable breach summary and structured details to
// alert.fired events. Details reflect the most recent Evaluate call.
type AlertBreachDetailer interface {
	BreachDetails() (summary string, payload map[string]any)
}

// breachDetailsOf extracts breach details when metric implements
// AlertBreachDetailer; nil metrics and non-implementers yield empty values.
func breachDetailsOf(metric AlertMetric) (string, map[string]any) {
	if d, ok := metric.(AlertBreachDetailer); ok {
		return d.BreachDetails()
	}
	return "", nil
}

// appendBreachSummary appends the breach summary to an alert description.
func appendBreachSummary(desc, summary string) string {
	if summary == "" {
		return desc
	}
	return desc + " — " + summary
}

// ListCatalog returns catalog metadata for all registered metrics, sorted
// by key. Metrics implementing AlertMetricCatalogProvider contribute rich
// metadata; others fall back to Key/Description with Unit "count".
func (r *AlertMetricRegistry) ListCatalog() []AlertMetricInfo {
	if r == nil {
		return nil
	}
	metrics := r.List()
	out := make([]AlertMetricInfo, 0, len(metrics))
	for _, m := range metrics {
		info := AlertMetricInfo{Key: m.Key(), Description: m.Description(), Unit: "count"}
		if cp, ok := m.(AlertMetricCatalogProvider); ok {
			if ci := cp.Catalog(); ci.Key != "" {
				info = ci
			}
		}
		if info.Description == "" {
			info.Description = m.Description()
		}
		out = append(out, info)
	}
	return out
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
	sort.Slice(out, func(i, j int) bool {
		return out[i].Key() < out[j].Key()
	})
	return out
}

type RunnerErrorRateMetric struct {
	repo EventRepo
	rb   *MetricRingBuffer
}

func NewRunnerErrorRateMetric(repo EventRepo, rb *MetricRingBuffer) *RunnerErrorRateMetric {
	return &RunnerErrorRateMetric{repo: repo, rb: rb}
}

func (m *RunnerErrorRateMetric) Key() string         { return "runner.error_rate" }
func (m *RunnerErrorRateMetric) Description() string { return "Runner error rate within time window" }
func (m *RunnerErrorRateMetric) Catalog() AlertMetricInfo {
	return AlertMetricInfo{
		Key:                  m.Key(),
		Name:                 "Runner error rate",
		Description:          "Share of failed chat/team runs within the time window (0..1). Fires when too many runs end with an error.",
		Unit:                 "ratio",
		DefaultWindowMinutes: 60,
		SuggestedThreshold:   0.25,
	}
}
func (m *RunnerErrorRateMetric) Evaluate(ctx context.Context, window time.Duration) (float64, error) {
	windowMin := int(math.Ceil(window.Minutes()))
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
	total, err := m.repo.CountMonitorEventsSince(ctx, "runner.completion", "", since, "")
	if err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, nil
	}
	errors, err := m.repo.CountMonitorEventsSince(ctx, "runner.completion", "error", since, "")
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

func (m *SkillFilesystemMissingMetric) Key() string { return "skill.filesystem_missing_count" }
func (m *SkillFilesystemMissingMetric) Description() string {
	return "Number of skills with missing filesystem"
}
func (m *SkillFilesystemMissingMetric) Catalog() AlertMetricInfo {
	return AlertMetricInfo{
		Key:                  m.Key(),
		Name:                 "Skill files missing",
		Description:          "Number of installed skills whose files are missing on disk. Fires when at least one skill is broken.",
		Unit:                 "count",
		DefaultWindowMinutes: 5,
		SuggestedThreshold:   1,
	}
}
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

// DeadLetterCountReader is a narrow port for reading the v2 sequencer's
// dead-letter backlog size. Implemented by *v2.Sequencer (agent layer);
// defined here so biz/monitor does not depend on the agent package
// (dependency direction: biz is inner).
type DeadLetterCountReader interface {
	DeadLetterCount() int
}

// SequencerDeadLetterMetric exposes the v2 sequencer dead-letter ring
// occupancy to the alert engine (P0-R2a). A value >= 1 means events were
// permanently dropped from persistence after retry exhaustion.
type SequencerDeadLetterMetric struct {
	reader DeadLetterCountReader
}

func NewSequencerDeadLetterMetric(r DeadLetterCountReader) *SequencerDeadLetterMetric {
	return &SequencerDeadLetterMetric{reader: r}
}

func (m *SequencerDeadLetterMetric) Key() string { return "sequencer.dead_letter_count" }
func (m *SequencerDeadLetterMetric) Description() string {
	return "Number of v2 events in the sequencer dead-letter ring (persist failed permanently)"
}
func (m *SequencerDeadLetterMetric) Catalog() AlertMetricInfo {
	return AlertMetricInfo{
		Key:                  m.Key(),
		Name:                 "Event persist dead-letter backlog",
		Description:          "Number of activity events that failed permanent storage after retries. Any value >= 1 means durable event loss.",
		Unit:                 "count",
		DefaultWindowMinutes: 5,
		SuggestedThreshold:   1,
	}
}
func (m *SequencerDeadLetterMetric) Evaluate(_ context.Context, _ time.Duration) (float64, error) {
	if m == nil || m.reader == nil {
		return 0, nil
	}
	return float64(m.reader.DeadLetterCount()), nil
}
