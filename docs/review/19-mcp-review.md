# 19 MCP Review

> **评分**：80 / 100 | **风险等级**：P1  
> **文档**：[19 mcp.md](../需求/19%20mcp.md) · [19 mcp.design.md](../需求/19%20mcp.design.md) · [19-mcp-development.md](../需求/19-mcp-development.md)  
> **代码锚点**：`internal/mcp/` · `internal/service/mcp_server.go` · `internal/biz/mcp_server.go` · `web/src/pages/McpServersPage.vue`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 16 | 20 | CRUD + 探活 + ToolSet/Broker + OAuth + 重连 ✅；统计闭环 🟡 |
| 架构一致性 | 22 | 25 | `internal/mcp` 平台配置层与工具运行时层（`internal/tools/mcp_production.go`）分离 ✅；OAuth 经 `internal/agent/mcp_oauth.go` ✅ |
| 后端实现质量 | 17 | 20 | `ReconnectObserver` + `session_reconnect_max` ✅；`RecordReconnectMetadata` 持久化 ✅；mcp_call_count 同步 ✅ |
| 前端实现质量 | 13 | 15 | `McpServerItem` + 重连状态 chip + `McpServerFormDialog` ✅；统计面板 🟡 |
| 测试与验证 | 6 | 10 | `config_test`、`classify_test`、`metadata_test` ✅；health runner 测试轻 |
| 文档一致性 | 6 | 10 | 三件套对齐 + P3-P4 changelog 已同步；AdHoc HTTP 设置文档化 ✅ |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| MCP Server CRUD | ✅ |
| 传输配置（stdio/sse/http）| ✅ |
| OAuth2 认证 | ✅ |
| 超时（默认 60s）| ✅ |
| `ReconnectObserver` → `mcp.session.reconnect` | ✅ I4 |
| `session_reconnect_max` 配置 | ✅ I4 |
| `RecordReconnectMetadata` 持久化 | ✅ I5 |
| 重连状态 chip（前端 + Monitor 事件联动）| ✅ |
| `mcp_call_count` session 计数同步 | ✅ |
| MCP Broker 自动挂载 | ✅ M4 |
| 用户级凭据（`mcp_user_credential`）| ✅ |
| AdHoc HTTP 系统设置开关 | ✅ |
| `ToolSet` 分组 | ✅ |
| 统计（per-tool 调用次数、延迟）| 🟡 统计闭环待补 |
| Alert 规则（探活失败告警）| 🟡 |

---

## 架构分层

```
internal/mcp/
    ├─ config/   — MCP Server 平台配置
    ├─ classify/ — 工具分类
    ├─ health/   — 探活 runner
    ├─ metadata/ — 工具元数据
    └─ alert/    — 探活告警
        ↓
internal/tools/mcp_production.go — 运行时工具连接
internal/agent/mcp_oauth.go — OAuth 桥接
```

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| MCP-P1-01 | 统计闭环待补：per-tool 调用次数、延迟分布在监控页不可见 | 补充统计维度并在 Monitor 页展示 |
| MCP-P1-02 | health/runner 测试轻量；探活失败场景（超时/认证失败）测试缺失 | 补探活失败单测 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| MCP-P2-01 | 动态 MCP 挂载工具名未入 catalog，仅计 `mcp_call`（非工具级别）| 规划 catalog 动态同步 |
| MCP-P2-02 | Alert 规则（探活失败 N 次后告警）前端 UI 未实现 | 规划 MCP Alert 配置面板 |

---

## 建议优化路径

1. 补充 MCP 统计维度（per-tool 调用次数、延迟）。
2. 补探活失败单测。
3. 规划 MCP Alert 配置面板。
4. 实现 catalog 动态同步（动态挂载工具）。
