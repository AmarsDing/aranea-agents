package session

import (
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
)

// RewriteSnapshotWithCompression rebuilds runner snapshot events after rolling summary compaction.
func RewriteSnapshotWithCompression(snapshotJSON, mergedSummariesMarkdown string, tail []biz.ChatMessage, assistantAuthor string) (string, error) {
	snapshotJSON = strings.TrimSpace(snapshotJSON)
	if snapshotJSON == "" {
		snapshotJSON = "{}"
	}
	var bundle map[string]any
	if err := json.Unmarshal([]byte(snapshotJSON), &bundle); err != nil {
		return "", err
	}
	if bundle == nil {
		bundle = map[string]any{}
	}
	summaryEvent := map[string]any{
		"author":    "user",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"content":   "[Conversation summary — earlier turns compressed]\n\n" + strings.TrimSpace(mergedSummariesMarkdown),
		"role":      "system",
	}
	var tailEvents []any
	for _, m := range tail {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role != "user" && role != "assistant" {
			continue
		}
		author := role
		if role == "assistant" {
			author = strings.TrimSpace(assistantAuthor)
			if author == "" {
				author = "agent"
			}
		}
		tailEvents = append(tailEvents, map[string]any{
			"author":    author,
			"timestamp": m.CreatedAt,
			"content":   strings.TrimSpace(m.ContentMarkdown),
			"role":      role,
		})
	}
	events := []any{summaryEvent}
	events = append(events, tailEvents...)
	bundle["events"] = events
	bundle["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	out, err := json.Marshal(bundle)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func mergeSessionSummariesMarkdown(rows []biz.SessionSummary) string {
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteString("\n\n---\n\n")
		}
		b.WriteString(strings.TrimSpace(r.SummaryMarkdown))
	}
	return strings.TrimSpace(b.String())
}

func buildCompressTranscript(msgs []biz.ChatMessage) string {
	var b strings.Builder
	for _, m := range msgs {
		role := strings.ToUpper(strings.TrimSpace(m.Role))
		b.WriteString(role)
		b.WriteString(": ")
		b.WriteString(strings.TrimSpace(m.ContentMarkdown))
		b.WriteString("\n\n")
	}
	return strings.TrimSpace(b.String())
}

func timelineUserAssistant(msgs []biz.ChatMessage) []biz.ChatMessage {
	var out []biz.ChatMessage
	for _, m := range msgs {
		r := strings.ToLower(strings.TrimSpace(m.Role))
		if r != "user" && r != "assistant" {
			continue
		}
		if strings.TrimSpace(m.ContentMarkdown) == "" {
			continue
		}
		out = append(out, m)
	}
	return out
}

func firstSummaryLine(md string) string {
	md = strings.TrimSpace(md)
	if md == "" {
		return ""
	}
	for _, line := range strings.Split(md, "\n") {
		t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
		t = strings.TrimSpace(t)
		if t != "" {
			r := []rune(t)
			if len(r) > 160 {
				return string(r[:160]) + "…"
			}
			return t
		}
	}
	return ""
}
