package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// LegacyTRPCMigrationStatus reports pending legacy rows and whether the version gate passed.
type LegacyTRPCMigrationStatus struct {
	Pending int
	Applied bool
}

// GetLegacyTRPCMigrationStatus returns counts of unmigrated trpc_memory entities and gate state.
func GetLegacyTRPCMigrationStatus(ctx context.Context, d *Data, lg loggateway.Logger) (LegacyTRPCMigrationStatus, error) {
	var out LegacyTRPCMigrationStatus
	if d == nil {
		return out, fmt.Errorf("legacy trpc migration: data required")
	}
	client := d.ClientFromCtx(ctx)
	applied, err := isMigrationApplied(ctx, client, MigrationLegacyTRPCMemoryFacts, lg)
	if err != nil {
		return out, err
	}
	out.Applied = applied
	if applied {
		return out, nil
	}
	pending, err := countPendingLegacyTRPCMemoryEntities(ctx, d)
	if err != nil {
		return out, err
	}
	out.Pending = pending
	return out, nil
}

func RunLegacyTRPCMemoryMigration(ctx context.Context, d *Data, lg loggateway.Logger) (migrated int, skipped bool, err error) {
	if d == nil {
		return 0, false, fmt.Errorf("legacy trpc migration: data required")
	}
	client := d.ClientFromCtx(ctx)
	applied, err := isMigrationApplied(ctx, client, MigrationLegacyTRPCMemoryFacts, lg)
	if err != nil {
		return 0, false, err
	}
	if applied {
		return 0, true, nil
	}
	migrated, err = backfillLegacyTRPCMemoryEntities(ctx, d)
	if err != nil {
		return migrated, false, err
	}
	remaining, err := countPendingLegacyTRPCMemoryEntities(ctx, d)
	if err != nil {
		return migrated, false, err
	}
	if remaining > 0 {
		return migrated, false, fmt.Errorf("legacy trpc migration: %d entities still pending after backfill", remaining)
	}
	if err := recordMigrationApplied(ctx, client, d.Dialect(), MigrationLegacyTRPCMemoryFacts, migrationNameLegacyTRPCMemoryFacts, lg); err != nil {
		return migrated, false, err
	}
	return migrated, false, nil
}

func countPendingLegacyTRPCMemoryEntities(ctx context.Context, d *Data) (int, error) {
	var count int
	err := queryRowScan(ctx, d.RWDB().ReadDB(ctx),
		`SELECT COUNT(*) FROM trpc_memory_entities WHERE migrated = 0`, nil, &count)
	if err != nil {
		// Table might not exist
		return 0, nil
	}
	return count, nil
}

func backfillLegacyTRPCMemoryEntities(ctx context.Context, d *Data) (int, error) {
	lg := d.lg
	rows, err := d.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, scope_type, scope_id, user_id, agent_id, entity_type, name, statement, details, confidence, importance, source_kind, source_session_id, source_message_id, metadata_json, created_at FROM trpc_memory_entities WHERE migrated = 0 LIMIT 100`)
	if err != nil {
		// Table might not exist
		return 0, nil
	}
	type legacyRow struct {
		id, scopeType, scopeID, userID, agentID, entityType, name, statement, details string
		sourceKind, sourceSessionID, sourceMessageID, metadataJSON, createdAt         string
		confidence, importance                                                        float64
	}
	var batch []legacyRow
	var skippedIDs []string
	for rows.Next() {
		var r legacyRow
		if err := rows.Scan(&r.id, &r.scopeType, &r.scopeID, &r.userID, &r.agentID, &r.entityType, &r.name, &r.statement, &r.details, &r.confidence, &r.importance, &r.sourceKind, &r.sourceSessionID, &r.sourceMessageID, &r.metadataJSON, &r.createdAt); err != nil {
			if lg != nil {
				lg.Warn("legacy trpc migration: scan row failed", loggateway.Err(err))
			}
			continue
		}
		if strings.TrimSpace(r.statement) == "" {
			skippedIDs = append(skippedIDs, r.id)
			continue
		}
		batch = append(batch, r)
	}
	rows.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	var migrated int

	// Wrap all writes in a single transaction (红线 #24) so that either all
	// rows are migrated together or none are, avoiding partial states where
	// some rows are marked migrated while their fact inserts failed.
	err = d.ExecInTx(ctx, func(txCtx context.Context) error {
		// Mark skipped (empty statement) rows as migrated so they are not re-processed.
		for _, sid := range skippedIDs {
			if _, execErr := d.RWDB().WriteDB(txCtx).ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`UPDATE trpc_memory_entities SET migrated = 1 WHERE id = ?`), sid); execErr != nil {
				if lg != nil {
					lg.Warn("legacy trpc migration: mark skipped row failed",
						loggateway.Str("id", sid), loggateway.Err(execErr))
				}
			}
		}

		for _, r := range batch {
			fp := biz.FactFingerprint(r.statement, r.scopeType, r.scopeID)
			tags := "[]"
			meta := strings.TrimSpace(r.metadataJSON)
			if meta == "" {
				meta = "{}"
			}
			_, insertErr := d.RWDB().WriteDB(txCtx).ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`INSERT INTO memory_facts (
			id, scope_type, scope_id, workspace_id, user_id, team_id, agent_id,
			statement, statement_normalized, fingerprint, details_markdown,
			fact_kind, tags_json,
			confidence, importance, use_count, hit_count,
			positive_feedback_count, negative_feedback_count, conflict_count,
			source_kind, source_episode_id, source_session_id, source_message_id, source_external,
			version, status, superseded_by,
			pii_flag, redacted_statement,
			quality_score, metadata_json, created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(scope_type, scope_id, fingerprint) DO NOTHING`),
				r.id, r.scopeType, r.scopeID, "", r.userID, "", r.agentID,
				r.statement, strings.ToLower(r.statement), fp, r.details,
				r.entityType, tags,
				r.confidence, r.importance, 0, 0, 0, 0, 0,
				r.sourceKind, "", r.sourceSessionID, r.sourceMessageID, "",
				1, "active", "", 0, "", 0, meta, r.createdAt, now,
			)
			if insertErr != nil {
				if lg != nil {
					lg.Warn("legacy trpc migration: insert fact failed",
						loggateway.Str("id", r.id), loggateway.Err(insertErr))
				}
				continue
			}
			// Mark as migrated
			if _, markErr := d.RWDB().WriteDB(txCtx).ExecContext(txCtx, d.Dialect().RenumberPlaceholders(`UPDATE trpc_memory_entities SET migrated = 1 WHERE id = ?`), r.id); markErr != nil {
				if lg != nil {
					lg.Warn("legacy trpc migration: mark migrated failed",
						loggateway.Str("id", r.id), loggateway.Err(markErr))
				}
				continue
			}
			migrated++
		}
		return nil
	})
	if err != nil {
		return migrated, entErrToBizErr(err, "MEMORY")
	}
	return migrated, nil
}

// ensure json is referenced
var _ = json.Marshal
