# Skill Intelligence 子系统设计文档

> 日期：2026-06-02
> 状态：Draft
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

### 1.2 skill_invocation 现有字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | String | UUID |
| `skill_id` | String | 关联 Skill |
| `agent_id` | String | 执行 Agent |
| `session_id` | String | 来源会话 |
| `status` | String | 当前仅 pending/running/completed/failed |
| `duration_ms` | Int | 耗时 |
| `error_code` | String | 错误码 |
| `error_message` | Text | 错误信息 |
| `input_json` / `output_json` | Text | 输入输出 |
| `skill_version` | String | 版本 |
| `source` | String | 来源（默认 runtime） |
| `activation_id` / `message_id` | String | 关联 ID |

### 1.3 skillruntime 现有评分机制

`ResolveSkillSlugsDetailed` 已实现两层过滤 + 评分：

1. Layer A：策略允许/拒绝过滤
2. Layer B：`skillrouter.DetectIntentPaths()` 意图路由 + 标签过滤
3. 评分：taxonomy path 精确匹配 +1000、部分匹配 +400、关键词 +100、embedding 可选

**缺失**：无历史成功率、耗时、用户偏好等运行时反馈因子。

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

### 3.1 skill_invocation 字段扩展

| 新增字段 | 类型 | 默认值 | 说明 |
|----------|------|--------|------|
| `selection_reason` | JSON | `null` | 路由路径、候选 slug 列表、最终选中 slug、评分因子快照 |
| `outcome` | String | `""` | 枚举：`success` / `failure` / `partial` / `cancelled` |
| `token_usage` | JSON | `null` | `{prompt: int, completion: int, total: int}` |

**写入点**：`skillruntime.ResolveSkillSlugsDetailed` 返回后、`skill_invocation` 创建时，将 `ResolveResult.Reasons` 序列化写入 `selection_reason`。

**非功能**：写入异步、不阻塞 Runner；热路径仅多写几个字段。

### 3.2 GetSkillHealth API

```protobuf
rpc GetSkillHealth(GetSkillHealthRequest) returns (SkillHealth) {
  option (google.api.http) = { get: "/v1/skills/{skill_id}/health" };
}

message SkillHealth {
  string skill_id = 1;
  int32 total_invocations_7d = 2;
  int32 success_count_7d = 3;
  double success_rate_7d = 4;
  int32 p95_duration_ms_7d = 5;
  int32 total_invocations_30d = 6;
  double success_rate_30d = 7;
  int32 p95_duration_ms_30d = 8;
  repeated DailyMetric daily_metrics = 9;
}

message DailyMetric {
  string date = 1;
  int32 invocations = 2;
  int32 successes = 3;
  double avg_duration_ms = 4;
}
```

### 3.3 管理面健康度卡片

Skill 详情页新增「健康度」卡片，展示 7d/30d 成功率折线、P95 耗时。

---

## 四、Phase 2 — 经验报告与诊断

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

## 五、Phase 3 — 推荐排序

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

## 六、Phase 4 — 半自动进化

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
