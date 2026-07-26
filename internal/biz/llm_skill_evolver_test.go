package biz

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeLLMCaller struct {
	got    LLMCallRequest
	text   string
	err    error
	called bool
}

func (f *fakeLLMCaller) Call(_ context.Context, req LLMCallRequest) (string, int, error) {
	f.called = true
	f.got = req
	if f.err != nil {
		return "", 0, f.err
	}
	return f.text, 100, nil
}

type fakeSkillLookupReader struct {
	body    string
	bodyErr error
}

func (f *fakeSkillLookupReader) GetSkillByID(context.Context, string) (Skill, error) {
	return Skill{}, errors.New("not implemented")
}
func (f *fakeSkillLookupReader) GetSkillBySkillKey(context.Context, string) (Skill, error) {
	return Skill{}, errors.New("not implemented")
}
func (f *fakeSkillLookupReader) GetSkillStorageDir(context.Context, string) (string, error) {
	return "", errors.New("not implemented")
}
func (f *fakeSkillLookupReader) GetLatestSkillMarkdown(context.Context, string) (string, error) {
	return f.body, f.bodyErr
}

// ── tests ────────────────────────────────────────────────────────────────────

func newTestEvolver(caller *fakeLLMCaller, skills *fakeSkillLookupReader) *LLMSkillEvolver {
	return NewLLMSkillEvolver(caller, skills, "openai", "gpt-4o", loggateway.NewNoop())
}

func TestLLMSkillEvolver_Success(t *testing.T) {
	caller := &fakeLLMCaller{text: "# Improved Skill\n\n做得更好。\n"}
	skills := &fakeSkillLookupReader{body: "# Old Skill\n\n旧内容。\n"}
	e := newTestEvolver(caller, skills)

	draft, err := e.EvolveDraft(context.Background(), SkillDraftInput{
		SkillID:       "sk-1",
		SuggestType:   EvoSuggestionFixFailure,
		TriggerReason: "7d 成功率 45% < 60%",
	})
	if err != nil {
		t.Fatalf("EvolveDraft error: %v", err)
	}
	if !strings.Contains(draft, "Improved Skill") {
		t.Fatalf("draft missing LLM content: %q", draft)
	}
	if !caller.called {
		t.Fatal("LLM caller not invoked")
	}
	// provider/model 透传
	if caller.got.Provider != "openai" || caller.got.Model != "gpt-4o" {
		t.Fatalf("provider/model not propagated: %+v", caller.got)
	}
	// prompt 必须包含触发原因与当前 body（反思者的现场证据）
	if !strings.Contains(caller.got.User, "7d 成功率 45%") {
		t.Fatalf("prompt missing trigger reason")
	}
	if !strings.Contains(caller.got.User, "旧内容") {
		t.Fatalf("prompt missing current skill body")
	}
}

func TestLLMSkillEvolver_LLMError(t *testing.T) {
	caller := &fakeLLMCaller{err: errors.New("llm unavailable")}
	skills := &fakeSkillLookupReader{body: "# Old\n"}
	e := newTestEvolver(caller, skills)

	_, err := e.EvolveDraft(context.Background(), SkillDraftInput{SkillID: "sk-1", TriggerReason: "r"})
	if err == nil {
		t.Fatal("expected error when LLM fails")
	}
}

func TestLLMSkillEvolver_BodyFetchError(t *testing.T) {
	caller := &fakeLLMCaller{text: "# X\n"}
	skills := &fakeSkillLookupReader{bodyErr: errors.New("db down")}
	e := newTestEvolver(caller, skills)

	_, err := e.EvolveDraft(context.Background(), SkillDraftInput{SkillID: "sk-1", TriggerReason: "r"})
	if err == nil {
		t.Fatal("expected error when current body unavailable")
	}
	if caller.called {
		t.Fatal("LLM must not be called when body fetch fails")
	}
}

func TestLLMSkillEvolver_OutputValidation(t *testing.T) {
	cases := []struct {
		name string
		text string
		ok   bool
	}{
		{"empty", "", false},
		{"blank", "   \n  ", false},
		{"no_heading", "只是一些建议文本，没有标题", false},
		{"too_long", "# T\n" + strings.Repeat("x", GateMaxDraftLength), false},
		{"valid", "# Valid\n\n内容。", true},
		{"fenced", "```markdown\n# Fenced\n\n内容。\n```", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &fakeLLMCaller{text: tc.text}
			skills := &fakeSkillLookupReader{body: "# Old\n"}
			e := newTestEvolver(caller, skills)
			draft, err := e.EvolveDraft(context.Background(), SkillDraftInput{SkillID: "sk-1", TriggerReason: "r"})
			if tc.ok && err != nil {
				t.Fatalf("expected success, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected validation error, got draft %q", draft)
			}
			if tc.ok && tc.name == "fenced" && strings.Contains(draft, "```") {
				t.Fatalf("fenced output not unwrapped: %q", draft)
			}
		})
	}
}

func TestGenerateDraft_FallbackChain(t *testing.T) {
	// evolver=nil → rule-based，llmGenerated=false
	uc := &SkillIntelligenceUsecase{lg: loggateway.NewNoop()}
	draft, llmGen := uc.generateDraft(context.Background(), &SkillEvolutionSuggestion{
		SkillID: "sk-1", Type: EvoSuggestionFixFailure, TriggerReason: "r",
	})
	if llmGen {
		t.Fatal("nil evolver must not report llmGenerated")
	}
	if !strings.Contains(draft, "fix_failure") {
		t.Fatalf("expected rule-based template, got %q", draft)
	}

	// evolver 失败 → 回退 rule-based
	uc2 := &SkillIntelligenceUsecase{
		lg:      loggateway.NewNoop(),
		evolver: &failingDraftEvolver{},
	}
	draft2, llmGen2 := uc2.generateDraft(context.Background(), &SkillEvolutionSuggestion{
		SkillID: "sk-1", Type: EvoSuggestionFixFailure, TriggerReason: "r",
	})
	if llmGen2 {
		t.Fatal("failing evolver must fall back with llmGenerated=false")
	}
	if !strings.Contains(draft2, "fix_failure") {
		t.Fatalf("expected rule-based fallback, got %q", draft2)
	}

	// evolver 成功 → llmGenerated=true
	uc3 := &SkillIntelligenceUsecase{
		lg:      loggateway.NewNoop(),
		evolver: &okDraftEvolver{text: "# LLM Draft\n"},
	}
	draft3, llmGen3 := uc3.generateDraft(context.Background(), &SkillEvolutionSuggestion{
		SkillID: "sk-1", Type: EvoSuggestionFixFailure, TriggerReason: "r",
	})
	if !llmGen3 {
		t.Fatal("successful evolver must report llmGenerated=true")
	}
	if !strings.Contains(draft3, "LLM Draft") {
		t.Fatalf("expected LLM draft, got %q", draft3)
	}
}

type failingDraftEvolver struct{}

func (f *failingDraftEvolver) EvolveDraft(context.Context, SkillDraftInput) (string, error) {
	return "", errors.New("llm down")
}

type okDraftEvolver struct{ text string }

func (f *okDraftEvolver) EvolveDraft(context.Context, SkillDraftInput) (string, error) {
	return f.text, nil
}
