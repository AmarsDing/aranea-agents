package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformtool"
)

type toolRepo struct {
	data *Data
}

// NewToolRepo implements biz.ToolRepo（legacy capability/storage 语义）.
func NewToolRepo(d *Data) biz.ToolRepo {
	return &toolRepo{data: d}
}

func adminToolPerms() biz.ToolPermissions {
	return biz.ToolPermissions{CanManage: true}
}

var toolIDCtr atomic.Uint64

func uniqueToolID(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UTC().UnixNano(), toolIDCtr.Add(1))
}

func toolSelectSQL() string {
	return `
		SELECT t.id, t.tool_key, t.display_name, t.description, t.category, t.source, t.risk_level,
		       t.enabled, t.readonly, t.requires_confirmation, t.supports_streaming, t.supports_concurrency,
		       t.parameters_schema_json, t.result_schema_json, t.config_schema_json, t.config_json, t.default_config_json, t.metadata_json,
		       COALESCE(stats.invoke_count, 0), COALESCE(stats.invoke_count_24h, 0), COALESCE(stats.success_count, 0),
		       COALESCE(stats.failure_count, 0), COALESCE(stats.blocked_count, 0), COALESCE(overrides.agent_override_count, 0),
		       stats.avg_duration_ms, COALESCE(last.started_at, ''), COALESCE(last.status, ''),
		       t.created_at, t.updated_at, t.deleted_at
		FROM tools t
		LEFT JOIN (
			SELECT tool_key,
			       COUNT(1) AS invoke_count,
			       SUM(CASE WHEN started_at >= ? THEN 1 ELSE 0 END) AS invoke_count_24h,
			       SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) AS success_count,
			       SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) AS failure_count,
			       SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END) AS blocked_count,
			       AVG(duration_ms) AS avg_duration_ms
			FROM tool_invocations
			GROUP BY tool_key
		) stats ON stats.tool_key = t.tool_key
		LEFT JOIN (
			SELECT tool_key, COUNT(1) AS agent_override_count
			FROM tool_agent_overrides
			WHERE deleted_at = ''
			GROUP BY tool_key
		) overrides ON overrides.tool_key = t.tool_key
		LEFT JOIN (
			SELECT ti.tool_key, ti.started_at, ti.status
			FROM tool_invocations ti
			INNER JOIN (
				SELECT tool_key, MAX(started_at) AS max_started_at
				FROM tool_invocations
				GROUP BY tool_key
			) latest ON latest.tool_key = ti.tool_key AND latest.max_started_at = ti.started_at
		) last ON last.tool_key = t.tool_key`
}

func toolWhereClause(q biz.ToolListQuery) (string, []any) {
	where := []string{"t.deleted_at = ''"}
	args := []any{}
	if search := strings.TrimSpace(q.Search); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = append(where, "(LOWER(t.tool_key) LIKE ? OR LOWER(t.display_name) LIKE ? OR LOWER(t.description) LIKE ?)")
		args = append(args, like, like, like)
	}
	if q.Category != "" {
		where = append(where, "t.category = ?")
		args = append(args, q.Category)
	}
	if q.Source != "" {
		where = append(where, "t.source = ?")
		args = append(args, q.Source)
	}
	if q.RiskLevel != "" {
		where = append(where, "t.risk_level = ?")
		args = append(args, q.RiskLevel)
	}
	if q.Enabled == "true" || q.Enabled == "false" {
		where = append(where, "t.enabled = ?")
		args = append(args, q.Enabled == "true")
	}
	return strings.Join(where, " AND "), args
}

func scanBizTool(rows *sql.Rows) ([]biz.Tool, error) {
	out := []biz.Tool{}
	for rows.Next() {
		var item biz.Tool
		var avg sql.NullFloat64
		if err := rows.Scan(
			&item.ID, &item.Key, &item.DisplayName, &item.Description, &item.Category, &item.Source, &item.RiskLevel,
			&item.Enabled, &item.Readonly, &item.RequiresConfirmation, &item.SupportsStreaming, &item.SupportsConcurrency,
			&item.ParametersSchemaJSON, &item.ResultSchemaJSON, &item.ConfigSchemaJSON, &item.ConfigJSON, &item.DefaultConfigJSON, &item.MetadataJSON,
			&item.InvokeCount, &item.InvokeCount24h, &item.SuccessCount, &item.FailureCount, &item.BlockedCount, &item.AgentOverrideCount,
			&avg, &item.LastInvokedAt, &item.LastStatus,
			&item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
		); err != nil {
			return nil, err
		}
		if avg.Valid {
			v := avg.Float64
			item.AvgDurationMS = &v
		}
		item.Permissions = adminToolPerms()
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *toolRepo) computeToolSummary(ctx context.Context, client *ent.Client, q biz.ToolListQuery) (biz.ToolSummary, error) {
	where, args := toolWhereClause(q)
	var s biz.ToolSummary
	if err := entQueryRowScan(client, ctx, `
		SELECT
		  COALESCE(COUNT(1), 0),
		  COALESCE(SUM(CASE WHEN enabled = 1 THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN enabled = 1 AND risk_level IN ('high', 'critical') THEN 1 ELSE 0 END), 0)
		FROM tools t WHERE `+where, args,
		&s.TotalTools, &s.EnabledTools, &s.HighRiskEnabled); err != nil {
		return biz.ToolSummary{}, err
	}
	cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	var success24h, failed24h, blocked24h int
	if err := entQueryRowScan(client, ctx, `
		SELECT
		  COALESCE(COUNT(1), 0),
		  COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END), 0)
		FROM tool_invocations WHERE started_at >= ?`,
		[]any{cutoff},
		&s.Calls24h, &success24h, &failed24h, &blocked24h); err != nil {
		return biz.ToolSummary{}, err
	}
	if s.Calls24h > 0 {
		s.FailureRate24h = float64(failed24h+blocked24h) / float64(s.Calls24h)
	}
	_ = success24h
	return s, nil
}

func (r *toolRepo) SearchTools(ctx context.Context, q biz.ToolListQuery) (biz.ToolListResult, error) {
	client := r.data.Ent()
	if client == nil {
		return biz.ToolListResult{}, errors.New("ent client unavailable")
	}
	cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	where, args := toolWhereClause(q)
	var total int
	if err := entQueryRowScan(client, ctx, `SELECT COUNT(1) FROM tools t WHERE `+where, args, &total); err != nil {
		return biz.ToolListResult{}, err
	}
	listArgs := append([]any{cutoff}, args...)
	listArgs = append(listArgs, q.Limit, q.Offset)
	rows, err := client.QueryContext(ctx, toolSelectSQL()+` WHERE `+where+` ORDER BY t.category ASC, t.display_name ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return biz.ToolListResult{}, err
	}
	defer rows.Close()
	items, err := scanBizTool(rows)
	if err != nil {
		return biz.ToolListResult{}, err
	}
	summary, err := r.computeToolSummary(ctx, client, q)
	if err != nil {
		return biz.ToolListResult{}, err
	}
	return biz.ToolListResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset, Summary: summary}, nil
}

func (r *toolRepo) GetTool(ctx context.Context, idOrKey string) (biz.Tool, error) {
	client := r.data.Ent()
	if client == nil {
		return biz.Tool{}, errors.New("ent client unavailable")
	}
	cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	rows, err := client.QueryContext(ctx, toolSelectSQL()+` WHERE (t.id = ? OR t.tool_key = ?) AND t.deleted_at = '' LIMIT 1`, cutoff, idOrKey, idOrKey)
	if err != nil {
		return biz.Tool{}, err
	}
	defer rows.Close()
	items, err := scanBizTool(rows)
	if err != nil {
		return biz.Tool{}, err
	}
	if len(items) == 0 {
		return biz.Tool{}, sql.ErrNoRows
	}
	return items[0], nil
}

func applyBuiltinToolDefaults(in *biz.ToolUpsertInput) {
	if strings.TrimSpace(in.Key) == "" {
		return
	}
	if in.Source == "" {
		in.Source = "builtin"
	}
	if in.RiskLevel == "" {
		in.RiskLevel = "low"
	}
	if in.ParametersSchemaJSON == "" {
		in.ParametersSchemaJSON = "{}"
	}
	if in.ResultSchemaJSON == "" {
		in.ResultSchemaJSON = "{}"
	}
	if in.ConfigSchemaJSON == "" {
		in.ConfigSchemaJSON = "{}"
	}
	if in.ConfigJSON == "" {
		in.ConfigJSON = "{}"
	}
	if in.DefaultConfigJSON == "" {
		in.DefaultConfigJSON = in.ConfigJSON
	}
	if in.MetadataJSON == "" {
		in.MetadataJSON = "{}"
	}
}

func (r *toolRepo) CreateTool(ctx context.Context, in biz.ToolUpsertInput) (biz.Tool, error) {
	if strings.TrimSpace(in.Key) == "" {
		return biz.Tool{}, errors.New("tool key is required")
	}
	applyBuiltinToolDefaults(&in)
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = "tool_" + strings.ReplaceAll(strings.TrimSpace(in.Key), "-", "_")
	}
	if id == "" {
		id = uniqueToolID("tool")
	}
	now := nowRFC3339()
	err := r.data.Ent().PlatformTool.Create().
		SetID(id).
		SetToolKey(strings.TrimSpace(in.Key)).
		SetDisplayName(strings.TrimSpace(in.DisplayName)).
		SetDescription(strings.TrimSpace(in.Description)).
		SetCategory(strings.TrimSpace(in.Category)).
		SetSource(strings.TrimSpace(in.Source)).
		SetRiskLevel(strings.TrimSpace(in.RiskLevel)).
		SetEnabled(in.Enabled).
		SetReadonly(in.Readonly).
		SetRequiresConfirmation(in.RequiresConfirmation).
		SetSupportsStreaming(in.SupportsStreaming).
		SetSupportsConcurrency(in.SupportsConcurrency).
		SetParametersSchemaJSON(in.ParametersSchemaJSON).
		SetResultSchemaJSON(in.ResultSchemaJSON).
		SetConfigSchemaJSON(in.ConfigSchemaJSON).
		SetConfigJSON(in.ConfigJSON).
		SetFallbackConfigJSON(in.DefaultConfigJSON).
		SetMetadataJSON(in.MetadataJSON).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		SetDeletedAt("").
		Exec(ctx)
	if err != nil {
		return biz.Tool{}, err
	}
	return r.GetTool(ctx, id)
}

func (r *toolRepo) toolByIDOrKey(ctx context.Context, idOrKey string) (*ent.PlatformTool, error) {
	return r.data.Ent().PlatformTool.Query().
		Where(
			platformtool.DeletedAtEQ(""),
			platformtool.Or(platformtool.IDEQ(idOrKey), platformtool.ToolKeyEQ(idOrKey)),
		).
		Only(ctx)
}

func (r *toolRepo) UpdateTool(ctx context.Context, idOrKey string, in biz.ToolUpsertInput) (biz.Tool, error) {
	ex, err := r.toolByIDOrKey(ctx, idOrKey)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Tool{}, sql.ErrNoRows
		}
		return biz.Tool{}, err
	}
	applyBuiltinToolDefaults(&in)
	key := strings.TrimSpace(in.Key)
	if key == "" {
		key = ex.ToolKey
	}
	now := nowRFC3339()
	err = r.data.Ent().PlatformTool.UpdateOneID(ex.ID).
		SetToolKey(key).
		SetDisplayName(strings.TrimSpace(in.DisplayName)).
		SetDescription(strings.TrimSpace(in.Description)).
		SetCategory(strings.TrimSpace(in.Category)).
		SetSource(strings.TrimSpace(in.Source)).
		SetRiskLevel(strings.TrimSpace(in.RiskLevel)).
		SetEnabled(in.Enabled).
		SetReadonly(in.Readonly).
		SetRequiresConfirmation(in.RequiresConfirmation).
		SetSupportsStreaming(in.SupportsStreaming).
		SetSupportsConcurrency(in.SupportsConcurrency).
		SetParametersSchemaJSON(in.ParametersSchemaJSON).
		SetResultSchemaJSON(in.ResultSchemaJSON).
		SetConfigSchemaJSON(in.ConfigSchemaJSON).
		SetConfigJSON(in.ConfigJSON).
		SetFallbackConfigJSON(in.DefaultConfigJSON).
		SetMetadataJSON(in.MetadataJSON).
		SetUpdatedAt(now).
		SetDeletedAt("").
		Exec(ctx)
	if err != nil {
		return biz.Tool{}, err
	}
	return r.GetTool(ctx, key)
}

func (r *toolRepo) DeleteTool(ctx context.Context, idOrKey string) error {
	ex, err := r.toolByIDOrKey(ctx, idOrKey)
	if err != nil {
		if ent.IsNotFound(err) {
			return sql.ErrNoRows
		}
		return err
	}
	now := nowRFC3339()
	err = r.data.Ent().PlatformTool.UpdateOneID(ex.ID).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Exec(ctx)
	return err
}

func (r *toolRepo) UpdateToolEnabled(ctx context.Context, idOrKey string, enabled bool) (biz.Tool, error) {
	ex, err := r.toolByIDOrKey(ctx, idOrKey)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Tool{}, sql.ErrNoRows
		}
		return biz.Tool{}, err
	}
	err = r.data.Ent().PlatformTool.UpdateOneID(ex.ID).
		SetEnabled(enabled).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
	if err != nil {
		return biz.Tool{}, err
	}
	return r.GetTool(ctx, ex.ToolKey)
}

func (r *toolRepo) SearchToolInvocations(ctx context.Context, q biz.ToolRunQuery) (biz.ToolRunResult, error) {
	client := r.data.Ent()
	if client == nil {
		return biz.ToolRunResult{}, errors.New("ent client unavailable")
	}
	where := []string{"1 = 1"}
	args := []any{}
	if q.ToolKey != "" {
		where = append(where, "ti.tool_key = ?")
		args = append(args, q.ToolKey)
	}
	if q.AgentID != "" {
		where = append(where, "ti.agent_id = ?")
		args = append(args, q.AgentID)
	}
	if q.SessionID != "" {
		where = append(where, "ti.session_id = ?")
		args = append(args, q.SessionID)
	}
	if q.Status != "" {
		where = append(where, "ti.status = ?")
		args = append(args, q.Status)
	}
	if q.From != "" {
		where = append(where, "ti.started_at >= ?")
		args = append(args, q.From)
	}
	if q.To != "" {
		where = append(where, "ti.started_at <= ?")
		args = append(args, q.To)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := entQueryRowScan(client, ctx, `SELECT COUNT(1) FROM tool_invocations ti WHERE `+whereSQL, args, &total); err != nil {
		return biz.ToolRunResult{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, q.Limit, q.Offset)
	rows, err := client.QueryContext(ctx, `
		SELECT ti.id, ti.request_id, ti.invocation_id, ti.tool_id, ti.tool_key, COALESCE(t.display_name, ti.tool_key),
		       ti.agent_id, ti.agent_key, COALESCE(a.display_name, ''), ti.session_id, ti.message_id, ti.user_id,
		       ti.source, ti.status, ti.started_at, ti.ended_at, ti.duration_ms,
		       ti.input_preview, ti.input_hash, ti.output_preview, ti.output_hash,
		       ti.error_code, ti.error_message, ti.redaction_applied, ti.metadata_json, ti.created_at
		FROM tool_invocations ti
		LEFT JOIN tools t ON t.tool_key = ti.tool_key
		LEFT JOIN agents a ON a.id = ti.agent_id
		WHERE `+whereSQL+`
		ORDER BY ti.started_at DESC, ti.created_at DESC
		LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return biz.ToolRunResult{}, err
	}
	defer rows.Close()
	items := []biz.ToolInvocation{}
	for rows.Next() {
		var item biz.ToolInvocation
		var redact int64
		if err := rows.Scan(
			&item.ID, &item.RequestID, &item.InvocationID, &item.ToolID, &item.ToolKey, &item.ToolDisplayName,
			&item.AgentID, &item.AgentKey, &item.AgentDisplayName, &item.SessionID, &item.MessageID, &item.UserID,
			&item.Source, &item.Status, &item.StartedAt, &item.EndedAt, &item.DurationMS,
			&item.InputPreview, &item.InputHash, &item.OutputPreview, &item.OutputHash,
			&item.ErrorCode, &item.ErrorMessage, &redact, &item.MetadataJSON, &item.CreatedAt,
		); err != nil {
			return biz.ToolRunResult{}, err
		}
		item.RedactionApplied = redact != 0
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return biz.ToolRunResult{}, err
	}
	return biz.ToolRunResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}
