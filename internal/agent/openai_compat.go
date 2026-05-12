package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"aranea-agents/internal/provider"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcprovider "trpc.group/trpc-go/trpc-agent-go/model/provider"
)

type OpenAICompatMessage struct {
	Role             string `json:"role"`
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
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
			out = append(out, trpcmodel.NewUserMessage(body))
		}
	}
	return out
}

func CallOpenAICompatChat(ctx context.Context, hc *http.Client, cfg ProviderAPIConfig, modelName string, messages []OpenAICompatMessage) (text string, reasoning string, promptTok, completionTok int, err error) {
	m, err := trpcCompatModel(cfg, hc)
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
			return "", "", 0, 0, kerrors.InternalServer("PROVIDER", resp.Error.Message)
		}
		last = resp
	}
	if last == nil {
		return "", "", 0, 0, kerrors.InternalServer("PROVIDER", "empty LLM response")
	}
	return extractFromTRPCResponse(last, modelName)
}

func CallOpenAICompatChatStream(ctx context.Context, hc *http.Client, cfg ProviderAPIConfig, modelName string, messages []OpenAICompatMessage, onDelta func(piece string) error) (fullText string, reasoningText string, promptTok, completionTok int, err error) {
	m, err := trpcCompatModel(cfg, hc)
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
			return strings.TrimSpace(accText.String()), strings.TrimSpace(accReason.String()), promptTok, completionTok, kerrors.InternalServer("PROVIDER", resp.Error.Message)
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
	return strings.TrimSpace(accText.String()), strings.TrimSpace(accReason.String()), promptTok, completionTok, nil
}

func trpcCompatModel(cfg ProviderAPIConfig, hc *http.Client) (trpcmodel.Model, error) {
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
	return trpcprovider.Model(providerName, "", opts...)
}

func extractFromTRPCResponse(resp *trpcmodel.Response, _ string) (text string, reasoning string, promptTok, completionTok int, err error) {
	if len(resp.Choices) == 0 {
		return "", "", 0, 0, kerrors.InternalServer("PROVIDER", "empty choices")
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
