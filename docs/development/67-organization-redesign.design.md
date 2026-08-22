# M67: 公司架构重塑 — 实现设计

> 对应需求：[67 organization-redesign.md](./67-organization-redesign.md)
> **方案报告**：[2026-06-07-proposal-organization-redesign.md](../reports/2026-06-07-proposal-organization-redesign.md)
> **开发计划**：[67 organization-redesign.development.md](./67-organization-redesign.development.md)

---

## 一、模块概述

### 1.1 设计定位

将"行业分类"(IndustryTaxonomy) 重塑为"公司架构"(Organization)，明确公司架构/Agent/Team/Graph 四者边界，新增部门主管和跨部门协作机制。

核心变更：
- **语义重塑**：IndustryTaxonomy → Organization，industry → company
- **挂载修正**：Team 从 industry 级下移到 department 级
- **新增角色**：部门主管 Agent（自动创建、自动加入、审批门禁）
- **新增机制**：交付物契约 + 审批门禁 + 驳回返工流程
- **Graph 归属**：Graph 可归属 Team 或作为模板

> **热路径断点（2026-08-22）**：`SpiritTeamParams.DepartmentID` 与主管自动加入已实现，但 `RealTeamOrchestrator` 组装 Spirit 团队时未赋值，导致自动编排的 Team 常无部门、主管/借调不触发。路由规则与修复任务见 [M78](./78-org-aware-orchestration.design.md)，本设计不重复展开匹配算法。

### 1.2 分层与依赖

```
api/kratos/organization/v1/organization.proto   ← 新 OrganizationService
api/kratos/agent/v1/agent.proto                 ← position_id 字段重命名
api/kratos/team/v1/team.proto                   ← department_id + 交付物契约
api/kratos/graph/v1/graph.proto                 ← team_id + verification_gates
        ↓
internal/service/
  organization.go                                ← OrganizationService 实现
  team.go                                        ← Team 字段适配
  spirit_team.go                                 ← 部门主管自动加入
        ↓
internal/biz/
  organization.go                                ← OrganizationUsecase（原 TaxonomyUsecase）
  dept_lead.go                                   ← 部门主管管理
  deliverable_contract.go                        ← 交付物契约验证
  verification_gate.go                           ← 审批门禁执行
  spirit_team_usecase.go                         ← DAG 调度适配
        ↓
internal/data/ent/schema/
  organization.go                                ← 原 industry_taxonomy.go
  agent.go                                       ← position_id 重命名
  team.go                                        ← department_id + 新字段
  graph_definition.go                            ← team_id + verification_gates
        ↓
internal/scenario/
  taxonomy.yaml → organization.yaml              ← 结构变更
  */agents.yaml                                  ← position_key 适配
  system/prompts/dept_lead.md                    ← 部门主管 Prompt
        ↓
web/src/features/platform/                       ← 原 industries/（暂未重命名为 organization/）
```

**红线**：
- `internal/biz` 不 import `pkg/trpc-agent-go`
- 部门主管的审批逻辑在 biz 层，不在 service 层
- 交付物契约验证为提示性，不阻断 Team 组建

---

## 二、数据模型设计

### 2.1 Organization（原 IndustryTaxonomy）

> **代码锚点**：`internal/data/ent/schema/organization.go`

**表名变更**：`industry_taxonomy` → `organizations`

**字段变更**：

| 字段 | 变更 | 说明 |
|------|------|------|
| `level` | 值变更 | `"industry"` → `"company"`，`"department"` / `"position"` 不变 |
| `dept_lead_agent_id` | **新增** | string, Default(""), 仅 department 级节点使用 |
| `dept_lead_config_json` | **新增** | text, 默认 `"{}"`，部门主管配置覆盖 |

**Ent Schema 关键定义**（与代码一致）：

```go
// internal/data/ent/schema/organization.go
func (Organization) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Immutable().Unique().MaxLen(256),
        field.String("org_key").Unique().MaxLen(512),    // renamed from taxonomy_key
        field.String("name").MaxLen(1024),
        field.Text("description").Default(""),
        field.String("status").Default("active"),
        field.Bool("enabled").Default(true),
        field.Int("sort_order").Default(0),
        field.String("parent_id").Default(""),            // 自引用，树形结构
        field.String("level").Default(""),                // "company" | "department" | "position"
        field.String("scenario_key").Default(""),
        field.String("workspace_id").Default(""),
        field.String("owner_user_id").Default(""),
        field.Bool("is_system").Default(false),
        field.Text("config_json").Default(""),
        field.Text("metadata_json").Default(""),
        // 部门级字段
        field.String("dept_lead_agent_id").Default("").Optional(),
        field.Text("dept_lead_config_json").Default("{}"),
        field.String("created_at").Default(""),
        field.String("updated_at").Default(""),
        field.String("deleted_at").Default(""),
    }
}
```

**多公司支持（预留）**：
- 当前为单公司模式，Organization 树根节点唯一（level="company"）
- 预留扩展：通过 workspace_id 隔离不同公司的 Organization 树
- 多公司场景：用户可创建多个 workspace，每个 workspace 有独立的 Organization 树
- 暂不实现，但 schema 设计时确保 workspace_id 字段存在且可索引

**业务规则**（`normalizeOrganization`）：

```go
func normalizeOrganization(node *Organization) error {
    switch node.Level {
    case "company":
        if node.ParentID != "" {
            return errors.New("company node cannot have parent")
        }
    case "department":
        if node.ParentID == "" {
            return errors.New("department must have parent (company)")
        }
        // parent must be company level
    case "position":
        if node.ParentID == "" {
            return errors.New("position must have parent (department)")
        }
        // parent must be department level
    }
    return nil
}
```

```go
// 部门删除级联规则
func (uc *OrganizationUsecase) deleteDepartmentWithCascade(ctx context.Context, deptID string) error {
    // 1. 检查该部门下是否有活跃 Team
    teams, err := uc.teamUC.ListByDepartmentID(ctx, deptID)
    if err != nil {
        return err
    }
    activeTeams := filter(teams, func(t Team) bool {
        return t.Status == "running" || t.Status == "pending"
    })
    if len(activeTeams) > 0 {
        return errors.New("cannot delete department with active teams, please archive or cancel them first")
    }

    // 2. 处理该部门下的 Agent
    agents, err := uc.agentUC.ListByPositionDepartment(ctx, deptID)
    if err != nil {
        return err
    }
    // Agent 不删除，但解除岗位关联（position_id 置空）
    for _, agent := range agents {
        _ = uc.agentUC.ClearPosition(ctx, agent.ID)
    }

    // 3. 归档该部门下的已完成/已归档 Team
    for _, team := range teams {
        if team.Status != "archived" {
            _ = uc.teamUC.ArchiveTeam(ctx, team.ID)
        }
    }

    // 4. 删除部门主管 Agent
    _ = uc.deptLeadMgr.DeleteDeptLead(ctx, deptID)

    // 5. 删除该部门下的岗位节点
    positions, _ := uc.ListByParentID(ctx, deptID)
    for _, pos := range positions {
        _ = uc.Delete(ctx, pos.ID)
    }

    // 6. 删除部门节点本身
    return uc.repo.DeleteOrgNode(ctx, deptID)
}
```

**部门删除级联规则**：

| 关联对象 | 处理策略 | 说明 |
|----------|----------|------|
| 活跃 Team | **阻止删除** | 有 running/pending Team 时禁止删除部门 |
| 已完成 Team | 归档 | 自动归档非活跃 Team |
| 部门主管 Agent | 级联删除 | 随部门一起删除 |
| 岗位下 Agent | 解除关联 | position_id 置空，Agent 保留 |
| 岗位节点 | 级联删除 | 随部门一起删除 |
| 跨部门借调 | 自动取消 | 被借调成员从目标 Team 移除 |

### 2.2 Agent 字段变更

| 旧字段 | 新字段 | 说明 |
|--------|--------|------|
| `taxonomy_position_id` | `position_id` | FK → Organization(position) |
| `position_key` | 保留不变 | 与 position_id 互补：position_id 是 FK，position_key 是业务 key（如 go_engineer），两者用途不同，均保留 |

### 2.3 Team 字段变更

> **代码锚点**：`internal/data/ent/schema/team.go`

| 旧字段 | 新字段 | 说明 |
|--------|--------|------|
| `category_industry_id` | `department_id` | FK → Organization(department) |
| - | `deliverables` | text, 默认 `"[]"`, 交付物定义 JSON |
| - | `input_contract` | text, 默认 `"[]"`, 输入契约 JSON |
| - | `dept_lead_agent_id` | string, Default(""), 部门主管（默认从部门继承） |
| - | `cross_dept_member_ids` | text, 默认 `"[]"`, 跨部门成员 Agent ID 列表 JSON |
| - | `linked_graph_id` | string, Default(""), FK → graph_definitions(id)，与 Graph.team_id 双向引用 |

**cross_dept_members 详细 Schema**：
```json
[
  {
    "agent_id": "agent_ui_designer_zhou",
    "source_department_id": "dept_design",
    "role": "ui_designer",
    "approval_status": "approved",
    "approved_by": "agent___dept_lead_design__",
    "requested_at": "2026-06-07T10:00:00Z"
  }
]
```

**跨部门成员业务规则**：
1. 主归属部门成员无限制加入，无需审批
2. 跨部门成员需其来源部门主管审批（初期实现：超时自动通过，不阻塞业务；后续迭代：支持主管主动审批/拒绝）
3. 跨部门成员数量不超过 Team 总人数的 50%（可配置）
4. 跨部门成员的工作产出由主归属部门主管审批
5. 跨部门成员的借调行为由来源部门主管审批
6. 跨部门成员的产出审批细化规则：
   - 产出在本 Team 内流转 → 主归属部门主管审批（主管了解 Team 目标）
   - 产出交付给其他 Team → 接收方部门主管验收（接收方了解自己的需求）
   - 借调成员的专业质量 → 来源部门主管可查看但无审批权（专业评估由主归属部门主管负责）
   - 简化原则：谁用谁批，谁产出谁负责

**跨部门成员策略（修订）**：
- Team 必须指定一个主归属部门（department_id）
- Team 成员**默认**从主归属部门选择
- 允许跨部门选人：通过 `cross_dept_member_ids` 字段记录跨部门成员
- 跨部门成员加入时需其所属部门的主管同意（自动审批）
- 跨部门协作的两种模式：
  1. 轻量协作：Team 内包含少数跨部门成员（如 1 个设计师加入研发 Team）
  2. 深度协作：多个 Team 组成 DAG（如设计 Team → 研发 Team）
- 用户根据实际需要选择模式，系统不强制

**交付物 Schema**（与代码一致）：

```go
// internal/biz/deliverable_contract.go
type DeliverableContract struct {
    Name        string `json:"name"`         // 交付物标识，如 "design_spec"
    Type        string `json:"type"`         // 类型: document/code/data
    Format      string `json:"format"`       // 格式: markdown/json/zip
    Description string `json:"description"`
}
```

**契约验证**：`DeliverableContractValidator.ValidateContractMatch(upstream, downstream []DeliverableContract) []string` 返回警告列表（提示性，不阻断）。验证逻辑：遍历 downstream 的每一项，在 upstream 中查找 name 匹配，检查 type/format 兼容性。

### 2.4 Graph 字段变更

> **代码锚点**：`internal/data/ent/schema/graph_definition.go`

| 新字段 | 类型 | 说明 |
|--------|------|------|
| `team_id` | string, Default(""), Optional | 归属 Team，空表示模板 |
| `is_template` | bool, 默认 false | 是否为模板 Graph |
| `verification_gates` | text, 默认 `"[]"` | 审批门禁定义 JSON |

**双向引用规则**：
- Team → Graph：`OrchestrationSpec.linked_graph_id`（权威方向，Team 主动引用 Graph）
- Graph → Team：`team_id`（反向索引，仅用于查询"这个 Graph 属于哪个 Team"）
- 写入时：Team 设置 linked_graph_id 后，系统自动回写 Graph.team_id
- 查询时：通过 Graph.team_id 快速定位归属 Team，无需遍历所有 Team
- 模板 Graph：team_id 为空，linked_graph_id 也不指向它

**双向引用回写实现**（与代码一致）：

```go
// internal/biz/team_usecase.go
// 在 TeamUsecase.Update 中，当 linked_graph_id 变更时调用 syncGraphTeamID
func (u *TeamUsecase) syncGraphTeamID(ctx context.Context, oldGraphID, newGraphID, teamID string)
    // oldGraphID 的 Graph.team_id 清空
    // newGraphID 的 Graph.team_id 设为 teamID
```

```go
// internal/biz/team_usecase.go — TeamUsecase.Delete 中的 Graph 清理（ORG-11c）
// 专属 Graph（!IsTemplate）：随 Team 一起删除
// 模板 Graph（IsTemplate）：仅清除 team_id 引用
if graphDef.IsTemplate {
    graphDef.TeamID = ""
    u.graphWriter.UpdateDefinition(ctx, graphDef)
} else {
    u.graphWriter.DeleteDefinition(ctx, team.LinkedGraphID)
}
```

**审批门禁 Schema**（与代码一致）：

```go
// internal/biz/verification_gate.go
type VerificationGateType string

const (
    GateTypeDeptLeadApproval  VerificationGateType = "dept_lead_approval"
    GateTypeCrossDeptDelivery VerificationGateType = "cross_dept_delivery"
    GateTypeBorrowApproval    VerificationGateType = "borrow_approval"
)

// VerificationGate 定义 Graph 中的审批门禁节点
type VerificationGate struct {
    GateType    VerificationGateType `json:"gate_type"`
    AgentID     string               `json:"agent_id,omitempty"`     // dept_lead_approval 用
    Description string               `json:"description"`
    MaxRetries  int                  `json:"max_retries"`            // 默认 3
}

// CrossDeptDeliveryGate 跨部门交付物双方审批门禁
type CrossDeptDeliveryGate struct {
    GateType              VerificationGateType `json:"gate_type"`               // "cross_dept_delivery"
    OutputDepartmentID    string               `json:"output_department_id"`    // 输出方部门
    ReceivingDepartmentID string               `json:"receiving_department_id"` // 接收方部门
    DeliverableName       string               `json:"deliverable_name"`
    Description           string               `json:"description"`
    MaxRetries            int                  `json:"max_retries"`             // 默认 3
}

// GateResult 审批结果
type GateResult struct {
    Approved bool
    Reason   string
}

// 审批流程：
// 1. 输出方部门主管：质量把关（输出是否符合本部门标准）
// 2. 接收方部门主管：验收确认（输出是否满足本部门输入需求）
// 3. 两方都通过 → 交付物传递到下游 Team
// 4. 任一方驳回 → 上游 Team 返工
// 5. 审批顺序：输出方先审（质量），接收方再审（验收）
// 6. LLM 解析失败默认拒绝（安全优先）
// 7. dept lead 缺失返回错误
```

**借调审批**：通过 `GateTypeBorrowApproval` 类型实现，由 `VerificationGateExecutor.executeBorrowApproval` 执行，不需要单独的 `BorrowApprovalGate` 结构体。借调请求通过 `BorrowRequest` 类型管理（见 4.2）。

---

## 三、Proto API 设计

### 3.1 OrganizationService

> **代码锚点**：`api/kratos/organization/v1/organization.proto`

```protobuf
// api/kratos/organization/v1/organization.proto

service OrganizationService {
  rpc ListOrganization(google.protobuf.Empty) returns (ListOrganizationResponse) {
    option (google.api.http) = { get: "/v1/organization" };
  }
  rpc ListOrganizationTree(google.protobuf.Empty) returns (ListOrganizationTreeResponse) {
    option (google.api.http) = { get: "/v1/organization/tree" };
  }
  rpc CreateOrganization(CreateOrganizationRequest) returns (OrganizationNode) {
    option (google.api.http) = { post: "/v1/organization" body: "*" };
  }
  rpc GetOrganization(GetOrganizationRequest) returns (OrganizationNode) {
    option (google.api.http) = { get: "/v1/organization/{id}" };
  }
  rpc UpdateOrganization(UpdateOrganizationRequest) returns (OrganizationNode) {
    option (google.api.http) = { patch: "/v1/organization/{id}" body: "node" };
  }
  rpc DeleteOrganization(DeleteOrganizationRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/organization/{id}" };
  }
  rpc ReorderOrganization(ReorderOrganizationRequest) returns (ReorderOrganizationResponse) {
    option (google.api.http) = { put: "/v1/organization/reorder" body: "*" };
  }
}

message OrganizationNode {
  string id = 1;
  string org_key = 2;
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
  // 部门级专用
  string dept_lead_agent_id = 18;
  string dept_lead_config_json = 19;
}

message OrganizationTreeNode {
  OrganizationNode node = 1;
  repeated OrganizationTreeNode children = 2;
}
```

**注意**：Biz 层 `OrganizationNode` 结构体的字段名为 `Key`（对应 proto 的 `org_key`），`OrganizationTreeNode` 的字段名为 `Category`（对应 proto 的 `node`）。Proto 与 Biz 层的字段映射在 `internal/service/organization.go` 的 `toProtoOrganization`/`fromProtoOrganization` 中完成。

### 3.2 Agent Proto 变更

```protobuf
// 字段重命名
- TaxonomyPositionID → PositionID
// 字段编号不变，仅名称变更
```

```
// ListAgentsRequest 中 category_id = 4 字段语义变更：
// 原：按 industry taxonomy 节点过滤
// 新：按 organization 节点过滤（可传 department_id 或 position_id）
// 字段名建议 rename 为 org_node_id，保持编号 4 不变
```

### 3.3 Team Proto 变更

```protobuf
message Team {
  // ...existing fields...
  string department_id = 14;          // 原 category_industry_id, 保持编号不变
  string deliverables = 41;           // 交付物 JSON
  string input_contract = 42;         // 输入契约 JSON
  string dept_lead_agent_id = 43;     // 部门主管 Agent ID
}
// 注意：CreateTeamRequest 中 category_industry_id = 6 同步 rename 为 department_id，保持编号 6 不变
```

### 3.4 Graph Proto 变更

```protobuf
message GraphDefinition {
  // ...existing fields...
  string team_id = 30;                // 归属 Team
  bool is_template = 31;              // 是否模板
  string verification_gates = 32;     // 审批门禁 JSON
}
```

---

## 四、Biz 层设计

### 4.1 OrganizationUsecase（原 TaxonomyUsecase）

> **代码锚点**：`internal/biz/organization.go`

```go
// internal/biz/organization.go
type OrganizationUsecase struct {
    repo        OrganizationRepo
    deptLeadMgr *DeptLeadManager
    teamLister  DeptTeamLister
    teamWriter  TeamWriter
    agentClear  DeptAgentPositionClearer
    eventBus    contract.Bus
    lg          loggateway.Logger
    posPrompt   *PositionPromptUsecase
}

// 核心方法（原 TaxonomyUsecase 方法迁移并重命名）
func (u *OrganizationUsecase) List(ctx context.Context) ([]OrganizationNode, error)
func (u *OrganizationUsecase) Tree(ctx context.Context) ([]OrganizationTreeNode, error)
func (u *OrganizationUsecase) Get(ctx context.Context, id string) (OrganizationNode, error)
func (u *OrganizationUsecase) Create(ctx context.Context, in OrganizationNode) (OrganizationNode, error)
func (u *OrganizationUsecase) Update(ctx context.Context, id string, patch OrganizationNode) (OrganizationNode, error)
func (u *OrganizationUsecase) Delete(ctx context.Context, id string) error
// ... 其他方法（ListByLevel/ListByParentID/GetByKey/Reorder/GetAncestors 等）

// OrgAncestors 结构体（原 TaxonomyAncestors）
type OrgAncestors struct {
    Company    OrganizationNode // 原 Industry
    Department OrganizationNode
    Position   OrganizationNode
}

// OrganizationNode biz 层结构体（字段名 Key 对应 proto 的 org_key）
type OrganizationNode struct {
    ID                  string
    Key                 string  // 对应 proto org_key
    Name                string
    Description         string
    Status              string
    Enabled             bool
    SortOrder           int
    ParentID            string
    Level               string
    ScenarioKey         string
    WorkspaceID         string
    OwnerUserID         string
    IsSystem            bool
    ConfigJSON          string
    MetadataJSON        string
    DeptLeadAgentID     string
    DeptLeadConfigJSON  string
    CreatedAt           string
    UpdatedAt           string
    DeletedAt           string
}

// OrganizationTreeNode（字段名 Category 对应 proto 的 node）
type OrganizationTreeNode struct {
    Category OrganizationNode
    Children []OrganizationTreeNode
}
```

**OrganizationRepo 接口拆分**（与代码一致）：

```go
// Stability:stable
type OrganizationReader interface {
    GetOrgNode(ctx context.Context, id string) (OrganizationNode, error)
    GetOrgNodeByKey(ctx context.Context, key string) (OrganizationNode, error)
    ListOrgNodes(ctx context.Context) ([]OrganizationNode, error)
    ListOrgNodesByLevel(ctx context.Context, level string) ([]OrganizationNode, error)
    ListOrgNodesByParentID(ctx context.Context, parentID string) ([]OrganizationNode, error)
}

// Stability:stable
type OrganizationWriter interface {
    CreateOrgNode(ctx context.Context, c OrganizationNode) (OrganizationNode, error)
    UpdateOrgNode(ctx context.Context, c OrganizationNode) (OrganizationNode, error)
    DeleteOrgNode(ctx context.Context, id string) error
    ReorderOrgNodes(ctx context.Context, ids []string) error
}

// Stability:stable
type OrganizationRepo interface {
    OrganizationReader
    OrganizationWriter
    GetOrgNodeByKeyAnyState(ctx context.Context, key string) (OrganizationNode, error)
}
```

**辅助接口**（部门级联删除使用）：

```go
// Stability:stable
type DeptTeamLister interface {
    ListTeamsByDepartmentID(ctx context.Context, deptID string) ([]Team, error)
}

// Stability:stable
type DeptAgentPositionClearer interface {
    ClearPositionByDepartment(ctx context.Context, deptID string) (int, error)
}
```

### 4.2 DeptLeadManager（部门主管管理）

> **代码锚点**：`internal/biz/dept_lead.go`

```go
// internal/biz/dept_lead.go
const DeptLeadAgentKeyPrefix = "__dept_lead_"
const maxCrossDeptRatio = 0.5  // 跨部门成员上限 50%

type DeptLeadManagerOpts struct {
    OrgRepo    OrganizationRepo
    BorrowRepo BorrowRequestRepo
    AgentRepo  AgentRepository
    AgentUC    *AgentUsecase
    TeamGetter DeptLeadTeamGetter
    EventBus   contract.Bus
    Logger     loggateway.Logger
}

type DeptLeadManager struct {
    orgRepo    OrganizationRepo
    borrowRepo BorrowRequestRepo
    agentRepo  AgentRepository
    agentUC    *AgentUsecase
    teamGetter DeptLeadTeamGetter
    eventBus   contract.Bus
    lg         loggateway.Logger
}

// DeptLeadTeamGetter is a narrow interface for fetching team info needed by DeptLeadManager.
type DeptLeadTeamGetter interface {
    GetTeamByID(ctx context.Context, id string) (Team, error)
}
```

```go
// 创建部门主管
func (m *DeptLeadManager) CreateDeptLead(ctx context.Context, deptNode OrganizationNode) (*Agent, error)
    // 1. 生成 Agent Key: "__dept_lead_{deptNode.Key}__"
    // 2. 设置 kind=system_builtin, source=system
    // 3. 设置 position_id 指向部门管理岗
    // 4. 加载部门主管 Prompt 模板（buildDeptLeadSystemPrompt）
    // 5. 创建 Agent 记录
    // 6. 幂等：若已存在则仅更新 Organization.dept_lead_agent_id

// 删除部门主管
func (m *DeptLeadManager) DeleteDeptLead(ctx context.Context, deptID string) error

// 替换部门主管（用户自定义）
func (m *DeptLeadManager) ReplaceDeptLead(ctx context.Context, deptID string, newAgentID string) error
    // 更新 Organization.dept_lead_agent_id

// 借调请求类型（与代码一致）
type BorrowRequest struct {
    ID           string
    TeamID       string
    AgentID      string
    FromDeptID   string // 拥有该 Agent 的部门
    ToDeptID     string // 想借调该 Agent 的部门
    Status       string // pending | approved | rejected | auto_approved
    Reason       string
    ReviewedBy   string // 审批的部门主管 Agent ID
    ReviewReason string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

// 借调请求状态常量
const (
    BorrowRequestPending      = "pending"
    BorrowRequestApproved     = "approved"
    BorrowRequestRejected     = "rejected"
    BorrowRequestAutoApproved = "auto_approved"
)
const BorrowAutoApproveTimeout = 5 * time.Minute

// BorrowRequestRepo 接口拆分
type BorrowRequestReader interface {
    GetBorrowRequest(ctx context.Context, id string) (BorrowRequest, error)
    ListPendingBorrowRequests(ctx context.Context, deptID string) ([]BorrowRequest, error)
    ListBorrowRequestsByTeam(ctx context.Context, teamID string) ([]BorrowRequest, error)
    ListExpiredPendingBorrowRequests(ctx context.Context) ([]BorrowRequest, error)
}
type BorrowRequestWriter interface {
    CreateBorrowRequest(ctx context.Context, r BorrowRequest) (BorrowRequest, error)
    UpdateBorrowRequest(ctx context.Context, r BorrowRequest) (BorrowRequest, error)
    CancelBorrowRequestsByFromDept(ctx context.Context, deptID string) (int, error)
}
type BorrowRequestRepo interface {
    BorrowRequestReader
    BorrowRequestWriter
}

// 提交借调请求
func (m *DeptLeadManager) SubmitBorrowRequest(ctx context.Context, r BorrowRequest) (BorrowRequest, error)

// 审批借调请求
func (m *DeptLeadManager) ApproveBorrowRequest(ctx context.Context, id string, reviewerAgentID string, reason string) (BorrowRequest, error)

// 拒绝借调请求
func (m *DeptLeadManager) RejectBorrowRequest(ctx context.Context, id string, reviewerAgentID string, reason string) (BorrowRequest, error)

// 借调超时自动通过
func (m *DeptLeadManager) AutoApproveExpiredBorrowRequests(ctx context.Context) error
    // 查找 pending 且超过 BorrowAutoApproveTimeout(5分钟) 的请求，自动通过

// 查看被借调成员的工作状态（只读）
func (m *DeptLeadManager) GetBorrowedMemberStatus(ctx context.Context, deptID string) ([]BorrowedMemberStatus, error)

type BorrowedMemberStatus struct {
    AgentID       string `json:"agent_id"`
    AgentName     string `json:"agent_name"`
    TargetTeamID  string `json:"target_team_id"`
    TargetTeamName string `json:"target_team_name"`
    TeamStatus    string `json:"team_status"` // running/completed/failed
    LastOutput    string `json:"last_output"` // 最近一次输出摘要
}
```

### 4.3 DeliverableContractValidator（交付物契约验证）

```go
// internal/biz/deliverable_contract.go
type DeliverableContractValidator struct{}

// 验证上下游契约匹配（提示性，不阻断）
// 返回警告列表（[]string），空列表表示完全匹配
func (v *DeliverableContractValidator) ValidateContractMatch(
    upstream []DeliverableContract,
    downstream []DeliverableContract,
) []string
    // 1. 遍历 downstream 的每一项
    // 2. 在 upstream 中查找 name 匹配
    // 3. 检查 type/format 兼容性
    // 4. 返回警告列表（提示性，不阻断 Team 组建）
```

### 4.4 VerificationGateExecutor（审批门禁执行）

```go
// internal/biz/verification_gate.go
type VerificationGateExecutor struct {
    deptLeadMgr *DeptLeadManager
    llmCaller   LLMCaller
    lg          loggateway.Logger
}

// 执行审批门禁
func (e *VerificationGateExecutor) ExecuteGate(
    ctx context.Context,
    gate VerificationGate,
    teamOutput string,
    truncateChars int,
) (bool, string, error)
    // 审批执行路径：
    // 方案A（推荐）：直接调用 LLM API
    //   1. 查找部门主管 Agent，获取其 model/provider 配置
    //   2. 构造审批 Prompt（交付物内容 + 审批要求 + 部门主管 system prompt）
    //   3. 直接调用 LLM ChatCompletion API（不经过 Agent 运行时）
    //   4. 解析 LLM 返回的 JSON 判断结果
    //   优点：简单、快速、无状态
    //   缺点：不经过 Agent 工具链，无法使用工具
    //
    // 方案B（后续迭代）：走 Agent 运行时
    //   1. 创建临时 Session，注入部门主管 Agent
    //   2. 将审批请求作为 User Message 发送
    //   3. 等待 Agent 响应（可使用工具查询上下文）
    //   4. 从 Agent 响应中提取审批结果
    //   优点：Agent 可使用工具获取上下文，审批更智能
    //   缺点：复杂度高，需创建/销毁 Session
    //
    // 初期采用方案A，后续按需升级为方案B
    // 审批角色区分：
    // - dept_lead_approval: 单方审批（本部门主管质量把关）
    // - cross_dept_delivery: 双方审批（输出方质量把关 + 接收方验收确认）
    // - borrow_approval: 借调审批（来源部门主管同意借出）

type GateResult struct {
    Approved bool   `json:"approved"`
    Reason   string `json:"reason"`      // 驳回理由
}
```

### 4.5 审批驳回返工模型

```go
// 驳回返工策略：
// - 初期方案：重新执行整个 Team（简单可靠）
//   1. 部门主管驳回 → 通过 TransitionStatus 标记 Team 状态为 pending（触发重执行）
//   2. 清除 Team 的执行结果
//   3. 重新启动 Team 执行（从 entry_point 开始）
//   4. 优点：实现简单，与现有 Team 生命周期一致
//   5. 缺点：已通过的节点也会重新执行
//
// - 后续迭代：部分重执行（仅重新执行被驳回节点及其下游）
//   1. 利用 Graph 的 checkpoint 机制回滚到被驳回节点
//   2. 仅重新执行该节点及其下游节点
//   3. 优点：效率高，不重复已通过的工作
//   4. 缺点：需要 Graph 运行时支持部分回滚，复杂度高
//
// 初期采用"重新执行整个 Team"方案

// internal/biz/rework.go
type ReworkStrategy string
const (
    ReworkStrategyFullTeam ReworkStrategy = "full_team" // 重新执行整个 Team（初期唯一实现）
    // 后续迭代可新增 ReworkStrategyPartial（部分重执行），当前代码未定义
)

// ReworkTracker 追踪 Team 的返工次数
type ReworkTracker struct {
    TeamID    string         `json:"team_id"`
    Strategy  ReworkStrategy `json:"strategy"`
    MaxRetries int           `json:"max_retries"`
    Attempts  int            `json:"attempts"`
}

// CanRetry 判断是否还能重试
func (r *ReworkTracker) CanRetry() bool

// IncrementAttempt 增加重试次数
func (r *ReworkTracker) IncrementAttempt() int
```

**返工流程实现**（与代码一致）：

```go
// internal/biz/spirit_team_usecase.go
// HandleTeamRejection 处理审批门禁驳回
func (u *SpiritTeamUsecase) HandleTeamRejection(ctx context.Context, teamID string, tracker ReworkTracker, reason string) (*ReworkTracker, error) {
    // 1. 检查是否还能重试（CanRetry）
    // 2. 若不能重试 → 调用 EscalateToSpirit 升级处理
    // 3. 若能重试 → IncrementAttempt + TransitionStatus(teamID, TeamStatusPending) 触发重执行
}

// EscalateToSpirit 升级给精灵助手（超过 max_retries）
func (u *SpiritTeamUsecase) EscalateToSpirit(ctx context.Context, teamID string, tracker ReworkTracker) error {
    // 1. TransitionStatus(teamID, TeamStatusFailed) 标记 Team 为 failed
    // 2. 发布升级事件
}
```

### 4.6 交付物传递机制

```go
// Team 间交付物传递设计（2026-07-21 P0 实现版）：
// 1. 底层存储：teams.deliverables_output_json 专用列（TECH-DEBT #B-03 修复），
//    JSON object keyed by dag_node_id。不再复用 Session State KV，
//    也不再超载 parallel_config_json（旧实现字段语义冲突且 Update 白名单不透传，从未真正持久化）。
// 2. 业务逻辑（internal/biz/spirit_team_usecase.go）：
//    a. WriteDeliverablesToSession：上游 Team 完成时（RecordTeamCompletion 内联调用，
//       保证先于下游调度），提取团队输出摘要写入 deliverables_output_json
//    b. readDeliverableOutput：读取已持久化的交付物输出
//    c. InjectUpstreamDeliverables：DAG 激活下游 Team 时收集上游交付物，
//       优先读持久化缓存，未命中回退到 ExtractTeamOutput 即时提取
//    d. ExtractTeamOutput 数据源（O-4 修复）：主源为 SpiritStepReader
//       （窄接口，ListStepsBySessionID 精确 session_id 语义，读团队主会话
//       最后一条 completed reply step）；团队主会话按 SessionType=team 识别
//       （成员会话共享 team_id 且 Search 无序）；无 stepReader 或无 reply
//       step 时回退 ListMessagesRecent 读 assistant 消息
// 3. 数据格式：Markdown 摘要文本（初期），后续可扩展为结构化 JSON
// 4. 时序保证：service 层 HandleTeamTurnResult 中 recordTeamCompletion（落库）
//    先于 scheduleDependentTeams / PlanExecutor.NotifyTeamCompletion（下游派发）

// 交付物注入下游 Team 的具体方式：
// 1. 注入位置：作为下游 Team 首个 Turn 的 User Message 前缀
//    格式：
//    """
//    --- 上游交付物 ---
//    ## 上游团队: {team_display_name}
//    {deliverable_content}
//    --- 请基于以上上游交付物执行任务 ---
//    """
// 2. 注入时机与路径（双路径）：
//    a. 生产主路径（v2）：PlanExecutor.dagRun → RealTeamOrchestrator.Orchestrate
//       → 建队时透传 step.DependsOn（P0-① 降级：形式契约待 P1 planner schema 扩展）
//       → StartTeamTurn 前组装 turnContent = 前缀 + taskDesc（存储的 TaskDescription 保持纯净）
//    b. 备份路径（v1）：TeamStarter.scheduleDependentTeams
//       → biz ScheduleDependentTeams 返回 DependentTeamAction.TaskDescription
//         （前缀 + 原任务描述），service 直接用其启动 Turn
// 3. 后续迭代（P1）：planner 输出 DeliverableContract，填充 Team.Deliverables/InputContract
//    并启用契约匹配校验；支持注入到 Graph 的 StateFields（结构化数据传递）
```

### 4.7 Spirit 编排管线适配

**现有三阶段管线**：
1. TaskPlanner → 2. AgentAllocator → 3. TaskOrchestrator

**适配变更**：

| 阶段 | 变更 | 说明 |
|------|------|------|
| TaskPlanner | 识别跨部门需求 | 拆解任务时标注部门归属 |
| AgentAllocator | 按部门匹配 Agent | 匹配时考虑 position → department 映射 |
| TaskOrchestrator | 组建 Team DAG + 契约验证 + 部门主管注入 | 新增三个步骤 |

**TaskOrchestrator 扩展流程**：

```
1. 组建 Team DAG（现有逻辑）
2. [新增] 验证交付物契约匹配
3. [新增] 为每个 Team 注入部门主管
4. [新增] 为跨部门边添加 verification_gate
5. 执行 DAG（现有逻辑）
6. [新增] 审批门禁触发 → 部门主管审批
7. [新增] 驳回返工 / 升级处理
8. 合成结果（现有逻辑）
```

---

## 四-B、Spirit 编排运行时融合分析

### 与现有编排规则的兼容性

| 现有机制 | 新设计影响 | 兼容性评估 |
|----------|-----------|-----------|
| TaskPlanner 三阶段编排 | 无影响 | Plan/Allocate/Orchestrate 流程不变 |
| AgentAllocator Agent 匹配 | 需扩展：按部门+岗位匹配 | 新增 `department_id` 过滤维度，不影响现有匹配逻辑 |
| TaskOrchestrator 5 种策略 | 需扩展：DAG 策略增加跨部门约束 | 现有策略不变，新增部门主管注入和交付物传递 |
| SpiritTeamAssembler 组装 | 需扩展：注入部门主管 + 跨部门成员 | 组装流程不变，新增成员注入步骤 |
| DAG 依赖调度 | 需扩展：交付物传递 + 审批门禁 | 调度逻辑不变，新增门禁检查步骤 |
| Graph 编译和执行 | 无影响 | Graph 运行时不变，verification_gates 是编译时注入 |
| 结果合成 | 无影响 | SynthesisEngine 不变 |

### 需要声明的模块协作

| 协作点 | 声明方式 | 说明 |
|--------|----------|------|
| Team ↔ Organization | Team.department_id FK | Team 必须声明归属部门 |
| Team ↔ Agent (跨部门) | Team.cross_dept_member_ids + BorrowRequest | 跨部门成员需声明借调关系 |
| Team ↔ Graph | Team.linked_graph_id + Graph.team_id | 双向引用需声明 |
| Team ↔ Team (DAG) | Team.depends_on + deliverables/input_contract | DAG 依赖需声明交付物契约 |
| DeptLead ↔ Team | Team.dept_lead_agent_id | 部门主管自动注入需声明 |
| Spirit → DeptLead | VerificationGate | 审批门禁需在 Graph 中声明 |

### 编排流程变更（增量）

现有 Spirit 编排流程不变，新增以下步骤：

**Phase 2 (Allocate) 扩展**：
1. 原有：按 RequiredCapabilities 匹配 Agent
2. 新增：按 department_id 过滤可用 Agent（同部门优先）
3. 新增：跨部门 Agent 需检查借调审批状态

**Phase 3 (Orchestrate) 扩展**：
1. 原有：SpiritTeamAssembler.AssembleTeam()
2. 新增：EnsureDeptLeadInTeam() — 自动注入部门主管
3. 新增：InjectVerificationGates() — 注入审批门禁节点
4. 新增：ValidateDeliverableContracts() — 验证 DAG 交付物契约

**DAG 调度扩展**：
1. 原有：ScheduleDependentTeams() — 前置完成后激活后续
2. 新增：PassDeliverables() — 将上游交付物注入下游初始输入
3. 新增：ExecuteVerificationGates() — 执行审批门禁

---

## 五、Scenario 层设计

### 5.1 organization.yaml（原 taxonomy.yaml）

> **代码锚点**：`internal/scenario/organization.yaml`、`internal/scenario/loader/organization_loader.go`

```yaml
# organization.yaml (原 taxonomy.yaml)
# 顶层 key 为 companies（复数），支持多公司预留
# 加载器 LoadOrganizationSpec 优先读 organization.yaml，回退到 taxonomy.yaml

companies:
  - key: finance                    # 公司 key
    name: 金融
    icon: trending_up
    description: 量化交易、风险管理等金融行业场景
    sort_order: 1
    departments:                     # 部门列表
      - key: quant_trading
        name: 量化交易
        description: 量化策略研发与交易执行
        sort_order: 1
        positions:                   # 岗位列表
          - key: quant_researcher
            name: 量化研究员
            description: 因子挖掘、回测验证与策略研发
            sort_order: 1
            seniority_level: mid
            skills_required:
              - 因子挖掘
              - 回测验证
            responsibilities:
              - 因子挖掘与策略研发
            variants:
              - key: alpha
                name: Alpha因子
```

**注意**：Team 归属部门通过 `agents.yaml` 中的 `CompanySpec` 结构管理（见 5.2），不在 `organization.yaml` 中定义 Team。

### 5.2 Spec 结构变更

> **代码锚点**：`internal/scenario/loader/organization_loader.go`、`internal/scenario/loader/spec.go`、`internal/scenario/loader/company_loader.go`

**organization.yaml 加载结构**（`organization_loader.go`）：

```go
// internal/scenario/loader/organization_loader.go
type OrganizationSpec struct {
    Companies       []OrgCompanySpec `yaml:"companies"`
    // LegacyIndustries 支持读取旧 "industries" key，向后兼容
    LegacyIndustries []OrgCompanySpec `yaml:"industries"`
}

type OrgCompanySpec struct {
    Key         string              `yaml:"key"`
    Name        string              `yaml:"name"`
    Icon        string              `yaml:"icon"`
    Description string              `yaml:"description"`
    SortOrder   int                 `yaml:"sort_order"`
    Departments []OrgDepartmentSpec `yaml:"departments"`
}

type OrgDepartmentSpec struct {
    Key         string            `yaml:"key"`
    Name        string            `yaml:"name"`
    Description string            `yaml:"description"`
    SortOrder   int               `yaml:"sort_order"`
    Positions   []OrgPositionSpec `yaml:"positions"`
}

type OrgPositionSpec struct {
    Key              string           `yaml:"key"`
    Name             string           `yaml:"name"`
    Description      string           `yaml:"description"`
    SortOrder        int              `yaml:"sort_order"`
    SeniorityLevel   string           `yaml:"seniority_level"`
    SkillsRequired   []string         `yaml:"skills_required"`
    Responsibilities []string         `yaml:"responsibilities"`
    Variants         []OrgVariantSpec `yaml:"variants"`
}
```

**agents.yaml 加载结构**（`spec.go`，原 `IndustrySpec` → `CompanySpec`）：

```go
// internal/scenario/loader/spec.go
type CompanySpec struct {
    CompanyKey string        `yaml:"company_key"`    // 原 IndustryKey
    Defaults   AgentDefaults `yaml:"defaults"`
    Agents     []AgentSpec   `yaml:"agents"`
    Teams      []TeamSpec    `yaml:"teams"`
}

// TeamSpec 未新增 DepartmentKey 字段
// Team 的 department_id 通过运行时从 Organization 树解析设置
type TeamSpec struct {
    Key            string        `yaml:"key"`
    DisplayName    string        `yaml:"display_name"`
    Mode           string        `yaml:"mode"`
    // ... 其他字段
    Members        []TeamMemberSpec `yaml:"members"`
    Graph          *GraphSpec    `yaml:"graph"`
}
```

**加载器**：
- `LoadOrganizationSpec(scenarioDir)` — 加载 `organization.yaml`（回退 `taxonomy.yaml`），返回 `OrganizationSpec`
- `LoadCompanySpec(scenarioDir, companyKey)` — 加载 `{scenarioDir}/{companyKey}/agents.yaml`，返回 `CompanySpec`

**映射变更**:
- `spec.IndustryKey` → `spec.CompanyKey`（公司级 key）
- `CategoryIndustryID: spec.IndustryKey` → `DepartmentID`（Team 级别映射，运行时从 Organization 树解析）

### 5.3 agents.yaml 适配

```yaml
# position_key 保持不变，语义从 taxonomy position 变为 organization position
agents:
  - key: go-senior-general
    position_key: go_engineer      # 引用 organization position
    variant: general
    display_name: Go 高级工程师
```

### 5.4 部门主管 Prompt

> **代码锚点**：`internal/scenario/system/prompts/dept_lead.md`

```markdown
# 部门主管

你是「{{.DepartmentName}}」的部门主管。

## 职责

1. **资源协调**：管理本部门的人力资源分配，审批跨部门借调请求
2. **质量把关**：审核本部门产出的交付物质量
3. **验收确认**：确认其他部门交付给本部门的工作是否满足需求

## 审批规则

- 跨部门交付物需要双方主管确认（输出方质量把关 + 接收方验收确认）
- 借调成员加入其他 Team 时，你需要审批同意
- 你自动加入本部门的所有 Team
- 借调请求超过 5 分钟未处理，系统自动批准

## 部门信息

- 部门名称：{{.DepartmentName}}
- 部门描述：{{.DepartmentDescription}}
```

**模板变量**：`{{.DepartmentName}}`、`{{.DepartmentDescription}}` 在 `DeptLeadManager.buildDeptLeadSystemPrompt` 中填充。

---

## 六、前端设计

### 6.1 目录结构现状

> **现状**：前端组织架构管理位于 `web/src/features/platform/`（非 `industries/` 或 `organization/`），使用 `useTaxonomyPage.ts` 等组合式函数。API 层在 `web/src/features/platform/api.ts` 中同时支持 `createOrganizationService`、`createTaxonomyService`、`createIndustryTaxonomyService` 三套服务（兼容层）。

```
web/src/features/platform/
  api.ts                    # 同时支持 Organization/Taxonomy/IndustryTaxonomy 三套 API
  useTaxonomyPage.ts        # 组织架构管理页面逻辑（CATEGORY_RESOURCE = 'organization'）
  taxonomyTreeUtils.ts      # 树形结构工具
  taxonomyLabels.ts         # 标签工具
  types.ts
```

### 6.2 页面变更（计划）

| 旧页面/组件 | 新页面/组件 | 说明 | 状态 |
|--------|--------|------|------|
| `useTaxonomyPage.ts` | `useOrganizationPage.ts` | 重命名（可选） | ⏳ 未实施 |
| `taxonomyTreeUtils.ts` | `organizationTreeUtils.ts` | 重命名（可选） | ⏳ 未实施 |

### 6.3 新增页面/组件（计划）

| 组件 | 说明 | 状态 |
|------|------|------|
| `DeptLeadConfigDialog.vue` | 部门主管配置（替换、Prompt 覆盖） | ⏳ 未实施 |
| `TeamDeliverableEditor.vue` | Team 交付物/契约编辑器 | ⏳ 未实施 |
| `VerificationGateConfig.vue` | 审批门禁配置（Graph 编辑器中） | ⏳ 未实施 |
| `CrossDeptDAGView.vue` | 跨部门 Team DAG 可视化 | ⏳ 未实施 |

### 6.4 i18n 变更清单

| 语言 | 文件 | 涉及条目数 | 关键变更 | 状态 |
|------|------|-----------|----------|------|
| zh-CN | `web/src/i18n/locales/zh-CN.ts` | 1+ 处 | `organization: '组织架构'` 已存在 | 🟡 部分完成 |
| en-US | `web/src/i18n/locales/en-US.ts` | ~11 处 | `taxonomy` → `organization` 等 | ⏳ 未实施 |

**i18n key 重命名策略**：保持 key 层级结构不变，仅替换 `taxonomy` → `organization`、`industry` → `company`/`department` 等前缀。

### 6.5 新增前端组件（计划）

| 组件 | 位置 | 功能 | 依赖 | 状态 |
|------|------|------|------|------|
| `CrossDeptDAGView.vue` | `web/src/features/teams/components/` | 跨部门 Team DAG 可视化 | Phase 3 后端 API | ⏳ 未实施 |
| `BorrowApprovalDialog.vue` | `web/src/features/platform/components/` | 借调审批对话框 | Phase 2 后端 API | ⏳ 未实施 |
| `BorrowStatusBadge.vue` | `web/src/features/teams/components/` | 借调审批状态徽章 | Phase 2 后端 API | ⏳ 未实施 |

---

## 七、迁移设计

### 7.1 数据库迁移

> **实现状态**：通过 Ent Auto-Migration（`Schema.Create()`）自动处理表结构和字段变更，未单独编写 DDL 迁移脚本。以下 SQL 为概念性说明，实际由 Ent 自动执行。

```sql
-- 概念性说明（实际由 Ent Auto-Migration 自动执行）
-- Step 1: 重命名表（Ent 通过 Schema Annotation 自动处理）
-- industry_taxonomy → organizations

-- Step 2: 重命名字段（Ent Auto-Migration 自动处理）
-- taxonomy_key → org_key

-- Step 3: 更新 level 值（需数据迁移脚本，当前未实施 MG-02）
-- UPDATE organizations SET level = 'company' WHERE level = 'industry';

-- Step 4: 新增字段（Ent Auto-Migration 自动处理）
-- dept_lead_agent_id, dept_lead_config_json

-- Step 5: Agent 字段重命名（Ent Auto-Migration 自动处理）
-- taxonomy_position_id → position_id

-- Step 6: Team 字段变更（Ent Auto-Migration 自动处理新增字段）
-- category_industry_id → department_id（需数据迁移，当前未实施 MG-02）

-- Step 7: Graph 新增字段（Ent Auto-Migration 自动处理）
-- team_id, is_template, verification_gates
```

**注意**：数据迁移（level 值更新、category_industry_id → department_id 映射）尚未实施（MG-02 ⏳）。

### 7.2 Wire 注入链路变更

| 文件 | 变更 | 说明 | 状态 |
|------|------|------|------|
| `internal/biz/biz.go` | `NewTaxonomyUsecase` → `NewOrganizationUsecase` | ProviderSet 更新 | ✅ |
| `internal/data/data.go` | `NewTaxonomyRepo` → `NewOrganizationRepo` | ProviderSet 更新 | ✅ |
| `internal/service/organization.go` | 新增 OrganizationService 实现 | Service 新增 | ✅ |
| `internal/service/taxonomy.go` | 保留作为兼容层，代理到 OrganizationUsecase | 兼容层 | ✅ |
| `internal/agent/builder_deps.go` | `TaxonomyUsecase` → `OrganizationUsecase` | 依赖注入更新 | ✅ |
| `internal/service/chat_orchestrator.go` | `TaxonomyUsecase` → `OrganizationUsecase` | 依赖注入更新 | ✅ |
| `internal/agent/prompt.go` | `Taxonomy *biz.TaxonomyUsecase` → `Organization *biz.OrganizationUsecase` | 依赖注入更新 | ✅ |
| `cmd/admin/wire.go` | 无需手动修改 | Wire 自动生成 wire_gen.go | ✅ |

### 7.3 API 兼容层

```go
// internal/service/taxonomy.go
// 旧 TaxonomyService / IndustryTaxonomyService 代理到新 OrganizationUsecase
// 兼容层保留旧 RPC 端点，内部转发到 OrganizationUsecase，字段名做映射
// 同时支持 createTaxonomyService / createIndustryTaxonomyService（前端兼容）
```

### 7.3 前端路由兼容

```typescript
// web/src/router/routes.ts
// 旧 /settings/taxonomy 重定向到 /settings/organization
{ path: 'settings/taxonomy', redirect: '/settings/organization' },
{ path: 'settings/organization', name: 'organization', component: OrganizationPage },
```

### 7.4 Pack 导入链路适配

| 文件 | 变更 | 说明 | 状态 |
|------|------|------|------|
| `internal/data/seed_pack.go` | `SeedPackIndustry` 函数名保留（内部调用 `LoadCompanySpec`） | 函数名未重命名，内部实现已适配 | 🟡 部分完成（`pack.ConvertCompanySpecToPack` 未实现） |
| `internal/scenario/loader/loader.go` | `LoadIndustrySpec` → `LoadOrganizationSpec` + `LoadCompanySpec` | 加载器拆分 | ✅ |
| `internal/scenario/loader/company_loader.go` | 新增 `LoadCompanySpec` | 新增加载器 | ✅ |
| `internal/scenario/loader/organization_loader.go` | 新增 `LoadOrganizationSpec` + `OrganizationSpec`/`OrgCompanySpec`/`OrgDepartmentSpec`/`OrgPositionSpec` 结构体 | 新增加载器 | ✅ |
| `internal/scenario/loader/spec.go` | `IndustrySpec` → `CompanySpec`（`IndustryKey` → `CompanyKey`） | 结构体重命名 | ✅ |
| `internal/data/ecosystem_preset.go` | `taxonomy_position_id` 硬编码 SQL → `position_id` | SQL 列名更新 | ✅ |
| `internal/biz/pack/exporter.go` | `agent.TaxonomyPositionID` → `agent.PositionID` | 字段引用更新 | ✅ |

### 7.5 TaxonomyAncestors 引用迁移

| 文件 | 行号 | 变更 |
|------|------|------|
| `internal/biz/pack/exporter.go` | 27 | `GetTaxonomyAncestors` → `GetOrgAncestors` |
| `internal/biz/pack/exporter.go` | 210 | `agent.TaxonomyPositionID` + `ancestors` 使用 → `agent.PositionID` + `ancestors` |
| `internal/data/pack_repo.go` | 70-89 | `GetTaxonomyAncestors` 实现 → `GetOrgAncestors` |

---

## 八、影响域

| 包 | 变更类型 | 说明 |
|----|----------|------|
| `internal/data/ent/schema/` | 重命名+修改 | industry_taxonomy.go → organization.go，字段变更 |
| `internal/biz/` | 重命名+新增 | taxonomy.go → organization.go，新增 dept_lead/deliverable_contract/verification_gate |
| `internal/service/` | 新增+保留兼容层 | 新增 organization.go，保留 taxonomy.go 作为兼容层 |
| `internal/scenario/` | 修改 | YAML 结构变更，新增部门主管 Prompt |
| `internal/data/seed_*.go` | 修改 | 种子数据适配新结构 |
| `api/kratos/` | 新增+修改 | 新增 organization proto，修改 agent/team/graph proto |
| `web/src/features/platform/` | 修改（未重命名） | 仍在 platform/ 下，未重命名为 organization/，API 层支持三套服务 |
| `web/src/pages/` | 重命名+修改 | 页面重命名，新增页面 |

---

## 九、非功能需求补充

| NFR ID | 类型 | 需求 | 验收标准 |
|--------|------|------|----------|
| NFR-06 | 安全性 | 跨部门借调需来源部门主管审批，审批不可伪造 | 审批记录有 approved_by 字段，不可篡改 |
| NFR-07 | 安全性 | 部门主管操作需审计日志 | 所有审批/拒绝操作记录到 audit log |
| NFR-08 | 可扩展性 | 单公司 Organization 树规模上限 | 部门 ≤ 50，岗位 ≤ 200，Agent ≤ 500 |
| NFR-09 | 并发性 | 多 Team 同时请求同一部门主管审批 | 审批请求排队，不丢失 |
| NFR-10 | 数据一致性 | 跨部门成员审批状态与 Team 成员列表一致 | 审批通过后才加入 Team.members |
| NFR-11 | 可用性 | 部门主管 Agent 故障时降级 | 超时自动通过（5 分钟），不阻塞业务 |
