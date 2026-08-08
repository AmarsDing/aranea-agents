package evaluation

import (
	"context"
	"strings"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	beval "aranea-agents/internal/biz/evaluation"
	"aranea-agents/pkg/loggateway"
)

// fakeTrendRepo extends fakeEvalRepo with programmable trend points.
type fakeTrendRepo struct {
	*fakeEvalRepo
	points []beval.TrendPoint
}

func (f *fakeTrendRepo) ListTrendPoints(context.Context, string, string, int) ([]beval.TrendPoint, error) {
	return append([]beval.TrendPoint(nil), f.points...), nil
}

// fakeConfigReader returns a fixed agent eval config.
type fakeConfigReader struct {
	cfg biz.AgentEvalAutoConfig
	err error
}

func (f fakeConfigReader) EvalAutoConfigForAgent(context.Context, string) (biz.AgentEvalAutoConfig, error) {
	return f.cfg, f.err
}

// captureBus records published events.
type captureBus struct {
	mu     sync.Mutex
	events []biz.Event
}

func (c *captureBus) Publish(_ context.Context, e biz.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func (c *captureBus) Subscribe(biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return nil, func() {}
}

func (c *captureBus) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

func (c *captureBus) last() biz.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.events) == 0 {
		return nil
	}
	return c.events[len(c.events)-1]
}

func trendPointsDescending(scores ...float32) []beval.TrendPoint {
	// ListTrendPoints returns newest-first rows.
	out := make([]beval.TrendPoint, 0, len(scores))
	for i, s := range scores {
		out = append(out, beval.TrendPoint{
			RunID:         string(rune('a'+i)) + "-run",
			CreatedAt:     "2026-08-08T00:00:0" + string(rune('0'+len(scores)-i)) + "Z",
			TriggerSource: triggerAfterTurn,
			LLMJudgeScore: s,
		})
	}
	return out
}

func TestScoreDropAlerterSkipsNonAfterTurnRuns(t *testing.T) {
	repo := &fakeTrendRepo{fakeEvalRepo: newFakeEvalRepo(), points: trendPointsDescending(0.5, 0.7, 0.9)}
	bus := &captureBus{}
	a := NewScoreDropAlerter(beval.NewUsecase(repo, loggateway.NewNoop()), fakeConfigReader{cfg: biz.AgentEvalAutoConfig{
		Enabled: true, DatasetID: "ds1", AlertConsecutiveDrops: 3, AlertMetric: "llm_as_judge",
	}}, bus, loggateway.NewNoop())

	a.CheckAfterRun(context.Background(), biz.EvalRun{ID: "x", DatasetID: "ds1", AgentID: "a1", Status: "completed", TriggerSource: "manual"})
	if bus.count() != 0 {
		t.Fatalf("manual run must not trigger drop detection, got %d events", bus.count())
	}
}

func TestScoreDropAlerterDisabledBelowTwoDrops(t *testing.T) {
	repo := &fakeTrendRepo{fakeEvalRepo: newFakeEvalRepo(), points: trendPointsDescending(0.5, 0.7)}
	bus := &captureBus{}
	a := NewScoreDropAlerter(beval.NewUsecase(repo, loggateway.NewNoop()), fakeConfigReader{cfg: biz.AgentEvalAutoConfig{
		Enabled: true, DatasetID: "ds1", AlertConsecutiveDrops: 0,
	}}, bus, loggateway.NewNoop())

	a.CheckAfterRun(context.Background(), biz.EvalRun{ID: "x", DatasetID: "ds1", AgentID: "a1", Status: "completed", TriggerSource: triggerAfterTurn})
	if bus.count() != 0 {
		t.Fatalf("drops=0 must disable alerting, got %d events", bus.count())
	}
}

func TestScoreDropAlerterFiresOnConsecutiveDrops(t *testing.T) {
	// newest-first: 0.40 < 0.55 < 0.80 → 3 consecutive drops (each newer run lower).
	repo := &fakeTrendRepo{fakeEvalRepo: newFakeEvalRepo(), points: trendPointsDescending(0.40, 0.55, 0.80)}
	bus := &captureBus{}
	a := NewScoreDropAlerter(beval.NewUsecase(repo, loggateway.NewNoop()), fakeConfigReader{cfg: biz.AgentEvalAutoConfig{
		Enabled: true, DatasetID: "ds1", AlertConsecutiveDrops: 3, AlertMetric: "llm_as_judge",
	}}, bus, loggateway.NewNoop())

	a.CheckAfterRun(context.Background(), biz.EvalRun{ID: "a-run", DatasetID: "ds1", AgentID: "a1", Status: "completed", TriggerSource: triggerAfterTurn})
	if bus.count() != 1 {
		t.Fatalf("expected exactly one drop alert, got %d", bus.count())
	}
	notice, ok := bus.last().(*biz.SystemNoticeEvent)
	if !ok {
		t.Fatalf("expected SystemNoticeEvent, got %T", bus.last())
	}
	if !strings.Contains(notice.Message, "llm_as_judge") || !strings.Contains(notice.Message, "0.40") {
		t.Fatalf("alert message must name metric and latest score, got %q", notice.Message)
	}
}

func TestScoreDropAlerterSilentWhenNotMonotonic(t *testing.T) {
	// newest-first: 0.70 > 0.55 → the newest run improved, no alert.
	repo := &fakeTrendRepo{fakeEvalRepo: newFakeEvalRepo(), points: trendPointsDescending(0.70, 0.55, 0.80)}
	bus := &captureBus{}
	a := NewScoreDropAlerter(beval.NewUsecase(repo, loggateway.NewNoop()), fakeConfigReader{cfg: biz.AgentEvalAutoConfig{
		Enabled: true, DatasetID: "ds1", AlertConsecutiveDrops: 3, AlertMetric: "llm_as_judge",
	}}, bus, loggateway.NewNoop())

	a.CheckAfterRun(context.Background(), biz.EvalRun{ID: "a-run", DatasetID: "ds1", AgentID: "a1", Status: "completed", TriggerSource: triggerAfterTurn})
	if bus.count() != 0 {
		t.Fatalf("non-monotonic trend must not alert, got %d events", bus.count())
	}
}

func TestScoreDropAlerterSilentWithInsufficientPoints(t *testing.T) {
	repo := &fakeTrendRepo{fakeEvalRepo: newFakeEvalRepo(), points: trendPointsDescending(0.40, 0.55)}
	bus := &captureBus{}
	a := NewScoreDropAlerter(beval.NewUsecase(repo, loggateway.NewNoop()), fakeConfigReader{cfg: biz.AgentEvalAutoConfig{
		Enabled: true, DatasetID: "ds1", AlertConsecutiveDrops: 3, AlertMetric: "llm_as_judge",
	}}, bus, loggateway.NewNoop())

	a.CheckAfterRun(context.Background(), biz.EvalRun{ID: "a-run", DatasetID: "ds1", AgentID: "a1", Status: "completed", TriggerSource: triggerAfterTurn})
	if bus.count() != 0 {
		t.Fatalf("fewer points than drops threshold must not alert, got %d events", bus.count())
	}
}

func TestScoreDropAlerterUsesConfiguredMetric(t *testing.T) {
	// exact_match newest-first: 0.30 < 0.60 < 0.90 → drops on exact_match.
	pts := trendPointsDescending(0.40, 0.55, 0.80) // llm improves (no llm alert)
	for i := range pts {
		pts[i].ExactMatchScore = []float32{0.30, 0.60, 0.90}[i]
	}
	repo := &fakeTrendRepo{fakeEvalRepo: newFakeEvalRepo(), points: pts}
	bus := &captureBus{}
	a := NewScoreDropAlerter(beval.NewUsecase(repo, loggateway.NewNoop()), fakeConfigReader{cfg: biz.AgentEvalAutoConfig{
		Enabled: true, DatasetID: "ds1", AlertConsecutiveDrops: 3, AlertMetric: "exact_match",
	}}, bus, loggateway.NewNoop())

	a.CheckAfterRun(context.Background(), biz.EvalRun{ID: "a-run", DatasetID: "ds1", AgentID: "a1", Status: "completed", TriggerSource: triggerAfterTurn})
	if bus.count() != 1 {
		t.Fatalf("expected alert on configured exact_match metric, got %d events", bus.count())
	}
	notice := bus.last().(*biz.SystemNoticeEvent)
	if !strings.Contains(notice.Message, "exact_match") {
		t.Fatalf("alert must reference exact_match, got %q", notice.Message)
	}
}

func TestScoreDropAlerterNilSafe(t *testing.T) {
	var a *ScoreDropAlerter
	a.CheckAfterRun(context.Background(), biz.EvalRun{ID: "x", Status: "completed", TriggerSource: triggerAfterTurn})
}
