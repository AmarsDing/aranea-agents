## ADDED Requirements

### Requirement: Skill 五阶段进化闭环
系统 SHALL 实现 Solve→Observe→Evolve→Gate→Reload 五阶段 Skill 进化闭环，每个阶段独立可审计。

#### Scenario: Solve 阶段
- **WHEN** Skill 健康度下降触发进化
- **THEN** 系统 SHALL 使用当前 Skill 配置执行目标任务，记录执行结果

#### Scenario: Observe 阶段
- **WHEN** Solve 阶段完成
- **THEN** 系统 SHALL 采集结构化日志、性能指标、Skill 调用成功率，存入经验报告

#### Scenario: Evolve 阶段
- **WHEN** Observe 阶段完成
- **THEN** 系统 SHALL 调用 Curator Agent 分析观察数据，生成 Skill 草案（SKILL.md）

#### Scenario: Gate 阶段
- **WHEN** Evolve 阶段生成 Skill 草案
- **THEN** 系统 SHALL 执行多维验证：功能正确性（Sandbox Runner）+ 安全性（CodeQL）+ 性能（Token/耗时对比）+ 风格（araneactl lint）

#### Scenario: Reload 阶段
- **WHEN** Gate 阶段验证通过且人工审批完成
- **THEN** 系统 SHALL 注册新 Skill 版本，标记 parent_version_id 和 evolution_reason

### Requirement: Gate 多维验证
系统 SHALL 在 Gate 阶段执行四维验证，任一维度失败则拒绝进化。

#### Scenario: 功能正确性验证
- **WHEN** Gate 阶段执行功能验证
- **THEN** 系统 SHALL 在 Sandbox Runner（E2B）中执行 Skill，验证输出符合预期

#### Scenario: 性能退化验证
- **WHEN** Gate 阶段执行性能验证
- **THEN** 系统 SHALL 对比新旧 Skill 的 Token 消耗和耗时，退化 > 20% 则拒绝进化

#### Scenario: 安全性验证
- **WHEN** Gate 阶段执行安全验证
- **THEN** 系统 SHALL 检查 Skill 草案不含敏感信息（API key/password/token），通过 CodeQL 扫描

### Requirement: 进化建议过期机制
系统 SHALL 对 7 天未审批的进化建议自动标记为过期。

#### Scenario: 进化建议过期
- **WHEN** 进化建议创建超过 7 天且状态仍为 pending
- **THEN** 系统 SHALL 将状态更新为 expired
