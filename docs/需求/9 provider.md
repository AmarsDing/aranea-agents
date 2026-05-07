# LLM Provider 管理

本文档合并 **产品/UI 规格**、**数据与 API 约定**、**验收要点** 以及 **功能需求**（原独立稿 `model.md` 已收入下文 §6～§8，避免双份维护）。**字段级 DDL** 以仓库内数据库迁移及专项表结构说明为准，本文不展开列级定义。

与 **`8 agent-title.md` §8.1**（供应商–模型级联）、**`2 agents-create.md`**（Provider/模型选择）、**`前端.md`** `agents.provider` 对齐。

**功能需求与 UI 的对应关系**：原 `model.md` §1～§3、§5（列表/连接/分类/统计趋势等）与本文 **§2～§5** 及 **§3.2 趋势看板** 一致，以本文控件与字段名为准；若有冲突以本文数据表 **§5** 与 **验收 §10** 为收敛点。

---

## 1. 页面定位

| 项目 | 说明 |
|------|------|
| **路由建议** | `/settings/llm-providers`（或项目统一设置前缀下） |
| **用户目标** | 维护可连接的 LLM 厂商；为每个 Provider 配置密钥、启用状态；为 **下属模型** 标注 **能力分类** 与可选性能指标，便于 Agent 选模与运营展示 |
| **入口** | Agent 高级设置 **「管理供应商与模型」**；侧边栏「模型厂商」等 |

---

## 2. 列表页布局

| 区域 | 说明 |
|------|------|
| **标题区** | 主标题 **Provider**；副标题 **管理 LLM Provider**；右上 **`+ 添加Provider`**（主色 `QBtn`） |
| **搜索** | `QInput` 带搜索图标；占位 **搜索Provider…**；对 `code`、`display_name`、标签等做前端过滤或 `GET ?q=` |
| **列表** | `QList`  卡片行；见 §3 |
| **分页** | 底部：总条数、`per_page`（如 20）、翻页 |

---

## 3. 列表行（一行对应 `llm_provider_models` 一条模型）

从左到右建议布局：

| 元素 | 说明 |
|------|------|
| **图标 + 名称** | 小图标（如芯片）+ **`provider_code` / `provider_display_name`** + **`model_display_name` 或 `model_api_id`** + **状态绿点**（连接正常/已配置等，规则由后端定） |
| **标签** | `QChip` 灰底，如厂商品牌或 **`provider_display_name`** |
| **模型类型** | 展示当前模型所选分类的 **`label`**（展示名），可用 **`QChip`**；**鼠标悬停**时 **`QTooltip`** 显示该类型的 **一句话说明**（与 `model_category` JSON 中对应项的 **`tooltip`** 一致；多选时 Tooltip 可并列展示多条或取首条） |
| **模型大小** | 只读展示 **`model_size_label`**（如 `7B` / `70B`）；空则 **「—」** 或隐藏 |
| **上下文** | 只读展示 **`context_window_k`**（如 `128K`）；帮助用户快速判断长文本能力 |
| **热度** | 展示 **`model_hotness_score`**（0～100），用 **进度条 + 等级文案** 表达近期使用活跃度；见 §3.1 |
| **近 30 天调用** | 展示 **`usage_call_count_30d`**，用于判断模型实际使用频率；暂无统计时展示 **「—」** |
| **近 30 天费用** | 展示 **`usage_cost_micro_usd_30d`** 格式化后的费用；暂无统计时展示 **「—」** |
| **TPS** | 只读展示 **`tokens_per_second`**（单位 `tokens/s`）；由后台根据实际模型输出计算后回写，**不在弹窗手工编辑**；空则 **「—」** |
| **成功率 / 延迟** | 可选展示 **`success_rate_30d`**、**`avg_latency_ms_30d`**；用于发现模型不稳定或响应慢 |
| **API 密钥** | 钥匙图标 + **已设置API密钥** / **未设置**（未设置可标橙或警告色） |
| **启用** | **`QToggle`**（**非**纯文案「已启用」）：开关 ON = 启用，OFF = 停用；变更即 **PATCH** `is_enabled`（可 debounce 或确认，由产品定） |
| **历史趋势** | **`QBtn` flat** `query_stats` 图标或文案 **趋势**；打开该模型的历史趋势看板（见 §3.2） |
| **编辑** | **`QBtn` flat** 文案 **编辑** 或 `edit` 图标；在 **删除** 左侧（紧挨删除之前） |
| **删除** | **`QBtn` flat** `delete` 图标；二次确认后 **DELETE** 或软删 |

**交互**：

- 点击 **编辑** → 打开与「添加」同构的 **编辑弹窗**（§4），`GET /llm-provider-models/:id`（或等价）预填当前行。
- 列表行内 **开关** 仅切换启用；不打开弹窗。
- 点击 **趋势** → 打开模型历史趋势看板；默认展示最近 30 天，可切换 7 天 / 30 天 / 本月 / 自定义。

### 3.1 模型热度显示

**热度定义**：`model_hotness_score`，范围 **0～100**，建议由统计服务根据近期调用次数、Token 消耗、费用占比、成功率等计算，不建议人工编辑。

建议计算口径：

```text
热度 = 近期调用次数标准分 * 0.45
     + Token 消耗标准分 * 0.25
     + 费用占比标准分 * 0.15
     + 成功率修正 * 0.10
     + 最近使用时间修正 * 0.05
```

展示方式：

| 热度分 | 文案 | 样式 |
|--------|------|------|
| 80～100 | 热门 | `QLinearProgress` 红/橙色，`QChip` 显示「热门」 |
| 50～79 | 活跃 | 蓝色，显示「活跃」 |
| 20～49 | 低频 | 灰蓝色，显示「低频」 |
| 0～19 | 冷门 | 灰色，显示「冷门」 |

列表建议采用 **进度条 + 分数 + 文案**：

```text
热门 86
[████████▌░]
```

Tooltip 展示热度来源：

- 近 30 天调用次数
- 近 30 天 Token
- 近 30 天费用
- 最近一次调用时间
- 成功率

### 3.2 历史趋势看板

点击列表行 **趋势** 按钮打开 `QDialog` / 右侧抽屉 / 独立页面。

建议展示：

| 模块 | 内容 |
|------|------|
| 顶部摘要 | 模型名、Provider、热度、30 天调用、30 天 Token、30 天费用 |
| 趋势图 | 调用次数趋势、Token 趋势、费用趋势、平均延迟趋势 |
| 占比 | 该模型在全部模型中的调用占比、Token 占比、费用占比 |
| 性能 | 平均 TPS、平均延迟、P95 延迟、成功率、失败次数 |
| 最近调用 | 最近 20 条调用记录：时间、Agent、Token、费用、状态、耗时 |

看板入口可先实现为弹窗占位，后续接入 `model_token_usage_events` / `model_token_usage_daily` 后展示真实图表。

### 3.3 列表字段建议分层

为避免列表过宽，建议按信息优先级展示：

| 层级 | 字段 | 说明 |
|------|------|------|
| 第一优先级 | 名称、Provider、模型类型、热度、启用、趋势、编辑/删除 | 日常管理必看 |
| 第二优先级 | 模型大小、上下文、最大输出 Token、TPS | 选模和性能判断 |
| 第三优先级 | 30 天调用、30 天 Token、30 天费用、成功率、平均延迟 | 使用情况和成本判断 |
| 展开/Tooltip | 最近调用时间、失败次数、费用占比、Token 占比 | 详情辅助信息 |
 
建议列表常显字段：

- 模型大小
- 上下文
- 热度
- 近 30 天调用
- 近 30 天费用
- TPS
- 成功率
- API 密钥

可放入 Tooltip 或趋势看板的字段：

- 近 30 天 Token
- 平均延迟 / P95 延迟
- 最近调用时间
- 失败次数
- 成本占比 / Token 占比

---

## 4. 添加 / 编辑 Provider 弹窗

`QDialog`；标题 **添加Provider** / **编辑Provider**；副标题 **配置 LLM Provider 连接**。

### 4.1 连接与身份（与线稿一致）

| 字段 | 控件 | 校验 / 说明 |
|------|------|-------------|
| **Provider类型** * | `QSelect` | 如 OpenAI Compatible、Anthropic、… |
| **模型ID** | `QInput`（线稿红框处） | 映射 **`llm_provider_models.model_api_id`**；与 §5 一致 |
| **名称** * | `QInput` | 占位「例如 openrouter」；**小写字母、数字、连字符**；映射 **`provider_code`**；同一厂商多模型时多行共用同一 `provider_code` |
| **显示名称** | `QInput` | 映射 **`provider_display_name`** |
| **模型展示名** | `QInput`（可选） | 映射 **`model_display_name`** |
| **API 基础 URL** | `QInput` | `https://…` |
| **API 密钥** | `QInput` `type=password` 或可切换明文；编辑时可 **留空表示不修改** |
| **已启用** | `QToggle` | 与列表开关同源；映射 **`is_enabled`**（本行模型是否启用） |

底部：**取消**、**创建** / **保存**；校验通过前主按钮可禁用。

### 4.2 模型分类（能力说明）

目的：让用户一眼明白 **该 Provider 下模型擅长什么**，用于列表/筛选/Agent 选模时的辅助文案。

**落点**：分类挂在 **`llm_provider_models` 每一行**（见 §5，单表含连接信息 + 模型信息）。

| 维度 | 说明 |
|------|------|
| **控件** | **`QSelect`** 单选为主（清晰）；若一模型多用途可 **`QSelect` multiple + chips** |
| **选项设计** | 预置枚举 + 可扩展；勾选结果写入 **`model_category`** JSON 数组，每项含 **`value`、`label`、`tooltip`**（与 §5 结构一致） |

**预置分类示例** ：

| value | 展示名 | 一句话说明（Tooltip） |
|-------|--------|-------------------------|
| `general` | 通用对话 | 均衡，适合日常问答与轻任务 |
| `reasoning` | 推理 / 复杂问题 | 数学、逻辑、多步推导 |
| `code` | 代码 | 生成、解释、重构代码 |
| `long_context` | 长上下文 | 大文档、长会话摘要 |
| `vision` | 视觉 / 多模态 | 图像理解 |
| `embedding` | 向量嵌入 | 记忆、检索 |
| `fast` | 低延迟 | 优先响应速度 |
| `creative` | 创意写作 | 文案、故事、营销 |

存库：见 §5 字段 **`model_category`**（JSON：**`value` | 展示名 | 一句话说明（Tooltip）** 的结构体数组）。

### 4.3 可选填：模型大小、上下文、最大输出、评级

用于 **展示与排序**，不参与鉴权；**可空**。

| 字段 | 控件 | 说明 | 建议列名 |
|------|------|------|----------|
| **模型大小** | `QInput` 或 `QSelect` | 参数量（如 **7B / 32B / 70B**）或 **约 70B** 自由文本；或数字 + 单位 `B` | `model_size_label` 或 `param_count_billions`（decimal，可选） |
| **上下文大小** | `QInput type=number` + suffix `K` | 如 128 表示 128K；用于聊天上下文裁剪与选模展示 | `context_window_k` |
| **最大输出 Token** | `QInput type=number` | 用于控制长回复上限，避免输出截断 | `max_output_tokens` |
| **模型评级** | `QSlider` / `QRating` / `QInput`（整数） | **用户主观评分**：**越高表示认为模型越强**；建议量程 **1～10**（或 1～5），与排序、筛选联动 | **`model_rating`**（int，可空） |
| **热度** | 只读展示 | 由后台统计计算，弹窗不编辑 | `model_hotness_score` |
| **TPS** | 只读展示 | 由后台按真实请求 `output_tokens / latency` 计算后回写，弹窗不编辑 | `tokens_per_second` |

**说明文案**：评级「用于团队内区分模型强弱偏好，不参与计费」；热度和 TPS 属于运行统计结果，不建议手工填写。

> 若 **添加 Provider** 弹窗只维护连接信息，**模型级** 字段可放在 **「模型矩阵」子页** 或 **展开行**：选中 Provider → 表格列出 `llm_provider_models`，每行有 **分类 / 模型大小 / TPS / 评级**。

---

## 5. 数据表建议（单表：`llm_provider_models`）

表名 **`llm_provider_models`**。  
**一行 = 一条「某厂商连接下的一个可选模型」**：连接字段（URL、密钥、类型等）在 **同一 `provider_code` 的多行之间重复存储**；业务上仍把相同 `provider_code` 的多行视为 **同一 Provider**。

| 字段名 | 类型 / 说明 |
|--------|-------------|
| `id` | PK，UUID 或雪花 |
| `provider_code` | **小写 slug** |
| `provider_display_name` | Provider 展示名 |
| `provider_type` | 枚举：OpenAI Compatible （厂商名，罗列支持的厂商名）等 |
| `api_base_url` | 基础 URL（同一 `provider_code` 各模型行应 **保持一致**） |
| `api_key_encrypted` | 密钥密文 |
| `model_api_id` | **字符串**；该 API 中的模型 id（与厂商文档一致） |
| `model_display_name` | 可选；本模型在 UI 上的展示名 |
| `model_category` | **JSON**（`JSONB` / `TEXT` 序列化）：**对象数组**，每项为同一结构体 **`{ "value": string, "label": string, "tooltip": string }`** —— 依次为 **枚举键**、**展示名**、**一句话说明**（供列表悬停与 `QTooltip` 使用）。单选时数组长度为 1，多选时为多条。示例见下。 |
| `model_size_label` | 可选；如 `7B` / `70B` |
| `context_window_k` | 可选；上下文大小，单位 K |
| `max_output_tokens` | 可选；最大输出 Token |
| `tokens_per_second` | 可选 float；后台根据实际调用计算的 TPS |
| `model_hotness_score` | 可选 INT（0～100）；后台统计计算的热度分 |
| `usage_call_count_30d` | 可选 INT；最近 30 天调用次数 |
| `usage_total_tokens_30d` | 可选 INT；最近 30 天总 Token |
| `usage_cost_micro_usd_30d` | 可选 INT；最近 30 天费用 |
| `success_rate_30d` | 可选 float；最近 30 天成功率 |
| `avg_latency_ms_30d` | 可选 float；最近 30 天平均延迟 |
| `last_used_at` | 可选时间；最近一次调用时间 |
| **`model_rating`** | **INT**（**1～100**）：用户可编辑，**数值越大表示主观认为模型越强**；默认评级60 |
| `sort_order` | 同一 `provider_code` 内的展示顺序 |
| `is_enabled` | 是否启用 |
| `created_at` / `updated_at` | 时间戳 |

**`model_category` 存表示例**（与 §4.2 预置项对应时可只存选中项）：

```json
[
  { "value": "reasoning", "label": "推理 / 复杂问题", "tooltip": "数学、逻辑、多步推导" },
  { "value": "code", "label": "代码", "tooltip": "生成、解释、重构代码" }
]
```

**唯一约束**：**`(provider_code, model_api_id)`** 联合唯一。

**实现注意**：修改 **API 地址/密钥** 时，应对 **`WHERE provider_code = ?`** 批量更新，避免同厂商各行连接信息漂移。列表 UI 可按 **`provider_code` 分组** 展示（折叠/首行），底层仍为多行。

**UI**：`model_rating` 可用 **`QSlider`**、`QRating`、`QInput`；保存 **PATCH** 单行 `id` 或 `(provider_code, model_api_id)`。

**统计字段来源**：`model_hotness_score`、`usage_*_30d`、`success_rate_30d`、`avg_latency_ms_30d` 建议由 `model_token_usage_events` / `model_token_usage_daily` 周期聚合后回写到 `llm_provider_models.config_json`，列表页读取该快照，趋势看板读取明细/聚合表。

分类枚举可维护 **`model_categories`** 字典表，或后端常量下发 `GET /meta/model-categories`。

---

## 6. 计价与成本（功能需求）

1. **价格配置**：支持维护输入 / 输出 / 缓存输入 / 推理 / 嵌入等单价（微美元每千 Token 等**具体口径与落库字段以后端计价模块为准**）；保存后与计费规则同步，供调用扣费使用。
2. **调用成本归因**：每次模型调用应能记录 Token、费用、延迟、成功失败等，并支持按 **模型 / Agent / 时间** 维度汇总（支撑列表指标与趋势）。
3. UI 侧在 **§3** 展示的 30 天费用、TPS、成功率等应与上述事件流或聚合表一致；无数据时占位不破坏布局（与上文列表约定一致）。

---

## 7. Embedding 与调用类型（功能需求）

1. **用途区分**：调用类型应能区分 **聊天 / 嵌入** 等（如 `usage_kind` 或等价字段），以便记忆检索侧 Embedding 配置、成本统计与看板分项一致。
2. **配置协同**：Embedding 相关模型选择与 **本文弹窗 §4 / 列表 §3** 及 Agent 运行时记忆配置一致；运行时开关见记忆与 Agent 相关需求文档。
3. **自检回填**：对在「连接与鉴权」中触发的自检 / 探测，可将模型元数据、价格建议等写回展示字段（与 **§9** 中 inspect 类接口对齐）。

---

## 8. 非功能与安全（功能需求）

1. **密钥安全**：生产环境应对 `api_key` **加密存储**或接入密钥管理服务；日志与排障信息**禁止**明文打印密钥。
2. **软删与审计**：删除宜支持软删或保留审计痕迹，避免关联引用瞬间断裂（与平台 `deleted_at` 等约定一致）。
3. **多租户（若适用）**：工作区 / 用户维度的统计与隔离与用量表中的 `workspace_id`、`user_id` 等字段对齐。

---

## 9. API 建议

资源对应表 **`llm_provider_models`**；路径可保留 **面向产品的** `/llm-providers` 前缀，由后端映射到行级 `id` 或 `(provider_code, model_api_id)`。与旧栈并存时，亦可映射既有 **`/api/v1/llm-provider-models`** 族，但**新开发以 Kratos 契约为准**。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/llm-provider-models` 或 `/llm-providers` | 扁平列表；或 **按 `provider_code` 聚合** 返回树，便于 UI |
| GET | `/llm-provider-models/:id` | 单行详情 |
| POST | `/llm-provider-models` | 插入一行（新 provider 首行或同 `provider_code` 增模型） |
| PATCH | `/llm-provider-models/:id` | 更新单行；若改 `api_base_url`/`api_key`，服务端可 **级联** 同 `provider_code` 各行 |
| PATCH | `/llm-providers/:code/connection`（可选） | 仅更新连接字段并 **批量** 作用于该 `provider_code` |
| DELETE | `/llm-provider-models/:id` | 删除单行；若删除后某 `provider_code` 无行，则该 Provider 消失 |
| DELETE | `/llm-providers/:code`（可选） | 删除该 `provider_code` 下 **所有** 行 |
| GET | `/llm-provider-models/:id/usage-trends`（建议） | 返回该模型历史趋势：调用、Token、费用、成功率、延迟等 |
| POST | `/llm-provider-models/:id/inspect`（或独立 `.../inspect`） | **连接 / 模型自检**：存在性、元数据探测；结果可回填展示名、上下文、价格建议等（与 **§7** 一致） |
| GET | `/meta/model-categories`（可选） | 分类枚举下发，供下拉与 Tooltip 与 **§4.2** 一致 |

**CLI / 技能**：支持管理员通过 **CLI** 或 **受控内置技能** 列出、增删改 Provider 模型（权限与审计与平台工具策略一致）。

---

## 10. 验收要点

- [ ] 列表行 **删除** 前存在 **编辑** 按钮，编辑弹窗数据正确回显。  
- [ ] **已启用** 为 **`QToggle`**，与详情/弹窗内 **已启用** 状态一致。  
- [ ] **模型分类** 落库为 **`{ value, label, tooltip }` 数组**；列表 **模型类型** 悬停显示 **tooltip**。  
- [ ] **模型大小**、**上下文**、**最大输出**、**TPS**、**模型评级** 行为符合 §4.3 / §5；**评级越高表示越强** 的排序与筛选可用。  
- [ ] **热度** 为只读统计字段，列表以进度条 + 等级文案展示，Tooltip 可解释热度来源。  
- [ ] 列表存在 **历史趋势** 按钮，可打开模型趋势看板入口。  
- [ ] 近 30 天调用、费用、成功率、延迟等字段缺失时显示 **「—」**，不影响列表布局。  
- [ ] 仅 **`llm_provider_models`** 一表：**`(provider_code, model_api_id)`** 唯一；改连接字段时 **同 `provider_code` 批量一致**。  
- [ ] **Embedding** 与 **调用类型**（聊天 / 嵌入等）在统计与配置上可区分，与记忆链路一致（**§7**）。  
- [ ] **计价** 与调用事件、日聚合能对齐列表/趋势中的费用与 Token（**§6**）。  
- [ ] **密钥**、**软删/审计**、多租户隔离符合 **§8**。  
- [ ] **自检（inspect）** 与 **元数据枚举** 接口（若提供）与弹窗/列表展示一致（**§9**）。  

---

*文档版本：与 `8 agent-title.md` §8.1、`2 agents-create.md` 对齐；已合并原 `model.md` 功能需求。*
