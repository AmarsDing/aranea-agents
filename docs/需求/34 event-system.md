# M18: Event 事件系统 — 详细需求

> 对标 `pkg/trpc-agent-go/event` 包，完善项目的事件系统。

---

## 1. 现状分析

项目已有基础的 SSE 推流：
- `internal/server/sse.go`：将事件投影为 SSE 推流
- 事件仅包含基本的文本内容

**缺失能力**：
1. 无 StateDelta（状态增量更新）
2. 无 Extensions（扩展元数据）
3. 无 FilterKey（层级过滤）
4. 无 Branch（分支追踪）
5. 无 Actions（流控制提示）
6. 无 Tag（业务标签）
7. 无 Clone（深拷贝）
8. 无 LongRunningToolIDs（长时运行工具标记）

---

## 2. trpc 框架参照

```
pkg/trpc-agent-go/event/
├── event.go     # Event 结构体
└── options.go   # 事件选项
```

### Event 完整结构

```go
type Event struct {
    *model.Response                          // LLM 响应基类
    RequestID           string               // 请求 ID
    InvocationID        string               // 调用 ID
    ParentInvocationID  string               // 父调用 ID
    Author              string               // 作者
    ID                  string               // 事件唯一 ID
    Timestamp           time.Time            // 时间戳
    Branch              string               // 分支追踪
    Tag                 string               // 业务标签
    RequiresCompletion  bool                 // 需要完成信号
    LongRunningToolIDs  map[string]struct{}  // 长时运行工具
    StateDelta          map[string][]byte    // 状态增量
    Extensions          map[string]json.RawMessage // 扩展元数据
    StructuredOutput    any                  // 结构化输出
    ExecutionTrace      *trace.Trace         // 执行追踪
    Actions             *EventActions        // 流控制提示
    FilterKey           string               // 层级过滤键
    Version             int                  // 版本号
}
```

### 关键能力

1. **StateDelta**：事件携带状态增量，Runner 自动应用到 Session State
2. **Extensions**：命名空间化的扩展元数据，用于传递自定义信息
3. **FilterKey**：层级过滤键，支持 `parent/child` 格式，用于多 Agent 场景
4. **Branch**：分支追踪，记录 Agent 执行链
5. **Actions**：流控制提示，如 `SkipSummarization`
6. **Tag**：业务标签，如 `code_execution_code`/`transfer`
7. **Clone**：深拷贝，用于事件转发
8. **LongRunningToolIDs**：标记长时运行工具，前端可显示进度

---

## 3. 需求清单

### 3.1 SSE 推流增强

**需求**：SSE 推流包含完整事件元数据

**实现要点**：
- 修改 `internal/server/sse.go`
- SSE 事件体包含 StateDelta/Extensions/FilterKey/Branch/Tag
- 前端可根据 FilterKey 过滤显示

**验收标准**：SSE 推流包含完整事件元数据

### 3.2 StateDelta 处理

**需求**：事件中的 StateDelta 自动应用到 Session State

**实现要点**：
- Runner 在处理事件时检查 StateDelta
- 自动合并到 Session State
- 前端可订阅状态变更

**验收标准**：StateDelta 正确应用到 Session State

### 3.3 FilterKey 层级过滤

**需求**：支持按层级过滤事件

**实现要点**：
- 事件携带 FilterKey（如 `agent_a/agent_b`）
- SSE 推流支持 `?filter_key=agent_a` 参数
- 只推送匹配的事件

**验收标准**：前端可按层级过滤事件流

### 3.4 Branch 追踪

**需求**：记录 Agent 执行链

**实现要点**：
- 事件携带 Branch 字段
- 多 Agent 场景中记录执行路径
- 前端可显示执行树

**验收标准**：多 Agent 场景中可追踪执行链

### 3.5 Extensions 扩展元数据

**需求**：支持自定义扩展元数据

**实现要点**：
- 事件可携带 Extensions
- 命名空间化，避免冲突
- 前端可解析和显示

**验收标准**：事件可携带自定义扩展元数据

### 3.6 Actions 流控制

**需求**：事件可携带流控制提示

**实现要点**：
- `SkipSummarization`：跳过摘要
- Runner 根据 Actions 调整行为

**验收标准**：Runner 正确处理 Actions 提示

### 3.7 前端事件可视化（超越层）

**需求**：前端可视化事件流

**实现要点**：
- 事件时间线视图
- 按分支/标签过滤
- 事件详情展开
- 执行追踪可视化

**验收标准**：前端可可视化查看事件流

---

## 4. 涉及文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/server/sse.go` | 修改 | 增强事件元数据推流 |
| `internal/service/chat_native.go` | 修改 | StateDelta 处理 |
| `web/src/features/chat/components/EventTimeline.vue` | 新建 | 事件时间线 |
| `web/src/features/chat/composables/useEventFilter.ts` | 新建 | 事件过滤 |

---

## 5. 验收标准总览

1. SSE 推流包含完整事件元数据
2. StateDelta 正确应用到 Session State
3. 前端可按层级过滤事件流
4. 多 Agent 场景中可追踪执行链
5. 事件可携带自定义扩展元数据
6. Runner 正确处理 Actions 提示
