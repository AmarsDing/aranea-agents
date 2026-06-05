## MODIFIED Requirements

### Requirement: 数据迁移幂等性保障
数据迁移 MUST 在重复执行时产生相同结果，不覆盖已有有效数据。

#### Scenario: SessionRevision 迁移重复执行
- **WHEN** `ddlSessionRevisionDataMigration` 因 `isMigrationApplied` 查询失败而重复执行
- **THEN** 仅更新 `session_revision IS NULL` 的记录，已有 revision 值不被覆盖

#### Scenario: 迁移状态记录失败
- **WHEN** `recordMigrationApplied` 写入失败
- **THEN** 下次启动时迁移可安全重复执行（幂等保护）
