## ADDED Requirements

### Requirement: SkillExperienceReport 领域模型
系统 SHALL 定义 SkillExperienceReport 结构体，包含 ID/TenantID/SessionID/InvocationID/SkillID/IsSuccess/Score/FailureTags/FlowSummary/OptimizationAdvice/SelectionSnapshot/GeneratedSuggestionID/CreatedAt 字段，以及失败标签常量（param_mismatch / wrong_tool_choice / tool_timeout / tool_api_error / context_overflow / instruction_ambiguity / user_cancelled / unknown）。

#### Scenario: 创建诊断报告
- **WHEN** Skill 调用完成且 outcome 为 failure 或 partial
- **THEN** 系统生成 SkillExperienceReport，包含失败标签、评分、优化建议

### Requirement: ExperienceReportRepo 端口接口
系统 SHALL 定义 ExperienceReportReader（ListBySkill/GetByID/ListByTimeRange）和 ExperienceReportWriter（Create/BatchCreate）接口。

#### Scenario: 按时间范围查询报告
- **WHEN** 调用 ListByTimeRange(ctx, skillID, start, end)
- **THEN** 返回该时间范围内的所有 SkillExperienceReport

### Requirement: SkillIntelligenceUsecase
系统 SHALL 实现 SkillIntelligenceUsecase，包含 AnalyzeInvocation/ScoreSkill/GenerateReport 方法。GenerateReport 支持规则层 + LLM 层（可降级）。

#### Scenario: 规则层生成报告
- **WHEN** LLM 不可用
- **THEN** GenerateReport 仅使用规则层提取结构化字段，返回不含 OptimizationAdvice 的报告

#### Scenario: LLM 层生成报告
- **WHEN** LLM 可用且调用成功
- **THEN** GenerateReport 在规则层基础上补充 OptimizationAdvice 和 FlowSummary

### Requirement: SkillIntelligenceWorker 定时任务
系统 SHALL 实现定时任务，可配置间隔（默认 15min），扫描近 N 小时新增且含 Skill 调用的 session，对每个 invocation 调用 GenerateReport。

#### Scenario: Worker 关闭时不影响对话
- **WHEN** SKILL_INTELLIGENCE_DISABLED=1
- **THEN** Worker 不启动，对话功能不受影响

### Requirement: ListExperienceReports / GetExperienceReport API
系统 SHALL 提供 gRPC/HTTP API 查询经验报告。

#### Scenario: 列表查询
- **WHEN** 调用 GET /v1/skills/{skill_id}/experience-reports
- **THEN** 返回该 Skill 的经验报告列表，支持分页和时间范围过滤

### Requirement: skillrecommend.Rank 推荐排序
系统 SHALL 实现 skillrecommend.Rank 函数，对候选 slug 列表按语义相似度 × 历史成功率 × 耗时倒数 × 用户偏好重排。缺数据时取中性值 0.5，新 Skill（< 7d）可配置探索加成 +0.1。

#### Scenario: 无历史数据时排序
- **WHEN** 候选 Skill 无历史调用数据
- **THEN** 该项权重取 0.5 中性值，不偏向也不惩罚

### Requirement: Skill 去重检测与合并
系统 SHALL 实现 DetectDuplicateSkills（名称不同但 description + 正文相似度 ≥ 0.2 的 Skill 归组）和 MergeSkills（保留主 Skill，副 Skill archived，合并后 invoke 统计归并）。

#### Scenario: 检测重复 Skill
- **WHEN** 两个 Skill 的 description 相似度 ≥ 0.2 且正文相似度 ≥ 0.2
- **THEN** 归为同一重复组

### Requirement: SkillEvolutionSuggestion 领域模型
系统 SHALL 定义 SkillEvolutionSuggestion 结构体（ID/SkillID/Type/Status/SourceReportIDs/DraftSkillVersionID/SandboxPassed/CreatedAt/ResolvedAt），类型枚举（fix_failure / boost_efficiency / merge_duplicate），状态枚举（pending / approved / rejected / applied）。

#### Scenario: 触发进化建议
- **WHEN** 30d 失败 ≥ 3 且评分 < 60
- **THEN** 自动创建 SkillEvolutionSuggestion（status=pending）

### Requirement: Curator Agent 草案生成
系统 SHALL 实现 Curator Agent，输入原 Skill markdown + Experience Report，输出新 draft 版本 + evolution_reason。Curator 走 service 层调用 trpc-agent-go。

#### Scenario: 生成 Skill 草案
- **WHEN** SkillEvolutionSuggestion 被 approved
- **THEN** Curator Agent 生成包含正文修改的完整草案

### Requirement: Sandbox Runner 验证
系统 SHALL 用历史失败 case 重放 ≥ 1 次验证草案，隔离 Runner 执行不影响生产环境。

#### Scenario: Sandbox 验证通过
- **WHEN** 草案通过历史失败 case 重放
- **THEN** SandboxPassed=true，可进入人工审批

### Requirement: 进化建议 API + 人工审批 UI
系统 SHALL 提供 ListSkillEvolutionSuggestions / ApproveSkillEvolutionSuggestion / RejectSkillEvolutionSuggestion API，前端实现审批 UI。

#### Scenario: 审批进化建议
- **WHEN** 用户点击"批准"
- **THEN** SkillEvolutionSuggestion status 变为 approved，Curator Agent 生成草案

### Requirement: Skill 元数据扩展 + 版本血缘
系统 SHALL 新增 parent_version_id / evolution_reason / lifecycle_status 字段。lifecycle_status 枚举：active / shadow / deprecated。

#### Scenario: 版本血缘追溯
- **WHEN** Skill 通过进化建议发布新版本
- **THEN** 新版本的 parent_version_id 指向原版本，evolution_reason 记录原因

### Requirement: token_usage 填充（Phase 1 补齐）
系统 SHALL 在 Skill 执行完成后收集 LLM 调用的 token 使用量，写入 skill_invocation.token_usage 字段。

#### Scenario: token_usage 写入
- **WHEN** Skill 执行完成
- **THEN** skill_invocation.token_usage 包含 {prompt, completion, total}

### Requirement: 前端 Skill 详情健康度卡片（Phase 1 补齐）
系统 SHALL 在 Skill 详情页新增健康度卡片，调用 GetSkillHealth API，展示 7d/30d 成功率折线图、P95 耗时指标。

#### Scenario: 查看健康度
- **WHEN** 用户打开 Skill 详情页
- **THEN** 健康度卡片显示 7d/30d 成功率和 P95 耗时
