# 流程日志（Flow Log）覆盖审计与补齐清单

> **类型**：audit | **日期**：2026-07-29
> **对应模块**：[52-flow-logger](../development/52-flow-logger.design.md)
> **结论**：流程日志机制（TraceEmitter → MonitorBus → WS `flow_log` + `flow_log_events` 表）运转正常，但**真实发射点仅覆盖 chat / team / channel 三个域**，注册表 93 条中 67 条「注册了标题但无任何发射点」，另有 16 条「有发射但未注册中文标题」。监控 - Logs 页「流程日志」因此只在对话/团队/渠道场景有输出。

---

## 1. 机制回顾

```
业务代码 TraceEmitter.Log*(stepID, msg)
  → FlowTracker.emit → FlowLogEntry (flow_log/v1)
  → MonitorEventBus (flow_log MonitorEvent)
      ├─ WS /v1/ws → 监控 Logs 页「流程日志」Tab + Traces 详情「流程」Tab
      └─ flowLogPersistConsumer → flow_log_events 表 → GET /v1/monitor/flow-logs
```

- 发射 API：`internal/event/flow_tracker.go`（LogStart/LogDone/LogError/LogSkip/LogWarn/LogCritical/Log）
- 创建方式：`NewTraceEmitterForRun(TraceEmitterOpts{Ctx, SessionID, RunID, Domain, LG, Infra})`；非 Turn 场景可 `NewInfraFromBus(monitorBus)` 包装共享总线（参考 [channel_ingress_flow.go](../../internal/service/channel_ingress_flow.go)）
- 步骤命名：`{domain}.{subsystem}.{action}`；中文标题注册表 `internal/event/flow_log.go` `stepTitleRegistry`（未注册时前端显示裸 stepID）

## 2. 现状量化

### 2.1 真实发射点（生产代码，2026-07-29 核实）

| 域 | stepID 数 | 发射位置 | 覆盖评价 |
|----|----------|----------|----------|
| chat（对话 Turn） | 31 | `chat_orchestrator_turn.go` / `_phases.go` / `_pipeline.go` / `_dispatch.go` / `chat_turn_metrics.go` / `chat_orchestrator_durable.go` / `team_turn_hooks.go` | ✅ 良好 |
| run（Session Run 生命周期） | 1（`run.start`） | `chat_orch_session_run_lifecycle.go:143` | 🟡 仅起点 |
| team（团队运行） | 4 + `team.member.*` 动态 | `runner_team_trpc*.go`、`runner_team_turn.go`、`runner_team_compiler.go`、`team_graph_run_finisher.go` | ✅ 良好 |
| channel（渠道 Turn） | 5（execute/done/timeout/cancel/background） | `channel_ingress_*.go` | 🟡 仅 Turn 级 |
| **其余全部域** | **0** | — | ❌ 无 |

合计约 **41 个 stepID 有真实发射点**。

### 2.2 注册表假覆盖（67 条注册无发射）

`stepTitleRegistry` 共 93 条，仅 26 条有真实发射。以下 67 条**注册了中文标题但全工程无任何发射点**（Logs 页永远不会出现）：

| 分组 | stepID |
|------|--------|
| event_bus（4） | `event_bus.usage.record`、`event_bus.monitor.persist`、`event_bus.state.persist`、`event_bus.state.apply` |
| system.bus/ws（6） | `system.bus.drop`、`system.ws.upgrade_failed`、`system.ws.read_error`、`system.ws.send_drop`、`system.ws.parse_error`、`system.ws.send_failed` |
| system.cron（4） | `system.cron.job_dead`、`system.cron.retry`、`system.cron.panic`、`system.cron.dispatch_skipped` |
| system.telemetry/auth（6） | `system.telemetry.init/noop/error`、`system.auth.bypass_warn/bypass_refused/bypass_active` |
| system.plugin/hook/mcp（5） | `system.plugin.seed_fail`、`system.plugin.reload_fail`、`system.hook.reload_fail`、`system.mcp.health_list_fail`、`system.mcp.health_persist_fail` |
| system.agent（6） | `system.agent.cache_hit/cache_miss/db_resolve/skill_build/tool_build/memory_disabled` |
| system.provider（6） | `system.provider.catalog_fail/preflight_fail/config_resolved/preflight_ok/ha_failover/ha_hedge` |
| system.tool/intent/memory_worker/auto_memory（7） | `system.tool.record_fail`、`system.intent.pass_done`、`system.memory_worker.enqueue`、`system.auto_memory.extract_fail/extract_max_retry/l4_fail/done` |
| system.monitor/session/graph（9） | `system.monitor.alert_webhook_fail/alert_channel_fail`、`system.session.compress/title_fail/rollback_fail`、`system.graph.task_start_fail/task_status_fail/task_resume_fail/runtime_run_fail` |
| system.graph/task/channel/knowledge 等（10） | `system.graph.runtime_resume_fail`、`system.task.timeout_update_fail/release_claim_fail/dispatcher_tick_fail/check_timeout_fail/claim_fail/dispatch_run_fail`、`system.channel.dead_letter`、`system.knowledge.embed_fail`、`system.safego.panic`、`system.grpc.unauthenticated`、`system.data.builtin_tool_sync`、`system.builtin_tools_sync_fail` |
| team 辅助（5） | `team.intent.merge_fail`、`team.intent_anchor_fallback`、`team.usage_record_fail`、`team.turn.usage`、`chat.intent.merge_fail`、`chat.usage_record_fail` |
| knowledge（1） | `knowledge.rerank.fallback` |

> 注：其中约 15 条（`knowledge.rerank.fallback`、`system.provider.ha_*`、`system.memory_worker.enqueue`、`event_bus.*`、`gateway.webhook.*` 等）以 `loggateway.StepID(...)` 字段形式出现在**进程日志**中（如 [retriever.go:123](../../internal/knowledge/retriever.go#L123)、[trpc_llm.go:591](../../internal/provider/trpc_llm.go#L591)），但进程日志不经过 `stepTitleRegistry`，不会出现在「流程日志」Tab。

### 2.3 发射无注册（16 条缺中文标题）

以下 stepID 有真实发射但未注册标题，前端显示裸 ID：

```
chat.receive · chat.active_check · chat.session_fetch · chat.session_ownership
chat.agent_hydrate · chat.provider_resolve · chat.attachment.preflight
chat.pre_planning_gate · chat.clarification_gate · chat.proactive_recall
chat.runner.ralph_loop · chat.runner.rollback · chat.runner.rollback_boundary
chat.turn.timeout_with_reply · run.start · channel.turn.background
```

## 3. 全模块业务流程清单 × 覆盖矩阵

> 模块划分依 [0-system-diagram.md §四](../development/0-system-diagram.md)。状态：✅ 已覆盖 / 🟡 部分 / ❌ 无流程日志。

| # | 模块 | 业务流程 | 状态 | 缺口 |
|---|------|----------|------|------|
| 1 | Chat 对话 | 发送 Turn（同步/流式）、排队消息、取消/Steer、Durable 续跑、澄清门/规划门 | ✅ | — |
| 2 | Runner 运行控制 | Run 注册/取消/状态查询/Steer 注入 | 🟡 | 仅 `run.start`；取消、Steer、终态无 |
| 3 | Session 会话 | 创建/重命名/删除、标题生成、上下文压缩、回滚 | ❌ | 全部（注册表有 `system.session.*` 无发射） |
| 4 | Provider 模型 | 目录同步（modelregistry）、预检、HA 切换、连通性 Inspect | ❌ | 全部 |
| 5 | Agent 管理 | 创建/更新/删除、运行时设置、进化、头像/标题生成 | ❌ | 全部 |
| 6 | Team 编排 | 定义解析→编译 GraphAgent→成员执行→收尾 | ✅ | — |
| 7 | Graph 工作流 | 创建/校验、运行、恢复（Resume）、HITL 确认、Checkpoint、TimeTravel | ❌ | 全部（注册表有 `system.graph.*` 无发射） |
| 8 | Tools 工具 | 目录同步、装配（Assemble）、内置工具种子同步 | ❌ | 全部 |
| 9 | MCP | 服务器添加/删除、探活、定时健康检查、Broker 工具发现 | ❌ | 全部 |
| 10 | Skill | 包导入（验证→冲突决策→落库）、启用/禁用、发布、运行时执行、Watch 热重载、CodeExecutor | ❌ | 全部 |
| 11 | Plugin/Callback | 插件种子同步、运行时重载、Hook 重试 | ❌ | 全部 |
| 12 | Memory 记忆 | 自动提取（Worker）、L1-L4 衰减维护、睡眠重组 | ❌ | 全部（进程日志有） |
| 13 | Knowledge 知识 | 文档上传→解析→分块→嵌入→索引、检索/重排、删除、Vault 扫描同步 | ❌ | 全部 |
| 14 | Artifact 产物 | 产物存储、版本管理、Runner 注入 | ❌ | 全部 |
| 15 | Cron 定时 | 触发→分发→执行→重试→死信 | ❌ | 全部（注册表有 `system.cron.*` 无发射） |
| 16 | Channel 渠道 | 入站→访问控制→路由→Peer 会话→Turn→出站投递、连接生命周期（WS/Webhook 接入/断开） | 🟡 | Turn 级已覆盖；连接生命周期、出站投递失败无 |
| 17 | Event 事件总线 | flow_log 落库、usage 汇总、state 持久化、队列满降级 | ❌ | 仅进程日志 |
| 18 | Monitor 监控 | 自检调度、告警评估→通知、诊断包、自愈 | ❌ | 全部 |
| 19 | A2A 联邦 | 联邦调用治理链（信任→策略→配额→调用→审计）、AgentCard 同步、远端健康检查 | ❌ | 全部 |
| 20 | Evaluation 评测 | EvalSet 运行、LLM Judge、结果落库 | ❌ | 全部 |
| 21 | Ecosystem 生态 | 模板/扩展安装 | ❌ | 全部 |
| 22 | MediaProvider 媒体 | 文生图/文生视频（Qwen/ComfyUI）、产物落库 | ❌ | 全部 |
| 23 | 平台启动/迁移 | NewData→L1/L2/L3 迁移→种子→ReadinessGate→服务就绪 | ❌ | 全部 |
| 24 | 系统设置/认证 | 设置更新+热更（A2A base URL/嵌入器）、登录、认证绕过 | ❌ | 全部 |
| 25 | Webhook 出站 | 出站 Webhook 投递/重试（gateway） | ❌ | 仅进程日志 |

**统计**：25 个模块、约 60 个业务流程；流程日志已覆盖 3 个模块（12%），部分覆盖 2 个，**20 个模块（80%）完全无流程日志**。

## 4. 补齐清单（新增 stepID 规划）

> 原则：① stepID 沿用 `{domain}.{subsystem}.{action}`；② 已注册标题的 67 条优先**补发射点**而非另起新 ID；③ 每条注明发射位置与典型 phase；④ 标题统一补入 `stepTitleRegistry`。
> 优先级：**P0**＝核心链路/出错难排查；**P1**＝重要辅助流程；**P2**＝低频管理操作。

### 4.0 先行修正：补注册 16 条已发射无标题 stepID（P0，纯注册表修改）

`chat.receive`=收到消息、`chat.active_check`=检查活跃运行、`chat.session_fetch`=加载会话、`chat.session_ownership`=会话归属校验、`chat.agent_hydrate`=加载 Agent 配置、`chat.provider_resolve`=解析模型配置、`chat.attachment.preflight`=附件预检、`chat.pre_planning_gate`=规划门决策、`chat.clarification_gate`=澄清门、`chat.proactive_recall`=主动召回、`chat.runner.ralph_loop`=Ralph Loop 配置、`chat.runner.rollback`=Runner 会话回滚、`chat.runner.rollback_boundary`=Runner 回滚边界、`chat.turn.timeout_with_reply`=超时但已保存回复、`run.start`=创建会话运行、`channel.turn.background`=渠道后台继续执行。

### 4.1 P0（核心链路，9 组 34 条）

| stepID | 标题 | 发射位置（函数） | phase |
|--------|------|------------------|-------|
| `cron.job.trigger` | 定时任务触发 | `cronrunner/runner.go` tick 命中 | start |
| `cron.job.dispatch` | 定时任务分发 | `cronrunner/execute.go` 派发 | start/done |
| `cron.job.execute` | 定时任务执行 | `execute.go` Run 完成 | done/error |
| `system.cron.retry` ★ | 定时任务重试 | `execute.go` 重试分支 | warn |
| `system.cron.job_dead` ★ | 定时任务进入死信 | `execute.go` 死信落库 | error |
| `system.cron.panic` ★ | 定时任务 panic | `execute.go` recover | error |
| `graph.run.start` | 图运行开始 | `graph/adapter/runtime_adapter.go` Run | start |
| `graph.run.finish` | 图运行结束 | 同上 终态投影 | done/error |
| `graph.run.resume` | 图运行恢复 | 同上 Resume | start/done/error |
| `graph.node.execute` | 图节点执行 | `graph/trpc/event_bridge.go` 节点事件 | done/error（动态后缀 `graph.node.<nodeID>`） |
| `graph.checkpoint.save` | 检查点保存 | `graph/trpc/checkpoint.go` | done/error |
| `graph.hitl.wait` | 等待人工确认 | HITL interrupt | start |
| `system.graph.task_start_fail` ★ | 图任务启动失败 | `service/graph.go` StartRun | error |
| `skill.import.start` | Skill 包导入开始 | `skill/importer/engine.go` Import | start |
| `skill.import.validate` | Skill 包校验 | `engine.go` 验证阶段 | done/error |
| `skill.import.conflict` | Skill 冲突决策 | `engine.go` 冲突组决策 | done（extra 含 keep/skip 数） |
| `skill.import.done` | Skill 导入完成 | `engine.go` 提交落库 | done/error |
| `skill.watch.reload` | Skill 热重载 | `skill/watch/reconcile.go` | done/error |
| `skill.execute` | Skill 运行时执行 | `tools/skillruntime` 执行入口 | start/done/error |
| `knowledge.ingest.start` | 知识文档摄取开始 | `service/knowledge.go` 上传入口 | start |
| `knowledge.ingest.parse` | 文档解析分块 | `internal/knowledge` 解析/chunk | done/error |
| `knowledge.ingest.embed` | 文档向量嵌入 | `embedder.go` 批嵌入 | done/error |
| `system.knowledge.embed_fail` ★ | 知识嵌入失败 | `embedder.go` 失败降级 | error |
| `knowledge.ingest.done` | 知识摄取完成 | 摄取收尾（含 chunk 数） | done |
| `knowledge.vault.sync` | Vault 同步 | `vault_sync_runner.go` RunVault | start/done/error |
| `knowledge.search` ★ | 知识库检索 | `retriever.go` Search | done/error |
| `a2a.invoke.start` | A2A 联邦调用开始 | `a2a/federation_invoke.go` 治理链入口 | start |
| `a2a.invoke.governance` | A2A 治理链检查 | Trust/Policy/Quota 各闸门 | done/skip/error |
| `a2a.invoke.remote` | A2A 远端调用 | `remote_invoke.go` HTTP 调用 | done/error |
| `a2a.invoke.done` | A2A 调用完成 | 审计落库后 | done/error |
| `system.startup.migration` | 数据库迁移 | `data/data.go` L1/L2/L3 各阶段 | start/done/error |
| `system.startup.seed` | 基础数据种子 | `data.go` seedP1Data | done/error |
| `system.startup.ready` | 服务就绪 | ReadinessGate 开放 | done |
| `system.startup.shutdown` | 服务关闭 | `cmd/admin/app.go` AfterStop | start/done |

★＝注册表已存在标题，直接补发射点。

### 4.2 P1（重要辅助，10 组 30 条）

| stepID | 标题 | 发射位置 | phase |
|--------|------|----------|-------|
| `session.create` / `session.delete` / `session.rename` | 会话创建/删除/重命名 | `biz/session/usecase.go` 对应方法 | done/error |
| `system.session.compress` ★ | 会话上下文压缩 | `session/compressor.go` Compress | start/done/error |
| `system.session.title_fail` ★ | 会话标题生成失败 | 标题生成 worker | error |
| `agent.crud.create` / `agent.crud.update` / `agent.crud.delete` | Agent 创建/更新/删除 | `service/agent.go` | done/error |
| `system.agent.cache_hit` ★ / `system.agent.cache_miss` ★ | Agent 缓存命中/未命中 | `agent/cache.go` | done |
| `provider.catalog.sync` | 模型目录同步 | `modelregistry/sync.go` | start/done/error |
| `system.provider.preflight_ok` ★ / `system.provider.preflight_fail` ★ | 模型预检通过/失败 | `provider/trpc_llm.go` 预检 | done/error |
| `system.provider.ha_failover` ★ / `system.provider.ha_hedge` ★ | HA 故障/对冲切换 | `haSwitchCallback`（需注入 Infra） | warn |
| `mcp.server.add` / `mcp.server.remove` | MCP 服务器添加/移除 | `service/mcp*.go` | done/error |
| `system.mcp.health_list_fail` ★ / `system.mcp.health_persist_fail` ★ | MCP 健康检查失败/保存失败 | `mcp/health/runner.go` | error |
| `memory.auto.extract` | 自动记忆提取 | `cronrunner/jobs/auto_memory.go` | start/done/error（★ `system.auto_memory.*` 复用） |
| `media.generate` | 媒体生成 | `provider/media` 生成入口 | start/done/error |
| `evaluation.run` | 评测集运行 | `evaluation/runner.go` | start/done/error |
| `channel.connect.open` / `channel.connect.close` / `channel.connect.error` | 渠道连接建立/断开/异常 | `channel/runtime/supervisor.go` + 各平台 ws/webhook | start/done/error |
| `system.channel.dead_letter` ★ | 渠道投递死信 | channel 投递死信分支 | error |
| `gateway.webhook.delivery` | 出站 Webhook 投递 | `biz/webhook_dispatcher.go`（失败已注册 `gateway.webhook.delivery_fail`） | done/error |
| `monitor.alert.evaluate` | 告警评估 | `biz/monitor/monitor.go` EvaluateAlerts | done（异常时 error） |
| `system.monitor.alert_webhook_fail` ★ / `system.monitor.alert_channel_fail` ★ | 告警通知失败 | 告警通知发送 | error |
| `monitor.selfcheck.run` | 系统自检 | `biz/monitor/self_check_scheduler.go` | start/done/error |
| `event_bus.flow_log.persist` | 流程日志落库 | `biz/event_bus_flow_log_consumer.go`（★ 失败侧已注册 `flow_log.persist` 进程日志） | error |
| `event_bus.usage.record` ★ / `event_bus.state.persist` ★ / `event_bus.state.apply` ★ / `event_bus.monitor.persist` ★ | 用量写入/状态保存/应用/监控持久化失败 | `biz/event_bus_*.go` 对应消费者 | error |

### 4.3 P2（低频管理，4 组 10 条）

| stepID | 标题 | 发射位置 | phase |
|--------|------|----------|-------|
| `settings.update` | 系统设置更新 | `service/system_setting.go` UpdateSystemSettings | done/error |
| `settings.hot_reload` | 配置热更新 | 同上 Reload 分支（A2A base URL/嵌入器） | done/error |
| `system.auth.bypass_warn` ★ / `bypass_refused` ★ / `bypass_active` ★ | 认证绕过警告/拒绝/启用 | `pkg/auth` / server 中间件 | warn |
| `system.telemetry.init` ★ / `noop` ★ / `error` ★ | 遥测初始化/未配置/失败 | `telemetry/telemetry.go` | done/error |
| `system.plugin.seed_fail` ★ / `reload_fail` ★、`system.hook.reload_fail` ★ | 插件种子/重载/Hook 重载失败 | `plugin/trpc/manager.go`、`hook_retry_worker.go` | error |
| `system.tool.record_fail` ★ | 工具调用记录失败 | `agent/tool_invocation_recorder.go` | error |
| `system.builtin_tools_sync_fail` ★ / `system.data.builtin_tool_sync` ★ | 内置工具同步 | `data/builtin_tools_seed.go` | done/error |
| `system.task.*` ★（6 条） | 任务调度/声明/超时 | task dispatcher（定位后补发射） | error |
| `system.ws.*` ★（5 条）、`system.bus.drop` ★ | WS 读写/解析/丢弃 | `server/ws*.go`、`event/generic_bus.go` | warn/error |
| `system.safego.panic` ★、`system.grpc.unauthenticated` ★ | 协程 panic/gRPC 未认证 | `pkg/safego`、`server/grpc.go` | error/warn |
| `ecosystem.pack.install` | 生态包安装 | pack 安装入口 | start/done/error |

### 4.4 数量统计

| 优先级 | 新增/补发射 stepID 数 | 涉及模块 |
|--------|----------------------|----------|
| 4.0 补注册 | 16（仅注册表） | chat/run/channel |
| P0 | 34 | cron、graph、skill、knowledge、a2a、startup |
| P1 | 30 | session、agent、provider、mcp、memory、media、evaluation、channel、webhook、monitor、event_bus |
| P2 | 10+（含 system.task.*/ws.* 组） | settings、auth、telemetry、plugin、tools、task、ws、ecosystem |
| **合计** | **约 90 条** | 全覆盖后 25/25 模块有流程日志 |

## 5. 实施约束

1. **非 Turn 场景发射方式**：无 session 的系统级步骤用 `NewTraceEmitterForRun(TraceEmitterOpts{Ctx, Domain: event.TraceDomainSystem, LG, Infra: event.NewInfraFromBus(monitorBus)})`；有 session 的填 `SessionID` 以便按会话过滤。
2. **Infra 依赖**：发射点需要 `contract.MonitorBus`。已有 lg 注入的 struct 增补 bus 依赖时走构造函数注入 + Wire；禁止 `loggateway.Global()` 式全局获取（参考红线）。`NewInfraFromBus` 已存在，专为存量调用点设计。
3. **发射即注册**：新增 stepID 必须同 PR 补 `stepTitleRegistry` 中文标题 + 更新 [52-flow-logger.design.md §5.1](../development/52-flow-logger.design.md)。
4. **脱敏**：extra 禁止 prompt 全文/凭据（遵循设计 §8）。
5. **验证**：`go build ./... && go test ./internal/event/... ./internal/service/... -count=1`；运行时触发一条流程在 Logs 页「流程日志」Tab 确认可见。
