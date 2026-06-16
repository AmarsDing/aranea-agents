# Review: 业务逻辑设计评审 — 全模块业务合理性评估

> **日期**：2026-06-16
> **版本**：v1.3（经评审交叉验证修订）
> **范围**：后端 biz 层全部业务模块 + 前端核心业务模块
> **方法**：逐模块读取源码，按职责清晰度、状态机合规、接口设计、关注点分离、过度/不足工程五个维度评审

---

## 摘要

对 Aranea-Agents 项目 12 个业务域的业务逻辑设计进行全面评审。项目整体呈现**架构方向正确、执行深度不均**的状态：状态机框架（`shared.GenericStateMachine`）和接口窄化是两大亮点，但 God Object、双重状态机、缺失状态机、压缩策略外溢等问题在多个域中反复出现。

**核心发现**：
- 3 个严重 God Object（AgentUsecase 21 方法/10 依赖、SpiritTeamUsecase 1432 行/28 方法、SessionUsecase Facade 委托链过深）
- 1 个超级结构体（AgentRuntimeSettings 139 字段）
- 8 个实体缺失显式状态机（违反 AS-FSM-01）
- 1 处双重状态机（Session）
- GraphExecution 状态机形同虚设（14 处直接赋值绕过 + 2 处构造函数绕过，仅 1 处校验且校验后仍直接赋值）
- 前端 Chat 模块 AF 架构设计优秀，消息分组逻辑正确

---

## 一、各域设计质量总览

| 域 | 职责清晰 | 状态机合规 | 接口设计 | 关注点分离 | 综合 | 核心问题 |
|----|----------|-----------|---------|-----------|------|---------|
| Agent | 6/10 | 9/10 | 7/10 | 5/10 | **6.5** | God Object + 超级结构体 |
| Session/Run | 7/10 | 5/10 | 9/10 | 7/10 | **7.0** | 双重状态机 + 压缩三层透传 |
| Team/Graph | 7/10 | 8/10 | 9/10 | 5/10 | **7.0** | SpiritTeamUsecase God Object |
| Memory | 7/10 | 4/10 | 8/10 | 7/10 | **6.5** | 5 实体缺状态机 + Reranker 名不副实 |
| Skill/Evolution | 6/10 | 6/10 | 6/10 | 5/10 | **5.8** | 三条并行进化管线 + 半完成统一 |
| Channel | 9/10 | 9/10 | 8/10 | 8/10 | **8.5** | 最佳设计域 |
| Tool | 8/10 | 7/10 | 8/10 | 7/10 | **7.5** | Tool struct 28 字段超标 |
| Plugin | 8/10 | N/A | 7/10 | 8/10 | **7.5** | Repo 接口 9 方法超标 |
| Hook/Cron | 8/10 | N/A | 8/10 | 8/10 | **8.0** | 设计简洁合理 |
| MCP Server | 8/10 | N/A | 8/10 | 7/10 | **7.5** | ID 生成 fallback 并发安全 |
| Task/Plan | 6/10 | 3/10 | 6/10 | 5/10 | **5.0** | 缺失状态机最严重 |
| 前端 | 8/10 | N/A | 9/10 | 9/10 | **8.5** | AF 架构优秀，数据流合规 |

---

## 二、逐域详细评审

### 2.1 Agent 域（6.5/10）

**业务范围**：Agent CRUD、设置管理、工具策略、Prompt 管理、状态生命周期

#### 亮点

1. **状态机规范**：`AgentStateMachine`（3 状态/3 事件）完全符合 AS-FSM-01，基于 `shared.GenericStateMachine` 泛型实现，含 Mermaid 文档
2. **子域视图模式**：`AgentRuntimeSettings` 的 GetXxx/ApplyXxx 方法对，在保持存储层扁平的同时提供领域视图
3. **接口窄化尝试**：AgentReader(4)/AgentWriter(4)/AgentRuntimeSettingsRepo(2) 等窄接口设计方向正确
4. **DEV-10 修复**：ConfigJSON 不再持久化，改为读路径计算

#### 严重问题

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| AG-1 | AgentUsecase 是 God Object | 🔴 | 21 个公开方法，12 个字段（10 个依赖字段超 AS-COG-01 biz 依赖上限 8，另含 logger + 状态机），承担 CRUD + 水合 + 遗留迁移 + 工具策略 + Token 估算 + 批量操作 |
| AG-2 | AgentRuntimeSettings 139 字段超级结构体 | 🔴 | 承载 8 个子域配置，GetXxx/ApplyXxx 8 对方法约 200 行纯机械映射，极易遗漏 |
| AG-3 | Status 三套类型不一致 | 🟡 | Agent.Status(string) vs AgentStatus(命名类型) vs AgentState(状态机类型)，语义重叠但类型不同；Agent.Status 未使用 AgentStatus 类型，丧失编译期类型安全 |
| AG-4 | L3RecallTopK 默认值不一致 | 🟡 | DefaultAgentRuntimeSettings(L3RecallTopK=5) vs ResolveMemoryRuntimePolicy 零值兜底(L3RecallTopK=12)，两条路径语义不同但文档未说明 |
| AG-5 | 遗留迁移逻辑嵌入 usecase | 🟡 | migrateLegacySettings/migrateLegacyFiles 属于数据迁移关注点，不应与业务编排耦合 |
| AG-6 | GetEffectiveTools/UpdateAgentToolPolicy 逻辑重复 | 🟡 | 两者有大量重叠的"加载→计算→应用"管道 |

#### 改进方案

1. **拆分 AgentUsecase**：优先拆出 `AgentPromptUsecase`（Prompt 文件管理 + Token 估算，边界最干净）。Create/Update/CreateWithFilesAndSettings 横跨 catalog + settings + files 三域，拆分后仍需一个"编排层"协调事务，收益有限。建议保留 `AgentCatalogUsecase` 作为编排入口，仅将 Prompt 文件操作提取为独立 Usecase
2. **统一 Status 类型**：Agent.Status 改为 `AgentState` 类型
3. **默认值收口**：所有默认值定义在 `agent_defaults.go`，`ResolveMemoryRuntimePolicy` 引用而非重定义
4. **长期**：AgentRuntimeSettings 拆分为独立子域实体，通过 AgentID 关联；短期可用代码生成 Get/Apply 方法

---

### 2.2 Session/Run 域（7.0/10）

**业务范围**：Session 生命周期、Run 编排、Turn 执行、上下文压缩、消息管理、指标聚合

#### 亮点

1. **Turn Gateway 接口层次**：按消费者需求拆分为 TurnExecutorGateway/TurnRunControlGateway/TurnControlGateway/DurableResumeGateway/PendingMessageGateway，每个接口方法数 ≤5，是 ISP 最佳实践
2. **SessionRepo 窄接口**：SessionReader(4)/SessionWriter(5)/SessionMutator(4) 等，均标注 Stability 等级
3. **TurnExecutor 接口设计**：7 个正交钩子接口（Admission/Locker/QueueManager/Registry/Tracer/UsageRecorder/Persistence），消除多入口各自为政
4. **SessionRunPhaseMachine**：基于 GenericStateMachine，CAS 乐观锁防竞态
5. **Metrics 增量聚合**：内存聚合 + 200ms 定时 flush，避免高频 DB 写入

#### 严重问题

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| SR-1 | Session 双重状态机 | 🔴 | SessionStateMachine(Event-driven, 5状态/6事件/8规则, GenericStateMachine) vs SessionStatusMachine(Target-driven, 5状态/0事件/5映射, 手写)，两套独立实现描述同一状态域，维护两份转换规则表 |
| SR-2 | 压缩逻辑三层透传 | 🔴 | SessionCompressionUsecase 是纯透传层（11 方法全部单行委托到子依赖），SessionUsecase.summary.go 又做一层 Facade 透传，形成 SessionUsecase→CompressionUsecase→子依赖 三层透传。核心压缩策略（soft/hard trigger、三级级联）在 `internal/session/compressor.go`，biz 层无感知 |
| SR-3 | Run vs SessionRun 状态映射缺失 | 🟡 | RunStateMachine(6状态) 和 SessionRunPhaseMachine(5阶段) 终态重叠但中间态不同，无映射文档 |
| SR-4 | 构造函数内创建子 Usecase | 🟡 | NewSessionUsecase/NewSessionMessageUsecase 内部 New 子 Usecase，Wire 无法独立管理 |
| SR-5 | Metrics flush 无退避策略 | 🟡 | DB 不可用时 delta 反复 flush 失败、重新入队，可能无限循环 |
| SR-6 | Recovery 绕过状态机 | 🟡 | BatchTransitionInterrupted 直接构造 "interrupted" 字符串，绕过 SessionStatusMachine 校验 |

#### 改进方案

1. **统一 Session 状态机**：废弃 SessionStatusMachine，新增 `sessionEventForTarget` 辅助函数（仿 `agentEventForTarget` 模式），将 statusReason/changedAt 管理从状态机移到 `SessionUsecase.TransitionStatus` 调用层。SessionStateMachine 当前零生产调用者，迁移成本低
2. **消除压缩 Facade 透传**：SessionCompressionUsecase 是纯透传层（11 个方法全部单行委托到 repo），且 `SessionUsecase.summary.go` 又做了一层 Facade 透传，形成三层透传（SessionUsecase → SessionCompressionUsecase → Repo）。核心压缩策略在 `internal/session/compressor.go`（依赖 LLM 调用、trpc-agent-go 运行时快照同步等），不适合下沉到 biz 层。实际改进：消除 `summary.go` Facade 透传层，让调用者直接使用 SessionCompressionUsecase
3. **编写 ADR**：明确 RunStateMachine 和 SessionRunPhaseMachine 的关系与使用场景
4. **消除构造函数内子 Usecase 创建**：所有子 Usecase 通过 Wire 注入
5. **Metrics flush 增加退避**：最大重试 3 次 + 指数退避

---

### 2.3 Team/Graph 域（7.0/10）

**业务范围**：Team 编排、Graph 定义/执行、Spirit 多团队调度、DAG 依赖管理

#### 亮点

1. **三台状态机统一范式**：Team(7状态)/TeamRun(6状态)/GraphExecution(5状态) 均基于 GenericStateMachine，含 Mermaid 文档
2. **Team Ports 设计**：约 15 个窄接口 + 编译时断言 + 生命周期钩子链，是项目中最精心设计的依赖倒置层
3. **TeamCompiler**：仅 2 个方法的窄接口，Team→Graph 编译解耦
4. **Graph Facade 分层**：GraphUsecase 组合 DefinitionUsecase + ExecutionUsecase + CacheManager
5. **GraphExecution 并发安全**：execMu + interruptMu 双锁 + SnapshotForPersist 深拷贝
6. **Spirit TaskDAG**：DFS 三色标记法环检测 + 拓扑推断 + 文本可视化

#### 严重问题

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| TG-1 | SpiritTeamUsecase 是 God Object | 🔴 | 1432 行，28 个公开方法，14 个字段（含 1 sync.Map），3 个子域（Assembly/Orchestration/Delivery）未拆分；交付物存储在 ParallelConfigJSON（语义严重混乱）；代码自身已标注 TECH-DEBT(COG) |
| TG-2 | GraphExecution 状态机形同虚设 | 🔴 | 14 处直接 `exec.Status =` 赋值绕过状态机（含 2 处回滚场景），另有 2 处通过构造函数传入初始失败状态；仅 1 处用状态机做前置校验（且校验后仍直接赋值）。状态机定义了合法转换规则但几乎未被遵守，存在非法转换风险；多数直接赋值使用原始字符串（`"failed"`/`"cancelled"`）而非状态机常量，增加拼写错误风险 |
| TG-3 | Team struct 27 字段严重超标 | 🟡 | TECH-DEBT 注释声称 23 字段已过时，实际 27 字段（上限 15，超标 180%），需拆分为 TeamOrgMeta + TeamOrchestrationMeta |
| TG-4 | TeamRun 与 GraphExecution 状态不同步 | 🟡 | 两者独立转换，可能出现不一致 |
| TG-5 | SpiritTeamUsecase 循环依赖 | 🟡 | SpiritTeamUsecase → TimeoutHandler → TeamStarter → SpiritTeamController → SpiritTeamUsecase，通过 SetTimeoutHandler + sync.Once 延迟注入解决 |

#### 改进方案

1. **拆分 SpiritTeamUsecase**（DEV-09）：AssemblyUsecase + OrchestrationUsecase + DeliveryUsecase
2. **交付物存储迁移**（TECH-DEBT #B-03）：从 ParallelConfigJSON 迁移到专用字段
3. **统一状态机使用**：GraphExecutionUsecase 中通过状态机 Transition 方法设置状态。需新增 `graphExecEventForTarget` 辅助函数，并处理回滚场景（2 处 `WaitingHuman→WaitingHuman` 需增加 `rollback` 事件或允许自环转换）
4. **提取 timeoutTimers**：独立为 TeamTimeoutManager，消除 sync.Map
5. **TeamRun/GraphExecution 状态映射**：建立显式映射关系文档

---

### 2.4 Memory 域（6.5/10）

**业务范围**：四层记忆体系（L2 情景/L3 语义/L4 知识图谱）、提取→整合→召回→重排生命周期

#### 亮点

1. **L4 Cascade Saga 模式**：4 步 Saga + 补偿 + CAS 并发控制，是项目中最严谨的分布式事务设计
2. **跨层去重**：DedupL3WithL1 + FactFingerprint(SHA-256) 确保记忆不重复
3. **PII 保护**：10 种 PII 类型检测 + redact/block/review 三种策略
4. **优雅降级**：L2/L3 Recall 在 embedding 失败时自动降级为关键词搜索
5. **L3 多 scope 融合**：agent + workspace + user 三 scope 去重排序

#### 严重问题

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| ME-1 | 5 个实体缺失显式状态机 | 🔴 | MemoryFact(L3 5+状态)、L4Entity(4+状态)、CascadeProposal(5+状态)、SkillEvolutionSuggestion(4+状态)、UnifiedEvolutionSuggestion(5+状态) — 均违反 AS-FSM-01 |
| ME-2 | CrossEncoderReranker 名不副实 | 🔴 | 实际只实现了词级 bigram Jaccard 相似度（注释承认是"lexical proxy until external CE model is wired"），不是 Cross-Encoder，误导召回质量预期 |
| ME-3 | L4 Cascade 单文件过长 | 🟡 | 927 行，正向逻辑与补偿逻辑交织 |
| ME-4 | L4 写入路径策略不清晰 | 🟡 | Path A(正则) 和 Path B(LLM) 的协调逻辑无显式策略控制 |
| ME-5 | Path B WriteEntities best-effort | 🟡 | 错误只记日志不返回，可能导致 L4 数据不完整而调用方不知情 |

#### 改进方案

1. **补全状态机**：为 MemoryFact/L4Entity/CascadeProposal/SkillEvolutionSuggestion/UnifiedEvolutionSuggestion 定义显式状态机
2. **重命名 Reranker**：`CrossEncoderReranker` → `BigramJaccardReranker`，或实现真正的 Cross-Encoder
3. **拆分 L4 Cascade**：补偿逻辑提取为 `cascade_compensator.go`
4. **L4 写入策略显式化**：定义 WriteStrategy 枚举（RegexOnly/LLMOnly/RegexThenLLM/Parallel）
5. **Path B 错误传播**：WriteEntities 返回 error 而非 best-effort

---

### 2.5 Skill/Evolution 域（5.8/10）

**业务范围**：Skill CRUD/进化/去重/合并/评分/健康检测，Evolution 建议/扫描/学习闭环

#### 亮点

1. **五阶段进化循环**：Solve→Observe→Evolve→Gate→Reload，Gate 四维度验证（功能/安全/性能/风格）
2. **Evolution 状态机**：符合 AS-FSM-01，使用泛型实现
3. **Skill 去重**：Union-Find 分组算法 + 四维度加权相似度
4. **Skill 合并**：三阶段（融合→Gate→事务应用）+ 三种策略

#### 严重问题

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| SE-1 | 三条并行进化管线 | 🔴 | SkillEvolutionUsecase + SkillIntelligenceUsecase + SkillEvolutionOrchestrator，职责重叠 |
| SE-2 | Legacy↔Unified 双写桥接 | 🔴 | 8 个桥接转换函数（在 skill_intelligence.go 中）+ bridgeWrite 双写模式，迁移期必要之恶但无移除时间线 |
| SE-3 | EvolutionCoordinator 空壳 | 🟡 | 82 行代码全部委托给 orchestrator（已标注 Deprecated），应直接移除 |
| SE-4 | SkillIntelligenceUsecase 依赖过多 | 🟡 | 9 个依赖超过 AS-COG-01 上限(8)，代码自身已标注 TECH-DEBT(COG) |
| SE-5 | Learning Loop 模式检测过于简单 | 🟡 | 分组键仅为 Kind，同一 Kind 下所有观察无差别合并（如不同工具的 tool_call 被合并为一个模式），缺少时序/异常分析 |
| SE-6 | Skill Dedup O(n²) | 🟡 | 大规模场景(>1000 Skill)可能成为瓶颈 |
| SE-7 | Evolution 状态机与 UnifiedEvolutionSuggestion 不一致 | 🟡 | 状态机定义 4 状态(Pending/Applied/Rejected/RolledBack)，但 UnifiedEvolutionSuggestion.Status 注释列出 5 状态(含 approved/expired)，缺少 approved 和 expired 的转换规则 |

#### 改进方案

1. **统一进化管线**：设定 EvolutionCoordinator 移除时间线，SkillEvolutionUsecase.DetectAndPropose 迁移到 Orchestrator
2. **Legacy 双写移除计划**：设定 2 个迭代后移除 Legacy 存储，消除桥接代码
3. **SkillIntelligenceUsecase 拆分**：提取 SkillHealthChecker 和 SkillScorer 为独立 Usecase
4. **Learning Loop 增强**：引入时序窗口 + 异常检测（Z-score）
5. **Dedup 优化**：引入 MinHash/LSH 近似去重，O(n²) → O(n)

---

### 2.6 Channel 域（8.5/10）— 最佳设计域

**业务范围**：IM 通道接入、路由、入站去重/防抖、Turn Job 管理、并发限流、凭证加密、出站投递

#### 亮点

1. **三层漏斗模型**：AccessPolicy → IngressRules → RouteMatching → TargetResolution，职责分离彻底
2. **ChannelTurnJobStateMachine**：8 状态/11 事件/16 转换规则，项目状态机标杆
3. **纯函数优先**：MatchRoute/ParseChannelRouting/IngressMessageDedupeKey 等无副作用，极易测试
4. **并发限流门**：进程内令牌桶 + stale entry 安全网
5. **凭证加密**：enc: 前缀标识，委托 CredentialCrypto

#### 小问题

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| CH-1 | channel_config_helpers.go 过于膨胀 | 🟡 | 327 行，混合长任务配置/异步关键词/流式开关/微信模式/模板渲染 |
| CH-2 | ChannelUsecase 9 个注入字段 | 🟡 | 接近上限 60% |
| CH-3 | ChannelTurnJobStatusFromEvent 冗余 | 🟢 | 与状态机转换规则存在冗余定义 |

#### 改进方案

1. 拆分 `channel_config_helpers.go` 为 `channel_long_task_config.go` + `channel_async_config.go`
2. 提取便利方法（GetTeamByID/AgentKeyResolver）到独立 Helper

---

### 2.7 Tool 域（7.5/10）

**业务范围**：工具目录管理、策略计算、配置校验、熔断器、敏感信息脱敏、结果持久化门

#### 亮点

1. **Catalog/Policy/Validation 分离清晰**：运行时状态、策略键规范化、输入验证、配置验证各自独立
2. **熔断器设计成熟**：Closed/Open/HalfOpen 三态 + 状态持久化/恢复 + 可配置阈值
3. **安全防护到位**：MCP URL SSRF 校验、敏感信息脱敏、高风险工具 I_UNDERSTAND_RISK 确认
4. **ToolResultGate**：超阈值自动 blob 化 + 预览截断

#### 问题

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| TL-1 | Tool struct 34 字段超标 | 🟡 | 运行时计算字段与持久化字段混合 |
| TL-2 | ToolRepo 复合接口 14+ 方法 | 🟡 | 已知债务 DB-DEBT-05 |
| TL-3 | 硬编码 tool key 集合 | 🟡 | registryBackedToolKeys/sessionBoundToolKeys 需手动维护 |

#### 改进方案

1. 拆分 Tool struct 为 `ToolCatalog`(持久化) + `ToolRuntimeView`(运行时计算)
2. 硬编码 tool key 改为从 catalog 表动态查询或注册机制

---

### 2.8 Task/Plan 域（5.0/10）— 最需改进

**业务范围**：任务生命周期管理、计划编排、心跳/租约、动态分配

#### 严重问题

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| TP-1 | Task 10 种状态无显式状态机 | 🔴 | 状态转换逻辑散落在各方法的 `if task.Status != X` 检查中（如 ClaimTask/SubmitTaskResult/ReportBlocked/UnblockTask/ReviewTask），违反 AS-FSM-01 |
| TP-2 | Plan 6 种状态未使用 GenericStateMachine | 🔴 | 手写 `validPlanTransitions` map + `canTransitionPlan` 线性查找，缺 Mermaid 图/事件枚举/Transition 方法，部分满足 AS-FSM-01 但不完整 |
| TP-3 | OrchestrationStatus 6 种状态无状态机 | 🔴 | 同上 |
| TP-4 | TaskUsecase 16 字段 + sync.RWMutex+map | 🟡 | 使用 sync.RWMutex + map[string]time.Time 管理 heartbeats/leaseDeadline，虽功能正确但与项目 sync.Map 规范不一致，应提取为独立 TaskHeartbeatManager |
| TP-5 | TaskRepo 复合接口 11 方法 | 🟡 | 超出建议上限 |

#### 改进方案

1. **定义 TaskStateMachine**：10 状态/约 12 条转换规则，基于 GenericStateMachine。条件转换（如 `Claimed→Complete` vs `Claimed→ReviewRequired` 取决于是否有 ReviewerAgent）通过拆分事件（`submit_complete`/`submit_review`）处理，由调用者根据业务条件选择事件
2. **定义 PlanStateMachine**：6 状态/6 事件，替换手写 map
3. **定义 OrchestrationStateMachine**：6 状态/6 事件
4. **提取 TaskHeartbeatManager**：独立管理 heartbeats + leaseDeadline，消除 sync.Map
5. **TaskRepo 拆分**：TaskReader(5)/TaskWriter(5)/TaskLifecycleManager(3)

---

### 2.9 其他支撑域速评

| 域 | 评分 | 亮点 | 问题 |
|----|------|------|------|
| Plugin | 7.5 | 沙箱三级/版本策略/CostGuard | Repo 9 方法超标，缺全量更新方法 |
| Hook/Cron | 8.0 | 类型重导出模式一致，CronTriggerGateway 解耦 | 无显著问题 |
| MCP Server | 7.5 | SSRF 防护/健康探测/重连元数据 | ID 生成 fallback 并发安全，缺 Logger |
| Knowledge | 7.0 | 8 种领域错误/稀疏搜索 | 纯重导出，子包设计待深入 |
| Evaluation | 7.0 | 完整评测生命周期 | 纯重导出 |
| Usage | 7.0 | 分析/写入/配额三接口分离 | 纯重导出 |
| Artifact | 7.5 | 4 种预览类型/5 种领域错误 | 设计简洁 |
| A2A | 7.5 | 本地/远程双源/Agent Card/审计 | 纯重导出 |
| Organization | 7.0 | 7 步级联删除/事件发布 | 级联删除无事务保护 |
| SystemSetting | 7.0 | 单例配置/子配置/配额同步 | Update 方法 6 参数 |
| EventStore | 7.0 | TTL 配置/WAL 幂等 | TTL 直接读环境变量 |
| LlmProviderModel | 7.5 | 接口窄化/定价源优先级/SSRF 防护 | RunHealthChecks 方法过长 |
| Monitor | 7.0 | 审计/告警/自愈/诊断 | 子包设计完整但复杂度高 |

---

### 2.10 前端（8.5/10）

**业务范围**：Chat 实时通信、Agent/Session/Team 管理、消息分组、Activity-First 架构

#### 亮点

1. **AF 架构**：Activity-First 消息分组使用 turn_id Stack 模型，零推理路径 + Legacy 回退，设计严谨
2. **数据流合规**：API→Store→Composable→Page→Component 链路完整，组件层与 API 层完全隔离
3. **Chat Store 拆分**：从单一 Store 拆为 SessionStore/MessageStore/ConversationStore，职责边界清晰
4. **sessionSync 事件总线**：跨 Store 通信避免循环依赖
5. **mergeSessionMessages**：6 种消息来源合并逻辑完整，35 个单测覆盖

#### 问题

| # | 问题 | 严重度 | 说明 |
|---|------|--------|------|
| FE-1 | useChatWorkspace 1122 行偏大 | 🟡 | 主编排 Composable 仍包含压缩轮询/停滞检测等可提取逻辑 |
| FE-2 | SpiritTeamStore 621 行偏大 | 🟡 | 混合 API 调用 + WS 事件处理 + UI 状态 |
| FE-3 | Domain 层过于单薄 | 🟡 | 跨 Feature 共享类型仍分散在各 Feature 的 types.ts |
| FE-4 | Store 错误处理不一致 | 🟡 | 部分用 error.value，部分 throw，部分 catch 后静默 |
| FE-5 | 部分 Feature 混入 Vue 组件 | 🟢 | artifact/channels/mcp/memory 的 Feature 目录含 .vue 文件 |

#### 改进方案

1. 提取 useChatWorkspace 中的压缩轮询和停滞检测为独立 Composable
2. SpiritTeamStore 的 WS 事件处理提取为 `spiritEnvelopeHandler.ts`
3. 逐步将跨 Feature 共享类型提升到 `domain/types.ts`
4. 统一 Store 错误处理策略

---

## 三、系统性问题汇总

### 3.1 状态机覆盖不足（最严重）

按 AS-FSM-01（>3 种状态必须定义显式状态机），以下实体缺失状态机：

| 实体 | 状态数 | 当前实现 | 优先级 |
|------|--------|---------|--------|
| Task | 10 | 散落在 if 检查中 | P0 |
| Plan | 6 | 手写 map 查找 | P0 |
| OrchestrationStatus | 6 | 无 | P0 |
| MemoryFact(L3) | 5+ | 无 | P1 |
| L4Entity | 4+ | 无 | P1 |
| CascadeProposal | 5+ | 无 | P1 |
| SkillEvolutionSuggestion | 4+ | 散落在方法中 | P1 |
| UnifiedEvolutionSuggestion | 5+ | 隐含在 orchestrator | P1 |

### 3.2 双重状态机

| 实体 | 状态机 A | 状态机 B | 问题 |
|------|---------|---------|------|
| Session | SessionStateMachine (Event-driven, GenericStateMachine) | SessionStatusMachine (Target-driven, 手写) | 两套独立实现描述同一状态域 |

### 3.3 God Object 清单

| 实体 | 方法数/行数 | 依赖字段 | 应拆分为 |
|------|-----------|---------|---------|
| AgentUsecase | 21 方法 | 10（超 biz 依赖上限 8） | CatalogUsecase + ToolPolicyUsecase + PromptUsecase + LegacyMigrator |
| SpiritTeamUsecase | 1432 行/28 方法 | 14（含 1 sync.Map） | AssemblyUsecase + OrchestrationUsecase + DeliveryUsecase |
| SessionUsecase | Facade 委托链过深 | 10 | 调用者直接依赖子 Usecase |

### 3.4 超级结构体

| 实体 | 字段数 | 上限 | 超标倍数 |
|------|--------|------|---------|
| AgentRuntimeSettings | 139 | 15 | 9.3x |
| Session | 52 | 15 | 3.5x |
| Tool | 34 | 15 | 2.3x |
| Team | 27 | 15 | 1.8x |
| Agent | 36 | 15 | 2.4x |

### 3.5 状态机使用不一致

| 场景 | 正确做法 | 实际做法 |
|------|---------|---------|
| GraphExecution 状态转换 | 通过状态机 Transition | 14 处直接 `exec.Status =` 赋值绕过状态机（含 2 处回滚场景），仅 1 处用状态机做前置校验（且校验后仍直接赋值）；多数使用原始字符串而非状态机常量 |
| Session Recovery | 通过状态机校验 | 直接构造 "interrupted" 字符串 |
| Team Rejection | 通过状态机 recover 事件 | 直接 TransitionStatus(ctx, teamID, Pending) |

---

## 四、改进路线图

### Phase 0：紧急修复（1 周）

| 改动 | 影响域 | 风险 |
|------|--------|------|
| 统一 Session 状态机（废弃 SessionStatusMachine） | Session | 中 |
| GraphExecution 状态转换走状态机（14 处直接赋值改为 Transition 调用 + 原始字符串改为状态机常量） | Graph | 中 |
| Session Recovery 走状态机校验 | Session | 低 |
| CrossEncoderReranker 重命名 | Memory | 低 |

### Phase 1：补全状态机（2 周）

| 改动 | 影响域 |
|------|--------|
| 定义 TaskStateMachine (10 状态) | Task |
| 定义 PlanStateMachine (6 状态) | Plan |
| 定义 OrchestrationStateMachine (6 状态) | Task |
| 定义 MemoryFactStateMachine (5 状态) | Memory |
| 定义 L4EntityStateMachine (4 状态) | Memory |
| 定义 CascadeProposalStateMachine (5 状态) | Memory |
| 定义 SkillEvolutionSuggestionStateMachine (4 状态) | Skill |
| 定义 UnifiedEvolutionSuggestionStateMachine (5 状态) | Skill |

### Phase 2：拆分 God Object（3 周）

| 改动 | 影响域 |
|------|--------|
| 拆分 SpiritTeamUsecase (DEV-09) | Team |
| 交付物存储迁移 (TECH-DEBT #B-03) | Team |
| 拆分 AgentUsecase（优先拆出 PromptUsecase） | Agent |
| 统一 Agent Status 类型 | Agent |
| 提取 TeamTimeoutManager | Team |

### Phase 3：架构改进（4 周）

| 改动 | 影响域 |
|------|--------|
| 消除压缩 Facade 透传层（summary.go） | Session |
| 统一 Skill 进化管线 | Skill |
| Legacy 双写移除 | Skill |
| AgentRuntimeSettings 子域实体化 | Agent |
| Tool struct 拆分 | Tool |
| 前端 Domain 层完善 | Frontend |

---

## 五、设计原则建议

基于本次评审发现的系统性模式，建议补充以下设计原则：

### 原则 1：状态机唯一出口

> 所有实体状态转换必须通过状态机 Transition 方法，禁止直接赋值 Status 字段。

检测方法：`grep -rn "\.Status\s*=" internal/biz/` 排除状态机文件

### 原则 2：单一状态域单一状态机

> 同一实体的同一维度状态只允许存在一个状态机实现。

检测方法：同一实体名 + "StateMachine"/"StatusMachine" 不应出现两个文件

### 原则 3：Usecase 方法数上限

> 单个 Usecase 公开方法数不超过 15。超过必须拆分子 Usecase。

### 原则 4：默认值单一真相源

> 所有默认值定义在 `xxx_defaults.go`，其他位置引用而非重定义。

### 原则 5：命名必须反映实现

> 接口/类型命名必须准确反映其实现能力。如 `CrossEncoderReranker` 必须实现 Cross-Encoder，否则重命名。

---

## 六、结论

Aranea-Agents 的业务逻辑设计在架构方向上是正确的——接口窄化、状态机显式化、子域视图、依赖倒置等模式得到了广泛应用。但执行深度不均：Channel 域和 Team Ports 层达到了教科书级设计，而 Task/Plan 域和 Skill 进化管线仍处于"方向正确但未完成"的状态。特别值得注意的是，GraphExecution 虽然定义了完整的状态机，但 14 处直接赋值绕过了它（另有 2 处构造函数绕过），使状态机形同虚设——这是"有规范但不执行"的典型表现。

**最需优先解决的三个问题**：
1. **状态机覆盖不足与形同虚设**：8 个实体缺失显式状态机 + GraphExecution 状态机被绕过，是最大的合规风险
2. **God Object**：3 个 Usecase 承担过多职责，是认知复杂度的主要来源
3. **双重状态机**：Session 域的两套状态机是数据不一致的定时炸弹

**设计最优秀的三个模块**：
1. **Channel 域**：三层漏斗 + 纯函数 + 标杆状态机
2. **Turn Gateway 接口层次**：ISP 最佳实践
3. **前端 AF 架构**：Activity-First 消息分组，零推理路径

---

## 附录：v1.1 修订记录

基于源码交叉验证，以下数据从 v1.0 修正：

| 项 | v1.0 原值 | v1.1 修正值 | 修正原因 |
|----|----------|-----------|---------|
| AgentUsecase 公开方法数 | 31 | **21** | 源码精确计数，含 UpsertByKey/CreateWithFilesAndSettings/BatchUpdateAgents |
| AgentRuntimeSettings 字段数 | ~140 | **139** | 源码精确计数 |
| Agent struct 字段数 | ~30 | **36** | 源码精确计数 |
| Team struct 字段数 | 23 | **27** | TECH-DEBT 注释过时，实际新增 4 字段 |
| Task 状态数 | 9 | **10** | 源码含 TaskStatusPendingAssignment |
| TaskUsecase 字段数 | 12 + 2 sync.Map | **16 + 0 sync.Map** | 实际用 sync.RWMutex + map，非 sync.Map |
| SpiritTeamUsecase 行数 | 1431 | **1432** | 源码精确行数 |
| SpiritTeamUsecase 方法数 | 25+ | **28** | 源码精确计数 |
| SpiritTeamUsecase 字段数 | 未声明 | **14（含 1 sync.Map）** | 源码精确计数 |
| GraphExecution 状态绕过 | 1 处提及 | **14 处直接赋值（含 2 处回滚），2 处构造函数传入初始失败状态，仅 1 处校验（且校验后仍直接赋值）** | 严重程度远超 v1.0 描述 |
| DefaultCompressionBufferRatio | 声称重复定义 | **仅定义一次**（agent_types.go），agent_defaults.go 引用 | 验证后纠正 |
| CrossEncoderReranker | bigram Jaccard | **词级 bigram Jaccard**（注释承认是 lexical proxy） | 更精确描述 |
| Evolution 状态机 | 未提及不一致 | **4 状态 vs UnifiedEvolutionSuggestion 5 状态** | 新增发现 |
| Learning Loop | "只按 Kind 分桶计数" | 分组键仅为 Kind，但有置信度阈值/去重/描述生成 | 修正过度简化 |
| 缺失状态机实体数 | 5 | **8** | 含 Task/Plan/OrchestrationStatus |
| Tool struct 字段数 | 未声明 | **34** | 源码精确计数（含运行时计算字段） |
| Session struct 字段数 | 未声明 | **52** | 源码精确计数（非 69） |

---

## 附录2：v1.2 修订记录（改进方案可行性验证）

基于源码深度验证，以下改进方案从 v1.1 修正：

| 项 | v1.1 方案 | v1.2 修正 | 修正原因 |
|----|----------|-----------|---------|
| Session 状态机统一 | "TransitionTo 语义通过事件查找实现" | 新增 `sessionEventForTarget` 辅助函数，将 statusReason/changedAt 管理从状态机移到调用层 | SessionStatusMachine 提供了 reason/changedAt 追踪，统一后需在调用层补偿 |
| 压缩策略收归 biz 层 | "在 SessionCompressionUsecase 中实现触发条件、轮次选择、完整性校验" | **撤销**。核心压缩策略在 `internal/session/compressor.go`，依赖 LLM 调用和 trpc-agent-go 运行时快照同步，不适合下沉到 biz 层。改为消除 `summary.go` Facade 透传层 | Compressor 依赖重（compress.Compressor/Runtime/MemoryResync/EventBus），与运行时耦合深 |
| AgentUsecase 拆分 | 拆为 4 个子 Usecase | 优先拆出 `AgentPromptUsecase`（边界最干净），其余暂缓 | Create/Update/CreateWithFilesAndSettings 横跨三域，拆分后仍需编排层协调事务，收益有限 |
| GraphExecution 状态机强制 | "通过状态机 Transition 方法设置状态" | 需新增 `graphExecEventForTarget`，并处理 2 处回滚场景（`WaitingHuman→WaitingHuman` 需增加 `rollback` 事件或自环转换）；原始字符串赋值需改为状态机常量；构造函数 `NewGraphExecution` 需增加状态值校验 | 验证发现 14 处直接赋值（含 2 处回滚）+ 2 处构造函数绕过 + 多处原始字符串硬编码，问题比 v1.1 评估更严重 |
| TaskStateMachine | "10 状态/10+ 事件" | 10 状态/约 12 条转换规则，条件转换通过拆分事件（`submit_complete`/`submit_review`）处理 | 验证发现 `Claimed→Complete` vs `Claimed→ReviewRequired` 取决于外部条件 |

---

## 附录3：v1.3 修订记录（评审交叉验证）

基于源码深度交叉验证，以下数据从 v1.2 修正：

| 项 | v1.2 原值 | v1.3 修正值 | 修正原因 |
|----|----------|-----------|---------|
| AgentUsecase 公开方法数 | 20 | **21** | 源码精确重计数，含 UpsertByKey/CreateWithFilesAndSettings/BatchUpdateAgents |
| AgentUsecase 依赖字段描述 | "12 个依赖字段" | **12 个字段（10 个依赖字段 + logger + 状态机）** | logger 和状态机不属于 AS-COG-01 依赖计数，但 10 个依赖字段仍超上限 8 |
| GraphExecution 状态绕过 | 11 处直接赋值 | **14 处直接赋值（含 2 处回滚）+ 2 处构造函数绕过** | 排除初始创建后的精确计数，问题比 v1.2 评估更严重 |
| Tool struct 字段数 | 28 | **34** | 源码精确计数，含 RuntimeStatus/RuntimeKind/LastInvokedAt/LastStatus/DeletedAt/Permissions 等 |
| Session struct 字段数 | 69 | **52** | 源码精确计数，v1.2 可能误将 Ent 生成模型字段数当作 biz 层字段数 |
| 超级结构体表 | 缺少 Agent struct | 新增 **Agent 36 字段（2.4x）** | Agent struct 36 字段超标但未列入 3.4 节 |
| SessionCompressionUsecase | "11 方法全部单行委托到 repo" | **11 方法全部单行委托到子依赖** | 部分委托到 contextUpdater/summaryWriter/summaryReader，非单一 repo |
| GraphExecution 原始字符串 | 未提及 | **新增发现**：多数直接赋值使用原始字符串（`"failed"`/`"cancelled"`）而非状态机常量 | 增加拼写错误风险，需在改进方案中明确替换为常量 |
