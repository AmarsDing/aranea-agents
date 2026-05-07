package agent

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/provider"

	"google.golang.org/adk/session"
)

// AppendLLMTextFromEvent appends main text and reasoning from a model event into builders (partial or final chunks).
func AppendLLMTextFromEvent(ev *session.Event, main, reasoning *strings.Builder) {
	if ev == nil {
		return
	}
	m, r := provider.TextsFromLLMResponse(&ev.LLMResponse)
	if m != "" && main != nil {
		main.WriteString(m)
	}
	if r != "" && reasoning != nil {
		reasoning.WriteString(r)
	}
}

// FinalTextFromEvent returns trimmed assistant-visible text from a final model response event.
func FinalTextFromEvent(ev *session.Event) (main string, reasoning string) {
	if ev == nil {
		return "", ""
	}
	m, r := provider.TextsFromLLMResponse(&ev.LLMResponse)
	return strings.TrimSpace(m), strings.TrimSpace(r)
}

// ProjectEvent applies a snapshot of an ADK session event onto a pair of chat rows (user + assistant draft).
func ProjectEvent(ev *session.Event, user *biz.ChatMessage, asst *biz.ChatMessage) {
	if ev == nil || asst == nil {
		return
	}
	main, rsn := FinalTextFromEvent(ev)
	if ev.IsFinalResponse() && !ev.LLMResponse.Partial {
		asst.ContentMarkdown = main
		_ = rsn
		return
	}
	if ev.LLMResponse.Partial {
		asst.ContentMarkdown += main
	}
	_ = user
}
