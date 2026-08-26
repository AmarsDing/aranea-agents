package biz

import "testing"

// --- Gate 1: kind whitelist ---

func TestIsFactKindWhitelisted(t *testing.T) {
	for _, k := range []string{"preference", "profile", "goal", "constraint", "decision", "relationship"} {
		if !IsFactKindWhitelisted(k) {
			t.Errorf("kind %q should be whitelisted", k)
		}
	}
	for _, k := range []string{"event", "knowledge", "fact", "other", "", "Preference"} {
		if IsFactKindWhitelisted(k) {
			t.Errorf("kind %q should NOT be whitelisted", k)
		}
	}
}

// --- Gates (pure) ---

func gateTestCandidate() FactWriteCandidate {
	return FactWriteCandidate{
		Statement:  "用户偏好简洁回复",
		FactKind:   "preference",
		Confidence: 0.9,
		Importance: 0.7,
	}
}

func TestGateFactWriteCandidate_Pass(t *testing.T) {
	d := GateFactWriteCandidate(gateTestCandidate())
	if d.DropReason != "" {
		t.Fatalf("expected pass, got drop reason %q", d.DropReason)
	}
}

func TestGateFactWriteCandidate_EmptyStatement(t *testing.T) {
	c := gateTestCandidate()
	c.Statement = "  "
	if d := GateFactWriteCandidate(c); d.DropReason != FactWriteDropEmptyStatement {
		t.Fatalf("drop reason: got %q want %q", d.DropReason, FactWriteDropEmptyStatement)
	}
}

func TestGateFactWriteCandidate_KindWhitelist(t *testing.T) {
	c := gateTestCandidate()
	c.FactKind = "event"
	if d := GateFactWriteCandidate(c); d.DropReason != FactWriteDropKindWhitelist {
		t.Fatalf("drop reason: got %q want %q", d.DropReason, FactWriteDropKindWhitelist)
	}
}

func TestGateFactWriteCandidate_Confidence(t *testing.T) {
	c := gateTestCandidate()
	c.Confidence = FactWriteMinConfidence - 0.01
	if d := GateFactWriteCandidate(c); d.DropReason != FactWriteDropConfidence {
		t.Fatalf("drop reason: got %q want %q", d.DropReason, FactWriteDropConfidence)
	}
	c.Confidence = FactWriteMinConfidence
	if d := GateFactWriteCandidate(c); d.DropReason != "" {
		t.Fatalf("confidence at threshold should pass, got %q", d.DropReason)
	}
}

func TestGateFactWriteCandidate_AbsenceMeta(t *testing.T) {
	// Absence meta-statements must be dropped even with a whitelisted kind and
	// high confidence — this is the auto_memory-path half of the 2026-08-26
	// domain-B pollution fix (the immediate writer has its own gate).
	c := gateTestCandidate()
	c.Statement = "用户询问值班电话，但暂无此信息记录"
	if d := GateFactWriteCandidate(c); d.DropReason != FactWriteDropAbsenceMeta {
		t.Fatalf("drop reason: got %q want %q", d.DropReason, FactWriteDropAbsenceMeta)
	}
	// Genuine negative fact (no inquiry marker) still passes.
	c.Statement = "原值班号码 8899-0000 已作废"
	if d := GateFactWriteCandidate(c); d.DropReason != "" {
		t.Fatalf("genuine negative fact should pass, got %q", d.DropReason)
	}
}

// --- Heuristic adjudication bands (pure) ---

func TestDecideFactWriteHeuristic_NoNeighbors(t *testing.T) {
	d := DecideFactWriteHeuristic(gateTestCandidate(), nil)
	if d.Operation != FactWriteOpAdd {
		t.Fatalf("op: got %q want add", d.Operation)
	}
}

func TestDecideFactWriteHeuristic_BelowMergeBand(t *testing.T) {
	neighbors := []MemoryConflictNeighbor{
		{FactID: "f1", Score: 0.79, FactKind: "preference"},
	}
	d := DecideFactWriteHeuristic(gateTestCandidate(), neighbors)
	if d.Operation != FactWriteOpAdd {
		t.Fatalf("op: got %q want add", d.Operation)
	}
}

func TestDecideFactWriteHeuristic_MergeBand_SameKind(t *testing.T) {
	neighbors := []MemoryConflictNeighbor{
		{FactID: "f1", Score: FactWriteMergeScore, FactKind: "preference"},
		{FactID: "f2", Score: 0.85, FactKind: "preference"},
	}
	d := DecideFactWriteHeuristic(gateTestCandidate(), neighbors)
	if d.Operation != FactWriteOpNoop {
		t.Fatalf("op: got %q want noop (merge)", d.Operation)
	}
	if d.TargetFactID != "f1" {
		t.Fatalf("merge target: got %q want f1", d.TargetFactID)
	}
}

func TestDecideFactWriteHeuristic_MergeBand_DifferentKindIsContested(t *testing.T) {
	// ≥0.92 but different kind → contested (LLM adjudicator territory);
	// heuristic fallback treats contested as ADD.
	c := gateTestCandidate()
	neighbors := []MemoryConflictNeighbor{
		{FactID: "f1", Score: 0.95, FactKind: "goal"},
	}
	d := DecideFactWriteHeuristic(c, neighbors)
	if d.Operation != FactWriteOpAdd {
		t.Fatalf("op: got %q want add (heuristic fallback for contested)", d.Operation)
	}
}

func TestDecideFactWriteHeuristic_ContestedBand(t *testing.T) {
	neighbors := []MemoryConflictNeighbor{
		{FactID: "f1", Score: 0.85, FactKind: "preference"},
	}
	d := DecideFactWriteHeuristic(gateTestCandidate(), neighbors)
	if d.Operation != FactWriteOpAdd {
		t.Fatalf("op: got %q want add (heuristic fallback for contested)", d.Operation)
	}
}

// --- Contested detection ---

func TestFactWriteIsContested(t *testing.T) {
	cases := []struct {
		name      string
		neighbors []MemoryConflictNeighbor
		want      bool
	}{
		{"none", nil, false},
		{"below band", []MemoryConflictNeighbor{{FactID: "f1", Score: 0.79, FactKind: "preference"}}, false},
		{"merge band same kind", []MemoryConflictNeighbor{{FactID: "f1", Score: 0.93, FactKind: "preference"}}, false},
		{"merge band diff kind", []MemoryConflictNeighbor{{FactID: "f1", Score: 0.93, FactKind: "goal"}}, true},
		{"ambiguous band", []MemoryConflictNeighbor{{FactID: "f1", Score: 0.85, FactKind: "preference"}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FactWriteIsContested("preference", tc.neighbors); got != tc.want {
				t.Fatalf("contested: got %v want %v", got, tc.want)
			}
		})
	}
}
