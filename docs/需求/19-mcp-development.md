# MCP 协议 — 开发计划

> **版本**：2026-05-21 | **状态**：🟢 P3–P4 已落地
> **需求**：[19 mcp.md](./19%20mcp.md) · **设计**：[19 mcp.design.md](./19%20mcp.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：I4-MCP-01 / I5-MCP-01 ✅

---

## 1. 模块定位

MCP（Model Context Protocol）集成：平台注册外部 MCP 服务器，Agent 通过 trpc `MCPToolSet` / `MCPBroker` 挂载并调用工具。

**代码锚点**：

| 层次 | 路径 |
|------|------|
| API | `api/kratos/mcp_server/v1/` |
| Service | `internal/service/mcp_server.go` |
| Biz | `internal/biz/mcp_server.go`、`agent_mcp_effective.go` |
| Data | `internal/data/mcp_server.go`、`ent/schema/platform_mcp_server.go` |
| MCP 子系统 | `internal/mcp/config`、`probe`、`metadata`、`health`、`alert`、`classify` |
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
| OAuth2 / API Key | 🟡 | `config_json.auth` + `mcp_oauth.go`；平台级 auth 在 config |
| 会话重连可观测 | ✅ | `mcpobserve` + `RecordReconnectMetadata` + 前端 chip |
| AdHoc HTTP 门禁 | ✅ | 服务器 flag + `system_settings.mcp_allow_adhoc_http` |
| 按用户凭据 | ✅ | `platform_mcp_user_credential` + API + 前端对话框 |
| MCP 调用统计闭环 | ✅ | `classify` + `mcp_call_count` + `aranea_mcp_invocation_total` |
| 健康持续告警 | ✅ | `mcp/alert` + Monitor 事件 `mcp.health_alert` |
| URL 预检 | ✅ | `POST /v1/mcp-servers/validate` + 表单预检 |

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

| 方向 | 现状 | 建议 |
|------|------|------|
| MCP 统计闭环 | 分类与 DB 字段已有 | Runner 回合结束校验 `mcp_call_count` 与 Prometheus |
| 按用户凭据 | 配置开关无 UI | 参考 Channel 凭据表 + Agent 用户作用域 |
| 探活告警 | 仅 metadata + 指标 | Monitor 规则：`health_status=error` 超 N 分钟 |
| URL 预检 API | 未实现 | 可选 `POST /v1/mcp-servers/validate` 复用 probe |

---

## 5. 开发阶段

- **Phase 1**（✅）：CRUD、探活、ToolSet/Broker、超时、重连、OAuth 基础
- **Phase 2**：MCP 调用统计 E2E + Monitor 告警
- **Phase 3**：按用户凭据 + 密钥加密存储

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

---

## 7. 验收标准

- [x] 后台定时探活更新 `metadata_json`
- [x] MCP 工具调用受 `timeout_sec` 约束
- [x] Broker 在有服务器配置时可发现/调用工具
- [x] SSE/Streamable 重连可观测（事件 + metadata）
- [x] OAuth2 Client Credentials / Refresh 可注入 Authorization
- [x] `mcp_call_count` 与 MCP 工具分类一致（`classify` + 指标；生产 E2E 建议抽检）
- [x] `require_user_credentials=true` 时有用户级凭据入口（MCP 列表 → 用户凭据）

---

## 8. 依赖与风险

- MCP 协议版本演进需跟踪 trpc-agent-go `tool/mcp` 更新。
- OAuth token 缓存在进程内；多副本部署需后续外置 token store。
- 探活 HTTP GET 不等同完整 MCP 握手，仅作可达性粗检。
