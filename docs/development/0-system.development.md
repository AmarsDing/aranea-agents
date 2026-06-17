# System 系统 — 架构健康度诊断与综合开发计划

> **版本**：2026-06-17（Agent 优化）| **状态**：M0–M3 ✅；M4 进行中（Monitor/Token/Quota/MCP 部分已通；Channel/Ecosystem/Telemetry UI 待补）  
> **系统总览**：[0-system-diagram.md](./0-system-diagram.md)  
> **模块索引**：[README-development.md](./README-development.md)  
> **进度真相**：[../guides/execution-plan.md](../guides/execution-plan.md)
>
> **文档性质**：开发计划（模块定位、代码锚点、现状评估、差距与优化、Phase 划分、任务清单、验收标准、改动文件清单）。架构设计、Proto/API 契约、模块关系见 [0-system-diagram.md](./0-system-diagram.md)。

## 1. 目标

本计划不是“修修补补”的任务清单，而是把系统从“难以预测变更影响的复杂体”，重构为“模块像乐高、调用关系清晰、可独立演进”的健壮系统。

最终目标：

- 模块职责单一明确，代码有清晰归属，不再自然长出巨大类或功能杂糅。
- 模块间调用方向稳定，修改一个模块不会连锁爆炸。
- 模块能力闭环完整，可通过组合快速搭建新功能。
- 文档能支持 AI 自主、安全、高效地拆任务、选文件、验证和更新计划。

## 2. 标杆架构对照

`pkg/trpc-agent-go` 是运行时框架真相源；OpenClaw 是产品化装配参考；GoClaw 是独立 Gateway 调度/IM 工程参考（非 vendored，见 [17-channel-external-reference-playbook.md](./17-channel-external-reference-playbook.md)）。架构对齐位置与模块关系见 [0-system-diagram.md](./0-system-diagram.md)。

| 标杆能力 | 当前差距 | 康复方向 |
|----------|----------|----------|
| Runner | ✅ RunnerRegistry（per-session cancel/status/enqueue）+ RunnerManager（统一装配）已落地；ArtifactService/SessionIngestor/AgentFactory/AwaitUserReplyRouting 已注入；RalphLoop 仍为 OpenClaw 侧实现 | 已通：Chat/Team/Cron/Channel 共用 RunGateway |
| Agent | Builder 汇聚 Provider / Tool / Skill / Memory / Callback；`TRPC*Deps` 分组类型已落地 | 保持 Builder；`AgentSettingsPage` 拆分、列表运行态聚合 |
| Session | 框架 session 与业务 session 边界需更清晰 | Session transcript 与业务索引分工定稿 |
| Memory | 框架 MemoryService 与 Aranea L0-L4 双轨 | L0-L4 作为 MemoryService 的产品实现 |
| Tool/MCP | ✅ ToolOverride/requires_confirmation/调用统计/TestTool/MCP 默认超时60s 已落地；MCP 认证/重连/Broker 默认发现仍待闭环 | 工具能力矩阵已通主路径；MCP 工程化待补 |
| Event | ✅ `/v1/ws` + 31 EnvelopeType；Consumer 已拆 + P3 侧效订阅（Tool/Callback/MessageStore/FlowLog） | SSE 仅限 A2A/MCP |
| Plugin/Callback | 9 内置插件 + Chain+Hook+OnEvent ✅；治理类插件多为策略/记录层 | 产品化：UpdateScope、运行记录表、`model_router` 真改模型 |
| Team/Graph | Team member_* WS + 前端分栏 ✅；Graph LLM/Tool 节点、ExecutionSummary 待补 | 编排输出统一 Envelope；Graph 节点类型补全 |
| Evaluation/A2A | ✅ Phase 5：FrameworkBridge、扩展指标、LLM UserSim、趋势/A/B、Eval LLM 系统配置；A2A Invoke + 联邦 Gateway | 质量门禁产品化、A2A Phase 4 Cron/限流 |

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
| `server -> service` | 大体成立 | 良好 | `service/skill_import_http.go` 旁路 proto service | Skill Import 迁入 proto + `SkillService` |
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

> 模块五面定义（Contract / Domain / Runtime / Persistence / UI/Operate）与架构原则已迁移到 [0-system-diagram.md §十二、§十三](./0-system-diagram.md)。本节仅保留与开发计划相关的判断规则。

模块五面完整性判断规则（用于任务拆解时评估模块闭环状态）：

- 缺任一面则只能标为 API-only、runtime-only 或 UI-mock，不能标为"完成"。
- AI 接到模块任务时，必须先定位模块五面，判断任务类型（边界修复、能力闭环、UI 闭环、观测闭环、文档口径），再写出变更影响半径。

## 5. 康复原则

1. 先修边界，再补功能；禁止在错误依赖上继续堆代码。
2. 运行时能力只通过 `pkg/trpc-agent-go` 公开 API 集成，不复制框架内部实现。
3. `internal/service` 是桥，不是状态机仓库；复杂运行控制下沉到 RunnerManager / Usecase。
4. `internal/biz` 不 import `trpc-agent-go`；`internal/server` 不 import Agent runtime。
5. 实时主通道是 `/v1/ws` + Envelope；SSE 只可用于外部协议明确要求的 A2A/MCP 等。
6. 前端新增域统一 `features/<domain>/{api,types,mappers,composables,ui}`，store 策略必须明确。
7. 文档状态优先级：`0-system-diagram.md` + 本计划 + `execution-plan.md` > 模块 development > design > 历史需求正文。

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
| 4 | Memory 端口统一 | 收敛 `SessionAdminStore` 直连，定稿 L0-L4 与 MemoryService 主从 | service/agent 不直接 import data store |
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

1. 读取 `docs/README.md`、`0-system-diagram.md`、本计划、`execution-plan.md`。
2. 定位模块五面：Contract、Domain、Runtime、Persistence、UI/Operate（定义见 [0-system-diagram.md §十二](./0-system-diagram.md)）。
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
| ART-01 | Chat 多模态引用制品 | P3 | 用户/Assistant/Team 消息气泡 + Vision 附件 ✅ | 流式上传 >10 MB | [27-artifact-development.md](./27-artifact-development.md) |

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
- [x] Memory L0-L4 与框架 MemoryService 主从关系清晰（`runtime.MemorySet` 已统一端口）。
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
| 4 | `PR-Memory-Boundary` | ✅ 已完成 | Memory 端口统一；`runtime.MemorySet` + SessionAdminStore |
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


---

## 子模块：System 优化路线图 2026-05-26

> **版本**：2026-05-26 · **状态**：📋 路线图草案 · **范围**：当前所有 5 份独立优化计划的统一执行序列
> **目的**：把分散在 5 个模块（Memory · Monitor · Tools/Plugin/Skill/MCP · Channel/Chat/Agent-Team · Team Graph）的 ~150 个优化项排成 **可顺序实施 / 可灰度回滚 / 可独立 ship** 的全局路线
> **真相源**：本文档是**单一执行序列**真相源；每个 Wave 内部细节回到对应需求/开发文档

---

## 1. 已纳入的优化计划

| # | 计划编号 | 主题 | 主文档 | 开发计划 | 体量 |
|---|---------|------|--------|----------|------|
| 1 | **M56 BLO** | Channel × Chat × Agent/Team 业务模型 5 主题 | [`56 business-logic-optimization.md`](./56%20business-logic-optimization.md) | [`56-business-logic-optimization-development.md`](./56-business-logic-optimization-development.md) | 5 主题 / ~40 任务 / 12 周 |
| 2 | **M57 TPM** | Tools / Plugin / Skill / MCP 子系统代码债 | — | [`38-tools-plugin-skill-mcp-optimization.development.md`](./38-tools-plugin-skill-mcp-optimization.development.md) | 12 P1 + 30 P2 + 13 P3 + 14 D / 4 Wave |
| 3 | **MEM-OPT** | Memory 业务逻辑（一致性 / 衰减 / 队列 / PII / 提取 / Cascade） | [`memory-optimization-2026-05-26.md`](./memory/memory-optimization-2026-05-26.md) | 内嵌 §8 排期 | 6 主题 / 5 Sprint / 11 周 |
| 4 | **MON-OPT** | Monitor 业务逻辑（Bus 分离 / 冷却 / 评估 / 反压 / Trace / DSL） | [`18 monitor-optimization-2026-05-26.md`](./18%20monitor-optimization-2026-05-26.md) | 内嵌 §8 排期 | 6 主题 / 6 Sprint / 13 周 |
| 5 | **TG-Q** | Team Graph 代码债（review 直接 backlog） | [Review §6](../review/2026-05-26-Team-Graph-Code-Review.md#6-问题清单按优先级) | 无独立 dev plan | 5 P1 + 6 P2 + 3 P3 / ~3 周 |

**合计**：~150 任务项 / 估算 30+ 人周。

---

## 2. 全局依赖图

```mermaid
flowchart TD
    classDef p0 fill:#fee,stroke:#c33,color:#900
    classDef p1 fill:#fef3c7,stroke:#d97706,color:#92400e
    classDef p2 fill:#dbeafe,stroke:#2563eb,color:#1e40af
    classDef p3 fill:#dcfce7,stroke:#16a34a,color:#166534

    M56_BLO5[M56 BLO-5 Unified BackgroundJob 基础设施]:::p0
    M56_BLO4[M56 BLO-4 Non-Blocking HITL]:::p1
    M56_BLO2[M56 BLO-2 Multi-Signal Escalation]:::p1
    M56_BLO1[M56 BLO-1 Intent-Aware Admission]:::p2
    M56_BLO3[M56 BLO-3 Channel Trigger Rules]:::p2

    M57_W1[M57 TPM Wave 1 P1×12]:::p0
    M57_W2[M57 TPM Wave 2 P2 + D-T1/S2/M2]:::p1
    M57_W3[M57 TPM Wave 3 D-P1/P2/P4/T2/S1/S3/S4]:::p2
    M57_W4[M57 TPM Wave 4 EventSourcing/OPA]:::p3

    MEM_A[MEM-OPT-01 双轨一致性 + MEM-OPT-03 队列优先级]:::p0
    MEM_B[MEM-OPT-02 L4 衰减 + Phase3 Reconciler]:::p1
    MEM_C[MEM-OPT-05 提取协议化 + MEM-OPT-04 PII Block]:::p1
    MEM_D[MEM-OPT-06 Cascade Saga + Dry-Run]:::p2
    MEM_E[MEM-OPT-04 PII Review 工作流]:::p2

    MON_A[MON-OPT-01 Bus 全分离 + MON-OPT-04 优先级队列]:::p0
    MON_B[MON-OPT-02 冷却持久化 + MON-OPT-03 RingBuffer 评估]:::p1
    MON_C[MON-OPT-05 Trace Projector]:::p1
    MON_D[MON-OPT-05 跨 trace 关联 + Lossless 模式]:::p2
    MON_E[MON-OPT-06 Registry + DSL]:::p2
    MON_F[MON-OPT-02 escalation + silence_windows]:::p2

    TG_P1[TG-Q-01..05 状态常量/GC 调度/拆函数/幽灵函数/单向依赖]:::p1
    TG_P2[TG-Q-06..11 critic 协议/watch 测试/resume 错误暴露]:::p2

    M56_BLO5 --> M56_BLO4
    M56_BLO5 --> M56_BLO2
    M56_BLO5 --> M56_BLO3
    M56_BLO4 --> M56_BLO1

    M57_W1 --> M57_W2
    M57_W2 --> M57_W3
    M57_W3 --> M57_W4

    MEM_A --> MEM_B
    MEM_A --> MEM_C
    MEM_C --> MEM_D
    MEM_D --> MEM_E
    MEM_C -.| MEM-OPT-05 用 schema | MON_C

    MON_A --> MON_B
    MON_B --> MON_C
    MON_C --> MON_D
    MON_B --> MON_F

    TG_P1 --> TG_P2

    M57_W1 -.| 同属 P1 速胜 | TG_P1
    M56_BLO5 -.| BackgroundJob 抽象可被复用 | MEM_A
    MON_A -.| Bus 分离影响 Memory Worker 事件 | MEM_A
```

**关键依赖判断**：
- **M56 BLO-5（BackgroundJob 抽象）解锁三个上层（BLO-1/2/3/4）** —— 必须最早完成。
- **MEM-OPT-01（一致性）与 MON-OPT-01（Bus 分离）互不依赖但都是 P0** —— 可并行。
- **M57 TPM Wave 1（12 项 P1）多为小修复（XS/S 体量）** —— 适合穿插在任意 Sprint 当"速胜"。
- **TG-Q-01..05** —— 5 项 P1 都是 1-2 天小改，可在 Sprint 间填空。

---

## 3. 全局执行序列（4 阶段）

按 **风险 + 依赖 + 业务可见度** 排序。每阶段产出 **可独立灰度上线 + 可回滚** 的能力集合。

### Phase 0 — 准备（0.5 周）

| 序号 | 任务 | 来源 | 工时 |
|------|------|------|------|
| P0-1 | 写 `56 business-logic-optimization.design.md`（BLO-PRE-01） | M56 | 1d |
| P0-2 | 注入 5 个 BLO Feature flag（BLO-PRE-02） | M56 | 0.5d |
| P0-3 | Datadog 看板雏形（BLO-PRE-03） | M56 | 0.5d |
| P0-4 | 本路线图归档到 `docs/需求/0-system-development.md §8.7`（路线图引用） | 本文 | 0.5d |

**Gate 0**：5 个 flag 默认 off，Datadog 看板可显示零数据。

---

### Phase 1 — 关键正确性 + 基础设施（4 周）

> **目标**：消除 P0/P1 业务正确性缺陷；为后续 Phase 提供基础设施。

#### Sprint 1.1（第 1-2 周）— 基础设施 + 静默失败收敛

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 1 | **M56 BLO-5 Sprint A1**（biz/data 层 BackgroundJob 抽象） | M56 | 1 周 | `internal/biz/backgroundjob/` + 表 |
| 2 | **MON-OPT-01 Phase 0/1**（Bus 路由表 + dual 灰度） | MON | 3d | `event.Infra.Publish` 路由 |
| 3 | **MEM-OPT-01 Phase 0/1**（fact `index_status` 列 + 写路径错误捕获） | MEM | 2d | DDL + sync 错误处理 |
| 4 | **TG-Q-01**（提取 `biz.TeamRunStatus*` 常量） | TG | 1d | `biz/team_run_status.go` |
| 5 | **TPM-P1-02 / P1-11 / P1-07**（XS 修复 3 项） | M57 | 1d | runtime alias / mcp probe / skill zipslip |

**Gate 1.1**：
- `BackgroundJob` 表上线，CRUD + `TryClaim` 单测通过
- Bus 路由表灰度 dual 模式下 flow_log 双发，监控页无可见变化
- Memory fact 写路径异常 100% 标 `index_status='stale'`，告警可见
- `failed` vs `error` 字面量全库统一为 `biz.TeamRunStatus*`

#### Sprint 1.2（第 3 周）— 业务可见的运维改善

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 6 | **MON-OPT-04 优先级 channel + drop metric** | MON | 3d | WS high/normal/low 三优先级 |
| 7 | **MON-OPT-02 firing 状态机 + DB 持久化冷却** | MON | 2d | `monitor_alert_rules` 加列 |
| 8 | **MEM-OPT-03 队列优先级 + Dead-Letter 表** | MEM | 3d | `MemoryJobQueue` 三档 + DL |
| 9 | **TG-Q-02**（`CleanupStaleSessions` 接入 cron） | TG | 0.5d | wire ticker |
| 10 | **TPM-P1-05 / P1-04**（plugin chain panic recover / output policy） | M57 | 2d | hook resilience |

**Gate 1.2**：
- WS 反压 metric 上 Datadog；告警优先级永不被丢
- 重启 / 多实例下告警 Webhook 重复率 0%
- Memory 高负载 session 不再静默失忆，dead-letter 可见
- Team Graph 长跑进程 Coordinator.sessions 不积累

#### Sprint 1.3（第 4 周）— Dispatcher + 评估批量化

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 11 | **M56 BLO-5 Sprint A2**（Dispatcher + 双 worker 池 + DAG） | M56 | 1 周 | runtime/backgroundjob/ |
| 12 | **MON-OPT-03 RingBuffer + EvalWorker** | MON | 3d | 评估 DB QPS -99% |
| 13 | **MEM-OPT-01 Phase 2/3**（读路径校验 + Reconciler cron） | MEM | 3d | pgvector 一致性收敛 |
| 14 | **TPM-P1-10 + P1-12**（MCP transport 归一化） | M57 | 2d | NormalizeTransport |
| 15 | **TPM-P1-01**（web_search alias 对齐） | M57 | 0.5d | runtime alias 表收敛 |

**Gate 1.3**：
- Dispatcher 100 Job 含 parent/child 全正确流转
- 1000 QPS completion 下评估 CPU < 5%
- Cascade Approve → pgvector 收敛时延 ≤ 15s

**Phase 1 总体 Gate**：
- ✅ MEM-OPT-01 / 03 完成（Memory 业务正确性 P0 消除）
- ✅ MON-OPT-01 / 02 / 03 / 04 完成（Monitor 4 项 P1 消除）
- ✅ M56 BLO-5 Sprint A1/A2 完成（BackgroundJob 基础就绪）
- ✅ 11 项 M57 P1 / TG-Q-01/02 速胜
- ✅ 所有改动有 Feature flag，可回滚

---

### Phase 2 — 异步任务 + 业务能力提升（4 周）

> **目标**：把 Phase 1 基础设施转化为业务能力（HITL 不阻塞 / 智能升级 / 反馈强化）。

#### Sprint 2.1（第 5 周）— BackgroundJob 接入 + Trace 写入

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 16 | **M56 BLO-5 Sprint A3**（迁移 Channel/Chat 异步至 BackgroundJob） | M56 | 1 周 | SessionRun durable + Channel async 接入 |
| 17 | **MON-OPT-05 MonitorTraceProjector** | MON | 3d | `monitor_traces` 100% 落行 |
| 18 | **TPM-P1-03 / P1-06**（cost_guard / skill summary） | M57 | 1.5d | double-block / slug 修复 |

**Gate 2.1**：
- `GET /v1/background-jobs` 统一返回 Channel + Session 任务
- Traces Tab 不再空，所有新 turn 落 trace 行

#### Sprint 2.2（第 6-7 周）— HITL 异步 + Escalation 智能化

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 19 | **M56 BLO-4**（Non-Blocking HITL 全套，3 Sprint 合并） | M56 | 2.5 周 | PendingTask 异步 |
| 20 | **M56 BLO-2**（Multi-Signal Escalation，并行） | M56 | 1 周 | EscalationPolicy |
| 21 | **MEM-OPT-02 L4 Decay Worker + 强化因子（无 UI）** | MEM | 1 周 | confidence 业务化公式 |
| 22 | **TG-Q-03 / TG-Q-05**（拆 620 行 + 移除 chatactivity 依赖） | TG | 3d | God function 收敛 |

**Gate 2.2**：
- `await_user_reply` 期间同 session 新 turn 可执行
- tool_calls=9 自动升 durable
- L4 entity 半年不活跃 confidence ≤ 0.4

#### Sprint 2.3（第 8 周）— 提取协议化 + Trace 关联

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 23 | **MEM-OPT-05**（function call schema 双轨提取） | MEM | 1 周 | extraction_quality 字段 |
| 24 | **MON-OPT-05 跨 trace 关联** | MON | 3d | parent_trace_id |
| 25 | **TG-Q-04**（删除幽灵函数 / 改写 E2E 直驱 watch） | TG | 1d | 真路径测试 |
| 26 | **TPM-P1-08 / P1-09**（skill Saga apply / mcp OAuth） | M57 | 1 周 | Wave 1 收口 |

**Gate 2.3**：
- LLM 提取解析成功率 ≥ 99.5%；heuristic 占比 < 5%
- Run 详情 Waterfall 跨 turn 可跳转

**Phase 2 总体 Gate**：
- ✅ M56 BLO-2/4/5 全完成
- ✅ MEM-OPT-02/05 完成
- ✅ MON-OPT-05 完成
- ✅ TG-Q-03/04/05 完成
- ✅ M57 Wave 1 全部 12 项 P1 完成

---

### Phase 3 — 体验与生态扩展（5 周）

> **目标**：用户/Admin 可见的产品能力扩展（Cascade Saga / PII 分级 / 群智能体 / Intent admission / 自定义告警 DSL）。

#### Sprint 3.1（第 9-10 周）— Cascade Saga + PII 升级

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 27 | **MEM-OPT-06**（Cascade Saga + Dry-Run + 前端 Tab 升级） | MEM | 3 周 | partial-fail 可恢复 |
| 28 | **MEM-OPT-04 PII Block 模式** | MEM | 1 周 | strict 合规可用 |
| 29 | **M56 BLO-1**（Intent-Aware Admission，3 Sprint） | M56 | 2.5 周 | classifier v0+v1 |
| 30 | **TG-Q-07**（critic_loop 协议化 → tool call） | TG | 2d | 字符串协议消除 |

#### Sprint 3.2（第 11-12 周）— Channel 触发器 + 告警 DSL + PII Review

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 31 | **M56 BLO-3**（Channel Trigger Rules，4 Sprint） | M56 | 4 周 | schedule/keyword/reaction/silent |
| 32 | **MON-OPT-06**（Registry + DSL 解析 + 旧规则迁移） | MON | 2 周 | 自定义 metric 不改代码 |
| 33 | **MEM-OPT-04 Review 工作流** | MEM | 2 周 | PII pending review Tab |
| 34 | **MON-OPT-02 escalation + silence_windows** | MON | 1 周 | 告警生命周期升级 |
| 35 | **TG-Q-06 / TG-Q-08 / TG-Q-09**（adaptive 裁剪可见 / watch 测试 / resume 错误暴露） | TG | 3d | P2 收口 |

#### Sprint 3.3（第 13 周）— MON Lossless + M57 Wave 2 部分

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 36 | **MON-OPT-04 Lossless 订阅模式** | MON | 3d | client 主动断重连 |
| 37 | **M57 Wave 2 安全 + 静默吞错组**（TPM-P2-08/11/12/27 + P2-01/02/13/21/25） | M57 | 2 周 | SerpAPI/PII/fail-open |

**Phase 3 总体 Gate**：
- ✅ M56 BLO-1/3 完成（5 主题全完成）
- ✅ MEM-OPT-04/06 完成（Memory 6 主题全完成）
- ✅ MON-OPT-06 完成（Monitor 6 主题全完成）
- ✅ TG-Q-06..09 完成（Team Graph P2 全部）
- ✅ M57 Wave 2 安全 + 静默吞错组完成

---

### Phase 4 — 架构升级与中长期愿景（按需启动，6-12 周）

> **目标**：进入"产品化 + 平台化"深水区。**Phase 1-3 完成后再评估是否启动**。

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 38 | **M57 Wave 2 死配置/性能组**（TPM-P2-03/09/10/14/15/18/26/30） | M57 | 2 周 | 配置清理 + 缓存 |
| 39 | **M57 Wave 3 重设计**（D-P1/P2/P4/T2/S1/S3/S4 + Schema-as-Code） | M57 | 4-6 周 | Cost Reservation / Hook Isolation 等 |
| 40 | **TG-Q-10..14**（代码质量收口 + 魔法常量配置化） | TG | 1 周 | P3 收口 |
| 41 | **M56 收口 Sprint F1/F2**（灰度 soak + 旧路径下线） | M56 | 3 周 | BLO 全量上线 |
| 42 | **M57 Wave 4 中长期**（EventSourcing / OPA / MCP FSM / Plugin Scope） | M57 | 8-12 周 | 平台化基础 |

---

## 4. 跨计划资源共享与并行机会

| 共享项 | 主题方 | 复用方 |
|--------|--------|--------|
| **Feature flag 框架** | M56 BLO-PRE-02 | MEM / MON / TPM / TG 共用 |
| **Datadog 看板** | M56 BLO-PRE-03 | 加面板即可 |
| **BackgroundJob 抽象** | M56 BLO-5 | MEM-OPT-03 队列可考虑迁；MON-OPT-03 EvalWorker 可注册为 scheduled job |
| **Function call schema** | MEM-OPT-05 | TG-Q-07（critic_loop）；MON 告警 enrichment |
| **Bus 路由表** | MON-OPT-01 | 所有发 Envelope 的模块 |
| **PolicyVersion 常量** | MEM-OPT 衍生 | 所有 ActionLog 写入方 |

**可并行 lanes**：

```
Lane A（基础设施）: M56 BLO-5 → BLO-4/2 → BLO-1/3
Lane B（Memory）  : MEM-OPT-01 → 03 → 02/05 → 04/06
Lane C（Monitor） : MON-OPT-01 → 04 → 02/03 → 05 → 06
Lane D（Tools）   : M57 Wave 1（小修速胜）→ Wave 2 → Wave 3
Lane E（Team）    : TG-Q-01/02/05（P1 速胜）→ 04/03 → 07/08/09
```

**最佳并行度**：5 lane 同跑需 ≥ 3 后端工程师 + 1 前端 + 1 QA。  
**单工程师串行**：按 Phase 1.1 → 1.2 → 1.3 → 2.x → 3.x 顺序，每周 1 sprint。

---

## 5. 推荐执行序列（Top-30 优先级展开）

| 顺序 | 任务 ID | 模块 | 体量 | 业务价值（一句话） | 依赖 |
|------|---------|------|------|------------------|------|
| 1 | M56 BLO-PRE-01..03 | infra | XS | 设计文档 + flag + 看板 | 无 |
| 2 | TG-Q-01 | team | XS | `failed`/`error` 常量化，Monitor 不漏判 | 无 |
| 3 | TPM-P1-02 / P1-11 / P1-07 | tools/mcp/skill | XS | 3 个 XS 修复速胜 | 无 |
| 4 | M56 BLO-5-BIZ-01..02 / DATA-01..03 | M56 | M | BackgroundJob 抽象与表 | BLO-PRE |
| 5 | MEM-OPT-01 Phase 0/1 | memory | S | fact `index_status` + 写错误捕获 | 无 |
| 6 | MON-OPT-01 Phase 0/1 | monitor | M | Bus 路由表 dual 模式灰度 | 无 |
| 7 | TG-Q-02 | team | XS | 调度 `CleanupStaleSessions` cron | 无 |
| 8 | MON-OPT-04 优先级 channel | monitor | S | WS 反压可观测；alert 永不丢 | 无 |
| 9 | MON-OPT-02 firing 状态机 | monitor | M | 重启不重发 Webhook | 无 |
| 10 | MEM-OPT-03 队列优先级 + DL | memory | M | 高峰不静默失忆 | 无 |
| 11 | TPM-P1-05 / P1-04 | plugin | M | hook panic recover / output policy | 无 |
| 12 | M56 BLO-5 Sprint A2 Dispatcher | M56 | L | DAG + cascade cancel | 步骤 4 |
| 13 | MON-OPT-03 RingBuffer 评估 | monitor | M | 监控 DB QPS -99% | 步骤 6 |
| 14 | MEM-OPT-01 Phase 2/3 | memory | M | 读校验 + Reconciler | 步骤 5 |
| 15 | TPM-P1-10/12/01 | mcp/tools | M | transport / web_search alias | 无 |
| 16 | M56 BLO-5 Sprint A3 接入 | M56 | L | Channel + Chat 异步统一 | 步骤 12 |
| 17 | MON-OPT-05 TraceProjector | monitor | M | Traces Tab 不再空 | 步骤 6 |
| 18 | TPM-P1-03/06 | plugin/skill | M | cost / summary 修复 | 步骤 11 |
| 19 | M56 BLO-4 PendingTask | M56 | L | HITL 不阻塞 session | 步骤 16 |
| 20 | M56 BLO-2 EscalationPolicy | M56 | M | 智能升级（与 19 并行） | 步骤 16 |
| 21 | MEM-OPT-02 L4 Decay Worker | memory | M | 业务化置信度 | 无 |
| 22 | TG-Q-03 / TG-Q-05 | team | M | 拆 620 行 + 解耦 chatactivity | 无 |
| 23 | MEM-OPT-05 function call schema | memory | L | 提取 99.5% 成功率 | 无 |
| 24 | MON-OPT-05 跨 trace 关联 | monitor | M | parent_trace_id | 步骤 17 |
| 25 | TG-Q-04 | team | S | 删幽灵函数 + 真路径测试 | 步骤 22 |
| 26 | TPM-P1-08 / P1-09 | skill/mcp | L | Saga apply / OAuth（Wave 1 收口） | 无 |
| 27 | MEM-OPT-06 Cascade Saga + Dry-Run | memory | L | partial-fail 可恢复 | 步骤 14 |
| 28 | M56 BLO-1 Intent Admission | M56 | L | classifier v0+v1 | 步骤 19 |
| 29 | M56 BLO-3 Channel Trigger Rules | M56 | XL | schedule/keyword/reaction/silent | 步骤 16 |
| 30 | MON-OPT-06 Registry + DSL | monitor | L | 自定义 metric 不改代码 | 步骤 13 |

> Top-30 之后回到对应需求文档自有排期：Phase 3 Sprint 3.1–3.3 + Phase 4。

---

## 6. 执行原则（贯穿所有 Phase）

| 原则 | 含义 |
|------|------|
| **Feature flag 优先** | 任何业务行为变化必须有 env / DB flag 控制；灰度可独立切换 |
| **DDL 加列默认值** | 不改既有列；新增列默认值不影响存量 |
| **静默失败必可观测** | `_ = err` 全部改为 `slog.Warn` + metric 或 status 字段持久化 |
| **路由表先行** | 改业务行为前先改路由 / 注册表（OPT-06 / OPT-01 类的设计） |
| **dual 灰度路径** | 新旧双写 → 双读校验 → 切单 → 下线（如 MON-OPT-01 dual→split） |
| **每 PR 一个 ID** | PR 标题前缀 `[BLO-X-YYY]` / `[TPM-P1-XX]` / `[MEM-OPT-N]` / `[MON-OPT-N]` / `[TG-Q-NN]` |
| **不顺带 refactor 相邻模块** | 严格按 `AGENTS.md` 执行纪律 |

---

## 7. 风险与中止条件

| 风险 | 触发条件 | 缓解 |
|------|---------|------|
| Sprint 1.1 BackgroundJob 抽象有 bug | A1 单测失败率 > 20% | 推迟 BLO-4/2/3，先稳基础 |
| MEM-OPT-01 读校验导致 SearchMemories 延迟翻倍 | p95 > 200ms | flag 关闭读校验，依赖 Reconciler |
| MON-OPT-01 Bus 路由表迁移期间 flow_log 丢失 | dual 期间双 Bus 都缺日志 | flag 回到 `MONITOR_BUS_ROUTING=session`（旧行为） |
| M56 BLO-5 旧表迁移失败 | 双写期间数据漂移 | 写新表为主 + 旧表只读快照 |
| 资源不足跨 lane 并行 | 团队 < 3 后端 | 退化为 Lane B+C 串行（先 MEM 再 MON） |

**中止 / 回退红线**：
- Phase 1 任一 Sprint Gate 失败 → 暂停后续 Sprint，先修
- 灰度租户 P95 延迟 > 1.5× 基线 → 立即回退该 flag
- 全量 `make ci` 失败 → 不进 Phase 2

---

## 8. 立即可启动的 Phase 1 Sprint 1.1 任务包

> **本节用于执行阶段**：以下 5 个任务可在第 1 周并行启动，相互无阻塞。

| # | 任务 ID | 我可以立刻动手的文件 | 预估 |
|---|---------|--------------------|------|
| 1 | **TG-Q-01** 状态常量 | 新建 `internal/biz/team_run_status.go` + 全库替换 `"failed"` / `"error"` | 1d |
| 2 | **TPM-P1-02** aliasTool 返 error | `internal/tools/runtime_alias.go` 1 行 | 0.5h |
| 3 | **TPM-P1-11** mcp probe CheckRedirect | `internal/mcp/probe/eval.go` ~10 行 | 1h |
| 4 | **TPM-P1-07** skill zipslip 加固 | `internal/skill/importer/engine.go` ~10 行 | 1h |
| 5 | **MEM-OPT-01 Phase 0** index_status DDL | `internal/data/sql/memory_chain.sql` + ent schema | 0.5d |

**5 个任务合计 ≤ 2 天工时**，全部 P1 / 体量 XS-S，无相互依赖，是路线图的**首批可立即合并 PR**。

---

## 9. 关联文档

- M56 BLO 主文档：[`56 business-logic-optimization.md`](./56%20business-logic-optimization.md)
- M56 BLO 开发计划：[`56-business-logic-optimization-development.md`](./56-business-logic-optimization-development.md)
- M57 TPM 开发计划：[`38-tools-plugin-skill-mcp-optimization.development.md`](./38-tools-plugin-skill-mcp-optimization.development.md)
- Memory OPT：[`memory/memory-optimization-2026-05-26.md`](./memory/memory-optimization-2026-05-26.md)
- Monitor OPT：[`18 monitor-optimization-2026-05-26.md`](./18%20monitor-optimization-2026-05-26.md)
- Team Graph Review backlog：[`../review/2026-05-26-Team-Graph-Code-Review.md`](../review/2026-05-26-Team-Graph-Code-Review.md)
- 系统级架构总览：[`0-system-diagram.md`](./0-system-diagram.md)
- 系统级开发计划：[`0-system.development.md`](./0-system.development.md)
- 红线规则：[`.cursor/rules/trpc-agent-framework-first.mdc`](../../.cursor/rules/trpc-agent-framework-first.mdc)
- 执行纪律：[`AGENTS.md`](../../AGENTS.md)


---

## 子模块：总览与路线图

> 基于 2026-05-31 全景评估，综合 OpenClaw / Hermes / trpc-agent-go 框架全量对比分析。

---

## 一、项目现状评估

### 1.1 规模指标

| 维度 | 数量 |
|------|------|
| 后端 internal/ 一级模块 | 35 |
| Agent 构建入口 | 3（LLM / A2A Proxy / Programmatic） |
| 工具注册 | 19 ToolRegistration + 16 子包 |
| 渠道平台 | 12（6 长连接 + 6 Webhook） |
| 记忆层级 | 5（L0/L2/L3/L4/Composite） |
| 定时任务 Worker | 17 |
| LLM Provider | 7 类型 + 3 Variant + 2 HA |
| Ent Schema | 57 |
| Usecase | 31 |
| Repository/端口接口 | 100+ |
| Service | 34 |
| 前端页面 | 41 |
| 前端 Store | 39 |
| 前端组件 | 180+（22 域） |
| 框架子系统 | 62 |

### 1.2 框架能力利用率

- **已使用**：34/62 子系统（~55%）
- **未使用但框架已具备**：28 个子系统
- **框架未具备需自研**：浏览器工具、Skill 市场

### 1.3 Human-Agent 达标度

| 特征 | 状态 | 达标度 |
|------|------|--------|
| 自主规划 | ⚠️ 有 Planner 但未深度集成 | 部分 |
| 工具使用 | ✅ 19 注册工具 + MCP + Skill | 基本达标 |
| 记忆积累 | ✅ 5 层记忆 + 自动提取 + 衰减 | 完全达标 |
| 自我纠错 | ⚠️ Graph 有熔断/恢复；Agent 层缺 RalphLoop | 部分 |
| 协作能力 | ✅ Team + A2A + Graph HITL | 完全达标 |
| 持续进化 | ⚠️ 学习闭环已实现（后端+前端）；技能自创建后端已实现、前端待集成 | 部分 |

**综合达标度：~70%**

### 1.4 竞品对比评分

| 平台 | 编排 | 记忆 | 渠道 | 工具 | 自主性 | 可观测 | UI | 总分 |
|------|------|------|------|------|--------|--------|-----|------|
| **Aranea** | 9 | 9 | 10 | 7 | 5 | 8 | 9 | **57/70** |
| OpenClaw | 5 | 5 | 4 | 8 | 5 | 7 | 2 | 36/70 |
| Hermes | 4 | 8 | 2 | 9 | 9 | 3 | 2 | 37/70 |

---

## 二、五阶段路线图

### Phase 1：补齐框架能力缺口（优先级 🔴 紧急）

> 目标：将框架已有但项目未集成的核心能力全部打通

| # | 模块 | 框架对应 | 预期收益 |
|---|------|---------|---------|
| 1.1 | RalphLoop 自我反思闭环 | `runner/ralph_loop.go` | Agent 自我纠错 |
| 1.2 | Guardrail 安全护栏 | `plugin/guardrail/` | 防止 Prompt 注入/不安全意图 |
| 1.3 | Evaluation 评估框架集成 | `evaluation/` | Agent 质量可量化 |
| 1.4 | 多模式 Agent 编排 | `agent/chainagent/` 等 | 串行/并行/循环编排 |
| 1.5 | FileTool 文件操作 | `tool/file/` | Agent 可读写/搜索/编辑文件 |
| 1.6 | WebFetch 与搜索工具 | `tool/webfetch/` + `tool/google/` | Agent 可浏览网页/搜索信息 |
| 1.7 | Artifact 云存储 | `artifact/s3/` + `artifact/cos/` | 生产级制品存储 |
| 1.8 | **L0 上下文压缩优化（阶段一）** | Claude Code 最佳实践 | 工具结果持久化 + 三层代价递进压缩 + 9章节摘要 + 手动压缩 |
| 1.9 | **工具调用可靠性增强** | Claude Code / arXiv 论文 / OpenClaw | 断路器 + 延迟加载 + 工具消歧 + 命令安全策略 |

### Phase 2：增强自主性（优先级 🟠 高）

> 目标：让 Agent 从"执行者"进化为"思考者"

| # | 模块 | 参考对象 | 预期收益 |
|---|------|---------|---------|
| 2.1 | Planner 深度集成 | 框架 `planner/a2ui/` | Agent 自主规划 |
| 2.2 | ClaudeCodeAgent 模式 | 框架 `agent/claudecode/` | 代码级自主操作 |
| 2.3 | 浏览器工具 | OpenClaw `browser-server/` | Agent 可操作网页 |
| 2.4 | SubAgent 后台派生 | OpenClaw `internal/subagentrun/` | 复杂任务自动拆分 |
| 2.5 | Outbound Router 出站路由 | OpenClaw `internal/outbound/` | Agent 主动通知 |
| 2.6 | Session 状态监控 | Trae 执行状态模型 | 执行中/完成/中断/等待确认实时状态 + 删除保护 + 优雅退出/异常恢复 |
| 2.7 | **L0 上下文压缩优化（阶段二）** | A-Mem / Mem0 / ROMEM | 记忆操作语义化 + 时间维度 + 动态链接 |

### Phase 3：进化能力（优先级 🟡 中）

> 目标：让 Agent 从"执行者"进化为"学习者"

| # | 模块 | 参考对象 | 预期收益 |
|---|------|---------|---------|
| 3.1 | 学习闭环 | Hermes Learning Loop | Agent 从经验中学习 |
| 3.2 | 技能自创建 | Hermes Skill Self-Creation | 能力自动增长 |
| 3.3 | Skill 市场生态 | Hermes agentskills.io / OpenClaw 60+ Skills | 社区驱动能力扩展 |
| 3.4 | Persona 角色系统 | OpenClaw `internal/persona/` | 多场景适配 |
| 3.5 | Runtime Profile 运行时配置 | OpenClaw `runtimeprofile/` | 不同场景不同配置 |
| 3.6 | **L0 上下文压缩优化（阶段三）** | Focus / Memento / Aider | Agent 自主压缩 + 代码骨架提取 |

### Phase 4：生产级增强（优先级 🟢 标准）

> 目标：从"能跑"到"敢上生产"

| # | 模块 | 框架对应 | 预期收益 |
|---|------|---------|---------|
| 4.1 | Session 分布式存储 | `session/postgres/` + `session/pgvector/` | 生产级会话持久化 |
| 4.2 | Memory 向量存储 | `memory/pgvector/` + `memory/mem0/` | 大规模向量记忆 |
| 4.3 | Graph Cache 与 Retry | `graph/cache.go` + `graph/retry.go` | 性能 + 容错 |
| 4.4 | OpenAI 兼容 API | `server/openai/` | 生态兼容 |
| 4.5 | Debug Recorder 调试录制 | OpenClaw `internal/debugrecorder/` | 开发调试效率 |
| 4.6 | Langfuse 可观测性 | OpenClaw `app/langfuse.go` | 生产级 Trace/评估 |
| 4.7 | CodeExecutor 云端沙箱 | `codeexecutor/e2b/` + `codeexecutor/jupyter/` | 安全代码执行 |

### Phase 5：差异化创新（优先级 🔵 远期）

> 目标：建立 Aranea 独特竞争力

| # | 模块 | 创新点 |
|---|------|--------|
| 5.1 | 行业 Agent 市场 | 按行业预置 Agent 模板 + Graph + Skill |
| 5.2 | 多模态 Agent | 语音/图像/视频输入输出 |
| 5.3 | Agent 工作流市场 | 类 n8n 的可视化工作流市场 |
| 5.4 | 联邦 A2A 网络 | 跨组织 Agent 协作网络 |
| 5.5 | Agent 评估认证 | 标准化评估 + 认证体系 |

---

## 三、文档索引

每个模块均包含三部分：**需求文档**、**设计文档**、**开发计划**。设计文档必须参考 trpc-agent-go 框架，结合本项目架构给出最优设计。

### Phase 1

| 模块 | 文档路径 |
|------|---------|
| RalphLoop 自我反思闭环 | [phase1-补齐框架能力缺口/01-RalphLoop自我反思闭环.md](./phase1-补齐框架能力缺口/01-RalphLoop自我反思闭环.md) |
| Guardrail 安全护栏 | [phase1-补齐框架能力缺口/02-Guardrail安全护栏.md](./phase1-补齐框架能力缺口/02-Guardrail安全护栏.md) |
| Evaluation 评估框架集成 | [phase1-补齐框架能力缺口/03-Evaluation评估框架集成.md](./phase1-补齐框架能力缺口/03-Evaluation评估框架集成.md) |
| 多模式 Agent 编排 | [phase1-补齐框架能力缺口/04-多模式Agent编排.md](./phase1-补齐框架能力缺口/04-多模式Agent编排.md) |
| FileTool 文件操作 | [phase1-补齐框架能力缺口/05-FileTool文件操作.md](./phase1-补齐框架能力缺口/05-FileTool文件操作.md) |
| WebFetch 与搜索工具 | [phase1-补齐框架能力缺口/06-WebFetch与搜索工具.md](./phase1-补齐框架能力缺口/06-WebFetch与搜索工具.md) |
| Artifact 云存储 | [phase1-补齐框架能力缺口/07-Artifact云存储.md](./phase1-补齐框架能力缺口/07-Artifact云存储.md) |
| **L0 上下文压缩优化** | [memory/L0-compression.md](./memory/L0-compression.md) · [设计](./memory/L0-compression.design.md) · [开发计划](./memory/L0-compression-development.md) |
| **工具调用可靠性增强** | [phase1-补齐框架能力缺口/08-工具调用可靠性增强.md](./phase1-补齐框架能力缺口/08-工具调用可靠性增强.md) |

### Phase 2

| 模块 | 文档路径 |
|------|---------|
| Planner 深度集成 | [phase2-增强自主性/01-Planner深度集成.md](./phase2-增强自主性/01-Planner深度集成.md) |
| ClaudeCodeAgent 模式 | [phase2-增强自主性/02-ClaudeCodeAgent模式.md](./phase2-增强自主性/02-ClaudeCodeAgent模式.md) |
| 浏览器工具 | [phase2-增强自主性/03-浏览器工具.md](./phase2-增强自主性/03-浏览器工具.md) |
| SubAgent 后台派生 | [phase2-增强自主性/04-SubAgent后台派生.md](./phase2-增强自主性/04-SubAgent后台派生.md) |
| Outbound Router 出站路由 | [phase2-增强自主性/05-OutboundRouter出站路由.md](./phase2-增强自主性/05-OutboundRouter出站路由.md) |

### Phase 3

| 模块 | 文档路径 |
|------|---------|
| 学习闭环 | [phase3-进化能力/01-学习闭环.md](./phase3-进化能力/01-学习闭环.md) |
| 技能自创建 | [phase3-进化能力/02-技能自创建.md](./phase3-进化能力/02-技能自创建.md) |
| Skill 市场生态 | [phase3-进化能力/03-Skill市场生态.md](./phase3-进化能力/03-Skill市场生态.md) |
| Persona 角色系统 | [phase3-进化能力/04-Persona角色系统.md](./phase3-进化能力/04-Persona角色系统.md) |
| Runtime Profile 运行时配置 | [phase3-进化能力/05-RuntimeProfile运行时配置.md](./phase3-进化能力/05-RuntimeProfile运行时配置.md) |

### Phase 4

| 模块 | 文档路径 |
|------|---------|
| Session 分布式存储 | [phase4-生产级增强/01-Session分布式存储.md](./phase4-生产级增强/01-Session分布式存储.md) |
| Memory 向量存储 | [phase4-生产级增强/02-Memory向量存储.md](./phase4-生产级增强/02-Memory向量存储.md) |
| Graph Cache 与 Retry | [phase4-生产级增强/03-GraphCache与Retry.md](./phase4-生产级增强/03-GraphCache与Retry.md) |
| OpenAI 兼容 API | [phase4-生产级增强/04-OpenAI兼容API.md](./phase4-生产级增强/04-OpenAI兼容API.md) |
| Debug Recorder 调试录制 | [phase4-生产级增强/05-DebugRecorder调试录制.md](./phase4-生产级增强/05-DebugRecorder调试录制.md) |
| Langfuse 可观测性 | [phase4-生产级增强/06-Langfuse可观测性.md](./phase4-生产级增强/06-Langfuse可观测性.md) |
| CodeExecutor 云端沙箱 | [phase4-生产级增强/07-CodeExecutor云端沙箱.md](./phase4-生产级增强/07-CodeExecutor云端沙箱.md) |

### Phase 5

| 模块 | 文档路径 |
|------|---------|
| 行业 Agent 市场 | [phase5-差异化创新/01-行业Agent市场.md](./phase5-差异化创新/01-行业Agent市场.md) |
| 多模态 Agent | [phase5-差异化创新/02-多模态Agent.md](./phase5-差异化创新/02-多模态Agent.md) |
| Agent 工作流市场 | [phase5-差异化创新/03-Agent工作流市场.md](./phase5-差异化创新/03-Agent工作流市场.md) |
| 联邦 A2A 网络 | [phase5-差异化创新/04-联邦A2A网络.md](./phase5-差异化创新/04-联邦A2A网络.md) |
| Agent 评估认证 | [phase5-差异化创新/05-Agent评估认证.md](./phase5-差异化创新/05-Agent评估认证.md) |

---

## 四、架构原则

> 架构原则已迁移到 [0-system-diagram.md §十三](./0-system-diagram.md)。本节仅保留与开发计划相关的执行约束。

开发执行约束（与架构原则配套使用）：

- 所有模块设计必须遵循 [0-system-diagram.md §十三](./0-system-diagram.md) 中的架构原则。
- 任务拆解时，先确认模块五面（见 [0-system-diagram.md §十二](./0-system-diagram.md)），再判断任务类型。
- 边界 PR 不混功能，功能 PR 不混 UI 大重构（见本文 §7）。

---

## 五、关键里程碑

| 里程碑 | 预期阶段 | 标志性成果 |
|--------|---------|-----------|
| M1: 框架对齐完成 | Phase 1 结束 | 框架能力利用率从 55% 提升到 80% |
| M2: 自主规划上线 | Phase 2 结束 | Agent 可自主规划并执行复杂任务 |
| M3: 进化闭环跑通 | Phase 3 结束 | Agent 可从经验中学习并创建新技能 |
| M4: 生产就绪 | Phase 4 结束 | Postgres + 云存储 + 完整可观测性 |
| M5: 生态成型 | Phase 5 结束 | 行业市场 + 联邦网络 + 评估认证 |
