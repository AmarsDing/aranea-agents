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

### Proto 字段（已实现）

```protobuf
message AgentRuntimeSettings {
  // ...
  string planner_kind = 100;
  string code_executor_type = 101;
  string planner_config_json = 102;  // 规划器配置 JSON（形状随 planner_kind 变化）
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

### 3.2 Biz 模型（已实现）

```go
// internal/biz/agent_types.go
type AgentRuntimeSettings struct {
    PlannerKind       string
    PlannerConfigJSON string
}
```

持久化边界校验：`internal/biz/planner.go` — `ValidatePlannerKind`、`ValidatePlannerConfigJSON`。

### 3.3 运行时包结构（已实现，单一职责）

```
internal/agent/planner/
├── selector.go   # Select(dialogMode, plannerKind, plannerConfigJSON)
├── config.go     # JSON → builtin / a2ui 配置结构
└── build.go      # 构造 trpc-agent-go Planner 实例
```

选择逻辑：
- `react` → `trpcreact.New()`
- `a2ui` → `trpca2ui.New(...Option)`（非空 JSON 字段才附加 Option）
- `builtin` → `trpcbuiltin.New(Options{...})` 自 JSON
- `plannerKind` 为空且 `dialogMode == "plan"` → builtin（兼容 S1）

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
if p := agentplanner.Select(deps.DialogMode, plannerKind(ag), plannerConfigJSON(ag)); p != nil {
    opts = append(opts, trpcllmagent.WithPlanner(p))
}
```

---

## 五、Data 层（已实现）

| 层 | planner_kind | planner_config_json |
|----|-------------|---------------------|
| Proto | ✅ field 100 | ✅ field 102 |
| Biz | ✅ | ✅ |
| Service | ✅ | ✅ |
| Ent Schema | ✅ | ✅ |
| Data 映射 | ✅ `entRuntimeToBiz` / `applyBizRuntimeToCreate` | ✅ |

**迁移**：`docs/sql/02_agent_planner.sql`（已有库增量）；`docs/sql/02_agent.sql` 基线含两列。

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

### 6.2 映射（已实现）

`PlannerConfigJSON` ↔ `planner_config_json`（`internal/service/agent.go`）。

---

## 七、Web 前端设计（已实现 2026-05-21）

### 7.1 分层与文件

| 层 | 文件 | 职责（单一） |
|----|------|----------------|
| 表单契约 | `features/agents/plannerConfig.ts` | parse / serialize / `validatePlannerForm`；`VALID_REASONING_EFFORTS` 与 biz 对齐 |
| 设置 UI | `components/agents/AgentPlannerSection.vue` | 规划模式 + 空 kind 三态 banner |
| 设置编排 | `features/agents/useAgentSettingsPage.ts` | hydrate / save `planner_*` |
| 共享类型 | `features/chat/types.ts` | `Message`、`ToolUseEvent`、`ReactToolLinkIndex`、`ReactStepWithTools` |
| ReAct 类型 | `features/chat/reactPlannerTypes.ts` | `ReactStep` / `ReactParsedContent`（无解析逻辑） |
| ReAct 解析 | `features/chat/reactPlannerParse.ts` | 标签切段，无 Vue 依赖 |
| ReAct 链接 | `features/chat/reactPlannerToolLink.ts` | ACTION ↔ 后续 `tool_event` 启发式（仅索引构建时调用） |
| ReAct 索引 | `features/chat/reactToolLinkIndex.ts` | `buildReactToolLinkIndex` O(n)；`isToolLinkedInReactIndex` |
| A2UI 解析 | `features/chat/a2uiParse.ts` | JSONL 行解析 |
| A2UI 路由 | `features/chat/a2ui/a2uiKindRegistry.ts` | kind → primitive/form/layout/container |
| userAction 展示 | `features/chat/a2uiUserActionDisplay.ts` | 用户气泡 JSON 摘要 |
| 展示门面 | `features/chat/messagePlannerPresentation.ts` | `buildMessagePresentation(plannerKind, message, index, reactLinkIndex)` |
| Chat UI | `ChatMessagePanel`（必填 `reactToolLinkIndex`）、`ChatMessageRow`、`ChatReactSteps`、`ChatA2UIPreview` | 纯展示 |
| Chat 编排 | `useChatWorkspace` | `computed(buildReactToolLinkIndex(displayMessages))` → Panel |

### 7.2 Agent 设置 — 配置面板

**位置**：`AgentSettingsAgentTab.vue`，「模型」与「能力」之间嵌入 `AgentPlannerSection`。

**模式**（`planner_kind`）：

| UI | API 值 | 子表单 |
|----|--------|--------|
| 无规划（继承对话模式） | `""` | Banner：Chat「深思考」仍可触发 Builtin |
| 内置思维 | `builtin` | `reasoning_effort`、`thinking_enabled`（未设置/开/关）、`thinking_tokens` |
| ReAct | `react` | 无；`planner_config_json` 固定 `{}` |
| A2UI | `a2ui` | `instruction` + 6 个 Schema JSON（折叠高级区） |

**与 Chat `dialog_mode` 关系**（须在 UI 文案中明示）：

- `dialog_mode=plan`（会话级「深思考」）≠ `planner_kind=builtin`（Agent 级持久化）。
- `planner_kind` 为空时，仅当会话 `dialog_mode` 为 `plan` 时运行时启用 Builtin。
- **持久化**：`planner_kind=""` 时 `planner_config_json` 仅允许 `{}`；非空配置须显式选择 `builtin`/`react`/`a2ui`（`ValidatePlannerConfigJSON`）。

**空 `planner_kind` 三态**（须在 UI 与文档中区分，避免与 `builtin` 混淆）：

| 维度 | 行为 |
|------|------|
| API / 保存 | 仅 `{}`；非空 config → 400 |
| 运行时 | `planner.Select`：仅当 `dialog_mode=plan` 注入 Builtin |
| Chat 展示 | 可按正文 `/*PLANNING*/` 等启发式展示 ReAct/A2UI，**不**写入 Agent settings |

历史脏数据（`planner_kind=''` 且 `planner_config_json` 非 `{}`）见 `docs/sql/02_agent_planner_legacy_cleanup.sql`。

**`reasoning_effort`**：biz 白名单 `low|medium|high|max`（与 OpenAI o 系 / DeepSeek v4 前端选项对齐）；空字符串表示不下发，由模型默认。

**保存路径**：`validatePlannerForm` → `serializePlannerForm` → `buildSettingsPayload()` → `PATCH` Agent。

### 7.3 Chat — ReAct 步骤展示

**触发**：

1. `activePlannerKind === 'react'`；或
2. `planner_kind` 为空且正文含 `/*PLANNING*/` 等标签（启发式，兼容历史消息）。

**解析**（`reactPlannerParse.ts`）：

| 标签 | 步骤标题 |
|------|----------|
| `/*PLANNING*/` | 规划 |
| `/*REASONING*/` | 推理 |
| `/*ACTION*/` | 动作 |
| `/*REPLANNING*/` | 重新规划 |
| `/*FINAL_ANSWER*/` | 之后内容为 **主气泡 Markdown**（非步骤卡） |

**布局**（`ChatMessageRow`）：`reasoning` 折叠 + `ChatReactSteps` + 主正文可叠加（不再与 reasoning 互斥二选一）。

**ReAct ↔ `tool_call` 链接（`reactToolLinkIndex` + `reactPlannerToolLink`）**：

- 会话级：`useChatWorkspace` 在 `displayMessages` 变更时一次 `buildReactToolLinkIndex`（O(n)）；`ChatMessagePanel` / `ChatMessageRow` **必填**传入该索引。
- `buildMessagePresentation` **仅**读 `reactLinkIndex` 去重与 `stepsByAssistantIndex`；无 per-row enrich、无 O(n²) 回退；索引条目含空数组 `[]` 时信任缓存（`cached !== undefined`）。
- 规则：每个 `/*ACTION*/` 至多链接其后、下一条「实质 assistant」之前的第一个未占用 `tool_event`；工具名 hint 来自 ACTION 正文正则（`functions.*` 等）。
- **流式 / 乱序**：工具 activity 若先于 assistant 正文落库，当轮可能暂无法链接；索引随列表刷新重算，最终顺序稳定后对齐。多 ACTION / 多 tool / Team 会话不保证一一对应（见 `39-planner-development.md` backlog）。

### 7.4 Chat — A2UI 预览（MVP）

**触发**：`activePlannerKind === 'a2ui'` 或正文 JSONL 含允许键（`beginRendering` 等）。

**Chat 行为**（`ChatA2UIPreview.vue` + `ChatA2UISurface.vue`）：`reduceA2UISurface` 折叠 JSONL 为 surface；`A2UIComponentNode` 渲染 StandardCatalog 核心组件（Text/Button/Row/Column/List/Card/Modal/Tabs/Divider/Image/Icon/Video/TextField/CheckBox）；`a2uiChildren` 支持 `explicitList` 与 `template.dataBinding`。Button 点击经 `formatUserActionMessage` 作为 WS `user_message.content` 单行 JSON 上行（与 [51 消息机制](./51%20消息机制.md) §4.5 一致）。

**ReAct Chat**（`ChatReactSteps.vue`）：`reactPlannerToolLink` 将 `/*ACTION*/` 步骤与同轮次后续 `tool_call` activity 行（`options_json.tool_event`）关联，内嵌 `ChatExecutionCard` 展示。

**与 ReAct 互斥**：`messagePlannerPresentation` 先判 a2ui，再 react。

### 7.5 Builtin 在 Chat 的展示

不新增步骤卡；继续使用 `envelope.content.reasoning` → `options_json.reasoning_markdown` →「思考过程」折叠（与 ReAct 正交）。
