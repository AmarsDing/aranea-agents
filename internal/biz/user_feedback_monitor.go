package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// feedbackContextSnapshotMaxRunes caps input/output snapshots persisted to
// monitor metadata — enough for review display, bounded against runaway size.
const feedbackContextSnapshotMaxRunes = 2000

// RecordUserFeedbackMonitor persists chat.user_feedback to monitor_events (quality analytics).
// contextJSON is an optional leniently-parsed JSON snapshot (task_id/input/output)
// that keeps the feedback review list self-contained.
func RecordUserFeedbackMonitor(ctx context.Context, u *MonitorUsecase, sessionID, messageID, rating, comment, contextJSON string) error {
	if u == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	messageID = strings.TrimSpace(messageID)
	rating = strings.TrimSpace(rating)
	if sessionID == "" || messageID == "" || rating == "" {
		return nil
	}
	metaMap := map[string]any{
		"session_id": sessionID,
		"message_id": messageID,
		"rating":     rating,
		"comment":    strings.TrimSpace(comment),
	}
	mergeFeedbackContextSnapshot(metaMap, contextJSON)
	meta, _ := json.Marshal(metaMap)
	status := "ok"
	if rating == "negative" {
		status = "warning"
	}
	return u.RecordMonitorEvent(ctx, MonitorEventWrite{
		EventKey:     "chat.user_feedback",
		Name:         "Message feedback",
		Description:  fmt.Sprintf("%s feedback on message %s", rating, messageID),
		Status:       status,
		MetadataJSON: string(meta),
	})
}

// mergeFeedbackContextSnapshot leniently merges task_id/input/output from the
// feedback context snapshot into the monitor metadata map. Malformed JSON is
// silently ignored (feedback persist must never fail on a bad snapshot).
func mergeFeedbackContextSnapshot(metaMap map[string]any, contextJSON string) {
	contextJSON = strings.TrimSpace(contextJSON)
	if contextJSON == "" {
		return
	}
	var snap struct {
		TaskID string `json:"task_id"`
		Input  string `json:"input"`
		Output string `json:"output"`
	}
	if err := json.Unmarshal([]byte(contextJSON), &snap); err != nil {
		return
	}
	if v := strings.TrimSpace(snap.TaskID); v != "" {
		metaMap["task_id"] = v
	}
	if v := truncateRunes(strings.TrimSpace(snap.Input), feedbackContextSnapshotMaxRunes); v != "" {
		metaMap["input"] = v
	}
	if v := truncateRunes(strings.TrimSpace(snap.Output), feedbackContextSnapshotMaxRunes); v != "" {
		metaMap["output"] = v
	}
}
