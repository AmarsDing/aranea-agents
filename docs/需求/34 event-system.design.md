# Event 事件系统模块 — 实现设计文档

> 对应需求：`34 event-system.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

完善事件系统：StateDelta 状态增量、Extensions 扩展元数据、FilterKey 层级过滤、Branch 分支追踪、Actions 流控制、Tag 业务标签。对标 trpc-agent-go `event` 包。

---

## 二、Proto 层

### 2.1 现有 Proto

SSE 推流已有基础事件结构。

### 2.2 待新增

```protobuf
message AgentEvent {
  string id = 1;
  string type = 2;
  string content = 3;
  string author = 4;
  string session_id = 5;
  string agent_id = 6;
  map<string, string> metadata = 7;
  StateDelta state_delta = 8;
  repeated Action actions = 9;
  repeated string tags = 10;
  string branch = 11;
  string filter_key = 12;
  google.protobuf.Struct extensions = 13;
  string created_at = 14;
}

message StateDelta {
  string operation = 1;  // "set"/"append"/"delete"
  string path = 2;
  string value_json = 3;
}

message Action {
  string type = 1;  // "interrupt"/"resume"/"cancel"/"retry"
  string label = 2;
  string payload_json = 3;
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type AgentEvent struct {
    ID         string
    Type       string  // "text"/"tool_call"/"tool_result"/"state_delta"/"transfer"/"done"/"error"
    Content    string
    Author     string
    SessionID  string
    AgentID    string
    Metadata   map[string]string
    StateDelta *StateDelta
    Actions    []Action
    Tags       []string
    Branch     string
    FilterKey  string
    Extensions map[string]interface{}
    CreatedAt  string
}

type StateDelta struct {
    Operation string  // "set"/"append"/"delete"
    Path      string
    ValueJSON string
}

type Action struct {
    Type       string  // "interrupt"/"resume"/"cancel"/"retry"
    Label      string
    PayloadJSON string
}
```

### 3.2 Event Broker

```go
type EventBroker struct {
    mu    sync.RWMutex
    chans map[string][]chan *AgentEvent
}

func (b *EventBroker) Publish(ctx, event *AgentEvent) error
func (b *EventBroker) Subscribe(sessionID string, filterKey string) <-chan *AgentEvent
func (b *EventBroker) Unsubscribe(sessionID string, ch <-chan *AgentEvent)
```

### 3.3 事件过滤

```go
func FilterByKey(events <-chan *AgentEvent, key string) <-chan *AgentEvent
func FilterByTag(events <-chan *AgentEvent, tag string) <-chan *AgentEvent
```

---

## 四、Data 层

### 4.1 SSE 投影

```go
// internal/server/sse.go
func (s *SSEServer) projectEvent(e *AgentEvent) SSEMessage {
    return SSEMessage{
        Event: e.Type,
        Data:  marshalEvent(e),
    }
}
```

### 4.2 事件持久化

事件存储在 `messages` 表，StateDelta 存储在 `session_state_deltas` 表。

---

## 五、Service 层

```go
// internal/server/sse.go
func (s *SSEServer) StreamEvents(ctx, sessionID, filterKey string) <-chan SSEMessage
```

---

## 六、Wire 注入

已有 `EventBroker`，无需新增。

---

## 七、Web 前端设计

### 7.1 组件

**EventStream.vue**：SSE 事件流渲染（嵌入 Chat 页面）

| 事件类型 | 渲染 | 说明 |
|----------|------|------|
| text | Markdown | 文本消息 |
| tool_call | 工具卡片 | 工具调用 |
| tool_result | 结果面板 | 工具结果 |
| state_delta | 状态指示器 | 状态变更 |
| transfer | 转移标签 | Agent 转移 |
| action | 操作按钮 | 流控制 |

### 7.2 SSE 客户端

```typescript
export function connectEventStream(sessionId: string, filterKey?: string): EventSource
```
