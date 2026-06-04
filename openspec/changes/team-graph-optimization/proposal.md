## Why

Team 和 Graph 模块之间存在 7 个耦合点，导致 `GraphBuildConfig` 成为承载过多关注点的上帝结构体（13 字段混合图拓扑、Team 领域概念、Task 元数据、运行时行为），引发因果链：类型双源定义 → Factory 膨胀 → God Usecase → God Object Runner → 12 个 Setter 注入绕过 Wire → 运行时 Bug 频发。同时存在 7 个 P0 业务逻辑 Bug（否定语义误匹配、双重事件发布、非确定性入口点、Fallback 绕过解析管线、硬编码 key 前缀、nil logger panic、EventBridge 重建导致摘要丢失）和 10 个 P1 架构级缺陷（竞态条件、全局可变状态、内存缓存驱逐导致恢复失败）。行业调研（LangGraph/Microsoft/AWS/MAN+ESM）确认 Team（声明式）→ Graph（执行式）编译模式是最佳实践，当前架构方向正确但解耦不彻底。

## What Changes

- **BUG-01~07**：修复 7 个 P0 业务逻辑 Bug
- **ARCH-06~10**：修复 5 个 P1 竞态/状态安全问题
- **BREAKING** 引入 `CompiledTeam` 结构体，作为 Team↔Graph 的编译产物桥梁
- **BREAKING** `FailurePolicy` 编译期展开为 NodeDef 通用字段，从 `GraphBuildConfig` 移除 `FailurePolicy` 和 `ParallelBranchIDs`（注：这两个字段从未在 `GraphBuildConfig` 上定义，实际是编译管线参数的变更）
- **BREAKING** NodeDef 的 8 个 Task 字段分离到 `NodeTaskMeta`
- 新增 `RoleManifest` 解决"角色语义编译后丢失"问题
- `CompiledTeam` 持久化替换 `teamBuildConfigs` 内存缓存
- biz/trpc 类型统一：trpc 层嵌入 biz 类型 + 类型别名，消除双源定义和手工映射
- `GraphBuilderFactory` 拆分为 4 个窄接口
- Runner 同包拆分 + `RunnerConfig` 替代 10 个非循环 Setter + `TeamRunMediator` 解决双向绑定
- GraphUsecase 拆分为 Definition + Execution + CacheManager

## Capabilities

### New Capabilities

- `compiled-team`: CompiledTeam 编译产物结构体、编译管线、持久化机制，作为 Team↔Graph 的解耦桥梁
- `role-manifest`: 角色清单生成与查询，解决 Team 角色语义在编译后丢失的问题
- `team-mediator`: TeamRunMediator 解决 Runner ↔ Coordinator 结构性双向依赖

### Modified Capabilities

- `architecture`: GraphBuildConfig 实际 12 字段（`FailurePolicy`/`ParallelBranchIDs` 从未在此结构体上定义，新增 `TaskMeta` 字段），NodeDef 从 28 字段降至 20 字段

## Impact

- **biz 层**：`graph.go`（GraphBuildConfig/NodeDef 结构体变更）、`graph_runtime.go`（Factory 拆分）、`graph_execution.go`（竞态修复）、`graph_team_execution.go`（CompiledTeam 持久化）、新增 `compiled_team.go`
- **team 层**：`runner.go`（拆分 + RunnerConfig）、`graph_runtime_config.go`（CompileToCompiledTeam）、`runner_helpers.go`（BUG-02）、`embedded_graph.go`（BUG-03）、`team_graph_run_finisher.go`（BUG-05）、`team_graph_run_coordinator.go`（竞态修复）
- **graph/adapter 层**：`critic_loop_cond.go`（BUG-01）、`runtime_adapter.go`（BUG-06/07 + 类型统一）
- **graph/trpc 层**：`builder.go`（类型统一）、`failure_recovery.go`（BUG-04）、`circuit_breaker.go`（ARCH-06）、`node_wiring.go`（FailurePolicy 展开）
- **data 层**：新增 `compiled_team_repo.go`
- **前端**：无直接影响（前端消费持久化模型，不消费编译模型）
- **API/Proto**：无变更
