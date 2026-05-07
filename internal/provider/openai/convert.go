package openai

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// OpenAIChatToolsFromRequest extracts genai function declarations from the ADK request for Chat Completions `tools`.
func OpenAIChatToolsFromRequest(req *model.LLMRequest) []map[string]any {
	if req == nil || req.Config == nil {
		return nil
	}
	return openAIToolsFromGenAIConfig(req.Config)
}

func openAIToolsFromGenAIConfig(cfg *genai.GenerateContentConfig) []map[string]any {
	if cfg == nil {
		return nil
	}
	var decls []*genai.FunctionDeclaration
	seen := map[string]bool{}
	for _, t := range cfg.Tools {
		if t == nil || len(t.FunctionDeclarations) == 0 {
			continue
		}
		for _, d := range t.FunctionDeclarations {
			if d == nil || strings.TrimSpace(d.Name) == "" {
				continue
			}
			name := strings.TrimSpace(d.Name)
			if seen[name] {
				continue
			}
			seen[name] = true
			decls = append(decls, d)
		}
	}
	if len(decls) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(decls))
	for _, d := range decls {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        d.Name,
				"description": d.Description,
				"parameters":  functionDeclarationParameters(d),
			},
		})
	}
	return out
}

func functionDeclarationParameters(d *genai.FunctionDeclaration) map[string]any {
	if d == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	if d.ParametersJsonSchema != nil {
		var m map[string]any
		b, err := json.Marshal(d.ParametersJsonSchema)
		if err == nil && json.Unmarshal(b, &m) == nil && m != nil {
			return sanitizeJSONSchemaForOpenAITools(m)
		}
	}
	if d.Parameters != nil {
		b, err := json.Marshal(d.Parameters)
		if err == nil {
			var m map[string]any
			if json.Unmarshal(b, &m) == nil && m != nil {
				return sanitizeJSONSchemaForOpenAITools(m)
			}
		}
	}
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

// sanitizeJSONSchemaForOpenAITools removes draft metadata some backends reject in Chat Completions "parameters".
func sanitizeJSONSchemaForOpenAITools(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	delete(m, "$schema")
	delete(m, "$id")
	delete(m, "definitions")
	delete(m, "$defs")
	delete(m, "unevaluatedProperties")
	if props, ok := m["properties"].(map[string]any); ok {
		for k, v := range props {
			if child, ok := v.(map[string]any); ok {
				props[k] = sanitizeJSONSchemaForOpenAITools(child)
			}
		}
	}
	if items, ok := m["items"].(map[string]any); ok {
		m["items"] = sanitizeJSONSchemaForOpenAITools(items)
	}
	return m
}

func openAIChatRole(genaiRole string) string {
	r := strings.TrimSpace(strings.ToLower(genaiRole))
	switch r {
	case "", string(genai.RoleUser):
		return "user"
	case string(genai.RoleModel), "assistant":
		return "assistant"
	case "system":
		return "system"
	default:
		return "user"
	}
}

// OpenAIMessagesFromContents maps ADK/genai turns into OpenAI /chat/completions message JSON objects,
// including assistant tool_calls and tool result messages.
func OpenAIMessagesFromContents(contents []*genai.Content) ([]map[string]any, error) {
	if len(contents) == 0 {
		return nil, fmt.Errorf("openai llm: empty Contents")
	}
	out := make([]map[string]any, 0, len(contents)+4)
	for _, c := range contents {
		if c == nil || len(c.Parts) == 0 {
			continue
		}
		role := openAIChatRole(c.Role)
		var texts []string
		var reasoning []string
		var calls []*genai.FunctionCall
		var responses []*genai.FunctionResponse
		for _, p := range c.Parts {
			if p == nil {
				continue
			}
			if p.FunctionCall != nil {
				calls = append(calls, p.FunctionCall)
				continue
			}
			if p.FunctionResponse != nil {
				responses = append(responses, p.FunctionResponse)
				continue
			}
			if strings.TrimSpace(p.Text) == "" {
				continue
			}
			if p.Thought {
				reasoning = append(reasoning, p.Text)
				continue
			}
			texts = append(texts, p.Text)
		}

		if len(responses) > 0 {
			if role == "user" && len(texts) > 0 {
				msg := map[string]any{"role": "user", "content": strings.Join(texts, "\n")}
				if len(reasoning) > 0 {
					msg["reasoning_content"] = strings.Join(reasoning, "")
				}
				out = append(out, msg)
			}
			for _, fr := range responses {
				toolID := strings.TrimSpace(fr.ID)
				if toolID == "" {
					return nil, fmt.Errorf("openai llm: function response missing tool_call id for %q", fr.Name)
				}
				payload, err := json.Marshal(fr.Response)
				if err != nil {
					return nil, err
				}
				out = append(out, map[string]any{
					"role":         "tool",
					"tool_call_id": toolID,
					"content":      string(payload),
				})
			}
			continue
		}

		if role == "assistant" && len(calls) > 0 {
			toolCalls := make([]map[string]any, 0, len(calls))
			for _, fc := range calls {
				args := fc.Args
				if args == nil {
					args = map[string]any{}
				}
				argsBytes, err := json.Marshal(args)
				if err != nil {
					return nil, err
				}
				toolCalls = append(toolCalls, map[string]any{
					"id":   strings.TrimSpace(fc.ID),
					"type": "function",
					"function": map[string]any{
						"name":      fc.Name,
						"arguments": string(argsBytes),
					},
				})
			}
			msg := map[string]any{
				"role":       "assistant",
				"tool_calls": toolCalls,
			}
			if content := strings.Join(texts, "\n"); content != "" {
				msg["content"] = content
			}
			if len(reasoning) > 0 {
				msg["reasoning_content"] = strings.Join(reasoning, "")
			}
			out = append(out, msg)
			continue
		}

		if len(texts) == 0 && len(reasoning) == 0 {
			continue
		}
		msg := map[string]any{
			"role":    role,
			"content": strings.Join(texts, "\n"),
		}
		if len(reasoning) > 0 {
			msg["reasoning_content"] = strings.Join(reasoning, "")
		}
		out = append(out, msg)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("openai llm: no convertible messages")
	}
	return out, nil
}

// SanitizeOpenAIChatMessagesToolSequence drops or neuters assistant turns whose tool_calls are not
// followed by tool-role messages for every tool_call_id. This matches OpenAI Chat Completions rules
// and fixes sessions interrupted mid-tool (cancelled stream) where ADK snapshot still has dangling tool_calls.
func SanitizeOpenAIChatMessagesToolSequence(msgs []map[string]any) []map[string]any {
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]map[string]any, 0, len(msgs))
	for i := 0; i < len(msgs); {
		m := msgs[i]
		role, _ := m["role"].(string)
		if role != "assistant" || m["tool_calls"] == nil {
			out = append(out, m)
			i++
			continue
		}
		tcObjs, ok := normalizedOpenAIToolCalls(m["tool_calls"])
		if !ok || len(tcObjs) == 0 {
			out = append(out, assistantMessageWithoutToolCalls(m))
			i++
			continue
		}
		ids := make([]string, 0, len(tcObjs))
		for _, obj := range tcObjs {
			id, _ := obj["id"].(string)
			id = strings.TrimSpace(id)
			if id != "" {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			out = append(out, assistantMessageWithoutToolCalls(m))
			i++
			continue
		}
		j := i + 1
		answered := map[string]bool{}
		for j < len(msgs) {
			r2, _ := msgs[j]["role"].(string)
			if r2 != "tool" {
				break
			}
			tid, _ := msgs[j]["tool_call_id"].(string)
			tid = strings.TrimSpace(tid)
			if tid != "" {
				answered[tid] = true
			}
			j++
		}
		complete := true
		for _, id := range ids {
			if !answered[id] {
				complete = false
				break
			}
		}
		if complete {
			for k := i; k < j; k++ {
				out = append(out, msgs[k])
			}
			i = j
			continue
		}
		out = append(out, assistantMessageWithoutToolCalls(m))
		i = j
	}
	return out
}

// normalizedOpenAIToolCalls accepts tool_calls as produced by OpenAIMessagesFromContents ([]map[string]any)
// or after JSON decode ([]any of map[string]any).
func normalizedOpenAIToolCalls(raw any) ([]map[string]any, bool) {
	switch v := raw.(type) {
	case []map[string]any:
		if len(v) == 0 {
			return nil, false
		}
		return v, true
	case []any:
		out := make([]map[string]any, 0, len(v))
		for _, item := range v {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, obj)
		}
		return out, len(out) > 0
	default:
		b, err := json.Marshal(raw)
		if err != nil {
			return nil, false
		}
		var arr []map[string]any
		if err := json.Unmarshal(b, &arr); err != nil || len(arr) == 0 {
			return nil, false
		}
		return arr, true
	}
}

func assistantMessageWithoutToolCalls(m map[string]any) map[string]any {
	c := maps.Clone(m)
	delete(c, "tool_calls")
	content, _ := c["content"].(string)
	if strings.TrimSpace(content) == "" {
		c["content"] = "(Previous model turn was interrupted before tool results were received.)"
	}
	return c
}

func temperatureFromConfig(cfg *genai.GenerateContentConfig) float64 {
	if cfg == nil || cfg.Temperature == nil {
		return 0.7
	}
	return float64(*cfg.Temperature)
}

func modelName(reqModel, fallback string) string {
	if strings.TrimSpace(reqModel) != "" {
		return strings.TrimSpace(reqModel)
	}
	return strings.TrimSpace(fallback)
}

func reasoningFromAPIRawJSON(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return strings.Trim(strings.TrimSpace(string(raw)), "\"")
}
