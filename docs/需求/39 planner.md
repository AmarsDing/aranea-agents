# M11: Planner 规划 — 详细需求

> 对标 `pkg/trpc-agent-go/planner` 包，实现 Agent 规划能力。

---

## 1. 现状分析

项目在 `internal/agent/trpc_build.go` 中仅集成了 `trpcbuiltin.New(trpcbuiltin.Options{})`：
- 当 `DialogMode == "plan"` 时启用 BuiltinPlanner
- 无 ReActPlanner
- 无 A2UIPlanner
- 无自定义规划 prompt
- 无规划结果的结构化处理

---

## 2. trpc 框架参照

```
pkg/trpc-agent-go/planner/
├── planner.go              # Planner 接口：BuildPlanningInstruction + ProcessPlanningResponse
├── builtin/
│   ├── builtin_planner.go  # BuiltinPlanner：简单规划指令注入
│   └── builtin_planner_test.go
├── react/
│   ├── react_planner.go    # ReActPlanner：PLANNING/REASONING/ACTION/FINAL_ANSWER 标签
│   └── react_planner_test.go
└── a2ui/
    ├── a2ui.go             # A2UIPlanner：A2UI 协议规划
    ├── a2ui_test.go
    ├── options.go          # A2UI 选项
    └── schema.go           # A2UI Schema 定义
```

### Planner 接口

```go
type Planner interface {
    BuildPlanningInstruction(ctx context.Context, invocation *agent.Invocation, llmRequest *model.Request) string
    ProcessPlanningResponse(ctx context.Context, invocation *agent.Invocation, response *model.Response) *model.Response
}
```

### BuiltinPlanner

简单规划：注入一段系统指令，让 LLM 在回复前先思考。不修改 LLM 响应。

### ReActPlanner

结构化规划：使用标签约束 LLM 输出格式：
- `/*PLANNING*/` — 初始规划
- `/*REASONING*/` — 推理步骤
- `/*ACTION*/` — 执行动作
- `/*REPLANNING*/` — 重新规划
- `/*FINAL_ANSWER*/` — 最终答案

`ProcessPlanningResponse` 会解析标签，提取规划内容。

### A2UIPlanner

A2UI 协议规划：生成符合 A2UI 规范的结构化输出，用于 UI 交互场景。

---

## 3. 需求清单

### 3.1 ReActPlanner 集成

**需求**：支持 ReAct 模式的规划

**实现要点**：
```go
import trpcreact "trpc.group/trpc-go/trpc-agent-go/planner/react"

opts = append(opts, trpcllmagent.WithPlanner(trpcreact.New(m)))
```

- 在 `BuildTRPCLLMAgent` 中增加 `DialogMode == "react"` 分支
- ReActPlanner 需要一个 Model 实例用于规划

**验收标准**：Agent 在复杂任务中先输出 PLANNING/REASONING 标签内容，再执行 ACTION

### 3.2 A2UIPlanner 集成

**需求**：支持 A2UI 协议的结构化规划

**实现要点**：
```go
import trpca2ui "trpc.group/trpc-go/trpc-agent-go/planner/a2ui"

planner := trpca2ui.New(m, trpca2ui.WithSchema(schema))
opts = append(opts, trpcllmagent.WithPlanner(planner))
```

- 在 `BuildTRPCLLMAgent` 中增加 `DialogMode == "a2ui"` 分支
- A2UIPlanner 生成符合 A2UI Schema 的结构化输出

**验收标准**：Agent 输出符合 A2UI 协议的结构化规划结果

### 3.3 规划模式可配置

**需求**：Agent 级别可配置规划模式

**实现要点**：
- 在 `AgentRuntimeSetting` 中增加 `planner_mode` 字段
- 可选值：`none`/`builtin`/`react`/`a2ui`
- 在 `BuildTRPCLLMAgent` 中根据配置选择 Planner

**验收标准**：不同 Agent 可配置不同的规划模式

### 3.4 自定义规划 Prompt

**需求**：支持自定义规划指令

**实现要点**：
- BuiltinPlanner 支持自定义 prompt
- ReActPlanner 支持自定义标签和指令模板

**验收标准**：规划指令可按 Agent 自定义

---

## 4. 涉及文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/agent/trpc_build.go` | 修改 | 增加 ReAct/A2UI Planner 分支 |
| `internal/biz/agent_types.go` | 修改 | AgentRuntimeSetting 增加 planner_mode |
| `internal/data/ent/schema/agent_runtime_setting.go` | 修改 | 增加 planner_mode 字段 |
| `api/kratos/agent/v1/agent.proto` | 修改 | 增加 planner_mode 字段 |

---

## 5. 验收标准总览

1. Agent 可配置 Builtin/ReAct/A2UI 三种规划模式
2. ReAct 模式输出 PLANNING/REASONING/ACTION 标签
3. A2UI 模式输出符合 A2UI 协议的结构化结果
4. 规划模式可在 Agent 设置中配置
5. 规划指令可自定义
