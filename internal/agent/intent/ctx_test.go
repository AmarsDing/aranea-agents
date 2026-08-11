package intent

import (
	"context"
	"testing"
)

func TestArtifactContext_RoundTrip(t *testing.T) {
	art := &Artifact{RefinedGoal: "做一个内部工具", IntentKind: "task"}
	ctx := WithArtifact(context.Background(), art)
	got := ArtifactFromContext(ctx)
	if got != art {
		t.Fatalf("ArtifactFromContext = %p, want %p", got, art)
	}
}

func TestArtifactContext_NilArtifact_NotStored(t *testing.T) {
	ctx := WithArtifact(context.Background(), nil)
	if got := ArtifactFromContext(ctx); got != nil {
		t.Fatalf("expected nil artifact, got %v", got)
	}
}

func TestArtifactFromContext_Missing(t *testing.T) {
	if got := ArtifactFromContext(context.Background()); got != nil {
		t.Fatalf("expected nil artifact from empty ctx, got %v", got)
	}
}

// C2：投机产物 ctx key 往返（与澄清续跑 key 隔离）。
func TestSpeculativeArtifactContext_RoundTrip(t *testing.T) {
	art := &Artifact{RefinedGoal: "今天天气怎么样", IntentKind: "question"}
	ctx := WithSpeculativeArtifact(context.Background(), art)
	if got := SpeculativeArtifactFromContext(ctx); got != art {
		t.Fatalf("SpeculativeArtifactFromContext = %p, want %p", got, art)
	}
	// 两个 key 互不串扰：投机 key 不影响澄清续跑 key。
	if got := ArtifactFromContext(ctx); got != nil {
		t.Fatalf("speculative ctx must not leak into resume key, got %v", got)
	}
}

func TestSpeculativeArtifactContext_NilArtifact_NotStored(t *testing.T) {
	ctx := WithSpeculativeArtifact(context.Background(), nil)
	if got := SpeculativeArtifactFromContext(ctx); got != nil {
		t.Fatalf("expected nil artifact, got %v", got)
	}
}

func TestSpeculativeArtifactFromContext_Missing(t *testing.T) {
	if got := SpeculativeArtifactFromContext(context.Background()); got != nil {
		t.Fatalf("expected nil artifact from empty ctx, got %v", got)
	}
}

func TestCloneWithoutClarification(t *testing.T) {
	art := &Artifact{
		RefinedGoal:     "做一个内部工具",
		IntentKind:      "task",
		Ambiguities:     []string{"平台不明"},
		RiskFlags:       []string{RiskFlagNeedsClarification, "other_flag"},
		Clarifications:  []ClarificationQuestion{{Question: "平台？"}},
		SuccessCriteria: []string{"可用"},
	}
	got := art.CloneWithoutClarification()
	if got == art {
		t.Fatal("expected a copy, not the same pointer")
	}
	if got.RefinedGoal != art.RefinedGoal || got.IntentKind != art.IntentKind {
		t.Errorf("core fields lost: %+v", got)
	}
	if len(got.SuccessCriteria) != 1 {
		t.Errorf("success criteria lost: %+v", got.SuccessCriteria)
	}
	if len(got.Clarifications) != 0 || len(got.Ambiguities) != 0 {
		t.Errorf("clarification residue: clarifications=%v ambiguities=%v", got.Clarifications, got.Ambiguities)
	}
	if got.HasRiskFlag(RiskFlagNeedsClarification) {
		t.Error("needs_clarification flag should be stripped")
	}
	if !got.HasRiskFlag("other_flag") {
		t.Error("unrelated risk flags should be preserved")
	}
	// 原产物不被修改
	if !art.HasRiskFlag(RiskFlagNeedsClarification) || len(art.Clarifications) != 1 {
		t.Error("source artifact mutated")
	}
}

func TestCloneWithoutClarification_Nil(t *testing.T) {
	var art *Artifact
	if got := art.CloneWithoutClarification(); got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestModelOverrideFromEnv(t *testing.T) {
	cases := []struct {
		name         string
		providerEnv  string
		modelEnv     string
		wantProvider string
		wantModel    string
	}{
		{"unset", "", "", "", ""},
		{"model_only", "", "gpt-4.1-mini", "", "gpt-4.1-mini"},
		{"provider_and_model", "openai", "gpt-4.1-mini", "openai", "gpt-4.1-mini"},
		{"provider_only_ignored", "openai", "", "", ""},
		{"whitespace_trimmed", " openai ", " gpt-4.1-mini ", "openai", "gpt-4.1-mini"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ARANEA_INTENT_PASS_PROVIDER", tc.providerEnv)
			t.Setenv("ARANEA_INTENT_PASS_MODEL", tc.modelEnv)
			p, m := ModelOverrideFromEnv()
			if p != tc.wantProvider || m != tc.wantModel {
				t.Fatalf("ModelOverrideFromEnv() = (%q, %q), want (%q, %q)", p, m, tc.wantProvider, tc.wantModel)
			}
		})
	}
}
