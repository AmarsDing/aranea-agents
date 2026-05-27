# Callback 回调模块 — 实现设计文档

> 对应需求：[28 callback.md](./28%20callback.md)  
> 遵循规范：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)  
> 关联模块：[22 plugin.design.md](./22%20plugin.design.md)（内置 Plugin、Runner 注入）

---

## 一、模块概述

全链路回调：Agent / Model / Tool 生命周期拦截与增强，以及事件 OnEvent。产品层通过 **Callback Chain**（`internal/agent/callbacks`）桥接 Hook 规则与 trpc-agent-go 原生 Callback；DB Plugin 经 **Runner WithPlugins** 注入，二者边界见 §四。

---

## 二、架构总览

```
┌─────────────────────────────────────────────────────────┐
│                     产品层                               │
│  Hook CRUD (hooks) ──→ biz.HookResolver                  │
│  Plugin CRUD (plugins) ──→ plugintrpc.Runtime.Apply()    │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│  plugintrpc.Manager                                        │
│  ├─ MergeChain(agent)     ← Hook → Chain 条目            │
│  ├─ RunnerPluginsForAgent ← DB Plugin + productEventPlugin│
│  └─ OnEvent               ← 作用域过滤后转发框架 Plugin   │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│  agent.buildCallbackChainOptions (callback_chain.go)       │
│  productCallbackChain + Manager.MergeChain               │
│  → AdaptAgent/Model/ToolCallbacks → llmagent.With*       │
└──────────────────────┬──────────────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────────────┐
│  trpc-agent-go                                             │
│  agent/model/tool.Callbacks · plugin.Plugin · Runner     │
└─────────────────────────────────────────────────────────┘
```

**ModelSelector 旁路**（不在 Chain 重复 BeforeModel 路由）：`model_router`、`cost_guard` 经 `agent.PluginModelSelector` / `PluginCostGuardSelector` 在 `trpc_build.go` 装配。

---

## 三、编排边界（四层）

权威注释：`internal/plugin/trpc/orchestration.go`。

| 层 | 职责 | 排序依据 |
|----|------|----------|
| **1. Runner WithPlugins** | DB 内置 Plugin（audit_log、guards、skill_tracker 等） | `plugins.sort_order` ASC |
| **2. LLMAgent Callback Chain** | 产品固定链 + Hook 规则 | 固定优先级 + Hook `300+sort_order` |
| **3. ModelSelector** | 仅 catalog 级模型切换（router / cost_guard blocked_models） | Agent 构造时单次选择 |
| **4. Hook on_event** | 用户规则的事件处理 | `productEventPlugin` 桥接 Manager.OnEvent |

**工具确认**：统一在 Chain `BeforeTool`（ConfirmGate，priority 10）；`confirmation_guard` Runner 插件仅遥测；`permission_guard` 只处理 `deny_tools`。

---

## 四、现有代码资产

### 4.1 Chain 抽象

**位置**：`internal/agent/callbacks/`

| 类型 | 说明 |
|------|------|
| `CallbackPoint` | BeforeAgent … AfterTool、OnError |
| `Callback` + `*Hook` 接口 | 按点实现 `Handle*` |
| `Chain` | 优先级稳定排序；`Adapt*Callbacks()` 转框架类型 |
| `adapter.go` | `ToolRecorderCallback`、`Before*HookFunc` 等函数适配器 |

### 4.2 产品链装配

**位置**：`internal/agent/callback_chain.go`、`product_chain_builtins.go`

| 组件 | 说明 |
|------|------|
| `buildCallbackChainOptions` | 按链内容注入 `WithAgent/Model/ToolCallbacks` |
| `productCallbackChain` | 指标遥测 + 工具链（guard/cache/timing/confirm/recorder） |
| `buildProductCallbackChain` | 合并 `PluginManager.MergeChain` |

### 4.3 PluginManager

**位置**：`internal/plugin/trpc/manager.go`

| 方法 | 说明 |
|------|------|
| `MergeChain` | HookResolver → `HookCallbacks` + `wrapResilientHooks` |
| `RunnerPluginsForAgent` | 作用域过滤 Plugin + `productEventPlugin` |
| `OnEvent` | 按 platform agent_id 过滤后 `plugin.NewManager` 转发 |
| `ReloadHooks` | CRUD 后刷新 Hook 快照 |

### 4.4 Hook 解析

**位置**：`internal/biz/hook_config.go`、`hook_resolver.go`；执行 `internal/plugin/trpc/hook_callbacks.go`、`hook_modify.go`、`hook_events.go`

`config_json` 结构：

```json
{
  "callback_point": "before_model",
  "condition": { "agent_id": "...", "tool_name": "..." },
  "action": {
    "type": "log|notify|block|modify",
    "webhook_url": "...",
    "modify_patch": {},
    "log_level": "info"
  }
}
```

### 4.6 Hook `modify` 合并策略（before_tool）

| 字段 | 行为 |
|------|------|
| `modify_patch.arguments` | 整包替换工具参数 JSON（优先级最高） |
| `modify_patch.merge_arguments` | 与当前参数 **深度合并**；嵌套 `map` 递归合并；标量与数组由 patch 覆盖 |
| 实现 | `internal/plugin/trpc/tool_argument_merge.go` |

### 4.7 审计查询（plugin_runs）

| 来源 | `plugin_key` | 落库条件 |
|------|----------------|----------|
| 内置 Plugin | `audit_log` 等 | `blocked` / `error` |
| Hook 规则 | `hook:<hook_key>` | `blocked` / `error`（`hook_audit.go`） |

管理端 `/plugins/runs`：按 lifecycle point、agent_id、status、时间范围筛选；`plugin_key=hook:` 前缀匹配所有 Hook 记录。

### 4.5 数据表

| 表 | 关键字段 |
|----|----------|
| `plugins` | `callback_points_json`、`config_json`、`scope`、`sort_order` |
| `hooks` | `config_json`、`enabled`、`sort_order` |

### 4.8 Plugin 编排路径（P3）

| `callback_orchestration` | Runner `WithPlugins` | LLMAgent Chain |
|--------------------------|----------------------|----------------|
| `runner`（默认） | ✅ | ❌ |
| `chain` | ❌ | ✅（`PluginToChainEntries`） |

内置插件（`audit_log`、`model_router`、`cost_guard` 等）与声明 `on_event` 的插件**强制 runner**，防止双触发与 OnEvent 丢失。

**Chain 白名单**（`orchestration_policy.go` → `chainAllowlistBuiltinKeys`）：仅列出的内置插件可在 `callback_orchestration:"chain"` 时镜像到 LLMAgent Chain；当前默认 **`skill_usage_tracker`**。其余内置插件即使配置 `chain` 也会回落 Runner 并打 warn 日志。自定义插件（无内置注册）可按配置走 Chain。

Hook 规则使用 **`wrapResilientHooks`**（非 block 错误不中断回合）。Chain 镜像插件**不**套用 Hook 韧性包装，避免吞掉 Plugin 自身需上抛的错误。

### 4.9 Hook notify 投递（P3）

表 `hook_deliveries`（`docs/sql/28_callback_delivery.sql`）：`pending` → 重试 → `success`/`failed`。

`HookAction.notify_max_retries`（默认 3）、`notify_timeout_sec`（默认 8）。

- **指标**：入队成功记 `PluginInvokeTotal{status=queued}`；最终投递失败记 `delivery_failed`。
- **SSRF**：`pkg/webhookurl.ValidateNotifyURL` — 仅 http/https、禁止 loopback/私网/metadata 解析结果。
- **管理 API**：`GET /v1/hooks/deliveries`（`ListHookDeliveries`）；前端 `/hooks/deliveries`。

---

## 五、运行时注入

### 5.1 Agent 构造

```
BuildTRPCLLMAgent(deps)
  ├─ ModelSelector（router / cost_guard）
  ├─ buildCallbackChainOptions → With*Callbacks
  └─ ConfirmGate 配置来自 PluginManager.ConfirmationGuardConfigForAgent
```

### 5.2 Runner 构造

```
NewTRPCRunner
  └─ WithPlugins(PluginManager.RunnerPluginsForAgent(...))
```

### 5.3 事件流

```
Run 事件 → productEventPlugin.OnEvent → Manager.OnEvent → 各 DB Plugin
Hook on_event 规则经 HookResolver + event 桥接（非 Chain 条目）
```

---

## 六、Wire 依赖

| 组件 | 注入点 |
|------|--------|
| `biz.NewHookResolver` | `HookUsecase` |
| `plugintrpc.NewManager` | `Runtime` + `HookResolver` |
| `TRPCBuilderDeps.PluginManager` | `internal/agent/builder_deps.go` |

---

## 七、涉及文件（维护索引）

| 文件 | 职责 |
|------|------|
| `internal/agent/callbacks/*.go` | Chain 类型与适配 |
| `internal/agent/callback_chain.go` | 链装配入口 |
| `internal/agent/product_chain_builtins.go` | 产品固定链（指标等） |
| `internal/agent/tool_*.go` | 工具确认、缓存、计时、记录 |
| `internal/plugin/trpc/manager.go` | 聚合 Hook + Plugin |
| `internal/plugin/trpc/hook_*.go` | Hook 动作执行 |
| `internal/plugin/trpc/orchestration.go` | 四层边界文档 |
| `internal/biz/hook_*.go` | Hook 领域与解析 |

---

## 八、Web 前端

| 组件 | 路由 / 用途 |
|------|-------------|
| `HooksPage.vue` | `/hooks` 全局 Hook CRUD |
| `AgentHooksPanel.vue` | Agent 设置内嵌作用域 Hook |
| `CallbackEditor.vue` | `config_json` 可视化编辑 |

API：`HookService` gRPC/HTTP；Agent 设置页复用同一编辑器组件。
