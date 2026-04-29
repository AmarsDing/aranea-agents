package sqlite

import (
	"database/sql"
	"errors"
	"strings"

	"arenea/backend/internal/domain"
)

// CreateSession 插入 `sessions` 行。
func (s *Store) CreateSession(d domain.Session) (domain.Session, error) {
	if d.ID == "" || d.Title == "" {
		return domain.Session{}, errors.New("missing required fields")
	}
	if d.OwnerType == "" {
		d.OwnerType = "agent"
	}
	if d.OwnerType == "agent" && d.AgentID == "" {
		return domain.Session{}, errors.New("agent_id is required")
	}
	if d.OwnerType == "team" && d.TeamID == "" {
		return domain.Session{}, errors.New("team_id is required")
	}
	now := nowISO()
	d.CreatedAt = now
	d.UpdatedAt = now
	if d.Status == "" {
		d.Status = "active"
	}
	if d.ContextStatus == "" {
		d.ContextStatus = contextStatusForRatio(d.ContextUsedRatio)
	}
	_, err := s.db.Exec(
		`INSERT INTO sessions(
		 id, owner_type, agent_id, team_id, title, summary, context_used_ratio, context_used_tokens, max_context_used_ratio, last_context_window_tokens, context_status,
		 dialog_mode, provider, model, status, message_count, run_count, model_call_count, tool_call_count, skill_call_count,
		 mcp_call_count, input_tokens, output_tokens, total_tokens, total_cost_micro_usd, last_message_at, created_at, updated_at, archived_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.OwnerType, d.AgentID, d.TeamID, d.Title, d.Summary, d.ContextUsedRatio, d.ContextUsedTokens, d.MaxContextUsedRatio, d.LastContextWindowTokens, d.ContextStatus,
		d.DialogMode, d.Provider, d.Model, d.Status, d.MessageCount, d.RunCount, d.ModelCallCount, d.ToolCallCount, d.SkillCallCount,
		d.MCPCallCount, d.InputTokens, d.OutputTokens, d.TotalTokens, d.TotalCostMicroUSD, d.LastMessageAt, d.CreatedAt, d.UpdatedAt, d.ArchivedAt, d.DeletedAt,
	)
	return d, err
}

// GetSessionByID 按 id 读取未软删 session。
func (s *Store) GetSessionByID(id string) (domain.Session, error) {
	row := s.db.QueryRow(sessionSelectSQL()+` WHERE id = ? AND deleted_at = ''`, id)
	return scanSession(row)
}

// ListSessions 为兼容保留：委托 SearchSessions。
func (s *Store) ListSessions(agentID string) ([]domain.Session, error) {
	result, err := s.SearchSessions(domain.SessionSearchQuery{AgentID: agentID, Limit: 200})
	return result.Items, err
}

// ListTeamSessions 委托 SearchSessions。
func (s *Store) ListTeamSessions(teamID string) ([]domain.Session, error) {
	result, err := s.SearchSessions(domain.SessionSearchQuery{TeamID: teamID, Limit: 200})
	return result.Items, err
}

// SearchSessions 筛选会话列表。
func (s *Store) SearchSessions(query domain.SessionSearchQuery) (domain.SessionListResult, error) {
	if query.Limit <= 0 || query.Limit > 100 {
		query.Limit = 20
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	clauses := []string{"deleted_at = ''"}
	args := []any{}
	if query.OwnerType != "" {
		clauses = append(clauses, "owner_type = ?")
		args = append(args, query.OwnerType)
	}
	if query.AgentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, query.AgentID)
	}
	if query.TeamID != "" {
		clauses = append(clauses, "team_id = ?")
		args = append(args, query.TeamID)
	}
	if query.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, query.Status)
	}
	if query.ContextStatus != "" {
		clauses = append(clauses, "context_status = ?")
		args = append(args, query.ContextStatus)
	}
	if query.Keyword != "" {
		clauses = append(clauses, "(title LIKE ? OR summary LIKE ? OR id LIKE ?)")
		like := "%" + query.Keyword + "%"
		args = append(args, like, like, like)
	}
	where := strings.Join(clauses, " AND ")
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE `+where, args...).Scan(&total); err != nil {
		return domain.SessionListResult{}, err
	}
	listArgs := append(append([]any{}, args...), query.Limit, query.Offset)
	rows, err := s.db.Query(sessionSelectSQL()+` WHERE `+where+` ORDER BY COALESCE(NULLIF(last_message_at, ''), updated_at) DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return domain.SessionListResult{}, err
	}
	defer rows.Close()
	items, err := scanSessions(rows)
	if err != nil {
		return domain.SessionListResult{}, err
	}
	return domain.SessionListResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

// UpdateSessionTitle 更新标题。
func (s *Store) UpdateSessionTitle(id string, title string) (domain.Session, error) {
	_, err := s.db.Exec(`UPDATE sessions SET title = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, title, nowISO(), id)
	if err != nil {
		return domain.Session{}, err
	}
	return s.GetSessionByID(id)
}

// UpdateSessionContextUsedRatio 按 ratio 写回 context 列。
func (s *Store) UpdateSessionContextUsedRatio(sessionID string, ratio float64) error {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	_, err := s.db.Exec(`UPDATE sessions SET context_used_ratio = ?, max_context_used_ratio = MAX(max_context_used_ratio, ?), context_status = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, ratio, ratio, contextStatusForRatio(ratio), nowISO(), sessionID)
	return err
}

// UpdateSessionL0Context 与 UpdateSessionContextUsedRatio 搭配，记录 L0 的 token/窗口信息。
func (s *Store) UpdateSessionL0Context(sessionID string, promptTokens int, contextWindow int, ratio float64) error {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	if promptTokens < 0 {
		promptTokens = 0
	}
	if contextWindow < 0 {
		contextWindow = 0
	}
	_, err := s.db.Exec(`UPDATE sessions SET context_used_ratio = ?, context_used_tokens = ?, last_context_window_tokens = ?, max_context_used_ratio = MAX(max_context_used_ratio, ?), context_status = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`,
		ratio, promptTokens, contextWindow, ratio, contextStatusForRatio(ratio), nowISO(), sessionID)
	return err
}

// ArchiveSession 将 session 标为已归档。
func (s *Store) ArchiveSession(id string) error {
	now := nowISO()
	_, err := s.db.Exec(`UPDATE sessions SET status = 'archived', archived_at = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, now, now, id)
	return err
}

// DeleteSession 软删 session。
func (s *Store) DeleteSession(id string) error {
	now := nowISO()
	_, err := s.db.Exec(`UPDATE sessions SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE id = ? AND deleted_at = ''`, now, now, id)
	return err
}

// DeleteSessionsByAgentID 按 agent 批量软删。
func (s *Store) DeleteSessionsByAgentID(agentID string) error {
	now := nowISO()
	_, err := s.db.Exec(`UPDATE sessions SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE agent_id = ? AND deleted_at = ''`, now, now, agentID)
	return err
}

func sessionSelectSQL() string {
	return `SELECT id, owner_type, agent_id, team_id, title, summary, context_used_ratio, context_used_tokens, max_context_used_ratio, last_context_window_tokens, context_status,
	 dialog_mode, provider, model, status, message_count, run_count, model_call_count, tool_call_count, skill_call_count,
	 mcp_call_count, input_tokens, output_tokens, total_tokens, total_cost_micro_usd, last_message_at, created_at, updated_at, archived_at, deleted_at FROM sessions`
}

func contextStatusForRatio(ratio float64) string {
	switch {
	case ratio >= 0.95:
		return "exceeded"
	case ratio >= 0.8:
		return "critical"
	case ratio >= 0.6:
		return "warning"
	default:
		return "normal"
	}
}

func scanSession(row scanner) (domain.Session, error) {
	var v domain.Session
	err := row.Scan(&v.ID, &v.OwnerType, &v.AgentID, &v.TeamID, &v.Title, &v.Summary, &v.ContextUsedRatio, &v.ContextUsedTokens, &v.MaxContextUsedRatio, &v.LastContextWindowTokens, &v.ContextStatus, &v.DialogMode, &v.Provider, &v.Model, &v.Status, &v.MessageCount, &v.RunCount, &v.ModelCallCount, &v.ToolCallCount, &v.SkillCallCount, &v.MCPCallCount, &v.InputTokens, &v.OutputTokens, &v.TotalTokens, &v.TotalCostMicroUSD, &v.LastMessageAt, &v.CreatedAt, &v.UpdatedAt, &v.ArchivedAt, &v.DeletedAt)
	return v, err
}

func scanSessions(rows *sql.Rows) ([]domain.Session, error) {
	var result []domain.Session
	for rows.Next() {
		v, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
