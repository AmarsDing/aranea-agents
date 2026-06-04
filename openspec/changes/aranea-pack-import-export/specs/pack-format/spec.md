## ADDED Requirements

### Requirement: Pack physical format
系统 SHALL 使用 `.arpack` 作为场景包文件扩展名，物理格式为 tar.gz 压缩包。

#### Scenario: 有效的 .arpack 文件结构
- **WHEN** 解压一个有效的 .arpack 文件
- **THEN** 根目录包含 `manifest.yaml`，以及可选的 `taxonomy.yaml`、`agents/` 目录、`teams/` 目录、`graphs/` 目录

#### Scenario: 无效的压缩格式
- **WHEN** 尝试读取一个非 tar.gz 格式的文件作为 .arpack
- **THEN** 系统 SHALL 返回格式校验错误

### Requirement: Manifest YAML schema
系统 SHALL 要求每个 .arpack 包含 `manifest.yaml`，定义包的元数据。

#### Scenario: 完整的 manifest 字段
- **WHEN** 解析 manifest.yaml
- **THEN** SHALL 包含以下必填字段：`api_version`（值为 "v1"）、`kind`（值为 "agent"|"team"|"industry"）、`name`、`version`；以及可选字段：`description`、`author`、`created_at`、`dependencies`、`contents`

#### Scenario: 缺少必填字段
- **WHEN** manifest.yaml 缺少 api_version 或 kind 字段
- **THEN** 系统 SHALL 返回校验错误，拒绝导入

### Requirement: Taxonomy YAML schema
系统 SHALL 支持 `taxonomy.yaml` 定义行业分类树，格式与现有 `taxonomy.yaml` 一致（industry → department → position 三级树）。

#### Scenario: 完整的三级分类树
- **WHEN** 解析 taxonomy.yaml
- **THEN** 每个 industry 包含 key、name、icon、description、sort_order，以及 departments 数组；每个 department 包含 positions 数组

#### Scenario: taxonomy.yaml 可选
- **WHEN** Pack 的 kind 为 "agent" 且不包含 taxonomy.yaml
- **THEN** 系统 SHALL 允许导入，Agent 的 position_key 引用已有的分类节点

### Requirement: Agent YAML schema
系统 SHALL 支持 `agents/<agent-key>.yaml` 定义 Agent 完整配置。

#### Scenario: Agent YAML 必填字段
- **WHEN** 解析 Agent YAML
- **THEN** SHALL 包含必填字段：`key`、`display_name`；可选字段包括：`description`、`icon`、`position_key`（taxonomy 路径格式）、`variant`、`variant_description`、`provider`、`model`、`model_tier`、`system_prompt_mode`、`context_window`、`tools_profile`、`tools_allow`、`tools_deny`、`tools_parallel`、`skills`、`subagents_enabled`、`code_executor`、`runtime`、`files`、`team_role`

#### Scenario: Agent 文件目录
- **WHEN** Agent YAML 中 files 列表包含 `IDENTITY.md`
- **THEN** 系统 SHALL 从 `agents/<agent-key>/IDENTITY.md` 读取文件内容

#### Scenario: position_key 路径格式
- **WHEN** Agent YAML 中 position_key 值为 `finance/quant_trading/quant_researcher`
- **THEN** 系统 SHALL 将其解析为 industry=finance、department=quant_trading、position=quant_researcher 的三级路径

### Requirement: Team YAML schema
系统 SHALL 支持 `teams/<team-key>.yaml` 定义 Team 完整配置。

#### Scenario: Team YAML 必填字段
- **WHEN** 解析 Team YAML
- **THEN** SHALL 包含必填字段：`key`、`display_name`、`mode`；可选字段包括：`description`、`max_concurrency`、`timeout_seconds`、`loop_max_iter`、`enable_checkpoint`、`members`、`intent_anchor_key`、`synthesizer_key`、`graph`

#### Scenario: Team 成员通过 agent_key 引用
- **WHEN** Team YAML 中 members 列表包含 `{agent_key: go-senior-architect, role: orchestrator}`
- **THEN** 系统 SHALL 在导入时将 agent_key 解析为目标系统中的 agent_id

### Requirement: Graph YAML schema
系统 SHALL 支持 `graphs/<graph-id>.yaml` 定义 Graph 模板。

#### Scenario: Graph YAML 字段
- **WHEN** 解析 Graph YAML
- **THEN** SHALL 包含字段：`id`、`name`、`description`、`category`、`entry_point`、`finish_point`、`nodes`、`edges`、`state_fields`、`execution_engine`、`enable_checkpoint`

### Requirement: 实体间引用使用业务 key
系统 SHALL 要求 Pack 内所有实体间引用使用业务 key 而非数据库 UUID。

#### Scenario: Team 引用 Agent 使用 agent_key
- **WHEN** Team YAML 中成员定义包含 `agent_key` 字段
- **THEN** 系统 SHALL 在导入时将 agent_key 映射为目标系统中的 agent_id

#### Scenario: Agent 引用 Taxonomy 使用路径格式
- **WHEN** Agent YAML 中 position_key 值为 `finance/quant_trading/quant_researcher`
- **THEN** 系统 SHALL 在导入时将路径解析为 taxonomy_position_id
