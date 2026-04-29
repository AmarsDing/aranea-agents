package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"arenea/backend/internal/domain"
)

// Store is the Capability/Tools management-plane port backed by capability-owned tables.
// The current schema still uses legacy table names (tools, tool_invocations, tool_agent_overrides);
// the target schema is capability_* as documented in docs/0 main design.md.
type Store interface {
	InsertToolInvocation(domain.ToolInvocation) (domain.ToolInvocation, error)
	SearchTools(domain.ToolListQuery) (domain.ToolListResult, error)
	GetToolByID(string) (domain.Tool, error)
	CreateTool(domain.ToolUpsertInput) (domain.Tool, error)
	UpdateTool(string, domain.ToolUpsertInput) (domain.Tool, error)
	DeleteTool(string) error
	UpdateToolEnabled(string, bool) (domain.Tool, error)
	SearchToolInvocations(domain.ToolRunQuery) (domain.ToolRunResult, error)
}

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) InsertToolInvocation(t domain.ToolInvocation) (domain.ToolInvocation, error) {
	if strings.TrimSpace(t.ToolKey) == "" {
		return domain.ToolInvocation{}, errors.New("tool_key is required")
	}
	if t.ID == "" {
		t.ID = uniqueID("toolinv")
	}
	now := nowISO()
	if t.StartedAt == "" {
		t.StartedAt = now
	}
	if t.CreatedAt == "" {
		t.CreatedAt = now
	}
	if t.Status == "" {
		t.Status = "success"
	}
	if t.Source == "" {
		t.Source = "adk"
	}
	if _, err := s.db.Exec(
		`INSERT INTO tool_invocations(
			id, request_id, invocation_id, tool_id, tool_key, agent_id, agent_key,
			session_id, message_id, user_id, source, status, started_at, ended_at,
			duration_ms, input_preview, input_hash, output_preview, output_hash,
			error_code, error_message, redaction_applied, metadata_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.RequestID, t.InvocationID, t.ToolID, t.ToolKey, t.AgentID, t.AgentKey,
		t.SessionID, t.MessageID, t.UserID, t.Source, t.Status, t.StartedAt, t.EndedAt,
		t.DurationMS, t.InputPreview, t.InputHash, t.OutputPreview, t.OutputHash,
		t.ErrorCode, t.ErrorMessage, boolToInt(true), firstNonEmpty(t.MetadataJSON, "{}"), t.CreatedAt,
	); err != nil {
		return domain.ToolInvocation{}, err
	}
	return t, nil
}

func (s *SQLiteStore) SearchTools(query domain.ToolListQuery) (domain.ToolListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	where, args := toolWhereClause(query)
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM tools t WHERE `+where, args...).Scan(&total); err != nil {
		return domain.ToolListResult{}, err
	}
	listArgs := append([]any{time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)}, args...)
	listArgs = append(listArgs, query.Limit, query.Offset)
	rows, err := s.db.Query(toolSelectSQL()+` WHERE `+where+` ORDER BY t.category ASC, t.display_name ASC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return domain.ToolListResult{}, err
	}
	defer rows.Close()
	items, err := scanTools(rows)
	if err != nil {
		return domain.ToolListResult{}, err
	}
	summary, err := s.toolSummary(query)
	if err != nil {
		return domain.ToolListResult{}, err
	}
	return domain.ToolListResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset, Summary: summary}, nil
}

func (s *SQLiteStore) GetToolByID(id string) (domain.Tool, error) {
	rows, err := s.db.Query(toolSelectSQL()+` WHERE (t.id = ? OR t.tool_key = ?) AND t.deleted_at = '' LIMIT 1`, time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339), id, id)
	if err != nil {
		return domain.Tool{}, err
	}
	defer rows.Close()
	items, err := scanTools(rows)
	if err != nil {
		return domain.Tool{}, err
	}
	if len(items) == 0 {
		return domain.Tool{}, sql.ErrNoRows
	}
	return items[0], nil
}

func (s *SQLiteStore) CreateTool(input domain.ToolUpsertInput) (domain.Tool, error) {
	row := toolFromInput(input)
	if strings.TrimSpace(row.Key) == "" {
		return domain.Tool{}, errors.New("tool key is required")
	}
	applyBuiltinToolDefaults(&row)
	if row.ID == "" {
		row.ID = uniqueID("tool")
	}
	now := nowISO()
	_, err := s.db.Exec(
		`INSERT INTO tools(
			id, tool_key, display_name, description, category, source, risk_level, enabled, readonly, requires_confirmation,
			supports_streaming, supports_concurrency, parameters_schema_json, result_schema_json, config_schema_json, config_json,
			default_config_json, metadata_json, created_at, updated_at, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '')`,
		row.ID, row.Key, row.DisplayName, row.Description, row.Category, row.Source, row.RiskLevel, row.Enabled, row.Readonly, row.RequiresConfirmation,
		row.SupportsStreaming, row.SupportsConcurrency, row.ParametersSchemaJSON, row.ResultSchemaJSON, row.ConfigSchemaJSON, row.ConfigJSON,
		row.DefaultConfigJSON, row.MetadataJSON, now, now,
	)
	if err != nil {
		return domain.Tool{}, err
	}
	return s.GetToolByID(row.ID)
}

func (s *SQLiteStore) UpdateTool(id string, input domain.ToolUpsertInput) (domain.Tool, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Tool{}, errors.New("tool id is required")
	}
	existing, err := s.GetToolByID(id)
	if err != nil {
		return domain.Tool{}, err
	}
	row := toolFromInput(input)
	if strings.TrimSpace(row.Key) == "" {
		row.Key = existing.Key
	}
	if strings.TrimSpace(row.ID) == "" {
		row.ID = existing.ID
	}
	applyBuiltinToolDefaults(&row)
	result, err := s.db.Exec(
		`UPDATE tools SET
			tool_key = ?, display_name = ?, description = ?, category = ?, source = ?, risk_level = ?,
			enabled = ?, readonly = ?, requires_confirmation = ?, supports_streaming = ?, supports_concurrency = ?,
			parameters_schema_json = ?, result_schema_json = ?, config_schema_json = ?, config_json = ?,
			default_config_json = ?, metadata_json = ?, updated_at = ?, deleted_at = ''
		WHERE (id = ? OR tool_key = ?) AND deleted_at = ''`,
		row.Key, row.DisplayName, row.Description, row.Category, row.Source, row.RiskLevel,
		row.Enabled, row.Readonly, row.RequiresConfirmation, row.SupportsStreaming, row.SupportsConcurrency,
		row.ParametersSchemaJSON, row.ResultSchemaJSON, row.ConfigSchemaJSON, row.ConfigJSON,
		row.DefaultConfigJSON, row.MetadataJSON, nowISO(), id, id,
	)
	if err != nil {
		return domain.Tool{}, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return domain.Tool{}, sql.ErrNoRows
	}
	return s.GetToolByID(row.Key)
}

func (s *SQLiteStore) DeleteTool(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("tool id is required")
	}
	result, err := s.db.Exec(`UPDATE tools SET deleted_at = ?, updated_at = ? WHERE (id = ? OR tool_key = ?) AND deleted_at = ''`, nowISO(), nowISO(), id, id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *SQLiteStore) UpdateToolEnabled(id string, enabled bool) (domain.Tool, error) {
	if strings.TrimSpace(id) == "" {
		return domain.Tool{}, errors.New("tool id is required")
	}
	result, err := s.db.Exec(`UPDATE tools SET enabled = ?, updated_at = ? WHERE (id = ? OR tool_key = ?) AND deleted_at = ''`, enabled, nowISO(), id, id)
	if err != nil {
		return domain.Tool{}, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return domain.Tool{}, sql.ErrNoRows
	}
	return s.GetToolByID(id)
}

func (s *SQLiteStore) SearchToolInvocations(query domain.ToolRunQuery) (domain.ToolRunResult, error) {
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	where := []string{"1 = 1"}
	args := []any{}
	if query.ToolKey != "" {
		where = append(where, "ti.tool_key = ?")
		args = append(args, query.ToolKey)
	}
	if query.AgentID != "" {
		where = append(where, "ti.agent_id = ?")
		args = append(args, query.AgentID)
	}
	if query.SessionID != "" {
		where = append(where, "ti.session_id = ?")
		args = append(args, query.SessionID)
	}
	if query.Status != "" {
		where = append(where, "ti.status = ?")
		args = append(args, query.Status)
	}
	if query.From != "" {
		where = append(where, "ti.started_at >= ?")
		args = append(args, query.From)
	}
	if query.To != "" {
		where = append(where, "ti.started_at <= ?")
		args = append(args, query.To)
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM tool_invocations ti WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return domain.ToolRunResult{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.Limit, query.Offset)
	rows, err := s.db.Query(`
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
		return domain.ToolRunResult{}, err
	}
	defer rows.Close()
	items := []domain.ToolInvocation{}
	for rows.Next() {
		var item domain.ToolInvocation
		if err = rows.Scan(
			&item.ID, &item.RequestID, &item.InvocationID, &item.ToolID, &item.ToolKey, &item.ToolDisplayName,
			&item.AgentID, &item.AgentKey, &item.AgentDisplayName, &item.SessionID, &item.MessageID, &item.UserID,
			&item.Source, &item.Status, &item.StartedAt, &item.EndedAt, &item.DurationMS,
			&item.InputPreview, &item.InputHash, &item.OutputPreview, &item.OutputHash,
			&item.ErrorCode, &item.ErrorMessage, &item.RedactionApplied, &item.MetadataJSON, &item.CreatedAt,
		); err != nil {
			return domain.ToolRunResult{}, err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return domain.ToolRunResult{}, err
	}
	return domain.ToolRunResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

func (s *SQLiteStore) toolSummary(query domain.ToolListQuery) (domain.ToolSummary, error) {
	cutoff := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
	var summary domain.ToolSummary
	where, args := toolWhereClause(domain.ToolListQuery{
		Search:    query.Search,
		Category:  query.Category,
		Source:    query.Source,
		RiskLevel: query.RiskLevel,
		Enabled:   query.Enabled,
	})
	if err := s.db.QueryRow(`
		SELECT
		  COALESCE(COUNT(1), 0),
		  COALESCE(SUM(CASE WHEN enabled = 1 THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN enabled = 1 AND risk_level IN ('high', 'critical') THEN 1 ELSE 0 END), 0)
		FROM tools t WHERE `+where,
		args...,
	).Scan(&summary.TotalTools, &summary.EnabledTools, &summary.HighRiskEnabled); err != nil {
		return domain.ToolSummary{}, err
	}
	var success24h, failed24h, blocked24h int
	if err := s.db.QueryRow(`
		SELECT
		  COALESCE(COUNT(1), 0),
		  COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END), 0)
		FROM tool_invocations WHERE started_at >= ?`,
		cutoff,
	).Scan(&summary.Calls24h, &success24h, &failed24h, &blocked24h); err != nil {
		return domain.ToolSummary{}, err
	}
	if summary.Calls24h > 0 {
		summary.FailureRate24h = float64(failed24h+blocked24h) / float64(summary.Calls24h)
	}
	_ = success24h
	return summary, nil
}

func toolFromInput(input domain.ToolUpsertInput) domain.Tool {
	return domain.Tool{
		ID:                   input.ID,
		Key:                  strings.TrimSpace(input.Key),
		DisplayName:          strings.TrimSpace(input.DisplayName),
		Description:          strings.TrimSpace(input.Description),
		Category:             strings.TrimSpace(input.Category),
		Source:               strings.TrimSpace(input.Source),
		RiskLevel:            strings.TrimSpace(input.RiskLevel),
		Enabled:              input.Enabled,
		Readonly:             input.Readonly,
		RequiresConfirmation: input.RequiresConfirmation,
		SupportsStreaming:    input.SupportsStreaming,
		SupportsConcurrency:  input.SupportsConcurrency,
		ParametersSchemaJSON: input.ParametersSchemaJSON,
		ResultSchemaJSON:     input.ResultSchemaJSON,
		ConfigSchemaJSON:     input.ConfigSchemaJSON,
		ConfigJSON:           input.ConfigJSON,
		DefaultConfigJSON:    input.DefaultConfigJSON,
		MetadataJSON:         input.MetadataJSON,
	}
}

func applyBuiltinToolDefaults(row *domain.Tool) {
	if row.ID == "" {
		row.ID = "tool_" + strings.ReplaceAll(row.Key, "-", "_")
	}
	if row.Source == "" {
		row.Source = "builtin"
	}
	if row.RiskLevel == "" {
		row.RiskLevel = "low"
	}
	if row.ParametersSchemaJSON == "" {
		row.ParametersSchemaJSON = "{}"
	}
	if row.ResultSchemaJSON == "" {
		row.ResultSchemaJSON = "{}"
	}
	if row.ConfigSchemaJSON == "" {
		row.ConfigSchemaJSON = "{}"
	}
	if row.ConfigJSON == "" {
		row.ConfigJSON = "{}"
	}
	if row.DefaultConfigJSON == "" {
		row.DefaultConfigJSON = row.ConfigJSON
	}
	if row.MetadataJSON == "" {
		row.MetadataJSON = "{}"
	}
}

func toolWhereClause(query domain.ToolListQuery) (string, []any) {
	where := []string{"t.deleted_at = ''"}
	args := []any{}
	if search := strings.TrimSpace(query.Search); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = append(where, "(LOWER(t.tool_key) LIKE ? OR LOWER(t.display_name) LIKE ? OR LOWER(t.description) LIKE ?)")
		args = append(args, like, like, like)
	}
	if query.Category != "" {
		where = append(where, "t.category = ?")
		args = append(args, query.Category)
	}
	if query.Source != "" {
		where = append(where, "t.source = ?")
		args = append(args, query.Source)
	}
	if query.RiskLevel != "" {
		where = append(where, "t.risk_level = ?")
		args = append(args, query.RiskLevel)
	}
	if query.Enabled == "true" || query.Enabled == "false" {
		where = append(where, "t.enabled = ?")
		args = append(args, query.Enabled == "true")
	}
	return strings.Join(where, " AND "), args
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

func scanTools(rows *sql.Rows) ([]domain.Tool, error) {
	items := []domain.Tool{}
	for rows.Next() {
		var item domain.Tool
		if err := rows.Scan(
			&item.ID, &item.Key, &item.DisplayName, &item.Description, &item.Category, &item.Source, &item.RiskLevel,
			&item.Enabled, &item.Readonly, &item.RequiresConfirmation, &item.SupportsStreaming, &item.SupportsConcurrency,
			&item.ParametersSchemaJSON, &item.ResultSchemaJSON, &item.ConfigSchemaJSON, &item.ConfigJSON, &item.DefaultConfigJSON, &item.MetadataJSON,
			&item.InvokeCount, &item.InvokeCount24h, &item.SuccessCount, &item.FailureCount, &item.BlockedCount, &item.AgentOverrideCount,
			&item.AvgDurationMS, &item.LastInvokedAt, &item.LastStatus,
			&item.CreatedAt, &item.UpdatedAt, &item.DeletedAt,
		); err != nil {
			return nil, err
		}
		item.Permissions = domain.ToolPermissions{CanManage: true}
		items = append(items, item)
	}
	return items, rows.Err()
}

var idCounter atomic.Uint64

func uniqueID(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UTC().UnixNano(), idCounter.Add(1))
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
