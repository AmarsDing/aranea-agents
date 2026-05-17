# Runner 运行器 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[40 runner.md](./40%20runner.md) · **设计**：[40 runner.design.md](./40%20runner.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Runner 运行器：管理 Agent/Team 的运行生命周期，包括启动、停止、状态监控和资源回收。

**代码锚点**：
- `internal/agent/team_runner.go` — team.Runner
- `internal/service/chat.go` — activeRuns / pendingQueue
- `internal/agent/trpc_build.go` — Agent 构建链

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Agent 运行 | ✅ | `runNativeAgentTurn` |
| Team 运行 | ✅ | `teamsNative.RunTurn` |
| 停止运行 | ✅ | `StopGeneration` + cancel |
| 运行状态 | ✅ | `runStatuses` sync.Map |
| 待执行队列 | ✅ | `pendingQueue` + `processPendingQueue` |
| 运行超时 | ✅ | `http.Client{Timeout: 300s}` |
| 资源回收 | 🟡 | 运行结束后未清理 LRU 缓存 |

---

## 3. 差距与优化

1. **P2**：运行结束后未清理 `BuildTRPCLLMAgentCached` 的 LRU 缓存，长时间运行后内存可能膨胀。
2. **P3**：无运行资源限制（CPU/内存），恶意 Agent 可能消耗过多资源。
3. **P3**：无运行优先级调度，所有运行平等对待。

---

## 4. 开发阶段

- **Phase 1**：LRU 缓存清理机制
- **Phase 2**：运行资源限制
- **Phase 3**：运行优先级调度

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | `BuildTRPCLLMAgentCached` LRU 缓存 TTL 过期清理 | P2 | — |
| 2 | 运行资源限制（CPU/内存配额） | P3 | — |
| 3 | 运行优先级调度（高/中/低） | P3 | — |

---

## 6. 验收标准

- [ ] LRU 缓存有过期清理机制
- [ ] 运行资源超限后优雅降级
- [ ] 高优先级运行优先调度

---

## 7. 依赖与风险

- 资源限制需与 Docker/容器运行时集成
- 优先级调度需修改 pendingQueue 实现
