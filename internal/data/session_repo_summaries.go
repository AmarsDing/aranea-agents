package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"

	entsession "aranea-agents/internal/data/ent/session"
	"aranea-agents/pkg/apierror"
)

func (r *sessionRepo) InsertSessionSummary(ctx context.Context, row biz.SessionSummary) error {
	if r == nil || r.data == nil {
		return apierror.Internal("SESSION", "session repo unavailable")
	}
	row.ID = strings.TrimSpace(row.ID)
	row.SessionID = strings.TrimSpace(row.SessionID)
	if row.ID == "" || row.SessionID == "" {
		return apierror.BadRequest("SESSION", "session summary id and session_id required")
	}
	row.CreatedAt = strings.TrimSpace(row.CreatedAt)
	if row.CreatedAt == "" {
		row.CreatedAt = nowRFC3339()
	}
	q := r.data.Dialect().RenumberPlaceholders(`INSERT INTO session_summaries (id, session_id, summary_markdown, from_turn, to_turn, token_estimate, created_at)
VALUES (?,?,?,?,?,?,?)`)
	_, err := r.data.RW().Write(ctx).ExecContext(ctx, q,
		row.ID, row.SessionID, row.SummaryMarkdown, row.FromTurn, row.ToTurn, row.TokenEstimate, row.CreatedAt)
	return err
}

// DeleteSessionSummaries removes all rolling summary rows for a session.
// Called inside CompressSessionInTx when the LLM absorbed prior summaries into
// a single merged row (recursive rolling summary).
func (r *sessionRepo) DeleteSessionSummaries(ctx context.Context, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return apierror.BadRequest("SESSION", "session id is required")
	}
	_, err := r.data.RW().Write(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`DELETE FROM session_summaries WHERE session_id = ?`), sessionID)
	return entErrToBizErr(err, apierror.DomainSession)
}

func (r *sessionRepo) MaxSessionSummaryToTurn(ctx context.Context, sessionID string) (int, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, apierror.BadRequest("SESSION", "session id is required")
	}
	var max int
	err := entQueryRowScan(r.data.RW().Read(ctx), ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT COALESCE(MAX(to_turn), 0) FROM session_summaries WHERE session_id = ?`), []any{sessionID}, &max)
	if err != nil {
		return 0, err
	}
	return max, nil
}

func (r *sessionRepo) ListSessionSummaries(ctx context.Context, sessionID string) ([]biz.SessionSummary, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, apierror.BadRequest("SESSION", "session id is required")
	}
	rows, err := r.data.RW().Read(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT id, session_id, summary_markdown, from_turn, to_turn, token_estimate, created_at
FROM session_summaries WHERE session_id = ? ORDER BY created_at ASC`), sessionID)
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
		return "", apierror.BadRequest("SESSION", "session id is required")
	}
	var created string
	err := entQueryRowScan(r.data.RW().Read(ctx), ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT created_at FROM session_summaries WHERE session_id = ? ORDER BY created_at DESC LIMIT 1`), []any{sessionID}, &created)
	if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeNotFound {
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
		return apierror.BadRequest("SESSION", "session id is required")
	}
	_, err := r.data.RW().Write(ctx).Session.Update().
		Where(entsession.IDEQ(sessionID), entsession.DeletedAtEQ("")).
		SetSummary(strings.TrimSpace(summary)).
		SetUpdatedAt(nowRFC3339()).
		Save(ctx)
	return err
}

func (r *sessionRepo) SessionSummaryExists(ctx context.Context, sessionID string, fromTurn, toTurn int) (bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, apierror.BadRequest("SESSION", "session id is required")
	}
	var cnt int
	err := entQueryRowScan(r.data.RW().Read(ctx), ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT COUNT(1) FROM session_summaries WHERE session_id = ? AND from_turn = ? AND to_turn = ?`),
		[]any{sessionID, fromTurn, toTurn}, &cnt)
	if err != nil {
		return false, err
	}
	return cnt > 0, nil
}
