# A2A 协议 — 开发计划

> **版本**：2026-05-17 | **状态**：🟡 AgentCard 可用；❌ call_agent 未注入
> **需求**：[26 a2a-protocol.md](./26%20a2a-protocol.md) · **设计**：[26 a2a-protocol.design.md](./26%20a2a-protocol.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-05

---

## 1. 模块定位

A2A（Agent-to-Agent）协议：支持 Agent 之间的跨实例通信和协作，包括 AgentCard 发现、call_agent 工具调用、远程 Agent 对话。

**代码锚点**：
- `api/kratos/agent/v1/` — AgentCard 相关字段
- `internal/agent/trpc_build.go` — Agent 构建链
- `pkg/trpc-agent-go/a2a/` — trpc-agent-go A2A 框架

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| AgentCard 字段 | ✅ | Agent proto 中有 a2a 相关字段 |
| A2A 框架 | ✅ | `pkg/trpc-agent-go/a2a/` 包
| call_agent 工具注入 | ❌ | `trpc_build.go` 中无 `call_agent` 注入 |
| 远程 Agent 发现 | ❌ | 无 Agent 注册中心 |
| A2A 消息路由 | ❌ | 无跨实例消息路由 |

---

## 3. 差距与优化

1. **P1（EP-BIZ-05）**：`call_agent` 工具未注入 Agent 工具集，Agent 无法调用远程 Agent。这是 A2A 协议的核心功能缺失。
2. **P2**：无 Agent 注册中心，远程 Agent 无法被发现。
3. **P3**：无 A2A 消息路由，跨实例通信未实现。

---

## 4. 开发阶段

- **Phase 1（EP-BIZ-05）**：注入 `call_agent` 工具到 Agent 工具集
- **Phase 2**：Agent 注册中心（AgentCard 发布/发现）
- **Phase 3**：A2A 消息路由（跨实例通信）

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | `trpc_build.go`：注入 `call_agent` 工具 | P1 | EP-BIZ-05 |
| 2 | AgentCard 发布/发现 API | P2 | — |
| 3 | A2A 消息路由 + 跨实例通信 | P3 | — |
| 4 | A2A 安全（认证/授权） | P3 | — |

---

## 6. 验收标准

- [ ] Agent 可通过 `call_agent` 调用其他 Agent
- [ ] AgentCard 可被远程发现
- [ ] 跨实例 Agent 通信正常

---

## 7. 依赖与风险

- A2A 协议仍在演进，需关注 Google A2A 规范更新
- 跨实例通信需考虑网络延迟和可靠性
