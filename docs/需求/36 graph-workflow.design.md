# Graph 工作流模块 — 实现设计文档

> 对应需求：`36 graph-workflow.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Graph 工作流引擎：节点/边/条件路由、State Schema + Reducer、HITL 中断/恢复、Checkpoint 持久化、时间旅行调试、子图嵌套。对标 trpc-agent-go `graph` 包。

---

## 二、Proto 层

### 2.1 待新增

```protobuf
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
    option (google.api.http) = { patch: "/v1/graphs/{id}" body: "*" };
  }
  rpc DeleteGraph(DeleteGraphRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/graphs/{id}" };
  }
  rpc RunGraph(RunGraphRequest) returns (GraphRun) {
    option (google.api.http) = { post: "/v1/graphs/{id}/run" body: "*" };
  }
  rpc GetGraphRun(GetGraphRunRequest) returns (GraphRun) {
    option (google.api.http) = { get: "/v1/graphs/{id}/runs/{run_id}" };
  }
  rpc GetCheckpoint(GetCheckpointRequest) returns (Checkpoint) {
    option (google.api.http) = { get: "/v1/graphs/runs/{run_id}/checkpoints/{checkpoint_id}" };
  }
  rpc ResumeFromCheckpoint(ResumeFromCheckpointRequest) returns (GraphRun) {
    option (google.api.http) = { post: "/v1/graphs/runs/{run_id}/checkpoints/{checkpoint_id}/resume" body: "*" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type Graph struct {
    ID          string
    Name        string
    Description string
    NodesJSON   string  // 节点定义
    EdgesJSON   string  // 边定义
    StateSchema string  // State Schema
    EntryNode   string
    ExitNode    string
    CreatedAt   string
    UpdatedAt   string
}

type GraphNode struct {
    ID       string
    Type     string  // "agent"/"tool"/"condition"/"subgraph"/"input"/"output"
    Config   map[string]interface{}
    Label    string
}

type GraphEdge struct {
    From    string
    To      string
    CondExpr string  // 条件表达式（可选）
    Label   string
}

type GraphRun struct {
    ID          string
    GraphID     string
    SessionID   string
    Status      string  // "running"/"waiting_human"/"completed"/"failed"/"cancelled"
    CurrentNode string
    Checkpoints []Checkpoint
    StartedAt   string
    EndedAt     string
}

type Checkpoint struct {
    ID        string
    RunID     string
    NodeID    string
    StateJSON string
    CreatedAt string
}
```

### 3.2 Usecase

```go
func (uc *GraphUsecase) Create(ctx, g Graph) (Graph, error)
func (uc *GraphUsecase) Run(ctx, graphID, sessionID string, input map[string]interface{}) (GraphRun, error)
func (uc *GraphUsecase) GetRun(ctx, runID string) (GraphRun, error)
func (uc *GraphUsecase) GetCheckpoint(ctx, runID, checkpointID string) (Checkpoint, error)
func (uc *GraphUsecase) ResumeFromCheckpoint(ctx, runID, checkpointID string, humanInput map[string]interface{}) (GraphRun, error)
```

---

## 四、运行时层

### 4.1 Graph 构建

```go
// internal/graph/trpc/builder.go（已有，需增强）
func BuildStateGraph(ctx, cfg GraphBuildConfig) (*trpcgraph.StateGraph, error)
```

增强：
- State Schema + Reducer
- 条件路由完整支持
- HITL 中断/恢复
- 子图嵌套

### 4.2 Checkpoint 持久化

```go
// internal/graph/checkpoint.go
type CheckpointStore interface {
    Save(ctx, cp Checkpoint) error
    Load(ctx, runID, checkpointID string) (Checkpoint, error)
    List(ctx, runID string) ([]Checkpoint, error)
}
```

### 4.3 时间旅行调试

```go
// internal/graph/timetravel.go
func ReplayFromCheckpoint(ctx, cp Checkpoint) (*GraphRun, error)
```

---

## 五、Data 层

### 5.1 Ent Schema

- `internal/data/ent/schema/graph.go`
- `internal/data/ent/schema/graph_run.go`
- `internal/data/ent/schema/graph_checkpoint.go`

---

## 六、Service 层

```go
func (s *GraphService) CreateGraph(ctx, req) (*Graph, error)
func (s *GraphService) RunGraph(ctx, req) (*GraphRun, error)
func (s *GraphService) GetGraphRun(ctx, req) (*GraphRun, error)
func (s *GraphService) GetCheckpoint(ctx, req) (*Checkpoint, error)
func (s *GraphService) ResumeFromCheckpoint(ctx, req) (*GraphRun, error)
```

---

## 七、Wire 注入

待新增：
```
data.ProviderSet → NewGraphRepo, NewCheckpointStore
biz.ProviderSet → NewGraphUsecase
service.ProviderSet → NewGraphService
```

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/features/graph/
├── api.ts
├── types.ts
└── components/
    ├── GraphListPage.vue
    ├── GraphEditorPage.vue
    ├── GraphCanvas.vue         ← 画布（节点/边拖拽）
    ├── GraphNodeItem.vue
    ├── GraphEdgeItem.vue
    ├── GraphRunDetailPage.vue
    └── CheckpointTimeline.vue  ← 时间旅行
```

### 8.2 组件设计

**GraphCanvas.vue**：可视化画布

| 功能 | 实现 | 说明 |
|------|------|------|
| 节点拖拽 | `@vue-flow/core` | 拖拽创建/移动 |
| 连线 | `@vue-flow/core` | 从端口拖拽连线 |
| 条件路由 | 双击边编辑条件 | 条件表达式 |
| 子图 | 节点双击进入 | 子图嵌套 |
| 运行 | 工具栏按钮 | 执行工作流 |

**CheckpointTimeline.vue**：检查点时间轴

| 功能 | 说明 |
|------|------|
| 时间轴 | 纵向展示检查点 |
| 状态快照 | 点击查看 State |
| 恢复 | 从检查点恢复运行 |

### 8.3 API

```typescript
export async function listGraphs(query: GraphQuery): Promise<GraphListResult>
export async function createGraph(req: CreateGraphRequest): Promise<Graph>
export async function getGraph(id: string): Promise<Graph>
export async function updateGraph(id: string, req: UpdateGraphRequest): Promise<Graph>
export async function runGraph(id: string, req: RunGraphRequest): Promise<GraphRun>
export async function getGraphRun(graphId: string, runId: string): Promise<GraphRun>
export async function getCheckpoint(runId: string, checkpointId: string): Promise<Checkpoint>
export async function resumeFromCheckpoint(runId: string, checkpointId: string, input: any): Promise<GraphRun>
```
