# M60: Spirit Parallel Orchestrator — 开发计划

> **版本**：2026-06-01 | **状态**：✅ P2 已完成（含集成修复）
> **需求**：[60-spirit-parallel-orchestrator.md](./60-spirit-parallel-orchestrator.md) · **设计**：[60-spirit-parallel-orchestrator.design.md](./60-spirit-parallel-orchestrator.design.md)
> **前置**：M59 P0 已完成

---

## 1. 模块定位

Spirit Parallel Orchestrator (SPO)：精灵多任务并行编排，支持同一精灵 Session 下多团队并行执行、任务依赖调度、结果自动合成、编排策略进化。

**代码锚点**：

| 层级 | 路径 | 阶段 |
|------|------|------|
| Biz 并行配置 | `internal/biz/spirit_parallel_config.go` | P1 |
| Biz 团队并行 | `internal/biz/spirit_team_usecase.go` | P1 |
| Biz Task DAG | `internal/biz/spirit_task_dag.go` | P2 |
| Biz 合成引擎 | `internal/biz/spirit_synthesis.go` | P2 |
| Service 团队组装 | `internal/service/spirit_team.go` | P1 |
| Service 合成 | `internal/service/spirit_synthesis.go` | P2 |
| Tools 精灵工具 | `internal/tools/spirit_tools.go` | P1-P2 |
| Event | `internal/event/contract/envelope.go` | P1-P2 |
| 前端 Store | `web/src/stores/spirit/index.ts` | P1-P2 |
| 前端组件 | `web/src/components/spirit/` | P1-P2 |

---

## 2. 前置依赖

| 依赖 | 状态 | 说明 |
|------|------|------|
| M59 P0 精灵模式骨架 | ✅ 已完成 | 精灵入口 + assemble_team + Session 树 |
| M59 精灵工具注入 | ✅ 已完成 | CustomTools 机制 |
| M59 团队生命周期事件 | ✅ 已完成 | spirit_team_completed/failed |
| Team AutoCreated 字段 | ✅ 已完成 | 区分精灵创建 vs 手动创建 |
| SpiritTeamAssemblerPort | ✅ 已完成 | assemble_team 工具接口 |
| EvolutionUsecase | ✅ 已完成 | DQ Score 计算和建议生成 |
| LearningLoopUsecase | ✅ 已完成 | Pattern 检测和 Proposal 生成 |

---

## 3. 开发阶段

### Phase P1 — 基础并行（约 2 周）

> **目标**：移除单团队限制，支持多团队并行，并行度可配置，进度监控，团队取消。

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| SPO-BE-01 | `SpiritTeamUsecase.ListActiveTeams()`：按 spirit_session_id 查询所有 active 团队 | `internal/biz/spirit_team_usecase.go` | ✅ 已完成 |
| SPO-BE-02 | `SpiritTeamUsecase.GetMaxParallelTeams()`：从 AgentRuntimeSettings 读取并行度配置 | `internal/biz/spirit_team_usecase.go` | ✅ 已完成 |
| SPO-BE-03 | `ParallelConfig` 结构体 + 默认值 + 存储到 Agent MetadataJSON | `internal/biz/spirit_parallel_config.go` (新) | ✅ 已完成 |
| SPO-BE-04 | TeamKey UUID 后缀：`"spirit_" + sessionID + "_" + uuid[:8]` | `internal/biz/spirit_team_usecase.go` | ✅ 已完成 |
| SPO-BE-05 | `assemble_team` 工具改造：移除 `GetActiveTeam()` 短路，改用 `ListActiveTeams()` + 并行度检查 | `internal/tools/spirit_tools.go` | ✅ 已完成 |
| SPO-BE-06 | `check_team_progress` 工具：查询所有活跃团队进度 | `internal/tools/spirit_tools.go` | ✅ 已完成 |
| SPO-BE-07 | `cancel_team` 工具：取消指定团队 | `internal/tools/spirit_tools.go` | ✅ 已完成 |
| SPO-BE-08 | `SpiritTeamAssemblerPort` 接口扩展：ListActiveTeams / GetMaxParallelTeams / CancelTeam / CheckTeamProgress | `internal/tools/spirit_tools.go` | ✅ 已完成 |
| SPO-BE-09 | `SpiritTeamAssembler` 实现扩展：ListActiveTeams / CancelTeam / CheckTeamProgress | `internal/service/spirit_team.go` | ✅ 已完成 |
| SPO-BE-10 | 精灵 CustomTools 扩展：注入 check_team_progress + cancel_team | `internal/service/cli_admin_tools.go` | ✅ 已完成 |
| SPO-BE-11 | 新增 EnvelopeType：`spirit_team_progress` / `spirit_teams_all_completed` / `spirit_synthesis_completed` | `internal/event/contract/envelope.go` | ✅ 已完成 |
| SPO-BE-12 | 精灵 Observer：订阅子团队完成事件，全部完成时发布 `spirit_teams_all_completed` | `internal/service/team_turn_hooks.go` | ✅ 已完成 |
| SPO-BE-13 | Data 层：`ListBySpiritSessionID` 查询已实现（M59 已完成） | `internal/data/team_repo.go` | ✅ 已完成 |
| SPO-BE-14 | 团队状态扩展：前端新增 `waiting_deps` / `cancelled` 状态 | `internal/biz/team_types.go` + 前端 types.ts | ✅ 已完成 |
| SPO-FE-01 | `ParallelConfig` / `TeamProgressView` 类型 | `web/src/features/spirit/types.ts` | ✅ 已完成 |
| SPO-FE-02 | `useSpiritTeamStore` 扩展：并行团队列表 + 进度查询 + 取消 + runningTeamCount | `web/src/stores/spirit/index.ts` | ✅ 已完成 |
| SPO-FE-03 | `ParallelTeamOverview.vue`：精灵对话中的并行团队总览卡片 | `web/src/components/spirit/` (新) | ✅ 已完成 |
| SPO-FE-04 | `TeamProgressCard.vue`：单团队进度卡片 | `web/src/components/spirit/` (新) | ✅ 已完成 |
| SPO-FE-05 | WS 事件处理：`spirit_team_progress` / `spirit_teams_all_completed` | `web/src/stores/spirit/index.ts` + `useChatInboundSync.ts` + `realtime/envelope.ts` | ✅ 已完成 |

---

### Phase P2 — 智能编排（约 3 周）

> **目标**：Task DAG 依赖调度、拓扑路由、Synthesis Engine、编排进化闭环。

| ID | 任务 | 影响域 | 验收 |
|----|------|--------|------|
| SPO-BE-15 | `TaskNode` / `TaskDAG` 数据模型 + 校验（环检测、依赖完整性） | `internal/biz/spirit_task_dag.go` (新) | ✅ 已完成 |
| SPO-BE-16 | `TaskDAG.RouteTopology()`：拓扑路由算法（parallel/sequential/hybrid/coordinator） | `internal/biz/spirit_task_dag.go` | ✅ 已完成 |
| SPO-BE-17 | `DependencyScheduler`：前置团队完成后自动启动依赖团队 | `internal/biz/spirit_dependency_scheduler.go` (新) | ✅ 已完成 |
| SPO-BE-18 | `SynthesisEngine`：结果合成引擎（模板/LLM/混合策略） | `internal/biz/spirit_synthesis.go` (新) | ✅ 已完成 |
| SPO-BE-19 | `SynthesisPort` 接口 + `SpiritSynthesisService` 实现 | `internal/service/spirit_synthesis.go` (新) | ✅ 已完成 |
| SPO-BE-20 | `synthesize_results` 工具 | `internal/tools/spirit_tools.go` | ✅ 已完成 |
| SPO-BE-21 | 精灵 CustomTools 扩展：注入 synthesize_results | `internal/service/cli_admin_tools.go` | ✅ 已完成 |
| SPO-BE-22 | `OrchestrationCache`：DQ Score 驱动的编排拓扑缓存 | `internal/biz/spirit_orchestration_cache.go` (新) | ✅ 已完成 |
| SPO-BE-23 | 编排进化闭环：团队完成后计算 DQ Score → 缓存/建议 | `internal/biz/spirit_orchestration_cache.go` | ✅ 已完成 |
| SPO-BE-24 | `assemble_team` 增强：先查编排缓存，命中则复用历史最优拓扑 | `internal/tools/spirit_tools.go` + `internal/service/spirit_team.go` | ✅ 已完成 |
| SPO-BE-25 | DAG 存储到 Team 记录：dag_node_id / depends_on 字段 | `internal/data/team_repo.go` | ✅ 已完成 |
| SPO-BE-26 | 新增 EnvelopeType：`spirit_synthesis_completed` | `internal/event/contract/envelope.go` | ✅ 已完成 |
| SPO-BE-27 | Team Proto 扩展：dag_node_id / depends_on / parallel_config_json | `api/kratos/team/v1` | ✅ 已完成 |
| SPO-FE-06 | `TaskDAG` 类型 + DAG 可视化（文本形式） | `web/src/features/spirit/types.ts` | ✅ 已完成 |
| SPO-FE-07 | `SynthesisResult` 类型 + 合成结果卡片 | `web/src/features/spirit/types.ts` + `components/spirit/SynthesisResultCard.vue` | ✅ 已完成 |
| SPO-FE-08 | 依赖调度 UI：团队卡片显示依赖状态（等待中 → 运行中） | `web/src/components/spirit/TeamProgressCard.vue` | ✅ 已完成 |
| SPO-FE-09 | 编排模式说明：精灵回复中展示拓扑选择理由 | `web/src/components/spirit/OrchestrationModeBadge.vue` | ✅ 已完成 |

---

### Phase P3 — 远期方向（不纳入本次开发）

| 方向 | 说明 | 参考 |
|------|------|------|
| 多团队竞速 | 同一任务启动多个团队，最早成功者胜出 | M1-Parallel (ICML 2025) |
| Git Worktree 文件隔离 | 团队绑定 Worktree 实现文件级隔离 | Claude Code / Cursor |
| 自适应并行度 | 根据系统负载和 Token 配额动态调整并行度 | ParaCook (AAMAS 2026) |
| A2UI Planner 集成 | 结构化执行计划 → Graph 自动转换 | 39-planner.md |
| SubAgent 后台派生 | Agent 通过工具动态创建后台子 Agent | phase2-04-SubAgent后台派生.md |

---

## 4. 任务板（P2 当前冲刺）

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 1 | SPO-BE-01 | ListActiveTeams 查询 | ✅ |
| 2 | SPO-BE-02 | GetMaxParallelTeams 配置读取 | ✅ |
| 3 | SPO-BE-03 | ParallelConfig 结构体 | ✅ |
| 4 | SPO-BE-04 | TeamKey UUID 后缀 | ✅ |
| 5 | SPO-BE-05 | assemble_team 工具改造 | ✅ |
| 6 | SPO-BE-06 | check_team_progress 工具 | ✅ |
| 7 | SPO-BE-07 | cancel_team 工具 | ✅ |
| 8 | SPO-BE-08 | SpiritTeamAssemblerPort 接口扩展 | ✅ |
| 9 | SPO-BE-09 | SpiritTeamAssembler 实现扩展 | ✅ |
| 10 | SPO-BE-10 | 精灵 CustomTools 扩展 | ✅ |
| 11 | SPO-BE-11 | 新增 EnvelopeType | ✅ |
| 12 | SPO-BE-12 | 精灵 Observer | ✅ |
| 13 | SPO-BE-13 | Data 层查询实现 | ✅ |
| 14 | SPO-BE-14 | 团队状态扩展 | ✅ |
| 15 | SPO-FE-01 | 前端类型 + API | ✅ |
| 16 | SPO-FE-02 | Store 扩展 | ✅ |
| 17 | SPO-FE-03 | ParallelTeamOverview 组件 | ✅ |
| 18 | SPO-FE-04 | TeamProgressCard 组件 | ✅ |
| 19 | SPO-FE-05 | WS 事件处理 | ✅ |
| 20 | SPO-BE-15 | TaskDAG 数据模型 + 环检测 | ✅ |
| 21 | SPO-BE-16 | 拓扑路由算法 | ✅ |
| 22 | SPO-BE-17 | DependencyScheduler | ✅ |
| 23 | SPO-BE-18 | SynthesisEngine | ✅ |
| 24 | SPO-BE-19 | SynthesisPort + Service | ✅ |
| 25 | SPO-BE-20 | synthesize_results 工具 | ✅ |
| 26 | SPO-BE-21 | CustomTools 注入 synthesize_results | ✅ |
| 27 | SPO-BE-22 | OrchestrationCache | ✅ |
| 28 | SPO-BE-23 | 编排进化闭环 | ✅ |
| 29 | SPO-BE-24 | assemble_team 缓存增强 | ✅ |
| 30 | SPO-BE-25 | DAG 存储到 Team 记录 | ✅ |
| 31 | SPO-BE-26 | spirit_synthesis_completed EnvelopeType | ✅ |
| 32 | SPO-BE-27 | Team Proto 扩展 | ✅ |
| 33 | SPO-FE-06 | TaskDAG 类型 | ✅ |
| 34 | SPO-FE-07 | SynthesisResult 卡片 | ✅ |
| 35 | SPO-FE-08 | 依赖调度 UI | ✅ |
| 36 | SPO-FE-09 | 编排模式说明 UI | ✅ |

### 集成修复（P2 Review 后）

| 排序 | ID | 任务 | 状态 |
|------|-----|------|------|
| 37 | SPO-INT-01 | Wire 注入链修复：provideChatServiceDeps 填充 SpiritAssembler/SpiritSynthesis/OrchCache | ✅ |
| 38 | SPO-INT-02 | SynthesizeResults 查询逻辑修复：ListActiveTeams→ListCompletedTeams | ✅ |
| 39 | SPO-INT-03 | DQ Score 进化闭环接入：团队完成时 ComputeDQScore + RecordCompletion | ✅ |
| 40 | SPO-INT-04 | TaskDAG 拓扑路由接入：assemble_team 集成 ParseTaskDAG + RouteTopology | ✅ |
| 41 | SPO-INT-05 | LLM 策略重命名为 Prompt 策略（SynthesisStrategyLLM→SynthesisStrategyPrompt） | ✅ |
| 42 | SPO-INT-06 | TeamProgressCard Mode→Topology 类型映射修复 | ✅ |

---

## 5. 验收标准

### Phase P1

- [ ] `make api && make wire && make build` 通过
- [ ] `go test ./internal/biz/... ./internal/service/... -count=1` 通过
- [ ] 同一精灵 Session 可创建多个并行团队（SPO-01）
- [ ] 并行度超限时精灵提示用户等待（SPO-02）
- [ ] 团队进度实时监控 + 精灵主动通知（SPO-03）
- [ ] 取消团队 + 释放配额（SPO-04）
- [ ] `cd web && pnpm lint && pnpm test && pnpm build` 通过

### Phase P2

- [ ] Task DAG 依赖调度正确执行（SPO-05）
- [ ] 拓扑路由自动选择编排模式（SPO-06）
- [ ] Synthesis Engine 结果合成（SPO-07）
- [ ] DQ Score 驱动编排缓存（SPO-08）
- [ ] 编排策略进化闭环（SPO-09）

---

## 6. 依赖与风险

| 风险 | 缓解 |
|------|------|
| 多 Team 并行导致 Token 配额耗尽 | `MaxConcurrentTeams` 硬限制 + 配额预检 |
| 多 Team 同时写数据库导致锁竞争 | 读写分离 + 乐观锁 |
| 团队间隐式依赖导致结果不一致 | Task DAG 显式声明依赖 + 拓扑路由 |
| 前端 WS 消息风暴（多 Team 同时推送） | 事件聚合 + 节流 + 按团队分组 |
| Session 树过深导致上下文丢失 | `MaxSessionDepth=2` 强制限制 |
| 编排进化策略退化 | `GuardrailMaxChangePerPeriod` 约束 + DQ Score < 0.3 回滚 |
| Synthesis Engine LLM 调用增加成本 | 简单场景用模板合成，仅复杂场景调用 LLM |

---

## 7. 关联文档更新

| 文档 | 更新内容 | 时机 |
|------|---------|------|
| [59-chat-spirit-mode.md](./59-chat-spirit-mode.md) | US-07 多任务并行指向 M60 | P1 |
| [11-multi-agent.md](./11-multi-agent.md) | 并行编排扩展 | P1 |
| [7-agent-evolution.md](./7-agent-evolution.md) | 编排进化闭环 | P2 |
| [architecture-blueprint.md](../architecture-blueprint.md) | SPO 模块卡片 | P1 |
| [module-cross-reference.md](../module-cross-reference.md) | M60 模块卡片 | P1 |

---

## 8. 学术参考索引

| 论文 | arXiv ID | 对 M60 的贡献 |
|------|----------|--------------|
| LAMaS | 2601.10560 | Task DAG 层级调度、关键路径优化 |
| AdaptOrch | 2602.16873 | 拓扑路由算法 O(\|V\|+\|E\|) |
| Maestro | 2511.06134 | 探索-合成分离、Synthesis Engine |
| M1-Parallel | 2507.08944 | 多团队并行竞速（远期参考） |
| APWA | 2605.15132 | Manager-Worker-Executor 三层分离 |
| DTA-Llama | 2501.12432 | Divide-Then-Aggregate 范式 |
| Hogwild! Inference | 2504.06261 | 共享上下文并行（远期参考） |
| ParaCook | 2510.11608 | 时间效率感知并行规划 |
| Puppeteer | OpenReview L0xZPXT3le | RL 动态编排、DQ Score 驱动策略优化 |
| Self-Resource Allocation | OpenReview 0ZnEzvSLNR | Planner 优于 Orchestrator |
| Multi-Agent Coordination Survey | 2502.14743 | 混合编排拓扑 |
