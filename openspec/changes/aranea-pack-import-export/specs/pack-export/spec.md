## ADDED Requirements

### Requirement: 单 Agent 导出
系统 SHALL 支持导出单个 Agent 为 .arpack 文件，kind 为 "agent"。

#### Scenario: 导出包含完整配置
- **WHEN** 用户请求导出 Agent（指定 agent_id）
- **THEN** 系统 SHALL 将 Agent 的身份属性、可移植运行时配置、Prompt 文件、Skill 引用写入 Pack

#### Scenario: 导出时 ID→Key 转换
- **WHEN** Agent 的 taxonomy_position_id 指向一个 TaxonomyNode
- **THEN** 系统 SHALL 将 taxonomy_position_id 转换为 taxonomy_key 路径格式（如 `finance/quant_trading/quant_researcher`）写入 position_key 字段

#### Scenario: 导出时排除实例绑定字段
- **WHEN** 导出 Agent RuntimeSettings
- **THEN** 系统 SHALL 排除 channel_id、chat_id、workspace、variables_json、model_instructions_json、code_executor_type 等实例绑定字段

### Requirement: 单 Team 导出
系统 SHALL 支持导出单个 Team 为 .arpack 文件，kind 为 "team"。

#### Scenario: Team 成员 Agent 递归导出
- **WHEN** 用户请求导出 Team（指定 team_id）
- **THEN** 系统 SHALL 将 Team 定义和所有成员 Agent 一并写入 Pack，成员 Agent 写入 agents/ 目录

#### Scenario: Team 成员引用转换为 agent_key
- **WHEN** Team 的 definition_json 中 members[].agent_id 为 UUID
- **THEN** 系统 SHALL 将 agent_id 转换为 agent_key 写入 Team YAML 的 members 列表

#### Scenario: Team 关联 Graph 导出
- **WHEN** Team 的 definition_json 中 linked_graph_id 非空
- **THEN** 系统 SHALL 将关联的 GraphDefinition 一并导出到 graphs/ 目录

#### Scenario: Team 内嵌 Graph 提取
- **WHEN** Team 的 definition_json 中包含 EmbeddedGraphSpec
- **THEN** 系统 SHALL 将内嵌图定义提取到 Team YAML 的 graph 字段，节点中的 agent_id 转换为 agent_key

### Requirement: 整行业导出
系统 SHALL 支持导出整个行业场景为 .arpack 文件，kind 为 "industry"。

#### Scenario: 从 Taxonomy 树反查关联实体
- **WHEN** 用户请求导出行业（指定 industry key）
- **THEN** 系统 SHALL 查找该 industry 下所有 position 节点，再查找所有关联这些 position 的 Agent，再查找所有包含这些 Agent 的 Team，以及 Team 关联的 Graph

#### Scenario: Agent 去重
- **WHEN** 多个 Team 引用同一个 Agent
- **THEN** 系统 SHALL 在 Pack 的 agents/ 目录中只写入一份 Agent 定义，Team 通过 agent_key 引用

#### Scenario: Taxonomy 树完整导出
- **WHEN** 导出整行业
- **THEN** 系统 SHALL 将完整的 industry→department→position 三级树写入 taxonomy.yaml

### Requirement: 导出时 Skill 依赖收集
系统 SHALL 在导出时收集 Agent 引用的 Skill slug 列表，写入 manifest.yaml 的 dependencies.skills 字段。

#### Scenario: Skill slug 收集
- **WHEN** Agent 的 SkillRuntimeJSON 中 AllowedSlugs 包含 `["code-review", "web-search"]`
- **THEN** manifest.yaml 的 dependencies.skills SHALL 包含 `code-review` 和 `web-search`

### Requirement: 导出时 FuncRef 依赖收集
系统 SHALL 在导出时收集 Graph 节点引用的 func_ref 列表，写入 manifest.yaml 的 dependencies.func_refs 字段。

#### Scenario: FuncRef 收集
- **WHEN** Graph 节点中 func_ref 包含 `aranea://func/generate`
- **THEN** manifest.yaml 的 dependencies.func_refs SHALL 包含 `aranea://func/generate`
