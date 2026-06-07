# Event 事件系统 — 开发计划

> **版本**：2026-06-06 | **状态**：🟢 P2 + P3 已实现，2.10 工具生命周期事件未实现
> **需求**：[34 event-system.md](./34%20event-system.md) · **设计**：[34 event-system.design.md](./34%20event-system.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-RT-06 ✅ / EP-FE-02 ✅ / I5-SYS-03 ✅

---

## 1. 模块定位

Event 事件系统：基于事件总线的发布/订阅机制，支持系统内部组件间的异步事件通信。

**代码锚点**：
- `internal/event/*` — Bus / Envelope / Buffer
- `internal/biz/event_bus_*` — Consumer 三 handler + persist handler
- `internal/service/event.go` — `GET /v1/events` 回放
- `web/src/features/chat/useEnvelopeStream.ts` — WS composable
- `web/src/components/monitor/RealtimeEvents.vue` — Monitor Events Tab
- `web/src/components/chat/SessionTimelineDialog.vue` — Chat Trace + Envelope 双 Tab

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| 后端 Event 核心 | ✅ | Bus / Envelope / WS / StateDelta / event_store |
| Monitor 实时事件 UI | ✅ | `RealtimeEvents.vue` |
| Chat 会话事件检视 | ✅ | `SessionTimelineDialog` 双 Tab + Inspector 组件 |
| Monitor EventTimeline 原型 | ✅ | 已删除（O1） |
| 工具生命周期事件 | ❌ | EnvelopeType 无 ToolRegistered/ToolUpdated/ToolRemoved |

---

## 3. 差距与优化

### 3.1 功能差距

1. ~~**P2** 事件持久化 + 回放 API~~ ✅
2. ~~**P3** Chat 会话事件检视（Dialog 双 Tab）~~ ✅
3. **P4** 工具生命周期事件与自动触发（ToolRegistered / ToolUpdated / ToolRemoved）— 见需求 §2.10

### 3.2 优化项

| # | 状态 | 项 |
|---|------|-----|
| O1 | ✅ | 删除未挂载 `monitor/EventTimeline.vue` |
| O4 | ✅ | domain_event_adapter 丢弃 SysLogWarn |
| O5 | ✅ | event_persist_handler 独立 SRP |
| O7 | ✅ | Chat Inspector vs Monitor 分工已定（设计 §12.1） |
| O8 | ✅ | ListEvents 会话存在性校验（SessionUsecase.Get） |
| O9 | ✅ | Inspector 复用 chat/team WS（subscribeSessionStream） |
| O10 | ✅ | 持久化有界队列 + 排除 text_delta/member_delta |
| O11 | ✅ | FilterKey 过滤 UI（EventFilterBar） |
| O12 | ✅ | createEventService + event_test 集成测试 |

---

## 4. 开发阶段

### Phase 1：事件持久化（P2）— ✅ 完成

见 [changelog/2026-05-21-Event-Store-P2.md](../changelog/2026-05-21-Event-Store-P2.md)。

### Phase 2：Chat 会话事件检视（P3）— ✅ 完成

**方案**：扩展 `SessionTimelineDialog` 为双 Tab（Trace | Envelope），**不**新增第四列侧边栏。

**任务**：
1. ~~删除 `web/src/components/monitor/EventTimeline.vue`（O1）~~ ✅
2. ~~`web/src/features/event/api.ts` — `listSessionEvents`~~ ✅
3. ~~`web/src/features/chat/eventFilter.ts` — filterEnvelopes / buildBranchTree~~ ✅
4. ~~`web/src/features/chat/composables/useEventFilter.ts`~~ ✅
5. ~~`web/src/features/chat/composables/useChatEventInspector.ts`~~ ✅
6. ~~`components/chat/` — EventFilterBar / BranchTree / StateDeltaIndicator / TransferBadge / SessionEventInspectorPanel~~ ✅
7. ~~扩展 `SessionTimelineDialog` — q-tabs + `initialTab` prop~~ ✅
8. ~~`ChatMessagePanel` — 「事件」按钮 → 打开 Envelope Tab~~ ✅
9. ~~`useChatWorkspace` — `openSessionTrace(id, tab?)`~~ ✅

### Phase 3：工具生命周期事件与自动触发（P4）— ❌ 未实现

> 来源：BabyAGI Triggers 机制，竞品分析差距 #8。见需求 §2.10。

**任务**：
1. 增加 `ToolRegistered` / `ToolUpdated` / `ToolRemoved` 三种 EnvelopeType
2. `ToolRegistered` 事件触发 LLM 自动生成工具描述和 embedding
3. `ToolUpdated` 事件触发 `BuildTRPCAgentCached` 缓存失效
4. `ToolRemoved` 事件触发依赖该工具的 Agent 配置告警
5. 所有触发操作经 broker/async 异步执行
6. 触发结果记录到 FlowLog

---

## 5. 任务清单

| # | 任务 | Phase | 状态 |
|---|------|-------|------|
| 1–7 | P2 持久化 / API / TTL | 1 | ✅ |
| 8 | 删除 Monitor EventTimeline 原型 | 2 | ✅ |
| 9 | useEventFilter + eventFilter.ts | 2 | ✅ |
| 10 | StateDeltaIndicator / TransferBadge | 2 | ✅ |
| 11 | BranchTree | 2 | ✅ |
| 12 | SessionEventInspectorPanel + Dialog Tab | 2 | ✅ |
| 13 | ChatMessagePanel 入口 | 2 | ✅ |
| 14 | ToolRegistered / ToolUpdated / ToolRemoved EnvelopeType | 3 | ❌ |
| 15 | ToolRegistered → 自动生成描述 + embedding | 3 | ❌ |
| 16 | ToolUpdated → 缓存失效 | 3 | ❌ |
| 17 | ToolRemoved → Agent 配置告警 | 3 | ❌ |
| 18 | 异步触发 + FlowLog 记录 | 3 | ❌ |

---

## 6. 验收标准

- [x] 系统重启后可查询历史事件
- [x] 事件回放 API 分页
- [x] TTL 清理
- [x] Monitor Events Tab
- [x] Chat Dialog Envelope Tab：类型/分支/标签过滤
- [x] Branch 树可视化
- [x] StateDelta 指示器
- [ ] 新工具注册后自动生成描述和 embedding
- [ ] 工具更新后相关 Agent 缓存自动失效
- [ ] 触发操作异步执行，不阻塞主流程
- [ ] 触发结果在 FlowLog 中可追踪

---

## 7. 依赖与风险

- Chat Inspector 依赖 P2 `GET /v1/events` 与 WS 同时在线
- 高流量 session 下 Envelope 列表需上限（Inspector 保留最近 N 条）
- FlowLogger 落库与 event_store 已分流（exclude flow_log）
