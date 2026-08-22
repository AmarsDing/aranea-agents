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
  rpc ListMCPServers(ListMCPServersRequest) returns (ListMCPServersResponse) {
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

**MCPServer**：`id` / `key`（slug）/ `name`（展示名）/ `description` / `status` / `enabled` / `sort_order` / `config_json` / `metadata_json` / 时间戳 / `shared`（派生自 `workspace_id == ""`：内置共享服务器，所有工作区可读但租户不可变更；不下发真实 workspace_id 避免泄露租户拓扑）。

**ListMCPServersRequest**：`page`（1-based）/ `page_size`（默认 20，上限 100）/ `search`。零值/缺省走旧版不分页路径（picker、health runner、CLI 等内部调用方）；HTTP query fallback（`page`/`page_size`/`search`）保留兼容旧客户端。

**ListMCPServersResponse**：`items` / `total` / `page` / `page_size`（分页元数据）。

**MCPServerTestResponse**：`ok` / `status` / `message` / `details_json`。

**ValidateMCPServerResponse**：`ok` / `status` / `message` / `details_json`（不持久化）。

**MCPServerUserCredential**：`id` / `mcp_server_id` / `user_id` / `credential_key` / `status` / `configured` / `masked_preview` / `created_at` / `updated_at`。

**DeleteMCPServerUserCredentialRequest**：`mcp_server_id` / `user_id` / `credential_key`（注意：使用 POST `/delete` 而非 RESTful DELETE，因需在 body 传 `user_id` + `credential_key`）。

---

## 三、API 端点表

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/v1/mcp-servers` | 列表；query 支持 `page` / `page_size` / `search`（服务端分页 + 搜索）；不带分页参数时返回全量（兼容内部调用方） |
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
    // WorkspaceID： owning workspace ID（P2-B 租户隔离）。
    // 空 = 共享/内置（所有工作区可见，如系统内置 playwright）；
    // 非空 = 租户私有（仅 owning workspace 可见）。
    WorkspaceID string
}
```

- **`ConfigJSON`**：由 `mcp/config.ServerConfig` 解析（transport、url、auth、timeout 等）。
- **`MetadataJSON`**：由 `mcp/metadata` 读写（`health_status`、`last_health_at`、`reconnect_count` 等）。

### 4.2 Usecase（`internal/biz/mcp_server.go`）

| 方法 | 说明 |
|------|------|
| `List` / `Get` / `Create` / `Update` / `Delete` | CRUD；`List` 接受 `MCPListQuery{WorkspaceID, Search, Limit, Offset}`；`Update` 经 `MCPServerUpdate` 部分更新 DTO（nil = 不改），**刻意不含 `status`/`metadata_json`**——两者由系统管理（health runner / 重连簿记 / 删除），repo 仅写字段级 admin 可编辑列，避免陈旧表单快照回滚并发健康写入（同 RV-01 类） |
| `ListPaged` | 管理台注册表分页查询（默认 `Limit=20`，上限 100），返回 `MCPListResult{Items, Total, Limit, Offset}` |
| `TestMCPServer` | `probe.Evaluate` + 内部 `persistHealth`（写入 `metadata_json`） |
| `ValidateConfig` | URL 预检（不持久化）；签名 `ValidateConfig(ctx, configJSON)`，内部始终以 `enabled=true` 评估 |
| `RecordReconnectMetadata` | `mcpobserve` 回调递增 `reconnect_count` |
| `MarkHealthAlertEmitted` | `mcp/alert` 回调标记告警已发送 |
| `PersistRotatedRefreshToken` | OAuth2 refresh token 轮换回写：解密 `config_json` → 替换 `auth.refresh_token` → 重加密 → **字段级写**（`UpdateMCPServerConfigJSON` 仅写 `config_json`+`updated_at`，不覆盖并发健康元数据，RV-01），防止进程重启后复活已被吊销的旧 token（T5） |

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
4. **工作区可见性（R2 收紧）**：非系统调用方按 `workspace.IDFromContext(ctx)` 过滤——仅共享服务器（`workspace_id=""`）+ 本工作区私有服务器生效，与 `ListMCPServers` RPC 租户可见性一致；系统调用方（cron/prewarm，`WithSystemWorkspace`）不过滤。此前 runtime 层不过滤，租户 Agent 可挂载其他租户私有 MCP 服务器（跨租户泄漏）。

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
| `workspace_id` | owning workspace ID（默认 `""` = 共享/系统内置；P2-B 租户隔离） |
| `created_at` / `updated_at` / `deleted_at` | 软删除时间戳 |

索引：`idx_mcp_server_server_key_active`（`server_key` 部分唯一索引，`WHERE deleted_at = ''`——R1 修复：列级 UNIQUE 含软删除墓碑行，同 key 软删后重建报 23505；改为仅约束活跃行，新库由 Ent Schema 自动创建，存量库经 DDL 迁移 `20261209_mcp_partial_unique_index` 清理旧约束并补齐）、`idx_mcp_server_status_enabled`（`status`, `enabled`）、`idx_mcp_server_deleted_at`（`deleted_at`）、（`workspace_id`, `enabled`）。

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

唯一索引：`idx_mcp_credential_unique_active`（`(mcp_server_id, user_id, credential_key)` 部分唯一索引，`WHERE deleted_at = ''`——R1 同因修复，墓碑行不再阻塞同 key 凭据重建）。

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
- `MCPServerConfig.AuthHeaderName` 透传 `auth.header_name`，使用户级凭据注入器（`HeaderInjector`）与静态 auth 路径写入同一 header（T2）。

#### 连接池（`internal/tools/mcp_pool.go`）

- 进程级 ToolSet 池，按 server 配置 key 复用连接；空闲超 `idleTTL`（默认 10min）由 reaper 回收。
- **关闭语义（T3）**：`Pool.Close` 幂等——空闲 entry 立即关闭；仍被引用的 entry 标记 `closing`，延迟到最后一次 `release` 时关闭（避免 shutdown use-after-close）；`Acquire` 在 pool 已关闭时降级为非池化新建，连接中途遇 Close 则将未入池的 ToolSet 直接交给调用方（所有权与非池化路径一致）。
- **装配错误路径（T4）**：`tools.Assemble` 任一 phase 失败时调用 `closeAll()` 关闭已装配的全部 ToolSet——池化 MCP 连接归还池引用、非池化连接真实关闭，不再泄漏。

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

- **oauth2_static 守卫（T1）**：令牌缺失/校验失败时禁止回退注入同一个（已过期）`access_token`——留空 key 不注入 auth header，让连接/探活显式失败以提示重新配置，避免 401 被掩盖成工具调用失败。
- **refresh token 轮换回写（T5）**：`oauth2_refresh` 换发成功且 provider 轮换 refresh token 时，经 `SetMCPRefreshTokenPersister` 钩子（service 层 `NewMCPServerService` 装配）调用 `MCPServerUsecase.PersistRotatedRefreshToken` 回写 `config_json`；回写失败非致命（内存缓存已持新 token），由钩子闭包记录告警日志。

---

## 七、Service 层

`internal/service/mcp_server.go`：Proto ↔ `biz.MCPServer`；`TestMCPServer` 返回探活结果；`ValidateMCPServer` 返回预检结果（不持久化）；用户凭据 CRUD；CRUD 写 Admin Audit（含用户凭据 upsert/delete，`AuditVerbCredentials`）。`patchFromProtoMCPWithDiff` 解决 proto3 零值歧义。

> **单次查询守卫（M1）**：`checkMCPServerAccess` 校验权限并返回已读取的 server，`Get`/`Update`/`Delete` 复用该返回值——每个 RPC 只读一次 DB，不再「守卫一次 + 业务再读一次」。IDOR 拒绝时记进程日志 Warn（`mcp.server.access_denied`，含 server_id/caller_workspace/mutate），对外仍返回 NotFound 不泄露存在性（M2）。`UpdateMCPServer` 发射流程日志 `mcp.server.update`（已登记步骤注册表）。

### 7.1 工作区守卫（P2-B IDOR 防护）

双级守卫，按操作读写语义选择：

| 守卫 | 语义 | 适用 RPC |
|------|------|---------|
| `assertMCPServerAccess` | **读级**：共享服务器（`workspace_id=""`）对所有租户放行；租户私有仅 owning workspace | `GetMCPServer`、`TestMCPServer`、`ListMCPServerUserCredentials`、凭据写/删 |
| `assertMCPServerMutateAccess` | **变更级**：共享服务器对租户调用方 fail-closed（仅系统工作区可变更） | `UpdateMCPServer`、`DeleteMCPServer` |

> **为什么 `TestMCPServer` 用读级守卫**：探活不变更租户配置，仅刷新系统健康簿记元数据（`health_status`/`last_health_at`）——与后台 health runner 对共享服务器的写入语义一致。若用变更级守卫，内置服务器（如 playwright）的「测试连接」按钮会对所有租户 404，而它们恰恰是运维最需要探测的对象。回归测试：`TestMCPServerService_TestMCPServer_SharedServerAllowedForTenant`。
>
> **为什么凭据写/删用读级守卫（N1）**：用户凭据是调用方自己的数据（`resolveMCPCredentialUserID` 将非管理员绑定到自身 user_id），不是服务器配置；变更级守卫会对共享/内置服务器（`workspace_id=""`）fail-closed，使每用户凭据在恰恰需要它们的行（`require_user_credentials`）上不可用。跨租户私有服务器仍不可见：守卫内的工作区作用域 Get 对其返回 NotFound。回归测试：`TestMCPServerService_UpsertCredential_SharedServerAllowedForTenant` / `TestMCPServerService_UpsertCredential_CrossTenantRejected`。

---

## 八、Wire 注入

- `NewMCPServerUsecase` → `NewMCPServerService`（构造函数内装配 `chatagent.SetMCPRefreshTokenPersister` 钩子 → `uc.PersistRotatedRefreshToken`，T5）
- `NewAgentMCPTooling` → `runtime.PersistenceSet.AgentMCP`
- `provideMCPHealthRunner` → `main` 启动后台探活
- MCP 会话重连遥测由框架内部处理（`mcpobserve` 包仅保留辅助函数 `DefaultSessionReconnectMax`/`EffectiveSessionReconnectMax`/`IsRecentReconnect`；框架已移除 `ReconnectObserver`/`ReconnectEvent` 回调，不再需要在 chat 启动时调用 `SetBus`/`SetMetadataRecorder`）

---

## 九、Web 前端

```
web/src/features/mcp/
├── api.ts                     — API 客户端（CRUD + validate + 用户凭据）；list/create/update 返回 McpServerRow 收紧类型
├── types.ts                   — McpServerRow / McpServerConfig / McpServerMetadata / McpUserCredential / McpHealthTone（'ok'|'error'|'degraded'|'unknown'）/ auth
├── utils.ts                   — parseJSON 安全解析
├── useMcpServersPage.ts       — 页面 composable（rows/search/paging/状态管理；healthTone 返回 McpHealthTone）
├── useMcpServerForm.ts        — 表单 composable（验证/config_json 序列化）
├── useMcpUserCredentialDialog.ts — 用户凭据对话框 composable
├── McpServerFormDialog.vue    — 创建/编辑对话框
├── McpUserCredentialDialog.vue — 用户凭据管理对话框
└── __tests__/                 — useMcpServerForm（buildPayload）/ useMcpServersPage（healthTone/healthTooltip）/ mcpServerListQuery 单测
web/src/pages/McpServersPage.vue
```

表格列宽复用 `features/ui/registryTableColumns` 的 `REGISTRY_COL_W` token（操作列 `actionsWide`），不硬编码像素值。

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
| 右上 | 「+ 添加服务器」（`QBtn` color=primary）；「刷新」（`QBtn` outline + `refresh` 图标）；手动刷新成功后 `Notify` 反馈「已刷新，共 N 个服务器，最近健康检测：时间」（取各行 `metadata_json.last_health_at` 最大值，让运维感知健康列数据新鲜度——探活由服务端定时执行） |
| 搜索 | `QInput` `debounce` + `clearable`，占位「搜索服务器…」；服务端搜索（`search` query 参数，匹配 `name`/`key`） |

#### 列表：`AppRegistryTable` 表格

实现为 Registry 表格（`components/mcp/McpServersTable.vue` + `mcpServerTableUi.ts` 列定义），与 Webhooks/Tools/Channels 等管理页一致；服务端分页 + 搜索。

| 列 | 内容 |
|----|------|
| 服务器 | 状态灯圆点（§10.2.3）+ `name`（主标题，`shared=true` 时追加「内置」outline 徽标）/ `key`（副标）；悬停 `AppRegistryHoverTip` 显示地址/命令（`url` 或 `command args`） |
| 传输 | `stdio` / `SSE` / `Streamable HTTP` |
| 工具前缀 | `mcp_{tool_prefix}__`（等宽 code 样式）；空显示 `—` |
| 超时 | `timeout_sec` + `s`（缺省 60s） |
| 健康 | `health_status` 本地化 `QChip`：`ok`→正常（positive）/ `error`→异常（negative）/ `degraded`→退化（warning）；悬停显示 `last_error_message` |
| 启用 | 可交互 `QToggle`（dense，primary），切换即调 `PATCH /mcp-servers/:id` 部分更新；切换中禁用；`shared=true` 行禁用（内置共享服务器租户不可变更） |
| 操作 | 用户凭据（`vpn_key`，仅 `require_user_credentials=true` 显示）、测试连接（`science`，共享行可用）、编辑、删除（`shared=true` 行编辑/删除禁用 + tooltip「内置共享服务器，不可编辑/删除」） |

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
- 全局错误文案：`services/axiosHandler.ts` 从 Kratos 错误信封（`{code, reason, message}`）提取后端 `message` 覆盖 axios 通用文案（如 "Request failed with status code 404"），调用方 `Notify` 直接展示后端解释（如 "mcp server not found"）。

#### Quasar 映射摘要

列表：`QList` → `QItem` → `McpServerItem`（MCP 组件）；表单对话框：`QDialog` + `QCard` + `QCardSection` + `QScrollArea` + `QForm`（`@submit.prevent`）；`QBtnToggle` `spread`/`no-caps`；`Notify` 成功/失败。

---

*文档版本：5.1 — 2026-08-14 同步深入评审整改：R1 软删除感知部分唯一索引（§5.1/§5.2）、R2 EffectiveServersForAgent 工作区过滤（§4.4）、M1-M6 单次查询守卫/IDOR 拒绝日志/凭据审计/`mcp.server.update` 流程日志/死代码清理（§七）、T1-T2 oauth2_static 守卫 + AuthHeaderName 透传（§6.5/§6.2）、T3-T4 连接池延迟关闭 + Assemble 错误路径清理（§6.2）、T5 refresh token 轮换回写（§4.2/§6.5/§八）、F1-F4 前端类型收紧与测试（§九）。*

---

## 子模块：Agent 作为 MCP Server（D3 评估，不实现）

> 日期：2026-08-22。来源：[2026-08-22-analysis-codex-vs-aranea.md](../reports/2026-08-22-analysis-codex-vs-aranea.md) Phase D3。  
> **结论：本期不实现。** 本附录只定边界，避免以后把「IDE 调一只 Aranea 专项」做成第二套编排。

### 动机

Codex / Claude Desktop 可以把本机 Agent 暴露成 MCP server，让 IDE 不经过 Chat UI 直接调工具。Aranea 已是 **MCP 客户端**（stdio / SSE / Streamable HTTP + Broker）。反向做 server 是产品能力，不是框架缺口。

### 不该做的事

| 禁止 | 原因 |
|------|------|
| 把整只公司 / 精灵工具箱挂成一只 MCP server | 违反组织铁律：专项自带工具面，禁止全员共用精灵工具箱 |
| 用 MCP 代替花名册 / PlanExecutor / Team Graph | 编排只绑编制，不把员工做成 IDE 精灵分身 |
| 复用 `spawn_agent` 当主编排 | 与 M67/M78 冲突 |
| 为了「像 Codex」先做 Code Mode / 去掉 SSE | SSE 已是一等传输；Code Mode 不是本期目标 |

### 若以后做，最小切片

1. **一只专项一个 server**，工具面 = 该 Agent 的 ToolsProfile / Allow / Deny / MCP mount，不并集。
2. 鉴权：workspace 级 token，禁止匿名 stdio 扫全库。
3. 会话：每次 MCP 调用对应独立 run，写入现有 Task/Step，不绕过确认门与 `workspace_sandbox`。
4. 治理岗（`dept_lead` / `company_lead`）不暴露为 MCP 工具。
5. 传输优先 Streamable HTTP（已有客户端对偶）；stdio 仅本机 IDE。

### 验收（未开工）

IDE 能对 **一只已注册专项** 列出并调用其允许工具，不经过 Chat UI；调用出现在该 Agent 的执行轨迹；越权工具 / 越界路径被拒绝。不验收「把 Aranea 当通用 MCP 网关」。

---

## 子模块：MCP Resource 面与 mid-turn catalog 刷新（E8 评估）

> 日期：2026-08-22。来源：[2026-08-22-analysis-codex-vs-aranea-post-ad.md](../reports/2026-08-22-analysis-codex-vs-aranea-post-ad.md) E8。  
> **结论：list/read 已落地；templates/subscribe 与 mid-turn 热刷新本期不实现。** 不做 Agent-as-server（见上一子模块）。

### 已有（客户端三件套的 2/3）

| 能力 | 状态 | 锚点 |
|------|------|------|
| `resources/list` | ✅ 业务工具 `mcp_list_resources` | `internal/tools/mcp_resources.go` → `tmcp.Connector.ListResources` |
| `resources/read` | ✅ 业务工具 `mcp_read_resource` | 同上 `ReadResource`；正文 100k runes |
| 命名 selector + 用户凭证头 | ✅ 与 broker 同语义 | `mcpResourceResolver`；禁止 ad-hoc URL（读面收敛攻击面） |
| 装配 | ✅ coding/spirit/full 走 broker 时一并挂上 | `buildMCPBrokerTools` + `mcp_schema_govern_test.go` 工具名表 |

Codex 的 Resource 三件套通常还包括 **templates**（URI 模板）和 **subscribe**（资源变更通知）。Aranea 已覆盖模型最常用的 list/read。

### 被客户端挡住的两面

`trpc-mcp-go@v0.0.10`：

| 方法 | Server | Client `Connector` |
|------|--------|--------------------|
| `resources/list` / `resources/read` | ✅ | ✅ |
| `resources/templates/list`（`MethodResourcesTemplatesList`） | ✅ `handler.go` | ❌ 无 `ListResourceTemplates` |
| `resources/subscribe`（`MethodResourcesSubscribe`） | ✅ | ❌ 无 Subscribe API |

本期 **不** 为 templates/subscribe 改 vendored 框架或手写 JSON-RPC。等客户端升版再评估封装。

### Mid-turn catalog refresh（E8，已落地脏标记）

Codex `refresh_mcp_if_dirty` 在 step 后热刷新。Aranea **不**打开「每轮 LLM 都 Initialize+ListTools」（0.2–5s）。做法：

1. 直连 `mcp_tool_set` 仍用 `WithToolsCacheTTL(5m)`。
2. 仅当装配结果里有可 `InvalidateToolsCache` 的 MCP ToolSet 时，打开 `WithRefreshToolSetsOnRun(true)`。缓存未失效时 `Tools()` 不打网络。
3. AfterTool：`mcp_list_tools` / `mcp_inspect_tools` / `mcp_list_servers` / `mcp_list_resources`，或 `unknown tool` / `tool not found`，调用 `InvalidateToolsCache`。下一轮 `FilterTools` → `Tools()` 再 `tools/list`。失败保留上一份目录。
4. 不热替换正在 `Call` 的 Tool 对象；不扫全工作区；配置变更仍走 `MCPVersionHash` 下一请求重建 Agent。

Broker 的 `mcp_list_tools` 本身已是 live list，脏标记主要服务直连 mount。
