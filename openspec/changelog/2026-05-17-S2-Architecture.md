# S2 架构债清理 — 变更记录（2026-05-17）

> Sprint 2 完成了 7 项架构债任务（T14~T20），全部合并至主干。

---

## PR9 — T14：`internal/runtime` 抽象（取代 runtimedeps）

**目标**：拆解 `runtimedeps.TurnDeps` "上帝对象"，迁移到专用 `internal/runtime` 包。

**变更**：
- 新建 `internal/runtime/deps.go`，定义四象限聚合类型：
  - `Catalog`（biz 仓储/用例）
  - `PersistenceSet`（trpc session + SQLite memory + AgentMCP）
  - `EventPipeline`（EventBus）
  - `TurnDeps`（整合上述三组 + Sessions/LLMHTTP/Compress）
  - `NewPersistenceSet` 构造函数（Wire 用）
- 更新 `internal/runtimedeps/deps.go` 为 deprecated 别名层（`type Runtime = rt.PersistenceSet`，`type TurnDeps = rt.TurnDeps`）
- 迁移所有调用点：
  - `internal/service/chat.go`：`ChatServiceDeps.RT *Runtime` → `Persist rt.PersistenceSet`
  - `internal/service/trpc_turn.go`：字段访问 `s.td.LLMCatalog` → `s.td.Catalog.LLM` 等
  - `internal/service/session_compress.go`：`c.RT *Runtime` → `c.Persist rt.PersistenceSet`
  - `internal/team/runner.go`、`runner_team_trpc.go`、`runner_helpers.go`：同步字段访问
  - `cmd/admin/wire.go`：`runtimedeps.NewRuntime` → `rt.NewPersistenceSet`
- 顺带修复 `internal/service/graph.go:718` 遗留 T4 类型不匹配问题（graphtrpc → biz StateFieldDef 转换）

**验收**：
- `grep -rn "runtimedeps" internal/` 仅命中 `runtimedeps/deps.go`（deprecated alias 文件本身）
- `go vet ./internal/runtime/... ./internal/service/... ./internal/team/... ./internal/runtimedeps/...` 通过

---

## PR10 — T15：EventBus 完整背压

**目标**：以正式 API 替代 S1 T8 引入的 `reliableTypes` 硬编码。

**变更**：
- 新增 `DropPolicy` 枚举：`DropOldest`、`DropNewest`、`BlockUpTo`
- 扩展 `SubscribeOptions` 新增字段：`Reliable bool`、`DropPolicy DropPolicy`、`BlockFor time.Duration`、`Selector func(EnvelopeType) bool`
- 投递逻辑按 policy 分派到三个独立方法：`deliverBlockUpTo`、`deliverDropOldest`、`deliverDropNewest`
- 删除 `reliableTypes()` 硬编码，改由 `criticalTypes()` + 订阅者 `Reliable` 标志共同决定投递策略
- 新增测试文件 `internal/event/bus_backpressure_test.go`，覆盖 5 个用例（DropOldest/DropNewest/BlockUpTo/Reliable/Selector）

**验收**：
- `go test ./internal/event/... -v -count=1` 全部通过

---

## PR11 — T16：Agent 构建缓存

**目标**：消除每次 chat turn 重复装配 LLMAgent 的开销（key=agent+tools+skills+model 指纹）。

**变更**：
- 新建 `internal/agent/cache.go`：
  - `BuildCache`：带 TTL（10min）的 LRU 实现（`container/list` 内联，无额外依赖）
  - `BuildCacheKey(ag, deps)`：sha256 指纹（agentID + UpdatedAt + ConfigJSON + Settings + Provider + Model + DialogMode）
  - `InvalidateAgentCache(agentID)`：agent 更新时逐出同 agentID 全部 slot
  - `BuildTRPCLLMAgentCached`：全局缓存包装器
- `internal/service/trpc_turn.go`：`BuildTRPCLLMAgent` → `BuildTRPCLLMAgentCached`
- `internal/team/trpc_build.go`：`TRPCTeamBuilderDeps.UseCache bool` 开关；team runner 默认开启

**验收**：`go vet ./internal/agent/... ./internal/team/...` 通过

---

## PR12 — T17 + T18：前端 Pinia store + axios/WS 升级

### T17：Pinia store 补齐（17 个业务域）

新建 `web/src/stores/<域>/index.ts`：
`admin` / `channels` / `chat` / `cron` / `graph` / `heartbeat` / `mcp` / `memory` / `monitor` / `platform` / `plugins` / `session` / `skills` / `system-settings` / `teams` / `tools` / `usage`

每个 store：
- `state`（`ref`）+ `actions` 调用 `features/<域>/api.ts`
- 从 `web/src/stores/index.ts` 统一 re-export

### T18：axios 拦截器 + 统一 WS 客户端

**`web/src/services/axiosHandler.ts`**：
- 新增 response interceptor：
  - 401 → 跳转 `/login?redirect=...`
  - 429 → `Notify.create` 弹出退避提示
  - 5xx → `Notify.create` 负面通知
  - 4xx（非 401/404）→ `Notify.create` 警告通知
  - 提取 Kratos 错误 envelope `message` 字段作为提示文案

**`web/src/services/wsClient.ts`**（新建）：
- `WsClient` 单例：指数退避重连（base 1s，max 30s）
- `connect(sessionId)`、`disconnect()`
- `subscribe(channel, handler)` → unsubscribe 函数
- `subscribeAll(handler)` → unsubscribe 函数

**`web/src/composables/useWS.ts`**（新建）：
- `useWS(channel?, onMessage?)` composable
- 自动 mount/unmount 订阅
- 2s 轮询同步 `isConnected` / `reconnectCount`
- 断连时弹出 `Notify` 提示

**验收**：`npx vite build` 通过（0 错误）

---

## PR13 — T19 + T20：命名清理与冗余删除

**T19 验收**：`grep -rn "errros\|_adk\|_legacy" internal/` 无命中（文件名已正确，本 Sprint 确认通过）

**T20 验收**：`AppendEffectiveMCPServerToolsets`、重复 `NewInMemoryTRPCSessionService`、`toolFilterForPrefix` 冗余代码均已不存在（本 Sprint 确认通过）

---

## Sprint 2 验收总结

| 条目 | 状态 |
|------|------|
| `grep -rn "runtimedeps" internal/` 仅在 alias 文件命中 | ✅ |
| `go vet ./internal/runtime/... ./internal/service/... ./internal/team/...` | ✅ |
| `go test ./internal/event/... -v` 背压用例全部通过 | ✅ |
| `go vet ./internal/agent/...` cache 模块 | ✅ |
| `npx vite build` 前端 0 错误 | ✅ |
| `web/src/stores/<域>/index.ts` 19 域（含原 agents/avatar）均有 store | ✅ |
| 401/429/5xx 拦截器 + WS 重连已接入 | ✅ |
