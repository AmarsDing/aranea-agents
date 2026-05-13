# Agent 设置页 — 产品设计说明（Quasar）

本文档描述 **单 Agent 详情/设置** 界面：顶栏、主导航 Tab、各分区控件与行为，并与 **`agents` 主表**、JSON 策略列及 **`2 agents-create.md` / `50 Avatar.md` / `4.agent-type.md`** 对齐。入口一般为列表点击卡片或 `GET /agents/:id` 后进入。

---

## 1. 页面定位

| 项目 | 说明 |
|------|------|
| **路由建议** | `/agents/:id/settings` |
| **用户目标** | 查看运行态摘要；编辑身份、模型、系统提示模式、能力、记忆、进化、钩子、文件与权限等 |
| **保存策略** | 分区 **自动保存**（debounce PATCH）或 **显式保存** 由产品二选一；本文按「字段失焦/区块保存」描述 |

---

## 2. 整体结构

```
┌──────────────────────────────────────────────────────────────────────────┐
│ [←] [头像] programer ●  [任务][V3][进化中]     [系统提示词][收藏][设置][删除] │
│     programer · deepseek / deepseek-v4-flash                               │
├──────────────────────────────────────────────────────────────────────────┤
│  Agent │ 文件 │ 权限 │ 进化 │ 钩子 │ 用户实例        （QTabs / QRouteTab）   │
├──────────────────────────────────────────────────────────────────────────┤
│  （当前 Tab 内容区：见 §3～§9）                                              │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## 3. 顶栏（全局）

| 控件 | 行为 | 数据 |
|------|------|------|
| **返回** | `QBtn` flat，`router.back()` 或回列表 | — |
| **头像** | `QAvatar` 方形圆角；`src` 见 `50 Avatar.md`；**点击**打开 `AgentAvatarPicker`，保存后刷新 `agents.icon` | `icon` → `avatar_assets.id` |
| **显示名 + 在线点** | 主标题；绿点表示 `status === active` 等 | `display_name`、`status` |
| **标签 chips** | 如「任务」「V3」「进化中」；只读或跳转对应配置 | `tags`、`agent_type` 版本、`self_evolve` 等推导 |
| **副标题** | `{agent_key} · {provider} / {model}` | 只读摘要 |
| **系统提示词** | 眼形图标 + 文案；打开侧滑/对话框展示当前系统提示全文（只读或编辑视权限） | 运行态生成或 `compaction_config` / 派生存储 |
| **收藏** | 心形；未设置/已收藏 | 同列表 `user_agent_favorites` |
| **设置齿轮** | 可打开「高级」抽屉或跳转子路由（与 Tab 不重复则省略） | — |
| **删除** | `QBtn`；二次确认后软删，回列表 | `deleted_at` |

---

## 4. 主导航 Tab

| Tab | 内容概要 |
|-----|-----------|
| **Agent** | 系统提示模式、Agent 个性、模型与预算、能力（子 Agent / 工具策略）、记忆、TTS、心跳/技能/编排等（见各节；可 `QScrollArea` 长页） |
| **文件** | 工作区文件浏览（`workspace`、`restrict_to_workspace`）；单独 PRD |
| **权限** | 谁能使用该 Agent；单独 PRD |
| **进化** | §7 |
| **钩子** | 钩子列表与编辑（与 Agent 区内「钩子」摘要卡片可二选一或互链） |
| **用户实例** | 多租户/用户级覆盖；单独 PRD |

实现：`QTabs` + `QTabPanels` 或 Vue Router 子路由 `children`。

---

## 5. Tab「Agent」— 系统提示模式

四张可选 **卡片**（`QCard` + `clickable`），单选；当前项橙色描边。

| 模式 | 标题 | 说明摘要（产品文案） |
|------|------|----------------------|
| **完整** | 完整 | 交互聊天 + 完整人格类能力 |
| **任务** | 任务 | 企业自动化、记忆、进化；弱化/无人格口吻 |
| **最小化** | 最小化 | 后台任务、核心规则、仅观察 |
| **无** | 无 | 纯工具调用自动化，无规则与人格 |

| 维度 | 说明 |
|------|------|
| **展示** | 每卡：标题、描述、特性 tag 列表、约 **~XK tokens** 估值（服务端可返回） |
| **绑定** | `system_prompt_mode`：`complete` \| `task` \| `minimized` \| `none`（存 `agents` 主表 `system_prompt_mode` 列） |
| **行为** | 切换即 PATCH；可 `Notify`「已切换模式」 |

### 运行时映射（trpc-agent-go）

`system_prompt_mode` 在 `BuildTRPCLLMAgent` 中控制哪些 `AgentPromptFile` 注入到系统提示：

| 模式 | 注入的文件 | 代码实现 |
|------|-----------|---------|
| `complete` | 全部文件（AGENTS_CORE + AGENTS_TASK + SOUL + IDENTITY + USER + USER_PREDEFINED + CAPABILITIES + RULE + HEARTBEAT） | `FilesForMode(files, "complete")` → 返回全部 |
| `task` | AGENTS_CORE + AGENTS_TASK + IDENTITY + CAPABILITIES + RULE + HEARTBEAT | `FilesForMode(files, "task")` → allowed 集合 |
| `minimized` | AGENTS_CORE + IDENTITY + RULE | `FilesForMode(files, "minimized")` → allowed 集合 |
| `none` | 无文件注入 | `FilesForMode(files, "none")` → 空集 |

**代码实现**：

- `BuildTRPCLLMAgent`（`internal/agent/trpc_build.go`）读取 `ag.SystemPromptMode` 并传递给 `BuildSystemPrompt`
- `BuildSystemPrompt`（`internal/agent/prompt.go`）接收 mode 参数，调用 `biz.FilesForMode` 过滤文件
- `FilesForMode`（`internal/biz/agent_catalog_legacy.go`）已导出，根据模式返回允许的文件子集
- 每个文件内容用 `<internal_config name="{Name}">` 标签包裹，便于 LLM 区分配置块

---

## 6. Tab「Agent」— Agent 个性

| 控件 | Quasar | 字段 |
|------|--------|------|
| **图标** | `QAvatar` + 点击 → **`50 Avatar.md`** | `icon` |
| **显示名称** | `QInput` | `display_name` |
| **专业摘要** | `QInput` textarea + 下方「LLM 预览」辅助行 | `frontmatter` 或 `agent_description`（与 `2 agents-create` 分工一致：短摘要优先 `frontmatter`） |
| **状态** | `QSelect`：活跃/停用… | `status` |
| **默认 Agent** | `QToggle` | `is_default` |
| **Agent 标识** | `QInput` **只读** + 复制 `QBtn` | `agent_key` |

**业务分类**：若创建页已支持 `category_position_id`，本页可只读展示路径或同级 `AgentCategoryCascade` 只编辑分类；见 **`4.agent-type.md`**。

---

## 7. Tab「Agent」— 模型与预算

| 控件 | 字段 |
|------|------|
| **Provider** | `QSelect` | `provider` |
| **模型** | `QSelect` + 校验 | `model` |
| **上下文窗口** | `QInput` type=number + 辅助说明 | `context_window` |
| **最大工具迭代** | `QInput` type=number | `max_tool_iterations` |
| **预算限额 (USD)** | `QInput` 前缀 `$` | `budget_monthly_cents`（分为单位存储时换算）或 `budget` |

变更 Provider/模型建议复用创建页 **模型检查** 逻辑（可选）。

---

## 8. Tab「Agent」— 语音合成（TTS）

| 控件 | 说明 |
|------|------|
| 空态 | 静音图标 +「未配置 TTS」+ 说明文案 |
| **配置 TTS** | `QBtn` → 打开对话框：选择 TTS Provider、音色、密钥等 |
| **存储** | `tts` 或 `sandbox_config` 扩展，结构由后端约定 |

---

## 9. Tab「Agent」— 能力

**页面分区标题**：能力 — Agent 可以使用的功能。

### 9.1 子 Agent

**子区块标题**：子 Agent — 控制子 Agent 生成限制和行为。

| 控件 | 说明 |
|------|------|
| **总开关** | 右侧 `QToggle`；关闭时隐藏下方参数（或置灰并忽略提交） |
| **布局** | 开关开启后，参数置于浅色描边容器内；**两列栅格**（`QGrid` / `row` + `col-12 col-md-6`）排列字段 |
| **标签** | 各字段标签旁 **`(i)`** → `QTooltip` / `QMenu` 说明 |

| 字段 | Quasar | 示例 / 占位 | `subagents_config` 建议键（与后端对齐） |
|------|--------|---------------|----------------------------------------|
| **最大并发数** | `QInput` type=number，min≥1 | `20` | `max_concurrency` |
| **最大生成深度** | `QSelect` 或 `QInput`（正整数） | `1` | `max_generation_depth` |
| **每 Agent 最大子数** | `QInput` type=number，min≥1 | `5` | `max_children_per_agent` |
| **归档时间（分钟）** | `QInput` type=number，min≥1 | `60` | `archive_after_minutes` |
| **最大重试次数** | `QInput` type=number，min≥0 | `2` | `max_retries` |
| **模型覆盖** | `QInput` 或 `QSelect`（可选模型列表） | 占位「继承自 Agent」：空表示不覆盖，沿用本 Agent 的 `provider` / `model` | `model_override`（空串或 `null` 表示继承） |

**继承逻辑**：未填写「模型覆盖」时，运行时子 Agent 使用父 Agent 在 §7 中配置的 Provider/模型；填写后仅子 Agent 调用链使用该覆盖值（具体以 `前端.md` / 服务端约定为准）。

### 9.2 工具策略

**子区块标题**：工具策略 — 控制此 Agent 可以使用哪些工具。

| 控件 | 说明 |
|------|------|
| **总开关** | 右上 `QToggle`；关闭时整块策略不生效（或回退系统默认），可隐藏内部表单 |
| **表单容器** | 开启后，配置项置于 **深色描边/卡片** 内，与截图一致 |
| **底部操作** | 右下 **`保存更改`**（磁盘图标 + 主色按钮）：若本区未做自动保存，点此提交 PATCH；与全局 debounce 策略二选一并在产品中统一 |

#### 9.2.1 配置文件与工具名前缀

| 字段 | Quasar | 说明 / 占位 | `tools_config` 建议键 |
|------|--------|-------------|----------------------|
| **配置文件** | `QSelect` | 预置策略包，示例值 **`full`**（完整内置工具能力）；选项由后端/注册表下发 | `profile` 或 `config_profile` |
| **工具调用前缀** | `QInput` | 占位 `e.g. proxy_`；**辅助说明**（字段下方小字）：从模型的工具调用名称中去除此前缀后再查找注册表 | `tool_call_prefix`（空串表示不剥离） |

**「工具调用前缀」`(i)` 文案（可与界面中英混排一致）**：在根据注册表解析工具前，先剥离模型返回的工具名前缀。示例：前缀为 `proxy_` 时，`proxy_exec` 解析为 `exec`。支持 `{tool_name}` 等占位符（具体以后端实现为准）。

#### 9.2.2 允许 / 拒绝 / 同时允许

三组均为 **`QSelect` 多选 + `use-chips`**（或等价），可从下拉按 **分组「内置」** 勾选；选中项以 **可删除 tag** 展示。标签旁 **`(i)`**：

| 列表 | 产品语义（实现以服务端为准） |
|------|-------------------------------|
| **允许** | 白名单：仅这些工具 id 可被调度（在配置文件策略之上进一步收窄；若与「默认全开」组合，以产品约定为准） |
| **拒绝** | 黑名单：明确禁止的工具 id，**即使出现在允许列表中也不应生效**（见 §9.2.3） |
| **同时允许** | 允许 **同一轮次 / 并行** 同时发起的工具子集（通常为「允许 ∩ 未拒绝」的子集），用于限制并行组合或并发安全 |

下拉 **「内置」** 分组展示 **展示名（英文）+ 技术名 `tool_id`**，与注册表一致。线稿中出现的内置项示例（**非穷举**，以运行时注册表为准）：

| 展示名（示例） | `tool_id` |
|----------------|-----------|
| Web / Browser | `browser` |
| Edit File | `edit` |
| List Files | `list_files` |
| Read File | `read_file` |
| Write File | `write_file` |
| Create Video | `create_video` |
| Create Audio | `create_audio` |
| Create Image | `create_image` |
| Read Audio | `read_audio` |
| Read Document | `read_document` |
| Read Image | `read_image` |
| Read Video | `read_video` |
| Speech-to-Text | `stt` |


| 键 | 类型 | 说明 |
|----|------|------|
| `enabled` | boolean | 对应总开关 |
| `profile` | string | 如 `full` |
| `tool_call_prefix` | string | 前缀剥离 |
| `allow` | string[] | 工具 id |
| `deny` | string[] | 工具 id |
| `concurrent_allow`（或 `simultaneous_allow`） | string[] | 「同时允许」列表 |

#### 9.2.3 冲突与校验（产品必做）

截图中 **同一 `tool_id` 同时出现在「允许」与「拒绝」** 属于无效配置。建议至少满足其一：

1. **保存时校验**：提示冲突项，禁止提交或要求用户修正；  
2. **运行时规则**：约定 **拒绝优先于允许**（最终可调用集合 = `(allow 语义 ∩ profile) − deny`），并在 UI 上以 **Banner** 提示「以下工具在允许与拒绝中重复，已按拒绝处理：…」；  
3. **输入时联动**：向「拒绝」添加某 id 时自动从「允许」移除（或反之），避免并存。

**「同时允许」** 中的 id 若不在「最终可调用集合」内，应 **警告** 或 **自动剔除**，避免并行白名单引用不可用工具。

---

## 10. Tab「Agent」— 记忆

**标题**：记忆 — 语义记忆搜索和嵌入配置。

### 10.1 记忆总开关

| 控件 | 存储 |
|------|------|
| 「已启用」`QToggle` | `memory_config.enabled`（或顶层约定键） |

### 10.2 检索参数（栅格）

| 字段 | 示例值 | `memory_config` 键 |
|------|--------|---------------------|
| 最大块长度 | 1000 | `max_chunk_length` |
| 块重叠 | 200 | `chunk_overlap` |
| 最大结果数 | 6 | `max_results` |
| 最低分数 | 0.35 | `min_score` |
| 向量权重 | 0.7 | `vector_weight` |
| 文本权重 | 0.3 | `text_weight` |

标签旁 `(i)` → 工具提示。

### 10.3 Dreaming（记忆整合）

| 控件 | 存储 |
|------|------|
| 说明文案 | 后台将会话摘要提升为长期记忆 |
| 「已启用」`QToggle` | `memory_config.dreaming.enabled` |
| 阈值 | `memory_config.dreaming.threshold`（如 5） |
| 防抖间隔 (ms) | `memory_config.dreaming.debounce_ms`（如 600000） |
| 「详细日志」`QToggle` | `memory_config.dreaming.verbose_logs` |

---

## 11. Tab「Agent」— 心跳 / 钩子 / 技能 / 编排（卡片组）

纵向 **多张 `QCard`**，与截图一致。


| 卡片 | 内容 | 存储 |
|------|------|------|
| **心跳** | 见 §11.1 | 上表三字段  |
| **钩子** | 空态 +「+ 添加钩子」；列表与表单见 **`16. hook.md`** | `hooks` 表 + `hook_agents`（替代 `hooks`） |
| **技能** | 搜索框「筛选 Skill…」+ 列表 `0/0` | 技能注册表 + `skills` |
| **Pinned Skills** | 英文说明 + `0/10` | `pinned_skills` 数组 |
| **Orchestration** | 标签 `team` + Team 展示如「研究院」 | `subagents_config`（团队/编排） |


### 11.1 心跳

| 项目 | 说明 |
|------|------|
| **卡片状态图标** | **未设置心跳**：**空心**图标；**已设置并保存**（含间隔、正文等持久化成功）：**红色实心**图标 |
| **间隔** | 图标时间 `QInput`框 标注单位:min，默认值：30 |
| **编辑区标题** | 示例：**检查清单 (HEARTBEAT.MD)**，旁绿色文档图标 |
| **编辑区说明** | 每次心跳运行时注入的指令，支持 Markdown。 |
| **正文编辑** | `QInput` textarea 或 Markdown 编辑器；典型内容如一级标题「心跳检查清单」、待办 bullet（检查待处理任务、报告状态等） |

**Agent 设置侧持久化（`agents` 或与 Agent 设置等价表）— 心跳相关字段建议：**

| 字段（产品含义） | 类型 | 建议列名或 JSON 路径 |
|------------------|------|----------------------|
| 是否开启 | boolean | `heartbeat_enabled` |
| 时间间隔（分钟） | integer，≥1 | `heartbeat_interval_minutes`  |
| HEARTBEAT.MD  | text（Markdown） | 库表列常用 `heartbeat_md`；产品与文件命名对齐时常称 **heartbeat.md**  |

**调度执行**：间隔与是否开启由运行时读库触发；注入内容每次心跳从 `heartbeat_md`读取。
---

## 12. Tab「进化」

页面标题 **进化**；垂直列表项 + `QToggle` + 分隔线。

| 项 | 标题 | 说明 | 字段 |
|----|------|------|------|
| 1 | 允许 Agent 进化其沟通风格 | SOUL.md 更新语调与风格；身份与操作指令锁定 | `self_evolve` |
| | 信息框 | 强调仅风格/语调变，身份与工作流规则固定 | `QBanner` / `QCard` bordered |
| 2 | 允许从经验中创建和管理技能 | `skill_manage` 默认可用；提醒保存工作流为技能 | `skill_evolve`、`skill_nudge_interval`（见扩展列） |
| 3 | 进化指标 | 记录工具效果、检索质量、反馈 | `evolution_metrics_enabled` |
| 4 | 进化建议 | 基于指标生成改进建议 | `evolution_suggestions_enabled` |

与 **`2 agents-create.md`** 中「自我进化」默认值策略对齐；本页为后续修改入口。

---

## 13. 字段 ↔ `agents` 与 JSON 汇总

| UI 区域 | 列或 JSON |
|---------|-----------|
| 顶栏 / 个性 | `display_name`、`agent_key`、`icon`、`status`、`is_default`、`frontmatter` / `agent_description` |
| 模型与预算 | `provider`、`model`、`context_window`、`max_tool_iterations`、`budget_monthly_cents` |
| 系统提示模式 | `system_prompt_mode`（建议） |
| TTS | `tts` |
| 能力 | `tools_config`（`enabled`、`profile`、`tool_call_prefix`、`allow`、`deny`、`concurrent_allow` / `simultaneous_allow` 等）；`subagents_config`（见 §9.1）；键名以 `前端.md` 为准 |
| 记忆 | `memory_config` |
| 进化 Tab | `self_evolve`、`skill_evolve`、`*` |
| 心跳 | `heartbeat_enabled`、`heartbeat_interval_minutes`、`heartbeat_md`（**heartbeat.md** 清单正文）；或 `heartbeat` 内 `enabled` / `interval_minutes` / `markdown` |
| 钩子 | `hooks`、`hook_agents`（**`16. hook.md`**） |
| 技能/编排 | `skills` 等；编排见 `subagents_config` |
| 分类 | `category_position_id`、`category_path`（`4.agent-type.md`） |

完整列清单仍以 **`前端.md`** / 迁移脚本为准。

---

## 14. API 建议

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/agents/:id` 或 `/:agentKey` | 详情 DTO（含解析后的标签、模式、记忆配置等） |
| PATCH | `/agents/:id` | 部分更新；各区块提交合并 JSON 时注意深度 merge |
| DELETE | `/agents/:id` | 软删 |
| GET | `/avatar-assets/...` | 头像出图（`50 Avatar.md`） |

---

## 15. 验收要点

- [ ] 顶栏信息与列表/详情一致；头像点击打开 **`AgentAvatarPicker`**。  
- [ ] Tab 切换保留未保存策略符合产品（自动保存或提示）。  
- [ ] 系统提示模式四选一与 `other_config`（或等价列）持久化一致。  
- [ ] 模型、预算、记忆数值校验与后端一致。  
- [ ] 工具策略：总开关、`profile`、`tool_call_prefix`、允许/拒绝/同时允许 与 `tools_config` 一致；内置工具选项与注册表 id 一致；**允许∩拒绝** 有明确校验或「拒绝优先」提示。  
- [ ] 子 Agent 总开关、六项参数（并发、深度、每 Agent 子数、归档分钟、重试、模型覆盖）与 `subagents_config` 一致。  
- [ ] 进化四项与 `self_evolve` / `skill_evolve` / `other_config` 一致。  
- [ ] 心跳：未配置空心 / 已配置红色实心；**是否开启**、**间隔（分钟）**、**HEARTBEAT.MD 正文** 与库表或 `heartbeat` 一致。  
- [ ] 与 **`3 agent-list.md`** 卡片展示字段可互推（同一 `AgentDTO` 子集）。  

---

*文档版本：基于 Agent 设置线稿整理；库表以 `前端.md` `agents` 为准；头像 **`50 Avatar.md`**，分类 **`4.agent-type.md`**，创建表单 **`2 agents-create.md`**。*
