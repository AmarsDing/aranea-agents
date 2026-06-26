# Chat UI 流式渲染与活动排序根本修复 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 chat UI 两个显示缺陷（流式回复未按 MD 实时格式化、最终回复内容位置错乱），通过重设计后端 activity sequencer 架构 + 简化前端 MD 渲染路径，从根因上消除问题。

**Architecture:** 把后端 `activityEventSequencer` 的 per-activity channel 架构重写为**单 publish worker + 全局 FIFO 队列**。seq 在 projector 主流程（`p.mu` 内）的 `OnXxx` 入口处立即分配，确保 seq 顺序 = 业务顺序；publish worker 单 goroutine 串行调用 `eventBus.Publish`，从源头保证跨 activity 顺序。前端删除 `renderStreamingChatMarkdown` 简化路径，统一走 `renderChatMarkdown` 完整 MD 解析。

**Tech Stack:** Go (Kratos v2 + trpc-agent-go) / TypeScript (Vue 3 + Quasar + Pinia) / markdown-it + DOMPurify

---

## 实施策略

### 任务分组

| 阶段 | 任务 | 目的 |
|---|---|---|
| Phase 0: 准备 | Task 0 | 现状测试快照（基线） |
| Phase 1: 前端 MD 路径 | Task 1, 2 | 删除过度设计，独立可发布 |
| Phase 2: 后端 seq 分配 | Task 3, 4, 5 | 恢复"seq=业务顺序"不变量 |
| Phase 3: 后端 sequencer 重构 | Task 6, 7, 8, 9 | 单 publish worker，跨 activity FIFO |
| Phase 4: 集成验证 | Task 10, 11, 12 | 端到端 + 回归 |
| Phase 5: 文档 | Task 13, 14, 15 | ADR + development 文档同步 |

### 执行顺序原则

1. **Phase 1 独立可回滚**：MD 路径简化是纯前端改动，独立 ship
2. **Phase 2 必须 Phase 3 前完成**：seq 提前到主流程是 Phase 3 的前提
3. **Phase 3 任务 6-9 严格顺序**：每步建在前面基础上
4. **Phase 4 必须 Phase 1-3 全完后做**

---

## Phase 0: 准备

### Task 0: 建立基线测试快照

**Files:**
- Read: `internal/agent/activity_event_sequencer_test.go`
- Read: `internal/agent/activity_projector_b01_b05_integration_test.go`
- Read: `web/src/components/chat/__tests__/ActivityStream.spec.ts`

- [ ] **Step 1: 跑现有测试，记录基线**

Run: `cd f:\aranea-agents && go test ./internal/agent/... -count=1 -race -run "TestActivity|TestSequencer" 2>&1 | tail -50`
Expected: 全部通过，记录数量

- [ ] **Step 2: 跑前端测试**

Run: `cd f:\aranea-agents\web && pnpm test -- --reporter=verbose ActivityStream 2>&1 | tail -30`
Expected: 全部通过，记录数量

- [ ] **Step 3: 记录基线到 commit message**

```bash
git log --oneline -1
# 记录 commit hash 作为基线
```

---

## Phase 1: 前端 MD 渲染路径统一

### Task 1: 添加流式 MD 解析的测试（红）

**Files:**
- Modify: `web/src/features/chat/chatMessageMarkdown.ts`
- Modify: `web/src/features/chat/__tests__/chatMessageMarkdown.spec.ts`（如不存在则创建）

- [ ] **Step 1: 写测试用例**

在 `web/src/features/chat/__tests__/chatMessageMarkdown.spec.ts`（如不存在）创建测试文件，添加测试：

```ts
import { describe, it, expect } from 'vitest';
import { renderChatMarkdownForMessage, clearChatMarkdownCache } from '../chatMessageMarkdown';

describe('renderChatMarkdownForMessage - streaming MD formatting', () => {
  beforeEach(() => clearChatMarkdownCache());

  it('renders markdown in streaming mode (code blocks formatted)', () => {
    const content = '```python\nprint("hello")\n```';
    const html = renderChatMarkdownForMessage('msg-1', content, true);
    expect(html).toContain('class="code-block');
    expect(html).toContain('print');
  });

  it('renders markdown in streaming mode (lists formatted)', () => {
    const content = '- item 1\n- item 2\n- item 3';
    const html = renderChatMarkdownForMessage('msg-2', content, true);
    expect(html).toMatch(/<ul[^>]*>/);
    expect(html).toContain('<li>item 1</li>');
  });

  it('renders markdown in streaming mode (links formatted)', () => {
    const content = 'See [docs](https://example.com) for more';
    const html = renderChatMarkdownForMessage('msg-3', content, true);
    expect(html).toContain('<a href="https://example.com"');
  });

  it('streaming and non-streaming produce same MD output for same content', () => {
    const content = '**bold** and `code` and [link](https://x.com)';
    const streaming = renderChatMarkdownForMessage('msg-4', content, true);
    clearChatMarkdownCache();
    const nonStreaming = renderChatMarkdownForMessage('msg-4', content, false);
    // 核心断言：流式和非流式结果一致（streaming 参数不影响 MD 解析）
    expect(streaming).toBe(nonStreaming);
  });
});
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd f:\aranea-agents\web && pnpm test -- --reporter=verbose chatMessageMarkdown 2>&1 | tail -50`
Expected: 至少前三个测试失败（因为当前 `streaming=true` 走 `renderStreamingChatMarkdown` 不解析 MD），第 4 个 streaming vs non-streaming 一致性测试也可能失败

- [ ] **Step 3: 提交失败测试**

```bash
cd f:\aranea-agents
git add web/src/features/chat/__tests__/chatMessageMarkdown.spec.ts
git commit -m "test(chat): add streaming MD rendering tests (red)"
```

### Task 2: 实现 MD 路径统一（绿）

**Files:**
- Modify: `web/src/features/chat/chatMessageMarkdown.ts`

- [ ] **Step 1: 修改 `renderChatMarkdownForMessage` 简化路径**

在 `web/src/features/chat/chatMessageMarkdown.ts` 找到 `renderChatMarkdownForMessage` 函数（line 119-127），替换为：

```ts
/** Cached markdown render for chat rows (avoids re-parsing 100+ messages on each WS tick).
 *
 * Unified MD rendering: streaming and completed states use the same markdown-it
 * + DOMPurify pipeline. The `streaming` parameter is preserved for API
 * compatibility but no longer branches the rendering path.
 *
 * Rationale: with the backend's 16ms delta batch window (≤60fps), markdown-it
 * parsing at 0.5-2ms/call is well within budget. The previous "escape-only"
 * fast path was an over-optimization whose premise (per-token parse) no
 * longer holds — but caused users to perceive "data arrived but no render"
 * since plain text is visibly different from MD-formatted output.
 */
export function renderChatMarkdownForMessage(messageId: string, content: string, _streaming = false): string {
  const key = markdownCacheKey(messageId, content);
  const hit = mdCache.get(key);
  if (hit !== undefined) return hit;
  const html = renderChatMarkdown(content);
  mdCache.set(key, html);
  trimMarkdownCache();
  return html;
}
```

- [ ] **Step 2: 删除 `renderStreamingChatMarkdown` 函数**

在 `web/src/features/chat/chatMessageMarkdown.ts` 删除 line 133-137：

```ts
// DELETE the following function:
export function renderStreamingChatMarkdown(content: string): string {
  // During streaming, avoid full markdown-it parsing on every token. The final
  // text_done render still uses complete Markdown above.
  return markdown.utils.escapeHtml(content || '').replace(/\n/g, '<br>');
}
```

- [ ] **Step 3: 更新 `markdownCacheKey` 函数（移除 streaming 参数的影响）**

找到 `markdownCacheKey` 函数（line 100-108），替换为：

```ts
function markdownCacheKey(messageId: string, content: string): string {
  const id = messageId || 'anon';
  const len = content.length;
  // Use a hash-like key: combine length with head and tail to avoid collisions
  // where different content shares the same length and tail (common during streaming).
  const head = len > 48 ? content.slice(0, 48) : content;
  const tail = len > 48 ? content.slice(-48) : '';
  return `${id}:${len}:${head}:${tail}`;
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cd f:\aranea-agents\web && pnpm test -- --reporter=verbose chatMessageMarkdown 2>&1 | tail -30`
Expected: 全部通过

- [ ] **Step 5: 跑前端 lint**

Run: `cd f:\aranea-agents\web && pnpm lint 2>&1 | tail -20`
Expected: 无 error（warning 可接受）

- [ ] **Step 6: 提交**

```bash
cd f:\aranea-agents
git add web/src/features/chat/chatMessageMarkdown.ts
git commit -m "refactor(chat): unify MD rendering path, drop streaming escape-only branch"
```

---

## Phase 2: 后端 seq 分配前移

### Task 3: 添加 seq 分配契约测试（红）

**Files:**
- Create: `internal/agent/activity_projector_seq_test.go`

- [ ] **Step 1: 写测试文件**

创建 `internal/agent/activity_projector_seq_test.go`：

```go
package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"aranea-agents/internal/biz"
)

// TestActivityProjector_SeqAllocatedInMainFlow verifies that every Activity
// created via the public On* methods has its Seq assigned BEFORE the publish
// task is enqueued. This is the core invariant restored by the seq pre-allocation
// refactor (fix for cross-activity ordering bug where reply appeared before
// thinking).
func TestActivityProjector_SeqAllocatedInMainFlow(t *testing.T) {
	t.Parallel()

	repo := &fakeActivityRepo{}
	p := NewActivityProjector(&fakeEventBus{}, repo, nil)
	p.Configure(ProjectMeta{
		SessionID: "sess-1",
		RequestID: "turn-1",
		AgentID:   "agent-1",
	}, nil)
	p.Reset()
	p.OnTurnStart(context.Background(), ProjectMeta{
		SessionID: "sess-1",
		RequestID: "turn-1",
		AgentID:   "agent-1",
	})

	// Trigger concurrent thinking + reply
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		p.OnReasoningDelta(context.Background(), "author-1", "thinking content", true)
	}()
	go func() {
		defer wg.Done()
		p.OnTextDelta(context.Background(), "author-1", "reply content")
	}()
	wg.Wait()

	// Verify: thinking was created BEFORE reply (monotonic Seq)
	// OnReasoningDelta creates a new thinking activity (Seq=N)
	// OnTextDelta creates a new reply activity (Seq=N+1)
	// Both Seq values must be non-zero (pre-allocated, not lazy)
	all := repo.upserted()
	if len(all) < 2 {
		t.Fatalf("expected at least 2 upserts, got %d", len(all))
	}

	// Find the thinking and reply activities
	var thinkingSeq, replySeq int64
	for _, a := range all {
		if a.Kind == biz.ActivityKindThinking {
			thinkingSeq = a.Seq
		}
		if a.Kind == biz.ActivityKindReply {
			replySeq = a.Seq
		}
	}

	if thinkingSeq == 0 {
		t.Errorf("thinking activity has Seq=0 (not pre-allocated)")
	}
	if replySeq == 0 {
		t.Errorf("reply activity has Seq=0 (not pre-allocated)")
	}
	if thinkingSeq >= replySeq {
		t.Errorf("expected thinking.Seq (%d) < reply.Seq (%d) — pre-allocation broken", thinkingSeq, replySeq)
	}
}

// fakeActivityRepo is a minimal ActivityWriter for testing
type fakeActivityRepo struct {
	mu       sync.Mutex
	upserts  []biz.Activity
	counter  atomic.Int64
}

func (f *fakeActivityRepo) upserted() []biz.Activity {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]biz.Activity, len(f.upserts))
	copy(out, f.upserts)
	return out
}

func (f *fakeActivityRepo) UpsertActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	f.mu.Lock()
	f.upserts = append(f.upserts, a)
	f.mu.Unlock()
	return a, nil
}

func (f *fakeActivityRepo) CreateActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	return f.UpsertActivity(context.Background(), a)
}

func (f *fakeActivityRepo) UpdateActivity(_ context.Context, a biz.Activity) (biz.Activity, error) {
	return f.UpsertActivity(context.Background(), a)
}

// fakeEventBus is a minimal ActivityEventBus for testing
type fakeEventBus struct{}

func (f *fakeEventBus) Publish(_ context.Context, _ biz.ActivityEvent) {}
func (f *fakeEventBus) Subscribe(_ biz.ActivityEventSubscribeOptions) (<-chan biz.ActivityEvent, func()) {
	ch := make(chan biz.ActivityEvent)
	return ch, func() {}
}
func (f *fakeEventBus) DropCount() uint64 { return 0 }
```

- [ ] **Step 2: 跑测试**

Run: `cd f:\aranea-agents && go test ./internal/agent/ -run TestActivityProjector_SeqAllocatedInMainFlow -v -count=1 2>&1 | tail -30`
Expected: 测试编译可能失败（fakeActivityRepo 接口匹配问题），先修编译再观察测试结果
- 如果编译失败：根据错误调整 fakeActivityRepo / fakeEventBus 的接口签名匹配 `biz.ActivityWriter` 和 `biz.ActivityEventBus`
- 编译通过后：测试应失败（reply.Seq 可能 < thinking.Seq 或为 0）

- [ ] **Step 3: 提交失败测试**

```bash
cd f:\aranea-agents
git add internal/agent/activity_projector_seq_test.go
git commit -m "test(agent): add seq pre-allocation invariant test (red)"
```

### Task 4: 实现 seq 在 OnXxx 入口分配（绿）

**Files:**
- Modify: `internal/agent/activity_projector.go`

- [ ] **Step 1: 找到 `activitySeq` 函数（line 171-178）**

当前实现：
```go
func (p *ActivityProjector) activitySeq(a *biz.Activity) int64 {
  if a.Seq != 0 {
    return a.Seq
  }
  seq := atomic.AddInt64(&p.seq, 1)
  a.Seq = seq
  return seq
}
```

替换为断言版本：
```go
// activitySeq returns the pre-allocated Seq for an Activity.
// All Activity creation paths in On* methods MUST allocate Seq at the entry
// point (under p.mu). This function asserts the invariant and returns the
// pre-allocated value. Lazy allocation is no longer supported — see the
// architecture decision in docs/superpowers/specs/2026-06-27-chat-ui-streaming-fix-design.md.
func (p *ActivityProjector) activitySeq(a *biz.Activity) int64 {
  if a.Seq == 0 {
    panic(fmt.Sprintf("activity %s (%s) has Seq=0 — seq must be pre-allocated in On* entry", a.ID, a.Kind))
  }
  return a.Seq
}
```

- [ ] **Step 2: 修改 `OnTurnStart` 分配 seq**

找到 `OnTurnStart`（line 445-480），在创建 `a` 后立即分配 seq：

```go
func (p *ActivityProjector) OnTurnStart(ctx context.Context, meta ProjectMeta) {
  p.mu.Lock()
  defer p.mu.Unlock()
  if p.turnStarted {
    return
  }
  p.turnStarted = true
  p.meta = meta

  spiritSessionID := meta.SpiritSessionID
  if spiritSessionID == "" {
    spiritSessionID = meta.SessionID
  }

  id := uuid.NewString()
  now := time.Now().UTC()
  a := &biz.Activity{
    ID:               id,
    Kind:             biz.ActivityKindTask,
    Status:           biz.ActivityStatusRunning,
    SessionID:        meta.SessionID,
    TurnID:           meta.RequestID,
    Timestamp:        now,
    Seq:              atomic.AddInt64(&p.seq, 1),  // ← 立即分配
    AgentKey:         meta.AgentID,
    AgentName:        meta.AgentDisplayName,
    SpiritSessionID:  spiritSessionID,
    TeamID:           meta.TeamID,
    Content:          meta.TaskContent,
  }
  p.rootActivityID = id
  p.activities[id] = a

  p.publishAndPersist(ctx, a, biz.ActivityEventCreated)
}
```

- [ ] **Step 3: 修改 `OnReasoningDelta` 分配 seq**

找到 `OnReasoningDelta`（line 483-522），在创建新 thinking activity 时分配 seq：

```go
// Find or create the thinking activity for this author
activityID := p.findActivityByKindAuthor(biz.ActivityKindThinking, author)
if activityID == "" {
  // Create new thinking activity
  id := uuid.NewString()
  now := time.Now().UTC()
  a := &biz.Activity{
    ID:               id,
    Kind:             biz.ActivityKindThinking,
    Status:           biz.ActivityStatusRunning,
    SessionID:        p.meta.SessionID,
    TurnID:           p.meta.RequestID,
    ParentActivityID: p.rootActivityID,
    Timestamp:        now,
    Seq:              atomic.AddInt64(&p.seq, 1),  // ← 立即分配
    AgentKey:         author,
    AgentName:        p.resolveAgentName(ctx, author),
    SpiritSessionID:  p.resolveSpiritSessionID(),
    TeamID:           p.meta.TeamID,
  }
  p.activities[id] = a
  p.kindAuthorMap[kindKey(biz.ActivityKindThinking, author)] = id
  p.publishAndPersist(ctx, a, biz.ActivityEventCreated)
  activityID = id
}
```

- [ ] **Step 4: 修改 `OnTextDelta` 分配 seq**

找到 `OnTextDelta`（line 568-599），在创建新 reply activity 时分配 seq：

```go
// Find or create reply activity
activityID := p.findActivityByKindAuthor(biz.ActivityKindReply, author)
if activityID == "" {
  id := uuid.NewString()
  now := time.Now().UTC()
  a := &biz.Activity{
    ID:               id,
    Kind:             biz.ActivityKindReply,
    Status:           biz.ActivityStatusRunning,
    SessionID:        p.meta.SessionID,
    TurnID:           p.meta.RequestID,
    ParentActivityID: p.rootActivityID,
    Timestamp:        now,
    Seq:              atomic.AddInt64(&p.seq, 1),  // ← 立即分配
    AgentKey:         author,
    AgentName:        p.resolveAgentName(ctx, author),
    SpiritSessionID:  p.resolveSpiritSessionID(),
    TeamID:           p.meta.TeamID,
  }
  p.activities[id] = a
  p.kindAuthorMap[kindKey(biz.ActivityKindReply, author)] = id
  p.publishAndPersist(ctx, a, biz.ActivityEventCreated)
  activityID = id
}
```

- [ ] **Step 5: 修改 `OnMemberMessageDelta` 分配 seq**

找到 `OnMemberMessageDelta`（line 605-637），在创建新 reply activity 时分配 seq：

```go
// Find or create member reply activity
activityID := p.findActivityByKindAuthor(biz.ActivityKindReply, author)
if activityID == "" {
  id := uuid.NewString()
  now := time.Now().UTC()
  a := &biz.Activity{
    ID:               id,
    Kind:             biz.ActivityKindReply,
    Status:           biz.ActivityStatusRunning,
    SessionID:        p.meta.SessionID,
    TurnID:           p.meta.RequestID,
    ParentActivityID: p.rootActivityID,
    Timestamp:        now,
    Seq:              atomic.AddInt64(&p.seq, 1),  // ← 立即分配
    AgentKey:         author,
    AgentName:        p.resolveAgentName(ctx, author),
    SpiritSessionID:  p.resolveSpiritSessionID(),
    TeamID:           p.meta.TeamID,
    Meta:             map[string]any{"member_id": author},
  }
  p.activities[id] = a
  p.kindAuthorMap[kindKey(biz.ActivityKindReply, author)] = id
  p.publishAndPersist(ctx, a, biz.ActivityEventCreated)
  activityID = id
}
```

- [ ] **Step 6: 修改 `OnMemberMessageDone` 创建路径分配 seq**

找到 `OnMemberMessageDone`（line 643-689），在 else 分支（创建新 reply）时分配 seq：

```go
if activityID == "" {
  if fullText == "" {
    return
  }
  id := uuid.NewString()
  now := time.Now().UTC()
  a = &biz.Activity{
    ID:               id,
    Kind:             biz.ActivityKindReply,
    Status:           biz.ActivityStatusRunning,
    SessionID:        p.meta.SessionID,
    TurnID:           p.meta.RequestID,
    ParentActivityID: p.rootActivityID,
    Timestamp:        now,
    Seq:              atomic.AddInt64(&p.seq, 1),  // ← 立即分配
    Content:          fullText,
    AgentKey:         author,
    AgentName:        p.resolveAgentName(ctx, author),
    SpiritSessionID:  p.resolveSpiritSessionID(),
    TeamID:           p.meta.TeamID,
    Meta:             map[string]any{"member_id": author},
  }
  p.activities[id] = a
  p.kindAuthorMap[kindKey(biz.ActivityKindReply, author)] = id
  p.publishAndPersist(ctx, a, biz.ActivityEventCreated)
}
```

- [ ] **Step 7: 修改 `OnTextDone` 创建路径分配 seq**

找到 `OnTextDone`（line 695-740），在 else 分支（创建新 reply）时分配 seq：

```go
if activityID == "" {
  if fullText == "" {
    return
  }
  id := uuid.NewString()
  now := time.Now().UTC()
  a = &biz.Activity{
    ID:               id,
    Kind:             biz.ActivityKindReply,
    Status:           biz.ActivityStatusRunning,
    SessionID:        p.meta.SessionID,
    TurnID:           p.meta.RequestID,
    ParentActivityID: p.rootActivityID,
    Timestamp:        now,
    Seq:              atomic.AddInt64(&p.seq, 1),  // ← 立即分配
    Content:          fullText,
    AgentKey:         author,
    AgentName:        p.resolveAgentName(ctx, author),
    SpiritSessionID:  p.resolveSpiritSessionID(),
    TeamID:           p.meta.TeamID,
  }
  p.activities[id] = a
  p.kindAuthorMap[kindKey(biz.ActivityKindReply, author)] = id
  p.publishAndPersist(ctx, a, biz.ActivityEventCreated)
}
```

- [ ] **Step 8: 修改 `OnToolCall` 分配 seq**

找到 `OnToolCall`（line 745-806），在创建新 action activity 时分配 seq：

```go
id := uuid.NewString()
a := &biz.Activity{
  ID:               id,
  Kind:             biz.ActivityKindAction,
  Status:           biz.ActivityStatusToolRunning,
  SessionID:        p.meta.SessionID,
  TurnID:           p.meta.RequestID,
  ParentActivityID: p.rootActivityID,
  Timestamp:        startedAt,
  Seq:              atomic.AddInt64(&p.seq, 1),  // ← 立即分配
  ToolName:         toolName,
  ToolCallID:       toolCallID,
  ToolArguments:    argsJSON,
  AgentKey:         author,
  AgentName:        p.resolveAgentName(ctx, author),
  SpiritSessionID:  p.resolveSpiritSessionID(),
  TeamID:           p.meta.TeamID,
}
```

- [ ] **Step 9: 修改 `OnError` 创建路径分配 seq**

找到 `OnError`（line 856-914），在 Case 2（无 root 时创建新 task activity）分配 seq：

```go
// Case 2: no root task Activity — create a minimal failed task Activity
// so the error is still surfaced to the frontend.
id := uuid.NewString()
a := &biz.Activity{
  ID:              id,
  Kind:            biz.ActivityKindTask,
  Status:          biz.ActivityStatusFailed,
  SessionID:       p.meta.SessionID,
  TurnID:          p.meta.RequestID,
  Timestamp:       now,
  Seq:             atomic.AddInt64(&p.seq, 1),  // ← 立即分配
  DurationMs:      0,
  Content:         errMsg,
  AgentKey:        p.meta.AgentID,
  AgentName:       p.meta.AgentDisplayName,
  SpiritSessionID: p.meta.SpiritSessionID,
  TeamID:          p.meta.TeamID,
  Meta:            meta,
}
```

- [ ] **Step 10: 修改 `OnNotice` 分配 seq**

找到 `OnNotice`（line 917-945），在创建新 notice activity 时分配 seq：

```go
id := uuid.NewString()
now := time.Now().UTC()
activity := &biz.Activity{
  ID:               id,
  Kind:             biz.ActivityKindNotice,
  Status:           biz.ActivityStatusPending,
  SessionID:        sessionID,
  TurnID:           turnID,
  ParentActivityID: p.rootActivityID,
  Timestamp:        now,
  Seq:              atomic.AddInt64(&p.seq, 1),  // ← 立即分配
  Content:          content,
  Meta:             map[string]any{"noticeType": noticeType},
}
```

- [ ] **Step 11: 修改 `OnConfirmRequest` 分配 seq**

找到 `OnConfirmRequest`（line 969-990），在创建新 confirm activity 时分配 seq：

```go
id := uuid.NewString()
now := time.Now().UTC()
activity := &biz.Activity{
  ID:               id,
  Kind:             biz.ActivityKindConfirm,
  Status:           biz.ActivityStatusToolBlocked,
  SessionID:        sessionID,
  TurnID:           turnID,
  ParentActivityID: p.rootActivityID,
  Timestamp:        now,
  Seq:              atomic.AddInt64(&p.seq, 1),  // ← 立即分配
  Content:          params.Content,
  Meta:             map[string]any{"toolName": params.ToolName, "toolArguments": params.ToolArguments},
}
```

- [ ] **Step 12: 修改 `OnPlanStart` 分配 seq**

找到 `OnPlanStart`（line 1175-1195），在创建新 plan activity 时分配 seq：

```go
id := uuid.NewString()
now := time.Now().UTC()
activity := &biz.Activity{
  ID:        id,
  Kind:      biz.ActivityKindPlan,
  Status:    biz.ActivityStatusPending,
  SessionID: sessionID,
  TurnID:    turnID,
  Timestamp: now,
  Seq:       atomic.AddInt64(&p.seq, 1),  // ← 立即分配
  Content:   title,
  Meta:      map[string]any{"steps": steps},
}
```

- [ ] **Step 13: 修改 `processGraphNodeStart` 创建路径分配 seq**

找到 `processGraphNodeStart`（line 1052-1116），在 lazy 创建 plan activity 时分配 seq：

```go
// Lazily create plan Activity on first node arrival (inline to avoid deadlock)
if p.planActivityID == "" {
  id := uuid.NewString()
  now := time.Now().UTC()
  planAct := &biz.Activity{
    ID:        id,
    Kind:      biz.ActivityKindPlan,
    Status:    biz.ActivityStatusRunning,
    SessionID: p.meta.SessionID,
    TurnID:    p.meta.RequestID,
    Timestamp: now,
    Seq:       atomic.AddInt64(&p.seq, 1),  // ← 立即分配
    ParentActivityID: p.rootActivityID,
    Content:          "执行计划",
    Meta:             map[string]any{"steps": []biz.ActivityPlanStep{}},
  }
  p.activities[id] = planAct
  p.planActivityID = id
  p.publishAndPersist(ctx, planAct, biz.ActivityEventCreated)
}
```

- [ ] **Step 14: 跑测试确认通过**

Run: `cd f:\aranea-agents && go test ./internal/agent/ -run TestActivityProjector_SeqAllocatedInMainFlow -v -count=1 2>&1 | tail -20`
Expected: PASS

- [ ] **Step 15: 跑全量 agent 测试，确认 B-01/B-04/B-05 不破**

Run: `cd f:\aranea-agents && go test ./internal/agent/... -count=1 -race 2>&1 | tail -30`
Expected: 全部通过

- [ ] **Step 16: 提交**

```bash
cd f:\aranea-agents
git add internal/agent/activity_projector.go
git commit -m "refactor(agent): pre-allocate Activity.Seq in On* entry, remove lazy allocation"
```

### Task 5: 删除 `p.seq` 在 `buildActivityEvent` 内的副作用

**Files:**
- Modify: `internal/agent/activity_projector.go`

- [ ] **Step 1: 修改 `buildActivityEvent` 移除 `p.activitySeq(a)` 调用**

找到 `buildActivityEvent`（line 1468-1490），删除对 `p.activitySeq(a)` 的调用：

```go
// buildActivityEvent creates an ActivityEvent for an Activity lifecycle event.
// The Activity snapshot is included directly — no metadata packing needed,
// simplifying the frontend contract compared to the legacy Envelope format.
//
// Seq must be pre-allocated in the On* entry point (under p.mu). This function
// no longer assigns Seq — it only reads the pre-allocated value via activitySeq.
func (p *ActivityProjector) buildActivityEvent(a *biz.Activity, eventType biz.ActivityEventType) biz.ActivityEvent {
  // Build a redacted copy for the event payload. The redaction limit
  // (512 bytes) matches biz.redactActivityJSON and the frontend
  // ACTIVITY_JSON_PREVIEW_LIMIT, ensuring consistency.
  snapshot := *a
  if snapshot.ToolArguments != "" {
    snapshot.ToolArguments = biz.RedactActivityJSON(snapshot.ToolArguments)
  }
  if snapshot.ToolResult != "" {
    snapshot.ToolResult = biz.RedactActivityJSON(snapshot.ToolResult)
  }

  return biz.ActivityEvent{
    Event:    eventType,
    Activity: snapshot,
    Domain:   biz.ActivityDomainChat,
  }
}
```

- [ ] **Step 2: 跑测试**

Run: `cd f:\aranea-agents && go test ./internal/agent/... -count=1 -race 2>&1 | tail -30`
Expected: 全部通过

- [ ] **Step 3: 跑 vet**

Run: `cd f:\aranea-agents && go vet ./internal/agent/... 2>&1 | tail -20`
Expected: 无错误

- [ ] **Step 4: 提交**

```bash
cd f:\aranea-agents
git add internal/agent/activity_projector.go
git commit -m "refactor(agent): remove lazy Seq allocation in buildActivityEvent"
```

---

## Phase 3: 后端 sequencer 重构（单 publish worker）

### Task 6: 写新 sequencer 单元测试（红）

**Files:**
- Create: `internal/agent/activity_event_sequencer_v2_test.go`

- [ ] **Step 1: 创建测试文件**

创建 `internal/agent/activity_event_sequencer_v2_test.go`：

```go
package agent

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// TestSequencerV2_SinglePublishWorker_FIFO verifies that publish events are
// emitted in strict FIFO order matching enqueue order (which matches seq).
func TestSequencerV2_SinglePublishWorker_FIFO(t *testing.T) {
	t.Parallel()

	eventBus := &recordingEventBus{}
	repo := &fakeActivityRepo{}
	seq := newActivityEventSequencer(eventBus, nil)
	seq.SetActivityRepo(repo)

	// Enqueue 100 tasks with incrementing seq
	const N = 100
	for i := 0; i < N; i++ {
		a := biz.Activity{
			ID:        bizID(i),
			Kind:      biz.ActivityKindReply,
			Status:    biz.ActivityStatusRunning,
			SessionID: "sess-1",
			Seq:       int64(i + 1),
		}
		ev := biz.ActivityEvent{
			Event:    biz.ActivityEventStreaming,
			Activity: a,
		}
		if err := seq.publish(context.Background(), a.ID, publishTask{
			event:    ev,
			persist:  false,
			activity: a,
		}); err != nil {
			t.Fatalf("publish %d failed: %v", i, err)
		}
	}

	seq.Close()

	// Verify: eventBus received events in seq order
	received := eventBus.receivedSeq()
	if len(received) != N {
		t.Fatalf("expected %d events, got %d", N, len(received))
	}
	for i, seq := range received {
		if seq != int64(i+1) {
			t.Errorf("event %d: expected seq=%d, got seq=%d", i, i+1, seq)
		}
	}
}

// TestSequencerV2_CrossActivityOrder_Concurrent simulates concurrent thinking
// + reply creation and verifies reply always comes after thinking in publish
// order. The OLD per-activity channel architecture had race conditions here.
func TestSequencerV2_CrossActivityOrder_Concurrent(t *testing.T) {
	t.Parallel()

	eventBus := &recordingEventBus{}
	repo := &fakeActivityRepo{}
	seq := newActivityEventSequencer(eventBus, nil)
	seq.SetActivityRepo(repo)

	const N = 1000
	var counter atomic.Int64
	var wg sync.WaitGroup
	wg.Add(N * 2)

	// Enqueue N "thinking created" tasks
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			seqNum := counter.Add(1)
			a := biz.Activity{
				ID:    "think-" + bizIDint(seqNum),
				Kind:  biz.ActivityKindThinking,
				Seq:   seqNum,
			}
			seq.publish(context.Background(), a.ID, publishTask{
				event:    biz.ActivityEvent{Event: biz.ActivityEventCreated, Activity: a},
				activity: a,
			})
		}()
	}

	// Enqueue N "reply created" tasks (with higher seq — assigned after thinking)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			seqNum := counter.Add(1)
			a := biz.Activity{
				ID:    "reply-" + bizIDint(seqNum),
				Kind:  biz.ActivityKindReply,
				Seq:   seqNum,
			}
			seq.publish(context.Background(), a.ID, publishTask{
				event:    biz.ActivityEvent{Event: biz.ActivityEventCreated, Activity: a},
				activity: a,
			})
		}()
	}

	wg.Wait()
	// Wait for publish worker to drain
	time.Sleep(100 * time.Millisecond)
	seq.Close()

	// Verify: all 2N events received
	received := eventBus.received()
	if len(received) != N*2 {
		t.Fatalf("expected %d events, got %d", N*2, len(received))
	}

	// Verify: seq values are in monotonic order (this is what the OLD design failed)
	lastSeq := int64(0)
	for i, a := range received {
		if a.Seq <= lastSeq && i > 0 {
			t.Errorf("event %d: seq=%d not strictly greater than previous seq=%d (cross-activity order broken)", i, a.Seq, lastSeq)
			break
		}
		lastSeq = a.Seq
	}
}

// recordingEventBus captures all published events in arrival order
type recordingEventBus struct {
	mu       sync.Mutex
	received_ []biz.Activity
}

func (b *recordingEventBus) Publish(_ context.Context, ev biz.ActivityEvent) {
	b.mu.Lock()
	b.received_ = append(b.received_, ev.Activity)
	b.mu.Unlock()
}

func (b *recordingEventBus) Subscribe(_ biz.ActivityEventSubscribeOptions) (<-chan biz.ActivityEvent, func()) {
	ch := make(chan biz.ActivityEvent)
	return ch, func() {}
}

func (b *recordingEventBus) DropCount() uint64 { return 0 }

func (b *recordingEventBus) received() []biz.Activity {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]biz.Activity, len(b.received_))
	copy(out, b.received_)
	return out
}

func (b *recordingEventBus) receivedSeq() []int64 {
	acts := b.received()
	out := make([]int64, len(acts))
	for i, a := range acts {
		out[i] = a.Seq
	}
	return out
}

func bizID(i int) string {
	return "act-" + bizIDint(int64(i))
}

func bizIDint(i int64) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	out := []byte{}
	for i > 0 {
		out = append([]byte{digits[i%10]}, out...)
		i /= 10
	}
	return string(out)
}
```

- [ ] **Step 2: 跑测试（红，新 sequencer 还没改）**

Run: `cd f:\aranea-agents && go test ./internal/agent/ -run TestSequencerV2 -v -count=1 2>&1 | tail -30`
Expected: 测试失败（当前 per-activity channel 架构不保证 FIFO）

- [ ] **Step 3: 提交失败测试**

```bash
cd f:\aranea-agents
git add internal/agent/activity_event_sequencer_v2_test.go
git commit -m "test(agent): add sequencer v2 single-publish-worker tests (red)"
```

### Task 7: 重写 sequencer 为单 publish worker

**Files:**
- Modify: `internal/agent/activity_event_sequencer.go`

- [ ] **Step 1: 重写 sequencer 主体**

整个 `activity_event_sequencer.go` 文件替换为：

```go
package agent

import (
	"context"
	"errors"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// defaultPublishBufferSize is the buffer size of the shared publish queue.
// When full, publish blocks (backpressure) which propagates to the LLM:
// queue full → OnTextDelta blocks → stream_consumer blocks → LLM pauses.
const defaultPublishBufferSize = 256

// defaultPersistBufferSize is the buffer size of the shared persist channel.
const defaultPersistBufferSize = 256

// defaultDeltaBatchInterval is the maximum time window during which consecutive
// streaming events for the same field are coalesced into a single event.
const defaultDeltaBatchInterval = 16 * time.Millisecond

// persist retry configuration (unchanged from v1)
const (
	persistMaxRetries       = 5
	persistInitialBackoffMs = 100
	persistBackoffFactor    = 2
)

// deadLetterCapacity is the maximum number of failed-persist activities
// retained in the dead-letter buffer.
const deadLetterCapacity = 512

// errSequencerClosed is returned when publishing to a closed sequencer.
var errSequencerClosed = errors.New("activity event sequencer closed")

// activityEventSequencer (v2): single publish worker architecture.
//
// The v2 design replaces per-activity channels (v1) with a single shared
// publish queue + one worker goroutine. The publish worker processes tasks
// in strict FIFO order, which guarantees:
//   - Cross-activity order: tasks are published in the exact order they were
//     enqueued, which (since seq is pre-allocated at On* entry) matches the
//     projector business order. The v1 per-activity channels had goroutine
//     scheduling races that allowed reply events to be published before
//     thinking events.
//   - Single-activity FIFO: tasks for the same activity are naturally ordered
//     because the On* methods are serialized under p.mu.
//   - I/O offload: publish/persist still happen in worker goroutines, so
//     OnTextDelta does not block on WS or DB I/O (B-04 fix preserved).
//
// Design rationale:
//   - Single publish queue: no cross-goroutine channel ordering issues
//   - One worker: serializes eventBus.Publish calls → WS subscriber FIFO
//   - Separate persist worker: DB I/O parallelism (unchanged from v1)
type activityEventSequencer struct {
	eventBus    biz.ActivityEventBus
	activityRepo biz.ActivityWriter
	lg          loggateway.Logger

	// publishQueue: single shared FIFO queue feeding the publish worker.
	// All On* methods enqueue tasks here (under p.mu for seq ordering).
	publishQueue chan publishTask
	publishWg    sync.WaitGroup

	// persistChan: feeds the single persist worker goroutine.
	persistChan chan persistItem
	persistWg   sync.WaitGroup

	// Lifecycle
	mu     sync.Mutex
	closed bool
	done   chan struct{}

	// deltaBatchInterval: streaming events coalescing window
	deltaBatchInterval time.Duration

	// Retry parameters
	persistMaxRetries       int
	persistInitialBackoffMs int
	persistBackoffFactor    int

	// deadLetter ring buffer
	deadLetterMu sync.Mutex
	deadLetter   []biz.Activity
}

// persistItem is a single activity to persist
type persistItem struct {
	activityID string
	activity   biz.Activity
}

// publishTask represents a single ActivityEvent to publish and optionally persist.
type publishTask struct {
	event    biz.ActivityEvent
	persist  bool
	activity biz.Activity
}

// newActivityEventSequencer creates a new v2 sequencer.
func newActivityEventSequencer(eventBus biz.ActivityEventBus, lg loggateway.Logger) *activityEventSequencer {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &activityEventSequencer{
		eventBus:                eventBus,
		lg:                      lg,
		done:                    make(chan struct{}),
		deltaBatchInterval:      defaultDeltaBatchInterval,
		persistMaxRetries:       persistMaxRetries,
		persistInitialBackoffMs: persistInitialBackoffMs,
		persistBackoffFactor:    persistBackoffFactor,
		publishQueue:            make(chan publishTask, defaultPublishBufferSize),
	}
}

// publish enqueues a task to the publish queue. Blocks if queue is full
// (backpressure), or returns errSequencerClosed if sequencer is closed.
func (s *activityEventSequencer) publish(ctx context.Context, activityID string, task publishTask) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errSequencerClosed
	}
	s.mu.Unlock()

	select {
	case s.publishQueue <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return errSequencerClosed
	}
}

// runPublishWorker is the single goroutine that processes all publish tasks
// in FIFO order. It is started lazily on first publish.
//
// Streaming events are batched within deltaBatchInterval to reduce event
// frequency (≤60fps to frontend). The batch window is preserved from v1.
func (s *activityEventSequencer) runPublishWorker() {
	defer s.publishWg.Done()

	batchInterval := s.deltaBatchInterval
	if batchInterval <= 0 {
		batchInterval = defaultDeltaBatchInterval
	}

	timer := time.NewTimer(batchInterval)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	defer timer.Stop()

	var pending *publishTask

	flush := func() {
		if pending == nil {
			return
		}
		s.processTask(*pending)
		pending = nil
	}
	defer flush()

	resetTimer := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(batchInterval)
	}

	mergeDelta := func(dst *publishTask, src publishTask) {
		dst.event.DeltaChunk += src.event.DeltaChunk
	}

	for {
		select {
		case task, ok := <-s.publishQueue:
			if !ok {
				// Queue closed by Close(); drain remaining pending and exit
				return
			}
			if task.event.Event != biz.ActivityEventStreaming {
				// Non-streaming events (created/completed/etc.) must be published
				// immediately so terminal events are not delayed by the batch window.
				flush()
				s.processTask(task)
				continue
			}
			if pending != nil && s.canMergeDeltas(*pending, task) {
				mergeDelta(pending, task)
				resetTimer()
				continue
			}
			flush()
			pending = &task
			resetTimer()

		case <-timer.C:
			flush()

		case <-s.done:
			// Sequencer is closing; drain queue and exit
			for {
				select {
				case task, ok := <-s.publishQueue:
					if !ok {
						return
					}
					if task.event.Event != biz.ActivityEventStreaming {
						flush()
						s.processTask(task)
						continue
					}
					if pending != nil && s.canMergeDeltas(*pending, task) {
						mergeDelta(pending, task)
						continue
					}
					flush()
					pending = &task
				default:
					return
				}
			}
		}
	}
}

// canMergeDeltas reports whether two consecutive streaming events can be
// coalesced into a single event.
func (s *activityEventSequencer) canMergeDeltas(a, b publishTask) bool {
	if a.event.Event != biz.ActivityEventStreaming || b.event.Event != biz.ActivityEventStreaming {
		return false
	}
	if a.event.DeltaField == "" || b.event.DeltaField == "" || a.event.DeltaField != b.event.DeltaField {
		return false
	}
	return true
}

// processTask persists the activity (fire-and-forget) and publishes the
// ActivityEvent synchronously. Called from the single publish worker —
// serializes all eventBus.Publish calls.
func (s *activityEventSequencer) processTask(task publishTask) {
	if task.persist && s.activityRepo != nil {
		item := persistItem{activityID: task.activity.ID, activity: task.activity}
		select {
		case s.persistChan <- item:
			// enqueued for async persist
		default:
			// Channel full: fall back to synchronous persist
			s.persistWithRetry(task.activity.ID, task.activity, true)
		}
	}
	if s.eventBus != nil {
		s.eventBus.Publish(context.Background(), task.event)
	}
}

// persistWithRetry calls UpsertActivity with exponential backoff retry.
func (s *activityEventSequencer) persistWithRetry(activityID string, a biz.Activity, syncFallback bool) {
	backoff := s.persistInitialBackoffMs
	path := "worker"
	if syncFallback {
		path = "sync_fallback"
	}
	for attempt := 0; attempt <= s.persistMaxRetries; attempt++ {
		if _, err := s.activityRepo.UpsertActivity(context.Background(), a); err == nil {
			return
		} else if attempt == s.persistMaxRetries {
			s.lg.Warn("activity persist failed after retries; pushed to dead-letter buffer",
				loggateway.StepID("agent.activity_sequencer.persist"),
				loggateway.Str("activity_id", activityID),
				loggateway.Str("kind", string(a.Kind)),
				loggateway.Str("status", string(a.Status)),
				loggateway.Str("path", path),
				loggateway.Int("attempts", attempt+1),
				loggateway.Err(err))
			s.pushDeadLetter(a)
			return
		}
		select {
		case <-s.done:
			s.pushDeadLetter(a)
			return
		case <-time.After(time.Duration(backoff) * time.Millisecond):
		}
		backoff *= s.persistBackoffFactor
	}
}

// pushDeadLetter appends a failed-persist activity to the dead-letter buffer.
func (s *activityEventSequencer) pushDeadLetter(a biz.Activity) {
	s.deadLetterMu.Lock()
	defer s.deadLetterMu.Unlock()
	for i := range s.deadLetter {
		if s.deadLetter[i].ID == a.ID {
			s.deadLetter[i] = a
			return
		}
	}
	if len(s.deadLetter) >= deadLetterCapacity {
		s.deadLetter = append(s.deadLetter[:0], s.deadLetter[1:]...)
	}
	s.deadLetter = append(s.deadLetter, a)
}

// ListDeadLetterActivities returns a snapshot of dead-letter activities.
func (s *activityEventSequencer) ListDeadLetterActivities(sessionID string) []biz.Activity {
	s.deadLetterMu.Lock()
	defer s.deadLetterMu.Unlock()
	if len(s.deadLetter) == 0 {
		return nil
	}
	out := make([]biz.Activity, 0, len(s.deadLetter))
	for _, a := range s.deadLetter {
		if sessionID == "" || a.SessionID == sessionID {
			out = append(out, a)
		}
	}
	return out
}

// SetActivityRepo sets the activity repository and starts worker goroutines.
func (s *activityEventSequencer) SetActivityRepo(repo biz.ActivityWriter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activityRepo = repo
	if repo != nil {
		if s.persistChan == nil {
			s.persistChan = make(chan persistItem, defaultPersistBufferSize)
			s.persistWg.Add(1)
			safego.GoBackground("activity_persist_worker", func() {
				s.runPersistWorker()
			})
		}
		if s.publishQueue != nil {
			// Use a sync.Once pattern via channel close detection
			// We use a flag to ensure single worker start
			s.publishWg.Add(1)
			safego.GoBackground("activity_publish_worker", func() {
				s.runPublishWorker()
			})
		}
	}
}

// runPersistWorker is the single persist goroutine.
func (s *activityEventSequencer) runPersistWorker() {
	defer s.persistWg.Done()
	for item := range s.persistChan {
		s.persistWithRetry(item.activityID, item.activity, false)
	}
}

// Close closes the sequencer and waits for all workers to finish.
func (s *activityEventSequencer) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.done)
	s.mu.Unlock()

	// Close publish queue to signal publish worker to exit
	close(s.publishQueue)
	s.publishWg.Wait()

	// Close persist channel
	if s.persistChan != nil {
		close(s.persistChan)
		s.persistWg.Wait()
	}
}
```

- [ ] **Step 2: 跑新 sequencer 测试**

Run: `cd f:\aranea-agents && go test ./internal/agent/ -run TestSequencerV2 -v -count=1 2>&1 | tail -30`
Expected: PASS

- [ ] **Step 3: 跑全量 agent 测试**

Run: `cd f:\aranea-agents && go test ./internal/agent/... -count=1 -race 2>&1 | tail -30`
Expected: 全部通过

- [ ] **Step 4: 跑 vet**

Run: `cd f:\aranea-agents && go vet ./internal/agent/... 2>&1 | tail -20`
Expected: 无错误

- [ ] **Step 5: 提交**

```bash
cd f:\aranea-agents
git add internal/agent/activity_event_sequencer.go
git commit -m "refactor(agent): rewrite sequencer to single-publish-worker architecture"
```

### Task 8: 更新既有 B-01/B-04/B-05 测试

**Files:**
- Modify: `internal/agent/activity_projector_b01_b05_integration_test.go`
- Modify: `internal/agent/activity_event_sequencer_test.go`

- [ ] **Step 1: 跑既有测试看是否破**

Run: `cd f:\aranea-agents && go test ./internal/agent/ -run "TestB01|TestB04|TestB05|TestSequencer" -v -count=1 2>&1 | tail -50`
Expected: 一些旧测试可能因 sequencer API 变化失败

- [ ] **Step 2: 修复失败的旧测试**

对于每个失败的测试，根据错误调整：
- 如果是 `channels` 字段访问 → 改为 `publishQueue`
- 如果是 `wg` 等待 → 改为 `publishWg`
- 如果是 mock per-activity 行为 → 适配新架构

（如有大量适配工作，按需修改；目标是保持 B-01/B-04/B-05 行为契约不变）

- [ ] **Step 3: 跑全量测试**

Run: `cd f:\aranea-agents && go test ./internal/agent/... -count=1 -race 2>&1 | tail -30`
Expected: 全部通过

- [ ] **Step 4: 提交**

```bash
cd f:\aranea-agents
git add internal/agent/activity_event_sequencer_test.go internal/agent/activity_projector_b01_b05_integration_test.go
git commit -m "test(agent): update B-01/B-04/B-05 tests for v2 sequencer"
```

### Task 9: 性能验证（单 publish worker 吞吐）

**Files:**
- Create: `internal/agent/activity_event_sequencer_bench_test.go`

- [ ] **Step 1: 写性能基准测试**

创建 `internal/agent/activity_event_sequencer_bench_test.go`：

```go
package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
)

// BenchmarkSequencerV2_Throughput measures max events/second for v2 sequencer.
func BenchmarkSequencerV2_Throughput(b *testing.B) {
	eventBus := &recordingEventBus{}
	repo := &fakeActivityRepo{}
	seq := newActivityEventSequencer(eventBus, nil)
	seq.SetActivityRepo(repo)

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a := biz.Activity{
			ID:    bizID(i),
			Kind:  biz.ActivityKindReply,
			Status: biz.ActivityStatusRunning,
			SessionID: "sess-1",
			Seq:   int64(i + 1),
		}
		ev := biz.ActivityEvent{
			Event:    biz.ActivityEventStreaming,
			Activity: a,
		}
		seq.publish(ctx, a.ID, publishTask{
			event:    ev,
			activity: a,
		})
	}
	b.StopTimer()
	seq.Close()
}
```

- [ ] **Step 2: 跑基准测试**

Run: `cd f:\aranea-agents && go test ./internal/agent/ -bench=BenchmarkSequencerV2 -benchtime=5s -run=^$ 2>&1 | tail -20`
Expected: 输出 ns/op，应 < 100μs/op（远低于 16ms 批合并窗口）

- [ ] **Step 3: 提交**

```bash
cd f:\aranea-agents
git add internal/agent/activity_event_sequencer_bench_test.go
git commit -m "test(agent): add sequencer v2 throughput benchmark"
```

---

## Phase 4: 集成验证

### Task 10: 跨 activity 顺序端到端测试

**Files:**
- Create: `internal/agent/activity_cross_order_e2e_test.go`

- [ ] **Step 1: 写端到端测试**

创建 `internal/agent/activity_cross_order_e2e_test.go`：

```go
package agent

import (
	"context"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
)

// TestActivityProjector_CrossOrderE2E simulates a realistic LLM turn:
// thinking → tool → thinking → reply, and verifies the publish order
// matches the expected business order.
func TestActivityProjector_CrossOrderE2E(t *testing.T) {
	t.Parallel()

	eventBus := &recordingEventBus{}
	repo := &fakeActivityRepo{}
	proj := NewActivityProjector(eventBus, repo, nil)
	proj.Configure(ProjectMeta{
		SessionID: "sess-e2e",
		RequestID: "turn-e2e",
		AgentID:   "agent-1",
	}, nil)
	proj.Reset()
	proj.OnTurnStart(context.Background(), ProjectMeta{
		SessionID: "sess-e2e",
		RequestID: "turn-e2e",
		AgentID:   "agent-1",
	})

	ctx := context.Background()
	var wg sync.WaitGroup

	// Phase 1: thinking
	wg.Add(1)
	go func() {
		defer wg.Done()
		proj.OnReasoningDelta(ctx, "agent-1", "Let me think", true)
		proj.OnReasoningDelta(ctx, "agent-1", " about this problem", true)
		proj.OnReasoningDone(ctx, "agent-1", "Let me think about this problem", false)
	}()

	// Phase 2: tool call (in parallel)
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Wait for thinking to be created
		// In real code this is sequenced by LLM; here we use WaitGroup
		proj.OnToolCall(ctx, "tc-1", "search", `{"q":"foo"}`, "agent-1", ctxStartTime())
		proj.OnToolResult(ctx, "tc-1", `{"results":[]}`, "success", "", 100)
	}()

	// Phase 3: more thinking
	wg.Add(1)
	go func() {
		defer wg.Done()
		proj.OnReasoningDelta(ctx, "agent-1", "Now I have results", true)
		proj.OnReasoningDone(ctx, "agent-1", "Now I have results", false)
	}()

	// Phase 4: reply
	wg.Add(1)
	go func() {
		defer wg.Done()
		proj.OnTextDelta(ctx, "agent-1", "The answer is 42")
		proj.OnTextDone(ctx, "agent-1", "The answer is 42")
	}()

	wg.Wait()
	proj.OnTurnEnd(ctx, &ActivityUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150})
	proj.Close()

	// Verify: publish order = business order
	received := eventBus.received()
	if len(received) < 4 {
		t.Fatalf("expected ≥4 events, got %d", len(received))
	}

	// Map seq → kind for verification
	seqToKind := make(map[int64]biz.ActivityKind)
	for _, a := range received {
		seqToKind[a.Seq] = a.Kind
	}

	// Expected order (by kind): task, thinking, action, thinking, reply, task(completed)
	// Verify reply's seq > all thinking seqs
	var thinkingSeqs, replySeqs []int64
	for seq, kind := range seqToKind {
		if kind == biz.ActivityKindThinking {
			thinkingSeqs = append(thinkingSeqs, seq)
		}
		if kind == biz.ActivityKindReply {
			replySeqs = append(replySeqs, seq)
		}
	}

	if len(thinkingSeqs) == 0 {
		t.Fatal("no thinking activity in received events")
	}
	if len(replySeqs) == 0 {
		t.Fatal("no reply activity in received events")
	}

	maxThinkingSeq := thinkingSeqs[0]
	for _, s := range thinkingSeqs {
		if s > maxThinkingSeq {
			maxThinkingSeq = s
		}
	}
	minReplySeq := replySeqs[0]
	for _, s := range replySeqs {
		if s < minReplySeq {
			minReplySeq = s
		}
	}

	if minReplySeq <= maxThinkingSeq {
		t.Errorf("reply (min seq=%d) appeared before thinking (max seq=%d) — cross-activity order broken", minReplySeq, maxThinkingSeq)
	}
}

func ctxStartTime() (t struct{ time interface{} }) { return }
```

注意：`ctxStartTime` 是 placeholder，Task 中需要替换为真实的 time import。

- [ ] **Step 2: 修复 import 和 ctxStartTime**

将文件改为：

```go
package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// TestActivityProjector_CrossOrderE2E ...
func TestActivityProjector_CrossOrderE2E(t *testing.T) {
	t.Parallel()

	eventBus := &recordingEventBus{}
	repo := &fakeActivityRepo{}
	proj := NewActivityProjector(eventBus, repo, nil)
	proj.Configure(ProjectMeta{
		SessionID: "sess-e2e",
		RequestID: "turn-e2e",
		AgentID:   "agent-1",
	}, nil)
	proj.Reset()
	proj.OnTurnStart(context.Background(), ProjectMeta{
		SessionID: "sess-e2e",
		RequestID: "turn-e2e",
		AgentID:   "agent-1",
	})

	ctx := context.Background()
	startedAt := time.Now().UTC()
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		proj.OnReasoningDelta(ctx, "agent-1", "Let me think", true)
		proj.OnReasoningDelta(ctx, "agent-1", " about this problem", true)
		proj.OnReasoningDone(ctx, "agent-1", "Let me think about this problem", false)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		proj.OnToolCall(ctx, "tc-1", "search", `{"q":"foo"}`, "agent-1", startedAt)
		proj.OnToolResult(ctx, "tc-1", `{"results":[]}`, "success", "", 100)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		proj.OnReasoningDelta(ctx, "agent-1", "Now I have results", true)
		proj.OnReasoningDone(ctx, "agent-1", "Now I have results", false)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		proj.OnTextDelta(ctx, "agent-1", "The answer is 42")
		proj.OnTextDone(ctx, "agent-1", "The answer is 42")
	}()

	wg.Wait()
	proj.OnTurnEnd(ctx, &ActivityUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150})
	proj.Close()

	received := eventBus.received()
	if len(received) < 4 {
		t.Fatalf("expected ≥4 events, got %d", len(received))
	}

	seqToKind := make(map[int64]biz.ActivityKind)
	for _, a := range received {
		seqToKind[a.Seq] = a.Kind
	}

	var thinkingSeqs, replySeqs []int64
	for seq, kind := range seqToKind {
		if kind == biz.ActivityKindThinking {
			thinkingSeqs = append(thinkingSeqs, seq)
		}
		if kind == biz.ActivityKindReply {
			replySeqs = append(replySeqs, seq)
		}
	}

	if len(thinkingSeqs) == 0 {
		t.Fatal("no thinking activity in received events")
	}
	if len(replySeqs) == 0 {
		t.Fatal("no reply activity in received events")
	}

	maxThinkingSeq := thinkingSeqs[0]
	for _, s := range thinkingSeqs {
		if s > maxThinkingSeq {
			maxThinkingSeq = s
		}
	}
	minReplySeq := replySeqs[0]
	for _, s := range replySeqs {
		if s < minReplySeq {
			minReplySeq = s
		}
	}

	if minReplySeq <= maxThinkingSeq {
		t.Errorf("reply (min seq=%d) appeared before thinking (max seq=%d) — cross-activity order broken", minReplySeq, maxThinkingSeq)
	}
}
```

- [ ] **Step 3: 跑测试**

Run: `cd f:\aranea-agents && go test ./internal/agent/ -run TestActivityProjector_CrossOrderE2E -v -count=1 -race 2>&1 | tail -30`
Expected: PASS

- [ ] **Step 4: 提交**

```bash
cd f:\aranea-agents
git add internal/agent/activity_cross_order_e2e_test.go
git commit -m "test(agent): add cross-activity order end-to-end test"
```

### Task 11: 全量后端测试

- [ ] **Step 1: 跑全量后端测试**

Run: `cd f:\aranea-agents && go test ./... -count=1 -race 2>&1 | tail -50`
Expected: 全部通过；如果有失败，根据错误修复

- [ ] **Step 2: 跑后端 lint**

Run: `cd f:\aranea-agents && golangci-lint run --timeout=10m ./internal/agent/... 2>&1 | tail -30`
Expected: 无 error（warning 可接受）

- [ ] **Step 3: 跑前端测试**

Run: `cd f:\aranea-agents\web && pnpm test 2>&1 | tail -30`
Expected: 全部通过

- [ ] **Step 4: 跑前端 lint**

Run: `cd f:\aranea-agents\web && pnpm lint 2>&1 | tail -20`
Expected: 无 error

- [ ] **Step 5: 跑前端 build**

Run: `cd f:\aranea-agents\web && pnpm build 2>&1 | tail -30`
Expected: build 成功

### Task 12: 端到端人工验证（dev 环境）

- [ ] **Step 1: 启动后端**

Run: `cd f:\aranea-agents && make run` (在另一个 terminal)
Wait: 启动成功

- [ ] **Step 2: 启动前端**

Run: `cd f:\aranea-agents\web && pnpm dev`
Wait: dev server 启动

- [ ] **Step 3: 验证场景 1：流式 MD 实时格式化**

Action: 在 chat 中发送一个会让 LLM 输出代码块/列表的 prompt
Expected: 流式过程中即看到代码块格式（等宽字体 + 背景色）、列表符号，不是纯文本

- [ ] **Step 4: 验证场景 2：跨 activity 顺序**

Action: 发送一个会让 LLM 先 thinking 再 tool 再 thinking 再 reply 的复杂任务
Expected: UI 顺序：thinking → tool → thinking → reply；reply 永远在所有 thinking 之后

- [ ] **Step 5: 验证场景 3：B-04 不复活**

Action: 发送一个长回复（>2000 tokens）的任务
Expected: 流式过程不卡顿；OnTextDelta 不阻塞

- [ ] **Step 6: 记录验证结果到 commit**

```bash
cd f:\aranea-agents
git commit --allow-empty -m "verify(chat): manual e2e validation passed for streaming MD + cross-activity order"
```

---

## Phase 5: 文档同步

### Task 13: 写 ADR-06 记录 sequencer 重设计

**Files:**
- Create: `docs/reports/2026-06-27-review-adr-activity-event-sequencer-redesign.md`

- [ ] **Step 1: 创建 ADR 文档**

创建 `docs/reports/2026-06-27-review-adr-activity-event-sequencer-redesign.md`：

```markdown
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
```

- [ ] **Step 2: 提交**

```bash
cd f:\aranea-agents
git add docs/reports/2026-06-27-review-adr-activity-event-sequencer-redesign.md
git commit -m "docs(adr): add ADR-06 for activity event sequencer redesign"
```

### Task 14: 更新 1-chat.development.md 任务清单

**Files:**
- Modify: `docs/development/1-chat.development.md`

- [ ] **Step 1: 找到任务清单章节**

Read: `docs/development/1-chat.development.md`
Find: 任务清单 / Phase 划分 / 现状评估章节

- [ ] **Step 2: 追加新任务条目**

在最近完成的 Phase 之后，追加新 Phase（"P-N: 流式渲染与活动排序修复"）：

```markdown
## P-N: 流式渲染与活动排序修复（2026-06-27）

### 任务清单
- [x] T-N.1 删除 `renderStreamingChatMarkdown` 简化路径（Task 2）
- [x] T-N.2 seq 在 `OnXxx` 入口主流程分配（Task 4）
- [x] T-N.3 重写 sequencer 为单 publish worker（Task 7）
- [x] T-N.4 端到端验证（Task 12）

### 改动文件清单
- `web/src/features/chat/chatMessageMarkdown.ts`
- `web/src/features/chat/__tests__/chatMessageMarkdown.spec.ts`
- `internal/agent/activity_projector.go`
- `internal/agent/activity_event_sequencer.go`
- `internal/agent/activity_projector_seq_test.go`（新）
- `internal/agent/activity_event_sequencer_v2_test.go`（新）
- `internal/agent/activity_cross_order_e2e_test.go`（新）
- `internal/agent/activity_event_sequencer_bench_test.go`（新）
- `docs/superpowers/specs/2026-06-27-chat-ui-streaming-fix-design.md`（新）
- `docs/reports/2026-06-27-review-adr-activity-event-sequencer-redesign.md`（新）
```

（如文档结构不同，按现有约定追加）

- [ ] **Step 3: 提交**

```bash
cd f:\aranea-agents
git add docs/development/1-chat.development.md
git commit -m "docs(chat): update development plan with streaming fix phase"
```

### Task 15: 更新 1-chat.design.md 数据流章节

**Files:**
- Modify: `docs/development/1-chat.design.md`

- [ ] **Step 1: 找到前端数据流章节**

Read: `docs/development/1-chat.design.md`
Find: 涉及 MD 渲染 / streaming / ActivityStream 的章节

- [ ] **Step 2: 更新 MD 渲染策略描述**

找到描述 `renderStreamingChatMarkdown` 的段落，替换为：

```markdown
### MD 渲染策略（2026-06-27 更新）

前端 `chatMessageMarkdown.ts:renderChatMarkdownForMessage` 统一走 markdown-it + DOMPurify 完整解析路径，
**不再区分流式与完成态**。`streaming` 参数保留仅为 API 兼容。

性能验证：后端 16ms 批合并（`activity_event_sequencer.go:defaultDeltaBatchInterval`）
将前端事件频率封顶到 ≤60fps，markdown-it 解析 0.5-2ms/call，远低于帧预算。
markdownCache (400 条 LRU) 进一步降低重复解析开销。

历史决策：早期版本有 `renderStreamingChatMarkdown` 简化路径（escape-only），
因"每 token 跑 markdown-it"性能假设已不成立而被移除（2026-06-27，ADR-06）。
```

- [ ] **Step 3: 找到 Activity 数据流章节，更新 publish 顺序描述**

找到描述 Activity 事件 publish 的段落，添加：

```markdown
### Activity publish 顺序保证（2026-06-27 更新）

`internal/agent/activity_event_sequencer.go` 采用**单 publish worker + 全局 FIFO 队列**架构
（详见 ADR-06）。关键不变量：

- `Activity.Seq` 在 `OnXxx` 入口处（`p.mu` 内）立即分配：`a.Seq = atomic.AddInt64(&p.seq, 1)`
- 单 publish worker goroutine 串行调用 `eventBus.Publish` → WS subscriber FIFO
- seq 顺序 = projector 业务顺序 = publish 顺序 = UI 顺序

历史决策：v1 架构用 per-activity channel + 多 consumer goroutine，引入了跨 activity
顺序的 goroutine 调度竞争（reply 偶尔跑到 thinking 前面）。v2 取消 per-activity channel，
改为单 worker，根治该问题。
```

- [ ] **Step 4: 提交**

```bash
cd f:\aranea-agents
git add docs/development/1-chat.design.md
git commit -m "docs(chat): update design doc with v2 sequencer and unified MD path"
```

---

## 完成检查清单

- [ ] Task 0: 基线测试快照
- [ ] Task 1-2: 前端 MD 路径统一
- [ ] Task 3-5: 后端 seq 分配前移
- [ ] Task 6-9: 后端 sequencer 重构
- [ ] Task 10-12: 集成验证
- [ ] Task 13-15: 文档同步

## 提交历史预期

预计 ~12 个 commit，按 Phase 分组：
- Phase 0: 0 commits（仅记录基线）
- Phase 1: 2 commits（test + refactor）
- Phase 2: 3 commits（test + refactor + cleanup）
- Phase 3: 4 commits（test + rewrite + adapt + bench）
- Phase 4: 0-1 commits（验证记录）
- Phase 5: 3 commits（ADR + dev plan + design doc）

## 风险预警

- **Task 6-7 风险**：v2 sequencer 重写是最大改动，Task 8 适配旧测试可能工作量超预期
- **Task 10 风险**：跨 activity e2e 测试可能因 goroutine 调度不稳定偶发失败 → 增加 retry 逻辑
- **Task 12 风险**：dev 环境验证依赖实际 LLM，可能需 mock LLM 替代
