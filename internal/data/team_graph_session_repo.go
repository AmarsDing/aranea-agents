package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

type teamGraphSessionRepo struct {
	data *Data
}

var _ biz.TeamGraphSessionRepo = (*teamGraphSessionRepo)(nil)

func NewTeamGraphSessionRepo(d *Data) biz.TeamGraphSessionRepo {
	return &teamGraphSessionRepo{data: d}
}

func (r *teamGraphSessionRepo) readDB(ctx context.Context) execer {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RWDB().ReadDB(ctx)
}

func (r *teamGraphSessionRepo) writeDB(ctx context.Context) execer {
	if r == nil || r.data == nil {
		return nil
	}
	return r.data.RWDB().WriteDB(ctx)
}

const teamGraphSessionSelectSQL = `
SELECT exec_id, team_run_id, team_id, session_id, input_preview, definition_json,
  status, registered_at, last_activity_at, created_at, updated_at
FROM team_graph_sessions`

func scanTeamGraphSessionRow(scanner interface {
	Scan(dest ...any) error
}) (biz.TeamGraphSession, error) {
	var s biz.TeamGraphSession
	err := scanner.Scan(
		&s.ExecID, &s.TeamRunID, &s.TeamID, &s.SessionID,
		&s.InputPreview, &s.DefinitionJSON,
		&s.Status, &s.RegisteredAt, &s.LastActivityAt,
		&s.CreatedAt, &s.UpdatedAt,
	)
	return s, err
}

func (r *teamGraphSessionRepo) SaveSession(ctx context.Context, sess biz.TeamGraphSession) error {
	db := r.writeDB(ctx)
	if db == nil {
		return nil
	}
	now := nowRFC3339()
	if sess.CreatedAt == "" {
		sess.CreatedAt = now
	}
	if sess.UpdatedAt == "" {
		sess.UpdatedAt = now
	}
	if sess.RegisteredAt == "" {
		sess.RegisteredAt = now
	}
	if sess.LastActivityAt == "" {
		sess.LastActivityAt = now
	}
	_, err := db.ExecContext(ctx, `
INSERT OR REPLACE INTO team_graph_sessions
  (exec_id, team_run_id, team_id, session_id, input_preview, definition_json,
   status, registered_at, last_activity_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ExecID, sess.TeamRunID, sess.TeamID, sess.SessionID,
		sess.InputPreview, sess.DefinitionJSON,
		sess.Status, sess.RegisteredAt, sess.LastActivityAt,
		sess.CreatedAt, sess.UpdatedAt,
	)
	return err
}

func (r *teamGraphSessionRepo) UpdateSessionStatus(ctx context.Context, execID, status string) error {
	db := r.writeDB(ctx)
	if db == nil {
		return nil
	}
	now := nowRFC3339()
	_, err := db.ExecContext(ctx, `
UPDATE team_graph_sessions SET status=?, last_activity_at=?, updated_at=?
WHERE exec_id=?`,
		status, now, now, strings.TrimSpace(execID),
	)
	return err
}

func (r *teamGraphSessionRepo) GetSession(ctx context.Context, execID string) (biz.TeamGraphSession, error) {
	db := r.readDB(ctx)
	if db == nil {
		return biz.TeamGraphSession{}, apierror.NotFound(apierror.DomainTeam, "not found")
	}
	rows, err := db.QueryContext(ctx, teamGraphSessionSelectSQL+` WHERE exec_id=? LIMIT 1`,
		strings.TrimSpace(execID))
	if err != nil {
		return biz.TeamGraphSession{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.TeamGraphSession{}, apierror.NotFound(apierror.DomainTeam, "not found")
	}
	return scanTeamGraphSessionRow(rows)
}

func (r *teamGraphSessionRepo) ListActiveSessions(ctx context.Context) ([]biz.TeamGraphSession, error) {
	db := r.readDB(ctx)
	if db == nil {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, teamGraphSessionSelectSQL+
		` WHERE status IN (?, ?) ORDER BY created_at ASC`,
		biz.TeamRunStatusRunning, biz.TeamRunStatusWaitingHuman)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.TeamGraphSession
	for rows.Next() {
		s, err := scanTeamGraphSessionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *teamGraphSessionRepo) DeleteSession(ctx context.Context, execID string) error {
	db := r.writeDB(ctx)
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `DELETE FROM team_graph_sessions WHERE exec_id=?`,
		strings.TrimSpace(execID))
	return err
}

func (r *teamGraphSessionRepo) MarkOrphanedSessionsTerminal(ctx context.Context) (int, error) {
	db := r.writeDB(ctx)
	if db == nil {
		return 0, nil
	}
	now := nowRFC3339()
	res, err := db.ExecContext(ctx, `
UPDATE team_graph_sessions
SET status='cancelled', last_activity_at=?, updated_at=?
WHERE status=?`,
		now, now,
		biz.TeamRunStatusRunning,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
