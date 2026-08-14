package data

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func (r *toolRepo) RecordToolInvocationAudit(ctx context.Context, in biz.ToolInvocationAuditWrite) error {
	client := r.data.RW().Write(ctx)
	if client == nil {
		return apierror.Internal("TOOL", "ent client unavailable")
	}
	toolKey := strings.TrimSpace(in.ToolKey)
	if toolKey == "" {
		return apierror.BadRequest("TOOL", "tool_key is required for audit record")
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
	summary := truncateUTF8(in.ResultSummary, 2000)
	_, err := client.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		INSERT INTO tool_invocation_audit (
			id, invocation_id, tool_key, agent_id, user_id, session_id,
			action, result_summary, status, source, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		id, strings.TrimSpace(in.InvocationID), toolKey,
		strings.TrimSpace(in.AgentID), strings.TrimSpace(in.UserID), strings.TrimSpace(in.SessionID),
		action, summary, status, source, now,
	)
	if err != nil {
		r.data.lg.Error("tool invocation audit write failed", loggateway.StepID("data.tool.audit_write"), loggateway.Err(err))
	}
	return entErrToBizErr(err, "TOOL")
}

func (r *toolRepo) SearchToolInvocationAudits(ctx context.Context, q biz.ToolAuditQuery) (biz.ToolAuditResult, error) {
	client := r.data.RW().Read(ctx)
	if client == nil {
		return biz.ToolAuditResult{}, apierror.Internal("TOOL", "ent client unavailable")
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
	if ws := strings.TrimSpace(q.WorkspaceID); ws != "" {
		// A3: same tenant-attribution rule as SearchToolInvocations — owning
		// session first, owning agent for session-less rows; unattributed
		// system rows stay hidden from tenant callers (fail-closed).
		where = append(where, `(EXISTS (SELECT 1 FROM sessions ws WHERE ws.id = tool_invocation_audit.session_id AND ws.workspace_id = ?)
			OR (tool_invocation_audit.session_id = '' AND EXISTS (SELECT 1 FROM agents wa WHERE wa.id = tool_invocation_audit.agent_id AND wa.workspace_id = ?)))`)
		args = append(args, ws, ws)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := entQueryRowScan(client, ctx, r.data.Dialect().RenumberPlaceholders(`SELECT COUNT(1) FROM tool_invocation_audit WHERE `+whereSQL), args, &total); err != nil {
		return biz.ToolAuditResult{}, entErrToBizErr(err, "TOOL")
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, q.Limit, q.Offset)
	rows, err := client.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		SELECT id, invocation_id, tool_key, agent_id, user_id, session_id,
		       action, result_summary, status, source, created_at
		FROM tool_invocation_audit
		WHERE `+whereSQL+`
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`), listArgs...)
	if err != nil {
		return biz.ToolAuditResult{}, entErrToBizErr(err, "TOOL")
	}
	defer rows.Close()
	items := []biz.ToolInvocationAudit{}
	for rows.Next() {
		var item biz.ToolInvocationAudit
		if err := rows.Scan(
			&item.ID, &item.InvocationID, &item.ToolKey, &item.AgentID, &item.UserID, &item.SessionID,
			&item.Action, &item.ResultSummary, &item.Status, &item.Source, &item.CreatedAt,
		); err != nil {
			return biz.ToolAuditResult{}, entErrToBizErr(err, "TOOL")
		}
		items = append(items, item)
	}
	return biz.ToolAuditResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, entErrToBizErr(rows.Err(), "TOOL")
}

// toolAuditPurgeBatchSize bounds each DELETE round so retention sweeps over
// large audit tables don't hold one long transaction/lock.
const toolAuditPurgeBatchSize = 1000

func (r *toolRepo) PurgeToolInvocationAuditsBefore(ctx context.Context, cutoffRFC3339 string) (int64, error) {
	client := r.data.RW().Write(ctx)
	if client == nil {
		return 0, apierror.Internal("TOOL", "ent client unavailable")
	}
	cutoffRFC3339 = strings.TrimSpace(cutoffRFC3339)
	if cutoffRFC3339 == "" {
		return 0, nil
	}
	var total int64
	for {
		res, err := client.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
			DELETE FROM tool_invocation_audit
			WHERE id IN (SELECT id FROM tool_invocation_audit WHERE created_at < ? LIMIT ?)`),
			cutoffRFC3339, toolAuditPurgeBatchSize)
		if err != nil {
			return total, entErrToBizErr(err, "TOOL")
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, entErrToBizErr(err, "TOOL")
		}
		total += n
		if n < toolAuditPurgeBatchSize {
			return total, nil
		}
	}
}
