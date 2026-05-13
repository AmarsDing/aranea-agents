# Skill 技能模块 — 实现设计文档

> 对应需求：`20 skill.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Skill 管理：技能注册、Agent 绑定、运行时挂载、执行追踪。Skill 是可复用的提示词+工具组合模板。

---

## 二、Proto 层

### 2.1 现有 Proto

文件：`api/kratos/skill/v1/skill.proto`

```protobuf
service SkillService {
  rpc ListSkills(ListSkillsRequest) returns (ListSkillsResponse) {
    option (google.api.http) = { get: "/v1/skills" };
  }
  rpc CreateSkill(CreateSkillRequest) returns (Skill) {
    option (google.api.http) = { post: "/v1/skills" body: "*" };
  }
  rpc UpdateSkill(UpdateSkillRequest) returns (Skill) {
    option (google.api.http) = { patch: "/v1/skills/{id}" body: "*" };
  }
  rpc DeleteSkill(DeleteSkillRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/skills/{id}" };
  }
}
```

### 2.2 待新增

| RPC | 路径 | 用途 |
|-----|------|------|
| `GetSkillDetail` | `GET /v1/skills/{id}/detail` | 含工具列表 |
| `ExecuteSkill` | `POST /v1/skills/{id}/execute` | 手动执行 |
| `GetSkillCatalog` | `GET /v1/skills/catalog` | 内置技能目录 |

---

## 三、Biz 层

### 3.1 领域模型

```go
type Skill struct {
    ID          string
    Name        string
    DisplayName string
    Description string
    Category    string  // "built-in"/"custom"
    SkillType   string  // "prompt"/"tool"/"hybrid"
    Prompt      string
    ToolsJSON   string  // 绑定工具列表
    ConfigJSON  string
    Status      string
    CreatedAt   string
    UpdatedAt   string
}

type AgentSkill struct {
    ID        string
    AgentID   string
    SkillID   string
    SortOrder int32
}
```

### 3.2 Usecase

```go
type SkillUsecase struct {
    repo SkillRepository
}

func (uc *SkillUsecase) List(ctx, query) (SkillListResult, error)
func (uc *SkillUsecase) Create(ctx, s Skill) (Skill, error)
func (uc *SkillUsecase) Update(ctx, s Skill) (Skill, error)
func (uc *SkillUsecase) Delete(ctx, id) error
func (uc *SkillUsecase) EffectiveSkills(ctx, agentID string) ([]Skill, error)
```

---

## 四、Data 层

### 4.1 Ent Schema

- `internal/data/ent/schema/skill.go` — Skill 主表
- `internal/data/ent/schema/agent_skill.go` — Agent-Skill 关联表

---

## 五、运行时层

### 5.1 Skill 挂载

```go
// internal/agent/trpc_build.go
func AppendEffectiveSkillToolsets(ctx, ag, skillUC, catalog) ([]tool.Toolset, error)
```

### 5.2 内置 Skill

```go
// internal/tools/skillbuiltin/
var BuiltinSkills = []Skill{
    {Name: "web_search", DisplayName: "Web Search", ...},
    {Name: "code_interpreter", DisplayName: "Code Interpreter", ...},
    {Name: "mcp_tool_search", DisplayName: "MCP Tool Search", ...},
}
```

---

## 六、Service 层

```go
func (s *SkillService) ListSkills(ctx, req) (*ListSkillsResponse, error)
func (s *SkillService) CreateSkill(ctx, req) (*Skill, error)
func (s *SkillService) UpdateSkill(ctx, req) (*Skill, error)
func (s *SkillService) DeleteSkill(ctx, req) (*emptypb.Empty, error)
```

---

## 七、Wire 注入

已有，无需新增。

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/features/skills/
├── api.ts
├── types.ts
├── SkillEditorDialog.vue
├── SkillItem.vue
└── components/
    ├── SkillListPage.vue
    └── SkillCatalogBrowser.vue
```

### 8.2 组件设计

**SkillEditorDialog.vue**：

| 控件 | 绑定 | 说明 |
|------|------|------|
| `QInput` 名称 | `displayName` | 必填 |
| `QSelect` 类型 | `skillType` | prompt/tool/hybrid |
| `QEditor` 提示词 | `prompt` | Markdown |
| `QSelect` 工具 | `tools` | 多选可用工具 |
| `QInput` 配置 | `configJSON` | JSON 编辑器 |

**SkillCatalogBrowser.vue**：内置技能目录浏览

### 8.3 API

```typescript
export async function listSkills(query: SkillQuery): Promise<SkillListResult>
export async function createSkill(req: CreateSkillRequest): Promise<Skill>
export async function updateSkill(id: string, req: UpdateSkillRequest): Promise<Skill>
export async function deleteSkill(id: string): Promise<void>
export async function getSkillCatalog(): Promise<Skill[]>
```
