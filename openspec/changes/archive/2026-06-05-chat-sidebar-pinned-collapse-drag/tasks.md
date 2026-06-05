# Chat Sidebar 增强：置顶 + 折叠 + 拖拽排序 — 任务清单

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 ChatEntitySidebar 拆分为子组件，增加系统 Agent 置顶、两层折叠、分组内拖拽排序三大交互能力。

**Architecture:** 方案 B — 拆分子组件。ChatEntitySidebar 变为编排层，新增 ChatSectionHeader + ChatEntityGroup + ChatEntityItem 三个展示子组件 + useChatEntityCollapse composable。

**Tech Stack:** Vue 3 + Quasar + vuedraggable + TypeScript + localStorage

**Design Doc:** [design.md](./design.md)

---

## Task 1: 创建 useChatEntityCollapse composable

**Files:**
- Create: `web/src/features/chat/composables/useChatEntityCollapse.ts`

- [x] **Step 1:** 创建 `useChatEntityCollapse.ts`，实现折叠状态管理

```typescript
const LS_SECTION_COLLAPSED = "chat:collapsed:sections";  // { agents: bool, teams: bool }
const LS_GROUP_COLLAPSED = "chat:collapsed:groups";       // { [groupKey]: bool }
```

功能：
- `sectionCollapsed` reactive：大区折叠状态
- `groupCollapsed` reactive：分组折叠状态
- `restore()`：从 localStorage 恢复
- `save()`：保存到 localStorage
- `toggleSection(section)`：切换大区折叠
- `toggleGroup(key)`：切换分组折叠

**DoD:**
- composable 存在且 TypeScript 编译通过
- 折叠状态持久化到 localStorage
- `cd web && pnpm build` 通过

---

## Task 2: 创建 ChatSectionHeader 组件

**Files:**
- Create: `web/src/components/chat/ChatSectionHeader.vue`

- [x] **Step 1:** 创建可复用的折叠头组件

Props: `icon`, `label`, `count`, `collapsed`
Emits: `update:collapsed`

模板：图标 + 标签 + 计数 badge + 展开箭头（旋转动画）

**DoD:**
- 组件存在且渲染正常
- 点击可切换折叠状态
- 箭头旋转动画流畅

---

## Task 3: 创建 ChatEntityItem 组件

**Files:**
- Create: `web/src/components/chat/ChatEntityItem.vue`

- [x] **Step 1:** 创建单行展示组件

Props: `entity`, `active`, `statusIcon`, `statusColor`, `statusLabel`
Emits: `click`, `settings`, `delete`

模板：状态图标 + 名称 + 状态 pill + 操作按钮（hover 显示）

**DoD:**
- 组件存在且渲染正常
- 点击/select 行为正确
- 操作按钮 hover 显示

---

## Task 4: 创建 ChatEntityGroup 组件

**Files:**
- Create: `web/src/components/chat/ChatEntityGroup.vue`

- [x] **Step 1:** 创建分组容器组件

Props: `items`, `label`, `icon`, `collapsed`, `draggable`, `pinnedId`
Emits: `update:collapsed`, `reorder`

功能：
- 使用 `ChatSectionHeader` 作为分组头
- 折叠时隐藏 items，仅显示分组头
- 使用 `vuedraggable` 实现组内排序
- `onMove` 回调阻止拖到 pinnedId 之前
- `delay: 300` 长按触发拖拽
- 系统 Agent（pinnedId）disabled 不可拖动

**DoD:**
- 组件存在且编译通过
- 折叠/展开正常
- 组内拖拽排序正常
- 系统 Agent 不可拖动
- 排序结果 emit 给上层

---

## Task 5: 更新 chatWorkspaceUtils 排序工具

**Files:**
- Modify: `web/src/features/chat/composables/chatWorkspaceUtils.ts`

- [x] **Step 1:** 新增组内排序工具函数

新增：
- `loadGroupOrder(groupKey: string): string[]`：从 localStorage 读取组内排序
- `saveGroupOrder(groupKey: string, ids: string[]): void`：保存组内排序到 localStorage
- `applyGroupOrder(items: Agent[], groupKey: string): Agent[]`：按组内排序重排 items

**DoD:**
- 函数存在且编译通过
- 组内排序与全局排序兼容

---

## Task 6: 重构 ChatEntitySidebar 为编排层

**Files:**
- Modify: `web/src/components/chat/ChatEntitySidebar.vue`

- [x] **Step 1:** 重构 ChatEntitySidebar

变更：
- 引入 `ChatSectionHeader`、`ChatEntityGroup`、`ChatEntityItem` 子组件
- 引入 `useChatEntityCollapse` composable
- Agent 大区和 Team 大区各使用 `ChatSectionHeader`
- 每个 Agent 分组使用 `ChatEntityGroup`
- 搜索激活时自动展开所有分组
- 搜索清空时恢复折叠状态
- 监听 `reorder` 事件，保存排序到 localStorage

**DoD:**
- ChatEntitySidebar 代码行数 < 300 行（从 500 行拆分后）
- 所有现有功能不受影响（搜索、选择、新建、删除）
- 新增折叠和拖拽功能正常
- `cd web && pnpm build` 通过

---

## Task 7: 全量验证

- [x] **Step 1:** 前端 lint

Run: `cd web && pnpm lint`
Expected: PASS

- [x] **Step 2:** 前端 build

Run: `cd web && pnpm build`
Expected: PASS

- [x] **Step 3:** 手动集成测试

1. 打开 Chat 页面
2. 验证系统 Agent 置顶
3. 验证大区折叠/展开
4. 验证分组折叠/展开
5. 验证分组内拖拽排序
6. 验证系统 Agent 不可拖动
7. 验证搜索时自动展开
8. 验证刷新后折叠状态持久化
9. 验证刷新后排序持久化

- [x] **Step 4:** Final commit

```bash
git add -A
git commit -m "feat(chat): add pinned/collapse/drag to ChatEntitySidebar"
```
