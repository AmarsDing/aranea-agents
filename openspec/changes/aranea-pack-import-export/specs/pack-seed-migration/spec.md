## ADDED Requirements

### Requirement: 内置数据转为 .arpack 格式
系统 SHALL 将现有 YAML 数据源转换为 .arpack 目录结构，通过 go:embed 嵌入二进制。

#### Scenario: 内置模板 Pack
- **WHEN** 应用编译时
- **THEN** `internal/scenario/packs/builtin-templates/` 目录 SHALL 包含 agent templates（fox/programmer/...）和 graph templates（pipeline/approval/...）的 .arpack 格式文件

#### Scenario: 行业 Pack
- **WHEN** 应用编译时
- **THEN** `internal/scenario/packs/finance/`、`internal/scenario/packs/selfmedia/`、`internal/scenario/packs/softwaredev/` 目录 SHALL 包含对应行业的 .arpack 格式文件

### Requirement: 启动时通过 Pack 引擎加载内置种子
系统 SHALL 在启动时使用统一的 Pack 导入引擎加载内置 .arpack 数据，替代 RawSQL 种子。

#### Scenario: P1 阶段加载基础数据
- **WHEN** 应用启动 P1 阶段
- **THEN** 系统 SHALL 加载 `builtin-templates.arpack`（taxonomy + agent templates + graph templates），使用 overwrite 冲突策略

#### Scenario: Lazy 阶段加载行业数据
- **WHEN** 应用启动 Lazy 阶段
- **THEN** 系统 SHALL 依次加载 `finance.arpack`、`selfmedia.arpack`、`softwaredev.arpack`，使用 overwrite 冲突策略

#### Scenario: 版本门控
- **WHEN** 内置 Pack 的版本号与 schema_migrations 表记录一致
- **THEN** 系统 SHALL 跳过该 Pack 的加载

### Requirement: 删除 RawSQL 种子代码
系统 SHALL 删除以下 RawSQL 种子文件，其功能由 Pack 引擎替代。

#### Scenario: 删除 RawSQL 文件
- **WHEN** Pack 种子迁移完成
- **THEN** 系统 SHALL 删除 `seed_industry_agents_rawsql.go`、`seed_builtin_taxonomy.go`（RawSQL 版）、`seed_agent_templates.go`（RawSQL 版）

### Requirement: 废弃 orgimport 包
系统 SHALL 废弃 `internal/orgimport/` 包，其功能由 Pack 导入引擎替代。

#### Scenario: orgimport 标记为废弃
- **WHEN** Pack 导入引擎可用后
- **THEN** `internal/orgimport/` 包 SHALL 标记为 deprecated，并在后续版本中删除

### Requirement: 删除 Go 硬编码 Graph 模板
系统 SHALL 将 `internal/graph/trpc/templates.go` 中的 6 个内置模板迁移到 .arpack 格式。

#### Scenario: 模板从 Go 代码迁移到 YAML
- **WHEN** builtin-templates.arpack 包含 pipeline.yaml、approval.yaml 等 Graph 模板
- **THEN** `templates.go` 中的 `builtinTemplates` 变量 SHALL 从 Pack 数据源加载，而非硬编码
