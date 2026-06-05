## Context

全栈代码审查发现 3 个阻断项 + 14 个建议项。项目架构整体优秀（依赖方向向内、零裸 goroutine、零 log/slog 残留），但存在以下需修复的问题：

- **构建阻断**：Ent `experiencereport` schema 已创建但生成物未提交，`go build ./...` 失败
- **前端分层违规**：2 个展示组件直接 import Store
- **后端架构**：ChatOrchestrator 94 方法承载部分业务逻辑、TeamUsecase 持有 22 方法聚合接口、20 个 setter 注入点
- **并发**：27 处 `safego.Go(context.Background(), ...)` 无法被优雅关闭
- **编程规范**：pack 子包 27 处业务 `fmt.Errorf`、self-heal 常量重复定义

## Goals / Non-Goals

**Goals:**
- 恢复构建通过（P0）
- 修复前端分层违规（P0）
- 改善后端 OOP 合规性：子接口注入、setter 消除（P1）
- 下沉 Orchestrator 业务逻辑到 biz 层（P2）
- 引入全局生命周期 context（P2）
- 编程规范修复（P3）

**Non-Goals:**
- ChatService 拆分为 7 个 gateway struct（BA-01 Step2）— 留待下个迭代
- 全面消除 20 个 setter 注入 — 本次只处理 SessionUsecase 的 3 个
- 前端 ECharts 配色提取 — 涉及第三方库，单独迭代
- 删除 `BuildTRPCTeam` — Phase 8 处理

## Decisions

### D1: Ent 生成物补齐 — 直接 `go generate`

**选择**：运行 `go generate ./internal/data/ent/` 补齐生成物并提交。

**替代方案**：手动创建 `experiencereport/` 目录 — 违反红线 #8（禁止手动编辑/创建工具生成代码）。

**理由**：Schema 是源头，生成物必须由工具产出。

### D2: 前端组件 — props/emits 上提

**选择**：将 Store 依赖从展示组件上提到 Page/容器层，通过 props 传入数据、emits 传出事件。

**替代方案**：将组件改为"容器组件"（`// Container: approved because ...`）— 但这两个组件（SelfCheckStatusPanel/ProviderLogo）本质上是纯展示，不应持有数据获取职责。

**理由**：符合前端红线 FD1（展示组件禁止 `useXxxStore`）。

### D3: TeamUsecase — 子接口注入

**选择**：`NewTeamUsecase` 接收 6 个子接口参数（TeamReader/TeamWriter/TeamRunReader/TeamRunWriter/OrchestrationStepRepo/TaskDeadLetterRepo），Wire 补充 5 个 `wire.Bind`。

**替代方案**：保持聚合接口但 Usecase 内部定义私有子接口 — 增加复杂度但不改善 Wire 可测试性。

**理由**：ISP 原则 + Wire 可以为其他消费者注入窄接口。

### D4: SessionUsecase setter 消除 — 构造函数完整化

**选择**：`NewSessionUsecase` 新增 `statusPublisher`/`metricsPublisher`/`runtimeWriter` 三个参数，删除 3 个 `Set*` 方法。

**替代方案**：引入事件总线解耦 — 改动面太大，留待后续。

**理由**：Wire 可以自然解析依赖顺序，setter 是反模式。

### D5: Orchestrator 业务逻辑下沉 — 逐方法迁移

**选择**：
- `checkTeamMemberQuotas` → `biz.UsageUsecase.CheckTeamMemberQuotas()`
- `recordTurnUsage` → `biz.UsageUsecase.RecordTurnUsage()`（新增 `TurnUsageInput` DTO）
- `validateTurnAttachmentCapabilities`/`hasImageAttachment`/`hasFileAttachment` → `provider.ValidateAttachmentCapabilities()`/`provider.HasImageAttachment()`/`provider.HasFileAttachment()`

**替代方案**：在 Orchestrator 内部提取子 struct — 不解决分层违规问题。

**理由**：Orchestrator 应只做 Runner 编排，业务逻辑归属 biz。

### D6: 全局生命周期 context — `pkg/appctx`

**选择**：新增 `pkg/appctx` 包，提供 `Init()`/`Ctx()`/`Cancel()` 三个函数。在 `cmd/admin/main.go` 启动时调用 `Init()`，shutdown 时调用 `Cancel()`。替换所有 `safego.Go(context.Background(), ...)` 为 `safego.Go(appctx.Ctx(), ...)`。

**替代方案**：每个组件自行管理 rootCtx — 分散且容易遗漏。

**理由**：集中管理生命周期，确保所有后台 goroutine 在服务关闭时被取消。

### D7: pack 子包 fmt.Errorf — 分层处理

**选择**：27 处业务操作错误（importer/exporter）改用 `kerrors.BadRequest`；27 处防御性错误（reader tar 安全检查、YAML 解析）保留 `fmt.Errorf`。

**替代方案**：全部改 kerrors — tar 安全检查等内部错误不应暴露为 HTTP 错误码。

**理由**：区分"面向客户端的业务错误"和"面向开发者的防御性错误"。

## Risks / Trade-offs

- **[Risk] Ent 生成物可能引入迁移变更** → 运行 `go generate` 后检查 `internal/data/ent/migrate/` 差异，确认无破坏性迁移
- **[Risk] TeamUsecase 构造函数参数增至 8 个** → 超过 CS-B7（参数 ≤ 5）但 6 个是同层接口，可接受；如后续继续增长则引入 Option struct
- **[Risk] SessionUsecase setter 消除可能影响 Wire 依赖顺序** → Wire 自动解析拓扑排序，但需验证 `wire_gen.go` 生成正确
- **[Risk] appctx 全局状态** → 仅用于服务生命周期 context，不承载业务状态；`Init()` 只在 main.go 调用一次
- **[Trade-off] provider 包新增 3 个导出函数** → `HasImageAttachment`/`HasFileAttachment` 是纯函数，无副作用，可测试性好
