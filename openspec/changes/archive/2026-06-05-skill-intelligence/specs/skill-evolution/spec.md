# Skill Evolution — Phase 4 规格说明

> 阶段：Phase 4 — 半自动进化
> 实现状态：未实现
> 前置依赖：Phase 1 可观测增强 + Phase 2 经验报告与诊断 + Phase 3 推荐排序

---

## ADDED Requirements

### Requirement: SkillEvolutionSuggestion 领域模型

系统 SHALL 定义 `SkillEvolutionSuggestion` 领域模型，表示基于 Experience Report 触发的 Skill 进化建议。

`SkillEvolutionSuggestion` MUST 包含以下字段：
- `ID`：建议唯一标识
- `SkillID`：目标 Skill 标识
- `Type`：进化类型，枚举值：`fix_failure` / `boost_efficiency` / `merge_duplicate`
- `Status`：状态，枚举值：`pending` / `approved` / `rejected` / `applied`
- `SourceReportIDs`：依据的 ExperienceReport ID 列表
- `DraftSkillVersionID`：生成的草案版本 ID（可选）
- `SandboxPassed`：Sandbox 验证结果（可选 bool）
- `CreatedAt`：创建时间
- `ResolvedAt`：处理时间（可选）

`SkillEvolutionSuggestion` 与 `SkillProposal`（skill-evolution-auto-creator 产出）定位不同：
- `SkillEvolutionSuggestion`：基于 Experience Report 触发，包含草案版本 ID + Sandbox 验证，面向已有 Skill 的优化
- `SkillProposal`：基于重复工具调用模式触发，含 SkillMD 草案正文，面向从零创建新 Skill

两者 MUST 使用不同的审批队列和数据表，SHALL NOT 合并。

#### Scenario: 失败触发 fix_failure 类型建议

WHEN 同一 Skill 30d 内失败 ≥ 3 次且评分 < 60
THEN MUST 创建 Type 为 "fix_failure" 的 SkillEvolutionSuggestion

#### Scenario: 性能退化触发 boost_efficiency 类型建议

WHEN 同一 Skill 成功但 P95 耗时较基线恶化 ≥ 20%
THEN MUST 创建 Type 为 "boost_efficiency" 的 SkillEvolutionSuggestion

#### Scenario: 去重组触发 merge_duplicate 类型建议

WHEN Phase 3 去重检测发现相似 Skill 组
THEN MUST 创建 Type 为 "merge_duplicate" 的 SkillEvolutionSuggestion

#### Scenario: SkillEvolutionSuggestion 与 SkillProposal 使用不同队列

WHEN SkillEvolutionSuggestion 被创建
THEN MUST 写入 skill_evolution_suggestion 表
AND MUST NOT 写入 skill_proposals 表

---

### Requirement: 触发条件

SkillEvolutionSuggestion MUST 在以下条件满足时由 Worker 自动触发：

条件一（失败触发）：
- 同一 Skill 30 天内失败调用 ≥ 3 次
- 且该 Skill 的 ExperienceReport 评分 < 60

条件二（性能退化触发）：
- 同一 Skill 成功调用但 P95 耗时较基线（30d 前 P95）恶化 ≥ 20%

触发判定 MUST 在 Worker 的离线扫描中执行，SHALL NOT 在对话热路径中执行。

同一 Skill 同一 Type 的 pending 状态建议 MUST 去重，不重复创建。

#### Scenario: 满足失败触发条件

WHEN Skill A 在 30d 内有 3 次失败调用
AND Skill A 的 ExperienceReport 评分为 55（< 60）
THEN Worker MUST 创建 Type 为 "fix_failure" 的 SkillEvolutionSuggestion

#### Scenario: 失败次数不足不触发

WHEN Skill A 在 30d 内有 2 次失败调用
THEN MUST NOT 创建 fix_failure 类型的 SkillEvolutionSuggestion

#### Scenario: 评分不低不触发失败建议

WHEN Skill A 在 30d 内有 5 次失败调用
AND Skill A 的 ExperienceReport 评分为 75（≥ 60）
THEN MUST NOT 创建 fix_failure 类型的 SkillEvolutionSuggestion

#### Scenario: 满足性能退化触发条件

WHEN Skill B 的当前 P95 耗时较 30d 前基线恶化 25%（≥ 20%）
THEN Worker MUST 创建 Type 为 "boost_efficiency" 的 SkillEvolutionSuggestion

#### Scenario: 性能退化不足不触发

WHEN Skill B 的当前 P95 耗时较基线恶化 15%（< 20%）
THEN MUST NOT 创建 boost_efficiency 类型的 SkillEvolutionSuggestion

#### Scenario: 同 Skill 同 Type 去重

WHEN Skill A 已存在 Type 为 "fix_failure" 且 Status 为 "pending" 的 SkillEvolutionSuggestion
AND 再次满足失败触发条件
THEN MUST NOT 创建重复的 SkillEvolutionSuggestion

---

### Requirement: 进化流程

SkillEvolutionSuggestion 的进化流程 MUST 按以下步骤执行：

| 步骤 | 执行者 | 输出 |
|------|--------|------|
| 聚合失败模式 | Worker 规则层 | `failure_tags[]` |
| 生成优化建议 | LLM | `optimization_advice` |
| 判定是否触发进化 | Worker 规则层 | `SkillEvolutionSuggestion` |
| 生成 Skill 草案 | Curator Agent（service 层 invoke） | 新 draft + diff |
| Sandbox 重放 | `internal/service` + 隔离 Runner | `sandbox_result` |
| 人工审批 | 管理面 | publish / reject |

步骤 1–3 MUST 在 Worker 中自动执行。

步骤 4（Curator Agent 生成草案）MUST 在人工触发或自动触发后执行，Curator 调用 MUST 走 `internal/service` 层，Biz 层 MUST NOT 直接 import `pkg/trpc-agent-go`。

步骤 5（Sandbox 验证）MUST 在草案生成后自动执行，使用隔离 Runner 重放历史调用场景。

步骤 6（人工审批）MUST 为最终门控，无审批 MUST NOT 发布到生产环境。

#### Scenario: Worker 自动执行步骤 1–3

WHEN Worker 扫描发现满足触发条件的 Skill
THEN MUST 自动聚合失败模式、生成优化建议、创建 SkillEvolutionSuggestion
AND SkillEvolutionSuggestion 的 Status MUST 为 "pending"

#### Scenario: Curator Agent 生成草案

WHEN SkillEvolutionSuggestion 被创建或人工触发草案生成
THEN Curator Agent MUST 基于 optimization_advice 和原始 Skill 正文生成新 draft
AND MUST 生成 diff 供审批参考
AND DraftSkillVersionID MUST 被设置

#### Scenario: Sandbox 自动验证草案

WHEN 草案版本被生成
THEN MUST 自动在隔离 Runner 中重放历史调用场景
AND SandboxPassed MUST 被设置为验证结果

#### Scenario: 人工审批为最终门控

WHEN SkillEvolutionSuggestion 的 Status 为 "pending"
AND Sandbox 验证通过
THEN 管理员 MUST 手动审批后才能发布到生产环境
AND MUST NOT 自动发布

---

### Requirement: Ent Schema skill_evolution_suggestion

系统 SHALL 定义 `skill_evolution_suggestion` Ent Schema，持久化 SkillEvolutionSuggestion。

Schema MUST 包含以下字段：
- `id`：String，主键
- `skill_id`：String，目标 Skill
- `type`：String，进化类型（fix_failure / boost_efficiency / merge_duplicate）
- `status`：String，状态（pending / approved / rejected / applied）
- `source_report_ids`：JSON，依据报告 ID 列表
- `draft_skill_version_id`：String（可选），生成的草案版本
- `sandbox_passed`：Bool（可选），Sandbox 验证结果
- `created_at`：Time，创建时间
- `resolved_at`：Time（可选），处理时间

Schema MUST 定义以下索引：
- `(skill_id, type, status)`：同 Skill 同 type 去重查询

此 Schema MUST 使用 Ent ORM 实现，SHALL NOT 使用原始 SQL DDL（与 `skill_proposals` 表的实现方式不同）。

#### Scenario: Ent Schema 生成正确的数据库表

WHEN 运行 Ent 代码生成
THEN MUST 生成 skill_evolution_suggestion 表
AND 表结构 MUST 包含上述所有字段和索引

#### Scenario: 同 Skill 同 type 去重

WHEN 尝试创建 (skill_id=X, type=fix_failure, status=pending) 的记录
AND 已存在相同 skill_id + type + pending 状态的记录
THEN MUST NOT 创建重复记录

---

### Requirement: Skill 元数据扩展

系统 MUST 在现有 `skill` / `skill_version` 上扩展以下字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `parent_version_id` | String（可选） | 血缘关系，指向进化前的版本 |
| `evolution_reason` | String（可选） | 进化原因：fix_failure / boost_efficiency |
| `lifecycle_status` | String | 生命周期状态：active / shadow / deprecated |

`lifecycle_status` 与现有的 `draft` / `published` 版本状态正交：
- `draft` / `published`：版本发布状态
- `active` / `shadow` / `deprecated`：运行时生命周期状态

`shadow` 状态的 Skill MUST 仅在 Sandbox 或 A/B 测试场景中被路由，SHALL NOT 出现在正常用户对话中。

`deprecated` 状态的 Skill MUST NOT 被路由，但 MUST 保留历史记录可查询。

#### Scenario: 进化版本记录血缘关系

WHEN 通过 SkillEvolutionSuggestion 生成新 Skill 版本
THEN 新版本的 parent_version_id MUST 指向进化前的版本
AND evolution_reason MUST 记录进化原因

#### Scenario: shadow 状态 Skill 不出现在正常路由

WHEN Skill 的 lifecycle_status 为 "shadow"
AND 用户发起正常对话
THEN skillruntime MUST NOT 将该 Skill 纳入候选集

#### Scenario: shadow 状态 Skill 可在 Sandbox 中路由

WHEN Skill 的 lifecycle_status 为 "shadow"
AND Sandbox 验证场景中请求路由
THEN skillruntime MUST 将该 Skill 纳入候选集

#### Scenario: deprecated 状态 Skill 不可路由

WHEN Skill 的 lifecycle_status 为 "deprecated"
THEN skillruntime MUST NOT 将该 Skill 纳入任何候选集
AND 该 Skill 的历史调用记录 MUST 仍可查询

#### Scenario: lifecycle_status 与版本状态正交

WHEN Skill 版本为 "published" 且 lifecycle_status 为 "shadow"
THEN 版本已发布但运行时不路由
AND 两者 MUST 独立控制，互不影响

---

### Requirement: ListSkillEvolutionSuggestions API

系统 SHALL 提供 `ListSkillEvolutionSuggestions` API，分页查询 SkillEvolutionSuggestion 列表。

API 路径 MUST 为 `GET /v1/skills/evolution-suggestions`。

请求参数 MUST 支持：
- `skill_id`（可选）：按 Skill 过滤
- `type`（可选）：按进化类型过滤
- `status`（可选）：按状态过滤
- `page_size` / `page_token`：分页参数

响应 MUST 包含 SkillEvolutionSuggestion 列表和下一页 token。

#### Scenario: 按 Skill ID 过滤建议

WHEN 请求 GET /v1/skills/evolution-suggestions?skill_id=xxx
THEN MUST 仅返回该 Skill 的 SkillEvolutionSuggestion

#### Scenario: 按状态过滤建议

WHEN 请求 GET /v1/skills/evolution-suggestions?status=pending
THEN MUST 仅返回 pending 状态的 SkillEvolutionSuggestion

#### Scenario: 分页查询建议

WHEN 请求 GET /v1/skills/evolution-suggestions?page_size=10
THEN MUST 返回最多 10 条记录
AND 若有更多记录，响应 MUST 包含 next_page_token

---

### Requirement: ApproveSkillEvolutionSuggestion API

系统 SHALL 提供 `ApproveSkillEvolutionSuggestion` API，审批通过 SkillEvolutionSuggestion。

API 路径 MUST 为 `POST /v1/skills/evolution-suggestions/{id}/approve`。

审批操作 MUST 将 Status 从 `pending` 变更为 `approved`。

仅 `pending` 状态的建议可被审批，非 pending 状态 MUST 返回错误。

审批通过后，若 DraftSkillVersionID 存在，MUST 将草案版本的 lifecycle_status 从 `shadow` 变更为 `active`，并将原版本的 lifecycle_status 变更为 `deprecated`。

审批 MUST 记录 ResolvedAt 时间。

#### Scenario: 审批 pending 状态的建议

WHEN 请求 POST /v1/skills/evolution-suggestions/{id}/approve
AND 建议状态为 pending
THEN Status MUST 变更为 "approved"
AND ResolvedAt MUST 被设置

#### Scenario: 审批非 pending 状态的建议返回错误

WHEN 请求 POST /v1/skills/evolution-suggestions/{id}/approve
AND 建议状态不是 pending
THEN MUST 返回 INVALID_ARGUMENT 或 FAILED_PRECONDITION 错误

#### Scenario: 审批后版本生命周期切换

WHEN 审批通过且 DraftSkillVersionID 存在
THEN 草案版本的 lifecycle_status MUST 变更为 "active"
AND 原版本的 lifecycle_status MUST 变更为 "deprecated"

---

### Requirement: RejectSkillEvolutionSuggestion API

系统 SHALL 提供 `RejectSkillEvolutionSuggestion` API，拒绝 SkillEvolutionSuggestion。

API 路径 MUST 为 `POST /v1/skills/evolution-suggestions/{id}/reject`。

拒绝操作 MUST 将 Status 从 `pending` 变更为 `rejected`。

仅 `pending` 状态的建议可被拒绝，非 pending 状态 MUST 返回错误。

拒绝后，若 DraftSkillVersionID 存在，MUST 将草案版本的 lifecycle_status 保持为 `shadow` 或设置为 `deprecated`。

拒绝 MUST 记录 ResolvedAt 时间。

#### Scenario: 拒绝 pending 状态的建议

WHEN 请求 POST /v1/skills/evolution-suggestions/{id}/reject
AND 建议状态为 pending
THEN Status MUST 变更为 "rejected"
AND ResolvedAt MUST 被设置

#### Scenario: 拒绝非 pending 状态的建议返回错误

WHEN 请求 POST /v1/skills/evolution-suggestions/{id}/reject
AND 建议状态不是 pending
THEN MUST 返回 INVALID_ARGUMENT 或 FAILED_PRECONDITION 错误

#### Scenario: 拒绝后草案版本处理

WHEN 拒绝通过且 DraftSkillVersionID 存在
THEN 草案版本的 lifecycle_status MUST 被设置为 "deprecated"
AND 原 Skill 的 lifecycle_status MUST 保持 "active"

---

### Requirement: skill.evolution.suggestion_created 事件

Worker 在创建 SkillEvolutionSuggestion 后 MUST 发布 `skill.evolution.suggestion_created` 事件。

事件负载 MUST 包含：
- `suggestion_id`：SkillEvolutionSuggestion ID
- `skill_id`：Skill 标识
- `type`：进化类型
- `source_report_ids`：依据报告 ID 列表

事件消费者（如管理面通知）可用于待办提醒。

#### Scenario: 建议创建后发布事件

WHEN Worker 成功创建 SkillEvolutionSuggestion
THEN MUST 发布 skill.evolution.suggestion_created 事件
AND 事件负载 MUST 包含 suggestion_id、skill_id、type、source_report_ids

---

### Requirement: 与 SkillEvolutionUsecase 的区分

`SkillEvolutionSuggestion`（Phase 4）与 `SkillEvolutionUsecase`（skill-evolution-auto-creator）MUST 保持独立，SHALL NOT 合并。

| 维度 | SkillEvolutionUsecase（已实现） | SkillEvolutionSuggestion（Phase 4） |
|------|--------------------------------|-------------------------------------|
| 触发 | 重复工具调用模式 | Skill 调用失败/低效 |
| 产出 | `SkillProposal`（含 SkillMD 草案正文） | `SkillEvolutionSuggestion`（含草案版本 ID + Sandbox 验证） |
| 审批 | SkillProposal 队列（pending → approved → registered） | SkillEvolutionSuggestion 队列（pending → approved → applied） |
| 数据存储 | `skill_proposals` 表（原始 SQL） | `skill_evolution_suggestion` 表（Ent Schema） |
| AI 生成 | SkillAutoCreator.GenerateSKILLMD 端口 | Curator Agent（service 层 invoke） |
| 注册 | SkillRegistrationPort（RegisterSkill/SkillExists） | 版本发布 + lifecycle_status 切换 |

#### Scenario: Phase 4 不修改 SkillEvolutionUsecase

WHEN Phase 4 实施完成
THEN SkillEvolutionUsecase MUST 保持原有行为不变
AND skill_proposals 表 MUST NOT 被修改

#### Scenario: 两条审批队列独立运作

WHEN 同时存在 SkillProposal 和 SkillEvolutionSuggestion
THEN 两者 MUST 使用独立的审批流程
AND 审批一个 MUST NOT 影响另一个的状态

---

### Requirement: 磁盘 watch 与 DB 草案冲突处理

进化产出的 Skill 草案 MUST 仅写入 DB + 文件，SHALL NOT 绕过现有 watch 机制。

磁盘 watch MUST 以 DB published 状态为准，确保 watch 同步不会覆盖进化产出的草案。

#### Scenario: 进化草案写入 DB

WHEN Curator Agent 生成 Skill 草案
THEN 草案 MUST 写入 DB，版本状态为 draft
AND lifecycle_status 为 shadow

#### Scenario: watch 不覆盖 DB 草案

WHEN 磁盘 watch 检测到文件变更
AND DB 中对应 Skill 的版本为 draft（进化草案）
THEN watch MUST NOT 用文件内容覆盖 DB 草案
AND MUST 以 DB published 状态为权威数据源

---

### Requirement: 版本回滚

系统 MUST 支持进化版本的回滚操作。

当进化版本引入回归时，管理员 MUST 能够将 lifecycle_status 从 `active` 回滚为 `deprecated`，同时将原版本的 lifecycle_status 从 `deprecated` 恢复为 `active`。

回滚操作 MUST 通过管理面 UI 执行。

回滚后，关联的 SkillEvolutionSuggestion 的 Status MUST 变更为 `rejected`（如果之前为 `applied`）。

#### Scenario: 回滚进化版本

WHEN 管理员对已 applied 的进化版本执行回滚
THEN 进化版本的 lifecycle_status MUST 变更为 "deprecated"
AND 原版本的 lifecycle_status MUST 恢复为 "active"

#### Scenario: 回滚后建议状态更新

WHEN 进化版本被回滚
AND 关联的 SkillEvolutionSuggestion 状态为 "applied"
THEN SkillEvolutionSuggestion 的 Status MUST 变更为 "rejected"

---

### Requirement: 无审批自动发布禁止

系统 MUST NOT 支持无审批自动发布进化版 Skill。

所有进化版 Skill MUST 经过人工审批门控后才能发布到生产环境。

Sandbox 验证通过 MUST NOT 自动触发发布，仅作为审批参考信息。

#### Scenario: Sandbox 通过不自动发布

WHEN SkillEvolutionSuggestion 的 SandboxPassed 为 true
AND 未经过人工审批
THEN 进化版本 MUST NOT 被发布到生产环境
AND Status MUST 保持 "pending"

#### Scenario: 必须人工审批才能发布

WHEN 管理员调用 ApproveSkillEvolutionSuggestion
THEN 进化版本才可发布到生产环境
AND 无其他途径可绕过审批门控
