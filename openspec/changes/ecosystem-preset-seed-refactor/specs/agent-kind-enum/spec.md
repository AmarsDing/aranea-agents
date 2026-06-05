## MODIFIED Requirements

### Requirement: Agent Kind enumeration
Agent Kind 枚举 SHALL 精简为 `user | system_builtin | ecosystem_preset | marketplace | certified`，删除 `system` 和 `industry_template`。

#### Scenario: 新创建的 Agent 默认 Kind
- **WHEN** 用户通过前端创建新 Agent
- **THEN** Agent Kind 默认为 `user`

#### Scenario: 系统内置 Agent Kind
- **WHEN** 系统启动时种子管道创建系统内置 Agent（精灵/管家/记忆/技能）
- **THEN** Agent Kind 为 `system_builtin`，`readonly` 为 `true`

#### Scenario: 附带生态 Agent Kind
- **WHEN** 用户通过加载生态 API 导入行业 Agent
- **THEN** Agent Kind 为 `ecosystem_preset`

#### Scenario: 数据迁移 - system 到 system_builtin
- **WHEN** 数据库中存在 Kind 为 `system` 的 Agent 记录
- **THEN** 迁移脚本将其 Kind 更新为 `system_builtin`

#### Scenario: 数据迁移 - industry_template 到 ecosystem_preset
- **WHEN** 数据库中存在 Kind 为 `industry_template` 的 Agent 记录
- **THEN** 迁移脚本将其 Kind 更新为 `ecosystem_preset`

### Requirement: Team Kind field
Team 表 SHALL 新增 `kind` 字段，枚举值与 Agent Kind 完全对齐：`user | system_builtin | ecosystem_preset | marketplace | certified`，默认值为 `user`。

#### Scenario: 新创建的 Team 默认 Kind
- **WHEN** 用户通过前端创建新 Team
- **THEN** Team Kind 默认为 `user`

#### Scenario: 系统内置 Team Kind
- **WHEN** 系统创建内置 Team（如精灵组建的编排 Team）
- **THEN** Team Kind 为 `system_builtin`

#### Scenario: 附带生态 Team Kind
- **WHEN** 用户通过加载生态 API 导入行业 Team
- **THEN** Team Kind 为 `ecosystem_preset`

#### Scenario: 数据迁移 - Team kind 初始化
- **WHEN** 数据库中 Team 表已有记录但 kind 字段为空或默认值
- **THEN** 迁移脚本将 `source = 'imported'` 的 Team Kind 更新为 `ecosystem_preset`，其余保持 `user`

#### Scenario: Team kind 与 source 字段职责区分
- **WHEN** 查询 Team 的权限分类
- **THEN** 使用 `kind` 字段（决定可编辑性/可删除性/徽章显示），`source` 字段仅用于来源追踪审计

## REMOVED Requirements

### Requirement: Agent Kind system value
**Reason**: `system` Kind 无实际使用场景，`system_builtin` 已覆盖系统内置语义
**Migration**: 数据迁移 `UPDATE agents SET kind = 'system_builtin' WHERE kind = 'system'`

### Requirement: Agent Kind industry_template value
**Reason**: `industry_template` 语义模糊（描述来源而非权限分类），被 `ecosystem_preset` 替代
**Migration**: 数据迁移 `UPDATE agents SET kind = 'ecosystem_preset' WHERE kind = 'industry_template'`
