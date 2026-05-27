# Agent 创建弹窗 — 产品设计说明（Quasar）

本文档基于「创建 Agent」界面线框，对控件、布局、行为与数据字段进行说明，并与 `agents` 主表及关联概念对齐。后端 Agent 运行时基于 trpc-agent-go 框架，Agent 构建入口为 `internal/agent/trpc_build.go` → `BuildTRPCLLMAgent`。

---

## 1. 页面定位与信息架构

| 项目 | 说明 |
|------|------|
| **入口** | Agent 管理列表页 →「创建」按钮 |
| **形态** | 居中模态对话框（`QDialog`） |
| **目标** | 采集创建 Agent 所需最小字段；可选描述由用户输入或模板填充；模型可用性经后端校验后再允许提交 |
| **成功结果** | 调用创建接口写入 `agents` 行，关闭弹窗并刷新列表（或跳转详情） |

---

## 2. 整体布局（Quasar 映射）

```
┌─────────────────────────────────────────────────────────────┐
│  创建 Agent                                           [×]    │
├─────────────────────────────────────────────────────────────┤
│  [头像]    [两列栅格]     显示名称*  | Agent标识*              │
│            [三列栅格]     行业 ▼  | 部门 ▼  | 职位 ▼          │
│            [两列栅格]     Provider* | 模型* [检查]           │
│  [单列]     描述您的 Agent                                   │
│            [模板芯片行]                                      │
│            [多行描述]                                        │
│            辅助说明文案                                       │
│  [卡片]     自我进化 .............................. [toggle]  │
├─────────────────────────────────────────────────────────────┤
│                          [取消]  [创建]                       │
└─────────────────────────────────────────────────────────────┘
```

| 区域 | 布局建议 | Quasar 组件 |
|------|-----------|-------------|
| 对话框容器 | `max-width` 约 640–720px，圆角 | `QDialog` + `QCard` 或 `QCardSection` |
| 标题栏 | 左右分布：标题 + 关闭 | `QToolbar` / `QCardSection` + `QBtn` flat icon `close` |
| 表单主体 | 响应式：`col-12 col-md-6` 两列，小屏单列 | `QForm` + `div.row.q-col-gutter-md` + `QInput` 等 |
| 描述区 | 全宽 | `QInput type="textarea"` |
| 高级项 | 全宽条带卡片 | `QCard` flat bordered 或 `QItem` + `QItemSection` |
| 页脚 | 右对齐按钮组 | `QCardActions` align="right" |

---

## 3. 控件清单（逐项）

### 3.1 显示名称 *（必填）

| 维度 | 说明 |
|------|------|
| **控件** | `QInput`，左侧 `prepend`：`QIcon`（机器人）或 `QAvatar` 小图 |
| **绑定字段** | `displayName`（提交映射 `display_name`） |
| **校验** | 非空；建议最大长度 255；trim |
| **行为** | 聚焦时可展示历史曾用名称（`QMenu` + 列表或 `QSelect` use-input），选中即回填 |
| **与库表** | `agents.display_name` |

### 3.2 Agent 标识 *（必填）

| 维度 | 说明 |
|------|------|
| **控件** | `QInput` |
| **占位** | 例：`my-agent` |
| **辅助文案** | 小写字母、数字、连字符（与截图一致） |
| **校验** | 正则：`^[a-z0-9]+(-[a-z0-9]+)*$`；长度上限与 `agent_key` 一致（如 100）；创建前查重（未软删行唯一） |
| **行为** | 失焦或防抖后可选调用「可用性检查」接口；冲突时 inline error |
| **与库表** | `agents.agent_key`（文档中原「Agent标识」即业务侧 slug，对应库中 `agent_key`） |

### 3.3 业务分类（Agent Type 组件，行业 / 部门 / 职位）

与技术指标 `agents.agent_type`（如 `open`）**不同**：本块为 **业务画像分类**，数据模型见 `4.agent-type.md`（`agent_category_nodes` + `agents.category_position_id`）。前端可封装为独立组件，例如 `AgentCategoryCascade.vue`（或项目内统称 `AgentTypeSelect`）。

| 维度 | 说明 |
|------|------|
| **控件** | 三个级联 `QSelect`（或 1 个组件内嵌三列）：**行业** → **部门** → **职位**；小屏可改为垂直堆叠或 `QUploader` 式步骤条（二期）。 |
| **占位** | 选择行业 / 选择部门 / 选择职位 |
| **数据源** | `GET /agent-categories/tree` 一次取树前端展开，或按父 id 懒加载 `GET /agent-categories?parent_id=`（与 `4.agent-type.md` §7 一致）。 |
| **绑定与提交** | 仅 **职位**（第 3 层叶子）写入接口：`category_position_id`；行业、部门仅用于联动与展示，不单独落库。 |
| **校验** | **可选**：默认三项均可为空（`category_position_id` null）；若产品要求必选，则职位标 `*` 且 `QForm` 校验 `category_position_id` 非空。 |
| **联动** | 切换行业 → 清空部门、职位并加载该行业下部门；切换部门 → 清空职位并加载该部门下职位。 |
| **辅助** | 可选 `QBtn`「管理分类」跳转 `/settings/agent-categories`（`4.agent-type.md` §5）。 |
| **与库表** | `agents.category_position_id`；列表/检索可用冗余 `category_path`（服务端拼接或异步刷新）。 |

### 3.4 Provider *（必填）

| 维度 | 说明 |
|------|------|
| **控件** | `QSelect`，可过滤、`emit-value` `map-options` |
| **占位** | 选择 Provider |
| **数据源** | 后端 `/providers` 或配置中心；选项：`{ label, value }`，`value` 为库中存储的供应商标识（字符串 slug/code，与业务一致即可） |
| **行为** | 变更时可清空已选模型并置「模型检查」为未通过 |
| **与库表** | `agents.provider` |

> 说明：若库注释写「UUID」，以实现为准：表单层仍选「可读名称」，存库为稳定 id/slug。

### 3.5 模型 *（必填）+ 检查

| 维度 | 说明 |
|------|------|
| **控件** | `QInput` + 尾部 `QBtn`「检查」；可选 `QMenu` 展示历史模型名 |
| **绑定字段** | `modelName`（提交映射 `model`） |
| **占位** | 输入或选择模型 |
| **「检查」启用条件** | `provider` 与 `modelName` 均非空（可 trim 后判断） |
| **点击「检查」** | `POST /agents/validate-model` 或等价：body `{ provider, model }`；成功则 `modelCheckPassed = true` 并 toast/绿色提示；失败则错误信息、`modelCheckPassed = false` |
| **与库表** | `agents.model` |

### 3.6 描述您的 Agent（模板 + 多行文本）

| 维度 | 说明 |
|------|------|
| **区块标题** | 描述您的 Agent |
| **模板芯片** | `QChip` / `QBtn` toggle 样式，横向 `QScrollArea` 或 `row wrap` |
| **模板项（与截图一致）** | 小狐、程序员、客服、写手、翻译、小罗、米米（各带图标） |
| **行为** | 点击芯片：将预设 prompt 写入描述框（**替换**或 **追加**由产品定；建议首次为空时替换，已有内容时二次确认或追加） |
| **多行输入** | `QInput` `filled` `type="textarea"`，`rows`/`min-height` 约 200px 等效 |
| **辅助说明** | 「AI 将根据此描述自动生成 Agent 的上下文文件。留空则使用模板。」 |
| **与库表** | 主要映射 `agents.agent_description`（长描述/人设）；若留空且选了模板，服务端用模板 id 解析默认文案；可选扩展：`other_config.template_id` 或独立列 `description_template_key`（见 §5 扩展建议） |

### 3.7 自我进化

| 维度 | 说明 |
|------|------|
| **控件** | `QToggle`，卡片内右侧对齐 |
| **文案** | 标题：自我进化；说明：允许 Agent 通过 SOUL.md 随时间进化其风格和语调 |
| **默认值** | 与 `2 agent.md` 一致：**默认开启**（`true`） |
| **与库表** | `agents.self_evolve` |

### 3.8 头像

**关联 `50 Avatar.md`**：本节只约定 **创建/编辑表单内的触点** 与 **`agents.icon` 含义**；头像 **库表、BLOB、`AgentAvatarPicker`、出图 API** 等均以 **`50 Avatar.md`** 为准，与本节 **成对维护**。

与「显示名称」同一行或名称字段 `prepend`：

| 维度 | 说明 |
|------|------|
| **控件** | `QAvatar` ~100px；点击打开 **`AgentAvatarPicker`**（见 `50 Avatar.md` §2） |
| **行为** | 内置图库 + 本地上传 + 裁剪 → 二进制入 **`avatar_assets`**；表单回写 **`avatar_assets.id`** 至 `agents.icon` |
| **与库表** | `agents.icon` 仅存资源 id；列表/预览用 **`GET /avatar-assets/{id}/thumbnail` 或 `/file`**；`emoji` 与 icon 二选一由 UI 定 |

### 3.9 页脚按钮

| 控件 | 类型 | 行为 |
|------|------|------|
| **取消** | `QBtn` outline/flat | 关闭对话框；若有未保存修改可 `QDialog` 二次确认（可选） |
| **创建** | `QBtn` color=primary | **建议规则**：`QForm` 校验通过 **且** `modelCheckPassed === true` 才可点；点击后 `loading`，调用创建 API，成功关闭并通知列表 |
| **关闭** | 标题栏 `QBtn` icon | 同取消，注意与「取消」行为一致 |

---

## 4. 前端状态与校验摘要

| 状态字段 | 用途 |
|-----------|------|
| `modelCheckPassed` | 模型检查是否通过，控制「创建」 |
| `submitting` | 提交中禁用双按钮 |
| `agentKeyAvailable` | 可选：标识唯一性校验结果 |
| `categoryPositionId` | 可选：业务分类选中的职位节点 id，提交为 `category_position_id` |

**校验顺序建议**：客户端必填/格式 →（可选）agent_key 唯一 → 模型检查 → 提交。

---

## 5. 表单字段 ↔ 数据库 `agents` 表

以下为**本弹窗直接相关**列；完整列清单仍以主表设计为准。

| 表单字段 / UI | 数据库列 | 类型（参照原设计） | 备注 |
|---------------|----------|-------------------|------|
| 显示名称 | `display_name` | VARCHAR(255) | 必填 |
| Agent 标识 | `agent_key` | VARCHAR(100) | 未软删唯一 |
| 业务分类（行业→部门→职位） | `category_position_id` | TEXT/UUID（与库一致） | 可选；仅绑定职位叶子，见 `4.agent-type.md` |
| 分类展示路径（可选冗余） | `category_path` | TEXT | 可由服务端维护 |
| Provider | `provider` | VARCHAR(50) | 存 slug/id |
| 模型 | `model` | VARCHAR(200) | 检查通过后写入 |
| 描述（多行） | `agent_description` | TEXT | 可空；空则服务端按模板默认 |
| 所选模板 | （扩展）`description_template_key` 或 `other_config.templateId` | VARCHAR / JSONB | 便于审计与复现 |
| 自我进化 | `self_evolve` | BOOLEAN | 默认 true（产品确认） |
| 头像资源 id | `icon` | VARCHAR(255) | 对应 `avatar_assets.id`；图片存库 BLOB，非外链 URL |
| — | `id` | UUID | 服务端生成 |
| — | `status` | VARCHAR(20) | 默认 `active` |
| — | `created_at` / `updated_at` | TIMESTAMPTZ | 服务端维护 |

**创建时不一定要在表单出现、可由默认填充的列**（与 `2 agent.md` 一致，供接口层默认值）：`context_window`、`max_tool_iterations`、`workspace`、`restrict_to_workspace`、`tools_config`、**`agent_type`（技术类型，如 `open`，≠ 业务分类组件）**、`is_default`、`frontmatter`（可由描述异步生成）、`embedding`（异步任务）等。

---

## 6. API 建议（实现参考）

| 接口 | 方法 | 说明 |
|------|------|------|
| 拉取 Provider 列表 | GET | 下拉选项 |
| 业务分类树 / 子节点 | GET | `4.agent-type.md`：`/agent-categories/tree` 或 `?parent_id=` |
| 校验模型 | POST | Provider + model，返回是否可用及可选 `context_window` 提示 |
| 检查 agent_key | GET/POST | 唯一性 |
| 创建 Agent | POST | body 为表单 + 默认值合并 |
| 描述模板列表 | GET | 芯片文案与 icon（若模板走服务端） |

---

## 7. 验收要点（产品）

- [ ] 必填项标 `*` 与提交前错误提示一致  
- [ ] Agent 标识符合「小写字母、数字、连字符」且唯一  
- [ ] **业务分类**：行业/部门/职位级联正确；提交仅带 `category_position_id`；与 `agents.agent_type` 无混淆  
- [ ] 未做模型「检查」时「创建」不可点（按既定规则）  
- [ ] 模板芯片与描述框、服务端生成上下文文件的约定一致（空描述 = 模板）  
- [ ] 自我进化默认开启，与 `self_evolve` 一致  
- [ ] **头像**：点击 `QAvatar` 打开选图流程；`agents.icon` 为资源 id，与 **`50 Avatar.md`** 一致  
- [ ] 深色主题下对比度与焦点态可读  

---

## 8. trpc-agent-go 对齐需求（M2 Agent 构建）

> 本节补充 `plan.md` M2 模块的对齐需求，确保 Agent 构建完全复刻 trpc-agent-go `llmagent` 能力。

### 8.1 占位符变量

**trpc 框架**：`llmagent.New` 支持 `WithInstruction` 中的 `{key}` 占位符，运行时由 `agent.Invocation` 的 `Variables` 替换。

**需求**：
- Agent 的 `system_prompt` 支持 `{variable_name}` 占位符
- 运行时从 `Invocation.Variables` 注入实际值
- 内置变量：`{agent_name}`、`{session_id}`、`{user_id}`、`{current_date}`
- 自定义变量：通过 `AgentRuntimeSetting.variables_json` 配置

**涉及文件**：`internal/agent/trpc_build.go`

**验收标准**：Agent Instruction 中 `{key}` 被正确替换为运行时值

### 8.2 ModelInstructions

**trpc 框架**：`llmagent.WithModelInstructions` 为不同模型注入不同的指令片段。

**需求**：
- `AgentRuntimeSetting` 增加 `model_instructions_json` 字段
- 格式：`{"gpt-4o": "你是一个精确的助手", "claude-3": "你是一个有创意的助手"}`
- `BuildTRPCLLMAgent` 中通过 `WithModelInstructions` 注入

**涉及文件**：`internal/agent/trpc_build.go`、`internal/biz/agent_types.go`

**验收标准**：不同模型使用不同的指令片段

### 8.3 ContextCompaction

**trpc 框架**：`llmagent.WithContextCompaction` 启用上下文自动压缩，当 token 接近上限时自动摘要。

**需求**：
- `AgentRuntimeSetting` 增加 `context_compaction` 布尔字段
- `BuildTRPCLLMAgent` 中通过 `WithContextCompaction` 启用
- 压缩策略：保留最近 N 轮 + 摘要历史

**涉及文件**：`internal/agent/trpc_build.go`、`internal/biz/agent_types.go`

**验收标准**：长对话自动压缩，不丢失关键信息

### 8.4 SessionSummary

**trpc 框架**：`llmagent.WithSessionSummary` 启用会话摘要，新 session 可加载旧 session 摘要。

**需求**：
- `AgentRuntimeSetting` 增加 `session_summary` 布尔字段
- `BuildTRPCLLMAgent` 中通过 `WithSessionSummary` 启用
- Session 结束时自动生成摘要
- 新 Session 可通过摘要继承上下文

**涉及文件**：`internal/agent/trpc_build.go`、`internal/biz/agent_types.go`

**验收标准**：新 Session 可通过摘要继承旧 Session 上下文

### 8.5 SkillLoadMode

**trpc 框架**：`llmagent.WithSkillLoadMode` 控制技能加载策略（auto/manual/none）。

**需求**：
- `AgentRuntimeSetting` 增加 `skill_load_mode` 字段
- 可选值：`auto`（自动匹配）、`manual`（手动指定）、`none`（不加载）
- `BuildTRPCLLMAgent` 中通过 `WithSkillLoadMode` 注入

**涉及文件**：`internal/agent/trpc_build.go`、`internal/biz/agent_types.go`

**验收标准**：Agent 按配置策略加载技能

### 8.6 StructuredOutput

**trpc 框架**：`llmagent.WithStructuredOutput` 强制 LLM 输出符合 JSON Schema。

**需求**：
- `AgentRuntimeSetting` 增加 `output_schema_json` 字段
- `BuildTRPCLLMAgent` 中通过 `WithStructuredOutput` 注入
- LLM 输出自动校验和解析

**涉及文件**：`internal/agent/trpc_build.go`、`internal/biz/agent_types.go`

**验收标准**：Agent 输出符合预定义的 JSON Schema

### 8.7 ModelSelector

**trpc 框架**：`llmagent.WithModelSelector` 动态选择模型。

**需求**：
- `AgentRuntimeSetting` 增加 `model_selector` 字段
- 可选值：`default`（使用配置模型）、`auto`（根据任务复杂度选择）
- `BuildTRPCLLMAgent` 中通过 `WithModelSelector` 注入

**涉及文件**：`internal/agent/trpc_build.go`、`internal/biz/agent_types.go`

**验收标准**：Agent 根据任务复杂度动态选择模型

---

*文档版本：与 `2 agent.md` 主表、`4.agent-type.md` 业务分类及 **`50 Avatar.md`（§3.8 头像）** 对齐；界面以当前「创建 Agent」弹窗为准。trpc 对齐需求见 §8。A2A 远程代理创建见 §9。*

---

## 9. Agent 类型：LLM 与 A2A 远程代理（2026-05-20）

> 完整 A2A 协议与 Endpoint 语义见 [26 a2a-protocol.md](./26%20a2a-protocol.md) §2.5。技术设计见 [26 a2a-protocol.design.md](./26%20a2a-protocol.design.md) §十一。

### 9.1 类型选择

创建对话框顶部增加 **Agent 类型**（单选，默认 LLM）：

| 类型 | 说明 | 用户可见标识 |
|------|------|--------------|
| **LLM Agent** | 现有行为：Provider + Model + 描述 + 进化等 | 无额外徽章 |
| **A2A 远程代理** | 接入外部 A2A 服务，无需本地 LLM | 列表/卡片 `A2A ↗` |

与 `agents.agent_type`（技术开放类型，如 `open`）、`category_position_id`（业务分类）**三者独立**。

### 9.2 LLM Agent 表单（默认）

与 §3 一致，无变更。

### 9.3 A2A 远程代理表单

选择「A2A 远程代理」后：

| 控件 | 必填 | 说明 |
|------|------|------|
| 显示名称、Agent 标识、头像、业务分类 | 同 LLM | 与 §3.1–3.3、§3.8 一致 |
| **远程 URL** | ✅ | A2A 服务根地址；旁侧「发现 AgentCard」按钮 |
| 流式响应 | — | `QToggle`，默认跟随远程 Card |
| 超时（秒） | — | 默认 30 |
| 鉴权类型 | — | none / api_key / mtls；展开对应配置项 |

**隐藏或禁用**：Provider、模型检查、描述模板（可选保留简短描述）、自我进化、记忆/Skill 相关（创建时不展示）。

**提交字段**：`agent_kind=a2a_proxy` + `a2a_proxy_config`（见设计文档）；`provider`/`model` 可为空或占位。

### 9.4 创建后行为

- 列表刷新并展示 `A2A ↗` 徽章
- 可选：自动跳转 Agent 设置 → A2A Tab 展示只读远程 AgentCard
- Chat 侧栏可选该 Agent，交互与 LLM Agent 一致

### 9.5 验收要点（A2A Proxy）

- [ ] 类型切换时表单字段正确显隐
- [ ] 远程 URL 为空时不可提交
- [ ] 「发现 AgentCard」成功后可预览远程名称与 capabilities
- [ ] 创建成功后 Chat 可选用该 Agent
- [ ] 与 LLM 创建流程互不干扰（默认仍为 LLM）
