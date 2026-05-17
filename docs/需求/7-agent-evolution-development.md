# Agent 进化 — 开发计划

> **版本**：2026-05-17 | **状态**：🟡 EvolutionScanner 未实现
> **需求**：[7 agent-evolution.md](./7%20agent-evolution.md) · **设计**：[7 agent-evolution.design.md](./7%20agent-evolution.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：EP-BIZ-07

---

## 1. 模块定位

Agent 自我进化：基于运行指标和反馈，自动或半自动地优化 Agent 的提示、工具配置和知识。

**代码锚点**：
- `internal/biz/evolution.go` — EvolutionUsecase（GetEvolutionMetrics / GetEvolutionSuggestions / ApplySuggestion）
- `internal/data/ent/schema/evolution_suggestion.go` — Ent Schema
- `internal/service/agent_evolution.go` — Evolution RPC
- `internal/agent/trpc_build.go` — `SelfEvolve` / `EvolutionSelfEvolve` 开关
- `internal/biz/agent_catalog_legacy.go` — `AgentRuntimeSettings` 定义

---

## 2. 现状评估

### 2.1 后端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 进化指标查询 RPC | ✅ | `GetEvolutionMetrics` RPC |
| 进化建议列表 RPC | ✅ | `GetEvolutionSuggestions` RPC |
| 应用/拒绝建议 RPC | ✅ | `ApplySuggestion` / `RejectSuggestion` RPC |
| EvolutionSuggestion 表 | ✅ | Ent schema + data repo |
| 进化开关字段 | ✅ | `AgentRuntimeSettings` 中 `evolution_*` / `evo_*` / `guardrail_*` |
| EvolutionScanner（30min ticker） | ❌ | `internal/` 无对应 worker |
| 指标真实计算 | ❌ | `GetEvolutionMetrics` 返回静态/零值数据 |
| 建议生成逻辑 | ❌ | 无基于指标自动生成建议的代码 |
| diff_preview 生成 | ❌ | 建议的 diff_preview 字段为空 |
| SOUL.md 自动演化 | ❌ | 开关已实现但运行时未自动修改 soul_md |
| 适应护栏运行时控制 | ❌ | 字段已定义但运行时未使用 |

### 2.2 前端状态

| 项 | 状态 | 证据 |
|----|------|------|
| 进化开关四项 | 🟡 待验证 | 需确认前端是否已渲染四个 QToggle |
| 时间范围选择（7d/30d/90d） | 🟡 待验证 | 需确认 QBtnGroup 是否已实现 |
| 工具成功率图表 | ❌ 未实现 | 需图表组件（ECharts/Chart.js） |
| 检索质量图表 | ❌ 未实现 | 需图表组件 |
| 建议列表 | 🟡 待验证 | 需确认建议列表是否已渲染 |
| 适应护栏配置 | 🟡 待验证 | 需确认三个数值输入是否已实现 |
| 空态设计 | ❌ 未实现 | 需求 §5/§6/§7 描述的空态文案 |

---

## 3. 差距与优化

1. **P2（EP-BIZ-07）**：EvolutionScanner（30min ticker 自动扫描运行指标并生成建议）代码不存在。当前进化建议需手动触发或外部注入。
2. **P2**：进化指标（tool_success_rate / retrieval_quality）为静态返回，无真实计算逻辑。需从 session/tool_invocation 表聚合。
3. **P3**：进化建议的 diff_preview 字段为空，无实际 diff 生成逻辑。
4. **P3**：SOUL.md 自动演化逻辑未实现（开关已实现但运行时无动作）。
5. **P3**：适应护栏参数在运行时未使用，无法控制演化幅度。
6. **P3**：前端图表组件未集成，指标看板为空态。

---

## 4. 开发阶段

- **Phase 1**：实现 EvolutionScanner（后台 worker + safego + 30min ticker）+ 指标聚合
- **Phase 2**：建议生成逻辑 + diff_preview 生成 + SOUL.md 自动演化
- **Phase 3**：适应护栏运行时控制 + 前端图表集成

---

## 5. 任务清单

| # | 任务 | 层 | 优先级 | EP | 需求回溯 |
|---|------|-----|--------|-----|----------|
| 1 | `internal/evolution/scanner.go`：30min ticker + safego | 后端 | P2 | EP-BIZ-07 | 需求 §3/§7 |
| 2 | 指标聚合：从 session/tool_invocation 计算 success_rate | 后端 | P2 | EP-BIZ-07 | 需求 §5 |
| 3 | 指标聚合：从 memory_recall 计算 retrieval_quality | 后端 | P2 | EP-BIZ-07 | 需求 §6 |
| 4 | 建议生成：基于指标阈值生成 EvolutionSuggestion | 后端 | P2 | EP-BIZ-07 | 需求 §7 |
| 5 | Wire 注入 Scanner 到启动流程 | 后端 | P2 | EP-BIZ-07 | — |
| 6 | diff_preview 生成逻辑（对比当前 prompt 与建议 prompt） | 后端 | P3 | — | 需求 §7 |
| 7 | SOUL.md 自动演化：Scanner 修改 soul_md 风格段落 | 后端 | P3 | — | 需求 §3 |
| 8 | 适应护栏运行时控制：使用 guardrail_* 参数限制演化 | 后端 | P3 | — | 需求 §8 |
| 9 | 前端图表组件集成（ECharts/Chart.js） | 前端 | P3 | — | 需求 §5/§6 |
| 10 | 前端空态设计（指标/建议无数据时） | 前端 | P3 | — | 需求 §5/§6/§7 |

---

## 6. EvolutionScanner 架构设计

```
┌──────────────────────────────────────────────────────┐
│  EvolutionScanner（单例，Wire 注入）                    │
│                                                      │
│  ┌─────────────┐    30min ticker                     │
│  │ safego.Go   │───► ScanAllAgents()                 │
│  └─────────────┘         │                           │
│                          ▼                           │
│              ┌───────────────────────┐               │
│              │ 遍历 self_evolve=true │               │
│              │ 的 Agent 列表         │               │
│              └───────────┬───────────┘               │
│                          │                           │
│              ┌───────────▼───────────┐               │
│              │ 对每个 Agent：         │               │
│              │ 1. 聚合指标            │               │
│              │ 2. 检查护栏阈值        │               │
│              │ 3. 生成建议（如有）     │               │
│              │ 4. 自动演化（如开启）   │               │
│              └───────────────────────┘               │
│                                                      │
│  数据源：                                             │
│  - sessions 表 → 工具调用统计                         │
│  - tool_invocation 表 → 成功/失败率                   │
│  - memory_recall 表 → 检索质量                        │
│  - evolution_suggestions 表 → 建议持久化              │
└──────────────────────────────────────────────────────┘
```

**关键设计决策**：
- Scanner 为单例，通过 Wire 注入，在 `kratos.App.Start()` 时启动
- 使用 `pkg/safego.Go` 包裹 goroutine，确保 panic 不崩溃
- 30min 间隔可配置（`EVOLUTION_SCAN_INTERVAL` 环境变量）
- 每次扫描需检查 `evolution_metrics_enabled` 和 `evolution_suggestions_enabled` 开关
- 建议默认为 `pending` 状态，需用户手动应用（除非 `evo_auto_apply = true`）

---

## 7. 指标聚合查询设计

### 7.1 工具成功率

```sql
SELECT
    agent_id,
    COUNT(*) as total_calls,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) as success_calls,
    CAST(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) as success_rate
FROM tool_invocations
WHERE agent_id = ?
  AND created_at >= ?
GROUP BY agent_id
```

### 7.2 检索质量

```sql
SELECT
    agent_id,
    AVG(score) as avg_score,
    COUNT(*) as total_recalls
FROM memory_recalls
WHERE agent_id = ?
  AND created_at >= ?
GROUP BY agent_id
```

### 7.3 时间序列

```sql
SELECT
    DATE(created_at) as date,
    CAST(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) AS FLOAT) / COUNT(*) as daily_rate
FROM tool_invocations
WHERE agent_id = ?
  AND created_at >= ?
GROUP BY DATE(created_at)
ORDER BY date
```

---

## 8. 验收标准

- [ ] EvolutionScanner 每 30min 自动运行，生成进化建议
- [ ] 进化指标基于真实运行数据计算（非零值/静态值）
- [ ] 建议生成遵循适应护栏参数限制
- [ ] 前端图表正确展示工具成功率和检索质量趋势
- [ ] `go test ./internal/evolution/...` 通过

---

## 9. 依赖与风险

### 9.1 跨模块依赖

| 依赖模块 | 依赖项 | 说明 |
|----------|--------|------|
| 模块5 Agent设置 | 进化开关字段 | Scanner 需读取 `evolution_*` 开关 |
| 模块6 Agent文件 | SOUL.md 修改 | 自动演化需修改 SOUL.md 文件内容 |
| 模块10 Session | session 数据 | 指标聚合需查询 sessions 表 |
| 模块23 Tools | tool_invocation 数据 | 工具成功率需查询工具调用记录 |
| 模块12-16 Memory | memory_recall 数据 | 检索质量需查询记忆召回记录 |

### 9.2 风险

- Scanner 需读取大量 session/tool_invocation 数据，需注意查询性能（建议加时间索引）
- 进化建议的自动应用需谨慎，建议默认为"待审核"状态
- SOUL.md 自动修改需确保只改风格段落，不改身份/操作指令
- Scanner 单例需考虑优雅关闭（context cancel）

---

## 10. 错误处理规格

| 场景 | HTTP 状态码 | 错误码 | 前端行为 |
|------|------------|--------|----------|
| 指标查询 Agent 无运行数据 | 200 OK | — | 前端展示空态文案 |
| 建议应用失败（SOUL.md 已被修改） | 409 Conflict | `SUGGESTION_STALE` | Toast：建议已过期，请刷新 |
| Scanner 内部错误 | — | — | 日志记录，不影响主服务 |
| 护栏阈值被突破 | 200 OK | — | 自动回滚 + 生成告警建议 |
| 进化开关关闭时请求指标 | 200 OK | — | 前端展示"已关闭进化指标"遮罩 |
