# 聊天「关注最新消息」机制重构设计：状态制跟随 + 左边线状态样式

> 日期：2026-07-22
> 状态：已确认（用户逐节评审通过）
> 范围：前端聊天界面（`web/`），团队外部精灵消息流 + 团队内部成员会话两个层级
> 影响文档：`docs/development/1-chat.design.md`（B.2.2 滚动锚点、B.4 折叠/滚动模型、B.5 团队 UI 章节需同步）

---

## 1. 背景与问题

### 1.1 当前实现

两层滚动体系：

- **外层主聊天**：[useChatMessageScroll.ts](../../../web/src/features/chat/composables/useChatMessageScroll.ts)，视口为 SessionPanelV2 容器。
- **内层成员面板**：[useActivityAutoScroll.ts](../../../web/src/features/chat/composables/useActivityAutoScroll.ts)，MemberSessionPanel 内 300px 活动区。

两套逻辑均为**时间制模型**：用户滚离底部 → 10s 冷却 → 强制拽回底部；滚回底部 20px 内立即恢复。

视觉层级：`TeamStagePanel[边框盒] → TeamRunCard[边框盒] → MemberSessionPanel[边框盒] → 300px 滚动区[thinking/action/reply 混装]`，共 4 层容器。

### 1.2 已确认问题（用户逐条确认）

| # | 问题 | 根因 |
|---|------|------|
| P1 | 流式/团队执行期间外层不跟随，新内容在视口外增长 | [ChatMessagePanel.vue](../../../web/src/components/chat/ChatMessagePanel.vue) 调用 `useChatMessageScroll` 时未传 `tasks`，v2 活动流驱动的 watcher 成为死代码 |
| P2 | 阅读历史时被 10s 定时器强制拽回底部，打断阅读/复制 | 时间制恢复模型本身（两套 composable 同病） |
| P3 | 未跟随期间新内容到达无任何感知 | 无新消息指示机制（现有 ↓ 按钮 >200px 才出现且不携带新消息语义） |
| P4 | 距底 20~200px 死区：不显示按钮却触发冷却强拽 | 阈值不统一（NEAR_BOTTOM=20 / SCROLL_BTN=200） |
| P5 | 团队内部消息块不明显、嵌套层级多 | 三层边框盒套娃；成员 reply 与 thinking/action 同权重混装在 300px 滚动区；容器背景淹没了 reply 气泡 |

### 1.3 用户决策记录（讨论结论）

1. 整体方向：**方案 A 状态制重构**（废弃时间制）。
2. **不要 unread 计数**。
3. **不要任何新消息提示 UI**：不加 pill、不加圆点；现有 ↓ 回底按钮一并移除。未跟随时 UI 零打扰，用户自己滚回底部即恢复跟随。
4. 团队内部块优化：**DOM 结构不变**，纯样式重构——学习 Cursor/Trae 的「左边线 + 状态色 + 缩进」模式，去盒子化；边框带状态（执行中/完成/中断等）。

---

## 2. 目标与非目标

### 目标

- G1：跟随中（用户在底部）时，流式/团队执行新内容实时滚底可见（修 P1）。
- G2：用户滚离底部后，**永不**自动滚动，无任何定时器强拽（修 P2、P4）。
- G3：内外两层（主聊天 / 成员面板）共用同一套跟随模型与 composable。
- G4：团队内部层级视觉扁平化：左边线 + 状态色，消除盒子套娃；成员 reply 气泡在无色容器上自然突出（修 P5）。
- G5：用户发新消息（新 Task）时滚到自己消息顶部（落实 1-chat.design.md B.2.2 未实现的锚点要求）。

### 非目标（YAGNI）

- 不做未读计数、未读持久化、跨会话未读。
- 不做新消息 pill / 圆点 / toast / 声音提示（用户明确拒绝）。
- 不做 DOM 结构重组（不做 reply 提级、不做过程折叠条、不做产出聚合墙）。
- 不改动 GraphStageBlock / PlanDAG / 观察视图（ObservationPanel）的样式。
- 不改动后端与 WS 协议。

---

## 3. 设计一：状态制跟随机制

### 3.1 统一 composable `useFollowScroll`

新文件：`web/src/features/chat/composables/useFollowScroll.ts`

**状态机**（二态，无定时器）：

```
FOLLOWING ──用户滚离底部(>80px)──► UNFOLLOWED
    ▲                                  │
    └────── 用户滚回底部(≤80px) ────────┘
```

- `FOLLOWING`：contentSignature 变化 → rAF 节流滚底（复用现有 50ms leading + trailing 节流逻辑）。
- `UNFOLLOWED`：任何自动滚动停止；不累计、不提示；仅等待用户滚回底部。
- 阈值统一为 **80px**（消除 P4 死区）：`distanceFromBottom ≤ 80` 即视为在底部。
- `programmaticScroll` flag 保留：程序滚底不触发状态切换。

**选区保护**：FOLLOWING 中内容更新前检测 `window.getSelection()`，若当前滚动容器内存在非空选区 → 转入 UNFOLLOWED（保护复制操作），用户滚回底部后恢复。

**API**：

```ts
type FollowScrollOpts = {
  scrollEl: Ref<HTMLElement | null>;
  /** 内容变化签名（流式增长也必须改变签名） */
  contentSignature: Ref<string> | ComputedRef<string>;
  /** 是否启用跟随（如 !collapsed && status==='running'）；
   *  false→true 时滚底并进入 FOLLOWING */
  enabled: Ref<boolean> | ComputedRef<boolean>;
};

useFollowScroll(opts) → {
  following: Readonly<Ref<boolean>>;
  onScroll: (e?: Event) => void;   // 绑定容器 @scroll.passive
  jumpToLatest: () => void;        // 显式滚底 + 恢复 FOLLOWING（供"发消息"等场景调用）
};
```

**enabled 语义**：`false → true` 跳变时（面板展开 / agent 启动 / 会话就绪）滚底并进入 FOLLOWING。`true → false` 时仅停止跟随，不动滚动条。

### 3.2 外层主聊天（重写 useChatMessageScroll）

[useChatMessageScroll.ts](../../../web/src/features/chat/composables/useChatMessageScroll.ts) 重写为基于 `useFollowScroll`：

- **contentSignature = 活动树末端签名**（O(1)，不遍历全树）：
  `tasks.length : 末端turn的steps.length : 末端step.ID : 末端step.Status : 末端step.Content.length : teamStages.size : teamRuns.size`
  其中「末端 turn / 末端 step」指 session 活动树按时间序的最后一个 task 的最后一个 turn 及其最后一步——外层视口的跟随语义是"跟随主流尾部"，历史 task/turn 的局部更新（如重新生成）不触发外层跟随。
  数据来源 `activityV2Store`（经 `useActivityQueries`），在 ChatMessagePanel 组装后传入——**修复 P1 的 tasks 接线**。
- `messages.length`（messageStore 持久化消息）变化并入签名，覆盖 turn 边界。
- **新 Task 锚点**（G5）：watch 最新 task ID；新 task 出现时（用户发送新消息即创建 Task，TaskCard 顶部渲染 `task.UserMessage`，根元素带 `[data-task-id]`），`scrollIntoView({ block: 'start' })` 到该 task 元素顶部，并进入 FOLLOWING。这是用户自身操作引导的滚动，不属于打扰。（v2 数据模型中用户消息挂载在 Task 上，Turn 为 agent 执行单元，故锚点为 Task 而非 Turn。）
- **会话切换 / 挂载**：滚底 + FOLLOWING（保留现状 `alignMessageScroll` 的 clamp 防 NaN 逻辑）。
- **删除**：10s RECOVERY 定时器、`showScrollBtn` 及阈值逻辑。
- **保留**：`scrollToTurnId` + `flashTurnHighlight`（Plan 面板定位功能）、`useChatScrollTitle` 的 scroll 事件转发兼容（`onMessagesScroll` 包装链不变）。

### 3.3 内层成员面板（MemberSessionPanel）

- 改用 `useFollowScroll`，删除 [useActivityAutoScroll.ts](../../../web/src/features/chat/composables/useActivityAutoScroll.ts)。
- `enabled = !collapsed && memberSession.Status === 'running'`（不变）。
- `contentSignature` 保持现有定义（`steps.length : lastStep.ID : lastStep.Content.length`）。
- 面板展开（含 autoExpandFor 信号）时滚底跟随（与现行为一致）。

### 3.4 边界规则表

| 场景 | 行为 |
|------|------|
| 跟随中，新内容/流式到达 | rAF 节流滚底 |
| 用户滚离底部读历史 | 停止一切自动滚动，无任何 UI 提示 |
| 跟随中用户选中容器内文字 | 转 UNFOLLOWED（保护复制） |
| 用户手动滚回底部（≤80px） | 恢复 FOLLOWING |
| 用户发送新消息（新 Task） | 滚到新 Task 的 UserMessage 顶部 + FOLLOWING |
| 会话切换 / 首次挂载 | 滚底 + FOLLOWING |
| 成员面板展开 / agent 启动 running | 该面板滚底 + FOLLOWING |
| 成员面板折叠 / agent 终态 | 停止跟随，不动滚动条 |
| WS 重连 replay 批量事件 | 签名去抖 + 节流，跟随行为不变 |

---

## 4. 设计二：左边线状态样式体系

> 设计依据：Cursor（Agent 面板）与 Trae（SOLO/Builder）均采用「左侧竖线 + 状态色 + 缩进」表达层级，零嵌套盒子。**DOM 结构不变，仅改样式。**

### 4.1 层级规则

| 元素 | 样式 |
|------|------|
| 精灵主流 steps（TurnContainer 直接子级） | **无左边线**——无线 = 主流 |
| [TeamStagePanel.vue](../../../web/src/components/chat/v2/TeamStagePanel.vue) | 去掉边框/背景/圆角，纯语义容器（保留 `data-team-stage-id` 与 `activity-locate-highlight` 定位高亮） |
| [TeamRunCard.vue](../../../web/src/components/chat/v2/TeamRunCard.vue) | 去四周边框+背景，改 **3px 左状态线** + 左内边距；hover 才出微弱玻璃背景（表达可点击） |
| [MemberSessionPanel.vue](../../../web/src/components/chat/v2/MemberSessionPanel.vue) | 去四周边框+背景，改 **3px 左状态线** + `margin-left: 14px`（挂在团队线之下，形成视觉树） |

### 4.2 状态色映射（左边线与状态徽章同色呼应）

| 状态 | 左边线 |
|------|--------|
| running | 蓝 / accent，**线体呼吸脉冲动画**（1.6s 循环，承担原"新动态呼吸点"语义，不再加额外圆点） |
| paused | 橙 |
| completed | 绿 |
| failed | 红 |
| cancelled / skipped | 灰 |

pending（成员）使用浅灰。

### 4.3 背景策略

- 上述容器背景全部去除；仅 hover 时出现 `--glass-surface-hover` 微弱背景。
- 容器无色后，成员 ReplyBlock 的气泡背景（`--glass-elevated`）在视觉上自然浮出——"成员说了什么"不靠提级结构也能辨识（修 P5 的另一半）。
- thinking/action 块保持现有折叠单行样式，在左边线体系下视觉更轻。

### 4.4 内外层级辨识

- 有左边线 = 团队子执行树（内部）；无线 = 精灵主流（外部）。用户扫一眼即可区分团队内外消息层级。

---

## 5. 改动清单

| 文件 | 改动 |
|------|------|
| `web/src/features/chat/composables/useFollowScroll.ts` | **新增**：状态制跟随 composable（状态机 + 节流 + 选区保护） |
| `web/src/features/chat/composables/useChatMessageScroll.ts` | **重写**：基于 useFollowScroll；删 10s 定时器与 showScrollBtn；保留 scrollToTurnId/highlight/scroll 转发；新增末端签名 + 新 Task 锚点 |
| `web/src/features/chat/composables/useActivityAutoScroll.ts` | **删除**（ MemberSessionPanel 改用 useFollowScroll） |
| `web/src/components/chat/ChatMessagePanel.vue` | 组装 v2 末端签名传入；移除 showScrollBtn 接线 |
| `web/src/components/chat/ChatMessageList.vue` | 移除 ↓ 回底按钮及 transition；其余不变 |
| `web/src/components/chat/v2/MemberSessionPanel.vue` | 换用 useFollowScroll；左边线样式改造 |
| `web/src/components/chat/v2/TeamRunCard.vue` | 左边线样式改造（含 running 脉冲） |
| `web/src/components/chat/v2/TeamStagePanel.vue` | 去边框/背景 |
| i18n（zh-CN / en-US） | 清理不再引用的 `chat.scrollToLatest` 等 key（先 Grep 确认无其他引用再删） |
| `docs/development/1-chat.design.md` | 同步 B.2.2 滚动锚点、B.4 自动滚动模型（时间制→状态制）、B.5 团队 UI 样式描述（DOC-SYNC） |

依赖方向合规：composable 层只依赖 Vue + store queries，组件层接线，无分层越界。

---

## 6. 错误与边界处理

- `scrollEl` 为 null（空会话 / 未挂载）→ 全部 no-op。
- 内容骤减（历史压缩 / 会话切换残留）→ 保留 `clampScrollTop` 防 NaN / 越界。
- 选区保护误判：选区在容器外（如输入框）不影响跟随；仅当 `getSelection()` 的 anchorNode 位于滚动容器内且非空选区时才暂停。
- 组件卸载：取消防抖定时器 / rAF（`onBeforeUnmount`）。
- `enabled=false` 期间 contentSignature 变化不累积任何状态，`enabled` 恢复时直接滚底对齐最新。

---

## 7. 测试计划

### 单测（vitest）

- `useFollowScroll.spec.ts`（新增）：
  - FOLLOWING 中签名变化 → 滚底（节流后）
  - 用户滚离 >80px → UNFOLLOWED，签名变化不滚
  - UNFOLLOWED 中滚回 ≤80px → 恢复 FOLLOWING
  - 程序滚底（programmaticScroll）不触发状态切换
  - 容器内非空选区 → 暂停跟随；容器外选区 → 不影响
  - enabled false→true → 滚底 + FOLLOWING；true→false → 不滚动
  - 卸载清理定时器/rAF
- `MemberSessionPanel` 组件测试：更新对旧 composable 的 mock 引用。

### 运行时验证（R3，必须）

1. `pnpm dev` 起前端 + admin 后端，实发消息触发精灵流式：跟随中实时滚底。
2. 触发团队执行（多成员）：外层跟随 TeamRunCard 增长；成员面板过程区跟随。
3. 滚离底部阅读历史：流式期间不打扰；滚回底部恢复跟随。
4. 跟随中选中文字复制：不滚动；滚回底部恢复。
5. 目检左边线：running 脉冲 / completed 绿 / failed 红；精灵主流无线；成员面板缩进层级。
6. 读 `logs/aranea-pipeline.log` 确认无异常。

### 提交前全量

`cd web && pnpm lint && pnpm test && pnpm build`。
