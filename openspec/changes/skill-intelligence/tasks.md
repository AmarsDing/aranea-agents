# Skill Intelligence 子系统 — 任务清单

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Skill Intelligence 子系统：调用可观测 → 经验报告诊断 → 推荐排序优化 → 半自动进化。

**Architecture:** 四层架构（Server → Service → Biz → Data）+ Wire DI + trpc-agent-go 框架集成。

**Design Doc:** [design.md](./design.md)

---

## Phase 1 — 可观测增强（SI-01~SI-04）

### Task 1: 扩展 skill_invocation Ent Schema

**Files:**
- Modify: `internal/data/ent/schema/skill_invocation.go`

- [ ] **Step 1:** 新增 `selection_reason` 字段（JSON 类型，默认 null），存储路由路径、候选 slug 列表、最终选中 slug、评分因子快照
- [ ] **Step 2:** 新增 `outcome` 字段（String 类型，默认 ""），枚举值：success / failure / partial / cancelled
- [ ] **Step 3:** 新增 `token_usage` 字段（JSON 类型，默认 null），存储 {prompt, completion, total}
- [ ] **Step 4:** 运行 `go generate ./internal/data/ent/...`

**DoD:** 三个新字段添加完成，代码生成无错误，`go build ./internal/data/...` 通过

---

### Task 2: 在 skillruntime 写入 selection_reason

**Files:**
- Modify: `internal/tools/skillruntime/resolve.go`

- [ ] **Step 1:** 在 `ResolveSkillSlugsDetailed` 返回 `ResolveResult` 时，将 `Reasons` map 序列化为 JSON 写入 `selection_reason` 字段
- [ ] **Step 2:** 确保 `ResolveResult` 结构体包含完整的评分因子信息（taxonomy score、embedding score 等）

**DoD:** 每次 Skill 调用的 `skill_invocation` 记录包含 `selection_reason`，`go build ./internal/tools/...` 通过

---

### Task 3: 在 Skill 执行完成后写入 outcome 和 token_usage

**Files:**
- Modify: `internal/biz/skill.go`（或相关 Skill 执行追踪代码）

- [ ] **Step 1:** Skill 执行完成后，根据执行结果判定 `outcome`（success / failure / partial / cancelled），写入 `skill_invocation`
- [ ] **Step 2:** 收集 LLM 调用的 token 使用量，写入 `token_usage` 字段

**DoD:** `skill_invocation` 记录包含 `outcome` 和 `token_usage`，`go build ./internal/biz/...` 通过

---

### Task 4: 实现 GetSkillHealth API

**Files:**
- Create: `api/kratos/skill/v1/skill_intelligence.proto`（或扩展 skill.proto）
- Create: `internal/service/skill_intelligence.go`
- Create: `internal/biz/skill_health.go`

- [ ] **Step 1:** 定义 `GetSkillHealth` proto，包含 7d/30d 成功率、P95 耗时、每日指标
- [ ] **Step 2:** 运行 `make api` 生成代码
- [ ] **Step 3:** 实现 `SkillHealthUsecase`，从 `skill_invocation` 聚合计算健康度指标
- [ ] **Step 4:** 实现 Service 层，proto↔biz 映射

**DoD:** `GET /v1/skills/{skill_id}/health` 返回正确的健康度数据，`make api && go build ./...` 通过

---

### Task 5: 前端 Skill 详情健康度卡片

**Files:**
- Modify: 前端 Skill 详情页组件

- [ ] **Step 1:** 在 Skill 详情页新增「健康度」卡片，调用 `GetSkillHealth` API
- [ ] **Step 2:** 展示 7d/30d 成功率折线图、P95 耗时指标

**DoD:** Skill 详情页可查看健康度指标，`pnpm lint && pnpm build` 通过

**Phase 1 验证：** `make api && make wire && make build && make test && make lint`；前端 `cd web && pnpm lint && pnpm build`

---

## Phase 2 — 经验报告与诊断（SI-10~SI-15）

### Task 6: 定义 ExperienceReport 领域模型

**Files:**
- Create: `internal/biz/skill_intelligence_types.go`

- [ ] **Step 1:** 创建 `ExperienceReport` 结构体（ID/TenantID/SessionID/InvocationID/SkillID/IsSuccess/Score/FailureTags/FlowSummary/OptimizationAdvice/SelectionSnapshot/GeneratedSuggestionID/CreatedAt）
- [ ] **Step 2:** 定义失败标签常量（param_mismatch / wrong_tool_choice / tool_timeout / tool_api_error / context_overflow / instruction_ambiguity / user_cancelled / unknown）

**DoD:** `ExperienceReport` 结构体和失败标签常量定义完成，`go build ./internal/biz/...` 通过

---

### Task 7: 定义 Repo 端口接口

**Files:**
- Create: `internal/biz/skill_intelligence_repo.go`

- [ ] **Step 1:** 定义 `ExperienceReportReader`（ListBySkill/GetByID/ListByTimeRange）接口
- [ ] **Step 2:** 定义 `ExperienceReportWriter`（Create/BatchCreate）接口
- [ ] **Step 3:** 定义 `SkillHealthAggregator`（GetHealthMetrics/GetFailureStats）接口

**DoD:** 端口接口定义完成，`go build ./internal/biz/...` 通过

---

### Task 8: 创建 Ent Schema experience_report

**Files:**
- Create: `internal/data/ent/schema/experience_report.go`

- [ ] **Step 1:** 创建 Ent Schema，字段：id/tenant_id/session_id/invocation_id/skill_id/is_success/score/failure_tags(JSON)/flow_summary/optimization_advice/selection_snapshot(JSON)/generated_suggestion_id/created_at
- [ ] **Step 2:** 添加索引：`(skill_id, created_at)`、`(invocation_id)`
- [ ] **Step 3:** 运行 `go generate ./internal/data/ent/...`

**DoD:** Ent Schema 创建完成，代码生成无错误，`go build ./internal/data/...` 通过

---

### Task 9: 实现 Data 层 Repo

**Files:**
- Create: `internal/data/skill_intelligence.go`
- Modify: `internal/data/data.go`（Wire 绑定）

- [ ] **Step 1:** 实现 `ExperienceReportReader` 和 `ExperienceReportWriter` 接口
- [ ] **Step 2:** 实现 `SkillHealthAggregator` 接口，从 `skill_invocation` 聚合计算
- [ ] **Step 3:** 在 `data.go` 中添加 Wire 绑定

**DoD:** Repo 实现完成，`go build ./internal/data/...` 通过

---

### Task 10: 实现 SkillIntelligenceUsecase

**Files:**
- Create: `internal/biz/skill_intelligence.go`

- [ ] **Step 1:** 实现 `SkillIntelligenceUsecase` 结构体，注入 `ExperienceReportWriter`/`ExperienceReportReader`/`SkillHealthAggregator`
- [ ] **Step 2:** 实现 `AnalyzeInvocation` 方法：规则层提取结构化字段（成败、耗时、失败标签）
- [ ] **Step 3:** 实现 `ScoreSkill` 方法：按配置权重计算 0–100 综合评分
- [ ] **Step 4:** 实现 `GenerateReport` 方法：规则层 + LLM 层（可降级）生成 ExperienceReport

**DoD:** Usecase 实现完成，`go build ./internal/biz/...` 通过

---

### Task 11: 实现 SkillIntelligenceWorker 定时任务

**Files:**
- Create: `internal/cronrunner/jobs/skill_intelligence_worker.go`

- [ ] **Step 1:** 创建定时任务，可配置间隔（默认 15min），`SKILL_INTELLIGENCE_DISABLED=1` 可关
- [ ] **Step 2:** 扫描近 N 小时新增且含 Skill 调用的 session
- [ ] **Step 3:** 对每个 invocation 调用 `SkillIntelligenceUsecase.GenerateReport`
- [ ] **Step 4:** 若触发阈值则写入 `skill_evolution_suggestion`（Phase 4 实现，此处预留接口）

**DoD:** Worker 实现完成，`go build ./internal/cronrunner/...` 通过

---

### Task 12: 实现 ListExperienceReports / GetExperienceReport API

**Files:**
- Modify: `api/kratos/skill/v1/skill_intelligence.proto`
- Modify: `internal/service/skill_intelligence.go`

- [ ] **Step 1:** 定义 `ListExperienceReports` / `GetExperienceReport` proto
- [ ] **Step 2:** 运行 `make api` 生成代码
- [ ] **Step 3:** 实现 Service 层，proto↔biz 映射

**DoD:** API 可用，`make api && go build ./...` 通过

---

### Task 13: 前端经验报告列表页

**Files:**
- Modify: 前端 Skill 管理相关页面

- [ ] **Step 1:** 新增「负熵报告」列表页，按 Skill / 评分 / 时间筛选
- [ ] **Step 2:** 支持跳转对应对话

**DoD:** 报告列表页可用，`pnpm lint && pnpm build` 通过

**Phase 2 验证：** Worker 关闭时不影响对话；开启后对模拟失败调用 24h 内产出 Report；`make api && make wire && make build && make test && make lint`

---

## Phase 3 — 推荐排序（SI-20~SI-24）

### Task 14: 实现 skillrecommend.Rank

**Files:**
- Create: `internal/tools/skillrecommend/rank.go`

- [ ] **Step 1:** 创建 `skillrecommend` 包，实现 `Rank` 函数：对候选 slug 列表按语义相似度 × 历史成功率 × 耗时倒数 × 用户偏好重排
- [ ] **Step 2:** 缺数据时该项取中性值 0.5；新 Skill（< 7d）可配置探索加成 +0.1
- [ ] **Step 3:** 排序因子快照写入 `selection_reason`

**DoD:** `Rank` 函数实现完成，`go build ./internal/tools/...` 通过

---

### Task 15: 集成推荐排序到 skillruntime

**Files:**
- Modify: `internal/tools/skillruntime/resolve.go`

- [ ] **Step 1:** 在 `ResolveSkillSlugsDetailed` 评分之后，调用 `skillrecommend.Rank` 重排候选
- [ ] **Step 2:** 确保 `ResolveResult.Reasons` 包含推荐排序因子

**DoD:** 推荐排序集成完成，`go build ./internal/tools/...` 通过

---

### Task 16: 去重候选组

**Files:**
- Create: `internal/biz/skill_dedup.go`
- Create: `internal/service/skill_dedup.go`

- [ ] **Step 1:** 实现 `DetectDuplicateSkills` 方法：名称不同但 description + 正文相似度 ≥ 0.2 的 Skill 归组
- [ ] **Step 2:** 实现 `MergeSkills` 方法：保留主 Skill，副 Skill archived，合并后 invoke 统计归并
- [ ] **Step 3:** 新增 API：`ListSkillDuplicateGroups` / `MergeSkills`

**DoD:** 去重检测与合并操作可用，`go build ./...` 通过

**Phase 3 验证：** 同一 intent 在 A/B 两个 Skill 成功率差 ≥ 30% 时，推荐排序 80% 以上选高成功率者（基准测试集）；`make api && make wire && make build && make test && make lint`

---

## Phase 4 — 半自动进化（SI-30~SI-36）

### Task 17: 定义 SkillEvolutionSuggestion 领域模型

**Files:**
- Create: `internal/biz/skill_evolution_suggestion_types.go`（或扩展 `skill_intelligence_types.go`）

- [ ] **Step 1:** 定义 `SkillEvolutionSuggestion` 结构体（ID/SkillID/Type/Status/SourceReportIDs/DraftSkillVersionID/SandboxPassed/CreatedAt/ResolvedAt）
- [ ] **Step 2:** 定义类型枚举（fix_failure / boost_efficiency / merge_duplicate）和状态枚举（pending / approved / rejected / applied）

**DoD:** 领域模型定义完成，`go build ./internal/biz/...` 通过

---

### Task 18: 创建 Ent Schema skill_evolution_suggestion

**Files:**
- Create: `internal/data/ent/schema/skill_evolution_suggestion.go`

- [ ] **Step 1:** 创建 Ent Schema，字段：id/skill_id/type/status/source_report_ids(JSON)/draft_skill_version_id/sandbox_passed/created_at/resolved_at
- [ ] **Step 2:** 添加索引：`(skill_id, type, status)`，同 Skill 同 type 去重
- [ ] **Step 3:** 运行 `go generate ./internal/data/ent/...`

**DoD:** Ent Schema 创建完成，代码生成无错误

---

### Task 19: 实现进化建议 Repo 与 Usecase

**Files:**
- Modify: `internal/biz/skill_intelligence_repo.go`
- Modify: `internal/data/skill_intelligence.go`
- Modify: `internal/biz/skill_intelligence.go`

- [ ] **Step 1:** 定义 `SkillEvolutionSuggestionReader`/`Writer` 端口接口
- [ ] **Step 2:** 实现 Data 层 Repo
- [ ] **Step 3:** 在 `SkillIntelligenceUsecase` 中实现触发条件判定（30d 失败 ≥ 3 且评分 < 60，或 P95 恶化 ≥ 20%）
- [ ] **Step 4:** 实现 `CreateSuggestion` / `ApproveSuggestion` / `RejectSuggestion` 方法

**DoD:** 进化建议 Usecase 实现完成，`go build ./...` 通过

---

### Task 20: 实现 Curator Agent 草案生成

**Files:**
- Create: `internal/service/skill_curator.go`

- [ ] **Step 1:** 实现 Curator Agent 装配与 invoke：输入原 Skill markdown + Experience Report，输出新 draft 版本 + evolution_reason
- [ ] **Step 2:** Curator 调用走 `internal/service`，不在 `biz` 层 import `pkg/trpc-agent-go`

**DoD:** Curator Agent 可生成 Skill 草案，`go build ./...` 通过

---

### Task 21: 实现 Sandbox Runner 验证

**Files:**
- Modify: `internal/service/skill_curator.go`

- [ ] **Step 1:** 用历史失败 case 重放 ≥ 1 次，记录 sandbox 成败
- [ ] **Step 2:** 隔离 Runner 执行，不影响生产环境

**DoD:** Sandbox 验证可用，`go build ./...` 通过

---

### Task 22: 实现进化建议 API + 人工审批 UI

**Files:**
- Modify: `api/kratos/skill/v1/skill_intelligence.proto`
- Modify: `internal/service/skill_intelligence.go`
- Modify: 前端 Skill 管理页面

- [ ] **Step 1:** 定义 `ListSkillEvolutionSuggestions` / `ApproveSkillEvolutionSuggestion` / `RejectSkillEvolutionSuggestion` proto
- [ ] **Step 2:** 实现 Service 层
- [ ] **Step 3:** 前端实现审批 UI：对比 diff + sandbox 结果；批准后 `PublishSkill`

**DoD:** 进化建议 API + 审批 UI 可用，`make api && go build ./...` 通过

---

### Task 23: Skill 元数据扩展 + 版本血缘

**Files:**
- Modify: `internal/data/ent/schema/skill.go`（或 skill_version）
- Modify: 相关 Biz / Service 层

- [ ] **Step 1:** 新增 `parent_version_id` / `evolution_reason` / `lifecycle_status` 字段
- [ ] **Step 2:** `lifecycle_status` 枚举：active / shadow / deprecated（与 draft/published 正交）
- [ ] **Step 3:** 运行 `go generate ./internal/data/ent/...`

**DoD:** 元数据扩展完成，版本血缘可追溯

---

### Task 24: Wire DI 装配

**Files:**
- Modify: `internal/biz/biz.go`（ProviderSet）
- Modify: `cmd/admin/wire.go`（依赖注入）

- [ ] **Step 1:** 在 `biz.go` ProviderSet 中添加所有新增 Usecase 构造函数
- [ ] **Step 2:** 在 `wire.go` 中添加依赖注入参数
- [ ] **Step 3:** 运行 `make wire && make build`

**DoD:** Wire DI 装配完成，`make wire && make build` 通过

**Phase 4 验证：** 从 Report → 草案 → Sandbox → 人工发布全链路可走通；未审批草案 never 进入生产路由；`make api && make wire && make build && make test && make lint`

---

## Phase 5 — 测试与验证

### Task 25: 单元测试

**Files:**
- Create: `internal/biz/skill_intelligence_test.go`
- Create: `internal/tools/skillrecommend/rank_test.go`

- [ ] **Step 1:** 编写 `SkillIntelligenceUsecase` 单元测试：AnalyzeInvocation 失败标签提取、ScoreSkill 评分计算、GenerateReport 降级逻辑
- [ ] **Step 2:** 编写 `skillrecommend.Rank` 单元测试：排序因子权重、缺数据中性值、新 Skill 探索加成

**DoD:** 所有单元测试通过，`go test ./internal/biz/... ./internal/tools/skillrecommend/... -count=1` 绿色

---

### Task 26: 集成测试

**Files:**
- Create: `internal/service/skill_intelligence_test.go`

- [ ] **Step 1:** 编写端到端集成测试：触发 Skill 调用 → 生成 ExperienceReport → 触发进化建议 → Curator 生成草案 → 审批 → 发布
- [ ] **Step 2:** 验证未审批草案不进入生产路由

**DoD:** 集成测试通过，全链路可走通

**Phase 5 验证：** `make api && make wire && make build && make test && make lint`；前端 `cd web && pnpm lint && pnpm test && pnpm build`
