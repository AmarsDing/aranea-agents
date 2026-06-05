## MODIFIED Requirements

### Requirement: GenerateReport 集成根因分析
GenerateReport 不仅生成经验报告，还 SHALL 调用 RootCauseAnalyzer 接口进行根因分析，将运行时自愈的知识注入 Skill 诊断。ExperienceReport MUST 新增 RootCauseAnalysis 和 SuggestedFix 字段。

#### Scenario: 经验报告包含根因分析
- **WHEN** GenerateReport 处理一次失败的 Skill 调用
- **THEN** 系统 SHALL 调用 RootCauseAnalyzer.AnalyzeFromReport，将根因分析结果写入 ExperienceReport.RootCauseAnalysis

#### Scenario: 经验报告包含修复建议
- **WHEN** 根因分析返回 FixAction
- **THEN** 系统 SHALL 将 FixAction 转换为人类可读的修复建议，写入 ExperienceReport.SuggestedFix

## ADDED Requirements

### Requirement: Skill Intelligence Cron Worker
系统 SHALL 提供 skill_intelligence_worker Cron Job，每 10 分钟扫描未分析的 skill_invocation，批量执行 AnalyzeInvocation/ScoreSkill/GenerateReport。

#### Scenario: 批量分析未处理的调用
- **WHEN** skill_intelligence_worker 执行
- **THEN** 系统 SHALL 查询最近 10 分钟内 analyzed_at IS NULL 的 skill_invocation，批量处理

#### Scenario: 更新 analyzed_at 时间戳
- **WHEN** 一条 skill_invocation 分析完成
- **THEN** 系统 SHALL 更新其 analyzed_at 字段为当前时间

### Requirement: 经验报告 API
系统 SHALL 提供 ListExperienceReports 和 GetExperienceReport API。

#### Scenario: 按 Skill 查询经验报告
- **WHEN** 调用 ListExperienceReports(skill_id, limit, offset)
- **THEN** 系统 SHALL 返回该 Skill 的经验报告列表，按 created_at DESC 排序

### Requirement: Curator Agent 半自动进化
系统 SHALL 通过 ChatOrchestrator 调用自身 Agent 生成 Skill 草案，每日调用上限 20 次。

#### Scenario: 触发条件判定
- **WHEN** Skill 的 7d 成功率 < 60% 或同一失败标签出现 >= 5 次
- **THEN** 系统 SHALL 创建 SkillEvolutionSuggestion 并触发 Curator Agent

#### Scenario: Curator Agent 生成草案
- **WHEN** Curator Agent 被触发
- **THEN** 系统 SHALL 通过 ChatOrchestrator 调用自身 Agent，输入失败模式+历史调用记录+现有 Skill 列表，输出 Skill 草案（SKILL.md）

#### Scenario: Sandbox Runner 验证
- **WHEN** Curator Agent 生成 Skill 草案
- **THEN** 系统 SHALL 在 Sandbox Runner（codeexecutor.CodeExecutor/E2B）中隔离执行验证
