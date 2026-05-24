# System 系统 — 架构健康度诊断与综合开发计划

> **版本**：2026-05-21（Agent 优化）| **状态**：M0–M3 ✅；M4 进行中（Monitor/Token/Quota/MCP 部分已通；Channel/Ecosystem/Telemetry UI 待补）  
> **系统总览**：[0 系统框图.md](./0%20系统框图.md)  
> **模块索引**：[README-development.md](./README-development.md)  
> **进度真相**：[../guides/execution-plan.md](../guides/execution-plan.md)

## 1. 目标

本计划不是“修修补补”的任务清单，而是把系统从“难以预测变更影响的复杂体”，重构为“模块像乐高、调用关系清晰、可独立演进”的健壮系统。

最终目标：

- 模块职责单一明确，代码有清晰归属，不再自然长出巨大类或功能杂糅。
- 模块间调用方向稳定，修改一个模块不会连锁爆炸。
- 模块能力闭环完整，可通过组合快速搭建新功能。
- 文档能支持 AI 自主、安全、高效地拆任务、选文件、验证和更新计划。

## 2. 标杆架构对照

`pkg/trpc-agent-go` 是运行时框架真相源；OpenClaw 是产品化装配参考；GoClaw 是独立 Gateway 调度/IM 工程参考（非 vendored，见 [17-channel-external-reference-playbook.md](./17-channel-external-reference-playbook.md)）。

| 标杆能力 | Aranea 对齐位置 | 当前差距 | 康复方向 |
|----------|-----------------|----------|----------|
| Runner | `internal/agent/trpc_runtime.go` + `internal/runtime/runner_manager.go` | ✅ RunnerRegistry（per-session cancel/status/enqueue）+ RunnerManager（统一装配）已落地；ArtifactService/SessionIngestor/AgentFactory/AwaitUserReplyRouting 已注入；RalphLoop 仍为 OpenClaw 侧实现 | 已通：Chat/Team/Cron/Channel 共用 RunGateway |
| Agent | `internal/agent/trpc_build.go` + `builder_deps.go` | Builder 汇聚 Provider / Tool / Skill / Memory / Callback；`TRPC*Deps` 分组类型已落地 | 保持 Builder；`AgentSettingsPage` 拆分、列表运行态聚合 |
| Session | `internal/session/trpc` + `biz.SessionUsecase` | 框架 session 与业务 session 边界需更清晰 | Session transcript 与业务索引分工定稿 |
| Memory | `internal/memory/trpc` + L0-L4 | 框架 MemoryService 与 Aranea L0-L4 双轨 | L0-L4 作为 MemoryService 的产品实现 |
| Tool/MCP | `internal/tools` + `internal/tools/trpc` | ✅ ToolOverride/requires_confirmation/调用统计/TestTool/MCP 默认超时60s 已落地；MCP 认证/重连/Broker 默认发现仍待闭环 | 工具能力矩阵已通主路径；MCP 工程化待补 |
| Event | `internal/agent/event_projector.go` + `internal/event` | ✅ `/v1/ws` + 31 EnvelopeType；Consumer 已拆 + P3 侧效订阅（Tool/Callback/MessageStore/FlowLog） | SSE 仅限 A2A/MCP |
| Plugin/Callback | `internal/plugin/trpc` + `internal/agent/callbacks` | 9 内置插件 + Chain+Hook+OnEvent ✅；治理类插件多为策略/记录层 | 产品化：UpdateScope、运行记录表、`model_router` 真改模型 |
| Team/Graph | `internal/team`、`internal/graph` | Team member_* WS + 前端分栏 ✅；Graph LLM/Tool 节点、ExecutionSummary 待补 | 编排输出统一 Envelope；Graph 节点类型补全 |
| Evaluation/A2A | `internal/evaluation`、`internal/a2a` | ✅ Phase 5：FrameworkBridge、扩展指标、LLM UserSim、趋势/A/B、Eval LLM 系统配置；A2A Invoke + 联邦 Gateway | 质量门禁产品化、A2A Phase 4 Cron/限流 |

OpenClaw 在 `pkg/trpc-agent-go/openclaw` 中完整存在，可直接对照。吸收三件事：
1. Runner 为执行中心（session-scoped run control）—— Aranea 已用 `RunRegistry` + `RunnerManager` 实现对应模式；
2. Gateway 按 session 管控运行（cancel / status / enqueue）—— Aranea 已通过 `RunGateway` 接口落地；
3. Admin/Registry 用接口隔离扩展（channel/plugin/model/session/memory 注册表）—— Aranea 以 Wire DI + `internal/plugin/trpc/registry.go` 实现类似隔离。

**不要复制**：OpenClaw 的单体 `app.go`、HTML Admin UI、Telegram/stdin channel 栈、文件记忆实现、`runtimeprofile`（含多租户隔离策略）。GoClaw 单 MessageBus、无 revision cursor、string session key 作主键同理 — 只借调度/intent/preview 模式，见 playbook §3。

**可借鉴（按优先级）**：Session 队列三模式、忙线 intent、Ingress debounce/dedupe、Run 级 preview registry — 任务卡 `CH-BOR-*` 见 [17-channel-development.md §13](./17-channel-development.md#13-phase-g--外部参考借鉴ch-bor)。

## 3. 三维架构健康度诊断

### 3.1 模块定义清晰度

| 等级 | 模块 | 判断 | 问题 | 目标边界 |
|------|------|------|------|----------|
| 清晰 | `internal/biz` | 领域模型、Usecase、Repo 接口明确 | 少量领域与 provider inspect 概念交叉 | 只定义业务规则和端口 |
| 清晰 | `internal/event` | Envelope、Bus、Buffer、TraceEmitter 职责单一 | — | Bus 只管发布订阅；副作用在 biz handler |
| 清晰 | `internal/graph` | Graph runtime adapter 与 biz 端口较清楚 | Data 层 checkpoint 绑定框架 | adapter 管框架，data 管存储 |
| 中等 | `internal/service` | 本应是 proto ↔ biz/runtime 桥 | `ChatUsecase` 已编排入队/排队/await；`PendingMessageQueue` 实现仍在 Service；`setRunStatus` 与 Webhook 触发留 Service | PendingQueue 下沉 runtime；终态通知可经 EventBus 解耦 |
| 中等 | `internal/agent` | 构建 Agent / Runner 的正确位置 | ✅ `TRPCBuilderDeps` + `TRPC*Deps` 分组（`builder_deps.go`） | 巨型设置页拆分、列表 `last_run_status` |
| 中等 | `internal/tools` | 工具注册与装配合理 | Tool catalog、runtime mount、统计闭环分散 | Catalog / Policy / Runtime / Invocation 分层 |
| 中等 | `internal/memory*` | 产品 L0-L4 与框架 MemoryService 都合理 | 双轨未定主从，service/agent 直连 store | L0-L4 是产品模型，MemoryService 是 Runner 适配口 |
| 清晰 | Gateway / RunRegistry | `RunRegistry` + `RunnerManager` + `RunGateway` 已通 | `StopGeneration` 未统一 `publishRunStatus(cancelled)` | 取消路径与 WS `run_status` 完全对齐 |
| 不清晰 | Frontend store/features | 部分域有 store，部分 page 直连 API，mapper 三套 | AI 难判断新逻辑放哪里 | 统一 feature 模板和 store 策略 |
| 不完整 | TTS | 文档有目标，代码几乎无后端闭环 | 容易被误认为已可用 | 标注占位或补需求/设计/API |

### 3.2 模块间关联与耦合

| 关系 | 当前状态 | 健康度 | 风险 | 重构方向 |
|------|----------|--------|------|----------|
| `server -> service` | 大体成立 | 良好 | `skill_import_http.go` 旁路 service | Skill Import 迁入 proto + `SkillService` |
| `service -> biz` | 大体成立 | 中等 | `service` 还直接拿部分 data store | Store 访问经 biz 或 infra 端口 |
| `service -> runtime adapter` | 成立 | 良好 | ChatService 直接管理 Runner 生命周期 | 引入 RunnerManager |
| `biz -> data` | 通过 Repo 接口 | 良好 | Data import biz 是 Kratos 常见实现方式 | 保持，不让 biz 反向 import data |
| `biz <-> provider` | 存在概念双向依赖 | 偏弱 | 模型 inspect 与模型目录边界不稳 | 抽 `internal/llminspect` 或 biz 端口 |
| `data -> trpc runtime` | 存在 | 偏弱 | Data 层绑定 graph/session 框架类型 | provider 上移到 runtime/wire |
| `agent/team/tools -> biz` | 构建运行时需要领域配置 | 可接受 | Builder 编译面较大 | 用 Catalog DTO 稳定依赖 |
| `event -> consumers` | EventBusConsumer + EventBusSideConsumers | 良好 | 成员消息与 Runner 汇总语义需产品持续对齐 | 保持按 EnvelopeType 扩展 |
| `frontend pages -> features/stores` | 混合 | 偏弱 | 页面巨型化、store 空转 | 统一 page -> composable/store -> api |

### 3.3 模块功能完整度

| 闭环等级 | 模块 | 已有能力 | 缺口 | 为什么影响组合 |
|----------|------|----------|------|----------------|
| 闭环较好 | Chat、Agent、Provider、Session、Cron、Graph 核心 | 可创建、运行、持久化、展示 | 运行控制、细粒度事件仍可增强 | 已能作为其他模块入口 |
| 半闭环 | Team | RunTeamTest、CancelTeamRun、member_* WS、前端成员分栏 ✅ | 结构化汇总、跨 Team 编排可观测性 | 作为编排积木需统一 summary Envelope |
| 闭环较好 | Tools/MCP/Skill | Override、confirmation、统计、TestTool、MCP 60s 超时、OAuth2、Broker 挂载 ✅ | 生产级重连策略、Broker 默认发现文档化 | MCP 稳定性与运维可观测 |
| 半闭环 | Memory | RuntimeSet 端口统一；L4 prompt 注入；MemoryWorker；AutoMemory 图写入 ✅ | 冲突检测、级联更新、衰减算法 | 长期语义记忆治理 |
| 半闭环 | Plugin/Callback | 9 内置 + Chain+Hook+OnEvent + Schema/Scope ✅ | 产品化配置、运行记录、Audit 查询体验 | 横切治理可配置化 |
| 半闭环 | Monitor/Telemetry/Token | Audit 落库、Usage 事件、Quota MVP、Provider 指标 ✅ | Dashboard、业务 Span UI、告警规则 | 运营闭环 |
| 核心可用 | Evaluation | Phase 5 ✅：4+扩展指标、UserSim、pass@k、AfterTurn、趋势/A/B、Eval LLM 系统配置 | 质量门禁与迭代闭环产品化 |
| 半闭环 | A2A | Invoke 派发、call_agent、admin 鉴权、管理页 ✅ | 远程发现、A2A Server 暴露、流式、Graph 恢复 | 跨工作区与标准协议互通 |
| 半闭环 | Knowledge | 管理页、Embedder UI、摄取 WS、EnsureKnowledgeSchema ✅ | Rerank/OCR、PG 多租户稳定性 | 检索质量与工程化 |
| 闭环较好 | Artifact | Preview、签名下载、Chat 制品面板、CodeExecutor Docker 产出物→Artifact 🟡 | 对话内附件引用、跨会话制品检索；Local/OutputSpec 产出物 | 与 Chat 多模态联动 |
| 半闭环 | Ecosystem | proto + service/biz/data + 前端页面 ✅ | 安装流程产品化、市场模型 | 不再是纯 mock |

## 4. 目标架构：乐高式模块模型

每个模块都应被定义为一个可组合“积木”，包含五个面：

| 面 | 必须回答的问题 | 产物 |
|----|----------------|------|
| Contract | 对外承诺是什么？ | `api/kratos/*.proto`、前端 `types.ts` |
| Domain | 领域规则在哪里？ | `internal/biz/*Usecase`、Repo 接口 |
| Runtime | 是否需要接入 `trpc-agent-go`？ | `internal/<domain>/trpc` 或 `internal/agent/team/tools` |
| Persistence | 状态在哪里？ | `internal/data`、Ent schema、文件/向量存储 |
| UI/Operate | 用户如何配置、运行、观测？ | `web/src/features/<domain>`、page、store、monitor |

AI 新增或优化模块时，必须先填这五个面。缺任一面则只能标为 API-only、runtime-only 或 UI-mock，不能标为“完成”。

## 5. 康复原则

1. 先修边界，再补功能；禁止在错误依赖上继续堆代码。
2. 运行时能力只通过 `pkg/trpc-agent-go` 公开 API 集成，不复制框架内部实现。
3. `internal/service` 是桥，不是状态机仓库；复杂运行控制下沉到 RunnerManager / Usecase。
4. `internal/biz` 不 import `trpc-agent-go`；`internal/server` 不 import Agent runtime。
5. 实时主通道是 `/v1/ws` + Envelope；SSE 只可用于外部协议明确要求的 A2A/MCP 等。
6. 前端新增域统一 `features/<domain>/{api,types,mappers,composables,ui}`，store 策略必须明确。
7. 文档状态优先级：`0 系统框图.md` + 本计划 + `execution-plan.md` > 模块 development > design > 历史需求正文。

## 6. 综合开发路线图

### Phase 0：真理库与口径统一

目标：让 AI 按文档能做出正确第一步。

| 任务 | 内容 | 验收 |
|------|------|------|
| P0-1 | 系统架构总览和综合开发计划 | 能回答职责、依赖、闭环、开发顺序 |
| P0-2 | 进度真相和模块索引 | `docs/README.md` 链接不指向空文件 |
| P0-3 | 清理旧 SSE 主链路口径 | Chat / Team / Monitor 主通道均为 `/v1/ws` |
| P0-4 | 修复 Memory 断链 | 不再指向缺失的 Memory UX 文件 |
| P0-5 | 建立模块五面模板 | 新任务能按 Contract/Domain/Runtime/Persistence/UI 拆分 |

### Phase 1：边界收敛与运行时地基

目标：降低变更爆炸半径。

| 顺序 | 任务 | 开发内容 | 验收 |
|------|------|----------|------|
| 1 | ✅ Gateway 状态机独立 | `RunRegistry` + `RunnerManager` + `ChatUsecase`；出站 Webhook Phase 3 ✅ | Chat/Team/Cron/Channel 共用 `RunGateway`；`GatewayService` 管理回调配置 |
| 2 | ✅ Runner 生命周期统一 | 单 Agent / Team / Cron / Channel 共用 `RunGateway`（`RunRegistry`）入口 | cancel/status/enqueue 行为一致；Cron/Channel 经 `RunNativeTurnUnary`/`RunCronTurn` 接入 |
| 3 | ✅ Runner 框架能力补齐 | ArtifactService（`provideArtifactRuntimeService`）、SessionIngestor（`BizSessionIngestor`）、AgentFactory（`BizAgentFactoryOptions`）、AwaitUserReplyRouting（`AwaitHook` 配置时启用）均已注入；RalphLoop 为 OpenClaw 侧能力，Aranea 不复制 | `40-runner-development.md` P1/P2 已验收 |
| 4 | Memory 端口统一 | 收敛 `sessionmemory.Store` 直连，定稿 L0-L4 与 MemoryService 主从 | service/agent 不直接 import data store |
| 5 | Data 运行时绑定上移 | trpc session / graph checkpoint provider 移出 data 主 provider | data 保持 Ent/SQL Repo 边界 |
| 6 | Provider 拆环 | 抽 `internal/llminspect` 或 biz 端口接口 | `biz` 与 `provider` 不再概念互绑 |
| 7 | Skill Import service 化 | 导入 API 进入 proto + `SkillService` | server 不直接依赖 importer |

### Phase 2：核心积木闭环

目标：让核心模块能被可靠组合。

| 顺序 | 模块 | 开发内容 | 验收 |
|------|------|----------|------|
| 1 | Team | RunTeamTest 端到端、member_* WS、结构化汇总、A2A call_agent | Team 可作为可观测编排积木 |
| 2 | Tools/MCP | ToolOverride 生效、MCP timeout、调用统计、Broker 默认发现、重连验证 | 工具策略影响真实运行且可观测 |
| 3 | ✅ Plugin/Callback | Agent/Model/Tool 全链路挂载，Hook 与 Plugin 分工定稿，9 内置插件均有实现 | 横切治理覆盖完整 turn ✅；产品化配置/AuditLog 可观测待扩展（P2）|
| 4 | Memory | L4、MemoryWorker、冲突检测、级联更新、衰减 | 记忆可作为跨会话上下文底座 |
| 5 | Graph | LLM/Tool 节点、Input/OutputMapper、ExecutionSummary | Graph 可组合 Agent/Tool/Memory |

### Phase 3：用户闭环与前端治理

目标：让已接 API 的模块变成用户可操作能力。

| 顺序 | 任务 | 内容 | 验收 |
|------|------|------|------|
| 1 | 前端模块矩阵 | `需求编号 / route / features / stores / status` | API-only、mock、完成态清晰 |
| 2 | Feature 模板统一 | `api/types/mappers/composables/ui` | 新模块按模板生成 |
| 3 | Mapper 抽取 | Knowledge / Artifact / Evaluation / A2A mapper 独立可测 | 测试引用真实 mapper |
| 4 | Store 策略收敛 | 选择 `page -> store -> api` 或 `page -> api` | 删除或标记空转 store |
| 5 | 巨型文件拆分 | `useChatWorkspace.ts`、`AgentSettingsPage.vue` | 单文件职责可读 |
| 6 | 🟡 补页面 | Knowledge/Artifact/Evaluation/A2A/Hooks 页面已创建路由已接入；页面内仍有列表+弹窗+逻辑混写，需按 `page-to-components` 规则抽组件 | API/store 模块有用户入口 ✅；组件拆分待补 |

### Phase 4：平台运营与生态

目标：让系统可长期运行、治理和扩展。

| 顺序 | 模块 | 开发内容 | 验收 |
|------|------|----------|------|
| 1 | Monitor/Telemetry/Token | Dashboard、业务 Span、采样、Quota、告警 | 可从失败定位到 agent/tool/model/cost |
| 2 | Channel | 多平台适配、投递、签名、重试、状态页 | Webhook 与出站消息闭环 |
| 3 | Evaluation | 框架 EvalSet 对齐、前端页面、回归趋势 | 可复现实验和质量门禁 |
| 4 | A2A | 标准 `server/a2a` 或内部工具边界定稿 | 互通协议清晰 |
| 5 | Ecosystem | 后端 API、模板/插件/Skill 市场、安装流程 | 不再是纯 mock |
| 6 | CLI / TTS | 按产品目标补齐需求、API、运行时和 UI | 不再是占位能力 |

## 7. AI 自主任务拆解蓝图

AI 接到任何模块任务时，必须按以下顺序拆解：

1. 读取 `docs/README.md`、`0 系统框图.md`、本计划、`execution-plan.md`。
2. 定位模块五面：Contract、Domain、Runtime、Persistence、UI/Operate。
3. 判断任务类型：边界修复、能力闭环、UI 闭环、观测闭环、文档口径。
4. 写出变更影响半径：会触碰哪些 proto、service、biz、data、runtime adapter、web feature。
5. 先补或更新模块 development 文档，再改代码。
6. 按最小 PR 原则执行：边界 PR 不混功能，功能 PR 不混 UI 大重构。
7. 验证必须覆盖：后端边界、关键用例、前端 mapper/store、文档状态。

### 7.1 标准任务卡模板

| 字段 | 内容 |
|------|------|
| 任务 ID | 如 `SYS-RUNNER-01` |
| 所属模块 | Runner / Memory / Team 等 |
| 类型 | Boundary / Runtime / Domain / UI / Observability / Docs |
| 当前问题 | 一句话描述痛点 |
| 目标状态 | 完成后模块如何更像“乐高” |
| 变更范围 | proto / service / biz / data / runtime / web / docs |
| 不做范围 | 明确避免范围膨胀 |
| 验收标准 | 可自动或人工验证的结果 |
| 回滚策略 | 如何恢复旧路径 |

## 8. 待优化项总览

> **用法**：AI 拆任务时先读本节，再进入对应 `*-development.md`。已完成项不重复展开，仅列**仍影响组合能力或生产可用性**的缺口。  
> **真相源**：[execution-plan.md](../guides/execution-plan.md) · 模块细节见 [README-development.md](./README-development.md)。

### 8.1 按优先级汇总

| 优先级 | 含义 | 条目数 | 下一迭代建议入口 |
|--------|------|--------|------------------|
| **P0** | 架构红线或主链路口径错误 | 0（已清零） | — |
| **P1** | 核心积木仍不完整，阻塞组合 | 6 | Graph 节点、Team 汇总、Plugin 产品化、Evaluation 高级能力 |
| **P2** | 主路径可用，生产/体验/治理不足 | 12 | 前端治理、Memory 图治理、MCP 稳定性、Knowledge Rerank |
| **P3** | 平台运营、生态、占位能力 | 8 | Channel、Ecosystem、Telemetry UI、Chat 多模态 |

### 8.2 系统级与架构边界

| ID | 待优化项 | 优先级 | 现状 | 目标 | 关联文档 |
|----|----------|--------|------|------|----------|
| SYS-01 | `PendingMessageQueue` 仍在 Service 层 | P2 | 入队/排队已委托 `ChatUsecase` | 队列实现下沉 runtime | [35-gateway-development.md](./35-gateway-development.md) Phase 2 |
| SYS-02 | `StopGeneration` 未统一 `publishRunStatus(cancelled)` | P2 | ✅ `chat_stop_generation_test` | — | [1-chat-development.md](./1-chat-development.md) |
| SYS-03 | `EventBusConsumer` 职责聚合（Usage/Buffer/StateDelta/Persist） | P2 | ✅ buffer/runner/state/persist 四 handler | P3 独立 ToolCall 订阅 | [34-event-development.md](./34-event-development.md) · [message-development.md](./message-development.md) |
| SYS-04 | 核心模块五面（Contract/Domain/Runtime/Persistence/UI）未全建档 | P2 | Chat/Agent/Runner 较完整 | Graph/Channel/Ecosystem 补全五面表 | 本文 §4 |
| SYS-05 | 前端 `features` 与 `stores` 双路径并存 | P2 | 新模块 page 直连 api + store 空转 | 统一 `page → composable/store → api` 策略 | [frontend-guide.md](../guides/frontend-guide.md) |
| SYS-06 | 巨型文件可读性 | P2 | `useChatWorkspace` 已薄；`AgentSettingsPage` 等仍大 | `page-to-components` 拆分 | `web/.cursor/rules/page-to-components.mdc` |

### 8.3 Chat / Message（主链路）

| ID | 待优化项 | 优先级 | 现状 | 目标 | 关联文档 |
|----|----------|--------|------|------|----------|
| CHAT-01 | 多模态附件（上传、持久化、Vision） | P3 | Artifact parts 已装配 | 消息气泡内嵌预览；跨会话检索 | [1-chat-development.md](./1-chat-development.md) |
| CHAT-02 | `awaiting_user` / RunStatus 进程重启可恢复 | P3 | `state_json` + resume 新 turn ✅ | mid-turn goroutine 恢复（非本期） | [1-chat-development.md](./1-chat-development.md) |
| CHAT-03 | 模型选项单一真相源 | P3 | Platform 优先 + `GetChatOptions` 回退 | 长期统一一处配置源 | [1-chat-development.md](./1-chat-development.md) |
| CHAT-04 | `tool_result` 部分路径缺稳定 `tool_call_id` | P2 | 前端用 `env.id` 回退合并 | Projector 保证 id 一致 | [message-development.md](./message-development.md) |
| CHAT-05 | Chat / WS 关键路径单测 | P2 | service 层部分覆盖 | `TestChat*` / envelope 投影回归 | [1-chat-development.md](./1-chat-development.md) |
| CHAT-06 | 新 UI 文案 i18n | P3 | 工具卡片/Reasoning/回放横幅硬编码或缺 locale | `zh-CN` / `en` 键补全 | `web/src/locales/` |

**近期已完成（不再列入待办）**：WS 主通道 ✅ · `run_status` Envelope ✅ · 工具结构化卡片 ✅ · Reasoning 折叠 ✅ · Team 成员分栏 ✅ · WS `replay_*` 提示 ✅ · Monitor/Team 全局 `session_id=*` ✅。

### 8.4 Team / Graph / Runner

| ID | 待优化项 | 优先级 | 现状 | 目标 | 关联文档 |
|----|----------|--------|------|------|----------|
| TEAM-01 | Team 结构化汇总 Envelope | P1 | ✅ `team_summary` WS | — | [11-multi-agent-development.md](./11-multi-agent-development.md) |
| GRAPH-01 | Graph LLM / Tool 节点 | P1 | ✅ builder 接线 | — | [36-graph-development.md](./36-graph-development.md) |
| GRAPH-02 | ExecutionSummary / 运行记录 UI | P2 | ✅ `graph_execution_done` metadata | 前端 Graph 运行记录页待补 | [36-graph-development.md](./36-graph-development.md) |
| RUN-01 | 独立 `CancelRun` RPC（可选） | P3 | `StopGeneration` + WS `cancel` 已通 | 与 Chat proto 解耦的通用取消 RPC | [40-runner-development.md](./40-runner-development.md) |

### 8.5 Tools / MCP / Plugin / Callback

| ID | 待优化项 | 优先级 | 现状 | 目标 | 关联文档 |
|----|----------|--------|------|------|----------|
| MCP-01 | MCP 生产级重连与可观测 | P2 | ReconnectObserver + Events/Prometheus ✅ | metadata 重连计数持久化 | [19-mcp-development.md](./19-mcp-development.md) |
| PLG-01 | Plugin `UpdatePluginScope` / 运行记录 | P2 | ✅ | Scope API + `plugin_runs` + `ListPluginRuns` | [22-plugin-development.md](./22-plugin-development.md) |
| PLG-02 | `model_router` 真正改写模型配置 | P2 | `ModelSelector` 真路由 ✅ | `cost_guard` 同模式 | [22-plugin-development.md](./22-plugin-development.md) |
| CB-01 | Callback 产品化配置与 Audit 查询体验 | P2 | ✅ | `/plugins/runs` 筛选 + Hook `hook:` 落库 | [28-callback-development.md](./28-callback-development.md) |

### 8.6 Memory / Knowledge

| ID | 待优化项 | 优先级 | 现状 | 目标 | 关联文档 |
|----|----------|--------|------|------|----------|
| MEM-01 | L4 图冲突检测与级联更新 | P2 | ✅ 冲突/级联/衰减 MVP | 冲突策略、级联、衰减 | [memory/memory-development.md](./memory/memory-development.md) |
| MEM-02 | MemoryWorker 与多租户 Session 边界 | P2 | TurnMemoryWorker 30s 去重已有 | 工作区级隔离与失败重试 | [memory/memory-development.md](./memory/memory-development.md) |
| KN-01 | Rerank / OCR 规划与实现 | P2 | Rerank ✅（`KRATOS_KNOWLEDGE_RERANKER`）；OCR 待补 | OCR + rerank fallback FlowLog ✅ | [37-knowledge-development.md](./37-knowledge-development.md) |
| KN-02 | Knowledge PG 多租户与稳定性 | P2 | `EnsureKnowledgeSchema` 启动调用已有 | 无 PG 时降级策略文档化 + 压测 | [37-knowledge-development.md](./37-knowledge-development.md) |

### 8.7 Evaluation / A2A / Artifact

| ID | 待优化项 | 优先级 | 现状 | 目标 | 关联文档 |
|----|----------|--------|------|------|----------|
| EVAL-01 | DeleteRun / UpdateDataset API | P2 | CRUD + Run 已有 | 数据集迭代与 run 清理 | [33-evaluation-development.md](./33-evaluation-development.md) |
| EVAL-02 | 人工评估与报告导出 | P2 | 标注 ✅；CSV/JSON ✅；趋势/A/B 前端 ✅ | 服务端 PDF 报告 | [33-evaluation-development.md](./33-evaluation-development.md) |
| EVAL-03 | AfterTurn 自动评估触发 | P3 | ✅ NativeTurnAfterHook | — | [33-evaluation-development.md](./33-evaluation-development.md) |
| EVAL-04 | Phase 5 扩展评估 | P3 | ✅ 扩展指标/UserSim/Eval LLM 系统配置 | — | [33-evaluation-development.md](./33-evaluation-development.md) |
| A2A-01 | 远程 Agent 发现与注册中心 | P2 | 本地 Invoke 已有 | URL 发现、跨实例 Card | [26-a2a-development.md](./26-a2a-development.md) |
| A2A-02 | A2A Server 暴露 + 流式 SSE | P3 | admin Invoke 已有 | 标准 `server/a2a` 或框架 a2aagent | [26-a2a-development.md](./26-a2a-development.md) |
| ART-01 | Chat 多模态引用制品 | P3 | 会话制品面板已有 | 消息气泡内嵌 artifact 预览 | [27-artifact-development.md](./27-artifact-development.md) |

### 8.8 平台运营与占位模块

| ID | 待优化项 | 优先级 | 现状 | 目标 | 关联文档 |
|----|----------|--------|------|------|----------|
| CH-01 | Channel Webhook 入站 | P1 | ✅ | — | [17-channel-development.md](./17-channel-development.md) |
| CH-02 | Channel 出站投递与适配器 | P1 | ✅ | 更多平台适配器 | [17-channel-development.md](./17-channel-development.md) |
| MON-01 | Monitor Dashboard 与告警规则 | P2 | 告警 ✅；概览 `/overview` ECharts + Usage Tab 去重 ✅ | latency 聚合、Phase 4 自动刷新 | [18-monitor-dashboard-development.md](./18-monitor-dashboard-development.md) |
| TEL-01 | Telemetry 业务 Span / OTel UI | P3 | 指标部分已有 | 链路 UI 与采样配置 | [24-telemetry-development.md](./24-telemetry-development.md) |
| ECO-01 | Ecosystem 后端与市场模型 | P3 | proto + service/biz/data + 前端页面 ✅ | 安装流程产品化、市场模型 | [30-ecosystem-development.md](./30-ecosystem-development.md) |
| CLI-01 | CLI 产品化（非 OpenClaw 复制） | P3 | 文档占位 | 需求 + API + 分发 | [25-cli-development.md](./25-cli-development.md) |
| TTS-01 | TTS 运行时闭环 | P3 | 几乎无后端 | 标注 API-only 或补实现 | [tts-development.md](./tts-development.md) |

### 8.11 Agent 全家桶（2–8 / 50）

> **模块开发计划**（实现差距以 `*-development.md` 为准，需求/设计正文为产品规格）：  
> [2–8 横切](./2-8-agent-modules-development.md) · [2 创建](./2-agents-create-development.md) · [3 列表](./3-agent-list-development.md) · [4 分类](./4-agent-type-development.md) · [5 设置](./5-agent-setting-development.md) · [6 文件](./6-agent-setting-file-development.md) · [7 进化](./7-agent-evolution-development.md) · [8 顶栏](./8-agent-title-development.md)

| ID | 模块 | 待优化项 | 优先级 | 现状 |
|----|------|----------|--------|------|
| AGT-01 | 运行时 | `TRPCBuilderDeps` 分组 + `system.agent.build` FlowLog | P2 | ✅ |
| AGT-02 | 5 设置 | `config_json` PATCH 浅合并 | P2 | ✅ |
| AGT-03 | 2 创建 | agent_key 查重 `GET /v1/agent-keys/check` | P3 | ✅ |
| AGT-04 | 2 创建 | Provider/Model `validate-model` | P2 | ✅ |
| AGT-05 | 5 设置 | ToolOverride 全链路 | P2 | ✅ |
| AGT-06 | 2 创建 | 后端 Agent 模板 API | P2 | ✅ 迭代 10 |
| AGT-07 | 3 列表 | `last_run_status` / `created_by` | P2 | ✅ 运行态 + `created_by` 列/筛选（2026-05-21） |
| AGT-08 | 5 设置 | `AgentSettingsPage` 拆分 | P2 | ✅ 迭代 10 |
| AGT-09 | 7 进化 | EvolutionScanner + 自动建议 | P2 | ✅ 迭代 10 |
| AGT-10 | 3 列表 | `DuplicateAgent` | P3 | ✅ 迭代 10；副本 `created_by` 归属当前用户（2026-05-21 审查） |
| AGT-11 | 6 文件 | AI 编辑（真实 LLM） | P2 | ✅ 迭代 10 |
| AGT-12 | 6 文件 | `EstimateTokens` 前端对接 | P2 | ✅ |
| AGT-13 | 4 分类 | 删除前 Agent 计数 / 拖拽排序 | P2–P3 | ✅ 计数；排序 ❌ |
| AGT-14 | 8 顶栏 | 「进化中」与 pending 建议对齐 | P2 | ✅ |
| AGT-15 | 8 顶栏 | `GenerateAgentTitle` | P3 | ❌ |
| AGT-16 | 7 进化 | 指标趋势图表 | P3 | ❌ |
| — | 横切 | 迭代 10 审查：ListExtras 批量、终态 `runtime.status`、`ApplySuggestion(prompt)` | — | ✅ [Iteration10](../changelog/2026-05-21-Agent-Iteration10.md) |
| — | 2/3 | LIST-02：`created_by`、模板全字段、结构化创建错误 | P2 | ✅ [CreatedBy-Templates-Errors](../changelog/2026-05-21-Agent-CreatedBy-Templates-Errors.md) |

### 8.9 前端治理（横切）

| ID | 待优化项 | 优先级 | 现状 | 目标 | 关联文档 |
|----|----------|--------|------|------|----------|
| FE-01 | `KnowledgePage` / `EvaluationPage` / `A2APage` 组件拆分 | P2 | ✅ 三页均 <300 行 | 弹窗/表格抽独立组件 | `page-to-components.mdc` |
| FE-02 | feature mapper 单测 | P2 | A2A mapper 单测 ✅ | knowledge/evaluation mapper 回归 | 各 `features/*/mappers` |
| FE-03 | 减少 `as any`（Chat/Team stream） | P3 | 部分 mapper 有 any | 严格 Envelope 类型 | `web/src/features/chat/` |

### 8.10 开发顺序总表（里程碑视角）

| 优先级 | 开发项 | 状态 | 说明 |
|--------|--------|------|------|
| P0 | 文档真理库、WS 口径、Gateway、Memory 端口 | ✅ | M0–M2 |
| P0 | Runner 框架能力、Team/Tools/Plugin 主闭环 | ✅ | M1–M3 |
| P1 | Evaluation EvalSet + LLMJudge、A2A Invoke、Artifact 预览 | ✅ | M3 |
| P1 | Chat 工具卡片 / Reasoning / run_status WS / Team UX | ✅ | 2026-05-19 迭代 |
| P1 | Team 结构化汇总、Graph LLM/Tool 节点、Channel 投递 | ✅ | 迭代 2–5 已验收 |
| P2 | 前端治理、Memory 图治理、Plugin 产品化、MCP 稳定性 | 🚧 | 见 §8.5–§8.9 |
| P3 | 多模态、RunStatus 持久化、Ecosystem/Telemetry/CLI/TTS | ⏳ | 见 §8.3、§8.8 |

## 9. 系统级验收标准

- [x] `internal/biz` 不 import `trpc-agent-go`（已验证，仅有注释提及）。
- [x] `internal/server` 不 import Agent runtime（已验证，仅注册 proto service，不直接调 Runner）。
- [x] `internal/data` 不直接绑定 Runner / Agent / Graph runtime 组装（Data 层 provider 已上移至 runtime）。
- [x] Chat / Team / Graph / Monitor 实时主链路统一为 `/v1/ws`（Monitor/Team 全局 `session_id=*`；旧 SSE 主链路已清理）。
- [x] Runner 可统一处理 status、cancel、enqueue、artifact、session ingest（RunRegistry + RunnerManager 已通）。
- [x] Memory L0-L4 与框架 MemoryService 主从关系清晰（`memory.RuntimeSet` 已统一端口）。
- [ ] 每个核心模块都有明确五面定义（Graph/Channel 仍缺 UI 或 Runtime 面定稿；Ecosystem 已有五面但市场模型待补）。
- [ ] `internal/service` 不承载复杂运行状态机（await/pending 仍在 ChatService；可接受短期存在）。
- [ ] 前端新增模块遵循统一 feature 模板，mapper 有真实单测。
- [ ] API-only 模块文档标注（TTS/CLI 需在 README 标占位）。

## 11. 代码质量评价（2026-05-19）

### 11.1 后端

| 维度 | 评价 | 主要问题 |
|------|------|----------|
| 架构边界 | **良好** | `biz` / `server` / `data` 红线基本守住；`internal/runtime` 新包职责清晰 |
| 运行时集成 | **良好** | RunnerManager/RunRegistry 模式正确；tRPC-Agent-Go 框架 API 使用合规，未复制内部实现 |
| Plugin/Tool | **良好** | 9 个内置插件均有实现；ToolOverride + confirmation + 统计形成闭环；StatsRecorder 模式值得复用 |
| Evaluation | **良好** | Phase 5 完整；Eval LLM 可系统设置；待补服务端 PDF 报告 |
| A2A | **良好** | Invoke + call_agent + 鉴权已通；待补远程发现、A2A Server、流式 |
| 测试覆盖 | **中等** | `internal/runtime`、`internal/agent`、`internal/biz` 有单测；`internal/plugin/trpc`、`internal/tools` 有单测；`internal/evaluation`、`internal/a2a` 测试覆盖偏少 |
| 生成文件噪音 | **低风险** | `*.pb.go` / `wire_gen.go` 均在 `.gitignore` 之外但不应手改；`api/kratos/tool/v1/tool.proto` 有重复路径（同时在 `??` 和 ` M` 区段），需排查 wire 依赖是否正确 |

### 11.2 前端

| 维度 | 评价 | 主要问题 |
|------|------|----------|
| 功能覆盖 | **良好** | 新增 A2A/Evaluation/Knowledge/Hooks/Artifacts 页面全部路由可达；mapper 模式统一 |
| 组件拆分 | **中等** | `KnowledgePage.vue`（414 行）、`EvaluationPage.vue`（340 行）、`A2APage.vue`（224 行）内含复杂弹窗/逻辑，应按 `page-to-components` 规则拆分 |
| Store 策略 | **中等** | 新模块（a2a/evaluation/knowledge）既有 store 又有页面直连 api，两条路径不统一，增加维护成本 |
| 类型安全 | **中等** | `useChatWorkspace.ts` 含 `as any`；部分 mapper 中有 `any` 类型；应逐步强化 |
| 测试覆盖 | **偏弱** | 仅有 `stores/__tests__/app.store.spec.ts`；新增 feature/mapper 无单测 |

### 11.3 总结与优先修复建议

> 完整待办表见 **§8 待优化项总览**。以下为下一迭代 Top 7：

1. **P1 — Channel 投递闭环**：Webhook 入站 + 出站 delivery + 至少一种平台适配器（EP-BIZ-08）。
2. **P1 — Graph LLM/Tool 节点**：补节点类型与 ExecutionSummary，使 Graph 可作为编排积木。
3. **P1 — Team 结构化汇总**：统一 summary Envelope，便于 Monitor 与下游自动化。
4. **P2 — 前端治理**：Knowledge/Evaluation/A2A 页面 `page-to-components`；store 策略二选一；mapper 单测。
5. **P2 — Memory 图治理**：L4 冲突检测、级联更新、衰减（AutoMemory 写入已有）。
6. **P2 — Plugin 产品化**：UpdateScope、运行记录、`model_router` 真改模型。
7. **P3 — Chat 多模态与 RunStatus 持久化**：附件 Vision 闭环；重启后 `awaiting_user` 可恢复。

## 10. 交付拆分建议

| # | PR 名称 | 状态 | 内容 |
|---|---------|------|------|
| 1 | `PR-Doc-Architecture` | ✅ 已完成 | 系统图、开发计划、执行计划、模块五面模板 |
| 2 | `PR-Runner-Registry` | ✅ 已完成 | RunRegistry + RunnerManager |
| 3 | `PR-Runner-Control` | ✅ 已完成 | EnqueueUserMessage / StopGeneration+WS cancel / RunStatus 对齐 |
| 4 | `PR-Memory-Boundary` | ✅ 已完成 | Memory 端口统一；`memory.RuntimeSet` + SessionAdminStore |
| 5 | `PR-Boundary-Cleanup` | ✅ 已完成 | Data provider 上移、Provider 拆环、Skill Import service 化 |
| 6 | `PR-Team-Observability` | ✅ 已完成 | RunTeamTest、CancelTeamRun、member_* WS Envelope |
| 7 | `PR-Plugin-Callback` | ✅ 已完成 | 9 内置插件实现、Chain+Hook+OnEvent、种子+Schema+Scope 过滤 |
| 8 | `PR-Tools-MCP-Core` | ✅ 已完成 | ToolOverride/confirmation/统计/TestTool/默认timeout |
| 9 | `PR-Knowledge-Artifact-Pages` | ✅ 已完成 | KnowledgePage/ArtifactsPage/路由/侧栏；EvalPage/A2APage/HooksPage |
| 10 | `PR-Evaluation-Runtime` | ✅ 已完成 | EvalSet 对齐、LLMJudge、异步 Runner |
| 11 | `PR-A2A-Invoke` | ✅ 已完成 | Invoke 派发、call_agent、admin 鉴权 |
| 12 | `PR-Artifact-Preview` | ✅ 已完成 | Preview、签名下载、Chat 制品面板 |
| 12b | `PR-Chat-UX-WS` | ✅ 已完成 | 工具卡片、Reasoning、run_status WS、Team 分栏、replay 提示 |
| 13 | `PR-MCP-Engineering` | 🟡 部分完成 | OAuth2/Broker/timeout ✅；重连可观测待补 |
| 14 | `PR-Memory-L4` | 🟡 部分完成 | L4 注入 + Worker + AutoMemory ✅；冲突/级联待补 |
| 15 | `PR-Frontend-Foundation` | 🚧 待做 | page-to-components；store 策略；mapper 单测 |
| 16 | `PR-Platform-Ops` | 🚧 进行中 | Quota/Usage/Audit ✅；Channel/Ecosystem/Telemetry UI 待补 |
