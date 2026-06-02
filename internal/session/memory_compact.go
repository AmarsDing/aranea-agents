package session

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type memoryCompactResult struct {
	summaryMarkdown string
	fromTurn        int
	toTurn          int
	didCompact      bool
}

func tryMemoryCompact(ctx context.Context, body []biz.ChatMessage, reader biz.MemoryFactReader, sessionID string, lg loggateway.Logger) memoryCompactResult {
	if len(body) == 0 || reader == nil {
		return memoryCompactResult{}
	}
	facts, err := reader.ReadSessionMemoryFacts(ctx, sessionID)
	if err != nil {
		lg.Warn("MemoryCompact: read memory facts failed", loggateway.StepID("session.compress"), loggateway.SessionID(sessionID), loggateway.Err(err))
		return memoryCompactResult{}
	}
	if len(facts) == 0 {
		return memoryCompactResult{}
	}
	var sb strings.Builder
	sb.WriteString("## Session Memory Summary\n\n### Key Facts\n")
	for _, f := range facts {
		sb.WriteString("- " + f.Statement)
		if f.Scope != "" {
			sb.WriteString(" _[" + f.Scope + "]_")
		}
		sb.WriteString("\n")
	}
	from := body[0].TurnNumber
	to := body[len(body)-1].TurnNumber
	return memoryCompactResult{
		summaryMarkdown: sb.String(),
		fromTurn:        from,
		toTurn:          to,
		didCompact:      true,
	}
}
