## Why

全栈代码深度审查发现 3 个阻断项 + 14 个建议项，涵盖构建失败、前端分层违规、后端架构/OOP/并发/编程规范问题。阻断项导致项目无法编译或违反分层铁律，必须立即修复；建议项影响可维护性和可测试性，需在近期迭代中消解。

## What Changes

### 阻断项（P0）
- 补齐 Ent `experiencereport` 实体生成物，恢复 `go build ./...` 通过
- 修复 `SelfCheckStatusPanel.vue` 展示组件直接 `useMonitorStore` 的分层违规
- 修复 `ProviderLogo.vue` 展示组件直接 `useModelCatalogStore` 的分层违规

### 建议项（P1 — 架构优化）
- 补充 ChatService/ChatOrchestrator 编译期接口断言（`biz.A2ARunnerFactory`）
- TeamUsecase 内部按子接口注入替代聚合接口（TeamRepository 22 方法 → 6 个子接口）
- 消除 SessionUsecase 的 3 个 setter 注入，改为构造函数参数

### 建议项（P2 — 分层与质量）
- Orchestrator 层 `checkTeamMemberQuotas` 业务逻辑下沉到 `biz.UsageUsecase`
- Orchestrator 层 `recordTurnUsage` 业务逻辑下沉到 `biz.UsageUsecase`
- Orchestrator 层 `validateTurnAttachmentCapabilities`/`hasImageAttachment`/`hasFileAttachment` 下沉到 `provider` 包
- 引入 `pkg/appctx` 全局生命周期 context，替换 27 处 `safego.Go(context.Background(), ...)`

### 建议项（P3 — 编程规范）
- `BuildTRPCTeam` 添加 Phase 8 清理标记（非死代码，条件性应急路径）
- `biz/pack` 子包 27 处业务 `fmt.Errorf` 迁移到 `kerrors`
- `biz/monitor/root_cause_engine.go` 4 处 `fmt.Errorf` 迁移到 `kerrors`
- `biz/monitor/self_heal.go` + `self_heal_observer.go` 常量去重到 `heal_constants.go`
- `ChannelTurnDeps` 拆为 `ChannelTurnJobDeps` + `ChannelNotifierDeps`

### 建议项（P3 — 前端 UX）
- 30 处纯 hex 硬编码颜色提取到 CSS 变量 / 主题常量文件

### 建议项（P4 — 文档同步）
- `event/flow_context.go` 注释标记 Deprecated
- `biz/doc.go` 索引补充缺失文件说明

## Capabilities

### New Capabilities
- `app-lifecycle-ctx`: 全局应用生命周期 context（`pkg/appctx`），替代散落的 `context.Background()` 启动后台 goroutine
- `provider-attachment-capabilities`: Provider 附件能力校验（从 Orchestrator 下沉到 provider 包）

### Modified Capabilities
- `team-usecase-injection`: TeamUsecase 依赖注入从聚合接口改为子接口
- `session-usecase-construction`: SessionUsecase 消除 setter 注入，改为完整构造函数
- `usage-usecase-quota`: UsageUsecase 新增 `CheckTeamMemberQuotas` 和 `RecordTurnUsage` 方法
- `frontend-component-isolation`: 前端展示组件消除 Store 反向依赖

## Impact

- **后端 biz 层**：`team_usecase.go`、`session/usecase.go`、`usage/usage.go`、`monitor/self_heal*.go`、`monitor/root_cause_engine.go`、`pack/*.go`
- **后端 service 层**：`chat_orchestrator.go`（ChannelTurnDeps 拆分）、`chat_orchestrator_turn.go`（业务逻辑下沉）、`chat_attachments.go`（校验下沉）
- **后端 provider 层**：新增 `capabilities.go`
- **后端 pkg 层**：新增 `pkg/appctx`
- **后端 data 层**：Ent 生成物补齐
- **后端 Wire**：`cmd/admin/wire.go` 新增子接口绑定
- **前端组件**：`SelfCheckStatusPanel.vue`、`ProviderLogo.vue`
- **前端主题**：CSS 变量文件新增调色板 token
- **文档**：`biz/doc.go`、`event/flow_context.go`

## Non-goals

- ChatService 拆分为 7 个 gateway struct（BA-01 Step2）— 留待下个迭代
- 全面消除 20 个 setter 注入 — 本次只处理 SessionUsecase 的 3 个，其余留待后续
- 前端 ECharts 配色提取 — 涉及第三方库配置，复杂度高，单独迭代处理
- `BuildTRPCTeam` 删除 — Phase 8 再处理
