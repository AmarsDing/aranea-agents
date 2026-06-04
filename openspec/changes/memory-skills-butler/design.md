# 记忆管家 & 技能管家 详细设计文档

> 日期：2026-05-31
> 状态：Draft
> 范围：经验分析引擎 + 记忆管家 + 技能管家 + 学术依据
> 前置文档：`2026-05-31-system-builtin-agents-design.md`（系统管家体系总览）

***

## 一、学术依据与设计原则

本设计基于以下学术论文的核心发现，每条设计原则均可追溯到具体论文。

### 1.1 编排任务分析

#### 论文 A：Multi-Agent Collaboration via Evolving Orchestration

* **来源**：Dang et al., "Multi-Agent Collaboration via Evolving Orchestration", arXiv:2505.19591, 2025.05（清华+腾讯）

* **核心发现**：

  * 通过强化学习训练"提线者"（Puppeteer）动态编排 Agent，系统会**自动进化出更紧凑、循环的推理结构**

  * 编排优化不是静态配置，而是**动态进化**——随着任务执行，编排器学会淘汰低效 Agent、强化高效 Agent

  * 组织拓扑在强化过程中会演化：冗余 Agent 被剪枝，关键 Agent 被强化

* **推导原则**：

  * **P-O1（编排进化原则）**：编排策略应基于历史执行数据动态调整，而非静态配置

  * **P-O2（Agent 贡献度原则）**：记录每个 Agent 在编排中的贡献度（成功率、Token 效率），用于淘汰低效成员

  * **P-O3（拓扑优化原则）**：成功的编排拓扑应被缓存和复用，失败的拓扑应被标记避免

#### 论文 B：Self-Resource Allocation in Multi-Agent LLM Systems

* **来源**：Amayuelas et al., "Self-Resource Allocation in Multi-Agent LLM Systems", ICLR 2026 submission

* **核心发现**：

  * Planner 模式（高层规划+低层自主执行）比 Orchestrator 模式（微操每个 Agent）**更高效且成本更低**

  * 暴露 Agent 的能力描述（capability profile）给 Planner，能显著提升分配策略

  * 提供显式的 worker 能力信息后，Planner 在异构 Agent 池中的分配效果显著提升

* **推导原则**：

  * **P-O4（能力画像原则）**：维护每个 Agent 的能力画像，基于历史表现动态更新

  * **P-O5（成本感知原则）**：编排决策应考虑成本效率，而非仅追求最高绝对性能

#### 论文 C：Multi-Agent LLM Orchestration Achieves Deterministic Quality

* **来源**：Drammeh, "Multi-Agent LLM Orchestration Achieves Deterministic, High-Quality Decision Support for Incident Response", arXiv:2511.15755, 2025.11

* **核心发现**：

  * 多 Agent 系统的核心价值不在速度，而在**确定性和质量**

  * 引入 **Decision Quality (DQ)** 指标：validity + specificity + correctness 三维度

  * 多 Agent 系统实现零质量方差（zero quality variance），适合生产部署

* **推导原则**：

  * **P-O6（质量优先原则）**：编排分析应关注结果质量（DQ），而非仅关注效率指标

### 1.2 记忆使用分析

#### 论文 D：How Memory Management Impacts LLM Agents

* **来源**：Lakkaraju et al., "How Memory Management Impacts LLM Agents: An Empirical Study of Experience-Following Behavior", arXiv:2505.16067, 2025.05（Harvard Business School）

* **核心发现**：

  * **选择性添加**：存储所有经验（add-all）反而比不存储更差；严格筛选后存储可提升 10% 性能

  * **历史删除**：删除"高输入相似但低输出相似"的错误经验（misaligned experience replay），能持续提升长期性能

  * **经验跟随效应**：当前任务与存储记忆相似时，输出也会相似，因此**错误记忆会传播**

  * 简单存储每条经验会导致显著更差的结果

* **推导原则**：

  * **P-M1（选择性写入原则）**：不是所有经验都值得记忆，必须基于质量标准筛选

  * **P-M2（质量驱动遗忘原则）**：主动删除低质量经验比保留更有益

  * **P-M3（错误传播防护原则）**：错误记忆会通过经验跟随效应传播，必须及时清除

#### 论文 E：Nemori - Adaptive Memory Distillation

* **来源**："What Deserves Memory: Adaptive Memory Distillation for LLM Agents", arXiv:2508.03341, 2025.08

* **核心发现**：

  * 通过**预测误差**判断什么值得记忆——如果模型已经能预测某条经验的内容，说明它是冗余的；预测失败的经验才是新知识

  * 两阶段蒸馏：Episodic Memory Integration（原始交互→叙事片段）→ Semantic Knowledge Distillation（通过预测误差提取洞察）

  * 框架与下游记忆管理系统无关（agnostic to downstream management）

* **推导原则**：

  * **P-M4（预测误差蒸馏原则）**：只记忆模型"没想到"的知识，冗余信息不存储

  * **P-M5（情景→语义蒸馏原则）**：将碎片化的情景记忆蒸馏为结构化的语义知识

#### 论文 F：MemoryField

* **来源**："MemoryField: Exploiting Gravitational Field for Long-Term Memory Management", ICLR 2026 submission

* **核心发现**：

  * 用"引力场"模型管理记忆——语义相似的节点互相吸引、重复的互相排斥、注意力驱动力、衰减机制

  * 活跃度低于阈值的节点自动标记为"遗忘"

  * 融合和遗忘过程确保语义连贯性和认知稳定性

* **推导原则**：

  * **P-M6（活跃度衰减原则）**：长期未被检索的记忆应自动降权，最终遗忘

  * **P-M7（语义去重原则）**：语义高度重复的记忆应合并，避免冗余

#### 论文 G：Forgetful but Faithful - MaRS

* **来源**："Forgetful but Faithful: A Cognitive Memory Architecture and Benchmark for Privacy-Aware Generative Agents", arXiv:2512.12856, 2025.12

* **核心发现**：

  * 6 种遗忘策略：FIFO、LRU、Priority Decay、Reflection-Summary、Random-Drop、Hybrid

  * 遗忘不是损失，而是**保持记忆健康**的必要机制

  * MaRS（Memory-Aware Retention Schema）提供策略可寻址的保留评分

* **推导原则**：

  * **P-M8（策略化遗忘原则）**：遗忘策略应可配置，不同场景适用不同策略

  * **P-M9（记忆健康原则）**：定期清理是记忆系统健康的必要条件

### 1.3 设计原则汇总

| ID   | 原则          | 来源论文 | 应用到        |
| ---- | ----------- | ---- | ---------- |
| P-O1 | 编排进化原则      | 论文 A | 技能管家：编排分析  |
| P-O2 | Agent 贡献度原则 | 论文 A | 技能管家：编排分析  |
| P-O3 | 拓扑优化原则      | 论文 A | 技能管家：编排分析  |
| P-O4 | 能力画像原则      | 论文 B | 技能管家：工具权重  |
| P-O5 | 成本感知原则      | 论文 B | 技能管家：编排分析  |
| P-O6 | 质量优先原则      | 论文 C | 技能管家：编排分析  |
| P-M1 | 选择性写入原则     | 论文 D | 记忆管家：选择性记忆 |
| P-M2 | 质量驱动遗忘原则    | 论文 D | 记忆管家：遗忘    |
| P-M3 | 错误传播防护原则    | 论文 D | 记忆管家：遗忘    |
| P-M4 | 预测误差蒸馏原则    | 论文 E | 记忆管家：蒸馏    |
| P-M5 | 情景→语义蒸馏原则   | 论文 E | 记忆管家：蒸馏    |
| P-M6 | 活跃度衰减原则     | 论文 F | 记忆管家：衰减    |
| P-M7 | 语义去重原则      | 论文 F | 记忆管家：去重    |
| P-M8 | 策略化遗忘原则     | 论文 G | 记忆管家：遗忘策略  |
| P-M9 | 记忆健康原则      | 论文 G | 记忆管家：做梦    |

***

## 二、经验分析引擎（Experience Analytics Engine）

### 2.1 定位

经验分析引擎是**共享基础设施层**，为记忆管家和技能管家提供数据分析能力。它不是 Agent，而是 biz 层的 Usecase，被管家工具调用。

### 2.2 数据来源

| 数据源        | 表/存储                                          | 可分析维度                     |
| ---------- | --------------------------------------------- | ------------------------- |
| 工具调用记录     | `tool_invocation`（EvolutionMetricsRepo）       | 工具成功率、调用频率、失败原因、耗时分布      |
| Skill 调用记录 | `skill_invocations`（SkillUsageTrackerPlugin）  | Skill 调用次数、成功率、7天趋势、平均耗时  |
| 编排执行记录     | `team_runs` + `team_run_steps`                | 编排成功率、模式效率、成员贡献度、Token 消耗 |
| Token 用量   | `token_usage_events`（UsageUsecase）            | 成本分析、模型效率、Agent 级别消耗      |
| 记忆检索记录     | `memory_search`/`memory_load` 工具调用            | 记忆命中率、检索质量、未命中分析          |
| 进化指标       | `evolution_metrics` + `evolution_suggestions` | 工具成功率趋势、检索质量趋势、负反馈        |

### 2.3 核心分析能力

```go
type ExperienceAnalyticsUsecase struct {
    toolInvRepo    biz.EvolutionMetricsRepo
    skillRepo      biz.SkillQueryReader
    teamRepo       biz.TeamRepository
    usageRepo      usage.AnalyticsRepo
    memoryAdmin    *biz.MemoryAdminUsecase
}
```

| 方法                         | 功能            | 输出                                                                                             | 原则            |
| -------------------------- | ------------- | ---------------------------------------------------------------------------------------------- | ------------- |
| `AnalyzeToolWeights()`     | 分析工具调用权重      | `{tool_key: {call_count, success_rate, avg_duration, weight_score}}`                           | P-O4          |
| `AnalyzeSkillHealth()`     | 分析 Skill 健康度  | `{skill_id: {invoke_count_7d, success_rate, avg_duration_ms, trend, health_status}}`           | —             |
| `AnalyzeOrchestration()`   | 分析编排效率        | `{mode: {success_rate, avg_tokens, avg_duration, member_contributions, dq_score}}`             | P-O1/O2/O5/O6 |
| `AnalyzeMemoryQuality()`   | 分析记忆质量        | `{hit_rate, miss_rate, redundancy_score, misaligned_count, inactive_count}`                    | P-M1/M2/M3    |
| `AnalyzeAgentCapability()` | 分析 Agent 能力画像 | `{agent_id: {tool_success_rates, skill_scores, orchestration_contributions, cost_efficiency}}` | P-O4          |

### 2.4 编排分析指标（Decision Quality）

基于论文 C 的 DQ 框架，定义编排质量指标：

```go
type OrchestrationQuality struct {
    Validity    float64  // 任务是否完成（success_rate）
    Specificity float64  // 结果具体程度（基于 output_preview 的信息熵）
    Correctness float64  // 结果正确程度（基于用户反馈 / 后续修正率）
    Efficiency  float64  // Token 效率（result_quality / total_tokens）
    DQScore     float64  // 综合评分 = w1*Validity + w2*Specificity + w3*Correctness
}
```

### 2.5 记忆质量指标

基于论文 D/E/F 的发现，定义记忆质量指标：

```go
type MemoryQualityReport struct {
    HitRate          float64   // 记忆检索命中率
    MissRate         float64   // 未命中率
    RedundancyScore  float64   // 语义重复度（基于 embedding cosine similarity > 0.95 的比例）
    MisalignedCount  int       // "高输入相似但低输出相似"的经验数
    InactiveCount    int       // 超过 N 天未被检索的记忆数
    PredictableCount int       // 预测误差低于阈值的"冗余"记忆数
    HealthScore      float64   // 综合健康度
}
```

***

## 三、记忆管家详细设计

### 3.1 定位

记忆管家（`__memory__`）是系统内置管家，负责**选择性记忆 + 质量驱动遗忘 + 记忆蒸馏**。它不是简单的记忆 CRUD，而是基于学术原则的智能记忆管理者。

### 3.2 Agent 定义

| 维度            | 值                                            |
| ------------- | -------------------------------------------- |
| Agent Key     | `__memory__`                                 |
| Kind          | `system_builtin`                             |
| Model         | `gpt-4.1`（需要强推理能力做预测误差判断）                    |
| Tools Profile | `system_memory`                              |
| System Prompt | `internal/scenario/system/prompts/memory.md` |

### 3.3 专属工具集

所有工具使用 `function.NewFunctionTool[I, O]` 构建，注册到 `internal/tools/memory_butler/` 包。

| 工具名                      | 功能                                   | 学术原则       | 调用的 Usecase                                |
| ------------------------ | ------------------------------------ | ---------- | ------------------------------------------ |
| `analyze_memory_quality` | 分析记忆命中率、冗余度、misaligned 数量            | P-M1/M2/M3 | `ExperienceAnalytics.AnalyzeMemoryQuality` |
| `selective_remember`     | 基于预测误差选择性写入记忆                        | P-M4       | `MemoryUsecase.Remember` + 预测误差计算          |
| `forget_low_quality`     | 删除 misaligned experience（高输入相似低输出相似） | P-M2/M3    | `MemoryAdminUsecase.DeleteFact`            |
| `forget_inactive`        | 对长期未被检索的记忆降权/遗忘                      | P-M6       | `MemoryAdminUsecase` + 活跃度衰减               |
| `deduplicate_memories`   | 合并语义高度重复的记忆                          | P-M7       | `MemoryUsecase` + embedding 相似度            |
| `consolidate_episodes`   | 将碎片化情景记忆蒸馏为语义知识                      | P-M5       | `MemoryAdminUsecase` + LLM 蒸馏              |
| `dream_cycle`            | 触发离线记忆整理（整合+遗忘+蒸馏）                   | P-M9       | 编排以上所有工具的复合操作                              |

### 3.4 selective\_remember 工具核心逻辑

基于论文 E（Nemori）的预测误差蒸馏原则：

```
输入：{content: string, context: string, agent_id: string}

1. 预测测试：
   → 用当前 Agent 的模型对 content 做预测（给定 context，让模型预测 content）
   → 计算预测误差 = 1 - cosine_similarity(prediction, actual_content)

2. 决策：
   → 如果预测误差 > 阈值（如 0.3）：这是"没想到"的知识 → 写入记忆
   → 如果预测误差 <= 阈值：这是冗余信息 → 跳过

3. 写入时质量检查：
   → 检查是否与已有记忆语义重复（embedding cosine > 0.95）→ 合并而非新增
   → 检查是否包含敏感信息 → 标记为 private
```

### 3.5 forget\_low\_quality 工具核心逻辑

基于论文 D（Harvard）的 misaligned experience replay 检测：

```
输入：{agent_id: string, dry_run: bool}

1. 扫描记忆库：
   → 对每条记忆，计算其"输入相似度"（与近期查询的相似度）和"输出相似度"（与实际结果的相似度）

2. 识别 misaligned 记忆：
   → 高输入相似度（>0.8）+ 低输出相似度（<0.5）= misaligned
   → 这些记忆会在经验跟随效应中传播错误

3. 执行遗忘：
   → 如果 dry_run=true：只返回待删除列表
   → 如果 dry_run=false：执行删除 + 记录遗忘日志
```

### 3.6 dream\_cycle 工具核心逻辑

基于论文 G（MaRS）的记忆健康原则，dream\_cycle 是一个**复合操作**，编排多个工具：

```
dream_cycle 执行流程：
  1. analyze_memory_quality → 获取当前记忆健康报告
  2. forget_low_quality → 删除 misaligned 记忆
  3. forget_inactive → 衰减/遗忘长期未检索的记忆
  4. deduplicate_memories → 合并语义重复的记忆
  5. consolidate_episodes → 将情景记忆蒸馏为语义知识
  6. 生成 dream_report（做了什么、删了什么、蒸馏了什么）
```

**触发方式**：

* 定时触发：`cronrunner` 每天凌晨触发 `dream_cycle`

* 手动触发：用户对记忆管家说"整理一下记忆"

* 阈值触发：当 `MemoryQualityReport.HealthScore < 0.6` 时自动触发

### 3.7 遗忘策略配置

基于论文 G（MaRS）的 6 种策略，支持可配置：

```go
type ForgetConfig struct {
    Policy           string  // "fifo" | "lru" | "priority_decay" | "reflection_summary" | "random_drop" | "hybrid"
    MaxMemoryCount   int     // 记忆条数上限
    MaxMemoryAgeDays int     // 记忆最大保留天数
    InactiveThresholdDays int // 不活跃阈值天数
    MisalignedInputSimThreshold  float64 // misaligned 检测：输入相似度阈值
    MisalignedOutputSimThreshold float64 // misaligned 检测：输出相似度阈值
    PredictionErrorThreshold     float64 // 预测误差阈值（低于此值视为冗余）
    DedupSimThreshold            float64 // 去重相似度阈值
}
```

默认策略为 `hybrid`（结合 priority\_decay + lru + reflection\_summary）。

***

## 四、技能管家详细设计

### 4.1 定位

技能管家（`__skills__`）是系统内置管家，负责**Skill 进化/消亡 + 工具权重优化 + 编排分析**。它基于历史使用数据做出数据驱动的决策。

### 4.2 Agent 定义

| 维度            | 值                                            |
| ------------- | -------------------------------------------- |
| Agent Key     | `__skills__`                                 |
| Kind          | `system_builtin`                             |
| Model         | `gpt-4.1`                                    |
| Tools Profile | `system_skills`                              |
| System Prompt | `internal/scenario/system/prompts/skills.md` |

### 4.3 专属工具集

| 工具名                      | 功能                        | 学术原则             | 调用的 Usecase                                |
| ------------------------ | ------------------------- | ---------------- | ------------------------------------------ |
| `analyze_skill_health`   | 分析 Skill 调用频率、成功率、趋势      | —                | `ExperienceAnalytics.AnalyzeSkillHealth`   |
| `evolve_skill`           | 基于失败模式分析优化 Skill          | P-O1             | `SkillUsecase.PatchSkill` + LLM 优化         |
| `retire_skill`           | 标记长期低效 Skill 为 deprecated | P-O2             | `SkillUsecase.UpdateSkillEnabled`          |
| `recommend_skills`       | 基于任务模式推荐 Skill 组合         | P-O4             | `SkillUsecase.ScoreByEmbedding`            |
| `analyze_tool_weights`   | 分析工具调用权重，生成调整建议           | P-O4/O5          | `ExperienceAnalytics.AnalyzeToolWeights`   |
| `analyze_orchestration`  | 分析编排效率、模式对比、成员贡献度         | P-O1/O2/O3/O5/O6 | `ExperienceAnalytics.AnalyzeOrchestration` |
| `optimize_orchestration` | 基于分析结果优化编排策略              | P-O1/O3          | 生成编排建议（不自动执行，需用户确认）                        |

### 4.4 Skill 健康度模型

```go
type SkillHealth struct {
    SkillID           string
    InvokeCount7d     int       // 7天调用次数
    SuccessRate       float64   // 成功率
    AvgDurationMS     float64   // 平均耗时
    Trend             string    // "rising" | "stable" | "declining" | "dormant"
    HealthStatus      string    // "healthy" | "warning" | "critical" | "dormant"
    Recommendation    string    // "keep" | "evolve" | "retire" | "merge"
}
```

**健康度判定规则**：

| 条件                      | HealthStatus | Recommendation |
| ----------------------- | ------------ | -------------- |
| 调用次数 > 10/周 且 成功率 > 80% | healthy      | keep           |
| 调用次数 > 5/周 且 成功率 60-80% | warning      | evolve         |
| 调用次数 < 2/周 或 成功率 < 60%  | critical     | evolve/retire  |
| 30天无调用                  | dormant      | retire         |

### 4.5 evolve\_skill 工具核心逻辑

基于论文 A（Evolving Orchestration）的进化原则：

```
输入：{skill_id: string, failure_patterns: []string}

1. 加载 Skill 当前版本（body + tags + description）
2. 分析失败模式：
   → 从 skill_invocations 中提取失败案例的 input_preview + error
   → LLM 分析失败原因（prompt 不够精确？参数缺失？边界条件？）
3. 生成优化方案：
   → LLM 生成优化后的 body/prompt
   → 计算与当前版本的 diff
4. 创建新版本：
   → 调用 SkillUsecase.CreateSkillWithVersion
   → 标记为 pending review（需用户确认后启用）
```

### 4.6 retire\_skill 工具核心逻辑

基于论文 A（淘汰低效 Agent）和论文 B（成本感知）：

```
输入：{skill_id: string, reason: string}

1. 验证 Skill 确实低效：
   → 调用 analyze_skill_health 确认 health_status = dormant/critical
2. 检查依赖：
   → 是否有 Agent 的 tools_allow 引用此 Skill？
   → 是否有编排模板依赖此 Skill？
3. 执行退役：
   → 调用 SkillUsecase.UpdateSkillEnabled(id, false)
   → 标记 description 添加 "[已退役] {reason}"
   → 通知依赖此 Skill 的 Agent 的所有者
```

### 4.7 analyze\_orchestration 工具核心逻辑

基于论文 A/B/C 的编排分析原则：

```
输入：{time_range: string, mode_filter: string}

1. 加载编排执行历史：
   → 从 team_runs + team_run_steps 聚合数据
2. 按模式分组分析：
   → 每种模式（sequential/parallel/coordinator/swarm/adaptive）的：
     - 成功率（Validity）
     - 平均 Token 消耗（Efficiency）
     - 平均耗时
     - 成员贡献度（每个 Agent 的 success_count / total_count）
3. 计算 DQ Score：
   → Validity = success_rate
   → Specificity = avg(output_entropy)  // 输出信息熵
   → Correctness = 1 - (negative_feedback_rate)
   → DQ = 0.4*Validity + 0.3*Specificity + 0.3*Correctness
4. 生成优化建议：
   → 哪种模式在什么场景下表现最好
   → 哪些 Agent 贡献度低建议替换
   → 哪些编排拓扑值得缓存复用
```

### 4.8 工具权重模型

基于论文 B（能力画像原则）：

```go
type ToolWeightReport struct {
    ToolKey       string
    CallCount     int
    SuccessRate   float64
    AvgDurationMS float64
    WeightScore   float64  // 综合权重 = normalize(success_rate) * 0.5 + normalize(call_count) * 0.3 + normalize(1/duration) * 0.2
    Recommendation string  // "promote" | "demote" | "keep" | "disable"
}
```

**权重应用**：工具权重影响 `tools.Assemble()` 中工具的排序——高权重工具优先暴露给 LLM，低权重工具降级或隐藏。

***

## 五、与现有系统的集成

### 5.1 与 EvolutionUsecase 的关系

| 现有能力                                             | 技能管家扩展                    |
| ------------------------------------------------ | ------------------------- |
| `GetEvolutionMetrics`（工具成功率、检索质量）                | 复用，增加 Skill 维度            |
| `ScanAll`（定时扫描进化指标）                              | 技能管家可触发扫描                 |
| `ApplySuggestion`（应用进化建议）                        | `evolve_skill` 工具复用此能力    |
| `EvolutionSuggestion`（persona/prompt/skill 类型建议） | 技能管家新增 `tool_weight` 类型建议 |

### 5.2 与 LearningLoopUsecase 的关系

| 现有能力                                                                | 记忆管家扩展                                 |
| ------------------------------------------------------------------- | -------------------------------------- |
| `CollectObservations`（tool\_call/feedback/memory\_hit/memory\_miss） | 记忆管家消费 memory\_hit/memory\_miss 观察     |
| `DetectPatterns`（频率>=3 且置信度>=10%）                                   | 记忆管家提供更精准的观察（预测误差维度）                   |
| `GenerateProposals`（生成知识提案）                                         | 记忆管家的 `consolidate_episodes` 产出更高质量的提案 |

### 5.3 与 SkillUsageTrackerPlugin 的关系

现有插件记录 Skill 调用数据，技能管家消费这些数据：

```
SkillUsageTrackerPlugin（BeforeTool/AfterTool 钩子）
  → skill_invocations 表
    → ExperienceAnalytics.AnalyzeSkillHealth()
      → 技能管家 analyze_skill_health / evolve_skill / retire_skill
```

### 5.4 与 MemoryAdminUsecase 的关系

记忆管家通过现有 MemoryAdminUsecase 执行记忆操作，不绕过：

```
记忆管家工具
  → MemoryAdminUsecase（L0-L4 CRUD + 索引同步）
    → MemoryUsecase（embedding + pgvector）
      → data 层（SQLite + PostgreSQL）
```

***

## 六、实现架构

### 6.1 后端新增文件

```
internal/
  biz/
    experience_analytics.go        # 经验分析引擎 Usecase
    experience_analytics_types.go  # 分析报告类型定义
  tools/
    memory_butler/
      registry.go                  # 记忆管家工具注册
      analyze_quality.go           # analyze_memory_quality
      selective_remember.go        # selective_remember
      forget_low_quality.go        # forget_low_quality
      forget_inactive.go           # forget_inactive
      deduplicate.go               # deduplicate_memories
      consolidate_episodes.go      # consolidate_episodes
      dream_cycle.go               # dream_cycle（复合操作）
    skills_butler/
      registry.go                  # 技能管家工具注册
      analyze_skill_health.go      # analyze_skill_health
      evolve_skill.go              # evolve_skill
      retire_skill.go              # retire_skill
      recommend_skills.go          # recommend_skills
      analyze_tool_weights.go      # analyze_tool_weights
      analyze_orchestration.go     # analyze_orchestration
      optimize_orchestration.go    # optimize_orchestration
  scenario/
    system/
      prompts/
        memory.md                  # 记忆管家 system prompt
        skills.md                  # 技能管家 system prompt
```

### 6.2 种子数据扩展

在 `seed_system_agents.go` 中新增：

```go
{
    AgentKey:     "__memory__",
    DisplayName:  "记忆管家",
    Description:  "基于学术原则的智能记忆管理者：选择性记忆、质量驱动遗忘、记忆蒸馏",
    Kind:         "system_builtin",
    ToolsProfile: "system_memory",
    Model:        "gpt-4.1",
},
{
    AgentKey:     "__skills__",
    DisplayName:  "技能管家",
    Description:  "基于使用数据的技能进化/消亡决策、工具权重优化、编排分析",
    Kind:         "system_builtin",
    ToolsProfile: "system_skills",
    Model:        "gpt-4.1",
},
```

### 6.3 工具注入路径

对齐现有 `cliAdminTools` 模式：

```go
// internal/service/system_builtin_tools.go
func (o *ChatOrchestrator) memoryButlerTools(ctx context.Context, ag biz.Agent) []trpctool.Tool {
    if ag.AgentKey != "__memory__" { return nil }
    return memory_butler.RegisterAll(memory_butler.Deps{...})
}

func (o *ChatOrchestrator) skillsButlerTools(ctx context.Context, ag biz.Agent) []trpctool.Tool {
    if ag.AgentKey != "__skills__" { return nil }
    return skills_butler.RegisterAll(skills_butler.Deps{...})
}
```

### 6.4 定时任务

| 任务                       | Cron 表达式             | 说明          |
| ------------------------ | -------------------- | ----------- |
| `dream_cycle`            | `0 3 * * *`（每天凌晨3点）  | 记忆管家离线整理    |
| `skill_health_scan`      | `0 4 * * 1`（每周一凌晨4点） | Skill 健康度扫描 |
| `orchestration_analysis` | `0 5 * * 1`（每周一凌晨5点） | 编排效率分析      |

***

## 七、分期实施计划

### P0：经验分析引擎 + 基础工具

1. `ExperienceAnalyticsUsecase` 核心分析能力
2. 记忆管家：`analyze_memory_quality` + `forget_low_quality` + `forget_inactive`
3. 技能管家：`analyze_skill_health` + `analyze_tool_weights` + `analyze_orchestration`
4. 种子数据 + Prompt 文件
5. 定时任务：dream\_cycle + skill\_health\_scan

### P1：高级能力

1. 记忆管家：`selective_remember`（预测误差蒸馏）+ `consolidate_episodes`
2. 技能管家：`evolve_skill` + `retire_skill` + `optimize_orchestration`
3. 工具权重应用到 `tools.Assemble()` 排序
4. 编排拓扑缓存与复用

### P2：策略化与自适应

1. 遗忘策略可配置（MaRS 6 种策略）
2. 编排策略自适应（基于 DQ Score 自动推荐模式）
3. Agent 能力画像自动更新
4. 前端：记忆健康仪表盘 + Skill 健康仪表盘

***

## 八、风险与缓解

| 风险                 | 影响         | 缓解措施                                   |
| ------------------ | ---------- | -------------------------------------- |
| 预测误差计算消耗额外 Token   | 记忆写入成本增加   | 仅对候选记忆做轻量预测，不做全量；P0 阶段先用简单规则替代         |
| 遗忘策略误删重要记忆         | 用户体验下降     | dream\_cycle 默认 dry\_run=true，需用户确认后执行 |
| Skill 退役影响依赖 Agent | Agent 功能缺失 | retire\_skill 前检查依赖，通知所有者              |
| 编排分析数据不足           | 分析结果不可靠    | 要求最少 10 次编排记录才生成分析报告                   |
| 经验分析引擎查询慢          | 管家工具响应超时   | 异步分析 + 结果缓存，工具返回缓存报告                   |

---

## 九、AI 落地补充规范

> 本节补充 AI 编码时必须知晓的实现细节，缺失这些内容将导致实现错误。

### 9.1 `ExperienceAnalyticsUsecase` 的仓库接口与 Wire 配置

**问题**：文档 §2.3 定义了 `ExperienceAnalyticsUsecase` 的结构体，但未定义其依赖的仓库接口和 Wire 注册方式。

**仓库接口**：`ExperienceAnalyticsUsecase` **不新增仓库接口**，而是组合现有接口：

```go
type ExperienceAnalyticsUsecase struct {
    toolInvRepo  biz.EvolutionMetricsRepo     // 现有：工具调用统计
    skillRepo    biz.SkillQueryReader          // 现有：Skill 查询
    teamRunRepo  biz.TeamRepository            // 现有：Team/TeamRun 查询
    usageRepo    usage.AnalyticsRepo           // 现有：Token 用量分析
    memoryAdmin  *biz.MemoryAdminUsecase       // 现有：记忆管理
    sessionRepo  biz.SessionRepository         // 现有：Session 查询
}
```

**Wire 注册**：

```go
// internal/biz/biz.go
var ProviderSet = wire.NewSet(
    // ... 现有
    NewExperienceAnalyticsUsecase,
)

// 构造函数
func NewExperienceAnalyticsUsecase(
    toolInvRepo biz.EvolutionMetricsRepo,
    skillRepo biz.SkillQueryReader,
    teamRunRepo biz.TeamRepository,
    usageRepo usage.AnalyticsRepo,
    memoryAdmin *MemoryAdminUsecase,
    sessionRepo biz.SessionRepository,
) *ExperienceAnalyticsUsecase {
    return &ExperienceAnalyticsUsecase{
        toolInvRepo: toolInvRepo,
        skillRepo:   skillRepo,
        teamRunRepo: teamRunRepo,
        usageRepo:   usageRepo,
        memoryAdmin: memoryAdmin,
        sessionRepo: sessionRepo,
    }
}
```

### 9.2 `selective_remember` 预测误差的精确计算流程

**问题**：文档 §3.4 说"用当前 Agent 的模型对 content 做预测"，但未定义具体 prompt 和计算方式。

**P0 阶段（简单规则替代）**：

P0 阶段不做 LLM 预测调用（节省 Token），改用**语义新颖度规则**：

```go
func (t *selectiveRememberTool) shouldRemember(ctx context.Context, content string, agentID string) (bool, error) {
    // 1. 计算候选内容的 embedding
    emb, err := t.deps.Embedder.Embed(ctx, content)
    if err != nil { return true, err }  // embedding 失败时默认记住

    // 2. 在已有记忆中搜索最相似的
    similar, err := t.deps.MemoryRepo.FindSimilar(ctx, agentID, emb, 5)
    if err != nil { return true, err }

    // 3. 如果最相似的记忆 cosine > 0.85，视为冗余，不记忆
    if len(similar) > 0 && cosineSimilarity(emb, similar[0].Embedding) > 0.85 {
        return false, nil
    }

    // 4. 否则值得记忆
    return true, nil
}
```

**P1 阶段（预测误差蒸馏）**：

```go
func (t *selectiveRememberTool) shouldRememberWithPrediction(ctx context.Context, content, contextStr, agentID string) (bool, error) {
    // 1. 先做简单规则检查（零 Token 消耗）
    if simple, err := t.shouldRemember(ctx, content, agentID); !simple || err != nil {
        return simple, err
    }

    // 2. 调用 LLM 做预测（给定 context，让模型预测 content 的关键信息）
    prediction, err := t.deps.LLM.Predict(ctx, fmt.Sprintf(
        "Based on the following context, predict what key information the user would want to remember:\n\nContext: %s\n\nPrediction:", contextStr,
    ))

    // 3. 计算预测误差
    predEmb, _ := t.deps.Embedder.Embed(ctx, prediction)
    actualEmb, _ := t.deps.Embedder.Embed(ctx, content)
    predictionError := 1 - cosineSimilarity(predEmb, actualEmb)

    // 4. 误差 > 阈值 → 值得记忆
    return predictionError > t.deps.PredictionErrorThreshold, nil
}
```

### 9.3 `forget_low_quality` 的 misaligned 检测精确定义

**问题**：文档 §3.5 说"高输入相似度+低输出相似度"，但"输入"和"输出"的语义未定义。

**精确定义**：

在记忆系统中，每条记忆（fact）有 `statement` 字段。当 Agent 检索记忆时，会产生 `memory_search`/`memory_load` 工具调用记录。

- **输入相似度**：记忆的 `statement` 与**触发检索的查询**之间的 embedding 相似度
- **输出相似度**：记忆的 `statement` 与**检索后 Agent 实际生成的回复**之间的 embedding 相似度

**misaligned 判定**：如果一条记忆经常被检索到（高输入相似度 > 0.8），但检索后 Agent 的回复与该记忆内容差异大（低输出相似度 < 0.5），说明这条记忆**被检索了但没被使用**，可能是过时或误导性的。

**P0 阶段简化实现**：

P0 阶段不追踪"输出相似度"（需要额外记录 Agent 回复与记忆的关联），改用**检索后负反馈率**：

```go
func (t *forgetLowQualityTool) detectMisaligned(ctx context.Context, agentID string) ([]string, error) {
    // 1. 获取该 Agent 的所有记忆
    facts, err := t.deps.MemoryAdmin.ListFacts(ctx, agentID)

    // 2. 对每条记忆，检查：
    //    - 被检索次数（从 memory_search 工具调用记录统计）
    //    - 检索后负反馈次数（从 feedback 记录统计）
    var misaligned []string
    for _, fact := range facts {
        retrievalCount := t.getRetrievalCount(ctx, fact.ID)
        negativeFeedbackCount := t.getNegativeFeedbackAfterRetrieval(ctx, fact.ID)
        if retrievalCount >= 3 && float64(negativeFeedbackCount)/float64(retrievalCount) > 0.5 {
            misaligned = append(misaligned, fact.ID)
        }
    }
    return misaligned, nil
}
```

### 9.4 `dream_cycle` 作为 cron 任务的执行路径

**问题**：文档 §3.6 说"定时触发：cronrunner 每天凌晨触发"，但 cron 任务通过 `CronChatRunner.RunCronTurn` 触发 Agent 对话，dream_cycle 不是对话而是后台操作。

**决策**：dream_cycle 有两种执行路径：

**路径 A：通过 CronChatRunner 触发（推荐）**

将 dream_cycle 包装为一次"对话"——cron 向记忆管家发送"请执行 dream_cycle"消息，记忆管家通过工具调用完成整理。这与现有 cron 机制完全兼容。

```go
// cronrunner 注册
cronTask := biz.CronTask{
    AgentID:    memoryButlerAgentID,  // __memory__ Agent
    Message:    "请执行 dream_cycle，整理记忆系统",
    Schedule:   "0 3 * * *",
}
```

**路径 B：直接调用 biz 层方法（备选）**

在 cronrunner 中新增 `dream_cycle` 类型任务，直接调用 `ExperienceAnalyticsUsecase` + `MemoryAdminUsecase` 方法，不经过 Agent LLM。成本更低但灵活性差。

**P0 阶段使用路径 A**，P2 阶段可考虑路径 B 优化成本。

### 9.5 `ForgetConfig` 的存储位置

**问题**：文档 §3.7 定义了 `ForgetConfig`，但未说明存储位置。

**决策**：存储在 Agent 的 `agent_runtime_settings` 表中，作为 JSON 字段。

```go
// 在 biz.AgentRuntimeSettings 中新增字段
type AgentRuntimeSettings struct {
    // ... 现有字段
    ForgetPolicyJSON string `json:"forget_policy_json,omitempty"`
}
```

**默认值**（种子数据中设置）：

```json
{
    "policy": "hybrid",
    "max_memory_count": 1000,
    "max_memory_age_days": 90,
    "inactive_threshold_days": 30,
    "misaligned_input_sim_threshold": 0.8,
    "misaligned_output_sim_threshold": 0.5,
    "prediction_error_threshold": 0.3,
    "dedup_sim_threshold": 0.95
}
```

### 9.6 `evolve_skill` 的 LLM 调用方式

**问题**：文档 §4.5 说"LLM 分析失败原因"和"LLM 生成优化后的 body"，但未说明如何调用 LLM。

**决策**：通过 `internal/provider` 的现有 Provider 系统调用 LLM，使用编排管家自身的模型配置。

```go
func (t *evolveSkillTool) callLLM(ctx context.Context, prompt string) (string, error) {
    // 使用 Agent 的 Provider/Model 配置
    model, err := t.deps.Provider.TRPCModelForProviderModel(ctx, t.deps.ProviderCode, t.deps.ModelAPIID)
    if err != nil { return "", err }

    resp, err := model.Generate(ctx, []*trpcmodel.Content{{Role: "user", Parts: []trpcmodel.Part{{Text: prompt}}}})
    if err != nil { return "", err }
    return resp.Text(), nil
}
```

**Deps 中新增**：

```go
type Deps struct {
    // ... 现有
    Provider    biz.LlmProviderModelUsecase
    ProviderCode string
    ModelAPIID   string
}
```

### 9.7 工具权重应用到 `tools.Assemble()` 的方式

**问题**：文档 §4.8 说"工具权重影响 `tools.Assemble()` 中工具的排序"，但未说明如何影响。

**决策**：在 `AssemblyConfig` 中新增 `ToolWeightOverrides` 字段。

```go
// internal/tools/toolset.go AssemblyConfig 新增字段
type AssemblyConfig struct {
    // ... 现有字段
    ToolWeightOverrides map[string]float64  // tool_key -> weight_score，影响排序
}
```

**排序逻辑**：在 `Assemble()` 中，当 `ToolWeightOverrides` 非空时，按权重降序排列工具。高权重工具排在前面，LLM 更容易选择。低权重工具（weight < 0.3）标记为 `SkipSummarization`，减少 prompt 占用。

### 9.8 HealthScore 和 DQ Score 的计算公式

**HealthScore**（记忆健康度）：

```
HealthScore = 0.3 * hit_rate
            + 0.2 * (1 - redundancy_score)
            + 0.2 * (1 - misaligned_count / max(total_facts, 1))
            + 0.15 * (1 - inactive_count / max(total_facts, 1))
            + 0.15 * (1 - predictable_count / max(total_facts, 1))
```

范围 [0, 1]，低于 0.6 触发 dream_cycle。

**DQ Score**（编排质量）：

```
DQ = 0.4 * Validity
   + 0.3 * Specificity
   + 0.3 * Correctness
```

- Validity = success_rate（0~1）
- Specificity = min(avg_output_length / 500, 1.0)（输出越长越具体，上限1）
- Correctness = 1 - negative_feedback_rate（0~1）

### 9.9 记忆管家和技能管家工具的 Go struct 定义

**记忆管家工具**（`internal/tools/memory_butler/`）：

```go
type AnalyzeMemoryQualityInput struct {
    AgentID string `json:"agent_id" jsonschema:"description=Agent ID，为空则分析全局"`
}
type AnalyzeMemoryQualityOutput struct {
    HitRate          float64 `json:"hit_rate"`
    MissRate         float64 `json:"miss_rate"`
    RedundancyScore  float64 `json:"reduancy_score"`
    MisalignedCount  int     `json:"misaligned_count"`
    InactiveCount    int     `json:"inactive_count"`
    PredictableCount int     `json:"predictable_count"`
    HealthScore      float64 `json:"health_score"`
}

type SelectiveRememberInput struct {
    Content  string `json:"content" jsonschema:"description=待记忆的内容"`
    Context  string `json:"context" jsonschema:"description=上下文信息"`
    AgentID  string `json:"agent_id" jsonschema:"description=Agent ID"`
}
type SelectiveRememberOutput struct {
    Remembered bool   `json:"remembered"`
    Reason     string `json:"reason"`
}

type ForgetLowQualityInput struct {
    AgentID string `json:"agent_id" jsonschema:"description=Agent ID"`
    DryRun  bool   `json:"dry_run" jsonschema:"description=true则只返回待删除列表不执行"`
}
type ForgetLowQualityOutput struct {
    DeletedCount int      `json:"deleted_count"`
    DeletedIDs   []string `json:"deleted_ids"`
}

type ForgetInactiveInput struct {
    AgentID             string `json:"agent_id"`
    InactiveThresholdDays int   `json:"inactive_threshold_days"`
    DryRun               bool  `json:"dry_run"`
}
type ForgetInactiveOutput struct {
    ForgottenCount int      `json:"forgotten_count"`
    ForgottenIDs   []string `json:"forgotten_ids"`
}

type DeduplicateMemoriesInput struct {
    AgentID        string  `json:"agent_id"`
    SimThreshold   float64 `json:"sim_threshold"`
}
type DeduplicateMemoriesOutput struct {
    MergedCount int `json:"merged_count"`
}

type ConsolidateEpisodesInput struct {
    AgentID string `json:"agent_id"`
}
type ConsolidateEpisodesOutput struct {
    DistilledCount int `json:"distilled_count"`
}

type DreamCycleInput struct {
    AgentID string `json:"agent_id"`
    DryRun  bool   `json:"dry_run"`
}
type DreamCycleOutput struct {
    QualityBefore   float64 `json:"quality_before"`
    QualityAfter    float64 `json:"quality_after"`
    ActionsTaken    []string `json:"actions_taken"`
    DeletedCount    int     `json:"deleted_count"`
    MergedCount     int     `json:"merged_count"`
    DistilledCount  int     `json:"distilled_count"`
}
```

**技能管家工具**（`internal/tools/skills_butler/`）：

```go
type AnalyzeSkillHealthInput struct {
    SkillID string `json:"skill_id" jsonschema:"description=Skill ID，为空则分析全部"`
}
type AnalyzeSkillHealthOutput struct {
    Skills []SkillHealth `json:"skills"`
}

type EvolveSkillInput struct {
    SkillID         string   `json:"skill_id" jsonschema:"description=Skill ID"`
    FailurePatterns []string `json:"failure_patterns" jsonschema:"description=失败模式描述"`
}
type EvolveSkillOutput struct {
    NewVersion  string `json:"new_version"`
    DiffPreview string `json:"diff_preview"`
    Status      string `json:"status"`
}

type RetireSkillInput struct {
    SkillID string `json:"skill_id" jsonschema:"description=Skill ID"`
    Reason  string `json:"reason" jsonschema:"description=退役原因"`
}
type RetireSkillOutput struct {
    Success          bool     `json:"success"`
    DependentAgents  []string `json:"dependent_agents"`
}

type RecommendSkillsInput struct {
    TaskDescription string `json:"task_description" jsonschema:"description=任务描述"`
    TopK            int    `json:"top_k" jsonschema:"description=返回数量，默认5"`
}
type RecommendSkillsOutput struct {
    Recommendations []SkillRecommendation `json:"recommendations"`
}
type SkillRecommendation struct {
    SkillID    string  `json:"skill_id"`
    Name       string  `json:"name"`
    Score      float64 `json:"score"`
}

type AnalyzeToolWeightsInput struct {
    AgentID string `json:"agent_id" jsonschema:"description=Agent ID，为空则分析全局"`
}
type AnalyzeToolWeightsOutput struct {
    Tools []ToolWeightReport `json:"tools"`
}

type AnalyzeOrchestrationInput struct {
    TimeRange  string `json:"time_range" jsonschema:"description=时间范围:7d/30d/90d"`
    ModeFilter string `json:"mode_filter" jsonschema:"description=模式过滤，为空则全部"`
}
type AnalyzeOrchestrationOutput struct {
    Modes []OrchestrationModeReport `json:"modes"`
}
type OrchestrationModeReport struct {
    Mode               string             `json:"mode"`
    SuccessRate        float64            `json:"success_rate"`
    AvgTokens          int                `json:"avg_tokens"`
    AvgDurationSec     int                `json:"avg_duration_sec"`
    MemberContributions map[string]float64 `json:"member_contributions"`
    DQScore            float64            `json:"dq_score"`
}

type OptimizeOrchestrationInput struct {
    TimeRange string `json:"time_range"`
}
type OptimizeOrchestrationOutput struct {
    Suggestions []OrchestrationSuggestion `json:"suggestions"`
}
type OrchestrationSuggestion struct {
    Type        string  `json:"type"`
    Description string  `json:"description"`
    Confidence  float64 `json:"confidence"`
}
```

### 9.10 `retire_skill` 的通知机制

**问题**：文档 §4.6 说"通知依赖此 Skill 的 Agent 的所有者"，但未说明通知方式。

**决策**：通过现有 `event.Bus` 发布 `skill.retired` 事件，前端通过 WebSocket 接收通知。

```go
// 发布事件
t.deps.EventBus.Publish(ctx, event.Envelope{
    Type:    "skill.retired",
    Payload: json.Marshal(map[string]string{"skill_id": skillID, "reason": reason}),
})

// 前端处理
// conversationEventDispatcher.ts 中新增 skill.retired 类型处理
// 显示 toast 通知："Skill {name} 已退役：{reason}"
```

### 9.11 Prompt 文件核心指令要点

**记忆管家（memory.md）核心指令**：

```
你是 Aranea 系统的记忆管家，负责管理 Agent 的记忆系统。

## 核心原则
- 选择性记忆：不是所有经验都值得记忆，只记忆"没想到"的知识
- 质量驱动遗忘：主动删除低质量记忆比保留更有益
- 错误传播防护：错误记忆会通过经验跟随效应传播，必须及时清除

## 工作流程
1. 用 analyze_memory_quality 评估记忆健康度
2. 如果健康度低于 0.6，执行 dream_cycle
3. dream_cycle 会依次：删除 misaligned 记忆 → 遗忘不活跃记忆 → 去重 → 蒸馏

## 约束
- dream_cycle 默认 dry_run=true，需用户确认后执行
- 遗忘策略为 hybrid（priority_decay + lru + reflection_summary）
- 记忆条数上限 1000，超过时优先遗忘不活跃的
```

**技能管家（skills.md）核心指令**：

```
你是 Aranea 系统的技能管家，负责 Skill 的进化/消亡和工具权重优化。

## 核心原则
- 数据驱动：所有决策基于历史使用数据
- 编排进化：编排策略应基于历史数据动态调整
- 成本感知：考虑成本效率，而非仅追求最高性能

## 工作流程
1. 用 analyze_skill_health 评估 Skill 健康度
2. 用 analyze_tool_weights 评估工具权重
3. 用 analyze_orchestration 分析编排效率
4. 基于分析结果：evolve_skill / retire_skill / optimize_orchestration

## 约束
- evolve_skill 创建的新版本需用户确认后启用
- retire_skill 前必须检查依赖
- 编排建议不自动执行，需用户确认
```

---

## 十、代码验证勘误与补充（第二轮）

> 本节基于代码库交叉验证，修正 §1~§9 中与实际 API 不符的内容，补充缺失的实现细节。

### 10.1 `MemoryAdminUsecase` 无 `DeleteFact`/`ListFacts` 方法——记忆删除路径

**§3.3 和 §3.5 引用的 `MemoryAdminUsecase.DeleteFact` 不存在**。§9.3 引用的 `MemoryAdmin.ListFacts(ctx, agentID)` 也不存在。

**实际 API**：

```go
// 记忆查询：ListFactRows（非 ListFacts）
func (uc *MemoryAdminUsecase) ListFactRows(
    ctx context.Context,
    scopeType, scopeID, kind, status, keyword string,
    limit, offset int32,
) ([][]byte, int32, int32, int32, error)

// 记忆写入：UpsertFactRow（非 CreateFact）
func (uc *MemoryAdminUsecase) UpsertFactRow(ctx context.Context, in FactUpsert) ([]byte, error)

// 记忆删除：无直接方法，需通过底层 SessionAdminStore
```

**记忆删除的正确路径**：

`MemoryAdminUsecase` 组合了 `SessionAdminStore`，后者组合了 `L3FactAdminStore`。但 `L3FactAdminStore` 也没有 `DeleteFact` 方法。

**新增删除能力的方案**：

在 `L3FactAdminStore` 接口中新增 `DeleteFactRow` 方法：

```go
// internal/biz/memory_admin_store.go L3FactAdminStore 新增
type L3FactAdminStore interface {
    // ... 现有方法
    DeleteFactRow(ctx context.Context, factID string) error              // 新增
    DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error)  // 新增：批量删除
}
```

Data 层实现通过 Ent ORM 删除 `memory_facts` 表记录，并同步调用 `MemoryFactIndexSyncer` 清理向量索引。

**`MemoryAdminUsecase` 新增代理方法**：

```go
func (uc *MemoryAdminUsecase) DeleteFactRow(ctx context.Context, factID string) error {
    return uc.admin.DeleteFactRow(ctx, factID)
}

func (uc *MemoryAdminUsecase) DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error) {
    return uc.admin.DeleteFactRowsByIDs(ctx, factIDs)
}
```

**`forget_low_quality` 工具修正**：

```go
func (t *forgetLowQualityTool) detectMisaligned(ctx context.Context, agentID string) ([]string, error) {
    // 1. 用 ListFactRows 查询该 Agent 的所有事实
    rows, _, _, _, err := t.deps.MemoryAdmin.ListFactRows(ctx, "agent", agentID, "", "", "", 1000, 0)
    if err != nil { return nil, err }

    // 2. 解析每条事实，检查检索后负反馈率
    var misaligned []string
    for _, raw := range rows {
        var fact struct {
            ID       string `json:"id"`
            Statement string `json:"statement"`
        }
        json.Unmarshal(raw, &fact)

        retrievalCount := t.getRetrievalCount(ctx, fact.ID)
        negativeFeedbackCount := t.getNegativeFeedbackAfterRetrieval(ctx, fact.ID)
        if retrievalCount >= 3 && float64(negativeFeedbackCount)/float64(retrievalCount) > 0.5 {
            misaligned = append(misaligned, fact.ID)
        }
    }
    return misaligned, nil
}

func (t *forgetLowQualityTool) deleteFacts(ctx context.Context, factIDs []string) (int, error) {
    return t.deps.MemoryAdmin.DeleteFactRowsByIDs(ctx, factIDs)
}
```

### 10.2 `evolve_skill` LLM 调用方式修正

**§9.6 使用 `t.deps.Provider.TRPCModelForProviderModel(...)` 有误**。实际是包级函数：

```go
// internal/provider/trpc_llm.go
func TRPCModelForProviderModel(
    ctx context.Context,
    catalog *biz.LlmProviderModelUsecase,
    rt *provider.RoundTrip,
    prov, modelAPI string,
) (trpcmodel.Model, error)
```

**修正后的 `evolve_skill` LLM 调用**：

```go
// 工具 Deps 中需要注入完整的 LLM 调用依赖
type Deps struct {
    // ... 现有
    ProviderCatalog *biz.LlmProviderModelUsecase
    RoundTrip       *provider.RoundTrip
    ProviderCode    string
    ModelAPIID      string
}

func (t *evolveSkillTool) callLLM(ctx context.Context, prompt string) (string, error) {
    model, err := provider.TRPCModelForProviderModel(
        ctx, t.deps.ProviderCatalog, t.deps.RoundTrip,
        t.deps.ProviderCode, t.deps.ModelAPIID,
    )
    if err != nil { return "", err }

    resp, err := model.Generate(ctx, []*trpcmodel.Content{
        {Role: "user", Parts: []trpcmodel.Part{{Text: prompt}}},
    })
    if err != nil { return "", err }
    return resp.Text(), nil
}
```

**Deps 注入路径**（在 `system_builtin_tools.go` 中）：

```go
func (o *ChatOrchestrator) skillsButlerTools(ctx context.Context, ag biz.Agent) []trpctool.Tool {
    if ag.AgentKey != "__skills__" { return nil }
    return skills_butler.RegisterAll(skills_butler.Deps{
        // ... 其他依赖
        ProviderCatalog: o.td.Catalog,
        RoundTrip:       o.td.RT,
        ProviderCode:    ag.ProviderCode,
        ModelAPIID:      ag.ModelAPIID,
    })
}
```

### 10.3 `tools.Assemble()` 排序支持——需新增排序机制

**§9.7 说"按权重降序排列工具"，但 `Assemble()` 没有排序逻辑**。当前 `Assemble()` 按 Registry 注册顺序 + 特殊处理顺序组装。

**新增排序支持的方案**：

在 `AssemblyConfig` 中新增 `ToolWeightOverrides` 字段，在 `Assemble()` 返回前对 `Tools` 切片排序：

```go
// internal/tools/toolset.go AssemblyConfig 新增字段
type AssemblyConfig struct {
    // ... 现有字段
    ToolWeightOverrides map[string]float64  // tool_key -> weight_score
}

// Assemble() 末尾新增排序逻辑
func Assemble(ctx context.Context, cfg AssemblyConfig) (*AssembledToolsets, error) {
    // ... 现有组装逻辑

    // 新增：按权重排序
    if len(cfg.ToolWeightOverrides) > 0 {
        sort.SliceStable(out.Tools, func(i, j int) bool {
            wi := cfg.ToolWeightOverrides[out.Tools[i].Name()]
            wj := cfg.ToolWeightOverrides[out.Tools[j].Name()]
            return wi > wj  // 降序
        })
    }

    return out, nil
}
```

**低权重工具降级**：权重 < 0.3 的工具标记 `SkipSummarization`（减少 prompt 占用但不移除）。

### 10.4 `ExperienceAnalyticsUsecase` 依赖修正与 Wire 绑定

**§9.1 的构造函数中 `usage.AnalyticsRepo` 跨包依赖**。`usage.AnalyticsRepo` 定义在 `internal/biz/usage/` 子包中，Wire 绑定需注意。

**修正后的构造函数**：

```go
func NewExperienceAnalyticsUsecase(
    toolInvRepo  biz.EvolutionMetricsRepo,
    skillRepo    biz.SkillQueryReader,
    teamRunRepo  biz.TeamRepository,
    usageRepo    usage.AnalyticsRepo,       // 跨包：internal/biz/usage
    memoryAdmin  *biz.MemoryAdminUsecase,
    sessionRepo  biz.SessionReader,         // 修正：用 SessionReader 而非 SessionRepository
    toolInvData  biz.ToolInvocationReader,  // 新增：工具调用明细查询
) *ExperienceAnalyticsUsecase
```

**新增 `ToolInvocationReader` 接口**：

`EvolutionMetricsRepo` 只有 4 个聚合方法，无法提供工具调用明细。需新增：

```go
// internal/biz/evolution.go 新增接口
type ToolInvocationReader interface {
    ListToolInvocations(ctx context.Context, q ToolInvocationQuery) ([]ToolInvocationSummary, error)
}

type ToolInvocationQuery struct {
    AgentID  string
    ToolKey  string
    Since    time.Time
    Limit    int
}

type ToolInvocationSummary struct {
    ToolKey       string
    CallCount     int
    SuccessCount  int
    FailureCount  int
    AvgDurationMS float64
}
```

**Wire 绑定**（`internal/data/data.go`）：

```go
var ProviderSet = wire.NewSet(
    // ... 现有
    wire.Bind(new(biz.ToolInvocationReader), new(*ToolInvocationData)),  // 新增
    NewToolInvocationData,  // 新增
)
```

### 10.5 `SkillQueryReader.SearchSkillInvocations` 查询路径

**§2.2 引用 `skill_invocations` 数据但未指定查询方式**。实际应走 `SkillQueryReader`：

```go
// internal/biz/skill/skill.go
type SkillQueryReader interface {
    // ... 现有方法
    SearchSkillInvocations(ctx context.Context, q RunQuery) (RunResult, error)
}
```

**`RunQuery` 结构**：

```go
type RunQuery struct {
    SkillID    string
    AgentID    string
    Status     string
    Since      string
    Limit      int
    Offset     int
}
```

**`ExperienceAnalyticsUsecase.AnalyzeSkillHealth` 修正**：

```go
func (uc *ExperienceAnalyticsUsecase) AnalyzeSkillHealth(ctx context.Context) ([]SkillHealth, error) {
    // 使用 SkillQueryReader.SearchSkillInvocations 查询调用记录
    result, err := uc.skillRepo.SearchSkillInvocations(ctx, skill.RunQuery{
        Since: time.Now().AddDate(0, 0, -7).Format(time.RFC3339),
        Limit: 1000,
    })
    // ... 聚合分析
}
```

### 10.6 `evolve_skill` 和 `consolidate_episodes` 的 LLM Prompt 定义

**§4.5 和 §3.6 缺少 LLM 调用的 prompt 模板**。

**`evolve_skill` 失败分析 prompt**：

```
你是一个技能优化专家。请分析以下 Skill 的失败模式，并生成优化方案。

## Skill 信息
- 名称：{skill_name}
- 当前 body：{skill_body}
- 描述：{skill_description}

## 失败案例
{failure_cases}

## 分析要求
1. 识别失败的根本原因（prompt 不精确？参数缺失？边界条件？）
2. 生成优化后的 body
3. 说明修改了什么以及为什么

## 输出格式
```json
{
  "failure_analysis": "失败原因分析",
  "optimized_body": "优化后的 body",
  "changes": ["修改1", "修改2"],
  "confidence": 0.8
}
```
```

**`consolidate_episodes` 蒸馏 prompt**：

```
你是一个知识蒸馏专家。请将以下碎片化的情景记忆蒸馏为结构化的语义知识。

## 情景记忆
{episode_list}

## 蒸馏要求
1. 提取共性和规律
2. 生成简洁的语义知识陈述
3. 保留关键细节，去除冗余
4. 每条知识不超过 100 字

## 输出格式
```json
{
  "distilled_facts": [
    {"statement": "知识陈述", "source_episodes": ["ep1", "ep2"]},
    ...
  ]
}
```
```

### 10.7 Cron 任务注册流程

**§6.4 和 §9.4 定义了 cron 任务但未说明注册方式**。

**Cron 任务结构**（`biz.CronTask`）：

```go
type CronTask struct {
    ID           string
    TaskKey      string
    Name         string
    Description  string
    Status       string
    Enabled      bool
    SortOrder    int
    AgentID      string       // 关联的 Agent ID
    ConfigJSON   string       // 调度配置
    MetadataJSON string
}
```

**`ConfigJSON` 结构**：

```json
{
  "schedule": "0 3 * * *",
  "message": "请执行 dream_cycle，整理记忆系统",
  "type": "agent"
}
```

**注册方式**：在种子数据 `seed_system_agents.go` 中创建 CronTask：

```go
func seedCronTasks(ctx context.Context, cronUC *biz.CronUsecase, agents biz.AgentRepository) error {
    memoryAgent, _ := agents.GetAgentByAgentKey(ctx, "__memory__")
    skillsAgent, _ := agents.GetAgentByAgentKey(ctx, "__skills__")

    tasks := []biz.CronTask{
        {
            TaskKey:     "dream_cycle",
            Name:        "记忆管家做梦",
            Description: "每天凌晨3点触发记忆整理",
            Enabled:     true,
            AgentID:     memoryAgent.ID,
            ConfigJSON:  `{"schedule":"0 3 * * *","message":"请执行 dream_cycle，整理记忆系统","type":"agent"}`,
        },
        {
            TaskKey:     "skill_health_scan",
            Name:        "Skill 健康度扫描",
            Description: "每周一凌晨4点扫描 Skill 健康度",
            Enabled:     true,
            AgentID:     skillsAgent.ID,
            ConfigJSON:  `{"schedule":"0 4 * * 1","message":"请分析所有 Skill 的健康度","type":"agent"}`,
        },
    }

    for _, t := range tasks {
        cronUC.CreateTask(ctx, t)
    }
    return nil
}
```

**cronrunner 执行路径**：`cronrunner.Runner.dispatchCronTask` 识别 `type=agent`，创建 Agent Session，调用 `Chat.RunCronTurn(sessionID, message, "")`。

### 10.8 `ForgetConfig` 存储位置——`agent_runtime_settings` 字段扩展

**§9.5 说存储在 `agent_runtime_settings` 中，但未指定字段**。

当前 `agent_runtime_settings` 表有 60+ 字段，已有 `skill_runtime_json` 字段作为 JSON 存储的先例。

**方案**：新增 `forget_policy_json` 字段到 Ent Schema：

```go
// internal/data/ent/schema/agent_runtime_setting.go 新增
field.String("forget_policy_json").Default("{}"),
```

**Biz 层 `AgentRuntimeSettings` 新增**：

```go
type AgentRuntimeSettings struct {
    // ... 现有 60+ 字段
    ForgetPolicyJSON string `json:"forget_policy_json,omitempty"`  // 新增
}
```

**种子数据中设置默认值**：

```go
// __memory__ Agent 的 runtime_settings
ForgetConfigJSON: `{
    "policy": "hybrid",
    "max_memory_count": 1000,
    "max_memory_age_days": 90,
    "inactive_threshold_days": 30,
    "misaligned_input_sim_threshold": 0.8,
    "misaligned_output_sim_threshold": 0.5,
    "prediction_error_threshold": 0.3,
    "dedup_sim_threshold": 0.95
}`,
```

### 10.9 `retire_skill` 事件类型——需新增 `EnvelopeType`

**§9.10 使用 `skill.retired` 事件类型，但不在现有 `EnvelopeType` 枚举中**。

**现有相关类型**：`EnvelopeTypeAlertNotify`（`"alert.notify"`）已存在，可用于通知。

**方案**：复用 `EnvelopeTypeAlertNotify` 而非新增类型：

```go
t.deps.EventBus.Publish(ctx, event.Envelope{
    Type: event.EnvelopeTypeAlertNotify,
    Payload: json.Marshal(map[string]string{
        "alert_type": "skill_retired",
        "skill_id":   skillID,
        "reason":     reason,
        "message":    fmt.Sprintf("Skill %s 已退役：%s", skillName, reason),
    }),
})
```

**前端处理**：`conversationEventDispatcher.ts` 已处理 `alert.notify` 类型，显示 toast 通知。

### 10.10 `SkillUsecase` 方法名映射

**§4.3~§4.6 引用的方法名与实际不完全一致**：

| 文档引用 | 实际方法 | 说明 |
|---------|---------|------|
| `SkillUsecase.PatchSkill` | `SkillUsecase.Patch(ctx, id, patch UpdateDraft)` | 方法名是 `Patch`，内部调 `repo.PatchSkill` |
| `SkillUsecase.CreateSkillWithVersion` | `SkillUsecase.Create(ctx, in CreateInput)` | 方法名是 `Create`，内部调 `repo.CreateSkillWithVersion` |
| `SkillUsecase.UpdateSkillEnabled` | `SkillUsecase.ToggleEnabled(ctx, id, enabled bool)` | 方法名是 `ToggleEnabled`，内部调 `repo.UpdateSkillEnabled` |
| `SkillUsecase.ScoreByEmbedding` | `SkillUsecase.ScoreByEmbedding(ctx, query, candidates)` | ✅ 一致 |

**`evolve_skill` 工具修正**：

```go
// 创建新版本
newSkill, err := t.deps.SkillUC.Create(ctx, skill.CreateInput{
    SkillKey:    existingSkill.SkillKey + "_v2",
    Name:        existingSkill.Name + " (优化版)",
    Body:        optimizedBody,
    // ...
})

// 标记为 pending review
t.deps.SkillUC.ToggleEnabled(ctx, newSkill.ID, false)  // 新版本默认禁用
```

**`retire_skill` 工具修正**：

```go
t.deps.SkillUC.ToggleEnabled(ctx, input.SkillID, false)  // 禁用而非删除
```

### 10.11 `EvolutionMetricsRepo` 能力边界与补充

**§2.2 说 `EvolutionMetricsRepo` 提供"工具调用统计"，但实际只有 4 个聚合方法**：

```go
type EvolutionMetricsRepo interface {
    GetToolSuccessRate(ctx, agentID, since) (float64, []MetricDataPoint, error)
    GetRetrievalQuality(ctx, agentID, since) (float64, []MetricDataPoint, error)
    GetEpisodeCount(ctx, agentID, since) (int, error)
    GetNegativeFeedbackCount(ctx, agentID, since) (int, error)
}
```

**无法提供**：工具调用频率、失败原因分布、耗时分布、按工具 key 分组的统计。

**补充方案**：新增 `ToolInvocationReader` 接口（见 §10.4），从 `tool_invocations` 表查询明细数据。

`tool_invocations` 表关键字段：

```go
// internal/data/ent/schema/tool_invocation.go
field.String("tool_key")           // 工具标识
field.String("agent_id")          // Agent ID
field.String("session_id")        // Session ID
field.String("status")            // success/failure
field.Int("duration_ms")          // 耗时
field.Text("input_preview")       // 输入预览
field.Text("output_preview")      // 输出预览
field.String("error_message")     // 错误信息
```

**`AnalyzeToolWeights` 修正**：

```go
func (uc *ExperienceAnalyticsUsecase) AnalyzeToolWeights(ctx context.Context) ([]ToolWeightReport, error) {
    // 1. 从 EvolutionMetricsRepo 获取整体成功率
    successRate, _, _ := uc.toolInvRepo.GetToolSuccessRate(ctx, "", time.Now().AddDate(0, 0, -30))

    // 2. 从 ToolInvocationReader 获取按工具分组的明细
    invocations, _ := uc.toolInvData.ListToolInvocations(ctx, biz.ToolInvocationQuery{
        Since: time.Now().AddDate(0, 0, -30),
        Limit: 10000,
    })

    // 3. 聚合计算权重
    // ...
}
```

### 10.12 `HealthScore` 和 `DQ Score` 告警阈值体系

**§9.8 只定义了 HealthScore < 0.6 触发 dream_cycle，缺少完整阈值体系**。

**记忆健康度阈值**：

| HealthScore 范围 | 状态 | 动作 |
|-----------------|------|------|
| 0.8~1.0 | 健康 | 无需操作 |
| 0.6~0.8 | 亚健康 | 记忆管家提示用户"建议整理记忆" |
| 0.4~0.6 | 不健康 | 自动触发 dream_cycle（dry_run=true） |
| < 0.4 | 危险 | 自动触发 dream_cycle（dry_run=false），发送告警通知 |

**编排质量阈值**：

| DQ Score 范围 | 状态 | 动作 |
|--------------|------|------|
| 0.7~1.0 | 优秀 | 缓存当前编排拓扑 |
| 0.5~0.7 | 合格 | 记录但无需操作 |
| 0.3~0.5 | 较差 | 技能管家生成优化建议 |
| < 0.3 | 失败 | 标记编排拓扑为"避免使用" |

### 10.13 dream_cycle 误删回滚策略

**§8 风险表提到"默认 dry_run=true"但未定义误删后的回滚**。

**方案**：

1. **dream_cycle 执行前快照**：在执行任何删除操作前，将待删除的记忆导出为 JSON 快照，存储在 `agent_runtime_settings` 的 `dream_snapshot_json` 字段中。

2. **7 天回滚窗口**：快照保留 7 天，用户可要求记忆管家"撤销上次整理"。

3. **回滚操作**：从快照中恢复被删除的记忆，调用 `MemoryAdminUsecase.UpsertFactRow` 重新写入。

```go
type DreamSnapshot struct {
    ExecutedAt   string         `json:"executed_at"`
    DeletedFacts []FactSnapshot `json:"deleted_facts"`
    MergedFacts  []FactSnapshot `json:"merged_facts"`
}

type FactSnapshot struct {
    ID        string `json:"id"`
    Statement string `json:"statement"`
    ScopeType string `json:"scope_type"`
    ScopeID   string `json:"scope_id"`
    Kind      string `json:"kind"`
}
```

### 10.14 记忆管家和技能管家工具 Deps 修正版

综合以上修正，完整 Deps 定义：

**记忆管家工具 Deps**：

```go
// internal/tools/memory_butler/registry.go
type Deps struct {
    Analytics     *biz.ExperienceAnalyticsUsecase
    MemoryAdmin   *biz.MemoryAdminUsecase
    Embedder      biz.SkillEmbedder              // 新增：用于 embedding 计算
    ProviderCatalog *biz.LlmProviderModelUsecase  // 新增：P1 阶段预测误差用
    RoundTrip     *provider.RoundTrip             // 新增：P1 阶段预测误差用
    ProviderCode  string                          // 新增
    ModelAPIID    string                          // 新增
    EventBus      event.Bus
}
```

**技能管家工具 Deps**：

```go
// internal/tools/skills_butler/registry.go
type Deps struct {
    Analytics       *biz.ExperienceAnalyticsUsecase
    SkillUC         *biz.SkillUsecase
    ProviderCatalog *biz.LlmProviderModelUsecase  // 修正：原 Provider
    RoundTrip       *provider.RoundTrip           // 新增
    ProviderCode    string
    ModelAPIID      string
    EventBus        event.Bus
}
```

### 10.15 `ExperienceAnalyticsUsecase` 完整构造函数与 Wire 绑定

**修正后的构造函数**：

```go
func NewExperienceAnalyticsUsecase(
    toolInvRepo  biz.EvolutionMetricsRepo,
    skillRepo    biz.SkillQueryReader,
    teamRunRepo  biz.TeamRepository,
    usageRepo    usage.AnalyticsRepo,
    memoryAdmin  *biz.MemoryAdminUsecase,
    sessionRepo  biz.SessionReader,
    toolInvData  biz.ToolInvocationReader,
) *ExperienceAnalyticsUsecase {
    return &ExperienceAnalyticsUsecase{
        toolInvRepo: toolInvRepo,
        skillRepo:   skillRepo,
        teamRunRepo: teamRunRepo,
        usageRepo:   usageRepo,
        memoryAdmin: memoryAdmin,
        sessionRepo: sessionRepo,
        toolInvData: toolInvData,
    }
}
```

**Wire 绑定**：

```go
// internal/biz/biz.go
var ProviderSet = wire.NewSet(
    // ... 现有
    NewExperienceAnalyticsUsecase,
)

// internal/data/data.go
var ProviderSet = wire.NewSet(
    // ... 现有
    wire.Bind(new(biz.ToolInvocationReader), new(*ToolInvocationData)),
    NewToolInvocationData,
)
```

**`cmd/admin/wire.go` 修改**：

`provideChatServiceDeps` 新增参数 `expAnalytics *biz.ExperienceAnalyticsUsecase`。

### 10.16 `EnvelopeType` 现有类型与管家事件映射

**框架已定义的与管家相关的事件类型**：

| EnvelopeType | 值 | 管家场景 |
|-------------|-----|---------|
| `EnvelopeTypeOrchestrationAgentStatus` | `"orchestration_agent_status"` | 编排管家汇报 Agent 状态 |
| `EnvelopeTypeTeamRunStarted` | `"team_run_started"` | Team 开始执行 |
| `EnvelopeTypeTeamRunFinished` | `"team_run_finished"` | Team 执行完成 |
| `EnvelopeTypeTeamRunFailed` | `"team_run_failed"` | Team 执行失败 |
| `EnvelopeTypeMemberMessageStart` | `"member_message_start"` | 管家开始回复 |
| `EnvelopeTypeMemberDelta` | `"member_delta"` | 管家回复流 |
| `EnvelopeTypeMemberMessageDone` | `"member_message_done"` | 管家回复完成 |
| `EnvelopeTypeAlertNotify` | `"alert.notify"` | Skill 退役通知等 |
| `EnvelopeTypeUserFeedback` | `"user_feedback"` | 用户反馈（用于 DQ 计算） |

**无需新增事件类型**：管家操作的事件流完全复用现有 Team 编排事件类型。

---

## 十一、决策定稿（结合路线图验证）

> 本节记录经代码库验证 + 路线图（Phase 1~5）兼容性分析后的最终决策。详细分析见主文档 [2026-05-31-system-builtin-agents-design.md](./2026-05-31-system-builtin-agents-design.md) §十三。

### 11.1 决策 1：Agent 归属查询——`Ownership` 新字段

**影响**：`buildSpiritTeam` 查询管家列表时使用 `AgentListQuery.Ownership = "system_builtin"` 过滤，而非 `Kind` 字段。详见主文档 §13.1。

### 11.2 决策 2：记忆删除——`L3FactWriter` 子接口 + 保护锁

**影响**：`forget_low_quality` 和 `deduplicate_memories` 工具的删除操作走 `L3FactWriter.DeleteFactRow`/`DeleteFactRowsByIDs`，删除前检查 observations 引用。`MemoryAdminUsecase` 新增 `factWriter L3FactWriter` 字段。详见主文档 §13.2。

**§10.1 修正**：`MemoryAdminUsecase` 构造函数新增 `factWriter L3FactWriter` 参数：

```go
type MemoryAdminUsecase struct {
    admin     SessionAdminStore
    vec       *MemoryUsecase
    indexSync MemoryFactIndexSyncer
    factWriter L3FactWriter  // 新增
}

func NewMemoryAdminUsecase(admin SessionAdminStore, vec *MemoryUsecase, indexSync MemoryFactIndexSyncer, factWriter L3FactWriter) *MemoryAdminUsecase
```

### 11.3 决策 3：工具调用明细查询——复用现有 `biz.ToolInvocationReader` 接口

**影响**：`ExperienceAnalyticsUsecase` 构造函数新增 `toolReader biz.ToolInvocationReader` 参数。代码库验证发现 `internal/biz/tool/tool.go` 已存在 `ToolInvocationReader` 接口（含 `SearchToolInvocations` + `GetToolInvocationParams`），Data 层实现已存在，无需新增接口。详见主文档 §13.3。

**§10.4 修正**：删除原计划在 `evolution.go` 新增 `ToolInvocationReader`/`ToolInvocationQuery`/`ToolInvocationSummary` 的方案，改为直接注入现有 `biz.ToolInvocationReader`。

### 11.4 决策 4：工具权重——`agent_runtime_settings.tool_weight_json` + Prompt 策略

**影响**：`analyze_tool_weights` 工具的输出写入 `agent_runtime_settings.tool_weight_json`，而非修改 `AssemblyConfig`。`ChatOrchestrator` 在构建 `TRPCBuilderDeps` 时读取权重配置，过滤 disabled 工具 + 注入 Prompt 优先级提示。**不修改 `tools.Assemble()`**。详见主文档 §13.4。

**§10.3 修正**：删除 `AssemblyConfig.ToolWeightOverrides` 方案，改为 `agent_runtime_settings.tool_weight_json`。

### 11.5 决策 5：事件通知——复用 `EnvelopeTypeAlertNotify` + `severity` 分级

**影响**：`retire_skill` 通知使用 `EnvelopeTypeAlertNotify` + `alert_type="skill_retired"` + `severity="warning"`。不新增 `EnvelopeType`。详见主文档 §13.5。

**§10.9 修正**：增加 `severity` 分级字段。

### 11.6 决策汇总

| # | 决策点 | 最终方案 | 本文档影响章节 |
|---|--------|---------|-------------|
| 1 | Agent 归属查询 | `Ownership` 新字段 | §10.1 查询修正 |
| 2 | 记忆删除 | `L3FactWriter` 子接口 + 保护锁 | §10.1 删除路径 |
| 3 | 工具调用明细 | 复用现有 `biz.ToolInvocationReader`（无需新增接口） | §10.4/§10.11 |
| 4 | 工具权重 | `agent_runtime_settings.tool_weight_json` + Prompt 策略 | §10.3 替代方案 |
| 5 | 事件通知 | `EnvelopeTypeAlertNotify` + `alert_type` + `severity` | §10.9 增加 severity |

