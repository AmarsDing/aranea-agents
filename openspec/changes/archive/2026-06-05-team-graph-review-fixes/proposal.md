## Why

team-graph-optimization 变更归档后的深度代码审查发现 15 个阻断级问题：5 个并发竞态（`evictIfNeeded`/`gc()` 未持有 `execMu` 读写 exec 字段、`c.finisher` 无锁保护）、1 个功能性 Bug（linked graph + adaptive 模式下 `applyAdaptiveAgentDestinations` 被跳过）、3 个架构违规（team 层依赖 biz/infra 具体类型而非接口）、1 个熔断状态机全链路空转（CircuitBreakerState 为死代码）、3 个 Agent 运行时死代码/未集成路径（GraphNodeResolverSet.Functions/Subgraphs 未消费、BuildDeps 标记 Deprecated 但仍为核心参数）、1 个红线违反（`loggateway.Global()` 使用）、1 个 Ent Schema 与手写 DDL 不一致。这些问题不修复将导致运行时数据竞争、功能缺失和架构退化。

## What Changes

- **FIX-01~03**：修复 `evictIfNeeded()`/`gc()`/`consumeRuntimeEvents` 中 5 个并发竞态——在读写 `GraphExecution` 字段时正确持有 `execMu`
- **FIX-04**：修复 `TeamGraphRunCoordinator.finisher` 无锁保护——添加 `sync.Once` 或互斥保护
- **FIX-05**：修复 `CompileToCompiledTeam` linked graph 路径跳过 adaptive mode 处理——在 `finalizeRuntimeGraphConfig` 之前添加 `applyAdaptiveAgentDestinations` 调用
- **FIX-06**：清理 CircuitBreakerState 死代码——删除 `circuit_breaker.go` 及 `cbState` 传递链路，或正确注册 `afterNode` 回调
- **FIX-07**：完成 `GraphNodeResolverSet` 集成——将 `BuildDeps` 迁移为 `GraphNodeResolverSet`，在 `wireNode` 中消费 `deps.Functions`，删除 `BuildDeps` 及转换方法
- **FIX-08**：修复 team 层架构违规——为 `KnowledgeFacade`/`RunnerConfig` 中的具体类型抽取 biz 接口，`NewRunner` 接收窄接口而非 `*biz.XxxUsecase` 具体类型
- **FIX-09**：修复 `loggateway.Global()` 红线违反——改为构造注入 `loggateway.Logger`
- **FIX-10**：修复 Ent Schema 与手写 DDL 不一致——`id` 字段 MaxLen 提升到 192，`updated_at` Nillable 与实际使用对齐
- **FIX-11**：修复 `biztool.SetGlobalWebResearchChecker` 全局副作用——改为构造注入
- **CODE-01~08**：编程规范修复——magic string/number 定义常量、参数超限引入 Option struct、`sql.ErrNoRows` 改用 `errors.Is`、`INSERT OR REPLACE` 改为 `ON CONFLICT DO UPDATE`、`LoadForSession` 空 sessionID 校验、JSON 反序列化添加版本字段和大小限制、补充 CompiledTeamRepo 单元测试

## Capabilities

### New Capabilities

- `concurrency-safety`: GraphExecution/TeamGraphRunCoordinator 并发竞态修复，确保 execMu/finisher 字段访问安全
- `circuit-breaker-cleanup`: CircuitBreakerState 死代码清理，决定激活或删除熔断状态机
- `resolver-integration`: GraphNodeResolverSet 完整集成，替代 Deprecated BuildDeps，wireNode 消费 Functions/Subgraphs 解析器
- `team-interface-abstraction`: team 层依赖接口化——KnowledgeFacade/RunnerConfig/NewRunner 抽取 biz 窄接口

### Modified Capabilities

- `architecture`: GraphBuildConfig 类型统一已完成但 trpc 层常量引用需规范化；Ent Schema 与 DDL 对齐；loggateway.Global 消除

## Impact

- **biz 层**：`graph_execution_usecase.go`（竞态修复）、`graph.go`（常量定义）、`graph_runtime.go`（参数结构体）
- **team 层**：`runner.go`/`runner_config.go`（接口化重构）、`runner_mediator.go`（并发保护）、`graph_compile.go`（adaptive 修复）、`team_graph_run_coordinator.go`（finisher 锁保护）
- **graph/trpc 层**：`circuit_breaker.go`（清理/激活）、`build_deps.go`（BuildDeps 迁移）、`node_wiring.go`（消费 Functions）、`builder.go`（cbState 传递链路）
- **data 层**：`compiled_team_repo.go`（SQL 修复、校验增强）、`ent/schema/compiled_team.go`（MaxLen 修复）
- **cmd/admin**：`wire.go`（Global 消除、SetGlobalWebResearchChecker 消除、接口绑定更新）
- **API/Proto**：无变更
- **前端**：无直接影响
