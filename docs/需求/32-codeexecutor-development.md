# CodeExecutor 代码执行 — 开发计划

> **版本**：2026-05-17 | **状态**：🟡 Docker Sandbox 可用；❌ E2B/Jupyter/Interactive 未实现
> **需求**：[32 codeexecutor.md](./32%20codeexecutor.md) · **设计**：[32 codeexecutor.design.md](./32%20codeexecutor.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-04

---

## 1. 模块定位

CodeExecutor 代码执行：提供安全的代码执行环境，支持 Docker Sandbox、E2B 沙箱、Jupyter 内核和交互式执行。

**代码锚点**：
- `internal/agent/codeexecutor/executor.go` — Docker Sandbox 执行器
- `internal/agent/codeexecutor/executor_test.go` — 测试
- `internal/skill/trpc/executor.go` — Skill 路径（仍用 `codeexecutor/local`）
- `pkg/trpc-agent-go/codeexecutor/` — trpc-agent-go CodeExecutor 框架

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Docker Sandbox 执行器 | ✅ | `executor.go`（timeout / 资源限制 / artifact 收集） |
| 本地执行器 | ✅ | `codeexecutor/local` |
| E2B 沙箱 | ❌ | 未实现 |
| Jupyter 内核 | ❌ | 未实现 |
| Interactive 模式 | ❌ | 未实现 |
| WorkspaceRegistry | ❌ | 未实现 |
| Skill 路径默认执行器 | 🟡 | 仍用 `local`，未替换为 Docker |

---

## 3. 差距与优化

1. **P2（EP-BIZ-04）**：Skill 路径仍使用 `codeexecutor/local`，未替换为 Docker 执行器，存在安全风险。
2. **P2**：E2B 沙箱未实现，无法提供云端隔离执行环境。
3. **P3**：Jupyter 内核未实现，无法支持交互式数据分析。
4. **P3**：Interactive 模式未实现，无法支持多轮代码执行。
5. **P3**：WorkspaceRegistry 未实现，无法管理工作区状态。

---

## 4. 开发阶段

- **Phase 1（EP-BIZ-04）**：Skill 路径替换为 Docker 执行器
- **Phase 2**：E2B 沙箱执行器
- **Phase 3**：Jupyter 内核 + Interactive 模式
- **Phase 4**：WorkspaceRegistry

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | `internal/skill/trpc/executor.go`：替换 local → Docker | P2 | EP-BIZ-04 |
| 2 | `internal/agent/codeexecutor/e2b.go`：E2B 沙箱执行器 | P2 | EP-BIZ-04 |
| 3 | `internal/agent/codeexecutor/jupyter.go`：Jupyter 内核 | P3 | — |
| 4 | `internal/agent/codeexecutor/interactive.go`：Interactive 模式 | P3 | — |
| 5 | `internal/agent/codeexecutor/workspace.go`：WorkspaceRegistry | P3 | — |
| 6 | 执行器选择配置（Agent 级别选择 Docker/E2B/Local） | P2 | — |

---

## 6. 验收标准

- [ ] Skill 代码执行默认使用 Docker Sandbox
- [ ] E2B 沙箱可执行代码并返回结果
- [ ] Agent 可选择执行器类型
- [ ] `go test ./internal/agent/codeexecutor/...` 通过

---

## 7. 依赖与风险

- Docker 执行器需 Docker daemon 运行
- E2B 需 E2B API Key 和网络访问
- Jupyter 需 Jupyter 内核进程管理
