# Tools 工具模块 — 结构设计

> **对应需求**：[23 tools.md](./23%20tools.md) · **技术设计**：[23 tools.design.md](./23%20tools.design.md) · **开发计划**：[23-tools-development.md](./23-tools-development.md)
> **遵循规范**：`AI-DEVELOPMENT-SPECIFICATION.md`
> **框架**：trpc-agent-go（`trpc.group/trpc-go/trpc-agent-go`）

---

## 一、文档定位与边界

| 维度 | 本文档范围 | 不含 |
|------|-----------|------|
| **焦点** | Go 类型定义、接口签名、目录结构、注册/装配数据流 | 产品需求（见 `23 tools.md`）、API/Proto 设计（见 `23 tools.design.md`）、开发排期（见 `23-tools-development.md`） |
| **深度** | 给出可编译的结构骨架与关键类型；AI 可据此生成实现代码 | 完整业务逻辑实现、前端 UI 设计 |
| **框架** | 以 trpc-agent-go 的 `tool.Tool` / `CallableTool` / `StreamableTool` / `ToolSet` 为核心 | 不自建 tooldef/toolctx 等抽象层；不自建洋葱中间件链 |

---

## 二、框架接口（trpc-agent-go/tool）

项目直接依赖 trpc-agent-go 框架的 tool 包，**不自建** Tool 接口或中间件链。框架提供以下核心接口：

```go
package trpctool // "trpc.group/trpc-go/trpc-agent-go/tool"

// Tool — 基础接口：所有工具的声明入口
type Tool interface {
    Declaration() *Declaration
}

// CallableTool — 可调用工具：Tool + 同步执行
type CallableTool interface {
    Tool
    Call(ctx context.Context, args []byte) (any, error)
}

// StreamableTool — 流式工具：Tool + 流式执行
type StreamableTool interface {
    Tool
    StreamableCall(ctx context.Context, args []byte) (*StreamReader, error)
}

// ToolSet — 工具集：聚合多个 Tool，支持延迟初始化与关闭
type ToolSet interface {
    Name() string
    Tools(ctx context.Context) []Tool
    Close() error
}

// Declaration — 工具声明：暴露给 LLM 的元数据
type Declaration struct {
    Name        string
    Description string
    InputSchema *Schema
}

// Schema — JSON Schema 描述
type Schema struct {
    Type       string
    Properties map[string]*Schema
    Required   []string
    // ... 其他 JSON Schema 字段
}
```

**关键决策**：项目通过 **type alias** 直接复用框架类型，不自建 tooldef / toolctx / middleware / executor 抽象层。横切关注点（校验、重试、追踪、过滤）通过框架内建的 Callbacks / Filter / Retry 机制注入，而非自建洋葱中间件链。

---

## 三、项目级类型定义

### 3.1 类型别名（internal/tools/tool.go）

```go
package tools

import (
    "context"
    trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type Tool            = trpctool.Tool
type CallableTool    = trpctool.CallableTool
type StreamableTool  = trpctool.StreamableTool
type ToolSet         = trpctool.ToolSet
type Declaration     = trpctool.Declaration
type Schema          = trpctool.Schema
```

### 3.2 工具注册项（ToolRegistration）

```go
type ToolRegistration struct {
    Name                 string
    Description          string
    Factory              func(ctx context.Context) (Tool, error)       // 单工具工厂
    ToolSetFactory       func(ctx context.Context) (ToolSet, error)    // 工具集工厂
    EnabledByDefault     bool
    Category             string   // filesystem / execution / web / search / communication / productivity / interaction / coding / integration / composition
    RiskLevel            string   // low / medium / high / critical
    RequiresConfirmation bool
    SupportsStreaming    bool
    SupportsConcurrency  bool
}
```

**语义**：
- `Factory` 与 `ToolSetFactory` 互斥：单工具用 Factory，工具集用 ToolSetFactory
- `EnabledByDefault`：true 表示 catalog 行 `enabled=true`（默认开放），false 表示仅显式允许
- `Category`：与 `builtin_tools_seed.go` 中的分类对齐
- `RiskLevel`：影响前端展示与 Agent 策略

### 3.3 装配配置（AssemblyConfig）

```go
type AssemblyConfig struct {
    EnabledTools  []string            // 从 effective tools 计算得出的启用工具名列表
    FilesystemDir string              // file ToolSet 的根目录覆盖
    GeminiModel   string              // geminifetch 的模型名
    GoogleAPIKey  string              // Google Search API Key
    GoogleCX      string              // Google Search Engine ID
    ClaudeCodeDir string              // claudecode ToolSet 的工作目录
    OpenAPISpecs  []OpenAPISpecConfig // OpenAPI 动态工具集
    AgentTools    []AgentToolConfig   // Agent-as-Tool 配置
    MCPServers    []MCPServerConfig   // MCP 服务器连接配置
    MCPBroker     *MCPBrokerConfig    // MCP Broker 配置
    CustomTools   []Tool              // 额外注入的自定义工具
}
```

### 3.4 装配输出（AssembledToolsets）

```go
type AssembledToolsets struct {
    ToolSets []ToolSet
    Tools    []Tool
}
```

### 3.5 Agent-as-Tool 配置

```go
type AgentToolConfig struct {
    Agent             trpcagent.Agent
    Name              string
    Description       string
    SkipSummarization bool
    StreamInner       bool
    HistoryScope      trpcagenttool.HistoryScope
    ResponseMode      trpcagenttool.ResponseMode
}
```

### 3.6 MCP 配置

```go
type MCPServerConfig struct {
    Name       string
    Transport  string            // stdio / sse / streamable_http
    ServerURL  string            // HTTP 传输的 URL
    Command    string            // stdio 传输的可执行文件
    Args       []string          // stdio 传输的参数
    Headers    map[string]string // HTTP 传输的请求头
    TimeoutSec int               // 连接超时
    ToolPrefix string            // 工具名前缀过滤
}

type MCPBrokerConfig struct {
    Servers         []MCPServerConfig
    AllowAdHocHTTP  bool
    AdHocTimeoutSec int
}

type OpenAPISpecConfig struct {
    Name     string
    SpecURL  string
    SpecData []byte
}
```

---

## 四、目录结构

```
internal/tools/
├── tool.go                  — 类型别名（Tool/CallableTool/StreamableTool/ToolSet/Declaration/Schema）
├── toolset.go               — Registry() 注册表 + Assemble() 装配入口
├── doc.go                   — 包文档（框架能力速查 + 注册工具清单 + 自定义工具指南）
│
├── trpc/                    — 向后兼容适配层：ToolsetConfig → AssemblyConfig → tools.Assemble()
│   └── toolsets.go          — BuildToolsets()，被 trpc_build.go 调用
│
├── custom/                  — 自定义工具实现
│   └── demo.go              — 示例：使用 function.NewFunctionTool 构建自定义工具
│
├── memory/                  — Memory 工具（add/update/load/search/delete）
│   └── tools.go             — DefaultTools() 返回 5 个标准 memory 工具
│
├── knowledge/               — Knowledge 工具
│   └── tool.go              — knowledge_search 工具 + WithRetriever context 注入
│
├── serviceawaitreply/       — 服务级 await_user_reply（阻塞式，替代框架内置版）
│   └── tool.go              — ServiceTool + ReplyFunc context 注入
│
├── mcpmount/                — MCP 服务器发现与 ToolSet 组装（迁移中）
│   ├── config.go
│   └── transport.go
│
├── skillrouter/             — Skill 检测与分类
│   ├── detect.go
│   ├── detect_test.go
│   └── taxonomy.go
│
└── skillruntime/            — Skill 工具集解析
    ├── resolve.go
    └── toolset.go
```

**与旧设计的差异**：不设 `tooldef/`、`toolctx/`、`middleware/`、`executor/`、`adkbridge/`、`backends/`、`telemetry/` 子包。框架接口直接通过 type alias 使用，横切关注点通过框架内建机制注入。

---

## 五、注册表（Registry）

### 5.1 注册工具清单

| 注册名 | Category | Factory / ToolSetFactory | EnabledByDefault | RiskLevel | RequiresConfirmation |
|--------|----------|--------------------------|------------------|-----------|---------------------|
| `file` | filesystem | `ToolSetFactory: trpcfile.NewToolSet` | ✅ | low | — |
| `hostexec` | execution | `ToolSetFactory: trpchostexec.NewToolSet` | ❌ | critical | ✅ |
| `httpfetch` | web | `Factory: trpchttpfetch.NewTool` | ❌ | medium | — |
| `claudefetch` | web | `Factory` (stub) | ❌ | medium | — |
| `geminifetch` | web | `Factory: trpcgeminifetch.NewTool` | ❌ | medium | — |
| `duckduckgo` | search | `Factory: trpcduckduckgo.NewTool` | ❌ | medium | — |
| `google_search` | search | `ToolSetFactory: trpcgooglesearch.NewToolSet` | ❌ | medium | — |
| `arxiv_search` | search | `ToolSetFactory: trpcarxivsearch.NewToolSet` | ❌ | low | — |
| `wikipedia` | search | `ToolSetFactory: trpcwikipedia.NewToolSet` | ❌ | low | — |
| `email` | communication | `ToolSetFactory: trpcemail.NewToolSet` | ❌ | high | ✅ |
| `todo` | productivity | `Factory: trpctodo.New` | ❌ | low | — |
| `await_user_reply` | interaction | `Factory: trpcawaitreply.New` | ❌ | low | — |
| `claudecode` | coding | `ToolSetFactory: trpcclaudecode.NewToolSet` | ❌ | critical | ✅ |
| `workspace_exec` | execution | `Factory: trpcworkspaceexec.NewExecTool` | ❌ | critical | ✅ |
| `openapi` | integration | `ToolSetFactory` (需 spec 配置) | ❌ | medium | — |
| `agent` | composition | 无 Factory（运行时通过 AgentToolConfig 注入） | ❌ | medium | — |
| `mcp` | integration | 无 Factory（运行时通过 MCPServerConfig 注入） | ❌ | medium | — |
| `mcpbroker` | integration | 无 Factory（运行时通过 MCPBrokerConfig 注入） | ❌ | medium | — |

### 5.2 非 Registry 注入的工具

以下工具不经过 Registry，通过其他路径注入到 Agent：

| 工具 | 注入路径 | 说明 |
|------|----------|------|
| memory (add/update/load/search/delete) | `trpc_build.go` → `memorytool.DefaultTools()` → `WithTools` | 仅当 `MemoryEnabled=true` 且 `HasMemory=true` 时注入 |
| knowledge_search | `trpc/toolsets.go` → `knowledgepkg.NewSearchTool()` → CustomTools | 仅当 `KnowledgeSearch=true` 时注入 |
| call_agent | `trpc/toolsets.go` → `a2a.NewCallAgentTool()` → CustomTools | 仅当 `CallAgent=true` 时注入 |
| await_user_reply (ServiceTool) | `trpc/toolsets.go` → `serviceawaitreply.New()` → CustomTools | 仅当 `AwaitHook != nil` 时注入，替代框架内置版 |
| skill tools | `trpc_build.go` → `WithSkills(repo)` + `WithCodeExecutor` | 通过框架 Skill 机制注入 |

---

## 六、装配数据流

### 6.1 端到端流程

```
AgentRuntimeSettings
        │
        ▼
GetEffectiveTools() ─── 计算 effective tool keys
        │                    (profile + allow/deny + catalog enabled)
        ▼
buildToolsetsForAgent() ─── ToolsetConfig 标记哪些工具启用
        │
        ▼
BuildToolsets() ─── ToolsetConfig → AssemblyConfig
        │              (enabled 名列表 + 配置参数)
        ▼
Assemble() ─── 遍历 Registry，按 enabled 名匹配
        │         Factory/ToolSetFactory 创建实例
        │         + 后处理（dir 覆盖、AgentTool、MCP、CustomTools）
        ▼
AssembledToolsets ─── { ToolSets, Tools }
        │
        ▼
BuildTRPCLLMAgent() ─── WithToolSets + WithTools
        │                 + WithToolFilter (deny)
        │                 + WithToolCallbacks (invocation 记录)
        │                 + WithToolCallRetryPolicy
        │                 + WithEnableParallelTools
        ▼
trpc-agent-go LLMAgent ─── 运行时执行
```

### 6.2 Effective Tools 计算语义

```
catalog.enabled=true  → "默认开放"：profile 门控通过且不在 deny 列表即可用
catalog.enabled=false → "仅显式允许"：必须在 allow 列表中才可用
deny 列表 + ToolsEnabled=false → 覆盖一切
```

计算公式（`computeEffectiveToolState`）：
```
baseEnabled = ToolsEnabled && (catalogOpenByDefault || policyNamesKey)
enabled     = baseEnabled && state == "allowed"  // 未被 deny
```

### 6.3 Tool Key 映射关系

| Registry 名 | Effective Tool Key | Declaration().Name | 说明 |
|-------------|-------------------|-------------------|------|
| `file` | `read_file`, `save_file`, `list_file` 等 | `read_file`, `save_file` 等 | ToolSet 展开为多个 key |
| `hostexec` | `shell_exec` | `shell_exec` | 注册名与 key 不同 |
| `duckduckgo` | `duckduckgo_search` | `duckduckgo_search` | 注册名与 key 不同 |
| `httpfetch` | `web_fetch` | `web_fetch` | 注册名与 key 不同 |
| `geminifetch` | `gemini_web_fetch` | `gemini_web_fetch` | 注册名与 key 不同 |
| `todo` | `todo_write` | `todo_write` | 注册名与 key 不同 |

> **关键约束**：`builtin_tools_seed.go` 中的 tool key 必须与框架工具的 `Declaration().Name` 一致，否则 effective tool 策略无法正确匹配。

---

## 七、框架横切机制映射

项目不自建中间件链，使用 trpc-agent-go 内建机制：

| 横切关注点 | 框架机制 | 项目注入点 | 代码位置 |
|-----------|---------|-----------|---------|
| **调用前拦截** | `tool.Callbacks.RegisterBeforeTool` | — | 暂未使用 |
| **调用后记录** | `tool.Callbacks.RegisterAfterTool` | 记录 ToolInvocation | `trpc_build.go` → `buildToolCallbacks` |
| **工具过滤** | `tool.NewExcludeToolNamesFilter` | deny 列表过滤 | `trpc_build.go` → `buildToolFilter` |
| **自动重试** | `tool.RetryPolicy` | 可配置重试策略 | `trpc_build.go` → `buildToolRetryPolicy` |
| **并行执行** | `llmagent.WithEnableParallelTools` | 开关控制 | `trpc_build.go` |
| **流式工具** | `StreamableTool` + `StreamReader` | 框架内置 | 工具实现侧 |
| **结果合并** | `tool.Merge[T]` | 框架内置 | 框架自动处理 |

### 7.1 Callbacks 注入结构

```go
callbacks := trpctool.NewCallbacks()

callbacks.RegisterAfterTool(func(ctx context.Context, args *trpctool.AfterToolArgs) (*trpctool.AfterToolResult, error) {
    recordToolInvocationAsync(ctx, args, ag, deps)
    return &trpctool.AfterToolResult{}, nil
})

// Agent 集成
llmagent.WithToolCallbacks(callbacks)
```

### 7.2 Filter 注入结构

```go
denyList := jsonStringList(settings.ToolsDenyJSON)
filter := trpctool.NewExcludeToolNamesFilter(denyList...)

// Agent 集成
llmagent.WithToolFilter(filter)
```

### 7.3 RetryPolicy 注入结构

```go
policy := &trpctool.RetryPolicy{
    MaxAttempts:     maxAttempts,      // 默认 2
    InitialInterval: initialMs * time.Millisecond,  // 默认 500ms
    BackoffFactor:   backoff,          // 默认 2.0
    MaxInterval:     maxMs * time.Millisecond,      // 默认 5000ms
    Jitter:          settings.ToolsRetryJitter,
    RetryOn:         trpctool.DefaultRetryOn,  // EOF, network timeout/temporary
}

// Agent 集成
llmagent.WithToolCallRetryPolicy(policy)
```

---

## 八、自定义工具开发模式

### 8.1 使用 function.NewFunctionTool（推荐）

```go
package custom

import (
    "context"
    trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
    "trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type MyInput struct {
    Query string `json:"query" jsonschema:"description=搜索关键词,required"`
    Limit int    `json:"limit" jsonschema:"description=最大结果数,default=5"`
}

type MyOutput struct {
    Results []MyResult `json:"results"`
}

type MyResult struct {
    Title string `json:"title"`
    URL   string `json:"url"`
}

func myExecute(ctx context.Context, input MyInput) (MyOutput, error) {
    // 业务逻辑
    return MyOutput{}, nil
}

func NewMyTool() trpctool.Tool {
    return function.NewFunctionTool(
        myExecute,
        function.WithName("my_search"),
        function.WithDescription("自定义搜索工具"),
    )
}
```

### 8.2 实现 CallableTool 接口

```go
type myTool struct{}

func (t *myTool) Declaration() *trpctool.Declaration {
    return &trpctool.Declaration{
        Name:        "my_tool",
        Description: "手动实现的工具",
        InputSchema: &trpctool.Schema{
            Type:       "object",
            Properties: map[string]*trpctool.Schema{...},
        },
    }
}

func (t *myTool) Call(ctx context.Context, args []byte) (any, error) {
    var input MyInput
    if err := json.Unmarshal(args, &input); err != nil {
        return nil, fmt.Errorf("my_tool: invalid args: %w", err)
    }
    // 业务逻辑
    return result, nil
}
```

### 8.3 注入路径

自定义工具通过 `AssemblyConfig.CustomTools` 注入：

```go
// 方式 1：通过 ToolsetConfig.CustomTools
cfg.CustomTools = append(cfg.CustomTools, custom.NewMyTool())

// 方式 2：在 BuildToolsets 中按条件注入
if cfg.KnowledgeSearch {
    customTools = append(customTools, knowledgepkg.NewSearchTool())
}
```

---

## 九、Tool Key 命名规范

| 规则 | 示例 |
|------|------|
| 使用 snake_case | `duckduckgo_search`、`web_fetch` |
| 动词_名词结构 | `read_file`、`send_email`、`search_content` |
| ToolSet 展开后的子工具保持独立 key | `file` → `read_file` + `save_file` + `list_file` + ... |
| MCP 工具加前缀 | `{server_name}_{original_tool_name}` |
| 注册名与 key 可能不同 | `duckduckgo`(注册名) → `duckduckgo_search`(key) |

**一致性约束**：
- `builtin_tools_seed.go` 的 key = `Declaration().Name` = effective tool policy 的 key
- 三处必须一致，否则策略无法生效

---

## 十、ToolInvocation 记录结构

```go
type ToolInvocationWrite struct {
    ToolKey       string    // Declaration().Name
    AgentID       string
    AgentKey      string
    SessionID     string
    UserID        string
    Status        string    // "success" / "error"
    DurationMS    int
    StartedAt     string    // RFC3339
    EndedAt       string    // RFC3339
    InputPreview  string    // 截断至 2000 字符
    OutputPreview string    // 截断至 2000 字符
    ErrorCode     string
    ErrorMessage  string    // 截断至 500 字符
    Source        string    // "adk"
    ToolCallID    string    // 模型分配的调用 ID
}
```

**脱敏规则**：
- `InputPreview` / `OutputPreview`：截断至 2000 字符，不存完整明文
- `ErrorMessage`：截断至 500 字符
- 敏感字段（API Key、Token 等）不落库

---

## 十一、Effective Tools 策略结构

### 11.1 Profile 预设

```go
var toolProfiles = map[string][]string{
    "chat_only": {},
    "read_only": {"datetime", "read_file", "read_multiple_files", "list_file", "search_file", "search_content", "todo_write"},
    "coding":    {"group:filesystem", "group:web", "group:skill", "group:session", "datetime"},
    "research":  {"duckduckgo_search", "web_fetch", "gemini_web_fetch", "google_search", "arxiv_search", "wikipedia_search",
                  "read_file", "read_multiple_files", "list_file", "search_file", "search_content",
                  "skill_search", "memory_search", "todo_write", "datetime"},
    "full":      {"group:filesystem", "group:web", "group:skill", "group:memory", "group:media",
                  "group:runtime", "group:messaging", "group:session", "group:cli_admin", "datetime"},
}
```

### 11.2 Tool Group 展开

```go
var toolGroupsFilesystem = []string{"read_file", "read_multiple_files", "save_file", "list_file", "search_file", "search_content", "replace_content"}
var toolGroupsWeb       = []string{"duckduckgo_search", "web_fetch", "gemini_web_fetch", "google_search", "arxiv_search", "wikipedia_search"}
var toolGroupsMemory    = []string{"memory_search", "memory_get"}
var toolGroupsSkill     = []string{"skill_search", "use_skill"}
var toolGroupsMedia     = []string{"read_image", "read_document", "create_image", "tts"}
var toolGroupsRuntime   = []string{"shell_exec", "claude_code", "workspace_exec"}
var toolGroupsMessaging = []string{"send_email"}
var toolGroupsSession   = []string{"await_user_reply", "todo_write"}
```

### 11.3 策略计算结果

```go
type AgentEffectiveTools struct {
    ToolsEnabled bool
    Profile      string
    Allow        []string
    Deny         []string
    Items        []EffectiveAgentTool
}

type EffectiveAgentTool struct {
    ToolKey        string
    DisplayName    string
    Category       string
    Source         string
    Enabled        bool
    EffectiveState string   // "allowed" / "denied"
    Reason         string   // "profile:coding" / "agent_deny" / "agent_tools_disabled"
}
```

---

## 十二、新增工具的步骤清单

1. 在 `internal/tools/toolset.go` 的 `Registry()` 中添加 `ToolRegistration` 条目
2. 若工具需要配置，在 `AssemblyConfig` 中添加对应字段
3. 在 `internal/tools/trpc/toolsets.go` 的 `BuildToolsets()` 中添加启用标志映射
4. 在 `internal/agent/trpc_build.go` 的 `buildToolsetsForAgent()` 中添加 effective key 到配置的映射
5. 在 `internal/data/builtin_tools_seed.go` 中添加种子数据（key 必须与 `Declaration().Name` 一致）
6. 在 `internal/biz/agent_effective_tools.go` 中按需更新 tool group 和 profile 定义
7. 编写单元测试验证注册 → 装配 → 注入链路
