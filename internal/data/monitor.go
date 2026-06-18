package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	bizmonitor "aranea-agents/internal/biz/monitor"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

type monitorRepo struct {
	data           *Data
	firingColsOnce sync.Once // H-04: run schema migration only once per process
}

var _ bizmonitor.AuditRepo = (*monitorRepo)(nil)
var _ bizmonitor.EventRepo = (*monitorRepo)(nil)
var _ bizmonitor.TraceRepo = (*monitorRepo)(nil)
var _ bizmonitor.AlertRepo = (*monitorRepo)(nil)
var _ bizmonitor.RunnerCompletionRepo = (*monitorRepo)(nil)

func NewMonitorAuditRepo(d *Data) biz.MonitorAuditRepo {
	return &monitorRepo{data: d}
}

func NewMonitorEventRepo(d *Data) biz.MonitorEventRepo {
	return &monitorRepo{data: d}
}

func NewMonitorTraceRepo(d *Data) biz.MonitorTraceRepo {
	return &monitorRepo{data: d}
}

func NewMonitorAlertRepo(d *Data) biz.MonitorAlertRepo {
	return &monitorRepo{data: d}
}

func NewMonitorRunnerCompletionRepo(d *Data) biz.MonitorRunnerCompletionRepo {
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
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`INSERT INTO audit_logs (id, action, resource, resource_id, request_id, detail, created_at, actor, ip, user_agent, severity, metadata_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, entry.Action, entry.Resource, entry.ResourceID, entry.RequestID, entry.Detail, entry.CreatedAt,
		entry.Actor, entry.IP, entry.UserAgent, entry.Severity, entry.MetadataJSON,
	)
	return entErrToBizErr(err, "MONITOR")
}

func (r *monitorRepo) InsertMonitorEvent(ctx context.Context, ev biz.MonitorEventWrite) error {
	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339)
	status := strings.TrimSpace(ev.Status)
	if status == "" {
		status = "ok"
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`INSERT INTO monitor_events (id, event_key, name, description, status, metadata_json, created_at, updated_at, deleted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, '')`,
		id, ev.EventKey, ev.Name, ev.Description, status, ev.MetadataJSON, now, now,
	)
	if err != nil && r.data.Dialect().UniqueConstraintErr(err) {
		r.data.lg.Warn("InsertMonitorEvent: duplicate event key, skipping",
			loggateway.StepID("monitor.event_duplicate"),
			loggateway.Str("event_key", ev.EventKey))
		return nil
	}
	return entErrToBizErr(err, "MONITOR")
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
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), countSQL, args[:len(args)-2], &total); err != nil {
		return biz.AuditListResult{}, entErrToBizErr(err, "MONITOR")
	}

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, listSQL, args...)
	if err != nil {
		return biz.AuditListResult{}, entErrToBizErr(err, "MONITOR")
	}
	defer rows.Close()

	var out []biz.AuditLog
	for rows.Next() {
		var v biz.AuditLog
		if err = rows.Scan(&v.ID, &v.Action, &v.Resource, &v.ResourceID, &v.RequestID, &v.Detail, &v.CreatedAt,
			&v.Actor, &v.IP, &v.UserAgent, &v.Severity, &v.MetadataJSON); err != nil {
			return biz.AuditListResult{}, entErrToBizErr(err, "MONITOR")
		}
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		return biz.AuditListResult{}, entErrToBizErr(err, "MONITOR")
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
const sqlMonitorTracesList = `SELECT id, trace_key, name, description, status, agent_id, provider, model, metadata_json, created_at, updated_at, deleted_at
	FROM monitor_traces WHERE deleted_at = ''`
const sqlMonitorTracesGet = `SELECT id, trace_key, name, description, status, agent_id, provider, model, metadata_json, created_at, updated_at, deleted_at
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

	d := r.data.Dialect()
	where, args := monitorEventsWhere(query, d)
	countSQL := d.RenumberPlaceholders(sqlMonitorEventsCount + where)
	listSQL := d.RenumberPlaceholders(sqlMonitorEventsList + where + " ORDER BY created_at DESC LIMIT ? OFFSET ?")
	listArgs := append(args, limit, offset)

	var total int32
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), countSQL, args, &total); err != nil {
		return biz.MonitorListResult{}, entErrToBizErr(err, "MONITOR")
	}

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return biz.MonitorListResult{}, entErrToBizErr(err, "MONITOR")
	}
	defer rows.Close()

	out, err := scanMonitorRows("monitor-events", rows)
	if err != nil {
		return biz.MonitorListResult{}, entErrToBizErr(err, "MONITOR")
	}
	return biz.MonitorListResult{Items: out, Total: total}, nil
}

func monitorEventsWhere(q biz.MonitorEventsQuery, d Dialect) (string, []any) {
	parts := []string{}
	args := []any{}
	if q.EventType != "" {
		parts = append(parts, "event_key LIKE ?")
		args = append(args, q.EventType+"%")
	}
	if q.AgentID != "" {
		parts = append(parts, d.JSONExtract("metadata_json", "agent_id")+" = ?")
		args = append(args, q.AgentID)
	}
	if q.SessionID != "" {
		parts = append(parts, "COALESCE(meta_session_id, "+d.JSONExtract("metadata_json", "session_id")+") = ?")
		args = append(args, q.SessionID)
	}
	if q.TraceID != "" {
		parts = append(parts, "COALESCE(meta_trace_id, "+d.JSONExtract("metadata_json", "trace_id")+") = ?")
		args = append(args, q.TraceID)
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
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, sqlMonitorEventsGet, id)
	if err != nil {
		return biz.MonitorPlatformRow{}, entErrToBizErr(err, "MONITOR")
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.MonitorPlatformRow{}, apierror.NotFound(apierror.DomainData, "not found")
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

	d := r.data.Dialect()
	where, args := monitorTracesWhere(query, d)
	countSQL := d.RenumberPlaceholders(sqlMonitorTracesCount + where)
	listSQL := d.RenumberPlaceholders(sqlMonitorTracesList + where + " ORDER BY created_at DESC LIMIT ? OFFSET ?")
	listArgs := append(args, limit, offset)

	var total int32
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), countSQL, args, &total); err != nil {
		return biz.MonitorListResult{}, entErrToBizErr(err, "MONITOR")
	}

	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, listSQL, listArgs...)
	if err != nil {
		return biz.MonitorListResult{}, entErrToBizErr(err, "MONITOR")
	}
	defer rows.Close()

	out, err := scanTraceRows(rows)
	if err != nil {
		return biz.MonitorListResult{}, entErrToBizErr(err, "MONITOR")
	}
	return biz.MonitorListResult{Items: out, Total: total}, nil
}

func monitorTracesWhere(q biz.MonitorTracesQuery, d Dialect) (string, []any) {
	parts := []string{}
	args := []any{}
	// TODO(debt): After backfill completes for all rows, simplify to direct column
	// comparison (e.g. "agent_id = ?") for index utilization. The COALESCE fallback
	// to json_extract is a transition pattern for rows created before the column existed.
	if q.AgentID != "" {
		parts = append(parts, "COALESCE(NULLIF(agent_id, ''), "+d.JSONExtract("metadata_json", "agent_id")+") = ?")
		args = append(args, q.AgentID)
	}
	if q.Provider != "" {
		parts = append(parts, "COALESCE(NULLIF(provider, ''), "+d.JSONExtract("metadata_json", "provider")+") = ?")
		args = append(args, q.Provider)
	}
	if q.Model != "" {
		parts = append(parts, "COALESCE(NULLIF(model, ''), "+d.JSONExtract("metadata_json", "model")+") = ?")
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
	d := r.data.Dialect()
	query := d.RenumberPlaceholders(`SELECT COUNT(*) FROM monitor_events WHERE deleted_at = '' AND event_key = 'runner.completion'
		 AND COALESCE(meta_session_id, ` + d.JSONExtract("metadata_json", "session_id") + `) = ?
		 AND COALESCE(meta_invocation_id, ` + d.JSONExtract("metadata_json", "invocation_id") + `) = ?`)
	var n int
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		query, []any{sessionID, invocationID}, &n)
	if err != nil {
		return false, entErrToBizErr(err, "MONITOR")
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
	d := r.data.Dialect()
	var id, existing string
	query := d.RenumberPlaceholders(`SELECT id, metadata_json FROM monitor_events WHERE deleted_at = '' AND event_key = 'runner.completion'
		 AND ` + d.JSONExtract("metadata_json", "session_id") + ` = ?
		 AND ` + d.JSONExtract("metadata_json", "invocation_id") + ` = ?
		 AND ` + d.JSONExtract("metadata_json", "run_id") + ` = ?
		 ORDER BY created_at DESC LIMIT 1`)
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), query, []any{sessionID, invocationID, runID},
		&id, &existing)
	if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeNotFound {
		return false, nil
	}
	if err != nil {
		return false, entErrToBizErr(err, "MONITOR")
	}
	merged, err := mergeJSONMetadata(r.data.lg, existing, patchJSON)
	if err != nil {
		return false, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE monitor_events SET metadata_json = ?, updated_at = ? WHERE id = ?`,
		merged, now, id,
	)
	if err != nil {
		return false, entErrToBizErr(err, "MONITOR")
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *monitorRepo) patchRunnerCompletionByKey(ctx context.Context, sessionID, jsonKey, jsonValue, patchJSON string) (bool, error) {
	jsonValue = strings.TrimSpace(jsonValue)
	if jsonValue == "" {
		return false, nil
	}
	safeKey, ok := safeJSONKey(jsonKey)
	if !ok {
		return false, nil
	}
	d := r.data.Dialect()
	var id, existing string
	query := d.RenumberPlaceholders(`SELECT id, metadata_json FROM monitor_events WHERE deleted_at = '' AND event_key = 'runner.completion'
		 AND ` + d.JSONExtract("metadata_json", "session_id") + ` = ?
		 AND ` + d.JSONExtract("metadata_json", safeKey) + ` = ?
		 ORDER BY created_at DESC LIMIT 1`)
	err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), query, []any{sessionID, jsonValue}, &id, &existing)
	if ae, ok := apierror.From(err); ok && ae.Code == apierror.CodeNotFound {
		return false, nil
	}
	if err != nil {
		return false, entErrToBizErr(err, "MONITOR")
	}
	merged, err := mergeJSONMetadata(r.data.lg, existing, patchJSON)
	if err != nil {
		return false, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE monitor_events SET metadata_json = ?, updated_at = ? WHERE id = ?`,
		merged, now, id,
	)
	if err != nil {
		return false, entErrToBizErr(err, "MONITOR")
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func mergeJSONMetadata(lg loggateway.Logger, existing, patch string) (string, error) {
	base := map[string]any{}
	if strings.TrimSpace(existing) != "" {
		if err := json.Unmarshal([]byte(existing), &base); err != nil {
			lg.Warn("unmarshal existing metadata failed, resetting base", loggateway.StepID("data.monitor"), loggateway.Err(err))
			base = map[string]any{}
		}
	}
	delta := map[string]any{}
	if err := json.Unmarshal([]byte(patch), &delta); err != nil {
		lg.Warn("unmarshal patch metadata failed", loggateway.StepID("data.monitor"), loggateway.Err(err))
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
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, sqlMonitorTracesGet, id)
	if err != nil {
		return biz.MonitorPlatformRow{}, entErrToBizErr(err, "MONITOR")
	}
	defer rows.Close()
	if !rows.Next() {
		return biz.MonitorPlatformRow{}, apierror.NotFound(apierror.DomainData, "not found")
	}
	return scanTracePlatformRow(rows)
}

func scanMonitorRows(resource string, rows *sql.Rows) ([]biz.MonitorPlatformRow, error) {
	var out []biz.MonitorPlatformRow
	for rows.Next() {
		item, err := scanMonitorPlatformRow(resource, rows)
		if err != nil {
			return nil, entErrToBizErr(err, "MONITOR")
		}
		out = append(out, item)
	}
	return out, entErrToBizErr(rows.Err(), "MONITOR")
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
		return biz.MonitorPlatformRow{}, entErrToBizErr(err, "MONITOR")
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

// scanTraceRows scans a batch of trace rows using the trace-specific column set.
func scanTraceRows(rows *sql.Rows) ([]biz.MonitorPlatformRow, error) {
	var out []biz.MonitorPlatformRow
	for rows.Next() {
		item, err := scanTracePlatformRow(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "MONITOR")
		}
		out = append(out, item)
	}
	return out, entErrToBizErr(rows.Err(), "MONITOR")
}

// scanTracePlatformRow scans a single monitor_traces row with extended columns
// (agent_id, provider, model) that are not present in the events table.
func scanTracePlatformRow(row scanner) (biz.MonitorPlatformRow, error) {
	var (
		v                               biz.MonitorPlatformRow
		id, key, name                   string
		description, status             string
		agentID, provider, model        string
		metaJSON                        string
		createdAt, updatedAt, deletedAt string
	)
	err := row.Scan(&id, &key, &name, &description, &status, &agentID, &provider, &model, &metaJSON, &createdAt, &updatedAt, &deletedAt)
	if err != nil {
		return biz.MonitorPlatformRow{}, entErrToBizErr(err, "MONITOR")
	}
	v.Resource = "monitor-traces"
	v.ID = id
	v.Key = key
	v.Name = name
	v.Description = description
	v.Status = status
	v.Enabled = true
	v.SortOrder = 0
	v.AgentID = agentID
	v.Provider = provider
	v.Model = model
	v.MetadataJSON = metaJSON
	v.ConfigJSON = "{}"
	v.CreatedAt = createdAt
	v.UpdatedAt = updatedAt
	v.DeletedAt = deletedAt
	return v, nil
}

// allowedJSONKeys is a whitelist for json_extract path construction in SQL queries.
var allowedJSONKeys = map[string]string{
	"invocation_id": "invocation_id",
	"run_id":        "run_id",
}

func safeJSONKey(key string) (string, bool) {
	if k, ok := allowedJSONKeys[key]; ok {
		return k, true
	}
	return "", false
}
