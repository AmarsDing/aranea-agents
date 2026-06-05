# 行业市场页规格

## ADDED Requirements

### Requirement: Metric Strip KPI 卡

行业市场页顶部 SHALL 展示 4 张 KPI glass 卡，分别显示：已启用行业数、总部门数、总岗位数、总 Agent 数。每张卡 SHALL 使用项目 `--glass-surface` / `--glass-border` / `--glass-blur-default` design token 渲染。数值 SHALL 使用等宽字体（tabular-nums）显示。已启用卡 SHALL 展示增量标记。岗位卡 SHALL 展示每岗位平均 Agent 比率。Agent 卡 SHALL 展示已部署数。

#### Scenario: 页面加载后展示 KPI 聚合

WHEN 行业列表加载完成
THEN Metric Strip SHALL 展示 4 张 KPI 卡，数值为所有行业的聚合统计（enabled / departments / positions / agents）
AND 每张卡的数值使用 `app-mono` 等宽字体

#### Scenario: 无行业数据时展示零值

WHEN 行业列表为空
THEN Metric Strip SHALL 展示 4 张 KPI 卡，所有数值为 0

#### Scenario: 已启用卡展示增量标记

WHEN 至少 1 个行业为 enabled 状态
THEN 已启用 KPI 卡 SHALL 在底部展示绿色增量标记文字

---

### Requirement: Toolbar 搜索与筛选

行业市场页 SHALL 提供 Toolbar 组件，包含搜索框、状态 chips、来源 chips 和视图切换按钮。搜索框 SHALL 支持按行业名称 / key / 描述模糊匹配（大小写不敏感）。状态 chips SHALL 包含"全部 / 已启用 / 已禁用"三个选项，每个 chip SHALL 显示对应计数。来源 chips SHALL 包含"全部 / 系统 / 自定义"三个选项。视图切换 SHALL 提供"网格"和"列表"两种模式。所有筛选状态 SHALL 通过 v-model 双向绑定到页面级 ref。

#### Scenario: 输入搜索词过滤行业

WHEN 用户在搜索框输入关键词
THEN 行业列表 SHALL 仅展示名称、key 或描述中包含该关键词（大小写不敏感）的行业

#### Scenario: 点击状态 chip 切换筛选

WHEN 用户点击"已启用"状态 chip
THEN 行业列表 SHALL 仅展示 enabled 为 true 的行业
AND 该 chip SHALL 显示已启用行业的数量

#### Scenario: 点击来源 chip 切换筛选

WHEN 用户点击"自定义"来源 chip
THEN 行业列表 SHALL 仅展示来源为自定义的行业（当前所有行业均为系统来源，结果为空）

#### Scenario: 视图切换不触发路由变化

WHEN 用户在网格和列表视图之间切换
THEN URL SHALL 保持不变
AND 搜索词、筛选条件和滚动位置 SHALL 保持不变

#### Scenario: 搜索框聚焦高亮

WHEN 搜索框获得焦点
THEN 搜索框边框 SHALL 变为 accent 色（`--color-accent`）并展示聚焦阴影

---

### Requirement: Monogram 图标替代 Emoji

行业卡、列表行和 Drawer 中的行业图标 SHALL 使用 monogram（大写字母缩写）替代 emoji。monogram SHALL 由行业 key 的前两个字母大写生成，放置在圆角矩形内，背景色由 key 的 hash 值映射到预设色板。色板 SHALL 包含至少 6 种渐变色（indigo / rose / sky / emerald / amber / violet）。新增行业时 SHALL 无需手动选色，自动由 key 派生。

#### Scenario: 行业卡展示 monogram 图标

WHEN 渲染 key 为 "software_development" 的行业卡
THEN monogram 区域 SHALL 显示 "SO" 两个大写字母
AND 背景为 key hash 映射的渐变色

#### Scenario: key 仅含非字母字符时回退

WHEN 行业 key 去除非字母后为空
THEN monogram SHALL 使用行业 name 的前两个字母

#### Scenario: 多个行业 monogram 不撞色

WHEN 存在 key 为 "software_development" 和 "finance" 的两个行业
THEN 两个行业的 monogram 背景色 SHALL 不同（由 hash 映射到不同色板索引）

---

### Requirement: IndustryCard 重写

IndustryCard SHALL 重写为 glass 卡样式，包含：monogram 图标 + 行业名称 + key 标签 + 启用状态 pill + 描述（最多 2 行截断）+ 4 个 metric（部门/岗位/Agent/已部署）+ 底部操作栏。卡 SHALL 支持点击选中（打开 Drawer）和键盘操作（Enter / Space）。hover 时 SHALL 上浮 2px 并高亮边框。选中状态 SHALL 展示 accent 色边框和光晕。所有 metric 数值 SHALL 使用等宽字体。

#### Scenario: 行业卡展示完整信息

WHEN 渲染一个拥有 3 部门、5 岗位的已启用行业
THEN 卡 SHALL 展示：monogram + 名称 + key + 绿色"已启用"pill + 描述 + 4 个 metric（3/5/5/0）+ "查看部门"操作按钮

#### Scenario: 行业卡 hover 上浮效果

WHEN 鼠标悬停在行业卡上
THEN 卡 SHALL 上浮 2px（translateY(-2px)）
AND 边框颜色变为 accent 半透明色

#### Scenario: 行业卡选中态

WHEN 行业卡处于选中状态（isOpen = true）
THEN 卡边框 SHALL 变为 `--color-accent` 色
AND 展示 2px accent 色光晕

#### Scenario: 行业卡键盘可访问

WHEN 行业卡获得焦点且用户按下 Enter 或 Space
THEN SHALL 触发 select 事件（与点击行为一致）

#### Scenario: metric 缺失时展示零值

WHEN 行业的 deptCount / posCount / agentCount / installed 字段为 undefined
THEN 对应 metric SHALL 展示 0

---

### Requirement: IndustryTableRow 列表视图行

列表视图 SHALL 使用 IndustryTableRow 组件渲染每行。每行 SHALL 展示：monogram + 名称 + key + 描述 + 部门数 + 岗位数 + Agent 数 + 已部署数 + 启用状态 pill + 来源标签。行 SHALL 支持点击触发 select 事件。hover 时 SHALL 高亮行背景。数值列 SHALL 右对齐并使用等宽字体。

#### Scenario: 列表行展示完整信息

WHEN 渲染一个拥有 3 部门、5 岗位的已启用行业行
THEN 行 SHALL 展示：monogram + 名称 + key + 描述 + 3 + 5 + 5 + 0 + 绿色"已启用"pill + "系统"来源标签

#### Scenario: 列表行点击触发 select

WHEN 用户点击列表行
THEN SHALL 触发 select 事件并传递对应 Industry 对象

#### Scenario: 列表行 hover 高亮

WHEN 鼠标悬停在列表行上
THEN 行背景 SHALL 变为 `--interaction-surface-hover` 色

---

### Requirement: IndustryDrawer 侧滑详情

点击行业卡或列表行 SHALL 在右侧滑出 Drawer（480px 宽），展示行业详情。Drawer SHALL 包含：monogram + 名称 + key + 来源 + 描述 + 3 个 metric 卡（部门/岗位/Agent）+ 部门列表（每个部门下展示岗位）。Drawer SHALL 支持遮罩点击关闭和 ESC 键关闭。Drawer SHALL 使用 `<Teleport to="body">` 渲染。打开/关闭 SHALL 有滑入滑出过渡动画。底部 SHALL 展示"查看 Prompt"和"安装"两个操作按钮。

#### Scenario: 点击行业卡打开 Drawer

WHEN 用户点击行业卡
THEN 右侧 SHALL 滑出 480px 宽的 Drawer
AND Drawer 展示该行业的 monogram、名称、key、描述和 metric

#### Scenario: Drawer 展示部门与岗位列表

WHEN Drawer 打开且行业详情加载完成
THEN Drawer 内 SHALL 展示部门列表
AND 每个部门下 SHALL 展示该部门的岗位列表（含 seniority_level 标签）

#### Scenario: 遮罩点击关闭 Drawer

WHEN Drawer 打开且用户点击遮罩区域
THEN Drawer SHALL 关闭

#### Scenario: ESC 键关闭 Drawer

WHEN Drawer 打开且用户按下 ESC 键
THEN Drawer SHALL 关闭

#### Scenario: Drawer 加载中状态

WHEN Drawer 打开且部门数据正在加载
THEN Drawer 内 SHALL 展示加载提示文字

#### Scenario: Drawer 无部门数据

WHEN Drawer 打开且该行业无部门数据
THEN Drawer 内 SHALL 展示空状态提示

#### Scenario: Drawer 底部操作按钮

WHEN Drawer 打开
THEN 底部 SHALL 展示"查看 Prompt"（ghost 样式）和"安装"（primary 样式）两个按钮

---

### Requirement: 网格/列表双视图

行业市场页 SHALL 支持网格视图和列表视图两种展示模式，默认为网格视图。网格视图 SHALL 使用 3 列 CSS Grid 布局，响应式断点：≤1024px 2 列，≤640px 1 列。列表视图 SHALL 使用 `<table>` 元素，表头包含行业名/描述/部门/岗位/Agent/已部署/状态/来源列。视图切换 SHALL 通过页面级 `view` ref 控制，不涉及路由变化。切换视图 SHALL 保留搜索词、筛选条件和滚动位置。

#### Scenario: 默认网格视图

WHEN 用户首次进入行业市场页
THEN 页面 SHALL 以网格视图展示行业卡

#### Scenario: 切换到列表视图

WHEN 用户点击列表视图按钮
THEN 页面 SHALL 以表格形式展示行业列表
AND 表头包含行业名、描述、部门、岗位、Agent、已部署、状态、来源 8 列

#### Scenario: 网格视图响应式

WHEN 视口宽度 ≤ 1024px
THEN 网格 SHALL 变为 2 列布局

WHEN 视口宽度 ≤ 640px
THEN 网格 SHALL 变为 1 列布局

#### Scenario: 列表视图数值列右对齐

WHEN 列表视图渲染表头和数据行
THEN 部门/岗位/Agent/已部署列 SHALL 右对齐并使用 tabular-nums

---

### Requirement: CTA 申请新行业卡

网格视图中 SHALL 在行业卡列表末尾展示一张 CTA 卡，使用 dashed 边框 + 透明背景。CTA 卡 SHALL 展示 "+" 圆形图标 + "申请新行业" 标题 + 副标题说明文字。hover 时 SHALL 变为 accent 色。CTA 卡 SHALL 不展示任何假数据。

#### Scenario: 网格视图展示 CTA 卡

WHEN 网格视图渲染完成
THEN 行业卡列表末尾 SHALL 展示一张 dashed 边框的 CTA 卡

#### Scenario: CTA 卡 hover 效果

WHEN 鼠标悬停在 CTA 卡上
THEN CTA 卡文字和边框 SHALL 变为 accent 色（`--color-accent`）

#### Scenario: 列表视图不展示 CTA 卡

WHEN 列表视图渲染
THEN SHALL 不展示 CTA 卡

---

### Requirement: useIndustryMarket 扩展

`useIndustryMarket` composable SHALL 扩展以下能力：`summary` computed 聚合所有行业的 KPI 数据（总行业数/已启用/已禁用/部门数/岗位数/Agent数/已部署数）；`fetchIndustries` 内部 SHALL 对每个行业并行调用 `listDepartments` + `listPositions` 填充 `deptCount` / `posCount` / `agentCount`；`fetchIndustryDetail` SHALL 拉取单个行业的部门+岗位详情供 Drawer 使用；单个行业的并行 fetch 失败 SHALL 不影响其他行业数据。Industry 类型 SHALL 扩展可选字段 `deptCount?` / `posCount?` / `agentCount?` / `installed?`，不破坏后端契约。

#### Scenario: fetchIndustries 并行填充 counts

WHEN 调用 fetchIndustries 且后端返回 3 个行业
THEN SHALL 并行发起 6 次 API 请求（每个行业 2 次：listDepartments + listPositions）
AND 每个行业对象的 deptCount / posCount / agentCount SHALL 被填充

#### Scenario: 单个行业 fetch 失败不影响整体

WHEN 某个行业的 listDepartments 或 listPositions 请求失败
THEN 该行业 SHALL 保留原始数据（counts 为 undefined）
AND 其他行业的 counts SHALL 正常填充

#### Scenario: summary computed 聚合 KPI

WHEN industries 包含 3 个行业，counts 分别为 [3,5,5,0] / [2,4,4,0] / [4,8,8,0]
THEN summary.departments SHALL 为 9
AND summary.positions SHALL 为 17
AND summary.agents SHALL 为 17
AND summary.enabled SHALL 为已启用行业数

#### Scenario: agentCount 缺失时按 posCount 兜底

WHEN 行业的 agentCount 为 undefined 但 posCount 有值
THEN summary 聚合时 SHALL 按 posCount 作为 agentCount 的兜底值

#### Scenario: fetchIndustryDetail 拉取详情

WHEN 调用 fetchIndustryDetail("software_development")
THEN SHALL 并行调用 listDepartments 和 listPositions
AND 返回的 IndustryDetail SHALL 包含 departments 数组和 positionsByDept 分组映射

---

### Requirement: 零新增依赖

本次变更 SHALL 不引入任何新的 npm 依赖。所有样式 SHALL 使用项目已有的 design token（`--glass-surface` / `--glass-border` / `--glass-blur-default` / `--color-accent` / `--color-success` / `--color-success-soft` / `--color-text-primary` / `--color-text-secondary` / `--color-text-tertiary` / `--space-*` / `--interaction-surface-hover` / `--color-border-soft` / `--color-surface-solid` / `--canvas-base` / `--color-icon-muted`）。新增样式 SHALL 作为 scoped CSS 或独立 partial 文件，不破坏现有 CSS 入口结构。

#### Scenario: 不引入新 npm 包

WHEN 变更完成
THEN `package.json` 的 dependencies 和 devDependencies SHALL 无新增条目

#### Scenario: 使用已有 design token

WHEN 渲染行业市场页所有组件
THEN 所有颜色、间距、边框、阴影 SHALL 引用项目已有的 CSS custom property（design token），不使用硬编码值（monogram 渐变色板除外）

---

### Requirement: 空状态处理

当筛选结果为空时，行业市场页 SHALL 展示空状态提示区域，包含标题和提示文字。空状态区域 SHALL 使用 dashed 边框 + glass 背景样式。

#### Scenario: 搜索无结果展示空状态

WHEN 用户输入搜索词且无匹配行业
THEN 页面 SHALL 展示空状态区域（dashed 边框 + glass 背景 + 标题 + 提示文字）

#### Scenario: 筛选无结果展示空状态

WHEN 用户选择筛选条件后无匹配行业
THEN 页面 SHALL 展示空状态区域

---

### Requirement: 签名引用区

行业市场页底部 SHALL 展示签名引用区（signature quote），包含水平装饰线 + 斜体引用文字 + 署名文字。引用区 SHALL 使用 `--color-accent` 装饰线和 `--color-text-tertiary` 文字色。

#### Scenario: 页面底部展示签名引用

WHEN 行业市场页渲染完成
THEN 页面底部 SHALL 展示包含装饰线、斜体引用和署名的签名区域
