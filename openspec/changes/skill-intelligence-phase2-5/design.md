## Context

原 `2026-06-05-skill-intelligence` 变更已完成 Phase 1（skill_invocation schema 扩展、selection_reason 写入、outcome 写入、GetSkillHealth API），但 Phase 2~5 未实施。本变更续接。

**当前状态**:
- `internal/biz/skill_health.go` + `internal/data/skill_health.go` + `internal/biz/types/skill_health.go` 已存在（Phase 1 产物）
- `internal/biz/experience_analytics.go` 已存在（memory-skills-butler 变更产物，聚合型报告，与 Phase 2 的单次调用诊断报告定位不同）
- `internal/biz/skill_evolution.go` + `internal/data/skill_evolution.go` 已存在（skill-evolution-auto-creator 变更产物，管理 SkillProposal，与 Phase 4 的 SkillEvolutionSuggestion 数据模型不同）
- `internal/tools/skills_butler/recommend_skills.go` 已存在（Agent 维度离线推荐，与 Phase 3 的运行时路由排序不同）

**约束**:
- biz 层禁止 import pkg/trpc-agent-go
- Curator Agent 走 service 层调用 trpc-agent-go
- 所有日志用 loggateway.Logger
- 所有业务错误用 kerrors

## Goals / Non-Goals

**Goals:**
- Phase 2: 实现 ExperienceReport 领域模型 + Repo + Usecase + Cron Worker + API + 前端
- Phase 3: 实现 skillrecommend.Rank 运行时推荐排序 + Skill 去重
- Phase 4: 实现 SkillEvolutionSuggestion + Curator Agent + Sandbox + 审批 UI
- Phase 5: 单元测试 + 集成测试
- 补齐 Phase 1 遗留：token_usage 填充 + 前端健康度卡片

**Non-Goals:**
- 不改变已有 skill_health / experience_analytics / skill_evolution / skills_butler 代码
- 不改变 trpc-agent-go 框架层
- 不实现全自动化进化（Phase 4 仅半自动，需人工审批）

## Decisions

### D1: ExperienceReport 与现有 ExperienceReport 的关系

**决策**: 新建 `internal/biz/skill_intelligence_types.go`，定义 `SkillExperienceReport`（单次调用诊断报告），不复用 `ExperienceReport`（聚合型报告）。

**理由**: 两者字段、生命周期、消费者完全不同。聚合型报告面向 skills-butler 工具，单次诊断报告面向 SkillIntelligenceUsecase + 前端报告页。

### D2: SkillEvolutionSuggestion 与现有 SkillProposal 的关系

**决策**: 新建 `internal/biz/skill_evolution_suggestion_types.go`，定义 `SkillEvolutionSuggestion`，不复用 `SkillProposal`。

**理由**: SkillProposal 是"从零创建新 Skill"的提议；SkillEvolutionSuggestion 是"基于 Experience Report 对已有 Skill 的优化建议"。数据模型、审批队列、触发条件均不同。

### D3: skillrecommend.Rank 的集成方式

**决策**: 在 `ResolveSkillSlugsDetailed` 评分之后、返回之前，调用 `skillrecommend.Rank` 重排候选。通过依赖注入传入 `SkillHealthAggregator` 接口获取历史数据。

**理由**: 最小侵入式集成，不改变 ResolveSkillSlugsDetailed 的签名，仅在后处理阶段引入排序。

### D4: Curator Agent 的层级

**决策**: Curator Agent 装配与 invoke 走 `internal/service/skill_curator.go`，不在 biz 层 import trpc-agent-go。

**理由**: 遵守红线"biz 层禁止 import pkg/trpc-agent-go"。

### D5: Ent Schema vs Raw SQL

**决策**: experience_report 和 skill_evolution_suggestion 均使用 Ent Schema，与项目主流一致。

**理由**: 已有的 skill_proposals 使用 raw SQL 是历史遗留，新表应走 Ent。

## Risks / Trade-offs

- **[Risk] Phase 2~5 工作量大** → 分 Phase 交付，每个 Phase 独立可验证
- **[Risk] Curator Agent 调用 LLM 可能失败** → 实现降级逻辑：LLM 失败时仅保留规则层报告
- **[Risk] skillrecommend.Rank 引入延迟** → 缓存历史数据，Rank 函数目标 <5ms
- **[Risk] 前端审批 UI 复杂度** → Phase 4 先实现 API + 简单列表页，diff 对比 UI 后续迭代
