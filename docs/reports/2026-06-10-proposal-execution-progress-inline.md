# Proposal: Chat 树形时间线 + 编排进度内联（统一 AgentTreeTimeline + execution_progress）

> **日期**：2026-06-10 | **版本**：v0.2 | **状态**：设计已定 · P0 实施中
> **作者上下文**：用户反馈单 Agent / Team 会话在 LLM 返回前需静默等待 ~20s，编排进度不可见
> **决策记录**：用户选择"全量重构 + 顺便验证单 agent 树形化"作为 P0 范围
> **前置报告**：[2026-06-10-proposal-chat-compact-timeline.md](./2026-06-10-proposal-chat-compact-timeline.md) M1 时间线
> **关联需求**：[59-chat-ui-optimization.md](../development/59-chat-ui-optimization.md)（合并自原 M59 + M69）
> **关联 SKILL**：`aranea-frontend-guide` §3 数据流 · §6 UX 主题 · `aranea-coding-guide` §6 Agent 运行时

---

## 摘要

项目里**已经存在**树形时间线架构（`AgentBlock` / `AgentTreeTimeline`），但被 team-only 条件锁住，单 agent 仍走老的 `TurnBlock`。用户原话：

> 学习 Trae，树形结构。每个 agent 的会话都是一个**带时间线的框体**，内容：`思考-工具-回复-思考-工具-回复`，**树形结构**指主 agent 会话中**嵌套了子 agent 的会话**，每个 agent 会话输出的**信息结构相同**。

本提案**全量重构**：

1. **`AgentTreeTimeline` 成为唯一主时间线**（替代 `TurnBlock` 在单 agent 场景）
2. **`TimelineEntry` union 扩展**为 5 种 kind：thinking / tool / **reply（新增）** / **progress（新增）** / subagent
3. **新增 `execution_progress` envelope**，由后端 15-18 个细步骤 emit，前端内联到 AgentBlock 时间线
4. **持久化** progress 节点到 `message.options_json.metadata.progress_nodes`
5. **隐藏开关** localStorage 持久化用户偏好（full / compact / hidden）

M1 的 `CompactTimeline` 工作不被破坏，作为 AgentBlock 内部 timeline 渲染器复用。

---

## 一、问题回顾

### 1.1 用户痛点

> "发送指令后需要等待很长时间大约 20 多 s，才回复，主要时间在编排上，能否将编排的信息内容也展示到前端。"

20 秒不是 LLM 推理本身，而是**编排前置阶段 + LLM 首字节**。在 `chat_orchestrator_turn.go` 中可分解为：

| 阶段 | 典型耗时 | 当前是否可见 |
|------|---------|------------|
| 加载会话 / Agent / Provider | 50-200ms | ❌ |
| 加载插件（10-20 个 callback） | 200-800ms | ❌ |
| 构建 Runner | 200-500ms | ❌ |
| **意图识别（Intent Pass）** | **1-3s** | ❌ |
| 用户消息持久化 | 50-200ms | ❌ |
| **首字节等待（LLM 思考）** | **3-15s** | ⚠️ 仅在 `thinking` 出现后 |
| 流式输出 + 工具调用 + 回复 | 后续 | ✅ 可见 |
| 助手消息持久化 | 50-200ms | ❌ |

**结论**：20s 中至少有 5-8s 完全静默，5-15s 在等 LLM 首字节时仅显示 composer 输入框的发送动画。

### 1.2 现有系统的局限

```
┌──────────────────────────────────────────────────────────┐
│  Trace/Flow 系统（TraceEmitter + FlowTracker）            │
│  - 12+ 个 chat.* 步骤（receiver → provider → runner →    │
│    intent → llm.invoke → stream → persist）              │
│  - 走 EnvelopeTypeFlowLog → MonitorBus（默认 split）     │
│  - 前端完全不订阅（仅 MonitorPage 看）                    │
└──────────────────────────────────────────────────────────┘
                          ↕ 不互通
┌──────────────────────────────────────────────────────────┐
│  Spirit Orchestration 系统（Spirit 模式专用）              │
│  - butler.orchestration.started/completed/failed         │
│  - spirit_plan_created / allocation_created /            │
│    orchestration_started / checkpoint                    │
│  - 走 SessionBus（chat channel）                         │
│  - 前端 useContextualLoadingMessage 消费                  │
│  - 渲染条件：panelMode === 'spirit'                      │
└──────────────────────────────────────────────────────────┘
```

**问题**：
1. 单 Agent 模式没有 spirit → 完全无编排可见性
2. Team 模式（非 spirit）有 TeamRunStarted 等事件但未内联渲染
3. 两套系统的事件格式、命名空间、消费路径都不一致 → 扩展困难

---

## 二、重新设计：统一 execution_progress 流

### 2.1 核心决策

**新增而非替换**：
- ✅ 新增 `EnvelopeTypeExecutionProgress = "execution_progress"`
- ✅ 走 `chat` channel 路由（SessionBus）
- ✅ 由编排器在关键步骤**额外** emit 一次（保留原 `flow_log` 走 monitor）
- ✅ 与现有 `spirit_*` envelope 并存（不取代，spirit UI 走自己的渲染路径）

### 2.2 Envelope 契约

```typescript
type ExecutionProgressEnvelope = {
  type: 'execution_progress';
  author: 'system';
  session_id: string;
  turn_id: string;
  metadata: {
    /** 唯一步骤 ID，用于 dedup + 关联 start/done/error */
    step_id: string;
    /** 步骤状态机 */
    phase: 'start' | 'done' | 'error';
    /** 面向用户的中文短句，≤ 24 字 */
    message: string;
    /** phase=done 时填，毫秒 */
    duration_ms?: number;
    /** 分类：用于颜色/图标 */
    category: 'orchestration' | 'team' | 'tool' | 'thinking';
    /** 可选：关联 agent / tool */
    agent_key?: string;
    tool_name?: string;
  };
  timestamp: string;
};
```

### 2.3 后端实施

| 文件 | 改动 |
|------|------|
| `internal/event/contract/envelope.go` | 新增 `EnvelopeTypeExecutionProgress`，注册到 `chat` channel |
| `internal/event/trace_emitter.go` | 新增 `EmitProgress(stepID, phase, message, category)` 便捷方法 |
| `internal/service/chat_orchestrator_turn.go` | 8-10 个关键步骤改用 `EmitProgress`（保留 `LogStart/LogDone` 走 monitor） |
| `internal/agent/team_orchestrator*.go` | team step started/finished emit 一次 |

**关键步骤清单（v1 范围）**：

| 步骤 ID | 中文 message | category | 典型耗时 |
|---------|------------|----------|---------|
| `chat.session_fetch` | 加载会话 | orchestration | <0.1s |
| `chat.agent_hydrate` | 加载 Agent | orchestration | <0.1s |
| `chat.provider_resolve` | 解析模型 | orchestration | <0.1s |
| `chat.plugins_load` | 加载插件 | orchestration | 0.2-0.5s |
| `chat.runner.create` | 构建 Runner | orchestration | 0.2-0.5s |
| `chat.intent.pass` | 意图识别 | orchestration | 1-3s |
| `chat.user_msg_persist` | 保存消息 | orchestration | <0.1s |
| `chat.llm.invoke` | **调用语言模型** | orchestration | **3-15s** |
| `chat.stream.consume` | 处理模型输出 | orchestration | 后继 |
| `team.step.started` | 团队步骤 | team | 1-30s |
| `team.step.finished` | 团队步骤完成 | team | - |

**emit 模式**（每个步骤两次）：
```go
emitter.EmitProgress("chat.intent.pass", "start", "意图识别中…", "orchestration")
// ... 实际处理 ...
emitter.EmitProgress("chat.intent.pass", "done", "意图识别完成", "orchestration") // 自动计算 duration_ms
```

### 2.4 前端实施

| 文件 | 改动 |
|------|------|
| `web/src/realtime/envelope.ts` | 加 `execution_progress` 到 `EnvelopeType` union |
| `web/src/features/chat/streamHandlers.ts` | 注册 `stream.onType('execution_progress', ...)` |
| `web/src/features/chat/executionProgress.ts` | **新增**：纯函数 `ExecutionProgressNode` + `mergeProgressEvents(events): ProgressNode[]` |
| `web/src/components/chat/ExecutionProgressCard.vue` | **新增**：内联步骤卡（spinner / ✓ / ✕ + 步骤名 + 耗时） |
| `web/src/components/chat/CompactTimeline.vue` | 接受 `progress` 节点，与 thinking/tools/reply 同级渲染 |
| `web/src/features/chat/compactTimeline.ts` | 扩展 `CompactNode` union 加 `progress` 变体 |
| `web/src/features/chat/composables/useChatWorkspace.ts` | 维护 `progressNodes: ComputedRef<ProgressNode[]>` |
| `web/src/features/chat/__tests__/executionProgress.spec.ts` | **新增**：单测覆盖 start/done/error 合并 |

### 2.5 渲染目标态

**单 Agent 等待中（关键场景）**：

```
[User] 帮我分析上月销售

[Assistant Turn]
  💭 思考                          · 0.4s
  ⏳ 加载会话                       · 完成 · 80ms
  ⏳ 加载 Agent                     · 完成 · 60ms
  ⏳ 解析模型                       · 完成 · 40ms
  ⏳ 加载插件                       · 完成 · 320ms
  ⏳ 构建 Runner                    · 完成 · 280ms
  ⏳ 意图识别中…                    · 进行中  ← spinner
  ⏳ 调用语言模型中…                · 等待中  ← 静默时显示这个
  💬 回复                          · 2.1s
      上月销售总额 XXX，同比增长 XX%…
```

**Team 模式（Spirit / 普通）**：

```
[User] 帮我做一个完整的故障复盘

[Assistant Turn]
  💭 思考                          · 0.4s
  ⏳ 意图识别                       · 完成 · 1.2s
  ⏳ 编排计划                       · 完成 · 0.5s
  ⏳ 分配 Agent                     · 完成 · 0.3s
  ⏳ 协调者调用                     · 进行中 · 3.2s
  🔧 get_sales                     · 完成 · 1.2s
  ⏳ 协调者继续                     · 等待中
  💬 回复                          · 2.1s
```

### 2.6 与现有系统的关系

| 现有系统 | 处理 |
|---------|------|
| `flow_log` envelope | **完全不动**，继续走 MonitorBus |
| `spirit_*` / `butler.orchestration.*` | **保留**，spirit UI 仍用 `useContextualLoadingMessage` |
| `run_status` envelope | **保留**，作超时/失败兜底 |
| `useContextualLoadingMessage` | 保留 spirit 加载条；execution_progress 走新组件 |
| `CompactTimeline` (M1) | **扩 union**，加 `progress` 变体 |
| `onRunStatus` | 不变 |
| ReAct 模式 | **不变**，不引入进度卡 |

### 2.7 性能与可观测性

- **频率**：每步骤 2 个 envelope（start + done），整轮最多 ~22 个 → WS 带宽增加 < 1KB，可忽略
- **降级**：超过 5s 仍处于 `start` 状态的步骤，前端自动从"进行中"切到"等待中"（提示用户）
- **错误**：phase=error 时渲染为 ⚠️ 红框，hover 显示原始 message
- **持久化**：v1 不持久化（仅本次会话内可见），后续可考虑写入 message metadata

---

## 三、实施分阶段

| 阶段 | 内容 | 涉及文件数 | 可独立验证 |
|------|------|----------|----------|
| **P0 骨架** | envelope 类型 + 路由 + handler 骨架 + 1 个端到端步骤（chat.llm.invoke） | 4 新 + 3 改 | ✅ |
| **P1 单 Agent 完整化** | 8-10 个 chat.* 步骤全部 emit | 2 改 | ✅ |
| **P2 Team 模式** | team step started/finished emit + 前端渲染 | 4 改 | ✅ |
| **P3 UI 打磨** | spinner 动画、超时降级、错误卡样式 | 1 改 | ✅ |
| **P4 可观测性** | duration 累计、慢步骤告警（>10s） | 1 改 | ✅ |

**P0 验证标准**：
1. 单 Agent 会话，等待 5s 后看到"调用语言模型中…进行中"
2. LLM 返回后 1s 内看到"调用语言模型中…完成 · 5.2s"
3. 单元测试 `mergeProgressEvents` 覆盖 start/done/error/超时场景
4. 不破坏现有 spirit UI / ReAct 模式 / 现有单测

---

## 四、风险与决策记录

| 风险 | 缓解 |
|------|------|
| 与 spirit_* 重复导致"两条线" | spirit UI 仍优先 spirit_* 渲染；progress 节点仅在 non-spirit 出现 |
| WS 流量增加 | 每个 envelope < 200B，整轮 < 5KB，远低于现有 1MB+ 的 stream |
| start/done 配对失败 | 前端 `mergeProgressEvents` 容忍孤儿：start 5s 后未 done 自动 done 状态=timeout |
| 改动 `CompactTimeline` 影响 M1 工作 | 纯 union 扩展，零破坏，14 个 M1 单测不受影响 |
| 后端 emit 散落在 10+ 处 | 集中在 `TraceEmitter.EmitProgress` 单一 API；调用点仅需一行 |

---

## 五、参考实现代码（P0 骨架示意）

### 后端：`internal/event/trace_emitter.go` 新增方法

```go
// EmitProgress emits a chat-visible execution progress envelope.
// Independent from flow_log (which still goes to monitor bus).
func (e *TraceEmitter) EmitProgress(stepID, phase, message, category string, extra ...Pair) {
    if e == nil || e.infra == nil {
        return
    }
    env := NewEnvelope(EnvelopeTypeExecutionProgress, "system", e.tc.SessionID)
    env.Metadata = map[string]any{
        "step_id": stepID,
        "phase":   phase,  // "start" | "done" | "error"
        "message": message,
        "category": category,
    }
    for _, p := range extra {
        env.Metadata[p.Key] = p.Value
    }
    if phase == "done" {
        if timing := e.fc.TakeTiming(stepID); timing != nil {
            env.Metadata["duration_ms"] = timing.DurationMS
        }
    } else if phase == "start" {
        e.fc.RecordStart(stepID)
    }
    e.infra.SessionBus.Publish(context.Background(), env)
}
```

### 前端：`web/src/features/chat/executionProgress.ts`

```typescript
export type ProgressCategory = 'orchestration' | 'team' | 'tool' | 'thinking';

export type ProgressNode = {
  stepId: string;
  category: ProgressCategory;
  message: string;
  status: 'running' | 'done' | 'failed' | 'timeout';
  startedAt: number;
  durationMs?: number;
};

export function mergeProgressEvents(
  events: ExecutionProgressEnvelope[],
  now: () => number = Date.now,
): ProgressNode[] {
  const byStep = new Map<string, ProgressNode>();
  for (const env of events) {
    const meta = env.metadata;
    const stepId = String(meta.step_id);
    const phase = String(meta.phase);
    const ts = Date.parse(env.timestamp);
    const existing = byStep.get(stepId);
    if (phase === 'start') {
      byStep.set(stepId, {
        stepId,
        category: meta.category as ProgressCategory,
        message: String(meta.message),
        status: 'running',
        startedAt: ts,
      });
    } else if (phase === 'done' && existing) {
      existing.status = 'done';
      existing.message = String(meta.message);
      existing.durationMs = Number(meta.duration_ms ?? ts - existing.startedAt);
    } else if (phase === 'error' && existing) {
      existing.status = 'failed';
      existing.message = String(meta.message);
    }
  }
  // Auto-timeout: start > 5s without done
  for (const node of byStep.values()) {
    if (node.status === 'running' && now() - node.startedAt > 5_000) {
      node.status = 'timeout';
    }
  }
  return Array.from(byStep.values());
}
```

### 前端：`CompactNode` union 扩展

```typescript
// web/src/features/chat/compactTimeline.ts
export type CompactNode =
  | { kind: 'thinking'; text: string; messageId: string }
  | { kind: 'tool'; event: ToolUseEvent }
  | { kind: 'progress'; node: ProgressNode }   // ← 新增
  | { kind: 'reply'; text: string; messageId: string; streaming: boolean; status: 'ok' | 'failed' | 'cancelled' | 'streaming' };
```

---

## 六、待评审问题（已确认决策）

| # | 问题 | 决策 |
|---|------|------|
| 1 | 步骤粒度 | **拆到 15-18 个细步骤**（callback 链、变量合并、toolcall 序列化等） |
| 2 | 持久化 | **写入 message metadata**（`options_json.metadata.progress_nodes`） |
| 3 | Spirit 模式是否显示 progress | **学习 Trae 树形结构** — 单一时间线架构，spirit 模式也走 AgentTreeTimeline（progress 节点在 spirit UI 中作为补充信息显示） |
| 4 | 是否加"隐藏编排进度"开关 | **加隐藏开关**（localStorage 持久化：full / compact / hidden） |
| 5 | phase=error 时是否弹 toast | **仅时间线卡内 ⚠️**，不弹 toast（避免噪音） |

---

## 七、树形时间线设计（Trae 风格）

### 7.1 核心隐喻：每个 Agent = 一个"框体"

无论主 agent 还是子 agent，**信息结构完全相同**：

```
┌─ 🤖 {agent_name} ───────────────────────────────────────┐
│  Task: {task}                                            │
│  Status: {status} · {duration}                           │
│                                                            │
│  timeline (按时间序):                                      │
│    💭 思考          · 0.4s                                 │
│    🔧 工具1         · ✓ 0.8s                                │
│    💬 回复          · 1.5s                                  │
│    💭 思考          · 0.3s                                 │
│    ⏳ 编排步骤     · ✓ 0.5s  (来自 execution_progress)     │
│    ⏳ 调用 LLM     · 等待中 (进行中)                       │
│    💬 回复          · 2.1s                                  │
│                                                            │
│  [optional 子 agent 嵌套, 每个子框体结构相同]              │
└────────────────────────────────────────────────────────────┘
```

**单 agent 场景**：整个对话只有 1 个 root 框体，没有子 agent。
**Team/Spirit 场景**：root 框体嵌套多个 sub-agent 框体。
**ReAct 模式**：保留 step 卡片（不强行套用本设计）。

### 7.2 TimelineEntry 五种 Kind

```typescript
// web/src/features/chat/agentTreeTypes.ts
export type TimelineEntry =
  | { kind: 'thinking'; section: ThinkingSection; sortKey: number }
  | { kind: 'tool'; section: ToolSection; sortKey: number }
  | { kind: 'reply'; section: ReplySection; sortKey: number }      // ← 新增
  | { kind: 'progress'; section: ProgressSection; sortKey: number } // ← 新增
  | { kind: 'subagent'; block: AgentBlock; sortKey: number };

export interface ReplySection {
  id: string;
  content: string;
  durationMs: number | null;
  streaming: boolean;
}

export interface ProgressSection {
  id: string;          // step_id, e.g. "chat.llm.invoke"
  category: 'orchestration' | 'team' | 'tool' | 'thinking';
  message: string;     // 面向用户的中文短句
  status: 'running' | 'done' | 'failed' | 'timeout';
  durationMs: number | null;
  startedAt: number;
}
```

### 7.3 完整目标态（Team 编排场景）

```
[User] 帮我做一个完整的故障复盘

┌─ 🤖 Coordinator (spirit-orchestrator) ─────────────────────┐
│ Task: 帮我做一个完整的故障复盘                                │
│ Status: running · 8.4s                                       │
│                                                               │
│ 💭 思考          · 0.4s                                       │
│ ⏳ 加载会话      · ✓ 0.08s                                    │
│ ⏳ 加载 Agent    · ✓ 0.06s                                    │
│ ⏳ 解析模型      · ✓ 0.04s                                    │
│ ⏳ 加载插件      · ✓ 0.32s                                    │
│ ⏳ 构建 Runner   · ✓ 0.28s                                    │
│ ⏳ 意图识别      · ✓ 1.2s                                     │
│ ⏳ 保存消息      · ✓ 0.10s                                    │
│ ⏳ 调用 LLM      · 等待中 · 3.2s                              │
│                                                               │
│ ┌─ 🤖 Architect (后端) ──────────────────────────────────┐   │
│ │ Task: 定位后端故障                                       │   │
│ │ Status: running · 2.1s                                    │   │
│ │                                                           │   │
│ │ 💭 思考       · 0.3s                                      │   │
│ │ ⏳ 调用 LLM   · ✓ 1.0s                                    │   │
│ │ 🔧 get_logs   · ✓ 0.8s                                    │   │
│ │ 💬 回复       · 1.5s                                       │   │
│ │     故障位于订单服务 X 处，原因是…                       │   │
│ └──────────────────────────────────────────────────────────┘   │
│                                                               │
│ 💬 汇总回复       · 2.1s                                       │
│     综合两份子报告，故障原因是…                              │
└───────────────────────────────────────────────────────────────┘
```

### 7.4 与 M1 CompactTimeline 的关系

`CompactTimeline` (M1) 是 AgentBlock 内部 timeline 渲染器**的原型**。本提案将其复用：

| 组件 | 角色 |
|------|------|
| `AgentTreeTimeline.vue` | 树形容器（嵌套 AgentBlock） |
| `AgentBlock.vue` | 单个框体（包含 timeline 渲染） |
| `CompactTimeline.vue` | AgentBlock 内部 timeline 的具体渲染器（thinking + tool + reply） |
| `ExecutionProgressCard.vue` (新) | AgentBlock 内部 timeline 的 progress 节点渲染器 |

M1 的 14 个单测继续通过，零破坏。

### 7.5 15-18 个细步骤清单

| # | step_id | 中文 message | category | 典型耗时 |
|---|---------|------------|----------|---------|
| 1 | `chat.session_fetch` | 加载会话 | orchestration | <0.1s |
| 2 | `chat.agent_hydrate` | 加载 Agent | orchestration | <0.1s |
| 3 | `chat.provider_resolve` | 解析模型 | orchestration | <0.1s |
| 4 | `chat.variables_parse` | 解析变量 | orchestration | <0.1s |
| 5 | `chat.plugins_load` | 加载插件 | orchestration | 0.2-0.5s |
| 6 | `chat.callbacks_chain_build` | 构建回调链 | orchestration | 0.1-0.3s |
| 7 | `chat.runner.create` | 创建 Runner | orchestration | 0.2-0.5s |
| 8 | `chat.intent.pass` | 意图识别 | orchestration | 1-3s |
| 9 | `chat.context_compress` | 上下文压缩 | orchestration | 0.1-0.5s |
| 10 | `chat.user_msg_persist` | 保存用户消息 | orchestration | <0.1s |
| 11 | `chat.llm.invoke` | **调用语言模型** | orchestration | **3-15s** |
| 12 | `chat.tool_call_dispatch` | 工具调用派发 | orchestration | <0.1s |
| 13 | `chat.tool_call_execute` | 工具执行 | tool | 0.5-5s |
| 14 | `chat.tool_call_serialize` | 工具结果序列化 | orchestration | <0.1s |
| 15 | `chat.stream.consume` | 处理流输出 | orchestration | 持续 |
| 16 | `chat.assistant_msg_persist` | 保存助手消息 | orchestration | <0.2s |
| 17 | `chat.usage_record` | 用量记录 | orchestration | <0.1s |
| 18 | `team.member_dispatch` | 团队成员派发 | team | 1-30s |
| 19 | `team.member_collect` | 团队结果收集 | team | - |

### 7.6 持久化设计

```typescript
// message.options_json (写入 user/assistant message)
{
  "agent_block_metadata": {
    "root_agent_key": "spirit-orchestrator",
    "progress_nodes": [
      { "step_id": "chat.llm.invoke", "category": "orchestration", "message": "调用语言模型", "status": "done", "duration_ms": 5200, "started_at": 1718000000000 }
    ]
  }
}
```

- v1 范围：仅 root agent 的 progress_nodes 持久化
- 加载历史消息时回放 progress 节点
- 写入时机：`runner_completion` 时一次性写入

### 7.7 隐藏开关

- 位置：ChatPage 头部"思考面板"按钮旁，加 `timeline_visibility` 切换
- 存储：`localStorage['chat.timeline.visibility']` = `'full' | 'compact' | 'hidden'`
- 行为：
  - `full`：完整显示（默认）
  - `compact`：折叠所有 progress 节点
  - `hidden`：隐藏所有非必要的 progress/reply（仅 thinking/tool/subagent）

---

## 八、实施分阶段（最终）

| 阶段 | 内容 | 涉及文件数 | 验证 |
|------|------|----------|------|
| **P0 骨架**（本次） | TimelineEntry 扩 union（reply/progress）+ AgentBlock 渲染器接入 + 1 个进度步骤（chat.llm.invoke）+ **useAgentBlocks 解除 team 限制** | 5 后端 + 7 前端 | ✅ 单测 + 构建 |
| **P1 完整步骤流** | 15-18 个步骤全量 emit + 前端合并 | 5 后端 + 3 前端 | ✅ |
| **P2 UI 打磨** | 树形缩进、连接线、collapse/expand、嵌套 subagent 渲染 | 4 前端 | ✅ |
| **P3 持久化** | 写入 message metadata + 历史回放 | 3 前端 | ✅ |
| **P4 隐藏开关** | localStorage 切换 + UI 入口 | 2 前端 | ✅ |
| **P5 清理** | 移除 `TurnBlock` 老路径依赖 | 2 前端 | ✅ |

**P0 验收标准**（用户确认）：
1. TypeScript 类型编译通过
2. 后端 `chat.llm.invoke` 步骤 emit progress (start + done)
3. 前端 AgentBlock 时间线显示 progress 节点
4. **单 agent 会话也走 AgentTreeTimeline**（解除 team 限制）
5. 不破坏现有 spirit UI / ReAct 模式 / 现有 M1 单测

---

## 九、关联材料

- [M1 紧凑时间线报告](./2026-06-10-proposal-chat-compact-timeline.md) — 已有 CompactTimeline 基础
- [agentTreeTypes.ts](../../web/src/features/chat/agentTreeTypes.ts) — 现有 AgentBlock / TimelineEntry 类型
- [useAgentBlocks.ts](../../web/src/features/chat/composables/useAgentBlocks.ts) — 现有树构建（team-only）
- [useContextualLoadingMessage.ts](../../web/src/features/chat/composables/useContextualLoadingMessage.ts) — spirit 加载条参考实现
- [observabilityConstants.ts](../../web/src/features/spirit/observabilityConstants.ts) — ORCHESTRATION_LOADING_MAP 模板
- [chat_orchestrator_turn.go](../../internal/service/chat_orchestrator_turn.go) — 后端 12+ 个编排步骤源点
- [infra.go:106-116](../../internal/event/infra.go) — 当前 flow_log 路由策略

---

## 十、Step ID 命名空间

> **S6 落地**：本节是 step_id 的**事实来源**。新增 step_id 时**必须**先在这里登记 + 在 [`internal/event/step_id.go`](../../internal/event/step_id.go) 增加常量，再在 callsite 引用。任何不在这份表格里的字符串字面量都会在 code review 被退回。

### 10.1 命名约定

- **点号分隔命名空间**：`"<domain>.<subsystem>.<verb>"`，如 `chat.llm.invoke`
- **公开 step 用常量**（`internal/event/step_id.go`）；私有 step 可保留字面量
- **category 字段**必须是 4 个合法值之一：`orchestration` / `team` / `tool` / `thinking`
- **phase 字段**必须是 3 个合法值之一：`start` / `done` / `error`

### 10.2 当前已注册

| step_id 常量 | 实际字符串 | category | 中文 message | callsite | 状态 |
|---|---|---|---|---|---|
| `event.StepIDChatLLMInvoke` | `chat.llm.invoke` | orchestration | "正在调用语言模型" / "语言模型已返回" / "语言模型调用失败" | `chat_orchestrator_turn.go:Em*` (3 处) | ✅ P0 已 emit |

### 10.3 计划注册（P1/P2 排队）

> 来自 §7.5 "15-18 个细步骤清单"。P0 仅实施 `chat.llm.invoke`（最大单点优化），其余按业务价值分批。

| # | step_id | category | 阻塞 P0？ | 备注 |
|---|---------|----------|----------|------|
| 1 | `chat.session_fetch` | orchestration | ❌ | <100ms，感知不强 |
| 2 | `chat.agent_hydrate` | orchestration | ❌ | 同上 |
| 3 | `chat.provider_resolve` | orchestration | ❌ | 同上 |
| 4 | `chat.variables_parse` | orchestration | ❌ | 同上 |
| 5 | `chat.plugins_load` | orchestration | 🟡 可选 | 2-800ms，密集插件场景可见 |
| 6 | `chat.callbacks_chain_build` | orchestration | ❌ | 同上 |
| 7 | `chat.runner.create` | orchestration | ❌ | 同上 |
| 8 | `chat.intent.pass` | orchestration | 🟡 推荐 | **1-3s** P0 后第二大黑屏源 |
| 9 | `chat.context_compress` | orchestration | 🟡 可选 | 长会话场景重要 |
| 10 | `chat.user_msg_persist` | orchestration | ❌ | <100ms |
| 11 | `chat.llm.invoke` | orchestration | ✅ P0 | **3-15s** 单一最大黑屏源 |
| 12 | `chat.tool_call_dispatch` | orchestration | ❌ | <100ms |
| 13 | `chat.tool_call_execute` | tool | 🟡 推荐 | **0.5-5s** 工具用户期望可见 |
| 14 | `chat.tool_call_serialize` | orchestration | ❌ | <100ms |
| 15 | `chat.stream.consume` | orchestration | ❌ | 持续，不适合做 step |
| 16 | `chat.assistant_msg_persist` | orchestration | ❌ | <200ms |
| 17 | `chat.usage_record` | orchestration | ❌ | <100ms |
| 18 | `team.member_dispatch` | team | 🟡 推荐 | 1-30s，team 模式关键 |
| 19 | `team.member_collect` | team | 🟡 推荐 | team 模式关键 |

### 10.4 注册流程

新增 step_id 时的 checklist：

1. **业务评估**：在 §10.3 表格中确认优先级（🔴 必 / 🟡 推荐 / ❌ 不必）
2. **常量声明**：[`internal/event/step_id.go`](../../internal/event/step_id.go) 增加 `StepIDXxxYyy` 常量
3. **callsite 引用**：`emitter.EmitProgress(ctx, event.StepIDXxxYyy, ...)` 替换字面量
4. **单元测试**：[`internal/event/trace_emitter_test.go`](../../internal/event/trace_emitter_test.go) 增加覆盖
5. **集成测试**：若 step 在 `service` 层集成路径，[`internal/service/chat_orchestrator_turn_test.go`](../../internal/service) 增加 mock
6. **文档同步**：本节 §10.2 表格 "callsite" 列更新

### 10.5 前端 auto-timeout 阈值

来自 [`executionProgress.ts`](../../web/src/features/chat/executionProgress.ts) 的 `AUTO_TIMEOUT_MS`，与本命名空间配合使用。Backend 不强制要求在 X 秒内 emit `done`，前端会用这个阈值把"超时未结束"的 step 标为 `timeout` 状态显示 `(等待中)`：

| category | 默认阈值 (ms) | 调优依据 |
|----------|---------------|----------|
| `thinking` | 8_000 | LLM 首 token P90 |
| `orchestration` | 15_000 | LLM dispatch + intent pass P90 |
| `team` | 30_000 | 多 agent fan-out P90 |
| `tool` | 60_000 | 网络 / 沙箱长操作 P90 |
