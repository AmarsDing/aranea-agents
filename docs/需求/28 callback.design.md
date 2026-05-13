# Callback 回调模块 — 实现设计文档

> 对应需求：`28 callback.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

全链路回调钩子：Agent/Model/Tool 执行前后的拦截、修改和增强。对标 trpc-agent-go `agent/callbacks` + `model/callbacks` + `tool/callbacks`。

---

## 二、Proto 层

无需独立 Proto，通过配置和运行时注册实现。

---

## 三、Biz 层

### 3.1 回调接口

```go
type CallbackFunc func(ctx context.Context, event CallbackEvent) (context.Context, error)

type CallbackEvent struct {
    Type      string  // "before_model"/"after_model"/"before_tool"/"after_tool"/"before_agent"/"after_agent"
    AgentID   string
    SessionID string
    Data      interface{}
}

type CallbackRegistry struct {
    mu       sync.RWMutex
    handlers map[string][]CallbackFunc
}

func (r *CallbackRegistry) Register(eventType string, fn CallbackFunc)
func (r *CallbackRegistry) Fire(ctx context.Context, event CallbackEvent) (context.Context, error)
```

---

## 四、Data 层

### 4.1 回调存储

回调配置存储在 `agent_runtime_settings` 的 `callbacks_json` 字段。

### 4.2 内置回调

```go
// internal/callback/builtin/
var BuiltinCallbacks = map[string]CallbackFunc{
    "log_model_call":    LogModelCall,
    "log_tool_call":     LogToolCall,
    "track_token_usage": TrackTokenUsage,
    "inject_memory":     InjectMemory,
    "check_budget":      CheckBudget,
}
```

---

## 五、运行时层

### 5.1 Agent 回调集成

```go
// internal/agent/trpc_build.go
func WithCallbacks(registry *CallbackRegistry, agentID string) llmagent.Option
```

### 5.2 Model 回调集成

```go
// internal/provider/trpc_llm.go
func WithModelCallbacks(registry *CallbackRegistry) model.Option
```

---

## 六、Wire 注入

待新增：
```
biz.ProviderSet → NewCallbackRegistry
```

---

## 七、Web 前端设计

### 7.1 组件

**CallbackEditor.vue**：回调配置编辑器（嵌入 Agent 设置页）

### 7.2 API

通过 `UpdateAgentSettings` 保存回调配置。
