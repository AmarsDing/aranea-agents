package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// embedderAdminSpy records Update calls for assertion.
type embedderAdminSpy struct {
	mu          sync.Mutex
	updateCalls []updateCall
	snap        embedderSnapshot
}

type updateCall struct {
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
	Dim      int
}

type embedderSnapshot struct {
	provider   string
	baseURL    string
	apiKey     string
	model      string
	dim        int
	configured bool
	hasAPIKey  bool
}

func (s *embedderAdminSpy) Config() (provider, baseURL, model string, dim int, configured bool, hasAPIKey bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap.provider, s.snap.baseURL, s.snap.model, s.snap.dim, s.snap.configured, s.snap.hasAPIKey
}

func (s *embedderAdminSpy) Update(provider, baseURL, apiKey, model string, dim int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateCalls = append(s.updateCalls, updateCall{
		Provider: provider,
		BaseURL:  baseURL,
		APIKey:   apiKey,
		Model:    model,
		Dim:      dim,
	})
}

func (s *embedderAdminSpy) updateCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.updateCalls)
}

// sysSettingRepoStub controls GetKnowledgeEmbed / UpdateKnowledgeEmbed behavior.
type sysSettingRepoStub struct {
	getErr     error
	updateErr  error
	getCalled  int
	updCalled  int
	updPayload biz.KnowledgeEmbedSetting
}

func (r *sysSettingRepoStub) Get(context.Context) (biz.SystemSetting, error) {
	return biz.SystemSetting{}, nil
}
func (r *sysSettingRepoStub) Update(context.Context, string, string, int64, string, bool) (biz.SystemSetting, error) {
	return biz.SystemSetting{}, nil
}
func (r *sysSettingRepoStub) UpdateKnowledgeEmbed(_ context.Context, patch biz.KnowledgeEmbedSetting, _ bool) (biz.KnowledgeEmbedSetting, error) {
	r.updCalled++
	r.updPayload = patch
	if r.updateErr != nil {
		return biz.KnowledgeEmbedSetting{}, r.updateErr
	}
	return patch, nil
}
func (r *sysSettingRepoStub) GetKnowledgeEmbed(context.Context) (biz.KnowledgeEmbedSetting, error) {
	r.getCalled++
	if r.getErr != nil {
		return biz.KnowledgeEmbedSetting{}, r.getErr
	}
	return biz.KnowledgeEmbedSetting{Provider: "openai", Model: "text-embedding-3-small", Dim: 1536}, nil
}
func (r *sysSettingRepoStub) UpdateEvalLLM(context.Context, biz.EvalLLMSetting) (biz.EvalLLMSetting, error) {
	return biz.EvalLLMSetting{}, nil
}
func (r *sysSettingRepoStub) GetWebResearch(context.Context) (biz.WebResearchSetting, error) {
	return biz.WebResearchSetting{}, nil
}
func (r *sysSettingRepoStub) UpdateWebResearch(context.Context, biz.WebResearchSetting, bool) (biz.WebResearchSetting, error) {
	return biz.WebResearchSetting{}, nil
}
func (r *sysSettingRepoStub) UpdateMemoryPlatform(context.Context, biz.MemoryPlatformSetting) (biz.MemoryPlatformSetting, error) {
	return biz.MemoryPlatformSetting{}, nil
}
func (r *sysSettingRepoStub) EnsureCredentialEncryptionKey(context.Context) (string, error) {
	return "", nil
}
func (r *sysSettingRepoStub) GetRefineLLM(context.Context) (biz.RefineLLMSetting, error) {
	return biz.RefineLLMSetting{}, nil
}
func (r *sysSettingRepoStub) UpdateRefineLLM(context.Context, biz.RefineLLMSetting, bool) (biz.RefineLLMSetting, error) {
	return biz.RefineLLMSetting{}, nil
}
func (r *sysSettingRepoStub) GetPlannerModel(context.Context) (biz.PlannerModelSetting, error) {
	return biz.PlannerModelSetting{}, nil
}
func (r *sysSettingRepoStub) UpdatePlannerModel(context.Context, biz.PlannerModelSetting) (biz.PlannerModelSetting, error) {
	return biz.PlannerModelSetting{}, nil
}

// TestUpdateEmbedderConfig_PersistFailureDoesNotTouchMemory is a regression test
// for the audit finding "Embedder 先改内存再持久化，失败仍返回成功" (Domain 4 Claim 4).
//
// Previously the flow was:
//   1. embedderAdmin.Update(...)  — mutate in-memory state
//   2. PersistKnowledgeEmbed(...) — best-effort, log Warn on failure
//   3. return success
//
// After the fix, persistence must run first; on persist error we must NOT
// mutate in-memory state and must surface the error to the caller.
func TestUpdateEmbedderConfig_PersistFailureDoesNotTouchMemory(t *testing.T) {
	repo := &sysSettingRepoStub{
		updateErr: errors.New("db unreachable"),
	}
	spy := &embedderAdminSpy{
		snap: embedderSnapshot{
			provider: "openai", baseURL: "https://api.openai.com",
			model: "text-embedding-3-small", dim: 1536,
			configured: true, hasAPIKey: true,
		},
	}
	svc := &KnowledgeService{
		embedderAdmin: spy,
		systemSetting: repo,
		lg:            loggateway.NewNoop(),
	}

	req := &v1.UpdateEmbedderConfigRequest{
		Provider: "gemini",
		BaseUrl:  "https://generativelanguage.googleapis.com",
		ApiKey:   "AIza-test-key",
		Model:    "gemini-embedding-001",
		Dim:      768,
	}

	resp, err := svc.UpdateEmbedderConfig(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error when persistence fails, got nil (resp=%+v)", resp)
	}
	if resp != nil {
		t.Errorf("expected nil response on persist error, got %+v", resp)
	}
	if got := spy.updateCallCount(); got != 0 {
		t.Fatalf("embedderAdmin.Update must NOT be called when persist fails; got %d calls", got)
	}
	if repo.updCalled != 1 {
		t.Errorf("expected UpdateKnowledgeEmbed to be called once, got %d", repo.updCalled)
	}
}

// TestUpdateEmbedderConfig_PersistSuccessMutatesMemory covers the happy path:
// after persistence succeeds, the in-memory embedder must be updated with the
// exact request parameters so the next Embed call uses the new config.
func TestUpdateEmbedderConfig_PersistSuccessMutatesMemory(t *testing.T) {
	repo := &sysSettingRepoStub{}
	spy := &embedderAdminSpy{
		snap: embedderSnapshot{
			provider: "openai", baseURL: "https://api.openai.com",
			model: "text-embedding-3-small", dim: 1536,
			configured: true, hasAPIKey: true,
		},
	}
	svc := &KnowledgeService{
		embedderAdmin: spy,
		systemSetting: repo,
		lg:            loggateway.NewNoop(),
	}

	req := &v1.UpdateEmbedderConfigRequest{
		Provider: "gemini",
		BaseUrl:  "https://generativelanguage.googleapis.com",
		ApiKey:   "AIza-test-key",
		Model:    "gemini-embedding-001",
		Dim:      768,
	}

	resp, err := svc.UpdateEmbedderConfig(context.Background(), req)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if resp == nil || resp.Config == nil {
		t.Fatalf("expected non-nil response with config, got %+v", resp)
	}
	if got := spy.updateCallCount(); got != 1 {
		t.Fatalf("expected exactly 1 embedderAdmin.Update call, got %d", got)
	}
	call := spy.updateCalls[0]
	if call.Provider != "gemini" || call.Model != "gemini-embedding-001" || call.Dim != 768 {
		t.Errorf("Update called with wrong params: %+v", call)
	}
	if call.BaseURL != "https://generativelanguage.googleapis.com" {
		t.Errorf("BaseURL = %q, want gemini endpoint", call.BaseURL)
	}
	if call.APIKey != "AIza-test-key" {
		t.Errorf("APIKey = %q, want test key", call.APIKey)
	}
	if repo.updCalled != 1 {
		t.Errorf("expected UpdateKnowledgeEmbed to be called once, got %d", repo.updCalled)
	}
}

// TestUpdateEmbedderConfig_GetFailsPropagatesError covers the case where the
// initial GetKnowledgeEmbed (inside PersistKnowledgeEmbed) fails. The fix must
// propagate this error rather than silently swallowing it.
func TestUpdateEmbedderConfig_GetFailsPropagatesError(t *testing.T) {
	repo := &sysSettingRepoStub{
		getErr: errors.New("db read failed"),
	}
	spy := &embedderAdminSpy{
		snap: embedderSnapshot{provider: "openai", configured: true, hasAPIKey: true},
	}
	svc := &KnowledgeService{
		embedderAdmin: spy,
		systemSetting: repo,
		lg:            loggateway.NewNoop(),
	}

	req := &v1.UpdateEmbedderConfigRequest{
		Provider: "openai",
		ApiKey:   "sk-new",
		Model:    "text-embedding-3-small",
		Dim:      1536,
	}

	_, err := svc.UpdateEmbedderConfig(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error when GetKnowledgeEmbed fails, got nil")
	}
	if got := spy.updateCallCount(); got != 0 {
		t.Fatalf("embedderAdmin.Update must NOT be called when Get fails; got %d calls", got)
	}
}
