## Context

team-graph-optimization 变更已归档，但深度代码审查发现 15 个阻断级问题。这些问题分布在 4 个层（biz/team/graph-trpc/data）和 Wire 配置中，涵盖并发安全、架构合规、Agent 运行时集成、编程规范四个维度。

当前状态关键数据：
- `GraphExecution`：5 个并发竞态点（evictIfNeeded/gc/consumeRuntimeEvents 未正确持有 execMu）
- `TeamGraphRunCoordinator.finisher`：无锁保护，safego goroutine 中读取
- `CircuitBreakerState`：全链路空转，`nodeOptions` 接收 `cbState` 但从未使用
- `GraphNodeResolverSet.Functions/Subgraphs`：Wire 组装但 `wireNode` 未消费
- `BuildDeps`：标记 Deprecated 但仍为所有核心构建函数参数类型
- `KnowledgeFacade`/`RunnerConfig`：4+2 个具体类型指针，team 层直接依赖 biz/infra 实现
- `NewRunner`：6 个 `*biz.XxxUsecase` 具体类型参数
- Ent Schema `id` MaxLen(64) 与实际 ID 格式 `teamID:graphID`（可达 129 字符）不一致
- `CompileToCompiledTeam` linked graph 路径跳过 adaptive mode 处理

## Goals / Non-Goals

**Goals:**

1. 修复 5 个并发竞态，确保 `GraphExecution` 和 `TeamGraphRunCoordinator` 字段访问安全
2. 修复 linked graph + adaptive 功能性 Bug
3. 清理或激活 CircuitBreakerState 死代码
4. 完成 GraphNodeResolverSet 集成，消除 BuildDeps Deprecated 状态
5. team 层依赖接口化，消除 6 个 Usecase 具体类型 + KnowledgeFacade/PluginRT/PluginManager 具体类型
6. 修复红线违反（loggateway.Global、SetGlobalWebResearchChecker）
7. 修复 Ent Schema 与 DDL 不一致
8. 编程规范修复（magic string/number、参数超限、SQL 语义、校验增强）

**Non-Goals:**

- 不改变 API/Proto 定义
- 不改变前端代码
- 不实现运行时动态角色解析（RoleManifest.Capabilities 仍为预留字段）
- 不实现 Team 运行时重编译
- 不改变 trpc-agent-go 框架代码
- 不引入新的外部依赖
- 不重构 GraphUsecase Facade（30+ 委托方法，独立变更处理）
- 不重构 NewGraphAgent 4 个构造函数（独立变更处理）

## Decisions

### D1：CircuitBreakerState 处理策略——删除而非激活

**选择**：删除 `circuit_breaker.go` 及 `cbState` 全链路传递

**理由**：
- 当前 `CircuitBreakerState` 从未生效（`nodeOptions` 不使用 `cbState` 参数）
- 编译期 `RetryMaxAttempts` 展开已覆盖重试需求，运行时熔断状态跟踪无实际消费者
- 激活需要设计熔断策略（open/close/half-open 转换阈值、恢复机制），超出本次修复范围
- 删除后 `builder.go`/`node_wiring.go`/`runtime_adapter.go` 简化，减少理解成本

**替代方案**：
- 激活：在 `nodeOptions` 中注册 `cbState.afterNode()` 为 `WithPostNodeCallback` → 否决（需设计熔断策略，超出修复范围）
- 保留但标记 Deprecated → 否决（死代码应删除而非标记）

### D2：BuildDeps 迁移策略——渐进式替换

**选择**：将构建函数签名从 `*BuildDeps` 迁移为 `*GraphNodeResolverSet`，删除 `BuildDeps` 及转换方法

**理由**：
- `BuildDeps` 标记 Deprecated 但仍为核心参数，`ToBuildDeps()` 丢弃 Functions/Subgraphs
- `GraphNodeResolverSet` 已定义且 Wire 已组装，仅需在 `wireNode` 中消费
- 一次性替换所有构建函数签名，避免双路径并存

**替代方案**：
- 逐步迁移（先新增 `BuildWithResolvers` 方法，再废弃旧方法）→ 否决（增加维护成本，不如一次性替换）

### D3：team 层接口化策略——窄接口 + Wire 绑定

**选择**：为 `NewRunner` 的 6 个 Usecase 参数和 `KnowledgeFacade`/`PluginRT`/`PluginManager` 抽取 biz 窄接口

**实施路径**：
1. 在 biz 层定义窄接口（如 `TeamUsageQuerier`、`TeamSessionManager`、`TeamAgentLookup` 等），每个接口 ≤5 方法
2. biz Usecase 实现这些窄接口（编译期断言）
3. `NewRunner` 构造函数接收窄接口
4. Wire 绑定在 `wire.go` 中完成

**理由**：team 层作为独立包，不应依赖 biz 层具体实现。窄接口使依赖关系显式化，便于测试和替换。

**替代方案**：
- 保持现状（具体类型依赖）→ 否决（架构违规，违反依赖倒置原则）
- 抽取到独立包 → 否决（增加包数量，当前 team 包内聚性足够）

### D4：并发竞态修复策略——execMu 粒度保持

**选择**：在 `evictIfNeeded`/`gc()`/`consumeRuntimeEvents` 中正确持有 `execMu`，不引入新锁

**理由**：
- `execMu` 已存在且语义正确（保护 `GraphExecution` 内部状态）
- 问题在于调用方未持有 `execMu` 就读写 exec 字段
- 修复方式：在读写前加 `exec.execMu.RLock()`/`exec.execMu.Lock()`
- 需注意锁顺序：`uc.mu` → `execMu`，避免死锁

**锁顺序约定**：先释放 `uc.mu`，再获取 `execMu`，操作完成后重新获取 `uc.mu`（如需）

### D5：Coordinator.finisher 保护策略——sync.Once

**选择**：`SetFinisher` 使用 `sync.Once` 保护，确保只设置一次

**理由**：
- `finisher` 在启动时设置一次，之后只读
- `sync.Once` 语义最匹配"只执行一次"的模式
- 比 `sync.RWMutex` 更轻量，比无保护更安全

### D6：Ent Schema 修复策略——MaxLen 提升 + Nillable 对齐

**选择**：
- `id` 字段 MaxLen 从 64 提升到 192（`teamID:graphID` 格式，每段 64 + 分隔符）
- `updated_at` 移除 `Optional().Nillable()`，改为 `Default(time.Now)` 与实际使用对齐

**理由**：Ent Schema 应与手写 DDL 保持一致，避免未来切换 Ent Client 时行为差异。

### D7：linked graph + adaptive 修复策略——在 finalizeRuntimeGraphConfig 之前调用

**选择**：在 `CompileToCompiledTeam` linked graph 路径中，`finalizeRuntimeGraphConfig` 之前添加 adaptive mode 检查和 `applyAdaptiveAgentDestinations` 调用

**理由**：
- 非 linked 路径已正确处理 adaptive mode
- linked graph 路径直接跳到 `finalizeRuntimeGraphConfig`，遗漏了 adaptive 处理
- 修复位置应在 `finalizeRuntimeGraphConfig` 之前，因为 `finalizeRuntimeGraphConfig` 可能依赖 Destinations 字段

## Risks / Trade-offs

### [R1] execMu 锁顺序变更可能引入死锁 → 缓解：遵循 uc.mu → execMu 顺序，必要时先释放 uc.mu

### [R2] BuildDeps 一次性替换影响面大 → 缓解：所有构建函数签名同步修改，`make wire && make build` 验证

### [R3] team 层接口化增加 biz 层接口数量 → 缓解：窄接口方法数 ≤5，按职责域拆分

### [R4] 删除 CircuitBreakerState 后无法快速恢复熔断功能 → 缓解：编译期 RetryMaxAttempts 已覆盖重试需求，熔断功能需独立设计后重新引入

### [R5] Ent Schema 变更需要 DDL 迁移 → 缓解：MaxLen 变更不影响现有数据（ID 长度未超 192），仅约束未来数据

### [R6] linked graph + adaptive 修复可能影响现有行为 → 缓解：当前该路径下 adaptive 模式不生效，修复是纯增量

## Migration Plan

1. **M1（并发安全）**：单文件修复，可独立回滚
2. **M2（功能性 Bug）**：单行修复，可独立回滚
3. **M3（死代码清理）**：删除 circuit_breaker.go + cbState 传递链路，需同步修改 builder/node_wiring/runtime_adapter
4. **M4（Resolver 集成）**：BuildDeps → GraphNodeResolverSet 迁移，需同步修改所有构建函数 + wireNode
5. **M5（接口化）**：biz 层新增接口 + team 层修改构造函数 + Wire 绑定更新
6. **M6（红线 + Schema + 规范）**：分散修复，互不依赖

每个 Milestone 完成后验证：`make wire && make build && make test`

## Open Questions

1. CircuitBreaker 删除后，是否需要在 M60（Spirit Parallel Orchestrator）中重新设计熔断机制？
2. `KnowledgeFacade` 接口化后，`knowledge` 包是否应移到 `internal/biz/knowledge` 以保持依赖方向？
3. `NewRunner` 的 6 个 Usecase 接口化后，是否应合并部分接口（如 `TeamUsageQuerier` + `TeamSessionManager` → `TeamRuntimeAccess`）？
