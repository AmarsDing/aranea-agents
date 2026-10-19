package data

import (
	"aranea-agents/pkg/apierror"
	"context"
	"encoding/json"
	"time"
)

// isCJKRune reports whether r is a CJK ideograph or syllable.
func isCJKRune(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) ||
		(r >= 0x3040 && r <= 0x309F) ||
		(r >= 0x30A0 && r <= 0x30FF) ||
		(r >= 0xAC00 && r <= 0xD7AF)
}

// ListIdleL1Tasks returns tasks pending archive in two branches (P1-2):
// idle active tasks (the worker ends + archives them) and ended-but-unarchived
// tasks (a previous archive attempt failed — the worker retries the archive tx
// only). The retry branch keeps failures inside the scan set instead of
// silently dropping them; its ended_at cutoff avoids racing the synchronous
// end+archive path in MemoryAdminUsecase.EndL1Task.
func (r *l1WorkingMemoryRepo) ListIdleL1Tasks(ctx context.Context, idleCutoffRFC3339, retryCutoffRFC3339 string) ([][]byte, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT id, session_id, agent_id, task_title, status, updated_at, ended_at FROM memory_l1_tasks WHERE archived_at = '' AND ((status = 'active' AND updated_at < ?) OR (status != 'active' AND ended_at != '' AND ended_at < ?))`),
		idleCutoffRFC3339, retryCutoffRFC3339,
	)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L1")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var id, sessID, agentID, title, status, updatedAt, endedAt string
		if err := rows.Scan(&id, &sessID, &agentID, &title, &status, &updatedAt, &endedAt); err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L1")
		}
		m := map[string]any{
			"id": id, "session_id": sessID, "agent_id": agentID,
			"task_title": title, "status": status, "updated_at": updatedAt, "ended_at": endedAt,
		}
		b, mErr := json.Marshal(m)
		if mErr != nil {
			return nil, entErrToBizErr(mErr, "MEMORY_L1")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L1")
}

// DeleteExpiredL1Fields removes all L1 fields whose expires_at is set and in
// the past. These fields are already filtered from normal reads; this method
// performs periodic DB cleanup so expired rows don't accumulate indefinitely.
// Returns the number of deleted rows.
func (r *l1WorkingMemoryRepo) DeleteExpiredL1Fields(ctx context.Context) (int, error) {
	nowUTC := time.Now().UTC().Format(time.RFC3339)
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`DELETE FROM memory_l1_fields WHERE expires_at != '' AND expires_at < ?`),
		nowUTC,
	)
	if err != nil {
		return 0, entErrToBizErr(err, "MEMORY_L1")
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *l1WorkingMemoryRepo) GetL1SchemaRow(ctx context.Context, schemaID string) ([]byte, error) {
	if schemaID == "" {
		return nil, apierror.BadRequest("MEMORY", "schema id is required")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`SELECT id, scope_type, scope_id, schema_key, schema_version, schema_json, description, enabled, metadata_json, created_at FROM memory_l1_schemas WHERE id = ?`),
		schemaID,
	)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L1")
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, apierror.NotFound(apierror.DomainMemory, "not found")
	}
	var (
		id, scopeType, scopeID, schemaKey string
		schemaVersion                     int
		schemaJSON, description           string
		enabled                           int
		metadataJSON, createdAt           string
	)
	if err := rows.Scan(&id, &scopeType, &scopeID, &schemaKey, &schemaVersion, &schemaJSON, &description, &enabled, &metadataJSON, &createdAt); err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L1")
	}
	m := map[string]any{
		"id": id, "scope_type": scopeType, "scope_id": scopeID,
		"schema_key": schemaKey, "schema_version": schemaVersion,
		"schema_json": schemaJSON, "description": description,
		"enabled": enabled, "metadata_json": metadataJSON, "created_at": createdAt,
	}
	return json.Marshal(m)
}
