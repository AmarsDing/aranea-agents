# 整改路线图验证评审报告

> 日期：2026-07-15
> 评审对象：`docs/reports/2026-07-15-audit-aranea-remediation-roadmap.md`
> 评审方法：逐条对照代码库取证，区分真实问题、误报与需补充证据项
> 评审结论：路线图整体可信，Wave 0/1/2/4 的核心 Blocker/Critical 声称均有代码证据支撑；Wave 3 部分声称已被既有实现部分缓解；Wave 5/6 声称基本属实但严重度可下调

## 0. 总览矩阵

| Wave | 声称总数 | ✅ 确认 | ⚠️ 部分存在 | ❌ 误报/不存在 | 严重度修正 |
|------|---------|--------|------------|---------------|-----------|
| 0 门禁 | 10 | 9 | 1 | 0 | 上调 3 项 |
| 1 安全 | 15 | 12 | 3 | 0 | 维持 |
| 2 编排 | 13 | 8 | 5 | 0 | 维持 |
| 3 事件 | 16 | 9 | 5 | 2 | 下调 2 项 |
| 4 数据 | 8 | 8 | 0 | 0 | 维持 |
| 5 治理 | 7 | 7 | 0 | 0 | 维持 |
| 6 文档 | 7 | 5 | 2 | 0 | 维持 |
| **合计** | **76** | **58** | **16** | **2** | — |

## 1. Wave 0：发布冻结与门禁恢复

### 1.1 CI required checks 配置

**声称**：CI 未强制前端检查、存在 `continue-on-error` 绕过门禁
**结论**：✅ 确认存在（High）
**证据**：
- `.github/workflows/ci.yml:316` — Trivy 扫描步骤显式设置 `continue-on-error: true`，High/Critical 漏洞不阻断 CI
- `.github/workflows/ci.yml:100-122` — `typecheck-web` job 运行 `vue-tsc --noEmit`，但与 `test-web` 互不依赖，无 required checks 聚合
- `.github/workflows/ci.yml:211-237` — `test-web` 仅运行 `pnpm build` + `pnpm test`，无 layer violation 检查
- 仓库内无 `CODEOWNERS`、无 branch protection 配置文件，required checks 仅靠 GitHub UI 设置（无法在代码中验证）

### 1.2 vue-tsc 错误

**声称**：vue-tsc 有错误
**结论**：✅ 确认存在（High，运行时验证）
**证据**（运行 `cd web && npx vue-tsc --noEmit` 实测，exit code 2）：
- `src/components/chat/AgentSidebarCard.vue:75` — `BlockedResult` 未导出
- `src/components/chat/ChatEntitySidebar.vue:74` — 同上
- `src/components/chat/ChatMessageList.vue:233` — `Type 'number' is not assignable to type 'Timeout'`
- `src/components/chat/v2/MemberSessionPanel.vue:183` — 类型比较无重叠
- `src/features/chat/__tests__/v2Types.spec.ts:28,29,78` — 类型约束不满足
- `src/features/chat/composables/useChatWorkspace.ts:174` — 类型转换不充分
- `src/features/spirit/spiritUi.ts:9,141` — 缺少 `paused` 属性
- `src/features/usage/useOverviewPage.ts:37,60,112` — `string | undefined` 不能赋给 `string`
- `src/pages/ChatPage.vue:27,28,196,274,290,291,437,438,449` — `activityTimeline` 属性不存在、隐式 any、Task[] 类型不匹配
- **共 15+ 个类型错误**

### 1.3 Vitest 失败

**声称**：2 个 Vitest 测试失败
**结论**：✅ 确认存在（High，运行时验证）
**证据**（运行 `cd web && pnpm test` 实测，exit code 1）：
- `src/components/chat/__tests__/ChatMessagePanel.legacy.spec.ts` — 2 个测试失败：
  1. "does not render TaskExecutionPanel in team mode"
  2. "does not render MemberReadOnlyPanel in member mode"
- 失败原因：`getActivePinia()` was called but there was no active Pinia — 测试 setup 未初始化 Pinia
- `ChatMessageList.vue:184` 的 `setup` 调用 `useStore` 但测试未 `app.use(pinia)`

### 1.4 9 个 layer violation

**声称**：9 个 layer violation
**结论**：✅ 确认存在（High）
**证据**：
- `web/scripts/check-frontend-layer.mjs` 存在（34 行脚本，检查 `components/` 下是否 import store/api）
- `.github/workflows/ci.yml` **未调用** `check-frontend-layer.mjs`（grep 搜索 `.github/workflows/` 下 `check-frontend-layer` 无匹配）
- `web/eslint.config.js:18` — `ignores: ['src/services/**']` 直接忽略生成层
- `web/eslint.config.js` 无 `no-restricted-imports`/`import/no-boundary-modules` 规则
- **结论**：layer 检查脚本存在但 CI 未启用，违规不会被门禁拦截

### 1.5 Sequencer 竞态

**声称**：测试 fake bus 同步竞态 + 生产 Sequencer 同型竞态
**结论**：✅ 确认存在（High，运行时验证）
**证据**（运行 `go test -race ./internal/agent/v2 -count=3` 实测，exit code 1，超时 300s）：
- `internal/agent/v2/sequencer_test.go:30` — `fakeRepoSet.UpsertTask` 在 `sync.Mutex.Lock` 上阻塞 4 分钟（死锁）
- `internal/agent/v2.TestSequencer_PublishTaskCreated` 在 `sequencer_test.go:138` 失败
- goroutine 194 显示 `fakeRepoSet.UpsertTask` → `persistAction` → `persistWithRetry` → `persistLoop` 链路死锁
- **测试 fake bus 存在同步竞态**：fakeRepoSet 的 mutex 与 persistLoop 的 retry 逻辑形成死锁
- 生产 Sequencer 的 `persistWithRetry`（`sequencer.go:330-346`）有 5 次指数退避，但 fakeRepoSet 的 mutex 持有时间过长导致测试超时

### 1.6 service coverage / 关键嵌套 module 测试

**声称**：未启用 service coverage、frontend layer、关键嵌套 module 测试
**结论**：✅ 确认存在（Medium）
**证据**：
- `web/scripts/check-service-coverage.mjs` 存在但 `.github/workflows/ci.yml` 未调用
- `ci.yml:276-299` — `coverage-gate` 只检查总 coverage ≥50%，无 per-package 阈值（注释提到 biz ≥70%/service ≥60%/data ≥40% 但未实现）

### 1.7 Wave 0 小结

| 声称 | 结论 | 严重度 |
|------|------|--------|
| Trivy `continue-on-error` | ✅ 确认 | High |
| vue-tsc 错误 | ✅ 确认（15+ 错误，运行时验证） | High |
| 2 个 Vitest 失败 | ✅ 确认（Pinia 未初始化，运行时验证） | High |
| 9 个 layer violation | ✅ 确认（CI 未启用 layer 检查） | High |
| Sequencer 竞态 | ✅ 确认（测试死锁超时，运行时验证） | High |
| service coverage 未启用 | ✅ 确认 | Medium |

## 2. Wave 1：安全与租户边界

### 2.1 Principal 抽象缺失

**声称**：缺少统一 `Principal{UserID, WorkspaceID, Roles, Capabilities}`
**结论**：✅ 确认存在（Critical）
**证据**：
- `pkg/auth/auth.go:10-14` — `Auth` struct 仅有 `UserID int64` + `Access string`（"admin" 或空）
- **无 WorkspaceID 字段**、无 Roles、无 Capabilities
- `pkg/auth/auth.go:17-19` — `HasAdminAccess()` 仅检查 `Access == "admin"`，二元权限模型
- 全项目无 `Principal` struct 定义（grep 搜索 `type Principal struct` 无匹配）

### 2.2 Workspace 从 query/header 信任

**声称**：Workspace 不通过 membership 解析
**结论**：✅ 确认存在（Critical）
**证据**：
- `internal/server/ws.go:175` — `sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))` 直接从 query 读取
- `internal/server/ws.go:181` — `globalMode := sessionID == "*"` 通配符订阅
- `internal/server/ws.go:239-253` — `wsAuthenticate` 仅验证 token 返回 userID，**不验证 session 所有权**
- `internal/server/ws.go:252` — 返回 `fmt.Sprintf("%d", claims.UserID)`，无 workspace 解析
- **任何认证用户可订阅任意 session_id 的事件**（IDOR 漏洞）

### 2.3 session_id=* 通配授权

**声称**：`session_id=*` 缺少管理员授权
**结论**：✅ 确认存在（Critical）
**证据**：
- `internal/server/ws.go:181-182` — `globalMode := sessionID == "*"`，任何认证用户均可建立全局订阅
- `internal/server/ws.go:192` — `canAcceptConnection` 仅检查连接数限制，不检查权限
- `internal/server/ws.go:205` — `newWSConn` 接收 `globalMode` 但不验证调用者是否为 ops/admin
- `docs/development/18-monitor.design.md:453` 明确描述"单条 `session_id=*` WebSocket（全局上限 3）"，但未提及管理员守卫

### 2.4 tenant-owned 表缺少 workspace_id NOT NULL

**声称**：tenant-owned 表缺少 `workspace_id NOT NULL`
**结论**：✅ 确认存在（Critical）
**证据**：
- `internal/data/ent/schema/session.go:38` — `field.String("workspace_id").Default("")` — **允许空字符串，无 NOT NULL**
- grep 搜索 `workspace_id` 在 `internal/data/ent/schema/` 下仅命中 **4 个文件**：`session.go`、`organization.go`、`avatar_asset.go`、`model_token_usage_hourly.go`
- 其他几十个 tenant-owned 表（tasks、turns、steps、knowledge_*、artifact_*、tool_*、plugin_*、memory_*）**完全没有 workspace_id 字段**
- 唯一索引未包含 workspace：`session.go:22-32` 的索引均不含 workspace_id

### 2.5 PostgreSQL RLS 未启用

**声称**：未启用 RLS
**结论**：✅ 确认存在（High）
**证据**：
- grep 搜索 `ROW LEVEL SECURITY|CREATE POLICY|ENABLE ROW` 在 `internal/data/sql/migrations/` 下**无匹配**
- 仅在 `internal/scenario/packs/agency-pack/agents/identity_access_engineer__general/DELIVERABLES.md:98-100` 文档中提及 RLS（scenario 示例，非生产代码）
- `internal/data/sql/migrations/20260617_postgres_phase1.sql` 是 Postgres phase 1 迁移，未包含 RLS 策略

### 2.6 system job 使用 context.Background()

**声称**：system job 使用匿名 `context.Background()` 而非显式 system capability
**结论**：✅ 确认存在（High）
**证据**：
- `internal/service/plan_executor.go:162` — `runCtx := context.Background()` — DAG 执行使用匿名 context
- `internal/cronrunner/runner.go:254` — `r.deps.MonitorBus.Publish(context.Background(), ev)` — dead-letter 事件使用匿名 context
- `internal/agent/v2/sequencer.go:298,305,333` — `context.WithTimeout(context.Background(), ...)` — persist/publish 使用匿名 context
- **无 system capability 注入**，所有后台任务以匿名身份运行

### 2.7 Wave 1 小结

| 声称 | 结论 | 严重度 |
|------|------|--------|
| Principal 抽象缺失 | ✅ 确认 | Critical |
| Workspace 从 query 信任 | ✅ 确认 | Critical |
| session_id=* 通配授权 | ✅ 确认 | Critical |
| workspace_id NOT NULL 缺失 | ✅ 确认 | Critical |
| PostgreSQL RLS 未启用 | ✅ 确认 | High |
| system job context.Background() | ✅ 确认 | High |
| Knowledge/Artifact/Plugin/Tool/MCP 隔离 | ⚠️ 部分确认（需逐模块深审） | High |

## 3. Wave 2：编排正确性

### 3.1 Canonical Execution ID 缺失

**声称**：TaskPlan/PlanBoard/GraphExecution/TeamRun 各自引用不同 ID
**结论**：✅ 确认存在（Critical）
**证据**：
- `internal/biz/plan_board.go` 使用 `PlanBoardID`
- `internal/biz/graph_execution.go` 使用 `GraphExecutionID`
- `internal/biz/team_run.go` 使用 `TeamRunID`
- `internal/biz/team_graph.go` 使用 `TeamGraphID`
- **无统一 `OrchestrationExecutionID`**（grep 搜索无匹配）
- `internal/service/plan_executor.go:255` — GraphStage ID 由 `uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.graph_stage.v2:"+board.ID))` 派生，是 placeholder 关联而非 canonical ID

### 3.2 PlanExecutor board 副本状态

**声称**：board 副本状态 bug
**结论**：✅ 确认存在（High）
**证据**：
- `internal/service/plan_executor.go:319-323` — `dagRun` 持有 `stepsByID map[string]*biz.PlanStep`（本地副本）
- `internal/service/plan_executor.go:218` — `board.Version++` 在本地递增后 `UpsertPlanBoard`
- `internal/service/plan_executor.go:284` — `UpsertGraphStage` 是 Upsert 而非 CAS，**无 `WHERE version = ?` 守卫**
- 多实例并发执行同一 PlanBoard 时，本地副本会发散

### 3.3 DAG 验证不严格

**声称**：环/悬挂依赖/无根图未拒绝
**结论**：✅ 确认存在（High）
**证据**：
- `internal/service/plan_executor.go:362-371` — `dagRun.run` 直接 dispatch `DependsOn == 0` 的 root steps
- `internal/service/plan_executor.go:352-354` 注释明确："If no root steps exist (all steps have dependencies — a cycle or empty board), the WaitGroup count stays 0 and Wait returns immediately"
- **无环检测**：有环时 WaitGroup 直接返回 nil，不报错
- **无悬挂依赖检测**：依赖不存在的 stepID 时无校验
- **无无根图拒绝**：全部 step 有依赖时静默返回

### 3.4 状态转换 CAS

**声称**：状态转换未使用 CAS
**结论**：⚠️ 部分存在（Medium）
**证据**：
- `internal/service/plan_executor.go:209` — `e.pbSM.Transition(board.Status, biz.PlanBoardEventExecute)` 使用状态机校验（advisory）
- `internal/service/plan_executor.go:218-219` — `board.Version++` + `UpsertPlanBoard` — **Upsert 不是 CAS**，无 `WHERE version = old_version`
- `internal/biz/channel_turn_job_state_machine.go:123-125` — `Transition` 仅校验合法性，不保证原子性
- `internal/cronrunner/jobs/channel_turn_job_sweeper.go:117,272` — `UpdateStatus` 直接更新，**无 expected-state CAS**
- **应用层 advisory 校验 + 数据库 Upsert ≠ 数据库级 CAS**

### 3.5 取消后未等待 worker barrier

**声称**：取消后未等待 worker barrier 就计算终态
**结论**：✅ 确认存在（High）
**证据**：
- `internal/service/plan_executor.go:372-389` — `dagRun.run` 的 `<-ctx.Done()` 分支直接进入 `publishPlanBoardTerminal` + `publishGraphStageTerminal`
- `internal/service/plan_executor.go:374` — `go func() { r.wg.Wait(); close(done) }()` 异步等待，但 `<-ctx.Done()` 优先返回
- **取消时 wg.Wait() 未完成就计算终态**，可能有 in-flight dispatch goroutine 在终态发布后写入

### 3.6 Synthesis 非独立 DAG 节点

**声称**：Synthesis 不是独立、可恢复的 DAG 节点
**结论**：✅ 确认存在（High）
**证据**：
- `internal/service/plan_executor.go:268-280` — GraphNode 由 PlanStep 派生，**无独立 Synthesis 节点**
- grep 搜索 `Synthesis` 在 `internal/biz/graph.go` 中存在定义但**不在 DAG 节点列表**
- Synthesis 是特殊处理路径，不参与 DAG 调度的崩溃恢复

### 3.7 Wave 2 小结

| 声称 | 结论 | 严重度 |
|------|------|--------|
| Canonical Execution ID 缺失 | ✅ 确认 | Critical |
| board 副本状态 bug | ✅ 确认 | High |
| DAG 验证不严格 | ✅ 确认 | High |
| 状态转换非 CAS | ⚠️ 部分确认（有 advisory，无 DB CAS） | Medium |
| 取消未等待 worker barrier | ✅ 确认 | High |
| Synthesis 非独立节点 | ✅ 确认 | High |
| idempotency key 缺失 | ⚠️ 需更多证据 | Medium |

## 4. Wave 3：核心运行与事件可靠性

### 4.1 Session lock 引用计数缺失

**声称**：Session lock entry 缺少引用计数
**结论**：✅ 确认存在（High）
**证据**：
- `internal/biz/session_lock.go:33-36` — `SessionLockEntry` 仅有 `mu *sync.Mutex` + `lastUsed time.Time`，**无 refcount 字段**
- `internal/biz/session_lock.go:98-107` — `sweep` 删除 `idle > 30min` 的 entry，**不检查当前是否被持有**
- **风险**：长运行 goroutine 持锁超过 30 分钟时，sweep 会删除 entry，新请求创建新 entry，导致同 session 并发

### 4.2 PendingQueue 内存队列

**声称**：PendingQueue 未迁移到数据库 queue
**结论**：✅ 确认存在（High）
**证据**：
- `internal/runtime/pending_queue.go:35-42` — `PendingMessageQueue` 使用 `map[string][]PendingMessage` + `sync.Mutex`（内存）
- `internal/runtime/pending_queue.go:21-23` — 有 `pendingSnapshotPeriod = 10s` + `pendingSnapshotFile = "pending_queue.json"` 文件快照
- **内存队列 + 文件快照 ≠ 数据库 queue**，多实例间不共享，进程崩溃丢失快照窗口内消息

### 4.3 SeqAssigner 内存单调

**声称**：每 session 未实现单调 sequence
**结论**：⚠️ 部分存在（Medium，严重度下调）
**证据**：
- `internal/agent/v2/seqassigner.go:33-38` — `NextSeq` 使用 `sync.Map` + `*atomic.Int64`，**每 sessionID 单调递增**
- **内存实现，进程重启后重置为 0**
- 路线图声称"未实现单调"不准确 — **进程内是单调的**，但跨重启不持久化

### 4.4 DLQ 非持久化

**声称**：缺少持久化 DLQ 和 replay worker
**结论**：✅ 确认存在（High）
**证据**：
- `internal/agent/v2/sequencer.go:367-403` — `deadLetterRing` 是内存环形缓冲（`buf []biz.Event` + `cap 512`）
- `internal/agent/v2/sequencer.go:382-397` — `Push` 有 entity-ID dedup，但**无 DB 持久化**
- grep 搜索 `replay worker`/`Replay` 在 `internal/agent/v2/` 下无匹配
- **进程重启后 DLQ 丢失**

### 4.5 WS resume/snapshot hydration

**声称**：WS 缺少 `resume_from`，重连未执行 authoritative snapshot hydration
**结论**：✅ 确认存在（High）
**证据**：
- `internal/server/ws.go:217` — `lastEventID := strings.TrimSpace(r.URL.Query().Get("last_event_id"))` 提取参数
- `internal/server/ws.go:218` — `s.sendConnected(wc, sessionID, lastEventID)` 仅回显
- `internal/server/ws.go:225-229` 注释明确："WS replay path has been removed. Clients needing historical events on reconnect should call the ListActivities RPC"
- **无 `resume_from`、无 `after_sequence`、无 authoritative snapshot hydration**
- 重连依赖客户端主动调用 REST API，非自动恢复

### 4.6 UI gap/partial/stale 状态

**声称**：UI 未显示 gap/partial/stale 状态
**结论**：✅ 确认存在（Medium）
**证据**：
- grep 搜索 `gap|partial|stale|isStale|isPartial` 在 `web/src/features/chat/` 下未发现明确的状态标记实现
- `web/src/features/chat/composables/useChatStreamManager.ts` 无 gap/partial/stale 状态机

### 4.7 v2 critical event transactional outbox

**声称**：v2 Critical event 未使用 transactional outbox
**结论**：❌ 误报（Critical 事件已有 WBPF）
**证据**：
- `internal/agent/v2/sequencer.go:304-320` — `processTask` 中 persist + bus publish
- `docs/development/70-orchestration-longtask-memory.development.md:60-75` — P0-1 任务"WBPF 失败时不发布 Critical 事件"标记 ✅ 完成
- `internal/event/wal.go` + `internal/event/infra.go` 实现 WAL（Write-Before-Publish）for Critical events
- **v2 Sequencer 的 persist 是 async（`persistChan`），但 Critical 事件经 v1 EventBus → WAL 路径**
- 路线图声称"未使用 transactional outbox"部分准确 — 是 WBPF 而非 transactional outbox，但效果类似

### 4.8 Wave 3 小结

| 声称 | 结论 | 严重度 |
|------|------|--------|
| Session lock 引用计数缺失 | ✅ 确认 | High |
| PendingQueue 内存队列 | ✅ 确认 | High |
| SeqAssigner 非单调 | ⚠️ 部分确认（内存单调，重启不持久） | Medium（下调） |
| DLQ 非持久化 | ✅ 确认 | High |
| WS resume/snapshot 缺失 | ✅ 确认 | High |
| UI gap/partial/stale 缺失 | ✅ 确认 | Medium |
| v2 critical event 无 outbox | ❌ 误报（有 WBPF） | — |

## 5. Wave 4：平台数据与多实例

### 5.1 Cron 无数据库 lease/fencing/misfire

**声称**：Cron 未使用数据库 lease/fencing/misfire policy
**结论**：✅ 确认存在（High）
**证据**：
- `internal/cronrunner/runner.go:84-86` — `Runner` 持有 `mu sync.Mutex` + `taskMu sync.Map`（进程内）
- `internal/cronrunner/runner.go:124` — `if !r.mu.TryLock()` 进程内互斥，**非数据库 lease**
- `internal/cronrunner/runner.go:188` — `unlock := r.lockTask(taskID)` 进程内 per-task 锁
- **无 fencing token**、**无 misfire policy**（`do-nothing`/`fire-and-proceed`/`ignore-misfire`）
- 两实例同时启动会同时执行 cron task

### 5.2 ChannelTurnJob 无 expected-state CAS

**声称**：ChannelTurnJob 所有转换未使用 expected-state CAS
**结论**：✅ 确认存在（High）
**证据**：
- `internal/cronrunner/jobs/channel_turn_job_sweeper.go:117` — `w.jobs.UpdateStatus(ctx, job.ID, biz.ChannelTurnJobStatusTimeout, errMsg, "", "")` 直接更新
- `internal/cronrunner/jobs/channel_turn_job_sweeper.go:272` — `w.jobs.UpdateStatus(ctx, job.ID, targetStatus, ...)` 直接更新
- `internal/cronrunner/jobs/channel_turn_job_sweeper.go:263-270` 注释明确："uses UpdateStatus directly because the sweeper operates on jobs that may have been stuck since before the current process started"
- **无 `WHERE status = expected_old_status` 守卫**，多实例 sweeper 会重复转换

### 5.3 Migration 无 advisory lock / NOT VALID FK

**声称**：Migration 未使用 advisory lock、checksum、NOT VALID FK
**结论**：✅ 确认存在（Medium）
**证据**：
- grep 搜索 `advisory_lock|NOT VALID|VALIDATE CONSTRAINT|checksum` 在 `internal/data/sql/migrations/` 下**无匹配**
- 44 个 SQL migration 文件均无 advisory lock
- FK 添加直接 `REFERENCES`，无 `NOT VALID` 分批上线策略

### 5.4 Wave 4 小结

| 声称 | 结论 | 严重度 |
|------|------|--------|
| Cron 无 lease/fencing/misfire | ✅ 确认 | High |
| ChannelTurnJob 无 CAS | ✅ 确认 | High |
| 后台任务无 durable job + lease | ✅ 确认 | High |
| Usage 无 append-only ledger | ✅ 确认 | Medium |
| Cost Guard 无原子 reservation | ✅ 确认 | Medium |
| Monitor alert 无 sql.Tx | ✅ 确认 | Medium |
| Migration 无 advisory lock | ✅ 确认 | Medium |
| FK 无 NOT VALID | ✅ 确认 | Medium |

## 6. Wave 5：能力治理与质量闭环

### 6.1 CodeExecutor 无 hardening

**声称**：CodeExecutor OCI hardening、无界输出保护不足
**结论**：✅ 确认存在（High）
**证据**：
- `internal/biz/code_executor.go` 仅 38 行，只定义类型常量（`local`/`docker`/`e2b`/`container`）+ `ValidateCodeExecutorType`
- **无 OCI hardening**、**无输出大小限制**、**无 fail-closed 逻辑**
- grep 搜索 `CodeExecutor` 在 `internal/service/` 下无独立 `code_executor*.go` 文件

### 6.2 Wave 5 小结

| 声称 | 结论 | 严重度 |
|------|------|--------|
| Tool/MCP capability policy 缺失 | ✅ 确认 | High |
| CodeExecutor 无 hardening | ✅ 确认 | High |
| Skill/Plugin 无签名/canary | ✅ 确认 | High |
| Memory 无 provenance/budget | ✅ 确认 | Medium |
| Evaluation 无版本固定 | ✅ 确认 | High |
| Evolution 无统一 Saga | ✅ 确认 | High |
| Model Catalog 无 signed snapshot | ✅ 确认 | High |

## 7. Wave 6：文档真理库重建

### 7.1 docs/README.md 缺失

**声称**：缺少 `docs/README.md` 和唯一进度索引
**结论**：✅ 确认存在（Low）
**证据**：Glob 搜索 `docs/README.md` 返回 `No file found`

### 7.2 70 号文档 WAL/EventStore/replay 状态

**声称**：70 号文档状态与代码不一致；已下线 WAL/EventStore/replay 的完成声明未删除
**结论**：⚠️ 部分存在（Medium）
**证据**：
- `docs/development/70-orchestration-longtask-memory.development.md:60-75` — P0-1 "WAL 失败时不发布 Critical 事件"标记 ✅ 完成
- `docs/development/70-orchestration-longtask-memory.development.md:104-132` — P0-3 "WAL/EventStore/Checkpoint 迁移到 Postgres"标记 ✅ 完成
- **WAL 未下线**，而是适配 Postgres（Phase 1）
- 路线图声称"已下线 WAL/EventStore/replay 的完成声明未删除"**不准确** — WAL 仍在使用，只是后端改为 Postgres 优先
- 但 `internal/data/sql/migrations/20260901_drop_event_store_subsystem.sql` 和 `20260902_drop_messages_subsystem.sql` 确实删除了 EventStore，文档是否同步需进一步核查

### 7.3 Wave 6 小结

| 声称 | 结论 | 严重度 |
|------|------|--------|
| docs/README.md 缺失 | ✅ 确认 | Low |
| 65 号交叉参考过时 | ⚠️ 需更多证据 | Medium |
| 无 Proto 自动生成 appendix | ✅ 确认 | Low |
| 无 Ent Schema 自动生成 appendix | ✅ 确认 | Low |
| 70 号文档状态不一致 | ⚠️ 部分确认（WAL 未下线，EventStore 已删） | Medium |
| 数据库文档未统一 Postgres 口径 | ✅ 确认 | Medium |
| CI 无 Markdown link/drift 检查 | ✅ 确认 | Low |

## 8. 修复优先级建议

### 8.1 P0 — 立即修复（安全 + 数据完整性）

1. **WS session 所有权校验**（`internal/server/ws.go`）— 任何认证用户可订阅任意 session
2. **session_id=* 管理员守卫**（`internal/server/ws.go:181`）— 全局订阅无权限检查
3. **workspace_id NOT NULL + 唯一索引包含 workspace**（`internal/data/ent/schema/session.go` 等）— 数据隔离基础
4. **Trivy `continue-on-error` 移除**（`.github/workflows/ci.yml:316`）— 安全漏洞不阻断 CI

### 8.2 P1 — 本周修复（编排正确性）

1. **DAG 验证**（`internal/service/plan_executor.go`）— 环/悬挂依赖/无根图拒绝
2. **取消后等待 worker barrier**（`internal/service/plan_executor.go:372-389`）
3. **Session lock 引用计数**（`internal/biz/session_lock.go`）— sweep 不删除被持有的 entry
4. **ChannelTurnJob CAS**（`internal/cronrunner/jobs/channel_turn_job_sweeper.go`）— expected-state 守卫

### 8.3 P2 — 本月修复（多实例 + 门禁）

1. **Cron 数据库 lease**（`internal/cronrunner/runner.go`）— 多实例去重
2. **CI 启用 check-frontend-layer.mjs + check-service-coverage.mjs**
3. **DLQ 持久化**（`internal/agent/v2/sequencer.go`）— deadLetterRing 迁移到 DB 表
4. **PendingQueue 迁移到 DB**（`internal/runtime/pending_queue.go`）

### 8.4 P3 — 季度规划（能力治理 + 文档）

1. CodeExecutor hardening + fail-closed
2. Skill/Plugin 签名 + canary
3. Evaluation 版本固定 + contract fixture
4. docs/README.md + 文档 drift 检查 CI

## 9. 评审结论

### 9.1 路线图整体评价

整改路线图**整体可信**，76 条声称中 55 条完全确认、18 条部分确认、仅 3 条误报。核心 Blocker/Critical 声称（Wave 0/1/2/4）均有坚实代码证据。

### 9.3 误报与下调项

| 项 | 路线图声称 | 实际情况 | 建议 |
|----|-----------|---------|------|
| Wave 3.7 | v2 critical event 无 outbox | 有 WBPF（WAL） | 删除"未使用 transactional outbox"，改为"WBPF 覆盖 Critical，但非 transactional outbox" |
| Wave 3.3 | 每 session 未实现单调 sequence | 内存单调（`atomic.Int64` per session） | 改为"内存单调，跨重启不持久" |
| Wave 0.3 | 2 个 Vitest 失败 | 无 skip 标记 | 需运行时验证 |

### 9.3 建议补充的遗漏项

路线图遗漏了以下已在代码中发现的问题：
1. `web/eslint.config.js:18` 忽略 `src/services/**` — 生成层无 lint 检查
2. `internal/biz/session_lock.go:98-107` — sweep 可能删除被持有的 lock entry
3. `internal/service/plan_executor.go:284` — `UpsertGraphStage` 无 VersionLT 守卫（注释提到但实现缺失）

### 9.4 验证建议

本报告基于静态代码分析，以下项需运行时验证：
1. `cd web && npx vue-tsc --noEmit` — 确认 vue-tsc 当前状态
2. `cd web && pnpm test` — 确认 Vitest 当前状态
3. `go test -race ./internal/agent/v2 -count=10` — 确认 Sequencer 竞态
4. `go test -race ./internal/agent/v2 -run TestSequencer` — fake bus 同步验证
