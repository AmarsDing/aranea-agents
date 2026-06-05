## MODIFIED Requirements

### Requirement: Seed pipeline architecture
种子管道 SHALL 统一为 2 条：L1 启动管道（系统内置，启动时强制执行）和 L2 API 触发管道（附带生态，用户按需触发）。

#### Scenario: L1 启动管道执行
- **WHEN** 系统启动完成 P1 阶段
- **THEN** 系统内置 Agent（精灵/管家/记忆/技能）、内置工具、内置模板 Pack、Prompt 文件、Cron 任务全部加载完成，Kind 为 `system_builtin`，`readonly` 为 `true`

#### Scenario: L2 附带生态不自动加载
- **WHEN** 系统启动完成
- **THEN** 行业 Pack（finance/selfmedia/softwaredev）不自动加载，等待用户通过 API 触发

#### Scenario: Pack 引擎支持 Kind 覆盖
- **WHEN** 调用 `pack.Importer.Import()` 时传入 `WithKindOverride("ecosystem_preset")`
- **THEN** 导入的 Agent Kind 被覆盖为 `ecosystem_preset`，而非 Pack Spec 中的默认值

## REMOVED Requirements

### Requirement: Lazy Seeder for industry packs
**Reason**: 行业 Pack 不再自动加载，改为 API 触发
**Migration**: 删除 `data.go` 中 Lazy Seeder 行业 Pack 注册代码

### Requirement: SeedBuiltinIndustryAgents legacy pipeline
**Reason**: 旧版 YAML Loader 管道与 Pack 引擎管道重复，统一走 Pack 引擎
**Migration**: 删除 `internal/service/industry_agent_seed.go`
