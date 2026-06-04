# Skill Evolution Auto Creator 设计文档

> 日期：2026-06-02
> 状态：Draft
> 范围：Agent 运行时重复模式检测 → SKILL.md 自动生成 → 人工审批 → Skill 注册

---

## 一、框架参考（§2.1）

### pkg/trpc-agent-go/skill/ 仓库抽象

```go
type Repository interface {
    Summaries() []Summary
    Get(name string) (*Skill, error)
    Path(name string) (string, error)
}

type Summary struct {
    Name        string
    Description string
}

type Skill struct {
    Summary Summary
    Body    string
    Docs    []Doc
}

type RefreshableRepository interface {
    Repository
    Refresh() error
}

type FSRepository struct { ... }
func NewFSRepository(roots ...string) (*FSRepository, error)
func (r *FSRepository) Refresh() error
```

```go
type VisibilityFilter func(ctx context.Context, summary Summary) bool

type ContextRepository interface {
    Repository
    SummariesForContext(ctx context.Context) []Summary
    GetForContext(ctx context.Context, name string) (*Skill, error)
    PathForContext(ctx context.Context, name string) (string, error)
}

func NewFilteredRepository(base Repository, filter VisibilityFilter) ContextRepository
```

### pkg/trpc-agent-go/tool/skill/ 工具集

```go
type LoadTool struct { ... }
func NewLoadTool(repo skill.Repository) *LoadTool
func NewLoadToolWithOptions(repo skill.Repository, opts ...LoadToolOption) *LoadTool

type RunTool struct { ... }
func NewRunTool(repo skill.Repository, exec codeexecutor.CodeExecutor, opts ...func(*RunTool)) *RunTool

type ExecTool struct { ... }
func NewExecTool(run *RunTool) *ExecTool
```

### OpenClaw skill-creator 预置 Skill

`pkg/trpc-agent-go/openclaw/skills/skill-creator/SKILL.md` 提供了 Skill 创建的参考模板，包含 `init_skill.py`、`package_skill.py`、`quick_validate.py` 脚本。

---

## 二、当前项目现状（§2.2）

| 文件 | 现状 | 不足 |
|------|------|------|
| `internal/skill/importer/engine.go` | Skill 导入引擎 | 仅支持手动导入，不支持自动提议 |
| `internal/skill/storage/filesystem.go` | 文件系统存储 | 支持 SKILL.md 读写，但无自动创建流程 |
| `internal/skill/trpc/repository.go` | DB Repository 适配 | 支持 DB 后端存储 Skill |
| `internal/skill/trpc/executor.go` | Skill 执行器 | 可执行已注册 Skill |
| `internal/skill/watch/runner.go` | Skill 文件监控 | 可检测文件变化并刷新 |
| `internal/biz/agent_settings.go` | `EvolutionCfg.SkillEvolve` 字段 | 配置已预留，逻辑未实现 |
| `internal/biz/evolution_scan.go` | `ScanAgent` 创建建议 | 建议类型含 `skill` 但仅是文本建议 |

---

## 三、架构设计（§2.3）

### 模块在四层架构中的位置

```
api/**/*.proto                    ← 新增 SkillProposal 相关 proto
        ↓
internal/service                  ← SkillEvolutionService：proto↔biz 映射
        ↓
internal/biz                      ← SkillEvolutionUsecase + 端口接口
        ↓
internal/data                     ← SkillProposalRepo 实现（Ent ORM）
```

Agent 运行时模块位置：

```
internal/skill                    ← 扩展现有模块，增加自动创建能力
internal/agent                    ← 模式检测回调
internal/cronrunner/jobs          ← skill_evolution.go 定时触发
```

### 新增/修改的文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `api/aranea/v1/skill_evolution.proto` | 新增 | Skill 自创建 HTTP/gRPC 接口 |
| `internal/service/skill_evolution.go` | 新增 | Service 层 |
| `internal/biz/skill_evolution.go` | 新增 | SkillEvolutionUsecase |
| `internal/biz/skill_evolution_types.go` | 新增 | SkillProposal 领域模型 |
| `internal/biz/skill_evolution_repo.go` | 新增 | Repo 端口接口 |
| `internal/data/skill_evolution.go` | 新增 | Repo 实现 |
| `internal/data/ent/schema/skill_proposal.go` | 新增 | Ent Schema |
| `internal/cronrunner/jobs/skill_evolution.go` | 新增 | 定时任务 |
| `internal/skill/auto_creator.go` | 新增 | SKILL.md 自动生成器 |
| `internal/skill/importer/engine.go` | 修改 | 扩展支持从 Proposal 导入 |
| `internal/biz/evolution_scan.go` | 修改 | ScanAgent 增加 skill 类型闭环 |

### 接口设计

```go
type SkillProposal struct {
    ID           string
    AgentID      string
    PatternHash  string
    PatternDesc  string
    SkillName    string
    SkillMD      string
    Status       string
    ApprovedBy   string
    RejectedBy   string
    CreatedAt    time.Time
    ApprovedAt   *time.Time
}

const (
    SkillProposalStatusPending    = "pending"
    SkillProposalStatusApproved   = "approved"
    SkillProposalStatusRejected   = "rejected"
    SkillProposalStatusRegistered = "registered"
    SkillProposalStatusExpired    = "expired"
)
```

```go
type SkillProposalReader interface {
    ListByAgent(ctx context.Context, agentID string, status string) ([]SkillProposal, error)
    GetByID(ctx context.Context, id string) (SkillProposal, error)
    GetByPatternHash(ctx context.Context, agentID string, hash string) (*SkillProposal, error)
}

type SkillProposalWriter interface {
    Create(ctx context.Context, p SkillProposal) (SkillProposal, error)
    UpdateStatus(ctx context.Context, id string, status string, operator string) (SkillProposal, error)
}
```

```go
type SkillEvolutionUsecase struct {
    proposalRepo SkillProposalReader
    agents       AgentRepository
    skillCreator *SkillAutoCreator
    skillRepo    SkillRegistrationPort
}

type SkillRegistrationPort interface {
    RegisterSkill(ctx context.Context, name string, skillMD string) error
    SkillExists(ctx context.Context, name string) (bool, error)
}

func NewSkillEvolutionUsecase(
    proposalRepo SkillProposalReader,
    agents AgentRepository,
    skillCreator *SkillAutoCreator,
    skillRepo SkillRegistrationPort,
) *SkillEvolutionUsecase

func (uc *SkillEvolutionUsecase) DetectAndPropose(ctx context.Context, agentID string) ([]SkillProposal, error)
func (uc *SkillEvolutionUsecase) GenerateSkillMD(ctx context.Context, patternDesc string) (string, error)
func (uc *SkillEvolutionUsecase) ApproveProposal(ctx context.Context, id string, approvedBy string) (SkillProposal, error)
func (uc *SkillEvolutionUsecase) RejectProposal(ctx context.Context, id string, rejectedBy string) (SkillProposal, error)
func (uc *SkillEvolutionUsecase) RegisterApproved(ctx context.Context, proposalID string) (SkillProposal, error)
func (uc *SkillEvolutionUsecase) ListProposals(ctx context.Context, agentID string, status string) ([]SkillProposal, error)
```

```go
type SkillAutoCreator struct {
    modelProvider func(ctx context.Context) (model.Model, error)
    rootDir       string
}

func NewSkillAutoCreator(modelProvider func(ctx context.Context) (model.Model, error), rootDir string) *SkillAutoCreator

func (c *SkillAutoCreator) GenerateSKILLMD(ctx context.Context, patternDesc string, toolHistory []ToolCallRecord) (string, error)
func (c *SkillAutoCreator) ValidateSKILLMD(ctx context.Context, content string) error
```

```go
type FileSystemSkillRegistrar struct {
    rootDir string
    repo    *skill.FSRepository
}

func NewFileSystemSkillRegistrar(rootDir string, repo *skill.FSRepository) *FileSystemSkillRegistrar

func (r *FileSystemSkillRegistrar) RegisterSkill(ctx context.Context, name string, skillMD string) error
func (r *FileSystemSkillRegistrar) SkillExists(ctx context.Context, name string) (bool, error)
```

### 数据流图

```
工具调用历史 (tool_invocation_stats)
    │
    ▼
重复模式检测 (SkillEvolutionUsecase.DetectAndPropose)
    │  分析 tool_call 频次 + 参数相似度
    │  计算 pattern hash 去重
    ▼
SKILL.md 生成 (SkillAutoCreator.GenerateSKILLMD)
    │  LLM 调用生成 SKILL.md
    │  验证 SKILL.md 格式（YAML front matter + Markdown body）
    ▼
SkillProposal 存储
    │  写入 skill_proposals 表
    │  status = pending
    ▼
人工审批 (前端 / API)
    │  approve → status = approved
    │  reject → status = rejected
    ▼
Skill 注册 (FileSystemSkillRegistrar.RegisterSkill)
    │  写入 <rootDir>/<skillName>/SKILL.md
    │  调用 FSRepository.Refresh() 刷新索引
    │  status = registered
    ▼
Skill 可用 → skill_load / skill_run 可调用
```

---

## 四、与框架的集成方式（§2.4）

1. **Skill 仓库集成**：新 Skill 注册后调用 `FSRepository.Refresh()` 刷新索引，确保 `Repository.Summaries()` 和 `Repository.Get()` 可发现新 Skill
2. **Skill 工具链集成**：注册后的 Skill 自动可被 `LoadTool`/`RunTool`/`ExecTool` 使用，无需额外配置
3. **ContextRepository 集成**：新 Skill 的可见性通过 `VisibilityFilter` 控制，遵循现有 `skillruntime.AgentVisibilityFilter` 逻辑
4. **SKILL.md 格式合规**：生成的 SKILL.md 必须通过 `skill.Repository.Get()` 的解析验证（YAML front matter + Markdown body）
5. **事件发射**：Skill 提议创建/审批/注册通过 `agent.EmitEvent()` 发射事件

---

## 五、错误处理（§2.5）

| 场景 | 处理方式 |
|------|----------|
| SKILL.md 生成 LLM 超时 | 30s 超时取消，记录 FlowLog Warn |
| SKILL.md 格式验证失败 | 标记 proposal 为 `invalid`，不进入审批 |
| Skill 名称冲突 | `SkillExists()` 检查，冲突时追加后缀 |
| 文件写入失败 | 返回 `kerrors.InternalServer`，cronrunner 重试 |
| Refresh 失败 | 记录 FlowLog Warn，Skill 已写入但索引未刷新，下次 Refresh 修复 |
