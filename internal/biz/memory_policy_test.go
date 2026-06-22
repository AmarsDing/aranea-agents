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

func TestIsNegationConflict(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want bool
	}{
		// Exact match after stripping negation prefix.
		{"english not exact", "like python", "not like python", true},
		{"english don't exact", "want dark mode", "don't want dark mode", true},
		{"english doesn't exact", "work", "doesn't work", true},

		// Auxiliary verb path: negation prefix appears after first word(s).
		{"auxiliary does not", "user like python", "user does not like python", true},
		{"auxiliary is not", "product available", "product is not available", true},

		// Chinese: exact prefix match (core equals other after strip).
		{"chinese 不喜欢", "python", "不喜欢python", true},
		{"chinese 不需要", "通知", "不需要通知", true},
		{"chinese 不", "go", "不go", true},

		// Non-conflicts: no negation prefix in either statement.
		{"different topics", "like python", "like javascript", false},
		{"both positive", "like python", "like python", false},
		{"unrelated statements", "likes python and java", "likes python and javascript and go", false},

		// Both statements are negated: cores still match, so conflict is detected.
		{"both negative cores match", "does not like python", "doesn't like python", true},

		// Short core clause.
		{"short core exact", "go", "not go", true},

		// Below 75% word overlap: core words don't sufficiently overlap other.
		{"below 75% overlap", "like python", "like javascript", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNegationConflict(tt.a, tt.b, negationPatterns)
			if got != tt.want {
				t.Errorf("isNegationConflict(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestStripPrefix(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		prefix string
		core   string
		ok     bool
	}{
		// Direct prefix at start.
		{"not at start", " not available", "not ", "available", true},
		{"don't at start", " don't like python", "don't ", "like python", true},
		{"doesn't at start", " doesn't work", "doesn't ", "work", true},
		{"chinese 不喜欢", " 不喜欢python", "不喜欢", "python", true},
		{"chinese 不需要", " 不需要通知", "不需要", "通知", true},

		// No matching prefix.
		{"no prefix match", " likes python", "not ", "", false},

		// Auxiliary verb path: prefix found after first word(s).
		{"auxiliary does not", " does not like python", "not ", "like python", true},
		{"auxiliary is not", " product is not available", "not ", "available", true},

		// Prefix matches but leaves empty core.
		{"empty core after strip", " not ", "not ", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, ok := stripPrefix(tt.s, tt.prefix)
			if ok != tt.ok {
				t.Errorf("stripPrefix(%q, %q) ok = %v, want %v", tt.s, tt.prefix, ok, tt.ok)
			}
			if ok && core != tt.core {
				t.Errorf("stripPrefix(%q, %q) core = %q, want %q", tt.s, tt.prefix, core, tt.core)
			}
		})
	}
}

func TestNegationCoreMatches(t *testing.T) {
	tests := []struct {
		name  string
		core  string
		other string
		want  bool
	}{
		// Exact match.
		{"exact match", "available", "available", true},

		// 75% word overlap (3 of 4 core words appear in other).
		{"75% overlap", "like python and java", "like python and javascript and go", true},

		// Below threshold.
		{"below threshold", "like python", "like javascript", false},

		// Empty inputs.
		{"empty core", "", "anything", false},
		{"empty other", "something", "", false},

		// All words match (100% overlap).
		{"full overlap", "like python", "user like python", true},

		// Case-insensitive matching.
		{"case insensitive", "Like Python", "like python", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := negationCoreMatches(tt.core, tt.other)
			if got != tt.want {
				t.Errorf("negationCoreMatches(%q, %q) = %v, want %v", tt.core, tt.other, got, tt.want)
			}
		})
	}
}
