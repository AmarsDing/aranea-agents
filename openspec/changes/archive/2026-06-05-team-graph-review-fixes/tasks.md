## 1. M1: Concurrency Safety — GraphExecution execMu

- [x] 1.1 Fix `evictIfNeeded()`: acquire `exec.execMu.RLock()` before reading `exec.Status`/`exec.FinishedAt`/`exec.StartedAt`/`exec.runtime`, release after reading. Follow lock order: release `uc.mu` → acquire `execMu` → read → release `execMu` → re-acquire `uc.mu` if needed. DoD: no exec field read without `execMu` in `evictIfNeeded`
- [x] 1.2 Fix `gc()`: acquire `exec.execMu.Lock()` before modifying `exec.Status`/`exec.FinishedAt`/`exec.runtime` and calling `exec.runtime.Cancel()`. DoD: no exec field write without `execMu` in `gc`
- [x] 1.3 Fix `consumeRuntimeEvents`: move `wasEvicted := exec.evicted` read inside `execMu`-protected section. DoD: `evicted` read only while holding `execMu`
- [x] 1.4 Add `NewGraphExecution()` factory function that enforces `ctx` initialization. Replace all `&GraphExecution{}` calls with `NewGraphExecution()`. DoD: zero direct `&GraphExecution{}` constructions
- [x] 1.5 M1 verification: `go build ./cmd/admin && go test ./internal/biz/... -run TestGraph -count=1`

## 2. M2: Concurrency Safety — Coordinator.finisher

- [x] 2.1 Add `sync.Once` to `TeamGraphRunCoordinator` for `finisher` field. Change `SetFinisher` to use `sync.Once.Do`. DoD: `SetFinisher` is concurrency-safe
- [x] 2.2 Add startup-order comment to `SetFinisher`/`SetCoordinator` documenting that `RecoverSessions` must be called after wiring completes. DoD: comment present
- [x] 2.3 M2 verification: `go build ./cmd/admin && go test ./internal/team/... -count=1`

## 3. M3: Functional Bug — Linked Graph + Adaptive Mode

- [x] 3.1 In `CompileToCompiledTeam` linked graph path (`graph_compile.go:197-203`), add adaptive mode check before `finalizeRuntimeGraphConfig`: if `normalizeCompileMode(def.Mode) == "adaptive"` then call `applyAdaptiveAgentDestinations(cfg)`. DoD: linked + adaptive path sets node Destinations
- [ ] 3.2 Add test case for linked graph + adaptive mode compilation. DoD: test passes (deferred — requires mock infrastructure)
- [x] 3.3 M3 verification: `go build ./cmd/admin && go test ./internal/team/... -run TestCompile -count=1`

## 4. M4: CircuitBreaker Dead Code Cleanup

- [x] 4.1 Delete `internal/graph/trpc/circuit_breaker.go` and its test file. DoD: file deleted
- [x] 4.2 Remove `cbState` parameter from `buildFromResolved`, `wireNode`, `nodeOptions` signatures. DoD: no `cbState` parameter in these functions
- [x] 4.3 Remove `cbState` creation from `BuildStateGraphWithAgents`. DoD: no `NewCircuitBreakerState()` call
- [x] 4.4 Remove `cbState` parameter from `NewGraphAgent`/`NewGraphAgentWithSubAgents`/`NewGraphAgentWithSaver`/`NewGraphAgentWithEngine`. DoD: no `cbState` in any `NewGraphAgent*` signature
- [x] 4.5 Remove `cbState` from `runtime_adapter.go` `createAgent` function. DoD: no `cbState` reference in adapter
- [x] 4.6 M4 verification: `go build ./cmd/admin && go test ./internal/graph/... -count=1`

## 5. M5: GraphNodeResolverSet Integration

- [x] 5.1 Change all build function signatures (`BuildStateGraph`/`BuildStateGraphWithRegistry`/`BuildStateGraphWithRegistryAndLogger`/`BuildStateGraphWithAgents`/`buildFromResolved`) from `*BuildDeps` to `*GraphNodeResolverSet`. DoD: all signatures use `*GraphNodeResolverSet`
- [x] 5.2 Update `wireNode` to accept `*GraphNodeResolverSet` and use `deps.Functions.ResolveFunction(ctx, funcRef)` for `"function"` type nodes with fallback to `FuncRef`. DoD: function nodes resolved via FunctionResolver
- [x] 5.3 Delete `BuildDeps` struct and `ToBuildDeps()`/`ToBuildDepsPtr()` methods from `build_deps.go`. DoD: no `BuildDeps` references
- [x] 5.4 Remove `Subgraphs SubgraphResolver` field from `GraphNodeResolverSet` (no implementation exists). Delete `SubgraphResolver` interface. DoD: no `SubgraphResolver` references
- [x] 5.5 Update `runtime_adapter.go` to pass `GraphNodeResolverSet` directly (no `ToBuildDepsPtr()` conversion). DoD: adapter uses `GraphNodeResolverSet`
- [x] 5.6 Update Wire providers in `wire.go` if needed. DoD: `make wire` passes
- [x] 5.7 M5 verification: `make wire && go build ./cmd/admin && go test ./internal/graph/... -count=1`

## 6. M6: Team Layer Interface Abstraction

- [x] 6.1 Define narrow interfaces in biz layer: `TeamUsageQuerier`, `TeamSessionManager`, `TeamAgentLookup`, `TeamToolLookup`, `TeamModelCatalog`, `TeamSkillLookup`. Each ≤5 methods covering only what Runner calls. DoD: interfaces defined in `internal/biz/team_interfaces.go`
- [x] 6.2 Add compile-time assertions: `var _ TeamUsageQuerier = (*UsageUsecase)(nil)` etc. DoD: assertions pass
- [x] 6.3 Change `NewRunner` signature to accept narrow interfaces instead of `*biz.XxxUsecase`. DoD: no `*biz.XxxUsecase` in `NewRunner` signature
- [x] 6.4 Add Wire bindings in `wire.go`: `wire.Bind(new(biz.TeamUsageQuerier), new(*biz.UsageUsecase))` etc. DoD: `make wire` passes
- [ ] 6.5 Define `biz.KnowledgeRetriever`/`biz.KnowledgeRouter`/`biz.KnowledgeFederatedRetriever`/`biz.KnowledgeEvaluator` interfaces. Change `KnowledgeFacade` to hold interfaces. DoD: no `*knowledge.Xxx` in `KnowledgeFacade` — TECH-DEBT: deferred, knowledge API not stable
- [ ] 6.6 Define `biz.PluginRuntime`/`biz.PluginManagerAccess` interfaces. Change `RunnerConfig.PluginRT`/`PluginManager` to interfaces. DoD: no `*plugintrpc.Xxx` in `RunnerConfig` — TECH-DEBT: deferred, plugin API not stable
- [ ] 6.7 Add Wire bindings for Knowledge and Plugin interfaces. DoD: `make wire` passes — deferred with 6.5/6.6
- [x] 6.8 M6 verification: `make wire && go build ./cmd/admin && go test ./internal/team/... ./internal/biz/... -count=1`

## 7. M7: Red-Line Violations + Ent Schema + Code Quality

- [x] 7.1 Fix `loggateway.Global()` in `wire.go:695`: add `lg loggateway.Logger` field to `wsTurnExecutorAdapter`, inject via constructor. DoD: no `loggateway.Global()` in `wsTurnExecutorAdapter`
- [x] 7.2 Fix `biztool.SetGlobalWebResearchChecker`: add `WebResearchReadinessChecker` to `NewToolUsecase` constructor, remove `SetGlobalWebResearchChecker` call and function. DoD: no `SetGlobalWebResearchChecker` in production code
- [x] 7.3 Fix Ent Schema `compiled_team.go`: change `id` MaxLen from 64 to 192, remove `Optional().Nillable()` from `updated_at`. DoD: Ent Schema matches DDL behavior
- [x] 7.4 Fix `CompiledTeamRepo.Save`: replace `INSERT OR REPLACE` with `INSERT ... ON CONFLICT(id) DO UPDATE SET` to preserve `created_at`. DoD: `created_at` preserved on update
- [x] 7.5 Fix `CompiledTeamRepo.LoadForSession`: return `kerrors.BadRequest` when `sessionID` is empty. DoD: empty sessionID returns error
- [ ] 7.6 Define constants for magic strings/numbers: `GraphExecStatusRunning`/`Failed`/etc in `graph.go`, `DefaultRetryMaxAttempts = 3` in `failure_policy.go`, node type constants. DoD: no hardcoded status/type strings — deferred (low priority, high risk of breaking consumers)
- [x] 7.7 Fix `runner.go:133`: change `err == sql.ErrNoRows` to `errors.Is(err, sql.ErrNoRows)`. DoD: wrapped errors matched correctly
- [ ] 7.8 Add `CompiledTeamRepo` unit tests: Save/Load round-trip, LoadForSession empty sessionID, Delete. DoD: tests pass — deferred (requires DB test infrastructure)
- [x] 7.9 M7 verification: `make wire && make build && make test`

## 8. Final Verification

- [x] 8.1 Full validation: `go build ./cmd/admin && go test ./internal/biz/... ./internal/team/... ./internal/graph/... ./internal/data/... -count=1`
- [x] 8.2 Verify no `loggateway.Global()` in new code: `grep -rn "loggateway.Global()" internal/ cmd/`
- [x] 8.3 Verify no `*biz.XxxUsecase` concrete types in `NewRunner` signature
- [x] 8.4 Verify no `BuildDeps` references: `grep -rn "BuildDeps" internal/graph/`
- [x] 8.5 Verify no `cbState` references: `grep -rn "cbState" internal/graph/`
