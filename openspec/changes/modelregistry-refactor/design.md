# Model Registry Refactor Design

> Date: 2026-05-30
> Status: Draft
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
    Directory  Directory       // Populated after FetchPhase
    Policy     Policy
    Checkpoint *MigrationCheckpoint
}
```

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
    syncer := NewSyncer(pc.Store)
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
    backend ApplyBackend
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

### 3.5 ApplyBackend — New Batch Interfaces

```go
type ApplyBackend interface {
    // Existing (kept for manual API compatibility)
    ListProviderModels(ctx context.Context) ([]ApplyRow, error)
    SaveProviderModel(ctx context.Context, row ApplyRow) error
    UpsertModelPricing(ctx context.Context, provider, model string, micro MicroPricing, source string) error
    CountProviderBindings(ctx context.Context, provider string) (ApplyMigrationStats, error)
    MigrateProviderBindings(ctx context.Context, from, to string) (ApplyMigrationStats, error)

    // New batch interfaces
    BatchMigrateProviderBindings(ctx context.Context, rules []ProviderMigrationRule, skipRules []string) BatchMigrationResult
    BatchApply(ctx context.Context, patches []ApplyRow, pricing []PricingUpsert) BatchApplyResult
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

```go
package agent

import (
    trpcagent "aranea-agents/pkg/trpc-agent-go/agent"
    trpcevent "aranea-agents/pkg/trpc-agent-go/event"
    trpctool "aranea-agents/pkg/trpc-agent-go/tool"
    "aranea-agents/internal/modelregistry"
    "aranea-agents/pkg/safego"
)

type ModelRegistrySyncAgent struct {
    phases    []modelregistry.Phase
    logoPhase modelregistry.Phase
    tools     []trpctool.Tool
    storeProv modelregistry.StoreProvider
    backend   modelregistry.ApplyBackend
}

func BuildModelRegistrySyncAgent(
    storeProv modelregistry.StoreProvider,
    backend modelregistry.ApplyBackend,
) (*ModelRegistrySyncAgent, error) {
    fetchPhase := modelregistry.NewFetchPhase()
    migratePhase := modelregistry.NewMigratePhase(backend)
    applyPhase := modelregistry.NewApplyPhase(backend)
    logoPhase := modelregistry.NewLogoPhase()

    tools := []trpctool.Tool{
        &fetchDirectoryTool{phase: fetchPhase},
        &migrateProvidersTool{phase: migratePhase},
        &applyDirectoryTool{phase: applyPhase},
        &syncProviderLogosTool{phase: logoPhase},
    }

    return &ModelRegistrySyncAgent{
        phases:    []modelregistry.Phase{fetchPhase, migratePhase, applyPhase},
        logoPhase: logoPhase,
        tools:     tools,
        storeProv: storeProv,
        backend:   backend,
    }, nil
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

## 5. Tool Layer Design (internal/tools/)

### 5.1 Tool Implementations

```go
package tools

type fetchDirectoryTool struct {
    phase *modelregistry.FetchPhase
}

func (t *fetchDirectoryTool) Declaration() *trpctool.Declaration {
    return &trpctool.Declaration{
        Name:        "fetch_model_directory",
        Description: "Fetch the latest model directory from models.dev",
        Parameters:  &trpctool.DeclarationParameters{Type: "object"},
    }
}

func (t *fetchDirectoryTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
    pc := modelregistry.PhaseContextFromCtx(ctx)
    result := t.phase.Run(pc)
    if result.Status == modelregistry.PhaseFailed {
        return nil, fmt.Errorf("fetch failed: %v", result.Errors)
    }
    return result, nil
}
```

Similarly: `migrateProvidersTool`, `applyDirectoryTool`, `syncProviderLogosTool`.

### 5.2 Registry Integration

```go
func RegisterModelRegistryTools(r *Registry, backend modelregistry.ApplyBackend) {
    r.Register(ToolRegistration{
        Key:  "fetch_model_directory",
        Tool: &fetchDirectoryTool{phase: modelregistry.NewFetchPhase()},
    })
    r.Register(ToolRegistration{
        Key:  "migrate_provider_bindings",
        Tool: &migrateProvidersTool{phase: modelregistry.NewMigratePhase(backend)},
    })
    r.Register(ToolRegistration{
        Key:  "apply_model_directory",
        Tool: &applyDirectoryTool{phase: modelregistry.NewApplyPhase(backend)},
    })
    r.Register(ToolRegistration{
        Key:  "sync_provider_logos",
        Tool: &syncProviderLogosTool{phase: modelregistry.NewLogoPhase()},
    })
}
```

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
func provideModelRegistrySyncAgent(
    sys biz.SystemSettingRepo,
    llm biz.LlmProviderModelRepo,
    d *data.Data,
) *agent.ModelRegistrySyncAgent {
    storeProv := biz.NewModelRegistryStoreProvider(biz.NewSystemSettingRootAdapter(sys))
    backend := data.NewModelRegistryApplyBackend(d, llm)
    ag, _ := agent.BuildModelRegistrySyncAgent(storeProv, backend)
    return ag
}
```

### 9.3 Agent Registry

The `ModelRegistrySyncAgent` is registered in the Runner's agent lookup table:

```go
// In ChatOrchestrator or RunnerManager
lookupAgents := map[string]trpcagent.Agent{
    "model-registry-sync": catalogSyncAgent,
}
```

---

## 10. File Change List

### New Files

| File | Description |
|------|-------------|
| `internal/modelregistry/phase.go` | Phase interface, PhaseResult, PhaseContext, PhaseStatus |
| `internal/modelregistry/fetch_phase.go` | FetchPhase implementation |
| `internal/modelregistry/migrate_phase.go` | MigratePhase + Checkpoint |
| `internal/modelregistry/apply_phase.go` | ApplyPhase + BatchApply |
| `internal/modelregistry/logo_phase.go` | LogoPhase |
| `internal/agent/model_registry_sync.go` | ModelRegistrySyncAgent (bridge) |
| `internal/tools/model_registry_sync.go` | 4 CallableTools (bridge) |

### Modified Files

| File | Change |
|------|--------|
| `internal/modelcatalog/` → `internal/modelregistry/` | Package rename + all type renames |
| `internal/modelregistry/sync.go` | Sync() removes SyncProviderLogos call |
| `internal/modelregistry/apply.go` | ApplyWithMigration split, BatchApply interface |
| `internal/modelregistry/migrate_bindings.go` | Adapt to Phase interface |
| `internal/modelregistry/logos.go` | LogoPhase wrapper |
| `internal/modelregistry/catalog.go` → `directory.go` | Catalog → Directory rename |
| `internal/data/model_catalog_apply.go` → `model_registry_apply.go` | BatchApply + BatchMigrate implementations |
| `internal/biz/model_catalog.go` → `model_registry.go` | Adapt to new architecture |
| `internal/provider/catalog.go` | CatalogConfig → ProviderConfig rename |
| `internal/runtime/deps.go` | Catalog → TurnDeps rename |
| `internal/session/context_update.go` | ModelConfigCatalog → ModelConfigResolver rename |
| `internal/cronrunner/runner.go` | Support model-registry-sync target routing |
| `cmd/admin/wire.go` | Wire injection changes |
| `cmd/admin/main.go` | Remove ModelCatalogRunner startup, add CronTask seed |

### Deleted Files

| File | Reason |
|------|--------|
| `internal/modelregistry/runner.go` | Replaced by CronRunner + ModelRegistrySyncAgent |

---

## 11. Red Line Fixes

| # | Violation | Current | Fix |
|---|-----------|---------|-----|
| 9 | Bare goroutine | `go r.loop(ctx)` | `safego.Go(ctx, "modelregistry.sync_agent", ...)` |
| 10 | log.Printf | `r.logger.Printf(...)` | Events flow through EventBus → FlowLog automatically |

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

### Phase 1: Package Rename + Domain Refactor

1. Rename `internal/modelcatalog/` → `internal/modelregistry/`
2. Rename `Catalog` → `Directory`, `CatalogConfig` → `ProviderConfig`, etc.
3. Add Phase interface and implementations
4. Add BatchApply/BatchMigrate to ApplyBackend
5. Add Checkpoint for migration
6. All existing tests must pass

### Phase 2: Bridge Layer

1. Add `internal/agent/model_registry_sync.go` (ModelRegistrySyncAgent)
2. Add `internal/tools/model_registry_sync.go` (4 CallableTools)
3. Wire injection

### Phase 3: CronRunner Integration

1. Add system CronTask seed
2. Extend CronRunner routing for model-registry-sync target
3. Remove standalone ModelCatalogRunner
4. End-to-end test

### Phase 4: Naming Cleanup (Separate Task)

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
