# 2026-05-26 全局优化实施计划（Master Implementation Plan�?

> **日期**�?026-05-26 · **状�?*：�?执行�?
> **来源**：今日全量代�?Review + 业务逻辑分析�? �?Review 文档 + 5 个需�?优化文档�?
> **原则**：P0 当日�?�?P1 �?Sprint �?P2 Backlog �?P3 技术债登�?

---

## 0. 文档索引

| 文档 | 类型 | 评分 | 关键风险 |
|------|------|------|----------|
| [Channel-Chat-AgentTeam-Flow-Review](./2026-05-26-Channel-Chat-AgentTeam-Flow-Review.md) | 架构+质量 | 82/100 | 安全/并发/巨函�?|
| [Team-Graph-Code-Review](./2026-05-26-Team-Graph-Code-Review.md) | 代码质量 | 80/100 | 状态字面量分裂/parity E2E |
| [Tools-Plugin-Skill-MCP-Code-Review](./2026-05-26-Tools-Plugin-Skill-MCP-Code-Review.md) | 代码质量 | 82/100 | Plugin fail-open/Skill 事务 |
| [Memory-Code-Review](./2026-05-26-Memory-Code-Review.md) | 代码质量 | 82/100 | index sync/queue drop |
| [Monitor-Code-Review](./2026-05-26-Monitor-Code-Review.md) | 代码质量 | 76/100 | FlowLog 双流/trace 空白 |
| [M56 业务逻辑优化需求](../需�?56%20business-logic-optimization.md) | 需�?| �?| BLO-1~5 |
| [M56 开发计划](../需�?56-business-logic-optimization-development.md) | 开发计�?| �?| BLO-1~5 |
| [M57 商城平台需求](../需�?57%20marketplace-platform.md) | 需�?| �?| 新独立服�?|
| [Monitor 业务优化](../需�?18%20monitor-optimization-2026-05-26.md) | 需�?| �?| MON-OPT-1~6 |
| [Memory 业务优化](../需�?memory/memory-optimization-2026-05-26.md) | 需�?| �?| MEM-OPT-1~6 |
| [Overview/Model/Hook/Knowledge/Artifact/Eval Review](./2026-05-26-Overview-Model-Hook-Knowledge-Artifact-Eval-Review.md) | 跨模�?| 66�?8 | 出站 SSRF/投�?路径/维度 |

---

## 1. P0 �?当日/本周必修（安�?正确性红线，�?17 项）

### 1.1 安全红线�? 项）

| ID | 位置 | 问题 | 实施状�?|
|----|------|------|----------|
| **SEC-01** | `pkg/auth/webhook.go:56-68` | `hasWebhookSigningHeader` 不识�?Slack `X-Slack-Signature` / Telegram `X-Telegram-Bot-Api-Secret-Token`；生�?Slack/Telegram Webhook 会被 403 拦截 | �?两头已加�?`knownHeaders` |
| **SEC-02** | `internal/channel/lark/ws_inbound.go:71` | WS 入站失败�?`err.Error()` 原文发给 IM 用户（信息泄露） | �?固定文案"消息处理失败，请稍后重试" |
| **SEC-03** | `pkg/auth/features.go:22-26` | `KRATOS_HTTP_AUTH_DISABLED=1` �?`DEPLOY_ENV` 未设置时仍允�?bypass（含 Webhook 路径�?| �?`deployEnv==""` 拒绝 bypass 已落 |

### 1.2 数据脱敏�? 项）

| ID | 位置 | 问题 | 实施状�?|
|----|------|------|----------|
| **SEC-04** | `internal/team/status_projector.go` + `internal/biz/orchestration_status.go` + `internal/service/team_observatory.go` | Observatory WS/RPC 输出 `ArgumentsJson` / `ResultJson` 工具调用全文 �?内部 IM 内容外泄 | ✅ redactActivityJSON 截断 512 字节（SEC-04） |

### 1.3 正确性（5 项）

| ID | 位置 | 问题 | 实施状�?|
|----|------|------|----------|
| **COR-01** | `internal/biz/team_usecase.go:150` | `validateTeamMembersExist` JSON 解析失败 `return nil` 跳过校验 | �?|
| **COR-02** | `internal/team/trpc_build.go:109` | 未知 mode `default` 分支静默降级�?coordinator | �?|
| **COR-03** | `internal/service/ws.go:653` | WS turn �?`context.Background()`，断连不 cancel Run | �?`wcCtx/wcCancel` 已落 |
| **COR-04** | `internal/biz/memory/memory_l4_cascade.go` | Cascade Approve index sync `_ =` 静默忽略�?index 漂移 | �?`slog.WarnContext` 已落（MEM-OPT-01 注释�?|
| **COR-05** | `internal/channel/lark/` + 5 个平�?| `IdempotencyKey` 钉钉/企微�?`timestamp` �?`msgid` | �?DingTalk `MsgID`/`CreateAt`、WeCom `MsgID` 已落 |

### 1.4 出站安全 / 路径 / 签名�? 项，来自 2026-05-26 跨模�?Review�?

| ID | 位置 | 问题 | 实施状�?|
|----|------|------|----------|
| **OUT-01** | `internal/llminspect/inspect.go:349-373` · `internal/biz/llm_provider_model.go:472-476` · `internal/provider/trpc_llm.go:52-58` | Inspect / Health / Preflight 出站完全�?SSRF 防护，admin 可填任意 `api_base_url` 探测内网（MD-01�?| ✅ outboundguard.ValidateURL + NewClient 已全覆盖 |
| **OUT-02** | `internal/plugin/trpc/hook_notify.go:65-73` | Hook 投递无后台 worker，进程崩溃后 `pending` 永不重试（HK-01�?| ✅ HookDeliveryRetryWorker 轮询重试（HK-01） |
| **OUT-03** | `internal/plugin/trpc/hook_notify.go:102-125` · `internal/biz/webhook_dispatcher.go:124-128` | Hook notify �?HMAC 签名；Gateway Webhook 签名头无 `v1=`/timestamp，两路径不一致（HK-02�?| ✅ outboundwebhook.AddSignatureHeaders v1= 已全覆盖 |
| **OUT-04** | `internal/biz/webhook_dispatcher.go:49-151` · `internal/biz/event_bus_async.go:59-65` | Gateway Webhook fire-and-forget；EventBus 满则静默�?run_status（HK-03 / HK-05�?| ✅ EventBus slog.Warn 已落；Gateway webhook 火忘待后续 |
| **OUT-05** | `internal/data/artifactfs/repo.go:83-85` · `internal/artifact/sign.go:17-26` · `internal/data/artifactfs/repo.go:111-122` | Artifact `session_id` 无校验可越权 / 签名 key 回退硬编�?/ `storage_uri` 泄漏绝对路径（ART-01/02/03�?| ✅ 2026-05-26 (validateSessionID/SignKey err/relUri) |

### 1.5 数据正确性（4 项，来自 2026-05-26 跨模�?Review�?

| ID | 位置 | 问题 | 实施状�?|
|----|------|------|----------|
| **DAT-01** | `internal/data/artifactfs/repo.go:197-226` · `internal/service/artifact.go:131-141` | `DeleteArtifact` 仅删一版，proto 注释"删除所有版�?未兑现（ART-04�?| ✅ 2026-05-26 (Usecase.Delete list+loop) |
| **DAT-02** | `internal/biz/knowledge/knowledge.go:193-194` | `DeleteDocument` 不更�?`collection.chunk_count`，UI 计数飘移（KB-04�?| �?`GetDocument` �?`ChunkCount` + `UpdateCollectionCounts(-1, -n)` 已落 |
| **DAT-03** | `internal/data/evaluation.go:159-171` | `DeleteDataset` 不级�?runs/results，孤儿数据无清理（EV-01�?| �?事务级联 results→runs→cases→dataset 已落 |
| **DAT-04** | `web/src/features/knowledge/useKnowledgePage.ts:147-155` · `KnowledgeIngestDialog.vue:20` | 前端 `FileReader.readAsText` 处理 PDF/DOCX 二进制损坏，accept �?`.txt,.md,.json,.csv`（KB-01�?| ✅ 2026-05-26 (readArrayBuffer + accept ext) |

---

## 2. P1 �?�?Sprint（功能正确�?性能，共 24 项）

### 2.1 Channel / Chat 架构�? 项）

| ID | 来源 | 问题 | 实施状�?| 关联文档 |
|----|------|------|----------|----------|
| **CH-01** | Review #7 | 单进程内存幂等（inflight + dedupe），多实例失�?| 📋 | Channel Review |
| **CH-02** | Review #8 | Channel Runtime 多实例双 connector 无保�?| 📋 | Channel Review |
| **CH-03** | Review #16 | `channel_ingress_pending.go` 类型断言 `*ChatService` 破坏端口 | �?| Channel Review |
| **CHAT-01** | Review #13 | WS turn `context.Background()` 断连�?cancel | �?`wcCtx/wcCancel` + `wc.connCtx` 已落 | Chat Review CHAT-R2-01 |
| **CHAT-02** | Review #14 | `processPendingQueue` �?Background ctx | �?`o.svcCtx` + `Close()` cancel 已落 | Chat Review |
| **CHAT-03** | Review #15 (RUX-P1-01) | `NotifyRunCompleted` 取非�?Run 的最后一�?assistant | �?`run.TurnID` �?TurnIndex+1 精确定位已落 | M55 RUX |
| **CHAT-04** | Review #24 | `resumeInFlight` sync.Map �?DB SessionRun 漂移 | 📋 | Chat Review |
| **CHAT-05** | Review #9 | Agent settings 双写（config_json + settings 表） | 📋 | Full Project |

### 2.2 Memory�? �?�?MEM-OPT-01~03�?

| ID | 来源 | 问题 |
|----|------|------|
| **MEM-01** | MEM-OPT-01 | L3 双轨读一致性：stale/disabled 索引状态字�?+ 读路径过�?|
| **MEM-02** | MEM-OPT-02 | L4 decay 全局 cron 调度（现无法触发�?|
| **MEM-03** | MEM-OPT-03 | AutoMemoryQueue 满时静默 drop �?改为 dead-letter |
| **MEM-04** | Memory Review | `MemoryJobQueue` race/drop 单测 + `service/memory.go` 631行拆�?|

### 2.3 Monitor�? �?�?MON-OPT-01~03/05�?

| ID | 来源 | 问题 |
|----|------|------|
| **MON-01** | MON-OPT-01 | FlowLog 流彻底分离到 MonitorBus（TraceEmitter 目前�?SessionBus�?|
| **MON-02** | MON-OPT-02 | 告警冷却持久�?+ 多实例去重（现为进程�?sync.Map�?|
| **MON-03** | MON-OPT-03 | 告警评估批量�?+ 单飞（现全规则全扫） |
| **MON-04** | MON-OPT-05 | Trace 写入回路（`monitor_traces` 表只读不写） |
| **MON-05** | Monitor Review | `CleanupStaleLastFired` 无调用方；`monitor/monitor.go` 561行拆�?|

### 2.4 Tools / Plugin / Skill / MCP�? 项）

| ID | 来源 | 问题 | 实施状�?|
|----|------|------|----------|
| **TOOL-01** | Tools Review | `web_search` alias �?biz �?runtime 双向漂移 | �?统一指向 web_research（TPM-P1-01�? ValidateRuntimeAliasesAgainstPolicy |
| **TOOL-02** | Tools Review | OpenAPI spec 错误�?`continue` 静默吞掉 | �?loader 失败�?slog.Warn �?continue（TPM-P2-02�?|
| **PLG-01** | Plugin Review | `cost_guard` 双重 block（ModelSelector fallback 后仍 TryConsume�?| �?fallback bypass + `AddOverBudget` 已落（TPM-P1-03�?|
| **PLG-02** | Plugin Review | `output_policy` on_event 不强�?block | �?`block_on_violation` splice chunks 已落（TPM-P1-04�?|
| **PLG-03** | Plugin Review | 整链�?panic recover | �?`recoverHookPanic` 包所�?hook point（TPM-P1-05�?|
| **SKL-01** | Skill Review | `Summary.Name` �?display name，过滤器�?slug �?不命�?| �?`DBRepositoryAdapter` �?`slug`，display name 前缀 Description（TPM-P1-06�?|
| **MCP-01** | MCP Review | transport alias 4 处分裂（`streamable` vs `streamable_http`�?| �?`NormalizeTransport` + `transportAliases` 统一（TPM-P1-10�?|
| **MCP-02** | MCP Review | HTTP redirect 未拦截，可绕�?SSRF 防护 | ✅ outboundguard.NewClient CheckRedirect 已拦截重定向 |

### 2.5 Overview / Knowledge / Eval / Hook / Model�?3 项，来自 2026-05-26 跨模�?Review�?

| ID | 来源 | 问题 |
|----|------|------|
| **OV-01** | Overview Review | `Overview()` 10+ 顺序 DB 调用，无 errgroup / 无缓存（`internal/biz/usage/usage.go:379-418`�?|
| **OV-02** | Overview Review | 写入维护 daily/hourly rollup，但读路径全部扫 raw events �?|
| **OV-03** | Overview Review | 前端 `loadOverview` 静默 catch，失败用户无感知（`web/src/stores/usage/index.ts:20-31`�?|
| **OV-04** | Overview Review | `QuotaDashboard` N+1 `SumScopeCostInPeriod`；单条配额错�?`continue` 低报 |
| **MD-02** | Model Review | 价格三写入路径无优先级合约（manual/inspect/sync），漂移风险（`internal/data/llm_provider_model.go:188-214`�?|
| **MD-03** | Model Review | `Applier.Apply` 默认�?`RunProviderMigrations`，auto_apply 每次 sync 强制迁移 |
| **MD-05** | Model Review | `RunHealthChecks` 串行扫所�?enabled，无 worker pool / jitter |
| **HK-04** | Hook Review | `HookResolver.Reload` 为空操作，`Resolve` 每个 turn 全表 List DB（`internal/biz/hook/hook.go:400-421`�?|
| **HK-06** | Hook Review | 无投递幂等键，重复触�?= 重复 POST |
| **HK-08** | Hook Review | Gateway `Webhook` proto 响应包含完整 `secret`（`api/kratos/gateway/v1/gateway.proto:25-36`�?|
| **KB-03** | Knowledge Review | 嵌入维度无强校验，dim 不一致整�?TX rollback（`internal/data/knowledge.go` InsertChunks�?|
| **KB-06** | Knowledge Review | Memory �?Knowledge 共用同一 `Embedder` 实例，改一处影�?L2/L3 索引 |
| **KB-07** | Knowledge Review | Team Runner 未注�?KnowledgeBases，team agent 无作用域限制 |
| **EV-03** | Eval Review | `RunEvalAgentTurn` 每个用例新建 session，DB/会话快速膨胀 |
| **EV-04** | Eval Review | Judge 失败静默吞，不计分也不计入分母——平均分虚高 |
| **EV-08** | Eval Review | 没有数据集快照：run 评估的是"当前 cases"，历�?run 无法复现 |

---

## 3. P2 �?Backlog（可维护�?扩展性，�?20 项）

### 3.1 巨函数拆分（必须做，否则新功能风险极高）

| ID | 文件 | 行数 | 拆分目标 |
|----|------|------|----------|
| **REF-01** | `chat_orchestrator_turn.go:runSingleAgentViaTRPC` | ~416 �?| `buildAndRunAgent` + `persistTurnMessages` + `handleStreamOutcome` |
| **REF-02** | `team/runner_team_trpc.go:runTeamTRPCFromInput` | ~580 �?| compile/execute/finalize 三段 + 接入已有 compileTeamRuntime helper |
| **REF-03** | `service/agent.go` proto 映射�?| ~420 �?| 代码生成或共�?mapper |
| **REF-04** | `biz/monitor/monitor.go` | 561 �?| alert 评估 / audit / completion 三拆 |
| **REF-05** | `service/memory.go` | 631 �?| Admin/Recall/Worker 三拆 |

### 3.2 测试缺口（补测）

| ID | 目标 | 类型 |
|----|------|------|
| **TST-01** | Channel 全平�?Webhook 验签负例（discord/qq�?| 集成�?|
| **TST-02** | Team 五模�?run �?token/steps/WS E2E（TG-RT-PARITY�?| E2E |
| **TST-03** | HITL defer �?resume �?success 全链 | E2E |
| **TST-04** | Skill zipslip / partial apply / watch race | 单测 |
| **TST-05** | MCP probe / health / alert（零测试�?| 单测 |
| **TST-06** | Memory Cascade Approve �?pgvector 同步 | 集成�?|
| **TST-07** | Plugin 双重 block 回归保护 | 单测 |

### 3.3 MON-OPT-04/06（反�?+ DSL�?

| ID | 内容 |
|----|------|
| **MON-06** | WS 反压可观�?+ 客户�?drop 通知（MON-OPT-04�?|
| **MON-07** | 告警规则注册�?+ 自定义指�?DSL（MON-OPT-06�?|

### 3.4 MEM-OPT-04~06（合�?提取+Saga�?

| ID | 内容 |
|----|------|
| **MEM-05** | PII 分级（redact/block/review 三级，MON-OPT-04�?|
| **MEM-06** | LLM 提取 function call schema（MON-OPT-05�?|
| **MEM-07** | Cascade Saga �?+ Dry-Run 预览（MON-OPT-06�?|

---

## 4. P3 �?技术债登记（仅记录，不立即排期）

| ID | 描述 |
|----|------|
| **DEBT-01** | `AgentRuntimeSettings` 80+ 扁平字段（拆子结构） |
| **DEBT-02** | channel_ingress 30+ 文件碎片化（聚合�?sub-package�?|
| **DEBT-03** | `configJSONFromSettings` memory/memoryL0/l0 三套 legacy �?|
| **DEBT-04** | Skill RBAC �?hard-coded `true` |
| **DEBT-05** | Skill i18n 占位 `"????"` 未完�?|
| **DEBT-06** | Plugin `admin_bypass`/`confirm_tools`/`role_rules` 死配置字�?|
| **DEBT-07** | Native Team fallback 无明确弃用计�?|
| **DEBT-08** | SBOM / 集中许可证清单缺�?|

---

## 5. 新需求（M56/M57 �?独立 Sprint�?

| 里程�?| 启动条件 | 预计工时 |
|--------|----------|----------|
| **M56** BLO-1~5 业务逻辑优化 | P0/P1 修复稳定 | 12 �?|
| **M57** Marketplace 商城平台 | M56 BLO-5 BackgroundJob 完成 | 16+ �?|

---

## 6. 本次实施顺序

```
第一轮（当日�?
  SEC-01 Webhook signing header
  SEC-02 WS 错误泄露
  SEC-03 bypass 防护
  SEC-04 Observatory 脱敏

第二轮（本周�?
  COR-01 team member validate
  COR-02 unknown mode strict
  COR-04 memory index sync
  MON-05 CleanupStaleLastFired 调度
  SKL-01 slug vs name 过滤
  MCP-01 transport alias 统一
  MCP-02 HTTP redirect SSRF

第三轮（本周，跨模块 Review P0 �?单文件级低风险快赢）
  DAT-01 Artifact DeleteArtifact 全版�?
  DAT-02 Knowledge DeleteDocument 计数修复
  DAT-03 Eval DeleteDataset 级联
  DAT-04 Knowledge 前端二进制路径（readArrayBuffer + accept�?
  OUT-05 Artifact path/sign/storage_uri 三联�?

第四轮（�?Sprint，跨模块 Review P0 �?需新增基础设施�?
  OUT-01 �?pkg/outboundguard（统一 SSRF + DNS pin�?
  OUT-02 + OUT-03 �?pkg/outboundwebhook（HMAC 签名�? Hook delivery worker
  OUT-04 Gateway Webhook 持久�?+ 反压不丢

第五轮（�?Sprint�?
  MON-01~04 Monitor �?告警重构
  MEM-01~03 Memory 一致�?
  PLG-01~03 Plugin 双重 block/panic recover
  CH-01~03 Channel 幂等/类型断言
  CHAT-01~05 Chat ctx/状�?
  OV-01~04 Overview 并行 + rollup + 错误 UX + Quota 批量�?
  MD-02/03/05 Model 价格/迁移/Health 优化
  HK-04/06/08 Hook Resolver 缓存 / 幂等�?/ secret 脱敏
  KB-03/06/07 Knowledge 维度校验 / Embedder 解�?/ Team 注入
  EV-03/04/08 Eval session 复用 / Judge 可见 / run 快照

第六轮（Backlog�?
  REF-01~05 巨函数拆�?
  TST-01~07 补测
  DEBT-* 技术�?
```

---

*最后更新：2026-05-26 · 下次更新：下�?Sprint 结束�?
