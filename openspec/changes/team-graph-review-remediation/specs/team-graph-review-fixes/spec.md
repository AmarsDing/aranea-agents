## MODIFIED Requirements

### Requirement: Knowledge/Plugin 接口抽象
原直接依赖具体实现。现改为通过接口抽象 + Wire 注入。

#### Scenario: Knowledge 接口
- **WHEN** Team Graph 需要访问 Knowledge
- **THEN** 通过 KnowledgeProvider 接口获取，默认实现为当前逻辑

### Requirement: magic string 常量化
原使用字符串字面量。现改为集中定义常量。

#### Scenario: 常量引用
- **WHEN** 代码引用 Team Graph 相关字符串
- **THEN** 使用 `team_graph_constants.go` 中定义的常量
