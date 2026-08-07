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
	"net/http"
	"strings"

	"aranea-agents/internal/provider"

	"aranea-agents/pkg/apierror"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcprovider "trpc.group/trpc-go/trpc-agent-go/model/provider"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ProviderAPIConfig holds outbound HTTP credential hints deserialized from llm_provider_models.config_json.
type ProviderAPIConfig struct {
	ProviderType string `json:"provider_type"`
	APIBaseURL   string `json:"api_base_url"`
	APIKey       string `json:"api_key"`
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
	req := &trpcmodel.Request{
		Messages: openAICompatToTRPCMessages(messages),
		GenerationConfig: trpcmodel.GenerationConfig{
			Temperature: float64Ptr(0.7),
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

func CallOpenAICompatChatStream(ctx context.Context, hc *http.Client, cfg ProviderAPIConfig, modelName string, messages []OpenAICompatMessage, onDelta func(piece string) error) (fullText string, reasoningText string, promptTok, completionTok int, err error) {
	m, err := trpcCompatModel(cfg, hc, modelName)
	if err != nil {
		return "", "", 0, 0, err
	}
	req := &trpcmodel.Request{
		Messages: openAICompatToTRPCMessages(messages),
		GenerationConfig: trpcmodel.GenerationConfig{
			Temperature: float64Ptr(0.7),
			Stream:      true,
		},
	}
	respCh, err := m.GenerateContent(ctx, req)
	if err != nil {
		return "", "", 0, 0, err
	}
	var accText, accReason strings.Builder
	for resp := range respCh {
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
				if onDelta != nil {
					if err = onDelta(ch.Delta.Content); err != nil {
						return strings.TrimSpace(accText.String()), strings.TrimSpace(accReason.String()), promptTok, completionTok, err
					}
				}
			}
			if ch.Delta.ReasoningContent != "" {
				accReason.WriteString(ch.Delta.ReasoningContent)
			}
			continue
		}
		if ch.Message.Content != "" {
			accText.WriteString(ch.Message.Content)
		}
		if ch.Message.ReasoningContent != "" {
			accReason.WriteString(ch.Message.ReasoningContent)
		}
		if resp.Usage != nil {
			promptTok = resp.Usage.PromptTokens
			completionTok = resp.Usage.CompletionTokens
		}
	}
	// 流正常关闭但 ctx 已取消 = 超时截断：返回错误而非静默的部分结果。
	if err := contextError(ctx); err != nil {
		return strings.TrimSpace(accText.String()), strings.TrimSpace(accReason.String()), promptTok, completionTok, err
	}
	return strings.TrimSpace(accText.String()), strings.TrimSpace(accReason.String()), promptTok, completionTok, nil
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
	if hc != nil && hc.Transport != nil {
		opts = append(opts, trpcprovider.WithHTTPClientTransport(hc.Transport))
	}
	m, err := trpcprovider.Model(providerName, modelName, opts...)
	if err != nil {
		return nil, err
	}
	return provider.WrapModelWithMetrics(m, providerName, modelName), nil
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
	req := &trpcmodel.Request{
		Messages: openAICompatToTRPCMessages(messages),
		GenerationConfig: trpcmodel.GenerationConfig{
			Temperature: float64Ptr(0.7),
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
