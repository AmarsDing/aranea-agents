package monitor_test

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz/monitor"
)

type stubAlertMetric struct {
	keyVal  string
	descVal string
	evalFn  func(ctx context.Context, window time.Duration) (float64, error)
}

func (m *stubAlertMetric) Key() string         { return m.keyVal }
func (m *stubAlertMetric) Description() string { return m.descVal }
func (m *stubAlertMetric) Evaluate(ctx context.Context, window time.Duration) (float64, error) {
	if m.evalFn != nil {
		return m.evalFn(ctx, window)
	}
	return 0, nil
}

func TestNewAlertMetricRegistry(t *testing.T) {
	r := monitor.NewAlertMetricRegistry()
	if r == nil {
		t.Fatal("NewAlertMetricRegistry() = nil, want non-nil")
	}
}

func TestAlertMetricRegistry_RegisterAndGet(t *testing.T) {
	r := monitor.NewAlertMetricRegistry()
	m := &stubAlertMetric{keyVal: "test.metric", descVal: "Test metric"}
	r.Register(m)

	got, ok := r.Get("test.metric")
	if !ok {
		t.Fatal("Get(\"test.metric\") returned not ok, want ok")
	}
	if got.Key() != "test.metric" {
		t.Errorf("Get().Key() = %q, want %q", got.Key(), "test.metric")
	}
}

func TestAlertMetricRegistry_Get_NotFound(t *testing.T) {
	r := monitor.NewAlertMetricRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("Get(\"nonexistent\") returned ok, want not ok")
	}
}

func TestAlertMetricRegistry_Register_NilRegistry(t *testing.T) {
	var r *monitor.AlertMetricRegistry
	r.Register(&stubAlertMetric{keyVal: "test"})
}

func TestAlertMetricRegistry_Register_NilMetric(t *testing.T) {
	r := monitor.NewAlertMetricRegistry()
	r.Register(nil)
	if len(r.List()) != 0 {
		t.Error("Register(nil) should not add anything")
	}
}

func TestAlertMetricRegistry_Get_NilRegistry(t *testing.T) {
	var r *monitor.AlertMetricRegistry
	_, ok := r.Get("test")
	if ok {
		t.Error("nil.Get() returned ok, want not ok")
	}
}

func TestAlertMetricRegistry_List_NilRegistry(t *testing.T) {
	var r *monitor.AlertMetricRegistry
	list := r.List()
	if list != nil {
		t.Errorf("nil.List() = %v, want nil", list)
	}
}

func TestAlertMetricRegistry_List_Empty(t *testing.T) {
	r := monitor.NewAlertMetricRegistry()
	list := r.List()
	if len(list) != 0 {
		t.Errorf("List() = %d items, want 0", len(list))
	}
}

func TestAlertMetricRegistry_List_Multiple(t *testing.T) {
	r := monitor.NewAlertMetricRegistry()
	r.Register(&stubAlertMetric{keyVal: "c.metric"})
	r.Register(&stubAlertMetric{keyVal: "a.metric"})
	r.Register(&stubAlertMetric{keyVal: "b.metric"})

	list := r.List()
	if len(list) != 3 {
		t.Fatalf("List() = %d items, want 3", len(list))
	}
	if list[0].Key() != "a.metric" {
		t.Errorf("list[0].Key() = %q, want %q (sorted)", list[0].Key(), "a.metric")
	}
	if list[1].Key() != "b.metric" {
		t.Errorf("list[1].Key() = %q, want %q (sorted)", list[1].Key(), "b.metric")
	}
	if list[2].Key() != "c.metric" {
		t.Errorf("list[2].Key() = %q, want %q (sorted)", list[2].Key(), "c.metric")
	}
}

func TestAlertMetricRegistry_Register_DuplicateOverwrites(t *testing.T) {
	r := monitor.NewAlertMetricRegistry()
	r.Register(&stubAlertMetric{keyVal: "test.metric", descVal: "first"})
	r.Register(&stubAlertMetric{keyVal: "test.metric", descVal: "second"})

	got, ok := r.Get("test.metric")
	if !ok {
		t.Fatal("Get() returned not ok")
	}
	if got.Description() != "second" {
		t.Errorf("Get().Description() = %q, want %q (overwritten)", got.Description(), "second")
	}
	list := r.List()
	if len(list) != 1 {
		t.Errorf("List() = %d items, want 1 (duplicate replaces)", len(list))
	}
}

type stubDeadLetterReader struct{ n int }

func (s stubDeadLetterReader) DeadLetterCount() int { return s.n }

func TestSequencerDeadLetterMetric_Evaluate(t *testing.T) {
	m := monitor.NewSequencerDeadLetterMetric(stubDeadLetterReader{n: 3})
	if m.Key() != "sequencer.dead_letter_count" {
		t.Errorf("Key() = %q, want %q", m.Key(), "sequencer.dead_letter_count")
	}
	v, err := m.Evaluate(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v != 3 {
		t.Errorf("Evaluate() = %v, want 3", v)
	}
}

func TestSequencerDeadLetterMetric_Evaluate_NilReader(t *testing.T) {
	m := monitor.NewSequencerDeadLetterMetric(nil)
	v, err := m.Evaluate(context.Background(), time.Minute)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v != 0 {
		t.Errorf("Evaluate() with nil reader = %v, want 0", v)
	}
}

func TestRunnerErrorRateMetric_Key(t *testing.T) {
	m := monitor.NewRunnerErrorRateMetric(nil, nil)
	if m.Key() != "runner.error_rate" {
		t.Errorf("Key() = %q, want %q", m.Key(), "runner.error_rate")
	}
}

func TestRunnerErrorRateMetric_Description(t *testing.T) {
	m := monitor.NewRunnerErrorRateMetric(nil, nil)
	if m.Description() == "" {
		t.Error("Description() is empty, want non-empty")
	}
}

func TestRunnerErrorRateMetric_Evaluate_RingBuffer(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	rb.RecordCompletion("success", 100)
	rb.RecordCompletion("error", 200)
	rb.RecordCompletion("success", 150)

	m := monitor.NewRunnerErrorRateMetric(nil, rb)
	val, err := m.Evaluate(context.Background(), 60*time.Minute)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if val != 1.0/3.0 {
		t.Errorf("Evaluate() = %.4f, want %.4f", val, 1.0/3.0)
	}
}

func TestRunnerErrorRateMetric_Evaluate_RingBufferZeroTotal(t *testing.T) {
	rb := monitor.NewMetricRingBuffer()
	m := monitor.NewRunnerErrorRateMetric(nil, rb)
	val, err := m.Evaluate(context.Background(), 60*time.Minute)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if val != 0 {
		t.Errorf("Evaluate() = %.4f, want 0", val)
	}
}

func TestSkillFilesystemMissingMetric_Key(t *testing.T) {
	m := monitor.NewSkillFilesystemMissingMetric(nil)
	if m.Key() != "skill.filesystem_missing_count" {
		t.Errorf("Key() = %q, want %q", m.Key(), "skill.filesystem_missing_count")
	}
}

func TestSkillFilesystemMissingMetric_Description(t *testing.T) {
	m := monitor.NewSkillFilesystemMissingMetric(nil)
	if m.Description() == "" {
		t.Error("Description() is empty, want non-empty")
	}
}

func TestSkillFilesystemMissingMetric_Evaluate_NilReader(t *testing.T) {
	m := monitor.NewSkillFilesystemMissingMetric(nil)
	val, err := m.Evaluate(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if val != 0 {
		t.Errorf("Evaluate() = %.2f, want 0", val)
	}
}

type stubFsHealthReader struct {
	missing int
	pending int
	err     error
}

func (s *stubFsHealthReader) FilesystemHealthStats(_ context.Context) (int, int, error) {
	return s.missing, s.pending, s.err
}

func TestSkillFilesystemMissingMetric_Evaluate_WithReader(t *testing.T) {
	fs := &stubFsHealthReader{missing: 5, pending: 2}
	m := monitor.NewSkillFilesystemMissingMetric(fs)
	val, err := m.Evaluate(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("Evaluate() error: %v", err)
	}
	if val != 5 {
		t.Errorf("Evaluate() = %.0f, want 5", val)
	}
}

func TestSkillFilesystemMissingMetric_Evaluate_ReaderError(t *testing.T) {
	fs := &stubFsHealthReader{err: context.DeadlineExceeded}
	m := monitor.NewSkillFilesystemMissingMetric(fs)
	_, err := m.Evaluate(context.Background(), time.Hour)
	if err == nil {
		t.Error("Evaluate() returned nil error, want error from reader")
	}
}
