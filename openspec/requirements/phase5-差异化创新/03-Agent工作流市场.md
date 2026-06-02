# Agent工作流市场

## 一、需求文档

### 1.1 背景

平台已有 Graph 工作流编辑器（GraphEditorPage）和 6 种内置模板（pipeline/approval/parallel_review/review_loop/dispatch/nested_subgraph），但用户从零设计工作流门槛高，且无法分享和复用他人创建的工作流。Agent 工作流市场旨在构建类 n8n 的可视化工作流市场，用户可浏览、导入、发布工作流模板。

行业参考：
- **n8n Workflow Market**：社区共享工作流模板，模板包含节点配置和连接关系，一键导入后可编辑
- **Zapier Template Gallery**：按场景分类的自动化模板（营销/销售/HR），模板展示触发器和动作链
- **Make (Integromat) Templates**：可视化工作流模板市场，模板含场景描述、节点数量、使用统计

### 1.2 目标

1. 建立工作流模板市场，用户可浏览和搜索模板
2. 一键导入模板到自己的工作流编辑器
3. 用户可发布自建工作流为模板
4. 模板包含完整 Graph 定义 + 节点配置 + 使用说明
5. 模板分类和标签体系

### 1.3 功能需求

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| F1 | 模板浏览 | P0 | 按分类/标签/关键词浏览工作流模板 |
| F2 | 模板详情 | P0 | 展示模板拓扑图、节点列表、使用说明 |
| F3 | 一键导入模板 | P0 | 从模板创建 Graph 实例，进入编辑器 |
| F4 | 模板发布 | P0 | 将自建 Graph 发布为模板 |
| F5 | 模板搜索 | P1 | 按关键词、分类、标签搜索 |
| F6 | 模板评分 | P1 | 用户对模板评分 |
| F7 | 模板使用统计 | P1 | 展示模板导入次数、活跃实例数 |
| F8 | 模板版本管理 | P1 | 模板更新后已导入实例可选择升级 |
| F9 | 模板审核 | P2 | 发布前审核，确保安全合规 |
| F10 | 模板 Fork | P2 | 导入后修改并发布为新模板 |

### 1.4 非功能需求

| # | 需求 | 指标 |
|---|------|------|
| NFR1 | 模板列表加载 | < 300ms |
| NFR2 | 模板导入延迟 | < 3s |
| NFR3 | 模板发布延迟 | < 2s |
| NFR4 | 模板数量 | 首批 ≥ 20 个模板 |
| NFR5 | 搜索响应 | < 500ms |

### 1.5 验收标准

1. 市场首页展示 ≥ 20 个工作流模板
2. 一键导入后 GraphEditorPage 可编辑导入的工作流
3. 用户可发布自建工作流为模板
4. 模板详情页展示拓扑图
5. 搜索功能可按关键词找到匹配模板

---

## 二、设计文档

### 2.1 行业参考

**n8n Workflow Market**：
- 模板 = Workflow JSON + 元数据（名称/描述/分类/标签）
- 节点类型固定（HTTP Request / IF / Code / Webhook 等）
- 一键导入后自动创建 Workflow 实例
- 社区贡献 + 官方模板

**Zapier Template Gallery**：
- 按场景分类（Marketing / Sales / IT / HR）
- 模板展示触发器 + 动作链的简化视图
- "Try this template" 一键创建 Zap

**框架可复用组件**：

| 框架组件 | 路径 | 复用方式 |
|----------|------|----------|
| `graph.NewStateGraph` | `pkg/trpc-agent-go/graph/state_graph.go` | 从模板构建 StateGraph |
| `graph.Graph` | `pkg/trpc-agent-go/graph/graph.go` | 编译后的 Graph 实例 |
| `graphagent.New` | `pkg/trpc-agent-go/agent/graphagent/graph_agent.go` | GraphAgent 创建 |
| `GraphTemplate` | `internal/graph/trpc/templates.go` | 现有 6 种内置模板 |
| `TemplateToBuildConfig` | `internal/graph/trpc/templates.go` | 模板转构建配置 |
| `GraphBuildConfig` | `internal/graph/trpc/builder.go` | Graph 构建配置 |
| `BuildStateGraphWithRegistry` | `internal/graph/trpc/builder.go` | 从配置构建 StateGraph |
| `Registry` | `internal/graph/trpc/registry.go` | 节点函数注册中心 |
| `VisualGraphNode` | `internal/graph/trpc/visualize.go` | 可视化节点定义 |
| `ParseDOTToVisualGraph` | `internal/graph/trpc/visualize.go` | DOT 图解析 |

**现有 `GraphTemplate` 结构**（`internal/graph/trpc/templates.go`）：

```go
type GraphTemplate struct {
    ID          string          `json:"id"`
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Category    string          `json:"category"`
    Nodes       []TemplateNode  `json:"nodes"`
    Edges       []TemplateEdge  `json:"edges"`
    StateFields []StateFieldDef `json:"state_fields"`
    EntryPoint  string          `json:"entry_point"`
    FinishPoint string          `json:"finish_point"`
}

type TemplateNode struct {
    NodeID      string `json:"node_id"`
    Type        string `json:"type"`
    Label       string `json:"label"`
    Description string `json:"description"`
}

type TemplateEdge struct {
    FromNode string `json:"from_node"`
    ToNode   string `json:"to_node"`
    Type     string `json:"type"`
    Label    string `json:"label"`
}
```

### 2.2 当前项目现状

| 现有代码 | 路径 | 说明 |
|----------|------|------|
| `GraphTemplate` | `internal/graph/trpc/templates.go` | 6 种内置模板定义 |
| `ListBuiltinTemplates` | `internal/graph/trpc/templates.go` | 列出内置模板 |
| `GetBuiltinTemplate` | `internal/graph/trpc/templates.go` | 按 ID 获取模板 |
| `TemplateToBuildConfig` | `internal/graph/trpc/templates.go` | 模板转 GraphBuildConfig |
| `GraphEditorPage` | 前端页面 | Graph 可视化编辑器 |
| `GraphUsecase` | `internal/biz/graph_usecase.go` | Graph CRUD 用例 |
| `GraphBuilderFactory` | `internal/graph/adapter/runtime_adapter.go` | Graph 构建工厂 |
| `Registry` | `internal/graph/trpc/registry.go` | 节点/条件函数注册中心 |
| `VisualGraphNode` | `internal/graph/trpc/visualize.go` | 可视化节点定义 |

### 2.3 架构设计

#### 模块在四层架构中的位置

```
api/kratos/graph/v1/graph.proto            ← 扩展：模板市场 RPC
        ↓
internal/service/graph.go                  ← 扩展：模板市场 RPC 适配
        ↓
internal/biz/workflow_template.go          ← 新增：工作流模板领域模型
internal/biz/workflow_template_usecase.go  ← 新增：模板市场用例
        ↓
internal/data/workflow_template_repo.go    ← 新增：模板 Ent 持久化
        ↓
internal/graph/marketplace/                ← 新增：工作流市场运行时
  ├── catalog.go                           ← 模板目录管理
  ├── publisher.go                         ← 模板发布
  ├── importer.go                          ← 模板导入
  └── search.go                            ← 模板搜索
```

#### 新增/修改的文件清单

| 操作 | 文件路径 | 说明 |
|------|----------|------|
| 修改 | `api/kratos/graph/v1/graph.proto` | 新增模板市场 RPC（5 个） |
| 新增 | `internal/biz/workflow_template.go` | 工作流模板领域模型 + 端口 |
| 新增 | `internal/biz/workflow_template_usecase.go` | 模板市场用例 |
| 新增 | `internal/data/workflow_template_repo.go` | 模板 Ent 持久化 |
| 新增 | `internal/graph/marketplace/catalog.go` | 模板目录管理 |
| 新增 | `internal/graph/marketplace/publisher.go` | 模板发布 |
| 新增 | `internal/graph/marketplace/importer.go` | 模板导入 |
| 新增 | `internal/graph/marketplace/search.go` | 模板搜索 |
| 修改 | `internal/service/graph.go` | 新增模板市场 RPC 适配 |
| 修改 | `cmd/admin/wire.go` | Wire 注入 |

#### 接口设计

**工作流模板领域模型**（`internal/biz/workflow_template.go`）：

```go
type WorkflowTemplate struct {
    ID           string
    Name         string
    Description  string
    Category     string
    Tags         []string
    AuthorID     string
    GraphDef     GraphDefinition
    ThumbnailURL string
    ImportCount  int
    Rating       float32
    RatingCount  int
    Version      string
    Status       string
    CreatedAt    string
    UpdatedAt    string
}

type WorkflowTemplateQuery struct {
    Keyword  string
    Category string
    Tags     []string
    SortBy   string
    Limit    int
    Offset   int
}

type WorkflowTemplateListResult struct {
    Items []WorkflowTemplate
    Total int
}

type WorkflowTemplateReader interface {
    ListTemplates(ctx context.Context, q WorkflowTemplateQuery) (WorkflowTemplateListResult, error)
    GetTemplate(ctx context.Context, id string) (WorkflowTemplate, error)
    SearchTemplates(ctx context.Context, keyword string, limit int) ([]WorkflowTemplate, error)
}

type WorkflowTemplateWriter interface {
    CreateTemplate(ctx context.Context, t WorkflowTemplate) (WorkflowTemplate, error)
    UpdateTemplate(ctx context.Context, t WorkflowTemplate) (WorkflowTemplate, error)
    DeleteTemplate(ctx context.Context, id string) error
    IncrementImportCount(ctx context.Context, id string) error
}

type WorkflowTemplateRepository interface {
    WorkflowTemplateReader
    WorkflowTemplateWriter
}
```

**模板市场用例**（`internal/biz/workflow_template_usecase.go`）：

```go
type WorkflowTemplateUsecase struct {
    repo    WorkflowTemplateRepository
    graphUC *GraphUsecase
}

func NewWorkflowTemplateUsecase(repo WorkflowTemplateRepository, graphUC *GraphUsecase) *WorkflowTemplateUsecase

func (u *WorkflowTemplateUsecase) List(ctx context.Context, q WorkflowTemplateQuery) (WorkflowTemplateListResult, error)
func (u *WorkflowTemplateUsecase) Get(ctx context.Context, id string) (WorkflowTemplate, error)
func (u *WorkflowTemplateUsecase) Publish(ctx context.Context, graphID string, meta WorkflowTemplate) (WorkflowTemplate, error)
func (u *WorkflowTemplateUsecase) Import(ctx context.Context, templateID string, name string) (Graph, error)
func (u *WorkflowTemplateUsecase) Rate(ctx context.Context, templateID string, score float32) error
```

**模板导入器**（`internal/graph/marketplace/importer.go`）：

```go
type TemplateImporter struct {
    graphUC  *biz.GraphUsecase
    registry *graphtrpc.Registry
}

func (imp *TemplateImporter) Import(ctx context.Context, tmpl biz.WorkflowTemplate, name string) (biz.Graph, error) {
    cfg := graphtrpc.TemplateToBuildConfig(graphtrpc.GraphTemplate{
        ID:          tmpl.ID,
        Name:        tmpl.Name,
        Nodes:       bizNodesToTemplateNodes(tmpl.GraphDef.Nodes),
        Edges:       bizEdgesToTemplateEdges(tmpl.GraphDef.Edges, tmpl.GraphDef.ConditionalEdges),
        StateFields: bizStateFieldsToTemplateFields(tmpl.GraphDef.StateFields),
        EntryPoint:  tmpl.GraphDef.EntryPoint,
        FinishPoint: tmpl.GraphDef.FinishPoint,
    })
    return imp.graphUC.Create(ctx, biz.GraphCreateInput{
        Name:        name,
        Definition:  tmpl.GraphDef,
    })
}
```

**新增 Proto RPC**：

```protobuf
service GraphService {
  rpc ListWorkflowTemplates(ListWorkflowTemplatesRequest) returns (ListWorkflowTemplatesResponse);
  rpc GetWorkflowTemplate(GetWorkflowTemplateRequest) returns (WorkflowTemplateProto);
  rpc PublishWorkflowTemplate(PublishWorkflowTemplateRequest) returns (WorkflowTemplateProto);
  rpc ImportWorkflowTemplate(ImportWorkflowTemplateRequest) returns (GraphProto);
  rpc RateWorkflowTemplate(RateWorkflowTemplateRequest) returns (RateWorkflowTemplateResponse);
}
```

#### 数据流图

```
用户浏览工作流市场
    │
    ▼
WorkflowMarketPage → GET /v1/graphs/templates?category=pipeline
    │
    ▼
GraphService.ListWorkflowTemplates()
    │
    ▼
WorkflowTemplateUsecase.List(ctx, query)
    │
    ▼
WorkflowTemplateRepository.ListTemplates(ctx, query)
    │
    ▼
返回模板列表（含缩略图/评分/导入次数）

用户一键导入
    │
    ▼
WorkflowMarketPage → POST /v1/graphs/templates/{id}/import
    │
    ▼
GraphService.ImportWorkflowTemplate()
    │
    ▼
WorkflowTemplateUsecase.Import(ctx, templateID, name)
    ├── TemplateImporter.Import()
    │   ├── TemplateToBuildConfig() 转换模板
    │   └── GraphUsecase.Create() 创建 Graph 实例
    └── IncrementImportCount() 更新导入计数
    │
    ▼
跳转 GraphEditorPage 编辑导入的工作流

用户发布模板
    │
    ▼
GraphEditorPage → POST /v1/graphs/templates (from graph_id)
    │
    ▼
GraphService.PublishWorkflowTemplate()
    │
    ▼
WorkflowTemplateUsecase.Publish(ctx, graphID, meta)
    ├── GraphUsecase.Get() 获取 Graph 定义
    ├── 验证 Graph 完整性
    └── WorkflowTemplateRepository.CreateTemplate() 保存模板
```

### 2.4 与框架的集成方式

| 集成点 | 框架组件 | 集成方式 |
|--------|----------|----------|
| 模板转 Graph | `TemplateToBuildConfig` | 模板定义转换为 `GraphBuildConfig` |
| Graph 构建 | `BuildStateGraphWithRegistry` | 从 `GraphBuildConfig` 构建 `StateGraph` |
| GraphAgent 创建 | `graphagent.New` | 编译后创建 GraphAgent |
| 节点注册 | `Registry` | 模板中的节点类型需在 Registry 中注册 |
| 可视化 | `ParseDOTToVisualGraph` | 模板详情页展示拓扑图 |
| 模板验证 | `ValidateGraph` | 发布前验证 Graph 完整性 |

### 2.5 错误处理

| 场景 | 错误码 | 处理 |
|------|--------|------|
| 模板不存在 | `NotFound("WORKFLOW_TEMPLATE", "template not found")` | 返回 404 |
| Graph 不存在 | `NotFound("GRAPH", "graph not found")` | 发布时源 Graph 不存在 |
| 模板验证失败 | `BadRequest("WORKFLOW_TEMPLATE", "invalid graph definition")` | 返回 400 |
| 导入失败 | `InternalServer("WORKFLOW_TEMPLATE", "import failed")` | 返回 500 |
| 评分越界 | `BadRequest("WORKFLOW_TEMPLATE", "rating must be 1-5")` | 返回 400 |
| 重复发布 | `Conflict("WORKFLOW_TEMPLATE", "template already published")` | 返回 409 |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|------------|
| T1 | 定义 `WorkflowTemplate` 领域模型 + 端口接口 | 无 | S |
| T2 | 定义模板市场 Proto（5 RPC） | T1 | S |
| T3 | 实现 `WorkflowTemplateRepository`（Ent 持久化） | T1 | M |
| T4 | 实现 `WorkflowTemplateUsecase` | T1, T3 | M |
| T5 | 实现 `TemplateImporter` | T1 | M |
| T6 | 实现 `TemplatePublisher` | T1 | M |
| T7 | 实现 `TemplateSearch`（关键词 + 标签搜索） | T1, T3 | M |
| T8 | 扩展内置模板至 20 个 | 无 | M |
| T9 | 实现 Service 层模板市场 RPC 适配 | T2, T4, T5, T6, T7 | M |
| T10 | Wire 注入 + 集成测试 | T9 | S |
| T11 | 前端 WorkflowMarketPage 模板浏览 | T9 | L |
| T12 | 前端模板详情页（含拓扑图） | T9 | M |
| T13 | 前端一键导入交互 | T9, T5 | M |
| T14 | 前端模板发布交互 | T9, T6 | M |
| T15 | 端到端验证 | T10, T11, T12, T13, T14 | M |

### 3.2 开发顺序

```
Phase 1（核心模型）：T1 → T2 → T3 → T4
Phase 2（运行时）：T5 → T6 → T7（可并行）
Phase 3（模板扩充）：T8（可与 Phase 2 并行）
Phase 4（服务层）：T9 → T10
Phase 5（前端）：T11 → T12 → T13 → T14
Phase 6（验证）：T15
```

### 3.3 验证方案

| 验证项 | 方法 | 通过标准 |
|--------|------|----------|
| 模板列表 | GET /v1/graphs/templates | 返回 ≥20 个模板 |
| 模板搜索 | GET /v1/graphs/templates?keyword=审批 | 返回匹配模板 |
| 一键导入 | POST /v1/graphs/templates/{id}/import | 创建 Graph 实例，可编辑 |
| 模板发布 | POST /v1/graphs/templates | 从 Graph 创建模板 |
| 拓扑图渲染 | 模板详情页 | 正确展示节点和边 |
| 模板评分 | POST /v1/graphs/templates/{id}/rate | 评分更新 |
| API 契约 | `make api && go build ./...` | 编译通过 |
| Wire 注入 | `make wire && go build ./cmd/admin` | 编译通过 |
