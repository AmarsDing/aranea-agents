# Skill Observability — Phase 1 规格说明

> 阶段：Phase 1 — 可观测增强
> 实现状态：部分已实现（selection_reason / outcome / GetSkillHealth API 已落地；token_usage 运行时采集和前端健康度卡片未实现）

---

## ADDED Requirements

### Requirement: skill_invocation selection_reason 字段

每次 Skill 调用 SHALL 在 `skill_invocation` 记录中写入 `selection_reason` JSON 字段，记录路由路径、候选 slug 列表、最终选中 slug 及评分因子快照。

`selection_reason` MUST 包含以下结构化信息：
- 路由路径（Layer A 策略过滤 + Layer B 意图路由）
- 候选 slug 列表
- 最终选中 slug
- 评分因子快照（各因子的原始值与权重）

写入 MUST 通过 invocation state 中转机制实现：`skill_guidance_inject.go` 在 BeforeModelHook 中将 `ResolveResult.Reasons` 存入 invocation state，`tool_invocation_recorder.go` 在记录 skill_invocation 时从 state 读取并写入。

写入 MUST 异步执行，SHALL NOT 阻塞 Runner 对话热路径。

#### Scenario: 正常路由选择写入 selection_reason

WHEN skillruntime.ResolveSkillSlugsDetailed 返回包含 Reasons 的结果
AND tool_invocation_recorder 记录 skill_invocation
THEN skill_invocation.SelectionReason MUST 包含路由路径、候选列表、选中 slug 和评分因子

#### Scenario: 路由无候选时 selection_reason 为空

WHEN ResolveSkillSlugsDetailed 返回空候选集
AND tool_invocation_recorder 记录 skill_invocation
THEN skill_invocation.SelectionReason MUST 为 null 或空 JSON

#### Scenario: selection_reason 写入不阻塞对话

WHEN selection_reason 写入过程中发生序列化错误
THEN 对话流程 MUST NOT 被阻塞
AND 错误 MUST 通过 loggateway.Logger 记录 Warn 级别日志

---

### Requirement: skill_invocation outcome 字段

每次 Skill 调用 SHALL 在 `skill_invocation` 记录中写入 `outcome` 字段，表示调用的最终结果。

`outcome` MUST 为以下枚举值之一：
- `success`：调用成功完成
- `failure`：调用失败
- `partial`：调用部分完成
- `cancelled`：调用被取消（暂未使用，预留）

outcome MUST 从 `ToolInvocationWrite.Status` 推导：
- `"success"` → `"success"`
- `"error"` → `"failure"`
- 其他状态 → `"partial"`

`outcome` 字段默认值为空字符串 `""`，兼容未填充的历史记录。

#### Scenario: 成功调用推导 outcome 为 success

WHEN ToolInvocationWrite.Status 为 "success"
THEN skill_invocation.outcome MUST 被设置为 "success"

#### Scenario: 错误调用推导 outcome 为 failure

WHEN ToolInvocationWrite.Status 为 "error"
THEN skill_invocation.outcome MUST 被设置为 "failure"

#### Scenario: 其他状态推导 outcome 为 partial

WHEN ToolInvocationWrite.Status 不是 "success" 也不是 "error"
THEN skill_invocation.outcome MUST 被设置为 "partial"

#### Scenario: 历史记录 outcome 为空时兼容

WHEN skill_invocation 记录的 outcome 字段为空字符串
THEN 统计逻辑 MUST 回退到 status 字段判断成败

---

### Requirement: skill_invocation token_usage 字段运行时采集

每次 Skill 调用 SHALL 在 `skill_invocation` 记录中写入 `token_usage` JSON 字段，记录 LLM 调用的 token 使用量。

`token_usage` MUST 包含以下结构：
- `prompt`：输入 token 数量
- `completion`：输出 token 数量
- `total`：总 token 数量

运行时 MUST 在 Skill 调用完成后从 trpc-agent-go 运行时获取 token 使用数据，并填充到 `SkillInvocationWrite.TokenUsage` 字段。

当运行时无法获取 token 数据时，`token_usage` MUST 为 null，SHALL NOT 抛出异常。

#### Scenario: Skill 调用包含 LLM token 使用数据

WHEN Skill 调用完成且 trpc-agent-go 运行时返回 token 使用数据
THEN skill_invocation.token_usage MUST 包含 prompt/completion/total 字段

#### Scenario: Skill 调用无 LLM token 数据

WHEN Skill 调用完成但运行时未返回 token 使用数据
THEN skill_invocation.token_usage MUST 为 null

#### Scenario: token_usage 采集不阻塞对话

WHEN token 数据采集过程中发生错误
THEN 对话流程 MUST NOT 被阻塞
AND 错误 MUST 通过 loggateway.Logger 记录 Warn 级别日志

---

### Requirement: GetSkillHealth API

系统 SHALL 提供 `GetSkillHealth` API，返回指定 Skill 的健康度指标。

API 路径 MUST 为 `GET /v1/skills/{skill_id}/health`。

返回类型 `SkillHealthMetric` MUST 包含：
- `skill_id`：Skill 标识
- `total_invocations_7d`：7 天总调用次数
- `success_count_7d`：7 天成功次数
- `success_rate_7d`：7 天成功率
- `p95_duration_ms_7d`：7 天 P95 耗时
- `total_invocations_30d`：30 天总调用次数
- `success_count_30d`：30 天成功次数
- `success_rate_30d`：30 天成功率
- `p95_duration_ms_30d`：30 天 P95 耗时
- `daily_metrics`：每日指标列表（`SkillHealthDailyMetric`：date/invocations/successes/avg_duration_ms）

API MUST 在 `SkillService` 中实现，通过构造注入 `SkillHealthUsecase`。

当 `SkillHealthUsecase` 未注入（为 nil）时，API MUST 返回 `ServiceUnavailable` 错误。

#### Scenario: 查询有调用记录的 Skill 健康度

WHEN 请求 GET /v1/skills/{skill_id}/health
AND 该 Skill 在 7d/30d 内有调用记录
THEN 返回 SkillHealthMetric MUST 包含正确的调用次数、成功次数、成功率和 P95 耗时

#### Scenario: 查询无调用记录的 Skill 健康度

WHEN 请求 GET /v1/skills/{skill_id}/health
AND 该 Skill 在 7d/30d 内无调用记录
THEN 返回 SkillHealthMetric MUST 包含零值指标（invocations=0, success_rate=0）

#### Scenario: SkillHealthUsecase 未注入时返回 ServiceUnavailable

WHEN 请求 GET /v1/skills/{skill_id}/health
AND SkillService 的 healthUC 为 nil
THEN MUST 返回 ServiceUnavailable 错误，错误域为 SKILL_INTELLIGENCE

---

### Requirement: SkillHealthUsecase 与 SkillHealthReader 端口

`SkillHealthUsecase` SHALL 作为健康度查询的业务层入口，从 `skill_invocation` 聚合 7d/30d 健康度指标。

`SkillHealthReader` MUST 定义为 Biz 层端口接口，方法签名为 `GetSkillHealth(ctx, skillID, since7d, since30d)`。

Data 层 MUST 提供 `skillHealthRepo` 实现 `SkillHealthReader` 端口，使用 Ent ORM 查询 `skill_invocation` 表。

#### Scenario: SkillHealthUsecase 聚合 7d 和 30d 指标

WHEN 调用 SkillHealthUsecase.GetSkillHealth(ctx, skillID, since7d, since30d)
THEN MUST 返回包含 7d 和 30d 时间窗口的成功率、P95 耗时、每日指标的 SkillHealthDetail

#### Scenario: SkillHealthReader 端口可被 Mock 替换

WHEN 单元测试需要隔离 Data 层
THEN SkillHealthReader 端口 MUST 可被 Mock 实现替换

---

### Requirement: skillHealthRepo isSuccess 兼容逻辑

`skillHealthRepo` 在聚合健康度指标时 MUST 使用 `isSuccess(outcome, status)` 函数判断调用是否成功。

`isSuccess` 逻辑 MUST 如下：
1. 优先检查 `outcome` 字段：`outcome == "success"` → 返回 true
2. 当 `outcome` 为空时，回退到 `status` 字段：`status == "completed" || status == "success"` → 返回 true
3. 其他情况 → 返回 false

此逻辑 MUST 保持向后兼容，确保未填充 outcome 的历史记录也能正确统计。

#### Scenario: outcome 为 success 时判定为成功

WHEN skill_invocation 记录的 outcome 为 "success"
THEN isSuccess MUST 返回 true，无论 status 值为何

#### Scenario: outcome 为空且 status 为 completed 时判定为成功

WHEN skill_invocation 记录的 outcome 为空
AND status 为 "completed"
THEN isSuccess MUST 返回 true

#### Scenario: outcome 为 failure 时判定为失败

WHEN skill_invocation 记录的 outcome 为 "failure"
THEN isSuccess MUST 返回 false

#### Scenario: outcome 为空且 status 非 completed/success 时判定为失败

WHEN skill_invocation 记录的 outcome 为空
AND status 不是 "completed" 也不是 "success"
THEN isSuccess MUST 返回 false

---

### Requirement: 管理面 Skill 健康度卡片

Skill 详情页 SHALL 新增「健康度」卡片，展示 7d/30d 成功率折线和 P95 耗时趋势。

卡片 MUST 调用 `GetSkillHealth` API 获取数据。

卡片 MUST 展示：
- 7d/30d 成功率折线图
- P95 耗时趋势图
- 关键指标数值（成功率、P95 耗时、总调用次数）

#### Scenario: Skill 详情页展示健康度卡片

WHEN 用户访问 Skill 详情页
AND 该 Skill 有调用记录
THEN 健康度卡片 MUST 展示 7d/30d 成功率折线和 P95 耗时趋势

#### Scenario: Skill 无调用记录时健康度卡片展示空状态

WHEN 用户访问 Skill 详情页
AND 该 Skill 无调用记录
THEN 健康度卡片 MUST 展示空状态提示（如"暂无数据"）

---

### Requirement: biz.SkillInvocation 查询结果包含 Outcome/SelectionReason/TokenUsage

`biz.SkillInvocation` 查询结果类型 MUST 包含 `Outcome`、`SelectionReason`、`TokenUsage` 字段，与 Ent 生成的 SkillInvocation entity 保持一致。

`SearchSkillInvocations` Data 层映射 MUST 将 Ent entity 的 Outcome/SelectionReason/TokenUsage 映射到 Biz 层类型。

此修复 MUST 确保所有通过 `SkillQueryReader.SearchSkillInvocations` 消费数据的代码（如 ExperienceAnalyticsUsecase）能够访问 Outcome 和 SelectionReason 字段。

#### Scenario: SearchSkillInvocations 返回包含 Outcome 的结果

WHEN 调用 SkillQueryReader.SearchSkillInvocations
AND skill_invocation 记录包含 outcome 字段
THEN 返回的 biz.SkillInvocation MUST 包含 Outcome 字段值

#### Scenario: ExperienceAnalyticsUsecase 可使用 Outcome 判断成败

WHEN ExperienceAnalyticsUsecase.AnalyzeSkillHealth 分析 Skill 健康
THEN MUST 使用 Outcome 字段（而非 Status 字段）判断调用成败
AND 当 Outcome 为空时 MUST 回退到 Status 字段

---

### Requirement: ExperienceAnalyticsUsecase 成败判断逻辑与 skillHealthRepo 统一

`ExperienceAnalyticsUsecase.AnalyzeSkillHealth` 的成败判断逻辑 MUST 与 `skillHealthRepo.isSuccess` 保持一致。

当前已知 Bug：ExperienceAnalyticsUsecase 使用 `inv.Status == "failure"` 判断失败，但 skill_invocation 表的 status 列不存在 "failure" 值（"failure" 是 outcome 枚举值），导致 FailureCount 始终为 0。

修复后 MUST 统一使用 `isSuccess(outcome, status)` 兼容逻辑：
1. 优先检查 Outcome 字段
2. Outcome 为空时回退到 Status 字段
3. Status 的失败值为 "error"（非 "failure"）

#### Scenario: ExperienceAnalyticsUsecase 正确统计失败调用

WHEN skill_invocation 记录的 outcome 为 "failure" 或 status 为 "error"
THEN ExperienceAnalyticsUsecase MUST 将该记录计入失败计数

#### Scenario: ExperienceAnalyticsUsecase 不再使用 Status == "failure"

WHEN ExperienceAnalyticsUsecase 分析 Skill 健康
THEN MUST NOT 使用 `inv.Status == "failure"` 判断失败
AND MUST 使用 Outcome 字段优先判断

---

### Requirement: skillInvocationStatsRepo 查询 skill_invocation 表

`skillInvocationStatsRepo.GetSkillInvocationStats` MUST 查询 `skill_invocation` 表而非 `tool_invocation` 表。

当前已知 Bug：该 Repo 使用 ToolInvocation Ent client 查询 tool_invocation 表，使用 ToolKey 作为 skill name，实际分析的是 tool 调用而非 skill 调用。

修复后 MUST 使用 SkillInvocation Ent client 查询 skill_invocation 表，使用 SkillID 关联 Skill 名称。

#### Scenario: skillInvocationStatsRepo 查询 skill_invocation 表

WHEN 调用 skillInvocationStatsRepo.GetSkillInvocationStats
THEN MUST 查询 skill_invocation 表
AND 返回的统计 MUST 基于 skill 调用维度（非 tool 调用维度）

#### Scenario: 统计结果使用 SkillID 关联 Skill 名称

WHEN skillInvocationStatsRepo 返回 SkillInvocationStat
THEN SkillName MUST 通过 SkillID 关联查询获取
AND MUST NOT 使用 ToolKey 作为 skill name
