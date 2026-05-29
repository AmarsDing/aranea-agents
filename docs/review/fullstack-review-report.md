# Aranea-Agents 全栈代码审查报告

> 审查日期：2026-05-29 | 审查工具：aranea-review SKILL | 审查范围：全栈（Go 后端 + Vue 3 前端）

---

## 概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **后端 — 架构合规** | 2 | 1 | 0 | 3 |
| **后端 — 分层合规** | 1 | 3 | 1 | 5 |
| **后端 — OOP** | 2 | 3 | 0 | 5 |
| **后端 — Agent 运行时** | 0 | 1 | 0 | 1 |
| **后端 — 并发安全** | 0 | 0 | 0 | 0 |
| **后端 — 错误处理** | 2 | 1 | 1 | 4 |
| **后端 — 依赖注入** | 0 | 0 | 0 | 0 |
| **前端 — 数据流合规** | 4 | 2 | 0 | 6 |
| **前端 — 组件分层** | 2 | 1 | 0 | 3 |
| **前端 — 业务逻辑归属** | 1 | 2 | 0 | 3 |
| **前端 — 聊天消息分组** | 0 | 0 | 0 | 0 |
| **前端 — UX 主题** | 2 | 2 | 0 | 4 |
| **构建与回归** | 0 | 0 | 0 | 0 |
| **合计** | **16** | **16** | **2** | **34** |

---

## 阻断项（必须修复）

| ID | 维度 | 端 | 文件 | 问题描述 | 修复建议 |
|----|------|----|------|----------|----------|
| BA4-1 | 架构合规 | 后端 | `internal/service/knowledge.go:100-188` | KnowledgeService.IngestDocument 在 Service 层执行 base64 解码、文本提取、chunk 参数校验、异步 goroutine 编排等完整业务流程 | 将编排逻辑下沉到 biz 层 KnowledgeUsecase，Service 层只做 proto→biz 映射 |
| BA4-2 | 架构合规 | 后端 | `internal/service/monitor_notify.go:30-163` | MonitorAlertNotifier 在 Service 层包含 SSRF 防护 + Webhook 发送逻辑 | 将 SSRF 防护和 Webhook 发送逻辑移至 biz 层 Usecase |
| BA4-3 | 架构合规 | 后端 | `internal/service/agent_prompt_ai.go:46-118` | PromptFileAIEditor.Revise 包含完整 LLM 调用编排（模型解析、请求构建、流式响应收集），且 import 了 trpcmodel | 将 LLM 调用编排移至 biz 层，Service 层只做映射 |
| BA6-1 | 架构合规 | 后端 | `internal/service/a2a_endpoint.go:20` | A2AEndpointBuilder 持有 `*ChatService` 具体类型，深度访问 ChatOrchestrator 内部字段 | 改用 biz 级窄接口（如 `biz.NativeTurnGateway`） |
| BA6-2 | 架构合规 | 后端 | `internal/service/session_run_durable_worker.go:18` | SessionRunDurableWorker 持有 `*ChatService` 具体类型 | 改用 biz 级窄接口 |
| BA6-3 | 架构合规 | 后端 | `internal/service/channel_ingress.go:30` | ChannelIngress 持有 `*CronService` 具体类型（chat 字段已正确使用接口） | 改用 biz 级窄接口 |
| BL2-1 | 分层合规 | 后端 | `internal/service/` 多文件（31 处） | Service 层使用 `fmt.Errorf` 返回错误而非 `kerrors` | 统一替换为 `kerrors.BadRequest`/`kerrors.InternalServer` 等 |
| BI3-1 | OOP | 后端 | `internal/biz/monitor/monitor.go:187` | `monitor.Repo` 约 20 个方法，混合审计日志、事件、追踪、告警规则、Runner 指标等职责 | 拆分为 `AuditRepo`, `EventRepo`, `TraceRepo`, `AlertRuleRepo`, `RunnerMetricsRepo` |
| BI3-2 | OOP | 后端 | `internal/biz/team_usecase.go:11` | `TeamRepository` 21 个方法，混合 Team CRUD、TeamRun、OrchestrationStep、DeadLetter | 拆分为 `TeamReader`, `TeamWriter`, `TeamRunRepo`, `OrchestrationStepRepo`, `DeadLetterRepo` |
| BI3-3 | OOP | 后端 | `internal/biz/session/usecase.go:420` | `SessionRepository` 17 个子接口组合，已标记 Deprecated 但仍在使用 | 按职责拆分并移除 Deprecated 标记 |
| BE4-1 | 错误处理 | 后端 | `internal/biz/task.go:191,533` 等 | 约 25 处高风险错误被静默忽略（`_ = someFunc()`） | 至少添加 `FlowLog` 日志记录，关键路径应返回错误 |
| BE4-2 | 错误处理 | 后端 | `internal/data/turn_index_migrate.go:65,127` | 2 处 `log.Println` 调试残留，违反"统一使用 FlowLog"红线 | 替换为 `FlowLog` |
| FD1-1 | 数据流合规 | 前端 | `web/src/components/tools/ToolEditorDialog.vue:313-314` | 展示组件 import `useToolEditorStore` + `useToolDetailStore` | 通过 props 注入或 emit 上报到 composable |
| FD1-2 | 数据流合规 | 前端 | `web/src/components/tools/ToolDetailDrawer.vue:307-308` | 展示组件 import `useToolDetailStore` + `useToolEditorStore` | 通过 props 注入或 emit 上报到 composable |
| FD1-3 | 数据流合规 | 前端 | `web/src/components/tools/ToolOverrideEditorDialog.vue:62` | 展示组件 import `listAgents` API | 通过 props 注入 agent 列表或收敛到 Store |
| FD2-1 | 数据流合规 | 前端 | `web/src/pages/SystemSettingsCatalogTab.vue:249` | Page 直接 import 9 个 API 函数，绕过 Store 层 | 抽到 `useSystemSettingsCatalogTab` composable 或 Store |
| FD4-1 | 数据流合规 | 前端 | `web/src/components/tools/ToolOverrideEditorDialog.vue:92-108` | Dialog 内部 onMounted 直接调 API 获取 agent 列表 | emit('loadAgents') 由 Page/Store 调 API |
| FD4-2 | 数据流合规 | 前端 | `web/src/features/mcp/McpServerFormDialog.vue:188` | Dialog 直接 import 4 个 API 函数 | 迁入 Store action |
| FD4-3 | 数据流合规 | 前端 | `web/src/features/mcp/McpUserCredentialDialog.vue:45` | Dialog 直接 import 3 个 API 函数 | 迁入 Store action |
| FD5-1 | 数据流合规 | 前端 | `web/src/features/memory/` 下 8 个 .vue | 组件内部直接调 API 并持有业务状态 | 迁入 Store/composable |
| FL1-1 | 组件分层 | 前端 | `web/src/features/memory/` 下 16 个 .vue | 展示组件放在 features/ 而非 components/memory/ | 迁移到 `components/memory/` |
| FL1-2 | 组件分层 | 前端 | `web/src/features/mcp/` 下 3 个 .vue | 展示组件放在 features/ 而非 components/mcp/ | 迁移到 `components/mcp/` |
| FU3-1 | UX 主题 | 前端 | `web/src/components/graph/GraphNodePalette.vue:120` | 内联 CSS 只有 `backdrop-filter:blur(8px)`，缺少 `-webkit-backdrop-filter` | 添加 `-webkit-backdrop-filter:blur(8px)` |
| FU4-1 | UX 主题 | 前端 | `web/src/pages/HooksPage.vue:64` | Dialog 使用裸 `<q-card>`，缺失 `app-dialog-card` | 添加 `app-dialog-card` class |
| FU9-1 | UX 主题 | 前端 | `web/src/components/tools/ToolDetailDrawer.vue:224` | 使用裸 `<q-table>` 而非 `AppRegistryTable` | 改用 `AppRegistryTable` |
| FU10-1 | UX 主题 | 前端 | `web/src/components/tools/ToolDetailDrawer.vue:370-374` | 表格列定义未使用 `registryCol` + `REGISTRY_COL_W` | 使用 `registryCol()` 和 `REGISTRY_COL_W` 常量 |

---

## 建议项（推荐修复）

| ID | 维度 | 端 | 文件 | 问题描述 | 修复建议 |
|----|------|----|------|----------|----------|
| BA4-4 | 架构合规 | 后端 | `internal/service/monitor.go:26-27` | MonitorService 持有 `*FlowLogService` + `*CodeExecutorService` 具体类型 | 改用 biz 级窄接口 |
| BL7-1 | 分层合规 | 后端 | `internal/data/` 多文件 | 约 19 个 Repo 缺少编译期接口检查 `var _ biz.XxxRepo = (*xxxRepo)(nil)` | 补充编译期接口检查 |
| BL6-1 | 分层合规 | 后端 | `internal/data/` 多文件 | 部分转换函数使用简写（如 `entToBizMCP` 而非 `entMCPServerToBiz`） | 统一使用 `entXxxToBiz` 全称命名 |
| BL8-1 | 分层合规 | 后端 | `internal/service/chat_orchestrator.go` | ChatOrchestrator 使用结构化依赖组 `ChatOrchestratorDeps`，可接受但需注意依赖数量 | 监控依赖数量，必要时拆分 |
| BI1-1 | OOP | 后端 | `internal/biz/` 多文件 | 14 个接口方法数 > 5（如 `plugin.Repo` 9 个、`AdminRepo` 7 个、`MCPServerRepo` 7 个等） | 按职责域拆分为多个小接口 |
| BI5-1 | OOP | 后端 | `internal/service/chat_orchestrator.go:35,325` | 2 处非必要 `interface{}` 使用（`timestampedEntry.value` 和 `ActiveRunner` 返回值） | 替换为具体类型或泛型 |
| BS1-1 | OOP | 后端 | `internal/data/` 多文件 | 部分 Repo 使用裸 `&xxxRepo{}` 构造 | 统一使用 `NewXxxRepo()` 工厂函数 |
| BR8-1 | Agent 运行时 | 后端 | `internal/tools/toolset.go:500-502` | 记忆工具存在双路径注入：框架标准路径 + Assemble 直注路径 | 统一为通过 `memory.Service.Tools()` 注入 |
| FD3-1 | 数据流合规 | 前端 | `web/src/` 多文件 | `listAgents` 被 8 处独立调用，未收敛到统一 Store | 收敛到 `useAgentsCatalogStore` |
| FD7-1 | 数据流合规 | 前端 | `web/src/stores/index.ts` | 6 个 Store 未在 index.ts 具名导出 | 补充导出 |
| FL3-1 | 组件分层 | 前端 | `web/src/pages/SystemSettingsCatalogTab.vue` | 逻辑过重，直接管理 API 调用和大量状态 | 抽到 composable 或 Store |
| FL5-1 | 组件分层 | 前端 | `web/src/features/` 多文件 | 12+ 处 composable 直接调 API 绕过 Store | 按场景评估，核心 CRUD 应走 Store |
| FB3-1 | 业务逻辑归属 | 前端 | `web/src/stores/tools/toolDetail.ts` | Store 中存在 `$q.notify`，与 composable 重复 | 明确职责划分，消除双重通知 |
| FB4-1 | 业务逻辑归属 | 前端 | `web/src/components/` 多文件 | 6 个展示组件 + 6 个 Page 中直接使用 `$q.notify` | 展示组件通过 emit 上报；Page 移入 composable |
| FU1-1 | UX 主题 | 前端 | `web/src/components/platform/ProviderTrendDialog.vue:146` | fallback `#00e5ff` 霓虹青色，与日间琥珀主题不协调 | 改为琥珀色系 fallback |
| FU4-2 | UX 主题 | 前端 | `web/src/components/` 多文件 | 5 个 Dialog 未使用 `app-dialog-card`（TraceList/A2UIKindContainer/AIRefineButton/TeamRunsDialog/ToolEditorHelpDrawer） | 评估是否应加 `app-dialog-card` |

---

## 提示项（记录备忘）

| ID | 维度 | 端 | 文件 | 描述 |
|----|------|----|------|------|
| BL7-2 | 分层合规 | 后端 | `internal/data/` | 仅 6 个 Repo 有编译期接口检查，其余约 19 个缺失 |
| BE3-1 | 错误处理 | 后端 | `internal/biz/` | 部分错误变量命名未使用 `Err` 前缀 |

---

## 亮点

- **biz 层零框架依赖**：`internal/biz` 不 import `pkg/trpc-agent-go` 任何包，不 import `api/*/v1` proto 包，架构铁律 #1 和 #2 严格遵守
- **Runner 装配收口**：Runner 装配只在 `internal/service`，Server 层无 Runner/Agent 实例化
- **Wire 绑定集中**：所有 `wire.Bind` 集中在 Service 层 `service.go`，biz 层只有 `wire.NewSet`
- **safego 全覆盖**：生产代码中所有 goroutine 均使用 `safego.Go`/`safego.GoRecover`
- **kerrors 在 biz 层统一**：biz 层 0 处 `fmt.Errorf`，所有业务错误使用 `kerrors`
- **聊天消息分组合规**：使用 `turn_id` 权威 FK + `role=user` 边界回退，无 `turn_index` 推算
- **CSS 单一入口**：`app-theme.sass` 聚合所有 partial，无第二套全局 CSS 入口
- **MCP 安全门控**：`AllowAdHocHTTP` 默认 false + `ProductionAllowAdHocHTTP` 双重门控
- **Chat/Team 共用工具装配**：共用 `buildToolsetsForAgent` → `BuildToolsets` → `Assemble` 链路
- **跨 Store 同步规范**：通过 `sessionSync.ts` 事件总线，无 Store 间直接 import
- **Provider 收口**：所有厂商经 `TRPCModelForProviderModel` 统一创建

---

## 后端合规性清单

- [x] 依赖方向向内（biz 不 import data/service/trpc-agent-go/proto）
- [x] Runner 装配在 Service 层
- [ ] Service 层无业务逻辑（3 处违规：KnowledgeService/MonitorAlertNotifier/PromptFileAIEditor）
- [ ] 跨模块通过窄接口（3 处违规：A2AEndpointBuilder/SessionRunDurableWorker/ChannelIngress 持有 Service 具体类型）
- [x] Wire 绑定在 Service 层
- [x] 无工具生成代码的手动修改
- [x] goroutine 走 safego
- [ ] 业务错误用 kerrors（Service 层 31 处 fmt.Errorf）
- [x] 日志用 FlowLog（biz 层 0 处 log/slog；data 层 2 处 log.Println 残留）
- [x] 共享状态有锁保护
- [x] 无上帝对象注入
- [ ] 接口方法 ≤ 5（14 个接口超标）
- [ ] Repository 接口方法 ≤ 5（3 个上帝接口：monitor.Repo/TeamRepository/SessionRepository）

---

## 前端合规性清单

- [ ] 展示组件无 Store/API import（3 处违规：ToolEditorDialog/ToolDetailDrawer/ToolOverrideEditorDialog）
- [ ] Page 无直接 API import（1 处违规：SystemSettingsCatalogTab）
- [ ] Dialog/浮层 emit 而非内部调 API（3 处违规：ToolOverrideEditorDialog/McpServerFormDialog/McpUserCredentialDialog）
- [x] 新 HTTP 调用在 api.ts
- [x] 跨 Store 同步走 sessionSync 事件总线
- [x] 聊天消息分组用堆栈模型（非 turn_index）
- [ ] 浮层 backdrop-filter 成对（1 处违规：GraphNodePalette.vue）
- [x] 主按钮用 --color-accent
- [ ] Dialog 用 app-dialog-card（6 处未使用）
- [ ] Registry 表格用 AppRegistryTable + registryCol()（1 处违规：ToolDetailDrawer）
- [ ] 表格列定义在 *Ui.ts（1 处违规：ToolDetailDrawer）
- [ ] Page script ≤~200 行（SystemSettingsCatalogTab 逻辑过重）
- [ ] 展示组件放 components/<域>/（27 个 .vue 违规放在 features/）

---

## 各维度详细审查结果

### 一、后端架构合规（BA1-BA9）

| # | 检查项 | 结果 | 说明 |
|---|--------|------|------|
| BA1 | Server 层 new Runner/Agent | ✅ 通过 | Server 层无 Runner/Agent 实例化 |
| BA2 | biz import trpc-agent-go | ✅ 通过 | biz 层无框架 import（仅有注释声明） |
| BA3 | biz import proto 包 | ✅ 通过 | biz 层零 proto import |
| BA4 | Service 层写业务逻辑 | ❌ 违规 | 3 处：KnowledgeService 异步编排、MonitorAlertNotifier SSRF+Webhook、PromptFileAIEditor LLM 调用 |
| BA5 | Server 层写业务路由 | ✅ 通过 | 自定义路由均有合理注释，非业务路由 |
| BA6 | 跨模块持有 Service 具体类型 | ❌ 违规 | 3 处：A2AEndpointBuilder/*ChatService、SessionRunDurableWorker/*ChatService、ChannelIngress/*CronService |
| BA7 | Wire 绑定在 Service 层 | ✅ 通过 | 所有 wire.Bind 集中在 service.go |
| BA8 | 修改工具生成代码 | ✅ 通过 | 未发现修改迹象 |
| BA9 | Graph 运行时类型泄漏到 biz | ✅ 通过 | biz 层通过端口接口隔离，无框架类型 |

### 二、后端分层合规（BL1-BL8）

| # | 检查项 | 结果 | 说明 |
|---|--------|------|------|
| BL1 | Service 层 toProto/fromProto | ✅ 通过 | 全面使用命名转换函数 |
| BL2 | Service 层 kerrors | ❌ 违规 | 31 处 fmt.Errorf |
| BL3 | Biz 层纯 Go struct | ✅ 通过 | 无 Ent/proto 依赖 |
| BL4 | Biz 层 Repo 接口定义 | ✅ 通过 | 所有接口在 biz |
| BL5 | Data 层 d.Ent()/d.Postgres() | ✅ 通过 | 84+ 处合规用法 |
| BL6 | Data 层 entXxxToBiz | ✅ 通过 | 部分简写变体 |
| BL7 | Data 层编译期接口检查 | ⚠️ 注意 | 仅 6 个 Repo 有检查，约 19 个缺失 |
| BL8 | Service 构造函数参数 | ✅ 通过 | 无上帝对象 |

### 三、后端 OOP（BI1-BI6, BS1-BS5）

| # | 检查项 | 结果 | 说明 |
|---|--------|------|------|
| BI1 | 接口方法数 ≤ 5 | ❌ 违规 | 14 个接口超标 |
| BI2 | 接口定义在使用方 | ✅ 通过 | 端口接口在 biz |
| BI3 | 上帝接口 | ❌ 违规 | 3 个：monitor.Repo(~20), TeamRepository(21), SessionRepository(17) |
| BI4 | 返回值为具体类型 | ✅ 通过 | -- |
| BI5 | 滥用 interface{} | ❌ 违规 | 2 处非必要 interface{} |
| BI6 | Repository 方法 ≤ 5 | ❌ 违规 | 同 BI1 |

### 四、Agent 运行时合规（BR1-BR14）

| # | 检查项 | 结果 | 说明 |
|---|--------|------|------|
| BR1 | Plugin 回调直接写库 | ✅ 通过 | 回调仅做 log/notify/block/modify |
| BR2 | 绕过 session/trpc 塞 Ent 行 | ✅ 通过 | 通过框架 Service 处理 |
| BR3 | Transport 层解析工具参数 | ✅ 通过 | Server 层仅做路由注册 |
| BR4 | 框架另起 HTTP 监听 | ✅ 通过 | 统一通过 Kratos HTTP Server |
| BR5 | Kratos middleware 复制进框架 | ✅ 通过 | 框架包无 Kratos 依赖 |
| BR6 | 工具先在 Registry 注册 | ✅ 通过 | 17 个工具全部注册 |
| BR7 | Chat/Team 共用 BuildToolsets | ✅ 通过 | 共用装配链路 |
| BR8 | 记忆工具通过 Service.Tools() | ⚠️ 注意 | 双路径注入，功能正确但绕过接口 |
| BR9 | 记忆写入经 async | ✅ 通过 | 所有写入经异步队列 |
| BR10 | Provider 收口 | ✅ 通过 | 经 TRPCModelForProviderModel 统一 |
| BR11 | 契约对齐 model 包 | ✅ 通过 | 全部使用框架 model 类型 |
| BR12 | 流式工具 FinalResultChunk | ✅ 通过 | 框架保障 |
| BR13 | MCP AllowAdHocHTTP 默认 false | ✅ 通过 | 默认 false + 双重门控 |
| BR14 | 工具策略在 biz 解析 | ✅ 通过 | 策略解析在 biz 层 |

### 五、后端并发安全与错误处理

| # | 检查项 | 结果 | 说明 |
|---|--------|------|------|
| BC1 | safego | ✅ 通过 | 生产代码全覆盖 |
| BC3 | 共享状态锁保护 | ✅ 通过 | 100+ 处 Mutex/RWMutex |
| BE1 | biz 层 kerrors | ✅ 通过 | 0 处 fmt.Errorf |
| BE4 | 吞掉错误 | ❌ 违规 | ~25 处高风险吞错 |
| BLG1 | log/slog | ✅ 通过 | 0 处 |
| BLG2 | 调试残留 | ❌ 违规 | 2 处 log.Println |

### 六、后端依赖注入

| # | 检查项 | 结果 | 说明 |
|---|--------|------|------|
| BD1 | Wire ProviderSet 每层一个 | ✅ 通过 | 7 个 ProviderSet |
| BD2 | 手动编辑 wire_gen.go | ✅ 通过 | 未发现 |

### 七、前端数据流合规（FD1-FD8）

| # | 检查项 | 结果 | 说明 |
|---|--------|------|------|
| FD1 | 展示组件 import Store/API | ❌ 违规 | 3 处 |
| FD2 | Page 直接 import api | ❌ 违规 | 1 处 |
| FD3 | 同一数据多处 fetch | ❌ 违规 | listAgents 8 处调用 |
| FD4 | Dialog 内部调 API | ❌ 违规 | 3 处 |
| FD5 | 展示组件 watch+fetch+ref | ❌ 违规 | 8 处 features/ 下 .vue |
| FD6 | HTTP 调用在 api.ts | ✅ 通过 | -- |
| FD7 | Store 在 index.ts 导出 | ⚠️ 注意 | 6 个 Store 未导出 |
| FD8 | 跨 Store 同步走事件总线 | ✅ 通过 | sessionSync.ts |

### 八、前端组件分层（FL1-FL6）

| # | 检查项 | 结果 | 说明 |
|---|--------|------|------|
| FL1 | 展示组件放 components/ | ❌ 违规 | 27 个 .vue 在 features/ |
| FL2 | features/ 只有 api/composable | ❌ 违规 | 同 FL1 |
| FL3 | Page script ≤~200 行 | ⚠️ 注意 | SystemSettingsCatalogTab 过重 |
| FL4 | 组件类型从 types.ts 引入 | ✅ 通过 | -- |
| FL5 | composable 绕过 Store | ⚠️ 注意 | 12+ 处 |
| FL6 | 多 Dialog/Tab 已拆分 | ✅ 通过 | -- |

### 九、前端业务逻辑归属（FB1-FB7）

| # | 检查项 | 结果 | 说明 |
|---|--------|------|------|
| FB1 | 数据转换在 api.ts | ✅ 通过 | -- |
| FB2 | 筛选/排序在 Store/Composable | ✅ 通过 | -- |
| FB3 | 错误处理在 Store action | ⚠️ 注意 | toolDetail/toolEditor Store 与 composable 重复 |
| FB4 | $q.notify 在 Composable/Store | ❌ 违规 | 6 个展示组件 + 6 个 Page 直接使用 |
| FB5 | Agent Panel 由 Page 注入 | ✅ 通过 | -- |
| FB7 | 表格列定义在 *Ui.ts | ✅ 通过 | -- |

### 十、聊天消息分组（FM1-FM5）

| # | 检查项 | 结果 | 说明 |
|---|--------|------|------|
| FM1 | turn_index 分组 | ✅ 通过 | 使用 turn_id 权威 FK |
| FM2 | role=user 边界 | ✅ 通过 | shouldStartNewBlock 回退到 role=user |
| FM3 | in-flight 排序 | ✅ 通过 | 持久化优先 |
| FM4 | created_at 排序 | ✅ 通过 | mergeSessionMessages 正确 |
| FM5 | 推算函数 | ✅ 通过 | 无 deriveTurnKey/inferTurnIndex |

### 十一、UX 主题（FU1-FU10）

| # | 检查项 | 结果 | 说明 |
|---|--------|------|------|
| FU1 | 硬编码 hex | ⚠️ 注意 | ProviderTrendDialog fallback #00e5ff |
| FU2 | 霓虹青紫默认强调 | ✅ 通过 | 主强调琥珀金 |
| FU3 | backdrop-filter 成对 | ❌ 违规 | GraphNodePalette.vue 缺 -webkit |
| FU4 | app-dialog-card | ❌ 违规 | 6 个 Dialog 未使用 |
| FU5 | 第二套 CSS 入口 | ✅ 通过 | 单一入口 |
| FU6 | 运行时改 quasar-variables | ✅ 通过 | 无运行时修改 |
| FU7 | --color-accent | ✅ 通过 | 100+ 处引用 |
| FU8 | positive 绿描边 | ✅ 通过 | 显式打掉 Quasar bg-positive |
| FU9 | AppRegistryTable | ❌ 违规 | ToolDetailDrawer 裸 q-table |
| FU10 | registryCol + REGISTRY_COL_W | ❌ 违规 | ToolDetailDrawer 列定义 |

---

## 修复优先级路线图

### P0 — 架构红线（必须修复）

1. **Service 层业务逻辑下沉**（BA4）：将 KnowledgeService/MonitorAlertNotifier/PromptFileAIEditor 中的业务逻辑移至 biz 层 Usecase
2. **跨模块窄接口化**（BA6）：A2AEndpointBuilder/SessionRunDurableWorker/ChannelIngress 改用 biz 级窄接口
3. **上帝接口拆分**（BI3）：拆分 monitor.Repo/TeamRepository/SessionRepository
4. **Service 层 kerrors 统一**（BL2）：31 处 fmt.Errorf 替换为 kerrors

### P1 — 质量保障（推荐修复）

5. **前端展示组件 Store/API import 清理**（FD1）：ToolEditorDialog/ToolDetailDrawer/ToolOverrideEditorDialog
6. **前端 Dialog 内部 API 调用清理**（FD4）：McpServerFormDialog/McpUserCredentialDialog
7. **前端 features/ 下 .vue 迁移**（FL1）：27 个 .vue 文件迁移到 components/
8. **高风险吞错添加日志**（BE4）：约 25 处
9. **Data 层编译期接口检查补充**（BL7）：约 19 个 Repo
10. **记忆工具注入路径统一**（BR8）：统一为 memory.Service.Tools()

### P2 — 整洁优化（后续迭代）

11. **log.Println 残留清理**（BLG2）：turn_index_migrate.go
12. **interface{} 替换**（BI5）：ChatOrchestrator 中 2 处
13. **listAgents 重复 fetch 收敛**（FD3）
14. **$q.notify 归属整理**（FB4）
15. **Store index.ts 补全导出**（FD7）
16. **UX 主题修复**（FU3/FU4/FU9/FU10）
17. **composable 直接调 API 标注 TECH-DEBT**（FL5）
