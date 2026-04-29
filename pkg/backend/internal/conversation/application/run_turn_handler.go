package application

import (
	mem "arenea/backend/internal/memory/domain"

	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	catalogapp "arenea/backend/internal/catalog/application"
	adkr "arenea/backend/internal/conversation/adapters/adkruntime"
	"arenea/backend/internal/domain"
	"arenea/backend/internal/kernel/runctx"
	memapp "arenea/backend/internal/memory/application"
	"arenea/backend/internal/repository"
)

type ChatService struct {
	repo           repository.Store
	runtime        *adkr.ADKRuntimeAdapter
	teamRunEvents  *TeamRunEventBroker
	memoryL0       *memapp.MemoryL0Service
	memoryL1       *memapp.MemoryL1Service
	memoryL2       *memapp.MemoryL2Service
	memoryL3       *memapp.MemoryL3Service
	memoryL4       *memapp.MemoryL4Service
	agentEvolution *catalogapp.AgentEvolutionService
}

type SendMessageInput struct {
	SessionID string             `json:"session_id"`
	AgentKey  string             `json:"agent_key"`
	TeamID    string             `json:"team_id"`
	Content   string             `json:"content"`
	Options   SendMessageOptions `json:"options"`
}

type SendMessageOptions struct {
	DialogMode  string               `json:"dialog_mode"`
	Provider    string               `json:"provider"`
	Model       string               `json:"model"`
	Attachments []AttachmentRefInput `json:"attachments"`
}

type AttachmentRefInput struct {
	ID string `json:"id"`
}

type SendMessageResult struct {
	UserMessage  domain.Message `json:"user_message"`
	AgentMessage domain.Message `json:"agent_message"`
}

type SendStreamCallbacks struct {
	OnUserMessage       func(domain.Message) error
	OnDelta             func(string) error
	OnAgentMessage      func(domain.Message) error
	OnToolEvent         func(adkr.ToolEvent) error
	OnTeamMemberStart   func(domain.Message) error
	OnTeamMemberDelta   func(messageID string, delta string) error
	OnTeamMemberMessage func(domain.Message) error
}

func NewChatService(repo repository.Store, runtimeAdapter *adkr.ADKRuntimeAdapter) *ChatService {
	memoryL0 := memapp.NewMemoryL0Service(repo)
	memoryL1 := memapp.NewMemoryL1Service(repo)
	memoryL2 := memapp.NewMemoryL2Service(repo)
	memoryL3 := memapp.NewMemoryL3Service(repo)
	memoryL4 := memapp.NewMemoryL4Service(repo)
	agentEvolution := catalogapp.NewAgentEvolutionService(repo)
	memoryL2.SetL1Source(memoryL1)
	memoryL2.SetL4ExtractionSource(memoryL4)
	memoryL3.SetL4ExtractionSource(memoryL4)
	memoryL0.SetL1Source(memoryL1)
	memoryL0.SetL2Source(memoryL2)
	memoryL0.SetL3Source(memoryL3)
	memoryL0.SetL4Source(memoryL4)
	memoryL0.SetEvolutionSource(agentEvolution)
	return &ChatService{
		repo:           repo,
		runtime:        runtimeAdapter,
		teamRunEvents:  NewTeamRunEventBroker(),
		memoryL0:       memoryL0,
		memoryL1:       memoryL1,
		memoryL2:       memoryL2,
		memoryL3:       memoryL3,
		memoryL4:       memoryL4,
		agentEvolution: agentEvolution,
	}
}

// MemoryL0 暴露 L0 组装服务，供 HTTP 处理程序提供
// 预览/快照端点，而无需在 main.go 中重新接线依赖。
func (s *ChatService) MemoryL0() *memapp.MemoryL0Service { return s.memoryL0 }

// MemoryL1 暴露 L1 工作记忆服务，供 HTTP 处理程序提供
// 任务/字段端点，而无需在 main.go 中重新接线依赖。
func (s *ChatService) MemoryL1() *memapp.MemoryL1Service { return s.memoryL1 }

// MemoryL2 暴露 L2 情节记忆服务，供 HTTP 处理程序提供
// 情节/事件/标记端点，而无需重新接线依赖。
func (s *ChatService) MemoryL2() *memapp.MemoryL2Service { return s.memoryL2 }

// MemoryL3 暴露 L3 语义记忆服务，供 HTTP 处理程序与
// 衰减任务调用 Upsert / Recall / RunDecayBatch，而无需在 main.go 中重新接线仓库。
func (s *ChatService) MemoryL3() *memapp.MemoryL3Service { return s.memoryL3 }

// MemoryL4 暴露 L4 持久记忆/知识图谱服务，供 HTTP 处理程序提供
// 实体/关系/邻域端点，而无需在 main.go 中重新接线仓库。
func (s *ChatService) MemoryL4() *memapp.MemoryL4Service { return s.memoryL4 }

// AgentEvolution 暴露 L4 自进化服务，供 HTTP 处理程序提供
// 身份/策略/提案/事件端点，而无需在 main.go 中重新接线仓库。
func (s *ChatService) AgentEvolution() *catalogapp.AgentEvolutionService { return s.agentEvolution }

func (s *ChatService) Send(ctx context.Context, in SendMessageInput) (SendMessageResult, error) {
	if in.SessionID == "" || in.Content == "" {
		return SendMessageResult{}, validationError("session_id and content are required")
	}
	session, err := s.repo.GetSessionByID(in.SessionID)
	if err != nil {
		return SendMessageResult{}, err
	}
	if session.OwnerType == "team" {
		return s.sendTeam(ctx, in, session, nil)
	}
	if in.AgentKey == "" {
		return SendMessageResult{}, validationError("agent_key is required")
	}
	agent, err := s.repo.GetAgentByKey(in.AgentKey)
	if err != nil {
		return SendMessageResult{}, err
	}
	if session.OwnerType != "" && session.OwnerType != "agent" {
		return SendMessageResult{}, validationError("chat send currently requires an agent-owned session")
	}
	if session.AgentID != agent.ID {
		return SendMessageResult{}, conflictError("session does not belong to agent")
	}
	provider, model := resolveProviderModel(in.Options, session, agent)
	providerModel, err := s.repo.GetProviderModel(provider, model)
	if err != nil {
		return SendMessageResult{}, err
	}
	optionsJSON := ""
	if in.Options.DialogMode != "" || in.Options.Provider != "" || in.Options.Model != "" || len(in.Options.Attachments) > 0 {
		raw, err := json.Marshal(in.Options)
		if err != nil {
			return SendMessageResult{}, err
		}
		optionsJSON = string(raw)
	}

	userMsg := domain.Message{
		ID:               newID(),
		SessionID:        in.SessionID,
		Role:             "user",
		Content:          in.Content,
		ModelName:        "",
		Status:           "ok",
		AttachmentsCount: len(in.Options.Attachments),
		OptionsJSON:      optionsJSON,
	}
	userMsg, err = s.repo.AddMessage(userMsg)
	if err != nil {
		return SendMessageResult{}, err
	}

	modelMessages, l0Result, err := s.assembleL0Prompt(ctx, in, session, agent, providerModel, userMsg)
	if err != nil {
		return SendMessageResult{}, err
	}

	generated, err := s.runtime.Generate(ctx, adkr.GenerateRequest{
		Agent:          agent,
		ProviderModel:  providerModel,
		Messages:       modelMessages,
		Input:          in.Content,
		ToolSettings:   s.runtimeToolSettings(agent.ID),
		RuntimeContext: s.singleAgentRuntimeContext(session, agent, in.Options),
		OnToolEvent: func(event adkr.ToolEvent) error {
			s.recordToolEvent(in.SessionID, "", event)
			return nil
		},
	})
	if err != nil {
		_ = s.recordModelTokenUsage(agent, session, providerModel, in.Options, adkr.GenerateResult{}, domain.Message{}, false, "failed", err)
		return SendMessageResult{}, err
	}

	agentMsg := domain.Message{
		ID:          newID(),
		SessionID:   in.SessionID,
		Role:        "assistant",
		Content:     generated.Content,
		ModelName:   generated.ModelName,
		TokenIn:     generated.PromptTokens,
		TokenOut:    generated.CompletionTokens,
		LatencyMS:   generated.LatencyMS,
		Status:      "ok",
		OptionsJSON: agentMessageOptions(agent),
	}
	agentMsg, err = s.repo.AddMessage(agentMsg)
	if err != nil {
		return SendMessageResult{}, err
	}
	_ = s.recordModelTokenUsage(agent, session, providerModel, in.Options, generated, agentMsg, false, "success", nil)
	_ = s.updateSessionContextRatio(in.SessionID, agent, providerModel, generated)
	_ = s.recordL0Actual(ctx, in.SessionID, agent, providerModel, l0Result, generated)
	_ = s.recordProviderModelTPS(providerModel, generated)

	return SendMessageResult{
		UserMessage:  userMsg,
		AgentMessage: agentMsg,
	}, nil
}

func (s *ChatService) SendStream(ctx context.Context, in SendMessageInput, callbacks SendStreamCallbacks) error {
	if in.SessionID == "" || in.Content == "" {
		return validationError("session_id and content are required")
	}
	session, err := s.repo.GetSessionByID(in.SessionID)
	if err != nil {
		return err
	}
	if session.OwnerType == "team" {
		_, err = s.sendTeam(ctx, in, session, &callbacks)
		return err
	}
	if in.AgentKey == "" {
		return validationError("agent_key is required")
	}
	agent, err := s.repo.GetAgentByKey(in.AgentKey)
	if err != nil {
		return err
	}
	if session.OwnerType != "" && session.OwnerType != "agent" {
		return validationError("chat send currently requires an agent-owned session")
	}
	if session.AgentID != agent.ID {
		return conflictError("session does not belong to agent")
	}
	provider, model := resolveProviderModel(in.Options, session, agent)
	providerModel, err := s.repo.GetProviderModel(provider, model)
	if err != nil {
		return err
	}
	optionsJSON := ""
	if in.Options.DialogMode != "" || in.Options.Provider != "" || in.Options.Model != "" || len(in.Options.Attachments) > 0 {
		raw, err := json.Marshal(in.Options)
		if err != nil {
			return err
		}
		optionsJSON = string(raw)
	}

	userMsg := domain.Message{
		ID:               newID(),
		SessionID:        in.SessionID,
		Role:             "user",
		Content:          in.Content,
		Status:           "ok",
		AttachmentsCount: len(in.Options.Attachments),
		OptionsJSON:      optionsJSON,
	}
	userMsg, err = s.repo.AddMessage(userMsg)
	if err != nil {
		return err
	}
	if callbacks.OnUserMessage != nil {
		if err = callbacks.OnUserMessage(userMsg); err != nil {
			return err
		}
	}

	modelMessages, l0Result, err := s.assembleL0Prompt(ctx, in, session, agent, providerModel, userMsg)
	if err != nil {
		return err
	}

	generated, err := s.runtime.StreamGenerate(ctx, adkr.GenerateRequest{
		Agent:          agent,
		ProviderModel:  providerModel,
		Messages:       modelMessages,
		Input:          in.Content,
		ToolSettings:   s.runtimeToolSettings(agent.ID),
		RuntimeContext: s.singleAgentRuntimeContext(session, agent, in.Options),
		OnToolEvent: func(event adkr.ToolEvent) error {
			s.recordToolEvent(in.SessionID, "", event)
			if callbacks.OnToolEvent != nil {
				return callbacks.OnToolEvent(event)
			}
			return nil
		},
	}, callbacks.OnDelta)
	if err != nil {
		status := "failed"
		if errors.Is(err, context.Canceled) {
			status = "cancelled"
		}
		_ = s.recordModelTokenUsage(agent, session, providerModel, in.Options, adkr.GenerateResult{}, domain.Message{}, true, status, err)
		return err
	}

	agentMsg := domain.Message{
		ID:          newID(),
		SessionID:   in.SessionID,
		Role:        "assistant",
		Content:     generated.Content,
		ModelName:   generated.ModelName,
		TokenIn:     generated.PromptTokens,
		TokenOut:    generated.CompletionTokens,
		LatencyMS:   generated.LatencyMS,
		Status:      "ok",
		OptionsJSON: agentMessageOptions(agent),
	}
	agentMsg, err = s.repo.AddMessage(agentMsg)
	if err != nil {
		return err
	}
	_ = s.recordModelTokenUsage(agent, session, providerModel, in.Options, generated, agentMsg, true, "success", nil)
	_ = s.updateSessionContextRatio(in.SessionID, agent, providerModel, generated)
	_ = s.recordL0Actual(ctx, in.SessionID, agent, providerModel, l0Result, generated)
	_ = s.recordProviderModelTPS(providerModel, generated)
	if callbacks.OnAgentMessage != nil {
		return callbacks.OnAgentMessage(agentMsg)
	}
	return nil
}

func agentMessageOptions(agent domain.Agent) string {
	raw, err := json.Marshal(map[string]any{
		"agent": map[string]string{
			"agent_id":  agent.ID,
			"agent_key": agent.AgentKey,
			"name":      firstNonEmptyString(agent.DisplayName, agent.AgentKey, agent.ID),
			"icon":      agent.Icon,
		},
	})
	if err != nil {
		return ""
	}
	return string(raw)
}

func (s *ChatService) runtimeToolSettings(agentID string) *domain.AgentRuntimeSettings {
	if strings.TrimSpace(agentID) == "" {
		return nil
	}
	settings, err := s.repo.GetAgentRuntimeSettings(agentID)
	if err != nil {
		return nil
	}
	return &settings
}

// singleAgentRuntimeContext 构建结构化运行时上下文，
// 在非团队对话中渲染到智能体系统提示中。上下文向模型说明
// 当前会话、启用的对话模式，以及以单智能体角色运行。
func (s *ChatService) singleAgentRuntimeContext(session domain.Session, agent domain.Agent, options SendMessageOptions) *runctx.RuntimeContext {
	dialogMode := strings.TrimSpace(options.DialogMode)
	if dialogMode == "" {
		dialogMode = strings.TrimSpace(session.DialogMode)
	}
	return &runctx.RuntimeContext{
		Session: runctx.SessionContext{
			SessionID:  session.ID,
			DialogMode: dialogMode,
			StartedAt:  session.CreatedAt,
		},
		SelfRole: "single_agent",
	}
}

func (s *ChatService) recordToolEvent(sessionID string, messageID string, event adkr.ToolEvent) {
	if event.Phase != "after" {
		return
	}
	metadata, _ := json.Marshal(event)
	_, _ = s.repo.InsertToolInvocation(domain.ToolInvocation{
		InvocationID:     event.ID,
		ToolKey:          event.ToolName,
		ToolDisplayName:  event.ToolLabel,
		AgentID:          event.AgentID,
		AgentKey:         event.AgentKey,
		AgentDisplayName: event.AgentName,
		SessionID:        sessionID,
		MessageID:        messageID,
		Source:           "adk",
		Status:           event.Status,
		StartedAt:        toolStartedAt(event),
		EndedAt:          event.OccurredAt,
		DurationMS:       event.DurationMS,
		InputPreview:     previewJSON(event.Arguments, 300),
		OutputPreview:    previewJSON(event.Result, 300),
		ErrorMessage:     event.Error,
		MetadataJSON:     string(metadata),
	})
}

func toolStartedAt(event adkr.ToolEvent) string {
	if event.DurationMS <= 0 || event.OccurredAt == "" {
		return ""
	}
	ended, err := time.Parse(time.RFC3339Nano, event.OccurredAt)
	if err != nil {
		return ""
	}
	return ended.Add(-time.Duration(event.DurationMS) * time.Millisecond).UTC().Format(time.RFC3339Nano)
}

func previewJSON(value any, limit int) string {
	if value == nil {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	text := string(raw)
	if limit <= 0 || len([]rune(text)) <= limit {
		return text
	}
	return string([]rune(text)[:limit]) + "..."
}

// assembleL0Prompt 通过 MemoryL0Service 构建提示，并将结果转换为
// 运行时适配器的 ChatMessage 形态。若退回到原始「历史+用户」提示会把 L0 逻辑泄露到 ChatService，
// 因此任何 L0 失败都向上传递。
func (s *ChatService) assembleL0Prompt(ctx context.Context, in SendMessageInput, session domain.Session, agent domain.Agent, providerModel domain.PlatformResource, userMsg domain.Message) ([]adkr.ChatMessage, mem.L0AssemblyResult, error) {
	if s.memoryL0 == nil {
		s.memoryL0 = memapp.NewMemoryL0Service(s.repo)
	}
	s.ensureL1Task(ctx, session, agent, in.Content)
	contextWindow := providerContextWindowTokens(providerModel, agent)
	req := mem.L0AssemblyRequest{
		SessionID:         in.SessionID,
		AgentID:           agent.ID,
		TeamID:            session.TeamID,
		Provider:          providerModel.Provider,
		Model:             providerModel.Model,
		ContextWindow:     contextWindow,
		ReservedForOutput: 0,
		UserMessage:       in.Content,
		UserMessageID:     userMsg.ID,
	}
	result, err := s.memoryL0.Assemble(ctx, req)
	if err != nil {
		return nil, mem.L0AssemblyResult{}, err
	}
	messages := make([]adkr.ChatMessage, 0, len(result.PromptMessages))
	for _, m := range result.PromptMessages {
		messages = append(messages, adkr.ChatMessage{Role: m.Role, Content: m.Content})
	}
	return messages, result, nil
}

// recordL0Actual 在模型调用成功后闭环，将真实提示词 token 用量写回
// 快照与会话行。此为尽力而为路径：快照/会话更新不得影响用户可见请求，
// 因此调用方用 `_ = ...` 包裹。
func (s *ChatService) recordL0Actual(ctx context.Context, sessionID string, agent domain.Agent, providerModel domain.PlatformResource, l0Result mem.L0AssemblyResult, generated adkr.GenerateResult) error {
	if s.memoryL0 == nil {
		return nil
	}
	contextWindow := providerContextWindowTokens(providerModel, agent)
	actual := generated.PromptTokens
	if actual <= 0 {
		actual = l0Result.PromptTokenEstimate
	}
	return s.memoryL0.RecordActual(ctx, sessionID, l0Result.SnapshotID, actual, contextWindow)
}

func resolveProviderModel(options SendMessageOptions, session domain.Session, agent domain.Agent) (string, string) {
	if options.Provider != "" && options.Model != "" {
		return options.Provider, options.Model
	}
	if session.Provider != "" && session.Model != "" {
		return session.Provider, session.Model
	}
	return agent.Provider, agent.Model
}

// RouteAgentModelCandidates 按智能体自进化的模型偏好（§5.9 ResolveModelRouting）
// 对给定的 (provider, model) 候选排序。尚无偏好记录时原样返回。
// 供后续回退/重试逻辑使用，并对测试暴露。
func (s *ChatService) RouteAgentModelCandidates(ctx context.Context, agentID string, candidates []catalogapp.ModelCandidate) ([]catalogapp.ModelCandidate, error) {
	if s.agentEvolution == nil || agentID == "" || len(candidates) == 0 {
		return candidates, nil
	}
	return s.agentEvolution.ResolveModelRouting(ctx, agentID, candidates)
}

func (s *ChatService) updateSessionContextRatio(sessionID string, agent domain.Agent, providerModel domain.PlatformResource, generated adkr.GenerateResult) error {
	contextTokens := providerContextWindowTokens(providerModel, agent)
	if sessionID == "" || contextTokens <= 0 || generated.PromptTokens <= 0 {
		return nil
	}
	ratio := float64(generated.PromptTokens) / float64(contextTokens)
	if ratio < 0 || math.IsInf(ratio, 0) || math.IsNaN(ratio) {
		return nil
	}
	if ratio > 1 {
		ratio = 1
	}
	return s.repo.UpdateSessionContextUsedRatio(sessionID, ratio)
}

func providerContextWindowTokens(providerModel domain.PlatformResource, agent domain.Agent) int {
	type config struct {
		ContextWindowK int `json:"context_window_k"`
	}
	var cfg config
	if providerModel.ConfigJSON != "" {
		_ = json.Unmarshal([]byte(providerModel.ConfigJSON), &cfg)
	}
	if cfg.ContextWindowK > 0 {
		return cfg.ContextWindowK * 1000
	}
	return agent.ContextWindow
}

func (s *ChatService) recordModelTokenUsage(agent domain.Agent, session domain.Session, providerModel domain.PlatformResource, options SendMessageOptions, generated adkr.GenerateResult, message domain.Message, streamEnabled bool, status string, callErr error) error {
	cfg := parseUsageProviderConfig(providerModel.ConfigJSON)
	occurredAt := nowUTC()
	pricing, err := s.repo.GetActiveModelPricingRule(providerModel.Provider, providerModel.Model, occurredAt)
	if err != nil {
		return err
	}

	inputTokens := generated.PromptTokens
	outputTokens := generated.CompletionTokens
	totalTokens := inputTokens + outputTokens
	inputCost := costMicroUSD(inputTokens, pricing.InputPriceMicroUSDPer1K)
	outputCost := costMicroUSD(outputTokens, pricing.OutputPriceMicroUSDPer1K)
	tps := 0.0
	if generated.CompletionTokens > 0 && generated.LatencyMS > 0 {
		tps = math.Round((float64(generated.CompletionTokens)/(float64(generated.LatencyMS)/1000))*100) / 100
	}
	errorMessage := ""
	if callErr != nil {
		errorMessage = callErr.Error()
	}
	if status == "" {
		status = "success"
	}
	if status == "failed" && errors.Is(callErr, context.DeadlineExceeded) {
		status = "timeout"
	}

	event := domain.ModelTokenUsageEvent{
		ID:                            newID(),
		OccurredAt:                    occurredAt,
		DateKey:                       occurredAt[:10],
		HourKey:                       occurredAt[:13] + ":00",
		TeamID:                        session.TeamID,
		AgentID:                       agent.ID,
		AgentKey:                      agent.AgentKey,
		SessionID:                     session.ID,
		MessageID:                     message.ID,
		ProviderCode:                  providerModel.Provider,
		ProviderType:                  cfg.ProviderType,
		ProviderDisplayName:           firstNonEmptyString(cfg.ProviderDisplayName, providerModel.Provider),
		ModelAPIID:                    providerModel.Model,
		ModelDisplayName:              firstNonEmptyString(providerModel.Name, providerModel.Model),
		ModelCategoryJSON:             cfg.ModelCategoryJSON,
		UsageKind:                     "chat",
		CallCount:                     1,
		InputTokens:                   inputTokens,
		OutputTokens:                  outputTokens,
		TotalTokens:                   totalTokens,
		InputPriceMicroUSDPer1K:       pricing.InputPriceMicroUSDPer1K,
		OutputPriceMicroUSDPer1K:      pricing.OutputPriceMicroUSDPer1K,
		InputCostMicroUSD:             inputCost,
		OutputCostMicroUSD:            outputCost,
		TotalCostMicroUSD:             inputCost + outputCost,
		LatencyMS:                     generated.LatencyMS,
		TokensPerSecond:               tps,
		Status:                        status,
		ErrorMessage:                  errorMessage,
		PromptMode:                    options.DialogMode,
		MaxOutputTokens:               cfg.MaxOutputTokens,
		ContextWindowK:                cfg.ContextWindowK,
		StreamEnabled:                 streamEnabled,
		MetadataJSON:                  "{}",
		CreatedAt:                     occurredAt,
		CachedInputPriceMicroUSDPer1K: pricing.CachedInputPriceMicroUSDPer1K,
		ReasoningPriceMicroUSDPer1K:   pricing.ReasoningPriceMicroUSDPer1K,
		EmbeddingPriceMicroUSDPer1K:   pricing.EmbeddingPriceMicroUSDPer1K,
	}
	event, err = s.repo.AddModelTokenUsageEvent(event)
	if err != nil {
		return err
	}
	return s.repo.UpsertModelTokenUsageDaily(event)
}

type usageProviderConfig struct {
	ProviderType        string `json:"provider_type"`
	ProviderDisplayName string `json:"provider_display_name"`
	ModelCategoryJSON   string
	ContextWindowK      int `json:"context_window_k"`
	MaxOutputTokens     int `json:"max_output_tokens"`
}

func parseUsageProviderConfig(raw string) usageProviderConfig {
	var body map[string]any
	cfg := usageProviderConfig{ModelCategoryJSON: "[]"}
	if raw == "" {
		return cfg
	}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return cfg
	}
	if value, ok := body["provider_type"].(string); ok {
		cfg.ProviderType = value
	}
	if value, ok := body["provider_display_name"].(string); ok {
		cfg.ProviderDisplayName = value
	}
	if value, ok := body["context_window_k"].(float64); ok {
		cfg.ContextWindowK = int(value)
	}
	if value, ok := body["max_output_tokens"].(float64); ok {
		cfg.MaxOutputTokens = int(value)
	}
	if value, ok := body["model_category"]; ok {
		if rawCategory, err := json.Marshal(value); err == nil {
			cfg.ModelCategoryJSON = string(rawCategory)
		}
	}
	return cfg
}

func costMicroUSD(tokens int, priceMicroUSDPer1K int64) int64 {
	if tokens <= 0 || priceMicroUSDPer1K <= 0 {
		return 0
	}
	return int64(tokens) * priceMicroUSDPer1K / 1000
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func previewText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes]) + "..."
}

func (s *ChatService) recordProviderModelTPS(providerModel domain.PlatformResource, generated adkr.GenerateResult) error {
	if providerModel.ID == "" || generated.CompletionTokens <= 0 || generated.LatencyMS <= 0 {
		return nil
	}
	tps := float64(generated.CompletionTokens) / (float64(generated.LatencyMS) / 1000)
	if tps <= 0 || math.IsInf(tps, 0) || math.IsNaN(tps) {
		return nil
	}

	config := map[string]any{}
	if providerModel.ConfigJSON != "" {
		if err := json.Unmarshal([]byte(providerModel.ConfigJSON), &config); err != nil {
			return err
		}
	}
	config["tokens_per_second"] = math.Round(tps*100) / 100
	raw, err := json.Marshal(config)
	if err != nil {
		return err
	}
	providerModel.ConfigJSON = string(raw)
	_, err = s.repo.UpdatePlatformResource(providerModel)
	return err
}

// ensureL1Task 在会话首条用户消息上启动默认 L1 任务，否则为空操作。
// task_goal 用首条消息播种，使 L0 渲染器从首轮起显示合理标题。
// 失败通过审计轨迹记录（尽力而为），不阻塞聊天路径，因 L1 为补充记忆而非硬依赖。
func (s *ChatService) ensureL1Task(ctx context.Context, session domain.Session, agent domain.Agent, userInput string) {
	if s.memoryL1 == nil || session.ID == "" || agent.ID == "" {
		return
	}
	settings, err := s.repo.GetAgentRuntimeSettings(agent.ID)
	if err == nil && !settings.L1Enabled {
		return
	}
	taskGoal := strings.TrimSpace(userInput)
	if existing, err := s.repo.GetL1TaskByKey(session.ID, "default", agent.ID); err == nil {
		if existing.Status.IsTerminal() {
			_, _ = s.memoryL1.StartTask(ctx, memapp.StartL1TaskInput{
				SessionID: session.ID,
				AgentID:   agent.ID,
				TeamID:    session.TeamID,
				TaskKey:   "default",
				TaskGoal:  taskGoal,
			})
		}
		return
	}
	_, _ = s.memoryL1.StartTask(ctx, memapp.StartL1TaskInput{
		SessionID: session.ID,
		AgentID:   agent.ID,
		TeamID:    session.TeamID,
		TaskKey:   "default",
		TaskGoal:  taskGoal,
	})
}

// EndSessionL1Tasks 将会话中所有活动任务标为已完成。
// 由会话归档流程/监控定时任务调用，避免会话关闭后未结束任务仍渗入提示。
//
// 若配置了 L2 服务，每个已结束任务还会归档到 `memory_episodes` 行
// （规范 §5.4「L1 任务 EndTask → ArchiveL1Task」）。L2 归档为尽力而为：失败不阻塞 L1 关闭路径。
func (s *ChatService) EndSessionL1Tasks(ctx context.Context, sessionID string, status mem.L1TaskStatus) {
	if s.memoryL1 == nil || sessionID == "" {
		return
	}
	if !status.IsTerminal() {
		status = mem.L1TaskCompleted
	}
	tasks, err := s.repo.ListL1TasksBySession(mem.L1TaskListQuery{SessionID: sessionID, IncludeEnded: false})
	if err != nil {
		return
	}
	for _, t := range tasks {
		if err := s.memoryL1.EndTask(ctx, t.ID, status); err != nil {
			continue
		}
		if s.memoryL2 != nil {
			_, _ = s.memoryL2.ArchiveL1Task(ctx, t.ID)
		}
	}
}

func (s *ChatService) ListMessages(sessionID string) ([]domain.Message, error) {
	return s.repo.ListMessages(sessionID)
}

func (s *ChatService) ListOptions(optionType string) ([]domain.ChatOption, error) {
	return s.repo.ListChatOptions(optionType)
}
