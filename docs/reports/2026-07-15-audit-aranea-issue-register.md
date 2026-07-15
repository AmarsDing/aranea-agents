# Aranea-Agents 审计问题登记册

> 日期：2026-07-15  
> 严重度：Blocker=发布阻断；Critical=高概率安全/数据/核心流程故障；Major=主要功能或生产工程缺陷；Minor=治理与维护问题。  
> 置信度：V=代码或命令已验证；H=多证据强推断；R=需真实外部环境复验。

## 1. Blocker

| ID | 问题与证据 | 影响 | 建议与收益 | 置信度 |
|---|---|---|---|---|
| B-01 | Workspace 来自客户端 Header/Query，未绑定 JWT membership：`internal/server/middleware/workspace.go:13-45`、`pkg/auth/auth.go:9-18` | 已登录用户可伪造 workspace，租户边界不可信 | JWT/服务端会话携带 membership；Repo 强制 tenant scope；Postgres RLS。收益：租户隔离不可绕过 | V |
| B-02 | WS 只认证，不校验 session 所有权；`session_id=*` 缺少管理员授权：`internal/server/ws.go:175-210`、`internal/server/ws_message_handler.go:109-146` | 跨用户监听、发送、取消和状态探测 | 引入统一 `SessionAuthorizer`，WS/HTTP/Channel 共用；全局订阅仅 ops/admin。收益：关闭实时链路 IDOR | V |
| B-03 | 隔离执行器不可用时可回退 Local：`internal/agent/codeexecutor/factory.go:149-185` | 配置或供应商故障可升级为宿主机执行不可信代码 | 生产环境全部 fail-closed；Local 仅显式 break-glass。收益：避免宿主机 RCE | V |
| B-04 | Spirit `plan_and_execute` 在真正 DAG 执行前发布完成：`internal/tools/spirit_tools.go:183-197,285-340` | 用户与上游收到假成功，进度/取消/恢复句柄无效 | 返回 accepted/running；只有 durable executor 终态才能发布 completed。收益：执行状态可信 | V |
| B-05 | PlanBoard `planning→executing` 修改副本，终态仍从 planning 转换：`internal/service/plan_executor.go:182-225,405-449` | 正常执行后状态无法正确完成 | 返回更新后的 board/使用指针，终态以数据库 CAS 当前版本转换。收益：状态收敛 | V |
| B-06 | v2 关键事件在 EventBus 和 WS 两层均可丢弃且无重放：`internal/event/bus_v2.go:68-107`、`internal/server/ws_priority.go:120-126` | 任务结束或错误事件丢失，UI 永久 running | Critical 事件 outbox/WBPF；session sequence + replay/snapshot。收益：可恢复实时投影 | V |
| B-07 | 当前发布门禁实测失败：前端 typecheck、unit、layer、strict eslint、prettier、stylelint；Go race 检出竞态 | 仓库绿色状态与实际不可合并状态不一致 | 先恢复 required checks，再允许功能 PR。收益：阻止已知回归进入主干 | V |

## 2. Critical

### 2.1 安全与多租户

| ID | 问题与证据 | 影响 | 建议与收益 | 置信度 |
|---|---|---|---|---|
| C-01 | Knowledge 创建/列表/点查未强制 workspace：`internal/service/knowledge.go:103-133`、`internal/data/knowledge.go:131-168` | 跨租户读、删、检索知识 | 所有实体冗余 workspace_id，点查加 tenant predicate。收益：RAG ACL-aware | V |
| C-02 | Artifact Service/Repo 缺对象级授权，Repo 忽略 context：`internal/service/artifact.go:36-109`、`internal/data/artifactfs/repo.go:113-176` | 知道 ID/session 即可能下载或删除他人产物 | 元数据加入 owner/workspace；签名 URL 绑定主体与用途。收益：关闭 Artifact IDOR | H |
| C-03 | 工具缓存键只有工具名和参数：`internal/tools/decorator.go:194-209,315-321` | 跨会话/用户命中敏感结果 | 加 workspace/user/session/config/credential 指纹；敏感工具禁用跨 invocation cache。收益：防止缓存串租户 | H |
| C-04 | 默认工具目录只按 agentKey；任意现存绝对路径可接受：`internal/agent/tool_assembly.go:272-321` | 不同用户共享文件，可能暴露宿主目录 | 固定 tenant/user/session 根；`EvalSymlinks` 后 containment 校验。收益：租户级文件沙箱 | V |
| C-05 | MCP 静态 API key/token 存于非 Sensitive 的 `config_json`：`internal/mcp/config/config.go:63-94`、`internal/data/ent/schema/platform_mcp_server.go:22-35` | DB、日志或生成 String 泄露长期凭据 | SecretRef/KMS；Schema `.Sensitive()`；递归脱敏。收益：支持安全轮换 | V |
| C-06 | Plugin runtime 全局重载使用 `context.Background()` 且无 workspace 分区：`internal/service/plugin.go:75-90`、`internal/plugin/trpc/runtime.go:143-199` | 插件配置跨租户覆盖或读取错误 workspace | 按 workspace 保存不可变版本快照；后台任务显式 system/tenant context。收益：热重载隔离 | V |
| C-07 | A2A remote URL 未做最终 IP/重定向 SSRF 校验：`internal/a2a/remote_client.go:111-137` | 探测内网/metadata 服务 | CheckRedirect + DialContext IP 校验 + allowlist。收益：关闭 A2A egress SSRF | V |

### 2.2 核心运行、并发与一致性

| ID | 问题与证据 | 影响 | 建议与收益 | 置信度 |
|---|---|---|---|---|
| C-08 | SessionLock sweep 可删除仍被持有的锁：`internal/biz/session_lock.go:61-106` | 同 session 出现两把锁，破坏串行执行 | entry 引用计数/持有状态；仅释放后 TTL 删除。收益：长任务互斥可靠 | V |
| C-09 | Cancel 立即删除 active run：`internal/runtime/run_registry.go:171-219` | 旧 runner 未退出时新 run 进入 | `cancelling` 状态保留 lease，执行 goroutine `Finish(runID)` CAS 清理。收益：消除重叠 runner | V |
| C-10 | cancelled 可被失败 defer 覆盖：`internal/service/chat_orchestrator_turn.go:436-451`、`internal/agent/v2/projector.go:648-662` | DB、Session、WS 三套终态分裂 | 使用 cancellation cause；用户取消只产生 cancelled。收益：终态一致 | V |
| C-11 | Run 状态持久化错误被吞：`internal/service/chat_orch_run_status.go:97-145` | WS 已完成但重启恢复为旧状态 | 持久化返回 error；终态先持久化再发布/outbox。收益：恢复可信 | V |
| C-12 | PendingQueue 周期快照，ACK 与 snapshot 间崩溃可丢失/复活消息：`internal/runtime/pending_queue.go:129-169,251-328` | 重复或丢失用户意图 | 数据库 durable queue 或 WAL；唯一 idempotency key。收益：可恢复 at-least-once | V |
| C-13 | `IdempotencyKey` 无唯一约束和查重：`internal/service/turn_service_persistent.go:29-68`、`internal/data/ent/schema/session_turn.go:21-63` | 网络重试重复调用 LLM、重复副作用 | `(workspace,session,source,key)` unique + insert-on-conflict 返回 canonical turn。收益：节省成本、防重复执行 | V |
| C-14 | `turn_number=MAX+1` 且索引不唯一：`internal/data/session_turn_repo.go:51-118` | 多实例下重复 turn number | 锁 session 主行/CAS counter；唯一约束兜底。收益：顺序稳定 | V |
| C-15 | v2 Sequencer race 实测失败：`internal/agent/v2/sequencer_test.go:100,356-380`、`sequencer.go:278-319` | 并发测试不可信，潜在真实共享状态竞态 | fake bus 与等待条件加同步；再确认生产对象是否同型。收益：race 门禁可恢复 | V |

### 2.3 编排、Graph 与长任务

| ID | 问题与证据 | 影响 | 建议与收益 | 置信度 |
|---|---|---|---|---|
| C-16 | 无根/循环 DAG 零执行即成功：`internal/service/plan_executor.go:351-389` | LLM 生成非法图却报告完成 | 运行前校验 ID、依赖、根、环、可达性。收益：fail-fast | V |
| C-17 | 取消时 executor 未等待 worker 退出即遍历共享 board：`internal/service/plan_executor.go:373-416,740-758` | 数据竞争、终态后继续写 step | cancel→worker ack→锁内快照→terminal。收益：happens-before 明确 | V |
| C-18 | TaskPlan、PlanBoard、OrchestrationHandle 三套 execution ID | 取消、恢复、事件与审计无法稳定关联 | 引入 canonical `OrchestrationExecutionID`，其他实体为版本化投影。收益：单一事实源 | V |
| C-19 | TaskOrchestrator 遇到非法状态转换仍写目标状态：`internal/agent/task_orchestrator_impl.go:1365-1387` | 状态机只是告警，不是约束 | Transition 返回 error；数据库 CAS。收益：非法状态不可落库 | V |
| C-20 | PlanExecutor 无 lease/inbox 去重，事件即启动后台执行：`internal/service/plan_executor.go:130-174` | replay、多实例或重复事件会重复组建 Team | execution lease + inbox event ID；应用生命周期 context。收益：支持 HA/replay | V |
| C-21 | OrchestrationSpec 无法无损表示 runtime 节点字段：`internal/biz/orchestration_spec.go:53-81`、`internal/team/embedded_graph.go:21-38` | API 读取再保存会丢 retry/fallback/reviewer 等行为 | 单一 schema 生成 Go/Proto/TS；golden round-trip。收益：配置不静默退化 | V |
| C-22 | Graph compiler 对未知节点/边 fail-open：`internal/team/embedded_graph.go:127-139,212-227,307-345` | 关键审批/节点被跳过后仍成功 | 严格聚合错误；禁止猜 entry/finish。收益：Graph 行为可预测 | V |
| C-23 | RuntimeReplanner 只记录决策，未控制 executor：`internal/graph/adapter/runtime_adapter.go:355-407` | 文档声明的 retry/reroute/fallback 实际不生效 | 返回 executor control command；先实现 rewind+retry/fallback。收益：真实恢复闭环 | V |
| C-24 | Prompt/Hybrid synthesis 仅生成提示词，不调用模型：`internal/biz/spirit_synthesis.go:138-175,226-236` | 用户得到原始 JSON/模型指令而非综合答案 | 注入 synthesis model port + 结构化 schema，模板作为 fallback。收益：输出真正整合 | V |

### 2.4 数据与平台

| ID | 问题与证据 | 影响 | 建议与收益 | 置信度 |
|---|---|---|---|---|
| C-25 | 大多数租户实体无统一 workspace_id | Repo 无法可靠隔离，后台任务易全库操作 | 全部 tenant-owned 表增加非空 workspace_id、复合唯一索引、RLS。收益：数据层硬隔离 | H |
| C-26 | Cron 排他仅进程内 mutex/map：`internal/cronrunner/runner.go:81-160` | 多副本重复执行 | Postgres lease/advisory lock/`SKIP LOCKED` + fencing token。收益：HA 调度 | V |
| C-27 | Monitor alert 用 `*sql.DB` 发送 `"BEGIN"`/`"COMMIT"`：`internal/data/monitor_alert.go:169-231` | 连接池下事务不成立，重复告警保护失效 | `BeginTx` + 条件更新/行锁。收益：告警窗口原子化 | V |
| C-28 | DDL migration 无 advisory lock，执行与版本登记不原子：`internal/data/ddl_migration_registry.go:137-240` | 滚动发布并发迁移、重复副作用 | 单实例迁移锁；单迁移事务/checksum。收益：安全部署 | V |

## 3. Major

| ID | 问题与证据 | 建议与收益 |
|---|---|---|
| M-01 | Critical/Important 事件可靠性规则与 v2 实体总线未统一 | 建立事件 catalog，按业务级别统一持久化、重试与丢弃策略 |
| M-02 | `last_event_id` 客户端发送但服务端不重放 | 实现 cursor replay，或删除字段并明确 snapshot reconciliation |
| M-03 | v2 重连不自动 hydration；子请求 `.catch(() => [])` | 引入 complete/partial/failed hydration 状态和重试 |
| M-04 | v1/v2 以 `hasV2Activities` 整体切换 | session 级版本门控，完整 snapshot 后原子切换 |
| M-05 | Step delta 早于 StepCreated 时丢失 | pending delta buffer + sequence merge |
| M-06 | 前端 Step/Spirit 状态 union 与后端不一致 | 由 Proto/schema 生成 enum，unknown 显式 fallback |
| M-07 | 9 个 chat/v2 component 直接使用 Pinia store | container/composable 取数，presentational component 只收 props/emits |
| M-08 | `v-html` 两处被 ESLint 报警 | 保证 DOMPurify/白名单在调用点可证明，增加 XSS fixture |
| M-09 | CodeExecutor 缺 PID/cap drop/non-root/seccomp，总输出无界 | OCI hardening + ring writer + 文件总数/总量限制 |
| M-10 | Artifact 文件与 sidecar 非原子，兼容绝对路径缺 containment | 临时文件+rename；绝对路径也做 root 校验 |
| M-11 | MCP per-user credential 在 Invocation 前解析 | 每次工具调用解析；连接池按 tenant/user/credential version 分区 |
| M-12 | 流式工具绕过统一 timeout/result budget | idle/total timeout、bytes/events 上限和显式 close |
| M-13 | Cost Guard 持久化失败 fail-open | 数据库原子 reservation；高成本请求在不确定时 fail-closed |
| M-14 | Skill import job 无 owner/workspace，崩溃后 applying 卡死 | owner/tenant + lease/heartbeat + Saga/DLQ |
| M-15 | Knowledge ingest 用进程内 goroutine，步骤非原子 | durable ingest job + 幂等步骤 + 恢复扫描 |
| M-16 | Memory/Sleep-time/Link Evolution 队列易失或静默 drop | Postgres job/outbox + fair quota + DLQ |
| M-17 | Rerank 丢 MetadataJSON；Memory 注入无统一 token budget | 保留 provenance；按模型窗口分层预算 |
| M-18 | Model Catalog snapshot/meta 非原子、同步无锁、ID 秒级冲突 | immutable snapshot + CURRENT 指针 + lease + ULID |
| M-19 | Catalog HTTP redirect 未重新做 SSRF 校验 | CheckRedirect + DialContext 最终 IP 检查 |
| M-20 | ChannelTurnJob 更新无 expected-status CAS，Sweeper 绕过 FSM | 所有转换统一 CAS API |
| M-21 | Evaluation/Learning 多状态实体缺权威 FSM/CAS | 显式 FSM、事务/outbox、partial 终态 |
| M-22 | Usage rollup 无 outbox/去重；purge 使用 SQLite 日期函数 | ledger + consumer idempotency；Go 计算 cutoff |
| M-23 | 外键延期，核心关系主要靠应用层级联 | Postgres `NOT VALID` FK 分批上线再 validate |
| M-24 | Trivy `continue-on-error: true` | 基线化后阻断新增 High/Critical |
| M-25 | 根模块 CI 不覆盖嵌套 `pkg/trpc-agent-go` modules | 启用已有多模块测试脚本，关键包单独 race |
| M-26 | E2E 仅 nightly，存在条件跳过/吞错 | PR 必跑 auth/chat/cancel/reconnect smoke；前置缺失直接失败 |
| M-27 | 架构 lint 对 9 个宽接口和 40+ 超限 struct 只 `t.Logf` | 建 allowlist/baseline，新增超限阻断，存量逐步清零 |

## 4. 文档与契约问题

| ID | 问题 | 证据 |
|---|---|---|
| D-01 | `docs/README.md`、`docs/guides/execution-plan.md`、`README-development.md` 缺失但被广泛引用 | `0-system-diagram.md:3-8`、`0-system.development.md:3-8` |
| D-02 | 70 号文档仍把已删除 Event WAL/EventStore/Postgres replay 标为完成 | `70-orchestration-longtask-memory.development.md:71-75,950,1007`；drop migration |
| D-03 | 65 号交叉参考仍以 Envelope/旧前端路径描述事件 | `65-module-cross-reference-full.md:175,871-880` |
| D-04 | Chat 设计缺 7 个现有 RPC | `1-chat.design.md:142-158` vs `chat.proto:264-390` |
| D-05 | Team 设计缺 Pause/Unpause/Inject 3 个 RPC | `11-multi-agent.design.md:124-151` vs `team.proto:739-759` |
| D-06 | 数据库设计记录 82 Schema，仍列已 DROP Message/EventStore，缺 11 个 v2 Schema | `66-database-architecture.design.md:386-430` |
| D-07 | 数据库需求同时存在 Postgres-only 和 SQLite 可降级两套口径 | `66-database-architecture.md:23-30,58-66,159-175,201-215` |
| D-08 | README 跨模块参考 8 项多数不存在或已改名 | `docs/development/README.md:71-84` |

## 5. 修复完成定义

任何问题只有同时满足以下条件才可关闭：

1. 代码修复或明确的产品降级决策已落地。
2. 有失败先行的单元/集成/安全测试。
3. 相关 Proto、Schema、前端类型和三件套同步。
4. 对 Critical/Blocker 提供故障注入或双租户验证。
5. 对行为变更定义回滚方式与可观测指标。

## 6. 整改进度（2026-07-16）

> 详细对照与残余见 `docs/reports/2026-07-16-review-b01-b06-cas-remediation.md`。

| ID | 状态 | 说明 |
|---|---|---|
| B-01 | 🟡 部分关闭 | JWT `workspace_id` + `admins.workspace_id` 1:1；中间件拒 forge；无 membership 表/RLS |
| B-02 | ✅ 已修 | SessionAuthorizer；`session_id=*` admin gate |
| B-03 | ✅ 已修 | 生产 CodeExecutor fail-closed |
| B-04 | ✅ 已修 | Spirit 不提前 completed |
| B-05 | ✅ 已修 | PlanBoard 返回更新后的 board |
| B-06 | 🟡 部分关闭 | 终态 WBPF + WS 高优；重连 REST hydrate；Seq `RestoreAtLeast`；无服务端 cursor replay |
| B-07 | ✅ 已修 | 前端门禁与 sequencer race 测试修复（以当前 CI 为准） |
| C-01 | ✅ 已修 | Knowledge workspace stamp/filter/assert |
| C-13 | ✅ 已修 | session_turns.idempotency_key unique + OnConflict |
| M-02 | 🟡 契约明确 | `last_event_id` echo-only；客户端 snapshot reconciliation |
| M-03 | 🟡 部分关闭 | 重连自动 `fetchSessionHistory`；尚无 complete/partial/failed 三态 |
| Upsert CAS (Task/Turn/Step) | ✅ 已具备 | `VersionLT` 单调守卫 + 三实体回归测试 |
| C-25 | 🟡 部分关闭 | Repo Get/List workspace predicates（channel/cron/task_v2/knowledge/mcp/skill/tool/agent/team）+ RLS ENABLE（无 FORCE）+ GUC helper；池化 `SET LOCAL` 未全链路接线 |
| D-01 | ✅ 已修 | `docs/README.md`、`guides/execution-plan.md`、`README-development.md` stub |
| D-02 | ✅ 已修 | 70 号 EventWAL/EventStore/Postgres replay 标 superseded |
| D-03 | ✅ 已修 | 65 事件节对齐 v2 Activity/Monitor bus |
| D-04 | ✅ 已确认 | `1-chat.design.md` §4.1 已含 Submit/Retry/Pause/Resume/Plans 等 RPC |
| D-05 | ✅ 已确认 | `11-multi-agent.design.md` 已含 Pause/Unpause/Inject |
| D-06 | ✅ 已修 | 66 design 剔除 Message/EventStore/FTS 活文档表述 |
| D-08 | ✅ 已修 | `development/README.md` 跨模块表指向真实文件 |
