package evaluation

import (
	"context"
	"strings"
	"testing"

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
	gateRuns := 0
	for _, r := range repo.runs {
		if r.TriggerSource == triggerGate {
			gateRuns++
		}
	}
	if gateRuns != 1 {
		t.Fatalf("expected 1 gate run recorded, got %d", gateRuns)
	}
}

func TestPublishGateBlocksBelowFloor(t *testing.T) {
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
	err := gate.Check(context.Background(), beval.GateTriggerSkillPublish)
	if err == nil {
		t.Fatal("score 0.0 < floor 0.8 must block")
	}
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("blocked publish must be Conflict, got %v", err)
	}
	if bus.count() != 1 {
		t.Fatalf("blocked gate must publish one notice, got %d", bus.count())
	}
}

func TestPublishGateBlocksOnExcessiveDrop(t *testing.T) {
	repo := newFakeEvalRepo()
	repo.datasets["ds1"] = beval.Dataset{ID: "ds1"}
	repo.cases["ds1"] = []beval.Case{
		{ID: "c1", DatasetID: "ds1", Input: "hello", ExpectedOutput: "world"},
	}
	// Baseline: a completed run with exact_match 0.9. Gate run scores 0.0;
	// max_drop 0.2 → 0.0 < 0.9-0.2 → block.
	repo.runs["base"] = beval.Run{
		ID: "base", DatasetID: "ds1", AgentID: "a1", Status: "completed",
		ExactMatchScore: 0.9, CreatedAt: "2026-08-07T00:00:00Z",
	}
	repo.gateCfg = beval.GateConfig{
		Enabled: true, AgentID: "a1", DatasetID: "ds1",
		Metric: "exact_match", MaxDrop: 0.2,
	}
	gate, bus := gateFixture(repo)
	err := gate.Check(context.Background(), beval.GateTriggerPackInstall)
	if err == nil {
		t.Fatal("drop 0.9 → 0.0 beyond max_drop 0.2 must block")
	}
	if !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("blocked install must be Conflict, got %v", err)
	}
	if !strings.Contains(err.Error(), "下跌") {
		t.Fatalf("expected drop reason in message, got %q", err.Error())
	}
	if bus.count() != 1 {
		t.Fatalf("blocked gate must publish one notice, got %d", bus.count())
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
