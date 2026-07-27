package biz

import (
	"context"
	"encoding/json"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// F8 (P-evo-3)：草稿来源必须可观测——evolver=nil 走模板时落
// draft_origin=rule_template，LLM 生成落 draft_origin=llm；
// 读取侧经 metadata 还原到建议视图，不再静默降级。

func TestGenerateDraftForSuggestion_PersistsDraftOrigin_RuleTemplate(t *testing.T) {
	store := newRecordingUnifiedStore()
	store.byID["sug-1"] = &UnifiedEvolutionSuggestion{
		ID: "sug-1", TargetType: EvolutionTargetSkill, TargetID: "sk-1",
		ActionType: EvolutionActionImprove, Status: "pending",
	}
	uc := NewSkillIntelligenceUsecase(nil, nil, store, nil, loggateway.NewNoop())

	if _, err := uc.GenerateDraftForSuggestion(context.Background(), "sug-1"); err != nil {
		t.Fatalf("GenerateDraftForSuggestion: %v", err)
	}
	if got := store.metaUpdates["sug-1"][EvoMetaDraftOrigin]; got != DraftOriginRuleTemplate {
		t.Fatalf("draft_origin = %q, want %q (evolver=nil must be observable, not silent)", got, DraftOriginRuleTemplate)
	}
}

func TestGenerateDraftForSuggestion_PersistsDraftOrigin_LLM(t *testing.T) {
	store := newRecordingUnifiedStore()
	store.byID["sug-2"] = &UnifiedEvolutionSuggestion{
		ID: "sug-2", TargetType: EvolutionTargetSkill, TargetID: "sk-1",
		ActionType: EvolutionActionImprove, Status: "pending",
	}
	uc := NewSkillIntelligenceUsecase(nil, nil, store, nil, loggateway.NewNoop(),
		SkillIntelligenceConfig{Evolver: &okDraftEvolver{text: "# LLM Draft\n"}})

	if _, err := uc.GenerateDraftForSuggestion(context.Background(), "sug-2"); err != nil {
		t.Fatalf("GenerateDraftForSuggestion: %v", err)
	}
	if got := store.metaUpdates["sug-2"][EvoMetaDraftOrigin]; got != DraftOriginLLM {
		t.Fatalf("draft_origin = %q, want %q", got, DraftOriginLLM)
	}
}

func TestUnifiedToLegacySuggestionPtr_MapsDraftOrigin(t *testing.T) {
	meta, _ := json.Marshal(map[string]string{EvoMetaDraftOrigin: DraftOriginRuleTemplate})
	u := &UnifiedEvolutionSuggestion{
		ID: "sug-3", TargetType: EvolutionTargetSkill, TargetID: "sk-1",
		ActionType: EvolutionActionImprove, Status: "pending",
		DraftBody: "# D\n", Metadata: meta,
	}
	got := unifiedToLegacySuggestionPtr(u)
	if got.DraftOrigin != DraftOriginRuleTemplate {
		t.Fatalf("DraftOrigin = %q, want %q", got.DraftOrigin, DraftOriginRuleTemplate)
	}
}
