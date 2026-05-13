# Agent 行业分类模块 — 实现设计文档

> 对应需求：`4.agent-type.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 业务分类体系：行业 → 部门 → 职位 三层自关联树。用户可自建分类节点，Agent 绑定叶子（职位）节点。与 `agents.agent_type`（技术类型）区分：`category_position_id` 是业务画像，用于展示、筛选、推荐。

---

## 二、Proto 层

### 2.1 完整 Proto 定义

文件：`api/kratos/agent_category/v1/agent_category.proto`

```protobuf
syntax = "proto3";

package kratos.agent_category.v1;

import "google/api/annotations.proto";
import "google/api/field_behavior.proto";
import "google/protobuf/empty.proto";

option go_package = "aranea-agents/api/kratos/agent_category/v1;v1";

message AgentCategory {
  string id = 1;
  string key = 2;
  string name = 3;
  string description = 4;
  string status = 5;
  bool enabled = 6;
  int32 sort_order = 7;
  string parent_id = 8;
  string level = 9;           // "industry" | "department" | "position"
  string workspace_id = 10;
  string owner_user_id = 11;
  bool is_system = 12;
  string config_json = 13;
  string metadata_json = 14;
  string created_at = 15;
  string updated_at = 16;
  string deleted_at = 17;
}

message ListAgentCategoriesResponse {
  repeated AgentCategory items = 1;
}

message ListAgentCategoryTreeResponse {
  repeated AgentCategoryTreeNode items = 1;
}

message AgentCategoryTreeNode {
  AgentCategory category = 1;
  repeated AgentCategoryTreeNode children = 2;
}

message CreateAgentCategoryRequest {
  string key = 1 [(google.api.field_behavior) = REQUIRED];
  string name = 2 [(google.api.field_behavior) = REQUIRED];
  string description = 3;
  string status = 4;
  bool enabled = 5;
  int32 sort_order = 6;
  string parent_id = 7;
  string level = 8;
  string workspace_id = 9;
  string owner_user_id = 10;
  string config_json = 11;
  string metadata_json = 12;
}

message GetAgentCategoryRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

message UpdateAgentCategoryRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  AgentCategory category = 2;
}

message DeleteAgentCategoryRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

service AgentCategoryService {
  rpc ListAgentCategories(google.protobuf.Empty) returns (ListAgentCategoriesResponse) {
    option (google.api.http) = { get: "/v1/agent-categories" };
  }
  rpc ListAgentCategoryTree(google.protobuf.Empty) returns (ListAgentCategoryTreeResponse) {
    option (google.api.http) = { get: "/v1/agent-categories/tree" };
  }
  rpc CreateAgentCategory(CreateAgentCategoryRequest) returns (AgentCategory) {
    option (google.api.http) = { post: "/v1/agent-categories" body: "*" };
  }
  rpc GetAgentCategory(GetAgentCategoryRequest) returns (AgentCategory) {
    option (google.api.http) = { get: "/v1/agent-categories/{id}" };
  }
  rpc UpdateAgentCategory(UpdateAgentCategoryRequest) returns (AgentCategory) {
    option (google.api.http) = { patch: "/v1/agent-categories/{id}" body: "category" };
  }
  rpc DeleteAgentCategory(DeleteAgentCategoryRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/agent-categories/{id}" };
  }
}
```

### 2.2 Proto 字段说明

| 消息 | 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `AgentCategory` | `id` | string | — | 主键，应用层生成 |
| | `key` | string | — | 唯一标识，用于 URL/导入导出 |
| | `name` | string | — | 展示名称，如 `IT行业`、`游戏开发部` |
| | `description` | string | — | 描述 |
| | `status` | string | — | 状态：`active`/`deleted` |
| | `enabled` | bool | — | 是否启用 |
| | `sort_order` | int32 | — | 同级排序，越小越靠前 |
| | `parent_id` | string | — | 父节点 ID；行业为空 |
| | `level` | string | — | `industry`/`department`/`position` |
| | `workspace_id` | string | — | 工作区隔离 |
| | `owner_user_id` | string | — | 自建节点创建者 |
| | `is_system` | bool | — | 官方预置分类 |
| | `config_json` | string | — | 扩展配置 |
| | `metadata_json` | string | — | 元数据 |
| `CreateAgentCategoryRequest` | `key` | string | ✅ | 唯一标识 |
| | `name` | string | ✅ | 展示名称 |
| | `level` | string | — | 层级；可由 parent_id 推导 |
| | `parent_id` | string | — | 父节点；行业为空 |
| `AgentCategoryTreeNode` | `category` | AgentCategory | — | 当前节点 |
| | `children` | AgentCategoryTreeNode[] | — | 子节点递归 |

---

## 三、Biz 层

### 3.1 领域模型

文件：`internal/biz/agent_category.go`

```go
type AgentCategory struct {
    ID           string
    Key          string
    Name         string
    Description  string
    Status       string
    Enabled      bool
    SortOrder    int
    ParentID     string
    Level        string  // "industry" | "department" | "position"
    WorkspaceID  string
    OwnerUserID  string
    IsSystem     bool
    ConfigJSON   string
    MetadataJSON string
    CreatedAt    string
    UpdatedAt    string
    DeletedAt    string
}

type AgentCategoryTreeNode struct {
    Category AgentCategory
    Children []AgentCategoryTreeNode
}
```

### 3.2 Repo 接口

文件：`internal/biz/agent_category.go`

```go
type AgentCategoryRepo interface {
    ListAgentCategories(ctx context.Context) ([]AgentCategory, error)
    GetAgentCategory(ctx context.Context, id string) (AgentCategory, error)
    CreateAgentCategory(ctx context.Context, c AgentCategory) (AgentCategory, error)
    UpdateAgentCategory(ctx context.Context, c AgentCategory) (AgentCategory, error)
    DeleteAgentCategory(ctx context.Context, id string) error
}
```

### 3.3 Usecase 实现

文件：`internal/biz/agent_category.go`

```go
type AgentCategoryUsecase struct {
    repo AgentCategoryRepo
}

func NewAgentCategoryUsecase(repo AgentCategoryRepo) *AgentCategoryUsecase {
    return &AgentCategoryUsecase{repo: repo}
}
```

#### 3.3.1 List — 扁平列表

```go
func (u *AgentCategoryUsecase) List(ctx context.Context) ([]AgentCategory, error) {
    return u.repo.ListAgentCategories(ctx)
}
```

#### 3.3.2 Tree — 构建树

```go
func (u *AgentCategoryUsecase) Tree(ctx context.Context) ([]AgentCategoryTreeNode, error) {
    items, err := u.repo.ListAgentCategories(ctx)
    if err != nil {
        return nil, err
    }
    nodes := make(map[string]AgentCategory, len(items))
    order := make([]string, 0, len(items))
    for _, item := range items {
        nodes[item.ID] = item
        order = append(order, item.ID)
    }
    childrenByParent := make(map[string][]string, len(items))
    rootIDs := make([]string, 0)
    for _, id := range order {
        node := nodes[id]
        if node.ParentID != "" {
            if _, ok := nodes[node.ParentID]; ok {
                childrenByParent[node.ParentID] = append(childrenByParent[node.ParentID], id)
                continue
            }
        }
        rootIDs = append(rootIDs, id)
    }
    var buildNode func(string) AgentCategoryTreeNode
    buildNode = func(id string) AgentCategoryTreeNode {
        n := AgentCategoryTreeNode{Category: nodes[id]}
        for _, childID := range childrenByParent[id] {
            n.Children = append(n.Children, buildNode(childID))
        }
        return n
    }
    roots := make([]AgentCategoryTreeNode, 0, len(rootIDs))
    for _, id := range rootIDs {
        roots = append(roots, buildNode(id))
    }
    return roots, nil
}
```

#### 3.3.3 Get — 获取单个

```go
func (u *AgentCategoryUsecase) Get(ctx context.Context, id string) (AgentCategory, error) {
    if strings.TrimSpace(id) == "" {
        return AgentCategory{}, ErrCategoryBadRequest("id is required")
    }
    return u.repo.GetAgentCategory(ctx, id)
}
```

#### 3.3.4 Create — 创建（含层级校验）

```go
func (u *AgentCategoryUsecase) Create(ctx context.Context, in AgentCategory) (AgentCategory, error) {
    in.Key = strings.TrimSpace(in.Key)
    in.Name = strings.TrimSpace(in.Name)
    if in.Key == "" || in.Name == "" {
        return AgentCategory{}, ErrCategoryBadRequest("key and name are required")
    }
    if in.ID == "" {
        in.ID = newRandID()
    }
    if in.Status == "" {
        in.Status = "active"
    }
    if err := u.normalizeAgentCategory(ctx, &in); err != nil {
        return AgentCategory{}, err
    }
    return u.repo.CreateAgentCategory(ctx, in)
}
```

#### 3.3.5 Update — 合并更新

```go
func (u *AgentCategoryUsecase) Update(ctx context.Context, id string, patch AgentCategory) (AgentCategory, error) {
    if strings.TrimSpace(id) == "" {
        return AgentCategory{}, ErrCategoryBadRequest("id is required")
    }
    current, err := u.repo.GetAgentCategory(ctx, id)
    if err != nil {
        return AgentCategory{}, err
    }
    merged := current
    patch.ID = id
    if patch.Key != "" {
        merged.Key = patch.Key
    }
    if patch.Name != "" {
        merged.Name = patch.Name
    }
    if patch.Status != "" {
        merged.Status = patch.Status
    }
    merged.Description = patch.Description
    merged.Enabled = patch.Enabled
    merged.SortOrder = patch.SortOrder
    merged.ParentID = patch.ParentID
    merged.Level = patch.Level
    merged.WorkspaceID = patch.WorkspaceID
    merged.OwnerUserID = patch.OwnerUserID
    merged.IsSystem = patch.IsSystem
    merged.ConfigJSON = patch.ConfigJSON
    merged.MetadataJSON = patch.MetadataJSON
    if err := u.normalizeAgentCategory(ctx, &merged); err != nil {
        return AgentCategory{}, err
    }
    return u.repo.UpdateAgentCategory(ctx, merged)
}
```

#### 3.3.6 Delete — 软删除（含引用检查）

```go
func (u *AgentCategoryUsecase) Delete(ctx context.Context, id string) error {
    if strings.TrimSpace(id) == "" {
        return ErrCategoryBadRequest("id is required")
    }
    return u.repo.DeleteAgentCategory(ctx, id)
}
```

#### 3.3.7 normalizeAgentCategory — 层级校验

```go
func (u *AgentCategoryUsecase) normalizeAgentCategory(ctx context.Context, in *AgentCategory) error {
    if strings.TrimSpace(in.ParentID) == "" {
        in.ParentID = ""
        if in.Level == "" {
            in.Level = "industry"
        }
        if in.Level != "industry" {
            return errors.BadRequest("CATEGORY_LEVEL", "industry category must not have parent_id")
        }
        return nil
    }
    parent, err := u.repo.GetAgentCategory(ctx, in.ParentID)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return errors.BadRequest("CATEGORY_PARENT", "parent category not found")
        }
        return err
    }
    switch parent.Level {
    case "industry":
        if in.Level == "" {
            in.Level = "department"
        }
        if in.Level != "department" {
            return errors.BadRequest("CATEGORY_LEVEL", "industry children must be department")
        }
    case "department":
        if in.Level == "" {
            in.Level = "position"
        }
        if in.Level != "position" {
            return errors.BadRequest("CATEGORY_LEVEL", "department children must be position")
        }
    case "position":
        return errors.BadRequest("CATEGORY_LEVEL", "position category cannot have children")
    default:
        return errors.BadRequest("CATEGORY_LEVEL", "parent category level is invalid")
    }
    return nil
}
```

---

## 四、Data 层

### 4.1 Ent Schema

文件：`internal/data/ent/schema/agent_category.go`

```go
package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/dialect/entsql"
    "entgo.io/ent/schema"
    "entgo.io/ent/schema/field"
)

type AgentCategory struct {
    ent.Schema
}

func (AgentCategory) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entsql.Annotation{Table: "agent_category_nodes"},
    }
}

func (AgentCategory) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("category_key").Unique().MaxLen(512),
        field.String("name").MaxLen(1024),
        field.Text("description").Default(""),
        field.String("status").Default("active"),
        field.Bool("enabled").Default(true),
        field.Int("sort_order").Default(0),
        field.String("parent_id").Default(""),
        field.String("level").Default(""),
        field.String("workspace_id").Default(""),
        field.String("owner_user_id").Default(""),
        field.Bool("is_system").Default(false),
        field.Text("config_json").Default(""),
        field.Text("metadata_json").Default(""),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
        field.String("deleted_at").Default(""),
    }
}
```

### 4.2 Repo 实现

文件：`internal/data/agent_category.go`

```go
package data

type agentCategoryRepo struct {
    data *Data
}

func NewAgentCategoryRepo(d *Data) biz.AgentCategoryRepo {
    return &agentCategoryRepo{data: d}
}
```

#### 4.2.1 Ent → Biz 类型转换

```go
func entToBizCat(e *ent.AgentCategory) biz.AgentCategory {
    if e == nil {
        return biz.AgentCategory{}
    }
    return biz.AgentCategory{
        ID:           e.ID,
        Key:          e.CategoryKey,
        Name:         e.Name,
        Description:  e.Description,
        Status:       e.Status,
        Enabled:      e.Enabled,
        SortOrder:    e.SortOrder,
        ParentID:     e.ParentID,
        Level:        e.Level,
        WorkspaceID:  e.WorkspaceID,
        OwnerUserID:  e.OwnerUserID,
        IsSystem:     e.IsSystem,
        ConfigJSON:   e.ConfigJSON,
        MetadataJSON: e.MetadataJSON,
        CreatedAt:    e.CreatedAt,
        UpdatedAt:    e.UpdatedAt,
        DeletedAt:    e.DeletedAt,
    }
}
```

#### 4.2.2 ListAgentCategories — 扁平查询

```go
func (r *agentCategoryRepo) ListAgentCategories(ctx context.Context) ([]biz.AgentCategory, error) {
    rows, err := r.data.entClient.AgentCategory.Query().
        Where(agentcategory.DeletedAtEQ("")).
        Order(
            agentcategory.BySortOrder(),
            agentcategory.ByCreatedAt(entsql.OrderDesc()),
        ).
        All(ctx)
    if err != nil {
        return nil, err
    }
    out := make([]biz.AgentCategory, 0, len(rows))
    for _, e := range rows {
        out = append(out, entToBizCat(e))
    }
    return out, nil
}
```

#### 4.2.3 GetAgentCategory — 按 ID 查询

```go
func (r *agentCategoryRepo) GetAgentCategory(ctx context.Context, id string) (biz.AgentCategory, error) {
    row, err := r.data.entClient.AgentCategory.Query().
        Where(
            agentcategory.IDEQ(id),
            agentcategory.DeletedAtEQ(""),
        ).
        Only(ctx)
    if err != nil {
        if ent.IsNotFound(err) {
            return biz.AgentCategory{}, sql.ErrNoRows
        }
        return biz.AgentCategory{}, err
    }
    return entToBizCat(row), nil
}
```

#### 4.2.4 CreateAgentCategory — 创建

```go
func (r *agentCategoryRepo) CreateAgentCategory(ctx context.Context, c biz.AgentCategory) (biz.AgentCategory, error) {
    now := nowRFC3339()
    if c.CreatedAt == "" {
        c.CreatedAt = now
    }
    c.UpdatedAt = now
    saved, err := r.data.entClient.AgentCategory.Create().
        SetID(c.ID).
        SetCategoryKey(c.Key).
        SetName(c.Name).
        SetDescription(c.Description).
        SetStatus(c.Status).
        SetEnabled(c.Enabled).
        SetSortOrder(c.SortOrder).
        SetParentID(c.ParentID).
        SetLevel(c.Level).
        SetWorkspaceID(c.WorkspaceID).
        SetOwnerUserID(c.OwnerUserID).
        SetIsSystem(c.IsSystem).
        SetConfigJSON(c.ConfigJSON).
        SetMetadataJSON(c.MetadataJSON).
        SetCreatedAt(c.CreatedAt).
        SetUpdatedAt(c.UpdatedAt).
        SetDeletedAt("").
        Save(ctx)
    if err != nil {
        return biz.AgentCategory{}, err
    }
    return entToBizCat(saved), nil
}
```

#### 4.2.5 UpdateAgentCategory — 更新

```go
func (r *agentCategoryRepo) UpdateAgentCategory(ctx context.Context, c biz.AgentCategory) (biz.AgentCategory, error) {
    c.UpdatedAt = nowRFC3339()
    err := r.data.entClient.AgentCategory.UpdateOneID(c.ID).
        SetCategoryKey(c.Key).
        SetName(c.Name).
        SetDescription(c.Description).
        SetStatus(c.Status).
        SetEnabled(c.Enabled).
        SetSortOrder(c.SortOrder).
        SetParentID(c.ParentID).
        SetLevel(c.Level).
        SetWorkspaceID(c.WorkspaceID).
        SetOwnerUserID(c.OwnerUserID).
        SetIsSystem(c.IsSystem).
        SetConfigJSON(c.ConfigJSON).
        SetMetadataJSON(c.MetadataJSON).
        SetUpdatedAt(c.UpdatedAt).
        Exec(ctx)
    if err != nil {
        return biz.AgentCategory{}, err
    }
    return r.GetAgentCategory(ctx, c.ID)
}
```

#### 4.2.6 DeleteAgentCategory — 软删除（含引用检查）

```go
func (r *agentCategoryRepo) DeleteAgentCategory(ctx context.Context, id string) error {
    if err := r.ensureCategoryCanDelete(ctx, id); err != nil {
        return err
    }
    now := nowRFC3339()
    return r.data.entClient.AgentCategory.UpdateOneID(id).
        SetDeletedAt(now).
        SetStatus("deleted").
        SetUpdatedAt(now).
        Exec(ctx)
}

func (r *agentCategoryRepo) ensureCategoryCanDelete(ctx context.Context, id string) error {
    n, err := r.data.entClient.AgentCategory.Query().
        Where(
            agentcategory.ParentIDEQ(id),
            agentcategory.DeletedAtEQ(""),
        ).
        Count(ctx)
    if err != nil {
        return err
    }
    if n > 0 {
        return fmt.Errorf("category has %d child nodes", n)
    }
    nAgents, err := r.data.entClient.Agent.Query().
        Where(
            agent.CategoryPositionIDEQ(id),
            agent.DeletedAtEQ(""),
        ).
        Count(ctx)
    if err != nil {
        return err
    }
    if nAgents > 0 {
        return fmt.Errorf("category is used by %d agents", nAgents)
    }
    return nil
}
```

---

## 五、Service 层

文件：`internal/service/agent_category.go`

### 5.1 Service 结构体

```go
type AgentCategoryService struct {
    v1.UnimplementedAgentCategoryServiceServer
    uc *biz.AgentCategoryUsecase
}

func NewAgentCategoryService(uc *biz.AgentCategoryUsecase) *AgentCategoryService {
    return &AgentCategoryService{uc: uc}
}
```

### 5.2 类型转换函数

```go
func toProtoCat(c biz.AgentCategory) *v1.AgentCategory {
    return &v1.AgentCategory{
        Id:           c.ID,
        Key:          c.Key,
        Name:         c.Name,
        Description:  c.Description,
        Status:       c.Status,
        Enabled:      c.Enabled,
        SortOrder:    int32(c.SortOrder),
        ParentId:     c.ParentID,
        Level:        c.Level,
        WorkspaceId:  c.WorkspaceID,
        OwnerUserId:  c.OwnerUserID,
        IsSystem:     c.IsSystem,
        ConfigJson:   c.ConfigJSON,
        MetadataJson: c.MetadataJSON,
        CreatedAt:    c.CreatedAt,
        UpdatedAt:    c.UpdatedAt,
        DeletedAt:    c.DeletedAt,
    }
}

func fromProtoCat(pb *v1.AgentCategory) biz.AgentCategory {
    if pb == nil {
        return biz.AgentCategory{}
    }
    return biz.AgentCategory{
        ID:           pb.GetId(),
        Key:          pb.GetKey(),
        Name:         pb.GetName(),
        Description:  pb.GetDescription(),
        Status:       pb.GetStatus(),
        Enabled:      pb.GetEnabled(),
        SortOrder:    int(pb.GetSortOrder()),
        ParentID:     pb.GetParentId(),
        Level:        pb.GetLevel(),
        WorkspaceID:  pb.GetWorkspaceId(),
        OwnerUserID:  pb.GetOwnerUserId(),
        IsSystem:     pb.GetIsSystem(),
        ConfigJSON:   pb.GetConfigJson(),
        MetadataJSON: pb.GetMetadataJson(),
        CreatedAt:    pb.GetCreatedAt(),
        UpdatedAt:    pb.GetUpdatedAt(),
        DeletedAt:    pb.GetDeletedAt(),
    }
}

func toProtoTree(nodes []biz.AgentCategoryTreeNode) []*v1.AgentCategoryTreeNode {
    out := make([]*v1.AgentCategoryTreeNode, 0, len(nodes))
    for i := range nodes {
        out = append(out, toProtoTreeNode(&nodes[i]))
    }
    return out
}

func toProtoTreeNode(n *biz.AgentCategoryTreeNode) *v1.AgentCategoryTreeNode {
    if n == nil {
        return nil
    }
    cat := toProtoCat(n.Category)
    children := make([]*v1.AgentCategoryTreeNode, 0, len(n.Children))
    for j := range n.Children {
        children = append(children, toProtoTreeNode(&n.Children[j]))
    }
    return &v1.AgentCategoryTreeNode{
        Category: cat,
        Children: children,
    }
}
```

### 5.3 RPC 实现

```go
func (s *AgentCategoryService) ListAgentCategories(ctx context.Context, _ *emptypb.Empty) (*v1.ListAgentCategoriesResponse, error) {
    items, err := s.uc.List(ctx)
    if err != nil {
        return nil, err
    }
    resp := &v1.ListAgentCategoriesResponse{Items: make([]*v1.AgentCategory, 0, len(items))}
    for i := range items {
        resp.Items = append(resp.Items, toProtoCat(items[i]))
    }
    return resp, nil
}

func (s *AgentCategoryService) ListAgentCategoryTree(ctx context.Context, _ *emptypb.Empty) (*v1.ListAgentCategoryTreeResponse, error) {
    nodes, err := s.uc.Tree(ctx)
    if err != nil {
        return nil, err
    }
    return &v1.ListAgentCategoryTreeResponse{Items: toProtoTree(nodes)}, nil
}

func (s *AgentCategoryService) CreateAgentCategory(ctx context.Context, req *v1.CreateAgentCategoryRequest) (*v1.AgentCategory, error) {
    in := biz.AgentCategory{
        Key:          req.GetKey(),
        Name:         req.GetName(),
        Description:  req.GetDescription(),
        Status:       req.GetStatus(),
        Enabled:      req.GetEnabled(),
        SortOrder:    int(req.GetSortOrder()),
        ParentID:     req.GetParentId(),
        Level:        req.GetLevel(),
        WorkspaceID:  req.GetWorkspaceId(),
        OwnerUserID:  req.GetOwnerUserId(),
        ConfigJSON:   req.GetConfigJson(),
        MetadataJSON: req.GetMetadataJson(),
    }
    created, err := s.uc.Create(ctx, in)
    if err != nil {
        return nil, err
    }
    return toProtoCat(created), nil
}

func (s *AgentCategoryService) GetAgentCategory(ctx context.Context, req *v1.GetAgentCategoryRequest) (*v1.AgentCategory, error) {
    c, err := s.uc.Get(ctx, req.GetId())
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, kerrors.NotFound("AGENT_CATEGORY", "category not found")
        }
        return nil, err
    }
    return toProtoCat(c), nil
}

func (s *AgentCategoryService) UpdateAgentCategory(ctx context.Context, req *v1.UpdateAgentCategoryRequest) (*v1.AgentCategory, error) {
    if req.GetCategory() == nil {
        return nil, kerrors.BadRequest("AGENT_CATEGORY", "category body is required")
    }
    patch := fromProtoCat(req.GetCategory())
    out, err := s.uc.Update(ctx, req.GetId(), patch)
    if err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return nil, kerrors.NotFound("AGENT_CATEGORY", "category not found")
        }
        return nil, err
    }
    return toProtoCat(out), nil
}

func (s *AgentCategoryService) DeleteAgentCategory(ctx context.Context, req *v1.DeleteAgentCategoryRequest) (*emptypb.Empty, error) {
    if err := s.uc.Delete(ctx, req.GetId()); err != nil {
        return nil, err
    }
    return &emptypb.Empty{}, nil
}
```

---

## 六、Wire 注入

已有，无需新增：

```
data.ProviderSet  → NewAgentCategoryRepo
biz.ProviderSet   → NewAgentCategoryUsecase
service.ProviderSet → NewAgentCategoryService
```

---

## 七、Web 前端设计

### 7.1 TypeScript 类型

文件：`web/src/features/agents/types.ts`（新增）

```typescript
export type AgentCategory = {
  id: string;
  key: string;
  name: string;
  description: string;
  status: string;
  enabled: boolean;
  sort_order: number;
  parent_id: string;
  level: "industry" | "department" | "position";
  workspace_id: string;
  owner_user_id: string;
  is_system: boolean;
  config_json: string;
  metadata_json: string;
  created_at: string;
  updated_at: string;
  deleted_at: string;
};

export type AgentCategoryTreeNode = {
  category: AgentCategory;
  children: AgentCategoryTreeNode[];
};

export type CreateCategoryRequest = {
  key: string;
  name: string;
  description?: string;
  status?: string;
  enabled?: boolean;
  sort_order?: number;
  parent_id?: string;
  level?: string;
  workspace_id?: string;
  owner_user_id?: string;
  config_json?: string;
  metadata_json?: string;
};
```

### 7.2 API 调用

文件：`web/src/features/agents/api.ts`（新增）

```typescript
import { createAgentCategoryService } from "../../services";

export async function listCategories(): Promise<AgentCategory[]> {
  const svc = createAgentCategoryService();
  const res = await svc.ListAgentCategories({});
  return (res.items ?? []).map(normalizeCategoryFromService);
}

export async function getCategoryTree(): Promise<AgentCategoryTreeNode[]> {
  const svc = createAgentCategoryService();
  const res = await svc.ListAgentCategoryTree({});
  return (res.items ?? []).map(normalizeTreeNodeFromService);
}

export async function createCategory(req: CreateCategoryRequest): Promise<AgentCategory> {
  const svc = createAgentCategoryService();
  const data = await svc.CreateAgentCategory({
    key: req.key,
    name: req.name,
    description: req.description,
    status: req.status,
    enabled: req.enabled,
    sortOrder: req.sort_order,
    parentId: req.parent_id,
    level: req.level,
    workspaceId: req.workspace_id,
    ownerUserId: req.owner_user_id,
    configJson: req.config_json,
    metadataJson: req.metadata_json,
  });
  return normalizeCategoryFromService(data);
}

export async function updateCategory(id: string, patch: Partial<AgentCategory>): Promise<AgentCategory> {
  const svc = createAgentCategoryService();
  const data = await svc.UpdateAgentCategory({
    id,
    category: partialCategoryToWire(patch),
  });
  return normalizeCategoryFromService(data);
}

export async function deleteCategory(id: string): Promise<void> {
  const svc = createAgentCategoryService();
  await svc.DeleteAgentCategory({ id });
}
```

### 7.3 数据规范化

文件：`web/src/features/agents/wireNormalize.ts`（新增）

```typescript
export function normalizeCategoryFromService(row: any): AgentCategory {
  return {
    id: row.id ?? "",
    key: row.key ?? "",
    name: row.name ?? "",
    description: row.description ?? "",
    status: row.status ?? "active",
    enabled: row.enabled ?? true,
    sort_order: row.sortOrder ?? 0,
    parent_id: row.parentId ?? "",
    level: row.level ?? "industry",
    workspace_id: row.workspaceId ?? "",
    owner_user_id: row.ownerUserId ?? "",
    is_system: row.isSystem ?? false,
    config_json: row.configJson ?? "",
    metadata_json: row.metadataJson ?? "",
    created_at: row.createdAt ?? "",
    updated_at: row.updatedAt ?? "",
    deleted_at: row.deletedAt ?? "",
  };
}

export function normalizeTreeNodeFromService(node: any): AgentCategoryTreeNode {
  return {
    category: normalizeCategoryFromService(node.category),
    children: (node.children ?? []).map(normalizeTreeNodeFromService),
  };
}

export function partialCategoryToWire(patch: Partial<AgentCategory>) {
  const wire: any = {};
  if (patch.key !== undefined) wire.key = patch.key;
  if (patch.name !== undefined) wire.name = patch.name;
  if (patch.description !== undefined) wire.description = patch.description;
  if (patch.status !== undefined) wire.status = patch.status;
  if (patch.enabled !== undefined) wire.enabled = patch.enabled;
  if (patch.sort_order !== undefined) wire.sortOrder = patch.sort_order;
  if (patch.parent_id !== undefined) wire.parentId = patch.parent_id;
  if (patch.level !== undefined) wire.level = patch.level;
  if (patch.workspace_id !== undefined) wire.workspaceId = patch.workspace_id;
  if (patch.owner_user_id !== undefined) wire.ownerUserId = patch.owner_user_id;
  if (patch.is_system !== undefined) wire.isSystem = patch.is_system;
  if (patch.config_json !== undefined) wire.configJson = patch.config_json;
  if (patch.metadata_json !== undefined) wire.metadataJson = patch.metadata_json;
  return wire;
}
```

### 7.4 组件设计

#### AgentCategoryCascade.vue — 三级级联选择器

用于 Agent 创建/编辑表单中的分类选择。

```vue
<template>
  <div class="row q-gutter-sm">
    <QSelect
      v-model="industryId"
      :options="industryOptions"
      label="行业"
      outlined
      dense
      emit-value
      map-options
      class="col"
      @update:model-value="onIndustryChange"
    />
    <QSelect
      v-model="departmentId"
      :options="departmentOptions"
      label="部门"
      outlined
      dense
      emit-value
      map-options
      class="col"
      :disable="!industryId"
      @update:model-value="onDepartmentChange"
    />
    <QSelect
      v-model="positionId"
      :options="positionOptions"
      label="职位"
      outlined
      dense
      emit-value
      map-options
      class="col"
      :disable="!departmentId"
      @update:model-value="onPositionChange"
    />
  </div>
</template>
```

| 级别 | 数据源 | 绑定 |
|------|--------|------|
| 行业 | `tree` 中 `level=industry` 的根节点 | 仅筛选，清空后续选择 |
| 部门 | 选中行业的 `children` | 仅筛选，清空后续选择 |
| 职位 | 选中部门的 `children` | `categoryPositionId`，emit 给父组件 |

**Composable**：`useCategoryCascade.ts`

```typescript
export function useCategoryCascade() {
  const tree = ref<AgentCategoryTreeNode[]>([]);
  const industryId = ref("");
  const departmentId = ref("");
  const positionId = ref("");

  const industryOptions = computed(() =>
    tree.value.map((n) => ({ label: n.category.name, value: n.category.id }))
  );
  const selectedIndustry = computed(() =>
    tree.value.find((n) => n.category.id === industryId.value)
  );
  const departmentOptions = computed(() =>
    (selectedIndustry.value?.children ?? []).map((n) => ({
      label: n.category.name,
      value: n.category.id,
    }))
  );
  const selectedDepartment = computed(() =>
    selectedIndustry.value?.children.find((n) => n.category.id === departmentId.value)
  );
  const positionOptions = computed(() =>
    (selectedDepartment.value?.children ?? []).map((n) => ({
      label: n.category.name,
      value: n.category.id,
    }))
  );

  async function loadTree() {
    tree.value = await getCategoryTree();
  }

  function onIndustryChange() {
    departmentId.value = "";
    positionId.value = "";
  }
  function onDepartmentChange() {
    positionId.value = "";
  }
  function onPositionChange() {
    // emit positionId to parent
  }

  return {
    tree, industryId, departmentId, positionId,
    industryOptions, departmentOptions, positionOptions,
    loadTree, onIndustryChange, onDepartmentChange, onPositionChange,
  };
}
```

#### CategoryManagePage.vue — 分类管理页面

路由：`/settings/agent-categories`

```vue
<template>
  <QPage padding>
    <div class="row items-center q-mb-md">
      <div class="text-h6">Agent 行业分类</div>
      <QSpace />
      <QInput v-model="search" dense outlined debounce="300" placeholder="搜索名称...">
        <template #prepend><QIcon name="search" /></template>
      </QInput>
      <QToggle v-model="onlyMine" label="仅看我的自建" class="q-ml-md" />
      <QBtn color="primary" label="新增行业" icon="add" class="q-ml-md" @click="openCreateDialog('industry')" />
    </div>

    <QTree
      :nodes="filteredTree"
      node-key="category.id"
      default-expand-all
    >
      <template #default-header="{ node }">
        <div class="row items-center full-width">
          <span>{{ node.category.name }}</span>
          <QBadge v-if="node.category.is_system" color="blue" label="系统" class="q-ml-sm" />
          <QBadge v-else color="teal" label="自建" class="q-ml-sm" />
          <QSpace />
          <QBtn v-if="node.category.level !== 'position'" flat round dense size="sm" icon="add"
            @click.stop="openCreateDialog(nextLevel(node.category.level), node.category.id)">
            <QTooltip>添加子{{ nextLevelLabel(node.category.level) }}</QTooltip>
          </QBtn>
          <QBtn flat round dense size="sm" icon="edit" @click.stop="openEditDialog(node.category)" />
          <QBtn flat round dense size="sm" icon="delete" color="negative"
            @click.stop="confirmDelete(node.category)" />
        </div>
      </template>
    </QTree>

    <CategoryEditDialog v-model="editDialog" :category="editingCategory" :level="creatingLevel"
      :parent-id="creatingParentId" @saved="onSaved" />
  </QPage>
</template>
```

| 控件 | 行为 |
|------|------|
| **搜索** | 前端过滤树节点名称 |
| **仅看我的自建** | 过滤 `is_system = false` 且 `owner_user_id = 当前用户` |
| **新增行业** | 打开 `CategoryEditDialog`，`level=industry`，`parent_id=""` |
| **某行业下「+」** | 打开 `CategoryEditDialog`，`level=department`，`parent_id=该行业ID` |
| **某部门下「+」** | 打开 `CategoryEditDialog`，`level=position`，`parent_id=该部门ID` |
| **编辑** | 打开 `CategoryEditDialog`，预填当前节点数据 |
| **删除** | `QDialog` 确认；若职位被 Agent 引用，提示数量并阻止删除 |

#### CategoryEditDialog.vue — 分类编辑弹窗

```vue
<template>
  <QDialog v-model="modelValue" persistent>
    <QCard style="min-width: 400px">
      <QCardSection>
        <div class="text-h6">{{ isEdit ? '编辑' : '新增' }}{{ levelLabel }}</div>
      </QCardSection>
      <QCardSection class="q-pt-none">
        <QForm @submit.prevent="onSave">
          <QInput v-model="form.key" label="标识" outlined dense :readonly="isEdit"
            :rules="[v => !!v || '标识必填']" />
          <QInput v-model="form.name" label="名称" outlined dense class="q-mt-sm"
            :rules="[v => !!v || '名称必填']" />
          <QInput v-model="form.description" label="描述" outlined dense type="textarea"
            class="q-mt-sm" />
          <QInput v-model.number="form.sort_order" label="排序" outlined dense type="number"
            class="q-mt-sm" />
        </QForm>
      </QCardSection>
      <QCardActions align="right">
        <QBtn flat label="取消" @click="modelValue = false" />
        <QBtn unelevated color="primary" label="保存" @click="onSave" />
      </QCardActions>
    </QCard>
  </QDialog>
</template>
```

| 字段 | 控件 | 必填 | 说明 |
|------|------|------|------|
| `key` | `QInput` | ✅ | 唯一标识，编辑时只读 |
| `name` | `QInput` | ✅ | 展示名称 |
| `description` | `QInput` textarea | — | 描述 |
| `sort_order` | `QInput` type=number | — | 同级排序 |

### 7.5 页面路由

```typescript
const routes: RouteRecordRaw[] = [
  {
    path: "/settings/agent-categories",
    component: () => import("pages/settings/CategoryManagePage.vue"),
  },
];
```

---

## 八、与 `agents` 表的关联

### 8.1 Agent 中的分类字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `category_position_id` | TEXT NULL, FK → `agent_category_nodes(id)` | 仅允许引用 `level = position` 且未删除的行；为空表示未分类 |

### 8.2 创建/编辑 Agent 时的分类选择

- 创建页：使用 `AgentCategoryCascade` 三级联动选择，最终提交 `category_position_id`
- 编辑页：同上，或只读展示完整路径
- 列表页：`category_id` 筛选参数映射到 `agents.category_position_id`

### 8.3 完整路径展示

列表/详情中展示完整路径（如 `IT行业 / 游戏开发部 / UE5场景设计师`）：
- 推荐方式：前端从 `getCategoryTree()` 构建路径映射，根据 `category_position_id` 查找完整路径
- 备选方式：后端在 Agent 列表响应中附带 `category_path` 冗余字段

---

## 九、种子数据

系统预置分类示例：

```text
IT行业 (is_system=true)
  ├── 游戏开发部
  │     └── UE5场景设计师
  └── 系统开发部
        └── golang后端高级工程师
```

导入脚本将 `is_system = true`，`workspace_id` 按产品设为全局或模板工作区。

---

## 十、验收要点

- [ ] 仅能形成合法三层：行业无父；部门父为行业；职位父为部门
- [ ] 同级重名在同一父节点下被拒绝
- [ ] 用户自建节点归属正确，与系统预置在 UI 上有区分（徽章「系统」「自建」）
- [ ] Agent 仅绑定职位节点，列表/详情可展示完整路径
- [ ] 删除职位时有 Agent 引用则阻止删除并提示数量
- [ ] 删除行业/部门时有子节点则阻止删除
- [ ] 三级级联选择器在创建/编辑 Agent 表单中正确联动
