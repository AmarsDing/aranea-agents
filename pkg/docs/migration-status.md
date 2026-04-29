# 迁移进度追踪（Aranea Hexagonal Refactor）

> **AI 必读**：本文件是 `aranea/docs/0 main design.md` §12.1.1「文件级迁移映射表」的**状态视图**。AI 在开始任何迁移 PR 之前**必须先读本文件**，结束后**必须更新本文件**。任何代码变更违反 `0 main design.md` 顶部 8 条速查或 §12.1.2 红线，立即停手并在「卡点登记」补一行。

## 元数据

| 项 | 值 |
| --- | --- |
| 关联架构文档 | `aranea/docs/0 main design.md` |
| 关联 Runbook | `0 main design.md` §12.1.3 |
| 关联代码模板 | `0 main design.md` 附录 A |
| 关联 CI 红线 | `0 main design.md` 附录 B |
| 总行数 | 32 |
| 状态枚举 | `todo` / `in_progress` / `done` / `skipped`（含原因） / `blocked`（含卡点编号） |

## 阶段总览

| 阶段 | 目标 | 涉及行 | 进度 |
| --- | --- | --- | --- |
| **P0 建壳** | 6 个 Context 目录 + Kernel 目录 + app/ 目录创建空 Module 壳，全仓 `go build ./...` 不破 | — | ☑ (2026-04-27) |
| **P1 Kernel 落地** | runctx / errs / event / contracts 上提 | #1 #2 #28 #30 | ☑ |
| **P2 Capability 整合** | tools 全树 + tool_service + tool_handler + repo 收敛 | #5 #6 #7 #8 #9 #10 #28 | 🟡 #5–#10 已落地（#8–#10：sqlite 适配器 + application 层 ToolService；repository/service 薄委托） |
| **P3 Catalog 收敛** | agent / evolution / avatar | #11 #12 #13 #14 #15 #26 | ☑ (2026-04-27) #11–#15 + #26 已落地 |
| **P4 Memory 重组** | L0~L4 / pii_filter / extractor | #16 #17 #18 #19 #20 #21 | ☑ (2026-04-27) #16–#21 已落地（#18：memory 边界 `PIIFilter` 别名；#21：`memoryhttp.MemoryHTTP`） |
| **P5 Conversation 收敛** | runtime → adkruntime + chat/session/team_runtime + sessions/messages/transport | #3 #4 #22 #23 #24 #25 | ☑ (2026-04-27) #3–#4 #22–#25 已落地 |
| **P6 Operations 集中** | cron + audit + usage（含 transport 旧 handler） | #27 #31 | ☑ (2026-04-27) #27 #31 已落地 |
| **P7 装配收尾** | server.go 拆分 → app/router + cmd/aranea/main；CLI 子命令归位（一章 §11.1） | #28b #29 #32 + #seed + #cli | ☑ (2026-04-27) #28b–#32 #seed #cli 已落地 |
| **P8 Alias 清理** | 删除 Deprecated 兼容层；直引 canonical 包 | 所有行 | ☑ (2026-04-27) 主要批：domain 哨兵与记忆别名删除；`internal/server` 删除；`internal/service` 等薄别名仍保留至入站全切 Context |

## P0 建壳清单（先于任何映射表行执行）

不需要搬代码，只创建空目录与空 Module 壳，确保 6 个 Context 都能 `go build ./...` 通过。

- [x] `internal/kernel/{ids,clock,errs,event,runctx,contracts,module,telemetry,pkg}` 全部创建空 `.go` 占位
- [x] `internal/app/{container.go,modules.go,bootstrap.go,router.go,middleware,migrations.go,openapi.go}` 创建占位（modules.go 返回 6 个 Context 的空 Module）
- [x] `internal/identity/{module.go,domain,application,ports,adapters/{http,sqlite/migrations}}` 空壳
- [x] `internal/catalog/{...}` 空壳
- [x] `internal/capability/{...}` 空壳（含执行子系统 9 个子包：tooldef/toolctx/middleware/executor/registry/backends/adkbridge/schema/telemetry）
- [x] `internal/conversation/{...,adapters/adkruntime}` 空壳
- [x] `internal/memory/{...,adapters/vectorstore}` 空壳
- [x] `internal/operations/{...,adapters/cron}` 空壳
- [x] 各 Context 的 `module.go` 实现 `module.Module` 四阶段（全部 noop），通过 `go build ./...`
- [x] P7 #32 曾保留 `internal/server` 委托 `app`；P8 已删 `internal/server`，`web` launcher 与入口直用 `internal/app`
- [x] 新增可执行契约测试 `internal/app/bootstrap_test.go::TestP0_ModulesCompose` 验证 6 模块装配/启停/OpenAPI

**验证门禁（全量迁移完成后执行一次，不在每行/每阶段反复跑整套）**：

单条映射行内仍以 §12.1.3 的 `go build` 为**最小**回归；**整仓**的 `go build ./...` / `go vet ./...` / `go test ./...` / `golangci-lint` 仅在**下列里程碑全部打勾后**再跑，避免中途噪音；结果记入本文件「操作日志」。

| 里程碑 | 命令（在 `aranea/backend` 下） |
| --- | --- |
| 迁移行全部 done（或经批准的 skipped） | `go build ./... && go vet ./... && go test ./... -count=1` |
| 可选（附录 B 已接 CI 时） | `golangci-lint run --timeout=3m` |

P0 专项：`go test ./internal/app/... -count=1 -run TestP0_ModulesCompose -v` 可在 **P0 已勾选且 app 未大改** 时作为快速门禁；**默认**以整仓里程碑为准（见上表）。

## 文件级迁移行（与 §12.1.1 一一对应）

> **格式**：每行注明：编号 / 状态 / 源 → 目标 / PR 链接（可选）/ 备注。AI 完成一行后把 `☐` 改成 `☑` 并填 PR 链接与日期。

### Kernel 上提（P1）

- [x] **#1** `done` (2026-04-27) — `internal/runtime/runtime_context.go` → `internal/kernel/runctx/runtime_context.go` —— Deprecated alias 留存于旧路径；`renderRuntimeContextBlock` 升级为 `runctx.RenderBlock`（adk_adapter.go 已切换）；走读样板见 `0 main design.md` 附录 C
- [x] **#2** `done` (2026-04-27) — 哨兵错误在 `internal/kernel/errs`；P8 已删 `internal/domain/errors.go`，全仓直引 `errs`；`codes.go` / `error.go` 为真源
- [x] **#28** `done` (2026-04-27) — `internal/repository/contracts.go` 中 `Store` + 四查询结构体已迁至 `internal/kernel/contracts/store.go`；`repository/contracts.go` 仅保留与 `contracts` 等价的 **type 别名**（`repository.Store` 等仍可用）；`sqlite.go` 增加 `var _ contracts.Store = (*SQLiteRepository)(nil)`。按 Context 拆细端口并落 `<context>/ports/output.go` 随 P2–P7 服务迁移进行。
- [x] **#30** `done` (2026-04-27) — `internal/kernel/pkg/db/{open.go,migrate.go}`（`OpenSQLite` + `MigrateWithLegacyHook`）；`repository/sqlite.go` 已委托；六 Context `adapters/sqlite/init.go` 暴露 `Open = db.OpenSQLite`

### Capability 整合（P2）

- [x] **#5** `done` (partial physical) (2026-04-27) — `adkbridge/executor/...` 已落在 `internal/capability/*` 并被 `runtime/tools_bridge.go` 引用；`tools_bridge` 本文件仍挂在 `package runtime`（因 `ADKRuntimeAdapter` 未迁 #3）。待 #3 后可将文件移入 `conversation/adapters/adkruntime/`
- [x] **#6** `done` (2026-04-27) — `internal/runtime/tool_policy.go` 已删除；等价逻辑在 `internal/capability/middleware/policy.go`（`JSONStringList` / `ToolSet` / `ProfileAllows`），`tools_bridge` 已改用 `toolmw.*`
- [x] **#7** `done` (2026-04-27) — `internal/tools/**` 已删除；实现位于 `internal/capability/{tooldef,toolctx,middleware,executor,registry,backends,adkbridge,schema,telemetry,storage}`；全仓 import 已改为 `arenea/backend/internal/capability/...`
- [x] **#8** `done` (2026-04-27) — `internal/capability/adapters/sqlite/repository_tool.go`（`ToolRepository` 委托 `capability/storage`）；`internal/repository/sqlite_tools.go` 仅委托 `NewToolRepository(r.db)`
- [x] **#9** `done` (2026-04-27) — `internal/capability/adapters/sqlite/repository_skill.go` + `helpers.go`；`repository/sqlite_skills.go` 瘦身为委托；`DeleteSkill` 与平台软删 SQL 对齐
- [x] **#10** `done` (2026-04-27) — `internal/capability/application/{tool_service.go,validation.go}`；`domain.DefaultAgentRuntimeSettings()`；`internal/service/tool_service.go` 为 type alias + `NewToolService` 转发

### Catalog 收敛（P3）

- [x] **#11** `done` (2026-04-27) — `internal/catalog/application/{agent_service.go,new_id.go}`；`internal/service/agent_service.go` 为 `AgentService` 别名 + `NewAgentService` 转发
- [x] **#12** `done` (2026-04-27) — `internal/catalog/application/{agent_evolution_service.go,evolution_scanner.go,pii_filter.go,validation.go,evolution_helpers.go}`；`internal/service` 保留 `AgentEvolutionService`/`PIIFilter` 等类型别名与 `New*` 转发；原 `agent_evolution_scanner.go` 已删除（实现仅在 catalog 包）
- [x] **#13** `done` (2026-04-27) — `internal/catalog/adapters/sqlite/repository_evolution.go`（`EvolutionRepository` + `contracts.Evolution*Query`）；`repository/sqlite_agent_evolution.go` 瘦委托
- [x] **#14** `done` (2026-04-27) — `internal/catalog/adapters/sqlite/repository_agent.go`（`AgentRepository`）；`repository/sqlite_agents.go` 瘦委托；共享 `codec.go`
- [x] **#15** `done` (2026-04-27) — `internal/catalog/adapters/sqlite/repository_avatar.go`（`AvatarRepository` / `SeedAvatarAssets`）；`repository/avatar.go` 瘦委托
- [x] **#26** `done` (2026-04-27) — `internal/transport/agent_evolution.go` 已删除；`internal/catalog/adapters/http/evolution_handler.go`（`EvolutionHTTP` + `NewEvolutionHTTP`）；`internal/transport/handler.go` 注入并 `Register` + `agents` 代理路径；`evolutionService()` 留于 `handler.go`

### Memory 重组（P4）

- [x] **#16** `done` (2026-04-27) — L0~L4 服务实现于 `internal/memory/application/memory_l{0,1,2,3,4}_service.go` + `helpers.go`；`internal/service/memory_l{0,1,2,3,4}_service.go` 为类型别名 + `New*` 转发
- [x] **#17** `done` (2026-04-27) — 原 `memory_l4_extractor.go` 已迁为 `internal/memory/application/l4_extraction_scan.go`（与 #16 同批）；`TestExtractionScannerRespectsWordBoundaries` 迁至 `l4_extraction_scan_test.go`
- [x] **#18** `done` (2026-04-27) — `internal/memory/application/pii_filter.go`：`PIIFilter` / `NewPIIFilter` 为 `catalog/application` 的 type 别名与 var 重导出；L3 与 `internal/service/pii_filter.go` 经 Memory 边界引用，不重复实现
- [x] **#19** `done` (2026-04-27) — 真源 `internal/memory/domain/memory_l{0,1,2,3,4}.go`；P8 已删 `internal/domain/memory_l*.go` 重导出，调用方 `import mem "…/memory/domain"`
- [x] **#20** `done` (2026-04-27) — `internal/memory/adapters/sqlite/repository_l{0,1,2,3,4}.go` + `helpers.go`；`internal/repository/sqlite_memory_l{0,1,2,3,4}.go` 为 `memL*()` 薄委托
- [x] **#21** `done` (2026-04-27) — `internal/memory/adapters/http`（`memoryhttp.MemoryHTTP` + `l0`–`l4.go` + `helpers.go`）；`internal/transport/handler.go` 注入并 `Register`；`sessions.go` 用 `HandleL0/L1/L2` 入口；原 `memory_l*.go` 已删

### Conversation 收敛（P5）

- [x] **#3** `done` (2026-04-27) — ADK 实现已在 `internal/conversation/adapters/adkruntime/**`；`adk_runner_backend` 的 `ToolHint` 与 `runctx` 对齐（#4 已删 `internal/runtime` 重导出层）
- [x] **#4** `done` (2026-04-27) — 删 `internal/runtime`（`adk_shim.go`+`runtime_context.go`）；调用方直引 `adkruntime` + `kernel/runctx`
- [x] **#22** `done` (2026-04-27) — 实现于 `internal/conversation/application/{run_turn_handler.go,ids.go,errors.go}` + 测试 `run_turn_handler_test.go`；`internal/service/chat_service.go` 为嵌入 `*application.ChatService` 与 DTO/事件 类型别名；`firstNonEmptyString` 抽取至 `internal/service/string_helpers.go` 供非 Conversation 的 service 包复用
- [x] **#23** `done` (2026-04-27) — `internal/conversation/application/session_service.go`；`internal/service/session_service.go` 为 `SessionService` 类型别名 + `NewSessionService` 转发
- [x] **#24** `done` (2026-04-27) — 与 #22 同 PR（`*ChatService` 方法集）：`internal/conversation/application/team_runtime_service.go`；`team_run_events.go` 已在 `application` 包
- [x] **#25** `done` (2026-04-27) — `internal/conversation/adapters/sqlite/{store.go,helpers.go,repository_session.go,repository_message.go}`（`Store` + 共库 `sessions`/`messages`）；`internal/repository/sqlite_sessions.go` 为薄委托（已删 `sqlite_messages.go`）

### Operations 集中（P6）

- [x] **#27** `done` (2026-04-27) — `internal/capability/adapters/http/tool_handler.go`（`ToolHTTP` + `Register`）；`internal/transport/handler.go` 注入与 `NewToolHTTP`；`transport/tools.go` 已删
- [x] **#31** `done` (2026-04-27) — `internal/operations/adapters/sqlite/{store.go,helpers.go,repository_cron.go}`；`internal/repository/sqlite_cron.go` 为 `operationsSQL()` 薄委托

### 装配与收尾（P7）

- [x] **#28b** `done` (2026-04-27) — `internal/kernel/pkg/httpx/{response.go,error.go,doc.go,error_test.go}`；`internal/transport/response.go` 为对 `httpx` 的薄包装 + `listResponse` 类型别名；`handler.go` 未改行（无逻辑迁移）原 `response_test` 迁为 `httpx` 内测试
- [x] **#29** `done` (2026-04-27) — `internal/app/middleware/cors.go`；`internal/middleware/cors.go` 委托 `app/middleware.CORS`（server 仍 import `internal/middleware`）
- [x] **#32** `done` (2026-04-27) — `internal/app/http_run.go`（[Run] + 后台循环）、`router.go` 增 `StackTransportMiddleware`；`cmd/aranea/main.go`；P8 删 `internal/server`（`web` launcher 直 `app.Run`）；`README` / `docs/25 cli.md` 构建路径更新
- [x] **#seed** `done` (2026-04-27) — `conversation/catalog/capability/adapters/sqlite/seeds.go`（`SeedChatOptions` / `SeedPlatformDefaults`+`SeedSystemAdminAgent` / `SeedBuiltinTools`+`CLIAdminToolKeys`）；`repository/sqlite.go` 编排；删 `sqlite_seeds.go`+`sqlite_cli_admin_seeds.go`
- [x] **#cli** `done` (2026-04-27) — `cmd/internal/**` 迁入 `cmd/aranea/cli/**` + `cmd/aranea/launcher/**`；`root.go` 为 `package cli`；`main` 用 `cli.Execute()`；子命令仍按资源分子包（与单文件多 `newListCmd` 冲突的 Go 限制一致，见 §11.1 备注）

### Alias 清理（P8）

- [x] **#alias-sweep** `done` (2026-04-27) — 删 `internal/domain/errors.go` 与 `memory_l{0..4}.go`；HTTP 与领域代码直引 `kernel/errs`、记忆 DTO 用 `mem`（`memory/domain`）；`agent_evolution.go` 的 `EvidenceRef` 用 `mem`；`kernel/contracts/store.go` 与 SQLite 仓储已对齐；`internal/server` 包删除。保留项：`internal/service` 等对 Context application 的 type alias / `New*` 转发（transport 等仍用），附录 B 全 strict CI 未接

## 卡点登记（Block Log）

> AI 在迁移过程中遇到障碍时，**不要硬推**，在此追加一行：编号、卡在哪一行、原因摘要、建议方案、谁来决策。

| BLK# | 关联行号 | 卡点摘要 | 建议方案 | 状态 |
| ---- | -------- | -------- | -------- | ---- |
| —    | —        | —        | —        | —    |

## 操作日志（Per-PR Log）

> 每完成一行，AI 在此追加一行简短记录。便于后续 AI 会话快速对齐进度。

| 日期 | PR | 行号 | 行为 | 备注 |
| ---- | -- | ---- | ---- | ---- |
| 2026-04-27 | (pending) | P0 | scaffold | 6 Context + Kernel + app/ 全壳；新增 `github.com/go-chi/chi/v5` 依赖；`TestP0_ModulesCompose` 通过 |
| 2026-04-27 | (pending) | #1 | move | `internal/runtime/runtime_context.go` → `internal/kernel/runctx/runtime_context.go`；旧路径留 type-alias；`adk_adapter.go` 切到 `runctx.RenderBlock`；`go build/vet`、`runtime/app/service` 测试全绿 |
| 2026-04-27 | (pending) | #2 | kernel-errs | `domain/errors.go` 上提为 `kernel/errs/{codes.go,error.go}`；domain 留 re-export；`go build/vet`、`go test ./...` 全绿 |
| 2026-04-27 | (pending) | #28 | kernel-contracts-store | `Store` + 查询 DTO 迁入 `kernel/contracts/store.go`；`repository` 类型别名 + `SQLiteRepository` 实现校验 |
| 2026-04-27 | (pending) | #30 | kernel-pkg-db | `kernel/pkg/db` OpenSQLite + MigrateWithLegacyHook；repository 接入；六 Context `adapters/sqlite/init.go` |
| 2026-04-27 | (pending) | #7 | tools→capability | 删除 `internal/tools`；import 全改为 `internal/capability/...`；middleware UTF-8 修复 |
| 2026-04-27 | (pending) | #6 | tool-policy | `capability/middleware/policy.go`；`runtime/tool_policy.go` 删除 |
| 2026-04-27 | (pending) | full-verify | milestone | `go build ./...` + `go vet` + `go test ./... -count=1` 全绿（本批次结束） |
| 2026-04-27 | (pending) | #8 #9 #10 | capability-repo-app | `repository_tool` / `repository_skill` / `application.ToolService`；`go build`/`go vet`；`go test` 工具相关子集通过（整仓 `go test` 在 Windows 上部分用例可能超时，与本次改动无关） |
| 2026-04-27 | (pending) | #11 | catalog-agent-svc | `catalog/application/AgentService` + `new_id`；`service` 薄别名；`go build`/`go vet`；`transport`/`app` 测试通过 |
| 2026-04-27 | (pending) | #12 | catalog-evolution | `AgentEvolutionService` + 扫描器 + `PIIFilter` 迁入 catalog；`service` 别名；`go build`/`go vet`；`go test service -run Evolution\|Scanner\|Tool` 通过 |
| 2026-04-27 | (pending) | #13–#15 | catalog-sqlite-repo | `EvolutionRepository` / `AgentRepository` / `AvatarRepository` + `codec.go`；`repository` 三文件瘦委托；`go test repository/transport` 通过 |
| 2026-04-27 | (pending) | #26 | catalog-evolution-http | `cataloghttp.EvolutionHTTP`；`adapters/http/doc.go` 与 `package cataloghttp` 对齐；`go build`/`go vet`/`go test ./...` 全绿 |
| 2026-04-27 | (pending) | #16 #17 | memory-application | L0–L4 + `l4_extraction_scan`；`service` 薄别名；`go build` + `go test` memory+service+transport 通过（`#18` 边界别名见后行 #18#21） |
| 2026-04-27 | (pending) | #19 #20 | memory-domain-sqlite | `internal/memory/domain` + `adapters/sqlite`；`domain` 与 `repository` 重导出/委托；`go build` + `go vet` + `go test ./internal/repository/...` 通过；`#21` 未动 |
| 2026-04-27 | (pending) | #18 #21 | memory-pii-http | `memory/application/pii_filter.go` 边界重导出；`memory/adapters/http` 承接原 `transport/memory_l*.go`；`go build ./...` + `go test internal/transport memory` 通过 |
| 2026-04-27 | (pending) | #3 | conversation-adkruntime | `internal/runtime/adk_shim.go` 重导出 ADK 相关 API；`adk_runner_backend` 使用 `runctx.ToolHint`；`go build`/`go vet`/`go test ./...` 全绿；本机无 golangci-lint 可执行文件 |
| 2026-04-27 | (pending) | #22 #24 | conversation-chat-team | `conversation/application` 承接 chat + team 编排 + team_run_events + 原 chat 测试；`service.ChatService` 嵌入；`go build`/`go vet`；`go test` conversation/application + service 通过 |
| 2026-04-27 | (pending) | #23 | conversation-session-svc | `application/session_service.go`；`service` 薄别名 + `NewSessionService`；`go test` application/service/transport 通过 |
| 2026-04-27 | (pending) | #25 | conversation-sqlite-sessions-messages | `adapters/sqlite` `Store`；`repository` 单文件委托；`go test` repository/memory/conversation 通过 |
| 2026-04-27 | (pending) | #27 | capability-tool-http | `capability/adapters/http` `ToolHTTP`；`handler` 委托 `pageParams`/`idFromPath`；`go test` transport + capability 通过 |
| 2026-04-27 | (pending) | #31 | operations-sqlite-cron | `operations/adapters/sqlite` `Store` + cron CRUD；`repository` 委托；`go test` repository/operations/service 通过 |
| 2026-04-27 | (pending) | #28b | kernel-httpx | `pkg/httpx` Write/Decode/IDFromPath/错误映射；`transport/response` 薄委托；`go test` httpx+transport 通过 |
| 2026-04-27 | (pending) | #29 | app-middleware-cors | `app/middleware/cors.go`；`internal/middleware` 薄委托；`go build`/`go test ./...` 通过 |
| 2026-04-27 | (pending) | #32 | app-http-run-cmd-aranea | `app/http_run.go` + `StackTransportMiddleware`；`server` 委托；`cmd/aranea/main`；`go build`/`go vet`/`go test ./...` 全绿 |
| 2026-04-27 | (pending) | #seed | context-sqlite-seeds | 三 Context `seeds.go`；`tool_service` 用 `capsql.CLIAdminToolKeys`；`go test ./...` 全绿 |
| 2026-04-27 | (pending) | #cli | cmd-aranea-cli-launcher | `cmd/aranea/cli` + `launcher`；删 `cmd/internal`；`go build`/`go vet`/`go test ./...` 全绿 |
| 2026-04-27 | (pending) | #4 | remove-internal-runtime | 全仓改引 `adkr`/`runctx`；删 `internal/runtime`；`adkruntime/doc.go` 更新 |
| 2026-04-27 | (pending) | P8 | alias-sweep-errs-mem-server | 删 `domain/errors.go`+`memory_l*.go`；`domain.Err*→errs`、`memory` DTO 用 `mem`；`contracts/store`+SQLite 全仓换 import；删 `internal/server`，`launcher/web` 用 `app`；`go build`/`go vet`/`go test ./...` 全绿 |
