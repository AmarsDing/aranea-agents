## ADDED Requirements

### Requirement: Pack physical format
系统 SHALL 使用 `.arpack` 作为场景包文件扩展名，物理格式为 tar.gz 压缩包。

#### Scenario: 有效的 .arpack 文件结构
- **WHEN** 解压一个有效的 .arpack 文件
- **THEN** 根目录包含 `manifest.yaml`，以及可选的 `taxonomy.yaml`、`agents/` 目录、`teams/` 目录、`graphs/` 目录

#### Scenario: 无效的压缩格式
- **WHEN** 尝试读取一个非 tar.gz 格式的文件作为 .arpack
- **THEN** 系统 SHALL 返回格式校验错误

### Requirement: Pack 安全防护
系统 SHALL 在读取 .arpack 文件时实施多层安全防护。

#### Scenario: 大小限制
- **WHEN** 读取 .arpack 文件
- **THEN** 系统 SHALL 限制原始文件大小不超过 200MB（`MaxPackSize`）、解压后总大小不超过 200MB（`MaxTotalSize`）、单条目大小不超过 10MB（`MaxEntrySize`）、条目数不超过 1000（`MaxTarEntries`）

#### Scenario: 路径遍历防护
- **WHEN** 解析 tar 条目路径
- **THEN** 系统 SHALL 拒绝包含 `..` 或以 `/` 开头的路径，防止路径遍历攻击

#### Scenario: 符号链接跳过
- **WHEN** 遇到 TypeSymlink 或 TypeLink 类型的 tar 条目
- **THEN** 系统 SHALL 跳过该条目，不读取其内容

#### Scenario: gzip 炸弹防护
- **WHEN** 解压 gzip 数据
- **THEN** 系统 SHALL 使用 `io.LimitReader` 限制解压大小，防止解压炸弹攻击

### Requirement: ReadPackFromFS 从 embed.FS 读取
系统 SHALL 支持从 `fs.FS`（如 `embed.FS`）读取 .arpack 目录结构。

#### Scenario: 从 embed.FS 读取内置模板
- **WHEN** 调用 `ReadPackFromFS(builtinTemplatesFS, "scenario/packs/builtin-templates")`
- **THEN** 系统 SHALL 遍历目录结构，解析 manifest.yaml、taxonomy.yaml、agents/*.yaml、graphs/*.yaml 等，构建与 `ReadPack` 相同的内存模型

#### Scenario: embed.FS 与 tar.gz 格式一致性
- **WHEN** 同一 Pack 数据以 tar.gz 和目录结构两种形式存储
- **THEN** 通过 `ReadPack` 和 `ReadPackFromFS` 读取后 SHALL 产生相同的内存模型

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
- **THEN** SHALL 包含必填字段：`key`、`display_name`；可选字段包括：`description`、`icon`、`position_key`（taxonomy 路径格式）、`variant`、`variant_description`、`provider`、`model`、`model_tier`、`system_prompt_mode`、`context_window`、`tools_profile`、`tools_allow`、`tools_deny`、`tools_parallel`、`skills`（含 `allowed`、`denied`、`load_mode`）、`subagents_enabled`、`subagents_max_concurrency`、`subagents_max_generation_depth`、`code_executor`、`runtime`（含 `memory`、`tools`、`evolution`、`reasoning`、`ralph_loop`、`context` 子域）、`files`、`team_role`、`kind`（llm | a2a_proxy）、`a2a_proxy`

#### Scenario: Agent 文件目录
- **WHEN** Agent YAML 中 files 列表包含 `IDENTITY.md`
- **THEN** 系统 SHALL 从 `agents/<agent-key>/IDENTITY.md` 读取文件内容

#### Scenario: position_key 路径格式
- **WHEN** Agent YAML 中 position_key 值为 `finance/quant_trading/quant_researcher`
- **THEN** 系统 SHALL 将其解析为 industry=finance、department=quant_trading、position=quant_researcher 的三级路径

#### Scenario: Agent kind 默认值
- **WHEN** Agent YAML 中未指定 kind 字段
- **THEN** 系统 SHALL 默认 kind 为 `"llm"`

#### Scenario: A2A Proxy Agent
- **WHEN** Agent YAML 中 kind 为 `"a2a_proxy"` 且包含 `a2a_proxy` 字段
- **THEN** 系统 SHALL 解析 `a2a_proxy` 子字段：`remote_url`（必填）、`agent_card_url`、`enable_streaming`、`auth_type`、`timeout_seconds`

### Requirement: Team YAML schema
系统 SHALL 支持 `teams/<team-key>.yaml` 定义 Team 完整配置。

#### Scenario: Team YAML 必填字段
- **WHEN** 解析 Team YAML
- **THEN** SHALL 包含必填字段：`key`、`display_name`、`mode`；可选字段包括：`description`、`max_concurrency`、`timeout_seconds`、`run_timeout_sec`、`turn_timeout_sec`、`first_byte_timeout_sec`、`loop_max_iter`、`enable_checkpoint`、`runtime_engine`、`team_graph_runtime`、`members`（含 `agent_key`、`role`、`name`、`task_prompt`、`enabled`、`sort_order`）、`intent_anchor_key`、`synthesizer_key`、`graph`（含 `linked`、`layout`、`nodes`、`edges`、`linked_graph_id`）、`failure_policy`、`critic_loop`

#### Scenario: Team 成员通过 agent_key 引用
- **WHEN** Team YAML 中 members 列表包含 `{agent_key: go-senior-architect, role: orchestrator}`
- **THEN** 系统 SHALL 在导入时将 agent_key 解析为目标系统中的 agent_id

#### Scenario: Team Graph 节点扩展字段
- **WHEN** Team YAML 中 graph.nodes 列表包含节点定义
- **THEN** 每个节点 SHALL 可包含 `id`、`type`、`label`、`agent_key`、`role`、`interrupt_before`、`interrupt_after`、`destinations`、`retry_max_attempts`、`fallback_agent`

#### Scenario: Team Graph 边扩展字段
- **WHEN** Team YAML 中 graph.edges 列表包含边定义
- **THEN** 每条边 SHALL 可包含 `id`、`source`、`target`、`label`、`condition`

### Requirement: Graph YAML schema
系统 SHALL 支持 `graphs/<graph-id>.yaml` 定义 Graph 模板。

#### Scenario: Graph YAML 字段
- **WHEN** 解析 Graph YAML
- **THEN** SHALL 包含字段：`id`、`name`、`description`、`category`、`entry_point`、`finish_point`、`execution_engine`、`enable_checkpoint`、`version`、`sort_order`、`state_fields`（含 `name`、`type`、`reducer`、`default_value`、`required`、`disable_deep_copy`）、`nodes`、`edges`（含 `from`、`to`、`kind`）、`conditional_edges`、`subgraphs`、`interrupt_before`、`interrupt_after`

#### Scenario: Graph 节点扩展字段
- **WHEN** Graph YAML 中 nodes 列表包含节点定义
- **THEN** 每个节点 SHALL 可包含 `id`、`type`、`label`、`description`、`func_ref`、`instruction`、`model_name`、`tool_names`、`agent_key`、`interrupt_before`、`interrupt_after`、`destinations`、`retry_max_attempts`、`failure_action`、`fallback_agent`、`input_mapper_json`、`output_mapper_json`、`isolated_messages`、`input_from_last_response`、`cache_enabled`、`cache_ttl_seconds`

#### Scenario: Graph conditional edges
- **WHEN** Graph YAML 包含 `conditional_edges` 列表
- **THEN** 每个条件边 SHALL 包含 `from`、`cond_func_ref`（可选）、`path_map`（map[string]string）

#### Scenario: Graph subgraphs
- **WHEN** Graph YAML 包含 `subgraphs` 列表
- **THEN** 每个子图 SHALL 包含 `id`、`name`、`entry_point`、`finish_point`、`nodes`、`edges`

#### Scenario: Graph node agent_key 引用
- **WHEN** Graph 节点 YAML 中包含 `agent_key` 字段
- **THEN** 系统 SHALL 在导入时将 agent_key 映射为目标系统中的 agent_id

### Requirement: 实体间引用使用业务 key
系统 SHALL 要求 Pack 内所有实体间引用使用业务 key 而非数据库 UUID。

#### Scenario: Team 引用 Agent 使用 agent_key
- **WHEN** Team YAML 中成员定义包含 `agent_key` 字段
- **THEN** 系统 SHALL 在导入时将 agent_key 映射为目标系统中的 agent_id

#### Scenario: Agent 引用 Taxonomy 使用路径格式
- **WHEN** Agent YAML 中 position_key 值为 `finance/quant_trading/quant_researcher`
- **THEN** 系统 SHALL 在导入时将路径解析为 taxonomy_position_id
