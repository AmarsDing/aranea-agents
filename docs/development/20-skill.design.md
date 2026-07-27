# Skill 技能模块 — 实现设计文档

> 对应需求：`20-skill.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Skill 是可安装、可版本化的能力包，由文件资产（SKILL.md + 附件）组成，通过上下文注入方式增强 Agent 能力。Skill 不替代 Tool 执行语义，而是作为 Tool 的上游——在运行时由 Assembler 选出并注入 Agent 工具链。

核心链路：**注册（CRUD / 导入）→ 发布 → 启用 → 运行时路由 → trpc-agent-go Skill 工具链 → 执行追踪**。

**一句话**：管理面由 **Kratos + SQLite（Ent）+ Skill 根目录文件** 承载；运行时按 Agent/会话选 Skill → trpc-agent-go 装配 → 记录 usage。

---

## 二、概念边界

- Skill 归属 **Capability Context**（与 Tool / MCP / Provider 并列）。
- **跨 Context 规则**：Catalog / Conversation 只能通过 **`kernel/contracts`**（未来端口）查询 Skill 视图；按目标态，**禁止**在非专用运行时适配包内 import `google.golang.org/adk`（当前仓库仍存在历史调用点如 `internal/tools/catalog`，迁移时应收敛）。
- **依赖方向（目标态）**：`proto → internal/service → internal/biz → internal/data`；导入等特殊路由可走同一服务的 HTTP 挂载，但业务逻辑应落在 **biz**，持久化在 **data**。
- Skill **默认不执行副作用**；需要读写文件、调用外部 API 时走 **Tool** 或显式「SkillAction」类扩展（若引入）。
- Skill 是知识/流程/提示包，Tool 是可执行函数；协作靠 Runtime Prompt + usage 关联。

---

## 三、目标分层架构

在「平台目标态」下，Skill 与 Tool 共享同一种 **编排句式**：**registry → executor → middleware → backends**，差别在后端语义（加载/渲染 vs 执行）。

```text
┌─────────────────────────────────────────────────────────┐
│ Driving：HTTP(gRPC-Gateway) / CLI / Cron                   │
├─────────────────────────────────────────────────────────┤
│ Capability.application                                     │
│   · SkillCatalog（CRUD / version / import）               │
│   · SkillRuntimeAssembler（按回合选 Skill → 视图）        │
├──────────────────────────┬────────────────────────────────┤
│ Middleware               │ Executor                       │
│ · validation             │ · manifest / frontmatter 解析   │
│ · policy（enabled/status）│ · load files                  │
│ · budget（数量/token）    │ · render prompt block          │
│ · cache                  │ · record usage                 │
│ · tracing                │                                │
├──────────────────────────┴────────────────────────────────┤
│ Backends                                                   │
│ · MarkdownSkill（SKILL.md + assets）                        │
│ · PromptPack（多文件拼接）                                  │
│ · Workflow / Recipe（可选）                                 │
│ · Tool-backed（说明 + 触发 Tool，副作用仍在 Tool）          │
├────────────────────────────────────────────────────────────┤
│ Driven：Ent/SQLite · OS 文件 · LLM（相似度/炼化）· OTel     │
└────────────────────────────────────────────────────────────┘
```

**与 trpc-agent-go 的交界（A + B + C 均已实现）**：

- **方式 A**：`FSRepositoryAdapter` → `WithSkills`
- **方式 B**：`DBRepositoryAdapter`（TTL 缓存）→ `WithSkills`
- **方式 C**：Prompt 块注入 system message（BeforeModelHook）

装配在 `internal/agent/trpc_build.go`；`internal/skill/trpc/` 不 import 框架运行时。

---

## 四、数据模型

### 4.1 逻辑模型（前端消费字段）

#### `Skill`

| 字段 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `id` | string | 是 | Skill ID |
| `name` | string | 是 | 展示名 |
| `slug` | string | 否 | 文件友好标识 |
| `description` | string | 是 | 描述 |
| `tags` | `SkillTag[]` | 是 | 标签 |
| `extends_skill_id` | string \| null | 否 | 父 Skill |
| `status` | `draft` \| `published` \| `archived` | 是 | 状态 |
| `enabled` | boolean | 是 | 是否参与运行时注入 / 调度 |
| `current_version` | `SkillVersionSummary` \| null | 否 | 当前版本 |
| `invoke_count` | number | 是 | 累计调用次数 |
| `success_count` | number | 是 | 累计成功数 |
| `failure_count` | number | 是 | 累计失败数 |
| `usage_count_7d` | number | 否 | 近 7 日调用次数 |
| `avg_duration_ms` | number \| null | 否 | 平均耗时 |
| `last_agent_id` | string \| null | 否 | 最近一次调用该 Skill 的 Agent ID |
| `last_agent_display_name` | string \| null | 否 | 最近一次调用该 Skill 的 Agent 名称 |
| `last_invoked_at` | string \| null | 否 | 最近调用时间 |
| `last_duration_ms` | number \| null | 否 | 最近一次调用耗时 |
| `created_at` | string | 是 | 创建时间 |
| `updated_at` | string | 是 | 更新时间 |
| `permissions` | object | 是 | 当前用户可执行操作 |
| `filesystem_missing` | boolean | 否 | 磁盘目录是否缺失 |
| `sync_origin` | string | 否 | `filesystem` \| `import` \| `manual` \| 空 |
| `visibility` | string | 否 | `system` \| `workspace` \| `agent` \| `private` \| `public` |
| `default_config_json` | string | 否 | 默认配置 JSON |

统计口径：

- `invoke_count` = `skill_invocation` 中同一 `skill_id` 的总记录数。
- `success_count` = `status = success` 的调用记录数。
- `failure_count` = `status = failure` 的调用记录数。
- `usage_count_7d` = `started_at >= now() - 7 days` 的调用记录数。
- `avg_duration_ms` = 所有调用记录 `duration_ms` 的平均值；可由后端异步聚合。
- `last_agent_id` / `last_agent_display_name` / `last_invoked_at` / `last_duration_ms` 来自 `started_at` 最新的一条调用记录。

#### `SkillVersionSummary`

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 版本 ID |
| `version` | string | 版本号 |
| `validation_status` | `pass` \| `warn` \| `block` | 最近发布校验结果 |
| `published_at` | string \| null | 发布时间 |

#### `SkillTag`

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | 标签名 |
| `source` | `system` \| `user` | 标签来源 |

#### `SkillInvocation`

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 调用记录 ID |
| `skill_id` | string | Skill ID |
| `skill_name` | string | Skill 名称快照 |
| `skill_version` | string | 实际执行版本 |
| `agent_id` | string | Agent ID |
| `agent_display_name` | string | Agent 名称快照 |
| `user_id` | string \| null | 触发用户 |
| `session_id` | string \| null | 会话 / 任务关联 |
| `status` | `success` \| `failure` | 执行结果 |
| `duration_ms` | number | 耗时 |
| `started_at` | string | 调用开始时间 |
| `ended_at` | string \| null | 调用结束时间 |
| `input_preview` | string \| null | 脱敏输入摘要 |
| `input_hash` | string \| null | 输入哈希 |
| `output_preview` | string \| null | 输出摘要 |
| `error_code` | string \| null | 错误码 |
| `error_message` | string \| null | 错误摘要 |
| `permissions` | object | 当前用户是否可查看详情 |
| `source` | string | `runtime` \| `filesystem_scan` \| `filesystem_watch` |
| `activation_id` | string | 激活 ID |
| `message_id` | string | 消息 ID |

### 4.2 Ent Schema（物理表）

| 表名 | Schema 文件 | 关键字段 |
|------|------------|----------|
| `skill` | `internal/data/ent/schema/platform_skill.go` | `skill_key`、`name`、`description`、`status`、`enabled`、`kind`、`risk_level`、`entry_path`、`filesystem_missing`、`config_json`、`metadata_json`、`visibility`、`fallback_config_json`(StorageKey `default_config_json`)、`parent_version_id`、`evolution_reason`、`lifecycle_status` |
| `skill_version` | `internal/data/ent/schema/skill_version.go` | `skill_id`、`version`、`status`、`content_markdown`、`metadata_json`、`manifest_json`、`published_at`、`validation_status`、`file_manifest_json`、`parent_version_id`、`evolution_reason`、`lifecycle_status` |
| `skill_invocation` | `internal/data/ent/schema/skill_invocation.go` | `skill_id`、`agent_id`、`status`、`skill_version`、`user_id`、`session_id`、`duration_ms`、`started_at`/`ended_at`、`input_preview`/`input_hash`、`output_preview`、`error_code`、`source`、`activation_id`、`message_id`、`selection_reason`(JSON)、`outcome`、`token_usage`(JSON)、`routed_slugs`(JSON)、`loaded_slug` |
| `skill_import_jobs` | `internal/data/ent/schema/skill_import_job.go` | `id`、`status`、`validation_status`、`storage_root`、`candidates_json`、`conflict_groups_json`、`temp_dir`、`created_at`、`applied_at` |

> **A6 物理收敛**：原 L2 表 `skill_evolution_suggestions`（Ent Schema）已删除。全部四类进化建议（L1 `skill_proposals` / L2 `skill_evolution_suggestions` / L3 `evolution_suggestions` / 统一表）已收敛到唯一的 `unified_evolution_suggestions` 表（raw SQL DDL，见 §6.8），legacy 专有字段保留在 `metadata` JSON 列中。迁移 `20261111` 完成 backfill 后 DROP 三张 legacy 表。

列表查询将 **`published` 与历史值 `active` 等同**（见 `skillListPredicates`），与迁移期数据共存。

### 4.3 规划表（可选，重度依赖/权限开启时）

- `skill_permissions`（subject + 动作位）
- `skill_dependencies`
- `skill_conflicts`

具体 DDL 见附录 B；落地前需与表前缀策略对齐（平台文档倾向 `capability_*` 前缀）。

### 4.4 Manifest（逻辑模型）

与 `pkg/trpc-agent-go/tool/skilltoolset/skill` 的 frontmatter 习惯对齐，同时支持独立 `skill.json`：

| 字段 | 说明 |
|------|------|
| `name` / `slug` / `description` / `version` | 展示与版本 |
| `tags[]` | 检索与 Agent 策略 |
| `kind` | `markdown` \| `prompt_pack` \| `workflow` \| `tool_backed` |
| `entry` | 默认 `SKILL.md` |
| `requires[]` / `conflicts[]` | 依赖与冲突 |
| `risk_level` | 策略中间件使用 |

**来源优先级**：`skill.json` / `manifest.json` → `SKILL.md` frontmatter → 导入 API 附加 metadata → 目录名推断。

---

## 五、API 契约

### 5.1 Proto 层（22 RPC）

文件：`api/kratos/skill/v1/skill.proto`

```protobuf
service SkillService {
  rpc ListSkills(ListSkillsRequest) returns (ListSkillsResponse) {
    option (google.api.http) = { get: "/v1/skills" };
  }
  rpc GetSkillFilesystemHealth(google.protobuf.Empty) returns (SkillFilesystemHealth) {
    option (google.api.http) = { get: "/v1/skills/filesystem-health" };
  }
  rpc GetSkill(GetSkillRequest) returns (GetSkillResponse) {
    option (google.api.http) = { get: "/v1/skills/{id}" };
  }
  rpc CreateSkill(CreateSkillRequest) returns (Skill) {
    option (google.api.http) = { post: "/v1/skills" body: "*" };
  }
  rpc UpdateSkill(UpdateSkillRequest) returns (Skill) {
    option (google.api.http) = { patch: "/v1/skills/{id}" body: "*" };
  }
  rpc PublishSkill(PublishSkillRequest) returns (Skill) {
    option (google.api.http) = { post: "/v1/skills/{id}/publish" body: "*" };
  }
  rpc PreviewSkillRuntime(PreviewSkillRuntimeRequest) returns (PreviewSkillRuntimeResponse) {
    option (google.api.http) = { get: "/v1/skill-runtime-preview" };
  }
  rpc ToggleSkillEnabled(ToggleSkillEnabledRequest) returns (Skill) {
    option (google.api.http) = { patch: "/v1/skills/{id}/enabled" body: "*" };
  }
  rpc DuplicateSkill(DuplicateSkillRequest) returns (Skill) {
    option (google.api.http) = { post: "/v1/skills/{id}/duplicate" body: "*" };
  }
  rpc DeleteSkill(DeleteSkillRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/skills/{id}" };
  }
  rpc ListSkillFiles(ListSkillFilesRequest) returns (ListSkillFilesResponse) {
    option (google.api.http) = { get: "/v1/skills/{id}/files" };
  }
  rpc GetSkillFile(GetSkillFileRequest) returns (SkillFileContent) {
    option (google.api.http) = { get: "/v1/skills/{id}/file" };
  }
  rpc UpdateSkillFile(UpdateSkillFileRequest) returns (SkillFileContent) {
    option (google.api.http) = { put: "/v1/skills/{id}/file" body: "*" };
  }
  rpc DeleteSkillFile(DeleteSkillFileRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { post: "/v1/skills/{id}/files:delete" body: "*" };
  }
  rpc ListSkillRuns(ListSkillRunsRequest) returns (ListSkillRunsResponse) {
    option (google.api.http) = { get: "/v1/skill-runs" };
  }
  rpc ImportSkillZip(google.protobuf.Empty) returns (ImportSkillZipResponse) {
    option (google.api.http) = { post: "/v1/skills/import" body: "*" };
  }
  rpc GetSkillImportJob(GetSkillImportJobRequest) returns (SkillImportJob) {
    option (google.api.http) = { get: "/v1/skills/import/{job_id}" };
  }
  rpc ApplySkillImport(ApplySkillImportRequest) returns (SkillImportApplyResult) {
    option (google.api.http) = { post: "/v1/skills/import/{job_id}/apply" body: "*" };
  }
  rpc RefineSkillImportConflict(RefineSkillImportConflictRequest) returns (SkillRefineResult) {
    option (google.api.http) = {
      post: "/v1/skills/import/{job_id}/conflict-groups/{group_id}/refine"
      body: "*"
    };
  }
  rpc GetSkillVersions(GetSkillVersionsRequest) returns (GetSkillVersionsResponse) {
    option (google.api.http) = { get: "/v1/skills/{skill_id}/versions" };
  }
  rpc RollbackSkillVersion(RollbackSkillVersionRequest) returns (Skill) {
    option (google.api.http) = { post: "/v1/skills/{skill_id}/versions/{version_id}/rollback" body: "*" };
  }
  rpc GetSkillHealth(GetSkillHealthRequest) returns (SkillHealthMetric) {
    option (google.api.http) = { get: "/v1/skills/{skill_id}/health" };
  }
}
```

### 5.2 ZIP 导入 multipart 端点

文件：`internal/service/skill_import_http.go`（由 `internal/server/http.go` 挂载）

| 端点 | 方法 | 用途 |
|------|------|------|
| `/v1/skills/import` | POST | 上传 ZIP（multipart；`ImportSkillZip` RPC 占位用于 OpenAPI） |
| `/v1/skills/import/{job_id}` | GET | 轮询导入状态 |
| `/v1/skills/import/{job_id}/apply` | POST | 应用导入结果 |
| `/v1/skills/import/{job_id}/conflict-groups/{group_id}/refine` | POST | AI 炼化冲突组 |

### 5.3 通用分页响应

```json
{
  "items": [],
  "page": 1,
  "page_size": 20,
  "total": 123
}
```

### 5.4 Skill 列表

`GET /v1/skills?search=&tags=&enabled=&status=&filesystem_missing=&sync_origin=&page=1&page_size=20`

响应示例：

```json
{
  "items": [
    {
      "id": "skill_01",
      "name": "Figma Code Connect",
      "slug": "figma-code-connect",
      "description": "Creates and maintains Figma Code Connect template files.",
      "tags": [{ "name": "figma", "source": "system" }],
      "extends_skill_id": null,
      "status": "published",
      "enabled": true,
      "current_version": {
        "id": "ver_01",
        "version": "1.0.0",
        "validation_status": "pass",
        "published_at": "2026-04-25T09:00:00Z"
      },
      "invoke_count": 12,
      "success_count": 10,
      "failure_count": 2,
      "usage_count_7d": 5,
      "avg_duration_ms": 2300,
      "last_agent_id": "agent_01",
      "last_agent_display_name": "Design Assistant",
      "last_invoked_at": "2026-04-25T09:30:00Z",
      "last_duration_ms": 1800,
      "created_at": "2026-04-20T09:00:00Z",
      "updated_at": "2026-04-25T09:00:00Z",
      "permissions": {
        "can_edit": true,
        "can_delete": true,
        "can_toggle_enabled": true,
        "can_duplicate": true
      },
      "filesystem_missing": false,
      "sync_origin": "import",
      "visibility": "workspace",
      "default_config_json": "{}"
    }
  ],
  "page": 1,
  "page_size": 20,
  "total": 1
}
```

### 5.5 创建 / 更新 / 发布

| 方法 | 路径 | 请求体 | 说明 |
|------|------|--------|------|
| `POST` | `/v1/skills` | `CreateSkillRequest` | 创建草稿 |
| `PATCH` | `/v1/skills/:id` | `UpdateSkillRequest` | 更新草稿或元数据 |
| `POST` | `/v1/skills/:id/publish` | `PublishSkillRequest` | 发布并触发校验 |
| `PATCH` | `/v1/skills/:id/enabled` | `ToggleSkillEnabledRequest` | 启用 / 停用 |
| `DELETE` | `/v1/skills/:id` | 无 | 软删 |
| `POST` | `/v1/skills/:id/duplicate` | 无 | 复制为新草稿 |

### 5.6 上传导入

`POST /v1/skills/import`

- 请求：`multipart/form-data`，字段名 `file`。
- 后端行为：接收 zip 后写入临时目录，解压并严格校验 Skill 编写规范；通过校验后生成导入任务，不直接入库。
- 响应：`{ "job_id": "job_01" }`

`GET /v1/skills/import/:job_id` — 轮询导入状态，返回 `SkillImportJob`（含 `candidates`、`conflict_groups`）。

`POST /v1/skills/import/:job_id/apply` — 应用导入结果，请求体含 `decisions[]`（`import_passed` / `skip_group` / `merge_group_with_ai`）。

`POST /v1/skills/import/:job_id/conflict-groups/:group_id/refine` — AI 炼化冲突组，返回 `SkillRefineResult`。

### 5.7 运行记录

`GET /v1/skill-runs?skill_id=&agent_id=&session_id=&status=&from=&to=&page=1&page_size=20`

响应使用通用分页结构，`items[]` 为 `SkillInvocation`。

### 5.8 版本历史与回滚

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/v1/skills/:skill_id/versions` | 版本历史列表（分页） |
| `POST` | `/v1/skills/:skill_id/versions/:version_id/rollback` | 版本回滚（不可变策略：新建版本 + patch 递增） |

### 5.9 健康指标

`GET /v1/skills/:skill_id/health` — 返回 `SkillHealthMetric`（7d/30d 调用统计、成功率、P95 耗时、每日明细）。

### 5.10 磁盘同步字段

| 字段 | 说明 |
|------|------|
| `Skill.filesystem_missing` | 磁盘目录是否缺失 |
| `Skill.sync_origin` | `filesystem` \| `import` \| `manual` \| 空 |
| `ListSkillsRequest.filesystem_missing` | 筛选：`true` / `false` / 空 |
| `ListSkillsRequest.sync_origin` | 按来源筛选 |
| `SkillFilesystemHealth` | 汇总：根目录可达、缺失数、待审核磁盘 Skill 数 |

### 5.11 错误格式

所有接口错误统一返回：

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "名称已存在",
    "details": {}
  }
}
```

前端展示规则：

- `message` 可直接展示给用户。
- `details.field_errors` 若存在，映射到表单字段。
- 未知错误展示「操作失败，请稍后重试」。

---

## 六、Biz 层

### 6.1 领域模型

```go
// internal/biz/skill/skill.go

type Skill struct {
    ID                   string
    Name                 string
    Slug                 string
    Description          string
    Tags                 []SkillTag
    ExtendsSkillID       string
    Status               string
    Enabled              bool
    CurrentVersion       *SkillVersionSummary
    InvokeCount          int
    SuccessCount         int
    FailureCount         int
    UsageCount7d         int
    AvgDurationMS        *float64
    LastAgentID          string
    LastAgentDisplayName string
    LastInvokedAt        string
    LastDurationMS       *int
    CreatedAt            string
    UpdatedAt            string
    Permissions          SkillPermissions
    FilesystemMissing    bool
    SyncOrigin           string
    Visibility           string
    DefaultConfigJSON    string
    ParentVersionID      string
    EvolutionReason      string
    LifecycleStatus      string
}

type SkillTag struct {
    Name   string
    Source string
}

type SkillPermissions struct {
    CanEdit          bool
    CanDelete        bool
    CanToggleEnabled bool
    CanDuplicate     bool
}

type SkillVersionSummary struct {
    ID               string
    Version          string
    ValidationStatus string
    PublishedAt      string
}
```

### 6.2 Repo 窄接口拆分

`Repo` 接口按职责拆分为 `SkillReader` + `SkillWriter`，`Repo` 组合两者保持向后兼容：

- **SkillReader**（组合 `SkillQueryReader` + `SkillLookupReader` + `SkillRuntimeReader`）：
  - `SearchSkills` / `SearchSkillInvocations` / `ListSkillVersions` / `ListSkillSimilaritySources` / `ListRegisteredSlugs`
  - `GetSkillByID` / `GetSkillBySkillKey` / `GetSkillStorageDir` / `GetLatestSkillMarkdown`
  - `BatchGetSkillMarkdownBySlugs` / `ListEnabledPublishedSkillKeys` / `ListEnabledPublishedSkillCandidates` / `FilesystemHealthStats`

- **SkillWriter**（组合 `SkillMutationWriter` + `SkillSyncWriter`）：
  - `CreateSkillWithVersion` / `UpdateSkillEnabled` / `DuplicateSkill` / `DeleteSkill` / `PatchSkill`
  - `PublishSkill` / `UpsertSkillFromDisk` / `MarkSkillFilesystemMissing` / `RecordSkillInvocation` / `RollbackSkillVersion`

新消费者应优先依赖窄接口（`SkillReader` 或 `SkillWriter`），仅同时需要读写时才使用 `Repo`。完整方法签名见 `internal/biz/skill/skill.go`。

### 6.3 Usecase

```go
// internal/biz/skill/skill.go

type Usecase struct {
    repo       Repo
    embedder   SkillEmbedder
    embedMu    sync.RWMutex
    embedCache map[string]embedEntry
    embedTTL   time.Duration
}

func NewUsecase(repo Repo, embedder SkillEmbedder) *Usecase

// 主要方法：
func (u *Usecase) List(ctx, q ListQuery) (ListResult, error)
func (u *Usecase) Get(ctx, id) (Skill, error)
func (u *Usecase) Create(ctx, in CreateInput) (Skill, error)
func (u *Usecase) Patch(ctx, id, patch UpdateDraft) (Skill, error)
func (u *Usecase) Publish(ctx, id) (Skill, error)
func (u *Usecase) ToggleEnabled(ctx, id, enabled) (Skill, error)
func (u *Usecase) Duplicate(ctx, id) (Skill, error)
func (u *Usecase) Delete(ctx, id) error
func (u *Usecase) SearchRuns(ctx, q RunQuery) (RunResult, error)
func (u *Usecase) ListVersions(ctx, q VersionListQuery) (VersionListResult, error)
func (u *Usecase) RollbackVersion(ctx, skillID, versionID) (Skill, error)
func (u *Usecase) UpsertSkillFromDisk(ctx, in DiskSyncInput) (Skill, DiskSyncOutcome, error)
func (u *Usecase) MarkFilesystemMissing(ctx, slug, missing) error
func (u *Usecase) ListEnabledPublishedSkillKeys(ctx) ([]string, error)
func (u *Usecase) ListEnabledPublishedSkillCandidates(ctx) ([]RuntimeCandidate, error)
func (u *Usecase) RecordInvocation(ctx, in InvocationWrite) error
func (u *Usecase) BatchGetSkillGuidance(ctx, slugs) ([]SkillGuidanceEntry, error)
func (u *Usecase) ScoreByEmbedding(ctx, query, candidates) (map[string]float64, error)
func (u *Usecase) InvalidateEmbedCache()
func (u *Usecase) InvalidateEmbedCacheForSlug(slug)
```

### 6.4 运行时策略模型

```go
// internal/biz/skill/skill.go

type RuntimePolicy struct {
    AllowedSlugs            []string
    DeniedSlugs             []string
    AllowedTags             []string
    IntentRoutingEnabled    bool   // 默认 true
    IntentMaxPaths          int    // 默认 3
    MaxSkillsInToolset      int    // 默认 32，上限 256
    EmbeddingScoringEnabled bool   // 默认 false
    EmbeddingScoreWeight    float64 // 默认 0.3，范围 0~1
}

type RuntimeCandidate struct {
    Slug          string
    Name          string
    Description   string
    Tags          []SkillTag
    TaxonomyPaths []string
}

func ParseRuntimePolicy(jsonStr string) (RuntimePolicy, error)
```

### 6.5 导入 DTO

```go
// internal/biz/skill/skill.go

type ImportJob struct {
    JobID           string
    Status          string
    TotalCandidates int
    ConflictGroups  []ConflictGroup
}

type ImportCandidate struct {
    Slug        string
    Name        string
    Description string
    Source      string
    Similarity  *SimilarityMetrics
}

type ConflictGroup struct {
    GroupID    string
    Slug       string
    Candidates []ImportCandidate
    Resolution string
}

type SimilarityMetrics struct {
    NameSimilarity        float64
    DescriptionSimilarity float64
    OverallSimilarity     float64
}
```

### 6.6 SkillSimilarityEngine — 统一相似度引擎

文件：`internal/biz/skill_similarity.go`

统一平台级去重与导入流程的相似度计算，替代原先仅用 Name/Description 2 维 Jaccard 的简单算法。

**4 维相似度计算**：

| 维度 | 权重 | 说明 |
|------|------|------|
| Name | 0.4 | 名称 Jaccard 相似度 |
| Description | 0.25 | 描述 Jaccard 相似度 |
| Body | 0.25 | 正文段落标题 Jaccard 相似度 |
| Tag | 0.1 | 标签 Jaccard 相似度 |

**可选 Embedding 增强**：当注入 `DedupEmbedder` 时，将 Jaccard 分数与 Embedding 余弦相似度按 `embeddingBlendWeight=0.5` 混合。

**阈值常量**：

| 常量 | 值 | 用途 |
|------|-----|------|
| `similarityHighThreshold` | 0.8 | 总分 ≥ 此值 → `high` 风险，建议合并 |
| `similarityNameHighThreshold` | 0.9 | 名称分 ≥ 此值 → 极可能重复 |
| `similarityMediumThreshold` | 0.5 | 总分 ≥ 此值 → `medium` 风险，建议审查 |
| `embeddingBlendWeight` | 0.5 | Embedding 与 Jaccard 混合权重 |

### 6.7 SkillMergeUsecase — 三阶段合并

文件：`internal/biz/skill_merge.go`、`internal/biz/skill_merge_ai_fuser.go`

**三阶段流程**：内容融合 → Gate 验证 → 事务应用

- **阶段 1**（`SkillContentFuser` 接口）：`append` / `ai_fuse` / `manual_pick` 策略；当前实现 `RuleBasedContentFuser`，基于 `##` 段落标题去重合并。
- **阶段 2**：融合后的内容需通过校验（如非空、长度限制等），验证失败则拒绝合并。
- **阶段 3**（`SkillMergeWriter` 接口）：在单个事务内执行 4 步操作：为 target 创建新版本 → 更新 metadata/tags → 转移 source 调用记录 → 废弃源 Skill（状态 → `deprecated`）。

**Data 层实现**：`internal/data/skill_merge.go` — `SkillMergeRepo` 实现 `SkillMergeReader` + `SkillMergeWriter`。

### 6.8 SkillEvolutionOrchestrator — 统一进化编排

文件：`internal/biz/skill_evolution_unified.go`、`internal/biz/skill_evolution_triggers.go`

**EvolutionTrigger 接口**（策略模式）：

| Trigger | 来源 | 检测逻辑 |
|---------|------|----------|
| `PatternTrigger` | 工具调用 Pattern | 从高频工具调用组合中检测新 Skill 需求，返回所有匹配 pattern |
| `HealthTrigger` | 健康指标 | 检测 30d 失败率 > 30% 或 score < 60；依赖 `SkillScorer` 窄接口 |
| `AgentConfigTrigger` | Agent 配置 | 预留扩展点（当前返回 nil） |

**SkillEvolutionOrchestrator**：

- `RegisterTrigger`：**线程安全**（`sync.RWMutex` 保护 `triggers` 切片）
- `CheckAndCreate`：原子化检查 + 创建，解决 TOCTOU 竞态
  - 先调用 `UnifiedEvolutionCheckReader.HasPendingForTarget` 检查
  - 遍历触发器时使用 **快照读取**（`RLock` → copy → `RUnlock`）
  - 每个 trigger 返回 `[]UnifiedEvolutionSuggestion`（支持多 pattern 同时触发）
  - 不存在则创建，DB UNIQUE 约束兜底（多实例并发安全）
  - 重复创建时返回 `nil, nil`（幂等）
  - 含 per-action-type 冷却期检查（F9：仅 `pending`/`approved`/`applied` 活跃状态计入冷却窗口——`rejected`/`expired`/`rolled_back` 不阻塞再次触发，见 `UnifiedEvolutionSuggestion.CountsForCooldown`）
- `Approve` / `Reject`：审批/拒绝进化建议
- `ExpirePending`：批量过期超过 7 天的 pending 建议

**接口拆分**（符合"接口方法 ≤ 5"规范）：

- `UnifiedEvolutionCheckReader`（3 方法）：`HasPendingForTarget` / `GetLatestByTarget` / `GetLatestByTargetAndAction`
- `UnifiedEvolutionQueryReader`（5 方法）：`GetByID` / `ListByTarget` / `CountByTarget` / `ListByTargetAndAction` / `CountByTargetAndAction`（AndAction 变体区分同 target_type 的 L1 proposal 与 L3 agent 建议）
- `UnifiedEvolutionPatternReader`（1 方法）：`GetLatestByPatternHash`（L1 pattern_hash 去重，hash 存于 metadata）
- `UnifiedEvolutionMutationWriter`（5 方法）：`Create` / `UpdateStatus` / `UpdateDraftBody` / `UpdateLifecycleStatus` / `UpdateSandboxResult`
- `UnifiedEvolutionMetadataWriter`（1 方法）：`UpdateMetadataKey`（单键 JSON 合并，如 L3 `pre_apply_snapshot`）
- `UnifiedEvolutionExpirationWriter`（1 方法）：`ExpireOlderThan`

**Data 层实现**：`internal/data/unified_evolution.go` — `UnifiedEvolutionRepo` 同时实现 Reader + Writer，使用 raw SQL + 读写分离；表结构由 `internal/data/sql/unified_evolution.sql` DDL 建立（非 Ent Schema）。

**A6 物理收敛（迁移 20261111）**：L1 `skill_proposals` / L2 `skill_evolution_suggestions` / L3 `evolution_suggestions` 三张 legacy 表已物理删除——迁移逐行 backfill（主键预检幂等）到 `unified_evolution_suggestions` 后 DROP。legacy 专有字段保留在 `metadata` JSON 列：

- L1：`pattern_hash` / `pattern_desc` / `approved_at` / `rejected_by`（status `registered` 原样保留）
- L2：`source_report_ids` / `draft_version_id` / `parent_version_id` / `evolution_reason` / `pre_verify_result` / `rejected_by` / `rejection_reason` / `resolved_at`
- L3：`legacy_type` / `title` / `diff_preview` / `pre_apply_snapshot`

**视图重建层**：biz 层通过转换函数从统一行重建 legacy 视图，对外 proto 契约不变——L1 `skillProposalFromUnified`（[skill_evolution.go](../../internal/biz/skill_evolution.go)）、L2 `unifiedToLegacySuggestionPtr`（[skill_intelligence.go](../../internal/biz/skill_intelligence.go)）、L3 `evolutionViewFromUnified`（[evolution.go](../../internal/biz/evolution.go)）。

**pending 去重索引**：`idx_ues_pending_target` 为 dialect-aware 部分唯一索引——`(target_type, target_id, action_type, COALESCE(json_extract(metadata,'$.pattern_hash'), json_extract(metadata,'$.legacy_type'), '')) WHERE status='pending'`（Postgres 使用 `metadata::jsonb->>` 变体），保留 legacy 去重语义：L1 按 pattern_hash、L3 按 legacy_type、health/curator 按 (target, action)。

**EvolutionCoordinator 状态**：已随 A6 物理收敛删除（`internal/biz/evolution_coordinator.go` 连同 `SetCoordinator` 委托与 fallback 逻辑一并移除）。跨流水线去重统一由 `SkillEvolutionOrchestrator` 统一 pending 检查 + trigger 内去重 + DB 唯一索引（`idx_ues_pending_target`）承担。

**SkillDedupUsecase.MergeSkills**：已标记 `Deprecated`，应使用 `SkillMergeUsecase.Merge`。Service 层不再回退到旧合并。

**SkillDedupUsecase.DetectDuplicateGroups**：添加 **10 分钟 TTL 内存缓存**，避免每次 API 调用全量 O(n²) 扫描。外部可通过 `InvalidateDedupCache()` 手动失效。

### 6.9 ScoreSkill 四维权重修复

文件：`internal/biz/skill_intelligence.go`

| 维度 | 权重 | 启用条件 |
|------|------|----------|
| SuccessRate | 0.4 | 始终启用 |
| Duration | 0.25 | 始终启用 |
| Token | 0.2 | `AvgTokenUsage > 0` 时启用 |
| Feedback | 0.15 | `FeedbackScore > 0` 时启用 |

**Token 归一化**：`normalizeTokenUsage(avgTokenUsage)` — 以 `baselineTokens=2000` 为基准，计算 `1 - avg/baseline`，值域 [0, 1]。

**Feedback 启发式计算**（标注 `TEMPORARY`，待接入真实用户反馈）：基于 SuccessRate/Duration/TokenUsage 启发式估算。

### 6.10 SkillFilesystem 端口

接口定义在 `internal/biz/skill/skill.go`（`SkillFilesystem`），实现在 `internal/skill/storage/filesystem.go`。

Service 层通过 `SkillFilesystem` 端口访问文件系统，不直接操作 `os` 包。Wire 绑定：`ProvideSkillResolveRootFn`（提供 `func(ctx) string`）+ `storage.NewSkillFilesystem`。

端口方法：
- `ResolveRoot(ctx)` — 解析存储根目录（动态：优先 SystemSettingRepo.root_directory，回落环境变量/默认路径）
- `CreateSkillDir(slug, body)` — 创建 skill 目录 + SKILL.md
- `ListFiles(dir)` — 遍历目录返回文件列表
- `ReadFile(dir, relPath)` — 读取文件内容（含安全检查 + 大小限制）
- `WriteFile(dir, relPath, content)` — 写入文件
- `DeleteFile(dir, relPath)` — 删除文件
- `RootAccessible(ctx)` — 检查根目录是否可访问
- `DirExists(dir)` — 检查目录是否存在
- `SafeFilePath(dir, relPath)` — 路径安全检查（防目录穿越）

---

## 七、运行时层

### 7.1 装配入口

文件：`internal/agent/trpc_build.go` → `buildSkillDeps`

流程：
1. `ListEnabledPublishedSkillKeys()` 确认存在已启用 + 已发布 Skill
2. 优先 `SkillDBRepo`（`DBRepositoryAdapter`）；否则 `FSRepositoryAdapter`
3. `skillruntime.NewAgentVisibilityFilter(SkillUC, ag.Settings)` — Layer A/B，按 invocation 读取 turn query
4. `CodeExecutor`（local / docker，`CODE_EXECUTOR_BACKEND`；产出物经 `artifact_executor.go`）
5. `WithSkills` + `WithSkillFilter` + `WithSkillToolProfile(SkillToolProfileFull)`

Turn query 注入：`internal/service/trpc_turn.go` · `internal/team/runner_team_trpc.go` → `skillruntime.RunOptionWithTurnQuery`

### 7.2 运行时路由

文件：`internal/tools/skillruntime/resolve.go`

两级筛选：

**Layer A**（`applyLayerA`）：
- 按 `RuntimePolicy.AllowedSlugs` / `DeniedSlugs` 过滤

**Layer B**（`ResolveSkillSlugsDetailed`）：
- `skillrouter.DetectIntentPaths(query, maxPaths)` → 分类路径关键词匹配
- `filterByIntentPathsWithReasons()` → 按分类路径缩小候选
- `filterByAllTagsWithReasons()` → 按 `AllowedTags` + `ExtractTagHints()` 过滤
- `scoreCandidatesWithReasons()` → 按分类路径匹配度评分
- **Embedding 语义精排**（可选）：`SkillUsecase.ScoreByEmbedding(query, candidates)` → 余弦相似度融合评分
- 排序后取 `MaxSkillsInToolset`，返回 `ResolveResult{Slugs, Reasons}`

### 7.3 意图路由与分类

文件：`internal/tools/skillrouter/`

- `detect.go`：`DetectIntentPaths(userQuery, maxPaths)` → 关键词 → 分类路径
- `taxonomy.go`：`TaxonomyLeaves` 定义 + `ExtractTagHints(userQuery)` 提取 `file_type:*` / `domain:*` 提示

### 7.4 trpc-agent-go 桥接

文件：`internal/skill/trpc/`

- `repository.go`：`FSRepositoryAdapter` — 磁盘 FS → `trpcskill.Repository`
- `db_repository.go`：`DBRepositoryAdapter` — DB + TTL 缓存 → `trpcskill.Repository`
- `filter.go`：`NewFilteredRepository(base, allowedSlugs)` → `trpcskill.ContextRepository`
- `tools.go`：`BuildSkillTools()` 产出 4 个内置 Skill 工具（Load / Run / ListDocs / SelectDocs）
- `executor.go`：`CodeExecutor` 适配（local / docker）；`artifact_executor.go`：产出物 `WrapWithArtifactSave`

### 7.5 运行时策略存储

存储在 `agent_runtime_settings.skill_runtime_json`，字段：
- `allowed_slugs`、`denied_slugs`、`allowed_tags`
- `intent_routing_enabled`（默认 true）
- `intent_max_paths`（默认 3）
- `max_skills_in_toolset`（默认 32，上限 256）
- `embedding_scoring_enabled`（默认 false）— 启用 embedding 语义精排
- `embedding_score_weight`（默认 0.3，范围 0~1）— embedding 分权重

### 7.6 Prompt 注入（方式 C）

文件：`internal/agent/skill_guidance_inject.go`

在 `productCallbackChain` 中注册 `newSkillGuidanceBeforeHook`（priority=5），仅 `SkillsUseFullProfile` 模式下启用。

流程：
1. `ResolveSkillSlugsDetailed` 获取当前 turn 的 skill slugs
2. `BatchGetSkillGuidance(slugs)` 批量获取 skill markdown（2 条 SQL：按 skill_key 查 Skill + 按 skill_id 查最新 Version）
3. `manifest.Parse` 解析 frontmatter → `render.SkillGuidance` 渲染指导内容
4. 拼接为 system message 注入 `args.Request.Messages` 头部
5. 截断保护：`maxSkillGuidanceChars=4000`；`written==0` 时不注入

### 7.7 Embedding 语义精排

文件：`internal/biz/skill/skill.go`（`ScoreByEmbedding`/`refreshEmbedCache`/`cosineSimilarity32`）

评分融合公式：`final_score = keyword_score + cosine_similarity × 1000 × embedding_score_weight`

- 默认 `embedding_score_weight=0.3`，最大 embedding 贡献 300 分
- 低于 taxonomy 精确匹配（1000）和部分匹配（400），高于关键词匹配（100）
- 仅在 `embedding_scoring_enabled: true` 时启用
- 内存缓存：`embedCache map[string]embedEntry`，按 slug 缓存 embedding（TTL 30min）
- 缓存失效：Publish/ToggleEnabled/Delete/Duplicate 时 `InvalidateEmbedCacheForSlug(slug)`
- 优雅降级：embedding 不可用时回退到纯关键词评分，`event.SysLogWarn` 记录失败

### 7.8 Skill Tool 产出

```go
// internal/skill/trpc/tools.go
func BuildSkillTools(cfg SkillToolsetConfig) []trpctool.Tool {
    // LoadTool: 加载 Skill 正文
    // RunTool: 执行 Skill 代码（需 CodeExecutor）
    // ListDocsTool: 列出 Skill 文档
    // SelectDocsTool: 选择 Skill 文档片段
}
```

### 7.9 运行时行为产品结论

| 结论 | 说明 |
|------|------|
| 挂载条件 | 仅 **已发布且已启用** 的平台 Skill 参与运行时 |
| Agent 策略 | `agent_runtime_settings.skill_runtime_json`：allow/deny slug、标签、意图收窄、数量上限 |
| 按回合收窄 | 用户本轮输入经 RuntimeState 传入，驱动 Layer B 意图/标签路由 |
| Team | 各成员 Agent **独立**构建，各自使用成员 Agent 的 `skill_runtime_json` |
| Prompt 注入方式 C | ✅ 已实现：BeforeModelHook + `BatchGetSkillGuidance` 批量获取 + 截断 + 空 guidance 防护 |
| Embedding 语义精排 | ✅ 已实现：`SkillEmbedder` + `ScoreByEmbedding` + 评分融合 + 优雅降级 |
| Preview 选中原因 | ✅ 已实现：`ResolveSkillSlugsDetailed` 返回 `Reasons map[string]string` + `agent_id` 关联 |
| 待实现 | Budget 中间件（token 上限裁剪）、Skill 依赖/冲突表 |

---

## 八、磁盘监听与同步

### 8.1 文件系统监听

文件：`internal/skill/watch/runner.go`

- 启动全量扫描 + `fsnotify` 增量监听（debounce 2s）
- 环境变量 `SKILL_WATCH_DISABLED=1` 可关闭
- 支持 `event.Bus` 集成，同步成功后发布 `skill.reload` 事件
- 磁盘目录缺失时调用 `MarkSkillFilesystemMissing(slug, true)` 标记，恢复时清除
- `reconcile.go`：定时 reconcile ticker（默认 5min，环境变量 `SKILL_FS_RECONCILE_INTERVAL`，`0/off` 关闭）

### 8.2 存储根解析

文件：`internal/skill/storage/root.go`

解析优先级（自上而下短路命中）：
1. `SKILL_ROOT` 环境变量
2. `SKILL_STORAGE_ROOT` 环境变量
3. `filepath.Join(Resolved(work_directory), "skills")`
4. 操作系统默认路径（`%AppData%\Aranea\skills` 等）

`ResolveRootWithPlatform(rootDirectory)` 实现完整回落链路。`SkillService.resolvedStorageRoot()`、`watch.Runner.resolveRoot()`、`importer.Engine.resolveRoot()` 均通过 `SystemSettingRepo.Get()` 读取 `work_directory` 后调用 `ResolveRootWithPlatform`。

### 8.3 磁盘同步行为

**拷贝进入（磁盘 → 登记）**

1. `watch.Runner` 启动全量 scan + `fsnotify` debounce（2s）。
2. 校验通过后 `UpsertSkillFromDisk`：新建为 draft；已存在同 slug 则更新 metadata/正文并清除 `filesystem_missing`。
3. 写入 `metadata_json.sync_origin=filesystem`。
4. 发布 Monitor 事件 `skill.filesystem.imported` / `skill.filesystem.updated`（经 EventBus + `monitor_events`）。
5. **不**自动 publish / enable。

**进化版本保护（F3 / P-evo-1）**：DB 最新版本为进化成果（`evolution_reason` 非空）且磁盘内容与其不一致时，认定磁盘陈旧——**不**创建新版本、**不**回退 draft，提交后以 DB 为准刷新磁盘 SKILL.md（`UpsertSkillFromDisk` 内 Warn 日志 + 落盘）。对称地，`CreateSkillVersion` 在事务提交后同步落盘（`syncSkillBodyToDisk`），保证进化 pipeline（DB 为真相）与 watcher（磁盘为真相）两个真相源一致，防止 watcher 用陈旧磁盘内容回滚进化成果。

**删除离开（磁盘 → 标记）**

1. 目录被外部删除或移走后，watch 将对应 slug 标记 **`filesystem_missing=true`**（DB 软删记录保留）。
2. 发布 `skill.filesystem.missing`；恢复目录并校验通过后发布 `skill.filesystem.recovered` 并清除标记。
3. 平台 UI「删除 Skill」为 **软删 DB**，**不**删除磁盘，**不**触发磁盘缺失告警。

**通知链路**

| 事件 | 触发 | 落点 |
|------|------|------|
| `skill.filesystem.imported` | 磁盘新建登记 | Monitor Events + EventBus |
| `skill.filesystem.updated` | 磁盘正文/metadata 变更 | 同上 |
| `skill.filesystem.missing` | 目录删除 | 同上 + Skill 页 Banner |
| `skill.filesystem.recovered` | 目录恢复 | 同上 |
| `skill.filesystem.rejected` | 校验失败（含 slug 目录名不一致） | 运行记录 + Monitor |

**D5 行为**

| 能力 | 说明 |
|------|------|
| Reconcile | 定时 scan 磁盘 + 对 DB 已登记 slug 补打 `filesystem_missing` |
| 回退 draft | 已发布 Skill 磁盘正文变更 → `draft` + `enabled=false` |
| 相似度 warn | 新磁盘 Skill 与已有 **同名** → `skill.filesystem.similarity_warn`（异步，非 LLM） |
| 告警 | Monitor 规则 `skill.filesystem_missing_count` ≥ threshold → Webhook/Channel |

---

## 九、ZIP 导入

### 9.1 导入引擎

文件：`internal/skill/importer/`

子包：
- `engine.go`：导入主流程（解压 → 校验 → 相似度检测 → 冲突分组）
- `validate.go`：ZIP 结构与 frontmatter 校验
- `helpers.go`：辅助函数
- `chat.go`：LLM 相似度检测与炼化
- `errors.go`：错误类型

### 9.2 HTTP 路由

业务逻辑在 `internal/service/skill_import.go`；multipart 挂载见 §5.2。

### 9.3 导入状态机

| 状态 | 来源 | 行为 |
|------|------|------|
| `idle` | 初始 | 展示上传区 |
| `uploading` | 文件提交中 | 禁用关闭和重复提交 |
| `processing` | 已获得 `job_id`，轮询中 | 每 1.5s 轮询一次 |
| `completed.pass` | 所有候选均无结构、名称、相似冲突 | 列表中每个上传 Skill 显示对号 |
| `completed.warn` | 存在模型判定相似度 `>= 0.2` 的冲突组 | 展示无冲突列表 + 冲突组 |
| `completed.block` | 存在结构错误、规范错误、名称重复等阻塞错误 | 展示错误，禁止入库 |
| `failed` | 导入任务失败 | 展示失败原因，允许重试 |

---

## 十、Web 前端设计

### 10.1 文件结构（与代码一致）

```
web/src/
├── pages/
│   ├── SkillsPage.vue              # 列表 + SkillUploadPlaceholder + SkillEditorDialog
│   ├── SkillDetailPage.vue         # Skill 详情页
│   └── SkillRunsPage.vue           # 运行记录页
├── pages/agent-settings/
│   └── AgentSettingsSkillsTab.vue  # skill_runtime_json 配置
├── components/skills/
│   ├── SkillTable.vue · SkillFilterBar.vue · SkillStatsStrip.vue
│   ├── SkillEditorDialog.vue · SkillUploadPlaceholder.vue
│   ├── SkillDeleteDialog.vue · SkillRunsTable.vue · SkillPagination.vue
│   ├── SkillFilesystemAlertBanner.vue · SkillHealthCard.vue
│   └── skillTableUi.ts
├── features/skills/
│   ├── api.ts · types.ts
│   ├── useSkillsPage.ts · useSkillDetailPage.ts · useSkillRunsPage.ts
│   └── useExperienceReportListPage.ts
└── stores/skills/index.ts
```

路由：`/skills` · `/skills/runs`（见 `frontend-pages.md` §4.6）

**SkillEditorDialog.vue** — 全屏 Dialog，左侧文件树 + 右侧内容编辑（`ListSkillFiles` / `GetSkillFile` / `UpdateSkillFile`）。

**SkillUploadPlaceholder.vue** — 上传 zip、轮询导入任务、冲突组炼化（调用 `features/skills/api` import 端点）。

### 10.2 Quasar 组件清单

| 页面 | 主要组件 |
|------|----------|
| Skill 列表 | `QPage`、`QTable`、`QToggle`、`QChip`、`QBtn`、`QDialog`、`QPagination`、`QSelect`、`QInput` |
| 上传导入 | `QDialog`、`QUploader` 或拖拽 `QCard`、`QLinearProgress`、`QBanner`、检查结果列表、冲突组 `QCard` |
| 冲突组炼化 | 冲突组内 `QBtn`、可选 `QSelect`（provider/model）、`QInput type=textarea` 或 Markdown 编辑器 |
| 编辑页 | `QForm`、`QInput`、`QSelect`、`QExpansionItem`、`QBtn` |
| 运行记录 | `QTable`、`QBadge`、`QTooltip`、`QDialog`、`QDate` |

### 10.3 API

```typescript
export async function listSkills(query: SkillListQuery): Promise<PaginatedResponse<Skill>>
export async function getSkill(id: string): Promise<{ skill: Skill; bodyMarkdown: string }>
export async function getSkillFilesystemHealth(): Promise<SkillFilesystemHealth>
export async function createSkill(payload: CreateSkillRequest): Promise<Skill>
export async function updateSkill(id: string, payload: UpdateSkillRequest): Promise<Skill>
export async function publishSkill(id: string): Promise<Skill>
export async function toggleSkillEnabled(id: string, enabled: boolean): Promise<Skill>
export async function duplicateSkill(id: string): Promise<Skill>
export async function deleteSkill(id: string): Promise<void>
export async function listSkillFiles(id: string): Promise<SkillFile[]>
export async function readSkillFile(id: string, path: string): Promise<SkillFileContent>
export async function updateSkillFile(id: string, path: string, content: string): Promise<SkillFileContent>
export async function deleteSkillFile(id: string, path: string): Promise<void>
export async function previewSkillRuntime(id: string): Promise<{ preview: string }>
export async function listSkillRuns(query: SkillRunQuery): Promise<PaginatedResponse<SkillInvocation>>
export async function getSkillHealth(skillId: string): Promise<SkillHealthMetric>
export async function getSkillVersions(id: string, page: number, pageSize: number): Promise<PaginatedResponse<unknown>>
export async function rollbackSkillVersion(id: string, versionId: string): Promise<Skill>
export async function uploadSkillZip(file: File): Promise<{ job_id: string }>
export async function getSkillImportJob(jobId: string): Promise<SkillImportJob>
export async function applySkillImport(jobId: string, decisions: SkillImportDecision[]): Promise<SkillImportApplyResult>
export async function refineSkillConflictGroup(jobId: string, groupId: string, payload): Promise<SkillRefineResult>
```

---

## 十一、Service 层与 Wire 注入

### 11.1 Service 层

文件：`internal/service/skill.go`（19 方法）+ `internal/service/skill_import.go`（4 方法）+ `internal/service/skill_import_http.go`（multipart 挂载）

薄适配层，职责：
- Proto Request → Biz DTO 转换
- `resolvedStorageRoot()`：通过 `SystemSettingRepo.Get()` 读取 `work_directory` → `storage.ResolveRootWithPlatform()`
- `safeSkillFilePath()`：路径安全校验（禁止 `..` 跳出 Skill 根）

### 11.2 Wire 注入

已有，无需新增。Skill 相关依赖通过 `wire.NewSet` 注入：
- `SkillRepo` → `SkillUsecase` → `SkillService`
- `SkillUsecase` → `buildSkillDeps`（Agent 构建）
- `ProvideSkillResolveRootFn` + `storage.NewSkillFilesystem`，动态解析 root_directory

---

## 十二、权限、可见性与冲突

### 12.1 可见性层级

`system` / `workspace` / `agent` / `private` / `public`——列表接口不返回无 `can_view` 的行。

### 12.2 RBAC 权限

- `requireAdminAccess`（biz 层写操作门控）
- `applySkillPermission`（biz 层读操作权限掩码）
- 未认证返回零权限

### 12.3 依赖与冲突

- **依赖**：`required_skill`、`optional_skill`、`tool_capability`、`runtime_feature`。
- **冲突**：导入 slug 冲突、语义相似（导入流水线已实现相似度与炼化）、运行时互斥策略。
- **分级**：`info` / `warn` / `block`——与需求文档的 pass/warn/block 一致。

---

## 十三、Skill 召回管线

本节描述端到端心智模型；**层 A（Agent 策略）与层 B（意图→候选收窄）已在运行时落地**，embedding 语义精排已实现。

### 13.1 详细分类树（示例）

意图可挂载到叶子路径（存储侧推荐写入 Skill `metadata_json.taxonomy_paths` 数组）：

- **数据获取与集成**
  - **内部数据源**
    - **文件系统读取（读取表格）** → 示例 Skill：`excel_reader`（读 xlsx）
- **分析与推理**
  - **自然语言理解（情感分析）** → 示例：`sentiment_analysis`
- **交互与执行**
  - **消息发送（发邮件）** → 示例：`email_sender`

### 13.2 多维标签

标签推荐落在 **`SkillTag.name`**，使用形如 `dim:value` 的扁平 token，例如：

- `file_type:xlsx`
- `domain:sales`

运行时：**同类约束为合取（AND）**——候选 Skill 必须同时具备本轮所需的每一个标签 token。

### 13.3 索引收窄 → 标签过滤 → 语义精排

1. **意图分类**：从用户话术中命中多条意图路径。
2. **候选召回**：在多条路径下做 **OR** 并集。
3. **标签过滤**：例如要求 `file_type:xlsx` 且 `domain:sales`，将候选 **AND** 缩至更小集合。
4. **语义排序**：对余下的少量 Skill 做 embedding / rerank，选出最优组合。

### 13.4 Skill `metadata_json` 约定

```json
{
  "taxonomy_paths": [
    "数据获取与集成/内部数据源/文件系统读取（读取表格）"
  ],
  "tags": [{ "name": "file_type:xlsx", "source": "user" }]
}
```

### 13.5 代码锚点

- `internal/tools/skillruntime`：`SkillToolsetOptions`、`ResolveSkillSlugs`、`NewAgentVisibilityFilter`
- `internal/tools/skillrouter`：`DetectIntentPaths`、`ExtractTagHints`、`TaxonomyLeaves`
- `internal/biz/skill/skill.go`：`RuntimePolicy`、`RuntimeCandidate`、`ParseRuntimePolicy`
- `internal/data/skill.go`：`ListEnabledPublishedSkillCandidates`
- `internal/agent/trpc_build.go`：`buildSkillDeps`

---

## 十四、可观测性

建议事件或 span：

- `skill.activated`、`skill.used`、`skill.failed`
- Span：`skills.assemble`、`skills.registry.search`、`skills.backend.load`、`skills.backend.render`、`skills.usage.record`

属性：`skill.id`、`skill.slug`、`skill.version`、`agent.id`、`session.id`、`activation.source`、`token.cost`。

磁盘同步建议补充：`skill.fs.scan`、`skill.fs.synced`、`skill.fs.error`（日志或指标）。

---

## 十五、Go 包布局

```text
internal/
├── biz/
│   ├── skill/                  # 用例子包
│   │   ├── skill.go            # 用例与端口（SkillReader/SkillWriter/Repo 接口、Usecase、DTO、SkillFilesystem、SkillEmbedder）
│   │   └── skill_test.go       # 单元测试
│   ├── skill.go                # 类型别名（type alias）+ 常量 + 构造函数
│   ├── skill_similarity.go     # 统一相似度引擎（4 维 + 可选 Embedding）
│   ├── skill_merge.go          # 三阶段合并 Usecase
│   ├── skill_merge_ai_fuser.go # 基于规则的内容融合器
│   ├── skill_evolution_unified.go  # 统一进化编排器 + UnifiedEvolutionSuggestion + Reader/Writer 接口
│   ├── skill_evolution_triggers.go # EvolutionTrigger 策略（Pattern/Health/AgentConfig）+ SkillScorer 窄接口
│   ├── skill_intelligence.go   # SkillIntelligenceUsecase（ScoreSkill 四维权重 + L2 视图重建 unifiedToLegacySuggestionPtr，A6）
│   ├── skill_evolution.go      # SkillEvolutionUsecase + L1 视图重建 skillProposalFromUnified（A6）
│   ├── skill_dedup.go          # SkillDedupUsecase（DetectDuplicateGroups 带 10min TTL 缓存）；MergeSkills Deprecated
│   ├── skill_health.go         # SkillHealthUsecase
│   ├── skill_scoring.go        # SkillScorer 窄接口
│   ├── skill_report.go         # 报告
│   ├── skill_load_mode.go      # 加载模式
│   └── skill_invocation_stats.go # 调用统计
├── data/
│   ├── skill.go                # Ent 仓储与聚合
│   ├── skill_merge.go          # 合并 Data 层（事务内 4 步操作）
│   ├── skill_dedup.go          # 去重 Data 层（含 SkillSimilarityEngine 集成）
│   ├── skill_intelligence.go   # 健康指标聚合（含 AvgTokenUsage/FeedbackScore）
│   ├── skill_health.go         # 健康 Data 层
│   ├── skill_invocation_stats.go # 调用统计 Data 层
│   ├── skill_import_job.go     # 导入任务 Data 层
│   ├── skill_evolution_schema.go # legacy `skill_proposals` DDL（仅作迁移 20261111 backfill 来源，backfill 后 DROP；A6 起不承载读写）
│   ├── unified_evolution.go    # 统一进化 Data 层（raw SQL + 读写分离；A6 起承载全部四类建议读写，legacy skill_evolution.go / skill_evolution_suggestion.go 已删除）
│   └── unified_evolution_schema.go # 统一进化 Schema
├── skill/
│   ├── importer/               # ZIP 导入引擎（engine / validate / helpers / chat / errors）
│   ├── watch/                  # Skill 根目录监听与磁盘同步（runner / reporter / reconcile）
│   ├── storage/                # Skill 存储根解析 + SkillFilesystem 实现（root / filesystem）
│   ├── manifest/               # frontmatter / skill.json 解析与校验
│   ├── render/                 # prompt 块渲染、截断策略
│   ├── trpc/                   # trpc-agent-go 桥接层（repository / db_repository / tools / executor / filter / artifact_executor）
│   ├── fs_registrar.go         # 文件系统登记
│   └── auto_creator.go         # 自动创建
├── tools/
│   ├── skillruntime/           # 运行时装配入口（toolset / resolve / filter / runtime）
│   ├── skillrouter/            # 意图路由与分类（detect / taxonomy）
│   ├── skills_butler/          # Skill 管家（registry / recommend / analyze / optimize / evolve）
│   └── skillrecommend/         # Skill 推荐（rank / rank_feedback / health_provider）
├── service/
│   ├── skill.go                # 薄适配（19 RPC）
│   ├── skill_import.go         # 导入用例桥接（4 RPC）
│   ├── skill_import_http.go    # multipart POST /v1/skills/import
│   ├── skill_intelligence.go   # 智能分析服务
│   ├── skill_evolution.go      # 进化服务
│   ├── skill_evolution_suggestion.go # 进化建议服务
│   ├── skill_curator.go        # 策展服务
│   ├── skill_dedup.go          # 去重服务
│   ├── skill_health_metrics_adapter.go # 健康指标适配器
│   └── skills_butler_adapter.go # 管家适配器
└── agent/
    ├── trpc_build.go           # Agent 构建中 Skill 装配（buildSkillDeps）
    └── skill_guidance_inject.go # Prompt 注入方式 C（BeforeModelHook + BatchGetSkillGuidance）
```

---

## 附录 A · 与代码模块映射

```text
api/kratos/skill/v1/skill.proto          → HTTP 契约（22 RPC）
internal/service/skill.go                → 适配层（19 RPC）
internal/service/skill_import.go         → 导入 biz 桥接（4 RPC）
internal/service/skill_import_http.go    → multipart POST /v1/skills/import
internal/biz/skill/skill.go              → 用例与 SkillReader/SkillWriter/Repo 端口
internal/biz/skill.go                    → 类型别名 + 常量 + 构造函数
internal/biz/skill_similarity.go         → 统一相似度引擎（4 维 + 可选 Embedding）
internal/biz/skill_merge.go              → 三阶段合并 Usecase
internal/biz/skill_merge_ai_fuser.go     → 基于规则的内容融合器
internal/biz/skill_evolution_unified.go  → 统一进化编排器 + UnifiedEvolutionSuggestion + Reader/Writer 接口
internal/biz/skill_evolution_triggers.go → EvolutionTrigger 策略（Pattern/Health/AgentConfig）+ SkillScorer 窄接口
internal/biz/skill_intelligence.go       → SkillIntelligenceUsecase（ScoreSkill 四维权重 + L2 视图重建，A6）
internal/biz/skill_evolution.go          → SkillEvolutionUsecase + L1 视图重建（A6）
internal/biz/skill_dedup.go              → SkillDedupUsecase（DetectDuplicateGroups 带 10min TTL 缓存）；MergeSkills Deprecated
internal/data/skill.go                   → Ent 仓储与聚合
internal/data/skill_merge.go             → 合并 Data 层（事务内 4 步操作）
internal/data/skill_dedup.go             → 去重 Data 层（含 SkillSimilarityEngine 集成）
internal/data/skill_intelligence.go      → 健康指标聚合（含 AvgTokenUsage/FeedbackScore）
internal/data/unified_evolution.go       → 统一进化 Data 层（raw SQL + 读写分离）
internal/skill/importer/*                → ZIP 导入领域实现
internal/skill/watch/*                   → 磁盘监听与幂等 upsert
internal/skill/storage/*                 → Skill 存储根解析 + SkillFilesystem 实现
internal/skill/manifest/*                → frontmatter 解析与校验
internal/skill/render/*                  → prompt 渲染与截断
internal/skill/trpc/*                    → trpc-agent-go Repository 桥接
internal/tools/skillruntime/*            → ResolveSkillSlugs + AgentVisibilityFilter
internal/tools/skillrouter/*             → 意图路径与标签 hint（层 B）
internal/tools/skills_butler/*           → Skill 管家（推荐/分析/优化/进化）
internal/tools/skillrecommend/*          → Skill 推荐（rank/feedback/health）
internal/agent/trpc_build.go             → buildSkillDeps
internal/agent/skill_guidance_inject.go  → Prompt 注入方式 C
api/kratos/system_setting/v1/system_setting.proto → work_directory
```

---

## 附录 B · 规划表 DDL（参考，落地前须评审前缀与迁移）

```sql
-- skill 本体增量列示例（名称仅供评审）
ALTER TABLE skill ADD COLUMN kind TEXT NOT NULL DEFAULT 'markdown';
ALTER TABLE skill ADD COLUMN risk_level TEXT NOT NULL DEFAULT 'low';
ALTER TABLE skill ADD COLUMN entry_path TEXT NOT NULL DEFAULT 'SKILL.md';
ALTER TABLE skill ADD COLUMN runtime_status TEXT NOT NULL DEFAULT 'catalog_only';
ALTER TABLE skill ADD COLUMN current_version_id TEXT NOT NULL DEFAULT '';
ALTER TABLE skill ADD COLUMN visibility TEXT NOT NULL DEFAULT 'workspace';

-- skill_version 增量列示例
ALTER TABLE skill_version ADD COLUMN manifest_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE skill_version ADD COLUMN file_manifest_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE skill_version ADD COLUMN published_at TEXT NOT NULL DEFAULT '';
ALTER TABLE skill_version ADD COLUMN validation_status TEXT NOT NULL DEFAULT '';

-- skill_invocation 增量列示例
ALTER TABLE skill_invocation ADD COLUMN activation_id TEXT NOT NULL DEFAULT '';
ALTER TABLE skill_invocation ADD COLUMN source TEXT NOT NULL DEFAULT 'runtime';
ALTER TABLE skill_invocation ADD COLUMN message_id TEXT NOT NULL DEFAULT '';

-- agent_runtime_settings：Skill 运行时策略 JSON
ALTER TABLE agent_runtime_settings ADD COLUMN skill_runtime_json TEXT NOT NULL DEFAULT '{}';
```

关系表 `skill_permissions` / `skill_dependencies` / `skill_conflicts` 及索引设计需要时从 Git 历史恢复全文 DDL。

---

## 附录 C · 落地原则

1. Skill 是知识/流程/提示包，Tool 是可执行函数；协作靠 Runtime Prompt + usage 关联。
2. **不得破坏**已安装 Skill 的存储目录与版本引用路径。
3. 运行时激活 **可解释**：每个 Skill 有选中原因与 token 估计。
4. Prompt 渲染 **可截断、可测试**，禁止无界注入大文件。
5. 副作用默认只走 Tool 或显式审批路径。
6. **Skill 物理根与系统设置工作目录一致可追溯**：解析规则唯一；变更工作目录须有迁移或再索引策略。
7. **磁盘增量同步须幂等、可观测**，且不得跨越 Skill 根路径访问文件系统。

---

*文档版本：5.0 — 三件套重组：整合两个子模块结构；迁入数据模型/API 契约/Quasar 组件清单（从需求文档）；迁出演进路线/测试关注点（到开发计划）；RPC 数量对齐代码现状（22 RPC）（2026-06-17）。*
