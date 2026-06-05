## ADDED Requirements

### Requirement: DynamicRankFactors 动态权重调整
系统 SHALL 引入 DynamicRankFactors，从 SkillHealthAggregator 读取近期指标动态调整排序权重，替代静态 RankFactors。

#### Scenario: 高成功率 Skill 降低探索加成
- **WHEN** Skill 的 7d 成功率 > 80%
- **THEN** 系统 SHALL 降低其 ExplorationBonus，使其在排序中更稳定

#### Scenario: 低成功率 Skill 提升探索加成或降权
- **WHEN** Skill 的 7d 成功率 < 40%
- **THEN** 系统 SHALL 提升其 ExplorationBonus（鼓励改进）或降低 HistoricalSuccess 权重

#### Scenario: 无历史数据使用默认权重
- **WHEN** Skill 无近期调用数据
- **THEN** 系统 SHALL 使用静态默认 RankFactors

### Requirement: HealthMetricsProvider 接口桥接
系统 SHALL 在 Tools 层定义 HealthMetricsProvider 接口，由 Biz 层实现并注入，避免 Tools 直接依赖 Biz。

#### Scenario: Biz 层实现 HealthMetricsProvider
- **WHEN** Wire DI 装配时
- **THEN** 系统 SHALL 将 SkillHealthAggregator 适配为 HealthMetricsProvider 接口的实现

#### Scenario: DynamicRankFactors 通过接口读取指标
- **WHEN** DynamicRankFactors 计算动态权重
- **THEN** 系统 SHALL 通过 HealthMetricsProvider 接口读取 GetRecentSuccessRate 和 GetRecentAvgDuration，不直接依赖 Biz 层

### Requirement: RankFeedback 排序反馈记录
系统 SHALL 记录排序结果与实际表现的对应关系，用于权重优化。

#### Scenario: 记录排序反馈
- **WHEN** Skill 被选中执行后
- **THEN** 系统 SHALL 记录 RankFeedback{skill_id, rank_score, actual_success, timestamp}
