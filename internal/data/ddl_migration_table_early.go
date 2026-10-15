package data

// ddlMigrationsEarly is the first half of the DDL migration registry table.
var ddlMigrationsEarly = []ddlMigration{
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
}
