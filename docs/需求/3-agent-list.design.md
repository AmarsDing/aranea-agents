# Agent 列表模块 — 实现设计文档

> 对应需求：`3 agent-list.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 管理主列表页，支持搜索、筛选（关键字/状态/Provider/业务分类）、网格/列表视图切换、收藏、删除、迁移入口。后端复用 `AgentService.ListAgents` / `DeleteAgent` RPC，新增 `ToggleFavorite` RPC。

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

message ListAgentsRequest {
  string keyword = 1;
  string status = 2;
  string provider = 3;
  string category_id = 4;
  int32 limit = 5;
  int32 offset = 6;
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

service AgentService {
  rpc ListAgents(ListAgentsRequest) returns (ListAgentsResponse) {
    option (google.api.http) = {get: "/v1/agents"};
  }
  rpc DeleteAgent(DeleteAgentRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {delete: "/v1/agents/{id}"};
  }
}
```

### 2.2 待新增 Proto

```protobuf
message ToggleFavoriteRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

// 在 AgentService 中新增
rpc ToggleFavorite(ToggleFavoriteRequest) returns (Agent) {
  option (google.api.http) = { patch: "/v1/agents/{id}/favorite" body: "*" };
}
```

### 2.3 消息字段说明

| 消息 | 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `ListAgentsRequest` | `keyword` | string | ❌ | 搜索关键字（命中 display_name/agent_key/provider/model/agent_description） |
| | `status` | string | ❌ | 状态筛选（active/inactive/deleted） |
| | `provider` | string | ❌ | Provider 筛选 |
| | `category_id` | string | ❌ | 业务分类职位 ID 筛选 |
| | `limit` | int32 | ❌ | 每页条数，默认 24，最大 100 |
| | `offset` | int32 | ❌ | 偏移量，默认 0 |
| `ListAgentsResponse` | `items` | Agent[] | — | Agent 列表（列表页不含 settings/files 水合） |
| | `total` | int32 | — | 总条数 |
| | `limit` | int32 | — | 实际 limit |
| | `offset` | int32 | — | 实际 offset |
| `ToggleFavoriteRequest` | `id` | string | ✅ | Agent ID |

---

## 三、Biz 层

### 3.1 领域模型

列表页使用已有 `biz.Agent`、`biz.AgentListQuery`、`biz.AgentListResult`，无需新增模型。

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

type AgentListQuery struct {
    Keyword    string
    Status     string
    Provider   string
    CategoryID string
    Limit      int
    Offset     int
}

type AgentListResult struct {
    Items  []Agent
    Total  int
    Limit  int
    Offset int
}
```

### 3.2 已有 Usecase 方法

```go
func (u *AgentUsecase) List(ctx context.Context, q AgentListQuery) (AgentListResult, error)
func (u *AgentUsecase) Delete(ctx context.Context, id string) error
```

### 3.3 新增 Usecase 方法

```go
func (u *AgentUsecase) ToggleFavorite(ctx context.Context, id string) (Agent, error) {
    id = strings.TrimSpace(id)
    if id == "" {
        return Agent{}, kerrors.BadRequest("AGENT", "id is required")
    }
    a, err := u.repo.GetAgentByID(ctx, id)
    if err != nil {
        if stderrors.Is(err, sql.ErrNoRows) {
            return Agent{}, kerrors.NotFound("AGENT", "agent not found")
        }
        return Agent{}, kerrors.InternalServer("AGENT", err.Error())
    }
    a.IsFavorite = !a.IsFavorite
    updated, err := u.repo.UpdateAgent(ctx, a)
    if err != nil {
        return Agent{}, kerrors.InternalServer("AGENT", err.Error())
    }
    return updated, nil
}
```

### 3.4 已有 Repo 接口

```go
type AgentRepository interface {
    SearchAgents(ctx context.Context, q AgentListQuery) (AgentListResult, error)
    GetAgentByID(ctx context.Context, id string) (Agent, error)
    UpdateAgent(ctx context.Context, a Agent) (Agent, error)
    DeleteAgent(ctx context.Context, id string) error
}
```

---

## 四、Data 层

### 4.1 已有实现

`SearchAgents` 已在 `internal/data/agent_repo.go` 中实现：

```go
func (r *agentRepo) SearchAgents(ctx context.Context, q biz.AgentListQuery) (biz.AgentListResult, error) {
    if q.Limit <= 0 {
        q.Limit = 24
    }
    if q.Limit > 100 {
        q.Limit = 100
    }
    if q.Offset < 0 {
        q.Offset = 0
    }
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
    if q.Status != "" {
        preds = append(preds, agent.StatusEQ(q.Status))
    }
    if q.Provider != "" {
        preds = append(preds, agent.ProviderEQ(q.Provider))
    }
    if q.CategoryID != "" {
        preds = append(preds, agent.CategoryPositionIDEQ(q.CategoryID))
    }
    where := agent.And(preds...)
    c := r.data.entClient
    total, err := c.Agent.Query().Where(where).Count(ctx)
    if err != nil {
        return biz.AgentListResult{}, err
    }
    rows, err := c.Agent.Query().Where(where).
        Order(agent.ByIsDefault(entsql.OrderDesc()), agent.ByUpdatedAt(entsql.OrderDesc())).
        Limit(q.Limit).
        Offset(q.Offset).
        All(ctx)
    if err != nil {
        return biz.AgentListResult{}, err
    }
    items := make([]biz.Agent, 0, len(rows))
    for _, row := range rows {
        items = append(items, entAgentToBiz(row))
    }
    return biz.AgentListResult{Items: items, Total: total, Limit: q.Limit, Offset: q.Offset}, nil
}
```

### 4.2 已有转换函数

```go
func entAgentToBiz(a *ent.Agent) biz.Agent {
    if a == nil {
        return biz.Agent{}
    }
    return biz.Agent{
        ID:                 a.ID,
        AgentKey:           a.AgentKey,
        DisplayName:        a.DisplayName,
        Provider:           a.Provider,
        Model:              a.Model,
        Status:             a.Status,
        IsDefault:          a.IsDefault,
        IsFavorite:         a.IsFavorite,
        Icon:               a.Icon,
        AgentDescription:   a.AgentDescription,
        CategoryPositionID: a.CategoryPositionID,
        SystemPromptMode:   a.SystemPromptMode,
        ContextWindow:      a.ContextWindow,
        BudgetMonthlyCents: a.BudgetMonthlyCents,
        ConfigJSON:         a.ConfigJSON,
        CreatedAt:          a.CreatedAt,
        UpdatedAt:          a.UpdatedAt,
        DeletedAt:          a.DeletedAt,
    }
}
```

### 4.3 DeleteAgent 实现

```go
func (r *agentRepo) DeleteAgent(ctx context.Context, id string) error {
    if id == "" {
        return fmt.Errorf("id is required")
    }
    now := nowRFC3339()
    _, err := r.data.entClient.Agent.UpdateOneID(id).
        SetDeletedAt(now).
        SetStatus("deleted").
        SetUpdatedAt(now).
        Save(ctx)
    return err
}
```

---

## 五、Service 层

### 5.1 已有 Service 实现

文件：`internal/service/agent.go`

```go
type AgentService struct {
    v1.UnimplementedAgentServiceServer
    uc *biz.AgentUsecase
}

func NewAgentService(uc *biz.AgentUsecase) *AgentService {
    return &AgentService{uc: uc}
}

func (s *AgentService) ListAgents(ctx context.Context, req *v1.ListAgentsRequest) (*v1.ListAgentsResponse, error) {
    page, err := s.uc.List(ctx, biz.AgentListQuery{
        Keyword:    req.GetKeyword(),
        Status:     req.GetStatus(),
        Provider:   req.GetProvider(),
        CategoryID: req.GetCategoryId(),
        Limit:      int(req.GetLimit()),
        Offset:     int(req.GetOffset()),
    })
    if err != nil {
        return nil, err
    }
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

func (s *AgentService) DeleteAgent(ctx context.Context, req *v1.DeleteAgentRequest) (*emptypb.Empty, error) {
    if err := s.uc.Delete(ctx, req.GetId()); err != nil {
        return nil, err
    }
    return &emptypb.Empty{}, nil
}
```

### 5.2 新增 ToggleFavorite 方法

```go
func (s *AgentService) ToggleFavorite(ctx context.Context, req *v1.ToggleFavoriteRequest) (*v1.Agent, error) {
    a, err := s.uc.ToggleFavorite(ctx, req.GetId())
    if err != nil {
        if stderrors.Is(err, sql.ErrNoRows) {
            return nil, kerrors.NotFound("AGENT", "agent not found")
        }
        return nil, err
    }
    return toProtoAgent(a), nil
}
```

### 5.3 类型转换函数

```go
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
```

---

## 六、Wire 注入

已有，无需新增。`AgentService` 通过 `NewAgentService(uc *biz.AgentUsecase)` 注入。

```go
var ProviderSet = wire.NewSet(NewAgentService)
```

---

## 七、Web 前端设计

### 7.1 文件结构

```
web/src/features/agents/
├── api.ts                     ← API 调用封装
├── types.ts                   ← TypeScript 类型定义
├── wireNormalize.ts           ← Proto ↔ TS 数据规范化
├── useAgentsPage.ts           ← 列表页 composable
└── components/
    ├── AgentListPage.vue       ← 列表页主组件
    ├── AgentCard.vue           ← 网格卡片
    ├── AgentListItem.vue       ← 列表行
    ├── AgentCreateDialog.vue   ← 创建弹窗（见 2 agents-create.design.md）
    └── AgentDeleteConfirm.vue  ← 删除确认弹窗
```

### 7.2 TypeScript 类型定义

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

export type AgentListQuery = {
  keyword?: string;
  status?: string;
  provider?: string;
  category_id?: string;
  limit?: number;
  offset?: number;
};

export type AgentListResult = {
  items: Agent[];
  total: number;
  limit: number;
  offset: number;
};
```

### 7.3 API 调用

```typescript
import { createAgentService } from "../../services";
import type {
  CreateAgentRequest as KratosCreateAgentRequest
} from "../../services/kratos/agent/v1/index";
import type { Agent, AgentListQuery, AgentListResult } from "./types";
import { normalizeAgentFromService, partialAgentToWire } from "./wireNormalize";

export async function listAgentsPaged(query: AgentListQuery = {}): Promise<AgentListResult> {
  const svc = createAgentService();
  const res = await svc.ListAgents({
    keyword: query.keyword,
    status: query.status,
    provider: query.provider,
    categoryId: query.category_id,
    limit: query.limit,
    offset: query.offset
  });
  return {
    items: (res.items ?? []).map((row) => normalizeAgentFromService(row)),
    total: Number(res.total ?? res.items?.length ?? 0),
    limit: Number(res.limit ?? query.limit ?? 24),
    offset: Number(res.offset ?? query.offset ?? 0)
  };
}

export async function deleteAgent(id: string): Promise<void> {
  const svc = createAgentService();
  await svc.DeleteAgent({ id });
}

export async function toggleFavorite(id: string): Promise<Agent> {
  const svc = createAgentService();
  const data = await svc.ToggleFavorite({ id });
  return normalizeAgentFromService(data);
}
```

### 7.4 Composable

```typescript
import { ref, reactive, computed, watch, onMounted } from "vue";
import { listAgentsPaged, deleteAgent, toggleFavorite } from "./api";
import type { Agent, AgentListQuery } from "./types";

type ViewMode = "grid" | "list";

export function useAgentsPage() {
  const agents = ref<Agent[]>([]);
  const total = ref(0);
  const loading = ref(false);

  const viewMode = ref<ViewMode>(
    (localStorage.getItem("agents.viewMode") as ViewMode) || "grid"
  );
  const filters = reactive<AgentListQuery>({
    keyword: "",
    status: "",
    provider: "",
    category_id: "",
    limit: 24,
    offset: 0
  });

  const currentPage = computed(() =>
    Math.floor(filters.offset! / filters.limit!) + 1
  );
  const totalPages = computed(() =>
    Math.max(1, Math.ceil(total.value / filters.limit!))
  );

  async function fetchAgents() {
    loading.value = true;
    try {
      const result = await listAgentsPaged(filters);
      agents.value = result.items;
      total.value = result.total;
    } finally {
      loading.value = false;
    }
  }

  async function onDelete(id: string) {
    await deleteAgent(id);
    await fetchAgents();
  }

  async function onToggleFavorite(id: string) {
    await toggleFavorite(id);
    const idx = agents.value.findIndex((a) => a.id === id);
    if (idx >= 0) {
      agents.value[idx].is_favorite = !agents.value[idx].is_favorite;
    }
  }

  function setViewMode(mode: ViewMode) {
    viewMode.value = mode;
    localStorage.setItem("agents.viewMode", mode);
  }

  function setPage(page: number) {
    filters.offset = (page - 1) * filters.limit!;
    fetchAgents();
  }

  function resetPage() {
    filters.offset = 0;
    fetchAgents();
  }

  watch(
    () => [filters.keyword, filters.status, filters.provider, filters.category_id],
    () => resetPage()
  );

  onMounted(fetchAgents);

  return {
    agents,
    total,
    loading,
    viewMode,
    filters,
    currentPage,
    totalPages,
    fetchAgents,
    onDelete,
    onToggleFavorite,
    setViewMode,
    setPage,
    resetPage
  };
}
```

### 7.5 组件设计

**AgentListPage.vue**：

```vue
<template>
  <QPage class="q-pa-md">
    <div class="row items-center justify-between q-mb-md">
      <div>
        <div class="text-h5">Agent</div>
        <div class="text-caption text-grey">管理您的 AI Agent</div>
      </div>
      <div class="row q-gutter-sm">
        <QBtn outline icon="swap_horiz" label="Agent迁移" @click="onMigrate" />
        <QBtn unelevated color="primary" icon="add" label="创建Agent" @click="createOpen = true" />
      </div>
    </div>

    <div class="row q-col-gutter-sm items-center q-mb-md">
      <div class="col-12 col-md-3">
        <QInput
          v-model="filters.keyword"
          outlined
          dense
          placeholder="搜索Agent..."
          debounce="400"
          clearable
        >
          <template #prepend><QIcon name="search" /></template>
        </QInput>
      </div>
      <div class="col-6 col-md-2">
        <QSelect
          v-model="filters.status"
          :options="statusOptions"
          outlined
          dense
          emit-value
          map-options
          clearable
          label="状态"
        />
      </div>
      <div class="col-6 col-md-2">
        <QSelect
          v-model="filters.provider"
          :options="providerOptions"
          outlined
          dense
          emit-value
          map-options
          clearable
          label="Provider"
        />
      </div>
      <div class="col-6 col-md-2">
        <QSelect
          v-model="filters.category_id"
          :options="categoryOptions"
          outlined
          dense
          emit-value
          map-options
          clearable
          label="业务分类"
        />
      </div>
      <div class="col-auto">
        <QBtnToggle
          v-model="viewMode"
          :options="[{value:'grid',icon:'grid_view'},{value:'list',icon:'view_list'}]"
          flat
          toggle-color="primary"
          @update:model-value="setViewMode"
        />
      </div>
    </div>

    <QInnerLoading :showing="loading" />

    <div v-if="!loading && agents.length === 0" class="column items-center q-pa-xl text-grey">
      <QIcon name="smart_toy" size="64px" />
      <div class="text-h6 q-mt-sm">暂无 Agent</div>
      <QBtn color="primary" label="创建 Agent" class="q-mt-md" @click="createOpen = true" />
    </div>

    <div v-if="viewMode === 'grid'" class="row q-col-gutter-md">
      <div v-for="agent in agents" :key="agent.id" class="col-12 col-sm-6 col-md-4 col-lg-3">
        <AgentCard
          :agent="agent"
          @toggle-favorite="onToggleFavorite"
          @delete="onDelete"
        />
      </div>
    </div>

    <QTable
      v-else
      :rows="agents"
      :columns="listColumns"
      row-key="id"
      flat
      :loading="loading"
      hide-pagination
    >
      <template #body-cell-is_favorite="props">
        <QTd :props="props">
          <QBtn
            flat
            round
            dense
            :icon="props.row.is_favorite ? 'star' : 'star_border'"
            :color="props.row.is_favorite ? 'amber' : 'grey'"
            @click="onToggleFavorite(props.row.id)"
          />
        </QTd>
      </template>
      <template #body-cell-actions="props">
        <QTd :props="props">
          <QBtn flat round dense icon="delete" color="negative" @click="onDelete(props.row.id)" />
        </QTd>
      </template>
    </QTable>

    <div class="row items-center justify-between q-mt-md">
      <div class="text-caption text-grey">{{ total }} 条</div>
      <div class="row items-center q-gutter-sm">
        <QSelect
          v-model="filters.limit"
          :options="[10, 20, 50]"
          outlined
          dense
          style="width: 80px"
          @update:model-value="resetPage"
        />
        <span class="text-caption">第 {{ currentPage }} / {{ totalPages }} 页</span>
        <QBtn
          round
          flat
          dense
          icon="chevron_left"
          :disable="currentPage <= 1"
          @click="setPage(currentPage - 1)"
        />
        <QBtn
          round
          flat
          dense
          icon="chevron_right"
          :disable="currentPage >= totalPages"
          @click="setPage(currentPage + 1)"
        />
      </div>
    </div>

    <AgentCreateDialog v-model="createOpen" @created="fetchAgents" />
  </QPage>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { useAgentsPage } from "../useAgentsPage";
import AgentCard from "./AgentCard.vue";
import AgentCreateDialog from "./AgentCreateDialog.vue";

const {
  agents, total, loading, viewMode, filters,
  currentPage, totalPages, fetchAgents,
  onDelete, onToggleFavorite, setViewMode, setPage, resetPage
} = useAgentsPage();

const createOpen = ref(false);

const statusOptions = [
  { label: "活跃", value: "active" },
  { label: "停用", value: "inactive" }
];

const providerOptions = [
  { label: "OpenAI", value: "openai" },
  { label: "DeepSeek", value: "deepseek" },
  { label: "Anthropic", value: "anthropic" },
  { label: "Gemini", value: "gemini" },
  { label: "Ollama", value: "ollama" }
];

const categoryOptions = ref<{ label: string; value: string }[]>([]);

const listColumns = [
  { name: "is_favorite", label: "", field: "is_favorite", align: "center" as const, sortable: false },
  { name: "display_name", label: "名称", field: "display_name", align: "left" as const, sortable: true },
  { name: "agent_key", label: "标识", field: "agent_key", align: "left" as const },
  { name: "status", label: "状态", field: "status", align: "center" as const },
  { name: "provider", label: "Provider", field: "provider", align: "left" as const },
  { name: "model", label: "模型", field: "model", align: "left" as const },
  { name: "context_window", label: "上下文", field: "context_window", align: "right" as const, format: (v: number) => v ? `${Math.round(v / 1000)}K ctx` : "" },
  { name: "actions", label: "", field: "actions", align: "center" as const, sortable: false }
];

function onMigrate() {}
</script>
```

**AgentCard.vue**：

```vue
<template>
  <QCard flat bordered class="agent-card cursor-pointer" @click="$emit('click', agent)">
    <QCardSection class="row items-start q-pb-none">
      <QAvatar size="40px" rounded class="q-mr-sm">
        <img v-if="agent.icon" :src="`/avatar-assets/${agent.icon}/thumbnail`" />
        <QIcon v-else name="smart_toy" size="40px" color="grey" />
      </QAvatar>
      <div class="col">
        <div class="row items-center">
          <span class="text-subtitle1 text-weight-medium ellipsis">{{ agent.display_name }}</span>
          <QBtn
            flat
            round
            dense
            size="sm"
            :icon="agent.is_favorite ? 'star' : 'star_border'"
            :color="agent.is_favorite ? 'amber' : 'grey'"
            class="q-ml-xs"
            @click.stop="$emit('toggle-favorite', agent.id)"
          />
        </div>
        <div class="text-caption text-grey ellipsis">{{ agent.agent_key }}</div>
      </div>
      <QBadge
        :color="statusColor"
        :label="agent.status"
        class="q-ml-sm"
      />
    </QCardSection>

    <QCardSection class="q-pt-xs q-pb-xs">
      <div class="text-caption text-grey-7">{{ agent.provider }} / {{ agent.model }}</div>
      <div class="text-body2 ellipsis-3-lines q-mt-xs" style="min-height: 40px">
        {{ agent.agent_description || '暂无描述' }}
      </div>
    </QCardSection>

    <QCardSection class="row items-center q-pt-none">
      <QChip v-if="agent.system_prompt_mode" dense size="sm" :label="agent.system_prompt_mode" />
      <QSpace />
      <span class="text-caption text-grey q-mr-sm">
        {{ agent.context_window ? `${Math.round(agent.context_window / 1000)}K ctx` : '' }}
      </span>
      <QBtn flat round dense size="sm" icon="delete" color="negative" @click.stop="confirmDelete = true" />
    </QCardSection>

    <QDialog v-model="confirmDelete" persistent>
      <QCard>
        <QCardSection class="row items-center">
          <QAvatar icon="warning" color="negative" text-color="white" />
          <span class="q-ml-sm">确认删除 Agent「{{ agent.display_name }}」？</span>
        </QCardSection>
        <QCardActions align="right">
          <QBtn flat label="取消" @click="confirmDelete = false" />
          <QBtn unelevated color="negative" label="删除" @click="$emit('delete', agent.id); confirmDelete = false" />
        </QCardActions>
      </QCard>
    </QDialog>
  </QCard>
</template>

<script setup lang="ts">
import { ref, computed } from "vue";
import type { Agent } from "../types";

const props = defineProps<{ agent: Agent }>();
defineEmits<{
  click: [agent: Agent];
  "toggle-favorite": [id: string];
  delete: [id: string];
}>();

const confirmDelete = ref(false);

const statusColor = computed(() => {
  switch (props.agent.status) {
    case "active": return "positive";
    case "inactive": return "grey";
    default: return "grey";
  }
});
</script>

<style scoped>
.ellipsis-3-lines {
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
</style>
```

### 7.6 状态映射

| Agent status | Badge 颜色 | 说明 |
|-------------|-----------|------|
| `active` | positive (绿) | 活跃 |
| `inactive` | grey (灰) | 停用 |
| `deleted` | 不展示 | 已软删 |

### 7.7 上下文窗口格式化

```typescript
function formatContextWindow(ctx: number): string {
  if (ctx <= 0) return "";
  if (ctx >= 1_000_000) return `${(ctx / 1_000_000).toFixed(1)}M ctx`;
  return `${Math.round(ctx / 1000)}K ctx`;
}
```

### 7.8 空状态与异常

| 场景 | 处理 |
|------|------|
| 无数据 | 插图 + 文案「暂无 Agent」+ 引导「创建 Agent」 |
| 搜索无结果 | 「未找到匹配的 Agent」+ 清除搜索 |
| 加载中 | `QInnerLoading` |
| 接口失败 | `Notify` 错误信息，保留上次成功数据 |
| 删除成功 | `Notify` 成功 + 刷新列表 |
| 收藏切换 | 即时更新本地状态 + 后台同步 |

---

## 八、与创建页的衔接

| 事件 | 列表页行为 |
|------|------------|
| 创建成功 | 关闭弹窗 + 刷新列表（保持筛选/页码） |
| 删除成功 | 从列表移除 + 刷新 |
| 收藏切换 | 即时更新卡片星标状态 |

---

## 九、实现检查清单

- [ ] Proto：`ToggleFavorite` RPC 已添加到 `agent.proto`
- [ ] `make api` 已执行，Go + TS 生成物已提交
- [ ] Biz：`AgentUsecase.ToggleFavorite` 已实现
- [ ] Data：`SearchAgents` 已支持 keyword/status/provider/category_id 筛选
- [ ] Service：`AgentService.ToggleFavorite` 已实现，含 `sql.ErrNoRows` → `kerrors.NotFound`
- [ ] Web：`api.ts` 新增 `toggleFavorite` 调用
- [ ] Web：`useAgentsPage.ts` composable 完整
- [ ] Web：`AgentListPage.vue` 网格/列表切换 + 筛选 + 分页
- [ ] Web：`AgentCard.vue` 卡片完整字段 + 收藏 + 删除确认
- [ ] Wire：`NewAgentService` 注入无需变更
