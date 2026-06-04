## ADDED Requirements

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
- **THEN** 系统 SHALL 返回 ValidatePackResponse：`{valid: true, errors: [], warnings: [], missing_skills: [...], missing_func_refs: [...], conflicts: [...]}`

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
- **THEN** SHALL 接受 `biz.AgentRepository`、`biz.TeamRepository`、`biz.TaxonomyRepo`、`biz.GraphRepo`、`biz.SkillLookupReader` 五个参数
