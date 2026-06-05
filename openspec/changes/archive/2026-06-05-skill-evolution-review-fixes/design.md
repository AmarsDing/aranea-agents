## Context

`skill-evolution-auto-creator` 变更已归档并合入主干，但代码审查发现 3 个阻断级 + 11 个建议级问题。其中最严重的是 `provideSkillAutoCreator()` 返回 nil 和 `SkillEvolutionScanner` 未注册 Wire，导致"自动检测重复模式并提议创建新 Skill"的核心功能在运行时完全不可用。

当前状态：
- API 层（proto + service）和审批流程可正常工作
- 自动检测（`DetectAndPropose`）因 `creator == nil` 永远返回空结果
- 定时扫描（`SkillEvolutionScanner`）因未注册 Wire 永远不启动
- `UpdateStatus` 写后读不在同一事务，存在并发数据不一致风险

## Goals / Non-Goals

**Goals:**

- 修复 3 个阻断项，使 SkillEvolution 核心功能在运行时可用
- 修复高优先级建议项（空 SkillMD 注册、分页未实现、ctx 传递错误）
- 提取魔法数字为命名常量，提升代码可维护性
- 优化 `GetSkillInvocationStats` 性能（SQL 聚合替代内存计算）

**Non-Goals:**

- 不修改 proto 接口定义（API 契约不变）
- 不新增 Ent Schema（表结构不变）
- 不实现 `SkillProposalStatusExpired` 的过期逻辑（仅删除未使用的常量）
- 不重构 `SkillAutoCreator` 接口签名

## Decisions

### D1: LLM 适配器实现策略

**决策**：在 `internal/skill/auto_creator.go` 中新增 `LLMModelAdapter` 结构体，实现 `LLMSkillGenerator` 接口，内部调用 `model.Model.Generate()` 方法。

**理由**：biz 层已定义 `SkillAutoCreator` 接口，skill 层已定义 `LLMSkillGenerator` 接口。只需在 Wire 层构造 `LLMModelAdapter` 并注入 `NewSkillAutoCreator(adapter, lg)` 即可。

**替代方案**：
- 直接在 `provideSkillAutoCreator` 中构造 → 违反了构造函数模式，Wire provider 应只做绑定
- 在 service 层实现 → 违反分层规范，LLM 调用属于 skill 领域

### D2: SkillEvolutionScanner Wire 注册

**决策**：新增 `provideSkillEvolutionScanner` provider 函数，参照 `provideLearningLoopScanner` 模式，支持 `SKILL_EVOLUTION_DISABLED` 环境变量禁用。将 scanner 加入 Bootstrap 结构体并在启动流程中调用 `Start()`。

**理由**：与 `LearningLoopScanner`/`EvolutionScanner` 保持一致的注册模式。

### D3: UpdateStatus 事务一致性

**决策**：在 `UpdateStatus` 中使用 `r.data.RWDB().WriteDB(ctx)` 开启事务，在事务内执行 UPDATE + SELECT，最后 commit。

**替代方案**：
- 使用 RETURNING 子句 → SQLite 不完全支持 RETURNING 语法
- 在写操作中直接构造返回对象 → 需要手动设置 `ApprovedAt`/`RejectedBy` 等字段，容易遗漏

### D4: nil 依赖处理统一策略

**决策**：当可选依赖为 nil 时，统一策略为"返回 nil/空结果 + Warn 日志"。不返回错误，因为 nil 依赖是配置层面的选择（如未启用 SkillEvolve），不应阻断调用方。

**理由**：当前 `creator == nil` 返回空结果，`registrar == nil` 返回 `kerrors.InternalServer`，行为不一致。统一为空结果+日志更符合"功能降级"语义。

### D5: 分页实现

**决策**：在 `SkillProposalReader.ListByAgent` 接口增加 `limit`/`offset` 参数，Data 层 SQL 添加 `LIMIT ? OFFSET ?`。Service 层传入 `biz.PageToLimitOffset` 计算结果。

**理由**：当前获取全量再切片效率低，且 biz 层接口签名需要调整以支持分页参数传递。

### D6: GetSkillInvocationStats 性能优化

**决策**：将 Ent 查询 + 内存聚合替换为原生 SQL 聚合查询（`SELECT tool_key, COUNT(*), SUM(CASE WHEN status='success' THEN 1 ELSE 0 END), SUM(duration_ms) FROM tool_invocations WHERE agent_id=? AND created_at>=? GROUP BY tool_key`）。

**理由**：当前实现将全部 ToolInvocation 记录加载到内存再聚合，数据量大时内存和 CPU 开销显著。SQL 聚合在数据库层完成，只返回统计结果。

## Risks / Trade-offs

- **[Risk] LLM 适配器需要 model.Model 实例** → 通过 Wire 注入 `model.Provider` 或 `LLMService`，需确认现有 Wire 图中可获取的依赖链
- **[Risk] ListByAgent 接口签名变更** → 影响所有实现方（data 层 + mock），需同步更新测试
- **[Risk] 事务引入增加 Data 层复杂度** → 事务范围小（单表 UPDATE + SELECT），复杂度可控
- **[Trade-off] SQL 聚合绕过 Ent ORM** → 性能优先，但牺牲了 Ent 类型安全。可接受因为这是纯统计查询
