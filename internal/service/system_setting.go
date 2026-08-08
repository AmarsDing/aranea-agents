package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/system_setting/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SystemSettingService implements system_setting.v1.
type SystemSettingService struct {
	v1.UnimplementedSystemSettingServiceServer

	uc            *biz.SystemSettingUsecase
	a2aPublicBase *A2APublicBaseReloader
	crypto        *biz.CredentialCrypto
	mon           *biz.MonitorUsecase
	// embedderAdmin hot-reloads in-memory knowledge embedder after DB persist (same path as KnowledgeService).
	embedderAdmin knowledge.EmbedderAdmin
	lg            loggateway.Logger
	monitorBus    contract.MonitorBus
}

func NewSystemSettingService(
	uc *biz.SystemSettingUsecase,
	a2aPublicBase *A2APublicBaseReloader,
	crypto *biz.CredentialCrypto,
	embedder knowledge.Embedder,
	mon *biz.MonitorUsecase,
	lg loggateway.Logger,
	monitorBus contract.MonitorBus,
) *SystemSettingService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	var admin knowledge.EmbedderAdmin
	if a, ok := embedder.(knowledge.EmbedderAdmin); ok {
		admin = a
	}
	return &SystemSettingService{
		uc:            uc,
		a2aPublicBase: a2aPublicBase,
		crypto:        crypto,
		mon:           mon,
		embedderAdmin: admin,
		lg:            lg,
		monitorBus:    monitorBus,
	}
}

// emitFlow emits a system-domain flow log for settings changes; nil-safe when
// the monitor bus is unavailable (tests).
func (s *SystemSettingService) emitFlow(ctx context.Context, stepID, message string, flowErr error, pairs ...event.Pair) {
	if s == nil || s.monitorBus == nil {
		return
	}
	em := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:    ctx,
		Domain: event.TraceDomainSystem,
		LG:     s.lg,
		Infra:  event.NewInfraFromBus(s.monitorBus),
	})
	if flowErr != nil {
		em.LogError(stepID, message, append(pairs, event.P("error", flowErr.Error()))...)
		return
	}
	em.LogDone(stepID, message, pairs...)
}

func (s *SystemSettingService) GetSystemSettings(ctx context.Context, _ *emptypb.Empty) (*v1.SystemSettings, error) {
	row, err := s.uc.Get(ctx)
	if err != nil {
		return nil, err
	}
	return toProtoSystemSettings(row), nil
}

func (s *SystemSettingService) UpdateSystemSettings(ctx context.Context, req *v1.UpdateSystemSettingsRequest) (*v1.SystemSettings, error) {
	patch := biz.SystemSettingAllPatch{
		RootDir:               req.GetRootDirectory(),
		WorkDir:               req.GetWorkDirectory(),
		GlobalMonthlyMicroUSD: req.GetGlobalMonthlyMicroUsd(),
		A2APublicBaseURL:      req.GetA2APublicBaseUrl(),
		MCPAllowAdHocHTTP:     req.GetMcpAllowAdhocHttp(),
	}
	if hasKnowledgeEmbedUpdate(req) {
		patch.KnowledgeEmbed = &biz.KnowledgeEmbedSetting{
			Provider: req.GetKnowledgeEmbedProvider(),
			BaseURL:  req.GetKnowledgeEmbedBaseUrl(),
			APIKey:   req.GetKnowledgeEmbedApiKey(),
			Model:    req.GetKnowledgeEmbedModel(),
			Dim:      int(req.GetKnowledgeEmbedDim()),
		}
		patch.KnowledgeEmbedUpdateKey = strings.TrimSpace(req.GetKnowledgeEmbedApiKey()) != ""
	}
	if hasEvalLLMUpdate(req) {
		patch.EvalLLM = &biz.EvalLLMSetting{
			SimProvider:   req.GetEvalSimProvider(),
			SimModel:      req.GetEvalSimModel(),
			JudgeProvider: req.GetEvalJudgeProvider(),
			JudgeModel:    req.GetEvalJudgeModel(),
		}
	}
	if hasWebResearchUpdate(req) {
		patch.WebResearch = &biz.WebResearchSetting{
			Provider:    req.GetWebResearchProvider(),
			APIKey:      req.GetWebResearchApiKey(),
			MaxResults:  int(req.GetWebResearchMaxResults()),
			FetchTop:    int(req.GetWebResearchFetchTop()),
			SearchDepth: req.GetWebResearchSearchDepth(),
			TimeoutSec:  int(req.GetWebResearchTimeoutSec()),
			HTTPProxy:   req.GetWebResearchHttpProxy(),
		}
		patch.WebResearchUpdateKey = strings.TrimSpace(req.GetWebResearchApiKey()) != ""
	}
	if hasSpeechUpdate(req) {
		patch.Speech = &biz.SpeechSetting{
			ASR: biz.ASRProviderConfig{
				Driver:     req.GetSpeechAsrDriver(),
				Endpoint:   req.GetSpeechAsrEndpoint(),
				APIKey:     req.GetSpeechAsrApiKey(),
				AppKey:     req.GetSpeechAsrAppKey(),
				AccessKey:  req.GetSpeechAsrAccessKey(),
				ResourceID: req.GetSpeechAsrResourceId(),
				Language:   req.GetSpeechAsrLanguage(),
			},
			TTS: biz.TTSProviderConfig{
				Driver:     req.GetSpeechTtsDriver(),
				Endpoint:   req.GetSpeechTtsEndpoint(),
				APIKey:     req.GetSpeechTtsApiKey(),
				AppKey:     req.GetSpeechTtsAppKey(),
				AccessKey:  req.GetSpeechTtsAccessKey(),
				ResourceID: req.GetSpeechTtsResourceId(),
				Voice:      req.GetSpeechTtsVoice(),
				SpeedRatio: req.GetSpeechTtsSpeedRatio(),
			},
			ArchiveUserAudio: req.SpeechArchiveUserAudio, // *bool: nil = keep stored
		}
		patch.SpeechUpdateASRCred = strings.TrimSpace(req.GetSpeechAsrApiKey()) != "" ||
			strings.TrimSpace(req.GetSpeechAsrAppKey()) != "" ||
			strings.TrimSpace(req.GetSpeechAsrAccessKey()) != ""
		patch.SpeechUpdateTTSCred = strings.TrimSpace(req.GetSpeechTtsApiKey()) != "" ||
			strings.TrimSpace(req.GetSpeechTtsAppKey()) != "" ||
			strings.TrimSpace(req.GetSpeechTtsAccessKey()) != ""
	}
	// 流程日志 extra 仅含更新的 key 列表（分区名），严禁记录任何配置值（可能敏感）。
	updateKeys := updatedSettingSections(req, patch)
	row, err := s.uc.UpdateAll(ctx, patch)
	if err != nil {
		s.emitFlow(ctx, "settings.update", "系统设置更新失败", err,
			event.P("keys", strings.Join(updateKeys, ",")))
		return nil, err
	}
	s.emitFlow(ctx, "settings.update", "系统设置更新完成", nil,
		event.P("keys", strings.Join(updateKeys, ",")))
	// 配置变更为单例实体：resource=config 无 resource_id，summary 仅列变更的分区名（严禁记录密钥值）。
	recordAudit(ctx, s.mon, biz.AdminAuditEntry{
		Action:   biz.AuditAction(biz.AuditVerbUpdate, "config"),
		Resource: "config",
		Summary:  "sections=" + strings.Join(updatedSettingSections(req, patch), ","),
	})
	var hotReloaded []string
	if s.a2aPublicBase != nil {
		s.a2aPublicBase.Reload(row.A2APublicBaseURL)
		hotReloaded = append(hotReloaded, "a2a_public_base_url")
	}
	// Keep in-memory knowledge embedder aligned with DB (authoritative for live RAG).
	if patch.KnowledgeEmbed != nil && s.embedderAdmin != nil {
		ke := row.KnowledgeEmbed
		apiKey := ""
		if patch.KnowledgeEmbedUpdateKey {
			apiKey = strings.TrimSpace(patch.KnowledgeEmbed.APIKey)
		}
		s.embedderAdmin.Update(ke.Provider, ke.BaseURL, apiKey, ke.Model, ke.Dim)
		s.lg.Info("system settings: knowledge embedder hot-reloaded after persist",
			loggateway.StepID("system_setting.embedder.reload"),
		)
		hotReloaded = append(hotReloaded, "knowledge_embedder")
	}
	if len(hotReloaded) > 0 {
		s.emitFlow(ctx, "settings.hot_reload", "配置热更新完成", nil,
			event.P("items", strings.Join(hotReloaded, ",")))
	}
	return toProtoSystemSettings(row), nil
}

// updatedSettingSections 列出本次更新涉及的配置分区名（用于审计 summary，不含任何密钥值）。
func updatedSettingSections(req *v1.UpdateSystemSettingsRequest, patch biz.SystemSettingAllPatch) []string {
	sections := []string{"general"} // root/work dir、月度配额、A2A base URL、MCP 开关始终随表单提交
	if patch.KnowledgeEmbed != nil || hasKnowledgeEmbedUpdate(req) {
		sections = append(sections, "knowledge_embed")
	}
	if patch.EvalLLM != nil || hasEvalLLMUpdate(req) {
		sections = append(sections, "eval_llm")
	}
	if patch.WebResearch != nil || hasWebResearchUpdate(req) {
		sections = append(sections, "web_research")
	}
	if patch.Speech != nil || hasSpeechUpdate(req) {
		sections = append(sections, "speech")
	}
	return sections
}

// hasSpeechUpdate reports whether the request carries any speech-group field.
// archive_user_audio uses proto3 optional presence (explicit false must count).
func hasSpeechUpdate(req *v1.UpdateSystemSettingsRequest) bool {
	if req == nil {
		return false
	}
	return req.GetSpeechAsrDriver() != "" ||
		req.GetSpeechAsrEndpoint() != "" ||
		req.GetSpeechAsrResourceId() != "" ||
		req.GetSpeechAsrLanguage() != "" ||
		strings.TrimSpace(req.GetSpeechAsrApiKey()) != "" ||
		strings.TrimSpace(req.GetSpeechAsrAppKey()) != "" ||
		strings.TrimSpace(req.GetSpeechAsrAccessKey()) != "" ||
		req.GetSpeechTtsDriver() != "" ||
		req.GetSpeechTtsEndpoint() != "" ||
		req.GetSpeechTtsResourceId() != "" ||
		req.GetSpeechTtsVoice() != "" ||
		req.GetSpeechTtsSpeedRatio() > 0 ||
		strings.TrimSpace(req.GetSpeechTtsApiKey()) != "" ||
		strings.TrimSpace(req.GetSpeechTtsAppKey()) != "" ||
		strings.TrimSpace(req.GetSpeechTtsAccessKey()) != "" ||
		req.SpeechArchiveUserAudio != nil
}

func hasWebResearchUpdate(req *v1.UpdateSystemSettingsRequest) bool {
	if req == nil {
		return false
	}
	return req.GetWebResearchProvider() != "" ||
		req.GetWebResearchMaxResults() > 0 ||
		req.GetWebResearchFetchTop() > 0 ||
		req.GetWebResearchSearchDepth() != "" ||
		req.GetWebResearchTimeoutSec() > 0 ||
		req.GetWebResearchHttpProxy() != "" ||
		strings.TrimSpace(req.GetWebResearchApiKey()) != ""
}

func hasKnowledgeEmbedUpdate(req *v1.UpdateSystemSettingsRequest) bool {
	if req == nil {
		return false
	}
	return req.GetKnowledgeEmbedProvider() != "" ||
		req.GetKnowledgeEmbedBaseUrl() != "" ||
		req.GetKnowledgeEmbedModel() != "" ||
		req.GetKnowledgeEmbedDim() > 0 ||
		strings.TrimSpace(req.GetKnowledgeEmbedApiKey()) != ""
}

func hasEvalLLMUpdate(req *v1.UpdateSystemSettingsRequest) bool {
	if req == nil {
		return false
	}
	return req.GetEvalSimProvider() != "" ||
		req.GetEvalSimModel() != "" ||
		req.GetEvalJudgeProvider() != "" ||
		req.GetEvalJudgeModel() != ""
}

func toProtoSystemSettings(row biz.SystemSetting) *v1.SystemSettings {
	return &v1.SystemSettings{
		WorkDirectory:                     row.WorkDirectory,
		UpdateTime:                        timestamppb.New(row.UpdateTime),
		RootDirectory:                     row.RootDirectory,
		GlobalMonthlyMicroUsd:             row.GlobalMonthlyMicroUSD,
		A2APublicBaseUrl:                  row.A2APublicBaseURL,
		CredentialEncryptionKeyConfigured: row.CredentialEncryptionKeyConfigured,
		KnowledgeEmbed:                    toProtoKnowledgeEmbed(row.KnowledgeEmbed),
		EvalLlm:                           toProtoEvalLLM(row.EvalLLM),
		McpAllowAdhocHttp:                 row.MCPAllowAdHocHTTP,
		WebResearch:                       toProtoWebResearch(row.WebResearch),
		Speech:                            toProtoSpeech(row.Speech),
	}
}

// toProtoSpeech maps stored speech settings to the API view. Credentials are
// never exposed — only has_api_key markers (knowledge_embed/web_research 同惯例)。
// has_api_key 双模式：X-Api-Key 或 legacy AppKey+AccessKey 对任一完整即 true。
func toProtoSpeech(row biz.SpeechSetting) *v1.SpeechSettings {
	return &v1.SpeechSettings{
		Asr: &v1.SpeechASRSettings{
			Driver:     row.ASR.Driver,
			Endpoint:   row.ASR.Endpoint,
			ResourceId: row.ASR.ResourceID,
			Language:   row.ASR.Language,
			Configured: biz.SpeechASRConfigured(row),
			HasApiKey:  biz.SpeechCredOK(row.ASR.APIKey, row.ASR.AppKey, row.ASR.AccessKey),
		},
		Tts: &v1.SpeechTTSSettings{
			Driver:     row.TTS.Driver,
			Endpoint:   row.TTS.Endpoint,
			ResourceId: row.TTS.ResourceID,
			Voice:      row.TTS.Voice,
			SpeedRatio: row.TTS.SpeedRatio,
			Configured: biz.SpeechTTSConfigured(row),
			HasApiKey:  biz.SpeechCredOK(row.TTS.APIKey, row.TTS.AppKey, row.TTS.AccessKey),
		},
		ArchiveUserAudio: row.ArchiveUserAudio != nil && *row.ArchiveUserAudio,
	}
}

func toProtoWebResearch(row biz.WebResearchSetting) *v1.WebResearchSettings {
	return &v1.WebResearchSettings{
		Provider:    row.Provider,
		MaxResults:  int32(row.MaxResults),
		FetchTop:    int32(row.FetchTop),
		SearchDepth: row.SearchDepth,
		TimeoutSec:  int32(row.TimeoutSec),
		HttpProxy:   row.HTTPProxy,
		Configured:  biz.WebResearchConfigured(row),
		HasApiKey:   row.HasAPIKey,
	}
}

func toProtoKnowledgeEmbed(row biz.KnowledgeEmbedSetting) *v1.KnowledgeEmbedSettings {
	return &v1.KnowledgeEmbedSettings{
		Provider:   row.Provider,
		BaseUrl:    row.BaseURL,
		Model:      row.Model,
		Dim:        int32(row.Dim),
		Configured: biz.KnowledgeEmbedConfigured(row),
		HasApiKey:  row.HasAPIKey,
	}
}

func (s *SystemSettingService) TestWebResearch(ctx context.Context, req *v1.TestWebResearchRequest) (*v1.TestWebResearchResponse, error) {
	if req == nil {
		req = &v1.TestWebResearchRequest{}
	}
	res, err := s.uc.TestWebResearch(ctx, biz.WebResearchSetting{
		Provider:    req.GetProvider(),
		MaxResults:  int(req.GetMaxResults()),
		FetchTop:    int(req.GetFetchTop()),
		SearchDepth: req.GetSearchDepth(),
		TimeoutSec:  int(req.GetTimeoutSec()),
		HTTPProxy:   req.GetHttpProxy(),
	}, req.GetApiKey())
	out := &v1.TestWebResearchResponse{
		Ok:              res.OK,
		Message:         res.Message,
		Provider:        res.Provider,
		ResultCount:     int32(res.ResultCount),
		LatencyMs:       int32(res.LatencyMS),
		ResponseTimeSec: res.ResponseTime,
	}
	if err != nil {
		if out.Message == "" {
			out.Message = err.Error()
		}
		return out, nil
	}
	return out, nil
}

func toProtoEvalLLM(row biz.EvalLLMSetting) *v1.EvalLLMSettings {
	return &v1.EvalLLMSettings{
		SimProvider:   row.SimProvider,
		SimModel:      row.SimModel,
		JudgeProvider: row.JudgeProvider,
		JudgeModel:    row.JudgeModel,
		Configured:    row.SimConfigured() || row.JudgeConfigured(),
	}
}
