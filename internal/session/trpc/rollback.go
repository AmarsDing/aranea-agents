package session

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"aranea-agents/pkg/ctxuser"
	loggateway "aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/trpcscope"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

const trpcSessionEventsTable = "trpc_session_events"

type RunnerRollbackStore struct {
	db *sql.DB
	lg loggateway.Logger
}

type rollbackCursor struct {
	AppName   string `json:"app_name"`
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
	EventID   int64  `json:"event_id"`
}

func NewRunnerRollbackStore(db *sql.DB, lg loggateway.Logger) *RunnerRollbackStore {
	if db == nil {
		return nil
	}
	return &RunnerRollbackStore{db: db, lg: lg}
}

func (s *RunnerRollbackStore) MarkBoundary(ctx context.Context, sessionID, _, _ string) (string, error) {
	if s == nil || s.db == nil {
		return "", nil
	}
	cur := rollbackCursor{
		AppName:   trpcscope.DefaultAppName,
		UserID:    ctxuser.TRPCUserKey(ctx),
		SessionID: strings.TrimSpace(sessionID),
	}
	if cur.SessionID == "" {
		return "", kerrors.BadRequest("SESSION", "runner rollback: session_id is required")
	}
	var maxID sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(id) FROM `+trpcSessionEventsTable+` WHERE app_name = ? AND user_id = ? AND session_id = ? AND deleted_at IS NULL`,
		cur.AppName, cur.UserID, cur.SessionID,
	).Scan(&maxID)
	if err != nil {
		s.lg.Warn("runner rollback mark failed", loggateway.StepID("system.session.rollback_fail"), loggateway.Str("session_id", sessionID), loggateway.Err(err))
		return "", kerrors.InternalServer("SESSION", "runner rollback mark: "+err.Error())
	}
	if maxID.Valid {
		cur.EventID = maxID.Int64
	}
	return encodeRollbackCursor(cur)
}

func (s *RunnerRollbackStore) RollbackToBoundary(ctx context.Context, sessionID, boundaryID string) error {
	if s == nil || s.db == nil || strings.TrimSpace(boundaryID) == "" {
		return nil
	}
	cur, err := decodeRollbackCursor(boundaryID)
	if err != nil {
		return err
	}
	if sid := strings.TrimSpace(sessionID); sid != "" && sid != cur.SessionID {
		s.lg.Warn("runner rollback session mismatch", loggateway.StepID("system.session.rollback_fail"), loggateway.Str("boundary_session", cur.SessionID), loggateway.Str("target_session", sid))
		return kerrors.BadRequest("SESSION", fmt.Sprintf("runner rollback: boundary session %q does not match %q", cur.SessionID, sid))
	}
	now := time.Now().UTC().UnixNano()
	_, err = s.db.ExecContext(ctx,
		`UPDATE `+trpcSessionEventsTable+` SET deleted_at = ?, updated_at = ? WHERE app_name = ? AND user_id = ? AND session_id = ? AND id > ? AND deleted_at IS NULL`,
		now, now, cur.AppName, cur.UserID, cur.SessionID, cur.EventID,
	)
	if err != nil {
		s.lg.Warn("runner rollback update failed", loggateway.StepID("system.session.rollback_fail"), loggateway.Str("session_id", cur.SessionID), loggateway.Err(err))
		return kerrors.InternalServer("SESSION", "runner rollback: "+err.Error())
	}
	return nil
}

func encodeRollbackCursor(cur rollbackCursor) (string, error) {
	b, err := json.Marshal(cur)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func decodeRollbackCursor(s string) (rollbackCursor, error) {
	var cur rollbackCursor
	raw := strings.TrimSpace(s)
	if raw == "" {
		return cur, kerrors.BadRequest("SESSION", "runner rollback: empty boundary")
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		if legacyID, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil {
			cur.EventID = legacyID
			return cur, nil
		}
		return cur, kerrors.InternalServer("SESSION", "runner rollback boundary decode: "+err.Error())
	}
	if err := json.Unmarshal(b, &cur); err != nil {
		return cur, kerrors.InternalServer("SESSION", "runner rollback boundary unmarshal: "+err.Error())
	}
	cur.AppName = strings.TrimSpace(cur.AppName)
	if cur.AppName == "" {
		cur.AppName = trpcscope.DefaultAppName
	}
	cur.UserID = strings.TrimSpace(cur.UserID)
	if cur.UserID == "" {
		cur.UserID = ctxuser.DefaultUserID
	}
	cur.SessionID = strings.TrimSpace(cur.SessionID)
	if cur.SessionID == "" {
		return cur, kerrors.BadRequest("SESSION", "runner rollback: boundary session_id is required")
	}
	return cur, nil
}
