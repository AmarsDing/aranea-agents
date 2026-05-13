# Agent 创建模块 — 实现设计文档

> 对应需求：`2 agents-create.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 创建弹窗，采集最小字段创建 Agent 行。复用 `AgentService.CreateAgent` RPC，前端为 `QDialog` 表单。创建时同步持久化 `AgentRuntimeSettings` 和 `AgentPromptFile`，并通过 `config_json` 保持向后兼容。

---

## 二、Proto 层

### 2.1 现有 Proto 定义

文件：`api/kratos/agent/v1/agent.proto`

```protobuf
message CreateAgentRequest {
  string agent_key = 1 [(google.api.field_behavior) = REQUIRED];
  string display_name = 2 [(google.api.field_behavior) = REQUIRED];
  string provider = 3 [(google.api.field_behavior) = REQUIRED];
  string model = 4 [(google.api.field_behavior) = REQUIRED];
  string icon = 5;
  string agent_description = 6;
  string category_position_id = 7;
  string system_prompt_mode = 8;
  int32 context_window = 9;
  int32 budget_monthly_cents = 10;
  string config_json = 11;
  AgentRuntimeSettings settings = 12;
  repeated AgentPromptFile files = 13;
}

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

rpc CreateAgent(CreateAgentRequest) returns (Agent) {
  option (google.api.http) = { post: "/v1/agents" body: "*" };
}
```

### 2.2 消息字段说明

| 消息 | 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `CreateAgentRequest` | `agent_key` | string | ✅ | 唯一标识，小写字母数字连字符 |
| | `display_name` | string | ✅ | 显示名称，max 255 |
| | `provider` | string | ✅ | LLM 提供商 slug |
| | `model` | string | ✅ | 模型名，需通过校验 |
| | `icon` | string | ❌ | 头像资源 ID（`avatar_assets.id`） |
| | `agent_description` | string | ❌ | Agent 描述/人设 |
| | `category_position_id` | string | ❌ | 业务分类叶子节点 ID |
| | `system_prompt_mode` | string | ❌ | 提示词模式：simple/complete |
| | `context_window` | int32 | ❌ | 上下文窗口大小，默认 128000 |
| | `budget_monthly_cents` | int32 | ❌ | 月预算（美分） |
| | `config_json` | string | ❌ | 兼容旧配置 JSON |
| | `settings` | AgentRuntimeSettings | ❌ | 运行时设置 |
| | `files` | AgentPromptFile[] | ❌ | 提示词文件列表 |

### 2.3 无需新增 Proto

创建弹窗是纯前端交互，调用已有 `CreateAgent` RPC。

---

## 三、Biz 层

### 3.1 领域模型

```go
// internal/biz/agent_types.go

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

### 3.2 Repository 接口

```go
// internal/biz/agent_usecase.go

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

### 3.3 Usecase 方法

```go
// internal/biz/agent_usecase.go

type AgentUsecase struct {
    repo  AgentRepository
    tools ToolRepo
}

func NewAgentUsecase(repo AgentRepository, tools ToolRepo) *AgentUsecase {
    return &AgentUsecase{repo: repo, tools: tools}
}

func (u *AgentUsecase) Create(ctx context.Context, in Agent) (Agent, error) {
    in.AgentKey = strings.TrimSpace(in.AgentKey)
    in.DisplayName = strings.TrimSpace(in.DisplayName)
    in.Provider = strings.TrimSpace(in.Provider)
    in.Model = strings.TrimSpace(in.Model)
    if in.AgentKey == "" || in.DisplayName == "" || in.Provider == "" || in.Model == "" {
        return Agent{}, kerrors.BadRequest("AGENT", "agent_key, display_name, provider, and model are required")
    }
    if in.ID == "" {
        in.ID = newAgentCatalogID()
    }
    settings := withSettingDefaults(settingsFromAgentInput(in))
    settings.AgentID = in.ID
    files := filesFromAgentInput(in)
    for i := range files {
        files[i].AgentID = in.ID
    }
    files = withFileDefaults(files)
    if strings.TrimSpace(in.ConfigJSON) == "" {
        in.ConfigJSON = configJSONFromSettings(settings, files)
    }
    in.Status = "active"
    if _, err := u.repo.CreateAgent(ctx, in); err != nil {
        return Agent{}, err
    }
    if _, err := u.repo.UpsertAgentRuntimeSettings(ctx, settings); err != nil {
        return Agent{}, err
    }
    if _, err := u.repo.ReplaceAgentPromptFiles(ctx, in.ID, files); err != nil {
        return Agent{}, err
    }
    if _, err := u.syncConfigJSON(ctx, in.ID, settings, files); err != nil {
        return Agent{}, err
    }
    return u.Get(ctx, in.ID)
}
```

### 3.4 校验逻辑

| 校验项 | 规则 | 错误码 |
|--------|------|--------|
| `agent_key` 非空 | Trim 后非空 | `BadRequest("AGENT", "agent_key is required")` |
| `display_name` 非空 | Trim 后非空 | `BadRequest("AGENT", "display_name is required")` |
| `provider` 非空 | Trim 后非空 | `BadRequest("AGENT", "provider is required")` |
| `model` 非空 | Trim 后非空 | `BadRequest("AGENT", "model is required")` |
| `agent_key` 格式 | 正则 `^[a-z0-9]+(-[a-z0-9]+)*$` | 前端校验 |
| `agent_key` 唯一性 | 未软删行中唯一 | 前端防抖查重 + 后端 DB unique constraint |
| `provider` + `model` 可用性 | 调用 `validateModel` 检查 | 前端「检查」按钮 |

---

## 四、Data 层

### 4.1 Ent Schema

文件：`internal/data/ent/schema/agent.go`

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

文件：`internal/data/ent/schema/agent_runtime_setting.go`

关键字段（共 79 个），与 `AgentRuntimeSettings` Proto 消息一一对应，ID 为主键即 `agent_id`。

文件：`internal/data/ent/schema/agent_prompt_file.go`

关键字段：`id`、`agent_id`、`file_name`、`body`、`sort_order`、`created_at`、`updated_at`。

### 4.2 Repo 实现

```go
// internal/data/agent_repo.go

type agentRepo struct {
    data *Data
}

func NewAgentRepo(d *Data) biz.AgentRepository {
    return &agentRepo{data: d}
}

func (r *agentRepo) CreateAgent(ctx context.Context, a biz.Agent) (biz.Agent, error) {
    if a.ID == "" || a.AgentKey == "" || a.DisplayName == "" || a.Provider == "" || a.Model == "" {
        return biz.Agent{}, fmt.Errorf("missing required fields")
    }
    now := nowRFC3339()
    if a.CreatedAt == "" {
        a.CreatedAt = now
    }
    a.UpdatedAt = now
    if a.Status == "" {
        a.Status = "active"
    }
    _, err := r.data.entClient.Agent.Create().
        SetID(a.ID).
        SetAgentKey(a.AgentKey).
        SetDisplayName(a.DisplayName).
        SetProvider(a.Provider).
        SetModel(a.Model).
        SetStatus(a.Status).
        SetIsDefault(a.IsDefault).
        SetIsFavorite(a.IsFavorite).
        SetIcon(a.Icon).
        SetAgentDescription(a.AgentDescription).
        SetCategoryPositionID(a.CategoryPositionID).
        SetSystemPromptMode(a.SystemPromptMode).
        SetContextWindow(a.ContextWindow).
        SetBudgetMonthlyCents(a.BudgetMonthlyCents).
        SetConfigJSON(a.ConfigJSON).
        SetCreatedAt(a.CreatedAt).
        SetUpdatedAt(a.UpdatedAt).
        SetDeletedAt(a.DeletedAt).
        Save(ctx)
    if err != nil {
        return biz.Agent{}, err
    }
    return r.GetAgentByID(ctx, a.ID)
}

func (r *agentRepo) UpsertAgentRuntimeSettings(ctx context.Context, v biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error) {
    if v.AgentID == "" {
        return biz.AgentRuntimeSettings{}, fmt.Errorf("agent id is required")
    }
    now := nowRFC3339()
    if v.CreatedAt == "" {
        v.CreatedAt = now
    }
    v.UpdatedAt = now
    b := r.data.entClient.AgentRuntimeSetting.Create().SetID(v.AgentID)
    applyBizRuntimeToCreate(b, v)
    if err := b.OnConflict(
        entsql.ConflictColumns(agentruntimesetting.FieldID),
        entsql.ResolveWithNewValues(),
    ).Exec(ctx); err != nil {
        return biz.AgentRuntimeSettings{}, err
    }
    row, err := r.data.entClient.AgentRuntimeSetting.Get(ctx, v.AgentID)
    if err != nil {
        return biz.AgentRuntimeSettings{}, err
    }
    return entRuntimeToBiz(row), nil
}

func (r *agentRepo) ReplaceAgentPromptFiles(ctx context.Context, agentID string, files []biz.AgentPromptFile) ([]biz.AgentPromptFile, error) {
    if agentID == "" {
        return nil, fmt.Errorf("agent id is required")
    }
    tx, err := r.data.entClient.Tx(ctx)
    if err != nil {
        return nil, err
    }
    defer func() { _ = tx.Rollback() }()
    if _, err = tx.AgentPromptFile.Delete().Where(agentpromptfile.AgentIDEQ(agentID)).Exec(ctx); err != nil {
        return nil, err
    }
    now := nowRFC3339()
    for i, file := range files {
        if strings.TrimSpace(file.Name) == "" {
            continue
        }
        id := file.ID
        if id == "" {
            id = fmt.Sprintf("%s_%s", agentID, sanitizePromptFileID(file.Name))
        }
        sortOrder := file.SortOrder
        if sortOrder == 0 {
            sortOrder = (i + 1) * 10
        }
        if _, err = tx.AgentPromptFile.Create().
            SetID(id).
            SetAgentID(agentID).
            SetFileName(strings.TrimSpace(file.Name)).
            SetBody(file.Body).
            SetSortOrder(sortOrder).
            SetCreatedAt(now).
            SetUpdatedAt(now).
            Save(ctx); err != nil {
            return nil, err
        }
    }
    if err = tx.Commit(); err != nil {
        return nil, err
    }
    return r.ListAgentPromptFiles(ctx, agentID)
}
```

### 4.3 转换函数

```go
func entAgentToBiz(a *ent.Agent) biz.Agent
func entRuntimeToBiz(e *ent.AgentRuntimeSetting) biz.AgentRuntimeSettings
func entPromptToBiz(e *ent.AgentPromptFile) biz.AgentPromptFile
func applyBizRuntimeToCreate(b *ent.AgentRuntimeSettingCreate, v biz.AgentRuntimeSettings)
```

---

## 五、Service 层

### 5.1 文件结构

```
internal/service/agent.go    ← AgentService 主结构 + 全部 RPC 方法
```

### 5.2 AgentService 结构体

```go
type AgentService struct {
    v1.UnimplementedAgentServiceServer
    uc *biz.AgentUsecase
}

func NewAgentService(uc *biz.AgentUsecase) *AgentService {
    return &AgentService{uc: uc}
}
```

### 5.3 CreateAgent RPC 实现

```go
func (s *AgentService) CreateAgent(ctx context.Context, req *agentv1.CreateAgentRequest) (*agentv1.Agent, error) {
    created, err := s.uc.Create(ctx, fromProtoCreate(req))
    if err != nil {
        return nil, err
    }
    return toProtoAgent(created), nil
}
```

### 5.4 类型转换函数

```go
func fromProtoCreate(req *v1.CreateAgentRequest) biz.Agent {
    if req == nil {
        return biz.Agent{}
    }
    a := biz.Agent{
        AgentKey:           req.GetAgentKey(),
        DisplayName:        req.GetDisplayName(),
        Provider:           req.GetProvider(),
        Model:              req.GetModel(),
        Icon:               req.GetIcon(),
        AgentDescription:   req.GetAgentDescription(),
        CategoryPositionID: req.GetCategoryPositionId(),
        SystemPromptMode:   req.GetSystemPromptMode(),
        ContextWindow:      int(req.GetContextWindow()),
        BudgetMonthlyCents: int(req.GetBudgetMonthlyCents()),
        ConfigJSON:         req.GetConfigJson(),
    }
    if s := fromProtoRuntime(req.GetSettings()); s != nil {
        a.Settings = s
    }
    for _, f := range req.GetFiles() {
        a.Files = append(a.Files, fromProtoFile(f))
    }
    return a
}

func toProtoAgent(b biz.Agent) *v1.Agent {
    out := &v1.Agent{
        Id:                 b.ID,
        AgentKey:           b.AgentKey,
        DisplayName:        b.DisplayName,
        Provider:           b.Provider,
        Model:              b.Model,
        Status:             b.Status,
        IsDefault:          b.IsDefault,
        IsFavorite:         b.IsFavorite,
        Icon:               b.Icon,
        AgentDescription:   b.AgentDescription,
        CategoryPositionId: b.CategoryPositionID,
        SystemPromptMode:   b.SystemPromptMode,
        ContextWindow:      int32(b.ContextWindow),
        BudgetMonthlyCents: int32(b.BudgetMonthlyCents),
        ConfigJson:         b.ConfigJSON,
        CreatedAt:          b.CreatedAt,
        UpdatedAt:          b.UpdatedAt,
        DeletedAt:          b.DeletedAt,
        Settings:           toProtoRuntime(b.Settings),
    }
    for i := range b.Files {
        out.Files = append(out.Files, toProtoFile(b.Files[i]))
    }
    return out
}

func fromProtoRuntime(pb *v1.AgentRuntimeSettings) *biz.AgentRuntimeSettings
func toProtoRuntime(b *biz.AgentRuntimeSettings) *v1.AgentRuntimeSettings
func fromProtoFile(pb *v1.AgentPromptFile) biz.AgentPromptFile
func toProtoFile(b biz.AgentPromptFile) *v1.AgentPromptFile
```

---

## 六、Wire 注入

已有注入链，无需新增：

```go
// internal/data/data.go
var ProviderSet = wire.NewSet(NewAgentRepo, ...)

// internal/biz/biz.go
var ProviderSet = wire.NewSet(NewAgentUsecase, ...)

// internal/service/service.go
var ProviderSet = wire.NewSet(NewAgentService, ...)
```

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/features/agents/
├── api.ts                    ← Agent API（含 createAgent）
├── types.ts                  ← Agent 类型定义
├── wireNormalize.ts          ← 数据规范化（Wire ↔ 内部类型双向转换）
├── useAgentsPage.ts          ← 列表页 composable（含创建表单逻辑）
└── components/
    └── AgentCreateDialog.vue ← 创建弹窗
```

### 7.2 TypeScript 类型定义

```typescript
// features/agents/types.ts

export type Agent = {
  id: string
  agent_key: string
  display_name: string
  provider: string
  model: string
  status: string
  is_default: boolean
  is_favorite: boolean
  icon: string
  agent_description: string
  category_position_id: string
  system_prompt_mode: string
  context_window: number
  budget_monthly_cents: number
  config_json: string
  created_at: string
  updated_at: string
  deleted_at: string
  settings?: AgentRuntimeSettings
  files?: AgentPromptFile[]
}

export type AgentRuntimeSettings = {
  agent_id?: string
  self_evolve: boolean
  subagents_enabled: boolean
  subagents_max_concurrency: number
  subagents_max_generation_depth: number
  subagents_max_children_per_agent: number
  subagents_archive_after_minutes: number
  subagents_max_retries: number
  subagents_model_override: string
  tools_enabled: boolean
  tools_profile: string
  tools_tool_call_prefix: string
  tools_allow_json: string
  tools_deny_json: string
  tools_concurrent_allow_json: string
  memory_enabled: boolean
  memory_max_chunk_length: number
  memory_max_results: number
  memory_min_score: number
  l0_recent_window_turns?: number
  l0_recent_window_tokens?: number
  l0_summary_threshold?: number
  l0_summary_keep_turns?: number
  l0_truncate_strategy?: string
  l0_inject_l1?: boolean
  l0_inject_l3?: boolean
  l0_inject_l4?: boolean
  l0_l3_max_chunks?: number
  l0_l4_max_paths?: number
  l0_snapshot_mode?: string
  l1_enabled?: boolean
  l1_budget_tokens?: number
  l1_field_max_tokens?: number
  l1_history_keep_revisions?: number
  l1_default_schema_id?: string
  l1_archive_on_idle_minutes?: number
  l2_episode_enabled?: boolean
  l2_episode_min_importance?: number
  l2_index_enabled?: boolean
  l2_index_embedding_model?: string
  l2_recall_enabled?: boolean
  l2_recall_max?: number
  l2_retention_days?: number
  l2_archive_after_days?: number
  l3_enabled?: boolean
  l3_recall_top_k?: number
  l3_recall_min_score?: number
  l3_recall_scopes_json?: string
  l3_embedding_model?: string
  l3_decay_interval_hours?: number
  l3_archive_threshold?: number
  l3_max_per_recall_chars?: number
  l4_enabled?: boolean
  l4_graph_inject_neighbors?: boolean
  l4_graph_max_neighbors?: number
  l4_graph_max_hops?: number
  l4_identity_inject?: boolean
  l4_strategy_inject?: boolean
  evo_enabled?: boolean
  evo_auto_apply?: boolean
  evo_min_episodes?: number
  evo_min_negative_feedback?: number
  evo_throttle_hours?: number
  evo_proposal_ttl_days?: number
  evo_persona_max_chars?: number
  evo_system_prompt_max_appends?: number
  heartbeat_enabled: boolean
  heartbeat_interval_minutes: number
  evolution_self_evolve: boolean
  evolution_skill_evolve: boolean
  evolution_metrics_enabled: boolean
  evolution_suggestions_enabled: boolean
  guardrail_max_change_per_period: number
  guardrail_min_data_points: number
  guardrail_rollback_on_decline_percent: number
  skill_runtime_json?: string
  intent_pass_enabled?: boolean
  created_at?: string
  updated_at?: string
}

export type AgentPromptFile = {
  id?: string
  agent_id?: string
  name: string
  body: string
  sort_order: number
  created_at?: string
  updated_at?: string
}

export type AgentListQuery = {
  keyword?: string
  status?: string
  provider?: string
  category_id?: string
  limit?: number
  offset?: number
}

export type AgentListResult = {
  items: Agent[]
  total: number
  limit: number
  offset: number
}
```

### 7.3 API 调用

```typescript
// features/agents/api.ts

import { createAgentService } from '../../services'
import type { CreateAgentRequest as KratosCreateAgentRequest } from '../../services/kratos/agent/v1/index'
import type { Agent, AgentListQuery, AgentListResult, AgentPromptFile, AgentRuntimeSettings } from './types'
import {
  normalizeAgentFromService,
  partialAgentToWire,
  promptFileToWire,
  runtimeSettingsToWire
} from './wireNormalize'

export async function createAgent(payload: {
  agent_key: string
  display_name: string
  provider: string
  model: string
  icon?: string
  agent_description?: string
  category_position_id?: string
  system_prompt_mode?: string
  context_window?: number
  budget_monthly_cents?: number
  config_json?: string
  settings?: AgentRuntimeSettings
  files?: AgentPromptFile[]
}): Promise<Agent> {
  const svc = createAgentService()
  const req: KratosCreateAgentRequest = {
    agentKey: payload.agent_key,
    displayName: payload.display_name,
    provider: payload.provider,
    model: payload.model,
    icon: payload.icon,
    agentDescription: payload.agent_description,
    categoryPositionId: payload.category_position_id,
    systemPromptMode: payload.system_prompt_mode,
    contextWindow: payload.context_window,
    budgetMonthlyCents: payload.budget_monthly_cents,
    configJson: payload.config_json,
    settings: payload.settings ? runtimeSettingsToWire(payload.settings) : undefined,
    files: payload.files?.map(promptFileToWire)
  }
  const data = await svc.CreateAgent(req)
  return normalizeAgentFromService(data)
}
```

### 7.4 数据规范化

```typescript
// features/agents/wireNormalize.ts

export function normalizeAgentFromService(raw: unknown): Agent {
  const w = asWireRecord(raw)
  return {
    id: pickStr(w, 'id', 'id'),
    agent_key: pickStr(w, 'agentKey', 'agent_key'),
    display_name: pickStr(w, 'displayName', 'display_name'),
    provider: pickStr(w, 'provider', 'provider'),
    model: pickStr(w, 'model', 'model'),
    status: pickStr(w, 'status', 'status', 'active'),
    is_default: pickBool(w, 'isDefault', 'is_default', false),
    is_favorite: pickBool(w, 'isFavorite', 'is_favorite', false),
    icon: pickStr(w, 'icon', 'icon'),
    agent_description: pickStr(w, 'agentDescription', 'agent_description'),
    category_position_id: pickStr(w, 'categoryPositionId', 'category_position_id'),
    system_prompt_mode: pickStr(w, 'systemPromptMode', 'system_prompt_mode', 'complete'),
    context_window: pickNum(w, 'contextWindow', 'context_window', 0),
    budget_monthly_cents: pickNum(w, 'budgetMonthlyCents', 'budget_monthly_cents', 0),
    config_json: pickStr(w, 'configJson', 'config_json'),
    created_at: pickStr(w, 'createdAt', 'created_at'),
    updated_at: pickStr(w, 'updatedAt', 'updated_at'),
    deleted_at: pickStr(w, 'deletedAt', 'deleted_at'),
    settings: normalizeRuntimeSettingsFromWire(w.settings),
    files: Array.isArray(w.files) ? w.files.map(normalizePromptFileFromWire) : undefined
  }
}

export function runtimeSettingsToWire(s: AgentRuntimeSettings): KratosRuntimeWire { ... }
export function promptFileToWire(f: AgentPromptFile): KratosFileWire { ... }
export function partialAgentToWire(payload: Partial<Agent>): KratosAgentWire { ... }
```

### 7.5 Composable

```typescript
// features/agents/useAgentsPage.ts

export type CreateAgentForm = {
  agent_key: string
  display_name: string
  provider: string
  model: string
  icon: string
  agent_description: string
  category_position_id: string
}

export function useAgentsPage() {
  const createOpen = ref(false)
  const creating = ref(false)
  const selfEvolve = ref(true)
  const modelCheckPassed = ref(false)

  const form = reactive<CreateAgentForm>({
    agent_key: '',
    display_name: '',
    provider: 'openrouter',
    model: 'gpt-4.1-mini',
    icon: 'smart_toy',
    agent_description: '',
    category_position_id: ''
  })

  const agentKeyError = computed(() => {
    if (!form.agent_key) return ''
    return /^[a-z0-9]+(-[a-z0-9]+)*$/.test(form.agent_key)
      ? ''
      : '仅支持小写字母、数字、连字符'
  })

  const canCreate = computed(() =>
    Boolean(form.display_name && form.agent_key && !agentKeyError.value && form.provider && form.model && modelCheckPassed.value)
  )

  async function onCreate() {
    if (!canCreate.value) return
    creating.value = true
    try {
      await appStore.addAgent({
        ...form,
        config_json: JSON.stringify({
          self_evolve: selfEvolve.value,
          description_template_key: selectedTemplateKey.value
        })
      })
      pageStore.resetListFiltersAfterCreate()
      await runLoadList()
      resetForm()
      createOpen.value = false
      $q.notify({ type: 'positive', message: '创建成功' })
    } finally {
      creating.value = false
    }
  }

  async function checkModel() {
    try {
      const result = await pageStore.validateCreateModel(form.provider, form.model)
      $q.notify({ type: result.ok ? 'positive' : 'negative', message: result.message })
    } catch (error) {
      $q.notify({ type: 'negative', message: '校验失败' })
    }
  }

  function applyTemplate(template: { key: string; text: string }) {
    selectedTemplateKey.value = template.key
    if (form.agent_description.trim()) {
      form.agent_description = `${form.agent_description}\n\n${template.text}`
    } else {
      form.agent_description = template.text
    }
  }

  function resetForm() {
    Object.assign(form, {
      agent_key: '',
      display_name: '',
      provider: 'openrouter',
      model: 'gpt-4.1-mini',
      icon: avatars.value[0]?.id ?? 'smart_toy',
      agent_description: '',
      category_position_id: ''
    })
    categoryIndustry.value = null
    categoryDepartment.value = null
    selectedTemplateKey.value = ''
    modelCheckPassed.value = false
    selfEvolve.value = true
  }

  return {
    form, createOpen, creating, selfEvolve, modelCheckPassed,
    agentKeyError, canCreate,
    openCreate, onCreate, checkModel, applyTemplate, resetForm
  }
}
```

### 7.6 组件设计

**AgentCreateDialog.vue**：

```vue
<template>
  <QDialog v-model="createOpen" persistent maximized>
    <QCard style="max-width: 720px">
      <QCardSection class="row items-center q-pb-none">
        <div class="text-h6">创建 Agent</div>
        <QSpace />
        <QBtn flat round dense icon="close" @click="createOpen = false" />
      </QCardSection>

      <QCardSection>
        <QForm @submit.prevent="onCreate">
          <div class="row q-col-gutter-md">
            <div class="col-12 col-md-6">
              <QInput
                v-model="form.display_name"
                label="显示名称 *"
                :rules="[val => !!val || '必填']"
                maxlength="255"
                outlined
                dense
              >
                <template #prepend>
                  <QAvatar size="32px">
                    <img v-if="form.icon" :src="`/avatar-assets/${form.icon}/thumbnail`" />
                    <QIcon v-else name="smart_toy" />
                  </QAvatar>
                </template>
              </QInput>
            </div>
            <div class="col-12 col-md-6">
              <QInput
                v-model="form.agent_key"
                label="Agent标识 *"
                hint="小写字母、数字、连字符"
                :error="!!agentKeyError"
                :error-message="agentKeyError"
                :rules="[val => !!val || '必填', val => /^[a-z0-9]+(-[a-z0-9]+)*$/.test(val) || '格式不正确']"
                maxlength="100"
                outlined
                dense
              />
            </div>
          </div>

          <div class="row q-col-gutter-md q-mt-sm">
            <div class="col-12 col-md-4">
              <QSelect v-model="categoryIndustry" :options="industryOptions" label="行业" outlined dense emit-value map-options clearable />
            </div>
            <div class="col-12 col-md-4">
              <QSelect v-model="categoryDepartment" :options="departmentOptions" label="部门" outlined dense emit-value map-options clearable />
            </div>
            <div class="col-12 col-md-4">
              <QSelect v-model="form.category_position_id" :options="positionOptions" label="职位" outlined dense emit-value map-options clearable />
            </div>
          </div>

          <div class="row q-col-gutter-md q-mt-sm">
            <div class="col-12 col-md-6">
              <QSelect v-model="form.provider" :options="providerOptions" label="Provider *" outlined dense emit-value map-options :rules="[val => !!val || '必选']" />
            </div>
            <div class="col-12 col-md-6">
              <QInput v-model="form.model" label="模型 *" outlined dense :rules="[val => !!val || '必填']">
                <template #append>
                  <QBtn flat dense label="检查" :disable="!form.provider || !form.model" :loading="checkingModel" @click="checkModel" />
                </template>
              </QInput>
            </div>
          </div>

          <div class="q-mt-sm">
            <div class="text-caption q-mb-xs">描述您的 Agent</div>
            <div class="row q-gutter-xs q-mb-xs">
              <QChip v-for="tpl in descriptionTemplates" :key="tpl.key" clickable @click="applyTemplate(tpl)">
                {{ tpl.label }}
              </QChip>
            </div>
            <QInput v-model="form.agent_description" type="textarea" filled rows="6" hint="AI 将根据此描述自动生成 Agent 的上下文文件。留空则使用模板。" />
          </div>

          <QCard flat bordered class="q-mt-sm q-pa-sm">
            <div class="row items-center justify-between">
              <div>
                <div class="text-subtitle2">自我进化</div>
                <div class="text-caption">允许 Agent 通过 SOUL.md 随时间进化其风格和语调</div>
              </div>
              <QToggle v-model="selfEvolve" />
            </div>
          </QCard>
        </QForm>
      </QCardSection>

      <QCardActions align="right">
        <QBtn flat label="取消" @click="createOpen = false" />
        <QBtn unelevated color="primary" label="创建" :loading="creating" :disable="!canCreate" @click="onCreate" />
      </QCardActions>
    </QCard>
  </QDialog>
</template>
```

### 7.7 控件清单

| 控件 | 绑定字段 | 校验 | 说明 |
|------|----------|------|------|
| `QInput` 显示名称 | `displayName` | 非空，max 255 | 左侧 prepend 为头像 |
| `QInput` Agent标识 | `agentKey` | 正则 `^[a-z0-9]+(-[a-z0-9]+)*$`，防抖查重 | 小写字母数字连字符 |
| `QSelect` 行业 | `categoryIndustry` | 可选 | 级联第一层 |
| `QSelect` 部门 | `categoryDepartment` | 可选 | 级联第二层，依赖行业 |
| `QSelect` 职位 | `categoryPositionId` | 可选 | 级联第三层叶子，提交为 `category_position_id` |
| `QSelect` Provider | `provider` | 必选 | 变更时清空模型 |
| `QInput` 模型 + 检查按钮 | `model` | 必填 + 检查通过 | 尾部「检查」按钮 |
| `QChip[]` 模板 | — | — | 点击填充描述 |
| `QInput` 描述 | `agentDescription` | 可选 | textarea，6行 |
| `QToggle` 自我进化 | `selfEvolve` | — | 默认 true |

### 7.8 交互流程

1. 用户点击「创建 Agent」→ 打开 `QDialog`
2. 填写必填字段 → `agentKey` 失焦时前端正则校验
3. 选择 Provider → 自动加载可用模型列表
4. 输入模型 → 点击「检查」→ 调用 `validateModel` API
5. 检查通过 → `modelCheckPassed = true` → 「创建」按钮可用
6. 点击「创建」→ 调用 `createAgent` API
7. 成功 → 关闭弹窗，刷新列表，toast「创建成功」
8. 失败 → 显示 inline error

### 7.9 状态字段

| 状态字段 | 用途 |
|-----------|------|
| `modelCheckPassed` | 模型检查是否通过，控制「创建」按钮 |
| `creating` | 提交中禁用双按钮 |
| `agentKeyError` | 标识格式校验结果 |
| `categoryIndustry` | 行业选择（级联中间态） |
| `categoryDepartment` | 部门选择（级联中间态） |
| `selectedTemplateKey` | 当前选中模板标识 |

---

## 八、trpc-agent-go 对齐需求（M2 Agent 构建）

> 本节补充 `plan.md` M2 模块的对齐需求，确保 Agent 构建完全复刻 trpc-agent-go `llmagent` 能力。

### 8.1 占位符变量

**trpc 框架**：`llmagent.New` 支持 `WithInstruction` 中的 `{key}` 占位符，运行时由 `agent.Invocation` 的 `Variables` 替换。

**需求**：
- Agent 的 `system_prompt` 支持 `{variable_name}` 占位符
- 运行时从 `Invocation.Variables` 注入实际值
- 内置变量：`{agent_name}`、`{session_id}`、`{user_id}`、`{current_date}`
- 自定义变量：通过 `AgentRuntimeSetting.variables_json` 配置

**涉及文件**：`internal/agent/trpc_build.go`

**验收标准**：Agent Instruction 中 `{key}` 被正确替换为运行时值

### 8.2 ModelInstructions

**trpc 框架**：`llmagent.WithModelInstructions` 为不同模型注入不同的指令片段。

**需求**：
- `AgentRuntimeSetting` 增加 `model_instructions_json` 字段
- 格式：`{"gpt-4o": "你是一个精确的助手", "claude-3": "你是一个有创意的助手"}`
- `BuildTRPCLLMAgent` 中通过 `WithModelInstructions` 注入

**涉及文件**：`internal/agent/trpc_build.go`、`internal/biz/agent_types.go`

**验收标准**：不同模型使用不同的指令片段

### 8.3 ContextCompaction

**trpc 框架**：`llmagent.WithContextCompaction` 启用上下文自动压缩，当 token 接近上限时自动摘要。

**需求**：
- `AgentRuntimeSetting` 增加 `context_compaction` 布尔字段
- `BuildTRPCLLMAgent` 中通过 `WithContextCompaction` 启用
- 压缩策略：保留最近 N 轮 + 摘要历史

**涉及文件**：`internal/agent/trpc_build.go`、`internal/biz/agent_types.go`

**验收标准**：长对话自动压缩，不丢失关键信息

### 8.4 SessionSummary

**trpc 框架**：`llmagent.WithSessionSummary` 启用会话摘要，新 session 可加载旧 session 摘要。

**需求**：
- `AgentRuntimeSetting` 增加 `session_summary` 布尔字段
- `BuildTRPCLLMAgent` 中通过 `WithSessionSummary` 启用
- Session 结束时自动生成摘要
- 新 Session 可通过摘要继承上下文

**涉及文件**：`internal/agent/trpc_build.go`、`internal/biz/agent_types.go`

**验收标准**：新 Session 可通过摘要继承旧 Session 上下文

### 8.5 SkillLoadMode

**trpc 框架**：`llmagent.WithSkillLoadMode` 控制技能加载策略（auto/manual/none）。

**需求**：
- `AgentRuntimeSetting` 增加 `skill_load_mode` 字段
- 可选值：`auto`（自动匹配）、`manual`（手动指定）、`none`（不加载）
- `BuildTRPCLLMAgent` 中通过 `WithSkillLoadMode` 注入

**涉及文件**：`internal/agent/trpc_build.go`、`internal/biz/agent_types.go`

**验收标准**：Agent 按配置策略加载技能

### 8.6 StructuredOutput

**trpc 框架**：`llmagent.WithStructuredOutput` 强制 LLM 输出符合 JSON Schema。

**需求**：
- `AgentRuntimeSetting` 增加 `output_schema_json` 字段
- `BuildTRPCLLMAgent` 中通过 `WithStructuredOutput` 注入
- LLM 输出自动校验和解析

**涉及文件**：`internal/agent/trpc_build.go`、`internal/biz/agent_types.go`

**验收标准**：Agent 输出符合预定义的 JSON Schema

### 8.7 ModelSelector

**trpc 框架**：`llmagent.WithModelSelector` 动态选择模型。

**需求**：
- `AgentRuntimeSetting` 增加 `model_selector` 字段
- 可选值：`default`（使用配置模型）、`auto`（根据任务复杂度选择）
- `BuildTRPCLLMAgent` 中通过 `WithModelSelector` 注入

**涉及文件**：`internal/agent/trpc_build.go`、`internal/biz/agent_types.go`

**验收标准**：Agent 根据任务复杂度动态选择模型

---

## 九、表单字段 ↔ 数据库 `agents` 表

| 表单字段 / UI | 数据库列 | 类型 | 备注 |
|---------------|----------|------|------|
| 显示名称 | `display_name` | VARCHAR(1024) | 必填 |
| Agent 标识 | `agent_key` | VARCHAR(512) | 未软删唯一 |
| 业务分类（行业→部门→职位） | `category_position_id` | TEXT | 可选；仅绑定职位叶子 |
| Provider | `provider` | TEXT | 存 slug |
| 模型 | `model` | TEXT | 检查通过后写入 |
| 描述（多行） | `agent_description` | TEXT | 可空；空则服务端按模板默认 |
| 自我进化 | `self_evolve` | BOOLEAN | 存 `agent_runtime_settings`，默认 true |
| 头像资源 id | `icon` | VARCHAR(255) | 对应 `avatar_assets.id` |
| — | `id` | UUID | 服务端生成 |
| — | `status` | VARCHAR(20) | 默认 `active` |
| — | `created_at` / `updated_at` | TEXT | 服务端维护 |
