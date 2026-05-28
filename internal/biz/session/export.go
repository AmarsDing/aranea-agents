package session

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type sessionExportPayload struct {
	Session   Session              `json:"session"`
	Messages  []ChatMessage        `json:"messages"`
	Timeline  SessionTimeline      `json:"timeline"`
	ExportedAt string              `json:"exported_at"`
}

func (uc *SessionUsecase) Export(ctx context.Context, id, format string) (content, filename, contentType string, err error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", "", "", validationErr("session id is required")
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "markdown"
	}
	if format != "markdown" && format != "json" {
		return "", "", "", validationErr("format must be markdown or json")
	}

	sess, err := uc.sessionReader.GetSessionByID(ctx, id)
	if err != nil {
		return "", "", "", err
	}
	messages, err := uc.listAllMessages(ctx, id)
	if err != nil {
		return "", "", "", err
	}
	timeline, err := uc.Timeline(ctx, id, TimelineQuery{SortOrder: "asc"})
	if err != nil {
		return "", "", "", err
	}

	baseName := sanitizeExportFilename(sess.Title, id)
	switch format {
	case "json":
		payload := sessionExportPayload{
			Session:    sess,
			Messages:   messages,
			Timeline:   timeline,
			ExportedAt: time.Now().UTC().Format(time.RFC3339),
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return "", "", "", err
		}
		return string(data), baseName + ".json", "application/json; charset=utf-8", nil
	default:
		return buildSessionMarkdown(sess, messages, timeline), baseName + ".md", "text/markdown; charset=utf-8", nil
	}
}

func (uc *SessionUsecase) listAllMessages(ctx context.Context, sessionID string) ([]ChatMessage, error) {
	total, err := uc.messageReader.CountMessagesBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if total == 0 {
		return nil, nil
	}
	out := make([]ChatMessage, 0, total)
	for offset := 0; offset < total; {
		limit := MessageListMaxLimit
		if remaining := total - offset; remaining < limit {
			limit = remaining
		}
		chunk, err := uc.messageReader.ListMessagesBySession(ctx, sessionID, limit, offset)
		if err != nil {
			return nil, err
		}
		out = append(out, chunk...)
		offset += len(chunk)
		if len(chunk) == 0 {
			break
		}
	}
	return out, nil
}

func sanitizeExportFilename(title, id string) string {
	base := strings.TrimSpace(title)
	if base == "" {
		base = "session-" + id
	}
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "session-" + id
	}
	return out
}

func buildSessionMarkdown(sess Session, messages []ChatMessage, timeline SessionTimeline) string {
	var sb strings.Builder
	sb.WriteString("# ")
	sb.WriteString(strings.TrimSpace(sess.Title))
	if sb.Len() == 1 {
		sb.WriteString("Untitled Session")
	}
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("- **Session ID**: `%s`\n", sess.ID))
	sb.WriteString(fmt.Sprintf("- **Owner**: %s\n", sess.OwnerType))
	if sess.AgentID != "" {
		sb.WriteString(fmt.Sprintf("- **Agent**: `%s`\n", sess.AgentID))
	}
	if sess.TeamID != "" {
		sb.WriteString(fmt.Sprintf("- **Team**: `%s`\n", sess.TeamID))
	}
	sb.WriteString(fmt.Sprintf("- **Messages**: %d\n", sess.MessageCount))
	sb.WriteString(fmt.Sprintf("- **Tokens**: %d\n", sess.TotalTokens))
	sb.WriteString(fmt.Sprintf("- **Exported**: %s\n\n", time.Now().UTC().Format(time.RFC3339)))

	if strings.TrimSpace(sess.Summary) != "" {
		sb.WriteString("## Summary\n\n")
		sb.WriteString(sess.Summary)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Messages\n\n")
	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("### %s · turn %d\n\n", msg.Role, msg.TurnNumber))
		if strings.TrimSpace(msg.ContentMarkdown) != "" {
			sb.WriteString(msg.ContentMarkdown)
			sb.WriteString("\n\n")
		}
	}

	sb.WriteString("## Timeline\n\n")
	for _, item := range timeline.Items {
		sb.WriteString(fmt.Sprintf("- [%s] **%s** — %s (%s)\n", item.OccurredAt, item.Kind, item.Title, item.Status))
		if strings.TrimSpace(item.Preview) != "" {
			sb.WriteString(fmt.Sprintf("  > %s\n", item.Preview))
		}
	}
	return sb.String()
}
