package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"aranea-agents/internal/provider"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"google.golang.org/genai"
)

// OpenAICompatMessage is one chat completion message (OpenAI-compatible).
// ReasoningContent is required by some providers (e.g. DeepSeek thinking mode) on follow-up turns.
type OpenAICompatMessage struct {
	Role             string `json:"role"`
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

func openAICompatToContents(msgs []OpenAICompatMessage) []*genai.Content {
	out := make([]*genai.Content, 0, len(msgs))
	for _, m := range msgs {
		body := strings.TrimSpace(m.Content)
		re := strings.TrimSpace(m.ReasoningContent)
		switch strings.ToLower(strings.TrimSpace(m.Role)) {
		case "system":
			c := genai.NewContentFromText(body, genai.RoleUser)
			c.Role = "system"
			out = append(out, c)
			continue
		case "assistant":
			if re != "" {
				parts := []*genai.Part{genai.NewPartFromText(body)}
				parts = append(parts, &genai.Part{Text: re, Thought: true})
				out = append(out, genai.NewContentFromParts(parts, genai.RoleModel))
				continue
			}
			out = append(out, genai.NewContentFromText(body, genai.RoleModel))
			continue
		default:
			out = append(out, genai.NewContentFromText(body, genai.RoleUser))
		}
	}
	return out
}

// CallOpenAICompatChat uses ADK-style model.LLM.GenerateContent (non-stream).
func CallOpenAICompatChat(ctx context.Context, hc *http.Client, cfg ProviderAPIConfig, model string, messages []OpenAICompatMessage) (text string, reasoning string, promptTok, completionTok int, err error) {
	cc := provider.CatalogFromEndpoints(cfg.ProviderType, cfg.APIBaseURL, cfg.APIKey)
	llm, err := provider.DefaultRegistry().Resolve(cc, &provider.RoundTrip{HTTP: hc})
	if err != nil {
		return "", "", 0, 0, err
	}
	req := &provider.LLMRequest{
		Model:    strings.TrimSpace(model),
		Contents: openAICompatToContents(messages),
		Config:   provider.DefaultChatConfig(),
	}
	var last *provider.LLMResponse
	for resp, e := range llm.GenerateContent(ctx, req, false) {
		if e != nil {
			return "", "", 0, 0, e
		}
		last = resp
	}
	if last == nil {
		return "", "", 0, 0, kerrors.InternalServer("PROVIDER", "empty LLM response")
	}
	text, reasoning = provider.TextsFromLLMResponse(last)
	text = strings.TrimSpace(text)
	reasoning = strings.TrimSpace(reasoning)
	promptTok, completionTok = provider.UsageFromLLMResponse(last)
	return text, reasoning, promptTok, completionTok, nil
}

// CallOpenAICompatChatStream uses model.LLM.GenerateContent with stream=true (SSE), forwarding text deltas.
func CallOpenAICompatChatStream(ctx context.Context, hc *http.Client, cfg ProviderAPIConfig, model string, messages []OpenAICompatMessage, onDelta func(piece string) error) (fullText string, reasoningText string, promptTok, completionTok int, err error) {
	cc := provider.CatalogFromEndpoints(cfg.ProviderType, cfg.APIBaseURL, cfg.APIKey)
	llm, err := provider.DefaultRegistry().Resolve(cc, &provider.RoundTrip{HTTP: hc})
	if err != nil {
		return "", "", 0, 0, err
	}
	req := &provider.LLMRequest{
		Model:    strings.TrimSpace(model),
		Contents: openAICompatToContents(messages),
		Config:   provider.DefaultChatConfig(),
	}
	var accText, accReason strings.Builder
	var finalMain, finalReason string
	for resp, e := range llm.GenerateContent(ctx, req, true) {
		if e != nil {
			return strings.TrimSpace(accText.String()), strings.TrimSpace(accReason.String()), promptTok, completionTok, e
		}
		if resp == nil {
			continue
		}
		m, r := provider.TextsFromLLMResponse(resp)
		if resp.Partial {
			if m != "" {
				accText.WriteString(m)
				if onDelta != nil {
					if err = onDelta(m); err != nil {
						return strings.TrimSpace(accText.String()), strings.TrimSpace(accReason.String()), promptTok, completionTok, err
					}
				}
			}
			if r != "" {
				accReason.WriteString(r)
			}
			continue
		}
		finalMain, finalReason = m, r
		promptTok, completionTok = provider.UsageFromLLMResponse(resp)
	}
	outMain := strings.TrimSpace(finalMain)
	if outMain == "" {
		outMain = strings.TrimSpace(accText.String())
	}
	outReason := strings.TrimSpace(finalReason)
	if outReason == "" {
		outReason = strings.TrimSpace(accReason.String())
	}
	return outMain, outReason, promptTok, completionTok, nil
}

// reasoningFromAPIRawJSON normalizes provider reasoning_content (string or JSON-encoded string).
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
