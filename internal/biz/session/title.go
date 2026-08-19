package session

import (
	"context"
	"strings"
)

// TitleGenRequest carries the input for LLM title generation.
// SessionID/AgentID are used for aux usage recording (P1-2, 2026-08-19);
// they do not affect the generated title.
type TitleGenRequest struct {
	UserMessage string
	SessionID   string
	AgentID     string
}

type SessionTitleGenerator interface {
	Generate(ctx context.Context, req TitleGenRequest) (string, error)
}

type noopSessionTitleGenerator struct{}

func NewNoopSessionTitleGenerator() SessionTitleGenerator {
	return &noopSessionTitleGenerator{}
}

func (noopSessionTitleGenerator) Generate(_ context.Context, _ TitleGenRequest) (string, error) {
	return "", nil
}

func sessionTitleFromUserSnippet(snippet string) string {
	s := strings.TrimSpace(snippet)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > 56 {
		return string(r[:56]) + "…"
	}
	return s
}

func shouldAutoNameSession(title string) bool {
	t := strings.TrimSpace(title)
	if t == "" {
		return true
	}
	lower := strings.ToLower(t)
	switch lower {
	case "untitled", "new chat":
		return true
	}
	if strings.Contains(t, "未命名") || strings.Contains(t, "新会话") || strings.Contains(t, "新对话") {
		return true
	}
	return false
}
