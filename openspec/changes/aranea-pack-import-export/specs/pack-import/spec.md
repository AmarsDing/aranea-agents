## ADDED Requirements

### Requirement: Pack 校验（validate）
系统 SHALL 支持对 .arpack 文件进行 dry-run 校验，不实际写入数据库。

#### Scenario: 格式校验通过
- **WHEN** 用户提交 .arpack 文件进行校验
- **THEN** 系统 SHALL 验证 manifest.yaml 格式正确、api_version 为 "v1"、各实体 YAML 格式正确

#### Scenario: Skill 依赖缺失报告
- **WHEN** manifest.yaml 中 dependencies.skills 包含目标系统不存在的 slug
- **THEN** 校验结果 SHALL 报告缺失的 Skill slug 列表，但不阻断导入

#### Scenario: FuncRef 依赖缺失报告
- **WHEN** manifest.yaml 中 dependencies.func_refs 包含目标系统未注册的函数
- **THEN** 校验结果 SHALL 报告缺失的 func_ref 列表，并标记为阻断项

#### Scenario: 冲突预检
- **WHEN** Pack 中的 agent_key 与目标系统已有 Agent 冲突
- **THEN** 校验结果 SHALL 报告冲突的实体列表，包含 key、类型和冲突详情

#### Scenario: 引用完整性校验
- **WHEN** Pack 中 Graph 节点或 Team 内嵌 Graph 节点引用的 agent_key 不在 Pack 的 Agents 列表中
- **THEN** 校验结果 SHALL 报告引用错误，包含 Graph ID、节点 ID 和缺失的 agent_key

### Requirement: ValidatorRepo 接口
系统 SHALL 定义 `ValidatorRepo` 接口（`internal/biz/pack/validator.go`），提供校验引擎所需的只读仓库方法。

#### Scenario: ValidatorRepo 方法列表
- **GIVEN** ValidatorRepo 接口定义
- **THEN** SHALL 包含以下方法：
  - `AgentKeyExists(ctx, agentKey) (bool, error)` — 检查 agent_key 是否已存在
  - `TeamKeyExists(ctx, teamKey) (bool, error)` — 检查 team_key 是否已存在
  - `TaxonomyKeyExists(ctx, key) (bool, error)` — 检查 taxonomy key 是否已存在
  - `SkillExists(ctx, slug) (bool, error)` — 检查 Skill slug 是否可用
  - `FuncRefExists(funcRef) bool` — 检查 func_ref 是否在注册表中（无 ctx，纯内存查询）

### Requirement: 四阶段导入写入
系统 SHALL 按依赖顺序分四阶段写入：Taxonomy → Agent → Graph → Team。

#### Scenario: Phase 1 — Taxonomy 写入
- **WHEN** Pack 包含 taxonomy.yaml
- **THEN** 系统 SHALL 按 industry → department → position 顺序 upsert 分类节点，维护 parent_id 关系

#### Scenario: Phase 2 — Agent 写入
- **WHEN** Pack 包含 agents/ 目录
- **THEN** 系统 SHALL 为每个 Agent 创建或更新记录，包含 Files 和 RuntimeSettings；position_key 路径解析为 taxonomy_position_id；Agent 的 `Readonly` 字段 SHALL 默认为 `false`；Agent 的 `Source` 字段 SHALL 设为 `"imported"`

#### Scenario: Phase 3 — Graph 写入
- **WHEN** Pack 包含 graphs/ 目录
- **THEN** 系统 SHALL 为每个 Graph 模板创建新的 GraphDefinition 记录

#### Scenario: Phase 4 — Team 写入
- **WHEN** Pack 包含 teams/ 目录
- **THEN** 系统 SHALL 为每个 Team 创建或更新记录，members 中的 agent_key 映射为 agent_id，graph 引用映射为新创建的 graph_id

### Requirement: ImporterRepo 接口
系统 SHALL 定义 `ImporterRepo` 接口（`internal/biz/pack/importer.go`），提供导入引擎所需的写入仓库方法。

#### Scenario: ImporterRepo 方法列表
- **GIVEN** ImporterRepo 接口定义
- **THEN** SHALL 包含以下方法：
  - **Taxonomy**：`CreateTaxonomyNode`、`UpdateTaxonomyNode`、`GetTaxonomyNodeByKey`、`ListTaxonomyNodesByParentID`
  - **Agent**：`GetAgentByAgentKey`、`CreateAgent`、`UpdateAgent`、`GetAgentRuntimeSettings`、`UpsertAgentRuntimeSettings`、`ReplaceAgentPromptFiles`
  - **Team**：`GetTeamByID`、`GetTeamByKey`、`CreateTeam`、`UpdateTeam`
  - **Graph**：`SaveGraphDefinition`

#### Scenario: GetTeamByKey 方法
- **GIVEN** ImporterRepo 接口
- **THEN** SHALL 包含 `GetTeamByKey(ctx, teamKey)` 方法，用于 Team overwrite 策略中按 key 查找已有 Team

### Requirement: Key→ID 映射
系统 SHALL 在导入过程中维护 key→ID 映射表，用于跨实体引用解析。

#### Scenario: agent_key → agent_id 映射
- **WHEN** 导入 Agent 成功后
- **THEN** 系统 SHALL 记录 `agent_key → agent_id` 映射，供后续 Team 成员引用解析使用

#### Scenario: taxonomy_key 路径 → taxonomy_position_id 映射
- **WHEN** 导入 Taxonomy 成功后
- **THEN** 系统 SHALL 记录 `industry/dept/pos → position_id` 映射，供 Agent 的 position_key 解析使用

#### Scenario: graph_id 映射
- **WHEN** 导入 Graph 创建新记录后
- **THEN** 系统 SHALL 记录 `原始graph_id → 新graph_id` 映射，供 Team 的 linked_graph_id 解析使用

### Requirement: 冲突策略
系统 SHALL 支持三种冲突策略：skip、overwrite、duplicate。

#### Scenario: skip 策略
- **WHEN** 冲突策略为 skip 且目标系统已存在相同 agent_key 的 Agent
- **THEN** 系统 SHALL 跳过该 Agent，不修改已有记录，但仍将其 agent_id 加入映射表

#### Scenario: overwrite 策略（Agent）
- **WHEN** 冲突策略为 overwrite 且目标系统已存在相同 agent_key 的 Agent
- **THEN** 系统 SHALL 更新已有 Agent 的可修改字段，保留原 ID 和 created_at

#### Scenario: overwrite 策略（Team）
- **WHEN** 冲突策略为 overwrite 且目标系统已存在相同 team_key 的 Team
- **THEN** 系统 SHALL 通过 `GetTeamByKey` 查找已有 Team，更新其 definition_json 和其他可修改字段，保留原 ID

#### Scenario: duplicate 策略
- **WHEN** 冲突策略为 duplicate 且目标系统已存在相同 agent_key 的 Agent
- **THEN** 系统 SHALL 生成新的 agent_key（如 `go-senior-general-copy`），创建新的 Agent 记录

#### Scenario: 默认冲突策略
- **WHEN** 用户未指定冲突策略
- **THEN** 系统 SHALL 使用 skip 作为默认策略

### Requirement: 导入结果报告
系统 SHALL 在导入完成后返回结果报告。

#### Scenario: 成功导入报告
- **WHEN** 导入完成
- **THEN** 系统 SHALL 返回导入统计：创建的 Agent 数、更新的 Agent 数、跳过的 Agent 数、创建的 Team 数、更新的 Team 数、跳过的 Team 数、创建的 Graph 数、创建的 Taxonomy 节点数

#### Scenario: 部分失败报告
- **WHEN** 导入过程中部分实体写入失败
- **THEN** 系统 SHALL 返回已成功的统计和失败实体列表（包含 entity_type、key、reason）

### Requirement: 导入使用 ImporterRepo 直接写入
系统 SHALL 通过 `ImporterRepo` 接口直接调用 data 层 Repo 写入数据库，不经过 biz 层 Usecase。

#### Scenario: Agent 写入通过 ImporterRepo
- **WHEN** 导入 Agent
- **THEN** 系统 SHALL 调用 `ImporterRepo.CreateAgent` 或 `ImporterRepo.UpdateAgent`，不经过 `AgentUsecase`

#### Scenario: 不触发 Usecase 业务校验
- **WHEN** Pack 导入写入实体
- **THEN** 系统 SHALL 不触发 Usecase 层的业务校验和事件触发逻辑

### Requirement: JSON 序列化工具函数
系统 SHALL 使用 `encoding/json` 进行 slice ↔ JSON list 的序列化/反序列化。

#### Scenario: sliceToJSONList
- **WHEN** 将 `[]string` 转为 JSON 数组字符串
- **THEN** 系统 SHALL 使用 `json.Marshal` 进行序列化（非手动拼接）

#### Scenario: jsonListToSlice
- **WHEN** 将 JSON 数组字符串转为 `[]string`
- **THEN** 系统 SHALL 使用 `json.Unmarshal` 进行反序列化（非 yaml.Unmarshal）

#### Scenario: parseSkillRuntime
- **WHEN** 解析 SkillRuntimeJSON 字段
- **THEN** 系统 SHALL 使用 `json.Unmarshal` 解析 `{allowed_slugs: [...], denied_slugs: [...]}` 结构

### Requirement: overwrite 策略保留原始元数据
系统 SHALL 在 overwrite 策略下保留原始记录的 Status、Readonly、Source 字段。

#### Scenario: Agent overwrite 保留元数据
- **WHEN** 冲突策略为 overwrite 且目标系统已存在相同 agent_key 的 Agent
- **THEN** 系统 SHALL 保留已有 Agent 的 Status、Readonly、Source 字段，不使用 Pack 中的默认值覆盖

#### Scenario: Team overwrite 保留元数据
- **WHEN** 冲突策略为 overwrite 且目标系统已存在相同 team_key 的 Team
- **THEN** 系统 SHALL 保留已有 Team 的 Status、Readonly、Source 字段

### Requirement: duplicate 策略下原始 key 映射
系统 SHALL 在 duplicate 策略下同时注册原始 key 和新 key 到 ID 的映射。

#### Scenario: Agent duplicate 注册双映射
- **WHEN** 冲突策略为 duplicate 且 Agent 被创建为新 key（如 `go-senior-general-copy`）
- **THEN** 系统 SHALL 同时注册 `go-senior-general → 新ID` 和 `go-senior-general-copy → 新ID`，确保后续 Team/Graph 引用能通过原始 key 解析

### Requirement: 导入结果包含 warnings
系统 SHALL 在导入结果中包含警告信息列表。

#### Scenario: Taxonomy 导入部分失败产生 warnings
- **WHEN** Taxonomy 导入过程中某个部门或岗位写入失败
- **THEN** 系统 SHALL 将失败信息记录为 warning 而非 error，继续导入后续节点
