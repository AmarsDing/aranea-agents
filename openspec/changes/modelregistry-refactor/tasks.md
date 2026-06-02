# Model Registry Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor `internal/modelcatalog/` → `internal/modelregistry/` based on trpc-agent-go Agent + Tool system, eliminating the standalone Runner and fixing red line violations.

**Architecture:** Four-layer design — Domain Layer (Phase interface + pure logic) → Tool Layer (CallableTool adapters) → Bridge Layer (ModelRegistrySyncAgent implementing agent.Agent) → Scheduling Layer (CronRunner). Each sync phase gets independent timeout, batch DB operations replace row-by-row I/O, and logo sync is decoupled from the main pipeline.

**Tech Stack:** Go, trpc-agent-go (agent/tool/event), CronRunner, Ent ORM, Wire DI, safego, FlowLog

**Design Spec:** `design.md`

---

## File Structure

### New Files

| File | Responsibility |
|------|---------------|
| `internal/modelregistry/phase.go` | Phase interface, PhaseResult, PhaseContext, PhaseStatus |
| `internal/modelregistry/fetch_phase.go` | FetchPhase implementation |
| `internal/modelregistry/migrate_phase.go` | MigratePhase + Checkpoint |
| `internal/modelregistry/apply_phase.go` | ApplyPhase + BatchApply |
| `internal/modelregistry/logo_phase.go` | LogoPhase |
| `internal/modelregistry/directory.go` | Directory type (renamed from Catalog) + Provider/Model/Meta/Policy |
| `internal/modelregistry/store.go` | Disk Store (adapted from modelcatalog) |
| `internal/modelregistry/store_provider.go` | StoreProvider interface |
| `internal/modelregistry/sync.go` | Syncer (without SyncProviderLogos) |
| `internal/modelregistry/fetch.go` | FetchCatalog + ParseCatalog |
| `internal/modelregistry/fetch_retry.go` | HTTP retry logic |
| `internal/modelregistry/apply.go` | ApplyBackend interface + Applier + BatchApply/BatchMigrate types |
| `internal/modelregistry/migrate_bindings.go` | RunProviderMigrations |
| `internal/modelregistry/migrate.go` | PreviewMigration |
| `internal/modelregistry/migration_map.go` | ProviderMigrationRule + ListProviderMigrationRules |
| `internal/modelregistry/migration_checkpoint.go` | MigrationCheckpoint |
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
| `internal/modelregistry/runtime_overlay.json` | Runtime overlay data (copy) |
| `internal/agent/model_registry_sync.go` | ModelRegistrySyncAgent (bridge) |
| `internal/tools/model_registry_sync.go` | 4 CallableTools (bridge) |

### Modified Files

| File | Change |
|------|--------|
| `internal/biz/model_catalog.go` → `model_registry.go` | Rename types, remove Runner dependency, add Phase-aware methods |
| `internal/data/model_catalog_apply.go` → `model_registry_apply.go` | Add BatchApply + BatchMigrate implementations |
| `internal/service/model_catalog.go` | Update imports from modelcatalog → modelregistry |
| `internal/cronrunner/runner.go` | Add model-registry-sync target routing |
| `cmd/admin/wire.go` | Replace provideModelCatalogRunner with provideModelRegistrySyncAgent |
| `cmd/admin/main.go` | Remove ModelCatalogRunner startup, add CronTask seed |

### Deleted Files

| File | Reason |
|------|--------|
| `internal/modelcatalog/runner.go` | Replaced by CronRunner + ModelRegistrySyncAgent |
| `internal/modelcatalog/` (entire directory) | Replaced by `internal/modelregistry/` |

---

## Task 1: Create Domain Layer — Core Types

**Files:**
- Create: `internal/modelregistry/directory.go`
- Create: `internal/modelregistry/phase.go`
- Create: `internal/modelregistry/store_provider.go`

- [ ] **Step 1: Create `internal/modelregistry/directory.go`**

Port from `internal/modelcatalog/catalog.go`, rename `Catalog` → `Directory`:

```go
package modelregistry

import "encoding/json"

type Directory map[string]Provider

type Provider struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	Env    []string        `json:"env"`
	Npm    string          `json:"npm"`
	API    string          `json:"api,omitempty"`
	Doc    string          `json:"doc"`
	Models map[string]Model `json:"models"`
}

type Model struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Family           string          `json:"family,omitempty"`
	Attachment       bool            `json:"attachment"`
	Reasoning        bool            `json:"reasoning"`
	ToolCall         bool            `json:"tool_call"`
	StructuredOutput *bool           `json:"structured_output,omitempty"`
	Temperature      *bool           `json:"temperature,omitempty"`
	Knowledge        string          `json:"knowledge,omitempty"`
	ReleaseDate      string          `json:"release_date"`
	LastUpdated      string          `json:"last_updated"`
	OpenWeights      bool            `json:"open_weights"`
	Interleaved      json.RawMessage `json:"interleaved,omitempty"`
	Status           string          `json:"status,omitempty"`
	Cost             *ModelCost      `json:"cost,omitempty"`
	Limit            ModelLimit      `json:"limit"`
	Modalities       Modalities      `json:"modalities"`
}

type ModelCost struct {
	Input       float64 `json:"input,omitempty"`
	Output      float64 `json:"output,omitempty"`
	Reasoning   float64 `json:"reasoning,omitempty"`
	CacheRead   float64 `json:"cache_read,omitempty"`
	CacheWrite  float64 `json:"cache_write,omitempty"`
	InputAudio  float64 `json:"input_audio,omitempty"`
	OutputAudio float64 `json:"output_audio,omitempty"`
}

type ModelLimit struct {
	Context int64 `json:"context"`
	Input   int64 `json:"input,omitempty"`
	Output  int64 `json:"output"`
}

type Modalities struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

type Meta struct {
	SyncedAt      string `json:"synced_at"`
	ETag          string `json:"etag,omitempty"`
	SHA256        string `json:"sha256"`
	SourceURL     string `json:"source_url"`
	ProviderCount int    `json:"provider_count"`
	ModelCount    int    `json:"model_count"`
	Bytes         int64  `json:"bytes"`
}

type Policy struct {
	SourceURL         string `json:"source_url"`
	SyncPolicy        string `json:"sync_policy"`
	SyncIntervalHours int    `json:"sync_interval_hours"`
	AutoApply         string `json:"auto_apply"`
}

func DefaultPolicy() Policy {
	return Policy{
		SourceURL:         "https://models.dev/api.json",
		SyncPolicy:        "scheduled",
		SyncIntervalHours: 24,
		AutoApply:         "metadata_and_pricing",
	}
}

type SyncStats struct {
	Providers          int `json:"providers"`
	Models             int `json:"models"`
	LogosSynced        int `json:"logos_synced,omitempty"`
	LogosFailed        int `json:"logos_failed,omitempty"`
	LogosRemoved       int `json:"logos_removed,omitempty"`
	DeprecatedDisabled int `json:"deprecated_disabled,omitempty"`
	LLMRowsUpdated     int `json:"llm_rows_updated,omitempty"`
	AgentsUpdated      int `json:"agents_updated,omitempty"`
}

type SyncLogEntry struct {
	ID         string    `json:"id"`
	StartedAt  string    `json:"started_at"`
	FinishedAt string    `json:"finished_at"`
	Status     string    `json:"status"`
	Message    string    `json:"message,omitempty"`
	SourceURL  string    `json:"source_url"`
	ETag       string    `json:"etag,omitempty"`
	DryRun     bool      `json:"dry_run,omitempty"`
	Stats      SyncStats `json:"stats"`
	Errors     []string  `json:"errors,omitempty"`
}
```

- [ ] **Step 2: Create `internal/modelregistry/phase.go`**

```go
package modelregistry

import (
	"context"
	"time"
)

type PhaseStatus string

const (
	PhaseSucceeded PhaseStatus = "succeeded"
	PhaseFailed    PhaseStatus = "failed"
	PhaseSkipped   PhaseStatus = "skipped"
)

type Phase interface {
	Name() string
	Timeout() time.Duration
	Run(pc *PhaseContext) PhaseResult
}

type PhaseResult struct {
	PhaseName  string
	Status     PhaseStatus
	Duration   time.Duration
	Stats      map[string]int
	Errors     []string
	Checkpoint *MigrationCheckpoint
}

type PhaseContext struct {
	Ctx        context.Context
	Store      *Store
	Backend    ApplyBackend
	Directory  Directory
	Policy     Policy
	Checkpoint *MigrationCheckpoint
}
```

- [ ] **Step 3: Create `internal/modelregistry/store_provider.go`**

```go
package modelregistry

import "context"

type StoreProvider interface {
	Store(ctx context.Context) (*Store, error)
}
```

- [ ] **Step 4: Verify new package compiles**

Run: `go build ./internal/modelregistry/...`
Expected: PASS (no import errors)

---

## Task 2: Create Domain Layer — Store + Supporting Logic

**Files:**
- Create: `internal/modelregistry/store.go`
- Create: `internal/modelregistry/pricing.go`
- Create: `internal/modelregistry/policy_validate.go`
- Create: `internal/modelregistry/urlguard.go`
- Create: `internal/modelregistry/chips.go`
- Create: `internal/modelregistry/migration_map.go`
- Create: `internal/modelregistry/migration_checkpoint.go`
- Create: `internal/modelregistry/runtime_overlay.json`

- [ ] **Step 1: Port `store.go` from modelcatalog**

Copy `internal/modelcatalog/store.go` to `internal/modelregistry/store.go`, change:
- `package modelcatalog` → `package modelregistry`
- `LoadCatalog() (Catalog, Meta, error)` → `LoadDirectory() (Directory, Meta, error)`
- `SaveCatalog(cat Catalog, meta Meta)` → `SaveDirectory(dir Directory, meta Meta)`
- `CountCatalog(cat Catalog)` → `CountDirectory(dir Directory)`
- All `Catalog` type references → `Directory`
- `SearchCatalogBlocks` → `SearchDirectoryBlocks` (store method delegates to search.go)

- [ ] **Step 2: Port remaining support files**

Copy and adapt each file from `internal/modelcatalog/`:
- `pricing.go` — change package name only
- `policy_validate.go` — change package name only
- `urlguard.go` — change package name, `ValidateCatalogSourceURL` → `ValidateDirectorySourceURL`
- `chips.go` — change package name only
- `migration_map.go` — change package name only
- `migration_checkpoint.go` — change package name only
- `runtime_overlay.json` — copy as-is

- [ ] **Step 3: Verify package compiles**

Run: `go build ./internal/modelregistry/...`
Expected: PASS

---

## Task 3: Create Domain Layer — Sync + Fetch + Apply + Migrate + Logos

**Files:**
- Create: `internal/modelregistry/fetch.go`
- Create: `internal/modelregistry/fetch_retry.go`
- Create: `internal/modelregistry/sync.go`
- Create: `internal/modelregistry/apply.go`
- Create: `internal/modelregistry/migrate_bindings.go`
- Create: `internal/modelregistry/migrate.go`
- Create: `internal/modelregistry/logos.go`
- Create: `internal/modelregistry/overlay.go`
- Create: `internal/modelregistry/search.go`
- Create: `internal/modelregistry/config_merge.go`
- Create: `internal/modelregistry/backfill.go`
- Create: `internal/modelregistry/logs.go`

- [ ] **Step 1: Port `fetch.go` + `fetch_retry.go`**

Copy from modelcatalog, change:
- `package modelcatalog` → `package modelregistry`
- `FetchCatalog` → `FetchDirectory`
- `ParseCatalog` → `ParseDirectory`
- Return types use `Directory` instead of `Catalog`

- [ ] **Step 2: Port `sync.go` — Remove SyncProviderLogos call**

Copy from modelcatalog, change:
- `package modelcatalog` → `package modelregistry`
- All `Catalog` → `Directory`
- **Remove** the `SyncProviderLogos` call from `Syncer.Sync()` (lines 120-126 in original)
- `FetchCatalog` → `FetchDirectory`
- `ParseCatalog` → `ParseDirectory`
- `CountCatalog` → `CountDirectory`
- `SaveCatalog` → `SaveDirectory`
- `LoadCatalog` → `LoadDirectory`

- [ ] **Step 3: Port `apply.go` — Add BatchApply types**

Copy from modelcatalog, change:
- `package modelcatalog` → `package modelregistry`
- All `Catalog` → `Directory`
- Add new batch interfaces to `ApplyBackend`:

```go
type ApplyBackend interface {
	ListProviderModels(ctx context.Context) ([]ApplyRow, error)
	SaveProviderModel(ctx context.Context, row ApplyRow) error
	UpsertModelPricing(ctx context.Context, provider, model string, micro MicroPricing, source string) error
	CountProviderBindings(ctx context.Context, provider string) (ApplyMigrationStats, error)
	MigrateProviderBindings(ctx context.Context, from, to string) (ApplyMigrationStats, error)
	BatchMigrateProviderBindings(ctx context.Context, rules []ProviderMigrationRule, skipRules []string) BatchMigrationResult
	BatchApply(ctx context.Context, patches []ApplyRow, pricing []PricingUpsert) BatchApplyResult
}

type PricingUpsert struct {
	Provider string
	Model    string
	Micro    MicroPricing
	Source   string
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

- [ ] **Step 4: Port `migrate_bindings.go` + `migrate.go` + `logos.go` + `overlay.go` + `search.go` + `config_merge.go` + `backfill.go` + `logs.go`**

Copy each from modelcatalog, change:
- `package modelcatalog` → `package modelregistry`
- All `Catalog` → `Directory`
- `SearchCatalogBlocks` → `SearchDirectoryBlocks`
- `IsCatalogJSONBlock` → `IsDirectoryJSONBlock`
- `isCatalogManaged` → `isDirectoryManaged`
- `catalog_managed` / `catalog_source` JSON field names — **keep unchanged** (these are persisted in DB, renaming would break existing data)
- `CountProviders` / `ListProviders` / `CountModels` / `ListModels` — parameter type `Catalog` → `Directory`

- [ ] **Step 5: Verify package compiles**

Run: `go build ./internal/modelregistry/...`
Expected: PASS

---

## Task 4: Create Domain Layer — Phase Implementations

**Files:**
- Create: `internal/modelregistry/fetch_phase.go`
- Create: `internal/modelregistry/migrate_phase.go`
- Create: `internal/modelregistry/apply_phase.go`
- Create: `internal/modelregistry/logo_phase.go`

- [ ] **Step 1: Create `fetch_phase.go`**

```go
package modelregistry

import "time"

type FetchPhase struct{}

func NewFetchPhase() *FetchPhase { return &FetchPhase{} }

func (p *FetchPhase) Name() string         { return "fetch" }
func (p *FetchPhase) Timeout() time.Duration { return 120 * time.Second }

func (p *FetchPhase) Run(pc *PhaseContext) PhaseResult {
	syncer := NewSyncer(pc.Store)
	out, err := syncer.Sync(pc.Ctx, SyncInput{})
	if err != nil {
		return PhaseResult{PhaseName: "fetch", Status: PhaseFailed, Errors: []string{err.Error()}}
	}
	if out.Status == "ok" && out.Meta.ETag != "" && out.Log.Message == "not modified (304)" {
		return PhaseResult{PhaseName: "fetch", Status: PhaseSkipped}
	}
	return PhaseResult{
		PhaseName: "fetch",
		Status:    PhaseSucceeded,
		Stats:     map[string]int{"providers": out.Meta.ProviderCount, "models": out.Meta.ModelCount},
	}
}
```

- [ ] **Step 2: Create `migrate_phase.go`**

```go
package modelregistry

import "time"

type MigratePhase struct {
	backend ApplyBackend
}

func NewMigratePhase(backend ApplyBackend) *MigratePhase {
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
		Stats:      map[string]int{"agents": result.Stats.Agents, "sessions": result.Stats.Sessions, "eval": result.Stats.Eval, "runtime_settings": result.Stats.RuntimeSettings, "skills": result.Stats.Skills, "knowledge_embed": result.Stats.KnowledgeEmbed, "web_research": result.Stats.WebResearch},
		Errors:     result.Errors,
		Checkpoint: NewCheckpoint(result.CompletedRules),
	}
}

func NewCheckpoint(completedRules []string) *MigrationCheckpoint {
	return &MigrationCheckpoint{CompletedRules: completedRules}
}
```

**Note:** `MigrationCheckpoint` struct needs a `CompletedRules []string` field added (currently only has `AppliedAt`, `Version`, `Stats`).

- [ ] **Step 3: Create `apply_phase.go`**

```go
package modelregistry

import (
	"encoding/json"
	"strings"
	"time"
)

type ApplyPhase struct {
	backend ApplyBackend
}

func NewApplyPhase(backend ApplyBackend) *ApplyPhase {
	return &ApplyPhase{backend: backend}
}

func (p *ApplyPhase) Name() string         { return "apply" }
func (p *ApplyPhase) Timeout() time.Duration { return 300 * time.Second }

func (p *ApplyPhase) Run(pc *PhaseContext) PhaseResult {
	if pc.Backend == nil || len(pc.Directory) == 0 {
		return PhaseResult{PhaseName: "apply", Status: PhaseSkipped}
	}
	mode := strings.ToLower(strings.TrimSpace(pc.Policy.AutoApply))
	if mode == "" || mode == "none" {
		return PhaseResult{PhaseName: "apply", Status: PhaseSkipped}
	}

	rows, err := p.backend.ListProviderModels(pc.Ctx)
	if err != nil {
		return PhaseResult{PhaseName: "apply", Status: PhaseFailed, Errors: []string{err.Error()}}
	}

	var patches []ApplyRow
	var pricingUpserts []PricingUpsert
	llmRowsUpdated := 0
	llmRowsDisabled := 0

	for _, row := range rows {
		var cfgProbe map[string]any
		_ = json.Unmarshal([]byte(row.ConfigJSON), &cfgProbe)
		if shouldSkipDirectoryApply(cfgProbe, row.MetadataJSON) {
			continue
		}
		providerID := MigrateProviderCode(row.Provider)
		prov, ok := pc.Directory[providerID]
		if !ok {
			continue
		}
		model, ok := prov.Models[row.Model]
		if !ok {
			continue
		}

		patch := row
		backfillChanged := false
		if backfilled, ok := BackfillCostFromMicro(row.ConfigJSON); ok {
			patch.ConfigJSON = backfilled
			row.ConfigJSON = backfilled
			backfillChanged = true
		}
		if providerID != row.Provider {
			patch.Provider = providerID
			patch.Key = providerID + ":" + row.Model
		}

		baseURL := extractAPIBaseURL(row.ConfigJSON)
		cfg, cfgChanged := mergeCatalogIntoConfig(row.ConfigJSON, prov, model, mode, baseURL)
		meta, metaChanged := mergeCatalogMetadata(row.MetadataJSON, prov, model)
		if cfgChanged {
			patch.ConfigJSON = cfg
		}
		if metaChanged {
			patch.MetadataJSON = meta
		}

		if strings.EqualFold(model.Status, "deprecated") && isDirectoryManaged(patch.ConfigJSON) {
			if patch.Enabled {
				patch.Enabled = false
				llmRowsDisabled++
			}
		}

		if providerID != row.Provider || cfgChanged || metaChanged || backfillChanged || patch.Enabled != row.Enabled {
			patches = append(patches, patch)
			llmRowsUpdated++
		}

		pricingJSON := patch.ConfigJSON
		var wrap struct {
			Cost CostUSDPer1M `json:"cost"`
		}
		_ = json.Unmarshal([]byte(pricingJSON), &wrap)
		micro := MicroPricingFromCostBlock(wrap.Cost)
		if micro.Input == 0 && micro.Output == 0 && model.Cost != nil {
			_, micro = MicroPricingFromModelCost(model.Cost)
		}
		if micro.Input > 0 || micro.Output > 0 || micro.CacheRead > 0 || micro.CacheWrite > 0 || micro.Reasoning > 0 || micro.Embedding > 0 {
			pricingUpserts = append(pricingUpserts, PricingUpsert{
				Provider: patch.Provider,
				Model:    patch.Model,
				Micro:    micro,
				Source:   "models.dev-sync",
			})
		}
	}

	result := p.backend.BatchApply(pc.Ctx, patches, pricingUpserts)
	return PhaseResult{
		PhaseName: "apply",
		Status:    PhaseSucceeded,
		Stats:     map[string]int{"rows_updated": llmRowsUpdated, "rows_disabled": llmRowsDisabled, "pricing_updated": result.PricingUpdated},
		Errors:    result.Errors,
	}
}
```

- [ ] **Step 4: Create `logo_phase.go`**

```go
package modelregistry

import "time"

type LogoPhase struct{}

func NewLogoPhase() *LogoPhase { return &LogoPhase{} }

func (p *LogoPhase) Name() string         { return "logos" }
func (p *LogoPhase) Timeout() time.Duration { return 120 * time.Second }

func (p *LogoPhase) Run(pc *PhaseContext) PhaseResult {
	if len(pc.Directory) == 0 {
		return PhaseResult{PhaseName: "logos", Status: PhaseSkipped}
	}
	res := SyncProviderLogos(pc.Ctx, pc.Store, pc.Directory, defaultLogosBaseURL)
	status := PhaseSucceeded
	if res.Failed > 0 && res.Synced == 0 {
		status = PhaseFailed
	}
	return PhaseResult{
		PhaseName: "logos",
		Status:    status,
		Stats:     map[string]int{"synced": res.Synced, "failed": res.Failed, "removed": res.Removed},
		Errors:    res.Errors,
	}
}
```

- [ ] **Step 5: Update `migration_checkpoint.go` to add CompletedRules field**

```go
package modelregistry

type MigrationCheckpoint struct {
	AppliedAt      string               `json:"applied_at,omitempty"`
	Version        string               `json:"version,omitempty"`
	Stats          ApplyMigrationStats  `json:"stats,omitempty"`
	CompletedRules []string             `json:"completed_rules,omitempty"`
}

func NewMigrationCheckpoint(stats ApplyMigrationStats) MigrationCheckpoint {
	return MigrationCheckpoint{
		AppliedAt: nowRFC3339(),
		Version:   ProviderMigrationVersion,
		Stats:     stats,
	}
}
```

- [ ] **Step 6: Verify package compiles**

Run: `go build ./internal/modelregistry/...`
Expected: PASS

---

## Task 5: Port Tests from modelcatalog

**Files:**
- Create: `internal/modelregistry/search_test.go`
- Create: `internal/modelregistry/fetch_retry_test.go`
- Create: `internal/modelregistry/urlguard_test.go`
- Create: `internal/modelregistry/sync_test.go`
- Create: `internal/modelregistry/store_test.go`
- Create: `internal/modelregistry/pricing_test.go`
- Create: `internal/modelregistry/overlay_sync_test.go`
- Create: `internal/modelregistry/logos_test.go`
- Create: `internal/modelregistry/config_merge_test.go`
- Create: `internal/modelregistry/backfill_test.go`
- Create: `internal/modelregistry/apply_test.go`

- [ ] **Step 1: Copy and adapt all test files**

For each `_test.go` file in `internal/modelcatalog/`:
1. Copy to `internal/modelregistry/`
2. Change `package modelcatalog` → `package modelregistry`
3. Change all `Catalog` type references → `Directory`
4. Change `CountCatalog` → `CountDirectory`
5. Change `SearchCatalogBlocks` → `SearchDirectoryBlocks`
6. Change `IsCatalogJSONBlock` → `IsDirectoryJSONBlock`
7. Change `ValidateCatalogSourceURL` → `ValidateDirectorySourceURL`

- [ ] **Step 2: Run tests**

Run: `go test ./internal/modelregistry/... -count=1`
Expected: All tests PASS

---

## Task 6: Data Layer — BatchApply + BatchMigrate

**Files:**
- Create: `internal/data/model_registry_apply.go`
- Modify: `internal/data/data.go` (if ProviderSet needs update)

- [ ] **Step 1: Create `internal/data/model_registry_apply.go`**

Copy from `internal/data/model_catalog_apply.go`, change:
- `package data` stays
- `modelcatalog` → `modelregistry` in all imports
- `modelCatalogApplyBackend` → `modelRegistryApplyBackend`
- `NewModelCatalogApplyBackend` → `NewModelRegistryApplyBackend`
- All `modelcatalog.` → `modelregistry.`
- Add `BatchMigrateProviderBindings` implementation:

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
	defer func() { _ = tx.Rollback() }()

	var result modelregistry.BatchMigrationResult
	now := nowRFC3339()

	for _, rule := range rules {
		rk := ruleKey(rule)
		if containsStr(skipRules, rk) {
			result.CompletedRules = append(result.CompletedRules, rk)
			continue
		}
		stats, err := migrateOneRuleInTx(ctx, tx, rule, now)
		if err != nil {
			result.FailedRules = append(result.FailedRules, rk)
			result.Errors = append(result.Errors, fmt.Sprintf("migrate %s→%s: %v", rule.Legacy, rule.Catalog, err))
			continue
		}
		result.CompletedRules = append(result.CompletedRules, rk)
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

func migrateOneRuleInTx(ctx context.Context, tx *sql.Tx, rule modelregistry.ProviderMigrationRule, now string) (modelregistry.ApplyMigrationStats, error) {
	var stats modelregistry.ApplyMigrationStats
	from, to := rule.Legacy, rule.Catalog

	res, err := tx.ExecContext(ctx, `UPDATE agents SET provider = ?, updated_at = ? WHERE provider = ?`, to, now, from)
	if err != nil {
		return stats, err
	}
	n, _ := res.RowsAffected()
	stats.Agents = int(n)

	res, err = tx.ExecContext(ctx, `UPDATE sessions SET default_provider = ? WHERE default_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.Sessions = int(n)
	res, err = tx.ExecContext(ctx, `UPDATE sessions SET last_provider = ? WHERE last_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.Sessions += int(n)

	res, err = tx.ExecContext(ctx, `UPDATE system_settings SET eval_sim_provider = ? WHERE eval_sim_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.Eval = int(n)
	res, err = tx.ExecContext(ctx, `UPDATE system_settings SET eval_judge_provider = ? WHERE eval_judge_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.Eval += int(n)

	res, err = tx.ExecContext(ctx, `UPDATE agent_runtime_settings SET l0_compress_provider = ? WHERE l0_compress_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.RuntimeSettings = int(n)
	res, err = tx.ExecContext(ctx, `UPDATE agent_runtime_settings SET memory_worker_provider = ? WHERE memory_worker_provider = ?`, to, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.RuntimeSettings += int(n)

	res, err = tx.ExecContext(ctx, `UPDATE skill SET provider = ?, updated_at = ? WHERE deleted_at = '' AND provider = ?`, to, now, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.Skills = int(n)

	res, err = tx.ExecContext(ctx, `UPDATE system_settings SET knowledge_embed_provider = ?, update_time = ? WHERE knowledge_embed_provider = ?`, to, now, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.KnowledgeEmbed = int(n)

	res, err = tx.ExecContext(ctx, `UPDATE system_settings SET web_research_provider = ?, update_time = ? WHERE web_research_provider = ?`, to, now, from)
	if err != nil {
		return stats, err
	}
	n, _ = res.RowsAffected()
	stats.WebResearch = int(n)

	res, err = tx.ExecContext(ctx, `UPDATE llm_provider_models SET provider = ?, model_key = ? || ':' || model, updated_at = ? WHERE provider = ?`, to, to, now, from)
	if err != nil {
		return stats, err
	}
	_, _ = res.RowsAffected()

	return stats, nil
}

func ruleKey(rule modelregistry.ProviderMigrationRule) string {
	return rule.Legacy + "->" + rule.Catalog
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
```

- Add `BatchApply` implementation:

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
	defer func() { _ = tx.Rollback() }()

	var result modelregistry.BatchApplyResult
	now := nowRFC3339()

	for _, p := range patches {
		if _, err := tx.ExecContext(ctx,
			`UPDATE llm_provider_models SET provider=?, model_key=?, name=?, enabled=?, config_json=?, metadata_json=?, updated_at=? WHERE id=?`,
			p.Provider, p.Key, p.Name, p.Enabled, p.ConfigJSON, p.MetadataJSON, now, p.ID,
		); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("save %s/%s: %v", p.Provider, p.Model, err))
			continue
		}
		result.RowsUpdated++
	}

	for _, p := range pricing {
		if err := b.upsertPricingInTx(ctx, tx, p); err != nil {
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

func (b *modelRegistryApplyBackend) upsertPricingInTx(ctx context.Context, tx *sql.Tx, p modelregistry.PricingUpsert) error {
	cost := modelregistry.CostUSDPer1M{
		Input:      modelregistry.MicroPer1KToUSDPer1M(p.Micro.Input),
		Output:     modelregistry.MicroPer1KToUSDPer1M(p.Micro.Output),
		CacheRead:  modelregistry.MicroPer1KToUSDPer1M(p.Micro.CacheRead),
		CacheWrite: modelregistry.MicroPer1KToUSDPer1M(p.Micro.CacheWrite),
		Reasoning:  modelregistry.MicroPer1KToUSDPer1M(p.Micro.Reasoning),
		Embedding:  modelregistry.MicroPer1KToUSDPer1M(p.Micro.Embedding),
	}
	return b.llm.UpsertModelPricingRule(ctx, biz.ModelPricingRule{
		ProviderCode:                  p.Provider,
		ModelAPIID:                    p.Model,
		Currency:                      "USD",
		InputPriceMicroUSDPer1K:       p.Micro.Input,
		OutputPriceMicroUSDPer1K:      p.Micro.Output,
		CachedInputPriceMicroUSDPer1K: p.Micro.CacheRead,
		CacheWritePriceMicroUSDPer1K:  p.Micro.CacheWrite,
		ReasoningPriceMicroUSDPer1K:   p.Micro.Reasoning,
		EmbeddingPriceMicroUSDPer1K:   p.Micro.Embedding,
		InputPriceUSDPer1M:            cost.Input,
		OutputPriceUSDPer1M:           cost.Output,
		CachedInputPriceUSDPer1M:      cost.CacheRead,
		CacheWritePriceUSDPer1M:       cost.CacheWrite,
		ReasoningPriceUSDPer1M:        cost.Reasoning,
		EmbeddingPriceUSDPer1M:        cost.Embedding,
		Source:                        p.Source,
		MetadataJSON:                  "{}",
	})
}
```

- [ ] **Step 2: Verify data layer compiles**

Run: `go build ./internal/data/...`
Expected: PASS (both old modelcatalog and new modelregistry references coexist)

---

## Task 7: Biz Layer — Update Usecase

**Files:**
- Create: `internal/biz/model_registry.go`
- Modify: `internal/biz/biz.go` (add new ProviderSet entry)

- [ ] **Step 1: Create `internal/biz/model_registry.go`**

Copy from `internal/biz/model_catalog.go`, change:
- `ModelCatalogRootResolver` → `ModelRegistryRootResolver`
- `ModelCatalogStoreProvider` → `ModelRegistryStoreProvider`
- `ModelCatalogUsecase` → `ModelRegistryUsecase`
- `ModelCatalogStatusView` → `ModelRegistryStatusView`
- `modelcatalog` → `modelregistry` in imports
- All `modelcatalog.` → `modelregistry.`
- Remove `runner *modelcatalog.Runner` field and `SetRunner` method
- Remove `applier *modelcatalog.Applier` field and `SetApplier` method
- `LoadCatalog` → `LoadDirectory`
- `CatalogLoaded` → `DirectoryLoaded` in StatusView
- `SearchCatalogBlocks` → `SearchDirectoryBlocks`

Key changes in `Sync` method:
- Remove `u.runner.SyncNow()` path — always use direct syncer
- After sync, run Phase pipeline: FetchPhase → MigratePhase → ApplyPhase → LogoPhase

- [ ] **Step 2: Verify biz layer compiles**

Run: `go build ./internal/biz/...`
Expected: PASS

---

## Task 8: Service Layer — Update Service

**Files:**
- Modify: `internal/service/model_catalog.go`

- [ ] **Step 1: Update `internal/service/model_catalog.go`**

Change:
- `modelcatalog` → `modelregistry` in imports
- All `modelcatalog.` → `modelregistry.`
- `biz.ModelCatalogUsecase` → `biz.ModelRegistryUsecase`
- `biz.ModelCatalogStatusView` → `biz.ModelRegistryStatusView`
- `ModelCatalogService` struct field `uc *biz.ModelCatalogUsecase` → `uc *biz.ModelRegistryUsecase`
- `NewModelCatalogService` parameter type → `*biz.ModelRegistryUsecase`
- `toProtoCatalogPolicy` → `toProtoRegistryPolicy`
- `toProtoCatalogStatus` → `toProtoRegistryStatus`
- `modelcatalog.Policy{...}` → `modelregistry.Policy{...}`
- `modelcatalog.ProviderMigrationVersion` → `modelregistry.ProviderMigrationVersion`

- [ ] **Step 2: Verify service layer compiles**

Run: `go build ./internal/service/...`
Expected: PASS

---

## Task 9: Update External References — biz/data/usage

**Files:**
- Modify: `internal/biz/usage/usage.go`
- Modify: `internal/biz/llm_provider_model.go`
- Modify: `internal/biz/llm_provider_model_pricing_test.go`
- Modify: `internal/data/usage_write.go`

- [ ] **Step 1: Update `internal/biz/usage/usage.go`**

Change import: `"aranea-agents/internal/modelcatalog"` → `"aranea-agents/internal/modelregistry"`
Change all `modelcatalog.` → `modelregistry.`

- [ ] **Step 2: Update `internal/biz/llm_provider_model.go`**

Change import: `"aranea-agents/internal/modelcatalog"` → `"aranea-agents/internal/modelregistry"`
Change all `modelcatalog.` → `modelregistry.`

- [ ] **Step 3: Update `internal/biz/llm_provider_model_pricing_test.go`**

Change import and all `modelcatalog.` → `modelregistry.`

- [ ] **Step 4: Update `internal/data/usage_write.go`**

Change import and `modelcatalog.MigrateProviderCode` → `modelregistry.MigrateProviderCode`

- [ ] **Step 5: Verify compilation**

Run: `go build ./internal/...`
Expected: PASS

---

## Task 10: Bridge Layer — ModelRegistrySyncAgent

**Files:**
- Create: `internal/agent/model_registry_sync.go`

- [ ] **Step 1: Create `internal/agent/model_registry_sync.go`**

```go
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	trpcagent "aranea-agents/pkg/trpc-agent-go/agent"
	trpcevent "aranea-agents/pkg/trpc-agent-go/event"
	trpctool "aranea-agents/pkg/trpc-agent-go/tool"
	"aranea-agents/internal/modelregistry"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
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
			emitAgentError(ch, err)
			return
		}

		policy, _ := store.LoadPolicy()

		pc := &modelregistry.PhaseContext{
			Ctx:     ctx,
			Store:   store,
			Backend: a.backend,
			Policy:  policy,
		}

		for _, phase := range a.phases {
			phaseCtx, cancel := context.WithTimeout(ctx, phase.Timeout())
			pc.Ctx = phaseCtx

			emitPhaseStart(ch, phase.Name())
			start := time.Now()
			result := phase.Run(pc)
			result.Duration = time.Since(start)
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

		emitAgentCompletion(ch)
	})

	return ch, nil
}

func (a *ModelRegistrySyncAgent) Tools() []trpctool.Tool { return a.tools }
func (a *ModelRegistrySyncAgent) Info() trpcagent.Info {
	return trpcagent.Info{Name: "model-registry-sync", Description: "Model registry sync agent (programmatic)"}
}
func (a *ModelRegistrySyncAgent) SubAgents() []trpcagent.Agent { return nil }
func (a *ModelRegistrySyncAgent) FindSubAgent(string) trpcagent.Agent { return nil }

func emitPhaseStart(ch chan<- *trpcevent.Event, phase string) {
	evt := trpcevent.New("model-registry-sync", "model-registry-sync")
	evt.Tag = "phase_start"
	evt.Extensions = map[string]json.RawMessage{
		"phase": json.RawMessage(`"` + phase + `"`),
	}
	trpcevent.EmitEvent(context.Background(), ch, evt)
}

func emitPhaseResult(ch chan<- *trpcevent.Event, phase string, result modelregistry.PhaseResult) {
	evt := trpcevent.New("model-registry-sync", "model-registry-sync")
	evt.Tag = "phase_" + string(result.Status)
	evt.Extensions = map[string]json.RawMessage{
		"phase":    json.RawMessage(`"` + phase + `"`),
		"status":   json.RawMessage(`"` + string(result.Status) + `"`),
		"duration": json.RawMessage(fmt.Sprintf(`%d`, result.Duration.Milliseconds())),
	}
	trpcevent.EmitEvent(context.Background(), ch, evt)
}

func emitAgentError(ch chan<- *trpcevent.Event, err error) {
	evt := trpcevent.NewErrorEvent("model-registry-sync", "model-registry-sync", "sync_error", err.Error())
	trpcevent.EmitEvent(context.Background(), ch, evt)
}

func emitAgentCompletion(ch chan<- *trpcevent.Event) {
	evt := trpcevent.New("model-registry-sync", "model-registry-sync")
	evt.Done = true
	evt.Object = "runner.completion"
	trpcevent.EmitEvent(context.Background(), ch, evt)
}
```

- [ ] **Step 2: Verify agent layer compiles**

Run: `go build ./internal/agent/...`
Expected: PASS

---

## Task 11: Bridge Layer — CallableTools

**Files:**
- Create: `internal/tools/model_registry_sync.go`

- [ ] **Step 1: Create `internal/tools/model_registry_sync.go`**

```go
package tools

import (
	"context"
	"fmt"

	trpctool "aranea-agents/pkg/trpc-agent-go/tool"
	"aranea-agents/internal/modelregistry"
)

type fetchDirectoryTool struct {
	phase *modelregistry.FetchPhase
}

func (t *fetchDirectoryTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name:        "fetch_model_directory",
		Description: "Fetch the latest model directory from models.dev",
	}
}

func (t *fetchDirectoryTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	pc := modelregistry.PhaseFromCtx(ctx)
	if pc == nil {
		return nil, fmt.Errorf("fetch_model_directory: no phase context in ctx")
	}
	result := t.phase.Run(pc)
	if result.Status == modelregistry.PhaseFailed {
		return nil, fmt.Errorf("fetch failed: %v", result.Errors)
	}
	return result, nil
}

type migrateProvidersTool struct {
	phase *modelregistry.MigratePhase
}

func (t *migrateProvidersTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name:        "migrate_provider_bindings",
		Description: "Migrate legacy provider bindings to current provider IDs",
	}
}

func (t *migrateProvidersTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	pc := modelregistry.PhaseFromCtx(ctx)
	if pc == nil {
		return nil, fmt.Errorf("migrate_provider_bindings: no phase context in ctx")
	}
	result := t.phase.Run(pc)
	if result.Status == modelregistry.PhaseFailed {
		return nil, fmt.Errorf("migrate failed: %v", result.Errors)
	}
	return result, nil
}

type applyDirectoryTool struct {
	phase *modelregistry.ApplyPhase
}

func (t *applyDirectoryTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name:        "apply_model_directory",
		Description: "Apply model directory changes to database",
	}
}

func (t *applyDirectoryTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	pc := modelregistry.PhaseFromCtx(ctx)
	if pc == nil {
		return nil, fmt.Errorf("apply_model_directory: no phase context in ctx")
	}
	result := t.phase.Run(pc)
	if result.Status == modelregistry.PhaseFailed {
		return nil, fmt.Errorf("apply failed: %v", result.Errors)
	}
	return result, nil
}

type syncProviderLogosTool struct {
	phase *modelregistry.LogoPhase
}

func (t *syncProviderLogosTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{
		Name:        "sync_provider_logos",
		Description: "Download and cache provider logos from models.dev",
	}
}

func (t *syncProviderLogosTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	pc := modelregistry.PhaseFromCtx(ctx)
	if pc == nil {
		return nil, fmt.Errorf("sync_provider_logos: no phase context in ctx")
	}
	result := t.phase.Run(pc)
	if result.Status == modelregistry.PhaseFailed {
		return nil, fmt.Errorf("logo sync failed: %v", result.Errors)
	}
	return result, nil
}
```

**Note:** Add `PhaseFromCtx` / `WithPhaseCtx` helpers to `internal/modelregistry/phase.go`:

```go
import "context"

type phaseCtxKey struct{}

func WithPhaseCtx(ctx context.Context, pc *PhaseContext) context.Context {
	return context.WithValue(ctx, phaseCtxKey{}, pc)
}

func PhaseFromCtx(ctx context.Context) *PhaseContext {
	pc, _ := ctx.Value(phaseCtxKey{}).(*PhaseContext)
	return pc
}
```

- [ ] **Step 2: Verify tools layer compiles**

Run: `go build ./internal/tools/...`
Expected: PASS

---

## Task 12: Wire Injection + CronRunner Integration

**Files:**
- Modify: `cmd/admin/wire.go`
- Modify: `cmd/admin/main.go`
- Modify: `internal/cronrunner/runner.go` (add model-registry-sync routing)
- Modify: `internal/server/service_registry.go`

- [ ] **Step 1: Update `cmd/admin/wire.go`**

Replace `provideModelCatalogRunner`:

```go
func provideModelRegistrySyncAgent(sys biz.SystemSettingRepo, llm biz.LlmProviderModelRepo, d *data.Data) *agent.ModelRegistrySyncAgent {
	storeProv := biz.NewModelRegistryStoreProvider(biz.NewSystemSettingRootAdapter(sys))
	backend := data.NewModelRegistryApplyBackend(d, llm)
	ag, _ := agent.BuildModelRegistrySyncAgent(storeProv, backend)
	return ag
}
```

Replace `provideModelCatalogUsecase`:

```go
func provideModelRegistryUsecase(sys biz.SystemSettingRepo, llm biz.LlmProviderModelRepo, d *data.Data) *biz.ModelRegistryUsecase {
	uc := biz.NewModelRegistryUsecase(biz.NewSystemSettingRootAdapter(sys))
	backend := data.NewModelRegistryApplyBackend(d, llm)
	uc.SetApplyBackend(backend)
	return uc
}
```

Update `wireOut` struct:
- Remove `ModelCatalogRunner *modelcatalog.Runner`
- Add `ModelRegistrySyncAgent *agent.ModelRegistrySyncAgent`

Update `provideWireOut`:
- Remove `modelCatalogRunner *modelcatalog.Runner` parameter
- Add `modelRegistrySyncAgent *agent.ModelRegistrySyncAgent` parameter
- Update field assignment

Update wire provider list:
- Replace `provideModelCatalogRunner` → `provideModelRegistrySyncAgent`
- Replace `provideModelCatalogUsecase` → `provideModelRegistryUsecase`

Remove `modelcatalog` import, add `agent` import.

- [ ] **Step 2: Update `cmd/admin/main.go`**

Replace:
```go
if out.ModelCatalogRunner != nil {
    out.ModelCatalogRunner.Start(cronCtx)
    logger.Log(log.LevelInfo, "msg", "model catalog sync runner started", "interval", "1h")
}
```

With:
```go
if out.ModelRegistrySyncAgent != nil {
    safego.Go(cronCtx, "modelregistry.cron_seed", func() {
        if err := biz.SeedModelRegistryCronTask(cronCtx, out.CronRepo); err != nil {
            event.SysLogWarn("modelregistry.cron_seed", "Failed to seed model registry cron task", event.P("error", err))
        }
    })
    logger.Log(log.LevelInfo, "msg", "model registry sync agent registered", "schedule", "via CronRunner")
}
```

- [ ] **Step 3: Add `SeedModelRegistryCronTask` to `internal/biz/model_registry.go`**

```go
func SeedModelRegistryCronTask(ctx context.Context, cronRepo CronRepo) error {
	tasks, err := cronRepo.ListCronTasks(ctx)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.TaskKey == "model-registry-sync" {
			return nil
		}
	}
	_, err = cronRepo.CreateCronTask(ctx, CronTask{
		TaskKey:  "model-registry-sync",
		Name:     "Model Registry Sync",
		Enabled:  true,
		Status:   "active",
		ConfigJSON: `{"message":"sync model registry","interval_seconds":3600}`,
	})
	return err
}
```

- [ ] **Step 4: Verify wire + build**

Run: `make wire && go build ./cmd/admin`
Expected: PASS

---

## Task 13: Delete Old modelcatalog Package

**Files:**
- Delete: `internal/modelcatalog/` (entire directory)
- Delete: `internal/data/model_catalog_apply.go`

- [ ] **Step 1: Delete old package**

Run: `rm -rf internal/modelcatalog/`
Run: `rm internal/data/model_catalog_apply.go`
Run: `rm internal/biz/model_catalog.go`

- [ ] **Step 2: Search for any remaining `modelcatalog` references**

Run: `grep -r "modelcatalog" internal/ cmd/ --include="*.go" -l`
Expected: No results (proto-generated code in `api/` is separate and uses `modelcatalogv1`)

- [ ] **Step 3: Verify full build**

Run: `make api && make wire && make build`
Expected: PASS

---

## Task 14: Full Verification

**Files:** None (verification only)

- [ ] **Step 1: Run all modelregistry tests**

Run: `go test ./internal/modelregistry/... -count=1 -v`
Expected: All PASS

- [ ] **Step 2: Run biz/data tests**

Run: `go test ./internal/biz/... ./internal/data/... -count=1`
Expected: All PASS

- [ ] **Step 3: Run service tests**

Run: `go test ./internal/service/... -count=1`
Expected: All PASS

- [ ] **Step 4: Run full build**

Run: `make api && make wire && make build && make test && make lint`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "refactor: modelcatalog → modelregistry with trpc-agent-go Agent+Tool system

- Rename internal/modelcatalog/ → internal/modelregistry/
- Rename Catalog type → Directory
- Add Phase interface with independent timeouts per phase
- Add BatchApply + BatchMigrate for bulk DB operations
- Add Checkpoint to skip completed migration rules
- Add ModelRegistrySyncAgent implementing agent.Agent
- Add 4 CallableTools (Fetch/Migrate/Apply/Logo)
- Replace standalone Runner with CronRunner scheduling
- Fix red line #9: bare go func() → safego.Go
- Fix red line #10: log.Printf → EventBus/FlowLog
- Decouple logo sync from main pipeline"
```

---

## Out of Scope (Separate Tasks)

The following naming cleanups are **not** part of this refactoring:
- `runtime.Catalog` → `TurnDeps` (55+ references across 15 files)
- `provider.CatalogConfig` → `ProviderConfig` (40+ references across 4 files)
- `session.ModelConfigCatalog` → `ModelConfigResolver` (4 references in 1 file)
- Frontend `model-catalog` feature directory rename
- `Repository` vs `Repo` suffix unification
- `Port`/`Bridge`/`Lookup`/`Resolver` suffix unification
