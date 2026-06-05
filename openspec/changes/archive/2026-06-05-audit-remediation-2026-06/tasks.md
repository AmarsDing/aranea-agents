## Non-goals

- ChatService 拆分为 7 个 gateway struct（BA-01 Step2）
- 全面消除 20 个 setter 注入（本次只处理 SessionUsecase 的 3 个）
- 前端 ECharts 配色提取
- 删除 BuildTRPCTeam

## 1. P0 — 构建修复

- [x] 1.1 运行 `go generate ./internal/data/ent/` 补齐 experiencereport 生成物。DoD: `internal/data/ent/experiencereport/` 目录存在且包含 `.go` 文件
- [x] 1.2 验证 `go build ./...` 通过。DoD: 零编译错误

## 2. P0 — 前端组件分层修复

- [x] 2.1 修改 `SelfCheckStatusPanel.vue`：删除 `useMonitorStore` import，改用 `defineProps`（loading/triggering/latestReport）+ `defineEmits`（refresh/trigger）。DoD: 组件内无 `useXxxStore` 调用
- [x] 2.2 修改使用 SelfCheckStatusPanel 的 Page 层，注入 Store 数据和事件处理。DoD: Page 传入 Store state 作为 props，处理 emit 事件调用 Store action
- [x] 2.3 修改 `ProviderLogo.vue`：删除 `useModelCatalogStore` import，新增 `fetchSvg` prop（`(id: string) => Promise<string>`），组件内调用 `props.fetchSvg`。DoD: 组件内无 `useXxxStore` 调用
- [x] 2.4 修改使用 ProviderLogo 的 Page/容器组件，传入 `fetchSvg` 函数。DoD: 父组件通过 `:fetch-svg="(id) => modelCatalogStore.fetchProviderLogoSvg(id)"` 传入
- [x] 2.5 验证前端 `pnpm lint` + `pnpm build` 通过。DoD: 零 lint 错误，构建成功

## 3. P1 — 编译期接口断言补充

- [x] 3.1 在 `internal/service/chat_orchestrator.go` 的 `var _` 块中补充 `biz.A2ARunnerFactory` 断言。DoD: `var _ biz.A2ARunnerFactory = (*ChatService)(nil)` 存在且编译通过

## 4. P1 — TeamUsecase 子接口注入

- [x] 4.1 修改 `internal/biz/team_usecase.go`：TeamUsecase 结构体字段从 `repo TeamRepository` 改为 6 个子接口字段（reader/writer/runReader/runWriter/stepRepo/deadLetter）。DoD: 结构体无 `repo TeamRepository` 字段
- [x] 4.2 修改 `NewTeamUsecase` 构造函数：接收 6 个子接口参数 + agentChecker。DoD: 参数列表包含 TeamReader/TeamWriter/TeamRunReader/TeamRunWriter/OrchestrationStepRepo/TaskDeadLetterRepo
- [x] 4.3 修改 TeamUsecase 所有方法：将 `uc.repo.Xxx` 替换为对应的子接口字段调用（如 `uc.reader.ListTeams`）。DoD: 无 `uc.repo.` 引用
- [x] 4.4 在 `cmd/admin/wire.go` 中补充 5 个 `wire.Bind`（TeamWriter/TeamRunReader/TeamRunWriter/OrchestrationStepRepo/TaskDeadLetterRepo → TeamRepository）。DoD: Wire 绑定完整
- [x] 4.5 运行 `make wire && go build ./...`。DoD: Wire 生成成功，编译通过

## 5. P1 — SessionUsecase setter 消除

- [x] 5.1 修改 `internal/biz/session/usecase.go`：`NewSessionUsecase` 新增 `statusPublisher`/`metricsPublisher`/`runtimeWriter` 三个参数。DoD: 构造函数签名包含这三个参数
- [x] 5.2 删除 `SetStatusPublisher`/`SetMetricsUpdatedPublisher`/`SetRuntimeWriter` 三个方法。DoD: 方法不存在
- [x] 5.3 修改 `internal/service/run_status_publish.go` 的 `WireSessionStatusPublisher`：从 setter 调用改为在 Wire provider 中直接传入参数。DoD: 无 setter 调用
- [x] 5.4 修改 `cmd/admin/wire.go` 中 SessionUsecase 相关的 provider 函数。DoD: Wire 解析正确
- [x] 5.5 运行 `make wire && go build ./...`。DoD: 编译通过

## 6. P2 — Orchestrator 业务逻辑下沉

- [x] 6.1 在 `internal/biz/usage/usage.go` 新增 `CheckTeamMemberQuotas(ctx, teamID) error` 方法，迁移 Orchestrator 中的配额检查逻辑。DoD: 方法存在且逻辑完整
- [x] 6.2 修改 `ChatOrchestrator.checkTeamMemberQuotas` 为委托调用 `o.usage.CheckTeamMemberQuotas`。DoD: Orchestrator 方法体仅 1 行委托
- [x] 6.3 在 `internal/biz/usage/usage.go` 新增 `TurnUsageInput` struct 和 `RecordTurnUsage(ctx, TurnUsageInput) error` 方法。DoD: 方法存在，包含 TokenUsageEvent 构造、落库、指标累加、事件发布、runner completion 关联
- [x] 6.4 修改 `ChatOrchestrator.recordTurnUsage` 为委托调用 `o.usage.RecordTurnUsage`。DoD: Orchestrator 方法体仅构造 TurnUsageInput + 委托调用
- [x] 6.5 在 `internal/provider/` 新增 `capabilities.go`：导出 `ValidateAttachmentCapabilities`/`HasImageAttachment`/`HasFileAttachment`。DoD: 函数签名与原 Orchestrator 版本一致
- [x] 6.6 修改 `ChatOrchestrator.validateTurnAttachmentCapabilities` 为委托调用 `provider.ValidateAttachmentCapabilities`。DoD: Orchestrator 方法体仅 1 行委托
- [x] 6.7 删除 `chat_attachments.go` 中的 `hasImageAttachment`/`hasFileAttachment` 包级函数（已迁到 provider）。DoD: 函数不存在于 service 包
- [x] 6.8 运行 `go build ./...`。DoD: 编译通过

## 7. P2 — 全局生命周期 context

- [x] 7.1 新增 `pkg/appctx/appctx.go`：导出 `Init()`/`Ctx()`/`Cancel()` 三个函数。DoD: 包可编译
- [x] 7.2 在 `cmd/admin/main.go` 启动流程中调用 `appctx.Init()`，在 shutdown 流程中调用 `appctx.Cancel()`。DoD: 生命周期 context 正确初始化和清理
- [x] 7.3 替换 `internal/` 下所有 `safego.Go(context.Background(), ...)` 为 `safego.Go(appctx.Ctx(), ...)`。DoD: grep `safego.Go(context.Background()` 返回 0 处
- [x] 7.4 替换 ChatOrchestrator 的 `svcCtx`/`svcCancel` 为 `appctx.Ctx()`，删除相关字段。DoD: ChatOrchestrator 无 svcCtx/svcCancel 字段
- [x] 7.5 运行 `go build ./...`。DoD: 编译通过

## 8. P3 — 编程规范修复

- [x] 8.1 在 `internal/team/trpc_build.go` 的 `BuildTRPCTeam` 函数注释中添加 Phase 8 清理标记 `TODO(phase-8)`。DoD: 注释包含清理计划
- [x] 8.2 修改 `internal/biz/pack/importer.go` 中 16 处业务 `fmt.Errorf` 为 `kerrors.BadRequest("PACK_XXX", ...)`。DoD: importer.go 无业务 fmt.Errorf
- [x] 8.3 修改 `internal/biz/pack/exporter.go` 中 6 处业务 `fmt.Errorf` 为 `kerrors.BadRequest("PACK_XXX", ...)`。DoD: exporter.go 无业务 fmt.Errorf
- [x] 8.4 修改 `internal/biz/pack/writer.go` 中 5 处业务 `fmt.Errorf` 为 `kerrors.BadRequest("PACK_XXX", ...)`。DoD: writer.go 无业务 fmt.Errorf
- [x] 8.5 修改 `internal/biz/monitor/root_cause_engine.go` 中 4 处 `fmt.Errorf` 为 `kerrors.BadRequest("RCA_XXX", ...)`。DoD: root_cause_engine.go 无 fmt.Errorf
- [x] 8.6 新增 `internal/biz/monitor/heal_constants.go`：合并 self_heal.go 和 self_heal_observer.go 的重复常量。DoD: 新文件包含所有常量和 SeverityCooldown map
- [x] 8.7 从 `self_heal.go` 和 `self_heal_observer.go` 删除重复常量定义，改为引用 heal_constants.go。DoD: 两个文件无重复常量
- [x] 8.8 拆分 `ChannelTurnDeps` 为 `ChannelTurnJobDeps`（TurnJobs/SessionRuns/Channels）+ `ChannelNotifierDeps`（RunEscalation）。DoD: ChatOrchestratorDeps 和 ChatOrchestrator 使用拆分后的类型
- [x] 8.9 运行 `go build ./...`。DoD: 编译通过

## 9. P3 — 前端 UX 修复

- [x] 9.1 新增 `web/src/css/theme/_color-palette.sass`：定义头像调色板、行业渐变色 CSS 变量。DoD: 文件包含 `--palette-avatar-*` 和 `--palette-industry-*` 变量
- [x] 9.2 修改 `features/industries/industryMonogram.ts`：将纯 hex 渐变色替换为 CSS 变量读取。DoD: 无纯 hex 硬编码
- [x] 9.3 修改 `features/chat/useChatMessageRow.ts`：将头像调色板 hex 替换为 CSS 变量读取。DoD: 无纯 hex 硬编码
- [x] 9.4 验证 `pnpm lint && pnpm build` 通过。DoD: 零 lint 错误，构建成功

## 10. P4 — 文档同步

- [x] 10.1 在 `internal/event/flow_context.go` 的 CtxFlowLog* 函数注释中标记 `Deprecated: Use loggateway.Logger + With()`。DoD: 注释包含 Deprecated 标记
- [x] 10.2 在 `internal/biz/doc.go` 中补充缺失文件说明（evaluation.go/json_list.go/json_schema.go/pagination.go）。DoD: doc.go 索引完整

## 11. 全量验证

- [x] 11.1 后端全量验证：`make api && make wire && make build && make test && make lint`。DoD: 全部通过
- [x] 11.2 前端全量验证：`cd web && pnpm lint && pnpm test && pnpm build`。DoD: 全部通过
