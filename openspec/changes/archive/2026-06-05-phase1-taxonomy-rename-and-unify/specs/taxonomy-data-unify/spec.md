## ADDED Requirements

### Requirement: 废弃 industries/departments/positions 三表体系
系统 SHALL 删除 `industries`、`departments`、`positions` 三表体系的所有代码，统一使用 `industry_taxonomy` 单表作为唯一分类真相源。

#### Scenario: Ent Schema 删除
- **WHEN** 查看 `internal/data/ent/schema/` 目录
- **THEN** `industry.go`、`department.go`、`position.go` 文件 SHALL 不存在，Ent 重新生成后 `internal/data/ent/` 中 SHALL 无 Industry/Department/Position 相关生成代码

#### Scenario: Biz 层删除
- **WHEN** 查看 `internal/biz/` 目录
- **THEN** 以下文件 SHALL 不存在：`industry_types.go`、`industry_usecase.go`、`department_types.go`、`department_usecase.go`、`position_types.go`、`position_usecase.go`

#### Scenario: Data 层删除
- **WHEN** 查看 `internal/data/` 目录
- **THEN** 以下文件 SHALL 不存在：`industry_repo.go`、`department_repo.go`、`position_repo.go`、`seed_builtin_industries.go`

#### Scenario: Service 层删除
- **WHEN** 查看 `internal/service/` 目录
- **THEN** `industry.go` 文件 SHALL 不存在

#### Scenario: Proto 删除
- **WHEN** 查看 `api/kratos/` 目录
- **THEN** `api/kratos/industry/` 目录 SHALL 不存在

#### Scenario: CLI 删除
- **WHEN** 查看 `cmd/` 目录
- **THEN** `cmd/seed-industries/` 目录 SHALL 不存在

#### Scenario: Server 注册移除
- **WHEN** 查看 `internal/server/grpc.go` 和 `http.go`
- **THEN** `RegisterIndustryServiceServer` 和 `RegisterIndustryServiceHTTPServer` 注册 SHALL 被移除

#### Scenario: Service Registry 移除
- **WHEN** 查看 `internal/server/service_registry.go`
- **THEN** `Industry *service.IndustryService` 字段 SHALL 被移除

#### Scenario: ProviderSet 清理
- **WHEN** 查看 `internal/biz/biz.go`、`internal/data/data.go`、`internal/service/service.go`
- **THEN** `NewIndustryUsecase`、`NewDepartmentUsecase`、`NewPositionUsecase`、`NewIndustryRepo`、`NewDepartmentRepo`、`NewPositionRepo`、`NewIndustryService`、`SeedBuiltinIndustries` SHALL 从 ProviderSet 中移除

#### Scenario: Wire 依赖清理
- **WHEN** 运行 `make wire`
- **THEN** wire_gen.go SHALL 重新生成成功，不包含任何三表体系的依赖注入

### Requirement: IndustryService API 迁移到 TaxonomyService
前端行业市场页当前读 `IndustryService` API，SHALL 迁移到 `TaxonomyService` API。

#### Scenario: 行业列表 API 迁移
- **WHEN** 前端请求行业列表
- **THEN** 系统 SHALL 使用 `GET /v1/taxonomy?level=industry` 替代 `GET /v1/industries`

#### Scenario: 部门列表 API 迁移
- **WHEN** 前端请求某行业下的部门列表
- **THEN** 系统 SHALL 使用 `GET /v1/taxonomy?level=department&parent_id={id}` 替代 `GET /v1/industries/{key}/departments`

#### Scenario: 岗位列表 API 迁移
- **WHEN** 前端请求某部门下的岗位列表
- **THEN** 系统 SHALL 使用 `GET /v1/taxonomy?level=position&parent_id={id}` 替代 `GET /v1/departments/{key}/positions`

#### Scenario: 岗位 Prompt API 迁移
- **WHEN** 前端请求某岗位的 Prompt
- **THEN** 系统 SHALL 使用 `GET /v1/taxonomy/{id}/prompt` 替代 `GET /v1/positions/{key}/prompt`

#### Scenario: 岗位 Variants API 迁移
- **WHEN** 前端请求某岗位的 Variants
- **THEN** 系统 SHALL 使用 `GET /v1/taxonomy/{id}/variants` 替代 `GET /v1/positions/{key}/variants`

#### Scenario: 旧 Industry API 不可用
- **WHEN** 客户端请求 `GET /v1/industries`
- **THEN** 系统 SHALL 返回 404 Not Found

### Requirement: 前端行业市场页迁移
前端行业市场页 SHALL 从 `IndustryService` API 迁移到 `TaxonomyService` API。

#### Scenario: features/industries/api.ts 迁移
- **WHEN** 查看 `web/src/features/industries/api.ts`
- **THEN** `listIndustries()` SHALL 调用 `GET /v1/taxonomy?level=industry`，`listDepartments(industryKey)` SHALL 调用 `GET /v1/taxonomy?level=department&parent_id={id}`，`listPositions(departmentKey)` SHALL 调用 `GET /v1/taxonomy?level=position&parent_id={id}`，`getPositionPrompt(positionKey)` SHALL 调用 `GET /v1/taxonomy/{id}/prompt`，`listPositionVariants(positionKey)` SHALL 调用 `GET /v1/taxonomy/{id}/variants`

#### Scenario: IndustryMarketPage 迁移
- **WHEN** 查看 `web/src/pages/industries/IndustryMarketPage.vue`
- **THEN** 页面 SHALL 使用 taxonomy API 获取行业列表，数据格式 SHALL 适配 TaxonomyNode 响应

#### Scenario: IndustryDetailPage 迁移
- **WHEN** 查看 `web/src/pages/industries/IndustryDetailPage.vue`
- **THEN** 页面 SHALL 使用 taxonomy API 按 parent_id 级联获取部门和岗位数据

#### Scenario: IndustryPositionPicker 迁移
- **WHEN** 查看 `web/src/components/industries/IndustryPositionPicker.vue`
- **THEN** 组件 SHALL 使用 taxonomy API 获取岗位数据

### Requirement: taxonomy.yaml 新格式
`taxonomy.yaml` SHALL 支持丰富字段格式，包含 `responsibilities`、`skills_required`、`seniority_level`、`icon`、`variants` 等扩展字段。

#### Scenario: 行业级字段
- **WHEN** 查看 `taxonomy.yaml` 中的行业定义
- **THEN** 每个行业 SHALL 包含 `key`、`name`、`icon`、`description`、`sort_order` 字段

#### Scenario: 部门级字段
- **WHEN** 查看 `taxonomy.yaml` 中的部门定义
- **THEN** 每个部门 SHALL 包含 `key`、`name`、`description`、`sort_order`、`responsibilities`（字符串数组）字段

#### Scenario: 岗位级字段
- **WHEN** 查看 `taxonomy.yaml` 中的岗位定义
- **THEN** 每个岗位 SHALL 包含 `key`、`name`、`seniority_level`、`skills_required`（字符串数组）、`responsibilities`（字符串数组）、`variants`（对象数组，含 `key` 和 `name`）字段

#### Scenario: Variant 定义
- **WHEN** 查看 `taxonomy.yaml` 中的 variant 定义
- **THEN** 每个 variant SHALL 包含 `key` 和 `name` 字段，variant key SHALL 匹配正则 `^[a-z0-9_]+$`（仅小写字母、数字、下划线，禁用连字符）

### Requirement: 部门 key 对齐
`taxonomy.yaml` 中的部门 key SHALL 以 `seed-industries/main.go` 为准进行对齐。

#### Scenario: finance 部门 key 对齐
- **WHEN** 查看 `taxonomy.yaml` 中 finance 行业的部门定义
- **THEN** 以下 key 对齐 SHALL 生效：`risk_compliance` → `compliance_risk`，`investment_research` → `equity_research`，`financial_engineering` → `fintech`，`wealth_management` → `wealth_mgmt`，`derivatives` → `fixed_income`

#### Scenario: selfmedia 部门 key 对齐
- **WHEN** 查看 `taxonomy.yaml` 中 selfmedia 行业的部门定义
- **THEN** `content_creation` SHALL 拆分为 `fiction_writing` + `content_graphic`，`growth_monetization` SHALL 变更为 `distribution`

#### Scenario: softwaredev 部门 key 对齐
- **WHEN** 查看 `taxonomy.yaml` 中 softwaredev 行业的部门定义
- **THEN** `game_client` SHALL 变更为 `gamedev`

### Requirement: taxonomy_loader.go 支持新格式
`taxonomy_loader.go` SHALL 支持 `taxonomy.yaml` 新格式中的所有扩展字段。

#### Scenario: 加载 responsibilities 字段
- **WHEN** `LoadTaxonomySpec` 解析 `taxonomy.yaml`
- **THEN** 部门和岗位的 `responsibilities` 字段 SHALL 被正确解析为 `[]string`

#### Scenario: 加载 skills_required 字段
- **WHEN** `LoadTaxonomySpec` 解析 `taxonomy.yaml`
- **THEN** 岗位的 `skills_required` 字段 SHALL 被正确解析为 `[]string`

#### Scenario: 加载 seniority_level 字段
- **WHEN** `LoadTaxonomySpec` 解析 `taxonomy.yaml`
- **THEN** 岗位的 `seniority_level` 字段 SHALL 被正确解析

#### Scenario: 加载 variants 字段
- **WHEN** `LoadTaxonomySpec` 解析 `taxonomy.yaml`
- **THEN** 岗位的 `variants` 字段 SHALL 被正确解析为结构化对象数组

### Requirement: SeedBuiltinTaxonomy 自动种子
系统 SHALL 在应用启动时自动执行 `SeedBuiltinTaxonomy`，从 `taxonomy.yaml` 加载数据并写入 `industry_taxonomy` 表。

#### Scenario: 启动时自动 seed
- **WHEN** 应用启动
- **THEN** `SeedBuiltinTaxonomy` SHALL 自动执行，从 `taxonomy.yaml` 加载三级分类数据并写入 `industry_taxonomy` 表

#### Scenario: 版本门控
- **WHEN** `SeedBuiltinTaxonomy` 执行
- **THEN** 系统 SHALL 通过版本门控 `SeedTaxonomyV1` 判断是否需要执行 seed，已执行过的版本 SHALL 不重复写入

#### Scenario: Upsert 语义
- **WHEN** `SeedBuiltinTaxonomy` 写入分类节点
- **THEN** 系统 SHALL 使用 upsert 语义（INSERT ... ON CONFLICT UPDATE），确保重复执行不会产生重复数据

#### Scenario: 三级顺序写入
- **WHEN** `SeedBuiltinTaxonomy` 写入分类节点
- **THEN** 系统 SHALL 按 industry → department → position 顺序写入，维护 parent_id 关系

#### Scenario: metadata_json 存储扩展字段
- **WHEN** `SeedBuiltinTaxonomy` 写入分类节点
- **THEN** `metadata_json` 字段 SHALL 存储 `responsibilities`、`skills_required`、`seniority_level` 等扩展字段（JSON 格式）

#### Scenario: 删除旧 CLI seed 工具
- **WHEN** 查看 `cmd/` 目录
- **THEN** `cmd/seed-industries/` 目录 SHALL 不存在（功能已被自动 seed 替代）

### Requirement: 数据库迁移脚本
系统 SHALL 提供数据库迁移脚本，将旧表 `agent_category_nodes` 重命名为 `industry_taxonomy`。

#### Scenario: 表重命名迁移
- **WHEN** 执行数据库迁移脚本
- **THEN** `ALTER TABLE agent_category_nodes RENAME TO industry_taxonomy` SHALL 被执行

#### Scenario: 列重命名迁移
- **WHEN** 执行数据库迁移脚本
- **THEN** `ALTER TABLE industry_taxonomy RENAME COLUMN category_key TO taxonomy_key` SHALL 被执行，`ALTER TABLE agents RENAME COLUMN category_position_id TO taxonomy_position_id` SHALL 被执行

#### Scenario: 索引重建迁移
- **WHEN** 执行数据库迁移脚本
- **THEN** 旧索引 `idx_agent_category_parent` 和 `idx_agent_category_level` SHALL 被删除，新索引 `idx_taxonomy_parent` 和 `idx_taxonomy_level` SHALL 被创建

#### Scenario: DDL 文件更新
- **WHEN** 查看 SQL DDL 文件
- **THEN** 新安装时的建表语句 SHALL 使用 `industry_taxonomy` 表名和 `taxonomy_key` 列名
