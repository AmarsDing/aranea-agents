# Aranea Memory 综合优化方案

> **文档版本**：v1.0 · 2026-05-28
> **定位**：架构师视角，基于学术论文与行业实践，给出系统级优化方案
> **前置文档**：[memory-module-gap-analysis.md](./memory-module-gap-analysis.md)（差距分析）

---

## 零、原始设计愿景回顾

在 `docs/_deprecated/需求/notes.md` 中，Memory-Agent 的核心愿景是：

> 做一个内置的 agent 专注于管理记忆，独立线程随系统一起启动，后台可以配置开启和关闭，功能：实时监控历史对话，异步压缩与存储，管理 memory L0-L4 层记忆，形成一个统一的**神经记忆系统**。每个 memory 块语意完整，相互关联，当某个记忆块改动，**自动复盘相关联记忆块是否需要更新**，形成知识图谱。

这一愿景的核心洞察——**记忆块联动更新**——至今仍是行业前沿课题。下文将基于最新学术研究，给出实现这一愿景的技术路径。

---

## 一、理论基础：六篇关键论文的核心贡献

### 1.1 形式-功能-动态三维框架

**论文**：Hu et al., "Memory in the Age of AI Agents: A Survey" (arXiv:2512.13564, 2025)

**核心贡献**：建立了 Agent Memory 的统一分类框架，取代了过时的"短期/长期"二分法。

| 维度 | 含义 | Aranea 对应 |
|------|------|-------------|
| **Forms（形式）**：记忆"存在哪里" | Token 级（上下文窗口）、参数化（模型权重）、隐空间（向量） | L1 = Token 级；L2/L3 = 隐空间；L4 = Token 级结构化 |
| **Functions（功能）**：记忆"用来做什么" | 事实记忆、经验记忆、工作记忆 | L3 = 事实记忆；L2 = 经验记忆；L1 = 工作记忆 |
| **Dynamics（动态）**：记忆"如何随时间变化" | 形成、演化（巩固/更新/遗忘）、检索 | 衰减 Worker = 遗忘；AutoMemoryWorker = 形成；CompositeRecall = 检索 |

**关键发现**：论文指出，绝大多数现有系统在 **Dynamics 的"演化"环节严重缺失**——记忆只会被创建和衰减，不会合并、修正、关联更新。这正是 Aranea 原始愿景中"自动复盘相关联记忆块"要解决的核心问题。

**对 Aranea 的启示**：
- L0-L4 分层已覆盖 Forms 和 Functions 两个维度，但 Dynamics 维度仅有"形成"和"遗忘"，**缺少"巩固"和"更新"两个关键环节**
- 需要将"记忆演化"从简单的衰减升级为包含巩固、合并、冲突解决、再巩固的完整生命周期

---

### 1.2 仿生记忆架构：六大脑认知机制

**论文**：Kerestecioglu et al., "Human-Inspired Memory Architecture for LLM Agents" (arXiv:2605.08538, Microsoft, 2026)

**核心贡献**：将人类神经科学的六大认知机制映射为工程组件，每个机制解决一种"朴素记忆积累"的失败模式。

| 认知机制 | 神经科学原理 | 工程实现 | 解决的失败模式 | Aranea 现状 |
|----------|-------------|----------|---------------|-------------|
| **睡眠期巩固** | 海马体→新皮层记忆转移 | 离线去重+合并 | 记忆膨胀、冗余 | ❌ 仅有简单衰减 |
| **干扰性遗忘** | 前摄/后摄干扰 | 语义冲突检测+选择性遗忘 | 过时信息干扰 | ❌ 固定衰减，不区分干扰 |
| **印迹成熟** | 反复激活→突触强化 | 重复召回→importance 递增 | 重要信息被遗忘 | ❌ 召回不刷新 importance |
| **提取再巩固** | 回忆时记忆可被修改 | 召回时检测冲突→更新 | 记忆过时不更新 | ❌ 召回是只读操作 |
| **实体知识图谱** | 概念网络 | Entity KG + 关系遍历 | 孤立事实无法推理 | ✅ L4 已有，但与 L3 断裂 |
| **混合多线索检索** | 多感官线索整合 | semantic + BM25 + graph | 单一检索遗漏 | ✅ 6 维评分已有 |

**关键实验结果**：
- 去重巩固实现 **97.2% 保留精度 + 58% 存储缩减**（+21.8pp over baseline）
- 在 LongMemEval S-tier 上，去重巩固带来 **+13.3pp 偏好召回提升**
- 200K token 预算下匹配原始检索精度（70.1% vs 71.2%），但存储量可调

**对 Aranea 的启示**：六大机制中 Aranea 仅实现了"实体知识图谱"和"混合多线索检索"，其余四个（睡眠巩固、干扰遗忘、印迹成熟、提取再巩固）均缺失。这四个恰恰是实现"记忆块联动更新"的关键机制。

---

### 1.3 A-Mem：Zettelkasten 式动态记忆网络

**论文**：Xu et al., "A-Mem: Agentic Memory for LLM Agents" (arXiv:2502.12110, NeurIPS 2025)

**核心贡献**：将 Zettelkasten（卡片盒笔记法）应用于 Agent 记忆，实现记忆的**自组织、自链接、自进化**。

**三大核心操作**：

1. **笔记构建**：每条记忆不是简单文本，而是结构化笔记 `(content, context, keywords, tags, embedding, links)`
2. **链接生成**：新记忆写入时，自动寻找与已有记忆的语义关联并建立双向链接
3. **记忆进化**：新记忆写入后，LLM 自动更新相关旧记忆的 context 和 links

**关键实验结果**：
- 每次记忆操作仅需 ~1,200 tokens，**85-93% token 缩减**
- 多跳推理任务性能达基线 **2 倍以上**

**对 Aranea 的启示**：
- A-Mem 的"记忆进化"操作正是原始愿景中"自动复盘相关联记忆块"的学术实现
- L3 fact 目前是孤立的，缺少 `links` 字段和自动链接生成机制
- L4 graph 虽有关系，但 L3 fact 之间没有显式关联

---

### 1.4 MARS：反思性自进化 + 艾宾浩斯遗忘曲线

**论文**：Liang et al., "MARS: Memory-Enhanced Agents with Reflective Self-improvement" (arXiv:2503.19271, 2025)

**核心贡献**：将艾宾浩斯遗忘曲线引入 Agent 记忆衰减，并通过三角色（User/Assistant/Checker）反思机制实现记忆自优化。

**关键机制**：
- **动态衰减**：遗忘速率不是固定值，而是基于记忆的"被访问频率"和"重要性"动态调整
- **反思闭环**：Checker 角色评估 Assistant 的输出质量，反馈驱动记忆策略更新
- **进化目标**：每次反思后生成新的进化目标 `(Ability, Direction)`，驱动 Agent 持续改进

**关键实验结果**：
- 闭源模型性能提升 **2.26×**
- 开源模型提升 **57.7%–100%**，小模型效果更显著

**对 Aranea 的启示**：
- 固定衰减因子（0.92/0.95/0.97）不符合认知科学规律，应改为基于访问频率的动态衰减
- 缺少"反思"环节——记忆被写入后，没有机制评估其质量和有效性

---

### 1.5 ROMEM：连续相位旋转时间知识图谱

**论文**：Li et al., "Time is Not a Label: Continuous Phase Rotation for Temporal Knowledge Graphs and Agentic Memory" (arXiv:2604.11544, 2026)

**核心贡献**：将时间从"离散元数据标签"升级为"连续几何算子"，通过复向量空间中的相位旋转解决"静态-动态困境"。

**核心创新**：
- **Semantic Speed Gate**：预训练模块，从关系的文本 embedding 预测其"波动性" `α_r ∈ (0,1)`
  - 静态关系（如"出生在"）→ `α_r ≈ 0`，不旋转
  - 动态关系（如"工作于"）→ `α_r ≈ 1`，快速旋转
- **几何遮蔽**：过时事实在复向量空间中被旋转出相位，自然被检索排序淘汰，无需删除
- **纯追加写入**：记忆只增不改，但 LLM 接收到的上下文始终是时间正确的

**关键实验结果**：
- 时序知识图谱补全 ICEWS0515 **72.6 MRR**（SOTA）
- 时序推理（MultiTQ）**2-3× MRR 和准确率提升**
- 混合基准（LoCoMo）**全面领先**
- 静态记忆**零退化**

**对 Aranea 的启示**：
- L3/L4 的时间建模应从"时间戳标签"升级为"语义感知的连续时间算子"
- 不同类型的事实应有不同的时间衰减速率——"出生地"不应衰减，"当前工作"应快速更新
- "纯追加写入 + 几何遮蔽"比"覆盖旧值"更安全，保留历史推理能力

---

### 1.6 TSM：超越对话时间的语义时间记忆

**论文**：Su et al., "Beyond Dialogue Time: Temporal Semantic Memory for Personalized LLM Agents" (arXiv:2601.07468, 2026)

**核心贡献**：区分"对话时间"和"语义时间"，支持**持续性记忆（durative memory）**的构建与检索。

**核心洞察**：
- 现有系统按"对话时间"组织记忆，但用户说"我上周换了工作"中的"上周"是语义时间，不是对话时间
- 持续性记忆（如"2023-2025 在北京工作"）比点状记忆（如"2024-03-15 提到在北京工作"）更有价值

**关键机制**：
- **语义时间线**：从对话中提取事实的实际发生时间，而非对话时间
- **持续性记忆构建**：将时间连续且语义相关的点状记忆合并为持续性记忆
- **时间意图检索**：查询时识别用户的时间意图（"现在"vs"之前"），检索时间匹配的持续性记忆

**关键实验结果**：
- LongMemEval 和 LoCoMo 上**最高 12.2% 绝对准确率提升**

**对 Aranea 的启示**：
- L3 fact 的 `created_at` 是对话时间，不是语义时间
- 需要从对话中提取事实的实际发生时间，构建语义时间线
- 支持续性记忆（如"2023.3-2025.5 住在北京"）而非仅点状事实

---

## 二、综合优化方案：神经记忆系统

基于上述六篇论文的核心发现，结合 Aranea 原始设计愿景，提出以下综合优化方案。方案以**"记忆块联动更新"**为核心目标，按认知科学原理重新设计记忆的完整生命周期。

### 2.1 总体架构：从五层存储到神经记忆系统

```
┌──────────────────────────────────────────────────────────────────────┐
│                    神经记忆系统 (Neural Memory System)                │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────────┐ │
│  │                    记忆生命周期引擎                              │ │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────────┐   │ │
│  │  │  形成    │→│  巩固    │→│  演化    │→│  检索/再巩固 │   │ │
│  │  │Formation │  │Consolid. │  │Evolution │  │Recall/Recons │   │ │
│  │  └──────────┘  └──────────┘  └──────────┘  └──────────────┘   │ │
│  └─────────────────────────────────────────────────────────────────┘ │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────────┐ │
│  │                    记忆存储层 (L0-L4)                            │ │
│  │  L0 快照 │ L1 工作记忆 │ L2 情景 │ L3 语义+时间 │ L4 图谱+时间 │ │
│  └─────────────────────────────────────────────────────────────────┘ │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────────┐ │
│  │                    Memory-Agent (后台守护)                       │ │
│  │  睡眠巩固 │ 干扰遗忘 │ 印迹成熟 │ 联动更新 │ 质量审计          │ │
│  └─────────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────────┘
```

### 2.2 优化项 A：时间感知记忆（Temporal Memory）

**论文依据**：ROMEM (arXiv:2604.11544)、TSM (arXiv:2601.07468)、Zep (arXiv:2501.13956)

**现状问题**：L3 fact 无时间线，L4 relation 无时间有效性，无法区分"出生在北京"和"目前住在北京"。

**优化方案**：

#### A1. L3 Fact 语义时间线

```
现有字段：  id, statement, scope_type, scope_id, importance, fingerprint, ...
新增字段：  semantic_time_start  TIMESTAMP    -- 事实实际发生的开始时间
            semantic_time_end    TIMESTAMP    -- 事实实际发生的结束时间 (NULL=持续中)
            dialogue_time        TIMESTAMP    -- 对话中提及的时间 (保留)
            temporal_confidence  FLOAT        -- 时间提取的置信度
            volatility_score     FLOAT        -- 关系波动性 (0=静态, 1=高度动态)
```

**提取流程**：
1. AutoMemoryWorker 提取 fact 时，LLM 同时提取语义时间（"上周"→具体日期）
2. 参考 TSM 的语义时间线构建，将点状事实合并为持续性记忆
3. 参考 ROMEM 的 Semantic Speed Gate，为每条 fact 计算 `volatility_score`

**检索流程**：
1. 查询时识别用户的时间意图（"现在"/"之前"/"去年"）
2. 优先返回 `semantic_time_end IS NULL`（当前有效）的事实
3. 时序推理时返回历史事实（`semantic_time_end IS NOT NULL`）
4. 向量检索时，`volatility_score` 高的事实如果 `semantic_time_end < now`，自动降权

#### A2. L4 Relation 双时态模型

```
现有字段：  id, source_entity_id, target_entity_id, relation_type, confidence, ...
新增字段：  valid_from       TIMESTAMP    -- 关系生效时间
            valid_until      TIMESTAMP    -- 关系失效时间 (NULL=持续有效)
            ingested_at      TIMESTAMP    -- 系统录入时间
            invalidation_reason  TEXT     -- 失效原因 (conflict/superseded/manual)
```

**冲突解决流程**（参考 Zep Graphiti）：
1. 新 relation 写入时，检查同 `(source, target, relation_type)` 的现有 relation
2. 如果语义冲突（如"住在北京"vs"住在纽约"），自动将旧 relation 的 `valid_until` 设为当前时间
3. 旧 relation 不删除，保留用于历史查询
4. 高风险变更（如身份变更）仍走级联提案审批

#### A3. 关系波动性自动分类

参考 ROMEM 的 Semantic Speed Gate，预训练一个轻量分类器：

| 关系类型 | 波动性 α | 衰减策略 |
|----------|---------|----------|
| 出生在、毕业于 | ≈ 0 | 不衰减，永久有效 |
| 偏好、习惯 | 0.2-0.4 | 慢衰减，长保留期 |
| 工作于、住在 | 0.6-0.8 | 中速衰减，冲突时自动更新 |
| 当前心情、今日计划 | ≈ 1.0 | 快衰减，短保留期 |

---

### 2.3 优化项 B：记忆联动更新（Linked Memory Evolution）

**论文依据**：A-Mem (arXiv:2502.12110)、Human-Inspired (arXiv:2605.08538)

**现状问题**：L3 fact 之间相互孤立，L4 级联提案仅在名字冲突时触发。当"工作地点从北京变为纽约"时，相关的"通勤方式""附近餐厅"等记忆不会自动更新。

**优化方案**：

#### B1. L3 Fact 关联链接

参考 A-Mem 的 Zettelkasten 链接机制：

```
新增表：memory_fact_links
  source_fact_id   UUID
  target_fact_id   UUID
  link_type        ENUM('semantic', 'temporal', 'causal', 'contradicts')
  link_strength    FLOAT    -- 0-1, 由 LLM 或 embedding 相似度决定
  created_at       TIMESTAMP
```

**链接生成流程**：
1. 新 fact 写入时，检索 top-K 语义相似的事实
2. LLM 判断关联类型和强度（语义关联/时间关联/因果关联/矛盾关联）
3. 建立双向链接

#### B2. 联动更新引擎

当一条 fact 被更新（如 importance 变更、时间线变更、内容修改）时：

```
联动更新流程：
1. 触发：fact F 被标记为"已变更"
2. 查询：SELECT * FROM memory_fact_links WHERE source_fact_id = F.id
3. 评估：对每条关联 fact F'，LLM 判断：
   - F 的变更是否影响 F' 的有效性？
   - F' 是否需要更新内容/importance/时间线？
4. 执行：
   - 需要更新 → 生成更新提案（低风险自动执行，高风险人工审批）
   - 需要失效 → 设置 F'.semantic_time_end = now
   - 无影响 → 跳过
5. 递归：对被更新的 F'，重复步骤 2-4（最大深度 3，防止无限递归）
6. 审计：记录联动更新链路到 memory_evolution_log
```

**这正是原始愿景"自动复盘相关联记忆块是否需要更新"的工程实现。**

#### B3. L3-L4 联合推理

```
联合检索流程：
1. L3 语义检索 → 返回 top-K facts
2. 从 facts 中提取实体 → L4 图遍历（扩展到 N-hop 邻居）
3. 图遍历结果中的新实体 → 反查 L3 中相关 facts
4. 合并去重，按综合评分排序
5. 交叉验证：L3 fact 与 L4 relation 的一致性检查
```

---

### 2.4 优化项 C：仿生记忆生命周期（Biological Memory Lifecycle）

**论文依据**：Human-Inspired (arXiv:2605.08538)、MARS (arXiv:2503.19271)、Memoria (arXiv:2310.03052)

**现状问题**：记忆生命周期仅有"写入→衰减→删除"，缺少"巩固→成熟→再巩固"环节。

**优化方案**：

#### C1. 睡眠期巩固（Sleep-Phase Consolidation）

新增 **MemoryConsolidationWorker**，在系统低峰期（如凌晨）执行：

| 巩固操作 | 触发条件 | 效果 |
|----------|----------|------|
| **去重合并** | 两条 fact 语义相似度 > 0.92 | 合并为一条，保留更完整的语义时间线 |
| **持续性记忆构建** | 多条点状 fact 属于同一语义时间线 | 合并为持续性记忆（如"2023-2025 住北京"） |
| **Episode→Fact 提炼** | L2 episode 的 importance < 阈值但包含未提取的事实 | 二次提取，补充到 L3 |
| **图谱实体消歧** | L4 中同一实体的多个别名 | 合并实体，更新所有关联 relation |
| **链接补全** | L3 fact 缺少关联链接 | 基于语义相似度补全 links |

**参考 Human-Inspired 论文的实验结果**：去重巩固可实现 **58% 存储缩减 + 97.2% 保留精度**。

#### C2. 干扰性遗忘（Interference-Based Forgetting）

替换当前的固定衰减因子，改为基于语义干扰的遗忘：

```
遗忘评分 = base_decay × (1 - access_frequency_boost) × interference_factor

其中：
- base_decay：由 volatility_score 决定的基础衰减率
  - 静态事实 (α≈0)：base_decay ≈ 1.0（几乎不衰减）
  - 动态事实 (α≈1)：base_decay ≈ 0.90（快速衰减）
- access_frequency_boost：每次被召回 +0.05，上限 0.3
  - 频繁被召回的记忆衰减更慢（印迹成熟）
- interference_factor：与同 scope 内更新事实的语义冲突度
  - 被新事实取代的旧事实 interference_factor ≈ 0.5（加速遗忘）
  - 无冲突 interference_factor = 1.0（正常衰减）
```

#### C3. 印迹成熟（Engram Maturation）

```
新增字段：memory_facts.access_count INT DEFAULT 0
          memory_facts.last_accessed_at TIMESTAMP

每次 fact 被召回时：
1. access_count += 1
2. last_accessed_at = now()
3. importance = min(1.0, importance + 0.02 × log(access_count + 1))
```

**效果**：频繁被召回的重要事实 importance 会逐渐升高，形成"记忆印迹"——这正是人类记忆中"反复激活→突触强化"的工程模拟。

#### C4. 提取再巩固（Reconsolidation upon Retrieval）

当 fact 被召回时，不是简单的只读操作，而是触发"再巩固"检查：

```
再巩固流程：
1. fact F 被召回
2. 检查 F.semantic_time_end 是否为 NULL
   - 如果不为 NULL（已过时），标记为"历史事实"，降权
3. 检查 F 与同 scope 内其他 fact 的冲突
   - 如果存在冲突，触发联动更新评估（优化项 B2）
4. 刷新 F.last_accessed_at 和 access_count（印迹成熟）
5. 如果 F.volatility_score > 0.5 且 F.semantic_time_start 距今 > 30 天
   → 标记为"待验证"，下次巩固时由 LLM 重新评估有效性
```

---

### 2.5 优化项 D：Memory-Agent 后台守护

**论文依据**：Letta Sleep-time Agent、MARS 反思机制、原始设计愿景

**现状问题**：记忆管理分散在多个 Cron Worker 中，没有统一的"记忆管家"角色。

**优化方案**：

#### D1. Memory-Agent 架构

```
Memory-Agent（独立 goroutine，随系统启动，可配置开关）
├── 实时监控层
│   ├── 监听 TurnMemoryWorker 事件 → 触发即时提取
│   ├── 监听用户反馈事件 → 触发偏好提取
│   └── 监听 fact 变更事件 → 触发联动更新评估
│
├── 睡眠巩固层（低峰期执行）
│   ├── 去重合并
│   ├── 持续性记忆构建
│   ├── Episode→Fact 二次提炼
│   ├── 图谱实体消歧
│   └── 链接补全
│
├── 质量审计层（定期执行）
│   ├── 评估记忆提取质量（抽样 LLM 评判）
│   ├── 评估检索相关性（用户反馈信号）
│   ├── 评估衰减策略有效性（importance 分布分析）
│   └── 生成进化建议（参考 MARS 的反思机制）
│
└── 策略自优化层
    ├── 根据审计结果调整提取 prompt
    ├── 根据召回频率调整衰减参数
    ├── 根据冲突模式调整冲突解决策略
    └── 输出进化事件到 Agent Strategy Profile
```

#### D2. 睡眠巩固调度

参考 Letta Sleep-time Agent 的设计，但适配 Aranea 的多租户架构：

```
调度策略：
- 触发条件：系统负载 < 阈值 且 距上次巩固 > 最小间隔
- 租户隔离：每个 tenant 独立巩固队列，按 tenant quota 排序
- 增量处理：每次巩固只处理自上次以来的增量数据
- 超时保护：单次巩固最长 5 分钟，超时则保存进度退出
- 失败重试：巩固失败的任务写入 dead-letter，下次重试
```

---

### 2.6 优化项 E：Agent 自主记忆编辑

**论文依据**：Letta (MemGPT)、A-Mem

**现状问题**：Agent 只能通过 L1 field 写入工作记忆，无法主动编辑长期记忆。

**优化方案**：

#### E1. Memory Tool 定义

为 Agent 提供以下 Memory Tool（通过 trpc-agent-go 的 Tool 机制注册）：

| Tool | 功能 | 权限控制 |
|------|------|----------|
| `memory_fact_add` | 主动添加一条 L3 fact | 需 agent 配置 `allow_memory_write: true` |
| `memory_fact_update` | 更新已有 L3 fact 的内容 | 需 agent 配置 + 语义冲突检测 |
| `memory_fact_delete` | 软删除一条 L3 fact | 需 agent 配置 + 人工审批 |
| `memory_fact_search` | 主动检索 L3 fact（非注入场景） | 默认允许 |
| `memory_entity_link` | 在 L4 图谱中建立实体关系 | 需 agent 配置 |
| `memory_consolidate` | 触发即时巩固（小范围） | 需 agent 配置 `allow_consolidation: true` |

#### E2. 写入安全机制

```
Agent 写入流程：
1. Agent 调用 memory_fact_add(statement, scope, confidence)
2. 系统检查：
   - confidence > 0.8？否则降级为"候选事实"，等待巩固验证
   - PII 检测？脱敏处理
   - 与现有 fact 冲突？触发冲突解决流程
3. 写入 L3 + 触发链接生成 + 触发联动更新评估
4. 写入审计日志
```

---

### 2.7 优化项 F：标准化评测体系

**论文依据**：LongMemEval (ICLR 2025)、LoCoMo、BEAM

**优化方案**：

#### F1. 评测基准接入

| 基准 | 规模 | 重点评测维度 | 接入优先级 |
|------|------|-------------|-----------|
| LongMemEval | 500 题 | 信息提取、多会话推理、时序推理、知识更新、弃权 | P0 |
| LoCoMo | 1,540 题 | 单跳、多跳、开放域、时序 | P0 |
| BEAM | 1M/10M token | 10 类（含矛盾解决、弃权） | P1 |

#### F2. 内部评测流水线

```
CI 评测流程：
1. 准备评测数据集（从 benchmark 导入）
2. 启动测试 Agent，配置待评测的 memory 策略
3. 模拟多会话交互，收集记忆写入和检索结果
4. LLM-as-Judge 评估回答质量
5. 输出各维度得分 + 与基线的对比
6. 得分低于阈值的变更阻断合并
```

---

## 三、实施路线图

### Phase 1：时间感知 + 冲突自动解决（2-3 周）

```
├── [A1] L3 fact 新增语义时间线字段 + 提取流程改造
├── [A2] L4 relation 新增双时态字段 + 冲突自动失效
├── [A3] 关系波动性分类器（规则版本，后续升级为学习版本）
├── [B1] memory_fact_links 表 + 链接生成流程
└── [F1] 接入 LongMemEval 评测
```

**验收标准**：LongMemEval temporal reasoning 维度得分 > 基线 10pp

### Phase 2：联动更新 + 仿生生命周期（3-4 周）

```
├── [B2] 联动更新引擎（核心：记忆块变更时的关联复盘）
├── [B3] L3-L4 联合推理
├── [C1] MemoryConsolidationWorker（睡眠巩固）
├── [C2] 干扰性遗忘（替换固定衰减）
├── [C3] 印迹成熟（access_count + importance 递增）
└── [C4] 提取再巩固（召回时触发检查）
```

**验收标准**：记忆存储量缩减 > 30%，检索精度不退化；联动更新覆盖率 > 80%

### Phase 3：Memory-Agent + 自主编辑 + 评测（3-4 周）

```
├── [D1] Memory-Agent 后台守护（统一记忆管理入口）
├── [D2] 睡眠巩固调度（多租户隔离）
├── [E1] Memory Tool 定义与注册
├── [E2] 写入安全机制
├── [F2] 内部 CI 评测流水线
└── [F1] 接入 LoCoMo 评测
```

**验收标准**：LoCoMo 综合得分 > 85；Agent 可通过 Tool 主动写入/更新记忆

### Phase 4：深度优化 + 多模态（4-6 周）

```
├── ROMEM 连续相位旋转（替换简单时间标签）
├── TSM 持续性记忆构建（点状→持续性）
├── 多模态记忆支持（图片/音频 embedding）
├── 跨 Agent 记忆共享（shared scope）
├── PII 自动脱敏 + GDPR 合规
├── 记忆可解释性（评分可视化 + 变更溯源）
└── BEAM 大规模评测
```

---

## 四、论文引用索引

| 编号 | 论文 | 核心贡献 | Aranea 应用 |
|------|------|----------|-------------|
| [1] | Hu et al., "Memory in the Age of AI Agents: A Survey" (arXiv:2512.13564) | 形式-功能-动态三维框架 | 记忆生命周期补全（巩固+更新） |
| [2] | Kerestecioglu et al., "Human-Inspired Memory Architecture" (arXiv:2605.08538) | 六大认知机制映射 | 睡眠巩固、干扰遗忘、印迹成熟、再巩固 |
| [3] | Xu et al., "A-Mem: Agentic Memory" (arXiv:2502.12110) | Zettelkasten 动态链接 | L3 fact 关联链接 + 联动更新 |
| [4] | Liang et al., "MARS" (arXiv:2503.19271) | 艾宾浩斯遗忘曲线 + 反思 | 动态衰减 + 质量审计反思 |
| [5] | Li et al., "ROMEM" (arXiv:2604.11544) | 连续相位旋转时间图谱 | 关系波动性分类 + 几何遮蔽 |
| [6] | Su et al., "TSM" (arXiv:2601.07468) | 语义时间线 + 持续性记忆 | 语义时间提取 + 持续性记忆构建 |
| [7] | Rasmussen et al., "Zep" (arXiv:2501.13956) | 双时态知识图谱 | L4 双时态模型 + 自动边失效 |
| [8] | Chhikara et al., "Mem0" (arXiv:2504.19413) | 双阶段提取-更新 | 冲突自动解决（ADD/UPDATE/DELETE/NOOP） |
| [9] | Packer et al., "MemGPT/Letta" (arXiv:2310.08560) | OS 路线 + Sleep-time Agent | Memory-Agent 后台守护 + Agent 自主编辑 |
| [10] | Park & Bak, "Memoria" (arXiv:2310.03052) | 仿生长期记忆 | 印迹成熟 + 选择性保留 |
| [11] | Du, "Memory for Autonomous LLM Agents" (arXiv:2603.07670) | Write-Manage-Read 循环 | 记忆管理统一框架 |
