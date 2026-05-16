# S1 — P0 红线 + 数据正确性 Hardening

> 日期：2026-05-17 | Sprint：S1 | PR：PR1~PR8

## 摘要

清零 AI-DEVELOPMENT-SPECIFICATION 红线违反（V-1~V-5），修复影响数据正确性的高危 Bug，让前端可正常构建。

## PR 清单

### PR1 — 单 SQLite 连接池（T1）

- `internal/data/data.go` 暴露 `RawDB()` 获取底层 `*sql.DB`
- `internal/session/trpc/sqlite.go` 改为接收注入的 `*sql.DB`
- `internal/graph/trpc/checkpoint.go` 改为接收注入的 `*sql.DB`
- Wire 更新：`data.NewTRPCSessionService`、`data.NewGraphCheckpointSaver` 使用 `Data.RawDB()`
- **验收**：`grep -rn "sql.Open" internal/ | grep -v data/data.go` 输出空

### PR2 — WS 接入 Kratos（T2）

- `internal/server/ws.go` 删除独立 `http.Server`，改为 `RegisterTo(httpSrv)` 挂载到 Kratos HTTP
- `internal/server/http.go` 注册 `/v1/ws` 路由
- 进程仅监听 8000（HTTP）+ 9000（gRPC）
- **验收**：`websocat ws://localhost:8000/v1/ws?...` 可订阅事件

### PR3 — biz 去框架依赖（T3 + T4）

- T3：新建 `internal/biz/domain_event.go` 定义 `DomainEvent`，新建 `internal/biz/domain_event_adapter.go` 实现 `DomainEventPublisher`/`DomainEventSubscriber` 适配器，`EventBusConsumer` 改用 `DomainEvent`
- T4：新建 `internal/biz/graph_runtime.go` 定义 `GraphRuntime`/`GraphBuilderFactory` 接口，新建 `internal/adapter/graph/runtime_adapter.go` 实现 trpc 适配器，`GraphUsecase`/`TaskUsecase` 改用接口
- `GraphDefinition` 数据类型保留 `graphtrpc` 具体类型（值对象共享），运行时行为通过接口抽象
- **验收**：`go list -deps aranea-agents/internal/biz/... | rg "trpc-agent-go"` 空输出

### PR4 — Memory cache 修复（T5）

- `internal/memory/trpc/sqlite_adapter.go` 删除 `s.cache` 字段，所有读写操作路由到 SQLite Store
- **验收**：前一 turn 写入的 memory 在后一 turn `ReadMemories` 可见

### PR5 — Graph 并发安全 + GC + panic recover + ctx 修复（T6 + T7 + T9 + T10）

- T6：`BuildStateGraphWithRegistry` 函数内 copy input 参数防止并发修改
- T7：`GraphUsecase` 添加 `executions` map GC（30 分钟过期清理）
- T9：`pkg/safego/safego.go` 统一 panic recovery helper
- T10：`internal/agent/trpc_build.go` `loadEffectiveToolKeys` 修复 `context.Background()` 误用
- **验收**：`go test -race ./internal/graph/...` 通过

### PR6 — EventBus 关键事件可靠投递（T8）

- `internal/event/bus.go` 添加 `reliableTypes`（tool_result/error/runner_completion/graph_node_end/team_run_finished/team_run_failed）
- 可靠事件在订阅者 buffer 满时阻塞最多 100ms 再投递
- **验收**：tool_result 不再被高频 text_delta 顶掉

### PR7 — 前端 graph/chat 客户端（T11 + T12）

- T11：`make api` 生成 `web/src/services/kratos/graph/v1/index.ts`，`services/index.ts` 添加 `createGraphService` 导出
- T12：`web/src/features/chat/api.ts` 改用 `createChatService()` 替代 `kratosApi.post("/v1/chat/...")`
- **验收**：`pnpm build` 通过；`grep -rn "kratosApi.post" web/src/features/chat` 空输出

### PR8 — 文档同步（T13）

- `docs/guides/plan.md` §3.2/§3.3 已确认最新
- `docs/README.md` 和 `AI-DEVELOPMENT-SPECIFICATION.md` 无 SSE/adksvc 残留
- 新增本 changelog
- **验收**：`grep -rn "SSE\|sse.go\|adksvc" docs/README.md docs/guides/AI-DEVELOPMENT-SPECIFICATION.md` 空输出
