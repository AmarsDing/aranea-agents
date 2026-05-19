# A2A 协议 — 开发计划

> **版本**：2026-05-19 | **状态**：⚠️ 基础设施已通，call_agent 运行时未打通
> **需求**：[26 a2a-protocol.md](./26%20a2a-protocol.md) · **设计**：[26 a2a-protocol.design.md](./26%20a2a-protocol.design.md)

---

## 1. 模块定位

A2A（Agent-to-Agent）协议：支持 Agent 之间的结构化通信和协作，包括 AgentCard 管理、call_agent 工具调用、远程 Agent 发现与通信。

**代码锚点**：
- `api/kratos/a2a/v1/` — Proto 定义 + HTTP 映射
- `internal/biz/a2a.go` — 领域模型 + A2ARepo + A2AUsecase
- `internal/data/a2a.go` — SQLite 持久化
- `internal/service/a2a.go` — Kratos 服务适配器
- `internal/a2a/tool.go` — call_agent 工具 + Context 辅助
- `internal/tools/trpc/toolsets.go` — call_agent 条件注入
- `internal/agent/trpc_build.go` — Agent 构建链
- `pkg/trpc-agent-go/a2a/` — trpc-agent-go A2A 框架

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Proto 定义 | ✅ | `a2a.proto`：Discover, Invoke, UpdateAgentCard, GetAgentCard, ListAudit |
| Biz 领域模型 | ✅ | A2ACapability, A2AAgentCard, A2AInvocation, A2AAuditEntry, A2ARepo, A2AUsecase |
| Data 持久化 | ✅ | SQLite 三表 + EnsureA2ASchema |
| Service 层 | ✅ | 5 个 RPC + Prometheus 指标 |
| call_agent 工具定义 | ✅ | `internal/a2a/tool.go`：NewCallAgentTool + Card 验证 + 审计 |
| call_agent 条件注入 | ✅ | `toolsets.go`：`cfg.CallAgent` 时注入 |
| Wire 注入 | ✅ | data/biz/service ProviderSet 均已注册 |
| HTTP/gRPC 注册 | ✅ | `server/http.go` + `grpc.go` |
| 前端 API + Store | ✅ | `features/a2a/` + `stores/a2a/` |
| Team A2A 配置 | ✅ | `TeamEditorDialog.vue` A2A 协议区 |
| **Context 注入** | ❌ | `WithA2AUsecase`/`WithCallerAgentID`/`WithInvoker` 未在 service 层调用 |
| **Invoke 实际派发** | ❌ | `A2AService.Invoke` 仅记录 pending，未执行目标 Agent |
| **跨工作区校验** | ❌ | Invoke 未校验 caller/callee 同工作区 |
| 远程 Agent 发现 | ❌ | 无 Agent 注册中心，无 URL 发现 |
| A2A Server | ❌ | 未将本地 Agent 暴露为 A2A 服务 |
| A2A Agent | ❌ | 未使用 trpc-agent-go a2aagent |
| 消息转换 | ❌ | trpc Event ↔ A2A Message 未实现 |
| 流式通信 | ❌ | A2A SSE 未实现 |
| Graph 恢复 | ❌ | A2A + Graph 集成未实现 |
| 前端管理页面 | ✅ | A2APage.vue（Discover/Audit/Invoke UI）；路由 /a2a 已注册；eatures/a2a/ api/types/mapper + stores/a2a/ 已实现 |

---

## 3. 差距与优先级

| # | 差距 | 优先级 | 说明 |
|---|------|--------|------|
| 1 | Context 注入缺失 | **P0** | call_agent 工具运行时报 "invoker not configured"，这是 A2A 的核心阻断问题 |
| 2 | Invoke 端点为 stub | **P0** | POST /v1/a2a/invoke 仅记录 pending，未实际执行目标 Agent |
| 3 | 跨工作区校验缺失 | **P0** | 安全红线：必须阻止跨工作区调用 |
| ~~4~~ | ~~前端管理页面缺失~~ | ~~**P1**~~ | ✅ A2APage.vue 已实现（Discover/Audit/Invoke UI）；路由 /a2a 已注册 |
| 5 | 远程 Agent 发现 | **P1** | 无法发现和调用远程 A2A Agent |
| 6 | A2A Server | **P1** | 无法将本地 Agent 暴露为 A2A 服务 |
| 7 | 消息转换 | **P2** | trpc Event ↔ A2A Message 双向转换 |
| 8 | 流式通信 | **P2** | A2A SSE 流式响应 |
| 9 | Graph 恢复 | **P2** | A2A 任务与 Graph 中断/恢复 |
| 10 | A2A 网关注册中心 | **P3** | 集中管理 Agent 注册和发现 |

---

## 4. 开发阶段

### Phase 1：call_agent 打通（P0）

目标：使 `call_agent` 工具在运行时可用，Agent 可通过工具调用同工作区其他 Agent。

| # | 任务 | 涉及文件 | 验收标准 |
|---|------|----------|----------|
| 1.1 | TRPCBuilderDeps 增加 A2AUsecase | `internal/agent/trpc_build.go` | deps 可访问 A2AUsecase |
| 1.2 | 实现 invokerFunc | `internal/service/trpc_turn.go` 或新建 `internal/a2a/invoker.go` | invokerFunc 可执行目标 Agent 并返回结果 |
| 1.3 | 注入 A2A 上下文 | `internal/service/trpc_turn.go` | 调用 WithA2AUsecase + WithCallerAgentID + WithInvoker |
| 1.4 | Invoke 端点实际派发 | `internal/service/a2a.go` | POST /v1/a2a/invoke 可执行目标 Agent |
| 1.5 | 跨工作区校验 | `internal/service/a2a.go` | 跨工作区调用返回 403 |
| 1.6 | 端到端测试 | `internal/biz/s6_coverage_test.go` | Agent A call_agent Agent B 成功 |

### Phase 2：前端管理页面（P1）

目标：用户可通过 UI 管理 A2A AgentCard 和审计日志。

| # | 任务 | 涉及文件 | 验收标准 |
|---|------|----------|----------|
| 2.1 | A2AAgentCardPage.vue | `web/src/features/a2a/` | AgentCard 列表 + 启用/禁用 + 能力编辑 |
| 2.2 | A2AAuditPage.vue | `web/src/features/a2a/` | 审计日志浏览 |
| 2.3 | 路由注册 | `web/src/router/` | A2A 页面可导航 |

### Phase 3：跨实例 A2A（P1-P2）

目标：支持与远程 A2A Agent 通信，将本地 Agent 暴露为 A2A 服务。

| # | 任务 | 涉及文件 | 验收标准 |
|---|------|----------|----------|
| 3.1 | A2AAgent 适配器 | `internal/a2a/trpc/agent.go` | 可通过 a2aagent 调用远程 Agent |
| 3.2 | A2A Server | `internal/a2a/trpc/server.go` | 本地 Agent 可被远程发现和调用 |
| 3.3 | 远程 Agent 注册 API | `api/kratos/a2a/v1/a2a.proto` + service | 注册/列出/删除远程 Agent |
| 3.4 | A2A HTTP Client | `internal/a2a/client.go` | 通过 URL 发现远程 AgentCard |
| 3.5 | 消息转换 | `internal/a2a/trpc/converter.go` | trpc Event ↔ A2A Message 双向转换 |
| 3.6 | 流式通信 | `internal/a2a/trpc/` | A2A SSE 流式响应 |
| 3.7 | Graph 恢复 | `internal/a2a/graph_resume.go` | A2A 任务中断/恢复 Graph |

### Phase 4：A2A 网关（P3）

目标：集中管理 A2A Agent 注册和发现。

| # | 任务 | 涉及文件 | 验收标准 |
|---|------|----------|----------|
| 4.1 | 注册中心 | `internal/a2a/registry/` | Agent 可注册/发现/健康检查 |
| 4.2 | 远端鉴权 / mTLS | `internal/a2a/` | 跨实例通信安全 |

---

## 5. 任务清单

| # | 任务 | 优先级 | Phase | 状态 |
|---|------|--------|-------|------|
| 1 | TRPCBuilderDeps 增加 A2AUsecase | P0 | 1 | ❌ |
| 2 | 实现 invokerFunc | P0 | 1 | ❌ |
| 3 | 注入 A2A 上下文（WithA2AUsecase + WithCallerAgentID + WithInvoker） | P0 | 1 | ❌ |
| 4 | Invoke 端点实际派发 | P0 | 1 | ❌ |
| 5 | 跨工作区校验 | P0 | 1 | ❌ |
| 6 | 端到端测试 | P0 | 1 | ❌ |
| 7 | A2AAgentCardPage.vue | P1 | 2 | ❌ |
| 8 | A2AAuditPage.vue | P1 | 2 | ❌ |
| 9 | 路由注册 | P1 | 2 | ❌ |
| 10 | A2AAgent 适配器 | P1 | 3 | ❌ |
| 11 | A2A Server | P1 | 3 | ❌ |
| 12 | 远程 Agent 注册 API | P1 | 3 | ❌ |
| 13 | A2A HTTP Client | P1 | 3 | ❌ |
| 14 | 消息转换 | P2 | 3 | ❌ |
| 15 | 流式通信 | P2 | 3 | ❌ |
| 16 | Graph 恢复 | P2 | 3 | ❌ |
| 17 | A2A 网关注册中心 | P3 | 4 | ❌ |
| 18 | 远端鉴权 / mTLS | P3 | 4 | ❌ |

---

## 6. 验收标准

- [ ] Phase 1：Agent 可通过 `call_agent` 工具调用同工作区其他 Agent
- [ ] Phase 1：`POST /v1/a2a/invoke` 可实际执行目标 Agent
- [ ] Phase 1：跨工作区调用返回 403 Forbidden
- [ ] Phase 1：审计日志正确记录所有调用
- [ ] Phase 2：前端可管理 AgentCard（启用/禁用/能力编辑）
- [ ] Phase 2：前端可浏览审计日志
- [ ] Phase 3：AgentCard 可被远程发现
- [ ] Phase 3：跨实例 Agent 通信正常
- [ ] Phase 3：消息双向转换无信息丢失
- [ ] Phase 3：A2A 通信支持流式响应
- [ ] Phase 3：A2A 长时间任务可中断/恢复 Graph
- [ ] Phase 4：A2A 网关注册中心可用

---

## 7. 依赖与风险

- **trpc-agent-go A2A 框架**：`pkg/trpc-agent-go/agent/a2aagent` 和 `server/a2a` 已可用，但 API 可能随 Google A2A 规范更新而变化
- **Context 注入位置**：需确认 `trpc_turn.go` 是 A2A 上下文注入的正确位置，可能需要调整
- **invokerFunc 实现**：需决定是同步执行（阻塞当前 Agent turn）还是异步执行（回调通知）
- **跨实例通信**：需考虑网络延迟、可靠性和超时处理
- **安全**：远端鉴权和 mTLS 是跨实例通信的前提条件
