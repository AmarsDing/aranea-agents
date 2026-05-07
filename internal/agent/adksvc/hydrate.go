package adksvc

import (
	"strings"
	"time"

	"aranea-agents/internal/biz"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// messagesToADKEvents converts legacy chat rows into ADK events (best-effort).
func messagesToADKEvents(msgs []biz.ChatMessage, assistantAuthor string) []*session.Event {
	if assistantAuthor == "" {
		assistantAuthor = "agent"
	}
	var out []*session.Event
	for _, m := range msgs {
		role := strings.TrimSpace(strings.ToLower(m.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		body := strings.TrimSpace(m.ContentMarkdown)
		if body == "" {
			continue
		}
		ev := session.NewEvent("legacy")
		switch role {
		case "user":
			ev.Author = "user"
			ev.LLMResponse = model.LLMResponse{
				Content: genai.NewContentFromText(body, genai.RoleUser),
			}
		case "assistant":
			ev.Author = assistantAuthor
			ev.LLMResponse = model.LLMResponse{
				Content: genai.NewContentFromText(body, genai.RoleModel),
			}
		}
		if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(m.CreatedAt)); err == nil {
			ev.Timestamp = ts
		}
		out = append(out, ev)
	}
	return out
}
