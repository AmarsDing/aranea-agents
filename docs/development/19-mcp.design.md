# MCP 集成模块 — 实现设计文档

> 对应需求：[19 mcp.md](./19%20mcp.md)
> 遵循规范：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)
> **2026-06-06 校准**：补 Probe 策略化、defaults 包、classify 包、用户凭据、ValidateMCPServer、alert 包。

---

## 一、模块概述

MCP（Model Context Protocol）服务器管理：平台注册、健康探活、Agent 级挂载、Broker 运行时发现。领域逻辑在 `internal/biz`；协议客户端在 `internal/tools`（trpc-agent-go `tool/mcp` + `tool/mcpbroker`）。

### 分层职责（SRP）

| 包 | 职责 |
|----|------|
| `internal/mcp/config` | `config_json` 解析、Transport 类型化、`ConnectionConfig` 映射 |
| `internal/mcp/probe` | 管理面连通性评估（stdio PATH / HTTP + SSRF）；`ProbeStrategy` 接口 + `ConnectivityProbe` / `AuthAwareProbe` |
| `internal/mcp/metadata` | `metadata_json` 健康与重连字段合并 |
| `internal/mcp/health` | 后台定时探活 → `MCPServerUsecase.PersistHealth`；bounded concurrency（semaphore） |
| `internal/mcp/alert` | 持续健康错误告警：`Publisher.MaybeEmitAfterHealth` → EventBus 信封 |
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
    option (google.api.http) = { delete: "/v1/mcp-servers/{mcp_server_id}/user-credentials/{id}" };
  }
}
```

**MCPServer**：`id` / `key`（slug）/ `name`（展示名）/ `description` / `status` / `enabled` / `sort_order` / `config_json` / `metadata_json` / 时间戳。

**MCPServerTestResponse**：`ok` / `status` / `message` / `details_json`。

**ValidateMCPServerResponse**：`ok` / `status` / `message` / `details_json`（不持久化）。

**MCPServerUserCredential**：`id` / `mcp_server_id` / `user_id` / `auth_type` / `masked_preview` / 时间戳。

---

## 三、Biz 层

### 3.1 领域模型

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

### 3.2 Usecase（`internal/biz/mcp_server.go`）

| 方法 | 说明 |
|------|------|
| `List` / `Get` / `Create` / `Update` / `Delete` | CRUD |
| `TestMCPServer` | `probe.Evaluate` + `persistHealth` |
| `ValidateConfig` | URL 预检（不持久化） |
| `PersistHealth` | 供 `mcp/health` 定时任务调用 |
| `RecordReconnectMetadata` | `mcpobserve` 回调递增 `reconnect_count` |
| `MarkHealthAlertEmitted` | `mcp/alert` 回调标记告警已发送 |

### 3.3 用户凭据（`internal/biz/mcp_user_credential.go`）

```go
type MCPServerUserCredential struct { ID, MCPServerID, UserID, AuthType, MaskedPreview string }
type MCPServerUserCredentialRepo interface { ... }
```

| 方法 | 说明 |
|------|------|
| `ListUserCredentials` | 列出用户在某 MCP 服务器上的凭据 |
| `UpsertUserCredential` | 新增/更新用户凭据（密钥加密存储） |
| `DeleteUserCredential` | 删除用户凭据 |
| `ResolveUserAuthHeaders` | 合并 per-user 凭据到请求 headers（支持 `require_user_credentials`） |

### 3.4 Agent MCP 关联（`internal/biz/agent_mcp_effective.go`）

```go
type AgentMCPTooling struct { agents *AgentUsecase; mcp *MCPServerUsecase }
func (t *AgentMCPTooling) EffectiveServersForAgent(ctx, agentID) ([]EffectiveMCPServer, error)
```

**生效规则**：

1. Agent 有效工具含 `mcp_tool_set` 或 `mcp_broker`（`biz.ToolKeyMCPToolSet` / `ToolKeyMCPBroker`）。
2. 平台行：`enabled` 且 `status=active` 且未软删。
3. `tools_allow_json` / `tools_deny_json` 中 `mcp:<server_key>` 过滤（`MCPPolicyFromAgentEffectiveTools` + `FilterEffectiveMCPServers`）。

---

## 四、Data 层

文件：`internal/data/ent/schema/platform_mcp_server.go` → 表 `mcp_server`。

文件：`internal/data/ent/schema/platform_mcp_user_credential.go` → 表 `platform_mcp_user_credential`（`mcp_server_id` + `user_id` + `auth_type` + `secret_ref` + `masked_preview`）。

删除为软删除：`deleted_at` + `status=deleted`。

`mcpServerRepo` 同时实现 `biz.MCPServerRepo` 和 `biz.MCPServerUserCredentialRepo`。

---

## 五、运行时层

### 5.1 装配链路

```
AgentMCPTooling.EffectiveServersForAgent
  → agent/tool_assembly.resolveMCPServers
      → mcp/config.ParseServerConfigJSON
      → applyMCPAuthHeaders (mcp_oauth.go)
  → tools/trpc.BuildToolsets
      → tools.buildMCPToolSet (timeout + ReconnectObserver)
      → tools.buildMCPBrokerTools (有服务器行时自动挂载 Broker)
  → llmagent.WithToolSets / WithTools
```

### 5.2 MCP ToolSet（`internal/tools/toolset.go`）

- `DefaultMCPServerTimeoutSec = 60`；`timeout_sec` 映射 `ConnectionConfig.Timeout`。
- `mcpobserve.ObserverForServer` + `WithSessionReconnect`（SSE/Streamable 默认最多 3 次，可配置 `session_reconnect_max`）。

### 5.3 MCPBroker

`buildMCPBrokerTools`：`mcp_list_servers` / `mcp_list_tools` / `mcp_inspect_tools` / `mcp_call`。

- 当存在有效 MCP 服务器行时，`tool_assembly` **自动**构建 `MCPBroker`（不必单独启用 `mcp_broker` 工具键）。
- AdHoc HTTP：`ProductionAllowAdHocHTTP(server.allow_adhoc_http, system_settings.mcp_allow_adhoc_http)`。

### 5.4 探活与健康

| 组件 | 说明 |
|------|------|
| `mcp/probe.Evaluate` | 管理面测试；HTTP 探活上限 `defaults.DefaultProbeTimeoutSec`（10s） |
| `mcp/probe.ProbeStrategy` | 探活策略接口：`ConnectivityProbe`（默认，仅网络连通性）、`AuthAwareProbe`（带 OAuth/API Key 探活）、`FullHandshakeProbe`（预留） |
| `mcp/health.Runner` | 默认 `defaults.DefaultHealthInterval`（5min）；bounded concurrency（`maxConcurrentProbes=8`）；`MCP_HEALTH_INTERVAL` / `MCP_HEALTH_DISABLED` |
| `mcp/alert.Publisher` | 持续错误超 `defaults.DefaultSustainedErrorAfter`（5min）后发布 EventBus 告警信封 |
| Prometheus | `aranea_mcp_health_probe_total` / `_duration_seconds` / `aranea_mcp_session_reconnect_total` / `aranea_mcp_invocation_total` |

### 5.5 认证（`internal/agent/mcp_oauth.go`）

`config_json.auth` 支持：`api_key` / `bearer` / `oauth2_static` / `oauth2_client_credentials` / `oauth2_refresh` → 合并进请求 `headers`。

---

## 六、Service 层

`internal/service/mcp_server.go`：Proto ↔ `biz.MCPServer`；`TestMCPServer` 返回探活结果；`ValidateMCPServer` 返回预检结果（不持久化）；用户凭据 CRUD；CRUD 写 Admin Audit。`patchFromProtoMCPWithDiff` 解决 proto3 零值歧义。

---

## 七、Wire 注入

- `NewMCPServerUsecase` → `NewMCPServerService`
- `NewAgentMCPTooling` → `runtime.PersistenceSet.AgentMCP`
- `provideMCPHealthRunner` → `main` 启动后台探活
- `chat` 启动时 `mcpobserve.SetBus` + `SetMetadataRecorder(RecordReconnectMetadata)`

---

## 八、Web 前端

```
web/src/features/mcp/
├── api.ts                     — API 客户端（CRUD + validate + 用户凭据）
├── types.ts                   — McpServerConfig / McpServerMetadata / McpUserCredential / auth
├── utils.ts                   — parseJSON 安全解析
├── useMcpServersPage.ts       — 页面 composable（rows/search/paging/状态管理）
├── useMcpServerForm.ts        — 表单 composable（验证/config_json 序列化）
├── McpServerFormDialog.vue    — 创建/编辑对话框
└── McpUserCredentialDialog.vue — 用户凭据管理对话框
web/src/pages/McpServersPage.vue
```

表单 `name` → API `key`；`display_name` → API `name`；连接字段写入 `config_json`；健康灯读 `metadata_json`；用户凭据通过 `McpUserCredentialDialog` 管理。

---

*文档版本：3.0 — 2026-06-06 与代码实现对齐：Probe 策略化、defaults/classify/alert 包、用户凭据、ValidateMCPServer、前端 composable 补全。*