# Planner 规划模块 — 实现设计文档

> 对应需求：`39 planner.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 规划能力：BuiltinPlanner、ReActPlanner、A2UIPlanner。对标 trpc-agent-go `planner` 包，通过 `planner.Select()` 选择器在 Agent 构建时注入对应规划器。规划器通过 `llmagent.WithPlanner()` 注入 Agent，在 LLM 请求前注入规划指令、在 LLM 响应后处理规划结果。

### 核心架构

```
用户消息 → Agent.Run()
             ↓
         planner.Select(dialogMode, plannerKind) → 选择 Planner 实例
             ↓
         BuildPlanningInstruction() → 注入规划指令到 LLM Request
             ↓
         LLM 生成响应
             ↓
         ProcessPlanningResponse() → 处理规划标签/结构化输出
             ↓
         返回处理后的响应
```

### trpc-agent-go planner 包结构

```
pkg/trpc-agent-go/planner/
├── planner.go              # Planner 接口：BuildPlanningInstruction + ProcessPlanningResponse
├── builtin/
│   ├── builtin_planner.go  # BuiltinPlanner：模型内置思维（reasoning_effort/thinking_enabled/thinking_tokens）
│   └── builtin_planner_test.go
├── react/
│   ├── react_planner.go    # ReActPlanner：PLANNING/REASONING/ACTION/REPLANNING/FINAL_ANSWER 标签
│   └── react_planner_test.go
└── a2ui/
    ├── a2ui.go             # A2UIPlanner：A2UI 协议规划（JSONL 输出约束）
    ├── a2ui_test.go
    ├── options.go          # A2UI 选项（WithInstruction/WithSchema 等）
    └── schema.go           # A2UI Schema 定义（ClientToServer/ServerToClient/StandardCatalog）
```

### Planner 接口

```go
type Planner interface {
    BuildPlanningInstruction(ctx context.Context, invocation *agent.Invocation, llmRequest *model.Request) string
    ProcessPlanningResponse(ctx context.Context, invocation *agent.Invocation, response *model.Response) *model.Response
}
```

### 三种规划模式对比

| 特性 | BuiltinPlanner | ReActPlanner | A2UIPlanner |
|------|---------------|--------------|-------------|
| 适用模型 | 思维模型（o系列/DeepSeek v4/Claude/Gemini） | 通用 LLM | 通用 LLM |
| 规划方式 | 模型内置思维 | 标签约束输出 | JSONL 协议约束 |
| 指令注入 | 空（配置 reasoning_effort 等） | 完整规划指令模板 | A2UI 协议 Schema |
| 响应处理 | nil（不处理） | 过滤无效 ToolCall、检测意图描述 | nil（不处理） |
| 构造函数 | `builtin.New(Options)` | `react.New()` | `a2ui.New(...Option)` |
| 配置项 | ReasoningEffort/ThinkingEnabled/ThinkingTokens | 无 | Instruction/Schema 等 |

---

## 二、Proto 层

### 已有 Proto 定义

```protobuf
// api/kratos/agent/v1/agent.proto — AgentRuntimeSettings

message AgentRuntimeSettings {
  // ... 已有字段 ...

  // planner_kind selects the planning strategy: "" | "builtin" | "react" | "a2ui"
  string planner_kind = 100;
}
```

### 待扩展 Proto

为支持规划器参数配置，需增加 `planner_config_json` 字段：

```protobuf
message AgentRuntimeSettings {
  // ... 已有字段 ...

  string planner_kind = 100;
  // 规划器配置 JSON（根据 planner_kind 解析为对应结构）
  string planner_config_json = 101;
}
```

`planner_config_json` 按 `planner_kind` 值解析为不同结构：

| planner_kind | planner_config_json 结构 |
|-------------|------------------------|
| `builtin` | `{"reasoning_effort":"high","thinking_enabled":true,"thinking_tokens":8192}` |
| `react` | `{}`（当前无配置项） |
| `a2ui` | `{"instruction":"...","server_to_client_schema_json":"...","client_to_server_schema_json":"...",...}` |

---

## 三、Biz 层

### 3.1 已有领域模型

```go
// internal/biz/agent_types.go

type AgentRuntimeSettings struct {
    // ... 已有字段 ...

    // PlannerKind selects the planning strategy: "" | "builtin" | "react" | "a2ui".
    PlannerKind string
}

// internal/biz/agent_settings.go

type ContextCfg struct {
    CompactionEnabled     bool   `json:"context_compaction_enabled,omitempty"`
    SessionSummaryEnabled bool   `json:"session_summary_enabled,omitempty"`
    OutputSchemaJSON      string `json:"output_schema_json,omitempty"`
    ModelSelector         string `json:"model_selector,omitempty"`
    PlannerKind           string `json:"planner_kind,omitempty"`
}
```

### 3.2 待扩展 Biz 模型

```go
type AgentRuntimeSettings struct {
    // ... 已有字段 ...

    PlannerKind       string
    PlannerConfigJSON string
}
```

规划器配置结构（从 `PlannerConfigJSON` 解析）：

```go
type BuiltinPlannerConfig struct {
    ReasoningEffort *string
    ThinkingEnabled *bool
    ThinkingTokens  *int
}

type A2UIPlannerConfig struct {
    Instruction                     string
    ServerToClientSchemaJSON        string
    ClientToServerSchemaJSON        string
    ClientCapabilitiesSchemaJSON    string
    ServerToClientOnlySchemaJSON    string
    StandardCatalogDefinitionJSON   string
    CatalogDescriptionSchemaJSON    string
}
```

### 3.3 规划器选择器（已实现）

```go
// internal/agent/planner/selector.go

func Select(dialogMode, plannerKind string) trpcplanner.Planner
```

选择逻辑：
- `plannerKind == "react"` → `trpcreact.New()`
- `plannerKind == "a2ui"` → `trpca2ui.New()`
- `plannerKind == "builtin"` → `trpcbuiltin.New(trpcbuiltin.Options{})`
- 默认：`dialogMode == "plan"` → `trpcbuiltin.New(trpcbuiltin.Options{})`，否则 `nil`

### 3.4 选择器扩展设计

当前 `Select()` 不接受配置参数，所有规划器均使用默认值。需扩展为接受配置：

```go
func Select(dialogMode, plannerKind string, config *PlannerConfig) trpcplanner.Planner
```

扩展后逻辑：
- `plannerKind == "builtin"` → 根据 `config.Builtin` 构造 `trpcbuiltin.Options`
- `plannerKind == "a2ui"` → 根据 `config.A2UI` 构造 `trpca2ui.Option` 列表
- `plannerKind == "react"` → `trpcreact.New()`（无配置项）

---

## 四、运行时层

### 4.1 BuiltinPlanner

BuiltinPlanner 专为具有内置思维能力的模型设计。它不生成显式规划指令，而是配置模型使用其内部思维机制。

**支持的模型**：
- OpenAI o-series（使用 `reasoning_effort` 参数，可选 low/medium/high）
- DeepSeek v4（使用 `reasoning_effort` + `thinking_enabled`，reasoning_effort 可选 high/max）
- Claude via OpenAI API（使用 `thinking_enabled` + `thinking_tokens`）
- Gemini via OpenAI API（使用 `thinking_enabled` + `thinking_tokens`）

**运行时行为**：
- `BuildPlanningInstruction`：将 `ReasoningEffort`/`ThinkingEnabled`/`ThinkingTokens` 注入 `llmRequest`，返回空字符串
- `ProcessPlanningResponse`：返回 `nil`，不修改响应

### 4.2 ReActPlanner

ReActPlanner 使用标签约束 LLM 输出格式，引导 LLM 遵循结构化思考过程。

**标签体系**：

| 标签 | 用途 | 时机 |
|------|------|------|
| `/*PLANNING*/` | 初始规划 | 首次收到用户查询 |
| `/*REASONING*/` | 推理分析 | 每次工具执行后 |
| `/*ACTION*/` | 执行动作 | 需要调用工具时 |
| `/*REPLANNING*/` | 重新规划 | 初始计划失败时 |
| `/*FINAL_ANSWER*/` | 最终答案 | 有足够信息时 |

**运行时行为**：
- `BuildPlanningInstruction`：返回完整规划指令模板（含工作流说明、关键规则、规划/推理/动作/答案要求、Few-Shot 示例）
- `ProcessPlanningResponse`：
  1. 过滤空函数名的 ToolCall
  2. 检测意图描述（"I will..."开头但无实际 ToolCall）→ 标记 `Done=false` 防止提前终止
  3. 检测空 `FINAL_ANSWER` 标签 → 标记 `Done=false`

**注入到 LLM 的关键规则**：
1. 每次响应只包含一个 Action 或 Final Answer
2. 工具调用必须使用 Function Calling API，不能在文本中写 JSON
3. Final Answer 后不再包含 PLANNING/ACTION/REASONING 标签
4. 只使用可用工具，不虚构工具
5. 不做回顾性总结

### 4.3 A2UIPlanner

A2UIPlanner 生成符合 A2UI 规范的结构化输出，用于 UI 交互场景。

**A2UI 协议核心**：
- 输出必须是 JSONL 兼容（每行一个完整 JSON 对象）
- 允许的消息键：`beginRendering`、`surfaceUpdate`、`dataModelUpdate`、`deleteSurface`
- 不允许 Markdown 代码围栏或额外说明文本

**A2UI Schema 体系**：

| Schema | 用途 |
|--------|------|
| `ClientToServer` | 客户端→服务端事件（userAction/error） |
| `ServerToClientWithStandardCatalog` | 服务端→客户端消息（beginRendering/surfaceUpdate/dataModelUpdate/deleteSurface） |
| `ClientCapabilities` | 客户端能力声明 |
| `StandardCatalogDefinition` | 标准组件目录定义 |
| `CatalogDescription` | 目录描述 Schema |

**A2UI 组件类型**：
- `Text`：文本显示（支持 h1-h5/caption/body 样式）
- `Image`：图片显示（支持 icon/avatar/feature/header 样式）
- `Icon`：图标显示（预定义图标集）
- `Video`：视频播放
- `AudioPlayer`：音频播放
- `Row`/`Column`：布局容器（支持 flex 布局）
- `List`：列表容器（支持模板化子组件）
- `Button`：交互按钮（支持 action 事件）
- `TextField`：文本输入
- `Dropdown`：下拉选择
- `Switch`：开关切换
- `Carousel`：轮播
- `TabBar`：标签栏
- `WebView`：内嵌网页

**运行时行为**：
- `BuildPlanningInstruction`：注入 A2UI 协议约束指令 + Schema 定义
- `ProcessPlanningResponse`：返回 `nil`，不修改响应

**A2UI Option 列表**：

| Option | 用途 |
|--------|------|
| `WithInstruction` | 覆盖默认 A2UI 协议约束指令 |
| `WithServerToClientWithStandardCatalogSchema` | Server-to-Client with Standard Catalog Schema |
| `WithClientToServerSchema` | Client-to-Server Schema |
| `WithClientCapabilitiesSchema` | Client Capabilities Schema |
| `WithServerToClientSchema` | Server-to-Client Schema（不含 Standard Catalog） |
| `WithStandardCatalogDefinition` | Standard Catalog Definition |
| `WithCatalogDescriptionSchema` | Catalog Description Schema |

### 4.4 Agent 集成（已实现）

```go
// internal/agent/trpc_build.go

if p := agentplanner.Select(deps.DialogMode, plannerKind(ag)); p != nil {
    opts = append(opts, trpcllmagent.WithPlanner(p))
}

func plannerKind(ag biz.Agent) string {
    if ag.Settings == nil {
        return ""
    }
    return ag.Settings.PlannerKind
}
```

### 4.5 Agent 集成扩展设计

扩展 `Select()` 调用以传入配置参数：

```go
var plannerCfg *planner.PlannerConfig
if ag.Settings != nil && ag.Settings.PlannerConfigJSON != "" {
    plannerCfg, _ = planner.ParseConfig(ag.Settings.PlannerKind, ag.Settings.PlannerConfigJSON)
}
if p := agentplanner.Select(deps.DialogMode, plannerKind(ag), plannerCfg); p != nil {
    opts = append(opts, trpcllmagent.WithPlanner(p))
}
```

---

## 五、Data 层

### 5.1 当前状态

`planner_kind` 字段已在 Proto（field 100）和 Biz 层定义，但 **Ent Schema 和数据层映射缺失**：

| 层 | planner_kind | planner_config_json |
|----|-------------|-------------------|
| Proto | ✅ field 100 | ❌ 未定义 |
| Biz | ✅ `PlannerKind string` | ❌ 未定义 |
| Service | ✅ proto ↔ biz 映射 | ❌ 未定义 |
| Ent Schema | ❌ 缺失 | ❌ 未定义 |
| Data 映射 | ❌ `entRuntimeToBiz` 缺失 | ❌ 未定义 |
| Data Upsert | ❌ `applyBizRuntimeToCreate` 缺失 | ❌ 未定义 |

### 5.2 Ent Schema 扩展

```go
// internal/data/ent/schema/agent_runtime_setting.go — 新增字段

field.String("planner_kind").Default(""),
field.String("planner_config_json").Default("{}"),
```

### 5.3 数据库迁移

```sql
ALTER TABLE agent_runtime_settings
  ADD COLUMN planner_kind VARCHAR(32) NOT NULL DEFAULT '',
  ADD COLUMN planner_config_json TEXT NOT NULL DEFAULT '{}';
```

### 5.4 Biz → Data 映射

在 `entRuntimeToBiz` 中增加：

```go
PlannerKind:       e.PlannerKind,
PlannerConfigJSON: e.PlannerConfigJSON,
```

在 `applyBizRuntimeToCreate` 中增加：

```go
SetPlannerKind(v.PlannerKind).
SetPlannerConfigJSON(v.PlannerConfigJSON).
```

---

## 六、Service 层

### 6.1 已有映射

```go
// internal/service/agent.go

// fromProto:
PlannerKind: pb.GetPlannerKind(),

// toProto:
PlannerKind: b.PlannerKind,
```

### 6.2 待扩展映射

增加 `PlannerConfigJSON` 的 proto ↔ biz 映射。

---

## 七、Web 前端设计

### 7.1 当前状态

- `planner_kind` 已在 `features/agents/types.ts` 和 `wireNormalize.ts` 中定义
- 无规划模式配置 UI 组件
- 无 Chat 页面规划步骤展示

### 7.2 配置面板设计

在 `AgentSettingsPage.vue` Agent Tab 中增加"规划模式" section：

- 下拉选择规划模式：无规划 / 内置思维 (Builtin) / ReAct 结构化规划 / A2UI 协议规划
- Builtin 模式：推理力度选择、思维模式开关、思维 Token 长度输入
- A2UI 模式：自定义指令、各 Schema JSON 输入

### 7.3 Chat 页面集成

- ReAct 模式：解析 `/*PLANNING*/`/`/*REASONING*/`/`/*ACTION*/`/`/*REPLANNING*/`/`/*FINAL_ANSWER*/` 标签，以步骤卡片展示
- A2UI 模式：解析 JSONL 输出，渲染 A2UI 组件预览

### 7.4 Store 扩展

`features/agents/types.ts` 增加 `planner_config_json` 字段，wire normalization 同步更新。
