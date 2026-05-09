package adksvc

import (
	"strings"
	"time"

	"aranea-agents/internal/biz"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// NewSummarySystemEvent builds one ADK event carrying merged session summaries as a system instruction.
func NewSummarySystemEvent(summaryMarkdown string) *session.Event {
	ev := session.NewEvent("session-summary")
	ev.Author = "user"
	body := strings.TrimSpace(summaryMarkdown)
	if body == "" {
		body = "(empty summary)"
	}
	c := genai.NewContentFromText("[Conversation summary — earlier turns compressed]\n\n"+body, genai.RoleUser)
	c.Role = "system"
	ev.LLMResponse = model.LLMResponse{Content: c}
	ev.Timestamp = time.Now().UTC()
	return ev
}

// RewriteSnapshotWithCompression replaces Events in the persisted bundle while preserving state/root metadata.
func RewriteSnapshotWithCompression(snapshotJSON string, mergedSummariesMarkdown string, tail []biz.ChatMessage, assistantAuthor string) (string, error) {
	bundle, err := unmarshalBundle(snapshotJSON)
	if err != nil {
		return "", err
	}
	sumEv := NewSummarySystemEvent(mergedSummariesMarkdown)
	tailEv := MessagesToADKEvents(tail, assistantAuthor)
	out := make([]*session.Event, 0, 1+len(tailEv))
	out = append(out, sumEv)
	out = append(out, tailEv...)
	bundle.Events = out
	bundle.UpdatedAt = time.Now().UTC()
	return marshalBundle(bundle)
}
