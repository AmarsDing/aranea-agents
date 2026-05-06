package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// Option configures an LLM instance.
type Option func(*llm)

// WithName sets LLM.Name() (ADK uses this as the logical backend id).
func WithName(n string) Option {
	return func(l *llm) {
		if strings.TrimSpace(n) != "" {
			l.name = strings.TrimSpace(n)
		}
	}
}

type llm struct {
	hc      *http.Client
	baseURL string
	apiKey  string
	name    string
}

// NewLLM returns model.LLM backed by OpenAI-compatible POST /v1/chat/completions.
func NewLLM(hc *http.Client, baseURL, apiKey string, opts ...Option) model.LLM {
	if hc == nil {
		hc = http.DefaultClient
	}
	l := &llm{
		hc:      hc,
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
		name:    "openai",
	}
	for _, o := range opts {
		o(l)
	}
	return l
}

func (l *llm) Name() string { return l.name }

func (l *llm) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	if stream {
		return l.generateStream(ctx, req)
	}
	return func(yield func(*model.LLMResponse, error) bool) {
		resp, err := l.generate(ctx, req)
		yield(resp, err)
	}
}

func (l *llm) generate(ctx context.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("openai llm: nil LLMRequest")
	}
	if l.baseURL == "" || l.apiKey == "" {
		return nil, fmt.Errorf("openai llm: base URL and API key are required")
	}
	messages, err := OpenAIMessagesFromContents(req.Contents)
	if err != nil {
		return nil, err
	}
	mid := modelName(req.Model, "")
	if mid == "" {
		return nil, fmt.Errorf("openai llm: LLMRequest.Model is required")
	}
	body := map[string]any{
		"model":       mid,
		"messages":    messages,
		"temperature": temperatureFromConfig(req.Config),
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := l.baseURL + "/chat/completions"
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Authorization", "Bearer "+l.apiKey)

	resp, err := l.hc.Do(hreq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("provider HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var parsed struct {
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
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode provider response: %w", err)
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return nil, fmt.Errorf("%s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("empty choices from provider")
	}
	msg := parsed.Choices[0].Message
	text := strings.TrimSpace(msg.Content)
	reasoning := reasoningFromAPIRawJSON(msg.ReasoningContent)
	out := assistantContent(text, reasoning)
	return &model.LLMResponse{
		Content:       out,
		Partial:       false,
		TurnComplete:  true,
		ModelVersion:  mid,
		UsageMetadata: usageMeta(parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens),
	}, nil
}

func (l *llm) generateStream(ctx context.Context, req *model.LLMRequest) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if req == nil {
			yield(nil, fmt.Errorf("openai llm: nil LLMRequest"))
			return
		}
		if l.baseURL == "" || l.apiKey == "" {
			yield(nil, fmt.Errorf("openai llm: base URL and API key are required"))
			return
		}
		messages, err := OpenAIMessagesFromContents(req.Contents)
		if err != nil {
			yield(nil, err)
			return
		}
		mid := modelName(req.Model, "")
		if mid == "" {
			yield(nil, fmt.Errorf("openai llm: LLMRequest.Model is required"))
			return
		}
		body := map[string]any{
			"model":       mid,
			"messages":    messages,
			"temperature": temperatureFromConfig(req.Config),
			"stream":      true,
		}
		raw, err := json.Marshal(body)
		if err != nil {
			yield(nil, err)
			return
		}
		url := l.baseURL + "/chat/completions"
		hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			yield(nil, err)
			return
		}
		hreq.Header.Set("Content-Type", "application/json")
		hreq.Header.Set("Authorization", "Bearer "+l.apiKey)
		hreq.Header.Set("Accept", "text/event-stream")

		resp, err := l.hc.Do(hreq)
		if err != nil {
			yield(nil, err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
			yield(nil, fmt.Errorf("provider HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody))))
			return
		}

		var fullText, fullReason strings.Builder
		promptTok, completionTok := 0, 0
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 4<<20)

		for sc.Scan() {
			select {
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
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
			if json.Unmarshal([]byte(payload), &chunk) != nil {
				continue
			}
			if chunk.Error != nil && strings.TrimSpace(chunk.Error.Message) != "" {
				yield(nil, fmt.Errorf("%s", chunk.Error.Message))
				return
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			d := chunk.Choices[0].Delta
			if d.ReasoningContent != "" {
				fullReason.WriteString(d.ReasoningContent)
				rc := genai.NewContentFromParts([]*genai.Part{{Text: d.ReasoningContent, Thought: true}}, genai.RoleModel)
				if !yield(&model.LLMResponse{Content: rc, Partial: true, ModelVersion: mid}, nil) {
					return
				}
			}
			if d.Content != "" {
				fullText.WriteString(d.Content)
				if !yield(&model.LLMResponse{
					Content:      genai.NewContentFromText(d.Content, genai.RoleModel),
					Partial:      true,
					ModelVersion: mid,
				}, nil) {
					return
				}
			}
			if chunk.Usage != nil {
				promptTok = chunk.Usage.PromptTokens
				completionTok = chunk.Usage.CompletionTokens
			}
		}
		if err := sc.Err(); err != nil {
			yield(nil, err)
			return
		}
		final := assistantContent(strings.TrimSpace(fullText.String()), strings.TrimSpace(fullReason.String()))
		_ = yield(&model.LLMResponse{
			Content:       final,
			Partial:       false,
			TurnComplete:  true,
			ModelVersion:  mid,
			UsageMetadata: usageMeta(promptTok, completionTok),
		}, nil)
	}
}

func assistantContent(text, reasoning string) *genai.Content {
	text = strings.TrimSpace(text)
	reasoning = strings.TrimSpace(reasoning)
	if reasoning == "" {
		return genai.NewContentFromText(text, genai.RoleModel)
	}
	parts := []*genai.Part{}
	if text != "" {
		parts = append(parts, genai.NewPartFromText(text))
	}
	parts = append(parts, &genai.Part{Text: reasoning, Thought: true})
	return genai.NewContentFromParts(parts, genai.RoleModel)
}

func usageMeta(promptTok, completionTok int) *genai.GenerateContentResponseUsageMetadata {
	if promptTok <= 0 && completionTok <= 0 {
		return nil
	}
	return &genai.GenerateContentResponseUsageMetadata{
		PromptTokenCount:     int32(promptTok),
		CandidatesTokenCount: int32(completionTok),
	}
}
