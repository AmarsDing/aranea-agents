# Agent 设置模块 — 实现设计文档

> 对应需求：`5 agent-setting.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 详情/设置页，包含身份、模型、系统提示、能力、记忆、进化、钩子、文件、权限等 Tab。复用 `AgentService.GetAgent`/`UpdateAgent` RPC，数据源为 `agents` 主表 + `agent_runtime_settings` O2O + `agent_prompt_files` O2M。

---

## 二、Proto 层

### 2.1 现有 Proto

文件：`api/kratos/agent/v1/agent.proto`

```protobuf
message Agent {
  string id = 1;
  string agent_key = 2;
  string display_name = 3;
  string provider = 4;
  string model = 5;
  string status = 6;
  bool is_default = 7;
  bool is_favorite = 8;
  string icon = 9;
  string agent_description = 10;
  string category_position_id = 11;
  string system_prompt_mode = 12;
  int32 context_window = 13;
  int32 budget_monthly_cents = 14;
  string config_json = 15;
  string created_at = 16;
  string updated_at = 17;
  string deleted_at = 18;
  AgentRuntimeSettings settings = 19;
  repeated AgentPromptFile files = 20;
}

message AgentRuntimeSettings {
  string agent_id = 1;
  bool self_evolve = 2;
  bool subagents_enabled = 3;
  int32 subagents_max_concurrency = 4;
  int32 subagents_max_generation_depth = 5;
  int32 subagents_max_children_per_agent = 6;
  int32 subagents_archive_after_minutes = 7;
  int32 subagents_max_retries = 8;
  string subagents_model_override = 9;
  bool tools_enabled = 10;
  string tools_profile = 11;
  string tools_tool_call_prefix = 12;
  string tools_allow_json = 13;
  string tools_deny_json = 14;
  string tools_concurrent_allow_json = 15;
  bool memory_enabled = 16;
  int32 memory_max_chunk_length = 17;
  int32 memory_max_results = 18;
  double memory_min_score = 19;
  bool heartbeat_enabled = 20;
  int32 heartbeat_interval_minutes = 21;
  bool evolution_self_evolve = 22;
  bool evolution_skill_evolve = 23;
  bool evolution_metrics_enabled = 24;
  bool evolution_suggestions_enabled = 25;
  double guardrail_max_change_per_period = 26;
  int32 guardrail_min_data_points = 27;
  int32 guardrail_rollback_on_decline_percent = 28;
  int32 l0_recent_window_turns = 29;
  int32 l0_recent_window_tokens = 30;
  double l0_summary_threshold = 31;
  int32 l0_summary_keep_turns = 32;
  string l0_truncate_strategy = 33;
  bool l0_inject_l1 = 34;
  bool l0_inject_l3 = 35;
  bool l0_inject_l4 = 36;
  int32 l0_l3_max_chunks = 37;
  int32 l0_l4_max_paths = 38;
  string l0_snapshot_mode = 39;
  bool l1_enabled = 40;
  int32 l1_budget_tokens = 41;
  int32 l1_field_max_tokens = 42;
  int32 l1_history_keep_revisions = 43;
  string l1_default_schema_id = 44;
  int32 l1_archive_on_idle_minutes = 45;
  bool l2_episode_enabled = 46;
  double l2_episode_min_importance = 47;
  bool l2_index_enabled = 48;
  string l2_index_embedding_model = 49;
  bool l2_recall_enabled = 50;
  int32 l2_recall_max = 51;
  int32 l2_retention_days = 52;
  int32 l2_archive_after_days = 53;
  bool l3_enabled = 54;
  int32 l3_recall_top_k = 55;
  double l3_recall_min_score = 56;
  string l3_recall_scopes_json = 57;
  string l3_embedding_model = 58;
  int32 l3_decay_interval_hours = 59;
  double l3_archive_threshold = 60;
  int32 l3_max_per_recall_chars = 61;
  bool l4_enabled = 62;
  bool l4_graph_inject_neighbors = 63;
  int32 l4_graph_max_neighbors = 64;
  int32 l4_graph_max_hops = 65;
  bool l4_identity_inject = 66;
  bool l4_strategy_inject = 67;
  bool evo_enabled = 68;
  bool evo_auto_apply = 69;
  int32 evo_min_episodes = 70;
  int32 evo_min_negative_feedback = 71;
  int32 evo_throttle_hours = 72;
  int32 evo_proposal_ttl_days = 73;
  int32 evo_persona_max_chars = 74;
  int32 evo_system_prompt_max_appends = 75;
  string created_at = 76;
  string updated_at = 77;
  string skill_runtime_json = 78;
  bool intent_pass_enabled = 79;
}

message AgentPromptFile {
  string id = 1;
  string agent_id = 2;
  string name = 3;
  string body = 4;
  int32 sort_order = 5;
  string created_at = 6;
  string updated_at = 7;
}

service AgentService {
  rpc ListAgents(ListAgentsRequest) returns (ListAgentsResponse) {
    option (google.api.http) = { get: "/v1/agents" };
  }
  rpc CreateAgent(CreateAgentRequest) returns (Agent) {
    option (google.api.http) = { post: "/v1/agents" body: "*" };
  }
  rpc GetAgent(GetAgentRequest) returns (Agent) {
    option (google.api.http) = { get: "/v1/agents/{id}" };
  }
  rpc UpdateAgent(UpdateAgentRequest) returns (Agent) {
    option (google.api.http) = { patch: "/v1/agents/{id}" body: "agent" };
  }
  rpc DeleteAgent(DeleteAgentRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/agents/{id}" };
  }
  rpc GetAgentPromptPreview(GetAgentPromptPreviewRequest) returns (GetAgentPromptPreviewResponse) {
    option (google.api.http) = { get: "/v1/agents/{id}/system-prompt/preview" };
  }
  rpc GetAgentEffectiveTools(GetAgentEffectiveToolsRequest) returns (AgentEffectiveToolsView) {
    option (google.api.http) = { get: "/v1/agents/{agent_id}/tools/effective" };
  }
  rpc UpdateAgentToolPolicy(UpdateAgentToolPolicyRequest) returns (AgentEffectiveToolsView) {
    option (google.api.http) = { put: "/v1/agents/{agent_id}/tools/policy" body: "*" };
  }
}
```

### 2.2 待新增 RPC

| RPC | 路径 | 用途 |
|-----|------|------|
| `ToggleFavorite` | `PATCH /v1/agents/{id}/favorite` | 收藏切换 |

---

## 三、Biz 层

### 3.1 领域模型

文件：`internal/biz/agent_types.go`

```go
type Agent struct {
    ID                 string
    AgentKey           string
    DisplayName        string
    Provider           string
    Model              string
    Status             string
    IsDefault          bool
    IsFavorite         bool
    Icon               string
    AgentDescription   string
    CategoryPositionID string
    SystemPromptMode   string
    ContextWindow      int
    BudgetMonthlyCents int
    ConfigJSON         string
    CreatedAt          string
    UpdatedAt          string
    DeletedAt          string
    Settings           *AgentRuntimeSettings
    Files              []AgentPromptFile
}

type AgentRuntimeSettings struct {
    AgentID                           string
    SelfEvolve                        bool
    SubagentsEnabled                  bool
    SubagentsMaxConcurrency           int
    SubagentsMaxGenerationDepth       int
    SubagentsMaxChildrenPerAgent      int
    SubagentsArchiveAfterMinutes      int
    SubagentsMaxRetries               int
    SubagentsModelOverride            string
    ToolsEnabled                      bool
    ToolsProfile                      string
    ToolsToolCallPrefix               string
    ToolsAllowJSON                    string
    ToolsDenyJSON                     string
    ToolsConcurrentAllowJSON          string
    MemoryEnabled                     bool
    MemoryMaxChunkLength              int
    MemoryMaxResults                  int
    MemoryMinScore                    float64
    HeartbeatEnabled                  bool
    HeartbeatIntervalMinutes          int
    EvolutionSelfEvolve               bool
    EvolutionSkillEvolve              bool
    EvolutionMetricsEnabled           bool
    EvolutionSuggestionsEnabled       bool
    GuardrailMaxChangePerPeriod       float64
    GuardrailMinDataPoints            int
    GuardrailRollbackOnDeclinePercent int
    L0RecentWindowTurns               int
    L0RecentWindowTokens              int
    L0SummaryThreshold                float64
    L0SummaryKeepTurns                int
    L0CompressProvider                string
    L0CompressModel                   string
    L0TruncateStrategy                string
    L0InjectL1                        bool
    L0InjectL3                        bool
    L0InjectL4                        bool
    L0L3MaxChunks                     int
    L0L4MaxPaths                      int
    L0SnapshotMode                    string
    L1Enabled                         bool
    L1BudgetTokens                    int
    L1FieldMaxTokens                  int
    L1HistoryKeepRevisions            int
    L1DefaultSchemaID                 string
    L1ArchiveOnIdleMinutes            int
    L2EpisodeEnabled                  bool
    L2EpisodeMinImportance            float64
    L2IndexEnabled                    bool
    L2IndexEmbeddingModel             string
    L2RecallEnabled                   bool
    L2RecallMax                       int
    L2RetentionDays                   int
    L2ArchiveAfterDays                int
    L3Enabled                         bool
    L3RecallTopK                      int
    L3RecallMinScore                  float64
    L3RecallScopesJSON                string
    L3EmbeddingModel                  string
    L3DecayIntervalHours              int
    L3ArchiveThreshold                float64
    L3MaxPerRecallChars               int
    L4Enabled                         bool
    L4GraphInjectNeighbors            bool
    L4GraphMaxNeighbors               int
    L4GraphMaxHops                    int
    L4IdentityInject                  bool
    L4StrategyInject                  bool
    EvoEnabled                        bool
    EvoAutoApply                      bool
    EvoMinEpisodes                    int
    EvoMinNegativeFeedback            int
    EvoThrottleHours                  int
    EvoProposalTTLDays                int
    EvoPersonaMaxChars                int
    EvoSystemPromptMaxAppends         int
    SkillRuntimeJSON                  string
    IntentPassEnabled                 bool
    CreatedAt                         string
    UpdatedAt                         string
}

type AgentPromptFile struct {
    ID        string
    AgentID   string
    Name      string
    Body      string
    SortOrder int
    CreatedAt string
    UpdatedAt string
}
```

### 3.2 Repo 接口

文件：`internal/biz/agent_usecase.go`

```go
type AgentRepository interface {
    SearchAgents(ctx context.Context, q AgentListQuery) (AgentListResult, error)
    GetAgentByID(ctx context.Context, id string) (Agent, error)
    GetAgentByAgentKey(ctx context.Context, agentKey string) (Agent, error)
    CreateAgent(ctx context.Context, a Agent) (Agent, error)
    UpdateAgent(ctx context.Context, a Agent) (Agent, error)
    DeleteAgent(ctx context.Context, id string) error
    GetAgentRuntimeSettings(ctx context.Context, agentID string) (AgentRuntimeSettings, error)
    UpsertAgentRuntimeSettings(ctx context.Context, v AgentRuntimeSettings) (AgentRuntimeSettings, error)
    ListAgentPromptFiles(ctx context.Context, agentID string) ([]AgentPromptFile, error)
    ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []AgentPromptFile) ([]AgentPromptFile, error)
}
```

### 3.3 Usecase

文件：`internal/biz/agent_usecase.go`

```go
type AgentUsecase struct {
    repo  AgentRepository
    tools ToolRepo
}

func NewAgentUsecase(repo AgentRepository, tools ToolRepo) *AgentUsecase

func (u *AgentUsecase) List(ctx context.Context, q AgentListQuery) (AgentListResult, error)
func (u *AgentUsecase) Get(ctx context.Context, id string) (Agent, error)
func (u *AgentUsecase) Create(ctx context.Context, in Agent) (Agent, error)
func (u *AgentUsecase) Update(ctx context.Context, id string, patch Agent) (Agent, error)
func (u *AgentUsecase) Delete(ctx context.Context, id string) error
func (u *AgentUsecase) PromptPreview(ctx context.Context, id, mode string) (string, error)
```

**Get 方法**：获取 Agent 后自动 hydrate Settings 和 Files。若 Settings 不存在则从 `config_json` 迁移并 Upsert；若 Files 为空则从 `config_json` 迁移并 Replace。

**Update 方法**：merge patch 到 current，然后同步更新 Agent 主表、Settings、Files、config_json 四处。

---

## 四、Data 层

### 4.1 Ent Schema

**Agent 主表** — `internal/data/ent/schema/agent.go`

```go
func (Agent) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("agent_key").Unique().MaxLen(512),
        field.String("display_name").MaxLen(1024),
        field.String("provider"),
        field.String("model"),
        field.String("status").Default("active"),
        field.Bool("is_default").Default(false),
        field.Bool("is_favorite").Default(false),
        field.String("icon").Default(""),
        field.Text("agent_description").Default(""),
        field.String("category_position_id").Default(""),
        field.String("system_prompt_mode").Default(""),
        field.Int("context_window").Default(0),
        field.Int("budget_monthly_cents").Default(0),
        field.Text("config_json").Default(""),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
        field.String("deleted_at").Default(""),
    }
}
```

**AgentRuntimeSetting** — `internal/data/ent/schema/agent_runtime_setting.go`

```go
func (AgentRuntimeSetting) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").StorageKey("agent_id").Unique().Immutable().MaxLen(256),
        field.Bool("self_evolve").Default(true),
        field.Bool("subagents_enabled").Default(true),
        field.Int("subagents_max_concurrency").Default(20),
        field.Int("subagents_max_generation_depth").Default(1),
        field.Int("subagents_max_children_per_agent").Default(5),
        field.Int("subagents_archive_after_minutes").Default(60),
        field.Int("subagents_max_retries").Default(2),
        field.String("subagents_model_override").Default(""),
        field.Bool("tools_enabled").Default(true),
        field.String("tools_profile").Default("full"),
        field.String("tools_tool_call_prefix").Default(""),
        field.String("tools_allow_json").Default("[]"),
        field.String("tools_deny_json").Default("[]"),
        field.String("tools_concurrent_allow_json").Default("[]"),
        field.Bool("memory_enabled").Default(true),
        field.Int("memory_max_chunk_length").Default(1000),
        field.Int("memory_max_results").Default(6),
        field.Float("memory_min_score").Default(0.35),
        field.Bool("heartbeat_enabled").Default(false),
        field.Int("heartbeat_interval_minutes").Default(30),
        field.Bool("evolution_self_evolve").Default(true),
        field.Bool("evolution_skill_evolve").Default(true),
        field.Bool("evolution_metrics_enabled").Default(true),
        field.Bool("evolution_suggestions_enabled").Default(true),
        field.Float("guardrail_max_change_per_period").Default(0.1),
        field.Int("guardrail_min_data_points").Default(100),
        field.Int("guardrail_rollback_on_decline_percent").Default(20),
        field.Int("l0_recent_window_turns").Default(12),
        field.Int("l0_recent_window_tokens").Default(0),
        field.Float("l0_summary_threshold").Default(0.6),
        field.Int("l0_summary_keep_turns").Default(4),
        field.String("l0_compress_provider").Default(""),
        field.String("l0_compress_model").Default(""),
        field.String("l0_truncate_strategy").Default("summary"),
        field.Bool("l0_inject_l1").Default(true),
        field.Bool("l0_inject_l3").Default(true),
        field.Bool("l0_inject_l4").Default(false),
        field.Int("l0_l3_max_chunks").Default(5),
        field.Int("l0_l4_max_paths").Default(3),
        field.String("l0_snapshot_mode").Default("on_warning"),
        field.Bool("l1_enabled").Default(true),
        field.Int("l1_budget_tokens").Default(8192),
        field.Int("l1_field_max_tokens").Default(2048),
        field.Int("l1_history_keep_revisions").Default(10),
        field.String("l1_default_schema_id").Default(""),
        field.Int("l1_archive_on_idle_minutes").Default(60),
        field.Bool("l2_episode_enabled").Default(true),
        field.Float("l2_episode_min_importance").Default(0.3),
        field.Bool("l2_index_enabled").Default(true),
        field.String("l2_index_embedding_model").Default(""),
        field.Bool("l2_recall_enabled").Default(false),
        field.Int("l2_recall_max").Default(3),
        field.Int("l2_retention_days").Default(90),
        field.Int("l2_archive_after_days").Default(30),
        field.Bool("l3_enabled").Default(true),
        field.Int("l3_recall_top_k").Default(5),
        field.Float("l3_recall_min_score").Default(0.55),
        field.String("l3_recall_scopes_json").Default("[\"agent\",\"user\",\"team\",\"workspace\"]"),
        field.String("l3_embedding_model").Default(""),
        field.Int("l3_decay_interval_hours").Default(24),
        field.Float("l3_archive_threshold").Default(0.2),
        field.Int("l3_max_per_recall_chars").Default(1500),
        field.Bool("l4_enabled").Default(true),
        field.Bool("l4_graph_inject_neighbors").Default(true),
        field.Int("l4_graph_max_neighbors").Default(6),
        field.Int("l4_graph_max_hops").Default(2),
        field.Bool("l4_identity_inject").Default(true),
        field.Bool("l4_strategy_inject").Default(false),
        field.Bool("evo_enabled").Default(false),
        field.Bool("evo_auto_apply").Default(false),
        field.Int("evo_min_episodes").Default(20),
        field.Int("evo_min_negative_feedback").Default(3),
        field.Int("evo_throttle_hours").Default(24),
        field.Int("evo_proposal_ttl_days").Default(14),
        field.Int("evo_persona_max_chars").Default(1500),
        field.Int("evo_system_prompt_max_appends").Default(5),
        field.String("skill_runtime_json").Default("{}"),
        field.Bool("intent_pass_enabled").Default(true),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
    }
}
```

### 4.2 Repo 实现

文件：`internal/data/agent_repo.go`

**Ent→Biz 转换函数**：

```go
func entAgentToBiz(a *ent.Agent) biz.Agent
func entRuntimeToBiz(e *ent.AgentRuntimeSetting) biz.AgentRuntimeSettings
func entPromptToBiz(e *ent.AgentPromptFile) biz.AgentPromptFile
```

**关键方法**：

```go
func (r *agentRepo) GetAgentByID(ctx context.Context, id string) (biz.Agent, error) {
    row, err := r.data.entClient.Agent.Query().
        Where(agent.IDEQ(id), agent.DeletedAtEQ("")).
        Only(ctx)
    if ent.IsNotFound(err) {
        return biz.Agent{}, sql.ErrNoRows
    }
    return entAgentToBiz(row), nil
}

func (r *agentRepo) UpsertAgentRuntimeSettings(ctx context.Context, v biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error) {
    b := r.data.entClient.AgentRuntimeSetting.Create().SetID(v.AgentID)
    applyBizRuntimeToCreate(b, v)
    b.OnConflict(
        entsql.ConflictColumns(agentruntimesetting.FieldID),
        entsql.ResolveWithNewValues(),
    ).Exec(ctx)
    row, _ := r.data.entClient.AgentRuntimeSetting.Get(ctx, v.AgentID)
    return entRuntimeToBiz(row), nil
}

func (r *agentRepo) ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []biz.AgentPromptFile) ([]biz.AgentPromptFile, error) {
    tx, _ := r.data.entClient.Tx(ctx)
    tx.AgentPromptFile.Delete().Where(agentpromptfile.AgentIDEQ(agentID)).Exec(ctx)
    for i, file := range files {
        id := file.ID
        if id == "" {
            id = fmt.Sprintf("%s_%s", agentID, sanitizePromptFileID(file.Name))
        }
        sortOrder := file.SortOrder
        if sortOrder == 0 {
            sortOrder = (i + 1) * 10
        }
        tx.AgentPromptFile.Create().
            SetID(id).SetAgentID(agentID).SetFileName(file.Name).
            SetBody(file.Body).SetSortOrder(sortOrder).
            Save(ctx)
    }
    tx.Commit()
    return r.ListAgentPromptFiles(ctx, agentID)
}
```

---

## 五、Service 层

文件：`internal/service/agent.go`

### 5.1 Service 结构体

```go
type AgentService struct {
    v1.UnimplementedAgentServiceServer
    uc *biz.AgentUsecase
}

func NewAgentService(uc *biz.AgentUsecase) *AgentService
```

### 5.2 类型转换函数

```go
func fromProtoRuntime(pb *v1.AgentRuntimeSettings) *biz.AgentRuntimeSettings
func toProtoRuntime(b *biz.AgentRuntimeSettings) *v1.AgentRuntimeSettings
func fromProtoFile(pb *v1.AgentPromptFile) biz.AgentPromptFile
func toProtoFile(b biz.AgentPromptFile) *v1.AgentPromptFile
func fromProtoAgent(pb *v1.Agent) biz.Agent
func toProtoAgent(b biz.Agent) *v1.Agent
func fromProtoCreate(req *v1.CreateAgentRequest) biz.Agent
```

### 5.3 RPC 实现

```go
func (s *AgentService) GetAgent(ctx context.Context, req *v1.GetAgentRequest) (*v1.Agent, error) {
    a, err := s.uc.Get(ctx, req.GetId())
    if stderrors.Is(err, sql.ErrNoRows) {
        return nil, kerrors.NotFound("AGENT", "agent not found")
    }
    return toProtoAgent(a), nil
}

func (s *AgentService) UpdateAgent(ctx context.Context, req *v1.UpdateAgentRequest) (*v1.Agent, error) {
    if req.GetAgent() == nil {
        return nil, kerrors.BadRequest("AGENT", "agent body is required")
    }
    patch := fromProtoAgent(req.GetAgent())
    a, err := s.uc.Update(ctx, req.GetId(), patch)
    if stderrors.Is(err, sql.ErrNoRows) {
        return nil, kerrors.NotFound("AGENT", "agent not found")
    }
    return toProtoAgent(a), nil
}

func (s *AgentService) GetAgentPromptPreview(ctx context.Context, req *v1.GetAgentPromptPreviewRequest) (*v1.GetAgentPromptPreviewResponse, error) {
    text, err := s.uc.PromptPreview(ctx, req.GetId(), req.GetMode())
    if stderrors.Is(err, sql.ErrNoRows) {
        return nil, kerrors.NotFound("AGENT", "agent not found")
    }
    return &v1.GetAgentPromptPreviewResponse{Preview: text}, nil
}

func (s *AgentService) GetAgentEffectiveTools(ctx context.Context, req *v1.GetAgentEffectiveToolsRequest) (*v1.AgentEffectiveToolsView, error) {
    out, err := s.uc.GetEffectiveTools(ctx, req.GetAgentId())
    if stderrors.Is(err, sql.ErrNoRows) {
        return nil, kerrors.NotFound("AGENT", "agent not found")
    }
    return bizEffectiveToolsToProto(out), nil
}

func (s *AgentService) UpdateAgentToolPolicy(ctx context.Context, req *v1.UpdateAgentToolPolicyRequest) (*v1.AgentEffectiveToolsView, error) {
    in := biz.AgentToolPolicyInput{
        ToolsEnabled: req.GetToolsEnabled(),
        Profile:      req.GetProfile(),
        Allow:        req.GetAllow(),
        Deny:         req.GetDeny(),
    }
    out, err := s.uc.UpdateAgentToolPolicy(ctx, req.GetAgentId(), in)
    if stderrors.Is(err, sql.ErrNoRows) {
        return nil, kerrors.NotFound("AGENT", "agent not found")
    }
    return bizEffectiveToolsToProto(out), nil
}
```

---

## 六、Wire 注入

已有，无需新增：

```
data.ProviderSet → NewAgentRepo
biz.ProviderSet → NewAgentUsecase
service.ProviderSet → NewAgentService
```

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/features/agents/
├── api.ts                    ← API 调用函数
├── types.ts                  ← TypeScript 类型定义
├── wireNormalize.ts          ← Wire 数据规范化
├── useAgentsPage.ts          ← 列表页 Composable
└── components/
    ├── AgentSettingsPage.vue       ← 主页面（QTabs + QTabPanels）
    ├── AgentHeader.vue             ← 顶栏（头像/名称/状态/标签/操作）
    ├── AgentIdentityTab.vue        ← 身份 Tab
    ├── AgentModelTab.vue           ← 模型与预算 Tab
    ├── AgentPromptModeTab.vue      ← 系统提示模式 Tab
    ├── AgentCapabilitiesTab.vue    ← 能力 Tab（子Agent/工具策略）
    ├── AgentMemoryTab.vue          ← 记忆 Tab（L0-L4）
    ├── AgentEvolutionTab.vue       ← 进化 Tab
    ├── AgentHeartbeatCard.vue      ← 心跳卡片
    ├── AgentFilesTab.vue           ← 文件 Tab（见 6 agent-setting-file.design.md）
    └── AgentPermissionsTab.vue     ← 权限 Tab
```

### 7.2 TypeScript 类型

文件：`web/src/features/agents/types.ts`

```typescript
export type Agent = {
  id: string;
  agent_key: string;
  display_name: string;
  provider: string;
  model: string;
  status: string;
  is_default: boolean;
  is_favorite: boolean;
  icon: string;
  agent_description: string;
  category_position_id: string;
  system_prompt_mode: string;
  context_window: number;
  budget_monthly_cents: number;
  config_json: string;
  created_at: string;
  updated_at: string;
  deleted_at: string;
  settings?: AgentRuntimeSettings;
  files?: AgentPromptFile[];
};

export type AgentRuntimeSettings = {
  agent_id?: string;
  self_evolve: boolean;
  subagents_enabled: boolean;
  subagents_max_concurrency: number;
  subagents_max_generation_depth: number;
  subagents_max_children_per_agent: number;
  subagents_archive_after_minutes: number;
  subagents_max_retries: number;
  subagents_model_override: string;
  tools_enabled: boolean;
  tools_profile: string;
  tools_tool_call_prefix: string;
  tools_allow_json: string;
  tools_deny_json: string;
  tools_concurrent_allow_json: string;
  memory_enabled: boolean;
  memory_max_chunk_length: number;
  memory_max_results: number;
  memory_min_score: number;
  heartbeat_enabled: boolean;
  heartbeat_interval_minutes: number;
  evolution_self_evolve: boolean;
  evolution_skill_evolve: boolean;
  evolution_metrics_enabled: boolean;
  evolution_suggestions_enabled: boolean;
  guardrail_max_change_per_period: number;
  guardrail_min_data_points: number;
  guardrail_rollback_on_decline_percent: number;
  l0_recent_window_turns?: number;
  l0_recent_window_tokens?: number;
  l0_summary_threshold?: number;
  l0_summary_keep_turns?: number;
  l0_truncate_strategy?: string;
  l0_inject_l1?: boolean;
  l0_inject_l3?: boolean;
  l0_inject_l4?: boolean;
  l0_l3_max_chunks?: number;
  l0_l4_max_paths?: number;
  l0_snapshot_mode?: string;
  l1_enabled?: boolean;
  l1_budget_tokens?: number;
  l1_field_max_tokens?: number;
  l1_history_keep_revisions?: number;
  l1_default_schema_id?: string;
  l1_archive_on_idle_minutes?: number;
  l2_episode_enabled?: boolean;
  l2_episode_min_importance?: number;
  l2_index_enabled?: boolean;
  l2_index_embedding_model?: string;
  l2_recall_enabled?: boolean;
  l2_recall_max?: number;
  l2_retention_days?: number;
  l2_archive_after_days?: number;
  l3_enabled?: boolean;
  l3_recall_top_k?: number;
  l3_recall_min_score?: number;
  l3_recall_scopes_json?: string;
  l3_embedding_model?: string;
  l3_decay_interval_hours?: number;
  l3_archive_threshold?: number;
  l3_max_per_recall_chars?: number;
  l4_enabled?: boolean;
  l4_graph_inject_neighbors?: boolean;
  l4_graph_max_neighbors?: number;
  l4_graph_max_hops?: number;
  l4_identity_inject?: boolean;
  l4_strategy_inject?: boolean;
  evo_enabled?: boolean;
  evo_auto_apply?: boolean;
  evo_min_episodes?: number;
  evo_min_negative_feedback?: number;
  evo_throttle_hours?: number;
  evo_proposal_ttl_days?: number;
  evo_persona_max_chars?: number;
  evo_system_prompt_max_appends?: number;
  skill_runtime_json?: string;
  intent_pass_enabled?: boolean;
  created_at?: string;
  updated_at?: string;
};

export type AgentPromptFile = {
  id?: string;
  agent_id?: string;
  name: string;
  body: string;
  sort_order: number;
  created_at?: string;
  updated_at?: string;
};
```

### 7.3 API 函数

文件：`web/src/features/agents/api.ts`

```typescript
export async function getAgent(id: string): Promise<Agent>
export async function updateAgent(id: string, payload: Partial<Agent>): Promise<Agent>
export async function getAgentPromptPreview(id: string, mode?: string): Promise<string>
export async function deleteAgent(id: string): Promise<void>
```

### 7.4 组件详细设计

#### AgentSettingsPage.vue

```vue
<template>
  <QPage padding>
    <AgentHeader :agent="agent" @toggle-favorite="onToggleFavorite" @delete="onDelete" />

    <QTabs v-model="activeTab" class="q-mt-md">
      <QTab name="agent" label="Agent" />
      <QTab name="files" label="文件" />
      <QTab name="permissions" label="权限" />
      <QTab name="evolution" label="进化" />
      <QTab name="hooks" label="钩子" />
    </QTabs>

    <QTabPanels v-model="activeTab">
      <QTabPanel name="agent">
        <AgentIdentityTab :agent="agent" @update="onUpdate" />
        <AgentModelTab :agent="agent" @update="onUpdate" />
        <AgentPromptModeTab :agent="agent" @update="onUpdate" />
        <AgentCapabilitiesTab :agent="agent" @update="onUpdate" />
        <AgentMemoryTab :agent="agent" @update="onUpdate" />
        <AgentHeartbeatCard :agent="agent" @update="onUpdate" />
      </QTabPanel>
      <QTabPanel name="files">
        <AgentFilesTab :agent="agent" @update="onUpdate" />
      </QTabPanel>
      <QTabPanel name="permissions">
        <AgentPermissionsTab :agent="agent" />
      </QTabPanel>
      <QTabPanel name="evolution">
        <AgentEvolutionTab :agent="agent" @update="onUpdate" />
      </QTabPanel>
      <QTabPanel name="hooks">
        <div>钩子管理（见 hook 模块设计）</div>
      </QTabPanel>
    </QTabPanels>
  </QPage>
</template>
```

#### AgentHeader.vue

```vue
<template>
  <div class="row items-center q-gutter-sm q-pb-md">
    <QBtn flat round icon="arrow_back" @click="$router.back()" />
    <QAvatar rounded size="48px">
      <img v-if="agent.icon" :src="agent.icon" />
      <QIcon v-else name="smart_toy" />
    </QAvatar>
    <div class="col">
      <div class="row items-center q-gutter-xs">
        <span class="text-h6">{{ agent.display_name }}</span>
        <QBadge v-if="agent.status === 'active'" color="positive" rounded>●</QBadge>
        <QBadge v-if="agent.system_prompt_mode" color="info" :label="agent.system_prompt_mode" />
      </div>
      <div class="text-caption text-grey">
        {{ agent.agent_key }} · {{ agent.provider }} / {{ agent.model }}
      </div>
    </div>
    <QBtn flat round :icon="agent.is_favorite ? 'favorite' : 'favorite_border'"
      :color="agent.is_favorite ? 'red' : undefined"
      @click="$emit('toggle-favorite')" />
    <QBtn flat round icon="visibility" @click="showPreview = true">
      <QTooltip>系统提示词预览</QTooltip>
    </QBtn>
    <QBtn flat round icon="delete" color="negative" @click="$emit('delete')">
      <QTooltip>删除</QTooltip>
    </QBtn>

    <QDialog v-model="showPreview" maximized>
      <QCard>
        <QBar>
          <span>系统提示词预览</span>
          <QSpace />
          <QBtn dense flat icon="close" v-close-popup />
        </QBar>
        <QTabs v-model="previewMode">
          <QTab name="complete" label="完整" />
          <QTab name="task" label="任务" />
          <QTab name="minimized" label="最小化" />
          <QTab name="none" label="无" />
        </QTabs>
        <QCardSection style="max-height: 70vh" class="scroll">
          <pre>{{ previewText }}</pre>
        </QCardSection>
      </QCard>
    </QDialog>
  </div>
</template>
```

#### AgentIdentityTab.vue

```vue
<template>
  <QCard class="q-mb-md">
    <QCardSection>
      <div class="text-h6">身份</div>
    </QCardSection>
    <QCardSection class="q-gutter-md">
      <QInput v-model="form.display_name" label="显示名称" outlined dense
        @blur="emitUpdate('display_name', form.display_name)" />
      <QInput v-model="form.agent_key" label="Agent 标识" outlined dense readonly>
        <template v-slot:append>
          <QBtn flat round icon="content_copy" size="sm" @click="copyKey" />
        </template>
      </QInput>
      <QInput v-model="form.agent_description" label="专业摘要" type="textarea" outlined dense autogrow
        @blur="emitUpdate('agent_description', form.agent_description)" />
      <QSelect v-model="form.status" :options="statusOptions" label="状态" outlined dense emit-value map-options
        @update:model-value="emitUpdate('status', form.status)" />
      <QToggle v-model="form.is_default" label="默认 Agent"
        @update:model-value="emitUpdate('is_default', form.is_default)" />
    </QCardSection>
  </QCard>
</template>
```

#### AgentModelTab.vue

```vue
<template>
  <QCard class="q-mb-md">
    <QCardSection>
      <div class="text-h6">模型与预算</div>
    </QCardSection>
    <QCardSection class="q-gutter-md">
      <div class="row q-gutter-sm">
        <QSelect v-model="form.provider" :options="providerOptions" label="Provider" outlined dense class="col"
          @update:model-value="onProviderChange" />
        <QSelect v-model="form.model" :options="modelOptions" label="模型" outlined dense class="col"
          @update:model-value="emitUpdate('model', form.model)" />
      </div>
      <QInput v-model.number="form.context_window" type="number" label="上下文窗口" outlined dense
        @blur="emitUpdate('context_window', form.context_window)" />
      <QInput v-model.number="form.budget_monthly_cents" type="number" label="预算限额 (cents)" outlined dense
        prefix="$" @blur="emitUpdate('budget_monthly_cents', form.budget_monthly_cents)" />
    </QCardSection>
  </QCard>
</template>
```

#### AgentPromptModeTab.vue

```vue
<template>
  <QCard class="q-mb-md">
    <QCardSection>
      <div class="text-h6">系统提示模式</div>
    </QCardSection>
    <QCardSection>
      <div class="row q-gutter-sm">
        <QCard v-for="mode in promptModes" :key="mode.value"
          clickable :class="{ 'selected-mode': form.system_prompt_mode === mode.value }"
          class="col cursor-pointer" @click="selectMode(mode.value)">
          <QCardSection>
            <div class="text-subtitle1">{{ mode.label }}</div>
            <div class="text-caption">{{ mode.description }}</div>
          </QCardSection>
        </QCard>
      </div>
    </QCardSection>
  </QCard>
</template>

<script setup lang="ts">
const promptModes = [
  { value: 'complete', label: '完整', description: '交互聊天 + 完整人格类能力' },
  { value: 'task', label: '任务', description: '企业自动化、记忆、进化' },
  { value: 'minimized', label: '最小化', description: '后台任务、核心规则' },
  { value: 'none', label: '无', description: '纯工具调用自动化' },
]
</script>
```

#### AgentCapabilitiesTab.vue

```vue
<template>
  <div>
    <QCard class="q-mb-md">
      <QCardSection class="row items-center">
        <div class="text-h6 col">子 Agent</div>
        <QToggle v-model="settings.subagents_enabled"
          @update:model-value="emitSettingsUpdate('subagents_enabled', $event)" />
      </QCardSection>
      <QCardSection v-if="settings.subagents_enabled" class="q-gutter-md">
        <div class="row q-gutter-sm">
          <QInput v-model.number="settings.subagents_max_concurrency" type="number" min="1"
            label="最大并发数" outlined dense class="col"
            @blur="emitSettingsUpdate('subagents_max_concurrency', settings.subagents_max_concurrency)" />
          <QInput v-model.number="settings.subagents_max_generation_depth" type="number" min="1"
            label="最大生成深度" outlined dense class="col"
            @blur="emitSettingsUpdate('subagents_max_generation_depth', settings.subagents_max_generation_depth)" />
        </div>
        <div class="row q-gutter-sm">
          <QInput v-model.number="settings.subagents_max_children_per_agent" type="number" min="1"
            label="每 Agent 最大子数" outlined dense class="col"
            @blur="emitSettingsUpdate('subagents_max_children_per_agent', settings.subagents_max_children_per_agent)" />
          <QInput v-model.number="settings.subagents_archive_after_minutes" type="number" min="1"
            label="归档时间 (min)" outlined dense class="col"
            @blur="emitSettingsUpdate('subagents_archive_after_minutes', settings.subagents_archive_after_minutes)" />
        </div>
        <div class="row q-gutter-sm">
          <QInput v-model.number="settings.subagents_max_retries" type="number" min="0"
            label="最大重试次数" outlined dense class="col"
            @blur="emitSettingsUpdate('subagents_max_retries', settings.subagents_max_retries)" />
          <QInput v-model="settings.subagents_model_override"
            label="模型覆盖" outlined dense class="col"
            placeholder="继承自 Agent"
            @blur="emitSettingsUpdate('subagents_model_override', settings.subagents_model_override)" />
        </div>
      </QCardSection>
    </QCard>

    <QCard class="q-mb-md">
      <QCardSection class="row items-center">
        <div class="text-h6 col">工具策略</div>
        <QToggle v-model="settings.tools_enabled"
          @update:model-value="emitSettingsUpdate('tools_enabled', $event)" />
      </QCardSection>
      <QCardSection v-if="settings.tools_enabled" class="q-gutter-md">
        <QSelect v-model="settings.tools_profile" :options="profileOptions" label="配置文件" outlined dense
          @update:model-value="emitSettingsUpdate('tools_profile', $event)" />
        <QInput v-model="settings.tools_tool_call_prefix" label="工具调用前缀" outlined dense
          placeholder="e.g. proxy_"
          @blur="emitSettingsUpdate('tools_tool_call_prefix', settings.tools_tool_call_prefix)" />
        <QSelect v-model="allowList" :options="builtinTools" multiple use-chips label="允许" outlined dense />
        <QSelect v-model="denyList" :options="builtinTools" multiple use-chips label="拒绝" outlined dense />
        <QSelect v-model="concurrentAllowList" :options="builtinTools" multiple use-chips label="同时允许" outlined dense />
        <QBanner v-if="conflictTools.length" class="bg-warning text-dark">
          以下工具在允许与拒绝中重复，已按拒绝处理：{{ conflictTools.join(', ') }}
        </QBanner>
      </QCardSection>
    </QCard>
  </div>
</template>
```

#### AgentMemoryTab.vue

```vue
<template>
  <QCard class="q-mb-md">
    <QCardSection class="row items-center">
      <div class="text-h6 col">记忆</div>
      <QToggle v-model="settings.memory_enabled"
        @update:model-value="emitSettingsUpdate('memory_enabled', $event)" />
    </QCardSection>
    <QCardSection v-if="settings.memory_enabled">
      <div class="text-subtitle2 q-mb-sm">检索参数</div>
      <div class="row q-gutter-sm q-mb-md">
        <QInput v-model.number="settings.memory_max_chunk_length" type="number" label="最大块长度" outlined dense class="col"
          @blur="emitSettingsUpdate('memory_max_chunk_length', settings.memory_max_chunk_length)" />
        <QInput v-model.number="settings.memory_max_results" type="number" label="最大结果数" outlined dense class="col"
          @blur="emitSettingsUpdate('memory_max_results', settings.memory_max_results)" />
        <QInput v-model.number="settings.memory_min_score" type="number" step="0.05" label="最低分数" outlined dense class="col"
          @blur="emitSettingsUpdate('memory_min_score', settings.memory_min_score)" />
      </div>

      <QSeparator class="q-mb-md" />
      <div class="text-subtitle2 q-mb-sm">L0 感官层</div>
      <div class="row q-gutter-sm q-mb-md">
        <QInput v-model.number="settings.l0_recent_window_turns" type="number" label="最近窗口轮次" outlined dense class="col" />
        <QInput v-model.number="settings.l0_recent_window_tokens" type="number" label="最近窗口 Token" outlined dense class="col" />
        <QInput v-model.number="settings.l0_summary_threshold" type="number" step="0.1" label="摘要阈值" outlined dense class="col" />
      </div>

      <QSeparator class="q-mb-md" />
      <div class="text-subtitle2 q-mb-sm">L1 工作层</div>
      <div class="row q-gutter-sm q-mb-md">
        <QToggle v-model="settings.l1_enabled" label="启用 L1" class="col-12"
          @update:model-value="emitSettingsUpdate('l1_enabled', $event)" />
        <QInput v-if="settings.l1_enabled" v-model.number="settings.l1_budget_tokens" type="number" label="预算 Token" outlined dense class="col" />
      </div>

      <QSeparator class="q-mb-md" />
      <div class="text-subtitle2 q-mb-sm">L2 情景层</div>
      <QToggle v-model="settings.l2_episode_enabled" label="启用 L2 情景" class="col-12"
        @update:model-value="emitSettingsUpdate('l2_episode_enabled', $event)" />

      <QSeparator class="q-mb-md" />
      <div class="text-subtitle2 q-mb-sm">L3 语义层</div>
      <QToggle v-model="settings.l3_enabled" label="启用 L3 语义" class="col-12"
        @update:model-value="emitSettingsUpdate('l3_enabled', $event)" />

      <QSeparator class="q-mb-md" />
      <div class="text-subtitle2 q-mb-sm">L4 持久层</div>
      <QToggle v-model="settings.l4_enabled" label="启用 L4 持久" class="col-12"
        @update:model-value="emitSettingsUpdate('l4_enabled', $event)" />
    </QCardSection>
  </QCard>
</template>
```

#### AgentHeartbeatCard.vue

```vue
<template>
  <QCard class="q-mb-md">
    <QCardSection>
      <div class="row items-center">
        <QIcon :name="settings.heartbeat_enabled ? 'favorite' : 'favorite_border'"
          :color="settings.heartbeat_enabled ? 'red' : 'grey'" size="sm" class="q-mr-sm" />
        <div class="text-h6">心跳</div>
      </div>
    </QCardSection>
    <QCardSection class="q-gutter-md">
      <QToggle v-model="settings.heartbeat_enabled" label="启用心跳"
        @update:model-value="emitSettingsUpdate('heartbeat_enabled', $event)" />
      <QInput v-if="settings.heartbeat_enabled" v-model.number="settings.heartbeat_interval_minutes"
        type="number" min="1" label="间隔 (分钟)" outlined dense
        suffix="min"
        @blur="emitSettingsUpdate('heartbeat_interval_minutes', settings.heartbeat_interval_minutes)" />
    </QCardSection>
  </QCard>
</template>
```

### 7.5 自动保存策略

| 字段类型 | 保存方式 | 实现 |
|----------|----------|------|
| 文本字段 | `debounce(500ms)` + `PATCH /v1/agents/{id}` | `QInput @blur` 触发 |
| Toggle 字段 | `@update:model-value` 立即 PATCH | `QToggle` 直接触发 |
| Settings 字段 | 通过 `updateAgent` 整体 PATCH | Settings 变更合并到 Agent 对象 |
| 工具策略 | `PUT /v1/agents/{id}/tools/policy` | `UpdateAgentToolPolicy` 独立 RPC |

### 7.6 数据规范化

文件：`web/src/features/agents/wireNormalize.ts`

核心函数：
- `normalizeAgentFromService(raw: unknown): Agent` — 将 Wire 响应规范化为 snake_case 类型
- `normalizeRuntimeSettingsFromWire(raw: unknown): AgentRuntimeSettings | undefined`
- `normalizePromptFileFromWire(raw: unknown): AgentPromptFile`
- `runtimeSettingsToWire(s: AgentRuntimeSettings): KratosRuntimeWire` — snake_case → camelCase
- `promptFileToWire(f: AgentPromptFile): KratosFileWire`
- `partialAgentToWire(payload: Partial<Agent>): KratosAgentWire` — 部分更新映射
