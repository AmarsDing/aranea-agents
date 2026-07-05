# 修复双重执行与 PlanStep AgentKeys 缺失问题

> **日期**：2026-07-05
> **类型**：Bug 修复计划
> **优先级**：P0（阻断 system-push 模式正确运行）
> **关联问题**：用户报告"11:41 时刻对话派出的团队-agent 看不明白，graph/计划列表状态更新也有问题"
> **关联文档**：
> - 需求：[1-chat.md](../../development/1-chat.md) §1.7.3 模式 C
> - 设计：[1-chat.design.md](../../development/1-chat.design.md) §B.10
> - 开发计划：[1-chat.development.md](../../development/1-chat.development.md) P-V2LF

---

## 一、问题背景

### 1.1 现象

用户在 11:41 时刻对话中观察到：
1. **团队-agent 看不明白**：3 个 team 都只有 1 个相同业务 agent（`__dept_lead_quant_trading__`），无法区分
2. **graph/计划列表状态更新异常**：PlanStep 和 GraphNode 状态被两条路径同时更新，产生冲突
3. **双重执行**：日志显示 10 个 team（Path A 6 个 + Path B 4 个），相同 step_id 重复

### 1.2 日志证据

| 时刻 | 路径 | 事件 |
|------|------|------|
| 11:36:26 | Path B | PlanExecutor 收到 PlanBoardCreatedEvent（steps=6） |
| 11:36:54 | Path A | TaskOrchestrator 开始 orchestration（6 allocations） |
| 11:36:54-11:37:01 | Path A | 创建 6 个 team（st_1~st_6，AutoStart=true） |
| 11:37:01-11:40:30 | Path A | 6 个 team 完成 |
| 11:41:25 | — | synthesis CAS 触发（Path A 完成） |
| 11:41:25 | Path B | dispatchStep st_6, st_2, st_4（新 team_ids） |
| 11:43:36-11:46:43 | Path B | 4 个 team 完成（结果被丢弃） |

---

## 二、根因分析

### 2.1 完整数据流图

```
用户消息 → Spirit LLM → plan_and_execute 工具（spirit_tools.go:83）
            │
            ├─ Phase 1: Plan (executePlanPhase)
            │   └─ planner.Plan()
            │       └─ publishPlanCreated
            │           ├─ v1 ActivityEvent
            │           └─ publishV2PlanBoard ← 【问题点 1】Phase 1 发布，allocPlan 不存在
            │               └─ seq.Publish(PlanBoardCreatedEvent)
            │                   │
            │                   ▼ 异步触发
            │               PlanExecutor.StartSubscription
            │               └─ Subscribe → dagRun.run → dispatchStep
            │                   └─ RealTeamOrchestrator.Orchestrate
            │                       └─ resolveAgentKeys ← 【问题点 2】查 DB，所有 team 同一 agent
            │
            ├─ Phase 2: Allocate (executeAllocatePhase)
            │   └─ allocator.Allocate(taskPlan) → allocPlan（含 AgentKey）
            │
            └─ Phase 3: Orchestrate (executeOrchestratePhase) ← 【问题点 3】双重执行
                └─ TaskOrchestratorImpl.orchestrateDAG
                    ├─ 使用 allocPlan（正确 agent）
                    ├─ AutoStart=true
                    └─ 不调用 MarkTeamDispatched（破坏 system-push）

[team 后台执行] → HandleTeamTurnResult
    ├─ checkAllTeamsCompleted → CAS 守卫 → synthesis turn #2
    └─ planExecutor.NotifyTeamCompletion → 唤醒 dispatchStep

[synthesis turn #2 完成] → OnTurnEnd
    └─ ParentTaskID != "" → 发 task.completed + ClearTeamDispatch
```

### 2.2 三个根本问题

| # | 问题 | 位置 | 后果 |
|---|------|------|------|
| 1 | `publishV2PlanBoard` 在 Phase 1 发布 | [task_planner_impl.go:1238](../../../internal/agent/task_planner_impl.go) | PlanStep 无法携带 AgentKeys（allocPlan 不存在） |
| 2 | `RealTeamOrchestrator.resolveAgentKeys` 查 DB | [team_orchestrator_real.go:165](../../../internal/service/team_orchestrator_real.go) | 所有 team 用同一 agent（`__dept_lead_quant_trading__`） |
| 3 | `executeOrchestratePhase` 在 Phase 3 创建 team | [spirit_tools.go:278](../../../internal/tools/spirit_tools.go) | 双重执行（10 team），破坏 system-push 模式 |

### 2.3 设计意图（基于 §B.10.1 + 代码分析）

system-push 模式的正确执行流：
1. Spirit turn #1 调用 `plan_and_execute`，发布 PlanBoardCreatedEvent 后**提前完成**（不等 team）
2. `PlanExecutor` 收到事件 → DAG 调度 → `dispatchStep` → `RealTeamOrchestrator.Orchestrate` 起 team
3. `MarkTeamDispatched(taskID)` 标记 → `OnTurnEnd` 跳过 `task.completed`
4. team 后台执行 → `checkAllTeamsCompleted` → synthesis turn #2
5. synthesis turn 完成 → 发 `task.completed` + `ClearTeamDispatch`

**结论**：`TaskOrchestratorImpl.orchestrateDAG`（Path A）是回归缺陷，应被废弃。`PlanExecutor`（Path B）是唯一执行器。

### 2.4 两条路径对比

| 维度 | Path A (TaskOrchestratorImpl) | Path B (PlanExecutor + RealTeamOrchestrator) |
|------|-------------------------------|----------------------------------------------|
| 入口 | `plan_and_execute` Phase 3 | `PlanBoardCreatedEvent` 订阅 |
| 接口签名 | `Orchestrate(ctx, *TaskPlan, *AllocationPlan)` | `Orchestrate(ctx, PlanStep, TeamStage)` |
| Agent 来源 | `allocPlan.Allocations`（正确） | `resolveAgentKeys` 查 DB（错误） |
| AutoStart | `true`（立即启动） | `false`（手动启动） |
| MarkTeamDispatched | **不调用**（破坏 system-push） | 调用（正确） |
| DAG 调度 | errgroup 并发（不等待依赖） | 反向邻接表 + checkDownstream（正确） |

---

## 三、修复方案

### 3.1 核心原则

- system-push 模式是设计意图（§B.10.1）
- PlanExecutor 是唯一执行器
- TaskOrchestratorImpl.orchestrateDAG 应被废弃

### 3.2 实施步骤（R5 小步快跑）

#### Step 1：禁用 Path A 的 team 创建（止血）

**目标**：立即停止双重执行

**修改文件**：`internal/tools/spirit_tools.go`

**改动**：
- `executeOrchestratePhase` 不再调用 `deps.orchestrator.Orchestrate(ctx, taskPlan, allocPlan)`
- 改为只保存 checkpoint（返回空 handle）
- team 创建完全交给 PlanExecutor

**验证**：
- 编译通过
- 运行时：发起复杂任务，确认只有 PlanExecutor 创建 team
- 日志：不再出现 "TaskOrchestrator: DAG path" 日志
- UI：team 数量正确（6 个而非 10 个）

#### Step 2：PlanStep 携带 AgentKeys 字段

**目标**：让 PlanStep 能携带 LLM 分配的 agent 信息

**修改文件**：

| 文件 | 改动 |
|------|------|
| `internal/biz/plan_step.go` | PlanStep struct 增加 `AgentKeys []string` |
| `internal/data/ent/schema/plan_step.go` | Schema 增加 AgentKeys 字段（JSON 类型） |
| `internal/data/plan_step_repo.go` | ent↔biz 转换函数更新 |
| `internal/data/sql/migrations/` | 新增迁移文件（增加 agent_keys 列） |

**验证**：
- `go generate ./internal/data/ent` 成功
- `make wire && make build` 通过
- 单测：PlanStep 序列化/反序列化正确

#### Step 3：publishV2PlanBoard 移到 Phase 2 之后

**目标**：PlanStep 携带正确的 AgentKeys

**修改文件**：

| 文件 | 改动 |
|------|------|
| `internal/agent/task_planner_impl.go` | 从 `publishPlanCreated` 移除 `publishV2PlanBoard` 调用；新增 `PublishV2Board(ctx, plan, allocPlan, ...)` 公开方法 |
| `internal/tools/spirit_tools.go` | Phase 2 之后调用 `planner.PublishV2Board`；从 `allocPlan.Allocations` 填充 `PlanStep.AgentKeys`；direct strategy 路径在 Phase 1 后直接发布（无 AgentKeys） |
| `internal/biz/task_planner_port.go` | 接口新增 `PublishV2Board` 方法（如存在） |

**验证**：
- 编译通过
- 运行时：PlanBoardCreatedEvent 在 Phase 2 之后发布
- 日志：PlanStep 携带 AgentKeys
- UI：PlanStep 显示正确的 agent

#### Step 4：RealTeamOrchestrator 使用 step.AgentKeys

**目标**：每个 team 使用 LLM 分配的正确 agent

**修改文件**：`internal/service/team_orchestrator_real.go`

**改动**：
- 优先使用 `step.AgentKeys`
- fallback 到 `resolveAgentKeys(ctx)`（保留兜底）
- 移除"PlanStep 本身不携带 AgentKeys"的注释

**验证**：
- 编译通过
- 运行时：每个 team 使用不同的 agent
- 日志：`Orchestrate 解析 AgentKeys` 显示 step.AgentKeys 的 agent
- UI：team 列表显示不同 agent，可区分

#### Step 5：清理 TaskOrchestratorImpl.orchestrateDAG

**目标**：移除死代码，避免混淆

**修改文件**：`internal/agent/task_orchestrator_impl.go`

**改动**：
- 搜索 `TaskOrchestratorPort.Orchestrate` 的所有调用点
- 若仅在 `spirit_tools.go` 调用（Step 1 已移除），标记 `orchestrateDAG` 为 Deprecated
- 或直接删除（如果确认无其他调用方）

**验证**：
- `Grep "orchestrateDAG"` 确认无调用
- 编译通过

#### Step 6：文档同步（DOC-SYNC）

**修改文件**：

| 文件 | 改动 |
|------|------|
| `docs/development/1-chat.design.md` | §B.10 新增 §B.10.8：PlanExecutor 唯一执行器设计 + PlanStep.AgentKeys 字段设计 |
| `docs/development/1-chat.development.md` | P-V2LF 新增任务条目 |

---

## 四、影响面分析

### 4.1 修改文件清单

| 文件 | Step | 改动类型 |
|------|------|----------|
| `internal/tools/spirit_tools.go` | 1, 3 | 修改 |
| `internal/biz/plan_step.go` | 2 | 修改 |
| `internal/data/ent/schema/plan_step.go` | 2 | 修改 |
| `internal/data/plan_step_repo.go` | 2 | 修改 |
| `internal/data/sql/migrations/` | 2 | 新增 |
| `internal/agent/task_planner_impl.go` | 3 | 修改 |
| `internal/biz/task_planner_port.go` | 3 | 修改（如存在） |
| `internal/service/team_orchestrator_real.go` | 4 | 修改 |
| `internal/agent/task_orchestrator_impl.go` | 5 | 修改 |
| `docs/development/1-chat.design.md` | 6 | 修改 |
| `docs/development/1-chat.development.md` | 6 | 修改 |

### 4.2 风险评估

| 风险 | 严重度 | 缓解措施 |
|------|--------|----------|
| direct strategy 路径丢失 v2 事件 | 中 | Step 3 中为 direct strategy 单独处理（Phase 1 后直接发布，无 AgentKeys） |
| Ent 生成失败 | 低 | Step 2 中先备份 schema，失败可回滚 |
| PlanExecutor 订阅时序变化 | 低 | PlanExecutor 通过 EventBus 订阅，事件发布时机变化不影响订阅逻辑 |
| TaskOrchestratorImpl 有其他调用方 | 低 | Step 5 中先 Grep 确认调用点，再决定删除或标记 Deprecated |

### 4.3 回滚方案

每个 Step 独立提交，若某步验证失败，可回滚该步的 commit 而不影响前序步骤。

---

## 五、验证清单

### 5.1 编译验证

```bash
go build ./...
make wire
```

### 5.2 单测验证

```bash
go test ./internal/biz/... -count=1
go test ./internal/service/... -count=1
go test ./internal/agent/... -count=1
go test ./internal/data/... -count=1
```

### 5.3 运行时验证（R3）

- 清空数据库
- 发起复杂任务（要求 3 个 team）
- 检查日志：
  - 不再出现 "TaskOrchestrator: DAG path"
  - 出现 "Orchestrate 解析 AgentKeys" 显示 step.AgentKeys
  - team 数量 = 3（而非 6 或 10）
- 检查 UI：
  - 3 个 team 显示不同 agent
  - PlanStep 状态正确更新
  - GraphNode 状态正确更新
  - synthesis turn 在所有 team 完成后触发

### 5.4 文档同步验证（DOC-SYNC）

- [ ] `1-chat.design.md` §B.10.8 新增
- [ ] `1-chat.development.md` P-V2LF 任务条目新增
- [ ] 代码锚点引用的文件路径真实存在
- [ ] 状态标记（✅/⏳/🟡/📋）反映代码真实状态

---

## 六、执行纪律

按 R5（小步快跑）原则：
1. 每个 Step 独立实施，完成后验证再进入下一步
2. 每个 Step 完成后运行对应验证清单
3. 若验证失败，立即回滚该 Step，分析原因后再实施
4. 全部 Step 完成后，进行完整的运行时验证（R3）
5. 完成后审查整个业务逻辑，反复检查和修改

---

## 七、变更历史

| 日期 | 变更 |
|------|------|
| 2026-07-05 | 初始版本 |
