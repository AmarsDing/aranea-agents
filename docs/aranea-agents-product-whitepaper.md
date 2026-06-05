# Aranea-Agents 产品白皮书

> **一人通过精灵控制 N 家公司，自己当发号施令的总裁，助力做你想做不敢做的事。**

---

## 一、项目定位与核心主旨

Aranea-Agents 是基于 trpc-agent-go 的企业级多智能体编排平台。以 Kratos v2 为传输壳层、trpc-agent-go 为运行时内核，提供 Agent 创建、编排、执行、监控的全生命周期管理。

**核心主旨**：让一个人通过"精灵"（Spirit 动态编排引擎）同时控制 N 家虚拟公司。你只需发号施令，行业专家 Agent 团队自动协作完成从分析、决策到执行的全流程。模拟现实公司的组织架构——分行业、分部门、分岗位，专人专事，让 AI 真正成为你的企业级生产力。

**技术栈**：Go + Kratos v2（HTTP/gRPC/WebSocket）| trpc-agent-go（Agent 运行时）| Vue 3 + Quasar + Pinia + TypeScript | SQLite（Ent ORM）| Wire（编译期 DI）

---

## 二、业务模块总览

Aranea-Agents 包含 **36+ 个业务，横跨以下核心业务域：

| 序号 | 模块 | 核心功能 |
|------|------|----------|
| 1 | 可观测性 | 全链路追踪、根因分析、自动自愈、实时监控 |
| 2 | 额度与 Token 消费 | 精细计费、多维统计、配额控制、预算告警 |
| 3 | 编排引擎 | 六模式 Team 编排 + Graph 图编排 + Spirit 动态编排 |
| 4 | 五层记忆 | L0~L4 完整记忆架构、多 scope 融合召回、可视化 |
| 5 | 组织架构 | 行业→部门→岗位三级分类、专人专事、模拟公司 |
| 6 | 技能进化 | 自动发现技能融合去重，自动进化（升级/淘汰）、人机协同审批、版本管理 |
| 7 | Agent 进化 | 运行指标采集、Persona/Prompt 自动建议、护栏机制 |
| 8 | A2A 协议 | Google A2A 标准、联邦发现、跨组织 Agent 互操作 |
| 9 | 多 Channel | 13 种 IM 平台一键接入、统一消息路由 |
| 10 | MCP 支持 | Model Context Protocol、连通性探测、健康监控 |
| 11 | 钩子系统 | 事件驱动 Webhook、精细过滤、投递保证 |
| 12 | Plugin 系统 | 11 个内置插件、安全护栏、成本守卫 |
| 13 | Provider 与模型目录 | models.dev 同步、六维定价、能力标记 |
| 14 | Agent 设置 | 50+ 运行时参数、细粒度配置 |
| 15 | 评估系统 | LLM Judge、PromptIter 优化、质量闭环 |
| 16 | 内置行业 | 金融/自媒体/软件开发三大行业、预置团队与岗位 |

---

## 三、各模块功能与优势详解

### 3.1 可观测性模块

**功能**：
- **审计日志**：记录所有管理操作（action/resource/actor/IP/severity），完整操作留痕
- **链路追踪**：每次 LLM 调用的完整 Trace + Span 瀑布图
- **Flow Log 流日志**：按 trace_id/session_id/run_id/domain/severity 多维检索
- **根因分析引擎**：基于规则匹配自动诊断失败原因，输出 RootCauseResult + FixAction
- **自动自愈系统**：DiagnoseAndHeal 完整闭环——故障检测 → 根因分析 → 自动修复
  - 置信度阈值 0.7 以上自动修复
  - 冷却期 5 分钟防抖，避免同一问题反复触发
  - 修复动作：retry / reconnect / fallback / log_only
- **告警系统**：AlertMetricRegistry + AlertEvalWorker 评估告警条件
- **诊断包**：一键聚合 trace + session + run + step 信息，便于故障排查
- **Runner 指标**：error_rate、P50/P95/P99 延迟实时统计

**优势**：
- 从故障检测到根因分析到自动修复的**完整闭环**，无需人工介入即可自愈
- 规则引擎可扩展，支持正则匹配 + 前置条件
- 全链路 Trace 让每次 Agent 调用的来龙去脉一目了然

---

### 3.2 额度与 Token 消费模块

**功能**：
- **Token 用量追踪**：每次 LLM 调用的 input/output/cached/reasoning/embedding tokens 全量记录
- **实时统计**：今日/昨日/本月/自定义时间范围用量趋势
- **多维拆解**：按 Provider、Model、Agent 维度拆解用量和成本
- **成本计算**：六维定价（input/output/cached/cache_write/reasoning/embedding）× token 数量，微美元精度
- **配额管理**：global → agent → team 三级配额控制，月度消费上限
- **预算告警**：按消费比例阈值触发告警，60 分钟冷却防抖
- **模型洞察**：标记低效模型（low_tps / high_failure / high_cost），推荐优化
- **配额仪表盘**：configured_count / total_cap / total_spent / max_utilization_ratio 一目了然

**优势**：
- **微美元精度**的精细成本核算，每分钱都算得清
- 三级配额 + 预算告警，**成本不失控**
- 低效模型洞察，帮你**发现并替换不划算的模型**
- 时间维度 + 模型维度 + Agent 维度三维统计，消费全透明

---

### 3.3 编排引擎

Aranea-Agents 提供三层编排能力：

#### 3.3.1 Team 多 Agent 编排

**六种编排模式**：

| 模式 | 说明 | 适用场景 |
|------|------|----------|
| **Sequential** | 顺序执行，前一步输出作为后一步输入 | 流水线式任务 |
| **Parallel** | 并行执行 + Synthesizer 汇总 | 独立子任务并行 |
| **Coordinator** | 协调者分派 Worker | 需要统一调度的复杂任务 |
| **CriticLoop** | 生成-批评循环，Generator + Critic 迭代优化 | 需要反复打磨的产出 |
| **Swarm** | 群智模式，成员间 transfer_to_agent 自由流转 | 开放式协作 |
| **Adaptive** | 自适应模式，运行时动态选择 | 不确定最优策略时 |

#### 3.3.2 Graph 图编排

- **可视化图定义**：拖拽式画布编辑节点和边
- **节点类型**：agent / llm / tool / task / review / subgraph
- **条件边 + 子图嵌套**：支持复杂分支逻辑
- **状态字段 + Reducer**：default / append / cover / merge 四种聚合策略
- **Checkpoint + TimeTravel**：可回溯任意检查点的状态快照
- **中断恢复**：InterruptBefore/After + ResumeExecution，支持人机协作
- **失败策略 + 熔断器**：Skip / RetryThenBlock / FailFast + CircuitBreakerPolicy
- **GC 自动回收**：30 分钟无活动自动标记失败

#### 3.3.3 Spirit 动态编排

Spirit 是 Aranea-Agents 的核心创新——**动态组装 Team 并行执行任务**：

**三阶段管线**：
1. **TaskPlanner**（任务规划）：评估 → 路由 → 记忆召回 → 分解 → 持久化 → 确认
2. **AgentAllocator**（Agent 分配）：匹配专业 Agent → 冲突检测 → 持久化
3. **TaskOrchestrator**（任务编排）：根据记忆和现状策略选择 → 图构建 → 执行 → 检查点 → 综合 → 学习 → 记录

**任务 DAG**：自动构建有向无环图，环检测 + 拓扑排序 + 就绪节点计算

**拓扑自动推断**：
- 无节点 → coordinator
- 所有节点都是根节点 → parallel
- 深度 > 3 → coordinator
- 宽度 > 1 → hybrid
- 否则 → sequential

**综合引擎**：template / prompt / hybrid 三种策略，自动选择最优

**编排缓存 + DQ 评分**：记录历史编排结果，下次同类任务推荐最优拓扑

**优势**：
- **三层编排**覆盖从简单到复杂的全场景——单 Agent 对话、多 Agent 协作、动态任务编排
- **Graph 即 Team**：统一底层引擎，Team 编排定义编译为 Graph 执行
- **Spirit 让你当总裁**：只需下达任务，系统自动分解、分配、编排、执行、综合
- **TimeTravel** 可回溯任意执行点，调试和审计无死角

---

### 3.4 五层记忆系统

Aranea-Agents 拥有业界最完整的 Agent 记忆架构：

| 层级 | 名称 | 功能 | 存储位置 |
|------|------|------|----------|
| **L0** | 会话上下文窗口 | 最近 N 轮对话 + 摘要压缩注入 | 会话快照 |
| **L1** | 工作记忆 | 结构化字段（角色/偏好/约束），token 预算控制 | 内存 + 持久化 |
| **L2** | 情景记忆 | 对话片段向量索引 + 时间衰减召回 | 向量数据库 |
| **L3** | 语义事实 | 结构化 Fact 存储，多 scope 融合召回，五维评分 | 向量数据库 |
| **L4** | 知识图谱 | 实体-关系图谱，人设/策略注入，级联更新 | 图数据库 |

**核心机制**：

- **五维评分召回**（L3）：Keyword + Vector + Importance + Recency + CrossEncoder 综合排序
- **多 scope 融合**：L3 召回可跨 agent / user / team / workspace / global 五个 scope 聚合去重
- **Saga 级联更新**（L4）：名称冲突检测 → Proposal → 审批 → Saga 四步原子更新（UpsertEntity → TouchAffected → ReplaceFacts → SyncIndex），失败自动补偿回滚
- **三链整合器**：ChainConsolidator 依次尝试 LLM → 启发式正则 → 反馈提取，确保即使 LLM 不可用也能提取记忆
- **策略审计**：MemoryPolicyEngine 记录所有记忆变更决策，strict 模式下审计失败阻塞写入
- **6 个 Cron Workers**：L1Archive / L2Consolidate / L2Decay / L3Decay / L4Decay / FactIndexReconciler

**记忆可视化**：
- **Memory Center 页面**：五层记忆统一浏览
- **知识图谱浏览器**（L4）：实体-关系图可视化
- **召回测试器**：调试 L2/L3 召回质量，查看 score breakdown
- **级联变更面板**：L4 Saga 步骤追踪
- **进化面板**：记忆进化事件可视化
- **Worker 状态面板**：6 个 Worker 运行状态 + 队列统计
- **死信管理**：失败任务查看/重试/放弃
- **PII 审查**：标记含个人隐私的 Fact，人工审核

**优势**：
- **五层记忆**让 Agent 从"金鱼脑"变成"过目不忘"——短期对话、中期经验、长期知识全覆盖
- **多 scope 融合**让知识在 Agent/用户/团队/工作空间之间流动
- **Saga 级联更新**保证知识图谱的一致性，改一处自动传播到所有关联
- **可视化**让记忆不再是黑盒，你可以看到 Agent 记住了什么、为什么记住、如何使用

---

### 3.5 组织架构——分行业、分部门、分岗位

**三级分类体系**：Industry（行业）→ Department（部门）→ Position（岗位）

**功能**：
- **行业预置**：内置金融/自媒体/软件开发三行业完整 Agent 种子
- **岗位 Prompt 模板**：每个岗位有预置 prompt 模板，支持变体（general / technical / management），自动 fallback
- **职责构建**：BuildResponsibility 根据岗位+部门描述生成 L1 注入内容
- **祖先链查询**：从岗位向上追溯行业和部门
- **FieldGuide 注册表**：9 个 scope 的写作指导（should_write / should_avoid / examples / budget），确保 prompt 质量一致
- **组织结构抽取**：从自由 markdown 文档自动抽取为 YAML 组织规格

**模拟现实公司**：
- 分行业：金融、自媒体、软件开发……每个行业有专属部门和岗位
- 分部门：量化交易、风控合规、投资研究……每个部门有专属 Agent
- 分岗位：量化研究员、合规官、行业分析师……每个岗位有专业 prompt 模板
- 专人专事：不同功能的 Agent 各司其职，协作完成复杂任务

**一人多公司**：
- 通过 workspace 隔离，一个人可以同时运营多家"虚拟公司"
- 每家公司有独立的行业、部门、岗位、Agent、Team 配置
- Spirit 引擎让你只需下达指令，各公司 Agent 团队自动协作

**生态市场**（开发中）：
- **公司市场**：行业公司模板一键安装，快速搭建虚拟公司
- **部门市场**：按行业分类的部门模板，包含专属 Agent 和协作流程
- **岗位市场**：专业岗位 Agent 模板，开箱即用的行业专家
- **Agent 市场**：用户创建的 Agent 可发布/安装/评分，支持 certified 认证
- **Team 市场**：预置团队编排模板，一键部署多 Agent 协作
- **Graph 市场**：可视化工作流模板，覆盖常见业务场景
- **Skill 市场**：可复用技能包，跨 Agent 共享专业能力
- **MCP 市场**：MCP 服务器目录，一键接入外部工具和数据源

**优势**：
- **开箱即用**的行业知识 + 岗位模板，快速创建专业 Agent
- **模拟真实公司**的组织架构，让 AI 协作更符合现实逻辑
- **生态市场**：从公司到 MCP 的全品类市场，让最佳实践流动起来

---

### 3.6 技能进化系统

技能进化是 Aranea-Agents 的核心自迭代能力，让技能从实践中诞生、在运行中优化、于低效时消亡，形成完整的生命周期闭环。

**功能概览**：
- **Skill CRUD**：创建/更新/删除/启用/禁用，支持版本管理（major.minor.patch）、标签、继承（extends）
- **自动发现**：`SkillEvolutionUsecase.DetectAndPropose` 自动检测工具调用模式 → 生成 SKILL.md 提案
- **相同技能去重**：基于 Pattern Hash 的确定性去重
- **相似功能融合**：六维相似度评估 + LLM 炼化合并
- **技能链路审批**：Proposal 生命周期状态机 + 人工审批
- **渐进式加载**：3 阶段按需加载，节省 Token 预算
- **技能消亡与新生**：健康度评估 + 版本回滚 + 文件系统恢复
- **运行时路由**：SkillRuntimePolicy + 嵌入向量评分控制 Skill 激活策略
- **文件系统同步**：Skill 与磁盘文件双向同步（fsnotify 实时监听 + 定时对账）
- **技能管家工具**：evolve_skill / optimize_skill / recommend_skills / analyze_skill_usage / analyze_skill_health / analyze_tool_weights / analyze_orchestration / optimize_orchestration

---

#### 3.6.1 相同技能去重（Pattern Hash 机制）

**依据**：Agent 运行时可能多次产生相同的工具调用模式，如果不做去重，同一行为会被反复提议创建 Skill，造成冗余。
**去重范围**：同一 Agent 下相同行为模式只产生一次 Proposal。不同 Agent 发现相同模式会各自产生独立 Proposal（因为业务上下文不同）。

---

#### 3.6.2 相似功能融合（六维相似度 + AI 炼化）

**依据**：导入或新建的 Skill 可能与已有 Skill 功能重叠，简单并存会导致 Agent 选择困难和 Token 浪费。需要智能识别相似度并建议合并。

**六维相似度评估**（`SimilarityMetrics`）：

| 维度 | 评估内容 |
|------|----------|
| NameSimilarity | 名称相似度 |
| DescriptionSimilarity | 描述相似度 |
| BodySimilarity | 正文/实现逻辑相似度 |
| TriggerSimilarity | 触发条件相似度 |
| ToolSimilarity | 工具调用相似度 |
| SimilarityScore | 综合相似度（加权汇总） |

**融合流程**：

1. **相似度检测**：对每个候选 Skill 与已有 Skill 进行 LLM 对比（上限 50 次调用）
2. **冲突组创建**：当 `SimilarityScore >= 0.2` 时，创建 `ConflictGroup`，标记为 `merge_suggested`
3. **LLM 推荐动作**：返回 `keep_separate`（保留独立）/ `suggest_refine`（建议合并）/ `block_duplicate`（阻止重复）
4. **AI 炼化**（`RefineConflictGroup`）：LLM 将相似 Skill 合并为新 Skill，输出 `MergedName + MergedDescription + MergedBody + MergedTags`
5. **人工决策**：5 种 action 可选——直接导入 / 批准高风险 / 拒绝 / AI 合并 / 跳过
6. **补偿回滚**：合并过程中任何一步失败，自动回滚所有已创建的 Skill（DB 行 + 磁盘目录）

---

#### 3.6.3 技能链路汇总与人工审批

**依据**：自动进化必须有人类把关，否则可能产生低质量或有害 Skill。审批流程确保每一步都有据可查。

**Proposal 生命周期状态机**：

```
detected pattern → pending → approved → registered
                          → rejected
                          → expired
```

| 状态 | 含义 | 允许的转换 |
|------|------|-----------|
| pending | 刚检测到，等待审批 | → approved / rejected / expired |
| approved | 人工批准 | → registered |
| rejected | 人工拒绝 | 终态 |
| registered | 已注册到文件系统 | 终态 |
| expired | 过期未审批 | 终态 |

**审批流程**：
1. **自动检测**：`DetectAndPropose` 扫描 Pattern，过滤 `Confidence >= 0.15` 的工具调用模式，生成 pending Proposal
2. **批量扫描**：`ScanAndProposeAll` 遍历所有 active Agent，对开启了 `EvolutionSkillEvolve` 设置的 Agent 执行检测
3. **人工审批**：`ApproveProposal` 只允许 pending 状态被批准
4. **注册**：`RegisterApproved` 检查 Skill 是否已存在，通过 `FileSystemSkillRegistrar` 将 SKILL.md 写入 Agent 的 Skill 目录
5. **拒绝**：`RejectProposal` 只允许 pending 状态被拒绝

**安全措施**：注册时拒绝包含 `..` 或 `/\` 的 Skill 名称，防止路径穿越攻击。

---

#### 3.6.4 技能递进加载（Progressive Loading）

**依据**：Agent 可能拥有数十个 Skill，如果全部注入系统提示，会消耗大量 Token 且干扰 LLM 判断。递进加载让 Agent 按需获取技能，既省 Token 又提精度。

**四种加载模式**：

| 模式 | 说明 |
|------|------|
| `once` | 下一次请求注入后卸载 |
| `turn` | 当前 invocation 内有效（默认） |
| `session` | 跨 invocation 直到会话过期 |
| `progressive` | 3 阶段渐进加载 |

**Progressive 模式 3 阶段**：

| 阶段 | 名称 | 触发方式 | 注入内容 | Token 消耗 |
|------|------|----------|----------|-----------|
| L0 | 清单注入 | 自动（每轮） | 所有 Skill 的 name + description 摘要 | 极低 |
| L1 | Body 按需加载 | LLM 主动调用 `skill_load` 工具 | 指定 Skill 的 SKILL.md 正文 + 可选 docs | 按需 |
| L2 | Refs 按需加载 | LLM 主动调用 `skill_select_docs` 工具 | 辅助文档和参考资料 | 按需 |

**关键机制**：
- **Turn 级状态清理**：Progressive 模式每轮对话开始时清理上一轮的 loaded/docs 状态，确保每轮重新按需加载
- **Tool Result 注入**：加载的 SKILL.md 内容注入 tool result 消息而非系统提示，保持系统提示稳定，有利于 prompt caching
- **加载上限**：可配置同时加载的 Skill 数量上限，超出时按最近使用时间淘汰
- **意图路由**：`IntentRoutingEnabled`（默认开启）+ 嵌入向量评分（权重 0.3）自动匹配最相关 Skill

---

#### 3.6.5 技能消亡与新生

**依据**：技能和生物一样有生命周期——长期不用的技能应被识别和清理，避免运行时路由噪音；被误删或降级的技能应能恢复。

**消亡机制**：

| 触发条件 | 机制 | 代码依据 |
|----------|------|----------|
| 人工主动停用 | `ToggleEnabled` 设为 false，从运行时候选列表移除 | skill.go `ToggleEnabled` |
| 软删除 | 设置 `deleted_at` + `status = "deleted"`，非物理删除 | data/skill.go `DeleteSkill` |
| 磁盘文件缺失 | 标记 `filesystem_missing = true`，文件系统 Watcher 检测 | watch/runner.go `MarkFilesystemMissing` |
| 磁盘内容变更 | 已 published 的 Skill 自动回退为 `draft` + `enabled = false` | data/skill.go `UpsertSkillFromDisk` |
| 健康度评估 | Skills Butler 工具基于调用统计评估 | skills_butler/analyze_skill_usage.go |

**健康度评估标准**（`assessHealth`）：

| 等级 | 条件 | 建议 |
|------|------|------|
| healthy | 周均调用 > 5 且成功率 >= 80% | 正常运行 |
| warning | 周均调用 > 5 且成功率 >= 60%，或中等使用率 | 关注优化 |
| critical | 周均调用 < 2 或成功率 < 60% | 评估是否保留 |

**消亡建议**（`optimize_skill` 工具）：

| 触发条件 | 建议 | 优先级 |
|----------|------|--------|
| 近 30 天无调用 | 评估是否仍需要此 Skill | high |
| 成功率 < 60% | 检查 Skill 实现逻辑 | high |
| 成功率 < 80% | 增加错误处理 | medium |
| 平均耗时 > 5000ms | 优化执行路径 | medium |
| 周均调用 < 2 | 评估是否需要保留 | medium |

**新生/重生机制**：

| 场景 | 机制 | 代码依据 |
|------|------|----------|
| 磁盘文件恢复 | `filesystem_missing` 标记自动清除，触发 `skill.filesystem.recovered` 事件 | watch/runner.go |
| 版本回滚 | 基于历史版本创建新版本（patch +1），状态恢复为 `published` | data/skill.go `RollbackSkillVersion` |
| 重新发布 | 将 `draft` 状态的 Skill 重新 `PublishSkill` | data/skill.go `PublishSkill` |
| 自动进化 | 从工具调用模式中检测到新 Pattern → 生成 Proposal → 审批 → 注册 | skill_evolution.go |

**技能生命周期全景**：

```
            ┌──────────────────────────────────────────────┐
            │              技能生命周期                       │
            │                                              │
            │  新生                                         │
            │  ├─ 自动检测：Pattern → Proposal → 审批 → 注册  │
            │  ├─ 手动创建：CRUD → Publish                   │
            │  └─ 导入融合：冲突检测 → AI 炼化 → 合并注册      │
            │                                              │
            │  成长                                         │
            │  ├─ 渐进加载：L0 清单 → L1 Body → L2 Refs      │
            │  ├─ 版本迭代：major.minor.patch 版本管理         │
            │  └─ 优化建议：Skills Butler 健康度分析           │
            │                                              │
            │  消亡                                         │
            │  ├─ 自然消亡：30天无调用 / 成功率 < 60%          │
            │  ├─ 人工停用：ToggleEnabled → false             │
            │  └─ 软删除：deleted_at 标记                     │
            │                                              │
            │  重生                                         │
            │  ├─ 版本回滚：历史版本恢复                       │
            │  ├─ 重新发布：draft → published                 │
            │  └─ 文件恢复：filesystem_missing 自动清除        │
            └──────────────────────────────────────────────┘
```

---

**技能进化系统整体优势**：
- **从实践中学习**：不是预设规则，而是从真实运行数据中发现可封装的行为
- **相同去重 + 相似融合**：Pattern Hash 确定性去重 + 六维相似度 LLM 评估 + AI 炼化合并，既防冗余又促整合
- **人机协同审批**：自动进化 + 人工审批，Proposal 状态机确保每步有据可查
- **递进加载省 Token**：3 阶段按需加载，L0 仅注入清单，L1/L2 按需获取，大幅节省 Token 预算
- **消亡与新生闭环**：健康度评估驱动消亡建议，版本回滚和文件恢复保障重生能力

---

### 3.7 Agent 进化系统

**功能**：
- **指标采集**：工具成功率、检索质量、情景数、负面反馈
- **建议生成**：
  - persona 类型：写入 IDENTITY.md 的 ## Persona 段
  - prompt 类型：写入 AGENTS_CORE.md
- **应用/拒绝**：ApplySuggestion 直接修改 Agent 的 prompt 文件
- **学习闭环**（LearningLoop）：Observation → Pattern → Proposal → Validation → Registration
  - 观察类型：tool_call / feedback / error / retrieval

**护栏机制**：
- `GuardrailMaxChangePerPeriod`：限制变更速率
- `GuardrailMinDataPoints`：最低数据点要求
- `GuardrailRollbackOnDeclinePercent`：质量下降自动回滚

**Agent 设置**（50+ 运行时参数）：详见 [3.14 Agent 设置](#314-agent-设置)

**优势**：
- **自动进化闭环**：从运行数据中提取模式 → 生成建议 → 人工审批 → 自动应用
- **护栏机制**确保进化不失控——限速、最低数据、回滚三重保障
- **50+ 参数**让每个 Agent 可精细调优

---

### 3.8 A2A 协议

**功能**：
- **AgentCard 管理**：发布/更新 Agent 的 A2A 能力卡片（Capabilities + InputSchema + OutputSchema）
- **能力发现**：Discover 聚合本地 + 远程 Agent 的能力卡片，支持按 capability 过滤
- **调用管理**：StartInvocation / FinishInvocation 记录跨 Agent 调用生命周期
- **审计日志**：所有跨 Agent 调用都有审计记录
- **远程注册**：RegisterRemoteAgent 自动发现远程 AgentCard 并持久化
- **网关发现**：GatewayDiscover 聚合本地端点 + 远程注册表，支持健康检查
- **A2A Proxy**：Agent 可作为 A2A 代理，将远程 Agent 包装为本地 Agent

**优势**：
- **Google A2A 标准**：基于业界标准的多 Agent 互操作协议
- **联邦发现**：跨组织 Agent 发现和调用，打破 Agent 孤岛
- **完整审计**：所有跨 Agent 调用可追溯

---

### 3.9 多 Channel 支持

**支持 13 种 IM 平台**：

| 平台 | 接入方式 |
|------|----------|
| 飞书 / Lark | WebSocket 长连接 |
| 钉钉 | Webhook / Stream |
| 企业微信（智能机器人） | Webhook |
| 企业微信（自建应用） | API |
| 微信公众号 | API |
| Slack | WebSocket |
| Telegram | Long Polling |
| Discord | WebSocket |
| LINE | Webhook |
| Microsoft Teams | API |
| Mattermost | WebSocket |
| QQ | OneBot 协议 |
| 个人 QQ | OneBot 协议 |

**功能**：
- **统一抽象**：所有平台通过统一 Channel 接口管理，Agent 无需关心底层平台差异
- **消息路由**：IM 消息自动路由到对应 Agent
- **IM 渲染**：Agent 输出自动渲染为各平台卡片/消息格式
- **凭证加密**：API 密钥加密存储 + masked preview
- **连通性测试**：实时验证通道连通性
- **入站去重**：防止消息重复处理

**优势**：
- **13 种平台一键接入**，Agent 一次创建、全平台可用
- **统一抽象**让 Agent 与平台解耦，新增平台无需改 Agent 代码

---

### 3.10 MCP 支持

**功能**：
- **MCP Server CRUD**：创建/配置/删除 MCP 服务器
- **连通性测试**：MCPProber.Evaluate 验证 MCP 服务器可达性
- **健康监控**：自动探测 + 告警去抖 + 重连计数
- **凭证加密**：CredentialCrypto 加密存储 MCP 认证信息
- **用户级凭证**：每个用户可配置独立的 MCP 认证

**优势**：
- **MCP（Model Context Protocol）**是 Anthropic 推出的工具调用标准，支持跨平台工具集成
- 健康监控确保 MCP 服务可靠性

---

### 3.11 钩子系统（Hook）

**功能**：
- **Hook CRUD**：创建/更新/删除/启用/禁用 Hook
- **条件过滤**：HookCondition 支持按 Agent + Tool + Event 三维过滤
- **动作执行**：HookAction 支持 Webhook 回调、日志记录等
- **Webhook 出站**：run.completed / run.failed / run.cancelled 等事件触发外部回调
- **投递追踪**：HookDelivery 记录每次投递状态（pending/success/failed）+ 重试机制

**优势**：
- **事件驱动**的松耦合通知机制，Agent 行为可触发外部系统
- **三维过滤**避免无关通知，精准触达
- **投递保证**确保通知可靠送达

---

### 3.12 Plugin 系统

**11 个内置插件**：

| 插件 | 功能 |
|------|------|
| **identity** | 身份注入，自动将 Agent 身份信息注入上下文 |
| **guardrail** | 安全护栏，防止 Agent 输出违规内容 |
| **toolcallid** | 工具调用 ID 追踪，确保调用链完整 |
| **messagemerger** | 消息合并，优化流式输出 |
| **confirmation_guard** | 工具调用确认，高危操作需人工审批 |
| **permission_guard** | 工具权限控制，deny_list 机制 |
| **cost_guard** | 成本预算守卫，按 scope 限流 |
| **model_router** | 模型路由，按规则自动切换模型 |
| **output_policy** | 输出策略控制，限制输出格式和内容 |
| **sensitive_mask** | 敏感信息脱敏，防止泄露隐私数据 |
| **skill_tracker** | Skill 调用追踪，记录技能使用情况 |

**回调编排边界**（三层）：
- Runner 层：DB-backed 插件 + 框架插件
- LLMAgent 层：产品指标 + 工具计时/记录 + Hook 规则
- ModelSelector 层：model_router / cost_guard

**优势**：
- **五重安全防护**：confirmation_guard + permission_guard + sensitive_mask + output_policy + cost_guard
- **成本控制**：cost_guard 按 scope 限流，避免超支
- **分层编排**：三层回调链职责清晰，互不干扰

---

### 3.13 Provider 与模型目录

**功能**：
- **models.dev 同步**：从 models.dev 拉取全球 AI 模型规格，作为模型参数和定价的外部真相源
- **定价优先级**：manual（100）> model-inspect（50）> models.dev-sync（10），低优先级不能覆盖高优先级
- **六维定价**：Input / Output / CacheRead / CacheWrite / Reasoning / Embedding
- **双轨定价模型**：MicroPricing（内部精确计算）+ CostUSDPer1M（对外展示）
- **能力标记**：text / vision / audio / file / tool_call / cache / thinking / text_only
- **Provider 迁移**：旧 provider_code 自动迁移到 models.dev id，支持断点续传
- **Runtime Overlay**：models.dev provider id → trpc-agent-go 运行时映射
- **定时同步**：每小时自动同步 models.dev 数据

**支持 12+ Provider**：OpenAI、Anthropic、Gemini、DeepSeek、Qwen（阿里通义）、Moonshot（月之暗面）、OpenRouter、ZhipuAI（智谱）、Amazon Bedrock、Ollama、Hunyuan（腾讯混元）、HuggingFace

**优势**：
- **models.dev 同步**确保模型参数和定价始终最新，无需手动维护
- **六维定价**精确计算每分钱，为消费统计做精准计量
- **定价优先级**防止自动同步覆盖手动设置
- **能力标记**让系统自动选择合适的模型（如仅 vision 模型用于图像理解）

---

### 3.14 Agent 设置

**50+ 运行时参数**，按配置域分组：

| 配置域 | 参数示例 |
|--------|----------|
| Identity | 路由身份、Agent Kind（llm/a2a_proxy）、Source（user/system/builtin/industry/marketplace） |
| Reasoning | 推理模式、推理深度 |
| Memory | L0~L4 每层独立开关和参数、记忆模式（Agentic/Auto） |
| Tools | 工具执行策略、重试次数、并行度、流式、熔断器 |
| Skills | Skill 加载模式、意图传递 |
| Evolution | 自进化开关、子Agent、护栏参数 |
| Context | 上下文压缩、输出 schema、压缩缓存 |
| CodeExecutor | 代码执行后端选择 |

**优势**：
- 每个 Agent 可独立配置，**术业专攻**
- 五层记忆全部可配，按需开关
- 工具策略细粒度控制（重试/并行/熔断/确认）

---

### 3.15 评估系统

**功能**：
- **数据集管理**：创建/上传/删除评估用例集
- **多维度评分**：
  - ExactMatch（精确匹配）
  - ContainsMatch（包含匹配）
  - LLMjudgeScore（LLM 评审打分）
  - ToolCallAccuracy（工具调用准确率）
  - PassAtK / PassHatK（统计通过率）
- **趋势分析**：GetAgentEvalTrend 生成质量趋势图
- **运行对比**：CompareEvalRuns 计算多次运行的指标增量（Delta）
- **人工标注**：AnnotateCaseResult 支持人工 pass/fail + 评分 + 评论
- **自动评估**：AgentEvalAutoConfig 支持每轮对话后自动触发评估
- **PromptIter 引擎**：多轮优化循环——训练集生成梯度 → 验证集验收 → 接受/拒绝补丁

**优势**：
- **闭环质量保障**：自动评估 → 趋势追踪 → PromptIter 优化，形成完整改进闭环
- **多维度评分**：不仅看文本匹配，还评估工具调用准确率和 LLM 评审质量
- **人工 + 自动混合**：LLM 评审 + 人工标注双轨

---

### 3.16 内置行业、Team、Graph、Agent

**三大内置行业**：

| 行业 | 部门数 | 岗位数 | 预置团队 |
|------|--------|--------|----------|
| **金融** | 6 | 30+ | 8 |
| **自媒体** | 5 | 24+ | - |
| **软件开发** | 10 | 30+ | - |

**金融行业（最完整）**：

6 个部门：量化交易、风控合规、投资研究、金融工程、财富管理、衍生品

8 个预置团队：
1. **盘前简报**（coordinator 模式，6 成员）
2. **个股深度研究**（coordinator 模式，10 成员）
3. **板块轮动**（sequential 模式，4 成员）
4. **组合诊断**（parallel 模式，4 成员）
5. **市场复盘**（sequential 模式，5 成员）
6. **量化策略研发**（sequential 模式，4 成员）
7. **投资决策委员会**（coordinator 模式，5 成员）
8. **风险监控**（parallel 模式，3 成员）

**Agent 来源分类**：user | system | system_builtin | industry_template | marketplace | certified

**Graph 模板**：ListGraphTemplates / CreateGraphFromTemplate / SaveGraphAsTemplate

**优势**：
- **开箱即用**：内置行业知识 + 岗位模板 + 预置团队，用户无需从零开始
- **可扩展**：通过 scenario 目录添加新行业/岗位/变体，无需改代码

---

## 四、竞品痛点与 Aranea-Agents 的解决方案

### 4.1 竞品痛点

| 痛点 | 说明 |
|------|------|
| **Agent 是"金鱼脑"** | 主流 Agent 平台只有短期对话记忆，跨会话即遗忘，无法积累经验和知识 |
| **单 Agent 天花板** | 单个 Agent 能力有限，复杂任务需要多 Agent 协作，但编排手段匮乏 |
| **编排黑盒** | 多 Agent 编排过程不可观测、不可调试、不可回溯，出了问题无从排查 |
| **成本失控** | Token 消费不透明，无法按模型/Agent/时间维度统计，月底账单吓人 |
| **平台锁定** | Agent 只能在 Web 端使用，无法接入飞书/钉钉/企微等 IM 工作台 |
| **无行业知识** | 创建 Agent 需要从零写 prompt，缺乏行业专业知识注入 |
| **不会进化** | Agent 创建后能力固化，无法从运行经验中学习和改进 |
| **孤岛效应** | 不同系统/组织的 Agent 无法互操作，形成 Agent 孤岛 |
| **安全失控** | Agent 可执行任意工具调用，缺乏权限控制和成本守卫 |
| **一人难成军** | 个人用户无法像管理公司一样管理多个 Agent 团队 |

### 4.2 Aranea-Agents 的解决方案

| 痛点 | 解决方案 |
|------|----------|
| Agent 是"金鱼脑" | **五层记忆系统**：L0~L4 完整记忆架构，从短期对话到长期知识图谱全覆盖，支持多 scope 融合召回和 Saga 级联更新 |
| 单 Agent 天花板 | **三层编排引擎**：Team 六模式编排 + Graph 图编排 + Spirit 动态编排，覆盖从简单到复杂的全场景 |
| 编排黑盒 | **全链路可观测**：Trace + Flow Log + 根因分析 + 自动自愈 + TimeTravel 回溯，每一步都可追踪 |
| 成本失控 | **精细计费体系**：六维定价 × 微美元精度 × 三级配额 × 预算告警 × 低效模型洞察，每分钱都算得清 |
| 平台锁定 | **13 种 Channel 一键接入**：飞书/钉钉/企微/微信/Slack/Telegram/Discord 等，Agent 一次创建全平台可用 |
| 无行业知识 | **内置行业体系**：三级分类 + FieldGuide + 预置 prompt 模板 + 预置团队，开箱即用 |
| 不会进化 | **三层自动进化**：LearningLoop → Evolution → SkillEvolution，从运行数据中学习并持续优化 |
| 孤岛效应 | **A2A 联邦协议**：基于 Google A2A 标准的跨组织 Agent 互操作，打破 Agent 孤岛 |
| 安全失控 | **五重安全防护**：confirmation_guard + permission_guard + sensitive_mask + output_policy + cost_guard |
| 一人难成军 | **Spirit 动态编排 + 组织架构**：一人控制 N 家虚拟公司，分行业/部门/岗位，专人专事，你当总裁 |

---

## 五、核心差异化优势总结

1. **五层记忆架构**：业界最完整的 Agent 记忆系统，从会话窗口到知识图谱全覆盖，支持多 scope 融合召回和 Saga 级联更新，让 Agent 从"金鱼脑"变成"过目不忘"

2. **三层编排引擎**：Team 六模式 + Graph 图编排 + Spirit 动态编排，覆盖从简单对话到复杂企业级任务的全场景，Graph 即 Team 统一底层引擎

3. **自动进化闭环**：LearningLoop → Evolution → SkillEvolution 三层自动进化，Agent 和技能都能从运行数据中学习并持续优化，护栏机制确保进化不失控

4. **全链路可观测**：Trace + Flow Log + 根因分析 + 自动自愈 + TimeTravel，从故障检测到根因分析到自动修复的完整闭环

5. **精细成本管控**：六维定价 × 微美元精度 × 三级配额 × 预算告警 × 低效模型洞察，Token 消费全透明

6. **13 通道统一接入**：Agent 一次创建、全平台可用，统一抽象让 Agent 与平台解耦

7. **A2A 联邦协议**：基于 Google A2A 标准的跨组织 Agent 互操作，打破 Agent 孤岛

8. **内置行业体系**：三级分类 + FieldGuide + 预置 prompt + 预置团队，开箱即用的行业专业知识

9. **五重安全防护**：confirmation_guard + permission_guard + sensitive_mask + output_policy + cost_guard，安全与成本双重保障

10. **一人当总裁**：Spirit 动态编排 + 组织架构模拟，一人通过精灵控制 N 家虚拟公司，分行业/部门/岗位，专人专事——助力做你想做不敢做的事

---

## 六、系统架构概览

```
┌─────────────────────────────────────────────────────────────────┐
│                        前端 (Vue 3 + Quasar)                      │
│  43 Store · 37+ 页面 · 45 种 Envelope 事件 · WebSocket 实时通信    │
├─────────────────────────────────────────────────────────────────┤
│                     传输层 (Kratos v2)                            │
│           HTTP · gRPC · WebSocket · 中间件链 · Wire DI            │
├─────────────────────────────────────────────────────────────────┤
│                      Service 层 (38+ Service)                     │
│    ChatService(核心编排器) · AgentService · TeamService · ...      │
├─────────────────────────────────────────────────────────────────┤
│                       Biz 层 (36+ Usecase)                        │
│  AgentUsecase · ChatUsecase · TeamUsecase · MemoryUsecase · ...   │
├─────────────────────────────────────────────────────────────────┤
│                      Data 层 (60+ Repo)                           │
│          Ent ORM + 原生 SQL · SQLite + PostgreSQL(向量)            │
├─────────────────────────────────────────────────────────────────┤
│                   运行时 (trpc-agent-go)                           │
│   Runner · Agent · Session · Memory · Tool · Skill · Graph · Team  │
├─────────────────────────────────────────────────────────────────┤
│                     基础设施层                                     │
│  Provider(12+) · Channel(13) · MCP · Hook · Plugin(11) · A2A      │
└─────────────────────────────────────────────────────────────────┘
```

---

*Aranea-Agents — 一人通过精灵控制 N 家公司，自己当发号施令的总裁。*
