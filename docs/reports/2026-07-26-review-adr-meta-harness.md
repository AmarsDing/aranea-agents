# ADR-07: Meta-Harness（Harness 自改进）归档为远期方向

## 状态：已接受（归档，不实施）

## 背景

[2026-07-25-review-harness-rsi-gap-analysis.md](./2026-07-25-review-harness-rsi-gap-analysis.md) §4 将项目进化能力划分为 P0→P3 阶梯。P3 定义为「候选 harness 版本库 + 沙箱 A/B + 优化器参数自调」，对应 RSI 方案中的 Meta-Harness / STOP 路线——即改进机制本身（触发阈值、Gate 标准、模式挖掘参数、Harness 代码）成为改进对象。

评审 §3.2 指出：当前改进机制的参数全部硬编码（如 `EvoTriggerScoreThreshold = 60`、`GatePerformanceDegradationThreshold = 0.20`、`CURATOR_DAILY_MAX = 20`），「改进者不可被改进」是递归自我改进的终极缺口。P0（LLM Curator + Reload 接线）与 P1（delta 协议 + 归因 + trace 观测 + Solve 回放）落地后，P3 成为唯一剩余阶梯，需要对其处置方式做出正式决策。

## 决策

**P3 归档为远期方向，当前阶段不实施。** 仅记录候选方案与触发条件，不在代码中预留任何 Meta-Harness 脚手架（YAGNI）。

1. 不以任何形式实现「候选 harness 版本库」「沙箱 A/B」「优化器参数自调」
2. 硬编码参数保持现状；当同一参数被人工调整 ≥3 次时，再评估是否将其迁入 SystemSetting 动态配置（这是运维信号，不是递归改进）
3. 若未来重启 P3，必须先产出新的 ADR 并明确：优化目标的度量函数、沙箱 A/B 的流量隔离方案、回滚边界

## 后果

**正面**：
- 避免在业务验证早期投入高复杂度、低确定性的 Meta 层建设（评审 §5 业务导向判断：投入产出比极低）
- 治理注意力集中在已通电的 ACE/MCE 链路质量上（人工审批在环是正确终态）
- P2（evolver 作为进化目标，架构零成本）仍开放，是优先级远高于 P3 的下一步

**负面**：
- 改进机制的参数（触发阈值、Gate 阈值、日上限）仍需人工调参，系统无法自主找到更优的进化策略
- 「改进者不可被改进」的递归缺口持续存在，项目定级停留在 MCE 之下

## 替代方案（考虑过但未选择）

| 方案 | 未选择原因 |
|------|-----------|
| 立即实现优化器参数自调（如自动调节 EvoTriggerScoreThreshold） | 无足够样本量支撑参数效果的统计判定；当前周期量级下人工调参更快更准 |
| 候选 harness 版本库 + 沙箱 A/B | 需要 harness 可序列化与运行时热替换能力，当前 Harness 与进程编译期绑定，改造成本远超收益 |
| STOP 式自修改代码 | 代码层自改的安全边界与验证体系（形式化 Gate）不存在，风险不可控 |

## 重启触发条件

满足以下全部条件时应重新评估本 ADR：
1. P2（evolver 自进化）已上线并稳定运行
2. 进化建议月产生量 ≥ 100 条，且人工审批通过率数据可用于优化目标度量
3. 业务确认「减少人工审批/调参」成为明确诉求

---

> 关联文档：[2026-07-25-research-harness-recursive-self-improvement.md](./2026-07-25-research-harness-recursive-self-improvement.md)、[2026-07-25-review-harness-rsi-gap-analysis.md](./2026-07-25-review-harness-rsi-gap-analysis.md)、[07-P1-delta协议与归因观测.design.md](../development/phase3-进化能力/07-P1-delta协议与归因观测.design.md)
