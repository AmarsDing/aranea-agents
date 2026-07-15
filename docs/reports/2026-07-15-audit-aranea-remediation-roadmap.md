# Aranea-Agents 整改路线图

> 日期：2026-07-15  
> 目标：先恢复安全与事实一致性，再建设 durable、多实例和行业级运营能力。  
> 原则：每批均独立可回滚；Blocker/Critical 必须带失败先行测试；不把大重构与功能扩展混在同一 PR。


## 1. 优先级与依赖

```text
租户身份/对象授权 ──┬── Tool/MCP/Knowledge/Artifact 隔离
                    └── WS/Channel/后台任务授权

Canonical Execution ID ── PlanExecutor 状态/CAS ── durable lease/inbox
                         └── Event outbox/replay ── 前端 v2 单一投影

质量门禁恢复 ── 契约生成/覆盖检查 ── 文档状态自动校验
```

## 2. Wave 0：发布冻结与门禁恢复

目标：不再让已知错误继续累积。

建议 PR：

1. `ci: restore frontend required checks`
   - 修复 vue-tsc 错误。
   - 修复 2 个 Vitest 失败。
   - 修复 9 个 layer violation。
   - 清理 strict ESLint、Prettier、Stylelint。
2. `test: make sequencer race-safe`
   - 修复测试 fake bus 的同步。
   - 单独运行 `go test -race ./internal/agent/v2 -count=10`。
   - 复验生产 Sequencer 是否有同型竞态。
3. `ci: enforce truthful gates`
   - Trivy 对新增 High/Critical 阻断。
   - 启用 service coverage、frontend layer、关键嵌套 module 测试。

验收：

- 所有现有 CI 命令本地通过。
- PR required checks 与 workflow 一致。
- 不允许通过 `continue-on-error` 或宽泛 skip 绕过核心门禁。

## 3. Wave 1：安全与租户边界

目标：建立不可绕过的 principal/tenant/object authorization。

### 1A：认证与授权内核

- 定义 `Principal{UserID, WorkspaceID, Roles, Capabilities}`。
- Workspace 不再信任 query/header；必须由 membership 解析。
- 建立统一 `Authorizer`，覆盖 HTTP、WS、Channel、Cron 和后台任务。
- 对 system job 使用显式 system capability，不使用匿名 `context.Background()`。

### 1B：数据隔离

- 给 tenant-owned 表补 `workspace_id NOT NULL`。
- 唯一索引包含 workspace。
- Repo API 强制 TenantScope。
- PostgreSQL 分阶段启用 RLS。

### 1C：高风险对象

- WS session 和 `session_id=*` 授权。
- Knowledge collection/document/chunk 授权。
- Artifact owner/workspace 和签名 URL。
- Plugin runtime 按 workspace 分区。
- Tool workspace 按 workspace/user/session/agent 分层。
- MCP/A2A credential 迁移 SecretRef/KMS。

建议 PR 边界：

1. `security: introduce principal and workspace membership`
2. `security: authorize websocket sessions`
3. `security: tenant-scope knowledge and artifacts`
4. `security: isolate tool and plugin runtimes`
5. `security: migrate mcp and a2a secrets`
6. `security: make code executor fail closed`

验收：

- 用户 A 无法读、订阅、取消、删除用户 B 的任何对象。
- 伪造 workspace header/query 无效。
- DB 直查和服务调用都有 tenant predicate。
- 生产配置下隔离执行器不可用时拒绝执行。

## 4. Wave 2：编排正确性与单一事实源

目标：消除假成功、多 ID 和非法状态。

### 2A：Canonical execution

- 定义 `OrchestrationExecutionID`。
- TaskPlan、PlanBoard、GraphExecution、TeamRun 统一引用该 ID。
- 删除不可持久化的 placeholder handle。
- 所有 command 带 idempotency key。

### 2B：PlanExecutor

- 修复 board 副本状态 bug。
- 运行前严格验证 DAG。
- 所有状态转换 CAS。
- 取消后等待 worker barrier，再计算终态。
- 事件消费使用 inbox 去重和 execution lease。

### 2C：Graph/Team

- OrchestrationSpec 由单一 schema 生成。
- Graph compiler 默认 fail-closed。
- FunctionResolver 失败默认终止。
- RuntimeReplanner 返回可执行 control command。
- Synthesis 成为独立、可恢复的 DAG 节点并实际调用模型。

建议 PR：

1. `fix: make plan board transitions authoritative`
2. `fix: reject invalid plan dags`
3. `feat: add canonical orchestration execution`
4. `feat: lease and deduplicate plan execution`
5. `fix: make graph compiler fail closed`
6. `feat: execute replanner control commands`
7. `feat: make synthesis a durable node`

验收：

- 环、悬挂依赖、无根图全部拒绝。
- 重复 PlanBoardCreated 只启动一次。
- cancelled 后无 step 尾写。
- accepted/running/completed 事件顺序可证明。
- 一次请求只产生一个 execution identity。

## 5. Wave 3：核心运行与事件可靠性

目标：实现 RPO=0 的关键终态和可恢复入站。

### 3A：Chat/Run

- Session lock entry 引用计数。
- Cancel 保留 active lease，直到 run goroutine Finish。
- cancellation cause 统一为 cancelled。
- Run 状态持久化返回错误。
- canonical turn idempotency unique。
- turn number 使用锁/CAS/唯一约束。

### 3B：Durable messaging

- PendingQueue 迁移数据库 queue。
- v2 Critical event 使用 transactional outbox/WBPF。
- 持久化 DLQ 和 replay worker。
- 每 session 单调 sequence。

### 3C：前端恢复

- WS `resume_from` 或 REST `after_sequence`。
- 重连执行 authoritative snapshot hydration。
- snapshot 期间缓存实时事件。
- Step delta pending buffer。
- UI 显示 gap/partial/stale 状态。
- v2 snapshot 完整前不遮蔽 legacy。

建议 PR：

1. `fix: retain run lease through cancellation`
2. `fix: make session locks sweep-safe`
3. `feat: enforce canonical turn idempotency`
4. `feat: persist pending turn queue`
5. `feat: add v2 event outbox and sequence`
6. `feat: reconcile websocket reconnects`
7. `refactor: make v2 the authoritative chat projection`

验收：

- publish/persist 任意窗口 kill 进程，重启后终态不丢。
- 相同 idempotency key 不重复调用模型。
- 断网 60 秒后重连，最终状态与数据库一致。
- 用户取消后不能启动重叠 run。

## 6. Wave 4：平台数据与多实例

目标：消除单实例假设和非原子后台任务。

- Cron 使用数据库 lease/fencing/misfire policy。
- ChannelTurnJob 所有转换使用 expected-state CAS。
- Knowledge/Skill/Memory/Model Catalog 后台任务使用 durable job + lease + heartbeat + DLQ。
- Usage 使用 append-only ledger、outbox 和 consumer dedupe。
- Cost Guard 使用原子 reservation。
- Monitor alert 使用 `sql.Tx`，通知走 delivery outbox。
- Migration 使用 advisory lock、checksum 和可恢复 data migration。
- 增加 FK：先 `NOT VALID`，清理数据后 validate。

验收：

- 两实例同时启动不会重复 Cron/Plan/Import/Migration。
- 后台任务 kill/restart 可自动继续。
- Usage rollup 可从 raw ledger 重建。
- 无孤儿核心记录。

## 7. Wave 5：能力治理与质量闭环

目标：让多 Agent 能力可运营、可评估、可控成本。

- Tool/MCP capability policy、审计、结果预算和流式 idle timeout。
- CodeExecutor OCI hardening、无界输出保护、恶意样本测试。
- Skill/Plugin 签名、审核、canary、rollback。
- Memory provenance、token budget、durable maintenance。
- Evaluation 固定 Agent/Prompt/Model/Dataset 版本，建立真实 contract fixture。
- Evolution/Learning 统一 Saga：Evidence→Proposal→Approval→Apply→Verify→Rollback。
- Model Catalog immutable signed snapshot + canary workspace rollout。

验收指标：

- 工具策略拒绝率、超时率、预算超限率可观测。
- 沙箱恶意测试全通过。
- LoCoMo/LongMemEval 或等价基准可复现。
- 每次模型/Prompt/Skill 变更都有 eval delta 和回滚点。

## 8. Wave 6：文档真理库重建

在行为稳定后处理，避免再次追着代码改文档。

1. 建立 `docs/README.md` 和唯一进度索引。
2. 重写 65 号交叉参考的 Activity v2/MonitorEvent 架构。
3. 从 Proto 自动生成 RPC appendix。
4. 从 Ent Schema/迁移自动生成数据库 appendix。
5. 70 号文档按当前代码重置状态；删除已下线 WAL/EventStore/replay 的完成声明。
6. 数据库文档统一 PostgreSQL 生产口径，SQLite 仅测试适配。
7. CI 增加 Markdown link、code anchor、RPC/Schema drift 检查。

验收：

- 0 broken links。
- API 表与 Proto 100% 一致。
- Schema 表与 Ent/迁移 100% 一致。
- 每个完成标记都有测试或代码证据链接。

## 9. 回归矩阵

| 风险 | 必须测试 |
|---|---|
| 多租户 | 两用户×两 workspace×HTTP/WS/Job/Repo |
| 幂等 | 重试、重复事件、并发相同 key、多实例 |
| 状态机 | 所有合法/非法/终态转换 + CAS 冲突 |
| 取消 | build/stream/tool/persist 各阶段取消 |
| 崩溃恢复 | enqueue、publish、persist、checkpoint、apply 各窗口 kill |
| Graph | chain/diamond/cycle/disconnected/unknown node/side effect |
| 沙箱 | fork bomb、无限 stdout、网络、宿主路径、海量文件 |
| 前端实时 | 乱序、重复、缺包、断网、snapshot partial、token expiry |
| 数据库 | 双实例 migration、FK/RLS、Postgres 方言 |
| 成本 | 并发 reservation、重复 provider request、rollup rebuild |

## 10. 不建议现在实施

- 完整 Event Sourcing/CQRS。
- 新增更多编排模式枚举。
- 跨 Team 自主协商扩展。
- 在 durable execution 和安全边界完成前扩大 Marketplace/远程插件面。

这些方向成本高，且不能解决当前最主要的生产风险。
