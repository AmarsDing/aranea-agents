package sessionmemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

const (
	legacyScopeTRPC            = "trpc_memory"
	legacyEntityStatusMigrated = "migrated"
	legacyEntityStatusSkipped  = "skipped"
)

type legacyTRPCMemoryEntity struct {
	id, scopeID, userID, name, desc, meta, createdAt, updatedAt string
	importance                                                  float64
}

// BackfillLegacyTRPCMemoryEntities migrates legacy memory_entities (scope_type=trpc_memory,
// entity_type=memory_fact) into authoritative memory_facts rows, then soft-deletes the legacy entity.
// Invalid rows (empty statement/scope) are marked status=skipped. Idempotent via UpsertFactRow fingerprint.
func (st *Store) BackfillLegacyTRPCMemoryEntities(ctx context.Context) (migrated int, skipped int, err error) {
	if st == nil || st.client == nil {
		return 0, 0, errors.New("session memory store not wired")
	}
	rows, err := st.client.QueryContext(ctx, `
SELECT id, scope_id, user_id, name, description, importance, metadata_json, created_at, updated_at
FROM memory_entities
WHERE scope_type = ? AND entity_type = 'memory_fact' AND deleted_at = '' AND status NOT IN (?, ?)
ORDER BY created_at ASC`, legacyScopeTRPC, legacyEntityStatusMigrated, legacyEntityStatusSkipped)
	if err != nil {
		return 0, 0, err
	}
	pending, err := scanLegacyTRPCMemoryEntities(rows)
	if err != nil {
		return 0, 0, err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, row := range pending {
		stmt := strings.TrimSpace(row.desc)
		if stmt == "" {
			stmt = strings.TrimSpace(row.name)
		}
		if stmt == "" || strings.TrimSpace(row.scopeID) == "" {
			if err := st.markLegacyTRPCEntitySkipped(ctx, row.id, now); err != nil {
				return migrated, skipped, err
			}
			skipped++
			continue
		}
		tags := legacyEntityTopicsJSON(row.meta, st.lg)
		metaJSON, err := json.Marshal(map[string]string{
			"source":           "legacy_trpc_backfill",
			"legacy_entity_id": row.id,
		})
		if err != nil {
			return migrated, skipped, err
		}
		if _, err := st.UpsertFactRow(ctx, MemoryFactUpsert{
			ScopeType:       "agent",
			ScopeID:         strings.TrimSpace(row.scopeID),
			UserID:          strings.TrimSpace(row.userID),
			AgentID:         strings.TrimSpace(row.scopeID),
			Statement:       stmt,
			DetailsMarkdown: stmt,
			FactKind:        "fact",
			TagsJSON:        tags,
			Confidence:      0.85,
			Importance:      row.importance,
			SourceKind:      "legacy_trpc_backfill",
			Status:          "active",
			MetadataJSON:    string(metaJSON),
			CreatedAt:       strings.TrimSpace(row.createdAt),
			UpdatedAt:       strings.TrimSpace(row.updatedAt),
		}); err != nil {
			return migrated, skipped, err
		}
		if _, err := st.client.ExecContext(ctx,
			`UPDATE memory_entities SET status = ?, deleted_at = ?, updated_at = ? WHERE id = ?`,
			legacyEntityStatusMigrated, now, now, row.id); err != nil {
			return migrated, skipped, err
		}
		migrated++
	}
	return migrated, skipped, nil
}

func (st *Store) markLegacyTRPCEntitySkipped(ctx context.Context, id, now string) error {
	_, err := st.client.ExecContext(ctx,
		`UPDATE memory_entities SET status = ?, deleted_at = ?, updated_at = ? WHERE id = ?`,
		legacyEntityStatusSkipped, now, now, id)
	return err
}

func scanLegacyTRPCMemoryEntities(rows *sql.Rows) ([]legacyTRPCMemoryEntity, error) {
	defer rows.Close()
	var pending []legacyTRPCMemoryEntity
	for rows.Next() {
		var row legacyTRPCMemoryEntity
		if err := rows.Scan(&row.id, &row.scopeID, &row.userID, &row.name, &row.desc, &row.importance, &row.meta, &row.createdAt, &row.updatedAt); err != nil {
			return nil, err
		}
		pending = append(pending, row)
	}
	return pending, rows.Err()
}

// CountPendingLegacyTRPCMemoryEntities returns unmigrated trpc_memory fact entities.
func (st *Store) CountPendingLegacyTRPCMemoryEntities(ctx context.Context) (int, error) {
	if st == nil || st.client == nil {
		return 0, errors.New("session memory store not wired")
	}
	var n int
	err := queryOne(ctx, st.client, `
SELECT COUNT(1) FROM memory_entities
WHERE scope_type = ? AND entity_type = 'memory_fact' AND deleted_at = '' AND status NOT IN (?, ?)`,
		[]any{legacyScopeTRPC, legacyEntityStatusMigrated, legacyEntityStatusSkipped}, &n)
	return n, err
}

func legacyEntityTopicsJSON(meta string, lg loggateway.Logger) string {
	meta = strings.TrimSpace(meta)
	if meta == "" || meta == "{}" {
		return "[]"
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(meta), &m); err != nil {
		lg.Warn("session memory json unmarshal failed", loggateway.StepID("data.sessionmemory"), loggateway.Err(err))
		return "[]"
	}
	if t, ok := m["topics"]; ok {
		if b, err := json.Marshal(t); err == nil {
			return string(b)
		}
	}
	return "[]"
}
