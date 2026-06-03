## ADDED Requirements

### Requirement: Export Pack API
系统 SHALL 提供 `POST /v1/packs/export` 端点，支持导出场景包。

#### Scenario: 导出单 Agent
- **WHEN** 请求 `{kind: "agent", id: "<agent_id>"}`
- **THEN** 系统 SHALL 返回 .arpack 文件流，kind 为 "agent"

#### Scenario: 导出单 Team
- **WHEN** 请求 `{kind: "team", id: "<team_id>"}`
- **THEN** 系统 SHALL 返回 .arpack 文件流，kind 为 "team"，包含成员 Agent

#### Scenario: 导出整行业
- **WHEN** 请求 `{kind: "industry", key: "finance"}`
- **THEN** 系统 SHALL 返回 .arpack 文件流，kind 为 "industry"，包含完整 Taxonomy 树、所有关联 Agent/Team/Graph

#### Scenario: 导出不存在的实体
- **WHEN** 请求导出不存在的 agent_id
- **THEN** 系统 SHALL 返回 404 错误

### Requirement: Import Pack API
系统 SHALL 提供 `POST /v1/packs/import` 端点，支持导入场景包。

#### Scenario: 成功导入
- **WHEN** 上传有效的 .arpack 文件，指定 conflict_strategy 为 "skip"
- **THEN** 系统 SHALL 执行四阶段导入，返回导入结果报告

#### Scenario: 导入无效文件
- **WHEN** 上传非 .arpack 格式的文件
- **THEN** 系统 SHALL 返回 400 错误，描述格式校验失败原因

### Requirement: Validate Pack API
系统 SHALL 提供 `POST /v1/packs/validate` 端点，支持 dry-run 校验。

#### Scenario: 校验通过
- **WHEN** 上传有效的 .arpack 文件进行校验
- **THEN** 系统 SHALL 返回 `{valid: true, conflicts: [...], missing_skills: [...], missing_func_refs: [...]}`

#### Scenario: 校验发现阻断问题
- **WHEN** 校验发现缺失的 func_ref
- **THEN** 系统 SHALL 返回 `{valid: false, ...}` 并列出阻断项

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

### Requirement: Pack Proto 定义
系统 SHALL 在 `api/kratos/pack/v1/pack.proto` 中定义 Pack 服务和消息类型。

#### Scenario: Proto 文件生成
- **WHEN** 运行 `make api`
- **THEN** pack.proto SHALL 正确生成 Go 代码，包含 ExportPack/ImportPack/ValidatePack RPC 方法
