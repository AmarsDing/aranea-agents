## ADDED Requirements

### Requirement: 统一失败模式知识库
系统 SHALL 提供 failure_pattern 表，统一存储运行时自愈规则和 CI Auto-Fix 模式。每条记录 MUST 包含：ID(UUID)、Source(runtime/ci/mined)、Type、PatternHash(SHA256)、PatternRegex、FixAction(JSON)、Confidence(0-1)、SuccessCount、FailCount、Version、IsActive、CreatedAt、UpdatedAt。

#### Scenario: 运行时规则同步到知识库
- **WHEN** failure_pattern_sync Cron Job 执行
- **THEN** 系统 SHALL 将 RootCauseEngine 的 12 条内置规则同步到 failure_pattern 表，source 为 "runtime"，confidence 为 0.9

#### Scenario: CI 模式同步到知识库
- **WHEN** failure_pattern_sync Cron Job 执行
- **THEN** 系统 SHALL 将 .auto-fix/patterns.jsonl 中的修复记录聚合后同步到 failure_pattern 表，source 为 "ci"

#### Scenario: 按模式哈希精确查询
- **WHEN** 查询特定失败模式
- **THEN** 系统 SHALL 通过 pattern_hash 字段精确索引查询，不使用 pattern_regex 做索引

### Requirement: 失败模式版本化
系统 SHALL 为每条 failure_pattern 记录维护版本号，支持回滚到历史版本。

#### Scenario: 动态挖掘规则版本递增
- **WHEN** 动态挖掘生成新的修复模板
- **THEN** 系统 SHALL 创建新的 failure_pattern 记录，version 从 1 开始，source 为 "mined"，confidence 为 0.5

#### Scenario: 规则回滚
- **WHEN** 管理员禁用某条 mined 规则
- **THEN** 系统 SHALL 将 is_active 设为 false，不影响其他版本

### Requirement: 知识库审计机制
系统 SHALL 提供每周审计机制，对 source="mined" 的规则进行人工审核。

#### Scenario: 新挖掘规则需验证
- **WHEN** 一条 mined 规则经过 3 次成功验证
- **THEN** 系统 SHALL 将其 confidence 从 0.5 提升到 0.8

#### Scenario: 低质量规则禁用
- **WHEN** 一条 mined 规则的 fail_count > success_count * 2
- **THEN** 系统 SHALL 自动将 is_active 设为 false
