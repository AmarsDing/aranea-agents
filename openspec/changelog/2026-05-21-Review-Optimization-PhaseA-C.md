# Review Optimization — Phase A–C (2026-05-21)

## Phase A

- **Chat composable split**: `useChatWorkspace.ts` ~1169 → ~505 行；抽出 `useFollowUpQueue`、`useAwaitReply`、`useChatTraceAndArtifacts` 等子 composable。
- **Doc sync**: `docs/review/README.md`、`26-a2a`、`18-monitor`、`12-16-memory`、`27-artifact`、`37-knowledge`、`24-telemetry`、`10-session` 与 execution-plan 对齐。

## Phase B

- **MemoryWorker**: `internal/biz/memory_worker_test.go`
- **Team modes**: `team_modes_test.go` 扩展 parallel/swarm
- **WS protocol**: `ws_protocol_test.go` 补 unsubscribe/enqueue
- **Session**: `session_timeline.go` 自 `session_usecase.go` 拆分 Timeline
- **Auth**: `pkg/auth/token_test.go` JWT 过期/签名

## Phase C

- **Graph**: `node_wiring` agent/router 分支；`CatalogAgentResolver` + Wire `provideGraphBuildDeps`
- **A2A**: `a2a_limit.go` 60/min 限流
- **Monitor**: `avg_duration_ms` proto + data/biz/service
- **Artifact Chat**: Chat 侧栏制品打开 signed download URL
- **Memory graph tab**: 默认隐藏；`VITE_MEMORY_GRAPH_TAB=1` 启用

## Phase D (partial)

- **Plugin sandbox**: `PluginVersionPolicy` + `ResolvePluginVersion` 占位

## 验证

```bash
go test ./internal/biz/... ./internal/server/... ./internal/graph/...
cd web && pnpm test
make wire-admin   # 更新 provideGraphBuildDeps 签名后
```
