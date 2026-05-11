# Aranea CLI（终端控制台 + Agent 对话操作）

本文档定义 **Aranea CLI** 的产品需求、命令体系、对话式系统管家、内置管理工具集、与后端 REST API 的契约，以及配置 / 会话 / 审计 / 输出的落地方案。

CLI 的目标是把现有 Web 控制台（`/skills`、`/agents`、`/teams`、`/tools`、`/plugins`、`/mcp-servers`、`/cron`、`/channels`、`/monitor` 等）所有可执行能力，**都搬到终端**：既支持 `aranea skill install <url>` 这样的脚本化命令，也支持 `aranea` 进入交互式对话，由内置「系统管家 Agent」用自然语言完成跨模块操作。

典型场景：

> 用户在终端输入：
>
> ```
> $ aranea
> aranea> 帮我把 https://github.com/anthropic/skills/tree/main/figma-code-connect 这个 skill 装上
> ```
>
> 系统管家 Agent 调用 `skill_install_from_url` 工具：拉取仓库 → 定位 `SKILL.md` → 本地校验编写规范 → 打包 zip → 调用 `POST /api/v1/skills/import` → 轮询 `job_id` → 与用户确认冲突组（无冲突直接 apply，有相似冲突询问保留 / 跳过 / 炼化）→ apply 入库 → 反馈安装结果与新 Skill ID。

---

## 0. 需求结论

### 0.1 本期范围

| 模块 | 本期是否做 | 说明 |
|------|------------|------|
| CLI 二进制 `aranea` | 是 | 单一可执行文件，跨平台（Windows / macOS / Linux），由后端 Go 项目同源发布 |
| 直接命令模式 | 是 | `aranea <资源> <动作> [参数]`，覆盖 Skill / Agent / Team / Tool / Plugin / MCP / Cron / Channel / Session / Monitor / System |
| 对话模式（REPL） | 是 | `aranea` 或 `aranea chat` 进入；与内置「系统管家 Agent」对话，自然语言驱动管理工具 |
| 系统管家 Agent | 是 | 内置 Agent（`agent_key = __system_admin__`），固定 Profile、固定 Tool 集、不在普通列表展示 |
| 安装 Skill from URL | 是 | 支持 `github.com / gitlab.com / git+https / ssh / 子目录 / 原始 zip URL`，本地完成 clone + 子目录定位 + 打包 + 上传 + 轮询 + 冲突处理 |
| Agent 对话执行所有管理操作 | 是 | 通过新增 `cli_admin_*` 内置 Tool 集；所有动作走后端 REST API，CLI 不直接写数据库 |
| 配置 / 会话 / 历史本地存储 | 是 | `~/.aranea/`（Windows: `%APPDATA%\aranea\`），含 `config.toml`、`sessions/*.jsonl`、`logs/cli-*.log` |
| 输出格式切换 | 是 | 默认人类可读（带色 + 表格）；`--json` 输出结构化 JSON；`--quiet` 仅返回关键字段 |
| 远程后端 / 本地内嵌后端 | 是 | 默认连接 `http://127.0.0.1:8080`；可 `aranea --base-url https://aranea.example.com` 远程接入 |
| 自动补全 | 是 | 生成 PowerShell / Bash / Zsh / Fish 补全脚本 |
| 升级与版本检测 | 是 | `aranea version` / `aranea upgrade`（仅打印升级指引，不强制自更新） |
| Web 控制台同源登录 | 后续 | 本期默认单用户本地控制台，无需登录；远程后端接入预留 `aranea login` |
| 插件式第三方 CLI 命令 | 后续 | 不允许 CLI 加载任意外部代码 |

### 0.2 默认产品决策

| 决策项 | 默认值 |
|--------|--------|
| 二进制名 | `aranea`；备选短别名 `arn`（仅 PATH 别名，不重复实现） |
| 默认后端地址 | `http://127.0.0.1:8080`，可被环境变量 `ARANEA_BASE_URL` 覆盖，也可被 `--base-url` 覆盖 |
| 配置文件路径 | `$HOME/.aranea/config.toml`（Windows：`%APPDATA%\aranea\config.toml`） |
| 会话历史 | `$HOME/.aranea/sessions/<session_id>.jsonl`，与 Web 控制台 `sessions` 表关联 |
| 默认对话 Agent | `__system_admin__` 系统管家；可通过 `aranea chat --agent <key>` 切换为任意已存在 Agent |
| 默认 Team | 不预设；`aranea chat --team <key>` 可切换 |
| 输出主题 | 自动检测终端是否支持 ANSI 色；不支持时回退纯文本；可被 `NO_COLOR=1` 强制关闭 |
| 输出格式 | `text`（默认）/ `json` / `yaml` / `table`；通过 `--output` 切换 |
| 长任务进度 | SSE 拉流时使用 `QSpinner` 类终端进度条；失败可继续追加日志 |
| 危险操作 | 高风险动作（`shell_exec`、`channel send`、删除资源、装入未签名 Skill 等）默认要求 `--yes` 或对话二次确认 |
| 系统管家 Agent 的工具集 | 见 §6；**不开放**普通业务工具（如 `web_search`、`tts`），只开放 `cli_admin_*` 管理工具 |
| 自动补全脚本 | `aranea completion <shell>` 输出补全脚本到 stdout |
| Telemetry | 默认关闭；可在 `config.toml` 开启匿名错误上报，对齐 `24 telemetry.md` |

### 0.3 角色与权限

当前产品为单用户本地控制台，CLI 默认拥有与 Web 控制台一致的全部能力。安全边界由后端按以下规则保证：

| 风险 | 控制方式 |
|------|----------|
| 删除资源 | CLI 必须显式 `--yes` 或对话内输入资源名；后端二次校验 |
| 启停高风险 Tool / Plugin | CLI 与对话模式都要求二次确认；调用 `PATCH /api/v1/tools/:id/enabled` 时携带 `confirm_key` |
| 写入文件 / 执行命令 | `cli_admin_*` 工具集**不**包含通用 `write_file` / `shell_exec`；如需写入由用户在 Web 端操作 |
| 安装 Skill | 默认进入 §5 的安全检查流程；未通过结构校验或名称重复时阻塞 |
| 远程后端接入 | 通过 `aranea login` 写入 token；token 仅落本地 `config.toml`，权限按目标后端策略 |
| 审计 | CLI 每次调用 admin tool 都通过后端走标准 `tool_invocations` 记录，UI 上可见来源 `source=cli` |

---

## 1. 命令模型与 Launcher 架构（对齐 **`pkg/trpc-agent-go` / tRPC-Agent-Go**；双层结构可参考 google/adk-go）

Aranea 的运行时与 Agent 框架以 **`pkg/trpc-agent-go`（tRPC-Agent-Go）** 为真相源。[google/adk-go](https://github.com/google/adk-go) 官方 CLI（`adkgo` + `cmd/launcher`）提供了一套**工程上可借鉴的双层结构**表格与接口形态；**本节在流程上对齐其模式，具体 import / 包路径以实现仓库根 `go.mod` 与 `pkg/trpc-agent-go` 为准**。

### 1.1 参考：google/adk-go 官方 CLI 双层结构（历史对照）

google/adk-go 仓库的 CLI 由两套互补的入口组成：

| 入口 | 实现位置 | 用途 | 命令风格 |
|------|----------|------|----------|
| `adkgo` | `adk-go/cmd/adkgo/`、子包 `internal/deploy/{cloudrun,agentengine}` | **管理 / 部署 / 工程化命令**：构建镜像、部署 Cloud Run、Agent Engine 等 | **Cobra**：`adkgo deploy cloudrun --region ... --service_name ...` |
| `adk` 运行时 | `adk-go/cmd/internal/adkcli/main.go` + `adk-go/cmd/launcher/` | **在工作目录内发现 `root_agent.yaml` → 加载 Agent → 运行**（console / web / api / webui / a2a / pubsub / eventarc） | **Launcher / SubLauncher** + stdlib `flag.FlagSet`：`adk console`、`adk web webui api`、`adk web api a2a pubsub` |

关键接口（`cmd/launcher/launcher.go`）：

```go
type Launcher interface {
    Execute(ctx context.Context, config *Config, args []string) error
    CommandLineSyntax() string
}

type SubLauncher interface {
    Keyword() string                                       // "console" / "web" / ...
    Parse(args []string) ([]string, error)                 // 返回未消费的 args
    CommandLineSyntax() string
    SimpleDescription() string
    Run(ctx context.Context, config *Config) error
}

type Config struct {
    SessionService   session.Service
    ArtifactService  artifact.Service
    MemoryService    memory.Service
    AgentLoader      agent.Loader
    A2AOptions       []a2asrv.RequestHandlerOption
    PluginConfig     runner.PluginConfig
    TelemetryOptions []telemetry.Option
}
```

组合方式：`universal.NewLauncher(console, web)` 与 `web.NewLauncher(webui, api, a2a, pubsub, eventarc)` 是**两层 Keyword 路由**。预设 `full.NewLauncher()`（`cmd/launcher/full/full.go`）一次性把 console + web + 全部 sublauncher 组合起来，作为开发者最常用的入口。

ADK Console Launcher 的核心循环（`cmd/launcher/console/console.go`）：

```go
r, _ := runner.New(runner.Config{
    AppName:        "console_app",
    Agent:          rootAgent,
    SessionService: sessionService,
    PluginConfig:   config.PluginConfig,
})
for {
    userInput := <-inputChan
    userMsg := genai.NewContentFromText(userInput, genai.RoleUser)
    fmt.Print("\nAgent -> ")
    for event, err := range r.Run(ctx, userID, sessionID, userMsg, agent.RunConfig{
        StreamingMode: streamingMode, // SSE 时按 token 增量打印
    }) {
        // 打印 event.LLMResponse.Content.Parts[*].Text
    }
    fmt.Print("\nUser -> ")
}
```

### 1.2 Aranea CLI 对应映射

Aranea 的 CLI 也分两层，二进制名都是 `aranea`：

| Aranea | 对应参考实现 | 实现位置 | 框架 |
|--------|--------------|----------|------|
| `aranea <资源> <动作>`：管理类命令（agent / skill / tool / plugin / mcp / cron / channel / monitor / system / config / completion / login / version） | `adkgo` Cobra 命令树 | `aranea/backend/cmd/aranea/cli/<feature>/` + `cmd/aranea/cli/root.go`（`package cli`） | **Cobra** |
| `aranea console / web / api / webui / full`：运行时 SubLauncher | `adk` 运行时 launcher（对照） | `aranea/backend/cmd/aranea/launcher/{console,web,api,webui,full}/...` | **复用框架 `cmd/launcher` 接口**（路径以 **`pkg/trpc-agent-go`/go.mod** 为准，迁移期可能仍为 `google.golang.org/adk/cmd/launcher`）+ stdlib `flag.FlagSet` |
| `aranea`（无参数） | `adk`（无参数 → `console`） | `cmd/aranea/main.go` 走 `full.NewLauncher().Execute(...)`，默认 sublauncher 是 `console` | 同上 |

设计原则：

1. **Launcher / SubLauncher 接口**：直接 import **框架**提供的 `cmd/launcher` 类型（**具体 module path 以 `go.mod` 为准**），不在 Aranea 重新定义；与官方 Console Launcher 组合方式兼容的子 Launcher 仍可复用。
2. **`Config` 结构体扩展而非替换**：`aranea/launcher.Config` 内嵌框架的 `launcher.Config`，再追加 Aranea 特有依赖（见 §1.5）。
3. **管理 CLI 用 Cobra**，因为子命令多、有 `init()` 自注册需求；写法可与 google/adk-go 的 `cmd/adkgo/internal/deploy/cloudrun/cloudrun.go` 对照。
4. **运行时 SubLauncher 用 stdlib `flag.FlagSet`**，与参考实现 `console.go` / `web.go` / `api.go` 相同的风格；统一通过 `internal/cli/util.FormatFlagUsage(fs)` 渲染帮助文本。
5. **共享 `Config` DI 容器**：所有 SubLauncher 不直接 import 业务包，而是从注入的 `Config` 拿 `AgentLoader` / `SessionService` / `PluginConfig`，便于测试与替换实现。

### 1.3 Go 包结构

```text
aranea/backend/
├── cmd/
│   └── aranea/
│       ├── main.go                         # 入口；launcher 关键字走 ADK 链，否则 Cobra
│       ├── cli/                            # 管理子命令（仅 HTTP 客户端）
│       │   ├── root.go                     # `package cli`；cobra Command "aranea"
│       │   ├── apiclient/  output/  config/  login/  completion/  version/
│       │   ├── agent/  skill/  tool/  plugin/  mcp/  cron/  channel/  monitor/  session/  system/
│       │   └── ...
│       ├── launcher/                       # 与 ADK 一致的 SubLauncher
│       │   ├── config.go
│       │   ├── console/console.go
│       │   ├── web/web.go
│       │   └── full/full.go
│       └── ...
├── internal/                               # 服务层、仓储层
└── ...
```

`main.go` 入口示例（参考 `adk-go/cmd/internal/adkcli/main.go` + `cmd/adkgo/adkgo.go` 两种写法的合并）：

```go
package main

import (
    "context"
    "log"
    "os"

    "google.golang.org/adk/runner"

    "arenea/backend/cmd/aranea/cli"
    "arenea/backend/cmd/aranea/launcher/full"
)

func main() {
    args := os.Args[1:]

    // 路由策略：第一个参数若是 launcher 关键字（console/web/api/webui/full），走 ADK Launcher。
    // 否则走 Cobra（管理命令、help、version 等）。
    if isLauncherKeyword(args) {
        ctx := context.Background()
        config := buildLauncherConfig(ctx) // §1.5
        l := full.NewLauncher(config)      // 内含 console + web(webui+api+a2a+pubsub)
        if err := l.Execute(ctx, config.ADK(), args); err != nil {
            log.Fatalf("run failed: %v\n\n%s", err, l.CommandLineSyntax())
        }
        return
    }
    // 无参数 = 默认进 console launcher
    if len(args) == 0 {
        ctx := context.Background()
        config := buildLauncherConfig(ctx)
        if err := full.NewLauncher(config).Execute(ctx, config.ADK(), []string{"console"}); err != nil {
            log.Fatalf("console failed: %v", err)
        }
        return
    }
    cli.Execute()
}
```

### 1.4 直接复用框架（**`pkg/trpc-agent-go`/go.mod**）的接口与帮助函数

下表所列 **`google.golang.org/adk/...`** 为**迁移期**常见示例路径；**规范以根 `go.mod` 与仓库内 [`pkg/trpc-agent-go`](../../pkg/trpc-agent-go)（或其所 re-export / replace 的路径）为准**。

| 框架包（示例 import） | Aranea 用法 |
|--------|-------------|
| `google.golang.org/adk/cmd/launcher` | 直接 import `Launcher` / `SubLauncher` / `Config`；不重新定义 |
| `google.golang.org/adk/cmd/launcher/universal` | 用 `universal.NewLauncher(...)` 串联 console + web；用 `universal.ErrorOnUnparsedArgs` 校验未消费参数 |
| `google.golang.org/adk/cmd/launcher/console` | console SubLauncher 既可直接复用，也可在 Aranea 里继承 / 包一层加入「斜杠命令」「会话本地存储」等增量特性（§4） |
| `google.golang.org/adk/cmd/launcher/web` | web SubLauncher 直接复用；Aranea 自己的 sublauncher（如 `aranea-api` 的 Aranea REST、Aranea 静态 WebUI）实现 `web.Sublauncher` 接口注入 |
| `google.golang.org/adk/cmd/launcher/web/webui` & `web/api`（即 `adkrest`） | Aranea 的 chat / sessions 路由若想与 ADK 保持兼容，可叠加 ADK `adkrest` Server，挂在 `/adk-api` 子路径上；Aranea 自有业务 API 仍在 `/api/v1/*` |
| `google.golang.org/adk/runner` | `runner.New(runner.Config{...})` + `r.Run(ctx, userID, sessionID, userMsg, agent.RunConfig{StreamingMode: SSE})` 迭代器，**在 console 与对话模式中按 ADK 原样使用**（§4.1） |
| `google.golang.org/adk/agent` | `agent.Loader`、`NewSingleLoader` / `NewMultiLoader`；Aranea 实现自己的 `dbAgentLoader`（见 §6.x） |
| `google.golang.org/adk/internal/cli/util` | `FormatFlagUsage(fs)` 渲染 flag 帮助；ANSI 调色板 `Reset/Red/Green/Yellow/...`；`LogStartStop(msg, fn)` 用「Starting / Finished successfully」风格输出长任务 |
| `google.golang.org/adk/telemetry` | OTel 初始化与 `--otel_to_cloud` 标志保留同名 |

### 1.5 Aranea 的 Launcher Config

`aranea/backend/cmd/aranea/launcher/config.go`：

```go
package launcher

import (
    adklauncher "google.golang.org/adk/cmd/launcher"
    "google.golang.org/adk/agent"
    "google.golang.org/adk/runner"

    "arenea/backend/internal/service"
)

// Config 内嵌 ADK launcher.Config，并附加 Aranea 业务服务。
// 任何 Aranea SubLauncher 都通过 Config 取到全部依赖；
// 任何 ADK 原生 SubLauncher 通过 Config.ADK() 拿到的就是 ADK 原生结构。
type Config struct {
    // === ADK 原生字段 ===
    Adk adklauncher.Config

    // === Aranea 业务服务（依赖注入）===
    Agents   *service.AgentService
    Teams    *service.TeamService
    Sessions *service.SessionService
    Chat     *service.ChatService
    Skills   *service.SkillService
    Tools    *service.ToolService
    Plugins  *service.PluginService
    Channels *service.ChannelService
    Cron     *service.CronService
    Audit    *service.AuditService

    // === CLI 运行态 ===
    BaseURL  string // 远程后端 URL（远程模式）
    Token    string
    Embedded bool   // true=进程内嵌后端（无需 BaseURL），false=远程后端
}

func (c *Config) ADK() *adklauncher.Config { return &c.Adk }

// AgentLoader 构造一个把 Aranea 数据库内 Agent 暴露给 ADK 的 Loader。
// 系统管家作为 RootAgent；其他 Agent 通过 LoadAgent("<agent_key>") 取出。
func (c *Config) BuildAgentLoader(systemAdmin agent.Agent, others ...agent.Agent) (agent.Loader, error) {
    return agent.NewMultiLoader(systemAdmin, others...)
}

// PluginConfig 复用现有 Aranea Plugin 列表，确保 console / web 两条路径
// 都走同一份 BeforeTool / AfterTool / OnToolError 审计。
func (c *Config) PluginConfig() runner.PluginConfig {
    return c.Adk.PluginConfig
}
```

`buildLauncherConfig(ctx)`（在 `main.go` 内实现）负责：打开 SQLite、初始化各 service、装配 plugin、用 `dbAgentLoader` 实例化 `agent.Loader`、把 ADK `session.Service` 桥接到 Aranea 的 `sessions` 表（也可直接复用 `session.InMemoryService()` 作为开发降级）。

### 1.6 与 §2 / §4 / §6 的映射

| 本文后续章节 | 对应 ADK 实体 |
|--------------|---------------|
| §2 直接命令模式（Cobra 命令树） | `cmd/adkgo/internal/<feature>/...`；`init()` 自注册到 `root.RootCmd` |
| §4 对话模式（REPL） | `cmd/launcher/console/console.go`：`runner.Runner.Run` 迭代器 + `os.Stdin` 行读 + `agent.RunConfig{StreamingMode: SSE}` |
| §6 系统管家 Agent / `cli_admin_*` 工具集 | `agent.Agent` 实例 + `tool.Tool` 集合，由 `agent.Loader` 暴露给 console launcher |
| §11 实施拆分建议 | Phase 1 拆 Cobra 子包；Phase 2 拆 Launcher 子包；Phase 3 加 ADK plugin / tool 注入 |

> 参考：`adk-go/cmd/launcher/console/console.go`、`adk-go/cmd/launcher/web/web.go`、`adk-go/cmd/launcher/full/full.go`、`adk-go/cmd/adkgo/internal/root/root.go`、`adk-go/cmd/adkgo/internal/deploy/cloudrun/cloudrun.go`、`adk-go/agent/loader.go`、`adk-go/internal/cli/util/oscmd.go`。

---

## 2. 信息架构与命令体系

### 2.1 顶层入口

Aranea 的入口策略与 ADK 的 `full.NewLauncher()` 一致：**第一个 token 既可能是 Launcher 关键字，也可能是 Cobra 子命令；都不是时回退到默认 Launcher**（即 `console`）。

| 入口 | 类型 | 作用 |
|------|------|------|
| `aranea` | Launcher（默认 → `console`） | 与 ADK `adk` 行为一致：直接进入对话模式 REPL |
| `aranea console [flags]` | ADK Launcher 关键字 | 进入对话模式；与 ADK `adk console` 同源 |
| `aranea web [webui] [api] [...]` | ADK Launcher 关键字 | 启动 Aranea 后端 HTTP 服务（Aranea WebUI + REST API + ADK API 可叠加） |
| `aranea api` / `aranea webui` | ADK Launcher 关键字（单独使用） | 仅启动其中一个服务，便于 K8s sidecar / 反向代理布署 |
| `aranea full` | ADK Launcher 关键字 | 一次性启动 console + web 全套 sublauncher（开发模式默认） |
| `aranea chat [--session <id>] [--agent <key>] [--team <key>]` | Cobra 别名 | 等价于 `aranea console`，提供更显眼的 UX |
| `aranea <资源> <动作> [...]` | Cobra 命令 | 直接命令模式（脚本 / 自动化），见 §2.2 / §3 |
| `aranea --help` / `aranea <资源> --help` | Cobra | 帮助；Launcher 关键字的帮助由 `Launcher.CommandLineSyntax()` 渲染 |
| `aranea version` | Cobra | 打印 CLI 版本、git commit、后端可达性、ADK runtime 版本 |
| `aranea config <get/set/edit/path>` | Cobra | 管理本地配置 |
| `aranea login [--base-url ...]` | Cobra | 远程后端接入（本期可视为占位，本地后端无需登录） |
| `aranea completion <bash/zsh/fish/powershell>` | Cobra（`cobra.Command.GenBashCompletion` 等内置） | 输出补全脚本 |

### 2.2 资源 / 动作命令树

约定：所有动作动词与后端 REST 一一对应；命令树由 `cmd/aranea/cli/root.go` 显式 `AddCommand`（与 `adkgo deploy cloudrun` 写法一致）。

| 资源 | 子命令（动作） | 对应后端 |
|------|----------------|----------|
| `agent` | `ls`、`get`、`create`、`update`、`delete`、`enable`、`disable`、`run`、`tools`、`tools-set` | `/api/v1/agents`、`/api/v1/agents/:id/tools/effective`、`/api/v1/agents/:id/tools/policy` |
| `team` | `ls`、`get`、`create`、`update`、`delete`、`run`、`runs`、`run-events` | `/api/v1/teams`、`/api/v1/team-runs`、`/api/v1/team-run-events` |
| `skill` | `ls`、`get`、`enable`、`disable`、`delete`、`duplicate`、`import <zip>`、`install <url>`、`refine`、`runs`、`publish` | `/api/v1/skills`、`/api/v1/skills/import`、`/api/v1/skills/import/:job_id`、`/api/v1/skills/import/:job_id/apply`、`/api/v1/skill-runs` |
| `tool` | `ls`、`get`、`enable`、`disable`、`config`、`runs`、`bind <agent_key>` | `/api/v1/tools`、`/api/v1/tools/:id/enabled`、`/api/v1/tools/:id/config`、`/api/v1/tools/runs` |
| `plugin` | `ls`、`get`、`enable`、`disable`、`order`、`config`、`runs` | `/api/v1/plugins`、`/api/v1/plugins/:id` |
| `mcp` | `ls`、`add`、`update`、`delete`、`test` | `/api/v1/mcp-servers`、`/api/v1/mcp-servers/:id/test` |
| `cron` | `ls`、`add`、`update`、`delete`、`pause`、`resume`、`trigger`、`runs` | `/api/v1/cron-tasks`、`/api/v1/cron-tasks/:id/trigger`、`/api/v1/cron-task-runs` |
| `channel` | `ls`、`add`、`update`、`delete`、`enable`、`disable`、`test`、`send` | `/api/v1/channels`、`/api/v1/channels/:id/test` |
| `provider` | `ls`、`add`、`update`、`delete`、`inspect` | `/api/v1/llm-provider-models`、`/api/v1/llm-provider-models/inspect` |
| `category` | `ls`、`tree`、`add`、`update`、`delete` | `/api/v1/agent-categories`、`/api/v1/agent-categories/tree` |
| `hook` | `ls`、`add`、`update`、`delete` | `/api/v1/hooks` |
| `session` | `ls`、`get`、`new`、`delete`、`messages`、`send <text>`、`stream` | `/api/v1/sessions`、`/api/v1/chat/messages`、`/api/v1/chat/messages/stream` |
| `monitor` | `audit`、`events`、`traces`、`usage`、`logs`、`tail` | `/api/v1/monitor/audit`、`/api/v1/monitor/events`、`/api/v1/monitor/traces`、`/api/v1/model-usage/*`、`/api/v1/monitor/logs/stream` |
| `system` | `health`、`info`、`config-show`、`backup`、`restore`、`migrate`、`reset` | `/healthz`、`/api/v1/system/*`（如不存在由后端补） |

### 2.3 通用全局选项

| 选项 | 含义 |
|------|------|
| `--base-url <url>` | 覆盖后端地址 |
| `--token <token>` | 覆盖配置文件中的 token |
| `--workspace <id>` | 选择工作区（多租户预留） |
| `--output text\|json\|yaml\|table` | 输出格式 |
| `--quiet` / `-q` | 只输出关键字段（适合 `xargs` / 管道） |
| `--no-color` | 关闭 ANSI 色 |
| `--yes` / `-y` | 跳过所有交互式确认（高风险操作仍可被后端拒绝） |
| `--dry-run` | 仅打印将要发送的 HTTP 请求与请求体，不实际调用 |
| `--timeout <duration>` | 单次请求超时（默认 30s） |
| `--debug` | 输出 HTTP 请求 / 响应、SSE 事件、内部状态机 |

### 2.4 退出码

| 退出码 | 含义 |
|--------|------|
| `0` | 成功 |
| `1` | 一般错误（参数 / 网络 / 后端 4xx 业务错误） |
| `2` | 用法错误（命令不存在 / 参数不匹配） |
| `3` | 后端 5xx |
| `4` | 取消（用户在确认中拒绝） |
| `5` | 校验失败（结构 / schema / 冲突 block） |
| `6` | 超时 |
| `64`-`78` | 沿用 BSD `sysexits` 风格保留位 |

---

## 3. 直接命令模式（脚本 / 自动化）

### 3.1 设计原则

- 一行一动作；命令名见 §2.2，动作动词与后端 REST 对齐。
- 入参支持 **位置参数** + **`--key value` 标志**；JSON / YAML 体可用 `--from-file` / `--from-stdin`。
- 列表查询统一支持：`--search`、`--page`、`--page-size`、`--sort`、`--filter k=v`。
- 写操作统一支持：`--yes`、`--dry-run`、`--output json`。
- 资源 ID 既可以传数据库 ID（如 `agent_xxx`），也可以传 `key`（如 `--key writer-agent`）；CLI 自动按 `key` 解析为 ID。

### 3.2 Skill 命令示例

```bash
# 从 GitHub URL 安装 Skill（核心命令，详见 §5）
aranea skill install https://github.com/anthropic/skills/tree/main/figma-code-connect

# 从本地 zip 导入
aranea skill import ./my-skill.zip

# 列表
aranea skill ls --search figma --enabled true --page 1

# 启停 / 删除
aranea skill enable figma-code-connect
aranea skill disable figma-code-connect
aranea skill delete figma-code-connect --yes

# 查看运行记录
aranea skill runs --skill figma-code-connect --status success --from 2026-04-25
```

### 3.3 Agent 命令示例

```bash
# 创建 Agent（YAML 描述）
aranea agent create --from-file ./agents/writer.yaml

# 修改 Agent 工具策略
aranea agent tools-set writer \
  --enabled true --profile coding \
  --allow group:filesystem,web_search --deny shell_exec

# 触发一次性会话
aranea agent run writer --message "写一段 Aranea 介绍" --stream
```

### 3.4 MCP / Plugin / Tool / Cron / Channel

```bash
aranea mcp add --name notion --transport streamable_http \
  --url https://mcp.example.com/notion --header Authorization='Bearer ...'

aranea plugin enable runtime_audit
aranea plugin order set runtime_audit=10 cost_guard=20

aranea tool enable web_search
aranea tool config web_search --from-stdin <<'JSON'
{ "provider": "tavily", "max_results": 5 }
JSON

aranea cron add --name nightly-report --schedule cron --expr "0 2 * * *" \
  --agent writer --message "生成昨日报告"

aranea channel test feishu-default
aranea channel send feishu-default --text "Hello from CLI"
```

### 3.5 Monitor / Session

```bash
aranea monitor audit --from 2026-04-26 --kind agent.update
aranea monitor logs tail --level warn
aranea monitor usage trends --from 2026-04-19

aranea session new --agent writer
aranea session send <session_id> "继续上面的任务"
aranea session stream <session_id>     # SSE 实时事件
```

### 3.6 输出格式契约

| 模式 | 行为 |
|------|------|
| `text`（默认） | 表格 + 关键字段，标题加粗 + 状态色（成功绿 / 失败红 / 警告黄） |
| `table` | 等价于 `text` 但强制使用表格 |
| `json` | 输出后端原始 JSON（去掉 CLI wrapper），适合 `jq` |
| `yaml` | 输出 YAML（用于 `aranea agent get xxx -o yaml > writer.yaml` 备份） |
| `--quiet` | 仅输出 ID / 关键字段，每行一个值 |

错误统一输出到 `stderr`，结构如：

```
ERROR: skill 'figma-code-connect' name conflict
  code:    NAME_CONFLICT
  request: POST /api/v1/skills/import
  hint:    使用 `aranea skill ls --search figma-code-connect` 查看已存在 skill
```

`--output json` 时错误也用 JSON：

```json
{
  "error": {
    "code": "NAME_CONFLICT",
    "message": "skill 'figma-code-connect' name conflict",
    "request": { "method": "POST", "path": "/api/v1/skills/import" },
    "details": {}
  }
}
```

---

## 4. 对话模式（系统管家 Agent）

> 实现层面**完全复用 ADK Console Launcher 模式**（`google.golang.org/adk/cmd/launcher/console`）：`runner.New(...).Run(ctx, userID, sessionID, userMsg, agent.RunConfig{StreamingMode: SSE})` 迭代器，沿用 `signal.NotifyContext(ctx, os.Interrupt)`、TTY 自动检测、`User -> ` / `Agent -> ` 提示符；只在其上叠加：1) 启动横幅 2) 斜杠命令 3) 工具调用折叠块 4) 二次确认气泡。详细对应见 §1.6。

### 4.1 启动与界面

```text
$ aranea
Aranea CLI 1.0.0  ·  backend: http://127.0.0.1:8080  ·  agent: 系统管家
Tip: 直接说人话；输入 /help 看命令；Ctrl+C 退出。

aranea> 帮我把 https://github.com/anthropic/skills 这个仓库里的 figma-code-connect 装上
```

| 区域 | 行为 |
|------|------|
| 启动横幅 | 一行：版本、后端地址、当前 Agent、当前 Session（若复用） |
| 提示符 | `aranea> `；执行中切换为 `aranea⏵ `（spinner 动画） |
| 输入 | 多行：默认 `Enter` 发送；`Shift+Enter` 或 `\` 行末换行；`Esc Esc` 取消正在输入；`Ctrl+L` 清屏 |
| 模型回复 | 流式渲染；引用块、代码块、Markdown 表格在终端中按 ANSI 色渲染 |
| 工具调用 | 每次 tool call 在终端显示折叠块：`▼ skill_install_from_url(url=…)`，工具结果默认折叠摘要，`/expand` 展开 |
| 用户确认 | 高风险动作进入交互气泡：`确认安装 figma-code-connect？(y/N)`，或下钻菜单选 `保留 / 跳过 / 炼化` |
| 结束执行 | 模型回复完后回到提示符；最近一次响应可用 `/copy` 复制到剪贴板 |

### 4.2 内置斜杠命令

| 命令 | 作用 |
|------|------|
| `/help` | 显示常用斜杠命令 |
| `/agent <key>` | 切换对话 Agent；`/agent default` 回到系统管家 |
| `/team <key>` | 切换为 Team 编排模式 |
| `/session new` | 开新会话 |
| `/session list` | 最近 20 个会话 |
| `/session resume <id>` | 切换到指定会话 |
| `/model <provider>:<model>` | 临时切换模型 |
| `/tools` | 列出当前 Agent 可用工具 |
| `/expand` | 展开上一条工具结果完整内容 |
| `/copy` | 复制上一条回复 |
| `/dry-run on/off` | 工具调用是否仅打印将要发送的 HTTP 请求 |
| `/yes` | 临时跳过本会话内的所有确认 |
| `/quit` | 退出 |

### 4.3 系统管家 Agent 的核心约定

| 项 | 值 |
|----|----|
| `agent_key` | `__system_admin__` |
| 显示名 | `系统管家` |
| 类型 | 内置 Agent，由后端 `SeedSystemAdminAgent()` 在启动时确保存在 |
| 是否在 Agent 列表展示 | 列表展示但锁定（`readonly=true`），不可删除、不可改名 |
| 默认模型 | 跟随当前 `default_provider` + `default_model`；可被 `/model` 临时覆盖 |
| `tools_profile` | `system_admin`（专用 profile，仅含 `cli_admin_*` 与必要的 `read_file` / `web_fetch`） |
| `tools_allow` | 仅 `group:cli_admin`、`web_fetch`、`read_file`、`datetime` |
| `tools_deny` | `shell_exec`、`write_file`、`create_image`、`tts` 等高风险 / 与系统管理无关的工具 |
| Instruction（系统提示词） | 见 §4.6 |
| 是否可被普通 Chat 页选中 | 否；仅 CLI 默认会话 + Web 控制台「系统管理」入口可见 |

### 4.4 安装 Skill GitHub URL 的对话样例

```text
aranea> 帮我把 https://github.com/anthropic/skills/tree/main/figma-code-connect 装一下

▼ skill_install_from_url(url=…/figma-code-connect)
  step 1/6  解析 URL                       ✓ owner=anthropic repo=skills subpath=figma-code-connect ref=main
  step 2/6  下载子目录                     ✓ 18 个文件，263 KB
  step 3/6  本地预校验 SKILL 编写规范      ✓ 包含 SKILL.md、frontmatter 完整
  step 4/6  打包 zip                       ✓ /tmp/aranea/figma-code-connect-2026-04-26.zip
  step 5/6  上传到后端 (/api/v1/skills/import)  job_id=job_7f3
  step 6/6  轮询 job_7f3                   warn ⚠ 1 个相似冲突组

冲突组 group_01：相似度 42% (medium risk)
  候选：figma-code-connect            （本次上传）
  已有：Figma Code Connect v1.0.0     （4 月 25 日由 admin 创建）
  原因：Both describe mapping Figma components to code snippets

请选择处理方式：
  [1] 跳过该候选            （不入库，保留已有）
  [2] 强制保留两份          （warn 升级为接受）
  [3] 让我用 AI 炼化合并    （生成新草稿，需人工确认）

aranea> 3

▼ skill_refine_conflict_group(job_id=job_7f3, group_id=group_01)
  using model: openai/gpt-4.1
  ✓ 生成合并草稿（merged_name="Figma Code Connect", body 1.8KB）

预览（前 10 行）：
  # Figma Code Connect
  Creates and maintains Figma Code Connect template files that map …
  …

是否将合并结果作为新草稿入库？(y/N) y

▼ skill_import_apply(job_id=job_7f3, decisions=[merge_group_with_ai])
  ✓ 入库成功 → skill_id=skill_8a2  status=draft  enabled=false

已完成。提示：
  · 新 Skill 是 draft 状态，运行 `aranea skill publish skill_8a2` 即可发布。
  · 旧 Skill `figma-code-connect` 未被自动归档，可用 `aranea skill disable figma-code-connect` 停用。
```

要求：

1. 工具步骤必须**步进式输出**（不能等结果一次性吐出来），每一步带 ✓ / ⚠ / ✗ 状态。
2. 任何会变更系统状态的步骤（apply、覆盖、删除、停用、启用高风险）都需要二次确认，除非 `--yes` 或会话内 `/yes`。
3. 工具内部失败必须把 HTTP 状态码、`error.code`、`error.message` 完整展示，并给出可执行的修复指引。

### 4.5 Console Launcher 实现骨架（对齐 ADK `console.NewLauncher()`）

`aranea/backend/cmd/aranea/launcher/console/console.go` 直接照搬 `adk-go/cmd/launcher/console/console.go` 的结构，差异点用注释标出：

```go
package console

import (
    "bufio"
    "context"
    "errors"
    "flag"
    "fmt"
    "io"
    "os"
    "os/signal"
    "time"

    "google.golang.org/genai"

    "google.golang.org/adk/agent"
    adklauncher "google.golang.org/adk/cmd/launcher"
    "google.golang.org/adk/cmd/launcher/universal"
    "google.golang.org/adk/internal/cli/util"
    "google.golang.org/adk/runner"

    araneal "arenea/backend/cmd/aranea/launcher"
)

type consoleConfig struct {
    streamingMode       agent.StreamingMode
    streamingModeString string
    shutdownTimeout     time.Duration
    sessionID           string // Aranea 扩展：复用旧会话
    agentKey            string // Aranea 扩展：切换默认 Agent
    teamKey             string // Aranea 扩展：切换为 Team 模式
    autoYes             bool   // Aranea 扩展：跳过二次确认
}

type consoleLauncher struct {
    flags  *flag.FlagSet
    config *consoleConfig
    arn    *araneal.Config // Aranea 业务依赖容器
}

func NewLauncher(arn *araneal.Config) adklauncher.SubLauncher {
    cfg := &consoleConfig{}
    fs := flag.NewFlagSet("console", flag.ContinueOnError)
    fs.StringVar(&cfg.streamingModeString, "streaming_mode", "",
        fmt.Sprintf("(%s|%s)", agent.StreamingModeNone, agent.StreamingModeSSE))
    fs.DurationVar(&cfg.shutdownTimeout, "shutdown-timeout", 2*time.Second, "")
    fs.StringVar(&cfg.sessionID, "session", "", "复用已有会话 ID")
    fs.StringVar(&cfg.agentKey, "agent", "__system_admin__", "默认 Agent key")
    fs.StringVar(&cfg.teamKey, "team", "", "切换为 Team 模式")
    fs.BoolVar(&cfg.autoYes, "yes", false, "跳过所有二次确认")
    return &consoleLauncher{config: cfg, flags: fs, arn: arn}
}

func (l *consoleLauncher) Keyword() string           { return "console" }
func (l *consoleLauncher) SimpleDescription() string { return "进入对话模式 REPL（默认系统管家）" }
func (l *consoleLauncher) CommandLineSyntax() string { return util.FormatFlagUsage(l.flags) }

func (l *consoleLauncher) Parse(args []string) ([]string, error) {
    if err := l.flags.Parse(args); err != nil {
        return nil, fmt.Errorf("parse: %w", err)
    }
    l.config.streamingMode = agent.StreamingMode(l.config.streamingModeString)
    return l.flags.Args(), nil
}

func (l *consoleLauncher) Execute(ctx context.Context, c *adklauncher.Config, args []string) error {
    rest, err := l.Parse(args)
    if err != nil {
        return err
    }
    if err := universal.ErrorOnUnparsedArgs(rest); err != nil {
        return err
    }
    return l.Run(ctx, c)
}

func (l *consoleLauncher) Run(ctx context.Context, c *adklauncher.Config) error {
    ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
    defer cancel()

    // === Aranea 差异点 1：Session 服务从 Aranea SessionService 桥接 ===
    sessionSvc := l.arn.Sessions.AsADKSessionService() // 包装为 adk session.Service
    sess, err := openOrResumeSession(ctx, sessionSvc, l.config.sessionID, l.config.agentKey)
    if err != nil {
        return err
    }
    userID := l.arn.LocalUserID()

    // === Aranea 差异点 2：从数据库 Loader 取出指定 agent_key 的 Agent ===
    rootAgent, err := c.AgentLoader.LoadAgent(l.config.agentKey)
    if err != nil {
        return fmt.Errorf("load agent %q: %w", l.config.agentKey, err)
    }

    r, err := runner.New(runner.Config{
        AppName:        "aranea-cli",
        Agent:          rootAgent,
        SessionService: sessionSvc,
        ArtifactService: c.ArtifactService,
        // === Aranea 差异点 3：注入 Aranea Plugin 链（runtime_audit / cost_guard / tool_recorder ...）
        // 与 Web 端共用，保证 source=cli 审计统一 ===
        PluginConfig:   c.PluginConfig,
    })
    if err != nil {
        return err
    }

    printBanner(l.arn, sess.ID(), l.config.agentKey)

    // === Aranea 差异点 4：REPL 处理「斜杠命令 + 工具气泡 + 二次确认」===
    return repl.Run(ctx, repl.Config{
        Runner:        r,
        Session:       sess,
        UserID:        userID,
        StreamingMode: resolveStreamingMode(l.config.streamingMode),
        AutoYes:       l.config.autoYes,
        Stdin:         bufio.NewReader(os.Stdin),
        Stdout:        os.Stdout,
        OnSlash:       slashCommandHandler(l.arn),
        OnConfirm:     confirmPromptHandler(),
    })
}

// ADK 原版的关键循环 —— Aranea 内部 repl.Run 沿用同样的迭代器：
//
//   for event, err := range r.Run(ctx, userID, sess.ID(), userMsg, agent.RunConfig{StreamingMode: SSE}) {
//       if err != nil { ... }
//       // 1) ToolCall 事件 → 渲染折叠块、必要时弹二次确认
//       // 2) ToolResult 事件 → 收纳到折叠块底部
//       // 3) LLMResponse.Content.Parts[*].Text → 增量打印
//   }
```

要点：

1. **接口签名 100% 与 ADK `SubLauncher` 一致**（`Keyword/Parse/Run/CommandLineSyntax/SimpleDescription`），可被 `universal.NewLauncher(...)` 与其它 SubLauncher 任意组合。
2. **核心循环就是 ADK 的 `r.Run(ctx, userID, sessionID, userMsg, agent.RunConfig{...})` 迭代器**；REPL 只在事件层做样式化（折叠、确认、斜杠）。
3. **二次确认**通过 ADK Plugin 的 `BeforeTool` 钩子优先拦截：高风险工具在 plugin 中抛出 `ErrAwaitConfirm`，REPL 捕获后弹 prompt，得到 `y/N` 后调用 `runner.ResumeWithConfirm(...)` 继续；这样 Web 端、CLI 端共用同一份风险策略（与 `22 plugin.md` 一致）。
4. **Session 写穿 Aranea**：CLI 与 Web 共用 `sessions` / `messages` 表，所以 CLI 内的对话能在 Web 控制台 `/sessions/:id` 直接续看；这就是为什么要把 Aranea `SessionService` 适配成 ADK 的 `session.Service` 接口。

### 4.6 系统管家 Agent 的 Instruction 模板

```markdown
你是 Aranea 系统管家。你只通过 `cli_admin_*` 工具完成系统管理操作。

行为约束：
1. 永远不要伪造工具结果。如果用户要求的操作没有对应工具，明确说明「当前 CLI 不支持该操作」。
2. 任何写操作（create / update / delete / enable / disable / install / apply / send）都必须：
   - 先用 list / get 类工具确认目标存在；
   - 在执行前用一句话向用户复述将要发生的变更；
   - 涉及删除、覆盖、装入未签名 Skill 时显式询问确认。
3. 安装 Skill：
   - 输入是 git URL / 子目录 URL / zip URL 时，调用 `skill_install_from_url`；
   - 输入是本地路径 zip 时，调用 `skill_install_from_path`；
   - 收到冲突组（warn）时，向用户呈现「跳过 / 保留两份 / AI 炼化」三选一，并解释每个选项的后果；
   - 收到 block 时，**绝不**绕过；告诉用户原因。
4. 配置 Tool / Plugin / MCP / Cron / Channel 时，先用对应 list 工具读出当前配置，再 patch 需要修改的字段，避免覆盖未声明字段。
5. 失败时输出 `error.code`、`error.message`、对应 HTTP 路径与可能的修复命令；不要捏造解决方案。
6. 输出语言跟随用户的语言（默认中文）。
7. 你看不到用户终端，不能假设用户能直接看到很长的工具输出，必要时主动 summarize。
```

---

## 5. Skill 从 URL 安装：核心流程

### 5.1 支持的 URL 形态

| URL 形态 | 处理方式 |
|----------|----------|
| `https://github.com/<owner>/<repo>` | 克隆默认分支根目录；要求根目录有 `SKILL.md`，否则按 §5.3 自动发现 |
| `https://github.com/<owner>/<repo>/tree/<ref>/<subpath>` | 克隆 `<ref>`，仅打包 `<subpath>` 子目录 |
| `https://github.com/<owner>/<repo>/blob/<ref>/<subpath>/SKILL.md` | 取 `SKILL.md` 所在目录为根 |
| `git@github.com:<owner>/<repo>.git` / `ssh://...` | 走 SSH，需要本地有可用 key |
| `https://gitlab.com/...` / `https://gitee.com/...` / `https://codeberg.org/...` | 同 GitHub 解析规则 |
| `https://example.com/path/skill.zip` | 直接下载 zip，跳过 clone |
| `npm:<pkg>` / `pypi:<pkg>` | 后续迭代，本期不支持 |

参数：

| 参数 | 默认 | 说明 |
|------|------|------|
| `--ref <branch/tag/sha>` | 仓库默认分支 | 锁定版本 |
| `--subpath <dir>` | 自动发现 | 强制指定 SKILL.md 所在目录 |
| `--name <slug>` | 由目录名派生 | 覆盖默认 slug |
| `--enable` | false | 安装后立即启用（需 `published`） |
| `--publish` | false | 安装后自动发布 |
| `--decision skip\|keep\|refine` | 进入冲突组时的默认决定 | 与 `--yes` 配合使用 |
| `--keep-temp` | false | 保留 clone / zip 临时文件用于排错 |

### 5.2 流程状态机

```text
parse_url
  └─> resolve_ref       (HEAD 或显式 ref)
        └─> fetch       (git clone --depth 1 / wget zip)
              └─> locate_skill_root   (§5.3)
                    └─> local_validate (§5.4)
                          └─> pack_zip
                                └─> POST /api/v1/skills/import
                                      └─> poll job (1.5s × 80 次)
                                            ├─ pass     → POST apply (import_passed)
                                            ├─ warn     → 询问 / 按 --decision 自动
                                            └─ block    → 报错退出 (exit 5)
```

### 5.3 自动发现 SKILL 根目录

当用户给的是仓库根，需要自动定位 `SKILL.md`：

| 规则 | 优先级 |
|------|--------|
| 根目录存在 `SKILL.md` | 直接采用 |
| 仅一个一级子目录有 `SKILL.md` | 采用该子目录 |
| 多个子目录有 `SKILL.md`，但仓库存在 `skills.json` / `pyproject.toml` 声明 | 用户必须 `--subpath` 选一个 |
| 未发现任何 `SKILL.md` | 报错 `SKILL_NOT_FOUND`，退出 5 |
| 多个 `SKILL.md` 且无声明 | 进入对话流程让 Agent 询问；脚本模式 `--subpath` 必填 |

### 5.4 本地预校验

CLI **不替代**后端校验，但本地先做一次轻量检查可以避免无效上传：

| 检查 | 失败行为 |
|------|----------|
| 存在 `SKILL.md` | 终止，返回路径建议 |
| `SKILL.md` frontmatter 必含 `name` / `description` | 终止 |
| 包内不含 `.git`、`node_modules`、`venv`、`*.dll`、`*.exe`、超过 50MB 单文件 | 拒绝并打印命中规则；可 `--allow-large` 强制 |
| zip 内总大小 ≤ 100MB（默认） | 拒绝；可在 `config.toml` 调高 `skill.max_zip_mb` |

### 5.5 冲突组交互

无冲突：直接 apply；输出新 Skill ID。

warn（相似度 ≥ 0.2）：

```text
冲突组 group_01：相似度 42% (medium risk)
  候选：figma-code-connect (本次上传)
  已有：Figma Code Connect v1.0.0
  评估：name=31%  description=58%  body=47%  trigger=52%  tool=66%  confidence=84%
  原因：两个 Skill 都在描述 Figma 组件与代码模板的映射流程。
  证据：Both mention Code Connect template files
        Both describe mapping Figma components to code snippets

请选择 (1-3，或 q 取消)：
  [1] skip       本次跳过该候选
  [2] keep       保留两份（警告升级为接受）
  [3] refine     调用 AI 炼化合并（默认模型 openai/gpt-4.1）
```

block：直接 exit 5 并附 block 原因表。

### 5.6 失败回退

| 失败点 | 回退策略 |
|--------|----------|
| `git clone` 失败 | 删除临时目录；报告网络 / 凭据错误 |
| 本地预校验失败 | 不上传，临时目录可 `--keep-temp` 保留 |
| 上传失败 | 临时 zip 保留路径，提示 `aranea skill import <path>` 可重试 |
| 轮询超时 | 提示 `aranea skill import status <job_id>` 手工查询 |
| apply 部分成功 | 输出 `created_skill_ids` 与 `skipped_candidate_ids`，并解释跳过原因 |

---

## 6. 系统管家 Agent 的 Tool 集（CLI Admin Toolkit）

新增一组内置 Tool，统一前缀 `cli_admin_`，分类 `system`，源 `builtin`，对齐 `23 tools.md` 的 Tool 模型与 `tool_invocations` 审计。每个 `cli_admin_*` 都被实现为 ADK `tool.Tool`（`google.golang.org/adk/tool`），通过 `llmagent.Config{Tools: ...}` 注入到系统管家 Agent；执行时统一走 ADK Plugin 链上的 `BeforeTool`/`AfterTool`/`OnToolError` 钩子，与 Web 端调用同一份 `tool_recorder` / `runtime_audit` Plugin（参见 `22 plugin.md`、`23 tools.md`）。

### 6.1 通用约定

| 约定 | 说明 |
|------|------|
| 命名 | `cli_admin_<resource>_<action>`，例如 `cli_admin_skill_install_from_url` |
| 入口 | 全部走后端 REST，不在 CLI 端直接读写数据库 |
| 风险 | 默认 `medium`；删除 / 启停高风险工具 / 安装 Skill / 发送 channel 消息提升为 `high` |
| 参数 schema | 每个工具暴露 `parameters_schema`，必含 `idempotency_key`、`dry_run` 字段 |
| 系统注入字段 | `agent_id`、`session_id`、`source=cli` 由后端注入，模型不可控制 |
| 输出 | 成功返回业务对象 + `next_actions[]`（建议下一步命令）；失败返回 `error.code` / `error.message` |
| 长任务 | 创建 job 的 Tool 同时返回 `job_id`，并提供配套 `cli_admin_*_status` 工具供轮询 |

### 6.2 Tool 列表

| Tool Key | 风险 | 后端调用 | 说明 |
|----------|------|----------|------|
| `cli_admin_skill_list` | low | `GET /api/v1/skills` | 搜索 / 分页查询 Skill |
| `cli_admin_skill_install_from_url` | high | clone + `POST /api/v1/skills/import` + 轮询 | §5 主流程 |
| `cli_admin_skill_install_from_path` | high | `POST /api/v1/skills/import`（zip 流） | 本地路径 zip 安装 |
| `cli_admin_skill_import_status` | low | `GET /api/v1/skills/import/:job_id` | 轮询导入状态 |
| `cli_admin_skill_import_apply` | high | `POST /api/v1/skills/import/:job_id/apply` | 应用导入决策 |
| `cli_admin_skill_refine_conflict` | medium | `POST /api/v1/skills/import/:job_id/conflict-groups/:group_id/refine` | AI 炼化冲突组 |
| `cli_admin_skill_enable` / `_disable` | medium | `PATCH /api/v1/skills/:id/enabled` | 启停 |
| `cli_admin_skill_delete` | high | `DELETE /api/v1/skills/:id` | 软删 |
| `cli_admin_agent_list` / `_get` / `_create` / `_update` / `_delete` | low~high | `/api/v1/agents*` | Agent CRUD |
| `cli_admin_agent_tools_get` / `_set` | medium | `/api/v1/agents/:id/tools/effective` & `/policy` | Agent 工具策略 |
| `cli_admin_team_list` / `_create` / `_update` / `_delete` / `_run` | low~high | `/api/v1/teams*` | Team |
| `cli_admin_tool_list` / `_enable` / `_disable` / `_config_set` | medium~high | `/api/v1/tools*` | 全局 Tool |
| `cli_admin_plugin_list` / `_enable` / `_disable` / `_order_set` / `_config_set` | medium | `/api/v1/plugins*` | Plugin |
| `cli_admin_mcp_list` / `_add` / `_update` / `_delete` / `_test` | medium~high | `/api/v1/mcp-servers*` | MCP |
| `cli_admin_cron_list` / `_add` / `_update` / `_delete` / `_pause` / `_resume` / `_trigger` | medium | `/api/v1/cron-tasks*` | Cron |
| `cli_admin_channel_list` / `_add` / `_update` / `_delete` / `_test` / `_send` | high | `/api/v1/channels*` | Channel |
| `cli_admin_provider_list` / `_add` / `_update` / `_delete` / `_inspect` | medium | `/api/v1/llm-provider-models*` | LLM Provider |
| `cli_admin_session_list` / `_get` / `_send` | low~medium | `/api/v1/sessions*`、`/api/v1/chat/messages` | Session |
| `cli_admin_monitor_audit` / `_events` / `_traces` / `_usage_overview` | low | `/api/v1/monitor/*` | 监控只读 |
| `cli_admin_system_health` | low | `/healthz` + `/api/v1/system/*` | 后端健康 |
| `cli_admin_system_backup` / `_restore` | high | `/api/v1/system/backup` / `/restore`（如不存在由后端补） | 备份 / 恢复 |

### 6.3 关键 Tool 的参数 schema 示例

`cli_admin_skill_install_from_url`：

```json
{
  "type": "object",
  "required": ["url"],
  "properties": {
    "url": { "type": "string", "format": "uri" },
    "ref": { "type": "string" },
    "subpath": { "type": "string" },
    "name": { "type": "string" },
    "enable": { "type": "boolean", "default": false },
    "publish": { "type": "boolean", "default": false },
    "decision": {
      "type": "string",
      "enum": ["ask", "skip", "keep", "refine"],
      "default": "ask"
    },
    "refine_provider": { "type": "string" },
    "refine_model": { "type": "string" },
    "refine_instructions": { "type": "string" },
    "dry_run": { "type": "boolean", "default": false },
    "idempotency_key": { "type": "string" }
  }
}
```

返回：

```json
{
  "job_id": "job_7f3",
  "validation_status": "warn",
  "created_skill_ids": ["skill_8a2"],
  "conflict_groups": [],
  "next_actions": [
    { "label": "publish", "command": "aranea skill publish skill_8a2" },
    { "label": "enable",  "command": "aranea skill enable skill_8a2" }
  ]
}
```

`cli_admin_agent_tools_set`：

```json
{
  "type": "object",
  "required": ["agent_key"],
  "properties": {
    "agent_key": { "type": "string" },
    "tools_enabled": { "type": "boolean" },
    "tools_profile": { "type": "string" },
    "tools_allow": { "type": "array", "items": { "type": "string" } },
    "tools_deny":  { "type": "array", "items": { "type": "string" } },
    "tools_concurrent_allow": { "type": "array", "items": { "type": "string" } },
    "dry_run": { "type": "boolean", "default": false }
  }
}
```

### 6.4 Tool 与现有 `tools` 表的关系

- 所有 `cli_admin_*` 在后端启动时由 `SeedBuiltinTools()` 注册到 `tools` 表，`category=system`，`source=builtin`。
- 默认 `enabled=1`，**仅对 `__system_admin__` Agent 在 `tools_allow_json` 中可见**；其他 Agent `tools_deny_json` 默认包含 `group:cli_admin`。
- `tool_invocations` 中 `source` 字段新增枚举值 `cli`，便于 Monitor 区分 Web 触发与 CLI 触发。

### 6.5 系统管家 Agent 通过 `agent.Loader` 暴露给 ADK

为了让 §4.5 的 Console Launcher 拿到的就是「内置系统管家」+「数据库里的全部用户 Agent」，Aranea 实现一个 `dbAgentLoader`，签名直接对齐 ADK `agent.Loader`：

```go
package agentloader

import (
    "context"

    "google.golang.org/adk/agent"

    "arenea/backend/internal/service"
)

type dbAgentLoader struct {
    svc       *service.AgentService
    builtin   agent.Agent           // __system_admin__
    cache     map[string]agent.Agent
}

func New(ctx context.Context, svc *service.AgentService, sysAdmin agent.Agent) (agent.Loader, error) {
    return &dbAgentLoader{svc: svc, builtin: sysAdmin, cache: map[string]agent.Agent{
        sysAdmin.Name(): sysAdmin,
    }}, nil
}

// RootAgent 返回系统管家 —— Console Launcher 默认使用 RootAgent
func (l *dbAgentLoader) RootAgent() agent.Agent { return l.builtin }

// LoadAgent 把 Aranea agent_key 翻译为 ADK agent.Agent
func (l *dbAgentLoader) LoadAgent(name string) (agent.Agent, error) {
    if a, ok := l.cache[name]; ok {
        return a, nil
    }
    rec, err := l.svc.GetByKey(context.Background(), name)
    if err != nil { return nil, err }
    a, err := buildLLMAgent(rec) // llmagent.Config{Name, Instruction, Model, Tools, ...}
    if err != nil { return nil, err }
    l.cache[name] = a
    return a, nil
}

func (l *dbAgentLoader) ListAgents() []string {
    keys, _ := l.svc.ListKeys(context.Background()) // 含 __system_admin__
    return keys
}
```

`buildLLMAgent(rec)` 内部把 Aranea 的 Agent 配置（Provider/Model/Instruction/Tool 策略/Hook）翻译成 `llmagent.Config{Tools: cliAdminTools, ...}`。这意味着：

1. CLI Console 默认接 `RootAgent()` → 系统管家。
2. `aranea console --agent writer` 走 `LoadAgent("writer")` → 数据库里的写作 Agent，行为与 Web Chat 完全一致。
3. ADK 自带的 `aranea web webui` 模式（如果叠加 `adkrest` Server）也用同一个 Loader，无需第二份配置。

---

## 7. 后端契约（CLI 与 Backend）

CLI 默认完全复用现有 `/api/v1/*` REST API，不引入 CLI 专属端点。仅以下接口需要新增或扩展：

### 7.1 系统信息

`GET /api/v1/system/info`

```json
{
  "version": "1.0.0",
  "git_commit": "abcdef",
  "build_time": "2026-04-26T08:00:00Z",
  "default_workspace_id": "ws_default",
  "default_provider": "openai",
  "default_model": "gpt-4.1",
  "system_admin_agent_id": "agent_system_admin",
  "system_admin_agent_key": "__system_admin__",
  "skills": {
    "storage_root": "/var/lib/aranea/skills",
    "max_zip_mb": 100
  }
}
```

CLI 启动横幅与 `aranea version` 用此接口。

### 7.2 系统管家 Agent 与 Tool 种子

| 时机 | 行为 |
|------|------|
| 后端启动 | `SeedSystemAdminAgent()`：插入或更新 `agent` 表中 `agent_key=__system_admin__` 记录，`readonly=1`，`category=system`，`tools_profile=system_admin`，`tools_allow_json=["group:cli_admin","web_fetch","read_file","datetime"]` |
| 后端启动 | `SeedBuiltinTools()`：注册 §6.2 所有 `cli_admin_*` 工具，分类 `system`，`source=builtin` |
| 后端启动 | `SeedToolGroups()`：建立 `group:cli_admin` 工具组 → 包含全部 `cli_admin_*` |

### 7.3 Skill 安装链路对 CLI 的扩展

为支持 §5 的「子目录定位 + 服务端安全收口」，建议 `POST /api/v1/skills/import` 增加可选字段：

```json
{
  "source": "cli_url",
  "source_url": "https://github.com/anthropic/skills/tree/main/figma-code-connect",
  "source_ref": "main",
  "source_subpath": "figma-code-connect",
  "client_validation": {
    "skill_md_found": true,
    "frontmatter_ok": true,
    "size_bytes": 270336
  }
}
```

后端在 `tool_invocations.metadata_json` 与 `audit_logs` 中保留 `source_url` 与 `source_ref`，便于 Web 控制台显示「该 Skill 来自 GitHub xxx」。

### 7.4 SSE 与流式

CLI 对话模式调用 `POST /api/v1/chat/messages/stream`，事件类型沿用 `18 monitor.md` 与 `1 chat.md` 的约定：

| 事件 | 渲染 |
|------|------|
| `message.delta` | 增量打印模型文本 |
| `tool.call` | 折叠块标题 + spinner |
| `tool.result` | 在折叠块内追加结果摘要 |
| `tool.error` | 红色追加错误码 |
| `usage` | 不渲染，写入 `--debug` 输出 |
| `done` | 结束 spinner，回到提示符 |

### 7.5 错误格式

复用 `20 skill.md` §7.7 / `23 tools.md` 的统一错误结构。CLI 在终端再封装一层：把 `error.code` 翻译成中文短句（可选），但 `--output json` 时原样透传。

---

## 8. 数据模型与新增表（如需）

CLI 主要复用现有数据模型。新增 / 扩展项：

### 8.1 `agent` 表

| 字段 | 变更 |
|------|------|
| `readonly` | 新增 `INTEGER NOT NULL DEFAULT 0`，标记 `__system_admin__` 等内置不可改不可删的 Agent |
| `kind` | 新增 `TEXT NOT NULL DEFAULT 'user'`，枚举 `user` / `system`；前端按 `system` 区分展示 |

### 8.2 `tool_invocations` 表

| 字段 | 变更 |
|------|------|
| `source` | 枚举扩展：`adk` / `manual` / `system` / `cli` |
| `metadata_json` | 增加约定字段 `cli.terminal`、`cli.version`、`cli.os`，便于排障 |

### 8.3 新增 `cli_sessions`（可选）

CLI 端会话本地 jsonl 已经够用，但若需要在 Web 端看到「来自 CLI 的会话」，建议在已有 `sessions` 表 `metadata_json` 里写入：

```json
{
  "origin": "cli",
  "cli_version": "1.0.0",
  "cli_terminal": "xterm-256color",
  "cli_os": "windows-amd64"
}
```

不新建表。

### 8.4 配置文件 `~/.aranea/config.toml`

```toml
[backend]
base_url = "http://127.0.0.1:8080"
token    = ""
workspace_id = ""

[ui]
output  = "text"      # text | json | yaml | table
color   = "auto"      # auto | always | never

[skill]
default_decision = "ask"   # ask | skip | keep | refine
refine_provider  = ""      # 留空跟随后端 default
refine_model     = ""
max_zip_mb       = 100
keep_temp        = false

[chat]
default_agent  = "__system_admin__"
default_team   = ""
auto_resume    = true     # 启动时自动恢复上次会话

[telemetry]
enabled = false
```

---

## 9. 安全、审计与边界

| 项 | 控制 |
|----|------|
| 不允许 CLI 直接执行任意 shell | `cli_admin_*` 工具集**不**包含 `shell_exec`；用户如需运行命令请走 Web 控制台 + 高风险确认 |
| 不允许 CLI 直接读写工作区任意文件 | CLI 只能在 `~/.aranea/` 与临时目录 `~/.aranea/tmp/<job>/` 写入；Skill 安装的 zip 在 apply 后清理 |
| 远程后端凭据 | `token` 落 `config.toml`，文件权限要求 `0600`；CLI 拒绝读取权限超过 0644 的 token 文件并提示 `chmod 600` |
| 危险动作 | 默认弹出确认；`--yes` 与 `/yes` 仅在当前会话内生效，重启后失效 |
| 审计 | 每次 `cli_admin_*` 调用通过后端走标准 `tool_invocations` + `audit_logs`；CLI 不持有「绕过审计」的能力 |
| Dry-run | 所有写操作工具都必须支持 `dry_run=true`，后端在 dry-run 时返回「将要发生的变更摘要」而不真正执行 |
| 风险升级 | 后端仍可对单次调用基于工作区策略升级风险，例如 channel 在生产环境强制人工确认 |

---

## 10. 输出与终端体验细节

| 场景 | 行为 |
|------|------|
| 无 TTY（管道 / CI） | 默认关闭色与进度条；`text` 输出降级为 `key=value` 单行；建议 CI 用 `--output json` |
| 大表格 | 超过终端宽度时优先截断中间列；`-o yaml` / `-o json` 输出全量 |
| Spinner | 所有阻塞 > 200ms 的操作显示 spinner；超过 5s 显示 `(已耗时 X 秒)` |
| 错误高亮 | `code` 高亮红 + 加粗，`hint` 黄色 |
| 链接 | 输出 Web 控制台对应路径（如 `查看：http://127.0.0.1:8080/skills/skill_8a2`） |
| 复制 | 关键 ID（Skill / Agent / Job）首尾用零宽符号包裹，便于多数终端的双击选择 |
| 国际化 | 默认中文；`LANG=en_US.UTF-8` 或 `--lang en` 切换英文（本期可只做关键消息） |

> 配色与「Starting / Finished successfully」横幅的实现复用 ADK 的 `internal/cli/util/oscmd.go` 风格：`util.LogStartStop("clone repo", func(p util.Printer) error { ... })` 自动产出绿色 / 红色状态行，与 `adkgo deploy cloudrun` 的部署日志风格保持一致。

---

## 11. 实施拆分建议

> 包结构与命名严格对齐 §1.3 的 `cmd/...` 树形布局，每个 Phase 都对应可独立 PR 的 Go 子包。

### Phase 1：CLI 骨架 + 直接命令（Cobra）

| 工作 | 说明 |
|------|------|
| Go 二进制 `aranea` | 与后端同 repo；`cmd/aranea/main.go` + `internal/root/root.go`；使用 `github.com/spf13/cobra`，与 `adk-go/cmd/adkgo` 完全相同的写法 |
| Cobra 自注册 | 每个资源一个子包（`internal/agent`、`internal/skill`、…），在 `init()` 内 `root.RootCmd.AddCommand(...)`，主包只保留 `_ "..."` 空导入 |
| HTTP 客户端 | 共用 `internal/transport` 客户端代码；自动注入 `--base-url` / `--token` |
| 命令树 | 实现 §2.2 全部 `ls` / `get` / 主要 `create` / `delete` / `enable` / `disable` |
| 输出层 | 支持 `text` / `json` / `yaml`；统一表格组件；ANSI 调色板复用 ADK `internal/cli/util` 风格 |

### Phase 2：Launcher / SubLauncher 框架（对齐 ADK）

| 工作 | 说明 |
|------|------|
| `cmd/aranea/launcher/config.go` | 定义 `aranea/launcher.Config`，内嵌 ADK `launcher.Config`，附加 Aranea 业务 service、`AgentLoader` 工厂 |
| `cmd/aranea/launcher/console/` | SubLauncher 实现（§4.5 骨架）；`Keyword="console"`，`flag.FlagSet` 解析；REPL 内核调用 ADK `runner.New(...).Run(ctx, userID, sessionID, msg, agent.RunConfig{StreamingMode: SSE})` |
| `cmd/aranea/launcher/web/` | SubLauncher 实现；`Keyword="web"`，挂载 Aranea HTTP server；`web.Sublauncher` 子启动器：`webui`（Quasar SPA）、`api`（Aranea REST）、可选 `adk-api`（叠加 `adkrest.Server`） |
| `cmd/aranea/launcher/full/` | `universal.NewLauncher(console, web(webui, api))`，作为默认入口 |
| `main.go` 路由策略 | 第一个参数是 launcher keyword 或为空 → 走 `full.NewLauncher().Execute(...)`；否则走 `root.RootCmd.Execute()` |

### Phase 3：系统管家 Agent + `agent.Loader` + `cli_admin_*` Tool 集

| 工作 | 说明 |
|------|------|
| 后端 | `SeedSystemAdminAgent()`、`agent.kind=system`、`agent.readonly=1` |
| 后端 | `cli_admin_*` Tool 集 + `group:cli_admin`；`tool_invocations.source` 增 `cli` |
| 后端 | `dbAgentLoader`（§6.5）实现 ADK `agent.Loader` 接口，把数据库内 Agent 暴露给 ADK |
| CLI | REPL：行编辑、历史、斜杠命令、流式渲染、工具折叠块、确认气泡 |

### Phase 4：Skill from URL 安装链路

| 工作 | 说明 |
|------|------|
| CLI | `git clone --depth 1` + 子目录定位 + 本地预校验 + 打包 |
| CLI | `cli_admin_skill_install_from_url` 工具：编排 clone + import + poll + apply |
| 后端 | `POST /api/v1/skills/import` 增加 `source_url` / `source_ref` / `source_subpath` 字段并落 `metadata_json` |
| 后端 | Web 控制台 Skill 详情显示「来源：GitHub <repo>@<ref>:<subpath>」 |

### Phase 5：体验与治理

| 工作 | 说明 |
|------|------|
| CLI | 自动补全脚本生成（Cobra 内置 `GenBashCompletion` / `GenZshCompletion` / `GenFishCompletion` / `GenPowerShellCompletion`）、`aranea config edit`、`aranea version` 升级提示 |
| CLI | `--otel_to_cloud` 直通 ADK telemetry 初始化；与 `24 telemetry.md` 共用同一份 OTel exporter |
| 后端 | dry-run 路径返回变更摘要 |
| 监控 | Monitor 页 audit / events 增加 `source=cli` 过滤器 |
| 文档 | `aranea help <topic>` 内嵌教程，至少覆盖：装 Skill、加 MCP、改 Agent 工具策略、定时任务 |

---

## 12. 验收标准

### 12.1 直接命令

- `aranea skill ls` 能正确分页显示当前 Skill。
- `aranea skill install <github-url>` 能完成 §5 全流程；无冲突时不需任何交互即入库。
- `aranea agent tools-set <key> --allow ... --deny ...` 修改后 Web 控制台立即可见。
- `aranea --output json` 输出可被 `jq` 解析；`--quiet` 输出每行一个 ID。
- 删除 / 启停高风险动作在没有 `--yes` 时拒绝执行并提示。

### 12.2 对话模式

- `aranea` 直接启动进入对话（即默认 `console` SubLauncher）；横幅展示后端可达性与 Agent 名称。
- 对话中工具调用以折叠块逐步出现，含 ✓ / ⚠ / ✗ 状态。
- 「装 GitHub 上的 figma-code-connect」语句能被识别并正确执行 §5 流程。
- 出现冲突组时能以菜单形式让用户选择 `skip / keep / refine`。
- 每次 `cli_admin_*` 调用都能在 Web 控制台 `/tools/runs` 看到一条 `source=cli` 的记录。
- `aranea web webui api` 能与 Aranea 现有后端进程互换：同一份 `Config` 注入、同一套 Plugin、同一个 `agent.Loader`。

### 12.3 安全与审计

- CLI 不能调用 `shell_exec` / `write_file` 等高危 / 与系统管理无关工具。
- `__system_admin__` Agent 不可删除、不可改名；尝试删除返回 `READONLY_AGENT` 错误。
- `--yes` / `/yes` 仅在当前进程会话内生效。
- 安装 Skill 在 block 时立即 exit 5；未给 `--decision` 且非交互终端时遇到 warn 也以 exit 5 退出（避免 CI 静默接受冲突）。

### 12.4 配置与可移植性

- `aranea config path` 在 Windows / macOS / Linux 输出正确路径。
- `config.toml` 的 `token` 文件权限不安全时 CLI 拒绝读取并打印修复建议。
- 升级到新版本后旧 `config.toml` 仍兼容（缺字段使用默认值）。

### 12.5 ADK 对齐

- `aranea/backend/cmd/aranea/launcher/console` 实现满足 ADK `launcher.SubLauncher` 接口（编译期断言：`var _ adklauncher.SubLauncher = (*consoleLauncher)(nil)`）。
- `aranea console` 与 `adk console` 行为一致：同样的 `streaming_mode` 标志、同样的 `User -> ` / `Agent -> ` 提示符、同样的 SSE 增量打印逻辑、同样的 `signal.NotifyContext` 退出。
- 把 ADK 官方 `console.NewLauncher()` 替换进 `aranea` 的 `full.NewLauncher()` 后仍能跑（验证依赖注入容器与 ADK 一致）。

---

## 13. 待确认问题

1. CLI 二进制是否随后端打包发布，还是单独 release？
2. 远程后端接入是否本期就引入 token / OIDC 登录？目前后端是否有用户体系？
3. `__system_admin__` Agent 是否在 Web 控制台 Agent 列表展示？还是隐藏在「系统」分类下？
4. `cli_admin_*` 工具是否允许其他自建 Agent 通过修改 `tools_allow_json` 启用？建议默认拒绝，需要后端硬编码白名单。
5. Skill 安装的临时目录 `~/.aranea/tmp/` 在 apply 成功后是否自动清理？默认清理还是保留 7 天？
6. 是否需要 `aranea backup` / `aranea restore`？后端是否提供导出整个 `aranea.db` + Skill 目录的接口？
7. CI 模式（`CI=true` 或非 TTY）下 `aranea skill install <url>` 默认行为：遇到 warn 应该 `skip` 还是 `exit 5`？本文默认 `exit 5`。
8. 是否引入 ADK `root_agent.yaml` 风格的开发模式：`aranea console --config ./agents/foo/root_agent.yaml` 直接加载本地 YAML 测试单个 Agent，绕过数据库？建议作为开发期可选项，默认仍走数据库 Loader。
9. 对话模式的多 Agent 切换：`/agent <key>` 是开新 session，还是同一 session 内仅改变下一条消息的执行 Agent？建议开新 session 以避免历史污染。
10. Skill 来自 git 私仓时的凭据如何管理？`git` 自身的 credential helper 是否足够？是否需要 `aranea config set git.token`？
11. 是否提供一个低门槛的 `aranea init` 引导（首次启动检测后端 / 配置默认 provider / 安装初始 Skill 包）？
12. ADK 的 `aranea web a2a` / `aranea web pubsub` / `aranea web eventarc` 是否本期就开放？还是只暴露 `webui` + `api` 两个 SubLauncher？建议本期只开 `webui` + `api` + `adk-api`（叠加 `adkrest`），其余按需扩展。

---

## 14. 与现有文档 / 模块的关系

| 模块 | 关系 |
|------|------|
| `1 chat.md` | 对话模式复用 chat 后端；UI 形态从 Quasar `QChatMessage` 切换为终端折叠块；底层运行循环对齐框架 Console Launcher（§4） |
| `2-8 agent*` | `aranea agent *` 直接命令与 §6 工具复用 Agent 模型与策略；`dbAgentLoader`（§6.5）实现 `agent.Loader`，把数据库 Agent 暴露给 Console / Web Launcher |
| `11 multi-agent.md` | `aranea team *` 与 `aranea chat --team` 复用 Team 编排；Team 模式下 Console Launcher 把 Team 当作 RootAgent 注入 |
| `19 mcp.md` | `aranea mcp *` 直接复用 MCP 表与 `/mcp-servers` API |
| `20 skill.md` | §5 安装链路严格依赖 `20 skill.md` 的导入 / 冲突组 / 炼化设计 |
| `21 cron.md` | `aranea cron *` 复用 `cron_task` |
| `22 plugin.md` | `aranea plugin *` 复用插件启停与排序；Console Launcher 通过 `runner.PluginConfig` 共用同一份 Plugin 链，CLI / Web 审计一致 |
| `23 tools.md` | §6 新增 `cli_admin_*` 一组 Tool，并扩展 `tool_invocations.source = cli`；Tool 实现为 **`pkg/trpc-agent-go` `tool.Tool`**，统一走 BeforeTool / AfterTool / OnToolError 钩子 |
| `17 channel.md` | `aranea channel send` 受同样的高风险二次确认约束 |
| `18 monitor.md` | CLI 调用全部进入 audit / events / traces；监控页可按 `source=cli` 过滤；OTel 由 `--otel_to_cloud` 走框架 telemetry |
| `24 telemetry.md` | CLI 与框架共用 `telemetry.InitAndSetGlobalOtelProviders(...)`，`--otel_to_cloud` 标志名也保持一致 |
| `30 ecosystem.md` | 商城安装可由 CLI 触发 `cli_admin_marketplace_install`（后续阶段） |
| **pkg/trpc-agent-go（tRPC-Agent-Go）** | `cmd/adkgo` 式管理命令 + `cmd/launcher/{universal,console,web,api,webui,full}`（Launcher / SubLauncher）+ `agent.Loader` + `runner.Runner` + `internal/cli/util` —— **运行时入口与对话循环以本仓库集成后的框架 API 为准**；代码示例中出现的 `google.golang.org/adk/...` 仅为迁移期路径参照（详见 §1、§1.4）。

---

*文档版本：1.2 — **规范对齐 `pkg/trpc-agent-go`/tRPC-Agent-Go**；保留 google/adk-go CLI 分层作工程对照；`aranea/launcher.Config` 内嵌框架 `launcher.Config`；Console Launcher 骨架与 Plugin / Telemetry / 输出风格与原 1.1 一致。*
