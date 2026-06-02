## 1. M1: Fix P0 Business Bugs

- [ ] 1.1 BUG-01: Add negation detection to `criticLoopCondFunc` in `internal/graph/adapter/critic_loop_cond.go` — add `containsNegationBeforeWord()` helper, update condition to reject "not approved" / "denied" / "rejected". DoD: `"not approved"` returns false; `"approved"` returns true; unit test passes
- [ ] 1.2 BUG-02: Fix `finishRunErr` double event publish in `internal/team/runner_helpers.go:110-121` — remove `TeamRunFinished` publish on error path, keep only `TeamRunFailed`. DoD: failure path publishes exactly 1 event (`TeamRunFailed`)
- [ ] 1.3 BUG-03: Fix non-deterministic entry point in `internal/team/embedded_graph.go:193-198` — sort `executableIDs` keys before selecting entry point. DoD: same definition produces same entry point across multiple runs
- [ ] 1.4 BUG-04: Fix Fallback Agent bypass in `internal/graph/trpc/failure_recovery.go:33-35` — replace `NewAgentNodeFunc(fallback)` with `deps.Agents.ResolveAgent(ctx, fallback)`. DoD: fallback agent gets tools and model config from resolver
- [ ] 1.5 BUG-05: Fix hardcoded "key-" prefix in `internal/team/team_graph_run_finisher.go:128` — replace `"key-" + agentID` with real AgentKey from catalog. DoD: HITL resume correctly associates steps to nodes
- [ ] 1.6 BUG-06: Fix nil logger in `internal/graph/adapter/runtime_adapter.go:212` — pass `lg` parameter instead of `nil` to `NewEventBridge`. DoD: JSON deserialization failure logs error instead of panic
- [ ] 1.7 BUG-07: Fix EventBridge per-event reconstruction in `internal/graph/adapter/runtime_adapter.go:210-213` — hold single `EventBridge` instance in `trpcGraphRuntime`, reuse across events. DoD: `execution_summary` contains all node records
- [ ] 1.8 M1 verification: `make api && make wire && make build && make test && make lint`

## 2. M2: Fix P1 Race Conditions and State Safety

- [ ] 2.1 ARCH-07: Fix TOCTOU race in `internal/biz/graph_execution.go:304-315` — copy data inside lock, write DB outside lock. DoD: no data race detected by `go test -race`
- [ ] 2.2 ARCH-08: Cancel runtime before GC eviction in `internal/biz/graph.go:291-296` — call `exec.runtime.Cancel()` before evicting from cache. DoD: no zombie goroutines after GC eviction
- [ ] 2.3 ARCH-10: Add per-execution mutex in `internal/biz/graph_team_execution.go:68-75` — replace shared pointer with `sync.RWMutex` per `GraphExecution`. DoD: `MarkTeamGraphInterrupt` concurrent access is safe
- [ ] 2.4 ARCH-06: Bind Circuit Breaker to GraphAgent instance in `internal/graph/trpc/circuit_breaker.go` + `builder.go` — replace package-level global map with instance-level map. DoD: no cross-session pollution; bounded growth
- [ ] 2.5 ARCH-09: Add recovery path for evicted team build configs in `internal/biz/graph_team_execution.go` — as interim fix, reload from DB or recompile. DoD: resume succeeds after GC eviction (will be fully replaced by M3.8)
- [ ] 2.6 M2 verification: `make api && make wire && make build && make test && make lint`

## 3. M3: Introduce CompiledTeam — Break Coupling Root

- [ ] 3.1 Define `CompiledTeam`, `RoleInfo`, `NodeTaskMeta` structs in `internal/biz/compiled_team.go` (new file). DoD: structs compile; `CompiledTeam` embeds `GraphBuildConfig`; `NodeTaskMeta` has 8 fields; `RoleInfo` has 5 fields + `Capabilities []string`
- [ ] 3.2 FailurePolicy compile-time expansion: in `internal/biz/failure_policy.go`, ensure `ApplyFailurePolicy` writes `NodeDef.FailureAction/FallbackAgent/RetryMaxAttempts`; in `internal/biz/graph.go`, remove `FailurePolicy *TeamFailurePolicy` field from `GraphBuildConfig`; update `internal/graph/trpc/builder.go` and `node_wiring.go` to consume expanded fields only. DoD: `GraphBuildConfig` has 12 fields (down from 13); `graph/trpc` no longer imports `biz.TeamFailurePolicy`
- [ ] 3.3 ParallelBranchIDs compile-time expansion: in `internal/biz/failure_policy.go`, expand parallel branch nodes to `FailureAction="skip_on_failure"`; remove `ParallelBranchIDs []string` from `GraphBuildConfig`. DoD: `GraphBuildConfig` has 11 fields; parallel branch failure behavior preserved
- [ ] 3.4 CircuitBreaker compile-time expansion: in `internal/graph/trpc/node_wiring.go`, write `RetryMaxAttempts` from breaker config; `nodeOptions` no longer receives `*biz.TeamFailurePolicy`. DoD: circuit breaker effect preserved via `RetryMaxAttempts`; `nodeOptions` has no Team policy dependency
- [ ] 3.5 Task field separation: move 8 Task fields from `NodeDef` to `NodeTaskMeta` in `internal/biz/graph.go`; update `internal/biz/graph_task_input.go`, `internal/team/embedded_graph.go`, `internal/graph/trpc/builder.go`. DoD: `NodeDef` has 20 fields; Task coordination logic reads from `CompiledTeam.TaskMeta`
- [ ] 3.6 Replace `CompileToGraphRuntimeConfig` with `CompileToCompiledTeam` in `internal/team/graph_runtime_config.go` + `internal/team/graph_compile.go` (new file). DoD: compilation produces `CompiledTeam`; all callers updated
- [ ] 3.7 Generate `RoleManifest` during compilation in `internal/team/graph_compile.go` — collect AgentKey/DisplayName/Role from catalog for each node. DoD: `CompiledTeam.RoleManifest` populated; `RoleInfo.AgentKey` matches runtime `ag.AgentKey`
- [ ] 3.8 Persist `CompiledTeam` to DB: add Ent schema for `compiled_team` table in `internal/data/ent/schema/`; implement `CompiledTeamRepo` in `internal/data/compiled_team_repo.go`; replace `teamBuildConfigs` in-memory cache in `internal/biz/graph_team_execution.go`. DoD: Team Graph survives process restart; GC eviction no longer breaks resume
- [ ] 3.9 M3 verification: `make api && make wire && make build && make test && make lint`; verify `graph/trpc` only imports `biz` constants (not `TeamFailurePolicy`)

## 4. M4: Graph Independence + biz/trpc Type Unification

- [ ] 4.1 biz/trpc type unification: in `internal/graph/trpc/builder.go`, replace dual-defined types with embedding + aliases — `type EdgeDef = biz.EdgeDef`, `type StateFieldDef = biz.StateFieldDef`, `type GraphBuildConfig = biz.GraphBuildConfig`; for `NodeDef` and `ConditionalEdgeDef`, embed biz type + add trpc-specific fields. DoD: 6 dual-source types eliminated
- [ ] 4.2 Remove `bizCfgToTrpc`/`trpcCfgToBiz` functions from `internal/graph/adapter/runtime_adapter.go` — update callers to use unified types directly. DoD: ~93 lines of manual mapping code removed
- [ ] 4.3 Split `GraphBuilderFactory` into 4 narrow interfaces in `internal/biz/graph_runtime.go` + `internal/graph/adapter/`: `GraphRunnerFactory` (3 methods), `GraphVisualizer` (1 method), `GraphValidator` (1 method), `GraphTemplateProvider` (3 methods), `GraphNodeInfoProvider` (2 methods). DoD: no interface exceeds 5 methods
- [ ] 4.4 Split `GraphRepo` into `GraphReader` + `GraphWriter` in `internal/biz/graph.go` + `internal/data/`. DoD: read and write paths separated
- [ ] 4.5 Split `GraphRuntime` into execution control + checkpoint in `internal/biz/graph_runtime.go`. DoD: execution methods and checkpoint methods on separate interfaces
- [ ] 4.6 M4 verification: `make api && make wire && make build && make test && make lint`

## 5. M5: Team Lifecycle Unification + Runner Refactoring

- [ ] 5.1 Define `RunnerConfig` in `internal/team/runner_config.go` — consolidate 10 non-circular dependency fields into single struct. DoD: Runner constructor accepts `RunnerConfig`
- [ ] 5.2 Implement `TeamRunMediator` in `internal/team/runner_mediator.go` — mediate Runner ↔ Coordinator bidirectional dependency. DoD: only 2 Setter methods remain on Runner
- [ ] 5.3 Encapsulate Knowledge 4 fields into `KnowledgeFacade` in `internal/team/runner.go` + `internal/knowledge/`. DoD: Runner holds single `knowledgeFacade` field
- [ ] 5.4 Split `GraphUsecase` into `GraphDefinitionUsecase` + `GraphExecutionUsecase` + `GraphCacheManager` in `internal/biz/graph.go` + `internal/biz/graph_*.go`. DoD: each usecase has ≤3 responsibilities
- [ ] 5.5 Eliminate `any` return types in `internal/biz/graph_runtime.go` — define biz-layer value types. DoD: no `any` returns in GraphRuntime interfaces
- [ ] 5.6 Optimize Team port interfaces (DES-03/04/05) in `internal/biz/team_ports.go` + `internal/biz/team_agent_ports.go`. DoD: `TeamAgentHelper` split into narrow interfaces; `TeamTurnRuntime`/`TeamBuildRunner` merged or clarified; `TeamTurnResult` decoupled from `ChatMessage`
- [ ] 5.7 Integrate or clean up `FunctionResolver` (DES-08/09) in `internal/graph/trpc/node_wiring.go` + `internal/graph/trpc/build_deps.go`. DoD: either wired into DI or removed
- [ ] 5.8 Batch fix P3 code quality issues (Q-01~Q-11) across multiple files. DoD: all P3 items resolved
- [ ] 5.9 M5 verification: `make api && make wire && make build && make test && make lint`

## 6. Documentation Sync + Review

- [ ] 6.1 Update `docs/team-graph问题与方案.md` — mark completed items, update field counts, add implementation notes
- [ ] 6.2 Update `docs/architecture-blueprint.md` — reflect CompiledTeam, new interfaces, type unification
- [ ] 6.3 Update `docs/module-cross-reference.md` — update Team/Graph module cards with new dependencies
- [ ] 6.4 Run `aranea-review` skill on all modified files
