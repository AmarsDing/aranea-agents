## ADDED Requirements

### Requirement: Knowledge/Plugin 接口抽象
系统 SHALL 通过接口抽象 + Wire 注入替代对具体实现的直接依赖。

#### Scenario: Knowledge 接口
- **WHEN** Team Graph 需要访问 Knowledge
- **THEN** 通过 KnowledgeProvider 接口获取，默认实现为当前逻辑

### Requirement: magic string 常量化
系统 SHALL 将字符串字面量改为集中定义常量。

#### Scenario: 常量引用
- **WHEN** 代码引用 Team Graph 相关字符串
- **THEN** 使用 `team_graph_constants.go` 中定义的常量
