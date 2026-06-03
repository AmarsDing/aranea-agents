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

### Requirement: 四阶段导入写入
系统 SHALL 按依赖顺序分四阶段写入：Taxonomy → Agent → Graph → Team。

#### Scenario: Phase 1 — Taxonomy 写入
- **WHEN** Pack 包含 taxonomy.yaml
- **THEN** 系统 SHALL 按 industry → department → position 顺序 upsert 分类节点，维护 parent_id 关系

#### Scenario: Phase 2 — Agent 写入
- **WHEN** Pack 包含 agents/ 目录
- **THEN** 系统 SHALL 为每个 Agent 创建或更新记录，包含 Files 和 RuntimeSettings；position_key 路径解析为 taxonomy_position_id

#### Scenario: Phase 3 — Graph 写入
- **WHEN** Pack 包含 graphs/ 目录
- **THEN** 系统 SHALL 为每个 Graph 模板创建新的 GraphDefinition 记录

#### Scenario: Phase 4 — Team 写入
- **WHEN** Pack 包含 teams/ 目录
- **THEN** 系统 SHALL 为每个 Team 创建或更新记录，members 中的 agent_key 映射为 agent_id，graph 引用映射为新创建的 graph_id

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

#### Scenario: overwrite 策略
- **WHEN** 冲突策略为 overwrite 且目标系统已存在相同 agent_key 的 Agent
- **THEN** 系统 SHALL 更新已有 Agent 的可修改字段，保留原 ID 和 created_at

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
- **THEN** 系统 SHALL 返回导入统计：创建的 Agent 数、更新的 Agent 数、创建的 Team 数、创建的 Graph 数、创建的 Taxonomy 节点数

#### Scenario: 部分失败报告
- **WHEN** 导入过程中部分实体写入失败
- **THEN** 系统 SHALL 返回已成功的统计和失败实体列表（包含 key、类型、错误原因）

### Requirement: 导入使用 ORM 路径写入
系统 SHALL 通过 biz 层 Usecase 的 ORM 路径写入数据库，不使用 RawSQL。

#### Scenario: Agent 写入通过 AgentUsecase
- **WHEN** 导入 Agent
- **THEN** 系统 SHALL 调用 `AgentUsecase.Create` 或 `AgentUsecase.Update`，确保 biz 层校验和事件触发
