# Agent 标题 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[8 agent-title.md](./8%20agent-title.md) · **设计**：[8 agent-title.design.md](./8%20agent-title.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Agent 标题生成：用户创建 Agent 后，系统自动根据 Agent 描述或首次对话内容生成标题。

**代码锚点**：
- `internal/service/session_title_llm.go` — LLMSessionTitleGenerator
- `internal/biz/agent_usecase.go` — Agent 创建流程

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| LLM 标题生成 | ✅ | `LLMSessionTitleGenerator.Generate` |
| Session 标题 | ✅ | 首次对话后自动生成 |
| Agent 名称 | ✅ | 创建时用户手动输入 |

---

## 3. 差距与优化

1. **P3**：Agent 标题（非 Session 标题）无自动生成逻辑，用户需手动输入。需求文档提到"根据描述自动生成"但未实现。

---

## 4. 开发阶段

- **Phase 1**：Agent 创建后自动根据描述生成标题（可选功能）

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | Agent 创建后可选自动生成标题 | P3 | — |

---

## 6. 验收标准

- [ ] Agent 创建时可选择自动生成标题

---

## 7. 依赖与风险

无重大依赖。
