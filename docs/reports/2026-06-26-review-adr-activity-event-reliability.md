# ADR-04: Activity 事件可靠性分级 — 将 Critical 并入 Important

## 状态：已接受

## 背景

AS-EVT-01 原定义三级事件可靠性：

| 级别 | 事件类型 | 可靠性保证 |
|------|---------|-----------|
| Critical | ToolResult / Error / RunnerCompletion / Checkpoint | WBPF（先写后发）+ 重试 |
| Important | StateDelta / TokenUsage / RunStatus / ... | BlockUpTo + 异步持久化 |
| Informational | TextDelta / FlowLog / ... | 尽力而为 |

Chat 模块重构（[2026-06-25-analysis-chat-module-refactor.md](../reports/2026-06-25-analysis-chat-module-refactor.md)）将三套并行体系（Activity / Envelope / Message）合并为单一 Activity 模型，原 Critical 事件被映射为 `ActivityEvent{event=completed/failed}`。

实现位于 [activity_event_sequencer.go:311](../../internal/agent/activity_event_sequencer.go#L311) `processTask`，采用 **Phase 1b parallel-async 设计**：

- **Persist**：异步入 `persistChan`（容量 256），由单一 persist worker 顺序处理；通道满时降级同步 persist
- **Publish**：同步 `eventBus.Publish`，保证 WS 推送低延迟
- **失败补偿**：`persistWithRetry` 重试 5 次（100/200/400/800/1600ms 指数退避），耗尽后入 `deadLetter` 环形缓冲（容量 512），WS 重连时通过 `ListDeadLetterActivities` RPC replay 合并

这与 AS-EVT-01 对 Critical 事件要求的 **WBPF（先写后发）** 不一致：当前是 publish-forth-then-async-persist。

## 决策

**将原 Critical 级别并入 Important，采用 2 级分级**：

| 级别 | 事件类型 | 可靠性保证 | 持久化 |
|------|---------|-----------|--------|
| Important | `created` / `completed` / `failed` / `cancelled` | async persist + 重试（5 次指数退避）+ dead-letter 缓冲 + sync publish | activities 表（SQLite WAL） |
| Informational | `streaming` / `updated` | async persist（失败丢弃）+ sync publish（streaming 16ms 批合并） | activities 表（尽力而为） |

同步更新 `project_rules.md` §九 AS-EVT-01 与 `biz/activity_event.go:83-88` 文档。

## 后果

**正面**：
- Terminal 事件（completed/failed）无 DB I/O 阻塞，WS 推送延迟 < 5ms
- streaming 路径 16ms 批合并不受影响，60fps 流畅
- dead-letter + reconnect replay 覆盖 WS 断连场景（进程存活）
- 实现简单，无需区分事件类型走不同 persist 路径

**负面**：
- 进程崩溃时，已 Publish 但未 persist 的事件会永久丢失（dead-letter 是 in-memory，进程重启即丢）
- 影响范围：用户在崩溃窗口内看到的 terminal 事件，reload 后会消失
- 概率：低（仅进程崩溃，非正常 WS 断连）

## 替代方案

**方案 A：补齐 WBPF**（未选择）
- 在 `processTask` 中对 `completed`/`failed` 事件先同步 `persistWithRetry` 成功后再 `eventBus.Publish`
- 否决理由：为 terminal 事件增加 5-30ms DB I/O 延迟，而 chat 应用非 checkpoint 恢复系统，崩溃丢数据风险可接受；收益不抵代价

**方案 B：保留 3 级分级但 Critical 走 WBPF**（未选择）
- 否决理由：增加 processTask 分支复杂度，且 Activity 模型已无 Checkpoint 概念，Critical 与 Important 边界模糊

**方案 C：dead-letter 持久化到磁盘**（未选择）
- 否决理由：增加 I/O 复杂度，且崩溃场景罕见，YAGNI

## 参考

- [activity_event_sequencer.go:311](../../internal/agent/activity_event_sequencer.go#L311) `processTask` 实现
- [biz/activity_event.go:83-88](../../internal/biz/activity_event.go#L83-L88) 可靠性文档
- [2026-06-25-analysis-chat-module-refactor.md §5.3](../reports/2026-06-25-analysis-chat-module-refactor.md) 可靠性分级设计
