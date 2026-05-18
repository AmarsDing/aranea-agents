package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

type monitorRepo struct {
	data *Data
}

func NewMonitorRepo(d *Data) biz.MonitorRepo {
	return &monitorRepo{data: d}
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
	listSQL := sqlMonitorEventsList + where + fmt.Sprintf(" ORDER BY created_at DESC LIMIT %d OFFSET %d", limit, offset)

	var total int32
	if err := r.data.RawDB().QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return biz.MonitorListResult{}, err
	}

	rows, err := r.data.RawDB().QueryContext(ctx, listSQL, args...)
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
		parts = append(parts, "metadata_json LIKE ?")
		args = append(args, "%"+q.AgentID+"%")
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
	listSQL := sqlMonitorTracesList + where + fmt.Sprintf(" ORDER BY created_at DESC LIMIT %d OFFSET %d", limit, offset)

	var total int32
	if err := r.data.RawDB().QueryRowContext(ctx, countSQL, args...).Scan(&total); err != nil {
		return biz.MonitorListResult{}, err
	}

	rows, err := r.data.RawDB().QueryContext(ctx, listSQL, args...)
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
		parts = append(parts, "metadata_json LIKE ?")
		args = append(args, "%"+q.AgentID+"%")
	}
	if q.Provider != "" {
		parts = append(parts, "metadata_json LIKE ?")
		args = append(args, "%"+q.Provider+"%")
	}
	if q.Model != "" {
		parts = append(parts, "metadata_json LIKE ?")
		args = append(args, "%"+q.Model+"%")
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
