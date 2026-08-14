package data

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/platformtool"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type toolRepo struct {
	data *Data
}

var _ biz.ToolRepo = (*toolRepo)(nil)

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

// truncateUTF8 truncates s to at most maxBytes bytes without splitting a
// multi-byte UTF-8 rune. Postgres text columns reject invalid UTF-8 (error
// 22021), so naive s[:n] byte-slicing on preview/summary fields can turn a
// large CJK payload into a failed INSERT and lose the whole record.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

// toolStatsWindowDays bounds the tool_invocations scans in toolSelectSQL's
// stats/p95/last subqueries (PERF-3). tool_invocations has no retention purge
// (only session-cascade delete), so without a window the admin tool-list query
// degrades as the table grows. The window is aligned with
// biz.ToolAuditRetentionDays (90): audit rows and statistics share one
// "recent history" horizon. Semantic note: InvokeCount/Avg/P95/LastInvokedAt
// are windowed metrics — a tool with no calls in the window reports zeros and
// an empty LastInvokedAt.
const toolStatsWindowDays = 90

// toolSelectPrefixArgs returns the leading args for toolSelectSQL in
// placeholder order: the 24h cutoff first (stats subquery's invoke_count_24h
// CASE), then one stats-window cutoff per windowed subquery (stats FROM, p95
// inner, last inner). Callers append their own WHERE/LIMIT args after these.
func toolSelectPrefixArgs() []any {
	now := time.Now().UTC()
	cutoff24h := now.Add(-24 * time.Hour).Format(time.RFC3339)
	window := now.AddDate(0, 0, -toolStatsWindowDays).Format(time.RFC3339)
	return []any{cutoff24h, window, window, window}
}

// toolP95Subquery returns the per-tool p95 duration subquery (one stats-window
// placeholder). Postgres uses percentile_cont(0.95) WITHIN GROUP — an exact
// interpolated percentile (PERF-2), replacing the old "average of the top 5%"
// approximation which overstated p95 on skewed latencies. SQLite has no
// ordered-set aggregates, so it keeps the top-5% AVG approximation (dev/CLI
// only — production is always Postgres).
func toolP95Subquery(d Dialect) string {
	if d.IsPostgres() {
		return `
			SELECT tool_key,
			       percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms) AS p95_duration_ms
			FROM tool_invocations
			WHERE started_at >= ?
			GROUP BY tool_key`
	}
	topN := d.Greatest("1", "CAST(total_cnt * 0.05 AS INTEGER)")
	return `
			SELECT tool_key, AVG(duration_ms) AS p95_duration_ms
			FROM (
				SELECT tool_key, duration_ms,
				       ROW_NUMBER() OVER (PARTITION BY tool_key ORDER BY duration_ms DESC) AS rn,
				       COUNT(1) OVER (PARTITION BY tool_key) AS total_cnt
				FROM tool_invocations
				WHERE started_at >= ?
			)
			WHERE rn <= ` + topN + `
			GROUP BY tool_key`
}

func toolSelectSQL(d Dialect) string {
	// d.Greatest emits GREATEST on Postgres (MAX is aggregate-only) and MAX on
	// SQLite (multi-argument MAX is a scalar function). Using GREATEST
	// unconditionally breaks SQLite drivers that don't support it (e.g.
	// glebarez/go-sqlite), and using MAX unconditionally breaks Postgres
	// (error 42883: function max(integer, integer) does not exist).
	//
	// Status semantics: the runtime recorder writes 'failed' for tool errors
	// (tool_invocation_recorder.go), while legacy paths (a2a/testexec) wrote
	// 'error'. failure_count must cover both or runtime failures vanish from
	// the tools list.
	//
	// Args quality (args_repaired/args_invalid booleans in metadata_json,
	// written by the repair guard): SQLite json_extract yields integer 1 for
	// JSON true; Postgres ->> yields text 'true'.
	repairedExpr := d.JSONExtract("metadata_json", "args_repaired")
	invalidExpr := d.JSONExtract("metadata_json", "args_invalid")
	trueLiteral := "'true'"
	if d.IsSQLite() {
		trueLiteral = "1"
	}
	return `
		SELECT t.id, t.tool_key, t.display_name, t.description, t.category, t.source, t.risk_level,
		       t.enabled, t.readonly, t.requires_confirmation, t.supports_streaming, t.supports_concurrency,
		       t.parameters_schema_json, t.result_schema_json, t.config_schema_json, t.config_json, t.default_config_json, t.metadata_json,
		       COALESCE(stats.invoke_count, 0), COALESCE(stats.invoke_count_24h, 0), COALESCE(stats.success_count, 0),
		       COALESCE(stats.failure_count, 0), COALESCE(stats.blocked_count, 0), COALESCE(overrides.agent_override_count, 0),
		       COALESCE(stats.repaired_count, 0), COALESCE(stats.invalid_count, 0),
		       stats.avg_duration_ms, COALESCE(p95.p95_duration_ms, 0), COALESCE(last.started_at, ''), COALESCE(last.status, ''),
		       t.created_at, t.updated_at, t.deleted_at, t.workspace_id
		FROM tools t
		LEFT JOIN (
			SELECT tool_key,
			       COUNT(1) AS invoke_count,
			       SUM(CASE WHEN started_at >= ? THEN 1 ELSE 0 END) AS invoke_count_24h,
			       SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) AS success_count,
			       SUM(CASE WHEN status IN ('failed', 'error') THEN 1 ELSE 0 END) AS failure_count,
			       SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END) AS blocked_count,
			       SUM(CASE WHEN ` + repairedExpr + ` = ` + trueLiteral + ` THEN 1 ELSE 0 END) AS repaired_count,
			       SUM(CASE WHEN ` + invalidExpr + ` = ` + trueLiteral + ` THEN 1 ELSE 0 END) AS invalid_count,
			       AVG(duration_ms) AS avg_duration_ms
			FROM tool_invocations
			WHERE started_at >= ?
			GROUP BY tool_key
		) stats ON stats.tool_key = t.tool_key
		LEFT JOIN (` + toolP95Subquery(d) + `
		) p95 ON p95.tool_key = t.tool_key
		LEFT JOIN (
			SELECT tool_key, COUNT(1) AS agent_override_count
			FROM tool_agent_overrides
			WHERE deleted_at = ''
			GROUP BY tool_key
		) overrides ON overrides.tool_key = t.tool_key
		LEFT JOIN (
			SELECT tool_key, started_at, status
			FROM (
				SELECT ti.tool_key, ti.started_at, ti.status,
				       ROW_NUMBER() OVER (PARTITION BY ti.tool_key ORDER BY ti.started_at DESC, ti.id DESC) AS rn
				FROM tool_invocations ti
				WHERE ti.started_at >= ?
			)
			WHERE rn = 1
		) last ON last.tool_key = t.tool_key`
}

func toolWhereClause(q biz.ToolListQuery) (string, []any) {
	where := []string{"t.deleted_at = ''"}
	args := []any{}
	// P2-B: workspace visibility filter.
	// empty WorkspaceID = system caller (see all); non-empty = tenant caller (shared + own).
	if ws := strings.TrimSpace(q.WorkspaceID); ws != "" {
		where = append(where, "(t.workspace_id = '' OR t.workspace_id = ?)")
		args = append(args, ws)
	}
	if search := strings.TrimSpace(q.Search); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = append(where, "(LOWER(t.tool_key) LIKE ? OR LOWER(t.display_name) LIKE ? OR LOWER(t.description) LIKE ? OR LOWER(t.category) LIKE ?)")
		args = append(args, like, like, like, like)
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
	if q.Abnormal {
		// 仅看异常：最近一次调用（按 started_at/id 倒序首行）以失败/阻塞收尾。
		// 'failed' 为运行时口径，'error' 为遗留口径（a2a/testexec）。
		// 相关子查询只依赖 t.tool_key，COUNT 查询（无 join）与列表查询均可复用。
		where = append(where, `COALESCE((
			SELECT ti.status FROM tool_invocations ti
			WHERE ti.tool_key = t.tool_key
			ORDER BY ti.started_at DESC, ti.id DESC
			LIMIT 1
		), '') IN ('failed', 'error', 'blocked')`)
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
			&item.RepairedCount, &item.InvalidCount,
			&avg, &p95, &item.LastInvokedAt, &item.LastStatus,
			&item.CreatedAt, &item.UpdatedAt, &item.DeletedAt, &item.WorkspaceID,
		); err != nil {
			return nil, entErrToBizErr(err, "TOOL")
		}
		if avg.Valid {
			v := avg.Float64
			item.AvgDurationMS = &v
		}
		item.P95DurationMS = p95
		item.Permissions = adminToolPerms()
		out = append(out, item)
	}
	return out, entErrToBizErr(rows.Err(), "TOOL")
}

func (r *toolRepo) computeToolSummary(ctx context.Context, client *ent.Client, q biz.ToolListQuery) (biz.ToolSummary, error) {
	where, args := toolWhereClause(q)
	var s biz.ToolSummary
	if err := entQueryRowScan(client, ctx, r.data.Dialect().RenumberPlaceholders(`
		SELECT
		  COALESCE(COUNT(1), 0),
		  COALESCE(SUM(CASE WHEN t.enabled THEN 1 ELSE 0 END), 0),
      COALESCE(SUM(CASE WHEN t.enabled AND risk_level IN ('high', 'critical') THEN 1 ELSE 0 END), 0)
		FROM tools t WHERE `+where), args,
		&s.TotalTools, &s.EnabledTools, &s.HighRiskEnabled); err != nil {
		return biz.ToolSummary{}, entErrToBizErr(err, "TOOL")
	}
	cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	var success24h, failed24h, blocked24h int
	if err := entQueryRowScan(client, ctx, r.data.Dialect().RenumberPlaceholders(`
	SELECT
	  COALESCE(COUNT(1), 0),
	  COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0),
	  COALESCE(SUM(CASE WHEN status IN ('failed', 'error') THEN 1 ELSE 0 END), 0),
	  COALESCE(SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END), 0)
	FROM tool_invocations WHERE started_at >= ?`),
		[]any{cutoff},
		&s.Calls24h, &success24h, &failed24h, &blocked24h); err != nil {
		return biz.ToolSummary{}, entErrToBizErr(err, "TOOL")
	}
	if s.Calls24h > 0 {
		s.FailureRate24h = float64(failed24h+blocked24h) / float64(s.Calls24h)
	}
	_ = success24h // already accounted for in Calls24h
	return s, nil
}

func (r *toolRepo) SearchTools(ctx context.Context, q biz.ToolListQuery) (biz.ToolListResult, error) {
	client := r.data.RW().Read(ctx)
	if client == nil {
		return biz.ToolListResult{}, apierror.Internal("TOOL", "ent client unavailable")
	}
	where, args := toolWhereClause(q)
	var total int
	if err := entQueryRowScan(client, ctx, r.data.Dialect().RenumberPlaceholders(`SELECT COUNT(1) FROM tools t WHERE `+where), args, &total); err != nil {
		r.data.lg.Warn("tool search count query failed", loggateway.StepID("data.tool.search"), loggateway.Err(err))
		return biz.ToolListResult{}, entErrToBizErr(err, "TOOL")
	}
	listArgs := append(toolSelectPrefixArgs(), args...)
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
	rows, err := client.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(toolSelectSQL(r.data.Dialect())+` WHERE `+where+` ORDER BY `+orderBy+` LIMIT ? OFFSET ?`), listArgs...)
	if err != nil {
		r.data.lg.Warn("tool search list query failed", loggateway.StepID("data.tool.search"), loggateway.Err(err))
		return biz.ToolListResult{}, entErrToBizErr(err, "TOOL")
	}
	defer rows.Close()
	items, err := scanBizTool(rows)
	if err != nil {
		r.data.lg.Warn("tool search scan failed", loggateway.StepID("data.tool.search"), loggateway.Err(err))
		return biz.ToolListResult{}, entErrToBizErr(err, "TOOL")
	}
	summary, err := r.computeToolSummary(ctx, client, q)
	if err != nil {
		r.data.lg.Warn("tool search summary failed", loggateway.StepID("data.tool.search"), loggateway.Err(err))
		return biz.ToolListResult{}, entErrToBizErr(err, "TOOL")
	}
	return biz.ToolListResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset, Summary: summary}, nil
}

func (r *toolRepo) GetTool(ctx context.Context, idOrKey string) (biz.Tool, error) {
	client := r.data.RW().Read(ctx)
	if client == nil {
		return biz.Tool{}, apierror.Internal("TOOL", "ent client unavailable")
	}
	where := `(t.id = ? OR t.tool_key = ?) AND t.deleted_at = ''`
	args := append(toolSelectPrefixArgs(), idOrKey, idOrKey)
	// C-25: shared-or-own workspace filter on Get.
	if ids := workspaceSharedOrOwnIDs(ctx); ids != nil {
		where += ` AND t.workspace_id IN (?, ?)`
		args = append(args, ids[0], ids[1])
	}
	rows, err := client.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(toolSelectSQL(r.data.Dialect())+` WHERE `+where+` LIMIT 1`), args...)
	if err != nil {
		return biz.Tool{}, entErrToBizErr(err, "TOOL")
	}
	defer rows.Close()
	items, err := scanBizTool(rows)
	if err != nil {
		return biz.Tool{}, entErrToBizErr(err, "TOOL")
	}
	if len(items) == 0 {
		return biz.Tool{}, apierror.NotFound("TOOL", "tool not found")
	}
	return items[0], nil
}

// ListToolCatalogEntries batch-loads lightweight build-time catalog rows in a
// single IN query. It deliberately avoids toolSelectSQL's aggregation joins
// (stats/p95/overrides/last), which made per-key GetTool loops cost ~49ms per
// tool. Callers must treat absent keys as "unknown" (fail-soft for runtime
// configs, fail-closed for the confirmation gate).
func (r *toolRepo) ListToolCatalogEntries(ctx context.Context, keys []string) ([]biz.ToolCatalogEntry, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	client := r.data.RW().Read(ctx)
	if client == nil {
		return nil, apierror.Internal("TOOL", "ent client unavailable")
	}
	where := `t.deleted_at = '' AND t.tool_key IN (` + strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",") + `)`
	args := make([]any, 0, len(keys)+2)
	for _, k := range keys {
		args = append(args, k)
	}
	// C-25: shared-or-own workspace filter, matching GetTool.
	if ids := workspaceSharedOrOwnIDs(ctx); ids != nil {
		where += ` AND t.workspace_id IN (?, ?)`
		args = append(args, ids[0], ids[1])
	}
	q := `SELECT t.tool_key, t.config_json, t.default_config_json, t.requires_confirmation, t.metadata_json FROM tools t WHERE ` + where
	rows, err := client.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "TOOL")
	}
	defer rows.Close()
	var out []biz.ToolCatalogEntry
	for rows.Next() {
		var e biz.ToolCatalogEntry
		if err := rows.Scan(&e.Key, &e.ConfigJSON, &e.DefaultConfigJSON, &e.RequiresConfirmation, &e.MetadataJSON); err != nil {
			return nil, entErrToBizErr(err, "TOOL")
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, entErrToBizErr(err, "TOOL")
	}
	return out, nil
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
		return biz.Tool{}, apierror.BadRequest("TOOL", "tool key is required")
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
	err := r.data.RW().Write(ctx).PlatformTool.Create().
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
		SetWorkspaceID(in.WorkspaceID). // P2-B: tenant isolation
		Exec(ctx)
	if err != nil {
		return biz.Tool{}, entErrToBizErr(err, "TOOL")
	}
	return r.GetTool(ctx, id)
}

func (r *toolRepo) toolByIDOrKey(ctx context.Context, idOrKey string) (*ent.PlatformTool, error) {
	return r.data.RW().Read(ctx).PlatformTool.Query().
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
			return biz.Tool{}, apierror.NotFound("TOOL", "tool not found")
		}
		return biz.Tool{}, entErrToBizErr(err, "TOOL")
	}
	applyBuiltinToolDefaults(&in)
	key := strings.TrimSpace(in.Key)
	if key == "" {
		key = ex.ToolKey
	}
	now := nowRFC3339()
	err = r.data.RW().Write(ctx).PlatformTool.UpdateOneID(ex.ID).
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
		return biz.Tool{}, entErrToBizErr(err, "TOOL")
	}
	return r.GetTool(ctx, key)
}

func (r *toolRepo) DeleteTool(ctx context.Context, idOrKey string) error {
	ex, err := r.toolByIDOrKey(ctx, idOrKey)
	if err != nil {
		if ent.IsNotFound(err) {
			return apierror.NotFound("TOOL", "tool not found")
		}
		return entErrToBizErr(err, "TOOL")
	}
	now := nowRFC3339()
	err = r.data.RW().Write(ctx).PlatformTool.UpdateOneID(ex.ID).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Exec(ctx)
	return entErrToBizErr(err, "TOOL")
}

func (r *toolRepo) UpdateToolEnabled(ctx context.Context, idOrKey string, enabled bool) (biz.Tool, error) {
	ex, err := r.toolByIDOrKey(ctx, idOrKey)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Tool{}, apierror.NotFound("TOOL", "tool not found")
		}
		return biz.Tool{}, entErrToBizErr(err, "TOOL")
	}
	err = r.data.RW().Write(ctx).PlatformTool.UpdateOneID(ex.ID).
		SetEnabled(enabled).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
	if err != nil {
		return biz.Tool{}, entErrToBizErr(err, "TOOL")
	}
	return r.GetTool(ctx, ex.ToolKey)
}

func (r *toolRepo) UpdateToolConfig(ctx context.Context, idOrKey string, configJSON string) (biz.Tool, error) {
	ex, err := r.toolByIDOrKey(ctx, idOrKey)
	if err != nil {
		if ent.IsNotFound(err) {
			return biz.Tool{}, apierror.NotFound("TOOL", "tool not found")
		}
		return biz.Tool{}, entErrToBizErr(err, "TOOL")
	}
	err = r.data.RW().Write(ctx).PlatformTool.UpdateOneID(ex.ID).
		SetConfigJSON(configJSON).
		SetUpdatedAt(nowRFC3339()).
		Exec(ctx)
	if err != nil {
		return biz.Tool{}, entErrToBizErr(err, "TOOL")
	}
	return r.GetTool(ctx, ex.ToolKey)
}

func (r *toolRepo) SearchToolInvocations(ctx context.Context, q biz.ToolRunQuery) (biz.ToolRunResult, error) {
	client := r.data.RW().Read(ctx)
	if client == nil {
		return biz.ToolRunResult{}, apierror.Internal("TOOL", "ent client unavailable")
	}
	where := []string{"ti.deleted_at = ''"}
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
			where = append(where, "ti.status IN ('error', 'failed')")
		} else {
			where = append(where, "ti.status NOT IN ('error', 'failed')")
		}
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := entQueryRowScan(client, ctx, r.data.Dialect().RenumberPlaceholders(`SELECT COUNT(1) FROM tool_invocations ti WHERE `+whereSQL), args, &total); err != nil {
		r.data.lg.Warn("tool invocation search count failed", loggateway.StepID("data.tool.invocation_search"), loggateway.Err(err))
		return biz.ToolRunResult{}, entErrToBizErr(err, "TOOL")
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, q.Limit, q.Offset)
	rows, err := client.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		SELECT ti.id, ti.request_id, ti.invocation_id, ti.tool_id, ti.tool_key, COALESCE(t.display_name, ti.tool_key),
		       ti.agent_id, ti.agent_key, COALESCE(a.display_name, ''), ti.session_id, ti.message_id, ti.user_id,
		       ti.source, ti.status, ti.started_at, ti.ended_at, ti.duration_ms,
		       ti.input_preview, ti.input_hash, ti.output_preview, ti.output_hash,
		       ti.error_code, ti.error_message, ti.redaction_applied,
		       COALESCE(ti.streaming, false), COALESCE(ti.chunk_count, 0),
		       ti.metadata_json, ti.created_at
		FROM tool_invocations ti
		LEFT JOIN tools t ON t.tool_key = ti.tool_key
		LEFT JOIN agents a ON a.id = ti.agent_id
		WHERE `+whereSQL+`
		ORDER BY ti.started_at DESC, ti.created_at DESC
		LIMIT ? OFFSET ?`), listArgs...)
	if err != nil {
		return biz.ToolRunResult{}, entErrToBizErr(err, "TOOL")
	}
	defer rows.Close()
	items := []biz.ToolInvocation{}
	for rows.Next() {
		var item biz.ToolInvocation
		var redact bool
		var streaming bool
		if err := rows.Scan(
			&item.ID, &item.RequestID, &item.InvocationID, &item.ToolID, &item.ToolKey, &item.ToolDisplayName,
			&item.AgentID, &item.AgentKey, &item.AgentDisplayName, &item.SessionID, &item.MessageID, &item.UserID,
			&item.Source, &item.Status, &item.StartedAt, &item.EndedAt, &item.DurationMS,
			&item.InputPreview, &item.InputHash, &item.OutputPreview, &item.OutputHash,
			&item.ErrorCode, &item.ErrorMessage, &redact, &streaming, &item.ChunkCount, &item.MetadataJSON, &item.CreatedAt,
		); err != nil {
			return biz.ToolRunResult{}, entErrToBizErr(err, "TOOL")
		}
		item.RedactionApplied = redact
		item.Streaming = streaming
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return biz.ToolRunResult{}, entErrToBizErr(err, "TOOL")
	}
	return biz.ToolRunResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}

func (r *toolRepo) RecordToolInvocation(ctx context.Context, in biz.ToolInvocationWrite) error {
	client := r.data.RW().Write(ctx)
	if client == nil {
		return apierror.Internal("TOOL", "ent client unavailable")
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
		source = biz.ToolInvocationSourceRuntime
	}
	inputPreview := truncateUTF8(in.InputPreview, 2000)
	outputPreview := truncateUTF8(in.OutputPreview, 2000)
	id := uniqueToolID("tinv")
	if tcid := strings.TrimSpace(in.ToolCallID); tcid != "" {
		id = "tinv-" + tcid
		if len(id) > 200 {
			id = id[:200]
		}
	}
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
		SetStreaming(in.Streaming).
		SetChunkCount(in.ChunkCount).
		SetMetadataJSON(invocationMetaJSON(in)).
		SetCreatedAt(now).
		Save(ctx)
	if err == nil {
		return nil
	}
	if !ent.IsConstraintError(err) {
		r.data.lg.Error("tool invocation write failed", loggateway.StepID("data.tool.invocation_write"), loggateway.Err(err))
		return entErrToBizErr(err, "TOOL")
	}
	// Constraint error: same tool_call_id already exists (streaming chunk
	// update). Only update mutable fields — do NOT overwrite identity
	// fields (tool_key, agent_id, session_id, input) to preserve the
	// original invocation record.
	update := client.ToolInvocation.UpdateOneID(id).
		SetEndedAt(ended).
		SetDurationMs(in.DurationMS).
		SetOutputPreview(outputPreview).
		SetOutputHash(hashTrim(in.OutputHash, in.OutputPreview)).
		SetChunkCount(in.ChunkCount).
		SetStatus(status).
		SetMetadataJSON(invocationMetaJSON(in))
	if in.ErrorMessage != "" {
		update = update.SetErrorCode(strings.TrimSpace(in.ErrorCode)).
			SetErrorMessage(strings.TrimSpace(in.ErrorMessage))
	}
	_, err = update.Save(ctx)
	if err != nil {
		r.data.lg.Error("tool invocation fallback update failed", loggateway.StepID("data.tool.invocation_write"), loggateway.Err(err))
	}
	return entErrToBizErr(err, "TOOL")
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
	m := map[string]any{}
	if in.ToolCallID != "" {
		m["tool_call_id"] = in.ToolCallID
	}
	if in.Streaming {
		m["streaming"] = true
	}
	if in.ChunkCount > 0 {
		m["chunk_count"] = in.ChunkCount
	}
	if in.ArgsRepaired {
		m["args_repaired"] = true
	}
	if in.ArgsInvalid {
		m["args_invalid"] = true
	}
	if len(m) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func (r *toolRepo) SyncBuiltinTools(ctx context.Context) error {
	return syncBuiltinToolsFromRegistry(ctx, r.data.RW().Write(ctx), r.data.dialect, r.data.lg)
}

func (r *toolRepo) ListToolAgentOverridesByAgent(ctx context.Context, agentID string) ([]biz.ToolAgentOverride, error) {
	client := r.data.RW().Read(ctx)
	if client == nil {
		return nil, apierror.Internal("TOOL", "ent client unavailable")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, nil
	}
	rows, err := client.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		SELECT id, COALESCE(tool_id, ''), tool_key, agent_id, enabled, mode, config_override_json, requires_confirmation, created_at, updated_at
		FROM tool_agent_overrides
		WHERE agent_id = ? AND deleted_at = ''
		ORDER BY tool_key`), agentID)
	if err != nil {
		return nil, entErrToBizErr(err, "TOOL")
	}
	defer rows.Close()
	return scanToolAgentOverrides(rows)
}

func (r *toolRepo) ListToolAgentOverrides(ctx context.Context, toolKey string) ([]biz.ToolAgentOverride, error) {
	client := r.data.RW().Read(ctx)
	if client == nil {
		return nil, apierror.Internal("TOOL", "ent client unavailable")
	}
	rows, err := client.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		SELECT id, COALESCE(tool_id, ''), tool_key, agent_id, enabled, mode, config_override_json, requires_confirmation, created_at, updated_at
		FROM tool_agent_overrides
		WHERE tool_key = ? AND deleted_at = ''
		ORDER BY agent_id`), toolKey)
	if err != nil {
		return nil, entErrToBizErr(err, "TOOL")
	}
	defer rows.Close()
	return scanToolAgentOverrides(rows)
}

func scanToolAgentOverrides(rows *sql.Rows) ([]biz.ToolAgentOverride, error) {
	var result []biz.ToolAgentOverride
	for rows.Next() {
		var o biz.ToolAgentOverride
		if err := rows.Scan(&o.ID, &o.ToolID, &o.ToolKey, &o.AgentID, &o.Enabled, &o.Mode, &o.ConfigOverrideJSON, &o.RequiresConfirmation, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, entErrToBizErr(err, "TOOL")
		}
		result = append(result, o)
	}
	return result, entErrToBizErr(rows.Err(), "TOOL")
}

func (r *toolRepo) UpsertToolAgentOverride(ctx context.Context, in biz.ToolAgentOverrideInput, toolID string) (biz.ToolAgentOverride, error) {
	client := r.data.RW().Write(ctx)
	if client == nil {
		return biz.ToolAgentOverride{}, apierror.Internal("TOOL", "ent client unavailable")
	}
	now := nowRFC3339()
	toolID = strings.TrimSpace(toolID)
	const q = `INSERT INTO tool_agent_overrides (id, tool_id, tool_key, agent_id, enabled, mode, config_override_json, requires_confirmation, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '')
		ON CONFLICT(tool_key, agent_id) DO UPDATE SET
			tool_id = excluded.tool_id,
			enabled = excluded.enabled,
			mode = excluded.mode,
			config_override_json = excluded.config_override_json,
			requires_confirmation = excluded.requires_confirmation,
			updated_at = excluded.updated_at,
			deleted_at = ''`
	id := uniqueToolID("tao")
	_, err := client.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(q),
		id, toolID, in.ToolKey, in.AgentID, b2i(in.Enabled), in.Mode, in.ConfigOverrideJSON, b2i(in.RequiresConfirmation),
		now, now,
	)
	if err != nil {
		r.data.lg.Warn("tool agent override upsert failed", loggateway.StepID("data.tool.override_upsert"), loggateway.Err(err))
		return biz.ToolAgentOverride{}, entErrToBizErr(err, "TOOL")
	}
	overrides, err := r.ListToolAgentOverrides(ctx, in.ToolKey)
	if err != nil {
		r.data.lg.Warn("tool agent override list after upsert failed", loggateway.StepID("data.tool.override_upsert"), loggateway.Err(err))
		return biz.ToolAgentOverride{}, entErrToBizErr(err, "TOOL")
	}
	for _, o := range overrides {
		if o.AgentID == in.AgentID {
			return o, nil
		}
	}
	return biz.ToolAgentOverride{}, nil
}

func (r *toolRepo) DeleteToolAgentOverride(ctx context.Context, toolKey string, agentID string) error {
	client := r.data.RW().Write(ctx)
	if client == nil {
		return apierror.Internal("TOOL", "ent client unavailable")
	}
	now := nowRFC3339()
	_, err := client.ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		UPDATE tool_agent_overrides SET deleted_at = ?, updated_at = ?
		WHERE tool_key = ? AND agent_id = ? AND deleted_at = ''`),
		now, now, toolKey, agentID,
	)
	return entErrToBizErr(err, "TOOL")
}

func (r *toolRepo) GetToolInvocationParams(ctx context.Context, invocationID string) (biz.ToolInvocationParam, error) {
	client := r.data.RW().Read(ctx)
	if client == nil {
		return biz.ToolInvocationParam{}, apierror.Internal("TOOL", "ent client unavailable")
	}
	rows, err := client.QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(`
		SELECT id, invocation_id, tool_key, params_json, redaction_applied, created_at
		FROM tool_invocation_params
		WHERE invocation_id = ?
		LIMIT 1`), invocationID)
	if err != nil {
		return biz.ToolInvocationParam{}, entErrToBizErr(err, "TOOL")
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.ToolInvocationParam{}, apierror.NotFound("TOOL", "tool not found")
	}
	var p biz.ToolInvocationParam
	var redaction int
	if err := rows.Scan(&p.ID, &p.InvocationID, &p.ToolKey, &p.ParamsJSON, &redaction, &p.CreatedAt); err != nil {
		return biz.ToolInvocationParam{}, entErrToBizErr(err, "TOOL")
	}
	p.RedactionApplied = redaction != 0
	return p, nil
}
