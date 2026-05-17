# Memory L0-L4 模块 — 实现设计文档

> 对应需求：`12 memory-L0-sensory.md` / `13 memory-L1-working.md` / `14 memory-L2-episodic.md` / `15 memory-L3-semantic.md` / `16 memory-L4-persistent.md` / `31 memery.md` / `38 memory.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

五层记忆架构：L0 感官（最近对话）、L1 工作（结构化状态）、L2 情景（重要事件）、L3 语义（向量知识）、L4 持久（身份图谱）。L0-L2/L4 通过 `sessionmemory.Store` 存储在 SQLite，L3 通过 pgvector 存储。

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

### 2.2 已新增（2026-05-17 校准）

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

---

## 五、运行时层

### 5.1 Runner MemoryService

```go
// internal/agent/adksvc 中桥接
func NewADKMemoryService(store *sessionmemory.Store) session.Service
```

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
```

---

## 七、Wire 注入

已有：
```
data.ProviderSet → NewSessionMemoryStore
biz.ProviderSet → (MemoryUsecase 待创建)
service.ProviderSet → NewMemoryService
```

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/features/memory/
├── api.ts
├── types.ts
├── wireJson.ts
├── memoryEndpoints.ts
├── MemoryHero.vue              ← 五层记忆总览
├── MemoryMetricCards.vue       ← 指标卡片
├── MemoryOverviewPanel.vue     ← L0-L4 概览
├── MemorySessionsPanel.vue     ← 会话记忆
├── MemoryEvolutionPanel.vue    ← 演化历史
├── MemoryKnowledgePanel.vue    ← L3 知识库
├── MemoryFactDrawer.vue        ← 事实抽屉
├── MemorySnapshotDrawer.vue    ← 快照抽屉
└── MemorySettingsStatusPanel.vue ← 设置状态
```

### 8.2 组件设计

**MemoryHero.vue**：五层同心圆可视化，每层启用/禁用状态

**MemoryOverviewPanel.vue**：各层记忆数量、最近活动

**MemoryKnowledgePanel.vue**：L3 向量搜索结果展示

### 8.3 API

```typescript
export async function listMemories(query: MemoryListQuery): Promise<MemoryListResult>
export async function recallMemories(req: RecallRequest): Promise<Memory[]>
export async function getMemoryOverview(agentId: string): Promise<MemoryOverview>
export async function searchSemanticMemory(req: SemanticSearchRequest): Promise<Memory[]>
```
