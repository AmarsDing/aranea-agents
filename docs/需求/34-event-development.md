# Event 事件系统 — 开发计划

> **版本**：2026-05-19 | **状态**：🟡 核心已实现，持久化与可视化待开发
> **需求**：[34 event-system.md](./34%20event-system.md) · **设计**：[34 event-system.design.md](./34%20event-system.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-RT-06 ✅ / EP-FE-02 ✅

---

## 1. 模块定位

Event 事件系统：基于事件总线的发布/订阅机制，支持系统内部组件间的异步事件通信。

**代码锚点**：
- `internal/event/bus.go` — Bus（发布/订阅 + 背压策略）
- `internal/event/envelope.go` — Envelope（统一事件信封）
- `internal/event/buffer.go` — Buffer（环形缓冲 + 断连重放）
- `internal/agent/event_projector.go` — EventProjector（trpc Event → Envelope）
- `internal/biz/event_bus_consumer.go` — EventBusConsumer（StateDelta 应用 + Usage 统计）
- `internal/server/ws.go` — WSServer（WebSocket 统一网关）
- `web/src/features/chat/useEnvelopeStream.ts` — useEnvelopeStream（前端 composable）

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| EventBus 发布/订阅 | ✅ | `internal/event/bus.go` — Bus 接口 + 三种背压策略 |
| Envelope 统一信封 | ✅ | `internal/event/envelope.go` — 完整元数据 + Clone / MatchFilterKey / ContainsTag |
| 事件投影 | ✅ | `internal/agent/event_projector.go` — trpc Event → Envelope |
| StateDelta 自动应用 | ✅ | `internal/biz/event_bus_consumer.go` + `session_usecase.go` ApplyStateDelta |
| FilterKey 层级过滤 | ✅ | `envelope.go` MatchFilterKey + `ws.go` filter_key 参数 |
| Branch 追踪 | ✅ | `envelope.go` Branch 字段 + InvocationID / ParentInvocationID |
| Tag 业务标签 | ✅ | `envelope.go` ContainsTag 逗号分隔匹配 |
| Extensions 扩展元数据 | ✅ | `envelope.go` map[string]string |
| Actions 流控制 | ✅ | `envelope.go` EnvelopeActions.SkipSummarization |
| LongRunningToolIDs | ✅ | `event_projector.go` 映射为 ToolCall.IsLongRunning |
| 事件缓冲与重放 | ✅ | `internal/event/buffer.go` 环形缓冲 + TTL + Replay |
| WebSocket 统一传输 | ✅ | `internal/server/ws.go` Channel 路由 + 可靠订阅 + 断连重放 |
| 前端 Envelope 类型 | ✅ | `web/src/features/chat/envelope.ts` |
| 前端 WsTransport | ✅ | `web/src/features/chat/ws-transport.ts` 自动重连 + 心跳 |
| 前端 useEnvelopeStream | ✅ | `web/src/features/chat/useEnvelopeStream.ts` composable |
| DomainEvent 适配 | ✅ | `internal/biz/domain_event.go` + `domain_event_adapter.go` |
| Flow Log v2（`flow_log`） | ✅ | `trace_emitter.go` + `system_flow.go`；SlogBridge 已删除 |
| 系统域基础设施打点 | ✅ | `SysLog*` / `SessionSysLog*`；Bus drop → `system.bus.drop` |
| Graph 事件桥接 | ✅ | `internal/graph/trpc/event_bridge.go` |
| Metrics 指标 | ✅ | `internal/metrics/vars.go` EventBusPublished / EventBusDropped |
| 事件持久化 | ❌ | 无事件存储，系统重启后事件丢失 |
| 事件回放 API | ❌ | 无按时间范围查询/回放历史事件的 API |
| 前端事件可视化 | ❌ | 无 EventTimeline / BranchTree / StateDeltaIndicator 组件 |

---

## 3. 差距与优化

1. **P2**：无事件持久化，系统重启后事件丢失。环形缓冲仅保留最近 200 条/Session，无法查询历史。
2. **P2**：无事件回放 API，无法按时间范围检索历史事件。
3. **P3**：无前端事件可视化组件，无法直观查看事件流、分支追踪、状态变更。

---

## 4. 开发阶段

### Phase 1：事件持久化（P2）

将事件写入 SQLite 持久化存储，支持历史查询。

**任务**：
1. 新建 `internal/data/ent/schema/event_store.go` — Ent Schema
2. 运行 `go generate ./internal/data/ent`
3. 新建 `internal/data/event_store_repo.go` — 事件写入 + 查询
4. 新建 `internal/biz/event_store.go` — EventStoreUsecase + Repo 接口
5. 修改 `internal/biz/biz.go` — Wire ProviderSet
6. 修改 `internal/biz/event_bus_consumer.go` — 消费时写入 EventStore
7. 新建 `api/kratos/event/v1/event.proto` — 事件回放 API Proto
8. 运行 `make api`
9. 新建 `internal/service/event.go` — 事件回放 Service
10. 修改 `internal/server/http.go` — 注册 Service
11. 修改 Wire 注入
12. 新增 TTL 清理后台任务
13. 验证：系统重启后可查询历史事件

### Phase 2：前端事件可视化（P3）

在 Chat 页面侧边栏展示事件时间线。

**任务**：
1. 新建 `web/src/features/chat/composables/useEventFilter.ts` — 事件过滤
2. 新建 `web/src/features/chat/components/StateDeltaIndicator.vue` — 状态变更指示器
3. 新建 `web/src/features/chat/components/TransferBadge.vue` — Agent 转移标签
4. 新建 `web/src/features/chat/components/BranchTree.vue` — 分支追踪树
5. 新建 `web/src/features/chat/components/EventTimeline.vue` — 事件时间线
6. 修改 ChatWorkspace — 集成 EventTimeline 侧边栏
7. 验证：前端可按类型/分支/标签过滤事件流

---

## 5. 任务清单

| # | 任务 | 优先级 | Phase | 状态 |
|---|------|--------|-------|------|
| 1 | `event_store` Ent Schema + 生成 | P2 | 1 | ❌ |
| 2 | EventStoreRepo 实现（写入 + 查询） | P2 | 1 | ❌ |
| 3 | EventStoreUsecase + Repo 接口 | P2 | 1 | ❌ |
| 4 | EventBusConsumer 集成写入 | P2 | 1 | ❌ |
| 5 | 事件回放 API Proto + Service | P2 | 1 | ❌ |
| 6 | Wire 注入 + Server 注册 | P2 | 1 | ❌ |
| 7 | TTL 清理后台任务 | P2 | 1 | ❌ |
| 8 | useEventFilter composable | P3 | 2 | ❌ |
| 9 | StateDeltaIndicator 组件 | P3 | 2 | ❌ |
| 10 | TransferBadge 组件 | P3 | 2 | ❌ |
| 11 | BranchTree 组件 | P3 | 2 | ❌ |
| 12 | EventTimeline 组件 | P3 | 2 | ❌ |
| 13 | ChatWorkspace 集成 | P3 | 2 | ❌ |

---

## 6. 验收标准

- [ ] 系统重启后可查询历史事件（按 session_id / 时间范围 / 事件类型）
- [ ] 事件回放 API 返回分页结果
- [ ] TTL 清理任务正常运行
- [ ] 前端事件时间线可按类型/分支/标签过滤
- [ ] 分支追踪树可视化多 Agent 执行链
- [ ] 状态变更指示器展示 StateDelta 操作

---

## 7. 依赖与风险

- 事件持久化需注意存储膨胀（需加 TTL 清理，建议默认 7 天）
- EventStore 写入为异步操作，需评估对 EventBusConsumer 吞吐量的影响
- 前端可视化依赖现有 useEnvelopeStream，需确保事件类型完整覆盖
