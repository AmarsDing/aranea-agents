package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// OpenAIMessagesFromContents maps ADK/genai turns into OpenAI /chat/completions message JSON objects.
func OpenAIMessagesFromContents(contents []*genai.Content) ([]map[string]any, error) {
	if len(contents) == 0 {
		return nil, fmt.Errorf("openai llm: empty Contents")
	}
	out := make([]map[string]any, 0, len(contents))
	for _, c := range contents {
		if c == nil {
			continue
		}
		role := strings.TrimSpace(strings.ToLower(c.Role))
		switch role {
		case "", string(genai.RoleUser):
			role = "user"
		case string(genai.RoleModel):
			role = "assistant"
		case "system":
			// keep
		case "assistant":
			role = "assistant"
		default:
			if role != "system" {
				role = "user"
			}
		}
		var texts []string
		var reasoning []string
		for _, p := range c.Parts {
			if p == nil {
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
