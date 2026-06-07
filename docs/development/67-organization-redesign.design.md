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
web/src/features/organization/                   ← 原 industries/
```

**红线**：
- `internal/biz` 不 import `pkg/trpc-agent-go`
- 部门主管的审批逻辑在 biz 层，不在 service 层
- 交付物契约验证为提示性，不阻断 Team 组建

---

## 二、数据模型设计

### 2.1 Organization（原 IndustryTaxonomy）

**表名变更**：`industry_taxonomy` → `organizations`

**字段变更**：

| 字段 | 变更 | 说明 |
|------|------|------|
| `level` | 值变更 | `"industry"` → `"company"`，`"department"` / `"position"` 不变 |
| `dept_lead_agent_id` | **新增** | string, 可空，仅 department 级节点使用 |
| `dept_lead_config_json` | **新增** | text, 默认 `"{}"`，部门主管配置覆盖 |

**Ent Schema 关键定义**：

```go
// internal/data/ent/schema/organization.go
func (Organization) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").MaxLen(256),
        field.String("org_key").MaxLen(512).Unique(),    // 原 taxonomy_key
        field.String("name").MaxLen(1024),
        field.Text("description").Optional(),
        field.String("status").Default("active"),
        field.Bool("enabled").Default(true),
        field.Int("sort_order").Default(0),
        field.String("parent_id").Default(""),            // 自引用，树形结构
        field.String("level").                           // "company" | "department" | "position"
            Default("company").
            Validate(func(s string) error {
                return validation.In(s, "company", "department", "position")
            }),
        field.String("scenario_key").Optional(),
        field.String("workspace_id").Optional(),
        field.String("owner_user_id").Optional(),
        field.Bool("is_system").Default(false),
        field.Text("config_json").Optional(),
        field.Text("metadata_json").Optional(),
        // 新增
        field.String("dept_lead_agent_id").Optional(),   // 部门主管 Agent ID
        field.Text("dept_lead_config_json").Default("{}"), // 部门主管配置
        field.Time("deleted_at").Optional(),
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

| 旧字段 | 新字段 | 说明 |
|--------|--------|------|
| `category_industry_id` | `department_id` | FK → Organization(department) |
| - | `deliverables` | text, 默认 `"[]"`, 交付物定义 JSON |
| - | `input_contract` | text, 默认 `"[]"`, 输入契约 JSON |
| - | `dept_lead_agent_id` | string, 可空, 部门主管（默认从部门继承） |
| - | `cross_dept_member_ids` | text, 默认 `"[]"` | 跨部门成员 Agent ID 列表 JSON |

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

**交付物 Schema**：

```go
// internal/biz/deliverable_contract.go
type DeliverableItem struct {
    Name        string `json:"name"`         // 交付物标识
    Description string `json:"description"`  // 交付物描述
    Format      string `json:"format"`       // 格式: markdown/json/file
    Required    bool   `json:"required"`     // 是否必需
}

type DeliverableContract struct {
    Deliverables   []DeliverableItem `json:"deliverables"`
    InputContract  []DeliverableItem `json:"input_contract"`
}
```

### 2.4 Graph 字段变更

| 新字段 | 类型 | 说明 |
|--------|------|------|
| `team_id` | string, 可空 | 归属 Team，空表示模板 |
| `is_template` | bool, 默认 false | 是否为模板 Graph |
| `verification_gates` | text, 默认 `"[]"` | 审批门禁定义 JSON |

**双向引用规则**：
- Team → Graph：`OrchestrationSpec.linked_graph_id`（权威方向，Team 主动引用 Graph）
- Graph → Team：`team_id`（反向索引，仅用于查询"这个 Graph 属于哪个 Team"）
- 写入时：Team 设置 linked_graph_id 后，系统自动回写 Graph.team_id
- 查询时：通过 Graph.team_id 快速定位归属 Team，无需遍历所有 Team
- 模板 Graph：team_id 为空，linked_graph_id 也不指向它

**双向引用回写实现**：
```go
// 在 TeamUsecase.UpdateTeam 中，当 linked_graph_id 变更时：
func (uc *TeamUsecase) syncGraphTeamID(ctx context.Context, team *Team) error {
    if team.OrchestrationSpec.LinkedGraphID != "" {
        // 回写 Graph.team_id
        return uc.graphUC.UpdateGraphTeamID(ctx, team.OrchestrationSpec.LinkedGraphID, team.ID)
    }
    return nil
}
```

```go
// Team 删除时清理 Graph.team_id
func (uc *TeamUsecase) cleanupGraphTeamID(ctx context.Context, team *Team) error {
    if team.OrchestrationSpec.LinkedGraphID != "" {
        graph, err := uc.graphUC.Get(ctx, team.OrchestrationSpec.LinkedGraphID)
        if err != nil {
            return err
        }
        // 专属 Graph：随 Team 一起删除
        if !graph.IsTemplate {
            return uc.graphUC.Delete(ctx, graph.ID)
        }
        // 模板 Graph：仅清除 team_id 引用
        return uc.graphUC.UpdateGraphTeamID(ctx, graph.ID, "")
    }
    return nil
}
```

**审批门禁 Schema**：

```go
// internal/biz/verification_gate.go
type VerificationGate struct {
    NodeID       string `json:"node_id"`        // Graph 节点 ID
    GateType     string `json:"gate_type"`       // "dept_lead_approval"
    DepartmentID string `json:"department_id"`   // 审批部门
    Description  string `json:"description"`     // 门禁描述
    MaxRetries   int    `json:"max_retries"`     // 最大重试次数，默认 3
    Escalation   string `json:"escalation"`      // 升级策略: "notify_user" | "auto_approve"
}

// 借调审批门禁（部门主管审批借出请求）
type BorrowApprovalGate struct {
    GateType           string `json:"gate_type"`            // "borrow_approval"
    SourceDepartmentID string `json:"source_department_id"` // 借出部门
    AgentID            string `json:"agent_id"`             // 被借调的 Agent
    TimeoutSeconds     int    `json:"timeout_seconds"`      // 超时时间，默认 300s
    AutoApproveOnTimeout bool  `json:"auto_approve_on_timeout"` // 超时自动通过，默认 true
}

// 跨部门交付物审批需要双方主管确认
type CrossDeptDeliveryGate struct {
    GateType              string `json:"gate_type"`               // "cross_dept_delivery"
    OutputDepartmentID    string `json:"output_department_id"`    // 输出方部门
    ReceivingDepartmentID string `json:"receiving_department_id"` // 接收方部门
    DeliverableName       string `json:"deliverable_name"`        // 交付物名称
    Description           string `json:"description"`             // 门禁描述
    MaxRetries            int    `json:"max_reries"`              // 最大重试次数，默认 3
}

// 审批流程：
// 1. 输出方部门主管：质量把关（输出是否符合本部门标准）
// 2. 接收方部门主管：验收确认（输出是否满足本部门输入需求）
// 3. 两方都通过 → 交付物传递到下游 Team
// 4. 任一方驳回 → 上游 Team 返工
// 5. 审批顺序：输出方先审（质量），接收方再审（验收）
```

---

## 三、Proto API 设计

### 3.1 OrganizationService

```protobuf
// api/kratos/organization/v1/organization.proto

service OrganizationService {
  rpc ListOrgNodes(ListOrgNodesRequest) returns (ListOrgNodesResponse) {
    option (google.api.http) = { get: "/v1/organization" };
  }
  rpc GetOrgTree(GetOrgTreeRequest) returns (GetOrgTreeResponse) {
    option (google.api.http) = { get: "/v1/organization/tree" };
  }
  rpc CreateOrgNode(CreateOrgNodeRequest) returns (OrgNode) {
    option (google.api.http) = { post: "/v1/organization" };
  }
  rpc GetOrgNode(GetOrgNodeRequest) returns (OrgNode) {
    option (google.api.http) = { get: "/v1/organization/{id}" };
  }
  rpc UpdateOrgNode(UpdateOrgNodeRequest) returns (OrgNode) {
    option (google.api.http) = { put: "/v1/organization/{id}" };
  }
  rpc DeleteOrgNode(DeleteOrgNodeRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/organization/{id}" };
  }
  rpc ReorderOrgNodes(ReorderOrgNodesRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { put: "/v1/organization/reorder" };
  }
}

message OrgNode {
  string id = 1;
  string org_key = 2;
  string name = 3;
  string description = 4;
  string parent_id = 5;
  string level = 6;           // "company" | "department" | "position"
  string status = 7;
  int32 sort_order = 8;
  string config_json = 9;
  string metadata_json = 10;
  bool is_system = 11;
  // 部门级专用
  string dept_lead_agent_id = 12;
  string dept_lead_config_json = 13;
  // 岗位级专用
  string scenario_key = 14;
  // 树形辅助
  repeated OrgNode children = 20;
}
```

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

```go
// internal/biz/organization.go
type OrganizationUsecase struct {
    repo    OrganizationRepo
    logger  *loggateway.Logger
}

// 核心方法（原 TaxonomyUsecase 14 个方法，全部迁移并重命名）
func (uc *OrganizationUsecase) List(ctx context.Context) ([]OrgNode, error)
func (uc *OrganizationUsecase) Tree(ctx context.Context) ([]OrgTreeNode, error)
func (uc *OrganizationUsecase) Get(ctx context.Context, id string) (OrgNode, error)
func (uc *OrganizationUsecase) Create(ctx context.Context, in OrgNode) (OrgNode, error)
func (uc *OrganizationUsecase) Update(ctx context.Context, id string, patch OrgNode) (OrgNode, error)
func (uc *OrganizationUsecase) Delete(ctx context.Context, id string) error
func (uc *OrganizationUsecase) ListByLevel(ctx context.Context, level string) ([]OrgNode, error)
func (uc *OrganizationUsecase) ListByParentID(ctx context.Context, parentID string) ([]OrgNode, error)
func (uc *OrganizationUsecase) GetByKey(ctx context.Context, key string) (OrgNode, error)
func (uc *OrganizationUsecase) Reorder(ctx context.Context, ids []string) error
func (uc *OrganizationUsecase) GetAncestors(ctx context.Context, positionID string) (OrgAncestors, error)
func (uc *OrganizationUsecase) GetPositionPrompt(ctx context.Context, companyKey, positionKey, variant string) (PositionPromptResult, error)
func (uc *OrganizationUsecase) ListPositionVariants(ctx context.Context, companyKey, positionKey string) ([]VariantInfo, error)
func (uc *OrganizationUsecase) BuildResponsibility(ctx context.Context, positionID string, mode string) (string, error)
// 注意：GetPositionPrompt/ListPositionVariants 的 industryKey 参数重命名为 companyKey

// OrgAncestors 结构体（原 TaxonomyAncestors）
type OrgAncestors struct {
    Company    OrgNode // 原 Industry
    Department OrgNode
    Position   OrgNode
}

// OrganizationRepo 接口（原 TaxonomyRepo，10 个方法）
type OrganizationListReader interface {
    ListOrgNodes(ctx context.Context) ([]OrgNode, error)
    ListOrgNodesByLevel(ctx context.Context, level string) ([]OrgNode, error)
    ListOrgNodesByParentID(ctx context.Context, parentID string) ([]OrgNode, error)
}

type OrganizationItemReader interface {
    GetOrgNode(ctx context.Context, id string) (OrgNode, error)
    GetOrgNodeByKey(ctx context.Context, key string) (OrgNode, error)
    GetOrgNodeByKeyAnyState(ctx context.Context, key string) (OrgNode, error)
}

type OrganizationWriter interface {
    CreateOrgNode(ctx context.Context, c OrgNode) (OrgNode, error)
    UpdateOrgNode(ctx context.Context, c OrgNode) (OrgNode, error)
    DeleteOrgNode(ctx context.Context, id string) error
    ReorderOrgNodes(ctx context.Context, ids []string) error
}

type OrganizationRepo = OrganizationListReader + OrganizationItemReader + OrganizationWriter
```

### 4.2 DeptLeadManager（部门主管管理）

```go
// internal/biz/dept_lead.go
type DeptLeadManager struct {
    agentRepo  AgentRepo
    orgRepo    OrganizationRepo
    logger     *loggateway.Logger
}

// 创建部门主管
func (m *DeptLeadManager) CreateDeptLead(ctx context.Context, dept *OrgNode) (*Agent, error)
    // 1. 生成 Agent Key: "__dept_lead_{dept.OrgKey}__"
    // 2. 生成 Agent ID: "agent___dept_lead_{dept.OrgKey}__"
    // 3. 设置 kind=system_builtin, source=system
    // 4. 设置 position_id 指向部门管理岗
    // 5. 加载部门主管 Prompt 模板
    // 6. 设置 tools_profile = "dept_lead"（部门主管专用工具集）
    //    初期工具列表：无（纯 LLM 判断）
    //    后续可添加：query_team_status, review_deliverable, escalate_to_spirit
    // 7. 创建 Agent 记录

// 删除部门主管
func (m *DeptLeadManager) DeleteDeptLead(ctx context.Context, deptID string) error

// 替换部门主管（用户自定义）
func (m *DeptLeadManager) ReplaceDeptLead(ctx context.Context, deptID string, newAgentID string) error
    // 更新 Organization.dept_lead_agent_id

// 借调请求
type BorrowRequest struct {
    AgentID             string    `json:"agent_id"`
    SourceDepartmentID  string    `json:"source_department_id"`
    TargetTeamID        string    `json:"target_team_id"`
    TargetDepartmentID  string    `json:"target_department_id"`
    Role                string    `json:"role"`
    ApprovalStatus      string    `json:"approval_status"` // "pending" | "approved" | "rejected"
    ApprovedBy          string    `json:"approved_by"`
    RequestedAt         time.Time `json:"requested_at"`
}

// 自动加入 Team
func (m *DeptLeadManager) EnsureDeptLeadInTeam(ctx context.Context, team *Team) error
    // 检查 Team.members 是否包含部门主管
    // 不包含则自动添加

// 审批借调请求
func (m *DeptLeadManager) ApproveBorrowRequest(ctx context.Context, req *BorrowRequest) error
    // 1. 验证请求合法性（被借调 Agent 确实属于本部门）
    // 2. 更新 cross_dept_members 中的 approval_status 为 "approved"
    // 3. 正式将 Agent 加入 Team.members
    // 4. 发布 BorrowApproved 事件
    // 初期实现策略：
    // - 借调请求创建后自动进入 pending 状态
    // - 5 分钟超时后自动通过（AutoApproveExpiredBorrowRequests）
    // - 不实现实时审批 UI（后续迭代）
    // - 后续迭代：部门主管收到通知后可主动审批/拒绝

// 拒绝借调请求
func (m *DeptLeadManager) RejectBorrowRequest(ctx context.Context, req *BorrowRequest, reason string) error
    // 1. 更新 cross_dept_members 中的 approval_status 为 "rejected"
    // 2. 从 Team.members 中移除该 Agent（如果已临时加入）
    // 3. 发布 BorrowRejected 事件

// 借调超时自动通过
func (m *DeptLeadManager) AutoApproveExpiredBorrowRequests(ctx context.Context) error
    // 1. 查找所有 approval_status = "pending" 且创建时间超过 5 分钟的借调请求
    // 2. 自动通过
    // 3. 发布 BorrowAutoApproved 事件

// 查看被借调成员的工作状态（只读）
func (m *DeptLeadManager) GetBorrowedMemberStatus(ctx context.Context, deptID string) ([]BorrowedMemberStatus, error)
    // 1. 查找本部门被借调到其他 Team 的所有 Agent
    // 2. 获取这些 Agent 所在 Team 的执行状态和最近输出
    // 3. 返回只读视图（不可修改 Team 状态）

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
type DeliverableContractValidator struct {
    logger *loggateway.Logger
}

// 验证上下游契约匹配（提示性，不阻断）
func (v *DeliverableContractValidator) ValidateMatch(
    upstream *DeliverableContract,
    downstream *DeliverableContract,
) *ContractMatchResult
    // 1. 遍历 downstream.InputContract
    // 2. 在 upstream.Deliverables 中查找 name 匹配
    // 3. 检查 format 兼容性
    // 4. 返回匹配结果（matched/unmatched/warnings）

type ContractMatchResult struct {
    Matched   []ContractMatch   `json:"matched"`
    Unmatched []ContractGap     `json:"unmatched"`   // 下游需要但上游未提供
    Warnings  []ContractWarning `json:"warnings"`     // 格式不匹配等
}
```

### 4.4 VerificationGateExecutor（审批门禁执行）

```go
// internal/biz/verification_gate.go
type VerificationGateExecutor struct {
    agentRepo AgentRepo
    logger    *loggateway.Logger
}

// 执行审批门禁
func (e *VerificationGateExecutor) ExecuteGate(
    ctx context.Context,
    gate *VerificationGate,
    teamOutput string,
) (*GateResult, error)
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
    Approved   bool   `json:"approved"`
    Reason     string `json:"reason"`      // 驳回理由
    RetryCount int    `json:"retry_count"` // 当前重试次数
}
```

### 4.5 审批驳回返工模型

```go
// 驳回返工策略：
// - 初期方案：重新执行整个 Team（简单可靠）
//   1. 部门主管驳回 → 标记 Team 状态为 "rework"
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

type ReworkStrategy string
const (
    ReworkStrategyFullTeam  ReworkStrategy = "full_team"   // 重新执行整个 Team
    ReworkStrategyPartial   ReworkStrategy = "partial"     // 部分重执行（后续迭代）
)
```

### 4.6 交付物传递机制

```go
// Team 间交付物传递设计：
// 1. 底层存储：复用 Session State KV（GetSessionState/SaveSessionState），无需新建存储层
// 2. 业务逻辑：需新建交付物读写逻辑（非已有机制），包括：
//    a. DeliverableWriter：上游 Team 完成后，将输出写入 Session State
//       key: "deliverable:{team_id}:{deliverable_name}"
//       value: 交付物内容（Markdown 或 JSON）
//    b. DeliverableReader：DAG 调度激活下游 Team 时，读取上游交付物
//    c. DeliverableInjector：将上游交付物注入下游 Team 的初始输入
// 3. 数据格式：Markdown 文本（初期），后续可扩展为结构化 JSON
// 4. 注意：Session State KV 是底层存储原语，交付物读写的业务逻辑完全需要新建

// 交付物注入下游 Team 的具体方式：
// 1. 注入位置：作为下游 Team 的 User Message 前缀
//    格式：
//    """
//    [上游交付物]
//    来源团队: {team_name} ({team_id})
//    交付物: {deliverable_name}
//    ---
//    {deliverable_content}
//    ---
//    请基于以上上游交付物继续执行任务。
//    """
// 2. 注入时机：DAG 调度激活下游 Team 时，在 StartTeamTurn 之前注入
// 3. 注入路径：DeliverableInjector.InjectUpstreamDeliverables()
//    → 读取上游 Team 的 Session State 中的交付物
//    → 构造 User Message 前缀
//    → 传入 SpiritTeamParams.TaskDescription 作为初始输入
// 4. 后续迭代：支持注入到 Graph 的 StateFields（结构化数据传递）
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

### 5.1 organization.yaml（原 industry.yaml）

```yaml
# organization.yaml (原 industry.yaml)
# 新增 department_key 字段在 Team 级别，而非 Industry 级别
# 原因：一个 company 下有多个 department，Team 归属某个 department

organization:
  company_key: tech_corp          # 原 industry_key
  display_name: 技术公司

  departments:                     # 新增：显式定义部门
    - key: rd
      display_name: 研发部
    - key: design
      display_name: 设计部

  agents:
    - key: go-senior-general
      position_key: go_engineer
      variant: senior_general
      # ... 同原结构

  teams:
    - key: xx_product_dev
      display_name: XX产品开发组
      department_key: rd           # 新增：Team 归属的部门
      mode: coordinator
      members:
        - agent_key: go-senior-general
        - agent_key: vue-senior-general
        - agent_key: ui-designer-zhou
          cross_dept: true
          source_department: design
      graph:
        layout: sequential
        nodes:
          - id: start
            type: start
          - id: dev
            type: agent
            agent_key: go-senior-general
          - id: end
            type: end
        edges:
          - source: start
            target: dev
          - source: dev
            target: end
```

### 5.2 Spec 结构变更

**原结构** (`IndustrySpec`):
```go
type IndustrySpec struct {
    IndustryKey string       `yaml:"industry_key"`
    Defaults    AgentDefaults `yaml:"defaults"`
    Agents      []AgentSpec  `yaml:"agents"`
    Teams       []TeamSpec   `yaml:"teams"`
}
```

**新结构** (`OrganizationSpec`):
```go
type OrganizationSpec struct {
    CompanyKey  string           `yaml:"company_key"`    // 原 IndustryKey
    DisplayName string           `yaml:"display_name"`
    Departments []DepartmentSpec `yaml:"departments"`    // 新增
    Defaults    AgentDefaults    `yaml:"defaults"`
    Agents      []AgentSpec      `yaml:"agents"`
    Teams       []TeamSpec       `yaml:"teams"`
}

type DepartmentSpec struct {
    Key         string `yaml:"key"`
    DisplayName string `yaml:"display_name"`
}

// TeamSpec 新增字段
type TeamSpec struct {
    // ... 原有字段不变
    DepartmentKey string `yaml:"department_key"` // 新增：归属部门
}
```

**映射变更**:
- `spec.IndustryKey` → `spec.CompanyKey`（公司级 key）
- `CategoryIndustryID: spec.IndustryKey` → `DepartmentID: teamSpec.DepartmentKey`（Team 级别映射）
- 新增 `Departments` 列表，用于自动创建 Organization 树的 department 节点

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

```markdown
# internal/scenario/system/prompts/dept_lead.md

你是 {department_name} 的部门主管。

## 核心职责
1. 审批本部门 Team 的跨部门交付物
2. 协调本部门资源分配
3. 把关本部门输出质量

## 审批规则
- 仔细审查交付物是否满足下游 Team 的输入契约
- 交付物质量达标 → 通过
- 交付物不达标 → 驳回，附具体改进建议
- 驳回理由必须具体、可操作

## 审批输出格式
```json
{
  "approved": true/false,
  "reason": "审批理由"
}
```
```

---

## 六、前端设计

### 6.1 目录结构变更

```
web/src/features/
  industries/          → organization/         # 重命名
    types.ts           → types.ts              # Industry→Company, Department/Position 不变
    api.ts             → api.ts                # TaxonomyService → OrganizationService
```

### 6.2 页面变更

| 旧页面 | 新页面 | 说明 |
|--------|--------|------|
| `TaxonomyPage.vue` | `OrganizationPage.vue` | 公司架构管理 |
| `IndustryMarketPage.vue` | `OrganizationMarketPage.vue` | 架构市场 |
| `TaxonomyIndustryCard.vue` | `OrgCompanyCard.vue` | 公司卡片 |
| `TaxonomyDepartmentNode.vue` | `OrgDepartmentNode.vue` | 部门节点 |

### 6.3 新增页面/组件

| 组件 | 说明 |
|------|------|
| `DeptLeadConfigDialog.vue` | 部门主管配置（替换、Prompt 覆盖） |
| `TeamDeliverableEditor.vue` | Team 交付物/契约编辑器 |
| `VerificationGateConfig.vue` | 审批门禁配置（Graph 编辑器中） |
| `CrossDeptDAGView.vue` | 跨部门 Team DAG 可视化 |

### 6.4 i18n 变更清单

| 语言 | 文件 | 涉及条目数 | 关键变更 |
|------|------|-----------|----------|
| zh-CN | `web/src/i18n/locales/zh-CN.ts` | 4 处 | `taxonomy: '行业分类'` → `organization: '公司架构'`；`industryMarket` → `orgMarket`；`statCategories` → `statDepartments`；kicker 文案 |
| en-US | `web/src/i18n/locales/en-US.ts` | ~11 处 | `taxonomy: 'Industry Taxonomy'` → `organization: 'Organization'`；`industryMarket` → `orgMarket` |

**i18n key 重命名策略**：保持 key 层级结构不变，仅替换 `taxonomy` → `organization`、`industry` → `company`/`department` 等前缀。

**注意**：zh-CN i18n 文件中实际只有 4 处直接引用，但 Vue 组件中可能存在硬编码的中文文本（如页面标题、空状态文案等），需在 FE-05 实施时逐组件排查。en-US 的 10 处更接近实际变更量。

### 6.5 新增前端组件

| 组件 | 位置 | 功能 | 依赖 |
|------|------|------|------|
| `CrossDeptDAGView.vue` | `web/src/features/teams/components/` | 跨部门 Team DAG 可视化 | Phase 3 后端 API |
| `BorrowApprovalDialog.vue` | `web/src/features/organization/components/` | 借调审批对话框 | Phase 2 后端 API |
| `BorrowStatusBadge.vue` | `web/src/features/teams/components/` | 借调审批状态徽章 | Phase 2 后端 API |

---

## 七、迁移设计

### 7.1 数据库迁移

```sql
-- Step 1: 重命名表
ALTER TABLE industry_taxonomy RENAME TO organizations;

-- Step 2: 重命名字段
ALTER TABLE organizations RENAME COLUMN taxonomy_key TO org_key;

-- Step 3: 更新 level 值
UPDATE organizations SET level = 'company' WHERE level = 'industry';

-- Step 4: 新增字段
ALTER TABLE organizations ADD COLUMN dept_lead_agent_id TEXT;
ALTER TABLE organizations ADD COLUMN dept_lead_config_json TEXT DEFAULT '{}';

-- Step 5: Agent 字段重命名
ALTER TABLE agents RENAME COLUMN taxonomy_position_id TO position_id;

-- Step 6: Team 字段变更
-- 需要程序化迁移：category_industry_id → department_id
-- 逻辑：查找原 industry 节点下的 department 节点，取第一个
-- 如果无 department 子节点，创建一个默认 department

-- Step 7: Graph 新增字段
ALTER TABLE graph_definitions ADD COLUMN team_id TEXT;
ALTER TABLE graph_definitions ADD COLUMN is_template BOOLEAN DEFAULT FALSE;
ALTER TABLE graph_definitions ADD COLUMN verification_gates TEXT DEFAULT '[]';
```

### 7.2 Wire 注入链路变更

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/biz/biz.go` | `NewTaxonomyUsecase` → `NewOrganizationUsecase` | ProviderSet 更新 |
| `internal/data/data.go` | `NewTaxonomyRepo` → `NewOrganizationRepo` | ProviderSet 更新 |
| `internal/service/taxonomy.go` | → `organization.go` | Service 重命名 |
| `internal/scenario/loader/deps.go` | `TaxonomyUsecase` → `OrganizationUsecase` | 依赖注入更新 |
| `internal/agent/builder_deps.go` | `TaxonomyUsecase` → `OrganizationUsecase` | 依赖注入更新 |
| `internal/service/chat_orchestrator.go` | `TaxonomyUsecase` → `OrganizationUsecase` | 依赖注入更新 |
| `internal/agent/prompt.go` | `Taxonomy *biz.TaxonomyUsecase` → `Organization *biz.OrganizationUsecase` | 依赖注入更新 |
| `cmd/admin/wire.go` | 无需手动修改 | Wire 自动生成 wire_gen.go |

### 7.3 API 兼容层

```go
// internal/service/industry_taxonomy_compat.go
// 旧 IndustryTaxonomyService 代理到新 OrganizationService
type IndustryTaxonomyCompatService struct {
    orgSvc *OrganizationService
}
// 所有方法转发到 orgSvc，字段名做映射
```

### 7.3 前端路由兼容

```typescript
// router 中添加重定向
{ path: '/industries/:rest*', redirect: to => ({ path: `/organization/${to.params.rest}` }) }
```

### 7.4 Pack 导入链路适配

| 文件 | 变更 | 说明 |
|------|------|------|
| `internal/data/seed_pack.go` | `SeedPackIndustry` → `SeedPackOrganization` | 函数重命名 |
| `internal/scenario/loader/loader.go` | `LoadIndustrySpec` → `LoadOrganizationSpec` | 加载器重命名 |
| `internal/scenario/loader/loader.go` | `BuildBizTeamFromSpec` 中 `CategoryIndustryID: spec.IndustryKey` → `DepartmentID: spec.DepartmentKey` | 字段映射更新 |
| `internal/scenario/loader/taxonomy_loader.go` | `TaxonomySpec`/`TaxonomyIndustrySpec` 等 5 个结构体重命名 | 结构体重命名 |
| `internal/data/ecosystem_preset.go` | 行 171, 229: `taxonomy_position_id` 硬编码 SQL | SQL 列名更新 |
| `cmd/admin/wire.go` | 行 1511: 直接调用 `SeedPackIndustry` | 函数调用重命名 |
| `internal/biz/ecosystem_preset.go` | 行 63: `SeedPackFunc` 类型别名 | 类型别名同步更新 |
| `internal/scenario/loader/spec.go` | `IndustrySpec` → `OrganizationSpec`，新增 `DepartmentSpec`/`DepartmentKey` 字段 | 结构体重命名 |

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
| `internal/service/` | 重命名+新增 | taxonomy.go → organization.go，新增兼容层 |
| `internal/scenario/` | 修改 | YAML 结构变更，新增部门主管 Prompt |
| `internal/data/seed_*.go` | 修改 | 种子数据适配新结构 |
| `api/kratos/` | 新增+修改 | 新增 organization proto，修改 agent/team/graph proto |
| `web/src/features/` | 重命名+修改 | industries → organization，新增组件 |
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
