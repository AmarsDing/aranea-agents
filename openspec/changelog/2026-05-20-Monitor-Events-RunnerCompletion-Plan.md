# Monitor：Runs / Events 分工（方案 C，文档）

**日期**：2026-05-20  
**模块**：Monitor  
**类型**：需求 + 设计 + 开发计划（无代码变更）

## 摘要

- **问题**：Chat 后 `runner.completion` 在 Events 展示空壳 JSON，与 Traces（Runs）列表信息重复。
- **决策（方案 C）**：
  - **Runs（Traces Tab）** = 单次运行真相源（`model_token_usage_events` + 现有 Trace 详情）。
  - **Events** = Team WS 实时流 + `alert.fired` + 无 Runs 行时的 completion 降级。
  - **`runner.completion`** 继续落库（告警、Runner 指标），metadata 以 **`trace_id` / `usage_event_id`** 关联为主，**不在 Events 为 Chat 建平行详情页**。

## 文档变更

| 文档 | 变更 |
|------|------|
| [需求/18 monitor.md](../需求/18%20monitor.md) | §3–§4 方案 C 定位；验收 RUN-01～06 |
| [需求/18 monitor.design.md](../需求/18%20monitor.design.md) | §九 重写为方案 C |
| [需求/18-monitor-development.md](../需求/18-monitor-development.md) | Phase 1d 任务 MON-1d-01～10 修订 |
| [guides/execution-plan.md](../guides/execution-plan.md) | I8-MON-02 描述更新 |

## 下一步

按 [18-monitor-development.md](../需求/18-monitor-development.md) Phase 1d 实现；完成后将本文标记为「已落地」。
