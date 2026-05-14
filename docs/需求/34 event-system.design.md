# Event 事件系统模块 — 实现设计文档

> 对应需求：`34 event-system.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

完善事件系统：StateDelta 状态增量、Extensions 扩展元数据、FilterKey 层级过滤、Branch 分支追踪、Actions 流控制、Tag 业务标签、LongRunningToolIDs 长时运行工具标记、Clone 深拷贝。对标 trpc-agent-go `event` 包。

**核心改动**：
1. 增强 SSE 推流，携带完整事件元数据（StateDelta/Extensions/FilterKey/Branch/Tag/Actions）
2. 新增 `AgentEventBroker` 替代现有 `TeamRunEventBroker` 的事件分发（保留 TeamRunEventBroker 不变）
3. Runner 处理事件时自动应用 StateDelta 到 Session State
4. 前端支持事件过滤、分支追踪可视化、状态变更指示

---

## 二、Proto 层

### 2.1 现有 Proto

`api/kratos/chat/v1/chat.proto` — SSE 推流不走 proto，直接 JSON 序列化。

### 2.2 SSE 事件体 JSON Schema（非 proto，直接 JSON 推流）

SSE 推流事件体统一为以下 JSON 结构，与 trpc-agent-go `event.Event` 字段对齐：

```json
{
  "id": "evt-uuid",
  "type": "text | tool_call | tool_result | state_delta | transfer | done | error | runner_completion",
  "content": "事件文本内容",
  "author": "agent_name | user",
  "session_id": "sess-uuid",
  "agent_id": "agent-uuid",
  "request_id": "req-uuid",
  "invocation_id": "inv-uuid",
  "parent_invocation_id": "parent-inv-uuid",
  "branch": "agent_a/agent_b",
  "tag": "code_execution_code;transfer",
  "filter_key": "agent_a/agent_b",
  "requires_completion": false,
  "long_running_tool_ids": ["tool-call-id-1"],
  "state_delta": {
    "operation": "set | append | delete",
    "path": "state.key.path",
    "value_json": "{\"count\": 1}"
  },
  "extensions": {
    "trpc_agent.tool_call_args": "{\"arg1\": \"val1\"}"
  },
  "actions": {
    "skip_summarization": true
  },
  "version": 1,
  "created_at": "2025-01-01T00:00:00Z"
}
```

**事件类型映射**（对齐 trpc-agent-go `model.Response.Object`）：

| SSE event 名 | type 字段 | 对应 model.ObjectType | 说明 |
|---------------|-----------|----------------------|------|
| `text` | text | chat.completion.chunk | 文本增量 |
| `tool_call` | tool_call | — | 工具调用开始 |
| `tool_result` | tool_result | tool.response | 工具返回结果 |
| `state_delta` | state_delta | state.update | 状态增量更新 |
| `transfer` | transfer | agent.transfer | Agent 转移控制权 |
| `done` | done | runner.completion | 运行完成 |
| `error` | error | error | 错误事件 |
| `tool_event` | tool_event | — | 工具执行进度（兼容现有） |
| `member_message_start` | member_message_start | — | Team 成员消息开始（兼容现有） |
| `member_delta` | member_delta | — | Team 成员增量（兼容现有） |
| `member_message_done` | member_message_done | — | Team 成员消息完成（兼容现有） |

**向后兼容**：现有 SSE 事件格式（`user_message`/`delta`/`done`/`tool_event`/`member_*`）保持不变，新事件类型为增量添加。

---

## 三、Biz 层

### 3.1 领域模型

```go
// internal/biz/agent_event.go

type AgentEvent struct {
    ID                  string
    Type                string
    Content             string
    Author              string
    SessionID           string
    AgentID             string
    RequestID           string
    InvocationID        string
    ParentInvocationID  string
    Branch              string
    Tag                 string
    FilterKey           string
    RequiresCompletion  bool
    LongRunningToolIDs  []string
    StateDelta          *EventStateDelta
    Extensions          map[string]string
    Actions             *EventActions
    Version             int
    CreatedAt           string
}

type EventStateDelta struct {
    Operation string
    Path      string
    ValueJSON string
}

type EventActions struct {
    SkipSummarization bool
}
```

**设计决策**：
- `StateDelta` 使用单条而非 `map[string][]byte`（与 trpc-agent-go 不同），因为每条 SSE 事件携带一个状态变更更直观
- `Extensions` 使用 `map[string]string`（JSON string 值），与 biz 层不依赖 `json.RawMessage` 的原则一致
- `LongRunningToolIDs` 使用 `[]string` 而非 `map[string]struct{}`，biz 层不暴露 set 语义
- `Actions` 使用指针，nil 表示无 Actions

### 3.2 AgentEventBroker

```go
// internal/biz/agent_event_broker.go

type AgentEventBroker struct {
    mu          sync.RWMutex
    subscribers map[chan AgentEvent]*eventFilter
}

type eventFilter struct {
    sessionID string
    filterKey string
}

func NewAgentEventBroker() *AgentEventBroker {
    return &AgentEventBroker{subscribers: map[chan AgentEvent]*eventFilter{}}
}

func (b *AgentEventBroker) Subscribe(sessionID, filterKey string) (chan AgentEvent, func()) {
    ch := make(chan AgentEvent, 64)
    b.mu.Lock()
    b.subscribers[ch] = &eventFilter{sessionID: sessionID, filterKey: filterKey}
    b.mu.Unlock()
    unsubscribe := func() {
        b.mu.Lock()
        if _, ok := b.subscribers[ch]; ok {
            delete(b.subscribers, ch)
            close(ch)
        }
        b.mu.Unlock()
    }
    return ch, unsubscribe
}

func (b *AgentEventBroker) Publish(event AgentEvent) {
    b.mu.RLock()
    defer b.mu.RUnlock()
    for ch, filter := range b.subscribers {
        if filter.sessionID != "" && filter.sessionID != event.SessionID {
            continue
        }
        if !matchFilterKey(filter.filterKey, event.FilterKey) {
            continue
        }
        select {
        case ch <- event:
        default:
        }
    }
}
```

**FilterKey 匹配规则**（对齐 trpc-agent-go `Event.Filter`）：

```go
// internal/biz/agent_event_broker.go

const filterKeyDelimiter = "/"

func matchFilterKey(subscriberKey, eventKey string) bool {
    if subscriberKey == "" || eventKey == "" {
        return true
    }
    subscriberKey += filterKeyDelimiter
    eventKey += filterKeyDelimiter
    return strings.HasPrefix(subscriberKey, eventKey) || strings.HasPrefix(eventKey, subscriberKey)
}
```

### 3.3 StateDelta 应用

```go
// internal/biz/session_state_delta.go

type SessionStateDeltaApplier struct {
    sessions SessionRepository
}

func NewSessionStateDeltaApplier(sessions SessionRepository) *SessionStateDeltaApplier {
    return &SessionStateDeltaApplier{sessions: sessions}
}

func (a *SessionStateDeltaApplier) Apply(ctx context.Context, sessionID string, delta EventStateDelta) error {
    if delta.Path == "" {
        return nil
    }
    state, err := a.sessions.GetSessionState(ctx, sessionID)
    if err != nil {
        return err
    }
    switch delta.Operation {
    case "set":
        state[delta.Path] = delta.ValueJSON
    case "append":
        existing, _ := state[delta.Path]
        state[delta.Path] = existing + delta.ValueJSON
    case "delete":
        delete(state, delta.Path)
    }
    return a.sessions.SaveSessionState(ctx, sessionID, state)
}
```

### 3.4 事件构造辅助

```go
// internal/biz/agent_event.go

const (
    EventTagCodeExecution      = "code_execution_code"
    EventTagCodeExecutionResult = "code_execution_result"
    EventTagTransfer           = "transfer"
    EventTagDelimiter          = ";"
)

func (e *AgentEvent) ContainsTag(tag string) bool {
    if e.Tag == "" {
        return false
    }
    tags := strings.Split(e.Tag, EventTagDelimiter)
    for _, t := range tags {
        if strings.TrimSpace(t) == strings.TrimSpace(tag) {
            return true
        }
    }
    return false
}

func (e *AgentEvent) IsRunnerCompletion() bool {
    return e.Type == "done"
}

func (e *AgentEvent) IsError() bool {
    return e.Type == "error"
}

func NewAgentEvent(invocationID, author, eventType string) *AgentEvent {
    return &AgentEvent{
        ID:           uuid.NewString(),
        InvocationID: invocationID,
        Author:       author,
        Type:         eventType,
        Version:      1,
        CreatedAt:    time.Now().UTC().Format(time.RFC3339),
    }
}
```

---

## 四、Data 层

### 4.1 Session State 持久化

在 `internal/data/session.go` 的 `sessionRepo` 中新增：

```go
func (r *sessionRepo) GetSessionState(ctx context.Context, sessionID string) (map[string]string, error) {
    row, err := r.data.entClient.Session.Get(ctx, sessionID)
    if err != nil {
        return nil, err
    }
    state := map[string]string{}
    if row.StateJSON != "" {
        _ = json.Unmarshal([]byte(row.StateJSON), &state)
    }
    return state, nil
}

func (r *sessionRepo) SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error {
    raw, _ := json.Marshal(state)
    _, err := r.data.entClient.Session.UpdateOneID(sessionID).
        SetStateJSON(string(raw)).
        Save(ctx)
    return err
}
```

### 4.2 Ent Schema 变更

在 `internal/data/ent/schema/session.go` 新增字段：

```go
field.String("state_json").Default("{}").Optional(),
```

### 4.3 Session Repo 接口扩展

在 `internal/biz/session.go` 的 `SessionRepository` 接口新增：

```go
GetSessionState(ctx context.Context, sessionID string) (map[string]string, error)
SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error
```

---

## 五、Service 层

### 5.1 SSE 推流增强

修改 `internal/service/chat_native.go` 的 `streamWriter.Emit`：

```go
func (w *streamWriter) Emit(event string, payload any) error {
    switch event {
    case "user_message":
        m, ok := payload.(biz.ChatMessage)
        if !ok {
            return nil
        }
        return w.writeEvent("user_message", chatMessageToMap(m))
    case "delta":
        return w.writeEvent("delta", payload)
    case "done":
        m, ok := payload.(biz.ChatMessage)
        if !ok {
            return nil
        }
        return w.writeEvent("done", map[string]any{"agent_message": chatMessageToMap(m)})
    case "agent_event":
        evt, ok := payload.(*biz.AgentEvent)
        if !ok {
            return nil
        }
        return w.writeAgentEvent(evt)
    default:
        return w.writeEvent(event, payload)
    }
}

func (w *streamWriter) writeAgentEvent(e *biz.AgentEvent) error {
    payload := map[string]any{
        "id":                   e.ID,
        "type":                 e.Type,
        "content":              e.Content,
        "author":               e.Author,
        "session_id":           e.SessionID,
        "agent_id":             e.AgentID,
        "request_id":           e.RequestID,
        "invocation_id":        e.InvocationID,
        "parent_invocation_id": e.ParentInvocationID,
        "branch":               e.Branch,
        "tag":                  e.Tag,
        "filter_key":           e.FilterKey,
        "requires_completion":  e.RequiresCompletion,
        "version":              e.Version,
        "created_at":           e.CreatedAt,
    }
    if len(e.LongRunningToolIDs) > 0 {
        payload["long_running_tool_ids"] = e.LongRunningToolIDs
    }
    if e.StateDelta != nil {
        payload["state_delta"] = map[string]string{
            "operation":  e.StateDelta.Operation,
            "path":       e.StateDelta.Path,
            "value_json": e.StateDelta.ValueJSON,
        }
    }
    if len(e.Extensions) > 0 {
        payload["extensions"] = e.Extensions
    }
    if e.Actions != nil {
        payload["actions"] = map[string]bool{
            "skip_summarization": e.Actions.SkipSummarization,
        }
    }
    return w.writeEvent(e.Type, payload)
}
```

### 5.2 AgentEventBroker SSE 端点

新增 `internal/server/agent_event_sse.go`：

```go
package server

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "aranea-agents/internal/biz"

    sse "github.com/tx7do/kratos-transport/transport/sse"
)

func registerAgentEventSSE(srv *sse.Server, broker *biz.AgentEventBroker) {
    if srv == nil || broker == nil {
        return
    }
    srv.HandleFunc("/agent-events/stream", func(w http.ResponseWriter, r *http.Request) {
        if !prepareSSEAccessControl(w, r) {
            return
        }
        if r.Method != http.MethodGet {
            http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
            return
        }
        flusher, ok := w.(http.Flusher)
        if !ok {
            http.Error(w, "streaming not supported", http.StatusInternalServerError)
            return
        }
        w.Header().Set("Content-Type", "text/event-stream")
        w.Header().Set("Cache-Control", "no-cache")
        w.Header().Set("Connection", "keep-alive")
        w.Header().Set("X-Accel-Buffering", "no")

        sessionID := r.URL.Query().Get("session_id")
        filterKey := r.URL.Query().Get("filter_key")
        events, unsubscribe := broker.Subscribe(sessionID, filterKey)
        defer unsubscribe()

        _, _ = fmt.Fprint(w, ": connected\n\n")
        flusher.Flush()

        heartbeat := time.NewTicker(15 * time.Second)
        defer heartbeat.Stop()

        for {
            select {
            case <-r.Context().Done():
                return
            case <-heartbeat.C:
                _, _ = fmt.Fprint(w, ": heartbeat\n\n")
                flusher.Flush()
            case event, ok := <-events:
                if !ok {
                    return
                }
                raw, err := json.Marshal(event)
                if err != nil {
                    continue
                }
                _, _ = fmt.Fprintf(w, "event: %s\n", event.Type)
                _, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
                flusher.Flush()
            }
        }
    })
}
```

### 5.3 StateDelta 应用集成

在 `internal/agent/` 层（框架桥接层），Runner 事件循环中处理 StateDelta：

```go
// internal/agent/event_processor.go

func ProcessEventStateDelta(ctx context.Context, applier *biz.SessionStateDeltaApplier, evt *event.Event) {
    if evt == nil || len(evt.StateDelta) == 0 {
        return
    }
    sessionID := extractSessionID(ctx)
    for key, value := range evt.StateDelta {
        delta := biz.EventStateDelta{
            Operation: "set",
            Path:      key,
            ValueJSON: string(value),
        }
        _ = applier.Apply(ctx, sessionID, delta)
    }
}
```

### 5.4 ChatService 集成

在 `ChatServiceDeps` 新增：

```go
type ChatServiceDeps struct {
    Broker       *biz.TeamRunEventBroker
    AgentEvents  *biz.AgentEventBroker
    StateApplier *biz.SessionStateDeltaApplier
    Teams        biz.TeamRepository
    TeamsNative  *team.Runner
    Usage        *biz.UsageUsecase
    Sessions     *biz.SessionUsecase
    Agents       biz.AgentRepository
    AgentsUC     *biz.AgentUsecase
    ToolsCatalog biz.ToolRepo
    LLMCatalog   *biz.LlmProviderModelUsecase
    SkillUC      *biz.SkillUsecase
    Sys          biz.SystemSettingRepo
    RT           *runtimedeps.Runtime
    Compress     biz.NativeTurnCompressor
    MonitorLogs  *biz.MonitorLogBroker
}
```

---

## 六、Wire 注入

### 6.1 Biz ProviderSet 新增

```go
// internal/biz/biz.go
var ProviderSet = wire.NewSet(
    NewTeamRunEventBroker,
    NewAgentEventBroker,
    NewSessionStateDeltaApplier,
    NewMonitorLogBroker,
    NewAdminUsecase,
    NewAvatarUsecase,
    NewMemoryUsecase,
    NewAgentUsecase,
    NewTeamUsecase,
    NewAgentCategoryUsecase,
    NewLlmProviderModelUsecase,
    NewHookUsecase,
    NewCronUsecase,
    NewPluginUsecase,
    NewMCPServerUsecase,
    NewSkillUsecase,
    NewSessionUsecase,
    NewToolUsecase,
    NewChannelUsecase,
    NewUsageUsecase,
    NewMonitorUsecase,
    NewSystemSettingUsecase,
    NewAgentMCPTooling,
    NewEvolutionUsecase,
)
```

### 6.2 SSE Server 注入

修改 `NewSSEServer` 签名，新增 `agentEvents *biz.AgentEventBroker`：

```go
func NewSSEServer(c *conf.Server, teamRunEvents *biz.TeamRunEventBroker, agentEvents *biz.AgentEventBroker, monitorLogs *biz.MonitorLogBroker) *sse.Server {
    // ... 现有逻辑 ...
    registerMonitorLogSSE(srv)
    registerTeamRunSSE(srv, teamRunEvents)
    registerAgentEventSSE(srv, agentEvents)
    // ... 其余不变 ...
}
```

---

## 七、Web 前端设计

### 7.1 类型定义

```typescript
// web/src/features/chat/types.ts 新增

export type AgentEvent = {
  id: string;
  type: "text" | "tool_call" | "tool_result" | "state_delta" | "transfer" | "done" | "error" | "runner_completion";
  content: string;
  author: string;
  session_id: string;
  agent_id: string;
  request_id: string;
  invocation_id: string;
  parent_invocation_id: string;
  branch: string;
  tag: string;
  filter_key: string;
  requires_completion: boolean;
  long_running_tool_ids: string[];
  state_delta: EventStateDelta | null;
  extensions: Record<string, string> | null;
  actions: EventActions | null;
  version: number;
  created_at: string;
};

export type EventStateDelta = {
  operation: "set" | "append" | "delete";
  path: string;
  value_json: string;
};

export type EventActions = {
  skip_summarization: boolean;
};
```

### 7.2 SSE 客户端增强

```typescript
// web/src/features/chat/api.ts 新增

export type AgentEventStreamCallbacks = {
  signal?: AbortSignal;
  onEvent?: (event: AgentEvent) => void;
  onStateDelta?: (delta: EventStateDelta) => void;
  onTransfer?: (event: AgentEvent) => void;
  onError?: (event: AgentEvent) => void;
  onDone?: () => void;
};

export function connectAgentEventStream(
  sessionId: string,
  filterKey: string,
  callbacks: AgentEventStreamCallbacks = {}
): { close: () => void } {
  const params = new URLSearchParams({ session_id: sessionId });
  if (filterKey) params.set("filter_key", filterKey);
  const origin = getBackendOrigin();
  const es = new EventSource(`${origin}/agent-events/stream?${params}`);

  es.addEventListener("state_delta", (e: MessageEvent) => {
    const evt = JSON.parse(e.data) as AgentEvent;
    callbacks.onEvent?.(evt);
    if (evt.state_delta) callbacks.onStateDelta?.(evt.state_delta);
  });

  es.addEventListener("transfer", (e: MessageEvent) => {
    const evt = JSON.parse(e.data) as AgentEvent;
    callbacks.onEvent?.(evt);
    callbacks.onTransfer?.(evt);
  });

  es.addEventListener("error", (e: MessageEvent) => {
    const evt = JSON.parse(e.data) as AgentEvent;
    callbacks.onEvent?.(evt);
    callbacks.onError?.(evt);
  });

  es.addEventListener("done", () => {
    callbacks.onDone?.();
  });

  if (callbacks.signal) {
    callbacks.signal.addEventListener("abort", () => es.close());
  }

  return { close: () => es.close() };
}
```

### 7.3 组件设计

#### EventTimeline.vue — 事件时间线

```
位置：web/src/features/chat/components/EventTimeline.vue
用途：在 Chat 页面侧边栏展示事件时间线，支持按分支/标签过滤
```

**Props**：

| Prop | 类型 | 说明 |
|------|------|------|
| sessionId | string | 当前会话 ID |
| filterKey | string | 过滤键（可选） |
| visible | boolean | 是否显示 |

**模板结构**：

```
<div class="event-timeline">
  <div class="timeline-header">
    <q-input v-model="searchFilter" dense placeholder="过滤事件..." />
    <q-btn-toggle v-model="typeFilter" :options="typeOptions" />
  </div>
  <q-scroll-area class="timeline-body">
    <div v-for="event in filteredEvents" :key="event.id" class="timeline-item">
      <div class="event-icon" :class="iconClass(event.type)">
        <q-icon :name="iconForType(event.type)" />
      </div>
      <div class="event-content">
        <div class="event-meta">
          <span class="event-type-badge">{{ event.type }}</span>
          <span v-if="event.branch" class="event-branch">{{ event.branch }}</span>
          <span v-if="event.tag" class="event-tag">{{ event.tag }}</span>
          <span class="event-time">{{ formatTime(event.created_at) }}</span>
        </div>
        <div class="event-body">
          <template v-if="event.type === 'state_delta'">
            <StateDeltaIndicator :delta="event.state_delta" />
          </template>
          <template v-else-if="event.type === 'transfer'">
            <TransferBadge :author="event.author" :content="event.content" />
          </template>
          <template v-else>
            {{ event.content }}
          </template>
        </div>
        <div v-if="event.long_running_tool_ids?.length" class="long-running-tools">
          <q-spinner-dots size="xs" />
          <span>长时运行工具: {{ event.long_running_tool_ids.join(', ') }}</span>
        </div>
      </div>
    </div>
  </q-scroll-area>
</div>
```

#### StateDeltaIndicator.vue — 状态变更指示器

```
位置：web/src/features/chat/components/StateDeltaIndicator.vue
用途：可视化展示 StateDelta 操作
```

**Props**：

| Prop | 类型 | 说明 |
|------|------|------|
| delta | EventStateDelta | 状态增量 |

**模板结构**：

```
<div class="state-delta" :class="delta.operation">
  <q-icon :name="operationIcon" size="xs" />
  <code class="delta-path">{{ delta.path }}</code>
  <template v-if="delta.operation !== 'delete'">
    <span class="arrow">→</span>
    <code class="delta-value">{{ delta.value_json }}</code>
  </template>
</div>
```

**操作图标映射**：

| 操作 | 图标 | 颜色 |
|------|------|------|
| set | edit | primary |
| append | add | positive |
| delete | delete | negative |

#### TransferBadge.vue — Agent 转移标签

```
位置：web/src/features/chat/components/TransferBadge.vue
用途：显示 Agent 间控制权转移
```

**Props**：

| Prop | 类型 | 说明 |
|------|------|------|
| author | string | 转移到的 Agent |
| content | string | 转移说明 |

#### BranchTree.vue — 分支追踪树

```
位置：web/src/features/chat/components/BranchTree.vue
用途：在 Team 会话中展示 Agent 执行分支树
```

**Props**：

| Prop | 类型 | 说明 |
|------|------|------|
| events | AgentEvent[] | 事件列表 |

**模板结构**：

```
<div class="branch-tree">
  <q-tree :nodes="branchNodes" node-key="id" default-expand-all>
    <template #default-header="prop">
      <div class="branch-node">
        <q-icon :name="iconForType(prop.node.type)" size="sm" />
        <span>{{ prop.node.label }}</span>
        <span class="text-caption q-ml-sm">{{ prop.node.author }}</span>
      </div>
    </template>
  </q-tree>
</div>
```

**数据转换逻辑**：

```typescript
function buildBranchTree(events: AgentEvent[]): TreeNode[] {
  const root: TreeNode[] = [];
  const map = new Map<string, TreeNode>();
  for (const evt of events) {
    const node: TreeNode = {
      id: evt.id,
      label: `${evt.type}: ${truncate(evt.content, 40)}`,
      type: evt.type,
      author: evt.author,
      children: [],
    };
    map.set(evt.invocation_id, node);
    if (evt.parent_invocation_id && map.has(evt.parent_invocation_id)) {
      map.get(evt.parent_invocation_id)!.children.push(node);
    } else {
      root.push(node);
    }
  }
  return root;
}
```

### 7.4 Composable

#### useEventFilter.ts — 事件过滤

```
位置：web/src/features/chat/composables/useEventFilter.ts
用途：提供事件过滤逻辑
```

```typescript
import { computed, ref } from "vue";
import type { AgentEvent } from "../types";

export function useEventFilter(events: Ref<AgentEvent[]>) {
  const typeFilter = ref<string>("all");
  const branchFilter = ref<string>("");
  const tagFilter = ref<string>("");
  const searchQuery = ref<string>("");

  const filteredEvents = computed(() => {
    return events.value.filter((evt) => {
      if (typeFilter.value !== "all" && evt.type !== typeFilter.value) return false;
      if (branchFilter.value && evt.branch !== branchFilter.value) return false;
      if (tagFilter.value && !evt.tag.includes(tagFilter.value)) return false;
      if (searchQuery.value) {
        const q = searchQuery.value.toLowerCase();
        return (
          evt.content.toLowerCase().includes(q) ||
          evt.author.toLowerCase().includes(q) ||
          evt.tag.toLowerCase().includes(q)
        );
      }
      return true;
    });
  });

  const branches = computed(() =>
    [...new Set(events.value.map((e) => e.branch).filter(Boolean))]
  );

  const tags = computed(() =>
    [...new Set(events.value.flatMap((e) => e.tag.split(";")).filter(Boolean))]
  );

  return { typeFilter, branchFilter, tagFilter, searchQuery, filteredEvents, branches, tags };
}
```

### 7.5 现有 SSE 处理增强

修改 `web/src/features/chat/api.ts` 的 `handleStreamEvent`，增加新事件类型处理：

```typescript
function handleStreamEvent(block: string, callbacks: SendMessageStreamCallbacks) {
  // ... 现有解析逻辑不变 ...

  // 新增事件类型
  if (event === "state_delta") {
    callbacks.onStateDelta?.(parsed as EventStateDelta);
  } else if (event === "transfer") {
    callbacks.onTransfer?.(parsed as AgentEvent);
  } else if (event === "agent_event") {
    callbacks.onAgentEvent?.(parsed as AgentEvent);
  }
  // ... 现有事件类型保持不变 ...
}
```

在 `SendMessageStreamCallbacks` 新增：

```typescript
export type SendMessageStreamCallbacks = {
  signal?: AbortSignal;
  onUserMessage?: (message: Message) => void;
  onDelta?: (content: string) => void;
  onDone?: (message: Message) => void;
  onToolEvent?: (event: ToolUseEvent) => void;
  onMemberMessageStart?: (message: Message) => void;
  onMemberDelta?: (messageID: string, content: string) => void;
  onMemberMessageDone?: (message: Message) => void;
  onStateDelta?: (delta: EventStateDelta) => void;
  onTransfer?: (event: AgentEvent) => void;
  onAgentEvent?: (event: AgentEvent) => void;
};
```

### 7.6 Chat 页面集成

在 `ChatWorkspace.vue` 中集成 EventTimeline：

```
<template>
  <div class="chat-workspace">
    <!-- 现有布局 -->
    <div class="chat-main">
      <ChatPanel ... />
    </div>
    <!-- 新增事件时间线侧边栏 -->
    <q-drawer v-model="eventTimelineVisible" side="right" :width="360" bordered>
      <EventTimeline
        :session-id="currentSessionId"
        :filter-key="eventFilterKey"
        :visible="eventTimelineVisible"
      />
    </q-drawer>
  </div>
</template>
```

---

## 八、涉及文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/biz/agent_event.go` | 新建 | AgentEvent 领域模型 + 辅助方法 |
| `internal/biz/agent_event_broker.go` | 新建 | AgentEventBroker 发布/订阅 |
| `internal/biz/session_state_delta.go` | 新建 | StateDelta 应用逻辑 |
| `internal/biz/session.go` | 修改 | SessionRepository 接口新增 GetSessionState/SaveSessionState |
| `internal/biz/biz.go` | 修改 | ProviderSet 新增 NewAgentEventBroker, NewSessionStateDeltaApplier |
| `internal/data/session.go` | 修改 | 实现 GetSessionState/SaveSessionState |
| `internal/data/ent/schema/session.go` | 修改 | 新增 state_json 字段 |
| `internal/server/agent_event_sse.go` | 新建 | AgentEvent SSE 端点 |
| `internal/server/sse.go` | 修改 | NewSSEServer 签名新增 agentEvents 参数 |
| `internal/service/chat_native.go` | 修改 | streamWriter 新增 writeAgentEvent |
| `internal/service/chat.go` | 修改 | ChatServiceDeps 新增 AgentEvents/StateApplier |
| `internal/agent/event_processor.go` | 新建 | StateDelta 应用桥接 |
| `cmd/admin/wire.go` | 修改 | Wire 注入更新 |
| `web/src/features/chat/types.ts` | 修改 | 新增 AgentEvent/EventStateDelta/EventActions 类型 |
| `web/src/features/chat/api.ts` | 修改 | 新增 connectAgentEventStream、handleStreamEvent 增强 |
| `web/src/features/chat/components/EventTimeline.vue` | 新建 | 事件时间线组件 |
| `web/src/features/chat/components/StateDeltaIndicator.vue` | 新建 | 状态变更指示器 |
| `web/src/features/chat/components/TransferBadge.vue` | 新建 | Agent 转移标签 |
| `web/src/features/chat/components/BranchTree.vue` | 新建 | 分支追踪树 |
| `web/src/features/chat/composables/useEventFilter.ts` | 新建 | 事件过滤 composable |

---

## 九、实现阶段

### 阶段一：后端核心（StateDelta + Broker）

1. 新建 `internal/biz/agent_event.go` — AgentEvent 模型
2. 新建 `internal/biz/agent_event_broker.go` — AgentEventBroker
3. 新建 `internal/biz/session_state_delta.go` — StateDelta 应用
4. 修改 `internal/biz/session.go` — Repo 接口扩展
5. 修改 `internal/biz/biz.go` — Wire ProviderSet
6. 修改 `internal/data/ent/schema/session.go` — state_json 字段
7. 运行 `go generate ./internal/data/ent`
8. 修改 `internal/data/session.go` — 实现 State 读写
9. 验证：单元测试 StateDelta 应用逻辑

### 阶段二：SSE 推流增强

1. 新建 `internal/server/agent_event_sse.go` — AgentEvent SSE 端点
2. 修改 `internal/server/sse.go` — 注入 AgentEventBroker
3. 修改 `internal/service/chat_native.go` — streamWriter 增强
4. 修改 `internal/service/chat.go` — ChatServiceDeps 扩展
5. 修改 `cmd/admin/wire.go` — Wire 注入
6. 验证：SSE 推流包含完整事件元数据

### 阶段三：前端可视化

1. 修改 `web/src/features/chat/types.ts` — 新增类型
2. 修改 `web/src/features/chat/api.ts` — SSE 客户端增强
3. 新建 `web/src/features/chat/components/StateDeltaIndicator.vue`
4. 新建 `web/src/features/chat/components/TransferBadge.vue`
5. 新建 `web/src/features/chat/components/BranchTree.vue`
6. 新建 `web/src/features/chat/components/EventTimeline.vue`
7. 新建 `web/src/features/chat/composables/useEventFilter.ts`
8. 集成到 ChatWorkspace
9. 验证：前端可按层级过滤事件流、追踪执行链

---

## 十、验收标准

1. ✅ SSE 推流包含完整事件元数据（StateDelta/Extensions/FilterKey/Branch/Tag/Actions）
2. ✅ StateDelta 正确应用到 Session State（set/append/delete 三种操作）
3. ✅ 前端可按层级过滤事件流（FilterKey 前缀匹配）
4. ✅ 多 Agent 场景中可追踪执行链（Branch + InvocationID/ParentInvocationID）
5. ✅ 事件可携带自定义扩展元数据（Extensions 命名空间化）
6. ✅ Runner 正确处理 Actions 提示（SkipSummarization）
7. ✅ 前端事件时间线可视化（按类型/分支/标签过滤）
8. ✅ 向后兼容：现有 SSE 事件格式不变
