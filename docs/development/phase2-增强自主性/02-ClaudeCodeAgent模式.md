# ClaudeCodeAgent 模式

## 一、需求文档

### 1.1 背景

框架 `pkg/trpc-agent-go/agent/claudecode/` 提供了 `claudeCodeAgent`，它封装本地安装的 Claude Code CLI，将 CLI 的 transcript 输出映射为 trpc-agent-go 事件流。同时框架 `pkg/trpc-agent-go/tool/claudecode/` 提供了 ClaudeCode ToolSet，包含 Bash/Edit/Read/Write/Glob/Grep/PDF/WebFetch/WebSearch 等代码操作工具。

当前项目 `internal/tools/toolset.go` 的 Registry 中已有 `claudecode` 注册项，但 `ToolSetFactory` 返回 `nil, nil`（占位未实现）。`AssemblyConfig` 中有 `ClaudeCodeDir` 字段，`Assemble()` 中有 `claudecode` 分支但仅做基础 ToolSet 构建。

ClaudeCodeAgent 模式的核心价值：让 Agent 获得完整的代码操作能力（文件读写、搜索、编辑、Shell 执行），成为真正的"编程 Agent"。

### 1.2 目标

1. 完整集成 ClaudeCode ToolSet，使 Agent 具备 Bash/Edit/Read/Write/Glob/Grep 等代码操作能力
2. 集成 ClaudeCodeAgent（CLI Agent 模式），让 Agent 可通过 CLI 执行复杂编程任务
3. 实现双模式切换：ToolSet 模式（工具级集成）和 Agent 模式（CLI 级集成）
4. 安全沙箱：限制文件操作范围、Shell 命令白名单

### 1.3 功能需求

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| F1 | ClaudeCode ToolSet 完整集成 | P0 | 实现 Registry 中 claudecode 的 ToolSetFactory |
| F2 | ToolSet 模式配置化 | P0 | 支持 baseDir/readOnly/maxFileSize/webFetch/webSearch 配置 |
| F3 | ClaudeCodeAgent CLI 模式集成 | P1 | 通过 `claudecode.New()` 创建 CLI Agent 作为子 Agent |
| F4 | Agent 模式配置化 | P1 | 支持 bin/args/env/workDir/outputFormat/rawOutputHook 配置 |
| F5 | 安全沙箱 | P0 | 文件操作限制在 baseDir 内，Shell 命令支持白名单 |
| F6 | Transcript 事件映射 | P1 | CLI 模式下将 transcript 中的工具调用映射为框架事件 |
| F7 | 双模式切换 | P2 | Agent 配置中指定 `claudecode_mode=toolset|agent` |

### 1.4 非功能需求

- ToolSet 模式下文件操作延迟 < 100ms（本地文件系统）
- Agent 模式下 CLI 启动到首事件 < 5s
- baseDir 必须为绝对路径，禁止路径穿越
- readOnly 模式下禁用 Write/Edit/NotebookEdit 工具
- Shell 命令执行超时默认 120s，可配置

### 1.5 验收标准

1. 配置 `enabled_tools=["claudecode"]` 的 Agent 能调用 Bash/Read/Write/Edit/Glob/Grep 工具
2. 配置 `claudecode_dir` 后文件操作限制在该目录内
3. `readOnly=true` 时 Write/Edit 工具不可用
4. CLI Agent 模式下能执行编程任务并返回结构化结果

---

## 二、设计文档

### 2.1 框架参考（trpc-agent-go / OpenClaw）

#### ClaudeCode Agent

**`claudecode.New`** — `pkg/trpc-agent-go/agent/claudecode/claude_agent.go`

```go
func New(opt ...Option) (agent.Agent, error)
```

`claudeCodeAgent` 实现了 `agent.Agent` 接口的 5 个方法：

```go
func (a *claudeCodeAgent) Run(ctx context.Context, invocation *agent.Invocation) (<-chan *event.Event, error)
func (a *claudeCodeAgent) Tools() []tool.Tool
func (a *claudeCodeAgent) Info() agent.Info
func (a *claudeCodeAgent) SubAgents() []agent.Agent
func (a *claudeCodeAgent) FindSubAgent(string) agent.Agent
```

关键特性：
- `Run()` 内部通过 `commandRunner` 执行 CLI 命令
- 支持 `--resume`（会话恢复）和 `--session-id`（新建会话）两种模式
- 通过 `parseTranscriptToolEvents()` 将 CLI transcript 解析为框架事件
- 事件发射走 `agent.EmitEvent(ctx, invocation, out, evt)`

**`claudecode.Option`** — `pkg/trpc-agent-go/agent/claudecode/options.go`

```go
func WithName(name string) Option
func WithBin(bin string) Option
func WithExtraArgs(args ...string) Option
func WithOutputFormat(format OutputFormat) Option
func WithEnv(env ...string) Option
func WithWorkDir(dir string) Option
func WithRawOutputHook(hook RawOutputHook) Option
```

**`commandRunner`** — `pkg/trpc-agent-go/agent/claudecode/command.go`

```go
type commandRunner interface {
    Run(ctx context.Context, cmd command) ([]byte, []byte, error)
}
```

#### ClaudeCode ToolSet

**`claudecode.NewToolSet`** — `pkg/trpc-agent-go/tool/claudecode/toolset.go`

```go
func NewToolSet(opts ...Option) (tool.ToolSet, error)
```

核心工具列表（`appendCoreTools`）：
- `newBashTool` — Shell 命令执行
- `newTaskStopTool` — 后台任务停止
- `newTaskOutputTool` — 后台任务输出
- `newReadTool` — 文件读取
- `newGlobTool` — 文件模式匹配
- `newGrepTool` — 内容搜索

写入工具（readOnly=false 时追加）：
- `newWriteTool` — 文件写入
- `newEditTool` — 文件编辑（old_string → new_string）
- `newNotebookEditTool` — Notebook 编辑

Web 工具（`appendWebTools`）：
- `newWebFetchTool` — 网页抓取
- `newWebSearchTool` — 网页搜索

**`claudecode.Option`（ToolSet）** — `pkg/trpc-agent-go/tool/claudecode/options.go`

```go
func WithBaseDir(baseDir string) Option
func WithName(name string) Option
func WithReadOnly(readOnly bool) Option
func WithMaxFileSize(maxFileSize int64) Option
func WithWebFetchOptions(webFetch WebFetchOptions) Option
func WithWebSearchOptions(webSearch WebSearchOptions) Option
```

**ToolSet 类型** — `pkg/trpc-agent-go/tool/claudecode/types.go`

```go
type bashInput struct {
    Command         string `json:"command"`
    Timeout         *int   `json:"timeout,omitempty"`
    RunInBackground bool   `json:"run_in_background,omitempty"`
}

type readInput struct {
    FilePath string `json:"file_path"`
    Offset   *int   `json:"offset,omitempty"`
    Limit    *int   `json:"limit,omitempty"`
}

type writeInput struct {
    FilePath string `json:"file_path"`
    Content  string `json:"content"`
}

type editInput struct {
    FilePath   string `json:"file_path"`
    OldString  string `json:"old_string"`
    NewString  string `json:"new_string"`
    ReplaceAll bool   `json:"replace_all,omitempty"`
}

type globInput struct {
    Pattern string `json:"pattern"`
    Path    string `json:"path,omitempty"`
}

type grepInput struct {
    Pattern     string `json:"pattern"`
    Path        string `json:"path,omitempty"`
    Glob        string `json:"glob,omitempty"`
    OutputMode  string `json:"output_mode,omitempty"`
    // ...更多字段
}
```

### 2.2 当前项目现状

| 文件 | 现状 |
|------|------|
| `internal/tools/toolset.go` Registry | `claudecode` 注册项 `ToolSetFactory` 返回 `nil, nil` |
| `internal/tools/toolset.go` Assemble | `claudecode` 分支已有，使用 `trpcclaudecode.NewToolSet(opts...)` |
| `internal/tools/toolset.go` AssemblyConfig | 有 `ClaudeCodeDir` 字段 |
| `internal/agent/factory.go` | `BizAgentFactoryOptions` 支持 AgentFactory 动态注册 |

**差距**：
1. Registry 中 claudecode 的 `ToolSetFactory` 是空实现
2. 无 ClaudeCodeAgent CLI 模式的集成
3. 无安全沙箱（baseDir 限制、命令白名单）
4. 无 WebFetch/WebSearch 工具的配置传递

### 2.3 架构设计

#### 模块在四层架构中的位置

```
internal/service     ← Runner 装配（已有）
internal/agent       ← 新增 ClaudeCodeAgent 构建器
internal/tools       ← 修改 Registry + Assemble，完善 claudecode 集成
```

#### 新增/修改的文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/tools/toolset.go` | 修改 | 完善 claudecode Registry 项的 ToolSetFactory |
| `internal/tools/toolset.go` | 修改 | 扩展 AssemblyConfig 增加 ClaudeCode 配置字段 |
| `internal/tools/toolset.go` | 修改 | 完善 Assemble() 中 claudecode 分支 |
| `internal/agent/claudecode_builder.go` | 新增 | ClaudeCodeAgent CLI 模式构建器 |
| `internal/tools/claudecode_sandbox.go` | 新增 | 安全沙箱：baseDir 限制 + 命令白名单 |

#### 接口设计

**扩展 AssemblyConfig**

```go
type ClaudeCodeConfig struct {
    BaseDir       string
    ReadOnly      bool
    MaxFileSize   int64
    Mode          string
    Bin           string
    ExtraArgs     []string
    Env           []string
    WorkDir       string
    OutputFormat  string
    WebFetch      *WebFetchConfig
    WebSearch     *WebSearchConfig
    CommandAllowList []string
}

type WebFetchConfig struct {
    AllowAll         bool
    AllowedDomains   []string
    BlockedDomains   []string
    Timeout          time.Duration
    MaxContentLength int
}

type WebSearchConfig struct {
    Provider  string
    BaseURL   string
    APIKey    string
    EngineID  string
}

type AssemblyConfig struct {
    // ...existing fields...
    ClaudeCode *ClaudeCodeConfig
}
```

**ClaudeCodeAgent 构建器**

```go
package agent

func BuildClaudeCodeAgent(cfg *tools.ClaudeCodeConfig) (trpcagent.Agent, error) {
    var opts []trpcclaudecode.Option
    if cfg.Bin != "" {
        opts = append(opts, trpcclaudecode.WithBin(cfg.Bin))
    }
    if len(cfg.ExtraArgs) > 0 {
        opts = append(opts, trpcclaudecode.WithExtraArgs(cfg.ExtraArgs...))
    }
    if len(cfg.Env) > 0 {
        opts = append(opts, trpcclaudecode.WithEnv(cfg.Env...))
    }
    if cfg.WorkDir != "" {
        opts = append(opts, trpcclaudecode.WithWorkDir(cfg.WorkDir))
    }
    return trpcclaudecode.New(opts...)
}
```

**安全沙箱**

```go
package tools

type SandboxConfig struct {
    BaseDir         string
    CommandAllowList []string
    ReadOnly        bool
}

func NewSandboxedToolSet(cfg SandboxConfig, inner tool.ToolSet) tool.ToolSet
```

#### 数据流图

**ToolSet 模式**

```
Agent 配置 enabled_tools=["claudecode"]
    ↓
tools.Assemble() → trpcclaudecode.NewToolSet(WithBaseDir, WithReadOnly, ...)
    ↓
Agent 调用 Bash/Read/Write/Edit/Glob/Grep 工具
    ↓
工具在 baseDir 内操作，受 Sandbox 约束
    ↓
结果返回 Agent
```

**Agent 模式**

```
Agent 配置 claudecode_mode=agent
    ↓
BuildClaudeCodeAgent() → claudecode.New(WithBin, WithWorkDir, ...)
    ↓
作为子 Agent 注册到 Runner
    ↓
父 Agent 通过 transfer_to_agent 调用
    ↓
ClaudeCodeAgent 执行 CLI 命令
    ↓
Transcript 解析 → 事件流 → 父 Agent
```

### 2.4 与框架的集成方式

1. **ToolSet 模式**：直接使用 `trpcclaudecode.NewToolSet(opts...)`，通过 `tools.Assemble()` 注入 Agent
2. **Agent 模式**：使用 `claudecode.New(opts...)` 创建 `agent.Agent`，通过 `runner.WithAgentFactory()` 注册
3. **事件发射**：Agent 模式下 CLI 输出通过 `agent.EmitEvent()` 映射为框架事件
4. **安全沙箱**：在 ToolSet 外层包装 `SandboxedToolSet`，拦截并校验文件路径和命令

### 2.5 错误处理

| 错误场景 | 处理方式 |
|----------|----------|
| Claude CLI 未安装 | `claudecode.New()` 返回错误，Agent 降级为无代码能力 |
| 文件路径穿越 baseDir | Sandbox 拦截，返回 `kerrors.BadRequest` |
| Shell 命令不在白名单 | Sandbox 拦截，返回 `kerrors.Forbidden` |
| CLI 执行超时 | `commandRunner` 返回超时错误，Agent 发射 `ErrorTypeFlowError` 事件 |
| Transcript 解析失败 | `emitFlowError` 发射错误事件，Agent 可重试 |
| readOnly 模式下调用写入工具 | 工具声明中不包含写入工具，LLM 不会调用 |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|-----------|
| P2-01 | 完善 Registry 中 claudecode 的 ToolSetFactory | 无 | S |
| P2-02 | 扩展 AssemblyConfig 增加 ClaudeCodeConfig | 无 | S |
| P2-03 | 完善 Assemble() 中 claudecode 分支 | P2-02 | M |
| P2-04 | 实现 `internal/tools/claudecode_sandbox.go` 安全沙箱 | 无 | M |
| P2-05 | 集成 Sandbox 到 Assemble 流程 | P2-03, P2-04 | M |
| P2-06 | 实现 `internal/agent/claudecode_builder.go` CLI Agent 构建器 | 无 | M |
| P2-07 | 集成 CLI Agent 到 Runner 装配 | P2-06 | M |
| P2-08 | WebFetch/WebSearch 配置传递 | P2-03 | S |
| P2-09 | 单元测试：ToolSet 模式 | P2-05 | M |
| P2-10 | 单元测试：Agent 模式 | P2-07 | M |
| P2-11 | 集成测试：安全沙箱 | P2-05 | M |

### 3.2 开发顺序

```
Phase 1（ToolSet 模式）: P2-01 → P2-02 → P2-03 → P2-04 → P2-05 → P2-08
Phase 2（Agent 模式）: P2-06 → P2-07
Phase 3（验证）: P2-09 → P2-10 → P2-11
```

### 3.3 验证方案

| 验证项 | 方法 |
|--------|------|
| ToolSet 工具可用性 | 单元测试：验证 NewToolSet 返回包含 Bash/Read/Write/Edit/Glob/Grep 的 ToolSet |
| baseDir 限制 | 单元测试：尝试读取 baseDir 外文件，验证被拦截 |
| readOnly 模式 | 单元测试：验证 readOnly=true 时 Tools() 不包含 Write/Edit |
| 命令白名单 | 单元测试：尝试执行白名单外命令，验证被拦截 |
| CLI Agent 创建 | 单元测试：验证 New() 返回实现 agent.Agent 的实例 |
| CLI Agent Run | 集成测试：模拟 CLI 执行，验证事件流输出 |
| 端到端 | 手动测试：配置 claudecode 工具的 Agent，执行代码操作任务 |
