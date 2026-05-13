# Planner 规划模块 — 实现设计文档

> 对应需求：`39 planner.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 规划能力：BuiltinPlanner、ReActPlanner、A2UIPlanner。对标 trpc-agent-go `planner` 包。

---

## 二、Proto 层

无需独立 Proto，通过 Agent 的 `dialog_mode` 和 `settings` 配置规划器。

---

## 三、Biz 层

### 3.1 领域模型

```go
type PlannerConfig struct {
    AgentID     string
    Type        string  // "builtin"/"react"/"a2ui"
    Prompt      string  // 自定义规划 prompt
    MaxSteps    int32
    AutoExecute bool
}
```

### 3.2 Planner 接口

```go
type Planner interface {
    Plan(ctx, task string) (*Plan, error)
    Type() string
}

type Plan struct {
    Steps    []PlanStep
    Summary  string
    Metadata map[string]interface{}
}

type PlanStep struct {
    ID          string
    Description string
    Tool        string
    Args        map[string]interface{}
    Status      string  // "pending"/"running"/"completed"/"failed"
    Result      string
}
```

---

## 四、运行时层

### 4.1 BuiltinPlanner

```go
// internal/planner/builtin.go
type BuiltinPlanner struct {
    llm model.LLM
}

func (p *BuiltinPlanner) Plan(ctx, task string) (*Plan, error)
```

### 4.2 ReActPlanner

```go
// internal/planner/react.go
type ReActPlanner struct {
    llm   model.LLM
    tools []tool.Tool
}

func (p *ReActPlanner) Plan(ctx, task string) (*Plan, error)
```

### 4.3 A2UIPlanner

```go
// internal/planner/a2ui.go
type A2UIPlanner struct {
    llm model.LLM
}

func (p *A2UIPlanner) Plan(ctx, task string) (*Plan, error)
```

### 4.4 Agent 集成

```go
// internal/agent/trpc_build.go
func WithPlanner(planner Planner) llmagent.Option
```

当 `dialog_mode == "plan"` 时启用对应 Planner。

---

## 五、Data 层

Planner 配置存储在 `agent_runtime_settings` 的 `planner_json` 字段。

---

## 六、Wire 注入

待新增：
```
planner.ProviderSet → NewBuiltinPlanner, NewReActPlanner, NewA2UIPlanner
```

---

## 七、Web 前端设计

### 7.1 组件

**PlannerConfigPanel.vue**：规划器配置面板（嵌入 Agent 设置页）

| 控件 | 绑定 | 说明 |
|------|------|------|
| `QSelect` 类型 | `type` | builtin/react/a2ui |
| `QEditor` 提示词 | `prompt` | 自定义规划 prompt |
| `QInput` 最大步骤 | `maxSteps` | 默认 10 |
| `QToggle` 自动执行 | `autoExecute` | 默认 false |

**PlanView.vue**：规划结果展示（嵌入 Chat 页面）

| 区域 | 组件 | 说明 |
|------|------|------|
| 摘要 | `QCard` | 规划摘要 |
| 步骤列表 | `QList` | 各步骤状态 |
| 执行按钮 | `QBtn` | 手动执行/自动执行 |
