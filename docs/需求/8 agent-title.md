# Agent 顶栏与「系统提示词」预览

本文档描述 **Agent 详情顶栏**（身份、标签、`agent_key`·`provider`/`model`）与 **「系统提示词」只读预览** 的交互；并与 **`5 agent-setting.md`**（系统提示模式 §5）、**`6 agent-setting-file.md`**（Markdown 分片字段）、运行态 **prompt 组装** 对齐。线稿参考：顶栏 chips、系统提示词对话框（Token 与 **完整 / 任务 / 最小化 / 无** 子 Tab）。

**通道与 Chat** 与 **`17 channel.md`** 中的 **`channel` 主表**、通道管理列表/新增流程对齐；高级设置 §8.2 仅描述 **Agent 侧绑定字段与级联 UI**，通道建模以 **`17 channel.md`** 为准。

---

## 1. 顶栏（摘要）

| 控件 | 行为 | 数据 |
|------|------|------|
| **返回** | 回列表 | — |
| **头像** | 点击换头像（见 **`50 Avatar.md`**） | `icon` |
| **显示名** | 主标题 | `display_name` |
| **收藏星标** | 已收藏高亮 | `user_agent_favorites` |
| **在线点** | 绿点等 | `status === active` 等 |
| **标签 chips** | 如 **完整**、**V3**、**进化中** | `other_config.system_prompt_mode` → 文案映射；`agent_type` 或产品版本；`self_evolve` 等 |
| **副标题** | `{agent_key} · {provider} / {model}` | 只读 |
| **系统提示词** | 眼形图标 + 文案；打开 **§2** 对话框 | 运行态渲染全文（非单文件落库） |
| **设置** | 齿轮 → **高级**模态（**§8**） | — |
| **删除** | 软删确认 | `deleted_at` |

**「进化中」**：当 `self_evolve === true` 且（可选）运行态判定正在演化时展示；与 **`7 agent-evolution.md`** 一致。

---

## 2.「系统提示词」对话框

居中或右侧 `QDrawer`；深色主题。

| 元素 | 说明 |
|------|------|
| **标题** | 如「系统提示词」 |
| **Token** | 展示当前预览的 **估算总 token**（如 `6,558 tokens`），与详情接口或独立 `GET .../system-prompt/preview` 一致 |
| **子 Tab** | **完整** \| **任务** \| **最小化** \| **无** — 与 **`5` §5** `system_prompt_mode` 四选一 **同一枚举**（`complete` / `task` / `minimized` / `none`），此处为 **只读切换预览**，若需改模式应在 **Agent Tab → 系统提示模式** PATCH |
| **正文区** | 等宽 / Markdown 渲染或纯文本；**只读**（编辑在「文件」Tab 或高级项） |

**产品说明**：预览内容 = 服务端按 **当前模式** 对若干源字段 + 动态块 **渲染后的全文**，与磁盘单文件不必 1:1。

---

## 3. 缓存边界（cache boundary）

运行时在完整提示中可插入标记（示例）：

```text
── cache boundary ── stable above · dynamic below
```

| 区段 | 含义 |
|------|------|
| **stable above** | 相对稳态：人格块、工具说明、AGENTS 分片、CAPABILITIES 等（仍可能随 PATCH 变，但同会话内多次请求可缓存复用） |
| **dynamic below** | 每轮或高频变化：**当前日期**、`<system_context name="TEAM.md">`、**Runtime** 行、会话任务片段等 |

UI 可在预览中 **弱化展示** 该分隔线，或仅调试模式显示。

---

## 4. 模式与注入块（runtime 契约）

下列与 **`5` §5** 绑定；以下为 **产品/实现约定**，便于前后端与 GoClaw 类运行时对齐。

| 模式 | `system_prompt_mode` | 典型包含（摘要） |
|------|----------------------|------------------|
| **完整** | `complete` | `IDENTITY.md` + `SOUL.md` + 完整 **Tooling** + Execution Bias + Tool Call Style + **Safety** + **Self-Evolution** + **Skills** + Workspace + Team + Memory Recall + **`AGENTS_CORE` + `AGENTS_TASK`** + `CAPABILITIES`（均经 `<internal_config>` 包裹，见 §5） |
| **任务** | `task` | 弱化人格与部分扩展说明；以 **`AGENTS_TASK`** + `CAPABILITIES` + 工具与工作区为主；适合自动化/企业任务 |
| **最小化** | `minimized` | **`AGENTS_CORE`** + `CAPABILITIES` + 精简 Safety + Workspace + Team 等；后台/观察 |
| **无** | `none` | **Tooling** + 极简 Safety + Workspace；几乎无人格与 AGENTS 长文 |

**Self-Evolution 文案**（完整模式中常见）：允许更新 **SOUL.md**、**CAPABILITIES.md** 的风格与领域表述；**禁止**改名称、身份、联系方式、核心目的、**IDENTITY.md**、**AGENTS\*** 等——与 **`6 agent-setting-file.md`**、**`7 agent-evolution.md`** 一致。

---

## 5. `<internal_config>` 与「文件」字段

运行时可将某列 Markdown 包在标签内注入，例如：

```xml
<internal_config name="IDENTITY.md">
…
</internal_config>
```

| `name` 属性 | 典型来源列（见 **`6` §8**） |
|-------------|----------------------------|
| `IDENTITY.md` | `identity_md` |
| `SOUL.md` | `soul_md` |
| `AGENTS_CORE.md` | `agents_core_md`（见 **`6`** AGENTS 拆分） |
| `AGENTS_TASK.md` | `agents_task_md` |
| `CAPABILITIES.md` | `capabilities_md` |

**预览对话框**可高亮块头 **或** 与「文件」Tab 联动跳转（选中同名逻辑文件）。

---

## 6. AGENTS 双文件与 `AGENTS.md`

部分运行时将操作规则拆为：

| 逻辑文件 | 职责摘要 |
|----------|----------|
| **AGENTS_CORE.md** | 通用：语言跟随、`[System Message]` 处理、保存须 `write_file`/`edit`、禁止用 exec 发消息等 |
| **AGENTS_TASK.md** | 任务向：memory 召回/写入路径、MEMORY.md 隐私、cron 使用约定等 |

存储上建议 **`agents_core_md` + `agents_task_md`** 两列；若产品仅提供单一 **`agents_md`**，可由服务端 **按标题拆段** 或 **任务模式只取一段**。详见 **`6 agent-setting-file.md` §8.3**。

---

## 7. 非「文件」Tab 的注入（运行时生成）

下列 **不必** 在「文件」侧栏以可编辑文件出现（或仅只读展示）：

| 块 | 说明 |
|----|------|
| **Tooling** | 来自己册 + `tools_config` 过滤后的工具列表 |
| **Workspace** | `workspace` 路径文案；团队共享路径来自租户/团队配置 |
| **`<system_context name="TEAM.md">`** | 团队与成员、委派规则；数据来自 **团队/成员 API**，非 Agent 表长文本 |
| **Current date** | 动态 |
| **Runtime** | 如 `Runtime: agent=… \| id=…` |

---

## 8.「高级」设置模态

居中 **`QDialog`** 或右侧 **`QDrawer`**；标题栏：**齿轮图标 +「高级」**；右上角 **关闭**。内容区 **`QScrollArea`** 纵向滚动，分段标题 + 卡片（与线稿：性能、压缩、上下文裁剪、沙箱、工作区等一致）。

### 8.1 供应商与模型（级联）

用于在 **不离开详情页** 的情况下调整 **LLM 路由**；数据源与 **`2 agents-create.md`** Provider/模型一致，并落 **`agents` 主表同一对字段**（与 **Agent Tab → 模型与预算** 同步，避免双源）。

| 控件 | Quasar | 行为 |
|------|--------|------|
| **供应商** | `QSelect`，`emit-value` `map-options`，可过滤 | 选项来自表 **`llm_provider_models`** 的 **去重 `provider_code`**（或 `GET /llm-providers` 聚合接口）：展示 `provider_display_name` |
| **模型** | `QSelect` | **级联**：仅在已选供应商后启用；选项为 **同 `provider_code` 下各行的 `model_api_id` / `model_display_name`**。切换供应商时 **清空模型** 并 `GET ...?provider_code=`（见 **`9 provider.md` §5 / §9**） |

| 数据关联（建议） | 说明 |
|------------------|------|
| **`llm_provider_models`** | **单表**：Provider 与 Model **不拆表**；每行含 `provider_code`、连接字段、`model_api_id`、分类、评级等；级联下拉 **唯一数据源**，详见 **`9 provider.md` §5** |

**持久化**：`agents.provider`、`agents.model`（**字符串**，与表中 **`provider_code` / `model_api_id`** 对齐，见 **`前端.md`**）。

**管理入口**：表单项旁可选 **「管理供应商与模型」** `QBtn` flat → 跳转 **`/settings/llm-providers`**，编辑 **`llm_provider_models`** 后回到本页 **重新拉取下拉选项**。

**校验**：可选复用 **`POST /agents/validate-model`**（`{ provider, model }`），与创建页「检查」一致。

---

### 8.2 通道与 Chat（级联，与 `channel` 关联）

用于绑定 **Agent 与消息入口**（某 IM / Webhook 通道上的具体会话或默认会话）。**Channel** 下拉选项 **必须** 与 **`17 channel.md`** 定义的 **`channel` 主表** 及 **`GET /channels`** 列表一致；**Chat** 为该通道下的外部会话标识（子资源），与 **`channel` 行**通过 `channel_id` 关联。

#### 与 `17 channel.md` 的对应关系

| 概念 | 来源 | 说明 |
|------|------|------|
| **Channel 记录** | 表 **`channel`**（见 **`17` §6.1**） | 一行 = 一个已配置的接入（飞书/Lark、微信、Telegram 等）；在 **通道管理** 中新增/编辑，**非** Agent 详情内嵌创建（Agent 仅「选择已有通道」） |
| **下拉展示字段** | `channel.name`、`channel.channel_type`（及微信时 `wechat_subtype`） | `QSelect` 选项 label 建议：`{name}` + 副文案 `「飞书」` / `「微信-公众号」` 等，与 **`17` §1.1.1** 列表列一致，便于运营识别 |
| **选项值（绑定键）** | 建议使用 **`channel.id`**（或 API 统一暴露 **`channel.uuid`**，二选一全项目一致） | Agent 持久化里存 **与 `channel` 主键一致** 的外键，禁止自造与 `channel` 表无关的字符串 |
| **Chat** | 关联表 **`channel_chats`**（或 `conversations` 等，见下） | 外部平台的 `chat_id` / `thread_id` + 展示名；**级联条件**：`channel_id = 当前选中的 channel` |

#### 控件与行为

| 控件 | Quasar | 行为 |
|------|--------|------|
| **Channel** | `QSelect` | `GET /channels`（同上）；`value` = `channel.id`（或 `uuid`）；选项展示：**左侧小图标**（`icon_url` 有则用其 URL，否则内置图标，见 **`17` §6.1 图标**）+ 名称 + 类型 |
| **Chat ID** | `QSelect`，可 `use-input` 允许粘贴 | **级联**：未选 Channel 时 **禁用**；`GET /channels/:channelId/chats`（或 `?channel_uuid=`）；选项为该平台下已同步/已绑定的会话列表 |

| 数据关联 | 说明 |
|----------|------|
| **`channel`** | 字段语义与枚举见 **`17` §6.1**（`channel_type`、`wechat_subtype`、`uuid`、`webhook_path` 等）；Agent **不重复存** AppSecret 等，只引用 `channel_id` |
| **`channel_chats`**（或等价名） | `channel_id`（FK → `channel.id`）+ 平台侧 `chat_id` + `title`/`name`；第二级下拉 **仅** `WHERE channel_id = :选中` |

#### 持久化（Agent 侧）

| 字段路径 | 类型 | 说明 |
|----------|------|------|
| `other_config.messaging` | object | 推荐：`{ "channel_id": <bigint 或 uuid 与 API 一致>, "chat_id": "<string>" }` |
| 或独立列 | | **`agents.channel_id`**（FK → `channel.id`）+ **`agents.default_chat_id`**（TEXT，平台会话 id）— 与 `other_config.messaging` **二选一**，避免双源 |

**约束**：`channel_id` 必须在 `channel` 表中存在；切换租户或删除通道时，后端应校验或级联提示（见 **`17`** 删除确认文案）。

**管理入口**：**「管理通道」** → **`/channels`**（与 **`17` §1** 列表、新增向导一致）；新建或编辑通道后回到本页 **重新 `GET /channels`** 与 **`GET .../chats`**。

**与供应商–模型（§8.1）关系**：二者独立；若产品需要「某类 `channel_type` 推荐某供应商」，可做 **可选** 下拉筛选或提示，非必须。

---

### 8.3 工作区

| 控件 | 字段 |
|------|------|
| **工作区路径** | `workspace`（创建时可自动分配 `~/.goclaw/workspace/{agent_key}`） |
| **说明** | 创建 Agent 时自动分配；运行时可为每用户创建子目录（产品文案） |

---

### 8.4 扩展思考（Reasoning）

与模型能力元数据相关；**仅当** 所选模型在 provider **models** 接口返回 **显式 reasoning 能力** 时展示高级控件（否则隐藏并提示，如线稿脚注）。

| 维度 | 建议存储 |
|------|----------|
| **策略** | `provider_default`（跟随厂商） / `custom`（本 Agent 覆盖） |
| **思考级别** | `off` \| `low` \| `medium` \| `high`（对应约 0 / ~4K / ~10–16K / ~32K **推理侧** token 预算，以后端换算为准） |

建议路径：`other_config.reasoning`：`{ "mode": "provider_default"|"custom", "level": "off"|"low"|"medium"|"high" }`。

---

### 8.5 其他区块（与 JSON 列对应）

线稿中的 **性能**、**压缩**、**上下文裁剪**、**沙箱**、**周期性启用** 等，建议映射如下（仅列产品级锚点，细项以 **`5 agent-setting.md`** / 后端 Schema 为准）：

| 线稿区块 | 建议列 / JSON |
|----------|----------------|
| 压缩（最大历史份额、保留最后消息数、压缩前记忆等） | `compaction_config` |
| 上下文裁剪（cache-ttl、软/硬裁剪等） | `context_pruning` |
| 沙箱（镜像、超时、资源限额等） | `sandbox_config` |
| 性能（上下文管理与执行环境） | `other_config.performance` 或与 `context_window`、`max_tool_iterations` 同区展示 |

**布局建议**：**§8.1、§8.2** 置于 **工作区（§8.3）之上**（先定模型路由与消息绑定，再路径与推理），或与线稿一致插在 **性能/压缩** 与 **工作区** 之间；以产品视觉稿为准。

---

## 9.「Agent Capabilities V3」说明弹窗（可选）

独立 **只读** 弹窗，四 Tab：**流水线**、**记忆**、**知识**、**编排**；用于解释 V3 智能体内核（与 **`6`/`7`** 及记忆配置呼应）。底部可注：**所有功能对 v3 智能体始终开启**（若产品如此声明）。

| Tab | 要点（产品文案级） |
|-----|---------------------|
| **流水线** | 初始化（配置、工作区、用户史、记忆）→ **迭代循环**（思考 → 裁剪 → 工具 → 观察 → 检查点）→ 完成（元数据、记忆事件、清理） |
| **记忆** | L0 工作记忆（压缩前 flush）→ L1 情景记忆（会话摘要、FTS+向量、TTL）→ L2 语义记忆（图谱、Dreaming） |
| **知识** | 知识图谱（`knowledge_graph_search`）、知识库（`[[wikilink]]`、混合检索比例）、Dreaming 防抖 |
| **编排** | `agent_links` 委派；自我进化周指标与护栏（与 **`7`** 护栏对象呼应） |

此弹窗 **不直接映射** 独立 `agents` 列，除非产品将说明固化进 `other_config.help_version` 等。

---

## 10. API 建议

| 方法 | 说明 |
|------|------|
| GET | `/agents/:id/system-prompt/preview?mode=complete\|task\|minimized\|none` | 返回渲染后全文 + `estimated_tokens` |
| GET | `/agents/:id` | 顶栏摘要 + 各 `*_md` 与 `other_config` |
| GET | `/llm-providers` 或 `/llm-provider-models` | 供应商/模型数据；底层 **`llm_provider_models`**（可按 `provider_code` 聚合，见 **`9 provider.md` §9**） |
| GET | `...?provider_code=` 或 `/llm-providers/:code/models` | 级联模型列表：筛选 **`provider_code`** 下各行 |
| POST | `/agents/validate-model` | 校验 `{ provider, model }`（与 **`2 agents-create.md`** 一致） |
| GET | `/channels` | Channel 下拉（§8.2）；数据模型见 **`17 channel.md` §6** |
| GET | `/channels/:channelId/chats` | 级联 Chat ID 列表；`channelId` 与主表 `id` 或 `uuid` 与 API 约定一致 |
| PATCH | `/agents/:id` | 更新 `provider`、`model`、`other_config.messaging` 等 |

---

## 11. 验收要点

- [ ] 顶栏 **完整 / 任务 / 最小化 / 无** 与详情 **`system_prompt_mode`** 一致；**进化中** 与 `self_evolve` 策略一致。  
- [ ] 系统提示词对话框 **Token** 与四子 Tab 预览与后端渲染一致。  
- [ ] `AGENTS_CORE` / `AGENTS_TASK` 在完整/任务/最小化模式下出现规则与 **`6` §8.3** 一致。  
- [ ] 高级中 **工作区**、**扩展思考** 与 `workspace`、`other_config.reasoning` 一致；无模型元数据时隐藏 reasoning 高级控件。  
- [ ] **供应商 → 模型** 级联：`agents.provider` / `agents.model` 与 **`llm_provider_models`** 可选集合一致；切换供应商清空并重拉模型。  
- [ ] **Channel → Chat ID** 级联：`other_config.messaging`（或独立列）中的 **`channel_id` 外键** 与 **`17 channel.md`** 表 **`channel`** 一致；`channel_chats` 与选中 Channel 一致；切换 Channel 清空并重拉 Chat 列表。  

---

*文档版本：与 `5 agent-setting.md`、`6 agent-setting-file.md`、`7 agent-evolution.md`、`2 agents-create.md`、**`17 channel.md`**（通道表与列表 API）对齐。*
