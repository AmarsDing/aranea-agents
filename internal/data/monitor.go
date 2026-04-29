package data

import (
	"context"
	"database/sql"

	"aranea-agents/internal/biz"
)

type monitorRepo struct {
	data *Data
}

func NewMonitorRepo(d *Data) biz.MonitorRepo {
	return &monitorRepo{data: d}
}

func (r *monitorRepo) ListAuditLogs(ctx context.Context, limit int) ([]biz.AuditLog, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.data.Ent().QueryContext(ctx,
		`SELECT id, action, resource, resource_id, request_id, detail, created_at
		 FROM audit_logs ORDER BY created_at DESC LIMIT ?`,
		limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []biz.AuditLog
	for rows.Next() {
		var v biz.AuditLog
		if err = rows.Scan(&v.ID, &v.Action, &v.Resource, &v.ResourceID, &v.RequestID, &v.Detail, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

const sqlMonitorEventsList = `SELECT id, event_key, name, description, status, metadata_json, created_at, updated_at, deleted_at
	FROM monitor_events WHERE deleted_at = '' ORDER BY created_at DESC`

const sqlMonitorEventsGet = `SELECT id, event_key, name, description, status, metadata_json, created_at, updated_at, deleted_at
	FROM monitor_events WHERE id = ? AND deleted_at = ''`

const sqlMonitorTracesList = `SELECT id, trace_key, name, description, status, metadata_json, created_at, updated_at, deleted_at
	FROM monitor_traces WHERE deleted_at = '' ORDER BY created_at DESC`

const sqlMonitorTracesGet = `SELECT id, trace_key, name, description, status, metadata_json, created_at, updated_at, deleted_at
	FROM monitor_traces WHERE id = ? AND deleted_at = ''`

func (r *monitorRepo) ListMonitorEvents(ctx context.Context) ([]biz.MonitorPlatformRow, error) {
	return r.scanMonitorRows(ctx, "monitor-events", sqlMonitorEventsList)
}

func (r *monitorRepo) GetMonitorEvent(ctx context.Context, id string) (biz.MonitorPlatformRow, error) {
	rows, err := r.data.Ent().QueryContext(ctx, sqlMonitorEventsGet, id)
	if err != nil {
		return biz.MonitorPlatformRow{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.MonitorPlatformRow{}, sql.ErrNoRows
	}
	return scanMonitorPlatformRow("monitor-events", rows)
}

func (r *monitorRepo) ListMonitorTraces(ctx context.Context) ([]biz.MonitorPlatformRow, error) {
	return r.scanMonitorRows(ctx, "monitor-traces", sqlMonitorTracesList)
}

func (r *monitorRepo) GetMonitorTrace(ctx context.Context, id string) (biz.MonitorPlatformRow, error) {
	rows, err := r.data.Ent().QueryContext(ctx, sqlMonitorTracesGet, id)
	if err != nil {
		return biz.MonitorPlatformRow{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.MonitorPlatformRow{}, sql.ErrNoRows
	}
	return scanMonitorPlatformRow("monitor-traces", rows)
}

func (r *monitorRepo) scanMonitorRows(ctx context.Context, resource string, query string) ([]biz.MonitorPlatformRow, error) {
	rows, err := r.data.Ent().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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
