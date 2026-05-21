# 26 A2A Protocol Review

> **评分**：81 / 100 | **风险等级**：P1  
> **文档**：[26 a2a-protocol.md](../需求/26%20a2a-protocol.md) · [26 a2a-protocol.design.md](../需求/26%20a2a-protocol.design.md) · [26-a2a-development.md](../需求/26-a2a-development.md)  
> **代码锚点**：`internal/a2a/` · `internal/service/a2a.go` · `internal/service/a2a_endpoint.go` · `internal/biz/a2a*.go`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 17 | 20 | Phase 1–3.5 ✅（Invoke/Endpoint/Proxy/联邦/Graph/流式）；Phase 4（网关 Cron/速率限制）待补 |
| 架构一致性 | 22 | 25 | SRP 分层说明文档化 ✅；公开 Endpoint 用 HTTP/SSE（非 WS，符合 A2A 协议）✅；`call_agent` 工具经 tools 层 ✅ |
| 后端实现质量 | 17 | 20 | 远程注册 + mTLS + Invoke 工作区策略 + 联邦 Gateway ✅；网关健康 Cron 未实现 |
| 前端实现质量 | 13 | 15 | A2A 页四 Tab + 远程注册面板 + mapper 单测 ✅；Invoke 测试 Tab ✅ |
| 测试与验证 | 6 | 10 | `features/a2a/__tests__/mappers.spec.ts` ✅；后端 Invoke 路径集成测试待补 |
| 文档一致性 | 6 | 10 | 三件套 2026-05-21 DocSync 完成；SRP 分层 §2 说明已更新 |

---

## Phase 状态

| Phase | 内容 | 状态 |
|-------|------|------|
| 1 | Invoke 派发、call_agent 工具 | ✅ |
| 2 | Admin 鉴权、管理页、`/a2a` 路由 | ✅ |
| 3 | A2A Server 暴露、远程注册、mTLS、Invoke 工作区策略 | ✅ |
| 3.5 | 联邦 Gateway、Graph metadata、远程 Invoke、流式 | ✅ |
| 4 | 网关健康 Cron + 速率限制 | ❌ |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| `call_agent` 工具（tools 层）| ✅ |
| A2A Invoke 派发 | ✅ |
| Agent Endpoint 开关（LLM Agent 暴露）| ✅ |
| A2A Proxy（远程代理 Agent）| ✅ |
| 公开 Endpoint（`/v1/a2a/public/{agent_id}`）| ✅ SSE 流式 |
| 远程 Agent 注册（api_key/bearer/mTLS）| ✅ |
| 联邦网关（`GET /v1/a2a/gateway/discover`）| ✅ |
| Graph metadata（A2A discover 包含 graph 信息）| ✅ |
| A2A 审计记录 | ✅ |
| A2A Invoke 测试 Tab | ✅ |
| 网关健康 Cron | ❌ Phase 4 |
| 速率限制 | ❌ Phase 4 |

---

## 协议兼容性说明

A2A 公开 Endpoint（`/v1/a2a/public/{agent_id}`）使用 **HTTP/SSE**，**非 WebSocket**。这与内部 Chat/Team/Monitor 主通道（`/v1/ws`）不同，符合 A2A 协议规范。前端 `A2APage` 正确标注了此区别。

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| A2A-P1-01 | 网关健康 Cron 未实现：外部远程 Agent 断线后网关不知晓 | 实现 Phase 4 健康 Cron |
| A2A-P1-02 | 后端 Invoke 路径（含 mTLS）无自动化测试 | 补 Invoke 集成测试 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| A2A-P2-01 | 速率限制未实现 | 规划 Phase 4 速率限制（per-Agent 或 per-caller）|
| A2A-P2-02 | `check_health` 参数在联邦发现时超时行为需文档化 | 补充 `check_health` 超时文档 |

---

## 建议优化路径

1. 实现网关健康 Cron（Phase 4，P1）。
2. 补 Invoke 集成测试（P1）。
3. 规划速率限制（Phase 4，P2）。
