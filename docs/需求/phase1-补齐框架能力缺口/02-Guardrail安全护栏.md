# Guardrail 安全护栏

## 一、需求文档

### 1.1 背景

Guardrail 是 trpc-agent-go 框架提供的安全护栏插件体系，包含三种子能力：Approval（工具审批）、PromptInjection（提示注入检测）、UnsafeIntent（不安全意图检测）。当前项目 `internal/agent/` 有 Plugin 注册机制，但未集成框架的 Guardrail 插件。Agent 在生产环境中缺乏对工具调用的审批控制和输入安全检测。

### 1.2 目标

- 集成框架 Guardrail 插件到项目的 Runner 装配流程
- 支持 Approval 工具审批：按工具粒度配置 require_approval / skip_approval / denied 策略
- 支持 PromptInjection 检测：在模型调用前检测提示注入攻击
- 支持 UnsafeIntent 检测：在模型调用前检测不安全意图
- 前端可配置各 Agent 的 Guardrail 策略

### 1.3 功能需求

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| F1 | Approval 插件集成 | P0 | 工具调用前审批，支持 LLM Reviewer 自动审批 |
| F2 | PromptInjection 插件集成 | P0 | 模型调用前检测提示注入，阻止则返回拦截响应 |
| F3 | UnsafeIntent 插件集成 | P1 | 模型调用前检测不安全意图 |
| F4 | 工具策略配置 | P0 | 按工具名配置 ToolPolicy：require_approval / skip_approval / denied |
| F5 | LLM Reviewer 自动审批 | P1 | Approval 的 Reviewer 使用 LLM 判断工具调用风险 |
| F6 | 前端 Guardrail 配置入口 | P1 | Agent 设置页增加安全护栏配置 |
| F7 | 拦截事件前端展示 | P1 | 被拦截的工具调用/输入在前端有明确提示 |

### 1.4 非功能需求

- Guardrail 拦截不得影响正常请求延迟超过 500ms（LLM Reviewer 除外）
- 所有拦截事件必须记录到 `internal/event` FlowLog
- Guardrail 配置变更后无需重启服务（热加载）
- 默认策略为 skip_approval（不阻断），显式配置后才启用审批

### 1.5 验收标准

- 配置 Approval 后，高风险工具调用被拦截并返回拒绝消息
- PromptInjection 检测到注入攻击时返回拦截响应，不调用模型
- UnsafeIntent 检测到不安全意图时返回拦截响应
- 未配置 Guardrail 时行为与现有完全一致
- 拦截事件在 FlowLog 中有记录

---

## 二、设计文档

### 2.1 框架参考（trpc-agent-go）

**核心包路径**：

| 子能力 | 包路径 |
|--------|--------|
| 顶层门面 | `pkg/trpc-agent-go/plugin/guardrail/guardrail.go` |
| Approval | `pkg/trpc-agent-go/plugin/guardrail/approval/approval.go` |
| PromptInjection | `pkg/trpc-agent-go/plugin/guardrail/promptinjection/promptinjection.go` |
| UnsafeIntent | `pkg/trpc-agent-go/plugin/guardrail/unsafeintent/unsafeintent.go` |

**核心类型和函数**：

```go
// guardrail.Plugin — 顶层门面
type Plugin struct {
    name            string
    approval        *approval.Plugin
    promptInjection *promptinjection.Plugin
    unsafeIntent    *unsafeintent.Plugin
}
func New(options ...Option) (*Plugin, error)
func (p *Plugin) Name() string
func (p *Plugin) Register(r *plugin.Registry)
func (p *Plugin) Close(ctx context.Context) error

// guardrail.Option
func WithName(name string) Option
func WithApproval(approvalPlugin *approval.Plugin) Option
func WithPromptInjection(promptInjectionPlugin *promptinjection.Plugin) Option
func WithUnsafeIntent(unsafeIntentPlugin *unsafeintent.Plugin) Option

// approval.Plugin — 工具审批
type Plugin struct { ... }
func New(options ...Option) (*Plugin, error)
func WithReviewer(reviewer review.Reviewer) Option
func WithDefaultToolPolicy(policy ToolPolicy) Option
func WithToolPolicy(name string, policy ToolPolicy) Option

// ToolPolicy 枚举
type ToolPolicy string
const (
    ToolPolicyRequireApproval ToolPolicy = "require_approval"
    ToolPolicySkipApproval    ToolPolicy = "skip_approval"
    ToolPolicyDenied          ToolPolicy = "denied"
)

// approval.Reviewer — 审批决策者
type Reviewer interface {
    Review(ctx context.Context, req *Request) (*Decision, error)
}

// promptinjection.Plugin — 提示注入检测
type Plugin struct { ... }
func New(options ...Option) (*Plugin, error)
func WithReviewer(reviewer review.Reviewer) Option
// 注册到 plugin.Registry 的 BeforeModel 回调

// unsafeintent.Plugin — 不安全意图检测
type Plugin struct { ... }
func New(options ...Option) (*Plugin, error)
func WithReviewer(reviewer review.Reviewer) Option
// 注册到 plugin.Registry 的 BeforeModel 回调
```

**关键行为**：
- `guardrail.Plugin` 实现 `plugin.Plugin` 接口（Name/Register/Close）
- Approval 注册 `BeforeTool` 回调，在工具执行前拦截
- PromptInjection 和 UnsafeIntent 注册 `BeforeModel` 回调，在模型调用前拦截
- Reviewer 是 LLM 驱动的决策器，框架内置了基于 transcript 的 review 实现
- Approval 默认策略为 `ToolPolicyRequireApproval`

### 2.2 当前项目现状

| 位置 | 现状 |
|------|------|
| `internal/agent/trpc_build.go` | 构建 LLMAgent，无 Guardrail Plugin |
| `internal/agent/builder_deps.go` | `TRPCBuilderDeps` 无 Guardrail 相关依赖 |
| `internal/plugin/trpc/` | 已有 Plugin 注册机制（用于 OnEvent/DB builtins），可扩展 |
| `internal/service/chat.go` | Runner 装配时使用 `runner.WithPlugins`，可追加 Guardrail |
| `internal/biz/agent.go` | `AgentRuntimeSettings` 无 Guardrail 配置字段 |
| 前端 | 无 Guardrail 配置入口 |

### 2.3 架构设计

**模块在四层架构中的位置**：

```
api/**/*.proto          ← 新增 guardrail 配置字段
        ↓
internal/service        ← Runner 装配时构建 Guardrail Plugin 并注入
        ↓
internal/biz            ← Agent 模型扩展 Guardrail 配置
        ↓
internal/data           ← Ent Schema 扩展（如需持久化）
```

**新增/修改的文件清单**：

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/biz/guardrail.go` | 新增 | GuardrailConfig biz 模型 + 工具策略定义 |
| `internal/agent/guardrail.go` | 新增 | `BuildGuardrailPlugin` 桥接函数 |
| `internal/agent/guardrail_reviewer.go` | 新增 | LLM Reviewer 实现（Approval/PromptInjection/UnsafeIntent） |
| `internal/biz/agent.go` | 修改 | `AgentRuntimeSettings` 新增 `Guardrail` 字段 |
| `internal/service/chat.go` | 修改 | Runner 装配时构建并注入 Guardrail Plugin |
| `api/admin/v1/agent.proto` | 修改 | 新增 Guardrail 配置消息类型 |
| `internal/data/ent/schema/` | 修改 | Agent 表扩展 guardrail JSON 字段 |

**接口设计**：

```go
// internal/biz/guardrail.go

type GuardrailConfig struct {
    Approval        *ApprovalConfig        `json:"approval,omitempty"`
    PromptInjection *PromptInjectionConfig `json:"prompt_injection,omitempty"`
    UnsafeIntent    *UnsafeIntentConfig    `json:"unsafe_intent,omitempty"`
}

type ApprovalConfig struct {
    Enabled          bool              `json:"enabled"`
    DefaultPolicy    string            `json:"default_policy"`
    ToolPolicies     map[string]string `json:"tool_policies"`
    ReviewerProvider string            `json:"reviewer_provider"`
    ReviewerModel    string            `json:"reviewer_model"`
}

type PromptInjectionConfig struct {
    Enabled          bool   `json:"enabled"`
    ReviewerProvider string `json:"reviewer_provider"`
    ReviewerModel    string `json:"reviewer_model"`
}

type UnsafeIntentConfig struct {
    Enabled          bool   `json:"enabled"`
    ReviewerProvider string `json:"reviewer_provider"`
    ReviewerModel    string `json:"reviewer_model"`
}

// internal/agent/guardrail.go

func BuildGuardrailPlugin(
    ctx context.Context,
    cfg biz.GuardrailConfig,
    modelFactory func(provider, model string) (model.LLM, error),
) (*guardrail.Plugin, error)
```

**数据流图**：

```
前端 Guardrail 配置
  → API UpdateAgent (guardrail JSON)
    → biz.AgentRuntimeSettings.Guardrail
      → service/chat.go Runner 装配
        → agent.BuildGuardrailPlugin(cfg, modelFactory)
          → guardrail.New(WithApproval/WithPromptInjection/WithUnsafeIntent)
            → runner.WithPlugins(guardrailPlugin)
              → Approval: BeforeTool 回调拦截工具调用
              → PromptInjection: BeforeModel 回调拦截提示注入
              → UnsafeIntent: BeforeModel 回调拦截不安全意图
```

### 2.4 与框架的集成方式

1. **桥接层**：`internal/agent/guardrail.go` 将 `biz.GuardrailConfig` 转换为框架 `guardrail.New()` 的 Option 链
2. **Reviewer 实现**：`internal/agent/guardrail_reviewer.go` 实现框架的 `review.Reviewer` 接口，内部调用项目的 LLM Provider
3. **装配点**：`internal/service/chat.go` 中 Runner 构建时，检查 Agent 的 Guardrail 配置，构建 Plugin 并追加到 `runner.WithPlugins`
4. **ToolPolicy 映射**：biz 层的字符串策略（`require_approval`/`skip_approval`/`denied`）直接映射到框架的 `approval.ToolPolicy` 常量

### 2.5 错误处理

| 场景 | 处理方式 |
|------|----------|
| Approval Reviewer 返回错误 | 工具调用被拒绝，返回 `CustomResult` 错误消息 |
| PromptInjection Reviewer 返回错误 | 请求被拦截，返回 `blockedResponse` |
| UnsafeIntent Reviewer 返回错误 | 请求被拦截，返回 `blockedResponse` |
| Reviewer 返回 nil Decision | 视为拒绝，记录错误日志 |
| 工具策略为 denied | 直接拒绝，不调用 Reviewer |
| LLM Reviewer 超时 | Reviewer 内部 ctx 控制超时，返回错误视为拒绝 |
| Guardrail Plugin Close 失败 | `errors.Join` 聚合所有子插件 Close 错误 |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|-----------|
| GR-01 | `internal/biz/guardrail.go`：定义 GuardrailConfig biz 模型 | 无 | S |
| GR-02 | `internal/biz/agent.go`：`AgentRuntimeSettings` 新增 `Guardrail` 字段 | GR-01 | S |
| GR-03 | `internal/agent/guardrail_reviewer.go`：实现 LLM Reviewer（Approval/PromptInjection/UnsafeIntent 三种） | GR-01 | L |
| GR-04 | `internal/agent/guardrail.go`：`BuildGuardrailPlugin` 桥接函数 | GR-01, GR-03 | M |
| GR-05 | `api/admin/v1/agent.proto`：新增 Guardrail 配置消息类型 | GR-01 | M |
| GR-06 | `make api` 重新生成 proto 代码 | GR-05 | S |
| GR-07 | `internal/service/chat.go`：Runner 装配时构建并注入 Guardrail Plugin | GR-04 | M |
| GR-08 | `internal/data/ent/schema/`：Agent 表扩展 guardrail JSON 字段 | GR-02 | M |
| GR-09 | `go generate` 重新生成 Ent 代码 | GR-08 | S |
| GR-10 | Service 层 proto↔biz 映射函数 | GR-06, GR-02 | M |
| GR-11 | 单元测试：biz 模型、桥接函数、Reviewer | GR-04 | M |
| GR-12 | 集成测试：Runner + Guardrail 拦截 | GR-07 | L |
| GR-13 | `make wire` 更新 Wire 注入 | GR-07 | S |

### 3.2 开发顺序

```
GR-01 → GR-02 → GR-03 → GR-04 → GR-05 → GR-06
                                    ↓
                          GR-07 → GR-10 → GR-11
                                    ↓
                          GR-08 → GR-09 → GR-12 → GR-13
```

### 3.3 验证方案

| 验证项 | 方法 |
|--------|------|
| biz 模型转换 | `go test ./internal/biz/... -run TestGuardrail -count=1` |
| 桥接函数 | `go test ./internal/agent/... -run TestBuildGuardrail -count=1` |
| Reviewer 实现 | `go test ./internal/agent/... -run TestReviewer -count=1` |
| Runner 集成 | `go test ./internal/service/... -run TestGuardrail -count=1` |
| Proto 生成 | `make api && go build ./...` |
| Wire 注入 | `make wire && go build ./cmd/admin` |
| 全量验证 | `make api && make wire && make build && make test` |
