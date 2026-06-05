# Skill Intelligence Phase 2~5 — 任务清单

**Goal:** 续接原 skill-intelligence 变更 Phase 2~5，实现经验报告诊断、推荐排序优化、半自动进化。

**Architecture:** 四层架构（Server → Service → Biz → Data）+ Wire DI + trpc-agent-go 框架集成。

**Design Doc:** [design.md](./design.md)

**Non-goals:**
- 不改变已有 skill_health / experience_analytics / skill_evolution / skills_butler 代码
- 不改变 trpc-agent-go 框架层
- 不实现全自动化进化（仅半自动，需人工审批）

---

## 1. Phase 1 补齐 — token_usage + 前端健康度卡片

- [x] 1.1 在 `internal/agent/tool_invocation_recorder.go` 中，Skill 执行完成后从 LLM response 提取 token usage，填充 `SkillInvocationWrite.TokenUsage`。DoD: `skill_invocation.token_usage` 包含 {prompt, completion, total}
- [x] 1.2 在前端 Skill 详情页新增「健康度」卡片组件，调用 `GetSkillHealth` API。DoD: 卡片显示 7d/30d 成功率和 P95 耗时
- [x] 1.3 验证：`go build ./internal/agent/...` 通过，前端 `pnpm lint && pnpm build` 通过

## 2. Phase 2 — 经验报告与诊断

- [x] 2.1 创建 `internal/biz/skill_intelligence_types.go`：定义 `SkillExperienceReport` 结构体和失败标签常量。DoD: `go build ./internal/biz/...` 通过
- [x] 2.2 创建 `internal/biz/skill_intelligence_repo.go`：定义 `ExperienceReportReader`/`ExperienceReportWriter`/`SkillHealthAggregator` 接口。DoD: `go build ./internal/biz/...` 通过
- [x] 2.3 创建 `internal/data/ent/schema/experience_report.go`：Ent Schema + 索引。DoD: `go generate ./internal/data/ent/...` 无错误
- [x] 2.4 创建 `internal/data/skill_intelligence.go`：实现 Repo 接口 + Wire 绑定。DoD: `go build ./internal/data/...` 通过
- [x] 2.5 创建 `internal/biz/skill_intelligence.go`：实现 `SkillIntelligenceUsecase`（AnalyzeInvocation/ScoreSkill/GenerateReport）。DoD: `go build ./internal/biz/...` 通过
- [x] 2.6 创建 `internal/cronrunner/jobs/skill_intelligence_worker.go`：定时任务。DoD: `go build ./internal/cronrunner/...` 通过
- [x] 2.7 定义 `skill_intelligence.proto`：ListExperienceReports / GetExperienceReport。DoD: `make api` 通过
- [x] 2.8 实现 Service 层：`internal/service/skill_intelligence.go`。DoD: `go build ./internal/service/...` 通过
- [x] 2.9 前端经验报告列表页。DoD: `pnpm lint && pnpm build` 通过
- [x] 2.10 Wire DI 装配。DoD: `make wire && make build` 通过

## 3. Phase 3 — 推荐排序

- [x] 3.1 创建 `internal/tools/skillrecommend/rank.go`：实现 `Rank` 函数。DoD: `go build ./internal/tools/...` 通过
- [x] 3.2 集成到 `internal/tools/skillruntime/resolve.go`：在 `ResolveSkillSlugsDetailed` 评分后调用 Rank 重排。DoD: 排序因子写入 `selection_reason`
- [x] 3.3 创建 `internal/biz/skill_dedup.go` + `internal/service/skill_dedup.go`：去重检测与合并。DoD: `go build ./...` 通过
- [x] 3.4 新增 API：ListSkillDuplicateGroups / MergeSkills。DoD: `make api && go build ./...` 通过

## 4. Phase 4 — 半自动进化

- [x] 4.1 创建 `internal/biz/skill_evolution_suggestion_types.go`：领域模型 + 类型/状态枚举。DoD: `go build ./internal/biz/...` 通过
- [x] 4.2 创建 `internal/data/ent/schema/skill_evolution_suggestion.go`：Ent Schema + 索引。DoD: `go generate ./internal/data/ent/...` 无错误
- [x] 4.3 扩展 `skill_intelligence_repo.go`：定义 `SkillEvolutionSuggestionReader`/`Writer` 端口。DoD: `go build ./internal/biz/...` 通过
- [x] 4.4 实现 Data 层 Repo。DoD: `go build ./internal/data/...` 通过
- [x] 4.5 在 `SkillIntelligenceUsecase` 中实现触发条件判定 + CreateSuggestion/ApproveSuggestion/RejectSuggestion。DoD: `go build ./internal/biz/...` 通过
- [x] 4.6 创建 `internal/service/skill_curator.go`：Curator Agent 装配与 invoke。DoD: `go build ./internal/service/...` 通过
- [x] 4.7 实现 Sandbox Runner 验证。DoD: 隔离执行，不影响生产
- [x] 4.8 定义进化建议 API proto + 实现 Service 层。DoD: `make api && go build ./...` 通过
- [x] 4.9 前端审批 UI。DoD: `pnpm lint && pnpm build` 通过
- [x] 4.10 Skill 元数据扩展：新增 parent_version_id / evolution_reason / lifecycle_status 字段。DoD: `go generate ./internal/data/ent/...` 无错误
- [x] 4.11 Wire DI 装配。DoD: `make wire && make build` 通过

## 5. Phase 5 — 测试与验证

- [x] 5.1 编写 `internal/biz/skill_intelligence_test.go`：AnalyzeInvocation/ScoreSkill/GenerateReport 单测。DoD: `go test ./internal/biz/... -run TestSkillIntelligence -count=1` 绿色
- [x] 5.2 编写 `internal/tools/skillrecommend/rank_test.go`：排序因子权重/缺数据/探索加成。DoD: `go test ./internal/tools/skillrecommend/... -count=1` 绿色
- [x] 5.3 编写 `internal/service/skill_intelligence_test.go`：端到端集成测试。DoD: 全链路可走通
- [x] 5.4 全量验证：`make api && make wire && make build && make test && make lint`；前端 `cd web && pnpm lint && pnpm test && pnpm build`
