// Package llmcompat provides OpenAI-compatible LLM chat helpers extracted from
// the agent package to break import cycles.
//
// BR10 note: this package intentionally builds lightweight models via
// trpcprovider.Model + WrapModelWithMetrics for side-channel calls (intent,
// prompt helpers) that only have inline ProviderAPIConfig (base URL + API key).
// Full catalog path (HA / retry / CB / decrypt) remains TRPCModelForProviderModel
// for Agent runtime turns. Do not expand this bypass for primary chat/team runs.
package llmcompat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"aranea-agents/internal/provider"

	"aranea-agents/pkg/apierror"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcprovider "trpc.group/trpc-go/trpc-agent-go/model/provider"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// streamIdleTimeout 是流式调用的停滞窗口：任一响应帧（增量/聚合/错误）到达
// 即重置看门狗；窗口内无任何帧判定为停滞。包级变量供测试覆盖。
//
// P3 设计：有数据流动的流不设总时长上限（取代旧的 60s 硬超时——它会掐断
// DeepSeek 推理段等健康慢流）。45s 覆盖高负载下的 TTFT 与帧间隔抖动。
var streamIdleTimeout = 45 * time.Second

// StreamIdleError 表示流式调用停滞：超过 streamIdleTimeout 未收到任何帧。
// 调用方应将其分类为瞬时故障（可重试）。
type StreamIdleError struct {
	Timeout time.Duration
}

func (e *StreamIdleError) Error() string {
	return fmt.Sprintf("llm stream idle: no frame received for %s", e.Timeout)
}

// ProviderAPIConfig holds outbound HTTP credential hints deserialized from llm_provider_models.config_json.
type ProviderAPIConfig struct {
	ProviderType string `json:"provider_type"`
	APIBaseURL   string `json:"api_base_url"`
	APIKey       string `json:"api_key"`
	// ThinkingDisabled 为 true 时对支持 thinking 开关的 provider（DeepSeek v4 等）
	// 显式关闭推理段（注入 thinking.type=disabled）。默认 false = 不注入该字段，
	// 保留 provider 服务端默认行为。
	ThinkingDisabled bool `json:"thinking_disabled"`
	// ThinkingEffort 是按次调用的思考强度路由决策（P2-5，off/low/medium/high/max）。
	// 不来自 config_json（json:"-"），由调用方按 biz.ResolveThinkingEffort 设置。
	// 显式 effort 优先于 ThinkingDisabled；"off" 映射 ThinkingEnabled=false，其余
	// 档位注入 reasoning_effort 且不动 thinking 开关。
	ThinkingEffort string `json:"-"`
	// ThinkingCapExplicit / SupportsThinking 来自模型目录 capability_thinking。
	// 显式 false（Ollama qwen2.5vl 等）时不得下发 thinking key，否则 400。
	ThinkingCapExplicit bool `json:"-"`
	SupportsThinking    bool `json:"-"`
}

// ApplyThinkingCapability copies catalog capability_thinking onto cfg so
// thinkingFieldsFromConfig can skip the thinking key for models that reject it.
func ApplyThinkingCapability(cfg *ProviderAPIConfig, explicit, thinking bool) {
	if cfg == nil || !explicit {
		return
	}
	cfg.ThinkingCapExplicit = true
	cfg.SupportsThinking = thinking
}

// MergeProviderConfigJSON overlays JSON config from LlmProviderModel.ConfigJSON.
func MergeProviderConfigJSON(raw string, out *ProviderAPIConfig) {
	var c ProviderAPIConfig
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &c) != nil {
		return
	}
	if out.ProviderType == "" {
		out.ProviderType = c.ProviderType
	}
	if out.APIBaseURL == "" {
		out.APIBaseURL = c.APIBaseURL
	}
	if out.APIKey == "" {
		out.APIKey = c.APIKey
	}
	if c.ThinkingDisabled {
		out.ThinkingDisabled = true
	}
}

// StreamCallbacks 拆分流式增量回调：内容段与推理段（reasoning_content）独立
// 上送，使调用方（如 TaskPlanner）能将思考过程实时发布给用户。零值安全——
// 不关心推理流的调用方传 StreamCallbacks{} 即可。
type StreamCallbacks struct {
	OnContent   func(piece string) error
	OnReasoning func(piece string) error
}

type OpenAICompatMessage struct {
	Role             string `json:"role"`
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	// ContentParts 是多模态内容（图片等）；非空时优先于 Content 进入 trpc 请求。
	ContentParts []trpcmodel.ContentPart `json:"-"`
}

func openAICompatToTRPCMessages(msgs []OpenAICompatMessage) []trpcmodel.Message {
	out := make([]trpcmodel.Message, 0, len(msgs))
	for _, m := range msgs {
		body := strings.TrimSpace(m.Content)
		re := strings.TrimSpace(m.ReasoningContent)
		switch strings.ToLower(strings.TrimSpace(m.Role)) {
		case "system":
			out = append(out, trpcmodel.NewSystemMessage(body))
		case "assistant":
			msg := trpcmodel.NewAssistantMessage(body)
			if re != "" {
				msg.ReasoningContent = re
			}
			out = append(out, msg)
		default:
			if len(m.ContentParts) > 0 {
				msg := trpcmodel.NewUserMessage(body)
				msg.ContentParts = m.ContentParts
				out = append(out, msg)
				continue
			}
			out = append(out, trpcmodel.NewUserMessage(body))
		}
	}
	return out
}

// providerResponseError wraps a provider error preserving the original
// *trpcmodel.ResponseError in the error chain (apierror.Wrap sets Cause), so
// upstream retry classification can inspect Type/Code via errors.As.
func providerResponseError(respErr *trpcmodel.ResponseError) error {
	if respErr == nil {
		return nil
	}
	return apierror.Wrap(respErr, apierror.CodeInternal, apierror.DomainProvider)
}

// contextError returns the wrapped ctx error when the call context is done
// (deadline/cancel), nil otherwise.
//
// 00:52 会话根因（B1+B2）：openai.go 在 ctx 取消时不保证发射错误响应——
// 非流式路径直接 silent return；流式路径的错误响应与 ctx.Done() 竞态可被
// 丢弃——导致 LLM 超时后被吞成「空结果 + nil error」（TaskPlanner 分解静默
// 产空，用户 60s 无反馈）。所有调用方必须在流结束后显式校验 ctx.Err()。
func contextError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return apierror.Wrap(err, apierror.CodeUnavailable, apierror.DomainProvider)
	}
	return nil
}

func CallOpenAICompatChat(ctx context.Context, hc *http.Client, cfg ProviderAPIConfig, modelName string, messages []OpenAICompatMessage) (fullText string, reasoningText string, promptTok, completionTok int, err error) {
	m, err := trpcCompatModel(cfg, hc, modelName)
	if err != nil {
		return "", "", 0, 0, err
	}
	thinkingEnabled, reasoningEffort := thinkingFieldsFromConfig(cfg)
	req := &trpcmodel.Request{
		Messages: openAICompatToTRPCMessages(messages),
		GenerationConfig: trpcmodel.GenerationConfig{
			Temperature:     float64Ptr(0.7),
			ThinkingEnabled: thinkingEnabled,
			ReasoningEffort: reasoningEffort,
		},
	}
	respCh, err := m.GenerateContent(ctx, req)
	if err != nil {
		return "", "", 0, 0, err
	}
	var last *trpcmodel.Response
	for resp := range respCh {
		if resp.Error != nil {
			// ctx 错误优先：超时/取消时框架发射的 stream error 只是次生症状。
			if err := contextError(ctx); err != nil {
				return "", "", 0, 0, err
			}
			return "", "", 0, 0, providerResponseError(resp.Error)
		}
		last = resp
	}
	if err := contextError(ctx); err != nil {
		return "", "", 0, 0, err
	}
	if last == nil {
		return "", "", 0, 0, apierror.Internal(apierror.DomainProvider, "empty LLM response")
	}
	return extractFromTRPCResponse(last, modelName)
}

func CallOpenAICompatChatStream(ctx context.Context, hc *http.Client, cfg ProviderAPIConfig, modelName string, messages []OpenAICompatMessage, callbacks StreamCallbacks) (fullText string, reasoningText string, promptTok, completionTok int, err error) {
	m, err := trpcCompatModel(cfg, hc, modelName)
	if err != nil {
		return "", "", 0, 0, err
	}
	thinkingEnabled, reasoningEffort := thinkingFieldsFromConfig(cfg)
	req := &trpcmodel.Request{
		Messages: openAICompatToTRPCMessages(messages),
		GenerationConfig: trpcmodel.GenerationConfig{
			Temperature:     float64Ptr(0.7),
			Stream:          true,
			ThinkingEnabled: thinkingEnabled,
			ReasoningEffort: reasoningEffort,
		},
	}
	respCh, err := m.GenerateContent(ctx, req)
	if err != nil {
		return "", "", 0, 0, err
	}
	var accText, accReason strings.Builder
	// finalText/finalReason 来自流结束时框架发射的聚合响应（IsPartial=false，
	// Message 为全量累积内容）。部分增量已计入 accText/accReason，聚合响应
	// 必须作为权威结果整体替换，否则内容翻倍。
	var finalText, finalReason string
	var haveFinal bool

	// P3 停滞看门狗：任一帧到达即重置；窗口内无任何帧判定停滞并返回
	// *StreamIdleError（取代旧的固定总超时——有数据流动的慢流不设上限）。
	idleTimer := time.NewTimer(streamIdleTimeout)
	defer idleTimer.Stop()
	resetIdleTimer := func() {
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(streamIdleTimeout)
	}

loop:
	for {
		select {
		case <-idleTimer.C:
			return strings.TrimSpace(accText.String()), strings.TrimSpace(accReason.String()), promptTok, completionTok, &StreamIdleError{Timeout: streamIdleTimeout}
		case <-ctx.Done():
			return strings.TrimSpace(accText.String()), strings.TrimSpace(accReason.String()), promptTok, completionTok, contextError(ctx)
		case resp, ok := <-respCh:
			if !ok {
				break loop
			}
			resetIdleTimer()
			if resp.Error != nil {
				// ctx 错误优先：超时/取消时框架发射的 stream error 只是次生症状。
				if err := contextError(ctx); err != nil {
					return strings.TrimSpace(accText.String()), strings.TrimSpace(accReason.String()), promptTok, completionTok, err
				}
				return strings.TrimSpace(accText.String()), strings.TrimSpace(accReason.String()), promptTok, completionTok, providerResponseError(resp.Error)
			}
			if len(resp.Choices) == 0 {
				continue
			}
			ch := resp.Choices[0]
			if resp.IsPartial {
				if ch.Delta.Content != "" {
					accText.WriteString(ch.Delta.Content)
					if callbacks.OnContent != nil {
						if err = callbacks.OnContent(ch.Delta.Content); err != nil {
							return strings.TrimSpace(accText.String()), strings.TrimSpace(accReason.String()), promptTok, completionTok, err
						}
					}
				}
				if ch.Delta.ReasoningContent != "" {
					accReason.WriteString(ch.Delta.ReasoningContent)
					if callbacks.OnReasoning != nil {
						if err = callbacks.OnReasoning(ch.Delta.ReasoningContent); err != nil {
							return strings.TrimSpace(accText.String()), strings.TrimSpace(accReason.String()), promptTok, completionTok, err
						}
					}
				}
				continue
			}
			if ch.Message.Content != "" {
				finalText = ch.Message.Content
				haveFinal = true
			}
			if ch.Message.ReasoningContent != "" {
				finalReason = ch.Message.ReasoningContent
			}
			if resp.Usage != nil {
				promptTok = resp.Usage.PromptTokens
				completionTok = resp.Usage.CompletionTokens
			}
		}
	}
	// 流正常关闭但 ctx 已取消 = 超时截断：返回错误而非静默的部分结果。
	if err := contextError(ctx); err != nil {
		return strings.TrimSpace(accText.String()), strings.TrimSpace(accReason.String()), promptTok, completionTok, err
	}
	full, reason := accText.String(), accReason.String()
	if haveFinal {
		full = finalText
		if finalReason != "" {
			reason = finalReason
		}
	}
	return strings.TrimSpace(full), strings.TrimSpace(reason), promptTok, completionTok, nil
}

func trpcCompatModel(cfg ProviderAPIConfig, hc *http.Client, modelName string) (trpcmodel.Model, error) {
	providerName := provider.MapProviderType(cfg.ProviderType)
	opts := []trpcprovider.Option{}
	if apiKey := strings.TrimSpace(cfg.APIKey); apiKey != "" {
		opts = append(opts, trpcprovider.WithAPIKey(apiKey))
	}
	if baseURL := strings.TrimSpace(cfg.APIBaseURL); baseURL != "" {
		opts = append(opts, trpcprovider.WithBaseURL(baseURL))
	}
	// 显式传递 variant（如 deepseek），否则 openai 模型仅靠 baseURL 推断，
	// 自定义网关/代理 URL 会退化为 OpenAI variant，导致 thinking 开关等
	// variant 特有行为（{"thinking":{"type":"disabled"}}）丢失。
	if v := provider.InferVariant(provider.ProviderModelConfig{ProviderType: cfg.ProviderType, BaseURL: cfg.APIBaseURL}); v != "" {
		opts = append(opts, trpcprovider.WithVariant(v))
	}
	if hc != nil && hc.Transport != nil {
		opts = append(opts, trpcprovider.WithHTTPClientTransport(hc.Transport))
	}
	m, err := trpcprovider.Model(providerName, modelName, opts...)
	if err != nil {
		return nil, err
	}
	return provider.WrapModelWithMetrics(m, providerName, modelName), nil
}

func skipThinkingKey(cfg ProviderAPIConfig) bool {
	if cfg.ThinkingCapExplicit {
		return !cfg.SupportsThinking
	}
	return strings.EqualFold(strings.TrimSpace(cfg.ProviderType), "ollama")
}

// thinkingFieldsFromConfig 将（cfg.ThinkingEffort, cfg.ThinkingDisabled）映射为
// 框架 GenerationConfig 的（ThinkingEnabled, ReasoningEffort）：
//   - 显式 effort="off"        → ThinkingEnabled=*false（等价 thinking_disabled）
//   - 显式 effort=low/medium/high/max → ReasoningEffort=&v（不动 thinking 开关，
//     保留 provider 服务端默认 enabled 行为；DeepSeek 服务端把 low/medium 映射
//     为 high——见框架 request.go 注释，本层只透传归一化档位）
//   - 未给 effort              → 回落 ThinkingDisabled 静态配置
//   - 两者皆无                 → (nil, nil)，请求体不携带 thinking 相关字段
//
// 显式 effort 优先于 ThinkingDisabled（与 P2-1「显式路由策略覆盖静态配置」同约）。
// capability_thinking=false / Ollama 默认跳过 thinking key（P-01）。
func thinkingFieldsFromConfig(cfg ProviderAPIConfig) (thinkingEnabled *bool, reasoningEffort *string) {
	if skipThinkingKey(cfg) {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(cfg.ThinkingEffort)) {
	case "off":
		disabled := false
		return &disabled, nil
	case "low", "medium", "high", "max":
		v := strings.ToLower(strings.TrimSpace(cfg.ThinkingEffort))
		return nil, &v
	}
	if cfg.ThinkingDisabled {
		disabled := false
		return &disabled, nil
	}
	return nil, nil
}

func extractFromTRPCResponse(resp *trpcmodel.Response, _ string) (text string, reasoning string, promptTok, completionTok int, err error) {
	if len(resp.Choices) == 0 {
		return "", "", 0, 0, apierror.Internal(apierror.DomainProvider, "empty choices")
	}
	ch := resp.Choices[0]
	text = strings.TrimSpace(ch.Message.Content)
	reasoning = strings.TrimSpace(ch.Message.ReasoningContent)
	if resp.Usage != nil {
		promptTok = resp.Usage.PromptTokens
		completionTok = resp.Usage.CompletionTokens
	}
	return text, reasoning, promptTok, completionTok, nil
}

func reasoningFromAPIRawJSON(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(strings.Trim(string(raw), `"`))
}

func float64Ptr(v float64) *float64 { return &v }

type OpenAICompatToolCallResult struct {
	Name      string
	Arguments string
}

func CallOpenAICompatChatWithTools(ctx context.Context, hc *http.Client, cfg ProviderAPIConfig, modelName string, messages []OpenAICompatMessage, tools []map[string]any) (text string, toolCalls []OpenAICompatToolCallResult, promptTok, completionTok int, err error) {
	m, err := trpcCompatModel(cfg, hc, modelName)
	if err != nil {
		return "", nil, 0, 0, err
	}
	toolDecls := make(map[string]trpctool.Tool, len(tools))
	for _, t := range tools {
		name, _ := t["name"].(string)
		desc, _ := t["description"].(string)
		params, _ := t["parameters"].(map[string]any)
		decl := &trpctool.Declaration{
			Name:        name,
			Description: desc,
		}
		if params != nil {
			schemaBytes, _ := json.Marshal(params)
			var schema trpctool.Schema
			if json.Unmarshal(schemaBytes, &schema) == nil {
				decl.InputSchema = &schema
			}
		}
		toolDecls[name] = &staticToolDecl{decl: decl}
	}
	thinkingEnabled, reasoningEffort := thinkingFieldsFromConfig(cfg)
	req := &trpcmodel.Request{
		Messages: openAICompatToTRPCMessages(messages),
		GenerationConfig: trpcmodel.GenerationConfig{
			Temperature:     float64Ptr(0.7),
			ThinkingEnabled: thinkingEnabled,
			ReasoningEffort: reasoningEffort,
		},
		Tools: toolDecls,
	}
	respCh, err := m.GenerateContent(ctx, req)
	if err != nil {
		return "", nil, 0, 0, err
	}
	var last *trpcmodel.Response
	for resp := range respCh {
		if resp.Error != nil {
			// ctx 错误优先：超时/取消时框架发射的 stream error 只是次生症状。
			if err := contextError(ctx); err != nil {
				return "", nil, 0, 0, err
			}
			return "", nil, 0, 0, providerResponseError(resp.Error)
		}
		last = resp
	}
	if err := contextError(ctx); err != nil {
		return "", nil, 0, 0, err
	}
	if last == nil {
		return "", nil, 0, 0, apierror.Internal(apierror.DomainProvider, "empty LLM response")
	}
	if len(last.Choices) == 0 {
		return "", nil, 0, 0, apierror.Internal(apierror.DomainProvider, "empty choices")
	}
	ch := last.Choices[0]
	text = strings.TrimSpace(ch.Message.Content)
	for _, tc := range ch.Message.ToolCalls {
		toolCalls = append(toolCalls, OpenAICompatToolCallResult{
			Name:      tc.Function.Name,
			Arguments: string(tc.Function.Arguments),
		})
	}
	if last.Usage != nil {
		promptTok = last.Usage.PromptTokens
		completionTok = last.Usage.CompletionTokens
	}
	return text, toolCalls, promptTok, completionTok, nil
}

type staticToolDecl struct {
	decl *trpctool.Declaration
}

func (s *staticToolDecl) Declaration() *trpctool.Declaration { return s.decl }
