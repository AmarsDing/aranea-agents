package biz

import (
	"context"
	"strings"
	"testing"
)

// Tests for PromptRefiner pure helpers (PGO-3-BIZ-05).

// ── EstimateTokenCount ────────────────────────────────────────────────────────

func TestEstimateTokenCount_Empty(t *testing.T) {
	if got := EstimateTokenCount(""); got != 0 {
		t.Errorf("empty string: want 0, got %d", got)
	}
}

func TestEstimateTokenCount_ShortEnglish(t *testing.T) {
	// "hello" = 5 chars → (5*10+24)/25 = 74/25 = 2 tokens
	got := EstimateTokenCount("hello")
	if got < 1 || got > 5 {
		t.Errorf("EstimateTokenCount(%q) = %d; expected 1-5", "hello", got)
	}
}

func TestEstimateTokenCount_LongText(t *testing.T) {
	text := strings.Repeat("你好世界", 100) // 400 CJK chars
	got := EstimateTokenCount(text)
	if got < 100 {
		t.Errorf("long CJK text (400 runes): expected >=100 tokens, got %d", got)
	}
}

// ── truncateAtLineBoundary ────────────────────────────────────────────────────

func TestTruncateAtLineBoundary_NoOp(t *testing.T) {
	s := "short text"
	got := truncateAtLineBoundary(s, 100)
	if got != s {
		t.Errorf("no-op truncation: want %q, got %q", s, got)
	}
}

func TestTruncateAtLineBoundary_AtNewline(t *testing.T) {
	s := "line one\nline two\nline three"
	got := truncateAtLineBoundary(s, 14) // 14 runes covers "line one\nline "
	if strings.Contains(got, "line three") {
		t.Errorf("should have truncated before line three, got:\n%s", got)
	}
	if !strings.Contains(got, "line one") {
		t.Errorf("should still contain line one, got:\n%s", got)
	}
}

func TestTruncateAtLineBoundary_NoNewline(t *testing.T) {
	s := "abcdefghijklmnopqrstuvwxyz"
	got := truncateAtLineBoundary(s, 10)
	// Should produce truncated + "…"
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated text without newlines should end with …, got %q", got)
	}
}

// ── UnifiedDiffSimple ─────────────────────────────────────────────────────────

func TestUnifiedDiffSimple_NoChange(t *testing.T) {
	diff := UnifiedDiffSimple("hello", "hello")
	if strings.Contains(diff, "+hello") || strings.Contains(diff, "-hello") {
		t.Errorf("identical strings should produce context-only diff, got:\n%s", diff)
	}
	if !strings.Contains(diff, " hello") {
		t.Errorf("context line expected, got:\n%s", diff)
	}
}

func TestUnifiedDiffSimple_Added(t *testing.T) {
	diff := UnifiedDiffSimple("line one", "line one\nline two")
	if !strings.Contains(diff, "+line two") {
		t.Errorf("added line not shown in diff:\n%s", diff)
	}
}

func TestUnifiedDiffSimple_Changed(t *testing.T) {
	diff := UnifiedDiffSimple("old text", "new text")
	if !strings.Contains(diff, "-old text") {
		t.Errorf("removed line not shown in diff:\n%s", diff)
	}
	if !strings.Contains(diff, "+new text") {
		t.Errorf("added line not shown in diff:\n%s", diff)
	}
}

// ── modeLabel ─────────────────────────────────────────────────────────────────

func TestModeLabel(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"complete", "complete（完整）"},
		{"", "complete（完整）"},
		{"task", "task（任务，需紧凑）"},
		{"minimized", "minimized（极简）"},
		{"unknown", "unknown"},
	}
	for _, tc := range cases {
		got := modeLabel(tc.in)
		if got != tc.want {
			t.Errorf("modeLabel(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// ── Agent.SkipCategoryResponsibility ─────────────────────────────────────────

func TestSkipCategoryResponsibility_EmptyJSON(t *testing.T) {
	a := Agent{}
	if a.SkipCategoryResponsibility() {
		t.Error("empty metadata should not skip")
	}
}

func TestSkipCategoryResponsibility_ExplicitTrue(t *testing.T) {
	a := Agent{MetadataJSON: `{"skip_category_responsibility":true}`}
	if !a.SkipCategoryResponsibility() {
		t.Error("explicit true should skip")
	}
}

func TestSkipCategoryResponsibility_ExplicitFalse(t *testing.T) {
	a := Agent{MetadataJSON: `{"skip_category_responsibility":false}`}
	if a.SkipCategoryResponsibility() {
		t.Error("explicit false should not skip")
	}
}

func TestSkipCategoryResponsibility_InvalidJSON(t *testing.T) {
	a := Agent{MetadataJSON: `{not valid json`}
	if a.SkipCategoryResponsibility() {
		t.Error("invalid JSON should default to false (don't skip)")
	}
}

func TestSkipCategoryResponsibility_MissingField(t *testing.T) {
	a := Agent{MetadataJSON: `{"other_field": "value"}`}
	if a.SkipCategoryResponsibility() {
		t.Error("missing field should default to false")
	}
}

// ── ScopeSpecExtract ─────────────────────────────────────────────────────────

type mockSystemSettingRepo struct{}

func (mockSystemSettingRepo) Get(context.Context) (SystemSetting, error) {
	return SystemSetting{
		DefaultRefineLLM: RefineLLMSetting{
			Provider: "openai",
			Model:    "gpt-test",
		},
	}, nil
}
func (mockSystemSettingRepo) Update(context.Context, string, string, int64, string, bool) (SystemSetting, error) {
	return SystemSetting{}, nil
}
func (mockSystemSettingRepo) UpdateKnowledgeEmbed(context.Context, KnowledgeEmbedSetting, bool) (KnowledgeEmbedSetting, error) {
	return KnowledgeEmbedSetting{}, nil
}
func (mockSystemSettingRepo) GetKnowledgeEmbed(context.Context) (KnowledgeEmbedSetting, error) {
	return KnowledgeEmbedSetting{}, nil
}
func (mockSystemSettingRepo) UpdateEvalLLM(context.Context, EvalLLMSetting) (EvalLLMSetting, error) {
	return EvalLLMSetting{}, nil
}
func (mockSystemSettingRepo) GetWebResearch(context.Context) (WebResearchSetting, error) {
	return WebResearchSetting{}, nil
}
func (mockSystemSettingRepo) UpdateWebResearch(context.Context, WebResearchSetting, bool) (WebResearchSetting, error) {
	return WebResearchSetting{}, nil
}
func (mockSystemSettingRepo) UpdateMemoryPlatform(context.Context, MemoryPlatformSetting) (MemoryPlatformSetting, error) {
	return MemoryPlatformSetting{}, nil
}
func (mockSystemSettingRepo) EnsureCredentialEncryptionKey(context.Context) (string, error) {
	return "", nil
}
func (mockSystemSettingRepo) GetRefineLLM(context.Context) (RefineLLMSetting, error) {
	return RefineLLMSetting{}, nil
}
func (mockSystemSettingRepo) UpdateRefineLLM(context.Context, RefineLLMSetting, bool) (RefineLLMSetting, error) {
	return RefineLLMSetting{}, nil
}

type mockLLMCaller struct {
	got LLMCallRequest
}

func (m *mockLLMCaller) Call(_ context.Context, req LLMCallRequest) (string, int, error) {
	m.got = req
	return "```yaml\nmeta:\n  version: \"1\"\nspec:\n  industries: []\n```", 42, nil
}

func TestPromptRefinerRefine_SpecExtract(t *testing.T) {
	llm := &mockLLMCaller{}
	refiner := NewPromptRefiner(nil, NewSystemSettingUsecase(mockSystemSettingRepo{}, nil), nil, llm)

	got, err := refiner.Refine(context.Background(), RefineRequest{
		Scope:        ScopeSpecExtract,
		OriginalText: "# 公司组织\n请抽取组织结构",
	})
	if err != nil {
		t.Fatalf("Refine(ScopeSpecExtract) error: %v", err)
	}
	if strings.Contains(got.Refined, "```") {
		t.Fatalf("expected code fence stripped, got:\n%s", got.Refined)
	}
	if !strings.Contains(got.Refined, "industries: []") {
		t.Fatalf("expected yaml body, got:\n%s", got.Refined)
	}
	if got.Provider != "openai" || got.Model != "gpt-test" || got.ModelSource != ModelSourceSystemDefault {
		t.Fatalf("unexpected model resolution: provider=%q model=%q source=%q", got.Provider, got.Model, got.ModelSource)
	}
	if llm.got.Provider != "openai" || llm.got.Model != "gpt-test" {
		t.Fatalf("LLM call did not receive resolved provider/model: %+v", llm.got)
	}
	if !strings.Contains(llm.got.System, "YAML") {
		t.Fatalf("expected spec_extract system prompt to mention YAML, got:\n%s", llm.got.System)
	}
}
