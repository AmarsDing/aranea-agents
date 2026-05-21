# Evaluation Phase 5 — 文档同步

**Date:** 2026-05-21

## 摘要

将 Evaluation Phase 5 实现（扩展指标、LLM UserSim、趋势/A/B 前端、Eval LLM 系统配置）同步至需求/设计/开发计划、前端页面索引与系统开发总览。

## 代码真相（已实现）

| 能力 | 锚点 |
|------|------|
| 扩展指标 | `framework_metrics.go`：`json_match` / `xml_match` / `rouge_l` / `tool_trajectory` |
| scores_json | `scores.go` + `eval_runs` / `eval_case_results` 列 |
| LLM UserSim | `llm_simulator.go` + trpc `simRunner` |
| 模型解析 | `eval_llm_resolve.go`：env > `system_settings` > catalog |
| Eval LLM 持久化 | `system_settings.eval_*` + `SystemSettingsPage` + 默认 Sim `openai`/`gpt-4o-mini` |
| 趋势 / A/B | `GetAgentEvalTrend` / `CompareEvalRuns` + `EvaluationAnalyticsPanel` |

## 已更新文档

| 文档 | 变更 |
|------|------|
| [33 evaluation.md](../需求/33%20evaluation.md) | 状态 Phase 5；US-7/8；§3.3 扩展指标；§3.4 Eval LLM 配置 |
| [33 evaluation.design.md](../需求/33%20evaluation.design.md) | 架构/分层/Schema/Runner/§6.5–6.6/前端/演进表 |
| [33-evaluation-development.md](../需求/33-evaluation-development.md) | 代码锚点、审计表、任务 #12–13、验收项 |
| [frontend-pages.md](../需求/frontend-pages.md) | `/settings` Eval LLM；`/evaluation` AnalyticsPanel |
| [README-development.md](../需求/README-development.md) | 接入度、近期完成、文档同步表 |
| [0-system-development.md](../需求/0-system-development.md) | Evaluation 行、§8.7 EVAL-02–04 |
| [2026-05-21-Evaluation-Phase5-Extended.md](./2026-05-21-Evaluation-Phase5-Extended.md) | 实现 changelog（已有） |

## 运行时配置速查

```
优先级：KRATOS_EVAL_* env  >  system_settings.eval_*  >  Provider 目录 mini/flash
Settings 默认（仅表单）：eval_sim_provider=openai, eval_sim_model=gpt-4o-mini
扩展指标 opt-in：metrics=json_match,xml_match,rouge_l,tool_trajectory
```
