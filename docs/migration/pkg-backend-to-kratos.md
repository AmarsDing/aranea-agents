# pkg/backend → Kratos（`internal/` + `api/`）迁移指南

> **全栈执行顺序（后端 → 前端分层 → UI/UX）** 已汇总为独立文档：**[AI-full-stack-migration-playbook.md](./AI-full-stack-migration-playbook.md)**。本文仍为本仓库 **后端迁移细节与硬约束** 的主参考。

面向 **AI 与人的分步执行**：将 `pkg/backend` 中基于 **手写 SQL + net/http** 的业务，按 **[接口与数据库开发规范](../API/接口与数据库开发规范.md)** 迁入 **`api/`、`internal/service|biz|data`、`web/src/services`**。

---

## 1. 两套运行时对照（迁移前必读）

| 维度 | 旧栈（`pkg/backend`） | 新栈（本仓库 Kratos Admin 线） |
|------|------------------------|--------------------------------|
| 入口 | `pkg/backend/internal/app/http_run.go`（`app.Run`）、各工具内嵌 | `cmd/admin/main.go` + `wire` |
| HTTP | 自管 `net/http` + `internal/transport`（如 `messages.go` 路由） | Kratos **`http.Server`**，`Register*HTTPServer`，路径来自 **proto `google.api.http`**，通常 **`/v1/...`** |
| 契约 | 无统一 proto；手写 JSON 结构与 handler | **`api/kratos/**/*.proto`** 为唯一对外契约 |
| Go 业务分层 | `service` → `repository.SQLiteRepository` / 各 `adapters/sqlite` | **`internal/service` → `internal/biz` → `internal/data`** |
| 领域模型 | `internal/domain` | **`internal/biz`** 中领域 struct + `Repo` 接口 |
| SQLite | **`database/sql`** + 嵌入 **`migrations/0001_init.sql`**（`repository/sqlite.go`） | **`ent` + `internal/data/ent/schema`**；开发见 `DEPLOY_ENV=dev` 自动迁移 |
| DB 文件 | 默认 **`data/arenea.db`**（`http_run.ServerOptions`/环境变量 **`DB_PATH`**）；你提到的 **`cmd/data/arenea.db`** 与同路径 **`pkg/backend/data/arenea.db`** 应为同库副本/部署拷贝 | **`configs`** 里 **`data.sqlite.source`**（如 `file:./data.sqlite`）；**合并库需单独迁移任务** |
| 前端 | `web/src` 可能仍调旧 **`/api/...`** 或网关代理 | **`web/src/services`** 使用 **`make api`** 生成的 **TypeScript**，经 **`axiosHandler`**、`getBackendOrigin()` |

迁移目标：**行为与数据语义对齐**；**对外协议**从「手写 JSON」收敛为 **proto + 生成代码**。

**范围约定（必须遵守）：迁移工作不包含种子数据。** 不要求在 Kratos 侧复刻旧栈 `Migrate()` 之后的 **`Seed*`**（如默认头像、聊天选项、内置工具等）。若运行时确实需要等价数据，交由**产品与部署另行处理**（例如手工导入、整库接续、单独工具），不作为本迁移文档内 AI/开发步骤的义务。

---

## 2. 迁移硬约束（proto · data · HTTP，复盘必守）

以 Avatar 等职能在迁入过程中的纠偏为原则，以下为 **必须与 [接口与数据库开发规范](../API/接口与数据库开发规范.md) 同时遵守** 的约束。

### 2.1 协议（`api/**/*.proto`）

1. **对外能力必须在 Proto 中印全**：同一业务的 **`service`** 应列出该域在 **`/v1/...`** 上暴露的 **全部 RPC**（列表、创建、二进制/按需下载类方法、删除等），并配以 **`google.api.http`**。**禁止**「一半写在 proto，另一半用手写 `srv.Route` / `HandleFunc` / `HandlePrefix` / 独立 `*_route.go` 补丁」——否则契约分裂、生成客户端缺失、中间件与 Operation 链路不一致。
2. **修改 `.proto` 必须跑生成**：仓库根执行 **`make api`**，提交生成的 **Go**（`*.pb.go`、`*_http.pb.go`、`*_grpc.pb.go`）与 **`web/src/services`**。**禁止**只改契约不重生。

### 2.2 持久化（`internal/data`）

1. **SQLite 侧以 `*ent.Client` 为主入口**：读写经 **`NewData` 打开的 `Ent()`**、`Repo` 持有 `*data.Data`（或等价注入）；实现风格对齐 **`internal/data/admin.go`**（`Query` / `Create` / `Update`、实体 **`convert*`** 到 `biz`；在 **`internal/data`** 包内可直接使用 **`r.data.entClient`**，与 **`admin.go`** 一致）。
2. **`NewData` 不另建 SQLite 连接**：只通过 **`ent.Open(driverName, dsn)`** 打开与应用配置一致的 SQLite 并得到 **`Data.entClient *ent.Client`**。**禁止**在 `NewData` 里再 **`sql.Open` 同一 DSN**、再用 **`entgo.io/ent/dialect/sql`（`OpenDB`）** 包装成**第二套** `*ent.Client` ——即不要「并联池化」SQLite。`Data` 结构体只保留 **`entClient *ent.Client`**（及可选的 **`pg`/`vectorDim`** 等）；跨表校验应通过 **补全 Ent schema**（如为实现 `agents` 计数而增加 **`Agent` 实体映射 `agents` 表**）改用 **`Query().Where(...).Count(ctx)`**，而不是为 raw SQL 再挂一个 `*sql.DB`。
3. **表结构进 Ent**：在 **`internal/data/ent/schema`** 声明实体；由既有流程（如 `DEPLOY_ENV=dev` 下 **`Schema.Create`**）管理。**禁止**在无说明的情况下长期平行维护「仅存 SQL 脚本、不进 Ent」。
4. **禁止在非 `NewData` 场景下无理由并联同一 SQLite 的第二套 `database/sql`**：**若**某条路径既不能建 Ent 实体、又必须由驱动执行少量 SQL，须在 PR/RFC **书面说明理由与收口**，避免双栈永久化。
5. **复杂 WHERE / BLOB**：优先 **`predicate` + `dialect/sql`（如 `ExprP`、`And`/`Or`）**，避免整页复制旧 `repository` 裸 SQL 与 Ent 分叉。

### 2.3 HTTP / gRPC 挂载（`internal/server`）

1. **与 Admin 同一注册范式**：HTTP 上对业务模块 **只做** **`Register<Module>HTTPServer(srv, svc)`**，gRPC **只做** **`Register<Module>ServiceServer`**（比照 **`RegisterAdminServiceHTTPServer`**）。**禁止**在同一业务模块上再叠加未写入 proto 的 **手写路由**。
2. **横切与非业务路由**（探测页、网关、下载代理等）：须单独写明，**不承担**业务 `FooService` 的契约盲区。

### 2.4 与旧 `transport` 的差异（须显式记录）

旧栈常见 **multipart、`application/octet-stream` 直连**；默认 Kratos/JSON 网关下常为 **protobuf/JSON，`bytes` 常以 base64 呈现**。若必须与旧客户端 **字节级兼容**，须在迁移说明中单写 **兼容性策略**，**禁止**仅靠未记入 proto 的私搭路由「悄悄对齐」。

---

## 3. 旧库表结构索引（`0001_init.sql`）

以下按**业务域**分组，便于拆模块迁移；完整 DDL 仍以  
`pkg/backend/internal/repository/migrations/0001_init.sql` 为准。

| 域 | 表名（节选） | 说明 |
|----|----------------|------|
| **Agent / 目录** | `agents`, `agent_runtime_settings`, `agent_prompt_files`, `agent_category_nodes`, `llm_provider_models` | Agent 配置、运行时、分类、模型目录 |
| **团队与运行** | `teams`, `team_runs`, `team_run_steps` | 多 Agent 编排与执行轨迹 |
| **会话与消息** | `sessions`, `messages`, `chat_entities_order`, `chat_attachments`, `chat_options`, `session_summaries` | 对话主链 |
| **计量与计费** | `model_token_usage_events`, `model_pricing_rules`, `model_token_usage_daily` | Token/价格/日汇总 |
| **工具与调用** | `tools`, `tool_agent_overrides`, `tool_invocations`, `tool_invocation_params`, `tool_usage_daily` | 工具目录与调用审计 |
| **审计** | `audit_logs` | 操作审计 |
| **资源 / Avatar** | `avatar_assets` | 头像资源（与 `repository/avatar.go` → `catalog/adapters/sqlite` 对应） |
| **集成** | `hooks`, `plugins`, `hook_agents`, `channel`, `channel_credential`, `channel_delivery`, `mcp_server` | 外部通道与插件 |
| **技能** | `skill`, `skill_version`, `skill_invocation` | Skill 生命周期 |
| **运维** | `cron_task`, `cron_task_run`, `monitor_events`, `monitor_traces` | 定时任务与监控 |
| **记忆 L0–L4** | `memory_l0_assembly_snapshots`, `memory_items`, `memory_l1_*`, `memory_episodes`, `memory_l2_index_meta`, `memory_event_marks`, `memory_facts` 及 fact 系列、`memory_entities`, `memory_relations`, `memory_entity_*` | 分层记忆与知识图谱相关 |
| **进化** | `agent_identity`, `agent_strategy_profile`, `agent_evolution_events`, `agent_evolution_proposals`, `agent_skill_stats` | Agent 进化与统计 |

---

## 4. 旧代码定位速查（以 Avatar 为例）

**分步清单（可勾选）**：Avatar 单独有一份 **[Avatar 迁移清单](./checklist-avatar.md)**，按规范列出协议、biz、data、service、Wire、前端与验证项。

| 层次 | 路径 | 职责 |
|------|------|------|
| 仓储门面 | `pkg/backend/internal/repository/avatar.go` | `SQLiteRepository` 上薄封装，委托 **`catalog/adapters/sqlite`** |
| SQLite 实现 | `pkg/backend/internal/catalog/adapters/sqlite/`（含 `AvatarRepository` 等） | 直接 SQL / 事务 |
| 领域 | `pkg/backend/internal/domain` | `AvatarAsset`、`AvatarImage` 等 |
| 服务 | `pkg/backend/internal/service/*` | 组合 repository 与用例 |
| HTTP | `pkg/backend/internal/transport/*` | 路由与 handler |

迁到 Kratos 后，对应关系应为：

`api/.../xxx.proto`（RPC）→ **`internal/service`**（实现 `*Server`）→ **`internal/biz`**（Usecase + `XxxRepo` 接口）→ **`internal/data`**（**优先 Ent + `Data.Ent()`**；特例见 **§2.2**）。

---

## 5. 单模块迁移标准步骤（AI 每迁一域照此执行）

以下与 **[接口与数据库开发规范](../API/接口与数据库开发规范.md)** 一致，仅补充 **自 legacy 迁入** 的注意点。

### 5.1 协议与生成

1. 在 **`api/kratos/<module>/v1/`** 新增或扩展 **`.proto`**：Resource 命名、**AIP 风格**列表/分页（与现有 `admin.proto` 对齐）。
2. 根目录执行 **`make api`**，提交 **Go** + **`web/src/services`** 生成物。

### 5.2 领域与用例

1. 在 **`internal/biz`** 定义与旧 **`domain`** 等价或有意裁剪的模型；**`Repo` 接口**只描述持久化，不暴露 SQL。
2. 若需与旧 HTTP 行为一致，列出 **差异清单**（字段名、错误码、分页 token）。

### 5.3 持久化策略

| **Ent 重建表** | 在 **`internal/data/ent/schema`** 建实体，字段映射旧表；写 **一次性数据迁移脚本**从 `arenea.db` 读入新库/新文件 | 长期维护、与 Kratos 配置统一到同一 SQLite |

**数据库接口约定（建议对齐 [`internal/data/admin.go`](../../internal/data/admin.go)）：**

- **`data` 注入**：Repo 结构体内嵌 **`\*data.Data`**（参见 **`adminRepo`**），通过 **`r.data.entClient`**（或 **`Data.Ent()`**，与本仓库其它 Repo 命名一致即可）访问 **`\*ent.Client`**，不再直接使用手写 **`database/sql`** 连接串起的 SQLite Repository。
- **查询**：单条用 **`Get`/`Only`**（`.Where(...).Only(ctx)`），列表用 **`Query().Where(...).Order(...).Offset(...).Limit(...).All(ctx)`**；与 AIP 列表参数对齐时，筛排序 **`github.com/go-kratos/aip-go/ents`** 的 **`ApplyFilter`/`ApplyOrderBy`**（同 **`ListAdmins`**）。
- **写操作**：**`Create()…Save`**、**`UpdateOneID`…`Save`**、**`DeleteOneID().Exec`**；领域错误映射 **`ent.IsNotFound`** → **`biz.Err*`**（参见 **`FindByID`**）。
- **转换**：持久实体 ↔ **`internal/biz`** 模型放在 Repo 包内小函数（如 **`convertAdmin`**），不把 SQL / Ent 类型泄漏到 **`biz`** 接口。
- **例外**：统计视图、跨表聚合若必须用原生 SQL，经 **`\*ent.Client` 上的 Exec/Query**（或封装助手），仍共用 **`NewData` 打开的同一 SQLite**，勿重复 **`sql.Open`**。

向量相关旧逻辑若在 PG，与当前 **`internal/data/pgvector`** 对齐；SQLite 仅存配置时延续 **[规范](../API/接口与数据库开发规范.md)** 中的分工。

### 5.4 服务装配

1. 实现 **`internal/service/<Module>Service`**，嵌入 **`Unimplemented*Server`**，转调 **`biz`**。
2. **`internal/server/http.go`（及 grpc.go）**：**`Register*HTTPServer`**（同 **§2.3**，不接手写路由补丁）。
3. **`data` / `biz` / `service` 的 `ProviderSet`** 增加构造函数；**`cmd/admin`** 下执行 **wire** 更新 **`wire_gen.go`**。

### 5.5 前端

1. 在 **`web/src/services/index.ts`** 增加 **`create<Module>Service()`**，与 Admin 同样挂 **`requestHandler`**。
2. 页面删除对旧 **`/api/...`** 的硬编码路径，改为生成客户端方法。

### 5.6 验证

- 单测：`biz` / `data` 关键路径；若有旧 **`pkg/backend`** 测试，可迁用例或对比 golden。

---

## 6. 建议迁移顺序、阶段划分与「全部迁移」说明

### 6.1 既定顺序（降低耦合与风险）

1. **已落地参考**：`AdminService`（认证、列表、CRUD）— 作为模板。
2. **独立强边界、表相对较少**：**Avatar / catalog**（`avatar_assets`、分类与平台资源等）— **proto + biz + Ent** 全链路（§2）。
3. **核心对话链**：**sessions / messages**（会话、消息顺序、附件、`chat/options` 等）。
4. **Agent / Team 配置**：agents、teams、`agent_runtime`、team-runs 等。
5. **工具与插件等集成面**：tools、plugins、hooks、channels、skills、mcp-servers（与 `cap`/`catalog` 适配器衔接）。
6. **记忆 L0–L4 + 进化**：依赖会话/Agent 已迁或可对齐契约后再迁；按 L0→L4 与 `agent_evolution_*` 分包。
7. **运维与可观测**：**`cron` 任务 CRUD/运行列表已迁 `cmd/admin` `cron/v1`**；monitor、model-usage 等仍待迁。

**dependencies**（外键、跨表事务、SSE/流式）密集的域可略作**顺序微调**，但优先保持上表**自下而上依赖**清晰。

### 6.2 「全部迁移」的含义与约束

- **全部迁移** = 将 `pkg/backend/internal/transport` 所挂载的 **全部业务 HTTP 能力**（见 `handler.go` 中 `registerRoutes`）按 §2、§5 迁到 **`api/kratos/**` + `internal/*` + `web/src/services`**，直至旧进程可退役或仅作兼容网关。
- **不可一次性合入**：须 **按域 / 按 PR** 推进；每合并一域应 **更新下表状态**、补充或勾选 **checklist**、跑通 **`make api`**、**`wire`**、**`go build`**。
- **双进程期**（`pkg/backend` 与 `cmd/admin` 并存）须遵守 **§7** 与部署约定（库锁、只读副本等）。

### 6.3 阶段总览表（与旧路由对齐，随 PR 更新）

路由前缀均指旧栈 **`/api/v1/...`**（`pkg/backend/internal/transport/handler.go`）；新栈一般为 **`/v1/...`**（无 `/api` 前缀时由网关或前端代理统一）。

| §6.1 步 | 域 | 旧侧能力摘要（不完全列举） | 建议 Kratos proto 划分（示例） | 仓库状态（`cmd/admin`） |
|--------|----|---------------------------|----------------------------------|-------------------------|
| 1 | Admin | 登录/登出/当前用户、管理员 CRUD | `api/kratos/admin/v1` | **已落地** |
| 2 | Avatar / 平台资源 | `avatar-assets`；`agent-categories`、`llm-provider-models`、`hooks`、`mcp-servers` 等平台 CRUD/树 | `avatar/v1`、`agent_category/v1`、`llm_provider_model/v1`、`hook/v1`、`mcp_server/v1`（Ent **`PlatformHook`** / **`PlatformMCPServer`**，避免 **`Hook`** / **`MCPServer`** 类保留冲突） | **Avatar、`agent-categories`、`llm-provider-models`、`hooks`、`mcp-servers` 已迁移**；其余 catalog 资源仍在收口 |
| 3 | 会话与聊天 | `sessions`、`chat/messages`（含 stream）、`chat/options` | `session/v1` + `chat/v1`（或合并为 `conversation/v1`，须在 proto 中一次定清） | **`session/v1` 已在 `cmd/admin` 落地**（`/v1/sessions`、`/v1/sessions/{id}/timeline` 等；timeline 依赖 Ent：`sessions`、`messages`、`tool_invocations`，skill 侧与 **`skill_invocation`/`skill`** 表对齐）；**`chat/v1` 仍未迁入**：`/chat/messages`、`/chat/messages/stream`、`/chat/options` **仍为遗留 REST `/api/v1/...`** |
| 4 | Agent 目录；Team / 运行 | `agents`（CRUD、runtime、prompt、preview）；`teams`、`team-runs`、`team-run-events` | `agent/v1`；`team/v1` | **`agent/v1`（目录）与 `team/v1`（teams + team-runs/steps）已落地**；**`team-run-events`（SSE）已在 `cmd/admin` 独立 SSE 端口挂载**（前端同源 **`/sse/team-run-events`**）；实时 **`biz.TeamRunEventBroker.Publish`** 仍待会话/编排栈迁入后接线（运行写入可能仍在遗留侧）。 |
| 5 | 技能 · 工具 · 插件 · 通道；**Cron（任务 CRUD + 运行列表）** | `skills`、`plugins`、`channels`、`/api/v1/tools`（`capability` 注册）；**`cron-tasks`、`cron-task-runs`** | `skill/v1`、`plugin/v1`、`channel/v1`、`tool/v1` 等；**`cron/v1`** | **`cron/v1` 已落地**（Ent `CronTask` / `CronTaskRun`，HTTP `/v1/cron-tasks`、`/v1/cron-task-runs`）；**调度执行仍由 `pkg/backend` CronRunner 等同库表读写**。**`plugin/v1` 已落地（管理 UI）**：Ent **`PlatformPlugin`**，`GET /v1/plugins`、`PATCH /v1/plugins/{id}/enabled`、`PUT /v1/plugins/{id}/config`；与旧栈相比 **未迁** `SyncBuiltins`、CLI/运行时插件装配等（仍 **`pkg/backend`**）。**`skill/v1` 已落地（管理 UI）**：Ent **`PlatformSkill`** / **`SkillVersion`** / **`SkillInvocation`** 等，HTTP **`GET /v1/skills`**、`PATCH /v1/skills/{id}/enabled`、`POST /v1/skills/{id}/duplicate`、`DELETE /v1/skills/{id}`、文件与子路径、`GET /v1/skill-runs`；**ZIP/冲突导入等多段 JSON 与 multipart 仍走旧 `/api/v1/skills/import*`**（见 Playbook §3 A9）。**`tool/v1` 已落地（管理 UI CRUD + runs）**：`/v1/tools`、`/v1/tools/runs` 等；**`GET /api/v1/agents/:id/tools/effective` 仍走遗留**（待 `agent/v1`）。**`channels` 等未迁移**（部分逻辑在 `capability`、`catalog`） |
| 6 | 记忆 · 进化 | `memory` 适配器挂接；`evolution` 注册 | `memory/v1`（可按 L0–L4 子 service 或 message 后缀拆分） | **未迁移**；本仓库 **`internal/data/pgvector` + `biz/memory`** 为 **另一类记忆存储**，与 SQLite 会话链并行时需 **文档说明边界** |
| 7 | 用量 · 监控 | `model-usage/*`、`monitor/*`（含 audit、占位 logs） | `usage/v1`、`monitor/v1` | **`usage/v1`**（`/v1/usage/*`）；**`monitor/v1`**（`/v1/monitor/audit|events|events/{id}|traces|traces/{id}|logs`；SQLite：`audit_logs`、`monitor_events`、`monitor_traces`）；**SSE（tx7do + broker）**：`/sse/monitor/logs/stream`、`/sse/team-run-events` → **`configs.server.sse`**；用量写入与其它路由仍可能走 **`pkg/backend`**。**前端**：[`features/monitor/api.ts`](../../web/src/features/monitor/api.ts)（门面）、[`components/monitor/`](../../web/src/components/monitor/)（展示 `.vue`，与 `vue-design.md` §2 路径硬性一致）。 |

**横切**：`GET /healthz` 可归入运维或 Ingress，不必强行塞进某一业务 proto；若迁入 Kratos，需在模块说明中单列。

**Catalog / 平台余项「逐个收口」顺序**（tools、channels、`usage`、`monitor`、skill import、SSE 等）：见 **[checklist-catalog-platform-remnants.md](./checklist-catalog-platform-remnants.md)**。

### 6.4 AI / 人肉执行「迁完全部」的操作顺序（每次只推进一行）

对每个 **状态为「未迁移」** 的域，从上到下按 §6.1 顺序选域 → 为该域单列 **迁移清单**（可仿 **[checklist-avatar](./checklist-avatar.md)**）→ 按 **§5** 与 **§9** 逐项完成 → **回写本 §6.3 表**为已落地。

**下一优先（在 §6.1 步 2～3 间二选一或由产品指定）**：  
- 继续 **逐个** 收口 catalog / 平台剩余资源（monitor 等），或  
- 启动 **sessions / messages**（步 3，核心对话链，工作量大）。

---

## 7. 数据与 ID 类型风险

- 旧库大量使用 **`TEXT` UUID** 主键；Kratos/Ent 示例多为 **int64**。新 proto 中 **id 类型**需统一设计（string vs int64），并 **全链路**（DB、Ent、biz、proto、TS）一致。
- **双写/双读窗口**：迁移期可能同时存在 `pkg/backend` 进程与 `cmd/admin`—需约定**库文件锁**或**只读副本**，避免双进程写同一 SQLite 文件。

---

## 8. 文档与规范关系

- **日常开发步骤**（proto、make、wire、web）：一律遵循  
  **[接口与数据库开发规范](../API/接口与数据库开发规范.md)**。
- **本文档**：解决 **从 `pkg/backend` 迁出** 时的**模块边界、表清单、顺序、数据策略**，以及 **§2 迁移硬约束**（proto 完整、仅 Ent 主链、注册范式、与旧 transport 差异须落字）。

---

## 9. AI 执行检查清单（迁一个子域时复制）

- [ ] 已读 `0001_init.sql` 中本域表结构及索引。  
- [ ] 已定 **ID/时间字段** proto 表示与存储类型。  
- [ ] **`api/**/*.proto`** 已覆盖本域 **全部** 对外 HTTP 能力，且已执行 **`make api`** 并提交生成物。  
- [ ] **未**为同一业务域增加 **手写 `Route`/`HandleFunc`** 补路由（参见 **§2.3 HTTP**）。  
- [ ] **`internal/biz`**：`Repo` + `Usecase`；无 `import api/…`。  
- [ ] **`internal/data`**：**`NewData`** 仅用 **`ent.Open`** 打开 SQLite（**§2.2** 第 2 条），以 **`Ent()`/`entClient`** + **Ent schema** 为主，风格对齐 **`admin.go`**；若在 `NewData` 之外并联 raw SQL 已 **书面说明理由**（同一节第 4 条）。  
- [ ] **`internal/service`**：**`Register*HTTPServer`** / **`Register*ServiceServer`** 与 Admin 同构装配。  
- [ ] **Wire** 更新并 **`go build`** 通过。  
- [ ] **`web/src/services/index.ts`** 已导出 **`create\*Service`**。  
- [ ] 已与旧 **`pkg/backend`**（multipart、二进制直出等）**显式对齐或写明差异策略**（参见 **§2.4 与旧 transport 的差异**）。  

完成以上条目，即视为该子域可按 Kratos 规范持续迭代维护。
