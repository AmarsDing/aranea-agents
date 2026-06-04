## Context

Aranea-Agents 的 Team（语义层）和 Graph（执行引擎层）通过编译-运行两阶段协作。当前 `GraphBuildConfig` 作为唯一传输载体承载了 7 个耦合点，导致因果链：类型双源 → Factory 膨胀 → God Usecase → God Runner → Setter 注入 → 运行时 Bug。同时存在 7 个 P0 Bug 和 10 个 P1 架构缺陷。

行业调研（LangGraph/Microsoft Agent Framework/AWS Strands Agents/MAN+ESM 论文）确认：Team（声明式/角色驱动）→ Graph（执行式/图驱动）的编译模式是最佳实践，Aranea 方向正确但解耦不彻底。

当前架构关键数据：
- `GraphBuildConfig`：13 字段（含 `FailurePolicy` + `ParallelBranchIDs`）
- `NodeDef`：28 字段（含 8 个 Task 字段 + 3 个 Failure 字段）
- `Runner`：20 字段 + 12 个 Setter
- `GraphBuilderFactory`：10 方法
- biz/trpc 双源类型：6 组，手工映射约 93 行

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

**Ent Schema**：新增 `CompiledTeam` 表，字段：`team_id`, `graph_id`, `config_json`, `created_at`, `updated_at`

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
