# Aranea-Agents 项目开发规则

> AI 每次开发必须遵守。本文件为精简版，详细规范见 SKILLs（冲突时以 SKILL 为准）。

---

## 一、项目概览

Aranea-Agents 是基于 trpc-agent-go 的多智能体编排平台。以 Kratos v2 为传输壳层、trpc-agent-go 为运行时内核。

**技术栈**：Go + Kratos v2（HTTP/gRPC/WebSocket）| trpc-agent-go（Agent 运行时）| Vue 3 + Quasar + Pinia + TypeScript | SQLite（Ent ORM）| Wire（编译期 DI）

**双框架分工**：
- Kratos v2：传输层（HTTP/gRPC/WebSocket）、配置、鉴权、中间件、Wire DI
- trpc-agent-go：Agent 编排（Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team）

**SKILL 体系**：

| SKILL | 定位 | 触发场景 |
|-------|------|----------|
| `aranea-coding-guide` | 后端项目编码指南（详细版） | 编写 Go 后端代码 |
| `go-oop-guide` | 通用 Go OOP 编程指导 | struct/接口/组合/工厂设计 |
| `go-oop-review` | Go OOP 代码审查 | 审查 Go 代码的 OOP 合规 |
| `aranea-frontend-guide` | 前端项目编码指南（详细版） | 编写 Vue 3/Quasar/Pinia/TS 代码 |
| `vue-frontend-guide` | 通用 Vue 3 编程指导 | 组件/Composable/TypeScript 设计 |
| `aranea-frontend-review` | 前端代码审查 | 审查前端数据流/分层/UX 合规 |
| `openspec-explore` | OpenSpec 探索模式 | 需求探索、问题分析、方案对比 |
| `openspec-propose` | OpenSpec 提案创建 | 新增变更提案（proposal+design+tasks） |
| `openspec-apply-change` | OpenSpec 实施执行 | 按 tasks.md 逐步实施变更 |
| `openspec-archive-change` | OpenSpec 归档 | 变更完成后归档、同步主规格 |
| `superpowers-workflow` | 开发纪律强制 | 实施阶段：TDD+两阶段审查+验证前置 |

> 本文件为精简速查，SKILLs 为完整规范。内容冲突时以 SKILL 为准。

---

## 二、架构铁律（违反即停）

### 2.1 依赖方向

```
api/**/*.proto          ← 唯一对外契约
        ↓
internal/service        ← 传输桥点：proto ↔ biz 映射 + Runner 编排
        ↓
internal/biz            ← 领域模型 + Usecase + Repo 接口定义
        ↓
internal/data           ← Repo 实现（Ent ORM + SQLite）
```

**跨层只允许向内依赖。违反即停。**

**禁止修改工具生成的代码**：protoc（`make api`）、wire（`make wire`）、Ent（`go generate`）等工具生成的文件不得手动编辑；改源头 → 重新生成 → 提交生成物。

### 2.2 后端红线

> 完整 19 条红线及详细说明见 `aranea-coding-guide` SKILL。

| # | 红线 | 正确做法 |
|---|------|----------|
| 1 | `internal/biz` 不得 import `pkg/trpc-agent-go` 任何包 | 框架交互通过 `internal/agent`/`internal/tools` 桥接 |
| 2 | `internal/biz` 不得 import `api/*/v1` proto 包 | proto 映射只在 Service 层；biz 定义端口接口 |
| 3 | Runner 装配只在 `internal/service` | `internal/server` 不得 new `runner.Runner` 或 `llmagent.New` |
| 4 | Service 层不得写业务逻辑 | Service 只做 proto↔biz 映射 + Runner 编排 |
| 5 | Server 层不得写业务路由 | 只做 `Register*HTTPServer`/`Register*ServiceServer` |
| 6 | 不得修改工具生成的代码（protoc/wire/Ent 等） | 改源头 → 重新生成 → 提交生成物（见 §2.1 架构铁律） |
| 7 | 跨模块调用不得持有对方 Service 具体类型 | 通过 biz 级窄接口（端口）交互，Wire 绑定在 Service 层 |
| 8 | 框架 plugin 回调不得直接写数据库 | 经 broker/async 异步写 |
| 9 | 所有 `go func()` 必须走 `pkg/safego` | 禁止裸 `go func()` 不处理 panic |
| 10 | 禁止使用 `log/slog` 记录日志 | 统一使用 `pkg/loggateway.Logger`（`lg.Info/Warn/Error` + `loggateway.StepID/Err/Str`） |
| 11 | 不得在 `NewData` 外另开 SQLite 连接 | 仅通过 `d.Ent()` 访问 SQLite |
| 12 | 不得新增已无调用者的 deprecated 方法 | 死代码即删 |
| 13 | Service 层不得直接依赖 Repo 接口 | 通过 Usecase 层访问，Repo 接口只在 biz/data 层出现 |
| 14 | 压缩操作必须通过 CAS + 事务保证原子性 | `TryIncrementCompressVersion` + `CompressSessionInTx` |
| 15 | Repository 接口方法不得超过 5 个 | 超过则按职责域拆分为子接口（`SessionReader`/`SessionWriter`/`MessageReader` 等） |

### 2.3 前端红线

> 完整 14 条红线及详细说明见 `aranea-frontend-guide` SKILL。

| # | 红线 | 正确做法 |
|---|------|----------|
| 1 | 展示组件不得 import `useXxxStore` / `defineStore` | 状态与请求收敛在 Store |
| 2 | 展示组件不得 import `features/*/api`、`services/`、`axios`、`kratosApi` | 网络请求只在 Store action 中触发 |
| 3 | 展示组件不得 `watch` + fetch + `ref` 存跨组件共享的业务数据 | 应进 Store |
| 4 | Dialog / Drawer / 浮层组件不得在组件内直接调 API | `emit('submit', payload)`，由 Page 或 Store action 调 API |
| 5 | 展示组件 `.vue` 放 `components/<域>/`，禁止放在 `features/<域>/` | `features/<域>/` 只放 api.ts、composable、容器组件 |
| 6 | 新 Store 必须在 `stores/index.ts` 具名导出，不得删除 default export Pinia 工厂 | 保持 Quasar Pinia 安装方式一致 |
| 7 | 新 HTTP 调用必须写在 `features/<域>/api.ts`，经 `services/index.ts` 的 `create*Service()` 或 `kratosApi` | 禁止在 `.vue` 中写裸 URL 或散装 `axios` |
| 8 | 浮层视觉必须遵守 UX 规范：`backdrop-filter` + `-webkit-backdrop-filter` 成对；主按钮用 `--color-accent` | 禁止日间用夜间霓虹青紫作默认强调 |
| 9 | 禁止运行时用脚本改 `quasar-variables` | 昼夜仅用 Dark + CSS 变量 + body 选择器 |
| 10 | 禁止与 `app-global.sass` 平行的第二套全局 CSS 入口 | 新 token 只在 `theme/` 增加 partial，由 `app-theme.sass` 聚合 |
| 11 | 跨 Store 同步必须通过 `stores/sessionSync.ts` 事件总线 | 禁止直接 import 另一 Store 导致循环依赖 |
| 12 | Page 不得直接 `import` `features/*/api` | 请求经 Store action；编排经 composable |
| 13 | 展示组件从 `features/<域>/api.ts` 引类型 | 共享类型放在 `features/<域>/types.ts`，组件只 import types |
| 14 | 单页 `*Page.vue` 的 `<script setup>` 不宜超过 ~200 行 | 拆 Dialog 组件 + 域内 composable + 子面板组件 |
| 15 | 前端禁止使用 `turn_index` 做消息分组，必须使用堆栈模型 | `groupMessagesByTurn` 按 `role=user` 边界 + 时间顺序 |

---

## 三、各层职责与约定

### 3.1 后端分层

| 层 | 职责 | 关键约定 |
|----|------|----------|
| **Server** | 传输注册 + 中间件 | 只做 `Register*HTTPServer`，中间件统一在此注册 |
| **Service** | proto↔biz 映射 + Runner 编排 | 类型转换 `toProtoXxx`/`fromProtoXxx`，错误映射用 `kerrors` |
| **Biz** | 领域模型 + Usecase + Repo 接口 | 纯 Go struct，错误用 `kerrors`，禁止 `fmt.Errorf` |
| **Data** | Repo 实现（Ent ORM） | 仅通过 `d.Ent()`/`d.Postgres()` 访问，转换函数 `entXxxToBiz`/`bizXxxToEnt` |

### 3.2 Agent 运行时模块

```
internal/service        ← Runner 装配入口
internal/agent          ← Agent 构建（BuildLLMAgent、Memory、Plugins）
internal/team           ← Team 工作流
internal/tools          ← 工具注册中心 + Assemble 装配
internal/provider       ← LLM 模型驱动
internal/memory         ← 会话记忆
internal/session        ← 会话存储
internal/graph          ← 图编排
internal/channel        ← 渠道集成
internal/cronrunner     ← 定时任务
```

**框架真相源**：`pkg/trpc-agent-go` 是 Agent 框架的唯一真相源。先查框架 API 后再实现，不复制框架内部逻辑。

**Agent 运行时集成铁律**：

| # | 铁律 | 正确做法 |
|---|------|----------|
| A1 | 所有 Agent 必须实现 `agent.Agent` 接口（5 方法） | `Run/Tools/Info/SubAgents/FindSubAgent` |
| A2 | 事件发射必须走 `agent.EmitEvent(ctx, inv, ch, evt)` | 禁止 `event.EmitEvent(context.Background(), ch, evt)` |
| A3 | Agent.Run() 内部不得发射 `ObjectTypeRunnerCompletion` | Runner 层统一发射完成事件 |
| A4 | 后台/定时 Agent 必须通过 `Runner.Run()` 调用 | 参考框架 `openclaw/internal/cron/service.go` 模式 |
| A5 | 工具构建使用 `function.NewFunctionTool[I, O]` | 禁止手动实现 `CallableTool` 接口 |
| A6 | 程序化 Agent（非 LLM 驱动）也必须走 Runner | Runner 管理 Session/Invocation/事件流生命周期 |

**工具装配**：新增工具先在 `Registry()` 注册 `ToolRegistration` + `builtin_tools_seed.go` 种子，Chat/Team 共用同一 `BuildToolsets` 逻辑。

**记忆系统**：记忆工具通过 `memory.Service.Tools()` 注入，记忆写入经 broker/async 异步写。

**Provider 集成**：厂商连接收口在 `internal/provider`，契约对齐以 `pkg/trpc-agent-go/model` 为准。

### 3.3 前端数据流

```
services/index.ts (createXxxService)
  → features/<域>/api.ts (HTTP 门面 + 类型归一化)
    → stores/<域>/index.ts (状态 + action 调 api)
      → features/<域>/useXxxPage.ts (composable 组合 Store)
        → pages/XxxPage.vue (布局 + 传参)
          → components/<域>/*.vue (纯展示：props in / emits out)
```

**CSS 主题**：单入口 `app-theme.sass` 聚合 partial；昼夜切换用 CSS 变量 + Quasar Dark mode；禁止与 `app-global.sass` 平行的第二套全局 CSS 入口。

---

## 四、决策树（代码该放哪？）

### 后端

```
新增 HTTP/gRPC 接口  → api/**/*.proto → internal/service → internal/server
新增业务逻辑        → internal/biz（模型 + Repo 接口 + Usecase）
新增数据库表/查询   → internal/data/ent/schema → go generate → internal/data
新增 LLM Agent 能力 → internal/agent（BuildLLMAgent 扩展）
新增程序化 Agent    → internal/agent（实现 agent.Agent 接口 + Runner 包装）
新增工具           → internal/tools（Registry 注册 + Assemble 装配）
新增 Team 工作流   → internal/team（BuildWorkflowRoot）
新增 LLM 厂商      → internal/provider（实现 model.LLM）
新增记忆能力       → internal/memory（适配器 → trpcmemory.Service）
新增定时同步任务   → internal/agent（Agent）→ internal/cronrunner（调度）→ cmd/admin/wire.go（装配）
新增横切关注点     → internal/server + pkg/auth
```

### 前端

```
新增 HTTP 请求       → features/<域>/api.ts（经 services/ 或 kratosApi）
新增业务状态/加载    → stores/<域>/index.ts（action 内调 api）
多页面复用逻辑      → composables/useXxx.ts（组合 Store）
单页编排过重        → features/<域>/useXxxPage.ts 或 useXxxPanel.ts
新增 Dialog / 浮层  → components/<域>/*Dialog.vue（props/emits）
新增展示组件        → components/<域>/*.vue（仅 props/emits）
新增页面           → pages/**/*Page.vue（布局 + composable + 传参）
新增 CSS 变量       → css/theme/_css-vars-light.sass + _css-vars-dark.sass
```

---

## 五、Go 编码约定

### 命名

- 包名：小写单词，不用下划线（`agent`, `mcp/config`）
- 结构体/接口：大驼峰，名词（`AgentUsecase`, `AgentRepository`）
- 函数：大驼峰导出/小驼峰内部（`NewAgentUsecase`, `fromProtoRuntime`）
- 错误变量：`Err` 前缀（`ErrNotFound`）

### 错误处理

统一使用 `kerrors`，禁止 `fmt.Errorf` 返回业务错误：

```go
kerrors.BadRequest("AGENT", "id is required")
kerrors.NotFound("AGENT", "agent not found")
kerrors.InternalServer("AGENT", err.Error())
```

### 依赖注入

- Wire ProviderSet：每层一个（`biz.go`/`data.go`/`service.go`/`server.go`）
- 构造函数参数：只接收接口或具体依赖，不接收"上帝对象"
- 禁止手动编辑 `wire_gen.go`，必须通过 `make wire` 生成

### 并发

- 所有跨层调用必须传递 `ctx`
- goroutine 必须走 `pkg/safego.Go` / `pkg/safego.GoRecover`
- 共享状态用 `sync.Mutex`/`sync.RWMutex`，不用全局变量

---

## 六、数据库编码规范（SQLite + Ent ORM）

> 源自 `docs/sqlite问题和解决方案.md` 的核心设计原则，所有 data 层开发必须遵守。

### 6.1 Schema 管理

- **单一 Schema 真相源**：所有表必须进 Ent Schema，`go generate` 是唯一的 Schema 演进方式
- **禁止野生表**：不得在 Ent Schema 之外通过 Raw SQL 创建新表
- **Ent 不支持的特性**（FTS5、pgvector、`BEGIN IMMEDIATE`）：在 Ent Schema 中标注 `Annotations`，用 Raw Query 补充但不另建表
- **新增数据库表/查询**：`internal/data/ent/schema` → `go generate ./internal/data/ent` → `internal/data`

### 6.2 数据访问模式

**Raw SQL → Ent Repo 迁移策略**：

| 场景 | 方案 |
|------|------|
| 简单 CRUD | 直接替换为 Ent API |
| 复杂查询 | 保留 `ent.Client.QueryContext()`，但用 Ent 生成的类型做结果映射 |
| SQLite 特有语法 | 通过 Ent 的 Raw Query + 类型映射保留 |

**Ent 无法覆盖的场景**：

| 场景 | 方案 |
|------|------|
| `ON CONFLICT DO UPDATE WHERE` | Ent 的 `OnConflictColumns` + `UpdateSet` |
| `INSERT OR IGNORE` | Ent 的 `OnConflictColumns` + 不更新 |
| `json_set()`/`json_remove()` | 保留 Raw SQL，但封装为 Repo 方法 |
| FTS5 全文搜索 | 保留 Raw SQL（Ent 不支持 FTS5） |
| pgvector 向量搜索 | 保留 Raw SQL（Ent 不支持向量） |
| `BEGIN IMMEDIATE` | 保留 Raw SQL（Ent 不支持事务隔离级别） |
| 50+ 列大表 | Ent 生成后用 `SetXxx()` 链式调用 |

### 6.3 Repo 接口规范

- **方法数上限**：每个 Repo 接口 ≤ 5 方法（红线 #15）
- **拆分维度**：按读写职责拆分（`XxxReader`/`XxxWriter`），或按业务子域拆分（`TeamRunRepo`/`OrchestrationStepRepo`）
- **Wire 绑定**：按需注入窄接口，消费方只看到自己需要的方法
- **接口定义位置**：biz 层定义接口，data 层实现

### 6.4 事务管理

- **统一事务接口**：一套 `TransactionManager` 覆盖 Ent + Raw SQL，通过 context 传播事务对象
- **Raw SQL Repo 从 ctx 获取事务**：优先使用 `ctx.Value(rawTxKey{})` 获取已开启的事务，无事务时回退到 `d.RawDB()`
- **压缩操作**：必须通过 CAS + 事务保证原子性（`TryIncrementCompressVersion` + `CompressSessionInTx`）

### 6.5 读写分离

- **SQLite 双连接**：写连接 `entClient`（`MaxOpenConns=1`），读连接 `readClient`（`MaxOpenConns=2`）
- **Ent Repo**：`readClient(ctx)` 读、`txClient(ctx)` 写
- **Raw SQL Repo**：读操作用 `d.ReadDB()`，写操作用 `d.RawDB()`
- **连接收口**：不得在 `NewData` 外另开 SQLite 连接（红线 #11）

### 6.6 Schema 迁移

- **框架化迁移**：所有 Schema 变更（包括 `ALTER TABLE ADD COLUMN`）纳入统一迁移框架
- **迁移要素**：有版本号、有依赖顺序、可回滚
- **禁止散落 patch**：不得新增 `*_patch.go` 模式的迁移，统一走迁移框架

---

## 七、模块间通信

| 方式 | 正确 | 错误 |
|------|------|------|
| 同步调用 | Usecase 之间通过接口调用 | 直接 import 另一模块的 data |
| 异步事件 | 通过 `Broker` 发布/订阅 | 通过全局变量共享状态 |
| 跨模块调用 | 通过 biz 级窄接口（端口） | 持有对方 Service 完整具体类型 |

**端口设计**：接口定义在 biz 层，Wire 绑定在 service 层，返回值用 biz 类型不返回 proto 类型。

---

## 八、验证

| 改动类型 | 最小验证 |
|----------|----------|
| 仅 Service + 单测 | `go test ./internal/service/... -run TestXxx -count=1` |
| 仅 Biz / Data | `go test ./internal/biz/... ./internal/data/... -count=1` |
| Proto 变更 | `make api && go build ./...` |
| Wire 注入 | `make wire && go build ./cmd/admin` |
| 前端 | `cd web && pnpm lint && pnpm test && pnpm build` |
| **提交前（全量）** | 后端：`make api && make wire && make build && make test && make lint`；前端：`cd web && pnpm lint && pnpm test && pnpm build` |

---

## 九、代码审查纪律

- 代码审查**必须使用项目 SKILL**（`go-oop-review` / `aranea-frontend-review`），不可仅依赖内置通用审查
- 通用审查（如 `TRAE-code-review`）只能作为补充，项目红线和业务规则检查以 SKILL 为准
- 后端审查必查：19 条红线、分层合规、框架集成、OOP 合规
- 前端审查必查：14 条红线、数据流合规、组件分层、聊天消息分组、UX 主题

---

## 十、任务执行纪律

- 有任务 ID 时：只读对应 development.md / blueprint 中该 ID 块
- 列假设 → 编码 → 分级验证 → 通过后再扩 scope
- 只改与任务直接相关的文件；不顺带 refactor 相邻模块

### 10.1 OpenSpec + Superpowers 工作流（推荐）

**新变更必须走 OpenSpec 流程**，开发纪律由 Superpowers 强制执行：

```
┌─────────────────────────────────────────────────────────────┐
│                  OpenSpec + Superpowers 工作流               │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  1. EXPLORE  → openspec-explore 技能                        │
│     需求探索、问题分析、方案对比（只思考不编码）              │
│     ↓                                                        │
│  2. PROPOSE  → openspec-propose 技能                        │
│     生成 proposal.md + design.md + tasks.md                  │
│     ↓                                                        │
│  3. APPLY    → openspec-apply-change + superpowers-workflow  │
│     TDD 强制：RED → GREEN → REFACTOR                        │
│     两阶段审查：规格合规 → 代码质量                          │
│     验证前置：提供证据才能声明完成                           │
│     ↓                                                        │
│  4. ARCHIVE  → openspec-archive-change 技能                 │
│     归档变更、同步主规格库                                   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

**目录约定**：
- 活跃变更：`openspec/changes/<change-name>/`
- 主规格库：`openspec/specs/`
- 已归档：`openspec/changes/archive/`
- CLI 命令：`openspec list` / `openspec status` / `openspec new change <name>`

**Superpowers 纪律**（实施阶段强制）：
1. **TDD 强制**：先写失败测试 → 最小实现 → 重构（hotfix/typo/CSS 除外）
2. **两阶段审查**：先过规格合规（是否满足 proposal+design+specs），再过代码质量（红线+OOP+分层）
3. **验证前置**：测试通过 + lint 通过 + build 通过 + 无红线违反 = 才能声明完成
4. **DoD 模板**：每个 task 必须有 Definition of Done

---

## 十一、模块关联强制读取（违反即停）

> **任何模块开发前必须先读关联文档。** 模块不是孤岛，改一处必知影响面。

### 11.1 文档索引

| 文档 | 路径 | 定位 |
|------|------|------|
| **架构蓝图** | `docs/architecture-blueprint.md` | "每个模块是什么"（静态结构、全貌） |
| **模块交叉参考** | `docs/module-cross-reference.md` | "改模块 X 时必须注意谁"（动态关联、影响面） |

### 11.2 强制读取规则

**开发任何后端/前端模块时，AI 必须：**

```
步骤 1：定位目标模块 → 读蓝图对应章节（了解静态结构）
步骤 2：读交叉参考手册 → 找到目标模块卡片（8 维度关联）
步骤 3：查变更影响表 → 确定需要同步修改的文件清单
步骤 4：按依赖方向逐层修改 → 验证时覆盖所有影响面
```

**具体场景速查：**

| 你要做什么 | 必须读的交叉参考章节 |
|-----------|---------------------|
| 新增/修改 biz 端口接口 | §四·端口接口变更影响表 + 目标模块卡片「下游影响」 |
| 新增/修改共享类型（DTO/struct） | §四·共享类型变更影响表 + 目标模块卡片「共享类型」 |
| 新增/修改事件类型（Envelope/EventBus） | §四·事件类型变更影响表 + 目标模块卡片「事件生产/消费」 |
| 新增/修改数据库 Schema | §四·数据库 Schema 变更影响表 + 目标模块卡片「数据库」 |
| 新增 LLM Provider | §五·场景演练 #1（7 个模块链路） |
| 新增工具 | §五·场景演练 #2（6 个模块链路） |
| 新增渠道平台 | §五·场景演练 #3（5 个模块链路） |
| 新增 Envelope 类型 | §五·场景演练 #4（前后端 4 层链路） |
| 修改 TurnInput/TurnResult | §五·场景演练 #5（最大影响面 8 模块） |
| 前端新增 Store/Page/组件 | §三·前端模块上下文卡 + §六·前后端对齐表 |

### 11.3 违反后果

- **未读交叉参考即编码** → 导致遗漏同步修改 → 编译失败 / 运行时 panic / 前后端不一致
- **典型遗漏案例**：
  - 改了 biz 接口签名但忘了 data 层实现 → 编译不通过
  - 改了 AssemblyConfig 但忘了 service/chat 的 Wire 构造函数 → 运行时 nil pointer
  - 新增 Envelope 类型但没加前端 dispatcher 处理 → WS 消息丢失
  - 加了 Ent 字段但没跑 go generate → ORM 不认识新列
