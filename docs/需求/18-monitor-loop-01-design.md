# LOOP-01 系统调试日志闭环 — 设计文档

> **版本**：2026-05-29-v2 | **状态**：🟡 待实施
> **需求**：[`18-monitor-loop-01-requirement.md`](./18-monitor-loop-01-requirement.md)
> **规则真相源**：[`project_rules.md`](../../.trae/rules/project_rules.md) · [`aranea-coding-guide`](../../.trae/skills/aranea-coding-guide.md)

---

## 1. 设计目标

用 FlowLog/SysLog 替代 `fmt.Println`/`log.Printf`，让系统运行信息直接显示在 Monitor Logs 界面，方便开发时定位问题。

**核心思路**：不是新增功能，而是**补全和统一**——把散落在 `log.Printf`、Kratos `log.Helper` 中的调试信息统一收敛到 FlowLog 体系。

---

## 2. 数据流

```
系统运行时
    │
    ├── event.SysLogInfo(stepID, msg, ...Pair)    ← 正确方式
    ├── event.SysLogWarn(stepID, msg, ...Pair)    ← 正确方式
    ├── event.SysLogError(stepID, msg, ...Pair)   ← 正确方式
    │
    ├── log.Printf(...)                            ← ❌ 红线违规，需消除
    ├── w.log.Warnf(...)                           ← ❌ 冗余，需消除
    └── fmt.Println(...)                           ← ❌ 调试残留，需消除
          │
          ▼
    MonitorBus (channel="monitor")
          │
          ├── FlowFileAppender → JSONL 文件落盘
          ├── TraceProjector → monitor_traces 表
          ├── flowLogPersistConsumer → monitor_events 表
          └── WS 推送 → 前端 Monitor Logs 页面
```

---

## 3. 修复方案

### 3.1 FR-01：消除 `log.Printf` 红线违规

#### 3.1.1 `internal/biz/evolution.go`

**当前**：
```go
import "log"

log.Printf("[EVOLUTION] GetToolSuccessRate agent=%s err=%v", agentID, err)
log.Printf("[EVOLUTION] GetRetrievalQuality agent=%s err=%v", agentID, err)
log.Printf("[EVOLUTION] GetEpisodeCount agent=%s err=%v", agentID, err)
log.Printf("[EVOLUTION] GetNegativeFeedbackCount agent=%s err=%v", agentID, err)
```

**修复**：
```go
import "event"

event.SysLogWarn("system.evolution.metrics_fail", "evolution metric query failed",
    event.P("metric", "tool_success_rate"), event.P("agent_id", agentID), event.P("error", err.Error()))
event.SysLogWarn("system.evolution.metrics_fail", "evolution metric query failed",
    event.P("metric", "retrieval_quality"), event.P("agent_id", agentID), event.P("error", err.Error()))
event.SysLogWarn("system.evolution.metrics_fail", "evolution metric query failed",
    event.P("metric", "episode_count"), event.P("agent_id", agentID), event.P("error", err.Error()))
event.SysLogWarn("system.evolution.metrics_fail", "evolution metric query failed",
    event.P("metric", "negative_feedback_count"), event.P("agent_id", agentID), event.P("error", err.Error()))
```

**改动**：移除 `import "log"`，新增 `import "event"`（路径 `aranea-agents/internal/event`）。

#### 3.1.2 `internal/modelcatalog/runner.go`

**当前**：
```go
logger *log.Logger

r.logger.Printf("model-catalog: store resolve failed: %v", err)
r.logger.Printf("model-catalog: schedule check failed: %v", err)
r.logger.Printf("model-catalog: scheduled sync failed: %v", err)
r.logger.Printf("model-catalog: scheduled sync apply failed: %v", applyRes.Errors)
r.logger.Printf("model-catalog: scheduled sync ok providers=%d models=%d policy=%s", ...)
```

**修复**：
```go
event.SysLogWarn("system.model_catalog.resolve_fail", "model catalog store resolve failed",
    event.P("error", err.Error()))
event.SysLogWarn("system.model_catalog.sync_fail", "model catalog schedule check failed",
    event.P("error", err.Error()))
event.SysLogWarn("system.model_catalog.sync_fail", "model catalog scheduled sync failed",
    event.P("error", err.Error()))
event.SysLogWarn("system.model_catalog.sync_fail", "model catalog sync apply failed",
    event.P("errors", fmt.Sprintf("%v", applyRes.Errors)))
event.SysLogInfo("system.model_catalog.sync_ok", "model catalog sync completed",
    event.P("providers", providers), event.P("models", models), event.P("policy", policy))
```

**改动**：移除 `*log.Logger` 字段和 `log.New(...)` 初始化，新增 `import "event"`。注意 `fmt.Sprintf` 仍保留用于 `applyRes.Errors` 的格式化（非红线违规）。

### 3.2 FR-02：清理 cronrunner 双重日志

**模式 A — 已有 FlowLog 的冗余调用**（12 处）：

直接删除 `w.log.*` 行，保留 `event.SysLogWarn`。

```go
// Before:
event.SysLogWarn("memory.l4_decay", "list targets failed", event.P("error", err.Error()))
w.log.Warnf("memory l4 decay: list targets: %v", err)  // ← 删除

// After:
event.SysLogWarn("memory.l4_decay", "list targets failed", event.P("error", err.Error()))
```

**模式 B — 仅有 Kratos 日志的缺口**（17 处）：

先补充 FlowLog，再删除 Kratos 日志。

```go
// Before:
w.log.Infof("memory l4 decay: %d agents, importance=%d", len(targets), importance)  // ← 仅 Kratos

// After:
event.SysLogInfo("memory.l4_decay", "decay completed",
    event.P("agents", len(targets)), event.P("importance", importance))
```

**最终**：移除 `cronrunner` 中所有 `*log.Helper` 字段和构造函数参数。

### 3.3 FR-03：补全 stepTitleRegistry

在 `internal/event/flow_log.go` 的 `stepTitleRegistry` 中新增 22 个条目：

```go
"system.evolution.metrics_fail":    "进化指标查询失败",
"system.model_catalog.resolve_fail": "模型目录解析失败",
"system.model_catalog.sync_fail":    "模型目录同步失败",
"system.model_catalog.sync_ok":      "模型目录同步完成",
"memory.l4_decay":                   "L4 图谱衰减",
"memory.l2_decay":                   "L2 情景衰减",
"memory.l3_decay":                   "L3 事实衰减",
"memory.index_reconcile":            "记忆索引对账",
"memory.dead_letter_replay":         "记忆死信重放",
"memory.data_migration":             "记忆数据迁移",
"memory.episode_backfill":           "情景嵌入回填",
"event_store.cleanup":               "事件存储清理",
"flow_log.cleanup":                  "流程日志清理",
"tool_audit.cleanup":                "工具审计清理",
"channel.delivery":                  "渠道投递",
"channel.health":                    "渠道健康检查",
"provider.health":                   "模型供应商健康检查",
"evolution.scanner":                 "进化扫描",
"monitor.alert_cooldown_cleanup":    "告警冷却清理",
"webresearch.proxy_parse":           "网络研究代理解析",
"knowledge_reflect.eval_fail":       "知识反思评估失败",
"graph.event_bridge":                "图事件桥接",
```

---

## 4. 实施分期

| 阶段 | 内容 | 文件数 | 改动量 |
|------|------|--------|--------|
| **Phase 1** | FR-01：消除 `log.Printf` 红线违规 | 2 | 9 处替换 |
| **Phase 2** | FR-02：清理 cronrunner 双重日志 | 15 | 29 处替换/删除 |
| **Phase 3** | FR-03：补全 stepTitleRegistry | 1 | 22 条注册 |

---

## 5. 验证方案

| 阶段 | 验证命令 |
|------|----------|
| Phase 1 | `go build ./internal/biz/... ./internal/modelcatalog/...` + `go vet` |
| Phase 2 | `go build ./internal/cronrunner/...` + `go vet` |
| Phase 3 | `go build ./internal/event/...` |
| 全量 | `grep -rn 'log\.Printf\|log\.Infof\|log\.Warnf\|log\.Errorf' internal/biz/ internal/modelcatalog/` → 0 结果 |

---

## 6. 远期展望：AI 辅助分析

当 FR-01~03 完成后，系统日志覆盖率和结构化程度将足够支撑 AI 分析。远期可考虑：

1. **AI 日志分析 Agent**：内置 `__system_optimizer__` Agent，读取 JSONL 日志，识别错误模式
2. **代码修复建议**：AI 定位到具体代码文件和行号，生成 diff
3. **人工审批闭环**：AI 建议经人工审批后执行，自动验证

此为独立需求，不在 LOOP-01 范围内，需另行设计。
