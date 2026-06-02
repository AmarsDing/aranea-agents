# Evaluation Phase 3/5 — AfterTurn + 趋势/A/B + 高级评估

**日期**：2026-05-21  
**模块**：Evaluation (33)

## 摘要

### US-5 AfterTurn 自动评估（与 Chat 解耦）

- 新增 `biz.NativeTurnAfterHook` + `evaluation.AfterTurnTrigger`
- Chat 成功 turn 后经 `notifyNativeTurnHooks` 调用，失败不影响对话
- Agent `config_json.evaluation` 配置：
  ```json
  {"evaluation":{"auto_after_turn":true,"dataset_id":"...","metrics":"","num_runs":1,"min_interval_sec":300}}
  ```
- 运行记录 `trigger_source=after_turn`

### US-7 服务端趋势 / A/B 对比

- `GET /v1/evaluation/agents/{agent_id}/trend` — 分数时间线
- `POST /v1/evaluation/runs/compare` — 多 run 指标 delta（baseline=首个 run_id）

### Phase 5 高级评估

- **多轮 EvalSet**：`metadata_json.turns` → 多个 Invocation
- **UserSimulation**：`metadata_json.user_simulation.script` + `RunEvaluation.use_user_simulation`
- **pass@k / pass^k**：`num_runs>1` 时 FrameworkBridge 计算并持久化 `pass_at_k` / `pass_hat_k`

## 验证

```bash
make api
go test ./internal/evaluation/... ./internal/biz/... ./internal/data/... -run Eval
```
