# 33 Evaluation Review

> **评分**：82 / 100 | **风险等级**：P1  
> **文档**：[33 evaluation.md](../需求/33%20evaluation.md) · [33 evaluation.design.md](../需求/33%20evaluation.design.md) · [33-evaluation-development.md](../需求/33-evaluation-development.md)  
> **代码锚点**：`internal/evaluation/` · `internal/service/evaluation.go` · `internal/biz/agent_eval_config.go` · `web/src/features/evaluation/`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 17 | 20 | Phase 5 ✅：4+扩展指标 + LLM UserSim + pass@k + AfterTurn + 趋势/A/B + Eval LLM 系统配置；质量门禁产品化待补 |
| 架构一致性 | 22 | 25 | `internal/evaluation` 独立适配层；框架 EvalSet 对齐（`FrameworkBridge`）✅；AfterTurn 挂点正确 |
| 后端实现质量 | 17 | 20 | EvalSet CRUD + LLMJudge + UserSim + scores_json + 人工标注 ✅；EvalSet 导出 ✅ |
| 前端实现质量 | 13 | 15 | `EvaluationPage.vue` + 趋势/A/B ✅；数据集 CRUD + 用例 + Run 启动 ✅；人工标注 + CSV/JSON 导出 ✅ |
| 测试与验证 | 7 | 10 | `evalset_adapter.go` ✅；EvalSet 框架层有测试；UserSim 路径测试待补 |
| 文档一致性 | 6 | 10 | 三件套 + Phase 5 changelog 已同步；`33 evaluation.md` US-7/8 扩展指标对齐 |

---

## 已验收功能（Phase 5）

| 功能 | 状态 |
|------|------|
| EvalSet CRUD + 用例上传 | ✅ |
| Eval Run 启动（metrics/num_runs/use_user_simulation）| ✅ |
| 基础指标（exact_match/token_overlap/latency）| ✅ |
| 扩展指标（4+ 项，US-7）| ✅ Phase 5 |
| LLM-as-Judge | ✅ |
| LLM UserSim（US-8）| ✅ Phase 5 |
| pass@k | ✅ |
| AfterTurn 挂点 | ✅ |
| scores_json 持久化 | ✅ Phase 5 |
| Agent 趋势表（`GetAgentEvalTrend`）| ✅ Phase 5 |
| A/B 多选 Run 对比（`compareEvalRuns`）| ✅ Phase 5 |
| 人工标注（`AnnotateCaseResult`）| ✅ EVAL-02 |
| CSV/JSON 导出 | ✅ I7-EVAL-01 |
| Eval LLM 系统配置（UserSim/Judge 模型）| ✅ Phase 5 |
| 质量门禁（PR 前自动评估）| ❌ |
| 迭代闭环产品化 | ❌ |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| EVAL-P1-01 | LLM UserSim 路径（`eval_sim_*` 系统配置 → UserSim 调用）无专项测试 | 补 UserSim 集成测试 |
| EVAL-P1-02 | Eval LLM 配置（`env KRATOS_EVAL_*`）优先于系统设置的行为需要用户文档化 | 在 SystemSettingsPage 评估 LLM 说明中标注 env 优先级 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| EVAL-P2-01 | 质量门禁（CI/PR 前自动评估）未实现 | 规划质量门禁 API + CI 集成方案 |
| EVAL-P2-02 | A/B 对比仅支持同 EvalSet，跨 EvalSet 对比未规划 | 规划跨 EvalSet 对比 |

---

## 系统配置集成

```
system_settings.eval_sim_*  → UserSim 默认模型（openai/gpt-4o-mini）
system_settings.eval_judge_* → LLM-as-Judge 默认模型
env KRATOS_EVAL_SIM_* / KRATOS_EVAL_JUDGE_* 优先
```

**正确实现**：env 优先，系统设置作为 fallback ✅

---

## 建议优化路径

1. 补 UserSim 集成测试（P1）。
2. 文档化 env vs 系统设置的优先级（P1）。
3. 规划质量门禁 API（P2）。
