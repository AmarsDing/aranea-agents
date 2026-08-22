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
| `triggers[]` | 确定性触发词（P1-3）：命中用户输入时路由层强制 preload，不依赖模型自觉加载；CJK 子串匹配、ASCII 词边界匹配 |
| `kind` | `markdown` \| `prompt_pack` \| `workflow` \| `tool_backed` |
| `entry` | 默认 `SKILL.md` |
| `requires[]` / `conflicts[]` | 依赖与冲突 |
| `risk_level` | 策略中间件使用 |

**来源优先级**：`skill.json` / `manifest.json` → `SKILL.md` frontmatter → 导入 API 附加 metadata → 目录名推断。

### 4.5 `skill_tags` 标签字典表

标签字典是「治理表」而非「存储表」——标签的真实存储仍在各 Skill 的 `metadata_json.tags` 数组；`skill_tags` 表只承担预建规范名、改名合并与孤儿治理的锚点角色。使用计数不落库，List 时实时聚合 `skill.metadata_json`，保证强一致、避免双写漂移。

| 列 | 类型 | 说明 |
|----|------|------|
| `id` | string(256) | `skilltag_<unixnano>`，不可变 |
| `name` | string(256) | 规范标签 token，全表唯一；小写，匹配 `[a-z0-9][a-z0-9_-]*(:suffix)?` |
| `dimension` | string(128) | `:` 前缀维度（如 `file_type` / `domain`），无前缀为空串；UI 分组依据，带索引 |
| `source` | string(32) | `system`（内置种子）\| `user`（管理员预建）；运行时聚合出的未收录标签以 `orphan` 出现在 List 结果中（不落库） |
| `created_at` / `updated_at` | string | RFC3339 |

设计要点：

- **唯一约束在 `name`**：改名到已存在的目标 = 删除源行（等价合并），不会产生重复行。
- **孤儿标签不落地**：List 时将「使用中但未收录」的标签以 `source=orphan` 合成进结果集，供 UI 提示治理；收录即以其名预建一行。
- **重写走事务**：Rename/Delete 在 `Data.ExecInTx` 内先改字典行，再扫描重写所有引用该标签的 Skill `metadata_json`（仅改 `tags` 键，其余键原样保留），返回重写条数。
- **缓存失效**：biz 层在 Rename/Delete 成功后调用 `InvalidateEmbedCache()` + `invalidateDedupCache()`——`skillCorpusText` 含 tags，向量与去重指纹必须重算。

> **字典模式（通用约定）**：后续开发中，凡「取值集合有限、需跨实体复用、需治理冗余写法」的字段（如标签、分类、维度枚举），优先采用本字典模式：真实数据留在宿主 JSON 列，字典表只存规范值 + 治理元数据，计数实时聚合，改名/删除走事务重写。新增此类字段前应复用或扩展既有字典，避免再建一套。

---

## 五、API 契约

### 5.1 Proto 层（26 RPC）

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
  rpc ImportSkillZip(ImportSkillZipRequest) returns (ImportSkillZipResponse) {
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
  // 标签字典：独立前缀 /v1/skill-tags，避免被 /v1/skills/{id} 路由吞掉。
  rpc ListSkillTags(google.protobuf.Empty) returns (ListSkillTagsResponse) {
    option (google.api.http) = { get: "/v1/skill-tags" };
  }
  rpc CreateSkillTag(CreateSkillTagRequest) returns (SkillTagInfo) {
    option (google.api.http) = { post: "/v1/skill-tags" body: "*" };
  }
  rpc RenameSkillTag(RenameSkillTagRequest) returns (RenameSkillTagResponse) {
    option (google.api.http) = { post: "/v1/skill-tags:rename" body: "*" };
  }
  rpc DeleteSkillTag(DeleteSkillTagRequest) returns (DeleteSkillTagResponse) {
    option (google.api.http) = { delete: "/v1/skill-tags/{name}" };
  }
}
```

### 5.2 ZIP 导入 HTTP 端点

由 `SkillService.ImportSkillZip` 标准 proto HTTP 绑定（`POST /v1/skills/import`）。请求体为 JSON `{ "file": "<base64>", "filename": "x.zip" }`；兼容 `multipart/form-data` 字段 `file`（Kratos `RequestDecoder`，非 `srv.Route` 旁路）。

| 端点 | 方法 | 用途 |
|------|------|------|
| `/v1/skills/import` | POST | 上传 ZIP（`ImportSkillZip` RPC） |
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
| `POST` | `/v1/skills/:id/publish` | `PublishSkillRequest` | 发布并触发校验；**发布即启用**（P2-7a，自动 `enabled=true`，消除「发布→再点启用」两步操作） |
| `PATCH` | `/v1/skills/:id/enabled` | `ToggleSkillEnabledRequest` | 启用 / 停用 |
| `DELETE` | `/v1/skills/:id` | 无 | 软删 |
| `POST` | `/v1/skills/:id/duplicate` | 无 | 复制为新草稿 |

> 发布校验（`evaluatePublishValidation`）：结构缺失（name/description/body 为空）→ `block`；description 无触发条件且 frontmatter 未声明 `triggers` → `warn`（P1-4，不阻断，引导补全确定性路由信号）；其余软问题（描述/正文过短）→ `warn`。
>
> **危险模式扫描（C1，2026-08-14）**：正文级正则扫描，宁漏不误报——
> - **block 级**（命中即阻断发布，block message 标注类别）：提示注入覆盖指令（`ignore/disregard ... previous/prior instructions`、`override system prompt`）；破坏性命令（`rm -rf /`·`~`·`$HOME`）；管道执行（`curl/wget ... | bash/sh/zsh`）
> - **warn 级**（仅提示人工复核）：敏感凭据路径（`~/.ssh`、`~/.aws`、`/etc/passwd`、`/etc/shadow`）、已知外泄端点（`webhook.site`、`requestbin.com`、`pipedream.com`、`burpcollaborator.net`）

### 5.6 上传导入

`POST /v1/skills/import`

- 请求：JSON `{ "file": "<base64 ZIP>", "filename": "skill.zip" }`（`ImportSkillZipRequest`）；兼容 `multipart/form-data` 字段 `file`。
- 后端行为：接收 zip 后写入临时目录，解压并严格校验 Skill 编写规范；通过校验后生成导入任务，不直接入库。
- 响应：`{ "job_id": "job_01" }`（proto JSON 亦可能为 `jobId`）。
- 鉴权：admin（`assertImportAdmin`），与 Get/Apply/Refine 相同。走标准 Kratos 中间件（tracing / recovery / 错误编码器），不再经 `srv.Route` 旁路。

`GET /v1/skills/import/:job_id` — 轮询导入状态，返回 `SkillImportJob`（含 `candidates`、`conflict_groups`）。

`POST /v1/skills/import/:job_id/apply` — 应用导入结果，请求体含 `decisions[]`：

| action | 适用对象 | 语义 |
|--------|---------|------|
| `import_passed` | `pass` 候选 | 直接导入为新 Skill |
| `keep_separate` | `warn` 候选 | LLM 判定低冲突（`recommendation=keep_separate`）时，保留双方、导入候选为新 Skill；不触碰已有 Skill、不记录血缘 |
| `skip_duplicate` | 重复阻塞候选 | 显式跳过重复项，不安装 |
| `overwrite_duplicate` | 重复阻塞候选 | 以上传包为既有同 slug Skill 追加新版本（parent 锚定当前最新版） |
| `approve_risky_import` / `reject_risky_upload` | 高风险文件阻塞候选 | 用户放行 / 拒绝高风险包 |
| `merge_group_with_ai` | 冲突组 | AI 炼化合并（`retire_sources` 可选归档源 Skill；血缘 `derived_from` 始终记录） |
| `skip_group` | 冲突组 | 整组跳过 |

`POST /v1/skills/import/:job_id/conflict-groups/:group_id/refine` — AI 炼化冲突组，返回 `SkillRefineResult`。

**安全约束（2026-08-14 P13）**：

- **鉴权**：import 相关 RPC（`ImportSkillZip`/`GetSkillImportJob`/`ApplySkillImport`/`RefineSkillImportConflict`）要求 admin 权限（`assertImportAdmin`）；skill 读写端点经 `assertSkillAccess` 做 workspace 隔离（IDOR 防护）。
- **ZIP 上限**：单包文件数 ≤ 200（`maxImportFiles`）、解压总大小 ≤ 100MB（`maxImportTotalBytes`），超限分别返回 `ErrTooManyFiles`/`ErrTotalSizeExceeded`（防 ZIP 炸弹）。
- **状态机**：`ApplyImport` CAS 到 `applying` 后任何失败路径经 `context.Background()` 回滚为 `completed` 并记录原因，杜绝任务卡死；refine 阶段内存 job 丢失时从 DB + tempDir 重建（`jobStateFromDB`）。
- **清理**：终态导入任务由 `SkillImportJobStore.DeleteOldJobs` 批量清理（`Engine.CleanupOldJobs` 于 inspect 入口触发）。

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
| `ListSkillsRequest.sort_by` | 排序字段：`tag`（按首个标签名，tags JSONB 数组首个元素）\| `name`（按名称）；空 = 默认按更新时间倒序 |
| `ListSkillsRequest.sort_order` | 排序方向：`asc` \| `desc`；空 = asc（仅 sort_by 非空时生效） |
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

### 5.12 标签字典端点

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/v1/skill-tags` | 字典全量 + 实时使用计数 + 孤儿标签合成，按 `dimension` + `name` 排序 |
| `POST` | `/v1/skill-tags` | 预建标签（name 已规范化）；冲突返回 `CodeConflict` |
| `POST` | `/v1/skill-tags:rename` | 改名 + 事务重写所有 Skill 引用，返回 `rewritten` 条数；目标已存在时等价合并 |
| `DELETE` | `/v1/skill-tags/{name}` | 删除字典行 + 事务移除所有 Skill 引用，返回 `rewritten` 条数 |

`SkillTagInfo` 消息：

| 字段 | 说明 |
|------|------|
| `name` | 规范标签名（小写，可选 `dimension:` 前缀） |
| `dimension` | `:` 前缀维度，无维度为空串 |
| `source` | `system` \| `user` \| `orphan`（orphan = 使用中但未收录，运行时合成不落库） |
| `used_count` | 实时聚合 `skill.metadata_json.tags` 的使用次数 |
| `created_at` / `updated_at` | RFC3339 |

路由设计注意：标签字典使用独立前缀 `/v1/skill-tags` 而非 `/v1/skills/tags`——后者会被已注册的 `/v1/skills/{id}` GET 路由吞掉（`{id}="tags"` 匹配到 GetSkill）。

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
    // Triggers 确定性触发词（P1-3），来自 SKILL.md frontmatter，
    // 随 metadata envelope 落库；路由层命中用户输入时强制 preload。
    Triggers             []string
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
    // Triggers 确定性触发词（P1-3）；命中时绕过 intent/tag 过滤并置顶。
    Triggers      []string
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
- **阶段 3**（`SkillMergeWriter` 接口）：在单个事务内执行 4 步操作：为 target 创建新版本 → 更新 metadata/tags → 转移 source 调用记录 → 废弃源 Skill（状态 → `deprecated`，软删墓碑保留审计）。
  - **墓碑唯一键释放（C2，2026-08-14）**：`skill_key` 为全表唯一索引（无状态过滤），废弃时同步将源 `skill_key` 改名为 `<slug>--deprecated-<unixnano>` 释放 slug——否则墓碑永久阻塞同名重建/导入（`skill_skill_key_key` 冲突）。存量 deprecated 墓碑如需释放可物理清理（连带 `skill_version`）。

**Data 层实现**：`internal/data/skill_merge.go` — `SkillMergeRepo` 实现 `SkillMergeReader` + `SkillMergeWriter`。

### 6.8 SkillEvolutionOrchestrator — 统一进化编排

文件：`internal/biz/skill_evolution_unified.go`、`internal/biz/skill_evolution_triggers.go`

**EvolutionTrigger 接口**（策略模式）：

| Trigger | 来源 | 检测逻辑 |
|---------|------|----------|
| `PatternTrigger` | 工具调用 Pattern | 从高频工具调用组合中检测新 Skill 需求，返回所有匹配 pattern |
| `HealthTrigger` | 健康指标 | 检测 30d 失败率 > 30% 或 score < 60；依赖 `SkillScorer` 窄接口 |
| `AgentConfigTrigger` | Agent 配置（L3） | 评估 Agent 30d 指标（工具成功率 / 检索质量 / 负反馈）是否越阈，产出 evolve 建议；L3 opt-in 门控（未开启进化或低于最小信号量时返回 nil）；`queryReader` 按 (type+title) 对 pending 去重 |
| `SuccessTrigger` | 成功沉淀（P2 F3） | 检测 30d 成功率 ≥ 0.85 且调用量 ≥ `EvoTriggerMinInvocations` 且当前正文含规则块；与 health 共用 `(skill, improve_skill)` 冷却槽；产出「固化强化」型（非修复型）delta，禁删 `helpful>0` 规则 |

**Gate 验证维度**（`GateVerifier`，`internal/biz/skill_evolution_loop.go`，共 9 维）：functional（sandbox + 数据集回放，P2 F1 扩展为 AB 对照棘轮——draft 通过率不得劣于当前正文基线，基线不可得时仅查绝对阈值 0.6）、security、performance、style、effectiveness（P1：harmful≥3 规则不得原样保留）、drift（P2 F2：删 helpful≥3 规则 / 删除比例 >50% / 臃肿双条件 >1.5× 且 >+5 → 拒绝）、trigger_accuracy（P2 F4：`{Name|Slug}__trigger` 黄金集确定性回归，复用 `skillruntime.MatchTrigger`，棘轮 + 绝对下限 0.8）、paired_regression（P3 M1：逐 case 配对判定——baseline 下通过的 case 在 draft 下不得失败，win 不抵 regression，reason 携带 win/loss/tie 计数供审批审计）、no_op_change（P3 M1：draft 与 baseline 输出在所有可比 case 上逐字节一致 → 无可测量效果，拒绝空转版本/审批周期；任一侧 LLM 调用失败的空 hash case 不参与等价比较）。统一降级语义：依赖未配置 / 数据缺失（含 per-case 数据不可用）时跳过不阻断。P3 M1 起 AB 回放在一次 Verify 中仅执行一次，供 functional / paired_regression / no_op_change 三维共用（functional 基线检查失败时不触发回放，非法 draft 不烧 LLM 调用）；per-case 数据由 `SkillReplayResult.CaseResults`（`CaseVerdict{CaseID, Passed, OutputHash}`，trim 后 sha256）承载，见 `service.SkillReplayRunner.replayCases`。P2 维度详见 [phase3-进化能力/08](./phase3-进化能力/08-P2-进化验证强化与触发扩展.design.md)。

**SkillEvolutionOrchestrator**：

- `RegisterTrigger`：**线程安全**（`sync.RWMutex` 保护 `triggers` 切片）
- `CheckAndCreate`：原子化检查 + 创建，解决 TOCTOU 竞态
  - 先调用 `UnifiedEvolutionCheckReader.HasPendingForTarget` 检查
  - 遍历触发器时使用 **快照读取**（`RLock` → copy → `RUnlock`）
  - 每个 trigger 返回 `[]UnifiedEvolutionSuggestion`（支持多 pattern 同时触发）
  - 不存在则创建，DB UNIQUE 约束兜底（多实例并发安全）
  - 重复创建时返回 `nil, nil`（幂等）
  - 含 per-action-type 冷却期检查（F9：仅 `pending`/`approved`/`applied` 活跃状态计入冷却窗口——`rejected`/`expired`/`rolled_back` 不阻塞再次触发，见 `UnifiedEvolutionSuggestion.CountsForCooldown`）
- `ExpirePending`：批量过期超过 7 天的 pending 建议（status → `expired`）。**过期收敛唯一入口**：仅由 `EvolutionOrchestratorWorker` 每 tick 调用；`CuratorWorker` 只跑验证半区（草稿 + 沙箱 + lifecycle），不再承担过期职责

> 注：Orchestrator 不提供审批接口（2026-08-14 起删除 `Approve`/`Reject` 死代码）——审批统一在 usecase 层完成（L1 `SkillEvolutionUsecase.ApproveProposal`/`RejectProposal`、L2 `SkillIntelligenceUsecase.ApproveSuggestion`/`RejectSuggestion`、L3 `EvolutionUsecase` apply/reject/rollback），编排器只负责触发创建与过期。

**审批并发守卫（2026-08 审查修复）**：所有层级的状态转换均先经 `UnifiedEvolutionStateMachine.Transition` 校验转换合法性，再走 `UpdateStatusCAS`（`WHERE status IN (...)` 原子前置条件）防并发竞态——CAS 未命中返回 Conflict 提示重试。覆盖：L1 `ApproveProposal`/`RejectProposal`/`RegisterApproved`、L2 `ApproveSuggestion`/`RejectSuggestion`、L3 `applyAndMark`/`RejectSuggestion`/`RollbackSuggestion`。

**接口拆分**（符合"接口方法 ≤ 5"规范）：

- `UnifiedEvolutionCheckReader`（3 方法）：`HasPendingForTarget` / `GetLatestByTarget` / `GetLatestByTargetAndAction`
- `UnifiedEvolutionQueryReader`（5 方法）：`GetByID` / `ListByTarget` / `CountByTarget` / `ListByTargetAndAction` / `CountByTargetAndAction`（AndAction 变体区分同 target_type 的 L1 proposal 与 L3 agent 建议）
- `UnifiedEvolutionPatternReader`（1 方法）：`GetLatestByPatternHash`（L1 pattern_hash 去重，hash 存于 metadata）
- `UnifiedEvolutionMutationWriter`（6 方法，超 ≤5 规范 1 个——`UpdateStatusCAS` 为审批并发守卫新增；无条件 `UpdateStatus` 仅残留于 L2 `ApplyApprovedSuggestion`（Reload 成功后落 applied，已经双状态机校验），故保留并存；TECH-DEBT）：`Create` / `UpdateStatus` / `UpdateStatusCAS` / `UpdateDraftBody` / `UpdateLifecycleStatus` / `UpdateSandboxResult`
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

### 6.11 SkillTag 端口（标签字典）

文件：`internal/biz/skill/tag.go`；实现：`internal/data/skill_tag_repo.go`（`NewSkillTagRepo`）。

独立于 Skill Repo 复合接口（DB-N3 窄接口规则），按读写职责拆分：

```go
// Stability:evolving
type SkillTagReader interface {
    // 字典全量 + 实时使用计数 + 孤儿标签合成
    ListSkillTags(ctx) ([]TagInfo, error)
    // 轻量选项源：仅规范标签名（筛选器下拉用）
    ListSkillTagNames(ctx) ([]string, error)
}

// Stability:evolving
type SkillTagWriter interface {
    // 预建标签（name 已规范化）；冲突返回 CodeConflict
    CreateSkillTag(ctx, name) (TagInfo, error)
    // 字典改名 + 事务重写所有 skill 引用，返回重写条数
    RenameSkillTag(ctx, oldName, newName) (int, error)
    // 字典删除 + 事务移除所有 skill 引用，返回重写条数
    DeleteSkillTag(ctx, name) (int, error)
}

type TagRepo interface { SkillTagReader; SkillTagWriter }
```

Usecase 侧（`internal/biz/skill/tag.go`）职责：

- `normalizeTagName` — trim + 小写 + 格式校验（`^[a-z0-9][a-z0-9_-]*(:[a-z0-9][a-z0-9_-]*)?$`），维度取自 `:` 前缀。
- `RenameTag` / `DeleteTag` 成功后调用 `InvalidateEmbedCache()` + `invalidateDedupCache()`（§4.5 缓存一致性）。
- `tagRepoOrErr` — 未注入 repo 时返回 `CodeInternal`（防御性，Wire 正常装配不会触发）。

Data 侧关键实现：

- `skillTagUsage` — 全表扫描 `PlatformSkill`（未软删）的 `metadata_json`，小写名归一聚合计数。
- `rewriteSkillTagReferences` — 在事务内用 `MetadataJSONContainsFold(target)` 预筛候选行，逐行重写 `tags` 键（`rewriteMetadataTags` 保留其他键），命中才 UPDATE。
- Rename 合并语义：目标名已收录时删除源字典行；未收录时原地改名。两种情况都继续执行引用重写。

### 6.12 Agent Case 经验记忆（P3 M2，EverOS Agent Memory 启发）

**定位**：User Memory（L3 facts）理解用户；Agent Case 理解任务。会话结束后由 AutoMemoryWorker 在用户记忆提取之后追加提取，产出结构化经验（goal/approach/outcome/pitfalls/tools_used），供 M3 召回注入与 M4 case→skill 蒸馏消费。

**存储**：`memory_agent_cases` 表（DDL 迁移 `20261207`，raw SQL 管理，不进 Ent Schema）。幂等锚点 `UNIQUE(agent_id, source_session_id)`——重复提取/重试覆盖更新而非新增重复行；`INDEX(agent_id, outcome)` 供 M3 召回过滤。Data 实现 `internal/data/memory_agent_case.go`（`NewMemoryAgentCaseStore`，raw SQL + `RWDB()` 读写分离 + `entErrToBizErr` 翻译）。

**biz 端口**（`internal/biz/agent_case.go`）：

- `AgentCaseReader.GetAgentCaseBySession` — 幂等守卫，无记录返回 `(nil, nil)`。
- `AgentCaseWriter.UpsertAgentCase` — 空 goal/agentID 静默拒绝（无 goal 的 Case 是噪声）。
- `AgentCaseExtractor.ExtractCase` — 返回 `ErrAgentCaseSkip` 表示会话无提取价值（闲聊/单轮问答），整条跳过不落启发式；其他错误降级启发式。
- `ShouldExtractAgentCase` — 零成本预过滤：user 消息 <2 条或总内容 <200 字符直接跳过，省 LLM 成本。
- `HeuristicAgentCase` — LLM 不可用时的保底：goal 取首条 user 消息（截 120 字符），outcome 按末条是否 assistant 回复判定 success/partial，tools_used 从工具消息去重收集；approach/pitfalls 留空（启发式无法可靠推断，宁缺毋滥）。

**LLM 提取器**（`internal/service/agent_case_llm_extractor.go`）：`AgentCaseLLMExtractor` 复用 `MemoryLLMExtractor` 的 provider 路由 LLM 通道（同一实例注入，不另建调用链）。System prompt 显式要求 Agent 自我改进视角、排除用户画像（那是用户记忆管线职责）；输出严格 JSON，`{"skip":true}` 映射 `ErrAgentCaseSkip`。

**Worker 接线**（`internal/cronrunner/jobs/auto_memory.go` `extractAgentCase`）：在主提取流程（facts/episode）完成后追加，复用同一 `ConsolidateInput`；`tools_used` 由 Worker 从 `ChatMessage.OptionsJSON` 解析 `tool_name` 填充 `ConsolidateMessage.ToolName`（Activity 适配器在 role=tool 时写入）。幂等读失败按未提取继续（重试安全）；Case 写入失败只 Warn，主流程与 job 结果不受影响。流程日志 step `memory.auto.case_extract`。

**Wire 装配**：`data.NewMemoryAgentCaseStore` 同实例绑定 `AgentCaseReader`/`AgentCaseWriter`；`service.NewAgentCaseLLMExtractor(memoryLLMExtractor)` 绑定 `AgentCaseExtractor`；三者注入 `provideAutoMemoryWorker`。全部 nil 时 Case 分支整体跳过（legacy 行为）。

**召回注入（P3 M3）**：

- **biz 端口**：`AgentCaseRecaller.RecallAgentCases(ctx, agentID, query, limit)`（`internal/biz/agent_case.go`）。query 为空时实现返回最近高质量 Case；limit 由调用方给上限。
- **data 实现**（`memoryAgentCaseRepo.RecallAgentCases`）：query 非空走 pg_trgm `word_similarity(query, goal||approach||pitfalls||outcome_summary)` + `%>` 操作符（2026-08-10 中文短查询根修的同一惯用法，避免 `similarity()` 分母稀释）。注意 `%>` 匹配的是**文本连续区间**的最大相似度：中文查询须为 Case 文本的连续子串方可命中（psql 实测"批量导入"命中，非连续 token 组合"导入数据"仅 0.2 不命中）；无命中时返回空（合法的"无相关经验"），由调用方接受无 Case 块。query 为空或 trigram 查询出错时降级为 `quality DESC, updated_at DESC` 的最近高质量 Case——trigram 失败只 Warn 不返回错误，召回 best-effort 绝不阻断 turn。
- **prompt 注入**（`internal/agent/case_prompt.go` `CaseMemoryCue`）：每 turn 最多注入 3 条（`caseRecallMax`），单字段截 120 字符；渲染 `## 任务经验（该 Agent 的历史案例）` 块，success 行展示 goal+approach、failure 行展示 goal+pitfalls，带 `[SUCCESS]/[PARTIAL]/[FAILURE]` 结局标记。nil recaller / 召回错误 / 空结果一律返回 `""`。
- **管线位置**（`memory_inject.go` `buildRuntimeMemoryCue`）：与 L2/L3 并列、位于 L4 之前并入 `recallParts`，复用统一预算截断（`JoinCuesWithTokenBudget`）与末尾追加（前缀稳定）。
- **接线**：`rt.MemorySet.AgentCaseRecaller` ← `data.NewMemoryAgentCaseStore(d)`（`providePersistenceSet`），6 个 `TRPCMemoryKnowledgeDeps` 消费点（wire.go ×2、chat_orch_agent_build、openai_compat、a2a_endpoint、runner_team_trpc_phases）透传；nil 时 Case 块整体跳过。

**Case→Skill 蒸馏触发（P3 M4，EverOS 蒸馏链落地）**：

- **定位**：Case 积累到阈值后，把最近一批高质量任务经验蒸馏成一份 SKILL.md 草稿，作为 `create_skill` 建议汇入统一进化建议漏斗（pending → 人工审批 → 落库），不在后台自动创建技能。
- **触发器**（`internal/biz/skill_evolution_trigger_case_distill.go` `CaseDistillTrigger`）：挂为 orchestrator 的 `EvolutionTrigger`（target=agent / action=create_skill / source=`agent_case_distill`）而非独立 job——免费获得 pending 短路、per-action 7 天冷却、D8 自适应降频与 DB UNIQUE 兜底，且 LLM 蒸馏不占 memory 队列关键路径。判定链：L1 opt-in（`EvolutionSkillEvolve`，与 PatternTrigger 共用开关）→ 空 query 召回最近高质量 Case（复用 M3 `AgentCaseRecaller`，上限 10 条）→ 不足 5 条（`caseDistillMinCases`）跳过 → LLM 蒸馏。
- **错误语义**：DB 召回失败上抛 error（orchestrator 记 Warn，K2）；LLM 蒸馏失败/输出非法仅本地 Warn 本轮跳过（best-effort，下轮重试，避免 Warn 刷屏）；name/body 为空跳过。
- **建议载荷**：`DraftName`+`DraftBody`（完整 SKILL.md 草稿，审批界面直接预览），`Metadata.source_case_ids`（`EvoMetaSourceCaseIDs`）记录来源 Case ID 供审计追溯。
- **蒸馏器**（`internal/service/agent_case_skill_distiller.go` `AgentCaseSkillDistiller`，实现 `biz.CaseSkillDistiller`）：复用 `MemoryLLMExtractor.callModel` 通道（`ConsolidateInput{AgentID}` 仅作 provider 路由，无会话上下文）；`buildCaseDistillDigest` 把 Case 渲染为带 `[SUCCESS]/[FAILURE]` 大写结局、goal/approach/pitfalls/tools 信号的摘要；prompt 要求只固化多条经验中反复出现的共性模式、无共性输出空对象；`parseCaseDistillResponse` 容忍 markdown fence，name 归一化为 `[a-z0-9-]` slug（纯非 ASCII 名折叠为空 → 按提取失败跳过），body 低于 10 runes 视为敷衍输出跳过。
- **Wire 装配**：`NewAgentCaseSkillDistiller` 绑定 `biz.CaseSkillDistiller`（service.ProviderSet）；`NewMemoryAgentCaseStore` 同实例追加绑定 `biz.AgentCaseRecaller`（data.ProviderSet）；`provideSkillEvolutionOrchestrator` 注入二者并 `RegisterTrigger(NewCaseDistillTrigger(agents, caseRecaller, caseDistiller, lg))`。distiller 为 nil（LLM 通道不可用）时 trigger 整体 no-op。

**进化元数据维度与多样性观测（P3 M5，EverMind GSME 启发）**：

- **定位**：各 trigger 产出建议时在 Metadata 写入确定性可得的维度标签（`dims` 键），供平台级多样性聚合观测搜索塌缩——某 trigger_source 桶长期无新建议即信号。维度只取确定信号（当前仅工具名集合），不做 LLM 推断（贵且不稳定，塌缩观测只需稳定可聚合的标签）。
- **维度模型**（`internal/biz/skill_evolution_dims.go`）：`EvolutionDims{Tools []string}`，经 `EvoMetaDims="dims"` 键写入 Metadata（JSON object，全字段 omitempty——无信号的维度缺席，聚合端按键存在性过滤）。`normalizeToolNames` 归一化（trim/去空/去重/字典序排序，保证聚合稳定）；`withDimsTools` 在归一化后非空时才写键，避免 `{}` 噪声。
- **写入点**：`PatternTrigger`（从候选 pattern 的 toolHistory 提取工具名）与 `CaseDistillTrigger`（聚合所有来源 Case 的 `ToolsUsed` 并集）。后续新增 trigger 产出建议时应在 Metadata 写 dims。
- **聚合端口**：`biz.UnifiedEvolutionDiversityReader.GetDiversityOverview(ctx, since, topTools)`（Stability:evolving），返回 `[]EvolutionDiversitySourceStat`（trigger_source / count / latest_at / top_tools）。
- **data 实现**（`UnifiedEvolutionRepo.GetDiversityOverview`）：分桶用纯 SQL `GROUP BY trigger_source`（count + MAX(created_at)，count 降序 + source 字典序 tie-break）；dims.tools 频次在 Go 侧解析 metadata 统计——建议表是人工审阅量级，单一代码路径避免 `jsonb_array_elements`/`json_each` 双方言分叉；metadata 缺失/无 dims/解析失败的行被容忍（best-effort）。`topTools<=0` 默认 5；空窗口返回空切片而非错误。
- **API**（平台级 proto `api/kratos/evolution/v1/evolution.proto`，区别于单 target 视角的 AgentService/SkillEvolutionSuggestionService）：`EvolutionService.GetEvolutionDiversityOverview` → `GET /v1/evolution/diversity-overview`，`since` 缺省默认最近 24h。service 实现 `internal/service/evolution.go`（只读、无写路径，reader 未装配返回 Unavailable）。
- **Wire 装配**：data.ProviderSet 中 `NewUnifiedEvolutionRepo` 追加 `wire.Bind(biz.UnifiedEvolutionDiversityReader, *UnifiedEvolutionRepo)`；`service.NewEvolutionService` 进 service.ProviderSet；HTTP/gRPC 双注册（`internal/server/http.go`/`grpc.go`）。

---

## 七、运行时层

### 7.1 装配入口

文件：`internal/agent/trpc_build.go` → `buildSkillDeps`

流程：
1. `ListEnabledPublishedSkillKeys()` 确认存在已启用 + 已发布 Skill
2. 优先 `SkillDBRepo`（`DBRepositoryAdapter`）；否则 `FSRepositoryAdapter`
3. `skillruntime.NewAgentVisibilityFilter(ag.Settings)` — **Layer A-only**（P9/F1，2026-08-13）：构造时一次性解析 `skill_runtime_json` 为内存集合，overview 注入仅按 allowed/denied 过滤，**不再按 turn query 走 Layer B**——保证框架 `Available skills:` overview 块在会话内字节稳定（prompt 缓存前缀命中前提）；Layer B 动态路由结果改由 guidance hook 以尾部 system message 注入（见 §7.6）
4. `CodeExecutor`（local / docker，`CODE_EXECUTOR_BACKEND`；产出物经 `artifact_executor.go`）
5. `WithSkills` + `WithSkillFilter` + `WithSkillToolProfile(skillOptionsForAgent)`：`complete` 默认 Full；`tools_profile=spirit|chat_only` 强制 KnowledgeOnly 且 `WithAllowedSkillTools(skill_load)`（去掉 `skill_exec`/`skill_run`/stdin+poll 以及 `skill_select_docs`/`skill_list_docs` 常驻 schema）

Turn query 注入：`internal/service/trpc_turn.go` · `internal/team/runner_team_trpc.go` → `skillruntime.RunOptionWithTurnQuery`

### 7.2 运行时路由

文件：`internal/tools/skillruntime/resolve.go`

两级筛选：

**Layer A**（`applyLayerA`）：
- 按 `RuntimePolicy.AllowedSlugs` / `DeniedSlugs` 过滤
- **策略红线**：deny 优先于一切后续阶段（含 trigger 命中），被拒候选不参与触发词匹配

**确定性触发**（P1-3，`computeTriggerHits` + `matchTrigger`）：
- 候选 Skill 在 SKILL.md frontmatter 声明 `triggers: [报销, pdf]`，随 metadata 落库并进入 `RuntimeCandidate.Triggers`
- 匹配语义：CJK trigger 用子串匹配（中文无词边界）；ASCII trigger 用词边界匹配（`pdf` 不误中 `pdftk`），大小写不敏感
- 命中后：绕过 Layer B 的 intent 收窄与 tag 过滤（`reincludeTriggered` 重新并入），排序分 `triggerScore=2000` 高于 taxonomy 精确匹配（1000）强制置顶；占用 `MaxSkillsInToolset` 配额；历史表现融合（`applyRankResults`）跳过命中候选，确定性 preload 不被稀释
- reason 记录为 `trigger match: <词>`，可在路由诊断中观测

**Layer B**（`ResolveSkillSlugsDetailed`）：
- `skillrouter.DetectIntentPaths(query, maxPaths)` → 分类路径关键词匹配
- `filterByIntentPathsWithReasons()` → 按分类路径缩小候选
- `filterByAllTagsWithReasons()` → 按 `AllowedTags` + `ExtractTagHints()` 过滤
- `scoreCandidatesWithReasons()` → 按分类路径匹配度评分
- **Embedding 语义精排**（可选）：`SkillUsecase.ScoreByEmbedding(query, candidates)` → 余弦相似度融合评分
- 排序后取 `MaxSkillsInToolset`，返回 `ResolveResult{Slugs, Reasons}`

**候选确定性排序**（P9/F2，2026-08-13）：`ListEnabledPublishedSkillCandidates`/`Keys`/`Refs` 三个 data 层查询均加 `ORDER BY skill_key ASC`——候选序 byte-stable 是 overview 渲染序与路由分确定性、进而 prompt 缓存命中的前提。

**路由结果 per-invocation 记忆化**（P9/F3，2026-08-13）：`resolveAndWriteSkillState` 将 `ResolveResult` 缓存于 invocation state（`aranea.skill_resolve_memo`）。BeforeModel hook 在 tool-call 循环中每次模型调用都触发，而路由输入（turn query + agent 策略）在单次 invocation 内不变——记忆化后每次 invocation 仅 1 次候选 DB 查询（及 embedding/健康度查询）；瞬态错误不缓存，下次调用重试。

**健康度查询进程内缓存**（P9/F4，2026-08-13）：`SkillHealthMetricsAdapter`（`internal/service/skill_health_metrics_adapter.go`）对 `GetHealthMetrics(skillID, days)` 做 TTL=5min、上限 1024 条的内存缓存——单轮排名对每个候选各查 2 次（成功率 + 平均耗时），连续 turn 重复同一聚合；错误不缓存。

**动态排名链路接线**（P10/R1，2026-08-13）：上述 adapter 经 Wire 单例装配（`service.NewSkillHealthMetricsAdapter`，backed by `biz.SkillHealthAggregator` = `SkillIntelligenceRepo`），同时注入两处——`RuntimeTooling.SkillHealth`（turn 路由：chat / a2a / openai_compat 三处 `TRPCSkillDeps.SkillHealthProvider`）与 `SkillService.skillHealth`（`PreviewSkillRuntime` 预览与生产路径一致）。`resolveAndWriteSkillState` 将 provider 透传至 `SkillToolsetOptions.HealthProvider`，激活 `ResolveSkillSlugsDetailed` 的历史表现融合分支（`applyRankResults`：rank 分与 keyword/embedding 分 60/40 融合，trigger 命中候选跳过）；nil provider 时分支跳过，排名保持 keyword/embedding。装配侧经 `RuntimeTooling.skillHealthProvider()` 归一化 typed-nil，防止 `opts.HealthProvider != nil` 守卫被 nil 指针接口击穿。

**Routed slugs 全模式落库**（P10/R2，2026-08-13）：`resolveAndWriteSkillState` 不再按 progressive/full-profile 分流写状态——两种模式均无条件写 `aranea.skill_routed_slugs` + `aranea.skill_selection_reasons`，invocation recorder 据此持久化路由结果供健康指标关联「routed vs run」。

**非 complete 模式路由落库补齐**（P10/Q4，2026-08-13）：`task`/`minimized`/`none` prompt 模式 + 非 progressive load mode 原先 hook 返回 nil，路由不执行、routed slugs 不落库，健康指标在该组合下丢失 routed-vs-run 关联。现注册 route-only BeforeModel hook（priority=5，LayerDynamic）：只调 `resolveAndWriteSkillState` 写 invocation state，**不注入任何 prompt 消息**——task 模式的极简 prompt 契约不受影响；resolve memo 保证单次 invocation 仅一次候选 DB 查询。

**Path() dir 缓存**（P10/R3，2026-08-13）：`DBRepositoryAdapter` 的两个惰性缓存（正文 bodies + 存储目录 dirs）提取为 `skillRepoCaches` 子管理器（AS-COG-01：单 struct 不堆叠 2+ sync.Map），整体指针在 reload/Invalidate 时原子交换。框架在每次 `skill_load`/`skill_run` 都解析存储目录，未缓存时每次 2 次 DB 查询（`GetBySlug` + `GetStorageDir`）；缓存后同快照期内零查询。空路径（DB-only skill）可缓存；错误不缓存，下次调用重试。

**运行时缓存主动失效**（P0）：`DBRepositoryAdapter` 快照（摘要 + 已加载正文）默认 TTL 2min。`SkillUsecase` 持有 `RuntimeCacheInvalidator` 端口（DI 时由 `NewSkillDBRepository` 注入 adapter 自身），在 `ToggleEnabled` / `Delete` / `RollbackVersion` / `Publish` / `UpsertSkillFromDisk(ContentChanged)` / `Patch` 成功后调用 `InvalidateSkillRuntimeCache()`，使启用状态与正文变更秒级生效；未注入时退化为纯 TTL 兜底。

### 7.3 意图路由与分类

文件：`internal/tools/skillrouter/`

- `detect.go`：`DetectIntentPaths(userQuery, maxPaths)` → 关键词 → 分类路径
- `taxonomy.go`：`TaxonomyLeaves` 定义 + `ExtractTagHints(userQuery)` 提取 `file_type:*` / `domain:*` 提示

### 7.4 trpc-agent-go 桥接

文件：`internal/skill/trpc/`

- `repository.go`：`FSRepositoryAdapter` — 磁盘 FS → `trpcskill.Repository`
- `db_repository.go`：`DBRepositoryAdapter` — DB + TTL 缓存 → `trpcskill.Repository`（含 R3 `skillRepoCaches` 子管理器：正文 + 存储目录双缓存原子交换）
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

在 `productCallbackChain` 中注册 `newSkillGuidanceBeforeHook`（priority=5，LayerDynamic）。三个互斥分支：

- **Progressive**（`skill_load_mode=progressive`，优先生效，task/complete prompt 模式均可用）：注入紧凑的 `## Routed Skills` 列表，引导 LLM 用 `skill_load` 按需加载
- **Full Profile**（仅 `complete` prompt 模式）：注入渲染后的完整 guidance（`## Available Skills`）
- **Route-only**（P10/Q4：非 complete prompt 模式 + 非 progressive）：不注入消息，仅 resolve 并落库 routed slugs（健康指标可观测性兜底）

流程（Full Profile）：
1. `resolveAndWriteSkillState` 获取当前 turn 的 skill slugs（结果 per-invocation 记忆化，见 §7.2 F3）
2. `BatchGetSkillGuidance(slugs)` 批量获取 skill markdown（2 条 SQL：按 skill_key 查 Skill + 按 skill_id 查最新 Version）；渲染后的 cue 亦 per-invocation 记忆化（`aranea.skill_guidance_cue_memo`，空结果缓存、瞬态错误不缓存）
3. `manifest.Parse` 解析 frontmatter → `render.SkillGuidance` 渲染指导内容
4. 拼接为 system message **追加到 `args.Request.Messages` 尾部**（P9/N1：`[system + history + user]` 保持单调增长前缀，路由变化不再使整段会话历史的 prompt 缓存前缀失效）
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

1. 目录被外部删除或移走后，watch 将对应 slug 标记 **`filesystem_missing=true`**（DB 记录保留）。
2. 发布 `skill.filesystem.missing`；恢复目录并校验通过后发布 `skill.filesystem.recovered` 并清除标记。
3. 平台 UI「删除 Skill」为 **物理删除 DB**（B1，2026-08-14：skill + skill_version 行同事务硬删，释放 `skill_key` 全表唯一约束，同 slug 可立即重建）；**不**删除磁盘，**不**触发磁盘缺失告警。**已知 caveat**：磁盘目录残留会被 watcher 在下一轮 reconcile（≤5min）以 `isNew` 重新登记为 draft——若需「彻底删除」，须先清空对应磁盘目录再在 UI 删除（磁盘清理是否并入 Delete 流程待决策）。

**通知链路**

| 事件 | 触发 | 落点 |
|------|------|------|
| `skill.filesystem.imported` | 磁盘新建登记 | Monitor Events + EventBus |
| `skill.filesystem.updated` | 磁盘正文/metadata 变更 | **info 级仅 MonitorBus 实时事件（SkipPersist，不落 `monitor_events`/`admin_audit`，防高频刷屏；2026-07-29 EVT-R）**；warn/error 级仍落库 |
| `skill.filesystem.missing` | 目录删除 | Monitor Events + EventBus + Skill 页 Banner |
| `skill.filesystem.recovered` | 目录恢复 | Monitor Events + EventBus |
| `skill.filesystem.rejected` | 校验失败（含 slug 目录名不一致） | 运行记录 + Monitor |

> **SkipPersist 实现**：`watch.Reporter.Report.SkipPersist=true` 时仅发布 MonitorBus 实时事件（Events 页脉搏区可见），跳过 `monitor_events` 与 `admin_audit` 落库；见 [18-monitor.design.md §10.4](./18-monitor.design.md)。

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

业务逻辑在 `internal/service/skill_import.go`（`ImportSkillZip` RPC）；HTTP 绑定见 §5.2。

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
│   ├── SkillRunsPage.vue           # 运行记录页
│   └── SkillTagsPage.vue           # 标签字典管理页（/skills/tags）
├── pages/agent-settings/
│   └── AgentSettingsSkillsTab.vue  # skill_runtime_json 配置（allowed_tags 选项源接字典）
├── components/skills/
│   ├── SkillTable.vue · SkillFilterBar.vue · SkillStatsStrip.vue
│   ├── SkillStatsHoverChart.vue    # 统计列悬浮图形面板（ECharts 趋势 + 占比，懒加载健康数据）
│   ├── SkillEditorDialog.vue · SkillUploadPlaceholder.vue
│   ├── SkillDeleteDialog.vue · SkillRunsTable.vue · SkillPagination.vue
│   ├── SkillMetaDialog.vue         # 元数据编辑（标签字段选项源接字典）
│   ├── SkillFilesystemAlertBanner.vue · SkillHealthCard.vue
│   └── skillTableUi.ts
├── features/skills/
│   ├── api.ts · types.ts           # 含 listSkillTagsApi 等 4 个字典端点 + SkillTagInfo 类型
│   ├── useSkillsPage.ts · useSkillDetailPage.ts · useSkillRunsPage.ts
│   └── useExperienceReportListPage.ts
└── stores/skills/index.ts          # 含 skillTags 状态 + loadSkillTags/createTag/renameTag/deleteTag
```

路由：`/skills` · `/skills/runs` · `/skills/tags`（见 `frontend-pages.md` §4.6）

**SkillEditorDialog.vue** — 全屏 Dialog，左侧文件树 + 右侧内容编辑（`ListSkillFiles` / `GetSkillFile` / `UpdateSkillFile`）。

**SkillUploadPlaceholder.vue** — 上传 zip、轮询导入任务、冲突组炼化（调用 `features/skills/api` import 端点）。

**SkillTagsPage.vue** — 标签字典独立管理页：按维度分组（`QList` + 组头 chip）、搜索 + 收录状态筛选、新建/改名/删除对话框、孤儿行浅黄底色 + 收录按钮。Store 层 `tagsLoaded` 缓存避免选项源场景重复请求，管理页 `force=true` 强制刷新；改名/删除后自动失效缓存。

**Skill 管理页（SkillsPage）交互要点**：

- **标签分组显示**：表格标签列按 `维度:值` 前缀分组（维度 chip + 值 chip），无维度标签归入通用组。
- **排序**：筛选栏排序控件支持 `tag`（按首个标签名）/ `name`（按名称）+ 升降序切换，默认标签升序；参数经 `ListSkillsRequest.sort_by/sort_order` 下推 Postgres JSONB 排序。
- **操作列双按钮**：`启用`（草稿 → 已发布，生命周期 publish，Agent 运行时可挂载）与 `发布到生态市场`（上架为 ecosystem product，`stores/ecosystem.publish`）分离；启用/停用由 `PATCH /v1/skills/:id/enabled` 开关控制。
- **统计列悬浮图形面板**：`SkillStatsHoverChart.vue` 包裹 `SkillStatsStrip`，悬停 150ms 后展示 `QTooltip` 面板——近 7 天调用趋势堆叠柱状图（成功/失败）+ 成功率环形图 + 7d/30d 调用、p95 耗时、路由命中率；健康数据按行懒加载（`loadSkillHealth` 经 Page → Table props 注入，展示层不直连 store）。
- **最近调用列**：相对时间显示（刚刚 / N 分钟前 / N 小时前 / N 天前 / 本地化日期），tooltip 展示完整时间；`last_invoked_at` 为空才显示「未调用」。
- **SkillMetaDialog**：加宽（`app-dialog-card--xl`），名称 / Slug / 标签同行 grid 布局且吸顶（滚动描述/正文时始终可见）。
- **SkillEditorDialog**：双栏编辑器左右面板独立滚动（高特异性选择器压过 `.app-glass-panel` 的 `overflow: hidden`），细滚动条样式。
- **i18n**：管理页文案集中于 `skillsPage.*` 语言包（zh-CN / en-US）。

### 10.2 Quasar 组件清单

| 页面 | 主要组件 |
|------|----------|
| Skill 列表 | `QPage`、`QTable`、`QToggle`、`QChip`、`QBtn`、`QDialog`、`QPagination`、`QSelect`、`QInput` |
| 上传导入 | `QDialog`、`QUploader` 或拖拽 `QCard`、`QLinearProgress`、`QBanner`、检查结果列表、冲突组 `QCard` |
| 冲突组炼化 | 冲突组内 `QBtn`、可选 `QSelect`（provider/model）、`QInput type=textarea` 或 Markdown 编辑器 |
| 编辑页 | `QForm`、`QInput`、`QSelect`、`QExpansionItem`、`QBtn` |
| 运行记录 | `QTable`、`QBadge`、`QTooltip`、`QDialog`、`QDate` |
| 标签字典 | `QPage`、`QList`、`QChip`、`QBadge`、`QBtn`、`QDialog`、`QInput`、`QSelect`、`QTooltip`、`AppPageHero`、`AppPageToolbar` |

标签字段三处选项源（统一来自 `stores/skills` 的 `tagNameOptions()`，即字典 + 使用中标签的并集）：

| 位置 | 组件 | 说明 |
|------|------|------|
| Skill 元数据编辑 | `SkillMetaDialog.vue` | `QSelect multiple use-chips use-input`，允许输入新标签，带字典 hint |
| Skill 列表筛选栏 | `SkillFilterBar.vue` | `QSelect multiple use-chips`，筛选不创建新标签 |
| Agent 设置 | `AgentSettingsSkillsTab.vue` | `allowed_tags`（AND 语义），`QSelect multiple use-chips` |

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
// 标签字典
export async function listSkillTagsApi(): Promise<SkillTagInfo[]>
export async function createSkillTagApi(name: string): Promise<SkillTagInfo>
export async function renameSkillTagApi(oldName: string, newName: string): Promise<number>
export async function deleteSkillTagApi(name: string): Promise<number>
```

---

## 十一、Service 层与 Wire 注入

### 11.1 Service 层

文件：`internal/service/skill.go`（23 方法：19 Skill + 4 标签字典）+ `internal/service/skill_import.go`（4 方法：Import/Get/Apply/Refine）

薄适配层，职责：
- Proto Request → Biz DTO 转换
- `resolvedStorageRoot()`：通过 `SystemSettingRepo.Get()` 读取 `work_directory` → `storage.ResolveRootWithPlatform()`
- `safeSkillFilePath()`：路径安全校验（禁止 `..` 跳出 Skill 根）

### 11.2 Wire 注入

已有，无需新增。Skill 相关依赖通过 `wire.NewSet` 注入：
- `SkillRepo` → `SkillUsecase` → `SkillService`
- `TagRepo`（`NewSkillTagRepo`）→ `SkillUsecase.tagRepo` → 标签字典 4 RPC
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

**上下文预算计量**（P9/F5，2026-08-13）：`chat.context_budget` 进程日志新增 `skill_overview_tokens` 类别（`ContextBudgetCategorySkillOverview`，`internal/agent/context_budget.go`）——镜像框架 `Available skills:` overview 渲染（header + `- name: description` 行，应用 Layer A filter）计量其 rune 数，摘要从 repo 内存快照读取，零额外 DB 查询；与 `skill_guidance_tokens`（动态路由 cue）分列。刻意排除项：capability/tooling 指导块与 `(dir: [sN]/...)` 后缀（后者每 skill 每请求需 1 次 Path 查询，计量本身会成为热路径负担）。

---

## 十五、Go 包布局

```text
internal/
├── biz/
│   ├── skill/                  # 用例子包
│   │   ├── skill.go            # 用例与端口（SkillReader/SkillWriter/Repo 接口、Usecase、DTO、SkillFilesystem、SkillEmbedder）
│   │   ├── tag.go              # 标签字典端口（SkillTagReader/SkillTagWriter/TagRepo、TagInfo、normalizeTagName、Rename/Delete 缓存失效）
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
│   ├── skill_tag_repo.go       # 标签字典 Data 层（Ent + 事务重写引用 + 实时使用聚合）
│   ├── skill_merge.go          # 合并 Data 层（事务内 4 步操作）
│   ├── skill_dedup.go          # 去重 Data 层（含 SkillSimilarityEngine 集成）
│   ├── skill_intelligence.go   # 健康指标聚合（含 AvgTokenUsage/FeedbackScore）
│   ├── skill_health.go         # 健康 Data 层
│   ├── skill_invocation_stats.go # 调用统计 Data 层
│   ├── skill_import_job_store.go # 导入任务 Data 层（含 DeleteOldJobs 终态任务批量清理）
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
│   ├── skills_butler/          # Skill 管家（registry / recommend / analyze / optimize / evolve；装配于 service/cli_admin_tools.go，agent key=__skills__ 时挂载 8 工具）
│   └── skillrecommend/         # Skill 推荐（rank / rank_feedback / health_provider）
├── service/
│   ├── skill.go                # 薄适配（23 RPC：19 Skill + 4 标签字典）
│   ├── skill_import.go         # 导入用例桥接（4 RPC，含 ImportSkillZip）
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
api/kratos/skill/v1/skill.proto          → HTTP 契约（26 RPC）
internal/service/skill.go                → 适配层（23 RPC：19 Skill + 4 标签字典）
internal/service/skill_import.go         → 导入 biz 桥接（4 RPC，含 ImportSkillZip）
internal/biz/skill/skill.go              → 用例与 SkillReader/SkillWriter/Repo 端口
internal/biz/skill/tag.go                → 标签字典端口（SkillTagReader/SkillTagWriter/TagRepo）+ normalizeTagName
internal/data/skill_tag_repo.go          → 标签字典 Data 层（Ent + 事务重写 + 实时使用聚合）
internal/data/ent/schema/skill_tag.go    → skill_tags 表 Ent Schema
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

---

## 子模块：Skill 编写合同（三级披露）

> 2026-08-22 Codex / skill-creator 对照 Phase A4。不新开文档编号；导入器软警告落在 `internal/skill/importer/validate.go`。

Skill 对模型的成本与召回质量，取决于 **什么时候写什么**，而不是把手册一次性塞进 `SKILL.md`。

### 三级披露

| 层 | 内容 | 何时进入上下文 |
|----|------|----------------|
| L1 目录 | `name` + `description`（触发导向，须能和其他 Skill 区分） | 技能概览 / 路由 / mention |
| L2 正文 | `SKILL.md` 正文：只写 **会改变决策** 的步骤、约束、失败处理 | `use_skill` / preload 命中后 |
| L3 附件 | `references/`、`scripts/`、示例、长清单 | 正文点名后再读，禁止预注入全文 |

### 编写原则（skill-creator）

1. **描述可区分**：`description` 写清何时触发、何时不要触发；禁止与 `name` 同文、禁止「一个很有用的技能」这类空描述。
2. **正文只写决策差**：重复常识、可 grep 到的长文档、与 Tool schema 重复的参数说明，放到 `references/`。
3. **软阈值**：`SKILL.md` 超过 **12 KiB 或 500 行** 时导入 `warning`（`body_too_long`），不 `block`。进化管家改写 Skill 时同样遵守。

内置技能管家提示（`internal/scenario/system/prompts/skills/skills.md`）在 evolve/optimize 时复述上述三条。

### `$skill` mention（B5）

用户消息中的 `$slug`（如 `$xlsx-review`）在 `ResolveSkillSlugsDetailed` 与 routed 列表合流：Layer A 通过的 mention **置顶**，reason=`user mention`，不依赖再搜。未发布 / deny 的 mention 不加载。

---

*文档版本：5.2 — 增补 `$skill` mention 与 routed 合流（2026-08-22）。*
