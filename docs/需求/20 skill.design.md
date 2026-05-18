# Skill 技能模块 — 实现设计文档

> 对应需求：`20 skill.md`
> 架构参考：`20 skill struct design.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Skill 是可安装、可版本化的能力包，由文件资产（SKILL.md + 附件）组成，通过上下文注入方式增强 Agent 能力。Skill 不替代 Tool 执行语义，而是作为 Tool 的上游——在运行时由 Assembler 选出并注入 Agent 工具链。

核心链路：**注册（CRUD / 导入）→ 发布 → 启用 → 运行时路由 → ADK Toolset 装配 → 执行追踪**。

---

## 二、Proto 层

### 2.1 已实现 Proto

文件：`api/kratos/skill/v1/skill.proto`

```protobuf
service SkillService {
  rpc ListSkills(ListSkillsRequest) returns (ListSkillsResponse) {
    option (google.api.http) = { get: "/v1/skills" };
  }
  rpc GetSkill(GetSkillRequest) returns (Skill) {
    option (google.api.http) = { get: "/v1/skills/{id}" };
  }
  rpc CreateSkill(CreateSkillRequest) returns (Skill) {
    option (google.api.http) = { post: "/v1/skills" body: "*" };
  }
  rpc UpdateSkill(UpdateSkillRequest) returns (Skill) {
    option (google.api.http) = { patch: "/v1/skills/{id}" body: "*" };
  }
  rpc PublishSkill(PublishSkillRequest) returns (Skill) {
    option (google.api.http) = { post: "/v1/skills/{id}/publish" };
  }
  rpc ToggleSkillEnabled(ToggleSkillEnabledRequest) returns (Skill) {
    option (google.api.http) = { patch: "/v1/skills/{id}/enabled" body: "*" };
  }
  rpc DuplicateSkill(DuplicateSkillRequest) returns (Skill) {
    option (google.api.http) = { post: "/v1/skills/{id}/duplicate" };
  }
  rpc DeleteSkill(DeleteSkillRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/skills/{id}" };
  }
  rpc ListSkillFiles(ListSkillFilesRequest) returns (ListSkillFilesResponse) {
    option (google.api.http) = { get: "/v1/skills/{id}/files" };
  }
  rpc GetSkillFile(GetSkillFileRequest) returns (SkillFile) {
    option (google.api.http) = { get: "/v1/skills/{id}/file" };
  }
  rpc UpdateSkillFile(UpdateSkillFileRequest) returns (SkillFile) {
    option (google.api.http) = { put: "/v1/skills/{id}/file" body: "*" };
  }
  rpc DeleteSkillFile(DeleteSkillFileRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { post: "/v1/skills/{id}/files:delete" body: "*" };
  }
  rpc PreviewSkillRuntime(PreviewSkillRuntimeRequest) returns (PreviewSkillRuntimeResponse) {
    option (google.api.http) = { get: "/v1/skill-runtime-preview" };
  }
  rpc ListSkillRuns(ListSkillRunsRequest) returns (ListSkillRunsResponse) {
    option (google.api.http) = { get: "/v1/skill-runs" };
  }
}
```

### 2.2 ZIP 导入 HTTP 端点（手写路由，不在 proto 内）

文件：`internal/server/skill_import_http.go`

| 端点 | 方法 | 用途 |
|------|------|------|
| `/v1/skills/import` | POST | 上传 ZIP，返回 `job_id` |
| `/v1/skills/import/{job_id}` | GET | 轮询导入状态 |
| `/v1/skills/import/{job_id}/apply` | POST | 应用导入结果 |
| `/v1/skills/import/{job_id}/conflict-groups/{group_id}/refine` | POST | AI 炼化冲突组 |

### 2.3 待规划 RPC

| RPC | 路径 | 用途 |
|-----|------|------|
| `GetSkillVersions` | `GET /v1/skills/{id}/versions` | 版本历史列表 |
| `RollbackSkillVersion` | `POST /v1/skills/{id}/versions/{version}/rollback` | 版本回滚 |

---

## 三、Biz 层

### 3.1 领域模型

```go
// internal/biz/skill.go

type Skill struct {
    ID                 string
    SkillKey           string
    Name               string
    Description        string
    Status             string
    Enabled            bool
    Kind               string
    RiskLevel          string
    EntryPath          string
    FilesystemMissing  bool
    ConfigJSON         string
    MetadataJSON       string
    BodyMarkdown       string
    Version            string
    Tags               []SkillTag
    Permissions        *SkillPermissions
    CreatedAt          time.Time
    UpdatedAt          time.Time
}

type SkillTag struct {
    Name   string
    Source string
}

type SkillPermissions struct {
    CanEdit   bool
    CanDelete bool
    CanView   bool
}

type SkillVersionSummary struct {
    ID        string
    Version   string
    Status    string
    CreatedAt time.Time
}
```

### 3.2 SkillRepo 接口

```go
// internal/biz/skill.go

type SkillRepo interface {
    SearchSkills(ctx, SkillListQuery) (SkillListResult, error)
    GetSkillByID(ctx, id) (Skill, error)
    UpdateSkillEnabled(ctx, id, enabled) (Skill, error)
    DuplicateSkill(ctx, id) (Skill, error)
    DeleteSkill(ctx, id) error
    SearchSkillInvocations(ctx, SkillRunQuery) (SkillRunResult, error)
    GetSkillStorageDir(ctx, id) (string, error)
    ListSkillSimilaritySources(ctx) ([]SkillSimilaritySource, error)
    CreateSkillWithVersion(ctx, SkillCreateInput) (Skill, error)
    GetSkillBySkillKey(ctx, skillKey) (Skill, error)
    UpsertSkillFromDisk(ctx, SkillDiskSyncInput) (Skill, error)
    ListEnabledPublishedSkillKeys(ctx) ([]string, error)
    ListEnabledPublishedSkillCandidates(ctx) ([]SkillRuntimeCandidate, error)
    RecordSkillInvocation(ctx, SkillInvocationWrite) error
    GetLatestSkillMarkdown(ctx, skillID) (string, error)
    PatchSkill(ctx, id, SkillUpdateDraft) (Skill, error)
    PublishSkill(ctx, id) (Skill, error)
    MarkSkillFilesystemMissing(ctx, slug, missing) error
}
```

### 3.3 SkillUsecase

```go
// internal/biz/skill.go

type SkillUsecase struct {
    repo SkillRepo
    // ...
}

func (uc *SkillUsecase) ListSkills(ctx, query) (SkillListResult, error)
func (uc *SkillUsecase) GetSkill(ctx, id) (Skill, error)
func (uc *SkillUsecase) CreateSkill(ctx, input) (Skill, error)
func (uc *SkillUsecase) UpdateSkill(ctx, id, draft) (Skill, error)
func (uc *SkillUsecase) PublishSkill(ctx, id) (Skill, error)
func (uc *SkillUsecase) ToggleSkillEnabled(ctx, id, enabled) (Skill, error)
func (uc *SkillUsecase) DuplicateSkill(ctx, id) (Skill, error)
func (uc *SkillUsecase) DeleteSkill(ctx, id) error
func (uc *SkillUsecase) ListSkillFiles(ctx, id) ([]SkillFileEntry, error)
func (uc *SkillUsecase) GetSkillFile(ctx, id, path) (SkillFileContent, error)
func (uc *SkillUsecase) UpdateSkillFile(ctx, id, path, content) error
func (uc *SkillUsecase) DeleteSkillFile(ctx, id, path) error
func (uc *SkillUsecase) PreviewRuntime(ctx) (SkillRuntimePreview, error)
func (uc *SkillUsecase) ListSkillRuns(ctx, query) (SkillRunResult, error)
func (uc *SkillUsecase) ListEnabledPublishedSkillKeys(ctx) ([]string, error)
func (uc *SkillUsecase) ListEnabledPublishedSkillCandidates(ctx) ([]SkillRuntimeCandidate, error)
func (uc *SkillUsecase) RecordSkillInvocation(ctx, write) error
func (uc *SkillUsecase) UpsertSkillFromDisk(ctx, input) (Skill, error)
func (uc *SkillUsecase) MarkSkillFilesystemMissing(ctx, slug, missing) error
```

### 3.4 运行时策略模型

```go
// internal/biz/skill_runtime.go

type SkillRuntimePolicy struct {
    AllowedSlugs         []string
    DeniedSlugs          []string
    AllowedTags          []string
    IntentRoutingEnabled bool
    IntentMaxPaths       int
    MaxSkillsInToolset   int
}

type SkillRuntimeCandidate struct {
    Slug          string
    Name          string
    Description   string
    Tags          []SkillTag
    TaxonomyPaths []string
}

func ParseSkillRuntimePolicy(jsonStr string) (SkillRuntimePolicy, error)
```

### 3.5 导入 DTO

```go
// internal/biz/skill_import.go

type SkillImportJob struct {
    JobID          string
    Status         string
    TotalCandidates int
    ConflictGroups []SkillImportConflictGroup
}

type SkillImportCandidate struct {
    Slug        string
    Name        string
    Description string
    Source      string
    Similarity  *SkillSimilarityMetrics
}

type SkillImportConflictGroup struct {
    GroupID     string
    Slug        string
    Candidates  []SkillImportCandidate
    Resolution  string
}

type SkillSimilarityMetrics struct {
    NameSimilarity       float64
    DescriptionSimilarity float64
    OverallSimilarity    float64
}

type SkillImportRefineRequest struct {
    JobID   string
    GroupID string
    Action  string
}

type SkillImportApplyRequest struct {
    JobID    string
    Resolutions map[string]string
}
```

---

## 四、Data 层

### 4.1 Ent Schema

- `internal/data/ent/schema/platform_skill.go` — Skill 主表
  - 关键字段：`skill_key`、`name`、`description`、`status`、`enabled`、`kind`、`risk_level`、`entry_path`、`filesystem_missing`、`config_json`、`metadata_json`
- `internal/data/ent/schema/skill_version.go` — 版本表
  - 关键字段：`skill_id`、`version`、`status`、`content_markdown`、`manifest_json`、`published_at`、`validation_status`
- `internal/data/ent/schema/skill_invocation.go` — 调用记录表
  - 关键字段：`skill_version`、`user_id`、`session_id`、`duration_ms`、`source`、`activation_id`

### 4.2 SkillRepo 实现

文件：`internal/data/skill.go`

关键方法：
- `SearchSkills`：支持搜索、标签、启用、状态筛选、分页；`published` 与历史 `active` 等同处理
- `CreateSkillWithVersion`：创建草稿 + 初始版本
- `PatchSkill`：可选字段更新（仅 `draft` 状态）
- `PublishSkill`：`draft` → `published`
- `UpsertSkillFromDisk`：磁盘同步（全量扫描 / fsnotify 增量）
- `ListEnabledPublishedSkillCandidates`：返回运行时候选（含 `TaxonomyPaths`）
- `MarkSkillFilesystemMissing`：标记/清除磁盘缺失状态

---

## 五、运行时层

### 5.1 装配入口

文件：`internal/agent/trpc_build.go` → `buildSkillDeps`

流程：
1. `SkillUC.ListEnabledPublishedSkillKeys()` 获取已启用 + 已发布的 slug 列表
2. 优先使用 `SkillDBRepo`（`DBRepositoryAdapter`，DB-backed + TTL 缓存）；否则回退到 `FSRepositoryAdapter`（磁盘 FS）
3. 构建 `FilteredRepository`（按 slug 白名单过滤）
4. 构建 `CodeExecutor`（local / docker，按 `CODE_EXECUTOR_BACKEND` 环境变量选择）
5. 传入 `trpcllmagent.WithSkills(repo)` + `WithSkillFilter(filter)` + `WithSkillToolProfile(SkillToolProfileFull)`

### 5.2 运行时路由

文件：`internal/tools/skillruntime/resolve.go`

两级筛选：

**Layer A**（`applyLayerA`）：
- 按 `SkillRuntimePolicy.AllowedSlugs` / `DeniedSlugs` 过滤

**Layer B**（`resolveSkillSlugs`）：
- `skillrouter.DetectIntentPaths(query, maxPaths)` → 分类路径关键词匹配
- `filterByIntentPaths()` → 按分类路径缩小候选
- `filterByAllTags()` → 按 `AllowedTags` + `ExtractTagHints()` 过滤
- `scoreCandidates()` → 按分类路径匹配度评分，排序后取 `MaxSkillsInToolset`

### 5.3 意图路由与分类

文件：`internal/tools/skillrouter/`

- `detect.go`：`DetectIntentPaths(userQuery, maxPaths)` → 关键词 → 分类路径
- `taxonomy.go`：`TaxonomyLeaves` 定义 + `ExtractTagHints(userQuery)` 提取 `file_type:*` / `domain:*` 提示

### 5.4 trpc-agent-go 桥接

文件：`internal/skill/trpc/`

- `repository.go`：`FSRepositoryAdapter` — 磁盘 FS → `trpcskill.Repository`
- `db_repository.go`：`DBRepositoryAdapter` — DB + TTL 缓存 → `trpcskill.Repository`
- `filter.go`：`NewFilteredRepository(base, allowedSlugs)` → `trpcskill.ContextRepository`
- `tools.go`：`BuildSkillTools()` 产出 4 个 ADK 工具（LoadTool / RunTool / ListDocsTool / SelectDocsTool）
- `executor.go`：`CodeExecutor` 适配（local / docker）

### 5.5 运行时策略存储

文件：`internal/biz/skill_runtime.go`

存储在 `agent_runtime_settings.skill_runtime_json`，字段：
- `allowed_slugs`、`denied_slugs`、`allowed_tags`
- `intent_routing_enabled`（默认 true）
- `intent_max_paths`（默认 3）
- `max_skills_in_toolset`（默认 32，上限 256）

---

## 六、磁盘监听与同步

### 6.1 文件系统监听

文件：`internal/skill/watch/runner.go`

- 启动全量扫描 + `fsnotify` 增量监听（debounce 2s）
- 环境变量 `SKILL_WATCH_DISABLED=1` 可关闭
- 支持 `event.Bus` 集成，同步成功后发布 `skill.reload` 事件
- 磁盘目录缺失时调用 `MarkSkillFilesystemMissing(slug, true)` 标记，恢复时清除

### 6.2 存储根解析

文件：`internal/skill/storage/root.go`

解析优先级（自上而下短路命中）：
1. `SKILL_ROOT` 环境变量
2. `SKILL_STORAGE_ROOT` 环境变量
3. `filepath.Join(Resolved(work_directory), "skills")`
4. 操作系统默认路径（`%AppData%\Aranea\skills` 等）

`ResolveRootWithPlatform(rootDirectory)` 实现完整回落链路。

---

## 七、ZIP 导入

### 7.1 导入引擎

文件：`internal/skill/importer/`

子包：
- `engine.go`：导入主流程（解压 → 校验 → 相似度检测 → 冲突分组）
- `validate.go`：ZIP 结构与 frontmatter 校验
- `helpers.go`：辅助函数
- `chat.go`：LLM 相似度检测与炼化
- `errors.go`：错误类型

### 7.2 HTTP 路由

文件：`internal/server/skill_import_http.go`

4 个端点（手写路由，不在 proto 内）：
- `POST /v1/skills/import` — 上传 ZIP
- `GET /v1/skills/import/{job_id}` — 轮询状态
- `POST /v1/skills/import/{job_id}/apply` — 应用结果
- `POST /v1/skills/import/{job_id}/conflict-groups/{group_id}/refine` — AI 炼化

---

## 八、Service 层

文件：`internal/service/skill.go`

薄适配层，职责：
- Proto Request → Biz DTO 转换
- `resolvedStorageRoot()`：通过 `SystemSettingRepo.Get()` 读取 `work_directory` → `storage.ResolveRootWithPlatform()`
- `safeSkillFilePath()`：路径安全校验（禁止 `..` 跳出 Skill 根）

已实现方法：
```go
func (s *SkillService) ListSkills(ctx, req) (*ListSkillsResponse, error)
func (s *SkillService) GetSkill(ctx, req) (*Skill, error)
func (s *SkillService) CreateSkill(ctx, req) (*Skill, error)
func (s *SkillService) UpdateSkill(ctx, req) (*Skill, error)
func (s *SkillService) PublishSkill(ctx, req) (*Skill, error)
func (s *SkillService) ToggleSkillEnabled(ctx, req) (*Skill, error)
func (s *SkillService) DuplicateSkill(ctx, req) (*Skill, error)
func (s *SkillService) DeleteSkill(ctx, req) (*emptypb.Empty, error)
func (s *SkillService) ListSkillFiles(ctx, req) (*ListSkillFilesResponse, error)
func (s *SkillService) GetSkillFile(ctx, req) (*SkillFile, error)
func (s *SkillService) UpdateSkillFile(ctx, req) (*SkillFile, error)
func (s *SkillService) DeleteSkillFile(ctx, req) (*emptypb.Empty, error)
func (s *SkillService) PreviewSkillRuntime(ctx, req) (*PreviewSkillRuntimeResponse, error)
func (s *SkillService) ListSkillRuns(ctx, req) (*ListSkillRunsResponse, error)
```

---

## 九、Wire 注入

已有，无需新增。Skill 相关依赖通过 `wire.NewSet` 注入：
- `SkillRepo` → `SkillUsecase` → `SkillService`
- `SkillUsecase` → `buildSkillDeps`（Agent 构建）

---

## 十、Web 前端设计

### 10.1 文件结构

```
web/src/features/skills/
├── api.ts
├── types.ts
├── SkillEditorDialog.vue
├── SkillItem.vue
├── SkillImportDialog.vue
└── components/
    ├── SkillListPage.vue
    ├── SkillDetailPage.vue
    ├── SkillFileEditor.vue
    └── SkillRuntimePreview.vue
```

### 10.2 组件设计

**SkillEditorDialog.vue**：

| 控件 | 绑定 | 说明 |
|------|------|------|
| `QInput` 名称 | `name` | 必填 |
| `QInput` 描述 | `description` | 可选 |
| `QSelect` 标签 | `tags` | 多选，支持 `file_type:*` / `domain:*` |
| `QSelect` 类型 | `kind` | markdown / prompt_pack / workflow / tool_backed |
| `QSelect` 风险等级 | `riskLevel` | low / medium / high |
| `QEditor` 正文 | `bodyMarkdown` | Markdown 编辑器 |

**SkillFileEditor.vue**：

| 控件 | 绑定 | 说明 |
|------|------|------|
| 文件树 | `files` | 左侧文件列表 |
| 代码编辑器 | `fileContent` | 右侧编辑区 |
| 保存按钮 | — | `UpdateSkillFile` |

**SkillImportDialog.vue**：

| 控件 | 绑定 | 说明 |
|------|------|------|
| 文件上传 | `zipFile` | ZIP 文件选择 |
| 进度显示 | `jobStatus` | 轮询导入状态 |
| 冲突解决 | `conflictGroups` | AI 炼化 / 手动选择 |

### 10.3 API

```typescript
export async function listSkills(query: SkillListQuery): Promise<SkillListResult>
export async function getSkill(id: string): Promise<Skill>
export async function createSkill(req: CreateSkillRequest): Promise<Skill>
export async function updateSkill(id: string, req: UpdateSkillRequest): Promise<Skill>
export async function publishSkill(id: string): Promise<Skill>
export async function toggleSkillEnabled(id: string, enabled: boolean): Promise<Skill>
export async function duplicateSkill(id: string): Promise<Skill>
export async function deleteSkill(id: string): Promise<void>
export async function listSkillFiles(id: string): Promise<SkillFileEntry[]>
export async function getSkillFile(id: string, path: string): Promise<SkillFile>
export async function updateSkillFile(id: string, path: string, content: string): Promise<SkillFile>
export async function deleteSkillFile(id: string, path: string): Promise<void>
export async function previewSkillRuntime(): Promise<SkillRuntimePreview>
export async function listSkillRuns(query: SkillRunQuery): Promise<SkillRunResult>
export async function importSkillZip(file: File): Promise<SkillImportJob>
export async function getImportStatus(jobId: string): Promise<SkillImportJob>
export async function applyImport(jobId: string, resolutions: Record<string, string>): Promise<void>
export async function refineConflictGroup(jobId: string, groupId: string): Promise<void>
```
