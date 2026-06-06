## ADDED Requirements

### Requirement: RootCauseAnalyzer 接口抽取
系统 SHALL 从 RootCauseEngine 抽取 RootCauseAnalyzer 接口，供 SkillIntelligenceUsecase 和 PredictiveHealUsecase 复用。接口 MUST 包含 Analyze(ctx, stepID, phase, err, metadata) 和 AnalyzeFromReport(ctx, report) 方法。

#### Scenario: RootCauseEngine 实现 RootCauseAnalyzer
- **WHEN** Wire DI 装配时
- **THEN** 系统 SHALL 将 RootCauseEngine 作为 RootCauseAnalyzer 接口的实现注入

#### Scenario: SkillIntelligenceUsecase 使用 RootCauseAnalyzer
- **WHEN** GenerateReport 需要根因分析
- **THEN** 系统 SHALL 通过注入的 RootCauseAnalyzer 接口调用 AnalyzeFromReport，不直接依赖 RootCauseEngine 具体类型

#### Scenario: PredictiveHealUsecase 使用 RootCauseAnalyzer
- **WHEN** 预测性自愈需要分析历史模式
- **THEN** 系统 SHALL 通过注入的 RootCauseAnalyzer 接口调用 Analyze，不直接依赖 RootCauseEngine 具体类型
