package biz

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"
)

// SystemSetting is the singleton platform configuration row.
type SystemSetting struct {
	RootDirectory                     string
	WorkDirectory                     string
	GlobalMonthlyMicroUSD             int64
	A2APublicBaseURL                  string
	CredentialEncryptionKeyConfigured bool
	KnowledgeEmbed                    KnowledgeEmbedSetting
	EvalLLM                           EvalLLMSetting
	WebResearch                       WebResearchSetting
	MCPAllowAdHocHTTP                 bool
	MemoryPlatform                    MemoryPlatformSetting
	// DefaultRefineLLM is the platform-level LLM config used by PromptRefiner
	// when the target agent doesn't have a model configured. PGO-3.
	DefaultRefineLLM RefineLLMSetting
	UpdateTime       time.Time
}

// RefineLLMSetting holds the platform default provider/model for AI refinement.
type RefineLLMSetting struct {
	Provider string
	Model    string
	BaseURL  string
	APIKey   string
}

// SystemSettingRepo is the repository interface for the singleton system
// setting row and its sub-settings (knowledge embed, eval LLM, web research,
// memory platform, refine LLM).
//
// Stability:evolving
// TECH-DEBT(DB-DEBT-02): This interface has 11 methods, exceeding the ≤5
// guideline (BI1/BI6). It should be split by domain into smaller interfaces
// (e.g., SystemSettingCoreRepo, KnowledgeEmbedSettingRepo,
// WebResearchSettingRepo, RefineLLMSettingRepo). Deferred because the split
// would touch Wire bindings, all callers, and test stubs — track for a
// dedicated refactoring iteration.
type SystemSettingRepo interface {
	Get(ctx context.Context) (SystemSetting, error)
	Update(ctx context.Context, rootDir, workDir string, globalMonthlyMicroUSD int64, a2aPublicBaseURL string, mcpAllowAdHocHTTP bool) (SystemSetting, error)
	UpdateKnowledgeEmbed(ctx context.Context, patch KnowledgeEmbedSetting, updateAPIKey bool) (KnowledgeEmbedSetting, error)
	GetKnowledgeEmbed(ctx context.Context) (KnowledgeEmbedSetting, error)
	UpdateEvalLLM(ctx context.Context, patch EvalLLMSetting) (EvalLLMSetting, error)
	GetWebResearch(ctx context.Context) (WebResearchSetting, error)
	UpdateWebResearch(ctx context.Context, patch WebResearchSetting, updateAPIKey bool) (WebResearchSetting, error)
	UpdateMemoryPlatform(ctx context.Context, patch MemoryPlatformSetting) (MemoryPlatformSetting, error)
	EnsureCredentialEncryptionKey(ctx context.Context) (string, error)
	// PGO-3: platform default LLM for AI refinement.
	GetRefineLLM(ctx context.Context) (RefineLLMSetting, error)
	UpdateRefineLLM(ctx context.Context, patch RefineLLMSetting, updateAPIKey bool) (RefineLLMSetting, error)
}

type SystemSettingUsecase struct {
	repo              SystemSettingRepo
	quota             UsageQuotaRepo
	webResearchTester WebResearchTester
}

func NewSystemSettingUsecase(repo SystemSettingRepo, quota UsageQuotaRepo) *SystemSettingUsecase {
	return &SystemSettingUsecase{repo: repo, quota: quota}
}

func (u *SystemSettingUsecase) SetWebResearchTester(tester WebResearchTester) {
	u.webResearchTester = tester
}

func (u *SystemSettingUsecase) Get(ctx context.Context) (SystemSetting, error) {
	s, err := u.repo.Get(ctx)
	if err != nil {
		return SystemSetting{}, err
	}
	if s.GlobalMonthlyMicroUSD <= 0 && u.quota != nil {
		q, qerr := u.quota.GetQuota(ctx, QuotaScopeGlobal, GlobalQuotaScopeID)
		if qerr == nil && q.MonthlyMicroUSD > 0 {
			s.GlobalMonthlyMicroUSD = q.MonthlyMicroUSD
		}
	}
	return s, nil
}

func (u *SystemSettingUsecase) Update(ctx context.Context, rootDir, workDir string, globalMonthlyMicroUSD int64, a2aPublicBaseURL string, mcpAllowAdHocHTTP bool) (SystemSetting, error) {
	if globalMonthlyMicroUSD < 0 {
		return SystemSetting{}, apierror.BadRequest("SYSTEM_SETTING", "global_monthly_micro_usd must be >= 0")
	}
	a2aPublicBaseURL = strings.TrimRight(strings.TrimSpace(a2aPublicBaseURL), "/")
	if a2aPublicBaseURL != "" && !strings.HasPrefix(a2aPublicBaseURL, "http://") && !strings.HasPrefix(a2aPublicBaseURL, "https://") {
		return SystemSetting{}, apierror.BadRequest("SYSTEM_SETTING", "a2a_public_base_url must start with http:// or https://")
	}
	s, err := u.repo.Update(ctx, rootDir, workDir, globalMonthlyMicroUSD, a2aPublicBaseURL, mcpAllowAdHocHTTP)
	if err != nil {
		return SystemSetting{}, err
	}
	if err := u.syncGlobalQuota(ctx, globalMonthlyMicroUSD); err != nil {
		return SystemSetting{}, err
	}
	s.GlobalMonthlyMicroUSD = globalMonthlyMicroUSD
	return s, nil
}

func (u *SystemSettingUsecase) syncGlobalQuota(ctx context.Context, monthlyMicroUSD int64) error {
	if u.quota == nil {
		return nil
	}
	_, err := u.quota.SetQuota(ctx, UsageQuota{
		ScopeType:       QuotaScopeGlobal,
		ScopeID:         GlobalQuotaScopeID,
		MonthlyMicroUSD: monthlyMicroUSD,
	})
	return MapUsageRepoErr(err)
}

// UpdateKnowledgeEmbed persists knowledge embedder defaults on the singleton row.
func (u *SystemSettingUsecase) UpdateKnowledgeEmbed(ctx context.Context, patch KnowledgeEmbedSetting, updateAPIKey bool) (KnowledgeEmbedSetting, error) {
	cur, err := u.repo.GetKnowledgeEmbed(ctx)
	if err != nil {
		return KnowledgeEmbedSetting{}, err
	}
	merged := ApplyKnowledgeEmbedPatch(cur, patch.Provider, patch.BaseURL, patch.APIKey, patch.Model, patch.Dim, updateAPIKey)
	return u.repo.UpdateKnowledgeEmbed(ctx, merged, updateAPIKey)
}

// GetKnowledgeEmbed returns stored knowledge embedder defaults (API key redacted).
func (u *SystemSettingUsecase) GetKnowledgeEmbed(ctx context.Context) (KnowledgeEmbedSetting, error) {
	return u.repo.GetKnowledgeEmbed(ctx)
}

// UpdateEvalLLM persists evaluation UserSim / LLM-as-Judge model defaults.
// Empty fields in the patch are preserved from the current value to avoid
// proto3 zero-value clobbering when the caller only intends to patch a subset.
func (u *SystemSettingUsecase) UpdateEvalLLM(ctx context.Context, patch EvalLLMSetting) (EvalLLMSetting, error) {
	cur, err := u.repo.Get(ctx)
	if err != nil {
		return EvalLLMSetting{}, err
	}
	merged := EvalLLMSetting{
		SimProvider:   firstNonEmpty(strings.TrimSpace(patch.SimProvider), cur.EvalLLM.SimProvider),
		SimModel:      firstNonEmpty(strings.TrimSpace(patch.SimModel), cur.EvalLLM.SimModel),
		JudgeProvider: firstNonEmpty(strings.TrimSpace(patch.JudgeProvider), cur.EvalLLM.JudgeProvider),
		JudgeModel:    firstNonEmpty(strings.TrimSpace(patch.JudgeModel), cur.EvalLLM.JudgeModel),
	}
	return u.repo.UpdateEvalLLM(ctx, merged)
}

// UpdateWebResearch persists Tavily/SerpAPI defaults for web_research.
func (u *SystemSettingUsecase) UpdateWebResearch(ctx context.Context, patch WebResearchSetting, updateAPIKey bool) (WebResearchSetting, error) {
	cur, err := u.repo.GetWebResearch(ctx)
	if err != nil {
		return WebResearchSetting{}, err
	}
	merged := ApplyWebResearchPatch(cur, patch, updateAPIKey)
	return u.repo.UpdateWebResearch(ctx, merged, updateAPIKey)
}

// UpdateMemoryPlatform persists memory worker / policy platform toggles.
func (u *SystemSettingUsecase) UpdateMemoryPlatform(ctx context.Context, patch MemoryPlatformSetting) (MemoryPlatformSetting, error) {
	return u.repo.UpdateMemoryPlatform(ctx, patch)
}

// GetRefineLLM returns the stored platform default LLM for AI refinement (API key redacted).
func (u *SystemSettingUsecase) GetRefineLLM(ctx context.Context) (RefineLLMSetting, error) {
	return u.repo.GetRefineLLM(ctx)
}

// UpdateRefineLLM persists the platform default LLM for AI refinement (PGO-3).
func (u *SystemSettingUsecase) UpdateRefineLLM(ctx context.Context, patch RefineLLMSetting, updateAPIKey bool) (RefineLLMSetting, error) {
	return u.repo.UpdateRefineLLM(ctx, RefineLLMSetting{
		Provider: strings.TrimSpace(patch.Provider),
		Model:    strings.TrimSpace(patch.Model),
		BaseURL:  strings.TrimSpace(patch.BaseURL),
		APIKey:   strings.TrimSpace(patch.APIKey),
	}, updateAPIKey)
}
