package data

import (
	"context"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"

	"github.com/google/uuid"
)

// ChatTurnSubmitClaimer provides durable claim/release for chat turn
// submit and enqueue idempotency keys.
type ChatTurnSubmitClaimer struct {
	data *Data
}

// NewChatTurnSubmitClaimer returns a durable claimer backed by chat_turn_submit_claim.
func NewChatTurnSubmitClaimer(d *Data) *ChatTurnSubmitClaimer {
	if d == nil {
		return nil
	}
	return &ChatTurnSubmitClaimer{data: d}
}

// TryClaim inserts (session_id, request_id). Returns true when this process
// won the claim; false when the key already existed (duplicate submission).
// Empty requestID is a no-op claim (returns true) — matches memory registry.
func (c *ChatTurnSubmitClaimer) TryClaim(ctx context.Context, sessionID, requestID string) (bool, error) {
	if c == nil || c.data == nil {
		return true, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	requestID = strings.TrimSpace(requestID)
	if sessionID == "" || requestID == "" {
		return true, nil
	}
	db := c.data.RWDB().WriteDB(ctx)
	if db == nil {
		return false, apierror.Internal("CHAT_TURN_SUBMIT_CLAIM", "repository unavailable")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.ExecContext(ctx, c.data.Dialect().RenumberPlaceholders(`
INSERT INTO chat_turn_submit_claim (id, session_id, request_id, created_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(session_id, request_id) DO NOTHING`),
		uuid.NewString(), sessionID, requestID, now,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Release frees a claim so a failed submission can be retried.
func (c *ChatTurnSubmitClaimer) Release(ctx context.Context, sessionID, requestID string) error {
	if c == nil || c.data == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	requestID = strings.TrimSpace(requestID)
	if sessionID == "" || requestID == "" {
		return nil
	}
	db := c.data.RWDB().WriteDB(ctx)
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, c.data.Dialect().RenumberPlaceholders(`
DELETE FROM chat_turn_submit_claim WHERE session_id = ? AND request_id = ?`),
		sessionID, requestID,
	)
	return err
}
