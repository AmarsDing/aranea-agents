# 87-orchestration-chain-hardening 开发计划

> 需求：[87-orchestration-chain-hardening.md](./87-orchestration-chain-hardening.md) · 设计：[87-orchestration-chain-hardening.design.md](./87-orchestration-chain-hardening.design.md)
> 审查依据：[docs/reports/2026-10-20-review-spirit-orchestration-chain.md](../reports/2026-10-20-review-spirit-orchestration-chain.md)

## 1. 模块定位

精灵编排链路全段加固：入口分发（取消语义/单跑互斥/synthesis 生命周期）、经验复用缓存契约、DAG 并发与 graph 运行时一致性、前端状态接入与展示。不改变架构边界（ADR-86 双引擎、M67/M78 组织铁律、AS-EVT-01 分级不变）。

## 2. 代码锚点

> **收官状态（2026-10-21）**：30/30 修复已落盘并终验通过，33 项 REQ 全闭环。证据：[终验报告](../testing/reports/report-20261021-060000.md)。动态集成电池：service -race / biz / agent / v2 / tools / server / vitest 530 全绿（除外来车道基线红，见报告 §5）。
> 修验互查中自查自除的实现缺陷 4 个（topologyFromStrategy 类型、signalStartup 取消信号、3 处浮点断言、UpdatedAt 断言粒度），均已修复并复验。

| 域 | 关键文件 |
|----|---------|
| 取消语义 | `internal/service/team_turn_hooks.go`、`spirit_team.go`、`chat_orchestrator_turn_dispatch.go`、`turn_error_publish.go`（新）、`step_failure_classifier.go`、`spirit_team_completion.go` |
| 单跑互斥 | `internal/service/spirit_team.go`、`chat_wire.go`、`chat_orchestrator.go`、`internal/biz/spirit_orchestration_timeout.go`、`internal/biz/team_types.go`、`internal/tools/spirit_tools.go` |
| Synthesis | `internal/biz/spirit_orchestration.go`、`internal/service/plan_executor_dag_publish.go`、`spirit_team_notices.go`、`spirit_team_schedule.go` |
| 事件 | `internal/biz/event.go`、`event_factory.go`、`internal/service/spirit_team_completion.go`、`internal/agent/v2/event_router.go`、`internal/server/ws_v2_wire_convert.go` |
| DAG | `internal/service/plan_executor_dag_flow.go`、`plan_executor_dag_dispatch.go`、`plan_executor_dag_run.go`、`plan_executor.go`、`playbook_confirm.go` |
| Graph | `internal/biz/graph_execution_usecase.go`、`graph.go`（孤儿扫描新文件 `graph_orphan_sweep.go`）、`internal/data/graph.go`、`internal/service/graph_wire.go` |
| 缓存 | `internal/biz/spirit_orchestration_cache.go`、`internal/agent/agent_domain_match.go`、`task_planner_impl.go`、`task_orchestrator_impl.go`、`agent_allocator_match.go`、`internal/data/orchestration_cache_repo.go`、migration `20261287_orchestration_cache_entries.sql`（新） |
| 前端接入 | `web/src/features/chat/composables/useChatRunStatus.ts`、`useChatSender.ts`、`useChatV2EventHandlers.ts`、`useChatEventRouter.ts`、`web/src/stores/chat/activityV2Store.ts`、`web/src/features/chat/v2Types.ts` |
| 前端展示 | `web/src/features/spirit/spiritUi.ts`、`web/src/features/spirit/components/TeamProgressCard.vue`、`web/src/components/chat/AgentSidebarCard.vue`、`observe/ObserveNodeDetail.vue`、`useContextualLoadingMessage.ts`、`observabilityConstants.ts` |

## 3. 现状评估（基线 2026-10-20，修复前）

- **构建**：`go build ./internal/...` ✅ 绿（基线日志 `test/orch-hardening/baseline-build.log`）。
- **测试基线红（预先存在，非本专项范围）**：
  - `internal/biz`：TestSkillSelfEvolution_EndToEnd、TestSkillAutoCreate_FromConversation、TestSkillDedup_ProposalLevel、TestSkillDedup_DetectDuplicateGroups、TestSkillMerge_FuseAndApply、TestMigrateLegacyEmbeddedGraphs_PG_EndToEnd 失败。
  - `internal/data`：包级 FAIL（详见 baseline-tests.log）。
  - `internal/service`：**测试包被其他会话未入库 WIP 阻塞**（`knowledge_asof_test.go` 引用未定义的 ServeSearchAsOf/ServeCollectionGraphAsOf）——本专项测试策略：照常写 `internal/service` 单测，以 `go build ./internal/service` 验证编译，阻塞在报告中标注。
  - `web/src/features/chat/api.ts` 为其他会话脏文件，禁改。
- **脏树禁碰清单**（其他会话 WIP）：`cmd/admin/wire.go`、`cmd/admin/wire_part2.go`、`cmd/admin/workers.go`、`internal/biz/knowledge*`、`internal/service/knowledge_asof.go`（若出现）、`internal/data/knowledge_*`、`internal/knowledge/*`、`web/pnpm-lock.yaml`。

## 4. 批次计划

文件归属已先行消解冲突（spirit_team.go、chat_wire.go、plan_executor_dag_dispatch.go、spirit_team_assembler.go、v2Types.ts/useChatEventRouter.ts）。

### Wave 1（7 agent 并行，文件零重叠）

| 批次 | 内容 | 持有修复 |
|------|------|---------|
| A1 团队生命周期 | `team_turn_hooks.go`、`spirit_team.go`、`chat_orchestrator_turn_dispatch.go`、`turn_error_publish.go`（新）、`spirit_team_handle_result.go`、`spirit_team_schedule.go`、`spirit_team_notices.go`、`internal/biz/spirit_orchestration.go` | D1-F1、D1-F2、D3 全部、C1 三处回调 |
| A2 cancelled 事件后端 | `internal/biz/event.go`、`event_factory.go`、`spirit_team_completion.go`、`internal/agent/v2/event_router.go`、`internal/server/ws_v2_wire_convert.go` | D4 后端（C5） |
| A3 graph 一致性 | `internal/biz/graph_execution_usecase.go`、`graph_orphan_sweep.go`（新）、`internal/data/graph.go`、`internal/service/graph_wire.go` | D6 全部（C16/C17/P4） |
| A4 经验缓存（除 assembler 属 B2） | `spirit_orchestration_cache.go`、`agent_domain_match.go`、`task_planner_impl.go`、`task_orchestrator_impl.go`、`agent_allocator_match.go`、`orchestration_cache_repo.go`、新迁移、`ddl_migration_table_late.go`、`biz/spirit_orchestration_progress.go`、`task_planner_decompose_llm.go` | D7 全部（C8–C14） |
| A5 互斥配额看守修补 | `internal/biz/spirit_orchestration_timeout.go`、`internal/biz/team_types.go`、`internal/tools/spirit_tools.go` | P2、P6、C7 占座口径部分 |
| F1 前端接入 | `useChatRunStatus.ts`、`useChatSender.ts`、`useChatV2EventHandlers.ts`、`useChatEventRouter.ts`、`activityV2Store.ts`、`v2Types.ts`、`teamRunEventFromV2Event.ts`、`eventView.ts`、`tracesQuery.ts`、`timeouts.ts` | D9 全 + D4 前端（C5-FE、C19–C22） |
| F2 前端展示 | `spiritUi.ts`、`TeamProgressCard.vue`、`AgentSidebarCard.vue`、`ObserveNodeDetail.vue`、`useContextualLoadingMessage.ts`、`observabilityConstants.ts` + i18n | D10 全（C23–C25、P7、P8） |

### Wave 2（Wave 1 完成后，2 agent 并行）

| 批次 | 内容 | 持有修复 |
|------|------|---------|
| B1 DAG 并发 | `plan_executor_dag_flow.go`、`plan_executor_dag_dispatch.go`、`plan_executor_dag_run.go`、`plan_executor.go`、`playbook_confirm.go`、`chat_wire.go` | D5 全（C15/C7 扇出/C18）+ D1-F3 分类器 |
| B2 组队归一 | `spirit_assembly.go`、`spirit_assembly_part2.go`、`spirit_team_assembler.go` | D8 全（C6、P3） |

> D1-F3 经分析挪入 B1：其唯一消费点在 `plan_executor_dag_dispatch.go`，与 D5 原子化改动同文件，避免 Wave1 冲突。
> A1 与 B2 都碰 `spirit_team_assembler.go`？否：C12 notice 相关修复（废弃死代码）归 B2/D8；A4 只读 assembler 不修改。

## 5. 任务清单（30 项）

| # | 修复 | REQ | 批次 | 状态 |
|---|------|-----|------|------|
| 1 | D1-F1 统一取消判别 `isUserCancelledErr` + 三处回调接线 | REQ-01 | A1 | ✅ |
| 2 | D1-F2 终态守卫补 partial_failure；completion 记账 CAS 门控 | REQ-05 | A1 | ✅ |
| 3 | D1-F3 失败分类器 terminal-cancel | REQ-02 | B1 | ✅ |
| 4 | D2-1 StartTeamTurn 运行注册+会话锁+互可见 | REQ-09 | A1（服务侧） | ✅ |
| 5 | D2-2 TransitionStatus(Running) 失败即中止 | REQ-10 | A1 | ✅ |
| 6 | D3-1 poller 扫 interrupted 会话复活 synthesis | REQ-11 | A1 | ✅ |
| 7 | D3-2 lease 释放注入 RootTaskActivityID | REQ-08 | A1 | ✅ |
| 8 | D3-3 synthesisAlreadyProduced fail-closed + 取消跳过总结 | REQ-04/REQ-12 | A1 | ✅ |
| 9 | D4-1 新增 EventKindTeamRunCancelled（教义+工厂+生产点） | REQ-07 | A2 | ✅ |
| 10 | D4-2 cancelled 事件 v2 路由 + ws 转换 | REQ-07 | A2 | ✅ |
| 11 | D4-3 cancelled 事件前端路由/视图映射 | REQ-07 | F1 | ✅ |
| 12 | D5-1 check-and-dispatch 原子化 | REQ-20 | B1 | ✅ |
| 13 | D5-2 r.canceled 持锁写 | REQ-21 | B1 | ✅ |
| 14 | D5-3 DAG 扇出配额闸门 | REQ-22 | B1 | ✅ |
| 15 | D6-1 interrupt≠done（onComplete 终结语义） | REQ-23 | A3 | ✅ |
| 16 | D6-2 streamGen 守卫对称（GraphDone/NodeStart/NodeEnd） | REQ-24 | A3 | ✅ |
| 17 | D6-3 终态先落库后外发 + 有界重试 | REQ-25 | A3 | ✅ |
| 18 | D6-4 standalone 启动终态化扫描（graph_orphan_sweep） | REQ-25 | A3 | ✅ |
| 19 | D7-1 DQ 失败降权 + synthesis 失败学习 | REQ-13 | A4 | ✅ |
| 20 | D7-2 指纹/PlaybookID 写入+全消费点 fail-closed 校验 | REQ-14 | A4 | ✅ |
| 21 | D7-3 读侧归一化 + 「其他」域禁读写 | REQ-15 | A4 | ✅ |
| 22 | D7-4 缓存分行持久化 + 版本 CAS + 迁移 | REQ-16 | A4 | ✅ |
| 23 | D7-5 缓存命中可见性信号 | REQ-17 | A4 | ✅ |
| 24 | D7-6 成员收缩有效 DQ + 确定性排序 | REQ-18 | A4 | ✅ |
| 25 | D7-7 配方 workspace 隔离 | REQ-19 | A4 | ✅ |
| 26 | D8-1 mode 归一化后移至模板路由之后 | REQ-C6 | B2 | ✅ |
| 27 | D8-2 单成员模板不授 RoleSynthesizer | REQ-C6 | B2 | ✅ |
| 28 | D8-3 删除 AutoStart 死路径（含 C12 死 notice） | REQ-P3 | B2 | ✅ |
| 29 | D9 前端接入四项（新鲜度守卫/MemberSession 补丁/GraphNode 终态粘性/节流告警）+ P6 重臂 | REQ-26~29 | F1 + A5 | ✅ |
| 30 | D10 前端展示五项（v2 词表/侧栏卡分支/Observe 文案/加载清除/PULSE 键） | REQ-30~33 | F2 | ✅ |

## 6. 改动文件清单

### 6.1 新增文件

| 文件 | 归属 |
|------|------|
| `internal/service/turn_error_publish.go` | A1（取消判别 helper + 终态落账 helper） |
| `internal/biz/graph_orphan_sweep.go` | A3 |
| `internal/data/sql/migrations/20261287_orchestration_cache_entries.sql` | A4 |

### 6.2 修改文件（按批次）

- **A1**：`internal/service/team_turn_hooks.go`、`spirit_team.go`、`chat_orchestrator_turn_dispatch.go`、`spirit_team_handle_result.go`、`spirit_team_schedule.go`、`spirit_team_notices.go`、`internal/biz/spirit_orchestration.go` + 各自 `_test.go`
- **A2**：`internal/biz/event.go`、`internal/biz/event_factory.go`、`internal/service/spirit_team_completion.go`、`internal/agent/v2/event_router.go`、`internal/server/ws_v2_wire_convert.go` + 测试
- **A3**：`internal/biz/graph_execution_usecase.go`、`internal/data/graph.go`、`internal/service/graph_wire.go` + 测试
- **A4**：`internal/biz/spirit_orchestration_cache.go`、`internal/agent/agent_domain_match.go`、`internal/agent/task_planner_impl.go`、`internal/agent/task_orchestrator_impl.go`、`internal/agent/agent_allocator_match.go`、`internal/data/orchestration_cache_repo.go`、`internal/data/ddl_migration_table_late.go`、`internal/biz/spirit_orchestration_progress.go`、`internal/agent/task_planner_decompose_llm.go` + 测试
- **A5**：`internal/biz/spirit_orchestration_timeout.go`、`internal/biz/team_types.go`、`internal/tools/spirit_tools.go` + 测试
- **F1**：`web/src/features/chat/composables/{useChatRunStatus,useChatSender,useChatV2EventHandlers,useChatEventRouter,timeouts}.ts`、`web/src/stores/chat/activityV2Store.ts`、`web/src/features/chat/{v2Types,teamRunEventFromV2Event,eventView,tracesQuery}.ts` + vitest
- **F2**：`web/src/features/spirit/spiritUi.ts`、`web/src/components/spirit/TeamProgressCard.vue`、`web/src/components/chat/AgentSidebarCard.vue`、`web/src/components/chat/observe/ObserveNodeDetail.vue`、`web/src/features/chat/composables/useContextualLoadingMessage.ts`、`web/src/features/chat/observability/observabilityConstants.ts` + i18n lang 文件
- **B1**：`internal/service/{plan_executor_dag_flow,plan_executor_dag_dispatch,plan_executor_dag_run,plan_executor,step_failure_classifier,playbook_confirm}.go`、`internal/service/chat_wire.go` + 测试
- **B2**：`internal/biz/spirit_assembly.go`、`internal/biz/spirit_assembly_part2.go`、`internal/service/spirit_team_assembler.go` + 测试

### 6.3 DI 变更

全部经 `internal/service/chat_wire.go` / `graph_wire.go` 手工透传（B1/A3/局部），**不 regen `cmd/admin/wire_gen.go`**（手工并合约定，wire_gen 三注入存活为验收点）。

## 7. 验收标准

| AC | 标准 |
|----|------|
| AC-1 | `go build ./internal/... ./cmd/admin` 绿，wire_gen 三注入存活（熔断表/Outbox/巡检） |
| AC-2 | 新增单测全绿：`internal/service`（如解阻）、`internal/biz`、`internal/agent`、`internal/data`（相关域）、`internal/tools` |
| AC-3 | 关键并发用例 `-race` 通过（D5-1/D5-2/D2-1） |
| AC-4 | `cd web && pnpm vitest run` 受影响 spec 全绿，无 console.log（lint 禁线） |
| AC-5 | 终审重跑：沿「精灵会话→任务分配→组队→经验复用→graph→状态上报→UI」逐条验证 33 项全部关闭，产出 `docs/testing/reports/report-YYYYMMDD-HHMMSS.md` |
| AC-6 | 基线红不外溢：§3 所列预先存在失败保持失败且不新增失败 |
| AC-7 | DOC-SYNC：65 交叉参考/78 org 编排/87 三件套状态回写真实进度 |

## 8. 风险与回滚

| 风险 | 缓解 |
|------|------|
| 缓存分行迁移丢数据 | 迁移一次性导入旧单行 blob；回滚=恢复旧行为读取（代码保留旧 blob 读路径至确认完成） |
| cancelled 终态变更波及回放 | 新 kind 纯增量，旧 FailedEvent 回放路径保留 |
| chat_wire.go 手工透传编译错 | 每批后立即 `go build ./internal/service` 验证 |
| 其他会话并发改同文件 | 禁碰清单 §3；A/B/F 文件零重叠；冲突即停批上报 |
