# Agent 行业分类模块 — 实现设计文档

> 本文档为**设计文档**，描述架构分层、Proto/API 契约、数据模型、接口定义、前端组件设计与 UX 规范。
> 用户故事、功能需求、验收标准见 [4-agent-type.md](./4-agent-type.md)。
> 代码锚点、现状评估、任务清单见 [4-agent-type.development.md](./4-agent-type.development.md)。

---

## 一、模块概述

Agent 业务分类体系：公司 → 部门 → 职位 三层自关联树。用户可自建分类节点，Agent 绑定叶子（职位）节点。与 `agents.agent_type`（技术类型）区分：`position_key` 是业务画像，用于展示、筛选、推荐与系统提示词上下文注入。

**命名演进**：历史称「行业分类」（level 值 `industry`），已通过 `organization_redesign_migrate.go` 迁移为「组织架构」（level 值 `company`）。Proto 服务名保留 `taxonomy`，Biz 层实体名为 `Organization`。

---

## 二、架构分层

```
api/kratos/taxonomy/v1/taxonomy.proto          ← Proto 契约（TaxonomyService）
api/kratos/industry_taxonomy/v1/               ← 遗留 Proto（IndustryTaxonomyService，前端仍引用）
        ↓
internal/service/taxonomy.go                   ← Service 层（TaxonomyNode ↔ OrganizationNode 转换）
        ↓
internal/biz/organization.go                   ← Biz 层（OrganizationUsecase + OrganizationRepo 接口）
internal/biz/organization_position_prompt.go   ← Biz 层（PositionPromptUsecase：层级校验/提示词构建）
        ↓
internal/data/organization_repo.go             ← Data 层（Repo 实现）
internal/data/ent/schema/organization.go       ← Ent Schema（表 organizations）
internal/data/ent/schema/agent.go              ← Agent 表关联字段（position_key/agent_variant）
        ↓
internal/agent/prompt.go                       ← 运行时（BuildIndustryContext 注入系统提示词）
```

**Wire 注入**：`data.ProviderSet → NewOrganizationRepo`、`biz.ProviderSet → NewOrganizationUsecase`、`service.ProviderSet → NewTaxonomyService`。

---

## 三、Proto 契约

### 3.1 主 Proto：`api/kratos/taxonomy/v1/taxonomy.proto`

```protobuf
message TaxonomyNode {
  string id = 1;
  string key = 2;
  string name = 3;
  string description = 4;
  string status = 5;
  bool enabled = 6;
  int32 sort_order = 7;
  string parent_id = 8;
  string level = 9;           // "company" | "department" | "position"
  string workspace_id = 10;
  string owner_user_id = 11;
  bool is_system = 12;
  string config_json = 13;
  string metadata_json = 14;
  string created_at = 15;
  string updated_at = 16;
  string deleted_at = 17;
}

service TaxonomyService {
  rpc ListTaxonomy(Empty) returns (ListTaxonomyResponse)           // GET /v1/taxonomy
  rpc ListTaxonomyTree(Empty) returns (ListTaxonomyTreeResponse)   // GET /v1/taxonomy/tree
  rpc CreateTaxonomy(CreateTaxonomyRequest) returns (TaxonomyNode) // POST /v1/taxonomy
  rpc GetTaxonomy(GetTaxonomyRequest) returns (TaxonomyNode)       // GET /v1/taxonomy/{id}
  rpc UpdateTaxonomy(UpdateTaxonomyRequest) returns (TaxonomyNode) // PATCH /v1/taxonomy/{id}
  rpc DeleteTaxonomy(DeleteTaxonomyRequest) returns (Empty)        // DELETE /v1/taxonomy/{id}
  rpc ReorderTaxonomy(ReorderTaxonomyRequest) returns (ReorderTaxonomyResponse) // PUT /v1/taxonomy/reorder
}
```

### 3.2 遗留 Proto：`api/kratos/industry_taxonomy/v1/industry_taxonomy.proto`

`IndustryTaxonomyService`（HTTP `/v1/industry-taxonomy`），消息体 `IndustryTaxonomy` 字段与 `TaxonomyNode` 同构。前端 `web/src/features/platform/api.ts` 中 `taxonomy-nodes` 资源仍走此服务，`taxonomy` 资源走主 Proto。两者最终都映射到同一 Biz 层 `OrganizationUsecase`。

### 3.3 API 端点表

| 方法 | 路径 | RPC | 说明 |
|------|------|-----|------|
| GET | `/v1/taxonomy` | `ListTaxonomy` | 扁平列表 |
| GET | `/v1/taxonomy/tree` | `ListTaxonomyTree` | 树形结构 |
| POST | `/v1/taxonomy` | `CreateTaxonomy` | 创建节点 |
| GET | `/v1/taxonomy/{id}` | `GetTaxonomy` | 获取单个 |
| PATCH | `/v1/taxonomy/{id}` | `UpdateTaxonomy` | 更新节点 |
| DELETE | `/v1/taxonomy/{id}` | `DeleteTaxonomy` | 软删除 |
| PUT | `/v1/taxonomy/reorder` | `ReorderTaxonomy` | 同级排序 |

> 注：`level` 可由服务端根据 `parent_id` 推导，避免客户端传错。

---

## 四、数据模型

### 4.1 Ent Schema：`internal/data/ent/schema/organization.go`

表名 `organizations`（单表自关联表达树），字段 `org_key`（唯一标识）。

| Ent 字段 | 类型 | 默认 | 说明 |
|---------|------|------|------|
| `id` | String | — | 主键，应用层生成，MaxLen 256 |
| `org_key` | String | — | 唯一标识，MaxLen 512 |
| `name` | String | — | 展示名称，MaxLen 1024 |
| `description` | Text | `""` | 描述 |
| `status` | String | `"active"` | 状态 |
| `enabled` | Bool | `true` | 是否启用 |
| `sort_order` | Int | `0` | 同级排序 |
| `parent_id` | String | `""` | 父节点 ID；公司为空 |
| `level` | String | `""` | `company`/`department`/`position` |
| `scenario_key` | String | `""` | 场景键 |
| `workspace_id` | String | `""` | 工作区隔离 |
| `owner_user_id` | String | `""` | 自建节点创建者 |
| `is_system` | Bool | `false` | 官方预置 |
| `config_json` | Text | `""` | 扩展配置 |
| `metadata_json` | Text | `""` | 元数据 |
| `dept_lead_agent_id` | String | `""` | 部门负责人 Agent ID（仅部门层） |
| `dept_lead_config_json` | Text | `"{}"` | 部门负责人配置覆盖 |
| `created_at` / `updated_at` / `deleted_at` | String | `""` | 时间戳（ISO8601 文本） |

**索引**：`idx_org_parent`(parent_id, sort_order)、`idx_org_level`(level, sort_order)。

### 4.2 Agent 表关联：`internal/data/ent/schema/agent.go`

| Ent 字段 | 类型 | 默认 | 说明 |
|---------|------|------|------|
| `position_key` | String | `""` | FK 到职位节点的 `org_key` |
| `agent_variant` | String | `"general"` | 职位方向变体：`general`/`code_review`/`architect`/... |
| `variant_description` | Text | `""` | 变体的人类可读描述 |

**唯一索引**：`(position_key, agent_variant)`。

> 注：Agent 通过 `position_key`（职位的 `org_key`）关联，非外键 ID；职位节点改名不影响关联。

### 4.3 层级校验状态机

`PositionPromptUsecase.normalizeOrg`（`internal/biz/organization_position_prompt.go`）实现层级约束：

| 父节点 level | 子节点允许 level | 推导规则 |
|-------------|-----------------|---------|
| 无（root） | `company` | `parent_id` 为空时，level 必须为 `company` |
| `company` | `department` | 父为公司时，level 必须为 `department` |
| `department` | `position` | 父为部门时，level 必须为 `position` |
| `position` | （拒绝） | 职位节点不能有子节点 |

非法层级返回 `ErrOrgBadRequest`。

### 4.4 软删除与引用约束

| 操作 | 规则 |
|------|------|
| 删除公司/部门 | 若有未删除子节点，阻止删除 |
| 删除职位 | 若有 Agent 引用（`agents.position_key` 指向该节点 `org_key`），阻止删除并提示数量 |
| 软删除 | 设置 `deleted_at` + `status="deleted"`，查询时过滤 `deleted_at=""` |

---

## 五、Biz 层接口与实现

### 5.1 领域模型（`internal/biz/organization.go`）

```go
type OrganizationNode struct {
    ID, Key, Name, Description, Status string
    Enabled bool
    SortOrder int
    ParentID, Level, ScenarioKey, WorkspaceID, OwnerUserID string
    IsSystem bool
    ConfigJSON, MetadataJSON string
    DeptLeadAgentID, DeptLeadConfigJSON string  // 部门负责人
    CreatedAt, UpdatedAt, DeletedAt string
}

type OrganizationTreeNode struct {
    Category OrganizationNode
    Children []OrganizationTreeNode
}

type OrgAncestors struct {
    Company, Department, Position OrganizationNode
}
```

### 5.2 Repo 接口（Stability:stable）

```go
type OrganizationReader interface {
    GetOrgNode(ctx, id) (OrganizationNode, error)
    GetOrgNodeByKey(ctx, key) (OrganizationNode, error)
    ListOrgNodes(ctx) ([]OrganizationNode, error)
    ListOrgNodesByLevel(ctx, level) ([]OrganizationNode, error)
    ListOrgNodesByParentID(ctx, parentID) ([]OrganizationNode, error)
}

type OrganizationWriter interface {
    CreateOrgNode(ctx, c) (OrganizationNode, error)
    UpdateOrgNode(ctx, c) (OrganizationNode, error)
    DeleteOrgNode(ctx, id) error
    ReorderOrgNodes(ctx, ids) error
}

type OrganizationRepo interface {
    OrganizationReader
    OrganizationWriter
    GetOrgNodeByKeyAnyState(ctx, key) (OrganizationNode, error)  // 含软删
}
```

辅助接口：`DeptTeamLister`（按部门列出团队）、`DeptAgentPositionClearer`（清除部门下 Agent 职位关联）。

### 5.3 Usecase

`OrganizationUsecase`（依赖 `repo`、`deptLeadMgr`、`teamLister`、`teamWriter`、`agentClear`、`posPrompt`、`eventBus`、`lg`）：
- `List` — 扁平列表
- `Tree` — 构建树（按 `parent_id` 组装递归结构）
- `Get` / `Create` / `Update` / `Delete` / `Reorder` — CRUD（Create/Update 经 `normalizeOrg` 校验层级）
- 部门负责人管理（`deptLeadMgr`）

`PositionPromptUsecase`（`internal/biz/organization_position_prompt.go`）：
- `normalizeOrg` — 层级校验（见 §4.3）
- `GetPositionPrompt` — 根据职位 + 变体构建提示词内容
- `ListPositionVariants` — 列出职位变体
- `BuildResponsibility` — 构建职责描述
- `GetAncestors` — 解析 company→department→position 祖先链

---

## 六、Service 层（`internal/service/taxonomy.go`）

`TaxonomyService` 包装 `OrganizationUsecase`，负责 Proto 消息与 Biz 模型互转：

| 转换函数 | 方向 |
|---------|------|
| `toProtoTaxonomy` | `biz.OrganizationNode` → `v1.TaxonomyNode` |
| `fromProtoTaxonomy` | `v1.TaxonomyNode` → `biz.OrganizationNode` |
| `toTaxonomyTree` / `toTaxonomyTreeNode` | 树结构互转 |

RPC 实现委托给 `OrganizationUsecase` 的对应方法。

---

## 七、行业上下文注入

`internal/agent/prompt.go` 的 `BuildIndustryContext(ctx, d, ag)`：

```
Agent.PositionKey → d.Organization.GetByKey(position_key) → 职位节点
    → GetAncestors(position_id) → 公司/部门/职位祖先链
    → 拼接名称与描述 → 注入系统提示词
```

- 未绑定职位（`PositionKey` 为空）时不注入。
- 注入内容包括公司名/描述、部门名/职责、职位名/职责、变体描述。

---

## 八、前端组件设计

### 8.1 页面与路由

| 文件 | 职责 | 路由 |
|------|------|------|
| `web/src/pages/OrganizationPage.vue` | 分类管理页 | `/settings/taxonomy` |

### 8.2 Composable 与工具

| 文件 | 职责 |
|------|------|
| `web/src/features/platform/useTaxonomyPage.ts` | 管理页状态（列表/树/CRUD/搜索） |
| `web/src/features/platform/useTaxonomyTreeField.ts` | 树字段联动 |
| `web/src/features/platform/taxonomyTreeUtils.ts` | 树工具（flatten/filter/patch） |
| `web/src/features/platform/taxonomyLabels.ts` | 标签映射（level → 数字 1/2/3，描述标签） |
| `web/src/features/platform/api.ts` | 平台资源分发（`listPlatformResourceTree` 等） |
| `web/src/components/agents/agentUi.ts` | `flattenTaxonomyPositions` 展平职位选项 |

### 8.3 组件清单

| 组件 | 职责 |
|------|------|
| `TaxonomyPicker.vue` | 三级级联选择器（创建/编辑 Agent 时选职位） |
| `TaxonomyFilter.vue` | 列表筛选组件（基于 TaxonomyPicker） |
| `TaxonomyTree.vue` | 树形展示 |
| `TaxonomyTreeNodeHeader.vue` | 树节点头部 |
| `TaxonomyNodeHeader.vue` | 节点头部 |
| `TaxonomyIndustryCard.vue` | 公司卡片 |
| `TaxonomyDepartmentNode.vue` | 部门节点 |
| `TaxonomyPositionCard.vue` | 职位卡片 |
| `TaxonomyNodeDialog.vue` | 节点编辑弹窗 |

### 8.4 API 分发（`web/src/features/platform/api.ts`）

前端按 `PlatformResourceName` 分发至不同 Kratos 客户端：
- `taxonomy` → `createTaxonomyService()`（主 Proto）
- `taxonomy-nodes` → `createIndustryTaxonomyService()`（遗留 Proto）
- `organization` → `createOrganizationService()`

`listPlatformResourceTree(resource)` 根据资源类型调用对应 `List*Tree` RPC，并统一映射为 `PlatformResourceTreeNode`。

### 8.5 UX 规范

| level 值 | 数字 | UI 标签 | 描述字段标签 |
|---------|------|---------|------------|
| `company` | 1 | 公司 | 公司说明 |
| `department` | 2 | 部门 | 部门职责 |
| `position` | 3 | 职位 | 岗位职责 |

页面标题「组织架构」，副标题「按公司、部门、职位三层组织 Agent 业务画像」。

---

## 九、与 `agents.agent_type` 的关系

| 字段 | 含义 |
|------|------|
| `agents.agent_type` | 技术/开放类型（如 `open`），用于能力、API 策略等 |
| `agents.position_key` | 业务画像：公司-部门-职位，用于展示、筛选、推荐、提示词注入 |
| `agents.agent_variant` | 职位方向变体，同一职位的不同侧重（通用/代码评审/架构） |

列表页筛选可同时支持 `agent_type` 与分类（按公司/部门/职位路径）。

---

## 十、种子数据示例

```text
某科技公司 (is_system=true)
  ├── 游戏开发部
  │     └── UE5场景设计师
  └── 系统开发部
        └── golang后端高级工程师
```

导入脚本将 `is_system = true`，`workspace_id` 按产品设为全局或模板工作区。

---

*文档版本：设计边界；需求见 `4-agent-type.md`，开发计划见 `4-agent-type.development.md`。*
