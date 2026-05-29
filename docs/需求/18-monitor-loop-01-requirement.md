# LOOP-01 系统调试日志闭环 — 需求文档

> **版本**：2026-05-29-v2 | **状态**：🟡 待实施 | **优先级**：P2
> **关联**：[`18-monitor-development.md`](./18-monitor-development.md) · [`18-monitor-ai-closed-loop-2026-05-28.md`](./18-monitor-ai-closed-loop-2026-05-28.md)
> **设计**：[`18-monitor-loop-01-design.md`](./18-monitor-loop-01-design.md)

---

## 1. 需求原文

> "通过后台的 logs 日志，记录服务的所有运行状态，AI 可以根据日志运行的记录文件追踪到问题，定位问题，形成闭环。"

**用户澄清（2026-05-29）**：

> "通过 monitor 模块，在系统运行的各个节点上打上输出日志到前端的 Logs 监控界面，相当于系统的调试信息，通过这个信息，方便我开发这个系统时定位问题。不用在 fmt 去打日志输出信息了。"

**核心意图**：用 FlowLog/SysLog 替代 `fmt.Println`/`log.Printf`，让系统运行信息直接显示在 Monitor Logs 界面，方便开发时定位问题。

---

## 2. 需求拆解

### 2.1 核心诉求

| 子需求 | 含义 | 当前状态 |
|--------|------|----------|
| **系统各节点有调试日志** | 关键运行路径都有 FlowLog 输出 | 🟡 ~80 个 step_id 已注册，但仍有缺口（见 §4） |
| **调试日志显示在前端** | Monitor Logs 页面实时展示系统运行状态 | ✅ WS 推送 + FlowFileAppender 落盘 |
| **替代 fmt/log 调试** | 开发者不再需要 `fmt.Println`/`log.Printf` | ❌ 仍有 9 处 `log.Printf` + 29 处 Kratos `log.Helper` |
| **AI 辅助分析** | AI 读取日志，分析问题，给出修复/优化建议 | ❌ 当前无 AI 分析能力（远期目标） |

### 2.2 用户角色与场景

| 角色 | 场景 | 期望 |
|------|------|------|
| **系统开发者** | 开发时 Provider 调用超时，需要知道哪个 Provider、哪个模型、耗时多少 | 在 Monitor Logs 界面看到 `system.provider.ha_failover` 日志，含 provider/model/duration_ms |
| **系统开发者** | Agent 运行异常，需要追踪完整执行链路 | 在 Monitor Logs 界面按 trace_id 过滤，看到完整的 start→done/error 链 |
| **系统开发者** | 定时任务执行失败，需要知道哪个任务、失败原因 | 在 Monitor Logs 界面看到 `system.cron.*` 日志，含 error_message |
| **系统开发者** | 想了解系统当前运行状态，不想加 `fmt.Println` | 直接打开 Monitor Logs 页面，实时观察系统行为 |

---

## 3. 业务价值

### 3.1 解决的痛点

| 痛点 | 现状 | LOOP-01 后 |
|------|------|-----------|
| 开发调试靠 `fmt.Println` | 临时加日志 → 忘记删除 → 污染代码 | FlowLog 结构化输出，无需临时日志 |
| `log.Printf` 看不到 | 日志打到 stdout，需要 SSH 到服务器查看 | Monitor Logs 界面实时展示 |
| 关键路径无日志 | 部分模块（evolution、modelcatalog）无 FlowLog | 全路径覆盖，无盲区 |
| 日志无结构 | `log.Printf` 输出自由格式，难以过滤 | FlowLog 有 step_id/severity/trace_id，可精确过滤 |
| 双重日志 | cronrunner 同时写 FlowLog + Kratos log.Helper | 统一为 FlowLog，消除冗余 |

### 3.2 价值量化目标

| 指标 | 目标 |
|------|------|
| 系统关键路径 FlowLog 覆盖率 | ≥ 95% |
| `log.Printf`/`log.Infof` 在 biz/service 层 | 0 处 |
| step_id 注册率 | 100%（使用的 step_id 全部注册中文标题） |
| 开发者使用 Monitor Logs 定位问题 | 替代 80% 的 `fmt.Println` 调试 |

---

## 4. 当前缺口（扫描结果）

### 4.1 P0 — 红线违规：biz 层使用 `log.Printf`

| 文件 | 行号 | 代码 | 应替换为 |
|------|------|------|----------|
| `internal/biz/evolution.go` | 80 | `log.Printf("[EVOLUTION] GetToolSuccessRate ...")` | `event.SysLogWarn("system.evolution.metrics_fail", ...)` |
| `internal/biz/evolution.go` | 84 | `log.Printf("[EVOLUTION] GetRetrievalQuality ...")` | 同上 |
| `internal/biz/evolution.go` | 88 | `log.Printf("[EVOLUTION] GetEpisodeCount ...")` | 同上 |
| `internal/biz/evolution.go` | 92 | `log.Printf("[EVOLUTION] GetNegativeFeedbackCount ...")` | 同上 |

### 4.2 P0 — 红线违规：modelcatalog 使用 `log.Logger`

| 文件 | 行号 | 代码 | 应替换为 |
|------|------|------|----------|
| `internal/modelcatalog/runner.go` | 59 | `r.logger.Printf("model-catalog: store resolve failed: %v", err)` | `event.SysLogWarn("system.model_catalog.resolve_fail", ...)` |
| `internal/modelcatalog/runner.go` | 65 | `r.logger.Printf("model-catalog: schedule check failed: %v", err)` | `event.SysLogWarn("system.model_catalog.sync_fail", ...)` |
| `internal/modelcatalog/runner.go` | 73 | `r.logger.Printf("model-catalog: scheduled sync failed: %v", err)` | 同上 |
| `internal/modelcatalog/runner.go` | 77 | `r.logger.Printf("model-catalog: scheduled sync apply failed: %v", ...)` | 同上 |
| `internal/modelcatalog/runner.go` | 79 | `r.logger.Printf("model-catalog: scheduled sync ok ...")` | `event.SysLogInfo("system.model_catalog.sync_ok", ...)` |

### 4.3 P1 — 冗余日志：cronrunner 15 个文件同时写 FlowLog + Kratos log.Helper

29 处 `w.log.*` 调用中：
- **12 处冗余**：已有 `event.SysLogWarn`，Kratos 日志可直接删除
- **17 处缺口**：仅有 Kratos 日志，需补充 FlowLog 后再删除

| 文件 | 冗余处 | 缺口处 |
|------|--------|--------|
| `memory_l4_decay.go` | 1 | 1 |
| `memory_fact_index_reconciler.go` | 1 | 1 |
| `memory_dead_letter_replayer.go` | 1 | 1 |
| `channel_delivery.go` | 0 | 1 |
| `monitor_alert_cooldown.go` | 0 | 1 |
| `memory_l2_decay.go` | 2 | 2 |
| `memory_l3_decay.go` | 2 | 2 |
| `memory_data_migration.go` | 1 | 2 |
| `memory_episode_backfill.go` | 0 | 1 |
| `event_store_cleanup.go` | 1 | 1 |
| `evolution_scanner.go` | 0 | 1 |
| `flow_log_cleanup.go` | 1 | 1 |
| `provider_health.go` | 0 | 1 |
| `tool_audit_cleanup.go` | 1 | 1 |
| `channel_health.go` | 0 | 1 |
| **合计** | **12** | **17** |

### 4.4 P2 — stepTitleRegistry 缺口：18 个已使用但未注册的 step_id

| step_id | 使用位置 | 建议中文标题 |
|---------|---------|------------|
| `system.evolution.metrics_fail` | evolution.go | 进化指标查询失败 |
| `system.model_catalog.resolve_fail` | modelcatalog/runner.go | 模型目录解析失败 |
| `system.model_catalog.sync_fail` | modelcatalog/runner.go | 模型目录同步失败 |
| `system.model_catalog.sync_ok` | modelcatalog/runner.go | 模型目录同步完成 |
| `memory.l4_decay` | memory_l4_decay.go | L4 图谱衰减 |
| `memory.l2_decay` | memory_l2_decay.go | L2 情景衰减 |
| `memory.l3_decay` | memory_l3_decay.go | L3 事实衰减 |
| `memory.index_reconcile` | memory_fact_index_reconciler.go | 记忆索引对账 |
| `memory.dead_letter_replay` | memory_dead_letter_replayer.go | 记忆死信重放 |
| `memory.data_migration` | memory_data_migration.go | 记忆数据迁移 |
| `memory.episode_backfill` | memory_episode_backfill.go | 情景嵌入回填 |
| `event_store.cleanup` | event_store_cleanup.go | 事件存储清理 |
| `flow_log.cleanup` | flow_log_cleanup.go | 流程日志清理 |
| `tool_audit.cleanup` | tool_audit_cleanup.go | 工具审计清理 |
| `channel.delivery` | channel_delivery.go | 渠道投递 |
| `channel.health` | channel_health.go | 渠道健康检查 |
| `provider.health` | provider_health.go | 模型供应商健康检查 |
| `evolution.scanner` | evolution_scanner.go | 进化扫描 |
| `monitor.alert_cooldown_cleanup` | monitor_alert_cooldown.go | 告警冷却清理 |
| `webresearch.proxy_parse` | tools/webresearch/http_client.go | 网络研究代理解析 |
| `knowledge_reflect.eval_fail` | tools/knowledge/tool.go | 知识反思评估失败 |
| `graph.event_bridge` | graph/trpc/event_bridge.go | 图事件桥接 |

---

## 5. 功能需求

### 5.1 FR-01：消除 `log.Printf`/`log.Infof` 红线违规

- 将 `internal/biz/evolution.go` 的 4 处 `log.Printf` 替换为 `event.SysLogWarn`
- 将 `internal/modelcatalog/runner.go` 的 5 处 `r.logger.Printf` 替换为 `event.SysLogWarn/SysLogInfo`
- 移除 `import "log"` 和 `*log.Logger` 依赖

### 5.2 FR-02：清理 cronrunner 双重日志

- 12 处冗余 Kratos `log.Helper` 调用直接删除
- 17 处缺口先补充 `event.SysLogInfo/SysLogWarn`，再删除 Kratos 日志
- 最终移除 cronrunner 中 `*log.Helper` 字段

### 5.3 FR-03：补全 stepTitleRegistry

- 在 `internal/event/flow_log.go` 的 `stepTitleRegistry` 中注册 22 个缺失 step_id
- 确保前端 Monitor Logs 界面显示中文标题

### 5.4 FR-04（远期）：AI 辅助分析

- AI 读取系统日志，分析错误模式，给出修复/优化建议
- 此为远期目标，不在本需求实施范围内
- 前置条件：FR-01~03 完成后，日志覆盖率和结构化程度足够 AI 分析

---

## 6. 验收标准

- [ ] `internal/biz/` 中 0 处 `log.Printf`/`log.Infof`/`log.Warnf`/`log.Errorf`
- [ ] `internal/modelcatalog/` 中 0 处 `log.Logger.Printf`
- [ ] `internal/cronrunner/jobs/` 中 0 处 Kratos `log.Helper` 调用
- [ ] 所有已使用的 step_id 在 `stepTitleRegistry` 中有中文标题
- [ ] `go build ./internal/...` 通过
- [ ] `go vet ./internal/...` 通过

---

## 7. 不在本需求范围

| 项 | 理由 |
|----|------|
| AI 自动分析日志 | 远期目标，前置条件是日志覆盖率达标 |
| AI 修改代码 | 远期目标，需独立设计安全审批机制 |
| 前端闭环 UI | 无闭环概念，本需求仅涉及日志覆盖 |
| `internal/data/data.go` 启动日志 | 启动阶段 FlowLog 未初始化，可接受 |
| `internal/cli/` CLI 日志 | CLI 工具不在服务器运行时路径上 |

---

## 8. 与已有功能的关系

| 已有功能 | 关系 |
|----------|------|
| LOG-03（P0/P1/P2 红线修复） | **延续**：LOOP-01 是 LOG-03 的扩展，覆盖更多模块 |
| FlowFileAppender（LOG-01） | **消费者**：FlowLog 输出后由 FlowFileAppender 落盘 |
| Monitor Logs 前端页面 | **展示层**：所有 FlowLog 通过 WS 推送到前端 |
| `stepTitleRegistry` | **注册层**：新增 step_id 需在此注册中文标题 |
