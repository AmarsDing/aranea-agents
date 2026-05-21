# Memory L0-L4 模块 — 实现设计文档

> 对应需求：`12 memory-L0-sensory.md` / `13 memory-L1-working.md` / `14 memory-L2-episodic.md` / `15 memory-L3-semantic.md` / `16 memory-L4-persistent.md` / `12-16 memory.md` / `38 memory.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
>
> **文档定位**：Memory 模块的**完整实现设计**，覆盖后端（Proto/Biz/Data/Service/Wire/MemoryWorker/运行时注入）和前端（组件/类型/API/路由）。
>
> 相关文档：
> - UX 需求（产品视角）→ [`12-16 memory.md`](./12-16%20memory.md)
> - Memory 框架对齐 → [`38 memory.md`](./38%20memory.md)
> - **实现差距与代码锚点** → [`12-16 memory-development.md`](./12-16%20memory-development.md) §1–§2（优先于本文历史 ADK 表述）

---

## 一、模块概述

五层记忆架构：L0 感官（最近对话）、L1 工作（结构化状态）、L2 情景（重要事件）、L3 语义（向量知识）、L4 持久（身份图谱）。L0-L2/L4 通过 `sessionmemory.Store` 存储在 SQLite，L3 通过 pgvector 存储。

核心能力包括：
1. **五层记忆存储与检索**：L0-L4 各层的 CRUD 和向量检索
2. **自动提取**：🟡 MVP — `TurnMemoryWorker` 入队 + `AutoMemoryWorker` 启发式（regex → L4）；完整 LLM 提取管道待 Phase 2
3. **检索增强**：L3 向量检索与 Agent 运行时注入
4. **冲突检测**：新 fact 与现有 fact 矛盾时标记冲突
5. **级联更新**：实体属性变更时，沿图谱关系传播更新
6. **记忆管理界面**：Memory Center 前端完整实现

---

## 二、Proto 层

### 2.1 现有 Proto

文件：`api/kratos/memory/v1/memory.proto`

```protobuf
service MemoryService {
  rpc ListMemories(ListMemoriesRequest) returns (ListMemoriesResponse) {
    option (google.api.http) = { get: "/v1/memories" };
  }
  rpc GetMemory(GetMemoryRequest) returns (Memory) {
    option (google.api.http) = { get: "/v1/memories/{id}" };
  }
  rpc CreateMemory(CreateMemoryRequest) returns (Memory) {
    option (google.api.http) = { post: "/v1/memories" body: "*" };
  }
  rpc DeleteMemory(DeleteMemoryRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/memories/{id}" };
  }
  rpc RecallMemories(RecallMemoriesRequest) returns (RecallMemoriesResponse) {
    option (google.api.http) = { post: "/v1/memories/recall" body: "*" };
  }
}
```

### 2.2 已新增

以下 RPC 已在 `api/kratos/memory/v1/memory.proto` 中定义：

| RPC | 路径 | 用途 | 状态 |
|-----|------|------|------|
| `ListL0Snapshots` | — | L0 快照列表 | ✅ 已定义 |
| `ListL1Tasks` | — | L1 任务列表 | ✅ 已定义 |
| `ListL1Fields` | — | L1 字段列表 | ✅ 已定义 |
| `ListMemoryFacts` | — | L3 fact 列表 | ✅ 已定义 |
| `ListMemoryEntities` | — | L4 实体列表 | ✅ 已定义 |
| `GetMemoryNeighborhood` | — | L4 邻居查询 | ✅ 已定义 |
| `GetAgentIdentity` | — | Agent 身份 | ✅ 已定义 |
| `GetAgentStrategy` | — | Agent 策略画像 | ✅ 已定义 |
| `ListEvolutionProposals` | — | 进化提议列表 | ✅ 已定义 |
| `ListEvolutionEvents` | — | 进化事件列表 | ✅ 已定义 |
| `GetEvolutionMetrics` | — | 进化指标 | ✅ 已定义 |
| `UpsertMemoryFact` | — | 写入 fact | ✅ 已定义 |
| `AppendEvolutionEvent` | — | 追加进化事件 | ✅ 已定义 |

### 2.3 仍待新增

| RPC | 路径 | 用途 |
|-----|------|------|
| `GetMemoryOverview` | `GET /v1/memories/overview` | 五层记忆总览 |
| `GetMemorySnapshot` | `GET /v1/memories/{session_id}/snapshot` | 会话记忆快照 |
| `ListCascadeProposals` | `GET /v1/memories/cascade/proposals` | 级联更新提议列表 |
| `ApproveCascadeProposal` | `POST /v1/memories/cascade/proposals/{id}/approve` | 批准级联更新 |
| `RejectCascadeProposal` | `POST /v1/memories/cascade/proposals/{id}/reject` | 拒绝级联更新 |
| `GetMemoryWorkerStatus` | `GET /v1/memories/worker/status` | MemoryWorker 运行状态 |

### 2.4 补充 Proto 定义（来自 31/38 需求）

```protobuf
service MemoryService {
  rpc GetMemoryLayers(GetMemoryLayersRequest) returns (GetMemoryLayersResponse) {
    option (google.api.http) = { get: "/v1/memories/layers" };
  }
  rpc SearchMemories(SearchMemoriesRequest) returns (SearchMemoriesResponse) {
    option (google.api.http) = { post: "/v1/memories/search" body: "*" };
  }
  rpc ListMemoryFacts(ListMemoryFactsRequest) returns (ListMemoryFactsResponse) {
    option (google.api.http) = { get: "/v1/memories/facts" };
  }
  rpc UpdateMemoryFact(UpdateMemoryFactRequest) returns (MemoryFact) {
    option (google.api.http) = { patch: "/v1/memories/facts/{id}" body: "*" };
  }
  rpc DeleteMemoryFact(DeleteMemoryFactRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/memories/facts/{id}" };
  }
  rpc ConfirmMemoryFact(ConfirmMemoryFactRequest) returns (MemoryFact) {
    option (google.api.http) = { post: "/v1/memories/facts/{id}/confirm" };
  }
  rpc RejectMemoryFact(RejectMemoryFactRequest) returns (MemoryFact) {
    option (google.api.http) = { post: "/v1/memories/facts/{id}/reject" };
  }
  rpc ListMemoryConflicts(ListMemoryConflictsRequest) returns (ListMemoryConflictsResponse) {
    option (google.api.http) = { get: "/v1/memories/conflicts" };
  }
  rpc ResolveMemoryConflict(ResolveMemoryConflictRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { post: "/v1/memories/conflicts/{id}/resolve" body: "*" };
  }
  rpc ListMemoryEpisodes(ListMemoryEpisodesRequest) returns (ListMemoryEpisodesResponse) {
    option (google.api.http) = { get: "/v1/memories/episodes" };
  }
  rpc ConsolidateEpisode(ConsolidateEpisodeRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { post: "/v1/memories/episodes/{id}/consolidate" };
  }
  rpc GetMemoryOverview(GetMemoryOverviewRequest) returns (MemoryOverview) {
    option (google.api.http) = { get: "/v1/memories/overview" };
  }
  rpc GetL0Snapshot(GetL0SnapshotRequest) returns (L0Snapshot) {
    option (google.api.http) = { get: "/v1/memories/sessions/{session_id}/l0-snapshot" };
  }
  rpc ListEvolutionProposals(ListEvolutionProposalsRequest) returns (ListEvolutionProposalsResponse) {
    option (google.api.http) = { get: "/v1/memories/evolution/proposals" };
  }
  rpc ApproveEvolutionProposal(ApproveEvolutionProposalRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { post: "/v1/memories/evolution/proposals/{id}/approve" };
  }
  rpc RejectEvolutionProposal(RejectEvolutionProposalRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { post: "/v1/memories/evolution/proposals/{id}/reject" };
  }
}

message GetMemoryLayersRequest {
  string agent_id = 1;
  string session_id = 2;
}

message GetMemoryLayersResponse {
  L0ContextLayer l0 = 1;
  L1WorkingLayer l1 = 2;
  L2EpisodicLayer l2 = 3;
  L3SemanticLayer l3 = 4;
  L4PersistentLayer l4 = 5;
}

message L0ContextLayer {
  int32 context_window_tokens = 1;
  int32 used_tokens = 2;
  float used_ratio = 3;
  string truncate_strategy = 4;
  repeated string warning_codes = 5;
  repeated SegmentInfo segments = 6;
}

message SegmentInfo {
  string name = 1;
  int32 token_estimate = 2;
  string source = 3;
  bool truncated = 4;
}

message L1WorkingLayer {
  repeated WorkingTask tasks = 1;
  int32 total_budget_tokens = 2;
  int32 used_tokens = 3;
}

message WorkingTask {
  string id = 1;
  string title = 2;
  string status = 3;
  repeated WorkingField fields = 4;
  int32 token_estimate = 5;
  string updated_at = 6;
}

message WorkingField {
  string key = 1;
  string value = 2;
  string source = 3;
  bool pinned = 4;
  int32 revision = 5;
  int32 token_estimate = 6;
}

message L2EpisodicLayer {
  repeated EpisodeInfo episodes = 1;
  int32 pending_consolidation_count = 2;
}

message EpisodeInfo {
  string id = 1;
  string title = 2;
  string kind = 3;
  string outcome = 4;
  float importance = 5;
  float confidence = 6;
  string consolidation_status = 7;
  string created_at = 8;
}

message L3SemanticLayer {
  repeated MemoryFact facts = 1;
  int32 total_count = 2;
  int32 conflict_count = 3;
  float avg_confidence = 4;
}

message L4PersistentLayer {
  repeated EntityInfo entities = 1;
  repeated RelationInfo relations = 2;
  IdentityInfo identity = 3;
  StrategyInfo strategy = 4;
  repeated EvolutionProposal proposals = 5;
}

message EntityInfo {
  string id = 1;
  string name = 2;
  string type = 3;
  float importance = 4;
  int32 relation_count = 5;
}

message RelationInfo {
  string id = 1;
  string source_entity_id = 2;
  string target_entity_id = 3;
  string relation_type = 4;
  float weight = 5;
}

message IdentityInfo {
  string persona = 1;
  repeated string values = 2;
  string tone = 3;
  repeated string domains = 4;
}

message StrategyInfo {
  float exploration = 1;
  float conciseness = 2;
  float caution = 3;
  float delegation = 4;
}

message EvolutionProposal {
  string id = 1;
  string target_field = 2;
  string current_value = 3;
  string proposed_value = 4;
  string rationale = 5;
  string risk_level = 6;
  string status = 7;
  string expires_at = 8;
  string created_at = 9;
}

message SearchMemoriesRequest {
  string agent_id = 1;
  string query = 2;
  int32 top_k = 3;
  float min_score = 4;
  repeated string scopes = 5;
  string layer = 6;
}

message SearchMemoriesResponse {
  repeated MemorySearchResult results = 1;
}

message MemorySearchResult {
  string id = 1;
  string content = 2;
  float score = 3;
  string layer = 4;
  string scope = 5;
  string source = 6;
  string created_at = 7;
}

message MemoryFact {
  string id = 1;
  string statement = 2;
  string kind = 3;
  string scope = 4;
  float confidence = 5;
  float importance = 6;
  int32 hit_count = 7;
  repeated string tags = 8;
  string source = 9;
  string status = 10;
  string created_at = 11;
  string updated_at = 12;
}

message ListMemoryFactsRequest {
  string agent_id = 1;
  string scope = 2;
  string kind = 3;
  string status = 4;
  int32 page_size = 5;
  string page_token = 6;
}

message ListMemoryFactsResponse {
  repeated MemoryFact facts = 1;
  string next_page_token = 2;
  int32 total_count = 3;
}

message UpdateMemoryFactRequest {
  string id = 1;
  string statement = 2;
  repeated string tags = 3;
  string scope = 4;
}

message DeleteMemoryFactRequest {
  string id = 1;
}

message ConfirmMemoryFactRequest {
  string id = 1;
}

message RejectMemoryFactRequest {
  string id = 1;
  string reason = 2;
}

message ListMemoryConflictsRequest {
  string agent_id = 1;
  string status = 2;
  int32 page_size = 3;
}

message ListMemoryConflictsResponse {
  repeated MemoryConflict conflicts = 1;
}

message MemoryConflict {
  string id = 1;
  repeated string fact_ids = 2;
  string description = 3;
  string status = 4;
  string created_at = 5;
}

message ResolveMemoryConflictRequest {
  string id = 1;
  string action = 2;
  string winner_fact_id = 3;
  string merged_statement = 4;
}

message ListMemoryEpisodesRequest {
  string session_id = 1;
  string agent_id = 2;
  string consolidation_status = 3;
  int32 page_size = 4;
}

message ListMemoryEpisodesResponse {
  repeated EpisodeInfo episodes = 1;
  int32 total_count = 2;
}

message ConsolidateEpisodeRequest {
  string id = 1;
}

message GetMemoryOverviewRequest {
  string agent_id = 1;
  string scope = 2;
}

message MemoryOverview {
  float context_used_ratio = 1;
  int32 active_working_tasks = 2;
  int32 pending_episodes = 3;
  int32 active_facts = 4;
  int32 open_conflicts = 5;
  int32 pending_proposals = 6;
  float recall_hit_rate = 7;
  repeated MemorySearchResult recent_injected = 8;
}

message GetL0SnapshotRequest {
  string session_id = 1;
  int32 limit = 2;
}

message L0Snapshot {
  string id = 1;
  string session_id = 2;
  int32 context_window_tokens = 3;
  int32 prompt_token_estimate = 4;
  float used_ratio = 5;
  string segments_json = 6;
  repeated string warning_codes = 7;
  string created_at = 8;
}

message ListEvolutionProposalsRequest {
  string agent_id = 1;
  string status = 2;
  int32 page_size = 3;
}

message ListEvolutionProposalsResponse {
  repeated EvolutionProposal proposals = 1;
}

message ApproveEvolutionProposalRequest {
  string id = 1;
}

message RejectEvolutionProposalRequest {
  string id = 1;
  string reason = 2;
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type Memory struct {
    ID          string
    SessionID   string
    AgentID     string
    Layer       string  // "L0"/"L1"/"L2"/"L3"/"L4"
    Type        string  // "sensory"/"working"/"episodic"/"semantic"/"persistent"
    Key         string
    Content     string
    Score       float64
    Metadata    map[string]interface{}
    CreatedAt   string
    UpdatedAt   string
}

type MemoryOverview struct {
    L0Enabled      bool
    L1Enabled      bool
    L2Enabled      bool
    L3Enabled      bool
    L4Enabled      bool
    TotalMemories  int64
    RecentActivity []MemoryActivity
}
```

### 3.2 Repo 接口

```go
type MemoryRepository interface {
    List(ctx, query) (MemoryListResult, error)
    GetByID(ctx, id) (Memory, error)
    Create(ctx, m Memory) (Memory, error)
    Delete(ctx, id) error
    Recall(ctx, agentID string, query string, topK int, minScore float64) ([]Memory, error)
    GetOverview(ctx, agentID string) (MemoryOverview, error)
}
```

### 3.3 Usecase

```go
type MemoryUsecase struct {
    repo MemoryRepository
}

func (uc *MemoryUsecase) List(ctx, query) (MemoryListResult, error)
func (uc *MemoryUsecase) Recall(ctx, agentID, query string, topK int, minScore float64) ([]Memory, error)
func (uc *MemoryUsecase) GetOverview(ctx, agentID string) (MemoryOverview, error)
```

### 3.4 记忆自动提取

```go
// internal/memory/extractor.go
type MemoryExtractor struct {
    llm      model.LLM
    memStore *sessionmemory.Store
    memRepo  biz.MemoryRepo
    embedder biz.EmbeddingService
}

func NewMemoryExtractor(llm model.LLM, store *sessionmemory.Store, repo biz.MemoryRepo, embedder biz.EmbeddingService) *MemoryExtractor

func (e *MemoryExtractor) ExtractAfterTurn(ctx context.Context, sessionID, agentID string, messages []Message) error {
    prompt := buildExtractionPrompt(messages)
    resp, err := e.llm.Generate(ctx, prompt)
    if err != nil {
        return err
    }
    extractions := parseExtractions(resp)
    for _, ext := range extractions {
        switch ext.Layer {
        case "L2":
            e.memStore.UpsertEventEntity(ctx, sessionmemory.EventEntityParams{
                ScopeType:  "episodic",
                ScopeID:    sessionID,
                UserID:     agentID,
                EntityType: "episode",
                Name:       ext.Title,
                Description: ext.Content,
                Importance: ext.Importance,
            })
        case "L3":
            vec, _ := e.embedder.Embed(ctx, ext.Content)
            e.memRepo.Insert(ctx, &biz.AgentMemory{
                AgentID:   agentID,
                Content:   ext.Content,
                Embedding: vec,
            })
        }
    }
    return nil
}

type ExtractionResult struct {
    Layer      string
    Title      string
    Content    string
    Importance float64
    Kind       string
}

func buildExtractionPrompt(messages []Message) string
func parseExtractions(resp string) []ExtractionResult
```

### 3.5 记忆检索增强

```go
// internal/memory/retriever.go
type MemoryRetriever struct {
    memSvc   trpcmemory.Service
    memRepo  biz.MemoryRepo
    embedder biz.EmbeddingService
}

func NewMemoryRetriever(svc trpcmemory.Service, repo biz.MemoryRepo, embedder biz.EmbeddingService) *MemoryRetriever

func (r *MemoryRetriever) Retrieve(ctx context.Context, agentID, query string, topK int, minScore float64) ([]*biz.AgentMemory, error) {
    vec, err := r.embedder.Embed(ctx, query)
    if err != nil {
        return nil, err
    }
    results, err := r.memRepo.FindSimilar(ctx, agentID, vec, topK)
    if err != nil {
        return nil, err
    }
    var filtered []*biz.AgentMemory
    for _, m := range results {
        if m.Score >= minScore {
            filtered = append(filtered, m)
        }
    }
    return filtered, nil
}

func (r *MemoryRetriever) RetrieveAndFormat(ctx context.Context, agentID, query string, topK int, minScore float64) (string, error) {
    results, err := r.Retrieve(ctx, agentID, query, topK, minScore)
    if err != nil {
        return "", err
    }
    if len(results) == 0 {
        return "", nil
    }
    var sb strings.Builder
    sb.WriteString("[Retrieved Memories]\n")
    for i, m := range results {
        sb.WriteString(fmt.Sprintf("%d. %s (score: %.2f)\n", i+1, m.Content, m.Score))
    }
    return sb.String(), nil
}
```

### 3.6 记忆管理 Usecase

```go
// internal/biz/memory_management.go
type MemoryManagementUsecase struct {
    memRepo    MemoryRepo
    embedder   EmbeddingService
    store      *sessionmemory.Store
    extractor  *MemoryExtractor
    retriever  *MemoryRetriever
    agents     AgentRepo
}

func NewMemoryManagementUsecase(
    memRepo MemoryRepo,
    embedder EmbeddingService,
    store *sessionmemory.Store,
    extractor *MemoryExtractor,
    retriever *MemoryRetriever,
    agents AgentRepo,
) *MemoryManagementUsecase

func (uc *MemoryManagementUsecase) GetLayers(ctx, agentID, sessionID string) (*MemoryLayers, error)
func (uc *MemoryManagementUsecase) Search(ctx, agentID, query string, topK int, minScore float64) ([]*MemorySearchResult, error)
func (uc *MemoryManagementUsecase) ListFacts(ctx, agentID, scope, kind, status string, page int, pageSize int) ([]*MemoryFact, int, error)
func (uc *MemoryManagementUsecase) UpdateFact(ctx, factID, statement string, tags []string, scope string) (*MemoryFact, error)
func (uc *MemoryManagementUsecase) DeleteFact(ctx, factID string) error
func (uc *MemoryManagementUsecase) ConfirmFact(ctx, factID string) (*MemoryFact, error)
func (uc *MemoryManagementUsecase) RejectFact(ctx, factID, reason string) (*MemoryFact, error)
func (uc *MemoryManagementUsecase) ListConflicts(ctx, agentID, status string) ([]*MemoryConflict, error)
func (uc *MemoryManagementUsecase) ResolveConflict(ctx, conflictID, action, winnerID, mergedStatement string) error
func (uc *MemoryManagementUsecase) ListEpisodes(ctx, sessionID, agentID, consolidationStatus string, page int, pageSize int) ([]*Episode, int, error)
func (uc *MemoryManagementUsecase) ConsolidateEpisode(ctx, episodeID string) error
func (uc *MemoryManagementUsecase) GetOverview(ctx, agentID, scope string) (*MemoryOverview, error)
func (uc *MemoryManagementUsecase) GetL0Snapshot(ctx, sessionID string, limit int) (*L0Snapshot, error)
func (uc *MemoryManagementUsecase) ListEvolutionProposals(ctx, agentID, status string) ([]*EvolutionProposal, error)
func (uc *MemoryManagementUsecase) ApproveProposal(ctx, proposalID string) error
func (uc *MemoryManagementUsecase) RejectProposal(ctx, proposalID, reason string) error
```

### 3.7 补充领域模型

```go
type MemoryFact struct {
    ID          string
    AgentID     string
    Statement   string
    Kind        string  // "preference"/"fact"/"rule"/"experience"
    Scope       string  // "user"/"agent"/"team"/"workspace"/"global"
    Confidence  float64
    Importance  float64
    HitCount    int
    Tags        []string
    Source      string
    Status      string  // "active"/"archived"/"disputed"
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type MemoryConflict struct {
    ID          string
    AgentID     string
    FactIDs     []string
    Description string
    Status      string  // "open"/"resolved"
    Resolution  string
    CreatedAt   time.Time
}

type Episode struct {
    ID                 string
    SessionID          string
    AgentID            string
    Title              string
    Kind               string  // "task"/"decision"/"error_recovery"/"feedback"
    Outcome            string
    Importance         float64
    Confidence         float64
    ConsolidationStatus string  // "pending"/"consolidated"/"skipped"
    CreatedAt          time.Time
}

type EvolutionProposal struct {
    ID            string
    AgentID       string
    TargetField   string
    CurrentValue  string
    ProposedValue string
    Rationale     string
    RiskLevel     string  // "low"/"medium"/"high"
    Status        string  // "pending"/"approved"/"rejected"/"expired"
    ExpiresAt     time.Time
    CreatedAt     time.Time
}

type MemorySearchResult struct {
    ID        string
    Content   string
    Score     float64
    Layer     string
    Scope     string
    Source    string
    CreatedAt time.Time
}
```

---

## 四、Data 层

### 4.1 L0-L2/L4：sessionmemory.Store

文件：`internal/data/sessionmemory/`

```go
type Store struct {
    db *ent.Client
}

func (s *Store) GetRecentMessages(ctx, sessionID string, limit int) ([]Memory, error)
func (s *Store) GetWorkingState(ctx, sessionID string) (*WorkingState, error)
func (s *Store) GetEpisodes(ctx, agentID string, minImportance float64) ([]Memory, error)
func (s *Store) GetIdentity(ctx, agentID string) (*IdentityState, error)
```

### 4.2 L3：pgvector

文件：`internal/data/pgvector/`

```go
type VectorStore struct {
    db *sql.DB
}

func (v *VectorStore) Search(ctx, agentID string, embedding []float64, topK int, minScore float64) ([]Memory, error)
func (v *VectorStore) Insert(ctx, m Memory, embedding []float64) error
```

### 4.3 Ent Schema

- `user_embedding_setting.go` — 嵌入模型配置

### 4.4 记忆提取持久化

```go
// internal/data/memory_extraction.go
type memoryExtractionRepo struct {
    store *sessionmemory.Store
}

func NewMemoryExtractionRepo(store *sessionmemory.Store) *memoryExtractionRepo

func (r *memoryExtractionRepo) SaveExtraction(ctx context.Context, params sessionmemory.EventEntityParams) error {
    return r.store.UpsertEventEntity(ctx, params)
}

func (r *memoryExtractionRepo) ListExtractions(ctx context.Context, scopeType, scopeID, entityType string, limit int) ([][]byte, error) {
    rows, _, err := r.store.ListEntityRows(ctx, scopeType, scopeID, "", "", entityType, "", limit, 0)
    return rows, err
}
```

### 4.5 L0 快照查询

```go
// internal/data/sessionmemory/l0_snapshot.go
func (st *Store) GetLatestL0Snapshot(ctx context.Context, sessionID string) (*L0SnapshotRow, error) {
    var row L0SnapshotRow
    err := queryOne(ctx, st.client, sqlL0Select+` WHERE session_id = ? ORDER BY created_at DESC LIMIT 1`,
        []any{sessionID},
        &row.ID, &row.SessionID, &row.RunID, &row.TurnID, &row.SpanID,
        &row.AgentID, &row.TeamID, &row.Provider, &row.Model,
        &row.ContextWindowTokens, &row.BudgetTokens, &row.RecentWindowTurns,
        &row.RecentWindowTokens, &row.SummaryTokenEstimate,
        &row.L1FieldCount, &row.L1TokenEstimate,
        &row.L3ChunkCount, &row.L3TokenEstimate,
        &row.L4PathCount, &row.L4TokenEstimate,
        &row.PromptTokenEstimate, &row.PromptTokenActual,
        &row.UsedRatio, &row.TruncateStrategy,
        &row.TruncatedMessageCount, &row.SummarizedTurnFrom, &row.SummarizedTurnTo,
        &row.SegmentsJSON, &row.WarningCodesJSON, &row.MetadataJSON, &row.CreatedAt,
    )
    return &row, err
}
```

### 4.6 冲突检测

```go
// internal/data/memory_conflict.go
type memoryConflictRepo struct {
    data *Data
}

func NewMemoryConflictRepo(d *Data) *memoryConflictRepo

func (r *memoryConflictRepo) FindConflicts(ctx context.Context, agentID, status string) ([]*biz.MemoryConflict, error) {
    rows, err := r.data.Ent().QueryContext(ctx,
        `SELECT id, agent_id, fact_ids, description, status, resolution, created_at
         FROM memory_conflicts WHERE agent_id = ? AND status = ? ORDER BY created_at DESC`,
        agentID, status)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var out []*biz.MemoryConflict
    for rows.Next() {
        var c biz.MemoryConflict
        var factIDsJSON string
        if err := rows.Scan(&c.ID, &c.AgentID, &factIDsJSON, &c.Description, &c.Status, &c.Resolution, &c.CreatedAt); err != nil {
            continue
        }
        json.Unmarshal([]byte(factIDsJSON), &c.FactIDs)
        out = append(out, &c)
    }
    return out, nil
}
```

### 4.7 Ent Schema 新增

```go
// internal/data/ent/schema/memory_fact.go
type MemoryFact struct {
    ent.Schema
}

func (MemoryFact) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").DefaultFunc(uuid.NewString),
        field.String("agent_id").NotEmpty(),
        field.Text("statement").NotEmpty(),
        field.String("kind").Default("fact"),
        field.String("scope").Default("agent"),
        field.Float("confidence").Default(1.0),
        field.Float("importance").Default(0.5),
        field.Int("hit_count").Default(0),
        field.JSON("tags", []string{}).Optional(),
        field.String("source").Default("auto"),
        field.String("status").Default("active"),
        field.String("created_at").Default(time.NowString),
        field.String("updated_at").Default(time.NowString),
    }
}

// internal/data/ent/schema/memory_conflict.go
type MemoryConflict struct {
    ent.Schema
}

func (MemoryConflict) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").DefaultFunc(uuid.NewString),
        field.String("agent_id").NotEmpty(),
        field.JSON("fact_ids", []string{}),
        field.Text("description"),
        field.String("status").Default("open"),
        field.Text("resolution").Default(""),
        field.String("created_at").Default(time.NowString),
    }
}

// internal/data/ent/schema/evolution_proposal.go
type EvolutionProposal struct {
    ent.Schema
}

func (EvolutionProposal) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").DefaultFunc(uuid.NewString),
        field.String("agent_id").NotEmpty(),
        field.String("target_field").NotEmpty(),
        field.Text("current_value"),
        field.Text("proposed_value"),
        field.Text("rationale"),
        field.String("risk_level").Default("low"),
        field.String("status").Default("pending"),
        field.String("expires_at").Default(""),
        field.String("created_at").Default(time.NowString),
    }
}
```

---

## 五、运行时层

### 5.1 Runner MemoryService（实现真相）

```go
// internal/memory/trpc/sqlite_adapter.go
func NewSQLiteMemoryService(store *sessionmemory.Store) trpcmemory.Service

// internal/biz/memory_runtime_set.go
type RuntimeSet struct {
    TRPC  trpcmemory.Service   // Runner load_memory / preload_memory
    Admin SessionAdminStore    // MemoryAdminUsecase / memory/v1 API
}
```

Wire：`PersistenceSet.Memory` → `agent.BuildTRPCLLMAgent`；**禁止**在 `internal/service` import `pkg/trpc-agent-go`。

### 5.2 记忆注入

```go
// internal/agent/trpc_build.go
func WithMemoryInjection(ctx, ag, deps) llmagent.Option
```

根据 `settings` 决定注入哪些层：
- `l0_inject_l1` → 注入 L1 工作记忆
- `l0_inject_l3` → 注入 L3 语义检索
- `l0_inject_l4` → 注入 L4 身份/策略

---

## 六、Service 层

```go
func (s *MemoryService) ListMemories(ctx, req) (*ListMemoriesResponse, error)
func (s *MemoryService) RecallMemories(ctx, req) (*RecallMemoriesResponse, error)
func (s *MemoryService) GetMemoryOverview(ctx, req) (*MemoryOverviewResponse, error)

func (s *MemoryService) GetMemoryLayers(ctx context.Context, req *GetMemoryLayersRequest) (*GetMemoryLayersResponse, error) {
    layers, err := s.uc.GetLayers(ctx, req.AgentId, req.SessionId)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoLayers(layers), nil
}

func (s *MemoryService) SearchMemories(ctx context.Context, req *SearchMemoriesRequest) (*SearchMemoriesResponse, error) {
    results, err := s.uc.Search(ctx, req.AgentId, req.Query, int(req.TopK), req.MinScore)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    resp := &SearchMemoriesResponse{}
    for _, r := range results {
        resp.Results = append(resp.Results, toProtoSearchResult(r))
    }
    return resp, nil
}

func (s *MemoryService) ListMemoryFacts(ctx context.Context, req *ListMemoryFactsRequest) (*ListMemoryFactsResponse, error)
func (s *MemoryService) UpdateMemoryFact(ctx context.Context, req *UpdateMemoryFactRequest) (*MemoryFact, error)
func (s *MemoryService) DeleteMemoryFact(ctx context.Context, req *DeleteMemoryFactRequest) (*emptypb.Empty, error)
func (s *MemoryService) ConfirmMemoryFact(ctx context.Context, req *ConfirmMemoryFactRequest) (*MemoryFact, error)
func (s *MemoryService) RejectMemoryFact(ctx context.Context, req *RejectMemoryFactRequest) (*MemoryFact, error)
func (s *MemoryService) ListMemoryConflicts(ctx context.Context, req *ListMemoryConflictsRequest) (*ListMemoryConflictsResponse, error)
func (s *MemoryService) ResolveMemoryConflict(ctx context.Context, req *ResolveMemoryConflictRequest) (*emptypb.Empty, error)
func (s *MemoryService) ListMemoryEpisodes(ctx context.Context, req *ListMemoryEpisodesRequest) (*ListMemoryEpisodesResponse, error)
func (s *MemoryService) ConsolidateEpisode(ctx context.Context, req *ConsolidateEpisodeRequest) (*emptypb.Empty, error)
func (s *MemoryService) GetL0Snapshot(ctx context.Context, req *GetL0SnapshotRequest) (*L0Snapshot, error)
func (s *MemoryService) ListEvolutionProposals(ctx context.Context, req *ListEvolutionProposalsRequest) (*ListEvolutionProposalsResponse, error)
func (s *MemoryService) ApproveEvolutionProposal(ctx context.Context, req *ApproveEvolutionProposalRequest) (*emptypb.Empty, error)
func (s *MemoryService) RejectEvolutionProposal(ctx context.Context, req *RejectEvolutionProposalRequest) (*emptypb.Empty, error)
```

---

## 七、Wire 注入

已有：
```
data.ProviderSet → NewSessionMemoryStore
biz.ProviderSet → (MemoryUsecase 待创建)
service.ProviderSet → NewMemoryService
```

待新增：
```
data.ProviderSet → NewMemoryWorker, NewMemoryExtractionRepo, NewMemoryConflictRepo
biz.ProviderSet → NewMemoryManagementUsecase
memory.ProviderSet → NewMemoryExtractor, NewMemoryRetriever
service.ProviderSet → NewMemoryService
```

---

## 八、MemoryWorker 设计（EP-MEM-01 / EP-MEM-02）

> 来源：`_deprecated/需求/随心记.md`「记忆管家 Memory-Agent」需求，经可行性分析后调整为后台 goroutine 方案。

### 8.1 定位

MemoryWorker 是后台运行的记忆管理 goroutine（非独立进程），可配置开启和关闭。核心目标：

1. **认知一致性**：当某个记忆块变动时，自动复盘关联记忆块是否需要更新
2. **异步提取**：Turn 完成后自动从对话中提取 fact / episode / entity
3. **冲突检测**：新 fact 与现有 fact 矛盾时标记冲突
4. **级联更新**：实体属性变更时，沿图谱关系传播更新

### 8.2 架构

```
┌─────────────────────────────────────────────────────────┐
│                    Aranea 主进程                          │
│                                                          │
│  ┌──────────────┐    EventBus     ┌──────────────────┐  │
│  │ Chat/Runner  │ ──── event ───→ │ MemoryWorker     │  │
│  │ (对话运行时)  │   turn.completed│ (safego.Go)      │  │
│  └──────────────┘                 │                  │  │
│                                   │ 1. 异步提取       │  │
│  ┌──────────────┐                 │ 2. 冲突检测       │  │
│  │ CronRunner   │ ──── tick ────→ │ 3. 级联更新检查   │  │
│  │ (定时任务)    │   30min ticker  │ 4. 巩固管道       │  │
│  └──────────────┘                 │ 5. Proposal 管理  │  │
│                                   └──────────────────┘  │
│                                          │               │
│                                    ┌─────┴──────┐       │
│                                    │ L3 Facts   │       │
│                                    │ L4 Graph   │       │
│                                    └────────────┘       │
└─────────────────────────────────────────────────────────┘
```

### 8.3 Biz 层

```go
type MemoryWorker struct {
    extractor   *MemoryExtractor
    retriever   *MemoryRetriever
    graphRepo   MemoryGraphRepo
    factRepo    MemoryFactRepo
    proposalRepo EvolutionProposalRepo
    eventBus    *event.Bus
    settings    MemoryWorkerSettings
}

func NewMemoryWorker(
    extractor *MemoryExtractor,
    retriever *MemoryRetriever,
    graphRepo MemoryGraphRepo,
    factRepo MemoryFactRepo,
    proposalRepo EvolutionProposalRepo,
    eventBus *event.Bus,
    settings MemoryWorkerSettings,
) *MemoryWorker

func (w *MemoryWorker) Start(ctx context.Context)
func (w *MemoryWorker) Stop()
func (w *MemoryWorker) OnTurnCompleted(ctx context.Context, event TurnCompletedEvent)
func (w *MemoryWorker) RunConsolidation(ctx context.Context)
func (w *MemoryWorker) RunCascadeCheck(ctx context.Context, entityID string, changedAttribute string)
func (w *MemoryWorker) GetStatus() MemoryWorkerStatus
```

### 8.4 级联更新流程

```
实体属性变更（如 work_location: 北京 → 纽约）
        ↓
MemoryWorker.RunCascadeCheck(entityID, "work_location")
        ↓
1. 从 L4 图谱 BFS 查找关联实体（≤ max_hops 跳）
   - 交通方式 ← depends_on ← work_location
   - 天气偏好 ← depends_on ← work_location
   - 时区设置 ← depends_on ← work_location
        ↓
2. 对每个关联实体，检查是否需要更新
   - 调用 LLM 判断：属性变更是否影响该关联实体
   - 生成 CascadeCheckProposal
        ↓
3. Proposal 进入审核队列
   - proposal 模式：等待用户/Critic 确认
   - auto 模式：自动应用（高风险，默认关闭）
        ↓
4. 审核通过后应用更新
   - 更新关联 L3 fact（superseded_by 指向新 fact）
   - 更新关联 L4 实体属性
   - 记录 EvolutionEvent（含变更前后值，可回滚）
```

### 8.5 Data 层

```go
type memoryGraphRepo struct {
    db *ent.Client
}

func (r *memoryGraphRepo) CreateEntity(ctx, entity *MemoryEntity) error
func (r *memoryGraphRepo) GetEntity(ctx, id string) (*MemoryEntity, error)
func (r *memoryGraphRepo) UpdateEntityAttribute(ctx, id, attribute, value string) error
func (r *memoryGraphRepo) BFSNeighbors(ctx, entityID string, maxHops int) ([]*MemoryEntity, []*MemoryRelation, error)
func (r *memoryGraphRepo) CreateRelation(ctx, relation *MemoryRelation) error
func (r *memoryGraphRepo) GetRelationsByEntity(ctx, entityID string) ([]*MemoryRelation, error)
```

### 8.6 配置

通过 `agent_runtime_settings` 扩展：

```sql
ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_enabled INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_extract_mode TEXT NOT NULL DEFAULT 'auto';
ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_cascade_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_cascade_max_hops INTEGER NOT NULL DEFAULT 2;
ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_cascade_mode TEXT NOT NULL DEFAULT 'proposal';
ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_batch_size INTEGER NOT NULL DEFAULT 10;
ALTER TABLE agent_runtime_settings ADD COLUMN memory_worker_consolidation_interval_minutes INTEGER NOT NULL DEFAULT 30;
```

### 8.7 前端补充

MemoryWorker 相关前端组件：

| 组件 | 说明 |
|------|------|
| `MemoryWorkerStatusCard.vue` | MemoryWorker 运行状态卡片（运行中/暂停/错误） |
| `MemoryCascadeProposalCard.vue` | 级联更新提议卡片（实体变更→关联影响→diff） |
| `MemoryCascadeReviewDialog.vue` | 级联更新审核对话框（批量批准/拒绝） |

TypeScript 类型补充：

```typescript
export interface MemoryWorkerStatus {
  enabled: boolean
  running: boolean
  extractMode: 'auto' | 'manual' | 'hybrid'
  cascadeEnabled: boolean
  cascadeMaxHops: number
  cascadeMode: 'proposal' | 'auto'
  lastExtractionAt: string
  lastConsolidationAt: string
  pendingCascadeProposals: number
  extractionCount24h: number
  cascadeCount24h: number
}

export interface CascadeProposal {
  id: string
  agentId: string
  triggerEntityId: string
  triggerEntityName: string
  triggerAttribute: string
  oldValue: string
  newValue: string
  affectedEntities: CascadeAffectedEntity[]
  status: 'pending' | 'approved' | 'rejected' | 'expired' | 'applied'
  riskLevel: 'low' | 'medium' | 'high'
  rationale: string
  createdAt: string
  expiresAt: string
}

export interface CascadeAffectedEntity {
  entityId: string
  entityName: string
  entityType: string
  relationType: string
  hops: number
  suggestedUpdate: string
  currentFactIds: string[]
}
```

API 补充：

```typescript
export async function getMemoryWorkerStatus(agentId: string): Promise<MemoryWorkerStatus>
export async function listCascadeProposals(agentId: string, status?: string): Promise<CascadeProposal[]>
export async function approveCascadeProposal(id: string): Promise<void>
export async function rejectCascadeProposal(id: string, reason?: string): Promise<void>
```

路由补充：

```typescript
{ path: '/memory/worker', component: MemoryWorkerPage, name: 'MemoryWorker' },
```

记忆中心总览页补充——在 `MemoryCenterPage` 健康卡片区域新增：

| KPI | 组件 | 颜色语义 |
|-----|------|----------|
| Worker Status | `QBadge` | 运行中=绿 / 暂停=灰 / 错误=红 |
| Cascade Proposals | `QBadge` | 0=绿 / >0=黄 |
| Extraction 24h | `QBadge` | 蓝色 |

在待处理事项区域新增：

- 待审核级联更新提议
- MemoryWorker 错误/重试

---

## 九、前端实现设计

> UX 需求（页面目标、交互规范、验收标准）见 [`12-16 memory.md`](./12-16%20memory.md)，本节聚焦实现层：组件文件结构、组件设计、TypeScript 类型、API 函数、路由配置。

### 9.1 文件结构

```
web/src/features/memory/
├── api.ts
├── types.ts
├── composables/
│   ├── useMemoryOverview.ts
│   ├── useMemorySearch.ts
│   ├── useMemoryFacts.ts
│   └── useMemoryLayers.ts
└── components/
    ├── MemoryCenterPage.vue         ← 记忆中心主页
    ├── MemoryOverviewCard.vue       ← 总览卡片
    ├── MemoryHealthDashboard.vue    ← 健康仪表盘
    ├── MemoryKnowledgePage.vue      ← 知识库（L3 facts）
    ├── MemoryFactTable.vue          ← Fact 列表
    ├── MemoryFactDetailDrawer.vue   ← Fact 详情抽屉
    ├── MemoryConflictCard.vue       ← 冲突卡片
    ├── MemoryConflictDialog.vue     ← 冲突解决对话框
    ├── MemorySessionsPage.vue       ← 会话记忆（L0/L1/L2）
    ├── MemoryContextPanel.vue       ← L0 上下文窗口
    ├── MemoryWorkingPanel.vue       ← L1 工作记忆
    ├── MemoryTimelinePanel.vue      ← L2 事件时间线
    ├── MemoryEpisodeTable.vue       ← Episode 列表
    ├── MemoryGraphPage.vue          ← 知识图谱（L4 graph）
    ├── MemoryEvolutionPage.vue      ← Agent 进化（L4 evolution）
    ├── MemoryProposalCard.vue       ← 进化提议卡片
    ├── MemoryEvolutionLog.vue       ← 进化日志
    ├── MemoryDebugPage.vue          ← 调试工具
    ├── MemoryPromptPreview.vue      ← Prompt 预览
    ├── MemoryRecallTester.vue       ← Recall 测试器
    ├── MemoryLayerConfig.vue        ← 各层记忆配置面板
    └── MemorySettingsTab.vue        ← Agent 记忆设置 Tab
```

### 9.2 核心组件设计

**MemoryCenterPage.vue**：记忆中心主页

| 区域 | 组件 | 说明 |
|------|------|------|
| 顶部选择器 | `QSelect` + `QBtnToggle` | Agent 选择 + scope 切换 |
| 健康卡片 | `MemoryHealthDashboard` | 7 个 KPI 指标 |
| 记忆流向图 | `QCard` + SVG | L0←L1/L2/L3/L4 流程图 |
| 最近影响 | `QList` | 最近 10 条注入 prompt 的记忆 |
| 待处理事项 | `QList` | 冲突/PII/巩固失败/proposal |

**MemoryHealthDashboard.vue**：健康仪表盘

| KPI | 组件 | 颜色语义 |
|-----|------|----------|
| Context Used | `QCircularProgress` | <70% 绿 / 70-90% 橙 / >90% 红 |
| Active Working Tasks | `QBadge` | 蓝色 |
| Pending Episodes | `QBadge` | 黄色 |
| Active Facts | `QBadge` | 蓝色 |
| Open Conflicts | `QBadge` | 红色 |
| Pending Proposals | `QBadge` | 黄色 |
| Recall Hit Rate | `QCircularProgress` | 绿/黄/红 |

**MemoryKnowledgePage.vue**：知识库页面

| 区域 | 组件 | 说明 |
|------|------|------|
| 顶栏 | `QChip` + `QBtn` | scope 切换 + 新增 Fact |
| 统计 | `QCard` | 总数/活跃/归档/冲突/平均置信度 |
| 筛选 | `QSelect` + `QInput` | kind/tags/status/scope/关键字 |
| 表格 | `MemoryFactTable` | statement/kind/scope/confidence/hit_count |
| 详情 | `MemoryFactDetailDrawer` | 5 个 Tab：内容/证据/使用/反馈/版本 |
| 批量操作 | `QBtnGroup` | 归档/删除/重建 embedding/导出 |

**MemoryFactDetailDrawer.vue**：Fact 详情抽屉

| Tab | 组件 | 说明 |
|-----|------|------|
| 内容 | `QForm` | statement/details/tags/kind/scope |
| 证据 | `QList` | 来源 episode/session/message |
| 使用 | `QTable` | 最近召回记录、hit_count |
| 反馈 | `QTimeline` | confirm/reject/refine 时间线 |
| 版本 | `QCard` + diff | fact_versions、回滚按钮 |

**MemoryContextPanel.vue**：L0 上下文窗口

| 区域 | 组件 | 说明 |
|------|------|------|
| Token 仪表 | `QCircularProgress` | used ratio / max ratio |
| Prompt 分段瀑布 | `QExpansionItem` 列表 | system/skill/L1/L2/L3/L4/summary/history |
| 装配快照 | `QSelect` | 最近 20 次快照选择 |
| 操作 | `QBtn` | 重新预览/复制 prompt/调整设置 |

**MemoryTimelinePanel.vue**：L2 事件时间线

| 区域 | 组件 | 说明 |
|------|------|------|
| KPI | `QBadge` 行 | 消息数/模型调用/工具调用/失败数/tokens/成本 |
| 筛选 | `QSelect` + `QInput` | 类型/actor/状态/关键字 |
| 时间线 | `QTimeline` | turn 聚合卡片 |
| 标记菜单 | `QBtnDropdown` | 标星/巩固/复盘/好范例/坏范例/遗忘 |

**MemoryGraphPage.vue**：知识图谱页面

| 区域 | 组件 | 说明 |
|------|------|------|
| Sidebar | `QDrawer` | scope/entity_type/keyword 筛选 |
| 主图 | `@vue-flow/core` 或 D3.js | 力导图；节点大小=importance |
| 右侧详情 | `QDrawer` | entity 属性/relations/facts/versions |
| 降级视图 | `QTable` | 实体表格/邻居列表/关系表格 |

**MemoryEvolutionPage.vue**：Agent 进化页面

| 区域 | 组件 | 说明 |
|------|------|------|
| Identity 面板 | `QCard` + `QForm` | persona/values/tone/domains |
| Strategy 面板 | `QSlider` 行 | exploration/conciseness/caution/delegation |
| Proposal 审核 | `MemoryProposalCard` 列表 | 当前值 vs 建议值 diff + 操作按钮 |
| Evolution Log | `QTimeline` | 所有 EvolutionEvent |

**MemoryProposalCard.vue**：进化提议卡片

| 区域 | 组件 | 说明 |
|------|------|------|
| 目标 | `QLabel` | target_field |
| Diff | `QCard` | 当前值 vs 建议值（红绿 diff） |
| 理由 | `QExpansionItem` | rationale + 证据 |
| 风险 | `QBadge` | low 绿 / medium 黄 / high 红 |
| 操作 | `QBtn` | 批准/拒绝/延后 |

**MemoryLayerConfig.vue**：各层记忆配置面板

| 层 | 控件 | 绑定字段 | 说明 |
|---|------|----------|------|
| L0 | `QToggle` + `QInput` | `l0_inject_l1` + `l0_recent_window_turns` | 启用注入 + 窗口轮数 |
| L0 | `QInput` + `QSelect` | `l0_recent_window_tokens` + `l0_truncate_strategy` | 窗口 tokens + 裁剪策略 |
| L0 | `QSlider` | `l0_summary_threshold` | 摘要触发阈值 |
| L0 | `QToggle` × 3 | `l0_inject_l1`/`l0_inject_l3`/`l0_inject_l4` | 注入各层开关 |
| L1 | `QToggle` | `l1_enabled` | 启用工作记忆 |
| L1 | `QInput` | `l1_budget_tokens` + `l1_field_max_tokens` | 预算 + 单字段上限 |
| L2 | `QToggle` + `QSlider` | `l2_episode_enabled` + `l2_episode_min_importance` | 启用 + 重要性阈值 |
| L2 | `QToggle` + `QSelect` | `l2_index_enabled` + `l2_index_embedding_model` | 启用索引 + 嵌入模型 |
| L3 | `QToggle` + `QInput` | `memory_enabled` + `memory_max_results` | 启用 + topK |
| L3 | `QSlider` | `memory_min_score` | 最低分数 |
| L4 | `QToggle` | `evolution_self_evolve` | 启用自我演化 |

**MemorySettingsTab.vue**：Agent 记忆设置 Tab

| 区域 | 组件 | 说明 |
|------|------|------|
| 总开关 | `QToggle` | 记忆总开关 |
| 模式选择 | `QBtnToggle` | 轻量/标准/深度 |
| 预计影响 | `QBadge` | 低/中/高 |
| 隐私级别 | `QSelect` | 严格/标准/宽松 |
| 各层配置 | `MemoryLayerConfig` | L0-L4 各层详细配置 |
| 配置防呆 | `QBanner` | 自动禁用冲突配置 + 风险提示 |

### 9.3 TypeScript 类型定义

```typescript
// web/src/features/memory/types.ts
export interface MemoryOverview {
  contextUsedRatio: number
  activeWorkingTasks: number
  pendingEpisodes: number
  activeFacts: number
  openConflicts: number
  pendingProposals: number
  recallHitRate: number
  recentInjected: MemorySearchResult[]
}

export interface MemoryFact {
  id: string
  statement: string
  kind: 'preference' | 'fact' | 'rule' | 'experience'
  scope: 'user' | 'agent' | 'team' | 'workspace' | 'global'
  confidence: number
  importance: number
  hitCount: number
  tags: string[]
  source: string
  status: 'active' | 'archived' | 'disputed'
  createdAt: string
  updatedAt: string
}

export interface MemoryConflict {
  id: string
  factIds: string[]
  description: string
  status: 'open' | 'resolved'
  createdAt: string
}

export interface Episode {
  id: string
  sessionId: string
  agentId: string
  title: string
  kind: 'task' | 'decision' | 'error_recovery' | 'feedback'
  outcome: string
  importance: number
  confidence: number
  consolidationStatus: 'pending' | 'consolidated' | 'skipped'
  createdAt: string
}

export interface EvolutionProposal {
  id: string
  agentId: string
  targetField: string
  currentValue: string
  proposedValue: string
  rationale: string
  riskLevel: 'low' | 'medium' | 'high'
  status: 'pending' | 'approved' | 'rejected' | 'expired'
  expiresAt: string
  createdAt: string
}

export interface MemorySearchResult {
  id: string
  content: string
  score: number
  layer: string
  scope: string
  source: string
  createdAt: string
}

export interface L0Snapshot {
  id: string
  sessionId: string
  contextWindowTokens: number
  promptTokenEstimate: number
  usedRatio: number
  segmentsJson: string
  warningCodes: string[]
  createdAt: string
}

export interface FactQueryParams {
  scope?: string
  kind?: string
  status?: string
  pageSize?: number
  pageToken?: string
}

export interface UpdateFactRequest {
  statement?: string
  tags?: string[]
  scope?: string
}

export interface ResolveConflictRequest {
  action: 'keep_a' | 'keep_b' | 'merge' | 'disputed' | 'split_scope'
  winnerFactId?: string
  mergedStatement?: string
}
```

### 9.4 TypeScript API 定义

```typescript
// web/src/features/memory/api.ts
import type {
  MemoryOverview, MemoryFact, MemoryConflict, Episode,
  EvolutionProposal, L0Snapshot, MemorySearchResult,
  GetMemoryLayersResponse
} from './types'

export async function getMemoryOverview(agentId: string, scope?: string): Promise<MemoryOverview>
export async function getMemoryLayers(agentId: string, sessionId?: string): Promise<GetMemoryLayersResponse>
export async function searchMemories(agentId: string, query: string, topK?: number, minScore?: number): Promise<MemorySearchResult[]>
export async function listMemoryFacts(agentId: string, params: FactQueryParams): Promise<{ facts: MemoryFact[]; totalCount: number }>
export async function updateMemoryFact(id: string, req: UpdateFactRequest): Promise<MemoryFact>
export async function deleteMemoryFact(id: string): Promise<void>
export async function confirmMemoryFact(id: string): Promise<MemoryFact>
export async function rejectMemoryFact(id: string, reason?: string): Promise<MemoryFact>
export async function listMemoryConflicts(agentId: string, status?: string): Promise<MemoryConflict[]>
export async function resolveMemoryConflict(id: string, req: ResolveConflictRequest): Promise<void>
export async function listMemoryEpisodes(sessionId: string, params?: EpisodeQueryParams): Promise<{ episodes: Episode[]; totalCount: number }>
export async function consolidateEpisode(id: string): Promise<void>
export async function getL0Snapshot(sessionId: string, limit?: number): Promise<L0Snapshot>
export async function listEvolutionProposals(agentId: string, status?: string): Promise<EvolutionProposal[]>
export async function approveEvolutionProposal(id: string): Promise<void>
export async function rejectEvolutionProposal(id: string, reason?: string): Promise<void>
```

### 9.5 路由配置

```typescript
const memoryRoutes = [
  { path: '/memory', component: MemoryCenterPage, name: 'MemoryCenter' },
  { path: '/memory/knowledge', component: MemoryKnowledgePage, name: 'MemoryKnowledge' },
  { path: '/memory/sessions', component: MemorySessionsPage, name: 'MemorySessions' },
  { path: '/memory/graph', component: MemoryGraphPage, name: 'MemoryGraph' },
  { path: '/memory/evolution', component: MemoryEvolutionPage, name: 'MemoryEvolution' },
  { path: '/memory/debug', component: MemoryDebugPage, name: 'MemoryDebug' },
  { path: '/memory/worker', component: MemoryWorkerPage, name: 'MemoryWorker' },
]
```
