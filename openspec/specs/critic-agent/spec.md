## ADDED Requirements

### Requirement: Critic Agent 语义回归检查
系统 SHALL 在 Auto-Fix 验证通过后，用 LLM 对比 diff 检查语义偏差。Critic Agent MUST 输出结构化 CriticResult，包含 is_safe(bool)、risk_level(low/medium/high)、concerns([]string)、suggestion(string)。

#### Scenario: 低风险修复直接创建 PR
- **WHEN** Critic Agent 评估修复的 risk_level 为 "low"
- **THEN** 系统 SHALL 直接创建 auto-fix PR

#### Scenario: 中等风险修复需仔细审查
- **WHEN** Critic Agent 评估修复的 risk_level 为 "medium"
- **THEN** 系统 SHALL 创建 auto-fix PR 并添加 "needs-careful-review" 标签

#### Scenario: 高风险修复放弃
- **WHEN** Critic Agent 评估修复的 risk_level 为 "high"
- **THEN** 系统 SHALL 放弃修复，记录到知识库，不创建 PR

#### Scenario: Critic Agent 每日调用上限
- **WHEN** Critic Agent 当日调用次数达到 10 次
- **THEN** 系统 SHALL 跳过 Critic Agent 步骤，直接创建 PR（降级为无语义检查）

### Requirement: Critic Agent 可禁用
系统 SHALL 支持通过环境变量 ENABLE_CRITIC_AGENT=false 跳过 Critic Agent 步骤。

#### Scenario: 禁用 Critic Agent
- **WHEN** 环境变量 ENABLE_CRITIC_AGENT 设置为 "false"
- **THEN** 系统 SHALL 跳过 Critic Agent 步骤，直接创建 auto-fix PR
