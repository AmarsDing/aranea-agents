package session

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/safego"
)

type SessionTitleGenerator interface {
	Generate(ctx context.Context, userMessage string) (string, error)
}

type noopSessionTitleGenerator struct{}

func NewNoopSessionTitleGenerator() SessionTitleGenerator {
	return &noopSessionTitleGenerator{}
}

func (noopSessionTitleGenerator) Generate(_ context.Context, _ string) (string, error) {
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

func (uc *SessionUsecase) maybeAutoTitleFromUserMessage(ctx context.Context, sessionID, content string) error {
	sess, err := uc.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if !shouldAutoNameSession(sess.Title) {
		return nil
	}
	snippet := sessionTitleFromUserSnippet(content)
	if snippet != "" {
		_, _ = uc.Rename(ctx, sessionID, snippet)
	}
	safego.Go(context.Background(), "generate-title-async", func() {
		uc.generateTitleAsync(sessionID, content)
	})
	return nil
}

func (uc *SessionUsecase) generateTitleAsync(sessionID, content string) {
	bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	title, err := uc.titleGenerator.Generate(bgCtx, content)
	if err != nil || title == "" {
		return
	}
	_, _ = uc.Rename(bgCtx, sessionID, title)
}
