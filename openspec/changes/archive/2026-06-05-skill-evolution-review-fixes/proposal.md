## Why

`skill-evolution-auto-creator` 变更已归档，但代码审查发现 3 个阻断级问题导致核心功能在运行时完全不可用：`provideSkillAutoCreator()` 返回 nil 使自动检测永远无结果；`SkillEvolutionScanner` 未注册 Wire 使定时扫描永不启动；`UpdateStatus` 写+读不在同一事务存在并发数据不一致风险。此外还有 11 个建议级问题（分页未实现、魔法数字、nil 依赖处理不一致等）需要修复。

## What Changes

- 实现 `LLMSkillGenerator` 适配器并注入 `provideSkillAutoCreator`，替换当前 nil 返回值
- 注册 `SkillEvolutionScanner` 到 Wire ProviderSet 和 Bootstrap，使定时扫描任务可启动
- 修复 `UpdateStatus` 写+读事务一致性问题
- 统一 nil 依赖处理策略（`creator == nil` 与 `registrar == nil` 行为不一致）
- 实现 `ListSkillProposals` 分页逻辑（当前分页参数被计算但未使用）
- 在 `RegisterApproved` 中校验 `SkillMD` 非空，防止注册空 body 的 Skill
- 提取魔法数字为命名常量（`500`、`0.15`、`"success"`）
- 修复 `skillsButlerTools` 中 `context.Background()` 替换为传入 ctx
- 删除未使用的 `SkillProposalStatusExpired` 常量
- 优化 `GetSkillInvocationStats` 使用 SQL 聚合替代内存计算
- 修复 `ScanAndProposeAll` 分页遍历（当前只扫描前 500 个 agent）

## Capabilities

### New Capabilities

- `skill-evolution-runtime-fix`: 修复 SkillEvolution 核心运行时功能（LLM 适配器注入、定时扫描注册、事务一致性）

### Modified Capabilities

## Impact

- **biz 层**：`skill_evolution.go`（nil 处理统一、常量提取、分页遍历）、`skill_evolution_types.go`（删除死代码）
- **data 层**：`skill_evolution.go`（事务修复）、`skill_invocation_stats.go`（SQL 聚合优化）
- **service 层**：`skill_evolution.go`（分页实现）、`cli_admin_tools.go`（ctx 传递修复）
- **wire 层**：`cmd/admin/wire.go`（新增 provider、注册 scanner）
- **skill 层**：`auto_creator.go`（LLM 适配器实现）
- **cronrunner 层**：`jobs/skill_evolution.go`（无变更，但需 Wire 注册）
