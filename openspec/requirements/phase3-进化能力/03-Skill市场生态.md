# Skill 市场生态

## 一、需求文档

### 1.1 背景

Hermes 的 agentskills.io 提供了技能市场，用户可浏览、搜索、安装、评分 Skill。OpenClaw 在 `pkg/trpc-agent-go/openclaw/skills/` 预置了 60+ Skills（1Password/GitHub/Slack/Notion/Discord/Spotify 等），覆盖密码管理、代码托管、通讯、音乐等场景。当前项目 `internal/skill/` 有基础管理能力（导入器、渲染器、存储、执行器），前端有 `SkillsPage.vue`，但缺少市场化的浏览、搜索、导入/导出、评分机制。

框架层 `pkg/trpc-agent-go/skill/` 提供了 `Repository` 接口（`Summaries`/`Get`/`Path`）和 `FSRepository` 实现，`ContextRepository` 支持按上下文过滤可见性。`pkg/trpc-agent-go/tool/skill/` 提供了完整的 Skill 工具链。

### 1.2 目标

1. 建立 Skill 市场前端 + 后端 API，支持浏览、搜索、安装、评分
2. 支持从 OpenClaw 预置 Skills 导入，以及用户自建 Skill 的导出/分享
3. 建立评分和审核机制，确保市场 Skill 质量
4. 与现有 `internal/skill/` 模块无缝集成

### 1.3 功能需求

| 编号 | 功能 | 优先级 | 说明 |
|------|------|--------|------|
| F1 | Skill 市场列表 API | P0 | 分页列表、分类筛选、关键词搜索 |
| F2 | Skill 详情 API | P0 | 返回 SKILL.md 内容、文档列表、评分统计、安装数 |
| F3 | Skill 安装 API | P0 | 从市场安装 Skill 到本地仓库，调用 `RefreshableRepository.Refresh()` |
| F4 | Skill 卸载 API | P1 | 从本地仓库移除已安装 Skill |
| F5 | Skill 评分 API | P1 | 用户对已安装 Skill 评分（1-5 星），聚合统计 |
| F6 | Skill 导入 API | P0 | 从 OpenClaw 预置 Skills 或外部 URL 导入 |
| F7 | Skill 导出 API | P2 | 将本地 Skill 打包为可分享格式 |
| F8 | Skill 审核流程 | P1 | 用户提交的 Skill 需经审核后才能进入市场 |
| F9 | 市场前端页面 | P0 | Skill 市场浏览、搜索、安装、评分 UI |
| F10 | OpenClaw 预置 Skills 索引 | P1 | 自动索引 `openclaw/skills/` 下 60+ 预置 Skills |

### 1.4 非功能需求

| 编号 | 需求 | 说明 |
|------|------|------|
| NF1 | 性能 | 市场列表 API 响应时间 < 200ms（分页 20 条） |
| NF2 | 安全性 | Skill 安装前必须通过安全扫描（检查危险命令） |
| NF3 | 可扩展性 | 市场后端支持未来接入远程 Skill 仓库 |
| NF4 | 幂等性 | 重复安装同一 Skill 不报错，覆盖更新 |

### 1.5 验收标准

1. 用户可在前端浏览市场 Skill 列表，按分类/关键词搜索
2. 一键安装 OpenClaw 预置 Skill 后，该 Skill 出现在 `skill.Repository.Summaries()` 中
3. 用户可对已安装 Skill 评分，评分聚合统计正确
4. Skill 安装后可通过 `skill_load`/`skill_run` 正常使用

---

## 二、设计文档

### 2.1 框架参考

#### pkg/trpc-agent-go/skill/ 仓库抽象

```go
// pkg/trpc-agent-go/skill/repository.go
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
func (r *FSRepository) Roots() []string
```

#### OpenClaw 预置 Skills

`pkg/trpc-agent-go/openclaw/skills/` 目录包含 60+ 预置 Skills，每个 Skill 由 `SKILL.md` + 可选脚本/文档组成。典型结构：

```
openclaw/skills/github/
  SKILL.md
  references/
    cli-examples.md
    get-started.md
openclaw/skills/slack/
  SKILL.md
openclaw/skills/1password/
  SKILL.md
  references/
    cli-examples.md
    get-started.md
```

#### pkg/trpc-agent-go/tool/skill/ 工具链

```go
// pkg/trpc-agent-go/tool/skill/load.go
func NewLoadTool(repo skill.Repository) *LoadTool

// pkg/trpc-agent-go/tool/skill/run.go
func NewRunTool(repo skill.Repository, exec codeexecutor.CodeExecutor, opts ...func(*RunTool)) *RunTool

// pkg/trpc-agent-go/tool/skill/exec.go
func NewExecTool(run *RunTool) *ExecTool

// pkg/trpc-agent-go/tool/skill/list_docs.go
func NewListDocsTool(repo skill.Repository) *ListDocsTool

// pkg/trpc-agent-go/tool/skill/select_docs.go
func NewSelectDocsTool(repo skill.Repository) *SelectDocsTool
```

### 2.2 当前项目现状

| 文件 | 现状 | 不足 |
|------|------|------|
| `internal/skill/storage/filesystem.go` | 文件系统存储，支持 SKILL.md 读写 | 无市场概念 |
| `internal/skill/storage/root.go` | 根目录解析 | 无多源支持 |
| `internal/skill/trpc/repository.go` | DB Repository 适配 | 无市场元数据（评分/安装数） |
| `internal/skill/importer/engine.go` | Skill 导入引擎 | 仅支持本地导入，无市场导入 |
| `internal/skill/render/render.go` | SKILL.md 渲染 | 可复用 |
| `internal/skill/watch/runner.go` | 文件监控 + 自动刷新 | 可复用 |
| `web/src/pages/SkillsPage.vue` | 前端 Skill 管理页 | 仅展示已安装 Skill，无市场浏览 |

### 2.3 架构设计

#### 模块在四层架构中的位置

```
api/**/*.proto                    ← 新增 SkillMarket 相关 proto
        ↓
internal/service                  ← SkillMarketService：proto↔biz 映射
        ↓
internal/biz                      ← SkillMarketUsecase + 端口接口
        ↓
internal/data                     ← SkillMarketRepo 实现（Ent ORM）
```

#### 新增/修改的文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `api/aranea/v1/skill_market.proto` | 新增 | 市场接口定义 |
| `internal/service/skill_market.go` | 新增 | Service 层 |
| `internal/biz/skill_market.go` | 新增 | SkillMarketUsecase |
| `internal/biz/skill_market_types.go` | 新增 | 市场领域模型 |
| `internal/biz/skill_market_repo.go` | 新增 | Repo 端口接口 |
| `internal/data/skill_market.go` | 新增 | Repo 实现 |
| `internal/data/ent/schema/skill_market_entry.go` | 新增 | 市场条目 Schema |
| `internal/data/ent/schema/skill_rating.go` | 新增 | 评分 Schema |
| `internal/skill/market_indexer.go` | 新增 | OpenClaw 预置 Skills 索引器 |
| `internal/skill/market_installer.go` | 新增 | Skill 安装器 |
| `internal/cronrunner/jobs/skill_market_sync.go` | 新增 | 定期同步市场索引 |
| `web/src/pages/SkillMarketPage.vue` | 新增 | 市场浏览页 |
| `web/src/features/skill-market/api.ts` | 新增 | 市场 API |
| `web/src/stores/skill-market/index.ts` | 新增 | 市场 Store |
| `web/src/components/skill-market/*.vue` | 新增 | 市场展示组件 |

#### 接口设计

```go
// internal/biz/skill_market_types.go

type MarketEntry struct {
    ID           string
    Name         string
    Description  string
    Category     string
    Source       string
    SourceURL    string
    SKILLMD      string
    AvgRating    float64
    RatingCount  int
    InstallCount int
    Status       string
    ReviewedAt   *time.Time
    ReviewedBy   string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

type SkillRating struct {
    ID         string
    EntryID    string
    UserID     string
    Score      int
    Comment    string
    CreatedAt  time.Time
}

type MarketQuery struct {
    Keyword   string
    Category  string
    Source    string
    SortBy    string
    SortOrder string
    Limit     int
    Offset    int
}
```

```go
// internal/biz/skill_market_repo.go

type MarketEntryReader interface {
    List(ctx context.Context, q MarketQuery) ([]MarketEntry, int, error)
    GetByID(ctx context.Context, id string) (MarketEntry, error)
    GetByName(ctx context.Context, name string) (*MarketEntry, error)
}

type MarketEntryWriter interface {
    Create(ctx context.Context, e MarketEntry) (MarketEntry, error)
    Update(ctx context.Context, e MarketEntry) (MarketEntry, error)
    IncrementInstalls(ctx context.Context, id string) error
}

type SkillRatingReader interface {
    ListByEntry(ctx context.Context, entryID string) ([]SkillRating, error)
    GetByUser(ctx context.Context, entryID string, userID string) (*SkillRating, error)
}

type SkillRatingWriter interface {
    Upsert(ctx context.Context, r SkillRating) (SkillRating, error)
}
```

```go
// internal/biz/skill_market.go

type SkillMarketUsecase struct {
    entryRepo  MarketEntryReader
    ratingRepo SkillRatingReader
    installer  *SkillInstaller
    skillRepo  SkillRegistrationPort
}

func NewSkillMarketUsecase(
    entryRepo MarketEntryReader,
    ratingRepo SkillRatingReader,
    installer *SkillInstaller,
    skillRepo SkillRegistrationPort,
) *SkillMarketUsecase

func (uc *SkillMarketUsecase) ListEntries(ctx context.Context, q MarketQuery) ([]MarketEntry, int, error)
func (uc *SkillMarketUsecase) GetEntry(ctx context.Context, id string) (MarketEntry, error)
func (uc *SkillMarketUsecase) InstallSkill(ctx context.Context, entryID string) error
func (uc *SkillMarketUsecase) UninstallSkill(ctx context.Context, name string) error
func (uc *SkillMarketUsecase) RateSkill(ctx context.Context, entryID string, userID string, score int, comment string) (SkillRating, error)
func (uc *SkillMarketUsecase) SubmitSkill(ctx context.Context, entry MarketEntry) (MarketEntry, error)
func (uc *SkillMarketUsecase) ReviewSkill(ctx context.Context, id string, approved bool, reviewer string) (MarketEntry, error)
func (uc *SkillMarketUsecase) ImportFromOpenClaw(ctx context.Context) (int, error)
```

```go
// internal/skill/market_indexer.go

type OpenClawIndexer struct {
    openclawRoot string
}

func NewOpenClawIndexer(openclawRoot string) *OpenClawIndexer

func (idx *OpenClawIndexer) ScanSkills(ctx context.Context) ([]MarketEntry, error)
```

```go
// internal/skill/market_installer.go

type SkillInstaller struct {
    rootDir string
    repo    skill.RefreshableRepository
}

func NewSkillInstaller(rootDir string, repo skill.RefreshableRepository) *SkillInstaller

func (inst *SkillInstaller) Install(ctx context.Context, name string, skillMD string, docs []skill.Doc) error
func (inst *SkillInstaller) Uninstall(ctx context.Context, name string) error
func (inst *SkillInstaller) IsInstalled(ctx context.Context, name string) (bool, error)
```

#### 数据流图

```
OpenClaw 预置 Skills (pkg/trpc-agent-go/openclaw/skills/)
    │
    ▼
OpenClawIndexer.ScanSkills() → 扫描 SKILL.md → 生成 MarketEntry
    │
    ▼
MarketEntry 存储 (skill_market_entries 表)
    │
    ▼
前端 SkillsMarketPage → 浏览/搜索/安装
    │
    ▼
SkillMarketUsecase.InstallSkill()
    │  SkillInstaller.Install() → 写入 <rootDir>/<name>/SKILL.md
    │  RefreshableRepository.Refresh() → 刷新索引
    │  IncrementInstalls() → 更新安装计数
    ▼
Skill 可用 → skill_load / skill_run 可调用
```

### 2.4 与框架的集成方式

1. **Repository 集成**：安装后调用 `RefreshableRepository.Refresh()` 刷新索引
2. **FSRepository 兼容**：安装的 Skill 写入 `FSRepository` 的 roots 目录，确保 `Summaries()`/`Get()` 可发现
3. **ContextRepository 集成**：市场 Skill 的可见性通过 `VisibilityFilter` 控制
4. **Skill 工具链**：安装后的 Skill 自动可被 `LoadTool`/`RunTool`/`ExecTool` 使用
5. **OpenClaw Skills 索引**：`OpenClawIndexer` 扫描 `openclaw/skills/` 目录，复用 `skill.Repository` 的 `parseSummary` 逻辑解析 SKILL.md

### 2.5 错误处理

| 场景 | 处理方式 |
|------|----------|
| OpenClaw Skills 扫描失败 | 记录 FlowLog Warn，跳过无效 Skill |
| Skill 安装写入失败 | 返回 `kerrors.InternalServer`，不更新安装计数 |
| 重复安装 | 幂等处理：覆盖更新 SKILL.md，不重复增加安装计数 |
| SKILL.md 解析失败 | 市场条目标记为 `invalid`，不展示 |
| 评分越界 | 返回 `kerrors.BadRequest`，score 必须在 1-5 范围 |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|-----------|
| T1 | 定义 `skill_market_types.go` 领域模型 | 无 | S |
| T2 | 定义 `skill_market_repo.go` 端口接口 | T1 | S |
| T3 | 创建 Ent Schema（market_entry/skill_rating） | T1 | M |
| T4 | 实现 `internal/data/skill_market.go` Repo | T2, T3 | M |
| T5 | 实现 `OpenClawIndexer` | 无 | M |
| T6 | 实现 `SkillInstaller` | 无 | M |
| T7 | 实现 `SkillMarketUsecase` 核心方法 | T4, T5, T6 | L |
| T8 | 新增 `skill_market.proto` + Service 层 | T7 | M |
| T9 | 新增 `cronrunner/jobs/skill_market_sync.go` | T5 | S |
| T10 | Wire DI 装配 | T8, T9 | S |
| T11 | 前端 `features/skill-market/api.ts` | T8 | S |
| T12 | 前端 `stores/skill-market/index.ts` | T11 | M |
| T13 | 前端 `SkillMarketPage.vue` + 组件 | T12 | L |
| T14 | 单元测试 | T7 | M |
| T15 | 集成测试 | T10 | L |

### 3.2 开发顺序

```
Phase 1 — 数据基础（T1 → T2 → T3 → T4）
Phase 2 — 核心能力（T5 → T6 → T7）
Phase 3 — 后端接入（T8 → T9 → T10）
Phase 4 — 前端（T11 → T12 → T13）
Phase 5 — 验证（T14 → T15）
```

### 3.3 验证方案

| 阶段 | 验证方式 |
|------|----------|
| Phase 1 | `go generate ./internal/data/ent/... && go build ./...` |
| Phase 2 | `go test ./internal/skill/... -run TestOpenClawIndexer -count=1` |
| Phase 3 | `make api && make wire && make build` |
| Phase 4 | `cd web && pnpm lint && pnpm build` |
| Phase 5 | 端到端：索引 OpenClaw Skills → 前端浏览 → 安装 → skill_load 可用 |
| 提交前 | 后端 `make api && make wire && make build && make test && make lint`；前端 `cd web && pnpm lint && pnpm test && pnpm build` |
