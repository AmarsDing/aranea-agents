package biz

import (
	"context"
	"testing"
)

// PGO-3-PROTO-02: UpdateAll 必须把 RefineLLM 补丁接入持久化路径，
// 并透传 updateAPIKey 语义（空 key 不轮换存值）。

type refineLLMStubRepo struct {
	refinePatch     RefineLLMSetting
	refineKeyUpdate bool
	refineCalled    bool
}

func (r *refineLLMStubRepo) Get(context.Context) (SystemSetting, error) { return SystemSetting{}, nil }
func (r *refineLLMStubRepo) Update(context.Context, string, string, int64, string, bool) (SystemSetting, error) {
	return SystemSetting{}, nil
}
func (r *refineLLMStubRepo) UpdateKnowledgeEmbed(context.Context, KnowledgeEmbedSetting, bool) (KnowledgeEmbedSetting, error) {
	return KnowledgeEmbedSetting{}, nil
}
func (r *refineLLMStubRepo) GetKnowledgeEmbed(context.Context) (KnowledgeEmbedSetting, error) {
	return KnowledgeEmbedSetting{}, nil
}
func (r *refineLLMStubRepo) UpdateEvalLLM(context.Context, EvalLLMSetting) (EvalLLMSetting, error) {
	return EvalLLMSetting{}, nil
}
func (r *refineLLMStubRepo) GetWebResearch(context.Context) (WebResearchSetting, error) {
	return WebResearchSetting{}, nil
}
func (r *refineLLMStubRepo) UpdateWebResearch(context.Context, WebResearchSetting, bool) (WebResearchSetting, error) {
	return WebResearchSetting{}, nil
}
func (r *refineLLMStubRepo) UpdateMemoryPlatform(context.Context, MemoryPlatformSetting) (MemoryPlatformSetting, error) {
	return MemoryPlatformSetting{}, nil
}
func (r *refineLLMStubRepo) EnsureCredentialEncryptionKey(context.Context) (string, error) {
	return "", nil
}
func (r *refineLLMStubRepo) GetRefineLLM(context.Context) (RefineLLMSetting, error) {
	return RefineLLMSetting{}, nil
}
func (r *refineLLMStubRepo) UpdateRefineLLM(_ context.Context, patch RefineLLMSetting, updateAPIKey bool) (RefineLLMSetting, error) {
	r.refineCalled = true
	r.refinePatch = patch
	r.refineKeyUpdate = updateAPIKey
	return patch, nil
}
func (r *refineLLMStubRepo) GetPlannerModel(context.Context) (PlannerModelSetting, error) {
	return PlannerModelSetting{}, nil
}
func (r *refineLLMStubRepo) UpdatePlannerModel(context.Context, PlannerModelSetting) (PlannerModelSetting, error) {
	return PlannerModelSetting{}, nil
}
func (r *refineLLMStubRepo) GetSpeech(context.Context) (SpeechSetting, error) {
	return SpeechSetting{}, nil
}
func (r *refineLLMStubRepo) UpdateSpeech(context.Context, SpeechSetting, bool, bool) (SpeechSetting, error) {
	return SpeechSetting{}, nil
}

func TestUpdateAll_RefineLLMPatchApplied(t *testing.T) {
	repo := &refineLLMStubRepo{}
	u := NewSystemSettingUsecase(repo, nil)
	_, err := u.UpdateAll(context.Background(), SystemSettingAllPatch{
		RefineLLM: &RefineLLMSetting{
			Provider: " deepseek ",
			Model:    "deepseek-v4-flash",
			APIKey:   " sk-x ",
		},
		RefineLLMUpdateKey: true,
	})
	if err != nil {
		t.Fatalf("UpdateAll: %v", err)
	}
	if !repo.refineCalled {
		t.Fatal("UpdateRefineLLM must be called when patch non-nil")
	}
	if repo.refinePatch.Provider != "deepseek" || repo.refinePatch.APIKey != "sk-x" {
		t.Fatalf("patch must be trimmed before persist: %#v", repo.refinePatch)
	}
	if !repo.refineKeyUpdate {
		t.Fatal("updateAPIKey flag must pass through")
	}
}

func TestUpdateAll_RefineLLMNilSkips(t *testing.T) {
	repo := &refineLLMStubRepo{}
	u := NewSystemSettingUsecase(repo, nil)
	if _, err := u.UpdateAll(context.Background(), SystemSettingAllPatch{}); err != nil {
		t.Fatalf("UpdateAll: %v", err)
	}
	if repo.refineCalled {
		t.Fatal("nil patch must not touch UpdateRefineLLM")
	}
}
