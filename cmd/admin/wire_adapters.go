package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/monitor/heal"
	bizsession "aranea-agents/internal/biz/session"
	biztool "aranea-agents/internal/biz/tool"
	bizusage "aranea-agents/internal/biz/usage"
	"aranea-agents/internal/llminspect"
	mcpconfig "aranea-agents/internal/mcp/config"
	mcpmetadata "aranea-agents/internal/mcp/metadata"
	mcpprobe "aranea-agents/internal/mcp/probe"
	"aranea-agents/internal/server"
	"aranea-agents/internal/service"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/testexec"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
	loggateway "aranea-agents/pkg/loggateway"
)

// mcpProberAdapter wraps internal/mcp/probe to implement biz.MCPProber.
type mcpProberAdapter struct {
	prober *mcpprobe.Prober
}

func (a mcpProberAdapter) Evaluate(ctx context.Context, enabled bool, configJSON string) biz.MCPTestResult {
	r := a.prober.Evaluate(ctx, enabled, configJSON)
	return biz.MCPTestResult{OK: r.OK, Status: r.Status, Message: r.Message, Details: r.Details}
}

func provideMCPProber() biz.MCPProber {
	// Probe path has no persisted server context: pass an empty serverKey so a
	// rotation during probing stays in-memory only (no config_json write-back).
	resolver := func(ctx context.Context, auth mcpconfig.AuthConfig) (string, error) {
		return chatagent.ResolveMCPAuthToken(ctx, "", auth)
	}
	prober := mcpprobe.NewProber(resolver)
	// P2: full_handshake probe mode rides the same real-handshake discovery as
	// the manual/runner paths. cfg.Name may be empty (config_json carries no
	// server key); prefix stripping is best-effort display sugar only.
	prober.SetHandshakeFunc(func(ctx context.Context, cfg mcpconfig.ServerConfig, headers map[string]string) ([]string, error) {
		return tools.DiscoverMCPToolNames(ctx, tools.MCPServerConfigFromServerConfig(cfg.Name, cfg, headers))
	})
	return mcpProberAdapter{prober: prober}
}

// mcpToolDiscovererAdapter wraps internal/tools real-handshake discovery to
// implement biz.MCPToolDiscoverer (P2). configJSON is already decrypted by
// the biz layer; auth headers are resolved with the same logic the probe
// pipeline uses so discovery never diverges from connectivity probing.
type mcpToolDiscovererAdapter struct{}

func (mcpToolDiscovererAdapter) DiscoverTools(ctx context.Context, serverKey string, configJSON string) ([]string, error) {
	cfg, err := mcpconfig.ParseServerConfigJSON(configJSON)
	if err != nil {
		return nil, fmt.Errorf("config_json 格式错误: %w", err)
	}
	headers, err := mcpprobe.ResolveHeaders(ctx, func(ctx context.Context, auth mcpconfig.AuthConfig) (string, error) {
		return chatagent.ResolveMCPAuthToken(ctx, serverKey, auth)
	}, cfg)
	if err != nil {
		return nil, fmt.Errorf("鉴权凭据解析失败: %w", err)
	}
	return tools.DiscoverMCPToolNames(ctx, tools.MCPServerConfigFromServerConfig(serverKey, cfg, headers))
}

// mcpMetadataAdapter wraps internal/mcp/metadata to implement biz.MCPMetadataEditor.
type mcpMetadataAdapter struct{}

func (mcpMetadataAdapter) Parse(raw string) map[string]any          { return mcpmetadata.Parse(raw) }
func (mcpMetadataAdapter) Marshal(m map[string]any) (string, error) { return mcpmetadata.Marshal(m) }
func (mcpMetadataAdapter) ApplyHealth(m map[string]any, healthStatus string, ok bool, errMsg string, at time.Time) (map[string]any, string) {
	return mcpmetadata.ApplyHealth(m, healthStatus, ok, errMsg, at)
}
func (mcpMetadataAdapter) ApplyReconnect(m map[string]any, at time.Time) map[string]any {
	return mcpmetadata.ApplyReconnect(m, at)
}
func (mcpMetadataAdapter) MarkHealthAlert(m map[string]any, at time.Time) map[string]any {
	return mcpmetadata.MarkHealthAlert(m, at)
}
func (mcpMetadataAdapter) ApplyToolDiscovery(m map[string]any, count int, names []string, at time.Time) map[string]any {
	return mcpmetadata.ApplyToolDiscovery(m, count, names, at)
}
func (mcpMetadataAdapter) ApplyToolDiscoveryError(m map[string]any, errMsg string, at time.Time) map[string]any {
	return mcpmetadata.ApplyToolDiscoveryError(m, errMsg, at)
}

func provideMCPMetadataEditor() biz.MCPMetadataEditor { return mcpMetadataAdapter{} }

// llmInspectorAdapter wraps internal/llminspect to implement biz.LLMInspector.
type llmInspectorAdapter struct {
	lg loggateway.Logger
}

func (a llmInspectorAdapter) Run(in biz.InspectMerge) (biz.LLMInspectResult, error) {
	r, err := llminspect.Run(llminspect.Input{
		ResourceID:   in.ResourceID,
		ProviderCode: in.ProviderCode,
		ProviderType: in.ProviderType,
		ModelAPIID:   in.ModelAPIID,
		APIBaseURL:   in.APIBaseURL,
		APIKey:       in.APIKey,
		Variant:      in.Variant,
		SecretID:     in.SecretID,
		SecretKey:    in.SecretKey,
		AWSRegion:    in.AWSRegion,
	}, a.lg)
	if err != nil {
		return biz.LLMInspectResult{}, err
	}
	return biz.LLMInspectResult{
		OK:                            r.OK,
		Message:                       r.Message,
		ProviderCode:                  r.ProviderCode,
		ProviderType:                  r.ProviderType,
		ModelAPIID:                    r.ModelAPIID,
		ModelDisplayName:              r.ModelDisplayName,
		ModelSizeLabel:                r.ModelSizeLabel,
		ContextWindowK:                r.ContextWindowK,
		MaxOutputTokens:               r.MaxOutputTokens,
		InputPriceMicroUSDPer1K:       r.InputPriceMicroUSDPer1K,
		OutputPriceMicroUSDPer1K:      r.OutputPriceMicroUSDPer1K,
		CachedInputPriceMicroUSDPer1K: r.CachedInputPriceMicroUSDPer1K,
		ReasoningPriceMicroUSDPer1K:   r.ReasoningPriceMicroUSDPer1K,
		EmbeddingPriceMicroUSDPer1K:   r.EmbeddingPriceMicroUSDPer1K,
		Source:                        r.Source,
		RawMetadataJSON:               r.RawMetadataJSON,
		Variant:                       r.Variant,
		EnableTokenTailoring:          r.EnableTokenTailoring,
		SupportsCache:                 r.SupportsCache,
		SupportsThinking:              r.SupportsThinking,
	}, nil
}

func provideLLMInspector(lg loggateway.Logger) biz.LLMInspector { return llmInspectorAdapter{lg: lg} }

// webResearchReadinessAdapter wraps internal/tools/webresearch to implement biztool.WebResearchReadinessChecker.
type webResearchReadinessAdapter struct{}

func (webResearchReadinessAdapter) ResolveReady(agentMap map[string]any, platform *biztool.WebResearchPlatformFields) bool {
	return webresearchpkg.ResolveReady(agentMap, bizToolToWebResearchPlatform(platform))
}

func (webResearchReadinessAdapter) IsReady(agentMap map[string]any, platform *biztool.WebResearchPlatformFields) bool {
	return webresearchpkg.CatalogReady(agentMap, bizToolToWebResearchPlatform(platform))
}

func bizToolToWebResearchPlatform(p *biztool.WebResearchPlatformFields) *webresearchpkg.PlatformFields {
	if p == nil {
		return nil
	}
	return &webresearchpkg.PlatformFields{
		HasAPIKey:   p.HasAPIKey,
		APIKey:      p.APIKey,
		Provider:    p.Provider,
		MaxResults:  p.MaxResults,
		FetchTop:    p.FetchTop,
		SearchDepth: p.SearchDepth,
		TimeoutSec:  p.TimeoutSec,
		HTTPProxy:   p.HTTPProxy,
	}
}

func provideWebResearchReadinessChecker() biztool.WebResearchReadinessChecker {
	return webResearchReadinessAdapter{}
}

// bizWebResearchReadinessAdapter wraps internal/tools/webresearch to implement biz.WebResearchReadinessChecker.
type bizWebResearchReadinessAdapter struct{}

func (bizWebResearchReadinessAdapter) ResolveReady(agentMap map[string]any, platform *biz.WebResearchPlatformFields) bool {
	return webresearchpkg.ResolveReady(agentMap, bizToWebResearchPlatform(platform))
}

func (bizWebResearchReadinessAdapter) IsReady(agentMap map[string]any, platform *biz.WebResearchPlatformFields) bool {
	return webresearchpkg.CatalogReady(agentMap, bizToWebResearchPlatform(platform))
}

func bizToWebResearchPlatform(p *biz.WebResearchPlatformFields) *webresearchpkg.PlatformFields {
	if p == nil {
		return nil
	}
	return &webresearchpkg.PlatformFields{
		HasAPIKey:   p.HasAPIKey,
		APIKey:      p.APIKey,
		Provider:    p.Provider,
		MaxResults:  p.MaxResults,
		FetchTop:    p.FetchTop,
		SearchDepth: p.SearchDepth,
		TimeoutSec:  p.TimeoutSec,
		HTTPProxy:   p.HTTPProxy,
	}
}

func provideBizWebResearchReadinessChecker() biz.WebResearchReadinessChecker {
	return bizWebResearchReadinessAdapter{}
}

// toolTesterAdapter wraps internal/tools/testexec to implement biztool.ToolTester.
type toolTesterAdapter struct {
	lg loggateway.Logger
}

func (a toolTesterAdapter) Execute(ctx context.Context, tool biztool.ToolTestInput, argumentsJSON string, timeoutSec int, platform *biztool.WebResearchPlatformFields) (biztool.ToolTestResult, error) {
	var pf *webresearchpkg.PlatformFields
	if platform != nil {
		pf = &webresearchpkg.PlatformFields{
			HasAPIKey:   platform.HasAPIKey,
			APIKey:      platform.APIKey,
			Provider:    platform.Provider,
			MaxResults:  platform.MaxResults,
			FetchTop:    platform.FetchTop,
			SearchDepth: platform.SearchDepth,
			TimeoutSec:  platform.TimeoutSec,
			HTTPProxy:   platform.HTTPProxy,
		}
	}
	res, err := testexec.Execute(ctx, testexec.CatalogTool{
		Key:               tool.Key,
		Source:            tool.Source,
		ConfigJSON:        tool.ConfigJSON,
		DefaultConfigJSON: tool.DefaultConfigJSON,
		MetadataJSON:      tool.MetadataJSON,
	}, argumentsJSON, timeoutSec, pf, a.lg)
	if err != nil {
		return biztool.ToolTestResult{}, err
	}
	return biztool.ToolTestResult{
		Status:        res.Status,
		ResultPreview: res.ResultPreview,
		ErrorMessage:  res.ErrorMessage,
		DurationMS:    res.DurationMS,
	}, nil
}

func provideToolTester(lg loggateway.Logger) biztool.ToolTester { return toolTesterAdapter{lg: lg} }

// sessionMetricsAdapter adapts SessionUsecase to the usage.SessionMetricsWriter interface.
type sessionMetricsAdapter struct {
	sessions *biz.SessionUsecase
}

func (a *sessionMetricsAdapter) AccumulateMetricsDelta(delta bizusage.SessionMetricsDelta) {
	if a.sessions == nil {
		return
	}
	a.sessions.AccumulateMetricsDelta(bizsession.SessionMetricsDelta{
		SessionID:         delta.SessionID,
		MessageCount:      delta.MessageCount,
		ModelCallCount:    delta.ModelCallCount,
		ToolCallCount:     delta.ToolCallCount,
		SkillCallCount:    delta.SkillCallCount,
		McpCallCount:      delta.McpCallCount,
		InputTokens:       delta.InputTokens,
		OutputTokens:      delta.OutputTokens,
		TotalTokens:       delta.TotalTokens,
		TotalCostMicroUsd: delta.TotalCostMicroUsd,
	})
}

// completionLinkerAdapter adapts MonitorUsecase to the usage.CompletionUsageLinker interface.
type completionLinkerAdapter struct {
	mon *biz.MonitorUsecase
}

func (a *completionLinkerAdapter) LinkRunnerCompletionUsage(ctx context.Context, sessionID, runID, usageEventID, traceID string) error {
	return biz.LinkRunnerCompletionUsage(ctx, a.mon, sessionID, runID, usageEventID, traceID)
}

// envelopePublisherAdapter adapts biz.EventBus to the usage.UsageEnvelopePublisher interface.
type envelopePublisherAdapter struct {
	eventBus biz.EventBus
}

func (a *envelopePublisherAdapter) PublishTokenUsageEnvelope(ctx context.Context, e bizusage.TokenUsageEvent) {
	biz.PublishTokenUsageEnvelope(ctx, a.eventBus, e)
}

// sessionChildLookupAdapter adapts SessionUsecase to team.SessionChildLookup.
// Uses ListChildSessions to find the child session with the matching MemberAgentKey.
type sessionChildLookupAdapter struct {
	sessions *biz.SessionUsecase
}

func (a *sessionChildLookupAdapter) LookupChildSessionID(ctx context.Context, parentSessionID, memberAgentKey string) (string, error) {
	children, err := a.sessions.ListChildSessions(ctx, parentSessionID)
	if err != nil {
		return "", err
	}
	for _, child := range children {
		if child.MemberAgentKey == memberAgentKey {
			return child.ID, nil
		}
	}
	return "", nil // not found is not an error — caller falls back to team session ID
}

// wsTurnExecutorAdapter adapts biz.TurnExecutorGateway to server.WSTurnExecutor.
type wsTurnExecutorAdapter struct {
	gateway biz.TurnExecutorGateway
	lg      loggateway.Logger
}

func provideWSTurnExecutor(gateway biz.TurnExecutorGateway, lg loggateway.Logger) server.WSTurnExecutor {
	return &wsTurnExecutorAdapter{gateway: gateway, lg: lg}
}

func (a *wsTurnExecutorAdapter) ExecuteTurn(ctx context.Context, input server.WSTurnInput) error {
	bizInput := biz.TurnInput{
		SessionID: input.SessionID,
		Content:   input.Content,
		AgentKey:  input.AgentKey,
		TeamID:    input.TeamID,
		// Voice 语音溯源元数据必须透传：prepareRunContext 依此打 voice
		// fast-path 标记（主 LLM 关思考），并持久化 ASR 溯源（V2-T6）。
		Voice: input.Voice,
		Options: biz.TurnOptions{
			DialogMode:     input.Options.DialogMode,
			Provider:       input.Options.Provider,
			Model:          input.Options.Model,
			AttachmentIDs:  input.Options.AttachmentIDs,
			KnowledgeBases: input.Options.KnowledgeBases,
		},
		EntryConfig: biz.TurnEntryPointConfig{
			EntryPoint:  biz.EntryPointWS,
			AllowQueue:  input.AllowQueue,
			AllowStream: input.AllowStream,
		},
	}
	start := time.Now()
	_, err := a.gateway.ExecuteTurn(ctx, bizInput)
	elapsed := time.Since(start)
	if errors.Is(err, service.ErrTurnMessageQueued) {
		// queued = 成功受理（消息已入排队队列），不是失败。
		// 与 chatTurnExecutor / spirit_team 的语义一致；透传会让前端渲染假 send_failed 卡片。
		a.lg.With(loggateway.SessionID(input.SessionID)).Info("wsTurnExecutorAdapter.ExecuteTurn 消息已排队",
			loggateway.StepID("ws.adapter_turn_done"),
			loggateway.Any("elapsed_ms", elapsed.Milliseconds()))
		return nil
	}
	a.lg.With(loggateway.SessionID(input.SessionID)).Info("wsTurnExecutorAdapter.ExecuteTurn 完成",
		loggateway.StepID("ws.adapter_turn_done"),
		loggateway.Any("elapsed_ms", elapsed.Milliseconds()),
		loggateway.Any("has_error", err != nil))
	return err
}

// provideBizRootCauseAdapter bridges heal.RootCauseAnalyzer to biz.RootCauseAnalyzer.
func provideBizRootCauseAdapter(rca heal.RootCauseAnalyzer) biz.RootCauseAnalyzer {
	return &skillIntelligenceRCAAdapter{inner: rca}
}

// skillIntelligenceRCAAdapter bridges heal.RootCauseAnalyzer to biz.RootCauseAnalyzer.
type skillIntelligenceRCAAdapter struct {
	inner heal.RootCauseAnalyzer
}

func (a *skillIntelligenceRCAAdapter) AnalyzeInvocationFailure(ctx context.Context, inv biz.SkillInvocationWrite) (*biz.RootCauseAnalysisResult, error) {
	report := &heal.FailureReport{
		Type:      heal.FailureTypeRuntime,
		Source:    "runtime",
		Job:       "skill",
		ErrorCode: inv.ErrorCode,
		Message:   inv.ErrorMessage,
		Metadata:  make(map[string]string),
	}
	if inv.DurationMS > biz.TimeoutThresholdMS {
		report.Metadata["duration_ms"] = fmt.Sprintf("%d", inv.DurationMS)
	}
	if inv.InputPreview != "" {
		report.Metadata["input_preview_len"] = fmt.Sprintf("%d", len(inv.InputPreview))
	}
	if inv.SkillID != "" {
		report.Metadata["skill_id"] = inv.SkillID
	}

	result, err := a.inner.AnalyzeFromReport(ctx, report)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return &biz.RootCauseAnalysisResult{
		RootCause:  result.RootCause,
		FixSuggest: result.FixSuggest,
		Severity:   result.Severity,
		Confidence: result.Confidence,
	}, nil
}
