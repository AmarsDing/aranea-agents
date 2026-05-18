# Skill 模块结构设计（对齐本仓库与平台架构）

本文档描述 **aranea-agents** 仓库中 Skill 能力的**现状基线**、**目标形态**与**演进路径**。产品交互与 API 字段细节以 [`20 skill.md`](./20%20skill.md) 为准；平台分层、Context 边界与 ADK 集成红线以 [`architecture/platform-architecture.md`](../architecture/platform-architecture.md) 为准。

---

## 零、文档定位

| 维度 | 说明 |
|------|------|
| **Skill 是什么** | 可安装、可版本化、带文件资产的能力包：说明书、约束、示例与触发条件；默认通过 **上下文注入** 影响 Agent，而不是替代 Tool 的执行语义。 |
| **Tool 是什么** | 模型可调用的可执行单元：参数校验、执行、副作用、返回值。 |
| **本文不写** | Quasar 页面文案与验收清单（见 `20 skill.md`）；不把 Skill **等同于** ADK `skilltoolset` 里的每一个工具——运行时可以是「提示注入 + 可选 toolset」。 |

**一句话**：管理面继续由 **Kratos + SQLite（Ent）+ Skill 根目录文件** 承载；运行时要补齐 **按 Agent/会话选 Skill → 渲染 Prompt 块 → 记录 usage**，并与现有 **`skilltoolset`（基于 FS）** 平滑衔接或替换。

---

## 一、概念边界（与平台架构一致）

- Skill 归属 **Capability Context**（与 Tool / MCP / Provider 并列），详见 `platform-architecture.md` 第一篇 §4、第三篇 §6「skill_bridge」示意。
- **跨 Context 规则**：Catalog / Conversation 只能通过 **`kernel/contracts`**（未来端口）查询 Skill 视图；按 [`platform-architecture.md`](../architecture/platform-architecture.md) **目标态**，**禁止**在非专用运行时适配包内 import `google.golang.org/adk`（当前仓库仍存在历史调用点如 `internal/tools/catalog`，迁移时应收敛）。
- **依赖方向（目标态）**：`proto → internal/service → internal/biz → internal/data`；导入等特殊路由可走同一服务的 HTTP 挂载，但业务逻辑应落在 **biz**，持久化在 **data**。
- Skill **默认不执行副作用**；需要读写文件、调用外部 API 时走 **Tool** 或显式「SkillAction」类扩展（若引入）。

---

## 二、现状基线（本仓库已实现）

### 2.1 代码与接口落点

| 层级 | 路径 | 职责 |
|------|------|------|
| API | `api/kratos/skill/v1/skill.proto` | SkillService：列表、详情、创建、更新、发布、启用、复制、删除、文件读写删除、运行时预览、运行记录分页（共 14 RPC） |
| 服务 | `internal/service/skill.go` | Proto ↔ biz 转换；`resolvedStorageRoot()` 对接系统设置 |
| 用例 | `internal/biz/skill.go`、`internal/biz/skill_import.go`、`internal/biz/skill_runtime.go` | 列表校验、Skill CRUD 端口定义、导入请求类型、运行时策略与候选模型 |
| 数据 | `internal/data/skill.go` | Ent 实现的 `SkillRepo`：查询、聚合统计、存储目录、草稿/发布、磁盘同步等 |
| Schema | `internal/data/ent/schema/platform_skill.go`（表名 `skill`）、`skill_version.go`、`skill_invocation.go` | 与 DB 表映射 |
| 导入 | `internal/skill/importer/*`（engine / validate / helpers / chat / errors） | ZIP 导入任务；LLM 相似度检查；挂载 **`/v1/skills/import*`**（multipart + JSON，不在 proto 内） |
| 监听 | `internal/skill/watch/runner.go` | `fsnotify` + debounce 目录监听；启动全量扫描；发布 `skill.reload` 事件 |
| 存储 | `internal/skill/storage/root.go` | 解析 Skill 文件根目录；**已实现**环境变量 + 系统设置 `work_directory` + OS 默认三级回落 |
| trpc 桥接 | `internal/skill/trpc/*`（repository / db_repository / tools / executor / filter） | FS 适配器 + DB 适配器 → trpc-agent-go `skill.Repository`；Skill Tool 构建；FilteredRepository；CodeExecutor 适配 |
| 运行时路由 | `internal/tools/skillrouter/*`（detect / taxonomy） | 用户意图 → 分类路径匹配；关键词路由；标签提示提取 |
| 运行时装配 | `internal/tools/skillruntime/*`（toolset / resolve） | DB 启用的 Skill → Layer A（allow/deny）+ Layer B（意图路由 + 标签过滤 + 评分）→ 过滤后的 `skill.Repository` → ADK skill tools |
| Agent 集成 | `internal/agent/trpc_build.go` | `buildSkillDeps()` 装配 Skill repo + filter + executor → `WithSkills` / `WithSkillFilter` / `WithSkillToolProfile` |
| 系统设置 | `api/kratos/system_setting/v1/system_setting.proto`、`internal/service/system_setting.go` | 单例 **`work_directory`**（前端「系统设置」页）；已用作 Skill 根路径推导锚点 |

### 2.2 `SkillService`（proto）已暴露的能力

已实现 RPC（共 14 个）：

| RPC | HTTP | 说明 |
|-----|------|------|
| `ListSkills` | `GET /v1/skills` | 列表（搜索、标签、启用、状态筛选、分页） |
| `GetSkill` | `GET /v1/skills/{id}` | 详情（含 `body_markdown`） |
| `CreateSkill` | `POST /v1/skills` | 创建草稿（写 SKILL.md + 入库） |
| `UpdateSkill` | `PATCH /v1/skills/{id}` | 更新草稿（元数据 + 正文可选更新） |
| `PublishSkill` | `POST /v1/skills/{id}/publish` | 发布（`draft` → `published`） |
| `ToggleSkillEnabled` | `PATCH /v1/skills/{id}/enabled` | 启用/停用 |
| `DuplicateSkill` | `POST /v1/skills/{id}/duplicate` | 复制为草稿 |
| `DeleteSkill` | `DELETE /v1/skills/{id}` | 软删 |
| `ListSkillFiles` | `GET /v1/skills/{id}/files` | 文件列表 |
| `GetSkillFile` | `GET /v1/skills/{id}/file` | 文件内容 |
| `UpdateSkillFile` | `PUT /v1/skills/{id}/file` | 文件更新 |
| `DeleteSkillFile` | `POST /v1/skills/{id}/files:delete` | 文件删除（禁止删 SKILL.md） |
| `PreviewSkillRuntime` | `GET /v1/skill-runtime-preview` | 运行时装配预览 |
| `ListSkillRuns` | `GET /v1/skill-runs` | 运行记录分页 |

此外，ZIP 导入链路通过 **`RegisterSkillImportHTTPServer`** 提供 4 个 HTTP 端点（不在 proto 内）：

| 端点 | 方法 | 说明 |
|------|------|------|
| `/v1/skills/import` | POST | 上传 ZIP，返回 `job_id` |
| `/v1/skills/import/{job_id}` | GET | 轮询导入状态 |
| `/v1/skills/import/{job_id}/apply` | POST | 应用导入结果 |
| `/v1/skills/import/{job_id}/conflict-groups/{group_id}/refine` | POST | AI 炼化冲突组 |

### 2.3 与 `20 skill.md` 的差距（产品契约）

以下能力在需求文档中已约定，对照当前实现状态：

| 能力 | 状态 | 说明 |
|------|------|------|
| 创建草稿 | ✅ 已实现 | `CreateSkill` RPC + `CreateSkillWithVersion` data |
| 更新草稿 | ✅ 已实现 | `UpdateSkill` RPC + `PatchSkill` data（可选字段更新） |
| 发布 | ✅ 已实现 | `PublishSkill` RPC + `PublishSkill` data（`draft` → `published`） |
| 文件删除 | ✅ 已实现 | `DeleteSkillFile` RPC（禁止删 SKILL.md） |
| 运行时装配预览 | ✅ 已实现 | `PreviewSkillRuntime` RPC（返回已启用 slug 列表 + 存储根） |
| `SkillDetail` 级响应 | ✅ 已实现 | `GetSkill` 返回 `Skill` + `body_markdown` |
| 版本历史 / 回滚 | ❌ 未实现 | 本期只展示当前版本号；版本回滚 API 待后续迭代 |
| 自动负熵报告 | ❌ 未实现 | 本期只展示已有聚合指标（invoke/success/failure/avg_duration） |
| 权限扩展字段 | 🟡 部分 | Proto 定义了 `SkillPermissions` / `SkillInvocationPermissions`；当前 data 层硬编码 `CanEdit=true` 等，未接入 RBAC |

导入链路已通过 **`RegisterSkillImportHTTPServer`** 提供 `POST /v1/skills/import`、`GET …/{job_id}`、`POST …/apply`、`POST …/conflict-groups/…/refine`，与 `20 skill.md` §7.4–7.5 一致；后续建议 **收敛进 proto** 或通过同一服务的 OpenAPI 聚合文档声明，避免「半手写路由」长期分叉。

### 2.4 运行时现状（重要）

当前 Agent 工具链中的 Skill 能力已通过 **`internal/tools/skillruntime/`** + **`internal/skill/trpc/`** 实现端到端装配：

**装配流程**（`internal/agent/trpc_build.go` → `buildSkillDeps`）：

1. `SkillUC.ListEnabledPublishedSkillKeys()` 获取已启用 + 已发布的 slug 列表。
2. 优先使用 `SkillDBRepo`（`DBRepositoryAdapter`，DB-backed + TTL 缓存）；否则回退到 `FSRepositoryAdapter`（磁盘 FS）。
3. 构建 `FilteredRepository`（按 slug 白名单过滤）。
4. 构建 `CodeExecutor`（local / docker，按 `CODE_EXECUTOR_BACKEND` 环境变量选择）。
5. 传入 `trpcllmagent.WithSkills(repo)` + `WithSkillFilter(filter)` + `WithSkillToolProfile(SkillToolProfileFull)`。

**运行时路由**（`internal/tools/skillruntime/resolve.go`）：

已实现 **Layer A + Layer B** 两级筛选：

- **Layer A**（`applyLayerA`）：按 `SkillRuntimePolicy` 的 `AllowedSlugs` / `DeniedSlugs` 过滤。
- **Layer B**（`resolveSkillSlugs`）：意图路由 + 标签过滤 + 评分排序。
  - `skillrouter.DetectIntentPaths()`：用户 query → 分类路径关键词匹配。
  - `filterByIntentPaths()`：按分类路径缩小候选。
  - `filterByAllTags()`：按 `AllowedTags` + `ExtractTagHints()`（从 query 提取 `file_type:*` / `domain:*` 提示）过滤。
  - `scoreCandidates()`：按分类路径匹配度评分，排序后取 `MaxSkillsInToolset`（默认 32，上限 256）。

**SkillRuntimePolicy**（`internal/biz/skill_runtime.go`）：

存储在 `agent_runtime_settings.skill_runtime_json`，字段包括 `allowed_slugs`、`denied_slugs`、`allowed_tags`、`intent_routing_enabled`（默认 true）、`intent_max_paths`（默认 3）、`max_skills_in_toolset`（默认 32）。

**Skill Tool 构建**（`internal/skill/trpc/tools.go`）：

`BuildSkillTools()` 产出 4 个 ADK 工具：`LoadTool`、`RunTool`（需 CodeExecutor）、`ListDocsTool`、`SelectDocsTool`。

**与 ADK 的衔接方式**：当前采用 **方式 A**（FS 适配器）+ **方式 B**（DB 适配器）混合：`FSRepositoryAdapter` 读磁盘 FS，`DBRepositoryAdapter` 从 DB 加载并缓存。两者均实现 `trpcskill.Repository` 接口，通过 `FilteredRepository` 包装后传入 ADK。方式 C（纯 Prompt 注入）尚未实现。

### 2.5 Skill 本地目录与系统设置「工作目录」

平台上 Skill **磁盘目录**应与用户在 **系统设置**里配置的 **工作目录**一致可追溯，避免「上传写到一处、运行时读到另一处」。

**系统设置契约（已实现）**

- Proto：`api/kratos/system_setting/v1/system_setting.proto` → 消息 `SystemSettings.work_directory`（注释：`Absolute or workspace-relative path used as the platform working directory`）。
- HTTP：`GET /v1/system-settings`、`PUT /v1/system-settings`。
- 持久化：`internal/data/ent/schema/system_setting.go` 单例行 **`work_directory`**。

**Skill 物理根路径（产品与实现对齐约定）**

| 项目 | 约定 |
|------|------|
| **Skill 根目录** | **`{ResolvedWorkDirectory}/skills`** —— 在工作目录下固定使用子目录名 **`skills`**，与 ZIP 导入解压目标、`skillstorage` 期望的根保持一致。 |
| **`ResolvedWorkDirectory`** | 服务端将用户填入的 `work_directory` 规范化为绝对路径（含 `~` 展开）；为空或未配置时的回落策略见下。 |

**解析优先级（已实现，自上而下短路命中）**

1. **`SKILL_ROOT`**（若设置 → 视作 Skill 根绝对路径）。
2. **`SKILL_STORAGE_ROOT`**（若设置 → 同上别名）。
3. **`filepath.Join(Resolved(work_directory), "skills")`** —— 仅当 **`work_directory` 非空** 且路径合法可读时使用。
4. **操作系统默认路径** —— 与 `storage.DefaultRoot(goos)` 一致（如 `%AppData%\Aranea\skills` 等）。

**已实现**：`storage.ResolveRootWithPlatform(rootDirectory)` 实现了 ①②③④ 完整回落链路。`SkillService.resolvedStorageRoot()`、`watch.Runner.resolveRoot()`、`importer.Engine.resolveRoot()` 均通过 `SystemSettingRepo.Get()` 读取 `work_directory` 后调用 `ResolveRootWithPlatform`。

运维或单机开发者仍可通过环境变量覆盖 ③④，便于 CI / 容器。

**注意事项**：前端「系统设置」保存成功后需提示：**更改工作目录后原有 `{旧路径}/skills` 下的 Skill 需迁移或重新索引**。ZIP 导入与文件读写均统一到同一解析函数。

### 2.6 目录监听 / 定时扫描 · 增量装载（磁盘 → 目录）

除 ZIP 导入与用户在前端编辑外，需要支持：**常驻监听 Skill 根目录**，在磁盘上出现新的合规 Skill 包时 **自动加载到系统目录登记**，无需重复手动导入。

**目标行为**

- Skill 根下每一个 **一级子目录**（或与现行导入一致的 slug 目录）若包含合规 **`SKILL.md`**（及规范允许的附带文件），应能通过后台同步进入 **`skill` / `skill_version`**（或与导入流水线共用同一校验与冲突策略）。
- 对已登记条目：**文件变更**可反映为新草稿版本或「待重新校验」；**目录删除**不建议硬删 DB，可标记 `archived` / `filesystem_missing`（字段名以迁移为准）。

**触发机制（可组合）**

| 方式 | 说明 |
|------|------|
| **定时扫描** | 服务端后台 **ticker**（间隔可配置，如 30s～5min）；比对子目录 **mtime / size / 内容 hash**，做增量 diff，避免每次全量暴力读大文件。 |
| **实时监听** | 在支持的 OS 上使用 **`fsnotify`**（或等价 API）监听 Skill 根及其一层或递归子树；对频繁保存 **debounce**（如 500ms～2s 合并事件）后再跑校验与入库。 |

**同步流水线（建议）**

```text
SkillRoot watcher/ticker
    → 枚举候选目录（跳过隐藏目录、临时前缀如 .）
    → 结构校验（与 20 skill.md / `internal/skill/importer` 一致）
    → slug/skill_key 幂等：已存在则比对 hash；仅变更则 upsert 版本或草稿
    → 可选：与「相似度 / 冲突组」策略对齐（磁盘导入可走轻量路径：同名目录视为同一 Skill）
    → 审计日志 / metrics：skill.fs.scan、skill.fs.synced、skill.fs.error
```

**与其它通路的关系**

- **ZIP 导入**：最终写入的仍是同一 Skill 根；扫描器应对同一 **`skill_key`/目录名** **幂等**，避免重复创建行。
- **权限与安全**：所有路径必须在 **解析后的 Skill 根** 之内，禁止 `..` 跳出；仅服务端进程执行扫描。

**现状**：`cmd/admin` 已集成 **`internal/skill/watch`**（启动全量扫描 + `fsnotify` debounce 2s；环境变量 **`SKILL_WATCH_DISABLED=1`** 可关闭）。`watch.Runner` 支持 `event.Bus` 集成，同步成功后发布 `skill.reload` 事件。磁盘目录缺失时调用 `MarkFilesystemMissing(slug, true)` 标记 `filesystem_missing` 字段，恢复时清除。可选的补充：**定时 ticker** 作为兜底仍可再加。

---

## 三、目标分层架构（Capability · 对齐 Tools 思路）

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

**与 ADK 的交界（已实现 A + B 混合，C 待规划）**：

- **方式 A**（已实现）：`FSRepositoryAdapter` 产出磁盘 FS → `FilteredRepository` → ADK `skill.Repository`。
- **方式 B**（已实现）：`DBRepositoryAdapter` 从 DB 加载并缓存（TTL 2min）→ `FilteredRepository` → ADK `skill.Repository`。
- **方式 C**（规划）：仅用 Prompt：Assembler 产出 **`Skill` 文本块**写入 system/developer message；`skilltoolset` 可选关闭——与「Skill 不必全是 Tool」一致。

`internal/skill/trpc/`（不 import ADK 的纯 Go 子包 `repository.go` / `filter.go`）作 **DTO → Repository** 的转换层；**真正引用 `trpc.group/trpc-go/trpc-agent-go` 的装配**在 `internal/agent/trpc_build.go` 的 `buildSkillDeps` 中完成。

---

## 四、Go 包布局（现状 + 规划）

当前仓库 Skill 相关包已基本成型，按增量演进原则维护：

```text
internal/
├── biz/
│   ├── skill.go               # 用例与端口（SkillRepo interface、SkillUsecase、DTO）
│   ├── skill_import.go         # 导入 DTO（SkillImportJob/Candidate/ConflictGroup/SimilarityMetrics/RefineRequest/ApplyRequest）
│   └── skill_runtime.go        # 运行时策略（SkillRuntimePolicy、SkillRuntimeCandidate、ParseSkillRuntimePolicy）
├── data/
│   └── skill.go                # Ent 仓储实现（SkillRepo、enrichSkill、聚合查询、草稿/发布/磁盘同步）
├── skill/
│   ├── importer/               # ZIP 导入引擎（已实现：engine / validate / helpers / chat / errors）
│   ├── watch/                  # Skill 根目录监听与磁盘同步（已实现：runner，含 fsnotify + debounce + eventBus）
│   ├── storage/                # Skill 存储根解析（已实现：root.go，含 ResolveRoot / ResolveRootWithPlatform / DefaultRoot）
│   └── trpc/                   # trpc-agent-go 桥接层（已实现：repository / db_repository / tools / executor / filter）
├── tools/
│   ├── skillruntime/           # 运行时装配入口（已实现：toolset.go / resolve.go）
│   └── skillrouter/            # 意图路由与分类（已实现：detect.go / taxonomy.go）
├── service/
│   └── skill.go                # 薄适配（已实现，含 resolvedStorageRoot / safeSkillFilePath）
├── server/
│   └── skill_import_http.go    # ZIP 导入 HTTP 路由注册（已实现，4 端点）
└── agent/
    └── trpc_build.go           # Agent 构建中 Skill 装配（已实现：buildSkillDeps）
```

**规划新增**（不破坏现有包结构）：

| 包 | 规划内容 |
|----|----------|
| `internal/skill/manifest/` | frontmatter / skill.json 解析与校验 |
| `internal/skill/render/` | prompt 块渲染、截断策略 |

若日后完整迁入「Capability Context」目录树，可将 `internal/skill/**` 整体映射为 `internal/capability/skill/**`（以迁移 playbook 为准）。

---

## 五、核心抽象（已实现 + 规划）

以下接口与类型用于统一「加载 → 路由 → 装配 → 记账」。

### 5.1 Skill 运行端口（已实现）

```go
// internal/biz/skill.go — SkillRepo interface
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

### 5.2 运行时策略与候选（已实现）

```go
// internal/biz/skill_runtime.go
type SkillRuntimePolicy struct {
    AllowedSlugs         []string
    DeniedSlugs          []string
    AllowedTags          []string
    IntentRoutingEnabled bool   // 默认 true
    IntentMaxPaths       int    // 默认 3
    MaxSkillsInToolset   int    // 默认 32，上限 256
}

type SkillRuntimeCandidate struct {
    Slug          string
    Name          string
    Description   string
    Tags          []SkillTag
    TaxonomyPaths []string
}
```

### 5.3 trpc-agent-go Repository 适配（已实现）

```go
// internal/skill/trpc/repository.go — FS 适配器
type FSRepositoryAdapter struct { ... }  // 实现 trpcskill.Repository

// internal/skill/trpc/db_repository.go — DB 适配器（TTL 缓存）
type DBRepositoryAdapter struct { ... }  // 实现 trpcskill.Repository

// internal/skill/trpc/filter.go — 白名单过滤
func NewFilteredRepository(base, allowedSlugs) trpcskill.ContextRepository
```

### 5.4 规划抽象（尚未实现）

| 抽象 | 规划内容 |
|------|----------|
| `SkillBackend` | 按 `kind`（markdown / prompt_pack / workflow / tool_backed）差异化加载与渲染 |
| `LoadedSkill` / `RenderedSkill` | 标准化加载结果与渲染结果，用于 Prompt 注入（方式 C） |
| `SkillManifest` | 统一 frontmatter / skill.json / manifest.json 解析 |

### 5.5 Manifest（逻辑模型）

与 `pkg/trpc-agent-go/tool/skilltoolset/skill` 的 frontmatter 习惯对齐，同时支持独立 `skill.json`：

| 字段 | 说明 |
|------|------|
| `name` / `slug` / `description` / `version` | 展示与版本 |
| `tags[]` | 检索与 Agent 策略 |
| `kind` | `markdown` \| `prompt_pack` \| `workflow` \| `tool_backed` |
| `entry` | 默认 `SKILL.md` |
| `requires[]` / `conflicts[]` | 依赖与冲突（见 §九） |
| `risk_level` | 策略中间件使用 |

**来源优先级**：`skill.json` / `manifest.json` → `SKILL.md` frontmatter → 导入 API 附加 metadata → 目录名推断。

---

## 六、中间件链（与 Tool 链对称）

Skill 链处理 **是否可选中、如何加载、如何裁剪、如何记录**，不做 Tool 的参数 schema 校验。

建议首批：

| 中间件 | 职责 |
|--------|------|
| Validation | manifest / entry 存在性 / slug |
| Policy | `enabled`、`status`、Agent allow/deny、tag |
| Budget | `MaxSkills`、注入 token 上限 |
| Cache | 按 `(skill_id, version)` 失效 |
| Tracing | `skills.assemble`、`skills.render` span |
| Usage | `activated` / `explicit_use` / `failed` 写入 `skill_invocation` 或事件总线 |

---

## 七、运行时 Assembler（已实现核心链路）

### 7.1 实际输入 / 输出

```go
// internal/tools/skillruntime/resolve.go
type SkillToolsetOptions struct {
    Runtime   *biz.AgentRuntimeSettings  // 含 skill_runtime_json
    UserQuery string                     // 当前用户输入
}

// resolveSkillSlugs → []string（过滤后的 slug 列表）
```

### 7.2 实际流程（Layer A + Layer B）

1. **读库**：`ListEnabledPublishedSkillCandidates()` 获取 `enabled` + `published`（兼容 `active`）的候选 Skill。
2. **Layer A**（`applyLayerA`）：按 `SkillRuntimePolicy.AllowedSlugs` / `DeniedSlugs` 过滤。
3. **Layer B**（`resolveSkillSlugs`）：
   - **意图路由**：`skillrouter.DetectIntentPaths(query, maxPaths)` → 分类路径关键词匹配。
   - **标签过滤**：`filterByAllTags()` 按 `AllowedTags` + `ExtractTagHints()` 过滤。
   - **评分排序**：`scoreCandidates()` 按分类路径匹配度评分，排序后取 `MaxSkillsInToolset`。
4. **装配**：`BuildTRPCSkillTools()` 将过滤后的 slug 列表 → `FilteredRepository` → ADK skill tools。

### 7.3 Skill Tool 产出

```go
// internal/skill/trpc/tools.go
func BuildSkillTools(cfg SkillToolsetConfig) []trpctool.Tool {
    // LoadTool: 加载 Skill 正文
    // RunTool: 执行 Skill 代码（需 CodeExecutor）
    // ListDocsTool: 列出 Skill 文档
    // SelectDocsTool: 选择 Skill 文档片段
}
```

### 7.4 规划扩展（尚未实现）

| 扩展 | 说明 |
|------|------|
| Prompt 注入（方式 C） | Assembler 产出 `## Available Skills` 文本块写入 system/developer message；`skilltoolset` 可选关闭 |
| embedding 相似度 | 候选筛选增加向量相似度匹配（当前仅关键词 + 标签） |
| Budget 中间件 | 注入 token 上限裁剪（当前仅 `MaxSkillsInToolset` 数量限制） |
| Preview API 增强 | 返回每个 Skill 的选中原因（`Reasons map[string]string`） |

---

## 八、持久化与 Ent / DB（现状 → 目标）

### 8.1 当前 Ent 字段摘要（与代码一致）

- **`skill`（`PlatformSkill`）**：`skill_key`、`name`、`description`、`status`、`enabled`、`config_json`、`metadata_json`（含 `tags` + `storage_dir` + `taxonomy_paths`）、软删 `deleted_at`，历史字段 `parent_id`、`level`、`agent_id`、`provider`、`model`，**已新增** `kind`（默认 `markdown`）、`risk_level`（默认 `low`）、`entry_path`（默认 `SKILL.md`）、`filesystem_missing`（默认 `false`）。
- **`skill_version`**：`skill_id`、`version`、`status`、`content_markdown`、`metadata_json`、时间戳，**已新增** `manifest_json`（默认 `{}`）、`published_at`、`validation_status`。
- **`skill_invocation`**：`skill_version`、`user_id`、`session_id`、`duration_ms`、`started_at`/`ended_at`、`preview`/`hash`、`error_code` 等；Ent 仍含 `input_json`/`output_json` 等广义字段，**已新增** `source`（默认 `runtime`，可选 `filesystem_scan` / `filesystem_watch`）、`activation_id`。

列表查询将 **`published` 与历史值 `active` 等同**（见 `skillListPredicates`），与迁移期数据共存。

### 8.2 已落地的新增字段（原 §8.2 建议演进 → 现状）

| 区域 | 原建议新增 | 当前状态 |
|------|-----------|----------|
| `skill` | `kind` | ✅ 已落地，默认 `markdown` |
| `skill` | `risk_level` | ✅ 已落地，默认 `low` |
| `skill` | `entry_path` | ✅ 已落地，默认 `SKILL.md` |
| `skill` | `filesystem_missing` | ✅ 已落地（原建议名 `runtime_status`，实际用 `filesystem_missing`） |
| `skill` | `current_version_id` | ❌ 未落地（通过 `skill_version` 查询最新版替代） |
| `skill` | `visibility` | ❌ 未落地 |
| `skill` | `default_config_json` | ❌ 未落地 |
| `skill_version` | `manifest_json` | ✅ 已落地 |
| `skill_version` | `published_at` | ✅ 已落地 |
| `skill_version` | `validation_status` | ✅ 已落地 |
| `skill_version` | `file_manifest_json` | ❌ 未落地 |
| `skill_invocation` | `source` | ✅ 已落地 |
| `skill_invocation` | `activation_id` | ✅ 已落地 |
| `skill_invocation` | `message_id` | ❌ 未落地 |

### 8.3 规划表（可选，重度依赖/权限开启时）

- `skill_permissions`（subject + 动作位）
- `skill_dependencies`
- `skill_conflicts`

具体 DDL 可参考下文「附录 B」中与上一版设计兼容的 SQL；落地前需与 **`接口与数据库开发规范`** 中的表前缀策略对齐（平台文档倾向 **`capability_*` 前缀**——若改名，需单独迁移任务）。

---

## 九、权限、可见性与冲突（浓缩）

### 9.1 可见性层级（目标）

`system` / `workspace` / `agent` / `private` / `public`——与 `20 skill.md` 的租户隔离、`permissions` 对象一起看；列表接口不返回无 `can_view` 的行。

### 9.2 运行时策略（已实现）

```go
// internal/biz/skill_runtime.go
type SkillRuntimePolicy struct {
    AllowedSlugs         []string
    DeniedSlugs          []string
    AllowedTags          []string
    IntentRoutingEnabled bool   // 默认 true
    IntentMaxPaths       int    // 默认 3
    MaxSkillsInToolset   int    // 默认 32，上限 256
}
```

来源：`agent_runtime_settings.skill_runtime_json`（JSON 对象），由 `ParseSkillRuntimePolicy()` 解析。

### 9.3 依赖与冲突

- **依赖**：`required_skill`、`optional_skill`、`tool_capability`、`runtime_feature`。
- **冲突**：导入 slug 冲突、语义相似（导入流水线已实现相似度与炼化）、运行时互斥策略。
- **分级**：`info` / `warn` / `block`——与 `20 skill.md` 的 pass/warn/block 一致。

---

## 十、HTTP / Proto 面（统一清单）

| 能力 | 状态 | 备注 |
|------|------|------|
| `GET /v1/skills` | ✅ 已实现 | 分页 query：`page`/`page_size` + 搜索/标签/启用/状态筛选 |
| `GET /v1/skills/{id}` | ✅ 已实现 | 详情含 `body_markdown` |
| `POST /v1/skills` | ✅ 已实现 | 创建草稿 |
| `PATCH /v1/skills/{id}` | ✅ 已实现 | 更新草稿 |
| `POST /v1/skills/{id}/publish` | ✅ 已实现 | 发布 |
| `PATCH /v1/skills/{id}/enabled` | ✅ 已实现 | 启用/停用 |
| `POST /v1/skills/{id}/duplicate` | ✅ 已实现 | 复制为草稿 |
| `DELETE /v1/skills/{id}` | ✅ 已实现 | 软删 |
| `GET /v1/skills/{id}/files` | ✅ 已实现 | 文件列表 |
| `GET /v1/skills/{id}/file` | ✅ 已实现 | 文件内容 |
| `PUT /v1/skills/{id}/file` | ✅ 已实现 | 文件更新 |
| `POST /v1/skills/{id}/files:delete` | ✅ 已实现 | 文件删除（禁止删 SKILL.md） |
| `GET /v1/skill-runtime-preview` | ✅ 已实现 | 运行时装配预览 |
| `GET /v1/skill-runs` | ✅ 已实现 | 运行记录分页 |
| `POST /v1/skills/import*` | ✅ 已实现（手写路由） | 4 端点：import / status / apply / refine；建议逐步 code-gen 或补 proto |
| `GET /v1/system-settings`、`PUT /v1/system-settings` | ✅ 已实现 | **`work_directory`**；Skill 磁盘根约定为 **`{work_directory}/skills`**（见 §2.5） |

前端类型与函数分组维持 `web/src/features/skills/api.ts` 与 proto 同源生成优先。**系统设置页**保存 `work_directory` 后，前端可在文案中提示 Skill 默认存放路径为 `{work_directory}/skills`（展示层可与后端返回的 **resolved skill root** 对齐，若后续 API 补充该只读字段）。

---

## 十一、可观测性

建议事件或 span：

- `skill.activated`、`skill.used`、`skill.failed`
- Span：`skills.assemble`、`skills.registry.search`、`skills.backend.load`、`skills.backend.render`、`skills.usage.record`

属性：`skill.id`、`skill.slug`、`skill.version`、`agent.id`、`session.id`、`activation.source`、`token.cost`。

磁盘同步（§2.6）建议补充：`skill.fs.scan`、`skill.fs.synced`、`skill.fs.error`（日志或指标），便于确认「文件夹新增已入库」。

## 十二、演进路线（2026-05-18 现状对齐）

| 阶段 | 内容 | 状态 |
|------|------|------|
| **P0** | `storage.ResolveRootWithPlatform()` 接通 `SystemSetting.work_directory`，约定 `{work_directory}/skills`；ZIP/文件 API/导入与工作目录一致 | ✅ 已完成 |
| **P1** | 补齐 proto：`CreateSkill`、`UpdateSkill`、`PublishSkill`、`GetSkill`、`DeleteSkillFile`、`PreviewSkillRuntime`；biz/data 贯通 | ✅ 已完成 |
| **P2** | `internal/tools/skillruntime`：Layer A + Layer B 路由 + `BuildTRPCSkillTools`；`DBRepositoryAdapter`；Agent 集成 `buildSkillDeps` | ✅ 已完成 |
| **P2′** | Skill 根目录 `fsnotify` 监听 + debounce + eventBus + `filesystem_missing` 标记 | ✅ 已完成 |
| **P3** | manifest/依赖/冲突表或 JSON 扩展；权限粒度（RBAC）；OpenTelemetry 贯通 | ❌ 待实现 |
| **P4** | Prompt 注入（方式 C）；embedding 语义精排；Budget 中间件；Context 目录迁移 | ❌ 待实现 |

---

## 十三′、Skill 召回管线：分类树 · 多维标签 · 索引收窄 · 语义精排（规划）

本节描述端到端心智模型；**层 A（Agent 策略）与层 B（意图→候选收窄）已在运行时落地**，小范围语义精排仍为后续能力。

### 13′.1 详细分类树（示例）

意图可挂载到叶子路径（存储侧推荐写入 Skill `metadata_json.taxonomy_paths` 数组）：

- **数据获取与集成**
  - **内部数据源**
    - **文件系统读取（读取表格）** → 示例 Skill：`excel_reader`（读 xlsx）
- **分析与推理**
  - **自然语言理解（情感分析）** → 示例：`sentiment_analysis`
- **交互与执行**
  - **消息发送（发邮件）** → 示例：`email_sender`

### 13′.2 多维标签

标签推荐落在 **`SkillTag.name`**（与检索惯例一致），使用形如 `dim:value` 的扁平 token，例如：

- `file_type:xlsx`
- `domain:sales`

运行时：**同类约束为合取（AND）**——候选 Skill 必须同时具备本轮所需的每一个标签 token（Agent 策略中的 `allowed_tags` 与用户话术中带出的 hint 合并后再过滤）。

### 13′.3 索引收窄 → 标签过滤 →（规划中）语义精排

**.walkthrough（与用户示例对齐）**

1. **意图分类**：从用户话术中命中多条意图路径（实现上先做关键词/规则路由；语义意图模型可后续替换）。
2. **候选召回**：在多条路径下做 **OR** 并集（示意：约 23 个 Skill）。
3. **标签过滤**：例如要求 `file_type:xlsx` 且 `domain:sales`，将候选 **AND** 缩至更小集合（示意：7 个）。
4. **语义排序（待实现）**：对余下的少量 Skill 做 embedding / rerank，选出最优组合，例如：
   - `excel_reader`（读 xlsx）
   - `sentiment_analysis`（情感分析）
   - `email_sender`（发送邮件）
5. **执行规划（Planner / LLM，与本节数据结构独立）** 示意：
   - Skill 1: `excel_reader(file_path=?, sheet="客户反馈")`
   - Skill 2: `sentiment_analysis(text_col="反馈内容")`
   - Skill 3: `email_sender(to=当前用户邮箱, body=负面反馈汇总)`

### 13′.4 已实现：层 A — Agent `skill_runtime_json`

持久化字段：**`agent_runtime_settings.skill_runtime_json`**（JSON 对象）。

| 字段 | 含义 |
|------|------|
| `allowed_slugs` | 非空时仅保留列出的 slug（skill_key），与其它过滤 **相交** |
| `denied_slugs` | 优先剔除 |
| `allowed_tags` | 与话术抽取的标签 hint **合并** 后，对 Skill 标签做 **合取** 过滤 |
| `intent_routing_enabled` | 默认 `true`；显式 `false` 时跳过层 B 路径收窄 |
| `intent_max_paths` | 意图路径上限，默认 `3` |
| `max_skills_in_toolset` | 最终挂载到 ADK toolset 的 Skill 数量上限，默认 `32` |

### 13′.5 已实现：层 B — 意图路径关键词路由

- 内置若干 **taxonomy leaf** 与 **召回关键词**（`internal/tools/skillrouter`）。
- `DetectIntentPaths(user_query)` → 多条叶子路径；候选 Skill 只要在 **taxonomy_paths** 或 **slug/name/description** 上与任一命中路径相关，则进入并集。
- `ExtractTagHints(user_query)`：解析 `file_type:`、`domain:` 及常见 `xlsx` 暗示。

### 13′.6 Skill `metadata_json` 约定

```json
{
  "taxonomy_paths": [
    "数据获取与集成/内部数据源/文件系统读取（读取表格）"
  ],
  "tags": [{ "name": "file_type:xlsx", "source": "user" }]
}
```

### 13′.7 代码锚点

- `internal/tools/skillruntime`：`SkillToolsetOptions`、`resolveSkillSlugs`、`BuildTRPCSkillTools`
- `internal/tools/skillrouter`：`DetectIntentPaths`、`ExtractTagHints`、`TaxonomyLeaves`
- `internal/biz/skill_runtime.go`：`SkillRuntimePolicy`、`SkillRuntimeCandidate`、`ParseSkillRuntimePolicy`
- `internal/data/skill.go`：`ListEnabledPublishedSkillCandidates`
- `internal/agent/trpc_build.go`：`buildSkillDeps`

---

## 十三、测试关注点

- manifest 解析与校验（SKILL.md / skill.json）
- Assembler：policy / budget / 裁剪顺序
- 导入：结构校验、名称冲突、相似度、炼化、apply 幂等
- **`work_directory` 拼接 Skill 根**：变更设置前后路径解析、空值回落、env 覆盖优先
- **磁盘扫描 / watch**：新增目录入库、重复 slug 幂等、文件变更检测、删除目录与 DB 状态一致
- 数据：`published` vs `active` 兼容查询
- 前端：`features/skills` 与分页契约

---

## 附录 A · 与代码模块映射（替换旧「§十八」）

```text
api/kratos/skill/v1/skill.proto          → HTTP 契约真相源（逐步扩展）
internal/service/skill.go                → 适配层
internal/biz/skill.go                    → 用例与 SkillRepo 端口
internal/data/skill.go                   → Ent 仓储与聚合
internal/skill/importer/*                → ZIP 导入领域实现
internal/skill/watch/*                   → 磁盘监听与幂等 upsert
internal/server/skill_import_http.go     → 导入路由挂载
internal/tools/catalog/assemble.go       → ADK tool 装配（SkillsFS → skilltoolset）
internal/tools/skillruntime/*           → 启用 Skill → **策略(A)+意图(B)收窄** → 过滤 FS → skilltoolset
internal/tools/skillrouter/*            → 意图路径关键词与标签 hint（层 B）
pkg/trpc-agent-go/tool/skilltoolset/**          → Skill 工具链参考实现（tRPC-Agent-Go）
api/kratos/system_setting/v1/system_setting.proto → `work_directory`；Skill 根推导见 §2.5
internal/service/system_setting.go       → 设置读写；与 skillstorage 接通待实现
```

---

## 附录 B · 规划表 DDL（参考，落地前须评审前缀与迁移）

与前一版设计兼容；**若启用 capability_ 前缀，仅追加映射层或迁移脚本**。

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

-- agent_runtime_settings：Skill 运行时策略 JSON（见正文「十三′」）
ALTER TABLE agent_runtime_settings ADD COLUMN skill_runtime_json TEXT NOT NULL DEFAULT '{}';
```

关系表 `skill_permissions` / `skill_dependencies` / `skill_conflicts` 及索引设计与前一版一致时可原文沿用；本文不再重复占篇幅，需要时从 Git 历史恢复全文 DDL。

---

## 附录 C · 落地原则（保留）

1. Skill 是知识/流程/提示包，Tool 是可执行函数；协作靠 Runtime Prompt + usage 关联。
2. **不得破坏**已安装 Skill 的存储目录与版本引用路径。
3. 运行时激活 **可解释**：每个 Skill 有选中原因与 token 估计。
4. Prompt 渲染 **可截断、可测试**，禁止无界注入大文件。
5. 副作用默认只走 Tool 或显式审批路径。
6. **Skill 物理根与系统设置工作目录一致可追溯**：解析规则唯一；变更工作目录须有迁移或再索引策略。
7. **磁盘增量同步须幂等、可观测**，且不得跨越 Skill 根路径访问文件系统。

---

*文档版本：4.1 — 增补 §2.5「工作目录 / `{work_directory}/skills`」与 §2.6「定时扫描与目录监听」；HTTP 清单与演进路线同步；附录与测试要点更新。*

*4.0 — 以 **aranea-agents** 仓库与 `platform-architecture.md` 为锚重写结构；区分现状基线与目标分层；修正旧稿中的模块路径与 HTTP 前缀表述。*
