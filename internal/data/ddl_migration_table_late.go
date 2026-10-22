package data

// ddlMigrationsLate is the second half of the DDL migration registry table.
var ddlMigrationsLate = []ddlMigration{
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
	// 20261247 builtin_platform_tools_sandbox_fs_reseed（M82 P1-2）：种子表新增
	// sandbox_fs_write / sandbox_fs_read 两行，存量库不会重跑版本化种子迁移
	// 20260610，照 20261216/20261243 先例以 reseed 迁移补齐（种子幂等
	// ON CONFLICT DO NOTHING，重跑安全）。
	{Version: 20261247, Name: "builtin_platform_tools_sandbox_fs_reseed", Func: ddlBuiltinPlatformTools},
	// 20261248 sessions_fork_from_turn（79-runtime-governance R6 Phase 4.1）：
	// 会话 Fork-from-Turn 血缘列——parent_session_id 记来源会话（既有），
	// fork_from_turn_id 记分叉点 turn id（v2 turn id / invocation id）。
	// 双方言通用，幂等（DB-N6 duplicate column 视为成功）。
	// 注：本注册曾在 48ddb3001 提交重写时被误删、2026-08-27 二轮审查恢复；
	// 迁移 runner 逐版本独立判 applied，存量库下次启动自动补应用。
	{Version: 20261248, Name: "sessions_fork_from_turn", SQL: "sql/migrations/20261248_sessions_fork_from_turn.sql"},
	// 20261249 memory_fact_pending（79-runtime-governance R3）：记忆高风险写
	// 人工审批层暂存表——自动管线 UPDATE / DELETE / contested 判决落 pending，
	// 审批通过执行原 bi-temporal 写，拒绝留痕。原分配 20261246 已被 0.1 索引
	// 占用，顺延 20261249（65 交叉参考号段统一分配）。双方言通用，幂等。
	{Version: 20261249, Name: "memory_fact_pending", SQL: "sql/migrations/20261249_memory_fact_pending.sql"},
	// 20261250/51/52 decision_records（M80 决策智能 Phase 1）：统一决策记录层——
	// 五类高价值决策（HITL/planner/系统闸/知识仲裁/进化应用）归一为可查询、
	// 可审计、可追溯的一等资产。20261250 主表（单父链 parent_decision_id，
	// ADR-3；JSON 列双方言 TEXT 惯例）；20261251 异步落库 outbox（E5 新表，
	// 模式复用 event_delivery_outbox 但不共用）；20261252 查询索引（常规索引
	// 双方言 SQL 先行，PG-only GIN 表达式索引由 Func 门控补建，SQLite 跳过）。
	// 号段经 2026-08-25 三方案统一分配，20261253-59 预留缓冲。幂等，重跑安全。
	{Version: 20261250, Name: "decision_records", SQL: "sql/migrations/20261250_decision_records.sql"},
	{Version: 20261251, Name: "decision_record_outbox", SQL: "sql/migrations/20261251_decision_record_outbox.sql"},
	{Version: 20261252, Name: "decision_records_idx", SQL: "sql/migrations/20261252_decision_records_idx.sql", Func: ddlDecisionRecordsGINIndexes},
	// 20261254 decision_records_runid_idx（M80 1.10 压测调优）：source_run_id
	// 等值过滤的 btree 表达式索引（GIN 只服务 @>，实测 1M 行全表扫 P95≈125ms
	// 虽过门禁但 10 倍量即破；补索引后 <5ms）。PG-only，号段取预留缓冲。
	{Version: 20261254, Name: "decision_records_runid_idx", Func: ddlDecisionRecordsRunIDIndex},
	// 20261255 memory_fact_pending_payload（79-runtime-governance R3 Phase 3.3）：
	// pending 表加 payload_json 存扣留时刻的 FactWriteDecision 快照——approve 回写后
	// 忠实重放原 bi-temporal 写（候选全字段元数据不丢失）。号段取 20261253-59
	// 预留缓冲。双方言通用，幂等（duplicate column 视为成功）。
	{Version: 20261255, Name: "memory_fact_pending_payload", SQL: "sql/migrations/20261255_memory_fact_pending_payload.sql"},
	// 20261256 memory_fact_allow_rules（79-runtime-governance R3 Phase 3.4，E4）：
	// approve_always 以 (agent_id, verdict) 维度持久化 allow 规则，命中后同 agent
	// 同类 verdict 免审直写（仍记 audit）。号段取 20261253-59 预留缓冲。双方言
	// 通用，幂等（IF NOT EXISTS + 唯一索引防重）。
	{Version: 20261256, Name: "memory_fact_allow_rules", SQL: "sql/migrations/20261256_memory_fact_allow_rules.sql"},
	// 20261260 config_graph（M81 配置资产图谱 P0-0.1）：配置资产依赖图
	// config_graph_nodes（12 类资产节点）+ config_graph_edges（27 类引用边），
	// generation 双代切换支撑全量重建无清表窗口。双方言通用，幂等。
	{Version: 20261260, Name: "config_graph", SQL: "sql/migrations/20261260_config_graph.sql"},
	// 20261261 config_graph_composite_pk（M81 P1-3 回放实测 bugfix）：20261260 的
	// PRIMARY KEY(id) 与「双代共存」设计冲突——同 id 跨代插入必现 23505，第二次
	// 全量重建永远失败。主键改 (id, generation)。PG：DROP+ADD 幂等；SQLite：整表
	// 重建。双方言，幂等。
	// 注：本注册曾在 48ddb3001 提交重写时被误删、2026-08-27 二轮审查恢复；
	// 未应用过的存量库下次启动自动补应用（Func 体全程幂等）。
	{Version: 20261261, Name: "config_graph_composite_pk", Func: ddlConfigGraphCompositePK},
	// 20261262/20261263 tool_param_rules（79-runtime-governance R9 Phase 5.4/5.5）：
	// 工具参数模式权限表 + 首批 builtin 种子（gns3_exec 白名单+兜底 ask；
	// exec_command 危险模式 deny）。双方言通用，幂等（IF NOT EXISTS +
	// INSERT OR IGNORE）。**2026-08-27 审计补注册**：两文件此前已存在但漏登
	// registry，已部署库的表从未创建（单测直接执行 SQL 文件绕过 registry
	// 所以全绿）——任何新迁移文件必须同步注册本表。
	{Version: 20261262, Name: "tool_param_rules", SQL: "sql/migrations/20261262_tool_param_rules.sql"},
	{Version: 20261263, Name: "tool_param_rules_seed", SQL: "sql/migrations/20261263_tool_param_rules_seed.sql"},
	// 20261264 tool_param_rules_reseed_regex（R9 P5.4 审计 P5）：builtin deny
	// 规则从整串锚定 glob 升级为正则子串语义——旧 glob 对多余空白/命令包装
	// （sudo、sh -c、绝对路径）敏感易绕过。UPDATE 幂等，仅命中旧 glob 文本的行。
	{Version: 20261264, Name: "tool_param_rules_reseed_regex", SQL: "sql/migrations/20261264_tool_param_rules_reseed_regex.sql"},
	// 20261265 tool_param_rules_reseed_harden（2026-08-27 二轮审查 H4）：builtin
	// deny 分隔符类补 ( $ 反引号 + rm flags 归一（命令替换/子 shell/flags 变形
	// 绕过修复）；gns3 allow glob 改单行锚定 regex（glob 跨换行后的多行注入
	// 修复）。UPDATE WHERE 旧 pattern 幂等，不回冲管理员自定义。
	{Version: 20261265, Name: "tool_param_rules_reseed_harden", SQL: "sql/migrations/20261265_tool_param_rules_reseed_harden.sql"},
	// 20261266 agent_runtime_loop_guard_gates（2026-08-27 二轮 Q1，S02「合法失控」
	// 根修）：agent_runtime_settings 补 loop_guard_tool_load_max / loop_guard_wall_soft_sec
	// / loop_guard_wall_hard_sec 三列（0=内置默认，>0 覆盖），PolicyResolver 每调用读取。
	{Version: 20261266, Name: "agent_runtime_loop_guard_gates", SQL: "sql/migrations/20261266_agent_runtime_loop_guard_gates.sql"},
	// 20261267 tool_param_rules_reseed_rmflags（2026-08-27 三轮审查）：rm deny
	// flags 全覆盖——--no-preserve-root/长选项 --recursive/粘连字母 -rfv/--
	// 分隔变形此前全部绕过；顺手修 gns3 allow 三行 created_at=0 占位。
	// UPDATE WHERE 旧 pattern 幂等，不回冲管理员自定义。
	{Version: 20261267, Name: "tool_param_rules_reseed_rmflags", SQL: "sql/migrations/20261267_tool_param_rules_reseed_rmflags.sql"},
	// 20261268 decision_records_sessionid_idx（T5 chat 侧闸事件聚合面）：
	// source_session_id 过滤 / SessionGateStats 聚合的会话归属表达式索引，
	// 新记录 SourceRef.SessionID 一等公民、旧记录仅 metadata.session_id，
	// COALESCE 两路兼容；表达式与 gateSessionExpr 生成串精确一致。
	{Version: 20261268, Name: "decision_records_sessionid_idx", Func: ddlDecisionRecordsSessionIDIndex},
	// 20261269 runtime_cost_defaults（Wave 2）：ReplyReminder 存量 DEFAULT 1
	// 回填 0（记忆管家除外）；L3 provenance 非 __memory__ 回填关。幂等 UPDATE。
	{Version: 20261269, Name: "runtime_cost_defaults", SQL: "sql/migrations/20261269_runtime_cost_defaults.sql"},
	// 20261270 intent_skip_governance（P0 档位）：Spirit + GM + dept_lead 关闭
	// simple-turn skip。存量 DEFAULT 1 导致管理层任务被 confident_simple 短路
	// （S06/S08）。幂等 UPDATE。
	{Version: 20261270, Name: "intent_skip_governance", SQL: "sql/migrations/20261270_intent_skip_governance.sql"},
	// 20261271 usage_wait_ms_columns（P2）：HITL wait 从 metadata_json 提升为
	// 独立列，便于 SQL 分析 latency 不含确认等待。幂等 ADD COLUMN + JSON 回填。
	{Version: 20261271, Name: "usage_wait_ms_columns", Func: ddlUsageWaitMSColumns},
	// 20261272 knowledge_chunks_embedding_hnsw（P2）：知识检索 ANN 从 ivfflat
	// 切 HNSW。存量 DROP ivfflat；fresh 库由 EnsureKnowledgeSchema 直接建 HNSW。
	{Version: 20261272, Name: "knowledge_chunks_embedding_hnsw", Func: ddlKnowledgeChunksEmbeddingHNSW},
	// 20261273 turns_v2_seq_per_session（R4-Q3）：turns_v2.seq 重排为
	// per-session 单调计数（此前与 step 共享全局计数器，同会话 seq 呈
	// 1→23→428）。按 (session_id, started_at, id) ROW_NUMBER 重排，幂等。
	{Version: 20261273, Name: "turns_v2_seq_per_session", SQL: "sql/migrations/20261273_turns_v2_seq_per_session.sql"},
	// 20261274 team_runs_v2_bridge（2026-09-01 取证断裂修复）：team_runs 添加
	// team_run_v2_id 桥接列，使 v1 team_run_steps.run_id 可与 v2 team_runs_v2.id
	// 按 team_run_v2_id 精确 join，取代时间窗对齐。
	{Version: 20261274, Name: "team_runs_v2_bridge", SQL: "sql/migrations/20261274_team_runs_v2_bridge.sql"},
	// 20261276 steps_v2_updated_at（P4-1 行级新鲜度）：steps_v2 添加
	// updated_at（ent UpdateDefault 每次写入自动刷新），idle 探测信号升级为
	// GREATEST(started_at, updated_at)。
	{Version: 20261276, Name: "steps_v2_updated_at", SQL: "sql/migrations/20261276_steps_v2_updated_at.sql"},
	// 20261277 team_runs_v2_heartbeat（P2-1 持久化心跳）：runner 流式节流写
	// heartbeat_at；idle 探测取 max(steps.started_at, heartbeat_at)，根治长
	// 流式生成误判 idle（P3-C）。原编号 20261275 与 SeedPackTwinMonitorV1
	// 撞号（schema_migrations 版本全局唯一），重编号至 20261277。
	{Version: 20261277, Name: "team_runs_v2_heartbeat", SQL: "sql/migrations/20261277_team_runs_v2_heartbeat.sql"},
	{Version: 20261278, Name: "chat_turn_submit_claim", Func: ddlChatTurnSubmitClaimSchema},
	// 20261280 knowledge_writeback_outbox（G-10，2026-10-19）：AutoMemory 写回
	// 团队库失败仅 Warn 即永久丢失（违反 AS-EVT-01 Important 分级）。写回批次
	// 重试队列——失败落 payload，knowledge_writeback worker 周期重放，触达
	// attempts 上限翻 dead；batch_key 幂等键（模式复用 decision_record_outbox）。
	{Version: 20261280, Name: "knowledge_writeback_outbox", SQL: "sql/migrations/20261280_knowledge_writeback_outbox.sql"},
	// 20261282 knowledge_access_log_columns（G-15，2026-10-19）：access_log 此前
	// 只落 collection/doc/query_hash，session_id 列建了从不写。补 user/agent/
	// rank/score/latency 列并接通写入（检索命中路径透传请求上下文），为「我的
	// 足迹」与审计查询建 (user_id, accessed_at DESC) 索引。20261281 为
	// twinmonitor 种子重编号（schema_migrations 版本全局唯一）。
	{Version: 20261282, Name: "knowledge_access_log_columns", SQL: "sql/migrations/20261282_knowledge_access_log_columns.sql"},
	// 20261283 knowledge_parent_child（G-06，2026-10-20）：knowledge_chunks 增
	// parent_chunk_id/chunk_role（父块语境行不入召回，部分索引支撑回填批量取）；
	// knowledge_documents 落库摄取生效的分块三元组（reembed 保形）。父子一致性由
	// 应用层 CommitIndexedDocument 单事务保证，不加自引用 FK。
	{Version: 20261283, Name: "knowledge_parent_child", SQL: "sql/migrations/20261283_knowledge_parent_child.sql"},
	// 20261284 knowledge_shared_visibility（G-08，2026-10-20）：knowledge_documents
	// 增 shared_with TEXT[]（visibility='shared' 的 org 部门/岗位名单）+ GIN
	// 重叠索引（谓词 `shared_with && $n`）。TEXT[] 为 knowledge 域数组先例
	// （heading_path），JSON-in-TEXT 双方言惯例不适用于 PG-only 域。
	{Version: 20261284, Name: "knowledge_shared_visibility", SQL: "sql/migrations/20261284_knowledge_shared_visibility.sql"},
	// 20261285 tool_invocations_workspace（A2，tools 端到端审计跟进）：tool_invocations
	// 加 workspace_id 列 + (workspace_id, started_at) 索引 + 自 agents 表按 agent_id
	// backfill；宿主缺失行保持 ''（仅系统调用方在 KPI 聚合可见）。Postgres-only
	// （SQLite/测试由 Ent schema 建列）；不加 RLS（本表租户边界一直在 service 层）。
	// dev 分支原编号 20261279 与 origin/dev 合并重排的 skill_invocation_analysis_retry
	//（20261278→20261279，见 ddlMigrationsMergeAuditFixes）撞号，重编号至 20261285
	// （20261280-20261284 已为本批 knowledge/审计条目占用，20261281 为 twinmonitor
	// 种子）；SQL 文件保留原磁盘文件名——版本唯一性由注册表 Version 字段保证。
	{Version: 20261285, Name: "tool_invocations_workspace", Func: ddlToolInvocationsWorkspace},
	// 20261286 knowledge_graph_embeddings（G-33，2026-10-20，ADR-KN-20261020-H）：
	// 图谱卡片独立向量档——词条文（kind='entity'，每实体一卡）与谓词文（kind='edge'，
	// 每 active semantic 边一卡）注入 RRF 第三腿。ref_id 不建 FK（kind 复用实体/链接
	// 两宿主表，孤儿行由 GraphProjectionWorker 墓碑回收）；可见性不落列，读路径经
	// 宿主文档投影下推 G-08 中央谓词。迁移不带 embedding 列（迁移轨道免 pgvector
	// 依赖先例）：存量库由 EnsureKnowledgeSchema ALTER 补列 + HNSW，维度与
	// knowledge_chunks 同源由 reconcileEmbeddingDim 对账。原拟 20261285 与
	// tool_invocations_workspace 撞号（合并重排后的注册版本唯一）→ 重编号 20261286。
	{Version: 20261286, Name: "knowledge_graph_embeddings", SQL: "sql/migrations/20261286_knowledge_graph_embeddings.sql"},
	// 20261287 knowledge_links_recorded_idx（G-05-asof，2026-10-20，ADR-KN-20261020-J，
	// 索引维度复审 F1[major]）：R 轴 as-of 读索引——ListCollectionLinksAsOf 的
	// R-only/双轴谓词 recorded_at <= $R 原有零索引，valid_to > $R 分支与 GiST 表达式
	// 索引凑不起 BitmapOr → 全库 as-of 图读退化全表 seq scan，且双时态表历史行只增
	// 不删代价线性膨胀。(collection_id, recorded_at) 组合序：collection_id 前置等值
	// + recorded_at 驱动范围，单索引同服务 R-only 与双轴两条 SQL 变体。
	{Version: 20261287, Name: "knowledge_links_recorded_idx", SQL: "sql/migrations/20261287_knowledge_links_recorded_idx.sql"},
	// 20261288 orchestration_cache_entries（D7-F5，2026-10-21）：编排配方缓存
	// 逐行持久化表（PK(workspace_id, task_pattern)，D7-F6 租户分桶）。legacy
	// system_settings 整包 JSON 由应用层 InitFromRepo 一次性逐条导入本表。
	// 原拟 20261287 与 knowledge_links_recorded_idx 撞号 → 重编号 20261288；
	// SQL 文件 2026-10-21 改 canonical 名（migration_version_guard_test 护栏）——
	// schema_migrations 版本唯一性本由注册表 Version 保证，文件名绑定经护栏转正。
	{Version: 20261288, Name: "orchestration_cache_entries", SQL: "sql/migrations/20261288_orchestration_cache_entries.sql"},
	// 20261289 coding_liveness（M4，2026-10-21，ADR-88）：coding_tasks 增 child_pid
	// （进程组根 PID，重启收割 taskkill /T /F）＋ liveness 四字段；coding_agents 增
	// liveness_override（按 key 覆盖阈值，JSON 文本）。存量行 last_active_at 回填
	// updated_at 基线（76 设计「活跃性监管」§B）。两表均为 Ent Schema.Create 管理
	// 的新表，fresh 建表自带全列，本迁移只服务存量库（重复列错误按 DB-N6 视为成功）。
	{Version: 20261289, Name: "coding_liveness", SQL: "sql/migrations/20261289_coding_liveness.sql"},
	// 20261290 chunk_embeddings（G-39，2026-10-21，ADR-KN-20261021-G）：多维度
	// 向量共存侧表——chunk 向量从 knowledge_chunks.embedding（typmod 固化
	// vector(N)，换模型须 ALTER 全表 + reconcileEmbeddingDim 置 NULL 作废，
	// 08-10 事故根因）迁至按 (chunk_id, dim) 分列的 chunk_embeddings；
	// embedding 列裸型 vector（typmod 无关先决，per-dim partial HNSW 由
	// EnsureKnowledgeSchema 注册），reconcileEmbeddingDim 随本轮原子退役。
	// G-33 卡片档（20261286 同源账，ADR-KN-20261020-H）同轮补 dim 列，
	// 裸型化/per-dim 索引与 backfill（JOIN collections 账本 dim，ON CONFLICT
	// 幂等、崩溃可断点续补）走 Ensure 双轨。迁移轨道免 pgvector 依赖先例
	//（20261286）：本 .sql 不建 vector 列、不建 HNSW。编号紧跟 20261289。
	{Version: 20261290, Name: "chunk_embeddings", SQL: "sql/migrations/20261290_chunk_embeddings.sql"},
	// 20261291 session_approval_policies（P0-5 审批工程化，批 A11，2026-10-22）：
	// 会话/Agent 作用域的确定性审批姿态表（ask|never|trusted × tool_pattern ×
	// command_prefix × args_pattern × priority × shadow_only × expires_at），
	// 消费链 BeforeTool 审批策略 gate（paramRuleGate 之后、循环守卫之前）。
	// 仅建表：不加 ent schema、不播种内置规则；幂等，重跑安全。编号紧跟 20261290。
	{Version: 20261291, Name: "session_approval_policies", SQL: "sql/migrations/20261291_session_approval_policies.sql"},
	// 20261292 memory_facts_cjk_bigram_fts（AMC C3-B 召回基建，2026-10-20 trgm
	// 风暴治本配套——编号顺延 20261291+1）：
	// bigram_text() IMMUTABLE STRICT SQL 函数（连续 CJK 段滑窗二元组 +
	// ASCII 段小写原词）+ 对该表达式的函数式 GIN 索引，L3 召回新增 CJK
	// 全文通道——FTS 'simple' 不分词中文整段成单 lexeme，原 CJK 召回仅靠
	// 20261242 trgm 单通道（其全表 GIN 候选扫描即 10-20 事故根因）。
	// AMC C3 修复 #9：弃用 STORED 生成列（PG≤17 全表重写 + 启动迁移窗口
	// 阻塞写），改函数式 GIN（零 heap 重写）；查询侧写同构表达式命中索引。
	// 幂等（CREATE OR REPLACE FUNCTION / IF NOT EXISTS），存量库重跑安全。
	{Version: 20261292, Name: "memory_facts_cjk_bigram_fts", SQL: "sql/migrations/20261292_memory_facts_cjk_bigram_fts.sql"},
	// 20261293 memory_facts_embedding_due_idx（AMC C3 修复 #7）：
	// MemoryFactIndexSweeper 每 30s 按 (embedding_status IN stale/pending/failed,
	// status='active', deleted_at=空串, attempts 上限+退避) ORDER BY updated_at
	// 拾取行批——无支撑索引时 38 万行/8.2GB 表每 30s 全表排序扫描并挤压
	// 评测期 32 并发搜索。部分索引只覆盖 due 候选谓词，体积极小写放大趋零。
	{Version: 20261293, Name: "memory_facts_embedding_due_idx", SQL: "sql/migrations/20261293_memory_facts_embedding_due_idx.sql"},
	// 20261294 document_versions_chunk_archive（G-32 内容版本链+增量重建基座，
	// 2026-10-22 拍板，ADR-KN-20261022-I）：document_versions 轻版本账本
	//（seed|create|edit|rename|supersede|tombstone）+ chunk_archive（删块同事务
	// 物化归档，无 FK，cited_count 快照随行）+ chunk_embeddings_archive（死化身
	// 向量镜档，与 20261290 同型——embedding 列与 per-dim partial HNSW 由
	// EnsureKnowledgeSchema 双轨补齐，迁移轨道免 pgvector 依赖先例）+
	// knowledge_chunks.content_key 稳定键列（'sha1:'+hex 口径登记）+
	// (doc_id, content_key) 索引 + knowledge_chunk_incarnations 化身区间视图
	//（G-05 as-of 向 chunk 推广的全量时态读基座）。种子=迁移内一次性纯 SQL
	//（content_hash=空串 未知档+asset_hash=正文列值），Ensure 不挂种子循环；
	// 存量 content_key 与文本档版本行 body-hash 富化由 Go 启动回填 runner
	// 幂等分批跑。初稿占用 20261292，当日 C3 连占 20261292/20261293
	//（ee29fca85）→ 顺延（canonical 铁律：SQL 文件名/注册行/设计引用同改
	// 同 commit）；TK-G39-01 DROP 顺延 ≥20261295。
	{Version: 20261294, Name: "document_versions_chunk_archive", SQL: "sql/migrations/20261294_document_versions_chunk_archive.sql"},
}
