package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// RecordUserFeedbackMonitor persists chat.user_feedback to monitor_events (quality analytics).
func RecordUserFeedbackMonitor(ctx context.Context, u *MonitorUsecase, sessionID, messageID, rating, comment string) error {
	if u == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	messageID = strings.TrimSpace(messageID)
	rating = strings.TrimSpace(rating)
	if sessionID == "" || messageID == "" || rating == "" {
		return nil
	}
	meta, _ := json.Marshal(map[string]any{
		"session_id": sessionID,
		"message_id": messageID,
		"rating":     rating,
		"comment":    strings.TrimSpace(comment),
	})
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
