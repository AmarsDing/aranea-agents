# Aranea-Agents 全模块深度架构分析

> 日期：2026-06-11 | 类型：analysis | 范围：全项目模块

---

## 一、项目全景

Aranea-Agents 是基于 **trpc-agent-go 运行时 + Kratos v2 传输壳层** 的多智能体编排平台。双框架分工明确：

- **Kratos v2**：HTTP/gRPC/WebSocket 传输、配置、鉴权、中间件、Wire DI
- **trpc-agent-go**：Agent 编排（Runner/Agent/Session/Memory/Tool/Event/Skill/Graph/Team）

代码规模：后端 ~600+ 文件（internal/ + pkg/），前端 ~200+ 文件（web/），覆盖 Agent/Team/Graph/Memory/Skill/Tool/Channel/A2A/Evaluation 等完整业务域。

---

## 二、各模块功能完善度评分

### 2.1 后端分层

| 层级 | 完善度 | 核心优势 | 关键短板 |
|------|--------|----------|----------|
| **biz 层** | 85% | 端口/适配器架构成熟，窄接口+编译时断言，领域模型丰富 | ConfigJSON 双向写入遗留，4套进化体系重叠，胖接口未拆分 |
| **service 层** | 86% | 大部分服务严格遵循 proto 映射模式，Wire DI 合规 | ChatOrchestrator 上帝对象，Channel 子系统业务逻辑泄漏 |
| **data 层** | 90% | 读写分离优雅，事务管理精良，级联删除完善 | 267处 kerrors 分层违规，双轨 Schema 管理，pgvector 旧包未迁移 |

### 2.2 运行时模块

| 模块 | 完善度 | 核心优势 | 关键短板 |
|------|--------|----------|----------|
| **Agent 运行时** | 82% | L1-L4 四级 prompt 注入管线完整，工具装配覆盖全品类 | TRPCBuilderDeps 30+ 字段膨胀，ModelSelector 仅 auto 一种策略 |
| **Session 运行时** | 78% | 三级压缩级联设计精巧（Micro→Memory→LLM），CAS 乐观并发 | Compressor 职责过重（700+ 行），压缩策略配置分散 |
| **Team 运行时** | 75% | 6种编排模式完整，失败策略三级容错，Mediator 解循环依赖 | 执行路径碎片化（5-6层委托），Swarm 模式成熟度不足 |
| **Skill 运行时** | 72% | 导入管线完整（含补偿式事务），双渲染模式，文件系统对账 | 相似度检测成本高（50次LLM调用），5分钟轮询不够实时 |
| **Graph 运行时** | 82% | Checkpoint/TimeTravel 完整，子图嵌套，DOT 可视化 | runtime 字段类型不安全，GC 参数硬编码，Subgraph 递归无深度限制 |

### 2.3 基础设施

| 模块 | 完善度 | 模块 | 完善度 |
|------|--------|------|--------|
| event/ 事件总线 | 85% | provider/ 模型路由 | 72% |
| server/ 传输层 | 80% | outbound/ 出站消息 | 80% |
| runtime/ 运行时网关 | 82% | knowledge/ 知识库 | 78% |
| compress/ 压缩服务 | 78% | modelregistry/ 模型注册 | 85% |
| metrics/ 指标 | 70% | mcp/ MCP管理 | 75% |
| telemetry/ 遥测 | 75% | evaluation/ 评估 | 70% |
| debug/ 调试 | 80% | llmcontext/ 上下文 | 75% |

### 2.4 前端

| 功能域 | 完善度 | 功能域 | 完善度 |
|--------|--------|--------|--------|
| UX 主题系统 | **95%** | 聊天消息分组 | **92%** |
| WebSocket 实时通信 | 90% | API 层 | 88% |
| Agent 管理 | 88% | 用量统计 | 80% |
| 团队编排 | 85% | 监控 | 78% |
| 图编辑器 | 82% | 知识库 | 70% |
| 评估 | 65% | A2A | 72% |

### 2.5 pkg 公共包

| 包 | 完善度 | 包 | 完善度 |
|----|--------|----|--------|
| apierror | 92% | logpipeline | 90% |
| appctx | 95% | outboundwebhook | 90% |
| loggateway | 88% | auth | 85% |
| webhookurl | 88% | validate | 85% |
| outboundguard | 82% | ctxuser | 88% |
| jsonutil | 75% | strutil | 78% |

---

## 三、模块间业务联系合理性评估

### 3.1 合理的方面

1. **依赖方向正确**：service → biz → data，运行时层 → biz 接口层，无循环依赖
2. **双框架桥接集中**：`internal/agent/trpc_build.go` 和 `trpc_runtime.go` 是 Kratos 与 trpc-agent-go 的唯一桥接层
3. **事件驱动解耦**：event.Bus 统一了实时推送、Webhook 回调、监控告警的事件管道
4. **窄接口隔离**：biz 层广泛使用 Reader/Writer/Mutator 等窄接口，编译时断言确保一致性
5. **前端三层 WS 架构**：transport → hub → dispatcher 分层清晰

### 3.2 不合理的方面

| 问题 | 严重度 | 影响 | 状态 |
|------|--------|------|------|
| **ChatOrchestrator 上帝对象**：20+ 依赖字段，同时承担传输适配和业务编排 | 严重 | service 层业务逻辑泄漏，难以测试和维护 | 🟡 部分修复：deps 分组+子管理器提取+TurnAdmissionUsecase/TurnLifecycleUsecase 下沉，sessionContextPressure 已完全委托 biz 层（含 channel 入口点），chatSessionStateMgr 已删除，enforceQuota 已下沉，但 ChatOrchestrator 仍有 36 字段 |
| **4套进化体系重叠**：Evolution / SkillEvolution / SkillIntelligence / LearningLoop | 中等 | 概念重叠，维护成本高，开发者困惑 | 🟡 部分修复：SkillEvolutionOrchestrator 统一编排器已建，EvolutionCoordinator 已 deprecated，但 DEV-04 未关闭 |
| **Hook vs Webhook 概念重叠**：两个模块功能高度相似 | 中等 | 重复实现，投递重试策略不一致 | 🟡 无需合并：职责已分化（Hook=平台内部回调，Webhook=外部HTTP通知），领域模型和接口完全不同 |
| **provider 与 modelregistry 职责重叠**：模型解析逻辑分散在两处 | 中等 | 边界模糊，修改需同步两处 | 🟡 无需合并：职责已分化（LlmProviderModel=DB配置管理，ModelRegistry=文件系统策略管理），无直接接口重叠 |
| ~~**safego 反向依赖**：pkg/safego 依赖 internal/metrics~~ | ~~轻微~~ | ~~违反 pkg 不依赖 internal 的分层原则~~ | ✅ 已修复 |
| ~~**outboundguard 与 webhookurl 逻辑重复**：IP 私有地址检查逻辑重复~~ | ~~轻微~~ | ~~维护时需同步修改~~ | ✅ 已修复：webhookurl 委托 outboundguard |

---

## 四、业务逻辑清晰度评估

| 清晰度 | 模块 |
|--------|------|
| **清晰** | Agent 类型/设置、Session 状态机、Graph 门面、Memory L0-L4、Channel 路由/投递、Cron/Hook/Plugin、apierror/logpipeline/auth、前端主题/WS/消息分组 |
| **一般** | Skill 进化（两套体系并存）、Team 运行时（执行路径碎片化）、provider/modelregistry（职责重叠）、metrics（覆盖面有限）、evaluation（存储与框架级关系不明确） |
| **模糊** | Organization（职责混合、文件过长）、LearningLoop vs Evolution（概念边界不清） |

---

## 五、各模块详细分析

### 5.1 biz 层核心模块

#### Agent 相关

- **核心职责**：Agent CRUD、运行时配置水合（Settings + Files → ConfigJSON）、Agent 类型区分（LLM vs A2A 代理）、运行时端口抽象
- **完善度**：85%
- **优势**：7 域子结构体拆分清晰（Identity/Reasoning/Memory/Tools/Skills/Evolution/Context）；AgentRuntimeSettings 扁平结构 + 域视图访问器设计合理；遗留 ConfigJSON 迁移逻辑健壮
- **关键缺失**：
  1. ConfigJSON 写路径遗留：Settings + Files → ConfigJSON 的投影应改为只读，消除双向写入不一致风险（代码中 TODO DEV-10 标注） ✅ 已修复：DEV-10 FIXED
  2. ~~AgentRepository 胖接口（14+ 方法）未拆分为窄接口~~ ✅ 已修复：拆为 8 个窄接口
  3. AgentCapability.Capacity 未使用（TODO DEV-03 标注），无冲突检测/负载均衡
  4. 缺少 Agent 状态机：Status 字段为自由字符串，无状态转换约束

#### Session 相关

- **核心职责**：会话运行状态机、Turn 生命周期编排、Chat 会话互斥与待处理队列、入口策略评估
- **完善度**：80%
- **优势**：SessionRun 状态机完整（含 durable resume claim + orphaned run 清理）；Turn 生命周期钩子设计精良（6 个独立接口解耦关注点）；IngressPolicy 为纯函数
- **关键缺失**：
  1. awaitChans 内存泄漏风险：仅靠 5 分钟 GC ticker，高并发场景下可能积压大量 channel
  2. TurnExecutor 接口定义在 biz 但实现在 service（ChatOrchestrator），biz 层缺少编排逻辑的测试覆盖
  3. SessionRun 缺少超时自动降级：HardBudgetSec 到期后无自动 Fail 机制

#### Team 相关

- **核心职责**：Team 定义管理、编排模式验证（6 种模式）、Team-Graph 双向关联、运行生命周期（含死信队列）
- **完善度**：88%
- **优势**：6 种编排模式（sequential/parallel/coordinator/critic_loop/swarm/adaptive）均有角色兼容性验证；Team 状态机转换约束完整；Team-Graph 双向关联同步（syncGraphTeamID）
- **关键缺失**：
  1. validateTeamMembersExist 仅检查存在性，不检查 active 状态
  2. ExportStructure 仅覆盖 3 种模式的结构生成
  3. DefinitionJSON 解析重复：3 个不同的 JSON 解析结构体，应统一

#### Graph 相关

- **核心职责**：Graph 定义 CRUD、执行生命周期（含 checkpoint/time-travel）、可视化、模板管理、版本回滚
- **完善度**：90%
- **优势**：GraphUsecase 门面模式拆分为 DefinitionUsecase + ExecutionUsecase + CacheManager；GraphBuildConfig 类型丰富（Node/Edge/ConditionalEdge/Subgraph/StateField）
- **关键缺失**：
  1. ~~GraphExecution 的 runtime 字段为 interface{}，缺少类型安全~~ ✅ 已修复：已类型化为 `GraphRuntime` 接口（组合 GraphExecutionControl + GraphCheckpoint）
  2. ~~GC 参数硬编码（gcInterval / executionMaxAge / maxExecutions），应支持配置覆盖~~ ✅ 已修复：通过环境变量 GRAPH_GC_* 注入
  3. SubgraphDef 嵌套深度无限制，可能递归过深导致栈溢出

#### Memory 相关

- **核心职责**：5 层记忆系统（L0 上下文窗口 → L1 工作记忆 → L2 情景 → L3 语义 → L4 知识图谱），含去重、PII 脱敏、策略审计、重排序
- **完善度**：82%
- **优势**：PII 检测覆盖 9 种模式；去重指纹计算为单源真相（FactFingerprint）；策略审计引擎支持 strict/best-effort 两种模式；L4 知识图谱含衰减/强化/归档
- **关键缺失**：
  1. CrossEncoderReranker 仅为 bigram Jaccard 代理，未接入真实 Cross-Encoder 模型
  2. PII 检测为正则匹配，无法检测语义级 PII
  3. L4 DecayConfig 半衰期硬编码，不支持动态调整

#### Skill 相关

- **核心职责**：技能生命周期管理、进化（观察→提议→审批→注册）、健康监控、合并（AI 融合/手动/追加）、相似度检测与冲突预防
- **完善度**：78%
- **优势**：SkillEvolutionUsecase 实现了完整的 DetectAndPropose → Approve → Register 流水线；SkillMergeUsecase 三阶段合并设计精良；SkillSimilarityEngine 多维加权评分 + 冲突分级
- **关键缺失**：
  1. SkillEvolution 与 SkillIntelligence 两套进化体系并存，SuggestionFromProposal 为过渡桥接（TODO DEV-04）
  2. SkillSimilarityEngine 默认仅 Jaccard，Embedding 为可选增强但未默认启用
  3. SkillDedup 候选发现逻辑缺失

#### A2A 相关

- **核心职责**：Agent-to-Agent 协议（AgentCard/Invocation/Audit）、远程代理发现与注册、调用限流
- **完善度**：80%
- **关键缺失**：
  1. ~~限流器为内存实现，多 Pod 部署无法共享状态（TODO DEV-08 标注）~~ ✅ 已修复：Redis 分布式限流器已实现并接入 Wire，旧内存限流器已删除
  2. 缺少 A2A 调用的超时/重试策略
  3. AuthType/AuthConfigJSON 定义了但 biz 层无验证逻辑

#### Channel 相关

- **核心职责**：IM 渠道管理（30+ 平台）、凭证加密存储、路由规则匹配、出站投递（含重试/死信）、对等会话绑定
- **完善度**：88%
- **关键缺失**：
  1. ~~ChannelRepo 胖接口（Reader + Writer + Credential + Delivery）未拆分~~ ✅ 已修复：拆为 ChannelReader/Writer + 独立子 Repo
  2. 健康检查并发度硬编码为 8
  3. 渠道类型 catalog 为硬编码列表，应支持动态注册

#### 其他 biz 模块

| 模块 | 完善度 | 关键缺失 |
|------|--------|---------|
| Cron | 75% | 缺少任务依赖关系定义、任务优先级 |
| Hook | 80% | 投递重试策略与 Channel Delivery 重复实现 |
| Monitor | 88% | 告警规则热更新机制 |
| Usage | 82% | 实时用量仪表盘数据推送 |
| Knowledge | 78% | 文档分块策略可配置化、增量更新 |
| Evolution | 80% | ApplySuggestion 缺少回滚机制 |
| Organization | 72% | 职责混合、文件过长；缺组织架构变更事件 |
| Spirit | 75% | DAG 执行引擎未在 biz 层实现、综合输出质量评估 |
| Plan/Planner | 70% | 计划步骤执行编排、计划与 Graph 的关联 |
| RalphLoop | 65% | 仅配置验证函数，无执行逻辑 |
| FailurePolicy | 80% | 熔断器状态持久化 |
| LLMCaller | 78% | 缺少流式输出支持 |
| MCPServer | 72% | MCP 工具发现与自动注册 |
| ModelRegistry | 70% | 模型能力声明、自动发现 |
| Webhook | 78% | 与 Hook 模块概念重叠，应统一 |
| LearningLoop | 72% | 与 Evolution/SkillEvolution 概念重叠 |
| Evaluation | 78% | 评估结果持久化 |

### 5.2 service 层核心问题

#### ChatOrchestrator 上帝对象（严重，持续改善中）

ChatOrchestrator 及其 ChatOrchestratorDeps 是项目最大的架构债务：

1. ChatOrchestratorDeps 含 20+ 字段，代码注释自身标注了 `TECH-DEBT(BL8)`
2. ~~直接持有 15+ 个 biz Usecase 引用~~ → 当前持有 10 个直接 biz Usecase 引用，已部分收敛
3. ~~`RunNativeAgentTurnWithOutcome` 方法长达数百行，包含 turn 准入检查、上下文压力计算、session 状态转换、事件发布、指标记录等——这些逻辑应在 biz 层~~ → turn 准入已下沉到 TurnAdmissionUsecase，session 状态转换已下沉到 TurnLifecycleUsecase，sessionContextPressure 已完全委托 biz 层（含 channel 入口点分支）
4. ~~`sessionLockManager` 属于并发控制基础设施，放在 service 层不合适~~ → 已删除
5. ~~`enforceQuota` / `enforceChatTurnQuotas` 是业务规则，应下沉到 biz 层~~ → 已下沉到 TurnAdmissionUsecase.EnforceChatTurnQuotas

**剩余问题**：ChatOrchestrator 仍有 36 字段（AS-COG-01 上限 15），需继续提取子管理器。

#### Channel 子系统业务逻辑泄漏（已修复）

~~ChannelIngress 包含消息去重（ingressMessageDedupe）、对等端防抖（ingressPeerDebouncer）、并发门控（channelConcurrentGate）、预览注册（turnPreviewRegistry）——这些并发控制和业务策略逻辑应在 biz 层。~~

✅ 已修复：实现已下沉 biz 层（biz.IngressDeduplicator/biz.PeerDebouncer/biz.ConcurrencyGate/biz.TurnPreviewManager），ChannelIngress 改为构造函数注入 biz 接口，sessionContextPressure 通过 TurnAdmissionUsecase 委托 biz 层。

#### 其他 service 层问题

- ~~SpiritSynthesisService 包含活跃团队检查和级联阻塞逻辑——业务逻辑泄漏~~ ✅ 已修复：仅薄适配层，业务逻辑在 biz.SynthesisUsecase
- ~~TeamService 持有 `team.Runner`（具体类型）和 `rt.RunRegistry`——违反依赖倒置原则~~ ✅ 已修复：改用 biz.TeamTurnRunnerPort 和 biz.RunRegistryPort 接口
- ~~Monitor 持有 `conf.Server` 配置引用——service 层不应依赖配置~~ ✅ 已修复：替换为 ProcessLogEnabledProvider 接口

### 5.3 data 层核心问题

#### kerrors 分层违规（已修复）

~~data 层有 267 处 `kerrors.` 调用。data 层使用 HTTP/gRPC 错误码是分层违规，应返回 `sql.ErrNoRows` 等标准错误或自定义领域错误，由 service/biz 层翻译为传输错误。~~

✅ 已修复：data 层 0 处 kerrors 调用。

#### 双轨 Schema 管理（轻微）

Ent schema 和原生 SQL DDL 并存（evaluation、knowledge），增加维护负担。

#### pgvector 旧包未迁移

`pgvector` 包标记为 Deprecated，但 `memoryRepo` 仍使用旧包，未迁移到 `vector.VectorStore`。

### 5.4 运行时模块详细分析

#### Agent 运行时 (internal/agent/)

- **完善度**：82%
- **优势**：多级 prompt 层（L1-L4）实现完整记忆注入管线；工具装配覆盖 filesystem/shell/MCP/knowledge/memory/subagent/kanban/outbound 全品类；回调链支持四阶段钩子；Ralph Loop 自验证循环；agent_as_tool 支持；a2ui Pipeline 计划-审批交互模式
- **关键缺失**：
  1. TRPCBuilderDeps 30+ 字段膨胀，缺乏分组抽象
  2. ModelSelector 仅 auto 一种策略，cost-aware / quality-aware / latency-aware 未实现
  3. 缓存一致性：依赖外部传入的 VersionHash，若调用方遗漏则缓存不会失效
  4. a2ui Pipeline 的 plan 存储使用内存 map，进程重启后丢失

#### Session 运行时 (internal/session/)

- **完善度**：78%
- **优势**：三级压缩级联（MicroCompact → MemoryCompact → LLM）设计精巧；CAS 乐观并发控制；自适应缓冲区根据对话模式动态调整压缩触发阈值
- **关键缺失**：
  1. Compressor 职责过重（700+ 行），建议拆分为 CompressorOrchestrator + CompressStrategy + CompressPersistence
  2. 压缩策略配置分散在多处，缺乏统一的 CompressPolicy 配置对象
  3. MemoryCompact 的 L1 字段与 L3 事实融合策略较简单，缺乏语义去重

#### Team 运行时 (internal/team/)

- **完善度**：75%
- **优势**：6种编排模式均有模板；失败策略支持 retry/fallback/circuit_breaker；TeamRunMediator 打破循环依赖
- **关键缺失**：
  1. Runner 执行路径碎片化：一个 Team turn 涉及 Runner → Mediator → Coordinator → Finisher → StepFlusher 五层委托
  2. Swarm 模式路由决策依赖 LLM 输出解析，缺乏确定性路由策略
  3. 图运行时与原生运行时的双轨制通过环境变量切换，行为差异缺乏文档
  4. 编译缓存基于 definition_json 哈希，Agent 配置变更时缓存不会自动失效

#### Skill 运行时 (internal/skill/)

- **完善度**：72%
- **优势**：导入管线完整（含补偿式事务）；双渲染模式（full/ai_optimized）；文件系统对账
- **关键缺失**：
  1. 相似度检测每次导入最多 50 次 LLM 调用，应引入向量嵌入预筛
  2. ~~文件系统监控为 5 分钟轮询，应引入 fsnotify 事件驱动~~ ✅ 已修复：已使用 fsnotify 事件驱动
  3. 存储后端仅支持本地文件系统，缺乏 S3/OSS 支持

### 5.5 前端详细分析

- **整体完善度**：82%
- **核心优势**：
  1. 主题系统极其完善（300+ CSS 变量，白昼奶油暖色 vs 夜间科技冷蓝双主题设计语言鲜明）
  2. WebSocket 三层架构健壮（transport/hub/dispatcher），含指数退避重连和 EventBuffer replay
  3. 聊天消息分组算法严谨（turn_id 权威分组 + 孤立块合并 + ReAct 去重）
  4. API 层 proto 生成 + 统一 handler 模式成熟
  5. Pinia store 按域拆分粒度合理（30+ store）
- **关键缺失**：
  1. 基础组件库不足（components/common/ 仅 5 个组件）
  2. E2E 测试覆盖率低（仅 3 个 spec）
  3. domain/ 层过薄（仅 3 个文件），领域逻辑散落在 features 中
  4. Spirit Service 和 Plan Service 未 proto 化，仍手写 HTTP 客户端
  5. A2UI 渲染管线缺少动态注册/卸载能力

### 5.6 pkg 公共包详细分析

- **整体完善度**：85%
- **核心优势**：
  1. apierror 统一错误模型设计优秀，biz/data/service 三层契约清晰
  2. loggateway + logpipeline 日志架构成熟（Gateway → Pipeline → SinkGroup → Sink 四层，背压/限流/断路器齐全）
  3. auth 包安全考量周全（JWT + Cookie + Bypass 双保险 + Webhook 签名头校验）
  4. outboundguard + webhookurl 双层 SSRF 防护，语义分工明确
- **关键缺失**：
  1. ~~safego 违反分层：pkg/safego 依赖 internal/metrics，造成 pkg → internal 反向依赖~~ ✅ 已修复
  2. ~~outboundguard 与 webhookurl 逻辑重复：IP 私有地址检查逻辑重复~~ ✅ 已修复：webhookurl 委托 outboundguard
  3. auth 仅支持 HS256 对称签名，生产环境应支持 RS256/ES256
  4. jsonutil/strutil 功能偏少
  5. logpipeline 断路器参数硬编码，Pipeline Stats 无运行时暴露

---

## 六、关键提升建议

### P0 — 架构健康度（必须解决）

| # | 建议 | 预期收益 | 状态 |
|---|------|----------|------|
| 1 | **拆分 ChatOrchestrator**：将 turn 准入、配额检查、session 状态转换提取到 biz 层 TurnAdmissionUsecase / TurnLifecycleUsecase | 消除 service 层最大业务逻辑泄漏，提升可测试性 | 🟡 部分完成：TurnAdmissionUsecase/TurnLifecycleUsecase 已下沉，sessionContextPressure 已完全委托 biz 层（含 channel 入口点），chatSessionStateMgr 已删除，enforceQuota 已下沉，但 ChatOrchestrator 仍有 36 字段 |
| 2 | **ConfigJSON 写路径改为只读投影**：Settings + Files → ConfigJSON 仅做读投影，消除双向写入不一致风险 | 消除数据一致性隐患（TODO DEV-10） | ✅ 已完成：DEV-10 FIXED |
| 3 | **A2ALimiter 替换为 Redis 分布式实现** | 多 Pod 部署可用（TODO DEV-08） | ✅ 已完成：Redis 实现已就绪并接入 Wire，service 层已使用注入的 a2a.Limiter，旧 a2a_limit.go 已删除 |

### P1 — 分层合规与概念统一

| # | 建议 | 预期收益 | 状态 |
|---|------|----------|------|
| 4 | **Channel 业务逻辑下沉**：消息去重/防抖/并发门控从 ChannelIngress 提取到 biz 层 | service 层职责纯化 | ✅ 已完成：实现已下沉 biz 层，接口已定义，ChannelIngress 改为构造函数注入 biz 接口，sessionContextPressure 通过 TurnAdmissionUsecase 委托 biz 层 |
| 5 | **消除 data 层 kerrors**：统一返回领域错误或标准错误，由上层翻译 | 分层合规（267处违规） | ✅ 已完成：data 层 0 处 kerrors 调用 |
| 6 | **统一进化体系**：废弃 SkillProposal，统一到 SkillEvolutionSuggestion；合并 LearningLoop 到 Evolution | 消除4套体系重叠 | 🟡 部分完成：SkillEvolutionOrchestrator 已建，EvolutionCoordinator 已 deprecated，但 DEV-04 未关闭 |
| 7 | **AgentRepository / ChannelRepo 拆分为窄接口** | 降低变更影响面 | ✅ 已完成：AgentRepository 拆为 8 个窄接口，Channel 侧拆为 8+ 窄接口 |
| 8 | **Team 运行时入口统一**：收敛 Runner turn 执行到 1-2 个核心方法，减少5-6层间接委托 | 可读性和可维护性 | 未修复：5层委托链仍在但有架构理由（Mediator 打破循环依赖） |

### P2 — 功能完善度提升

| # | 建议 | 预期收益 | 状态 |
|---|------|----------|------|
| 9 | **Memory Reranker 接入真实 Cross-Encoder** | 记忆召回质量提升（当前仅为 bigram Jaccard 代理） | 未修复 |
| 10 | **PII 检测增加 LLM 辅助** | 语义级 PII 检测（当前仅正则匹配） | 未修复 |
| 11 | **Skill 导入引入向量预筛** | 将 LLM 相似度调用从 50 次降至 ~5 次 | 未修复 |
| 12 | **provider/RoundTrip 增加重试/限流/熔断** | LLM 调用弹性 | 未修复 |
| 13 | **auth 支持 RS256/ES256 非对称签名** | 生产环境安全性 | 未修复 |
| 14 | **前端基础组件库扩充** | 消除通用 UI 模式散落 | 未修复 |
| 15 | **前端 E2E 测试覆盖** | 核心流程质量保障（当前仅3个spec） | 未修复 |

### P3 — 长期演进

| # | 建议 | 预期收益 | 状态 |
|---|------|----------|------|
| 16 | **Hook + Webhook 合并** | 消除概念重叠 | 🟡 无需合并：职责已分化（Hook=平台内部回调，Webhook=外部HTTP通知） |
| 17 | **统一模型路由**：provider 模型解析迁移到 modelregistry | 职责清晰 | 🟡 无需合并：职责已分化（LlmProviderModel=DB配置，ModelRegistry=文件策略） |
| 18 | **Skill 文件系统监控改用 fsnotify** | 实时性从5分钟提升到秒级 | ✅ 已完成：已使用 fsnotify 事件驱动 |
| 19 | **evaluation 持久化存储** | 评估结果跨运行对比 | 未修复 |
| 20 | **渠道类型插件化** | 可扩展性 | 未修复 |
| 21 | **Graph GC 参数可配置化** | 生产环境灵活性 | ✅ 已完成：通过环境变量 GRAPH_GC_* 注入，Wire provider 传递 GCConfig |

---

## 七、总体评价

**Aranea-Agents 是一个架构成熟度较高的多智能体编排平台**，整体完善度约 **82%**。

### 核心优势

1. 双框架分工明确，桥接层集中
2. biz 层端口/适配器架构执行到位，窄接口+编译时断言
3. 记忆系统（L0-L4）是最大的差异化能力
4. 前端主题系统（95%）和消息分组算法（92%）完成度极高
5. data 层读写分离和事务管理设计精良

### 核心架构债务

1. **ChatOrchestrator 上帝对象**——36 字段仍超标（AS-COG-01 上限 15），但业务逻辑已大幅下沉 biz 层
2. **4 套进化体系重叠**——DEV-04 未关闭，SkillProposal 与 SkillEvolutionSuggestion 并存（需 proto API 变更）
3. **Team 运行时执行路径碎片化**——4 层委托链（Runner→Mediator→Coordinator→Finisher），有架构理由（Mediator 打破循环依赖）

### 提升方向

- 架构整洁度：分层违规、概念重叠
- 基础设施弹性：重试/熔断/持久化
- 前端基础组件和测试覆盖
- 分布式就绪：内存限流器→Redis、事件持久化、会话分布式存储
