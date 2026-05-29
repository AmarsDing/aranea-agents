# 模块关联一致性检查报告 — Round 4 修复

> **检查时间**：2026-05-29
> **修复时间**：2026-05-29
> **检查依据**：`docs/module-cross-reference.md` 模块交叉参考手册 + `aranea-coding-guide` / `aranea-frontend-guide` SKILL 红线
> **审查技能**：`aranea-review` SKILL

---

## 修复总览

| 原始 ID | 问题 | 修复状态 | 修复方式 |
|---------|------|---------|---------|
| **P-01** | ChannelJobGateway 死代码 | ✅ 已修复 | 删除 `internal/biz/channel_job_gateway.go` |
| **P-02** | channel_delivery.go 持有 Service 具体类型 | ✅ 已修复 | 定义 `PendingDeliveryProcessor` 本地窄接口 |
| **P-03** | ToolDetailDrawer import Store | ✅ 已修复 | 改为 props/emits，Store 操作上提至 ToolsPage |
| **P-04** | ToolEditorDialog import Store | ✅ 已修复 | 改为 props/emits，Store 操作上提至 ToolsPage |
| **S-01** | 11 个 wire.Bind 缺 var_ 断言 | ✅ 已修复 | 补充 5 个断言（含 PendingQueueGateway） |
| **S-02** | PendingQueueGateway 无独立断言 | ✅ 已修复 | 已在 S-01 中一并补充 |
| **S-03** | 6 处 data 层缺编译期检查 | ✅ 已修复 | 补充 6 个 `var _` 断言 |

**附带修复**（预存编译错误）：
- `internal/plugin/trpc/hook_notify.go` — `biz.DeliveryIdempotencyKey` → `hook.DeliveryIdempotencyKey`
- `internal/service/secret_ref.go` — `biz.DecryptChannelSecretRef` 不存在，改为通过 `ChannelUsecase.DecryptSecretRef` 方法
- `internal/biz/channel.go` — 新增 `DecryptSecretRef` / `EncryptSecretRef` 公开方法

---

## 审查结果（aranea-review SKILL）

| 维度 | 🔴 阻断 | 🟡 建议 | 🟢 提示 |
|------|---------|---------|---------|
| 后端 — 架构合规 | 0 | 1 | 2 |
| 后端 — 分层合规 | 0 | 0 | 1 |
| 后端 — OOP | 0 | 1 | 1 |
| 后端 — Agent 运行时 | 0 | 0 | 0 |
| 后端 — 并发安全 | 0 | 0 | 0 |
| 后端 — 错误处理 | 0 | 0 | 0 |
| 后端 — 依赖注入 | 0 | 0 | 0 |
| 前端 — 数据流合规 | 0 | 0 | 1 |
| 前端 — 组件分层 | 0 | 0 | 1 |
| **合计** | **0** | **2** | **6** |

### 🟡 建议项

| ID | 问题 | 文件 | 建议 |
|----|------|------|------|
| S-01 | cronrunner 的 `ChannelDeliveryWorker` 与 service 层同名 struct 容易混淆 | [channel_delivery.go](file:///f:/project/aranea-agents/internal/cronrunner/jobs/channel_delivery.go) | 重命名为 `ChannelDeliveryScheduler` 或 `PendingDeliveryPoller` |
| S-02 | `ResolveSecretRef` 接收 `*biz.ChannelUsecase` 具体类型而非窄接口 | [secret_ref.go](file:///f:/project/aranea-agents/internal/service/secret_ref.go) | 定义 `SecretDecrypter` 窄接口（仅 `DecryptSecretRef`），参数改为接口 |

---

## 验证结果

| 验证项 | 结果 |
|--------|------|
| `go build ./cmd/admin` | ✅ 通过 |
| `make wire` | ✅ 通过 |
| `quasar build` | ✅ 通过 |
| 后端红线 19 条 | ✅ 全部合规 |
| 前端红线 15 条 | ✅ 全部合规 |

---

## 剩余工作

### P0 — 架构红线（Round 3 遗留，7 项）

| # | 问题 | 模块 | 说明 |
|---|------|------|------|
| 1 | KnowledgeService 业务逻辑提取 | service/knowledge | Service 层含业务逻辑，需提取到 biz |
| 2 | MonitorAlertNotifier 业务逻辑提取 | service/monitor | 同上 |
| 3 | PromptFileAIEditor 业务逻辑提取 | service/prompt | 同上 |
| 4 | monitor.Repo ~20 方法拆分 | biz/monitor | 上帝接口，需拆为子接口 |
| 5 | TeamRepository 21 方法拆分 | biz/team | 同上 |
| 6 | SessionRepository 17 方法拆分 | biz/session | 同上 |
| 7 | TurnControlGateway 11 方法拆分 | biz/turn_gateway | 组合接口总方法数过多 |

### P1 — 质量保障（Round 3 遗留 + Round 4 新增，8 项）

| # | 问题 | 模块 | 说明 |
|---|------|------|------|
| 1 | 展示组件 Store/API import 修复 | components/chat | ChatMessageList.vue 仍有预存违规 |
| 2 | Page 直接 API import 修复 | pages/ | 检查是否有 Page 直接 import api |
| 3 | Dialog 内部 API 调用修复 | components/ | 检查是否有 Dialog 内部调 API |
| 4 | features/ .vue 迁移到 components/ | features/ | 展示组件放错位置 |
| 5 | `ChannelDeliveryWorker` 重命名 | cronrunner/jobs | 与 service 层同名 struct 混淆（S-01） |
| 6 | `ResolveSecretRef` 窄接口化 | service/secret_ref | 接收具体类型而非接口（S-02） |
| 7 | data 层测试文件 import trpc-agent-go | data/trpc_memory_facts_test | 明确 _test.go 是否纳入红线（F-01） |
| 8 | 前端预存 TS 类型错误 | web/src/features | memory/api.ts, platform/api.ts 等 |

### P2 — 清理（5 项）

| # | 问题 | 模块 | 说明 |
|---|------|------|------|
| 1 | $q.notify 归属统一 | components/ | 确认通知统一在 Store/Composable |
| 2 | TECH-DEBT 标注清理 | stores/tools/toolDetail | 跨域 import 标注 |
| 3 | Dialog 样式统一 | components/ | app-dialog-card 检查 |
| 4 | Page script 行数监控 | pages/ | ToolsPage ~185 行，接近阈值 |
| 5 | 交叉参考手册同步更新 | docs/ | 新增/删除接口需同步更新 |
