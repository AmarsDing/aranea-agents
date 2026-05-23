package service

import "strings"

// publishChannelTurnRunStatus syncs Channel turn terminal state to Web (run_status + Finish).
func (h *ChannelIngress) publishChannelTurnRunStatus(sessionID, runID, status, errMsg string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || h == nil {
		return
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = sessionID
	}
	if h.chat != nil {
		h.chat.setRunStatus(sessionID, runID, status, errMsg)
		return
	}
	if h.eventBus != nil {
		PublishRunStatus(h.eventBus, sessionID, runID, status, errMsg)
	}
}
