# ADR-06: Activity Event Sequencer 重设计 — 单 Publish Worker 架构

> **状态**: 已接受 · **日期**: 2026-06-27 · **作者**: AI Assistant

## 背景

Chat UI 出现两个显示缺陷：

1. **流式回复未按 MD 实时格式化**：前端 `renderStreamingChatMarkdown` 走 escape-only 简化路径。
2. **最终回复内容跑到思考前面**：后端 `p.seq` 在 consumer goroutine 中 lazy 分配，跨 activity publish 顺序由 goroutine 调度决定。

经根因分析，问题 2 的根因链为：seq lazy 分配 → per-activity consumer 并发 publish → bus 内部跨 goroutine send 顺序不可控 → WS subscriber FIFO 但跨 activity 错乱 → 前端按到达顺序处理 → seq 错位。

## 决策

重设计 `activityEventSequencer`，从 per-activity channel 架构改为**单 publish worker + 全局 FIFO 队列**：

### 关键变更

1. **取消 per-activity channel**：删除 `channels map[string]chan publishTask` 和 per-activity consumer goroutine
2. **单 publish worker**：所有 publish 任务进入 `publishQueue`（buffer 256），单 goroutine 串行处理
3. **保留 persist worker**：DB I/O 仍独立 goroutine，避免阻塞 publish
4. **seq 在 projector 主流程分配**：在每个 `OnXxx` 入口（`p.mu` 内）`a.Seq = atomic.AddInt64(&p.seq, 1)`，删除 `activitySeq` 的 lazy 分配
5. **保留 16ms 批合并**：在 publish worker 内做，行为等价
6. **保留 dead-letter**：persist 失败入 ring buffer 512

### 保留的不变量

- 单 activity 内部 FIFO：On* 入口在 p.mu 下串行 + publishQueue FIFO → 同 activity 事件按入队顺序处理
- I/O offload：publish/persist 仍异步，OnTextDelta 不阻塞（B-04 防御保留）
- 失败重试与死信：persistWithRetry + pushDeadLetter 机制不变

## 后果

### 正面

- **跨 activity 顺序强保证**：single worker 串行 publish → bus subscriber FIFO → 前端按到达顺序处理 → seq 顺序 = UI 顺序
- **架构简化**：从 N 个 channel 减到 1 个 channel + 1 个 worker
- **调试更清晰**：single goroutine 易于加 tracing / metrics

### 负面

- **失去 per-activity 独立 backpressure**：所有 activity 共享 publishQueue 256 buffer
  - 缓解：16ms 批合并 + 监控队列深度
- **throughput 理论上限降低**：单 worker 串行 publish
  - 实测：1000 events/s 内 < 5ms/op，远低于批合并窗口
- **测试改动**：v1 既有测试需适配新 API

### 替代方案

| 方案 | 评价 |
|---|---|
| A. 纯同步 publish（取消 sequencer） | 复活 B-04：p.mu 内做 WS send，OnTextDelta 阻塞 |
| B. 多 publish worker + seq 排序 | 复杂度高，merge 逻辑复杂 |
| C. 前端 seq 校正（不修后端） | 不解根因，前端逻辑复杂 |

选择 v2 单 publish worker：保留 I/O offload（B-04）+ 强顺序保证 + 架构简化。

## 参考

- 设计文档：`docs/superpowers/specs/2026-06-27-chat-ui-streaming-fix-design.md`
- 实施计划：`docs/superpowers/plans/2026-06-27-chat-ui-streaming-fix.md`
- 相关 ADR：
  - ADR-04: Activity 事件可靠性分级（2026-06-26）
  - ADR-05: FlowLog 与 OTel Span 对齐（2026-06-27）
