package data

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
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
		       stats.avg_duration_ms, COALESCE(p95.p95_duration_ms, 0), COALESCE(last.started_at, ''), COALESCE(last.status, ''),
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
			SELECT ti95.tool_key, AVG(ti95.duration_ms) AS p95_duration_ms
			FROM tool_invocations ti95
			INNER JOIN (
				SELECT tool_key, MIN(duration_ms) AS threshold_ms
				FROM (
					SELECT tool_key, duration_ms,
					       ROW_NUMBER() OVER (PARTITION BY tool_key ORDER BY duration_ms DESC) AS rn,
					       COUNT(1) OVER (PARTITION BY tool_key) AS total_cnt
					FROM tool_invocations
				)
				WHERE rn <= MAX(1, CAST(total_cnt * 0.05 AS INTEGER))
				GROUP BY tool_key
			) top5 ON top5.tool_key = ti95.tool_key AND ti95.duration_ms >= top5.threshold_ms
			GROUP BY ti95.tool_key
		) p95 ON p95.tool_key = t.tool_key
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
		var p95 float64
		if err := rows.Scan(
			&item.ID, &item.Key, &item.DisplayName, &item.Description, &item.Category, &item.Source, &item.RiskLevel,
			&item.Enabled, &item.Readonly, &item.RequiresConfirmation, &item.SupportsStreaming, &item.SupportsConcurrency,
			&item.ParametersSchemaJSON, &item.ResultSchemaJSON, &item.ConfigSchemaJSON, &item.ConfigJSON, &item.DefaultConfigJSON, &item.MetadataJSON,
			&item.InvokeCount, &item.InvokeCount24h, &item.SuccessCount, &item.FailureCount, &item.BlockedCount, &item.AgentOverrideCount,
			&avg, &p95, &item.LastInvokedAt, &item.LastStatus,
			&item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
		); err != nil {
			return nil, err
		}
		if avg.Valid {
			v := avg.Float64
			item.AvgDurationMS = &v
		}
		item.P95DurationMS = p95
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
	orderBy := "t.category ASC, t.display_name ASC"
	switch q.Sort {
	case "last_invoked_at":
		orderBy = "last.started_at DESC NULLS LAST, t.display_name ASC"
	case "invoke_count":
		orderBy = "stats.invoke_count DESC NULLS LAST, t.display_name ASC"
	case "failure_rate":
		orderBy = "CASE WHEN stats.invoke_count > 0 THEN CAST(stats.failure_count AS REAL) / stats.invoke_count ELSE 0 END DESC, t.display_name ASC"
	case "avg_duration_ms":
		orderBy = "stats.avg_duration_ms DESC NULLS LAST, t.display_name ASC"
	}
	rows, err := client.QueryContext(ctx, toolSelectSQL()+` WHERE `+where+` ORDER BY `+orderBy+` LIMIT ? OFFSET ?`, listArgs...)
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

func (r *toolRepo) UpdateToolConfig(ctx context.Context, idOrKey string, configJSON string) (biz.Tool, error) {
	ex, err := r.toolByIDOrKey(ctx, idOrKey)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Tool{}, sql.ErrNoRows
		}
		return biz.Tool{}, err
	}
	err = r.data.Ent().PlatformTool.UpdateOneID(ex.ID).
		SetConfigJSON(configJSON).
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
	if q.HasError != nil {
		if *q.HasError {
			where = append(where, "ti.status = 'error'")
		} else {
			where = append(where, "ti.status != 'error'")
		}
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

func (r *toolRepo) RecordToolInvocation(ctx context.Context, in biz.ToolInvocationWrite) error {
	client := r.data.Ent()
	if client == nil {
		return errors.New("ent client unavailable")
	}
	now := nowRFC3339()
	started := strings.TrimSpace(in.StartedAt)
	if started == "" {
		started = now
	}
	ended := strings.TrimSpace(in.EndedAt)
	if ended == "" {
		ended = now
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		if in.ErrorMessage != "" {
			status = "error"
		} else {
			status = "success"
		}
	}
	source := strings.TrimSpace(in.Source)
	if source == "" {
		source = "adk"
	}
	inputPreview := in.InputPreview
	if len(inputPreview) > 2000 {
		inputPreview = inputPreview[:2000]
	}
	outputPreview := in.OutputPreview
	if len(outputPreview) > 2000 {
		outputPreview = outputPreview[:2000]
	}
	id := uniqueToolID("tinv")
	_, err := client.ToolInvocation.Create().
		SetID(id).
		SetToolKey(strings.TrimSpace(in.ToolKey)).
		SetAgentID(strings.TrimSpace(in.AgentID)).
		SetAgentKey(strings.TrimSpace(in.AgentKey)).
		SetSessionID(strings.TrimSpace(in.SessionID)).
		SetUserID(strings.TrimSpace(in.UserID)).
		SetSource(source).
		SetStatus(status).
		SetStartedAt(started).
		SetEndedAt(ended).
		SetDurationMs(in.DurationMS).
		SetInputPreview(inputPreview).
		SetInputHash(hashTrim(in.InputHash, in.InputPreview)).
		SetOutputPreview(outputPreview).
		SetOutputHash(hashTrim(in.OutputHash, in.OutputPreview)).
		SetErrorCode(strings.TrimSpace(in.ErrorCode)).
		SetErrorMessage(strings.TrimSpace(in.ErrorMessage)).
		SetRedactionApplied(true).
		SetMetadataJSON(invocationMetaJSON(in)).
		SetCreatedAt(now).
		Save(ctx)
	return err
}

func hashTrim(explicit, fallback string) string {
	if h := strings.TrimSpace(explicit); h != "" {
		if len(h) > 64 {
			return h[:64]
		}
		return h
	}
	s := strings.TrimSpace(fallback)
	if s == "" {
		return ""
	}
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h)
}

func invocationMetaJSON(in biz.ToolInvocationWrite) string {
	m := map[string]string{}
	if in.ToolCallID != "" {
		m["tool_call_id"] = in.ToolCallID
	}
	if len(m) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func (r *toolRepo) SyncBuiltinTools(ctx context.Context) error {
	return syncBuiltinToolsFromRegistry(ctx, r.data.Ent())
}

func (r *toolRepo) ListToolAgentOverrides(ctx context.Context, toolKey string) ([]biz.ToolAgentOverride, error) {
	client := r.data.Ent()
	if client == nil {
		return nil, errors.New("ent client unavailable")
	}
	rows, err := client.QueryContext(ctx, `
		SELECT id, COALESCE(tool_id, ''), tool_key, agent_id, enabled, mode, config_override_json, requires_confirmation, created_at, updated_at
		FROM tool_agent_overrides
		WHERE tool_key = ? AND deleted_at = ''
		ORDER BY agent_id`, toolKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []biz.ToolAgentOverride
	for rows.Next() {
		var o biz.ToolAgentOverride
		var enabled int
		var reqConfirm int
		if err := rows.Scan(&o.ID, &o.ToolID, &o.ToolKey, &o.AgentID, &enabled, &o.Mode, &o.ConfigOverrideJSON, &reqConfirm, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		o.Enabled = enabled != 0
		o.RequiresConfirmation = reqConfirm != 0
		result = append(result, o)
	}
	return result, rows.Err()
}

func (r *toolRepo) UpsertToolAgentOverride(ctx context.Context, in biz.ToolAgentOverrideInput) (biz.ToolAgentOverride, error) {
	client := r.data.Ent()
	if client == nil {
		return biz.ToolAgentOverride{}, errors.New("ent client unavailable")
	}
	now := nowRFC3339()
	const q = `INSERT INTO tool_agent_overrides (id, tool_id, tool_key, agent_id, enabled, mode, config_override_json, requires_confirmation, created_at, updated_at, deleted_at)
		VALUES (?, '', ?, ?, ?, ?, ?, ?, ?, ?, '')
		ON CONFLICT(tool_key, agent_id) DO UPDATE SET
			enabled = excluded.enabled,
			mode = excluded.mode,
			config_override_json = excluded.config_override_json,
			requires_confirmation = excluded.requires_confirmation,
			updated_at = excluded.updated_at,
			deleted_at = ''`
	id := uniqueToolID("tao")
	_, err := client.ExecContext(ctx, q,
		id, in.ToolKey, in.AgentID, b2i(in.Enabled), in.Mode, in.ConfigOverrideJSON, b2i(in.RequiresConfirmation),
		now, now,
	)
	if err != nil {
		return biz.ToolAgentOverride{}, err
	}
	overrides, err := r.ListToolAgentOverrides(ctx, in.ToolKey)
	if err != nil {
		return biz.ToolAgentOverride{}, err
	}
	for _, o := range overrides {
		if o.AgentID == in.AgentID {
			return o, nil
		}
	}
	return biz.ToolAgentOverride{}, nil
}

func (r *toolRepo) DeleteToolAgentOverride(ctx context.Context, toolKey string, agentID string) error {
	client := r.data.Ent()
	if client == nil {
		return errors.New("ent client unavailable")
	}
	now := nowRFC3339()
	_, err := client.ExecContext(ctx, `
		UPDATE tool_agent_overrides SET deleted_at = ?, updated_at = ?
		WHERE tool_key = ? AND agent_id = ? AND deleted_at = ''`,
		now, now, toolKey, agentID,
	)
	return err
}

func (r *toolRepo) GetToolInvocationParams(ctx context.Context, invocationID string) (biz.ToolInvocationParam, error) {
	client := r.data.Ent()
	if client == nil {
		return biz.ToolInvocationParam{}, errors.New("ent client unavailable")
	}
	rows, err := client.QueryContext(ctx, `
		SELECT id, invocation_id, tool_key, params_json, redaction_applied, created_at
		FROM tool_invocation_params
		WHERE invocation_id = ?
		LIMIT 1`, invocationID)
	if err != nil {
		return biz.ToolInvocationParam{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.ToolInvocationParam{}, sql.ErrNoRows
	}
	var p biz.ToolInvocationParam
	var redaction int
	if err := rows.Scan(&p.ID, &p.InvocationID, &p.ToolKey, &p.ParamsJSON, &redaction, &p.CreatedAt); err != nil {
		return biz.ToolInvocationParam{}, err
	}
	p.RedactionApplied = redaction != 0
	return p, nil
}
