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

	"aranea-agents/internal/data"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	loggateway "aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/trpcscope"
)

const trpcSessionEventsTable = "trpc_session_events"

type RunnerRollbackStore struct {
	rwdb    *data.ReadWriteDB
	dialect data.Dialect
	lg      loggateway.Logger
}

type rollbackCursor struct {
	AppName   string `json:"app_name"`
	UserID    string `json:"user_id"`
	SessionID string `json:"session_id"`
	EventID   int64  `json:"event_id"`
}

func NewRunnerRollbackStore(rwdb *data.ReadWriteDB, dialect data.Dialect, lg loggateway.Logger) *RunnerRollbackStore {
	if rwdb == nil {
		return nil
	}
	return &RunnerRollbackStore{rwdb: rwdb, dialect: dialect, lg: lg}
}

func (s *RunnerRollbackStore) MarkBoundary(ctx context.Context, sessionID, _, _ string) (string, error) {
	if s == nil || s.rwdb == nil {
		return "", nil
	}
	cur := rollbackCursor{
		AppName:   trpcscope.DefaultAppName,
		UserID:    ctxuser.TRPCUserKey(ctx),
		SessionID: strings.TrimSpace(sessionID),
	}
	if cur.SessionID == "" {
		return "", apierror.BadRequest(apierror.DomainSession, "runner rollback: session_id is required")
	}
	var maxID sql.NullInt64
	query := s.dialect.RenumberPlaceholders(
		`SELECT MAX(id) FROM ` + trpcSessionEventsTable + ` WHERE app_name = ? AND user_id = ? AND session_id = ? AND deleted_at IS NULL`,
	)
	err := data.QueryRowScan(ctx, s.rwdb.ReadDB(ctx), query, []any{cur.AppName, cur.UserID, cur.SessionID}, &maxID)
	if err != nil {
		s.lg.Warn("runner rollback mark failed", loggateway.StepID("system.session.rollback_fail"), loggateway.Str("session_id", sessionID), loggateway.Err(err))
		return "", apierror.Internal(apierror.DomainSession, "runner rollback mark").WithCause(err)
	}
	if maxID.Valid {
		cur.EventID = maxID.Int64
	}
	return encodeRollbackCursor(cur)
}

func (s *RunnerRollbackStore) RollbackToBoundary(ctx context.Context, sessionID, boundaryID string) error {
	if s == nil || s.rwdb == nil || strings.TrimSpace(boundaryID) == "" {
		return nil
	}
	cur, err := decodeRollbackCursor(boundaryID)
	if err != nil {
		return err
	}
	if sid := strings.TrimSpace(sessionID); sid != "" && sid != cur.SessionID {
		s.lg.Warn("runner rollback session mismatch", loggateway.StepID("system.session.rollback_fail"), loggateway.Str("boundary_session", cur.SessionID), loggateway.Str("target_session", sid))
		return apierror.BadRequest(apierror.DomainSession, fmt.Sprintf("runner rollback: boundary session %q does not match %q", cur.SessionID, sid))
	}
	// Pass time.Time directly — database/sql drivers serialize it correctly
	// per dialect (pq → PostgreSQL TIMESTAMP, go-sqlite3 → TEXT/INTEGER).
	// Using UnixNano() produces an int64 that PostgreSQL rejects with
	// datetime_field_overflow (22008) on TIMESTAMP columns.
	now := time.Now().UTC()
	query := s.dialect.RenumberPlaceholders(
		`UPDATE ` + trpcSessionEventsTable + ` SET deleted_at = ?, updated_at = ? WHERE app_name = ? AND user_id = ? AND session_id = ? AND id > ? AND deleted_at IS NULL`,
	)
	_, err = s.rwdb.WriteDB(ctx).ExecContext(ctx, query, now, now, cur.AppName, cur.UserID, cur.SessionID, cur.EventID)
	if err != nil {
		s.lg.Warn("runner rollback update failed", loggateway.StepID("system.session.rollback_fail"), loggateway.Str("session_id", cur.SessionID), loggateway.Err(err))
		return apierror.Internal(apierror.DomainSession, "runner rollback").WithCause(err)
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
		return cur, apierror.BadRequest(apierror.DomainSession, "runner rollback: empty boundary")
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		if legacyID, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil {
			cur.EventID = legacyID
			return cur, nil
		}
		return cur, apierror.Internal(apierror.DomainSession, "runner rollback boundary decode").WithCause(err)
	}
	if err := json.Unmarshal(b, &cur); err != nil {
		return cur, apierror.Internal(apierror.DomainSession, "runner rollback boundary unmarshal").WithCause(err)
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
		return cur, apierror.BadRequest(apierror.DomainSession, "runner rollback: boundary session_id is required")
	}
	return cur, nil
}
