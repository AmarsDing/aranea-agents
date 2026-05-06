# AI 全栈新功能开发规范

适用范围：**`cmd/admin`（Kratos + Wire + Ent）** 与 **`web/`（Vue 3 + Quasar + Pinia + TypeScript）** 上的新能力与迭代。

**规范冲突时的优先级**：① 接口契约与持久化（proto、单层 SQLite、禁止业务域手写路由）→ ② 前端分层（展示组件禁令、Pinia/API 流向）→ ③ 视觉与交互 token（奶油昼·玻璃夜，变量与数值以本文「UI·UX」章为准）。

---

## 第一部分：后端分层与契约

### 1.1 分层与依赖方向

```
api/**/*.proto （唯一对外契约：HTTP + gRPC）
        ↓  make api / protoc 生成 *.pb.go、*_grpc.pb.go、*_http.pb.go、web/src/services/ TypeScript）
internal/service  （实现 *Server / AdminServiceServer 等，转调 biz）
        ↓
internal/biz      （领域模型、Usecase、Repo 抽象接口、paging/filter 组装）
        ↓
internal/data     （Ent + SQLite CRUD / pgvector Repo 实现）
```

**禁止**：`data` 直接依赖 `service` / `proto`；`biz` 不导入 `api/*/v1`。跨层只允许 **向内**依赖。

仓库现状提示：HTTP JSON 挂载在 **`/v1/...`**，由生成的 `*_http.pb.go` 与前端的 `axios` `baseURL`（通常经代理落到同源 `/v1/...`）配合。

---

### 1.2 新增一对「HTTP(JSON) API」的标准流程

#### 编辑 Proto

- 路径：`api/kratos/<module>/v1/<module>.proto` 。
- 使用 **Google API HTTP 注解**：`google/api/annotations.proto` 中为每个 RPC 配置 `post:` / `get:` 等与 **`body`**（按需 `"*"` 或指定嵌套消息字段，与存量 CRUD 服务一致）。
- 必填语义：使用 **`(google.api.field_behavior) = REQUIRED`**，与校验中间件一致。
- **列表**：与存量对齐时使用 **`internal/biz/pagination.go`** 的 `ListOption`，以及 **`github.com/go-kratos/aip-go/ents`** 的 **`ApplyFilter` / `ApplyOrderBy`**（对照 `internal/service/admin.go`、`ListAdmins`）。

#### 生成代码（仓库根）

1. **`make init`**（首次或未装插件时）。
2. **`make api`**：生成  
   - `./api/**`：`*.pb.go`、`*_grpc.pb.go`、`*_http.pb.go`  
   - **`web/src/services/**`**：`protoc-gen-typescript-http` 输出（路径与 `Makefile` 中 `--typescript-http_out` 一致）。  
3. 若只改 **`internal/conf`**：**`make config`**。

合并入 PR 时请包含生成的 **Go + TS**，避免 CI 与本地不一致。

#### 实现服务端

| 步骤 | 位置 | 说明 |
|------|------|------|
| 嵌入未实现 RPC | `internal/service/*.go` | `struct XxxService struct { embed v1.UnimplementedXxxServer; uc *biz.Usecase }`，实现新方法：将 `proto` Request 转成 `biz` 入参，`biz` 结果转 proto（对照存量 `convertAdmin`、`timestamppb`）。 |
| 业务逻辑 | `internal/biz/` | 扩写或新增 Usecase；错误用 **`errors`** / **`github.com/go-kratos/kratos/v2/errors`** 与存量风格一致；列表用 **`pagination.go`** 的 **`ListOption`**。 |
| 注册 HTTP | `internal/server/http.go` | **`v1.RegisterXxxHTTPServer(srv, service实例)`**。 |
| Wire | `biz.ProviderSet`、`service.ProviderSet`、`data.ProviderSet`；`cmd/admin/wire.go` 不手写实现，改动后执行 **`go run github.com/google/wire/cmd/wire ./cmd/admin`** 生成 **`wire_gen.go`**。 |

#### 服务端实现形态小结

| 层 | 职责 |
|----|------|
| `internal/service` | 嵌入 **`Unimplemented*Server`**；proto ↔ biz 映射 |
| `internal/biz` | 领域模型、**`XxxRepo`** 接口、**`XxxUsecase`**；**不**引用 proto |
| `internal/data` | Ent Repo；**`convert*`**；SQLite 仅经 **`NewData` 打开的同一个 `*ent.Client`** |

---

### 1.3 错误码、分页、校验（与存量对齐）

- **HTTP**：Kratos 错误链；中间件：`internal/server/http.go` 中 **`recovery`**、**`validate`**。
- **列表**：Biz 传入 **`biz.ListOffset` / `ListLimit` / `ListFilter` / `ListOrderBy`**（见 **`internal/biz/pagination.go`**、**`internal/service/admin.go`**）。

---

### 1.4 数据库：SQLite（Ent）+ 可选 Postgres（向量）

#### 原则

| 用途 | 技术 | 说明 |
|------|------|------|
| 后台业务表、用户配置缓存 | **SQLite + Ent** | `internal/data/ent/schema/*.go` 建模；开发环境 **`DEPLOY_ENV=dev`** 等与 Makefile 既定流程触发迁移 |
| 向量记忆（按维度拆表） | **Postgres pgvector** | `internal/data/pgvector`、`internal/data/memory.go` 等；用户维度偏好等仍存 SQLite。**不要**在同一套 Ent Client 上使用 Postgres |

#### 新增实体（Ent）

1. 在 **`internal/data/ent/schema/`** 添加 **`XXX.go`**，定义 Fields（及 Index、Edge 如需）。  
2. **`go generate ./internal/data/ent`**（或项目 Makefile 中带 `go generate ./...` 的目标）。  
3. 实现 **`internal/data/<entity>.go`**：类型 **`xxxRepo`** 实现 **`biz.XxxRepo`**，`NewXxxRepo(d *data.Data)` 内使用 **`d.Ent()`**（与 **`Data.entClient`** 同一指针，仅 SQLite）。  
4. 在 **`internal/biz/`**：定义 **`Xxx` 模型 + `XxxRepo` interface + `XxxUsecase`**；对外暴露再在 proto 中加 RPC。

#### Biz 仓储接口契约

- 接口放在 **`internal/biz`**（如 **`AdminRepo`**）。  
- 实现放在 **`internal/data`**，`admin.go` **仅依赖** `biz` 与 **`internal/data/ent`**。  
- 列表：**`ListXXX(ctx, ...ListOption)`** 与 **`pagination.go`**、**`ents.ApplyFilter`** 一致。

#### Wire

- **`data.ProviderSet`**：`NewData`、`NewXxxRepo`。  
- **`biz.ProviderSet`**：`NewXxxUsecase`。  
- **`service.ProviderSet`**：`NewXxxService`。  
- 重新生成 **`cmd/admin/wire_gen.go`**。

---

### 1.5 迁移与迭代时的硬约束（proto · data · HTTP）

#### 协议（`api/**/*.proto`）

1. **对外能力必须在 Proto 中印全**：同一业务的 **`service`** 应列出该域在 **`/v1/...`** 暴露的 **全部 RPC**（列表、创建、二进制/按需下载、删除等），并配以 **`google.api.http`**。**禁止**「一半在 proto，一半用手写 **`srv.Route` / `HandleFunc` / `HandlePrefix` / 独立 `*_route.go`」——否则契约分裂、生成 TS 缺失、中间件链路不一致。
2. **修改 `.proto` 必须跑生成**：根目录 **`make api`**，提交 **`*.pb.go`、`*_http.pb.go`、*_grpc.pb.go`** 与 **`web/src/services`**。**禁止**只改契约不重生。

#### 持久化（`internal/data`）

1. **SQLite 侧以 `*ent.Client` 为主入口**：经 **`NewData` 打开的 `Ent()`**，Repo 持有 **`*data.Data`** ；风格对齐 **`internal/data/admin.go`**（Query / Create / Update、实体 **`convert*`** → `biz`；包内可用 **`r.data.entClient`**）。
2. **`NewData` 不另建 SQLite 连接**：只通过 **`ent.Open(driverName, dsn)`** 得到 **`Data.entClient *ent.Client`**。**禁止**在 `NewData` 里再 **`sql.Open` 同一 DSN**，再用 **`entgo.io/ent/dialect/sql`（`OpenDB`）** 包装成第二套 **`*ent.Client`**（勿「并联池化」SQLite）。`Data` 只保留 **`entClient`**（及可选 **`pg`** / **`vectorDim`** 等）。跨表校验应 **补 Ent schema**，用 **`Query().Where(...).Count(ctx)`**，不要为 raw SQL 再挂一个 **`*sql.DB`**。
3. **表结构进 Ent**：**`internal/data/ent/schema`** 声明实体；禁止长期平行维护「仅存 SQL、不进 Ent」而无说明。
4. **禁止在非 `NewData` 场景下无理由并联同一 SQLite 的第二套 `database/sql`**：若某路径既不能建 Ent、又必须用驱动执行少量 SQL，须在 PR 书面说明理由与收口。
5. **复杂 WHERE / BLOB**：优先 **`predicate` + `dialect/sql`**（如 **`ExprP`**、**And/Or`**），避免整页复制裸露 SQL 与 Ent 分叉。

#### HTTP / gRPC 挂载（`internal/server`）

1. 业务模块 HTTP **只做 **`Register<Module>HTTPServer(srv, svc)`**，gRPC **只做 **`Register<Module>ServiceServer`**。**禁止**在同一业务域叠加 **未写入 proto** 的手写路由。
2. **横切路由**（健康检查、网关、探测等）单独列出，**不**充当业务 **`FooService` 的补丁契约**。

#### 与旧 HTTP 的差异（须在 PR/说明中显式写明）

遗留栈常有 **multipart、application/octet-stream 直连**；默认 Kratos/JSON 下 **`bytes`** 常为 **base64**。若必须与旧客户端 **字节级兼容**，须在说明中单写 **兼容性策略**，**禁止**仅靠未记入 proto 的私搭路由「悄悄对齐」。

---

### 1.6 横切与运维相关 HTTP 边界

- **`GET /healthz`**：在 **`cmd/admin`** 挂载，响应 **`{"status":"ok"}`**；常与鉴权的 **`noAuthPaths`** 放行配合探针。

**环境变量（与遗留上游分工）**

| 变量 | 作用 |
|------|------|
| **`LEGACY_REST_ORIGIN`** | 上游根 URL（**无**尾部 **`/`**）。① **`chat_legacy_forward`**：将 **`POST /v1/chat/messages`**、**`POST /v1/chat/messages/stream`**、**`GET /v1/chat/options`** **反向代理**到 **`{origin}`** + 遗留路径推导值。② **`internal/cronrunner`**：到期任务 **`POST`** **`{origin}` + 遗留 Messages 路径**。未设置：`/v1/chat/*` 可能 **503**；Cron 派发失败记入 **`cron_task_run`**。 |
| **`CRON_RUNNER_INTERVAL`** | Cron tick，**`time.ParseDuration`**；空或非法默认 **`1m`**。 |
| **`CRON_RUNNER_DISABLED`** | 设为 **`1`** 则不启动 **`internal/cronrunner`**（仅 **`cron/v1` CRUD**）。 |

**竞态**：若仍存在另一进程的 Cron，其数据源可能与 Ent **`cron_task`** **不同源**；避免同一业务任务在两侧重复配置，并明确派发责任方。

---

### 1.7 用量上报：浏览器 ingest 与「语义双写」

**HTTP**：**`POST /v1/usage/token-events`**，请求体为完整 **`TokenUsageEvent`**。**`ctx.Bind`** 使用 **`encoding/json`** 标签，字段名为 **snake_case**（如 **`occurred_at`**、**`agent_id`**、**`total_cost_micro_usd`**）。

**前端注意**：`**protoc-gen-typescript-http`** 生成的默认体可能对嵌套消息用 **`JSON.stringify`** 产出 **camelCase** 键，与 Go **`json`** 标签不一致，导致**静默丢字段或校验失败**。此类接口应在 **`features/<域>/api.ts`** 中用 **`kratosApi.post`** 等 **显式构造 snake_case**，并在源码注释写明原因。

**单一写入方（避免重复计数）**

| 场景 | 说明 |
|------|------|
| **常见风险** | 对话完成已由**后端**写入 **`model_token_usage_events`** 时，若在浏览器 **`onDone` / SSE 结束** 再 **`POST /v1/usage/token-events`**，会对同一轮交互 **重复插入**。 |
| **目标态** | **仅服务端**在同一请求路径写入用量时，浏览器**不应**再报同一事件。 |
| **例外** | 仅当服务端**确认从不写入**且不重叠会话/`id` 时，才可单独浏览器上报；须在 PR 写明。 |
| **过渡** | 若须二选一并行，应有 **feature flag**，默认只开一侧。**禁止**在未知后端是否已写时默认开启浏览器 ingest。 |

**工程双写**：两进程争用同一 SQLite 文件。**语义双写**：同一业务事件两处各写用量。二者都需在部署前核对。

---

### 1.8 数据类型与迁移风险摘要

- 旧库大量 **TEXT UUID**；新设计中 **string vs int64** 须在 **DB、Ent、proto、biz、TS** **全链路统一**。
- **双进程 / 双写窗口**：迁移期 **`cmd/admin`** 与其它进程同城 SQLite 时需约定 **单写源** 或 **只读副本**。

---

### 1.9 后端检查清单
数据库的 REST 逐项勾选：

- [ ] **`api/**/*.proto`**：RPC、HTTP path、请求/响应已定义，`google.api.http` 齐全。  
- [ ] **`make api`**（及 **`make config`** 如需要）；Go + **`web/src/services`** 已提交。  
- [ ] **`internal/biz`**：模型 + **`Repo`** + **`Usecase`**；**无** **`import api/...`**。  
- [ ] **`internal/data`**：Ent Schema + Repo；**仅** `Ent()` / `Postgres()` 访问对应库；**无**并联 SQLite `sql.Open`。  
- [ ] **`internal/service`**：嵌入 **`Unimplemented*`**；proto ↔ biz 映射完整。  
- [ ] **`internal/server`**：**`Register*HTTPServer`**（gRPC 若启用则 **`Register*ServiceServer`**）；**无非 proto 手写业务路由**。  
- [ ] **`web/src/services/index.ts`**： **`export function createXXXService`**。  
- [ ] **`wire`** 更新，**`go build ./cmd/admin`** 通过。

---

## 第二部分：前端 TypeScript · 分层与门禁

### 2.1 生成客户端与网关

#### 生成物与工厂

- 生成目录形如 **`web/src/services/kratos/<module>/v1/`**。
- **`web/src/services/index.ts`** 对每个 service 导出工厂，形如：

```typescript
import { createAdminServiceClient } from "./kratos/admin/v1/index";
import { requestHandler } from "./axiosHandler";
export function createAdminService() {
  return createAdminServiceClient(requestHandler);
}
```

新增模块步骤：

1. Proto 中加 **`service`/RPC**，**`make api`**。  
2. 确认生成了 **`createXXXServiceClient`**。  
3. 在 **`index.ts`** 增加 **`createFooService()` → `createFooServiceClient(requestHandler)`**。  
4. 运行时根地址统一用 **`getBackendOrigin()`**（**`web/src/config/runtime.ts`**）；不按接口拆分多套 origin（除非仓库已有特例注释）。

#### 运行时请求（`axiosHandler.ts`）

- **`kratosApi`**：`axios.create({ baseURL: getBackendOrigin(), timeout: … })`。  
- **`syncHttpClients()`**：加载 runtime 后刷新 **`baseURL`**。  
- **`requestHandler({ path, method, body })`**：`kratosApi.request({ url: '/' + path, method, data: body, Content-Type … })`，返回 **`res.data`**，供 **`createXxxServiceClient`** 使用。  
- 开发环境常为 **空 **`baseURL` + Quasar/`Vite` 代理** 使请求同源命中 **`/v1/...`**；**`/sse`** 等按 **`quasar.config.ts`** 代理到独立 SSE 端口时，前端用同源 **`/sse/...`**。

**注释约定（源码中）**：**`kratosApi` + **`create*Service()`** → **`/v1/...`**（含 **`memory/v1`**）；**`/v1/chat/*`** 可能走 **`LEGACY_REST_ORIGIN`** 转发；Skill **import*** 等特殊 multipart 可由本进程 **`RegisterSkillImportHTTPServer`** 挂载，不走遗留网关。**ADK** 的 **`run_sse` / WebSocket`** 可走 **`services/adk`**，不经 **`axios`**。

---

### 2.2 唯一合法数据流（禁止逆行）

```text
features/<域>/api.ts（及域内子模块如 features/session/api.ts）— 纯 HTTP 与类型
        ↓ 仅能被 Store actions（或 Store 内私有助手）调用
Pinia Store（状态、loading、error、业务流程）
        ↓ 仅能被 Composable / Page 读取或触发 action
Composable（页面级薄 API，可多 Store 组合）
        ↓
Page（路由、布局、传参、处理 emits）
        ↓ props
Component（展示：props in / emits out）
```

**定义**

- **展示组件**：位于 **`components/**`**、以渲染与局部交互表象为主；**禁止** Pinia；**禁止**业务 API。**容器组件**须经白名单，文件首行注释 **`// Container: approved because ...`**。
- **页面**：**`pages/**`** 下 **`*Page.vue`**；可调 Composable；**禁止**长串散装 **`await fetch`** 与跨页复制业务流程。

---

### 2.3 展示组件 `<script>` 禁止项（逐条自检）

| 禁止 | 说明 |
|------|------|
| **`useXxxStore` / `defineStore` / `storeToRefs`** 驱动业务真源 | 状态与请求在 Store |
| **`features/*/api`**、**`services/`**（**`kratosApi`** / **`createFooService`**）、**`axios`**、`api.get` | 写入与列表加载不得在此 |
| **`watch` + fetch + ref** 存跨组件共享业务数据 | 进 Store |

**允许**：`vue`、无网络的 **`@vueuse/*`**、**`quasar`**、**`defineProps` / `defineEmits`**、仅依赖 props 的 **`computed`**、纯本地 UI（tab、expanded）且不构成业务真源。

---

### 2.4 放置决策树（按顺序）

1. 涉及后端交互或全局共享业务状态 → **不进**纯展示组件；用 **Store + `features/<域>/api`**。  
2. 仅单接口且无跨页状态 → 仍在 **Store action**；Composable 只调 action。  
3. 多页面复用加载/筛选 → **`useXxx` Composable**，内部只组合 Store，不直接 **`axios`**。  
4. 仅外观、数据全部由父级传入 → **展示组件** + props/emits。

---

### 2.5 目录映射（`web/src/`）

| 层级 | 路径 | 职责 |
|------|------|------|
| HTTP 封装 | **`features/<域>/api.ts`**、**`services/index.ts`**、**`axiosHandler.ts`**（**`kratosApi`**）；旧前缀收口 **`features/*/legacyRest.ts`** 或同域 **`api.ts`**（**`getBackendBaseURL()`**）；**禁止**在此追加已迁 Kratos 的新逻辑 | 请求与类型映射，无业务 loading |
| Store | **`stores/<域>/index.ts`**，经 **`stores/index.ts`** **具名导出**；保留 **default export** 的 Pinia 工厂（Quasar 要求） | 状态 + actions |
| Composable | **`composables/useXxx.ts`** 或 **`features/<域>/useXxx.ts`** | Page 可调薄 API |
| 展示组件 | **`components/<域>/**/*.vue`** | props / emits |
| 页面 | **`pages/**/*Page.vue`** | 组合布局与 Composable |
| Feature | **`features/<域>/`** | **`api.ts`**、composable；**不放**上文定义的展示 **`*.vue`**（除白名单容器） |

**路径硬性**

- 展示 **`*.vue` 必须在 **`components/<域>/`**，不得长期放在 **`features/<域>/`** 或 **`pages`** 内搭伪目录代替。  
- **Dialog / Drawer / 全屏表单**：同理；**submit** → **`emit('submit', payload)`**，由 Page 或 Store 调 API。**禁止**浮层 **`script`** 直连 **`features/*/api`**。**浮层样式**须满足本文 **「第四部分 UI·UX」** 的玻璃与强调色条款。  
- 与展示紧耦合且无网络的 **`.ts`** 可放在 **`components/<域>/`**；可对 **`features/<域>/api` 只做 **type-only** import。  
- Monitor 示例：展示在 **`components/monitor/`**；门面在 **`features/monitor/`** 仅 **`api.ts` / types / utils**。

---

### 2.6 各层细则

#### Service / `features/*/api.ts`

- 一函数对准**一个**后端能力（或资源的单一操作）。  
- **不得**：读 **`useRoute`**、改 Pinia、**`$q.notify`**（通知策略在 Store/Composable）。  
- **Kratos**：`**import { createXxxService } from "../../services/index"`**（按相对路径修正），默认经 **`requestHandler`** 访问 **`/v1/...`**。  
- 新路径写 feature api，**.vue** 不写裸 URL。

#### Store

- 按域拆分；**异步、列表重置、错误** 均在 **actions**；暴露 **`loadXxx` / `saveXxx`**。

#### Composable

- **`use`** 前缀；默认只依赖 Store。若迁移期直连 Service，须在文件顶部 **`// TECH-DEBT: direct API call; move to store`**；**新代码禁止照搬**。

#### 展示组件

- 磁盘路径符合 **路径硬性**；完整 **props/emits**；业务真源、权限、列表来源由父级或 Page。

#### Page

- **理想**：`**script setup`** 以 import + 少量 composable + 路由绑定为主；复杂逻辑下沉。

---

### 2.7 迁移旧代码的步骤（遗留 → 合规）

1. 标出当前谁发请求、谁存列表、谁被多组件读。  
2. **抽 **`features/<域>/api.ts`****：Kratos 经 **`create*Service`** 或 **`kratosApi`**；旧前缀 **`axios` + `getBackendBaseURL()`** 收口同域。**勿新建 mega facade。**  
3. **建或扩 Store**：**`loadXxx`** actions，迁入 **`ref` 列表、loading`。  
4. **Composable**：只暴露 **`storeToRefs`** / 调用 **`store.loadXxx`**。  
5. **瘦 Page**：删掉散装请求。  
6. **瘦组件**：删 Store/API，改 props/emits；**emit** 由 Page 接住再调 composable/store。  
7. **回归**；杜绝 Store import **`.vue`**。

---

### 2.8 正反例摘要

| 场景 | 反例 | 正例 |
|------|------|------|
| 列表卡片 | 卡片内 `listMessages()` | Page/composable/store 拉数，卡片只收 **`rows`** |
| 头像 | 展示组件内 **`createAvatarService()`** | Store 或父级算出 **`src`** |
| Dialog 只读 | **onMounted + api.get** | Store **`openDialogLoad`** 或 Page 预先 **`store.load`** |

---

### 2.9 端到端形状示例（路径按项目调整）

**`features/skill/api.ts`**

```typescript
import { createAgentService } from "../../services";

export async function fetchAgent(agentId: string) {
  const svc = createAgentService();
  return svc.GetAgent({ id: agentId });
}
```

**`stores/skill/index.ts`**（节选）

```typescript
import { defineStore } from "pinia";
import { ref } from "vue";
import { fetchAgentSkillStats } from "../../features/skill/api";

export const useSkillStore = defineStore("skill", () => {
  const stats = ref<unknown[]>([]);
  const loading = ref(false);

  async function loadStats(agentId: string) {
    loading.value = true;
    try {
      stats.value = await fetchAgentSkillStats(agentId);
    } finally {
      loading.value = false;
    }
  }

  return { stats, loading, loadStats };
});
```

**`components/skill/SkillStatsTable.vue`**：props **`stats`/`loading`**，无 API。

**`pages/agent/AgentSkillPage.vue`**：**`useRoute` + `useSkillStats` composable**，模板只组合展示组件。

**`stores/index.ts`**：**具名导出** **`useSkillStore`**，**不得**删除 Pinia factory 的 **default export**。

---

### 2.10 页面与布局组织约定

- **`pages`、`layouts`**：**只保留页面级编排**（路由、组合子组件、布局类样式）。凡是可复用或单块过长的 UI，抽到 **`components/<域>/`** 或 **`components/common/`**。  
- 组件命名 **PascalCase**（如 **`AgentListCard.vue`**）。  
- 列表项、表单区、工具栏、空态、加载态、重复外壳 → **优先组件化**。

---

### 2.11 前端交付检查清单（复制用）

**架构**

- [ ] **展示组件无**直连 API / Store（或已 **`// Container`** 白名单）。  
- [ ] 新请求仅出现在 **`features/*/api`** 或 **`services/`**，由 **Store action** 触发。  
- [ ] Page 主要为 composable + 传参；无大段散装业务分支。  
- [ ] **`stores/index.ts`** 具名导出新 store，**默认 Pinia factory 仍在**。  
- [ ] **`boot/pinia`** 与 **`stores/index`** 无双重 Pinia。  
- [ ] **浮层**若有：路径 **`components/<域>/`**；无 **`features/*/api`**；玻璃与 **`--color-accent`** 符合本文 UI 部分。  

**端到端对接**

- [ ] **`createXXXService`** 已导出。  
- [ ] **`features/<域>/api.ts`** 完成；不必要裸 **`/api/v1`**。**JSON 绑定与生成 TS 键名不一致**的接口已在门面显式处理并注释。

---

### 2.12 英文 Agent 速记（系统提示压缩版）

**MUST**：新 HTTP 在 **`features/<domain>/api.ts`**（或子模块）；经 **`services/index`** 的 **`createFooService`** + **`requestHandler`**（或 **`kratosApi`**）访问 **`/v1/...`**；**不得在 `axiosHandler` 外再造巨型胶水**；触发请求仅在 **Pinia actions**；**Composable → Store**；Page 短小；展示组件 **`defineProps`+`defineEmits`**；新 store **`stores/<domain>/`** **具名导出**且不删 Pinia factory。

**MUST NOT**（展示 **`components/**/*.vue`**）：**`useXxxStore`/`defineStore`/`storeToRefs`**（非白名单容器）；**`features/*/api`** / **`services`** 入口 / **`axios`** / **`kratosApi`** / **`create*Service()`** 用于远程读写；对话框内亦然，用 **`emit('submit')`**。**`watch`+fetch+ref`** 承载跨组件业务数据。

**迁移微步骤**：抽到 API → Store action → Composable → 瘦 Page / 组件。

---

### 2.13 迭代优先顺序摘要（后端 → 前端 → UX）

对每个业务域建议使用同一节奏（可拆分 PR）：  
**A. 后端**：proto 全集 + **`make api`** + biz/data/service + **`Register*HTTPServer`** + **wire + `go build ./cmd/admin`**。  
**B. 前端**：**`services/index`** → **`features/<域>/api`** → Store → Composable/Page；**不写裸路径**；**能组件化则组件化**（重复 UI → **`components/<域>/`**）。  
**C. UX**：在完成 B 的子组件与页面上逐项落实本文 **「第四部分」** token 与数值，避免仅在父页糊样式分叉。

一票否决：**proto 未定却长期手写新路径**；**展示组件违禁 import**；业务域 **`HandleFunc`** 补丁路由。

---

## 第三部分：域与路由对照备忘录（运维/重构用）

以下内容用于**选型与对账**，不替代 proto 源码。

### 模块与前端主要落点（概括）

本仓库已实现或部分实现的主要 Kratos 域包括但不限于：`admin`、avatar、agent_category、llm_provider_model、hook、mcp_server、channel、cron、plugin、skill（含本进程挂载的 **`/v1/skills/import*`** multipart 导入）、tool、team、session、memory、usage、monitor、agent。Chat 的 **`POST /v1/chat/messages`**、**`POST …/messages/stream`**、**`GET /v1/chat/options`** 在未配置上游时可由 admin **503**，配置 **`LEGACY_REST_ORIGIN`** 时将这些入口**反向代理**到遗留前缀 **`/api/v1/chat/*`**。会话列表、timeline、会话内消息列表等走原生 **`session/v1`**。

### 遗留旧路由前缀速查（`pkg/backend` 侧曾用）

以下为**历史前缀**示意，网关或客户端可能仍为 **`/api/v1`**；本项目 Kratos 常见 **`/v1`**（前缀由网关与前端 **`baseURL`** 对齐）：

| 分组 | 路径前缀示例 |
|------|----------------|
| Agent | **`/api/v1/agents`**… |
| Team | **`/api/v1/teams`**、**`team-runs`**、**`team-run-events`** |
| 会话 / 聊天 | **`sessions`**；**`chat/messages`**、**messages/stream`、`chat/options`** |
| 平台资源示例 | **`agent-categories`**、**`llm-provider-models`**、**`avatar-assets`**、**`hooks`**、**`mcp-servers`**、**`cron-tasks`** |

每个对外 RPC 的**唯一权威**仍为 **`google.api.http` 方法与完整路径**。

---

### 可复制任务卡片（按域勾选）

```
域：<name>
A 后端：[ ] proto全量 [ ] make api [ ] biz+Ent+service+RegisterHTTPServer [ ] wire+build [ ] 与遗留字段/字节差异已说明
B 前端：[ ] create*Service [ ] features/<域>/api+store actions [ ] 无展示组件违禁 [ ] Composable 技术债标记或收口 [ ] 重复UI已抽组件
C UX ： [ ] 玻璃双前缀+token（本文第四部分）[ ] 按钮/卡片/对话框/导航 [ ] Do/Don't 自检 [ ] 移动端降级
收尾：遗留 env / 用量双写已与本文 1.6～1.7 对齐声明
```

---

## 第四部分：UI · UX 执行规范（奶油昼 · 玻璃夜）

**约束**：下文数值与 token 为实现权威；除非刻意设计稿要求，不要用「相近」色替代；浮层 **`backdrop-filter`** 必须与 **`-webkit-backdrop-filter`** 成对。**昼夜**：布局、间距、圆角与字号阶梯**不变**，只换语义色与材质。**Quasar Dark**：`**Dark.set()`** → **`body.body--dark`**，样式分叉 **`body:not(.body--dark)` / `body.body--dark`**。

### UI-1 强制自检（实现前）

| 检查项 | 要求 |
|--------|------|
| 玻璃材质 | 半透明 + **`backdrop-filter` / `-webkit-backdrop-filter`**，blur 一般 **12–24px**，移动端 **8–12px** |
| 边框 | 半透明；**禁止**纯黑或纯白硬边作玻璃边框 |
| 阴影 | 日间优先不靠重 **`box-shadow`**，用厚度与边框；夜间用微弱光晕 |
| 昼夜结构 | **间距·圆角·字体阶梯不变**，只换语义色与材质参数 |
| 日间锚点 | 金盏花 **`#E9A23B`**（悬 **`#D48C1A`**）贯穿主按钮、链接、**`:focus-visible`、表单聚焦边**；玻璃悬停：**略提不透明度与 blur（如 +2px）、边框提亮**；可加极细暖内高光 **`inset 0 1px 0 rgba(255,255,255,0.45)`**；**禁用**日间以青紫霓虹为默认强调、忌冷色强光晕；按压可 **`scale(0.98)`**；次级悬停奶油底 **`#FEF3E4`** |
| 夜间霓虹 | **`#00E5FF`**、**`#A855F7`** 仅占交互焦点与小面积强调渐变，**禁用**铺满；**日间不得**将它们作默认强调 |

#### UI-1a 日间交互速查

| 场景 | 要做 | 避免 |
|------|------|------|
| 主操作 | **`#E9A23B`** 填充，字 **`#FFFFFF`**；悬 **`#D48C1A`** | 大范围渐变抢眼 |
| 链接 / 次强调 | 字色/下划线金盏花系 | 夜间霓虹默认 |
| **`:focus-visible`** | 2px 环 `rgba(233,162,59,0.45)`（金盏色系） | 只靠默认蓝 |
| 可点玻璃卡片 | 悬：**`rgba(255,253,245,0.78)`、`blur(20px)`**，边向 **`rgba(235,220,200,0.85)`** | 单靠阴影深浅 |
| 图标按钮 | 默认 `#B8A590`，悬停/激活 `#E9A23B` 或 `#3A322C` | 彩虹描边 |

---

### UI-2 CSS 变量（`:root` / `body.body--dark`）

**实现路径**：`web/src/css/theme/_css-vars-light.sass`（`:root`）、`_css-vars-dark.sass`（`body.body--dark`）；聚合入口 `web/src/css/app-theme.sass`。页面与组件取值用 **`var(--*)`**，一般不硬编码 hex。

#### 日间（`:root`）

| Token | 值 | 用途 |
|-------|-----|------|
| **`--canvas-base`** | **`#FEFBF4`** | 主画布 |
| **`--glass-surface`** | **`rgba(255,253,245,0.65)`** | 标准玻璃 |
| **`--glass-surface-hover`** | **`rgba(255,253,245,0.78)`** | 玻璃悬停 |
| **`--glass-blur-default`** | **`18px`** | 与 surface 配对 |
| **`--glass-blur-hover`** | **`20px`** | 悬停略增 |
| **`--glass-border`** | **`rgba(235,220,200,0.7)`** | 边框 |
| **`--glass-elevated`** | **`rgba(255,255,255,0.72)`** | 弹层 |
| **`--glass-blur-elevated`** | **`24px`** | 抬高 blur |
| **`--color-accent`** | **`#E9A23B`** | 主操作 |
| **`--color-accent-hover`** | **`#D48C1A`** | 主操作悬 |
| **`--focus-ring-light`** | **`2px solid rgba(233,162,59,0.45)`** | 键盘焦点 |
| **`--interaction-surface-hover`** | **`#FEF3E4`** | 次级悬停衬底 |
| **`--glass-inner-highlight`** | **`inset 0 1px 0 rgba(255,255,255,0.45)`** | 顶缘高光（可选） |
| **`--color-text-primary`** | **`#3A322C`** | 正文主色 |
| **`--color-text-secondary`** | **`#8B7A6B`** | 辅文案 |
| **`--color-icon-muted`** | **`#B8A590`** | 图标/线 |
| **`--color-success`** | **`#4CAF7C`** | 成功 |
| **`--color-warning`** | **`#F09B54`** | 警告 |
| **`--color-danger`** | **`#E55C5C`** | 危险 |
| **`--nav-bg-light`** | **`rgba(255,249,236,0.85)`** | 顶栏 |

#### 夜间（`body.body--dark`）

| Token | 值 | 用途 |
|-------|-----|------|
| **`--canvas-base`** | **`#090D14`** | 画布 |
| **`--glass-surface`** | **`rgba(18,24,34,0.65)`** | 玻璃 |
| **`--glass-surface-hover`** | **`rgba(22,28,40,0.75)`** | 悬停 |
| **`--glass-border`** | **`rgba(255,255,255,0.08)`** | 边框 |
| **`--glass-border-hover`** | **`rgba(255,255,255,0.16)`** | 悬停边 |
| **`--color-accent`**（语义） | **`#00E5FF`** | 霓虹主强调 |
| **`--color-accent-hover`** | **`#5aebff`** | — |
| **`--color-neon-cyan`** | **`#00E5FF`** | 焦点/链接 |
| **`--color-neon-violet`** | **`#A855F7`** | 二级渐变 |
| **`--gradient-flow-border`** | **`linear-gradient(120deg,#00E5FF,#A855F7,#00E5FF)`** | 流动边 |
| **`--color-text-primary`** | **`#EBEBF0`** | — |
| **`--color-text-secondary`** | **`#9CA0B0`** | — |
| **`--color-success`** | **`#3FE0A0`** | — |
| **`--color-warning`** | **`#FFAF4D`** | — |
| **`--color-danger`** | **`#FF5E7A`** | — |
| **`--nav-bg-dark`** | **`rgba(9,13,20,0.7)`** | — |
| **`--nav-divider-dark`** | **`rgba(255,255,255,0.06)`** | — |

#### 最小玻璃片段（复制）

```css
background: var(--glass-surface);
backdrop-filter: blur(var(--glass-blur-default));
-webkit-backdrop-filter: blur(var(--glass-blur-default));
```

---

### UI-3 样式工程（放置规则）

| 层级 | 路径 | 职责 |
|------|------|------|
| 构建常量 | **`web/src/css/quasar-variables.sass`** | **`$primary`** 等（Vite **`sassVariables`**）；不随 Dark 重算 |
| Token | **`app-theme.sass` → `theme/*`** | — |
| 全局类 | **`app-global.sass`** | 字体、shell、页面 class；分叉用 **`body:not(.body--dark)`** / **`body.body--dark`** |
| 入口链 | **`style.sass` → `css/style.sass`** | 构建 **`css: ['style.sass']`**；业务改 **`css/`** |

**规则**

1. 新 token → **`_css-vars-*.sass`** 或新 partial并由 **`app-theme`** 聚合。  
2. 新页面/布局 class → **`app-global`**。  
3. 主强调、链接、焦点以 **`--color-accent`**；**`$primary`** 仅兼容默认 Quasar。**禁止**运行时改 **`quasar-variables`**。  
4. Token 增殖：仅在 **`theme/`** 扩充；**勿**并行第二全局 CSS 入口。

---

### UI-4 排版

展示字体：**`SF Pro Display, Inter Tight, Helvetica Neue, sans-serif`**。正文：**`SF Pro Text, Inter, Helvetica Neue, sans-serif`**。字色用 token。标题：**负字距、偏紧行高**。夜间可选用微弱青辉：**`text-shadow: 0 0 12px rgba(0,229,255,0.15)`**。字号阶梯不因昼夜重写。

---

### UI-5 组件数值

#### 按钮

| 模式 | 要点 |
|------|------|
| 昼·主 | 背 **`#E9A23B`**，字白，悬 **`#D48C1A`**；圆角 **10px**；内边 **10px 20px** |
| 昼·次 | 透明，字 **`#3A322C`**，边 **`1px solid #D0C0A8`**，悬 **`#FEF3E4`** |
| 昼·玻璃次 | **`rgba(255,253,245,0.5)`+blur** |
| 夜·主 | **`rgba(0,229,255,0.15)`**，霓虹边，字 cyan；可加 **`box-shadow: 0 0 16px rgba(0,229,255,0.3)`** |
| 夜·次 | 玻璃边框 **`rgba(255,255,255,0.1)`** |

#### 卡片

昼玻璃：**`rgba(255,253,245,0.65)`+blur18+边**，**无重阴影**。昼实体少用：**`#FFFDF5`、细边、极小影 `0 2px 12px rgba(0,0,0,0.04)`、圆角 16–20px**；同级勿与玻璃混搭。夜间：**`rgba(18,24,34,0.65)`+blur+webkit**；选中可加弱 **`box-shadow`** 青辉。

#### 对话框（内容卡）

**`background: var(--glass-elevated)`**；**`backdrop-filter` + `-webkit-backdrop-filter`** 用 **`blur(var(--glass-blur-elevated))`**；边 **`var(--glass-border)`**；圆角 **20–24px**；主 CTA 用 **`var(--color-accent)`**；**日间不得用霓虹作主色默认值**。

#### 输入

昼实体：**`#fff`** 底；边 **`#D0C0A8`**；聚焦边 **`#E9A23B`**。昼玻璃半透明 + blur。**夜**：深透 + 白边渐变；聚焦青 + 微弱光。**圆角 12–16px**。

#### 导航 / 工具条

昼：奶色半透明 + blur；分割线 **`rgba(235,220,200,0.6)`**。夜：**`rgba(9,13,20,0.7)` blur 20**，底分割 **`rgba(255,255,255,0.06)`**，悬停霓虹。

#### 媒体

昼图贴奶油底；夜轻微压暗或玻璃蒙层，产品图极弱青辉勿扰主体。

---

### UI-6 夜间特效（可选）

流动边：**`border-image` 或渐变 `00E5FF↔A855F7`**。扫描 Hero 慢动画需**不遮读**。霓虹 **`drop-shadow`** 仅供小面积。**移动**：光流动改静态渐变；blur 见 UI-1。

---

### UI-7 布局

间距刻度：**`4,8,12,16,20,24,32,48,64` px**。**圆角**：控件 **5–8**；卡片/面板 **16–20**；大模块 **28–36**；胶囊 **56–980**；圆 **50%**。**层级**：不靠重阴影，靠不透明、blur、边亮与昼夜焦点策略。

---

### UI-8 Do / Don’t

**Do**：全昼夜磨砂玻璃；昼奶油 rgba255,253,245系；夜深透+弱光；强调仅锚点。**Don’t**：昼大白硬块铺满；层级靠堆砌阴影；同层混搭实体与玻璃；玻璃上大纯色块挡内容；移动端忽略 blur 降级。

---

### UI-9 响应式

断点遵从项目全局。移动端 blur **8–12px**，动效/UI-6 降级。

---

### UI-10 AI 极短复述

- **昼**：底 **`#FEFBF4`**；卡 **`rgba(255,253,245,0.65)` blur 18**，边 **`rgba(235,220,200,0.7)`**，主钮 **`#E9A23B`**，少用重投影。  
- **夜**：底 **`#090D14`**；板 **`rgba(18,24,34,0.65)` blur**，边 **`rgba(255,255,255,0.08)`**，强调 cyan。  
- **对话框**：背景 `var(--glass-elevated)`；`backdrop-filter` 与 `-webkit-backdrop-filter` 同时使用 `blur(var(--glass-blur-elevated))`；边 `var(--glass-border)`；主 CTA 用 `var(--color-accent)`。

---

## 第五部分：全链路合并检查清单（后端 + 前端 + UX）

- [ ] **`api/**/*.proto`** 覆盖本迭代全部 **`/v1`** 能力，`make api`，Go + TS 已提交。  
- [ ] **`internal/biz` / data / service / server`** 合规；Wire + **`go build ./cmd/admin`**。  
- [ ] **`web/src/services/index.ts`** **`createXXXService`**。  
- [ ] **`features/<域>/api` + Pinia**，展示组件门禁，浮层 **`emit`** 链闭环。  
- [ ] UX：**玻璃双前缀**、**变量 token**、**组件数值/UI-8**；深浅色自检。  
- [ ] **`LEGACY_*` / 用量 ingest`**：按需阅读并声明本文 **1.6～1.7**。

若需求违反本文硬性分层（例如必须在叶子组件打点请求），须在 PR 写明 **例外、边界、偿还计划**，否则评审可拒。

# 开发规范

## Agent / 运行时（以 `pkg/adk-go` 为框架真相源）

- **`pkg/adk-go` 是本项目的 Agent 框架**：凡涉及「Agent 执行、Runner 编排、Session 状态、LLM 请求/响应（`model`）、工具（`tool`）、流式与事件语义」的设计与实现，**必须先对照 `pkg/adk-go` 中的包结构、接口与约定**，再落代码。
- **`pkg/backend` 不作为新能力的结构范本**：`pkg/backend`（含 `internal/conversation`、`adapters/adkruntime`、旧版 HTTP 对话与团队编排等）仅保留历史或并行实现；**新增或重构功能时，不得以 `pkg/backend` 的分层、调用链或事件命名为权威参考**。需要兼容旧行为时，单独说明迁移路径即可。
- **集成原则**：在 `internal/agent`、`internal/team`、`internal/service` 等与对话/团队相关的代码中，通过**薄适配层**调用 `pkg/adk-go` 公开 API，避免在业务包内重复实现 ADK 已提供的编排或会话语义。
- **与 API/服务层的边界**：对外 Kratos、gRPC/HTTP、AIP 资源形态仍遵循本仓库既有约定；**对话与多 Agent 的语义模型**以 `pkg/adk-go` 为准，避免再引入一套与 ADK 冲突的「第二套运行时」。

### `internal/provider`：厂商与模型集成单点

- **职责范围**：**`internal/provider`**（含 **`provider/openai`**、**`provider/deepseek`**、**`provider/gemini`** 等子包）承载 **厂商连接与模型的初始化、解析与调用**——例如：目录/Biz 侧 **`provider_type` / `api_base_url` / `api_key` / 模型名**的合并、**`Registry.Resolve`** 绑定具体后端、**HTTP 传输（`RoundTrip`）**、以及实现 **`google.golang.org/adk/model.LLM`** 的 **`GenerateContent`**（含流式）等。
- **契约对齐**：对模型的入参/出参形态以 **`pkg/adk-go/model`** 为准（如 **`LLMRequest` / `LLMResponse`**、`genai.Content` 与配置）；不要在业务包中平行维护另一套「驱动接口」或重复的厂商 HTTP 客户端分散在各处。
- **业务集成约定**：凡与 **调用大模型** 相关的业务能力（选厂商、走补全/流式、聚合用量与文本解析等），**优先在 `internal/provider` 及其子包内收口实现**；**`internal/agent`、`internal/team`、`internal/service`** 等仅保留编排、proto/会话消息与 **`LLMRequest`** 之间的**必要适配**，避免在多处复制厂商协议细节或私自新建直连调用路径。
- **新增厂商或协议变体**：通过扩展 **`Registry`** 注册工厂、在子包中实现 **`model.LLM`**，并保持与现有 **`CatalogClient`**、**`MergeCatalogIntoRequest`** 等辅助方法一致；评审时检查是否仍有绕过 **`provider`** 的零散模型调用。
- **`provider_type`：`gemini`**：走 **`google.golang.org/genai`** + **`pkg/adk-go/model/gemini`**（官方 Gemini API，非 OpenAI 兼容层）。目录 **`ConfigJSON`** 中 **`api_key`** 必填或依赖环境变量 **`GOOGLE_API_KEY` / `GEMINI_API_KEY`**；**`api_base_url`** 可选，用于自定义/代理端点；**`model`** 行字段为 Gemini 模型 id（如 **`gemini-2.5-flash`**）；**`google` / `vertex`** 仍默认绑定 OpenAI 兼容工厂（网关场景），不要用它们表示原生 Gemini。

