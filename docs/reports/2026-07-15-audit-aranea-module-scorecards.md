# Aranea-Agents 模块评分卡与行业方向

> 日期：2026-07-15  
> 评分：5=生产级闭环；4=主体成熟、有局部缺口；3=可用但可靠性/治理不足；2=核心闭环不完整；1=占位或高风险不可用。

## 1. 核心与编排

| 模块 | 当前定位 | 评分 | 主要问题 | 建议目标态 | 收益与验收指标 |
|---|---|---:|---|---|---|
| Chat/Turn | 多入口对话与执行协调器 | 3.0 | 取消租约、终态覆盖、持久化错误、幂等未闭环 | Canonical Turn + durable admission + terminal outbox | 同 key 只执行一次；取消后无重叠 run；三套终态一致 |
| Session | 会话、历史与运行上下文 | 3.0 | session lock sweep、tenant predicate、v1/v2 并存 | Tenant-scoped aggregate + entity CAS + snapshot revision | 双租户矩阵 100%；长任务持锁 sweep 无并发进入 |
| Event/Activity v2 | 实时实体投影 | 2.0 | 双层 drop、无 replay、DLQ 内存态 | Durable event log/outbox + session sequence + projector | kill/restart 后终态不丢；reconnect 自动收敛 |
| Agent | Agent 配置与运行时装配 | 3.5 | runtime setting 复杂、工具 workspace/凭据装配边界弱 | Policy-bound runtime profile + per-invocation identity | 所有工具调用携带 tenant/principal/policy version |
| Team | 多 Agent 团队定义与执行 | 3.0 | OrchestrationSpec 非无损、状态真源与 Graph 重叠 | Team 作为 versioned Graph spec + execution view | Go/Proto/TS round-trip 0 字段丢失 |
| Graph | 确定性编排和恢复 | 3.0 | compiler fail-open、replanner 不执行、function no-op | Strict compiler + versioned graph + durable control commands | 非法图 100% 拒绝；retry/fallback 故障注入通过 |
| Spirit/Planner | 自然语言计划与多 Team 编排 | 2.0 | 假完成、三套 ID、策略依赖 LLM mode | Deterministic policy + canonical execution + durable PlanBoard | accepted/running/completed 时序正确；一个请求一个 execution ID |
| PlanExecutor | DAG 调度器 | 2.0 | board 副本、循环图成功、取消竞态、无 lease | Lease/CAS scheduler + validated DAG + worker barrier | 环/悬挂依赖拒绝；多实例只执行一次；取消后无尾写 |
| HITL | 工具、Graph、Team 人工介入 | 3.0 | 多层机制分散，Resume 双写 | `HumanLoopGate` + durable suspend token + policy | approval/checkpoint 共用协议；恢复 RTO 可测 |
| Checkpoint/TimeTravel | Graph 状态恢复与分叉 | 3.5 | Graph 能力实质存在，但端到端进程恢复证据弱 | Process-kill recovery + side-effect idempotency | 真实 Postgres kill/restart 测试通过 |

行业选择原则：

- **确定性 Graph**：支付、审批、数据修改、合规、可重放流程。
- **Coordinator/Agent-as-Tool**：任务可结构化、需要专家能力但控制权必须集中。
- **Swarm/Adaptive**：开放探索、信息检索、低副作用任务；必须有最大 handoff、预算和停止条件。
- **禁止增加自治性**：没有幂等、沙箱、成本上限、可回滚副作用或人工门禁的场景。

## 2. 能力模块

| 模块 | 当前定位 | 评分 | 主要问题 | 行业目标态 | 核心指标 |
|---|---|---:|---|---|---|
| Tools | 内置/自定义工具装配与治理 | 2.5 | cache 串租户、目录逃逸、stream 绕预算 | Capability token + PDP/PEP + invocation sandbox | 敏感工具 0 跨 invocation cache；100% policy audit |
| MCP | 外部工具协议接入 | 2.5 | 静态凭据明文、用户凭据装配时机、连接分池不足 | SecretRef + per-call credential + identity-partitioned pool | 凭据轮换无需重启；日志无 secret |
| Skill | 技能发现、导入、演进 | 3.0 | import owner/tenant、lease/Saga、临时文件回收 | Signed supply chain + approval + lease/Saga | kill/restart 可恢复；包签名和来源可追溯 |
| Plugin | 运行时插件扩展 | 2.5 | 全局 active 状态、workspace 重载覆盖 | Workspace-scoped immutable snapshot + staged rollout | 多 workspace 并发重载互不影响 |
| Memory L0-L4 | 分层长期记忆 | 3.0 | 维护队列易失、重巩固未接生产、token budget 缺失 | Provenance-aware temporal memory + durable maintenance | LoCoMo/LongMemEval；token budget；增长上限 |
| Knowledge/RAG | 文档摄取与检索 | 2.0 | tenant 隔离、goroutine ingest、全局进度事件 | ACL-aware RAG + durable ingest workflow | 双租户检索 0 泄漏；kill/restart 自动续跑 |
| Artifact | 会话产物存储 | 2.0 | IDOR、sidecar 非原子、绝对路径兼容 | Object store + metadata DB + fine-grained ACL | owner/workspace 强制；崩溃无孤儿/路径逃逸 |
| CodeExecutor | 不可信代码执行 | 1.5 | Local fail-open、容器 hardening、无界输出 | Per-run microVM/rootless sandbox + egress policy | 生产绝不 Local；恶意样本测试全部通过 |
| Model Catalog | 模型目录同步与应用 | 2.5 | snapshot 非原子、同步竞态、redirect SSRF、partial success | Signed immutable catalog + canary rollout + rollback | snapshot checksum；并发同步单写；一键回滚 |
| Provider/Model Router | 模型提供商和路由 | 3.0 | 配置事务、成本治理和 capability 证据仍分散 | Policy router：质量/价格/区域/合规/容量共同决策 | 每次选择保留 reason；预算预留强一致 |

## 3. 接入、治理与数据

| 模块 | 当前定位 | 评分 | 主要问题 | 行业目标态 | 核心指标 |
|---|---|---:|---|---|---|
| Channel | 外部消息入口 | 3.0 | Job CAS、异步 deadline、租户字段 | Durable inbox → policy → workflow → outbox | receipt 永久幂等；终态单调 |
| Cron | 定时任务 | 2.5 | 进程内排他、重试可重复建 session | DB lease + fencing + misfire policy | 双实例同 task 只执行一次 |
| A2A | 远程 Agent 协作 | 2.5 | SSRF、凭据明文、能力范围 | mTLS/OIDC + capability scope + egress allowlist | 每个 task ID 幂等；调用全审计 |
| Evaluation | Agent/模型评测 | 2.5 | Run FSM、结果/计数事务、mock 假阳性 | Immutable eval snapshot + recoverable shard | 固定 agent/prompt/model/dataset version；结果完整率 100% |
| Evolution | Agent/Skill 演进 | 3.0 | 部分状态机已好，跨模块应用仍需 Saga | Evidence → Proposal → Approval → Apply → Verify → Rollback | 高风险双人审批；可一键回滚 |
| Learning Loop | 知识学习闭环 | 2.5 | Proposal CAS、建议创建失败仍 applied | Unified governance Saga + outbox | applied 必有可追溯 suggestion/artifact |
| Monitor/Telemetry | 指标、Trace、告警 | 2.5 | 告警事务错误、通知 fire-and-forget、MonitorEvent 可丢 | OTel + alert claim/outbox/DLQ + cardinality governance | 通知去重；delivery attempt 可查询 |
| Usage/Quota | 用量与额度 | 2.5 | rollup 无去重、purge 方言、cost guard fail-open | Append-only ledger + atomic reservation + rebuildable rollup | provider request ID 去重；额度不可并发超卖 |
| Auth/Workspace | 身份、权限与租户 | 1.5 | workspace 可伪造、RBAC/ABAC 不完整、Repo 无硬隔离 | OIDC + tenant membership + capability + RLS | 双租户安全矩阵；所有写 RPC 有 policy decision |
| Database | PostgreSQL/Ent/迁移 | 2.5 | FK 延期、migration lock、文档方言冲突 | Postgres-native、FK/RLS、advisory migration lock | 滚动部署迁移互斥；0 orphan；Schema 文档自动生成 |

## 4. 前端与质量

| 模块 | 当前定位 | 评分 | 主要问题 | 行业目标态 | 核心指标 |
|---|---|---:|---|---|---|
| Chat UI v2 | Agent 运行时间线与控制面 | 2.0 | v1/v2 双轨、部分 hydration、状态类型漂移 | Authoritative snapshot + realtime overlay + gap UX | reconnect 后状态收敛；无幽灵 running |
| Frontend architecture | Feature/store/composable/component | 2.0 | 9 个组件直连 store，page/API 例外散落 | Container/composable 数据层 + presentational components | `check:layer` 0 violation |
| API contract | Proto 生成 client + mapper | 2.5 | 手写 mapper 默认值、enum 漂移、服务覆盖脚本未进 CI | Generated contract + runtime schema validation | RPC coverage 100%；enum exhaustive |
| Permission UX | 登录态路由 | 2.0 | 无 capability route/button guard | 后端 policy + capability manifest 驱动 UI | 403 前置拦截；危险操作 reason/approval |
| Realtime UX | WS + store | 2.0 | 无 replay、无 gap marker、断连竞态 | sequence/cursor/snapshot/partial-data UX | reconnect、gap、hydration latency SLO |
| Unit/Contract tests | Vitest/Go tests | 2.5 | 当前失败、mock 假阳性、关键安全路径缺失 | Generated fixture + contract/integration/fault tests | 单测 100% 通过；关键 mapper branch coverage |
| E2E | Playwright nightly | 2.0 | PR 不阻断、条件跳过/吞错 | PR smoke + nightly full matrix | auth/chat/cancel/reconnect/tenant 必过 |
| CI/Security | Lint/race/coverage/Trivy/CodeQL | 2.0 | 当前多门禁失败；Trivy 非阻断；嵌套模块未覆盖 | Required checks + baseline delta blocking | 新 High/Critical 漏洞 0；关键模块 race |

## 5. 文档治理评分

| 维度 | 评分 | 事实 | 目标 |
|---|---:|---|---|
| 三件套覆盖 | 4.0 | 约 52 个完整模块 | 保持 |
| 全局入口 | 1.5 | 多个入口/进度真理文件缺失 | 单一可导航索引 |
| API/Schema 同步 | 2.0 | Chat/Team RPC、v2 Schema 漂移 | 自动生成 appendix + CI diff |
| 状态真实性 | 1.5 | 70 号文档多项已删除能力仍标完成 | 状态由测试/代码证据反向生成 |
| 代码锚点 | 2.5 | 多数有锚点，部分文件名/路径失效 | 自动链接检查 |

## 6. 总体目标态

建议用五个可量化目标定义下一阶段，而不是用新增功能数量：

1. **Security**：所有 tenant-owned 资源有 workspace predicate/RLS；双租户攻击矩阵全绿。
2. **Durability**：所有 Critical command/event/job 可在 kill/restart 后恢复，RPO=0。
3. **Determinism**：一个用户意图只有一个 canonical turn/execution ID，状态转换均 CAS。
4. **Operability**：所有后台队列具有 depth/lag/retry/DLQ；所有错误可关联 request/run ID。
5. **Quality**：typecheck、unit、race、layer、format、style、安全扫描和 PR smoke E2E 全部 required。
