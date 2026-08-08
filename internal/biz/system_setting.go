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
	// PlannerModel controls how plan_and_execute planner/allocator resolve
	// their internal LLM model. Replaces ARANEA_PLANNER_PROVIDER/MODEL env vars.
	PlannerModel PlannerModelSetting
	// Speech holds the voice companion (M74) ASR/TTS provider configuration
	// (V2-T7). Empty fields fall back to SPEECH_* env vars at read time.
	Speech     SpeechSetting
	UpdateTime time.Time
}

// RefineLLMSetting holds the platform default provider/model for AI refinement.
type RefineLLMSetting struct {
	Provider string
	Model    string
	BaseURL  string
	APIKey   string
	// HasAPIKey marks a fallback key is stored; populated on redacted reads
	// (API edge exposes only this marker, never the key value).
	HasAPIKey bool
}

// PlannerModelMode controls how the plan_and_execute planner/allocator resolve
// their internal LLM model.
const (
	// PlannerModelModeSpecify uses the admin-specified provider+model from
	// system settings. If that model is unavailable, falls back to inherit.
	PlannerModelModeSpecify = "specify"
	// PlannerModelModeInherit uses the session's selected provider/model
	// (the effective model driving the current Spirit agent turn).
	PlannerModelModeInherit = "inherit"
)

// PlannerModelSetting holds the plan_and_execute planner model configuration.
type PlannerModelSetting struct {
	Mode     string // "specify" or "inherit" (default)
	Provider string // used when Mode == specify
	Model    string // used when Mode == specify
}

// SystemSettingRepo is the repository interface for the singleton system
// setting row and its sub-settings (knowledge embed, eval LLM, web research,
// memory platform, refine LLM).
//
// Stability:evolving
// TECH-DEBT(DB-DEBT-02): This interface has 13 methods, exceeding the ≤5
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
	// PlannerModel: plan_and_execute planner/allocator model resolution config.
	GetPlannerModel(ctx context.Context) (PlannerModelSetting, error)
	UpdatePlannerModel(ctx context.Context, patch PlannerModelSetting) (PlannerModelSetting, error)
	// Speech: voice companion (M74) ASR/TTS provider config (V2-T7).
	GetSpeech(ctx context.Context) (SpeechSetting, error)
	UpdateSpeech(ctx context.Context, patch SpeechSetting, updateASRCred, updateTTSCred bool) (SpeechSetting, error)
}

// SystemSettingTxProvider provides transactional execution for atomic
// multi-step system setting updates. Implemented by *data.Data.
type SystemSettingTxProvider interface {
	ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type SystemSettingUsecase struct {
	repo              SystemSettingRepo
	quota             UsageQuotaRepo
	webResearchTester WebResearchTester
	txProvider        SystemSettingTxProvider
}

func NewSystemSettingUsecase(repo SystemSettingRepo, quota UsageQuotaRepo) *SystemSettingUsecase {
	return &SystemSettingUsecase{repo: repo, quota: quota}
}

func (u *SystemSettingUsecase) SetWebResearchTester(tester WebResearchTester) {
	u.webResearchTester = tester
}

func (u *SystemSettingUsecase) SetTxProvider(tp SystemSettingTxProvider) {
	u.txProvider = tp
}

// SystemSettingAllPatch encapsulates all optional sub-setting updates for
// atomic persistence via UpdateAll. Sub-settings are applied only when their
// pointer field is non-nil.
type SystemSettingAllPatch struct {
	// Core settings (always applied)
	RootDir               string
	WorkDir               string
	GlobalMonthlyMicroUSD int64
	A2APublicBaseURL      string
	MCPAllowAdHocHTTP     bool

	// Optional sub-settings (applied when non-nil)
	KnowledgeEmbed          *KnowledgeEmbedSetting
	KnowledgeEmbedUpdateKey bool
	EvalLLM                 *EvalLLMSetting
	WebResearch             *WebResearchSetting
	WebResearchUpdateKey    bool
	Speech                  *SpeechSetting
	SpeechUpdateASRCred     bool
	SpeechUpdateTTSCred     bool
	RefineLLM               *RefineLLMSetting
	RefineLLMUpdateKey      bool
}

// UpdateAll atomically persists core settings and any optional sub-settings
// in a single transaction when txProvider is set. Falls back to sequential
// non-transactional updates for backward compatibility.
func (u *SystemSettingUsecase) UpdateAll(ctx context.Context, p SystemSettingAllPatch) (SystemSetting, error) {
	if u.txProvider != nil {
		var result SystemSetting
		err := u.txProvider.ExecInTx(ctx, func(txCtx context.Context) error {
			row, err := u.Update(txCtx, p.RootDir, p.WorkDir, p.GlobalMonthlyMicroUSD, p.A2APublicBaseURL, p.MCPAllowAdHocHTTP)
			if err != nil {
				return err
			}
			if p.KnowledgeEmbed != nil {
				embed, err := u.UpdateKnowledgeEmbed(txCtx, *p.KnowledgeEmbed, p.KnowledgeEmbedUpdateKey)
				if err != nil {
					return err
				}
				row.KnowledgeEmbed = embed
			}
			if p.EvalLLM != nil {
				evalLLM, err := u.UpdateEvalLLM(txCtx, *p.EvalLLM)
				if err != nil {
					return err
				}
				row.EvalLLM = evalLLM
			}
			if p.WebResearch != nil {
				web, err := u.UpdateWebResearch(txCtx, *p.WebResearch, p.WebResearchUpdateKey)
				if err != nil {
					return err
				}
				row.WebResearch = web
			}
			if p.Speech != nil {
				speech, err := u.UpdateSpeech(txCtx, *p.Speech, p.SpeechUpdateASRCred, p.SpeechUpdateTTSCred)
				if err != nil {
					return err
				}
				row.Speech = speech
			}
			if p.RefineLLM != nil {
				refine, err := u.UpdateRefineLLM(txCtx, *p.RefineLLM, p.RefineLLMUpdateKey)
				if err != nil {
					return err
				}
				row.DefaultRefineLLM = refine
			}
			result = row
			return nil
		})
		if err != nil {
			return SystemSetting{}, err
		}
		return result, nil
	}
	// Legacy non-transactional path (backward compatibility).
	row, err := u.Update(ctx, p.RootDir, p.WorkDir, p.GlobalMonthlyMicroUSD, p.A2APublicBaseURL, p.MCPAllowAdHocHTTP)
	if err != nil {
		return SystemSetting{}, err
	}
	if p.KnowledgeEmbed != nil {
		embed, err := u.UpdateKnowledgeEmbed(ctx, *p.KnowledgeEmbed, p.KnowledgeEmbedUpdateKey)
		if err != nil {
			return SystemSetting{}, err
		}
		row.KnowledgeEmbed = embed
	}
	if p.EvalLLM != nil {
		evalLLM, err := u.UpdateEvalLLM(ctx, *p.EvalLLM)
		if err != nil {
			return SystemSetting{}, err
		}
		row.EvalLLM = evalLLM
	}
	if p.WebResearch != nil {
		web, err := u.UpdateWebResearch(ctx, *p.WebResearch, p.WebResearchUpdateKey)
		if err != nil {
			return SystemSetting{}, err
		}
		row.WebResearch = web
	}
	if p.Speech != nil {
		speech, err := u.UpdateSpeech(ctx, *p.Speech, p.SpeechUpdateASRCred, p.SpeechUpdateTTSCred)
		if err != nil {
			return SystemSetting{}, err
		}
		row.Speech = speech
	}
	if p.RefineLLM != nil {
		refine, err := u.UpdateRefineLLM(ctx, *p.RefineLLM, p.RefineLLMUpdateKey)
		if err != nil {
			return SystemSetting{}, err
		}
		row.DefaultRefineLLM = refine
	}
	return row, nil
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

// GetKnowledgeEmbed returns stored knowledge embedder defaults.
// NOTE: API key is returned in plaintext for internal embedder construction.
// Use embedderAdmin.Config() (which only exposes HasAPIKey) for API responses.
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

// GetPlannerModel returns the plan_and_execute planner model configuration.
func (u *SystemSettingUsecase) GetPlannerModel(ctx context.Context) (PlannerModelSetting, error) {
	return u.repo.GetPlannerModel(ctx)
}

// UpdatePlannerModel persists the plan_and_execute planner model configuration.
func (u *SystemSettingUsecase) UpdatePlannerModel(ctx context.Context, patch PlannerModelSetting) (PlannerModelSetting, error) {
	mode := strings.TrimSpace(patch.Mode)
	if mode == "" {
		mode = PlannerModelModeInherit
	}
	if mode != PlannerModelModeSpecify && mode != PlannerModelModeInherit {
		return PlannerModelSetting{}, apierror.BadRequest("SYSTEM_SETTING", "planner_model_mode must be 'specify' or 'inherit'")
	}
	return u.repo.UpdatePlannerModel(ctx, PlannerModelSetting{
		Mode:     mode,
		Provider: strings.TrimSpace(patch.Provider),
		Model:    strings.TrimSpace(patch.Model),
	})
}

// GetSpeech returns the stored voice companion ASR/TTS provider configuration
// (raw stored values; empty fields fall back to SPEECH_* env at read time).
func (u *SystemSettingUsecase) GetSpeech(ctx context.Context) (SpeechSetting, error) {
	return u.repo.GetSpeech(ctx)
}

// UpdateSpeech persists voice companion speech settings (M74 V2-T7). Empty
// patch fields preserve current stored values; credentials are replaced only
// when the matching updateXxxCred flag is set and the value is non-empty.
func (u *SystemSettingUsecase) UpdateSpeech(ctx context.Context, patch SpeechSetting, updateASRCred, updateTTSCred bool) (SpeechSetting, error) {
	if patch.TTS.SpeedRatio < 0 {
		return SpeechSetting{}, apierror.BadRequest("SYSTEM_SETTING", "speech_tts_speed_ratio must be > 0")
	}
	cur, err := u.repo.GetSpeech(ctx)
	if err != nil {
		return SpeechSetting{}, err
	}
	merged := ApplySpeechPatch(cur, patch, updateASRCred, updateTTSCred)
	return u.repo.UpdateSpeech(ctx, merged, updateASRCred, updateTTSCred)
}
