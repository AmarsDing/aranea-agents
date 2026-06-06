# MCP 服务器管理

> **2026-05-21 现状对齐**：
> - ✅ 控制台 CRUD + 测试连接 + 状态灯（`metadata_json.health_*`）。
> - ✅ API：`/v1/mcp-servers`（`MCPServerService`）；存储为 `mcp_server` + `config_json` / `metadata_json` 拆分（非扁平列）。
> - ✅ 运行时装配：`AgentMCPTooling` → `tool_assembly.resolveMCPServers` → `tools.buildMCPToolSet` / `buildMCPBrokerTools`。
> - ✅ 后台探活：`internal/mcp/health`；OAuth2 静态/Client Credentials/Refresh（`config_json.auth`）。
> - 🟡 `mcp_call_count` 会话统计已有字段与分类逻辑，与全链路验收待补。
> - ❌ 按用户动态凭据（`require_user_credentials`）独立配置页未实现。
>
> 进度以 [19-mcp-development.md](./19-mcp-development.md) 与 [execution-plan.md](../guides/execution-plan.md) 为准。

本文档描述 **Model Context Protocol（MCP）服务器** 在控制台中的 **列表、CRUD、状态灯、添加/编辑表单** 的 UI 设计，以及 **持久化模型** 与 **HTTP API**。前端采用 **Quasar（Vue 3）**，与 Monitor 控制台风格一致。

---

## 1. 信息架构与路由

| 页面 | 路由（示例） | 说明 |
|------|--------------|------|
| MCP 服务器列表 |  `/mcp-servers` | 搜索、**`QList`** 列表（每项一个 MCP 组件）、空态、刷新、添加 |
| 新建 | 列表内 **「+ 添加服务器」** → **`QDialog`** 或独立路由 `/mcp-servers/new` | 表单同编辑 |
| 编辑 | **编辑** → 同上对话框预填 或 `/mcp-servers/:id/edit` | 与创建共用组件 `McpServerForm` |

---

## 2. 列表页 UI

### 2.1 顶栏与工具条

| 区域 | 内容 |
|------|------|
| **标题** | 「MCP 服务器」 |
| **副标题** | 「管理 Model Context Protocol 服务器连接」 |
| **右上** | **「+ 添加服务器」**（`QBtn` color=primary）；**「刷新」**（`QBtn` outline + `refresh` 图标） |
| **搜索** | `QInput` `debounce` + `clearable`，占位「搜索服务器…」；按 `name`、`display_name` 前端过滤或 `GET ...?q=` |

### 2.2 有数据时：`QList` + 单项 MCP 组件（推荐）

不使用表格；外层 **`QList`** `padding`/`separator`，**每个 `QItem` 内嵌一个独立 MCP 展示组件**（如 `McpServerItem.vue` / `McpServerCard`），便于样式复用与响应式换行。

| 层级 | Quasar / 结构 | 内容 |
|------|----------------|------|
| 列表容器 | **`QList`** + 可选 **`QScrollArea`**（列表很长时） | `v-for="server in filteredServers"` → 一条 `QItem` 对应一条 `mcp_server` 记录 |
| 单项 | **`QItem`** `clickable`（可选：点击进入编辑） | `key=server.id` |
| 单项内部 | **`QItemSection`** `side` **top** | **状态灯**（§2.4），与列表左缘对齐 |
|  | **`QItemSection`** | 主体：包一层自定义 **MCP 组件**（见下） |
| MCP 组件（每个 list 一项一个） | 自定义组件根节点可用 **`QCard`** `flat` `bordered` 或纯 `div.row` | 内含该服务器的全部摘要与操作 |

**MCP 组件（每个 `QItem` 内一份）建议布局**：

| 区块 | 内容 |
|------|------|
| **标题行** | `display_name` 或 `name`（主标题）；`name` 可作 `text-caption` 副标 |
| **元信息行** | **传输**：`stdio` / `SSE` / `Streamable HTTP`；**地址/命令**：`url` 或 `command` 单行截断 + `QTooltip` |
| **次要行** | **工具前缀** `tool_prefix`；**超时** `timeout_sec` + `s`；**启用** `QChip` 或只读 `QToggle` |
| **操作行** | **编辑**、**删除**、可选 **测试连接**（`QBtn` `flat` `dense`），见 §2.5 |

列表项之间可用 **`QSeparator`** `inset` 或 MCP 卡片自带 `margin` 区分层次。

### 2.3 空态

与线稿一致：中央 **插头图标**（`QIcon` 或插画）、主文案「暂无 MCP 服务器」、副文案「添加您的第一个 MCP 服务器以开始使用。」、可选主按钮复用「添加服务器」。

### 2.4 状态灯（连接/健康）

| 灯色 | 含义 | 数据来源 |
|------|------|----------|
| **灰** | 未检测 / 从未成功连接 | `last_health_at` 为空且 `enabled=false` 或未跑过检测 |
| **绿** | 最近一次 **测试连接** 或 **后台探活** 成功 | `health_status=ok` |
| **红** | 最近一次失败或初始化错误 | `health_status=error` 或存在 `last_error_message` |
| **黄**（可选） | 已启用但超过 N 分钟未探活成功 | `health_status=degraded` 或超时策略 |

**实现**：在 **MCP 组件** 左侧或 `QItemSection` side 内放 **`QBadge`** `rounded` 小圆点，或 `span` 8px 圆 + `bg-positive`/`bg-negative`/`bg-grey`；**`QTooltip`** 展示最近错误摘要或「最近成功：时间」。

探活可由用户点击该项 **「测试连接」** 或定时任务写入 `last_health_at`、`health_status`。

### 2.5 行内操作（CRUD）

| 操作 | 行为 |
|------|------|
| **编辑** | 打开 **`QDialog`** 表单，`GET /mcp-servers/:id` 预填 |
| **删除** | **`QDialog`** 确认：「删除后依赖该服务器的工具将不可用」；`DELETE /mcp-servers/:id` |
| **测试连接**（可选，在 MCP 组件操作行） | `POST /mcp-servers/:id/test`，结果 **`Notify`** 或该项内短暂提示 |


---

## 3. 添加 / 编辑表单（对话框）

标题：**「添加 MCP 服务器」** / **「编辑 MCP 服务器」**；内容区 **`QScrollArea`**；底栏按钮：**测试连接**（左）、**取消**、**创建/保存**（主色）。

### 3.1 字段与控件

| 字段（逻辑名） | 控件 | 必填 | 校验 / 说明 |
|----------------|------|------|-------------|
| `name`（表单）→ API `key` | `QInput` | 是 | **Slug**：仅小写字母、数字、连字符；平台内唯一；占位 `my-mcp-server` |
| `display_name`（表单）→ API `name` | `QInput` | 否 | 展示用，如 `SQL Server` |
| `transport` | **`QBtnToggle`** 或 `QOptionGroup` `inline` | 是 | **`stdio`** \| **`sse`** \| **`streamable_http`**（界面对应 Streamable HTTP） |
| `url` | `QInput` | `sse`/`streamable_http` 必填 | HTTP(S) URL；**服务端 SSRF 校验**（截图错误示例：`resolve "mysql": no such host`） |
| `command` / `args` | `QInput` + 多行或数组编辑 | `stdio` 时必填 | 可执行路径与参数；无 URL |
| `headers` | 动态键值行 | 否 | 每行 key + value（敏感 value 用 `password` 或掩码）；**+ 添加请求头**；行内删除 |
| `env` | 动态键值行 | 否 | 占位「变量名称」「值」；**+ 添加变量** |
| `tool_prefix` | `QInput`，前缀可视化 `mcp_` + 输入 | 否 | 空则服务端从 `name` 派生；说明文案：`Tools: mcp_{prefix}__{tool}` |
| `timeout_sec` | `QInput` `type=number` 或 `QSlider` | 否 | 默认 `60` |
| `enabled` | `QToggle` | 否 | 默认开 |
| `require_user_credentials` | `QToggle` | 否 | 副文案：每个用户须配置自己的凭据，否则无法使用 |

**动态键值**：`v-for` 行 + `QInput`×2 + `QBtn` `icon="delete"`；或用小型 **`QTable`** `hide-pagination` 内嵌编辑。

### 3.2 传输与字段显隐

| `transport` | 展示 URL | 展示 command/args |
|-------------|----------|-------------------|
| `streamable_http` / `sse` | 是 | 否 |
| `stdio` | 否 | 是 |

### 3.3 校验与错误展示

- 表单底部或字段下方：**红色** `div.text-negative` 展示服务端返回（如 URL 非法、SSRF、传输初始化失败）。
- **测试连接**：`POST` 不持久化或先保存草稿再测（产品二选一）；失败时保留错误在对话框内。

### 3.4 Quasar 映射摘要

**列表**：`QList` → `QItem` → **`McpServerItem`（MCP 组件）**；**表单对话框**：`QDialog` + `QCard` + `QCardSection` + `QScrollArea` + `QForm`（`@submit.prevent`）；`QBtnToggle` `spread`/`no-caps`；`Notify` 成功/失败。

---

## 4. 持久化模型（实现对齐）

### 4.1 表：`mcp_server`

| 列 | 说明 |
|----|------|
| `id` | 主键（随机 hex） |
| `server_key` | Slug，对应表单 `name`、API `key`；Agent 策略前缀 `mcp:<server_key>` |
| `name` | 展示名，对应表单 `display_name` |
| `description` | 描述 |
| `status` | `active` / `error` / `deleted`（探活失败时可为 `error`） |
| `enabled` | 是否启用 |
| `sort_order` | 排序 |
| `config_json` | 连接配置（见下） |
| `metadata_json` | 健康与重连元数据（见下） |
| `created_at` / `updated_at` / `deleted_at` | 软删除时间戳 |

### 4.2 `config_json` 字段（逻辑）

| 字段 | 说明 |
|------|------|
| `transport` | `stdio` \| `sse` \| `streamable_http` |
| `url` / `command` / `args` | 按传输类型二选一 |
| `headers` / `env` | 键值对 |
| `auth` | `api_key` / `oauth2_*`（运行时注入 Authorization） |
| `tool_prefix` | MCP 工具名前缀 |
| `timeout_sec` | 默认 60s，传入 trpc `ConnectionConfig.Timeout` |
| `session_reconnect_max` | SSE/Streamable 重连次数（0=关闭） |
| `allow_adhoc_http` | Broker AdHoc，需叠加系统设置 `mcp_allow_adhoc_http` |
| `require_user_credentials` | 产品标记；按用户凭据页待实现 |

### 4.3 `metadata_json` 字段（逻辑）

| 字段 | 说明 |
|------|------|
| `health_status` | `ok` / `error` / `unknown` |
| `last_health_at` | RFC3339 |
| `last_error_message` | 列表状态灯 Tooltip |
| `last_reconnect_at` / `reconnect_count` | `mcpobserve` 重连可观测 |

**说明**：敏感 `headers` / `auth` 不落日志明文；列表 API 可对值脱敏。


---

## 5. API（已实现）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/mcp-servers` | 列表（前端本地 `q` 过滤） |
| GET | `/v1/mcp-servers/{id}` | 详情 |
| POST | `/v1/mcp-servers` | 创建（`key` + `name` 必填） |
| PATCH | `/v1/mcp-servers/{id}` | 更新（body `mcp_server`） |
| DELETE | `/v1/mcp-servers/{id}` | 软删除 |
| POST | `/v1/mcp-servers/{id}/test` | 探活并写入 `metadata_json` |

可选预检 `POST /v1/mcp-servers/validate` **未实现**；创建前依赖「测试连接」或保存后 Test。

---

## 6. 安全与运维

- **SSRF**：对 `url` 做解析与白名单/内网限制；错误信息可返回「URL 校验失败」而不泄露内网拓扑。
- **敏感头**：`Authorization`、`token` 等不落日志明文；列表接口可对 `headers` 脱敏为 `***`。
- **stdio**：校验 `command` 路径与参数，防止注入。

---

## 7. 验收要点

- [ ] 列表：**`QList`** 每项挂载 **McpServerItem/MCP 组件**；搜索、状态灯、空态、刷新、添加入口。  
- [ ] CRUD：创建、编辑、删除确认；`name` slug 校验。  
- [ ] 传输切换：`url` vs `command`/`args` 显隐正确。  
- [ ] 测试连接与错误文案（含 SSRF/网络失败）可感知。  
- [ ] 数据库字段与表单一致；`require_user_credentials` 为真时有用户凭据入口（可另页）。  

---

## 8. 参考截图（本地）

线稿与空态、添加对话框可参考工作区 `assets/` 下对应 `image-*.png`。

---

*文档版本：1.4 — 2026-05-21 与 PlatformResource + config/metadata JSON 存储对齐；运行时装配见 `19 mcp.design.md`。*
