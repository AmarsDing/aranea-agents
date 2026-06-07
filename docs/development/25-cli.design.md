# 25 Aranea CLI — 设计文档（Design, 2026-05-27）

> **版本**：3.1（取代 `25 cli.design.md` v2.0）
> **同系列**：需求 → [`25-cli.md`](./25-cli.md)；开发计划 → [`25-cli.development.md`](./25-cli.development.md)；上层方案 → [`25-cli-implementation.md`](./25-cli-implementation.md)
> **规范基线**：`docs/guides/AI-DEVELOPMENT-SPECIFICATION.md` · `.cursor/rules/trpc-agent-framework-first.mdc` · `docs/AGENT_RUNTIME_BOUNDARY.md`

---

## 0. 文档导读

本文档是 Aranea CLI 的**技术设计**：

- 不重复 PRD 的产品意图；
- 不复制开发计划的任务表；
- 聚焦：架构 / 目录结构 / 关键类型 / 与后端的契约 / 数据流 / 错误与配置 schema / 红线守护。

为方便 AI 落地，每个章节按 **「契约 → 结构 → 代码骨架 → 测试策略」** 的顺序组织；代码骨架仅给接口形状，**不替代真实实现**。

---

## 1. 架构总览

### 1.1 三层

```
┌──────────────────────────────────────────────────────────────────────┐
│                            cmd/aranea/                               │
│   main.go (~30 行) → internal/cli.Execute(ctx)                       │
└──────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────────────┐
│  internal/cli/  —— 全部业务逻辑（cobra commands、output、repl、ctx） │
│  ├─ 直接命令模式: command → client.HTTP.Do → output.Printer          │
│  └─ 对话模式  : command → repl → client.WS → render                  │
└──────────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────────────┐
│  internal/cli/client/  —— 唯一对外出口                              │
│  ├─ http.go (Bearer/重试/--debug)                                    │
│  ├─ ws.go   (envelope 编解码)        ──┐                             │
│  └─ errors.go (后端 error → 退出码) ──┤  P0: http; P1: ws            │
└──────────────────────────────────────────────────────────────────────┘
                              │
                              │ HTTP /v1/*  ·  WS /v1/ws
                              ▼
┌──────────────────────────────────────────────────────────────────────┐
│  cmd/admin  ◄── Kratos v2 (HTTP) + internal/server/ws.go (WS)        │
│  ▲                                                                   │
│  └── Runner 装配仍只在 internal/service/                             │
│      (内含 P1 新增 internal/tools/cli_admin/* 工具集)                │
└──────────────────────────────────────────────────────────────────────┘
```

### 1.2 单向依赖（**红线**，由 lint R12 强制）

```
cmd/aranea/         ─┐
internal/cli/**      ├─► api/kratos/*/v1 (pb)
internal/cli/cmd/**  │  pkg/auth (仅常量 / 解析函数)
internal/cli/client/ │  pkg/safego
                     └─► (无)  internal/biz | internal/data | internal/agent
                              | internal/server | internal/service
                              | pkg/trpc-agent-go
```

> **必须**禁止：CLI 二进制 import `internal/biz`、`internal/data`、`internal/agent`、`internal/server`、`internal/service`、`pkg/trpc-agent-go`。
> 系统管家 Agent / `cli_admin_*` 工具集**只**在后端实现；CLI 不参与种子。

### 1.3 入口路由

`cmd/aranea/main.go` 几乎为空壳：

```go
package main

import (
    "context"
    "os"

    cli "aranea-agents/internal/cli"
)

var (
    Version = "dev"
    Commit  = "none"
)

func main() {
    ctx := context.Background()
    if err := cli.Execute(ctx, cli.BuildInfo{Version: Version, Commit: Commit}); err != nil {
        os.Exit(cli.ExitCodeOf(err))
    }
}
```

无参数与有参数路由由 `internal/cli/root.go` 中 cobra root 的 `RunE` 决定（无子命令且非 help/version → 进 REPL；P0 阶段直接打印"REPL 待 P1 上线"提示并退出）。

---

## 2. 目录结构

### 2.1 `cmd/aranea/`

```
cmd/aranea/
├── main.go              # ~30 行入口；声明 Version / Commit；调 internal/cli.Execute
└── (无其它文件)
```

> **不在 cmd/aranea 下放任何业务文件**：`cmd/` 不便测试，业务全部下沉到 `internal/cli/`。

### 2.2 `internal/cli/`

```
internal/cli/
├── execute.go              # Execute(ctx, BuildInfo) error；Build root + Run
├── root.go                 # NewRoot(ctx) *cobra.Command；全局 flag；PersistentPreRunE
├── ctx.go                  # type Context / WithCLI / CLIFrom；持有 Config + Client + Printer
├── exit.go                 # ExitCodeOf(err) int；退出码常量
├── buildinfo.go            # type BuildInfo struct { Version, Commit, BuildTime string }
│
├── clierr/                 # CLI 错误类型（独立包，避免循环依赖）
│   ├── clierr.go           # type Error struct { Code, HTTPStatus, Message, Hint, Metadata, Cause }
│   └── clierr_test.go
│
├── config/
│   ├── config.go           # CLIConfig + Load/Save/OverrideFromEnv/OverrideFromFlags
│   ├── paths.go            # 跨平台路径解析；FilePerm 检查
│   ├── secret.go           # token mask / 0600 校正
│   ├── config_test.go
│   └── secret_test.go
│
├── client/
│   ├── http.go             # type Client；Do(ctx,method,path,body,out) error
│   ├── ws.go               # type WSClient；Envelope 编解码
│   ├── errors.go           # decode(*http.Response,out) error；clierr.Error 映射
│   ├── retry.go            # 仅幂等 GET 重试；指数退避
│   ├── multipart.go        # Skill import 专用
│   ├── agent.go            # ListAgents / GetAgent / CreateAgent / ...
│   ├── skill.go
│   ├── tool.go
│   ├── system.go
│   ├── admin.go            # Login / Current
│   ├── team.go
│   ├── plugin.go
│   ├── mcp.go
│   ├── cron.go
│   ├── channel.go
│   ├── session.go
│   ├── graph.go            # Graph 资源（实施中新增）
│   ├── pack.go             # Pack 资源（实施中新增）
│   ├── agent_test.go
│   ├── skill_test.go
│   ├── tool_test.go
│   ├── client_test.go
│   └── errors_test.go
│
├── output/
│   ├── printer.go          # interface Printer + 工厂
│   ├── text.go             # TTY/非TTY 自动切换；表格；color
│   ├── json.go             # protojson.Marshal
│   ├── kv.go               # 非 TTY 的 key=value 退化
│   ├── golden/             # *.golden 测试快照
│   └── output_test.go
│
├── ui/
│   ├── tty.go              # isatty / NO_COLOR / 宽度探测
│   ├── color.go            # fatih/color wrapper
│   ├── spinner.go          # ASCII spinner；TTY 才动；超 5s 显示已耗时
│   ├── table.go            # tablewriter wrapper
│   ├── prompt.go           # 二次确认 / 多选；P1 也复用
│   └── ui_test.go
│
├── repl/
│   ├── repl.go             # 主循环；连 client.WS
│   ├── slash.go            # 斜杠命令处理
│   ├── render.go           # WS 事件 → 终端折叠块
│   ├── history.go          # peterh/liner 行编辑 + 历史
│   ├── slash_test.go
│   └── repl_test.go        # mock WS server
│
└── cmd/                    # 一个资源一个文件；handler 只组装请求 + 调 client + 调 printer
    ├── version.go
    ├── login.go
    ├── config_cmd.go
    ├── system.go
    ├── agent.go
    ├── skill.go
    ├── skill_install.go
    ├── tool.go
    ├── chat.go
    ├── team.go
    ├── plugin.go
    ├── mcp.go
    ├── cron.go
    ├── channel.go
    ├── session.go
    ├── graph.go             # 实施中新增
    ├── import.go            # 实施中新增（import 命令入口）
    ├── pack.go              # 实施中新增
    ├── pkg.go               # 实施中新增（pkg install 快捷方式）
    ├── config_test.go
    └── completion.go
```

### 2.3 后端侧新增目录

```
internal/service/
├── system_info.go          GET /v1/system/info Handler（已实现；手动注册 HTTP handler，非 proto 生成）
└── ... cli_admin 工具集装配到 system_admin Agent 走这里

internal/data/
├── seed_system_admin.go    启动时 upsert __system_admin__（已实现；含 SeedSpiritAgent/SeedMemoryAgent/SeedSkillsAgent/SeedBuiltinCLIAdminTools 等）
└── ...

internal/tools/cli_admin/   系统管家工具集；走 service 注入（已实现）
├── registry.go             # RegisterAll(deps) []tool.Tool
├── agent_tools.go          # agent_list / agent_get
├── skill_list.go
├── skill_install_from_url.go
├── pkg_install_from_url.go # 实施中新增：包安装
├── registry_test.go
├── agent_tools_test.go
├── skill_install_from_url_test.go
├── pkg_install_test.go
└── export_test.go
```

> **`internal/tools/cli_admin/` 不 import `cmd/aranea` 或 `internal/cli`**（反向依赖会污染服务端二进制）。CLI 与 `cli_admin` 工具完全解耦：前者是终端二进制，后者是服务端 Agent 工具。

---

## 3. 关键类型契约

### 3.1 `internal/cli` 入口

```go
package cli

type BuildInfo struct {
    Version   string
    Commit    string
    BuildTime string
}

// Execute 是 cmd/aranea/main.go 唯一入口。
func Execute(ctx context.Context, bi BuildInfo) error

// ExitCodeOf 把 error 翻译成退出码（见 §5 退出码表）。
func ExitCodeOf(err error) int

// clierr.Error 是 client/errors.go 输出的错误类型；独立包避免循环依赖。
// root 在 RunE 之外捕获并转退出码。
type Error struct {
    Code       string          // 后端 error.code 或 CLI 自定义（如 "USER_CANCELED"）
    HTTPStatus int             // 0 表示非 HTTP
    Message    string
    Hint       string
    Metadata   map[string]any
    Cause      error
}

func (e *Error) Error() string
func (e *Error) Unwrap() error
```

### 3.2 Context

`Context` 是 CLI 内贯穿一整次命令调用的可注入对象，避免 cobra 全局变量。

```go
package cli

type Context struct {
    Cfg     *config.CLIConfig
    Client  *client.Client       // P0 仅 HTTP；P1 加 WS 字段
    Printer output.Printer
    UI      ui.UI                // tty / color / spinner 句柄
    Logger  *log.Logger          // 写 ~/.cache/aranea/logs
    Debug   bool
    Quiet   bool
    AutoYes bool                 // --yes / 会话内 /yes
}

// 透过 cobra.Command 的 context 传递
func WithCLI(parent context.Context, c *Context) context.Context
func CLIFrom(ctx context.Context) *Context
```

### 3.3 Config

```go
package config

type CLIConfig struct {
    Backend   BackendConfig   `toml:"backend"`
    UI        UIConfig        `toml:"ui"`
    Skill     SkillConfig     `toml:"skill"`
    Chat      ChatConfig      `toml:"chat"`     // P1
    Telemetry TelemetryConfig `toml:"telemetry"`
}

type BackendConfig struct {
    BaseURL     string `toml:"base_url"`
    Token       string `toml:"token"`
    WorkspaceID string `toml:"workspace_id"` // 预留；本期无效
}

type UIConfig struct {
    Output string `toml:"output"` // text | json （P1: yaml | table）
    Color  string `toml:"color"`  // auto | always | never
}

type SkillConfig struct {
    DefaultDecision string `toml:"default_decision"` // ask|skip|keep|refine
    MaxZipMB        int    `toml:"max_zip_mb"`
    KeepTemp        bool   `toml:"keep_temp"`
}

type ChatConfig struct {
    DefaultAgent string `toml:"default_agent"`
    AutoResume   bool   `toml:"auto_resume"`
}

type TelemetryConfig struct {
    Enabled bool `toml:"enabled"`
}

// 加载、保存、路径
func Load(path string) (*CLIConfig, error)           // 空 path = 默认路径
func (c *CLIConfig) Save(path string) error          // 写文件并校正权限到 0600
func (c *CLIConfig) OverrideFromEnv()                // 读 ARANEA_*
func (c *CLIConfig) OverrideFromFlags(base, token, output string, noColor bool)
func DefaultPath() (string, error)                   // 跨平台
func EnsureSecurePerm(path string) error             // 读时检查 ≤0600；Win 跳过
```

### 3.4 Client

```go
package client

type Doer interface {
    Do(*http.Request) (*http.Response, error)
}

type Client struct {
    Base     string
    Token    string
    UA       string         // "aranea/<version> (<os>/<arch>)"
    Doer     Doer
    Debug    bool
    LogFunc  func(format string, args ...any)
}

// Do 是统一入口。
//   - body / out 均为 proto.Message（CLI 直接 import api/kratos/*/v1 的 pb 类型）。
//   - body / out 任一可为 nil。
//   - 自动 Content-Type / Authorization / UA / 重试（仅幂等）。
//   - 错误经 errors.go 解码为 *CLIError 返回。
func (c *Client) Do(ctx context.Context, method, path string, body, out proto.Message) error

// DoRaw 用于 multipart / 非 pb 响应（如登录的 Set-Cookie 解析）。
func (c *Client) DoRaw(ctx context.Context, req *http.Request) (*http.Response, error)
```

资源方法签名（举例）：

```go
package client

import agentv1 "aranea-agents/api/kratos/agent/v1"

func (c *Client) ListAgents(ctx context.Context, req *agentv1.ListAgentsRequest) (*agentv1.ListAgentsResponse, error)
func (c *Client) GetAgent(ctx context.Context, id string) (*agentv1.Agent, error)
func (c *Client) CreateAgent(ctx context.Context, req *agentv1.CreateAgentRequest) (*agentv1.Agent, error)
func (c *Client) UpdateAgent(ctx context.Context, id string, req *agentv1.UpdateAgentRequest) (*agentv1.Agent, error)
func (c *Client) DeleteAgent(ctx context.Context, id string) error
// ... 同理 Skill / Tool / Team / Plugin / ...
```

**禁止**：

- 在 `client/*.go` 之外 import 任何 `api/kratos/*/v1` —— 防止 pb 类型从客户端层漏出到 `cmd/` / `ui/` 等；
- 在 `client/` 内手抄请求 / 响应 struct（违反方案 §0 D7）。

### 3.5 Output / Printer

```go
package output

type Format string

const (
    FormatText Format = "text"
    FormatJSON Format = "json"
)

type Printer interface {
    PrintList(items any, total int) error    // any = []proto.Message；reflection 提取字段
    PrintDetail(item proto.Message) error
    PrintError(e *cli.CLIError) error
    PrintSuccess(message string, kv ...string) error
    PrintKeyValue(pairs ...string) error
}

func NewPrinter(format Format, quiet, noColor bool, w io.Writer) Printer
```

`text` 实现要点：

- TTY 检测后选「带色 + 表格」分支；
- 非 TTY 选「key=value」分支（同字段名 snake_case）；
- `--quiet`：list 每行 `id`；detail 仅 `id\tstatus`；
- 字段提取：对 pb message 用 `protoreflect` 拿 `JSONName`，避免硬编码字段名；
- 长字符串自动截断（默认 60 字符）+ 终端宽度自适应；
- 字段排序优先级：`id` / `key` / `name` / `display_name` / `status` / `enabled` / 其余按 proto 顺序。

`json` 实现：

- 使用 `protojson.MarshalOptions{Multiline:true, Indent:"  ", UseProtoNames:false}`；
- `--quiet` 时仅 marshal `{ "id": "..." }` 单字段；
- 错误 schema 严格符合 PRD §5.3。

### 3.6 UI

```go
package ui

type UI struct {
    IsTTY     bool
    Width     int
    NoColor   bool
    Verbose   bool
    In        io.Reader
    Out, Err  io.Writer
}

func Detect(in io.Reader, out, err io.Writer, noColorFlag bool) UI

func (u UI) Spinner(label string) StopFunc                 // 阻塞 >200ms 才显示；超 5s 加 "(Xs)"
func (u UI) ConfirmYesNo(prompt string, defaultYes bool) (bool, error)
func (u UI) Select(prompt string, choices []string) (int, error)
func (u UI) Color(name string) ColorFn                     // red/yellow/green/dim/bold
```

---

## 4. 后端契约（CLI 视角）

### 4.1 既有可直接复用

| 资源 | 路径 | proto 文件 |
|------|------|-----------|
| Admin Login | `POST /v1/admins/login`（`noAuthPaths` 已放行） | `api/kratos/admin/v1/admin.proto` (`AdminService.Login`) |
| Admin Current | `GET /v1/admins/current` | 同上 |
| Agent CRUD + tools | `/v1/agents*` / `/v1/agents/{id}/tools/effective` / `/v1/agents/{agent_id}/tools/policy` | `api/kratos/agent/v1/agent.proto` |
| Skill CRUD + import | `/v1/skills*` / `/v1/skills/import` / `/v1/skills/import/{job_id}` / `.../apply` / `.../conflict-groups/{group_id}/refine` | `api/kratos/skill/v1/skill.proto` |
| Tool | `/v1/tools*` | `api/kratos/tool/v1/tool.proto` |
| Team / Plugin / MCP / Cron / Channel / Session / Monitor / LLM provider | `/v1/teams*` / `/v1/plugins*` / `/v1/mcp-servers*` / `/v1/cron-tasks*` / `/v1/channels*` / `/v1/sessions*` / `/v1/monitor/*` / `/v1/llm-provider-models*` | 对应 proto |
| Chat 上下行 | `/v1/chat/messages` / `/v1/chat/await-reply` / `/v1/chat/messages/enqueue` / WS `/v1/ws` | `api/kratos/chat/v1/chat.proto`（含 `AwaitUserReply`, `EnqueueUserMessage`） |

> 路径以 proto 注解为唯一真相源；实施前对 `api/kratos/<svc>/v1/*.proto` 的 `google.api.http` 做精确比对（许多资源有别于 `RESTful` 命名）。

### 4.2 必须新增的后端能力（详细需求见 PRD §3.4）

#### 4.2.1 BE-1：`GET /v1/system/info`（P0，CLI-07）— 已实现

**已实现**：`internal/service/system_info.go` + `internal/server/http.go` 手动注册路由。

> **注意**：当前实现为手动 HTTP handler（非 proto 生成），路由在 `internal/server/http.go` 第 159-161 行注册。

**当前服务端 `SystemInfoResponse` 字段**（实际）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `version` | string | 后端版本 |
| `git_commit` | string | Git commit |
| `build_time` | string | 构建时间 |
| `default_provider` | string | 默认 LLM provider |
| `default_model` | string | 默认模型 |
| `skill_storage_root` | string | Skill 存储根路径 |
| `features` | map | 功能特性检测 |

**与设计预期的差距**（待补齐）：

| 缺失字段 | 说明 |
|----------|------|
| `system_admin_agent_id` | 系统管家 Agent ID（当前未返回） |
| `system_admin_agent_key` | 系统管家 Agent key（当前未返回） |
| `skill_max_zip_mb` | Skill zip 最大体积限制（当前未返回） |

#### 4.2.2 BE-2：`POST /v1/skills/import` 接收来源字段（P1，CLI-25）

**不改 proto 服务**。multipart form 解析端（`internal/service/skill_import.go`）扩展：

| form 字段 | 类型 | 写入位置 |
|-----------|------|----------|
| `file` | binary | zip 内容（已有） |
| `source` | string | `metadata_json.source`（如 `cli_url` / `cli_path`） |
| `source_url` | string | `metadata_json.source_url` |
| `source_ref` | string | `metadata_json.source_ref` |
| `source_subpath` | string | `metadata_json.source_subpath` |
| `client_validation` | JSON string | `metadata_json.client_validation` |

后端在 `SkillImportJob` / `tool_invocations` / `audit_logs` 中保留以上字段，便于审计追溯。

#### 4.2.3 BE-3 / BE-4：种子数据（P1，CLI-20）— 已实现

**已实现**：`internal/data/seed_system_admin.go` 包含以下 seed 函数：
- `SeedSystemAdminAgent` — upsert `agent_key = "__system_admin__"`，`readonly=1`, `kind="system"`, `tools_profile="system_admin"`
- `SeedSpiritAgent` / `SeedMemoryAgent` / `SeedSkillsAgent` — 其他系统 Agent
- `SeedBuiltinCLIAdminTools` — 注册 `cli_admin_*` 工具记录
- `SeedCronTasks` / `SeedSpiritPromptFiles` / `SeedButlerPromptFiles` — 其他种子数据

> **与原设计的差异**：原设计计划在 `internal/data/ent/schema/agent.go` 新增 `readonly` / `kind` 字段。实际实现中需确认 Ent schema 是否已包含这些字段。

#### 4.2.4 BE-5：`cli_admin_*` 工具实现（P1，CLI-21/29）— 部分实现

**已实现**（`internal/tools/cli_admin/`）：
- `cli_admin_skill_list` / `cli_admin_skill_get`
- `cli_admin_skill_install_from_url`
- `cli_admin_skill_import_status` / `cli_admin_skill_import_apply`
- `cli_admin_agent_list` / `cli_admin_agent_get`
- `cli_admin_pkg_install_from_url`（实施中新增）

**待实现**（CLI-29）：
- `cli_admin_team_*` / `cli_admin_plugin_*` / `cli_admin_mcp_*`
- `cli_admin_cron_*` / `cli_admin_channel_*` / `cli_admin_provider_*` / `cli_admin_session_*`

```go
// internal/tools/cli_admin/registry.go
package cliadmin

type Deps struct {
    SkillUC   *biz.SkillUsecase
    AgentUC   *biz.AgentUsecase
    TeamUC    *biz.TeamUsecase
    ToolUC    *biz.ToolUsecase
    // ... 走 biz 层接口，不直接打 DB
}

// RegisterAll 由 internal/service 调用，注入到 system_admin Agent 工具集。
func RegisterAll(deps Deps) []tool.Tool
```

工具命名：`cli_admin_<resource>_<action>`（保持与 PRD §3.1 一致）。

首批（P1，CLI-21）：

- `cli_admin_skill_list`
- `cli_admin_skill_get`
- `cli_admin_skill_install_from_url`
- `cli_admin_skill_import_status`
- `cli_admin_skill_import_apply`
- `cli_admin_agent_list`
- `cli_admin_agent_get`

完整（P1，CLI-29）：team / plugin / mcp / cron / channel / provider / session 全量。

**风险白名单**（Q4 推荐答案）：

```go
// 后端硬编码：仅 __system_admin__ 可加载 group:cli_admin
const SystemAdminAgentKey = "__system_admin__"

func IsCLIAdminAllowed(agentKey string) bool {
    return agentKey == SystemAdminAgentKey
}
```

#### 4.2.5 BE-6：WS envelope 细化（P1，CLI-22 前置）

`internal/event/contract/envelope.go` 已定义 envelope 类型常量。CLI REPL 渲染需要的 `Type` 取值：

| 期望 Type | 含义 | 当前实现状态 |
|-----------|------|------------|
| `message.delta` | 模型增量文本 | 待对照 `internal/event/` 实际值 |
| `tool.call` | 工具调用开始 | **已实现**（`EnvelopeTypeToolCall`） |
| `tool.result` | 工具结果 | **已实现**（`EnvelopeTypeToolResult`） |
| `tool.error` | 工具错误 | **未实现**：当前使用通用 `EnvelopeTypeError`（值为 `"error"`），非 `tool.error` 子类型 |
| `await.user.reply` | 等待用户回复 | 已通过 `AwaitUserReply` 协议 |
| `system.done` / `done` | 一轮结束 | 已有 |

> **差距**：`tool.error` 作为独立 envelope 子类型尚未定义。当前 `internal/cli/repl/render.go` 消费 `tool_call` 和 `tool_result`，但错误事件走通用 `error` 类型。建议后续在 `internal/event/contract/envelope.go` 新增 `EnvelopeTypeToolError = "tool.error"`，并在 `internal/event/span_collector.go` 中投影工具错误到该类型。

**Payload schema**（CLI-22 实施前与后端对齐）：

```json
{ "type": "tool.call",   "payload": { "tool_call_id": "tc_001", "tool_name": "cli_admin_skill_install_from_url", "arguments": { "url": "..." } } }
{ "type": "tool.result", "payload": { "tool_call_id": "tc_001", "summary": "...", "raw_size_bytes": 1234 } }
{ "type": "tool.error",  "payload": { "tool_call_id": "tc_001", "code": "...", "message": "...", "http_status": 409 } }
{ "type": "await.user.reply", "payload": { "request_id": "await_001", "prompt": "...", "expect": "yesno|select|free" } }
```

---

## 5. 数据流

### 5.1 直接命令（P0）

```
User → cobra cmd handler (internal/cli/cmd/*.go)
        │
        │ assemble *agentv1.ListAgentsRequest
        ▼
      Client.Do(ctx, GET, "/v1/agents", req, resp)   ← internal/cli/client/http.go
        │
        │ protojson.Marshal(body) / decode(resp, out)
        ▼
      HTTP /v1/agents  ──►  cmd/admin (Kratos HTTP server)
        │
        │ 200 OK + protojson body  OR  4xx/5xx + Kratos error
        ▼
      handler → Printer.PrintList(resp.Items, total)
        OR
      handler returns *CLIError → root catches → ExitCodeOf → os.Exit(code)
```

### 5.2 Skill install from URL（P1）

```
User: aranea skill install https://github.com/.../tree/main/figma-code-connect

internal/cli/cmd/skill_install.go
   │
   ├─ ui.Spinner("解析 URL")
   │  └─ skillinstall.ParseURL(url) → (owner, repo, ref, subpath)
   │
   ├─ ui.Spinner("拉取仓库")
   │  └─ go-git Clone(shallow, ref) → ~/.cache/aranea/tmp/<job>/repo
   │
   ├─ ui.Spinner("定位 SKILL.md")
   │  └─ skillinstall.LocateRoot(dir, subpath)
   │
   ├─ ui.Spinner("本地预校验")
   │  └─ skillinstall.Validate(dir, cfg.Skill.MaxZipMB)
   │
   ├─ ui.Spinner("打包 zip")
   │  └─ skillinstall.Pack(dir, zipPath)
   │
   ├─ ui.Spinner("上传到后端")
   │  └─ Client.ImportSkill(ctx, multipart{file, source, source_url, ...})
   │     ◄── HTTP POST /v1/skills/import → SkillImportJob{job_id, status:pending}
   │
   ├─ poll: every 1.5s, max 80 times (cap 120s)
   │  └─ Client.GetImportStatus(ctx, job_id)
   │     ◄── HTTP GET /v1/skills/import/{job_id} → SkillImportJob{status:pass|warn|block}
   │
   ├─ branch:
   │     pass  → Client.ApplyImport(ctx, job_id, decisions=[]) → exit 0
   │     warn  →
   │              interactive: ui.Select(...) → decision
   │              non-interactive + --decision: use that
   │              non-interactive + no --decision: exit 5
   │              → Client.ApplyImport(ctx, job_id, decisions=[...])
   │     block → exit 5 + print block reasons
   │
   └─ cleanup tmp dir unless cfg.Skill.KeepTemp
```

### 5.3 对话模式（P1）

```
User: aranea
   │
   ├─ root.RunE: 进 REPL
   │  └─ repl.Run(ctx, client)
   │
   ├─ WS Dial /v1/ws?token=...  (token via Sec-WebSocket-Protocol 或 query；实施前对照 ws.go)
   │  └─ goroutine: readPump → ch_evt
   │
   ├─ liner 主循环:
   │     line := liner.Prompt("aranea> ")
   │     if line.startsWith("/"): slash.Handle(line)
   │     else:
   │        Client.WS.Send({type:"user.message", payload:{session_id, agent_key, content:line}})
   │
   ├─ render goroutine: for evt := range ch_evt
   │     switch evt.Type {
   │       case "message.delta":  render.AppendDelta(evt.Payload.text)
   │       case "tool.call":      render.OpenFold(evt.Payload.tool_name, evt.Payload.arguments)
   │       case "tool.result":    render.CloseFold(✓, evt.Payload.summary)
   │       case "tool.error":     render.CloseFold(✗, evt.Payload.code, evt.Payload.message)
   │       case "await.user.reply":
   │           answer := ui.Prompt(evt.Payload.prompt)
   │           Client.HTTP.Do(POST /v1/chat/await-reply, {request_id, answer})
   │       case "done":           render.End(); ch_done <- true
   │     }
   │
   └─ Ctrl+C 在生成中: client.HTTP.Do(POST /v1/chat/run-status/cancel ...)
      Ctrl+C 在 prompt 阶段: exit 4
      Ctrl+D: 退出 REPL
```

---

## 6. 错误处理与退出码

### 6.1 解码流程

```go
// internal/cli/client/errors.go
func decode(resp *http.Response, out proto.Message) error {
    if resp.StatusCode < 300 {
        if out == nil { return nil }
        return protojson.Unmarshal(body, out)
    }
    // 4xx / 5xx：尝试按 Kratos 错误结构解码
    var ke kratosError
    if json.Unmarshal(body, &ke); ke.Reason != "" {
        return &CLIError{
            Code:       ke.Reason,
            HTTPStatus: resp.StatusCode,
            Message:    ke.Message,
            Metadata:   ke.Metadata,
        }
    }
    return &CLIError{
        Code: "UNKNOWN", HTTPStatus: resp.StatusCode,
        Message: string(body),
    }
}
```

### 6.2 退出码映射

| `CLIError.Code` / 触发条件 | 退出码 |
|----------------------------|--------|
| 无错误 | 0 |
| cobra 参数错误 / `--help` 误用 | 1 |
| 后端业务 4xx（非 401/403） | 2 |
| 网络 / DNS / TLS / 5xx | 3 |
| `USER_CANCELED`（prompt 输入 n / Ctrl+C 在 prompt） | 4 |
| `SKILL_IMPORT_BLOCKED` / warn 且非交互且无 `--decision` | 5 |
| HTTP 401 / 403 | 6 |
| 执行中 Ctrl+C（signal） | 130 |

`ExitCodeOf(err)` 据 `*CLIError.Code` + `HTTPStatus` 决策。

### 6.3 错误展示 schema

见 PRD §5.3。`output.text.PrintError` 与 `output.json.PrintError` 必须各有一组 golden 测试覆盖：

- 400 / 401 / 403 / 404 / 409 / 500；
- 网络错误（`net.OpError`）；
- 自定义错误（如 `USER_CANCELED`、`SKILL_IMPORT_BLOCKED`）。

---

## 7. 安全设计

### 7.1 凭据存储

| 项 | 设计 |
|----|------|
| token 写入 | `config.Save` 写完后 `os.Chmod(path, 0o600)`；Win 跳过 chmod 仅打印 stderr 警告 |
| token 读取 | `config.Load` 先 `EnsureSecurePerm`：若 `Mode().Perm() > 0o600` 则拒绝读 `[backend].token`（仅返回非 token 字段）+ 提示 `chmod 600 <path>` |
| token 显示 | `config get backend.token` 默认 `***<last4>`；`--show-token` 才显全文并打 stderr 警告 |
| `--debug` 日志 | HTTP 请求 / 响应日志中 `Authorization: Bearer ***<last4>` |
| `--debug` body | request / response body 完整记录；建议在文档提示用户敏感场景不要开 `--debug` |
| stdout 永不打印 token | login 成功后只输出 `token saved to <path>` |

### 7.2 二次确认

| 触发场景 | 行为 |
|----------|------|
| 删除 agent / skill / 任意资源 | text 模式 prompt；json 模式必须 `--yes` 否则 exit 1 + 提示 |
| 启停高风险 tool / plugin | 同上 |
| `channel send` | 强制 `--yes`，无 prompt 路径（避免误触发外部消息） |
| 后端 `confirm_key` 字段 | 由 CLI 透传，不豁免 |

```go
// internal/cli/ui/prompt.go
func RequireConfirm(c *cli.Context, prompt string, dangerous bool) error {
    if c.AutoYes { return nil }
    if !c.UI.IsTTY {
        return &CLIError{Code: "CONFIRMATION_REQUIRED", Message: "non-interactive; use --yes to confirm"}
    }
    ok, err := c.UI.ConfirmYesNo(prompt, !dangerous)
    if err != nil { return err }
    if !ok { return &CLIError{Code: "USER_CANCELED"} }
    return nil
}
```

### 7.3 黑名单 lint

`cmd/araneactl/lint/main.go` 现有规则 R1–R11。**新增 R12**：

```
R12: cmd/aranea/** 与 internal/cli/** 不得 import:
       - aranea-agents/internal/biz/...
       - aranea-agents/internal/data/...
       - aranea-agents/internal/agent/...
       - aranea-agents/internal/server/...
       - aranea-agents/internal/service/...
       - trpc.group/trpc-go/trpc-agent-go/...
     例外：无（CLI 全程通过 client/http.go 走后端 API）。
```

实施：参考 R1 / R2 现有 AST 扫描代码模板，在同一文件添加 `checkR12()` 并加入 main 中调度。CI 失败即拦截。

---

## 8. 配置与本地存储（详细）

### 8.1 路径解析

```go
// internal/cli/config/paths.go
func DefaultPath() (string, error) {
    dir, err := os.UserConfigDir() // Linux: $XDG_CONFIG_HOME or ~/.config
                                   // mac: ~/Library/Application Support
                                   // Win: %APPDATA%
    if err != nil { return "", err }
    return filepath.Join(dir, "aranea", "config.toml"), nil
}

func CacheDir() (string, error) {
    dir, err := os.UserCacheDir() // Linux: ~/.cache  mac: ~/Library/Caches  Win: %LOCALAPPDATA%\...\Cache
    if err != nil { return "", err }
    return filepath.Join(dir, "aranea"), nil
}

func LogsDir() (string, error)   // CacheDir() + "/logs"
func TmpDir() (string, error)    // CacheDir() + "/tmp"
```

> 不使用 `$HOME` 拼路径（Win 上 `$HOME` 行为不一致）。

### 8.2 TOML 读写

依赖 `github.com/pelletier/go-toml/v2`（保留注释 + 性能）。

```go
// internal/cli/config/config.go
func Load(path string) (*CLIConfig, error)
func (c *CLIConfig) Save(path string) error
```

- `Save` 调用 `toml.Marshal` → `os.WriteFile(path, data, 0o600)` → `os.Chmod(path, 0o600)`；
- 字段缺失时填默认（不抛错）；
- 解析失败：返回错误 `CONFIG_INVALID`，提示 `aranea config path` + 备份 + 修复。

### 8.3 环境变量优先级实现

```go
func (c *CLIConfig) OverrideFromEnv() {
    if v := os.Getenv("ARANEA_BASE_URL"); v != "" { c.Backend.BaseURL = v }
    if v := os.Getenv("ARANEA_TOKEN");    v != "" { c.Backend.Token   = v }
    if v := os.Getenv("ARANEA_OUTPUT");   v != "" { c.UI.Output       = v }
    if v := os.Getenv("ARANEA_DEBUG");    v != "" { /* parse bool */ }
    if v := os.Getenv("NO_COLOR");        v != "" { c.UI.Color = "never" }
}
```

`OverrideFromFlags` 接受 cobra 已解析的 flag 值，最后调用，覆盖 env 与 file。

---

## 9. 依赖（主 `go.mod`）

| 依赖 | 阶段 | 用途 |
|------|------|------|
| `github.com/spf13/cobra` | P0 | 命令框架 |
| `github.com/spf13/pflag` | P0（cobra 间接） | POSIX flag |
| `github.com/pelletier/go-toml/v2` | P0 | config.toml 读写（保留注释） |
| `github.com/mattn/go-isatty` | P0 | TTY 检测 |
| `github.com/fatih/color` | P0 | ANSI 色（NO_COLOR 兼容） |
| `github.com/olekukonko/tablewriter` | P0 | text/table 输出 |
| `google.golang.org/protobuf` | P0（已在） | `protojson` |
| `github.com/peterh/liner` | P1 | REPL 行编辑 |
| `github.com/gorilla/websocket` | P1 | WS（与 `internal/server/ws.go` 同库） |
| `github.com/go-git/go-git/v5` | P1 | skill install clone |

**明确不引入**：`viper`、`bubbletea`、`urfave/cli`、`go-resty`、`chzyer/readline`。

---

## 10. 测试策略

| 层 | 范围 | 工具 | 纳入 `make ci` |
|----|------|------|----------------|
| 单元 | `internal/cli/config / output / client/errors / cmd/*` 的纯逻辑 | `testing` + `testify` | ✅ |
| 契约 | `internal/cli/client/*` 调后端 → `httptest.NewServer` 假后端断言请求 & 响应 | `httptest` | ✅ |
| Golden | `internal/cli/output/golden/*.golden` 文本快照 | 自写 diff 或 `goldie` | ✅ |
| Lint | R12 黑名单（`cmd/araneactl/lint`） | 自家工具 | ✅ |
| 编译验证 | `go build ./cmd/aranea/` | go | ✅（建议在 CI 矩阵加） |
| Smoke (单平台) | `make smoke-cli`：启 admin → login → agent ls → skill ls | shell + 已构建 binary | ❌ P0；P1 evaluate |
| E2E（多平台） | 跨平台二进制 + admin 后端 | 自建 | ❌ P2 |

### 10.1 httptest 模板

```go
func TestClient_ListAgents(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        assert.Equal(t, "GET", r.Method)
        assert.Equal(t, "/v1/agents", r.URL.Path)
        assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"items":[{"id":"a1","display_name":"A"}],"total":1}`))
    }))
    defer srv.Close()

    c := &client.Client{Base: srv.URL, Token: "test-token", Doer: http.DefaultClient}
    resp, err := c.ListAgents(context.Background(), &agentv1.ListAgentsRequest{})
    require.NoError(t, err)
    assert.Equal(t, int32(1), resp.Total)
}
```

### 10.2 Golden 测试目录

```
internal/cli/output/golden/
├── agent_ls_text_tty.golden
├── agent_ls_text_pipe.golden
├── agent_ls_json.golden
├── error_skill_blocked_text.golden
└── error_skill_blocked_json.golden
```

环境变量 `UPDATE_GOLDEN=1` 用于刷新 golden。

---

## 11. 构建与发布

### 11.1 Makefile 新增 target

```makefile
.PHONY: cli
cli:
	go build -ldflags "-X main.Version=$(VERSION) -X main.Commit=$$(git rev-parse --short HEAD)" \
	    -o ./bin/aranea ./cmd/aranea/

.PHONY: cli-all
cli-all:
	@for os in windows darwin linux; do \
	  for arch in amd64 arm64; do \
	    ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
	    GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
	      go build -ldflags "-X main.Version=$(VERSION)" \
	      -o ./bin/aranea-$$os-$$arch$$ext ./cmd/aranea/; \
	  done; \
	done

.PHONY: smoke-cli      # P1 引入
smoke-cli: cli
	./scripts/smoke-cli.sh
```

`make ci` **不**强制 `make cli`；改在 CI 矩阵单独跑 `go build ./cmd/aranea/` 保证编译不破。

### 11.2 ldflags 注入

`cmd/aranea/main.go` 必须有 `var Version, Commit, BuildTime string`，Makefile 用 `-X main.Version=...` 注入。

---

## 12. 与既有方案的差异回顾（实施前 review 用）

| 项 | 原 `25 cli.design.md` v2.0 | 本设计 v3.0 |
|----|----------------------------|-------------|
| CLI 业务代码位置 | `cmd/aranea/{apiclient,output,config,repl,agent,...}` | `internal/cli/{client,output,config,repl,cmd/...}`；`cmd/aranea/` 仅入口 |
| CLI 进程内 Runner | 在 main 里手动组装 `BuildTRPCLLMAgent` + `NewTRPCRunner` + `dbAgentLoader` + `SQLiteSessionService` | **删除**；CLI 只是 WS 客户端 |
| CLI 依赖 | `internal/agent`、`internal/biz`、trpc-agent-go | 仅 `api/kratos/*/v1` + `pkg/auth` 常量 + `pkg/safego` |
| 请求 / 响应类型 | 手抄 Go struct | 直接复用 pb 生成代码 + `protojson` |
| 二次确认 | 新建 `ConfirmPlugin` BeforeTool 钩子 | 复用 `chat.proto` 既有 `AwaitUserReply` |
| Skill import 来源字段 | 改 proto 添加字段 | 改 multipart form 解析，写入 `metadata_json` |
| Wire 注入 | CLI 手动组装；触及 `internal/data` `NewData` | 不需要 wire；CLI 没有数据层依赖 |
| 路径前缀 | `/api/v1/*` | `/v1/*`（对齐 proto 注解） |
| `workspace_id` | 全局 flag + config 生效 | 仅 config 占位字段，本期无效 |
| 输出格式 | MVP 4 种 | MVP 2 种（text/json）；yaml/table P1 |

---

## 13. 红线再确认（实施 PR 必查）

- [ ] `cmd/aranea/**` 与 `internal/cli/**` **不** import `internal/biz`、`internal/data`、`internal/agent`、`internal/server`、`internal/service`、`pkg/trpc-agent-go`；
- [ ] CLI **不**直连 SQLite / Postgres；所有数据走 HTTP / WS；
- [ ] CLI **不**在进程内构建 `trpcrunner.Runner` 或 `trpcagent.Agent`；
- [ ] 系统管家 Agent 种子、`cli_admin_*` 工具集**只**在后端（`internal/data` / `internal/tools/cli_admin`）；CLI 不参与 seed；
- [ ] Runner 装配仍只在 `internal/service`；`cli_admin_*` 工具组装通过 service 注入到 system_admin Agent；
- [ ] `internal/server/**` 不新增对 `pkg/trpc-agent-go` 的直接依赖（包括为 WS envelope 新增子类型，仍走 `internal/service/chat_wire.go`）；
- [ ] 所有 HTTP 路径以 `/v1/*` 为准；
- [ ] Token 文件权限 0600；CLI 不打印 token 明文；
- [ ] 高风险写操作无 `--yes` 时拒绝；`--yes`/`/yes` 仅当前进程会话有效。

---

## 14. 实施代码骨架（参考）

> 仅为接口形状，不替代实现。详细任务粒度见开发计划。

### 14.1 `internal/cli/execute.go`

```go
package cli

func Execute(ctx context.Context, bi BuildInfo) error {
    root := NewRoot(ctx, bi)
    return root.ExecuteContext(ctx)
}

func NewRoot(ctx context.Context, bi BuildInfo) *cobra.Command {
    var (
        cfgPath, baseURL, token, output string
        quiet, debug, autoYes, noColor  bool
    )
    root := &cobra.Command{
        Use:           "aranea",
        Short:         "Aranea 终端控制台",
        Version:       bi.Version,
        SilenceUsage:  true,
        SilenceErrors: true,
        PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
            cfg, err := config.Load(cfgPath)
            if err != nil { return err }
            cfg.OverrideFromEnv()
            cfg.OverrideFromFlags(baseURL, token, output, noColor)

            cc := newContext(ctx, cfg, ContextOpts{
                Debug: debug, Quiet: quiet, AutoYes: autoYes, BuildInfo: bi,
            })
            cmd.SetContext(WithCLI(cmd.Context(), cc))
            return nil
        },
    }
    root.PersistentFlags().StringVar(&cfgPath, "config", "", "config file path")
    root.PersistentFlags().StringVar(&baseURL, "base-url", "", "override [backend].base_url")
    root.PersistentFlags().StringVar(&token,   "token",    "", "override [backend].token")
    root.PersistentFlags().StringVarP(&output, "output",   "o", "", "output: text | json")
    root.PersistentFlags().BoolVarP(&quiet,    "quiet",    "q", false, "minimal output")
    root.PersistentFlags().BoolVarP(&autoYes,  "yes",      "y", false, "skip confirmations")
    root.PersistentFlags().BoolVar(&debug,     "debug",    false, "log HTTP requests/responses to stderr")
    root.PersistentFlags().BoolVar(&noColor,   "no-color", false, "disable ANSI colors")

    root.AddCommand(
        cmdpkg.NewVersionCmd(bi),
        cmdpkg.NewLoginCmd(),
        cmdpkg.NewConfigCmd(),
        cmdpkg.NewSystemCmd(),
        cmdpkg.NewAgentCmd(),
        cmdpkg.NewSkillCmd(),
        cmdpkg.NewToolCmd(),
        // P1: NewChatCmd / NewTeamCmd / ...
    )
    return root
}
```

### 14.2 `internal/cli/client/http.go`

```go
package client

func (c *Client) Do(ctx context.Context, method, path string, body, out proto.Message) error {
    var reqBody io.Reader
    if body != nil {
        b, err := protojson.Marshal(body)
        if err != nil { return err }
        reqBody = bytes.NewReader(b)
    }
    req, err := http.NewRequestWithContext(ctx, method, c.Base+path, reqBody)
    if err != nil { return err }
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Accept", "application/json")
    req.Header.Set("User-Agent", c.UA)
    if c.Token != "" { req.Header.Set("Authorization", "Bearer "+c.Token) }

    if c.Debug { logRequest(c.LogFunc, req) }

    resp, err := c.doWithRetry(req, method)
    if err != nil { return wrapNetErr(err) }
    defer resp.Body.Close()

    if c.Debug { logResponse(c.LogFunc, resp) }

    return decode(resp, out)
}
```

### 14.3 `internal/cli/cmd/agent.go`

```go
package cmdpkg

func NewAgentCmd() *cobra.Command {
    c := &cobra.Command{Use: "agent", Short: "Agent 管理"}
    c.AddCommand(
        agentLs(),
        agentGet(),
        agentCreate(),
        agentUpdate(),
        agentDelete(),
        agentEnable(),
        agentDisable(),
        agentTools(),
        agentToolsSet(),
    )
    return c
}

func agentLs() *cobra.Command {
    var page, pageSize int32
    var search string
    cmd := &cobra.Command{
        Use:  "ls",
        RunE: func(cmd *cobra.Command, _ []string) error {
            cc := cli.CLIFrom(cmd.Context())
            req := &agentv1.ListAgentsRequest{Page: page, PageSize: pageSize, Search: search}
            resp, err := cc.Client.ListAgents(cmd.Context(), req)
            if err != nil { return err }
            return cc.Printer.PrintList(resp.Items, int(resp.Total))
        },
    }
    cmd.Flags().Int32Var(&page, "page", 1, "page index")
    cmd.Flags().Int32Var(&pageSize, "page-size", 20, "page size")
    cmd.Flags().StringVar(&search, "search", "", "search keyword")
    return cmd
}
```

### 14.4 `internal/cli/cmd/login.go`

```go
package cmdpkg

func NewLoginCmd() *cobra.Command {
    var user, password string
    cmd := &cobra.Command{
        Use:   "login",
        Short: "登录并写 token 到本地",
        RunE: func(cmd *cobra.Command, _ []string) error {
            cc := cli.CLIFrom(cmd.Context())
            req := &adminv1.LoginRequest{Password: password}
            req.Identity = &adminv1.LoginRequest_Username{Username: user}
            admin, token, err := cc.Client.Login(cmd.Context(), req) // 见 client/admin.go: 响应可能在 body 或 Set-Cookie
            if err != nil { return err }
            cc.Cfg.Backend.Token = token
            if err := cc.Cfg.Save(""); err != nil { return err }
            return cc.Printer.PrintSuccess("token saved", "user", admin.Name, "path", config.MustPath())
        },
    }
    cmd.Flags().StringVar(&user,     "user",     "", "username")
    cmd.Flags().StringVar(&password, "password", "", "password")
    _ = cmd.MarkFlagRequired("user")
    _ = cmd.MarkFlagRequired("password")
    return cmd
}
```

---

## 15. 开放假设（实施时必须验证）

> 这些假设在文档定稿时无法不读源码完全核验；部分已在实施中验证。

1. **A1：`/v1/admins/login` 的 token 返回方式**。当前 `pkg/auth/middleware.go::noAuthPaths` 已包含 `/v1/admins/login`，但 token 是 body 字段、cookie，还是 header，需读 `internal/service/admin.go` 与 `pkg/auth/cookie.go` 后决定 CLI 怎样取 token；
2. **A2：WS token 携带方式**。`internal/server/` 下无 `ws.go`，WebSocket 实现位置需确认；token 读取位置（query / Sec-WebSocket-Protocol / cookie）必须在 CLI WS 客户端实现前确认；
3. **A3：WS 下行事件 type 取值**。`internal/event/contract/envelope.go` 已定义 `EnvelopeTypeToolCall` / `EnvelopeTypeToolResult`，但 `tool.error` 子类型缺失（使用通用 `error` 类型）；`message.delta` 的实际值待对照；
4. **A4：Skill import multipart 字段命名空间**。`internal/service/skill_import.go` 当前接收哪些 form 字段，需在 CLI-25 之前盘点；
5. **A5：Agent enable/disable RPC**。`api/kratos/agent/v1/agent.proto` 是否暴露独立 enable/disable RPC，还是只能走 `UpdateAgent + FieldMask`；CLI 实现按 proto 真相为准；
6. **A6：是否已有 R1～R11 之外的 lint 规则**。`cmd/araneactl/` 目录当前不存在，R12 lint 规则尚未实现；
7. **A7：`SystemInfoResponse` 字段补齐**。当前服务端缺少 `system_admin_agent_id` / `system_admin_agent_key` / `skill_max_zip_mb` 字段，需决定是补齐还是 CLI 端适配。

---

*文档版本：3.1 — 2026-06-06；与 [`25-cli.md`](./25-cli.md) / [`25-cli.development.md`](./25-cli.development.md) 同步。*
