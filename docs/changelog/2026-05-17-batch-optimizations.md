# 2026-05-17 — 批量优化（execution-plan.md §5 Top-20 阶段性落地）

## 变更摘要

本次批量实施 `docs/guides/execution-plan.md` §3 / §5 中识别的高优先级问题，覆盖安全加固、运行时正确性、可观测性、工程化 CI 与前端清理五个维度。所有修改已通过 `go build ./...`、`go vet ./...`、`araneactl lint` 三重验证。

---

## 已完成 EP 编号

### 安全（P0）

| EP 编号 | 文件 | 变更描述 |
|---|---|---|
| EP-SEC-01 | `pkg/auth/config.go` | `authSecretFromEnv`：未设 `KRATOS_AUTH_SECRET` 且非 dev/CI 环境时 panic fail-fast；DEPLOY_ENV=dev/test 或 CI=true 时使用 placeholder 不 panic |
| EP-SEC-02 | `pkg/auth/features.go` | `HTTPAuthBypassEnabled`：新增 DEPLOY_ENV 检查，非 dev/test/CI 环境拒绝 bypass；新增 `WarnIfBypassEnabled()` 启动 banner 函数 |
| EP-SEC-02 | `cmd/admin/main.go` | 在 config.Load 之后调用 `auth.WarnIfBypassEnabled()` 打印启动告警 |

### 运行时正确性（P0/P1）

| EP 编号 | 文件 | 变更描述 |
|---|---|---|
| EP-RT-01 | `internal/service/trpc_turn.go` | `runSingleAgentViaTRPC` 开始时设 `running`，出错时设 `failed`，完成时设 `completed`；pendingQueue goroutine 改用 `safego.Go` |
| EP-BIZ-04 | `internal/evaluation/runner.go` | `execute()` 开头加 nil guard：`r.agent == nil` 时优雅 `failRun` 而非 panic |
| EP-RT-06 | `internal/server/ws.go` | WS 订阅：session 模式 `Reliable=true`；全局监控模式 lossy（默认）；区分关键事件与 delta |

### 可观测性（P1/P2）

| EP 编号 | 文件 | 变更描述 |
|---|---|---|
| EP-OBS-01 | `internal/server/http.go` | `NewHTTPServer` 末尾挂 `promhttp.Handler()` 至 `/metrics`；bypass auth |
| EP-OBS-03 | `cmd/admin/main.go` | `newApp` 新增 `*server.WSServer` 参数；nil 安全地加入 `transport.Server` 列表，保证优雅退出触发 `broadcastShutdown` |
| EP-OBS-03 | `cmd/admin/wire_gen.go` | `wireApp` 中 `newApp` 调用传入 `wsServer` |

### 前端清理（P2）

| EP 编号 | 文件 | 变更描述 |
|---|---|---|
| EP-FE-02 | `web/src/services/wsClient.ts` | **已删除**（全仓无引用，与真实 ws-transport 双轨） |
| EP-FE-02 | `web/src/composables/useWS.ts` | **已删除** |

### 工程化 CI（P1）

| EP 编号 | 文件 | 变更描述 |
|---|---|---|
| EP-ENG-01 | `.github/workflows/ci.yml` | Go test 从白名单改为 `go test -race ./...` 全量；覆盖率阈值阶梯化（M3=40%→M4=60%→M5=70%）|
| EP-ENG-02 | `.github/workflows/ci.yml` | 前端 `npm test` 去掉 `\|\| echo`；失败即阻断 CI |
| EP-ENG-03 | `Makefile` | 新增 `make wire`（wire-admin 别名）与 `make wire-clean`（重生成 + git diff 检查）|
| EP-ENG-03 | `.github/workflows/ci.yml` | 新增 `wire-clean` CI job：安装 wire → `make wire` → `git diff --exit-code wire_gen.go` |

### 红线合规（R14 EP-RULE-04）

| EP 编号 | 文件 | 变更描述 |
|---|---|---|
| EP-RULE-04 | `internal/server/ws.go` | 全部 `go func()` 替换为 `safego.Go` |
| EP-RULE-04 | `internal/service/trpc_turn.go` | `processPendingQueue` goroutine 替换为 `safego.Go` |
| EP-RULE-04 | `internal/service/session_compress.go` | `AfterNativeTurn` goroutine 替换为 `safego.Go` |
| EP-RULE-04 | `internal/biz/event_bus_consumer.go` | `Start` goroutine 替换为 `safego.Go`；同时将订阅设为 `Reliable=true` |
| EP-RULE-04 | `internal/biz/domain_event_adapter.go` | `SubscribeDomainEvents` goroutine 替换为 `safego.Go` |
| EP-RULE-04 | `internal/adapter/graph/runtime_adapter.go` | `Run` / `Resume` 两处 goroutine 替换为 `safego.Go` |

---

## 影响范围

- `pkg/auth/**` — JWT 初始化逻辑变更；现有测试需 CI=true 或 DEPLOY_ENV=dev/test 才能通过
- `internal/server/http.go` — 新增 `/metrics` 路由（无 auth，供 Prometheus scrape）
- `cmd/admin/main.go` / `wire_gen.go` — `newApp` 签名变更；`wire_gen.go` 已手动同步（待 `make wire` 重新生成验证）
- `web/src/services/wsClient.ts` / `web/src/composables/useWS.ts` — 已删除

## 未完成（待后续迭代）

| EP 编号 | 原因 |
|---|---|
| EP-RT-02 | await_user_reply 需要深入 trpc-agent-go BeforeTool 机制，不在本次 PR 范围内 |
| EP-RT-03 | auto_memory extractor 需要 LLM provider 上下文注入 worker，依赖 EP-RT-07 先行 |
| EP-OBS-02 | OTel 接入需要新增 telemetry.go + go.mod 依赖，单独 PR |
| EP-BIZ-04（部分）| Evaluation AgentRunner 真实实现需要跨层 adapter，已加 nil guard 防崩溃 |
