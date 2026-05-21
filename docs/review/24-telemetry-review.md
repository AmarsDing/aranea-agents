# 24 Telemetry Review

> **评分**：73 / 100 | **风险等级**：P1  
> **文档**：[24 telemetry.md](../需求/24%20telemetry.md) · [24 telemetry.design.md](../需求/24%20telemetry.design.md) · [24-telemetry-development.md](../需求/24-telemetry-development.md)  
> **代码锚点**：`internal/telemetry/` · `internal/service/turn_trace.go` · `internal/service/graph_telemetry.go`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 14 | 20 | Chat/Team/Graph turn OTel Span + HTTP 采样 ✅；gRPC 采样 / Trace UI 待补；per-span otel_id ✅ |
| 架构一致性 | 20 | 25 | `internal/telemetry` 独立初始化层 ✅；OTel SDK 桥接 trpc-agent-go telemetry ✅；双轨（OTLP + Prometheus）清晰 |
| 后端实现质量 | 16 | 20 | `telemetry.Init`（OTLP exporter）✅；`turntrace/bridge.go`（trpc Span 桥接）✅；采样率 HTTP 配置 ✅ |
| 前端实现质量 | 11 | 15 | Monitor Traces Tab + `TraceWaterfall.vue` ✅；Trace 详情 + Span 树 ✅；JSONL 导出 ✅；Trace UI 体验仍弱 |
| 测试与验证 | 6 | 10 | `sampler_test.go`、`bridge_test.go`、`telemetry_test.go` ✅；OTLP init 路径多为手动测试 |
| 文档一致性 | 6 | 10 | 三件套对齐；双轨观测分工在设计文档中描述 |

---

## 双轨观测架构

| 轨道 | 实现 | 用途 |
|------|------|------|
| OTLP Trace/Metrics | `internal/telemetry/telemetry.go` → OTLP exporter | LLM turn Span、分布式追踪 |
| Prometheus | `internal/metrics/` → `/metrics` endpoint | Runner 指标、Provider 调用量、配额告警 |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| OTLP 初始化（`telemetry.Init`）| ✅ I6-TEL-01 |
| `chat.turn` OTel Span | ✅ I6-TEL-01 |
| Monitor Trace 瀑布图 + usage spans | ✅ I6-TEL-02 |
| `turn_spans` metadata | ✅ |
| `TraceWaterfall.vue` | ✅ |
| Span 树展示 | ✅ |
| JSONL 导出 | ✅ |
| HTTP 采样率配置 | ✅ |
| Prometheus Provider 指标 | ✅ M4 |
| gRPC 采样配置 | ❌ 待补 |
| per-span `otel_id` | ✅ `TraceEmitter.SyncOtelSpanIDs` |
| Trace UI 体验优化 | 🟡 基础可用 |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| TEL-P1-01 | gRPC 采样配置（当使用 gRPC transport 时的 Span 采样率）未实现 | 补充 gRPC 采样配置项 |
| TEL-P1-02 | per-span `otel_id` | ✅ FlowLog + OTel Span 已同步 |
| TEL-P1-03 | Trace UI 体验弱：Span 树缺乏过滤、时间轴对比、Span 详情侧边栏 | 规划 Trace 详情 UI 增强 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| TEL-P2-01 | OTLP exporter 目标（Jaeger/Tempo/OTLP endpoint）的配置验证和连通性检查缺失 | 启动时验证 OTLP exporter 配置 |
| TEL-P2-02 | Usage 三路径（`recordTurnUsage` vs `recordChatIngressUsage`）运维手册未完成 | 补 Usage 三路径运维文档 |

---

## 建议优化路径

1. 实现 per-span `otel_id` 关联（P1）。
2. 补 gRPC 采样配置（P1）。
3. 增强 Trace 详情 UI（Span 过滤、对比视图）。
4. 完成 Usage 三路径运维手册（P2）。
