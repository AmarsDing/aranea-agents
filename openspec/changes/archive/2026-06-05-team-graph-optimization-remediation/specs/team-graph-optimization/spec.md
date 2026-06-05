## ADDED Requirements

### Requirement: FunctionResolver 集成
系统 SHALL 在 wireNode 中消费 FunctionResolver，完成集成，降级时 SHALL 记录 warning。

#### Scenario: wireNode 消费 FunctionResolver
- **WHEN** wireNode.Type == "function"
- **THEN** 调用 FunctionResolver.Resolve 获取函数定义，降级时记录 warning
