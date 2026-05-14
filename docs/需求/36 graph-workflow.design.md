# Graph 工作流模块 — 实现设计文档

> 对应需求：`36 graph-workflow.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Graph 工作流引擎：基于 trpc-agent-go `graph` 包构建完整的 Graph 工作流系统，包括节点/边/条件路由、State Schema + Reducer、HITL 中断/恢复、Checkpoint 持久化、时间旅行调试、子图嵌套、DAG 并行执行、缓存/重试策略、可视化导出。

**现有基础**：`internal/graph/trpc/builder.go` 已实现基础 Graph 构建（`GraphBuildConfig` → `BuildStateGraph` → `GraphAgent`），但缺少 State Schema/Reducer、HITL、Checkpoint、时间旅行、子图、DAG、API 端点和可视化编辑器。

---

## 二、Proto 层

### 2.1 新增文件

`api/kratos/graph/v1/graph.proto`

```protobuf
syntax = "proto3";

package graph.v1;

option go_package = "aranea-agents/api/kratos/graph/v1;v1";

import "google/api/annotations.proto";
import "google/protobuf/empty.proto";
import "google/protobuf/timestamp.proto";

// ===== 消息定义 =====

message StateFieldDef {
  string name = 1;
  string type = 2;           // "string"/"integer"/"float"/"bool"/"[]string"/"[]any"/"map"/"messages"
  string reducer = 3;        // "default"/"append"/"merge"/"cover"/"message"/"custom"
  string default_value = 4;  // JSON 编码的默认值
  bool required = 5;
  bool disable_deep_copy = 6;
}

message NodeDef {
  string id = 1;
  string name = 2;
  string type = 3;           // "function"/"llm"/"tool"/"agent"/"join"/"router"
  string description = 4;
  string config_json = 5;    // 节点配置（LLM instruction、tool 名称等）
  string instruction = 6;    // LLM 节点指令
  string model_name = 7;     // LLM 节点模型
  repeated string tool_names = 8;  // 工具节点工具名
  string agent_name = 9;     // Agent 子图节点名
  string subgraph_id = 10;   // 子图 ID
  string input_mapper_json = 11;   // 子图输入映射
  string output_mapper_json = 12;  // 子图输出映射
  bool isolated_messages = 13;     // 子图隔离消息
  bool interrupt_before = 14;
  bool interrupt_after = 15;
  string ends_json = 16;     // map[string]string 命名出口
  string destinations_json = 17;  // map[string]string 声明目标
  string cache_policy_json = 18;  // 缓存策略
  string retry_policy_json = 19;  // 重试策略
  string stream_output_name = 20; // 流式输出通道名
}

message EdgeDef {
  string from = 1;
  string to = 2;
}

message ConditionalEdgeDef {
  string from = 1;
  string condition_expr = 2;       // 条件表达式
  map<string, string> path_map = 3; // 分支 → 目标节点
}

message Graph {
  string id = 1;
  string name = 2;
  string description = 3;
  repeated NodeDef nodes = 4;
  repeated EdgeDef edges = 5;
  repeated ConditionalEdgeDef conditional_edges = 6;
  repeated StateFieldDef state_fields = 7;
  string entry_point = 8;
  string finish_point = 9;
  string execution_engine = 10;  // "bsp"/"dag"
  int32 max_steps = 11;
  int32 max_concurrency = 12;
  string version = 13;           // 图版本（缓存命名空间）
  google.protobuf.Timestamp created_at = 14;
  google.protobuf.Timestamp updated_at = 15;
}

message GraphRun {
  string id = 1;
  string graph_id = 2;
  string session_id = 3;
  string status = 4;         // "running"/"waiting_human"/"completed"/"failed"/"cancelled"
  string current_node = 5;
  string lineage_id = 6;     // 检查点 lineage
  string error_message = 7;
  google.protobuf.Timestamp started_at = 8;
  google.protobuf.Timestamp ended_at = 9;
}

message Checkpoint {
  string id = 1;
  string run_id = 2;
  string lineage_id = 3;
  string namespace = 4;
  string parent_checkpoint_id = 5;
  string source = 6;           // "input"/"loop"/"update"/"fork"/"interrupt"
  int32 step = 7;
  string state_json = 8;       // 状态快照
  string next_nodes_json = 9;  // 下一步节点
  string interrupt_state_json = 10; // 中断状态
  google.protobuf.Timestamp created_at = 11;
}

message StateSnapshot {
  CheckpointInfo checkpoint_info = 1;
  string state_json = 2;
  string next_nodes_json = 3;
  string next_channels_json = 4;
}

message CheckpointInfo {
  string lineage_id = 1;
  string namespace = 2;
  string checkpoint_id = 3;
  string parent_checkpoint = 4;
  string source = 5;
  int32 step = 6;
  google.protobuf.Timestamp timestamp = 7;
}

// ===== 请求/响应 =====

message ListGraphsRequest {
  int32 page = 1;
  int32 page_size = 2;
  string keyword = 3;
}

message ListGraphsResponse {
  repeated Graph items = 1;
  int32 total = 2;
}

message CreateGraphRequest {
  string name = 1;
  string description = 2;
  repeated NodeDef nodes = 3;
  repeated EdgeDef edges = 4;
  repeated ConditionalEdgeDef conditional_edges = 5;
  repeated StateFieldDef state_fields = 6;
  string entry_point = 7;
  string finish_point = 8;
  string execution_engine = 9;
  int32 max_steps = 10;
  int32 max_concurrency = 11;
  string version = 12;
}

message GetGraphRequest {
  string id = 1;
}

message UpdateGraphRequest {
  string id = 1;
  string name = 2;
  string description = 3;
  repeated NodeDef nodes = 4;
  repeated EdgeDef edges = 5;
  repeated ConditionalEdgeDef conditional_edges = 6;
  repeated StateFieldDef state_fields = 7;
  string entry_point = 8;
  string finish_point = 9;
  string execution_engine = 10;
  int32 max_steps = 11;
  int32 max_concurrency = 12;
  string version = 13;
}

message DeleteGraphRequest {
  string id = 1;
}

message RunGraphRequest {
  string id = 1;                 // Graph ID
  string session_id = 2;         // 关联会话
  string input_json = 3;         // 初始状态 JSON
  string checkpoint_id = 4;      // 恢复检查点 ID（可选）
  string resume_value_json = 5;  // 恢复值（HITL）
  string resume_map_json = 6;    // 恢复映射
}

message RunGraphResponse {
  GraphRun run = 1;
}

message GetGraphRunRequest {
  string graph_id = 1;
  string run_id = 2;
}

message ListGraphRunsRequest {
  string graph_id = 1;
  int32 page = 2;
  int32 page_size = 3;
}

message ListGraphRunsResponse {
  repeated GraphRun items = 1;
  int32 total = 2;
}

message CancelGraphRunRequest {
  string graph_id = 1;
  string run_id = 2;
}

message CancelGraphRunResponse {}

message GetCheckpointRequest {
  string run_id = 1;
  string checkpoint_id = 2;
}

message ListCheckpointsRequest {
  string run_id = 1;
  string lineage_id = 2;
  int32 limit = 3;
}

message ListCheckpointsResponse {
  repeated CheckpointInfo items = 1;
}

message ResumeFromCheckpointRequest {
  string run_id = 1;
  string checkpoint_id = 2;
  string resume_value_json = 3;
  string resume_map_json = 4;
}

message ResumeFromCheckpointResponse {
  GraphRun run = 1;
}

message GetStateSnapshotRequest {
  string lineage_id = 1;
  string checkpoint_id = 2;
  string namespace = 3;
}

message EditStateRequest {
  string lineage_id = 1;
  string checkpoint_id = 2;
  string namespace = 3;
  string patch_json = 4;       // 要修改的状态 patch
}

message EditStateResponse {
  CheckpointRef new_checkpoint = 1;
}

message CheckpointRef {
  string lineage_id = 1;
  string namespace = 2;
  string checkpoint_id = 3;
}

message GetGraphDotRequest {
  string id = 1;
  string rank_dir = 2;        // "LR"/"TB"
  bool include_destinations = 3;
  bool include_start_end = 4;
}

message GetGraphDotResponse {
  string dot = 1;
}

// ===== Service =====

service GraphService {
  rpc ListGraphs(ListGraphsRequest) returns (ListGraphsResponse) {
    option (google.api.http) = { get: "/v1/graphs" };
  }
  rpc CreateGraph(CreateGraphRequest) returns (Graph) {
    option (google.api.http) = { post: "/v1/graphs" body: "*" };
  }
  rpc GetGraph(GetGraphRequest) returns (Graph) {
    option (google.api.http) = { get: "/v1/graphs/{id}" };
  }
  rpc UpdateGraph(UpdateGraphRequest) returns (Graph) {
    option (google.api.http) = { put: "/v1/graphs/{id}" body: "*" };
  }
  rpc DeleteGraph(DeleteGraphRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/graphs/{id}" };
  }
  rpc RunGraph(RunGraphRequest) returns (RunGraphResponse) {
    option (google.api.http) = { post: "/v1/graphs/{id}/run" body: "*" };
  }
  rpc GetGraphRun(GetGraphRunRequest) returns (GraphRun) {
    option (google.api.http) = { get: "/v1/graphs/{graph_id}/runs/{run_id}" };
  }
  rpc ListGraphRuns(ListGraphRunsRequest) returns (ListGraphRunsResponse) {
    option (google.api.http) = { get: "/v1/graphs/{graph_id}/runs" };
  }
  rpc CancelGraphRun(CancelGraphRunRequest) returns (CancelGraphRunResponse) {
    option (google.api.http) = { post: "/v1/graphs/{graph_id}/runs/{run_id}/cancel" body: "*" };
  }
  rpc GetCheckpoint(GetCheckpointRequest) returns (Checkpoint) {
    option (google.api.http) = { get: "/v1/graphs/runs/{run_id}/checkpoints/{checkpoint_id}" };
  }
  rpc ListCheckpoints(ListCheckpointsRequest) returns (ListCheckpointsResponse) {
    option (google.api.http) = { get: "/v1/graphs/runs/{run_id}/checkpoints" };
  }
  rpc ResumeFromCheckpoint(ResumeFromCheckpointRequest) returns (ResumeFromCheckpointResponse) {
    option (google.api.http) = { post: "/v1/graphs/runs/{run_id}/checkpoints/{checkpoint_id}/resume" body: "*" };
  }
  rpc GetStateSnapshot(GetStateSnapshotRequest) returns (StateSnapshot) {
    option (google.api.http) = { get: "/v1/graphs/state-snapshot" };
  }
  rpc EditState(EditStateRequest) returns (EditStateResponse) {
    option (google.api.http) = { post: "/v1/graphs/state-edit" body: "*" };
  }
  rpc GetGraphDot(GetGraphDotRequest) returns (GetGraphDotResponse) {
    option (google.api.http) = { get: "/v1/graphs/{id}/dot" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
// internal/biz/graph.go

type Graph struct {
    ID               string
    Name             string
    Description      string
    NodesJSON        string
    EdgesJSON        string
    CondEdgesJSON    string
    StateFieldsJSON  string
    EntryPoint       string
    FinishPoint      string
    ExecutionEngine  string
    MaxSteps         int
    MaxConcurrency   int
    Version          string
    CreatedAt        time.Time
    UpdatedAt        time.Time
}

type GraphNodeDef struct {
    ID               string                 `json:"id"`
    Name             string                 `json:"name"`
    Type             string                 `json:"type"`
    Description      string                 `json:"description"`
    ConfigJSON       string                 `json:"config_json,omitempty"`
    Instruction      string                 `json:"instruction,omitempty"`
    ModelName        string                 `json:"model_name,omitempty"`
    ToolNames        []string               `json:"tool_names,omitempty"`
    AgentName        string                 `json:"agent_name,omitempty"`
    SubgraphID       string                 `json:"subgraph_id,omitempty"`
    InputMapperJSON  string                 `json:"input_mapper_json,omitempty"`
    OutputMapperJSON string                 `json:"output_mapper_json,omitempty"`
    IsolatedMessages bool                   `json:"isolated_messages"`
    InterruptBefore  bool                   `json:"interrupt_before"`
    InterruptAfter   bool                   `json:"interrupt_after"`
    EndsJSON         string                 `json:"ends_json,omitempty"`
    DestinationsJSON string                 `json:"destinations_json,omitempty"`
    CachePolicyJSON  string                 `json:"cache_policy_json,omitempty"`
    RetryPolicyJSON  string                 `json:"retry_policy_json,omitempty"`
    StreamOutputName string                 `json:"stream_output_name,omitempty"`
}

type GraphEdgeDef struct {
    From string `json:"from"`
    To   string `json:"to"`
}

type GraphCondEdgeDef struct {
    From         string            `json:"from"`
    ConditionExpr string           `json:"condition_expr"`
    PathMap      map[string]string `json:"path_map"`
}

type GraphStateFieldDef struct {
    Name           string `json:"name"`
    Type           string `json:"type"`
    Reducer        string `json:"reducer"`
    DefaultValue   string `json:"default_value,omitempty"`
    Required       bool   `json:"required"`
    DisableDeepCopy bool  `json:"disable_deep_copy"`
}

type GraphRun struct {
    ID            string
    GraphID       string
    SessionID     string
    Status        string
    CurrentNode   string
    LineageID     string
    ErrorMessage  string
    StartedAt     time.Time
    EndedAt       time.Time
}

type GraphCheckpoint struct {
    ID                 string
    RunID              string
    LineageID          string
    Namespace          string
    ParentCheckpointID string
    Source             string
    Step               int
    StateJSON          string
    NextNodesJSON      string
    InterruptStateJSON string
    CreatedAt          time.Time
}
```

### 3.2 Repo 接口

```go
// internal/biz/graph.go

type GraphRepo interface {
    Save(ctx context.Context, g *Graph) (*Graph, error)
    Get(ctx context.Context, id string) (*Graph, error)
    List(ctx context.Context, keyword string, page, pageSize int) ([]*Graph, int, error)
    Update(ctx context.Context, g *Graph) (*Graph, error)
    Delete(ctx context.Context, id string) error
}

type GraphRunRepo interface {
    Save(ctx context.Context, r *GraphRun) (*GraphRun, error)
    Get(ctx context.Context, id string) (*GraphRun, error)
    ListByGraph(ctx context.Context, graphID string, page, pageSize int) ([]*GraphRun, int, error)
    Update(ctx context.Context, r *GraphRun) (*GraphRun, error)
}

type GraphCheckpointRepo interface {
    Save(ctx context.Context, cp *GraphCheckpoint) (*GraphCheckpoint, error)
    Get(ctx context.Context, runID, checkpointID string) (*GraphCheckpoint, error)
    ListByRun(ctx context.Context, runID string, limit int) ([]*GraphCheckpoint, error)
    ListByLineage(ctx context.Context, lineageID string, limit int) ([]*GraphCheckpoint, error)
    DeleteByLineage(ctx context.Context, lineageID string) error
}
```

### 3.3 Usecase

```go
// internal/biz/graph.go

type GraphUsecase struct {
    graphRepo      GraphRepo
    runRepo        GraphRunRepo
    checkpointRepo GraphCheckpointRepo
    graphBuilder   GraphBuilder
    log            *log.Helper
}

func NewGraphUsecase(
    graphRepo GraphRepo,
    runRepo GraphRunRepo,
    checkpointRepo GraphCheckpointRepo,
    graphBuilder GraphBuilder,
    logger log.Logger,
) *GraphUsecase

func (uc *GraphUsecase) CreateGraph(ctx context.Context, g *Graph) (*Graph, error)
func (uc *GraphUsecase) GetGraph(ctx context.Context, id string) (*Graph, error)
func (uc *GraphUsecase) ListGraphs(ctx context.Context, keyword string, page, pageSize int) ([]*Graph, int, error)
func (uc *GraphUsecase) UpdateGraph(ctx context.Context, g *Graph) (*Graph, error)
func (uc *GraphUsecase) DeleteGraph(ctx context.Context, id string) error

func (uc *GraphUsecase) RunGraph(ctx context.Context, graphID, sessionID string, initialState map[string]any, checkpointID string, resumeValue map[string]any) (*GraphRun, error)
func (uc *GraphUsecase) GetGraphRun(ctx context.Context, id string) (*GraphRun, error)
func (uc *GraphUsecase) ListGraphRuns(ctx context.Context, graphID string, page, pageSize int) ([]*GraphRun, int, error)
func (uc *GraphUsecase) CancelGraphRun(ctx context.Context, id string) error

func (uc *GraphUsecase) GetCheckpoint(ctx context.Context, runID, checkpointID string) (*GraphCheckpoint, error)
func (uc *GraphUsecase) ListCheckpoints(ctx context.Context, runID, lineageID string, limit int) ([]*GraphCheckpoint, error)
func (uc *GraphUsecase) ResumeFromCheckpoint(ctx context.Context, runID, checkpointID string, resumeValue map[string]any) (*GraphRun, error)

func (uc *GraphUsecase) GetStateSnapshot(ctx context.Context, lineageID, checkpointID, namespace string) (*StateSnapshot, error)
func (uc *GraphUsecase) EditState(ctx context.Context, lineageID, checkpointID, namespace string, patch map[string]any) (*CheckpointRef, error)

func (uc *GraphUsecase) GetGraphDot(ctx context.Context, id string, rankDir string, includeDestinations, includeStartEnd bool) (string, error)
```

---

## 四、运行时层（internal/graph）

### 4.1 GraphBuilder 接口

```go
// internal/graph/builder.go

type GraphBuilder interface {
    BuildAndRun(ctx context.Context, cfg *BuildConfig, initialState map[string]any, checkpointID string, resumeValue map[string]any) (<-chan *event.Event, *trpcgraph.Executor, error)
    BuildGraph(ctx context.Context, cfg *BuildConfig) (*trpcgraph.Graph, error)
    GetDot(ctx context.Context, g *trpcgraph.Graph, opts ...trpcgraph.VizOption) string
}
```

### 4.2 增强现有 builder.go

```go
// internal/graph/trpc/builder.go（增强）

type BuildConfig struct {
    GraphID          string
    Nodes            []NodeDef
    Edges            []EdgeDef
    ConditionalEdges []ConditionalEdgeDef
    StateFields      []StateFieldDef
    EntryPoint       string
    FinishPoint      string
    ExecutionEngine  string
    MaxSteps         int
    MaxConcurrency   int
    Version          string
    CheckpointSaver  trpcgraph.CheckpointSaver
}

type StateFieldDef struct {
    Name           string
    Type           reflect.Type
    Reducer        trpcgraph.StateReducer
    Default        func() any
    Required       bool
    DisableDeepCopy bool
}

func BuildStateGraph(cfg BuildConfig) (*trpcgraph.Graph, error) {
    schema := trpcgraph.NewStateSchema()
    for _, sf := range cfg.StateFields {
        schema.AddField(sf.Name, trpcgraph.StateField{
            Type:            sf.Type,
            Reducer:         sf.Reducer,
            Default:         sf.Default,
            Required:        sf.Required,
            DisableDeepCopy: sf.DisableDeepCopy,
        })
    }

    sg := trpcgraph.NewStateGraph(schema)

    for _, n := range cfg.Nodes {
        opts := buildNodeOptions(n)
        switch n.Type {
        case "function":
            sg.AddNode(n.ID, n.Func, opts...)
        case "llm":
            sg.AddLLMNode(n.ID, n.Model, n.Instruction, n.Tools, opts...)
        case "tool":
            sg.AddToolsNode(n.ID, n.Tools, opts...)
        case "agent":
            sg.AddAgentNode(n.ID, n.Agent, opts...)
        }
    }

    sg.AddEdge(trpcgraph.Start, cfg.EntryPoint)
    for _, e := range cfg.Edges {
        sg.AddEdge(e.From, e.To)
    }
    for _, ce := range cfg.ConditionalEdges {
        sg.AddConditionalEdges(ce.From, ce.CondFunc, ce.PathMap)
    }
    if cfg.FinishPoint != "" {
        sg.AddEdge(cfg.FinishPoint, trpcgraph.End)
    }

    return sg.Compile()
}

func BuildExecutor(g *trpcgraph.Graph, cfg BuildConfig) (*trpcgraph.Executor, error) {
    var engine trpcgraph.ExecutionEngine = trpcgraph.ExecutionEngineBSP
    if cfg.ExecutionEngine == "dag" {
        engine = trpcgraph.ExecutionEngineDAG
    }

    opts := []trpcgraph.ExecutorOption{
        trpcgraph.WithExecutionEngine(engine),
    }
    if cfg.MaxSteps > 0 {
        opts = append(opts, trpcgraph.WithMaxSteps(cfg.MaxSteps))
    }
    if cfg.MaxConcurrency > 0 {
        opts = append(opts, trpcgraph.WithMaxConcurrency(cfg.MaxConcurrency))
    }
    if cfg.CheckpointSaver != nil {
        opts = append(opts, trpcgraph.WithCheckpointSaver(cfg.CheckpointSaver))
    }

    return trpcgraph.NewExecutor(g, opts...)
}
```

### 4.3 HITL 中断/恢复

```go
// internal/graph/trpc/hitl.go

func InterruptNode(ctx context.Context, state trpcgraph.State, key string, prompt any) (any, error) {
    return trpcgraph.Interrupt(ctx, state, key, prompt)
}

func ResumeValue[T any](ctx context.Context, state trpcgraph.State, key string) (T, bool) {
    return trpcgraph.ResumeValue[T](ctx, state, key)
}

func BuildResumeState(resumeValue map[string]any) map[string]any {
    state := map[string]any{
        trpcgraph.CfgKeyCheckpointID: resumeValue["checkpoint_id"],
        trpcgraph.CfgKeyLineageID:    resumeValue["lineage_id"],
    }
    if ns, ok := resumeValue["namespace"].(string); ok && ns != "" {
        state[trpcgraph.CfgKeyCheckpointNS] = ns
    }
    if rm, ok := resumeValue["resume_map"].(map[string]any); ok {
        state[trpcgraph.CfgKeyResumeMap] = rm
    }
    if rv, ok := resumeValue["resume_value"]; ok {
        state[trpcgraph.ResumeChannel] = rv
    }
    return state
}
```

### 4.4 Checkpoint 持久化

```go
// internal/graph/trpc/checkpoint.go

type SQLiteCheckpointSaver struct {
    db *ent.Client
}

func NewSQLiteCheckpointSaver(db *ent.Client) *SQLiteCheckpointSaver

func (s *SQLiteCheckpointSaver) Get(ctx context.Context, config map[string]any) (*trpcgraph.Checkpoint, error)
func (s *SQLiteCheckpointSaver) GetTuple(ctx context.Context, config map[string]any) (*trpcgraph.CheckpointTuple, error)
func (s *SQLiteCheckpointSaver) List(ctx context.Context, config map[string]any, filter *trpcgraph.CheckpointFilter) ([]*trpcgraph.CheckpointTuple, error)
func (s *SQLiteCheckpointSaver) Put(ctx context.Context, req trpcgraph.PutRequest) (map[string]any, error)
func (s *SQLiteCheckpointSaver) PutWrites(ctx context.Context, req trpcgraph.PutWritesRequest) error
func (s *SQLiteCheckpointSaver) PutFull(ctx context.Context, req trpcgraph.PutFullRequest) (map[string]any, error)
func (s *SQLiteCheckpointSaver) DeleteLineage(ctx context.Context, lineageID string) error
func (s *SQLiteCheckpointSaver) Close() error
```

实现要点：
- 实现 `trpcgraph.CheckpointSaver` 接口
- 使用 Ent ORM 操作 `graph_checkpoint` 表
- `ChannelValues`、`ChannelVersions`、`VersionsSeen` 等字段 JSON 序列化存储
- `PutFull` 使用事务保证原子性
- `List` 支持按 `lineage_id` + `namespace` 过滤，按 `timestamp DESC` 排序

### 4.5 时间旅行

```go
// internal/graph/trpc/timetravel.go

type TimeTravelService struct {
    executor *trpcgraph.Executor
}

func NewTimeTravelService(executor *trpcgraph.Executor) (*TimeTravelService, error)

func (t *TimeTravelService) GetState(ctx context.Context, ref trpcgraph.CheckpointRef) (*trpcgraph.StateSnapshot, error)
func (t *TimeTravelService) History(ctx context.Context, lineageID, namespace string, limit int) ([]trpcgraph.CheckpointInfo, error)
func (t *TimeTravelService) EditState(ctx context.Context, base trpcgraph.CheckpointRef, patch trpcgraph.State) (trpcgraph.CheckpointRef, error)
```

### 4.6 子图嵌套

```go
// internal/graph/trpc/subgraph.go

func BuildSubgraphNode(subGraph *trpcgraph.Graph, inputMapper trpcgraph.SubgraphInputMapper, outputMapper trpcgraph.SubgraphOutputMapper) (trpcgraph.NodeFunc, error)

func DefaultInputMapper(parent trpcgraph.State) trpcgraph.State {
    child := make(trpcgraph.State)
    if msgs, ok := trpcgraph.GetStateValue[[]model.Message](parent, trpcgraph.StateKeyMessages); ok {
        child[trpcgraph.StateKeyMessages] = msgs
    }
    if ui, ok := trpcgraph.GetStateValue[string](parent, trpcgraph.StateKeyUserInput); ok {
        child[trpcgraph.StateKeyUserInput] = ui
    }
    return child
}

func DefaultOutputMapper(parent trpcgraph.State, result trpcgraph.SubgraphResult) trpcgraph.State {
    update := make(trpcgraph.State)
    if result.LastResponse != "" {
        update[trpcgraph.StateKeyLastResponse] = result.LastResponse
    }
    return update
}
```

### 4.7 可视化导出

```go
// internal/graph/trpc/visualize.go

func GraphToDot(g *trpcgraph.Graph, rankDir string, includeDestinations, includeStartEnd bool) string {
    opts := []trpcgraph.VizOption{
        trpcgraph.WithRankDir(rankDir),
        trpcgraph.WithIncludeDestinations(includeDestinations),
        trpcgraph.WithIncludeStartEnd(includeStartEnd),
    }
    return g.DOT(opts...)
}

func GraphToMermaid(g *trpcgraph.Graph) string {
    // 基于 DOT 输出转换或直接遍历 Graph 结构生成 Mermaid 格式
}
```

### 4.8 RunRegistry（运行时管理）

```go
// internal/graph/trpc/registry.go

type ActiveRun struct {
    RunID     string
    GraphID   string
    SessionID string
    LineageID string
    Executor  *trpcgraph.Executor
    Cancel    context.CancelFunc
    Status    string
    StartedAt time.Time
}

type RunRegistry struct {
    mu        sync.RWMutex
    runs      map[string]*ActiveRun
    bySession map[string]string
}

func NewRunRegistry() *RunRegistry

func (r *RunRegistry) Register(runID, graphID, sessionID, lineageID string, executor *trpcgraph.Executor, cancel context.CancelFunc) error
func (r *RunRegistry) Get(runID string) (*ActiveRun, bool)
func (r *RunRegistry) GetBySession(sessionID string) (*ActiveRun, bool)
func (r *RunRegistry) CancelRun(runID string) bool
func (r *RunRegistry) UpdateStatus(runID, status string)
func (r *RunRegistry) Remove(runID string)
```

---

## 五、Data 层

### 5.1 Ent Schema

**`internal/data/ent/schema/graph.go`**

```go
type Graph struct {
    ent.Schema
}

func (Graph) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").DefaultFunc(uuid.NewString),
        field.String("name").NotEmpty(),
        field.String("description").Default(""),
        field.Text("nodes_json").Default("[]"),
        field.Text("edges_json").Default("[]"),
        field.Text("cond_edges_json").Default("[]"),
        field.Text("state_fields_json").Default("[]"),
        field.String("entry_point").NotEmpty(),
        field.String("finish_point").Default(""),
        field.String("execution_engine").Default("bsp"),
        field.Int("max_steps").Default(100),
        field.Int("max_concurrency").Default(0),
        field.String("version").Default(""),
        field.Time("created_at").Default(time.Now),
        field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
    }
}

func (Graph) Edges() []ent.Edge {
    return []ent.Edge{
        edge.To("runs", GraphRun.Type),
    }
}
```

**`internal/data/ent/schema/graph_run.go`**

```go
type GraphRun struct {
    ent.Schema
}

func (GraphRun) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").DefaultFunc(uuid.NewString),
        field.String("graph_id").NotEmpty(),
        field.String("session_id").Default(""),
        field.String("status").Default("running"),
        field.String("current_node").Default(""),
        field.String("lineage_id").Default(""),
        field.Text("error_message").Default(""),
        field.Time("started_at").Default(time.Now),
        field.Time("ended_at").Default(time.Time{}),
    }
}

func (GraphRun) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("graph", Graph.Type).Ref("runs").Field("graph_id").Unique().Required(),
        edge.To("checkpoints", GraphCheckpoint.Type),
    }
}
```

**`internal/data/ent/schema/graph_checkpoint.go`**

```go
type GraphCheckpoint struct {
    ent.Schema
}

func (GraphCheckpoint) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").DefaultFunc(uuid.NewString),
        field.String("run_id").NotEmpty(),
        field.String("lineage_id").NotEmpty(),
        field.String("namespace").Default(""),
        field.String("parent_checkpoint_id").Default(""),
        field.String("source").Default(""),
        field.Int("step").Default(-1),
        field.Text("state_json").Default("{}"),
        field.Text("next_nodes_json").Default("[]"),
        field.Text("interrupt_state_json").Default(""),
        field.Text("channel_values_json").Default("{}"),
        field.Text("channel_versions_json").Default("{}"),
        field.Text("versions_seen_json").Default("{}"),
        field.Text("pending_writes_json").Default("[]"),
        field.Text("metadata_json").Default("{}"),
        field.Time("created_at").Default(time.Now),
    }
}

func (GraphCheckpoint) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("run", GraphRun.Type).Ref("checkpoints").Field("run_id").Unique().Required(),
    }
}
```

### 5.2 Repo 实现

```go
// internal/data/graph.go

type graphRepo struct {
    data *Data
}

func NewGraphRepo(data *Data) biz.GraphRepo {
    return &graphRepo{data: data}
}

func (r *graphRepo) Save(ctx context.Context, g *biz.Graph) (*biz.Graph, error) {
    builder := r.data.db.Graph.Create().
        SetName(g.Name).
        SetDescription(g.Description).
        SetNodesJSON(g.NodesJSON).
        SetEdgesJSON(g.EdgesJSON).
        SetCondEdgesJSON(g.CondEdgesJSON).
        SetStateFieldsJSON(g.StateFieldsJSON).
        SetEntryPoint(g.EntryPoint).
        SetFinishPoint(g.FinishPoint).
        SetExecutionEngine(g.ExecutionEngine).
        SetMaxSteps(g.MaxSteps).
        SetMaxConcurrency(g.MaxConcurrency).
        SetVersion(g.Version)
    saved, err := builder.Save(ctx)
    return entGraphToBiz(saved), err
}

func (r *graphRepo) Get(ctx context.Context, id string) (*biz.Graph, error) {
    entG, err := r.data.db.Graph.Get(ctx, id)
    if err != nil {
        return nil, err
    }
    return entGraphToBiz(entG), nil
}

func (r *graphRepo) List(ctx context.Context, keyword string, page, pageSize int) ([]*biz.Graph, int, error) {
    query := r.data.db.Graph.Query()
    if keyword != "" {
        query = query.Where(graph.NameContains(keyword))
    }
    total, err := query.Count(ctx)
    if err != nil {
        return nil, 0, err
    }
    items, err := query.
        Offset((page - 1) * pageSize).
        Limit(pageSize).
        Order(ent.Asc(graph.FieldCreatedAt)).
        All(ctx)
    result := make([]*biz.Graph, len(items))
    for i, item := range items {
        result[i] = entGraphToBiz(item)
    }
    return result, total, nil
}

func (r *graphRepo) Update(ctx context.Context, g *biz.Graph) (*biz.Graph, error) {
    builder := r.data.db.Graph.UpdateOneID(g.ID).
        SetName(g.Name).
        SetDescription(g.Description).
        SetNodesJSON(g.NodesJSON).
        SetEdgesJSON(g.EdgesJSON).
        SetCondEdgesJSON(g.CondEdgesJSON).
        SetStateFieldsJSON(g.StateFieldsJSON).
        SetEntryPoint(g.EntryPoint).
        SetFinishPoint(g.FinishPoint).
        SetExecutionEngine(g.ExecutionEngine).
        SetMaxSteps(g.MaxSteps).
        SetMaxConcurrency(g.MaxConcurrency).
        SetVersion(g.Version)
    updated, err := builder.Save(ctx)
    return entGraphToBiz(updated), err
}

func (r *graphRepo) Delete(ctx context.Context, id string) error {
    return r.data.db.Graph.DeleteOneID(id).Exec(ctx)
}
```

```go
// internal/data/graph_run.go

type graphRunRepo struct {
    data *Data
}

func NewGraphRunRepo(data *Data) biz.GraphRunRepo {
    return &graphRunRepo{data: data}
}

func (r *graphRunRepo) Save(ctx context.Context, run *biz.GraphRun) (*biz.GraphRun, error) {
    builder := r.data.db.GraphRun.Create().
        SetGraphID(run.GraphID).
        SetSessionID(run.SessionID).
        SetStatus(run.Status).
        SetCurrentNode(run.CurrentNode).
        SetLineageID(run.LineageID).
        SetErrorMessage(run.ErrorMessage)
    if !run.StartedAt.IsZero() {
        builder.SetStartedAt(run.StartedAt)
    }
    saved, err := builder.Save(ctx)
    return entRunToBiz(saved), err
}

func (r *graphRunRepo) Get(ctx context.Context, id string) (*biz.GraphRun, error) {
    entR, err := r.data.db.GraphRun.Get(ctx, id)
    if err != nil {
        return nil, err
    }
    return entRunToBiz(entR), nil
}

func (r *graphRunRepo) ListByGraph(ctx context.Context, graphID string, page, pageSize int) ([]*biz.GraphRun, int, error) {
    query := r.data.db.GraphRun.Query().Where(graphrun.GraphID(graphID))
    total, err := query.Count(ctx)
    if err != nil {
        return nil, 0, err
    }
    items, err := query.
        Offset((page - 1) * pageSize).
        Limit(pageSize).
        Order(ent.Desc(graphrun.FieldStartedAt)).
        All(ctx)
    result := make([]*biz.GraphRun, len(items))
    for i, item := range items {
        result[i] = entRunToBiz(item)
    }
    return result, total, nil
}

func (r *graphRunRepo) Update(ctx context.Context, run *biz.GraphRun) (*biz.GraphRun, error) {
    builder := r.data.db.GraphRun.UpdateOneID(run.ID).
        SetStatus(run.Status).
        SetCurrentNode(run.CurrentNode).
        SetErrorMessage(run.ErrorMessage)
    if !run.EndedAt.IsZero() {
        builder.SetEndedAt(run.EndedAt)
    }
    updated, err := builder.Save(ctx)
    return entRunToBiz(updated), err
}
```

```go
// internal/data/graph_checkpoint.go

type graphCheckpointRepo struct {
    data *Data
}

func NewGraphCheckpointRepo(data *Data) biz.GraphCheckpointRepo {
    return &graphCheckpointRepo{data: data}
}

func (r *graphCheckpointRepo) Save(ctx context.Context, cp *biz.GraphCheckpoint) (*biz.GraphCheckpoint, error) {
    builder := r.data.db.GraphCheckpoint.Create().
        SetRunID(cp.RunID).
        SetLineageID(cp.LineageID).
        SetNamespace(cp.Namespace).
        SetParentCheckpointID(cp.ParentCheckpointID).
        SetSource(cp.Source).
        SetStep(cp.Step).
        SetStateJSON(cp.StateJSON).
        SetNextNodesJSON(cp.NextNodesJSON).
        SetInterruptStateJSON(cp.InterruptStateJSON).
        SetChannelValuesJSON(cp.ChannelValuesJSON).
        SetChannelVersionsJSON(cp.ChannelVersionsJSON).
        SetVersionsSeenJSON(cp.VersionsSeenJSON).
        SetPendingWritesJSON(cp.PendingWritesJSON).
        SetMetadataJSON(cp.MetadataJSON)
    saved, err := builder.Save(ctx)
    return entCheckpointToBiz(saved), err
}

func (r *graphCheckpointRepo) Get(ctx context.Context, runID, checkpointID string) (*biz.GraphCheckpoint, error) {
    entC, err := r.data.db.GraphCheckpoint.Query().
        Where(graphcheckpoint.ID(checkpointID), graphcheckpoint.RunID(runID)).
        Only(ctx)
    if err != nil {
        return nil, err
    }
    return entCheckpointToBiz(entC), nil
}

func (r *graphCheckpointRepo) ListByRun(ctx context.Context, runID string, limit int) ([]*biz.GraphCheckpoint, error) {
    query := r.data.db.GraphCheckpoint.Query().Where(graphcheckpoint.RunID(runID))
    if limit > 0 {
        query = query.Limit(limit)
    }
    items, err := query.Order(ent.Desc(graphcheckpoint.FieldCreatedAt)).All(ctx)
    result := make([]*biz.GraphCheckpoint, len(items))
    for i, item := range items {
        result[i] = entCheckpointToBiz(item)
    }
    return result, err
}

func (r *graphCheckpointRepo) ListByLineage(ctx context.Context, lineageID string, limit int) ([]*biz.GraphCheckpoint, error) {
    query := r.data.db.GraphCheckpoint.Query().Where(graphcheckpoint.LineageID(lineageID))
    if limit > 0 {
        query = query.Limit(limit)
    }
    items, err := query.Order(ent.Desc(graphcheckpoint.FieldCreatedAt)).All(ctx)
    result := make([]*biz.GraphCheckpoint, len(items))
    for i, item := range items {
        result[i] = entCheckpointToBiz(item)
    }
    return result, err
}

func (r *graphCheckpointRepo) DeleteByLineage(ctx context.Context, lineageID string) error {
    _, err := r.data.db.GraphCheckpoint.Delete().Where(graphcheckpoint.LineageID(lineageID)).Exec(ctx)
    return err
}
```

---

## 六、Service 层

```go
// internal/service/graph.go

type GraphService struct {
    v1.UnimplementedGraphServiceServer
    uc *biz.GraphUsecase
}

func NewGraphService(uc *biz.GraphUsecase) *GraphService {
    return &GraphService{uc: uc}
}

func (s *GraphService) CreateGraph(ctx context.Context, req *v1.CreateGraphRequest) (*v1.Graph, error) {
    g := fromProtoCreateGraphRequest(req)
    created, err := s.uc.CreateGraph(ctx, g)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoGraph(created), nil
}

func (s *GraphService) GetGraph(ctx context.Context, req *v1.GetGraphRequest) (*v1.Graph, error) {
    g, err := s.uc.GetGraph(ctx, req.Id)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoGraph(g), nil
}

func (s *GraphService) ListGraphs(ctx context.Context, req *v1.ListGraphsRequest) (*v1.ListGraphsResponse, error) {
    items, total, err := s.uc.ListGraphs(ctx, req.Keyword, int(req.Page), int(req.PageSize))
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return &v1.ListGraphsResponse{
        Items: toProtoGraphs(items),
        Total: int32(total),
    }, nil
}

func (s *GraphService) UpdateGraph(ctx context.Context, req *v1.UpdateGraphRequest) (*v1.Graph, error) {
    g := fromProtoUpdateGraphRequest(req)
    updated, err := s.uc.UpdateGraph(ctx, g)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoGraph(updated), nil
}

func (s *GraphService) DeleteGraph(ctx context.Context, req *v1.DeleteGraphRequest) (*emptypb.Empty, error) {
    if err := s.uc.DeleteGraph(ctx, req.Id); err != nil {
        return nil, kerrors.FromError(err)
    }
    return &emptypb.Empty{}, nil
}

func (s *GraphService) RunGraph(ctx context.Context, req *v1.RunGraphRequest) (*v1.RunGraphResponse, error) {
    var initialState map[string]any
    if req.InputJson != "" {
        json.Unmarshal([]byte(req.InputJson), &initialState)
    }
    var resumeValue map[string]any
    if req.ResumeMapJson != "" {
        json.Unmarshal([]byte(req.ResumeMapJson), &resumeValue)
    }
    run, err := s.uc.RunGraph(ctx, req.Id, req.SessionId, initialState, req.CheckpointId, resumeValue)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return &v1.RunGraphResponse{Run: toProtoGraphRun(run)}, nil
}

func (s *GraphService) GetGraphRun(ctx context.Context, req *v1.GetGraphRunRequest) (*v1.GraphRun, error) {
    run, err := s.uc.GetGraphRun(ctx, req.RunId)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoGraphRun(run), nil
}

func (s *GraphService) ListGraphRuns(ctx context.Context, req *v1.ListGraphRunsRequest) (*v1.ListGraphRunsResponse, error) {
    items, total, err := s.uc.ListGraphRuns(ctx, req.GraphId, int(req.Page), int(req.PageSize))
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return &v1.ListGraphRunsResponse{
        Items: toProtoGraphRuns(items),
        Total: int32(total),
    }, nil
}

func (s *GraphService) CancelGraphRun(ctx context.Context, req *v1.CancelGraphRunRequest) (*v1.CancelGraphRunResponse, error) {
    if err := s.uc.CancelGraphRun(ctx, req.RunId); err != nil {
        return nil, kerrors.FromError(err)
    }
    return &v1.CancelGraphRunResponse{}, nil
}

func (s *GraphService) GetCheckpoint(ctx context.Context, req *v1.GetCheckpointRequest) (*v1.Checkpoint, error) {
    cp, err := s.uc.GetCheckpoint(ctx, req.RunId, req.CheckpointId)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoCheckpoint(cp), nil
}

func (s *GraphService) ListCheckpoints(ctx context.Context, req *v1.ListCheckpointsRequest) (*v1.ListCheckpointsResponse, error) {
    items, err := s.uc.ListCheckpoints(ctx, req.RunId, req.LineageId, int(req.Limit))
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return &v1.ListCheckpointsResponse{Items: toProtoCheckpointInfos(items)}, nil
}

func (s *GraphService) ResumeFromCheckpoint(ctx context.Context, req *v1.ResumeFromCheckpointRequest) (*v1.ResumeFromCheckpointResponse, error) {
    var resumeValue map[string]any
    if req.ResumeMapJson != "" {
        json.Unmarshal([]byte(req.ResumeMapJson), &resumeValue)
    }
    run, err := s.uc.ResumeFromCheckpoint(ctx, req.RunId, req.CheckpointId, resumeValue)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return &v1.ResumeFromCheckpointResponse{Run: toProtoGraphRun(run)}, nil
}

func (s *GraphService) GetStateSnapshot(ctx context.Context, req *v1.GetStateSnapshotRequest) (*v1.StateSnapshot, error) {
    snapshot, err := s.uc.GetStateSnapshot(ctx, req.LineageId, req.CheckpointId, req.Namespace)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return toProtoStateSnapshot(snapshot), nil
}

func (s *GraphService) EditState(ctx context.Context, req *v1.EditStateRequest) (*v1.EditStateResponse, error) {
    var patch map[string]any
    if req.PatchJson != "" {
        json.Unmarshal([]byte(req.PatchJson), &patch)
    }
    ref, err := s.uc.EditState(ctx, req.LineageId, req.CheckpointId, req.Namespace, patch)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return &v1.EditStateResponse{NewCheckpoint: toProtoCheckpointRef(ref)}, nil
}

func (s *GraphService) GetGraphDot(ctx context.Context, req *v1.GetGraphDotRequest) (*v1.GetGraphDotResponse, error) {
    dot, err := s.uc.GetGraphDot(ctx, req.Id, req.RankDir, req.IncludeDestinations, req.IncludeStartEnd)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return &v1.GetGraphDotResponse{Dot: dot}, nil
}
```

---

## 七、Wire 注入

```go
// internal/data/data.go — ProviderSet 新增
var ProviderSet = wire.NewSet(
    // ... 已有
    NewGraphRepo,
    NewGraphRunRepo,
    NewGraphCheckpointRepo,
)

// internal/biz/biz.go — ProviderSet 新增
var ProviderSet = wire.NewSet(
    // ... 已有
    NewGraphUsecase,
)

// internal/service/service.go — ProviderSet 新增
var ProviderSet = wire.NewSet(
    // ... 已有
    NewGraphService,
)

// internal/graph/trpc/trpc.go — ProviderSet 新增
var ProviderSet = wire.NewSet(
    NewGraphBuilder,
    NewRunRegistry,
    NewSQLiteCheckpointSaver,
)

// cmd/admin/wire.go — 新增注入
wire.Build(
    // ... 已有
    graphTrpc.ProviderSet,
)
```

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/features/graph/
├── api.ts
├── types.ts
├── stores/
│   └── graphStore.ts
└── components/
    ├── GraphListPage.vue
    ├── GraphEditorPage.vue
    ├── GraphCanvas.vue
    ├── GraphNodeItem.vue
    ├── GraphEdgeItem.vue
    ├── GraphNodeConfigDialog.vue
    ├── GraphStateSchemaEditor.vue
    ├── GraphRunDetailPage.vue
    ├── GraphRunListPage.vue
    ├── CheckpointTimeline.vue
    ├── StateSnapshotViewer.vue
    └── GraphDotPreview.vue
```

### 8.2 类型定义

```typescript
// web/src/features/graph/types.ts

export interface StateFieldDef {
  name: string
  type: string
  reducer: string
  default_value?: string
  required: boolean
  disable_deep_copy: boolean
}

export interface NodeDef {
  id: string
  name: string
  type: 'function' | 'llm' | 'tool' | 'agent' | 'join' | 'router'
  description: string
  config_json?: string
  instruction?: string
  model_name?: string
  tool_names?: string[]
  agent_name?: string
  subgraph_id?: string
  input_mapper_json?: string
  output_mapper_json?: string
  isolated_messages?: boolean
  interrupt_before?: boolean
  interrupt_after?: boolean
  ends_json?: string
  destinations_json?: string
  cache_policy_json?: string
  retry_policy_json?: string
  stream_output_name?: string
}

export interface EdgeDef {
  from: string
  to: string
}

export interface ConditionalEdgeDef {
  from: string
  condition_expr: string
  path_map: Record<string, string>
}

export interface Graph {
  id: string
  name: string
  description: string
  nodes: NodeDef[]
  edges: EdgeDef[]
  conditional_edges: ConditionalEdgeDef[]
  state_fields: StateFieldDef[]
  entry_point: string
  finish_point: string
  execution_engine: 'bsp' | 'dag'
  max_steps: number
  max_concurrency: number
  version: string
  created_at: string
  updated_at: string
}

export interface GraphRun {
  id: string
  graph_id: string
  session_id: string
  status: 'running' | 'waiting_human' | 'completed' | 'failed' | 'cancelled'
  current_node: string
  lineage_id: string
  error_message: string
  started_at: string
  ended_at: string
}

export interface Checkpoint {
  id: string
  run_id: string
  lineage_id: string
  namespace: string
  parent_checkpoint_id: string
  source: string
  step: number
  state_json: string
  next_nodes_json: string
  interrupt_state_json: string
  created_at: string
}

export interface CheckpointInfo {
  lineage_id: string
  namespace: string
  checkpoint_id: string
  parent_checkpoint: string
  source: string
  step: number
  timestamp: string
}

export interface StateSnapshot {
  checkpoint_info: CheckpointInfo
  state_json: string
  next_nodes_json: string
  next_channels_json: string
}

export interface CheckpointRef {
  lineage_id: string
  namespace: string
  checkpoint_id: string
}
```

### 8.3 API

```typescript
// web/src/features/graph/api.ts

import axios from 'axios'
import type {
  Graph, GraphRun, Checkpoint, CheckpointInfo,
  StateSnapshot, CheckpointRef
} from './types'

const BASE = '/api/v1/graphs'

export async function listGraphs(params: {
  page?: number
  page_size?: number
  keyword?: string
}): Promise<{ items: Graph[]; total: number }> {
  const { data } = await axios.get(BASE, { params })
  return data
}

export async function createGraph(req: Partial<Graph>): Promise<Graph> {
  const { data } = await axios.post(BASE, req)
  return data
}

export async function getGraph(id: string): Promise<Graph> {
  const { data } = await axios.get(`${BASE}/${id}`)
  return data
}

export async function updateGraph(id: string, req: Partial<Graph>): Promise<Graph> {
  const { data } = await axios.put(`${BASE}/${id}`, req)
  return data
}

export async function deleteGraph(id: string): Promise<void> {
  await axios.delete(`${BASE}/${id}`)
}

export async function runGraph(id: string, req: {
  session_id?: string
  input_json?: string
  checkpoint_id?: string
  resume_value_json?: string
  resume_map_json?: string
}): Promise<{ run: GraphRun }> {
  const { data } = await axios.post(`${BASE}/${id}/run`, req)
  return data
}

export async function getGraphRun(graphId: string, runId: string): Promise<GraphRun> {
  const { data } = await axios.get(`${BASE}/${graphId}/runs/${runId}`)
  return data
}

export async function listGraphRuns(graphId: string, params: {
  page?: number
  page_size?: number
}): Promise<{ items: GraphRun[]; total: number }> {
  const { data } = await axios.get(`${BASE}/${graphId}/runs`, { params })
  return data
}

export async function cancelGraphRun(graphId: string, runId: string): Promise<void> {
  await axios.post(`${BASE}/${graphId}/runs/${runId}/cancel`)
}

export async function getCheckpoint(runId: string, checkpointId: string): Promise<Checkpoint> {
  const { data } = await axios.get(`${BASE}/runs/${runId}/checkpoints/${checkpointId}`)
  return data
}

export async function listCheckpoints(runId: string, params: {
  lineage_id?: string
  limit?: number
}): Promise<{ items: CheckpointInfo[] }> {
  const { data } = await axios.get(`${BASE}/runs/${runId}/checkpoints`, { params })
  return data
}

export async function resumeFromCheckpoint(
  runId: string,
  checkpointId: string,
  req: { resume_value_json?: string; resume_map_json?: string }
): Promise<{ run: GraphRun }> {
  const { data } = await axios.post(
    `${BASE}/runs/${runId}/checkpoints/${checkpointId}/resume`,
    req
  )
  return data
}

export async function getStateSnapshot(params: {
  lineage_id: string
  checkpoint_id?: string
  namespace?: string
}): Promise<StateSnapshot> {
  const { data } = await axios.get(`${BASE}/state-snapshot`, { params })
  return data
}

export async function editState(req: {
  lineage_id: string
  checkpoint_id: string
  namespace?: string
  patch_json: string
}): Promise<{ new_checkpoint: CheckpointRef }> {
  const { data } = await axios.post(`${BASE}/state-edit`, req)
  return data
}

export async function getGraphDot(id: string, params: {
  rank_dir?: string
  include_destinations?: boolean
  include_start_end?: boolean
}): Promise<{ dot: string }> {
  const { data } = await axios.get(`${BASE}/${id}/dot`, { params })
  return data
}
```

### 8.4 组件设计

**GraphListPage.vue**：Graph 列表页

| 区域 | 组件 | 功能 |
|------|------|------|
| 顶部 | `QBtn` | 新建 Graph |
| 搜索 | `QInput` | 关键词搜索 |
| 列表 | `QTable` | 名称/引擎/节点数/创建时间/操作 |
| 操作 | `QBtn` | 编辑/删除/运行/DOT 预览 |

**GraphEditorPage.vue**：Graph 编辑器主页面

| 区域 | 组件 | 功能 |
|------|------|------|
| 左侧 | `GraphCanvas` | 节点/边可视化画布 |
| 右侧 | `QTab` 面板 | 节点配置/State Schema/运行配置 |
| 工具栏 | `QBtnGroup` | 添加节点/添加边/保存/运行/导出 DOT |
| 底部 | `QBanner` | 验证错误提示 |

**GraphCanvas.vue**：可视化画布（基于 `@vue-flow/core`）

| 功能 | 实现方式 | 说明 |
|------|----------|------|
| 节点渲染 | `@vue-flow/core` CustomNode | 不同类型不同样式：LLM 蓝色、Tool 橙色、Agent 绿色、Router 灰色菱形 |
| 边渲染 | `@vue-flow/core` CustomEdge | 实线=普通边、虚线=条件边 |
| 拖拽创建 | `@vue-flow/core` DnD | 从侧边栏拖入节点 |
| 连线 | `@vue-flow/core` Handle | 从端口拖拽创建边 |
| 条件路由 | 双击边编辑 | 弹出条件表达式和 PathMap 编辑器 |
| 子图 | 双击 Agent 节点 | 进入子图编辑模式 |
| 中断点 | 节点右键菜单 | 设置 interruptBefore/interruptAfter |
| 入口/出口 | 右键菜单 | 设置 entry/finish 节点 |
| 缩放/平移 | `@vue-flow/core` 内置 | 鼠标滚轮缩放、拖拽平移 |

**GraphNodeItem.vue**：节点渲染组件

| 属性 | 说明 |
|------|------|
| 样式 | 根据 NodeType 显示不同颜色和形状 |
| 标签 | 显示节点 ID + Name |
| 端口 | 左侧输入 Handle、右侧输出 Handle |
| 图标 | LLM=🤖、Tool=🔧、Agent=📦、Router=🔀 |
| 徽章 | 有中断点显示 ⏸ 标记 |

**GraphNodeConfigDialog.vue**：节点配置对话框

| Tab | 字段 | 适用类型 |
|-----|------|----------|
| 基本 | ID、名称、描述、类型 | 全部 |
| LLM | 指令、模型选择 | LLM |
| 工具 | 工具列表选择 | Tool |
| 子图 | 子图选择、输入/输出映射 | Agent |
| 中断 | interruptBefore、interruptAfter | 全部 |
| 出口 | ends 映射编辑 | Router/有条件出口 |
| 高级 | 缓存策略、重试策略、流式输出 | 全部 |

**GraphStateSchemaEditor.vue**：State Schema 编辑器

| 功能 | 说明 |
|------|------|
| 字段列表 | 表格显示 name/type/reducer/required |
| 添加字段 | 弹出对话框填写字段定义 |
| Reducer 选择 | 下拉：default/append/merge/cover/message |
| 类型选择 | 下拉：string/integer/float/bool/[]string/[]any/map/messages |
| 默认值 | JSON 编辑器 |
| 拖拽排序 | 调整字段顺序 |

**GraphRunDetailPage.vue**：运行详情页

| 区域 | 组件 | 功能 |
|------|------|------|
| 顶部 | `QBadge` | 状态徽章（running=蓝/waiting_human=黄/completed=绿/failed=红） |
| 画布 | `GraphCanvas`（只读） | 高亮当前执行节点 |
| 时间轴 | `CheckpointTimeline` | 检查点时间轴 |
| 状态 | `StateSnapshotViewer` | 当前状态查看 |
| 操作 | `QBtn` | 取消运行/恢复执行（HITL 时显示） |

**CheckpointTimeline.vue**：检查点时间轴

| 功能 | 说明 |
|------|------|
| 时间轴 | 纵向时间线，每个检查点一个节点 |
| 标签 | 步骤号 + 来源 + 时间 |
| 颜色 | input=蓝/loop=绿/interrupt=黄/update=紫/fork=橙 |
| 点击 | 展开 StateSnapshotViewer |
| 恢复 | "从此恢复"按钮 |
| 编辑 | "编辑状态"按钮（时间旅行） |
| 分支 | 显示 fork 分支关系 |

**StateSnapshotViewer.vue**：状态快照查看器

| 功能 | 说明 |
|------|------|
| JSON 树 | `vue-json-pretty` 展示状态 JSON |
| 搜索 | 关键词过滤 |
| 编辑 | 时间旅行模式下可编辑 patch |
| 差异 | 高亮与上一检查点的差异 |

**GraphDotPreview.vue**：DOT 可视化预览

| 功能 | 说明 |
|------|------|
| 渲染 | 调用后端 DOT 接口，使用 `d3-graphviz` 或 `viz.js` 渲染 |
| 布局 | LR/TB 切换 |
| 导出 | 导出 PNG/SVG |

### 8.5 Pinia Store

```typescript
// web/src/features/graph/stores/graphStore.ts

import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { Graph, GraphRun, CheckpointInfo, StateSnapshot } from '../types'
import * as api from '../api'

export const useGraphStore = defineStore('graph', () => {
  const graphs = ref<Graph[]>([])
  const currentGraph = ref<Graph | null>(null)
  const runs = ref<GraphRun[]>([])
  const currentRun = ref<GraphRun | null>(null)
  const checkpoints = ref<CheckpointInfo[]>([])
  const stateSnapshot = ref<StateSnapshot | null>(null)
  const loading = ref(false)

  async function fetchGraphs(keyword?: string) {
    loading.value = true
    try {
      const result = await api.listGraphs({ keyword })
      graphs.value = result.items
    } finally {
      loading.value = false
    }
  }

  async function fetchGraph(id: string) {
    loading.value = true
    try {
      currentGraph.value = await api.getGraph(id)
    } finally {
      loading.value = false
    }
  }

  async function saveGraph(graph: Partial<Graph>) {
    if (graph.id) {
      currentGraph.value = await api.updateGraph(graph.id, graph)
    } else {
      currentGraph.value = await api.createGraph(graph)
    }
  }

  async function executeGraph(id: string, sessionId?: string, inputJson?: string) {
    const result = await api.runGraph(id, {
      session_id: sessionId,
      input_json: inputJson
    })
    currentRun.value = result.run
    return result.run
  }

  async function fetchRuns(graphId: string) {
    const result = await api.listGraphRuns(graphId, {})
    runs.value = result.items
  }

  async function fetchCheckpoints(runId: string, lineageId?: string) {
    const result = await api.listCheckpoints(runId, { lineage_id: lineageId })
    checkpoints.value = result.items
  }

  async function fetchStateSnapshot(lineageId: string, checkpointId?: string, namespace?: string) {
    stateSnapshot.value = await api.getStateSnapshot({
      lineage_id: lineageId,
      checkpoint_id: checkpointId,
      namespace: namespace
    })
  }

  async function resumeFromCheckpoint(runId: string, checkpointId: string, resumeMapJson: string) {
    const result = await api.resumeFromCheckpoint(runId, checkpointId, {
      resume_map_json: resumeMapJson
    })
    currentRun.value = result.run
    return result.run
  }

  async function editState(lineageId: string, checkpointId: string, patchJson: string, namespace?: string) {
    const result = await api.editState({
      lineage_id: lineageId,
      checkpoint_id: checkpointId,
      namespace: namespace,
      patch_json: patchJson
    })
    return result.new_checkpoint
  }

  return {
    graphs, currentGraph, runs, currentRun, checkpoints, stateSnapshot, loading,
    fetchGraphs, fetchGraph, saveGraph, executeGraph, fetchRuns,
    fetchCheckpoints, fetchStateSnapshot, resumeFromCheckpoint, editState
  }
})
```

---

## 九、实现阶段

### Phase 1：基础增强（State Schema + 条件路由 + API）

1. 增强 `internal/graph/trpc/builder.go`，支持 `StateFieldDef` 和 `BuildConfig`
2. 创建 `api/kratos/graph/v1/graph.proto`，生成 Go 代码
3. 创建 Ent Schema：`graph.go`、`graph_run.go`、`graph_checkpoint.go`
4. 实现 Data 层 Repo
5. 实现 Biz 层 Usecase
6. 实现 Service 层
7. 注册 Wire 注入
8. 前端 GraphListPage + GraphEditorPage 基础版

### Phase 2：HITL + Checkpoint 持久化

1. 实现 `SQLiteCheckpointSaver`（`internal/graph/trpc/checkpoint.go`）
2. 实现 HITL 中断/恢复（`internal/graph/trpc/hitl.go`）
3. 实现 `RunRegistry`（`internal/graph/trpc/registry.go`）
4. 增强 Usecase 支持 RunGraph/ResumeFromCheckpoint
5. 前端 CheckpointTimeline + StateSnapshotViewer

### Phase 3：时间旅行 + 子图 + DAG

1. 实现 `TimeTravelService`（`internal/graph/trpc/timetravel.go`）
2. 实现子图嵌套（`internal/graph/trpc/subgraph.go`）
3. 支持 DAG 执行引擎配置
4. 实现可视化导出（`internal/graph/trpc/visualize.go`）
5. 前端 GraphDotPreview + 时间旅行编辑

### Phase 4：可视化编辑器（超越层）

1. 集成 `@vue-flow/core` 实现 GraphCanvas
2. 实现节点拖拽创建、连线、条件路由编辑
3. 实现节点配置对话框
4. 实现 State Schema 编辑器
5. 实现运行时节点高亮和状态展示
