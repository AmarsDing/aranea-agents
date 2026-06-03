## 1. M1: Fix P0 Business Bugs

> **状态**: ✅ 核心任务已完成（M1-M5 + Additional Improvements 全部通过验证）
> **剩余项**: 4.4/5.7/5.8 已标记 deferred（低价值）；7.1/7.2 文档同步待做

- [x] 1.1 BUG-01: Add negation detection to `criticLoopCondFunc` in `internal/graph/adapter/critic_loop_cond.go`
- [x] 1.2 BUG-02: Fix `finishRunErr` double event publish in `internal/team/runner_helpers.go`
- [x] 1.3 BUG-03: Fix non-deterministic entry point in `internal/team/embedded_graph.go`
- [x] 1.4 BUG-04: Fix Fallback Agent bypass in `internal/graph/trpc/failure_recovery.go`
- [x] 1.5 BUG-05: Fix hardcoded "key-" prefix in `internal/team/team_graph_run_finisher.go`
- [x] 1.6 BUG-06: Fix nil logger in `internal/graph/adapter/runtime_adapter.go`
- [x] 1.7 BUG-07: Fix EventBridge per-event reconstruction in `internal/graph/adapter/runtime_adapter.go`
- [x] 1.8 M1 verification: passed

## 2. M2: Fix P1 Race Conditions and State Safety

- [x] 2.1 ARCH-07: Fix TOCTOU race — copy data inside lock, write DB outside lock
- [x] 2.2 ARCH-08: Cancel runtime before GC eviction — two-phase GC (mark evicted, Cancel+persist outside lock)
- [x] 2.3 ARCH-10: Add per-execution mutex (`execMu sync.RWMutex`) — `MarkTeamGraphInterrupt` uses `execMu`
- [x] 2.4 ARCH-06: Bind Circuit Breaker to GraphAgent instance — instance-level `CircuitBreakerState` map
- [x] 2.5 ARCH-09: Recovery path for evicted team build configs — `buildConfigForExecution` loads from `compiledTeamRepo`
- [x] 2.6 M2 verification: passed

## 3. M3: Introduce CompiledTeam — Break Coupling Root

- [x] 3.1 Define `CompiledTeam`, `RoleInfo`, `NodeTaskMeta` structs in `internal/biz/compiled_team.go`
- [x] 3.2 FailurePolicy compile-time expansion — `graph/trpc` no longer imports `biz.TeamFailurePolicy`
- [x] 3.3 ParallelBranchIDs compile-time expansion — removed from `GraphBuildConfig`
- [x] 3.4 CircuitBreaker compile-time expansion — `NodeBreakerConfig` local value object replaces `biz.CircuitBreakerPolicy`
- [x] 3.5 Task field separation — `NodeTaskMeta` map in `GraphBuildConfig`
- [x] 3.6 Replace `CompileToGraphRuntimeConfig` with `CompileToCompiledTeam`
- [x] 3.7 Generate `RoleManifest` during compilation
- [x] 3.8 Persist `CompiledTeam` to DB — `CompiledTeamRepo` + `compiled_team` table
- [x] 3.9 M3 verification: passed

## 4. M4: Graph Independence + biz/trpc Type Unification

- [x] 4.1 biz/trpc type unification — `NodeDef`/`EdgeDef`/`ConditionalEdgeDef`/`StateFieldDef`/`GraphBuildConfig` embedded or aliased
- [x] 4.2 Remove `bizCfgToTrpc`/`trpcCfgToBiz` mapping functions from `runtime_adapter.go`
- [x] 4.3 Split `GraphBuilderFactory` into 5 narrow interfaces — `GraphRunnerFactory`/`GraphVisualizer`/`GraphValidator`/`GraphTemplateProvider`/`GraphNodeInfoProvider`
- [ ] 4.4 Split `GraphRepo` into `GraphReader` + `GraphWriter` — deferred (low value, current interface adequate)
- [x] 4.5 Split `GraphRuntime` into execution + checkpoint — `GraphRuntime` (4 methods) + `GraphCheckpointRuntime` (adds 4 TimeTravel methods)
- [x] 4.6 M4 verification: passed

## 5. M5: Team Lifecycle Unification + Runner Refactoring

- [x] 5.1 Define `RunnerConfig` in `internal/team/runner_config.go`
- [x] 5.2 Implement `TeamRunMediator` in `internal/team/runner_mediator.go` — `TeamGraphCoordAccess` interface breaks Runner ↔ Coordinator circular dependency
- [x] 5.3 Encapsulate Knowledge 4 fields into `KnowledgeFacade` in `internal/team/runner_config.go`
- [x] 5.4 Split `GraphUsecase` into `GraphDefinitionUsecase` + `GraphExecutionUsecase` — `GraphUsecase` delegates definition methods to `defUC`
- [x] 5.5 Eliminate `any` return types in `internal/biz/graph_runtime.go` — 6 biz-layer value objects: `StateSnapshot`/`CheckpointHistory`/`CheckpointInfo`/`CheckpointRef`/`CheckpointList`/`VisualGraph`/`GraphTemplateInfo`
- [x] 5.6 Optimize Team port interfaces — `TeamAgentHelper` split into `TeamOptionsBuilder` + `TeamDisplayHelper` + `TeamAuthHelper`
- [ ] 5.7 Integrate or clean up `FunctionResolver` — deferred (low impact, no current consumers)
- [ ] 5.8 Batch fix P3 code quality issues — deferred (incremental improvement, not blocking)
- [x] 5.9 M5 verification: passed

## 6. Additional Improvements

- [x] 6.1 Eliminate graph/trpc reverse dependency on `biz.TeamFailurePolicy`/`biz.CircuitBreakerPolicy` — `NodeBreakerConfig` local value object
- [x] 6.2 Split `GraphRuntime` into execution + checkpoint sub-interfaces (ISP)
- [x] 6.3 Eliminate `any` return types in `GraphRuntime` — typed biz value objects
- [x] 6.4 `GraphUsecase` 拆分 — `GraphDefinitionUsecase` handles CRUD; `GraphUsecase` delegates and retains execution
- [x] 6.5 Split `TeamAgentHelper` into narrow sub-interfaces (ISP)
- [x] 6.6 Implement `TeamRunMediator` for Runner ↔ Coordinator — `TeamGraphCoordAccess` interface
- [x] 6.7 Fix `EdgeDef.Kind` comment + `EdgeKindTransfer` constant
- [x] 6.8 Fix `GraphExecution` accessor methods — `GetStatus()`/`IsEvicted()`/`GetCurrentNode()`/`Snapshot()`
- [x] 6.9 Fix `graph_runtime_e2e_test.go`/`parity_runtime_test.go` NodeDef struct literal construction

## 7. Documentation Sync + Review

- [ ] 7.1 Update OpenSpec specs to reflect completed changes
- [ ] 7.2 Run `aranea-review` skill on all modified files
