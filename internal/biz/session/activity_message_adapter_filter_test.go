package session

import (
	"testing"
	"time"
)

// F2: system-internal machine notices (metrics payloads, memory recall
// hit-lists) must not leak into the user-facing message view
// (GET /v1/sessions/{id}/messages). The v2 chat stream filters them via
// web/src/features/chat/noticeFilter.ts SYSTEM_NOTICE_TYPES; the messages API
// adapter must apply the same set. User-facing notices (model_router,
// cost_guard, info) stay visible.
func TestActivityToChatMessage_FiltersSystemInternalNotices(t *testing.T) {
	now := time.Now()
	internal := []string{"context_usage", "context_window", "metrics_updated", "token_usage", "memory_recalled", "knowledge_recalled"}
	for _, nt := range internal {
		e := ActivityEntry{
			ID:         "n-" + nt,
			Kind:       "notice",
			NoticeType: nt,
			Status:     "completed",
			SessionID:  "s1",
			Timestamp:  now,
			Content:    `{"hits":[{"layer":"L2"}]}`,
		}
		if _, ok := activityToChatMessage(e); ok {
			t.Errorf("notice_type=%q should be filtered from message view", nt)
		}
	}
}

func TestActivityToChatMessage_KeepsUserFacingNotices(t *testing.T) {
	now := time.Now()
	userFacing := []string{"model_router", "cost_guard", "info", ""}
	for _, nt := range userFacing {
		e := ActivityEntry{
			ID:         "n-" + nt,
			Kind:       "notice",
			NoticeType: nt,
			Status:     "completed",
			SessionID:  "s1",
			Timestamp:  now,
			Content:    "model switched to gpt-5",
		}
		if _, ok := activityToChatMessage(e); !ok {
			t.Errorf("notice_type=%q should remain in message view", nt)
		}
	}
}
