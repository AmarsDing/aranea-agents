package data

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/data/ent/message"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func (r *sessionRepo) UpdateMessageFeedbackJSON(ctx context.Context, sessionID, messageID, rating, comment string) error {
	sessionID = strings.TrimSpace(sessionID)
	messageID = strings.TrimSpace(messageID)
	if sessionID == "" || messageID == "" {
		return apierror.BadRequest("SESSION", "session id and message id are required")
	}
	row, err := r.data.RW().Read(ctx).Message.Query().
		Where(message.IDEQ(messageID), message.SessionIDEQ(sessionID)).
		Only(ctx)
	if err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(row.Role), "assistant") {
		return apierror.BadRequest("SESSION", "feedback is only allowed on assistant messages")
	}
	opts := map[string]any{}
	if raw := strings.TrimSpace(row.OptionsJSON); raw != "" {
		if err := json.Unmarshal([]byte(raw), &opts); err != nil {
			r.data.lg.Warn("message feedback json unmarshal failed", loggateway.StepID("data.session_message_feedback"), loggateway.Err(err))
		}
	}
	fb := map[string]any{
		"rating":     rating,
		"comment":    comment,
		"updated_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	opts["feedback"] = fb
	encoded, err := json.Marshal(opts)
	if err != nil {
		return err
	}
	_, err = r.data.RW().Write(ctx).Message.UpdateOneID(messageID).
		SetOptionsJSON(string(encoded)).
		Save(ctx)
	return err
}
