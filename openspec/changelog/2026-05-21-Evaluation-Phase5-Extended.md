# Evaluation Phase 5 — Extended Metrics & UserSim

**Date:** 2026-05-21

## Summary

- **LLM UserSimulation**: `NewLLMUserSimulator` + `simRunner` via trpc `usersimulation.New`; `resolveUserSimulator` picks scripted vs LLM path.
- **Extended metrics**: `json_match`, `xml_match`, `rouge_l`, `tool_trajectory` (order-sensitive) in `framework_metrics.go`; opt-in via `metrics` comma list.
- **scores_json**: run/result extended metric map persisted; legacy columns unchanged.
- **ToolTrajectory**: `expected_tool_calls` / `expected_tools` attached to evalset invocations.
- **Frontend**: `getAgentEvalTrend`, `compareEvalRuns`, `EvaluationAnalyticsPanel`.
- **System settings**: `eval_sim_provider/model`, `eval_judge_provider/model` in `system_settings` + Settings 页；运行时 precedence：env > DB > catalog。

## Files

| Area | Path |
|------|------|
| Metrics | `internal/evaluation/framework_metrics.go`, `scores.go` |
| UserSim | `internal/evaluation/llm_simulator.go`, `scripted_simulator.go` |
| Resolve | `internal/evaluation/eval_llm_resolve.go` |
| Bridge | `internal/evaluation/framework.go`, `evaluation_runner.go` |
| Schema | `eval_runs.scores_json`, `eval_case_results.scores_json`, `docs/sql/00_system_setting_eval_llm.sql` |
| API | `evaluation.proto` field `scores_json`; `system_setting.proto` `eval_llm` |
| UI | `EvaluationAnalyticsPanel.vue`, `SystemSettingsPage.vue` eval LLM block |
