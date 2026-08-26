package decision

import (
	"context"
	"testing"
)

type captureGateCollector struct {
	recs []Record
}

func (c *captureGateCollector) Emit(_ context.Context, r Record) {
	c.recs = append(c.recs, r)
}

// TestEmitGate_FullMapping pins the S2 GateDecision → system_guard Record
// mapping (design §3.2 row 3): category/actor/source_ref/metadata 全字段。
func TestEmitGate_FullMapping(t *testing.T) {
	cc := &captureGateCollector{}
	EmitGate(context.Background(), cc, GateDecision{
		TriggerRule:   TriggerTokenBudgetTripped,
		Outcome:       "tripped",
		Scenario:      "run 累计 input token 超预算",
		Reasoning:     "run 累计 input 超 150 万",
		GuardName:     "token_budget",
		RunID:         "run-1",
		Entities:      []EntityRef{{Type: "team", Key: "team-9"}},
		ObservedValue: int64(1_600_000),
		Threshold:     int64(1_500_000),
		Action:        "cancel_run",
		Extra:         map[string]any{"session_id": "sess-1"},
	})
	if len(cc.recs) != 1 {
		t.Fatalf("expected 1 record, got %d", len(cc.recs))
	}
	r := cc.recs[0]
	if r.Category != CategorySystemGuard {
		t.Errorf("category = %q, want system_guard", r.Category)
	}
	if r.Outcome != "tripped" {
		t.Errorf("outcome = %q, want tripped", r.Outcome)
	}
	if r.ActorType != ActorSystem || r.ActorKey != "system:token_budget" {
		t.Errorf("actor = %q/%q, want system/system:token_budget", r.ActorType, r.ActorKey)
	}
	if r.SourceRef.RunID != "run-1" {
		t.Errorf("source_ref.run_id = %q, want run-1", r.SourceRef.RunID)
	}
	if got := r.Metadata["trigger_rule"]; got != TriggerTokenBudgetTripped {
		t.Errorf("metadata.trigger_rule = %v", got)
	}
	if got := r.Metadata["observed_value"]; got != int64(1_600_000) {
		t.Errorf("metadata.observed_value = %v", got)
	}
	if got := r.Metadata["threshold"]; got != int64(1_500_000) {
		t.Errorf("metadata.threshold = %v", got)
	}
	if got := r.Metadata["action"]; got != "cancel_run" {
		t.Errorf("metadata.action = %v", got)
	}
	if got := r.Metadata["session_id"]; got != "sess-1" {
		t.Errorf("metadata.session_id (extra) = %v", got)
	}
	if r.DecisionKey == "" {
		t.Error("decision_key must be auto-assigned")
	}
	if err := r.Validate(); err != nil {
		t.Errorf("emitted record must pass validation: %v", err)
	}
}

// TestEmitGate_OptionalFieldsOmitted: 零值可选字段不进 metadata。
func TestEmitGate_OptionalFieldsOmitted(t *testing.T) {
	cc := &captureGateCollector{}
	EmitGate(context.Background(), cc, GateDecision{
		TriggerRule: TriggerLoopGuardBlocked,
		Outcome:     "blocked",
		GuardName:   "tool_loop_guard",
	})
	r := cc.recs[0]
	for _, k := range []string{"observed_value", "threshold", "action"} {
		if _, ok := r.Metadata[k]; ok {
			t.Errorf("metadata[%q] must be omitted when zero", k)
		}
	}
	if len(r.Metadata) != 1 {
		t.Errorf("metadata = %v, want only trigger_rule", r.Metadata)
	}
}

// TestEmitGate_ExtraCannotOverwriteReserved: Extra 不覆盖保留键。
func TestEmitGate_ExtraCannotOverwriteReserved(t *testing.T) {
	cc := &captureGateCollector{}
	EmitGate(context.Background(), cc, GateDecision{
		TriggerRule: TriggerNoProgressTripped,
		Outcome:     "tripped",
		GuardName:   "no_progress_auditor",
		Extra:       map[string]any{"trigger_rule": "forged", "agent_key": "ops"},
	})
	r := cc.recs[0]
	if got := r.Metadata["trigger_rule"]; got != TriggerNoProgressTripped {
		t.Errorf("reserved key overwritten: %v", got)
	}
	if got := r.Metadata["agent_key"]; got != "ops" {
		t.Errorf("extra key lost: %v", got)
	}
}

// TestEmitGate_SkipsIncomplete: 必填语义字段缺失时不产出记录。
func TestEmitGate_SkipsIncomplete(t *testing.T) {
	for name, gd := range map[string]GateDecision{
		"no_trigger": {Outcome: "tripped", GuardName: "g"},
		"no_outcome": {TriggerRule: TriggerParamRuleDeny, GuardName: "g"},
		"no_guard":   {TriggerRule: TriggerParamRuleDeny, Outcome: "blocked"},
	} {
		cc := &captureGateCollector{}
		EmitGate(context.Background(), cc, gd)
		if len(cc.recs) != 0 {
			t.Errorf("%s: expected no record, got %d", name, len(cc.recs))
		}
	}
}

// TestEmitGate_NilCollector 不 panic。
func TestEmitGate_NilCollector(t *testing.T) {
	EmitGate(context.Background(), nil, GateDecision{
		TriggerRule: TriggerTokenBudgetTripped, Outcome: "tripped", GuardName: "g",
	})
}

// TestGateTriggerEnumComplete C6 枚举一次列全且互异。
func TestGateTriggerEnumComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, tr := range []string{
		TriggerTokenBudgetTripped, TriggerNoProgressTripped, TriggerLoopGuardBlocked,
		TriggerParamRuleDeny, TriggerTeamCountMismatch,
		TriggerToolResultPruned, TriggerContextCompacted,
	} {
		if tr == "" {
			t.Fatal("empty trigger rule constant")
		}
		if seen[tr] {
			t.Fatalf("duplicate trigger rule %q", tr)
		}
		seen[tr] = true
	}
	if len(seen) != 7 {
		t.Fatalf("trigger enum = %d, want 7 (C6)", len(seen))
	}
}
