package service

import (
	"context"
	"time"

	"aranea-agents/internal/event"
)

func (s *ChatService) recordTurnUsage(
	ctx context.Context,
	emitter *event.TraceEmitter,
	sessionID, runID, agentKey, agentID, prov, mod, status string,
	promptTok, completionTok int,
	latency time.Duration,
	errMsg string,
) {
	s.orch.recordTurnUsage(ctx, emitter, sessionID, runID, agentKey, agentID, prov, mod, status, promptTok, completionTok, latency, errMsg)
}
