package data

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

//go:embed sql/migrations/*.sql
var migrationSQLFS embed.FS

type ddlMigration struct {
	Version int
	Name    string
	// SQL is an optional path to a SQL file to execute (relative to project root).
	// If set, the SQL file is executed first via rawDB, then Func is called (if not nil).
	SQL  string
	Func func(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error
}

// ddlMigrations is the ordered registry of DDL patches. Split across
// ddl_migration_table_early.go / ddl_migration_table_late.go for AS-COG-01.
var ddlMigrations = append(append([]ddlMigration{}, ddlMigrationsEarly...), ddlMigrationsLate...)

// RunDDLMigrationsExternal runs DDL migrations with the given dialect.
// This is exported for use by external tools (e.g. cmd/migrate-sqlite-to-postgres)
// to ensure DDL-managed tables (FTS5, monitor, memory, trpc session, etc.) exist
// before data migration. The entClient must be connected to the same database as
// rawDB and must use the same dialect.
func RunDDLMigrationsExternal(rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return runDDLMigrationsWithDialect(rawDB, entClient, d, lg)
}

// runDDLMigrationsWithDialect runs DDL migrations with dialect-aware error handling.
// Use this when the primary database is Postgres to ensure idempotent error detection
// matches the active dialect.
//
// C-28: on Postgres, the whole migration loop is wrapped in a session-scoped
// advisory lock so concurrent replicas cannot apply/record migrations in parallel.
func runDDLMigrationsWithDialect(rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	ctx := context.Background()
	return withDDLMigrationAdvisoryLock(ctx, rawDB, d, lg, func() error {
		for _, m := range ddlMigrations {
			applied, err := isMigrationApplied(ctx, entClient, m.Version, lg)
			if err != nil {
				lg.Warn("migration check failed, running patch anyway",
					loggateway.StepID("data.ddl_migration.check"),
					loggateway.Int("version", m.Version),
					loggateway.Err(err))
				applied = false
			}
			if applied {
				continue
			}
			// Execute SQL file first if set
			if m.SQL != "" {
				if err := executeSQLFileWithDialect(ctx, rawDB, m.SQL, d, lg); err != nil {
					lg.Error("schema step (SQL) failed",
						loggateway.StepID("data.schema."+m.Name),
						loggateway.Int("version", m.Version),
						loggateway.Err(err))
					return fmt.Errorf("%s: %w", m.Name, err)
				}
			}
			// Then execute Func if set
			if m.Func != nil {
				if err := m.Func(ctx, rawDB, entClient, d, lg); err != nil {
					lg.Error("schema step failed",
						loggateway.StepID("data.schema."+m.Name),
						loggateway.Int("version", m.Version),
						loggateway.Err(err))
					return fmt.Errorf("%s: %w", m.Name, err)
				}
			}
			if err := recordMigrationApplied(ctx, entClient, d, m.Version, m.Name, lg); err != nil {
				lg.Error("failed to record migration, aborting to prevent re-execution",
					loggateway.StepID("data.ddl_migration.record"),
					loggateway.Int("version", m.Version),
					loggateway.Err(err))
				return fmt.Errorf("record migration %s: %w", m.Name, err)
			}
		}
		// P6 类问题根治（2026-08-21）：版本化迁移条目是「applied 即跳过」的一次性
		// 执行，而若干条目（20260601/0606/0616/0617/0702/0710-0713 等）的 Func 只是
		// 转调幂等 Ensure*Schema 函数。这些函数体在版本发布后被追加 ADD COLUMN 时，
		// 存量库永不重跑 → 列永久缺失（已实证两例：orchestrations.cancel_reason、
		// audit_logs.user_agent，分别由 20261235/20261236 显式迁移补救）。
		// 这里在迁移循环结束后每次启动幂等重跑全部此类 Ensure/补丁函数（均在
		// advisory lock 内、自带列存在检查或 AlreadyExistsErr 容错），使存量库在
		// 下一次启动时自动收敛到代码当前确保的列集——今后再向这些函数体追加
		// ALTER 不再需要补 reseed 迁移。与 EnsureKnowledgeSchema 的启动期 reconcile
		// 先例（data.go ensurePostgresSchemas）对齐。
		return reconcileVersionedEnsureSchemas(ctx, rawDB, entClient, d, lg)
	})
}

// reconcileVersionedEnsureSchemas 每次启动幂等重跑所有「被版本化迁移条目包装、
// 且函数体内嵌 ADD COLUMN」的 Ensure/补丁函数。全部满足：nil-safe、幂等
// （列存在预检或 AlreadyExistsErr 容错）、双方言。失败即报错阻断启动——
// 与迁移 runner 同等严格，静默发散比失败更危险（EVAL-07 同哲学）。
func reconcileVersionedEnsureSchemas(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	if rawDB != nil {
		rawEnsures := []struct {
			name string
			fn   func() error
		}{
			{"orchestration_schema", func() error { return EnsureOrchestrationSchema(ctx, rawDB, d, lg) }},
			{"eval_schema", func() error { return EnsureEvalSchema(ctx, rawDB, d) }},
			{"a2a_schema", func() error { return EnsureA2ASchema(ctx, rawDB, d) }},
			{"task_plan_schema", func() error { return EnsureTaskPlanSchema(ctx, rawDB, lg) }},
			{"allocation_plan_schema", func() error { return EnsureAllocationPlanSchema(ctx, rawDB, lg) }},
			{"agent_performance_schema", func() error { return EnsureAgentPerformanceSchema(ctx, rawDB, lg) }},
			{"compiled_team_schema", func() error { return EnsureCompiledTeamSchema(ctx, rawDB) }},
			{"channel_inbound_schema", func() error { return EnsureChannelInboundSchema(ctx, rawDB) }},
			{"channel_turn_job_schema", func() error { return EnsureChannelTurnJobSchema(ctx, rawDB) }},
			{"channel_runtime_lease_schema", func() error { return EnsureChannelRuntimeLeaseSchema(ctx, rawDB) }},
		}
		for _, e := range rawEnsures {
			if err := e.fn(); err != nil {
				return fmt.Errorf("reconcile %s: %w", e.name, err)
			}
		}
	}
	if entClient != nil {
		if err := sessionMemoryEnsurePatches(ctx, entClient, d); err != nil {
			return fmt.Errorf("reconcile session_memory_patches: %w", err)
		}
		if err := sessionMemoryEnsureMonitorSchemaPatches(ctx, entClient, d); err != nil {
			return fmt.Errorf("reconcile monitor_schema_patches: %w", err)
		}
		if err := EnsureMonitorAlertSchema(ctx, entClient, d); err != nil {
			return fmt.Errorf("reconcile monitor_alert_schema: %w", err)
		}
	}
	return nil
}

// ddlMigrationAdvisoryLockKey is a stable int64 key for pg_advisory_lock.
// Value chosen to be unique among Aranea locks (ASCII "ARAN" + migration).
const ddlMigrationAdvisoryLockKey int64 = 0x4152414E_4D4947 // "ARANMIG" truncated

// withDDLMigrationAdvisoryLock serializes migration runners on Postgres via a
// dedicated connection holding pg_advisory_lock for the duration of fn.
// Non-Postgres dialects run fn without a lock (single-process / test adapters).
func withDDLMigrationAdvisoryLock(ctx context.Context, rawDB *sql.DB, d Dialect, lg loggateway.Logger, fn func() error) error {
	if rawDB == nil || !d.IsPostgres() {
		return fn()
	}
	conn, err := rawDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("ddl migration advisory lock: acquire conn: %w", err)
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			lg.Warn("ddl migration lock conn close failed",
				loggateway.StepID("data.ddl_migration.lock"),
				loggateway.Err(cerr))
		}
	}()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, ddlMigrationAdvisoryLockKey); err != nil {
		return fmt.Errorf("ddl migration advisory lock: %w", err)
	}
	defer func() {
		if _, uerr := conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, ddlMigrationAdvisoryLockKey); uerr != nil {
			lg.Warn("ddl migration advisory unlock failed",
				loggateway.StepID("data.ddl_migration.unlock"),
				loggateway.Err(uerr))
		}
	}()
	lg.Info("ddl migration advisory lock acquired",
		loggateway.StepID("data.ddl_migration.lock"),
		loggateway.Int64("lock_key", ddlMigrationAdvisoryLockKey))
	return fn()
}

// executeSQLFileWithDialect is the dialect-aware SQL file executor.
// Uses Dialect.AlreadyExistsErr and Dialect.UndefinedObjectErr for idempotent
// error detection across SQLite and Postgres.
func executeSQLFileWithDialect(ctx context.Context, rawDB *sql.DB, path string, d Dialect, lg loggateway.Logger) error {
	if rawDB == nil {
		return fmt.Errorf("execute SQL file %s: rawDB is nil", path)
	}
	sqlBytes, err := fs.ReadFile(migrationSQLFS, path)
	if err != nil {
		return fmt.Errorf("read SQL file %s: %w", path, err)
	}
	ddl := strings.TrimPrefix(string(sqlBytes), "\ufeff")
	if d.IsPostgres() {
		// Translate SQLite-specific DDL syntax to Postgres equivalents.
		// INTEGER PRIMARY KEY AUTOINCREMENT -> BIGSERIAL PRIMARY KEY
		ddl = translateSQLiteDDLToPostgres(ddl)
	}
	for _, stmt := range splitDDLStatements(strings.TrimSpace(ddl)) {
		if stmt == "" {
			continue
		}
		if d.IsPostgres() {
			stmt = translateSQLiteStatementToPostgres(stmt)
		}
		// Wrap each DDL statement in a transaction. When a statement fails
		// (e.g. "column already exists"), Postgres aborts the current
		// transaction; without an explicit rollback, the connection is
		// returned to the pool in an aborted state, causing subsequent
		// statements to fail with "could not complete operation in a failed
		// transaction". Using a per-statement transaction ensures the
		// connection is always returned clean.
		tx, txErr := rawDB.BeginTx(ctx, nil)
		if txErr != nil {
			return fmt.Errorf("begin tx for SQL statement in %s: %w", path, txErr)
		}
		_, err := tx.ExecContext(ctx, stmt)
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				lg.Debug("rollback after exec failure",
					loggateway.StepID("data.ddl_migration.sql_file"),
					loggateway.Err(rbErr))
			}
			if d.AlreadyExistsErr(err) {
				lg.Debug("ddl patch skipped (already exists)",
					loggateway.StepID("data.ddl_migration.sql_file"),
					loggateway.Str("statement", stmt[:min(len(stmt), 120)]))
				continue
			}
			if d.UndefinedObjectErr(err) {
				lg.Debug("ddl patch skipped (table not yet created, will be created by later migration)",
					loggateway.StepID("data.ddl_migration.sql_file"),
					loggateway.Str("statement", stmt[:min(len(stmt), 120)]))
				continue
			}
			return fmt.Errorf("execute SQL statement in %s: %w\n---\n%s", path, err, stmt)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit SQL statement in %s: %w\n---\n%s", path, err, stmt)
		}
	}
	lg.Info("executed SQL migration file",
		loggateway.StepID("data.ddl_migration.sql_file"),
		loggateway.Str("path", path))
	return nil
}

// ddlSessionRevisionDataMigration backfills session_revision from message counts.
// The ALTER TABLE is handled by the SQL file; this Func only does the data migration.
// Uses dialect-aware WHERE clause: SQLite needs `OR session_revision = ”` for
// type-coerced empty strings; Postgres uses proper bigint type and only needs IS NULL.

// Skip on fresh databases where the messages table was never created.
// Migration 20260902 drops the messages subsystem (superseded by activities);
// on a fresh DB the messages table never exists, so there is nothing to backfill.

// to_regclass returns NULL when the table doesn't exist; scan into a
// nullable string and check for empty/NULL.

// Phase 1c-3: messages_fts schema maintenance removed. The FTS5 virtual
// table and triggers are dropped by migration 20260902_drop_messages_subsystem.sql.
// This migration entry is retained for gate continuity on existing deployments.

// Wrap in transaction for atomicity

// Migrate agent kind: system -> system_builtin

// Migrate agent kind: industry_template -> ecosystem_preset

// Migrate team kind: source=imported -> kind=ecosystem_preset

// ddlAgentSourceDataMigration populates the new agents.source column from existing kind values.
// Mapping: kind=user -> source=user, kind=system_builtin -> source=system, kind=ecosystem_preset/marketplace/certified -> source=imported.
// Also fixes Team source and corrects over-broad Team kind migration from 20260718.

// Wrap in transaction for atomicity

// Agent source migration

// Team source migration: align with Agent source semantics
// kind=system_builtin -> source=system

// kind=ecosystem_preset -> source=imported

// Fix over-broad migration from 20260718: teams with kind='ecosystem_preset' but
// no ecosystem_preset agent members should be 'user'. The 20260718 migration set
// kind='ecosystem_preset' for ALL source='imported' teams, including user-imported packs.
// Heuristic: if a team has NO members referencing ecosystem_preset agents, revert to 'user'.
// Use dialect-aware JSON extraction: SQLite json_extract / json_each vs Postgres ->> / json_array_elements.
//
// This runs in a separate transaction because the complex JSON query may fail
// on some rows (e.g. malformed JSON); we treat it as non-critical and don't
// want to abort the entire migration.
//
// TECH-DEBT(debt): This migration is best-effort: errors are logged but not
// returned, which means the migration version is still recorded as applied
// even if the fix failed. This could leave teams with incorrect kind on
// retry. Acceptable because the heuristic is non-destructive (only reverts
// kind to 'user', never corrupts data). Issue: track via TECH-DEBT log.

// Non-critical: log the error but don't fail the entire migration

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

// ddlUnifiedEvolutionRebuildPendingIndex replaces the (target_type, target_id)
// pending unique index with one whose dedup key preserves legacy semantics.

// ddlUnifiedBackfillRow is one legacy suggestion row normalized for insertion
// into unified_evolution_suggestions.

// ddlInsertUnifiedBackfill inserts one backfilled row, skipping rows whose id
// already exists (idempotent re-runs and bridge-dual-written rows).

// already present

// ddlDropLegacyTable drops a legacy table after successful backfill.

// ddlBackfillSkillEvolutionSuggestions backfills L2 (skill-scoped curator
// suggestions) into the unified store, then drops the legacy table.

// sourceReportIDsOrEmpty normalizes a legacy JSON-array TEXT column to a valid
// JSON array literal for embedding into backfilled metadata.

// ddlBackfillSkillProposals backfills L1 (agent-scoped skill-creation
// proposals) into the unified store, then drops the legacy table.

// ddlBackfillEvolutionSuggestions backfills L3 (agent-scoped persona/prompt/
// skill suggestions) into the unified store, then drops the legacy table.

// Fresh databases never had the legacy L3 table (its Ent schema was removed
// in the A6 unified-store change); only pre-A6 databases carry it, and only
// they need the column before convergence (20261111) backfills and drops it.

// ddlHealRecordMetadataColumn adds the metadata column to heal_records table
// for persisting extra context (root cause metadata, event metadata, etc.)
// as a JSON-encoded TEXT field. Idempotent: "duplicate column" errors are
// treated as success per DB-N6.

// ddlIntentPassDefaultOnMigration corrects the historical false default of
// intent_pass_enabled for non-A2A agents. A2A proxy agents (config_json contains
// "a2a_proxy") keep false (set explicitly in biz.validateAgentUpdate when
// IsA2AProxyAgent(agent) is true).
// Idempotent: only updates rows where intent_pass_enabled = FALSE.

// Flip false → true for non-A2A agents. A2A proxy agents are identified by
// config_json containing "a2a_proxy" (agent_kind=a2a_proxy embedded by
// EmbedAgentKindInConfigJSON). Excluding them preserves their explicit false.
// Use TRUE/FALSE literals (not 1/0): Ent field.Bool maps to PostgreSQL boolean,
// which rejects "boolean = integer" with pq: operator does not exist (42883).
// SQLite 3.23+ also accepts TRUE/FALSE as aliases for 1/0, so this is portable.

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

// ddlMemoryFactsFTSIndex creates the GIN FTS index on memory_facts (P2-3).
// Postgres-only: to_tsvector/GIN do not exist on SQLite, so CLI tools and
// SQLite tests skip it (FTS candidate search degrades to nil there).

// ddlMemoryFactsTrgmIndex creates the pg_trgm GIN index on memory_facts.statement
// (P1-2). Postgres-only: gin_trgm_ops does not exist on SQLite.

// ddlDecisionRecordsGINIndexes creates the GIN expression indexes on
// decision_records source_ref / related_entities (M80 Phase 1, NFR-80-04).
// Postgres-only: GIN/jsonb cast do not exist on SQLite; columns are TEXT-stored
// JSON per dual-dialect convention, so the index targets (col::jsonb).

// ddlDecisionRecordsRunIDIndex creates the btree expression index serving
// source_run_id equality filters (M80 1.10 perf tuning, NFR-80-04).
// Postgres-only: the indexed expression casts TEXT->jsonb; SQLite skips.

// ddlDecisionRecordsSessionIDIndex creates the btree expression index serving
// source_session_id equality filters and SessionGateStats aggregation (T5,
// 2026-08-27 chat-side gate aggregation surface). Postgres-only: the indexed
// expression casts TEXT->jsonb; SQLite skips.

// ddlTenantRLSPhase1 enables Postgres RLS on tenant-owned tables (C-25).
// Skipped on non-Postgres dialects (SQLite tests / CLI).

// ddlAgentsPositionPartialUnique narrows agents(position_key, agent_variant) to a
// partial unique index (BUG-01, 2026-08-17): rows with empty position_key and
// soft-deleted rows no longer occupy a position slot; the design intent is only
// "one active agent per (position, variant)". Postgres-only: SQLite 侧由 Ent
// schema（entsql.IndexWhere 同谓词）建库时直接落成部分索引。
// Idempotent: DROP IF EXISTS + CREATE UNIQUE INDEX IF NOT EXISTS.

// ddlDropRetiredLegacyTables drops retired tables (activities / event_store)
// that linger in存量库: fresh installs已不含这些表，但旧二进制经 Ent auto-migrate
// 可将其复活（BUG-02 F4，2026-08-17）。现行代码无 Ent schema、级联/生产代码均无引用，
// 数据为退役遗留。Idempotent: DROP TABLE IF EXISTS.
// sessions_v2 刻意保留：spirit 会话根实体（session_v2_repo.go 活跃），非死表。

// ddlUnifiedEvolutionWorkspace adds workspace_id to unified_evolution_suggestions
// (进化建议模块 P0-1a, IDOR 修复): column + index + backfill from host tables
// (skill/agents) + RLS policy. Skipped on non-Postgres dialects.

// ddlUsageEventsTraceIDIndex adds an expression index on
// model_token_usage_events(metadata.trace_id). AggregateUsageByTrace filters
// by this expression once per trace completion and per backfill row; without
// an index every lookup seq-scans the growing events table. The index uses
// the same Dialect.JSONExtract expression as the query so the planner can
// always match it. Idempotent: CREATE INDEX IF NOT EXISTS.

// Double parens: Postgres requires index expressions that are not a plain
// function call (here: COALESCE(...) ->> 'trace_id') to be parenthesized.

// ddlToolGrantsExpiresAt adds expires_at to tool_grants (BUG-MON-B, 2026-08-17):
// persisted "always allow" grants previously had no TTL — residual high-risk
// grants (shell_exec/hostexec/playwright 等) stayed effective forever.
// Adds the column and backfills existing rows to created_at + 72h (most
// expire immediately; read paths filter + lazily clean them). New grants
// write now+72h via ToolUsecase.GrantTool. created_at is RFC3339 UTC text
// (nowRFC3339 惯例); the regex guard skips malformed rows so the cast can
// never abort startup. Postgres-only: SQLite 侧由 Ent schema 建列。
// Idempotent: ADD COLUMN IF NOT EXISTS + value-guarded UPDATE.

// ddlToolsRetryParallelDefaultOn flips historical false defaults for
// tools_retry_enabled / tools_parallel_enabled so existing agents pick up
// selective retry and parallel tool calls. Operators who want them off can
// disable per agent in settings after this one-shot backfill.
// Idempotent: only FALSE rows are updated. TRUE/FALSE literals (see
// ddlIntentPassDefaultOnMigration).

// ddlMonitorAlertFiringMsBigint widens the three millisecond-epoch columns on
// monitor_alert_rules from INTEGER to BIGINT (2026-08-17 真机 P2). The read
// path interprets them via time.UnixMilli, but PG int4 cannot hold ms epochs
// (≈1.7e12), so every MarkAlertFiredPersistent write failed with 22003 and
// the alert state machine was never persisted. Existing values are always
// NULL (writes never succeeded), so the type change is lossless.
// Postgres-only: SQLite INTEGER is 64-bit and unaffected.
// Idempotent: information_schema type guard, ALTER only when still integer.

// 列不存在：由 ensure DDL 以 BIGINT 形态补建

// ddlSIRunClosedReasonWiden widens self_improvement_runs.closed_reason from
// varchar(64) to varchar(512) (2026-08-19 排障事故：64 字符截断丢失失败根因）。
// PG varchar 加宽仅系统目录变更、不重写表、无损。SQLite 不校验 varchar 长度，
// 跳过。幂等：information_schema 长度守卫，已达 512 不再 ALTER。

// 列不存在：新库由 ent Schema.Create 以 512 形态建表

// ddlUsageWaitMSColumns adds wait_ms / model_latency_ms on model_token_usage_events
// and backfills from metadata_json (HITL wait previously JSON-only).

// ddlKnowledgeChunksEmbeddingHNSW converts the knowledge ANN index from
// ivfflat to HNSW. Skipped when knowledge_chunks is not present yet
// (EnsureKnowledgeSchema owns the fresh shape).
