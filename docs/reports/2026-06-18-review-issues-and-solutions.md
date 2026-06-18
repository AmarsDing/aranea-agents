# 残留问题与根本性解决方案

> 生成日期：2026-06-18
> 报告类型：问题与解决方案合并报告
> 关联文档：
> - `docs/reports/2026-06-17-research-orchestration-longtask-memory-upgrade.md`（综合升级调研）
> - `docs/development/70-orchestration-longtask-memory.development.md`（开发计划，30/30 已完成）
>
> **范围说明**：本报告排除数据库迁移（Phase A Postgres 全量迁移）相关内容，聚焦于综合升级方案未覆盖或未完全解决的 20 个问题及其根本性解决方案。

---

## 一、概要

| 维度 | 数量 |
|------|------|
| 问题总数 | 20（🔴 阻断 4 + 🟡 建议 16） |
| 方案族 | 4（A 生命周期 / B 前端收敛 / C 记忆生产化 / D LLM+测试） |
| 总工作量 | 63 人天 |
| 真实次生风险 | 14 个（均有缓解措施） |

**问题分布**：

| 端 | 数量 |
|----|------|
| 后端 | 8 |
| 前端 | 9 |
| 记忆系统 | 3 |

---

## 二、方案 A：统一生命周期管理框架

> 覆盖 6 个问题，工作量 18 人天。核心思路：引入 LifecycleManager + GoroutinePool + ManagedMap + DeadLetterQueue 统一管理后端资源生命周期。

### A1. RunRegistry 多 sync.Map 非原子操作（TOCTOU 风险）

| 项 | 内容 |
|----|------|
| **严重度** | 🔴 阻断 |
| **位置** | `internal/runtime/run_registry.go:129-141` `StoreRunner` |
| **问题** | `StoreRunner` 先 `load` 再 `store`，存在 TOCTOU 风险。两个并发 goroutine 可能同时 load 到旧值，然后各自 store，导致 cancel 函数丢失或 runner 引用错乱。不同入口（Chat/Channel/Cron/A2A）可能绕过同一 session lock。 |
| **根本性方案** | 用 ManagedMap（内置锁 + 原子操作）替代裸 `sync.Map`，`LoadOrStore` 替代 `load+store` |
| **优点** | 原子操作根治 TOCTOU；ManagedMap 可复用于其他场景 |
| **缺点** | 引入新抽象，需迁移现有调用点 |
| **工作量** | 1 人天 |

### A2. goroutine 生命周期管理不完整

| 项 | 内容 |
|----|------|
| **严重度** | 🔴 阻断 |
| **位置** | `internal/server/ws_message_handler.go:129,169`、`internal/service/chat_orchestrator_turn_dispatch.go:163`、`internal/biz/graph_execution_usecase.go:229,272,379` |
| **问题** | WS `handleUserMessage` 使用 `appctx.Ctx()` 启动 goroutine（注：appctx.Ctx() 是进程级生命周期 ctx，非全局 background，会在 shutdown 时取消）。WS 场景下 goroutine 本身不随连接关闭退出（虽然内部 turn ctx 会取消）。`processPendingQueue` 和 `graph.consumeEvents` 必须使用 appctx.Ctx()（跨请求场景），不应改动。 |
| **根本性方案** | 引入 GoroutinePool 支持多模式：<br>- `Go(ctx, ...)` 用于请求级（WS 场景改用 connCtx）<br>- `GoBackground(name, ...)` 用于进程级（内部用 appctx.Ctx()，保留 processPendingQueue/graph.consumeEvents 语义）<br>- 保留 SessionRunDurableWorker 的 context.Background() 语义 |
| **优点** | 强制 ctx 传播根治泄漏；多模式兼容跨请求场景 |
| **缺点** | 132 处 safego.Go 调用点需迁移（3-5 人天）；大规模改造可能引入新并发 bug |
| **风险** | 强制统一 ctx 可能破坏跨请求执行（processPendingQueue、graph.consumeEvents）→ 多模式设计缓解 |
| **工作量** | 4 人天（含 GoroutinePool 实现 + WS 场景迁移） |

### A3. globalBuildCache.Close() 未接入生命周期

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议 |
| **位置** | `internal/agent/cache.go:58, 155` |
| **问题** | `globalBuildCache` 是进程级单例，`BuildCache.Close()` 方法存在但全代码库无调用点。缓存项无 TTL，仅靠 dirty 标记 + Invalidate 释放。24h 运行可能积累失效 Agent 缓存项。 |
| **根本性方案** | 将 globalBuildCache 注册到 LifecycleManager，应用关闭时调用 Close；或改为构造注入随 Data 生命周期管理 |
| **优点** | 资源释放有明确路径 |
| **工作量** | 1 人天 |

### A4. pending queue 无 dead-letter 机制

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议 |
| **位置** | `internal/service/chat_orchestrator_turn_dispatch.go:200-204` |
| **问题** | pending 消息处理失败只 `publishTurnFailure`，消息已从队列 dequeue，可能丢失。排队消息失败后无法重试或人工干预。 |
| **根本性方案** | 引入 DeadLetterQueue：失败消息入死信队列（内存缓冲 + DB 持久化），提供管理 API 查询/重试/丢弃，限制队列大小避免无限增长 |
| **优点** | 失败消息可追溯可重试 |
| **缺点** | 死信机制需与方案 C 的 DeadLetterRepo 统一设计，避免两套并存 |
| **工作量** | 2 人天 |

### A5. RuntimeReplanner attemptCount map 无清理机制

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议 |
| **位置** | `internal/graph/runtime_replanner.go:70-92` |
| **问题** | `attemptCount map[string]int` 按 execution ID 跟踪重规划次数，永不清理。每个 execution 完成后留下 map 条目（约 50 字节），长生命周期进程内存缓慢增长。 |
| **根本性方案** | 用 ManagedMap 替代裸 map，TTL > HITLSLATimeout（24h）或改为"终态清理"（execution 完成时删除 entry） |
| **优点** | 自动清理，无内存泄漏 |
| **风险** | TTL 清理可能与 waiting_human 状态（持续 24h）冲突 → TTL > 24h 或终态清理缓解 |
| **工作量** | 0.5 人天 |

### A6. TopologyEvolver addedEdges map 无清理机制

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议 |
| **位置** | `internal/graph/topology_evolution.go` |
| **问题** | `addedEdges map[execID]map[edgeKey]bool` 无清理机制，与 A5 相同模式。 |
| **根本性方案** | 同 A5，用 ManagedMap 替代 |
| **工作量** | 0.5 人天 |

### 方案 A 架构设计

```
internal/runtime/lifecycle/
├── manager.go          # LifecycleManager（统一注册/销毁）
├── goroutine_pool.go   # GoroutinePool（Go + GoBackground 多模式）
├── managed_map.go      # ManagedMap（带 TTL 或终态清理）
├── managed_cache.go    # ManagedCache（带 TTL 的 cache）
└── dead_letter.go      # DeadLetterQueue（内存缓冲 + DB 持久化）
```

### 方案 A 客观评估

| 维度 | 评估 |
|------|------|
| **优点** | 统一抽象根治 6 个问题；框架可复用于未来新增资源；强制 ctx 传播根治泄漏 |
| **缺点** | 引入新抽象增加学习成本；改造范围广（132 处 safego.Go）；ManagedMap TTL 对高频路径有性能开销 |
| **关键风险** | GoroutinePool 强制 ctx 破坏跨请求执行（🔴 高）→ 多模式设计缓解；132 处迁移引入新并发 bug（🟡 中）→ 分批迁移 + race detector |
| **工作量** | 18 人天（含框架实现 + 132 处迁移 + 多模式设计） |

---

## 三、方案 B：前端架构收敛与体验统一

> 覆盖 9 个问题，工作量 25 人天。核心思路：AF 路径收敛 + ConnectionManager 统一连接 + 分层超时模型 + VirtualScroller + ThemeManager。

### B1. AF 与 Legacy 双路径技术债

| 项 | 内容 |
|----|------|
| **严重度** | 🔴 阻断 |
| **位置** | `web/src/features/chat/composables/useConversationTimeline.ts:94-131`、`web/src/features/chat/mergeSessionMessages.ts:210-213`、`web/src/features/chat/sessionCompletionReload.ts` |
| **问题** | AF（Activity-First）与 Legacy 路径并存，根据 `activityFirst` 标志切换。两条路径行为不一致，bug 修复易遗漏其中一条。 |
| **根本性方案** | 完成 AF 迁移，移除 Legacy 路径：移除 `findUserTurns` 等 Legacy 函数，移除 `activityFirst` 标志切换逻辑 |
| **优点** | 架构收敛降低维护成本；单一真相源 |
| **缺点** | 改造范围最大；AF 可能未覆盖所有 Legacy 场景 |
| **风险** | 移除 Legacy 后部分场景消息无法显示（🔴 高）→ 灰度发布 + 监控 Legacy fallback 触发次数 |
| **工作量** | 5 人天 |

### B2. HTTP/WS 双通道职责不清导致消息管理混乱

| 项 | 内容 |
|----|------|
| **严重度** | 🔴 阻断 |
| **位置** | `web/src/features/chat/composables/useChatSender.ts:552-585`、后端 HTTP 消息接口 |
| **问题** | 当前 HTTP 和 WS 都承担数据传输职责：WS 失败回退 HTTP 时，HTTP 响应承担流式推送，但 HTTP 响应不经过 streamHandlers，pending-user 占位消息无法正确转换。双通道数据流导致竞态、重复、占位残留等问题。当前方案"HTTP 成功后触发 hydrate"只是补丁，未解决双通道数据流的根本矛盾。 |
| **根本性方案** | **通道职责分离架构**：HTTP 仅作为命令通道（提交消息，返回 ACK），WS 作为唯一数据通道（所有状态/消息/流式推送）。详见方案 B 架构设计。 |
| **优点** | 数据流单一无竞态；pending-user 转换由 WS 自动驱动；ConnectionManager 简化；WS 断连后事件回放保证不丢 |
| **缺点** | HTTP 接口语义破坏性变更；WS 断连期间需 UI 反馈 |
| **风险** | 接口变更需灰度（🟡 中）→ 版本化 + 双写期；WS 断连无实时反馈（🟡 中）→ UI 提示"消息已排队" + 事件回放 |
| **工作量** | 7 人天 |

### B3. 前端 7 个超时模型复杂

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议 |
| **位置** | `web/src/features/constants/timeouts.ts` |
| **问题** | 7 个独立超时：dispatch(30s) / turn-ack(30s) / first-byte(90s) / stall(180s) / run-stale(30s) / heartbeat(25s) / stream(10min)。交互关系不清晰，可能相互干扰。 |
| **根本性方案** | 统一为分层超时模型：连接级（heartbeat + run-stale）/ 请求级（dispatch + turn-ack）/ 响应级（first-byte + stall）/ 会话级（stream），文档化各层级触发条件与取消关系 |
| **优点** | 超时可调优；层级清晰 |
| **工作量** | 2 人天 |

### B4. 30s turn-ack 超时对慢模型过于激进

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议 |
| **位置** | `web/src/features/constants/timeouts.ts` `CHAT_TURN_ACK_TIMEOUT_MS = 30_000` |
| **问题** | 复杂 agent（多工具调用、长思考）可能 30s 内未发出 `run_status=running`，触发误报超时。用户看到错误但任务实际在跑。 |
| **根本性方案** | 动态 turn-ack：后端在 ack 中返回 `task_type`，前端按 task_type 选择超时（simple 30s / moderate 60s / complex 120s / unknown 90s 兜底） |
| **优点** | 慢模型兼容 |
| **风险** | 依赖后端 complexity 判断（🟢 低）→ 默认 moderate 60s + unknown 90s 兜底 |
| **工作量** | 1 人天 |

### B5. WS 重连 10 次后无自动恢复

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议 |
| **位置** | `web/src/realtime/ws-transport.ts:149-161` `WS_MAX_RECONNECT_ATTEMPTS = 10` |
| **问题** | 达到 10 次重连上限后放弃，需用户手动操作。网络抖动场景下用户体验差。 |
| **根本性方案** | 引入 ReconnectStrategy：10 次快速重连后切换慢速模式（每 5 分钟一次），显示"连接中断，正在尝试恢复"提示，用户手动操作可立即重连 |
| **优点** | 网络抖动自动恢复 |
| **工作量** | 1 人天 |

### B6. 消息列表无虚拟滚动

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议 |
| **位置** | `web/src/components/chat/ChatMessageList.vue:29-48` |
| **问题** | 长会话（1000+ 消息）下全量渲染导致性能瓶颈。滚动卡顿，内存占用高。 |
| **根本性方案** | 引入 VirtualScroller（使用 `vue-virtual-scroller` 的 `DynamicScroller` 支持动态高度），仅渲染可视区域 + 上下缓冲区，新消息到达时 `scrollIntoView({ behavior: 'smooth' })` |
| **优点** | 长会话性能提升 |
| **风险** | 动态高度消息（代码块/图片/折叠）位置计算错误（🟡 中）→ DynamicScroller + 展开/折叠时手动触发高度重算 |
| **工作量** | 4 人天 |

### B7. 主题无"跟随系统"选项

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议 |
| **位置** | `web/src/` 主题切换相关组件 |
| **问题** | 用户需手动切换昼/夜模式，无法根据系统偏好自动切换。 |
| **根本性方案** | 引入 ThemeManager：新增 `auto` 模式，监听 `prefers-color-scheme` 媒体查询，系统切换时自动应用，用户手动切换后覆盖自动模式 |
| **优点** | 体验提升 |
| **工作量** | 1 人天 |

### B8. ChatPage.vue 既有 FD2 违规

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议 |
| **位置** | `web/src/pages/ChatPage.vue:242` |
| **问题** | Page 直接 import api（FD2 违规），非本次方案引入，但未修复。 |
| **根本性方案** | 迁移到 Store + composable 模式 |
| **工作量** | 1 人天 |

### B9. i18n 覆盖率 100% 但 458 文件技术债务

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议 |
| **位置** | `web/scripts/i18n-baseline.json`（458 个文件基线） |
| **问题** | P3-6 已完成 CI 检查脚本 + baseline 增量比对，但 458 个既有文件仍有硬编码中文（已纳入 baseline，新增违规才失败）。 |
| **根本性方案** | 按模块优先级逐步迁移（高可见 UI 优先），每个迭代清理 10-20 个文件，长期目标 baseline 清零 |
| **优点** | 国际化完整覆盖 |
| **缺点** | 无法一次性完成，需长期投入 |
| **工作量** | 3 人天（首批 50 文件）+ 持续 |

### 方案 B 架构设计

#### 核心设计：通道职责分离（Command/Data Channel Separation）

**设计原则**：HTTP 只做"命令提交"（fire-and-ack），WS 接管所有"状态与数据"推送。从根本上消除双通道数据流竞态。

```
前端：
┌─────────────────────────────────────────────────┐
│  useChatSender                                  │
│  ├─ sendViaWS(message) → WS 提交 + 等待 ack     │
│  └─ sendViaHTTP(message) → 仅提交，返回 ACK     │
│      └─ 返回 {messageId, turnId, status:"queued"}│
│      └─ 不接收任何流式数据                       │
│                                                  │
│  所有消息/状态/流式 → WS 数据通道推送            │
│  WS 断连 → 事件持久化 → 重连后 afterRevision 回放│
└─────────────────────────────────────────────────┘

后端：
┌─────────────────────────────────────────────────┐
│  HTTP /messages（命令通道）                     │
│  └─ 仅入队 + 返回 {messageId, turnId}           │
│      不启动 turn，不返回流                       │
│                                                  │
│  WS handleUserMessage（命令通道 + 数据通道）     │
│  └─ 入队 + 启动 turn + 流式推送                 │
│                                                  │
│  Turn Orchestrator                              │
│  └─ 无论消息来自 HTTP 还是 WS，                 │
│      turn 执行结果都通过 WS 推送                │
│  └─ WS 未连接时，事件持久化到 EventStore        │
└─────────────────────────────────────────────────┘
```

**关键设计点**：

1. **HTTP 接口语义改变**：
   - 当前：POST /messages → 启动 turn + 返回流（SSE/chunked）
   - 新：POST /messages → 仅入队 + 返回 `{messageId, turnId, status: "queued"}`
   - HTTP 不启动 turn，不返回流

2. **WS 作为唯一数据通道**：
   - 用户消息入队后，由 WS 消费者启动 turn
   - turn 的所有事件（run_status、text_delta、tool_result、error、message.persisted）都通过 WS 推送
   - pending-user 占位消息的转换由 WS 推送的 `message.persisted` 事件自动驱动，无需手动 hydrate

3. **统一入队机制**：
   - HTTP 和 WS 都把消息放入同一个 pending queue
   - queue 消费者启动 turn，turn 结果通过 WS 推送
   - 彻底消除"HTTP 路径 vs WS 路径"的分支

4. **WS 断连处理**：
   - WS 断连时，turn 继续执行（durable 模式）
   - 事件持久化到 EventStore
   - 客户端重连后，发送 `sync_request { afterRevision }`，服务端回放缺失事件
   - UI 显示"连接中断，消息已排队，正在恢复..."

5. **HTTP 的唯一职责**：
   - 降级通道：当 WS 不可用时，用户仍能发送消息
   - 只做"提交"动作，不承担数据传输
   - 返回简单 ACK，告知消息已入队

**与补丁方案的对比**：

| 维度 | 补丁方案（hydrate） | 根本性方案（通道分离） |
|------|-------------------|---------------------|
| HTTP 职责 | 发送 + 返回流 + fallback hydrate | 仅发送，返回 ACK |
| 数据通道 | HTTP 和 WS 都可能返回数据 | 只有 WS |
| pending-user 转换 | HTTP 成功后手动 hydrate | WS 推送 message.persisted 自动转换 |
| 竞态风险 | HTTP hydrate 与 WS 推送竞态 | 无（数据流单一） |
| ConnectionManager 复杂度 | 高（需协调两通道） | 低（HTTP 仅 fire-and-ack） |
| WS 断连恢复 | 依赖 hydrate | afterRevision 事件回放 |

#### 文件结构

```
web/src/realtime/
├── command_channel.ts       # HTTP 命令通道（仅发送，返回 ACK）
├── data_channel.ts          # WS 数据通道（唯一数据流）
├── event_replay.ts          # 事件回放（afterRevision 增量同步）
├── timeout_model.ts         # 分层超时模型
└── reconnect_strategy.ts    # 重连策略（快速 + 慢速）

web/src/features/chat/
├── conversation_timeline_af.ts  # AF 路径（唯一真相源）
└── legacy/                      # 待删除的 Legacy 路径

web/src/components/common/
├── VirtualScroller.vue      # 虚拟滚动组件
└── ThemeManager.ts          # 主题管理（含 auto 模式）
```

### 方案 B 客观评估

| 维度 | 评估 |
|------|------|
| **优点** | 架构收敛降低维护成本；通道分离根治双通道竞态；超时可调优；性能提升 |
| **缺点** | 改造范围最大（9 个问题）；AF 迁移风险高；HTTP 接口语义破坏性变更；VirtualScroller 与动态高度兼容复杂 |
| **关键风险** | AF 收敛遗漏 Legacy 场景（🔴 高）→ 灰度发布 + 监控；HTTP 接口变更兼容性（🟡 中）→ 版本化 + 双写期；VirtualScroller 动态高度（🟡 中）→ DynamicScroller；WS 断连无实时反馈（🟡 中）→ UI 提示 + 事件回放 |
| **工作量** | 29 人天（含通道分离架构 7 人天） |

---

## 四、方案 C：记忆系统持久化与可靠性

> 覆盖 3 个问题，工作量 9 人天。核心思路：Schema 补全 + 统一 Job 框架 + 事务保证。

### C1. Ebbinghaus 衰减评分 — cron job 不读写 DB

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议（待生产化） |
| **位置** | `internal/cronrunner/jobs/memory_ebbinghaus_decay.go`、`internal/data/ent/schema/memory.go` |
| **问题** | 核心算法已实现（`EbbinghausDecayCalculator.ComputeDecay` + `FuseWithScore`），但 Schema 未新增 `access_count`/`last_accessed_at`/`decay_score` 列，cron job 不绑定数据库仅作为骨架启动，Decay 值未自动从 DB 读取。衰减评分无法自动反映记忆访问频次变化。 |
| **根本性方案** | 1. Schema 新增 3 列 + DDL migration + Ent 重新生成<br>2. cron job 从 DB 读取记忆列表，用单条 `UPDATE ... WHERE agent_id = ?` 批量计算 Decay 并回写（参考现有 L3 Decay 实现）<br>3. `RecallFactsFused` 自动从 DB 读取 Decay 值 |
| **优点** | 衰减评分持久化；自动反映访问频次 |
| **风险** | 批量更新锁表影响在线检索（🟡 中）→ 单条 UPDATE 批量 + 命中索引 + 每小时一次避开高峰 |
| **工作量** | 2 人天 |

### C2. Sleep-time Agent 异步整理 — 失败 job 无重试无死信

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议（待生产化） |
| **位置** | `internal/cronrunner/jobs/memory_sleep_time.go`、`internal/memory/sleep_time.go` |
| **问题** | `SleepTimeService` 三阶段已实现，但失败 job 无重试无死信（对比 `AutoMemoryWorker.processWithRetry`）。LLM 调用失败时整理任务直接丢弃，重要记忆合并可能遗漏。`AgentUserKeyLister` 从环境变量读取，非生产级。 |
| **根本性方案** | 1. 引入统一 JobFramework（重试上限 3 次 + 指数退避 1s/2s/4s + 熔断器连续 N 次失败暂停 5 分钟）<br>2. 失败 job 入死信队列（与方案 A 的 DeadLetterQueue 统一设计）<br>3. `AgentUserKeyLister` 改为从 SessionRepo 派生活跃用户（最近 N 天有活动） |
| **优点** | 生产级可靠性；框架可复用 |
| **风险** | 重试风暴加剧 LLM 服务故障（🟡 中）→ 熔断器 + 指数退避 + Sleep-time 低频（每小时一次） |
| **工作量** | 2 人天 |

### C3. 记忆链接图 Evolution — applyLinks 无事务包裹

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议（待生产化） |
| **位置** | `internal/memory/link_evolution.go` `applyLinks` 方法 |
| **问题** | `applyLinks` 中多次 `UpsertFactRow` 调用无事务包裹（best-effort 设计）。部分更新可能导致链接单向（新记忆有 Links，历史记忆无反向链接）。 |
| **根本性方案** | 用 `Data.ExecInTx` 包裹 `applyLinks` 的多次 `UpsertFactRow` 调用，失败时回滚保证原子性 |
| **优点** | 原子性保证；避免单向链接 |
| **风险** | 事务超时（❌ 不成立，验证发现最多 20 次 upsert 约 200ms，远低于 30s） |
| **工作量** | 0.5 人天 |

### 方案 C 架构设计

```
internal/cronrunner/
├── job_framework.go        # 统一 Job 框架（重试 + 熔断 + 死信）
├── jobs/
│   ├── memory_ebbinghaus_decay.go  # 增强：DB 读写
│   ├── memory_sleep_time.go        # 增强：重试 + 死信
│   └── memory_link_evolution.go    # 增强：事务包裹
└── dead_letter_repo.go     # 死信持久化（与方案 A 统一）

internal/data/ent/schema/
└── memory.go               # 增强：access_count/last_accessed_at/decay_score
```

### 方案 C 客观评估

| 维度 | 评估 |
|------|------|
| **优点** | 生产级可靠性；Schema 完整；JobFramework 可复用；与现有架构一致 |
| **缺点** | Schema 变更需迁移；JobFramework 可能与 AutoMemoryWorker 重复 |
| **关键风险** | 重试风暴（🟡 中）→ 熔断器；批量更新锁表（🟡 中）→ 单条 UPDATE；死信机制与方案 A 重复（🟡 中）→ 统一设计 |
| **工作量** | 9 人天（含 JobFramework + Schema + 熔断器 + 死信统一） |

---

## 五、方案 D：LLM 调用策略与测试治理

> 覆盖 2 个问题，工作量 7 人天。核心思路：动态超时配置 + 测试分层。

### D1. LLM HTTP 30min 超时仍是上限

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议（部分解决） |
| **位置** | `cmd/admin/wire.go:747` LLM HTTP 超时配置 |
| **问题** | P0-3 已将 5min → 30min，但 30min 仍是硬上限。深度推理任务（长链代码生成、复杂 Graph 编排）可能超时。24h 长任务期间单次 LLM 调用超过 30 分钟会被切断。 |
| **根本性方案** | 引入 TimeoutPolicy 按任务类型动态调整：普通对话 30min / 深度推理 60-120min / Graph 节点 60min / 代码生成 90min，通过 RunOption 传递 TaskType，配置驱动无需改代码 |
| **优点** | 灵活超时；配置驱动 |
| **风险** | 配置错误导致资源长时间占用（🟡 中）→ 范围校验（max 120min）+ 全局硬上限 + P99 监控告警 |
| **工作量** | 2.5 人天 |

### D2. 既有测试失败（非本次引入）

| 项 | 内容 |
|----|------|
| **严重度** | 🟡 建议 |
| **位置** | `internal/agent` `TestErrL1BudgetOverflow`、`TestAccumulateStreamUsage_multiLLMRounds`；`internal/biz/tool` `TestValidateMCPConfigURLs`（MCP SSRF DNS 环境问题） |
| **问题** | 既有测试失败，非综合升级方案引入，但影响 CI 绿灯。 |
| **根本性方案** | 1. 测试分层：unit test（必须通过）+ integration test（需环境，可 skip）<br>2. 修复 `TestErrL1BudgetOverflow`/`TestAccumulateStreamUsage_multiLLMRounds` 失败原因<br>3. MCP SSRF DNS 测试改为 mock 或标记 integration test<br>4. 每日定时跑 integration test + PR 合并前跑相关模块 |
| **优点** | CI 绿灯有意义；集成问题早发现 |
| **风险** | Integration test 长期 skip 导致集成问题积累（🟡 中）→ 每日定时跑 + 失败告警 |
| **工作量** | 1.5 人天 |

### 方案 D 架构设计

```
internal/provider/
└── timeout_policy.go       # TimeoutPolicy（按 TaskType 动态超时）

Makefile:
├── make test              # 仅 unit test
├── make test-integration  # 含 integration test
└── make test-all          # 全部
```

### 方案 D 客观评估

| 维度 | 评估 |
|------|------|
| **优点** | 灵活超时；测试可信度提升 |
| **缺点** | TaskType 分类需维护；测试分层需改造 |
| **工作量** | 7 人天 |

---

## 六、方案间协调

| 协调点 | 涉及方案 | 协调方式 |
|--------|---------|---------|
| 死信机制统一 | A + C | DeadLetterQueue（内存缓冲）+ DeadLetterRepo（DB 持久化）统一设计，避免两套并存 |
| complexity 判断 | B + D | 后端在 ack 中返回 `task_type`，前端根据 task_type 选择 turn-ack 超时，统一判断入口 |
| WS 协议兼容 | A + B | 后端 goroutine 生命周期变化不影响 envelope 格式和推送顺序，WS 协议向后兼容 |
| Schema 变更顺序 | C + Postgres | 方案 C 的 Schema 变更先于 Postgres 迁移完成，用 DDL Migration Registry 管理 |

---

## 七、实施建议

### 7.1 实施顺序

```
阶段 1（P0，立即）：4 个阻断项
  ├─ A1 RunRegistry TOCTOU（1 人天）
  ├─ A2 goroutine 生命周期（4 人天）
  ├─ B1 AF/Legacy 双路径（5 人天）
  └─ B2 通道职责分离架构（7 人天）
  → 17 人天

阶段 2（P1，本迭代）：记忆生产化 + LLM 策略
  ├─ C1 Ebbinghaus DB 列（2 人天）
  ├─ C2 Sleep-time 重试（2 人天）
  ├─ C3 记忆链接事务（0.5 人天）
  ├─ D1 LLM 动态超时（2.5 人天）
  └─ D2 测试分层（1.5 人天）
  → 8.5 人天

阶段 3（P2，下迭代）：生命周期框架 + 前端收敛
  ├─ A3-A6 生命周期剩余（4 人天）
  ├─ B3-B7 前端体验（9 人天）
  └─ B8-B9 技术债务（4 人天）
  → 17 人天

阶段 4（P3，机会性）：i18n baseline 持续清理
  → 持续
```

### 7.2 系统级风险防范

| 风险 | 缓解措施 |
|------|---------|
| 改造期间稳定性下降 | 分阶段实施 + 每阶段独立验证 + 灰度发布 + 改造期间冻结新功能 |
| 回归测试覆盖不足 | 改造前建立性能基准 + 改造后对比 + 关键路径 E2E 测试 + `goleak` 检测 |
| 文档同步滞后 | 每个 PR 必须同步文档（DOC-SYNC-1）+ 代码审查维度 12 必查 |

### 7.3 工作量汇总

| 方案 | 工作量 | 覆盖问题 |
|------|--------|---------|
| 方案 A（生命周期） | 18 人天 | 6 |
| 方案 B（前端收敛） | 29 人天 | 9 |
| 方案 C（记忆生产化） | 9 人天 | 3 |
| 方案 D（LLM+测试） | 7 人天 | 2 |
| **合计** | **63 人天** | **20** |

---

## 八、结论

综合升级方案（`70-orchestration-longtask-memory`）已完成 30/30 验收项，但全栈代码审查发现 **20 个问题**未被覆盖或未完全解决。本报告提出 **4 个根本性方案族**（统一生命周期管理 + 前端架构收敛 + 记忆系统生产化 + LLM 调用策略），总工作量 **63 人天**。

**核心评判**：
- 方案从架构层面消除问题根源，非临时补丁
- 14 个真实次生风险均有明确缓解措施
- 风险收益比可接受：解决 20 个问题的收益 > 可缓解风险的代价

**建议路径**：
1. **立即修复 4 个阻断项**（13 人天）— 影响并发安全与资源管理
2. **本迭代生产化记忆系统 + LLM 策略**（8.5 人天）— 完成领先记忆系统目标
3. **下迭代完成生命周期框架 + 前端收敛**（17 人天）— 达到生产级 24h 可靠性
4. **机会性清理技术债务**（i18n baseline 持续）
