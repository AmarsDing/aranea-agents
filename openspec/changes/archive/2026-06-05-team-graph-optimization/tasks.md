## 1. M1: Fix P0 Business Bugs

- [x] 1.1 BUG-01: Add negation detection to `criticLoopCondFunc` — implemented
- [x] 1.2 BUG-02: Fix `finishRunErr` double event publish — implemented
- [x] 1.3 BUG-03: Fix non-deterministic entry point — implemented
- [x] 1.4 BUG-04: Fix Fallback Agent bypass — implemented (DEV-12 noted incomplete)
- [x] 1.5 BUG-05: Fix hardcoded "key-" prefix — verified correct
- [x] 1.6 BUG-06: Fix nil logger — implemented
- [x] 1.7 BUG-07: Fix EventBridge per-event reconstruction — implemented
- [x] 1.8 M1 verification: passed

## 2. M2: Fix P1 Race Conditions and State Safety

- [x] 2.1 ARCH-07: Fix TOCTOU race — implemented (execMu + SnapshotForPersist)
- [x] 2.2 ARCH-08: Cancel runtime before GC eviction — implemented
- [x] 2.3 ARCH-10: Add per-execution mutex — implemented
- [x] 2.4 ARCH-06: Bind Circuit Breaker to instance — implemented (dead code noted in DEV-18, fixed in 3.4)
- [x] 2.5 ARCH-09: Recovery path for evicted configs — implemented
- [x] 2.6 M2 verification: passed

## 3. M3: Introduce CompiledTeam — Break Coupling Root

- [x] 3.1 Define `CompiledTeam`, `RoleInfo`, `NodeTaskMeta` structs — implemented. DEV-01 fixed: TaskMeta moved from GraphBuildConfig to CompiledTeam
- [x] 3.2 FailurePolicy compile-time expansion — implemented. `ApplyFailurePolicy` fully expanded; `GraphBuildConfig` never had `FailurePolicy` field (DEV-02)
- [x] 3.3 ParallelBranchIDs compile-time expansion — implemented. `ApplyParallelFailContinue` fully expanded; `ParallelBranchIDs` never on `GraphBuildConfig` (DEV-02)
- [x] 3.4 CircuitBreaker compile-time expansion — implemented. Removed `*biz.TeamFailurePolicy` from `nodeOptions`/`wireNode`; deleted dead `circuitBreakerOptions`; `graph/trpc` no longer imports `biz.TeamFailurePolicy`
- [x] 3.5 Task field separation — implemented. NodeDef 20 fields, TaskMeta on CompiledTeam
- [x] 3.6 Replace `CompileToGraphRuntimeConfig` with `CompileToCompiledTeam` — implemented
- [x] 3.7 Generate `RoleManifest` during compilation — implemented
- [x] 3.8 Persist `CompiledTeam` to DB — implemented (Ent Schema + raw SQL + DDL migration)
- [x] 3.9 M3 verification: `make wire && go build ./cmd/admin` passed; `graph/trpc` has zero `TeamFailurePolicy` imports

## 4. M4: Graph Independence + biz/trpc Type Unification

- [x] 4.1 biz/trpc type unification — completed. All 6 dual-source types unified:
  - `EdgeDef = biz.EdgeDef` (alias)
  - `StateFieldDef = biz.StateFieldDef` (alias)
  - `GraphBuildConfig = biz.GraphBuildConfig` (alias, 11 fields after DEV-01 fix)
  - `NodeDef` embeds `biz.NodeDef` + `Func trpcgraph.NodeFunc`
  - `ConditionalEdgeDef` embeds `biz.ConditionalEdgeDef` + `CondFunc any`
  - `SubgraphDef` embeds `biz.SubgraphDef` + `InputMapper`/`OutputMapper`
- [x] 4.2 Remove mapping code — completed. `bizCfgToTrpc`/`trpcCfgToBiz` eliminated; test mapping code simplified; `ExecutionEngineType` unified to biz constants
- [x] 4.3 Split `GraphBuilderFactory` into 5 narrow interfaces — implemented
- [x] 4.4 Split `GraphRepo` into `GraphReader` + `GraphWriter` — implemented. Composite `GraphRepo` preserved for backward compat
- [x] 4.5 Split `GraphRuntime` into `GraphExecutionControl` + `GraphCheckpoint` — implemented. Composite `GraphRuntime` preserved
- [x] 4.6 M4 verification: `make wire && go build ./cmd/admin` passed

## 5. M5: Team Lifecycle Unification + Runner Refactoring

- [x] 5.1 Define `RunnerConfig` — implemented
- [x] 5.2 Implement `TeamRunMediator` — fully integrated. Runner holds `mediator *TeamRunMediator` instead of `*TeamGraphRunCoordinator`; only 2 Setters remain (`SetMediator` + `SetAwaitHookProvider`); Wire updated
- [x] 5.3 Encapsulate Knowledge into `KnowledgeFacade` — implemented
- [x] 5.4 Split `GraphUsecase` — fully implemented. Three sub-usecases:
  - `GraphDefinitionUsecase` — definition CRUD + templates
  - `GraphExecutionUsecase` — execution lifecycle + GC + executions map
  - `GraphCacheManager` — teamBuildConfigs + CompiledTeamRepo
  - `GraphUsecase` is thin facade composing all three
- [x] 5.5 Eliminate `any` return types — fully implemented. 9 biz-layer value types defined:
  - `GraphCheckpointRef`, `GraphCheckpointInfo`, `GraphCheckpointState`, `GraphEditedState`, `GraphCheckpointList`
  - `GraphVisualization`, `GraphVisualizationNode`, `GraphVisualizationEdge`
  - `GraphTemplateRef`, `GraphTemplateNodeRef`, `GraphTemplateEdgeRef`
  - `GraphRawEvent`
- [ ] 5.6 Optimize Team port interfaces (DES-03/04/05) — deferred (low priority, high risk)
- [ ] 5.7 Integrate or clean up `FunctionResolver` (DES-08/09) — partially implemented (interface defined + DI wired, but wireNode not consuming deps.Functions)
- [ ] 5.8 Batch fix P3 code quality issues (Q-01~Q-11) — deferred
- [x] 5.9 M5 verification: `make wire && go build ./cmd/admin` passed

## 6. Documentation Sync + Review

- [ ] 6.1 Update `docs/team-graph问题与方案.md`
- [ ] 6.2 Update `docs/architecture-blueprint.md`
- [ ] 6.3 Update `docs/module-cross-reference.md`
- [ ] 6.4 Run `aranea-review` skill on all modified files
