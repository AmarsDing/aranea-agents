## ADDED Requirements

### Requirement: 后端 Proto Service 前端覆盖率检查
系统 SHALL 在 CI 中检查后端 proto Service 是否有对应的前端 `createXxxService` 工厂，缺失时报告警告。

#### Scenario: 后端 Service 无前端客户端
- **WHEN** 后端 proto 定义了一个 Service 但前端 `services/index.ts` 无对应 `createXxxService`
- **THEN** CI 输出警告，列出缺失的前端 Service 工厂名称

#### Scenario: 所有后端 Service 都有前端客户端
- **WHEN** 后端所有 proto Service 在前端都有对应的 `createXxxService`
- **THEN** CI 检查通过

### Requirement: 前端 Service 客户端补齐
系统 SHALL 为以下 5 个后端 Service 补齐前端 proto 生成的客户端：AgentCategoryService、SkillEvolutionService、WebhookService、PackService、PlanService。

#### Scenario: AgentCategoryService 前端可调用
- **WHEN** 前端调用 `createAgentCategoryService()` 返回的客户端方法
- **THEN** 请求正确路由到后端 `agent_category/v1` proto Service

#### Scenario: SkillEvolutionService 前端可调用
- **WHEN** 前端调用 `createSkillEvolutionService()` 返回的客户端方法
- **THEN** 请求正确路由到后端 `skill_evolution/v1` proto Service

### Requirement: Spirit Service proto 化
系统 SHALL 使用 `spirit/v1` proto 生成的客户端替换手工 `createSpiritService`。

#### Scenario: Spirit API 调用类型安全
- **WHEN** 前端调用 Spirit 相关 API
- **THEN** 使用 proto 生成的类型安全客户端，而非手工拼接的 HTTP 端点
