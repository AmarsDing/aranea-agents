# Skill Intelligence 子系统设计文档

> 日期：2026-06-02（最后更新：2026-06-05，文档与代码对齐更新）
> 状态：Draft（Phase 1 部分已实现，文档已与代码对齐）
> 范围：Skill 调用可观测 → 经验报告诊断 → 推荐排序优化 → 半自动进化
> 来源：`docs/_deprecated/需求/skill-lifecycle-requirements.md`

---

## 一、当前项目现状

### 1.1 已有能力

| 能力 | 实现位置 | 状态 |
|------|----------|------|
| Skill CRUD + 发布/启用 | `internal/service/skill.go` + `internal/biz/skill.go` | ✅ |
| 磁盘 watch 同步 | `internal/skill/watch/runner.go` | ✅ |
| 冲突检测 + AI 炼化 | `internal/skill/importer/engine.go` | ✅ |
| 运行记录 `skill_invocation` | `internal/data/ent/schema/skill_invocation.go` | ✅ |
| 意图路由 `skillrouter` | `internal/tools/skillrouter/detect.go` | ✅ |
| 运行时装配 `skillruntime` | `internal/tools/skillruntime/resolve.go` | ✅ |
| Skill 自创建提议 | `skill-evolution-auto-creator` 变更 | 🚧 |
| Skill 进化提议 CRUD | `internal/biz/skill_evolution.go` + `internal/biz/skill_evolution_types.go` + `internal/biz/skill_evolution_repo.go` | ✅ |
| `skill_proposals` 表 | `internal/data/skill_evolution.go` + `internal/data/sql/skill_evolution.sql`（原始 SQL，非 Ent Schema） | ✅ |
| Skill 进化扫描 | `internal/biz/skill_evolution.go`（`ScanAndProposeAll`） | ✅ |

### 1.2 skill_invocation 现有字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | String | UUID |
| `skill_id` | String | 关联 Skill |
| `agent_id` | String | 执行 Agent |
| `session_id` | String | 来源会话 |
| `user_id` | String | 用户 ID |
| `status` | String | 当前仅 pending/running/completed/failed |
| `duration_ms` | Int | 耗时 |
| `started_at` | String | 开始时间 |
| `ended_at` | String | 结束时间 |
| `error_code` | String | 错误码 |
| `error_message` | Text | 错误信息 |
| `input_json` / `output_json` | Text | 输入输出 |
| `input_preview` | Text | 输入预览（截断） |
| `input_hash` | String | 输入哈希 |
| `output_preview` | Text | 输出预览（截断） |
| `skill_version` | String | 版本 |
| `source` | String | 来源（默认 runtime） |
| `activation_id` / `message_id` | String | 关联 ID |
| `selection_reason` | JSON（可选） | ✅ **已实现** — 路由路径、候选 slug 列表、最终选中 slug、评分因子快照 |
| `outcome` | String（默认 ""） | ✅ **已实现** — 枚举：success / failure / partial / cancelled |
| `token_usage` | JSON（可选） | ✅ **Schema 已实现**，运行时暂未实际采集 token 数据写入 |

### 1.3 skillruntime 现有评分机制

`ResolveSkillSlugsDetailed` 已实现两层过滤 + 评分：

1. Layer A：策略允许/拒绝过滤
2. Layer B：`skillrouter.DetectIntentPaths()` 意图路由 + 标签过滤
3. 评分：taxonomy path 精确匹配 +1000、部分匹配 +400、关键词 +100、embedding 可选

**缺失**：无历史成功率、耗时、用户偏好等运行时反馈因子。

### 1.4 已实现的 Skill Health 能力（Phase 1 部分）

以下能力已在 Phase 1 实施过程中落地，但不在原始设计文档中：

| 能力 | 实现位置 | 说明 |
|------|----------|------|
| `SkillHealthUsecase` | `internal/biz/skill_health.go` | 从 `skill_invocation` 聚合 7d/30d 健康度指标 |
| `SkillHealthReader` 端口 | `internal/biz/skill_health.go` | Repo 端口接口 |
| `skillHealthRepo` | `internal/data/skill_health.go` | Ent 实现，含 P95 计算、日维度聚合 |
| `SkillHealthDetail` 类型 | `internal/biz/types/skill_health.go` | 7d/30d 成功率、P95 耗时、每日指标 |
| `DailyMetric` 类型 | `internal/biz/types/skill_health.go` | 每日指标（Date/Invocations/Successes/AvgDurationMs） |
| `GetSkillHealth` API | `api/kratos/skill/v1/skill.proto` + `internal/service/skill.go` | `GET /v1/skills/{skill_id}/health` |
| `selection_reason` 写入 | `internal/agent/skill_guidance_inject.go` + `internal/agent/tool_invocation_recorder.go` | 通过 invocation state 传递 Reasons → 写入 skill_invocation |
| `outcome` 推导 | `internal/agent/tool_invocation_recorder.go` | 从 tool invocation status 推导 success/failure/partial |
| `ExperienceAnalyticsUsecase` | `internal/biz/experience_analytics.go` | skills-butler 用的分析能力（AnalyzeSkillHealth/AnalyzeToolWeights/AnalyzeOrchestration/AnalyzeMemoryQuality/AnalyzeAgentCapability） |
| `skills_butler_analyze_skill_health` 工具 | `internal/tools/skills_butler/analyze_skill_health.go` | Agent 可主动查询 Skill 健康状态 |
| `skillsButlerAnalyticsAdapter` | `internal/service/skills_butler_adapter.go` | 桥接 `ExperienceAnalyticsUsecase` → `AnalyticsPort`，将 `SkillHealthAnalysis.Items` 转换为 `[]biz.SkillHealth` |
| `SkillHealth` 类型 | `internal/biz/experience_analytics_types.go` | skills-butler 用的健康报告类型（SkillID/InvokeCount7d/SuccessRate/AvgDurationMS/Trend/HealthStatus/Recommendation） |
| `SkillHealthItem` 类型 | `internal/biz/experience_analytics.go` | ExperienceAnalytics 内部聚合类型（SkillID/SkillName/InvokeCount/SuccessCount/FailureCount/SuccessRate/AvgDurationMS/HealthStatus/Recommendation） |
| `ExperienceReport` 类型 | `internal/biz/types/skill_health.go` | 聚合型报告（SkillSlug/Period/HealthScore/SuccessRate 等），与 Phase 2 设计的单次调用诊断报告不同 |
| `SkillInvocationStat` 类型 | `internal/biz/skill_invocation_stats.go` | Skill 调用统计（SkillName/Count/SuccessRate/AvgDurationMs） |
| `SkillInvocationStatsReader` 端口 | `internal/biz/skill_invocation_stats.go` | Skill 调用统计查询端口 |
| `SkillProposal` 类型 | `internal/biz/skill_evolution_types.go` | Skill 进化提议（ID/AgentID/PatternHash/PatternDesc/SkillName/SkillMD/Status/ApprovedBy/RejectedBy/CreatedAt/ApprovedAt） |
| `SkillProposalStatus` 枚举 | `internal/biz/skill_evolution_types.go` | pending/approved/rejected/registered/expired |
| `SkillProposalReadWriter` 端口 | `internal/biz/skill_evolution_repo.go` | Skill 提议 CRUD 端口 |
| `SkillEvolutionUsecase` | `internal/biz/skill_evolution.go` | Skill 进化提议生命周期管理（DetectAndPropose/ApproveProposal/RejectProposal/RegisterApproved/ScanAndProposeAll） |
| `SkillAutoCreator` 端口 | `internal/biz/skill_evolution.go` | AI 生成 SKILL.md 的端口（GenerateSKILLMD） |
| `SkillRegistrationPort` 端口 | `internal/biz/skill_evolution.go` | Skill 注册端口（RegisterSkill/SkillExists） |
| `skillProposalRepo` | `internal/data/skill_evolution.go` | 原始 SQL 实现（非 Ent），使用 `skill_proposals` 表 |
| `skill_proposals` 表 | `internal/data/sql/skill_evolution.sql` | 原始 SQL DDL，含 idx_sprop_agent_status 和 idx_sprop_pattern_hash 索引 |
| `skill_evolution_schema.go` | `internal/data/skill_evolution_schema.go` | DDL 嵌入加载器，通过 `//go:embed sql/skill_evolution.sql` 将 DDL 嵌入二进制，`EnsureSkillEvolutionSchema` 在启动时执行建表 |
| `SkillEvolutionUsecase` 单元测试 | `internal/biz/skill_evolution_test.go` | 覆盖 ApproveProposal/RejectProposal/RegisterApproved/GetProposal/ListProposals/DetectAndPropose/CreateProposal 等方法，含去重、低置信度过滤、冲突检测等场景 |
| `isSuccess` 兼容逻辑 | `internal/data/skill_health.go` | `skillHealthRepo` 的 `isSuccess` 函数：优先检查 `outcome` 字段（`"success"` → true），`outcome` 为空时回退到 `status` 字段（`"completed"` 或 `"success"` → true），兼容未填充 outcome 的历史记录 |
| `SkillService.healthUC` 注入 | `internal/service/skill.go` | `SkillService` 构造函数注入 `*biz.SkillHealthUsecase`，`GetSkillHealth` 方法在 `healthUC == nil` 时返回 `ServiceUnavailable` |
| `skillsButlerSkillUsecaseAdapter` | `internal/service/skills_butler_adapter.go` | 桥接 `SkillEvolutionUsecase` → `SkillUsecasePort`（ListProposals/ApproveProposal/RejectProposal/RegisterApproved/CreateProposal） |
| `skillsButlerEvolutionAdapter` | `internal/service/skills_butler_adapter.go` | 桥接 `EvolutionUsecase` → `EvolutionUsecasePort`（GetEvolutionMetrics） |
| `skillsButlerQueryAdapter` | `internal/service/skills_butler_adapter.go` | 桥接 `SkillInvocationStatsReader` → `SkillQueryReaderPort`（GetSkillInvocationStats） |
| `skillsButlerRegistrationAdapter` | `internal/service/skills_butler_adapter.go` | 桥接 `SkillUsecase` → `SkillRegistrationPort`（RegisterSkill/SkillExists） |
| `skills_butler_recommend_skills` 工具 | `internal/tools/skills_butler/recommend_skills.go` | 基于 Skill 使用模式和进化建议推荐新增/优化/移除的 Skill |
| `skills_butler_evolve_skill` 工具 | `internal/tools/skills_butler/evolve_skill.go` | 为指定 Agent 创建 Skill 进化提议（通过 SkillProposal 队列） |
| `skills_butler_analyze_tool_weights` 工具 | `internal/tools/skills_butler/analyze_tool_weights.go` | 工具权重分析 |
| `skills_butler_analyze_orchestration` 工具 | `internal/tools/skills_butler/analyze_orchestration.go` | 编排模式分析 |
| `skills_butler_optimize_orchestration` 工具 | `internal/tools/skills_butler/optimize_orchestration.go` | 编排优化建议 |
| `skills_butler_optimize_skill` 工具 | `internal/tools/skills_butler/optimize_skill.go` | Skill 优化建议 |
| `skills_butler_analyze_skill_usage` 工具 | `internal/tools/skills_butler/analyze_skill_usage.go` | Skill 使用分析 |

---

## 二、架构设计

### 2.1 模块落点（遵循 AGENT_RUNTIME_BOUNDARY）

```text
internal/
├── biz/
│   ├── skill_intelligence.go          # 端口：ReportRepo, SuggestionRepo, 评分/触发规则
│   ├── skill_intelligence_types.go    # ExperienceReport / SkillEvolutionSuggestion 领域模型
│   ├── skill_intelligence_repo.go     # Repo 端口接口
│   └── skill_recommend.go             # 纯函数排序，无框架依赖
├── data/
│   └── skill_intelligence.go          # Ent 实现
├── data/ent/schema/
│   ├── experience_report.go           # 新增 Ent Schema
│   └── skill_evolution_suggestion.go  # 新增 Ent Schema
├── cronrunner/jobs/
│   └── skill_intelligence_worker.go   # 定时分析任务
├── service/
│   └── skill_intelligence.go          # Service 层（含 Curator Agent invoke）
└── tools/skillrecommend/
    └── rank.go                        # 运行时排序，由 skillruntime 调用
```

### 2.2 四层架构位置

```text
api/**/*.proto                              ← 新增 SkillIntelligence 相关 proto
        ↓
internal/service                            ← SkillIntelligenceService：proto↔biz 映射
        ↓
internal/biz                                ← SkillIntelligenceUsecase + 端口接口
        ↓
internal/data                               ← ReportRepo / SuggestionRepo 实现（Ent ORM）
```

### 2.3 数据流总览

```text
[对话结束] → [skill_invocation 入库]
                    ↓
[SkillIntelligenceWorker 定时扫描] ← 可配置间隔（默认 15min）
                    ↓
[规则层：提取结构化字段] → failure_tags / score / is_success
                    ↓
[LLM 层：生成摘要与建议] → flow_summary / optimization_advice
                    ↓
[ExperienceReport 入库] → 发布 skill.intelligence.report_created 事件
                    ↓
[触发阈值判定] → 若满足条件 → SkillEvolutionSuggestion 入队
                    ↓
[Curator Agent 生成草案] ← 人工审批 / Sandbox 验证
                    ↓
[新版本 draft → publish]
```

---

## 三、Phase 1 — 可观测增强

> **实现状态**：大部分已完成。`selection_reason`/`outcome`/`token_usage` Schema 已落地，`GetSkillHealth` API 已实现。`token_usage` 运行时采集和前端健康度卡片尚未实现。

### 3.1 skill_invocation 字段扩展 ✅ Schema 已实现（token_usage 运行时采集待实现）

| 新增字段 | 类型 | 默认值 | 说明 |
|----------|------|--------|------|
| `selection_reason` | JSON | `null` | ✅ 路由路径、候选 slug 列表、最终选中 slug、评分因子快照 |
| `outcome` | String | `""` | ✅ 枚举：`success` / `failure` / `partial` / `cancelled`（`cancelled` 暂未使用） |
| `token_usage` | JSON | `null` | ⚠️ Schema + Data 层写入逻辑已实现，但运行时调用方未填充 `SkillInvocationWrite.TokenUsage` 字段 |

**写入点**：`skillruntime.ResolveSkillSlugsDetailed` 返回后、`skill_invocation` 创建时，将 `ResolveResult.Reasons` 序列化写入 `selection_reason`。

**实际实现**：`skill_guidance_inject.go` 在 BeforeModelHook 中调用 `ResolveSkillSlugsDetailed`，将 `result.Reasons` 通过 `inv.SetState(skillSelectionReasonStateKey, result.Reasons)` 存入 invocation state；`tool_invocation_recorder.go` 的 `recordSkillInvocation` 从 state 读取并写入 `skill_invocation.SelectionReason`。

**非功能**：写入异步、不阻塞 Runner；热路径仅多写几个字段。

### 3.2 GetSkillHealth API ✅ 已实现

> **注意**：实际 proto 返回类型名为 `SkillHealthMetric`（非设计中的 `SkillHealth`），每日指标类型为 `SkillHealthDailyMetric`（非设计中的 `DailyMetric`），并额外包含 `success_count_7d` 和 `success_count_30d` 字段。详见 §10.4 和 §10.8。

```protobuf
rpc GetSkillHealth(GetSkillHealthRequest) returns (SkillHealthMetric) {
  option (google.api.http) = { get: "/v1/skills/{skill_id}/health" };
}

message SkillHealthMetric {
  string skill_id = 1;
  int32 total_invocations_7d = 2;
  int32 success_count_7d = 3;
  double success_rate_7d = 4;
  int32 p95_duration_ms_7d = 5;
  int32 total_invocations_30d = 6;
  int32 success_count_30d = 7;
  double success_rate_30d = 8;
  int32 p95_duration_ms_30d = 9;
  repeated SkillHealthDailyMetric daily_metrics = 10;
}

message SkillHealthDailyMetric {
  string date = 1;
  int32 invocations = 2;
  int32 successes = 3;
  double avg_duration_ms = 4;
}
```

### 3.3 管理面健康度卡片 ❌ 未实现

Skill 详情页新增「健康度」卡片，展示 7d/30d 成功率折线、P95 耗时。

---

## 四、Phase 2 — 经验报告与诊断 ❌ 未实现

### 4.1 ExperienceReport 领域模型

```go
type ExperienceReport struct {
    ID                    string
    TenantID              string
    SessionID             string
    InvocationID          string
    SkillID               string
    IsSuccess             bool
    Score                 int
    FailureTags           []string
    FlowSummary           string
    OptimizationAdvice    string
    SelectionSnapshot     json.RawMessage
    GeneratedSuggestionID *string
    CreatedAt             time.Time
}
```

### 4.2 失败标签枚举（v1）

```go
const (
    FailureTagParamMismatch       = "param_mismatch"
    FailureTagWrongToolChoice     = "wrong_tool_choice"
    FailureTagToolTimeout         = "tool_timeout"
    FailureTagToolAPIError        = "tool_api_error"
    FailureTagContextOverflow     = "context_overflow"
    FailureTagInstructionAmbiguity = "instruction_ambiguity"
    FailureTagUserCancelled       = "user_cancelled"
    FailureTagUnknown             = "unknown"
)
```

### 4.3 SkillIntelligenceWorker

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `skill_intelligence.enabled` | `false` | 总开关 |
| `skill_intelligence.scan_interval` | `15m` | 扫描间隔 |
| `skill_intelligence.lookback_hours` | `24` | 每次扫描回看窗口 |
| `skill_intelligence.min_invocations_for_score` | `5` | 低于此次数仅记录不评分 |
| `skill_intelligence.score_weights` | `{"success_rate":0.4,"duration":0.25,"token":0.2,"feedback":0.15}` | 评分权重 |

**行为**：

1. 从 `skill_invocation` + session 消息拉取待分析样本
2. 规则层提取结构化字段（成败、耗时、标签）
3. LLM 层生成摘要与建议（可降级：仅结构化）
4. 写入 `experience_report`；若触发阈值则写入 `skill_evolution_suggestion`
5. 发布 `skill.intelligence.report_created` 事件

**分析执行者**：

- 结构化字段 → 规则 + SQL 聚合
- 自然语言摘要与建议 → 调用已配置 LLM（与导入炼化共用 provider 配置）
- **禁止**在 `internal/biz` import `pkg/trpc-agent-go`；Curator 调用走 `internal/service`

### 4.4 评分模型 v1

```text
score = w_success_rate * success_rate_30d
      + w_duration     * (1 - norm_duration)
      + w_token        * (1 - norm_token_usage)
      + w_feedback     * feedback_score
```

- 缺数据时该项取中性值 0.5，避免新 Skill 被永久打压
- 无用户反馈时 feedback 权重均分到其他因子

### 4.5 Ent Schema: experience_report

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | String (UUID) | 主键 |
| `tenant_id` | String | 租户 |
| `session_id` | String | 来源会话 |
| `invocation_id` | String | 关联 `skill_invocation` |
| `skill_id` | String | 被分析 Skill |
| `is_success` | Bool | 整体成败 |
| `score` | Int | 0–100 |
| `failure_tags` | JSON | 失败标签数组 |
| `flow_summary` | Text | 调用链摘要 |
| `optimization_advice` | Text | 可操作建议 |
| `selection_snapshot` | JSON | 选型因子快照 |
| `generated_suggestion_id` | String (可选) | 若已触发进化建议 |
| `created_at` | Time | 创建时间 |

索引：`(skill_id, created_at)`、`(invocation_id)`

### 4.6 API

```protobuf
rpc ListExperienceReports(ListExperienceReportsRequest) returns (ListExperienceReportsResponse) {
  option (google.api.http) = { get: "/v1/skills/intelligence/reports" };
}
rpc GetExperienceReport(GetExperienceReportRequest) returns (ExperienceReport) {
  option (google.api.http) = { get: "/v1/skills/intelligence/reports/{id}" };
}
```

---

## 五、Phase 3 — 推荐排序 ❌ 未实现

> **已有相关实现**：`skills_butler_recommend_skills` 工具（`internal/tools/skills_butler/recommend_skills.go`）已实现基于使用模式的 Skill 推荐（检测低成功率、低使用率 Skill），但这是 Agent 维度的离线推荐，不是运行时路由排序。Phase 3 的 `skillrecommend.Rank` 是在 `ResolveSkillSlugsDetailed` 热路径中引入历史反馈因子重排候选，两者定位不同。

### 5.1 插入点

在 `skillruntime.ResolveSkillSlugsDetailed` 评分之后、返回结果之前：

```text
candidates := skillrouter.Detect(...)
ranked     := skillrecommend.Rank(ctx, candidates, intent, userCtx)
selected   := ranked[:policy.MaxSkillsInToolset]
```

### 5.2 排序公式（v1）

```text
score = w1 * semantic_sim + w2 * success_rate_30d + w3 * (1 / norm_duration) + w4 * user_pref
```

- 缺数据时该项取中性值 0.5
- 新 Skill（< 7d）可配置「探索加成」+0.1，防止马太效应

### 5.3 因子快照

排序因子写入 `selection_reason`，便于事后解释「为何选 B 而非 A」。

### 5.4 去重候选组

名称不同但 description + 正文相似度 ≥ 0.2 的 Skill 归组，管理面展示「建议合并」。合并操作保留主 Skill，副 Skill `archived`。

---

## 六、Phase 4 — 半自动进化 ❌ 未实现

> **已有相关实现**：`skills_butler_evolve_skill` 工具（`internal/tools/skills_butler/evolve_skill.go`）已实现创建 Skill 进化提议的能力，但仅创建 `SkillProposal`（轻量提议，不含草案正文），走 `skill-evolution-auto-creator` 的审批队列。Phase 4 的 `SkillEvolutionSuggestion` 是基于 Experience Report 触发的、包含草案正文和 Sandbox 验证的完整进化流程，两者产出类型和审批队列不同。

### 6.1 SkillEvolutionSuggestion 领域模型

```go
type SkillEvolutionSuggestion struct {
    ID                  string
    SkillID             string
    Type                string    // fix_failure / boost_efficiency / merge_duplicate
    Status              string    // pending / approved / rejected / applied
    SourceReportIDs     []string
    DraftSkillVersionID *string
    SandboxPassed       *bool
    CreatedAt           time.Time
    ResolvedAt          *time.Time
}
```

### 6.2 触发条件

- 同一 Skill 30d 内失败 ≥ 3 次且评分 < 60
- 或成功但 P95 耗时较基线恶化 ≥ 20%

### 6.3 进化流程

| 步骤 | 执行者 | 输出 |
|------|--------|------|
| 聚合失败模式 | Worker 规则 | `failure_tags[]` |
| 生成优化建议 | LLM | `optimization_advice` |
| 判定是否触发进化 | Worker 规则 | `SkillEvolutionSuggestion` |
| 生成 Skill 草案 | Curator Agent（service 层 invoke） | 新 draft + diff |
| Sandbox 重放 | `internal/service` + 隔离 Runner | `sandbox_result` |
| 人工审批 | 管理面 | publish / reject |

### 6.4 Ent Schema: skill_evolution_suggestion

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | String | 主键 |
| `skill_id` | String | 目标 Skill |
| `type` | String | fix_failure / boost_efficiency / merge_duplicate |
| `status` | String | pending / approved / rejected / applied |
| `source_report_ids` | JSON | 依据报告 ID 列表 |
| `draft_skill_version_id` | String (可选) | 生成的草案版本 |
| `sandbox_passed` | Bool (可选) | Sandbox 验证结果 |
| `created_at` | Time | 创建时间 |
| `resolved_at` | Time (可选) | 处理时间 |

索引：`(skill_id, type, status)`，同 Skill 同 type 去重

### 6.5 Skill 元数据扩展

在现有 `skill` / `skill_version` 上扩展：

| 字段 | 类型 | 说明 |
|------|------|------|
| `parent_version_id` | String (可选) | 血缘 |
| `evolution_reason` | String (可选) | fix_failure / boost_efficiency |
| `lifecycle_status` | String | active / shadow / deprecated（与 draft/published 正交） |

### 6.6 API

```protobuf
rpc ListSkillEvolutionSuggestions(ListSkillEvolutionSuggestionsRequest) returns (ListSkillEvolutionSuggestionsResponse) {
  option (google.api.http) = { get: "/v1/skills/evolution-suggestions" };
}
rpc ApproveSkillEvolutionSuggestion(ApproveSkillEvolutionSuggestionRequest) returns (SkillEvolutionSuggestion) {
  option (google.api.http) = { post: "/v1/skills/evolution-suggestions/{id}/approve" };
}
rpc RejectSkillEvolutionSuggestion(RejectSkillEvolutionSuggestionRequest) returns (SkillEvolutionSuggestion) {
  option (google.api.http) = { post: "/v1/skills/evolution-suggestions/{id}/reject" };
}
```

---

## 七、与现有模块的接口

### 7.1 与 skill-evolution-auto-creator 的关系

| 维度 | auto-creator | intelligence |
|------|-------------|-------------|
| 触发 | 重复工具调用模式 | Skill 调用失败/低效 |
| 产出 | 新 Skill（从零创建） | 已有 Skill 的优化版本 |
| 审批 | SkillProposal 队列 | SkillEvolutionSuggestion 队列 |
| 共用 | LLM 配置、Sandbox Runner、审批 UI 模式 | 同左 |

### 7.2 与 Manage-Agent / Memory-Agent 的接口

| 依赖 | 方式 | 说明 |
|------|------|------|
| Memory 用户偏好 | `biz` 端口 `UserPreferenceReader` | Phase 3 可选；无实现时权重跳过 |
| Tools 可用性 | 现有 tool health / invocation 统计 | 失败标签 `tool_api_error` 时引用 |
| Manage-Agent 规划 | **不耦合** | Manage 可查询 Skill 推荐 API；Skill Intelligence 不回调 Manage |

### 7.3 事件

| 事件 | 消费者 | 动作 |
|------|--------|------|
| `skill.invocation.completed` | Intelligence Worker | 纳入下轮扫描 |
| `skill.intelligence.report_created` | Monitor Audit | 可观测 |
| `skill.evolution.suggestion_created` | 管理面通知 | 待办 |
| `skill.published` | watch + trpc cache | 热加载 |

---

## 八、错误处理

| 场景 | 处理方式 |
|------|----------|
| LLM 分析超时 | 30s 超时取消，降级为仅结构化报告，记录 FlowLog Warn |
| LLM 分析成本过高 | 采样 + 仅分析失败/低分调用；可配置每日上限 |
| 进化引入回归 | Sandbox + 人工审批 + 版本回滚 |
| 推荐加剧「富者愈富」 | 新 Skill 探索加成 + 最低 invocation 阈值 |
| 磁盘 watch 与 DB 草案冲突 | 进化产出仅写 DB + 文件；watch 以 DB published 为准 |
| Worker 关闭 | 不影响对话；Worker 是独立 cronrunner 任务 |

---

## 九、风险与约束

| 风险 | 缓解 |
|------|------|
| LLM 分析成本 | 采样 + 仅分析失败/低分调用；可配置每日上限 |
| 进化引入回归 | Sandbox + 人工审批 + 版本回滚 |
| 推荐加剧「富者愈富」 | 新 Skill 探索加成 + 最低 invocation 阈值 |
| 与 Agent Evolution 概念混淆 | Skill 进化管能力包；Agent Evolution 管 Agent 配置；共用 Worker 模式但分表分队列 |
| 磁盘 watch 与 DB 草案冲突 | 进化产出仅写 DB + 文件；watch 以 DB published 为准 |

---

## 十、代码实现与设计差异记录

> 本节记录代码实际实现与原始设计文档的差异，供后续实施参考。

### 10.1 selection_reason 写入机制

**设计**：在 `skillruntime.ResolveSkillSlugsDetailed` 返回后、`skill_invocation` 创建时直接写入。

**实际实现**：采用两步传递机制：
1. `skill_guidance_inject.go`（BeforeModelHook）调用 `ResolveSkillSlugsDetailed`，将 `result.Reasons` 通过 `inv.SetState(skillSelectionReasonStateKey, result.Reasons)` 存入 trpc-agent-go 的 invocation state
2. `tool_invocation_recorder.go` 的 `recordSkillInvocation` 从 invocation state 读取，写入 `skill_invocation.SelectionReason`

这种设计是因为 skill 路由发生在 BeforeModelHook 阶段，而 skill_invocation 写入发生在 AfterToolHook 阶段，两者不在同一调用栈中，需要通过 invocation state 中转。

### 10.2 outcome 推导逻辑

**设计**：枚举值 success / failure / partial / cancelled。

**实际实现**：`recordSkillInvocation` 从 `ToolInvocationWrite.Status` 推导：
- `"success"` → `"success"`
- `"error"` → `"failure"`
- 其他 → `"partial"`

`cancelled` 枚举值暂未使用（blocked/confirmation_required 的 tool invocation 不记录 skill_invocation）。

### 10.3 token_usage 字段

**设计**：收集 LLM 调用的 token 使用量，写入 `{prompt, completion, total}`。

**实际状态**：Ent Schema 和 Data 层写入逻辑已支持，但运行时暂未实际采集 token 数据写入 `token_usage` 字段。`SkillInvocationWrite.TokenUsage` 字段存在但调用方未填充。

### 10.4 GetSkillHealth API 实现差异

**设计**：独立 proto 文件 `skill_intelligence.proto`，独立 Service `SkillIntelligenceService`，返回类型 `SkillHealth`。

**实际实现**：合并到现有 `skill.proto` 和 `SkillService` 中：
- proto 定义在 `api/kratos/skill/v1/skill.proto` 的 `SkillService` 服务内
- 返回类型为 `SkillHealthMetric`（非设计中的 `SkillHealth`），每日指标类型为 `SkillHealthDailyMetric`（非设计中的 `DailyMetric`）
- Service 层实现在 `internal/service/skill.go` 的 `SkillService.GetSkillHealth` 方法
- `SkillService` 构造函数注入 `*biz.SkillHealthUsecase`
- Biz 层为独立的 `SkillHealthUsecase`（`internal/biz/skill_health.go`）
- Data 层为独立的 `skillHealthRepo`（`internal/data/skill_health.go`）
- `SkillHealthReader` 端口接口定义在 `internal/biz/skill_health.go`，方法签名 `GetSkillHealth(ctx, skillID, since7d, since30d)`

### 10.5 ExperienceReport 类型差异

**设计**（Phase 2）：单次调用诊断报告，字段包括 ID/TenantID/SessionID/InvocationID/SkillID/IsSuccess/Score/FailureTags/FlowSummary/OptimizationAdvice/SelectionSnapshot/GeneratedSuggestionID/CreatedAt。

**实际实现**（`internal/biz/types/skill_health.go`）：聚合型报告，字段为 SkillSlug/Period/HealthScore/SuccessRate/AvgLatencyMs/InvocationCount/FailurePatterns/OptimizationSuggestions/GeneratedAt。这是 skills-butler 使用的分析能力，与 Phase 2 设计的单次调用诊断报告定位不同。Phase 2 实施时需注意区分两者。

### 10.6 ExperienceAnalyticsUsecase（额外实现）

代码中已实现 `ExperienceAnalyticsUsecase`（`internal/biz/experience_analytics.go`），提供以下分析能力供 skills-butler 使用：
- `AnalyzeToolWeights`：工具权重分析（success_rate×0.5 + call_count×0.3 + 1/duration×0.2）
- `AnalyzeSkillHealth`：Skill 健康分析（按 success_rate 分级：healthy/degraded/unstable/critical/unused）
- `AnalyzeOrchestration`：编排模式分析（DQ = 0.4×Validity + 0.3×Specificity + 0.3×Correctness）
- `AnalyzeMemoryQuality`：记忆质量分析（0.4×coverage + 0.4×retrieval + 0.2×(1-penalty)）
- `AnalyzeAgentCapability`：综合能力分析（合并以上四项 + 成本摘要）

依赖注入（`NewExperienceAnalyticsUsecase`）：`EvolutionMetricsRepo` / `SkillQueryReader` / `TeamRepository` / `UsageAnalyticsRepo` / `*MemoryAdminUsecase` / `SessionReader` / `ToolInvocationReader` / `loggateway.Logger`。其中 `*MemoryAdminUsecase`（指针）可为 nil（降级时跳过记忆质量分析）；`SessionReader` 当前未被任何分析方法调用，但保留为未来扩展端口；其余依赖为必需。

适配层：`skillsButlerAnalyticsAdapter`（`internal/service/skills_butler_adapter.go`）将 `ExperienceAnalyticsUsecase` 的方法签名适配为 `AnalyticsPort` 接口，例如将 `AnalyzeSkillHealth(ctx, agentID, since)` 转换为 `AnalyzeSkillHealth(ctx) ([]biz.SkillHealth, error)`。

这些能力与 Skill Intelligence 的 Phase 2 诊断能力有重叠但定位不同：ExperienceAnalytics 面向 Agent 维度聚合，Skill Intelligence 面向 Skill 维度诊断。

### 10.7 skills_butler 工具集（额外实现）

代码中已实现完整的 skills-butler 工具集（`internal/tools/skills_butler/`），通过 `RegisterAll(deps Deps)` 注册到 Agent 运行时。**注册逻辑有条件分支**：`AnalyticsPort` 为 nil 时只注册 4 个基础工具（analyze_skill_usage / recommend_skills / evolve_skill / optimize_skill），`AnalyticsPort` 非 nil 时额外注册 4 个分析工具（analyze_skill_health / analyze_tool_weights / analyze_orchestration / optimize_orchestration）。

工具列表：

| 工具名 | 文件 | 说明 |
|--------|------|------|
| `skills_butler_analyze_skill_usage` | `analyze_skill_usage.go` | Skill 使用分析 |
| `skills_butler_recommend_skills` | `recommend_skills.go` | 基于 Skill 使用模式和进化建议推荐新增/优化/移除的 Skill |
| `skills_butler_evolve_skill` | `evolve_skill.go` | 为指定 Agent 创建 Skill 进化提议（通过 SkillProposal 队列） |
| `skills_butler_optimize_skill` | `optimize_skill.go` | Skill 优化建议 |
| `skills_butler_analyze_skill_health` | `analyze_skill_health.go` | Skill 健康状态分析（依赖 AnalyticsPort） |
| `skills_butler_analyze_tool_weights` | `analyze_tool_weights.go` | 工具权重分析（依赖 AnalyticsPort） |
| `skills_butler_analyze_orchestration` | `analyze_orchestration.go` | 编排模式分析（依赖 AnalyticsPort） |
| `skills_butler_optimize_orchestration` | `optimize_orchestration.go` | 编排优化建议（依赖 AnalyticsPort） |

此外，`errors.go` 定义了工具级错误常量（`errAgentIDRequired` / `errSkillNameRequired` / `errImprovementDescRequired` / `errTimeRangeRequired`），`registry.go` 定义了端口接口（`SkillUsecasePort` / `EvolutionUsecasePort` / `SkillQueryReaderPort` / `AnalyticsPort`）和 `Deps` 依赖注入结构。

其中 `skills_butler_evolve_skill` 与 Phase 4 的 Curator Agent 有功能重叠，但当前实现仅创建 `SkillProposal`（轻量提议），不生成 Skill 草案正文。Phase 4 实施时需注意区分 `SkillProposal`（auto-creator 产出）与 `SkillEvolutionSuggestion`（intelligence 产出）。

### 10.8 SkillHealthDetail 与 proto SkillHealthMetric 字段映射

设计文档 §3.2 中 proto 返回类型名为 `SkillHealth`，实际实现为 `SkillHealthMetric`。字段映射：

| Biz 类型 `SkillHealthDetail` 字段 | Proto `SkillHealthMetric` 字段 | 说明 |
|-----------------------------------|-------------------------------|------|
| `SkillID` | `skill_id` | 一致 |
| `TotalInvocations7d` | `total_invocations_7d` | 一致 |
| `SuccessCount7d` | `success_count_7d` | 设计中无此字段，实际 proto 已包含 |
| `SuccessRate7d` | `success_rate_7d` | 一致 |
| `P95DurationMs7d` | `p95_duration_ms_7d` | 一致 |
| `TotalInvocations30d` | `total_invocations_30d` | 一致 |
| `SuccessCount30d` | `success_count_30d` | 设计中无此字段，实际 proto 已包含 |
| `SuccessRate30d` | `success_rate_30d` | 一致 |
| `P95DurationMs30d` | `p95_duration_ms_30d` | 一致 |
| `DailyMetrics []DailyMetric` | `daily_metrics []SkillHealthDailyMetric` | proto 类型名不同 |

### 10.9 SkillEvolutionUsecase 实现差异（额外实现）

代码中已实现 `SkillEvolutionUsecase`（`internal/biz/skill_evolution.go`），这是 `skill-evolution-auto-creator` 变更的一部分，与 Skill Intelligence Phase 4 的进化建议有功能重叠但定位不同：

| 维度 | SkillEvolutionUsecase（已实现） | SkillEvolutionSuggestion（Phase 4 设计） |
|------|--------------------------------|------------------------------------------|
| 触发 | 重复工具调用模式检测 | Skill 调用失败/低效 |
| 产出 | `SkillProposal`（含 SkillMD 草案正文） | `SkillEvolutionSuggestion`（含草案版本 ID + Sandbox 验证） |
| 审批 | `SkillProposal` 队列（pending → approved → registered） | `SkillEvolutionSuggestion` 队列（pending → approved → applied） |
| 数据存储 | `skill_proposals` 表（原始 SQL，非 Ent Schema） | `skill_evolution_suggestion` 表（设计为 Ent Schema） |
| AI 生成 | `SkillAutoCreator.GenerateSKILLMD` 端口 | Curator Agent（service 层 invoke） |
| 注册 | `SkillRegistrationPort`（RegisterSkill/SkillExists） | 版本发布 + lifecycle_status 切换 |

**关键实现细节**：
- `skill_proposals` 表使用原始 SQL DDL（`internal/data/sql/skill_evolution.sql`），而非 Ent Schema，通过 `skillProposalRepo`（`internal/data/skill_evolution.go`）直接操作
- `SkillEvolutionUsecase` 注入 `SkillAutoCreator` 端口（AI 生成 SKILL.md），该端口在 `internal/service` 层实现
- `ScanAndProposeAll` 方法遍历所有启用了 `EvolutionSkillEvolve` 配置的 Agent，批量触发检测
- `DetectAndPropose` 方法从 `PatternReader` 读取 `tool_call` 类型的检测模式，调用 AI 生成后创建 `SkillProposal`
- `skills_butler_evolve_skill` 工具通过 `skillsButlerSkillUsecaseAdapter` 调用 `SkillEvolutionUsecase.CreateProposal`，仅创建轻量提议（SkillMD 为空），不触发 AI 生成

### 10.10 skills_butler 适配层实现细节

`internal/service/skills_butler_adapter.go` 中实现了 5 个适配器，将 Biz 层 Usecase 适配为 skills_butler 工具所需的端口接口：

| 适配器 | 桥接 | 说明 |
|--------|------|------|
| `skillsButlerSkillUsecaseAdapter` | `SkillEvolutionUsecase` → `SkillUsecasePort` | ListProposals/ApproveProposal/RejectProposal/RegisterApproved/CreateProposal，uc 为 nil 时返回空 |
| `skillsButlerEvolutionAdapter` | `EvolutionUsecase` → `EvolutionUsecasePort` | GetEvolutionMetrics，uc 为 nil 时返回空 |
| `skillsButlerQueryAdapter` | `SkillInvocationStatsReader` → `SkillQueryReaderPort` | GetSkillInvocationStats，将 biz.SkillInvocationStat 转换为 skills_butler.SkillInvocationStat |
| `skillsButlerAnalyticsAdapter` | `ExperienceAnalyticsUsecase` → `AnalyticsPort` | AnalyzeToolWeights/AnalyzeSkillHealth/AnalyzeOrchestration，将 ExperienceAnalytics 的内部类型转换为 biz 级别类型；内含 `agentID` 字段，自动填充到每次分析调用 |
| `skillsButlerRegistrationAdapter` | `SkillUsecase` → `SkillRegistrationPort` | RegisterSkill/SkillExists，用于 SkillEvolutionUsecase 的注册端口；通过 `NewSkillsButlerRegistrationAdapter` 构造，uc 为 nil 时安全降级 |

所有适配器在 uc 为 nil 时安全降级（返回 nil/空值），不抛异常。`skillsButlerRegistrationAdapter` 例外：它通过独立构造函数 `NewSkillsButlerRegistrationAdapter` 创建，而非内联初始化。

### 10.11 skillHealthRepo 的 isSuccess 兼容逻辑

`skillHealthRepo`（`internal/data/skill_health.go`）在聚合健康度指标时，使用 `isSuccess(outcome, status)` 函数判断调用是否成功：

```go
func isSuccess(outcome, status string) bool {
    if outcome == "success" {
        return true
    }
    if outcome == "" && (status == "completed" || status == "success") {
        return true
    }
    return false
}
```

此逻辑为**向后兼容**设计：优先检查 `outcome` 字段（Phase 1 新增），当 `outcome` 为空时回退到 `status` 字段（原始字段），确保未填充 outcome 的历史记录也能正确统计。

### 10.12 SkillService 的 healthUC 注入与降级

`SkillService`（`internal/service/skill.go`）通过构造函数注入 `*biz.SkillHealthUsecase`：

```go
func NewSkillService(uc *biz.SkillUsecase, agentUC *biz.AgentUsecase, healthUC *biz.SkillHealthUsecase, ...) *SkillService
```

`GetSkillHealth` 方法在 `healthUC == nil` 时返回 `ServiceUnavailable` 错误（错误域 `SKILL_INTELLIGENCE`），确保未配置健康度功能时不会 panic。

### 10.13 skill_evolution_schema.go DDL 嵌入机制

`internal/data/skill_evolution_schema.go` 使用 Go 的 `//go:embed` 机制将 `sql/skill_evolution.sql` DDL 嵌入二进制文件：

```go
//go:embed sql/skill_evolution.sql
var skillEvolutionDDL string

func EnsureSkillEvolutionSchema(ctx context.Context, client *ent.Client) error {
    return execDDLFile(ctx, client, skillEvolutionDDL, "skill_evolution")
}
```

此函数在应用启动时被调用，确保 `skill_proposals` 表和索引存在。这是原始 SQL DDL 模式（非 Ent Schema）的标准加载方式。

### 10.14 SkillEvolutionUsecase 单元测试覆盖

`internal/biz/skill_evolution_test.go` 包含完整的单元测试，覆盖以下场景：

| 测试 | 说明 |
|------|------|
| `TestSkillEvolutionUsecase_ApproveProposal` | 审批 pending 提案 |
| `TestSkillEvolutionUsecase_ApproveProposal_NotPending` | 非 pending 提案不可审批 |
| `TestSkillEvolutionUsecase_RejectProposal` | 拒绝 pending 提案 |
| `TestSkillEvolutionUsecase_RegisterApproved` | 注册已审批提案 |
| `TestSkillEvolutionUsecase_RegisterApproved_Conflict` | 同名 Skill 已存在时返回 Conflict |
| `TestSkillEvolutionUsecase_RegisterApproved_NotApproved` | 非 approved 提案不可注册 |
| `TestSkillEvolutionUsecase_GetProposal` | 按 ID 查询提案 |
| `TestSkillEvolutionUsecase_GetProposal_EmptyID` | 空 ID 返回错误 |
| `TestSkillEvolutionUsecase_ListProposals` | 按 agentID + status 筛选 |
| `TestSkillEvolutionUsecase_DetectAndPropose_NoCreator` | 无 creator 时返回空 |
| `TestSkillEvolutionUsecase_DetectAndPropose_WithPatterns` | 有模式时生成提案 |
| `TestSkillEvolutionUsecase_DetectAndPropose_DedupByHash` | 重复模式去重 |
| `TestSkillEvolutionUsecase_DetectAndPropose_LowConfidence` | 低置信度模式过滤（< 0.15） |
| `TestSkillEvolutionUsecase_CreateProposal` | 手动创建提案 |
| `TestPatternHash_Deterministic` | 模式哈希确定性和大小写不敏感 |
| `TestExtractToolNamesFromDesc` | 从模式描述提取工具名 |

注意：这些测试属于 `skill-evolution-auto-creator` 变更的产出，不属于 Skill Intelligence Phase 1 的任务范围，但记录在此以便后续实施 Phase 4 时参考复用。

### 10.15 biz.SkillInvocation 查询结果类型缺少 Outcome/SelectionReason 字段

- **位置**：`internal/biz/skill/skill.go:112-134`（SkillInvocation struct）、`internal/data/skill.go:530-554`（SearchSkillInvocations mapping）
- **问题**：Ent 生成的 SkillInvocation entity 包含 Outcome/SelectionReason/TokenUsage 字段，但 Biz 层查询结果类型 biz.SkillInvocation 缺少这些字段。Data 层的 SearchSkillInvocations 在映射时跳过了这三个字段。
- **影响**：所有通过 SkillQueryReader.SearchSkillInvocations 消费数据的代码（如 ExperienceAnalyticsUsecase.AnalyzeSkillHealth）无法访问 Outcome 和 SelectionReason，只能依赖 Status 字段。

### 10.16 ExperienceAnalyticsUsecase.AnalyzeSkillHealth 成败判断逻辑与 skillHealthRepo 不一致

- **位置**：`internal/biz/experience_analytics.go:283-285` vs `internal/data/skill_health.go:107-115`
- **问题**：存在两套不同的成败判断逻辑：
  - ExperienceAnalyticsUsecase 使用 `inv.Status == "success"` 和 `inv.Status == "failure"`
  - skillHealthRepo 使用 `isSuccess(outcome, status)` 函数，优先检查 outcome，回退到 status
- **关键**：skill_invocation 表的 status 列值为 "success"/"error"/"pending"/"running"/"completed"/"failed"，**不存在 "failure" 值**。"failure" 是 outcome 枚举值。因此 `inv.Status == "failure"` 永远不会匹配，导致 SkillHealthItem.FailureCount 始终为 0。
- **根因**：§10.15 — SkillInvocation 查询结果缺少 Outcome 字段，分析代码只能使用 Status 但使用了错误的枚举值。

### 10.17 skillInvocationStatsRepo 查询 tool_invocation 表而非 skill_invocation 表

- **位置**：`internal/data/skill_invocation_stats.go:23-28`
- **问题**：skillInvocationStatsRepo.GetSkillInvocationStats 查询的是 tool_invocation 表（使用 ToolInvocation Ent client）而非 skill_invocation 表。使用 ToolKey 作为 skill name，实际分析的是 tool 调用而非 skill 调用。
- **影响**：skills_butler 的 analyze_skill_usage/recommend_skills/optimize_skill 工具获取的是 tool-call 维度的统计，而非 skill-call 维度的统计。

### 10.18 两套独立的 SkillHealth 类型

- **位置**：`internal/biz/experience_analytics_types.go:14-22`（biz.SkillHealth）vs `internal/biz/types/skill_health.go:29-41`（types.SkillHealthDetail）
- **问题**：存在两套完全不同的"Skill Health"类型，服务于不同场景：
  - biz.SkillHealth：SkillID/InvokeCount7d/SuccessRate/AvgDurationMS/Trend/HealthStatus/Recommendation — 用于 skills_butler 工具
  - types.SkillHealthDetail：SkillID/TotalInvocations7d/SuccessCount7d/SuccessRate7d/P95DurationMs7d/...30d/DailyMetrics — 用于 GetSkillHealth API
- 两者数据源不同、判断逻辑不同。

### 10.19 biz.SkillHealth 类型注释枚举值与实际产出值不匹配

- **位置**：`internal/biz/experience_analytics_types.go:14-22`
- **问题**：注释标注的枚举值与实际产出值不匹配：
  - HealthStatus：注释说 healthy/warning/critical/dormant，实际产出 unused/healthy/degraded/unstable/critical
  - Recommendation：注释说 keep/evolve/retire/merge，实际产出 consider_removing/keep/review_errors/investigate_failures/disable_or_rewrite
  - Trend：一致（rising/stable/declining/dormant）

### 10.20 ExperienceAnalyticsUsecase 中间分析类型

- **位置**：`internal/biz/experience_analytics.go:15-98`
- **问题**：§10.6 记录了 5 个分析方法但未记录它们返回的中间分析类型：
  - AnalyzeToolWeights → ToolWeightAnalysis（含 []ToolWeightItem）
  - AnalyzeSkillHealth → SkillHealthAnalysis（含 []SkillHealthItem）
  - AnalyzeOrchestration → OrchestrationAnalysis（含 []OrchestrationModeItem）
  - AnalyzeMemoryQuality → MemoryQualityAnalysis
  - AnalyzeAgentCapability → AgentCapabilityAnalysis（合并以上 + CostSummary）
  - 中间类型拥有更丰富的字段（如 ToolWeightItem 有 normSR/normCount/normInvDur），biz 类型是简化版本。

### 10.21 EvolutionUsecase 与 SkillEvolutionUsecase 的区分

- **位置**：`internal/biz/evolution.go`（EvolutionUsecase）vs `internal/biz/skill_evolution.go`（SkillEvolutionUsecase）
- **问题**：存在两个不同的进化相关 Usecase。§10.9 仅详述了 SkillEvolutionUsecase 而未记录 EvolutionUsecase：
  - EvolutionUsecase：产出 EvolutionSuggestion（persona/prompt 类型），使用 EvolutionSuggestionRepo
  - SkillEvolutionUsecase：产出 SkillProposal（tool_call 类型），使用 skill_proposals 表（原始 SQL）
  - 两者在 skills_butler_adapter.go 中有各自的适配器

### 10.22 skillHealthRepo 和 skillInvocationStatsRepo 使用不同的数据源表

- **位置**：`internal/data/skill_health.go`（查询 skill_invocation）vs `internal/data/skill_invocation_stats.go`（查询 tool_invocation）
- **问题**：两者都为 skills_butler 功能提供统计数据，但查询不同的数据库表，可能对同一 skill 产出不一致的结果。

### 10.23 OrchestrationModeReport biz 类型部分字段始终为零值

- **位置**：`internal/biz/experience_analytics_types.go:34-41` vs `internal/biz/experience_analytics.go:56-71`
- **问题**：biz.OrchestrationModeReport 的 AvgTokens/AvgDurationSec/MemberContributions 字段从未被适配器填充（skillsButlerAnalyticsAdapter.AnalyzeOrchestration 仅映射 Mode/SuccessRate/DQScore），这些字段始终为零值。
