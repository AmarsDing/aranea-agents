# Evaluation 评估 — 开发计划

> **版本**：2026-05-17 | **状态**：🟡 基础框架可用；❌ 自动评估未实现
> **需求**：[33 evaluation.md](./33%20evaluation.md) · **设计**：[33 evaluation.design.md](./33%20evaluation.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Evaluation 评估：对 Agent 的输出质量进行评估，支持自动评估（LLM-as-Judge）和人工评估，评估结果用于进化优化。

**代码锚点**：
- `api/kratos/evaluation/v1/` — Evaluation CRUD RPC
- `internal/service/evaluation.go` — EvaluationService
- `internal/biz/evaluation.go` — EvaluationUsecase
- `internal/data/evaluation.go` — EvaluationRepo
- `internal/evaluation/runner.go` — EvaluationRunner
- `internal/service/wire_providers.go` — Wire 注入

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Evaluation CRUD | ✅ | Create/Update/Delete/Get/List |
| EvaluationRunner | ✅ | `runner.go`（LLM-as-Judge） |
| 评估指标 | ✅ | accuracy / relevance / completeness |
- | 自动评估触发 | ❌ | 无自动评估触发机制 |
| 人工评估 | ❌ | 无人工评估 UI |
| 评估报告 | ❌ | 无评估报告生成 |
| 评估集管理 | ❌ | 无评估集（test suite）管理 |

---

## 3. 差距与优化

1. **P2**：无自动评估触发机制，评估需手动触发。
2. **P2**：无评估集管理，无法批量评估 Agent。
3. **P3**：无人工评估 UI，无法人工标注评估结果。
4. **P3**：无评估报告生成，无法汇总展示评估结果。

---

## 4. 开发阶段

- **Phase 1**：评估集管理（test suite CRUD + 批量评估）
- **Phase 2**：自动评估触发（Agent 运行后自动评估）
- **Phase 3**：人工评估 UI + 评估报告

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | `evaluation_suite` Ent 表 + CRUD API | P2 | — |
| 2 | 批量评估 API（RunSuite） | P2 | — |
| 3 | 自动评估触发（AfterTurn Hook） | P2 | — |
| 4 | 人工评估 UI | P3 | — |
| 5 | 评估报告生成 + 导出 | P3 | — |

---

## 6. 验收标准

- [ ] 可创建评估集并批量评估 Agent
- [ ] Agent 运行后可自动触发评估
- [ ] 评估结果可人工标注
- [ ] `go test ./internal/evaluation/...` 通过

---

## 7. 依赖与风险

- 评估消耗 LLM Token，需考虑成本
- 自动评估需与 Chat turn 流程集成
