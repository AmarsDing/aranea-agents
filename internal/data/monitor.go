package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"

	"github.com/google/uuid"
)

type monitorRepo struct {
	data           *Data
	firingColsOnce sync.Once // H-04: run schema migration only once per process
}

func NewMonitorRepo(d *Data) biz.MonitorRepo {
	return &monitorRepo{data: d}
}

func (r *monitorRepo) InsertAuditLog(ctx context.Context, entry biz.AuditLog) error {
	id := strings.TrimSpace(entry.ID)
	if id == "" {
		id = uuid.NewString()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(entry.CreatedAt) == "" {
		entry.CreatedAt = now
	}
	_, err := r.data.RawDB().ExecContext(ctx,
		`INSERT INTO audit_logs (id, action, resource, resource_id, request_id, detail, created_at, actor, ip, user_agent, severity, metadata_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, entry.Action, entry.Resource, entry.ResourceID, entry.RequestID, entry.Detail, entry.CreatedAt,
		entry.Actor, entry.IP, entry.UserAgent, entry.Severity, entry.MetadataJSON,
	)
	return err
}

func (r *monitorRepo) InsertMonitorEvent(ctx context.Context, ev biz.MonitorEventWrite) error {
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	status := strings.TrimSpace(ev.Status)
	if status == "" {
		status = "ok"
	}
	_, err := r.data.RawDB().ExecContext(ctx,
		`INSERT INTO monitor_events (id, event_key, name, description, status, metadata_json, created_at, updated_at, deleted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, '')`,
		id, ev.EventKey, ev.Name, ev.Description, status, ev.MetadataJSON, now, now,
	)
	if err != nil && isSQLiteUniqueConstraintError(err) {
		return nil
	}
	return err
}

func (r *monitorRepo) ListAuditLogs(ctx context.Context, query biz.AuditQuery) (biz.AuditListResult, error) {
	limit := int(query.Limit)
	if limit <= 0 {
		limit = 200
	}
	offset := int(query.Offset)
	if offset < 0 {
		offset = 0
	}

	where, args := auditWhere(query)
	countSQL := `SELECT COUNT(*) FROM audit_logs` + where
	listSQL := `SELECT id, action, resource, resource_id, request_id, detail, created_at,
		COALESCE(actor, '') AS actor, COALESCE(ip, '') AS ip,
		COALESCE(user_agent, '') AS user_agent, COALESCE(severity, '') AS severity,
		COALESCE(metadata_json, '') AS metadata_json
		FROM audit_logs` + where + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	var total int32
	if err := r.data.RawDB().QueryRowContext(ctx, countSQL, args[:len(args)-2]...).Scan(&total); err != nil {
		return biz.AuditListResult{}, err
	}

	rows, err := r.data.RawDB().QueryContext(ctx, listSQL, args...)
	if err != nil {
		return biz.AuditListResult{}, err
	}
	defer rows.Close()

	var out []biz.AuditLog
	for rows.Next() {
		var v biz.AuditLog
		if err = rows.Scan(&v.ID, &v.Action, &v.Resource, &v.ResourceID, &v.RequestID, &v.Detail, &v.CreatedAt,
			&v.Actor, &v.IP, &v.UserAgent, &v.Severity, &v.MetadataJSON); err != nil {
			return biz.AuditListResult{}, err
		}
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		return biz.AuditListResult{}, err
	}
	return biz.AuditListResult{Items: out, Total: total}, nil
}

func auditWhere(q biz.AuditQuery) (string, []any) {
	parts := []string{}
	args := []any{}
	if q.Action != "" {
		parts = append(parts, "action = ?")
		args = append(args, q.Action)
	}
	if q.Resource != "" {
		parts = append(parts, "resource = ?")
		args = append(args, q.Resource)
	}
	if q.Actor != "" {
		parts = append(parts, "actor = ?")
		args = append(args, q.Actor)
	}
	if q.Keyword != "" {
		kw := "%" + q.Keyword + "%"
		parts = append(parts, "(action LIKE ? OR resource LIKE ? OR resource_id LIKE ? OR detail LIKE ?)")
		args = append(args, kw, kw, kw, kw)
	}
	if len(parts) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

const sqlMonitorEventsCount = `SELECT COUNT(*) FROM monitor_events WHERE deleted_at = ''`
const sqlMonitorEventsList = `SELECT id, event_key, name, description, status, metadata_json, created_at, updated_at, deleted_at
	FROM monitor_events WHERE deleted_at = ''`
const sqlMonitorEventsGet = `SELECT id, event_key, name, description, status, metadata_json, created_at, updated_at, deleted_at
	FROM monitor_events WHERE id = ? AND deleted_at = ''`

const sqlMonitorTracesCount = `SELECT COUNT(*) FROM monitor_traces WHERE deleted_at = ''`
const sqlMonitorTracesList = `SELECT id, trace_key, name, description, status, metadata_json, created_at, updated_at, deleted_at
	FROM monitor_traces WHERE deleted_at = ''`
const sqlMonitorTracesGet = `SELECT id, trace_key, name, description, status, metadata_json, created_at, updated_at, deleted_at
	FROM monitor_traces WHERE id = ? AND deleted_at = ''`

func (r *monitorRepo) ListMonitorEvents(ctx context.Context, query biz.MonitorEventsQuery) (biz.MonitorListResult, error) {
	limit := int(query.Limit)
	if limit <= 0 {
		limit = 100
	}
	offset := int(query.Offset)
	if offset < 0 {
		offset = 0
	}

	where, args := monitorEventsWhere(query)
	countSQL := sqlMonitorEventsCount + where
	listSQL := sqlMonitorEventsList + where + " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	listArgs := append(args, limit, offset)

	var total int32
	if err := r.data.RawDB().QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return biz.MonitorListResult{}, err
	}

	rows, err := r.data.RawDB().QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return biz.MonitorListResult{}, err
	}
	defer rows.Close()

	out, err := scanMonitorRows("monitor-events", rows)
	if err != nil {
		return biz.MonitorListResult{}, err
	}
	return biz.MonitorListResult{Items: out, Total: total}, nil
}

func monitorEventsWhere(q biz.MonitorEventsQuery) (string, []any) {
	parts := []string{}
	args := []any{}
	if q.EventType != "" {
		parts = append(parts, "event_key LIKE ?")
		args = append(args, q.EventType+"%")
	}
	if q.AgentID != "" {
		parts = append(parts, "json_extract(metadata_json, '$.agent_id') = ?")
		args = append(args, q.AgentID)
	}
	if q.Status != "" {
		parts = append(parts, "status = ?")
		args = append(args, q.Status)
	}
	if len(parts) == 0 {
		return "", args
	}
	return " AND " + strings.Join(parts, " AND "), args
}

func (r *monitorRepo) GetMonitorEvent(ctx context.Context, id string) (biz.MonitorPlatformRow, error) {
	rows, err := r.data.RawDB().QueryContext(ctx, sqlMonitorEventsGet, id)
	if err != nil {
		return biz.MonitorPlatformRow{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.MonitorPlatformRow{}, sql.ErrNoRows
	}
	return scanMonitorPlatformRow("monitor-events", rows)
}

func (r *monitorRepo) ListMonitorTraces(ctx context.Context, query biz.MonitorTracesQuery) (biz.MonitorListResult, error) {
	limit := int(query.Limit)
	if limit <= 0 {
		limit = 100
	}
	offset := int(query.Offset)
	if offset < 0 {
		offset = 0
	}

	where, args := monitorTracesWhere(query)
	countSQL := sqlMonitorTracesCount + where
	listSQL := sqlMonitorTracesList + where + " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	listArgs := append(args, limit, offset)

	var total int32
	if err := r.data.RawDB().QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return biz.MonitorListResult{}, err
	}

	rows, err := r.data.RawDB().QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return biz.MonitorListResult{}, err
	}
	defer rows.Close()

	out, err := scanMonitorRows("monitor-traces", rows)
	if err != nil {
		return biz.MonitorListResult{}, err
	}
	return biz.MonitorListResult{Items: out, Total: total}, nil
}

func monitorTracesWhere(q biz.MonitorTracesQuery) (string, []any) {
	parts := []string{}
	args := []any{}
	if q.AgentID != "" {
		parts = append(parts, "json_extract(metadata_json, '$.agent_id') = ?")
		args = append(args, q.AgentID)
	}
	if q.Provider != "" {
		parts = append(parts, "json_extract(metadata_json, '$.provider') = ?")
		args = append(args, q.Provider)
	}
	if q.Model != "" {
		parts = append(parts, "json_extract(metadata_json, '$.model') = ?")
		args = append(args, q.Model)
	}
	if q.Status != "" {
		parts = append(parts, "status = ?")
		args = append(args, q.Status)
	}
	if len(parts) == 0 {
		return "", args
	}
	return " AND " + strings.Join(parts, " AND "), args
}

func (r *monitorRepo) ExistsRunnerCompletion(ctx context.Context, sessionID, invocationID string) (bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	invocationID = strings.TrimSpace(invocationID)
	if sessionID == "" || invocationID == "" {
		return false, nil
	}
	var n int
	err := r.data.RawDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM monitor_events WHERE deleted_at = '' AND event_key = 'runner.completion'
		 AND COALESCE(meta_session_id, json_extract(metadata_json, '$.session_id')) = ?
		 AND COALESCE(meta_invocation_id, json_extract(metadata_json, '$.invocation_id')) = ?`,
		sessionID, invocationID,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *monitorRepo) PatchRunnerCompletionMetadata(ctx context.Context, sessionID, runID, invocationID, patchJSON string) (bool, error) {
	sessionID = strings.TrimSpace(sessionID)
	runID = strings.TrimSpace(runID)
	invocationID = strings.TrimSpace(invocationID)
	if sessionID == "" || strings.TrimSpace(patchJSON) == "" {
		return false, nil
	}
	// When both invocationID and runID are available, try combined match first for precision.
	if invocationID != "" && runID != "" && runID != invocationID {
		if patched, err := r.patchRunnerCompletionByDualKey(ctx, sessionID, invocationID, runID, patchJSON); err != nil || patched {
			return patched, err
		}
	}
	if invocationID != "" {
		if patched, err := r.patchRunnerCompletionByKey(ctx, sessionID, "invocation_id", invocationID, patchJSON); err != nil || patched {
			return patched, err
		}
	}
	if runID != "" && runID != invocationID {
		return r.patchRunnerCompletionByKey(ctx, sessionID, "run_id", runID, patchJSON)
	}
	return false, nil
}

func (r *monitorRepo) patchRunnerCompletionByDualKey(ctx context.Context, sessionID, invocationID, runID, patchJSON string) (bool, error) {
	var id, existing string
	err := r.data.RawDB().QueryRowContext(ctx,
		`SELECT id, metadata_json FROM monitor_events WHERE deleted_at = '' AND event_key = 'runner.completion'
		 AND json_extract(metadata_json, '$.session_id') = ?
		 AND json_extract(metadata_json, '$.invocation_id') = ?
		 AND json_extract(metadata_json, '$.run_id') = ?
		 ORDER BY created_at DESC LIMIT 1`, sessionID, invocationID, runID,
	).Scan(&id, &existing)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	merged, err := mergeJSONMetadata(existing, patchJSON)
	if err != nil {
		return false, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := r.data.RawDB().ExecContext(ctx,
		`UPDATE monitor_events SET metadata_json = ?, updated_at = ? WHERE id = ?`,
		merged, now, id,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *monitorRepo) patchRunnerCompletionByKey(ctx context.Context, sessionID, jsonKey, jsonValue, patchJSON string) (bool, error) {
	jsonValue = strings.TrimSpace(jsonValue)
	if jsonValue == "" {
		return false, nil
	}
	var id, existing string
	query := fmt.Sprintf(
		`SELECT id, metadata_json FROM monitor_events WHERE deleted_at = '' AND event_key = 'runner.completion'
		 AND json_extract(metadata_json, '$.session_id') = ?
		 AND json_extract(metadata_json, '$.%s') = ?
		 ORDER BY created_at DESC LIMIT 1`, jsonKey)
	err := r.data.RawDB().QueryRowContext(ctx, query, sessionID, jsonValue).Scan(&id, &existing)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	merged, err := mergeJSONMetadata(existing, patchJSON)
	if err != nil {
		return false, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := r.data.RawDB().ExecContext(ctx,
		`UPDATE monitor_events SET metadata_json = ?, updated_at = ? WHERE id = ?`,
		merged, now, id,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func mergeJSONMetadata(existing, patch string) (string, error) {
	base := map[string]any{}
	if strings.TrimSpace(existing) != "" {
		if err := json.Unmarshal([]byte(existing), &base); err != nil {
			base = map[string]any{}
		}
	}
	delta := map[string]any{}
	if err := json.Unmarshal([]byte(patch), &delta); err != nil {
		return existing, err
	}
	for k, v := range delta {
		base[k] = v
	}
	raw, err := json.Marshal(base)
	if err != nil {
		return existing, err
	}
	return string(raw), nil
}

func (r *monitorRepo) GetMonitorTrace(ctx context.Context, id string) (biz.MonitorPlatformRow, error) {
	rows, err := r.data.RawDB().QueryContext(ctx, sqlMonitorTracesGet, id)
	if err != nil {
		return biz.MonitorPlatformRow{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.MonitorPlatformRow{}, sql.ErrNoRows
	}
	return scanMonitorPlatformRow("monitor-traces", rows)
}

func scanMonitorRows(resource string, rows *sql.Rows) ([]biz.MonitorPlatformRow, error) {
	var out []biz.MonitorPlatformRow
	for rows.Next() {
		item, err := scanMonitorPlatformRow(resource, rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func scanMonitorPlatformRow(resource string, row scanner) (biz.MonitorPlatformRow, error) {
	var (
		v                               biz.MonitorPlatformRow
		id, key, name                   string
		description, status             string
		metaJSON                        string
		createdAt, updatedAt, deletedAt string
	)
	v.Resource = resource
	err := row.Scan(&id, &key, &name, &description, &status, &metaJSON, &createdAt, &updatedAt, &deletedAt)
	if err != nil {
		return biz.MonitorPlatformRow{}, err
	}
	v.ID = id
	v.Key = key
	v.Name = name
	v.Description = description
	v.Status = status
	v.Enabled = true
	v.SortOrder = 0
	v.MetadataJSON = metaJSON
	v.ConfigJSON = "{}"
	v.CreatedAt = createdAt
	v.UpdatedAt = updatedAt
	v.DeletedAt = deletedAt
	return v, nil
}

func isSQLiteUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed") || strings.Contains(msg, "unique constraint")
}
