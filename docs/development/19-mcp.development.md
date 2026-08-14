# MCP 协议 — 开发计划

> **版本**：2026-08-14 | **状态**：🟢 Phase 6 已落地；Lifecycle FSM 🟡（状态机 + ApplyHealth 已接入，告警/重连编排可继续收敛）；2026-08-14 深入评审整改 ✅（R1/R2/M1-M6/T1-T5/F1-F4）
> **需求**：[19-mcp.md](./19-mcp.md) · **设计**：[19-mcp.design.md](./19-mcp.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：I4-MCP-01 / I5-MCP-01 ✅
> **优化计划**：[38-tools-plugin-skill-mcp-optimization.development.md](./38-tools-plugin-skill-mcp-optimization.development.md)

---

## 1. 模块定位

MCP（Model Context Protocol）集成：平台注册外部 MCP 服务器，Agent 通过 trpc `MCPToolSet` / `MCPBroker` 挂载并调用工具。

**代码锚点**：

| 层次 | 路径 |
|------|------|
| API | `api/kratos/mcp_server/v1/` |
| Service | `internal/service/mcp_server.go` |
| Biz | `internal/biz/mcp_server.go`、`mcp_user_credential.go`、`agent_mcp_effective.go` |
| Data | `internal/data/mcp_server.go`、`mcp_user_credential.go`、`ent/schema/platform_mcp_server.go`、`platform_mcp_user_credential.go` |
| MCP 子系统 | `internal/mcp/config`、`probe`、`metadata`、`health`、`alert`、`classify`、`defaults` |
| 运行时装配 | `internal/agent/tool_assembly.go`、`mcp_oauth.go` |
| 工具运行时 | `internal/tools/toolset.go`、`toolset_assemble.go`、`mcp_pool.go`、`mcpobserve/` |
| Wire | `runtime.PersistenceSet.AgentMCP`、MCP health runner |
| 前端 | `web/src/features/mcp/`、`McpServersPage.vue` |

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| MCPServer CRUD + Test | ✅ | `MCPServerService` + 前端对话框 |
| 连通性探测 + SSRF | ✅ | `mcp/probe.Evaluate` |
| 健康定时探活 | ✅ | `mcp/health/runner.go` → `TestMCPServer`（内部 `persistHealth`） |
| 健康元数据 | ✅ | `metadata_json`：`health_status` / `last_health_at` |
| MCP ToolSet 挂载 | ✅ | `buildMCPToolSet` + `timeout_sec` 默认 60s |
| MCPBroker | ✅ | 需 `mcp_broker` 工具键启用且有服务器行时 `buildMCPBrokerFromServers` 挂载 |
| Effective MCP 策略 | ✅ | `mcp:<server_key>` allow/deny |
| OAuth2 / API Key | ✅ | `config_json.auth` + `mcp_oauth.go`；OAuth2 refresh 失败强失败不再 fallback |
| 会话重连可观测 | ✅ | `mcpobserve` + `RecordReconnectMetadata` + 前端 chip |
| AdHoc HTTP 门禁 | ✅ | 服务器 flag + `system_settings.mcp_allow_adhoc_http` |
| 按用户凭据 | ✅ | `mcp_server_user_credential` 表 + API + 前端对话框 |
| MCP 调用统计闭环 | ✅ | `classify` + `mcp_call_count` + `aranea_mcp_invocation_total` |
| 健康持续告警 | ✅ | `mcp/alert` + Monitor 事件 `mcp.health_alert` |
| URL 预检 | ✅ | `POST /v1/mcp-servers/validate` + 表单预检 |
| Probe 策略化 | ✅ | `ProbeStrategy` 接口 + `ConnectivityProbe` / `AuthAwareProbe` + `probe_mode` 配置 |
| Transport 类型化 | ✅ | `config.Transport` 类型 + `UnmarshalJSON` 自动 normalize |
| Defaults 集中 | ✅ | `internal/mcp/defaults.go`：超时/间隔/重连等常量 |
| Health bounded concurrency | ✅ | `maxConcurrentProbes=8` semaphore |
| mcpobserve 精确查询 | ✅ | `GetMCPServerByKey` 替代 O(n) 全表扫 |
| Metadata 并发写隔离 | ✅ | `UpdateMCPServerMetadata` 只写 metadata+status 字段 |
| 工作区隔离（P2-B IDOR） | ✅ | `workspace_id` 列 + `assertMCPServerAccess`（读级）/ `assertMCPServerMutateAccess`（变更级）双守卫；`shared` 派生字段下发 |
| 服务端分页 + 搜索 | ✅ | `ListMCPServersRequest{page,page_size,search}` + `ListPaged`（默认 20，上限 100）；旧版不分页路径保留给内部调用方 |
| 内置共享服务器只读 UI | ✅ | 「内置」徽标 + 编辑/删除/启用开关禁用（tooltip 说明）；测试连接对共享行放行（读级守卫） |
| 手动刷新反馈 | ✅ | `refreshFeedback`：行数 + 最近 `last_health_at` |
| 后端错误文案透出 | ✅ | `axiosHandler` 提取 Kratos 错误信封 `message` 覆盖 axios 通用文案 |
| 软删除感知唯一索引 | ✅ | `idx_mcp_server_server_key_active` / `idx_mcp_credential_unique_active` 部分唯一索引（`WHERE deleted_at=''`），同 key 软删后可重建（R1） |
| 运行时工作区过滤 | ✅ | `EffectiveServersForAgent` 按调用方 workspace 过滤（共享+自有），系统调用方豁免（R2） |
| oauth2_static 守卫 | ✅ | 令牌失效不再回退注入过期 access_token，显式失败提示重配（T1） |
| AuthHeaderName 透传 | ✅ | 用户级凭据注入器与静态 auth 写入同一 header（T2） |
| 连接池关闭安全 | ✅ | 被引用 entry 延迟到最后一次 release 关闭；Assemble 失败 `closeAll()` 清理（T3/T4） |
| refresh token 轮换回写 | ✅ | `PersistRotatedRefreshToken` + `SetMCPRefreshTokenPersister` 钩子（T5） |
| 单次查询守卫 + 凭据审计 | ✅ | `checkMCPServerAccess` 返回 server 复用；IDOR 拒绝 Warn 日志；凭据 upsert/delete 写 Admin Audit；`mcp.server.update` 流程日志（M1-M3） |
| 前端类型与测试 | ✅ | `McpServerRow`/`McpHealthTone` 收紧；`useMcpServerForm`/`useMcpServersPage` 单测 12 例（F3/F4） |

---

## 3. 2026-05-21 文档/代码优化（已完成）

| # | 项 | 说明 |
|---|-----|------|
| O1 | 统一 `config_json` 模型 | `internal/mcp/config` 供 probe / agent 共用 |
| O2 | 元数据 SRP | `internal/mcp/metadata` 供 biz 健康/重连持久化 |
| O3 | 删除死代码 | 移除无引用的 `tools/mcpmount`、`mcp/mount` |
| O4 | 文档三件套对齐 | 需求/设计/开发计划 + README §5.2 + changelog |

---

## 4. 演进方向

| 方向 | 状态 | 说明 |
|------|------|------|
| MCP 统计闭环 | ✅ | `classify` + `mcp_call_count` + Prometheus `aranea_mcp_invocation_total` |
| 按用户凭据 | ✅ | `mcp_server_user_credential` + API + `McpUserCredentialDialog` |
| 探活告警 | ✅ | `mcp/alert` + Monitor 事件 `mcp.health_alert` + 持续错误判定 |
| URL 预检 API | ✅ | `POST /v1/mcp-servers/validate` 复用 probe |
| Probe 策略化 | ✅ | `ProbeStrategy` 接口 + `ConnectivityProbe` / `AuthAwareProbe` |
| Lifecycle FSM | 🟡 | `internal/mcp/lifecycle` 状态机 + `metadata.ApplyHealth` 经 Transition；告警/重连全量 FSM 编排仍可演进 |

---

## 5. 开发阶段

- **Phase 1**（✅）：CRUD、探活、ToolSet/Broker、超时、重连、OAuth 基础
- **Phase 2**（✅）：MCP 调用统计 E2E + Monitor 告警
- **Phase 3**（✅）：按用户凭据 + 密钥加密存储
- **Phase 4**（✅）：URL 预检 API + P1-09 probe auth_required + P1-08 skill ApplyImport
- **Phase 5**（✅）：MCP 子系统架构优化（TPM-P1-10/11/12 + P2 修复 + 测试补全 + defaults 集中）
- **Phase 6**（✅ 大部分完成）：MCP 中长期优化（P2 安全/性能 ✅ + Probe 策略化 ✅ + Review 修复 ✅ + Lifecycle FSM 📋）

---

## 6. 任务清单

| # | 任务 | 优先级 | 阶段 |
|---|------|--------|------|
| 1 | ~~健康定时探活~~ ✅ | P2 | Phase 1 |
| 2 | ~~`timeout_sec` → ConnectionConfig.Timeout~~ ✅ | P2 | Phase 1 |
| 3 | ~~包结构 SRP（config/metadata）~~ ✅ | P2 | Phase 1 |
| 4 | ~~MCP `mcp_call_count` + Prometheus~~ ✅ | P3 | Phase 2 |
| 5 | ~~Monitor：`health_status=error` 持续告警~~ ✅ | P3 | Phase 2 |
| 6 | ~~按用户凭据配置页~~ ✅ | P3 | Phase 3 |
| 7 | ~~`POST /v1/mcp-servers/validate`~~ ✅ | P4 | Phase 3 |
| 8 | ~~probe 401/403 → auth_required~~ ✅ | P1 | Phase 4 |
| 9 | ~~Transport 类型化 + NormalizeTransport 全链统一~~ ✅ | P1 | Phase 5 |
| 10 | ~~probe SSRF CheckRedirect 修复~~ ✅ | P1 | Phase 5 |
| 11 | ~~ToConnectionConfig 统一 + MCPServerConfig 消除双 Config~~ ✅ | P1 | Phase 5 |
| 12 | ~~mcpobserve 统一使用 config.NormalizeTransport~~ ✅ | P1 | Phase 5 |
| 13 | ~~MCP Broker 显式挂载~~ ✅ | P2 | Phase 5 |
| 14 | ~~mcpobserve context.Background() → context.WithoutCancel~~ ✅ | P2 | Phase 5 |
| 15 | ~~alert.MarkHealthAlertEmitted 失败不再静默吞错~~ ✅ | P2 | Phase 5 |
| 16 | ~~补 probe/health/alert 单元测试~~ ✅ | P2 | Phase 5 |
| 17 | ~~Magic numbers 集中到 defaults.go~~ ✅ | P3 | Phase 5 |

---

## 7. 验收标准

- [x] 后台定时探活更新 `metadata_json`
- [x] MCP 工具调用受 `timeout_sec` 约束
- [x] Broker 在有服务器配置时可发现/调用工具
- [x] SSE/Streamable 重连可观测（事件 + metadata）
- [x] OAuth2 Client Credentials / Refresh 可注入 Authorization
- [x] `mcp_call_count` 与 MCP 工具分类一致（`classify` + 指标；生产 E2E 建议抽检）
- [x] `require_user_credentials=true` 时有用户级凭据入口（MCP 列表 → 用户凭据）
- [x] URL 预检 API 可用（`POST /v1/mcp-servers/validate`）
- [x] Probe 策略化：`ConnectivityProbe` / `AuthAwareProbe` + `probe_mode` 配置
- [x] Transport 类型化 + `NormalizeTransport` 全链统一
- [x] Magic numbers 集中到 `defaults.go`
- [x] Health bounded concurrency（semaphore）
- [x] mcpobserve 使用 `GetMCPServerByKey` 精确查询
- [x] OAuth2 refresh 失败强失败不再 fallback
- [x] Metadata 并发写隔离（`UpdateMCPServerMetadata` 只写 metadata+status 字段）

---

## 8. 依赖与风险

- MCP 协议版本演进需跟踪 trpc-agent-go `tool/mcp` 更新。
- OAuth token 缓存在进程内；多副本部署需后续外置 token store。
- ~~探活 HTTP GET 不等同完整 MCP 握手，仅作可达性粗检。~~ 已通过 Probe 策略化支持 `AuthAwareProbe`；`FullHandshakeProbe` 预留但需 trpc-agent-go 框架侧支持 MCP Initialize 探活接口。
- ~~metadata 并发写 last-write-wins 可能丢数据。~~ 已通过 `UpdateMCPServerMetadata` 字段级隔离缓解；根本解为 Lifecycle FSM（D-M3）。

---

## 9. Phase 5 执行进度

| # | 任务 | 状态 | 完成日期 | 备注 |
|---|------|------|---------|------|
| 9 | Transport 类型化 + NormalizeTransport 全链统一 | ✅ | 2026-05-28 | TPM-P1-10：Transport 类型 + UnmarshalJSON 自动 normalize |
| 10 | probe SSRF CheckRedirect 修复 | ✅ | 2026-05-26 | TPM-P1-11：outboundguard.NewClient 已内置 CheckRedirect |
| 11 | ToConnectionConfig 统一 + MCPServerConfig 补全 Env | ✅ | 2026-06-23 | TPM-P1-12：MCPServerConfig.Env 已补；ToConnectionConfig 映射 Env + 框架 ConnectionConfig.Env + createClient 传递（2026-06-23 审计修复全链路） |
| 12 | mcpobserve 统一使用 config.NormalizeTransport | ✅ | 2026-05-28 | TPM-P1-10 残留：删除本地 switch |
| 13 | MCP Broker 显式挂载 | ✅ | 2026-05-28 | 需 mcp_broker 工具键启用才挂载 |
| 14 | mcpobserve context.WithoutCancel | ✅ | 2026-05-28 | TPM-P2-07：保留 trace |
| 15 | alert 失败不再静默吞错 | ✅ | 2026-05-28 | TPM-P2-25：log + metric |
| 16 | 补 config 单元测试 | ✅ | 2026-05-28 | TPM-P2-29 部分：ParseTransport + UnmarshalJSON |
| 17 | Magic numbers 集中 | ✅ | 2026-05-28 | TPM-P3-12：internal/mcp/defaults.go |

---

## 10. Phase 6 待办清单（MCP 中长期优化）

> 来源：[2026-05-26 综合Review](../review/2026-05-26-Tools-Plugin-Skill-MCP-Code-Review.md) 中 MCP 相关 P2/P3/D 项

### 10.1 P2 安全与性能（Wave 2 剩余）

| # | ID | 任务 | 说明 | 优先级 | 状态 |
|---|-----|------|------|--------|------|
| 18 | TPM-P2-26 | ~~mcpobserve 每次 reconnect O(n) 全表扫 server 找 key~~ | 改 `GetMCPServerByKey` 精确查询 | P2 | ✅ 2026-05-28 |
| 19 | TPM-P2-27 | ~~OAuth refresh 失败 fallback 用陈旧 `access_token`~~ | 强失败不再 fallback | P2 | ✅ 2026-05-28 |
| 20 | TPM-P2-28 | ~~metadata row 并发 health + reconnect 写为 last-write-wins~~ | `UpdateMCPServerMetadata` 只写 metadata+status 字段 | P2 | ✅ 2026-05-28 |
| 21 | TPM-P2-29 | ~~probe/health/alert 三个包测试覆盖不足~~ | probe + alert + config 测试已补 | P2 | ✅ 2026-05-28 |
| 22 | TPM-P2-30 | ~~health.probeAll 每 server safego.Go 无 worker pool 上限~~ | semaphore bounded concurrency | P2 | ✅ 2026-05-28 |

### 10.2 P3 维护性

| # | ID | 任务 | 说明 | 优先级 |
|---|-----|------|------|--------|
| 23 | TPM-P3-13 | ~~classify.IsMCPToolInvocation 仅前缀启发式~~ | 评估后保留：`mcp_`+`__` 模式与框架一致，误判风险极低 | P3 | ✅ 2026-05-28 |

### 10.3 架构重设计（中长期）

| # | ID | 任务 | 说明 | 优先级 | 波次 |
|---|-----|------|------|--------|------|
| 24 | TPM-D-M1 | ~~Transport 类型化 + 单一 Codec~~ | ✅ Transport 类型化 + ToConnectionConfig 委托 + 默认超时保持一致 | P1→P2 | ✅ 2026-05-28 |
| 25 | TPM-D-M2 | ~~Probe 策略化（Handshake Strategy）~~ | ✅ ProbeStrategy 接口 + ConnectivityProbe + AuthAwareProbe + ProbeMode 配置 + validateHTTPConfig DRY 修复 + auth_aware 无 auth 路径语义修复 | P2 | ✅ 2026-05-28 |
| 26 | TPM-D-M3 | Health/Reconnect/Alert 统一为 Server Lifecycle FSM | ✅ 落地 `internal/mcp/lifecycle` + ApplyHealth 走 Transition；重连/告警编排可继续收敛 | P2→P3 | Wave 4 🟡 |

### 10.4 依赖与风险

- **P2-27 OAuth token 管理**：✅ 已修复。OAuth2 refresh 失败强失败不再 fallback；但每次 Agent 回合仍重新获取 token（client_credentials），无缓存无主动刷新；多副本部署需外置 token store
- **P2-28 metadata 并发写**：✅ 已缓解。`UpdateMCPServerMetadata` 只写 metadata+status 字段，避免 last-write-wins；FSM 是根本解
- **P2-30 无 worker pool**：✅ 已修复。semaphore bounded concurrency（`maxConcurrentProbes=8`）
- **D-M2 Probe 策略化**：✅ 已完成。ConnectivityProbe（默认，仅网络连通性）+ AuthAwareProbe（带 OAuth/API Key 探活）。full_handshake 模式预留但未实现，需 trpc-agent-go 框架侧支持 MCP Initialize 探活接口
- **D-M3 Lifecycle FSM**：📋 待规划。是中长期最大架构改动，需要独立 design 文档

---

## 11. 2026-06-23 MCP 加载链路审计修复

> 来源：MCP 模块功能代码逻辑深入检查，发现三个问题并修复。

### 11.1 修复清单

| # | 问题 | 严重度 | 修复方案 | 状态 |
|---|------|--------|---------|------|
| 27 | Env 字段传递链路断裂：`config_json.env` → `MCPServerConfig.Env` 已解析，但 `ToConnectionConfig()` 丢弃 + 框架 `ConnectionConfig` 无 `Env` 字段 + `createClient` 未传 `StdioServerParameters.Env` | 高（BUG） | 框架 `ConnectionConfig` 增 `Env` 字段 + `createClient`/broker `createClient` 传 `Env` + `normalizeConnectionConfig` 保留 `Env` + 应用层 `ToConnectionConfig` 映射 `Env` | ✅ 2026-06-23 |
| 28 | MCP 服务器不可达时静默失败：`Init()` 从未被调用，`Tools()` 失败仅框架 log 记录后返回空，Agent 无 MCP 工具无告警 | 中 | `assembleMCPTools` 创建 ToolSet 后调用 `Init(ctx)`，失败时通过 `loggateway` 记录结构化告警（含 server 名），不中断组装保持弹性降级 | ✅ 2026-06-23 |
| 29 | 缓存淘汰时 ToolSet 未关闭：`BuildCache.evict`/`Close` 不调用 `ToolSet.Close()`，MCP 会话/stdio 子进程/HTTP 连接泄漏 | 中（资源泄漏） | `buildCacheEntry` 增 `toolSets` 字段 + `BuildTRPCLLMAgent` 拆分为 `buildTRPCLLMAgentWithToolSets` 返回 ToolSets + `put`/`evict`/`Close` 关闭 ToolSets | ✅ 2026-06-23 |

### 11.2 改动文件清单

**框架层（pkg/trpc-agent-go/）**：
- `tool/mcp/config.go`：`ConnectionConfig` 增加 `Env map[string]string` 字段
- `tool/mcp/toolset.go`：`createClient` stdio 分支传 `Env` 给 `StdioServerParameters`
- `tool/mcpbroker/client.go`：broker `createClient` stdio 分支传 `Env`
- `tool/mcpbroker/config.go`：`normalizeConnectionConfig` 保留 `Env`（`cloneStringMap`）

**应用层（internal/）**：
- `internal/tools/toolset.go`：`ToConnectionConfig` 映射 `Env` 字段
- `internal/tools/toolset_assemble.go`：`assembleMCPTools` 调用 `Init(ctx)` + loggateway 告警
- `internal/agent/trpc_build.go`：拆分 `buildTRPCLLMAgentWithToolSets` 返回 `(Agent, []ToolSet, error)`
- `internal/agent/cache.go`：`buildCacheEntry` 增 `toolSets` 字段 + `put`/`evict`/`Close` 关闭 ToolSets + `closeToolSets` 辅助函数

**测试**：
- `internal/tools/mcp_verify_issue_test.go`：Env 传递回归测试

### 11.3 验收标准

- [x] `TestMCPEnvPropagation_Regression` 验证 `config_json.env` → `ConnectionConfig.Env` 全链路传递
- [x] `TestBuildCache*` 全部 9 个缓存测试通过
- [x] 框架 `tool/mcp/...` + `tool/mcpbroker/...` 测试全部通过
- [x] `internal/tools/...` 全量测试通过
- [x] Init 失败不中断组装（弹性降级语义保持）
- [x] 缓存淘汰/关闭时 ToolSet 被关闭（资源释放）

---

## 12. 2026-08-11 共享服务器测试 404 + 列表分页/刷新反馈修复

> 来源：用户反馈 Playwright（内置共享服务器）点击「测试连接」404、点击「刷新」无效果无反馈、状态不更新。

### 12.1 根因分析

| # | 现象 | 根因 |
|---|------|------|
| 30 | 内置服务器「测试连接」404 | `TestMCPServer` 误用变更级 IDOR 守卫，对 `workspace_id=""` 的共享服务器 fail-closed |
| 31 | 刷新「无效果」 | 健康数据由服务端定时探活更新，手动刷新只重拉列表且无成功反馈，用户无法感知数据新鲜度 |
| 32 | 分页/搜索失效 | rpc 声明 `ListMCPServers(google.protobuf.Empty)`，生成客户端丢弃 page/page_size/search，请求以无 query 的裸 GET 发出 |
| 33 | 错误文案无信息量 | axios 通用文案 "Request failed with status code 404" 掩盖了 Kratos 错误信封中的后端 message |

### 12.2 修复清单

| # | 修复 | 状态 |
|---|------|------|
| 30 | `TestMCPServer` 改用读级守卫 `assertMCPServerAccess`（探测仅刷新系统健康簿记，与 health runner 语义一致） | ✅ |
| 31 | 手动刷新成功后 `Notify` 反馈行数 + 最近 `last_health_at`（`refreshFeedback`） | ✅ |
| 32 | proto 改 `ListMCPServersRequest{page,page_size,search}`；service 优先取请求参数、HTTP query fallback 兼容旧客户端 | ✅ |
| 33 | `axiosHandler.humanizeAxiosError` 提取 Kratos 错误信封 `message` | ✅ |
| 34 | `MCPServer.shared` 派生字段下发；前端内置徽标 + 编辑/删除/开关禁用（tooltip 说明） | ✅ |

### 12.3 改动文件清单

**后端**：
- `api/kratos/mcp_server/v1/mcp_server.proto`：`ListMCPServersRequest` + `MCPServer.shared` + 响应分页字段
- `internal/service/mcp_server.go`：读级/变更级双守卫；`ListMCPServers` 分页搜索；`toProtoMCP` 派生 `shared`
- `internal/biz/mcp_server.go`：`WorkspaceID`、`MCPListQuery`、`ListPaged`
- `internal/data/ent/schema/platform_mcp_server.go`：`workspace_id` 列 + 索引

**前端**：
- `web/src/features/mcp/api.ts`：`shared` 映射 + `listMcpServersPaged`
- `web/src/features/mcp/useMcpServersPage.ts`：`loadRows(manual)` + `refreshFeedback`
- `web/src/components/mcp/McpServersTable.vue`：内置徽标 + 共享行禁用编辑/删除/开关
- `web/src/services/axiosHandler.ts`：后端错误 message 提取

**测试**：
- `internal/service/mcp_server_test.go`：`TestMCPServerService_TestMCPServer_SharedServerAllowedForTenant`（租户探测共享服务器回归）
- `web/src/features/mcp/__tests__/mcpServerListQuery.spec.ts`：生成客户端序列化 page/page_size/search 回归

### 12.4 验收标准

- [x] `go build ./...` + MCP 相关单测（service/biz/data）通过
- [x] 前端 `eslint` 0 错误、`vitest` 13/13 MCP 测试通过、`quasar build` 成功
- [x] 租户调用 `POST /v1/mcp-servers/{共享服务器id}/test` 不再 404
- [x] 列表请求携带 `page`/`page_size`/`search` query 参数
- [x] 手动刷新后展示行数 + 最近健康检测时间
- [x] HTTP 错误提示展示后端 message（如 "mcp server not found"）

---

## 13. 2026-08-14 深入评审整改

> 来源：MCP 管理模块整体深入评审（业务逻辑/代码逻辑/架构设计/死代码），方案经用户评审确认（R2 收紧、T5 回写、全部 🟡 及以上实施）。

### 13.1 修复清单

| # | 问题 | 严重度 | 修复方案 | 状态 |
|---|------|--------|---------|------|
| R1 | `mcp_server.server_key` 列级 UNIQUE 与凭据复合唯一索引含软删除墓碑行，同 key 软删后重建报 23505 | 🔴 | 改为部分唯一索引（`WHERE deleted_at=''`）：Ent Schema 声明 + DDL 迁移 `20261209_mcp_partial_unique_index` 清理存量库 + PG 集成测试 | ✅ |
| M1 | service 守卫与业务重复 `Get`（每 RPC 两次 DB 读） | 🟡 | `checkMCPServerAccess` 返回已读 server，`Get`/`Update`/`Delete` 复用 | ✅ |
| M2 | IDOR 拒绝静默（TECH-DEBT 注释遗留） | 🟡 | 进程日志 Warn（`mcp.server.access_denied`），对外仍 NotFound | ✅ |
| M3 | 用户凭据 upsert/delete 未写 Admin Audit | 🟡 | `recordAudit`（`AuditVerbCredentials`） | ✅ |
| M4 | 死代码：`effectiveToolsAllowsMCP` 无引用包装、`ValidateConfig` 废弃 `enabled` 形参、`toolset.go` 已废弃 `FilesystemDirWithDir/FromContext` | 🟡 | 删除/签名收紧 | ✅ |
| M5 | `UpdateMCPServer` 缺流程日志（仅有 add/remove） | 🟡 | 发射 `mcp.server.update`（成功/失败双轨），登记步骤注册表 + 52-flow-logger §5.1 | ✅ |
| M6 | `DeleteMCPServer` 审计摘要 best-effort Get 失败即丢 key/name | 🟡 | 随 M1 单次查询一并解决（守卫返回值直接供审计） | ✅ |
| R2 | `EffectiveServersForAgent` 不按 workspace 过滤，租户 Agent 可挂载他租户私有 MCP 服务器 | 🔴 | 非系统调用方按 `workspace.IDFromContext` 过滤（共享+自有）；系统调用方豁免 | ✅ |
| T1 | `oauth2_static` 令牌失效时回退注入同一过期 `access_token`，401 被掩盖 | 🟡 | 守卫：失败留空 key 不注入，Warn 日志提示重配 | ✅ |
| T2 | 用户级凭据注入器未透传 `auth.header_name`，与静态 auth 路径 header 不一致 | 🟡 | `MCPServerConfig.AuthHeaderName` 全链透传至 `ResolveUserAuthHeaders` | ✅ |
| T3 | `mcp_pool.Close` 与在用 entry 竞态：被引用 ToolSet 被立即关闭（use-after-close）；Acquire 连接中途遇 Close 双重关闭 | 🟠 | 被引用 entry 标记 `closing` 延迟到最后一次 release 关闭；中途遇 Close 的 ToolSet 直接交调用方 | ✅ |
| T4 | `tools.Assemble` 任一 phase 失败时已装配 ToolSet 泄漏（池化连接占用池引用） | 🟠 | 错误路径 `closeAll()` 统一关闭/归还 | ✅ |
| T5 | OAuth2 provider 轮换 refresh token 仅落内存缓存，进程重启复活已吊销旧 token | 🟠 | `PersistRotatedRefreshToken`（解密→patch→重加密→更新）+ `SetMCPRefreshTokenPersister` 钩子（service 装配，失败非致命） | ✅ |
| F1 | 前端注释问题（SFC 根级多行注释、遗留修复记录注释） | 🟡 | 注释格式化/清理 | ✅ |
| F2 | `mcpServerTableUi.ts` 操作列硬编码 `100px` | 🟡 | 复用 `REGISTRY_COL_W.actionsWide` token | ✅ |
| F3 | API/store 返回 `PlatformResource` 泛型需断言；`healthTone` 返回 `string` | 🟡 | 收紧 `McpServerRow` + 新增 `McpHealthTone` 联合类型，消除断言 | ✅ |
| F4 | MCP 前端关键逻辑缺测试 | 🟡 | `useMcpServerForm`（buildPayload 6 例）+ `useMcpServersPage`（healthTone/Tooltip 6 例） | ✅ |
| RV-01 | `PersistRotatedRefreshToken` 全行写与健康探活字段级写存在 last-write-wins 窗口（整体评审发现） | 🟡 | 新增 `MCPServerWriter.UpdateMCPServerConfigJSON` 字段级写（仅 `config_json`+`updated_at`），usecase 改走该方法；biz 测试断言禁止全行写 + 元数据不被覆盖 | ✅ |

### 13.2 改动文件清单

**后端**：
- `internal/data/ent/schema/platform_mcp_server.go` / `platform_mcp_user_credential.go`：部分唯一索引声明（R1）
- `internal/data/ent/migrate/schema.go`：生成物（R1）
- `internal/data/ddl_migration_registry.go` + `internal/data/sql/migrations/20261209_mcp_partial_unique_index.sql`：存量库迁移（R1）
- `internal/service/mcp_server.go`：单次查询守卫、IDOR 日志、凭据审计、`mcp.server.update` 流程日志、T5 钩子装配（M1-M3/M5/M6/T5）
- `internal/biz/mcp_server.go`：`ValidateConfig` 签名收紧、`PersistRotatedRefreshToken`（M4/T5）
- `internal/biz/agent_mcp_effective.go`：workspace 过滤 + 死代码删除（R2/M4）
- `internal/data/mcp_server.go`：`UpdateMCPServerConfigJSON` 字段级写（RV-01）
- `internal/agent/tool_assembly.go`：oauth2_static 守卫、AuthHeaderName 透传（T1/T2）
- `internal/agent/mcp_oauth.go`：`ResolveMCPAuthToken` 增 serverKey 形参 + 轮换回写钩子（T5）
- `internal/tools/mcp_pool.go`：closing 延迟关闭语义（T3）
- `internal/tools/toolset.go` / `toolset_assemble.go`：Assemble 错误路径 `closeAll()`（T4）+ 死代码删除（M4）
- `internal/event/flow_log.go`：`mcp.server.update` 步骤登记（M5）

**前端**：
- `web/src/features/mcp/`：`types.ts`（McpServerRow/McpHealthTone）、`api.ts`、`useMcpServersPage.ts`、`useMcpUserCredentialDialog.ts`、`McpUserCredentialDialog.vue`（F1/F3）
- `web/src/components/mcp/`：`mcpServerTableUi.ts`（F2）、`McpServersTable.vue`（F3）
- `web/src/stores/mcp/index.ts` + `stores/__tests__/mcp.store.spec.ts`（F3/F4）
- `web/src/features/mcp/__tests__/useMcpServerForm.spec.ts` / `useMcpServersPage.spec.ts`（F4，新增）

**测试（后端）**：`mcp_server_test.go`（biz/service）、`mcp_oauth_test.go`、`mcp_pool_test.go` 等配套更新

### 13.3 验收标准

- [x] 后端 `go build ./cmd/... ./internal/... ./api/... ./pkg/...` + MCP 相关包 `go test` 通过
- [x] PG 集成测试验证部分唯一索引（同 key 软删后重建成功）
- [x] 前端门禁：vitest 25/25（MCP 相关 4 文件）、eslint 0 错误、check-i18n 通过
- [x] 同 key 软删重建不再报 23505
- [x] 租户 Agent `EffectiveServersForAgent` 不再返回他租户私有服务器
- [x] `mcp.server.update` 流程日志在 Monitor「流程日志」可见（有中文标题）
- [x] token 回写走字段级写，biz 测试断言元数据/状态不被覆盖（RV-01）
