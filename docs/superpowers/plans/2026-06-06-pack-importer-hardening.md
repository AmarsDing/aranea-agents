# Pack Importer Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 [aranea-review 报告](file:///f:/aranea-agents/internal/biz/pack/importer.go) 中识别的 2 个 🔴 阻断 + 12 个 🟡 建议，让 Pack 导入引擎具备事务原子性、强引用校验、可观察的回滚与版本可演进能力。

**Architecture:**
- **接口拆分**: 把 16 方法的 `ImporterRepo` 拆为 4 个子域端口（Taxonomy / Agent / Team / Graph），符合 BI3/BI6；数据层 `PackRepoAdapter` 改为组合注入而非扁平转发。
- **原子化 Agent 导入**: 在 `agentRepo` 新增 `ImportAgentAtomic` 高阶方法，包裹 `UpdateAgent + ReplaceAgentPromptFiles + UpsertAgentRuntimeSettings` 三步到 `ExecInTx`；`AgentImporter` 端口只暴露这一对方法（Create/Update）。
- **强引用校验**: 必填跨实体引用（IntentAnchor / Synthesizer / Team.Members / TeamGraph.Nodes.AgentKey）改为硬失败；可选引用保留软失败但写入 `result.Warnings`。
- **Mapper 防御**: `KeyMapper` 加 `sync.RWMutex` 防御性并发安全；新增 `RegisterTeam/ResolveTeamKey`；`ConflictSkip` 路径补注册。
- **可演进性**: `OrchestrationSpec.Version` / `EmbeddedGraphSpec.Version` 从 `Pack.Manifest.APIVersion` 推导；`SubgraphPackSpec` 补 `InterruptBefore/After`；`FailurePolicy.CircuitBreaker.RecoveryTimeoutMs` 改 `RecoveryTimeoutSec`。

**Tech Stack:** Go 1.23, Ent ORM, Ent `ExecInTx`, kerrors, testify, pnpm 前端不动。

**Traceability (aranea-review):**
- 阻断项 #1 (Agent overwrite 事务性) → Phase 2
- 阻断项 #2 (ImporterRepo 上帝接口) → Phase 1
- 🟡 建议 #3-#14 → Phase 3-14

---

## 文件结构

| 文件 | 动作 | 责任 |
|------|------|------|
| `internal/biz/pack/ports.go` | 新增 | 4 个 Importer 子端口（Taxonomy/Agent/Team/Graph） |
| `internal/biz/pack/importer.go` | 修改 | `NewImporter` 改为组合注入；`importAgent`/`importTeam` 拆函数；硬失败+警告 |
| `internal/biz/pack/mapper.go` | 修改 | 加 `RegisterTeam/ResolveTeamKey/UnregisterAgent`；加 `sync.RWMutex` |
| `internal/biz/pack/spec.go` | 修改 | `SubgraphPackSpec` 加 `InterruptBefore/After`；`CircuitBreakerSpec` 改 `RecoveryTimeoutSec`；`Pack` 加可选 `OrchestrationVersion` |
| `internal/biz/pack/exporter.go` | 修改 | 同步 `RecoveryTimeoutSec` 输出 |
| `internal/biz/pack/convert.go` | 修改 | 同步 `RecoveryTimeoutSec` 转换 |
| `internal/biz/pack/importer_test.go` | 修改 | 按子域拆分 stub；新增硬失败/警告测试 |
| `internal/data/agent_repo.go` | 修改 | 新增 `ImportAgentAtomic(ctx, spec, files, settings)` |
| `internal/data/pack_repo.go` | 修改 | 适配新端口接口 |
| `internal/data/seed_pack.go` | 修改 | 调用新构造函数 |
| `internal/service/pack.go` | 修改 | 适配新构造函数（若签名变更） |

---

## Phase 1: 拆分 ImporterRepo 接口（阻断项 #2）

### Task 1: 定义 4 个子端口接口

**Files:**
- Create: `internal/biz/pack/ports.go`
- Modify: `internal/biz/pack/importer.go:15-42`

- [ ] **Step 1: 写失败测试（编译验证端口可独立 mock）**

在 `internal/biz/pack/importer_test.go` 末尾新增（文件先 Read 一遍以确认末尾内容）：

```go
// 验证 4 个子端口可独立 mock（编译期断言）。
type fakeTaxonomyImporter struct{ stubImporterRepo }
type fakeAgentImporter struct{ stubImporterRepo }
type fakeTeamImporter struct{ stubImporterRepo }
type fakeGraphImporter struct{ stubImporterRepo }

var (
    _ TaxonomyImporter = (*fakeTaxonomyImporter)(nil)
    _ AgentImporter    = (*fakeAgentImporter)(nil)
    _ TeamImporter     = (*fakeTeamImporter)(nil)
    _ GraphImporter    = (*fakeGraphImporter)(nil)
)
```

- [ ] **Step 2: 运行测试，确认编译失败**

Run: `go test ./internal/biz/pack/ -run TestPortsCompile -v`
Expected: FAIL（"undefined: TaxonomyImporter" 等）

- [ ] **Step 3: 在 `internal/biz/pack/ports.go` 新增 4 个子端口**

```go
package pack

import (
    "context"
    "aranea-agents/internal/biz"
)

// TaxonomyImporter 行业分类导入端口。
type TaxonomyImporter interface {
    CreateTaxonomyNode(ctx context.Context, node biz.TaxonomyNode) (biz.TaxonomyNode, error)
    UpdateTaxonomyNode(ctx context.Context, node biz.TaxonomyNode) (biz.TaxonomyNode, error)
    GetTaxonomyNodeByKey(ctx context.Context, key string) (biz.TaxonomyNode, error)
    GetTaxonomyNodeByKeyAnyState(ctx context.Context, key string) (biz.TaxonomyNode, error)
    ListTaxonomyNodesByParentID(ctx context.Context, parentID string) ([]biz.TaxonomyNode, error)
}

// AgentImporter Agent 导入端口（lookup + 原子化写入）。
type AgentImporter interface {
    GetAgentByAgentKey(ctx context.Context, agentKey string) (biz.Agent, error)
    CreateAgentAtomic(ctx context.Context, agent biz.Agent, files []biz.AgentPromptFile, settings biz.AgentRuntimeSettings) (biz.Agent, error)
    UpdateAgentAtomic(ctx context.Context, agent biz.Agent, files []biz.AgentPromptFile, settings *biz.AgentRuntimeSettings) (biz.Agent, error)
    DeleteAgent(ctx context.Context, id string) error
}

// TeamImporter Team 导入端口。
type TeamImporter interface {
    GetTeamByID(ctx context.Context, id string) (biz.Team, error)
    GetTeamByKey(ctx context.Context, teamKey string) (biz.Team, error)
    CreateTeam(ctx context.Context, t biz.Team) (biz.Team, error)
    UpdateTeam(ctx context.Context, t biz.Team) (biz.Team, error)
}

// GraphImporter Graph 导入端口。
type GraphImporter interface {
    GetGraphDefinitionByName(ctx context.Context, name string) (*biz.GraphDefinition, error)
    SaveGraphDefinition(ctx context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error)
    UpdateGraphDefinition(ctx context.Context, def *biz.GraphDefinition) (*biz.GraphDefinition, error)
}
```

- [ ] **Step 4: 从 `importer.go` 删除原 16-方法 `ImporterRepo` 接口**

删除 `importer.go:15-42` 的 `ImporterRepo` 接口整段。

- [ ] **Step 5: 运行测试，确认接口拆分后整个包编译失败（预期，未实现 Phase 2 之前不算完成）**

Run: `go build ./internal/biz/pack/ 2>&1 | head -5`
Expected: FAIL（`Importer` 仍持有 `ImporterRepo` 字段，类型已删除）

不要在这一步解决，Phase 2 任务里会同时改完 `Importer` 结构 + `NewImporter` 签名 + 所有调用点。

- [ ] **Step 6: Commit**

```bash
git add internal/biz/pack/ports.go internal/biz/pack/importer.go
git commit -m "refactor(pack): split ImporterRepo into 4 sub-ports (Phase 1)"
```

---

### Task 2: 在数据层实现 `ImportAgentAtomic`

**Files:**
- Modify: `internal/data/agent_repo.go:658-700` 区域附近
- Modify: `internal/data/agent_repo.go:773-830` 区域附近

- [ ] **Step 1: 写失败测试（在 `internal/data/agent_repo_test.go` 中）**

```go
func TestAgentRepo_CreateAgentAtomic_rollsBackOnFileFailure(t *testing.T) {
    // 准备：打开 sqlite 内存库 + 迁移
    // 动作：CreateAgentAtomic 传入有效的 agent + 故意触发文件冲突（重名）
    // 断言：(1) 报错；(2) agents 表无该 agent_id；(3) agent_prompt_files 表无遗留
}
```

如果项目没有 `agent_repo_test.go`，新建并只跑这一条用例（不需要 100% 覆盖），或者在 `internal/biz/pack/importer_test.go` 用 stubRepo 模拟 `CreateAgentAtomic` 部分失败来端到端验证（**推荐后者，更快**）。

- [ ] **Step 2: 在 `agent_repo.go` 新增 `ImportAgentAtomic` 内部辅助**

在文件尾部新增：

```go
// CreateAgentAtomic 创建 agent + 写入 prompt files + upsert runtime settings，三步包在同一事务中。
func (r *agentRepo) CreateAgentAtomic(ctx context.Context, a biz.Agent, files []biz.AgentPromptFile, settings biz.AgentRuntimeSettings) (biz.Agent, error) {
    var created biz.Agent
    err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
        var err error
        created, err = r.CreateAgent(txCtx, a)
        if err != nil {
            return fmt.Errorf("create agent: %w", err)
        }
        settings.AgentID = created.ID
        if _, err = r.UpsertAgentRuntimeSettings(txCtx, settings); err != nil {
            return fmt.Errorf("upsert runtime settings: %w", err)
        }
        if len(files) > 0 {
            if _, err = r.ReplaceAgentPromptFiles(txCtx, created.ID, files); err != nil {
                return fmt.Errorf("replace prompt files: %w", err)
            }
        }
        return nil
    })
    return created, err
}

// UpdateAgentAtomic 覆盖 agent + 可选 prompt files + 可选 runtime settings，三步包在同一事务中。
func (r *agentRepo) UpdateAgentAtomic(ctx context.Context, a biz.Agent, files []biz.AgentPromptFile, settings *biz.AgentRuntimeSettings) (biz.Agent, error) {
    var updated biz.Agent
    err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
        var err error
        updated, err = r.UpdateAgent(txCtx, a)
        if err != nil {
            return fmt.Errorf("update agent: %w", err)
        }
        if settings != nil {
            if _, err = r.UpsertAgentRuntimeSettings(txCtx, *settings); err != nil {
                return fmt.Errorf("upsert runtime settings: %w", err)
            }
        }
        if files != nil {
            if _, err = r.ReplaceAgentPromptFiles(txCtx, updated.ID, files); err != nil {
                return fmt.Errorf("replace prompt files: %w", err)
            }
        }
        return nil
    })
    return updated, err
}
```

- [ ] **Step 3: 运行 `go build ./internal/data/...`，确认无编译错误**

Run: `go build ./internal/data/... 2>&1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/data/agent_repo.go
git commit -m "feat(data): add ImportAgentAtomic with ExecInTx wrapping"
```

---

### Task 3: 重构 `Importer` 注入 4 个子端口

**Files:**
- Modify: `internal/biz/pack/importer.go:58-65` (`Importer` 结构 + `NewImporter`)
- Modify: `internal/data/pack_repo.go`（实现新端口）
- Modify: `internal/data/seed_pack.go:156`（调用新构造函数）
- Modify: `internal/service/pack.go:27`（调用新构造函数）

- [ ] **Step 1: 写失败测试（验证 `NewImporter` 接受 4 个端口）**

在 `importer_test.go` 顶部 stub 拆分后追加：

```go
// TestNewImporter_AcceptsSubPorts 编译期断言 NewImporter 接受 4 个独立端口。
func TestNewImporter_AcceptsSubPorts(t *testing.T) {
    tax := &fakeTaxonomyImporter{stubImporterRepo: *newStubImporterRepo()}
    ag := &fakeAgentImporter{stubImporterRepo: *newStubImporterRepo()}
    tm := &fakeTeamImporter{stubImporterRepo: *newStubImporterRepo()}
    gr := &fakeGraphImporter{stubImporterRepo: *newStubImporterRepo()}
    im := NewImporter(tax, ag, tm, gr)
    if im == nil {
        t.Fatal("NewImporter returned nil")
    }
}
```

为 fake 实现各自子端口的最小方法（`fakeAgentImporter` 也要 stub 6 个方法等），或直接在 stub 上提供全部方法、用 type embedding 共享。

- [ ] **Step 2: 修改 `Importer` 结构和构造函数**

```go
type Importer struct {
    tax   TaxonomyImporter
    agent AgentImporter
    team  TeamImporter
    graph GraphImporter
}

func NewImporter(tax TaxonomyImporter, agent AgentImporter, team TeamImporter, graph GraphImporter) *Importer {
    return &Importer{tax: tax, agent: agent, team: team, graph: graph}
}
```

- [ ] **Step 3: 把 `importAgent` 内部所有 `im.repo.X` 调用替换为 `im.agent.X` / `im.tax.X` 等**

| 旧调用 | 新调用 |
|--------|--------|
| `im.repo.GetAgentByAgentKey` | `im.agent.GetAgentByAgentKey` |
| `im.repo.CreateAgent` | 删除（改用 `im.agent.CreateAgentAtomic`） |
| `im.repo.UpdateAgent` | 删除（改用 `im.agent.UpdateAgentAtomic`） |
| `im.repo.DeleteAgent` | `im.agent.DeleteAgent` |
| `im.repo.ReplaceAgentPromptFiles` | 包含在 atomic 调用内 |
| `im.repo.UpsertAgentRuntimeSettings` | 包含在 atomic 调用内 |
| `im.repo.GetTaxonomyNodeByKey` | `im.tax.GetTaxonomyNodeByKey` |
| `im.repo.GetTaxonomyNodeByKeyAnyState` | `im.tax.GetTaxonomyNodeByKeyAnyState` |
| `im.repo.CreateTaxonomyNode` | `im.tax.CreateTaxonomyNode` |
| `im.repo.UpdateTaxonomyNode` | `im.tax.UpdateTaxonomyNode` |
| `im.repo.GetTeamByKey` | `im.team.GetTeamByKey` |
| `im.repo.CreateTeam` | `im.team.CreateTeam` |
| `im.repo.UpdateTeam` | `im.team.UpdateTeam` |
| `im.repo.GetGraphDefinitionByName` | `im.graph.GetGraphDefinitionByName` |
| `im.repo.SaveGraphDefinition` | `im.graph.SaveGraphDefinition` |
| `im.repo.UpdateGraphDefinition` | `im.graph.UpdateGraphDefinition` |

- [ ] **Step 4: 让 `PackRepoAdapter` 实现 4 个子端口**

修改 `internal/data/pack_repo.go`：

```go
var (
    _ pack.TaxonomyImporter = (*PackRepoAdapter)(nil)
    _ pack.AgentImporter    = (*PackRepoAdapter)(nil)
    _ pack.TeamImporter     = (*PackRepoAdapter)(nil)
    _ pack.GraphImporter    = (*PackRepoAdapter)(nil)
)

// 把原 16 个 ImporterRepo 方法按归属归到 4 个分组（用 method set 表达），并新增
// CreateAgentAtomic / UpdateAgentAtomic 委托到 agents 字段。
func (a *PackRepoAdapter) CreateAgentAtomic(ctx context.Context, ag biz.Agent, files []biz.AgentPromptFile, s biz.AgentRuntimeSettings) (biz.Agent, error) {
    if ir, ok := a.agents.(interface {
        CreateAgentAtomic(context.Context, biz.Agent, []biz.AgentPromptFile, biz.AgentRuntimeSettings) (biz.Agent, error)
    }); ok {
        return ir.CreateAgentAtomic(ctx, ag, files, s)
    }
    return biz.Agent{}, fmt.Errorf("agent repo does not support atomic create")
}
// UpdateAgentAtomic 同理
```

- [ ] **Step 5: 更新 3 个调用点**

`internal/data/seed_pack.go:156` 改为：

```go
return pack.NewImporter(adapter, adapter, adapter, adapter)
```

`internal/service/pack.go:27` 同样。

- [ ] **Step 6: 运行 build + test**

Run: `go build ./... 2>&1`
Run: `go test ./internal/biz/pack/ -count=1 2>&1`
Expected: 全部 PASS

- [ ] **Step 7: Commit**

```bash
git add internal/biz/pack/importer.go internal/data/pack_repo.go internal/data/seed_pack.go internal/service/pack.go
git commit -m "refactor(pack): wire Importer with 4 sub-port composition"
```

---

## Phase 2: 原子化 Agent 导入（阻断项 #1）

### Task 4: 简化 `importAgent` 写入路径

**Files:**
- Modify: `internal/biz/pack/importer.go:300-372`

- [ ] **Step 1: 写失败测试（PromptFiles 失败回滚）**

在 `importer_test.go` 新增：

```go
// stubAgentImporter 让 ReplaceAgentPromptFiles 失败，验证 CreateAgentAtomic
// 整体回滚（agent 不入库）。
type stubAgentImporterAtomic struct{ *stubImporterRepo }
func (s *stubAgentImporterAtomic) CreateAgentAtomic(_ context.Context, a biz.Agent, _ []biz.AgentPromptFile, _ biz.AgentRuntimeSettings) (biz.Agent, error) {
    return biz.Agent{}, errors.New("simulated atomic failure")
}
func (s *stubAgentImporterAtomic) UpdateAgentAtomic(_ context.Context, a biz.Agent, _ []biz.AgentPromptFile, _ *biz.AgentRuntimeSettings) (biz.Agent, error) {
    return biz.Agent{}, errors.New("simulated atomic failure")
}

func TestImport_AgentPromptFilesFailure_RollsBackAgent(t *testing.T) {
    // Arrange: stubAgentImporterAtomic 包住 stub，让 atomic 失败
    // Act: Import 一个带文件的 agent
    // Assert: 报 kerrors.BadRequest；stub agents map 中无该 agent
}
```

- [ ] **Step 2: 在 `importAgent` 中把 `CreateAgent + Upsert + ReplaceFiles` 合并**

```go
// 替换 importer.go:300-318 块
files := im.buildPromptFiles(spec.Key, agentFiles)
settings := im.buildRuntimeSettings("", spec) // agentID 在 atomic 内填

if findErr == nil && strategy == ConflictOverwrite {
    agent.ID = existing.ID
    updatedAgent, err := im.agent.UpdateAgentAtomic(ctx, agent, files, &settings)
    if err != nil {
        return 0, 0, 0, warns, kerrors.BadRequest("PACK_AGENT_UPDATE",
            fmt.Sprintf("更新 Agent %s 失败: %s", spec.Key, err.Error()))
    }
    agentID = updatedAgent.ID
    updated = 1
} else {
    createdAgent, err := im.agent.CreateAgentAtomic(ctx, agent, files, settings)
    if err != nil {
        return 0, 0, 0, warns, kerrors.BadRequest("PACK_AGENT_CREATE",
            fmt.Sprintf("创建 Agent %s 失败: %s", spec.Key, err.Error()))
    }
    agentID = createdAgent.ID
    created = 1
}
```

- [ ] **Step 3: 删除原 `importAgent` 中独立的 `ReplaceAgentPromptFiles` 和 `UpsertAgentRuntimeSettings` 段**

删除 importer.go:327-372 整段（含旧的 try-rollback 逻辑）。

- [ ] **Step 4: 运行 build + test**

Run: `go build ./... 2>&1`
Run: `go test ./internal/biz/pack/ -run TestImport_AgentPromptFilesFailure_RollsBackAgent -v 2>&1`
Expected: PASS

- [ ] **Step 5: 运行完整包测试**

Run: `go test ./internal/biz/pack/ -count=1 2>&1`
Expected: PASS（无回归）

- [ ] **Step 6: Commit**

```bash
git add internal/biz/pack/importer.go internal/biz/pack/importer_test.go
git commit -m "fix(pack): use ImportAgentAtomic for create+update paths"
```

---

### Task 5: 拆 `importAgent` 为小函数

**Files:**
- Modify: `internal/biz/pack/importer.go:204-374`

- [ ] **Step 1: 写测试（验证拆分后子函数被正确调用）**

```go
func TestImport_AgentConflictSkip(t *testing.T) {
    // 预存同名 agent
    // 用 ConflictSkip
    // 断言：不调用 CreateAgentAtomic；returns created=0,updated=0,skipped=1
}
```

- [ ] **Step 2: 拆出 `resolveAgentConflict`**

```go
// resolveAgentConflict 根据 strategy 决定：跳过 / 覆盖 / 复制 / 新建。
// 返回 (finalKey, existingAgent, action, err)。
type conflictAction int
const (
    conflictCreate conflictAction = iota
    conflictSkip
    conflictOverwrite
    conflictDuplicate
)

func (im *Importer) resolveAgentConflict(ctx context.Context, spec AgentPackSpec, strategy ConflictStrategy) (string, *biz.Agent, conflictAction, error) {
    existing, err := im.agent.GetAgentByAgentKey(ctx, spec.Key)
    if err != nil { // NotFound 视为新建
        return spec.Key, nil, conflictCreate, nil
    }
    switch strategy {
    case ConflictSkip:
        return spec.Key, &existing, conflictSkip, nil
    case ConflictOverwrite:
        return spec.Key, &existing, conflictOverwrite, nil
    case ConflictDuplicate:
        newKey := spec.Key + "-copy"
        for i := 2; ; i++ {
            _, e := im.agent.GetAgentByAgentKey(ctx, newKey)
            if e != nil { break }
            newKey = fmt.Sprintf("%s-copy-%d", spec.Key, i)
        }
        return newKey, &existing, conflictDuplicate, nil
    }
    return spec.Key, nil, conflictCreate, nil
}
```

- [ ] **Step 3: 拆出 `buildAgentFromSpec`**

把 `importAgent` 行 234-298 抽到 `buildAgentFromSpec(spec AgentPackSpec, finalKey string, cfg *importConfig, positionID string, source string) biz.Agent`。

- [ ] **Step 4: 拆出 `buildPromptFiles`**

把行 327-343 抽到 `buildPromptFiles(specKey string, agentFiles map[string]map[string]string) []biz.AgentPromptFile`。

- [ ] **Step 5: 重写 `importAgent` 主体为 ~50 行的 orchestrator**

```go
func (im *Importer) importAgent(ctx context.Context, spec AgentPackSpec, agentFiles map[string]map[string]string, strategy ConflictStrategy, mapper *KeyMapper, cfg *importConfig) (created, updated, skipped int, warns []string, err error) {
    originalKey := spec.Key
    finalKey, existing, action, _ := im.resolveAgentConflict(ctx, spec, strategy)
    if action == conflictSkip {
        mapper.RegisterAgent(spec.Key, existing.ID)
        return 0, 0, 1, nil, nil
    }
    spec.Key = finalKey
    posID, posKey := im.resolvePositionKey(spec.PositionKey)
    if posKey == "" && spec.PositionKey != "" {
        warns = append(warns, fmt.Sprintf("agent %q: PositionKey %q 解析失败", spec.Key, spec.PositionKey))
    }
    agent := im.buildAgentFromSpec(spec, cfg, posID, posKey, existing, action == conflictOverwrite)
    files := im.buildPromptFiles(spec.Key, agentFiles)
    settings := im.buildRuntimeSettings(spec)
    var agentID string
    switch action {
    case conflictOverwrite:
        agent.ID = existing.ID
        u, e := im.agent.UpdateAgentAtomic(ctx, agent, files, &settings)
        if e != nil { return 0,0,0,warns,kerrors.BadRequest("PACK_AGENT_UPDATE", errMsg(e, spec.Key)) }
        agentID, updated = u.ID, 1
    default: // create + duplicate
        c, e := im.agent.CreateAgentAtomic(ctx, agent, files, settings)
        if e != nil { return 0,0,0,warns,kerrors.BadRequest("PACK_AGENT_CREATE", errMsg(e, spec.Key)) }
        agentID, created = c.ID, 1
    }
    mapper.RegisterAgent(spec.Key, agentID)
    if action == conflictDuplicate && spec.Key != originalKey {
        mapper.RegisterAgent(originalKey, agentID)
    }
    return
}
```

- [ ] **Step 6: 跑全量测试**

Run: `go test ./internal/biz/pack/ -count=1 -v 2>&1`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/biz/pack/importer.go
git commit -m "refactor(pack): decompose importAgent into resolver+builder+orchestrator"
```

---

## Phase 3: 必填引用硬失败（建议 #3 Tier 1）

### Task 6: Team Members 引用硬失败

**Files:**
- Modify: `internal/biz/pack/importer.go:560-577`

- [ ] **Step 1: 写测试（missing agent_key 应当硬失败）**

```go
func TestImport_TeamMemberMissingAgent_Fails(t *testing.T) {
    p := &Pack{
        Agents: []AgentPackSpec{{Key: "a1", DisplayName: "A1"}},
        Teams:  []TeamPackSpec{{Key: "t1", Mode: "coordinator", Members: []MemberPackSpec{{AgentKey: "ghost"}}}},
    }
    _, err := im.Import(ctx, p, ConflictOverwrite)
    if err == nil || !strings.Contains(err.Error(), "ghost") {
        t.Fatalf("expected missing agent key error, got %v", err)
    }
}
```

- [ ] **Step 2: 替换 `if err == nil` 为硬失败**

```go
// importer.go:560-577
for i, m := range spec.Members {
    agentID, err := mapper.ResolveAgentKey(m.AgentKey)
    if err != nil {
        return 0, 0, 0, warns, kerrors.BadRequest("PACK_TEAM_MEMBER",
            fmt.Sprintf("Team %s 第 %d 个成员 agent_key=%q 未找到: %s",
                spec.Key, i+1, m.AgentKey, err.Error()))
    }
    ospec.Members = append(ospec.Members, biz.OrchestrationMember{...})
}
```

- [ ] **Step 3: 跑测试**

Run: `go test ./internal/biz/pack/ -run TestImport_TeamMemberMissingAgent_Fails -v 2>&1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/biz/pack/importer.go internal/biz/pack/importer_test.go
git commit -m "fix(pack): fail hard on missing team member agent_key"
```

---

### Task 7: IntentAnchor / Synthesizer 引用硬失败

**Files:**
- Modify: `internal/biz/pack/importer.go:580-591`

- [ ] **Step 1: 写测试**

```go
func TestImport_TeamIntentAnchorMissing_Fails(t *testing.T) {
    p := &Pack{Teams: []TeamPackSpec{{Key: "t1", Mode: "coordinator",
        IntentAnchorKey: "ghost-anchor"}}}
    _, err := im.Import(ctx, p, ConflictOverwrite)
    if err == nil { t.Fatal("expected error") }
}
```

- [ ] **Step 2: 替换为硬失败**

```go
if spec.IntentAnchorKey != "" {
    id, err := mapper.ResolveAgentKey(spec.IntentAnchorKey)
    if err != nil {
        return 0,0,0,warns, kerrors.BadRequest("PACK_TEAM_INTENT_ANCHOR",
            fmt.Sprintf("Team %s IntentAnchorKey=%q 未找到: %s", spec.Key, spec.IntentAnchorKey, err.Error()))
    }
    ospec.IntentAnchorAgentID = id
}
if spec.SynthesizerKey != "" { /* 同上错误码 PACK_TEAM_SYNTHESIZER */ }
```

- [ ] **Step 3: 跑测试 + 全量**

Run: `go test ./internal/biz/pack/ -count=1 2>&1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/biz/pack/importer.go internal/biz/pack/importer_test.go
git commit -m "fix(pack): fail hard on missing IntentAnchor/Synthesizer"
```

---

### Task 8: TeamGraph.Nodes.AgentKey 硬失败

**Files:**
- Modify: `internal/biz/pack/importer.go:730-735`（在 `buildEmbeddedGraph` 中）

- [ ] **Step 1: 写测试**

```go
func TestImport_TeamGraphNodeMissingAgent_Fails(t *testing.T) {
    p := &Pack{Teams: []TeamPackSpec{{Key: "t1", Mode: "graph",
        Graph: &TeamGraphPackSpec{
            Nodes: []GraphNodePackSpec{{ID: "n1", AgentKey: "ghost"}},
        }}}}
    _, err := im.Import(ctx, p, ConflictOverwrite)
    if err == nil { t.Fatal("expected error") }
}
```

- [ ] **Step 2: `buildEmbeddedGraph` 改为返回 error**

```go
func (im *Importer) buildEmbeddedGraph(spec *TeamGraphPackSpec, mapper *KeyMapper, teamKey string) (*biz.EmbeddedGraphSpec, error) {
    eg := &biz.EmbeddedGraphSpec{Version: 1, Layout: spec.Layout}
    for _, n := range spec.Nodes {
        if n.AgentKey == "" {
            continue
        }
        id, err := mapper.ResolveAgentKey(n.AgentKey)
        if err != nil {
            return nil, kerrors.BadRequest("PACK_TEAM_GRAPH_NODE",
                fmt.Sprintf("Team %s 节点 %s agent_key=%q 未找到: %s",
                    teamKey, n.ID, n.AgentKey, err.Error()))
        }
        eg.Nodes = append(eg.Nodes, biz.EmbeddedGraphNodeSpec{
            ID: n.ID, AgentID: id, ...
        })
    }
    // edges
    return eg, nil
}
```

调用方（`importTeam` 中）改为 `eg, err := im.buildEmbeddedGraph(spec.Graph, mapper, spec.Key)` 并 unwrap error。

- [ ] **Step 3: 跑测试 + 全量**

Run: `go test ./internal/biz/pack/ -count=1 2>&1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/biz/pack/importer.go internal/biz/pack/importer_test.go
git commit -m "fix(pack): fail hard on missing TeamGraph node agent_key"
```

---

## Phase 4: 可选引用软失败 + 警告（建议 #3 Tier 2）

### Task 9: LinkedGraphID / PositionKey 软失败 + warning

**Files:**
- Modify: `internal/biz/pack/importer.go:594-599` (LinkedGraphID)
- Modify: `internal/biz/pack/importer.go:285` (PositionKey 解析)

- [ ] **Step 1: 写测试（warning 应出现在 result.Warnings）**

```go
func TestImport_TeamLinkedGraphMissing_Warns(t *testing.T) {
    p := &Pack{
        Agents: []AgentPackSpec{{Key: "a1"}},
        Teams: []TeamPackSpec{{Key: "t1", Mode: "graph",
            Graph: &TeamGraphPackSpec{Linked: true, LinkedGraphID: "ghost"}}},
    }
    res, err := im.Import(ctx, p, ConflictOverwrite)
    if err != nil { t.Fatal(err) }
    if len(res.Warnings) == 0 { t.Fatal("expected warning") }
}
```

- [ ] **Step 2: 替换为软失败**

```go
// LinkedGraphID
if spec.Graph.Linked && spec.Graph.LinkedGraphID != "" {
    if newID, ok := mapper.GraphID(spec.Graph.LinkedGraphID); ok {
        ospec.LinkedGraphID = newID
    } else {
        warns = append(warns, fmt.Sprintf("Team %s LinkedGraphID=%q 在本次 import 中未生成，已忽略", spec.Key, spec.Graph.LinkedGraphID))
    }
}
```

PositionKey 解析：在 `resolvePositionKey`（Phase 2 拆分出的）里，若 `ParseTaxonomyKeyPath` 失败，返回空 posID/posKey，但 caller 已记录 warning。

- [ ] **Step 3: 跑测试 + 全量**

Run: `go test ./internal/biz/pack/ -count=1 2>&1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/biz/pack/importer.go internal/biz/pack/importer_test.go
git commit -m "fix(pack): warn (not fail) on missing optional references"
```

---

## Phase 5: ConflictSkip 注册 Team（建议 #5）

### Task 10: 给 `KeyMapper` 加 team 维度 + 修补 ConflictSkip

**Files:**
- Modify: `internal/biz/pack/mapper.go:40-86`
- Modify: `internal/biz/pack/importer.go:680-682`

- [ ] **Step 1: 写测试**

```go
func TestKeyMapper_RegisterTeam(t *testing.T) {
    m := NewKeyMapper()
    m.RegisterTeam("t1", "team-id-1")
    id, ok := m.ResolveTeamKey("t1")
    if !ok || id != "team-id-1" { t.Fatal("register/resolve roundtrip failed") }
    if _, ok := m.ResolveTeamKey("missing"); ok { t.Fatal("expected not found") }
}
```

- [ ] **Step 2: 在 `mapper.go` 加 `teamKeyToID` 字段和方法**

```go
type KeyMapper struct {
    mu             sync.RWMutex
    agentKeyToID   map[string]string
    teamKeyToID    map[string]string  // 新增
    taxonomyKeyToID map[string]string
    graphIDMap     map[string]string
}

func (m *KeyMapper) RegisterTeam(k, id string) {
    m.mu.Lock(); defer m.mu.Unlock()
    m.teamKeyToID[k] = id
}
func (m *KeyMapper) ResolveTeamKey(k string) (string, bool) {
    m.mu.RLock(); defer m.mu.RUnlock()
    id, ok := m.teamKeyToID[k]; return id, ok
}
```

`NewKeyMapper` 初始化 `teamKeyToID`。

- [ ] **Step 3: 在 `importTeam` 的 `ConflictSkip` 路径补注册**

```go
case ConflictSkip:
    mapper.RegisterTeam(spec.Key, existing.ID)
    return 0, 0, 1, warns, nil
```

- [ ] **Step 4: 跑测试 + 全量**

Run: `go test ./internal/biz/pack/ -count=1 2>&1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/biz/pack/mapper.go internal/biz/pack/importer.go internal/biz/pack/importer_test.go
git commit -m "feat(pack): register team in KeyMapper on ConflictSkip"
```

---

## Phase 6: KeyMapper 并发安全（建议 #7）

### Task 11: `KeyMapper` 全面加锁

**Files:**
- Modify: `internal/biz/pack/mapper.go`

- [ ] **Step 1: 写并发测试**

```go
func TestKeyMapper_ConcurrentAccessSafe(t *testing.T) {
    m := NewKeyMapper()
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(2)
        go func(i int) { defer wg.Done(); m.RegisterAgent(fmt.Sprintf("k%d", i), fmt.Sprintf("id%d", i)) }(i)
        go func(i int) { defer wg.Done(); _, _ = m.AgentID(fmt.Sprintf("k%d", i)) }(i)
    }
    wg.Wait()
}
```

Run: `go test ./internal/biz/pack/ -race -run TestKeyMapper_ConcurrentAccessSafe -v 2>&1`
Expected: PASS（-race 不报数据竞争）

- [ ] **Step 2: 给所有 6 个方法加锁**

`RegisterAgent/AgentID/RegisterTaxonomy/TaxonomyID/RegisterGraph/GraphID/ResolveAgentKey/ResolvePositionKey/RegisterTeam/ResolveTeamKey`（Phase 5 新增）全部走 `m.mu.Lock()/m.mu.RLock()`。

- [ ] **Step 3: 跑全量 + -race**

Run: `go test ./internal/biz/pack/ -race -count=1 2>&1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/biz/pack/mapper.go
git commit -m "chore(pack): add sync.RWMutex to KeyMapper for concurrent safety"
```

---

## Phase 7: 拆 `importTeam`（建议 #8）

### Task 12: 把 `importTeam` 拆为 orchestrator + builder

**Files:**
- Modify: `internal/biz/pack/importer.go:537-712`

- [ ] **Step 1: 写测试（行为不变，只验证拆分后无回归）**

不需要新测试 — 现有 `TestImport_DefaultKind` / `TestImport_WithKindOverride` 覆盖。

- [ ] **Step 2: 拆出 `buildOrchestrationSpec`**

行 540-641 抽到 `buildOrchestrationSpec(ctx, spec, mapper, teamKey) (biz.OrchestrationSpec, []string, error)`。

- [ ] **Step 3: 拆出 `resolveTeamConflict`**

与 `resolveAgentConflict` 对称。

- [ ] **Step 4: 重写 `importTeam` 主体为 ~50 行的 orchestrator**

模式同 `importAgent`（Phase 2 Task 5）。

- [ ] **Step 5: 跑全量测试**

Run: `go test ./internal/biz/pack/ -count=1 2>&1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/biz/pack/importer.go
git commit -m "refactor(pack): decompose importTeam into resolver+builder+orchestrator"
```

---

## Phase 8: `RecoveryTimeoutMs` 改 `RecoveryTimeoutSec`（建议 #6）

### Task 13: 重命名 yaml 字段

**Files:**
- Modify: `internal/biz/pack/spec.go`（`CircuitBreakerSpec` 字段）
- Modify: `internal/biz/pack/convert.go`（读取字段）
- Modify: `internal/biz/pack/exporter.go`（写入字段）
- Modify: `internal/biz/pack/importer.go:628`（删除 `/1000`）

- [ ] **Step 1: 写测试（验证 spec 字段名）**

```go
func TestCircuitBreakerSpec_RecoveryTimeoutSec(t *testing.T) {
    yamlData := []byte("circuit_breaker:\n  failure_threshold: 5\n  recovery_timeout_sec: 30\n")
    var spec TeamFailurePolicyPackSpec
    if err := yaml.Unmarshal(yamlData, &spec); err != nil { t.Fatal(err) }
    if spec.CircuitBreaker.RecoveryTimeoutSec != 30 { t.Fatal("parse failed") }
}
```

- [ ] **Step 2: 改字段名**

```go
// spec.go
type CircuitBreakerPackSpec struct {
    FailureThreshold     int `yaml:"failure_threshold"`
    RecoveryTimeoutSec   int `yaml:"recovery_timeout_sec"`  // 原 RecoveryTimeoutMs
}
```

- [ ] **Step 3: 同步 importer.go:628**

```go
fp.CircuitBreaker = &biz.CircuitBreakerPolicy{
    FailureThreshold:    spec.FailurePolicy.CircuitBreaker.FailureThreshold,
    ResetTimeoutSeconds: spec.FailurePolicy.CircuitBreaker.RecoveryTimeoutSec,
}
```

- [ ] **Step 4: 同步 exporter.go + convert.go**

- [ ] **Step 5: 跑测试**

Run: `go test ./internal/biz/pack/ -count=1 2>&1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/biz/pack/spec.go internal/biz/pack/convert.go internal/biz/pack/exporter.go internal/biz/pack/importer.go internal/biz/pack/importer_test.go
git commit -m "refactor(pack): rename CircuitBreaker.RecoveryTimeoutMs to RecoveryTimeoutSec"
```

---

## Phase 9: Graph ConflictDuplicate 显式 warn（建议 #4）

### Task 14: Graph duplicate 显式降级 + warning

**Files:**
- Modify: `internal/biz/pack/importer.go:388-392`

- [ ] **Step 1: 写测试（warning 出现 + 走 overwrite）**

```go
func TestImport_GraphDuplicateStrategy_EmitsWarning(t *testing.T) {
    repo := newStubImporterRepo()
    repo.graphs["g1"] = &biz.GraphDefinition{Name: "g1", ID: "existing-id"}
    im := NewImporter(...)
    p := &Pack{Graphs: []GraphPackSpec{{ID: "g1", Name: "g1"}}}
    res, err := im.Import(ctx, p, ConflictDuplicate)
    if err != nil { t.Fatal(err) }
    if len(res.Warnings) == 0 { t.Fatal("expected warning for graph duplicate") }
    if res.GraphsUpdated != 1 { t.Fatal("expected overwrite path") }
}
```

`importGraph` 当前签名只返回 `(created, updated, skipped, err)` — 需要扩展为返回 warns：

```go
func (im *Importer) importGraph(ctx, spec, strategy, mapper) (created, updated, skipped int, warns []string, err error)
```

`Import()` 主体在 `Phase 4: Graphs` 累加 warns：

```go
result.GraphsCreated += created
result.GraphsUpdated += updated
result.GraphsSkipped += skipped
result.Warnings = append(result.Warnings, warns...)
```

- [ ] **Step 2: 在 `importGraph` 加 `case ConflictDuplicate` 显式降级**

```go
case ConflictDuplicate:
    // Graph 无独立 key 机制；按 overwrite 处理并 warning
    // （warning 通过包级 warns 切片传出）
```

- [ ] **Step 3: 跑测试 + 全量**

Run: `go test ./internal/biz/pack/ -count=1 2>&1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/biz/pack/importer.go internal/biz/pack/importer_test.go
git commit -m "fix(pack): emit warning when Graph ConflictDuplicate is downgraded to overwrite"
```

---

## Phase 10: OrchestrationSpec.Version 推导（建议 #9, #10）

### Task 15: 从 `Pack.Manifest.APIVersion` 推导 Version

**Files:**
- Modify: `internal/biz/pack/spec.go`（`Pack` 加可选 `OrchestrationVersion int`）
- Modify: `internal/biz/pack/importer.go:541, 716`

- [ ] **Step 1: 写测试**

```go
func TestImport_OrchestrationVersionFromManifest(t *testing.T) {
    p := &Pack{
        Manifest: ManifestSpec{APIVersion: "v1"},
        Agents: []AgentPackSpec{{Key: "a1"}},
        Teams: []TeamPackSpec{{Key: "t1", Mode: "coordinator"}},
    }
    res, _ := im.Import(ctx, p, ConflictOverwrite)
    if repo.teams["t1"].DefinitionJSON == "" { t.Fatal(...) }
    // 断言 definition_json 中的 version 字段为 1（Manifest v1 对应 v1）
}
```

需要先确定映射规则：`v1 → 1, v2 → 2`，可读 `Pack.Manifest.APIVersion`，否则 fallback 2。

- [ ] **Step 2: 改 `importTeam` 和 `buildEmbeddedGraph`**

```go
// importTeam
func (im *Importer) orchestrationVersion(p *Pack) int {
    if p.Manifest.APIVersion == "v1" { return 1 }
    return 2 // 默认
}

ospec := biz.OrchestrationSpec{Version: im.orchestrationVersion(p), ...}

// buildEmbeddedGraph
eg := &biz.EmbeddedGraphSpec{Version: im.orchestrationVersion(p), ...}
```

- [ ] **Step 3: 跑测试 + 全量**

Run: `go test ./internal/biz/pack/ -count=1 2>&1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/biz/pack/importer.go internal/biz/pack/importer_test.go
git commit -m "feat(pack): derive OrchestrationSpec/EmbeddedGraph version from Pack.Manifest"
```

---

## Phase 11: Subgraph Interrupt 字段补齐（建议 #11）

### Task 16: `SubgraphPackSpec` 加 `InterruptBefore/After`

**Files:**
- Modify: `internal/biz/pack/spec.go:426-433`
- Modify: `internal/biz/pack/importer.go:489-494`（在 `buildGraphDefinition` 中读取）
- Modify: `internal/biz/pack/exporter.go`（导出时写入）

- [ ] **Step 1: 写测试（yaml 解析 + 透传）**

```go
func TestSubgraphPackSpec_InterruptFields(t *testing.T) {
    yamlData := []byte("id: s1\ninterrupt_before: true\ninterrupt_after: true\n")
    var sg SubgraphPackSpec
    yaml.Unmarshal(yamlData, &sg)
    if !sg.InterruptBefore || !sg.InterruptAfter { t.Fatal(...) }
}
```

- [ ] **Step 2: 加字段 + 透传**

```go
type SubgraphPackSpec struct {
    ID              string              `yaml:"id"`
    Name            string              `yaml:"name,omitempty"`
    EntryPoint      string              `yaml:"entry_point"`
    FinishPoint     string              `yaml:"finish_point,omitempty"`
    InterruptBefore bool                `yaml:"interrupt_before,omitempty"`
    InterruptAfter  bool                `yaml:"interrupt_after,omitempty"`
    Nodes           []GraphNodePackSpec `yaml:"nodes,omitempty"`
    Edges           []GraphEdgePackSpec `yaml:"edges,omitempty"`
}
```

`buildGraphDefinition`（importer.go:489-494）改为读 `sg.InterruptBefore / sg.InterruptAfter`。

- [ ] **Step 3: 跑测试**

Run: `go test ./internal/biz/pack/ -count=1 2>&1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/biz/pack/spec.go internal/biz/pack/importer.go internal/biz/pack/exporter.go
git commit -m "feat(pack): expose SubgraphDef.InterruptBefore/After in spec"
```

---

## Phase 12: `sliceToJSONList` 返回 error（建议 #12）

### Task 17: 错误不静默

**Files:**
- Modify: `internal/biz/pack/importer.go:898-907`
- Modify: `internal/biz/pack/importer.go:797-804`（调用方）

- [ ] **Step 1: 写测试**

```go
func TestSliceToJSONList_PropagatesError(t *testing.T) {
    // 无法直接触发 json.Marshal 失败（[]string 不会失败），
    // 改测调用方 buildRuntimeSettings 中传入 nil 的情况
}
```

`json.Marshal([]string)` 几乎不会失败，但合同上应当返回 error。也可以在 helper 改为接受 `[]string` 并返回 `(string, error)`，调用方按需丢弃 error（带 warning）。

- [ ] **Step 2: 改签名 + 调用方**

```go
func sliceToJSONList(items []string) (string, error) {
    if len(items) == 0 { return "[]", nil }
    b, err := json.Marshal(items)
    if err != nil { return "[]", err }
    return string(b), nil
}
```

调用方 `buildRuntimeSettings`：

```go
if spec.ToolsAllow != nil {
    s, err := sliceToJSONList(spec.ToolsAllow)
    if err != nil {
        // 此 helper 对 []string 不会失败；但 contract 已升级
        // 记录 warn 而非硬失败
    }
    s.ToolsAllowJSON = s
}
```

更安全方案：保持返回 `string`，但同时返回 `error`，调用方按需 warn。这是非破坏性升级。

- [ ] **Step 3: 跑测试**

Run: `go test ./internal/biz/pack/ -count=1 2>&1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/biz/pack/importer.go
git commit -m "refactor(pack): make sliceToJSONList return error for explicit error handling"
```

---

## Phase 13: Mapper 回滚清理（建议 #14）

### Task 18: `UnregisterAgent` + 在 atomic 失败时清理 mapper

**Files:**
- Modify: `internal/biz/pack/mapper.go`
- Modify: `internal/biz/pack/importer.go`（atomic 失败时调用）

- [ ] **Step 1: 写测试**

```go
func TestKeyMapper_UnregisterAgent(t *testing.T) {
    m := NewKeyMapper()
    m.RegisterAgent("k", "id")
    m.UnregisterAgent("k")
    if _, ok := m.AgentID("k"); ok { t.Fatal("expected removed") }
}
```

- [ ] **Step 2: 在 KeyMapper 加 `UnregisterAgent(key string)`**

- [ ] **Step 3: 在 `importAgent` 失败路径加 `mapper.UnregisterAgent(spec.Key)`**

注意 Phase 2 的 atomic 调用失败时 — `originalKey` 和 `spec.Key` 都需要清理（duplicate 情况下两个都注册了）。

- [ ] **Step 4: 跑测试**

Run: `go test ./internal/biz/pack/ -count=1 2>&1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/biz/pack/mapper.go internal/biz/pack/importer.go
git commit -m "fix(pack): unregister agent from mapper on atomic failure"
```

---

## Phase 14: Taxonomy duplicate 策略语义明确化（建议 #13）

### Task 19: 入口处校验 + 警告

**Files:**
- Modify: `internal/biz/pack/importer.go:69-72`（Import 入口）

- [ ] **Step 1: 写测试**

```go
func TestImport_TaxonomyDuplicateStrategy_Warns(t *testing.T) {
    p := &Pack{Taxonomy: &TaxonomyPackSpec{Industries: []IndustrySpec{{Key: "ind1", Name: "Ind"}}}}
    res, _ := im.Import(ctx, p, ConflictDuplicate)
    if len(res.Warnings) == 0 { t.Fatal("expected warning") }
}
```

- [ ] **Step 2: 在 `Import` 入口加守卫**

```go
if p.Taxonomy != nil && strategy == ConflictDuplicate {
    result.Warnings = append(result.Warnings,
        "taxonomy 不支持 duplicate 策略，重复 key 将被忽略并使用现有节点")
}
```

- [ ] **Step 3: 跑测试**

Run: `go test ./internal/biz/pack/ -count=1 2>&1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/biz/pack/importer.go internal/biz/pack/importer_test.go
git commit -m "fix(pack): emit upfront warning for taxonomy+ConflictDuplicate combo"
```

---

## 验证清单（每个 Phase 完成后跑一遍）

- [ ] `go build ./...` 0 错误
- [ ] `go test ./internal/biz/pack/ -count=1` 全 PASS
- [ ] `go test ./internal/data/ -count=1 -race` 全 PASS（验证事务不会泄漏）
- [ ] `go vet ./internal/biz/pack/` 0 警告
- [ ] 若有新增端口：`grep -rn "ImporterRepo" internal/biz/pack/` 应为 0 hit
- [ ] 若改了 spec：`go test ./internal/biz/pack/... -run "TestPack|TestSpec|TestExport" -v` 全 PASS

---

## 总 commit 计划

| Phase | 任务 | Commit 数 | 风险 |
|-------|------|-----------|------|
| 1 | Task 1-3 | 3 | 中（接口重构） |
| 2 | Task 4-5 | 2 | 中（事务重构） |
| 3 | Task 6-8 | 3 | 低（行为变化） |
| 4 | Task 9 | 1 | 低 |
| 5 | Task 10 | 1 | 低 |
| 6 | Task 11 | 1 | 低 |
| 7 | Task 12 | 1 | 低 |
| 8 | Task 13 | 1 | 中（spec 字段重命名，向后不兼容） |
| 9 | Task 14 | 1 | 低 |
| 10 | Task 15 | 1 | 低 |
| 11 | Task 16 | 1 | 低（spec 加字段，向后兼容） |
| 12 | Task 17 | 1 | 低 |
| 13 | Task 18 | 1 | 低 |
| 14 | Task 19 | 1 | 低 |
| **合计** | **19 任务** | **19 commits** | |

---

## 不在本计划范围（明确不做）

- ❌ 添加 P0-P1 之外的 stretch goal（versioning protocol for cross-pack imports）
- ❌ 给 `Importer` 加 metrics/tracing（属于横切关注点，下一个 plan）
- ❌ 前端 ImportError UI 改造（属于 web 层，下次 Sprint）

---

## Self-Review（写完计划后）

1. **Spec coverage**：
   - 阻断 #1 → Task 4 ✓
   - 阻断 #2 → Task 1-3 ✓
   - 建议 #3 → Task 6-9 ✓
   - 建议 #4 → Task 14 ✓
   - 建议 #5 → Task 10 ✓
   - 建议 #6 → Task 13 ✓
   - 建议 #7 → Task 11 ✓
   - 建议 #8 → Task 5, 12 ✓
   - 建议 #9 → Task 15 ✓
   - 建议 #10 → Task 15 ✓
   - 建议 #11 → Task 16 ✓
   - 建议 #12 → Task 17 ✓
   - 建议 #13 → Task 19 ✓
   - 建议 #14 → Task 18 ✓

2. **Placeholder scan**：无 "TBD"/"implement later"。

3. **Type consistency**：
   - `importAgent` 在 Phase 2 拆为 `CreateAgentAtomic/UpdateAgentAtomic`，Phase 5 仍调用同一组 — 一致。
   - `KeyMapper` 在 Phase 5 加 `teamKeyToID`，Phase 6 加 `mu` — 字段名一致。
   - `importer_test.go` 的 `stubImporterRepo` 在 Phase 1 拆为 4 个 fake，但所有现有测试应继续通过（`stubImporterRepo` 保留向后兼容 + 4 个 fake 嵌入）。
