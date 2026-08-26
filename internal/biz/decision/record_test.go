package decision

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func validRecord() Record {
	return Record{
		DecisionKey: "dk-1",
		Category:    CategoryHITLApproval,
		Scenario:    "高危工具 gns3_fault_inject 待审批",
		Outcome:     "approved",
		ActorType:   ActorHuman,
		ActorKey:    "user-1",
	}
}

func TestRecordValidate_OK(t *testing.T) {
	for _, cat := range []Category{
		CategoryHITLApproval, CategoryPlannerOrchestration, CategorySystemGuard,
		CategoryKnowledgeArbitration, CategoryEvolutionApplied,
	} {
		r := validRecord()
		r.Category = cat
		if err := r.Validate(); err != nil {
			t.Fatalf("category %s should validate: %v", cat, err)
		}
	}
}

func TestRecordValidate_Rejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Record)
	}{
		{"unknown category", func(r *Record) { r.Category = "bogus" }},
		{"empty category", func(r *Record) { r.Category = "" }},
		{"missing decision_key", func(r *Record) { r.DecisionKey = " " }},
		{"missing outcome", func(r *Record) { r.Outcome = "" }},
		{"unknown actor_type", func(r *Record) { r.ActorType = "robot" }},
		{"missing actor_key", func(r *Record) { r.ActorKey = "" }},
		{"missing scenario", func(r *Record) { r.Scenario = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validRecord()
			tc.mutate(&r)
			if err := r.Validate(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestRecordNormalize_Truncates(t *testing.T) {
	r := validRecord()
	r.Scenario = strings.Repeat("a", maxScenarioLen+100)
	r.Reasoning = strings.Repeat("b", maxReasoningLen+100)
	r.Outcome = strings.Repeat("c", maxOutcomeLen+10)
	r.ActorKey = strings.Repeat("d", maxActorKeyLen+10)
	r.Normalize()
	if len(r.Scenario) != maxScenarioLen {
		t.Fatalf("scenario len = %d, want %d", len(r.Scenario), maxScenarioLen)
	}
	if len(r.Reasoning) != maxReasoningLen {
		t.Fatalf("reasoning len = %d, want %d", len(r.Reasoning), maxReasoningLen)
	}
	if len(r.Outcome) != maxOutcomeLen {
		t.Fatalf("outcome len = %d, want %d", len(r.Outcome), maxOutcomeLen)
	}
	if len(r.ActorKey) != maxActorKeyLen {
		t.Fatalf("actor_key len = %d, want %d", len(r.ActorKey), maxActorKeyLen)
	}
}

func TestRecordNormalize_CJKSafe(t *testing.T) {
	r := validRecord()
	// 2001 CJK runes = 6003 UTF-8 bytes; byte-level truncation would split a
	// rune and produce invalid UTF-8.
	r.Scenario = strings.Repeat("决", maxScenarioLen+1)
	r.Normalize()
	if got := len([]rune(r.Scenario)); got != maxScenarioLen {
		t.Fatalf("scenario runes = %d, want %d", got, maxScenarioLen)
	}
	if !utf8.ValidString(r.Scenario) {
		t.Fatalf("scenario is not valid UTF-8 after truncation")
	}
}

func TestCodecRoundtrip(t *testing.T) {
	r := validRecord()
	conf := 0.87
	parent := int64(42)
	r.Confidence = &conf
	r.ParentDecisionID = &parent
	r.RelatedEntities = []EntityRef{{Type: "tool", Key: "gns3_fault_inject"}}
	r.SourceRef = SourceRef{RunID: "run-1", ToolInvocationID: "tc-1"}
	r.Metadata = map[string]any{"trigger_rule": "policy_danger"}
	r.WorkspaceID = "ws-1"
	r.CreatedAt = "2026-08-26T00:00:00Z"
	r.UpdatedAt = r.CreatedAt

	raw, err := encodeRecord(r)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	back, err := decodeRecord(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if back.DecisionKey != r.DecisionKey || back.Category != r.Category ||
		back.ActorType != r.ActorType || back.ActorKey != r.ActorKey {
		t.Fatalf("roundtrip identity mismatch: %+v", back)
	}
	if back.Confidence == nil || *back.Confidence != conf {
		t.Fatalf("confidence lost: %+v", back.Confidence)
	}
	if back.ParentDecisionID == nil || *back.ParentDecisionID != parent {
		t.Fatalf("parent lost: %+v", back.ParentDecisionID)
	}
	if len(back.RelatedEntities) != 1 || back.RelatedEntities[0].Key != "gns3_fault_inject" {
		t.Fatalf("entities lost: %+v", back.RelatedEntities)
	}
	if back.SourceRef.ToolInvocationID != "tc-1" {
		t.Fatalf("source_ref lost: %+v", back.SourceRef)
	}
	if back.Metadata["trigger_rule"] != "policy_danger" {
		t.Fatalf("metadata lost: %+v", back.Metadata)
	}
}

func TestDecodeRecord_Poison(t *testing.T) {
	if _, err := decodeRecord([]byte("{not json")); err == nil {
		t.Fatalf("expected decode error for poison payload")
	}
}
