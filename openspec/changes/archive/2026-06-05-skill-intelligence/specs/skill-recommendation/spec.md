# Skill Recommendation — Phase 3 规格说明

> 阶段：Phase 3 — 推荐排序
> 实现状态：未实现
> 前置依赖：Phase 1 可观测增强 + Phase 2 经验报告与诊断

---

## ADDED Requirements

### Requirement: skillrecommend.Rank 排序函数

系统 SHALL 提供 `skillrecommend.Rank` 纯函数，在 `skillruntime.ResolveSkillSlugsDetailed` 评分之后、返回结果之前对候选 Skill 进行重排序。

`Rank` MUST 为纯函数，无框架依赖，不访问数据库或外部服务。所需的历史数据（成功率、耗时等）MUST 通过参数传入。

`Rank` MUST 接受以下输入：
- `candidates`：候选 Skill 列表（含 Layer A/B 评分）
- `intent`：用户意图描述
- `userCtx`：用户上下文（含偏好数据，可选）

`Rank` MUST 返回按综合评分降序排列的候选列表。

#### Scenario: Rank 函数为纯函数

WHEN 检查 skillrecommend 包的 import 列表
THEN MUST NOT 包含数据库驱动、HTTP 客户端或 trpc-agent-go 依赖

#### Scenario: Rank 接受候选列表和上下文

WHEN 调用 skillrecommend.Rank(ctx, candidates, intent, userCtx)
THEN MUST 返回按综合评分降序排列的候选列表

---

### Requirement: 排序公式 v1

`skillrecommend.Rank` MUST 使用以下排序公式计算综合评分：

```
score = w1 * semantic_sim + w2 * success_rate_30d + w3 * (1 / norm_duration) + w4 * user_pref
```

其中：
- `semantic_sim`：语义相似度（来自 Layer B 的 taxonomy/embedding 评分，归一化到 0–1）
- `success_rate_30d`：30 天成功率（来自 skillHealthRepo 或 ExperienceReport 聚合）
- `norm_duration`：归一化耗时（0–1，越低越好）
- `user_pref`：用户偏好评分（0–1，来自 Memory 用户偏好，可选）
- `w1`、`w2`、`w3`、`w4`：可配置权重

缺数据时该因子 MUST 取中性值 0.5，避免新 Skill 被永久打压。

#### Scenario: 所有因子数据完整时排序

WHEN 候选 Skill 有完整的语义评分、30d 成功率、耗时和用户偏好数据
THEN 排序 MUST 按公式计算综合评分
AND 综合评分 MUST 为各因子加权和

#### Scenario: 缺少历史成功率数据时排序

WHEN 候选 Skill 无 30d 调用记录（新 Skill）
THEN success_rate_30d 因子 MUST 取中性值 0.5
AND 排序 MUST NOT 因无历史数据而将该 Skill 排到末尾

#### Scenario: 缺少用户偏好数据时排序

WHEN 用户无偏好数据
THEN user_pref 因子 MUST 取中性值 0.5
AND user_pref 权重 MUST 均分到其他因子

---

### Requirement: 新 Skill 探索加成

排序公式 MUST 支持新 Skill（创建时间 < 7d）的可配置「探索加成」。

当探索加成启用时，新 Skill 的综合评分 MUST 额外加 0.1，防止马太效应导致新 Skill 永远不被选中。

探索加成 MUST 可通过配置开关控制，默认启用。

#### Scenario: 新 Skill 获得探索加成

WHEN 候选 Skill 的创建时间 < 7 天
AND 探索加成配置启用
THEN 该 Skill 的综合评分 MUST 额外加 0.1

#### Scenario: 老 Skill 不获得探索加成

WHEN 候选 Skill 的创建时间 ≥ 7 天
THEN 该 Skill MUST NOT 获得探索加成

#### Scenario: 探索加成可关闭

WHEN 探索加成配置关闭
THEN 所有 Skill（包括新 Skill）MUST NOT 获得探索加成

---

### Requirement: 插入点在 ResolveSkillSlugsDetailed

`skillrecommend.Rank` MUST 在 `skillruntime.ResolveSkillSlugsDetailed` 的评分阶段之后、返回结果之前被调用。

调用顺序 MUST 为：
1. `candidates := skillrouter.Detect(...)` — Layer A/B 过滤
2. `ranked := skillrecommend.Rank(ctx, candidates, intent, userCtx)` — 历史反馈重排
3. `selected := ranked[:policy.MaxSkillsInToolset]` — 截取最终结果

`Rank` MUST 在热路径中执行，SHALL NOT 引入数据库查询。所有历史数据 MUST 在调用前预加载。

#### Scenario: Rank 在 Detect 之后调用

WHEN ResolveSkillSlugsDetailed 执行
THEN skillrecommend.Rank MUST 在 skillrouter.Detect 之后被调用
AND Rank 的输入 MUST 为 Detect 的输出候选列表

#### Scenario: Rank 不引入热路径数据库查询

WHEN skillrecommend.Rank 被调用
THEN MUST NOT 在 Rank 函数内部执行数据库查询
AND 所有历史数据 MUST 通过参数传入

#### Scenario: Rank 结果截取 MaxSkillsInToolset

WHEN Rank 返回排序后的候选列表
THEN MUST 截取前 policy.MaxSkillsInToolset 个作为最终选中结果

---

### Requirement: 因子快照写入 selection_reason

排序因子 MUST 写入 `skill_invocation` 的 `selection_reason` 字段，便于事后解释「为何选 B 而非 A」。

因子快照 MUST 包含：
- 每个候选 Skill 的各因子原始值（semantic_sim、success_rate_30d、norm_duration、user_pref）
- 每个候选 Skill 的综合评分
- 探索加成是否生效
- 最终权重配置

因子快照 MUST 在 `selection_reason` JSON 中以 `ranking_factors` 键存储，与现有的路由信息共存。

#### Scenario: selection_reason 包含排序因子快照

WHEN skillrecommend.Rank 完成排序
AND selection_reason 被写入 skill_invocation
THEN selection_reason MUST 包含 ranking_factors 字段
AND ranking_factors MUST 包含每个候选的各因子值和综合评分

#### Scenario: 因子快照包含探索加成标记

WHEN 某个候选 Skill 获得了探索加成
THEN 该候选的因子快照 MUST 包含 exploration_bonus: true 标记

#### Scenario: 因子快照包含权重配置

WHEN 因子快照被写入
THEN MUST 包含当前使用的权重配置（w1/w2/w3/w4）

---

### Requirement: 去重候选组

系统 SHALL 支持名称不同但 description + 正文相似度 ≥ 0.2 的 Skill 归组为「候选去重组」。

管理面 MUST 展示「建议合并」提示，标识可能重复的 Skill 组。

合并操作 MUST 保留主 Skill，副 Skill 设置为 `archived` 状态。

去重检测 MUST 在离线分析中执行（Worker 扫描），SHALL NOT 在热路径中执行。

#### Scenario: 相似 Skill 被归入去重组

WHEN 两个 Skill 的 description + 正文相似度 ≥ 0.2
AND 名称不同
THEN 系统 MUST 将它们归入同一去重组
AND 管理面 MUST 展示「建议合并」提示

#### Scenario: 合并操作保留主 Skill

WHEN 管理员确认合并去重组
THEN 主 Skill MUST 保持 active 状态
AND 副 Skill MUST 被设置为 archived 状态

#### Scenario: 去重检测不在热路径执行

WHEN 用户发起对话触发 Skill 路由
THEN 去重检测 MUST NOT 在路由过程中执行
AND 去重检测 MUST 在 Worker 离线扫描中执行

---

### Requirement: UserPreferenceReader 端口

系统 SHALL 定义 `UserPreferenceReader` Biz 层端口，用于读取用户对 Skill 的偏好数据。

`UserPreferenceReader` MUST 为可选依赖。当无实现时，`user_pref` 因子 MUST 取中性值 0.5，权重均分到其他因子。

此端口 MUST 在 Phase 3 作为可选集成点，不依赖 Memory-Agent 的具体实现。

#### Scenario: UserPreferenceReader 可用时使用偏好数据

WHEN UserPreferenceReader 实现被注入
AND 用户有 Skill 偏好数据
THEN user_pref 因子 MUST 使用实际偏好评分

#### Scenario: UserPreferenceReader 不可用时降级

WHEN UserPreferenceReader 实现为 nil
THEN user_pref 因子 MUST 取中性值 0.5
AND user_pref 权重 MUST 均分到其他因子
AND 排序 MUST 正常执行，不抛异常

---

### Requirement: 与 skills_butler_recommend_skills 的区分

`skillrecommend.Rank`（Phase 3 运行时排序）与 `skills_butler_recommend_skills`（已有离线推荐工具）定位不同，MUST NOT 合并。

| 维度 | skillrecommend.Rank | skills_butler_recommend_skills |
|------|---------------------|-------------------------------|
| 执行时机 | 对话热路径 | Agent 离线分析 |
| 目的 | 重排候选集 | 推荐 Skill 新增/优化/移除 |
| 输入 | 候选列表 + 历史数据 | Agent 使用模式 |
| 输出 | 排序后的候选列表 | 推荐建议列表 |

#### Scenario: Rank 不替代 skills_butler 推荐

WHEN skillrecommend.Rank 被实现
THEN skills_butler_recommend_skills 工具 MUST 保持独立
AND 两者 MUST NOT 共享排序逻辑

#### Scenario: Rank 仅影响路由候选排序

WHEN Rank 重排候选列表
THEN MUST NOT 改变候选集的范围（不新增或删除候选）
AND MUST 仅改变候选的排列顺序
