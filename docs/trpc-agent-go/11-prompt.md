# 提示词（Prompt）— 框架对齐分析

> 模块路径：`pkg/trpc-agent-go/prompt/`、`pkg/trpc-agent-go/internal/prompt/`、`pkg/trpc-agent-go/evaluation/workflow/promptiter/`
> 项目实现路径：`internal/agent/prompt*.go`、`internal/biz/prompt_refiner.go`、`internal/biz/field_guides.go`、`internal/biz/organization_position_prompt.go`、`internal/service/prompt_refine.go`、`internal/service/agent_prompt_ai.go`
> 当前对齐度：☆☆☆☆☆

---

## 一、框架能力全景

### 1.1 核心接口

| 接口 | 方法 | 说明 |
|------|------|------|
| `prompt.Source` | `FetchPrompt(ctx context.Context) (Text, error)` | 动态获取 prompt 模板，实现应自行处理缓存 |
| `prompt.Resolver` | `Resolve(ref Ref) (string, bool, error)` | 解析占位符引用，返回替换文本、是否找到、错误 |

### 1.2 关键类型

| 类型 | 说明 |
|------|------|
| `prompt.Text` | 文本 prompt 模板，含 `Template`/`Meta`/`Syntax` |
| `prompt.Meta` | Prompt 身份标识（`Name`/`Version`），用于可观测性 |
| `prompt.Vars` | `map[string]string`，渲染时的运行时变量值 |
| `prompt.RenderEnv` | 渲染环境，包含 `Vars` 和 `Resolver` |
| `prompt.Ref` | Resolver 支持的占位符引用（`Name`） |
| `prompt.Syntax` | 占位符分隔符识别模式（MixedBrace/SingleBrace/DoubleBrace） |
| `prompt.UnknownBehavior` | 未解析占位符的处理行为（PreserveUnknown/ErrorOnUnknown） |
| `prompt.RenderOption` | `func(*renderConfig)`，渲染行为配置函数 |

### 1.3 扩展点

| 扩展点 | 机制 | 适用场景 |
|--------|------|---------|
| 实现 `prompt.Source` | 接口实现 | 自定义远程 prompt 获取（如自建 prompt 管理平台） |
| 实现 `prompt.Resolver` | 接口实现 | 自定义占位符解析逻辑（如数据库查询、API 调用） |
| `prompt.RenderOption` | Option 函数 | 目前仅有 `WithUnknownBehavior`，可扩展渲染行为 |
| `langfuse.ClientOption` | Option 函数 | 自定义 HTTP 客户端 |
| `langfuse.FetchOption` | Option 函数 | `WithLabel`/`WithVersion` 选择 prompt 版本 |
| `langfuse.SourceOption` | Option 函数 | `WithCacheTTL` 配置缓存 TTL |
| PromptIter 全套接口 | 接口实现 | Backwarder/Aggregator/Optimizer/Store 均可替换 |

### 1.4 配置选项

| Option | 说明 | 默认值 |
|--------|------|--------|
| `prompt.WithUnknownBehavior(behavior)` | 未解析占位符处理方式 | `PreserveUnknown` |
| `langfuse.WithLabel(label)` | 按 label 获取 prompt | `"production"` |
| `langfuse.WithVersion(version)` | 按版本号获取 prompt | — |
| `langfuse.WithCacheTTL(ttl)` | 缓存 TTL | `60s` |
| `state.WithSession(sess)` | 覆盖 session 用于非 invocation 占位符 | 使用 invocation.Session |

### 1.5 框架内置实现

| 实现 | 路径 | 说明 |
|------|------|------|
| Langfuse Provider | `prompt/provider/langfuse/` | 从 Langfuse REST API 获取 prompt，含缓存 |
| 内部渲染引擎 | `internal/prompt/core/` | 占位符扫描、解析和渲染 |
| 状态适配层 | `internal/prompt/adapter/state/` | 将 prompt 渲染与 session/invocation 状态注入桥接 |
| PromptIter Engine | `evaluation/workflow/promptiter/engine/` | 提示词同步迭代入口 |
| PromptIter Manager | `evaluation/workflow/promptiter/manager/` | 提示词异步迭代管理 |
| PromptIter Backwarder | `evaluation/workflow/promptiter/backwarder/` | 反向传播归因 |
| PromptIter Aggregator | `evaluation/workflow/promptiter/aggregator/` | 文本梯度聚合 |
| PromptIter Optimizer | `evaluation/workflow/promptiter/optimizer/` | 优化补丁生成 |
| PromptIter Store (inmemory) | `evaluation/workflow/promptiter/store/inmemory/` | 内存存储 |
| PromptIter Store (mysql) | `evaluation/workflow/promptiter/store/mysql/` | MySQL 持久化存储 |
| PromptIter HTTP Server | `server/promptiter/` | HTTP API 服务 |

---

## 二、项目实现现状

### 2.1 框架接口实现情况

| 框架接口/功能 | 项目实现 | 合规性 | 说明 |
|--------------|---------|--------|------|
| `prompt.Source` | ❌ 未实现 | ❌ | 项目未实现框架 Source 接口，prompt 全部从 SQLite 持久层加载 |
| `prompt.Resolver` | ❌ 未实现 | ❌ | 项目未使用框架 Resolver，占位符解析完全自建 |
| `prompt.Text.Render()` | ❌ 未使用 | ❌ | 项目 prompt 组装使用 `strings.Builder` 拼接，未使用框架模板渲染 |
| `prompt.Text.ValidateRequired()` | ❌ 未使用 | ❌ | 项目无 prompt 模板验证机制 |
| 状态适配层 `state.Render()` | ❌ 未使用 | ❌ | 项目未使用框架的 session/invocation 状态注入渲染 |
| Langfuse Provider | ❌ 未使用 | ❌ | 项目未集成 Langfuse 远程 prompt 管理 |
| PromptIter Engine | ❌ 未使用 | ❌ | 项目有自建 PromptRefiner 但未使用框架 PromptIter |
| PromptIter Manager | ❌ 未使用 | ❌ | 同上 |
| PromptIter HTTP Server | ❌ 未使用 | ❌ | 同上 |

### 2.2 自建功能清单

| 自建功能 | 实现位置 | 替代框架功能 | 自建原因 |
|---------|---------|-------------|---------|
| Agent Prompt File CRUD | `internal/biz/agent_usecase.go`、`internal/data/agent_repo.go` | 框架无对应功能 | 框架不提供 prompt 文件持久化 CRUD，项目需要 Agent 级别的多文件 prompt 管理 |
| System Prompt 组装 | `internal/agent/prompt.go` — `BuildSystemPrompt()` | `prompt.Text.Render()` | 项目 prompt 不是模板渲染模式，而是按模式过滤文件后拼接 |
| Prompt Mode 过滤 | `internal/biz/agent_settings_helpers.go` — `FilesForMode()` | 框架无对应功能 | 框架无 prompt mode 概念，项目需要按 complete/task/minimized/none 过滤 prompt 文件 |
| Runtime Capability Cue | `internal/agent/prompt.go` — `StaticRuntimeCapabilityCue()`/`DynamicRuntimeCapabilityCue()` | 框架无对应功能 | 框架无运行时能力提示注入机制，项目需要动态注入工具/子代理/内存等可用性信息 |
| Runtime Cue BeforeModel Hook | `internal/agent/runtime_cue_inject.go` | 框架无对应功能 | 框架无 BeforeModel Hook 注入 runtime cue 的机制 |
| Prompt Preview | `internal/agent/prompt_preview.go` — `BuildPreviewReport()` | 框架无对应功能 | 框架无 prompt 预览功能，项目需要为设置 UI 生成预览 |
| PromptRefiner（AI 优化） | `internal/biz/prompt_refiner.go` | PromptIter Engine | PromptRefiner 是单次 AI 优化，PromptIter 是评估驱动的多轮迭代优化 |
| PromptFileAIEditor | `internal/service/agent_prompt_ai.go` | 框架无对应功能 | 框架无 prompt 文件 AI 编辑功能 |
| FieldGuide 注册表 | `internal/biz/field_guides.go` | 框架无对应功能 | 框架无 prompt 字段元数据与规范系统 |
| PositionPromptUsecase | `internal/biz/organization_position_prompt.go` | 框架无对应功能 | 框架无基于组织架构的岗位 prompt 系统 |
| 默认 Prompt 文件模板 | `internal/biz/agent_settings_helpers.go` — `defaultPromptFilesV2()` | 框架无对应功能 | 框架无 prompt 文件模板系统 |
| AI Refine Service + 速率限制 | `internal/service/prompt_refine.go` | 框架无对应功能 | 框架无 AI Refine HTTP 服务 |
| Category Responsibility 注入 | `internal/agent/trpc_build.go` — `shouldInjectCategoryResponsibility()` | 框架无对应功能 | 框架无岗位职责注入机制 |
| Industry Context 构建 | `internal/agent/prompt.go` — `BuildIndustryContext()` | 框架无对应功能 | 框架无行业上下文构建机制 |

### 2.3 未使用的框架功能

| 框架功能 | 未使用原因 | 是否需要启用 |
|---------|-----------|-------------|
| `prompt.Text.Render()` 模板渲染 | 项目 prompt 是文件拼接模式而非模板变量替换模式 | 是 — 可用于 prompt 文件内的变量替换 |
| `prompt.Text.ValidateRequired()` | 项目无模板验证需求 | 评估中 — 若启用模板渲染则应启用 |
| `prompt.Source` + Langfuse Provider | 项目使用 SQLite 持久化 + 磁盘文件，未接入 Langfuse | 否 — 当前架构不需要远程 prompt 管理 |
| `state.Render()` 状态适配 | 项目未使用框架的 session/invocation 状态注入 | 评估中 — 若启用模板渲染可配合使用 |
| PromptIter 全套 | 项目有自建 PromptRefiner，但功能远弱于 PromptIter | 是 — PromptIter 提供评估驱动的多轮迭代优化，远优于单次 AI 优化 |
| PromptIter HTTP Server | 项目未使用 | 评估中 — 可作为 PromptIter 的对外接口 |

---

## 三、对比分析

### 3.1 框架优势（项目应采纳的）

| # | 框架优势 | 项目现状 | 对齐收益 |
|---|---------|---------|---------|
| 1 | **模板渲染引擎**：`prompt.Text.Render()` 支持变量替换、可选占位符、多种语法模式、Resolver 扩展 | 项目使用 `strings.Builder` 硬拼接，无变量替换能力 | 代码减少约 50 行拼接逻辑；prompt 文件可参数化，支持运行时动态注入 |
| 2 | **状态适配层**：`state.Render()` 自动将 session/invocation 状态注入 prompt 占位符 | 项目通过 BeforeModel Hook 手动注入 Runtime Cue | 代码减少约 80 行 Hook 逻辑；状态注入声明式化，更可维护 |
| 3 | **PromptIter 评估驱动迭代**：训练集/验证集分离、反向传播归因、文本梯度聚合、优化补丁生成、接受/停止策略 | 项目 PromptRefiner 仅做单次 AI 优化，无评估闭环 | 功能增强：从"单次改写"升级为"评估驱动的多轮迭代优化"，显著提升 prompt 优化质量 |
| 4 | **PromptIter 异步运行**：Manager + Store 支持后台迭代、状态查询、取消 | 项目无异步 prompt 优化能力 | 功能增强：支持长时间 prompt 迭代任务 |
| 5 | **PromptIter HTTP Server**：开箱即用的 HTTP API | 项目需自建 API | 代码减少：无需自建 PromptIter HTTP 层 |
| 6 | **`ValidateRequired()`**：模板必需占位符验证 | 项目无验证机制 | 质量提升：运行前检测缺失变量，避免运行时错误 |

### 3.2 项目优势（框架缺失的）

| # | 项目优势 | 框架现状 | 建议处理 |
|---|---------|---------|---------|
| 1 | **Agent Prompt File 持久化 CRUD**：多文件 prompt 管理、原子创建/更新、级联删除 | 框架无 prompt 持久化能力 | 保持自建 — 框架定位是运行时渲染，不含持久化 |
| 2 | **Prompt Mode 系统**：complete/task/minimized/none 四级模式过滤 | 框架无 prompt mode 概念 | 保持自建 — 业务特有需求 |
| 3 | **Runtime Capability Cue**：动态注入工具/子代理/内存可用性信息 | 框架无对应机制 | 保持自建 — 业务特有需求，但可结合框架 `state.Render()` 简化 |
| 4 | **FieldGuide 注册表**：prompt 字段元数据与规范 | 框架无对应功能 | 保持自建 — 业务特有需求 |
| 5 | **PositionPrompt**：基于组织架构的岗位 prompt | 框架无对应功能 | 保持自建 — 业务特有需求 |
| 6 | **PromptFileAIEditor**：prompt 文件 AI 编辑 | 框架无对应功能 | 保持自建 — 可与 PromptIter 互补 |
| 7 | **AI Refine Service + 速率限制**：HTTP API + 限流 | 框架无对应功能 | 保持自建 — 可作为 PromptIter 的轻量替代入口 |
| 8 | **Prompt Preview**：为设置 UI 生成预览 | 框架无对应功能 | 保持自建 — UI 特有需求 |
| 9 | **默认 Prompt 文件模板**：PGO-1 规范的 5 文件集 | 框架无对应功能 | 保持自建 — 业务特有需求 |
| 10 | **Category Responsibility / Industry Context**：岗位职责和行业上下文注入 | 框架无对应功能 | 保持自建 — 业务特有需求 |

### 3.3 差异根因分析

| 差异点 | 根因 | 影响范围 |
|--------|------|---------|
| 未使用 `prompt.Text.Render()` | 认知缺失 — 项目开发时框架 prompt 包可能尚未稳定或未被充分了解 | `internal/agent/prompt.go` 的 `BuildSystemPrompt()` |
| 未使用 `state.Render()` | 认知缺失 — 项目未意识到框架已提供状态注入渲染适配 | `internal/agent/runtime_cue_inject.go` |
| 未使用 PromptIter | 功能缺失 + 认知缺失 — PromptIter 是框架较新功能，项目开发时可能不存在 | `internal/biz/prompt_refiner.go` |
| 自建 Prompt File CRUD | 架构决策 — 框架定位是运行时渲染，不含持久化，项目需自建 | `internal/biz/agent_usecase.go`、`internal/data/agent_repo.go` |
| 自建 Prompt Mode 系统 | 架构决策 — 业务特有的 prompt 模式过滤需求 | `internal/biz/agent_settings_helpers.go` |
| 自建 Runtime Capability Cue | 架构决策 — 业务特有的运行时能力提示需求 | `internal/agent/prompt.go` |
| 自建 FieldGuide / PositionPrompt | 架构决策 — 业务特有的 prompt 元数据和岗位 prompt 需求 | `internal/biz/field_guides.go`、`internal/biz/organization_position_prompt.go` |

---

## 四、对齐方案

### 4.1 对齐项清单

| # | 对齐项 | 类型 | 优先级 | 影响范围 | 预期收益 |
|---|--------|------|--------|---------|---------|
| 1 | 启用框架 `prompt.Text.Render()` 替换硬拼接 | 替换自建实现 | P2 | `internal/agent/prompt.go` | 代码减少约 50 行；prompt 文件可参数化 |
| 2 | 启用框架 `state.Render()` 替换手动状态注入 | 替换自建实现 | P2 | `internal/agent/runtime_cue_inject.go` | 代码减少约 80 行；状态注入声明式化 |
| 3 | 启用框架 PromptIter 替换自建 PromptRefiner | 替换自建实现 | P1 | `internal/biz/prompt_refiner.go`、`internal/service/prompt_refine.go` | 功能增强：评估驱动的多轮迭代优化 |
| 4 | 启用框架 `ValidateRequired()` | 启用框架功能 | P3 | `internal/agent/prompt.go` | 质量提升：运行前检测缺失变量 |

### 4.2 对齐项详情

#### 对齐项 #1：启用框架 `prompt.Text.Render()` 替换硬拼接

**类型**：替换自建实现

**现状**：
- 项目当前实现：`BuildSystemPrompt()` 使用 `strings.Builder` 硬拼接 Agent Description + 过滤后的 Prompt File Body
- 框架提供能力：`prompt.Text.Render()` 支持模板变量替换、可选占位符、多种语法模式、Resolver 扩展

**对齐方案**：
1. 将 Prompt File Body 中需要动态注入的部分改为框架模板语法（如 `{agent_name}`、`{variant?}`）
2. 在 `BuildSystemPrompt()` 中，对每个 Prompt File 的 Body 调用 `prompt.Text{Template: body, Syntax: prompt.SyntaxMixedBrace}.Render(env)` 替换硬拼接
3. 构建 `RenderEnv`，将运行时变量（如 agent_name、variant）注入 `Vars`，将需要动态解析的占位符通过 `Resolver` 处理
4. 保留 `<internal_config name=...>` 包裹逻辑不变

**代码变更范围**：
- 修改：`internal/agent/prompt.go` — `BuildSystemPrompt()` 改用框架渲染
- 修改：各场景 Prompt File 模板（将硬编码值改为占位符）
- 新增：`RenderEnv` 构建逻辑

**兼容性风险**：
- Prompt File Body 中若已包含 `{name}` 或 `{{name}}` 格式的文本（如 JSON 模板），会被误识别为占位符
- 缓解：使用 `SyntaxSingleBrace` 或 `SyntaxDoubleBrace` 避免冲突，或在 Prompt File 中转义

**回退方案**：
- 保留原 `BuildSystemPrompt()` 作为 fallback，通过配置开关切换

**验证方法**：
- 单元测试：对比渲染结果与原拼接结果一致
- 集成测试：Agent 运行时 system prompt 内容不变

**预期收益**：
- 代码减少：约 50 行
- 性能影响：可忽略（模板渲染开销极小）
- 维护成本：prompt 文件可参数化，减少硬编码

---

#### 对齐项 #2：启用框架 `state.Render()` 替换手动状态注入

**类型**：替换自建实现

**现状**：
- 项目当前实现：`runtime_cue_inject.go` 通过 BeforeModel Callback 手动将 Runtime Capability Cue 作为 System Message 注入
- 框架提供能力：`state.Render()` 自动将 session/invocation 状态注入 prompt 占位符（`{app:key}`、`{user:key}`、`{temp:key}`、`{invocation:key}`、`{artifact.filename}`）

**对齐方案**：
1. 评估 `state.Render()` 的占位符子集是否覆盖项目的 Runtime Cue 需求
2. 将 Runtime Cue 中的动态信息（如 effective tool keys、deny list、memory cue）存入 session/invocation state
3. 在 Prompt File 模板中使用 `{invocation:tool_keys}` 等占位符引用这些状态
4. 调用 `state.Render()` 替换 BeforeModel Hook 中的手动注入

**注意**：`state.Render()` 的占位符子集是有限的（仅支持 `app:`/`user:`/`temp:`/`invocation:`/`artifact.` 前缀），而项目的 Runtime Cue 包含大量动态生成的结构化文本（如工具列表、MCP 提示等），这些内容不适合用简单的键值对状态表示。因此，**此对齐项需要谨慎评估**，可能仅适用于部分场景。

**代码变更范围**：
- 修改：`internal/agent/runtime_cue_inject.go` — 部分替换为 `state.Render()`
- 修改：Prompt File 模板 — 添加状态占位符
- 修改：Agent 运行时 — 在 invocation state 中预设 Runtime Cue 数据

**兼容性风险**：
- `state.Render()` 的占位符子集可能不覆盖所有 Runtime Cue 需求
- Runtime Cue 的结构化文本（如工具列表）不适合简单键值对表示
- 缓解：仅对简单状态注入使用 `state.Render()`，复杂结构化内容保留 BeforeModel Hook

**回退方案**：
- 保留 BeforeModel Hook 作为主要注入方式，`state.Render()` 仅用于简单变量

**验证方法**：
- 单元测试：对比状态注入结果
- 集成测试：Agent 运行时行为不变

**预期收益**：
- 代码减少：约 30-50 行（部分替换）
- 维护成本：简单状态注入声明式化
- 性能影响：可忽略

---

#### 对齐项 #3：启用框架 PromptIter 替换自建 PromptRefiner

**类型**：替换自建实现

**现状**：
- 项目当前实现：`PromptRefiner` 做单次 AI 优化——接收原始文本 + 用户提示，调用 LLM 生成优化版本，返回 diff 和 token 统计
- 框架提供能力：PromptIter 提供评估驱动的多轮迭代优化——训练集/验证集分离、反向传播归因、文本梯度聚合、优化补丁生成、接受/停止策略、同步/异步运行、HTTP Server

**对齐方案**：
1. **Phase 1 — 集成 PromptIter 作为高级优化选项**：
   - 新增 `PromptIterService`，封装 PromptIter Engine/Manager 的创建和调用
   - 在 Service 层新增 `/v1/ai/prompt-iter` API，支持同步/异步 prompt 迭代
   - 保留现有 `PromptRefiner` 作为轻量快速优化入口
   - PromptIter 需要 `AgentEvaluator`、`Backwarder`、`Aggregator`、`Optimizer` 四个组件，需从项目的 LLM Provider 创建对应 Runner

2. **Phase 2 — 统一优化入口**：
   - 评估是否可将 `PromptRefiner` 的单次优化作为 PromptIter 的单轮特例
   - 统一前端 UI 的优化入口

**代码变更范围**：
- 新增：`internal/biz/prompt_iter.go` — PromptIter 业务逻辑封装
- 新增：`internal/service/prompt_iter.go` — PromptIter HTTP API
- 修改：Wire 注入 — 新增 PromptIter 相关依赖
- 修改：Proto 定义 — 新增 PromptIter 相关消息和服务
- 保留：`internal/biz/prompt_refiner.go` — 轻量优化入口保留

**兼容性风险**：
- PromptIter 需要 Evaluation 模块支持（评估集、指标、评估器），项目 Evaluation 模块尚未对齐（见 `10-evaluation.md`）
- PromptIter 的 Store 需要持久化（inmemory/mysql），项目使用 SQLite，需评估是否需要 SQLite Store 实现
- 缓解：Phase 1 使用 inmemory Store，Phase 2 评估是否贡献 SQLite Store 回框架

**回退方案**：
- PromptIter 为新增功能，不影响现有 PromptRefiner，可随时禁用

**验证方法**：
- 集成测试：使用示例评估集运行 PromptIter 同步迭代
- E2E 测试：通过 HTTP API 触发异步迭代并查询结果

**预期收益**：
- 功能增强：从"单次 AI 改写"升级为"评估驱动的多轮迭代优化"
- 代码减少：约 0 行（新增功能，不删除现有代码）
- 维护成本：利用框架成熟实现，减少自建优化算法的维护

---

#### 对齐项 #4：启用框架 `ValidateRequired()`

**类型**：启用框架功能

**现状**：
- 项目当前实现：无 prompt 模板验证机制
- 框架提供能力：`prompt.Text.ValidateRequired(names ...string) error` 检查模板是否包含所有必需占位符

**对齐方案**：
1. 在对齐项 #1 完成后（Prompt File 使用模板语法），在 `BuildSystemPrompt()` 中添加验证步骤
2. 对每个 Prompt File 调用 `ValidateRequired()` 检查必需变量是否存在
3. 验证失败时记录警告日志并保留原始模板（不中断运行）

**代码变更范围**：
- 修改：`internal/agent/prompt.go` — 添加 `ValidateRequired()` 调用

**兼容性风险**：
- 低 — 验证失败不中断运行，仅记录警告

**回退方案**：
- 移除验证调用即可

**验证方法**：
- 单元测试：缺失必需变量时返回错误

**预期收益**：
- 质量提升：运行前检测缺失变量，避免运行时错误
- 代码减少：约 0 行（新增约 10 行验证代码）

---

## 五、实施路线

### 5.1 阶段规划

| 阶段 | 对齐项 | 前置依赖 | 预计工作量 |
|------|--------|---------|-----------|
| Phase 1 | #3（PromptIter 集成） | Evaluation 模块对齐（`10-evaluation.md`） | 中 |
| Phase 2 | #1（模板渲染替换硬拼接） | 无 | 小 |
| Phase 3 | #2（状态注入替换） | #1 完成 | 小 |
| Phase 4 | #4（ValidateRequired） | #1 完成 | 极小 |

### 5.2 风险与缓解

| 风险 | 概率 | 影响 | 缓解措施 |
|------|------|------|---------|
| Prompt File Body 中已有 `{name}` 格式文本被误识别为占位符 | 中 | 中 | 使用 `SyntaxDoubleBrace` 避免冲突，或在模板中转义 |
| `state.Render()` 占位符子集不覆盖 Runtime Cue 需求 | 高 | 低 | 仅对简单状态注入使用 `state.Render()`，复杂内容保留 BeforeModel Hook |
| PromptIter 依赖 Evaluation 模块尚未对齐 | 高 | 高 | 先完成 Evaluation 模块对齐（`10-evaluation.md`），再集成 PromptIter |
| PromptIter Store 无 SQLite 实现 | 中 | 低 | Phase 1 使用 inmemory Store，Phase 2 评估贡献 SQLite Store |
| PromptIter Runner 创建需要 LLM Provider 适配 | 中 | 中 | 复用项目已有的 LLM Provider 体系创建 Runner |

---

## 六、附录

### A. 框架示例代码参考（必填）

| 示例 | 路径 | 关键 API | 初始化模式 | 与项目实现差异 |
|------|------|---------|-----------|--------------|
| Langfuse Prompt 集成 | `examples/prompt/langfuse/` | `promptlangfuse.NewClient()`、`Client.TextPromptSourceWithOptions()`、`source.FetchPrompt()`、`text.Render()` | Client → Source → FetchPrompt → Render → SetInstruction | 项目未使用 Langfuse，prompt 从 SQLite 加载；项目无模板渲染，使用硬拼接 |
| PromptIter 同步迭代 | `examples/evaluation/promptiter/syncrun/` | `engine.New()`、`engine.Run()`、`astructure.SurfaceID()` | Engine(New) → RunRequest → Run → RunResult | 项目有自建 PromptRefiner（单次优化），未使用 PromptIter（多轮迭代） |
| PromptIter 异步迭代 | `examples/evaluation/promptiter/asyncrun/` | `manager.New()`、`manager.Start()`、`manager.Get()` | Manager(New) → Start → Get → RunResult | 项目无异步 prompt 优化能力 |
| PromptIter HTTP 服务 | `examples/evaluation/promptiter/server/` | `spromptiter.New()`、`WithEngine()`、`WithManager()` | Server(New) → Handler → ListenAndServe | 项目需自建 API |
| PromptIter 多节点迭代 | `examples/evaluation/promptiter/multinode/` | `graphagent.New()`、`WithSubAgents()`、多 SurfaceID | GraphAgent → Engine → Run | 项目暂无 Graph Agent 场景 |

### B. 框架文档参考

| 文档 | 路径 |
|------|------|
| PromptIter 使用文档（中文） | `docs/mkdocs/zh/promptiter.md` |
| PromptIter 使用文档（英文） | `docs/mkdocs/en/promptiter.md` |
