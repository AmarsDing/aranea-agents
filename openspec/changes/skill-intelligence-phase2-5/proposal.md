## Why

原 `2026-06-05-skill-intelligence` 变更仅完成 Phase 1（15.3%，11/72 步），Phase 2~5 共 61 步未实现即被归档。Skill Intelligence 子系统的核心价值——经验报告诊断、推荐排序优化、半自动进化——尚未落地。本变更续接 Phase 2~5 实施。

## What Changes

- **Phase 2**: 实现 ExperienceReport 领域模型、Repo 端口、Ent Schema、Data 层 Repo、SkillIntelligenceUsecase、Cron Worker、API、前端报告列表页
- **Phase 3**: 实现 skillrecommend.Rank 推荐排序、集成到 skillruntime、Skill 去重检测与合并
- **Phase 4**: 实现 SkillEvolutionSuggestion 领域模型、Ent Schema、Repo/Usecase、Curator Agent 草案生成、Sandbox Runner 验证、进化建议 API + 人工审批 UI、Skill 元数据扩展 + 版本血缘
- **Phase 5**: 单元测试 + 集成测试
- 补齐 Phase 1 遗留：Task 3 Step 2（token_usage 填充）、Task 5（前端健康度卡片）

## Capabilities

### New Capabilities
- `skill-intelligence-phase2`: 经验报告与诊断（ExperienceReport 领域模型、Repo、Usecase、Worker、API、前端）
- `skill-intelligence-phase3`: 推荐排序（skillrecommend.Rank + Skill 去重）
- `skill-intelligence-phase4`: 半自动进化（SkillEvolutionSuggestion + Curator Agent + Sandbox + 审批 UI）

### Modified Capabilities
- `skill-intelligence-phase1`: 补齐 token_usage 填充 + 前端健康度卡片

## Impact

- **biz 层**: 新增 SkillIntelligenceUsecase、SkillEvolutionSuggestionUsecase、skillrecommend 包、skill_dedup
- **data 层**: 新增 experience_report Ent Schema + Repo、skill_evolution_suggestion Ent Schema + Repo
- **service 层**: 新增 SkillIntelligenceService、SkillDedupService
- **tools 层**: 新增 skillrecommend 包
- **cron 层**: 新增 SkillIntelligenceWorker 定时任务
- **api 层**: 新增 skill_intelligence.proto（ListExperienceReports / GetExperienceReport / ListSkillEvolutionSuggestions / ApproveSkillEvolutionSuggestion / RejectSkillEvolutionSuggestion / ListSkillDuplicateGroups / MergeSkills）
- **前端**: Skill 详情健康度卡片、经验报告列表页、进化建议审批 UI
- **Wire DI**: 更新 biz.go / wire.go

## Non-goals

- 不改变 skill_invocation Ent Schema（Phase 1 已完成）
- 不改变 GetSkillHealth API（Phase 1 已完成）
- 不改变 skills_butler 工具集（memory-skills-butler 变更已完成）
- Curator Agent 不在 biz 层 import pkg/trpc-agent-go（走 service 层）
