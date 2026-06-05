## ADDED Requirements

### Requirement: LLM Skill Generator 适配器注入

系统 SHALL 通过 Wire 注入非 nil 的 `SkillAutoCreator` 实现，使 `DetectAndPropose` 方法可正常调用 LLM 生成 SKILL.md。

#### Scenario: provideSkillAutoCreator 返回有效实例

- **WHEN** 应用启动并执行 Wire 注入
- **THEN** `SkillEvolutionUsecase.creator` SHALL 不为 nil
- **AND** `DetectAndPropose` SHALL 能调用 `GenerateSKILLMD` 生成 SKILL.md 内容

#### Scenario: LLM 调用超时

- **WHEN** LLM 生成 SKILL.md 超过 30 秒
- **THEN** `GenerateSKILLMD` SHALL 返回 `kerrors.GatewayTimeout`
- **AND** `DetectAndPropose` SHALL 记录 Warn 日志并 continue 处理下一个模式

### Requirement: SkillEvolutionScanner 定时扫描注册

系统 SHALL 将 `SkillEvolutionScanner` 注册到 Wire ProviderSet 和 Bootstrap 结构体，使定时扫描任务在应用启动时自动运行。

#### Scenario: 默认启动扫描

- **WHEN** 应用启动且未设置 `SKILL_EVOLUTION_DISABLED=1` 环境变量
- **THEN** `SkillEvolutionScanner` SHALL 被构造并加入 Bootstrap
- **AND** 扫描任务 SHALL 以 60 分钟为默认间隔周期执行

#### Scenario: 环境变量禁用

- **WHEN** 设置了 `SKILL_EVOLUTION_DISABLED=1` 环境变量
- **THEN** `provideSkillEvolutionScanner` SHALL 返回 nil
- **AND** 定时扫描任务 SHALL 不启动

### Requirement: UpdateStatus 事务一致性

`UpdateStatus` 方法 SHALL 在同一数据库事务中完成写操作和读回操作，确保并发场景下数据一致性。

#### Scenario: 并发审批同一 Proposal

- **WHEN** 两个请求并发调用 `UpdateStatus` 对同一 proposal 执行审批
- **THEN** 事务 SHALL 保证其中一个成功，另一个基于事务内最新状态判断
- **AND** 返回的结果 SHALL 反映事务提交后的最终状态

### Requirement: nil 依赖处理统一

当 `SkillEvolutionUsecase` 的可选依赖（`creator`/`registrar`/`patterns`/`agents`）为 nil 时，SHALL 统一返回 nil/空结果并记录 Warn 日志，不返回错误。

#### Scenario: creator 为 nil

- **WHEN** `SkillAutoCreator` 依赖为 nil
- **THEN** `DetectAndPropose` SHALL 返回 `nil, nil`
- **AND** SHALL 记录 Warn 日志表明 creator 未配置

#### Scenario: registrar 为 nil

- **WHEN** `SkillRegistrationPort` 依赖为 nil
- **THEN** `RegisterApproved` SHALL 返回 `SkillProposal{}, nil`（降级而非报错）
- **AND** SHALL 记录 Warn 日志表明 registrar 未配置

### Requirement: ListProposals 分页实现

`ListProposals` SHALL 支持 limit/offset 分页，避免全量查询。

#### Scenario: 传入分页参数

- **WHEN** 调用 `ListProposals(ctx, agentID, status, limit, offset)`
- **THEN** Data 层 SHALL 使用 `LIMIT ? OFFSET ?` SQL 子句
- **AND** 返回结果 SHALL 只包含当前页数据

#### Scenario: 不传分页参数

- **WHEN** limit 和 offset 均为 0
- **THEN** SHALL 返回全部结果（不应用分页限制）

### Requirement: RegisterApproved 校验 SkillMD 非空

`RegisterApproved` SHALL 在注册前校验 proposal 的 `SkillMD` 不为空，防止注册空 body 的 Skill。

#### Scenario: SkillMD 为空

- **WHEN** proposal 的 `SkillMD` 为空字符串
- **THEN** `RegisterApproved` SHALL 返回 `kerrors.BadRequest`
- **AND** SHALL 不调用 `SkillRegistrationPort.RegisterSkill`

#### Scenario: SkillMD 有内容

- **WHEN** proposal 的 `SkillMD` 以 `---` 开头（有效 YAML front matter）
- **THEN** `RegisterApproved` SHALL 正常执行注册流程

### Requirement: 魔法数字提取为命名常量

系统 SHALL 将以下硬编码值提取为命名常量：

- `500`（ScanAndProposeAll 的 agent 查询限制）→ `defaultScanAgentLimit`
- `0.15`（模式置信度阈值）→ `skillPatternMinConfidence`
- `"success"`（ToolInvocation 状态）→ 使用 biz 层定义的状态常量

#### Scenario: 常量定义位置

- **WHEN** 代码需要引用扫描限制或置信度阈值
- **THEN** SHALL 使用 `skill_evolution.go` 中定义的常量
- **AND** 禁止在业务逻辑中硬编码数字字面量

### Requirement: skillsButlerTools 使用传入 ctx

`skillsButlerTools` 方法 SHALL 使用传入的 `ctx context.Context` 参数，不使用 `context.Background()`。

#### Scenario: 传递请求上下文

- **WHEN** 调用 `skillsButlerTools(ctx, ag)`
- **THEN** 内部调用 `GetAgentRuntimeSettings` SHALL 使用传入的 `ctx`
- **AND** 请求取消时 SHALL 正确传播

### Requirement: GetSkillInvocationStats SQL 聚合优化

`GetSkillInvocationStats` SHALL 使用 SQL 聚合查询替代内存计算，避免加载全量 ToolInvocation 记录。

#### Scenario: 大量调用记录

- **WHEN** agent 有超过 1000 条 ToolInvocation 记录
- **THEN** `GetSkillInvocationStats` SHALL 通过 SQL GROUP BY 聚合
- **AND** SHALL 只返回统计结果（SkillName/Count/SuccessRate/AvgDurationMs）
- **AND** 内存占用 SHALL 不随记录数线性增长

### Requirement: ScanAndProposeAll 分页遍历

`ScanAndProposeAll` SHALL 分页遍历所有 active agent，不限制为前 500 个。

#### Scenario: 超过 500 个 active agent

- **WHEN** 系统中有 600 个 active agent
- **THEN** `ScanAndProposeAll` SHALL 遍历全部 600 个 agent
- **AND** 不遗漏任何启用了 `EvolutionSkillEvolve` 的 agent

#### Scenario: 分页循环

- **WHEN** 第一页查询返回满额结果
- **THEN** SHALL 继续查询下一页直到返回空结果

### Requirement: 删除未使用的 SkillProposalStatusExpired

系统 SHALL 删除 `SkillProposalStatusExpired` 常量，该常量已定义但从未使用。

#### Scenario: 编译验证

- **WHEN** 删除 `SkillProposalStatusExpired` 常量
- **THEN** `go build ./...` SHALL 通过
- **AND** 无任何代码引用该常量
