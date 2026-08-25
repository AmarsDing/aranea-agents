package data

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	"aranea-agents/internal/biz"
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

var ddlMigrations = []ddlMigration{
	{Version: 20260601, Name: "session_memory_patches", Func: ddlSessionMemoryPatches},
	{Version: 20260604, Name: "session_memory_schema", Func: ddlSessionMemorySchema},
	// 20260602 memory_facts_index_status: columns already included in memory_chain.sql CREATE TABLE (20260604).
	{Version: 20260602, Name: "memory_facts_index_status"},
	{Version: 20260603, Name: "messages_turn_number", Func: ddlMessagesTurnNumber},
	// 20260605 memory_relation_patches: valid_from/valid_to already included in memory_chain.sql CREATE TABLE (20260604).
	{Version: 20260605, Name: "memory_relation_patches"},
	{Version: 20260606, Name: "monitor_schema_patches", Func: ddlMonitorSchemaPatches},
	{Version: 20260607, Name: "agent_runtime_patches", SQL: "sql/migrations/20260607_agent_runtime_patches.sql"},
	{Version: 20260608, Name: "entity_reinforcements_schema", SQL: "sql/migrations/20260608_entity_reinforcements_schema.sql"},
	{Version: 20260609, Name: "cascade_saga_patches", SQL: "sql/migrations/20260609_cascade_saga_patches.sql"},
	{Version: 20260610, Name: "builtin_platform_tools", Func: ddlBuiltinPlatformTools},
	{Version: 20260611, Name: "system_setting_patches", SQL: "sql/migrations/20260611_system_setting_patches.sql"},
	{Version: 20260612, Name: "pricing_rule_patches", SQL: "sql/migrations/20260612_pricing_rule_patches.sql"},
	{Version: 20260613, Name: "llm_provider_model_capability", SQL: "sql/migrations/20260613_llm_provider_model_capability.sql"},
	{Version: 20260614, Name: "default_system_setting", Func: ddlDefaultSystemSetting},
	{Version: 20260615, Name: "credential_encryption_key", Func: ddlCredentialEncryptionKey},
	{Version: 20260616, Name: "eval_schema", Func: ddlEvalSchema},
	{Version: 20260617, Name: "a2a_schema", Func: ddlA2ASchema},
	{Version: 20260618, Name: "a2a_remote_health_patches", SQL: "sql/migrations/20260618_a2a_remote_health_patches.sql"},
	{Version: 20260619, Name: "team_run_summary_patches", SQL: "sql/migrations/20260619_team_run_summary_patches.sql"},
	{Version: 20260620, Name: "session_revision_patches", SQL: "sql/migrations/20260620_session_revision_patches.sql", Func: ddlSessionRevisionDataMigration},
	{Version: 20260621, Name: "plugin_run_schema", Func: ddlPluginRunSchema},
	{Version: 20260622, Name: "hook_delivery_schema", SQL: "sql/migrations/20260622_hook_delivery_schema.sql"},
	{Version: 20260623, Name: "flow_log_schema", Func: ddlFlowLogSchema},
	{Version: 20260624, Name: "message_fts_schema", Func: ddlMessageFTSSchema},
	{Version: 20260625, Name: "channel_inbound_schema", Func: ddlChannelInboundSchema},
	{Version: 20260626, Name: "channel_turn_job_schema", Func: ddlChannelTurnJobSchema},
	{Version: 20260627, Name: "channel_runtime_lease_schema", Func: ddlChannelRuntimeLeaseSchema},
	{Version: 20260628, Name: "session_run_schema", Func: ddlSessionRunSchema},
	{Version: 20260629, Name: "session_participant_schema", Func: ddlSessionParticipantSchema},
	{Version: 20260630, Name: "session_run_checkpoint_schema", SQL: "sql/migrations/20260630_session_run_checkpoint_schema.sql"},
	// 20260701 session_run_column_patches: checkpoint_id/agent_id/resume_started_at already in session_run_schema.go CREATE TABLE (20260628).
	{Version: 20260701, Name: "session_run_column_patches"},
	{Version: 20260702, Name: "monitor_alert_schema", Func: ddlMonitorAlertSchema},
	{Version: 20260703, Name: "ecosystem_schema", Func: ddlEcosystemSchema},
	{Version: 20260704, Name: "team_graph_session_schema", Func: ddlTeamGraphSessionSchema},
	{Version: 20260705, Name: "compiled_team_schema", Func: ddlCompiledTeamSchema},
	{Version: 20260706, Name: "skill_evolution_schema", Func: ddlSkillEvolutionSchema},
	{Version: 20260707, Name: "memory_facts_extra_patches", SQL: "sql/migrations/20260707_memory_facts_extra_patches.sql"},
	{Version: 20260708, Name: "session_table_split", SQL: "sql/migrations/20260708_session_table_split.sql"},
	{Version: 20260709, Name: "vector_embedding_ref", SQL: "sql/migrations/20260709_vector_embedding_ref.sql"},
	{Version: 20260710, Name: "task_plan_schema", Func: ddlTaskPlanSchema},
	{Version: 20260711, Name: "allocation_plan_schema", Func: ddlAllocationPlanSchema},
	{Version: 20260712, Name: "agent_performance_schema", Func: ddlAgentPerformanceSchema},
	{Version: 20260713, Name: "orchestration_schema", Func: ddlOrchestrationSchema},
	// 20260714 compiled_team_session_id: session_id + index already in compiled_team_schema.go CREATE TABLE (20260705).
	{Version: 20260714, Name: "compiled_team_session_id"},
	{Version: 20260715, Name: "self_check_report_schema", SQL: "sql/migrations/20260715_self_check_report_schema.sql"},
	{Version: 20260716, Name: "missing_indexes", SQL: "sql/migrations/20260716_missing_indexes.sql"},
	{Version: 20260717, Name: "usage_events_schema", SQL: "sql/migrations/20260717_usage_events_schema.sql"},
	{Version: 20260718, Name: "ecosystem_preset_schema", SQL: "sql/migrations/20260718_ecosystem_preset_schema.sql", Func: ddlEcosystemPresetDataMigration},
	{Version: 20260719, Name: "agent_source_column", SQL: "sql/migrations/20260719_agent_source_column.sql", Func: ddlAgentSourceDataMigration},
	{Version: 20260720, Name: "unified_evolution_schema", Func: ddlUnifiedEvolutionSchema},
	{Version: 20260721, Name: "evolution_suggestion_pre_apply_snapshot", Func: ddlEvolutionSuggestionPreApplySnapshot},
	{Version: 20260722, Name: "activity_schema", SQL: "sql/migrations/20260722_activity_schema.sql"},
	{Version: 20260723, Name: "activity_token_columns", SQL: "sql/migrations/20260723_activity_token_columns.sql"},
	{Version: 20260724, Name: "invariant_constraints", SQL: "sql/migrations/20260724_invariant_constraints.sql"},
	{Version: 20260725, Name: "memory_bitemporal", SQL: "sql/migrations/20260725_memory_bitemporal.sql"},
	{Version: 20260726, Name: "memory_links", SQL: "sql/migrations/20260726_memory_links.sql"},
	{Version: 20260727, Name: "memory_decay_columns", SQL: "sql/migrations/20260727_memory_decay_columns.sql"},
	{Version: 20260728, Name: "memory_job_deadletter_schema", SQL: "sql/migrations/20260728_memory_job_deadletter_schema.sql"},
	{Version: 20260730, Name: "runtime_profile_schema", SQL: "sql/migrations/20260730_runtime_profile_schema.sql"},
	{Version: 20260731, Name: "heal_record_metadata_column", Func: ddlHealRecordMetadataColumn},
	{Version: 20260801, Name: "memory_job_deadletter_unique", SQL: "sql/migrations/20260801_memory_job_deadletter_unique.sql"},
	// 20260802 memory_episodes_l1_task_unique 与 20260803 cascade_saga_id_type_fix
	// 原版本号被同名数据迁移（session_turn_number_backfill/rebackfill）抢先占用，
	// 导致 DDL 被静默跳过（生产 memory_episodes 缺部分唯一索引、cascade_saga_steps.id
	// 仍为 integer）。已重编号为 20261118/20261119，见本文件末尾。
	{Version: 20260804, Name: "planner_model_columns", SQL: "sql/migrations/20260804_planner_model_columns.sql"},
	// 20260808 speech_columns: M74 V2-T7 System Settings「语音服务」分组——
	// ASR/TTS driver/endpoint/凭据/音色/语速 + 语音留档开关（nullable 三态，
	// NULL=回退 env）。同 planner_model_columns 的 raw-SQL 读写模式。
	{Version: 20260808, Name: "speech_columns", SQL: "sql/migrations/20260808_speech_columns.sql"},
	// 20260809 speech_api_key_columns: M74 真机校准——X-Api-Key 鉴权模式列
	// （火山控制台新 API Key，与 legacy AppKey+AccessKey 双模式并存）。
	{Version: 20260809, Name: "speech_api_key_columns", SQL: "sql/migrations/20260809_speech_api_key_columns.sql"},
	{Version: 20260825, Name: "activity_session_tree_columns", SQL: "sql/migrations/20260825_activity_session_tree_columns.sql"},
	{Version: 20260826, Name: "event_dead_letter_schema", SQL: "sql/migrations/20260826_event_dead_letter_schema.sql"},
	{Version: 20260901, Name: "drop_event_store_subsystem", SQL: "sql/migrations/20260901_drop_event_store_subsystem.sql"},
	{Version: 20260902, Name: "drop_messages_subsystem", SQL: "sql/migrations/20260902_drop_messages_subsystem.sql"},
	// 20260903 intent_pass_default_on: correct historical false default for non-A2A agents (P1-1).
	// Ent schema default was false (bug); DDL migration 20260607 set column default 1 but Ent always
	// wrote explicit false on create. A2A proxy agents keep false (set explicitly in biz layer).
	{Version: 20260903, Name: "intent_pass_default_on", Func: ddlIntentPassDefaultOnMigration},
	// 20261001 v2_indexes: supplementary single-column indexes for v2 entity tables
	// (LLM Activity Ordering Phase 1). Ent Schema.Create() already creates table columns,
	// primary keys, and composite indexes declared in Indexes(); this migration adds
	// single-column indexes for common query patterns not covered by leftmost-prefix rule.
	{Version: 20261001, Name: "v2_indexes", SQL: "sql/migrations/20261001_v2_indexes.sql"},
	// 20261002 team_stage_team_name: add team_name column to team_stages_v2.
	// Ent Schema.Create() 不会为已存在表新增列，需要 ALTER TABLE 补列。
	{Version: 20261002, Name: "team_stage_team_name", SQL: "sql/migrations/20261002_team_stage_team_name.sql"},
	// 20261003 plan_step_agent_keys: add agent_keys column to plan_steps_v2.
	// PlanStep 携带 LLM 分配的 agent key 列表，RealTeamOrchestrator 优先使用此字段。
	{Version: 20261003, Name: "plan_step_agent_keys", SQL: "sql/migrations/20261003_plan_step_agent_keys.sql"},
	// 20261004 memory_context_note: add context_note column to memory_facts for
	// A-MEM style memory evolution (Phase 6A-03). Stores LLM-generated contextual
	// annotation explaining how this fact relates to / evolved from related memories.
	{Version: 20261004, Name: "memory_context_note", SQL: "sql/migrations/20261004_memory_context_note.sql"},
	// 20261005 memory_neuron_enhancement: add neuron-model fields to memory_entities
	// (activation/activation_updated_at/source_type/valence/arousal) and memory_relations
	// (co_activation_count/last_reinforced_at/context_note) for Phase E spreading activation,
	// Hebbian reinforcement, and A-MEM style relationship evolution (FR-10.1/10.2/10.7).
	{Version: 20261005, Name: "memory_neuron_enhancement", SQL: "sql/migrations/20261005_memory_neuron_enhancement.sql"},
	// 20261006 admin_workspace_id: add workspace_id column to admins table for
	// admin-Workspace 一对一绑定模型 (P2-A). empty = legacy/default admin.
	{Version: 20261006, Name: "admin_workspace_id", SQL: "sql/migrations/20261006_admin_workspace_id.sql"},
	// 20261007 tenant_owned_workspace_id: add workspace_id column to tenant-owned
	// entities (agents/teams/graph_definitions/plugins) for P2-B repo 层硬隔离.
	// empty = shared/legacy (visible to all workspaces); non-empty = tenant-private.
	{Version: 20261007, Name: "tenant_owned_workspace_id", SQL: "sql/migrations/20261007_tenant_owned_workspace_id.sql"},
	// 20261008 session_turn_idempotency_key: C-13 unique (session_id, idempotency_key)
	// so AdmitTurn retries return the canonical turn instead of duplicating LLM work.
	{Version: 20261008, Name: "session_turn_idempotency_key", SQL: "sql/migrations/20261008_session_turn_idempotency_key.sql"},
	// 20261009 platform_workspace_id: P2-B Phase 2 — workspace_id on tools/skill/
	// mcp_server/channel/cron_task/eval_runs/tasks_v2/task_plans (Ent already had fields).
	{Version: 20261009, Name: "platform_workspace_id", SQL: "sql/migrations/20261009_platform_workspace_id.sql"},
	// 20261010 event_delivery_outbox: B-06 durable critical-event outbox for
	// WS last_event_id cursor replay. Primary durability; critical_journal is secondary.
	{Version: 20261010, Name: "event_delivery_outbox", SQL: "sql/migrations/20261010_event_delivery_outbox.sql"},
	// 20261011 tenant_rls_phase1: ENABLE ROW LEVEL SECURITY (no FORCE) on tenant-owned
	// tables with workspace_id. Postgres-only; skipped on SQLite via Func.
	{Version: 20261011, Name: "tenant_rls_phase1", Func: ddlTenantRLSPhase1},
	// 20261012 drop_activities_table: v1 activities persistence retired.
	// Reads already use steps_v2; Ent Activity schema removed; no production writers.
	{Version: 20261012, Name: "drop_activities_table", SQL: "sql/migrations/20261012_drop_activities_table.sql"},
	// 20261013 plan_step_contracts: add deliverables / input_contract columns to
	// plan_steps_v2 (P1 形式契约 B.10.15.2). Crash recovery rebuilds dagRun with
	// contracts intact; same field.JSON pattern as agent_keys.
	{Version: 20261013, Name: "plan_step_contracts", SQL: "sql/migrations/20261013_plan_step_contracts.sql"},
	// 20261014 media_providers: media generation provider configs (media generation
	// observation view). Originally 20261008 on release/installer-0.1.35; renumbered
	// to avoid collision with session_turn_idempotency_key on dev.
	{Version: 20261014, Name: "media_providers", SQL: "sql/migrations/20261008_media_providers.sql"},
	// 20261106 learning_loop_schema: learning_observations/learning_patterns/learning_proposals
	// tables were never wired into the migration registry (EnsureLearningLoopSchema was
	// defined but never called), causing 500 on /v1/agents/{id}/learning/* endpoints.
	// NOTE: version must exceed the max version already recorded in the target DB's
	// schema_migrations (20261105 at time of writing), otherwise it is skipped as applied.
	{Version: 20261106, Name: "learning_loop_schema", Func: ddlLearningLoopSchema},
	// 20261107 drop_micro_compact: drop micro_compact_enabled column from
	// agent_runtime_settings. L1 MicroCompact retired 2026-07-20 (dead feature:
	// loadCompressBody keeps only user/assistant messages, so tool-message
	// filtering never triggered).
	{Version: 20261107, Name: "drop_micro_compact", SQL: "sql/migrations/20261107_drop_micro_compact.sql"},
	// 20261108 agent_runtime_clarification: add clarification_enabled column to
	// agent_runtime_settings (P-CLARIFY B.10.18). Ent Schema.Create() 不会为已存在表
	// 新增列，需要 ALTER TABLE 补列；默认 1（门默认开启）。
	{Version: 20261108, Name: "agent_runtime_clarification", SQL: "sql/migrations/20261108_agent_runtime_clarification.sql"},
	// 20261109 steps_v2_session_seq: composite index for chat history lazy
	// load paged session query (WHERE session_id=? ORDER BY seq DESC LIMIT n+1).
	{Version: 20261109, Name: "steps_v2_session_seq", SQL: "sql/migrations/20261109_steps_v2_session_seq.sql"},
	// 20261110 agent_mission_domain: add mission_statement/domain_path columns to
	// agents (B.10.21 mission-driven matching). Ent Schema.Create() 不会为已存在表
	// 新增列，需要 ALTER TABLE 补列；存量行默认空串（走旧匹配管线）。
	{Version: 20261110, Name: "agent_mission_domain", SQL: "sql/migrations/20261110_agent_mission_domain.sql"},
	// 20261111 unified_evolution_convergence (A6): converge the four evolution-
	// suggestion stores into unified_evolution_suggestions. Rebuilds the
	// pending-dedup unique index (dialect-aware JSON path expression), backfills
	// legacy rows row-by-row (metadata JSON preserves legacy-only fields), and
	// drops the legacy tables. Pure Func: JSON functions are dialect-specific
	// and fresh databases never create the legacy tables.
	{Version: 20261111, Name: "unified_evolution_convergence", Func: ddlUnifiedEvolutionConvergence},
	// 20261112 organizations_enable_copy_position_backfill: set organizations.enabled=true
	// for the seeded agency hierarchy (legacy industry_taxonomy seed wrote enabled=false;
	// frontend taxonomy filter requires enabled=true), and backfill position_id/position_key
	// for copy agents created before the agent_duplicate.go fix cleared them.
	{Version: 20261112, Name: "organizations_enable_copy_position_backfill", Func: ddlOrganizationsEnableCopyPositionBackfill},
	// 20261113 a2a_remote_agents_org_id: add org_id column to a2a_remote_agents for
	// federation support. Links remote agents to federated organizations.
	{Version: 20261113, Name: "a2a_remote_agents_org_id", SQL: "sql/migrations/20261113_a2a_remote_agents_org_id.sql"},
	// 20261114 usage_events_trace_id_index: expression index on
	// model_token_usage_events metadata.trace_id so AggregateUsageByTrace
	// (trace completion + 6h backfill) does not seq-scan the events table.
	{Version: 20261114, Name: "usage_events_trace_id_index", Func: ddlUsageEventsTraceIDIndex},
	// 20261118 memory_episodes_l1_task_unique（原 20260802，版本碰撞重编号；
	// 20261116/20261117 已被生产 pack_it_ops_v1 种子记录占用，故取 20261118）：
	// memory_episodes 两个部分唯一索引——L1 归档 episode 按 (session_id,l1_task_id)
	// 去重，consolidation episode 按 (session_id,title,agent_id) WHERE l1_task_id=''
	// 去重。缺索引时 ON CONFLICT ... WHERE 报 42P10，episode 写入全部失败。
	{Version: 20261118, Name: "memory_episodes_l1_task_unique", SQL: "sql/migrations/20261118_memory_episodes_l1_task_unique.sql"},
	// 20261119 cascade_saga_id_type_fix（原 20260803，版本碰撞重编号）：
	// cascade_saga_steps 重建为 TEXT 主键（旧 INTEGER 无法存 UUID）。
	{Version: 20261119, Name: "cascade_saga_id_type_fix", SQL: "sql/migrations/20261119_cascade_saga_id_type_fix.sql"},
	// 20261120 self_improvement_observing_index（原 20261115，与数据迁移
	// monitor_trace_interrupted_backfill 碰撞重编号）：partial index on
	// self_improvement_runs(observe_until) WHERE status='observing' for the V3
	// Watchdog scan. Tables are Ent-managed (Schema.Create); only the partial
	// index needs DDL.
	{Version: 20261120, Name: "self_improvement_observing_index", SQL: "sql/migrations/20261120_self_improvement_observing_index.sql"},
	// 20261201 si_risk_rule_columns（原 20261121，与数据迁移
	// team_copy_ownership_to_user 的历史编号碰撞重编号——该数据迁移在以
	// 20261124 落库前曾用 20261121，凡在此窗口跑过它的库会把本迁移误判为
	// 已应用而永久跳过，si_risk_* 列缺失导致 GetRiskRules 500）：P5 console
	// risk-rule config on the system_settings singleton (same raw-SQL pattern
	// as planner_model_columns)。SQL 逐句幂等（AlreadyExistsErr 跳过），
	// 已建列库重跑安全。
	{Version: 20261201, Name: "si_risk_rule_columns", SQL: "sql/migrations/20261201_si_risk_rule_columns.sql"},
	// 20261125 memory_fact_three_counters: FR-12.6 recalled/injected/cited
	// three-stage counters replacing the semantically-wrong use_count, plus the
	// memory_fact_citations dedup ledger for the citation backfill worker.
	{Version: 20261125, Name: "memory_fact_three_counters", SQL: "sql/migrations/20261125_memory_fact_three_counters.sql"},
	// 20261126 memory_profile_cards: FR-12.7 resident profile card store, one
	// card per (agent_id, user_id) distilled by Sleep-time and injected 100%.
	{Version: 20261126, Name: "memory_profile_cards", SQL: "sql/migrations/20261126_memory_profile_cards.sql"},
	// 20261128 memory_facts_fts_index: P2-3 GIN index for L3 fact full-text
	// search (to_tsvector over statement+details_markdown). Postgres-only;
	// gated by Func so SQLite CLI/tests skip it.
	{Version: 20261128, Name: "memory_facts_fts_index", Func: ddlMemoryFactsFTSIndex},
	// 20261129 knowledge_entity_governance: G5-F B9/B12 实体治理——
	// knowledge_entities 加 name_norm 归一化列（Go 回填，PG 无 NFKC）+
	// knowledge_entity_aliases 别名表 + 归一化冲突组自动合并（keeper=id 最小者）
	// + (collection_id, name_norm) 唯一索引。Postgres-only（knowledge 依赖
	// pgvector）；fresh 库由 EnsureKnowledgeSchema 以新形态建表，Func 整体跳过。
	{Version: 20261129, Name: "knowledge_entity_governance", Func: ddlKnowledgeEntityGovernance},
	// 20261130 memory_recall_defaults_fix: P0-3/P0-4 记忆召回默认值修正——
	// l0_inject_l4 历史默认 false 导致 L4 图谱从未注入（存量 l4_enabled=true 的行
	// 全部置 true）；l3_recall_min_score 历史默认 0.55 按加权分布误杀典型相关命中
	// （≈0.4-0.5），存量 0.55 行降到 0.35（显式 0.00 = 用户关过滤，不动）。
	{Version: 20261130, Name: "memory_recall_defaults_fix", Func: ddlMemoryRecallDefaultsFix},
	// 20261202 builtin_platform_tools_client_reseed（M74 V2-T8 差距1修复）：
	// 20260610 builtin_platform_tools 在 client_open_app/client_open_url 加入种子
	// 列表之前已在存量库应用，schema_migrations 门控使其永不重跑 → 存量库 tools
	// 表缺 client 桥接工具（语音「打开微信」因查无工具行而误报客户端离线）。
	// 种子函数幂等（ON CONFLICT DO NOTHING + catalog/registry UPDATE），重跑安全。
	{Version: 20261202, Name: "builtin_platform_tools_client_reseed", Func: ddlBuiltinPlatformTools},
	// SP1-B 块级双链派生索引表（knowledge 域 Raw SQL 通道：TEXT id 与 knowledge_documents 一致；
	// 部分唯一索引/FK 级联语义超出 Ent 表达能力，版本化 DDL 显式控制）。
	{Version: 20261203, Name: "knowledge_blocks", SQL: "sql/migrations/20261203_knowledge_blocks.sql"},
	// SP1-C 跨库双链解析支撑列：documents.title/aliases（Resolver 文档键）+ links.weight（N-3 投影权重）。
	{Version: 20261204, Name: "knowledge_resolve", SQL: "sql/migrations/20261204_knowledge_resolve.sql"},
	// SP1-F 团队库后端维度：knowledge_collections.vault_backend（local=文件真相源 /
	// team=PG 真相源，设计 S6）。存量行默认 local 与历史语义一致，无需回填。
	{Version: 20261205, Name: "knowledge_vault_backend", SQL: "sql/migrations/20261205_knowledge_vault_backend.sql"},
	// SP2 #9 embedding 熔断：knowledge_documents.embed_fail_count/embed_last_tried
	// （embed 失败降级词法索引 + 指数退避后台重试）+ 降级文档扫描部分索引。
	{Version: 20261206, Name: "knowledge_embed_circuit", SQL: "sql/migrations/20261206_knowledge_embed_circuit.sql"},
	// 20261207 memory_agent_cases: P3 M2 Agent Case 经验记忆（EverOS Agent
	// Memory 启发）。会话结束后 AutoMemoryWorker 追加提取结构化任务经验
	// （goal/approach/outcome/pitfalls/tools_used），唯一锚点
	// (agent_id, source_session_id) 保证重试幂等。供 M3 召回注入与 M4
	// case→skill 蒸馏消费。
	{Version: 20261207, Name: "memory_agent_cases", SQL: "sql/migrations/20261207_memory_agent_cases.sql"},
	// 20261208 builtin_platform_tools_cua_reseed（75-computer-use + 76-coding-agent-bridge 修复）：
	// computer_use_*/coding_* 种子在 20260610/20261202 已应用的存量库中从未插入 →
	// spirit profile（group:computeruse）生效工具计算查无工具行，桌面自动化工具集整体缺席。
	// 种子函数幂等（ON CONFLICT DO NOTHING + catalog/registry UPDATE），重跑安全；
	// 顺带把 computer_use_act 的 actions[] 批量参数 schema 带给存量库。
	{Version: 20261208, Name: "builtin_platform_tools_cua_reseed", Func: ddlBuiltinPlatformTools},
	// 20261209 mcp_partial_unique_index（MCP 管理模块 R1 修复）：mcp_server.server_key
	// 列级 UNIQUE 与 mcp_server_user_credential 复合唯一索引均含软删除墓碑行，
	// 同 key 软删后重建报 23505。改为仅约束活跃行（deleted_at = ''）的部分唯一索引。
	// 新索引已声明进 Ent Schema（新库自动创建）；本迁移清理存量库旧约束/索引并补齐，
	// 逐句 IF EXISTS/IF NOT EXISTS 幂等。生产 Schema.Create 不删索引（无 WithDropIndex），
	// dev 模式 Ent 会先自动删旧建新，本迁移兜底 no-op。
	{Version: 20261209, Name: "mcp_partial_unique_index", SQL: "sql/migrations/20261209_mcp_partial_unique_index.sql"},
	// 20261210 plugin_cost_guard_usage_schema（plugin 模块 GAP-01 / I-2）：
	// cost_guard 日预算持久化表此前无 DDL 注册，全新部署 AddTokens 必败
	// （fail-closed 路径会持续报 plugin.cost_guard.try_consume_fail_closed）。
	// 版本须超过目标库 schema_migrations 已记录最大值（20261209）。
	{Version: 20261210, Name: "plugin_cost_guard_usage_schema", SQL: "sql/migrations/20261210_plugin_cost_guard_usage.sql"},
	// 20261211 plugin_runs_workspace（N-B5）：plugin_runs 加 workspace_id 列 +
	// 普通索引，支撑运行审计的租户可见性过滤（空串共享行全员可见，存量行为不变）。
	{Version: 20261211, Name: "plugin_runs_workspace", SQL: "sql/migrations/20261211_plugin_runs_workspace.sql"},
	// 20261212 unified_evolution_workspace（进化建议模块 P0-1a，IDOR 修复）：
	// unified_evolution_suggestions 加 workspace_id 列 + 索引 + 从宿主表
	// （skill/agents）backfill + RLS 策略（同 20261011 模板，ENABLE only）。
	// 含 RLS 语句，Postgres-only，经 Func 守卫跳过其他方言。
	{Version: 20261212, Name: "unified_evolution_workspace", Func: ddlUnifiedEvolutionWorkspace},
	// 20261213 tool_invocation_time_indexes（tools 模块 C2 修复）：
	// tool_invocations.started_at 与 tool_invocation_audit.created_at 的
	// 单列索引，支撑 24h 汇总、List 查询、ORDER BY created_at DESC、批量清理
	// 的时间范围扫描。复合索引 (tool_key, started_at) 不覆盖纯 started_at 过滤。
	{Version: 20261213, Name: "tool_invocation_time_indexes", SQL: "sql/migrations/20261213_tool_invocation_time_indexes.sql"},
	// 20261214 graph_execution_spirit_hash: Y4/Y5 恢复路径正确性——
	// graph_executions 增加 spirit_session_id（重启后 resume 的会话归属）与
	// definition_hash（C1 物化后 resume 的图定义一致性校验）；
	// team_graph_sessions 增加 spirit_session_id（RecoverSessions 恢复 watch 订阅过滤键）。
	{Version: 20261214, Name: "graph_execution_spirit_hash", SQL: "sql/migrations/20261214_graph_execution_spirit_hash.sql"},
	// 20261215 knowledge_citation_counters（29-token P2-2）：
	// knowledge_chunks 增加 cited_count（助手回复显式引用计数，回填 worker 维护）；
	// knowledge_chunk_citations(chunk_id, turn_id) 引用去重账本，重叠窗口重扫幂等。
	// 知识侧无 recalled/injected 计数（检索走工具调用而非 prompt 注入），只追踪 cited 段。
	{Version: 20261215, Name: "knowledge_citation_counters", SQL: "sql/migrations/20261215_knowledge_citation_counters.sql"},
	// 20261216 builtin_platform_tools_twinops_reseed（方案10 TwinOps 工具集）：
	// 17 个 twin_*/gns3_* 种子在 20260610 等已应用的存量库中从未插入 →
	// ops_* 岗位 effective keys 查无工具行。种子函数幂等
	// （ON CONFLICT DO NOTHING + catalog/registry UPDATE），重跑安全。
	{Version: 20261216, Name: "builtin_platform_tools_twinops_reseed", Func: ddlBuiltinPlatformTools},
	// 20261217 si_trigger_cooldown_multipliers（P1-14）：D8 触发器冷却倍率
	// 持久化到 system_settings 单例 JSON 列。重启后 Hydrate，避免冷却被
	// 重置后立刻再 auto-apply。语句天然幂等（AlreadyExistsErr 跳过）。
	{Version: 20261217, Name: "si_trigger_cooldown_multipliers", SQL: "sql/migrations/20261217_si_trigger_cooldown_multipliers.sql"},
	// 20261218 builtin_platform_tools_knowledge_write_reseed（知识库评审 P1）：
	// knowledge_write 种子在 20260610 已应用的存量库中不会插入（同 20261216
	// twinops 的情形）→ effective keys 查无工具行，工具永不装配。种子函数幂等
	// （ON CONFLICT DO NOTHING + catalog UPDATE），重跑安全。
	{Version: 20261218, Name: "builtin_platform_tools_knowledge_write_reseed", Func: ddlBuiltinPlatformTools},
	// 20261219 builtin_platform_tools_officecli_reseed（OfficeCLI 办公工具集）：
	// officecli_read/write/render 种子在 20260610 等已应用的存量库中不会插入
	// （同 20261216 twinops 的情形）→ effective keys 查无工具行，工具永不装配。
	// 种子函数幂等（ON CONFLICT DO NOTHING + catalog UPDATE），重跑安全。
	{Version: 20261219, Name: "builtin_platform_tools_officecli_reseed", Func: ddlBuiltinPlatformTools},
	// 20261220 knowledge_links_bitemporal: 自治理知识图谱 M1 时序地基——
	// links 加双时态列（valid_from/valid_to + recorded_at）、语义谓词 relation、
	// 浮点权重 weight_f、置信度 confidence；新建 knowledge_access_log（base-level 激活分
	// 与 Hebbian 共激活的检索命中日志）。幂等 IF NOT EXISTS。
	{Version: 20261220, Name: "knowledge_links_bitemporal", SQL: "sql/migrations/20261220_knowledge_links_bitemporal.sql"},
	// 20261221 knowledge_relation_vocab: 自治理知识图谱 M2 语义关系层地基——
	// knowledge_relation_vocab 受控涌现谓词词表（core 硬编码 8 谓词 + candidate LLM 提议），
	// knowledge_relation_state 抽取幂等状态（content_hash 一致跳过，控 LLM 成本）。
	// 种子幂等 ON CONFLICT DO NOTHING，重跑安全。
	{Version: 20261221, Name: "knowledge_relation_vocab", SQL: "sql/migrations/20261221_knowledge_relation_vocab.sql"},
	// 20261222 knowledge_fact_version: 自治理知识图谱 M3 演化时序层地基——
	// knowledge_fact_version supersedes 版本链旧段快照（演化可审计可回滚，不污染 links 表），
	// knowledge_governance_proposal 治理提案（M3.2 矛盾仲裁高风险人工二审；M4 dream_cycle 复用）。
	// 幂等 IF NOT EXISTS，重跑安全。
	{Version: 20261222, Name: "knowledge_fact_version", SQL: "sql/migrations/20261222_knowledge_fact_version.sql"},
	// 20261223 knowledge_stale_mark: 自治理知识图谱 M4 stale 标记落地（深度检查
	// P1-c）——stale 治理任务原只在提案表留痕（status=applied），文档本身无标记，
	// 检索侧无从消费 → 陈旧词条照常满分命中。documents.stale_at 可空列承载标记，
	// 检索三路径统一降权 ×0.5（降权非排除），内容变更清 NULL 复活。
	{Version: 20261223, Name: "knowledge_stale_mark", SQL: "sql/migrations/20261223_knowledge_stale_mark.sql"},
	// 20261224 session_summaries_task_state_json: v4 压缩契约——session_summaries
	// 加 task_state_json 列，承载压缩 LLM 产出的结构化任务状态段（双段化）。
	{Version: 20261224, Name: "session_summaries_task_state_json", SQL: "sql/migrations/20261224_session_summaries_task_state_json.sql"},
	// 20261225 agents_position_variant_partial_unique: BUG-01（2026-08-17 真机 P1）——
	// 原全量唯一索引把无岗位 agent（position_key=''）与软删墓碑行纳入唯一键，
	// 无岗位创建必败、岗位删后无法重建。改部分唯一：仅约束「一岗一变体一在任」。
	{Version: 20261225, Name: "agents_position_variant_partial_unique", Func: ddlAgentsPositionPartialUnique},
	// 20261226 drop_retired_legacy_tables: BUG-02 F4 附带处置——activities/event_store
	// 为已退役死表（fresh install 已 drop；存量库因旧二进制 Ent auto-migrate 复活残留，
	// 现行代码无 Ent schema 无引用）。幂等 DROP IF EXISTS。
	// 注意：sessions_v2 不在此列——实施复核（2026-08-17）确认其为 spirit 会话根实体
	// （session_v2_repo.go 活跃读写、Ent schema 现存），属活跃表，严禁 drop。
	{Version: 20261226, Name: "drop_retired_legacy_tables", Func: ddlDropRetiredLegacyTables},
	// 20261227 tool_grants_expires_at: BUG-MON-B（2026-08-17 真机 P3）——
	// 持久化"始终允许"授权无 TTL（演练残留高危授权永存）。加 expires_at 列 +
	// 存量行按 created_at+72h 回填（多数立即过期，读径惰性清理）；
	// 新授权由 biz GrantTool 写 now+72h，读径过滤过期行。幂等可重跑。
	{Version: 20261227, Name: "tool_grants_expires_at", Func: ddlToolGrantsExpiresAt},
	// 20261228 tools_retry_parallel_default_on: product default flipped to
	// true (selective retry + LLM parallel tools). Ent/frontend defaults
	// already true for new rows; this one-shot UPDATE enables existing
	// agents that still carry the historical false. Idempotent: value-guarded.
	{Version: 20261228, Name: "tools_retry_parallel_default_on", Func: ddlToolsRetryParallelDefaultOn},
	// 20261229 monitor_alert_firing_ms_bigint（2026-08-17 真机 P2）：
	// monitor_alert_rules 三个毫秒时间戳列（last_fired_at/last_fired_window_start/
	// recovered_at）ensure DDL 误建为 INTEGER（int4 放不下 ms epoch ≈1.7e12），
	// MarkAlertFiredPersistent 每次写入 22003 越界静默失败（Warn 级），告警状态机
	// 永不持久化。存量库 ALTER TYPE BIGINT（列值恒 NULL——写入从未成功，转换无损）；
	// fresh 库由 ensure DDL 直接建 BIGINT。SQLite INTEGER 为 64 位不受影响，跳过。
	{Version: 20261229, Name: "monitor_alert_firing_ms_bigint", Func: ddlMonitorAlertFiringMsBigint},

	// agent_runtime_settings.reply_reminder_enabled：代码层（biz.AgentRuntimeSettings +
	// ent schema）已有该字段，但存量库缺列 → 查询组装 settings 时列不存在直接报错/
	// 开关恒 false，reply_reminder 无法按 agent 关闭。补列，默认 1 保持旧行为。
	{Version: 20261230, Name: "agent_runtime_reply_reminder", SQL: "sql/migrations/20261230_agent_runtime_reply_reminder.sql"},
	// 2026-08-19 自我改进排障事故：closed_reason MaxLen(64) 把失败根因（git stderr
	// 摘要）截成 "exit status 255: exit statu..."，详情永久丢失。扩到 512（与 ent
	// schema 同步）；PG varchar 加宽仅目录变更、无损，SQLite 不校验长度直接跳过。
	{Version: 20261231, Name: "si_run_closed_reason_widen", Func: ddlSIRunClosedReasonWiden},
	// 20261232 agent_runtime_token_budget（2026-08-20 token 成本审查）：
	// 方案A 新增 l2_recall_budget_tokens（L2 独立召回预算，此前复用
	// l3_recall_budget_tokens，两块互相挤占）；方案B 新增 l3_inject_provenance
	// （L3 事实 provenance 注入开关，此前硬编码 true 无法关闭省 token）。
	// ent schema 默认值已同步（800 / true）；存量库补列，默认保持旧行为。
	{Version: 20261232, Name: "agent_runtime_token_budget", SQL: "sql/migrations/20261232_agent_runtime_token_budget.sql"},

	// 20261233: cron 深度审查 F1 — 内置任务 model-registry-sync 的 cron_task.id 为 ''
	//（seed 直插 repo 绕过 ID 生成），其 cron_task_run.task_id 亦全为 ''，导致 UI 启停/触发
	// 禁用碰撞、编辑死胡同、执行历史无法按该任务筛选。统一改为显式 ID 并回填运行记录。
	{Version: 20261233, Name: "cron_model_registry_sync_id", SQL: "sql/migrations/20261233_cron_model_registry_sync_id.sql"},

	// 20261234 builtin_platform_tools_config_reseed（TwinMonitor × Aranea Phase B 配置自动化）：
	// twin_config_diff/push/rollback 三工具种子在 20260610 等已应用的存量库中不会插入
	//（同 20261216 twinops 的情形）→ effective keys 查无工具行，工具永不装配。
	// 种子函数幂等（ON CONFLICT DO NOTHING + catalog UPDATE），重跑安全。
	{Version: 20261234, Name: "builtin_platform_tools_config_reseed", Func: ddlBuiltinPlatformTools},
	// 20261235 orchestrations_cancel_reason（2026-08-21 P6 修复）：
	// cancel_reason 的 ALTER 此前嵌在 EnsureOrchestrationSchema（20260713）中，
	// 仅在初次建表时执行；存量 PG 库已应用 20260713 后不会重跑 → 列永久缺失，
	// orchestrator trigger check 持续报 pq: column "cancel_reason" does not exist (42703)。
	// 幂等 IF NOT EXISTS，重跑安全。
	{Version: 20261235, Name: "orchestrations_cancel_reason", SQL: "sql/migrations/20261235_orchestrations_cancel_reason.sql"},
	// 20261236 audit_logs_user_agent（2026-08-21 P6 类审计第二例）：
	// user_agent 的 ALTER 嵌在 sessionMemoryEnsureMonitorSchemaPatches（20260606）中，
	// 存量库已应用 20260606 后不会重跑 → 列永久缺失，InsertAuditLog 每次写入必报
	// pq: column "user_agent" does not exist (42703)。幂等 IF NOT EXISTS，重跑安全。
	{Version: 20261236, Name: "audit_logs_user_agent", SQL: "sql/migrations/20261236_audit_logs_user_agent.sql"},
	// 20261237 remove_workspace_exec_tool（2026-08-21 工具契约评审 S5）：workspace_exec
	// 运行时装配路径从未实现（registry 占位 + prune 强关），目录行「撒谎」。种子已删；
	// 存量库由版本化迁移 20260610 写入的行永不自动消失，需显式 DELETE。幂等，重跑安全。
	{Version: 20261237, Name: "remove_workspace_exec_tool", SQL: "sql/migrations/20261237_remove_workspace_exec_tool.sql"},
	// 20261238 builtin_platform_tools_mcp_labels_reseed（2026-08-21 全链路审查 B2）：
	// mcp_broker 标注「推荐默认」、mcp_tool_set 标注「仅限小工具面」。存量库行由
	// 20260610 等迁移 INSERT（ON CONFLICT DO NOTHING）写入，文案永不刷新 → 需重跑
	// 种子函数驱动 syncBuiltinMCPToolCatalogPatches 刷 display_name/description。
	// 幂等（纯 UPDATE 展示元数据，不动 enabled），重跑安全。
	{Version: 20261238, Name: "builtin_platform_tools_mcp_labels_reseed", Func: ddlBuiltinPlatformTools},
	// 20261239 session_turn_cached_input: session_turns 加 cached_input_tokens 列
	// （turn 级 prompt 缓存命中数，与 input_tokens 同语义）。Ent Schema.Create()
	// 不为存量表 ALTER 补列，存量库必须走此补丁（同 20261236 先例）。幂等。
	{Version: 20261239, Name: "session_turn_cached_input", SQL: "sql/migrations/20261239_session_turn_cached_input.sql"},
	// 20261240 pending_queue_entries: 追问队列落库，启动回放优先于 JSON 快照。
	{Version: 20261240, Name: "pending_queue_entries", SQL: "sql/migrations/20261240_pending_queue_entries.sql"},
	// 20261241 remove_read_image_tool（2026-08-24 agent 配置全面审计 P5）：read_image
	// 无任何 factory/装配实现，enabled=false 且非 opt-in → 全员硬 deny，目录行「撒谎」。
	// 种子行与 toolGroupsMedia 引用已删；存量库行需显式 DELETE（同 20261237 先例）。
	// 幂等，重跑安全。
	{Version: 20261241, Name: "remove_read_image_tool", SQL: "sql/migrations/20261241_remove_read_image_tool.sql"},
	// 20261242 memory_facts_trgm_index: P1-2 GIN trigram index on
	// memory_facts.statement so CJK / short queries enter the L3 candidate
	// pool via word_similarity (FTS 'simple' does not segment CJK).
	// Postgres-only; gated by Func so SQLite CLI/tests skip it.
	{Version: 20261242, Name: "memory_facts_trgm_index", Func: ddlMemoryFactsTrgmIndex},
	// 20261243 builtin_platform_tools_spirit_cleanup_reseed: 精灵工具治理——
	// 新增 get_team_deliverable 目录行（此前仅在 profile/实现层，catalog 缺失导致
	// effective-tools 展示与确认门禁覆盖不到），并软删 6 个已被 plan_and_execute
	// 取代的 legacy spirit 工具（DEAD-3）。种子幂等（ON CONFLICT DO NOTHING +
	// 条件 UPDATE），重跑安全。
	{Version: 20261243, Name: "builtin_platform_tools_spirit_cleanup_reseed", Func: ddlBuiltinPlatformTools},
	// 20261244 agent_runtime_assembly_budget（2026-08-25 包A A1/A4）：装配预算
	// soft/hard 两列。Ent Schema.Create 不为存量表 ALTER 补列（同 20261230/
	// 20261232 先例），INTEGER 兼容双方言。幂等，重跑安全。
	{Version: 20261244, Name: "agent_runtime_assembly_budget", SQL: "sql/migrations/20261244_agent_runtime_assembly_budget.sql"},
	// 20261245 agent_runtime_intent_skip（2026-08-25 包B B1）：简单轮 skip 快路径
	// agent 维度开关。DEFAULT 1 存量零行为变化；管理层 agent SQL 置 0（P-INTENT-SKIP，
	// 宁重勿轻 R4）。INTEGER 兼容双方言，幂等。
	{Version: 20261245, Name: "agent_runtime_intent_skip", SQL: "sql/migrations/20261245_agent_runtime_intent_skip.sql"},
	// 20261246 usage_events_kind_message_index（2026-08-25 79-runtime-governance
	// Phase 0 任务 0.1）：run 级 cache_hit_ratio 读路径按 message_id 查 events，
	// 无索引会 seq scan 持续增长的事件表。双方言通用，幂等。
	{Version: 20261246, Name: "usage_events_kind_message_index", SQL: "sql/migrations/20261246_usage_events_kind_message_index.sql"},
}

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

func ddlSessionMemoryPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return sessionMemoryEnsurePatches(ctx, entClient, d)
}

func ddlMessagesTurnNumber(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return ensureMessagesTurnNumberPatch(ctx, entClient, d, lg)
}

func ddlSessionMemorySchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureSessionMemorySchema(ctx, entClient, d, lg)
}

func ddlMonitorSchemaPatches(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return sessionMemoryEnsureMonitorSchemaPatches(ctx, entClient, d)
}

func ddlBuiltinPlatformTools(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return ensureBuiltinPlatformTools(ctx, entClient, d, lg)
}

func ddlDefaultSystemSetting(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return ensureDefaultSystemSetting(ctx, entClient)
}

func ddlCredentialEncryptionKey(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return ensureDefaultCredentialEncryptionKey(ctx, entClient, d)
}

func ddlEvalSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureEvalSchema(ctx, rawDB, d)
}

func ddlA2ASchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureA2ASchema(ctx, rawDB, d)
}

// ddlSessionRevisionDataMigration backfills session_revision from message counts.
// The ALTER TABLE is handled by the SQL file; this Func only does the data migration.
// Uses dialect-aware WHERE clause: SQLite needs `OR session_revision = ”` for
// type-coerced empty strings; Postgres uses proper bigint type and only needs IS NULL.
func ddlSessionRevisionDataMigration(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	if entClient == nil {
		return nil
	}
	// Skip on fresh databases where the messages table was never created.
	// Migration 20260902 drops the messages subsystem (superseded by activities);
	// on a fresh DB the messages table never exists, so there is nothing to backfill.
	var tableExists bool
	if d.IsPostgres() {
		// to_regclass returns NULL when the table doesn't exist; scan into a
		// nullable string and check for empty/NULL.
		var regclass *string
		err := rawDB.QueryRowContext(ctx, "SELECT to_regclass('public.messages')::text").Scan(&regclass)
		if err != nil || regclass == nil || *regclass == "" {
			lg.Info("ddlSessionRevisionDataMigration: messages table not found, skipping backfill")
			return nil
		}
		tableExists = true
	} else {
		var name string
		err := rawDB.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='messages'").Scan(&name)
		if err != nil {
			lg.Info("ddlSessionRevisionDataMigration: messages table not found, skipping backfill")
			return nil
		}
		tableExists = name == "messages"
	}
	if !tableExists {
		lg.Info("ddlSessionRevisionDataMigration: messages table not found, skipping backfill")
		return nil
	}
	whereClause := "WHERE session_revision IS NULL OR session_revision = ''"
	if d.IsPostgres() {
		whereClause = "WHERE session_revision IS NULL"
	}
	_, err := entClient.ExecContext(ctx, fmt.Sprintf(`
UPDATE sessions SET session_revision = (
  SELECT COUNT(*) FROM messages WHERE messages.session_id = sessions.id AND role = 'user'
) %s`, whereClause))
	return err
}

func ddlPluginRunSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsurePluginRunSchema(ctx, entClient)
}

func ddlFlowLogSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureFlowLogSchema(ctx, entClient)
}

func ddlMessageFTSSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	// Phase 1c-3: messages_fts schema maintenance removed. The FTS5 virtual
	// table and triggers are dropped by migration 20260902_drop_messages_subsystem.sql.
	// This migration entry is retained for gate continuity on existing deployments.
	return nil
}

func ddlChannelInboundSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureChannelInboundSchema(ctx, rawDB)
}

func ddlChannelTurnJobSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureChannelTurnJobSchema(ctx, rawDB)
}

func ddlChannelRuntimeLeaseSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureChannelRuntimeLeaseSchema(ctx, rawDB)
}

func ddlSessionRunSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureSessionRunSchema(ctx, rawDB, lg)
}

func ddlSessionParticipantSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureSessionParticipantSchema(ctx, rawDB, lg)
}

func ddlMonitorAlertSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureMonitorAlertSchema(ctx, entClient, d)
}

func ddlEcosystemSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureEcosystemSchema(ctx, entClient)
}

func ddlTeamGraphSessionSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureTeamGraphSessionSchema(ctx, rawDB)
}

func ddlCompiledTeamSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureCompiledTeamSchema(ctx, rawDB)
}

func ddlSkillEvolutionSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureSkillEvolutionSchema(ctx, entClient)
}

func ddlLearningLoopSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureLearningLoopSchema(ctx, entClient)
}

func ddlTaskPlanSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureTaskPlanSchema(ctx, rawDB, lg)
}

func ddlAllocationPlanSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureAllocationPlanSchema(ctx, rawDB, lg)
}

func ddlAgentPerformanceSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureAgentPerformanceSchema(ctx, rawDB, lg)
}

func ddlOrchestrationSchema(ctx context.Context, rawDB *sql.DB, entClient *ent.Client, d Dialect, lg loggateway.Logger) error {
	return EnsureOrchestrationSchema(ctx, rawDB, d, lg)
}

func ddlEcosystemPresetDataMigration(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, _ loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	// Wrap in transaction for atomicity
	tx, err := rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer tx.Rollback()
	// Migrate agent kind: system -> system_builtin
	if _, err := tx.ExecContext(ctx, `UPDATE agents SET kind = 'system_builtin' WHERE kind = 'system'`); err != nil {
		return fmt.Errorf("migrate agent kind system->system_builtin: %w", err)
	}
	// Migrate agent kind: industry_template -> ecosystem_preset
	if _, err := tx.ExecContext(ctx, `UPDATE agents SET kind = 'ecosystem_preset' WHERE kind = 'industry_template'`); err != nil {
		return fmt.Errorf("migrate agent kind industry_template->ecosystem_preset: %w", err)
	}
	// Migrate team kind: source=imported -> kind=ecosystem_preset
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
	// Wrap in transaction for atomicity
	tx, err := rawDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration tx: %w", err)
	}
	defer tx.Rollback()
	// Agent source migration
	if _, err := tx.ExecContext(ctx, `UPDATE agents SET source = 'system' WHERE kind = 'system_builtin' AND source = 'user'`); err != nil {
		return fmt.Errorf("migrate agent source system_builtin->system: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE agents SET source = 'imported' WHERE kind IN ('ecosystem_preset', 'marketplace', 'certified') AND source = 'user'`); err != nil {
		return fmt.Errorf("migrate agent source ecosystem->imported: %w", err)
	}
	// Team source migration: align with Agent source semantics
	// kind=system_builtin -> source=system
	if _, err := tx.ExecContext(ctx, `UPDATE teams SET source = 'system' WHERE kind = 'system_builtin' AND source = 'user'`); err != nil {
		return fmt.Errorf("migrate team source system_builtin->system: %w", err)
	}
	// kind=ecosystem_preset -> source=imported
	if _, err := tx.ExecContext(ctx, `UPDATE teams SET source = 'imported' WHERE kind = 'ecosystem_preset' AND source = 'user'`); err != nil {
		return fmt.Errorf("migrate team source ecosystem_preset->imported: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit agent source migration: %w", err)
	}

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
		// Non-critical: log the error but don't fail the entire migration
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
	dedupExpr := `COALESCE(json_extract(metadata, '$.pattern_hash'), json_extract(metadata, '$.legacy_type'), '')`
	if d.IsPostgres() {
		dedupExpr = `COALESCE(metadata::jsonb->>'pattern_hash', metadata::jsonb->>'legacy_type', '')`
	}
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
		return false, nil // already present
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
	// Fresh databases never had the legacy L3 table (its Ent schema was removed
	// in the A6 unified-store change); only pre-A6 databases carry it, and only
	// they need the column before convergence (20261111) backfills and drops it.
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
func ddlHealRecordMetadataColumn(ctx context.Context, rawDB *sql.DB, _ *ent.Client, d Dialect, _ loggateway.Logger) error {
	if rawDB == nil {
		return nil
	}
	if _, err := rawDB.ExecContext(ctx, `ALTER TABLE heal_records ADD COLUMN metadata TEXT NOT NULL DEFAULT '{}'`); err != nil && !d.AlreadyExistsErr(err) {
		return fmt.Errorf("add heal_records.metadata: %w", err)
	}
	return nil
}

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
	// Flip false → true for non-A2A agents. A2A proxy agents are identified by
	// config_json containing "a2a_proxy" (agent_kind=a2a_proxy embedded by
	// EmbedAgentKindInConfigJSON). Excluding them preserves their explicit false.
	// Use TRUE/FALSE literals (not 1/0): Ent field.Bool maps to PostgreSQL boolean,
	// which rejects "boolean = integer" with pq: operator does not exist (42883).
	// SQLite 3.23+ also accepts TRUE/FALSE as aliases for 1/0, so this is portable.
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
	// Double parens: Postgres requires index expressions that are not a plain
	// function call (here: COALESCE(...) ->> 'trace_id') to be parenthesized.
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
			continue // 列不存在：由 ensure DDL 以 BIGINT 形态补建
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
		return nil // 列不存在：新库由 ent Schema.Create 以 512 形态建表
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
