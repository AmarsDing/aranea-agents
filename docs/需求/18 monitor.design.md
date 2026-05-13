# Monitor 监控模块 — 实现设计文档

> 对应需求：`18 monitor.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

运行时监控：模型调用日志、Token 用量统计、错误追踪。通过 `MonitorLogBroker` 实时推送。

---

## 二、Proto 层

### 2.1 现有 Proto

文件：`api/kratos/monitor/v1/monitor.proto`

```protobuf
service MonitorService {
  rpc ListMonitorLogs(ListMonitorLogsRequest) returns (ListMonitorLogsResponse) {
    option (google.api.http) = { get: "/v1/monitor/logs" };
  }
  rpc GetMonitorStats(GetMonitorStatsRequest) returns (MonitorStats) {
    option (google.api.http) = { get: "/v1/monitor/stats" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type MonitorLog struct {
    ID          string
    SessionID   string
    AgentID     string
    Level       string  // "info"/"warn"/"error"
    Type        string  // "model_call"/"tool_call"/"error"
    Message     string
    Metadata    map[string]interface{}
    CreatedAt   string
}

type MonitorStats struct {
    TotalCalls      int64
    TotalTokens     int64
    ErrorCount      int64
    AvgLatencyMs    float64
}
```

### 3.2 Broker

```go
type MonitorLogBroker struct {
    mu    sync.RWMutex
    chans map[string]chan *MonitorLog
}

func (b *MonitorLogBroker) Subscribe(id string) <-chan *MonitorLog
func (b *MonitorLogBroker) Unsubscribe(id string)
func (b *MonitorLogBroker) Publish(log *MonitorLog)
```

---

## 四、Data 层

Ent Schema：`internal/data/ent/schema/` 中无独立 monitor 表，日志从 `tool_invocations` 和 `model_token_usage_events` 聚合。

---

## 五、Service 层

```go
func (s *MonitorService) ListMonitorLogs(ctx, req) (*ListMonitorLogsResponse, error)
func (s *MonitorService) GetMonitorStats(ctx, req) (*MonitorStats, error)
```

---

## 六、Wire 注入

已有，无需新增。

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/features/monitor/
├── api.ts
├── types.ts
├── utils.ts
└── components/
    ├── MonitorDashboard.vue
    ├── MonitorLogTable.vue
    └── MonitorStatsCards.vue
```

### 7.2 API

```typescript
export async function listMonitorLogs(query: MonitorLogQuery): Promise<MonitorLogListResult>
export async function getMonitorStats(req: MonitorStatsRequest): Promise<MonitorStats>
```
