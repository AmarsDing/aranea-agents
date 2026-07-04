# 空 ReplyStep 清理与多轮 Step 模型澄清

> **设计稿** · 2026-07-04 · 状态：可实施
>
> **范围**：修复 chat 模块"空回复块"显示问题；澄清 turn 内 step 模型为多轮模式
> **关联 spec**：[2026-07-02-llm-activity-ordering-design.md](./2026-07-02-llm-activity-ordering-design.md) §3.2.1

---

## 一、背景与问题

### 1.1 用户反馈

浏览器中观察到多个空的 ReplyBlock（`<div class="reply-block__markdown">` 内容为空）。用户怀疑设计上"定死了思考-act-回复"三步模型，不符合实际业务模式。

### 1.2 根因

#### 根因 A：后端过早创建 ReplyStep

[`internal/agent/v2/projector.go:790-795`](../../internal/agent/v2/projector.go) `handleTextDelta`：

```go
func (p *ActivityProjector) handleTextDelta(ctx context.Context, delta string) {
    if p.replyStepID == "" {
        p.replyStepID = p.BeginStep(p.meta, biz.StepKindReply)  // ← 第一次 delta 立即创建
    }
    p.OnTextDelta(ctx, p.replyStepID, delta, "")
}
```

第一次 text delta 到达时立即创建 ReplyStep，**未判断 delta 是否纯空白**。LLM 框架常发出 `\n`、空格等引导空白作为 delta → 创建空 ReplyStep → 后续若未再产出实质内容，`handleTextDone` 的 `finalContent` 也是空白 → 留下空 ReplyStep。

#### 根因 B：空 ReplyStep 未清理

[`projector.go:814-824`](../../internal/agent/v2/projector.go) `handleTextDone`：

- 若 `replyStepID == ""` 且 `finalContent` 为空：no-op（OK）
- 若 `replyStepID != ""`（已创建）但 `step.Content` 与 `finalContent` 均为空白：仍走 `OnTextDone → completeStep` 路径，发布 `step.completed` 事件，留下空 ReplyStep（**BUG**）

#### 根因 C：spec §3.2.1 图示误导

[`2026-07-02-llm-activity-ordering-design.md` §3.2.1](./2026-07-02-llm-activity-ordering-design.md) 图示：

```
Turn (turn_id, seq)
├── ThinkingStep (seq=1)
├── ActionStep   (seq=2)
├── ReplyStep     (seq=3, is_final)
```

让人误以为固定三步。实际实现是**懒创建 + 多轮支持**：`thinkingStepID`/`replyStepID` 在 Done 时清空，下一次 delta 会创建新 step。所以底层已支持 `thinking → action → thinking → action → ... → reply` 模式，但 spec 文档未澄清。

#### 根因 D：前端无兜底过滤

[`TurnContainer.vue:29-31`](../../web/src/components/chat/v2/TurnContainer.vue) `visibleSteps` 只过滤 system notice，未过滤空 reply step。后端遗漏场景无防线。

---

## 二、设计目标

1. **不再创建空 ReplyStep**：LLM 输出纯空白 delta 时不创建 ReplyStep
2. **已创建的空 ReplyStep 走 cancelled 路径**：前端可识别并过滤
3. **前端兜底**：过滤空 ReplyStep，防止后端遗漏场景
4. **spec 澄清**：明确 turn 内 step 模型为多轮模式
5. **不破坏现有 reply 行为**：流式期间 ReplyStep 仍正常显示

## 三、非目标

- 不重构 step 模型（保留 6 种 kind）
- 不引入新事件类型（复用 `StepCompletedEvent` + `Status=cancelled`）
- 不修改 thinking step 创建逻辑（thinking 可空，已有 no-op 路径）

---

## 四、设计方案

### 4.1 后端修改 — `internal/agent/v2/projector.go`

#### 4.1.1 `handleTextDelta` 防过早创建

第一次 text delta 到达时，trim 后为空则**不创建** ReplyStep（直接 return）。LLM 框架的纯空白引导 delta 会被丢弃；一旦后续 delta 含实质内容，再创建 step 并正常累积（包括后续空白）。

```go
func (p *ActivityProjector) handleTextDelta(ctx context.Context, delta string) {
    if p.replyStepID == "" {
        // 防止 LLM 输出引导空白（"\n", " "）导致创建空 ReplyStep。
        // 仅在 delta 含非空白字符时才创建 step；纯空白 delta 丢弃。
        if strings.TrimSpace(delta) == "" {
            return
        }
        p.replyStepID = p.BeginStep(p.meta, biz.StepKindReply)
    }
    p.OnTextDelta(ctx, p.replyStepID, delta, "")
}
```

#### 4.1.2 `handleTextDone` 清理空 ReplyStep

若 `replyStepID` 已存在但 `step.Content` 与 `finalContent` 均为空白，发布 `StepCompletedEvent`（status=cancelled, is_final=false），不进入正常 completed 路径。

```go
func (p *ActivityProjector) handleTextDone(ctx context.Context, finalContent string) {
    if p.replyStepID == "" {
        if strings.TrimSpace(finalContent) == "" {
            return  // 无 reply step 且 finalContent 为空：no-op
        }
        p.replyStepID = p.BeginStep(p.meta, biz.StepKindReply)
    }
    // 检查 step 是否为空（已创建但 Content 为空且 finalContent 也为空）
    p.mu.Lock()
    step, ok := p.activeStep[p.replyStepID]
    isBlank := ok && strings.TrimSpace(step.Content) == "" && strings.TrimSpace(finalContent) == ""
    if isBlank {
        // 空 reply 取消而非完成，前端可按 status=cancelled 过滤
        now := time.Now()
        step.Status = biz.StepStatusCancelled
        step.IsFinal = false
        step.CompletedAt = &now
        step.Version++
        delete(p.activeStep, p.replyStepID)
    }
    p.mu.Unlock()
    if isBlank {
        p.seq.Publish(ctx, biz.NewStepCompletedEvent(*step))
        p.replyStepID = ""
        return
    }
    p.OnTextDone(ctx, p.replyStepID, finalContent, true)
    p.replyStepID = ""
}
```

**事件选择说明**：复用 `NewStepCompletedEvent`（与 `EmitConfirmResult` denied 路径一致，参见 [projector_test.go:168-174](../../internal/agent/v2/projector_test.go#L168-174)）。语义：step 走完生命周期但被取消，不是失败。

### 4.2 前端兜底 — `web/src/components/chat/v2/TurnContainer.vue`

`visibleSteps` 增加过滤规则：

```ts
const visibleSteps = computed(() =>
  store.getTurnSteps(props.turn.ID).filter((s) => {
    // 过滤系统内部通知
    if (isSystemInternalNotice(s.Kind, s.NoticeType)) return false;
    // 过滤空 reply step（cancelled 或 completed 但 content 为空）
    if (s.Kind === 'reply' && !s.Content?.trim() && s.Status !== 'running') {
      return false;
    }
    return true;
  }),
);
```

**注意**：`Status === 'running'` 的 reply step 仍显示（流式中），避免流式期间被隐藏导致用户看不到正在生成。

### 4.3 spec 文档更新 — `2026-07-02-llm-activity-ordering-design.md` §3.2.1

把固定三步图示改为多轮模式：

```
Turn (turn_id, seq)         ← 最小对话单元
├── ThinkingStep? (seq, 可多轮，0..N)
├── ActionStep?   (seq, 可多轮，0..N)
├── ... (thinking/action 交替，按 Seq 排序)
├── ReplyStep?     (seq, is_final=true, 0..1 个)
├── NoticeStep?    (seq, 0..N)
├── ConfirmStep?   (seq, 0..N)
├── ErrorStep?     (seq, 0..1)
├── TeamStage?     ← turn 内触发 team 执行
│   └── TeamRun (run_id, dag_node_id=plan_step.id)
│       └── MemberSession (agent_key)
│           └── Turn (member 自己的 turn, seq)
│               └── ThinkingStep? / ActionStep? / ReplyStep?
└── (并行其他 TeamStage)
```

明确说明：
- **实际 LLM 业务模式**：`thinking → action → thinking → action → ... → reply`
- turn 内可有 0..N 个 thinking/action step（按 Seq 排序）
- turn 内可有 0..1 个 final reply step（`is_final=true`）
- **ReplyStep 仅在 LLM 输出非空文本时创建**：纯空白 delta 不创建；已创建但内容为空的 step 走 cancelled 路径

---

## 五、测试计划

### 5.1 后端单测（`internal/agent/v2/projector_test.go`）

| 测试 | 场景 | 期望 |
|------|------|------|
| `TestHandleTextDelta_WhitespaceNoCreate` | 第一次 delta="\n"，第二次 delta="Hello" | 仅在第二次创建 ReplyStep，最终 content 含 "Hello" |
| `TestHandleTextDone_EmptyContentCancelled` | delta=" "（已创建 step）+ finalContent="" | 发布 `StepCompletedEvent`，status=cancelled |
| `TestHandleTextDone_NormalReply` | delta="Hi" + finalContent="Hi" | 正常 `StepCompletedEvent`，status=completed, is_final=true |
| `TestHandleTextDone_NoReplyStep_NoOp` | 无 delta + finalContent="" | 无任何事件 |

### 5.2 前端单测（`web/src/components/chat/__tests__/TurnContainer.spec.ts`）

| 测试 | 场景 | 期望 |
|------|------|------|
| `filters empty completed reply step` | step.Kind='reply', Content='', Status='completed' | 不渲染 ReplyBlock |
| `filters empty cancelled reply step` | step.Kind='reply', Content='', Status='cancelled' | 不渲染 ReplyBlock |
| `keeps running empty reply step` | step.Kind='reply', Content='', Status='running' | 渲染 ReplyBlock（流式中） |
| `keeps non-empty reply step` | step.Kind='reply', Content='Hi' | 渲染 ReplyBlock |

---

## 六、文档同步

按 DOC-SYNC 规则：
- 本设计稿新建（`2026-07-04-empty-reply-step-cleanup-design.md`）
- 同步更新 `2026-07-02-llm-activity-ordering-design.md` §3.2.1 图示为多轮模式
- 无 RPC/Schema 变更，无需更新其他文档

---

## 七、改动文件清单

| 文件 | 改动 |
|------|------|
| `internal/agent/v2/projector.go` | `handleTextDelta` + `handleTextDone` 修改 |
| `internal/agent/v2/projector_test.go` | 新增 4 个测试 |
| `web/src/components/chat/v2/TurnContainer.vue` | `visibleSteps` 过滤规则 |
| `web/src/components/chat/__tests__/TurnContainer.spec.ts` | 新建测试文件 |
| `docs/superpowers/specs/2026-07-02-llm-activity-ordering-design.md` | §3.2.1 图示更新 |
| `docs/superpowers/specs/2026-07-04-empty-reply-step-cleanup-design.md` | 本设计稿 |
