package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/monitor/heal"
	"aranea-agents/pkg/loggateway"
	"context"
	"strings"
	"testing"
)

type siFakePatternKB struct {
	byHash     map[string]*heal.FailurePattern
	created    []heal.FailurePattern
	incrementd []string
}

func (f *siFakePatternKB) GetByPatternHash(_ context.Context, hash string) (*heal.FailurePattern, error) {
	return f.byHash[hash], nil
}
func (f *siFakePatternKB) Create(_ context.Context, p heal.FailurePattern) error {
	f.created = append(f.created, p)
	return nil
}
func (f *siFakePatternKB) IncrementFail(_ context.Context, id string) error {
	f.incrementd = append(f.incrementd, id)
	return nil
}

func TestSIKBNegativePatternSink_CreateNew(t *testing.T) {
	kb := &siFakePatternKB{byHash: map[string]*heal.FailurePattern{}}
	sink := NewSIKBNegativePatternSink(kb, loggateway.NewNoop())
	rec := biz.SINegativePatternRecord{RunID: "run-1", TriggerSource: biz.TriggerSourceErrorCluster, PatternHash: "h1", PatternRegex: "x.go"}
	if err := sink.RecordNegativePattern(context.Background(), rec); err != nil {
		t.Fatalf("RecordNegativePattern: %v", err)
	}
	if len(kb.created) != 1 {
		t.Fatalf("created = %d, want 1", len(kb.created))
	}
	p := kb.created[0]
	if string(p.Source) != "self_improvement" || p.FailCount != 1 || !p.IsActive {
		t.Errorf("unexpected pattern: %+v", p)
	}
	if p.FixAction.Type != "log_only" {
		t.Errorf("negative pattern must be log_only, got %q", p.FixAction.Type)
	}
}

func TestSIKBNegativePatternSink_ExistingIncrementsFail(t *testing.T) {
	kb := &siFakePatternKB{byHash: map[string]*heal.FailurePattern{
		"h1": {ID: "p1", PatternHash: "h1"},
	}}
	sink := NewSIKBNegativePatternSink(kb, loggateway.NewNoop())
	rec := biz.SINegativePatternRecord{RunID: "run-2", TriggerSource: biz.TriggerSourceErrorCluster, PatternHash: "h1"}
	if err := sink.RecordNegativePattern(context.Background(), rec); err != nil {
		t.Fatalf("RecordNegativePattern: %v", err)
	}
	if len(kb.created) != 0 {
		t.Fatalf("created = %d, want 0 (dedup)", len(kb.created))
	}
	if len(kb.incrementd) != 1 || kb.incrementd[0] != "p1" {
		t.Fatalf("IncrementFail = %v, want [p1]", kb.incrementd)
	}
}

func TestSIOrchestratorFeedbackSink_Escalates(t *testing.T) {
	orch := biz.NewSkillEvolutionOrchestrator(nil, nil, loggateway.NewNoop())
	sink := NewSIOrchestratorFeedbackSink(orch)
	if err := sink.EscalateTriggerCooldown(context.Background(), biz.TriggerSourceErrorCluster, 2); err != nil {
		t.Fatalf("EscalateTriggerCooldown: %v", err)
	}

	if err := sink.EscalateTriggerCooldown(context.Background(), biz.TriggerSourceErrorCluster, 2); err != nil {
		t.Fatalf("second EscalateTriggerCooldown: %v", err)
	}
}

func TestSIMonitorNotifier_InsertsWarnEvent(t *testing.T) {
	events := &siFakeMonitorEvents{}
	n := NewSIMonitorNotifier(events, loggateway.NewNoop())
	run := &biz.SelfImprovementRun{ID: "run-1", SuggestionID: "sug-1"}
	if err := n.NotifySelfImprovement(context.Background(), run, "已自动应用"); err != nil {
		t.Fatalf("NotifySelfImprovement: %v", err)
	}
	if len(events.inserted) != 1 {
		t.Fatalf("inserted = %d, want 1", len(events.inserted))
	}
	ev := events.inserted[0]
	if ev.EventKey != "self_improvement.notify" || ev.Status != "warn" {
		t.Errorf("unexpected event: %+v", ev)
	}
	if !strings.Contains(ev.MetadataJSON, `"run_id":"run-1"`) {
		t.Errorf("metadata must carry run_id: %s", ev.MetadataJSON)
	}
}
