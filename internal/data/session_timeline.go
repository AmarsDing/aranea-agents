package data

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/agent"
	"aranea-agents/internal/data/ent/message"
	"aranea-agents/internal/data/ent/platformskill"
	skillinvocationpkg "aranea-agents/internal/data/ent/skillinvocation"
	toolinvocationpkg "aranea-agents/internal/data/ent/toolinvocation"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func (r *sessionRepo) ListTimelineEventRefsPaged(ctx context.Context, sessionID string, q biz.TimelineQuery) ([]biz.TimelineEventRef, int, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, 0, kerrors.BadRequest("SESSION", "session id is required")
	}
	unionSQL, args := buildTimelineUnionSQL(sessionID, q.KindFilter)
	if unionSQL == "" {
		return nil, 0, nil
	}

	client := r.data.entClient
	var total int
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s)", unionSQL)
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
	listSQL := fmt.Sprintf(
		"SELECT src_kind, id, occurred_at FROM (%s) ORDER BY occurred_at %s, id %s LIMIT ? OFFSET ?",
		unionSQL, orderDir, orderDir,
	)
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

func buildTimelineUnionSQL(sessionID, kindFilter string) (string, []any) {
	kindFilter = strings.TrimSpace(strings.ToLower(kindFilter))
	var branches []string
	var args []any

	appendMessage := kindFilter == "" || kindFilter == "message"
	appendSkill := kindFilter == "" || kindFilter == "skill"
	appendTool := kindFilter == "" || kindFilter == "tool"
	appendMCP := kindFilter == "" || kindFilter == "mcp"

	if appendMessage {
		branches = append(branches, `SELECT 'message' AS src_kind, id, created_at AS occurred_at FROM messages WHERE session_id = ?`)
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

func (r *sessionRepo) ListMessagesByIDs(ctx context.Context, sessionID string, ids []string) ([]biz.ChatMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	ids = dedupeStrings(ids)
	if sessionID == "" || len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.data.entClient.Message.Query().
		Where(message.SessionIDEQ(sessionID), message.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]biz.ChatMessage, len(rows))
	for _, m := range rows {
		byID[m.ID] = entMessageToBiz(m)
	}
	out := make([]biz.ChatMessage, 0, len(ids))
	for _, id := range ids {
		if msg, ok := byID[id]; ok {
			out = append(out, msg)
		}
	}
	return out, nil
}

func (r *sessionRepo) ListToolInvocationsByIDs(ctx context.Context, sessionID string, ids []string) ([]biz.ToolInvocationView, error) {
	sessionID = strings.TrimSpace(sessionID)
	ids = dedupeStrings(ids)
	if sessionID == "" || len(ids) == 0 {
		return nil, nil
	}
	c := r.data.entClient
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
	views, err := toolInvocationRowsToViews(ctx, c, rows)
	if err != nil {
		return nil, err
	}
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
	c := r.data.entClient
	rows, err := c.SkillInvocation.Query().
		Where(skillinvocationpkg.SessionIDEQ(sessionID), skillinvocationpkg.IDIn(ids...)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	views, err := skillInvocationRowsToViews(ctx, c, rows)
	if err != nil {
		return nil, err
	}
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

func toolInvocationRowsToViews(ctx context.Context, c *ent.Client, rows []*ent.ToolInvocation) ([]biz.ToolInvocationView, error) {
	agentNames := map[string]string{}
	agentIDs := make([]string, 0)
	for _, row := range rows {
		if row.AgentID != "" {
			agentIDs = append(agentIDs, row.AgentID)
		}
	}
	agentIDs = dedupeStrings(agentIDs)
	if len(agentIDs) > 0 {
		agents, err := c.Agent.Query().Where(agent.IDIn(agentIDs...), agent.DeletedAtEQ("")).All(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range agents {
			agentNames[a.ID] = a.DisplayName
		}
	}
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
	return out, nil
}

func skillInvocationRowsToViews(ctx context.Context, c *ent.Client, rows []*ent.SkillInvocation) ([]biz.SkillInvocationView, error) {
	skillIDs := dedupeStrings(skillIDsFromSkillInvocations(rows))
	names := map[string]string{}
	if len(skillIDs) > 0 {
		skills, err := c.PlatformSkill.Query().Where(platformskill.IDIn(skillIDs...), platformskill.DeletedAtEQ("")).All(ctx)
		if err != nil {
			return nil, err
		}
		for _, s := range skills {
			names[s.ID] = s.Name
		}
	}
	agentIDs := dedupeStrings(agentIDsFromSkillInvocations(rows))
	agentNames := map[string]string{}
	if len(agentIDs) > 0 {
		agents, err := c.Agent.Query().Where(agent.IDIn(agentIDs...), agent.DeletedAtEQ("")).All(ctx)
		if err != nil {
			return nil, err
		}
		for _, a := range agents {
			agentNames[a.ID] = a.DisplayName
		}
	}
	out := make([]biz.SkillInvocationView, 0, len(rows))
	for _, row := range rows {
		started := row.StartedAt
		if strings.TrimSpace(started) == "" {
			started = row.CreatedAt
		}
		out = append(out, biz.SkillInvocationView{
			ID:               row.ID,
			SkillID:          row.SkillID,
			SkillName:        names[row.SkillID],
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
	return out, nil
}
