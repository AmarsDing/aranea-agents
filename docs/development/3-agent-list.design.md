# Agent 列表模块 — 实现设计文档

> 对应需求：`3 agent-list.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 管理主列表页，支持搜索、筛选（关键字/状态/Provider/业务分类/创建者/归属类型）、网格/列表视图切换、三组分组（Built-in / Preset / User）+ 拖拽排序、收藏、删除、复制、迁移入口。

后端复用 `AgentService` 下的 `ListAgents` / `DeleteAgent` / `ToggleFavorite` / `DuplicateAgent` / `ListAgentCreators` RPC，并通过 `ListExtrasForAgents` 富化运行态字段（`last_run_status` / `last_run_at` / `pending_evolution_count`）。

`BatchUpdateAgents` 已实现并通过 proto RPC 暴露（`POST /v1/agents:batchUpdate`，LIST-04 批量启用/停用/删除）；`ReorderAgents` 仍为 biz stub，尚无 proto RPC 暴露。

---

## 二、Proto 层

文件：`api/kratos/agent/v1/agent.proto`

### 2.1 Agent 消息（列表项相关字段）

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
  string position_id = 11;            // 业务分类职位节点 id
  string system_prompt_mode = 12;
  int32 context_window = 13;
  int32 budget_monthly_cents = 14;
  string config_json = 15;
  string created_at = 16;
  string updated_at = 17;
  string deleted_at = 18;
  AgentRuntimeSettings settings = 19;
  repeated AgentPromptFile files = 20;
  string agent_kind = 21;             // llm | a2a_proxy
  A2AProxyConfig a2a_proxy_config = 22;
  bool a2a_endpoint_enabled = 23;
  string last_run_status = 24;        // 列表富化：最近会话运行状态
  string last_run_at = 25;
  int32 pending_evolution_count = 26;
  string created_by = 27;             // 创建者用户 id
  bool readonly = 28;                 // 系统内置不可删
  string position_key = 29;
  string agent_variant = 30;
  string variant_description = 31;
  string source = 32;                 // user | system | imported
  string kind = 33;                   // user | system_builtin | ecosystem_preset | marketplace | certified
}
```

### 2.2 列表/删除/收藏/复制/创建者 RPC

```protobuf
message ListAgentsRequest {
  string keyword = 1;
  string status = 2;
  string provider = 3;
  string org_node_id = 4;             // 业务分类职位节点 id
  int32 limit = 5;
  int32 offset = 6;
  string created_by = 7;              // 空 = 全部；"mine" = 当前用户；否则用户 id
}

message ListAgentsResponse {
  repeated Agent items = 1;
  int32 total = 2;
  int32 limit = 3;
  int32 offset = 4;
}

message DeleteAgentRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ToggleFavoriteRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message DuplicateAgentRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message AgentCreator {
  string user_id = 1;
  string label = 2;
}

message ListAgentCreatorsResponse {
  repeated AgentCreator items = 1;
}

service AgentService {
  rpc ListAgents(ListAgentsRequest) returns (ListAgentsResponse) {
    option (google.api.http) = {get: "/v1/agents"};
  }
  rpc DeleteAgent(DeleteAgentRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {delete: "/v1/agents/{id}"};
  }
  rpc ToggleFavorite(ToggleFavoriteRequest) returns (Agent) {
    option (google.api.http) = { patch: "/v1/agents/{id}/favorite" body: "*" };
  }
  rpc DuplicateAgent(DuplicateAgentRequest) returns (Agent) {
    option (google.api.http) = { post: "/v1/agents/{id}/duplicate" body: "*" };
  }
  rpc ListAgentCreators(google.protobuf.Empty) returns (ListAgentCreatorsResponse) {
    option (google.api.http) = {get: "/v1/agents/creators"};
  }
  rpc BatchUpdateAgents(BatchUpdateAgentsRequest) returns (BatchUpdateAgentsResponse) {
    option (google.api.http) = {post: "/v1/agents:batchUpdate" body: "*"};
  }
}
```

`BatchUpdateAgentsRequest`：`ids`（必填）+ `status`（active/inactive）与 `delete` 互斥、必须恰设其一；`BatchUpdateAgentsResponse` 返回 `affected` 条数。

### 2.3 消息字段说明

| 消息 | 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `ListAgentsRequest` | `keyword` | string | ❌ | 搜索关键字（命中 display_name/agent_key/provider/model/agent_description） |
| | `status` | string | ❌ | 状态筛选（active/inactive/deleted） |
| | `provider` | string | ❌ | Provider 筛选 |
| | `org_node_id` | string | ❌ | 业务分类职位节点 id 筛选 |
| | `created_by` | string | ❌ | 空 = 全部；`mine` = 当前用户；否则用户 id |
| | `limit` | int32 | ❌ | 每页条数，默认 24，最大 100 |
| | `offset` | int32 | ❌ | 偏移量，默认 0 |
| `ListAgentsResponse` | `items` | Agent[] | — | Agent 列表（列表页不含 settings/files 水合，但含运行态富化字段） |
| | `total` | int32 | — | 总条数 |
| | `limit` | int32 | — | 实际 limit |
| | `offset` | int32 | — | 实际 offset |
| `ToggleFavoriteRequest` | `id` | string | ✅ | Agent ID |
| `DuplicateAgentRequest` | `id` | string | ✅ | Agent ID |

---

## 三、Biz 层

### 3.1 领域模型

文件：`internal/biz/agent_types.go`

```go
type Agent struct {
    ID                  string
    AgentKey            string
    DisplayName         string
    Provider            string
    Model               string
    Status              string
    IsDefault           *bool  // nil = not set; explicit true/false for merge
    IsFavorite          *bool
    Icon                string
    AgentDescription    string
    PositionID          string
    PositionKey         string
    AgentVariant        string
    VariantDescription  string
    SystemPromptMode    string
    ContextWindow       int
    BudgetMonthlyCents  int
    ConfigJSON          string
    MetadataJSON        string
    Roles               []string
    Kind                string // user | system_builtin | ecosystem_preset | marketplace | certified
    AgentKind           string // llm | a2a_proxy
    A2AProxy            *A2AProxyConfig
    A2AEndpointEnabled  bool
    LastRunStatus       string
    LastRunAt           string
    PendingEvolutionCount int
    CreatedBy           string
    Readonly            bool
    Source              string // user | system | imported
    CreatedAt           string
    UpdatedAt           string
    DeletedAt           string
    Settings            *AgentRuntimeSettings
    Files               []AgentPromptFile
}

type AgentListQuery struct {
    Keyword   string
    Status    string
    Provider  string
    OrgNodeID string
    CreatedBy string
    Role      string
    Kind      string // user | system_builtin | ecosystem_preset | marketplace | certified
    Limit     int
    Offset    int
}

type AgentListResult struct {
    Items  []Agent
    Total  int
    Limit  int
    Offset int
}

// AgentListExtras 是列表行富化字段（不持久化在 agents 表）
type AgentListExtras struct {
    LastRunStatus         string
    LastRunAt             string
    PendingEvolutionCount int
}

type AgentCreator struct {
    UserID string
    Label  string
}
```

### 3.2 Usecase 方法

文件：`internal/biz/agent_usecase.go`、`internal/biz/agent_duplicate.go`

```go
func (u *AgentUsecase) List(ctx context.Context, q AgentListQuery) (AgentListResult, error)
func (u *AgentUsecase) Delete(ctx context.Context, id string) error
func (u *AgentUsecase) ToggleFavorite(ctx context.Context, id string) (Agent, error)
func (u *AgentUsecase) Duplicate(ctx context.Context, id string) (Agent, error)  // agent_duplicate.go
func (u *AgentUsecase) ListAgentCreators(ctx context.Context) ([]AgentCreator, error)
func (u *AgentUsecase) BatchUpdateAgents(ctx context.Context, in AgentBatchUpdateInput) (int, error)
func (u *AgentUsecase) ReorderAgents(ctx context.Context, ids []string) error  // 当前 stub
```

`List` 内部调用 `reader.SearchAgents` 取分页结果，再用 `reader.ListExtrasForAgents` 批量富化 `LastRunStatus` / `LastRunAt` / `PendingEvolutionCount`。

### 3.3 创建者筛选解析

文件：`internal/biz/agent_context.go`

```go
const AgentListCreatedByMine = "mine"

func AgentCreatedByFromContext(ctx context.Context) string
func ResolveListCreatedByFilter(ctx context.Context, filter string) string
```

`ResolveListCreatedByFilter` 将 `"mine"` 解析为当前 ctx 用户 id，空字符串保持空（全部），其他值透传。

### 3.4 归属类型归一化

文件：`internal/biz/agent_kind.go`

```go
func NormalizeAgentKind(raw string) string
func IsA2AProxyAgent(ag Agent) bool
func HydrateAgentKind(a *Agent)
```

### 3.5 Repo 接口（窄接口）

文件：`internal/biz/agent_usecase.go`

```go
// Stability:stable
type AgentReader interface {
    SearchAgents(ctx context.Context, q AgentListQuery) (AgentListResult, error)
    GetAgentByID(ctx context.Context, id string) (Agent, error)
    GetAgentByAgentKey(ctx context.Context, agentKey string) (Agent, error)
    ListExtrasForAgents(ctx context.Context, agentIDs []string) (map[string]AgentListExtras, error)
}

// Stability:stable
type AgentWriter interface {
    CreateAgent(ctx context.Context, a Agent) (Agent, error)
    UpdateAgent(ctx context.Context, a Agent) (Agent, error)
    DeleteAgent(ctx context.Context, id string) error
    ToggleFavorite(ctx context.Context, id string) (Agent, error)
}

// Stability:stable
type AgentPositionRepo interface {
    ListAgentCreators(ctx context.Context) ([]AgentCreator, error)
    ReorderAgents(ctx context.Context, ids []string) error
    ClearPositionByDepartment(ctx context.Context, deptID string) (int, error)
}
```

---

## 四、Data 层

### 4.1 SearchAgents 实现

文件：`internal/data/agent_repo.go`

```go
func (r *agentRepo) SearchAgents(ctx context.Context, q biz.AgentListQuery) (biz.AgentListResult, error) {
    if q.Limit <= 0 { q.Limit = 24 }
    if q.Limit > 100 { q.Limit = 100 }
    if q.Offset < 0 { q.Offset = 0 }

    preds := []predicate.Agent{agent.DeletedAtEQ("")}
    if kw := strings.TrimSpace(q.Keyword); kw != "" {
        preds = append(preds, agent.Or(
            agent.AgentKeyContainsFold(kw),
            agent.DisplayNameContainsFold(kw),
            agent.ProviderContainsFold(kw),
            agent.ModelContainsFold(kw),
            agent.AgentDescriptionContainsFold(kw),
        ))
    }
    if q.Status != ""   { preds = append(preds, agent.StatusEQ(q.Status)) }
    if q.Provider != "" { preds = append(preds, agent.ProviderEQ(q.Provider)) }
    if q.OrgNodeID != "" {
        // 解析职位子树，匹配 position_id IN (...)
        positionIDs, err := r.categoryPositionIDsForFilter(ctx, q.OrgNodeID)
        // ...
    }
    if cb := strings.TrimSpace(q.CreatedBy); cb != "" {
        preds = append(preds, agent.CreatedByEQ(cb))
    }
    if role := strings.TrimSpace(q.Role); role != "" {
        preds = append(preds, agent.RolesJSONContains(role))
    }
    if q.Kind != "" { preds = append(preds, agent.KindEQ(agent.Kind(q.Kind))) }

    where := agent.And(preds...)
    c := r.data.RW().Read(ctx)  // 读写分离：读走 readClient
    total, err := c.Agent.Query().Where(where).Count(ctx)
    // ...
    rows, err := c.Agent.Query().Where(where).
        Order(
            agent.ByIsDefault(entsql.OrderDesc()),
            // 内置管家优先：system_builtin 且非 dept_lead（精灵/系统/记忆/技能管家）排最前
            func(s *entsql.Selector) {
                s.OrderBy(entsql.Asc("CASE WHEN " + s.C(agent.FieldKind) + " = 'system_builtin' AND " + s.C(agent.FieldAgentVariant) + " <> 'dept_lead' THEN 0 ELSE 1 END"))
            },
            agent.ByKind(entsql.OrderDesc()),
            agent.ByUpdatedAt(entsql.OrderDesc()),
            agent.ByID(entsql.OrderAsc()), // 唯一决胜键：同 updated_at 组内分页稳定
        ).
        Limit(q.Limit).Offset(q.Offset).All(ctx)
    // ...
}
```

要点：
- 使用 `r.data.RW().Read(ctx)` 走读连接（WAL 并发读），符合 DB 读写分离规范
- 排序层级：`is_default DESC` → 内置管家 CASE 表达式（4 个核心管家优先）→ `kind DESC`（system_builtin 在 ecosystem_preset 前）→ `updated_at DESC` → `id ASC`（唯一决胜键，保证 LIMIT/OFFSET 分页不跳行/重复）
- `OrgNodeID` 通过 `categoryPositionIDsForFilter` 解析为职位 id 列表后用 `PositionIDIn` 匹配

### 4.2 DeleteAgent 实现（软删）

```go
func (r *agentRepo) DeleteAgent(ctx context.Context, id string) error {
    if id == "" { return fmt.Errorf("id is required") }
    now := nowRFC3339()
    _, err := r.data.RW().Write(ctx).Agent.UpdateOneID(id).
        SetDeletedAt(now).
        SetStatus("deleted").
        SetUpdatedAt(now).
        Save(ctx)
    return err
}
```

### 4.3 ListExtrasForAgents（运行态富化）

文件：`internal/data/agent_repo.go`（同文件）

批量查询每个 Agent 的最近会话运行状态 + pending 进化建议计数，返回 `map[agentID]AgentListExtras`。

### 4.4 ListAgentCreators

```go
func (r *agentRepo) ListAgentCreators(ctx context.Context) ([]biz.AgentCreator, error) {
    rows, err := r.data.RW().Read(ctx).Agent.Query().
        Where(agent.DeletedAtEQ(""), agent.CreatedByNEQ("")).
        Select(agent.FieldCreatedBy).
        GroupBy(agent.FieldCreatedBy).
        Strings(ctx)
    // 首项追加「仅我的」（当前用户）
    // ...
}
```

### 4.5 ReorderAgents（stub）

```go
func (r *agentRepo) ReorderAgents(ctx context.Context, ids []string) error {
    return nil  // TODO: 实现 position 排序持久化
}
```

---

## 五、Service 层

文件：`internal/service/agent.go`

### 5.1 ListAgents

```go
func (s *AgentService) ListAgents(ctx context.Context, req *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error) {
    page, err := s.uc.List(ctx, biz.AgentListQuery{
        Keyword:   req.GetKeyword(),
        Status:    req.GetStatus(),
        Provider:  req.GetProvider(),
        OrgNodeID: req.GetOrgNodeId(),
        CreatedBy: biz.ResolveListCreatedByFilter(ctx, req.GetCreatedBy()),
        Limit:     int(req.GetLimit()),
        Offset:    int(req.GetOffset()),
    })
    if err != nil { return nil, err }
    s.enrichEndpointFlags(ctx, page.Items)  // A2A endpoint 富化
    out := &v1.ListAgentsResponse{
        Total:  int32(page.Total),
        Limit:  int32(page.Limit),
        Offset: int32(page.Offset),
    }
    for i := range page.Items {
        out.Items = append(out.Items, toProtoAgent(page.Items[i]))
    }
    return out, nil
}
```

### 5.2 DeleteAgent / ToggleFavorite / DuplicateAgent / ListAgentCreators

```go
func (s *AgentService) DeleteAgent(ctx context.Context, req *v1.DeleteAgentRequest) (*emptypb.Empty, error)
func (s *AgentService) ToggleFavorite(ctx context.Context, req *v1.ToggleFavoriteRequest) (*v1.Agent, error)
func (s *AgentService) DuplicateAgent(ctx context.Context, req *v1.DuplicateAgentRequest) (*v1.Agent, error)
func (s *AgentService) ListAgentCreators(ctx context.Context, _ *emptypb.Empty) (*v1.ListAgentCreatorsResponse, error)
```

- `DeleteAgent`：删除后 `invalidateAgentBuildCache` + 审计
- `ToggleFavorite`：`NotFound` 错误翻译为 `apierror.NotFound`
- `DuplicateAgent`：调用 `uc.Duplicate`，深拷贝 files，副本 `created_by` = 当前用户；副本继承源 `position_id`/`position_key`（唯一索引 `(position_key, agent_variant)` 由 `agent_variant = 副本 agent_key` 保证不冲突），保留行业/岗位分类
- `ListAgentCreators`：返回创建者列表（含「仅我的」首项）

### 5.3 类型转换

`toProtoAgent(b biz.Agent) *v1.Agent` 完成 Biz → Proto 字段映射（含 `LastRunStatus` / `PendingEvolutionCount` / `Kind` / `Source` 等）。

---

## 六、Wire 注入

已有，无需新增。`AgentService` 通过 `NewAgentService(uc *biz.AgentUsecase)` 注入。

```go
var ProviderSet = wire.NewSet(NewAgentService)
```

---

## 七、Web 前端设计

### 7.1 文件结构（实际）

```
web/src/
├── pages/
│   └── AgentsPage.vue                    ← 列表页壳（组合 Hero + Filters + List + Pagination + Dialog）
├── features/agents/
│   ├── api.ts                            ← API 调用封装
│   ├── types.ts                          ← TypeScript 类型定义
│   ├── wireNormalize.ts                  ← Proto ↔ TS 数据规范化
│   ├── useAgentsPage.ts                  ← 列表页 composable（组合 Pinia Store + 局部 UI）
│   └── useAgentProviderModelPicker.ts    ← Provider/Model 选择器
├── components/agents/
│   ├── AgentsWorkspaceHero.vue           ← 页头 Hero（标题 + 创建/迁移按钮）
│   ├── AgentsFiltersCard.vue             ← 筛选行（关键字 + 状态 + Provider + 业务分类 + 创建者 + 视图切换）
│   ├── AgentsListSection.vue             ← 网格/表格/空态/骨架屏/三组分组/拖拽排序
│   ├── AgentsPaginationBar.vue           ← 底部分页栏
│   ├── AgentCard.vue                     ← 网格卡片
│   ├── KindBadge.vue                     ← 归属类型徽章（builtin/preset/user）
│   └── TaxonomyFilter.vue                ← 分类体系筛选（替代旧 category filter）
└── stores/
    └── agents/
        └── index.ts                    ← Pinia Store `useAgentsPageStore`（列表状态、筛选、分页、CRUD）
```

### 7.2 TypeScript 类型定义

文件：`web/src/features/agents/types.ts`

```typescript
export type AgentOwnership = '' | 'user' | 'system_builtin' | 'ecosystem_preset' | 'marketplace' | 'certified';
export type AgentKind = '' | 'llm' | 'a2a_proxy';

export type Agent = {
  id: string;
  agent_key: string;
  display_name: string;
  provider: string;
  model: string;
  agent_kind?: AgentKind;
  kind?: AgentOwnership;
  a2a_proxy_config?: A2AProxyConfig;
  a2a_endpoint_enabled?: boolean;
  last_run_status?: string;
  last_run_at?: string;
  pending_evolution_count?: number;
  status: string;
  is_default: boolean;
  is_favorite: boolean;
  icon: string;
  agent_description: string;
  position_key?: string;
  agent_variant?: string;
  variant_description?: string;
  taxonomy_position_id: string;
  system_prompt_mode: string;
  context_window: number;
  budget_monthly_cents: number;
  config_json: string;
  created_at: string;
  updated_at: string;
  deleted_at: string;
  created_by?: string;
  readonly?: boolean;
  source?: string;
  settings?: AgentRuntimeSettings;
  files?: AgentPromptFile[];
};

export type AgentListQuery = {
  keyword?: string;
  status?: string;
  provider?: string;
  org_node_id?: string;   // 对应 proto org_node_id
  created_by?: string;    // 空 | "mine" | 用户 id
  limit?: number;
  offset?: number;
};

export type AgentListResult = {
  items: Agent[];
  total: number;
  limit: number;
  offset: number;
};

export type AgentCreatorOption = {
  user_id: string;
  label: string;
};
```

### 7.3 API 调用

文件：`web/src/features/agents/api.ts`

```typescript
export async function listAgentsPaged(query: AgentListQuery = {}): Promise<AgentListResult> {
  const svc = createAgentService();
  const res = await svc.ListAgents({
    keyword: query.keyword,
    status: query.status,
    provider: query.provider,
    orgNodeId: query.org_node_id,
    createdBy: query.created_by,
    limit: query.limit,
    offset: query.offset,
  });
  return {
    items: (res.items ?? []).map(normalizeAgentFromService),
    total: Number(res.total ?? res.items?.length ?? 0),
    limit: Number(res.limit ?? query.limit ?? 24),
    offset: Number(res.offset ?? query.offset ?? 0),
  };
}

export async function deleteAgent(id: string): Promise<void> {
  await createAgentService().DeleteAgent({ id });
}

export async function toggleAgentFavorite(id: string): Promise<Agent> {
  const res = await createAgentService().ToggleFavorite({ id });
  return normalizeAgentFromService(res);
}

export async function duplicateAgent(id: string): Promise<Agent> {
  const res = await createAgentService().DuplicateAgent({ id });
  return normalizeAgentFromService(res);
}

export async function listAgentCreators(): Promise<AgentCreatorOption[]> {
  const res = await createAgentService().ListAgentCreators({});
  return (res.items ?? []).map((row) => ({
    user_id: row.userId ?? '',
    label: row.label ?? row.userId ?? '',
  }));
}
```

### 7.4 Composable（Pinia Store 组合）

文件：`web/src/features/agents/useAgentsPage.ts`

`useAgentsPage` 组合 `useAgentsPageStore`（Pinia）与局部 UI 状态（创建弹窗、删除弹窗、迁移弹窗、表单）。列表/筛选/分页状态由 Store 管理，符合 aranea-frontend-guide §3 数据流铁律。

```typescript
export function useAgentsPage() {
  const pageStore = useAgentsPageStore();
  const { agents, keyword, selectedStatus, selectedProvider, selectedTaxonomy,
          selectedCreator, creatorOptions, page, rowsPerPage, total,
          listLoading, taxonomyTree, pageMax, providerOptions, tableColumns } = storeToRefs(pageStore);

  const createOpen = ref(false);
  const migrationOpen = ref(false);
  const deleteOpen = ref(false);
  const viewMode = ref<ViewMode>((localStorage.getItem(LS_VIEW) as ViewMode) || 'grid');

  // ... 创建/删除/复制/收藏/迁移事件处理
}
```

Store 内部 watch `keyword` / `selectedStatus` / `selectedProvider` / `selectedTaxonomy` / `selectedCreator` 变化时重置 page=1 并触发 `loadAgentList`。

### 7.5 组件契约

**AgentsListSection.vue**（核心列表区）：

```typescript
defineProps<{
  loading: boolean;
  agents: Agent[];
  keyword: string;
  viewMode: 'grid' | 'list';
  rowsPerPage: number;
  tableColumns: QTableColumn<Agent>[];
  isFavorite: (id: string) => boolean;
  getCategoryLabel: (taxonomyPositionId: string) => string;
}>();

defineEmits<{
  create: [];
  'toggle-favorite': [id: string];
  'copy-key': [key: string];
  delete: [agent: Agent];
  duplicate: [agent: Agent];
  reorder: [ids: string[]];
}>();
```

三组分组逻辑（`isDeptLead = a.agent_variant === 'dept_lead'`）：
- `builtinAgents` = `agents.filter(a => a.readonly && a.kind === 'system_builtin' && !isDeptLead(a))` — 仅 4 个核心管家（精灵助手/系统管家/记忆管家/技能管家），与后端 `CleanupNonSystemData` 保留规则一致
- `presetAgents` = `agents.filter(a => isDeptLead(a) || (!a.readonly && a.kind === 'ecosystem_preset'))` — 26 个部门主管归入预设模板区
- `userAgents` = `agents.filter(a => !a.readonly && a.kind !== 'ecosystem_preset' && !isDeptLead(a))`

`userAgents` 通过 `vuedraggable` 支持拖拽排序，拖拽结束触发 `reorder` 事件。

**AgentCard.vue**：

```typescript
defineProps<{ agent: Agent }>();
defineEmits<{
  'toggle-favorite': [id: string];
  'copy-key': [key: string];
  delete: [agent: Agent];
  duplicate: [agent: Agent];
}>();
```

系统内置 Agent（`readonly=true`）不显示「复制」「删除」按钮。

**AgentsFiltersCard.vue**：

使用 `defineModel` 双向绑定 `keyword` / `selectedStatus` / `selectedTaxonomy` / `selectedCreator` / `selectedProvider` / `viewMode`。

### 7.6 状态映射

| Agent status | Badge 颜色 | 说明 |
|-------------|-----------|------|
| `active` | positive (绿) | 活跃 |
| `inactive` | grey (灰) | 停用 |
| `deleted` | 不展示 | 已软删 |

### 7.7 上下文窗口格式化

```typescript
function formatContextWindow(ctx: number): string {
  if (ctx <= 0) return '';
  if (ctx >= 1_000_000) return `${(ctx / 1_000_000).toFixed(1)}M ctx`;
  return `${Math.round(ctx / 1000)}K ctx`;
}
```

### 7.8 运行态格式化

`formatLastRunContext(agent)` 位于 `components/agents/agentUi.ts`，组合 `last_run_status` + `last_run_at` 输出如 `active · 2 小时前`。

### 7.9 进化中判定

```typescript
function isAgentEvolving(agent: Agent): boolean {
  return agent.settings?.self_evolve === true && (agent.pending_evolution_count ?? 0) > 0;
}
```

### 7.10 空状态与异常

| 场景 | 处理 |
|------|------|
| 无数据 | 插图 + 文案「暂无 Agent」+ 引导「创建 Agent」 |
| 搜索无结果 | 「未找到匹配的 Agent」+ 清除搜索 |
| 加载中 | 骨架屏 `QSkeleton` |
| 接口失败 | `Notify` 错误信息，保留上次成功数据 |
| 删除成功 | `Notify` 成功 + 刷新列表 |
| 收藏切换 | 即时更新本地状态 + 后台同步 |
| 复制成功 | `Notify` 成功 + 刷新列表 |

---

## 八、列表项字段与数据模型映射

### 8.1 Ent Schema

文件：`internal/data/ent/schema/agent.go`

表名：`agents`（`entsql.Annotation{Table: "agents"}`）

| Ent 字段 | 类型 | 默认 | 说明 |
|---------|------|------|------|
| `id` | string(256) | — | Immutable Unique |
| `agent_key` | string(512) | — | Unique |
| `display_name` | string(1024) | — | |
| `provider` | string | — | |
| `model` | string | — | |
| `status` | string | `active` | |
| `is_default` | bool | false | |
| `is_favorite` | bool | false | |
| `icon` | string | `""` | 头像资产 id |
| `agent_description` | text | `""` | |
| `system_prompt_mode` | string | `""` | |
| `context_window` | int | 0 | |
| `budget_monthly_cents` | int | 0 | |
| `config_json` | text | `""` | |
| `roles_json` | text | `[]` | |
| `created_by` | string | `""` | 创建者用户 id |
| `created_at` / `updated_at` / `deleted_at` | string | `""` | RFC3339 |
| `readonly` | bool | false | 系统内置不可删 |
| `kind` | enum | `user` | `user` / `system_builtin` / `ecosystem_preset` / `marketplace` / `certified` |
| `source` | enum | `user` | `user` / `system` / `imported` |
| `position_key` | string | `""` | FK to positions.key |
| `position_id` | string | `""` | FK to organizations(position) |
| `agent_variant` | string | `general` | |
| `variant_description` | text | `""` | |

索引：
- `deleted_at`
- `deleted_at` + `status`
- `position_key` + `agent_variant`（Unique）

### 8.2 界面展示 ↔ 数据库列映射

| 界面展示 | 数据库列 / 来源 | 说明 |
|----------|-----------------|------|
| 名称 | `display_name` | 必填展示 |
| handle | `agent_key` | 唯一业务键展示 |
| 头像 | `icon` | 头像资产 id；`src` 指向只读出图接口 |
| 归属徽章 | `kind` + `readonly` | `readonly=true` → builtin；`kind=ecosystem_preset` → preset；其余 → user |
| 状态徽章 | `status` | 如 `active` |
| 模型行 | `provider` + `model` | 拼接为 `provider / model` |
| 业务分类 | `position_id` 解析 | 副文案或 `QChip`，与筛选同源 |
| 卡片描述摘要 | `agent_description` 截断 | |
| 标签 chips | `roles_json` 或 `config_json.tags` | 产品枚举需与后端约定 |
| 进化中 | `settings.self_evolve` + `pending_evolution_count` | `self_evolve && pending_evolution_count > 0` |
| 运行态 | `last_run_status` + `last_run_at` | 来自 `ListExtrasForAgents` 富化 |
| 200K ctx | `context_window` | UI 格式化为 `200K ctx` |
| 收藏星标 | `is_favorite` | 见 §九 |
| 创建者 | `created_by` | 筛选用 |

---

## 九、收藏设计

当前实现：`is_favorite` 字段存储在 `agents` 表行内（单用户维度）。

| 方案 | 说明 | 现状 |
|------|------|------|
| **当前** | `agents.is_favorite` bool 字段，`ToggleFavorite` RPC 直接翻转 | ✅ 已实现 |
| **推荐（多用户）** | 独立表 `user_agent_favorites(user_id, agent_id, created_at)`，唯一 `(user_id, agent_id)` | 未实现 |

`ToggleFavorite` RPC：`PATCH /v1/agents/{id}/favorite`，返回更新后的 `Agent`。

多用户场景需迁移至独立收藏表，列表接口返回 `is_favorite`（当前用户维度）。

---

## 十、列表 API 契约

### 10.1 端点表

| 方法 | 路由 | RPC | 说明 |
|------|------|-----|------|
| GET | `/v1/agents` | `ListAgents` | 列表查询 |
| DELETE | `/v1/agents/{id}` | `DeleteAgent` | 软删 |
| PATCH | `/v1/agents/{id}/favorite` | `ToggleFavorite` | 收藏切换 |
| POST | `/v1/agents/{id}/duplicate` | `DuplicateAgent` | 复制 |
| GET | `/v1/agents/creators` | `ListAgentCreators` | 创建者列表 |
| POST | `/v1/agents:batchUpdate` | `BatchUpdateAgents` | 批量启用/停用/删除（status 与 delete 互斥） |

### 10.2 列表查询参数

| 参数 | 类型 | 默认 | 说明 |
|------|------|------|------|
| `keyword` | string | `""` | 命中 display_name/agent_key/provider/model/agent_description |
| `status` | string | `""` | active/inactive/deleted |
| `provider` | string | `""` | |
| `org_node_id` | string | `""` | 业务分类职位节点 id |
| `created_by` | string | `""` | 空 = 全部；`mine` = 当前用户；否则用户 id |
| `limit` | int32 | 24 | 最大 100 |
| `offset` | int32 | 0 | |

### 10.3 响应

```json
{
  "items": [Agent],
  "total": 100,
  "limit": 24,
  "offset": 0
}
```

`Agent` 含列表所需列 + `is_favorite` + `last_run_status` + `last_run_at` + `pending_evolution_count` + `kind` + `source` + `readonly`。

---

## 十一、与创建页的衔接

| 事件 | 列表页行为 |
|------|------------|
| 创建成功 | 关闭弹窗 + 刷新列表（保持筛选/页码） |
| 删除成功 | 从列表移除 + 刷新 |
| 收藏切换 | 即时更新卡片星标状态 |
| 复制成功 | 刷新列表，新副本出现在用户分组 |

---

## 十二、设计覆盖与实现差距

> 任务清单与进度状态详见 [3-agent-list.development.md §4](./3-agent-list.development.md#4-任务清单)。

**已实现（设计已覆盖）**：
- Proto：`ListAgents` / `DeleteAgent` / `ToggleFavorite` / `DuplicateAgent` / `ListAgentCreators` / `BatchUpdateAgents` RPC
- Biz：`AgentUsecase.List` / `Delete` / `ToggleFavorite` / `Duplicate` / `ListAgentCreators` / `BatchUpdateAgents`
- Data：`SearchAgents`（keyword/status/provider/org_node_id/created_by/role/kind 筛选）/ `ListExtrasForAgents` / `ListAgentCreators`
- Service：`ListAgents` / `DeleteAgent` / `ToggleFavorite` / `DuplicateAgent` / `ListAgentCreators` / `BatchUpdateAgents`（含逐 ID 变更权限校验 + 缓存失效）
- Web：`api.ts` / `useAgentsPage.ts` / `AgentsPage.vue` / `AgentsListSection.vue` / `AgentCard.vue` / `KindBadge.vue` / `TaxonomyFilter.vue`
- Web：批量操作 UI（卡片多选 + 批量启用/停用/删除工具条，`stores/agents` selectedAgentIds）
- Wire：`NewAgentService` 注入

**待实现（设计未覆盖，需新增设计）**：
- Proto：`ReorderAgents` RPC（biz/data 为 stub，需同步实现 data 层持久化 + proto 暴露）
- 前端：Agent 迁移导入/导出流程
