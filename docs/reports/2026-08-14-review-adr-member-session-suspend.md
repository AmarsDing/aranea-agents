# ADR-D: TeamRun 图执行挂起/唤醒语义（P3-3 动态激活）

## 状态：已接受（2026-08-14 调研修订后实施）

## 背景

P3-3 原始命题："长任务 Team 的成员 session 常驻成本高 → 成员 session 空闲超时自动挂起，新任务到来秒级恢复"。

### 初稿假设（已证伪）

初稿假设成员 agent 以常驻 goroutine/runner 形态存活于 TeamRun 期间，挂起=释放 runner 内存。深读代码后**证伪**：

1. **成员非常驻 goroutine**：TeamRunV2 成员是 trpc graph 节点，由 graph runtime 按 turn 驱动执行（[trpc_build.go](file:///f:/aranea-agents/internal/team/trpc_build.go) `BuildTeamMemberAgents`），turn 间无 per-member goroutine。
2. **agent 实例常驻是刻意设计**：`BuildCache` 为 Always-Ready LRU（[cache.go:30](file:///f:/aranea-agents/internal/agent/cache.go#L30)：无 TTL，注释明确"eliminates the 2-15s cold-start penalty"）。对它做"挂起释放"与本设计直接冲突。
3. **成员 token 成本已治理**：P2-4 project-state 有界注入（Batch-4）+ session 压缩（CompressRepo 版本 CAS）覆盖了长会话上下文增长问题。

### 前沿方案调研（2026-08-14）

| 来源 | 机制 | 可借鉴点 |
|------|------|---------|
| 阿里 ACS Agent Sandbox 深休眠 | `lifecycle.on_timeout: pause` 闲置超时自动休眠 + 快照保留现场 + `connect()` 秒级唤醒 + `reserve-paused-sandbox-duration` 保留期 TTL 超时删除；前置条件"无活跃请求和长连接" | **超时挂起 + 唤醒恢复 + 保留期 TTL** 三段式；挂起前置=无 inflight |
| Anthropic managed agents / 工业共识（Long-Running Agent Runtime） | "harness instances become disposable and restartable; durable state lives elsewhere"；五要素 Session/Harness/Sandbox/Checkpoint/Trace；Postgres checkpointer 为默认 | worker 可抛弃、状态外置——本项目 graph 状态本就在 DB/checkpoint，协调器会话可安全 evict 重建 |
| PRIMA 论文（arXiv 2605.24775） | typed pause record 落盘 + model-preserving resume + completed-step loader 拒绝污染输出 | 恢复必须保持原配置；已完成 step 不重复执行（本项目 graph checkpoint 已保证） |

### 真实成本定位

长任务 Team 的常驻成本不在成员 runner，而在**协调器图执行会话**：

- `TeamGraphRunCoordinator.sessions` map + 每会话 watch goroutine（[team_graph_run_coordinator.go:55](file:///f:/aranea-agents/internal/team/team_graph_run_coordinator.go#L55)）
- `waiting_human`（HITL 等待）会话**无限期常驻**：内存结构 + completion watch goroutine，直到 `CleanupStaleSessions` 按 `registeredAt` 超 `sessionMaxAge` 后被**误杀为 stale**（DB 行删除）
- 被 evict 的会话 DB 行即删，resume 信号到达时**无运行时唤醒路径**——`RecoverSessions` 仅在进程启动时调用一次（[chat_orchestrator.go:612](file:///f:/aranea-agents/internal/service/chat_orchestrator.go#L612)）

**缺口**：缺"挂起"中间态（内存释放、DB 保留、可唤醒），导致 HITL 长等待会话要么白占内存、要么被误杀丢恢复能力。

## 决策

### D1: 挂起 = 内存 evict + DB 保留（不新增状态枚举）

- `TeamGraphSession` 已有 `Status`（镜像 TeamRun 状态）+ `LastActivityAt` 字段，挂起**不新增状态值**：会话 status 保持 `waiting_human`，DB 行保留，仅从内存 evict（stopWatch + delete(sessions)）。
- 挂起判定：`status == waiting_human` 且 `now - LastActivityAt > idleThreshold`（默认 30min，config 可配）。
- 挂起前置：仅 `waiting_human` 可挂起（running 会话有 inflight step，对齐 ACS"无活跃请求才休眠"）。
- 实现位置：`TeamGraphRunCoordinator.SuspendIdleWaits(now)`，由 provider.go 既有 cleanup ticker 同循环驱动（不新增后台任务）。

### D2: 唤醒 = resume 信号到达时按需重建

- 新增 `ensureSessionResident(execID)`：内存 miss 时 `sessionRepo.GetSession` → 重建内存会话（复用 `RecoverSessions` 的 `buildResumeSessionContext` 逻辑，提取为 `restoreSession(dbSess)` 共享方法）→ 重启 completion watch。
- 接入点：`HandleTeamGraphTaskCompleted`（HITL resume 主入口）开头调用，内存命中零开销。
- 对齐 Anthropic 模式：协调器会话 disposable，DB 为真相源；唤醒延迟 = 一次 DB 读 + 轻量结构重建（无 agent 重建，BuildCache 热）。

### D3: 保留期 TTL 复用 CleanupStaleSessions，基准改为 LastActivityAt

- 现状 `CleanupStaleSessions` 按 `registeredAt` 判 stale——对挂起会话不公平（注册早但最近仍有活动的会被误杀）。
- 修订：stale 判定基准从 `registeredAt` 改为 `LastActivityAt`（回退 `registeredAt`），`sessionMaxAge` 语义即"保留期"（对齐 ACS `reserve-paused-sandbox-duration`）。超时清理 = 内存 evict + DB 删除（现状行为不变）。
- 效果：活跃/挂起会话都按"最后活动"计龄，挂起不再加速死亡。

### D4: 可观测（双轨制）

- 挂起：流程日志 `team.session.suspended`（info）+ 进程日志 Info（K7）。
- 唤醒：流程日志 `team.session.resumed`（info）+ 进程日志 Info（K7）。
- stale 清理已有 `team.session.stale_evicted` warn（现状保留）。
- step_id 登记 `stepTitleRegistry` + 同步 52-flow-logger.design.md §5.1。

### D5: 明确不做成员 runner 挂起

- 成员 graph 节点无常驻 goroutine，无挂起对象。
- BuildCache Always-Ready 是刻意的冷启动消除设计，不引入冲突的逐出策略。
- 成员级 token 成本由 P2-4 project-state + session 压缩覆盖，不在本 ADR 范围。

## 后果

### 正面

- HITL 长等待会话内存驻留时间从"直到 maxAge"降到"idleThreshold"（默认 30min），watch goroutine 同步释放。
- 挂起会话不再被 stale 清理误杀（D3），HITL 恢复能力从"maxAge 内有效"延长到"保留期内可唤醒"。
- 复用既有基础设施（persistSession/GetSession/buildResumeSessionContext/checkpoint），新增代码量小。
- 与 BuildCache Always-Ready、graph checkpoint、事件溯源零冲突。

### 负面

- 唤醒增加一次 DB 读（命中内存时零开销）；挂起后首次 resume 多一步会话重建（实测远低于 2s 目标，无 LLM 调用）。
- `CleanupStaleSessions` 判龄基准变更影响存量行为：长 running 会话按 LastActivityAt 计龄后**更不易**被误清（正向，但属行为变更需说明）。

### 不变量守护

- 挂起不改 TeamRun 状态机（waiting_human 状态不变），不写 member_sessions_v2，不触碰 version bands。
- 唤醒幂等：`ensureSessionResident` 内存命中直接返回；并发唤醒由 `c.mu` 保护单例重建。
- 进程重启行为不变：`RecoverSessions` 照常全量恢复 active 会话（挂起会话回内存后由 idle 检测自然再挂起）。

## 替代方案

### A1: 成员 runner 级挂起（初稿方案）

- 拒绝：成员无常驻 runner（graph 节点按 turn 执行），无挂起对象；且与 BuildCache Always-Ready 设计冲突。

### A2: 复用 durable run checkpoint（session_run_checkpoints）

- 拒绝：该表服务交互式单会话升级恢复（字段 SessionRunID/TurnID/AgentID），与图执行会话语义不兼容；`TeamGraphSession` 行本身已是充分的恢复锚点（含 DefinitionJSON/InputPreview/Status/LastActivityAt）。

### A3: waiting_human 会话永驻内存

- 拒绝：长等待（人工审批跨小时/天）场景内存与 goroutine 线性增长，无界。

## 验证

| 指标 | 验证方式 |
|------|---------|
| 挂起正确性 | 单测：waiting_human + LastActivityAt 超阈值 → 内存 evict + DB 保留 + watch 停止；running 会话不挂起 |
| 唤醒正确性 | 单测：内存 miss 时 HandleTeamGraphTaskCompleted 从 DB 重建会话并继续处理；行为与未挂起一致 |
| 保留期语义 | 单测：CleanupStaleSessions 按 LastActivityAt 判龄；挂起会话在保留期内不被清理 |
| 幂等 | 单测：并发 ensureSessionResident 只重建一次；重复挂起无副作用 |
| 恢复延迟 | 基准：唤醒路径无 LLM 调用，DB 读 + 结构重建 ≪ 2s |

## 相关文件

| 改动 | 文件 | 说明 |
|------|------|------|
| 修改 | `internal/team/team_graph_run_coordinator.go` | SuspendIdleWaits + ensureSessionResident + restoreSession 提取 + CleanupStaleSessions 判龄基准改 LastActivityAt |
| 修改 | `internal/team/provider.go` | cleanup ticker 同循环驱动挂起扫描 |
| 修改 | `internal/team/team_graph_run_coordinator_test.go` | TDD 用例 |
| 修改 | `internal/event/flow_log.go` | stepTitleRegistry 登记 team.session.suspended/resumed |
| 修改 | `docs/development/52-flow-logger.design.md` | §5.1 步骤注册表同步 |

## 批次

Batch-6（P3-3 + P3-4），与评测态 profile（P3-4）同批实施。
