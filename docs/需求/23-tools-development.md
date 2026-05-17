# Tools 工具 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[23 tools.md](./23%20tools.md) · **设计**：[23 tools.design.md](./23%20tools.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Tools 工具系统：管理 Agent 可调用的工具（内置工具 + 自定义工具 + MCP 工具），支持工具的注册、发现、调用和参数校验。

**代码锚点**：
- `api/kratos/tool/v1/` — Tool CRUD RPC
- `internal/service/tool.go` — ToolService
- `internal/biz/tool.go` — ToolUsecase
- `internal/data/tool.go` — ToolRepo
- `internal/agent/trpc_build.go` — Tool 注入
- `internal/biz/agent_effective_tools.go` — Effective Tools 计算

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Tool CRUD | ✅ | Create/Update/Delete/Get/List |
| Tool 类型 | ✅ | builtin / custom / mcp |
| Effective Tools | ✅ | `agent_effective_tools.go` |
| Tool 注入 | ✅ | `BuildTRPCLLMAgent` 中 `WithTools` |
| Tool 参数 Schema | ✅ | `parameters_json` 字段 |
| 前端管理 | ✅ | Tool 设置页 |

---

## 3. 差距与优化

1. **P2**：ToolOverride（工具参数覆盖）biz/Repo/Service 未实现（见 5-agent-setting-development.md）。
2. **P3**：自定义工具无在线测试功能，用户无法在配置时验证工具是否可用。
3. **P3**：工具调用无审计日志，无法追溯谁在何时调用了什么工具。

---

## 4. 开发阶段

- **Phase 1**：ToolOverride 实现（见 5-agent-setting-development.md）
- **Phase 2**：工具在线测试
- **Phase 3**：工具调用审计日志

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | ToolOverride biz/Repo/Service（见 5-agent-setting） | P2 | EP-BIZ-06 |
| 2 | `TestTool` RPC：在线测试自定义工具 | P3 | — |
| 3 | `tool_invocation_audit` 表 + 查询 API | P3 | — |

---

## 6. 验收标准

- [ ] Agent 可覆盖特定工具的参数
- [ ] 自定义工具可在配置时在线测试
- [ ] 工具调用可审计追溯

---

## 7. 依赖与风险

- ToolOverride 与 Agent 设置页紧耦合
- 审计日志需注意存储膨胀
