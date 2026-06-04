# Pack Import Export

## Pack API

### Requirement: Pack Proto 定义
系统 SHALL 在 `api/kratos/pack/v1/pack.proto` 中定义 Pack 服务和消息类型，使用 unary RPC + bytes 传输。

#### Scenario: Proto 文件生成
- **WHEN** 运行 `make api`
- **THEN** pack.proto SHALL 正确生成 Go 代码，包含 ExportPack/ImportPack/ValidatePack 三个 unary RPC 方法

#### Scenario: RPC 方法签名
- **GIVEN** PackService 定义
- **THEN** 三个 RPC 方法 SHALL 为 unary（非流式）：
  - `rpc ExportPack(ExportPackRequest) returns (ExportPackResponse)`
  - `rpc ImportPack(ImportPackRequest) returns (ImportPackResponse)`
  - `rpc ValidatePack(ValidatePackRequest) returns (ValidatePackResponse)`

### Requirement: Export Pack API
系统 SHALL 提供 `POST /v1/pack/export` 端点，支持导出场景包。

#### Scenario: 导出单 Agent
- **WHEN** 请求 `{kind: "agent", ref: "<agent_id>"}`
- **THEN** 系统 SHALL 返回 ExportPackResponse，data 字段包含 .arpack tar.gz 字节，kind 为 "agent"

#### Scenario: 导出单 Team
- **WHEN** 请求 `{kind: "team", ref: "<team_id>"}`
- **THEN** 系统 SHALL 返回 ExportPackResponse，data 字段包含 .arpack tar.gz 字节，kind 为 "team"，包含成员 Agent

#### Scenario: 导出整行业
- **WHEN** 请求 `{kind: "industry", ref: "finance"}`
- **THEN** 系统 SHALL 返回 ExportPackResponse，data 字段包含 .arpack tar.gz 字节，kind 为 "industry"，包含完整 Taxonomy 树、所有关联 Agent/Team/Graph

#### Scenario: 导出不存在的实体
- **WHEN** 请求导出不存在的 agent_id
- **THEN** 系统 SHALL 返回 404 错误

#### Scenario: ExportPackRequest 消息结构
- **GIVEN** ExportPackRequest 定义
- **THEN** SHALL 包含字段：`kind`（string, REQUIRED，agent/team/industry）、`ref`（string, REQUIRED，实体 ID 或 key）

#### Scenario: ExportPackResponse 消息结构
- **GIVEN** ExportPackResponse 定义
- **THEN** SHALL 包含字段：`data`（bytes，.arpack tar.gz 内容）、`name`（string，manifest name）、`kind`（string，manifest kind）

### Requirement: Import Pack API
系统 SHALL 提供 `POST /v1/pack/import` 端点，支持导入场景包。

#### Scenario: 成功导入
- **WHEN** 上传有效的 .arpack 文件（bytes），指定 conflict_strategy 为 "skip"
- **THEN** 系统 SHALL 执行四阶段导入，返回 ImportPackResponse 包含导入统计

#### Scenario: 导入无效文件
- **WHEN** 上传非 .arpack 格式的文件
- **THEN** 系统 SHALL 返回 400 错误，描述格式校验失败原因

#### Scenario: ImportPackRequest 消息结构
- **GIVEN** ImportPackRequest 定义
- **THEN** SHALL 包含字段：`data`（bytes, REQUIRED，.arpack tar.gz 内容）、`conflict_strategy`（string，skip/overwrite/duplicate）

#### Scenario: ImportPackResponse 消息结构
- **GIVEN** ImportPackResponse 定义
- **THEN** SHALL 包含字段：`taxonomy_nodes`（int32）、`agents_created`（int32）、`agents_updated`（int32）、`agents_skipped`（int32）、`graphs_created`（int32）、`teams_created`（int32）、`teams_updated`（int32）、`teams_skipped`（int32）、`conflict_strategy`（string）、`failures`（repeated ImportFailure）

#### Scenario: ImportFailure 消息结构
- **GIVEN** ImportFailure 定义
- **THEN** SHALL 包含字段：`entity_type`（string）、`key`（string）、`reason`（string）

### Requirement: Validate Pack API
系统 SHALL 提供 `POST /v1/pack/validate` 端点，支持 dry-run 校验。

#### Scenario: 校验通过
- **WHEN** 上传有效的 .arpack 文件进行校验
- **THEN** 系统 SHALL 返回 ValidatePackResponse：`{valid: true, errors: [], missing_skills: [...], missing_func_refs: [...], conflicts: [...]}`

#### Scenario: 校验发现阻断问题
- **WHEN** 校验发现缺失的 func_ref
- **THEN** 系统 SHALL 返回 `{valid: false, ...}` 并列出阻断项

#### Scenario: ValidatePackRequest 消息结构
- **GIVEN** ValidatePackRequest 定义
- **THEN** SHALL 包含字段：`data`（bytes, REQUIRED，.arpack tar.gz 内容）

#### Scenario: ValidatePackResponse 消息结构
- **GIVEN** ValidatePackResponse 定义
- **THEN** SHALL 包含字段：`valid`（bool）、`errors`（repeated string，阻断性错误）、`warnings`（repeated string，非阻断性警告）、`missing_skills`（repeated string）、`missing_func_refs`（repeated string）、`conflicts`（repeated ConflictItem）

#### Scenario: ConflictItem 消息结构
- **GIVEN** ConflictItem 定义
- **THEN** SHALL 包含字段：`entity_type`（string）、`key`（string）

### Requirement: Pack CLI 命令
系统 SHALL 提供 `aranea pack` CLI 子命令。

#### Scenario: CLI 导出
- **WHEN** 执行 `aranea pack export --kind agent --id <id> -o output.arpack`
- **THEN** 系统 SHALL 生成 .arpack 文件到指定路径

#### Scenario: CLI 导入
- **WHEN** 执行 `aranea pack import -f input.arpack --conflict-strategy overwrite`
- **THEN** 系统 SHALL 执行导入并输出结果报告

#### Scenario: CLI 校验
- **WHEN** 执行 `aranea pack validate -f input.arpack`
- **THEN** 系统 SHALL 输出校验结果，不实际写入数据库

### Requirement: PackRepoAdapter 组合适配器
系统 SHALL 提供 `PackRepoAdapter`（`internal/data/pack_repo.go`），组合 ExporterRepo + ImporterRepo + ValidatorRepo 三个接口。

#### Scenario: 接口满足验证
- **GIVEN** PackRepoAdapter 定义
- **THEN** SHALL 通过编译期断言满足 `pack.ExporterRepo`、`pack.ImporterRepo`、`pack.ValidatorRepo` 三个接口

#### Scenario: 构造方式
- **GIVEN** NewPackRepoAdapter 函数
- **THEN** SHALL 接受 `biz.AgentRepository`、`biz.TeamRepository`、`biz.TaxonomyRepo`、`biz.GraphRepo` 四个参数

## Pack Export

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

## Pack Format

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

## Pack Import

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

### Requirement: 导入使用 ORM 路径写入
系统 SHALL 通过 biz 层 Usecase 的 ORM 路径写入数据库，不使用 RawSQL。

#### Scenario: Agent 写入通过 AgentUsecase
- **WHEN** 导入 Agent
- **THEN** 系统 SHALL 调用 `AgentUsecase.Create` 或 `AgentUsecase.Update`，确保 biz 层校验和事件触发

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
