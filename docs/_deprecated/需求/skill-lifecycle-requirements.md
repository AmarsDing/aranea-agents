# Skill 生命周期智能管理 — 需求提案

> **文档性质**：对 [`skill-notes.md`](./skill-notes.md) 中 Skill-Agent 构想的重构与落地化表述。  
> **关联文档**：[`20-skill.md`](../../需求/20-skill.md)（管理面 UI/API）、[`20-skill-struct-design.md`](../../需求/20-skill-struct-design.md)（架构基线）、[`7-agent-evolution-development.md`](../../需求/7-agent-evolution-development.md)（Agent 进化先例）。  
> **作者视角**：从「已有 CRUD + 运行追踪」出发，补齐「选得准、用得久、坏得快修」三条链路，而非一步到位做全自动进化。

---

## 0. 一句话结论

**Skill 管理的下一阶段，不是再做一个「会写 Skill 的 Agent」，而是先把「调用可观测 → 问题可诊断 → 变更可审计」做扎实；在此之上，用半自动推荐与人工审批，逐步开放进化能力。**

---

## 1. 问题陈述

### 1.1 当前已解决什么

平台已具备 Skill 的完整管理面与基础运行时：

| 能力 | 状态 |
|------|------|
| 上传 / 编辑 / 发布 / 启用 | ✅ |
| 磁盘 watch 同步 | ✅ |
| 冲突检测 + AI 炼化（上传时） | ✅ |
| 运行记录（`skill_invocation`）与聚合指标 | ✅ |
| 意图路由（`skillrouter`）+ 运行时装配 | ✅ |

### 1.2 当前缺口

| 缺口 | 表现 | 业务影响 |
|------|------|----------|
| **选不准** | 路由主要靠 taxonomy + 关键词；无历史成功率加权 | 同类任务有时命中错误 Skill，用户重复澄清 |
| **坏不知** | 失败调用有记录，但无结构化根因与趋势 | 运维/编辑者需人工翻日志才能发现退化 |
| **优不动** | 炼化仅在上传冲突时触发；运行期问题无法反哺 Skill 正文 | Skill 池静态膨胀，低质 Skill 长期占用上下文预算 |
| **版本难管** | 版本历史 / 回滚 API 未实现 | 无法安全试验改进版，不敢动已发布 Skill |

### 1.3 核心诉求（按优先级）

1. **P0 — 可观测**：每次 Skill 调用能回答「为什么选它、结果如何、耗时/成本多少」。
2. **P1 — 可诊断**：对失败/低效调用自动生成「经验报告」，供人决策，而非直接改生产 Skill。
3. **P2 — 可选得准**：在现有路由之上，引入轻量推荐排序（成功率、耗时、语义匹配）。
4. **P3 — 可进化**：在 Sandbox + 人工审批门控下，支持从报告生成 Skill 草案并 A/B 对比。

---

## 2. 产品边界

### 2.1 本期要做（Skill Intelligence 子系统）

- Skill 调用全链路追踪增强（选型理由、评分因子快照）
- 后台分析 Worker：扫描 `skill_invocation`，产出 **Experience Report**
- Skill 健康度面板：成功率趋势、P95 耗时、近期失败 Top N
- 推荐排序 v1：在 `skillrouter` 候选集内重排，不扩大候选范围
- 进化建议队列：类似 Agent `evolution_suggestions`，默认 **仅建议、不自动应用**

### 2.2 明确不做（至少 Phase 3 之前）

| 不做 | 原因 |
|------|------|
| 生产环境双 Skill 并行对决、直接替换用户可见结果 | 成本翻倍、结果不一致、难以解释 |
| 无审批自动发布进化版 Skill | 安全风险与回归不可控 |
| Skill-Agent 替代 Manage-Agent 做任务规划 | 职责重叠；Skill 只管能力包，不管团队编排 |
| 实时（毫秒级）进化 | 与对话热路径解耦，一律异步 |

### 2.3 与 Skill-Agent（skill-notes 原构想）的关系

原 `skill-notes.md` 中的 Skill-Agent 愿景保留，但拆成 **Skill Intelligence Worker**（后台分析）+ **Skill Curator Agent**（按需 invoked，生成草案），而非常驻 goroutine 自主改库：

```
[对话结束] → [Invocation 入库] → [Intelligence Worker 异步分析]
                                        ↓
                              [Experience Report 入库]
                                        ↓
                    [Curator Agent 生成草案] ← 人工审批 / Sandbox 验证
                                        ↓
                              [新版本 draft → publish]
```

---

## 3. 用户角色与场景

| 角色 | 场景 | 期望 outcome |
|------|------|--------------|
| **编辑者** | 上传后 Skill 长期无人用或失败率高 | 收到「负熵报告」与具体修改建议，一键生成草稿 |
| **管理员** | 技能池膨胀、疑似重复 | 看到去重候选组，合并或归档 |
| **Agent 运行时** | 用户问「帮我写发布说明」 | 在 3 个候选 Skill 中选出历史成功率最高者 |
| **平台运维** | 某 Skill 昨晚开始超时 | 告警 + 报告定位到工具/API 变更 |

---

## 4. 分阶段交付

### Phase 0 — 基线（已完成，仅对齐）

- 沿用 `20-skill.md` 管理面 + `ListSkillRuns` + 聚合字段（`invoke_count` / `success_count` / `avg_duration_ms`）

### Phase 1 — 可观测增强（建议下一迭代）

**目标**：让每次调用「可复盘」，为后续分析提供结构化输入。

| ID | 需求 | 验收标准 |
|----|------|----------|
| SI-01 | `skill_invocation` 增加 `selection_reason` JSON | 记录路由路径、候选 slug 列表、最终选中 slug |
| SI-02 | 增加 `outcome` 枚举：`success` / `failure` / `partial` / `cancelled` | 与 session 最终状态对齐，非仅 tool 返回码 |
| SI-03 | 记录 `token_usage` / `duration_ms` / `error_code` | 运行记录页可筛选失败原因 |
| SI-04 | 管理面「Skill 详情 → 健康度」卡片 | 展示 7d/30d 成功率折线、P95 耗时 |

**非功能**：写入异步、不阻塞 Runner；热路径仅多写几个字段。

### Phase 2 — 经验报告与负熵（诊断层）

**目标**：把 `skill-notes.md` §3.1–3.2 落成结构化产物，供人读、供 Agent 读。

| ID | 需求 | 验收标准 |
|----|------|----------|
| SI-10 | 新增 `ExperienceReport` 实体 | 字段见 §6.1；与 `dialogue_id` + `skill_id` 关联 |
| SI-11 | `SkillIntelligenceWorker` 定时任务 | 可配置间隔（默认 15min）；`SKILL_INTELLIGENCE_DISABLED=1` 可关；参考 `evolution_scanner` |
| SI-12 | 扫描范围：近 N 小时新增且含 Skill 调用的 session | 可配置 `lookback_hours` |
| SI-13 | 报告内容：成败判定、流程摘要、失败标签、优化建议、综合评分 0–100 | 失败标签枚举见 §6.2 |
| SI-14 | 管理面「负熵报告」列表 | 按 Skill / 评分 / 时间筛选；可跳转对应对话 |
| SI-15 | 评分模型 v1（可配置权重） | 默认：成功率 40%、耗时 25%、token 20%、用户反馈 15%（无反馈时均分） |

**分析执行者**：

- 结构化字段（成败、耗时、标签）→ 规则 + SQL 聚合
- 自然语言摘要与建议 → 调用已配置 LLM（与导入炼化共用 provider 配置）
- **禁止**在 `internal/biz` import `pkg/trpc-agent-go`；Curator 调用走 `internal/service`

### Phase 3 — 推荐与去重（选型层）

**目标**：在**不扩大候选集**的前提下，提高命中率；降低技能池冗余。

| ID | 需求 | 验收标准 |
|----|------|----------|
| SI-20 | 推荐排序 v1 | 对 `skillrouter` 输出的候选 slug 列表重排 |
| SI-21 | 排序因子 | 语义相似度（embedding，可选）× 历史成功率 × 耗时倒数 × 用户偏好（Memory 端口，可选） |
| SI-22 | 因子快照写入 `selection_reason` | 便于事后解释「为何选 B 而非 A」 |
| SI-23 | 去重候选组 | 名称不同但 description+正文相似度 ≥ 0.2 的 Skill 归组；管理面展示「建议合并」 |
| SI-24 | 合并操作 | 保留主 Skill，副 Skill `archived`；合并后 invoke 统计归并（可选） |

**与 skill-notes §1.2–1.3 的差异**：去重默认**人工确认**；推荐默认**单版本执行**，不做运行时对决。

### Phase 4 — 半自动进化（变更层）

**目标**：闭环 skill-notes §1.4–3.4，但加硬门控。

| ID | 需求 | 验收标准 |
|----|------|----------|
| SI-30 | 触发条件 | 同一 Skill 30d 内失败 ≥ 3 次且评分 < 60；或成功但 P95 耗时较基线恶化 ≥ 20% |
| SI-31 | `SkillEvolutionSuggestion` 队列 | 状态：`pending` / `approved` / `rejected` / `applied`；同 Skill 同 type 去重 |
| SI-32 | Curator Agent 生成草案 | 输入：原 Skill markdown + Experience Report；输出：新 `draft` 版本 + `evolution_reason` |
| SI-33 | Sandbox Runner 验证 | 用历史失败 case 重放 ≥ 1 次；记录 sandbox 成败 |
| SI-34 | 人工审批 UI | 对比 diff + sandbox 结果；批准后 `PublishSkill` |
| SI-35 | Shadow 模式（可选） | 新版本仅记录「若使用则会如何」，不改变生产路由；积累 50 次 shadow 后再提审批 |
| SI-36 | 版本血缘 | `parent_skill_id` / `parent_version_id` / `source_report_id` |

**明确不做（Phase 4）**：生产环境 Skill A vs A' 并行对决并择优返回（skill-notes §3.4.1）；改为 Shadow + 人工审批。

### Phase 5 — 自动优胜劣汰（远期）

- 长期统计显著劣化版本自动 `deprecated`
- 需：Shadow 数据充分 + 管理员开启 `skill_auto_deprecate_enabled`
- 仍保留回滚能力

---

## 5. 功能需求详述

### 5.1 Skill Intelligence Worker

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `skill_intelligence.enabled` | `false` | 总开关 |
| `skill_intelligence.scan_interval` | `15m` | 扫描间隔 |
| `skill_intelligence.lookback_hours` | `24` | 每次扫描回看窗口 |
| `skill_intelligence.min_invocations_for_score` | `5` | 低于此次数仅记录不评分 |
| `skill_intelligence.score_weights` | 见 SI-15 | JSON |

行为：

1. 从 `skill_invocation` + session 消息拉取待分析样本
2. 规则层提取结构化字段
3. LLM 层生成摘要与建议（可降级：仅结构化）
4. 写入 `experience_report`；若触发阈值则写入 `skill_evolution_suggestion`
5. 发布 `skill.intelligence.report_created` 事件（供 Monitor 订阅）

### 5.2 推荐排序（Phase 3）

插入点：`internal/tools/skillrouter` 产出候选后、`skillruntime` 装配前。

```text
candidates := skillrouter.Detect(...)
ranked     := skillrecommend.Rank(ctx, candidates, intent, userCtx)
selected   := ranked[:policy.MaxSkillsInToolset]
```

排序公式（v1，可调）：

```text
score = w1 * semantic_sim + w2 * success_rate_30d + w3 * (1 / norm_duration) + w4 * user_pref
```

- 缺数据时该项取中性值 0.5，避免新 Skill 被永久打压
- 新 Skill（< 7d）可配置「探索加成」+0.1，防止马太效应

### 5.3 经验报告 → 进化建议

| 步骤 | 执行者 | 输出 |
|------|--------|------|
| 聚合失败模式 | Worker 规则 | `failure_tags[]` |
| 生成优化建议 | LLM | `optimization_advice` |
| 判定是否触发进化 | Worker 规则 | `SkillEvolutionSuggestion` |
| 生成 Skill 草案 | Curator Agent（service 层 invoke） | 新 draft + diff |
| Sandbox 重放 | `internal/service` + 隔离 Runner | `sandbox_result` |
| 人工审批 | 管理面 | publish / reject |

### 5.4 反馈环路

监听事件（与 skill-notes §1.5 对齐，落地化）：

| 事件 | 消费者 | 动作 |
|------|--------|------|
| `skill.invocation.completed` | Intelligence Worker | 纳入下轮扫描 |
| `skill.intelligence.report_created` | Monitor Audit | 可观测 |
| `skill.evolution.suggestion_created` | 管理面通知 | 待办 |
| `skill.published` | watch + trpc cache | 热加载 |

---

## 6. 数据模型

### 6.1 ExperienceReport

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | UUID |
| `tenant_id` | string | 租户 |
| `session_id` | string | 来源会话 |
| `invocation_id` | string | 关联 `skill_invocation` |
| `skill_id` | string | 被分析 Skill |
| `is_success` | bool | 整体成败 |
| `score` | int | 0–100 |
| `failure_tags` | []string | 见 §6.2 |
| `flow_summary` | text | 调用链摘要 |
| `optimization_advice` | text | 可操作建议 |
| `selection_snapshot` | json | 选型因子快照 |
| `generated_suggestion_id` | string? | 若已触发进化建议 |
| `created_at` | datetime | |

### 6.2 失败标签枚举（v1）

`param_mismatch` · `wrong_tool_choice` · `tool_timeout` · `tool_api_error` · `context_overflow` · `instruction_ambiguity` · `user_cancelled` · `unknown`

### 6.3 SkillEvolutionSuggestion

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | |
| `skill_id` | string | 目标 Skill |
| `type` | enum | `fix_failure` / `boost_efficiency` / `merge_duplicate` |
| `status` | enum | `pending` / `approved` / `rejected` / `applied` |
| `source_report_ids` | []string | 依据报告 |
| `draft_skill_version_id` | string? | 生成的草案版本 |
| `sandbox_passed` | bool? | |
| `created_at` / `resolved_at` | datetime | |

### 6.4 Skill 元数据扩展（Phase 4）

在现有 `skill` / `skill_version` 上扩展：

| 字段 | 类型 | 说明 |
|------|------|------|
| `parent_version_id` | string? | 血缘 |
| `evolution_reason` | enum? | `fix_failure` / `boost_efficiency` |
| `lifecycle_status` | enum | `active` / `shadow` / `deprecated`（与 draft/published 正交） |

**不单独建「Skill 对决表」**；Shadow 模式用 `skill_invocation.shadow=true` + 关联建议 ID 即可。

---

## 7. 架构与集成

### 7.1 模块落点（遵循 AGENT_RUNTIME_BOUNDARY）

```text
internal/
├── biz/
│   ├── skill_intelligence.go      # 端口：ReportRepo, SuggestionRepo, 评分/触发规则
│   └── skill_recommend.go         # 纯函数排序，无框架依赖
├── data/
│   └── skill_intelligence.go      # Ent 实现
├── cronrunner/jobs/
│   └── skill_intelligence_worker.go
├── service/
│   └── skill_curator.go           # Curator Agent 装配与 invoke（可复用 evolution 模式）
└── tools/skillrecommend/
    └── rank.go                    # 运行时排序，由 skillruntime 调用
```

- **Worker / 规则 / 聚合** → `biz` + `data` + `cronrunner`
- **LLM 调用与 Agent Run** → `service`
- **运行时排序** → `internal/tools`，由 `internal/agent/trpc_build.go` 间接使用

### 7.2 与 Manage-Agent / Memory-Agent 的接口

| 依赖 | 方式 | 说明 |
|------|------|------|
| Memory 用户偏好 | `biz` 端口 `UserPreferenceReader` | Phase 3 可选；无实现时权重跳过 |
| Tools 可用性 | 现有 tool health / invocation 统计 | 失败标签 `tool_api_error` 时引用 |
| Manage-Agent 规划 | **不耦合** | Manage 可在规划阶段查询 Skill 推荐 API；Skill Intelligence 不回调 Manage |

### 7.3 API 规划（Proto 增量）

| RPC | 路径 | Phase |
|-----|------|-------|
| `ListExperienceReports` | `GET /v1/skills/intelligence/reports` | 2 |
| `GetExperienceReport` | `GET /v1/skills/intelligence/reports/{id}` | 2 |
| `ListSkillEvolutionSuggestions` | `GET /v1/skills/evolution-suggestions` | 4 |
| `ApproveSkillEvolutionSuggestion` | `POST /v1/skills/evolution-suggestions/{id}/approve` | 4 |
| `RejectSkillEvolutionSuggestion` | `POST /v1/skills/evolution-suggestions/{id}/reject` | 4 |
| `GetSkillHealth` | `GET /v1/skills/{id}/health` | 1 |

版本历史 / 回滚（`20-skill.design.md` 已规划）应在 **Phase 1 末或 Phase 2 初** 优先实现，为 Phase 4 进化提供基础设施。

---

## 8. 验收标准（跨阶段）

| 阶段 | 核心验收 |
|------|----------|
| Phase 1 | 任意一次 Skill 调用可在运行记录中看到选型理由与 outcome；健康度 API 返回 7d 成功率 |
| Phase 2 | Worker 关闭时不影响对话；开启后对模拟失败调用 24h 内产出 Report；报告可在 UI 列表查看 |
| Phase 3 | 同一 intent 在 A/B 两个 Skill 成功率差 ≥ 30% 时，推荐排序 80% 以上选高成功率者（基准测试集） |
| Phase 4 | 从 Report → 草案 → Sandbox → 人工发布全链路可走通；未审批草案 never 进入生产路由 |

---

## 9. 风险与约束

| 风险 | 缓解 |
|------|------|
| LLM 分析成本 | 采样 + 仅分析失败/低分调用；可配置每日上限 |
| 进化引入回归 | Sandbox + 人工审批 + 版本回滚 |
| 推荐加剧「富者愈富」 | 新 Skill 探索加成 + 最低 invocation 阈值 |
| 与 Agent Evolution 概念混淆 | Skill 进化管**能力包**；Agent Evolution 管**Agent 配置**；共用 Worker 模式但分表分队列 |
| 磁盘 watch 与 DB 草案冲突 | 进化产出仅写 DB + 文件；watch 以 DB published 为准 |

---

## 10. 开放问题（需产品确认）

1. **用户反馈权重**：当前 session 是否有点赞/踩？若无，Phase 2 评分模型中该项是否改为「重试次数」代理？
2. **Shadow 模式是否默认开启**：建议默认 off，仅高价值 Skill 手动开启。
3. **跨 Agent 共享 Skill 统计**：成功率是否按 `(skill_id, agent_id)` 分桶，还是全局聚合？
4. **Curator 与导入炼化是否合并为同一 Agent**：建议共用 LLM 配置，分 prompt template。
5. **Phase 1 是否与版本历史 API 捆绑发布**：建议捆绑，否则进化无回滚保障。

---

## 11. 与原 skill-notes 对照

| skill-notes 条目 | 本提案处理 |
|------------------|------------|
| Skill-Agent 常驻 goroutine | 拆为 Worker + 按需 Curator |
| 竞争上岗 / 并行对决 | 改为 Shadow + 人工审批；远期 Phase 5 |
| 综合评分模型 | 保留，Phase 2 落地 |
| 经验报告 / 数据模型 | 保留并细化字段 |
| 去重合并 | 保留，Phase 3，人工确认 |
| 推荐因子（语义/成功率/耗时/偏好/工具） | 保留，分阶段接入 |
| Sandbox 验证 | 保留，Phase 4 硬门槛 |

---

## 12. 建议的下一步

1. **产品确认** §10 开放问题  
2. **技术预研**：Phase 1 字段扩展对 `skill_invocation` schema 的 migration  
3. **复用**：对齐 `evolution_scanner` 的 Worker 骨架与 `evolution_suggestions` 队列 UX  
4. **文档归位**：Phase 1 确认后，将 §5–§7 迁入 `docs/需求/20-skill-intelligence.md`（正式需求），本文件保留为 deprecated 提案存档
