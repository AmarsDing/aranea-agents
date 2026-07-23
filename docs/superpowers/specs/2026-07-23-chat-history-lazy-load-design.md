# 聊天历史懒加载设计（Lazy Hydration of Chat Session History）

> 日期：2026-07-23 · 状态：已评审（用户确认方向 + 展示效果，含 2 条修正）
> 交互 mockup：[2026-07-23-chat-history-lazy-load/mockup.html](./2026-07-23-chat-history-lazy-load/mockup.html)（v2，已含修正）

## 1. 背景与问题

打开会话 / WS 重连时，`activityV2Store.fetchSessionHistory` 一次性全量水合整棵 v2 实体树：

```
tasks(全部) + steps(全部根 session)
  └─ 每 task 并行: turns + team_stages + plan_boards + plan_steps + graph_stages
      └─ 每 team_stage: team_runs → 每 team_run: member_sessions
      └─ 每 graph_stage: graph_nodes
```

长会话（几十\~几百 task、几千 step）下三层卡顿：

| 层  | 瓶颈                                                               | 现状位置                                           |
| -- | ---------------------------------------------------------------- | ---------------------------------------------- |
| 协议 | List RPC 无分页参数（仅 id 过滤）                                          | `api/kratos/session/v1/session.proto` L671-692 |
| 网络 | N+1 请求风暴：`2 + 5×N_tasks + N_stages + N_runs + N_graph` 个 HTTP 请求 | `activityV2Store.ts` fetchSessionHistory       |
| 渲染 | 全部 task 的活动树一次性挂载 DOM，无懒渲染/虚拟滚动                                  | `TaskList.vue` / `TaskCard.vue`                |

## 2. 目标与验收标准

**目标**：打开长会话立即可用（秒级内可交互），默认定位最新消息，历史执行过程按需加载。

**验收标准**：

1. 打开 200 task 会话：首屏 HTTP 请求数 = `2 + 5×N_active`（N\_active = 非终态 task 数，通常 ≤2），DOM 仅含折叠卡 + 已水合 task
2. 打开后直接停在消息底部，无白屏等待
3. 向上滚动：折叠卡进入视口停留 500ms 自动水合；点击卡片立即水合；视口不跳动（滚动锚定）
4. 运行中 task 的 WS 实时事件渲染不受懒加载影响
5. `go build ./... && go test ./...` 全过；`cd web && pnpm lint && pnpm test && pnpm build` 全过

## 3. 产品行为定义（用户确认）

| #  | 行为              | 说明                                                                                                      |
| -- | --------------- | ------------------------------------------------------------------------------------------------------- |
| P1 | 用户指令全部加载        | 全部 task 一次拉回（轻量），**指令显示方式与现状完全一致**（时间+头像+名称+全文+复制/重新生成按钮），不重新设计、不加 clamp                                |
| P2 | 执行过程只加载最后一轮     | 首屏只水合「最后一个 task + 所有非终态 task」的完整子树                                                                      |
| P3 | 历史轮次懒加载         | 折叠卡 = 现有指令面板 + 下方一行 slim 状态摘要（状态徽章+耗时；**不含步数/错误摘要**——折叠态 steps 未加载、Task 实体无此字段，2026-07-23 用户决策不加后端聚合字段）；**无「执行过程」按钮，点击卡片任意位置水合**（复制/重新生成按钮不触发）；滚入视口 500ms 自动水合 |
| P4 | 默认停在最后          | 折叠卡瞬时渲染后直接定位底部；视口上方内容展开时 scrollTop 锚定                                                                   |
| P5 | 非终态 task 永远自动水合 | running / pending / awaiting\_confirmation / interrupted（中断卡需「继续执行」按钮，必须可见）                             |
| P6 | 可重新折叠           | 水合后底部「收起执行过程」可折叠释放 DOM；数据保留内存，再展开不重新请求                                                                  |

## 4. 架构设计

### 4.1 后端（唯一协议改动：ListSteps 分页）

**proto**（`api/kratos/session/v1/session.proto`）：

```protobuf
message ListStepsV2Request {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
  string turn_id = 2;
  string task_id = 3;
  int32 limit = 4;        // 0 = 不分页（现状语义，全量）
  int64 before_seq = 5;   // 0 = 最新窗口；>0 = 取 seq < before_seq 的上一页
}
message ListStepsV2Response {
  repeated StepV2 steps = 1;  // 始终按 seq 升序返回
  bool has_more = 2;          // limit>0 时有效：是否还有更早的 steps
}
```

* **语义**：`limit=0` 保持现状（全量，向后兼容）；`limit>0 && before_seq=0` 返回最新 limit 条；`before_seq>0` 返回 cursor 之前的一页

* **Repo**：`StepReader` 增加分页变体（options struct：`{Limit, BeforeSeq}`），SQL `ORDER BY seq DESC LIMIT n+1` 判定 has\_more 后反转为升序

* **索引**：实现时确认 `steps_v2` 存在 `(session_id, seq)` / `(task_id, seq)` 索引，缺失则补 DDL 迁移

* tasks / turns / team\_stages / plan / graph 等 RPC **不改**（task 轻量全量；其余本就是 per-task 拉取，天然适配懒加载）

### 4.2 Store（`activityV2Store`）

**新增 state**：

```typescript
hydratedTaskIds: Ref<Set<string>>          // 已水合 task（数据已加载）
taskHydration: Ref<Map<string, 'loading' | 'error'>>  // 水合中/失败态
```

* `hydratedTaskIds` **跨 WS 重连持久**：`fetchSessionHistory` 重建数据但不清空该集合，用户已展开的卡片不因重连重新折叠

**改造** **`fetchSessionHistory(sessionId)`** **为分阶段**：

```
Phase 1: listTasksV2(sessionId)
       + listStepsV2(sessionId, { limit: 100 })        // 最近窗口，覆盖 spirit 级散 steps
Phase 2: 计算 autoHydrate = 最后一个 task
       + 状态 ∈ {running, pending, awaiting_confirmation, interrupted} 的 task
       + hydratedTaskIds 中已有的本会话 task（重连场景保持展开）
Phase 3: 对 autoHydrate 集合逐 task 调 hydrateTask(taskId)（并行）
```

**新增 action** **`hydrateTask(taskId)`**（幂等：已水合/水合中直接返回）：

```
taskHydration.set(taskId, 'loading')
并行: listTurnsV2(taskId)
     + listStepsV2(sessionId, { taskId })        // limit=0 全量：单 task 有界
     + listTeamStagesV2 / listPlanBoardsV2 / listPlanStepsV2 / listGraphStagesV2
→ team_stages 下钻 team_runs → member_sessions；graph_stages 下钻 graph_nodes
→ 全部 upsert（与 WS 事件同路径，天然去重合并）
成功: hydratedTaskIds.add(taskId)，taskHydration.delete(taskId)
失败: taskHydration.set(taskId, 'error')
```

**WS 兼容性**：事件 handler 无条件 upsert 进 Map，与 task 是否水合无关；会话进行中新建的 task 创建时即加入 `hydratedTaskIds`（活跃任务默认展开）。

### 4.3 Composable（新增 `features/chat/composables/useLazyTaskHydration.ts`）

为 TaskList 提供懒加载编排（不含网络请求，请求走 store action）：

* **IntersectionObserver**：root = 消息滚动容器，threshold 0.4；折叠卡进入视口启动 500ms dwell 定时器 → 调 `store.hydrateTask`；离开视口取消定时器（快速滑过不触发）

* **手动触发**：`onTaskCardClick(taskId)` → 立即 `hydrateTask`

* **滚动锚定**：水合完成渲染后（nextTick），若该卡片原位置在视口上方，按高度差补偿 scrollTop

* **初始定位**：`fetchSessionHistory` 完成 + 已水合卡渲染后，scrollToBottom（instant）

### 4.4 组件（展示层，props/emits，不碰 store）

**`TaskCard.vue`**：新增 props `hydrated: boolean`、`hydrationState?: 'loading' | 'error'`；新增 emits `expand`、`collapse`。

* **折叠态**（`!hydrated`）：现有用户面板原样渲染 + 下方新增 `task-meta-bar`（状态徽章 + ⏱耗时，`color-mix` 状态色，日夜 token）；整卡 `cursor: pointer`，hover 时 body 玻璃提亮 + 边框向 accent 过渡；点击 emit `expand`（复制/重新生成按钮 `@click.stop` 不触发）

* **水合中**：用户面板 + 3 条 shimmer 骨架（thinking/action/reply，宽度 62%/38%/81%）

* **失败态**：meta-bar 显示「加载失败，点击重试」（danger 色），点击重新 emit `expand`

* **水合态**：现状完整渲染 + 底部「收起执行过程 ▴」文字按钮 emit `collapse`

**`TaskList.vue`**：接入 `useLazyTaskHydration`，向 TaskCard 传 hydrated/hydrationState，转发 expand/collapse。

**折叠语义**：collapse 仅是 UI 状态（component 本地或 composable 持有），不清除 store 数据、不从 `hydratedTaskIds` 移除——再展开零请求。

### 4.5 数据流（合规：红线 #1/#2）

```
v2Api.ts（新增 limit/beforeSeq 参数透传）
  → activityV2Store.fetchSessionHistory / hydrateTask（唯一发请求处）
    → useLazyTaskHydration（observer + dwell + 锚定，调 store action）
      → TaskList → TaskCard（props: task/hydrated/hydrationState；emits: expand/collapse）
```

## 5. 边界情况

| 场景                      | 处理                                                                                                                        |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| WS 重连                   | `hydratedTaskIds` 不清空，重连后已展开卡片保持展开；数据 upsert 刷新                                                                           |
| 会话中新建 task              | 创建即加入 `hydratedTaskIds`（活跃任务永远展开）                                                                                         |
| interrupted task        | 属非终态，自动水合（「继续执行」按钮必须直接可见）                                                                                                 |
| 无 step 的 task（如纯澄清）     | 折叠卡正常显示状态徽章；水合后内容区即澄清卡                                                                                                    |
| 水合失败                    | meta-bar「加载失败，点击重试」；点击重试                                                                                                  |
| 单 task 超大（数千 steps）     | 协议分页已就位；UI 级「加载更多」本期不做（YAGNI），实测出现再加                                                                                      |
| spirit 级无 task 归属 steps | 由 Phase 1 最近窗口（limit=100）覆盖。实施时先验证此类 steps 是否真实存在（现有 orphan steps 均有 TaskID）；仅当存在且窗口不够时，才实现 has\_more + 触顶补拉，否则不实现（YAGNI） |

## 6. 错误处理

* Phase 1 失败：沿用现有 `hydrationErrors` 机制，UI 顶部错误条 + 重试

* `hydrateTask` 单 task 失败：`taskHydration[taskId]='error'`，不影响其他 task

* 后端 repo 错误统一经 `entErrToBizErr` 翻译（红线 DB-R5）

## 7. 测试策略

| 层             | 测试                                                                                                                              |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| 后端 repo       | 分页语义：limit / before\_seq / has\_more / 升序返回；limit=0 向后兼容                                                                        |
| 后端 service    | ListSteps 参数校验与透传                                                                                                               |
| Store 单测      | fetchSessionHistory 分阶段（断言 listStepsV2 带 limit；per-task API 仅对 autoHydrate 集合调用）；hydrateTask 幂等；失败置 error；hydratedTaskIds 重连不清空 |
| Composable 单测 | mock IntersectionObserver：dwell 500ms 触发、离开取消、点击立即触发                                                                            |
| 组件测试          | TaskCard 折叠态只渲染 meta-bar（零 step DOM）；点击 emit expand；操作按钮不冒泡；收起 emit collapse                                                    |
| 验证命令          | `make api && go build ./... && go test ./internal/...`；`cd web && pnpm lint && pnpm test && pnpm build`                         |

## 8. 不在范围（YAGNI）

* task 卡片列表虚拟滚动（折叠卡足够轻，几百张无压力；实测需要再加）

* 单 task 内 steps 虚拟滚动 / UI 分页（协议已备）

* 离屏已水合卡片自动重新折叠（手动收起已释放主要压力）

* member session steps 加载策略（已有懒加载 A.4.7，不动）

* tasks/turns 等 RPC 分页（task 轻量全量拉取保留）

## 9. 文档同步（DOC-SYNC）

实施时同步更新 `docs/development/1-chat` 三件套：

* `1-chat.md`：追加需求「长会话历史懒加载」（P1-P6 用户视角行为）

* `1-chat.design.md`：ListSteps 分页契约、分阶段水合流程、TaskCard 状态机、useLazyTaskHydration 设计

* `1-chat.development.md`：任务清单与状态、代码锚点（proto / activityV2Store / useLazyTaskHydration / TaskCard / TaskList）

