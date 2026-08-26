# 05 五层记忆系统

## 功能

业界最完整的 Agent 记忆架构：**L0~L4 五层全覆盖**——短期对话、中期经验、长期知识一站解决，让 Agent 从"金鱼脑"变成"过目不忘"。

## 原理

![五层记忆架构](../assets/diagrams/memory-layers.svg)

| 层级 | 名称 | 功能 | 存储 |
|------|------|------|------|
| **L0** | 会话上下文窗口 | 最近 N 轮对话 + 摘要压缩注入，控制上下文窗口 | 会话快照 |
| **L1** | 工作记忆 | 结构化字段（角色/偏好/约束），token 预算控制 | 内存 + 持久化 |
| **L2** | 情景记忆 | 对话片段向量索引 + 时间衰减召回，FTS-OR 混合 | pgvector |
| **L3** | 语义事实 | 结构化 Fact 存储，多 scope 融合召回，五维评分 | pgvector |
| **L4** | 知识图谱 | 实体-关系图谱，人设/策略注入，Saga 级联更新 | 图存储 |

### 召回引擎（L3 核心）

- **五维评分**：Keyword + Vector + Importance + Recency + CrossEncoder 综合排序；
- **多 scope 融合**：可跨 agent / user / team / workspace / global 五个 scope 聚合去重——知识在 Agent、用户、团队之间流动；
- **三链整合器**：ChainConsolidator 依次尝试 LLM → 启发式正则 → 反馈提取，**即使 LLM 不可用也能提取记忆**；
- **策略审计**：MemoryPolicyEngine 记录所有记忆变更决策，strict 模式下审计失败阻塞写入。

### Saga 级联更新（L4）

改一处、自动传播到所有关联，且保证一致性：

```text
名称冲突检测 → Proposal → 审批
  → Saga 四步原子更新：UpsertEntity → TouchAffected → ReplaceFacts → SyncIndex
  → 任一步失败自动补偿回滚
```

### 记忆维护 Worker（后台，cronrunner/jobs）

**生命周期（按层）：**

| Worker | 职责 |
|--------|------|
| MemoryL1ArchiveWorker | L1 空闲任务归档（自动桥接创建 L2 Episode）+ 过期字段清理 |
| MemoryL2DecayWorker | L2 情景记忆时间衰减 |
| MemoryL3DecayWorker | L3 语义事实衰减 |
| MemoryEbbinghausDecayWorker | L3 艾宾浩斯指数衰减（记忆强度评分） |
| MemoryL4DecayWorker | L4 图谱关系衰减（Hebbian：久未激活的关系降权/归档） |
| MemorySleepTimeWorker | 睡眠时固化：L2→L3 提炼 / 反思 / merge / update_core |

**回填与运维：**

| Worker | 职责 |
|--------|------|
| MemoryEpisodeBackfillWorker | Episode 向量索引回填 |
| MemoryCitationBackfillWorker | 事实引用回填（回复引用识别，幂等去重） |
| MemoryFactIndexReconciler | 事实索引对账 |
| MemoryDataMigrationWorker | 旧版记忆数据迁移 |
| MemoryCanaryWorker | 固化链路金丝雀自检 |
| MemoryDeadLetterReplayer | 记忆任务死信重放 |

另有事件驱动的 **AutoMemoryWorker**（非 cron）：Turn 结束后自动提取结构化记忆任务，经 AutoMemoryQueue 异步消费。

### Token 预算控制

L0 装配受 `MemoryPromptTotalBudgetTokens` 等预算约束（策略层字段，不落库）；压缩阈值 0.7/0.9 两级触发摘要压缩——注意压缩机制拦不住"轮数型"二次方累计，需配合 [Token 双闸](04-team-graph.md#43-token-双闸安全边界)。

## 设计要点

- **五层独立开关**：每个 Agent 的 L0~L4 可单独开关与调参（见 Agent 设置）；
- **写入不阻塞对话**：记忆沉淀全部异步落库；
- **PII 保护**：事实可被打标为含个人隐私，进入人工审查队列，不改正文；
- **可解释**：记忆中心能回答「Agent 为什么这样回答」——每层注入了什么都可查。

## 界面配置

左侧导航 **记忆中心**，五个视角：

![记忆中心](../assets/screenshots/aranea-memory.png)

1. **层级全景**（上图）：L0→L4 流水线卡片，显示每层健康状态、条目数、今日新增、召回次数；
2. **关联图谱**：L4 知识图谱浏览器，实体-关系可视化；
3. **记忆浏览**：逐层查看条目；**召回测试器**可调试 L2/L3 召回质量、查看 score breakdown；
4. **信任**：PII 审查、级联变更面板（Saga 步骤追踪）；
5. **运维**：各 Worker 运行状态 + 队列统计、死信管理（查看/重试/放弃）、进化面板。

右上角可切换**智能体**，查看任一 Agent 的记忆视图。

## 深入阅读

- [65 模块交叉引用 · memory 章节](../../docs/development/65-module-cross-reference-full.md)
- [Agent Memory Challenge 方法披露](../../docs/scenarios/agent-memory-challenge/README.md)
