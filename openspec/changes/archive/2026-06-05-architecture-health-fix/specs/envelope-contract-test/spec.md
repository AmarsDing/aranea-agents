## ADDED Requirements

### Requirement: EnvelopeType 前后端契约自动比对
系统 SHALL 在 CI 中自动比对后端 `internal/event/contract/envelope.go` 的 EnvelopeType 常量与前端 `web/src/realtime/envelope.ts` 的 EnvelopeType 常量，不一致时测试失败。

#### Scenario: 后端新增 EnvelopeType 但前端未同步
- **WHEN** 后端 `envelope.go` 新增一个 EnvelopeType 常量但前端 `envelope.ts` 未添加对应常量
- **THEN** CI 契约测试失败，输出缺失的常量名称

#### Scenario: 前端有但后端无的 EnvelopeType
- **WHEN** 前端 `envelope.ts` 有一个 EnvelopeType 常量但后端 `envelope.go` 无对应常量
- **THEN** CI 契约测试失败，输出多余的常量名称

#### Scenario: 前后端 EnvelopeType 完全一致
- **WHEN** 后端和前端的 EnvelopeType 常量值完全匹配
- **THEN** CI 契约测试通过

### Requirement: 缺失 EnvelopeType 补齐
系统 SHALL 确保后端所有 EnvelopeType 在前端都有对应的类型定义和事件处理器。

#### Scenario: token_usage 事件前端可解析
- **WHEN** 后端发送 `token_usage` 类型的 Envelope
- **THEN** 前端 `Envelope` 类型包含 `token_usage` 字段，类型为 `EnvelopeTokenUsage`

#### Scenario: butler 编排事件前端可识别
- **WHEN** 后端发送 `butler.orchestration.started`/`completed`/`failed` 类型的 Envelope
- **THEN** 前端 `EnvelopeType` 包含这 3 个常量，且有对应的 onType 处理器

#### Scenario: skill 事件前端可识别
- **WHEN** 后端发送 `skill.health_changed` 或 `skill.evolution_proposed` 类型的 Envelope
- **THEN** 前端 `EnvelopeType` 包含这 2 个常量，且有对应的 onType 处理器

### Requirement: 无处理器事件注册默认 handler
系统 SHALL 为所有已定义类型但无专门业务处理器的事件注册默认 onType handler（至少做日志记录）。

#### Scenario: mcp.session.reconnect 事件到达
- **WHEN** 前端收到 `mcp.session.reconnect` 事件
- **THEN** 默认 handler 记录日志，不静默丢弃

#### Scenario: alert.notify 事件到达
- **WHEN** 前端收到 `alert.notify` 事件
- **THEN** 默认 handler 显示 toast 通知

### Requirement: 前端死代码清理
系统 SHALL 移除前端 `EnvelopeUsage.prompt_breakdown` 字段及其相关逻辑，因为后端 `EnvelopeUsage` 无此字段。

#### Scenario: prompt_breakdown 字段移除
- **WHEN** 前端代码引用 `EnvelopeUsage.prompt_breakdown`
- **THEN** 编译失败（字段已删除），相关逻辑使用后端实际返回的字段
