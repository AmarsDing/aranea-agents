# Agent 进化 Tab — 实现设计文档

> 对应需求：[7 agent-evolution.md](./7%20agent-evolution.md)
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> 开发进度与代码锚点：[7-agent-evolution.development.md](./7-agent-evolution.development.md)

---

## 一、模块概述

Agent 设置页「进化」Tab：进化开关、指标看板、建议列表、适应护栏配置。数据来自 `agent_runtime_settings` 的 `evolution_*` / `evo_*` / `guardrail_*` 字段和运行时指标聚合。

开关字段在 `AgentRuntimeSettings` 中定义并通过 `settingsFromLegacyConfig` 解析存储。运行时指标采集通过查询 `tool_invocations` 表实现，建议生成由 `EvolutionScanner` 定时任务驱动。

---

## 二、Proto 层

### 2.1 现有 Proto（`api/kratos/agent/v1/agent.proto`）

`AgentRuntimeSettings` 消息中已定义进化相关字段：

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
```

`Agent` 消息中包含 `int32 pending_evolution_count = 26;` 用于列表徽章推导。

### 2.2 进化指标与建议 RPC（已实现）

`AgentService` 中已定义以下 RPC，对应需求文档 §10 的 API 端点：

```protobuf
message GetAgentEvolutionMetricsRequest {
  string agent_id = 1 [(google.api.field_behavior) = REQUIRED];
  string time_range = 2; // "7d" | "30d" | "90d"
}

message MetricDataPoint {
  string date = 1; // "2026-05-01"
  double value = 2;
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
  string status = 6; // "pending" | "applied" | "rejected" | "rolled_back"
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
    AgentID                string
    TimeRange              string
    ToolSuccessRate        float64
    RetrievalQuality       float64
    TotalEpisodes          int
    NegativeFeedback       int
    ToolSuccessSeries      []MetricDataPoint
    RetrievalQualitySeries []MetricDataPoint
    // 标记指标数据因部分查询失败而不完整
    Partial       bool
    PartialErrors []string
}

type MetricDataPoint struct {
    Date  string
    Value float64
}

type EvolutionSuggestion struct {
    ID               string
    AgentID          string
    Type             string // "persona" | "skill" | "prompt"
    Title            string
    Content          string
    Status           string // "pending" | "applied" | "rejected" | "rolled_back"
    DiffPreview      string
    PreApplySnapshot string // JSON-encoded map[filename]content，用于 Rollback
    CreatedAt        string
    AppliedAt        string
}
```

### 3.2 Repository 接口

```go
// internal/biz/evolution.go

// Stability:stable
type EvolutionMetricsRepo interface {
    GetToolSuccessRate(ctx context.Context, agentID string, since time.Time) (float64, []MetricDataPoint, error)
    GetRetrievalQuality(ctx context.Context, agentID string, since time.Time) (float64, []MetricDataPoint, error)
    GetEpisodeCount(ctx context.Context, agentID string, since time.Time) (int, error)
    GetNegativeFeedbackCount(ctx context.Context, agentID string, since time.Time) (int, error)
}

// Stability:stable
type EvolutionSuggestionRepo interface {
    ListByAgent(ctx context.Context, agentID string, status string) ([]EvolutionSuggestion, error)
    GetByID(ctx context.Context, id string) (EvolutionSuggestion, error)
    Create(ctx context.Context, s EvolutionSuggestion) (EvolutionSuggestion, error)
    UpdateStatus(ctx context.Context, id string, status string) (EvolutionSuggestion, error)
    UpdateSnapshot(ctx context.Context, id string, snapshot string) error
}
```

### 3.3 Usecase

```go
// internal/biz/evolution.go

type EvolutionUsecase struct {
    metricsRepo      EvolutionMetricsRepo
    suggestionRepo   EvolutionSuggestionRepo
    agents           AgentRepository
    coordinator      *EvolutionCoordinator      // 遗留：跨流水线去重
    orchestrator     *SkillEvolutionOrchestrator // 统一：跨流水线去重
    orchestratorOnce sync.Once
    lg               loggateway.Logger
    evolutionSM      *EvolutionStateMachine
}

func NewEvolutionUsecase(
    metricsRepo EvolutionMetricsRepo,
    suggestionRepo EvolutionSuggestionRepo,
    agents AgentRepository,
    lg loggateway.Logger,
) *EvolutionUsecase
```

核心方法：

- `GetEvolutionMetrics(ctx, agentID, timeRange)` — 聚合四项指标，部分查询失败时标记 `Partial=true` 并记录 `PartialErrors`
- `GetEvolutionSuggestions(ctx, agentID, status)` — 列表查询
- `ApplySuggestion(ctx, agentID, suggestionID)` — 应用建议：
  - 校验状态机转换 `Pending → Applied`
  - 保存 `PreApplySnapshot`（应用前的 prompt files 快照）
  - `type=persona`：写入 `IDENTITY.md` 的 `## Persona` 段（PGO V2 后替代 SOUL.md，保留 SOUL.md 作为遗留兜底）
  - `type=prompt`：写入 `AGENTS_CORE.md` 或首匹配 `AGENTS*.md` 文件
- `RejectSuggestion(ctx, agentID, suggestionID)` — 拒绝建议，状态机转换 `Pending → Rejected`
- `RollbackSuggestion(ctx, agentID, suggestionID)` — 回滚建议，状态机转换 `Applied → RolledBack`，从 `PreApplySnapshot` 恢复 prompt files
- `ScanAll(ctx)` / `ScanAgent(ctx, agentID)` — 扫描指标并生成建议（由 `EvolutionScanner` 定时调用）

### 3.4 状态机（AS-FSM-01）

`EvolutionSuggestion` 实体拥有 4 种状态，已定义显式状态机 `internal/biz/evolution_state_machine.go`：

```
┌─────────┐  apply   ┌─────────┐  rollback  ┌────────────┐
│ Pending │ ───────► │ Applied │ ─────────► │ RolledBack │
└────┬────┘          └─────────┘            └────────────┘
     │ reject
     ▼
┌──────────┐
│ Rejected │
└──────────┘
```

| From | Event | To |
|------|-------|----|
| Pending | apply | Applied |
| Pending | reject | Rejected |
| Applied | rollback | RolledBack |

终态：`Rejected`、`RolledBack`。所有状态转换经 `EvolutionStateMachine.Transition()` 校验，非法转换返回错误。

### 3.5 跨流水线去重

`EvolutionCoordinator`（`internal/biz/evolution_coordinator.go`）与 `SkillEvolutionOrchestrator`（`internal/biz/skill_evolution_unified.go`）提供跨流水线的 pending 建议去重：

- `ScanAgent` 优先委托 `orchestrator.HasPendingForTarget(ctx, "agent", agentID)` 检查
- 回退到 `coordinator.HasPendingEvolution(ctx, EvolutionTarget{Type, ID})`
- 已有 pending 同 type 建议时跳过扫描

---

## 四、Data 层

### 4.1 Ent Schema — `evolution_suggestions`

```go
// internal/data/ent/schema/evolution_suggestion.go

type EvolutionSuggestion struct {
    ent.Schema
}

func (EvolutionSuggestion) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "evolution_suggestions"},
    }
}

func (EvolutionSuggestion) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("agent_id").MaxLen(256),
        field.String("type").MaxLen(64),   // "persona" | "skill" | "prompt"
        field.String("title").MaxLen(512),
        field.Text("content").Default(""),
        field.String("status").MaxLen(32).Default("pending"), // "pending" | "applied" | "rejected" | "rolled_back"
        field.Text("diff_preview").Default(""),
        field.Text("pre_apply_snapshot").Default(""), // JSON map[filename]content，用于 Rollback
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

func NewEvolutionMetricsRepo(data *Data) biz.EvolutionMetricsRepo

// GetToolSuccessRate 从 tool_invocations 表聚合工具成功率
// 使用 r.data.RW().Read(ctx) 事务感知读连接
// 按日聚合返回 []MetricDataPoint
func (r *evolutionMetricsRepo) GetToolSuccessRate(ctx, agentID, since) (float64, []biz.MetricDataPoint, error)

// GetRetrievalQuality 聚合检索质量（基于记忆工具调用成功率）
func (r *evolutionMetricsRepo) GetRetrievalQuality(ctx, agentID, since) (float64, []biz.MetricDataPoint, error)

// GetEpisodeCount 统计 Session 数量
func (r *evolutionMetricsRepo) GetEpisodeCount(ctx, agentID, since) (int, error)

// GetNegativeFeedbackCount 统计负反馈数
func (r *evolutionMetricsRepo) GetNegativeFeedbackCount(ctx, agentID, since) (int, error)
```

### 4.3 建议存储实现

```go
// internal/data/evolution_suggestion_repo.go

type evolutionSuggestionRepo struct {
    data *Data
}

func NewEvolutionSuggestionRepo(data *Data) biz.EvolutionSuggestionRepo

// ListByAgent 按 agent_id 查询，可选 status 过滤，按 created_at 倒序
func (r *evolutionSuggestionRepo) ListByAgent(ctx, agentID, status) ([]biz.EvolutionSuggestion, error)

// GetByID 单条查询，ent.IsNotFound 时返回 fmt.Errorf("suggestion not found")
func (r *evolutionSuggestionRepo) GetByID(ctx, id) (biz.EvolutionSuggestion, error)

// Create 创建建议，自动填充 created_at
func (r *evolutionSuggestionRepo) Create(ctx, s) (biz.EvolutionSuggestion, error)

// UpdateStatus 更新状态，status="applied" 时同步填充 applied_at
func (r *evolutionSuggestionRepo) UpdateStatus(ctx, id, status) (biz.EvolutionSuggestion, error)

// UpdateSnapshot 更新 pre_apply_snapshot 字段
func (r *evolutionSuggestionRepo) UpdateSnapshot(ctx, id, snapshot) error
```

所有读写均通过 `r.data.RW().Read(ctx)` / `r.data.RW().Write(ctx)` 事务感知访问器，遵循 DB-R6 红线。

---

## 五、Service 层

```go
// internal/service/agent_evolution.go

func (s *AgentService) GetAgentEvolutionMetrics(ctx, req) (*v1.EvolutionMetricsResponse, error)
func (s *AgentService) GetAgentEvolutionSuggestions(ctx, req) (*v1.ListEvolutionSuggestionsResponse, error)
func (s *AgentService) ApplyEvolutionSuggestion(ctx, req) (*v1.EvolutionSuggestion, error)
func (s *AgentService) RejectEvolutionSuggestion(ctx, req) (*v1.EvolutionSuggestion, error)
```

`AgentService` 通过 `evoUC *biz.EvolutionUsecase` 字段持有 usecase（见 `internal/service/agent.go`）。

**关键设计**：`ApplyEvolutionSuggestion` 在应用建议后调用 `invalidateAgentBuildCache(req.GetAgentId())` 失效 Agent 构建缓存，避免下次加载 Agent 时使用过期的 prompt files。

---

## 六、Wire 注入

```go
// internal/data/data.go — ProviderSet
var ProviderSet = wire.NewSet(
    // ... 现有 ...
    NewEvolutionMetricsRepo,
    NewEvolutionSuggestionRepo,
)

// internal/biz/biz.go — ProviderSet
var ProviderSet = wire.NewSet(
    // ... 现有 ...
    NewEvolutionUsecase,
)

// internal/service/agent.go — AgentService 构造
func NewAgentService(uc *biz.AgentUsecase, evoUC *biz.EvolutionUsecase, ...) *AgentService

// cmd/admin/wire.go — EvolutionScanner Provider
func provideEvolutionScanner(evo *biz.EvolutionUsecase, logger log.Logger) *jobs.EvolutionScanner {
    if strings.TrimSpace(os.Getenv("EVOLUTION_SCANNER_DISABLED")) == "1" {
        return nil
    }
    return jobs.NewEvolutionScanner(0, evo, logger)
}
```

---

## 七、运行时层

### 7.1 指标采集

指标采集通过查询 `tool_invocations` 表实现（非 hook 方式）：

- `GetToolSuccessRate` 查询 `tool_invocations` 表，按 `agent_id` + `created_at >= since` 过滤，统计 `status="success"` 比例
- `GetRetrievalQuality` 基于记忆工具调用成功率聚合
- `GetEpisodeCount` 查询 `session` 表
- `GetNegativeFeedbackCount` 统计负反馈消息

按日聚合返回 `[]MetricDataPoint`，供前端绘制趋势图。

### 7.2 建议生成（定时任务）

```go
// internal/cronrunner/jobs/evolution_scanner.go

type EvolutionScanner struct {
    interval time.Duration
    evo      *biz.EvolutionUsecase
    log      *log.Helper
}

// NewEvolutionScanner 创建扫描器，interval ≤ 0 时默认 30 分钟
func NewEvolutionScanner(interval time.Duration, evo *biz.EvolutionUsecase, logger log.Logger) *EvolutionScanner

// Start 阻塞运行直到 ctx 取消，使用 safego.Go 隔离 panic
func (w *EvolutionScanner) Start(ctx context.Context)
```

`EvolutionUsecase.ScanAgent` 扫描逻辑（`internal/biz/evolution_scan.go`）：

1. 跨流水线去重：检查 orchestrator / coordinator 是否已有 pending 建议
2. 读取 `AgentRuntimeSettings`，校验 `EvolutionSuggestionsEnabled` 或 `EvoEnabled`
3. 获取近 30d 指标，校验 `EvoMinEpisodes`（默认 3）/ `EvoMinNegativeFeedback`（默认 2）阈值
4. 阈值触发：
   - 工具成功率 < 0.75 → 生成 `type=prompt` 建议
   - 检索质量 < 0.60 → 生成 `type=skill` 建议
   - 负反馈累积 → 生成 `type=persona` 建议
5. `ensurePendingSuggestion` 去重：同 agent + 同 type + 同 title 的 pending 建议已存在则跳过

启动注册：`cmd/admin/workers.go` 中 `goAfterReady("evolution", ...)` 在 ReadinessGate 通过后启动。

### 7.3 SOUL.md / IDENTITY.md 演化

`ApplySuggestion` 的 `type=persona` 分支已实现 prompt file 写入：

- PGO V2 后：优先写入 `IDENTITY.md` 的 `## Persona` 段（`replaceOrAppendPersona` 函数处理段替换/追加）
- 遗留兜底：若 `IDENTITY.md` 不存在则写入 `SOUL.md`
- 都不存在时：追加新的 `IDENTITY.md`，包含 `# IDENTITY\n\n## Persona\n\n<content>`

**未实现**：运行时自动触发 persona 演化（`Evolver.EvolvePersona`），目前仅由用户手动应用建议触发。

### 7.4 适应护栏

`guardrail_max_change_per_period` / `guardrail_min_data_points` / `guardrail_rollback_on_decline_percent` 字段已在 Schema 与 Proto 中定义，前端可配置。

**未实现**：扫描器未读取护栏参数控制演化幅度；`RollbackSuggestion` 已实现手动回滚，但未基于护栏自动触发。

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/
├── components/agents/
│   ├── AgentEvolutionPanel.vue          ← 进化 Tab 主组件
│   ├── AgentLearningLoopPanel.vue       ← Learning Loop 面板
│   ├── LearningLoopOverview.vue         ← Learning Loop 概览
│   ├── LearningObservationList.vue      ← 观察列表
│   ├── LearningPatternList.vue          ← 模式列表
│   └── LearningProposalList.vue         ← 提议列表
├── features/agents/
│   ├── api.ts                           ← Agent 通用 API
│   ├── api.learning.ts                  ← Learning Loop API
│   ├── types.ts                         ← 类型定义
│   ├── useAgentEvolutionPanel.ts        ← 进化面板 composable
│   ├── useAgentEvolutionSettings.ts     ← 进化设置 composable
│   └── useLearningLoopPanel.ts          ← Learning Loop composable
└── stores/
    ├── agents/detail.ts                 ← Agent 详情 store（含 fetchEvolutionMetrics）
    ├── skillEvolution/index.ts          ← 技能进化 store（含建议列表）
    └── learningLoop/index.ts            ← Learning Loop store
```

### 8.2 TypeScript 类型

```typescript
// web/src/features/agents/types.ts

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
  status: "pending" | "applied" | "rejected" | "rolled_back";
  diff_preview: string;
  created_at: string;
  applied_at: string;
};
```

### 8.3 Vue 组件 — AgentEvolutionPanel.vue

组件结构（自上而下）：

1. **进化开关** — `q-list` + `q-toggle`，四项开关（`self_evolve` / `skill_evolve` / `evolution_metrics_enabled` / `evolution_suggestions_enabled`），附说明 Banner
2. **自动提议流水线** — `evo_enabled` / `evo_auto_apply` 开关 + `evo_min_episodes` / `evo_min_negative_feedback` / `evo_throttle_hours` / `evo_proposal_ttl_days` / `evo_persona_max_chars` / `evo_system_prompt_max_appends` 数值输入
3. **指标与建议** — `q-btn-toggle` 时间范围（7d/30d/90d）+ 三张 KPI 卡片（工具成功率 / 检索质量 / 待处理建议数）+ 工具成功率趋势迷你柱状图
4. **进化建议列表** — `q-list` 列表，pending 项显示应用/拒绝按钮，其他状态显示徽章
5. **适应护栏** — 三项数值输入（`max_change_per_period` / `min_data_points` / `rollback_on_decline_percent`）

**数据流**：

- `props.evolution` / `props.evolutionSettings` / `props.guardrails` 由父组件 `AgentSettingsPage` 通过 `agentRuntimeConfig` 表单双向绑定
- 指标与建议通过 `useAgentEvolutionPanel(agentId, range)` composable 加载
- 建议列表从 `useSkillEvolutionStore()` 过滤 `targetType === 'agent' && targetId === agentId` 的 pending 项
- 应用/拒绝建议调用 `evolutionStore.approveSuggestion(id, 'agent-panel')` / `rejectSuggestion(id, 'agent-panel', '')`

### 8.4 API 调用

```typescript
// web/src/features/agents/api.ts 与 stores/agents/detail.ts

// 指标查询通过 agentDetailStore.fetchEvolutionMetrics(id, range) 触发
// 实际 HTTP 调用：GET /v1/agents/{agentId}/evolution/metrics?time_range={range}

// web/src/features/agents/api.learning.ts 与 stores/skillEvolution/index.ts

// 建议列表通过 evolutionStore.loadSuggestions({ targetType, targetId, status }) 触发
// 应用建议：evolutionStore.approveSuggestion(id, source)
// 拒绝建议：evolutionStore.rejectSuggestion(id, source, reason)
```

---

## 九、设计验收要点

- [ ] Proto 中 `evolution_*` / `evo_*` / `guardrail_*` 字段与 `AgentRuntimeSettings` Go struct 一一对应
- [ ] `EvolutionStateMachine` 覆盖所有合法状态转换，终态无出边
- [ ] `ApplySuggestion` 在写入 prompt files 前保存 `PreApplySnapshot`，支持 `RollbackSuggestion` 恢复
- [ ] `EvolutionScanner` 通过 `safego.Go` 隔离 panic，失败时聚合错误并由 worker 打 Warn 日志
- [ ] Repo 层所有读写通过 `r.data.RW().Read(ctx)` / `r.data.RW().Write(ctx)` 事务感知访问器
- [ ] `ApplyEvolutionSuggestion` Service 方法在应用后调用 `invalidateAgentBuildCache` 失效缓存
- [ ] 前端 `AgentEvolutionPanel.vue` 与 `useAgentEvolutionPanel.ts` 数据流单向：composable → store → 组件
- [ ] 跨流水线去重优先使用 `SkillEvolutionOrchestrator`，回退到 `EvolutionCoordinator`
