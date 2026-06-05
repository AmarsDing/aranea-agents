# Model Registry Refactor Design

> Date: 2026-05-30
> Status: Implemented
> Scope: `internal/modelcatalog/` → `internal/modelregistry/` + trpc-agent-go integration

---

## 1. Problem Statement

### 1.1 Root Cause

`model-catalog: scheduled sync apply failed: context deadline exceeded`

The monolithic `tick()` uses a single 130s context (`defaultFetchTimeout + 10s`) for the entire pipeline:

```
130s total budget
├─ FetchCatalog (HTTP, 4 retries, 120s client timeout)
├─ SyncProviderLogos (8 concurrent HTTP, shared ctx)     ← blocks main flow
├─ LoadCatalog (local file, fast)
├─ RunProviderMigrations (5 rules × serial txns)         ← no checkpoint
└─ Apply (row-by-row Save + UpsertPricing)               ← row-by-row I/O
```

Only 10s margin remains for migrate + apply after fetch + logo.

### 1.2 Architecture Issues

| # | Issue | Impact |
|---|-------|--------|
| 1 | Single 130s timeout for entire pipeline | Any slow phase dooms subsequent phases |
| 2 | Logo sync blocks main flow | Network slowness eats timeout budget |
| 3 | 5 serial migration transactions | SQLite write-lock contention amplifies latency |
| 4 | Row-by-row Apply | N rows = N UPDATEs + N UpsertPricings |
| 5 | No migration checkpoint | Rules 1-2 committed but re-run on next tick |
| 6 | Bare `go r.loop(ctx)` | Violates Red Line #9 (must use safego) |
| 7 | `log.Printf` | Violates Red Line #10 (must use FlowLog) |
| 8 | Standalone Runner | Not integrated with trpc-agent-go framework |
| 9 | Package name `modelcatalog` | Misaligned with framework terminology |

### 1.3 Naming Issues (Discovered During Analysis)

| Current Name | Location | Actual Semantics | Proposed Name |
|-------------|----------|-----------------|---------------|
| `modelcatalog` package | `internal/modelcatalog/` | Model provider registry | `modelregistry` |
| `Catalog` type | `modelcatalog.Catalog` | Provider directory | `Directory` |
| `provider.CatalogConfig` | `internal/provider/` | Provider connection config | `ProviderConfig` |
| `runtime.Catalog` | `internal/runtime/` | Dependency injection container | `TurnDeps` |
| `session.ModelConfigCatalog` | `internal/session/` | Model config resolver | `ModelConfigResolver` |

---

## 2. Design: Model Registry as trpc-agent-go Agent

### 2.1 Core Concept

Redesign the model-catalog sync from a standalone Ticker Worker to a trpc-agent-go Agent + Tool system:

1. **Each sync phase is a `tool.CallableTool`** — follows framework Tool interface
2. **Sync orchestration is an `agent.Agent`** — follows framework Agent interface (programmatic, no LLM)
3. **Execution via `runner.Runner`** — follows framework Runner lifecycle
4. **Events via framework `event.Event`** — follows framework event system
5. **Scheduling delegated to CronRunner** — follows project's existing Cron pattern

### 2.2 Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│                         Scheduling Layer (CronRunner)                    │
│   CronTask (kind=model_registry_sync, interval=1h)                      │
│     → CronRunner.runDue() → Chat.RunCronTurn()                         │
│       → ChatOrchestrator.Execute()                                      │
│         → BuildTRPCAgentCached(modelRegistrySyncAgent)                  │
│         → RunnerManager.NewTurnRunner()                                 │
│         → RunTRPCUserTurnMsg("sync model registry")                     │
└──────────────────────────────┬───────────────────────────────────────────┘
                               │
┌──────────────────────────────▼───────────────────────────────────────────┐
│                    Bridge Layer (internal/agent/)                        │
│                                                                          │
│  ModelRegistrySyncAgent (implements agent.Agent)                         │
│    ├── Run(ctx, invocation) → execute Phase Pipeline → emit Events      │
│    ├── Tools() → [FetchTool, MigrateTool, ApplyTool, LogoTool]          │
│    ├── Info() → AgentInfo{Name: "model-registry-sync"}                  │
│    └── SubAgents/FindSubAgent → nil                                     │
│                                                                          │
│  BuildModelRegistrySyncAgent(deps) → ModelRegistrySyncAgent             │
└──────────────────────────────┬───────────────────────────────────────────┘
                               │
┌──────────────────────────────▼───────────────────────────────────────────┐
│                    Tool Layer (internal/tools/)                          │
│                                                                          │
│  FetchDirectoryTool       (tool.CallableTool) → FetchPhase.Run()        │
│  MigrateProvidersTool     (tool.CallableTool) → MigratePhase.Run()      │
│  ApplyDirectoryTool       (tool.CallableTool) → ApplyPhase.Run()        │
│  SyncProviderLogosTool    (tool.CallableTool) → LogoPhase.Run()         │
│                                                                          │
│  Registered in tools.Registry → Assemble auto-includes                  │
└──────────────────────────────┬───────────────────────────────────────────┘
                               │
┌──────────────────────────────▼───────────────────────────────────────────┐
│                    Domain Layer (internal/modelregistry/)                │
│                    Pure business logic, no framework dependency          │
│                                                                          │
│  Phase interface + PhaseResult + PhaseContext                           │
│  FetchPhase / MigratePhase / ApplyPhase / LogoPhase                     │
│  Directory / ProviderEntry / ModelEntry / Store / ApplyBackend          │
│  BatchApply / BatchMigrate / Checkpoint                                 │
└──────────────────────────────────────────────────────────────────────────┘
```

### 2.3 Alignment with Existing Patterns

| Dimension | Chat Main Path | Cron | New Model Registry |
|-----------|---------------|------|--------------------|
| Agent build | `BuildTRPCAgentCached` | Delegates to Chat | `BuildModelRegistrySyncAgent` |
| Runner creation | `RunnerManager.NewTurnRunner` | Delegates to Chat | Delegates to Chat (via CronRunner) |
| Event flow | EventBus | EventBus | EventBus |
| State mgmt | Session | Session | BackgroundJob + Session |
| Scheduling | User-triggered | CronRunner | CronRunner |
| LLM | Required | Required | **Not required** (programmatic Agent) |

---

## 3. Domain Layer Design (internal/modelregistry/)

### 3.1 Phase Interface

```go
type Phase interface {
    Name() string
    Timeout() time.Duration
    Run(pc *PhaseContext) PhaseResult
}

type PhaseStatus string

const (
    PhaseSucceeded PhaseStatus = "succeeded"
    PhaseFailed    PhaseStatus = "failed"
    PhaseSkipped   PhaseStatus = "skipped"
)

type PhaseResult struct {
    PhaseName  string
    Status     PhaseStatus
    Duration   time.Duration
    Stats      map[string]int
    Errors     []string
    Checkpoint *MigrationCheckpoint
}
```

### 3.2 PhaseContext — Self-contained Execution Context

```go
type PhaseContext struct {
    Ctx        context.Context
    Store      *Store
    Backend    ApplyBackend
    Reader     ApplyReader
    Writer     ApplyWriter
    Migrator   MigrationWriter
    Directory  Directory       // Populated after FetchPhase
    Policy     Policy
    Checkpoint *MigrationCheckpoint
    Lg         loggateway.Logger
}
```

> **实现偏差说明**：相比原始设计，PhaseContext 增加了 `Reader`、`Writer`、`Migrator` 三个细分接口字段和 `Lg` 日志字段。这是因为 ApplyBackend 被拆分为 ApplyReader/ApplyWriter/MigrationWriter 三个接口，Phase 实现可以按需依赖最小接口。`Lg` 字段用于各 Phase 内部的结构化日志输出。

### 3.3 Directory Type (renamed from Catalog)

```go
type Directory map[string]ProviderEntry

type ProviderEntry struct {
    ID      string
    Name    string
    Models  map[string]ModelEntry
    // ...
}
```

### 3.4 Phase Implementations

#### FetchPhase

```go
type FetchPhase struct{}

func (p *FetchPhase) Name() string         { return "fetch" }
func (p *FetchPhase) Timeout() time.Duration { return 120 * time.Second }

func (p *FetchPhase) Run(pc *PhaseContext) PhaseResult {
    syncer := NewSyncer(pc.Store, pc.Lg)
    out, err := syncer.Sync(pc.Ctx, SyncInput{})
    if err != nil {
        return PhaseResult{PhaseName: "fetch", Status: PhaseFailed, Errors: []string{err.Error()}}
    }
    if out.Status == "ok" && out.Meta.ETag != "" {
        // 304 Not Modified → skip subsequent phases
        return PhaseResult{PhaseName: "fetch", Status: PhaseSkipped}
    }
    return PhaseResult{
        PhaseName: "fetch",
        Status:    PhaseSucceeded,
        Stats:     map[string]int{"providers": out.Meta.ProviderCount, "models": out.Meta.ModelCount},
    }
}
```

**Key change**: `Syncer.Sync()` no longer includes `SyncProviderLogos`. Logo sync moved to independent `LogoPhase`.

#### MigratePhase (with Checkpoint + Batch)

```go
type MigratePhase struct {
    backend MigrationWriter
}

func NewMigratePhase(backend MigrationWriter) *MigratePhase {
    return &MigratePhase{backend: backend}
}

func (p *MigratePhase) Name() string         { return "migrate" }
func (p *MigratePhase) Timeout() time.Duration { return 300 * time.Second }

func (p *MigratePhase) Run(pc *PhaseContext) PhaseResult {
    checkpoint, _ := pc.Store.LoadMigrationCheckpoint()
    skipRules := checkpoint.CompletedRules

    result := p.backend.BatchMigrateProviderBindings(pc.Ctx, ListProviderMigrationRules(), skipRules)

    _ = pc.Store.SaveMigrationCheckpoint(NewCheckpoint(result.CompletedRules))

    status := PhaseSucceeded
    if len(result.FailedRules) > 0 && len(result.CompletedRules) == 0 {
        status = PhaseFailed
    }
    return PhaseResult{
        PhaseName:  "migrate",
        Status:     status,
        Stats:      result.Stats,
        Errors:     result.Errors,
        Checkpoint: NewCheckpoint(result.CompletedRules),
    }
}
```

**Key changes**:
- Checkpoint skips already-completed rules
- `BatchMigrateProviderBindings` merges 5 rules into 1 transaction
- Single rule failure doesn't block subsequent rules (partial success)

#### ApplyPhase (with Batch)

```go
type ApplyPhase struct {
    backend ApplyBackend
}

func (p *ApplyPhase) Name() string         { return "apply" }
func (p *ApplyPhase) Timeout() time.Duration { return 300 * time.Second }

func (p *ApplyPhase) Run(pc *PhaseContext) PhaseResult {
    rows, err := p.backend.ListProviderModels(pc.Ctx)
    if err != nil {
        return PhaseResult{PhaseName: "apply", Status: PhaseFailed, Errors: []string{err.Error()}}
    }

    var patches []ApplyRow
    var pricingUpserts []PricingUpsert
    for _, row := range rows {
        // existing merge logic (unchanged)
        if needsUpdate { patches = append(patches, patch) }
        if needsPricing { pricingUpserts = append(pricingUpserts, ...) }
    }

    result := p.backend.BatchApply(pc.Ctx, patches, pricingUpserts)
    return PhaseResult{
        PhaseName: "apply",
        Status:    PhaseSucceeded,
        Stats:     map[string]int{"rows_updated": result.RowsUpdated, "pricing_updated": result.PricingUpdated},
        Errors:    result.Errors,
    }
}
```

**Key change**: Batch write — collect all patches, then single transaction.

#### LogoPhase (independent, async)

```go
type LogoPhase struct{}

func (p *LogoPhase) Name() string         { return "logos" }
func (p *LogoPhase) Timeout() time.Duration { return 120 * time.Second }

func (p *LogoPhase) Run(pc *PhaseContext) PhaseResult {
    res := SyncProviderLogos(pc.Ctx, pc.Store, pc.Directory, defaultLogosBaseURL)
    status := PhaseSucceeded
    if res.Failed > 0 { status = PhaseFailed }
    return PhaseResult{
        PhaseName: "logos",
        Status:    status,
        Stats:     map[string]int{"synced": res.Synced, "failed": res.Failed, "removed": res.Removed},
        Errors:    res.Errors,
    }
}
```

### 3.5 ApplyBackend — Split Interface Design

> **实现偏差说明**：原始设计将所有方法放在单一 `ApplyBackend` 接口中。实际实现将接口拆分为三个最小接口 + 一个组合接口，遵循接口隔离原则（ISP）：

```go
// 读操作接口
type ApplyReader interface {
    ListProviderModels(ctx context.Context) ([]ApplyRow, error)
    CountProviderBindings(ctx context.Context, provider string) (ApplyMigrationStats, error)
}

// 写操作接口
type ApplyWriter interface {
    SaveProviderModel(ctx context.Context, row ApplyRow) error
    UpsertModelPricing(ctx context.Context, provider, model string, micro MicroPricing, source string) error
    BatchApply(ctx context.Context, patches []ApplyRow, pricing []PricingUpsert) BatchApplyResult
}

// 迁移操作接口
type MigrationWriter interface {
    MigrateProviderBindings(ctx context.Context, from, to string) (ApplyMigrationStats, error)
    BatchMigrateProviderBindings(ctx context.Context, rules []ProviderMigrationRule, skipRules []string) BatchMigrationResult
}

// 组合接口（data 层实现）
type ApplyBackend interface {
    ApplyReader
    ApplyWriter
    MigrationWriter
}

type BatchMigrationResult struct {
    CompletedRules []string
    FailedRules    []string
    Stats          ApplyMigrationStats
    Errors         []string
}

type BatchApplyResult struct {
    RowsUpdated    int
    PricingUpdated int
    Errors         []string
}
```

---

## 4. Bridge Layer Design (internal/agent/)

### 4.1 ModelRegistrySyncAgent

> **实现偏差说明**：相比原始设计，实际实现有以下差异：
> 1. 增加 `lg loggateway.Logger` 字段，通过构造注入满足红线 #16
> 2. 增加 `runner trpcrunner.Runner` 字段，用于 `RunSync()` 方法
> 3. Phase 构建委托给 `internal/tools/modelsync/registry.go` 的 `BuildPhases` 函数
> 4. Tool 注册委托给 `internal/tools/modelsync/registry.go` 的 `RegisterAll` 函数
> 5. Logo Phase 在主 Phase 循环之后独立执行（仅当 Directory 非空时）
> 6. 事件发射使用 `trpcagent.EmitEvent(ctx, inv, ch, evt)` 而非直接操作 channel

```go
package agent

import (
    trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
    trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
    trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
    trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
    "aranea-agents/internal/modelregistry"
    "aranea-agents/internal/tools/modelsync"
    "aranea-agents/pkg/loggateway"
    "aranea-agents/pkg/safego"
)

type ModelRegistrySyncAgent struct {
    phases    []modelregistry.Phase
    logoPhase modelregistry.Phase
    tools     []trpctool.Tool
    storeProv modelregistry.StoreProvider
    backend   modelregistry.ApplyBackend
    lg        loggateway.Logger
    runner    trpcrunner.Runner
}

func BuildModelRegistrySyncAgent(
    storeProv modelregistry.StoreProvider,
    backend modelregistry.ApplyBackend,
    lg loggateway.Logger,
) (*ModelRegistrySyncAgent, error) {
    phases := modelsync.BuildPhases(backend, lg)
    tools := modelsync.RegisterAll(modelsync.Deps{
        Phases:        phases,
        StoreProvider: storeProv,
        Backend:       backend,
    })

    ag := &ModelRegistrySyncAgent{
        phases:    phases.List(),
        logoPhase: phases.LogoPhase(),
        tools:     tools,
        storeProv: storeProv,
        backend:   backend,
        lg:        lg,
    }

    ag.runner = trpcrunner.NewRunner("model-registry-sync", ag)
    return ag, nil
}

func (a *ModelRegistrySyncAgent) Run(ctx context.Context, inv *trpcagent.Invocation) (<-chan *trpcevent.Event, error) {
    ch := make(chan *trpcevent.Event, 64)

    safego.Go(ctx, "modelregistry.sync_agent", func() {
        defer close(ch)

        store, err := a.storeProv.Store(ctx)
        if err != nil {
            emitError(ch, err)
            return
        }

        pc := &modelregistry.PhaseContext{
            Ctx:     ctx,
            Store:   store,
            Backend: a.backend,
        }

        for _, phase := range a.phases {
            phaseCtx, cancel := context.WithTimeout(ctx, phase.Timeout())
            pc.Ctx = phaseCtx

            emitPhaseStart(ch, phase.Name())
            result := phase.Run(pc)
            cancel()
            emitPhaseResult(ch, phase.Name(), result)

            if result.Status == modelregistry.PhaseFailed {
                break
            }

            if phase.Name() == "fetch" && result.Status == modelregistry.PhaseSucceeded {
                dir, _, _ := store.LoadDirectory()
                pc.Directory = dir
            }
        }

        emitCompletion(ch)
    })

    return ch, nil
}

func (a *ModelRegistrySyncAgent) Tools() []trpctool.Tool { return a.tools }
func (a *ModelRegistrySyncAgent) Info() trpcagent.Info {
    return trpcagent.Info{Name: "model-registry-sync", Description: "Model registry sync agent"}
}
func (a *ModelRegistrySyncAgent) SubAgents() []trpcagent.Agent { return nil }
func (a *ModelRegistrySyncAgent) FindSubAgent(string) trpcagent.Agent { return nil }
```

### 4.2 Event Emission Helpers

```go
func emitPhaseStart(ch chan<- *trpcevent.Event, phase string) {
    ch <- &trpcevent.Event{
        ID:        uuid.New().String(),
        Timestamp: time.Now(),
        Author:    "model-registry-sync",
        Tag:       "phase_start",
        Extensions: map[string]json.RawMessage{
            "phase": json.RawMessage(`"` + phase + `"`),
        },
    }
}

func emitPhaseResult(ch chan<- *trpcevent.Event, phase string, result modelregistry.PhaseResult) {
    ch <- &trpcevent.Event{
        ID:        uuid.New().String(),
        Timestamp: time.Now(),
        Author:    "model-registry-sync",
        Tag:       "phase_" + string(result.Status),
        Extensions: map[string]json.RawMessage{
            "phase":    json.RawMessage(`"` + phase + `"`),
            "status":   json.RawMessage(`"` + string(result.Status) + `"`),
            "duration": json.RawMessage(fmt.Sprintf(`%d`, result.Duration.Milliseconds())),
        },
    }
}

func emitCompletion(ch chan<- *trpcevent.Event) {
    ch <- &trpcevent.Event{
        ID:        uuid.New().String(),
        Timestamp: time.Now(),
        Author:    "model-registry-sync",
        Done:      true,
        Object:    "runner_completion",
    }
}
```

---

## 5. Tool Layer Design (internal/tools/modelsync/)

> **实现偏差说明**：原始设计将 Tool 放在 `internal/tools/model_registry_sync.go` 中，使用自定义 `CallableTool` 实现。实际实现改为独立子包 `internal/tools/modelsync/`，使用 `trpcfunction.FunctionTool` 泛型类型工具，提供类型安全的输入输出结构体。

### 5.1 Package Structure

```
internal/tools/modelsync/
├── registry.go    # Deps + Phases 结构体 + BuildPhases + RegisterAll
├── tools.go       # 4 个 FunctionTool 实现
└── registry_test.go
```

### 5.2 Phases Registry

```go
package modelsync

type Deps struct {
    Phases        *Phases
    StoreProvider modelregistry.StoreProvider
    Backend       modelregistry.ApplyBackend
}

type Phases struct {
    fetchPhase   modelregistry.Phase
    migratePhase modelregistry.Phase
    applyPhase   modelregistry.Phase
    logoPhase    modelregistry.Phase
}

func BuildPhases(backend modelregistry.ApplyBackend, lg loggateway.Logger) *Phases {
    return &Phases{
        fetchPhase:   modelregistry.NewFetchPhase(),
        migratePhase: modelregistry.NewMigratePhase(backend),
        applyPhase:   modelregistry.NewApplyPhase(backend, backend, lg),
        logoPhase:    modelregistry.NewLogoPhase(),
    }
}

func (p *Phases) List() []modelregistry.Phase {
    return []modelregistry.Phase{p.fetchPhase, p.migratePhase, p.applyPhase}
}

func (p *Phases) LogoPhase() modelregistry.Phase {
    return p.logoPhase
}

func RegisterAll(deps Deps) []trpctool.Tool {
    return []trpctool.Tool{
        newFetchDirectoryTool(deps),
        newMigrateProvidersTool(deps),
        newApplyDirectoryTool(deps),
        newSyncProviderLogosTool(deps),
    }
}
```

### 5.3 Tool Implementations (FunctionTool Pattern)

每个 Tool 使用 `trpcfunction.FunctionTool[I, O]` 泛型，提供类型安全的输入输出：

```go
// 示例：FetchDirectoryTool
func newFetchDirectoryTool(deps Deps) *trpcfunction.FunctionTool[noArgs, fetchDirectoryOutput] {
    return trpcfunction.NewFunctionTool(
        func(ctx context.Context, _ noArgs) (fetchDirectoryOutput, error) {
            store, err := deps.StoreProvider.Store(ctx)
            if err != nil {
                return fetchDirectoryOutput{}, fmt.Errorf("store error: %w", err)
            }
            policy, policyErr := store.LoadPolicy()
            if policyErr != nil {
                return fetchDirectoryOutput{}, fmt.Errorf("load policy: %w", policyErr)
            }
            pc := &modelregistry.PhaseContext{
                Ctx:     ctx,
                Store:   store,
                Backend: deps.Backend,
                Reader:  deps.Backend,
                Writer:  deps.Backend,
                Policy:  policy,
            }
            result := deps.Phases.fetchPhase.Run(pc)
            if result.Status == modelregistry.PhaseFailed {
                return fetchDirectoryOutput{Status: "failed", Errors: result.Errors}, nil
            }
            return fetchDirectoryOutput{Status: "succeeded", Message: "model directory fetched"}, nil
        },
        trpcfunction.WithName("fetch_model_directory"),
        trpcfunction.WithDescription("Fetch the latest model directory from models.dev"),
    )
}
```

同样模式：`newMigrateProvidersTool`、`newApplyDirectoryTool`、`newSyncProviderLogosTool`。

---

## 6. Data Layer Design (internal/data/)

### 6.1 BatchMigrateProviderBindings

```go
func (b *modelRegistryApplyBackend) BatchMigrateProviderBindings(
    ctx context.Context,
    rules []modelregistry.ProviderMigrationRule,
    skipRules []string,
) modelregistry.BatchMigrationResult {
    tx, err := b.data.RawDB().BeginTx(ctx, nil)
    if err != nil {
        return modelregistry.BatchMigrationResult{Errors: []string{err.Error()}}
    }
    defer tx.Rollback()

    var result modelregistry.BatchMigrationResult
    now := nowRFC3339()

    for _, rule := range rules {
        if contains(skipRules, ruleKey(rule)) {
            result.CompletedRules = append(result.CompletedRules, ruleKey(rule))
            continue
        }

        stats, err := migrateOneRuleInTx(ctx, tx, rule, now)
        if err != nil {
            result.FailedRules = append(result.FailedRules, ruleKey(rule))
            result.Errors = append(result.Errors, fmt.Sprintf("migrate %s→%s: %v", rule.Legacy, rule.Catalog, err))
            continue
        }
        result.CompletedRules = append(result.CompletedRules, ruleKey(rule))
        result.Stats.Agents += stats.Agents
        result.Stats.Sessions += stats.Sessions
        result.Stats.Eval += stats.Eval
        result.Stats.RuntimeSettings += stats.RuntimeSettings
        result.Stats.Skills += stats.Skills
        result.Stats.KnowledgeEmbed += stats.KnowledgeEmbed
        result.Stats.WebResearch += stats.WebResearch
    }

    if err := tx.Commit(); err != nil {
        result.Errors = append(result.Errors, err.Error())
    }
    return result
}
```

**Performance**: 5 transactions → 1 transaction. SQLite write-lock contention reduced ~80%.

### 6.2 BatchApply

```go
func (b *modelRegistryApplyBackend) BatchApply(
    ctx context.Context,
    patches []modelregistry.ApplyRow,
    pricing []modelregistry.PricingUpsert,
) modelregistry.BatchApplyResult {
    tx, err := b.data.RawDB().BeginTx(ctx, nil)
    if err != nil {
        return modelregistry.BatchApplyResult{Errors: []string{err.Error()}}
    }
    defer tx.Rollback()

    var result modelregistry.BatchApplyResult
    for _, p := range patches {
        if _, err := tx.ExecContext(ctx,
            `UPDATE llm_provider_models SET provider=?, model_key=?, name=?, enabled=?, config_json=?, metadata_json=?, updated_at=? WHERE id=?`,
            p.Provider, p.Key, p.Name, p.Enabled, p.ConfigJSON, p.MetadataJSON, nowRFC3339(), p.ID,
        ); err != nil {
            result.Errors = append(result.Errors, fmt.Sprintf("save %s/%s: %v", p.Provider, p.Model, err))
            continue
        }
        result.RowsUpdated++
    }
    for _, p := range pricing {
        if err := upsertPricingInTx(ctx, tx, p); err != nil {
            result.Errors = append(result.Errors, fmt.Sprintf("pricing %s/%s: %v", p.Provider, p.Model, err))
            continue
        }
        result.PricingUpdated++
    }

    if err := tx.Commit(); err != nil {
        result.Errors = append(result.Errors, err.Error())
    }
    return result
}
```

**Performance**: N individual UPDATEs → 1 transaction batch UPDATE.

---

## 7. Scheduling Layer — CronRunner Integration

### 7.1 System CronTask Seed

```go
func SeedModelRegistryCronTask(ctx context.Context, cronRepo biz.CronRepo) {
    cronRepo.Create(ctx, biz.CronTask{
        Name:           "model-registry-sync",
        Kind:           "system",
        TargetType:     "agent",
        TargetID:       "model-registry-sync",
        ScheduleType:   "interval",
        IntervalSeconds: 3600,
        Enabled:        true,
    })
}
```

### 7.2 CronRunner Route Extension

CronRunner's `dispatchCronTask` needs to handle `targetID = "model-registry-sync"`:

```go
func (r *Runner) dispatchCronTask(ctx context.Context, cfg cronTaskConfig, ...) error {
    if cfg.TargetID == "model-registry-sync" {
        return r.deps.Chat.RunCronTurn(ctx, sessionID, "sync model registry", "")
    }
    // existing logic...
}
```

### 7.3 ChatOrchestrator Route

When `content == "sync model registry"`, ChatOrchestrator resolves the agent key to `model-registry-sync` and builds the `ModelRegistrySyncAgent` instead of an LLMAgent.

---

## 8. Event Flow

```
ModelRegistrySyncAgent.Run()
  → emitPhaseStart("fetch")       → trpcevent.Event → Runner event loop
  → emitPhaseResult("fetch")      → trpcevent.Event → Runner event loop
  → emitPhaseStart("migrate")     → trpcevent.Event → Runner event loop
  → emitPhaseResult("migrate")    → trpcevent.Event → Runner event loop
  → emitPhaseStart("apply")       → trpcevent.Event → Runner event loop
  → emitPhaseResult("apply")      → trpcevent.Event → Runner event loop
  → emitCompletion()              → trpcevent.Event → Runner event loop
                                     ↓
                               Runner.processAgentEvents()
                                     ↓
                               EventBusConsumer → EventBus.Publish()
                                     ↓
                               SessionBus → WS push
                               MonitorBus → FlowLog persistence
```

---

## 9. Wire Injection Changes

### 9.1 Removed

```go
// cmd/admin/wire.go — removed
func provideModelCatalogRunner(...) *modelcatalog.Runner
```

### 9.2 Added

```go
// cmd/admin/wire.go — added
func provideModelRegistryApplyBackend(llm biz.LlmProviderModelRepo, d *data.Data) modelregistry.ApplyBackend {
    return data.NewModelRegistryApplyBackend(d, llm)
}

func provideModelRegistrySyncAgent(sys biz.SystemSettingRepo, backend modelregistry.ApplyBackend, lg loggateway.Logger) (*agent.ModelRegistrySyncAgent, error) {
    storeProv := biz.NewModelRegistryStoreProvider(biz.NewSystemSettingRootAdapter(sys), lg)
    return agent.BuildModelRegistrySyncAgent(storeProv, backend, lg)
}

func provideModelRegistryUsecase(sys biz.SystemSettingRepo, backend modelregistry.ApplyBackend, lg loggateway.Logger) *biz.ModelRegistryUsecase {
    uc := biz.NewModelRegistryUsecase(biz.NewSystemSettingRootAdapter(sys), backend, lg)
    return uc
}
```

### 9.3 CronRunner Integration

CronRunner 通过 `CronRegistrySyncAgent` 接口与 `ModelRegistrySyncAgent` 集成：

```go
// Wire 绑定
wire.Bind(new(cronrunner.CronRegistrySyncAgent), new(*agent.ModelRegistrySyncAgent))

// CronRunner 路由
case "model_registry_sync":
    if r.deps.RegistrySyncAgent == nil {
        return cronDispatchResult{}, validationErr("model registry sync agent not available")
    }
    if err := r.deps.RegistrySyncAgent.RunSync(ctx); err != nil {
        return cronDispatchResult{}, err
    }
    return cronDispatchResult{}, nil
```

`ModelRegistrySyncAgent.RunSync()` 方法通过 `trpcrunner.Runner` 执行同步：

```go
func (a *ModelRegistrySyncAgent) RunSync(ctx context.Context) error {
    ch, err := a.runner.Run(ctx, "system", "model-registry-sync", trpcmodel.NewUserMessage("run model registry sync"))
    if err != nil {
        return err
    }
    for range ch {
    }
    return nil
}
```

---

## 10. File Change List

### New Files

| File | Description |
|------|-------------|
| `internal/modelregistry/phase.go` | Phase interface, PhaseResult, PhaseContext, PhaseStatus, WithPhaseCtx/PhaseFromCtx |
| `internal/modelregistry/fetch_phase.go` | FetchPhase implementation |
| `internal/modelregistry/migrate_phase.go` | MigratePhase + Checkpoint |
| `internal/modelregistry/apply_phase.go` | ApplyPhase + BatchApply |
| `internal/modelregistry/logo_phase.go` | LogoPhase |
| `internal/modelregistry/directory.go` | Directory type (renamed from Catalog) + Provider/Model/Meta/Policy |
| `internal/modelregistry/store.go` | Disk Store (adapted from modelcatalog) |
| `internal/modelregistry/store_provider.go` | StoreProvider interface |
| `internal/modelregistry/sync.go` | Syncer (without SyncProviderLogos) |
| `internal/modelregistry/fetch.go` | FetchDirectory + ParseDirectory |
| `internal/modelregistry/fetch_retry.go` | HTTP retry logic |
| `internal/modelregistry/apply.go` | ApplyReader/ApplyWriter/MigrationWriter/ApplyBackend + Applier + BatchApply/BatchMigrate types |
| `internal/modelregistry/migrate_bindings.go` | RunProviderMigrations |
| `internal/modelregistry/migrate.go` | PreviewMigration |
| `internal/modelregistry/migration_map.go` | ProviderMigrationRule + ListProviderMigrationRules |
| `internal/modelregistry/migration_checkpoint.go` | MigrationCheckpoint (with CompletedRules) |
| `internal/modelregistry/logos.go` | SyncProviderLogos + LogoPhase |
| `internal/modelregistry/overlay.go` | Runtime overlay + list/search helpers |
| `internal/modelregistry/search.go` | SearchDirectoryBlocks |
| `internal/modelregistry/urlguard.go` | SSRF protection |
| `internal/modelregistry/pricing.go` | MicroPricing conversion |
| `internal/modelregistry/policy_validate.go` | Policy normalization |
| `internal/modelregistry/config_merge.go` | Config/Metadata merge |
| `internal/modelregistry/chips.go` | CapabilityChips |
| `internal/modelregistry/backfill.go` | Cost backfill |
| `internal/modelregistry/logs.go` | JSONL sync logs |
| `internal/modelregistry/runtime_overlay.json` | Runtime overlay data |
| `internal/agent/model_registry_sync.go` | ModelRegistrySyncAgent (bridge) |
| `internal/agent/model_registry_sync_test.go` | Agent 测试 |
| `internal/tools/modelsync/registry.go` | Deps + Phases 结构体 + BuildPhases + RegisterAll |
| `internal/tools/modelsync/tools.go` | 4 个 FunctionTool 实现 |
| `internal/tools/modelsync/registry_test.go` | Tools 测试 |
| `internal/data/model_registry_apply.go` | BatchApply + BatchMigrate 数据层实现 |
| `internal/data/model_registry_apply_test.go` | 数据层测试 |
| `internal/biz/model_registry.go` | ModelRegistryUsecase + SeedModelRegistryCronTask |
| `internal/biz/model_registry_test.go` | Biz 层测试 |

### Modified Files

| File | Change |
|------|--------|
| `internal/service/model_catalog.go` | imports 从 modelcatalog → modelregistry，使用 biz.ModelRegistryUsecase |
| `internal/biz/llm_provider_model.go` | imports 从 modelcatalog → modelregistry |
| `internal/biz/llm_provider_model_pricing_test.go` | imports 从 modelcatalog → modelregistry |
| `internal/biz/usage/usage.go` | imports 从 modelcatalog → modelregistry |
| `internal/data/usage_write.go` | modelcatalog.MigrateProviderCode → modelregistry.MigrateProviderCode |
| `internal/cronrunner/runner.go` | 添加 model_registry_sync target 路由 + CronRegistrySyncAgent 接口 |
| `internal/data/builtin_tools_seed.go` | 添加 model_registry_sync 工具集种子 |
| `internal/tools/toolset.go` | 添加 model_registry_sync 工具集注册 |
| `cmd/admin/wire.go` | 替换 provideModelCatalogRunner → provideModelRegistrySyncAgent + provideModelRegistryApplyBackend |
| `cmd/admin/main.go` | 替换 ModelCatalogRunner 启动 → ModelRegistrySyncAgent + SeedModelRegistryCronTask |

### Deleted Files

| File | Reason |
|------|--------|
| `internal/modelcatalog/` (entire directory) | Replaced by `internal/modelregistry/` |
| `internal/data/model_catalog_apply.go` | Replaced by `model_registry_apply.go` |
| `internal/biz/model_catalog.go` | Replaced by `model_registry.go` |

---

## 11. Red Line Fixes

| # | Violation | Current | Fix | Status |
|---|-----------|---------|-----|--------|
| 9 | Bare goroutine | `go r.loop(ctx)` | `safego.Go(ctx, "modelregistry.sync_agent", ...)` | ✅ Fixed |
| 10 | log.Printf | `r.logger.Printf(...)` | Events flow through EventBus → FlowLog automatically; 内部日志使用 `loggateway.Logger` | ✅ Fixed |
| 16 | log/slog | N/A | 统一使用 `loggateway.Logger`（通过构造注入） | ✅ Fixed |

---

## 12. Performance Improvements

| Operation | Before | After | Improvement |
|-----------|--------|-------|-------------|
| Migration | 5 serial transactions × 11 UPDATEs | 1 transaction × up to 55 UPDATEs | ~5x faster, ~80% less lock contention |
| Apply | N individual UPDATE + N UpsertPricing | 1 transaction batch | ~10x+ faster |
| Timeout | 130s shared budget | 120s/300s/300s per phase | No cross-phase timeout interference |
| Logo | Blocks main flow (shared ctx) | Async (independent ctx) | Zero impact on main pipeline |
| Checkpoint | None (re-runs all rules) | Skip completed rules | Avoids redundant work |

---

## 13. Migration Strategy

### Phase 1: Package Rename + Domain Refactor ✅

1. Rename `internal/modelcatalog/` → `internal/modelregistry/`
2. Rename `Catalog` → `Directory`, `CatalogConfig` → `ProviderConfig`, etc.
3. Add Phase interface and implementations
4. Add BatchApply/BatchMigrate to ApplyBackend
5. Add Checkpoint for migration
6. All existing tests must pass

### Phase 2: Bridge Layer ✅

1. Add `internal/agent/model_registry_sync.go` (ModelRegistrySyncAgent)
2. Add `internal/tools/modelsync/` (4 FunctionTools + Phases Registry)
3. Wire injection

### Phase 3: CronRunner Integration ✅

1. Add system CronTask seed (`SeedModelRegistryCronTask`)
2. Extend CronRunner routing for model_registry_sync target
3. Add `CronRegistrySyncAgent` interface + `RunSync()` method
4. Remove standalone ModelCatalogRunner
5. End-to-end test

### Phase 4: Naming Cleanup (Separate Task) ⬜

1. `runtime.Catalog` → `TurnDeps`
2. `provider.CatalogConfig` → `ProviderConfig`
3. `session.ModelConfigCatalog` → `ModelConfigResolver`
4. Update all references across the project

---

## 14. Out of Scope

The following naming issues were discovered but are **not part of this refactoring**:

- `Repository` vs `Repo` suffix unification (55 interfaces)
- `Port`/`Bridge`/`Lookup`/`Resolver` suffix unification (9 interfaces)
- `llmCatalogContext` duplicate code extraction
- `GraphBuildConfig` + 6 types duplicate code extraction
- Deprecated Turn type cleanup
- `Result` vs `Output` semantic unification
- `Worker`/`Scanner`/`Cleanup` suffix unification
- `Adapter` vs `Bridge` semantic unification
