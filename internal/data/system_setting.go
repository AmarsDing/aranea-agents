package data

import (
	"context"
	"database/sql"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/apierror"
)

type systemSettingRepo struct {
	data *Data
}

var _ biz.SystemSettingRepo = (*systemSettingRepo)(nil)

// NewSystemSettingRepo implements biz.SystemSettingRepo and registers DB-backed credential key resolution.
func NewSystemSettingRepo(d *Data) biz.SystemSettingRepo {
	repo := &systemSettingRepo{data: d}
	return repo
}

func entToBizSystemSetting(e *ent.SystemSetting) biz.SystemSetting {
	if e == nil {
		return biz.SystemSetting{}
	}
	return biz.SystemSetting{
		RootDirectory:                     e.RootDirectory,
		WorkDirectory:                     e.WorkDirectory,
		GlobalMonthlyMicroUSD:             e.GlobalMonthlyMicroUsd,
		A2APublicBaseURL:                  e.A2aPublicBaseURL,
		CredentialEncryptionKeyConfigured: strings.TrimSpace(e.CredentialEncryptionKey) != "",
		KnowledgeEmbed:                    entToKnowledgeEmbed(e),
		EvalLLM:                           entToEvalLLM(e),
		WebResearch:                       entToWebResearch(e),
		MCPAllowAdHocHTTP:                 e.McpAllowAdhocHTTP,
		MemoryPlatform: biz.MemoryPlatformSetting{
			PolicyStrict:            e.MemoryPolicyStrict,
			EpisodeBackfillDisabled: e.MemoryEpisodeBackfillDisabled,
		},
		// DefaultRefineLLM is not loaded from entToBizSystemSetting because
		// it's stored via raw SQL patches — use GetRefineLLM for full data.
		UpdateTime: e.UpdateTime,
	}
}

func entToKnowledgeEmbed(e *ent.SystemSetting) biz.KnowledgeEmbedSetting {
	if e == nil {
		return biz.KnowledgeEmbedSetting{}
	}
	return biz.KnowledgeEmbedSetting{
		Provider:  e.KnowledgeEmbedProvider,
		BaseURL:   e.KnowledgeEmbedBaseURL,
		Model:     e.KnowledgeEmbedModel,
		Dim:       e.KnowledgeEmbedDim,
		HasAPIKey: strings.TrimSpace(e.KnowledgeEmbedAPIKey) != "",
	}
}

func entToWebResearch(e *ent.SystemSetting) biz.WebResearchSetting {
	if e == nil {
		return biz.WebResearchSetting{}
	}
	return biz.WebResearchSetting{
		Provider:    e.WebResearchProvider,
		MaxResults:  e.WebResearchMaxResults,
		FetchTop:    e.WebResearchFetchTop,
		SearchDepth: e.WebResearchSearchDepth,
		TimeoutSec:  e.WebResearchTimeoutSec,
		HTTPProxy:   e.WebResearchHTTPProxy,
		HasAPIKey:   strings.TrimSpace(e.WebResearchAPIKey) != "",
	}
}

func entToEvalLLM(e *ent.SystemSetting) biz.EvalLLMSetting {
	if e == nil {
		return biz.EvalLLMSetting{}
	}
	return biz.EvalLLMSetting{
		SimProvider:   e.EvalSimProvider,
		SimModel:      e.EvalSimModel,
		JudgeProvider: e.EvalJudgeProvider,
		JudgeModel:    e.EvalJudgeModel,
	}
}

func (r *systemSettingRepo) Get(ctx context.Context) (biz.SystemSetting, error) {
	row, err := r.data.RW().Read(ctx).SystemSetting.Get(ctx, systemSettingSingletonID)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.SystemSetting{}, apierror.NotFound(apierror.DomainData, "not found")
		}
		return biz.SystemSetting{}, err
	}
	out := entToBizSystemSetting(row)
	// PGO-3 (review follow-up): ent schema does not include refine_llm_* fields
	// because the generator can't run (tablewriter conflict); overlay them via
	// raw SQL so PromptRefiner.resolveModel Tier-2 fallback works again.
	// API key intentionally NOT loaded here — read it only via GetRefineLLM.
	if rl, rerr := r.getRefineLLMRedacted(ctx); rerr == nil {
		out.DefaultRefineLLM = rl
	}
	// Overlay planner model config (stored via DDL migration columns).
	if pm, perr := r.GetPlannerModel(ctx); perr == nil {
		out.PlannerModel = pm
	}
	// Overlay speech config (M74 V2-T7, DDL migration columns). Read error is
	// non-fatal: a missing speech column set (pre-migration DB) must not break
	// the whole settings read.
	if sp, serr := r.GetSpeech(ctx); serr == nil {
		out.Speech = sp
	}
	return out, nil
}

// getRefineLLMRedacted loads refine_llm_* columns without the api_key field.
// Used by Get() to populate biz.SystemSetting.DefaultRefineLLM safely.
func (r *systemSettingRepo) getRefineLLMRedacted(ctx context.Context) (biz.RefineLLMSetting, error) {
	rows, err := r.data.RW().Read(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT refine_llm_provider, refine_llm_model, refine_llm_base_url
		 FROM system_settings WHERE id = ? LIMIT 1`), systemSettingSingletonID)
	if err != nil {
		return biz.RefineLLMSetting{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.RefineLLMSetting{}, apierror.NotFound(apierror.DomainData, "not found")
	}
	var s biz.RefineLLMSetting
	if err := rows.Scan(&s.Provider, &s.Model, &s.BaseURL); err != nil {
		return biz.RefineLLMSetting{}, err
	}
	return s, nil
}

func (r *systemSettingRepo) EnsureCredentialEncryptionKey(ctx context.Context) (string, error) {
	return ensureCredentialEncryptionKeyOnClient(ctx, r.data.RW().Write(ctx), r.data.dialect)
}

func (r *systemSettingRepo) Update(ctx context.Context, rootDir, workDir string, globalMonthlyMicroUSD int64, a2aPublicBaseURL string, mcpAllowAdHocHTTP bool) (biz.SystemSetting, error) {
	row, err := r.data.RW().Write(ctx).SystemSetting.UpdateOneID(systemSettingSingletonID).
		SetRootDirectory(rootDir).
		SetWorkDirectory(workDir).
		SetGlobalMonthlyMicroUsd(globalMonthlyMicroUSD).
		SetA2aPublicBaseURL(a2aPublicBaseURL).
		SetMcpAllowAdhocHTTP(mcpAllowAdHocHTTP).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.SystemSetting{}, apierror.NotFound(apierror.DomainData, "not found")
		}
		return biz.SystemSetting{}, err
	}
	return entToBizSystemSetting(row), nil
}

func (r *systemSettingRepo) GetKnowledgeEmbed(ctx context.Context) (biz.KnowledgeEmbedSetting, error) {
	row, err := r.data.RW().Read(ctx).SystemSetting.Get(ctx, systemSettingSingletonID)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.KnowledgeEmbedSetting{}, apierror.NotFound(apierror.DomainData, "not found")
		}
		return biz.KnowledgeEmbedSetting{}, err
	}
	out := entToKnowledgeEmbed(row)
	out.APIKey = row.KnowledgeEmbedAPIKey
	return out, nil
}

func (r *systemSettingRepo) UpdateKnowledgeEmbed(ctx context.Context, patch biz.KnowledgeEmbedSetting, updateAPIKey bool) (biz.KnowledgeEmbedSetting, error) {
	up := r.data.RW().Write(ctx).SystemSetting.UpdateOneID(systemSettingSingletonID).
		SetKnowledgeEmbedProvider(patch.Provider).
		SetKnowledgeEmbedBaseURL(patch.BaseURL).
		SetKnowledgeEmbedModel(patch.Model).
		SetKnowledgeEmbedDim(patch.Dim)
	if updateAPIKey && strings.TrimSpace(patch.APIKey) != "" {
		up = up.SetKnowledgeEmbedAPIKey(strings.TrimSpace(patch.APIKey))
	}
	row, err := up.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.KnowledgeEmbedSetting{}, apierror.NotFound(apierror.DomainData, "not found")
		}
		return biz.KnowledgeEmbedSetting{}, err
	}
	return entToKnowledgeEmbed(row), nil
}

func (r *systemSettingRepo) GetWebResearch(ctx context.Context) (biz.WebResearchSetting, error) {
	row, err := r.data.RW().Read(ctx).SystemSetting.Get(ctx, systemSettingSingletonID)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.WebResearchSetting{}, apierror.NotFound(apierror.DomainData, "not found")
		}
		return biz.WebResearchSetting{}, err
	}
	out := entToWebResearch(row)
	out.APIKey = row.WebResearchAPIKey
	return out, nil
}

func (r *systemSettingRepo) UpdateWebResearch(ctx context.Context, patch biz.WebResearchSetting, updateAPIKey bool) (biz.WebResearchSetting, error) {
	up := r.data.RW().Write(ctx).SystemSetting.UpdateOneID(systemSettingSingletonID).
		SetWebResearchProvider(defaultWebResearchProvider(patch.Provider)).
		SetWebResearchMaxResults(defaultWebResearchMaxResults(patch.MaxResults)).
		SetWebResearchFetchTop(defaultWebResearchFetchTop(patch.FetchTop)).
		SetWebResearchSearchDepth(defaultWebResearchSearchDepth(patch.SearchDepth)).
		SetWebResearchTimeoutSec(defaultWebResearchTimeoutSec(patch.TimeoutSec)).
		SetWebResearchHTTPProxy(strings.TrimSpace(patch.HTTPProxy))
	if updateAPIKey && strings.TrimSpace(patch.APIKey) != "" {
		up = up.SetWebResearchAPIKey(strings.TrimSpace(patch.APIKey))
	}
	row, err := up.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.WebResearchSetting{}, apierror.NotFound(apierror.DomainData, "not found")
		}
		return biz.WebResearchSetting{}, err
	}
	return entToWebResearch(row), nil
}

func defaultWebResearchProvider(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "tavily"
	}
	return p
}

func defaultWebResearchMaxResults(n int) int {
	if n <= 0 {
		return 8
	}
	return n
}

func defaultWebResearchFetchTop(n int) int {
	if n <= 0 {
		return 5
	}
	return n
}

func defaultWebResearchSearchDepth(d string) string {
	d = strings.TrimSpace(d)
	if d == "" {
		return "basic"
	}
	return d
}

func defaultWebResearchTimeoutSec(n int) int {
	if n <= 0 {
		return 15
	}
	return n
}

func (r *systemSettingRepo) UpdateEvalLLM(ctx context.Context, patch biz.EvalLLMSetting) (biz.EvalLLMSetting, error) {
	row, err := r.data.RW().Write(ctx).SystemSetting.UpdateOneID(systemSettingSingletonID).
		SetEvalSimProvider(patch.SimProvider).
		SetEvalSimModel(patch.SimModel).
		SetEvalJudgeProvider(patch.JudgeProvider).
		SetEvalJudgeModel(patch.JudgeModel).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.EvalLLMSetting{}, apierror.NotFound(apierror.DomainData, "not found")
		}
		return biz.EvalLLMSetting{}, err
	}
	return entToEvalLLM(row), nil
}

// GetRefineLLM returns the stored platform default LLM for AI refinement (API key included).
// Uses raw SQL because the ent generator cannot be run due to a tablewriter version conflict.
func (r *systemSettingRepo) GetRefineLLM(ctx context.Context) (biz.RefineLLMSetting, error) {
	rows, err := r.data.RW().Read(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT refine_llm_provider, refine_llm_model, refine_llm_base_url, refine_llm_api_key
		 FROM system_settings WHERE id = ? LIMIT 1`), systemSettingSingletonID)
	if err != nil {
		return biz.RefineLLMSetting{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.RefineLLMSetting{}, apierror.NotFound(apierror.DomainData, "not found")
	}
	var s biz.RefineLLMSetting
	if err := rows.Scan(&s.Provider, &s.Model, &s.BaseURL, &s.APIKey); err != nil {
		return biz.RefineLLMSetting{}, err
	}
	return s, nil
}

// UpdateRefineLLM persists the platform default LLM for AI refinement.
// Uses raw SQL because the ent generator cannot be run due to a tablewriter version conflict.
func (r *systemSettingRepo) UpdateRefineLLM(ctx context.Context, patch biz.RefineLLMSetting, updateAPIKey bool) (biz.RefineLLMSetting, error) {
	if updateAPIKey {
		_, err := r.data.RW().Write(ctx).ExecContext(ctx,
			r.data.Dialect().RenumberPlaceholders(`UPDATE system_settings SET refine_llm_provider=?, refine_llm_model=?, refine_llm_base_url=?, refine_llm_api_key=? WHERE id=?`),
			patch.Provider, patch.Model, patch.BaseURL, strings.TrimSpace(patch.APIKey), systemSettingSingletonID)
		if err != nil {
			return biz.RefineLLMSetting{}, err
		}
	} else {
		_, err := r.data.RW().Write(ctx).ExecContext(ctx,
			r.data.Dialect().RenumberPlaceholders(`UPDATE system_settings SET refine_llm_provider=?, refine_llm_model=?, refine_llm_base_url=? WHERE id=?`),
			patch.Provider, patch.Model, patch.BaseURL, systemSettingSingletonID)
		if err != nil {
			return biz.RefineLLMSetting{}, err
		}
	}
	return biz.RefineLLMSetting{Provider: patch.Provider, Model: patch.Model, BaseURL: patch.BaseURL}, nil
}

// GetPlannerModel returns the plan_and_execute planner model configuration.
// Uses raw SQL because the columns are managed via DDL migration (same pattern
// as RefineLLM).
func (r *systemSettingRepo) GetPlannerModel(ctx context.Context) (biz.PlannerModelSetting, error) {
	rows, err := r.data.RW().Read(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT planner_model_mode, planner_model_provider, planner_model_model
		 FROM system_settings WHERE id = ? LIMIT 1`), systemSettingSingletonID)
	if err != nil {
		return biz.PlannerModelSetting{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.PlannerModelSetting{}, apierror.NotFound(apierror.DomainData, "not found")
	}
	var s biz.PlannerModelSetting
	if err := rows.Scan(&s.Mode, &s.Provider, &s.Model); err != nil {
		return biz.PlannerModelSetting{}, err
	}
	if strings.TrimSpace(s.Mode) == "" {
		s.Mode = biz.PlannerModelModeInherit
	}
	return s, nil
}

// UpdatePlannerModel persists the plan_and_execute planner model configuration.
// Uses raw SQL because the columns are managed via DDL migration.
func (r *systemSettingRepo) UpdatePlannerModel(ctx context.Context, patch biz.PlannerModelSetting) (biz.PlannerModelSetting, error) {
	_, err := r.data.RW().Write(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE system_settings SET planner_model_mode=?, planner_model_provider=?, planner_model_model=? WHERE id=?`),
		patch.Mode, patch.Provider, patch.Model, systemSettingSingletonID)
	if err != nil {
		return biz.PlannerModelSetting{}, err
	}
	return patch, nil
}

// GetSpeech returns the stored voice companion (M74) ASR/TTS provider config.
// Uses raw SQL because the columns are managed via DDL migration (same pattern
// as RefineLLM / PlannerModel). Empty fields mean "unset" — the runtime reader
// falls back to SPEECH_* env vars. speech_archive_user_audio is nullable:
// NULL = unset (env fallback), scanned into SpeechSetting.ArchiveUserAudio.
func (r *systemSettingRepo) GetSpeech(ctx context.Context) (biz.SpeechSetting, error) {
	rows, err := r.data.RW().Read(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT speech_asr_driver, speech_asr_endpoint, speech_asr_app_key,
		 speech_asr_access_key, speech_asr_resource_id, speech_asr_language,
		 speech_tts_driver, speech_tts_endpoint, speech_tts_app_key,
		 speech_tts_access_key, speech_tts_resource_id, speech_tts_voice,
		 speech_tts_speed_ratio, speech_archive_user_audio
		 FROM system_settings WHERE id = ? LIMIT 1`), systemSettingSingletonID)
	if err != nil {
		return biz.SpeechSetting{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.SpeechSetting{}, apierror.NotFound(apierror.DomainData, "not found")
	}
	var s biz.SpeechSetting
	var archive sql.NullBool
	if err := rows.Scan(
		&s.ASR.Driver, &s.ASR.Endpoint, &s.ASR.AppKey, &s.ASR.AccessKey, &s.ASR.ResourceID, &s.ASR.Language,
		&s.TTS.Driver, &s.TTS.Endpoint, &s.TTS.AppKey, &s.TTS.AccessKey, &s.TTS.ResourceID, &s.TTS.Voice,
		&s.TTS.SpeedRatio, &archive,
	); err != nil {
		return biz.SpeechSetting{}, err
	}
	if archive.Valid {
		v := archive.Bool
		s.ArchiveUserAudio = &v
	}
	return s, nil
}

// UpdateSpeech persists the voice companion speech settings. Credentials are
// replaced only when the matching updateXxxCred flag is set (empty = keep).
// ArchiveUserAudio nil writes NULL (unset → env fallback).
func (r *systemSettingRepo) UpdateSpeech(ctx context.Context, patch biz.SpeechSetting, updateASRCred, updateTTSCred bool) (biz.SpeechSetting, error) {
	var archive any
	if patch.ArchiveUserAudio != nil {
		archive = *patch.ArchiveUserAudio
	}
	query := `UPDATE system_settings SET speech_asr_driver=?, speech_asr_endpoint=?, speech_asr_resource_id=?,
		 speech_asr_language=?, speech_tts_driver=?, speech_tts_endpoint=?, speech_tts_resource_id=?,
		 speech_tts_voice=?, speech_tts_speed_ratio=?, speech_archive_user_audio=?`
	args := []any{
		patch.ASR.Driver, patch.ASR.Endpoint, patch.ASR.ResourceID, patch.ASR.Language,
		patch.TTS.Driver, patch.TTS.Endpoint, patch.TTS.ResourceID, patch.TTS.Voice,
		patch.TTS.SpeedRatio, archive,
	}
	if updateASRCred {
		query += `, speech_asr_app_key=?, speech_asr_access_key=?`
		args = append(args, strings.TrimSpace(patch.ASR.AppKey), strings.TrimSpace(patch.ASR.AccessKey))
	}
	if updateTTSCred {
		query += `, speech_tts_app_key=?, speech_tts_access_key=?`
		args = append(args, strings.TrimSpace(patch.TTS.AppKey), strings.TrimSpace(patch.TTS.AccessKey))
	}
	query += ` WHERE id=?`
	args = append(args, systemSettingSingletonID)
	if _, err := r.data.RW().Write(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(query), args...); err != nil {
		return biz.SpeechSetting{}, err
	}
	// Return the stored row (with credentials populated for has_api_key mapping).
	return r.GetSpeech(ctx)
}

func (r *systemSettingRepo) UpdateMemoryPlatform(ctx context.Context, patch biz.MemoryPlatformSetting) (biz.MemoryPlatformSetting, error) {
	row, err := r.data.RW().Write(ctx).SystemSetting.UpdateOneID(systemSettingSingletonID).
		SetMemoryPolicyStrict(patch.PolicyStrict).
		SetMemoryEpisodeBackfillDisabled(patch.EpisodeBackfillDisabled).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.MemoryPlatformSetting{}, apierror.NotFound(apierror.DomainData, "not found")
		}
		return biz.MemoryPlatformSetting{}, err
	}
	return biz.MemoryPlatformSetting{
		PolicyStrict:            row.MemoryPolicyStrict,
		EpisodeBackfillDisabled: row.MemoryEpisodeBackfillDisabled,
	}, nil
}
