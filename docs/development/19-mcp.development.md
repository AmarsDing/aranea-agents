# MCP 协议 — 开发计划

> **版本**：2026-06-06 | **状态**：🟢 Phase 6 大部分已落地，仅 Lifecycle FSM 待规划
> **需求**：[19 mcp.md](./19%20mcp.md) · **设计**：[19 mcp.design.md](./19%20mcp.design.md)
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
| 工具运行时 | `internal/tools/toolset.go`、`mcpobserve/` |
| Wire | `runtime.PersistenceSet.AgentMCP`、MCP health runner |
| 前端 | `web/src/features/mcp/`、`McpServersPage.vue` |

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| MCPServer CRUD + Test | ✅ | `MCPServerService` + 前端对话框 |
| 连通性探测 + SSRF | ✅ | `mcp/probe.Evaluate` |
| 健康定时探活 | ✅ | `mcp/health/runner.go` → `PersistHealth` |
| 健康元数据 | ✅ | `metadata_json`：`health_status` / `last_health_at` |
| MCP ToolSet 挂载 | ✅ | `buildMCPToolSet` + `timeout_sec` 默认 60s |
| MCPBroker | ✅ | 有服务器行时 `buildMCPBrokerFromServers` 自动挂载 |
| Effective MCP 策略 | ✅ | `mcp:<server_key>` allow/deny |
| OAuth2 / API Key | ✅ | `config_json.auth` + `mcp_oauth.go`；OAuth2 refresh 失败强失败不再 fallback |
| 会话重连可观测 | ✅ | `mcpobserve` + `RecordReconnectMetadata` + 前端 chip |
| AdHoc HTTP 门禁 | ✅ | 服务器 flag + `system_settings.mcp_allow_adhoc_http` |
| 按用户凭据 | ✅ | `platform_mcp_user_credential` + API + 前端对话框 |
| MCP 调用统计闭环 | ✅ | `classify` + `mcp_call_count` + `aranea_mcp_invocation_total` |
| 健康持续告警 | ✅ | `mcp/alert` + Monitor 事件 `mcp.health_alert` |
| URL 预检 | ✅ | `POST /v1/mcp-servers/validate` + 表单预检 |
| Probe 策略化 | ✅ | `ProbeStrategy` 接口 + `ConnectivityProbe` / `AuthAwareProbe` + `probe_mode` 配置 |
| Transport 类型化 | ✅ | `config.Transport` 类型 + `UnmarshalJSON` 自动 normalize |
| Defaults 集中 | ✅ | `internal/mcp/defaults.go`：超时/间隔/重连等常量 |
| Health bounded concurrency | ✅ | `maxConcurrentProbes=8` semaphore |
| mcpobserve 精确查询 | ✅ | `GetMCPServerByKey` 替代 O(n) 全表扫 |
| Metadata 并发写隔离 | ✅ | `UpdateMCPServerMetadata` 只写 metadata+status 字段 |

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
| 按用户凭据 | ✅ | `platform_mcp_user_credential` + API + `McpUserCredentialDialog` |
| 探活告警 | ✅ | `mcp/alert` + Monitor 事件 `mcp.health_alert` + 持续错误判定 |
| URL 预检 API | ✅ | `POST /v1/mcp-servers/validate` 复用 probe |
| Probe 策略化 | ✅ | `ProbeStrategy` 接口 + `ConnectivityProbe` / `AuthAwareProbe` |
| Lifecycle FSM | 📋 | 中长期：消除 metadata 并发写 + 告警逻辑散落，需独立 design 文档 |

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
| 11 | ToConnectionConfig 统一 + MCPServerConfig 补全 Env | ✅ | 2026-05-28 | TPM-P1-12：ToConnectionConfig 补 Env 字段 |
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
| 26 | TPM-D-M3 | Health/Reconnect/Alert 统一为 Server Lifecycle FSM | 消除 metadata 并发写 + 告警逻辑散落 | P2→P3 | Wave 4 |

### 10.4 依赖与风险

- **P2-27 OAuth token 管理**：✅ 已修复。OAuth2 refresh 失败强失败不再 fallback；但每次 Agent 回合仍重新获取 token（client_credentials），无缓存无主动刷新；多副本部署需外置 token store
- **P2-28 metadata 并发写**：✅ 已缓解。`UpdateMCPServerMetadata` 只写 metadata+status 字段，避免 last-write-wins；FSM 是根本解
- **P2-30 无 worker pool**：✅ 已修复。semaphore bounded concurrency（`maxConcurrentProbes=8`）
- **D-M2 Probe 策略化**：✅ 已完成。ConnectivityProbe（默认，仅网络连通性）+ AuthAwareProbe（带 OAuth/API Key 探活）。full_handshake 模式预留但未实现，需 trpc-agent-go 框架侧支持 MCP Initialize 探活接口
- **D-M3 Lifecycle FSM**：📋 待规划。是中长期最大架构改动，需要独立 design 文档
