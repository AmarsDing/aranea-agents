# Chat Sidebar 增强：系统 Agent 置顶 + 折叠 + 拖拽排序

> 日期：2026-05-30
> 状态：设计已批准

---

## 1. 需求摘要

| 需求 | 决策 |
|------|------|
| 系统 Agent 置顶 | `is_default=true` 的 Agent 始终在列表最顶部，不可被拖拽超越 |
| 折叠功能 | 两层折叠：大区（Agent/Team）可折叠 + 分组可折叠，状态持久化到 localStorage |
| 拖拽排序 | 仅分组内排序，不可跨分组拖动；长按 300ms 触发，无拖拽手柄图标 |
| 持久化 | 仅 localStorage，不改后端 |

---

## 2. 方案选择

**方案 B：拆分子组件**（已批准）

将 ChatEntitySidebar 拆分为 `ChatSectionHeader` + `ChatEntityGroup` + `ChatEntityItem` 三个展示子组件，ChatEntitySidebar 变为编排层。

理由：
1. ChatEntitySidebar 已 500 行，加入折叠+拖拽后必然膨胀，拆分必要
2. 折叠和拖拽天然内聚在分组级别，ChatEntityGroup 封装最自然
3. 符合项目前端规范：展示组件放 `components/chat/`，props in / emits out
4. vuedraggable 已有使用先例（AgentsListSection.vue），模式可复用

---

## 3. 组件架构

### 3.1 新组件层级

```
ChatEntitySidebar.vue              ← 编排层：搜索 + 两大区 + 空状态
  ├─ ChatSectionHeader.vue         ← 可复用的大区/分组折叠头
  ├─ ChatEntityGroup.vue           ← 分组容器：折叠 + 组内 draggable
  │   └─ ChatEntityItem.vue        ← 单个 Agent/Team 行（纯展示）
  └─ (Team 侧同结构)
```

### 3.2 各组件职责

| 组件 | 职责 | 关键 props | 关键 emits |
|------|------|-----------|-----------|
| **ChatEntitySidebar** | 编排：搜索过滤、大区折叠、分组聚合 | `agents`, `teams`, `search`, `selectedId` | `select-agent`, `select-team`, `settings`, `delete`, `agent-reorder-end` |
| **ChatSectionHeader** | 可点击的折叠头，带图标+标签+计数+展开箭头 | `icon`, `label`, `count`, `collapsed` | `update:collapsed` |
| **ChatEntityGroup** | 分组折叠 + 组内 vuedraggable 排序 | `items`, `label`, `icon`, `collapsed`, `draggable`, `pinnedId` | `update:collapsed`, `reorder` |
| **ChatEntityItem** | 单行展示：状态图标+名称+状态pill+操作按钮 | `entity`, `active`, `statusIcon`, `statusColor`, `statusLabel` | `click`, `settings`, `delete` |

### 3.3 系统 Agent 置顶约束

三层保障：

1. **初始加载**：`loadAgentOrder()` 将 default Agent 排到首位（已有逻辑）
2. **拖拽约束**：`<draggable :move="onMove">` 回调阻止任何 Agent 拖到 index 0
3. **onEnd 修正**：`onEndAgent()` 若系统 Agent 不在首位则强制归位（已有逻辑）

```typescript
function onMove(evt: MoveEvent): boolean {
  if (props.pinnedId && evt.toIndex === 0) return false;
  return true;
}
```

---

## 4. 数据流与排序逻辑

### 4.1 改动后数据流

```
useAppStore.loadAgents()
  → appStore.agents
    → useChatWorkspace.displayAgents (经 loadAgentOrder 重排，系统 Agent 置顶)
      → ChatEntitySidebar :agents prop
        → filteredAgents = agents.filter(agentMatches)
        → agentGroups = groupEntities(filteredAgents)
        → 每组内: pinned Agent 排首位 + 组内 localStorage 排序
          → ChatEntityGroup :items prop
            → <draggable v-model="localItems">
              → onEnd → emit('reorder', ids)
```

### 4.2 排序持久化策略

| 级别 | localStorage Key | 格式 | 说明 |
|------|-----------------|------|------|
| 全局排序 | `chat:order:agents` | `string[]` (id 列表) | 保持现有，用于初始加载排序 |
| 组内排序 | `chat:order:agents:{groupKey}` | `string[]` (id 列表) | 新增，每个分组独立排序 |

排序逻辑变更：
1. `loadAgentOrder()` 保持不变
2. `ChatEntityGroup` 内部维护 `localItems`，从 props.items 初始化时按组内 localStorage 排序
3. 拖拽 `onEnd` 时：更新 `localItems` → emit `reorder` → 保存到组内 localStorage → 同步更新全局 localStorage

### 4.3 折叠状态管理

新建 `useChatEntityCollapse.ts` composable：

```typescript
const LS_SECTION_COLLAPSED = "chat:collapsed:sections";  // { agents: bool, teams: bool }
const LS_GROUP_COLLAPSED = "chat:collapsed:groups";       // { [groupKey]: bool }

export function useChatEntityCollapse() {
  const sectionCollapsed = reactive({ agents: false, teams: false });
  const groupCollapsed = reactive<Record<string, boolean>>({});

  function restore() { /* 从 localStorage 恢复 */ }
  function save() { /* 保存到 localStorage */ }
  function toggleSection(section: 'agents' | 'teams') { ... }
  function toggleGroup(key: string) { ... }

  return { sectionCollapsed, groupCollapsed, toggleSection, toggleGroup };
}
```

### 4.4 搜索与折叠交互

- 搜索过滤在分组前执行（保持现有逻辑）
- 搜索有内容时：自动展开所有折叠的分组（方便查看结果）
- 搜索清空时：恢复之前的折叠状态
- 大区折叠时：搜索仍然过滤，但整个区域隐藏

---

## 5. 交互细节

### 5.1 折叠交互

| 交互 | 行为 |
|------|------|
| 点击大区头（"Agent" / "Team"） | 折叠/展开整个区域，箭头旋转动画 |
| 点击分组头（"未分类 Agent" 等） | 折叠/展开该分组，箭头旋转动画 |
| 折叠状态 | 分组头始终可见（只隐藏 items），大区头始终可见 |
| 持久化 | 折叠状态实时保存到 localStorage |
| 搜索激活 | 自动展开所有分组（大区不自动展开） |

### 5.2 拖拽交互

| 交互 | 行为 |
|------|------|
| 拖拽触发 | 无拖拽手柄图标，鼠标悬停在 Agent 行上，左键长按 300ms 后进入拖拽模式 |
| 拖拽提示 | 长按后行轻微上浮 + 阴影加深 + 光标变为 grabbing |
| 拖拽范围 | 仅分组内，不可跨分组 |
| 系统 Agent | 不可拖动（disabled），长按无反应 |
| 动画 | `animation: 200`，ghost 样式半透明 |
| 完成后 | 保存排序到 localStorage，emit 事件通知上层 |

### 5.3 视觉规格

```
┌─────────────────────────────┐
│ ▼ Agent                    │  ← 大区头（可折叠）
│   ▼ 系统 Agent      [1]  │  ← 分组头（可折叠）
│     🔵 系统管家  空闲       │  ← 系统 Agent（置顶，不可拖拽）
│     ⚪ 助手A      空闲      │  ← 可拖拽（长按触发）
│     ⚪ 助手B      空闲      │  ← 可拖拽（长按触发）
│   ▶ 技术 / 后端      [3]  │  ← 折叠的分组
│ ▶ Team                     │  ← 折叠的大区
└─────────────────────────────┘
```

---

## 6. 文件变更清单

| 操作 | 文件 | 说明 |
|------|------|------|
| **新建** | `components/chat/ChatSectionHeader.vue` | 可复用折叠头组件 |
| **新建** | `components/chat/ChatEntityGroup.vue` | 分组容器（折叠+draggable） |
| **新建** | `components/chat/ChatEntityItem.vue` | 单行展示组件 |
| **新建** | `features/chat/composables/useChatEntityCollapse.ts` | 折叠状态管理 |
| **修改** | `components/chat/ChatEntitySidebar.vue` | 重构为编排层，消费子组件 |
| **修改** | `features/chat/composables/chatWorkspaceUtils.ts` | 新增组内排序工具函数 |
| **修改** | `features/chat/composables/useChatSidebarOrder.ts` | 适配组内排序持久化 |

**不需要改动的**：
- 后端（无 API 变更）
- Store 层（数据流不变）
- `useChatWorkspace.ts`（接口不变）

---

## 7. 风险与缓解

| 风险 | 缓解措施 |
|------|----------|
| 拆分子组件后 emit 链路变长 | ChatEntitySidebar 统一转发，子组件不跨层通信 |
| vuedraggable delay 模式与 Quasar q-item 点击冲突 | 使用 `delay-on-touch-only=false` + `fallback-on-body=true`；拖拽中阻止 click 事件 |
| 折叠状态与搜索交互复杂 | 搜索激活时展开所有分组，清空时恢复，用 snapshot 模式保存/恢复 |
| 组内排序与全局排序不一致 | 组内排序优先，全局排序作为 fallback；新增 Agent 时追加到组末尾 |
