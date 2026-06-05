## Context

Aranea-Agents 的 Team（语义层）和 Graph（执行引擎层）通过编译-运行两阶段协作。当前 `GraphBuildConfig` 作为唯一传输载体承载了 7 个耦合点，导致因果链：类型双源 → Factory 膨胀 → God Usecase → God Runner → Setter 注入 → 运行时 Bug。同时存在 7 个 P0 Bug 和 10 个 P1 架构缺陷。

行业调研（LangGraph/Microsoft Agent Framework/AWS Strands Agents/MAN+ESM 论文）确认：Team（声明式/角色驱动）→ Graph（执行式/图驱动）的编译模式是最佳实践，Aranea 方向正确但解耦不彻底。

当前架构关键数据（变更前基线）：
- `GraphBuildConfig`：11 字段（`FailurePolicy`/`ParallelBranchIDs` 从未在此结构体上定义，它们存在于 `team.Definition` 和编译管线参数中；原始设计文档误记为 13 字段）
- `NodeDef`：28 字段（含 8 个 Task 字段 + 3 个 Failure 字段）
- `Runner`：20 字段 + 12 个 Setter
- `GraphBuilderFactory`：10 方法
- biz/trpc 双源类型：6 组，手工映射约 93 行

### 实现后实际数据（截至当前代码）

- `GraphBuildConfig`（biz 层）：12 字段（`FailurePolicy`/`ParallelBranchIDs` 从未在此结构体上定义，新增 `TaskMeta` 字段）
- `GraphBuildConfig`（trpc 层）：11 字段（不含 `TaskMeta`，含 `Nodes []NodeDef`/`Subgraphs []SubgraphDef` 等 trpc 特有类型）
- `NodeDef`（biz 层）：20 字段（8 个 Task 字段已移至 `NodeTaskMeta`，3 个 Failure 字段保留为 `RetryMaxAttempts`/`FailureAction`/`FallbackAgent`）
- `NodeDef`（trpc 层）：嵌入 `biz.NodeDef` + `Func trpcgraph.NodeFunc`
- `Runner`：8 字段（`teams`/`usage`/`td`/`skillDBRepo`/`codeExecFactory`/`cfg RunnerConfig`/`teamGraphCoord`/`lg`）+ 5 个 Setter（`SetTeamGraphRunCoordinator`/`SetAwaitHookProvider`/`SetRuns`/`SetStreamOptsFactory`/`SetAgentHelper`）
- `GraphBuilderFactory`：5 个窄接口 + 1 个复合接口（`GraphRunnerFactory` 3 方法、`GraphVisualizer` 1 方法、`GraphValidator` 1 方法、`GraphTemplateProvider` 3 方法、`GraphNodeInfoProvider` 2 方法）
- biz/trpc 双源类型：6 组中 4 组已统一（`EdgeDef`/`StateFieldDef` 类型别名，`NodeDef`/`ConditionalEdgeDef` 嵌入），2 组待统一（`GraphBuildConfig`/`SubgraphDef`）
- `CompiledTeam`：值嵌入 `GraphBuildConfig`，含 `RoleManifest`/`OriginalPolicy`/`CompiledAt`，提供 `TaskMetaForNode`/`RoleForNode` 访问器
- `CompiledTeamRepo`：手写 SQL DDL + Ent Schema 双定义（Ent Schema 用于文档/DDL 追踪，实际 CRUD 使用手写 SQL），含 `Save`/`Load`/`LoadForSession`/`Delete`，`LoadForSession` 校验 session 活跃状态。DDL 迁移通过 `ddl_migration_registry.go` 注册（含初始建表 + session_id 字段增量迁移）
- `TeamRunMediator`：已定义并实现 `TeamGraphCoordAccess`（4 方法）+ `TeamGraphRunFinisher`（2 方法）双接口，含 `SetCoordinator`/`SetFinisher` 后置接线，但尚未集成到 Runner
- `GraphDefinitionUsecase`：已从 `GraphUsecase` 中分离，`GraphUsecase.DefUC()` 返回定义子用例
- `GraphUsecase`：仍持有 `teamBuildConfigs` 内存缓存（与 DB 双写），`buildConfigForExecution` 实现三级回退（内存→DB→重编译）
- `GraphNodeResolverSet`：已定义（Models/Tools/Agents/Functions/Subgraphs），注入到 `trpcGraphBuilderFactory`。`Functions` 字段已在 DI 中接线（`wire.go` 中 `CatalogFunctionResolver` 赋值），但 `wireNode` 中未消费 `deps.Functions`（函数节点仍通过 `FuncRef` 解析，未走 `FunctionResolver` 管线）；`Subgraphs` 字段已定义但同样未在 `wireNode` 中消费

## Goals / Non-Goals

**Goals:**

1. 修复 7 个 P0 业务逻辑 Bug，消除运行时 panic 和数据丢失
2. 修复 5 个 P1 竞态/状态安全问题，保证并发安全
3. 引入 `CompiledTeam` 作为 Team↔Graph 编译产物桥梁，断开 7 个耦合点
4. 实现 `FailurePolicy` 编译期展开，Graph 运行时零 Team 类型依赖
5. 分离 NodeDef 的 Task 元数据，NodeDef 从 28 字段降至 20 字段
6. 新增 `RoleManifest` 解决角色语义编译后丢失
7. `CompiledTeam` 持久化替换内存缓存，解决 GC 驱逐导致恢复失败
8. biz/trpc 类型统一，消除双源定义和手工映射
9. Runner 同包拆分 + Mediator 解决双向绑定
10. GraphUsecase 拆分为 Definition + Execution + CacheManager

**Non-Goals:**

- 不改变 API/Proto 定义
- 不改变前端代码（前端消费持久化模型，不消费编译模型）
- 不实现运行时动态角色解析（`RoleManifest.Capabilities` 仅预留字段）
- 不实现 Team 运行时重编译（M5 之后考虑）
- 不改变 trpc-agent-go 框架代码
- 不引入新的外部依赖

## Decisions

### D1：CompiledTeam 嵌入 GraphBuildConfig vs 独立字段

**选择**：CompiledTeam 嵌入 `GraphBuildConfig`（值嵌入，非指针）

**理由**：
- 嵌入使 `CompiledTeam` 可直接作为 `GraphBuildConfig` 使用，减少调用方改动
- 值嵌入（非指针）保证 CompiledTeam 持有完整的图拓扑快照，避免共享可变状态
- 替代方案（独立字段 `GraphConfig GraphBuildConfig`）需要每个调用点加 `.GraphConfig` 前缀

**替代方案**：
- 指针嵌入：节省拷贝开销，但引入共享可变状态风险 → 否决
- 独立字段：更明确的语义边界，但改动面大 → 作为未来重构选项

### D2：FailurePolicy 编译期展开策略

**选择**：在 `CompileToCompiledTeam` 阶段展开，展开后从 `GraphBuildConfig` 移除 `FailurePolicy` 和 `ParallelBranchIDs`

**展开规则**：
- `FailurePolicy.Retry` → `NodeDef.RetryMaxAttempts = policy.MaxRetries`
- `FailurePolicy.Failover` → `NodeDef.FailureAction = "failover"`, `NodeDef.FallbackAgent = policy.FallbackAgentID`
- `FailurePolicy.Skip` → `NodeDef.FailureAction = "skip_on_failure"`
- `ParallelBranchIDs` → 并行分支节点 `NodeDef.FailureAction = "skip_on_failure"`
- `CircuitBreaker` → `NodeDef.RetryMaxAttempts = breaker.MaxRetries`

**理由**：Graph 运行时只消费展开后的通用字段（`FailureAction/FallbackAgent/RetryMaxAttempts`），不需要理解 Team 语义。展开在编译阶段完成，运行时零 Team 类型依赖。

### D3：NodeTaskMeta 分离策略

**选择**：从 NodeDef 移出 8 个 Task 字段到 `NodeTaskMeta`，通过 `CompiledTeam.TaskMeta map[string]NodeTaskMeta` 传递

**移出的字段**：`RequiredRole`, `AssignmentMode`, `AssignmentStrategy`, `ReviewerAgent`, `ReviewRules`, `TimeoutSeconds`, `HeartbeatIntervalSeconds`, `EnableLeaseExtension`

**理由**：这 8 个字段被 graph/trpc 层零消费，只是透传。真正消费者是 biz 层 Task 协调逻辑。分离后 NodeDef 从 28 降至 20 字段，Graph 运行时不再承载 Task 语义。

### D4：biz/trpc 类型统一策略

**选择**：trpc 层嵌入 biz 类型 + 类型别名

**实施路径**：
1. 简单类型别名：`EdgeDef = biz.EdgeDef`, `StateFieldDef = biz.StateFieldDef`, `GraphBuildConfig = biz.GraphBuildConfig`
2. 嵌入扩展：trpc.NodeDef 嵌入 `biz.NodeDef` + 添加 `Func trpcgraph.NodeFunc`
3. 嵌入扩展：trpc.ConditionalEdgeDef 嵌入 `biz.ConditionalEdgeDef` + 添加 `CondFunc any`
4. 递归类型：`SubgraphDef` 通过 `BuildConfig = biz.GraphBuildConfig` 类型别名规避

**理由**：biz 不引入 trpc 依赖（合规），trpc 向内依赖 biz（合规）。消除约 93 行手工映射代码。

### D5：Runner 拆分策略

**选择**：同包拆分（保持在 `internal/team/` 包内），不拆子包

**理由**：拆到不同子包会产生循环 import（`team` ↔ `team/graph`）。同包拆分只拆 struct 不拆包，避免循环依赖。

**拆分方案**：
- `RunnerConfig` struct 替代 10 个非循环 Setter
- `TeamRunMediator` 解决 Runner ↔ Coordinator 双向绑定
- Knowledge 4 字段封装为 `KnowledgeFacade`

### D6：CompiledTeam 持久化策略

**选择**：持久化 biz 层的 `CompiledTeam`（字符串引用），JSON 序列化到 SQLite

**关键约束**：CompiledTeam 嵌入的 `GraphBuildConfig` 使用 biz 层定义（`FuncRef string`/`CondFuncRef string`），不含函数指针，可安全序列化。trpc 层的 `Func`/`CondFunc` 在运行时从引用解析，不参与持久化。

**Ent Schema**：新增 `CompiledTeam` 表，字段：`team_id`, `graph_id`, `config_json`, `created_at`, `updated_at`。实际实现同时包含 Ent Schema（`internal/data/ent/schema/compiled_team.go`，用于文档/DDL 追踪）和手写 SQL DDL（`compiled_team_schema.go`），CRUD 使用手写 SQL。DDL 迁移通过 `ddl_migration_registry.go` 管理（版本 20260705 初始建表 + 版本 20260714 新增 `session_id` 字段）。

### D7：Milestone 依赖与执行顺序

**选择**：5 个 Milestone 串行执行，M1/M2 可并行

```
M1 (Bug 修复) ← 无前置
M2 (竞态修复) ← 无前置，可与 M1 并行
M3 (CompiledTeam) ← 依赖 M1
M4 (Graph 独立化) ← 依赖 M3
M5 (Team 统一) ← 依赖 M4
```

**关键路径**：M1 → M3.2 → M3.6 → M4.1 → M5.1

## Risks / Trade-offs

### [R1] FailurePolicy 展开后无法回溯原始策略 → 缓解：CompiledTeam.OriginalPolicy 保留原始策略引用

### [R2] NodeTaskMeta 需要在 Task 协调逻辑中传递 → 缓解：通过 CompiledTeam 传递，不影响 Graph 运行时

### [R3] biz/trpc 类型统一后 SubgraphDef 递归类型 → 缓解：GraphBuildConfig = biz.GraphBuildConfig 类型别名规避

### [R4] Runner 拆分到不同子包会循环 import → 缓解：保持在同包内，只拆 struct 不拆包

### [R5] M3.5 Task 字段分离影响前端 → 缓解：前端消费 GraphDefinition（持久化模型），不是 NodeDef（编译模型）

### [R6] CompiledTeam 持久化增加 DB 写入 → 缓解：仅 Team 路径需要，独立 Graph 路径不受影响

### [R7] M3 回滚成本较高 → 缓解：M3 内部 8 个子任务有依赖关系，可按子任务粒度回滚

## Migration Plan

1. **M1**：单文件修复，无依赖链，可独立回滚
2. **M2**：单文件修复，需检查竞态修复是否引入新问题
3. **M3**：按子任务粒度逐步推进，3.1→3.2→3.3→3.4→3.5→3.6→3.7→3.8
4. **M4**：类型统一后删除手工映射代码，需同步 revert
5. **M5**：Runner/Usecase 拆分 + Wire 配置，回滚成本最高

每个 Milestone 完成后验证：`make api && make wire && make build && make test && make lint`

## Open Questions

1. `EdgeDef.Kind` 的消费者是否仅限可视化层？需确认前端是否直接消费此字段
2. `CircuitBreaker` 编译期展开后，运行时是否还需要动态调整熔断阈值？
3. `CompiledTeam` 持久化的 JSON 大小上限？大型 Team（50+ 节点）的序列化性能

## 实现偏差记录

以下记录代码实际实现与设计文档不一致的地方，供后续对齐参考。

### DEV-01：TaskMeta 放置位置偏差

**设计**：`TaskMeta map[string]NodeTaskMeta` 应为 `CompiledTeam` 的独立字段，`GraphBuildConfig` 保持 11 字段纯图拓扑。

**实际**：`TaskMeta` 放在了 `GraphBuildConfig` 上（第 12 字段），`CompiledTeam` 通过值嵌入继承。`GraphBuildConfig` 实际为 12 字段。

**影响**：`GraphBuildConfig` 承载了非图拓扑数据，与"纯图拓扑"目标不完全一致。但由于 `CompiledTeam` 嵌入 `GraphBuildConfig`，功能上等价——`CompiledTeam.TaskMeta` 仍可访问。

**建议**：在 M3 收尾阶段将 `TaskMeta` 从 `GraphBuildConfig` 移至 `CompiledTeam` 独立字段，使 `GraphBuildConfig` 回归 11 字段纯图拓扑。

### DEV-02：FailurePolicy/ParallelBranchIDs 从未在 GraphBuildConfig 上

**设计**：文档描述 `GraphBuildConfig` 原有 13 字段含 `FailurePolicy` 和 `ParallelBranchIDs`，需移除。

**实际**：`GraphBuildConfig` 从未定义 `FailurePolicy *TeamFailurePolicy` 或 `ParallelBranchIDs []string` 字段。这两个概念存在于 `team.Definition.FailurePolicy` 和编译管线的参数传递中，而非 `GraphBuildConfig` 结构体上。

**影响**：设计文档中"从 13 字段简化为 11 字段"的描述基于错误前提。实际 `GraphBuildConfig` 原有 11 字段，新增 `TaskMeta` 后为 12 字段。

### DEV-03：CircuitBreaker 编译期展开未完成

**设计**：`CircuitBreaker` 应在编译期展开为 `NodeDef.RetryMaxAttempts`，`nodeOptions` 不再接收 `*biz.TeamFailurePolicy`。

**实际**：`nodeOptions` 仍接收 `*biz.TeamFailurePolicy` 参数，通过 `policy.CircuitBreaker` 调用 `circuitBreakerOptions`。`ApplyCircuitBreakerPolicy` 已实现将 `RetryMaxAttempts` 写入 NodeDef，但 `graph/trpc` 层仍直接消费 `TeamFailurePolicy.CircuitBreaker`。

**更严重发现**：`BuildStateGraphWithAgents` 调用 `wireNode` 时始终传入 `nil` 作为 policy 参数（`internal/graph/trpc/builder.go:184`：`extras, err := wireNode(ctx, sg, n, deps, nil, cbState)`），导致 `nodeOptions` 中 `if policy != nil && policy.CircuitBreaker != nil` 分支（`internal/graph/trpc/node_wiring.go:58-60`）永远不会执行。`CircuitBreakerState` 虽然每次构建时创建新实例（ARCH-06 修复正确），但运行时熔断回调从未注册到任何节点，`cbState` 成为死对象。

**影响**：
1. `graph/trpc` 层仍依赖 `biz.TeamFailurePolicy` 类型，Goal 4（"Graph 运行时零 Team 类型依赖"）未完全达成
2. 编译期 `RetryMaxAttempts` 展开有效（重试策略生效），但运行时熔断状态跟踪（open/close/half-open 转换）完全失效——`CircuitBreakerState` 的 `State()`/`afterNode()`/`Reset()` 方法均为死代码

### DEV-04：CompiledTeam 持久化使用手写 SQL + Ent Schema 双定义

**设计**：使用 Ent Schema 定义 `compiled_team` 表。

**实际**：同时存在 Ent Schema（`internal/data/ent/schema/compiled_team.go`，用于文档/DDL 追踪）和手写 SQL DDL（`compiled_team_schema.go` 中的 `EnsureCompiledTeamSchema`）。实际 CRUD 操作使用手写 SQL（`compiledTeamRepo` 直接执行 `INSERT OR REPLACE`/`SELECT`/`DELETE`），不使用 Ent 生成的代码。表结构包含 `id`/`team_id`/`graph_id`/`session_id`/`config_json`/`created_at`/`updated_at`，比设计多了 `session_id` 和 `id`（复合主键）字段。DDL 迁移通过 `ddl_migration_registry.go` 注册（版本 20260705 初始建表 + 版本 20260714 新增 `session_id` 字段）。

**影响**：功能等价，Ent Schema 与手写 SQL 需保持同步。`LoadForSession` 方法额外校验 session 活跃状态，这是设计文档未提及的安全增强。

### DEV-05：TeamRunMediator 已定义但未完全集成

**设计**：`TeamRunMediator` 解决 Runner ↔ Coordinator 双向绑定，Runner 仅保留 2 个 Setter。

**实际**：`TeamRunMediator` 结构体已定义并实现 `TeamGraphCoordAccess` + `TeamGraphRunFinisher` 双接口，但 Runner 仍直接持有 `teamGraphCoord *TeamGraphRunCoordinator`，仍有 5 个 Setter 方法。

**影响**：Mediator 的价值（解耦双向依赖）尚未体现，Runner 与 Coordinator 仍直接耦合。

### DEV-06：GraphUsecase 仅部分拆分

**设计**：拆分为 `GraphDefinitionUsecase` + `GraphExecutionUsecase` + `GraphCacheManager`。

**实际**：仅 `GraphDefinitionUsecase` 已独立拆分，`GraphUsecase` 通过 `defUC` 字段持有并委托定义操作。执行逻辑和缓存管理仍在 `GraphUsecase` 中。

### DEV-07：bizCfgToTrpc/trpcCfgToBiz 映射代码仍存在

**设计**：类型统一后消除约 93 行手工映射代码。

**实际**：`bizCfgToTrpc`/`trpcCfgToBiz` 仍存在于 `runtime_adapter.go`（约 46 行），因 `GraphBuildConfig` 和 `SubgraphDef` 仍为双源定义。已统一的 4 组类型（`EdgeDef`/`StateFieldDef`/`NodeDef`/`ConditionalEdgeDef`）在映射函数中使用了嵌入语法简化。

### DEV-08：CompileToGraphRuntimeConfig 保留为委托包装

**设计**：用 `CompileToCompiledTeam` 替换 `CompileToGraphRuntimeConfig`。

**实际**：`CompileToGraphRuntimeConfig` 保留为委托包装（调用 `CompileToCompiledTeam` 后取 `.GraphBuildConfig`），`CompileToGraphRuntimeConfigFromJSON` 也委托给 `CompileToCompiledTeam`。这保持了向后兼容，但旧函数名可能造成混淆。

### DEV-09：buildRoleManifest 未从 catalog 解析 agent_key

**设计**：`RoleManifest` 从 agent catalog 填充 `AgentKey` 等字段。

**实际**：`buildRoleManifest()` 仅从 `GraphBuildConfig.Nodes` 提取信息，`AgentID`/`AgentKey`/`DisplayName` 均设为 `node.AgentName`，`Role` 设为 `node.Type`。未调用 `CompileAgentKey` 回调解析实际的 `agent_key`。

**影响**：`RoleInfo.AgentKey` 与运行时 `ag.AgentKey` 可能不一致（运行时 `AgentKey` 可能与 `AgentName` 不同）。功能上不影响执行，但 RoleManifest 的查询准确性受限。

**建议**：在编译时传入 `CompileAgentKey` 回调，使 `buildRoleManifest` 能解析真实的 `agent_key`。

### DEV-10：GraphUsecase 仍持有 teamBuildConfigs 内存缓存

**设计**：`CompiledTeam` 持久化替换 `teamBuildConfigs` 内存缓存。

**实际**：`GraphUsecase` 仍持有 `teamBuildConfigs map[string]*CompiledTeam` 内存缓存，`RegisterTeamGraphExecution` 同时写入内存和 DB，`buildConfigForExecution` 实现三级回退（内存→DB→重编译）。

**影响**：功能上更健壮（热路径走内存、冷路径走 DB、兜底走重编译），但内存缓存仍占用空间。GC 清理时同时删除内存和 DB 条目。

**建议**：当前双写策略合理，可保留。后续若内存压力大，可改为纯 DB 查询 + LRU 缓存。

### DEV-11：Runner Setter 方法修改 RunnerConfig 字段

**设计**：`RunnerConfig` 替代所有非循环依赖 Setter。

**实际**：`SetAwaitHookProvider`/`SetRuns`/`SetStreamOptsFactory`/`SetAgentHelper` 仍存在，但它们修改的是 `Runner.cfg` 中的字段而非 Runner 自身字段。这些 Setter 是 Wire 注入时序问题的变通方案（部分依赖在 Runner 构造后才可用）。

**影响**：Setter 仍存在但语义已改变（从"设置 Runner 字段"变为"设置 RunnerConfig 字段"），代码可读性降低。

**建议**：在 Wire 配置中确保这些依赖在 Runner 构造时即可用，消除剩余 Setter。

### DEV-12：failure_recovery.go 的 Fallback 逻辑仍使用 NewAgentNodeFunc

**设计**：BUG-04 修复后，fallback agent 仅在通过 resolver 管线正确解析后才调用。

**实际**：`failureRecoveryAfterNode` 中，当 `resolvedFallback` 为 nil 但 `fallback` 字符串非空时，仍使用 `trpcgraph.NewAgentNodeFunc(fallback)` 创建节点函数。这与 BUG-04 的修复意图不完全一致——`wireNode` 中已通过 `deps.Agents.ResolveAgent` 解析 fallback，但 `failureRecoveryAfterNode` 闭包捕获的 `resolvedFallback` 可能为 nil（解析失败时）。

**影响**：在 `wireNode` 中 fallback 解析失败会返回错误阻止图构建，因此运行时 `resolvedFallback` 为 nil 的情况仅出现在 `fallback` 字段为空时，此时不会进入 fallback 分支。风险较低但代码意图不清晰。

### DEV-13：trpc 层 GraphBuildConfig 缺少 TaskMeta 字段

**设计**：biz/trpc 类型统一后，`GraphBuildConfig = biz.GraphBuildConfig` 类型别名。

**实际**：trpc 层 `GraphBuildConfig` 仍为独立结构体（11 字段，不含 `TaskMeta`），与 biz 层 `GraphBuildConfig`（12 字段，含 `TaskMeta`）不同。`bizCfgToTrpc` 转换时丢弃 `TaskMeta`，`trpcCfgToBiz` 转换时不恢复 `TaskMeta`。

**影响**：这是 `GraphBuildConfig` 和 `SubgraphDef` 仍为双源定义的根本原因。trpc 层不需要 `TaskMeta`（它只用于 biz 层 Task 协调），但类型不统一导致映射代码无法消除。

**建议**：在 4.1 完成类型统一时，考虑让 trpc 层 `GraphBuildConfig` 嵌入 `biz.GraphBuildConfig`（类似 `NodeDef` 的嵌入模式），这样 `TaskMeta` 自然继承但 trpc 层不消费。

### DEV-14：GraphNodeResolverSet 已定义但 BuildDeps 仍被使用

**设计**：`FunctionResolver` 应集成到 DI 或移除。

**实际**：`GraphNodeResolverSet` 已定义（含 Models/Tools/Agents/Functions/Subgraphs 五个解析器），注入到 `trpcGraphBuilderFactory`。`Functions` 字段已在 DI 中接线（`wire.go` 中 `CatalogFunctionResolver` 赋值），`Subgraphs` 字段已定义但无实现。旧版 `BuildDeps` 仍标记为 Deprecated 并通过 `ToBuildDepsPtr()` 使用（`buildRuntime` 调用 `resolvers.ToBuildDepsPtr()` 传入 `BuildStateGraphWithRegistryAndLogger`）。`wireNode` 中未消费 `deps.Functions`（函数节点仍通过 `FuncRef` 解析，未走 `FunctionResolver` 管线）。

**影响**：存在两套并行的依赖注入路径（`BuildDeps` 和 `GraphNodeResolverSet`），增加维护成本。`Functions`/`Subgraphs` 解析器已定义但未在图构建管线中消费。

**建议**：完成 `BuildDeps` 到 `GraphNodeResolverSet` 的迁移后，删除 `BuildDeps` 及其转换方法。

### DEV-15：CompileToGraphRuntimeConfigFromJSON 返回 *biz.CompiledTeam 而非 GraphBuildConfig

**设计**：`CompileToGraphRuntimeConfig` 委托给 `CompileToCompiledTeam` 后取 `.GraphBuildConfig`。

**实际**：`CompileToGraphRuntimeConfigFromJSON` 直接返回 `*biz.CompiledTeam`（而非 `biz.GraphBuildConfig`），函数签名与名称不一致。调用方 `runner_team_compiler.go` 使用的是 `CompileToGraphRuntimeConfigFromJSON` 的返回值作为 `*biz.CompiledTeam`。

**影响**：函数名暗示返回 `GraphBuildConfig`，但实际返回 `CompiledTeam`，可能造成混淆。功能上等价且更优（返回更多信息）。

**建议**：重命名为 `CompileToCompiledTeamFromJSON` 或在函数注释中明确说明返回类型。

### DEV-16：CompileToGraphBuildConfig 系列函数未在设计文档中记录

**设计**：设计文档仅描述 `CompileToCompiledTeam` 和 `CompileToGraphRuntimeConfig` 两个编译入口。

**实际**：`graph_compile.go` 中还存在 `CompileToGraphBuildConfig` 和 `CompileToGraphBuildConfigFromJSON` 两个函数，它们直接返回 `biz.GraphBuildConfig`（不包装为 `CompiledTeam`），用于仅需要图拓扑的场景（如可视化、模板生成）。内部实现调用 `compileToGraphBuildConfigWithLoader`，与 `CompileToCompiledTeam` 共享底层编译逻辑，但不执行 `finalizeRuntimeGraphConfig`（不应用 FailurePolicy 展开、不生成 RoleManifest）。

**影响**：这些函数是合法的"纯图拓扑编译"入口，与 `CompileToCompiledTeam`（完整编译）互补。设计文档未记录可能导致维护者遗漏这些路径。

### DEV-17：DDL 迁移注册机制

**设计**：设计文档未描述 DDL 迁移管理机制。

**实际**：`internal/data/ddl_migration_registry.go` 实现了版本化 DDL 迁移注册机制。`compiled_teams` 表有两个迁移条目：版本 20260705（初始建表）和版本 20260714（新增 `session_id` 字段 + 索引）。迁移在应用启动时按版本号顺序执行。

**影响**：这是项目统一的 DDL 迁移模式，`compiled_teams` 表的 schema 变更应通过新增迁移条目管理，而非直接修改 `compiled_team_schema.go`。

### DEV-18：CircuitBreaker 运行时回调为死代码

**设计**：`CircuitBreakerState` 应在运行时跟踪熔断状态（open/close/half-open 转换），通过 `afterNode` 回调注册到节点执行管线。

**实际**：`BuildStateGraphWithAgents` 调用 `wireNode` 时始终传入 `nil` 作为 policy 参数（`internal/graph/trpc/builder.go:184`），导致 `nodeOptions` 中 `if policy != nil && policy.CircuitBreaker != nil` 分支（`internal/graph/trpc/node_wiring.go:58-60`）永远不会执行。`circuitBreakerOptions` 函数从未被调用，`CircuitBreakerState.afterNode` 回调从未注册到任何节点。

**影响**：
- `CircuitBreakerState` 结构体及其方法（`State()`/`afterNode()`/`Reset()`）均为死代码
- 编译期 `RetryMaxAttempts` 展开有效（重试策略生效），但运行时熔断状态跟踪完全失效
- `cbState` 对象被创建但从未产生运行时效果

**File**: `internal/graph/trpc/builder.go:184`, `internal/graph/trpc/node_wiring.go:58-60`

### DEV-19：wireNode 和 nodeOptions 的 policy 参数为无用参数

**设计**：`wireNode` 和 `nodeOptions` 应接收 `*biz.TeamFailurePolicy` 参数以支持 CircuitBreaker 运行时回调注册。

**实际**：`wireNode` 签名中的 `policy *biz.TeamFailurePolicy` 和 `nodeOptions` 签名中的 `policy *biz.TeamFailurePolicy` 在当前代码中始终接收 `nil` 值（`builder.go:184` 传入 `nil`）。这些参数是冗余参数，可以安全移除。

**影响**：冗余参数增加了代码理解成本，且使 `graph/trpc` 层不必要地依赖 `biz.TeamFailurePolicy` 类型。

**File**: `internal/graph/trpc/node_wiring.go`, `internal/graph/trpc/builder.go`

### DEV-20：evictIfNeeded 不清理 DB 中的 CompiledTeam 记录

**设计**：GC 驱逐执行记录时应同步清理内存缓存和 DB 中的 `CompiledTeam` 记录。

**实际**：`evictIfNeeded()` 在驱逐执行记录时仅删除 `teamBuildConfigs[oldestID]`（内存），但不删除 DB 中对应的 `CompiledTeam` 记录。`gc()` 删除内存条目时同样不清理 DB。

**影响**：DB 中的 `compiled_teams` 记录会无限增长，没有清理机制。虽然 `buildConfigForExecution` 的三级回退（内存→DB→重编译）使功能不受影响，但 DB 空间持续膨胀。

**File**: `internal/biz/graph_execution.go:52`

### DEV-21：CompileToCompiledTeam linked 路径跳过 applyAdaptiveAgentDestinations

**设计**：`CompileToCompiledTeam` 的所有路径都应执行完整的编译管线，包括 `applyAdaptiveAgentDestinations`。

**实际**：在 `CompileToCompiledTeam` 的 linked graph 路径中，调用了 `finalizeRuntimeGraphConfig`，但跳过了 `applyAdaptiveAgentDestinations`。如果 linked graph 同时是 adaptive 模式，`Destinations` 不会被设置。

**影响**：linked + adaptive 组合场景下，节点的 `Destinations` 字段为空，可能导致自适应路由逻辑失效。

**File**: `internal/team/graph_compile.go:197-203`
