# Planner 规划模块 — 实现设计文档

> 对应需求：`39 planner.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 规划能力：BuiltinPlanner、ReActPlanner、A2UIPlanner。对标 trpc-agent-go `planner` 包，完整实现 Planner 接口，支持三种规划模式：内置思维模型规划、ReAct 结构化规划、A2UI 协议规划。规划器通过 `llmagent.WithPlanner()` 注入 Agent，在 LLM 请求前注入规划指令、在 LLM 响应后处理规划结果。

### 核心架构

```
用户消息 → Agent.Run()
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

### Planner 接口定义

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
| 配置项 | ReasoningEffort/ThinkingEnabled/ThinkingTokens | 无 | Instruction/Schema 等 |

---

## 二、Proto 层

无需独立 Proto 服务。通过 Agent 的 `agent_runtime_settings` 表增加 `planner_mode` 和 `planner_config_json` 字段配置规划器。

### Agent Proto 扩展

```protobuf
// api/kratos/agent/v1/agent.proto — AgentRuntimeSettings 消息扩展

message AgentRuntimeSettings {
  // ... 已有字段 ...

  // 规划模式：none / builtin / react / a2ui
  string planner_mode = 50;
  // 规划器配置 JSON
  string planner_config_json = 51;
}

message PlannerConfig {
  // BuiltinPlanner 配置
  message BuiltinConfig {
    // 推理力度：low / medium / high（OpenAI o系列）；high / max（DeepSeek v4）
    string reasoning_effort = 1;
    // 是否启用思维模式（DeepSeek v4 / Claude / Gemini）
    optional bool thinking_enabled = 2;
    // 思维 Token 长度（Claude / Gemini）
    optional int32 thinking_tokens = 3;
  }

  // ReActPlanner 配置
  message ReactConfig {
    // 自定义规划指令前缀（追加到默认模板前）
    string custom_instruction_prefix = 1;
    // 是否启用 Few-Shot 示例
    bool enable_few_shot = 2;
  }

  // A2UIPlanner 配置
  message A2UIConfig {
    // 自定义指令
    string instruction = 1;
    // Server-to-Client with Standard Catalog Schema JSON
    string server_to_client_schema_json = 2;
    // Client-to-Server Schema JSON
    string client_to_server_schema_json = 3;
    // Client Capabilities Schema JSON
    string client_capabilities_schema_json = 4;
    // Server-to-Client Schema JSON
    string server_to_client_only_schema_json = 5;
    // Standard Catalog Definition JSON
    string standard_catalog_definition_json = 6;
    // Catalog Description Schema JSON
    string catalog_description_schema_json = 7;
  }

  oneof config {
    BuiltinConfig builtin = 1;
    ReactConfig react = 2;
    A2UIConfig a2ui = 3;
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type PlannerMode string

const (
    PlannerModeNone    PlannerMode = "none"
    PlannerModeBuiltin PlannerMode = "builtin"
    PlannerModeReact   PlannerMode = "react"
    PlannerModeA2UI    PlannerMode = "a2ui"
)

type BuiltinPlannerConfig struct {
    ReasoningEffort *string
    ThinkingEnabled *bool
    ThinkingTokens  *int
}

type ReactPlannerConfig struct {
    CustomInstructionPrefix string
    EnableFewShot           bool
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

type PlannerConfig struct {
    Mode    PlannerMode
    Builtin *BuiltinPlannerConfig
    React   *ReactPlannerConfig
    A2UI    *A2UIPlannerConfig
}
```

### 3.2 AgentRuntimeSettings 扩展

```go
type AgentRuntimeSettings struct {
    // ... 已有字段 ...

    PlannerMode        string
    PlannerConfigJSON  string
}
```

### 3.3 PlannerFactory

```go
type PlannerFactory struct {
    catalog *biz.LlmProviderModelUsecase
    rt      *provider.RoundTrip
}

func NewPlannerFactory(
    catalog *biz.LlmProviderModelUsecase,
    rt *provider.RoundTrip,
) *PlannerFactory

func (f *PlannerFactory) CreatePlanner(
    ctx context.Context,
    mode PlannerMode,
    config *PlannerConfig,
    m model.LLM,
) (planner.Planner, error)
```

**创建逻辑**：

```go
func (f *PlannerFactory) CreatePlanner(
    ctx context.Context,
    mode PlannerMode,
    config *PlannerConfig,
    m model.LLM,
) (planner.Planner, error) {
    switch mode {
    case PlannerModeBuiltin:
        opts := trpcbuiltin.Options{}
        if config != nil && config.Builtin != nil {
            opts.ReasoningEffort = config.Builtin.ReasoningEffort
            opts.ThinkingEnabled = config.Builtin.ThinkingEnabled
            opts.ThinkingTokens = config.Builtin.ThinkingTokens
        }
        return trpcbuiltin.New(opts), nil

    case PlannerModeReact:
        return trpcreact.New(), nil

    case PlannerModeA2UI:
        var a2uiOpts []trpca2ui.Option
        if config != nil && config.A2UI != nil {
            if config.A2UI.Instruction != "" {
                a2uiOpts = append(a2uiOpts, trpca2ui.WithInstruction(config.A2UI.Instruction))
            }
            if config.A2UI.ServerToClientSchemaJSON != "" {
                a2uiOpts = append(a2uiOpts, trpca2ui.WithServerToClientWithStandardCatalogSchema(config.A2UI.ServerToClientSchemaJSON))
            }
            if config.A2UI.ClientToServerSchemaJSON != "" {
                a2uiOpts = append(a2uiOpts, trpca2ui.WithClientToServerSchema(config.A2UI.ClientToServerSchemaJSON))
            }
            if config.A2UI.ClientCapabilitiesSchemaJSON != "" {
                a2uiOpts = append(a2uiOpts, trpca2ui.WithClientCapabilitiesSchema(config.A2UI.ClientCapabilitiesSchemaJSON))
            }
            if config.A2UI.ServerToClientOnlySchemaJSON != "" {
                a2uiOpts = append(a2uiOpts, trpca2ui.WithServerToClientSchema(config.A2UI.ServerToClientOnlySchemaJSON))
            }
            if config.A2UI.StandardCatalogDefinitionJSON != "" {
                a2uiOpts = append(a2uiOpts, trpca2ui.WithStandardCatalogDefinition(config.A2UI.StandardCatalogDefinitionJSON))
            }
            if config.A2UI.CatalogDescriptionSchemaJSON != "" {
                a2uiOpts = append(a2uiOpts, trpca2ui.WithCatalogDescriptionSchema(config.A2UI.CatalogDescriptionSchemaJSON))
            }
        }
        return trpca2ui.New(a2uiOpts...), nil

    default:
        return nil, nil
    }
}
```

### 3.4 PlannerConfig 解析

```go
func ParsePlannerConfig(mode string, configJSON string) (*PlannerConfig, error) {
    if mode == "" || mode == string(PlannerModeNone) {
        return nil, nil
    }
    cfg := &PlannerConfig{Mode: PlannerMode(mode)}
    if configJSON == "" || configJSON == "{}" {
        return cfg, nil
    }
    switch cfg.Mode {
    case PlannerModeBuiltin:
        var bc BuiltinPlannerConfig
        if err := json.Unmarshal([]byte(configJSON), &bc); err != nil {
            return nil, err
        }
        cfg.Builtin = &bc
    case PlannerModeReact:
        var rc ReactPlannerConfig
        if err := json.Unmarshal([]byte(configJSON), &rc); err != nil {
            return nil, err
        }
        cfg.React = &rc
    case PlannerModeA2UI:
        var ac A2UIPlannerConfig
        if err := json.Unmarshal([]byte(configJSON), &ac); err != nil {
            return nil, err
        }
        cfg.A2UI = &ac
    }
    return cfg, nil
}
```

---

## 四、运行时层

### 4.1 BuiltinPlanner 集成

BuiltinPlanner 专为具有内置思维能力的模型设计。它不生成显式规划指令，而是配置模型使用其内部思维机制。

**支持的模型**：
- OpenAI o-series（使用 `reasoning_effort` 参数）
- DeepSeek v4（使用 `reasoning_effort` + `thinking_enabled`）
- Claude via OpenAI API（使用 `thinking_enabled` + `thinking_tokens`）
- Gemini via OpenAI API（使用 `thinking_enabled` + `thinking_tokens`）

**运行时行为**：
- `BuildPlanningInstruction`：将 `ReasoningEffort`/`ThinkingEnabled`/`ThinkingTokens` 注入 `llmRequest`，返回空字符串
- `ProcessPlanningResponse`：返回 `nil`，不修改响应

```go
func (p *Planner) BuildPlanningInstruction(
    ctx context.Context,
    invocation *agent.Invocation,
    llmRequest *model.Request,
) string {
    if p.reasoningEffort != nil {
        llmRequest.ReasoningEffort = p.reasoningEffort
    }
    if p.thinkingEnabled != nil {
        llmRequest.ThinkingEnabled = p.thinkingEnabled
    }
    if p.thinkingTokens != nil {
        llmRequest.ThinkingTokens = p.thinkingTokens
    }
    return ""
}

func (p *Planner) ProcessPlanningResponse(
    ctx context.Context,
    invocation *agent.Invocation,
    response *model.Response,
) *model.Response {
    return nil
}
```

### 4.2 ReActPlanner 集成

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

**关键规则（注入到 LLM 的指令）**：
1. 每次响应只包含一个 Action 或 Final Answer
2. 工具调用必须使用 Function Calling API，不能在文本中写 JSON
3. Final Answer 后不再包含 PLANNING/ACTION/REASONING 标签
4. 只使用可用工具，不虚构工具
5. 不做回顾性总结

**意图描述检测逻辑**：

```go
func (p *Planner) isIntentDescription(content string) bool {
    actionTagPrefixes := []string{"/*ACTION", "/*PLANNING", "/*REPLANNING"}
    for _, prefix := range actionTagPrefixes {
        if strings.Contains(content, prefix) {
            return true
        }
    }
    intentPrefixes := []string{"I will ", "I'll ", "I am going to ", "I'm going to "}
    trimmedContent := strings.TrimSpace(content)
    for _, prefix := range intentPrefixes {
        if strings.HasPrefix(trimmedContent, prefix) {
            return true
        }
    }
    return false
}
```

### 4.3 A2UIPlanner 集成

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

### 4.4 Agent 集成 — trpc_build.go 修改

```go
func BuildTRPCLLMAgent(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) (trpcagent.Agent, error) {
    // ... 已有逻辑 ...

    // 规划器集成
    plannerMode := resolvePlannerMode(deps)
    if plannerMode != "" && plannerMode != "none" {
        plannerCfg, _ := ParsePlannerConfig(plannerMode, deps.Settings.PlannerConfigJSON)
        p, err := deps.PlannerFactory.CreatePlanner(ctx, PlannerMode(plannerMode), plannerCfg, m)
        if err != nil {
            return nil, err
        }
        if p != nil {
            opts = append(opts, trpcllmagent.WithPlanner(p))
        }
    }

    // ... 已有逻辑 ...
}

func resolvePlannerMode(deps TRPCBuilderDeps) string {
    if deps.Settings != nil && deps.Settings.PlannerMode != "" {
        return deps.Settings.PlannerMode
    }
    if strings.EqualFold(deps.DialogMode, "plan") {
        return string(PlannerModeBuiltin)
    }
    if strings.EqualFold(deps.DialogMode, "react") {
        return string(PlannerModeReact)
    }
    if strings.EqualFold(deps.DialogMode, "a2ui") {
        return string(PlannerModeA2UI)
    }
    return ""
}
```

### 4.5 TRPCBuilderDeps 扩展

```go
type TRPCBuilderDeps struct {
    Catalog         *biz.LlmProviderModelUsecase
    AgentUC         *biz.AgentUsecase
    Agents          biz.AgentRepository
    RT              *provider.RoundTrip
    SkillUC         *biz.SkillUsecase
    Sys             biz.SystemSettingRepo
    Provider        string
    Model           string
    DialogMode      string
    Settings        *biz.AgentRuntimeSettings
    PlannerFactory  *PlannerFactory
}
```

---

## 五、Data 层

### 5.1 Ent Schema 扩展

```go
// internal/data/ent/schema/agent_runtime_setting.go — 新增字段

field.String("planner_mode").Default("none"),
field.String("planner_config_json").Default("{}"),
```

### 5.2 数据库迁移

```sql
ALTER TABLE agent_runtime_settings
  ADD COLUMN planner_mode VARCHAR(32) NOT NULL DEFAULT 'none',
  ADD COLUMN planner_config_json TEXT NOT NULL DEFAULT '{}';
```

### 5.3 Biz → Data 映射

```go
func agentRuntimeSettingFromEnt(row *ent.AgentRuntimeSetting) *biz.AgentRuntimeSettings {
    return &biz.AgentRuntimeSettings{
        // ... 已有映射 ...
        PlannerMode:       row.PlannerMode,
        PlannerConfigJSON: row.PlannerConfigJSON,
    }
}
```

### 5.4 Upsert 逻辑

```go
func (r *agentRepo) UpsertAgentRuntimeSettings(ctx context.Context, v biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error) {
    builder := r.data.db.AgentRuntimeSetting.Create().
        SetAgentID(v.AgentID).
        // ... 已有字段 ...
        SetPlannerMode(v.PlannerMode).
        SetPlannerConfigJSON(v.PlannerConfigJSON).
        OnConflictColumns(agentruntimesetting.FieldAgentID).
        UpdateNewValues()
    row, err := builder.Save(ctx)
    // ...
}
```

---

## 六、Service 层

### 6.1 Agent Service 扩展

在 `GetAgentRuntimeSettings` 和 `UpsertAgentRuntimeSettings` 的 proto 映射中增加 `planner_mode` 和 `planner_config_json` 字段。

```go
func toProtoRuntimeSettings(s *biz.AgentRuntimeSettings) *v1.AgentRuntimeSettings {
    return &v1.AgentRuntimeSettings{
        // ... 已有字段 ...
        PlannerMode:       s.PlannerMode,
        PlannerConfigJSON: s.PlannerConfigJSON,
    }
}

func fromProtoRuntimeSettings(p *v1.AgentRuntimeSettings) *biz.AgentRuntimeSettings {
    return &biz.AgentRuntimeSettings{
        // ... 已有字段 ...
        PlannerMode:       p.PlannerMode,
        PlannerConfigJSON: p.PlannerConfigJSON,
    }
}
```

---

## 七、Wire 注入

### 7.1 新增 ProviderSet

```go
// internal/agent/planner_factory.go

var PlannerProviderSet = wire.NewSet(NewPlannerFactory)
```

### 7.2 Wire 注入链

```
cmd/admin/wire.go
  ├── biz.ProviderSet → AgentUsecase, LlmProviderModelUsecase
  ├── provider.ProviderSet → RoundTrip
  ├── agent.ProviderSet → PlannerFactory
  └── service.ProviderSet → ChatService (使用 PlannerFactory)
```

### 7.3 TRPCBuilderDeps 装配

```go
// internal/service/chat.go — 装配 PlannerFactory

func (s *ChatService) buildTRPCDeps(ctx context.Context, ag biz.Agent, req *chatv1.SendChatMessageRequest) agent.TRPCBuilderDeps {
    settings, _ := s.uc.GetAgentRuntimeSettings(ctx, ag.ID)
    return agent.TRPCBuilderDeps{
        // ... 已有字段 ...
        Settings:       settings,
        PlannerFactory: s.plannerFactory,
    }
}
```

---

## 八、Web 前端设计

### 8.1 页面结构

规划器配置嵌入 `AgentSettingsPage.vue` 的 Agent Tab 中，作为新的 section。

### 8.2 组件设计

#### PlannerConfigSection.vue

嵌入 `AgentSettingsPage.vue` → Agent Tab → "规划模式" section。

```vue
<template>
  <section class="settings-section">
    <div class="section-heading">
      <div>
        <div class="text-subtitle1 text-weight-bold">规划模式</div>
        <div class="text-caption text-grey-7">
          控制 Agent 在回复前如何规划行动：内置思维模型、ReAct 结构化规划或 A2UI 协议输出。
        </div>
      </div>
    </div>
    <div class="row q-col-gutter-md">
      <q-select
        v-model="plannerMode"
        class="col-12 col-md-4"
        dense
        outlined
        emit-value
        map-options
        label="规划模式"
        :options="plannerModeOptions"
      />
    </div>

    <!-- BuiltinPlanner 配置 -->
    <div v-if="plannerMode === 'builtin'" class="row q-col-gutter-md q-mt-md">
      <q-card flat bordered class="col-12">
        <q-card-section>
          <div class="text-subtitle2">内置思维配置</div>
          <div class="text-caption text-grey-7 q-mb-md">
            适用于具有内置思维能力的模型（OpenAI o系列、DeepSeek v4、Claude、Gemini）。
          </div>
          <div class="row q-col-gutter-sm">
            <q-select
              v-model="builtinConfig.reasoning_effort"
              class="col-12 col-md-4"
              dense
              outlined
              emit-value
              map-options
              label="推理力度"
              :options="reasoningEffortOptions"
              clearable
            />
            <q-toggle
              v-model="builtinConfig.thinking_enabled"
              class="col-12 col-md-4"
              color="primary"
              label="启用思维模式"
            />
            <q-input
              v-model.number="builtinConfig.thinking_tokens"
              class="col-12 col-md-4"
              dense
              outlined
              type="number"
              label="思维 Token 长度"
              hint="仅 Claude/Gemini 有效"
            />
          </div>
        </q-card-section>
      </q-card>
    </div>

    <!-- ReActPlanner 配置 -->
    <div v-if="plannerMode === 'react'" class="row q-col-gutter-md q-mt-md">
      <q-card flat bordered class="col-12">
        <q-card-section>
          <div class="text-subtitle2">ReAct 规划配置</div>
          <div class="text-caption text-grey-7 q-mb-md">
            使用 PLANNING/REASONING/ACTION/FINAL_ANSWER 标签约束 LLM 输出格式，适用于通用 LLM。
          </div>
          <div class="row q-col-gutter-sm">
            <q-input
              v-model="reactConfig.custom_instruction_prefix"
              class="col-12"
              dense
              outlined
              autogrow
              type="textarea"
              label="自定义指令前缀"
              hint="追加到默认 ReAct 规划指令模板前"
            />
            <q-toggle
              v-model="reactConfig.enable_few_shot"
              class="col-12 col-md-4"
              color="primary"
              label="启用 Few-Shot 示例"
            />
          </div>
          <q-separator class="q-my-md" />
          <div class="text-caption text-grey-7">
            <strong>ReAct 工作流</strong>：
            <ol>
              <li><code>/*PLANNING*/</code> — 创建编号计划</li>
              <li><code>/*ACTION*/</code> — 描述动作并调用工具</li>
              <li><code>/*REASONING*/</code> — 分析工具结果</li>
              <li><code>/*REPLANNING*/</code> — 重新规划（计划失败时）</li>
              <li><code>/*FINAL_ANSWER*/</code> — 最终答案</li>
            </ol>
          </div>
        </q-card-section>
      </q-card>
    </div>

    <!-- A2UIPlanner 配置 -->
    <div v-if="plannerMode === 'a2ui'" class="row q-col-gutter-md q-mt-md">
      <q-card flat bordered class="col-12">
        <q-card-section>
          <div class="text-subtitle2">A2UI 协议配置</div>
          <div class="text-caption text-grey-7 q-mb-md">
            生成符合 A2UI 规范的 JSONL 结构化输出，用于 UI 交互场景。
          </div>
          <div class="row q-col-gutter-sm">
            <q-input
              v-model="a2uiConfig.instruction"
              class="col-12"
              dense
              outlined
              autogrow
              type="textarea"
              label="自定义指令"
              hint="覆盖默认 A2UI 协议约束指令"
            />
            <q-input
              v-model="a2uiConfig.server_to_client_schema_json"
              class="col-12"
              dense
              outlined
              autogrow
              type="textarea"
              label="Server-to-Client Schema"
              hint="JSON Schema 定义服务端到客户端的消息格式"
            />
            <q-input
              v-model="a2uiConfig.client_to_server_schema_json"
              class="col-12"
              dense
              outlined
              autogrow
              type="textarea"
              label="Client-to-Server Schema"
              hint="JSON Schema 定义客户端到服务端的事件格式"
            />
            <q-input
              v-model="a2uiConfig.client_capabilities_schema_json"
              class="col-12"
              dense
              outlined
              autogrow
              type="textarea"
              label="Client Capabilities Schema"
            />
            <q-input
              v-model="a2uiConfig.standard_catalog_definition_json"
              class="col-12"
              dense
              outlined
              autogrow
              type="textarea"
              label="Standard Catalog Definition"
              hint="标准组件目录定义"
            />
            <q-input
              v-model="a2uiConfig.catalog_description_schema_json"
              class="col-12"
              dense
              outlined
              autogrow
              type="textarea"
              label="Catalog Description Schema"
            />
          </div>
        </q-card-section>
      </q-card>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive } from "vue";

const props = defineProps<{
  modelValue: { planner_mode: string; planner_config_json: string };
}>();

const emit = defineEmits<{
  "update:modelValue": [value: { planner_mode: string; planner_config_json: string }];
}>();

const plannerModeOptions = [
  { label: "无规划", value: "none" },
  { label: "内置思维 (Builtin)", value: "builtin" },
  { label: "ReAct 结构化规划", value: "react" },
  { label: "A2UI 协议规划", value: "a2ui" },
];

const reasoningEffortOptions = [
  { label: "低 (low)", value: "low" },
  { label: "中 (medium)", value: "medium" },
  { label: "高 (high)", value: "high" },
  { label: "最大 (max) — DeepSeek v4", value: "max" },
];

const plannerMode = computed({
  get: () => props.modelValue.planner_mode || "none",
  set: (v: string) => emit("update:modelValue", { ...props.modelValue, planner_mode: v }),
});

const builtinConfig = reactive({
  reasoning_effort: "",
  thinking_enabled: undefined as boolean | undefined,
  thinking_tokens: undefined as number | undefined,
});

const reactConfig = reactive({
  custom_instruction_prefix: "",
  enable_few_shot: true,
});

const a2uiConfig = reactive({
  instruction: "",
  server_to_client_schema_json: "",
  client_to_server_schema_json: "",
  client_capabilities_schema_json: "",
  standard_catalog_definition_json: "",
  catalog_description_schema_json: "",
});
</script>
```

#### PlanStepIndicator.vue

嵌入 `ChatMessagePanel.vue`，在 ReAct 模式下显示当前规划步骤状态。

```vue
<template>
  <div class="plan-steps q-mb-md">
    <q-card flat bordered class="plan-card">
      <q-card-section class="q-pa-sm">
        <div class="text-subtitle2 q-mb-xs">
          <q-icon name="account_tree" size="xs" class="q-mr-xs" />
          规划步骤
        </div>
        <q-list dense separator>
          <q-item v-for="(step, idx) in steps" :key="idx" class="q-pa-xs">
            <q-item-section avatar>
              <q-icon :name="stepIcon(step.type)" :color="stepColor(step.type)" size="sm" />
            </q-item-section>
            <q-item-section>
              <div class="text-caption">
                <q-badge :color="stepBadgeColor(step.type)" :label="step.type" class="q-mr-xs" />
                {{ step.content }}
              </div>
            </q-item-section>
          </q-item>
        </q-list>
      </q-card-section>
    </q-card>
  </div>
</template>

<script setup lang="ts">
interface PlanStep {
  type: "planning" | "reasoning" | "action" | "replanning" | "final_answer";
  content: string;
}

defineProps<{ steps: PlanStep[] }>();

function stepIcon(type: string) {
  const map: Record<string, string> = {
    planning: "lightbulb",
    reasoning: "psychology",
    action: "play_arrow",
    replanning: "refresh",
    final_answer: "check_circle",
  };
  return map[type] || "circle";
}

function stepColor(type: string) {
  const map: Record<string, string> = {
    planning: "blue",
    reasoning: "purple",
    action: "orange",
    replanning: "teal",
    final_answer: "green",
  };
  return map[type] || "grey";
}

function stepBadgeColor(type: string) {
  const map: Record<string, string> = {
    planning: "blue",
    reasoning: "purple",
    action: "orange",
    replanning: "teal",
    final_answer: "positive",
  };
  return map[type] || "grey";
}
</script>
```

#### A2UIPreviewPanel.vue

嵌入 `ChatMessagePanel.vue`，在 A2UI 模式下渲染 A2UI 协议输出的 UI 组件。

```vue
<template>
  <div class="a2ui-preview q-mb-md">
    <q-card flat bordered>
      <q-card-section class="q-pa-sm">
        <div class="text-subtitle2 q-mb-xs">
          <q-icon name="widgets" size="xs" class="q-mr-xs" />
          A2UI 渲染预览
        </div>
        <div class="a2ui-surface">
          <component
            :is="resolveComponent(msg)"
            v-for="(msg, idx) in messages"
            :key="idx"
            :data="msg"
          />
        </div>
      </q-card-section>
    </q-card>
  </div>
</template>

<script setup lang="ts">
interface A2UIMessage {
  beginRendering?: { surfaceId: string; root: string; styles?: object };
  surfaceUpdate?: { surfaceId: string; components: object[] };
  dataModelUpdate?: { surfaceId: string; data: object };
  deleteSurface?: { surfaceId: string };
}

defineProps<{ messages: A2UIMessage[] }>();

function resolveComponent(msg: A2UIMessage) {
  if (msg.beginRendering) return "A2UIBeginRendering";
  if (msg.surfaceUpdate) return "A2UISurfaceUpdate";
  if (msg.dataModelUpdate) return "A2UIDataModelUpdate";
  if (msg.deleteSurface) return "A2UIDeleteSurface";
  return "div";
}
</script>
```

### 8.3 Chat 页面集成

在 `ChatMessagePanel.vue` 中根据 Agent 的 `planner_mode` 选择渲染组件：

```typescript
// features/chat/composables/useChatWorkspace.ts

const plannerSteps = computed(() => {
  if (agentPlannerMode.value !== "react") return [];
  return parseReactSteps(currentMessage.value?.content ?? "");
});

const a2uiMessages = computed(() => {
  if (agentPlannerMode.value !== "a2ui") return [];
  return parseA2UIMessages(currentMessage.value?.content ?? "");
});

function parseReactSteps(content: string): PlanStep[] {
  const steps: PlanStep[] = [];
  const tagRegex = /\/\*(PLANNING|REASONING|ACTION|REPLANNING|FINAL_ANSWER)\*\/([\s\S]*?)(?=\/\*(?:PLANNING|REASONING|ACTION|REPLANNING|FINAL_ANSWER)\*\/|$)/g;
  let match;
  while ((match = tagRegex.exec(content)) !== null) {
    steps.push({
      type: match[1].toLowerCase() as PlanStep["type"],
      content: match[2].trim(),
    });
  }
  return steps;
}

function parseA2UIMessages(content: string): A2UIMessage[] {
  const messages: A2UIMessage[] = [];
  for (const line of content.split("\n")) {
    try {
      const obj = JSON.parse(line.trim());
      messages.push(obj);
    } catch { /* skip non-JSONL lines */ }
  }
  return messages;
}
```

### 8.4 Store 扩展

```typescript
// features/agents/types.ts

export interface AgentRuntimeSettings {
  // ... 已有字段 ...
  planner_mode: string;
  planner_config_json: string;
}

export interface BuiltinPlannerConfig {
  reasoning_effort?: string;
  thinking_enabled?: boolean;
  thinking_tokens?: number;
}

export interface ReactPlannerConfig {
  custom_instruction_prefix?: string;
  enable_few_shot?: boolean;
}

export interface A2UIPlannerConfig {
  instruction?: string;
  server_to_client_schema_json?: string;
  client_to_server_schema_json?: string;
  client_capabilities_schema_json?: string;
  standard_catalog_definition_json?: string;
  catalog_description_schema_json?: string;
}
```

### 8.5 API 扩展

```typescript
// features/agents/api.ts — 更新 AgentRuntimeSettings 的保存逻辑

export async function updatePlannerConfig(
  agentId: string,
  plannerMode: string,
  configJson: string,
): Promise<Agent> {
  return updateAgent(agentId, {
    settings: { planner_mode: plannerMode, planner_config_json: configJson },
  });
}
```

---

## 九、实现阶段

### Phase 1：基础集成（1 天）

1. `internal/data/ent/schema/agent_runtime_setting.go` 增加 `planner_mode` + `planner_config_json` 字段
2. `internal/biz/agent_types.go` 增加对应字段
3. `internal/agent/trpc_build.go` 修改 `BuildTRPCLLMAgent`，根据 `PlannerMode` 创建对应 Planner
4. `internal/agent/planner_factory.go` 新建 `PlannerFactory`
5. 数据库迁移

### Phase 2：ReAct + A2UI 模式（1 天）

1. `internal/agent/trpc_build.go` 增加 `DialogMode == "react"` 和 `"a2ui"` 分支
2. 验证 ReAct 标签解析和意图描述检测
3. 验证 A2UI Schema 注入

### Phase 3：前端配置面板（1 天）

1. `PlannerConfigSection.vue` 组件开发
2. 嵌入 `AgentSettingsPage.vue` Agent Tab
3. `PlanStepIndicator.vue` 组件开发
4. `A2UIPreviewPanel.vue` 组件开发
5. Chat 页面集成规划步骤展示

### Phase 4：测试与文档（0.5 天）

1. 单元测试：`PlannerFactory`、`ParsePlannerConfig`
2. 集成测试：三种规划模式的端到端验证
3. 前端 E2E 测试

---

## 十、验收标准

1. Agent 可配置 none/builtin/react/a2ui 四种规划模式
2. Builtin 模式正确注入 `ReasoningEffort`/`ThinkingEnabled`/`ThinkingTokens` 到 LLM 请求
3. React 模式输出 `/*PLANNING*/`/`/*REASONING*/`/`/*ACTION*/`/`/*FINAL_ANSWER*/` 标签
4. React 模式正确过滤空 ToolCall、检测意图描述防止提前终止
5. A2UI 模式输出符合 A2UI 协议的 JSONL 结构化结果
6. 规划模式可在 Agent 设置页配置
7. Builtin/A2UI 规划器参数可自定义
8. Chat 页面正确展示 ReAct 规划步骤和 A2UI 渲染预览
9. 兼容现有 `DialogMode == "plan"` 的 BuiltinPlanner 行为
