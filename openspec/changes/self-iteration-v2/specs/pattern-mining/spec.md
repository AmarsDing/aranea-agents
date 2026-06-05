## ADDED Requirements

### Requirement: 历史修复记录自动挖掘
系统 SHALL 每日从历史修复记录中自动提取修复模板，动态更新知识库。PatternMiningWorker MUST 聚类相似失败模式并提取共性修复策略。

#### Scenario: 聚类相似失败模式
- **WHEN** PatternMiningWorker 执行
- **THEN** 系统 SHALL 将相同 error_code + 相似 stack_trace 的修复记录聚类为同一模式

#### Scenario: 提取成功修复的 diff 模式
- **WHEN** 同一聚类中有 >= 3 次成功修复
- **THEN** 系统 SHALL 提取共性 diff 模式，生成修复模板，写入 failure_pattern 表（source="mined"）

#### Scenario: 不足 3 次成功修复不提取
- **WHEN** 同一聚类中成功修复 < 3 次
- **THEN** 系统 SHALL 不生成修复模板，等待更多数据

### Requirement: 挖掘规则初始低置信度
系统 SHALL 对动态挖掘的规则设置初始 confidence=0.5，低于内置规则的 0.9。

#### Scenario: 新挖掘规则初始置信度
- **WHEN** 动态挖掘生成新规则
- **THEN** 系统 SHALL 设置 confidence=0.5，is_active=true

#### Scenario: 验证后置信度提升
- **WHEN** 一条 mined 规则经过 3 次成功验证
- **THEN** 系统 SHALL 将 confidence 提升到 0.8

### Requirement: 挖掘规则版本化
系统 SHALL 为每次挖掘生成的规则维护版本号，支持回滚。

#### Scenario: 规则更新版本递增
- **WHEN** 同一 pattern_hash 的规则被重新挖掘
- **THEN** 系统 SHALL 创建新版本记录，version 递增
