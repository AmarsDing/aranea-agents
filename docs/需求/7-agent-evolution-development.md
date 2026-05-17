# Agent 进化 — 开发计划

> **版本**：2026-05-17 | **状态**：🟡 EvolutionScanner 未实现
> **需求**：[7 agent-evolution.md](./7%20agent-evolution.md) · **设计**：[7 agent-evolution.design.md](./7%20agent-evolution.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-07

---

## 1. 模块定位

Agent 自我进化：基于运行指标和反馈，自动或半自动地优化 Agent 的提示、工具配置和知识。

**代码锚点**：
- `internal/biz/evolution.go` — EvolutionUsecase（GetEvolutionMetrics / GetEvolutionSuggestions / ApplySuggestion）
- `internal/data/ent/schema/evolution_suggestion.go` — Ent Schema
- `internal/service/agent_evolution.go` — Evolution RPC
- `internal/agent/trpc_build.go` — `SelfEvolve` / `EvolutionSelfEvolve` 开关

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| 进化指标查询 | ✅ | `GetEvolutionMetrics` RPC |
| 进化建议列表 | ✅ | `GetEvolutionSuggestions` RPC |
| 应用建议 | ✅ | `ApplySuggestion` RPC |
| EvolutionSuggestion 表 | ✅ | Ent schema + data repo |
| EvolutionScanner（30min ticker） | ❌ | `internal/` 无对应 worker |
| L4 持久层进化 | ❌ | 需求已降级为"未实现" |

---

## 3. 差距与优化

1. **P2（EP-BIZ-07）**：EvolutionScanner（30min ticker 自动扫描运行指标并生成建议）代码不存在。当前进化建议需手动触发或外部注入。
2. **P3**：进化建议的 diff_preview 字段为空，无实际 diff 生成逻辑。
3. **P3**：进化指标（tool_success_rate / retrieval_quality）为静态返回，无真实计算逻辑。

---

## 4. 开发阶段

- **Phase 1**：实现 EvolutionScanner（后台 worker + safego + 30min ticker）
- **Phase 2**：进化指标真实计算（从 session/tool_invocation 聚合）
- **Phase 3**：进化建议 diff 生成（对比当前 prompt 与建议 prompt）

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | `internal/evolution/scanner.go`：30min ticker + safego | P2 | EP-BIZ-07 |
| 2 | 指标聚合：从 session/tool_invocation 计算 success_rate | P2 | EP-BIZ-07 |
| 3 | 建议生成：基于指标阈值生成 EvolutionSuggestion | P2 | EP-BIZ-07 |
| 4 | diff_preview 生成逻辑 | P3 | — |
| 5 | Wire 注入 Scanner 到启动流程 | P2 | EP-BIZ-07 |

---

## 6. 验收标准

- [ ] EvolutionScanner 每 30min 自动运行，生成进化建议
- [ ] 进化指标基于真实运行数据计算
- [ ] `go test ./internal/evolution/...` 通过

---

## 7. 依赖与风险

- Scanner 需读取大量 session/tool_invocation 数据，需注意查询性能
- 进化建议的自动应用需谨慎，建议默认为"待审核"状态
