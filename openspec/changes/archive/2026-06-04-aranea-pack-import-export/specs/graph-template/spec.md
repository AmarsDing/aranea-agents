## MODIFIED Requirements

### Requirement: Graph 模板从 Pack 数据源加载
ListGraphTemplates API SHALL 同时返回内置模板（从 .arpack 加载）和用户模板（从 DB 加载）。

#### Scenario: 内置模板从 Pack 加载
- **WHEN** 调用 ListGraphTemplates API
- **THEN** 系统 SHALL 从 embed 的 builtin-templates.arpack 中读取内置 Graph 模板，与用户模板合并返回

#### Scenario: Pack 中无 Graph 模板
- **WHEN** builtin-templates.arpack 不包含 graphs/ 目录
- **THEN** 系统 SHALL 只返回用户模板，不报错

### Requirement: Graph 模板 YAML 与 Go 结构体互转
系统 SHALL 支持 Graph 模板在 YAML 格式和 GraphTemplate Go 结构体之间互转。

#### Scenario: YAML 转为 GraphTemplate
- **WHEN** 从 Pack 读取 graphs/pipeline.yaml
- **THEN** 系统 SHALL 将其反序列化为 `GraphTemplate` 结构体，与现有 `templates.go` 中的结构一致

#### Scenario: GraphTemplate 转为 YAML
- **WHEN** 导出 Graph 模板
- **THEN** 系统 SHALL 将 `GraphTemplate` 结构体序列化为 YAML 格式写入 Pack
