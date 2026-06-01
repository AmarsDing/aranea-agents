# RuntimeProfile 运行时配置

## 一、需求文档

### 1.1 背景

OpenClaw 在 `pkg/trpc-agent-go/openclaw/runtimeprofile/` 提供了完整的运行时配置系统：Profile 定义（模型/工具/技能/知识/工作区/凭证/隔离策略）、Resolver 解析、Store 持久化、CachedResolver 缓存刷新。Profile 可按请求维度选择，实现不同场景使用不同的模型/工具/记忆策略组合。

当前项目 `internal/agent/` 有 `AgentRuntimeSettings`（含 Memory/Tools/Skills/Evolution/Context 等配置），前端有配置入口，但存在以下不足：
1. 配置是 Agent 级别的静态配置，不支持按请求/会话维度动态切换
2. 缺少 Profile 概念，无法预定义多套运行时配置组合
3. 配置变更需要重启 Agent 才能生效
4. 缺少工具/技能/知识库的细粒度访问控制

### 1.2 目标

1. 实现 RuntimeProfile 系统，支持预定义多套运行时配置组合
2. 支持按请求/会话维度动态选择 Profile，无需重启 Agent
3. 提供工具/技能/知识库的细粒度访问控制（include/exclude）
4. 与现有 `AgentRuntimeSettings` 兼容，Profile 作为运行时覆盖层
5. 支持前端 Profile 管理和切换 UI

### 1.3 功能需求

| 编号 | 功能 | 优先级 | 说明 |
|------|------|--------|------|
| F1 | Profile CRUD | P0 | 创建/读取/更新/删除运行时配置 Profile |
| F2 | Profile 字段定义 | P0 | 支持模型、工具策略、技能策略、知识库策略、Prompt 覆盖、工作区策略 |
| F3 | Profile 选择 API | P0 | 按 Agent + 请求参数选择 Profile |
| F4 | Profile 热切换 | P0 | Profile 变更后下一次 LLM 调用立即生效 |
| F5 | 工具访问控制 | P0 | Profile 可定义工具 include/exclude 列表 |
| F6 | 技能访问控制 | P1 | Profile 可定义技能 include/exclude 列表 |
| F7 | 知识库访问控制 | P1 | Profile 可定义知识库索引列表 |
| F8 | Prompt 覆盖 | P1 | Profile 可覆盖 system prompt 或追加 instruction |
| F9 | 默认 Profile | P0 | 每个 Agent 可设置默认 Profile |
| F10 | 前端 Profile 管理 | P0 | Profile 列表、编辑、切换 UI |

### 1.4 非功能需求

| 编号 | 需求 | 说明 |
|------|------|------|
| NF1 | 实时性 | Profile 切换后下一次 LLM 调用立即生效 |
| NF2 | 安全性 | Profile 中的凭证引用必须通过 `CredentialPolicy` 校验 |
| NF3 | 性能 | Profile 解析耗时 < 5ms（缓存命中时 < 1ms） |
| NF4 | 兼容性 | 不破坏现有 `AgentRuntimeSettings` 的行为 |

### 1.5 验收标准

1. 用户可为 Agent 创建多个 Profile，每个 Profile 配置不同的模型/工具/技能策略
2. 会话中切换 Profile 后，Agent 的工具列表、技能可见性、模型选择立即变化
3. Profile 的工具 include/exclude 正确过滤 `tool.Declaration` 列表
4. 默认 Profile 在无显式选择时自动生效
5. Profile 变更后 `CachedResolver.Reload()` 刷新缓存

---

## 二、设计文档

### 2.1 框架参考

#### OpenClaw RuntimeProfile

```go
// pkg/trpc-agent-go/openclaw/runtimeprofile/profile.go
type Config struct {
    Default           string             `yaml:"default,omitempty"`
    Required          bool               `yaml:"required,omitempty"`
    FallbackToDefault bool               `yaml:"fallback_to_default,omitempty"`
    Profiles          map[string]Profile `yaml:"profiles,omitempty"`
}

type Profile struct {
    ID          string           `yaml:"id,omitempty"`
    Version     string           `yaml:"version,omitempty"`
    AppName     string           `yaml:"app_name,omitempty"`
    AgentName   string           `yaml:"agent_name,omitempty"`
    ModelName   string           `yaml:"model_name,omitempty"`
    Prompt      Prompt           `yaml:"prompt,omitempty"`
    Tools       ToolPolicy       `yaml:"tools,omitempty"`
    Knowledge   KnowledgePolicy  `yaml:"knowledge,omitempty"`
    Workspace   WorkspacePolicy  `yaml:"workspace,omitempty"`
    Credentials CredentialPolicy `yaml:"credentials,omitempty"`
    Skills      SkillPolicy      `yaml:"skills,omitempty"`
    Isolation   IsolationPolicy  `yaml:"isolation,omitempty"`
    State       map[string]any   `yaml:"runtime_state,omitempty"`
    ExtraModel  map[string]any   `yaml:"model_request_extra,omitempty"`
}

type Prompt struct {
    Instruction  string `yaml:"instruction,omitempty"`
    SystemPrompt string `yaml:"system_prompt,omitempty"`
}

type ToolPolicy struct {
    Include          []string          `yaml:"include,omitempty"`
    Exclude          []string          `yaml:"exclude,omitempty"`
    ExecutionInclude []string          `yaml:"execution_include,omitempty"`
    ExecutionExclude []string          `yaml:"execution_exclude,omitempty"`
    ToolSets         []string          `yaml:"toolsets,omitempty"`
    CredentialRefs   map[string]string `yaml:"credential_refs,omitempty"`
}

type SkillPolicy struct {
    Include []string `yaml:"include,omitempty"`
    Exclude []string `yaml:"exclude,omitempty"`
    Roots   []string `yaml:"roots,omitempty"`
}

type KnowledgePolicy struct {
    Indexes []string       `yaml:"indexes,omitempty"`
    Filter  map[string]any `yaml:"filter,omitempty"`
}

type WorkspacePolicy struct {
    Workdir      string   `yaml:"workdir,omitempty"`
    AllowedRoots []string `yaml:"allowed_roots,omitempty"`
}

type CredentialPolicy struct {
    AllowedRefs []string `yaml:"allowed_refs,omitempty"`
}

type IsolationPolicy struct {
    Mode         IsolationMode `yaml:"mode,omitempty"`
    AgentCache   bool          `yaml:"agent_cache,omitempty"`
    ToolSetCache bool          `yaml:"toolset_cache,omitempty"`
    ServiceMode  string        `yaml:"service_mode,omitempty"`
}
```

```go
// pkg/trpc-agent-go/openclaw/runtimeprofile/profile.go — Resolver
type Resolver interface {
    Resolve(ctx context.Context, req Request) (Profile, error)
}

type Request struct {
    Channel    string
    ProfileID  string
    TenantID   string
    UserID     string
    SessionID  string
    RequestID  string
    Extensions map[string]json.RawMessage
}

func RunOptions(profile Profile) []agent.RunOption
```

```go
// pkg/trpc-agent-go/openclaw/runtimeprofile/store.go — Store
type Store interface {
    Load(ctx context.Context) (Config, error)
}

type Catalog interface {
    ProfileIDs(ctx context.Context) ([]string, error)
    AppNames(ctx context.Context) ([]string, error)
}

type CachedResolver struct { ... }
func NewCachedResolver(store Store) *CachedResolver
func (r *CachedResolver) Resolve(ctx context.Context, req Request) (Profile, error)
func (r *CachedResolver) Reload(ctx context.Context) error
func (r *CachedResolver) Invalidate()
```

```go
// pkg/trpc-agent-go/openclaw/runtimeprofile/policy.go — 策略执行
func WorkspaceFromContext(ctx context.Context) (WorkspacePolicy, bool)
func CredentialPolicyFromContext(ctx context.Context) (CredentialPolicy, bool)
func ResolveWorkdir(ctx context.Context, requested string) (string, error)
func CheckCredentialRef(ctx context.Context, ref string) error
func SkillVisibilityFilter(ctx context.Context, summary skill.Summary) bool
func SkillVisibilityFilterForRepository(resolver SkillPathResolver) skill.VisibilityFilter
```

关键设计点：
- `RunOptions()` 将 Profile 转换为 `agent.RunOption` 列表，直接注入 Runner
- `toolNamesFilter()` 生成 `tool.FilterFunc`，按 include/exclude 过滤工具
- `SkillVisibilityFilter` 生成 `skill.VisibilityFilter`，按 include/exclude 过滤技能
- `WithProfile()`/`ProfileFromContext()` 通过 context 传递解析后的 Profile

### 2.2 当前项目现状

| 文件 | 现状 | 不足 |
|------|------|------|
| `internal/agent/trpc_build.go` | `BuildTRPCLLMAgent` 从 `AgentRuntimeSettings` 构建选项 | 配置是静态的，不支持动态切换 |
| `internal/agent/trpc_runtime.go` | 运行时选项构建 | 无 Profile 概念 |
| `internal/biz/agent_settings.go` | `AgentRuntimeSettings` 含 Memory/Tools/Skills 等配置 | 配置是 Agent 级别，不支持请求级覆盖 |
| `internal/agent/tool_assembly.go` | 工具装配 | 无 Profile 级工具过滤 |
| `internal/tools/` | 工具注册中心 | 无 Profile 级访问控制 |

### 2.3 架构设计

#### 模块在四层架构中的位置

```
api/**/*.proto                    ← 新增 RuntimeProfile 相关 proto
        ↓
internal/service                  ← RuntimeProfileService：proto↔biz 映射
        ↓
internal/biz                      ← RuntimeProfileUsecase + 端口接口
        ↓
internal/data                     ← RuntimeProfileRepo 实现（Ent ORM）
```

Agent 运行时模块位置：
```
internal/agent                    ← Profile 解析 + RunOption 注入
```

#### 新增/修改的文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `api/aranea/v1/runtime_profile.proto` | 新增 | RuntimeProfile HTTP/gRPC 接口 |
| `internal/service/runtime_profile.go` | 新增 | Service 层 |
| `internal/biz/runtime_profile.go` | 新增 | RuntimeProfileUsecase |
| `internal/biz/runtime_profile_types.go` | 新增 | Profile 领域模型 |
| `internal/biz/runtime_profile_repo.go` | 新增 | Repo 端口接口 |
| `internal/data/runtime_profile.go` | 新增 | Repo 实现 |
| `internal/data/ent/schema/runtime_profile.go` | 新增 | Ent Schema |
| `internal/agent/profile_resolver.go` | 新增 | Profile 解析 + RunOption 转换 |
| `internal/agent/trpc_build.go` | 修改 | 集成 Profile 解析 |
| `internal/agent/tool_assembly.go` | 修改 | 支持 Profile 级工具过滤 |
| `web/src/features/runtime-profile/api.ts` | 新增 | Profile API |
| `web/src/stores/runtime-profile/index.ts` | 新增 | Profile Store |
| `web/src/components/runtime-profile/*.vue` | 新增 | Profile 管理组件 |

#### 接口设计

```go
// internal/biz/runtime_profile_types.go

type RuntimeProfile struct {
    ID          string
    AgentID     string
    Name        string
    Description string
    IsDefault   bool
    ModelName   string
    Prompt      ProfilePrompt
    Tools       ProfileToolPolicy
    Skills      ProfileSkillPolicy
    Knowledge   ProfileKnowledgePolicy
    Workspace   ProfileWorkspacePolicy
    Credentials ProfileCredentialPolicy
    State       map[string]any
    CreatedBy   string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type ProfilePrompt struct {
    Instruction  string
    SystemPrompt string
}

type ProfileToolPolicy struct {
    Include          []string
    Exclude          []string
    ExecutionInclude []string
    ExecutionExclude []string
}

type ProfileSkillPolicy struct {
    Include []string
    Exclude []string
}

type ProfileKnowledgePolicy struct {
    Indexes []string
    Filter  map[string]any
}

type ProfileWorkspacePolicy struct {
    Workdir      string
    AllowedRoots []string
}

type ProfileCredentialPolicy struct {
    AllowedRefs []string
}
```

```go
// internal/biz/runtime_profile_repo.go

type RuntimeProfileReader interface {
    ListByAgent(ctx context.Context, agentID string) ([]RuntimeProfile, error)
    GetByID(ctx context.Context, id string) (RuntimeProfile, error)
    GetDefault(ctx context.Context, agentID string) (*RuntimeProfile, error)
}

type RuntimeProfileWriter interface {
    Create(ctx context.Context, p RuntimeProfile) (RuntimeProfile, error)
    Update(ctx context.Context, p RuntimeProfile) (RuntimeProfile, error)
    Delete(ctx context.Context, id string) error
    SetDefault(ctx context.Context, agentID string, profileID string) error
}
```

```go
// internal/biz/runtime_profile.go

type RuntimeProfileUsecase struct {
    profileRepo RuntimeProfileReader
    agents      AgentRepository
}

func NewRuntimeProfileUsecase(
    profileRepo RuntimeProfileReader,
    agents AgentRepository,
) *RuntimeProfileUsecase

func (uc *RuntimeProfileUsecase) ListProfiles(ctx context.Context, agentID string) ([]RuntimeProfile, error)
func (uc *RuntimeProfileUsecase) GetProfile(ctx context.Context, id string) (RuntimeProfile, error)
func (uc *RuntimeProfileUsecase) CreateProfile(ctx context.Context, p RuntimeProfile) (RuntimeProfile, error)
func (uc *RuntimeProfileUsecase) UpdateProfile(ctx context.Context, p RuntimeProfile) (RuntimeProfile, error)
func (uc *RuntimeProfileUsecase) DeleteProfile(ctx context.Context, id string) error
func (uc *RuntimeProfileUsecase) SetDefaultProfile(ctx context.Context, agentID string, profileID string) error
func (uc *RuntimeProfileUsecase) ResolveProfile(ctx context.Context, agentID string, profileID string) (*RuntimeProfile, error)
```

```go
// internal/agent/profile_resolver.go

type ProfileResolver struct {
    uc *RuntimeProfileUsecase
}

func NewProfileResolver(uc *RuntimeProfileUsecase) *ProfileResolver

func (r *ProfileResolver) ResolveToRunOptions(ctx context.Context, agentID string, profileID string) ([]agent.RunOption, error)

func (r *ProfileResolver) ToolFilterForProfile(p *biz.RuntimeProfile) tool.FilterFunc

func (r *ProfileResolver) SkillFilterForProfile(p *biz.RuntimeProfile) skill.VisibilityFilter
```

#### 数据流图

```
前端 Profile 管理 → RuntimeProfileService.CreateProfile/UpdateProfile
    │
    ▼
RuntimeProfileUsecase → 写入 runtime_profiles 表
    │
    ▼
会话请求 → ChatService
    │  读取请求中的 profile_id
    ▼
ProfileResolver.ResolveToRunOptions()
    │  RuntimeProfileUsecase.ResolveProfile() → 查询 Profile
    │  转换为 agent.RunOption 列表：
    │    - WithModelName(p.ModelName)
    │    - WithInstruction(p.Prompt.Instruction)
    │    - WithToolFilter(ToolFilterForProfile)
    │    - WithSkillFilter(SkillFilterForProfile)
    ▼
Runner.Run() 使用 Profile 覆盖后的选项
    │
    ▼
LLM 调用使用 Profile 指定的模型/工具/技能
```

### 2.4 与框架的集成方式

1. **RunOptions 转换**：复用 `runtimeprofile.RunOptions()` 的逻辑，将 Profile 转换为 `agent.RunOption` 列表
2. **Tool Filter**：复用 `runtimeprofile.toolNamesFilter()` 逻辑，生成 `tool.FilterFunc`
3. **Skill Filter**：复用 `runtimeprofile.SkillVisibilityFilterForRepository()` 逻辑，生成 `skill.VisibilityFilter`
4. **Context 传递**：使用 `runtimeprofile.WithProfile()`/`ProfileFromContext()` 在 context 中传递解析后的 Profile
5. **CachedResolver**：参考 `runtimeprofile.CachedResolver` 设计，实现 DB 后端的缓存解析器
6. **与 AgentRuntimeSettings 兼容**：Profile 作为运行时覆盖层，覆盖 `AgentRuntimeSettings` 中的对应字段；未覆盖的字段保持 `AgentRuntimeSettings` 的值

### 2.5 错误处理

| 场景 | 处理方式 |
|------|----------|
| Profile 不存在 | 回退到默认 Profile，记录 FlowLog Warn |
| 默认 Profile 未设置 | 使用 `AgentRuntimeSettings` 原始配置 |
| Profile 工具过滤后无可用工具 | 记录 FlowLog Warn，保留最小工具集 |
| Profile 模型不可用 | 回退到 Agent 配置的默认模型 |
| Profile 解析失败 | 跳过 Profile 覆盖，使用原始配置 |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|-----------|
| T1 | 定义 `runtime_profile_types.go` 领域模型 | 无 | S |
| T2 | 定义 `runtime_profile_repo.go` 端口接口 | T1 | S |
| T3 | 创建 Ent Schema `runtime_profile` | T1 | M |
| T4 | 实现 `internal/data/runtime_profile.go` Repo | T2, T3 | M |
| T5 | 实现 `RuntimeProfileUsecase` 核心方法 | T4 | L |
| T6 | 实现 `ProfileResolver` + RunOption 转换 | T5 | L |
| T7 | 实现 `ToolFilterForProfile` | 无 | M |
| T8 | 实现 `SkillFilterForProfile` | 无 | M |
| T9 | 集成到 `BuildTRPCLLMAgent` | T6, T7, T8 | M |
| T10 | 新增 `runtime_profile.proto` + Service 层 | T5 | M |
| T11 | Wire DI 装配 | T9, T10 | S |
| T12 | 前端 `features/runtime-profile/api.ts` | T10 | S |
| T13 | 前端 `stores/runtime-profile/index.ts` | T12 | M |
| T14 | 前端 Profile 管理组件 | T13 | L |
| T15 | 单元测试 | T5, T6, T7, T8 | M |
| T16 | 集成测试 | T11 | L |

### 3.2 开发顺序

```
Phase 1 — 数据基础（T1 → T2 → T3 → T4）
Phase 2 — 核心能力（T5 → T6 → T7 → T8 → T9）
Phase 3 — 后端接入（T10 → T11）
Phase 4 — 前端（T12 → T13 → T14）
Phase 5 — 验证（T15 → T16）
```

### 3.3 验证方案

| 阶段 | 验证方式 |
|------|----------|
| Phase 1 | `go generate ./internal/data/ent/... && go build ./...` |
| Phase 2 | `go test ./internal/agent/... -run TestProfileResolver -count=1` |
| Phase 3 | `make api && make wire && make build` |
| Phase 4 | `cd web && pnpm lint && pnpm build` |
| Phase 5 | 端到端：创建 Profile → 切换 Profile → 验证工具/技能过滤生效 |
| 提交前 | 后端 `make api && make wire && make build && make test && make lint`；前端 `cd web && pnpm lint && pnpm test && pnpm build` |
