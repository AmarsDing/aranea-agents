package data

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"aranea-agents/internal/biz"

	entsession "aranea-agents/internal/data/ent/session"
	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func (r *sessionRepo) InsertSessionSummary(ctx context.Context, row biz.SessionSummary) error {
	if r == nil || r.data == nil || r.data.entClient == nil {
		return kerrors.InternalServer("SESSION", "session repo unavailable")
	}
	row.ID = strings.TrimSpace(row.ID)
	row.SessionID = strings.TrimSpace(row.SessionID)
	if row.ID == "" || row.SessionID == "" {
		return kerrors.BadRequest("SESSION", "session summary id and session_id required")
	}
	row.CreatedAt = strings.TrimSpace(row.CreatedAt)
	if row.CreatedAt == "" {
		row.CreatedAt = nowRFC3339()
	}
	q := `INSERT INTO session_summaries (id, session_id, summary_markdown, from_turn, to_turn, token_estimate, created_at)
VALUES (?,?,?,?,?,?,?)`
	_, err := r.data.entClient.ExecContext(ctx, q,
		row.ID, row.SessionID, row.SummaryMarkdown, row.FromTurn, row.ToTurn, row.TokenEstimate, row.CreatedAt)
	return err
}

func (r *sessionRepo) MaxSessionSummaryToTurn(ctx context.Context, sessionID string) (int, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, kerrors.BadRequest("SESSION", "session id is required")
	}
	var max int
	err := entQueryRowScan(r.data.entClient, ctx,
		`SELECT COALESCE(MAX(to_turn), 0) FROM session_summaries WHERE session_id = ?`, []any{sessionID}, &max)
	if err != nil {
		return 0, err
	}
	return max, nil
}

func (r *sessionRepo) ListSessionSummaries(ctx context.Context, sessionID string) ([]biz.SessionSummary, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, kerrors.BadRequest("SESSION", "session id is required")
	}
	rows, err := r.data.entClient.QueryContext(ctx,
		`SELECT id, session_id, summary_markdown, from_turn, to_turn, token_estimate, created_at
FROM session_summaries WHERE session_id = ? ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.SessionSummary
	for rows.Next() {
		var s biz.SessionSummary
		if err := rows.Scan(&s.ID, &s.SessionID, &s.SummaryMarkdown, &s.FromTurn, &s.ToTurn, &s.TokenEstimate, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *sessionRepo) LatestSessionSummaryTime(ctx context.Context, sessionID string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "", kerrors.BadRequest("SESSION", "session id is required")
	}
	var created string
	err := entQueryRowScan(r.data.entClient, ctx,
		`SELECT created_at FROM session_summaries WHERE session_id = ? ORDER BY created_at DESC LIMIT 1`, []any{sessionID}, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return created, nil
}

func (r *sessionRepo) UpdateSessionListSummary(ctx context.Context, sessionID, summary string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return kerrors.BadRequest("SESSION", "session id is required")
	}
	_, err := r.data.entClient.Session.Update().
		Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).
		SetSummary(strings.TrimSpace(summary)).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	return err
}
