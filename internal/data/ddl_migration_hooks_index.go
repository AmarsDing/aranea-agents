package data

import (
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ddlIntentPassDefaultOnMigration corrects the historical false default of
// intent_pass_enabled for non-A2A agents. A2A proxy agents (config_json contains
// "a2a_proxy") keep false (set explicitly in biz.validateAgentUpdate when
// IsA2AProxyAgent(agent) is true).
// Idempotent: only updates rows where intent_pass_enabled = FALSE.
func ddlIntentPassDefaultOnMigration(ctx context.Context, rawDB *sql.DB, _ *ent.Client, _ Dialect, lg loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	tx, err := rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin intent_pass_default_on tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		UPDATE agent_runtime_settings
		SET intent_pass_enabled = TRUE
		WHERE intent_pass_enabled = FALSE
		  AND agent_id IN (
		    SELECT id FROM agents
		    WHERE config_json NOT LIKE '%a2a_proxy%'
		      AND deleted_at = ''
		  )
	`)
	if err != nil {
		return fmt.Errorf("migrate intent_pass_enabled default on: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		lg.Info("intent_pass_default_on migration applied",
			loggateway.StepID("data.migration.intent_pass_default_on"),
			loggateway.Int("rows_updated", int(n)))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit intent_pass_default_on migration: %w", err)
	}
	return nil
}

// ddlMemoryRecallDefaultsFix corrects two historical memory-recall defaults
// (2026-08-08 review, P0-3/P0-4):
//   - l0_inject_l4 defaulted false → L4 entity graph was never injected into any
//     prompt. Flip rows that have the L4 pipeline on (l4_enabled=true) but the
//     injection toggle off. Downstream guards (0.3 confidence gate + maxPaths
//     cap) bound the risk. Rows with l4_enabled=false keep their explicit off.
//   - l3_recall_min_score defaulted 0.55 → typical relevant hits score
//     Total≈0.4-0.5 under the weighted mix and were false-killed. Lower 0.55
//     rows to 0.35. Explicit 0.00 rows (user disabled filtering) stay untouched.
//
// Idempotent: both UPDATEs are value-guarded no-ops on re-run. TRUE/FALSE
// literals for the bool column (see ddlIntentPassDefaultOnMigration).
func ddlMemoryRecallDefaultsFix(ctx context.Context, rawDB *sql.DB, _ *ent.Client, _ Dialect, lg loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	tx, err := rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin memory_recall_defaults_fix tx: %w", err)
	}
	defer tx.Rollback()
	resL4, err := tx.ExecContext(ctx, `
		UPDATE agent_runtime_settings
		SET l0_inject_l4 = TRUE
		WHERE l4_enabled = TRUE AND l0_inject_l4 = FALSE
	`)
	if err != nil {
		return fmt.Errorf("migrate l0_inject_l4 default on: %w", err)
	}
	resMin, err := tx.ExecContext(ctx, `
		UPDATE agent_runtime_settings
		SET l3_recall_min_score = 0.35
		WHERE l3_recall_min_score = 0.55
	`)
	if err != nil {
		return fmt.Errorf("migrate l3_recall_min_score 0.55→0.35: %w", err)
	}
	nL4, _ := resL4.RowsAffected()
	nMin, _ := resMin.RowsAffected()
	if nL4 > 0 || nMin > 0 {
		lg.Info("memory_recall_defaults_fix migration applied",
			loggateway.StepID("data.migration.memory_recall_defaults_fix"),
			loggateway.Int("l0_inject_l4_rows", int(nL4)),
			loggateway.Int("l3_min_score_rows", int(nMin)))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit memory_recall_defaults_fix migration: %w", err)
	}
	return nil
}

// ddlMemoryFactsFTSIndex creates the GIN FTS index on memory_facts (P2-3).
// Postgres-only: to_tsvector/GIN do not exist on SQLite, so CLI tools and
// SQLite tests skip it (FTS candidate search degrades to nil there).
func ddlMemoryFactsFTSIndex(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if !d.IsPostgres() || rawDB == nil {
		lg.Info("memory_facts_fts_index skipped (non-postgres or nil db)",
			loggateway.StepID("data.ddl_migration.memory_facts_fts"))
		return nil
	}
	return executeSQLFileWithDialect(ctx, rawDB, "sql/migrations/20261128_memory_facts_fts_index.sql", d, lg)
}

// ddlMemoryFactsTrgmIndex creates the pg_trgm GIN index on memory_facts.statement
// (P1-2). Postgres-only: gin_trgm_ops does not exist on SQLite.
func ddlMemoryFactsTrgmIndex(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if !d.IsPostgres() || rawDB == nil {
		lg.Info("memory_facts_trgm_index skipped (non-postgres or nil db)",
			loggateway.StepID("data.ddl_migration.memory_facts_trgm"))
		return nil
	}
	return executeSQLFileWithDialect(ctx, rawDB, "sql/migrations/20261242_memory_facts_trgm_index.sql", d, lg)
}

// ddlDecisionRecordsGINIndexes creates the GIN expression indexes on
// decision_records source_ref / related_entities (M80 Phase 1, NFR-80-04).
// Postgres-only: GIN/jsonb cast do not exist on SQLite; columns are TEXT-stored
// JSON per dual-dialect convention, so the index targets (col::jsonb).
func ddlDecisionRecordsGINIndexes(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if !d.IsPostgres() || rawDB == nil {
		lg.Info("decision_records_gin_indexes skipped (non-postgres or nil db)",
			loggateway.StepID("data.ddl_migration.decision_records_gin"))
		return nil
	}
	return executeSQLFileWithDialect(ctx, rawDB, "sql/migrations/20261252_decision_records_gin.sql", d, lg)
}

// ddlDecisionRecordsRunIDIndex creates the btree expression index serving
// source_run_id equality filters (M80 1.10 perf tuning, NFR-80-04).
// Postgres-only: the indexed expression casts TEXT->jsonb; SQLite skips.
func ddlDecisionRecordsRunIDIndex(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if !d.IsPostgres() || rawDB == nil {
		lg.Info("decision_records_runid_idx skipped (non-postgres or nil db)",
			loggateway.StepID("data.ddl_migration.decision_records_runid_idx"))
		return nil
	}
	return executeSQLFileWithDialect(ctx, rawDB, "sql/migrations/20261254_decision_records_runid_idx.sql", d, lg)
}

// ddlDecisionRecordsSessionIDIndex creates the btree expression index serving
// source_session_id equality filters and SessionGateStats aggregation (T5,
// 2026-08-27 chat-side gate aggregation surface). Postgres-only: the indexed
// expression casts TEXT->jsonb; SQLite skips.
func ddlDecisionRecordsSessionIDIndex(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if !d.IsPostgres() || rawDB == nil {
		lg.Info("decision_records_sessionid_idx skipped (non-postgres or nil db)",
			loggateway.StepID("data.ddl_migration.decision_records_sessionid_idx"))
		return nil
	}
	return executeSQLFileWithDialect(ctx, rawDB, "sql/migrations/20261268_decision_records_sessionid_idx.sql", d, lg)
}

// ddlTenantRLSPhase1 enables Postgres RLS on tenant-owned tables (C-25).
// Skipped on non-Postgres dialects (SQLite tests / CLI).
func ddlTenantRLSPhase1(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	if !d.IsPostgres() || rawDB == nil {
		lg.Info("tenant_rls_phase1 skipped (non-postgres or nil db)",
			loggateway.StepID("data.ddl_migration.tenant_rls_phase1"))
		return nil
	}
	return executeSQLFileWithDialect(ctx, rawDB, "sql/migrations/20261011_tenant_rls_phase1.sql", d, lg)
}

// ddlAgentsPositionPartialUnique narrows agents(position_key, agent_variant) to a
// partial unique index (BUG-01, 2026-08-17): rows with empty position_key and
// soft-deleted rows no longer occupy a position slot; the design intent is only
// "one active agent per (position, variant)". Postgres-only: SQLite 侧由 Ent
// schema（entsql.IndexWhere 同谓词）建库时直接落成部分索引。
// Idempotent: DROP IF EXISTS + CREATE UNIQUE INDEX IF NOT EXISTS.
func ddlAgentsPositionPartialUnique(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if !d.IsPostgres() || rawDB == nil {
		lg.Info("agents_position_variant_partial_unique skipped (non-postgres or nil db)",
			loggateway.StepID("data.ddl_migration.agents_position_partial_unique"))
		return nil
	}
	if _, err := rawDB.ExecContext(ctx, `DROP INDEX IF EXISTS agent_position_key_agent_variant`); err != nil {
		return fmt.Errorf("drop agents position/variant index: %w", err)
	}
	if _, err := rawDB.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS agent_position_key_agent_variant ON agents (position_key, agent_variant) WHERE position_key <> '' AND deleted_at = ''`); err != nil {
		return fmt.Errorf("create agents position/variant partial unique index: %w", err)
	}
	return nil
}

// ddlDropRetiredLegacyTables drops retired tables (activities / event_store)
// that linger in存量库: fresh installs已不含这些表，但旧二进制经 Ent auto-migrate
// 可将其复活（BUG-02 F4，2026-08-17）。现行代码无 Ent schema、级联/生产代码均无引用，
// 数据为退役遗留。Idempotent: DROP TABLE IF EXISTS.
// sessions_v2 刻意保留：spirit 会话根实体（session_v2_repo.go 活跃），非死表。
func ddlDropRetiredLegacyTables(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if !d.IsPostgres() || rawDB == nil {
		lg.Info("drop_retired_legacy_tables skipped (non-postgres or nil db)",
			loggateway.StepID("data.ddl_migration.drop_retired_legacy_tables"))
		return nil
	}
	for _, table := range []string{"activities", "event_store"} {
		if _, err := rawDB.ExecContext(ctx, `DROP TABLE IF EXISTS `+table+` CASCADE`); err != nil {
			return fmt.Errorf("drop retired table %s: %w", table, err)
		}
	}
	return nil
}

// ddlUnifiedEvolutionWorkspace adds workspace_id to unified_evolution_suggestions
// (进化建议模块 P0-1a, IDOR 修复): column + index + backfill from host tables
// (skill/agents) + RLS policy. Skipped on non-Postgres dialects.
func ddlUnifiedEvolutionWorkspace(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	if !d.IsPostgres() || rawDB == nil {
		lg.Info("unified_evolution_workspace skipped (non-postgres or nil db)",
			loggateway.StepID("data.ddl_migration.unified_evolution_workspace"))
		return nil
	}
	return executeSQLFileWithDialect(ctx, rawDB, "sql/migrations/20261212_unified_evolution_workspace.sql", d, lg)
}

// ddlUsageEventsTraceIDIndex adds an expression index on
// model_token_usage_events(metadata.trace_id). AggregateUsageByTrace filters
// by this expression once per trace completion and per backfill row; without
// an index every lookup seq-scans the growing events table. The index uses
// the same Dialect.JSONExtract expression as the query so the planner can
// always match it. Idempotent: CREATE INDEX IF NOT EXISTS.
func ddlUsageEventsTraceIDIndex(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, _ loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	traceExpr := d.JSONExtract("metadata_json", "trace_id")

	stmt := `CREATE INDEX IF NOT EXISTS idx_model_token_usage_events_trace_id ON model_token_usage_events ((` + traceExpr + `))`
	if _, err := rawDB.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("usage events trace_id index: %w", err)
	}
	return nil
}

// ddlToolGrantsExpiresAt adds expires_at to tool_grants (BUG-MON-B, 2026-08-17):
// persisted "always allow" grants previously had no TTL — residual high-risk
// grants (shell_exec/hostexec/playwright 等) stayed effective forever.
// Adds the column and backfills existing rows to created_at + 72h (most
// expire immediately; read paths filter + lazily clean them). New grants
// write now+72h via ToolUsecase.GrantTool. created_at is RFC3339 UTC text
// (nowRFC3339 惯例); the regex guard skips malformed rows so the cast can
// never abort startup. Postgres-only: SQLite 侧由 Ent schema 建列。
// Idempotent: ADD COLUMN IF NOT EXISTS + value-guarded UPDATE.
func ddlToolGrantsExpiresAt(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if !d.IsPostgres() || rawDB == nil {
		lg.Info("tool_grants_expires_at skipped (non-postgres or nil db)",
			loggateway.StepID("data.ddl_migration.tool_grants_expires_at"))
		return nil
	}
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE tool_grants ADD COLUMN IF NOT EXISTS expires_at VARCHAR(64) NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add tool_grants.expires_at: %w", err)
	}
	res, err := rawDB.ExecContext(ctx, `
		UPDATE tool_grants
		SET expires_at = to_char((created_at::timestamptz + interval '72 hours') AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		WHERE expires_at = ''
		  AND created_at ~ '^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}'
	`)
	if err != nil {
		return fmt.Errorf("backfill tool_grants.expires_at: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		lg.Info("tool_grants expires_at backfilled",
			loggateway.StepID("data.ddl_migration.tool_grants_expires_at"),
			loggateway.Int("rows_updated", int(n)))
	}
	return nil
}

// ddlToolInvocationsWorkspace adds workspace_id to tool_invocations
// (A2, tools 端到端审计 2026-10-19): column + (workspace_id, started_at) index
// + backfill from agents by agent_id. 宿主缺失的存量行保持 ''（仅系统调用方
// 在 KPI 聚合可见）。Postgres-only：SQLite 侧由 Ent schema 建列（同
// ddlToolGrantsExpiresAt 惯例）。Idempotent: ADD COLUMN IF NOT EXISTS +
// CREATE INDEX IF NOT EXISTS + 空值守卫 UPDATE。
func ddlToolInvocationsWorkspace(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if !d.IsPostgres() || rawDB == nil {
		lg.Info("tool_invocations_workspace skipped (non-postgres or nil db)",
			loggateway.StepID("data.ddl_migration.tool_invocations_workspace"))
		return nil
	}
	return executeSQLFileWithDialect(ctx, rawDB, "sql/migrations/20261279_tool_invocations_workspace.sql", d, lg)
}

// ddlToolsRetryParallelDefaultOn flips historical false defaults for
// tools_retry_enabled / tools_parallel_enabled so existing agents pick up
// selective retry and parallel tool calls. Operators who want them off can
// disable per agent in settings after this one-shot backfill.
// Idempotent: only FALSE rows are updated. TRUE/FALSE literals (see
// ddlIntentPassDefaultOnMigration).
func ddlToolsRetryParallelDefaultOn(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	exists, err := d.TableExists(ctx, rawDB, "agent_runtime_settings")
	if err != nil {
		return fmt.Errorf("tools_retry_parallel_default_on: check table: %w", err)
	}
	if !exists {
		return nil
	}
	tx, err := rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tools_retry_parallel_default_on tx: %w", err)
	}
	defer tx.Rollback()
	resRetry, err := tx.ExecContext(ctx, `
		UPDATE agent_runtime_settings
		SET tools_retry_enabled = TRUE
		WHERE tools_retry_enabled = FALSE
	`)
	if err != nil {
		return fmt.Errorf("migrate tools_retry_enabled default on: %w", err)
	}
	resPar, err := tx.ExecContext(ctx, `
		UPDATE agent_runtime_settings
		SET tools_parallel_enabled = TRUE
		WHERE tools_parallel_enabled = FALSE
	`)
	if err != nil {
		return fmt.Errorf("migrate tools_parallel_enabled default on: %w", err)
	}
	nRetry, _ := resRetry.RowsAffected()
	nPar, _ := resPar.RowsAffected()
	if nRetry > 0 || nPar > 0 {
		lg.Info("tools_retry_parallel_default_on migration applied",
			loggateway.StepID("data.migration.tools_retry_parallel_default_on"),
			loggateway.Int("retry_rows", int(nRetry)),
			loggateway.Int("parallel_rows", int(nPar)))
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tools_retry_parallel_default_on migration: %w", err)
	}
	return nil
}

// ddlMonitorAlertFiringMsBigint widens the three millisecond-epoch columns on
// monitor_alert_rules from INTEGER to BIGINT (2026-08-17 真机 P2). The read
// path interprets them via time.UnixMilli, but PG int4 cannot hold ms epochs
// (≈1.7e12), so every MarkAlertFiredPersistent write failed with 22003 and
// the alert state machine was never persisted. Existing values are always
// NULL (writes never succeeded), so the type change is lossless.
// Postgres-only: SQLite INTEGER is 64-bit and unaffected.
// Idempotent: information_schema type guard, ALTER only when still integer.
func ddlMonitorAlertFiringMsBigint(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if !d.IsPostgres() || rawDB == nil {
		lg.Info("monitor_alert_firing_ms_bigint skipped (non-postgres or nil db)",
			loggateway.StepID("data.ddl_migration.monitor_alert_firing_ms_bigint"))
		return nil
	}
	for _, col := range []string{"last_fired_at", "last_fired_window_start", "recovered_at"} {
		var dataType string
		err := rawDB.QueryRowContext(ctx, `
			SELECT data_type FROM information_schema.columns
			WHERE table_name = 'monitor_alert_rules' AND column_name = $1`, col).Scan(&dataType)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return fmt.Errorf("check monitor_alert_rules.%s type: %w", col, err)
		}
		if dataType == "bigint" {
			continue
		}
		if _, err := rawDB.ExecContext(ctx, `ALTER TABLE monitor_alert_rules ALTER COLUMN `+col+` TYPE BIGINT`); err != nil {
			return fmt.Errorf("widen monitor_alert_rules.%s to bigint: %w", col, err)
		}
		lg.Info("monitor_alert_rules column widened to bigint",
			loggateway.StepID("data.ddl_migration.monitor_alert_firing_ms_bigint"),
			loggateway.Str("column", col))
	}
	return nil
}

// ddlSIRunClosedReasonWiden widens self_improvement_runs.closed_reason from
// varchar(64) to varchar(512) (2026-08-19 排障事故：64 字符截断丢失失败根因）。
// PG varchar 加宽仅系统目录变更、不重写表、无损。SQLite 不校验 varchar 长度，
// 跳过。幂等：information_schema 长度守卫，已达 512 不再 ALTER。
func ddlSIRunClosedReasonWiden(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if !d.IsPostgres() || rawDB == nil {
		lg.Info("si_run_closed_reason_widen skipped (non-postgres or nil db)",
			loggateway.StepID("data.ddl_migration.si_run_closed_reason_widen"))
		return nil
	}
	var maxLen *int
	err := rawDB.QueryRowContext(ctx, `
		SELECT character_maximum_length FROM information_schema.columns
		WHERE table_name = 'self_improvement_runs' AND column_name = 'closed_reason'`).Scan(&maxLen)
	if err == sql.ErrNoRows || maxLen == nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check self_improvement_runs.closed_reason length: %w", err)
	}
	if *maxLen >= 512 {
		return nil
	}
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE self_improvement_runs ALTER COLUMN closed_reason TYPE varchar(512)`); err != nil {
		return fmt.Errorf("widen self_improvement_runs.closed_reason to 512: %w", err)
	}
	lg.Info("self_improvement_runs.closed_reason widened to 512",
		loggateway.StepID("data.ddl_migration.si_run_closed_reason_widen"))
	return nil
}

func ddlRelationMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "does not exist") || strings.Contains(msg, "no such table")
}

// ddlUsageWaitMSColumns adds wait_ms / model_latency_ms on model_token_usage_events
// and backfills from metadata_json (HITL wait previously JSON-only).
func ddlUsageWaitMSColumns(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	for _, col := range []string{"wait_ms", "model_latency_ms"} {
		stmt := fmt.Sprintf(`ALTER TABLE model_token_usage_events ADD COLUMN IF NOT EXISTS %s INTEGER NOT NULL DEFAULT 0`, col)
		if _, err := rawDB.ExecContext(ctx, stmt); err != nil {
			if ddlRelationMissing(err) {
				lg.Info("usage_wait_ms_columns skipped (table missing)",
					loggateway.StepID("data.ddl_migration.usage_wait_ms"))
				return nil
			}
			if d.AlreadyExistsErr(err) {
				continue
			}
			return fmt.Errorf("add model_token_usage_events.%s: %w", col, err)
		}
	}
	var backfill string
	if d.IsPostgres() {
		backfill = `
UPDATE model_token_usage_events SET
  wait_ms = COALESCE((metadata_json::json->>'wait_ms')::int, 0),
  model_latency_ms = COALESCE((metadata_json::json->>'model_latency_ms')::int, 0)
WHERE wait_ms = 0 AND model_latency_ms = 0
  AND metadata_json IS NOT NULL AND metadata_json <> '' AND metadata_json <> '{}'
  AND (metadata_json::jsonb ? 'wait_ms' OR metadata_json::jsonb ? 'model_latency_ms')`
	} else {
		backfill = `
UPDATE model_token_usage_events SET
  wait_ms = COALESCE(CAST(json_extract(metadata_json, '$.wait_ms') AS INTEGER), 0),
  model_latency_ms = COALESCE(CAST(json_extract(metadata_json, '$.model_latency_ms') AS INTEGER), 0)
WHERE wait_ms = 0 AND model_latency_ms = 0
  AND (json_extract(metadata_json, '$.wait_ms') IS NOT NULL
    OR json_extract(metadata_json, '$.model_latency_ms') IS NOT NULL)`
	}
	if _, err := rawDB.ExecContext(ctx, backfill); err != nil {
		return fmt.Errorf("backfill usage wait_ms: %w", err)
	}
	return nil
}

// ddlKnowledgeChunksEmbeddingHNSW converts the knowledge ANN index from
// ivfflat to HNSW. Skipped when knowledge_chunks is not present yet
// (EnsureKnowledgeSchema owns the fresh shape).
func ddlKnowledgeChunksEmbeddingHNSW(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, lg loggateway.Logger) error {
	if !d.IsPostgres() || rawDB == nil {
		lg.Info("knowledge_chunks_embedding_hnsw skipped (non-postgres or nil db)",
			loggateway.StepID("data.ddl_migration.knowledge_hnsw"))
		return nil
	}
	if err := ensureKnowledgeEmbeddingHNSW(ctx, rawDB); err != nil {
		if ddlRelationMissing(err) {
			lg.Info("knowledge_chunks_embedding_hnsw skipped (table missing)",
				loggateway.StepID("data.ddl_migration.knowledge_hnsw"))
			return nil
		}
		return err
	}
	return nil
}

func ddlHealRecordMetadataColumn(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, _ loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE heal_records ADD COLUMN metadata TEXT NOT NULL DEFAULT '{}'`); err != nil && !d.AlreadyExistsErr(err) {
		return fmt.Errorf("add heal_records.metadata: %w", err)
	}
	return nil
}
