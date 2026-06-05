## MODIFIED Requirements

### Requirement: FunctionResolver 集成
原仅定义接口和 DI 绑定。现改为 wireNode 消费 FunctionResolver，完成集成。

#### Scenario: wireNode 消费 FunctionResolver
- **WHEN** wireNode.Type == "function"
- **THEN** 调用 FunctionResolver.Resolve 获取函数定义，降级时记录 warning
