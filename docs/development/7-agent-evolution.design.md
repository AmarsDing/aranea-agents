# Agent 进化 Tab — 实现设计文档

> 对应需求：[7 agent-evolution.md](./7%20agent-evolution.md)
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> 开发进度与代码锚点：[7-agent-evolution.development.md](./7-agent-evolution.development.md)

---

## 一、模块概述

Agent 设置页「进化」Tab：进化开关、指标看板、建议列表、适应护栏配置。数据来自 `agent_runtime_settings` 的 `evolution_*` / `evo_*` / `guardrail_*` 字段和运行时指标聚合。

开关字段在 `AgentRuntimeSettings` 中定义并通过 `settingsFromLegacyConfig` 解析存储。运行时指标采集通过查询 `tool_invocations` 表实现，建议生成由 `EvolutionOrchestratorWorker` 定时任务驱动（统一进化编排入口，A1/A6），L3 扫描逻辑由 `AgentConfigTrigger` 执行（移植自 legacy `EvolutionUsecase.ScanAgent`）。

> **A6 物理收敛**：L3 建议的物理存储已从 legacy `evolution_suggestions` 表（Ent Schema，已删除）收敛到统一的 `unified_evolution_suggestions` 表（raw SQL DDL，详见 [20-skill.design.md](./20-skill.design.md) §6.8）。legacy 专有字段（`type`/`title`/`diff_preview`/`pre_apply_snapshot`）保留在 unified 行的 `metadata` JSON 列中，biz 层通过 `evolutionViewFromUnified` 重建 L3 视图，对外 proto 契约不变。迁移 `20261111` 完成逐行 backfill（主键预检幂等）后 DROP 三张 legacy 表。

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
  // 拒绝原因（可选），持久化到 metadata.rejection_reason 供审计。
  string reason = 3;
}

message RollbackEvolutionSuggestionRequest {
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
  rpc RollbackEvolutionSuggestion(RollbackEvolutionSuggestionRequest) returns (EvolutionSuggestion) {
    option (google.api.http) = { post: "/v1/agents/{agent_id}/evolution/suggestions/{suggestion_id}/rollback" body: "*" };
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

> **A6 视图语义**：`EvolutionSuggestion` 不再是物理表实体，而是从 unified 行重建的 L3 视图——`evolutionViewFromUnified`（[evolution.go](../../internal/biz/evolution.go)）从 `UnifiedEvolutionSuggestion` 重建，`Type`/`Title`/`DiffPreview`/`PreApplySnapshot` 读自 `metadata` JSON（`EvoMetaLegacyType`/`EvoMetaTitle`/`EvoMetaDiffPreview`/`EvoMetaPreApplySnapshot`）；`unifiedFromEvolutionView` 为反向转换（创建用：target_type=agent / action_type=evolve_agent / trigger_source=agent_config）。

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

// Stability:evolving — 窄端口：L3 建议创建/列表，供非进化消费者
//（精灵团队完成学习、任务编排 DQ 反馈）使用，由 EvolutionUsecase 实现。
type EvolutionSuggestionCreator interface {
    CreateSuggestion(ctx context.Context, s EvolutionSuggestion) (EvolutionSuggestion, error)
    GetEvolutionSuggestions(ctx context.Context, agentID string, status string) ([]EvolutionSuggestion, error)
}

// Stability:stable — 事务提供者：ApplySuggestion / RollbackSuggestion 将
// prompt files 替换 + 建议状态更新包裹在单个事务中（红线 #24），
// 由 data.Data 实现并经 SetTxProvider / ProvideEvolutionUsecase 注入。
type EvolutionTxProvider interface {
    ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
}
```

> **A6 变更**：legacy `EvolutionSuggestionRepo` 接口已删除。L3 建议的读写改经 `UnifiedEvolutionStore`（定义于 [skill_evolution_unified.go](../../internal/biz/skill_evolution_unified.go)，含 `UnifiedEvolutionQueryReader.ListByTargetAndAction` / `UnifiedEvolutionWriter.UpdateStatus` / `UnifiedEvolutionMetadataWriter.UpdateMetadataKey` 等窄接口），由 `data.UnifiedEvolutionRepo` 实现。

### 3.3 Usecase

```go
// internal/biz/evolution.go

type EvolutionUsecase struct {
    metricsRepo EvolutionMetricsRepo
    store       UnifiedEvolutionStore // A6：统一进化存储
    agents      AgentRepository
    lg          loggateway.Logger
    evolutionSM *EvolutionStateMachine
    txProvider  EvolutionTxProvider   // 可选；注入后 apply/rollback 事务化
}

// 测试直接调用（无 txProvider，保持 legacy 非事务行为）
func NewEvolutionUsecase(
    metricsRepo EvolutionMetricsRepo,
    store UnifiedEvolutionStore,
    agents AgentRepository,
    lg loggateway.Logger,
) *EvolutionUsecase

// Wire provider：注入 txProvider，apply/rollback 包裹单事务（红线 #24）
func ProvideEvolutionUsecase(
    metricsRepo EvolutionMetricsRepo,
    store UnifiedEvolutionStore,
    agents AgentRepository,
    tp EvolutionTxProvider,
    lg loggateway.Logger,
) *EvolutionUsecase
```

核心方法：

- `GetEvolutionMetrics(ctx, agentID, timeRange)` — 聚合四项指标，部分查询失败时标记 `Partial=true` 并记录 `PartialErrors`（采集逻辑由 `collectEvolutionMetrics` 实现，与 `AgentConfigTrigger` 共享）
- `GetEvolutionSuggestions(ctx, agentID, status)` — 列表查询：经 `store.ListByTargetAndAction(agent, action=evolve_agent, status)` 拉取 unified 行后逐行 `evolutionViewFromUnified` 重建 L3 视图
- `CreateSuggestion(ctx, s)` — 创建 L3 建议（`EvolutionSuggestionCreator` 端口，供精灵团队学习 / DQ 反馈等非进化消费者调用）
- `ApplySuggestion(ctx, agentID, suggestionID)` — 应用建议：
  - 校验状态机转换 `Pending → Applied`
  - **apply payload 门（2026-08-07 P0-2）**：`type=persona`/`prompt` 要求 metadata `apply_payload` 非空，否则拒绝（`BadRequest`，提示"该建议为指标通知，不包含可应用的修改内容"）。现存全部生产者（`AgentConfigTrigger` 指标通知、编排优化通知）只生成通知文本，不携带 payload——防止通知文本被写入 prompt 文件腐蚀配置。存量行无该键，默认拒绝，免迁移；未来 LLM 草稿生成器设置 payload 即解锁 apply
  - 保存 `PreApplySnapshot`（应用前的 prompt files 快照，经 `store.UpdateMetadataKey` 写入 metadata）
  - `type=persona`：将 payload 写入 `IDENTITY.md` 的 `## Persona` 段（PGO V2 后替代 SOUL.md，保留 SOUL.md 作为遗留兜底）
  - `type=prompt`：将 payload 写入 `AGENTS_CORE.md` 或首匹配 `AGENTS*.md` 文件
  - 注入 txProvider 时，prompt files 替换 + 状态更新在单事务中执行（红线 #24）
- `RejectSuggestion(ctx, agentID, suggestionID)` — 拒绝建议，状态机转换 `Pending → Rejected`
- `RollbackSuggestion(ctx, agentID, suggestionID)` — 回滚建议，状态机转换 `Applied → RolledBack`，从 `PreApplySnapshot` 恢复 prompt files（同样支持事务包裹）

> **A6 变更**：`ScanAll` / `ScanAgent` 已从 `EvolutionUsecase` 移除，L3 自动扫描逻辑移植到 `AgentConfigTrigger`（见 §3.5 与 §7.2）；legacy `EvolutionCoordinator` / orchestrator 委托字段随之删除。

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

### 3.5 跨流水线去重（A6 重构后）

去重分两层：

1. **触发器内去重**：`AgentConfigTrigger.Check`（[skill_evolution_triggers.go](../../internal/biz/skill_evolution_triggers.go)）在生成候选前，通过 `UnifiedEvolutionQueryReader.ListByTargetAndAction(agent, evolve_agent, pending)` 拉取 pending 建议，按 `legacy_type + title`（metadata）建 key 去重（镜像 legacy `ensurePendingSuggestion` 语义）。
2. **编排层统一去重**：`SkillEvolutionOrchestrator`（[skill_evolution_unified.go](../../internal/biz/skill_evolution_unified.go)）在 worker 层对同 target 的 pending 建议做跨流水线检查（`HasPendingForTarget`），DB 层由 pending 唯一索引兜底（方言感知 JSON 路径，见 [20-skill.design.md](./20-skill.design.md) §6.8）。

> legacy `EvolutionCoordinator`（`internal/biz/evolution_coordinator.go`）及 `EvolutionUsecase` 中的 orchestrator 委托已随 A6 删除。

---

## 四、Data 层

### 4.1 建议存储 — A6 收敛到统一表

> Ent Schema `internal/data/ent/schema/evolution_suggestion.go` 已删除（A6）。

L3 建议的物理存储为 `unified_evolution_suggestions` 表（raw SQL DDL，表结构与方言感知索引见 [20-skill.design.md](./20-skill.design.md) §6.8）。L3 行映射约定：

| unified 列 | L3 取值 |
|-----------|---------|
| `target_type` / `target_id` | `agent` / agentID |
| `action_type` | `evolve_agent` |
| `trigger_source` | `agent_config`（`unifiedFromEvolutionView` 固定；自动扫描与 `CreateSuggestion` 消费者——精灵团队完成学习、任务编排 DQ 反馈——均走此值） |
| `draft_body` | legacy `content` |
| `metadata` JSON | `legacy_type`（persona/skill/prompt）、`title`、`diff_preview`、`pre_apply_snapshot`、`apply_payload`（apply 实际写入内容；空 = 通知类建议不可应用） |
| `status` | `pending` / `applied` / `rejected` / `rolled_back`（原样保留） |

迁移 `20261111` 逐行 backfill（主键预检幂等）legacy `evolution_suggestions` 后 DROP 该表。

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

### 4.3 建议读写路径（A6 重构后）

> `internal/data/evolution_suggestion_repo.go` 已删除（A6），不再有独立的 L3 Repo 实现。

L3 建议读写全部经由 biz 层 `EvolutionUsecase` + `UnifiedEvolutionStore` 端口，data 层由 `UnifiedEvolutionRepo`（raw SQL、方言感知占位符、事务感知 `RWDB()` 访问器）承载：

| 操作 | 路径 |
|------|------|
| 列表 | `store.ListByTargetAndAction(agent, agentID, evolve_agent, status)` → `evolutionViewFromUnified` 逐行重建视图 |
| 单条 | `store.GetByID(id)` → nil 时返回 `apierror.NotFound` |
| 创建 | `unifiedFromEvolutionView` 转换 → `store.Create`（pending 唯一索引兜底去重） |
| 状态更新 | `store.UpdateStatus(id, status, actor, reason)`（`applied` 填充 `applied_at` 列；`approved`/`rejected` 经方言感知 `JSONSetMulti` 合并 metadata 的 `approved_at`/`rejected_by`/`resolved_at` 等视图字段） |
| 快照写入 | `store.UpdateMetadataKey(id, EvoMetaPreApplySnapshot, json)`（`savePreApplySnapshot`） |

指标查询仍由 `internal/data/evolution_metrics_repo.go`（Ent/事务感知读）实现，读写均通过 `r.data.RW().Read(ctx)` / `r.data.RW().Write(ctx)` 事务感知访问器，遵循 DB-R6 红线。

---

## 五、Service 层

```go
// internal/service/agent_evolution.go

func (s *AgentService) GetAgentEvolutionMetrics(ctx, req) (*v1.EvolutionMetricsResponse, error)
func (s *AgentService) GetAgentEvolutionSuggestions(ctx, req) (*v1.ListEvolutionSuggestionsResponse, error)
func (s *AgentService) ApplyEvolutionSuggestion(ctx, req) (*v1.EvolutionSuggestion, error)
func (s *AgentService) RejectEvolutionSuggestion(ctx, req) (*v1.EvolutionSuggestion, error)
func (s *AgentService) RollbackEvolutionSuggestion(ctx, req) (*v1.EvolutionSuggestion, error)
```

`AgentService` 通过 `evoUC *biz.EvolutionUsecase` 字段持有 usecase（见 `internal/service/agent.go`）。

**关键设计**：`ApplyEvolutionSuggestion` 在应用建议后调用 `invalidateAgentBuildCache(req.GetAgentId())` 失效 Agent 构建缓存，避免下次加载 Agent 时使用过期的 prompt files。`RejectEvolutionSuggestion` 将可选 `reason` 持久化到建议 metadata 的 `rejection_reason` 供审计；`RollbackEvolutionSuggestion` 仅允许 `applied` 状态回滚，恢复 apply 前快照（snapshot）后同样失效构建缓存。

---

## 六、Wire 注入

```go
// internal/data/data.go — ProviderSet
var ProviderSet = wire.NewSet(
    // ... 现有 ...
    NewEvolutionMetricsRepo,
    NewUnifiedEvolutionRepo,   // A6：统一进化存储（NewEvolutionSuggestionRepo 已移除）
)

// internal/biz/biz.go — ProviderSet
// NOTE(A1)：EvolutionUsecase / LearningLoopUsecase / SkillEvolutionUsecase
// 需要 SetOrchestrator / SetTxProvider，无法以裸构造函数表达，
// 已从本集合排除，改由 cmd/admin/wire.go 的 provide* 函数组装。

// internal/service/agent.go — AgentService 构造
func NewAgentService(uc *biz.AgentUsecase, evoUC *biz.EvolutionUsecase, ...) *AgentService

// cmd/admin/wire.go — 进化相关 providers（A1/A6 重构后）
func provideEvolutionUsecase(
    metricsRepo biz.EvolutionMetricsRepo,
    unifiedRepo *data.UnifiedEvolutionRepo,
    agents biz.AgentRepository,
    tp biz.EvolutionTxProvider,   // data.Data 实现
    lg loggateway.Logger,
) *biz.EvolutionUsecase          // → biz.ProvideEvolutionUsecase（注入事务提供者）

func provideSkillEvolutionOrchestrator(...) *biz.SkillEvolutionOrchestrator
    // 组装统一编排器并注册三个触发器：
    //   PatternTrigger（L1，原 SkillEvolutionScanner 职责）
    //   HealthTrigger（L2，原 CuratorWorker 触发职责）
    //   AgentConfigTrigger（L3，A6 移植自 EvolutionUsecase.ScanAgent）

func provideEvolutionOrchestratorWorker(
    orch *biz.SkillEvolutionOrchestrator,
    agents biz.AgentRepository,
    skills biz.SkillQueryReader,
    lg loggateway.Logger,
) *jobs.EvolutionOrchestratorWorker
    // EVOLUTION_ORCHESTRATOR_DISABLED=1 时返回 nil
    // 统一自动进化触发入口（A1），取代 legacy EvolutionScanner /
    // SkillEvolutionScanner 触发半区 / CuratorWorker 触发半区
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

### 7.2 建议生成（定时任务，A6 重构后）

> legacy `internal/cronrunner/jobs/evolution_scanner.go`（`EvolutionScanner`）与 `internal/biz/evolution_scan.go` 已删除（A6）。L3 自动扫描由统一编排 worker 驱动。

```go
// internal/cronrunner/jobs/evolution_orchestrator_worker.go

type EvolutionOrchestratorWorker struct { /* interval 默认 2 小时 */ }

// NewEvolutionOrchestratorWorker 创建 worker，interval ≤ 0 时默认 2 小时；
// drafter 为 EVO-20 post-pass（nil = 禁用草稿生成）
func NewEvolutionOrchestratorWorker(interval time.Duration, orch *biz.SkillEvolutionOrchestrator, agents EvolutionAgentLister, skills biz.SkillQueryReader, drafter EvolutionDrafterPort, lg loggateway.Logger) *EvolutionOrchestratorWorker

// Start 阻塞运行直到 ctx 取消，使用 safego.Go 隔离 panic
func (w *EvolutionOrchestratorWorker) Start(ctx context.Context)
```

`AgentConfigTrigger.Check`（`internal/biz/skill_evolution_triggers.go`，移植自 legacy `EvolutionUsecase.ScanAgent`）扫描逻辑：

1. L3 opt-in 门控：读取 `AgentRuntimeSettings`，`EvolutionSuggestionsEnabled` 或 `EvoEnabled` 均未开启则跳过
2. 获取近 30d 指标（复用 `collectEvolutionMetrics`），校验 `EvoMinEpisodes`（默认 3）/ `EvoMinNegativeFeedback`（默认 2）阈值
3. 阈值触发：
   - 工具成功率 < 0.75 → 生成 `legacy_type=prompt` 建议
   - 检索质量 < 0.60 → 生成 `legacy_type=skill` 建议
   - 负反馈累积 → 生成 `legacy_type=persona` 建议
4. 去重：经 `UnifiedEvolutionQueryReader` 拉取 pending 建议，同 `legacy_type + title`（metadata）已存在则跳过；DB pending 唯一索引兜底 check-then-create 竞态
5. 建议落库为 unified 行：`target_type=agent` / `action_type=evolve_agent` / `trigger_source=agent_config`

启动注册：`cmd/admin/workers.go` 中 `goAfterReady("evolution_orchestrator", ...)` 在 ReadinessGate 通过后启动；`EVOLUTION_ORCHESTRATOR_DISABLED=1` 可禁用。

#### 7.2.1 EVO-20：通知类建议的 LLM 草稿 post-pass（已实现 2026-08-08）

**问题**：`AgentConfigTrigger` 只产出指标通知文本（如"近30d负反馈 10 次…"），无 `apply_payload`，建议不可应用（EVO-17 门拒绝），用户看得到问题却拿不到修改方案。

**方案**：`EvolutionDrafter`（`internal/biz/evolution_drafter.go`）作为 worker 的 post-pass，在 `scanAgents` 每个 L3-opted-in agent 的 trigger 之后调用一次 `DraftPending`：

1. 拉取该 agent 的 pending `evolve_agent` 建议（上限 20），筛出 `legacy_type=persona|prompt` 且无 `apply_payload` 的行
2. 每 agent 每周期最多处理 1 条；每条 LLM 尝试（无论成败）记录 `draft_attempt_at`，1h 节流
3. 模型解析：agent 自有 `Provider/Model` → 平台 `DefaultRefineLLM` 回退；两者皆空则跳过
4. 草稿生成：
   - `persona`：提取 `IDENTITY.md` `## Persona` 段正文 → LLM 输出修订段正文（段级替换，与 `ApplySuggestion` persona 分支语义一致）
   - `prompt`：读 `AGENTS_CORE.md`/首个 `AGENTS*` 全文件 → LLM 输出完整修订文件（全文件替换语义）
5. 校验兜底：草稿非空、≤ 长度上限（persona 用 `EvoPersonaMaxChars`，默认 2000；prompt 硬上限 20000）、不短于原文一半（防 LLM 截断丢配置）；不通过则丢弃
6. 写回 `apply_payload` + `diff_preview`（`UnifiedDiffSimple`，超 4000 字符截断）→ `Applicable()` 自动为 true，前端显示应用按钮

**降级**：LLM 不可用/失败/校验失败一律静默降级为通知态（Warn 进程日志 `step=evolution.draft`），不影响 trigger 主流程；nil `LLMCaller` 时整个 drafter 为 no-op。

**日志策略**：仅进程日志（K6 外部调用）。进化流水线现有 trigger/orchestrator 均不发流程日志，drafter 保持一致——草稿结果经建议列表直接对用户可见。

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
- 建议列表调用 L3 API `agentDetailStore.fetchEvolutionSuggestions(id)`（`GET /v1/agents/{id}/evolution/suggestions`），展示 `evolve_agent` 行（2026-08-07 P0-1 修复：此前错接 L1 `ListSkillProposals`，L3 建议不可见）
- 应用/拒绝建议调用 `agentDetailStore.applyEvolution(agentId, id)` / `rejectEvolution(agentId, id)`；apply 被 payload 门拒绝时 toast 后端原因

### 8.4 API 调用

```typescript
// web/src/features/agents/api.ts 与 stores/agents/detail.ts

// 指标查询通过 agentDetailStore.fetchEvolutionMetrics(id, range) 触发
// 实际 HTTP 调用：GET /v1/agents/{agentId}/evolution/metrics?time_range={range}

// 建议列表：agentDetailStore.fetchEvolutionSuggestions(id)
//   → GET /v1/agents/{agentId}/evolution/suggestions（L3，evolve_agent）
// 应用建议：agentDetailStore.applyEvolution(agentId, suggestionId)
//   → POST /v1/agents/{agentId}/evolution/suggestions/{suggestionId}/apply
// 拒绝建议：agentDetailStore.rejectEvolution(agentId, suggestionId)
//   → POST /v1/agents/{agentId}/evolution/suggestions/{suggestionId}/reject

// 全局建议中心（Skill 进化页）仍走 web/src/features/skills/api.learning.ts
// 的 useSkillEvolutionStore（L1/L2 统一建议），与本面板 L3 链路解耦。
```

---

## 九、设计验收要点

- [ ] Proto 中 `evolution_*` / `evo_*` / `guardrail_*` 字段与 `AgentRuntimeSettings` Go struct 一一对应
- [ ] `EvolutionStateMachine` 覆盖所有合法状态转换，终态无出边
- [ ] `ApplySuggestion` 在写入 prompt files 前保存 `PreApplySnapshot`（metadata JSON），支持 `RollbackSuggestion` 恢复
- [ ] 注入 `EvolutionTxProvider` 时，apply/rollback 的 prompt files 替换 + 状态更新在单事务中执行（红线 #24）
- [ ] `EvolutionOrchestratorWorker` 通过 `safego.Go` 隔离 panic，失败时聚合错误并由 worker 打 Warn 日志
- [ ] 指标 Repo 读写通过 `r.data.RW().Read(ctx)` / `r.data.RW().Write(ctx)` 事务感知访问器；建议读写经 `UnifiedEvolutionRepo`（raw SQL + `RWDB()` 事务感知 + 方言感知占位符/JSON 路径）
- [ ] L3 建议视图经 `evolutionViewFromUnified` 从 unified 行重建，proto 契约不变（A6）
- [ ] `ApplyEvolutionSuggestion` Service 方法在应用后调用 `invalidateAgentBuildCache` 失效缓存
- [ ] `ApplySuggestion` 对 `type=persona`/`prompt` 强制要求 metadata `apply_payload` 非空（通知类建议拒绝写入 prompt 文件）
- [ ] 前端 `AgentEvolutionPanel.vue` 与 `useAgentEvolutionPanel.ts` 数据流单向：composable → store → 组件
- [ ] L3 自动扫描由 `AgentConfigTrigger` 执行（opt-in 门控 + type+title 去重），跨流水线 pending 去重由 `SkillEvolutionOrchestrator` 统一处理
