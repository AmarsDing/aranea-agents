package evaluation

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	beval "aranea-agents/internal/biz/evaluation"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// gateFixture wires a PublishGate over the fake repo + echo legacy runner.
// The echo agent replies with the case input, so cases whose expected_output
// equals their input score exact_match=1 and mismatched cases score 0.
func gateFixture(repo *fakeEvalRepo) (*PublishGate, *captureBus) {
	uc := beval.NewUsecase(repo, loggateway.NewNoop())
	runner := NewRunner(uc, echoAgent, nil, loggateway.NewNoop())
	bus := &captureBus{}
	return NewPublishGate(uc, runner, bus, loggateway.NewNoop()), bus
}

// waitBusCount polls the capture bus until want events have arrived or the
// deadline passes. Gate evaluation runs on a background goroutine (Y2), so
// tests asserting the advisory outcome must wait for it.
func waitBusCount(t *testing.T, bus *captureBus, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if bus.count() >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d bus events, got %d", want, bus.count())
}

// lastNoticeMessage returns the Message of the most recent SystemNoticeEvent.
func lastNoticeMessage(t *testing.T, bus *captureBus) string {
	t.Helper()
	ev, ok := bus.last().(*biz.SystemNoticeEvent)
	if !ok || ev == nil {
		t.Fatalf("last event is not a SystemNoticeEvent: %T", bus.last())
	}
	return ev.Message
}

func TestPublishGateDisabledIsNoop(t *testing.T) {
	repo := newFakeEvalRepo()
	gate, bus := gateFixture(repo)
	if err := gate.Check(context.Background(), beval.GateTriggerSkillPublish); err != nil {
		t.Fatalf("disabled gate must allow, got %v", err)
	}
	if bus.count() != 0 {
		t.Fatalf("disabled gate must not publish, got %d events", bus.count())
	}
}

func TestPublishGateNilSafe(t *testing.T) {
	var gate *PublishGate
	if err := gate.Check(context.Background(), beval.GateTriggerSkillPublish); err != nil {
		t.Fatalf("nil gate must allow, got %v", err)
	}
}

func TestPublishGatePassesWhenScoreAboveFloor(t *testing.T) {
	repo := newFakeEvalRepo()
	repo.datasets["ds1"] = beval.Dataset{ID: "ds1"}
	repo.cases["ds1"] = []beval.Case{
		{ID: "c1", DatasetID: "ds1", Input: "hello", ExpectedOutput: "hello"},
	}
	repo.gateCfg = beval.GateConfig{
		Enabled: true, AgentID: "a1", DatasetID: "ds1",
		Metric: "exact_match", MinScore: 0.8,
	}
	gate, bus := gateFixture(repo)
	if err := gate.Check(context.Background(), beval.GateTriggerSkillPublish); err != nil {
		t.Fatalf("score 1.0 >= floor 0.8 must pass, got %v", err)
	}
	if bus.count() != 0 {
		t.Fatalf("passing gate must not publish, got %d events", bus.count())
	}
	// The gate run must be recorded with the gate trigger source so trend
	// panels can split it out of manual/after_turn series.
	if n := len(repo.gateRunsSnapshot()); n != 1 {
		t.Fatalf("expected 1 gate run recorded, got %d", n)
	}
}

// Y2: a below-floor regression must NOT block the publish — Check allows and
// the breach surfaces as an advisory notification once the background run
// completes.
func TestPublishGateAdvisesBelowFloor(t *testing.T) {
	repo := newFakeEvalRepo()
	repo.datasets["ds1"] = beval.Dataset{ID: "ds1"}
	repo.cases["ds1"] = []beval.Case{
		{ID: "c1", DatasetID: "ds1", Input: "hello", ExpectedOutput: "world"},
	}
	repo.gateCfg = beval.GateConfig{
		Enabled: true, AgentID: "a1", DatasetID: "ds1",
		Metric: "exact_match", MinScore: 0.8,
	}
	gate, bus := gateFixture(repo)
	if err := gate.Check(context.Background(), beval.GateTriggerSkillPublish); err != nil {
		t.Fatalf("advisory gate must allow publish, got %v", err)
	}
	waitBusCount(t, bus, 1)
	msg := lastNoticeMessage(t, bus)
	if !strings.Contains(msg, "低于下限") || !strings.Contains(msg, "已放行") {
		t.Fatalf("expected advisory below-floor notice, got %q", msg)
	}
}

// Y2: an excessive drop vs the baseline is likewise advisory — the publish
// proceeds and admins are notified after the fact.
func TestPublishGateAdvisesExcessiveDrop(t *testing.T) {
	repo := newFakeEvalRepo()
	repo.datasets["ds1"] = beval.Dataset{ID: "ds1"}
	repo.cases["ds1"] = []beval.Case{
		{ID: "c1", DatasetID: "ds1", Input: "hello", ExpectedOutput: "world"},
	}
	// Baseline: a completed run with exact_match 0.9. Gate run scores 0.0;
	// max_drop 0.2 → 0.0 < 0.9-0.2 → advisory breach.
	repo.runs["base"] = beval.Run{
		ID: "base", DatasetID: "ds1", AgentID: "a1", Status: "completed",
		ExactMatchScore: 0.9, CreatedAt: "2026-08-07T00:00:00Z",
	}
	repo.gateCfg = beval.GateConfig{
		Enabled: true, AgentID: "a1", DatasetID: "ds1",
		Metric: "exact_match", MaxDrop: 0.2,
	}
	gate, bus := gateFixture(repo)
	if err := gate.Check(context.Background(), beval.GateTriggerPackInstall); err != nil {
		t.Fatalf("advisory gate must allow install, got %v", err)
	}
	waitBusCount(t, bus, 1)
	msg := lastNoticeMessage(t, bus)
	if !strings.Contains(msg, "下跌") || !strings.Contains(msg, "已放行") {
		t.Fatalf("expected advisory drop notice, got %q", msg)
	}
}

// Y12: max_drop configured but no completed baseline exists — the drop check
// would be silently skipped, so this is the one remaining hard block. The
// gate run launched alongside measures the pre-publish state and becomes the
// baseline for the retry.
func TestPublishGateBlocksWithoutBaseline(t *testing.T) {
	repo := newFakeEvalRepo()
	repo.datasets["ds1"] = beval.Dataset{ID: "ds1"}
	repo.cases["ds1"] = []beval.Case{
		{ID: "c1", DatasetID: "ds1", Input: "hello", ExpectedOutput: "hello"},
	}
	repo.gateCfg = beval.GateConfig{
		Enabled: true, AgentID: "a1", DatasetID: "ds1",
		Metric: "exact_match", MaxDrop: 0.2,
	}
	gate, bus := gateFixture(repo)
	err := gate.Check(context.Background(), beval.GateTriggerSkillPublish)
	if err == nil {
		t.Fatal("max_drop without baseline must block (Y12)")
	}
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("no-baseline block must be Conflict, got %v", err)
	}
	if !strings.Contains(err.Error(), "无可用基线") {
		t.Fatalf("expected baseline reason in message, got %q", err.Error())
	}
	if bus.count() != 1 {
		t.Fatalf("block must publish one notice, got %d", bus.count())
	}
	if n := len(repo.gateRunsSnapshot()); n != 1 {
		t.Fatalf("no-baseline block must still launch a baseline run, got %d gate runs", n)
	}
}

// A publish burst must not fan out N full-dataset evaluations: while one
// gate run is pending/running, further publishes pass without launching
// duplicates (the in-flight run covers the current code state).
func TestPublishGateDeduplicatesInFlightRun(t *testing.T) {
	repo := newFakeEvalRepo()
	repo.datasets["ds1"] = beval.Dataset{ID: "ds1"}
	repo.cases["ds1"] = []beval.Case{
		{ID: "c1", DatasetID: "ds1", Input: "hello", ExpectedOutput: "hello"},
	}
	repo.runs["inflight"] = beval.Run{
		ID: "inflight", DatasetID: "ds1", AgentID: "a1", Status: "running",
		TriggerSource: triggerGate, CreatedAt: "2026-08-07T00:00:00Z",
	}
	repo.gateCfg = beval.GateConfig{
		Enabled: true, AgentID: "a1", DatasetID: "ds1",
		Metric: "exact_match", MinScore: 0.5,
	}
	gate, bus := gateFixture(repo)
	if err := gate.Check(context.Background(), beval.GateTriggerSkillPublish); err != nil {
		t.Fatalf("in-flight dedup must allow, got %v", err)
	}
	if n := len(repo.gateRunsSnapshot()); n != 1 {
		t.Fatalf("in-flight gate run must suppress a duplicate launch, got %d gate runs", n)
	}
	if bus.count() != 0 {
		t.Fatalf("dedup must not publish, got %d events", bus.count())
	}
}

func TestPublishGateAllowsWithinDropTolerance(t *testing.T) {
	repo := newFakeEvalRepo()
	repo.datasets["ds1"] = beval.Dataset{ID: "ds1"}
	repo.cases["ds1"] = []beval.Case{
		{ID: "c1", DatasetID: "ds1", Input: "hello", ExpectedOutput: "hello"},
	}
	// Baseline 1.0, gate run 1.0 (echo), max_drop 0.2 → no drop → pass.
	repo.runs["base"] = beval.Run{
		ID: "base", DatasetID: "ds1", AgentID: "a1", Status: "completed",
		ExactMatchScore: 1.0, CreatedAt: "2026-08-07T00:00:00Z",
	}
	repo.gateCfg = beval.GateConfig{
		Enabled: true, AgentID: "a1", DatasetID: "ds1",
		Metric: "exact_match", MaxDrop: 0.2,
	}
	gate, _ := gateFixture(repo)
	if err := gate.Check(context.Background(), beval.GateTriggerPackInstall); err != nil {
		t.Fatalf("no drop must pass, got %v", err)
	}
}

func TestPublishGateUpdateConfigValidation(t *testing.T) {
	repo := newFakeEvalRepo()
	uc := beval.NewUsecase(repo, loggateway.NewNoop())
	// Enabled gate requires agent + dataset.
	_, err := uc.UpdateGateConfig(context.Background(), beval.GateConfig{Enabled: true})
	if err == nil {
		t.Fatal("enabled gate without agent/dataset must be rejected")
	}
	// Out-of-range thresholds are normalized into [0,1].
	cfg, err := uc.UpdateGateConfig(context.Background(), beval.GateConfig{
		Enabled: true, AgentID: "a1", DatasetID: "ds1", MinScore: 2, MaxDrop: -1,
	})
	if err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if cfg.MinScore != 1 || cfg.MaxDrop != 0 {
		t.Fatalf("thresholds must clamp to [0,1], got min=%v max=%v", cfg.MinScore, cfg.MaxDrop)
	}
	if cfg.Metric != "exact_match" {
		t.Fatalf("empty metric must default to exact_match, got %q", cfg.Metric)
	}
}

var _ biz.EventBus = (*captureBus)(nil)
