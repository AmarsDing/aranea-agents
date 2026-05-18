# Callback 回调模块 — 实现设计文档

> 对应需求：[28 callback.md](./28%20callback.md)
> 遵循规范：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)

---

## 一、模块概述

全链路回调钩子：Agent/Model/Tool 执行前后的拦截、修改和增强。基于 trpc-agent-go 原生 Callback 体系，通过 `internal/agent/callbacks` Chain 抽象桥接产品层回调与框架回调。

---

## 二、架构总览

```
┌─────────────────────────────────────────────────────────┐
│                     产品层                               │
│  Hook CRUD (hooks 表)  ──→  HookRuleResolver            │
│  Plugin CRUD (plugins 表) ──→  PluginRuntime.Apply()     │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│                 回调桥接层                                │
│  Chain (internal/agent/callbacks)                        │
│  ├─ BeforeAgentHook / AfterAgentHook                     │
│  ├─ BeforeModelHook / AfterModelHook                     │
│  ├─ BeforeToolHook / AfterToolHook                       │
│  ├─ PluginCallback (PluginManager 适配)                  │
│  └─ AdaptAgentCallbacks / AdaptModelCallbacks /          │
│     AdaptToolCallbacks → 框架原生 Callbacks              │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│              trpc-agent-go 框架层                         │
│  agent.Callbacks  model.Callbacks  tool.Callbacks        │
│  plugin.Plugin / plugin.Registry                        │
│  runner.WithPlugins / llmagent.WithAgentCallbacks       │
│  llmagent.WithModelCallbacks / llmagent.WithToolCallbacks│
└─────────────────────────────────────────────────────────┘
```

---

## 三、现有代码资产

### 3.1 Chain 抽象（已实现）

**位置**：`internal/agent/callbacks/callbacks.go`

| 类型 | 说明 |
|------|------|
| `CallbackPoint` | 枚举：PointBeforeAgent / PointAfterAgent / PointBeforeModel / PointAfterModel / PointBeforeTool / PointAfterTool / PointOnError |
| `Callback` 接口 | `Point() CallbackPoint` + `Priority() int` |
| `BeforeAgentHook` | `HandleBeforeAgent(ctx, *BeforeAgentArgs) (*BeforeAgentResult, error)` |
| `AfterAgentHook` | `HandleAfterAgent(ctx, *AfterAgentArgs) (*AfterAgentResult, error)` |
| `BeforeModelHook` | `HandleBeforeModel(ctx, *BeforeModelArgs) (*BeforeModelResult, error)` |
| `AfterModelHook` | `HandleAfterModel(ctx, *AfterModelArgs) (*AfterModelResult, error)` |
| `BeforeToolHook` | `HandleBeforeTool(ctx, *BeforeToolArgs) (*BeforeToolResult, error)` |
| `AfterToolHook` | `HandleAfterTool(ctx, *AfterToolArgs) (*AfterToolResult, error)` |
| `PluginCallback` | 嵌入接口，`PluginName() string`（预留给 PluginManager） |
| `Chain` | 优先级排序的不可变回调列表，提供 `AdaptAgentCallbacks()` / `AdaptModelCallbacks()` / `AdaptToolCallbacks()` 转换为框架原生类型 |

**适配器**（`internal/agent/callbacks/adapter.go`）：

| 类型 | 说明 |
|------|------|
| `ToolRecorderCallback` | AfterTool 函数适配器，用于产品层工具调用记录 |
| `BeforeAgentHookFunc` | BeforeAgent 函数适配器 |
| `AfterAgentHookFunc` | AfterAgent 函数适配器 |

### 3.2 Tool 回调（已接通）

**位置**：`internal/agent/trpc_build.go` → `buildToolCallbacks()`

当前仅注册 AfterTool 回调（记录工具调用到 ToolInvocation），通过 `trpcllmagent.WithToolCallbacks(callbacks)` 注入 LLMAgent。

### 3.3 Plugin 运行时（已实现）

**位置**：`internal/plugin/trpc/`

| 组件 | 说明 |
|------|------|
| `Runtime` | 管理 `[]trpcplugin.Plugin`，`Apply()` 从 DB 行构建，`Plugins()` 返回快照 |
| `adapter.go` | `builtin()` 将 biz.Plugin 映射为 trpcplugin.Plugin（当前仅 audit_log） |
| `AuditLogPlugin` | 注册 AfterTool 回调，记录工具调用审计日志 |
| Runner 注入 | `internal/agent/trpc_runtime.go` 通过 `trpcrunner.WithPlugins(deps.Plugins...)` 注入 |

### 3.4 Hook CRUD（已实现）

**位置**：`internal/biz/hook.go` + `internal/data/hook.go` + `internal/service/hook.go`

Hook 为通用钩子资源，当前仅 CRUD，未与回调链路打通。

### 3.5 Plugin CRUD（已实现）

**位置**：`internal/biz/plugin.go` + `internal/data/plugin_repo.go` + `internal/service/plugin.go`

Plugin 表含 `callback_points_json` 字段，当前仅 CRUD + 启用/禁用/配置更新，未与 Chain 桥接。

---

## 四、接口设计

### 4.1 Chain 构建（扩展现有）

在 `BuildTRPCLLMAgent` 中构建 Chain 并注入 Agent/Model/Tool 回调：

```
BuildTRPCLLMAgent
  ├─ 构建 Chain = NewChain(builtinCallbacks..., pluginCallbacks..., hookCallbacks...)
  ├─ chain.AdaptAgentCallbacks() → WithAgentCallbacks()
  ├─ chain.AdaptModelCallbacks() → WithModelCallbacks()
  └─ chain.AdaptToolCallbacks()  → WithToolCallbacks()  ← 替换现有 buildToolCallbacks
```

### 4.2 PluginManager

**位置**：新建 `internal/plugin/trpc/manager.go`

PluginManager 聚合 Plugin Runtime 和 Hook 规则，生成 Chain：

```
PluginManager
  ├─ Runtime *plugintrpc.Runtime        ← 已有
  ├─ HookResolver                       ← 新增：从 hooks 表加载规则
  ├─ BuildChain(agentID) *callbacks.Chain
  │    ├─ 从 Runtime.Plugins() 收集 PluginCallback
  │    ├─ 从 HookResolver 收集匹配的 Hook 规则
  │    └─ 合并为 Chain
  └─ OnEvent(ctx, invocation, event) (*Event, error)
       └─ 遍历 Plugin 的 OnEvent 处理器
```

### 4.3 Hook 规则解析

**位置**：新建 `internal/biz/hook_resolver.go`

将 Hook 的 `config_json` 解析为 Chain 中的 Callback：

```
Hook.config_json 结构：
{
  "callback_point": "before_model",
  "condition": {
    "agent_id": "xxx",       // 可选
    "tool_name": "yyy"       // 可选
  },
  "action": {
    "type": "log|notify|block|modify",
    "webhook_url": "...",    // notify 时必填
    "modify_patch": {},      // modify 时必填
    "log_level": "info"      // log 时可选
  }
}
```

HookResolver 将匹配的 Hook 转换为对应 CallbackPoint 的 Callback 实现。

### 4.4 内置回调

| 名称 | 回调点 | 优先级 | 说明 |
|------|--------|--------|------|
| `tool_invocation_recorder` | AfterTool | 100 | 记录工具调用（现有 `buildToolCallbacks` 逻辑迁移） |
| `audit_log` | AfterTool / AfterAgent / AfterModel | 200 | 审计日志（扩展现有 AuditLogPlugin） |
| `token_usage_tracker` | AfterModel | 50 | Token 用量统计 |

---

## 五、数据模型

### 5.1 现有表

**plugins 表**（已有）：

| 字段 | 说明 |
|------|------|
| `callback_points_json` | Plugin 声明的回调点列表，如 `["after_tool","after_agent"]` |
| `config_json` | Plugin 运行时配置 |

**hooks 表**（已有）：

| 字段 | 说明 |
|------|------|
| `config_json` | 回调规则定义（callback_point + condition + action） |
| `enabled` | 是否启用 |

### 5.2 无需新建表

回调投递（Webhook 通知）复用 `hooks` 表的 `config_json.action.webhook_url`，不新建 `callback_delivery` 表。投递失败重试由 Hook 规则的动作配置决定，不在本模块实现独立投递队列。

---

## 六、运行时层

### 6.1 Agent 构造注入

修改 `internal/agent/trpc_build.go`：

```
BuildTRPCLLMAgent(deps)
  ├─ chain := deps.PluginManager.BuildChain(ag.ID)
  ├─ WithAgentCallbacks(chain.AdaptAgentCallbacks())
  ├─ WithModelCallbacks(chain.AdaptModelCallbacks())
  └─ WithToolCallbacks(chain.AdaptToolCallbacks())   ← 替换 buildToolCallbacks
```

### 6.2 Runner 注入

修改 `internal/agent/trpc_runtime.go`：

```
NewTRPCRunner(root, deps)
  └─ trpcrunner.WithPlugins(deps.Plugins...)   ← 已有，不变
```

PluginManager 不直接注入 Runner，而是通过 `BuildChain()` 在 Agent 构造时参与。Runner 级别的 Plugin 注入保持现有机制。

### 6.3 事件流

```
Agent.Run()
  └─ 事件流 → PluginManager.OnEvent(ctx, invocation, event)
       ├─ Plugin 可修改/过滤事件
       └─ 返回处理后的事件
```

OnEvent 在 Runner 的事件分发点注入，需框架支持事件拦截（trpc-agent-go `plugin.Plugin` 的 `OnEvent` 接口）。

---

## 七、Wire 注入

### 7.1 新增 Provider

```
biz.ProviderSet ← NewHookResolver
```

### 7.2 修改 Provider

```
TRPCBuilderDeps 新增字段:
  PluginManager *plugintrpc.Manager
```

---

## 八、涉及文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/plugin/trpc/manager.go` | 新建 | PluginManager：聚合 Plugin + Hook → Chain |
| `internal/biz/hook_resolver.go` | 新建 | Hook 规则解析：Hook → Callback |
| `internal/agent/trpc_build.go` | 修改 | 用 Chain 替换 buildToolCallbacks，注入 Agent/Model/Tool 回调 |
| `internal/agent/trpc_build.go` | 修改 | TRPCBuilderDeps 新增 PluginManager 字段 |
| `internal/plugin/trpc/audit.go` | 修改 | 扩展 AuditLogPlugin 覆盖 Agent/Model 回调点 |
| `internal/biz/hook.go` | 修改 | Hook 增加 CallbackPoint / Condition / Action 语义方法 |

---

## 九、Web 前端设计

### 9.1 组件

**CallbackEditor.vue**：回调配置编辑器（嵌入 Agent 设置页）

### 9.2 API

通过 `UpdateAgentSettings` 保存回调配置；通过 `HookService` CRUD 管理回调规则。
