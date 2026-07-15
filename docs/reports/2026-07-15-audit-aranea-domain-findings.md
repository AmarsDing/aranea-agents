# Aranea-Agents 分域审计结论

> 日期：2026-07-15  
> 本文保留各域的独立结论；跨域同源问题已在问题登记册中去重。

## 1. 核心架构与 Chat 运行链

### 合理之处

- Biz 与 tRPC Agent 解耦、Service 与 Data 隔离均已通过架构测试。
- TurnGateway、RunRegistry、状态机和 Activity v2 的抽象方向正确。
- Provider transport 已区分 timeout/retry/circuit breaker。

### 主要缺陷

1. WS/session 授权未形成对象级边界。
2. SessionLock sweep 可破坏 per-session 互斥。
3. Cancel 过早释放 active lease，且 failed defer 可覆盖 cancelled。
4. Run terminal persist error 被吞。
5. PendingQueue、Activity persist/DLQ 仍有内存态崩溃窗口。
6. IdempotencyKey 没有数据库唯一语义。
7. WS v2 无 sequence/ack/resume cursor。

结论：架构组件齐全，但一致性协议尚未闭环。应从“多个管理器协调”升级为“canonical command + durable state + outbox projection”。

## 2. Agent、Team、Graph、Spirit 与长任务

### 合理之处

- Graph Checkpoint/TimeTravel 是实质实现，不是空壳。
- GraphExecution WaitingHuman/Resume 状态机较完整。
- Team 默认采用 Graph 编译路径符合确定性和恢复方向。
- Agent allocator、failure policy、replanner、memory injection 等扩展点覆盖广。

### 主要缺陷

1. Spirit 工具在执行前发布 orchestration completed。
2. PlanBoard 状态副本导致 executing 后无法正确 terminal。
3. DAG 无完整拓扑校验，循环图可零执行成功。
4. TaskPlan/PlanBoard/Handle 多 ID。
5. PlanExecutor 无 lease/inbox，重复事件和多实例可重复执行。
6. OrchestrationSpec 无法无损表达 runtime 节点。
7. Graph compiler 和 FunctionResolver fail-open。
8. RuntimeReplanner/TopologyEvolver 主要停留在记录层。
9. Synthesis 没有真正执行模型综合。
10. 旧 Spirit recovery 只恢复标记，不证明 executor 恢复。

结论：确定性 Graph 基础强于自治编排闭环。短期应加强 deterministic/durable 路径，不应继续增加编排模式。

## 3. Tools、MCP、Skill 与 Plugin

### 合理之处

- 工具装饰器、结果预算、MCP probe/health、Skill import、Plugin runtime 已有完整工程框架。
- 工具审计、确认和分类具有明确扩展点。

### 主要缺陷

1. 工具缓存键缺 tenant/user/session。
2. 工具工作目录只按 agentKey，绝对路径缺少统一 containment。
3. StreamableCall 绕过 timeout/result budget。
4. MCP 用户凭据解析时机早于 Invocation。
5. MCP 静态凭据明文保存。
6. Plugin runtime 是进程全局 active 快照。
7. Cost Guard 持久化故障 fail-open。
8. Skill import job 缺 owner/workspace、lease 和崩溃恢复。

行业方向：Capability token + PDP/PEP；调用时解析身份；连接池按身份分区；Skill/Plugin 采用签名供应链、审批、canary 和 rollback。

## 4. Memory、Knowledge 与 RAG

### 合理之处

- L0-L4 分层、Composite recall、主动记忆、Sleep-time、Link Evolution、Reconsolidation 等设计覆盖行业前沿能力。
- 检索、rerank、摄取和前端进度均有实现。

### 主要缺陷

1. Knowledge workspace 未形成硬隔离。
2. Ingest 使用进程内 goroutine，状态/chunk/count 非原子。
3. 进度通过全局 WS 广播。
4. Embedder 先改内存再持久化，失败仍返回成功。
5. Memory queue tenant key/公平性不足。
6. Rerank 丢失 MetadataJSON。
7. Reconsolidation/ConflictResolver 未接生产链路。
8. 维护队列易失，mutation 失败被当成功。
9. Memory 注入无统一 token budget。
10. LoCoMo、24h、主动召回准确率缺可复现实验。

行业方向：ACL-aware temporal memory；每条记忆带 provenance/version/confidence/TTL；维护任务 durable；以离线基准、token cost、memory growth 和恢复 RTO/RPO 作为验收。

## 5. Artifact、CodeExecutor 与 Model Catalog

### Artifact

- 对象级授权缺失。
- 文件与 sidecar 非原子。
- 兼容绝对 StorageURI 时未执行 root containment。

目标：内容寻址对象存储 + 元数据数据库 + workspace/owner ACL + tombstone cleanup。

### CodeExecutor

- 隔离后端不可用可回退 Local。
- Docker 缺 pids/cap-drop/non-root/no-new-privileges/seccomp 等硬化。
- stdout/stderr 与输出文件总量无界。
- 测试没有覆盖真实恶意样本。

目标：每 Run 独立 microVM/rootless sandbox；固定镜像 digest；网络/资源/输出预算；生产 fail-closed。

### Model Catalog

- Catalog/Meta 非原子快照。
- 同步无互斥，日志 ID 秒级碰撞。
- HTTP redirect 可绕初始 SSRF 检查。
- Apply 有行级错误仍报告 succeeded。
- RPC 将基础设施失败包装为 transport success。

目标：签名不可变 snapshot；lease + ULID；dry-run→canary workspace→全量→rollback。

## 6. Channel、Cron 与 A2A

### Channel

- 入站 receipt 幂等是正向基础。
- ChannelTurnJob Repo 更新没有 expected-state CAS，Sweeper 绕过 FSM。
- `context.WithoutCancel` 后没有新 deadline。

目标：Durable Inbox → Policy/Quota → Workflow → Transactional Outbox。

### Cron

- Retry 包裹整个 dispatch，可重复建 Session。
- 排他仅进程内锁。

目标：Postgres lease、fencing token、run idempotency key、misfire/catch-up policy。

### A2A

- Remote URL 缺 Dial/redirect SSRF 校验。
- auth config 明文 JSON。

目标：mTLS/OIDC、capability scope、egress allowlist、secret reference、task ID 幂等。

## 7. Evaluation、Evolution 与 Learning Loop

### 合理之处

- EvolutionSuggestion 已有显式状态机和 Apply/Reject/Rollback。
- Evaluation、Learning、Skill evolution 已覆盖数据、API 和 UI 基础。

### 主要缺陷

- EvalRun 多状态但无权威 FSM/CAS。
- Case result 插入失败仍增加 completedCases 并可能最终 completed。
- 批量 Cases 与 case_count 双写非原子。
- Learning Proposal 六态只做局部 if，Repo 无 CAS。
- 创建统一演化建议失败仍将 Proposal 标为 applied。
- 前端测试大量 mock 自定义 DTO，无法发现真实 Proto 漂移。

行业方向：固定 Agent/Prompt/Model/Dataset 版本的不可变 EvalRun；Observation→Evidence→Proposal→Approval→Apply→Verify→Rollback 的统一治理 Saga。

## 8. Monitor、Telemetry、Usage 与 Quota

### Monitor/Telemetry

- OTel、Trace、Flow、Alert 结构齐全。
- Alert 状态事务使用方式错误。
- Notification fire-and-forget，无 durable delivery。
- MonitorEvent 明确 best-effort，不适合作为审计或关键告警真源。

### Usage/Quota

- Raw usage 先持久化是正向基础。
- Rollup 通过内存事件累加，无 outbox/消费去重。
- Postgres purge 使用 SQLite 日期表达式。
- Cost Guard queue 满/DB 失败时可漏计。

行业方向：append-only usage ledger；provider request ID 去重；原子预算 reservation；rollup 可重建；alert delivery outbox。

## 9. Auth、Workspace 与数据库

### Auth/Workspace

- 认证存在，但授权模型主要是“已登录”。
- workspace 来自客户端输入。
- 高权限 RPC 和页面缺少统一 capability。
- 多数表无 workspace_id。

目标：OIDC + RBAC/ABAC；JWT/服务端 membership；统一 capability decision；Repo tenant predicate；Postgres RLS。

### Database

- 当前生产代码已明确 PostgreSQL/pgvector。
- 文档仍存在 SQLite 降级口径。
- 外键被延期，核心关系依赖应用层级联。
- Migration 无多实例锁和原子版本登记。

目标：Postgres-native 方言、advisory migration lock、FK/RLS、timestamptz、真实 Postgres integration test。

## 10. 前端、契约与实时 UX

### 主要缺陷

1. v2 关键事件可丢且无 replay。
2. `last_event_id` 契约无效。
3. 重连不自动 snapshot reconciliation。
4. 子资源 hydration 错误被吞。
5. v1/v2 部分数据竞争。
6. Step delta 乱序丢失。
7. Step/Spirit 状态 enum 漂移。
8. 9 个 presentational component 直接依赖 Pinia。
9. SkillDedup/Pack/RuntimeProfile 的产品链路不完整。
10. Provider SVG `v-html` 需要供应链级消毒证明。
11. 路由/菜单缺 capability guard。
12. 跨域 WS bearer token 放 URL query。

目标：前端成为“可信 Agent 运行控制面”，明确展示 live/snapshot/gap/partial/stale/canceling/retrying/awaiting-approval。

## 11. 测试与 CI

- 后端普通单测和基础架构红线较好。
- 当前 race、前端 typecheck/unit/layer/lint/format/style 均不全绿。
- 架构复杂度告警不阻断。
- 根 CI 不覆盖嵌套 tRPC Agent modules。
- 前端无 coverage gate。
- E2E 不阻断 PR，且存在条件跳过/吞错。
- Trivy 明确非阻断。

行业目标：required checks 必须覆盖 contract generation、enum exhaustive、tenant matrix、race、fault injection、PR smoke E2E 和新增安全漏洞 delta。
