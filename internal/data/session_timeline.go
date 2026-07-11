package data

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	skillinvocationpkg "aranea-agents/internal/data/ent/skillinvocation"
	toolinvocationpkg "aranea-agents/internal/data/ent/toolinvocation"
	"aranea-agents/pkg/apierror"
)

func (r *sessionRepo) ListTimelineEventRefsPaged(ctx context.Context, sessionID string, q biz.TimelineQuery) ([]biz.TimelineEventRef, int, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, 0, apierror.BadRequest("SESSION", "session id is required")
	}
	d := r.data.Dialect()
	unionSQL, args := buildTimelineUnionSQL(sessionID, q.KindFilter, d)
	if unionSQL == "" {
		return nil, 0, nil
	}

	client := r.data.RW().Read(ctx)
	var total int
	countSQL := d.RenumberPlaceholders(fmt.Sprintf("SELECT COUNT(*) FROM (%s)", unionSQL))
	if err := entQueryRowScan(client, ctx, countSQL, args, &total); err != nil {
		return nil, 0, err
	}

	limit := clampTimelinePageLimit(q.Limit, total)
	offset := q.Offset
	if offset < 0 {
		offset = 0
	}
	orderDir := "ASC"
	if strings.EqualFold(strings.TrimSpace(q.SortOrder), "desc") {
		orderDir = "DESC"
	}
	listSQL := d.RenumberPlaceholders(fmt.Sprintf(
		"SELECT src_kind, id, occurred_at FROM (%s) ORDER BY occurred_at %s, id %s LIMIT ? OFFSET ?",
		unionSQL, orderDir, orderDir,
	))
	listArgs := append(append([]any{}, args...), limit, offset)
	rows, err := client.QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]biz.TimelineEventRef, 0, limit)
	for rows.Next() {
		var ref biz.TimelineEventRef
		if err := rows.Scan(&ref.Kind, &ref.ID, &ref.OccurredAt); err != nil {
			return nil, 0, err
		}
		out = append(out, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func clampTimelinePageLimit(limit, total int) int {
	if limit <= 0 {
		if total > 0 {
			return total
		}
		return biz.MessageListMaxLimit
	}
	if limit > biz.MessageListMaxLimit {
		return biz.MessageListMaxLimit
	}
	return limit
}

func buildTimelineUnionSQL(sessionID, kindFilter string, dialect Dialect) (string, []any) {
	kindFilter = strings.TrimSpace(strings.ToLower(kindFilter))
	var branches []string
	var args []any

	appendMessage := kindFilter == "" || kindFilter == "message"
	appendSkill := kindFilter == "" || kindFilter == "skill"
	appendTool := kindFilter == "" || kindFilter == "tool"
	appendMCP := kindFilter == "" || kindFilter == "mcp"

	if appendMessage {
		// Activity-First architecture: messages are now StepV2 rows with
		// kind IN ('task','reply'). The legacy `messages` table was dropped
		// (Phase 3b-D). The `activities` table is reserved for projected
		// plan/stage metadata only — chat-shaped messages live in steps_v2.
		// Filter by spirit_session_id (root Spirit session) and use
		// started_at as occurred_at.
		//
		// steps_v2.started_at is a timestamp column (Postgres timestamptz /
		// SQLite TEXT), while skill/tool branches return occurred_at as an
		// ISO8601 string. We normalize via dialect-specific formatting so
		// the UNION ALL ORDER BY is consistent.
		var tsExpr string
		if dialect.IsPostgres() {
			tsExpr = `to_char(started_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`
		} else {
			// SQLite: started_at stored as TEXT in ISO8601 already.
			tsExpr = `started_at`
		}
		branches = append(branches, fmt.Sprintf(
			`SELECT 'message' AS src_kind, id, %s AS occurred_at FROM steps_v2 WHERE spirit_session_id = ? AND kind IN ('task', 'reply')`,
			tsExpr,
		))
		args = append(args, sessionID)
	}
	if appendSkill {
		branches = append(branches, `SELECT 'skill' AS src_kind, id, CASE WHEN trim(started_at) != '' THEN started_at ELSE created_at END AS occurred_at FROM skill_invocation WHERE session_id = ?`)
		args = append(args, sessionID)
	}
	if appendTool || appendMCP {
		toolWhere := "session_id = ?"
		args = append(args, sessionID)
		mcpExpr := "(lower(source) = 'mcp' OR lower(tool_key) LIKE '%mcp%')"
		switch {
		case appendTool && !appendMCP:
			toolWhere += " AND NOT " + mcpExpr
			branches = append(branches, fmt.Sprintf(`SELECT 'tool' AS src_kind, id, CASE WHEN trim(started_at) != '' THEN started_at ELSE created_at END AS occurred_at FROM tool_invocations WHERE %s`, toolWhere))
		case appendMCP && !appendTool:
			toolWhere += " AND " + mcpExpr
			branches = append(branches, fmt.Sprintf(`SELECT 'mcp' AS src_kind, id, CASE WHEN trim(started_at) != '' THEN started_at ELSE created_at END AS occurred_at FROM tool_invocations WHERE %s`, toolWhere))
		default:
			branches = append(branches, fmt.Sprintf(`SELECT CASE WHEN %s THEN 'mcp' ELSE 'tool' END AS src_kind, id, CASE WHEN trim(started_at) != '' THEN started_at ELSE created_at END AS occurred_at FROM tool_invocations WHERE %s`, mcpExpr, toolWhere))
		}
	}
	if len(branches) == 0 {
		return "", nil
	}
	return strings.Join(branches, " UNION ALL "), args
}

func (r *sessionRepo) ListToolInvocationsByIDs(ctx context.Context, sessionID string, ids []string) ([]biz.ToolInvocationView, error) {
	sessionID = strings.TrimSpace(sessionID)
	ids = dedupeStrings(ids)
	if sessionID == "" || len(ids) == 0 {
		return nil, nil
	}
	c := r.data.RW().Read(ctx)
	rows, err := c.ToolInvocation.Query().
		Where(toolinvocationpkg.SessionIDEQ(sessionID), toolinvocationpkg.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	byID := make(map[string]biz.ToolInvocationView, len(rows))
	views := toolInvocationRowsToViews(rows, nil)
	for _, view := range views {
		byID[view.ID] = view
	}
	out := make([]biz.ToolInvocationView, 0, len(ids))
	for _, id := range ids {
		if view, ok := byID[id]; ok {
			out = append(out, view)
		}
	}
	return out, nil
}

func (r *sessionRepo) ListSkillInvocationsByIDs(ctx context.Context, sessionID string, ids []string) ([]biz.SkillInvocationView, error) {
	sessionID = strings.TrimSpace(sessionID)
	ids = dedupeStrings(ids)
	if sessionID == "" || len(ids) == 0 {
		return nil, nil
	}
	c := r.data.RW().Read(ctx)
	rows, err := c.SkillInvocation.Query().
		Where(skillinvocationpkg.SessionIDEQ(sessionID), skillinvocationpkg.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	views := skillInvocationRowsToViews(rows, nil)
	byID := make(map[string]biz.SkillInvocationView, len(views))
	for _, view := range views {
		byID[view.ID] = view
	}
	out := make([]biz.SkillInvocationView, 0, len(ids))
	for _, id := range ids {
		if view, ok := byID[id]; ok {
			out = append(out, view)
		}
	}
	return out, nil
}

func toolInvocationRowsToViews(rows []*ent.ToolInvocation, agentNames map[string]string) []biz.ToolInvocationView {
	out := make([]biz.ToolInvocationView, 0, len(rows))
	for _, row := range rows {
		out = append(out, biz.ToolInvocationView{
			ID:               row.ID,
			ToolKey:          row.ToolKey,
			ToolDisplayName:  row.ToolKey,
			AgentID:          row.AgentID,
			AgentDisplayName: agentNames[row.AgentID],
			SessionID:        row.SessionID,
			Source:           row.Source,
			Status:           row.Status,
			StartedAt:        row.StartedAt,
			EndedAt:          row.EndedAt,
			DurationMS:       row.DurationMs,
			InputPreview:     row.InputPreview,
			OutputPreview:    row.OutputPreview,
			ErrorCode:        row.ErrorCode,
			ErrorMessage:     row.ErrorMessage,
			MetadataJSON:     row.MetadataJSON,
			CreatedAt:        row.CreatedAt,
		})
	}
	return out
}

func skillInvocationRowsToViews(rows []*ent.SkillInvocation, agentNames map[string]string) []biz.SkillInvocationView {
	out := make([]biz.SkillInvocationView, 0, len(rows))
	for _, row := range rows {
		started := row.StartedAt
		if strings.TrimSpace(started) == "" {
			started = row.CreatedAt
		}
		out = append(out, biz.SkillInvocationView{
			ID:               row.ID,
			SkillID:          row.SkillID,
			SkillName:        "",
			SkillVersion:     row.SkillVersion,
			AgentID:          row.AgentID,
			AgentDisplayName: agentNames[row.AgentID],
			SessionID:        row.SessionID,
			Status:           row.Status,
			DurationMS:       row.DurationMs,
			StartedAt:        started,
			EndedAt:          row.EndedAt,
			InputPreview:     row.InputPreview,
			OutputPreview:    row.OutputPreview,
			ErrorCode:        row.ErrorCode,
			ErrorMessage:     row.ErrorMessage,
		})
	}
	return out
}

func (r *sessionRepo) LookupAgentDisplayNames(ctx context.Context, agentIDs []string) (map[string]string, error) {
	agentIDs = dedupeStrings(agentIDs)
	if len(agentIDs) == 0 {
		return map[string]string{}, nil
	}
	agents, err := r.data.RW().Read(ctx).Agent.Query().Where(agent.IDIn(agentIDs...), agent.DeletedAtEQ("")).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(agents))
	for _, a := range agents {
		out[a.ID] = a.DisplayName
	}
	return out, nil
}
