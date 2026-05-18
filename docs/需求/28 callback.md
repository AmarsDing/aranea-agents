# M28: Callback 回调 — 详细需求

> 全链路回调钩子：Agent / Model / Tool 执行前后的拦截、修改和增强。

> **2026-05-18 现状对齐**：
> - ✅ **Tool 回调已接通**：AfterTool 回调已挂载，用于记录工具调用；Plugin 运行时回调经 Runner 注入。
> - ✅ **Chain 抽象已实现**：优先级排序的回调链，含 Agent/Model/Tool 三层 Hook 接口与适配器。
> - 🟡 **Agent/Model 生命周期回调未接通**：Chain 抽象存在但未挂到 LLMAgent / Model 构造（EP-CB-01）。
> - 🟡 **Plugin 仅 AuditLog**：当前仅 audit_log 插件注册了 AfterTool，未覆盖 Agent/Model/OnEvent。
> - ❌ **产品层回调规则未实现**：Hook CRUD 可用但未与回调链路打通。
> - 进度真相以 `guides/execution-plan.md` 附录 A 为准。

---

## 1. 现状分析

| 项 | 状态 | 说明 |
|----|------|------|
| Tool AfterTool 回调 | ✅ 已接通 | 记录工具调用到 ToolInvocation |
| Plugin 运行时注入 | ✅ 已接通 | Runner 级别注入 trpcplugin.Plugin |
| Chain 回调链抽象 | ✅ 已实现 | 优先级排序 + AdaptAgent/Model/Tool |
| Agent BeforeAgent/AfterAgent | ❌ 未接通 | Chain 抽象存在但未挂到 LLMAgent |
| Model BeforeModel/AfterModel | ❌ 未接通 | Chain 抽象存在但未挂到 Model |
| PluginManager 统一管理 | ❌ 未实现 | 无聚合 Agent/Model/Tool 三层回调的管理器 |
| OnEvent 事件回调 | ❌ 未实现 | 事件流未经回调处理 |
| 产品层回调规则 | ❌ 未实现 | Hook CRUD 存在但未与回调链路打通 |

---

## 2. 术语定义

| 术语 | 定义 |
|------|------|
| **回调钩子（Callback Hook）** | 在 Agent/Model/Tool 生命周期特定点触发的拦截函数 |
| **回调链（Callback Chain）** | 多个回调按优先级排序组成的执行链，前一个输出为后一个输入 |
| **回调点（Callback Point）** | 回调触发的生命周期位置：BeforeAgent / AfterAgent / BeforeModel / AfterModel / BeforeTool / AfterTool / OnError |
| **Plugin** | 通过 PluginManager 注册的运行时插件，可跨多层回调点注入行为 |
| **Hook** | 产品层配置的回调规则，存储在 `hooks` 表，通过条件匹配触发动作 |

---

## 3. 用户故事

### US-1：审计员审计 Agent 调用

**作为** 审计员，**我希望** 在 Agent 执行前后自动记录调用信息，**以便** 追溯所有 Agent 行为。

**验收标准**：
- Agent 开始执行时记录 AgentID、SessionID、调用时间
- Agent 执行完成时记录响应摘要和执行耗时
- 审计日志可通过管理界面查询

### US-2：安全员拦截不安全请求

**作为** 安全员，**我希望** 在 LLM 调用前检查请求内容，**以便** 拦截不安全的 Prompt 注入。

**验收标准**：
- BeforeModel 回调可检查请求内容
- 回调返回拦截信号时，跳过 LLM 调用并返回预设响应
- 拦截事件被记录

### US-3：运营配置回调规则

**作为** 运营人员，**我希望** 通过界面配置回调规则，**以便** 无需修改代码即可调整回调行为。

**验收标准**：
- 可创建回调规则：指定回调点 + 条件 + 动作
- 动作类型支持：日志记录 / 通知 / 拦截 / 修改
- 规则变更后热更新，无需重启服务

### US-4：开发者扩展插件

**作为** 开发者，**我希望** 通过 Plugin 接口注册多层回调，**以便** 实现跨 Agent/Model/Tool 的横切关注点。

**验收标准**：
- Plugin 可声明关注的回调点
- Plugin 回调按优先级排序执行
- Plugin 回调错误不导致整个链路崩溃（可配置）

---

## 4. 功能需求

### 4.1 Agent 生命周期回调

**需求描述**：Agent 执行前后可注入回调函数。

**功能规格**：
- **BeforeAgent**：Agent 执行前触发；可修改 Invocation 上下文；可跳过 Agent 执行（返回自定义响应）
- **AfterAgent**：Agent 执行后触发；可修改或替换响应；可获取执行错误信息

**典型场景**：
- 审计日志：记录 Agent 调用
- 权限检查：BeforeAgent 中检查调用权限
- 响应过滤：AfterAgent 中过滤敏感信息

**验收标准**：Agent 执行前后回调正确触发，BeforeAgent 可跳过执行，AfterAgent 可替换响应。

### 4.2 Model 生命周期回调

**需求描述**：LLM 调用前后可注入回调函数。

**功能规格**：
- **BeforeModel**：LLM 调用前触发；可修改请求（如注入 system prompt、替换模型参数）
- **AfterModel**：LLM 响应后触发；可修改响应（如内容过滤、Token 统计）

**典型场景**：
- Token 计费：BeforeModel 中记录请求 Token
- 内容安全：AfterModel 中过滤不安全内容
- 请求增强：BeforeModel 中注入上下文

**验收标准**：LLM 调用前后回调正确触发，BeforeModel 可修改请求，AfterModel 可修改响应。

### 4.3 Tool 生命周期回调

**需求描述**：Tool 调用前后可注入回调函数。

**功能规格**：
- **BeforeTool**：Tool 执行前触发；可修改参数或拒绝调用
- **AfterTool**：Tool 执行后触发；可修改结果或记录日志

**典型场景**：
- 工具权限：BeforeTool 中检查调用权限
- 执行日志：AfterTool 中记录工具调用
- 参数校验：BeforeTool 中校验参数

**验收标准**：Tool 调用前后回调正确触发，BeforeTool 可拒绝调用，AfterTool 可修改结果。

### 4.4 Plugin 统一管理

**需求描述**：Runner 级别的统一回调管理，聚合 Agent/Model/Tool 三层回调。

**功能规格**：
- PluginManager 聚合所有 Plugin 的回调注册
- Plugin 可声明关注的回调点（通过 `callback_points_json`）
- PluginManager 将 Plugin 回调转换为框架原生 Callbacks
- 在 Runner 创建时注入 PluginManager

**验收标准**：PluginManager 统一管理三层回调，Plugin 回调在对应生命周期点正确触发。

### 4.5 OnEvent 事件回调

**需求描述**：Agent 运行时产生的每个事件可被回调处理。

**功能规格**：
- 事件流经 PluginManager 的 OnEvent 方法
- 可修改事件内容
- 可过滤事件

**典型场景**：
- 事件审计：记录所有事件
- 事件转换：修改事件格式
- 事件过滤：过滤敏感事件

**验收标准**：事件流经 OnEvent 回调正确处理，可修改和过滤事件。

### 4.6 回调链顺序与错误处理

**需求描述**：多个回调按优先级顺序执行，错误正确传播。

**功能规格**：
- 回调按优先级排序执行（数值越小越先执行）
- 同优先级按注册顺序执行
- Before 回调链：前一个的输出是后一个的输入
- After 回调链：前一个的输出是后一个的输入
- 任一回调返回错误时，可配置是否继续执行后续回调

**验收标准**：回调按优先级顺序执行，错误传播行为可配置。

### 4.7 产品层回调规则

**需求描述**：产品层可通过界面配置回调规则，无需修改代码。

**功能规格**：
- 回调规则存储在 `hooks` 表
- 规则定义：回调点 + 条件匹配 + 动作
- 条件匹配：按 AgentID / ToolName / 事件类型等字段匹配
- 动作类型：日志记录 / Webhook 通知 / 拦截 / 修改
- 规则变更后热更新

**验收标准**：产品层可配置回调规则，规则变更后无需重启即可生效。

---

## 5. 非功能需求

| 项 | 要求 |
|----|------|
| 性能 | 回调链执行不增加超过 50ms 延迟（单层回调） |
| 可靠性 | 回调错误不导致 Agent 运行崩溃（可配置 continue-on-error） |
| 可观测 | 回调执行次数、耗时、错误率可通过 Prometheus 指标观测 |
| 安全 | 回调不可访问其他 Agent 的会话数据 |

---

## 6. 验收标准总览

1. Agent 执行前后回调正确触发
2. LLM 调用前后回调正确触发
3. Tool 调用前后回调正确触发
4. PluginManager 统一管理三层回调
5. 事件流经 OnEvent 回调正确处理
6. 回调按优先级顺序执行，错误正确传播
7. 产品层可配置回调规则，无需修改代码
