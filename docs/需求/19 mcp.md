# MCP 服务器管理

本文档描述 **Model Context Protocol（MCP）服务器** 在控制台中的 **列表、CRUD、状态灯、添加/编辑表单** 的 UI 设计，以及 **数据库表字段** 与 **API** 建议。前端实现建议采用 **Quasar（Vue 3）**，与 **`18 monitor.md`** 控制台风格一致。

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
| `name` | `QInput` | 是 | **Slug**：仅小写字母、数字、连字符；租户内唯一；占位示例 `my-mcp-server` |
| `display_name` | `QInput` | 否 | 展示用，如 `sqlserver` |
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

## 4. 数据库设计

### 4.1 表：`mcp_server`（主表）

| 字段名 | 类型 | 约束 | 说明 |
|--------|------|------|------|
| `id` | BIGSERIAL / UUID | PK | |
| `tenant_id` | BIGINT | NULL, INDEX | 多租户时 NOT NULL |
| `name` | VARCHAR(64) | NOT NULL, UNIQUE(tenant_id, name) | Slug，小写+数字+连字符 |
| `display_name` | VARCHAR(128) | NULL | |
| `transport` | VARCHAR(32) | NOT NULL | `stdio` `sse` `streamable_http` |
| `url` | VARCHAR(2048) | NULL | `sse` / `streamable_http` 使用 |
| `command` | VARCHAR(512) | NULL | `stdio`：可执行文件路径 |
| `args` | JSONB | NULL | `stdio`：参数数组 |
| `headers` | JSONB | NOT NULL, DEFAULT `{}` | 请求头键值；**值敏感时加密或引用凭据表** |
| `env` | JSONB | NOT NULL, DEFAULT `{}` | 子进程/客户端环境变量 |
| `tool_prefix` | VARCHAR(128) | NULL | 空则服务端按 `name` 派生 |
| `timeout_sec` | INT | NOT NULL, DEFAULT 60 | |
| `enabled` | BOOLEAN | NOT NULL, DEFAULT TRUE | |
| `require_user_credentials` | BOOLEAN | NOT NULL, DEFAULT FALSE | |
| `health_status` | VARCHAR(16) | NULL | `ok` `error` `unknown` `degraded` |
| `last_health_at` | TIMESTAMPTZ | NULL | 最近一次探活/测试时间 |
| `last_error_message` | VARCHAR(2048) | NULL | 最近一次失败摘要（列表状态灯 Tooltip） |
| `created_at` / `updated_at` | TIMESTAMPTZ | NOT NULL | |
| `created_by` / `updated_by` | BIGINT | NULL | |

**说明**：若 `headers` 含密钥，生产环境建议 **应用层加密** 或拆 **`mcp_server_secret`** 表（见 `17 channel.md` 凭据思路）。


---

## 5. API 建议

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/mcp-servers` | 列表 + 查询参数 `q` |
| GET | `/mcp-servers/:id` | 详情（编辑预填） |
| POST | `/mcp-servers` | 创建 |
| PATCH | `/mcp-servers/:id` | 更新 |
| DELETE | `/mcp-servers/:id` | 删除 |
| POST | `/mcp-servers/:id/test` | 测试连接（不必须落库 health） |
| POST | `/mcp-servers/validate` | 可选：仅校验 URL/SSRF，创建前预检 |

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

*文档版本：1.3 — 2026-05-18 文档治理：§9 运行时实现与 §10 trpc 对齐需求迁移至 `19-mcp-development.md`，需求文档仅保留功能规格与运维指南。*
