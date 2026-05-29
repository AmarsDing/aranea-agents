# 2026-05-26 全局优化实施计划（Master Implementation Plan）

> **日期**：2026-05-26 · **状态**：🟡 执行中
> **来源**：今日全量代码 Review + 业务逻辑分析 → Review 文档 + 5 个需求优化文档
> **原则**：P0 当日修、P1 下 Sprint、P2 Backlog、P3 技术债登记

---

## 0. 文档索引

| 文档 | 类型 | 评分 | 关键风险 |
|------|------|------|----------|
| [Channel-Chat-AgentTeam-Flow-Review](./2026-05-26-Channel-Chat-AgentTeam-Flow-Review.md) | 架构+质量 | 82/100 | 安全/并发/巨函数 |
| [Team-Graph-Code-Review](./2026-05-26-Team-Graph-Code-Review.md) | 代码质量 | 80/100 | 状态字面量分裂/parity E2E |
| [Tools-Plugin-Skill-MCP-Code-Review](./2026-05-26-Tools-Plugin-Skill-MCP-Code-Review.md) | 代码质量 | 82/100 | Plugin fail-open/Skill 事务 |
| [Memory-Code-Review](./2026-05-26-Memory-Code-Review.md) | 代码质量 | 82/100 | index sync/queue drop |
| [Monitor-Code-Review](./2026-05-26-Monitor-Code-Review.md) | 代码质量 | 76/100 | FlowLog 双流/trace 空白 |
| [M56 业务逻辑优化需求](../需求/56%20business-logic-optimization.md) | 需求 | — | BLO-1~5 |
| [M56 开发计划](../需求/56-business-logic-optimization-development.md) | 开发计划 | — | BLO-1~5 |
| [M57 商城平台需求](../需求/57%20marketplace-platform.md) | 需求 | — | 新独立服务 |
| [Monitor 业务优化](../需求/18%20monitor-optimization-2026-05-26.md) | 需求 | — | MON-OPT-1~6 |
| [Memory 业务优化](../需求/memory/memory-optimization-2026-05-26.md) | 需求 | — | MEM-OPT-1~6 |
| [Overview/Model/Hook/Knowledge/Artifact/Eval Review](./2026-05-26-Overview-Model-Hook-Knowledge-Artifact-Eval-Review.md) | 跨模块 | 66~78 | 出站 SSRF/投毒路径/维度 |

---

## 1. P0 — 当日/本周必修（安全+正确性红线，共 17 项）

### 1.1 安全红线（3 项）

| ID | 位置 | 问题 | 实施状态 |
|----|------|------|----------|
| **SEC-01** | `pkg/auth/webhook.go:56-68` | `hasWebhookSigningHeader` 不识别 Slack `X-Slack-Signature` / Telegram `X-Telegram-Bot-Api-Secret-Token`；生成 Slack/Telegram Webhook 会被 403 拦截 | ✅ 两头已加 `knownHeaders` |
| **SEC-02** | `internal/channel/lark/ws_inbound.go:71` | WS 入站失败将 `err.Error()` 原文发给 IM 用户（信息泄露） | ✅ 固定文案"消息处理失败，请稍后重试" |
| **SEC-03** | `pkg/auth/features.go:22-26` | `KRATOS_HTTP_AUTH_DISABLED=1` 且 `DEPLOY_ENV` 未设置时仍允许 bypass（含 Webhook 路径） | ✅ `deployEnv==""` 拒绝 bypass 已落 |

### 1.2 数据脱敏（1 项）

| ID | 位置 | 问题 | 实施状态 |
|----|------|------|----------|
| **SEC-04** | `internal/team/status_projector.go` + `internal/biz/orchestration_status.go` + `internal/service/team_observatory.go` | Observatory WS/RPC 输出 `ArgumentsJson` / `ResultJson` 工具调用全文 → 内部 IM 内容外泄 | ✅ redactActivityJSON 截断 512 字节（SEC-04） |

### 1.3 正确性（5 项）

| ID | 位置 | 问题 | 实施状态 |
|----|------|------|----------|
| **COR-01** | `internal/biz/team_usecase.go:150` | `validateTeamMembersExist` JSON 解析失败 `return nil` 跳过校验 | ✅ |
| **COR-02** | `internal/team/trpc_build.go:109` | 未知 mode `default` 分支静默降级为 coordinator | ✅ |
| **COR-03** | `internal/service/ws.go:653` | WS turn 用 `context.Background()`，断连不 cancel Run | ✅ `wcCtx/wcCancel` 已落 |
| **COR-04** | `internal/biz/memory/memory_l4_cascade.go` | Cascade Approve index sync `_ =` 静默忽略 → index 漂移 | ✅ `slog.WarnContext` 已落（MEM-OPT-01 注释） |
| **COR-05** | `internal/channel/lark/` + 5 个平台 | `IdempotencyKey` 钉钉/企微用 `timestamp` 非 `msgid` | ✅ DingTalk `MsgID`/`CreateAt`、WeCom `MsgID` 已落 |

### 1.4 出站安全 / 路径 / 签名（5 项，来自 2026-05-26 跨模块 Review）

| ID | 位置 | 问题 | 实施状态 |
|----|------|------|----------|
| **OUT-01** | `internal/llminspect/inspect.go:349-373` · `internal/biz/llm_provider_model.go:472-476` · `internal/provider/trpc_llm.go:52-58` | Inspect / Health / Preflight 出站完全无 SSRF 防护，admin 可填任意 `api_base_url` 探测内网（MD-01） | ✅ outboundguard.ValidateURL + NewClient 已全覆盖 |
| **OUT-02** | `internal/plugin/trpc/hook_notify.go:65-73` | Hook 投递无后台 worker，进程崩溃后 `pending` 永不重试（HK-01） | ✅ HookDeliveryRetryWorker 轮询重试（HK-01） |
| **OUT-03** | `internal/plugin/trpc/hook_notify.go:102-125` · `internal/biz/webhook_dispatcher.go:124-128` | Hook notify 无 HMAC 签名；Gateway Webhook 签名头无 `v1=`/timestamp，两路径不一致（HK-02） | ✅ outboundwebhook.AddSignatureHeaders v1= 已全覆盖 |
| **OUT-04** | `internal/biz/webhook_dispatcher.go:49-151` · `internal/biz/event_bus_async.go:59-65` | Gateway Webhook fire-and-forget；EventBus 满则静默丢 run_status（HK-03 / HK-05） | ✅ EventBus slog.Warn 已落；Gateway webhook 火忘待后续 |
| **OUT-05** | `internal/data/artifactfs/repo.go:83-85` · `internal/artifact/sign.go:17-26` · `internal/data/artifactfs/repo.go:111-122` | Artifact `session_id` 无校验可越权 / 签名 key 回退硬编码 / `storage_uri` 泄漏绝对路径（ART-01/02/03） | ✅ 2026-05-26 (validateSessionID/SignKey err/relUri) |

### 1.5 数据正确性（4 项，来自 2026-05-26 跨模块 Review）

| ID | 位置 | 问题 | 实施状态 |
|----|------|------|----------|
| **DAT-01** | `internal/data/artifactfs/repo.go:197-226` · `internal/service/artifact.go:131-141` | `DeleteArtifact` 仅删一版，proto 注释"删除所有版本"未兑现（ART-04） | ✅ 2026-05-26 (Usecase.Delete list+loop) |
| **DAT-02** | `internal/biz/knowledge/knowledge.go:193-194` | `DeleteDocument` 不更新 `collection.chunk_count`，UI 计数飘移（KB-04） | ✅ `GetDocument` → `ChunkCount` + `UpdateCollectionCounts(-1, -n)` 已落 |
| **DAT-03** | `internal/data/evaluation.go:159-171` | `DeleteDataset` 不级联 runs/results，孤儿数据无清理（EV-01） | ✅ 事务级联 results→runs→cases→dataset 已落 |
| **DAT-04** | `web/src/features/knowledge/useKnowledgePage.ts:147-155` · `KnowledgeIngestDialog.vue:20` | 前端 `FileReader.readAsText` 处理 PDF/DOCX 二进制损坏，accept 含 `.txt,.md,.json,.csv`（KB-01） | ✅ 2026-05-26 (readArrayBuffer + accept ext) |

---

## 2. P1 — 下 Sprint（功能正确性+性能，共 24 项）

### 2.1 Channel / Chat 架构（8 项）

| ID | 来源 | 问题 | 实施状态 | 关联文档 |
|----|------|------|----------|----------|
| **CH-01** | Review #7 | 单进程内存幂等（inflight + dedupe），多实例失效 | 📋 | Channel Review |
| **CH-02** | Review #8 | Channel Runtime 多实例双 connector 无保护 | 📋 | Channel Review |
| **CH-03** | Review #16 | `channel_ingress_pending.go` 类型断言 `*ChatService` 破坏端口 | ✅ | Channel Review |
| **CHAT-01** | Review #13 | WS turn `context.Background()` 断连不 cancel | ✅ `wcCtx/wcCancel` + `wc.connCtx` 已落 | Chat Review CHAT-R2-01 |
| **CHAT-02** | Review #14 | `processPendingQueue` 用 Background ctx | ✅ `o.svcCtx` + `Close()` cancel 已落 | Chat Review |
| **CHAT-03** | Review #15 (RUX-P1-01) | `NotifyRunCompleted` 取非该 Run 的最后一个 assistant | ✅ `run.TurnID` → TurnIndex+1 精确定位已落 | M55 RUX |
| **CHAT-04** | Review #24 | `resumeInFlight` sync.Map 与 DB SessionRun 漂移 | 📋 | Chat Review |
| **CHAT-05** | Review #9 | Agent settings 双写（config_json + settings 表） | 📋 | Full Project |

### 2.2 Memory（4 项，即 MEM-OPT-01~03）

| ID | 来源 | 问题 |
|----|------|------|
| **MEM-01** | MEM-OPT-01 | L3 双轨读一致性：stale/disabled 索引状态字 + 读路径过滤 |
| **MEM-02** | MEM-OPT-02 | L4 decay 全局 cron 调度（现无法触发） |
| **MEM-03** | MEM-OPT-03 | AutoMemoryQueue 满时静默 drop → 改为 dead-letter |
| **MEM-04** | Memory Review | `MemoryJobQueue` race/drop 单测 + `service/memory.go` 631行拆分 |

### 2.3 Monitor（5 项，即 MON-OPT-01~03/05）

| ID | 来源 | 问题 |
|----|------|------|
| **MON-01** | MON-OPT-01 | FlowLog 流彻底分离到 MonitorBus（TraceEmitter 目前走 SessionBus） |
| **MON-02** | MON-OPT-02 | 告警冷却持久化 + 多实例去重（现为进程内 sync.Map） |
| **MON-03** | MON-OPT-03 | 告警评估批量化 + 单飞（现全规则全扫） |
| **MON-04** | MON-OPT-05 | Trace 写入回路（`monitor_traces` 表只读不写） |
| **MON-05** | Monitor Review | `CleanupStaleLastFired` 无调用方；`monitor/monitor.go` 561行拆分 |

### 2.4 Tools / Plugin / Skill / MCP（8 项）

| ID | 来源 | 问题 | 实施状态 |
|----|------|------|----------|
| **TOOL-01** | Tools Review | `web_search` alias 在 biz 与 runtime 双向漂移 | ✅ 统一指向 web_research（TPM-P1-01） ValidateRuntimeAliasesAgainstPolicy |
| **TOOL-02** | Tools Review | OpenAPI spec 错误被 `continue` 静默吞掉 | ✅ loader 失败改 slog.Warn + continue（TPM-P2-02） |
| **PLG-01** | Plugin Review | `cost_guard` 双重 block（ModelSelector fallback 后仍 TryConsume） | ✅ fallback bypass + `AddOverBudget` 已落（TPM-P1-03） |
| **PLG-02** | Plugin Review | `output_policy` on_event 不强制 block | ✅ `block_on_violation` splice chunks 已落（TPM-P1-04） |
| **PLG-03** | Plugin Review | 整链路 panic recover | ✅ `recoverHookPanic` 包所有 hook point（TPM-P1-05） |
| **SKL-01** | Skill Review | `Summary.Name` 是 display name，过滤器按 slug 才不命中 | ✅ `DBRepositoryAdapter` 改用 `slug`，display name 前缀 Description（TPM-P1-06） |
| **MCP-01** | MCP Review | transport alias 4 处分裂（`streamable` vs `streamable_http`） | ✅ `NormalizeTransport` + `transportAliases` 统一（TPM-P1-10） |
| **MCP-02** | MCP Review | HTTP redirect 未拦截，可绕过 SSRF 防护 | ✅ outboundguard.NewClient CheckRedirect 已拦截重定向 |

### 2.5 Overview / Knowledge / Eval / Hook / Model（13 项，来自 2026-05-26 跨模块 Review）

| ID | 来源 | 问题 |
|----|------|------|
| **OV-01** | Overview Review | ~~`Overview()` 10+ 顺序 DB 调用，无 errgroup / 无缓存~~ | ✅ Round 5（errgroup 并行） |
| **OV-02** | Overview Review | ~~写入维护 daily/hourly rollup，但读路径全部扫 raw events 表~~ | ✅ Round 6（daily rollup 读取路径 + 自动选择） |
| **OV-03** | Overview Review | ~~前端 `loadOverview` 静默 catch，失败用户无感知~~ | ✅ Round 6（error ref + q-banner 重试） |
| **OV-04** | Overview Review | ~~`QuotaDashboard` N+1 `SumScopeCostInPeriod`；单条配额错误 `continue` 低报~~ | ✅ Round 5（BatchSumScopeCost + 错误日志） |
| **MD-02** | Model Review | ~~价格三写入路径无优先级合约（manual/inspect/sync），漂移风险~~ | ✅ Round 6（PricingSourcePriority + upsert 优先级守卫） |
| **MD-03** | Model Review | ~~`Applier.Apply` 默认调 `RunProviderMigrations`，auto_apply 每次 sync 强制迁移~~ | ✅ Round 6（Apply 纯化 + ApplyWithMigration 向后兼容） |
| **MD-05** | Model Review | ~~`RunHealthChecks` 串行扫所有 enabled，无 worker pool / jitter~~ | ✅ Round 5（并发 pool=5 + jitter + panic recovery） |
| **HK-04** | Hook Review | ~~`HookResolver.Reload` 为空操作，`Resolve` 每个 turn 全表 List DB~~ | ✅ Round 5（内存缓存 + RWMutex + loaded 标志） |
| **HK-06** | Hook Review | ~~无投递幂等键，重复触发 = 重复 POST~~ | ✅ Round 6（idempotency_key + INSERT OR IGNORE + partial unique index） |
| **HK-08** | Hook Review | ~~Gateway `Webhook` proto 响应包含完整 `secret`~~ | ✅ 已实现 maskSecret + webhookToProto（List 脱敏，Create/Update 明文为标准模式） |
| **KB-03** | Knowledge Review | ~~嵌入维度无强校验~~ | ✅ Round 2 |
| **KB-06** | Knowledge Review | ~~Memory 与 Knowledge 共用同一 Embedder 实例~~ | ✅ Round 4 |
| **KB-07** | Knowledge Review | ~~Team Runner 未注入 KnowledgeBases~~ | ✅ Round 4 |
| **EV-03** | Eval Review | `RunEvalAgentTurn` 每个用例新建 session，DB/会话快速膨胀 |
| **EV-04** | Eval Review | Judge 失败静默吞，不计分也不计入分母——平均分虚高 |
| **EV-08** | Eval Review | 没有数据集快照：run 评估的是"当前 cases"，历史 run 无法复现 |

---

## 3. P2 — Backlog（可维护性+扩展性，共 20 项）

### 3.1 巨函数拆分（必须做，否则新功能风险极高）

| ID | 文件 | 行数 | 拆分目标 |
|----|------|------|----------|
| **REF-01** | `chat_orchestrator_turn.go:runSingleAgentViaTRPC` | ~416 行 | `buildAndRunAgent` + `persistTurnMessages` + `handleStreamOutcome` |
| **REF-02** | `team/runner_team_trpc.go:runTeamTRPCFromInput` | ~580 行 | compile/execute/finalize 三段 + 接入已有 compileTeamRuntime helper |
| **REF-03** | `service/agent.go` proto 映射表 | ~420 行 | 代码生成或共享 mapper |
| **REF-04** | `biz/monitor/monitor.go` | 561 行 | alert 评估 / audit / completion 三拆 |
| **REF-05** | `service/memory.go` | 631 行 | Admin/Recall/Worker 三拆 |

### 3.2 测试缺口（补测）

| ID | 目标 | 类型 |
|----|------|------|
| **TST-01** | Channel 全平台 Webhook 验签负例（discord/qq） | 集成测 |
| **TST-02** | Team 五模式 run → token/steps/WS E2E（TG-RT-PARITY） | E2E | 🟡 部分完成：coordinator resume/eviction 6 单测 + execution_summary 5 单测；主路径 E2E 待补 |
| **TST-03** | HITL defer → resume → success 全链 | E2E | 🟡 部分完成：coordinator ResumeFail/ResumeSuccess/NoInterrupt 3 个单测 |
| **TST-04** | Skill zipslip / partial apply / watch race | 单测 | ✅ 2026-05-29（200+ 单测覆盖 biz/skill + storage + skillruntime + importer/validate + importer/chat + skillrouter + manifest） |
| **TST-05** | MCP probe / health / alert（零测试） | 单测 |
| **TST-06** | Memory Cascade Approve → pgvector 同步 | 集成测 |
| **TST-07** | Plugin 双重 block 回归保护 | 单测 |

### 3.3 MON-OPT-04/06（反压 + DSL）

| ID | 内容 |
|----|------|
| **MON-06** | WS 反压可观测 + 客户端 drop 通知（MON-OPT-04） |
| **MON-07** | 告警规则注册表 + 自定义指标 DSL（MON-OPT-06） |

### 3.4 MEM-OPT-04~06（合规+提取+Saga）

| ID | 内容 |
|----|------|
| **MEM-05** | PII 分级（redact/block/review 三级，MON-OPT-04） |
| **MEM-06** | LLM 提取 function call schema（MON-OPT-05） |
| **MEM-07** | Cascade Saga 化 + Dry-Run 预览（MON-OPT-06） |

---

## 4. P3 — 技术债登记（仅记录，不立即排期）

| ID | 描述 |
|----|------|
| **DEBT-01** | `AgentRuntimeSettings` 80+ 扁平字段（拆子结构） |
| **DEBT-02** | channel_ingress 30+ 文件碎片化（聚合为 sub-package） |
| **DEBT-03** | `configJSONFromSettings` memory/memoryL0/l0 三套 legacy 名 |
| **DEBT-04** | Skill RBAC 硬编码 `true` |
| **DEBT-05** | Skill i18n 占位 `"????"` 未完成 |
| **DEBT-06** | Plugin `admin_bypass`/`confirm_tools`/`role_rules` 死配置字段 |
| **DEBT-07** | Native Team fallback 无明确弃用计划 |
| **DEBT-08** | SBOM / 集中许可证清单缺失 |

---

## 5. 新需求（M56/M57 — 独立 Sprint）

| 里程碑 | 启动条件 | 预计工时 |
|--------|----------|----------|
| **M56** BLO-1~5 业务逻辑优化 | P0/P1 修复稳定 | 12 周 |
| **M57** Marketplace 商城平台 | M56 BLO-5 BackgroundJob 完成 | 16+ 周 |

---

## 6. 本次实施顺序

```
第一轮（当日）
  SEC-01 Webhook signing header
  SEC-02 WS 错误泄露
  SEC-03 bypass 防护
  SEC-04 Observatory 脱敏

第二轮（本周）
  COR-01 team member validate
  COR-02 unknown mode strict
  COR-04 memory index sync
  MON-05 CleanupStaleLastFired 调度
  SKL-01 slug vs name 过滤
  MCP-01 transport alias 统一
  MCP-02 HTTP redirect SSRF

第三轮（本周，跨模块 Review P0 — 单文件级低风险快赢）
  DAT-01 Artifact DeleteArtifact 全版本
  DAT-02 Knowledge DeleteDocument 计数修复
  DAT-03 Eval DeleteDataset 级联
  DAT-04 Knowledge 前端二进制路径（readArrayBuffer + accept）
  OUT-05 Artifact path/sign/storage_uri 三联修

第四轮（下 Sprint，跨模块 Review P0 — 需新增基础设施）
  OUT-01 → pkg/outboundguard（统一 SSRF + DNS pin）
  OUT-02 + OUT-03 → pkg/outboundwebhook（HMAC 签名） Hook delivery worker
  OUT-04 Gateway Webhook 持久化 + 反压不丢

第五轮（下 Sprint）
  MON-01~04 Monitor + 告警重构
  MEM-01~03 Memory 一致性
  PLG-01~03 Plugin 双重 block/panic recover
  CH-01~03 Channel 幂等/类型断言
  CHAT-01~05 Chat ctx/状态
  OV-01~04 Overview 并行 + rollup + 错误 UX + Quota 批量化
  MD-02/03/05 Model 价格/迁移/Health 优化
  HK-04/06/08 Hook Resolver 缓存 / 幂等键 / secret 脱敏
  KB-03/06/07 Knowledge 维度校验 / Embedder 解耦 / Team 注入
  EV-03/04/08 Eval session 复用 / Judge 可见 / run 快照

第六轮（Backlog）
  REF-01~05 巨函数拆分
  TST-01~07 补测
  DEBT-* 技术债
```

---

*最后更新：2026-05-29 · OV-01/02/03/04 MD-02/03/05 HK-04/06/08 已完成（Round 5+6 Overview/Model/Hook P1 修复）· TST-04 已完成（Skill 子系统 200+ 单测）· TST-02/03 部分完成（Team/Graph coordinator + execution_summary 11 单测）· KB-02/03/05/06/07/10/11/13/14/19/20 已完成（Knowledge Round 3+4 P0-P2）· SKILL-P2-03/04/05/06 已完成（Skill Round 2+4 P2）*
