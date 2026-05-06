package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"
)

const maxNativeToolRounds = 12

type openAIToolCallPayload struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIAssistantChoice struct {
	Role             string                  `json:"role"`
	Content          json.RawMessage         `json:"content"`
	ReasoningContent json.RawMessage         `json:"reasoning_content"`
	ToolCalls        []openAIToolCallPayload `json:"tool_calls"`
}

func openAIMessagesFromCompat(msgs []OpenAICompatMessage) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		row := map[string]any{"role": m.Role, "content": m.Content}
		if strings.TrimSpace(m.ReasoningContent) != "" {
			row["reasoning_content"] = m.ReasoningContent
		}
		out = append(out, row)
	}
	return out
}

func rawJSONStringOrEmpty(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func assistantPayloadForHistory(msg openAIAssistantChoice) map[string]any {
	out := map[string]any{"role": "assistant"}
	text := rawJSONStringOrEmpty(msg.Content)
	if len(msg.ToolCalls) > 0 {
		arr := make([]map[string]any, 0, len(msg.ToolCalls))
		for _, tc := range msg.ToolCalls {
			arr = append(arr, map[string]any{
				"id":   tc.ID,
				"type": tc.Type,
				"function": map[string]any{
					"name":      tc.Function.Name,
					"arguments": tc.Function.Arguments,
				},
			})
		}
		out["tool_calls"] = arr
		if text != "" {
			out["content"] = text
		} else {
			out["content"] = nil
		}
		if r := reasoningFromAPIRawJSON(msg.ReasoningContent); r != "" {
			out["reasoning_content"] = r
		}
		return out
	}
	out["content"] = text
	if r := reasoningFromAPIRawJSON(msg.ReasoningContent); r != "" {
		out["reasoning_content"] = r
	}
	return out
}

func effectiveNativeFilesystemToolKeys(ctx context.Context, d Deps, ag biz.Agent) ([]string, error) {
	if d.AgentUC == nil {
		return nil, nil
	}
	eff, err := d.AgentUC.GetEffectiveTools(ctx, ag.ID)
	if err != nil {
		return nil, err
	}
	if !eff.ToolsEnabled {
		return nil, nil
	}
	native := map[string]struct{}{
		"read_file": {}, "list_files": {}, "write_file": {}, "edit_file": {},
	}
	var keys []string
	for _, it := range eff.Items {
		if !it.Enabled {
			continue
		}
		if _, ok := native[it.ToolKey]; ok {
			keys = append(keys, it.ToolKey)
		}
	}
	return keys, nil
}

func stripToolNamePrefix(name, prefix string) string {
	name = strings.TrimSpace(name)
	if p := strings.TrimSpace(prefix); p != "" {
		name = strings.TrimPrefix(name, p)
	}
	return strings.TrimSpace(name)
}

func openAIChatCompletionNonStream(ctx context.Context, hc *http.Client, baseURL, apiKey, model string, body map[string]any) (choice openAIAssistantChoice, finishReason string, promptTok, completionTok int, err error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	apiKey = strings.TrimSpace(apiKey)
	model = strings.TrimSpace(model)
	endpoint := baseURL + "/chat/completions"
	raw, err := json.Marshal(body)
	if err != nil {
		return openAIAssistantChoice{}, "", 0, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return openAIAssistantChoice{}, "", 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := hc.Do(req)
	if err != nil {
		return openAIAssistantChoice{}, "", 0, 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return openAIAssistantChoice{}, "", 0, 0, err
	}

	var parsed struct {
		Choices []struct {
			FinishReason string                `json:"finish_reason"`
			Message      openAIAssistantChoice `json:"message"`
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
		return openAIAssistantChoice{}, "", 0, 0, fmt.Errorf("decode provider response: %w", err)
	}
	if parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
		return openAIAssistantChoice{}, "", 0, 0, fmt.Errorf("%s", parsed.Error.Message)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return openAIAssistantChoice{}, "", 0, 0, fmt.Errorf("provider HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if len(parsed.Choices) == 0 {
		return openAIAssistantChoice{}, "", 0, 0, fmt.Errorf("empty choices from provider")
	}
	return parsed.Choices[0].Message, parsed.Choices[0].FinishReason, parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens, nil
}

func emitBufferedDeltas(stream StreamEmitter, text string) error {
	if stream == nil || text == "" {
		return nil
	}
	const chunk = 64
	runes := []rune(text)
	for i := 0; i < len(runes); i += chunk {
		j := i + chunk
		if j > len(runes) {
			j = len(runes)
		}
		if err := stream.Emit("delta", map[string]string{"content": string(runes[i:j])}); err != nil {
			return err
		}
	}
	return nil
}

// RunOpenAICompatWithNativeTools calls /chat/completions with tools and runs filesystem tools until the model returns plain text.
// failedRound is the loop index where a provider/error occurred, or -1 on success.
func RunOpenAICompatWithNativeTools(ctx context.Context, hc *http.Client, baseURL, apiKey, model, toolNamePrefix string, messages []map[string]any, tools []map[string]any, stream StreamEmitter) (finalText string, finalReasoning string, promptTok, completionTok int, failedRound int, err error) {
	failedRound = -1
	msgs := append([]map[string]any{}, messages...)
	var pt, ct int
	for round := 0; round < maxNativeToolRounds; round++ {
		body := map[string]any{
			"model":       model,
			"messages":    msgs,
			"temperature": 0.7,
			"tools":       tools,
			"tool_choice": "auto",
		}
		msg, finish, pAdd, cAdd, err := openAIChatCompletionNonStream(ctx, hc, baseURL, apiKey, model, body)
		if err != nil {
			return "", "", pt, ct, round, err
		}
		pt += pAdd
		ct += cAdd

		if len(msg.ToolCalls) == 0 {
			finalText = strings.TrimSpace(rawJSONStringOrEmpty(msg.Content))
			finalReasoning = reasoningFromAPIRawJSON(msg.ReasoningContent)
			_ = finish
			if stream != nil && finalText != "" {
				if err := emitBufferedDeltas(stream, finalText); err != nil {
					return finalText, finalReasoning, pt, ct, -1, err
				}
			}
			return finalText, finalReasoning, pt, ct, -1, nil
		}

		msgs = append(msgs, assistantPayloadForHistory(msg))
		for _, tc := range msg.ToolCalls {
			name := stripToolNamePrefix(tc.Function.Name, toolNamePrefix)
			outMap, execErr := executeNativeFilesystemTool(name, tc.Function.Arguments)
			var payload string
			if execErr != nil {
				b, _ := json.Marshal(map[string]any{"ok": false, "error": execErr.Error()})
				payload = string(b)
			} else {
				b, mErr := json.Marshal(outMap)
				if mErr != nil {
					payload = fmt.Sprintf(`{"ok":false,"error":"encode: %s"}`, mErr.Error())
				} else {
					payload = string(b)
				}
			}
			toolID := strings.TrimSpace(tc.ID)
			if toolID == "" {
				toolID = fmt.Sprintf("call_%d", round)
			}
			msgs = append(msgs, map[string]any{
				"role":         "tool",
				"tool_call_id": toolID,
				"content":      payload,
			})
		}
	}
	return "", "", pt, ct, maxNativeToolRounds, fmt.Errorf("native tool loop exceeded %d rounds", maxNativeToolRounds)
}

// shouldFallbackChatWithoutTools returns true when the provider likely rejected OpenAI-style tools (common with partial compat APIs).
func shouldFallbackChatWithoutTools(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "tool_choice") {
		return true
	}
	if strings.Contains(s, "empty choices") {
		return true
	}
	if strings.Contains(s, "tools") && (strings.Contains(s, "not support") || strings.Contains(s, "unsupported") ||
		strings.Contains(s, "invalid") || strings.Contains(s, "unknown") || strings.Contains(s, "unexpected")) {
		return true
	}
	if strings.Contains(s, "function") && (strings.Contains(s, "not support") || strings.Contains(s, "unsupported")) {
		return true
	}
	if strings.Contains(s, "http 400") || strings.Contains(s, "http 422") {
		return true
	}
	return false
}

// CompleteOpenAIModelReply runs streaming or non-streaming completion; when filesystem tools are allowed, uses a tool loop (non-streaming upstream, optional buffered deltas to stream).
func CompleteOpenAIModelReply(ctx context.Context, d Deps, cfg providerAPIConfig, model string, ag biz.Agent, oaMsgs []OpenAICompatMessage, stream StreamEmitter) (reply string, reasoning string, tin, tout int, err error) {
	toolKeys, err := effectiveNativeFilesystemToolKeys(ctx, d, ag)
	if err != nil {
		return "", "", 0, 0, err
	}
	enabled := map[string]bool{}
	for _, k := range toolKeys {
		enabled[k] = true
	}
	tools := nativeToolDefinitionsForKeys(enabled)
	prefix := ""
	if ag.Settings != nil {
		prefix = strings.TrimSpace(ag.Settings.ToolsToolCallPrefix)
	}
	if len(tools) > 0 {
		messages := openAIMessagesFromCompat(oaMsgs)
		reply, reasoning, tin, tout, failRound, toolErr := RunOpenAICompatWithNativeTools(ctx, d.HTTP, cfg.APIBaseURL, cfg.APIKey, model, prefix, messages, tools, stream)
		if toolErr != nil && failRound == 0 && shouldFallbackChatWithoutTools(toolErr) {
			if stream != nil {
				return CallOpenAICompatChatStream(ctx, d.HTTP, cfg, model, oaMsgs, func(piece string) error {
					return stream.Emit("delta", map[string]string{"content": piece})
				})
			}
			return CallOpenAICompatChat(ctx, d.HTTP, cfg, model, oaMsgs)
		}
		return reply, reasoning, tin, tout, toolErr
	}
	if stream != nil {
		return CallOpenAICompatChatStream(ctx, d.HTTP, cfg, model, oaMsgs, func(piece string) error {
			return stream.Emit("delta", map[string]string{"content": piece})
		})
	}
	return CallOpenAICompatChat(ctx, d.HTTP, cfg, model, oaMsgs)
}
