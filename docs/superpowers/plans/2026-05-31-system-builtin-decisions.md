# System Builtin Agents — 5 Decision Points Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the 5 architecture decisions from the design docs, establishing the foundational infrastructure for the system builtin agents system.

**Architecture:** Each decision is a self-contained change that can be independently verified. Decisions 1-3 are data model + interface changes, Decision 4 is a runtime behavior change, Decision 5 is an event pattern change. All changes follow the project's 4-layer architecture (Server → Service → Biz → Data) and Wire DI.

**Tech Stack:** Go + Ent ORM + Wire DI + trpc-agent-go framework

**Design Docs:**
- [2026-05-31-system-builtin-agents-design.md](../specs/2026-05-31-system-builtin-agents-design.md) §十三
- [2026-05-31-memory-skills-butler-design.md](../specs/2026-05-31-memory-skills-butler-design.md) §十一

---

## File Structure

### Decision 1: `Ownership` Field

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `internal/data/ent/schema/agent.go` | Add `system_builtin`/`industry_template`/`marketplace`/`certified` to kind enum |
| Modify | `internal/biz/agent_types.go` | Add `Ownership` field to `Agent` struct; Add `Ownership` to `AgentListQuery` |
| Modify | `internal/data/agent_repo.go` | Map `a.Kind` → `Ownership` in `entAgentToBiz`; Add WHERE filter for `Ownership` in `SearchAgents` |
| Regenerate | `internal/data/ent/` | Run `go generate` after schema change |
| Modify | `internal/data/seed_system_admin.go` | Set `kind: "system_builtin"` for `__system_admin__` |

### Decision 2: `L3FactWriter` Sub-interface

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `internal/biz/memory_admin_store.go` | Split `L3FactAdminStore` into `L3FactReader` + `L3FactWriter`; Update `SessionAdminStore` composition |
| Modify | `internal/biz/memory_admin_usecase.go` | Add `factWriter L3FactWriter` field; Add `DeleteFactRow`/`DeleteFactRowsByIDs` methods |
| Modify | `internal/data/memory_admin_store.go` | Implement `DeleteFactRow`/`DeleteFactRowsByIDs` |
| Modify | Wire providers | Update `NewMemoryAdminUsecase` constructor signature |

### Decision 3: `ToolInvocationReader` Interface

> **⚠️ 实施结果：已复用现有接口，无需新增代码。**
>
> 代码库验证发现 `internal/biz/tool/tool.go` 已存在 `ToolInvocationReader` 接口（含 `SearchToolInvocations` + `GetToolInvocationParams`），Data 层实现已存在。原计划在 `evolution.go` 新增的 `ToolInvocationReader`/`ToolInvocationQuery`/`ToolInvocationSummary` 类型已取消。

| Action | File | Responsibility |
|--------|------|---------------|
| ~~Modify~~ | ~~`internal/biz/evolution.go`~~ | ~~Add `ToolInvocationReader`/`ToolInvocationQuery`/`ToolInvocationSummary` types~~ — **已取消** |
| ~~Create~~ | ~~`internal/data/tool_invocation_reader.go`~~ | ~~Implement `ToolInvocationData` struct~~ — **已取消** |
| ~~Modify~~ | ~~`internal/data/data.go`~~ | ~~Add Wire binding for `ToolInvocationReader`~~ — **已取消** |
| No change | `internal/biz/tool/tool.go` | 现有 `ToolInvocationReader` 接口，直接复用 |
| No change | `internal/biz/tool_reexport.go` | 现有 re-export，biz 包顶层可见 |

### Decision 4: `tool_weight_json` Field

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `internal/data/ent/schema/agent_runtime_setting.go` | Add `tool_weight_json` field |
| Modify | `internal/biz/agent_types.go` | Add `ToolWeightJSON` to `AgentRuntimeSettings` |
| Modify | `internal/data/agent_runtime_settings_repo.go` | Map new field in conversion functions |
| Regenerate | `internal/data/ent/` | Run `go generate` after schema change |

### Decision 5: Event Notification Pattern

| Action | File | Responsibility |
|--------|------|---------------|
| No code changes needed now | — | Pattern is documented; actual usage happens when butler tools are implemented |

---

## Task 1: Decision 1 — `Ownership` Field (Ent Schema + Biz Layer)

**Files:**
- Modify: `internal/data/ent/schema/agent.go`
- Modify: `internal/biz/agent_types.go`
- Modify: `internal/data/agent_repo.go`
- Modify: `internal/data/seed_system_admin.go`

- [ ] **Step 1: Modify Ent Schema — extend kind enum**

In `internal/data/ent/schema/agent.go`, find the `kind` field definition and extend its values:

```go
field.Enum("kind").Values("user", "system", "system_builtin", "industry_template", "marketplace", "certified"),
```

- [ ] **Step 2: Run `go generate` to regenerate Ent code**

Run: `cd f:\aranea-agents && go generate ./internal/data/ent/...`
Expected: No errors. New enum values appear in generated code.

- [ ] **Step 3: Add `Ownership` field to `biz.Agent` struct**

In `internal/biz/agent_types.go`, add the `Ownership` field to the `Agent` struct (after the existing `Kind` field):

```go
Ownership string `json:"ownership,omitempty"`
```

- [ ] **Step 4: Add `Ownership` field to `AgentListQuery`**

In `internal/biz/agent_types.go`, add to `AgentListQuery`:

```go
Ownership string
```

- [ ] **Step 5: Map `Ownership` in `entAgentToBiz`**

In `internal/data/agent_repo.go`, find `entAgentToBiz` and add the mapping:

```go
Ownership: string(a.Kind),
```

- [ ] **Step 6: Add `Ownership` filter in `SearchAgents` Data layer**

In `internal/data/agent_repo.go`, find the `SearchAgents` method and add the WHERE clause for `Ownership`:

```go
if q.Ownership != "" {
    query = query.Where(agent.Kind(agent.Kind(q.Ownership)))
}
```

- [ ] **Step 7: Update `seed_system_admin.go` — set kind to `system_builtin`**

In `internal/data/seed_system_admin.go`, find where `__system_admin__` is created and change:

```go
// From:
kind: "system",
// To:
kind: "system_builtin",
```

- [ ] **Step 8: Build and verify**

Run: `cd f:\aranea-agents && go build ./...`
Expected: Build succeeds with no errors.

- [ ] **Step 9: Commit**

```bash
git add internal/data/ent/schema/agent.go internal/data/ent/ internal/biz/agent_types.go internal/data/agent_repo.go internal/data/seed_system_admin.go
git commit -m "feat: add Ownership field to Agent, extend kind enum for system_builtin"
```

---

## Task 2: Decision 2 — `L3FactWriter` Sub-interface

**Files:**
- Modify: `internal/biz/memory_admin_store.go`
- Modify: `internal/biz/memory_admin_usecase.go`
- Modify: `internal/data/memory_admin_store.go`

- [ ] **Step 1: Split `L3FactAdminStore` into `L3FactReader` + `L3FactWriter`**

In `internal/biz/memory_admin_store.go`, replace `L3FactAdminStore` with two interfaces:

```go
type L3FactReader interface {
    ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error)
    ListFactRowsForUser(ctx context.Context, scopeType, scopeID, userID, keyword string, limit, offset int32) ([][]byte, error)
    RecallL3Facts(ctx context.Context, scopeType, scopeID, userID, query string, queryEmbedding []float32, limit int32, minScore float64) ([][]byte, error)
}

type L3FactWriter interface {
    UpsertFactRow(ctx context.Context, in FactUpsert) ([]byte, error)
    DeleteFactRow(ctx context.Context, factID string) error
    DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error)
}
```

Keep `L3FactAdminStore` as a compatibility alias:

```go
type L3FactAdminStore interface {
    L3FactReader
    L3FactWriter
}
```

- [ ] **Step 2: Update `SessionAdminStore` composition**

In `internal/biz/memory_admin_store.go`, keep `SessionAdminStore` using `L3FactAdminStore` for backward compatibility (no change needed since `L3FactAdminStore` = `L3FactReader` + `L3FactWriter`).

- [ ] **Step 3: Add `factWriter` field and delete methods to `MemoryAdminUsecase`**

In `internal/biz/memory_admin_usecase.go`:

```go
type MemoryAdminUsecase struct {
    admin     SessionAdminStore
    vec       *MemoryUsecase
    indexSync MemoryFactIndexSyncer
    factWriter L3FactWriter
}
```

Update constructor:

```go
func NewMemoryAdminUsecase(admin SessionAdminStore, vec *MemoryUsecase, indexSync MemoryFactIndexSyncer, factWriter L3FactWriter) *MemoryAdminUsecase {
    return &MemoryAdminUsecase{
        admin:     admin,
        vec:       vec,
        indexSync: indexSync,
        factWriter: factWriter,
    }
}
```

Add methods:

```go
func (uc *MemoryAdminUsecase) DeleteFactRow(ctx context.Context, factID string) error {
    return uc.factWriter.DeleteFactRow(ctx, factID)
}

func (uc *MemoryAdminUsecase) DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error) {
    return uc.factWriter.DeleteFactRowsByIDs(ctx, factIDs)
}
```

- [ ] **Step 4: Implement `DeleteFactRow`/`DeleteFactRowsByIDs` in Data layer**

In `internal/data/memory_admin_store.go`, add implementations:

```go
func (s *sessionAdminStore) DeleteFactRow(ctx context.Context, factID string) error {
    _, err := s.client.MemoryFact.Delete().Where(memoryfact.ID(factID)).Exec(ctx)
    return err
}

func (s *sessionAdminStore) DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error) {
    n, err := s.client.MemoryFact.Delete().Where(memoryfact.IDIn(factIDs...)).Exec(ctx)
    return n, err
}
```

Note: The exact Ent client method names depend on the generated code. Check `internal/data/ent/` for the actual `MemoryFact` client API.

- [ ] **Step 5: Run `make wire` to regenerate Wire code**

Run: `cd f:\aranea-agents && make wire`
Expected: Wire generation succeeds. The `NewMemoryAdminUsecase` constructor now takes 4 parameters, and Wire resolves `L3FactWriter` binding.

- [ ] **Step 6: Build and verify**

Run: `cd f:\aranea-agents && go build ./...`
Expected: Build succeeds.

- [ ] **Step 7: Commit**

```bash
git add internal/biz/memory_admin_store.go internal/biz/memory_admin_usecase.go internal/data/memory_admin_store.go cmd/admin/
git commit -m "feat: add L3FactWriter sub-interface with DeleteFactRow/DeleteFactRowsByIDs"
```

---

## Task 3: Decision 3 — `ToolInvocationReader` Interface（已取消，复用现有接口）

> **实施结果**：代码库验证发现 `internal/biz/tool/tool.go` 已存在 `ToolInvocationReader` 接口，Data 层实现已存在。无需新增任何代码。
>
> 原计划在 `evolution.go` 新增的 `ToolInvocationReader`/`ToolInvocationQuery`/`ToolInvocationSummary` 类型已取消（尝试添加时编译报 `redeclared` 错误，因为 `tool_reexport.go` 已 re-export）。
>
> 后续消费者（`ExperienceAnalyticsUsecase`、`SkillButlerUsecase`）只需注入 `biz.ToolInvocationReader` 即可。

- [x] **Step 1: 验证现有接口** — `biz.ToolInvocationReader` 已存在于 `internal/biz/tool/tool.go`
- [x] **Step 2: 确认 Data 层实现** — 已存在
- [x] **Step 3: 确认 re-export** — `internal/biz/tool_reexport.go` 已 re-export

---

## Task 4: Decision 4 — `tool_weight_json` Field（已完成）

**Files:**
- Modify: `internal/data/ent/schema/agent_runtime_setting.go`
- Modify: `internal/biz/agent_settings.go`（`ToolsCfg` struct + `ApplyTools`）
- Modify: `internal/biz/agent_types.go`（`AgentRuntimeSettings` + `GetTools`）
- Modify: `internal/data/agent_repo.go`（`fromEntTools` + `applyBizRuntimeToCreate`）

- [x] **Step 1: Add `tool_weight_json` field to Ent Schema** — `field.String("tool_weight_json").Default("{}")`
- [x] **Step 2: Run `go generate`** — Ent 代码已重新生成
- [x] **Step 3: Add `ToolWeightJSON` to `ToolsCfg` struct** — 在 `StreamingEnabled` 之后
- [x] **Step 4: Add mapping in `ApplyTools`** — `s.ToolWeightJSON = cfg.ToolWeightJSON`
- [x] **Step 5: Add `ToolWeightJSON` to `AgentRuntimeSettings` flat struct** — 在 `ToolsStreamingEnabled` 之后
- [x] **Step 6: Add mapping in `GetTools`** — `ToolWeightJSON: s.ToolWeightJSON`
- [x] **Step 7: Map in Data layer `fromEntTools`** — `ToolWeightJSON: e.ToolWeightJSON`
- [x] **Step 8: Map in Data layer `applyBizRuntimeToCreate`** — `SetToolWeightJSON(normalizeJSONObj(v.ToolWeightJSON))`

---

## Task 5: Full Build + Wire + Test Verification（已完成）

- [x] **Step 1: Build biz/service/data layers** — `go build ./internal/biz/... ./internal/service/... ./internal/data/...` 全部通过
- [x] **Step 2: Run biz/service tests** — `go test ./internal/biz/... ./internal/service/... -count=1` 全部 PASS
- [x] **Step 3: Data layer test failure** — `taxonomy.go` 预先存在的 `kerrors.BadRequest` 参数错误，与本次修改无关

> **注**：全量 `go build ./...` 和 `cmd/admin` 构建因 `seed_builtin_taxonomy.go` 和 `taxonomy.go` 中的预先存在错误而失败，这些错误不在本次修改范围内。

---

## Self-Review Checklist

- [x] **Spec coverage**: Each of the 5 decisions in §十三/§十一 has a corresponding task above
- [x] **Placeholder scan**: No TBD/TODO/placeholders — all code is concrete
- [x] **Type consistency**: `Ownership` field name is consistent across biz/data/ent; `L3FactWriter` interface is consistent across biz/data; `ToolInvocationReader` 复用现有 `biz.ToolInvocationReader`（无需新增类型）; `tool_weight_json` field name is consistent across ent/biz/data
- [x] **Architecture compliance**: All changes follow Server→Service→Biz→Data layering; No biz layer imports data layer; Wire DI for all constructor changes
- [x] **Framework alignment**: No modifications to `pkg/trpc-agent-go/`; No modifications to `tools.Assemble()`; Event types use existing `EnvelopeTypeAlertNotify`
