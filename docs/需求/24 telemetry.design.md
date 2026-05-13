# Telemetry 遥测模块 — 实现设计文档

> 对应需求：`24 telemetry.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

OpenTelemetry 遥测集成：Trace/Metrics/Logs 三大信号，覆盖 Agent 运行全链路。

---

## 二、Proto 层

无需独立 Proto，通过 Kratos 中间件和 OTLP Exporter 集成。

---

## 三、Biz 层

### 3.1 领域模型

```go
type TraceSpan struct {
    TraceID   string
    SpanID    string
    ParentID  string
    Operation string  // "agent_run"/"model_call"/"tool_call"
    AgentID   string
    SessionID string
    StartTime time.Time
    EndTime   time.Time
    Status    string
    Attributes map[string]string
}
```

---

## 四、Data 层

### 4.1 Kratos 中间件

```go
// internal/server/middleware/telemetry.go
func TelemetryMiddleware() middleware.Middleware {
    return func(handler middleware.Handler) middleware.Handler {
        return func(ctx context.Context, req interface{}) (interface{}, error) {
            span := trace.SpanFromContext(ctx)
            span.SetAttributes(attribute.String("agent.id", extractAgentID(req)))
            return handler(ctx, req)
        }
    }
}
```

### 4.2 OTLP Exporter

```go
// internal/server/telemetry.go
func InitTelemetry(cfg *conf.Telemetry) error {
    exporter, _ := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(cfg.Otlp.Endpoint))
    tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
    otel.SetTracerProvider(tp)
}
```

---

## 五、Service 层

无需独立 Service，通过中间件自动采集。

---

## 六、Wire 注入

待新增：`internal/server/telemetry.go` → `InitTelemetry`

---

## 七、Web 前端设计

### 7.1 组件

**MonitorDashboard.vue** 中增加 Trace 面板：
- Trace 列表
- Span 详情瀑布图
- Metrics 图表（Grafana 嵌入或自绘）

### 7.2 API

```typescript
export async function getTraceList(query: TraceQuery): Promise<TraceListResult>
export async function getTraceDetail(traceId: string): Promise<TraceDetail>
```
