package biz

import (
	"context"
	"testing"
)

type memoryActionLogRecorder struct {
	last MemoryPolicyRecord
}

func (r *memoryActionLogRecorder) WriteMemoryActionLog(_ context.Context, rec MemoryPolicyRecord) error {
	r.last = rec
	return nil
}

func TestMemoryPolicyEngine_RecordBestEffort(t *testing.T) {
	rec := &memoryActionLogRecorder{}
	engine := NewMemoryPolicyEngineStatic(rec, false)
	engine.RecordBestEffort(context.Background(), MemoryPolicyRecord{
		Action:     "DECAY",
		TargetKind: "episode_scope",
		TargetID:   "agent-1",
		Reason:     "batch",
	})
	if rec.last.Action != "DECAY" {
		t.Fatalf("action=%q", rec.last.Action)
	}
	if rec.last.PolicyVersion != PolicyVersionConsolidateV1 {
		t.Fatalf("policy_version=%q", rec.last.PolicyVersion)
	}
}

func TestMemoryPolicyEngine_RecordUsesExplicitVersion(t *testing.T) {
	rec := &memoryActionLogRecorder{}
	engine := NewMemoryPolicyEngineStatic(rec, false)
	_ = engine.Record(context.Background(), MemoryPolicyRecord{
		Action:        "PROPOSE",
		TargetKind:    "cascade_proposal",
		TargetID:      "p1",
		PolicyVersion: PolicyVersionCascadeV1,
	})
	if rec.last.PolicyVersion != PolicyVersionCascadeV1 {
		t.Fatalf("policy_version=%q", rec.last.PolicyVersion)
	}
}

func TestMemoryPolicyEngine_StrictEnabled(t *testing.T) {
	engine := NewMemoryPolicyEngineStatic(&memoryActionLogRecorder{}, true)
	if !engine.StrictEnabled(context.Background()) {
		t.Fatal("expected strict")
	}
	relaxed := NewMemoryPolicyEngineStatic(&memoryActionLogRecorder{}, false)
	if relaxed.StrictEnabled(context.Background()) {
		t.Fatal("expected non-strict")
	}
}
