# Session — 开发计划

> **版本**：2026-05-18 | **状态**：✅ 端到端可用
> **需求**：[10 session.md](./10%20session.md) · **设计**：[10 session.design.md](./10%20session.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Session 管理：管理用户与 Agent/Team 的对话会话，包括会话创建、列表、删除、标题更新、上下文压缩、轮次记录、状态管理等。

**代码锚点**：
- `api/kratos/session/v1/` — Session CRUD RPC
- `internal/service/session.go` — SessionService
- `internal/service/session_compress.go` — 上下文压缩
- `internal/biz/session_usecase.go` — SessionUsecase
- `internal/biz/session_state.go` — Session State KV
- `internal/biz/session_title.go` — SessionTitleGenerator
- `internal/data/session_repo.go` — SessionRepo
- `internal/data/session_repo_summaries.go` — SessionSummaryRepo
- `internal/data/session_turn_repo.go` — SessionTurnRepo

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Session CRUD | ✅ | Create/Get/Update/Delete/Search |
| Session 归档/恢复 | ✅ | ArchiveSession/RestoreSession RPC |
| Session 部分更新 | ✅ | UpdateSession（title/tags/visibility/metadata/dialog_mode/provider/model） |
| 消息列表 | ✅ | `ListMessages` RPC + limit/offset 分页 |
| Timeline 聚合 | ✅ | `GetSessionTimeline` + kind_filter/sort_order/limit/offset |
| 上下文压缩 | ✅ | `SessionCompressor.AfterNativeTurn` |
| 标题生成 | ✅ | `LLMSessionTitleGenerator` + 截取双策略 |
| Session Turns | ✅ | CreateTurn/UpdateTurn/ListTurns |
| Session State KV | ✅ | GetSessionState/SaveSessionState/ApplyStateDelta |
| Runner Snapshot | ✅ | UpdateRunnerSnapshotJSON |
| Session Summaries | ✅ | InsertSessionSummary/ListSessionSummaries |
| Session 类型 | ✅ | agent / team |

---

## 3. 差距与优化

### 3.1 代码优化

1. **P1**：`session_repo_summaries.go` 使用 `errors.New` 而非 `kerrors`，需替换为 `kerrors.BadRequest`/`kerrors.InternalServer`，对齐开发规范 §10 原则 10。
2. **P2**：Timeline 聚合全量加载 messages + tools + skills 再内存排序分页，大量数据时性能差；应改为 DB 层分页。
3. **P2**：`ListToolInvocationsBySession` 硬编码 `limit=100`，应支持调用方传入 limit。
4. **P2**：压缩防抖策略 `sessionCompressMinGap = 10min` 硬编码，应从 Agent Settings 读取。
5. **P3**：`AppendChatTurn` 事务内两次查询（`maxMessageTurnTx` + session 查询），可合并为一次。
6. **P3**：SessionCompressor 压缩模型选择 fallback 逻辑分散，应统一为策略模式。

### 3.2 功能差距

1. **P2**：Session 无"置顶"功能，用户无法固定重要会话。
2. **P2**：session_runs / session_run_steps 编排记录未实现，无法追踪完整 Run 生命周期。
3. **P2**：session_participants 未实现，Team Session 缺少参与者角色与贡献指标。
4. **P3**：Session 无"导出"功能，用户无法导出对话记录。
5. **P3**：Session 消息无"搜索"功能，用户无法在历史消息中搜索关键词。
6. **P3**：session_trace_spans 未实现，缺少完整追踪链路。
7. **P3**：session_context_snapshots 未实现，缺少 Context ratio 趋势数据。
8. **P3**：session_model_summaries 未实现，缺少多模型分布汇总。
9. **P3**：trpc session.Service 适配器未实现，无法桥接 trpc-agent-go 框架。
10. **P4**：多后端支持（Redis/PG）未实现。
11. **P4**：Session Ingestor 未实现，无法自动摄入外部记忆平台。

---

## 4. 开发阶段

### Phase 1（近期优化）
- O1: `session_repo_summaries.go` 错误处理统一（kerrors）
- O5: 压缩防抖策略可配置化
- F1: Session 置顶功能（pinned_at 字段 + PinSession RPC + 前端置顶分组）

### Phase 2（编排增强）
- F4: session_runs 编排记录
- F5: session_run_steps 步骤记录
- F6: session_participants Team 参与者
- F17: 前端 Team Session 专属展示（Participants Panel / Handoff Badge）

### Phase 3（可观测性）
- F7: session_trace_spans 完整追踪链路
- F8: session_context_snapshots Context 趋势
- F9: session_model_summaries 多模型分布
- F15: 前端 Trace 链路页（树形/瀑布视图）
- F16: 前端 Context 趋势线

### Phase 4（导出与搜索）
- F2: Session 导出功能（Markdown/JSON）
- F3: 消息搜索功能（全文检索）

### Phase 5（框架对齐）
- F10: trpc session.Service 适配器
- F11: Event 分页
- F12: Session Track
- F13: Session Ingestor
- F14: 多后端支持（Redis/PG/MySQL/ClickHouse）

---

## 5. 任务清单

| # | 任务 | 优先级 | 阶段 |
|---|------|--------|------|
| O1 | `session_repo_summaries.go` 错误处理统一 | P1 | Phase 1 |
| O5 | 压缩防抖策略可配置化 | P2 | Phase 1 |
| 1 | Session 表增加 `pinned_at` 字段 + PinSession/UnpinSession RPC | P2 | Phase 1 |
| 2 | session_runs 表 + CreateRun/UpdateRun/ListRuns | P2 | Phase 2 |
| 3 | session_run_steps 表 + CreateStep/UpdateStep/ListSteps | P2 | Phase 2 |
| 4 | session_participants 表 + CRUD + 贡献指标 | P2 | Phase 2 |
| 5 | 前端 Team Session Participants Panel | P2 | Phase 2 |
| 6 | session_trace_spans 表 + Trace API | P3 | Phase 3 |
| 7 | session_context_snapshots 表 + 快照 API | P3 | Phase 3 |
| 8 | session_model_summaries 表 + 模型分布 API | P3 | Phase 3 |
| 9 | 前端 Trace 链路页 | P3 | Phase 3 |
| 10 | 前端 Context 趋势线 | P3 | Phase 3 |
| 11 | ExportSession RPC（Markdown/JSON） | P3 | Phase 4 |
| 12 | SearchMessages RPC（全文检索） | P3 | Phase 4 |
| 13 | trpc session.Service 适配器 | P3 | Phase 5 |
| 14 | Event 分页 + Session Track | P3 | Phase 5 |
| 15 | Session Ingestor | P4 | Phase 5 |
| 16 | 多后端支持（Redis/PG） | P4 | Phase 5 |

---

## 6. 验收标准

### Phase 1
- [ ] `session_repo_summaries.go` 全部使用 kerrors
- [ ] 压缩防抖间隔可从 Agent Settings 读取
- [ ] 用户可置顶/取消置顶会话
- [ ] 置顶会话在列表中优先显示

### Phase 2
- [ ] 可查看 Session 的 Run 列表和步骤详情
- [ ] Team Session 可查看参与者列表和贡献指标
- [ ] 前端展示 Team Session 参与者面板

### Phase 3
- [ ] 可查看 Session 的完整追踪链路
- [ ] 可查看 Context ratio 趋势图
- [ ] 可查看模型分布统计

### Phase 4
- [ ] 可导出会话为 Markdown/JSON
- [ ] 可在历史消息中搜索关键词

### Phase 5
- [ ] trpc session.Service 接口可用
- [ ] 支持 Redis/PG 后端

---

## 7. 依赖与风险

- 全文检索需评估 SQLite FTS5 或引入搜索引擎
- session_runs / session_run_steps 需与 ChatService/TeamRunner 的生命周期对齐
- trpc session.Service 适配器需与 trpc-agent-go 框架版本同步
- 多后端支持需抽象 Repository 接口，确保 Ent 方案可替换
