package data

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// ddlSessionRevisionDataMigration backfills session_revision from message counts.
// The ALTER TABLE is handled by the SQL file; this Func only does the data migration.
// Postgres uses a proper bigint type for session_revision, so the backfill
// only needs an IS NULL guard.
func ddlSessionRevisionDataMigration(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	if entClient == nil {
		return nil
	}
	// Skip on fresh databases where the messages table was never created.
	// Migration 20260902 drops the messages subsystem (superseded by activities);
	// on a fresh DB the messages table never exists, so there is nothing to backfill.
	var tableExists bool
	// to_regclass returns NULL when the table doesn't exist; scan into a
	// nullable string and check for empty/NULL.
	var regclass *string
	err := rawDB.QueryRowContext(ctx, "SELECT to_regclass('public.messages')::text").Scan(&regclass)
	if err != nil || regclass == nil || *regclass == "" {
		lg.Info("ddlSessionRevisionDataMigration: messages table not found, skipping backfill")
		return nil
	}
	tableExists = true
	if !tableExists {
		lg.Info("ddlSessionRevisionDataMigration: messages table not found, skipping backfill")
		return nil
	}
	whereClause := "WHERE session_revision IS NULL"
	_, err = entClient.ExecContext(ctx, fmt.Sprintf(`
UPDATE sessions SET session_revision = (
  SELECT COUNT(*) FROM messages WHERE messages.session_id = sessions.id AND role = 'user'
) %s`, whereClause))
	return err
}

func ddlEcosystemPresetDataMigration(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, _ loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}

	tx, err := rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE agents SET kind = 'system_builtin' WHERE kind = 'system'`); err != nil {
		return fmt.Errorf("migrate agent kind system->system_builtin: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE agents SET kind = 'ecosystem_preset' WHERE kind = 'industry_template'`); err != nil {
		return fmt.Errorf("migrate agent kind industry_template->ecosystem_preset: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE teams SET kind = 'ecosystem_preset' WHERE source = 'imported'`); err != nil {
		return fmt.Errorf("migrate team kind imported->ecosystem_preset: %w", err)
	}
	return tx.Commit()
}

// ddlAgentSourceDataMigration populates the new agents.source column from existing kind values.
// Mapping: kind=user -> source=user, kind=system_builtin -> source=system, kind=ecosystem_preset/marketplace/certified -> source=imported.
// Also fixes Team source and corrects over-broad Team kind migration from 20260718.
func ddlAgentSourceDataMigration(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}

	tx, err := rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `UPDATE agents SET source = 'system' WHERE kind = 'system_builtin' AND source = 'user'`); err != nil {
		return fmt.Errorf("migrate agent source system_builtin->system: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agents SET source = 'imported' WHERE kind IN ('ecosystem_preset', 'marketplace', 'certified') AND source = 'user'`); err != nil {
		return fmt.Errorf("migrate agent source ecosystem->imported: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE teams SET source = 'system' WHERE kind = 'system_builtin' AND source = 'user'`); err != nil {
		return fmt.Errorf("migrate team source system_builtin->system: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `UPDATE teams SET source = 'imported' WHERE kind = 'ecosystem_preset' AND source = 'user'`); err != nil {
		return fmt.Errorf("migrate team source ecosystem_preset->imported: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent source migration: %w", err)
	}

	jsonExtract := d.JSONExtract
	jsonEach := d.JSONEach
	tx2, err := rawDB.BeginTx(ctx, nil)
	if err != nil {
		lg.Warn("ddl migration: begin tx for team kind fix failed", loggateway.Err(err))
		return nil
	}
	defer tx2.Rollback()
	if _, err := tx2.ExecContext(ctx, fmt.Sprintf(`
		UPDATE teams SET kind = 'user', source = 'user'
		WHERE kind = 'ecosystem_preset'
		  AND deleted_at = ''
		  AND id NOT IN (
		    SELECT DISTINCT t2.id
		    FROM teams t2, %s AS tm
		    INNER JOIN agents a ON a.id = %s AND a.kind = 'ecosystem_preset' AND a.deleted_at = ''
		    WHERE t2.kind = 'ecosystem_preset' AND t2.deleted_at = ''
		  )
	`, jsonEach("t2.definition_json"), jsonExtract("tm.value", "agent_id"))); err != nil {

		lg.Warn("ddl migration: fix over-broad team kind migration failed", loggateway.Err(err))
		return nil
	}
	if err := tx2.Commit(); err != nil {
		lg.Warn("ddl migration: commit team kind fix failed", loggateway.Err(err))
	}
	return nil
}

func ddlUnifiedEvolutionSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureUnifiedEvolutionSchema(ctx, entClient)
}

// ddlUnifiedEvolutionConvergence backfills the three legacy evolution-suggestion
// stores into unified_evolution_suggestions and drops the legacy tables (A6).
// Legacy-only fields are preserved in the metadata JSON column so the service
// layer can reconstruct the legacy proto views:
//
//	L1 skill_proposals:              pattern_hash / pattern_desc / approved_at / rejected_by
//	L2 skill_evolution_suggestions:  source_report_ids / draft_version_id / parent_version_id /
//	                                 evolution_reason / pre_verify_result / rejected_by /
//	                                 rejection_reason / resolved_at
//	L3 evolution_suggestions:        legacy_type / title / diff_preview / pre_apply_snapshot
//
// Design notes:
//   - Fresh databases never contain L2/L3 legacy rows because their Ent
//     schemas are removed in the same change; L1 skill_proposals may exist
//     transiently on fresh databases (created empty by migration 20260706,
//     which runs earlier in version order) and is dropped below after a
//     zero-row backfill.
//   - Backfill is row-by-row with a primary-key pre-check instead of
//     INSERT...SELECT so no dialect-specific JSON/ON CONFLICT SQL is needed.
//     Migrations run single-threaded at startup (advisory lock on Postgres),
//     so the check-then-insert race cannot occur in practice.
//   - L1 status 'registered' is kept as-is (frontend matches the literal
//     string); the L1 view layer interprets it.
//   - The pending-dedup unique index is rebuilt with a dialect-aware JSON path
//     expression preserving legacy dedup semantics: L1 by metadata.pattern_hash,
//     L3 by metadata.legacy_type, health/curator by (target, action).
//
// Idempotent: existence pre-check per row, DROP TABLE IF EXISTS.
func ddlUnifiedEvolutionConvergence(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	if err := ddlUnifiedEvolutionRebuildPendingIndex(ctx, rawDB, d); err != nil {
		return err
	}
	if err := ddlBackfillSkillEvolutionSuggestions(ctx, rawDB, d, lg); err != nil {
		return err
	}
	if err := ddlBackfillSkillProposals(ctx, rawDB, d, lg); err != nil {
		return err
	}
	if err := ddlBackfillEvolutionSuggestions(ctx, rawDB, d, lg); err != nil {
		return err
	}
	return nil
}

// ddlUnifiedEvolutionRebuildPendingIndex replaces the (target_type, target_id)
// pending unique index with one whose dedup key preserves legacy semantics.
func ddlUnifiedEvolutionRebuildPendingIndex(ctx context.Context, rawDB *sql.DB, d Dialect) error {
	if _, err := rawDB.ExecContext(ctx, `DROP INDEX IF EXISTS idx_ues_pending_target`); err != nil {
		return fmt.Errorf("unified evolution convergence: drop old pending index: %w", err)
	}
	dedupExpr := `COALESCE(metadata::jsonb->>'pattern_hash', metadata::jsonb->>'legacy_type', '')`
	stmt := `CREATE UNIQUE INDEX IF NOT EXISTS idx_ues_pending_target
  ON unified_evolution_suggestions(target_type, target_id, action_type, ` + dedupExpr + `)
  WHERE status = 'pending'`
	if _, err := rawDB.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("unified evolution convergence: create pending index: %w", err)
	}
	return nil
}

// ddlUnifiedBackfillRow is one legacy suggestion row normalized for insertion
// into unified_evolution_suggestions.
type ddlUnifiedBackfillRow struct {
	id              string
	targetType      string
	targetID        string
	actionType      string
	triggerSource   string
	triggerReason   string
	status          string
	draftBody       string
	draftName       string
	lifecycleStatus string
	sandboxPassed   bool
	sandboxResult   *string
	metadata        map[string]any
	createdAt       string
	approvedBy      string
	appliedAt       *string
}

// ddlInsertUnifiedBackfill inserts one backfilled row, skipping rows whose id
// already exists (idempotent re-runs and bridge-dual-written rows).
func ddlInsertUnifiedBackfill(ctx context.Context, rawDB *sql.DB, d Dialect, row ddlUnifiedBackfillRow) (bool, error) {
	var exists int
	checkQ := d.RenumberPlaceholders(`SELECT 1 FROM unified_evolution_suggestions WHERE id = ?`)
	if err := rawDB.QueryRowContext(ctx, checkQ, row.id).Scan(&exists); err == nil {
		return false, nil
	} else if err != sql.ErrNoRows {
		return false, fmt.Errorf("check existing %s: %w", row.id, err)
	}
	var metadataStr *string
	if len(row.metadata) > 0 {
		b, err := json.Marshal(row.metadata)
		if err != nil {
			return false, fmt.Errorf("marshal metadata %s: %w", row.id, err)
		}
		s := string(b)
		metadataStr = &s
	}
	sandboxPassed := 0
	if row.sandboxPassed {
		sandboxPassed = 1
	}
	insertQ := d.RenumberPlaceholders(`INSERT INTO unified_evolution_suggestions
  (id, target_type, target_id, action_type, trigger_source, trigger_reason,
   status, priority, draft_body, draft_name, merge_target_id,
   lifecycle_status, sandbox_passed, sandbox_result, metadata,
   created_at, approved_by, applied_at)
  VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?, '', ?, ?, ?, ?, ?, ?, ?)`)
	_, err := rawDB.ExecContext(ctx, insertQ,
		row.id, row.targetType, row.targetID, row.actionType, row.triggerSource, row.triggerReason,
		row.status, row.draftBody, row.draftName, row.lifecycleStatus,
		sandboxPassed, row.sandboxResult, metadataStr,
		row.createdAt, row.approvedBy, row.appliedAt)
	if err != nil {
		return false, fmt.Errorf("insert %s: %w", row.id, err)
	}
	return true, nil
}

// ddlDropLegacyTable drops a legacy table after successful backfill.
func ddlDropLegacyTable(ctx context.Context, rawDB *sql.DB, table string, migrated int, lg loggateway.Logger) error {
	if _, err := rawDB.ExecContext(ctx, `DROP TABLE IF EXISTS `+table); err != nil {
		return fmt.Errorf("drop %s: %w", table, err)
	}
	lg.Info("unified evolution convergence: legacy table backfilled and dropped",
		loggateway.StepID("ddl.unified_evolution_convergence"),
		loggateway.Str("table", table),
		loggateway.Int("migrated", migrated))
	return nil
}

// ddlBackfillSkillEvolutionSuggestions backfills L2 (skill-scoped curator
// suggestions) into the unified store, then drops the legacy table.
func ddlBackfillSkillEvolutionSuggestions(ctx context.Context, rawDB *sql.DB, d Dialect, lg loggateway.Logger) error {
	const table = "skill_evolution_suggestions"
	exists, err := d.TableExists(ctx, rawDB, table)
	if err != nil {
		return fmt.Errorf("unified evolution convergence: check %s: %w", table, err)
	}
	if !exists {
		return nil
	}
	rows, err := rawDB.QueryContext(ctx, `SELECT id, skill_id, type, status, source_report_ids, trigger_reason,
		draft_skill_body, draft_version_id, sandbox_passed, sandbox_result, pre_verify_result,
		approved_by, rejected_by, rejection_reason, created_at, resolved_at, parent_version_id,
		evolution_reason, lifecycle_status FROM `+table)
	if err != nil {
		return fmt.Errorf("unified evolution convergence: read %s: %w", table, err)
	}
	defer rows.Close()
	migrated := 0
	for rows.Next() {
		var r ddlUnifiedBackfillRow
		var sourceReportIDs, sandboxResult, preVerifyResult *string
		var draftVersionID, rejectedBy, rejectionReason, resolvedAt, parentVersionID, evolutionReason, lifecycleStatus string
		if err := rows.Scan(&r.id, &r.targetID, &r.actionType, &r.status, &sourceReportIDs, &r.triggerReason,
			&r.draftBody, &draftVersionID, &r.sandboxPassed, &sandboxResult, &preVerifyResult,
			&r.approvedBy, &rejectedBy, &rejectionReason, &r.createdAt, &resolvedAt, &parentVersionID,
			&evolutionReason, &lifecycleStatus); err != nil {
			return fmt.Errorf("unified evolution convergence: scan %s: %w", table, err)
		}
		r.targetType = string(biz.EvolutionTargetSkill)
		r.triggerSource = "health"
		r.lifecycleStatus = lifecycleStatus
		if r.lifecycleStatus == "" {
			r.lifecycleStatus = "draft"
		}
		r.sandboxResult = sandboxResult
		r.metadata = map[string]any{
			biz.EvoMetaSourceReportIDs: json.RawMessage(sourceReportIDsOrEmpty(sourceReportIDs)),
			biz.EvoMetaDraftVersionID:  draftVersionID,
			biz.EvoMetaParentVersionID: parentVersionID,
			biz.EvoMetaEvolutionReason: evolutionReason,
			biz.EvoMetaRejectedBy:      rejectedBy,
			biz.EvoMetaRejectionReason: rejectionReason,
			biz.EvoMetaResolvedAt:      resolvedAt,
		}
		if preVerifyResult != nil && *preVerifyResult != "" {
			r.metadata[biz.EvoMetaPreVerifyResult] = json.RawMessage(*preVerifyResult)
		}
		inserted, err := ddlInsertUnifiedBackfill(ctx, rawDB, d, r)
		if err != nil {
			return fmt.Errorf("unified evolution convergence: backfill %s: %w", table, err)
		}
		if inserted {
			migrated++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("unified evolution convergence: iterate %s: %w", table, err)
	}
	return ddlDropLegacyTable(ctx, rawDB, table, migrated, lg)
}

// sourceReportIDsOrEmpty normalizes a legacy JSON-array TEXT column to a valid
// JSON array literal for embedding into backfilled metadata.
func sourceReportIDsOrEmpty(s *string) string {
	if s == nil || *s == "" {
		return "[]"
	}
	return *s
}

// ddlBackfillSkillProposals backfills L1 (agent-scoped skill-creation
// proposals) into the unified store, then drops the legacy table.
func ddlBackfillSkillProposals(ctx context.Context, rawDB *sql.DB, d Dialect, lg loggateway.Logger) error {
	const table = "skill_proposals"
	exists, err := d.TableExists(ctx, rawDB, table)
	if err != nil {
		return fmt.Errorf("unified evolution convergence: check %s: %w", table, err)
	}
	if !exists {
		return nil
	}
	rows, err := rawDB.QueryContext(ctx, `SELECT id, agent_id, pattern_hash, pattern_desc, skill_name, skill_md,
		status, approved_by, rejected_by, created_at, approved_at FROM `+table)
	if err != nil {
		return fmt.Errorf("unified evolution convergence: read %s: %w", table, err)
	}
	defer rows.Close()
	migrated := 0
	for rows.Next() {
		var r ddlUnifiedBackfillRow
		var patternHash, patternDesc, rejectedBy string
		var approvedAt *string
		if err := rows.Scan(&r.id, &r.targetID, &patternHash, &patternDesc, &r.draftName, &r.draftBody,
			&r.status, &r.approvedBy, &rejectedBy, &r.createdAt, &approvedAt); err != nil {
			return fmt.Errorf("unified evolution convergence: scan %s: %w", table, err)
		}
		r.targetType = string(biz.EvolutionTargetAgent)
		r.actionType = string(biz.EvolutionActionCreate)
		r.triggerSource = "pattern"
		r.triggerReason = patternDesc
		r.lifecycleStatus = "draft"
		approvedAtStr := ""
		if approvedAt != nil {
			approvedAtStr = *approvedAt
		}
		r.metadata = map[string]any{
			biz.EvoMetaPatternHash: patternHash,
			biz.EvoMetaPatternDesc: patternDesc,
			biz.EvoMetaApprovedAt:  approvedAtStr,
			biz.EvoMetaRejectedBy:  rejectedBy,
		}
		inserted, err := ddlInsertUnifiedBackfill(ctx, rawDB, d, r)
		if err != nil {
			return fmt.Errorf("unified evolution convergence: backfill %s: %w", table, err)
		}
		if inserted {
			migrated++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("unified evolution convergence: iterate %s: %w", table, err)
	}
	return ddlDropLegacyTable(ctx, rawDB, table, migrated, lg)
}

// ddlBackfillEvolutionSuggestions backfills L3 (agent-scoped persona/prompt/
// skill suggestions) into the unified store, then drops the legacy table.
func ddlBackfillEvolutionSuggestions(ctx context.Context, rawDB *sql.DB, d Dialect, lg loggateway.Logger) error {
	const table = "evolution_suggestions"
	exists, err := d.TableExists(ctx, rawDB, table)
	if err != nil {
		return fmt.Errorf("unified evolution convergence: check %s: %w", table, err)
	}
	if !exists {
		return nil
	}
	rows, err := rawDB.QueryContext(ctx, `SELECT id, agent_id, type, title, content, status,
		diff_preview, pre_apply_snapshot, created_at, applied_at FROM `+table)
	if err != nil {
		return fmt.Errorf("unified evolution convergence: read %s: %w", table, err)
	}
	defer rows.Close()
	migrated := 0
	for rows.Next() {
		var r ddlUnifiedBackfillRow
		var legacyType, title, diffPreview, preApplySnapshot, appliedAt string
		if err := rows.Scan(&r.id, &r.targetID, &legacyType, &title, &r.draftBody, &r.status,
			&diffPreview, &preApplySnapshot, &r.createdAt, &appliedAt); err != nil {
			return fmt.Errorf("unified evolution convergence: scan %s: %w", table, err)
		}
		r.targetType = string(biz.EvolutionTargetAgent)
		r.actionType = string(biz.EvolutionActionEvolve)
		r.triggerSource = "agent_config"
		r.triggerReason = title
		r.lifecycleStatus = "draft"
		if appliedAt != "" {
			at := appliedAt
			r.appliedAt = &at
		}
		r.metadata = map[string]any{
			biz.EvoMetaLegacyType:       legacyType,
			biz.EvoMetaTitle:            title,
			biz.EvoMetaDiffPreview:      diffPreview,
			biz.EvoMetaPreApplySnapshot: preApplySnapshot,
		}
		inserted, err := ddlInsertUnifiedBackfill(ctx, rawDB, d, r)
		if err != nil {
			return fmt.Errorf("unified evolution convergence: backfill %s: %w", table, err)
		}
		if inserted {
			migrated++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("unified evolution convergence: iterate %s: %w", table, err)
	}
	return ddlDropLegacyTable(ctx, rawDB, table, migrated, lg)
}

func ddlEvolutionSuggestionPreApplySnapshot(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, _ loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}

	exists, err := d.TableExists(ctx, rawDB, "evolution_suggestions")
	if err != nil {
		return fmt.Errorf("check evolution_suggestions: %w", err)
	}
	if !exists {
		return nil
	}
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE evolution_suggestions ADD COLUMN pre_apply_snapshot TEXT NOT NULL DEFAULT ''`); err != nil && !d.AlreadyExistsErr(err) {
		return fmt.Errorf("add evolution_suggestions.pre_apply_snapshot: %w", err)
	}
	return nil
}

// ddlHealRecordMetadataColumn adds the metadata column to heal_records table
// for persisting extra context (root cause metadata, event metadata, etc.)
// as a JSON-encoded TEXT field. Idempotent: "duplicate column" errors are
// treated as success per DB-N6.
