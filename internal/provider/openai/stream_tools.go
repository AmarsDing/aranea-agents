package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

type streamToolCallAcc struct {
	index     int
	id        string
	type_     string
	name      strings.Builder
	arguments strings.Builder
}

// generateStreamWithTools streams chat completions with tools, aggregating tool_calls deltas
// and emitting the same partial text deltas as the no-tools path when possible.
func (l *llm) generateStreamWithTools(ctx context.Context, req *model.LLMRequest) func(yield func(*model.LLMResponse, error) bool) {
	return func(yield func(*model.LLMResponse, error) bool) {
		messages, err := OpenAIMessagesFromContents(req.Contents)
		if err != nil {
			yield(nil, err)
			return
		}
		messages = SanitizeOpenAIChatMessagesToolSequence(messages)
		mid := modelName(req.Model, "")
		if mid == "" {
			yield(nil, fmt.Errorf("openai llm: LLMRequest.Model is required"))
			return
		}
		tools := OpenAIChatToolsFromRequest(req)
		body := map[string]any{
			"model":       mid,
			"messages":    messages,
			"temperature": temperatureFromConfig(req.Config),
			"stream":      true,
			"tools":       tools,
			"tool_choice": "auto",
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
		byIndex := map[int]*streamToolCallAcc{}

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
						Content          string          `json:"content"`
						ReasoningContent string          `json:"reasoning_content"`
						ToolCalls        json.RawMessage `json:"tool_calls"`
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
				yield(nil, fmt.Errorf("%s", chunk.Error.Message))
				return
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			d := chunk.Choices[0].Delta

			var rawCalls []map[string]any
			if len(d.ToolCalls) > 0 && string(d.ToolCalls) != "null" {
				if err := json.Unmarshal(d.ToolCalls, &rawCalls); err != nil {
					var one map[string]any
					if err2 := json.Unmarshal(d.ToolCalls, &one); err2 == nil && len(one) > 0 {
						rawCalls = []map[string]any{one}
					}
				}
			}
			for _, rc := range rawCalls {
				idxf, _ := rc["index"].(float64)
				idx := int(idxf)
				acc := byIndex[idx]
				if acc == nil {
					acc = &streamToolCallAcc{index: idx}
					byIndex[idx] = acc
				}
				if id, ok := rc["id"].(string); ok && id != "" {
					acc.id = id
				}
				if typ, ok := rc["type"].(string); ok && typ != "" {
					acc.type_ = typ
				}
				if fn, ok := rc["function"].(map[string]any); ok {
					if n, ok := fn["name"].(string); ok {
						acc.name.WriteString(n)
					}
					if a, ok := fn["arguments"].(string); ok {
						acc.arguments.WriteString(a)
					}
				}
			}

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

		msg := chatAssistantMessage{
			Content:          strings.TrimSpace(fullText.String()),
			ReasoningContent: nil,
		}
		if rs := strings.TrimSpace(fullReason.String()); rs != "" {
			b, _ := json.Marshal(rs)
			msg.ReasoningContent = b
		}
		if len(byIndex) > 0 {
			keys := slices.Sorted(maps.Keys(byIndex))
			for _, k := range keys {
				acc := byIndex[k]
				name := acc.name.String()
				argsStr := acc.arguments.String()
				msg.ToolCalls = append(msg.ToolCalls, struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				}{
					ID:   acc.id,
					Type: acc.type_,
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: name, Arguments: argsStr},
				})
			}
		}
		final, err := assistantMessageToLLMResponse(msg, mid, promptTok, completionTok)
		if err != nil {
			if len(byIndex) == 0 && (fullText.Len() > 0 || fullReason.Len() > 0) {
				final = &model.LLMResponse{
					Content:       assistantContent(fullText.String(), fullReason.String()),
					Partial:       false,
					TurnComplete:  true,
					ModelVersion:  mid,
					UsageMetadata: usageMeta(promptTok, completionTok),
				}
				err = nil
			}
		}
		if err != nil {
			yield(nil, err)
			return
		}
		_ = yield(final, nil)
	}
}
