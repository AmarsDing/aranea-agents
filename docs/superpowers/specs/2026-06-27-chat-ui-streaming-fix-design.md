# Chat UI 流式渲染与活动排序根本修复

> **设计稿** · 2026-06-27 · 状态：待用户复核

## 一、背景与问题陈述

前端 chat UI 在流式回复时出现两类显示缺陷：

### 现象 1：流式回复未按 Markdown 格式渲染
- 用户报告："回复内容流式输出时是纯文本，完成后才突然变成 MD 格式，感觉中间'没渲染'"
- 期望：流式过程中就应看到 MD 格式化效果（代码块、列表、链接等）

### 现象 2：最终回复内容位置错乱
- 用户报告："最终回复内容跑到思考（reasoning）前面了"
- 期望：思考始终出现在最终回复之前

## 二、根因分析

### 2.1 现象 1 根因：`renderStreamingChatMarkdown` 过度设计

[web/src/features/chat/chatMessageMarkdown.ts:133-137](file:///f:/aranea-agents/web/src/features/chat/chatMessageMarkdown.ts#L133-L137) 实现了流式时的简化渲染：

```ts
export function renderStreamingChatMarkdown(content: string): string {
  // During streaming, avoid full markdown-it parsing on every token.
  return markdown.utils.escapeHtml(content || '').replace(/\n/g, '<br>');
}
```

`ReplyBlock.vue:33-35` 在 `streaming=true` 时调用此函数，**只做 HTML 转义，不跑 markdown-it**。

**过度设计判断**：原设计意图是"避免每个 token 都跑 markdown-it 解析"。但**优化前提已不成立**——
[internal/agent/activity_event_sequencer.go:28](file:///f:/aranea-agents/internal/agent/activity_event_sequencer.go#L28) 的 `defaultDeltaBatchInterval = 16ms` 早已将前端事件频率封顶到 ≤60fps。

60fps × markdown-it 0.5-2ms 解析 ≈ 单核 CPU 占用 3-12%，完全可接受。

### 2.2 现象 2 根因链：`p.seq` 分配时机错误 + 跨 activity publish 顺序不可控

[internal/agent/activity_projector.go:171-178](file:///f:/aranea-agents/internal/agent/activity_projector.go#L171-L178)：

```go
func (p *ActivityProjector) activitySeq(a *biz.Activity) int64 {
  if a.Seq != 0 { return a.Seq }
  seq := atomic.AddInt64(&p.seq, 1)  // ← 在 consumer goroutine 中执行！
  a.Seq = seq
  return seq
}
```

完整因果链：

```
[1] LLM 流式 chunk → trpc-agent-go Event
[2] turnStreamConsumer.consume → ActivityProjector.ProcessEvent
[3] processChatCompletionChunk (p.mu 内串行)
    ├── OnReasoningDelta → publishAndPersist → publishEvent → buildActivityEvent → activitySeq
    │                                                            ↑
    │                                              在 consumer goroutine 中执行
    │                                              不是 projector 主流程
    │
    └── OnTextDelta   → publishAndPersist → publishEvent → buildActivityEvent → activitySeq
                                                                       ↑
                                                            同上问题
[4] sequencer.publish → per-activity channel → per-activity consumer goroutine
[5] 两个 consumer goroutine 并发 s.eventBus.Publish(thinking | reply)
[6] bus.Publish 内部 b.mu.RLock → 允许多 goroutine 并发进入
    → 两个 goroutine 各自 `sub.ch <- event` → 跨 goroutine channel send 顺序由 Go runtime 决定
[7] WS subscriber 顺序读取 → 但 thinking/reply 相对顺序错乱
[8] 前端 handleActivityEvent 按到达顺序处理 → seq 错位
[9] activityTree 按 seq 升序排 → reply 排在 thinking 前面 ❌
```

**核心错误**：`p.seq` 分配不在 projector 主流程（`p.mu` 内）而在 consumer goroutine 内，**seq 顺序 ≠ projector 业务顺序**。

### 2.3 过度设计全景

| 设计 | 评价 | 行动 |
|---|---|---|
| `renderStreamingChatMarkdown` 分流 | 优化前提已不存在 | **删除** |
| `p.seq` 在 consumer goroutine lazy 分配 | 破坏 seq = 业务顺序 不变量 | **前移到 OnXxx 入口** |
| per-activity channel + consumer goroutine | 解决 B-01/04/05 但**无跨 activity 顺序防御** | **重设计为单 publish worker** |
| Activity vs StreamEvent 双重模型 | 合理的数据/渲染解耦 | 保留 |
| `isFinal` 标志 | 轻微冗余 | 保留（避免破坏前端契约） |
| 16ms 批合并 | 合理 | 保留 |
| markdownCache (400 条 LRU) | 合理 | 保留 |

## 三、设计目标

1. **流式回复按 MD 实时格式化**（删除过度设计）
2. **跨 activity 顺序强保证**：thinking 必然在 reply 之前
3. **保留 I/O offload 特性**：不复活 B-04
4. **保留单 activity 内部 FIFO**：不复活 B-01
5. **保留失败持久化与 dead-letter 机制**

## 四、架构设计

### 4.1 核心不变量

| 不变量 | 含义 | 实现位置 |
|---|---|---|
| `seq` 顺序 = 业务顺序 | `OnXxx` 入口在 `p.mu` 内串行分配 seq | `OnXxx` 入口处 `a.Seq = atomic.AddInt64(&p.seq, 1)` |
| publish 顺序 = seq 升序 | 单 publish worker 串行处理 | 新 `publishWorker` goroutine |
| 单 activity 内部 FIFO | 同 activity 事件按 seq 升序入队 | `publishTaskQueue` 是全局 FIFO |
| I/O offload | publish/persist 不阻塞 `p.mu` | 主流程只入队，worker 异步处理 |

### 4.2 重构后的 sequencer 架构

**旧架构（per-activity channel）**：
```
OnXxx → publishEvent → buildActivityEvent(activitySeq 在此分配)
                     → sequencer.publish → per-activity channel
                                          → per-activity consumer goroutine
                                                                       → eventBus.Publish
```

**新架构（单 publish worker）**：
```
OnXxx [p.mu 内]
  a.Seq = atomic.AddInt64(&p.seq, 1)            ← seq 分配在主流程
  enqueuePublishTask(task{seq, event, activity}) ← 异步入队（不阻塞 p.mu）

[Single publish worker goroutine]
  loop:
    task := <- publishTaskQueue                   ← seq 升序（FIFO by 入队顺序）
    if task.kind == streaming && 同 seq/同 field 还有后续:
      merge_within_16ms_batch_window               ← 保留 16ms 批合并
    persistWithRetry(task)                         ← 异步持久化（保留 dead-letter）
    eventBus.Publish(task.event)                   ← 单 goroutine 串行 publish
```

**保留**：
- `persistChan` + `persistWorker`（独立 DB I/O goroutine）
- dead-letter 环形缓冲
- 16ms 批合并窗口（在 publish worker 内做）
- 持久化重试与退避

**取消**：
- per-activity channel（取消 `channels map[string]chan publishTask`）
- per-activity consumer goroutine（取消 `wg` per-activity 计数）
- `activitySeq` 的 lazy 分配分支

### 4.3 publish worker 内部批合并

新 publish worker 内仍做 16ms 批合并：

```go
type publishTask struct {
  seq       int64
  event     biz.ActivityEvent
  activity  biz.Activity
  persist   bool
}

func (w *publishWorker) run() {
  timer := time.NewTimer(batchInterval)
  var pending *publishTask
  
  for {
    select {
    case task := <-w.queue:
      if task.event.Event != biz.ActivityEventStreaming {
        // 非 streaming 立即 flush
        w.flush(pending)
        pending = nil
        w.processTask(task)
        continue
      }
      if pending != nil && w.canMerge(pending, task) {
        // 合并同 field delta
        pending.event.DeltaChunk += task.event.DeltaChunk
        w.resetTimer(timer)
        continue
      }
      w.flush(pending)
      pending = &task
      w.resetTimer(timer)
    case <-timer.C:
      w.flush(pending)
      pending = nil
    case <-w.done:
      // 排空后退出
      ...
    }
  }
}
```

**关键差异**：旧版的 merge 在 per-activity consumer 内做；新版在单 worker 内做。**merge 范围扩大到跨 activity**（thinking 和 reply 不会合并，因为 field 不同），但批合并延迟效果等价。

### 4.4 seq 分配不变量保证

`OnXxx` 入口在 `p.mu` 下串行：

```go
// OnReasoningDelta 修改示例
func (p *ActivityProjector) OnReasoningDelta(ctx context.Context, author string, chunk string, isPartial bool) {
  p.mu.Lock()
  defer p.mu.Unlock()
  
  // ... find or create thinking activity
  a := p.activities[activityID]
  
  // 立即分配 seq（在 p.mu 内、projector 主流程）
  if a.Seq == 0 {
    a.Seq = atomic.AddInt64(&p.seq, 1)
  }
  
  a.Reasoning += chunk
  p.enqueuePublish(ctx, a, biz.ActivityEventStreaming, "reasoning", chunk, false)
}
```

**所有 `OnXxx` 入口覆盖**：
- `OnTurnStart` (task created)
- `OnReasoningDelta`
- `OnReasoningDone`
- `OnTextDelta`
- `OnTextDone`
- `OnMemberMessageDelta`
- `OnMemberMessageDone`
- `OnToolCall`
- `OnToolResult`
- `OnError`（含 no-root 时的 task created）
- `OnNotice`
- `OnConfirmRequest`
- `OnConfirmResult`
- `OnPlanStart`
- `OnPlanStepUpdate`
- `OnStuckTools`
- `OnTurnEnd`
- `EmitSystemEvent`（系统事件，domain=system，**不分配 seq**，因为不进入 chat 渲染）

`activitySeq` 函数删除（或保留为编译期断言：`if a.Seq == 0 { panic("seq not pre-allocated") }`）。

### 4.5 MD 渲染路径统一

```ts
// 旧
export function renderChatMarkdownForMessage(messageId, content, streaming = false) {
  const key = markdownCacheKey(messageId, content, streaming);
  const hit = mdCache.get(key);
  if (hit !== undefined) return hit;
  const html = streaming ? renderStreamingChatMarkdown(content) : renderChatMarkdown(content);
  mdCache.set(key, html);
  ...
}

// 新
export function renderChatMarkdownForMessage(messageId, content, _streaming = false) {
  const key = markdownCacheKey(messageId, content);
  const hit = mdCache.get(key);
  if (hit !== undefined) return hit;
  const html = renderChatMarkdown(content);
  mdCache.set(key, html);
  trimMarkdownCache();
  return html;
}

// 删除
- export function renderStreamingChatMarkdown(content: string): string { ... }
```

`streaming` 参数保留仅为 API 兼容，渲染路径统一。

### 4.6 数据流（新）

```
[trpc Event] → turnStreamConsumer
  → ActivityProjector.ProcessEvent
    → processChatCompletionChunk
      → OnReasoningDelta  [p.mu 内]
        a.Seq = atomic.AddInt64(&p.seq, 1)  ← 主流程分配 ✅
        a.Reasoning += chunk
        enqueuePublishTask({a.Seq, "streaming", a, "reasoning", chunk, persist=false})
      → OnTextDelta  [p.mu 内]
        a.Seq = atomic.AddInt64(&p.seq, 1)  ← 必然 > thinking.Seq
        a.Content += chunk
        enqueuePublishTask({a.Seq, "streaming", a, "content", chunk, persist=false})

[Single publish worker goroutine]
  for task := range publishTaskQueue {
    if streaming + canMerge(pending, task):
      merge delta
      wait 16ms
    if persist:
      persistChan <- {task.activity}  // 异步给 persist worker
    eventBus.Publish(task.event)  // 串行
  }

[Single persist worker goroutine]
  for item := range persistChan {
    persistWithRetry(item.activity)  // 失败入 dead-letter
  }

[bus.Publish]  ← 串行（来自单 goroutine）
  → WS subscriber channel 严格 FIFO
  → WS handler 转发前端
  → 前端 handleActivityEvent 按到达顺序处理
  → activityTree 按 seq 升序排 → thinking 必然在 reply 之前 ✅
```

## 五、影响面

### 5.1 代码改动

| 文件 | 改动 |
|---|---|
| `internal/agent/activity_event_sequencer.go` | **重写**：单 publish worker 替代 per-activity channel；保留 persist worker |
| `internal/agent/activity_projector.go` | 所有 `OnXxx` 入口加 `a.Seq = atomic.AddInt64(&p.seq, 1)`；删除 `activitySeq` lazy 分支；`OnError`/task created path 同样 |
| `internal/agent/activity_event_sequencer_test.go` | 更新测试用例适配新架构 |
| `internal/agent/activity_projector_b01_b05_integration_test.go` | 验证 B-01/B-04/B-05 仍成立 |
| `web/src/features/chat/chatMessageMarkdown.ts` | 删除 `renderStreamingChatMarkdown`；简化 `renderChatMarkdownForMessage` |
| `web/src/components/chat/__tests__/ActivityStream.spec.ts` | 添加 reply-streaming-MD 测试 |

### 5.2 不变量回归测试

| 测试 | 验证内容 |
|---|---|
| `TestActivitySequencer_SinglePublishWorker` | 新架构下 publish FIFO |
| `TestActivitySequencer_CrossActivityOrder` | 1000 次并发 thinking+reply，100% 顺序正确 |
| `TestActivityProjector_B01_Regress` | 单 activity 内部 FIFO |
| `TestActivityProjector_B04_Regress` | 高频 delta 下 p.mu 持有时间 < 5ms |
| `TestActivityProjector_B05_Regress` | sync/async start race 不再发生 |
| `TestChatMarkdown_StreamingRenders` | 流式时也调用 markdown-it |

### 5.3 文档同步

| 文档 | 改动 |
|---|---|
| `docs/reports/2026-06-27-review-adr-activity-event-sequencer-redesign.md` | **新增 ADR**：sequencer 重设计背景/决策/后果 |
| `docs/development/1-chat.design.md` | §前端数据流章节更新 MD 渲染策略 |
| `docs/development/1-chat.development.md` | 新增 task：seq 前移 + 单 publish worker 改造 |

## 六、错误处理

| 场景 | 处理 |
|---|---|
| publishTaskQueue 满（buffer 256） | publish 阻塞 → OnTextDelta 阻塞 → 复活 B-04 风险 |
| 防御 | 16ms 批合并 + 监控队列深度；超过 200 触发告警 |
| eventBus.Publish panic | publish worker recover；记录日志；继续处理后续任务 |
| persist 失败 5 次 | dead-letter；前端通过 ListDeadLetterActivities + ListActivities API 合并 |
| Close() 时序 | consumers → publishTaskQueue（停止新任务） → publishWorker 排空 → persistChan → persistWorker 排空 |

## 七、风险评估

| 风险 | 等级 | 缓解 |
|---|---|---|
| 单 publish worker 吞吐下降 | 中 | 单元测试验证 1000 events/s 正常；publish < 5ms |
| 失去 per-activity 独立 backpressure | 低 | global channel buffer 256 + 16ms 批合并足够；监控队列深度 |
| B-04 复活 | 低 | publish 仍异步，OnTextDelta 不做 I/O；高频场景压测 |
| 取消 per-activity channel 导致 test 大改 | 中 | 新增等价测试覆盖；旧测试按新接口更新 |
| 跨进程升级 | 低 | Activity 持久化结构不变；WS 协议不变 |

## 八、验证策略

### 8.1 单元测试
- `go test ./internal/agent/... -count=1 -race` 全部通过
- `go test ./web/... -run TestChatMarkdown` 通过

### 8.2 集成测试
- `TestActivityProjector_B01_B05_Integration`：并发 reasoning + text delta，验证顺序
- `TestActivitySequencer_Order_1k`：1000 次并发事件，验证 100% FIFO

### 8.3 端到端验证
1. **MD 实时格式化**：发送 LLM 任务，观察长 reply 流式时即看到代码块/列表格式化
2. **顺序正确性**：复杂任务（thinking → tool → thinking → reply），观察最终回复在所有 thinking 之后
3. **性能**：1000 token 回复流式时间 < 5s，UI 不卡顿

### 8.4 回归
- B-01/B-04/B-05 既有测试全部通过
- 现有 chat 端到端测试不破

## 九、范围外

- Activity 持久化结构变更：不变
- WS 协议变更：不变
- 前端 ActivityStream 重构：不变（仅 MD 渲染路径简化）
- 跨进程 seq 同步：不变
- dead-letter 机制：不变

## 十、决策记录

| 决策 | 备选 | 选择理由 |
|---|---|---|
| 单 publish worker 而非取消 sequencer | 纯同步 publish / 多 publish worker | 保留 I/O offload（B-04 防御）；单 worker 保证 FIFO |
| seq 在 OnXxx 入口分配而非 buildActivityEvent | 保留 lazy 分配 + 前端校正 | 语义不变量"seq=业务顺序"必须在源头保证 |
| 保留 persist worker | 合并到 publish worker | persist 是 DB I/O（慢），与 publish（快）解耦，吞吐更优 |
| 保留 16ms 批合并 | 取消 | 仍是合理的前端频率控制 |
| 保留 markdownCache | 取消 | 60fps × 2ms 解析虽然可接受，但 cache 命中时 < 0.1ms，体验更稳 |
| 保留 isFinal 标志 | 删除（用 !streaming 推导） | 避免破坏前端契约；isFinal 语义明确 |

## 十一、参考资料

- [internal/agent/activity_projector.go](file:///f:/aranea-agents/internal/agent/activity_projector.go)
- [internal/agent/activity_event_sequencer.go](file:///f:/aranea-agents/internal/agent/activity_event_sequencer.go)
- [web/src/features/chat/chatMessageMarkdown.ts](file:///f:/aranea-agents/web/src/features/chat/chatMessageMarkdown.ts)
- [web/src/components/chat/ActivityStream.vue](file:///f:/aranea-agents/web/src/components/chat/ActivityStream.vue)
- [pkg/trpc-agent-go/event/bus/bus.go](file:///f:/aranea-agents/pkg/trpc-agent-go/event/bus/bus.go)
- ADR-04: Activity 事件可靠性分级（2026-06-26）
- ADR-05: FlowLog 与 OTel Span 对齐（2026-06-27）
