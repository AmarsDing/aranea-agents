# Team Graph 代码层 Review（业务逻辑 / 代码质量 / 架构设计）

> **评分**：80 / 100 | **风险等级**：P1
> **审查时间**：2026-05-26
> **范围**：`internal/team/`（共 58 个 Go 文件 / ~6.5k 行；不含 frontend、不含 docs）
> **聚焦**：team graph 编译、graph runtime、HITL/resume 协调、step 持久化、native fallback、可观测投影
> **真相源**：`docs/AGENT_RUNTIME_BOUNDARY.md`、`.cursor/rules/trpc-agent-framework-first.mdc`
> **历史 Review**：[11-team-review.md](./11-team-review.md) · [2026-05-23 M53 Phase7](./2026-05-23-Team-Graph-M53-Phase7-Review.md)

---

## 1. 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 业务逻辑正确性 | 17 | 20 | 6 种模式编译路径清晰，HITL/resume 状态机闭环；但 `failed` vs `error` 状态字面量分裂、`CleanupStaleSessions` 未被调度、`persistGraphMemberStepsFromResult` 仅测试可达 |
| 架构一致性 | 21 | 25 | 编译/运行时/协调器分层得当，`GraphRunStepContext` DTO 解耦合理；但 `runner_team_compiler.go` 已有提取但未启用、`runner_stream_opts.go` 仍直接 import `chatactivity` 与既有 refactor 提交目标冲突 |
| 代码质量 | 17 | 20 | 命名规范、错误处理多用 FlowLog warn，整体 Go 风格干净；`runner_team_trpc.go` 620 行单方法是主要复杂度集中点，并存在 import 块分裂 / 缩进对齐错误 |
| 测试覆盖 | 6 | 10 | 编译/parity/canary/projector/bridge 单测齐全；但缺少 `runTeamTRPCFromInput` 主路径 E2E、parity E2E 在测试一个生产未调用的函数、graph watch 超时/eviction 缺测 |
| 可扩展性与抽象 | 12 | 15 | embedded graph、subgraph、failure policy、circuit breaker 都已抽象；adaptive 模式 N² 边构造硬上限 30 隐藏裁剪、critic_loop "approved" 字符串硬编码 |
| 文档/注释一致性 | 7 | 10 | 关键导出符号有 Phase 编号注释（M53/Phase 6/7/BL-01..03/ARCH-01）；但 native fallback 已是冷路径，文档未明确弃用计划 |

---

## 2. 模块组成（按层归类）

| 层 | 文件 | 职责 |
|----|------|------|
| **定义/解析** | `definition.go` `graph_definition_json.go` `embedded_graph.go` `compile_snapshot.go` | DefinitionJSON → Go 结构、embedded graph 解析、subgraph 递归装配 |
| **编译** | `graph_compile.go` `graph_runtime_config.go` `graph_runtime_options.go` `graph_structure.go` | 模式 → biz.GraphBuildConfig；运行时 finalize（checkpoint/中断/熔断/重试） |
| **运行时** | `runner.go` `runner_team_trpc.go` `trpc_build.go` `runner_team_observer.go` `runner_helpers.go` `runner_finish_steps.go` `runner_stream_opts.go` | turn 编排、graph 优先 → native fallback、step 持久化、回填 |
| **协调（HITL/resume）** | `team_graph_run_coordinator.go` `team_graph_run_finisher.go` `team_graph_run_context.go` `team_graph_task_bridge.go` `team_graph_execution_tracker.go` `task_creator.go` | Graph execution 注册、HITL 推迟、watch loop、Kanban task 桥接、resume finisher |
| **可观测** | `status_projector.go` `activity_step_flusher.go` `summary.go` `usage_record.go` `usage_tokens.go` `llm_catalog.go` | orchestration_agent_status 投影、activity 批写、team_summary 聚合、token usage 落库 |
| **运行时网关** | `graph_runtime.go` `graph_runtime_canary.go` `fallback_policy.go` | 环境/版本开关、灰度桶、fallback 决策树 |
| **辅助 / 装配** | `provider.go` `agent_keys.go` `builder.go` `graph_loader.go` | Wire ProviderSet、agent_key 解析、循环边界、subgraph loader 接口 |

---

## 3. 业务逻辑分析

### 3.1 编译路径（5 种模式 + adaptive/swarm 别名 + embedded）

`graph_compile.go` 输出统一的 `biz.GraphBuildConfig`：

- **sequential**：相邻节点 → `flow` 边链。
- **parallel**：worker fan-out / fan-in；`synthesizer_agent_id` 作为 finish point；同时排除自环（worker == finish）。
- **coordinator**：hub-and-spoke，dispatch → flow 双向；显式跳过 self-loop（`hub != finish`）。
- **critic_loop**：相邻 flow + 末节点 `PathMap{"approved": last, "retry": first}` 条件边。
- **adaptive**（含 `swarm` → 归一化）：sequential + 总数 ≤ `adaptiveMaxTransferEdges = 30` 的 transfer overlay；runtime 阶段由 `applyAdaptiveAgentDestinations` 把 transfer 边搬到 `Destinations` 并通过 `FilterVisualizationEdges` 剔除可视化边。

`embedded_graph.go` 提供 UI 自定义拓扑入口：支持 `agent / task / review / subgraph` 四种执行节点 + `start / end / join` 装饰节点；`join` 边触发 ParallelBranchIDs 推导，subgraph 递归装配带循环检测。

> **业务正确性**：六种模式编译均有专属测试（`graph_compile_test.go` / `parity_test.go` / `parity_runtime_test.go`）。`TestParityNativeVsGraph_RuntimeBuildAllModes` 比对 native member key 与 graph node 集合完全一致——这是关键的不变量。

### 3.2 运行时分支（Graph vs Native）

`runTeamTRPCFromInput`（`runner_team_trpc.go`）主链路：

1. 解析 `Definition` 与 mode。
2. `useGraph = r.graphRoot != nil && TeamGraphRuntimeEnabledForTeam(def, teamID) && SupportsTeamGraphRuntimeMode(mode)`。
3. Graph 路径：`CompileToGraphRuntimeConfigFromJSON` → `BuildTeamGraphRoot` → `BuildTeamMemberAgents` → 注册 graph_execution_id；失败按 `DecideNativeFallback` 决策。
4. Native fallback 仅当 `ARANEA_TEAM_NATIVE=1` 或灰度 holdout；否则返回 `kerrors.InternalServer` 并带明确诊断。
5. 启动四个 observer：`OrchestrationStatusProjector`、`TeamGraphTaskBridge`、`TeamGraphExecutionTracker`、`StartGraphStepWatch`。
6. `RunTRPCUserTurnMsg` + `ConsumeWithFirstByteGuard` 流式消费。
7. **HITL 分支**（关键）：在 bulk step 持久化前 `DeferTeamRunSuccessIfHITL` — 若 graph 处于 `waiting_human`，立即返回，不再写 bulk steps（避免与 graph 事件 step 冲突，对应 BL-01 修复）。
8. **非 HITL**：根据 `graphExecID == ""` 选择 `persistNativeBulkMemberSteps`（写 N 条 step）或 `finalizeGraphRunStepsFallback`（仅写 anchor 兜底，真正 per-member steps 来自 graph watch）。

### 3.3 HITL / Resume 状态机

`TeamGraphRunCoordinator` 维护 `execID → teamGraphRunSession` 映射：

```352:392:internal/team/team_graph_run_coordinator.go
func (c *TeamGraphRunCoordinator) finalizeTeamRun(ctx context.Context, sess *teamGraphRunSession, failed bool, errMsg string) {
	if c == nil || sess == nil {
		return
	}
	if c.finisher != nil {
		c.finisher.FinalizeGraphTeamRun(ctx, sess.stepContext(), failed, errMsg)
		c.evictSession(sess.execID)
		return
	}
	...
}
```

状态流：
`running` → `Checkpoint envelope` → `MarkTeamGraphInterrupt` → `waiting_human` → `Task completed` → `ResumeExecution` → `running` → `GraphExecutionDone` → `finalizeTeamRun(success)`。

- BL-02 修复后，watch 超时 30 分钟也会触发 `finalizeTeamRun(failed)`，避免长跑 run 永久滞留。
- BL-01 修复后，HITL 时不再 bulk 持久化 step，让 graph 事件成为真相源。

### 3.4 状态机字面量

`teamRunStatusWaitingHuman = "waiting_human"` 提取为常量，但 `"running"`、`"success"`、`"failed"`、`"error"`、`"cancelled"`、`"pending"`、`"ok"` 等仍是散落字面量；并存在 **不一致风险**：

| 位置 | 失败状态字面量 |
|------|--------------|
| `runner_helpers.go:80` (finishRunErr) | `"failed"` |
| `team_graph_run_coordinator.go:367` (finalizeTeamRun fallback) | `"error"` |
| `runner_team_trpc.go` 多处 (`turnStatus`) | `"error"` |

`team_summary` / `run_status` 下游若按字符串过滤会漏判一类。建议提取 `biz` 层 `TeamRunStatus*` 常量统一。

---

## 4. 代码质量评估

### 4.1 复杂度热点

| 文件 | 行数 | 复杂度问题 |
|------|------|----------|
| `runner_team_trpc.go` | **620** | 单方法 `runTeamTRPCFromInput` ≈ 590 行：意图预热、附件解析、Ralph Loop、observers 启动、graph 编译/构建、HITL defer、bulk 持久化、token 落库、context patch、emitter 收尾——9 个职责堆叠 |
| `team_graph_run_coordinator.go` | **447** | Coordinator 同时承担：注册、HITL 标记、resume、watch loop、终结、stale GC、session DTO 构造 |
| `embedded_graph.go` | 344 | 节点/边解析 + subgraph 递归 + 模式归一化，函数较短但分支密 |
| `graph_compile.go` | 300 | 编译路径线性，分函数清晰 |
| `trpc_build.go` | 273 | 五模式分发 + swarm/critic 配置；deprecated 但仍是 emergency 入口 |

> **复杂度评级**：除 `runner_team_trpc.go` 之外，其它文件单函数体量合理；该文件应继续拆分（已有 `runner_team_compiler.go` / `runner_team_observer.go` / `runner_finish_steps.go` 等剥离尝试，但**主函数未真正使用提取后的 helper**——见 §4.3 死代码）。

### 4.2 命名与一致性

- **优点**：导出符号几乎都带任务/Phase ID（`BL-01`, `ARCH-01`, `M53 Phase 7`, `TG-RT-PARITY`），溯源极佳；`CompileToGraphBuildConfig` vs `CompileToGraphRuntimeConfig` 语义清晰区分（前者用于结构展示/快照，后者带 checkpoint/policy 给运行时）。
- **小瑕疵**：
  - `runner_team_trpc.go` import 分两块且把 `"strings"` `"time"` 夹在自定义包之间（违反 Go 标准库分组惯例）。
  - `definition.go` 字段标签缩进对齐错乱（`IntentAnchorAgentID`、`FailurePolicy` 与同列对不齐）。
  - `runner_team_trpc.go:154-156` 三个 `Memory*Recall` 字段对齐空格不一致。

### 4.3 死代码 / 测试-生产偏离（**新发现**）

| 符号 | 文件 | 性质 |
|------|------|------|
| `compileTeamRuntime` | `runner_team_compiler.go` | 提取出的 helper 从未被 `runner_team_trpc.go` 调用，**真正生产路径仍是 inline 实现** |
| `observerSetup.stopAll` | `runner_team_observer.go:89` | 定义但 caller 都是逐个 `defer stopXxx()`，永远不调用 stopAll |
| `boundedLoopIterations` / `loopMaxIterations` / `chunkParallelWorkers` | `builder.go` | 仅 `definition_test.go` 调用，生产无引用（native chain/parallel/cycle 由 trpc 框架管理） |
| `persistGraphMemberStepsFromResult` | `runner_finish_steps.go:49` | **只在 `parity_run_e2e_test.go` 出现**；生产 graph 路径用 `finalizeGraphRunStepsFallback`（anchor 兜底） + coordinator watch 写 step。E2E 测试因此测的是"幽灵路径" |
| `CleanupStaleSessions` | `team_graph_run_coordinator.go:394` | 已实现 2h 老化清理，但**全代码库无任何 cron / ticker / wire 调用** — GC 永不触发 |
| `runner_stream_opts.go` | 整文件 | 注释明确说 "to be removed once all callers inject the factory"，但仍直接 import `chatactivity`，且 commit 历史已有 *"refactor(team): 移除team包对chatactivity的直接导入"*，本意未达成 |

### 4.4 错误处理风格

- 模式：90%+ 使用 `event.CtxFlowLogWarn` + 容忍后继续，少数关键失败 `r.finishRunErr` 终结 run。
- 风险：
  - `ResumeExecution` 失败仅 `slog.Warn`（`HandleTeamGraphTaskCompleted`），未发 `team_run_failed` 事件，前端可能无感知。
  - `_ = c.teams.UpdateTeamRun(ctx, *run)`（`team_graph_run_coordinator.go:372`）— 仍存在两处 `_ =` 静默忽略 update 失败。

### 4.5 并发安全

- `TeamGraphRunCoordinator.sessions` 用 `sync.RWMutex` 保护读多写少；`graphStepDedup` 独立 mutex；`ActivityStepFlusher` 自带 channel + once。整体并发模型清晰，未见明显竞态。
- 小问题：`startGraphWatch` 内 `c.sessions[sess.execID] = sess` 在 unlock 之外读取 `sess.watchStop` 与下次写入 `sess.watchStop` 存在轻度数据竞争（写入路径加锁，读取在 watch goroutine 外不加锁）。`session` getter 加 RLock 后立即返回指针，调用方持有的指针字段访问无锁——长跑场景可能在 race detector 下复现。

### 4.6 测试质量

| 维度 | 评价 |
|------|------|
| 编译模式覆盖 | ★★★★★ 6 种模式全 |
| 灰度/开关 | ★★★★★ `graph_runtime_canary_test.go` 覆盖灰度桶稳定性 |
| Embedded graph | ★★★★ 含 cycle、subgraph、parallel join 用例 |
| Observers | ★★★★ projector / task bridge / exec tracker 都有 |
| 主路径 E2E | ★★ `parity_run_e2e_test.go` 测的是 §4.3 中的幽灵函数；缺真正 `runTeamTRPCFromInput` 集成 |
| HITL / Resume | ★★★ Coordinator 有单元测试，但缺 task completed → resume → graph done 的端到端用例 |
| Watch 超时 / eviction | ★ `defaultGraphWatchTimeout` 30 分钟 / `CleanupStaleSessions` / `evictSession` 无测试 |
| `failed` vs `error` | 0 已存在的不一致无回归保护 |

---

## 5. 架构与设计评估

### 5.1 依赖方向（红线核查）

| 红线 | 状态 |
|------|------|
| `internal/biz` 不 import `pkg/trpc-agent-go` / `trpc.group/...` | ✅ 验证：biz 树内 0 处 trpc import |
| Runner 装配在 `internal/service` | ✅ `internal/service/team.go` 持有 `*team.Runner`，team 包内仅暴露构造函数；service 通过 `TeamTurnRuntime`（biz/team_ports.go）端口反向调用 |
| Team 包向 trpc 框架直依赖 | ✅ 允许：team 是 trpc agent framework 的集成层（rule: trpc-agent-framework-first） |

### 5.2 端口/适配

- `biz.TeamTurnRuntime` / `TeamRunObserver` / `TeamBuildRunner` / `TeamRunnerHandle` 等接口齐全（biz/team_ports.go），实现散落在 service 与 team。
- `TeamGraphExecutionBackend` / `TeamGraphTaskCreator` / `TeamGraphRunFinisher` / `GraphBuildConfigLoader` — Coordinator 依赖接口而非具体类型，**可测性优秀**（test 文件使用 stub 全覆盖）。
- `TeamGraphRunFinisher` 通过 `SetFinisher` setter 注入（避免循环依赖），但 `Runner` 同时实现了该接口并被 setter 自反挂载——`graph_runtime_canary.go` / `provider.go` 这部分 wire 在 `service` 层完成，文档可再补一句 ASCII 序列图。

### 5.3 可扩展点

- **新增模式**：在 `graph_compile.go::compileEdges` switch 加一个 case + `SupportsTeamGraphRuntimeMode` 加键即可；`trpc_build.go` 同步加 native 路径。
- **新增节点类型**（如 `tool`, `webhook`）：扩展 `embedded_graph.go::isEmbeddedExecutableNode` + `compileFromEmbeddedGraph` 的 switch；但 `ShouldCreateTeamGraphTaskNode` 在 biz 层，跨包 - 已合理。
- **新增 observer**：`runner_team_observer.go::startObservers` 是单一入口，扩展友好（已为 5 种 observer 预留 `observerSetup` 结构）。
- **跨 Team 编排**：`TG-RT-PARITY` 仍在路上；`linked_graph_id` 已支持但 trace 关联弱（与 11-team-review 中 TEAM-P2-02 一致）。

### 5.4 设计气味

| 气味 | 位置 | 说明 |
|------|------|------|
| **God Object / God Function** | `runTeamTRPCFromInput` | 620 行单函数；已有局部 helpers 提取，但未投入使用 |
| **Stringly-typed status** | 全包 | 见 §3.4 |
| **隐藏裁剪** | `adaptiveMaxTransferEdges = 30` | N≥6 时 transfer overlay 静默丢失，编译期无 warning |
| **Magic timeout** | `defaultGraphWatchTimeout=30min`、`sessionMaxAge=2h`、`activityFlushBatchSize=10` | 全文件常量；未走配置中心 |
| **字符串解析协议** | `critic_loop` "approved" / `extractScore` 解析 LLM 输出 | LLM 措辞漂移即失效；缺结构化协议（function call/JSON schema） |
| **死路径** | `BuildTRPCTeam` 路径 | 已标 `// Deprecated`，但仍是 fallback 救生圈；fallback 决策树清晰，建议增加 metrics dashboard 监控 native 触发率 |
| **未完成重构** | `runner_team_compiler.go` / `runner_team_observer.go::stopAll` / `runner_stream_opts.go` | 提取出的代码未替换 inline；提交记录显示 refactor 在途 |

---

## 6. 问题清单（按优先级）

### P1 — 当前迭代应处理

| ID | 问题 | 影响 | 建议 |
|----|------|------|------|
| **TG-Q-01** | `run.Status` 字面量 `failed` / `error` 在 finishRunErr vs Coordinator finalize 不一致 | Monitor / Channel / Audit 过滤可能漏判一类失败 | 提到 `biz` 层定义 `TeamRunStatusFailed/Cancelled/...` 常量，team / service 全引用 |
| **TG-Q-02** | `CleanupStaleSessions` 永不触发 | 长跑进程下 Coordinator.sessions 仍可能积累 |  在 wire 装配处用 `time.Ticker(10*time.Minute)` + `safego.Go` 调度；或挂到现有 cron |
| **TG-Q-03** | `runTeamTRPCFromInput` 620 行未拆 | 维护性、回归测试成本高 | 真正调用已存在的 `compileTeamRuntime` 与 `observerSetup.stopAll`；把 intent / attachment / finalize 段进一步抽到 helper |
| **TG-Q-04** | `persistGraphMemberStepsFromResult` 仅测试可达 | parity E2E 测的是"幽灵函数"，对真实 graph watch 路径无保护 | 要么在生产中替代 `finalizeGraphRunStepsFallback`，要么删除并改写 E2E 直接驱动 `StartGraphStepWatch` |
| **TG-Q-05** | `runner_stream_opts.go` 仍 import `chatactivity` | 与既有 refactor 提交目标冲突，违反 team→service 单向依赖 | 将 fallback 构造迁到 `internal/service/chat_activity.go`，团队包仅持 `StreamOptsFactory` 接口 |

### P2 — 下一迭代

| ID | 问题 | 建议 |
|----|------|------|
| **TG-Q-06** | `adaptiveMaxTransferEdges=30` 静默裁剪 | 触发上限时 `event.CtxFlowLogWarn` + metrics counter |
| **TG-Q-07** | `critic_loop` "approved" 字符串解析脆弱 | 改为 escalation tool / function call schema |
| **TG-Q-08** | watch 超时与 eviction 无测试 | 增加 `time.AfterFunc` mock；覆盖 BL-02 / ARCH-02 修复 |
| **TG-Q-09** | `ResumeExecution` 失败静默 | 失败时发 `team_run_failed` envelope + FlowLog error |
| **TG-Q-10** | `definition.go` / `runner_team_trpc.go` import 格式 / 标签对齐 | `gofmt -s` / `goimports` 走一遍 |
| **TG-Q-11** | `builder.go` 死代码（仅测试可达） | 删除函数与对应测试，或在 native trpc 装配处真正接入 |

### P3 — 优化建议

| ID | 问题 | 建议 |
|----|------|------|
| **TG-Q-12** | 30 分钟 / 2 小时 / 批次大小 10 等魔法常量 | 移到 `internal/team` 配置结构或环境变量 |
| **TG-Q-13** | `recordTeamRunUsage` 表达式 `r==nil \|\| r.usage==nil \|\| promptTok<=0 && completionTok<=0` 优先级 | 显式括号 `(promptTok<=0 && completionTok<=0)` |
| **TG-Q-14** | `runner_team_observer.go::stopAll` 未使用 | 在 `runner_team_trpc.go` 用 `defer obs.stopAll()` 替换 5 个独立 defer |

---

## 7. 回归风险点（已修复但建议加测）

| 修复 ID（历史） | 风险 | 当前测试 |
|----------------|------|----------|
| BL-01 (HITL bulk skip) | Graph HITL 多写 step / 与事件冲突 | 间接覆盖，建议加端到端 |
| BL-02 (watch 超时 finalize) | 长跑 run 永久 `running` | 无超时单测 |
| BE-02 (output_preview 回填) | `enrichTeamRunMetricsFromSteps` ✅ 已测 | OK |
| ARCH-02 (Coordinator evict) | sessions map 泄漏 | 仅 `evictSession` 路径间接覆盖；`CleanupStaleSessions` 无测试 |
| BE-01 (PersistResumeGraphStep FlowLog warn) | 静默失败 | 无 |

---

## 8. 验证命令

```bash
# 编译 + parity + 单元
go test ./internal/team/... -count=1

# 仅关键路径
go test ./internal/team/... -run 'Compile|Parity|Coordinator|Projector|TaskBridge|Finisher' -count=1 -v

# race 检测（建议持续集成开启）
go test ./internal/team/... -race -count=1

# 全量
go build ./...
```

---

## 9. 业务逻辑优化空间与重设计草案

> 前文 §1–§8 聚焦"代码层"问题（命名、复杂度、死代码、测试覆盖）。本章关注**业务逻辑/领域建模**层面，提出 12 个优化点与对应的 redesign 方案。
> 每个优化点采用统一格式：**现状 → 问题本质 → 重设计 → 落地路径 → 影响面**。

### 9.1 BL-01 — 双轨架构（Graph/Native）已是历史包袱，建议彻底单轨化

**现状**：
- `trpc_build.go::BuildTRPCTeam` 注释明确 `// Deprecated: ... retained only for emergency fallback`；
- `DecideNativeFallback` 决策树要求 `ARANEA_TEAM_NATIVE=1` 或灰度 holdout 才使用；
- 但相关代码占据 `trpc_build.go`(273)、`fallback_policy.go`(85)、`graph_runtime_canary.go`(90)、`builder.go`(53) 合计 ~500 行，且 parity 测试持续耗费维护成本。

**问题本质**：
- 这是一份"我不敢删 native"的保险，但实际上灰度已 100%（`teamGraphCanaryPercent()` 默认 100）；
- 双轨产生**两套真相**：step 持久化策略不同、token 聚合口径不同、WS envelope 类型不同（见 `parity_run_test.go::graphOnlyEnvelopeTypes` / `nativeOnlyEnvelopeTypes`）；
- 任何模式调整都要写两遍并加 parity 测试，违反单一职责。

**重设计**：
```
现：Graph (default) → fallback → Native (chainagent / parallelagent / cycleagent / trpcteam)
目标：Graph (唯一) → 编译失败 → fail-fast 返回明确错误码

废弃路径：
  - 删除 BuildTRPCTeam / buildSwarmTeam / buildAdaptiveSwarm / buildCoordinatorOptions
  - 删除 fallback_policy.go / graph_runtime_canary.go
  - 简化 graph_runtime.go 为单一 SupportsTeamGraphRuntimeMode
  - 保留 ARANEA_TEAM_GRAPH_RUNTIME=0 紧急熔断（仅用于运维事故，不再支持自动降级）
```

**落地路径**（建议分 3 步，每步独立 PR）：
1. **观测**：在生产环境采集 `metrics.TeamGraphRuntimeTotal{path="native"}` 30 天，确认触发率 < 0.01%；
2. **下线**：将 `DecideNativeFallback` 返回值改为始终 `UseNative: false`，保留 30 天观察；
3. **删除**：删除 trpc_build.go / fallback_policy.go / graph_runtime_canary.go / builder.go / parity_*.go 中的 native 分支。

**影响面**：~600 行代码、~500 行测试可删；`runTeamTRPCFromInput` 主路径减少 5 个分支；消除字面量分裂源头之一。

---

### 9.2 BL-02 — 模式（mode）从枚举上升为模板（Template），消除分支膨胀

**现状**：
- 5 种业务模式 + swarm 别名 → 6 个 `compileXxxEdges` 函数（`compileSequentialEdges` / `compileParallelEdges` / `compileCoordinatorEdges` / `compileCriticLoopEdges` / `compileAdaptiveEdges`）；
- 每加一个新模式需要：①扩展 `SupportsTeamGraphRuntimeMode` 白名单 ②加 `compileXxxEdges` ③加 `CompileTemplateID` 映射 ④加 `applyAdaptiveAgentDestinations` 等 runtime 特化 ⑤加 parity fixture ⑥前端 buildGraphFromDefinition 同步。

**问题本质**：
- mode 是"业务概念"而非"图构造算法"——sequential / coordinator / critic_loop 都可以用同一套 `(NodeDef[], EdgeDef[])` 表达，仅是不同的预制拓扑；
- 当用户需要"sequential 但末节点条件回环"或"coordinator 但 worker 间也能 transfer"时，目前只能新增模式或走 embedded graph。

**重设计**：
```go
// 1. 抽出 OrchestrationTemplate 接口
type OrchestrationTemplate interface {
    ID() string                                              // "pipeline" / "dispatch" / "review_loop" / "parallel_review"
    Build(members []MemberDef, opts TemplateOpts) GraphSpec  // 直接产出 NodeDef+EdgeDef+条件边
}

// 2. 注册表
var templates = map[string]OrchestrationTemplate{
    "sequential":  pipelineTemplate{},
    "parallel":    parallelReviewTemplate{},
    "coordinator": dispatchTemplate{},
    "critic_loop": reviewLoopTemplate{},
    "adaptive":    swarmTemplate{},
}

// 3. 编译器只做模板查找
func compileToGraphBuildConfig(def Definition, ...) (biz.GraphBuildConfig, error) {
    tmpl, ok := templates[normalizeCompileMode(def.Mode)]
    if !ok { return cfg, ErrUnknownMode }
    spec := tmpl.Build(EnabledMembers(def), TemplateOpts{...})
    return spec.ToBuildConfig(), nil
}
```

**附带收益**：
- 用户自定义模板：插件可通过 `RegisterTemplate("my_workflow", impl)` 注入；
- 模板可参数化：`pipelineTemplate{LoopBack: true}` 即得到带 conditional retry 的 sequential；
- 删除 `compileEdges` switch + `compileEntryFinish` switch + `normalizeCompileMode` 的特殊化逻辑。

**落地**：1 个 PR 引入接口与注册表 + 5 个内置模板实现；保持向后兼容（mode 字符串不变）。

---

### 9.3 BL-03 — Coordinator 决策协议化（取消字符串解析）

**现状**：
- `critic_loop` 升级判定：`buildEscalationFunc` 扫描 LLM 输出找 `"approved"` 子串或 `{"score": X}` JSON；
- `coordinator` 由 trpc-team 的 member_tool 内部决策（黑盒）；
- `swarm` 用 `defaultSwarmHandoffInput` 拼字符串作为 transfer 输入。

**问题本质**：
- **LLM 文本协议是最脆弱的契约**——任何 prompt 微调、模型替换都可能破坏；
- 当前 critic_loop 用 `strings.Contains(content, "approved")` 判定，若 critic 说 "I cannot approve this" 也会判通过；
- `extractScore` 同时尝试数组与单对象 JSON，但若 LLM 把 score 写成 "0.85" 字符串则 0；
- 没有 round-trip 测试保证 LLM 输出符合协议。

**重设计**：
```go
// 1. critic / coordinator / swarm 统一通过 tool call 表达控制流
type orchestrationTool struct{}

func (orchestrationTool) Spec() ToolSpec {
    return ToolSpec{
        Name: "orchestration_control",
        Schema: jsonschema.Object{
            "action": {Enum: []string{"approve", "retry", "handoff", "escalate"}},
            "target": {Type: "string"},      // 仅 handoff 时使用
            "reason": {Type: "string"},
            "score":  {Type: "number"},      // 仅 critic 时使用
        },
    }
}

// 2. trpc graph 节点级别 escalation hook
type EscalationDecision struct {
    Action  string  // approve / retry / handoff
    Target  string
    Score   float64
    Reason  string
}

func criticLoopEscalation(ev *trpcevent.Event) EscalationDecision {
    // 从 tool_calls 中提取 orchestration_control 调用结果
    // 不再扫描 content 文本
}
```

**附带收益**：
- 可观测：决策动作直接写入 `team_run_steps.orchestration_decision_json`；
- 可重放：决策与文本解耦，rerun 测试只需重放 tool call；
- 跨模型稳定：tool call schema 是显式契约。

**落地**：1 个 PR 引入 orchestration tool + 改 critic / coordinator / swarm 三处决策点（保留旧路径 30 天）。

---

### 9.4 BL-04 — HITL 语义化：把"等待人类"从超时模型移出

**现状**：
```210:232:internal/team/team_graph_run_coordinator.go
		deadline := time.After(watchTimeout)
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-deadline:
				if mode == graphWatchStepsAndFinalize {
					c.finalizeTeamRun(watchCtx, sess, true, fmt.Sprintf("graph resume watch timed out after %s", watchTimeout))
				}
				return
```

`defaultGraphWatchTimeout = 30 * time.Minute` 一刀切：HITL 等人审批 30 分钟未完即标记 failed。

**问题本质**：
- HITL 的语义是"等待外部输入"，不是"等待程序响应"；
- 30 分钟对绝大多数人审场景**远远不够**（PR 审批、内容审核常以小时甚至天计）；
- 当前设计混用了两种超时：① graph 内部 step 推进的超时（合理） ② 人审等待的超时（不合理）；
- 失败后被 finishRunErr 写入 `task_dead_letter`，但其实只是"用户没及时来"，不应进死信。

**重设计**：
```
1. 拆分两类超时：
   - GraphProgressTimeout (5min)：graph 内部节点无事件推进 → failed
   - HITLSlaTimeout       (24h)：人审节点等待 → escalate(发通知) 而非 failed

2. 持久化 session 状态到 DB：
   - 新增表 team_graph_sessions(exec_id, team_run_id, state_json, last_activity_at, sla_at, ...)
   - Coordinator 重启时从 DB 恢复活跃 session（消除 BL-02 修复带来的进程重启数据丢失）

3. 状态机扩充：
   running → waiting_human (HITL 中断) → 三个分支：
       ① resumed → running          (人完成审批)
       ② escalated → waiting_human  (SLA 触发升级通知)
       ③ cancelled                  (用户/管理员显式取消)
       去除"超时 → failed"路径
```

**附带收益**：
- 解决 11-team-review 的 TEAM-P1-01（跨进程重启 summary 丢失）；
- Kanban 任务列表可显示 SLA 剩余时间；
- `CleanupStaleSessions` 改为基于 SLA 而非固定 2h，且由 cron 定时调度（解决 TG-Q-02）。

**落地**：1 个 schema 迁移 + 1 个 PR；watch 改为订阅 `team_graph_sessions` change feed 而非纯内存 map。

---

### 9.5 BL-05 — Step 持久化策略统一为事件驱动（消除"bulk vs event"双轨）

**现状**：
- Graph 路径：`StartGraphStepWatch` 订阅 `member_message_done` / `graph_node_end` / `team_step_finished` → `PersistGraphRunStep` 写一条 step；
- Native 路径（一旦走到）：`persistNativeBulkMemberSteps` 一次性写 N 条 step；
- 二者输出的 step 字段略有差异（native 把 anchor 的 token 全归 sortIdx=0，graph 按 MemberUsage 分配）。

**问题本质**：
- "bulk vs event" 不是业务需求，而是历史遗留；
- 单轨化后（BL-01）即可统一为事件驱动；
- 但即使保留双轨，bulk 也可以"伪装"成事件——native 也走 event bus emit `team_step_finished`，由统一 watcher 持久化。

**重设计**：
```
Producer 侧：
  - Graph 节点完成 → emit team_step_finished envelope (已有)
  - Native chain/parallel 子 agent 完成 → 同样 emit (新增)

Consumer 侧（唯一）：
  TeamStepWatcher 订阅 team_step_finished → persistStep

收益：
  - 删除 persistNativeBulkMemberSteps + persistGraphMemberStepsFromResult
  - parity 不再需要（产物天然一致）
  - 删除 finalizeGraphRunStepsFallback / ensureGraphRunStepsFallback 的"兜底"逻辑
```

**落地**：本项目依赖 BL-01 推进；若 BL-01 暂不能做，可独立完成事件驱动统一。

---

### 9.6 BL-06 — DefinitionJSON 一次解析，禁止重复 unmarshal

**现状**（搜 raw JSON 解析点）：
| 函数 | 解析对象 |
|------|---------|
| `ParseDefinition` | 主 Definition 结构 |
| `parseEmbeddedGraph` | `graph` 字段 |
| `LinkedGraphIDFromDefinition` | `linked_graph_id` 字段 |
| `parseOrchestrationCheckpoint` | `enable_checkpoint` 字段 |
| `collectGraphInterrupts` | `graph.nodes[*].interrupt_*` |
| `applyEmbeddedNodePolicies` | `graph.nodes[*]`（再次） |

同一份 `DefinitionSnapshotJSON` 在一次 turn 中被解析 **5–6 次**。

**问题本质**：
- 每次解析忽略错误（返回零值），可能在不同地方对同一份"坏 JSON"行为不一致；
- 修改 schema 时需要同步多处；
- 性能上每次 turn 多花 ~1-2ms（可忽略），但**正确性风险**显著。

**重设计**：
```go
// 1. 完整结构体（包含 graph / linked_graph_id / enable_checkpoint）
type OrchestrationSpec struct {
    Definition
    LinkedGraphID    string             `json:"linked_graph_id"`
    EnableCheckpoint *bool              `json:"enable_checkpoint"`
    Graph            *embeddedGraphSpec `json:"graph"`
}

// 2. 唯一入口
func ParseOrchestrationSpec(raw string) (OrchestrationSpec, error) {...}

// 3. 所有下游接收 OrchestrationSpec 而非 raw string
func CompileToGraphRuntimeConfigFromSpec(ctx, spec, agentKey, loader) (biz.GraphBuildConfig, error)
```

**附带收益**：
- 解析失败一次性暴露；
- 编译器签名清晰；
- 单元测试 fixture 直接构造 Go 结构体，不需写 JSON 字符串。

**落地**：1 个 PR；保留 `ParseDefinition` 兼容旧 caller，新增 `ParseOrchestrationSpec`。

---

### 9.7 BL-07 — 状态机显式建模（取代散落字符串）

**现状**：team_run.status 实际取值集合（通过全文搜索得出）：
```
running, success, failed, error, cancelled, pending, waiting_human
```
转换规则散落在 ~10 个赋值点，没有状态机表。

**重设计**：
```go
package biz

type TeamRunStatus string

const (
    TeamRunStatusPending       TeamRunStatus = "pending"
    TeamRunStatusRunning       TeamRunStatus = "running"
    TeamRunStatusWaitingHuman  TeamRunStatus = "waiting_human"
    TeamRunStatusSuccess       TeamRunStatus = "success"
    TeamRunStatusFailed        TeamRunStatus = "failed"
    TeamRunStatusCancelled     TeamRunStatus = "cancelled"
)

// 显式 transition 函数，拒绝非法迁移
func (s TeamRunStatus) Transition(target TeamRunStatus) error {
    valid := map[TeamRunStatus][]TeamRunStatus{
        Pending:      {Running, Cancelled},
        Running:      {WaitingHuman, Success, Failed, Cancelled},
        WaitingHuman: {Running, Success, Failed, Cancelled},
        // 终态不可迁出
    }
    ...
}

func (r *TeamRun) SetStatus(target TeamRunStatus) error {
    if err := r.Status.Transition(target); err != nil { return err }
    r.Status = target
    r.UpdatedAt = agent.RFC3339Now()
    return nil
}
```

**附带收益**：
- 解决 TG-Q-01（failed/error 分裂）；
- 误状态写入立即报错（防御性）；
- 文档自然生成：从 transition 表导出状态机图。

**落地**：本质是 TG-Q-01 的彻底版；建议合并 PR。

---

### 9.8 BL-08 — Token / Usage 记账双计语义澄清

**现状**：每次 team turn 写入：
- `recordMemberUsage` × N（每个 member step 一条 `usage_kind=team_member`）
- `recordTeamRunUsage` × 1（一条 `usage_kind=team_turn`，记录 anchor agent 的 promptTok+completionTok）

**问题本质**：
- 直觉上 `team_turn.total = sum(team_member)`，但**实际不等**：
  - `team_turn` 用 anchor agent 的 stream usage（首个 member）；
  - `team_member` 用 `MemberUsage` map 按 agent_key 分配；
  - 若 anchor 还作为某个 worker，token 被重复计入；
- 计费/配额视图选择哪个聚合？文档未明确。

**重设计**：
```
1. 明确语义：
   - team_member（per-step）：成员级实际消耗，作为对账与按 agent 计费依据；
   - team_turn  （per-turn） ：删除，或改为"聚合视图"（不另写一行，由 view 计算）；

2. 若必须保留 team_turn 行（监控大盘需要快速读取）：
   - InputTokens/OutputTokens = sum(team_member.tokens)
   - MetadataJSON 标注 source="aggregated_from_members"
   - usage_kind 改为 team_turn_summary
```

**附带收益**：
- 计费不重复；
- 用户在仪表盘上看到的"团队消耗"=各成员之和，符合直觉。

**落地**：1 个 PR；需要同步 Channel / Monitor 视图的查询脚本。

---

### 9.9 BL-09 — Observer Pipeline 统一为单订阅+多 handler

**现状**：4 个 observer 各自 `bus.Subscribe(SessionID=...)`：
- `OrchestrationStatusProjector` — 关心 envelope 类型几乎所有
- `TeamGraphTaskBridge` — 仅关心 `graph_node_start`
- `TeamGraphExecutionTracker` — 仅关心 `checkpoint`
- `StartGraphStepWatch` — 关心 `graph_node_end / team_step_finished / member_message_done`

每个订阅都创建独立的 goroutine + channel buffer + drop policy；
每个都过滤 `execution_id` 重复 3-4 次。

**重设计**：
```go
type teamRunPipeline struct {
    handlers []runEventHandler  // 按 envelope 类型路由
}

type runEventHandler interface {
    Interested(env event.Envelope) bool
    Handle(ctx context.Context, env event.Envelope)
}

// 单订阅，多 handler 分发
func (p *teamRunPipeline) Start(ctx, bus, sessionID, execID) context.CancelFunc {
    ch, unsub := bus.Subscribe(...)
    go func() {
        for env := range ch {
            if trackerMetaString(env.Metadata, "execution_id") != execID { continue }
            for _, h := range p.handlers {
                if h.Interested(env) { h.Handle(ctx, env) }
            }
        }
    }()
    return cancel
}
```

**附带收益**：
- bus 订阅数从 4 → 1，减少 fan-out 放大；
- `execution_id` 过滤集中一次；
- 启动顺序由 `handlers` 顺序确定，停止序列简化（取代 5 个独立 defer）。

**落地**：1 个 PR；测试可由现有 4 个 observer 测试合并。

---

### 9.10 BL-10 — Embedded Graph vs Mode 的"双重真相源"消除

**现状**：
```31:37:internal/team/graph_compile.go
	if spec, ok := parseEmbeddedGraph(rawDefinitionJSON); ok {
		cfg, err := compileFromEmbeddedGraph(ctx, def, spec, agentKey, loader)
		...
	}
	// 走 mode 编译
```

如果 `definition.graph` 存在且包含至少一个 executable 节点 → 用 embedded；否则用 mode。

**问题本质**：
- 用户从前端编辑后，UI 总是序列化 graph，导致 mode 实际上被忽略；
- 但 mode 仍参与 normalize（embedded_graph::compileEmbeddedEdges 用 mode 决定 edge kind）；
- 这种"半生效"语义难以预测：用户改了 mode 但没改 graph，行为不变；
- structure_export / compile_snapshot 走 mode 路径，runtime 走 embedded 路径 → 视觉与实际不一致。

**重设计**：
```
方案 A（推荐）：mode 退化为"模板生成器"
  - 用户选模式 → 后端立即生成 embedded graph 并写入 definition.graph
  - 编译永远只看 definition.graph
  - mode 字段保留但仅作 metadata（用于 UI 展示）

方案 B：彻底删除 mode
  - 仅保留 graph
  - 前端模板选择转为"插入预制 graph 节点集"
```

**附带收益**：
- 编译路径单一化；
- structure / runtime / snapshot 视图一致；
- mode 字段歧义消失。

**落地**：方案 A 优先，向后兼容好。需要 1 次数据迁移（把所有 team 的 mode 转写成 graph）。

---

### 9.11 BL-11 — 跨 Team / 子图 trace 链路打通

**现状**：
- `linked_graph_id` 支持引用持久化图；
- subgraph 节点可递归装配；
- 但 `parent_team_run_id` / `parent_trace_id` 不存在；
- Monitor / Observatory 无法显示"团队 A → 子团队 B → 子团队 C"的级联。

**重设计**：
```
TeamRun {
    ID, TeamID, SessionID, ...
    ParentTeamRunID    string  // 新增：上层 team 调用本团队时填充
    ParentGraphNodeID  string  // 新增：上层 graph 中触发本 run 的节点 id
    TraceID            string  // 与父 span link
}

// 在 service / chat_orchestrator 中：
//   if 当前是子团队调用：
//     childRun.ParentTeamRunID = parentRun.ID
//     emitter.LinkTo(parentSpan)
```

**落地**：1 次 schema 迁移 + 在 `runTeamTRPCFromInput` 入口注入父引用（通过 context 传递）。

---

### 9.12 BL-12 — Failure Policy 上升为独立策略引擎

**现状**：
- `Definition.FailurePolicy.Default / Retry / ParallelFail / CircuitBreaker / OnError` 各自影响 graph 编译；
- `applyTeamRuntimeExecutionOptions` 拼接 5 个策略；
- `OnError == "await_review"` 在 status projector 中改 status 显示为 `waiting_review`。

**问题本质**：
- failure policy 散布在 graph 编译期（retry/fallback）与 runtime 投影期（OnError）；
- 用户难以预测："retry_then_block + ParallelFail=continue + OnError=await_review" 三个组合的行为；
- 缺少 dry-run / 预览。

**重设计**：
```go
type FailurePolicyEngine interface {
    OnNodeError(ctx, node, attempt, err) FailureDecision
}

type FailureDecision struct {
    Action   string  // retry / skip / fallback / await_review / fail_run
    NextNode string  // 对 fallback / skip 有意义
    Delay    time.Duration  // 对 retry 有意义
}

// 编译期只标注哪些节点开启 policy；运行时所有失败统一由 engine 决策
```

**附带收益**：
- 可单元测试（输入 attempt+err，输出 Decision）；
- 用户能 dry-run 看到"重试 3 次后会做什么"；
- circuit breaker 状态机集中维护。

**落地**：依赖 biz.failure 改造，工作量较大；列入 P3。

---

### 9.13 业务逻辑优化优先级矩阵

| 优化点 | 业务价值 | 实施难度 | 风险 | 优先级 |
|--------|---------|----------|------|--------|
| BL-01 单轨化（删 native） | 高 | 中 | 中（需 30 天观察） | P1（本季度） |
| BL-02 模式→模板 | 中 | 中 | 低 | P2 |
| BL-03 协议化决策（critic/swarm） | 高 | 中 | 中（涉及 LLM prompt） | P1 |
| BL-04 HITL 语义化（SLA） | 高 | 高（schema 改） | 中 | P1 |
| BL-05 Step 事件驱动统一 | 中 | 低 | 低 | P2（搭 BL-01） |
| BL-06 DefinitionJSON 一次解析 | 中 | 低 | 低 | P2 |
| BL-07 状态机显式建模 | 高 | 低 | 低 | **P1（合并 TG-Q-01）** |
| BL-08 Usage 双计澄清 | 中 | 中 | 中（计费口径） | P2 |
| BL-09 Observer 单订阅化 | 低 | 低 | 低 | P3 |
| BL-10 mode→template 退化 | 高 | 高（数据迁移） | 中 | P2 |
| BL-11 跨 team trace 链 | 中 | 中 | 低 | P2 |
| BL-12 Failure Policy 引擎 | 中 | 高 | 中 | P3 |

---

### 9.14 推荐的两阶段重设计路线

#### 阶段一（1-2 个迭代，**最小可见收益**）

1. **BL-07** + **TG-Q-01**：合并为"状态机定义 + 字面量统一" — 1 个 PR、零业务风险、立刻收益；
2. **BL-06**：`ParseOrchestrationSpec` 一次性解析 — 1 个 PR、可直接降低 5 处 unmarshal；
3. **BL-03**：critic_loop 协议化（仅这一种模式，先打样） — 1 个 PR；
4. **TG-Q-02 + BL-04 部分**：把 `CleanupStaleSessions` 接入 cron，watchTimeout 拆分为 progress/HITL 两种 — 1 个 PR；

#### 阶段二（3-4 个迭代，**结构性整改**）

1. **BL-01 单轨化**：观察 + 下线 + 删除三步走，跨 60-90 天；
2. **BL-10 mode→template**：与前端协作落地编辑器输出；
3. **BL-04 完整版**：team_graph_sessions 持久化 + 进程重启恢复；
4. **BL-02 模板注册表**：在 BL-10 之后实施；
5. **BL-05 / BL-09**：搭 BL-01 顺势完成；

---

## 10. 总结

- **业务逻辑正确性高**：6 模式编译 + 灰度 fallback + HITL/resume 协调闭环已落地，单元测试在编译层、parity 层、灰度桶层均有覆盖。
- **架构边界基本干净**：biz / team / service 三层职责清楚，端口化（`TeamTurnRuntime` 等）符合 trpc-agent-framework-first 规则；红线无违反。
- **主要代码债集中在三处**（§4 / §6）：
  1. `runner_team_trpc.go` 620 行 God Function 未投入已提取的拆分 helper；
  2. 状态字面量未常量化导致 `failed/error` 字面分裂；
  3. 维护代码与测试代码出现**幽灵函数 / 永不触发的 GC**（`persistGraphMemberStepsFromResult` / `CleanupStaleSessions`）。
- **主要业务逻辑债集中在四处**（§9）：
  1. 双轨架构（Graph/Native）已过历史使命，建议单轨化（BL-01）；
  2. mode 与 embedded graph 双重真相源（BL-10）；
  3. HITL 超时混用了"程序响应"与"人审 SLA"语义（BL-04）；
  4. 关键决策依赖 LLM 文本解析（BL-03）。
- **建议**：本迭代解决 TG-Q-01..05 + BL-07/06/03 简化版（合并 4-5 个 PR）；下个季度推进 BL-01 单轨化与 BL-04 HITL 重设计。
