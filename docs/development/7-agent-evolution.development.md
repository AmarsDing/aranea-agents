# Agent 进化 — 开发计划

> **版本**：2026-06-17 | **状态**：✅ API + 指标 + Scanner + 状态机 + Rollback；🟡 趋势图 / diff / 护栏运行时未通
> **需求**：[7 agent-evolution.md](./7%20agent-evolution.md) · **设计**：[7 agent-evolution.design.md](./7-agent-evolution.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-07

---

## 1. 模块定位

Agent 自我进化：运行指标、改进建议、应用/拒绝/回滚；运行时开关在 `AgentRuntimeSettings`。

**代码锚点**：

| 层 | 路径 | 职责 |
|----|------|------|
| Biz — 领域 | `internal/biz/evolution.go` | `EvolutionUsecase` / `EvolutionMetrics` / `EvolutionSuggestion`（A6 起为 unified 行重建视图）/ 端口接口 |
| Biz — 扫描 | `internal/biz/skill_evolution_triggers.go` | `AgentConfigTrigger`（A6 移植自 legacy `evolution_scan.go` 的 `ScanAgent`；opt-in 门控 + type+title 去重） |
| Biz — 状态机 | `internal/biz/evolution_state_machine.go` | `EvolutionStateMachine`（Pending/Applied/Rejected/RolledBack） |
| Biz — 统一去重 | `internal/biz/skill_evolution_unified.go` | `SkillEvolutionOrchestrator`（统一跨流水线去重；legacy `evolution_coordinator.go` 已随 A6 删除） |
| Biz — 草稿生成 | `internal/biz/evolution_drafter.go` | `EvolutionDrafter`（EVO-20：L3 通知类 persona/prompt 建议的 LLM 草稿生成，写回 `apply_payload` + `diff_preview`，1h 节流） |
| Biz — 设置 | `internal/biz/agent_types.go` | `AgentRuntimeSettings` 定义（含 `Evolution*` / `Evo*` / `Guardrail*` 字段） |
| Biz — 设置解析 | `internal/biz/agent_settings_helpers.go` | `settingsFromLegacyConfig` 解析存储 |
| Biz — 设置视图 | `internal/biz/agent_settings.go` | `EvolutionCfg` 定义 + `GetEvolution()` 视图 |
| Data — 指标 | `internal/data/evolution_metrics_repo.go` | 从 `tool_invocations` 聚合成功率/检索质量 |
| Data — 建议 | `internal/data/unified_evolution.go` | `UnifiedEvolutionRepo`（A6：统一进化存储，raw SQL + 方言感知；legacy `evolution_suggestion_repo.go` / `evolution_store_bridge.go` / Ent Schema `evolution_suggestion.go` 已删除） |
| Service | `internal/service/agent_evolution.go` | Evolution RPC（挂在 `AgentService`） |
| Service — 主 | `internal/service/agent.go` | `AgentService` 持有 `evoUC` 字段 + `invalidateAgentBuildCache` |
| Cron | `internal/cronrunner/jobs/evolution_orchestrator_worker.go` | `EvolutionOrchestratorWorker`（30min 默认，统一进化触发入口；legacy `evolution_scanner.go` 已删除） |
| Wire | `cmd/admin/wire.go` | `provideEvolutionUsecase` / `provideSkillEvolutionOrchestrator` / `provideEvolutionDrafter` / `provideEvolutionOrchestratorWorker`（`EVOLUTION_ORCHESTRATOR_DISABLED=1` 可关） |
| Workers | `cmd/admin/workers.go` | `goAfterReady("evolution_orchestrator", ...)` 启动 |
| Agent Build | `internal/agent/trpc_build.go` | `BuildTRPCLLMAgent` 读取 `ag.Settings`（含进化开关） |
| Web — 组件 | `web/src/components/agents/AgentEvolutionPanel.vue` | 进化 Tab 主组件 |
| Web — composable | `web/src/features/agents/useAgentEvolutionPanel.ts` | 进化面板 composable |
| Web — composable | `web/src/features/agents/useAgentEvolutionSettings.ts` | 进化设置 composable |
| Web — composable | `web/src/features/agents/useLearningLoopPanel.ts` | Learning Loop composable |
| Web — 组件 | `web/src/components/agents/AgentLearningLoopPanel.vue` | Learning Loop 面板 |
| Web — 组件 | `web/src/components/agents/LearningLoopOverview.vue` | Learning Loop 概览 |
| Web — 组件 | `web/src/components/agents/LearningObservationList.vue` | 观察列表 |
| Web — 组件 | `web/src/components/agents/LearningPatternList.vue` | 模式列表 |
| Web — 组件 | `web/src/components/agents/LearningProposalList.vue` | 提议列表 |
| Web — API | `web/src/features/agents/api.learning.ts` | Learning Loop API |
| Web — Store | `web/src/stores/learningLoop/index.ts` | Learning Loop store |
| Web — Store | `web/src/stores/skillEvolution/index.ts` | 技能进化 store（含建议列表） |
| Web — Store | `web/src/stores/agents/detail.ts` | Agent 详情 store（含 `fetchEvolutionMetrics`） |

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 指标 RPC | ✅ | `GetAgentEvolutionMetrics`（`internal/service/agent_evolution.go:24`） |
| 建议列表 RPC | ✅ | `GetAgentEvolutionSuggestions`（`internal/service/agent_evolution.go:46`） |
| 应用建议 RPC | ✅ | `ApplyEvolutionSuggestion`（`internal/service/agent_evolution.go:58`） |
| 拒绝建议 RPC | ✅ | `RejectEvolutionSuggestion`（`internal/service/agent_evolution.go:69`） |
| 指标真实计算 | ✅ | `evolutionMetricsRepo` 查 `tool_invocations`（非静态零值） |
| `EvolutionOrchestratorWorker` worker | ✅ | `internal/cronrunner/jobs/evolution_orchestrator_worker.go`；Wire + `main` 启动；`EVOLUTION_ORCHESTRATOR_DISABLED=1` 可关（A6 起取代 legacy `EvolutionScanner`） |
| 自动生成建议 | ✅ | `AgentConfigTrigger.Check` 阈值写入 `unified_evolution_suggestions`（pending 同 legacy_type+title 去重，DB 唯一索引兜底） |
| `ApplySuggestion` type=persona | ✅ | 写入 `IDENTITY.md` ## Persona section（PGO V2 后替代 SOUL.md，保留 SOUL.md 兜底） |
| `ApplySuggestion` type=prompt | ✅ | 写入 `AGENTS_CORE.md` 或首匹配 `AGENTS*.md` 文件 |
| `ApplySuggestion` type=skill | ✅ | 写入 `unified_evolution_suggestions` 表（扫描器生成，A6 收敛） |
| 状态机（AS-FSM-01） | ✅ | `EvolutionStateMachine`（`internal/biz/evolution_state_machine.go`）；4 状态 3 转换 |
| Rollback 建议 | ✅ | `RollbackSuggestion`（`internal/biz/evolution.go`）+ `PreApplySnapshot`（metadata JSON）恢复 prompt files |
| 跨流水线去重 | ✅ | `AgentConfigTrigger` 内 type+title 去重 + `SkillEvolutionOrchestrator` 统一 pending 检查（legacy `EvolutionCoordinator` 已删除） |
| Apply 后失效缓存 | ✅ | `invalidateAgentBuildCache(req.GetAgentId())`（`internal/service/agent_evolution.go:65`） |
| Learning Loop API | ✅ | observation/pattern/proposal CRUD + run |
| `diff_preview` 生成 | ❌ | 多为空（`AIRefineButton` 有 diff，但 evolution suggestion `diff_preview` 仍空） |
| SOUL.md 自动演化 | ❌ | 开关有，无 Scanner 自动写文件（PGO V2 已 deprecated SOUL.md，persona target 改为 IDENTITY.md） |
| 护栏运行时 | ❌ | `guardrail_*` 字段未参与扫描 |
| `evo_auto_apply` 自动应用 | ❌ | 字段已定义，扫描器未自动应用 pending 建议 |
| per-agent 节流 TTL | ❌ | `evo_throttle_hours` 字段已定义，扫描器未实现节流 |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 进化四项开关 | ✅ | `AgentEvolutionPanel.vue` + `evolutionToggles` 绑定 |
| 自动提议流水线表单 | ✅ | `evo_enabled` / `evo_auto_apply` / `evo_min_*` / `evo_throttle_hours` 等输入 |
| 时间范围 7d/30d/90d | ✅ | `q-btn-toggle` + `rangeOptions` |
| 指标卡片 | ✅ | 三张 KPI 卡片（成功率 / 检索质量 / 待处理建议数） |
| 工具成功率趋势迷你柱状图 | ✅ | `app-mini-bar-chart` 渲染 `tool_success_series` |
| 建议列表 | ✅ | 应用/拒绝按钮 + 状态徽章 |
| 适应护栏表单 | ✅ | 三项数值输入（`max_change_per_period` / `min_data_points` / `rollback_on_decline_percent`） |
| Learning loop panel | ✅ | 4 sub-components（Overview/Observation/Pattern/Proposal） |
| 折线图表（ECharts/Chart.js） | ❌ | 仅有迷你柱状图，无完整折线图（`retrieval_quality_series` 未绘图） |
| Diff preview 展示 | 🟡 | `AIRefineButton` 有 diff，但 evolution suggestion `diff_preview` 仍空 |
| 指标/建议空态 | 🟡 | 零值时仍展示卡片，可增强文案 |
| Rollback UI | ❌ | 后端 `RollbackSuggestion` 已实现，前端无入口 |

### 2.3 进化开关字段实现状态

进化开关字段已在 `AgentRuntimeSettings`（`internal/biz/agent_types.go`）中定义并通过 `settingsFromLegacyConfig`（`internal/biz/agent_settings_helpers.go`）解析存储。

| 功能 | 开关字段 | 字段状态 | 运行时状态 |
|------|---------|---------|-----------|
| 风格进化 | `EvolutionSelfEvolve` / `SelfEvolve`（deprecated） | ✅ 字段已实现 | 🟡 运行时未自动修改 `IDENTITY.md`，仅手动应用建议触发 |
| 技能进化 | `EvolutionSkillEvolve` | ✅ 字段已实现 | 🟡 运行时未实现技能自动创建逻辑 |
| 进化指标 | `EvolutionMetricsEnabled` | ✅ 字段已实现 | ✅ 运行时通过 `tool_invocations` 表查询统计 |
| 进化建议 | `EvolutionSuggestionsEnabled` | ✅ 字段已实现 | ✅ `EvolutionOrchestratorWorker`（`AgentConfigTrigger`）定时任务基于指标生成建议 |
| 自动提议流水线 | `EvoEnabled` / `EvoAutoApply` / `EvoMin*` | ✅ 字段已实现 | 🟡 `EvoEnabled` 参与扫描门控；`EvoAutoApply` 未实现自动应用 |
| 适应护栏 | `GuardrailMaxChangePerPeriod` 等 | ✅ 字段已定义 | ❌ 运行时未使用这些参数控制演化幅度 |

---

## 3. 差距与优化

| ID | 优先级 | 待优化项 | 状态 |
|----|--------|----------|------|
| EVO-01 | P2 | `evolution_scanner.go` + Wire 启动 + `safego` | ✅ |
| EVO-02 | P2 | 阈值生成 `EvolutionSuggestion` | ✅ |
| EVO-03 | P2 | 状态机 `EvolutionStateMachine`（AS-FSM-01） | ✅ |
| EVO-04 | P2 | `RollbackSuggestion` + `PreApplySnapshot` | ✅ |
| EVO-05 | P2 | 跨流水线去重（`SkillEvolutionOrchestrator`） | ✅ |
| EVO-06 | P2 | Apply 后 `invalidateAgentBuildCache` 失效缓存 | ✅ |
| EVO-07 | P3 | `diff_preview` 生成（🟡 `AIRefineButton` has diff, evolution suggestion still empty） | ✅ 2026-08-08（EVO-20 草稿写回 diff_preview） |
| EVO-08 | P3 | SOUL.md 自动演化（仅风格段）（❌ 但 PGO V2 已 deprecated SOUL.md，target 改为 IDENTITY.md） | ❌ |
| EVO-09 | P3 | 护栏运行时参与扫描 | ❌ |
| EVO-10 | P3 | `evo_auto_apply` 自动应用 pending 建议 | ❌ |
| EVO-11 | P3 | per-agent 节流 TTL（`evo_throttle_hours`） | ❌ |
| EVO-12 | P3 | 前端趋势图（ECharts/Chart.js）展示 `retrieval_quality_series` | ❌ |
| EVO-13 | P3 | 前端 Rollback UI 入口 | ❌ |
| EVO-14 | P3 | Learning Loop 与 Evolution 对齐（共享 Apply 逻辑） | 🟡 |
| EVO-15 | P3 | PGO V2 影响 — SOUL.md deprecated，evolution persona target 变更 | ✅ 已迁移到 IDENTITY.md |
| EVO-16 | P0 | 面板建议区错接 L1 `ListSkillProposals`，L3 `evolve_agent` 建议不可见 → 切换 L3 API（列表 + apply/reject） | ✅ 2026-08-07 |
| EVO-17 | P0 | 通知类建议（无实质修改内容）apply 会把通知文本写入 IDENTITY.md/AGENTS*.md → `apply_payload` metadata 门，空则拒绝 | ✅ 2026-08-07 |
| EVO-18 | P1 | 趋势图柱条不可见（`--color-primary` 未定义）→ 双主题补定义 | ✅ 2026-08-07 |
| EVO-19 | P1 | 无记忆调用时检索质量误显 0.0% → series 空时前端显「暂无数据」 | ✅ 2026-08-07 |
| EVO-20 | P1 | 通知类建议内容低质（无 diff_preview、文案重复）→ LLM 生成具体修改草稿并设置 `apply_payload` 解锁 apply | ✅ 2026-08-08 |

---

## 4. 验收标准

- [x] 指标 API 可返回基于 `tool_invocations` 的数据
- [x] 前端可展示指标卡片与建议列表
- [x] EvolutionOrchestratorWorker 周期性运行（默认 30min；L3 由 AgentConfigTrigger 执行）
- [x] 建议可由扫描自动生成（非仅手工/种子）
- [x] Learning Loop panel 可用（Overview/Observation/Pattern/Proposal）
- [x] `ApplySuggestion` persona type 写入 IDENTITY.md ## Persona 段
- [x] `ApplySuggestion` prompt type 写入 AGENTS_CORE.md 或首匹配 AGENTS*.md
- [x] `EvolutionStateMachine` 校验所有状态转换（Pending→Applied/Rejected，Applied→RolledBack）
- [x] `RollbackSuggestion` 从 `PreApplySnapshot` 恢复 prompt files
- [x] 跨流水线去重（trigger 内 type+title 去重 + orchestrator 统一 pending 检查 + DB 唯一索引兜底）
- [x] Apply 后 `invalidateAgentBuildCache` 失效缓存
- [ ] 前端趋势图展示 `tool_success_series` / `retrieval_quality_series`（完整折线图）
- [x] Evolution suggestion `diff_preview` 非空（EVO-20：drafter 写回草稿时生成 unified diff）
- [ ] 护栏运行时参与扫描
- [ ] `evo_auto_apply` 自动应用 pending 建议
- [ ] 前端 Rollback UI 入口

---

## 5. 架构备注

### 5.1 Scanner（A6 重构后）

| 组件 | 路径 |
|------|------|
| 扫描逻辑 | `internal/biz/skill_evolution_triggers.go` → `AgentConfigTrigger`（移植自 legacy `evolution_scan.go`） |
| Worker | `internal/cronrunner/jobs/evolution_orchestrator_worker.go` |
| 注册 | `cmd/admin/wire.go` → `provideSkillEvolutionOrchestrator` + `provideEvolutionOrchestratorWorker` |
| 启动 | `cmd/admin/workers.go` → `goAfterReady("evolution_orchestrator", ...)` |

`AgentConfigTrigger.Check` 读取 `evolution_suggestions_enabled` / `evo_enabled` 与 `evo_min_*`（L3 opt-in 门控）；worker 失败时聚合错误并打 Warn 日志。

**扫描阈值**（`internal/biz/skill_evolution_triggers.go`）：
- `agentConfigScanToolSuccessThreshold = 0.75` — 工具成功率低于此值生成 prompt 建议
- `agentConfigScanRetrievalThreshold = 0.60` — 检索质量低于此值生成 skill 建议
- `agentConfigScanDefaultTimeRange = "30d"` — 扫描默认时间范围
- `EvoMinEpisodes` 默认 3，`EvoMinNegativeFeedback` 默认 2

**未做**：per-agent 节流 TTL（`evo_throttle_hours`）、`evo_auto_apply` 自动应用。

### 5.2 状态机

| 组件 | 路径 |
|------|------|
| 状态机 | `internal/biz/evolution_state_machine.go` |
| 测试 | `internal/biz/evolution_state_machine_test.go` |

状态枚举：`Pending` / `Applied` / `Rejected` / `RolledBack`
事件枚举：`Apply` / `Reject` / `Rollback`
终态：`Rejected` / `RolledBack`

基于 `shared.GenericStateMachine[EvolutionState, EvolutionEvent]` 实现，所有转换经 `Transition()` 校验。

### 5.3 Learning Loop

| 组件 | 路径 |
|------|------|
| API | `web/src/features/agents/api.learning.ts` |
| Store | `web/src/stores/learningLoop/index.ts` |
| Panel | `web/src/components/agents/AgentLearningLoopPanel.vue` |
| Overview | `web/src/components/agents/LearningLoopOverview.vue` |
| Observations | `web/src/components/agents/LearningObservationList.vue` |
| Patterns | `web/src/components/agents/LearningPatternList.vue` |
| Proposals | `web/src/components/agents/LearningProposalList.vue` |
| Composable | `web/src/features/agents/useLearningLoopPanel.ts` |

### 5.4 Rollback

| 组件 | 路径 |
|------|------|
| Usecase | `internal/biz/evolution.go` → `RollbackSuggestion` / `savePreApplySnapshot` |
| Store | `internal/data/unified_evolution.go` → `UpdateMetadataKey`（快照写入 metadata JSON，A6） |

`ApplySuggestion` 在写入 prompt files 前调用 `savePreApplySnapshot` 保存 `map[filename]content` JSON 快照（经 `store.UpdateMetadataKey` 写入 unified 行 metadata 的 `pre_apply_snapshot` key）；`RollbackSuggestion` 解码快照并恢复文件内容。注入 `EvolutionTxProvider` 时，文件替换 + 状态更新包裹在单事务中（红线 #24）。

---

## 6. 依赖

| 模块 | 说明 |
|------|------|
| 5 设置 | 进化开关 |
| 6 文件 | Apply 建议改 IDENTITY.md / AGENTS_CORE.md（PGO V2 已 deprecated SOUL.md） |
| 23 Tools | `tool_invocations` 数据源 |
| Learning Loop | observation/pattern/proposal 与 evolution 建议共享 Apply 逻辑 |
| Skill Evolution | `SkillEvolutionOrchestrator` 提供跨流水线去重 |
