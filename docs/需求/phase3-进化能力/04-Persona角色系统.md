# Persona 角色系统

## 一、需求文档

### 1.1 背景

OpenClaw 在 `pkg/trpc-agent-go/openclaw/internal/persona/` 提供了完整的 Persona 系统：预置角色模板（Default/Girlfriend/Concise/Coach/Creative）、按作用域（DM/Thread）持久化角色选择、角色切换时注入 prompt 片段。当前项目 Agent 有 `system_prompt` 但无角色切换机制，所有会话使用固定的系统提示词。

框架层 Persona Store 的核心设计：
- `Preset` 结构体：ID/Name/Description/Prompt
- `Store` 结构体：基于 JSON 文件的持久化，按 scopeKey 存储 presetID
- 作用域模型：DM（用户级）和 Thread（会话级）
- 角色切换：`Store.Set()` 修改作用域角色，`Store.Get()` 读取当前角色

当前项目的 `EvolutionUsecase.ApplySuggestion` 已支持 `persona` 类型建议（修改 IDENTITY.md 的 `## Persona` 段），但这是静态修改，不支持运行时动态切换。

### 1.2 目标

1. 实现 Persona 角色系统，让 Agent 可在不同会话/用户维度动态切换角色
2. 支持预置角色模板 + 自定义角色，角色切换实时生效
3. 与现有 `system_prompt` 体系兼容，角色 prompt 作为 system prompt 的补充层
4. 支持前端角色选择 UI

### 1.3 功能需求

| 编号 | 功能 | 优先级 | 说明 |
|------|------|--------|------|
| F1 | Persona 预置模板管理 | P0 | 内置 Default/Concise/Coach/Creative 等角色模板 |
| F2 | 自定义 Persona CRUD | P0 | 用户可创建/编辑/删除自定义角色 |
| F3 | 角色切换 API | P0 | 按 Agent + User/Session 维度切换角色 |
| F4 | 角色 Prompt 注入 | P0 | 切换角色后，角色 prompt 注入 system prompt |
| F5 | 角色作用域 | P1 | 支持 Agent 级（全局）和 Session 级（会话）作用域 |
| F6 | 角色继承 | P2 | 自定义角色可继承预置角色并覆盖部分字段 |
| F7 | 前端角色选择器 | P0 | Agent 设置页和会话页提供角色切换 UI |
| F8 | 角色与进化系统集成 | P2 | 进化建议可推荐角色变更 |

### 1.4 非功能需求

| 编号 | 需求 | 说明 |
|------|------|------|
| NF1 | 实时性 | 角色切换后下一次 LLM 调用立即生效 |
| NF2 | 隔离性 | 不同用户的角色选择互不影响 |
| NF3 | 安全性 | 角色 prompt 不可包含指令注入攻击内容 |
| NF4 | 性能 | 角色查询响应时间 < 10ms |

### 1.5 验收标准

1. 用户可在前端为 Agent 选择不同角色，角色 prompt 正确注入 system prompt
2. 不同用户对同一 Agent 可选择不同角色，互不影响
3. 自定义角色可创建/编辑/删除，删除后回退到 Default
4. 角色切换后 Agent 行为风格明显变化（如 Concise 角色回复更简洁）

---

## 二、设计文档

### 2.1 框架参考

#### OpenClaw Persona Store

```go
// pkg/trpc-agent-go/openclaw/internal/persona/store.go
type Preset struct {
    ID          string
    Name        string
    Description string
    Prompt      string
}

var presetList = []Preset{
    {ID: "default", Name: "Default", Description: "Use the normal assistant behavior."},
    {ID: "girlfriend", Name: "Girlfriend", Description: "Warm, playful, and affectionate companion tone.", Prompt: "..."},
    {ID: "concise", Name: "Concise", Description: "Direct, brief, and action-first replies.", Prompt: "..."},
    {ID: "coach", Name: "Coach", Description: "Structured, pragmatic, and goal-oriented.", Prompt: "..."},
    {ID: "creative", Name: "Creative", Description: "More imaginative, vivid, and idea-rich.", Prompt: "..."},
}

type Store struct {
    path string
    mu    sync.Mutex
    state storeState
}

type storeState struct {
    Version int               `json:"version"`
    Scopes  map[string]string `json:"scopes,omitempty"`
}

func NewStore(path string) (*Store, error)
func (s *Store) Get(scopeKey string) (Preset, error)
func (s *Store) Set(ctx context.Context, scopeKey string, presetID string) (Preset, error)
func (s *Store) ForgetUser(ctx context.Context, channel string, userID string) error

func List() []Preset
func Lookup(id string) (Preset, bool)
func DefaultPreset() Preset
func DMScopeKey(channel string, userID string) string
func ThreadScopeKey(channel string, thread string) string
func ScopeKeyFromSession(channel string, userID string, sessionID string) string
```

关键设计点：
- 作用域模型：`channel:dm:userID`（用户级）和 `channel:thread:threadID`（会话级）
- 持久化：JSON 文件 `persona/presets.json`
- 角色 prompt 末尾追加 `personaTaskCompletionPrompt` 确保任务完成优先

### 2.2 当前项目现状

| 文件 | 现状 | 不足 |
|------|------|------|
| `internal/agent/trpc_build.go` | `BuildTRPCLLMAgent` 构建 system prompt | 无角色切换逻辑 |
| `internal/agent/prompt.go` | `BuildSystemPrompt` 构建 system prompt | 无角色层注入 |
| `internal/agent/composite_prompt.go` | 复合 prompt 组装 | 可扩展角色层 |
| `internal/biz/evolution.go` | `ApplySuggestion` 支持 persona 类型 | 仅静态修改 IDENTITY.md |
| `internal/biz/agent_settings.go` | `AgentRuntimeSettings` | 无 Persona 相关配置 |

### 2.3 架构设计

#### 模块在四层架构中的位置

```
api/**/*.proto                    ← 新增 Persona 相关 proto
        ↓
internal/service                  ← PersonaService：proto↔biz 映射
        ↓
internal/biz                      ← PersonaUsecase + 端口接口
        ↓
internal/data                     ← PersonaRepo 实现（Ent ORM）
```

Agent 运行时模块位置：
```
internal/agent                    ← Persona prompt 注入（BeforeModel callback）
```

#### 新增/修改的文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `api/aranea/v1/persona.proto` | 新增 | Persona HTTP/gRPC 接口 |
| `internal/service/persona.go` | 新增 | Service 层 |
| `internal/biz/persona.go` | 新增 | PersonaUsecase |
| `internal/biz/persona_types.go` | 新增 | Persona 领域模型 |
| `internal/biz/persona_repo.go` | 新增 | Repo 端口接口 |
| `internal/data/persona.go` | 新增 | Repo 实现 |
| `internal/data/ent/schema/persona.go` | 新增 | Ent Schema |
| `internal/data/ent/schema/persona_scope.go` | 新增 | 作用域映射 Schema |
| `internal/agent/persona_inject.go` | 新增 | BeforeModel callback 注入角色 prompt |
| `internal/agent/trpc_build.go` | 修改 | 集成 persona_inject callback |
| `web/src/features/persona/api.ts` | 新增 | Persona API |
| `web/src/stores/persona/index.ts` | 新增 | Persona Store |
| `web/src/components/persona/*.vue` | 新增 | 角色选择器组件 |

#### 接口设计

```go
// internal/biz/persona_types.go

type Persona struct {
    ID          string
    Name        string
    Description string
    Prompt      string
    IsBuiltin   bool
    ParentID    string
    CreatedBy   string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type PersonaScope struct {
    ID         string
    PersonaID  string
    AgentID    string
    ScopeKind  string
    ScopeKey   string
    SetBy      string
    SetAt      time.Time
}

const (
    ScopeKindAgent   = "agent"
    ScopeKindSession = "session"
    ScopeKindUser    = "user"
)
```

```go
// internal/biz/persona_repo.go

type PersonaReader interface {
    List(ctx context.Context, includeBuiltin bool) ([]Persona, error)
    GetByID(ctx context.Context, id string) (Persona, error)
    GetByName(ctx context.Context, name string) (*Persona, error)
}

type PersonaWriter interface {
    Create(ctx context.Context, p Persona) (Persona, error)
    Update(ctx context.Context, p Persona) (Persona, error)
    Delete(ctx context.Context, id string) error
}

type PersonaScopeReader interface {
    GetEffective(ctx context.Context, agentID string, scopeKind string, scopeKey string) (*PersonaScope, error)
}

type PersonaScopeWriter interface {
    Set(ctx context.Context, scope PersonaScope) (PersonaScope, error)
    Clear(ctx context.Context, agentID string, scopeKind string, scopeKey string) error
}
```

```go
// internal/biz/persona.go

type PersonaUsecase struct {
    personaRepo  PersonaReader
    scopeRepo    PersonaScopeReader
    builtinCache *BuiltinPersonaCache
}

func NewPersonaUsecase(
    personaRepo PersonaReader,
    scopeRepo PersonaScopeReader,
    builtinCache *BuiltinPersonaCache,
) *PersonaUsecase

func (uc *PersonaUsecase) ListPersonas(ctx context.Context) ([]Persona, error)
func (uc *PersonaUsecase) GetPersona(ctx context.Context, id string) (Persona, error)
func (uc *PersonaUsecase) CreatePersona(ctx context.Context, p Persona) (Persona, error)
func (uc *PersonaUsecase) UpdatePersona(ctx context.Context, p Persona) (Persona, error)
func (uc *PersonaUsecase) DeletePersona(ctx context.Context, id string) error
func (uc *PersonaUsecase) SetPersona(ctx context.Context, agentID string, scopeKind string, scopeKey string, personaID string) error
func (uc *PersonaUsecase) ClearPersona(ctx context.Context, agentID string, scopeKind string, scopeKey string) error
func (uc *PersonaUsecase) GetEffectivePersona(ctx context.Context, agentID string, scopeKind string, scopeKey string) (*Persona, error)
func (uc *PersonaUsecase) ResolvePrompt(ctx context.Context, agentID string, scopeKind string, scopeKey string) (string, error)
```

```go
// internal/agent/persona_inject.go

type PersonaInjector struct {
    uc *PersonaUsecase
}

func NewPersonaInjector(uc *PersonaUsecase) *PersonaInjector

func (pi *PersonaInjector) BeforeModel(ctx context.Context, args *model.BeforeModelArgs) (*model.BeforeModelResult, error)
```

```go
// internal/biz/persona_builtin.go

type BuiltinPersonaCache struct {
    personas []Persona
}

func NewBuiltinPersonaCache() *BuiltinPersonaCache

func (c *BuiltinPersonaCache) List() []Persona
func (c *BuiltinPersonaCache) Get(id string) (Persona, bool)
```

#### 数据流图

```
前端角色选择 → PersonaService.SetPersona()
    │
    ▼
PersonaUsecase.SetPersona()
    │  写入 persona_scopes 表
    │  scopeKind=session 时写入 Session state
    ▼
Agent LLM 调用 → BeforeModel callback
    │
    ▼
PersonaInjector.BeforeModel()
    │  PersonaUsecase.ResolvePrompt() → 查询有效角色
    │  将角色 prompt 注入 system message
    ▼
LLM 收到含角色 prompt 的 system message → 按角色风格回复
```

### 2.4 与框架的集成方式

1. **BeforeModel callback**：通过 `llmagent.WithBeforeModel()` 注册 `PersonaInjector`，在每次 LLM 调用前注入角色 prompt
2. **Session state**：Session 级角色选择写入 `Session.SetState()`，确保同一会话内角色一致
3. **Prompt 组装**：角色 prompt 作为 `composite_prompt` 的一个层，在 system prompt 之后注入
4. **预置角色**：复用 OpenClaw `persona.presetList` 的角色定义，存储在 `BuiltinPersonaCache` 中
5. **作用域模型**：参考 OpenClaw 的 `DMScopeKey`/`ThreadScopeKey` 设计，适配项目的 Agent/User/Session 三级作用域

### 2.5 错误处理

| 场景 | 处理方式 |
|------|----------|
| 角色不存在 | 回退到 Default 角色，记录 FlowLog Warn |
| 角色 prompt 为空 | 跳过注入，使用原始 system prompt |
| 自定义角色删除 | 已使用该角色的作用域自动回退到 Default |
| 角色注入 LLM 调用失败 | 跳过注入，不影响主流程 |
| 角色 prompt 包含注入攻击 | 创建时校验，拒绝包含危险指令的 prompt |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|-----------|
| T1 | 定义 `persona_types.go` 领域模型 | 无 | S |
| T2 | 定义 `persona_repo.go` 端口接口 | T1 | S |
| T3 | 创建 Ent Schema（persona/persona_scope） | T1 | M |
| T4 | 实现 `internal/data/persona.go` Repo | T2, T3 | M |
| T5 | 实现 `BuiltinPersonaCache` | 无 | S |
| T6 | 实现 `PersonaUsecase` 核心方法 | T4, T5 | L |
| T7 | 实现 `PersonaInjector` BeforeModel callback | T6 | M |
| T8 | 集成到 `BuildTRPCLLMAgent` | T7 | M |
| T9 | 新增 `persona.proto` + Service 层 | T6 | M |
| T10 | Wire DI 装配 | T8, T9 | S |
| T11 | 前端 `features/persona/api.ts` | T9 | S |
| T12 | 前端 `stores/persona/index.ts` | T11 | M |
| T13 | 前端角色选择器组件 | T12 | M |
| T14 | 单元测试 | T6, T7 | M |
| T15 | 集成测试 | T10 | L |

### 3.2 开发顺序

```
Phase 1 — 数据基础（T1 → T2 → T3 → T4）
Phase 2 — 核心能力（T5 → T6 → T7 → T8）
Phase 3 — 后端接入（T9 → T10）
Phase 4 — 前端（T11 → T12 → T13）
Phase 5 — 验证（T14 → T15）
```

### 3.3 验证方案

| 阶段 | 验证方式 |
|------|----------|
| Phase 1 | `go generate ./internal/data/ent/... && go build ./...` |
| Phase 2 | `go test ./internal/agent/... -run TestPersonaInjector -count=1` |
| Phase 3 | `make api && make wire && make build` |
| Phase 4 | `cd web && pnpm lint && pnpm build` |
| Phase 5 | 端到端：切换角色 → 发送消息 → 验证回复风格变化 |
| 提交前 | 后端 `make api && make wire && make build && make test && make lint`；前端 `cd web && pnpm lint && pnpm test && pnpm build` |
