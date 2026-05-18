# M11: Planner 规划 — 详细需求

> 对标 `pkg/trpc-agent-go/planner` 包，实现 Agent 规划能力。

---

## 1. 现状分析

项目已集成 BuiltinPlanner、ReActPlanner、A2UIPlanner 三种规划器的运行时选择逻辑，通过 `planner_kind` 字段和 `dialogMode` 参数在 Agent 构建时选择对应规划器。当前存在以下不足：

- 规划器参数不可配置（BuiltinPlanner 的 reasoning_effort/thinking_enabled/thinking_tokens、A2UIPlanner 的 Schema 等均使用默认值）
- `planner_kind` 字段未持久化到数据库（Ent Schema 缺失，数据层映射缺失）
- 前端无规划模式配置 UI
- Chat 页面无 ReAct 规划步骤展示和 A2UI 渲染预览

---

## 2. 需求清单

### 2.1 规划模式可配置

**用户故事**：作为平台管理员，我希望为不同 Agent 配置不同的规划模式，以便 Agent 根据任务类型选择合适的规划策略。

**功能规格**：
- Agent 级别可配置规划模式，可选值：`""`（默认，兼容旧行为）、`builtin`、`react`、`a2ui`
- 配置通过 `AgentRuntimeSettings.planner_kind` 字段持久化
- 兼容现有 `DialogMode == "plan"` 的 BuiltinPlanner 行为（planner_kind 为空时回退到 dialogMode 判断）

**验收标准**：不同 Agent 可配置不同的规划模式，配置持久化且重启后生效

### 2.2 BuiltinPlanner 参数可配置

**用户故事**：作为平台管理员，我希望配置 BuiltinPlanner 的推理参数（reasoning_effort、thinking_enabled、thinking_tokens），以便针对不同思维模型优化推理效果。

**功能规格**：
- `reasoning_effort`：推理力度，OpenAI o系列可选 low/medium/high，DeepSeek v4 可选 high/max
- `thinking_enabled`：是否启用思维模式，适用于 DeepSeek v4 / Claude / Gemini
- `thinking_tokens`：思维 Token 长度，适用于 Claude / Gemini

**验收标准**：BuiltinPlanner 的推理参数可在 Agent 设置中配置，参数正确注入到 LLM 请求

### 2.3 A2UIPlanner 参数可配置

**用户故事**：作为平台管理员，我希望配置 A2UIPlanner 的协议参数（自定义指令、Schema 定义），以便针对不同 UI 交互场景定制 A2UI 输出。

**功能规格**：
- 自定义指令：覆盖默认 A2UI 协议约束指令
- Server-to-Client with Standard Catalog Schema：服务端到客户端消息格式
- Client-to-Server Schema：客户端到服务端事件格式
- Client Capabilities Schema：客户端能力声明
- Server-to-Client Schema：服务端到客户端消息格式（不含 Standard Catalog）
- Standard Catalog Definition：标准组件目录定义
- Catalog Description Schema：目录描述 Schema

**验收标准**：A2UIPlanner 的协议参数可在 Agent 设置中配置，参数正确注入到规划指令

### 2.4 前端规划模式配置面板

**用户故事**：作为平台管理员，我希望在 Agent 设置页面配置规划模式和参数，以便无需手动修改数据库即可调整规划策略。

**功能规格**：
- 在 Agent 设置页 Agent Tab 中增加"规划模式"配置区域
- 下拉选择规划模式：无规划 / 内置思维 / ReAct 结构化规划 / A2UI 协议规划
- 根据所选模式动态展示对应参数配置表单
- BuiltinPlanner：推理力度选择、思维模式开关、思维 Token 长度输入
- A2UIPlanner：自定义指令、各 Schema JSON 输入

**验收标准**：规划模式和参数可在前端 Agent 设置页配置并保存

### 2.5 Chat 页面规划步骤展示

**用户故事**：作为用户，我希望在 Chat 页面看到 ReAct 规划器的步骤分解和 A2UI 的渲染预览，以便理解 Agent 的推理过程。

**功能规格**：
- ReAct 模式：解析 `/*PLANNING*/`/`/*REASONING*/`/`/*ACTION*/`/`/*REPLANNING*/`/`/*FINAL_ANSWER*/` 标签，以步骤卡片形式展示
- A2UI 模式：解析 JSONL 输出，渲染 A2UI 组件预览

**验收标准**：ReAct 模式下 Chat 页面展示规划步骤卡片；A2UI 模式下展示渲染预览

---

## 3. 验收标准总览

1. Agent 可配置 none/builtin/react/a2ui 四种规划模式
2. 规划模式配置持久化到数据库，重启后生效
3. Builtin 模式正确注入推理参数到 LLM 请求
4. React 模式输出 PLANNING/REASONING/ACTION/FINAL_ANSWER 标签
5. A2UI 模式输出符合 A2UI 协议的 JSONL 结构化结果
6. Builtin/A2UI 规划器参数可在前端配置
7. Chat 页面正确展示 ReAct 规划步骤和 A2UI 渲染预览
8. 兼容现有 `DialogMode == "plan"` 的 BuiltinPlanner 行为
