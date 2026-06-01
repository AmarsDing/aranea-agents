package data

import (
	"context"
	kerrors "github.com/go-kratos/kratos/v2/errors"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func (r *toolRepo) RecordToolInvocationAudit(ctx context.Context, in biz.ToolInvocationAuditWrite) error {
	client := r.data.Ent()
	if client == nil {
		return kerrors.InternalServer("TOOL", "ent client unavailable")
	}
	toolKey := strings.TrimSpace(in.ToolKey)
	if toolKey == "" {
		return nil
	}
	now := nowRFC3339()
	id := uniqueToolID("taud")
	action := strings.TrimSpace(in.Action)
	if action == "" {
		action = "tool.call"
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = "success"
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = biz.ToolInvocationSourceRuntime
	}
	summary := in.ResultSummary
	if len(summary) > 2000 {
		summary = summary[:2000]
	}
	_, err := client.ExecContext(ctx, `
		INSERT INTO tool_invocation_audit (
			id, invocation_id, tool_key, agent_id, user_id, session_id,
			action, result_summary, status, source, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, strings.TrimSpace(in.InvocationID), toolKey,
		strings.TrimSpace(in.AgentID), strings.TrimSpace(in.UserID), strings.TrimSpace(in.SessionID),
		action, summary, status, source, now,
	)
	if err != nil {
		r.data.lg.Error("tool invocation audit write failed", loggateway.StepID("data.tool.audit_write"), loggateway.Err(err))
	}
	return err
}

func (r *toolRepo) SearchToolInvocationAudits(ctx context.Context, q biz.ToolAuditQuery) (biz.ToolAuditResult, error) {
	client := r.readClient(ctx)
	if client == nil {
		return biz.ToolAuditResult{}, kerrors.InternalServer("TOOL", "ent client unavailable")
	}
	where := []string{"1 = 1"}
	args := []any{}
	if q.ToolKey != "" {
		where = append(where, "tool_key = ?")
		args = append(args, q.ToolKey)
	}
	if q.AgentID != "" {
		where = append(where, "agent_id = ?")
		args = append(args, q.AgentID)
	}
	if q.UserID != "" {
		where = append(where, "user_id = ?")
		args = append(args, q.UserID)
	}
	if q.SessionID != "" {
		where = append(where, "session_id = ?")
		args = append(args, q.SessionID)
	}
	if q.Status != "" {
		where = append(where, "status = ?")
		args = append(args, q.Status)
	}
	if q.From != "" {
		where = append(where, "created_at >= ?")
		args = append(args, q.From)
	}
	if q.To != "" {
		where = append(where, "created_at <= ?")
		args = append(args, q.To)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := entQueryRowScan(client, ctx, `SELECT COUNT(1) FROM tool_invocation_audit WHERE `+whereSQL, args, &total); err != nil {
		return biz.ToolAuditResult{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, q.Limit, q.Offset)
	rows, err := client.QueryContext(ctx, `
		SELECT id, invocation_id, tool_key, agent_id, user_id, session_id,
		       action, result_summary, status, source, created_at
		FROM tool_invocation_audit
		WHERE `+whereSQL+`
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return biz.ToolAuditResult{}, err
	}
	defer rows.Close()
	items := []biz.ToolInvocationAudit{}
	for rows.Next() {
		var item biz.ToolInvocationAudit
		if err := rows.Scan(
			&item.ID, &item.InvocationID, &item.ToolKey, &item.AgentID, &item.UserID, &item.SessionID,
			&item.Action, &item.ResultSummary, &item.Status, &item.Source, &item.CreatedAt,
		); err != nil {
			return biz.ToolAuditResult{}, err
		}
		items = append(items, item)
	}
	return biz.ToolAuditResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, rows.Err()
}

func (r *toolRepo) PurgeToolInvocationAuditsBefore(ctx context.Context, cutoffRFC3339 string) (int64, error) {
	client := r.data.Ent()
	if client == nil {
		return 0, kerrors.InternalServer("TOOL", "ent client unavailable")
	}
	cutoffRFC3339 = strings.TrimSpace(cutoffRFC3339)
	if cutoffRFC3339 == "" {
		return 0, nil
	}
	res, err := client.ExecContext(ctx, `DELETE FROM tool_invocation_audit WHERE created_at < ?`, cutoffRFC3339)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
