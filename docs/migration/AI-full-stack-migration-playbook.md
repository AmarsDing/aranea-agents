# AI 全栈迁移 Playbook（Kratos 后端 → Vue 架构 → 奶油昼/玻璃夜 UX）

> **目的**：把 [`pkg-backend-to-kratos.md`](./pkg-backend-to-kratos.md)、[`docs/vue-design/vue-design.md`](../vue-design/vue-design.md)、[`docs/UI/UX.md`](../UI/UX.md) 收敛成 **单一执行顺序**，供人或 AI **按域（domain）迭代**：**先完成该域 Kratos 后端与前端对接**，再对该域相关页面做 **设计与 UX 样式落地**（可同一 PR 或紧随其后的 UX PR）。
>
> **范围**：与迁移主文档一致——**不包含种子数据**（`Seed*`）；需要的数据由部署或独立工具处理。

---

## 0. AI 执行提要（开工先读）

1. **选定目标域**：在 [**§6 模块总表**](#6-模块总表后端状态--建议前端落点) 中确认本迭代要迁的行；顺带打开 [`pkg-backend-to-kratos.md`](./pkg-backend-to-kratos.md) §6.3、**§6.3.1**（`LEGACY_REST_ORIGIN`、`CRON_*`）与旧路由 [**§7**](#7-旧路由索引便于对账) 对账路径是否全覆盖。  
2. **读规范栈**（按 **[§1](#1-规范优先级冲突时)** 优先级）：API/DB + Kratos 主文档 → **`vue-design.md` 全文**（前端分层与自检）→ 若改 UI 再读 `UX.md`。  
3. **按域执行顺序**：**[§2 A → B → C](#2-强制阶段顺序每个业务域)**，不要在 proto 未定稿时让前端长期手写新路径。  
4. **前端能组件化则组件化**（详见 **[§4 B8](#4-阶段-b--前端架构condensed-from-vue-design)**）：列表/筛选/表格行/空态/玻璃外壳等重复结构抽到 `components/<域>/`，页面保持瘦；**网页 UI 的优化与验收**一律以 **[`UX.md`](../UI/UX.md) 为权威**（阶段 C 按 §1～§8 对齐 token / 组件数值 / 布局 / Do·Don't），见 **[§5](#5-阶段-c--设计与-uxcondensed-from-uxmd--quasar)**。  
5. **收尾**：用 [**§8 任务卡片**](#8-ai-单次会话任务卡片复制模板) 勾选；更新 **§6 表**与 `pkg-backend-to-kratos.md` §6.3 / **§6.3.1**；PR 描述写清与旧 `pkg/backend` **路径/字段差异**及**未迁子能力**（若有）。  
6. **编号别混**：本文 **§3～§5** = 阶段 A/B/C（本 Playbook）；`vue-design.md` 里的 **§4 / §5** = 该文档自检清单与迁移剧本——提及「§5」时写明是哪份文档。

**最低验收（每域可机械核对）**

| 层级 | 命令 / 条件 |
|------|----------------|
| 后端 | `make api`（或与本仓库等价的 proto 生成流程）已跑通；`go run github.com/google/wire/cmd/wire ./cmd/admin`（或 `go generate`）更新 `wire_gen.go`；**`go build ./cmd/admin`** 通过 |
| 前端 | `features/<域>/api.ts` 存在且**不**在展示组件里直连 HTTP；`web/src/services` 已暴露 `create*Service` 与 **`kratosApi`**；过渡 **`/api/v1`** 经 **`kratosApi` `/v1/...`** 或 **`axios` + `getBackendBaseURL()`**，收口在 **`features/*/legacyRest.ts`** 等 |
| UX（若本轮做 C） | 触达页面按 **`UX.md` 全文**对照（至少 **§1 自检、§2 token、§5 组件数值、§7 布局、§8 Do / Don’t**）；优先在**已抽好的展示组件**上落样式，避免只在 Page 堆叠覆盖 |
| 组件化（B/C） | 同一域内 **可复用 UI** 已抽到 `components/<域>/`（见 §4 B8），无「单文件超长模板」 |

---

## 1. 规范优先级（冲突时）

1. **接口与持久化**：[接口与数据库开发规范](../API/接口与数据库开发规范.md) + [`pkg-backend-to-kratos.md`](./pkg-backend-to-kratos.md) §2 硬约束  
2. **前端分层**：[vue-design.md](../vue-design/vue-design.md)（含 [vue-design-agent-rules.md](../vue-design/vue-design-agent-rules.md) 英文摘要）  
3. **视觉与交互 token**：[UX.md](../UI/UX.md)  

---

## 2. 强制阶段顺序（每个业务域）

对 **下方 [§6 模块总表](#6-模块总表后端状态--建议前端落点) 中每一行「未完成」的域**，在单次迭代（或一条 PR 链）中按此顺序执行：

```text
A. 后端（Kratos）
      ↓  make api 已提交、wire 已更新、go build 通过
B. 前端对齐（同一域）
      ↓  web/src/services + features/*/api + Store actions；去掉该域对 legacy `/api/v1` 的依赖
      ↓  **能组件化则组件化**：重复 UI 抽到 components/<域>/（§4 B8）
C. 设计与 UX（同一域触达页面）
      ↓  **严格按 UX.md** 做网页 UI（token / 玻璃 / 按钮卡片 / 导航 / Do·Don't）；优先改组件而非复制粘贴
```

- **不要**在未定 proto、未生成客户端的情况下，让前端长期手写新路径。  
- **不要**在展示组件里绕过 Store 直连 API（vue-design §0.2）。  
- **不要**为「省事」在 Page 内堆数百行模板；**同一模式出现 ≥2 次**就应评估抽组件（与 B5/B5b 一致）。  
- **UX 可与 B 并行设计稿**，但 **落代码** 建议在 **B 稳定** 后进行，避免重复改调用链。

**阶段 B 正文**：[`docs/vue-design/vue-design.md`](../vue-design/vue-design.md)（含迁移剧本 **§5** 与交付自检 **§4**）。本 Playbook **§4 仅为摘要**；实现与评审以 **vue-design 全文** 为准，冲突时按 **本文 §1** 优先级处理。

---

## 3. 阶段 A — 后端（ condensed from pkg-backend-to-kratos §5 + §9）

| 步骤 | 动作 |
|------|------|
| A1 | 读 `pkg/backend/internal/repository/migrations/0001_init.sql` 中本域表；定 **ID/时间** 在 proto、biz、Ent、TS 一致 |
| A2 | `api/kratos/<module>/v1/*.proto`：**本轮对外 HTTP 能力全部写进 proto**，配 `google.api.http`，路径习惯 **`/v1/...`** |
| A3 | 仓库根 **`make api`**，提交 **Go 生成物** + **`web/src/services/kratos/**`** |
| A4 | `internal/biz`：领域模型 + `Repo` 接口；**不要** `import api/...` |
| A5 | `internal/data`：**以 `ent.Client` 为主**；`NewData` **仅** `ent.Open`（禁止双开 SQLite）；对齐 `admin.go` 风格 |
| A6 | `internal/service`：实现 `*Server`，嵌入 `Unimplemented*` |
| A7 | `internal/server/http.go`（及 grpc）：**仅** `Register*HTTPServer` / `Register*ServiceServer`，**禁止**同域手写 `HandleFunc` 补漏 |
| A8 | `cmd/admin`：**wire** 更新，`go build ./cmd/admin` 通过 |
| A9 | 若存在 **multipart / 二进制直传 / 与旧 JSON 不完全一致**：在 PR 描述中单列 **兼容策略**，禁止仅靠私路由「悄悄对齐」（参见主文档 §2.4） |

**一票否决**：只改 `.proto` 不跑 `make api`；同一业务域契约分裂（一半 proto 一半手写路由）。

---

## 4. 阶段 B — 前端架构（condensed from vue-design）

| 步骤 | 动作 |
|------|------|
| B1 | `web/src/services/index.ts` 增加 `create<Module>Service()` → `create*Client(requestHandler)` |
| B2 | `features/<域>/api.ts`：**纯函数**封装生成客户端；复杂映射保留在 api 层，**不在** `.vue` 写裸 URL |
| B3 | **Pinia** `stores/<域>/`：异步、loading、error、列表真源进 **actions**；`stores/index.ts` **具名导出**，保留 **default export** Pinia 工厂 |
| B4 | Composable：`useXxx` **默认**只组合 Store；若暂直连 Service，须 `// TECH-DEBT: ...` |
| B5 | Page 瘦、展示组件 **仅 props/emits**，禁止 `useXxxStore` / `createFooService` 出现在纯展示组件 |
| B5b | **路径**：展示 `.vue` **必须**在 `components/<域>/`（见 `vue-design.md` §2「路径硬性」），不得长期留在 `features/<域>/` |
| B5c | **浮层**：Dialog/Drawer 等同域组件路径同 B5b；材质与强调色遵守 **`docs/UI/UX.md` §1～§2**（玻璃 `backdrop-filter` **与** `-webkit-backdrop-filter` 成对；日间主操作 **`--color-accent`**）；**禁止**在展示浮层 `script` 中直接调 **`features/*/api`**（只 **`emit`**，Page/Store 调 API） |
| B6 | 删除或缩小该域对 **裸 `/api/v1/...`** 的依赖；优先 **`create*Service` / `kratosApi`**；代理与 `getBackendOrigin()` 行为与运维约定一致 |
| B7 | 交付前自检：[vue-design §4 检查清单](../vue-design/vue-design.md#4-ai-开发迁移检查清单交付前必跑) |
| **B8** | **组件化（强制倾向）**：**能组件化则组件化**。同一域内至少 **出现两次**的区块（筛选条、表格工具栏、表格列模板、空状态、分页区、玻璃卡片外壳、`q-dialog` 内容骨架）应拆为 **`components/<域>/`** 下独立 `.vue`，由 Page **组合**；跨域可复用模式（如玻璃面板、指标胶囊）优先 **对齐已有页面**（如 Tools / Channels 迁移后的组件拆分）或抽到 **`components/common/`**（须经 `UX.md` §3 样式入口约定）。**禁止**单文件 Page 过长且不拆组件「以后再治」。 |

---

## 5. 阶段 C — 设计与 UX（condensed from UX.md + Quasar）

**设计系统**：[UX.md](../UI/UX.md) — 奶油昼 / 玻璃夜；所有浮层 **`backdrop-filter` + `-webkit-backdrop-filter`**，token 以 UX **§2** 为准。

**网页 UI 优化（本阶段含义）**：不是随意「美化」，而是把触达页面 **逐项对齐 `UX.md`**——实现前 **§1 强制自检**（玻璃双前缀、日间金盏花锚点、夜间霓虹用途边界）→ **§2 CSS 变量**（禁止页面硬编码 hex 取代 token）→ **§3 样式工程**（token / 全局类放哪里）→ **§4 排版** → **§5 组件数值**（按钮 / 卡片 / 对话框 / 输入 / 导航）→ **§7 布局**（间距与圆角刻度昼夜一致）→ **§8 Do / Don’t** → **§9 响应式**。阶段 C 的修改应 **落在阶段 B 已抽好的组件**上，避免仅在父级 Page 覆盖样式导致与 `UX.md` 分叉。

| 步骤 | 动作 |
|------|------|
| C1 | 在 `web/src/css/theme/_css-vars-*.sass` 与 `app-theme.sass` 维护 CSS 变量：`--canvas-base`、`--glass-surface`、`--color-accent`、`--focus-ring-light`、`--color-neon-cyan` 等（见 UX §2） |
| C2 | **深色模式**：Quasar `Dark` 与 `body.body--dark`；布局与圆角 **§7** 昼夜同一套 |
| C3 | **页面/布局**：顶栏、侧栏按 UX **§5.4** |
| C4 | **卡片 / 列表 / 按钮**：**§5.2** 玻璃优先；主按钮 **§5.1** |
| C5 | **交互状态**：日间 **§1**（金盏花、玻璃提亮）；夜间霓虹 **§1** 表格（不占满） |
| C6 | **移动端**：**§9**（blur 8–12px；光效降级） |
| C7 | 交付前自检：UX.md **§8** |

---

## 6. 模块总表（后端状态 + 建议前端落点）

**旧 HTTP 前缀**：`pkg/backend` 多为 **`/api/v1/...`**；Kratos 常见 **`/v1/...`**（由网关或 `axiosHandler` baseURL 统一，以现网为准）。

| 顺序 | 域 | `cmd/admin` / Kratos（目标） | 前端主要落点（现状供对照） | 后端 Playbook 状态 | 前端 B | UX C |
|------|----|------------------------------|----------------------------|-------------------|--------|------|
| 1 | Admin | `api/kratos/admin/v1` | `services` + admin 相关 pages | **已落地** | 维持规范 | 按需 |
| 2a | Avatar | `api/kratos/avatar/v1` | `features/avatar`, `stores/avatar` | **已落地** | 维持 | 按需 |
| 2b | Agent 分类 | `api/kratos/agent_category/v1` | `pages/AgentCategoriesPage` 等 | **已落地** | 维持 | 按需 |
| 2c | LLM 模型目录 | `api/kratos/llm_provider_model/v1` | `features/platform`, `ResourceManagerPage` | **已落地** | 维持 | 按需 |
| 2d | Hooks | `api/kratos/hook/v1` | `Ecosystem` / 平台页 | **已落地** | 维持 | 按需 |
| 2e | MCP Servers | `api/kratos/mcp_server/v1` | **`features/mcp/api.ts`** → **`kratosApi`** **`/v1/mcp-servers`**；`McpServersPage` | **已落地** | **已接 Kratos** | 按需 |
| 3 | **会话与聊天** | **`session/v1`**（列表等）；**`/v1/chat/*`** 已由 **`cmd/admin`** 显式注册（**`chat_legacy_forward.go`**）：上游 **`LEGACY_REST_ORIGIN`** → **`/api/v1/chat/*`**；原生 **`chat/v1`** 仍待 | `components/chat`, `features/chat/api.ts`, `stores/app` | **`session/v1`**：**会话 CRUD**、**timeline**、**消息列表**已 Ent；**发送 / SSE / options**：HTTP 入口在 **admin**，实现仍在上游（直至嵌入 ADK） | **列表已 Kratos**；**发送·流式·options** 路径 **`/v1/chat/*`**（与前端一致），**业务仍依赖上游** | **待做**（流式对齐 UX token；原生 **`chat/v1`**） |
| 4 | **Agent 目录**（CRUD、runtime settings、prompt 文件、preview） | `api/kratos/agent/v1` | `features/agents`, `stores/agents`（`kratosApi` `/v1/agents`） | **已落地** | **已接 Kratos** | **待做** |
| 5 | **Team** | `api/kratos/team/v1` | `features/teams/api.ts`（Kratos `/v1/teams`、`/v1/team-runs`；**`subscribeTeamRunEvents`** → **`/sse/team-run-events`**） | **已落地** | **已接 Kratos** | **待做** |
| 6 | **Cron 定时任务** | **`cron/v1`** + **`internal/cronrunner`**（**`cmd/admin`** 内调度；派发对话依赖 **`LEGACY_REST_ORIGIN`** → **`/api/v1/chat/messages`**） | `features/cron`（Kratos `/v1/cron-tasks`、`/v1/cron-task-runs`）；调度与同库 **`cron_task`** / **`cron_task_run`** 由 admin 进程负责 | **已落地** | **已接 Kratos** | **待做**（派发完全原生 **`chat/v1`** 后可去除 **`LEGACY_REST_ORIGIN`**） |
| 7 | 技能 / 工具 / 插件 / **通道** | **`plugin/v1` 已落地**；**`skill/v1` 已落地**（列表 / 启停 / 复制 / 删除 / 文件 / `skill-runs`；**`/v1/skills/import*`** 本进程 **`skillimport`**）；**`tool/v1`**、**`channel/v1`** 已落地（前端 **`features/tools`、`features/channels`** → 生成客户端） | **`plugins` + skills + tools + channels** 已 **`kratosApi`** | **catalog 面子域已收口**；**chat（含 Cron 派发）**依赖 **`LEGACY_REST_ORIGIN`**（见 **`pkg-backend-to-kratos.md` §6.3.1**）；插件运行时装配等仍可能依赖 **`pkg/backend`** | **维持** | **待做** |
| 8 | 记忆 / 进化 | **`memory/v1`**：`MemoryService` 读 **`internal/data/sessionmemory`**（与 Ent 共用 SQLite）；**不复用** **`LEGACY_REST_ORIGIN`** | `features/memory`, `MemoryCenterPage` | **读路径已原生**；写能力若需对齐旧栈须扩 proto | **`createMemoryService`** + **`kratosApi`** | **按需**（记忆 **写** RPC / UX） |
| 9 | 用量 / 监控 | **`usage/v1`** **`monitor/v1`**（读）；SSE 仍遗留 | **`features/usage`、`features/monitor`**（`api.ts`：`createUsageService` / `createMonitorService`）；**`components/monitor`**（监控页与各 Tab 展示）；Trace 用量事件走 **`usage/v1`** | **`usage/v1` + `monitor/v1`（读）已落地**；**SSE / 部分写入仍 pkg/backend** | **monitor 读路径已接 Kratos** | **待做** |

**维护约定**：每合并一域，将上表 **后端 / 前端 B / UX C** 更新为 **已落地**（或 **进行中**），并在 [`pkg-backend-to-kratos.md`](./pkg-backend-to-kratos.md) §6.3（及 **`LEGACY_REST_ORIGIN` / Cron 变量**：§6.3.1）保持同步。

**下一优先域**（与主文档一致）：**原生会话与聊天（步 3 续）**——在 **`cmd/admin`** 内实现 **`chat/v1`**（POST / SSE / options），摆脱 **`LEGACY_REST_ORIGIN`** / ADK 上游（届时 **Cron** 派发亦可不再依赖遗留 **`/api/v1/chat/messages`**）；其次 **记忆 / 进化 HTTP**（`memory.Register`、`evolution.Register`）。**Cron 调度循环**已在 **`cmd/admin`**（**`internal/cronrunner`**）。技能 ZIP 导入已在 **`cmd/admin`**。

---

## 7. 旧路由索引（便于对账）

来源：`pkg/backend/internal/transport/handler.go` `registerRoutes`。

| 分组 | 路径前缀（节选） |
|------|------------------|
| Agent | `/api/v1/agents`, `/api/v1/agents/`, `/api/v1/agents/validate-model` |
| Team | `/api/v1/teams`, `/api/v1/teams/`, `/api/v1/team-runs`, `/api/v1/team-run-events` |
| 平台资源（节选） | 旧 **`/api/v1/...`**：`agent-categories`、`llm-provider-models`、`avatar-assets`、`hooks`、`mcp-servers`；**`cron-tasks` / `cron-task-runs`**：**管理 UI** 已由 Kratos **`/v1/cron-tasks`** 等承接；**到期执行**由 **`cmd/admin`** **`internal/cronrunner`** 负责（派发 **`LEGACY_REST_ORIGIN`** → **`/api/v1/chat/messages`**，见 [`pkg-backend-to-kratos.md`](./pkg-backend-to-kratos.md) **§6.3.1**） |
| 通道 / 技能 / 插件 | **`/v1/channels*`**（**`channel/v1`**）；**`/v1/skills*`**、**`/v1/skills/import*`**（**`skill/v1`** + **`skillimport`**）；**`/v1/plugins*`（`plugin/v1`）** |
| 会话 / 聊天 | `sessions`，**`GET /v1/sessions/{id}/messages`**（Kratos）；**`POST /v1/chat/messages`**、**`POST /v1/chat/messages/stream`**、**`GET /v1/chat/options`**（admin 挂载；配置 **`LEGACY_REST_ORIGIN`** 时转发至 **`/api/v1/chat/*`**） |
| 探测 | **`GET /healthz`**（**`cmd/admin`** 与旧栈均返回 **`{"status":"ok"}`**，**无鉴权 cookie**） |
| 用量 / 监控 | `model-usage/*`, `monitor/*` |

迁移每个 RPC 时，在 proto 的 `google.api.http` 中写清 **方法与完整路径**，避免遗漏 stream、import 等特殊接口。

---

## 8. AI 单次会话任务卡片（复制模板）

```markdown
## 域：<name>

### A 后端
- [ ] 0001_init.sql 相关表已读
- [ ] proto 全量 RPC + HTTP 注解
- [ ] make api 已提交
- [ ] biz + data(Ent) + service + Register*HTTPServer
- [ ] wire + go build ./cmd/admin
- [ ] 与旧 transport 字节/字段差异已写明

### B 前端（须同时满足 [vue-design.md](../vue-design/vue-design.md) §0～§5 与 §4 自检）
- [ ] services/index.ts 已导出 create*Service
- [ ] features/<域>/api + store actions（数据流见 vue-design §0.1）
- [ ] 无展示组件直连 API/Store 违规（vue-design §0.2）
- [ ] Composable 直连 Service 已按 vue-design §1 标注 TECH-DEBT 或已上收到 Store
- [ ] 该域 legacy `/api/v1` 已移除或仅限兼容层
- [ ] **组件化**：重复 UI 已抽至 `components/<域>/`（本 Playbook §4 **B8**），Page 以组合为主

### C UX（[`UX.md`](../UI/UX.md) 全文对齐）
- [ ] **§1～§2**：玻璃双前缀、token（`var(--*)`），日间主强调 `--color-accent`、夜间霓虹边界清晰
- [ ] **§5～§7**：按钮/卡片/对话框/输入/导航与布局刻度符合文档数值；相关页已接 UX token（玻璃 / accent / 夜间霓虹）
- [ ] **§8～§9**：Do/Don’t 已自检；移动端 blur 与动效降级已顾及

### 回写
- [ ] Playbook §6 与 `pkg-backend-to-kratos` §6.3 / §6.3.1 状态已更新
```

---

## 9. 数据与并发风险提示（必读）

- 旧库大量 **TEXT UUID**；新 proto/Ent 须 **统一 string vs int64**，全链路一致。  
- **双进程期**（`pkg/backend` 与 `cmd/admin` 同库）：约定 **单写** 或 **只读副本**，避免 SQLite 锁冲突。

---

## 10. 附录：给 AI 的一句话系统指令

> 按 `docs/migration/AI-full-stack-migration-playbook.md` 顺序工作：先为该域补齐 Kratos proto、biz、Ent data、service 与 `make api`；**`web/` 前端迁移与分层必须遵守** [`docs/vue-design/vue-design.md`](../vue-design/vue-design.md)（含 Store / Composable / Page / 展示组件数据流、§5 迁移步骤与 §4 自检）——在 `web/src/services` 与 `features/<域>/api.ts` 对接生成客户端与 Pinia，**展示组件不得引用 Store/API**；**能组件化则组件化**（重复 UI 抽到 `components/<域>/`，见本 Playbook §4 B8）；再把本域页面与组件按 [`docs/UI/UX.md`](../UI/UX.md) **全文**（§1～§8 及响应式 §9）做**网页 UI 对齐**，奶油昼/玻璃夜 token 落到 Quasar；同时遵守 [`docs/API/接口与数据库开发规范.md`](../API/接口与数据库开发规范.md) 与 [`pkg-backend-to-kratos.md`](./pkg-backend-to-kratos.md) §2。

---

## 11. 复盘：本文能否单独指导 AI 完成迁移？

| 维度 | 说明 |
|------|------|
| **够用的部分** | 规范优先级（§1）、**固定阶段顺序 A→B→C**（§2）、后端逐步清单（§3）、前端摘要 + **`vue-design` 全文** + **§4 B8 组件化**（§4）、**`UX.md` 驱动的网页 UI 优化路径**（§5）、**域级总表 + 旧路由索引**（§6～§7）、可复制的任务卡（§8）、双进程风险提示（§9）、一句话系统指令（§10）。 |
| **必须配合 mother docs** | **Ent 字段级约定、SQLite 单连接、`make api` 参数、wire 写法**等仍以 `pkg-backend-to-kratos.md` 与 `接口与数据库开发规范` 为准；**组件谁能调 API** 以 `vue-design.md` **§0～§5** 为准——本文 §4 是摘要，**不能替代**该全文；**网页 UI 细则（token、玻璃、组件数值、布局、Do/Don't）** 以 **`UX.md` 全文** 为准——本文 §5 是摘要，**不能替代** `UX.md`。 |
| **AI 常见失效点** | ① 只迁 proto 未跑生成或未注册 HTTP/gRPC；② 前端只改入口文件未落到 `features/<域>/api`；③ 展示组件仍 import Store/API；④ §6 表与 §7 路由、旧 `handler` **未对账**，漏迁子路径；⑤ **部分子能力仍走旧栈** 未在 PR 说明；⑥ **未组件化**导致 Page 臃肿、阶段 C 只能在父级糊样式，与 **`UX.md`** 分叉；⑦ **只做局部配色**未按 **`UX.md` §1～§8** 系统对齐。 |
| **建议用法** | 将 **§0 + §10** 粘进会话；每域用 **§8** 勾选；拿 **§6** 选域、**§7** 对路径。复杂域（如会话/流式）另开 checklist 链接进 §6 备注列（可在表尾加「备注」列扩充）。 |

---

*文档版本：2026-04-29（增补：§4 B8 组件化、`UX.md` 驱动的网页 UI 优化约定）· §6 维护约定链至 **`pkg-backend-to-kratos` §6.3.1**；§7 平台资源行补充 Cron 执行路径；与三份母文档同源维护；母文档更新时同步核对 §6 与硬约束。*
