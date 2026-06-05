# Skill Diagnosis — Phase 2 规格说明

> 阶段：Phase 2 — 经验报告与诊断
> 实现状态：未实现
> 前置依赖：Phase 1 可观测增强（selection_reason / outcome / token_usage）

---

## ADDED Requirements

### Requirement: ExperienceReport 领域模型

系统 SHALL 定义 `ExperienceReport` 领域模型，表示单次 Skill 调用的结构化诊断报告。

`ExperienceReport` MUST 包含以下字段：
- `ID`：报告唯一标识（UUID）
- `TenantID`：租户标识
- `SessionID`：来源会话标识
- `InvocationID`：关联的 `skill_invocation` 记录 ID
- `SkillID`：被分析的 Skill 标识
- `IsSuccess`：整体成败（bool）
- `Score`：0–100 的评分
- `FailureTags`：失败标签数组（成功调用为空）
- `FlowSummary`：调用链摘要
- `OptimizationAdvice`：可操作建议
- `SelectionSnapshot`：选型因子快照（JSON）
- `GeneratedSuggestionID`：若已触发进化建议，指向 `SkillEvolutionSuggestion` ID（可选）
- `CreatedAt`：创建时间

`ExperienceReport` 与 `types.SkillHealthDetail`（Phase 1 聚合型报告）定位不同：ExperienceReport 是单次调用诊断报告，面向 Skill 维度；SkillHealthDetail 是时间窗口聚合报告，面向 API 查询。

`ExperienceReport` 与 `biz.SkillHealth`（skills-butler 用的健康报告）定位不同：ExperienceReport 是持久化的诊断记录，SkillHealth 是实时聚合的分析结果。

#### Scenario: 失败调用生成 ExperienceReport

WHEN Skill 调用结果为 failure
AND SkillIntelligenceWorker 扫描到该 invocation
THEN MUST 生成一条 ExperienceReport，其中 IsSuccess 为 false，FailureTags 非空

#### Scenario: 成功调用生成 ExperienceReport

WHEN Skill 调用结果为 success
AND SkillIntelligenceWorker 扫描到该 invocation
THEN MUST 生成一条 ExperienceReport，其中 IsSuccess 为 true，FailureTags 为空

#### Scenario: ExperienceReport 关联原始 invocation

WHEN ExperienceReport 被创建
THEN InvocationID MUST 指向对应的 skill_invocation 记录
AND SelectionSnapshot MUST 包含该 invocation 的 selection_reason 数据

---

### Requirement: 失败标签枚举

系统 SHALL 定义失败标签枚举（Failure Tags v1），用于结构化分类 Skill 调用失败原因。

失败标签 MUST 包含以下值：
- `param_mismatch`：参数不匹配
- `wrong_tool_choice`：错误的工具选择
- `tool_timeout`：工具超时
- `tool_api_error`：工具 API 错误
- `context_overflow`：上下文溢出
- `instruction_ambiguity`：指令歧义
- `user_cancelled`：用户取消
- `unknown`：未知原因

失败标签 MUST 由规则层从 skill_invocation 的 error_code / error_message / outcome / duration_ms 等字段推导生成。

一条 ExperienceReport MUST 可以包含多个失败标签。

#### Scenario: error_code 指向工具超时

WHEN skill_invocation 的 error_code 包含 "timeout" 或 duration_ms 超过配置阈值
THEN FailureTags MUST 包含 "tool_timeout"

#### Scenario: error_message 指向 API 错误

WHEN skill_invocation 的 error_message 包含 "api_error" 或 HTTP 5xx 相关关键字
THEN FailureTags MUST 包含 "tool_api_error"

#### Scenario: 无法识别的失败原因

WHEN skill_invocation 失败但无法匹配任何已知标签模式
THEN FailureTags MUST 包含 "unknown"

#### Scenario: 多重失败原因

WHEN skill_invocation 同时存在超时和 API 错误
THEN FailureTags MUST 同时包含 "tool_timeout" 和 "tool_api_error"

---

### Requirement: SkillIntelligenceWorker 定时分析任务

系统 SHALL 提供 `SkillIntelligenceWorker` 定时分析任务，定期扫描 `skill_invocation` 记录并生成 ExperienceReport。

Worker MUST 通过 `cronrunner` 框架注册，默认扫描间隔为 15 分钟。

Worker 配置项 MUST 包含：
- `skill_intelligence.enabled`：总开关，默认 `false`
- `skill_intelligence.scan_interval`：扫描间隔，默认 `15m`
- `skill_intelligence.lookback_hours`：每次扫描回看窗口，默认 `24`
- `skill_intelligence.min_invocations_for_score`：低于此次数仅记录不评分，默认 `5`
- `skill_intelligence.score_weights`：评分权重，默认 `{"success_rate":0.4,"duration":0.25,"token":0.2,"feedback":0.15}`

Worker 行为 MUST 按以下顺序执行：
1. 从 `skill_invocation` + session 消息拉取待分析样本（回看窗口内的未分析记录）
2. 规则层提取结构化字段（成败、耗时、标签）
3. LLM 层生成摘要与建议（可降级：仅结构化）
4. 写入 `experience_report`
5. 若触发进化阈值则写入 `skill_evolution_suggestion`
6. 发布 `skill.intelligence.report_created` 事件

Worker MUST 独立于对话热路径运行，SHALL NOT 影响对话性能。

#### Scenario: Worker 定时扫描并生成报告

WHEN skill_intelligence.enabled 为 true
AND 到达 scan_interval 时间点
THEN Worker MUST 扫描 lookback_hours 内的未分析 skill_invocation
AND 为每条符合条件的 invocation 生成 ExperienceReport

#### Scenario: Worker 总开关关闭

WHEN skill_intelligence.enabled 为 false
THEN Worker MUST NOT 执行任何扫描

#### Scenario: 调用次数不足时不评分

WHEN 某个 Skill 在 lookback_hours 内的调用次数 < min_invocations_for_score
THEN 生成的 ExperienceReport MUST 仅记录结构化字段
AND Score MUST 为 0 或不设置

#### Scenario: Worker 不影响对话热路径

WHEN Worker 正在执行扫描分析
AND 同时有用户对话进行
THEN 对话响应 MUST NOT 受到 Worker 的延迟影响

---

### Requirement: LLM 分析降级

Worker 的 LLM 分析层 MUST 支持降级为仅结构化报告。

当 LLM 调用超时（30s）或失败时，Worker MUST 降级为仅写入规则层提取的结构化字段，FlowSummary 和 OptimizationAdvice 可为空。

降级时 MUST 通过 `loggateway.Logger` 记录 Warn 级别日志。

#### Scenario: LLM 分析超时降级

WHEN Worker 调用 LLM 生成摘要超过 30s
THEN MUST 取消 LLM 调用
AND 生成仅包含结构化字段的 ExperienceReport
AND FlowSummary 和 OptimizationAdvice MUST 为空字符串

#### Scenario: LLM 分析失败降级

WHEN Worker 调用 LLM 返回错误
THEN MUST 降级为仅结构化报告
AND MUST 通过 loggateway.Logger 记录 Warn 日志

#### Scenario: LLM 分析成功

WHEN Worker 调用 LLM 成功返回摘要和建议
THEN ExperienceReport MUST 包含 FlowSummary 和 OptimizationAdvice

---

### Requirement: 评分模型 v1

系统 SHALL 实现评分模型 v1，为 ExperienceReport 计算 0–100 的综合评分。

评分公式 MUST 为：
```
score = w_success_rate * success_rate_30d
      + w_duration     * (1 - norm_duration)
      + w_token        * (1 - norm_token_usage)
      + w_feedback     * feedback_score
```

其中：
- `w_success_rate`、`w_duration`、`w_token`、`w_feedback` 为可配置权重，默认值分别为 0.4、0.25、0.2、0.15
- `norm_duration` 和 `norm_token_usage` 为归一化值（0–1）
- `feedback_score` 为用户反馈评分（0–1）

缺数据时该因子 MUST 取中性值 0.5，避免新 Skill 被永久打压。

无用户反馈时，feedback 权重 MUST 均分到其他因子。

#### Scenario: 所有因子数据完整时计算评分

WHEN Skill 有 30d 成功率、耗时、token 使用和用户反馈数据
THEN 评分 MUST 按公式计算，各因子使用实际值

#### Scenario: 缺少 token 使用数据时评分

WHEN Skill 无 token 使用数据
THEN token 因子 MUST 取中性值 0.5
AND 其他因子正常计算

#### Scenario: 无用户反馈时权重重分配

WHEN 无用户反馈数据
THEN feedback 权重 MUST 均分到 success_rate、duration、token 三个因子
AND 评分 MUST 使用重分配后的权重计算

#### Scenario: 新 Skill 评分不被打压

WHEN Skill 调用次数不足 min_invocations_for_score
THEN 所有缺数据因子 MUST 取中性值 0.5
AND 评分 MUST NOT 因数据不足而显著偏低

---

### Requirement: Ent Schema experience_report

系统 SHALL 定义 `experience_report` Ent Schema，持久化 ExperienceReport。

Schema MUST 包含以下字段：
- `id`：String (UUID)，主键
- `tenant_id`：String，租户
- `session_id`：String，来源会话
- `invocation_id`：String，关联 `skill_invocation`
- `skill_id`：String，被分析 Skill
- `is_success`：Bool，整体成败
- `score`：Int，0–100
- `failure_tags`：JSON，失败标签数组
- `flow_summary`：Text，调用链摘要
- `optimization_advice`：Text，可操作建议
- `selection_snapshot`：JSON，选型因子快照
- `generated_suggestion_id`：String（可选），若已触发进化建议
- `created_at`：Time，创建时间

Schema MUST 定义以下索引：
- `(skill_id, created_at)`：按 Skill 和时间查询
- `(invocation_id)`：按 invocation 查询关联报告

#### Scenario: Ent Schema 生成正确的数据库表

WHEN 运行 Ent 代码生成
THEN MUST 生成 experience_report 表
AND 表结构 MUST 包含上述所有字段和索引

#### Scenario: 同一 invocation 不生成重复报告

WHEN Worker 扫描到已生成 ExperienceReport 的 invocation
THEN MUST NOT 为同一 invocation 重复创建 ExperienceReport
AND invocation_id 索引 MUST 支持快速去重检查

---

### Requirement: ListExperienceReports API

系统 SHALL 提供 `ListExperienceReports` API，分页查询 ExperienceReport 列表。

API 路径 MUST 为 `GET /v1/skills/intelligence/reports`。

请求参数 MUST 支持：
- `skill_id`（可选）：按 Skill 过滤
- `is_success`（可选）：按成败过滤
- `min_score` / `max_score`（可选）：按评分范围过滤
- `failure_tag`（可选）：按失败标签过滤
- `page_size` / `page_token`：分页参数

响应 MUST 包含 ExperienceReport 列表和下一页 token。

#### Scenario: 按 Skill ID 过滤报告

WHEN 请求 GET /v1/skills/intelligence/reports?skill_id=xxx
THEN MUST 仅返回该 Skill 的 ExperienceReport

#### Scenario: 按失败标签过滤报告

WHEN 请求 GET /v1/skills/intelligence/reports?failure_tag=tool_timeout
THEN MUST 仅返回 FailureTags 包含 "tool_timeout" 的 ExperienceReport

#### Scenario: 分页查询报告

WHEN 请求 GET /v1/skills/intelligence/reports?page_size=10
THEN MUST 返回最多 10 条记录
AND 若有更多记录，响应 MUST 包含 next_page_token

---

### Requirement: GetExperienceReport API

系统 SHALL 提供 `GetExperienceReport` API，查询单条 ExperienceReport 详情。

API 路径 MUST 为 `GET /v1/skills/intelligence/reports/{id}`。

响应 MUST 包含完整的 ExperienceReport 信息，包括 FlowSummary、OptimizationAdvice 和 SelectionSnapshot。

#### Scenario: 查询存在的报告

WHEN 请求 GET /v1/skills/intelligence/reports/{id}
AND 报告存在
THEN MUST 返回完整的 ExperienceReport

#### Scenario: 查询不存在的报告

WHEN 请求 GET /v1/skills/intelligence/reports/{id}
AND 报告不存在
THEN MUST 返回 NOT_FOUND 错误

---

### Requirement: skill.intelligence.report_created 事件

Worker 在生成 ExperienceReport 后 MUST 发布 `skill.intelligence.report_created` 事件。

事件负载 MUST 包含：
- `report_id`：ExperienceReport ID
- `skill_id`：Skill 标识
- `is_success`：成败
- `score`：评分
- `failure_tags`：失败标签列表

事件消费者（如 Monitor Audit）可用于可观测和告警。

#### Scenario: 报告创建后发布事件

WHEN Worker 成功写入 ExperienceReport
THEN MUST 发布 skill.intelligence.report_created 事件
AND 事件负载 MUST 包含 report_id、skill_id、is_success、score、failure_tags

#### Scenario: 降级报告也发布事件

WHEN Worker 降级生成仅结构化字段的 ExperienceReport
THEN MUST 仍然发布 skill.intelligence.report_created 事件

---

### Requirement: Worker 禁止在 biz 层 import trpc-agent-go

`SkillIntelligenceWorker` 的 Biz 层代码 MUST NOT import `pkg/trpc-agent-go`。

Curator Agent 调用 MUST 通过 `internal/service` 层执行，Biz 层仅定义端口接口。

#### Scenario: biz 层不依赖 trpc-agent-go

WHEN 检查 internal/biz/skill_intelligence*.go 的 import 列表
THEN MUST NOT 包含 "pkg/trpc-agent-go" 或其子包

#### Scenario: Curator 调用走 service 层

WHEN Worker 需要调用 Curator Agent 生成 Skill 草案
THEN MUST 通过 service 层定义的端口接口调用
AND 实际的 Agent invoke 逻辑 MUST 在 service 层实现
