# Aranea-Agents 全栈代码审查报告（第二轮）

> 审查日期：2026-05-29 | 审查工具：aranea-review SKILL | 审查范围：第一轮修复后的代码变更

---

## 第一轮修复总结

### 已修复项

| 原ID | 维度 | 修复内容 | 状态 |
|------|------|----------|------|
| BA6-1 | 架构合规 | A2AEndpointBuilder 改用 `biz.A2ARunnerFactory` 窄接口，不再持有 `*ChatService` | ✅ 已修复 |
| BA6-2 | 架构合规 | SessionRunDurableWorker 改用 `biz.TurnGateway` + `biz.DurableResumeGateway` | ✅ 已修复 |
| BA6-3 | 架构合规 | ChannelIngress 改用 `biz.CronTriggerGateway`，移除 cronv1 proto 依赖 | ✅ 已修复 |
| BL2-1 | 分层合规 | Service 层 26 处 `fmt.Errorf` 替换为 `kerrors` | ✅ 已修复 |
| BE4-1 | 错误处理 | 约 15 处高风险吞错添加 `event.SysLogWarn` 日志 | ✅ 已修复 |
| BE4-2 | 错误处理 | `turn_index_migrate.go` 2 处 `log.Println` 替换为 `event.SysLogInfo`/`event.SysLogWarn` | ✅ 已修复 |
| BI5-1 | OOP | `ChatOrchestrator` 中 `interface{}` 替换为 `any`/`trpcrunner.Runner` 具体类型 | ✅ 已修复 |
| FU3-1 | UX 主题 | `GraphNodePalette.vue` 添加 `-webkit-backdrop-filter` | ✅ 已修复 |
| FU4-1 | UX 主题 | `HooksPage.vue` Dialog 添加 `app-dialog-card` class | ✅ 已修复 |
| FU9-1 | UX 主题 | `ToolDetailDrawer.vue` 裸 `q-table` 替换为 `AppRegistryTable` | ✅ 已修复 |
| FU10-1 | UX 主题 | `ToolDetailDrawer.vue` 列定义改用 `registryCol` + `REGISTRY_COL_W` | ✅ 已修复 |

### 新增文件

| 文件 | 说明 |
|------|------|
| `internal/biz/a2a.go` | `A2ARunnerFactory` 接口定义 |
| `internal/service/cron_trigger_gateway_adapter.go` | `CronTriggerGateway` 适配器 |

### 新增接口

| 接口 | 方法数 | 定义位置 | 用途 |
|------|--------|----------|------|
| `biz.DurableResumeGateway` | 1 | `internal/biz/turn_gateway.go` | Durable session resume |
| `biz.CronTriggerGateway` | 2 | `internal/biz/cron.go` | Cron 任务触发与查询 |
| `biz.A2ARunnerFactory` | 1 | `internal/biz/a2a.go` | A2A Agent Runner 构建 |

---

## 第二轮审查结果

### 概要

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 | 合计 |
|------|---------|---------|---------|------|
| **后端 — 架构合规** | 0 | 1 | 0 | 1 |
| **后端 — 分层合规** | 0 | 1 | 0 | 1 |
| **后端 — OOP** | 1 | 1 | 0 | 2 |
| **后端 — Agent 运行时** | 0 | 0 | 0 | 0 |
| **后端 — 并发安全** | 0 | 0 | 0 | 0 |
| **后端 — 错误处理** | 0 | 1 | 1 | 2 |
| **后端 — 依赖注入** | 0 | 0 | 0 | 0 |
| **前端 — 数据流合规** | 1 | 0 | 0 | 1 |
| **前端 — 组件分层** | 0 | 0 | 0 | 0 |
| **前端 — 业务逻辑归属** | 0 | 0 | 0 | 0 |
| **前端 — 聊天消息分组** | 0 | 0 | 0 | 0 |
| **前端 — UX 主题** | 0 | 0 | 1 | 1 |
| **构建与回归** | 0 | 0 | 0 | 0 |
| **合计** | **2** | **4** | **2** | **8** |

### 阻断项（必须修复）

| ID | 维度 | 端 | 文件 | 问题描述 | 修复建议 |
|----|------|----|------|----------|----------|
| R-01 | OOP (BI1) | 后端 | `internal/biz/turn_gateway.go:12-33` | `TurnGateway` 接口有 7 个方法，超过上限 5。`TurnControlGateway` 嵌入 `TurnGateway` 后总计 11 个方法 | 拆分为 `TurnExecutorGateway`（ExecuteTurn/RunNativeTurn/RunNativeTurnWithOutcome）+ `TurnRunControlGateway`（HasActiveRun/CancelRun/SetRunStatus/LastPendingMessageID） |
| R-04 | 数据流合规 (FD1) | 前端 | `web/src/components/tools/ToolOverrideEditorDialog.vue:62` | 展示组件直接 import `listAgents` from `features/agents/api` | 将 `listAgents` 调用上收到 Store action 或 composable，组件通过 props 接收 agent 列表 |

### 建议项（推荐修复）

| ID | 维度 | 端 | 文件 | 问题描述 | 修复建议 |
|----|------|----|------|----------|----------|
| S-01 | 架构合规 (BA6) | 后端 | `internal/service/channel_ingress.go:27` | `chat` 字段类型为 `biz.NativeTurnGateway`，但实际调用了 `ActiveSessionRunPhase`（不在接口中），运行时依赖具体实现 | 将 `chat` 字段类型改为 `TurnControlGateway` 或组合接口 |
| S-04 | OOP (BI5) | 后端 | `internal/service/chat_orchestrator.go:36` | `timestampedEntry.value` 类型为 `any`，缺乏类型安全 | 使用泛型或为不同用途定义具体类型 |
| S-05 | 错误处理 (BE4) | 后端 | `internal/biz/task.go:368` | `AddTaskComment` 的 error 被吞掉 | 添加 `event.SysLogWarn` |
| S-07 | 架构合规 (BA9) | 后端 | `internal/service/a2a_endpoint.go:40-123` | `ChatService.BuildA2ARunner` 方法体超过 80 行 | 提取 Agent 构建逻辑到 `internal/agent` 包 |

### 提示项（记录备忘）

| ID | 维度 | 端 | 文件 | 描述 |
|----|------|----|------|------|
| T-01 | 错误处理 (BE3) | 后端 | `internal/biz/task.go` | biz 层使用 `kerrors` + `fmt.Sprintf` 构造错误消息，可接受 |
| T-02 | UX 主题 (FU3) | 前端 | `GraphNodePalette.vue:120` | drag ghost 已正确添加 `-webkit-backdrop-filter` 成对声明 |

---

## 剩余工作清单

### P0 — 架构红线（必须修复，需较大重构）

| # | 原ID | 问题 | 工作量评估 | 说明 |
|---|------|------|-----------|------|
| 1 | BA4-1 | KnowledgeService.IngestDocument 业务逻辑下沉 | 大 | 需新建 `KnowledgeIngestUsecase`，将 base64 解码/文本提取/chunk 校验/异步编排逻辑全部移至 biz 层 |
| 2 | BA4-2 | MonitorAlertNotifier SSRF+Webhook 下沉 | 中 | 需新建 `AlertNotificationUsecase`，将 SSRF 防护和 Webhook 发送逻辑移至 biz 层 |
| 3 | BA4-3 | PromptFileAIEditor LLM 编排下沉 | 中 | 需新建 `PromptRefineUsecase`，将 LLM 调用编排移至 biz 层，Service 层只做映射 |
| 4 | BI3-1 | monitor.Repo 上帝接口拆分（~20 方法） | 大 | 拆分为 AuditRepo/EventRepo/TraceRepo/AlertRuleRepo/RunnerMetricsRepo 5+ 个子接口 |
| 5 | BI3-2 | TeamRepository 上帝接口拆分（21 方法） | 大 | 拆分为 TeamReader/TeamWriter/TeamRunRepo/OrchestrationStepRepo/DeadLetterRepo |
| 6 | BI3-3 | SessionRepository Deprecated 接口清理（17 方法） | 大 | 按职责拆分并移除 Deprecated 标记 |
| 7 | R-01 | TurnGateway 接口方法数超标（7 > 5） | 中 | 拆分为 TurnExecutorGateway + TurnRunControlGateway |

### P1 — 质量保障（推荐修复）

| # | 原ID | 问题 | 工作量评估 |
|---|------|------|-----------|
| 8 | FD1-1/2 | ToolEditorDialog/ToolDetailDrawer 展示组件 import Store | 中 |
| 9 | FD1-3 | ToolOverrideEditorDialog 展示组件 import API（R-04 阻断项） | 小 |
| 10 | FD2-1 | SystemSettingsCatalogTab Page 直接 import API | 中 |
| 11 | FD4-2/3 | McpServerFormDialog/McpUserCredentialDialog 内部调 API | 中 |
| 12 | FD5-1 | features/memory/ 下 8 个 .vue 组件内部调 API | 大 |
| 13 | FL1-1 | features/memory/ 下 16 个 .vue 迁移到 components/memory/ | 大 |
| 14 | FL1-2 | features/mcp/ 下 3 个 .vue 迁移到 components/mcp/ | 中 |
| 15 | FD3-1 | listAgents 8 处重复 fetch 收敛到 useAgentsCatalogStore | 中 |
| 16 | BL7-1 | Data 层约 19 个 Repo 补充编译期接口检查 | 小 |
| 17 | BR8-1 | 记忆工具注入路径统一为 memory.Service.Tools() | 中 |
| 18 | S-01 | ChannelIngress.chat 字段类型改为 TurnControlGateway | 小 |

### P2 — 整洁优化（后续迭代）

| # | 原ID | 问题 | 工作量评估 |
|---|------|------|-----------|
| 19 | FB4-1 | 6 个展示组件 + 6 个 Page 中 $q.notify 归属整理 | 中 |
| 20 | FD7-1 | 6 个 Store 补充 index.ts 具名导出 | 小 |
| 21 | FL5-1 | 12+ 处 composable 直接调 API 标注 TECH-DEBT | 小 |
| 22 | FU4-2 | 5 个 Dialog 评估是否加 app-dialog-card | 小 |
| 23 | FU1-1 | ProviderTrendDialog fallback #00e5ff 改为琥珀色 | 小 |
| 24 | S-04 | ChatOrchestrator timestampedEntry.value any → 泛型/具体类型 | 中 |
| 25 | S-07 | ChatService.BuildA2ARunner 方法提取到 agent 包 | 中 |
| 26 | FB3-1 | toolDetail/toolEditor Store 与 composable 重复 $q.notify | 中 |

---

## 修复进度统计

| 优先级 | 总计 | 已修复 | 剩余 | 完成率 |
|--------|------|--------|------|--------|
| P0（架构红线） | 10 | 3 | 7 | 30% |
| P1（质量保障） | 11 | 5 | 6 | 45% |
| P2（整洁优化） | 8 | 3 | 5 | 38% |
| **合计** | **29** | **11** | **18** | **38%** |

### 按维度统计

| 维度 | 修复前阻断 | 已修复 | 剩余阻断 |
|------|-----------|--------|---------|
| 后端 — 架构合规 | 5 | 3 | 2（BA4-1/2/3 Service 层业务逻辑） |
| 后端 — 分层合规 | 1 | 1 | 0 |
| 后端 — OOP | 2 | 0 | 2（BI3 上帝接口 + R-01 TurnGateway） |
| 后端 — 错误处理 | 2 | 2 | 0 |
| 前端 — 数据流合规 | 4 | 0 | 4（FD1/FD2/FD4/FD5） |
| 前端 — UX 主题 | 2 | 2 | 0 |

---

## 后端合规性清单（当前状态）

- [x] 依赖方向向内（biz 不 import data/service/trpc-agent-go/proto）
- [x] Runner 装配在 Service 层
- [ ] Service 层无业务逻辑（3 处违规：KnowledgeService/MonitorAlertNotifier/PromptFileAIEditor）
- [x] 跨模块通过窄接口（DurableResumeGateway/CronTriggerGateway/A2ARunnerFactory）
- [x] Wire 绑定在 Service 层
- [x] 无工具生成代码的手动修改
- [x] goroutine 走 safego
- [x] 业务错误用 kerrors（Service 层 fmt.Errorf 已全部迁移）
- [x] 日志用 FlowLog（turn_index_migrate.go 已迁移）
- [x] 共享状态有锁保护
- [x] 无上帝对象注入
- [ ] 接口方法 ≤ 5（TurnGateway 7 方法需拆分）
- [ ] Repository 接口方法 ≤ 5（3 个上帝接口需拆分）

## 前端合规性清单（当前状态）

- [ ] 展示组件无 Store/API import（3 处违规）
- [ ] Page 无直接 API import（1 处违规）
- [ ] Dialog/浮层 emit 而非内部调 API（3 处违规）
- [x] 新 HTTP 调用在 api.ts
- [x] 跨 Store 同步走 sessionSync 事件总线
- [x] 聊天消息分组用堆栈模型（非 turn_index）
- [x] 浮层 backdrop-filter 成对
- [x] 主按钮用 --color-accent
- [x] Dialog 用 app-dialog-card（HooksPage 已修复，5 个待评估）
- [x] Registry 表格用 AppRegistryTable + registryCol()
- [x] 表格列定义在 *Ui.ts
- [ ] Page script ≤~200 行（SystemSettingsCatalogTab 过重）
- [ ] 展示组件放 components/<域>/（27 个 .vue 违规放在 features/）
