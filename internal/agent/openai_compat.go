package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OpenAICompatMessage is one chat completion message (OpenAI-compatible).
// ReasoningContent is required by some providers (e.g. DeepSeek thinking mode) on follow-up turns.
type OpenAICompatMessage struct {
	Role             string `json:"role"`
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
}

type openAIChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content          string          `json:"content"`
			ReasoningContent json.RawMessage `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// CallOpenAICompatChat posts to {baseURL}/chat/completions.
func CallOpenAICompatChat(ctx context.Context, hc *http.Client, baseURL, apiKey, model string, messages []OpenAICompatMessage) (text string, reasoning string, promptTok, completionTok int, err error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	model = strings.TrimSpace(model)
	if baseURL == "" || apiKey == "" {
		return "", "", 0, 0, fmt.Errorf("api_base_url and api_key are required on the provider model")
	}
	if model == "" {
		return "", "", 0, 0, fmt.Errorf("model is required")
	}
	endpoint := baseURL + "/chat/completions"
	body := map[string]any{
		"model":       model,
		"messages":    messages,
		"temperature": 0.7,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", "", 0, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", "", 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := hc.Do(req)
	if err != nil {
		return "", "", 0, 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", "", 0, 0, err
	}

	var parsed openAIChatCompletionResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", "", 0, 0, fmt.Errorf("decode provider response: %w", err)
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return "", "", 0, 0, fmt.Errorf("%s", parsed.Error.Message)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", 0, 0, fmt.Errorf("provider HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if len(parsed.Choices) == 0 {
		return "", "", 0, 0, fmt.Errorf("empty choices from provider")
	}
	msg := parsed.Choices[0].Message
	text = strings.TrimSpace(msg.Content)
	reasoning = reasoningFromAPIRawJSON(msg.ReasoningContent)
	return text, reasoning, parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens, nil
}

// CallOpenAICompatChatStream posts stream: true and forwards text deltas via onDelta (OpenAI SSE).
func CallOpenAICompatChatStream(ctx context.Context, hc *http.Client, baseURL, apiKey, model string, messages []OpenAICompatMessage, onDelta func(piece string) error) (fullText string, reasoningText string, promptTok, completionTok int, err error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	model = strings.TrimSpace(model)
	if baseURL == "" || apiKey == "" {
		return "", "", 0, 0, fmt.Errorf("api_base_url and api_key are required on the provider model")
	}
	if model == "" {
		return "", "", 0, 0, fmt.Errorf("model is required")
	}
	if hc == nil {
		hc = http.DefaultClient
	}
	endpoint := baseURL + "/chat/completions"
	body := map[string]any{
		"model":       model,
		"messages":    messages,
		"temperature": 0.7,
		"stream":      true,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", "", 0, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", "", 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := hc.Do(req)
	if err != nil {
		return "", "", 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return "", "", 0, 0, fmt.Errorf("provider HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var b strings.Builder
	var reasoningB strings.Builder
	sc := bufio.NewScanner(resp.Body)
	// Allow long lines from SSE JSON payloads.
	sc.Buffer(make([]byte, 64*1024), 4<<20)

	for sc.Scan() {
		select {
		case <-ctx.Done():
			return b.String(), strings.TrimSpace(reasoningB.String()), promptTok, completionTok, ctx.Err()
		default:
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		const prefix = "data:"
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if payload == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					ReasoningContent string `json:"reasoning_content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		if chunk.Error != nil && strings.TrimSpace(chunk.Error.Message) != "" {
			return b.String(), strings.TrimSpace(reasoningB.String()), promptTok, completionTok, fmt.Errorf("%s", chunk.Error.Message)
		}
		if len(chunk.Choices) > 0 {
			d := chunk.Choices[0].Delta
			if d.ReasoningContent != "" {
				reasoningB.WriteString(d.ReasoningContent)
			}
			if d.Content != "" {
				b.WriteString(d.Content)
				if onDelta != nil {
					if err := onDelta(d.Content); err != nil {
						return b.String(), strings.TrimSpace(reasoningB.String()), promptTok, completionTok, err
					}
				}
			}
		}
		if chunk.Usage != nil {
			promptTok = chunk.Usage.PromptTokens
			completionTok = chunk.Usage.CompletionTokens
		}
	}
	if err := sc.Err(); err != nil {
		return b.String(), strings.TrimSpace(reasoningB.String()), promptTok, completionTok, err
	}
	return b.String(), strings.TrimSpace(reasoningB.String()), promptTok, completionTok, nil
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
