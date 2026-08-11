# MCP 集成模块 — 实现设计文档

> 对应需求：[19-mcp.md](./19-mcp.md)
> 开发计划：[19-mcp.development.md](./19-mcp.development.md)
> **2026-06-17 校准**：补 UX 规范、持久化模型、API 端点表（从需求文档迁入）；修正 user-credential 表名/字段、Delete 端点方法、PersistHealth 描述、Broker 挂载条件。

---

## 一、模块概述

MCP（Model Context Protocol）服务器管理：平台注册、健康探活、Agent 级挂载、Broker 运行时发现。领域逻辑在 `internal/biz`；协议客户端在 `internal/tools`（trpc-agent-go `tool/mcp` + `tool/mcpbroker`）。

### 分层职责（SRP）

| 包 | 职责 |
|----|------|
| `internal/mcp/config` | `config_json` 解析、Transport 类型化、`ConnectionConfig` 映射 |
| `internal/mcp/probe` | 管理面连通性评估（stdio PATH / HTTP + SSRF）；`ProbeStrategy` 接口 + `ConnectivityProbe` / `AuthAwareProbe` |
| `internal/mcp/metadata` | `metadata_json` 健康与重连字段合并 |
| `internal/mcp/health` | 后台定时探活 → `MCPServerUsecase.TestMCPServer`（内部 `persistHealth`）；bounded concurrency（semaphore） |
| `internal/mcp/alert` | 持续健康错误告警：`Publisher.MaybeEmitAfterHealth` → EventBus 信封 |
| `internal/mcp/lifecycle` | Server 健康 Lifecycle FSM（`Transition` / `EventFromProbeStatus`）；`metadata.ApplyHealth` 经此收敛状态 |
| `internal/mcp/classify` | MCP 工具调用分类：`IsMCPToolInvocation` + `mcp_call_count` |
| `internal/mcp/defaults` | 全局默认常量集中（超时、间隔、重连次数等） |
| `internal/biz` | CRUD、Effective MCP 策略、`AgentMCPTooling`、用户凭据 |
| `internal/agent/tool_assembly.go` | Agent 回合：解析 config + OAuth 头 → `ToolsetConfig` |
| `internal/tools/toolset.go` | `buildMCPToolSet` / `buildMCPBrokerTools` |
| `internal/tools/mcpobserve` | 重连 EventBus + Prometheus + metadata 回写 |

---

## 二、Proto 层

文件：`api/kratos/mcp_server/v1/mcp_server.proto`

```protobuf
service MCPServerService {
  rpc ListMCPServers(google.protobuf.Empty) returns (ListMCPServersResponse) {
    option (google.api.http) = { get: "/v1/mcp-servers" };
  }
  rpc CreateMCPServer(CreateMCPServerRequest) returns (MCPServer) {
    option (google.api.http) = { post: "/v1/mcp-servers" body: "*" };
  }
  rpc GetMCPServer(GetMCPServerRequest) returns (MCPServer) {
    option (google.api.http) = { get: "/v1/mcp-servers/{id}" };
  }
  rpc UpdateMCPServer(UpdateMCPServerRequest) returns (MCPServer) {
    option (google.api.http) = { patch: "/v1/mcp-servers/{id}" body: "mcp_server" };
  }
  rpc DeleteMCPServer(DeleteMCPServerRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/mcp-servers/{id}" };
  }
  rpc TestMCPServer(TestMCPServerRequest) returns (MCPServerTestResponse) {
    option (google.api.http) = { post: "/v1/mcp-servers/{id}/test" body: "*" };
  }
  rpc ValidateMCPServer(ValidateMCPServerRequest) returns (ValidateMCPServerResponse) {
    option (google.api.http) = { post: "/v1/mcp-servers/validate" body: "*" };
  }
  rpc ListMCPServerUserCredentials(ListMCPServerUserCredentialsRequest) returns (ListMCPServerUserCredentialsResponse) {
    option (google.api.http) = { get: "/v1/mcp-servers/{mcp_server_id}/user-credentials" };
  }
  rpc UpsertMCPServerUserCredential(UpsertMCPServerUserCredentialRequest) returns (MCPServerUserCredential) {
    option (google.api.http) = { post: "/v1/mcp-servers/{mcp_server_id}/user-credentials" body: "*" };
  }
  rpc DeleteMCPServerUserCredential(DeleteMCPServerUserCredentialRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { post: "/v1/mcp-servers/{mcp_server_id}/user-credentials/delete" body: "*" };
  }
}
```

**MCPServer**：`id` / `key`（slug）/ `name`（展示名）/ `description` / `status` / `enabled` / `sort_order` / `config_json` / `metadata_json` / 时间戳。

**MCPServerTestResponse**：`ok` / `status` / `message` / `details_json`。

**ValidateMCPServerResponse**：`ok` / `status` / `message` / `details_json`（不持久化）。

**MCPServerUserCredential**：`id` / `mcp_server_id` / `user_id` / `credential_key` / `status` / `configured` / `masked_preview` / `created_at` / `updated_at`。

**DeleteMCPServerUserCredentialRequest**：`mcp_server_id` / `user_id` / `credential_key`（注意：使用 POST `/delete` 而非 RESTful DELETE，因需在 body 传 `user_id` + `credential_key`）。

---

## 三、API 端点表

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/mcp-servers` | 列表（前端本地 `q` 过滤） |
| GET | `/v1/mcp-servers/{id}` | 详情 |
| POST | `/v1/mcp-servers` | 创建（`key` + `name` 必填） |
| PATCH | `/v1/mcp-servers/{id}` | 更新（body `mcp_server`） |
| DELETE | `/v1/mcp-servers/{id}` | 软删除 |
| POST | `/v1/mcp-servers/{id}/test` | 探活并写入 `metadata_json` |
| POST | `/v1/mcp-servers/validate` | URL 预检（不持久化，仅校验连通性） |
| GET | `/v1/mcp-servers/{mcp_server_id}/user-credentials` | 列出用户凭据 |
| POST | `/v1/mcp-servers/{mcp_server_id}/user-credentials` | 新增/更新用户凭据 |
| POST | `/v1/mcp-servers/{mcp_server_id}/user-credentials/delete` | 删除用户凭据（body: `user_id` + `credential_key`） |

---

## 四、Biz 层

### 4.1 领域模型

```go
type MCPServer struct {
    ID, Key, Name, Description, Status string
    Enabled bool
    SortOrder int
    ConfigJSON, MetadataJSON string
    CreatedAt, UpdatedAt, DeletedAt string
}
```

- **`ConfigJSON`**：由 `mcp/config.ServerConfig` 解析（transport、url、auth、timeout 等）。
- **`MetadataJSON`**：由 `mcp/metadata` 读写（`health_status`、`last_health_at`、`reconnect_count` 等）。

### 4.2 Usecase（`internal/biz/mcp_server.go`）

| 方法 | 说明 |
|------|------|
| `List` / `Get` / `Create` / `Update` / `Delete` | CRUD |
| `TestMCPServer` | `probe.Evaluate` + 内部 `persistHealth`（写入 `metadata_json`） |
| `ValidateConfig` | URL 预检（不持久化） |
| `RecordReconnectMetadata` | `mcpobserve` 回调递增 `reconnect_count` |
| `MarkHealthAlertEmitted` | `mcp/alert` 回调标记告警已发送 |

> **注**：`mcp/health` 后台探活通过调用 `TestMCPServer` 间接触发 `persistHealth`，无独立公开 `PersistHealth` 方法。

### 4.3 用户凭据（`internal/biz/mcp_user_credential.go`）

```go
type MCPServerUserCredential struct {
    ID, MCPServerID, UserID, CredentialKey, Status string
    SecretRef, MetadataJSON string
    CreatedAt, UpdatedAt, DeletedAt string
    Configured bool
    MaskedPreview string
}
type MCPServerUserCredentialRepo interface { ... }
```

| 方法 | 说明 |
|------|------|
| `ListUserCredentials` | 列出用户在某 MCP 服务器上的凭据 |
| `UpsertUserCredential` | 新增/更新用户凭据（密钥经 `crypto.EncryptChannelSecretRef` 加密存储为 `secret_ref`） |
| `DeleteUserCredential` | 按 `credential_key` 删除用户凭据 |
| `ResolveUserAuthHeaders` | 合并 per-user 凭据到请求 headers（支持 `require_user_credentials`） |

**`MaskedPreview` / `Configured`** 为 biz 层派生字段（`sanitizeMCPUserCredential` 由 `secret_ref` 派生），不落库。

### 4.4 Agent MCP 关联（`internal/biz/agent_mcp_effective.go`）

```go
type AgentMCPTooling struct { agents *AgentUsecase; mcp *MCPServerUsecase }
func (t *AgentMCPTooling) EffectiveServersForAgent(ctx, agentID) ([]EffectiveMCPServer, error)
```

**生效规则**：

1. Agent 有效工具含 `mcp_tool_set` 或 `mcp_broker`（`biz.ToolKeyMCPToolSet` / `ToolKeyMCPBroker`）。
2. 平台行：`enabled` 且 `status=active` 且未软删。
3. `tools_allow_json` / `tools_deny_json` 中 `mcp:<server_key>` 过滤（`MCPPolicyFromAgentEffectiveTools` + `FilterEffectiveMCPServers`）。

---

## 五、Data 层

### 5.1 表：`mcp_server`

文件：`internal/data/ent/schema/platform_mcp_server.go` → 表 `mcp_server`。

| 列 | 说明 |
|----|------|
| `id` | 主键（随机 hex） |
| `server_key` | Slug，对应表单 `name`、API `key`；Agent 策略前缀 `mcp:<server_key>` |
| `name` | 展示名，对应表单 `display_name` |
| `description` | 描述 |
| `status` | `active` / `error` / `deleted`（探活失败时可为 `error`） |
| `enabled` | 是否启用 |
| `sort_order` | 排序 |
| `config_json` | 连接配置（见 §5.3） |
| `metadata_json` | 健康与重连元数据（见 §5.4） |
| `created_at` / `updated_at` / `deleted_at` | 软删除时间戳 |

索引：`idx_mcp_server_status_enabled`（`status`, `enabled`）、`idx_mcp_server_deleted_at`（`deleted_at`）。

### 5.2 表：`mcp_server_user_credential`

文件：`internal/data/ent/schema/platform_mcp_user_credential.go` → 表 `mcp_server_user_credential`。

| 列 | 说明 |
|----|------|
| `id` | 主键 |
| `mcp_server_id` | 关联 MCP 服务器 |
| `user_id` | 用户 ID |
| `credential_key` | 凭据键名（如 `authorization`） |
| `status` | 状态（默认 `active`） |
| `secret_ref` | 加密后的密钥引用 |
| `metadata_json` | 元数据（默认 `{}`） |
| `created_at` / `updated_at` / `deleted_at` | 时间戳 |

唯一索引：`(mcp_server_id, user_id, credential_key)`。

### 5.3 `config_json` 字段（逻辑）

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
| `require_user_credentials` | 产品标记；为真时需用户级凭据（`mcp_server_user_credential`） |
| `probe_mode` | `connectivity`（默认）\| `auth_aware`；控制探活策略 |
| `adhoc_timeout_sec` | Broker AdHoc 请求超时 |

### 5.4 `metadata_json` 字段（逻辑）

| 字段 | 说明 |
|------|------|
| `health_status` | `ok` / `error` / `unknown` |
| `last_health_at` | RFC3339 |
| `last_error_message` | 列表状态灯 Tooltip |
| `health_error_since` | 健康错误起始时间（用于持续告警判定） |
| `last_health_alert_at` | 最近一次健康告警时间 |
| `last_reconnect_at` / `reconnect_count` | `mcpobserve` 重连可观测 |

**说明**：敏感 `headers` / `auth` 不落日志明文；列表 API 可对值脱敏。

删除为软删除：`deleted_at` + `status=deleted`。

`mcpServerRepo` 同时实现 `biz.MCPServerRepo` 和 `biz.MCPServerUserCredentialRepo`。

---

## 六、运行时层

### 6.1 装配链路

```
AgentMCPTooling.EffectiveServersForAgent
  → agent/tool_assembly.resolveMCPServers
      → mcp/config.ParseServerConfigJSON
      → applyMCPAuthHeaders (mcp_oauth.go)
  → tools/trpc.BuildToolsets
      → tools.buildMCPToolSet (timeout + ReconnectObserver)
      → tools.buildMCPBrokerTools (有服务器行时挂载 Broker)
  → llmagent.WithToolSets / WithTools
```

### 6.2 MCP ToolSet（`internal/tools/toolset.go`）

- `DefaultMCPServerTimeoutSec = mcpdefaults.DefaultRuntimeTimeoutSec`（60）；`timeout_sec` 映射 `ConnectionConfig.Timeout`。
- `mcpobserve.ObserverForServer` + `WithSessionReconnect`（SSE/Streamable 默认最多 3 次，可配置 `session_reconnect_max`）。

### 6.3 MCPBroker

`buildMCPBrokerTools`：`mcp_list_servers` / `mcp_list_tools` / `mcp_inspect_tools` / `mcp_call`。

**挂载条件**（`internal/agent/tool_assembly.go`）：
- Agent 有效工具须含 `mcp_broker`（`biz.ToolKeyMCPBroker`）。
- 当存在有效 MCP 服务器行时，`buildMCPBrokerFromServers` 构建 `MCPBrokerConfig`。
- AdHoc HTTP：`ProductionAllowAdHocHTTP(server.allow_adhoc_http, system_settings.mcp_allow_adhoc_http)`。

### 6.4 探活与健康

| 组件 | 说明 |
|------|------|
| `mcp/probe.Evaluate` | 管理面测试；HTTP 探活上限 `defaults.DefaultProbeTimeoutSec`（10s） |
| `mcp/probe.ProbeStrategy` | 探活策略接口：`ConnectivityProbe`（默认，仅网络连通性）、`AuthAwareProbe`（带 OAuth/API Key 探活）、`FullHandshakeProbe`（预留） |
| `mcp/health.Runner` | 默认 `defaults.DefaultHealthInterval`（5min）；bounded concurrency（`maxConcurrentProbes=8`）；`MCP_HEALTH_INTERVAL` / `MCP_HEALTH_DISABLED` |
| `mcp/alert.Publisher` | 持续错误超 `defaults.DefaultSustainedErrorAfter`（5min）后发布 EventBus 告警信封 |
| Prometheus | `aranea_mcp_health_probe_total` / `_duration_seconds` / `aranea_mcp_session_reconnect_total` / `aranea_mcp_invocation_total` |

### 6.5 认证（`internal/agent/mcp_oauth.go`）

`config_json.auth` 支持：`api_key` / `bearer` / `oauth2_static` / `oauth2_client_credentials` / `oauth2_refresh` → 合并进请求 `headers`。

---

## 七、Service 层

`internal/service/mcp_server.go`：Proto ↔ `biz.MCPServer`；`TestMCPServer` 返回探活结果；`ValidateMCPServer` 返回预检结果（不持久化）；用户凭据 CRUD；CRUD 写 Admin Audit。`patchFromProtoMCPWithDiff` 解决 proto3 零值歧义。

---

## 八、Wire 注入

- `NewMCPServerUsecase` → `NewMCPServerService`
- `NewAgentMCPTooling` → `runtime.PersistenceSet.AgentMCP`
- `provideMCPHealthRunner` → `main` 启动后台探活
- MCP 会话重连遥测由框架内部处理（`mcpobserve` 包仅保留辅助函数 `DefaultSessionReconnectMax`/`EffectiveSessionReconnectMax`/`IsRecentReconnect`；框架已移除 `ReconnectObserver`/`ReconnectEvent` 回调，不再需要在 chat 启动时调用 `SetBus`/`SetMetadataRecorder`）

---

## 九、Web 前端

```
web/src/features/mcp/
├── api.ts                     — API 客户端（CRUD + validate + 用户凭据）
├── types.ts                   — McpServerConfig / McpServerMetadata / McpUserCredential / auth
├── utils.ts                   — parseJSON 安全解析
├── useMcpServersPage.ts       — 页面 composable（rows/search/paging/状态管理）
├── useMcpServerForm.ts        — 表单 composable（验证/config_json 序列化）
├── useMcpUserCredentialDialog.ts — 用户凭据对话框 composable
├── McpServerFormDialog.vue    — 创建/编辑对话框
└── McpUserCredentialDialog.vue — 用户凭据管理对话框
web/src/pages/McpServersPage.vue
```

表单 `name` → API `key`；`display_name` → API `name`；连接字段写入 `config_json`；健康灯读 `metadata_json`；用户凭据通过 `McpUserCredentialDialog` 管理。

---

## 十、UX 规范

> 用户视角的交互规格见需求文档 [19-mcp.md](./19-mcp.md)。本节定义前端组件层面的 UX 设计规范。

### 10.1 信息架构与路由

| 页面 | 路由（示例） | 说明 |
|------|--------------|------|
| MCP 服务器列表 | `/mcp-servers` | 搜索、Registry 表格列表（`AppRegistryTable`）、空态、刷新、添加 |
| 新建 | 列表内「+ 添加服务器」→ `QDialog` 或独立路由 `/mcp-servers/new` | 表单同编辑 |
| 编辑 | 编辑 → 同上对话框预填 或 `/mcp-servers/:id/edit` | 与创建共用组件 `McpServerFormDialog` |

### 10.2 列表页 UI

#### 顶栏与工具条

| 区域 | 内容 |
|------|------|
| 标题 | 「MCP 服务器」 |
| 副标题 | 「管理 Model Context Protocol 服务器连接」 |
| 右上 | 「+ 添加服务器」（`QBtn` color=primary）；「刷新」（`QBtn` outline + `refresh` 图标） |
| 搜索 | `QInput` `debounce` + `clearable`，占位「搜索服务器…」；按 `name`、`display_name` 前端过滤 |

#### 列表：`AppRegistryTable` 表格

实现为 Registry 表格（`components/mcp/McpServersTable.vue` + `mcpServerTableUi.ts` 列定义），与 Webhooks/Tools/Channels 等管理页一致；服务端分页 + 搜索。

| 列 | 内容 |
|----|------|
| 服务器 | 状态灯圆点（§10.2.3）+ `name`（主标题）/ `key`（副标）；悬停 `AppRegistryHoverTip` 显示地址/命令（`url` 或 `command args`） |
| 传输 | `stdio` / `SSE` / `Streamable HTTP` |
| 工具前缀 | `mcp_{tool_prefix}__`（等宽 code 样式）；空显示 `—` |
| 超时 | `timeout_sec` + `s`（缺省 60s） |
| 健康 | `health_status` 本地化 `QChip`：`ok`→正常（positive）/ `error`→异常（negative）/ `degraded`→退化（warning）；悬停显示 `last_error_message` |
| 启用 | 可交互 `QToggle`（dense，primary），切换即调 `PATCH /mcp-servers/:id` 部分更新；切换中禁用 |
| 操作 | 用户凭据（`vpn_key`，仅 `require_user_credentials=true` 显示）、测试连接（`science`）、编辑、删除 |

#### 空态

中央插头图标（`QIcon` 或插画）、主文案「暂无 MCP 服务器」、副文案「添加您的第一个 MCP 服务器以开始使用。」、可选主按钮复用「添加服务器」。

#### 状态灯（连接/健康）

| 灯色 | 含义 | 数据来源 |
|------|------|----------|
| 灰 | 未检测 / 从未成功连接 | `last_health_at` 为空且 `enabled=false` 或未跑过检测 |
| 绿 | 最近一次测试连接或后台探活成功 | `health_status=ok` |
| 红 | 最近一次失败或初始化错误 | `health_status=error` 或存在 `last_error_message` |
| 黄（可选） | 已启用但超过 N 分钟未探活成功 | `health_status=degraded` 或超时策略 |

**实现**：在 MCP 组件左侧或 `QItemSection` side 内放 `QBadge` `rounded` 小圆点，或 `span` 8px 圆 + `bg-positive`/`bg-negative`/`bg-grey`；`QTooltip` 展示最近错误摘要或「最近成功：时间」。

#### 行内操作（CRUD）

| 操作 | 行为 |
|------|------|
| 编辑 | 打开 `QDialog` 表单，`GET /mcp-servers/:id` 预填 |
| 删除 | `QDialog` 确认：「删除「{name}」后，依赖该服务器的工具（mcp_{prefix}__*）将不可用〔；所有用户已配置的凭据将一并删除〕」；`DELETE /mcp-servers/:id` |
| 测试连接（可选） | `POST /mcp-servers/:id/test`，结果 `Notify` 或该项内短暂提示 |

### 10.3 添加/编辑表单（对话框）

标题：「添加 MCP 服务器」/「编辑 MCP 服务器」；内容区 `QScrollArea`；底栏按钮：测试连接（左）、取消、创建/保存（主色）。

#### 字段控件映射

| 字段（逻辑名） | 控件 | 校验 / 说明 |
|----------------|------|-------------|
| `name`（表单）→ API `key` | `QInput` | Slug：仅小写字母、数字、连字符；平台内唯一；占位 `my-mcp-server` |
| `display_name`（表单）→ API `name` | `QInput` | 展示用，如 `SQL Server` |
| `transport` | `QBtnToggle` 或 `QOptionGroup` `inline` | `stdio` \| `sse` \| `streamable_http`（界面对应 Streamable HTTP） |
| `url` | `QInput` | `sse`/`streamable_http` 必填；HTTP(S) URL；服务端 SSRF 校验 |
| `command` / `args` | `QInput` + 多行或数组编辑 | `stdio` 时必填；可执行路径与参数 |
| `headers` | 动态键值行 | 每行 key + value（敏感 value 用 `password` 或掩码）；+ 添加请求头；行内删除 |
| `env` | 动态键值行 | 占位「变量名称」「值」；+ 添加变量 |
| `tool_prefix` | `QInput`，前缀可视化 `mcp_` + 输入 | 空则服务端从 `name` 派生；说明文案：`Tools: mcp_{prefix}__{tool}` |
| `timeout_sec` | `QInput` `type=number` 或 `QSlider` | 默认 60 |
| `session_reconnect_max` | `QInput` `type=number`（0–10） | 标签「会话重连次数」；说明：SSE / Streamable HTTP 断线重连上限，0=关闭 |
| `enabled` | `QToggle` | 默认开 |
| `require_user_credentials` | `QToggle` | 副文案：每个用户须配置自己的凭据，否则无法使用 |
| `probe_mode` | `QSelect` | `connectivity`（默认）\| `auth_aware` |
| `allow_adhoc_http` | `QToggle` | 服务器级 AdHoc 开关；仍需系统设置 `mcp_allow_adhoc_http` 双门禁 |

**动态键值**：`v-for` 行 + `QInput`×2 + `QBtn` `icon="delete"`；或用小型 `QTable` `hide-pagination` 内嵌编辑。

#### 传输与字段显隐

| `transport` | 展示 URL | 展示 command/args |
|-------------|----------|-------------------|
| `streamable_http` / `sse` | 是 | 否 |
| `stdio` | 否 | 是 |

#### 校验与错误展示

- 表单底部或字段下方：红色 `div.text-negative` 展示服务端返回（如 URL 非法、SSRF、传输初始化失败）。
- 测试连接：`POST` 不持久化或先保存草稿再测；失败时保留错误在对话框内。

#### Quasar 映射摘要

列表：`QList` → `QItem` → `McpServerItem`（MCP 组件）；表单对话框：`QDialog` + `QCard` + `QCardSection` + `QScrollArea` + `QForm`（`@submit.prevent`）；`QBtnToggle` `spread`/`no-caps`；`Notify` 成功/失败。

---

*文档版本：4.0 — 2026-06-17 按三件套边界重组：UX 规范/持久化模型/API 端点表从需求文档迁入；修正 user-credential 表名/字段、Delete 端点方法、PersistHealth 描述、Broker 挂载条件。*
