# Chat Sidebar: Pinned + Collapse + Drag

## Why

Chat 侧边栏（ChatEntitySidebar）已膨胀至 500 行，随着 Agent/Team 数量增长，缺少分组折叠和排序能力导致用户体验下降：系统 Agent 无法置顶、分组不可折叠、无法自定义排序。需要拆分子组件并增加置顶、折叠、拖拽排序三大交互能力。

## Goals

- 系统 Agent（`is_default=true`）始终置顶，不可被拖拽超越
- 两层折叠：大区（Agent/Team）可折叠 + 分组可折叠，状态持久化到 localStorage
- 分组内拖拽排序，长按 300ms 触发，不可跨分组拖动
- 将 ChatEntitySidebar 拆分为 ChatSectionHeader + ChatEntityGroup + ChatEntityItem 三个展示子组件

## Non-goals

- 不改后端（无 API 变更）
- 不改 Store 层（数据流不变）
- 不实现跨分组拖动
- 不实现拖拽手柄图标
