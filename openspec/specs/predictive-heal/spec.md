## ADDED Requirements

### Requirement: 预测性自愈
系统 SHALL 基于历史失败模式和当前系统指标，预测即将发生的错误并提前干预。PredictiveHealUsecase MUST 仅对置信度 > 0.8 的预测执行预防行动。

#### Scenario: Provider 延迟趋势预测 RateLimit
- **WHEN** Provider 延迟持续上升且历史有 RateLimit 失败模式
- **THEN** 系统 SHALL 预测 RateLimit 错误，置信度 > 0.8 时提前切换 Provider

#### Scenario: Memory 使用率预测 OOM
- **WHEN** Memory 使用率 > 80% 且历史有 OOM 失败模式
- **THEN** 系统 SHALL 预测 OOM 错误，置信度 > 0.8 时预热 Memory 缓存或限流

#### Scenario: 低置信度预测不执行
- **WHEN** 预测置信度 <= 0.8
- **THEN** 系统 SHALL 仅记录预测结果，不执行预防行动

### Requirement: 预防行动冷却期
系统 SHALL 对同类型的预防行动设置 30 分钟冷却期，防止重复执行。

#### Scenario: 冷却期内不重复执行
- **WHEN** 同类型预防行动在 30 分钟内已执行过
- **THEN** 系统 SHALL 跳过本次预防行动，记录冷却期命中

### Requirement: 预防行动可审计
系统 SHALL 将所有预防行动记录到 HealRecord，包含预测依据、置信度、执行结果。

#### Scenario: 预防行动记录
- **WHEN** 执行预防行动
- **THEN** 系统 SHALL 创建 HealRecord，包含 prediction_basis、confidence、action_taken、result 字段
