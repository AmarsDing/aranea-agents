# Chat Sidebar: Pinned + Collapse + Drag

## ADDED Requirements

### Requirement: System Agent Pinning

系统 Agent（`is_default=true`）SHALL 始终显示在 Agent 列表的最顶部位置，不可被其他 Agent 通过拖拽超越。系统 Agent 自身 SHALL 不可被拖拽移动。

系统 Agent 置顶 MUST 通过三层保障实现：
1. 初始加载时，`loadAgentOrder()` 将 default Agent 排到首位
2. 拖拽过程中，`<draggable :move="onMove">` 回调 MUST 阻止任何 Agent 拖到 pinnedId 元素之前（检查 `relatedContext.element.id === pinnedId && newIndex <= 0`）
3. 拖拽结束时，`onDragEnd()` 若系统 Agent 不在首位 MUST 强制归位

#### Scenario: 系统Agent初始加载置顶

WHEN 页面加载完成且 Agent 列表渲染完毕
THEN `is_default=true` 的 Agent MUST 出现在 Agent 大区的第一个位置

#### Scenario: 拖拽阻止超越系统Agent

WHEN 用户拖拽一个非系统 Agent 尝试将其放到 pinnedId 元素之前的位置
THEN draggable 的 move 回调 MUST 返回 false 阻止该移动

#### Scenario: 系统Agent不可拖拽

WHEN 用户对系统 Agent（`is_default=true`）执行长按操作
THEN 该 Agent 行 MUST 不进入拖拽模式，长按无反应

#### Scenario: 拖拽结束后系统Agent归位

WHEN 拖拽操作结束后系统 Agent 不在首位
THEN `onDragEnd()` MUST 将系统 Agent 强制归位到列表首位

---

### Requirement: Collapsible Sections

Chat 侧边栏 SHALL 支持多层折叠：大区（Agent/Active Teams/Completed Teams）可折叠 + Agent 分组可折叠。折叠状态 MUST 持久化到 localStorage。

大区折叠头和分组折叠头 SHALL 始终可见（只隐藏 items 内容）。折叠/展开时箭头 MUST 有旋转动画。

> **实现偏差**：原始设计为"两层折叠（Agent/Team 大区 + 分组）"，实际实现中 Team 拆分为 Active Teams 和 Completed Teams 两个独立大区，且 Team 大区不使用分组折叠，而是使用 TeamTaskCard 的展开/折叠。

#### Scenario: 点击大区头折叠

WHEN 用户点击 Agent、Active Teams 或 Completed Teams 大区头
THEN 该大区下的所有分组和 items MUST 隐藏，大区头箭头旋转为折叠状态

#### Scenario: 点击大区头展开

WHEN 用户点击已折叠的大区头
THEN 该大区下的所有分组和 items MUST 显示，大区头箭头旋转为展开状态

#### Scenario: 点击分组头折叠

WHEN 用户点击 Agent 分组头（如"系统 Agent"、"自定义 Agent"）
THEN 该分组下的 items MUST 隐藏，分组头箭头旋转为折叠状态，分组头本身 MUST 保持可见

#### Scenario: 点击分组头展开

WHEN 用户点击已折叠的 Agent 分组头
THEN 该分组下的 items MUST 显示，分组头箭头旋转为展开状态

#### Scenario: 折叠状态持久化

WHEN 用户折叠或展开任意大区或分组
THEN 折叠状态 MUST 实时保存到 localStorage（大区 key 为 `chat:collapsed:sections`，分组 key 为 `chat:collapsed:groups`）

> **实现偏差**：`chat:collapsed:sections` 的值结构为 `{ agents: bool, teams: bool, activeTeams: bool, completedTeams: bool }`，而非原始设计的 `{ agents: bool, teams: bool }`。

#### Scenario: 页面加载恢复折叠状态

WHEN 页面重新加载
THEN 折叠状态 MUST 从 localStorage 恢复，与上次离开时一致

---

### Requirement: Intra-Group Drag Reorder

分组内 Agent SHALL 支持拖拽排序。拖拽 MUST 仅限分组内，不可跨分组拖动。拖拽 MUST 通过长按 300ms 触发，不使用拖拽手柄图标。

拖拽排序结果 MUST 持久化到 localStorage（组内 key 为 `chat:order:agents:{groupKey}`），同时 MUST 同步更新全局排序 localStorage（key 为 `chat:order:agents`）。

#### Scenario: 长按触发拖拽

WHEN 用户在可拖拽的 Agent 行上长按 300ms
THEN 该行 MUST 进入拖拽模式，行轻微上浮 + 阴影加深 + 光标变为 grabbing

#### Scenario: 短按不触发拖拽

WHEN 用户在 Agent 行上按下但未持续 300ms 即释放
THEN 该行 MUST NOT 进入拖拽模式，正常触发点击选择事件

#### Scenario: 分组内拖拽排序

WHEN 用户在分组内拖拽一个 Agent 到新位置并释放
THEN 该 Agent MUST 移动到新位置，组内排序 MUST 保存到 localStorage，全局排序 MUST 同步更新

#### Scenario: 禁止跨分组拖拽

WHEN 用户尝试将 Agent 拖拽到其他分组
THEN 拖拽 MUST 被阻止，Agent MUST 留在原分组

#### Scenario: 拖拽动画

WHEN 拖拽进行中
THEN 被拖拽元素的 ghost 样式 MUST 为半透明，动画时长 MUST 为 200ms

#### Scenario: 新增Agent排序

WHEN 新增一个 Agent 到某分组
THEN 该 Agent MUST 追加到该分组的末尾位置

---

### Requirement: Search and Collapse Interaction

搜索过滤在分组前执行（保持现有逻辑）。搜索有内容时 MUST 自动展开所有折叠的分组。搜索清空时 MUST 恢复之前的折叠状态。大区折叠时，搜索仍然过滤但整个区域隐藏。

#### Scenario: 搜索激活时展开分组

WHEN 用户在搜索框输入内容且存在匹配结果
THEN 所有折叠的分组 MUST 自动展开以显示搜索结果

#### Scenario: 搜索清空时恢复折叠

WHEN 用户清空搜索框
THEN 分组折叠状态 MUST 恢复到搜索前的状态

#### Scenario: 大区折叠时搜索

WHEN 用户折叠了 Agent 大区并在搜索框输入内容
THEN 搜索过滤 MUST 仍然执行，但整个 Agent 大区 MUST 保持隐藏

---

### Requirement: Component Refactoring

ChatEntitySidebar MUST 拆分为以下子组件：

1. **ChatSectionHeader** — 可复用的折叠头组件，接收 `icon`、`label`、`count`、`collapsed` props，emit `update:collapsed`
2. **ChatEntityGroup** — 分组容器组件，封装折叠逻辑和组内 vuedraggable 排序，接收 `items`、`label`、`icon`、`collapsed`、`activeId`、`pinnedId`、`settingsAriaLabel`、`deleteAriaLabel` props，emit `update:collapsed`、`select`、`settings`、`delete` 和 `reorder`
3. **ChatEntityItem** — 单行展示组件（纯展示），接收 `name`、`active`、`statusIcon`、`statusColor`、`statusLabel`、`settingsAriaLabel`、`deleteAriaLabel` props，emit `click`、`settings`、`delete`

ChatEntitySidebar MUST 变为编排层，消费上述子组件以及 SpiritEntry 和 TeamTaskCard。子组件 MUST NOT 跨层通信，所有事件由 ChatEntitySidebar 统一转发。

> **实现偏差**：
> - ChatEntityItem 使用 `name` prop 替代原始设计的 `entity` prop（将 entity 对象拆分为独立 props）
> - ChatEntityGroup 无 `draggable` prop（draggable 始终启用），新增 `activeId`、`settingsAriaLabel`、`deleteAriaLabel` props 和 `select`、`settings`、`delete` emits
> - ChatEntitySidebar 额外消费 SpiritEntry（精灵助手入口）和 TeamTaskCard（Team 行卡片）组件

#### Scenario: ChatSectionHeader渲染

WHEN ChatEntitySidebar 渲染大区头或分组头
THEN MUST 使用 ChatSectionHeader 组件，正确传入 icon、label、count、collapsed props

#### Scenario: ChatEntityGroup渲染

WHEN ChatEntitySidebar 渲染一个分组
THEN MUST 使用 ChatEntityGroup 组件，正确传入 items、label、icon、collapsed、activeId、pinnedId props

#### Scenario: ChatEntityItem渲染

WHEN ChatEntityGroup 渲染分组内的每个条目
THEN MUST 使用 ChatEntityItem 组件，正确传入 name、active、statusIcon、statusColor、statusLabel props

#### Scenario: 事件转发

WHEN ChatEntityItem 或 ChatEntityGroup emit 事件
THEN ChatEntitySidebar MUST 统一转发事件到上层，子组件 MUST NOT 跨层直接通信

---

### Requirement: Collapse State Composable

SHALL 新建 `useChatEntityCollapse` composable 管理折叠状态。该 composable MUST 提供 `sectionCollapsed`（大区折叠状态，含 `agents`、`teams`、`activeTeams`、`completedTeams` 布尔值）、`groupCollapsed`（分组折叠状态，Record<string, boolean>）、`toggleSection`、`toggleGroup`、`isGroupCollapsed`、`expandAllGroups`、`onSearchActive`、`onSearchClear` 方法，并从 localStorage 恢复和保存状态。

#### Scenario: composable提供折叠状态

WHEN ChatEntitySidebar 使用 `useChatEntityCollapse` composable
THEN MUST 获得 `sectionCollapsed`（reactive，含 `agents`、`teams`、`activeTeams`、`completedTeams` 布尔值）和 `groupCollapsed`（reactive，Record<string, boolean>）

#### Scenario: 切换大区折叠

WHEN 调用 `toggleSection('agents')`、`toggleSection('teams')`、`toggleSection('activeTeams')` 或 `toggleSection('completedTeams')`
THEN 对应大区的折叠状态 MUST 翻转，并自动保存到 localStorage

#### Scenario: 切换分组折叠

WHEN 调用 `toggleGroup(groupKey)`
THEN 对应分组的折叠状态 MUST 翻转，并自动保存到 localStorage

#### Scenario: composable恢复状态

WHEN `useChatEntityCollapse` 初始化
THEN MUST 从 localStorage 恢复之前保存的折叠状态

#### Scenario: 搜索激活时展开分组

WHEN 调用 `onSearchActive()`
THEN MUST 快照当前分组折叠状态，并展开所有分组

#### Scenario: 搜索清空时恢复折叠

WHEN 调用 `onSearchClear()`
THEN MUST 从快照恢复分组折叠状态，并保存到 localStorage

---

### Requirement: Spirit Entry

ChatEntitySidebar 顶部 SHALL 显示 SpiritEntry 组件作为精灵助手入口。SpiritEntry 接收 `active` prop，emit `click` 事件。

#### Scenario: SpiritEntry渲染

WHEN ChatEntitySidebar 渲染
THEN MUST 在搜索框下方、Agent 大区上方显示 SpiritEntry 组件

#### Scenario: 点击SpiritEntry

WHEN 用户点击 SpiritEntry
THEN ChatEntitySidebar MUST emit `select-spirit` 事件

---

### Requirement: Team Sections

ChatEntitySidebar SHALL 将 Team 列表拆分为"进行中"（Active Teams）和"已完成"（Completed Teams）两个大区。每个大区使用 ChatSectionHeader 作为折叠头，内部使用 TeamTaskCard 组件渲染每个 Team。

#### Scenario: Team大区拆分

WHEN ChatEntitySidebar 渲染 Team 列表
THEN MUST 将 `status !== 'completed'` 的 Team 显示在"进行中"大区，`status === 'completed'` 的 Team 显示在"已完成"大区

#### Scenario: TeamTaskCard渲染

WHEN 渲染一个 Team 条目
THEN MUST 使用 TeamTaskCard 组件，传入 `team`、`expanded`、`active` props

#### Scenario: TeamTaskCard交互

WHEN 用户点击 TeamTaskCard
THEN ChatEntitySidebar MUST emit `select-spirit-team` 事件（传入 teamId）

WHEN 用户点击 TeamTaskCard 的展开箭头
THEN ChatEntitySidebar MUST emit `toggle-team-expand` 事件（传入 teamId）

---

### Requirement: EntityItem Status Helpers

SHALL 在 `chatUi.ts` 中提供 EntityItem 状态工具函数：`entityStatusIconFor`、`entityStatusColorFor`、`entityStatusLabelFor`，供 ChatEntityGroup 使用。

#### Scenario: 状态图标

WHEN EntityItem 处于工作中状态
THEN `entityStatusIconFor` MUST 返回 `'bolt'`

WHEN EntityItem 处于非工作中状态
THEN `entityStatusIconFor` MUST 返回 `'task_alt'`

#### Scenario: 状态颜色

WHEN EntityItem 处于工作中状态
THEN `entityStatusColorFor` MUST 返回 `'negative'`

WHEN EntityItem 处于停用状态
THEN `entityStatusColorFor` MUST 返回 `'grey'`

WHEN EntityItem 处于空闲状态
THEN `entityStatusColorFor` MUST 返回 `'positive'`

#### Scenario: 状态标签

WHEN EntityItem 处于工作中状态
THEN `entityStatusLabelFor` MUST 返回 `'工作中'`

WHEN EntityItem 处于停用状态
THEN `entityStatusLabelFor` MUST 返回 `'已停用'`

WHEN EntityItem 处于空闲状态
THEN `entityStatusLabelFor` MUST 返回 `'空闲'`

---

### Requirement: Group Reorder Persistence

SHALL 在 `useChatSidebarOrder` 中提供 `onGroupReorder(groupKey, ids)` 方法，保存组内排序到 localStorage 并同步更新全局排序。

#### Scenario: 组内排序持久化

WHEN 调用 `onGroupReorder(groupKey, ids)`
THEN MUST 调用 `saveGroupOrder(groupKey, ids)` 保存组内排序到 `chat:order:agents:{groupKey}`
AND MUST 同步更新全局排序 `chat:order:agents`

---

### Requirement: Agent Grouping

ChatEntitySidebar SHALL 将 Agent 列表分为"系统 Agent"（`is_default=true`）和"自定义 Agent"（`is_default=false`）两个分组。每个分组使用 ChatEntityGroup 组件渲染。

#### Scenario: 系统Agent分组

WHEN 存在 `is_default=true` 的 Agent
THEN MUST 创建 key 为 `'system'`、label 为 `'系统 Agent'`、icon 为 `'verified'` 的分组

#### Scenario: 自定义Agent分组

WHEN 存在 `is_default=false` 的 Agent
THEN MUST 创建 key 为 `'custom'`、label 为 `'自定义 Agent'`、icon 为 `'person'` 的分组

#### Scenario: 分组排序

WHEN 渲染分组内的 Agent 列表
THEN MUST 使用 `loadGroupOrder(items, groupKey, pinnedId)` 按组内 localStorage 排序
