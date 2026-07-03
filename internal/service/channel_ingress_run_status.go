package service

import (
	"context"
	"strings"
)

func (h *ChannelIngress) publishChannelTurnRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || h == nil {
		return
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = sessionID
	}
	if h.chat != nil {
		h.chat.SetRunStatus(ctx, sessionID, runID, status, errMsg)
		return
	}
	if h.eventBus != nil {
		PublishRunStatus(h.eventBus, sessionID, runID, status, errMsg)
	}
}
