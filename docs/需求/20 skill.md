# Skill 管理（Quasar UI + 前后端契约）

本文档定义 **Skill 管理** 与 **Skill 运行记录** 的前端可开发需求。后台机制与判定规则对齐 **`go/skill.md`**：录入、验证、冲突分级、版本化、追踪与聚合。

## 0. 需求结论

### 0.1 本期范围

| 模块 | 本期是否做 | 说明 |
|------|------------|------|
| Skill 列表 | 是 | 支持搜索、标签筛选、启用筛选、服务端分页、启用/停用、复制、删除 |
| 上传 Skill zip | 是 | 单文件上传、存放到指定 Skill 目录、严格校验 Skill 编写规范、展示逐条检查结果 |
| 冲突处理 | 是 | 先程序检查名称重复，再通过已配置模型检查简介与正文相似度；相似度达到 20% 建议炼化 |
| AI 炼化 | 是，上传后按冲突组启用 | 不提供单独炼化入口；仅当上传检查产生相似冲突组时，在组内显示炼化按钮 |
| 编辑 Skill | 是 | 编辑元数据与正文，支持保存草稿和发布 |
| 运行记录 | 是 | 按 Skill、Agent、结果、时间范围筛选，服务端分页 |
| 版本历史 / 回滚 | 后续迭代 | 本期只展示当前版本号 |
| 自动负熵报告 | 后续迭代 | 本期只展示已有聚合指标 |

### 0.2 默认产品决策

以下决策用于推进实现，若产品或后端有不同约定，需在 §8 更新：

| 决策项 | 默认值 |
|--------|--------|
| 路由 | `GET /skills` 页面为 Skill 管理；`GET /skills/runs` 页面为运行记录 |
| 作用域 | 默认按当前工作区 / 租户隔离；前端不手动传 `tenant_id`，由后端根据登录态解析 |
| 唯一性 | 同一租户下 `name` 唯一；`slug` 若存在也唯一 |
| 分页 | 所有表格使用服务端分页；页码从 `1` 开始 |
| 删除 | 软删；删除后列表不显示 |
| 启用开关 | 仅 `published` Skill 可启用；`draft` 和 `archived` 禁用开关 |
| `warn` 发布 | 允许具备编辑权限的用户确认后继续入库或发布 |
| `block` 发布 | 不允许前端强制通过 |
| 输入输出详情 | 默认仅管理员可看完整详情；普通用户只看 preview / hash |
| 上传存储 | 上传包解析通过后由后端存放到配置的 Skill 根目录；前端不直接写文件路径 |
| 相似阈值 | 模型判定简介 + 正文相似度 `>= 0.2` 时标记为建议炼化 |

### 0.3 角色与权限

| 能力 | 管理员 | 编辑者 | 只读用户 |
|------|--------|--------|----------|
| 查看 Skill 列表 | 是 | 是 | 是 |
| 查看运行记录 | 是 | 是 | 是 |
| 查看完整输入 / 输出详情 | 是 | 否，除非后端授权 | 否 |
| 上传 Skill | 是 | 是 | 否 |
| 编辑 / 发布 Skill | 是 | 是 | 否 |
| 启用 / 停用 Skill | 是 | 是 | 否 |
| 删除 Skill | 是 | 否 | 否 |
| 确认 `warn` 冲突 | 是 | 是 | 否 |
| 处理 `block` 冲突 | 不能强制通过，只能修正后重试 | 不能强制通过 | 否 |

前端根据后端返回的 `permissions` 控制按钮可见性或禁用态，不在本地硬编码角色名。

---

## 1. 路由与信息架构

| 路由 | 页面 | 说明 |
|------|------|------|
| `/skills` | Skill 管理 | 列表、上传、编辑、冲突处理、AI 炼化 |
| `/skills/runs` | Skill 运行记录 | 按 Skill / Agent / 结果 / 时间筛选调用明细 |

侧栏与 **`18 monitor.md`** 保持一致，使用父级 `QDrawer` + `QList` / `QItem`。路由入口展示在管理类导航下，文案分别为「Skill 管理」和「运行记录」。

---

## 2. Skill 管理页

### 2.1 页面结构

| 区域 | 需求 |
|------|------|
| 标题 | 「Skill 管理」 |
| 副标题 | 展示当前工作区 / 租户名；无数据时隐藏 |
| 右上操作 | 「上传 Skill」「刷新」 |
| 工具栏 | 搜索框、标签多选、仅看启用、重置筛选 |
| 主表 | `QTable`，服务端分页，使用 `@request` 拉取数据 |
| 底栏 | 每页条数、总条数、页码切换 |

搜索行为：

- 搜索字段匹配 `name`、`slug`、`description`。
- 输入框使用 `debounce="300"`。
- 搜索、筛选、每页条数变化后重置到第 1 页。

筛选行为：

| 筛选项 | 参数 | 说明 |
|--------|------|------|
| 搜索 | `search` | 空值不传 |
| 标签 | `tags` | 多选，以逗号或数组传输，最终格式见后端契约 |
| 启用状态 | `enabled` | `true` / `false` / 空 |
| 状态 | `status` | 默认不展示；需要时支持 `draft` / `published` / `archived` |

### 2.2 表格列

| 列 | 字段 | UI 与交互 |
|----|------|-----------|
| 名称 | `name`、`slug` | 主行展示 `name`；副行展示 `slug`，灰色小字 |
| 描述 | `description` | 单行截断，悬浮 `QTooltip` 展示全文 |
| 标签 | `tags[]` | `QChip` 密集排列；`system` 用 outline，`user` 用主色 |
| 状态 | `status` | `draft` 草稿、`published` 已发布、`archived` 已归档 |
| 版本 | `current_version.version` | 展示如 `v3` / `1.0.0`；无版本展示 `-` |
| 启用 | `enabled` | `QToggle dense`；无权限、非 `published`、请求中时禁用 |
| 使用频率 | `invoke_count` | 展示 Skill 被使用总次数；可扩展展示近 7 日调用次数 |
| 成功 / 失败 | `success_count`、`failure_count` | 展示成功次数、失败次数，可计算成功率 |
| 耗时 | `avg_duration_ms`、`last_duration_ms` | 展示平均耗时；悬浮或副文案展示最近一次耗时 |
| 最近调用 | `last_agent_display_name`、`last_invoked_at` | 展示最近一次调用的 Agent 与时间；无值展示「未调用」 |
| 操作 | `permissions` | 编辑、复制、删除 |

启用 / 停用行为：

1. 用户切换 `QToggle`。
2. 前端乐观更新为请求中禁用态，不立即改变最终状态。
3. 调用 `PATCH /skills/:id/enabled`。
4. 成功后更新该行；失败后恢复原状态并 `Notify` negative。

删除行为：

1. 点击「删除」打开确认 `QDialog`。
2. 确认文案必须包含 Skill 名称。
3. 调用 `DELETE /skills/:id`。
4. 成功后刷新当前页；若当前页为空且页码大于 1，则回到上一页。

复制行为：

1. 点击「复制」调用 `POST /skills/:id/duplicate`。
2. 成功后跳转到新 Skill 编辑页，或在当前列表高亮新草稿。
3. 默认复制结果为 `draft` 且 `enabled=false`。

### 2.3 状态展示

| 状态 | UI |
|------|----|
| 首次加载 | `QInnerLoading` 或表格 loading |
| 空列表 | `QBanner` + 主按钮「上传 Skill」 |
| 搜索无结果 | 文案「没有匹配的 Skill」+「重置筛选」 |
| 请求失败 | `QBanner` negative +「重试」 |
| 无权限操作 | 按钮禁用，`QTooltip` 展示原因 |

---

## 3. 上传与导入流程

### 3.1 上传入口

| 区域 | 组件 / 行为 |
|------|-------------|
| 入口 | 点击「上传 Skill」打开 `QDialog` |
| 文件选择 | `QUploader` 或自定义拖拽卡片；只允许 `.zip`；`multiple=false` |
| 文件限制 | 前端校验扩展名；文件大小上限由后端返回或配置 |
| 提交 | 选择文件后点击「开始上传」 |
| 进度 | 上传中展示进度；导入处理中展示 `QLinearProgress indeterminate` |

上传存储与格式要求：

- 上传文件由后端接收、解压、校验后存放到配置的 Skill 根目录，例如 `SKILL_ROOT` 或系统配置项 `skill_storage_root`。
- 前端只上传 `.zip` 文件，不允许用户直接指定服务器目录，避免路径穿越和误写。
- zip 内每个 Skill 必须严格符合 Skill 编写规范：必须包含 `SKILL.md`，文件名大小写按规范处理，正文结构、frontmatter / 元数据、名称、简介、触发说明等由后端校验。
- 后端保存时使用规范化目录名，默认由 `slug` 或规范化后的 `name` 生成；同名目录冲突时必须阻塞或由冲突处理流程生成合并结果。
- 原始上传包可进入临时区，只有通过结构校验和冲突处理的 Skill 才能进入正式 Skill 目录。

### 3.1.1 上传检查顺序

上传后，后端必须按以下顺序检查：

1. **结构校验**：解压 zip，检查是否符合 Skill 编写规范；不符合则 `block`。
2. **名称检查**：程序先对比已存在 Skill 的 `name` 和 `slug`；同作用域完全相同则 `block`，不进入模型相似度检查。
3. **相似度检查**：对名称不重复的候选 Skill，调用已配置模型，对候选的 `description + body` 与已有 Skill 的 `description + body` 做语义相似度判断。
4. **冲突分组**：模型返回相似度 `similarity_score >= 0.2` 时，标记为建议炼化，并把候选 Skill 与所有相似 Skill 放入同一个冲突组。
5. **结果展示**：无冲突候选显示对号；存在冲突的候选按冲突组展示，并只在组内提供「炼化」按钮。

相似度检查要求：

- 使用系统中已配置且可用的模型；若无可用模型，上传检查不能直接判定相似，需展示「模型不可用，无法完成相似度检查」。
- 模型输入必须包含候选 Skill 与已有 Skill 的名称、简介和正文摘要 / 正文；后端负责截断到模型上下文允许范围。
- 模型输出必须结构化，至少包含 `similarity_score`、分项指标、`reason`、`evidence`、`recommendation`、`confidence`。
- `similarity_score` 取值范围为 `0` 到 `1`；`0.2` 表示 20% 相似度阈值。

大模型需返回的相似度指标：

| 指标 | 字段 | 类型 | 说明 |
|------|------|------|------|
| 总相似度 | `similarity_score` | number | 综合名称、简介、正文、触发场景、工具使用后的总体相似度，范围 `0`-`1` |
| 名称相似度 | `name_similarity` | number | Skill 名称 / slug 的语义相似度；完全同名仍由程序检查直接 `block` |
| 简介相似度 | `description_similarity` | number | `description` 的语义相似度 |
| 正文相似度 | `body_similarity` | number | `SKILL.md` 正文步骤、约束、输出要求的相似度 |
| 触发场景相似度 | `trigger_similarity` | number | 适用任务、触发词、使用场景是否重叠 |
| 工具 / MCP 相似度 | `tool_similarity` | number | 是否使用相同工具、MCP、外部系统或执行链路 |
| 冲突风险 | `conflict_risk` | `low` \| `medium` \| `high` | 两个 Skill 同时存在时是否会导致误触发、重复注入或相反指令 |
| 建议动作 | `recommendation` | `keep_separate` \| `suggest_refine` \| `block_duplicate` | 模型建议；`suggest_refine` 对应前端展示炼化按钮 |
| 置信度 | `confidence` | number | 模型对本次相似度判断的置信度，范围 `0`-`1` |
| 原因 | `reason` | string | 面向用户展示的简要中文解释 |
| 证据 | `evidence` | string[] | 支撑判断的关键片段或摘要 |

推荐规则：

- `similarity_score < 0.2`：默认 `keep_separate`，前端显示无冲突对号。
- `similarity_score >= 0.2`：默认 `suggest_refine`，进入冲突组并显示炼化按钮。
- 若模型判断为几乎完全重复，且 `similarity_score >= 0.8` 或 `recommendation = block_duplicate`，可升级为 `block`，禁止直接导入。
- 前端展示百分比时统一按 `Math.round(score * 100)` 显示，例如 `0.42` 展示为 `42%`。

### 3.2 导入状态机

| 状态 | 来源 | 前端行为 |
|------|------|----------|
| `idle` | 初始 | 展示上传区 |
| `uploading` | 文件提交中 | 禁用关闭和重复提交 |
| `processing` | 已获得 `job_id`，轮询中 | 每 1.5s 轮询一次 |
| `completed.pass` | 所有候选均无结构、名称、相似冲突 | 列表中每个上传 Skill 显示对号，允许「导入无冲突 Skill」 |
| `completed.warn` | 存在模型判定相似度 `>= 0.2` 的冲突组 | 展示无冲突列表 + 冲突组；冲突组内显示「炼化」按钮 |
| `completed.block` | 存在结构错误、规范错误、名称重复、模型不可用且无法继续检查等阻塞错误 | 展示错误，禁止入库，只能关闭或重新上传 |
| `failed` | 导入任务失败 | 展示失败原因，允许重试 |

轮询规则：

- 调用 `POST /skills/import` 成功后拿到 `job_id`。
- 使用 `GET /skills/import/:job_id` 轮询。
- 轮询终止条件：`status` 为 `completed` 或 `failed`。
- 前端最多轮询 120 秒；超时后提示「导入仍在处理中，请稍后刷新」。

### 3.3 导入结果处理

| 结果 | UI | 后续动作 |
|------|----|----------|
| 无冲突 | 每个候选 Skill 行显示绿色对号、名称、简介、目标存储目录 | 点击「导入无冲突 Skill」后入库并写入正式 Skill 目录 |
| 存在相似冲突组 | 按组展示候选 Skill 与已存在相似 Skill；组标题展示相似度、模型原因、证据摘要 | 每组显示「炼化」按钮；炼化完成并确认后，合并结果入库 |
| 存在 `block` | `QBanner` negative 展示阻塞原因 | 不能调用 apply；用户修正 zip 后重新上传 |

上传检查结果列表：

| 区域 | 展示内容 |
|------|----------|
| 无冲突列表 | 每个上传 Skill 一行，显示对号、`name`、`description`、`slug`、目标目录 |
| 阻塞列表 | 每个失败 Skill 一行，显示错误类型：结构错误、缺少 `SKILL.md`、名称重复、规范不通过 |
| 冲突组 | 按组展示，组内包含一个或多个上传候选 Skill，以及与其相似的已有 Skill |
| 冲突指标与证据 | 展示模型返回的 `similarity_score`、分项相似度、`conflict_risk`、`confidence`、`reason`、`evidence` |
| 组内操作 | 仅冲突组显示「炼化」按钮；没有冲突时不显示炼化入口 |

冲突组示例：

- 组标题：「发现 3 个相似 Skill，最高相似度 42%，建议炼化」
- 左侧：本次上传候选 Skill 的名称、简介、正文摘要。
- 右侧：库中已有相似 Skill 的名称、简介、版本、正文摘要。
- 底部：模型判定原因与「炼化」按钮。

处理策略：

| 策略 | 值 | 含义 |
|------|----|------|
| 导入无冲突 | `import_passed` | 仅将无冲突候选写入正式 Skill 目录并入库 |
| 跳过冲突组 | `skip_group` | 本次不导入该冲突组内的上传候选 |
| 炼化冲突组 | `merge_group_with_ai` | 调用已配置模型，将组内相似 Skill 合并为一个新 Skill 草稿 |

应用规则：

- 无冲突候选可以直接导入。
- 冲突组不会自动导入，也不会默认保留新的。
- 每个冲突组需要单独炼化或跳过。
- `block` 存在时，「应用结果」按钮始终禁用。
- `merge_group_with_ai` 默认生成 `draft`，不自动启用。
- 应用成功后关闭弹窗，刷新列表，并展示导入摘要。

### 3.4 AI 炼化

AI 炼化用于将一个冲突组中的相似 Skill 合并成一份新草稿。**不提供单独的全局炼化按钮**；只有上传检查后存在冲突组时，才在对应冲突组中显示「炼化」按钮。

| 区域 | 需求 |
|------|------|
| Provider / Model | 默认使用后端当前已配置可用模型；如存在多个可用模型，可在冲突组内展开高级设置选择 |
| 炼化按钮 | 仅冲突组内显示；无可用模型、请求中、冲突组为空时禁用 |
| 结果预览 | 使用 Markdown 编辑器或 `QInput type=textarea autogrow` 展示 |
| 保存 | 用户确认预览后，随 apply 请求提交 `merged_name`、`merged_description`、`merged_body`、`merged_tags` |
| 结果落库 | 合并结果作为新 Skill 草稿写入正式 Skill 目录；被合并的已有 Skill 不自动删除、不自动归档 |

失败处理：

- Provider / Model 不可用时，冲突组显示「模型不可用，无法炼化」。
- 炼化请求失败时，不影响无冲突 Skill 导入；该冲突组可重试或跳过。
- 炼化预览生成后，用户必须确认保存，不能自动入库。

---

## 4. 编辑 Skill

### 4.1 编辑入口

点击列表「编辑」进入编辑页或打开全屏 `QDialog`。本期默认使用独立路由：

| 路由 | 页面 |
|------|------|
| `/skills/new` | 新建 Skill 草稿 |
| `/skills/:id/edit` | 编辑已有 Skill |

若项目当前路由结构不支持子页面，可先用全屏 `QDialog` 实现，但字段与行为保持一致。

### 4.2 表单字段

| 字段 | 组件 | 校验 |
|------|------|------|
| 名称 `name` | `QInput` | 必填，1-80 字符，同租户唯一 |
| Slug `slug` | `QInput` | 可选；小写字母、数字、短横线、下划线 |
| 描述 `description` | `QInput type=textarea` | 必填，1-500 字符 |
| 标签 `tags` | `QSelect use-input use-chips` | 可为空；新增标签默认 `source=user` |
| 继承 `extends_skill_id` | `QSelect` 远程搜索 | 可为空；环检测由服务端完成 |
| 正文 `body` | Markdown 编辑器或大文本输入 | 发布时必填 |

### 4.3 保存与发布

| 动作 | API | 行为 |
|------|-----|------|
| 保存草稿 | `POST /skills` 或 `PATCH /skills/:id` | 保存元数据和正文，状态保持 `draft` |
| 发布 | `POST /skills/:id/publish` | 服务端校验结构、继承链、冲突检测，并生成不可变版本 |
| 取消 | 无 | 有未保存改动时弹确认 |

发布结果：

| 后端结果 | 前端行为 |
|----------|----------|
| `pass` | 展示成功，回到列表或留在编辑页 |
| `warn` | 展示冲突警告；用户确认后可继续发布 |
| `block` | 展示阻塞原因；不可发布 |

编辑页保存成功后，列表页需要能看到最新 `updated_at`、`status`、`current_version`。

---

## 5. Skill 运行记录页

### 5.1 页面结构

| 区域 | 内容 |
|------|------|
| 标题 | 「Skill 运行记录」 |
| 筛选 | Skill 远程搜索、Agent 选择、结果、时间范围、重置 |
| 表格 | `QTable` 服务端分页 |
| 行操作 | 「详情」按钮，根据权限展示完整输入 / 输出 |

筛选字段：

| 筛选项 | 参数 | 说明 |
|--------|------|------|
| Skill | `skill_id` | 远程搜索，展示 `name` + `version` |
| Agent | `agent_id` | 远程搜索或从 Agent 列表接口获取 |
| 结果 | `status` | 空 / `success` / `failure` |
| 时间范围 | `from`、`to` | ISO8601 字符串 |
| 页码 | `page`、`page_size` | 页码从 1 开始 |

### 5.2 表格列

| 列 | 字段 | UI |
|----|------|----|
| 时间 | `started_at` | 本地时间 |
| Skill | `skill_name`、`skill_version` | 主行名称，副行版本 `QChip size=sm` |
| Agent | `agent_display_name` | 若存在 Agent 详情页则可点击 |
| 结果 | `status` | `success` 绿色；`failure` 红色 |
| 耗时 | `duration_ms` | 小于 1000 展示 `ms`，否则展示 `s` |
| 输入 | `input_preview` / `input_hash` | preview 优先；敏感环境只展示 hash 短串 |
| 输出 | `output_preview` / `error_message` | 成功绿色；失败红色 |
| 操作 | `permissions.can_view_detail` | 可查看时展示「详情」 |

### 5.3 详情弹窗

点击「详情」打开 `QDialog`：

- 展示基本信息：时间、Skill、版本、Agent、状态、耗时、会话 ID。
- 管理员可查看完整 `input` / `output`，否则仅展示 preview / hash。
- 失败记录展示 `error_code`、`error_message`。
- 长文本使用等宽字体、可复制、最大高度滚动。

---

## 6. 数据模型字段

以下为前端需要消费或提交的逻辑字段。物理表可由后端拆分为主表、版本表、聚合表、明细日志。

### 6.1 `Skill`

| 字段 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `id` | string | 是 | Skill ID |
| `name` | string | 是 | 展示名 |
| `slug` | string | 否 | 文件友好标识 |
| `description` | string | 是 | 描述 |
| `tags` | `SkillTag[]` | 是 | 标签 |
| `extends_skill_id` | string \| null | 否 | 父 Skill |
| `status` | `draft` \| `published` \| `archived` | 是 | 状态 |
| `enabled` | boolean | 是 | 是否参与运行时注入 / 调度 |
| `current_version` | `SkillVersionSummary` \| null | 否 | 当前版本 |
| `invoke_count` | number | 是 | 累计调用次数 |
| `success_count` | number | 是 | 累计成功数 |
| `failure_count` | number | 是 | 累计失败数 |
| `usage_count_7d` | number | 否 | 近 7 日调用次数，用于展示近期使用频率 |
| `avg_duration_ms` | number \| null | 否 | 平均耗时 |
| `last_agent_id` | string \| null | 否 | 最近一次调用该 Skill 的 Agent ID |
| `last_agent_display_name` | string \| null | 否 | 最近一次调用该 Skill 的 Agent 名称 |
| `last_invoked_at` | string \| null | 否 | 最近调用时间 |
| `last_duration_ms` | number \| null | 否 | 最近一次调用耗时 |
| `created_at` | string | 是 | 创建时间 |
| `updated_at` | string | 是 | 更新时间 |
| `permissions` | object | 是 | 当前用户可执行操作 |

统计口径：

- `invoke_count` = `skill_invocation` 中同一 `skill_id` 的总记录数。
- `success_count` = `status = success` 的调用记录数。
- `failure_count` = `status = failure` 的调用记录数。
- `usage_count_7d` = `started_at >= now() - 7 days` 的调用记录数，便于展示近期使用频率。
- `avg_duration_ms` = 所有调用记录 `duration_ms` 的平均值；可由后端异步聚合。
- `last_agent_id` / `last_agent_display_name` / `last_invoked_at` / `last_duration_ms` 来自 `started_at` 最新的一条调用记录。
- 前端列表优先消费聚合字段，避免在列表页扫描运行明细。

### 6.2 `SkillVersionSummary`

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 版本 ID |
| `version` | string | 版本号 |
| `validation_status` | `pass` \| `warn` \| `block` | 最近发布校验结果 |
| `published_at` | string \| null | 发布时间 |

### 6.3 `SkillTag`

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | 标签名 |
| `source` | `system` \| `user` | 标签来源 |

### 6.4 `SkillInvocation`

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 调用记录 ID |
| `skill_id` | string | Skill ID |
| `skill_name` | string | Skill 名称快照 |
| `skill_version` | string | 实际执行版本 |
| `agent_id` | string | Agent ID |
| `agent_display_name` | string | Agent 名称快照 |
| `user_id` | string \| null | 触发用户 |
| `session_id` | string \| null | 会话 / 任务关联 |
| `status` | `success` \| `failure` | 执行结果 |
| `duration_ms` | number | 耗时 |
| `started_at` | string | 调用开始时间，用于按时间统计使用频率 |
| `ended_at` | string \| null | 调用结束时间；为空时以前端展示 `duration_ms` 为准 |
| `input_preview` | string \| null | 脱敏输入摘要 |
| `input_hash` | string \| null | 输入哈希 |
| `output_preview` | string \| null | 输出摘要 |
| `error_code` | string \| null | 错误码 |
| `error_message` | string \| null | 错误摘要 |
| `permissions` | object | 当前用户是否可查看详情 |

---

## 7. API 契约

### 7.1 通用分页响应

```json
{
  "items": [],
  "page": 1,
  "page_size": 20,
  "total": 123
}
```

### 7.2 Skill 列表

`GET /skills?search=&tags=&enabled=&status=&page=1&page_size=20`

响应：

```json
{
  "items": [
    {
      "id": "skill_01",
      "name": "Figma Code Connect",
      "slug": "figma-code-connect",
      "description": "Creates and maintains Figma Code Connect template files.",
      "tags": [{ "name": "figma", "source": "system" }],
      "extends_skill_id": null,
      "status": "published",
      "enabled": true,
      "current_version": {
        "id": "ver_01",
        "version": "1.0.0",
        "validation_status": "pass",
        "published_at": "2026-04-25T09:00:00Z"
      },
      "invoke_count": 12,
      "success_count": 10,
      "failure_count": 2,
      "usage_count_7d": 5,
      "avg_duration_ms": 2300,
      "last_agent_id": "agent_01",
      "last_agent_display_name": "Design Assistant",
      "last_invoked_at": "2026-04-25T09:30:00Z",
      "last_duration_ms": 1800,
      "created_at": "2026-04-20T09:00:00Z",
      "updated_at": "2026-04-25T09:00:00Z",
      "permissions": {
        "can_edit": true,
        "can_delete": true,
        "can_toggle_enabled": true,
        "can_duplicate": true
      }
    }
  ],
  "page": 1,
  "page_size": 20,
  "total": 1
}
```

### 7.3 创建 / 更新 / 发布

| 方法 | 路径 | 请求体 | 说明 |
|------|------|--------|------|
| `POST` | `/skills` | `SkillDraftPayload` | 创建草稿 |
| `PATCH` | `/skills/:id` | `SkillDraftPayload` | 更新草稿或元数据 |
| `POST` | `/skills/:id/publish` | `{ "confirm_warning": boolean }` | 发布并触发校验 |
| `PATCH` | `/skills/:id/enabled` | `{ "enabled": boolean }` | 启用 / 停用 |
| `DELETE` | `/skills/:id` | 无 | 软删 |
| `POST` | `/skills/:id/duplicate` | 无 | 复制为新草稿 |

`SkillDraftPayload`：

```json
{
  "name": "Skill name",
  "slug": "skill-name",
  "description": "Short description",
  "tags": [{ "name": "frontend", "source": "user" }],
  "extends_skill_id": null,
  "body": "# Skill body"
}
```

发布响应：

```json
{
  "validation_status": "pass",
  "skill": {},
  "warnings": [],
  "blocks": []
}
```

### 7.4 上传导入

`POST /skills/import`

- 请求：`multipart/form-data`，字段名 `file`。
- 后端行为：接收 zip 后写入临时目录，解压并严格校验 Skill 编写规范；通过校验后生成导入任务，不直接入库。
- 响应：

```json
{
  "job_id": "job_01"
}
```

`GET /skills/import/:job_id`

```json
{
  "job_id": "job_01",
  "status": "completed",
  "validation_status": "warn",
  "storage_root": "/var/lib/aranea/skills",
  "candidates": [
    {
      "candidate_id": "candidate_01",
      "name": "Figma Code Connect",
      "slug": "figma-code-connect",
      "description": "Imported skill description",
      "body_preview": "# Skill...",
      "target_dir": "/var/lib/aranea/skills/figma-code-connect",
      "validation_status": "pass",
      "status_icon": "check",
      "warnings": [],
      "blocks": []
    },
    {
      "candidate_id": "candidate_02",
      "name": "Figma Component Mapping",
      "slug": "figma-component-mapping",
      "description": "Map Figma components to code snippets.",
      "body_preview": "# Skill...",
      "target_dir": "/var/lib/aranea/skills/figma-component-mapping",
      "validation_status": "warn",
      "status_icon": "merge_suggested",
      "warnings": [
        {
          "type": "similarity",
          "message": "模型判定与已有 Skill 相似，建议炼化"
        }
      ],
      "blocks": []
    }
  ],
  "conflict_groups": [
    {
      "group_id": "group_01",
      "highest_similarity_score": 0.42,
      "metrics": {
        "similarity_score": 0.42,
        "name_similarity": 0.31,
        "description_similarity": 0.58,
        "body_similarity": 0.47,
        "trigger_similarity": 0.52,
        "tool_similarity": 0.66,
        "conflict_risk": "medium",
        "recommendation": "suggest_refine",
        "confidence": 0.84
      },
      "reason": "两个 Skill 都在描述 Figma 组件与代码模板的映射流程。",
      "evidence": [
        "Both mention Code Connect template files",
        "Both describe mapping Figma components to code snippets"
      ],
      "candidate_ids": ["candidate_02"],
      "existing_skills": [
        {
          "id": "skill_01",
          "name": "Figma Code Connect",
          "slug": "figma-code-connect",
          "description": "Creates and maintains Figma Code Connect template files.",
          "version": "1.0.0",
          "body_preview": "# Figma Code Connect..."
        }
      ],
      "can_refine": true
    }
  ]
}
```

`POST /skills/import/:job_id/apply`

```json
{
  "decisions": [
    {
      "candidate_id": "candidate_01",
      "action": "import_passed"
    },
    {
      "group_id": "group_01",
      "action": "merge_group_with_ai",
      "merged_name": "Figma Code Connect",
      "merged_description": "Merged description",
      "merged_body": "# Merged Skill body",
      "merged_tags": [{ "name": "figma", "source": "system" }]
    }
  ]
}
```

响应：

```json
{
  "created_skill_ids": ["skill_02"],
  "skipped_candidate_ids": [],
  "message": "导入完成"
}
```

### 7.5 冲突组炼化

`POST /skills/import/:job_id/conflict-groups/:group_id/refine`

```json
{
  "provider": "openai",
  "model": "gpt-4.1",
  "instructions": "合并重复功能，保留更清晰的触发条件和步骤。"
}
```

响应：

```json
{
  "merged_name": "Figma Code Connect",
  "merged_description": "Merged description",
  "merged_body": "# Merged Skill body",
  "merged_tags": [{ "name": "figma", "source": "system" }],
  "source_candidate_ids": ["candidate_02"],
  "source_existing_skill_ids": ["skill_01"]
}
```

### 7.6 运行记录

`GET /skill-runs?skill_id=&agent_id=&status=&from=&to=&page=1&page_size=20`

响应使用通用分页结构，`items[]` 为 `SkillInvocation`。

### 7.7 错误格式

所有接口错误统一返回：

```json
{
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "名称已存在",
    "details": {}
  }
}
```

前端展示规则：

- `message` 可直接展示给用户。
- `details.field_errors` 若存在，映射到表单字段。
- 未知错误展示「操作失败，请稍后重试」。

---

## 8. 待确认问题

以下问题需要产品 / 后端确认后才能完全闭环：

1. Skill 管理是否一定在全局 `/skills` 下，还是必须挂在 Team / Workspace 子路由下？
2. `tags` 查询参数最终使用逗号字符串、重复参数，还是 JSON 数组？
3. 上传 zip 的最大文件大小是多少？是否允许包内包含图片、脚本或多文件目录？脚本类文件是否一律拒绝？
4. `warn` 确认发布是否需要更高权限，还是编辑者即可确认？
5. Skill 正式存储根目录由哪个配置项提供？是否需要按租户 / 工作区分目录？
6. 编辑页本期使用独立路由还是全屏弹窗？本文默认独立路由。
7. 运行记录详情是否需要展示完整输入 / 输出原文？若展示，权限和脱敏规则由哪个接口返回？
8. Agent 选择器的数据来源是已有 Agent 列表接口，还是运行记录接口返回可选项？
9. `enabled=true` 的语义是参与注入、允许被调度，还是两者都包含？
10. 版本号由后端自动递增，还是前端允许用户填写 Semver？
11. 模型相似度检查使用哪个默认 Provider / Model？如果多个模型可用，是否允许用户在冲突组内切换？
12. 20% 相似阈值是否固定，还是需要后台配置？
13. 炼化成功后，参与合并的旧 Skill 是否仅保留不动，还是需要进入待复核 / 归档流程？

---

## 9. 验收标准

### 9.1 Skill 管理

- 进入 `/skills` 能看到 Skill 表格，支持加载、空态、错误态。
- 搜索、标签、启用状态、分页任一变化都会触发服务端查询。
- 启用 / 停用成功后当前行状态正确；失败后恢复原状态并提示。
- 无权限或状态不允许时，对应按钮禁用并展示原因。
- 删除前有确认弹窗，删除成功后列表刷新。

### 9.2 上传导入

- 只允许选择单个 `.zip` 文件。
- 上传成功后能进入轮询状态，并按结构校验、名称检查、模型相似度检查顺序执行。
- 不符合 Skill 编写规范、缺少 `SKILL.md`、名称 / slug 重复时进入 `block`。
- 无冲突候选在结果列表中显示绿色对号、名称、简介、slug、目标存储目录。
- 模型判定相似度 `>= 0.2` 时，候选 Skill 与已有相似 Skill 被归入冲突组。
- 冲突组展示总相似度、名称 / 简介 / 正文 / 触发场景 / 工具分项相似度、冲突风险、置信度、模型原因、证据摘要、候选 Skill 和已有 Skill 对比。
- 只有冲突组内显示「炼化」按钮；页面不提供单独的全局炼化按钮。
- 炼化生成合并预览后，用户确认才会保存为新 Skill 草稿。
- `block` 不能应用，只能关闭或重新上传。
- 无可用模型时，相似度检查和炼化都展示模型不可用原因，不能静默跳过。

### 9.3 编辑发布

- 新建和编辑都能保存草稿。
- 发布时能展示 `pass` / `warn` / `block` 结果。
- `warn` 需要用户确认后才能继续发布。
- `block` 阻止发布并展示原因。
- 有未保存改动时离开页面需要确认。

### 9.4 运行记录

- 进入 `/skills/runs` 能看到运行记录表格。
- Skill、Agent、结果、时间范围筛选会触发服务端查询。
- 成功和失败记录有明显颜色区分。
- 详情按钮按权限展示；无权限时不暴露完整输入 / 输出。

---

## 10. Quasar 组件清单

| 页面 | 主要组件 |
|------|----------|
| Skill 列表 | `QPage`、`QTable`、`QToggle`、`QChip`、`QBtn`、`QDialog`、`QPagination`、`QSelect`、`QInput` |
| 上传导入 | `QDialog`、`QUploader` 或拖拽 `QCard`、`QLinearProgress`、`QBanner`、检查结果列表、冲突组 `QCard` |
| 冲突组炼化 | 冲突组内 `QBtn`、可选 `QSelect`（provider/model）、`QInput type=textarea` 或 Markdown 编辑器 |
| 编辑页 | `QForm`、`QInput`、`QSelect`、`QExpansionItem`、`QBtn` |
| 运行记录 | `QTable`、`QBadge`、`QTooltip`、`QDialog`、`QDate` |

---

## 11. 运行时实现与演进方向

> 本节整合自 `architecture/agent-skills-tools-mcp-memory.md` 与 `architecture/trpc-agent-go-implementation-plan.md`，描述 Skill 在 Agent 运行时的装配机制与后续演进方向。

### 11.1 运行时加载机制（已实现）

Skill 在运行时通过 **trpc-agent-go ToolSet** 挂载到 `llmagent`，来源是启用且已发布的 Skill + 运行时策略与用户 query 收窄。

| 环节 | 位置 | 行为 |
|------|------|------|
| 列举候选 | `biz.SkillUsecase` | `ListEnabledPublishedSkillCandidates` 提供 slug、标签、taxonomy 等路由元数据 |
| 运行时策略 | `ag.Settings.SkillRuntimeJSON` | `biz.ParseSkillRuntimePolicy` → allow/deny、标签、intent 路由开关、`MaxSkillsInToolset` |
| 按需收窄 | `skillruntime.resolveSkillSlugs` | **Layer A**：allow/deny slug 过滤；**Layer B**：`skillrouter.DetectIntentPaths` 意图路径检测 + `filterByAllTags` 标签过滤 + `scoreCandidates` 评分排序 |
| Repository 适配 | `internal/skill/trpc/` | `DBRepositoryAdapter`（DB + TTL 缓存，优先）或 `FSRepositoryAdapter`（磁盘 FS，回退）→ `FilteredRepository`（白名单过滤） |
| Skill Tool 构建 | `internal/skill/trpc/tools.go` | `BuildSkillTools()` 产出 4 个 ADK 工具：LoadTool / RunTool / ListDocsTool / SelectDocsTool |
| CodeExecutor | `internal/skill/trpc/executor.go` | local / docker，按 `CODE_EXECUTOR_BACKEND` 环境变量选择 |
| 物理根路径 | `internal/skill/storage/root.go` | `SKILL_ROOT` / `SKILL_STORAGE_ROOT` 优先；否则结合系统设置 `work_directory`，或 OS 默认目录 |
| Agent 装配 | `internal/agent/trpc_build.go` | `buildSkillDeps` 组装 Repository + Filter + CodeExecutor → `trpcllmagent.WithSkills(repo)` + `WithSkillFilter(filter)` + `WithSkillToolProfile(SkillToolProfileFull)` |

**调用点**：单 Agent / Team 均通过 `buildSkillDeps`（`internal/agent/trpc_build.go`）统一装配。Team 对**每个成员**分别调用装配，每个成员使用**自己的** `SkillRuntimeJSON` 和生效工具集。用户首轮 `content` 作为 Skill 路由 query 在成员之间共享。

### 11.2 装配顺序（已实现）

`buildSkillDeps` 按以下顺序装配：

1. **Builtin Tools**（`ADKToolsForAgentPolicy`）
2. **Skill Toolsets**（`BuildSkillTools` → LoadTool / RunTool / ListDocsTool / SelectDocsTool）
3. **MCP Toolsets**（`mcpmount.AppendEffectiveMCPServerToolsets`）

新增工具类型必须通过统一装配路径挂载，不另开入口。Chat 和 Team 共用同一装配逻辑。

### 11.3 演进方向

| 方向 | 现状 | 建议 |
|------|------|------|
| Team 与 Skill 路由 | Team 仅用首位成员的 `SkillRuntimeJSON` + 用户 query 生成共享 Toolsets，其余成员共用同一 Skill 挂载 | 产品上若需要「成员 A 挂载写作 skill、成员 B 挂载检索 skill」，应在 builder 层按成员拆分或复制 Toolsets；或在团队定义中显式 `skill_profile` |
| Prompt 注入（方式 C） | 当前仅实现方式 A（FS 适配器）+ 方式 B（DB 适配器）→ ADK Toolset | 后续可增加纯 Prompt 注入：Assembler 产出 `## Available Skills` 文本块写入 system/developer message；`skilltoolset` 可选关闭 |
| embedding 语义精排 | 当前仅关键词 + 标签路由 | 候选筛选增加向量相似度匹配，替换或增强 `scoreCandidates` |
| Budget 中间件 | 当前仅 `MaxSkillsInToolset` 数量限制 | 注入 token 上限裁剪，按 Skill 优先级与 token 预算动态调整 |
| Preview API 增强 | `PreviewSkillRuntime` 返回已启用 slug 列表 + 存储根 | 返回每个 Skill 的选中原因（`Reasons map[string]string`），便于调试路由策略 |
| 配置来源统一 | Skill 根路径已统一到 `storage.ResolveRootWithPlatform()`，支持 env + 系统设置 + OS 默认 | 后续可增加热更新：系统设置变更后自动重新解析 Skill 根路径 |

---

*文档版本：3.3 — 更新运行时装配机制为已实现状态，对齐 `20 skill struct design.md`（2026-05-18）。*
