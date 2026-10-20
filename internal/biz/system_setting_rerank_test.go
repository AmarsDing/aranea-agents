package biz

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// U2-5: knowledge reranker settings persistence on SystemSettingUsecase —
// merge/redaction semantics over an in-memory fake port.

type rerankSettingFakeRepo struct {
	stored KnowledgeRerankSettings
	err    error
}

func (r *rerankSettingFakeRepo) Get(context.Context) (SystemSetting, error) {
	return SystemSetting{}, nil
}
func (r *rerankSettingFakeRepo) Update(context.Context, string, string, int64, string, bool) (SystemSetting, error) {
	return SystemSetting{}, nil
}
func (r *rerankSettingFakeRepo) UpdateKnowledgeEmbed(context.Context, KnowledgeEmbedSetting, bool) (KnowledgeEmbedSetting, error) {
	return KnowledgeEmbedSetting{}, nil
}
func (r *rerankSettingFakeRepo) GetKnowledgeEmbed(context.Context) (KnowledgeEmbedSetting, error) {
	return KnowledgeEmbedSetting{}, nil
}
func (r *rerankSettingFakeRepo) UpdateEvalLLM(context.Context, EvalLLMSetting) (EvalLLMSetting, error) {
	return EvalLLMSetting{}, nil
}
func (r *rerankSettingFakeRepo) GetWebResearch(context.Context) (WebResearchSetting, error) {
	return WebResearchSetting{}, nil
}
func (r *rerankSettingFakeRepo) UpdateWebResearch(context.Context, WebResearchSetting, bool) (WebResearchSetting, error) {
	return WebResearchSetting{}, nil
}
func (r *rerankSettingFakeRepo) UpdateMemoryPlatform(context.Context, MemoryPlatformSetting) (MemoryPlatformSetting, error) {
	return MemoryPlatformSetting{}, nil
}
func (r *rerankSettingFakeRepo) EnsureCredentialEncryptionKey(context.Context) (string, error) {
	return "", nil
}
func (r *rerankSettingFakeRepo) GetRefineLLM(context.Context) (RefineLLMSetting, error) {
	return RefineLLMSetting{}, nil
}
func (r *rerankSettingFakeRepo) UpdateRefineLLM(context.Context, RefineLLMSetting, bool) (RefineLLMSetting, error) {
	return RefineLLMSetting{}, nil
}
func (r *rerankSettingFakeRepo) GetPlannerModel(context.Context) (PlannerModelSetting, error) {
	return PlannerModelSetting{}, nil
}
func (r *rerankSettingFakeRepo) UpdatePlannerModel(context.Context, PlannerModelSetting) (PlannerModelSetting, error) {
	return PlannerModelSetting{}, nil
}
func (r *rerankSettingFakeRepo) GetSpeech(context.Context) (SpeechSetting, error) {
	return SpeechSetting{}, nil
}
func (r *rerankSettingFakeRepo) UpdateSpeech(context.Context, SpeechSetting, bool, bool) (SpeechSetting, error) {
	return SpeechSetting{}, nil
}

// KnowledgeRerankSettingRepo port (in-memory).
func (r *rerankSettingFakeRepo) GetKnowledgeRerank(context.Context) (KnowledgeRerankSettings, error) {
	if r.err != nil {
		return KnowledgeRerankSettings{}, r.err
	}
	return r.stored, nil
}
func (r *rerankSettingFakeRepo) UpdateKnowledgeRerank(_ context.Context, merged KnowledgeRerankSettings) (KnowledgeRerankSettings, error) {
	if r.err != nil {
		return KnowledgeRerankSettings{}, r.err
	}
	r.stored = merged
	return merged, nil
}

func TestGetKnowledgeRerank_RedactsAPIKey(t *testing.T) {
	repo := &rerankSettingFakeRepo{stored: KnowledgeRerankSettings{
		Provider: "cohere", Model: "rerank-v3.5", APIKey: "secret-key", Enabled: true,
	}}
	u := NewSystemSettingUsecase(repo, nil)
	got, err := u.GetKnowledgeRerank(context.Background())
	if err != nil {
		t.Fatalf("GetKnowledgeRerank: %v", err)
	}
	if got.APIKey != "" {
		t.Fatalf("APIKey must be redacted, got %q", got.APIKey)
	}
	if !got.HasAPIKey {
		t.Fatal("HasAPIKey must mark stored key presence")
	}
	if got.Provider != "cohere" || !got.Enabled {
		t.Fatalf("other fields must round-trip: %#v", got)
	}
}

func TestGetKnowledgeRerank_RepoWithoutPortErrors(t *testing.T) {
	// refineLLMStubRepo (system_setting_refine_llm_test.go) does not implement
	// KnowledgeRerankSettingRepo.
	u := NewSystemSettingUsecase(&refineLLMStubRepo{}, nil)
	if _, err := u.GetKnowledgeRerank(context.Background()); err == nil {
		t.Fatal("expected error when repo lacks the rerank port")
	}
	if _, err := u.UpdateKnowledgeRerank(context.Background(), KnowledgeRerankSettings{Enabled: true, Provider: "topk"}); err == nil {
		t.Fatal("expected error when repo lacks the rerank port")
	}
}

func TestUpdateKnowledgeRerank_PreservesKeyWhenPatchEmpty(t *testing.T) {
	repo := &rerankSettingFakeRepo{stored: KnowledgeRerankSettings{
		Provider: "cohere", Model: "old", APIKey: "keep-me", HasAPIKey: true, Enabled: true,
	}}
	u := NewSystemSettingUsecase(repo, nil)
	got, err := u.UpdateKnowledgeRerank(context.Background(), KnowledgeRerankSettings{
		Provider: " Cohere ", Model: "rerank-v3.5", BaseURL: "https://api.cohere.com/", Enabled: true,
	})
	if err != nil {
		t.Fatalf("UpdateKnowledgeRerank: %v", err)
	}
	// Returned view is redacted.
	if got.APIKey != "" || !got.HasAPIKey {
		t.Fatalf("returned view must be redacted with HasAPIKey set: %#v", got)
	}
	// Stored row keeps the key, normalizes provider, trims base URL slash.
	if repo.stored.APIKey != "keep-me" {
		t.Fatalf("stored key must be preserved, got %q", repo.stored.APIKey)
	}
	if repo.stored.Provider != "cohere" {
		t.Fatalf("provider must be normalized, got %q", repo.stored.Provider)
	}
	if repo.stored.BaseURL != "https://api.cohere.com" {
		t.Fatalf("base_url trailing slash must be trimmed, got %q", repo.stored.BaseURL)
	}
	if repo.stored.UpdatedAt == "" {
		t.Fatal("UpdatedAt must be stamped")
	}
	if _, terr := time.Parse(time.RFC3339, repo.stored.UpdatedAt); terr != nil {
		t.Fatalf("UpdatedAt must be RFC3339: %v", terr)
	}
}

func TestUpdateKnowledgeRerank_ReplacesKeyWhenPatchNonEmpty(t *testing.T) {
	repo := &rerankSettingFakeRepo{stored: KnowledgeRerankSettings{
		Provider: "cohere", APIKey: "old", HasAPIKey: true, Enabled: true,
	}}
	u := NewSystemSettingUsecase(repo, nil)
	if _, err := u.UpdateKnowledgeRerank(context.Background(), KnowledgeRerankSettings{
		Provider: "cohere", APIKey: " new-key ", Enabled: true,
	}); err != nil {
		t.Fatalf("UpdateKnowledgeRerank: %v", err)
	}
	if repo.stored.APIKey != "new-key" {
		t.Fatalf("stored key must be rotated+trimmed, got %q", repo.stored.APIKey)
	}
}

func TestUpdateKnowledgeRerank_Validation(t *testing.T) {
	repo := &rerankSettingFakeRepo{}
	u := NewSystemSettingUsecase(repo, nil)
	if _, err := u.UpdateKnowledgeRerank(context.Background(), KnowledgeRerankSettings{
		Provider: "not-a-provider",
	}); err == nil {
		t.Fatal("invalid provider must be rejected")
	}
	if _, err := u.UpdateKnowledgeRerank(context.Background(), KnowledgeRerankSettings{
		Enabled: true,
	}); err == nil {
		t.Fatal("enabled without provider must be rejected")
	}
	// Disabled without provider is a legitimate "turn it off" write.
	got, err := u.UpdateKnowledgeRerank(context.Background(), KnowledgeRerankSettings{Enabled: false})
	if err != nil {
		t.Fatalf("disable write must succeed: %v", err)
	}
	if got.Enabled {
		t.Fatal("disabled flag must persist")
	}
}

func TestUpdateKnowledgeRerank_PropagatesRepoError(t *testing.T) {
	repo := &rerankSettingFakeRepo{err: errors.New("db down")}
	u := NewSystemSettingUsecase(repo, nil)
	if _, err := u.GetKnowledgeRerank(context.Background()); err == nil {
		t.Fatal("get must propagate repo error")
	}
	if _, err := u.UpdateKnowledgeRerank(context.Background(), KnowledgeRerankSettings{Provider: "topk"}); err == nil {
		t.Fatal("update must propagate repo error")
	}
}

func TestApplyKnowledgeRerankPatch(t *testing.T) {
	t.Parallel()
	cur := KnowledgeRerankSettings{
		Provider: "cohere", Model: "old", BaseURL: "https://old.example.com",
		APIKey: "secret", HasAPIKey: true, Enabled: true,
	}
	out := ApplyKnowledgeRerankPatch(cur, KnowledgeRerankSettings{Model: "new-model"})
	if out.APIKey != "secret" || !out.HasAPIKey {
		t.Fatalf("empty patch key must preserve stored key: %#v", out)
	}
	if out.Provider != "cohere" {
		t.Fatalf("empty patch provider must preserve stored provider: %#v", out)
	}
	if out.BaseURL != "" {
		t.Fatalf("base_url follows embed proto3 semantics (empty clears): %#v", out)
	}
	if out.Model != "new-model" {
		t.Fatalf("model must be replaced: %#v", out)
	}
}

func TestKnowledgeRerankConfigJSON_RoundTrip(t *testing.T) {
	t.Parallel()
	in := KnowledgeRerankSettings{
		Provider: "infinity", Model: "bge-reranker-v2", BaseURL: "http://infinity.local/v1",
		APIKey: "stored-secret", Enabled: true, UpdatedAt: "2026-10-20T01:02:03Z",
	}
	raw, err := MarshalKnowledgeRerankConfig(in)
	if err != nil {
		t.Fatalf("MarshalKnowledgeRerankConfig: %v", err)
	}
	if strings.Contains(string(raw), "\n") {
		t.Fatalf("stored config must be single-line JSON, got %q", raw)
	}
	out, err := UnmarshalKnowledgeRerankConfig(raw)
	if err != nil {
		t.Fatalf("UnmarshalKnowledgeRerankConfig: %v", err)
	}
	if out.APIKey != "stored-secret" || !out.HasAPIKey {
		t.Fatalf("plaintext key must round-trip with HasAPIKey derived: %#v", out)
	}
	if out.Provider != in.Provider || out.Model != in.Model || out.BaseURL != in.BaseURL ||
		out.Enabled != in.Enabled || out.UpdatedAt != in.UpdatedAt {
		t.Fatalf("fields must round-trip: %#v", out)
	}
}

func TestUnmarshalKnowledgeRerankConfig_EmptyAndBad(t *testing.T) {
	t.Parallel()
	if got, err := UnmarshalKnowledgeRerankConfig(nil); err != nil || got != (KnowledgeRerankSettings{}) {
		t.Fatalf("empty input must yield zero settings, got %#v err=%v", got, err)
	}
	if _, err := UnmarshalKnowledgeRerankConfig([]byte("{not json")); err == nil {
		t.Fatal("bad JSON must error")
	}
}
