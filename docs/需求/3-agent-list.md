# Agent 列表页 — 产品设计说明（Quasar）

本文档描述「Agent 管理」主列表界面（标题 **Agent**、副标题 **AI Agent 管理 **），涵盖布局、控件、交互、筛选分页与 **`agents` 主表**字段对应关系。后端 Agent 运行时基于 trpc-agent-go 框架。创建流程见 `2 agents-create.md`。

---

## 1. 页面定位

| 项目 | 说明 |
|------|------|
| **路由建议** | `/agents` |
| **用户目标** | 浏览、搜索、筛选自己可见的 Agent；创建、迁移、收藏、删除；切换网格/列表视图 |
| **视觉** | 白昼模式 |
| **数据主实体** | `agents` 表（未软删 `deleted_at IS NULL` 的行） |

---

## 2. 整体布局

```
┌──────────────────────────────────────────────────────────────────────┐
│  Agent                                          [Agent迁移] [+ 创建Agent] │
│  管理您的 AI Agent                                                     │
├──────────────────────────────────────────────────────────────────────┤
│  [🔍 搜索Agent...]  [All Types ▼]  [业务分类 Agent Type ▼]  [创建者 ▼]  [网格|列表] │
├──────────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ...                                │
│  │ [头像] ★名称 │  │ ...         │                                      │
│  │    handle   │  │             │                                      │
│  │ [active]    │  │             │                                      │
│  │ provider/model│ │             │                                      │
│  │ 描述摘要... │  │             │                                      │
│  │ [标签][进化] │  │             │                                      │
│  │ 200K ctx 删除│  │             │                                      │
│  └─────────────┘  └─────────────┘                                      │
├──────────────────────────────────────────────────────────────────────┤
│  2 条                    [行 20 ▼]  第 1/1 页  [←] [→]                 │
└──────────────────────────────────────────────────────────────────────┘
```

| 区域 | Quasar 建议 |
|------|-------------|
| 页面根 | `QPage` + `q-pa-md`，纵向 `column` 或 `q-layout` 内容区 |
| 页头 | `div.row.items-center.justify-between`：左侧标题块，右侧 `QBtn` 组 |
| 筛选行 | `div.row.q-col-gutter-sm.items-center`：`QInput`、`QSelect`（技术类型 + **业务分类** + 创建者）、右侧视图切换；窄屏可 `wrap` 换行 |
| 内容区 | **网格**：`div.row.q-col-gutter-md` + `div.col-12 col-sm-6 col-md-4` 包裹 `QCard`；**列表**：`QTable` 或 `QList` + `QItem` |
| 页脚 | `div.row.items-center.justify-between`：总数、`QSelect` 每页条数、文案「第 x/y 页」、`QBtn` icon 翻页 |

---

## 3. 控件与行为（逐项）

### 3.1 页头 — 标题与副标题

| 维度 | 说明 |
|------|------|
| **文案** | 主标题：`Agent`；副标题：`管理您的 AI Agent` |
| **样式** | 主标题大字重、高对比；副标题次级灰字 |
| **行为** | 无交互；可选点击标题刷新列表（非必须） |

### 3.2 页头 —「Agent迁移」

| 维度 | 说明 |
|------|------|
| **控件** | `QBtn`：`outline` 或 `flat`，带交换/迁移类 `QIcon` |
| **行为** | 点击打开「迁移」流程：新页面（导入/导出、批量映射、冲突处理）；**不在本文展开**，需单独 PRD |

### 3.3 页头 —「+ 创建Agent」

| 维度 | 说明 |
|------|------|
| **控件** | `QBtn`：`color` 为主色（橙），`unelevated` 或带轻微阴影 |
| **行为** | 打开创建弹窗（见 `2 agents-create.md`）；创建成功后 **关闭弹窗并刷新当前列表**（保持筛选/页码策略见 §6） |

### 3.4 搜索框

| 维度 | 说明 |
|------|------|
| **控件** | `QInput`：`outlined` 或 `standout`，圆角；`prepend` 搜索图标 |
| **占位** | `搜索Agent...` |
| **绑定** | 查询参数 `q`（或 `search`） |
| **行为** | 输入防抖（建议 300–500ms）后请求列表接口；清空则恢复无关键字查询 |
| **后端** | 建议命中 `display_name`、`agent_key`、`frontmatter`、`agent_description`（与 `tsv` / 全文检索设计一致时走 GIN；否则 ILIKE 多列） |

### 3.5 类型筛选 —「All Types」

| 维度 | 说明 |
|------|------|
| **控件** | `QSelect`：`outlined`，圆角，可清空 |
| **选项** | 全部分类 + 业务类型（与 `agents.agent_type` 或产品定义的枚举一致，如 `open` 等） |
| **行为** | 变更即重置到第 1 页并请求列表 |
| **库表** | `agents.agent_type` |

### 3.6 业务分类筛选 —「Agent Type / 业务分类」

与 `4.agent-type.md` 一致：**行业 → 部门 → 职位** 体系；筛选的是 Agent 绑定的 **职位叶子**（`agents.category_position_id`），**不是**上一节的 `agents.agent_type`。

| 维度 | 说明 |
|------|------|
| **控件** | **方案 A（推荐 MVP）**：单个 `QSelect`，可清空；选项标签为完整路径（如 `IT行业 / 游戏开发部 / UE5场景设计师`），值为 `category_position_id`。 |
| **占位** | `业务分类` 或 `全部业务分类` |
| **数据源** | `GET /agent-categories/positions?workspace_id=` 返回 `{ id, path_label }` 列表，前端由 `tree` 展平叶子节点。 |
| **绑定** | 查询参数 `category_position_id`；清空表示不按分类过滤。 |
| **行为** | 变更即 **page 重置为 1** 并请求列表；可与关键字搜索、`agent_type`、创建者组合。 |
| **可选增强** | 增加按 **行业** 或 **部门** 聚合筛选（参数如 `category_industry_id` / `category_department_id`，服务端匹配子树）。 |
| **库表** | `agents.category_position_id`（及可选 `category_path` 展示） |

### 3.7 创建者筛选 —「所有创建者」

| 维度 | 说明 |
|------|------|
| **控件** | `QSelect`：占位「所有创建者」 |
| **选项** | 当前租户/空间下出现过创建者的用户列表（需后端 `created_by` 或审计字段支持） |
| **行为** | 变更即第 1 页刷新 |
| **库表** | ✅ 已实现 `agents.created_by`（TEXT，auth `UserID` 字符串）；筛选参数 `created_by` 支持空 / `mine` / 具体用户 id；见 [3-agent-list-development.md](./3-agent-list-development.md) |

### 3.8 视图切换 — 网格 / 列表

| 维度 | 说明 |
|------|------|
| **控件** | 两个 `QBtn` `flat`/`toggle` 或 `QBtnGroup`，当前模式高亮（橙色） |
| **状态** | `viewMode: 'grid' | 'list'`，持久化到 `localStorage`（键如 `agents.viewMode`） |
| **行为** | 切换仅影响展示，**不改变**当前查询条件与分页 |

### 3.9 Agent 卡片（网格模式）

单张卡片信息结构与截图一致，建议 `QCard` + `QCardSection`（或 section 分块）。

| UI 区块 | 说明 | 行为 |
|---------|------|------|
| **左上角头像** | `QAvatar` 方形圆角；`src` 为 **`/avatar-assets/{agents.icon}/thumbnail`**（或 `/file`），`agents.icon` 存 **`avatar_assets.id`**，见 **`50 Avatar.md`** | 可选点击进入详情/编辑 |
| **名称行** | 粗体：`display_name` | — |
| **收藏星标** | `QIcon`/`QBtn` 金色星 | 点击切换收藏；见 §5「收藏」 |
| **副标题 handle** | 小字灰色：`agent_key` | 可选点击复制 |
| **右上角状态** | `QBadge` 胶囊：如 `active` 绿色 | 颜色与 `status` 映射（`active`/`inactive`/…） |
| **模型行** | 文案：`{provider} / {model}` | 只读展示 |
| **描述** | 多行截断，省略号 | 来源见 §4；悬停可选 `QTooltip` 全文 |
| **底部标签** | `QChip` 多枚 | 来源见 §4「标签」 |
| **进化状态** | 带闪光图标的按钮/徽章：`进化中` | 当 `self_evolve === true` 且服务端判定「正在进化」时显示；纯关闭进化可不展示 |
| **上下文** | 文案如 `200K ctx` | 由 `context_window` 格式化为 K/M |
| **删除** | `QBtn` `flat` + 垃圾桶图标，文案「删除」 | 点击 `QDialog` 确认 → 软删或硬删 API |

**列表模式**：用表格列对齐上述字段（名称+星标、handle、状态、模型、描述摘要、标签、ctx、操作列）。

### 3.10 底部分页栏

| 维度 | 说明 |
|------|------|
| **总条数** | 文案：`{total} 条`，取自接口 `total` |
| **每页行数** | `QSelect`：选项如 10、20、50；标签「行」与截图一致 |
| **页码** | 文案：`第 {page} / {pages} 页` |
| **翻页** | 上一页/下一页 `QBtn` `round` icon；首页末页可选 |
| **行为** | 变更 `page` / `pageSize` 触发列表请求；筛选或搜索变更时 **page 重置为 1** |

---

## 4. 列表项展示字段与 `agents` 表映射

| 界面展示 | 数据库列 / 来源 | 说明 |
|----------|-----------------|------|
| 名称 | `display_name` | 必填展示 |
| handle | `agent_key` | 唯一业务键展示 |
| 头像 | `icon` | `avatar_assets.id`；`src` 指向只读出图接口（见 `50 Avatar.md`） |
| 状态徽章 | `status` | 如 `active` |
| 模型行 | `provider` + `model` | 拼接为 `provider / model` |
| 业务分类（可选） | `category_path` 或由 `category_position_id` 解析 | 副文案或 `QChip`，与筛选「Agent Type」同源 |
| 卡片描述摘要 | 优先 `frontmatter`（短）；若空则用 `agent_description` 截断 | 与创建文档分工一致 |
| 标签 chips | `other_config.tags` 或独立 `tags` JSONB / 关联表 | 产品枚举如「任务」「完整」需与后端约定 |
| 进化中 | `self_evolve` + 运行态（可选 `other_config.evolution_status`） | 「进化中」可为异步任务状态 |
| 200K ctx | `context_window` | UI 格式化为 `200K ctx` |
| 收藏星标 | 见下节 | 未必在 `agents` 单行 |

---

## 5. 收藏（星标）

截图中名称旁有星标，通常为 **用户维度** 偏好，不建议仅存在 `agents` 行内（多用户共享同一 Agent 时语义冲突）。

| 方案 | 说明 |
|------|------|
| **推荐** | 表 `user_agent_favorites(user_id, agent_id, created_at)`，唯一 `(user_id, agent_id)` |
| **简化** | MVP 仅前端 `localStorage` 存 id 列表（无跨设备同步） |

列表接口可返回 `is_favorite: boolean`（当前用户），或前端根据收藏表/本地集合合并。

---

## 6. 列表 API 建议（查询参数）

| 参数 | 说明 |
|------|------|
| `q` | 搜索关键字 |
| `agent_type` | **技术**类型筛选（`agents.agent_type`），空表示全部 |
| `category_position_id` | **业务分类**（Agent Type）：职位叶子 id，与 `4.agent-type.md` 一致；空表示全部 |
| `created_by` | 创建者 id，空表示全部 |
| `page` | 从 1 开始 |
| `page_size` | 每页条数 |
| `sort` | 可选：`updated_at desc` 默认 |

**响应**建议：`{ items: AgentDTO[], total: number, page, page_size }`。`AgentDTO` 含列表所需列 + `is_favorite`。

**删除**：`DELETE /agents/:id` 或 `PATCH` 软删（与 `deleted_at` 一致）。

---

## 7. 空状态与异常

| 场景 | 处理 |
|------|------|
| 无数据 | 插图 + 文案「暂无 Agent」+ 引导「创建 Agent」 |
| 搜索无结果 | 「未找到匹配的 Agent」+ 清除搜索 |
| 加载中 | 卡片骨架屏 `QSkeleton` 或 `QInnerLoading` |
| 接口失败 | `Notify` 错误信息，保留上次成功数据或清空由产品定 |

---

## 8. 与 `2 agents-create.md` 的衔接

| 事件 | 列表页行为 |
|------|------------|
| 创建成功 | 刷新列表；新行通常出现在当前排序下（如按更新时间） |
| `agent_key` 在列表展示 | 与创建表单校验一致，列表只读 |
| `context_window` | 创建页可默认值，列表仅展示 |

---

## 9. 验收要点（产品）

- [ ] 标题、副标题、主色与截图信息架构一致  
- [ ] 搜索防抖 + **技术类型 + 业务分类（Agent Type）+ 创建者** 筛选与分页联动正确  
- [ ] 网格与列表切换可记忆  
- [ ] 卡片字段与 `agents` 映射正确；描述截断与 Tooltip（若做）合理  
- [ ] 删除二次确认；软删后列表不再出现  
- [ ] 收藏状态切换有反馈且与后端或本地策略一致  
- [ ] 「Agent迁移」入口存在；具体流程可后续文档化  

---

*文档版本：与 `agents` 主表、`2 agents-create.md`、`4.agent-type.md` 对齐；界面以当前 Agent 管理列表线稿为准。*
