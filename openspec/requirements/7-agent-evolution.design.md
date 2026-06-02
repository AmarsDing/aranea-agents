# Agent 进化 Tab — 实现设计文档

> 对应需求：`7 agent-evolution.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 设置页「进化」Tab：进化开关、指标看板、建议列表、适应护栏配置。数据来自 `agent_runtime_settings` 的 `evolution_*` / `evo_*` / `guardrail_*` 字段和运行时指标聚合。

开关字段已在 `AgentRuntimeSettings` 中定义并通过 `settingsFromLegacyConfig` 解析存储，运行时逻辑（指标采集、建议生成、SOUL.md 自动演化、护栏控制）待实现。

---

## 二、Proto 层

### 2.1 现有 Proto（`api/kratos/agent/v1/agent.proto`）

```protobuf
message AgentRuntimeSettings {
  // ... 其他字段 ...
  bool evolution_self_evolve = 22;
  bool evolution_skill_evolve = 23;
  bool evolution_metrics_enabled = 24;
  bool evolution_suggestions_enabled = 25;
  double guardrail_max_change_per_period = 26;
  int32 guardrail_min_data_points = 27;
  int32 guardrail_rollback_on_decline_percent = 28;
  // ... evo_* 字段 ...
  bool evo_enabled = 68;
  bool evo_auto_apply = 69;
  int32 evo_min_episodes = 70;
  int32 evo_min_negative_feedback = 71;
  int32 evo_throttle_hours = 72;
  int32 evo_proposal_ttl_days = 73;
  int32 evo_persona_max_chars = 74;
  int32 evo_system_prompt_max_appends = 75;
}

service AgentService {
  rpc GetAgent(GetAgentRequest) returns (Agent) { ... }
  rpc UpdateAgent(UpdateAgentRequest) returns (Agent) { ... }
  // ... 其他 RPC ...
}
```

### 2.2 待新增 Proto

在 `agent.proto` 中新增进化指标和建议相关消息与 RPC：

```protobuf
message GetAgentEvolutionMetricsRequest {
  string agent_id = 1 [(google.api.field_behavior) = REQUIRED];
  string time_range = 2; // "7d" | "30d" | "90d"
}

message EvolutionMetricsResponse {
  string agent_id = 1;
  string time_range = 2;
  double tool_success_rate = 3;
  double retrieval_quality = 4;
  int32 total_episodes = 5;
  int32 negative_feedback = 6;
  repeated MetricDataPoint tool_success_series = 7;
  repeated MetricDataPoint retrieval_quality_series = 8;
}

message MetricDataPoint {
  string date = 1; // "2026-05-01"
  double value = 2;
}

message GetAgentEvolutionSuggestionsRequest {
  string agent_id = 1 [(google.api.field_behavior) = REQUIRED];
  string status = 2; // "pending" | "applied" | "rejected" | ""
}

message EvolutionSuggestion {
  string id = 1;
  string agent_id = 2;
  string type = 3; // "persona" | "skill" | "prompt"
  string title = 4;
  string content = 5;
  string status = 6; // "pending" | "applied" | "rejected"
  string diff_preview = 7;
  string created_at = 8;
  string applied_at = 9;
}

message ListEvolutionSuggestionsResponse {
  repeated EvolutionSuggestion items = 1;
}

message ApplyEvolutionSuggestionRequest {
  string agent_id = 1 [(google.api.field_behavior) = REQUIRED];
  string suggestion_id = 2 [(google.api.field_behavior) = REQUIRED];
}

message RejectEvolutionSuggestionRequest {
  string agent_id = 1 [(google.api.field_behavior) = REQUIRED];
  string suggestion_id = 2 [(google.api.field_behavior) = REQUIRED];
}

// 在 AgentService 中新增：
service AgentService {
  // ... 现有 RPC ...

  rpc GetAgentEvolutionMetrics(GetAgentEvolutionMetricsRequest) returns (EvolutionMetricsResponse) {
    option (google.api.http) = { get: "/v1/agents/{agent_id}/evolution/metrics" };
  }
  rpc GetAgentEvolutionSuggestions(GetAgentEvolutionSuggestionsRequest) returns (ListEvolutionSuggestionsResponse) {
    option (google.api.http) = { get: "/v1/agents/{agent_id}/evolution/suggestions" };
  }
  rpc ApplyEvolutionSuggestion(ApplyEvolutionSuggestionRequest) returns (EvolutionSuggestion) {
    option (google.api.http) = { post: "/v1/agents/{agent_id}/evolution/suggestions/{suggestion_id}/apply" body: "*" };
  }
  rpc RejectEvolutionSuggestion(RejectEvolutionSuggestionRequest) returns (EvolutionSuggestion) {
    option (google.api.http) = { post: "/v1/agents/{agent_id}/evolution/suggestions/{suggestion_id}/reject" body: "*" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
// internal/biz/evolution.go

type EvolutionMetrics struct {
    AgentID              string
    TimeRange            string
    ToolSuccessRate      float64
    RetrievalQuality     float64
    TotalEpisodes        int
    NegativeFeedback     int
    ToolSuccessSeries    []MetricDataPoint
    RetrievalQualitySeries []MetricDataPoint
}

type MetricDataPoint struct {
    Date  string
    Value float64
}

type EvolutionSuggestion struct {
    ID           string
    AgentID      string
    Type         string // "persona" | "skill" | "prompt"
    Title        string
    Content      string
    Status       string // "pending" | "applied" | "rejected"
    DiffPreview  string
    CreatedAt    string
    AppliedAt    string
}
```

### 3.2 Repository 接口

```go
// internal/biz/evolution.go

type EvolutionMetricsRepo interface {
    GetToolSuccessRate(ctx context.Context, agentID string, since time.Time) (float64, []MetricDataPoint, error)
    GetRetrievalQuality(ctx context.Context, agentID string, since time.Time) (float64, []MetricDataPoint, error)
    GetEpisodeCount(ctx context.Context, agentID string, since time.Time) (int, error)
    GetNegativeFeedbackCount(ctx context.Context, agentID string, since time.Time) (int, error)
}

type EvolutionSuggestionRepo interface {
    ListByAgent(ctx context.Context, agentID string, status string) ([]EvolutionSuggestion, error)
    GetByID(ctx context.Context, id string) (EvolutionSuggestion, error)
    Create(ctx context.Context, s EvolutionSuggestion) (EvolutionSuggestion, error)
    UpdateStatus(ctx context.Context, id string, status string) (EvolutionSuggestion, error)
}
```

### 3.3 Usecase

```go
// internal/biz/evolution.go

type EvolutionUsecase struct {
    metricsRepo     EvolutionMetricsRepo
    suggestionRepo  EvolutionSuggestionRepo
    agents          AgentRepository
}

func NewEvolutionUsecase(
    metricsRepo EvolutionMetricsRepo,
    suggestionRepo EvolutionSuggestionRepo,
    agents AgentRepository,
) *EvolutionUsecase

func (uc *EvolutionUsecase) GetEvolutionMetrics(ctx context.Context, agentID string, timeRange string) (EvolutionMetrics, error) {
    since := timeRangeToSince(timeRange)
    toolRate, toolSeries, _ := uc.metricsRepo.GetToolSuccessRate(ctx, agentID, since)
    retrievalRate, retrievalSeries, _ := uc.metricsRepo.GetRetrievalQuality(ctx, agentID, since)
    episodes, _ := uc.metricsRepo.GetEpisodeCount(ctx, agentID, since)
    negFeedback, _ := uc.metricsRepo.GetNegativeFeedbackCount(ctx, agentID, since)
    return EvolutionMetrics{
        AgentID: agentID, TimeRange: timeRange,
        ToolSuccessRate: toolRate, RetrievalQuality: retrievalRate,
        TotalEpisodes: episodes, NegativeFeedback: negFeedback,
        ToolSuccessSeries: toolSeries, RetrievalQualitySeries: retrievalSeries,
    }, nil
}

func (uc *EvolutionUsecase) GetEvolutionSuggestions(ctx context.Context, agentID string, status string) ([]EvolutionSuggestion, error) {
    return uc.suggestionRepo.ListByAgent(ctx, agentID, status)
}

func (uc *EvolutionUsecase) ApplySuggestion(ctx context.Context, agentID string, suggestionID string) error {
    s, err := uc.suggestionRepo.GetByID(ctx, suggestionID)
    if err != nil {
        return err
    }
    if s.AgentID != agentID {
        return ErrNotFound
    }
    if s.Status != "pending" {
        return ErrInvalidArgument
    }
    switch s.Type {
    case "persona":
        ag, _ := uc.agents.Get(ctx, agentID)
        files, _ := uc.agents.ListAgentPromptFiles(ctx, agentID)
        for i, f := range files {
            if f.Name == "SOUL.md" {
                f.Body = s.Content
                files[i] = f
                break
            }
        }
        uc.agents.ReplaceAgentPromptFiles(ctx, agentID, files)
    case "prompt":
        // 应用到指定 prompt file
    }
    _, err = uc.suggestionRepo.UpdateStatus(ctx, suggestionID, "applied")
    return err
}

func (uc *EvolutionUsecase) RejectSuggestion(ctx context.Context, agentID string, suggestionID string) error {
    _, err := uc.suggestionRepo.UpdateStatus(ctx, suggestionID, "rejected")
    return err
}

func timeRangeToSince(tr string) time.Time {
    now := time.Now()
    switch tr {
    case "7d":
        return now.AddDate(0, 0, -7)
    case "30d":
        return now.AddDate(0, 0, -30)
    case "90d":
        return now.AddDate(0, 0, -90)
    default:
        return now.AddDate(0, 0, -30)
    }
}
```

---

## 四、Data 层

### 4.1 Ent Schema — `evolution_suggestions`

```go
// internal/data/ent/schema/evolution_suggestion.go

type EvolutionSuggestion struct{}

func (EvolutionSuggestion) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("agent_id").MaxLen(256),
        field.String("type").MaxLen(64),   // "persona" | "skill" | "prompt"
        field.String("title").MaxLen(512),
        field.Text("content"),
        field.String("status").MaxLen(32).Default("pending"), // "pending" | "applied" | "rejected"
        field.Text("diff_preview").Default(""),
        field.String("created_at").Default(""),
        field.String("applied_at").Default(""),
    }
}

func (EvolutionSuggestion) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("agent_id", "status"),
        index.Fields("agent_id", "created_at"),
    }
}
```

### 4.2 指标查询实现

```go
// internal/data/evolution_metrics_repo.go

type evolutionMetricsRepo struct {
    data *Data
}

func NewEvolutionMetricsRepo(data *Data) biz.EvolutionMetricsRepo {
    return &evolutionMetricsRepo{data: data}
}

func (r *evolutionMetricsRepo) GetToolSuccessRate(ctx context.Context, agentID string, since time.Time) (float64, []biz.MetricDataPoint, error) {
    var total, success int
    r.data.entClient.ToolInvocation.Query().
        Where(
            toolinvocation.AgentIDEQ(agentID),
            toolinvocation.CreatedAtGTE(since.Format(time.RFC3339)),
        ).
        Select(toolinvocation.FieldStatus).
        ForEach(ctx, func(row *ent.ToolInvocation) error {
            total++
            if row.Status == "success" {
                success++
            }
            return nil
        })
    rate := 0.0
    if total > 0 {
        rate = float64(success) / float64(total)
    }
    series := r.aggregateByDay(ctx, agentID, since, "tool_success")
    return rate, series, nil
}

func (r *evolutionMetricsRepo) GetRetrievalQuality(ctx context.Context, agentID string, since time.Time) (float64, []biz.MetricDataPoint, error) {
    // 从 memory_entities 或 session 上下文快照聚合检索命中率
    series := r.aggregateByDay(ctx, agentID, since, "retrieval_quality")
    avg := 0.0
    if len(series) > 0 {
        sum := 0.0
        for _, p := range series {
            sum += p.Value
        }
        avg = sum / float64(len(series))
    }
    return avg, series, nil
}

func (r *evolutionMetricsRepo) GetEpisodeCount(ctx context.Context, agentID string, since time.Time) (int, error) {
    count, err := r.data.entClient.Session.Query().
        Where(
            session.AgentIDEQ(agentID),
            session.CreatedAtGTE(since.Format(time.RFC3339)),
        ).
        Count(ctx)
    return count, err
}

func (r *evolutionMetricsRepo) GetNegativeFeedbackCount(ctx context.Context, agentID string, since time.Time) (int, error) {
    // 从消息中统计负反馈（如 thumbs_down 标记）
    count, _ := r.data.entClient.Message.Query().
        Where(
            message.AgentIDEQ(agentID),
            message.CreatedAtGTE(since.Format(time.RFC3339)),
            message.HasFeedbackWith(feedback.TypeEQ("negative")),
        ).
        Count(ctx)
    return count, nil
}

func (r *evolutionMetricsRepo) aggregateByDay(ctx context.Context, agentID string, since time.Time, metricType string) []biz.MetricDataPoint {
    // 按 created_at 日期分组聚合，返回每日数据点
    return nil // 实际实现用 raw SQL 或 Ent group by
}
```

### 4.3 建议存储实现

```go
// internal/data/evolution_suggestion_repo.go

type evolutionSuggestionRepo struct {
    data *Data
}

func NewEvolutionSuggestionRepo(data *Data) biz.EvolutionSuggestionRepo {
    return &evolutionSuggestionRepo{data: data}
}

func (r *evolutionSuggestionRepo) ListByAgent(ctx context.Context, agentID string, status string) ([]biz.EvolutionSuggestion, error) {
    query := r.data.entClient.EvolutionSuggestion.Query().
        Where(evolutionsuggestion.AgentIDEQ(agentID))
    if status != "" {
        query = query.Where(evolutionsuggestion.StatusEQ(status))
    }
    rows, err := query.Order(ent.Desc(evolutionsuggestion.FieldCreatedAt)).All(ctx)
    if err != nil {
        return nil, err
    }
    out := make([]biz.EvolutionSuggestion, 0, len(rows))
    for _, row := range rows {
        out = append(out, entSuggestionToBiz(row))
    }
    return out, nil
}

func (r *evolutionSuggestionRepo) GetByID(ctx context.Context, id string) (biz.EvolutionSuggestion, error) {
    row, err := r.data.entClient.EvolutionSuggestion.Get(ctx, id)
    if err != nil {
        return biz.EvolutionSuggestion{}, err
    }
    return entSuggestionToBiz(row), nil
}

func (r *evolutionSuggestionRepo) Create(ctx context.Context, s biz.EvolutionSuggestion) (biz.EvolutionSuggestion, error) {
    row, err := r.data.entClient.EvolutionSuggestion.Create().
        SetID(s.ID).SetAgentID(s.AgentID).SetType(s.Type).
        SetTitle(s.Title).SetContent(s.Content).SetStatus(s.Status).
        SetDiffPreview(s.DiffPreview).
        Save(ctx)
    if err != nil {
        return biz.EvolutionSuggestion{}, err
    }
    return entSuggestionToBiz(row), nil
}

func (r *evolutionSuggestionRepo) UpdateStatus(ctx context.Context, id string, status string) (biz.EvolutionSuggestion, error) {
    row, err := r.data.entClient.EvolutionSuggestion.UpdateOneID(id).
        SetStatus(status).
        Save(ctx)
    if err != nil {
        return biz.EvolutionSuggestion{}, err
    }
    return entSuggestionToBiz(row), nil
}

func entSuggestionToBiz(row *ent.EvolutionSuggestion) biz.EvolutionSuggestion {
    return biz.EvolutionSuggestion{
        ID:          row.ID,
        AgentID:     row.AgentID,
        Type:        row.Type,
        Title:       row.Title,
        Content:     row.Content,
        Status:      row.Status,
        DiffPreview: row.DiffPreview,
        CreatedAt:   row.CreatedAt,
        AppliedAt:   row.AppliedAt,
    }
}
```

---

## 五、Service 层

```go
// internal/service/agent_evolution.go

func (s *AgentService) GetAgentEvolutionMetrics(ctx context.Context, req *v1.GetAgentEvolutionMetricsRequest) (*v1.EvolutionMetricsResponse, error) {
    m, err := s.evoUC.GetEvolutionMetrics(ctx, req.GetAgentId(), req.GetTimeRange())
    if err != nil {
        return nil, err
    }
    resp := &v1.EvolutionMetricsResponse{
        AgentId:          m.AgentID,
        TimeRange:        m.TimeRange,
        ToolSuccessRate:  m.ToolSuccessRate,
        RetrievalQuality: m.RetrievalQuality,
        TotalEpisodes:    int32(m.TotalEpisodes),
        NegativeFeedback: int32(m.NegativeFeedback),
    }
    for _, p := range m.ToolSuccessSeries {
        resp.ToolSuccessSeries = append(resp.ToolSuccessSeries, &v1.MetricDataPoint{Date: p.Date, Value: p.Value})
    }
    for _, p := range m.RetrievalQualitySeries {
        resp.RetrievalQualitySeries = append(resp.RetrievalQualitySeries, &v1.MetricDataPoint{Date: p.Date, Value: p.Value})
    }
    return resp, nil
}

func (s *AgentService) GetAgentEvolutionSuggestions(ctx context.Context, req *v1.GetAgentEvolutionSuggestionsRequest) (*v1.ListEvolutionSuggestionsResponse, error) {
    items, err := s.evoUC.GetEvolutionSuggestions(ctx, req.GetAgentId(), req.GetStatus())
    if err != nil {
        return nil, err
    }
    resp := &v1.ListEvolutionSuggestionsResponse{}
    for _, item := range items {
        resp.Items = append(resp.Items, toProtoSuggestion(item))
    }
    return resp, nil
}

func (s *AgentService) ApplyEvolutionSuggestion(ctx context.Context, req *v1.ApplyEvolutionSuggestionRequest) (*v1.EvolutionSuggestion, error) {
    err := s.evoUC.ApplySuggestion(ctx, req.GetAgentId(), req.GetSuggestionId())
    if err != nil {
        return nil, err
    }
    item, _ := s.evoUC.GetSuggestionByID(ctx, req.GetSuggestionId())
    return toProtoSuggestion(item), nil
}

func (s *AgentService) RejectEvolutionSuggestion(ctx context.Context, req *v1.RejectEvolutionSuggestionRequest) (*v1.EvolutionSuggestion, error) {
    err := s.evoUC.RejectSuggestion(ctx, req.GetAgentId(), req.GetSuggestionId())
    if err != nil {
        return nil, err
    }
    item, _ := s.evoUC.GetSuggestionByID(ctx, req.GetSuggestionId())
    return toProtoSuggestion(item), nil
}

func toProtoSuggestion(s biz.EvolutionSuggestion) *v1.EvolutionSuggestion {
    return &v1.EvolutionSuggestion{
        Id:          s.ID,
        AgentId:     s.AgentID,
        Type:        s.Type,
        Title:       s.Title,
        Content:     s.Content,
        Status:      s.Status,
        DiffPreview: s.DiffPreview,
        CreatedAt:   s.CreatedAt,
        AppliedAt:   s.AppliedAt,
    }
}
```

---

## 六、Wire 注入

```go
// internal/data/data.go — ProviderSet 新增
var ProviderSet = wire.NewSet(
    // ... 现有 ...
    NewEvolutionMetricsRepo,
    NewEvolutionSuggestionRepo,
)

// internal/biz/biz.go — ProviderSet 新增
var ProviderSet = wire.NewSet(
    // ... 现有 ...
    NewEvolutionUsecase,
)

// internal/service/service.go — AgentService 新增 evoUC 字段
type AgentService struct {
    uc     *biz.AgentUsecase
    evoUC  *biz.EvolutionUsecase
    // ... 其他字段 ...
}
```

---

## 七、运行时层（待实现）

### 7.1 指标采集

```go
// internal/agent/evolution/collector.go

type MetricsCollector struct {
    metricsRepo biz.EvolutionMetricsRepo
}

// AfterToolInvocation 在工具调用完成后采集指标
func (c *MetricsCollector) AfterToolInvocation(ctx context.Context, agentID string, toolKey string, status string, durationMs int) {
    // 写入 tool_invocations 记录（已有），此处可触发聚合缓存更新
}

// AfterMemoryRecall 在记忆召回后采集检索质量
func (c *MetricsCollector) AfterMemoryRecall(ctx context.Context, agentID string, query string, score float64, hitCount int) {
    // 写入 memory_recall_events 或更新聚合
}
```

### 7.2 建议生成（定时任务）

```go
// internal/agent/evolution/suggester.go

type Suggester struct {
    suggestionRepo biz.EvolutionSuggestionRepo
    metricsRepo    biz.EvolutionMetricsRepo
    agents         biz.AgentRepository
}

// GenerateSuggestions 由 cron 任务触发，为启用进化建议的 Agent 生成建议
func (s *Suggester) GenerateSuggestions(ctx context.Context) error {
    // 1. 查询所有 evolution_suggestions_enabled = true 的 Agent
    // 2. 对每个 Agent 获取近期指标
    // 3. 基于规则或 LLM 分析生成改进建议
    // 4. 写入 evolution_suggestions 表
    return nil
}
```

### 7.3 SOUL.md 自动演化

```go
// internal/agent/evolution/evolver.go

type Evolver struct {
    agents    biz.AgentRepository
    suggester *Suggester
}

// EvolvePersona 在满足条件时自动修改 SOUL.md 的风格段落
// 条件：evo_enabled && evo_auto_apply && episodes >= evo_min_episodes && negative_feedback >= evo_min_negative_feedback
func (e *Evolver) EvolvePersona(ctx context.Context, agentID string) error {
    ag, _ := e.agents.Get(ctx, agentID)
    if ag.Settings == nil || !ag.Settings.EvoEnabled || !ag.Settings.EvoAutoApply {
        return nil
    }
    // 检查护栏：guardrail_max_change_per_period / guardrail_min_data_points / guardrail_rollback_on_decline_percent
    // 生成 persona 变更建议
    // 应用到 SOUL.md
    return nil
}
```

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/features/agents/
├── api.ts                          ← 新增进化相关 API
├── types.ts                        ← 新增进化相关类型
└── components/
    └── settings/
        └── AgentEvolutionTab.vue   ← 进化 Tab 主组件
```

### 8.2 TypeScript 类型

```typescript
// web/src/features/agents/types.ts 新增

export type EvolutionMetrics = {
  agent_id: string;
  time_range: string;
  tool_success_rate: number;
  retrieval_quality: number;
  total_episodes: number;
  negative_feedback: number;
  tool_success_series: MetricDataPoint[];
  retrieval_quality_series: MetricDataPoint[];
};

export type MetricDataPoint = {
  date: string;
  value: number;
};

export type EvolutionSuggestion = {
  id: string;
  agent_id: string;
  type: "persona" | "skill" | "prompt";
  title: string;
  content: string;
  status: "pending" | "applied" | "rejected";
  diff_preview: string;
  created_at: string;
  applied_at: string;
};
```

### 8.3 API 调用

```typescript
// web/src/features/agents/api.ts 新增

export async function getEvolutionMetrics(
  agentId: string,
  timeRange: string
): Promise<EvolutionMetrics> {
  const { data } = await http.get(`/v1/agents/${agentId}/evolution/metrics`, {
    params: { time_range: timeRange },
  });
  return normalizeEvolutionMetrics(data);
}

export async function getEvolutionSuggestions(
  agentId: string,
  status?: string
): Promise<EvolutionSuggestion[]> {
  const params: Record<string, string> = {};
  if (status) params.status = status;
  const { data } = await http.get(
    `/v1/agents/${agentId}/evolution/suggestions`,
    { params }
  );
  return (data?.items ?? []).map(normalizeSuggestion);
}

export async function applySuggestion(
  agentId: string,
  suggestionId: string
): Promise<EvolutionSuggestion> {
  const { data } = await http.post(
    `/v1/agents/${agentId}/evolution/suggestions/${suggestionId}/apply`
  );
  return normalizeSuggestion(data);
}

export async function rejectSuggestion(
  agentId: string,
  suggestionId: string
): Promise<EvolutionSuggestion> {
  const { data } = await http.post(
    `/v1/agents/${agentId}/evolution/suggestions/${suggestionId}/reject`
  );
  return normalizeSuggestion(data);
}
```

### 8.4 Vue 组件 — AgentEvolutionTab.vue

```vue
<template>
  <QScrollArea style="height: calc(100vh - 120px)">
    <div class="q-pa-md q-gutter-md">

      <!-- §3 进化开关 -->
      <QCard flat bordered>
        <QCardSection>
          <div class="text-subtitle1 q-mb-md">进化开关</div>

          <div class="q-gutter-sm">
            <div class="row items-center justify-between">
              <div>
                <div class="text-body2">允许 Agent 进化其沟通风格</div>
                <div class="text-caption text-grey">
                  允许随时间更新 SOUL.md 中的语调与风格；身份与操作指令保持锁定
                </div>
              </div>
              <QToggle v-model="settings.evolution_self_evolve" @update:model-value="onSettingChange" />
            </div>

            <QSeparator />

            <QBanners v-if="settings.evolution_self_evolve" inline-actions rounded class="bg-info text-white q-mb-sm">
              <template #avatar>
                <QIcon name="info" />
              </template>
              仅风格/语调可变，身份与工作流规则不变
            </QBanners>

            <div class="row items-center justify-between">
              <div>
                <div class="text-body2">允许从经验中创建和管理技能</div>
                <div class="text-caption text-grey">可提示用户将工作流保存为技能</div>
              </div>
              <QToggle v-model="settings.evolution_skill_evolve" @update:model-value="onSettingChange" />
            </div>

            <QSeparator />

            <div class="row items-center justify-between">
              <div>
                <div class="text-body2">进化指标</div>
                <div class="text-caption text-grey">记录工具效果、检索质量、反馈等，供看板展示</div>
              </div>
              <QToggle v-model="settings.evolution_metrics_enabled" @update:model-value="onSettingChange" />
            </div>

            <QSeparator />

            <div class="row items-center justify-between">
              <div>
                <div class="text-body2">进化建议</div>
                <div class="text-caption text-grey">基于指标由分析任务生成改进建议</div>
              </div>
              <QToggle v-model="settings.evolution_suggestions_enabled" @update:model-value="onSettingChange" />
            </div>
          </div>
        </QCardSection>
      </QCard>

      <!-- §4 时间范围 -->
      <div class="row items-center q-mb-none">
        <span class="text-body2 q-mr-sm">时间范围:</span>
        <QBtnToggle
          v-model="timeRange"
          no-caps
          rounded
          toggle-color="primary"
          :options="[
            { label: '7天', value: '7d' },
            { label: '30天', value: '30d' },
            { label: '90天', value: '90d' },
          ]"
          @update:model-value="onTimeRangeChange"
        />
      </div>

      <!-- §5 工具成功率 -->
      <QCard flat bordered v-if="settings.evolution_metrics_enabled">
        <QCardSection>
          <div class="text-subtitle1 q-mb-sm">工具成功率</div>
          <template v-if="metrics.tool_success_series.length > 0">
            <div class="text-h4 text-primary">{{ (metrics.tool_success_rate * 100).toFixed(1) }}%</div>
            <!-- 图表占位：可接入 Chart.js / ECharts -->
            <div class="text-caption text-grey q-mt-sm">
              共 {{ metrics.total_episodes }} 次调用
            </div>
          </template>
          <template v-else>
            <div class="column items-center q-pa-lg text-grey-6">
              <QIcon name="trending_up" size="48px" class="q-mb-sm" />
              <div>在 Agent 处理足够请求后，此处将展示工具调用相关成功率等指标</div>
            </div>
          </template>
        </QCardSection>
      </QCard>

      <!-- §6 检索质量 -->
      <QCard flat bordered v-if="settings.evolution_metrics_enabled">
        <QCardSection>
          <div class="text-subtitle1 q-mb-sm">检索质量</div>
          <template v-if="metrics.retrieval_quality_series.length > 0">
            <div class="text-h4 text-primary">{{ (metrics.retrieval_quality * 100).toFixed(1) }}%</div>
            <!-- 图表占位 -->
          </template>
          <template v-else>
            <div class="column items-center q-pa-lg text-grey-6">
              <QIcon name="search" size="48px" class="q-mb-sm" />
              <div>在 Agent 产生足够检索/记忆相关请求后展示</div>
            </div>
          </template>
        </QCardSection>
      </QCard>

      <!-- §7 建议 -->
      <QCard flat bordered v-if="settings.evolution_suggestions_enabled">
        <QCardSection>
          <div class="text-subtitle1 q-mb-sm">建议</div>
          <template v-if="suggestions.length > 0">
            <QList separator>
              <QItem v-for="s in suggestions" :key="s.id">
                <QItemSection>
                  <QItemLabel>{{ s.title }}</QItemLabel>
                  <QItemLabel caption>{{ s.type }} · {{ s.created_at }}</QItemLabel>
                </QItemSection>
                <QItemSection side>
                  <QBadge v-if="s.status === 'applied'" color="positive">已应用</QBadge>
                  <QBadge v-else-if="s.status === 'rejected'" color="grey">已忽略</QBadge>
                  <div v-else class="q-gutter-xs">
                    <QBtn flat dense label="应用" color="primary" @click="onApplySuggestion(s.id)" />
                    <QBtn flat dense label="忽略" color="grey" @click="onRejectSuggestion(s.id)" />
                  </div>
                </QItemSection>
              </QItem>
            </QList>
          </template>
          <template v-else>
            <div class="column items-center q-pa-lg text-grey-6">
              <QIcon name="lightbulb_outline" size="48px" class="q-mb-sm" />
              <div>建议由每日分析定时任务生成后展示于此</div>
            </div>
          </template>
        </QCardSection>
      </QCard>

      <!-- §8 适应护栏 -->
      <QCard flat bordered>
        <QCardSection>
          <div class="row items-center q-mb-md">
            <QIcon name="security" size="24px" class="q-mr-sm" />
            <div class="text-subtitle1">适应护栏</div>
          </div>

          <div class="q-gutter-md">
            <div class="row items-center q-gutter-md">
              <div class="col text-body2">每周期最大变化</div>
              <QInput
                v-model.number="settings.guardrail_max_change_per_period"
                type="number"
                dense
                outlined
                style="max-width: 120px"
                step="0.01"
                min="0"
                max="1"
                @change="onSettingChange"
              />
            </div>

            <div class="row items-center q-gutter-md">
              <div class="col text-body2">最少数据点</div>
              <QInput
                v-model.number="settings.guardrail_min_data_points"
                type="number"
                dense
                outlined
                style="max-width: 120px"
                min="1"
                @change="onSettingChange"
              />
            </div>

            <div class="row items-center q-gutter-md">
              <div class="col text-body2">下降时回滚</div>
              <QInput
                v-model.number="settings.guardrail_rollback_on_decline_percent"
                type="number"
                dense
                outlined
                style="max-width: 120px"
                suffix="%"
                min="0"
                max="100"
                @change="onSettingChange"
              />
            </div>
          </div>
        </QCardSection>
      </QCard>

    </div>
  </QScrollArea>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from "vue";
import { useQuasar } from "quasar";
import type { AgentRuntimeSettings, EvolutionMetrics, EvolutionSuggestion } from "../types";
import {
  getEvolutionMetrics,
  getEvolutionSuggestions,
  applySuggestion,
  rejectSuggestion,
} from "../api";

const props = defineProps<{
  agentId: string;
  settings: AgentRuntimeSettings;
}>();

const emit = defineEmits<{
  (e: "settings-changed", settings: AgentRuntimeSettings): void;
}>();

const $q = useQuasar();
const timeRange = ref<string>("30d");
const metrics = ref<EvolutionMetrics>({
  agent_id: "",
  time_range: "30d",
  tool_success_rate: 0,
  retrieval_quality: 0,
  total_episodes: 0,
  negative_feedback: 0,
  tool_success_series: [],
  retrieval_quality_series: [],
});
const suggestions = ref<EvolutionSuggestion[]>([]);

onMounted(() => {
  loadMetrics();
  loadSuggestions();
});

watch(() => props.settings.evolution_metrics_enabled, (v) => {
  if (v) loadMetrics();
});
watch(() => props.settings.evolution_suggestions_enabled, (v) => {
  if (v) loadSuggestions();
});

function onTimeRangeChange() {
  loadMetrics();
  loadSuggestions();
}

async function loadMetrics() {
  if (!props.settings.evolution_metrics_enabled) return;
  try {
    metrics.value = await getEvolutionMetrics(props.agentId, timeRange.value);
  } catch { /* ignore */ }
}

async function loadSuggestions() {
  if (!props.settings.evolution_suggestions_enabled) return;
  try {
    suggestions.value = await getEvolutionSuggestions(props.agentId, "pending");
  } catch { /* ignore */ }
}

function onSettingChange() {
  emit("settings-changed", { ...props.settings });
}

async function onApplySuggestion(id: string) {
  try {
    await applySuggestion(props.agentId, id);
    $q.notify({ type: "positive", message: "建议已应用" });
    loadSuggestions();
  } catch {
    $q.notify({ type: "negative", message: "应用失败" });
  }
}

async function onRejectSuggestion(id: string) {
  try {
    await rejectSuggestion(props.agentId, id);
    loadSuggestions();
  } catch { /* ignore */ }
}
</script>
```

---

## 九、验收要点

- [ ] 四项进化开关与 `evolution_self_evolve` / `evolution_skill_evolve` / `evolution_metrics_enabled` / `evolution_suggestions_enabled` 一致
- [ ] 「仅演化 SOUL 风格」的说明 Banner 与 `6 agent-setting-file.md` 中 SOUL 语义一致
- [ ] 时间范围 7d / 30d / 90d 切换后重新拉取指标/建议
- [ ] 工具成功率和检索质量空态文案与有数据态切换正确；关闭「进化指标」时隐藏对应卡片
- [ ] 建议列表在关闭「进化建议」时隐藏或展示禁用遮罩
- [ ] 适应护栏三项与 `guardrail_max_change_per_period` / `guardrail_min_data_points` / `guardrail_rollback_on_decline_percent` 一致
- [ ] 应用建议时正确修改 SOUL.md 并更新建议状态
- [ ] 与 `2 agents-create.md` 默认策略、`3 agent-list.md`「进化中」徽章推导无矛盾
