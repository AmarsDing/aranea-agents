# Team + Graph 模块问题与方案

> 审查范围：`internal/team/`、`internal/graph/`、`internal/biz/`（Team/Graph 相关端口接口）
> 审查日期：2026-06-02
> 修订日期：2026-06-02（v2 — 整合行业调研、业务语义差距、CompiledTeam 方案）

---

## 目录

- [一、行业调研：Team 与 Graph 的业务定位](#一行业调研team-与-graph-的业务定位)
- [二、当前模块关系与协作模式](#二当前模块关系与协作模式)
- [三、业务语义差距分析](#三业务语义差距分析)
- [四、问题全景图](#四问题全景图)
- [五、P0 — 业务逻辑 Bug（必须修复）](#五p0--业务逻辑-bug必须修复)
- [六、P1 — 架构级缺陷（应该修复）](#六p1--架构级缺陷应该修复)
- [七、P2 — 设计级问题（建议修复）](#七p2--设计级问题建议修复)
- [八、P3 — 代码质量问题（优化项）](#八p3--代码质量问题优化项)
- [九、问题因果链与根因分析](#九问题因果链与根因分析)
- [十、目标架构：Team 是一等公民，Graph 是运行时投影](#十目标架构team-是一等公民graph-是运行时投影)
- [十一、CompiledTeam：连接现状与目标的关键桥梁](#十一compiledteam连接现状与目标的关键桥梁)
- [十二、渐进式演进方案（5 个 Milestone）](#十二渐进式演进方案5-个-milestone)
- [十三、方案可行性评估](#十三方案可行性评估)
- [十四、验证清单](#十四验证清单)

---

## 一、行业调研：Team 与 Graph 的业务定位

### 1.1 两大范式的本质区别

根据 Anthropic《Building Effective Agents》(2024)、Microsoft Agent Framework (2025)、AWS Strands Agents SDK (2025) 的最新实践，以及 MAN+ESM 论文（Adya Research, 2025），业界对多 Agent 系统的编排形成了清晰的两层范式：

```
┌─────────────────────────────────────────────────────────────────┐
│                   多 Agent 编排的两层范式                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Layer 2: Team（角色驱动的协作层）                                │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  关注点：WHO — 谁来做？角色分工、能力边界、协作协议       │    │
│  │  隐喻：人类团队 — 产品经理、架构师、工程师、QA            │    │
│  │  代表：CrewAI、AutoGen GroupChat、Microsoft Agent Team    │    │
│  │  核心价值：专业化分工 → 降低单个 Agent 的 prompt 复杂度   │    │
│  └─────────────────────────────────────────────────────────┘    │
│                          ↕ 编译                                 │
│  Layer 1: Graph（图驱动的工作流层）                              │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │  关注点：HOW — 怎么做？执行顺序、条件分支、状态流转       │    │
│  │  隐喻：状态机 — 节点、边、条件、检查点                    │    │
│  │  代表：LangGraph、Temporal、Microsoft Workflow            │    │
│  │  核心价值：确定性执行 → 可预测、可审计、可恢复             │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

**Anthropic 的核心论断**：Workflow（开发者编排路径）vs Agent（LLM 动态决定步骤）不是互斥的，而是**按需升级的谱系**：

| 维度 | Workflow (Graph) | Agent (Team) |
|------|------------------|--------------|
| 谁控制流程 | 开发者预先编排 | LLM 动态决定 |
| 适合场景 | 步骤可预判、需要稳定输出 | 开放任务、步骤不可预知 |
| 可预测性 | 高 | 低 |
| 灵活性 | 低 | 高 |
| 调试难度 | 低 | 高 |

### 1.2 行业最佳实践：Team 编译为 Graph

**LangGraph**（LangChain 团队，2024-2026）：

> 核心思想是**一切皆节点，一切皆边**。无论上层是角色驱动的 CrewAI 还是对话驱动的 AutoGen，底层都编译为 LangGraph 的 StateGraph 执行。Graph-based orchestration scales linearly — adding a new node is O(1) in complexity.

**Microsoft Agent Framework**（2025）：

> Workflow 是旗舰能力，将编排从简单线性流提升为**动态协作图**。Agent 是专业化执行单元，Workflow 是连接 Agent 的协作图。支持 Sequential/Concurrent/Conditional 三种模式。

**AWS Strands Agents**（2025）定义了四种模式：

| 模式 | 对应 Aranea | 说明 |
|------|------------|------|
| Agents as Tools | — | Manager Agent 调用专家 Agent |
| Swarm | Team.Swarm | 自由协作、handoff |
| Agent Graph | Team → Graph 编译 | 图驱动的精确控制 |
| Agent Workflow | Graph 独立使用 | 固定步骤的确定性流程 |

**关键洞察**：所有主流框架都采用了 **Team（声明式）→ Graph（执行式）** 的编译模式。用户关心"谁做什么"（Team），系统需要"怎么执行"（Graph）。

### 1.3 MAN+ESM 论文的关键警告

> "The graph executor holds the complete workflow state at runtime and is responsible for every transition; it cannot scale horizontally because it is the runtime. Agent capabilities are bound to graph nodes at design time, so adding a new capability requires rewriting and redeploying the graph. Past approximately fifty concurrent agents, latency grows roughly linearly with the agent count."

这个警告直指一个核心问题：**Team 的语义（角色、能力）与 Graph 的拓扑（节点、边）在编译时绑定，运行时无法动态调整**。Aranea 当前架构也存在这个问题。

---

## 二、当前模块关系与协作模式

### 2.1 职责分工

| 模块 | 定位 | 核心职责 |
|------|------|----------|
| **Team** (`internal/team/`) | 语义层 | 定义多 Agent 编排的**意图**——6 种模式（Sequential/Parallel/Coordinator/CriticLoop/Swarm/Adaptive）、成员角色、失败策略 |
| **Graph** (`internal/graph/`) | 执行引擎 | 提供图编排的**底层能力**——节点定义、条件边、并行分支、子图、HITL 中断、状态管理 |

### 2.2 协作模式

Team 和 Graph 遵循**编译-运行**两阶段模式：

```
Team Definition (mode + members + failure_policy)
    │
    ▼  CompileToGraphRuntimeConfig
biz.GraphBuildConfig (nodes + edges + conditional_edges)
    │
    ▼  BuildTeamGraphRoot()
GraphAgent (可运行的 trpcagent.Agent)
    │
    ▼  GraphAgent.Run()
图执行引擎 → 事件流 → TeamGraphRunCoordinator / TeamGraphTaskBridge
```

### 2.3 解耦机制

两者通过 biz 层端口接口解耦：

| 端口接口 | 定义位置 | 实现位置 | 作用 |
|----------|----------|----------|------|
| `GraphBuilderFactory` | `biz/graph_runtime.go` | `graph/adapter/` | Team 获取图构建能力 |
| `GraphRuntime` | `biz/graph_runtime.go` | `graph/adapter/` | 图运行/恢复/时间旅行 |
| `TeamCompiler` | `biz/team_compiler.go` | `team/` 包 | Graph 获取 Team 编译能力 |
| `TeamGraphRootBuilder` | `graph/adapter/team_graph_root.go` | `graph/adapter/` | 为 Team 构建图根节点 |
| `GraphBuildConfigLoader` | `team/graph_loader.go` | `graph/adapter/` | 加载关联图资产 |

### 2.4 Aranea 已经做对了什么

| 方面 | 评价 | 说明 |
|------|------|------|
| Team → Graph 编译方向 | ✅ 正确 | 与行业共识一致 |
| 6 种编排模式 | ✅ 丰富 | 覆盖 Sequential/Parallel/Coordinator/CriticLoop/Swarm/Adaptive |
| HITL 支持 | ✅ 完整 | interrupt + resume + SLA 超时 |
| 检查点 + 时间旅行 | ✅ 先进 | 前端可视化回退 |
| 原生路径回退 | ✅ 健壮 | Graph 编译失败时自动降级 |

---

## 三、业务语义差距分析

### 差距 1：Team 的"角色语义"在编译后丢失

**现状**：Team Definition → GraphBuildConfig（纯拓扑），角色名、能力描述、协作意图 → 编译后只剩 agentID + nodeID。

**行业做法**：
- CrewAI 的 `Agent.backstory` 在运行时仍然可查
- LangGraph 的 State 保留完整的消息历史和角色标签
- Microsoft 的 Agent 保留 Identity + Capabilities

**后果**：
- 前端无法展示"哪个角色在执行"（只能展示 nodeID）
- 日志中无法追溯"角色决策路径"
- 运行时无法动态调整角色分配

### 差距 2：Graph 缺少"运行时角色解析"能力

**现状**：Graph 节点在编译时绑定到具体 Agent，`node.AgentName = "reviewer"` → 编译时解析为具体 Agent 实例。

**行业做法**：
- LangGraph 支持运行时动态选择 Agent
- CrewAI 的 Hierarchical 模式支持 Manager 动态分配 Task
- MAN+ESM 支持运行时 Capability Discovery

**后果**：
- 同一个 Graph 不能复用于不同 Team 配置
- Agent 故障时无法自动切换到备选 Agent
- 无法实现"按能力匹配"的动态调度

### 差距 3：Team 和 Graph 的生命周期不匹配

**现状**：Team 编译为 Graph 后，Team 定义不再参与运行时。FailurePolicy 编译展开后原始策略对象被丢弃。TeamRun 状态由 Coordinator 管理，与 Team Definition 脱钩。

**行业做法**：
- LangGraph 的 State 保留完整的 Workflow 上下文
- Microsoft 的 Agent Identity 贯穿执行全生命周期
- Temporal 的 Workflow 定义可运行时查询

**后果**：
- 运行时无法回答"这个 Team 的原始意图是什么"
- 无法支持"运行时修改 Team 配置并重新编译"
- 故障恢复时无法回溯到 Team 语义层

### 差距 4：缺少"Team 作为一等公民"的 API

**现状**：Team 的 CRUD 和执行是分离的。TeamUsecase 只做 CRUD，Team Runner 只做执行。没有统一的"Team 生命周期管理"。

**行业做法**：
- CrewAI 的 Crew 是一等对象（定义 + 执行 + 状态）
- Microsoft 的 Agent + Workflow 是统一实体
- LangGraph 的 CompiledGraph 包含定义 + 运行时

---

## 四、问题全景图

### 4.1 严重度分布

| 严重度 | 数量 | 说明 |
|--------|------|------|
| **P0** | 7 | 业务逻辑 Bug / 运行时 panic / 数据丢失 |
| **P1** | 10 | 架构级缺陷 / 竞态条件 / 状态不一致 |
| **P2** | 12 | 设计级问题 / 逻辑缺陷 / 边界情况 |
| **P3** | 11 | 代码质量 / 资源泄漏 / 死代码 |

### 4.2 问题因果链

```
GraphBuildConfig 承载过多关注点（7 个耦合点）
  ↓
TeamFailurePolicy 泄漏进 Graph → trpc 反向依赖 biz
  ↓
biz/trpc 类型双源定义（trpc 不想依赖 biz，但又需要 FailurePolicy）
  ↓
双源定义需要手工映射 → 维护风险极高
  ↓
映射复杂度 → GraphBuilderFactory 膨胀到 10 方法
  ↓
Factory 膨胀 → GraphUsecase 承担过多职责
  ↓
Usecase 复杂 → Runner 需要大量依赖 → God Object + Setter 注入
  ↓
Setter 注入绕过 Wire → 编译期保证丧失 → 运行时 Bug 频发
```

### 4.3 7 个耦合点详解

Team↔Graph 之间存在 **7 个耦合点**，而非仅 `FailurePolicy` 和 `ParallelBranchIDs`：

| # | 耦合点 | 说明 |
|---|--------|------|
| ① | `GraphBuildConfig.FailurePolicy` | Team 策略对象直接嵌入 Graph 配置 |
| ② | `GraphBuildConfig.ParallelBranchIDs` | Team 编译中间产物嵌入 Graph 配置 |
| ③ | NodeDef 的 8 个 Task 字段 | graph/trpc 层零消费，只是透传；真正消费者是 biz 层 Task 协调逻辑 |
| ④ | NodeDef 的 3 个 Failure 字段 | 由 `ApplyFailurePolicy` 展开，graph/trpc 层直接消费 |
| ⑤ | `biz.SkipNodeFuncRef`/`SkippedNodesStateKey` 等共享常量 | graph/trpc 直接引用 biz 常量 |
| ⑥ | `EdgeDef.Kind` 语义耦合 | Team 编译定义（transfer/flow/dispatch），被 FailurePolicy 逻辑依赖 |
| ⑦ | `graph/trpc` 包 import `biz.TeamFailurePolicy` | 违反依赖方向 |

---

## 五、P0 — 业务逻辑 Bug（必须修复）

### BUG-01：Critic Loop 否定语义误匹配

**文件**：`internal/graph/adapter/critic_loop_cond.go:36`

**现状**：`containsWord(content, "approved")` 无法处理否定语义。`"not approved"` 会匹配为通过。

**修复方案**：增加否定词检测 `containsNegationBeforeWord(content, "approved")`，优先依赖 `OrchestrationControlTool` 的结构化输出，将自然语言匹配降级为 fallback。

### BUG-02：`finishRunErr` 双重事件发布

**文件**：`internal/team/runner_helpers.go:110-121`

**现状**：失败时同时发布 `TeamRunFinished` 和 `TeamRunFailed`。

**修复方案**：失败时只发布 `TeamRunFailed`。

### BUG-03：嵌入式图编译入口点非确定性

**文件**：`internal/team/embedded_graph.go:193-198`

**现状**：`for id := range executableIDs` 使用 map 迭代，顺序不确定。

**修复方案**：排序后取第一个：`sort.Strings(ids); entry = ids[0]`。

### BUG-04：Fallback Agent 绕过正常解析管线

**文件**：`internal/graph/trpc/failure_recovery.go:33-35`

**现状**：fallback agent 直接 `NewAgentNodeFunc(fallback)`，不经过 `AgentResolver`。

**修复方案**：通过 `deps.Agents.ResolveAgent(ctx, fallback)` 走完整解析管线。

### BUG-05：`buildResumeSessionContext` 硬编码 "key-" 前缀

**文件**：`internal/team/team_graph_run_finisher.go:128`

**现状**：`"key-" + agentID` 与运行时 `ag.AgentKey` 不匹配，HITL 恢复后步骤持久化失败。

**修复方案**：从 catalog 获取真实 AgentKey，与运行时路径保持一致。

### BUG-06：`convertTrpcEvent` 传 nil logger 导致 panic

**文件**：`internal/graph/adapter/runtime_adapter.go:212`

**现状**：`NewEventBridge(bus, sessionID, graphID, execID, nil)` 硬编码 nil，JSON 反序列化失败时空指针 panic。

**修复方案**：传入 `lg` 参数。

### BUG-07：EventBridge 每事件重建，ExecutionSummaryTracker 永远为空

**文件**：`internal/graph/adapter/runtime_adapter.go:210-213`

**现状**：每次 `convertTrpcEvent` 都 `NewEventBridge`，summary 只包含最后一个节点。

**修复方案**：在 `trpcGraphRuntime` 中持有单一 `EventBridge` 实例。

---

## 六、P1 — 架构级缺陷（应该修复）

### ARCH-01：Runner God Object + 12 个 Setter 注入绕过 Wire

**文件**：`internal/team/runner.go`

**问题**：20 个字段、6 种职责、12 个 `Set*` 方法绕过 Wire 编译期检查。其中 Runner ↔ TeamGraphRunCoordinator 存在结构性双向依赖（Runner 启动 Graph 执行，Coordinator 回调 Runner 持久化结果），Setter 是解决构造期循环的标准手段，但 12 个 Setter 中只有 2 个是循环依赖导致的。

**修复方案**：同包拆分 + `RunnerConfig` struct 替代 10 个非循环 Setter + `TeamRunMediator` 解决双向绑定。

### ARCH-02：GraphBuildConfig 上帝结构体 + TeamFailurePolicy 泄漏

**文件**：`internal/biz/graph.go:105-120`

**问题**：13 个字段混合图拓扑、Team 领域概念、运行时行为。`FailurePolicy *TeamFailurePolicy` 形成 Graph→Team 反向依赖。

**修复方案**：编译期展开——`FailurePolicy` 展开为 `NodeDef.FailureAction/FallbackAgent/RetryMaxAttempts` 后丢弃原始策略对象；`ParallelBranchIDs` 展开为 `FailureAction=skip_on_failure` 后丢弃。引入 `CompiledTeam` 承载 Team 语义。

### ARCH-03：biz/trpc 类型双源定义 + 手工映射

**文件**：`internal/biz/graph.go` + `internal/graph/trpc/builder.go` + `internal/graph/adapter/runtime_adapter.go`

**问题**：6 组类型双源定义，`bizCfgToTrpc`/`trpcCfgToBiz` 约 100 行手工映射，遗漏即 bug（如 `EdgeDef.Kind` 已丢失）。

**修复方案**：biz 层定义唯一真相源，trpc 层嵌入 + 类型别名。可行性已验证：biz 不引入 trpc 依赖（合规），trpc 向内依赖 biz（合规），`SubgraphDef.BuildConfig` 递归类型通过 `GraphBuildConfig = biz.GraphBuildConfig` 类型别名规避。

### ARCH-04：GraphUsecase 上帝 Usecase

**文件**：`internal/biz/graph.go:200-211`

**问题**：8 个职责集于一身。`teamBuildConfigs` 内存缓存无持久化，进程重启后 Team Graph 无法恢复。

**修复方案**：拆分为 `GraphDefinitionUsecase` + `GraphExecutionUsecase` + `GraphCacheManager`。`teamBuildConfigs` 替换为持久化的 `CompiledTeam`。

### ARCH-05：GraphBuilderFactory 10 方法违反 ISP + 红线 #15

**文件**：`internal/biz/graph_runtime.go:37-48`

**修复方案**：拆分为 `GraphRunnerFactory`（3 方法）+ `GraphVisualizer`（1 方法）+ `GraphValidator`（1 方法）+ `GraphTemplateProvider`（3 方法）+ `GraphNodeInfoProvider`（2 方法）。

### ARCH-06：Circuit Breaker 全局可变状态

**文件**：`internal/graph/trpc/circuit_breaker.go:26-29`

**问题**：包级全局 map，无界增长，跨会话/跨租户污染。

**修复方案**：绑定到 `GraphAgent` 生命周期。

### ARCH-07：TOCTOU 竞态——释放锁后写 DB

**文件**：`internal/biz/graph_execution.go:304-315`

**修复方案**：锁内拷贝数据，锁外写 DB。

### ARCH-08：GC 标记 failed 不取消 runtime

**文件**：`internal/biz/graph.go:291-296`

**修复方案**：GC 驱逐前先 `exec.runtime.Cancel()`。

### ARCH-09：`teamBuildConfigs` 驱逐导致 resume 失败

**文件**：`internal/biz/graph_execution.go:22-46`

**问题**：`team:` 前缀的 graph 不是持久化 asset，GC 驱逐后 `buildConfigForExecution` 回退失败。

**修复方案**：`CompiledTeam` 持久化到 DB。

### ARCH-10：`MarkTeamGraphInterrupt` 修改共享指针竞态

**文件**：`internal/biz/graph_team_execution.go:68-75`

**修复方案**：为每个 `GraphExecution` 增加独立的 `sync.RWMutex`。

---

## 七、P2 — 设计级问题（建议修复）

| 编号 | 问题 | 文件 |
|------|------|------|
| DES-01 | NodeDef 28 字段 God Object（4 个关注点混合） | `biz/graph.go:47-76` |
| DES-02 | 大量 `any` 返回值丧失类型安全 | `biz/graph_runtime.go` |
| DES-03 | `TeamAgentHelper` 8 方法"工具箱接口"违反 ISP | `biz/team_agent_ports.go` |
| DES-04 | `TeamTurnRuntime` 与 `TeamBuildRunner` 功能重叠 | `biz/team_ports.go` |
| DES-05 | `TeamTurnResult` 包含 `ChatMessage`——跨领域耦合 | `biz/team_ports.go` |
| DES-06 | `GraphRepo` 7 方法违反红线 #15 | `biz/graph.go:178-186` |
| DES-07 | `GraphRuntime` 混合执行控制与检查点 | `biz/graph_runtime.go:13-22` |
| DES-08 | FunctionResolver 已定义但未接入 wireNode（死代码 + 功能缺失） | `graph/adapter/resolver_function.go` |
| DES-09 | Deprecated `BuildDeps` 仍是主路径（迁移半成品） | `graph/trpc/build_deps.go` |
| DES-10 | `CompileToGraphRuntimeConfig` 静默忽略 linked graph 加载错误 | `team/graph_runtime_config.go:28-31` |
| DES-11 | `criticLoopCondFunc` 只检查最后一条消息 | `graph/adapter/critic_loop_cond.go:23` |
| DES-12 | `compileFromEmbeddedGraph` 不检测重复 node ID | `team/embedded_graph.go:125-186` |

---

## 八、P3 — 代码质量问题（优化项）

| 编号 | 问题 | 文件 |
|------|------|------|
| Q-01 | `SQLiteCheckpointSaver.Close()` 空实现 | `graph/trpc/checkpoint.go:80-82` |
| Q-02 | 事件桥 goroutine 可能泄漏 | `graph/adapter/runtime_adapter.go:90-99` |
| Q-03 | `startGraphWatch` 在锁外修改 session | `team/team_graph_run_coordinator.go:277-282` |
| Q-04 | `ConditionalEdgeDef` 的 `CondFunc` 为 nil 时无构建期校验 | `graph/trpc/builder.go:268-270` |
| Q-05 | `ActivityStepFlusher.Enqueue` 静默丢弃 | `team/activity_step_flusher.go:79-82` |
| Q-06 | Coordinator 清理 goroutine 用 `context.Background()` 无 stop | `team/provider.go:23` |
| Q-07 | 环境变量运行时读取 | `team/graph_runtime.go` |
| Q-08 | Coordinator nil 接收者静默成功 | `team/team_graph_run_coordinator.go` |
| Q-09 | HITL SLA 缩进错误 + 超时消息不准确 | `team/team_graph_run_coordinator.go:308-319` |
| Q-10 | 中英文混合日志 + 非结构化错误码 | 多处 |
| Q-11 | 残留 `.tmp/.recovered` 文件 | `internal/team/` |

---

## 九、问题因果链与根因分析

### 9.1 根因

所有问题的根因可以追溯到**一个设计决策**：`GraphBuildConfig` 作为 Team↔Graph 的唯一传输载体，承载了过多关注点。

```
GraphBuildConfig 承载了：
  ├── 图拓扑信息（Nodes/Edges/EntryPoint）         ← Graph 领域
  ├── Team 领域概念（FailurePolicy/ParallelBranchIDs） ← Team 领域
  ├── Task 管理元数据（NodeDef 中的 RequiredRole 等）  ← Task 领域
  └── 运行时行为（InterruptBefore/After/Checkpoint）   ← 运行时关注点
```

### 9.2 原方案评估：TeamGraphExtras 不能达到最优解

原方案（仅移动 `FailurePolicy` + `ParallelBranchIDs` 到 `TeamGraphExtras`）存在 3 个根本性缺陷：

1. **只覆盖了 2/7 的耦合点**——NodeDef 的 Task 字段、Failure 字段、共享常量、EdgeDef.Kind 语义均未处理
2. **FailurePolicy 的"展开"语义被忽略**——`ApplyFailurePolicy` 将策略展开写入 NodeDef 字段，Graph 运行时消费的是展开后的字段值，移走原始策略对象不消除耦合
3. **Runner ↔ Coordinator 的双向依赖是结构性的**——简单拆分 Runner 不会消除双向依赖

**结论**：原方案是形式上的解耦，不是实质上的解耦。真正的解耦需要将 Team 语义在编译阶段完全展开为 Graph 通用的 NodeDef 字段，并分离 Task 元数据。

---

## 十、目标架构：Team 是一等公民，Graph 是运行时投影

### 10.1 核心原则

**"Team 是一等公民，Graph 是 Team 的运行时投影"**——与 LangGraph、Microsoft Agent Framework、AWS Strands Agents 的行业共识一致。

```
┌──────────────────────────────────────────────────────────────────────┐
│                  Team 与 Graph 的理想协作模型                         │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Team（一等公民）                                                     │
│  ┌─────────────────────────────────────────────────────────────┐     │
│  │  定义：角色 + 协作模式 + 策略 + 约束                         │     │
│  │  生命周期：创建 → 配置 → 编译 → 执行 → 监控 → 调整 → 重编译  │     │
│  │  状态：完整的 Team 上下文（定义 + 编译产物 + 运行时状态）     │     │
│  └──────────────────────────────┬──────────────────────────────┘     │
│                                 │                                    │
│                    编译（可重复、可缓存）                              │
│                                 │                                    │
│                                 ▼                                    │
│  Graph（Team 的运行时投影）                                          │
│  ┌─────────────────────────────────────────────────────────────┐     │
│  │  拓扑：节点 + 边 + 条件 + 状态                               │     │
│  │  角色：确定性执行 + 检查点 + 恢复 + 时间旅行                 │     │
│  │  约束：不持有 Team 语义，只消费编译后的通用字段               │     │
│  └─────────────────────────────────────────────────────────────┘     │
│                                                                      │
│  关键约束：                                                          │
│  1. Team 编译为 Graph 后，Team 定义仍然可查（不丢弃）                │
│  2. Graph 不直接依赖 Team 类型（编译后只消费通用字段）                │
│  3. Team 可以在运行时重新编译（配置变更 → 重编译 → 新 Graph）        │
│  4. Graph 可以独立使用（不绑定 Team，如可视化编辑器创建的 Graph）     │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

### 10.2 三层架构模型

```
┌──────────────────────────────────────────────────────────────────────┐
│                       三层架构模型                                    │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Layer 3: Team Layer（业务语义层）                                    │
│  ┌──────────────────────────────────────────────────────────────┐    │
│  │  TeamDefinition: 角色 + 模式 + FailurePolicy + 约束          │    │
│  │  TeamRun: 执行实例 + 状态 + 步骤 + 上下文                     │    │
│  │  TeamCompiler: TeamDefinition → CompiledTeam                  │    │
│  │                                                               │    │
│  │  CompiledTeam = {                                             │    │
│  │    GraphBuildConfig,     // 纯图拓扑（给 Graph 用）           │    │
│  │    TaskMeta,             // Task 元数据（给 Task 协调用）      │    │
│  │    RoleManifest,         // 角色清单（给前端/日志用）          │    │
│  │    OriginalPolicy,       // 原始策略（给运行时查询用）         │    │
│  │  }                                                            │    │
│  └──────────────────────────────────────────────────────────────┘    │
│                          │                                           │
│                          ▼                                           │
│  Layer 2: Graph Layer（执行引擎层）                                   │
│  ┌──────────────────────────────────────────────────────────────┐    │
│  │  GraphBuildConfig: 纯图拓扑（节点 + 边 + 条件 + 状态）       │    │
│  │  GraphAgent: 编译后的可执行图                                 │    │
│  │  GraphRuntime: Run/Resume/Cancel/Checkpoint                  │    │
│  │                                                               │    │
│  │  不持有：TeamFailurePolicy、RoleManifest、NodeTaskMeta        │    │
│  │  只消费：NodeDef.FailureAction/FallbackAgent/RetryMaxAttempts │    │
│  └──────────────────────────────────────────────────────────────┘    │
│                          │                                           │
│                          ▼                                           │
│  Layer 1: Agent Layer（执行单元层）                                   │
│  ┌──────────────────────────────────────────────────────────────┐    │
│  │  Agent: LLM + Tools + Memory + Identity                      │    │
│  │  AgentResolver: 运行时动态解析 Agent 引用                     │    │
│  │  AgentRegistry: Agent 能力注册与发现                           │    │
│  └──────────────────────────────────────────────────────────────┘    │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

### 10.3 与当前架构的关键差异

| 维度 | 当前架构 | 目标架构 | 差距 |
|------|----------|----------|------|
| Team 定义在运行时的可见性 | 编译后丢弃 | **保留在 CompiledTeam 中** | 需要新增 `CompiledTeam` 结构 |
| Graph 对 Team 类型的依赖 | `FailurePolicy *TeamFailurePolicy` | **零依赖**——只消费通用 NodeDef 字段 | 需要编译期展开 |
| NodeDef 的职责 | 图拓扑 + Task + Failure 混合 | **纯图拓扑 + 通用 Failure 字段** | 需要分离 NodeTaskMeta |
| Team 的生命周期 | CRUD（Usecase）和执行（Runner）分离 | **统一生命周期**——定义→编译→执行→监控 | 需要重构 Runner |
| Graph 的独立性 | 必须通过 Team 编译路径使用 | **可独立使用**（可视化编辑器创建） | 已部分支持 |
| 运行时角色解析 | 编译时绑定到具体 Agent | **运行时动态解析**（AgentResolver） | 需要新增能力 |

---

## 十一、CompiledTeam：连接现状与目标的关键桥梁

### 11.1 核心设计

```go
type CompiledTeam struct {
    GraphBuildConfig                        // 纯图拓扑（Graph 运行时消费）
    TaskMeta       map[string]NodeTaskMeta  // nodeID → Task 元数据（Task 协调逻辑消费）
    RoleManifest   map[string]RoleInfo      // nodeID → 角色信息（前端/日志消费）
    OriginalPolicy *TeamFailurePolicy       // 原始策略（可查但 Graph 不消费）
}

type RoleInfo struct {
    AgentID      string
    AgentKey     string
    DisplayName  string
    Role         string    // worker/reviewer/coordinator/synthesizer
    Capabilities []string  // 预留：运行时动态调度
}

type NodeTaskMeta struct {
    RequiredRole             string
    AssignmentMode           string
    AssignmentStrategy       string
    ReviewerAgent            string
    ReviewRules              string
    TimeoutSeconds           int
    HeartbeatIntervalSeconds int
    EnableLeaseExtension     bool
}
```

### 11.2 关键约束

1. `GraphBuildConfig` 不再包含 `FailurePolicy` 和 `ParallelBranchIDs`
2. `FailurePolicy` 在编译阶段展开为 `NodeDef.FailureAction/FallbackAgent/RetryMaxAttempts`
3. `ParallelBranchIDs` 在编译阶段展开为 `NodeDef.FailureAction=skip_on_failure`
4. `CircuitBreaker` 通过编译阶段展开为 `RetryMaxAttempts`
5. `NodeTaskMeta` 从 `NodeDef` 中分离，通过 `CompiledTeam.TaskMeta` 传递
6. `RoleManifest` 解决"角色语义在编译后丢失"的问题
7. `OriginalPolicy` 保留原始策略用于日志和调试，但 Graph 运行时不消费

### 11.3 数据流变化

```
修复前：
  Team Definition
    → CompileToGraphRuntimeConfig
    → GraphBuildConfig(含 FailurePolicy + ParallelBranchIDs + NodeDef 含 Task 字段)
    → BuildTeamGraphRoot
    → GraphAgent

修复后：
  Team Definition
    → CompileToCompiledTeam
    → CompiledTeam {
        GraphBuildConfig(纯拓扑 + 通用 Failure 字段),
        TaskMeta(nodeID → Task 元数据),
        RoleManifest(nodeID → 角色信息),
        OriginalPolicy(可查但 Graph 不消费)
      }
    → BuildTeamGraphRoot(compiled.GraphBuildConfig)
    → GraphAgent

  Task 协调逻辑消费 compiled.TaskMeta（不经过 Graph）
  前端/日志消费 compiled.RoleManifest（不经过 Graph）
  故障恢复查询 compiled.OriginalPolicy（不经过 Graph）
```

### 11.4 CompiledTeam 如何解决所有问题

| 问题 | 解决方式 |
|------|----------|
| ARCH-02: GraphBuildConfig 上帝结构体 | 拆分为 `GraphBuildConfig`（纯拓扑）+ `CompiledTeam`（Team 语义） |
| ARCH-03: biz/trpc 类型双源定义 | `GraphBuildConfig` 简化后，trpc 层用嵌入 + 类型别名 |
| ARCH-04: GraphUsecase 上帝 Usecase | `teamBuildConfigs` 替换为 `CompiledTeam`，Team 注册逻辑归入 Team 层 |
| ARCH-05: GraphBuilderFactory 10 方法 | `GraphBuildConfig` 简化后，Factory 方法数自然减少 |
| DES-01: NodeDef 28 字段 | Task 字段移到 `NodeTaskMeta`，NodeDef 降至 20 字段 |
| BUG-05: 恢复路径硬编码 key | `RoleManifest` 提供真实 AgentKey/DisplayName |
| ARCH-09: teamBuildConfigs 驱逐 | `CompiledTeam` 持久化到 DB，不再依赖内存缓存 |
| 差距 1: 角色语义丢失 | `RoleManifest` 保留角色信息 |
| 差距 2: 运行时角色解析 | `RoleManifest.Capabilities` 为未来动态调度预留 |
| 差距 3: 生命周期不匹配 | `CompiledTeam` 作为 Team 定义和运行时之间的桥梁 |
| 差距 4: Team 不是一等公民 | M5 统一 Team 生命周期 |

---

## 十二、渐进式演进方案（5 个 Milestone）

> **实施进度**：M1 ✅ 已完成 | M2 ✅ 已完成 | M3 ✅ 已完成 | M4 ✅ 已完成 | M5 ✅ 已完成

### M1：修复 P0 业务 Bug（可独立回滚）✅ 已完成

| 任务 | 修复项 | 影响范围 | 状态 |
|------|--------|----------|------|
| 1.1 | BUG-01：Critic Loop 否定语义修复 | `critic_loop_cond.go` | ✅ |
| 1.2 | BUG-02：`finishRunErr` 只发布一个事件 | `runner_helpers.go` | ✅ |
| 1.3 | BUG-03：嵌入式图入口点排序确定性 | `embedded_graph.go` | ✅ |
| 1.4 | BUG-04：Fallback Agent 走正常解析管线 | `failure_recovery.go` + `node_wiring.go` | ✅ |
| 1.5 | BUG-05：恢复路径使用真实 AgentKey | `team_graph_run_finisher.go` + `team_graph_run_coordinator.go` + `runner.go` | ✅ |
| 1.6 | BUG-06：`convertTrpcEvent` 传入 logger | `runtime_adapter.go` | ✅ |
| 1.7 | BUG-07：EventBridge 共享实例 | `runtime_adapter.go` + `event_bridge.go` | ✅ |

**M1 实施细节**：
- BUG-01：新增 `containsNegationBeforeWord` 函数，检测 "not"/"don't" 等否定词
- BUG-04：`failureRecoveryOptions`/`failureRecoveryAfterNode` 新增 `resolvedFallback trpcagent.Agent` 参数；`wireNode` 中预解析 fallback agent；新增 `resolvedAgentNodeFunc` + `fallbackAgentWrapper` 确保已解析 agent 走完整管线
- BUG-05：`buildResumeSessionContext` 新增 `agentKeyFn` 参数；Coordinator 新增 `agentKeyFn` 字段；Runner 在 `SetTeamGraphRunCoordinator` 中注入 catalog 查询函数
- BUG-07：`trpcGraphRuntime` 新增 `bridge *EventBridge` 字段，Run/Resume 时懒初始化；`convertTrpcEvent` 改为接受 `*EventBridge` 参数；`EventBridge` 新增 `EventBus()` 方法

### M2：修复 P1 竞态和状态安全（可独立回滚，可与 M1 并行）✅ 已完成

| 任务 | 修复项 | 影响范围 | 状态 |
|------|--------|----------|------|
| 2.1 | ARCH-07：TOCTOU 竞态修复 | `graph_execution.go` | ✅ |
| 2.2 | ARCH-08：GC 驱逐前取消 runtime | `graph.go` | ✅ |
| 2.3 | ARCH-10：`GraphExecution` 增加独立 mutex | `graph.go` + `graph_team_execution.go` + `graph_execution.go` + 多个读取方 | ✅ |
| 2.4 | ARCH-06：Circuit Breaker 绑定到 GraphAgent 实例 | `circuit_breaker.go` + `builder.go` + `node_wiring.go` + `runtime_adapter.go` + `team_graph_root.go` + 测试文件 | ✅ |
| 2.5 | ARCH-09：Team build config 恢复路径 | `graph_team_execution.go` | ✅（临时修复，M3.8 将完全替换） |

**M2 实施细节**：
- ARCH-06：`CircuitBreakerState` 从包级变量改为实例级 struct，绑定到 `GraphAgent`；`BuildStateGraph*` 系列函数返回值新增 `*CircuitBreakerState`；所有调用方和测试适配
- ARCH-07：`updateExecutionFromRuntimeEvent` 中锁内深拷贝 Steps 切片，锁外用快照写 DB
- ARCH-08：GC 驱逐前调用 `exec.runtime.Cancel()` 终止运行时
- ARCH-10：`GraphExecution` 新增 `interruptMu sync.RWMutex` + `interrupted bool`；新增 `IsInterrupted()`/`GetInterruptNode()` 访问器；所有读取方改用访问器

### M3：引入 CompiledTeam——断开耦合根（回滚成本较高但收益大）✅ 已完成

> 这是连接当前架构和目标架构的关键 Milestone。

| 任务 | 修复项 | 影响范围 | 状态 |
|------|--------|----------|------|
| 3.1 | 定义 `CompiledTeam`、`RoleInfo`、`NodeTaskMeta` 结构体 | `biz/compiled_team.go`（新文件） | ✅ |
| 3.2 | `FailurePolicy` 编译期展开：从 `GraphBuildConfig` 移除 `FailurePolicy` 字段 | `biz/graph.go` + `biz/failure_policy.go` + `graph/trpc/builder.go` | ✅ |
| 3.3 | `ParallelBranchIDs` 编译期展开：从 `GraphBuildConfig` 移除 | `biz/graph.go` + `biz/failure_policy.go` | ✅ |
| 3.4 | `CircuitBreaker` 编译期展开（预留，wireNode 传 nil） | `graph/trpc/node_wiring.go` | ✅ |
| 3.5 | Task 字段分离：8 个 Task 字段移到 `NodeTaskMeta`，`TaskMeta` 加入 `GraphBuildConfig` | `biz/graph.go` + `biz/graph_task_input.go` + `team/embedded_graph.go` + `graph/trpc/builder.go` + 多个消费方 | ✅ |
| 3.6 | `CompileToCompiledTeam`：替换 `CompileToGraphRuntimeConfig`，产出 `CompiledTeam` | `team/graph_compile.go` + `team/graph_runtime_config.go` | ✅ |
| 3.7 | `RoleManifest` 生成：编译时从节点收集 AgentKey/DisplayName/Role | `team/graph_compile.go` | ✅ |
| 3.8 | `CompiledTeam` 持久化：替换 `teamBuildConfigs` 内存缓存，DB 作为二级缓存 | `biz/graph_team_execution.go` + `biz/graph_execution.go` + `data/compiled_team_repo.go`（新文件） + `data/compiled_team_schema.go`（新文件） | ✅ |

**M3 完成后的效果**：

| 指标 | 修复前 | 修复后 |
|------|--------|--------|
| `graph/trpc` import `biz` | 是（`TeamFailurePolicy` + 常量） | 仅常量（`SkipNodeFuncRef` 等） ✅ |
| `GraphBuildConfig` 字段数 | 13 | 12（移除 `FailurePolicy` + `ParallelBranchIDs`，新增 `TaskMeta`） |
| `NodeDef` 字段数 | 28 | 20（移除 8 个 Task 字段） ✅ |
| `EdgeDef.Kind` 丢失 bug | 存在 | 待 M4 类型统一后修复 |
| 角色语义可查 | 否 | 是（`RoleManifest`） ✅ |
| Task 元数据传递 | 透传 NodeDef | 独立 `NodeTaskMeta` ✅ |
| Team Graph 恢复 | GC 驱逐后失败 | DB 持久化 + 内存二级缓存 ✅ |
| `CompiledTeam` | 不存在 | 编译产物桥梁 ✅ |

### M4：Graph 独立化 + biz/trpc 类型统一 ✅ 已完成

| 任务 | 修复项 | 影响范围 | 状态 |
|------|--------|----------|------|
| 4.1 | biz/trpc 类型统一：别名 + 嵌入扩展 | `graph/trpc/builder.go` + `graph/adapter/runtime_adapter.go` | ✅ |
| 4.2 | 消除 `bizCfgToTrpc`/`trpcCfgToBiz` 手工映射 | `runtime_adapter.go`（约 93 行 → 约 30 行） | ✅ |
| 4.3 | `GraphBuilderFactory` 拆分为 5 个窄接口 | `biz/graph_runtime.go` + `graph/adapter/runtime_adapter.go` | ✅ |
| 4.4 | `EdgeDef.Kind` 丢失 bug 修复 | `graph/trpc/builder.go`（类型别名自然保留 `Kind` 字段） | ✅ |
| 4.5 | `NodeDefInfo` 替换为 `NodeTaskMeta` | `biz/graph_runtime.go` + `biz/task.go` + `biz/graph.go` | ✅ |

**M4 实施细节**：
- 类型别名：`EdgeDef = biz.EdgeDef`, `StateFieldDef = biz.StateFieldDef`, `ReducerType = biz.ReducerType`, `ExecutionEngineType = biz.ExecutionEngineType`
- 嵌入扩展：`NodeDef` 嵌入 `biz.NodeDef` + `Func trpcgraph.NodeFunc`；`ConditionalEdgeDef` 嵌入 `biz.ConditionalEdgeDef` + `CondFunc any`
- `SubgraphDef` 和 `GraphBuildConfig` 保持独立（因 `BuildConfig` 字段类型不同）
- `GraphBuilderFactory` 拆分为：`GraphRunnerFactory`(3)、`GraphVisualizer`(1)、`GraphValidator`(1)、`GraphTemplateProvider`(3)、`GraphNodeInfoProvider`(2)

### M5：Team 生命周期统一 + Runner 重构 ✅ 已完成

| 任务 | 修复项 | 影响范围 | 状态 |
|------|--------|----------|------|
| 5.1 | `RunnerConfig` 替代 10 个非循环 Setter | `team/runner_config.go`（新文件） + `team/runner.go` | ✅ |
| 5.2 | `SetTeamGraphRunCoordinator` 保留为唯一 Setter（循环依赖） | `team/runner.go` | ✅ |
| 5.3 | `KnowledgeFacade` 封装 4 个 Knowledge 字段 | `team/runner_config.go` + `team/runner.go` | ✅ |
| 5.4 | Wire 配置更新 | `cmd/admin/wire.go` + `wire_gen.go` | ✅ |

**M5 实施细节**：
- `Runner` 从 20 字段 + 12 Setter → 3 字段（`cfg RunnerConfig` + `teamGraphCoord` + `td TurnDeps`）+ 1 Setter
- `KnowledgeFacade` 封装 `Retriever`/`Router`/`FederatedRetriever`/`Evaluator`
- `chat_orchestrator.go` 从 11 行 Setter 调用 → 4 行直接字段赋值

### Milestone 依赖关系

```
M1 (Bug 修复)     ← 无前置依赖，可立即开始
    ↓
M2 (竞态修复)     ← 无前置依赖，可与 M1 并行
    ↓
M3 (CompiledTeam) ← 依赖 M1（Bug 修复后行为才稳定）
    │ 3.1 定义结构体 ← 无子依赖
    │ 3.2 FailurePolicy 展开 ← 无子依赖
    │ 3.3 ParallelBranchIDs 展开 ← 依赖 3.2
    │ 3.4 CircuitBreaker 展开 ← 依赖 3.2
    │ 3.5 Task 字段分离 ← 无子依赖，可与 3.2 并行
    │ 3.6 CompileToCompiledTeam ← 依赖 3.2+3.5
    │ 3.7 RoleManifest ← 依赖 3.6
    │ 3.8 持久化 ← 依赖 3.6
    ↓
M4 (Graph 独立化) ← 依赖 M3（类型稳定后再统一）
    ↓
M5 (Team 统一)    ← 依赖 M4（Graph 独立后才能安全拆 Runner）
```

**关键路径**：M1 → M3.2 → M3.6 → M4.1 → M5.1

---

## 十三、方案可行性评估

### 13.1 原方案 vs 修正方案 vs 目标架构

| 维度 | 原方案（TeamGraphExtras） | 修正方案（编译期展开） | 目标架构（CompiledTeam） |
|------|--------------------------|----------------------|------------------------|
| 解耦彻底性 | 形式解耦 | 实质解耦（Graph 层面） | 完全解耦 + 业务语义保留 |
| 改动范围 | 小 | 中 | 中-大（但分 Milestone） |
| 回滚风险 | 低 | 中 | 中（每步可独立回滚） |
| 长期收益 | 低 | 高 | 最高 |
| 是否解决因果链根因 | 否 | 是 | 是 |
| 是否弥补业务语义差距 | 否 | 部分 | 全部 |

### 13.2 风险点

| 风险 | 严重程度 | 缓解措施 |
|------|----------|----------|
| `FailurePolicy` 展开后无法回溯原始策略 | 中 | `CompiledTeam.OriginalPolicy` 保留原始策略引用 |
| `NodeTaskMeta` 需要在 Task 协调逻辑中传递 | 中 | 通过 `CompiledTeam` 传递，不影响 Graph 运行时 |
| biz/trpc 类型统一后 `SubgraphDef.BuildConfig` 递归类型 | 低 | `GraphBuildConfig = biz.GraphBuildConfig` 类型别名规避 |
| Runner 拆分到不同子包会循环 import | 高 | **保持在同包内**，只拆 struct 不拆包 |
| M3.5 Task 字段分离影响前端 | 中 | 前端消费 `GraphDefinition`（持久化模型），不是 `NodeDef`（编译模型） |
| `CompiledTeam` 持久化增加 DB 写入 | 低 | 仅 Team 路径需要，独立 Graph 路径不受影响 |

### 13.3 biz/trpc 类型统一可行性

| 维度 | 评估 |
|------|------|
| 架构合规 | ✅ biz 不引入 trpc 依赖，trpc 向内依赖 biz，符合依赖方向 |
| 技术可行性 | ✅ Go 的嵌入机制支持 trpc 扩展 biz 类型 |
| 实施路径 | 先类型别名（`EdgeDef`/`StateFieldDef`/`GraphBuildConfig`），再嵌入扩展（`NodeDef`/`ConditionalEdgeDef`），最后处理 `SubgraphDef` |
| 风险 | ⚠️ `SubgraphDef.BuildConfig` 递归类型需类型别名规避 |

### 13.4 Runner 拆分可行性

| 维度 | 评估 |
|------|------|
| 循环 import | ✅ 同包内不产生（拆到不同子包则 `team` ↔ `team/graph` 循环） |
| Setter 消除 | ⚠️ 2 个 Setter（Runner ↔ Coordinator）是结构性循环，需 Mediator |
| Wire 复杂度 | ✅ 更简单——Service 层不再需要知道 Runner 的内部 Setter |

---

## 十四、验证清单

### 14.1 每个 Milestone 完成后的验证

| 验证项 | 命令 |
|--------|------|
| 编译通过 | `make api && make wire && make build` |
| 全量测试 | `make test` |
| Lint 通过 | `make lint` |
| 无红线违反 | 检查 `internal/biz` 不 import `pkg/trpc-agent-go`、Service 层不直接依赖 Repo |

### 14.2 关键业务场景验证

| 场景 | 验证要点 |
|------|----------|
| Critic Loop 审批 | 否定语义（"not approved"）不误判为通过 |
| Team Run 失败 | 只发布 `TeamRunFailed`，不发布 `TeamRunFinished` |
| 嵌入式图编译 | 相同 definition 多次编译产生相同入口点 |
| Fallback Agent | 走完整解析管线，获得工具和模型配置 |
| HITL 恢复 | 恢复后步骤正确关联到 node |
| Graph 执行摘要 | 前端收到的 `execution_summary` 包含所有节点记录 |
| 并行节点执行 | 多节点同时完成时不产生数据竞态 |
| GC 驱逐 | 驱逐前取消 runtime，不产生僵尸 goroutine |
| 角色语义可查（M3+） | 前端/日志可通过 `RoleManifest` 展示角色信息 |
| CompiledTeam 持久化（M3+） | 进程重启后 Team Graph 可恢复 |
| Graph 独立使用（M4+） | 可视化编辑器创建的 Graph 不依赖 Team 类型 |

### 14.3 回滚策略

| Milestone | 回滚方式 |
|-----------|----------|
| M1 | 单文件 revert，无依赖链 |
| M2 | 单文件 revert，需检查竞态修复是否引入新问题 |
| M3 | 需同步 revert `CompiledTeam` + 编译期展开 + Task 分离，影响面较大 |
| M4 | 需同步 revert 类型统一 + Factory 拆分 |
| M5 | 需同步 revert Runner/Usecase 拆分 + Wire 配置，回滚成本最高 |
