# 行业Agent市场

## 一、需求文档

### 1.1 背景

当前平台已具备通用 Agent 构建能力（LLMAgent / GraphAgent / Team），但用户从零构建行业专用 Agent 门槛高。行业 Agent 市场旨在按行业（金融、医疗、教育、法律、电商等）预置 Agent 模板 + Graph 工作流 + Skill 组合包，用户可一键部署并按需定制。

行业参考：
- **Salesforce Einstein GPT**：按行业（金融服务、医疗健康、零售）预置 AI 模板，内置行业数据模型和合规规则
- **Microsoft Copilot Studio**：提供行业场景模板（客服、销售、HR），模板包含预置对话流、连接器和知识库

### 1.2 目标

1. 建立行业 Agent 模板体系，覆盖 5 大行业首批模板
2. 每个行业模板包含：Agent 配置 + Graph 工作流 + Skill 组合 + 工具集
3. 用户可浏览、一键部署、定制行业模板
4. 支持社区贡献行业模板（P2）

### 1.3 功能需求

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| F1 | 行业分类体系 | P0 | 金融/医疗/教育/法律/电商五大行业分类，每个行业含子场景 |
| F2 | 行业模板浏览 | P0 | 按行业浏览模板列表，支持搜索、筛选、预览 |
| F3 | 一键部署行业模板 | P0 | 从模板创建 Agent + Graph + Skill 完整实例 |
| F4 | 行业模板详情 | P0 | 展示模板包含的 Agent/Graph/Skill/Tool 组合 |
| F5 | 行业模板定制 | P1 | 部署后可修改 Agent 指令、替换 Skill、调整 Graph 节点 |
| F6 | 行业合规规则注入 | P1 | 按行业自动注入合规 Skill（如金融合规审查、医疗隐私保护） |
| F7 | 行业数据模型预置 | P1 | 预置行业实体类型和关系（如金融的交易/账户/客户） |
| F8 | 社区模板贡献 | P2 | 用户可发布自建模板到市场，经审核后上架 |
| F9 | 模板评分与评论 | P2 | 用户对已部署模板评分和评论 |
| F10 | 模板版本管理 | P2 | 模板更新后已部署实例可选择升级 |

### 1.4 非功能需求

| # | 需求 | 指标 |
|---|------|------|
| NFR1 | 模板部署延迟 | 一键部署 < 5s（不含 LLM 首次推理） |
| NFR2 | 模板加载 | 行业列表 API < 200ms |
| NFR3 | 合规规则 | 金融/医疗行业模板必须包含合规 Skill |
| NFR4 | 可扩展性 | 新增行业分类无需改代码，仅配置 |

### 1.5 验收标准

1. 5 大行业各有至少 1 个完整模板（Agent + Graph + Skill + Tool）
2. 一键部署后 Agent 可立即对话
3. 金融行业模板包含合规审查 Skill
4. 医疗行业模板包含隐私保护 Skill
5. 前端 IndustryMarketPage 可浏览和部署模板

---

## 二、设计文档

### 2.1 行业参考

**Salesforce Einstein GPT**：
- 行业云（Financial Services Cloud / Health Cloud）预置数据模型 + AI 模板
- 模板包含：预置 Prompt、数据映射规则、合规检查点
- 用户在行业云内一键激活 AI 能力

**Microsoft Copilot Studio**：
- 场景模板（Customer Service / Sales / HR）包含预置对话主题、连接器
- 模板 = Topic（对话流）+ Action（连接器调用）+ Knowledge（知识源）
- 部署后可在 Copilot Studio 中可视化编辑

**框架可复用组件**：

| 框架组件 | 路径 | 复用方式 |
|----------|------|----------|
| `skill.Repository` | `pkg/trpc-agent-go/skill/repository.go` | 行业 Skill 包加载 |
| `skill.ContextRepository` | `pkg/trpc-agent-go/skill/context_repository.go` | 行业 Skill 运行时过滤 |
| `graph.NewStateGraph` | `pkg/trpc-agent-go/graph/state_graph.go` | 行业 Graph 模板构建 |
| `graphagent.New` | `pkg/trpc-agent-go/agent/graphagent/graph_agent.go` | 行业 GraphAgent 创建 |
| `team.New` | `pkg/trpc-agent-go/team/team.go` | 行业 Team 编排 |
| `llmagent.New` | `pkg/trpc-agent-go/agent/llmagent/agent.go` | 行业 LLMAgent 创建 |
| `function.NewFunctionTool` | `pkg/trpc-agent-go/tool/function/` | 行业专用工具构建 |

### 2.2 当前项目现状

| 现有代码 | 路径 | 说明 |
|----------|------|------|
| `IndustryUsecase` | `internal/biz/industry_usecase.go` | 行业 CRUD 用例，含 `IndustryReader`/`IndustryWriter`/`IndustryRepository` 接口 |
| `IndustryMarketPage` | 前端页面 | 行业市场浏览页 |
| `IndustryDetailPage` | 前端页面 | 行业详情页 |
| `AgentCategoryUsecase` | `internal/biz/` | Agent 分类体系 |
| `SkillUsecase` | `internal/biz/skill/` | Skill 管理用例 |
| `GraphTemplate` | `internal/graph/trpc/templates.go` | 6 种内置 Graph 模板 |
| `BuildTRPCLLMAgent` | `internal/agent/trpc_build.go` | Agent 构建入口 |
| `FSRepositoryAdapter` | `internal/skill/trpc/repository.go` | Skill FS 适配器 |
| `DBRepositoryAdapter` | `internal/skill/trpc/db_repository.go` | Skill DB 适配器 |

**现有 `IndustryUsecase` 接口**：

```go
type IndustryReader interface {
    ListIndustries(ctx context.Context, q IndustryListQuery) (IndustryListResult, error)
    GetIndustryByKey(ctx context.Context, key string) (Industry, error)
}

type IndustryWriter interface {
    CreateIndustry(ctx context.Context, ind Industry) (Industry, error)
    UpdateIndustry(ctx context.Context, ind Industry) (Industry, error)
    UpsertIndustryByKey(ctx context.Context, ind Industry) (Industry, error)
}

type IndustryRepository interface {
    IndustryReader
    IndustryWriter
}
```

### 2.3 架构设计

#### 模块在四层架构中的位置

```
api/kratos/industry/v1/industry.proto    ← 新增 Proto（模板 CRUD + 部署）
        ↓
internal/service/industry.go             ← Proto↔biz 映射 + 模板部署编排
        ↓
internal/biz/industry_usecase.go         ← 扩展：模板管理 + 部署用例
internal/biz/industry_template.go        ← 新增：行业模板领域模型
        ↓
internal/data/industry_repo.go           ← 扩展：模板持久化
        ↓
internal/industry/                       ← 新增：行业模板运行时
  ├── template/                          ← 模板定义与解析
  │   ├── registry.go                    ← IndustryTemplateRegistry
  │   ├── finance.go                     ← 金融行业模板
  │   ├── healthcare.go                  ← 医疗行业模板
  │   ├── education.go                   ← 教育行业模板
  │   ├── legal.go                       ← 法律行业模板
  │   └── ecommerce.go                   ← 电商行业模板
  ├── deployer.go                        ← IndustryDeployer（一键部署）
  └── compliance/                        ← 行业合规规则
      ├── finance_compliance.go          ← 金融合规 Skill 注入
      └── healthcare_compliance.go       ← 医疗隐私 Skill 注入
```

#### 新增/修改的文件清单

| 操作 | 文件路径 | 说明 |
|------|----------|------|
| 新增 | `api/kratos/industry/v1/industry.proto` | 行业模板 Proto（6 RPC） |
| 新增 | `internal/biz/industry_template.go` | 行业模板领域模型 + 端口接口 |
| 修改 | `internal/biz/industry_usecase.go` | 扩展模板管理方法 |
| 新增 | `internal/data/industry_template_repo.go` | 模板 Ent 持久化 |
| 新增 | `internal/industry/template/registry.go` | 模板注册中心 |
| 新增 | `internal/industry/template/finance.go` | 金融行业模板定义 |
| 新增 | `internal/industry/template/healthcare.go` | 医疗行业模板定义 |
| 新增 | `internal/industry/template/education.go` | 教育行业模板定义 |
| 新增 | `internal/industry/template/legal.go` | 法律行业模板定义 |
| 新增 | `internal/industry/template/ecommerce.go` | 电商行业模板定义 |
| 新增 | `internal/industry/deployer.go` | 一键部署编排 |
| 新增 | `internal/industry/compliance/finance_compliance.go` | 金融合规 Skill |
| 新增 | `internal/industry/compliance/healthcare_compliance.go` | 医疗隐私 Skill |
| 修改 | `internal/service/industry.go` | 新增模板 RPC 适配 |
| 修改 | `cmd/admin/wire.go` | Wire 注入新增依赖 |

#### 接口设计

**行业模板领域模型**（`internal/biz/industry_template.go`）：

```go
type IndustryTemplate struct {
    ID          string
    IndustryKey string
    Name        string
    Description string
    Version     string
    AgentSpec   AgentTemplateSpec
    GraphSpec   *GraphTemplateSpec
    SkillSlugs  []string
    ToolNames   []string
    Compliance  []ComplianceRule
    Tags        []string
}

type AgentTemplateSpec struct {
    Instruction string
    ModelName   string
    DialogMode  string
}

type GraphTemplateSpec struct {
    TemplateID string
    Overrides  map[string]string
}

type ComplianceRule struct {
    Type        string
    Description string
    SkillSlug   string
}

type IndustryTemplateReader interface {
    ListTemplates(ctx context.Context, industryKey string) ([]IndustryTemplate, error)
    GetTemplate(ctx context.Context, id string) (IndustryTemplate, error)
    GetTemplateByIndustryAndName(ctx context.Context, industryKey, name string) (IndustryTemplate, error)
}

type IndustryTemplateWriter interface {
    CreateTemplate(ctx context.Context, t IndustryTemplate) (IndustryTemplate, error)
    UpdateTemplate(ctx context.Context, t IndustryTemplate) (IndustryTemplate, error)
}

type IndustryTemplateRepository interface {
    IndustryTemplateReader
    IndustryTemplateWriter
}
```

**一键部署接口**（`internal/industry/deployer.go`）：

```go
type DeployResult struct {
    AgentID    string
    GraphID    string
    SkillIDs   []string
    SessionID  string
}

type IndustryDeployer interface {
    Deploy(ctx context.Context, templateID string, overrides map[string]string) (*DeployResult, error)
}
```

**模板注册中心**（`internal/industry/template/registry.go`）：

```go
type TemplateProvider interface {
    IndustryKey() string
    Templates() []IndustryTemplate
}

type IndustryTemplateRegistry struct {
    providers map[string]TemplateProvider
}

func (r *IndustryTemplateRegistry) Register(p TemplateProvider)
func (r *IndustryTemplateRegistry) ListByIndustry(industryKey string) []IndustryTemplate
func (r *IndustryTemplateRegistry) Get(templateID string) *IndustryTemplate
```

**新增 Proto RPC**：

```protobuf
service IndustryService {
  rpc ListIndustryTemplates(ListIndustryTemplatesRequest) returns (ListIndustryTemplatesResponse);
  rpc GetIndustryTemplate(GetIndustryTemplateRequest) returns (IndustryTemplateProto);
  rpc DeployIndustryTemplate(DeployIndustryTemplateRequest) returns (DeployIndustryTemplateResponse);
  rpc CreateIndustryTemplate(CreateIndustryTemplateRequest) returns (IndustryTemplateProto);
  rpc UpdateIndustryTemplate(UpdateIndustryTemplateRequest) returns (IndustryTemplateProto);
  rpc ListComplianceRules(ListComplianceRulesRequest) returns (ListComplianceRulesResponse);
}
```

#### 数据流图

```
用户浏览行业市场
    │
    ▼
IndustryMarketPage → GET /v1/industries/templates?industry_key=finance
    │
    ▼
IndustryService.ListIndustryTemplates()
    │
    ▼
IndustryUsecase.ListTemplates(ctx, "finance")
    │
    ▼
IndustryTemplateRegistry.ListByIndustry("finance")
    │
    ▼
FinanceProvider.Templates() → 返回金融行业模板列表

用户一键部署
    │
    ▼
IndustryDetailPage → POST /v1/industries/templates/{id}/deploy
    │
    ▼
IndustryService.DeployIndustryTemplate()
    │
    ▼
IndustryDeployer.Deploy(ctx, templateID, overrides)
    ├── 1. 创建 Agent（AgentUsecase.Create）
    ├── 2. 部署 Graph（GraphUsecase.CreateFromTemplate）
    ├── 3. 启用 Skill（SkillUsecase.UpdateEnabled）
    ├── 4. 注入合规规则（ComplianceRule.SkillSlug → Skill 注入）
    └── 5. 返回 DeployResult{AgentID, GraphID, SkillIDs}
```

### 2.4 与框架的集成方式

| 集成点 | 框架组件 | 集成方式 |
|--------|----------|----------|
| 行业 Skill 加载 | `skill.Repository` / `skill.ContextRepository` | 行业模板的 `SkillSlugs` 通过 `DBRepositoryAdapter` 加载，经 `NewFilteredRepository` 过滤 |
| 行业 Graph 构建 | `graph.NewStateGraph` / `graphagent.New` | 模板 `GraphSpec.TemplateID` 映射到 `GraphTemplate`，经 `TemplateToBuildConfig` 转换后构建 |
| 行业 Agent 创建 | `llmagent.New` / `BuildTRPCLLMAgent` | 模板 `AgentSpec` 映射到 `TRPCBuilderDeps`，调用 `BuildTRPCLLMAgent` |
| 行业工具注册 | `function.NewFunctionTool[I, O]` | 行业专用工具在 `internal/tools/` 注册，经 `BuildToolsets` 装配 |
| 合规 Skill 注入 | `skill.ContextRepository` + `SkillRuntimePolicy` | 合规 Skill 强制加入 `AllowedSlugs`，不可被用户禁用 |

### 2.5 错误处理

| 场景 | 错误码 | 处理 |
|------|--------|------|
| 行业 Key 不存在 | `BadRequest("INDUSTRY", "unknown industry key")` | 返回 400 |
| 模板 ID 不存在 | `NotFound("INDUSTRY", "template not found")` | 返回 404 |
| 部署时 Agent 创建失败 | `InternalServer("INDUSTRY", "agent creation failed")` | 回滚已创建资源 |
| 合规 Skill 缺失 | `InternalServer("INDUSTRY", "compliance skill not found")` | 阻止部署 |
| 模板版本冲突 | `Conflict("INDUSTRY", "template version conflict")` | 返回 409 |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|------------|
| T1 | 定义 `IndustryTemplate` 领域模型 + 端口接口 | 无 | S |
| T2 | 定义行业模板 Proto（6 RPC） | T1 | S |
| T3 | 实现 `IndustryTemplateRegistry` + 5 个 `TemplateProvider` | T1 | M |
| T4 | 实现 `IndustryDeployer`（一键部署编排） | T1, T3 | L |
| T5 | 实现金融行业模板 + 合规 Skill | T3 | M |
| T6 | 实现医疗行业模板 + 隐私 Skill | T3 | M |
| T7 | 实现教育/法律/电商行业模板 | T3 | M |
| T8 | 实现 `IndustryTemplateRepository`（Ent 持久化） | T2 | M |
| T9 | 实现 Service 层模板 RPC 适配 | T2, T4, T8 | M |
| T10 | Wire 注入 + 集成测试 | T9 | S |
| T11 | 前端 IndustryMarketPage 模板浏览 | T9 | M |
| T12 | 前端一键部署交互 | T9, T4 | M |
| T13 | 端到端验证（5 行业模板部署 + 对话） | T10, T11, T12 | M |

### 3.2 开发顺序

```
Phase 1（核心框架）：T1 → T2 → T3 → T4 → T8 → T9 → T10
Phase 2（行业模板）：T5 → T6 → T7（可并行）
Phase 3（前端交互）：T11 → T12
Phase 4（端到端验证）：T13
```

### 3.3 验证方案

| 验证项 | 方法 | 通过标准 |
|--------|------|----------|
| 模板注册 | 单元测试 `IndustryTemplateRegistry` | 5 行业各返回 ≥1 模板 |
| 一键部署 | 集成测试 `IndustryDeployer.Deploy` | 返回有效 AgentID + GraphID + SkillIDs |
| 金融合规 | 部署金融模板后检查 Skill 列表 | 包含合规审查 Skill |
| 医疗隐私 | 部署医疗模板后检查 Skill 列表 | 包含隐私保护 Skill |
| 对话可用 | 部署后发送 Chat Turn | Agent 正常响应 |
| API 契约 | `make api && go build ./...` | 编译通过 |
| Wire 注入 | `make wire && go build ./cmd/admin` | 编译通过 |
