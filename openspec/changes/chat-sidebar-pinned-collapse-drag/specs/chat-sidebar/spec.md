# Chat Sidebar: Pinned + Collapse + Drag

## ADDED Requirements

### Requirement: System Agent Pinning

系统 Agent（`is_default=true`）SHALL 始终显示在 Agent 列表的最顶部位置，不可被其他 Agent 通过拖拽超越。系统 Agent 自身 SHALL 不可被拖拽移动。

系统 Agent 置顶 MUST 通过三层保障实现：
1. 初始加载时，`loadAgentOrder()` 将 default Agent 排到首位
2. 拖拽过程中，`<draggable :move="onMove">` 回调 MUST 阻止任何 Agent 拖到 index 0
3. 拖拽结束时，`onEndAgent()` 若系统 Agent 不在首位 MUST 强制归位

#### Scenario: 系统Agent初始加载置顶

WHEN 页面加载完成且 Agent 列表渲染完毕
THEN `is_default=true` 的 Agent MUST 出现在 Agent 大区的第一个位置

#### Scenario: 拖拽阻止超越系统Agent

WHEN 用户拖拽一个非系统 Agent 尝试将其放到 index 0 位置
THEN draggable 的 move 回调 MUST 返回 false 阻止该移动

#### Scenario: 系统Agent不可拖拽

WHEN 用户对系统 Agent（`is_default=true`）执行长按操作
THEN 该 Agent 行 MUST 不进入拖拽模式，长按无反应

#### Scenario: 拖拽结束后系统Agent归位

WHEN 拖拽操作结束后系统 Agent 不在首位
THEN `onEndAgent()` MUST 将系统 Agent 强制归位到列表首位

---

### Requirement: Collapsible Sections

Chat 侧边栏 SHALL 支持两层折叠：大区（Agent/Team）可折叠 + 分组可折叠。折叠状态 MUST 持久化到 localStorage。

大区折叠头和分组折叠头 SHALL 始终可见（只隐藏 items 内容）。折叠/展开时箭头 MUST 有旋转动画。

#### Scenario: 点击大区头折叠

WHEN 用户点击 Agent 或 Team 大区头
THEN 该大区下的所有分组和 items MUST 隐藏，大区头箭头旋转为折叠状态

#### Scenario: 点击大区头展开

WHEN 用户点击已折叠的大区头
THEN 该大区下的所有分组和 items MUST 显示，大区头箭头旋转为展开状态

#### Scenario: 点击分组头折叠

WHEN 用户点击分组头（如"系统 Agent"、"未分类 Agent"）
THEN 该分组下的 items MUST 隐藏，分组头箭头旋转为折叠状态，分组头本身 MUST 保持可见

#### Scenario: 点击分组头展开

WHEN 用户点击已折叠的分组头
THEN 该分组下的 items MUST 显示，分组头箭头旋转为展开状态

#### Scenario: 折叠状态持久化

WHEN 用户折叠或展开任意大区或分组
THEN 折叠状态 MUST 实时保存到 localStorage（大区 key 为 `chat:collapsed:sections`，分组 key 为 `chat:collapsed:groups`）

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
2. **ChatEntityGroup** — 分组容器组件，封装折叠逻辑和组内 vuedraggable 排序，接收 `items`、`label`、`icon`、`collapsed`、`draggable`、`pinnedId` props，emit `update:collapsed` 和 `reorder`
3. **ChatEntityItem** — 单行展示组件（纯展示），接收 `entity`、`active`、`statusIcon`、`statusColor`、`statusLabel` props，emit `click`、`settings`、`delete`

ChatEntitySidebar MUST 变为编排层，消费上述子组件。子组件 MUST NOT 跨层通信，所有事件由 ChatEntitySidebar 统一转发。

#### Scenario: ChatSectionHeader渲染

WHEN ChatEntitySidebar 渲染大区头或分组头
THEN MUST 使用 ChatSectionHeader 组件，正确传入 icon、label、count、collapsed props

#### Scenario: ChatEntityGroup渲染

WHEN ChatEntitySidebar 渲染一个分组
THEN MUST 使用 ChatEntityGroup 组件，正确传入 items、label、icon、collapsed、draggable、pinnedId props

#### Scenario: ChatEntityItem渲染

WHEN ChatEntityGroup 渲染分组内的每个条目
THEN MUST 使用 ChatEntityItem 组件，正确传入 entity、active、statusIcon、statusColor、statusLabel props

#### Scenario: 事件转发

WHEN ChatEntityItem 或 ChatEntityGroup emit 事件
THEN ChatEntitySidebar MUST 统一转发事件到上层，子组件 MUST NOT 跨层直接通信

---

### Requirement: Collapse State Composable

SHALL 新建 `useChatEntityCollapse` composable 管理折叠状态。该 composable MUST 提供 `sectionCollapsed`（大区折叠状态）、`groupCollapsed`（分组折叠状态）、`toggleSection`、`toggleGroup` 方法，并从 localStorage 恢复和保存状态。

#### Scenario: composable提供折叠状态

WHEN ChatEntitySidebar 使用 `useChatEntityCollapse` composable
THEN MUST 获得 `sectionCollapsed`（reactive，含 `agents` 和 `teams` 布尔值）和 `groupCollapsed`（reactive，Record<string, boolean>）

#### Scenario: 切换大区折叠

WHEN 调用 `toggleSection('agents')` 或 `toggleSection('teams')`
THEN 对应大区的折叠状态 MUST 翻转，并自动保存到 localStorage

#### Scenario: 切换分组折叠

WHEN 调用 `toggleGroup(groupKey)`
THEN 对应分组的折叠状态 MUST 翻转，并自动保存到 localStorage

#### Scenario: composable恢复状态

WHEN `useChatEntityCollapse` 初始化
THEN MUST 从 localStorage 恢复之前保存的折叠状态
