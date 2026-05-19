# CLI 终端控制台 — 实现设计文档

> 对应需求：`25 cli.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Aranea CLI：终端命令行工具，支持直接命令模式和交互式对话模式。内置系统管家 Agent，通过自然语言驱动跨模块操作。

CLI 作为独立二进制 `aranea`，与后端 `cmd/admin` 同仓库构建。直接命令模式通过 HTTP 客户端调用后端 REST API；对话模式通过 trpc-agent-go Runner 驱动 Agent 运行。

---

## 二、架构设计

### 2.1 双层入口架构

CLI 采用**双层入口**策略，由 `main.go` 根据第一个参数路由：

```text
os.Args[1]
  ├─ 空参数           → 进入对话模式（默认 REPL）
  ├─ "chat"           → 进入对话模式（显式入口）
  ├─ "version"        → 信息命令
  ├─ "config"         → 配置命令
  ├─ "login"          → 登录命令
  ├─ "completion"     → 补全命令
  ├─ "<资源> <动作>"  → 直接命令模式
  └─ "--help"         → 帮助
```

### 2.2 二进制结构

```text
cmd/aranea/
├── main.go                    # 入口：路由到 Cobra 或 REPL
├── root.go                    # Cobra root command + 全局 flags
├── apiclient/                 # HTTP 客户端封装
│   ├── client.go              # APIClient 结构体
│   └── transport.go           # 认证 / 重试 / 日志中间件
├── output/                    # 输出格式化
│   ├── printer.go             # Printer 接口 + 工厂
│   ├── text.go                # text 格式（带色 + 表格）
│   ├── json.go                # JSON 格式
│   ├── yaml.go                # YAML 格式
│   └── table.go               # table 格式
├── config/                    # 配置管理
│   ├── config.go              # CLIConfig + 加载/保存
│   └── paths.go               # 跨平台路径解析
├── repl/                      # 对话模式
│   ├── repl.go                # REPL 主循环
│   ├── slash.go               # 斜杠命令处理
│   ├── confirm.go             # 二次确认交互
│   ├── spinner.go             # 进度条 / spinner
│   └── history.go             # 会话历史管理
├── agent/                     # agent 子命令
│   └── agent.go
├── skill/                     # skill 子命令
│   ├── skill.go               # CRUD 子命令
│   └── install.go             # install from URL 子命令
├── team/                      # team 子命令
│   └── team.go
├── tool/                      # tool 子命令
│   └── tool.go
├── plugin/                    # plugin 子命令
│   └── plugin.go
├── mcp/                       # mcp 子命令
│   └── mcp.go
├── cron/                      # cron 子命令
│   └── cron.go
├── channel/                   # channel 子命令
│   └── channel.go
├── session/                   # session 子命令
│   └── session.go
├── monitor/                   # monitor 子命令
│   └── monitor.go
├── system/                    # system 子命令
│   └── system.go
└── version/                   # version 子命令
    └── version.go
```

### 2.3 依赖

| 依赖 | 用途 | 说明 |
|------|------|------|
| `github.com/spf13/cobra` | 命令框架 | 直接命令模式 |
| `trpc.group/trpc-go/trpc-agent-go` | Agent 运行时 | 对话模式（Runner / Agent / Event） |
| `github.com/go-kratos/kratos/v2` | 传输层 | 复用现有 HTTP 传输（仅 service 层类型引用） |
| `aranea-agents/internal/agent` | Agent 构建 | 复用 `BuildTRPCLLMAgent` |
| `aranea-agents/internal/biz` | 业务模型 | 复用 Usecase / Repo 接口 |

### 2.4 与现有代码的关系

| 现有代码 | CLI 复用方式 |
|----------|-------------|
| `internal/agent/trpc_build.go` → `BuildTRPCLLMAgent` | 对话模式构建 Agent |
| `internal/agent/trpc_runtime.go` → `NewTRPCRunner` / `RunTRPCUserTurn` | 对话模式运行 Agent |
| `internal/service/*` | 直接命令模式通过 HTTP 调用这些 Service 暴露的 REST API |
| `api/kratos/*/v1/*.proto` | API 契约真相源；CLI HTTP 客户端按 proto 定义调用 |
| `internal/tools/trpc/toolsets.go` → `BuildToolsets` | 系统管家 Agent 的工具装配 |
| `internal/plugin/trpc/audit.go` | CLI / Web 共用审计 Plugin |
| `cmd/araneactl/lint/` | 现有 CLI 工具（lint / fmtcheck），新 CLI 不替代而是共存 |

---

## 三、直接命令模式设计

### 3.1 Cobra 命令树

```text
aranea (root)
├── agent
│   ├── ls
│   ├── get
│   ├── create
│   ├── update
│   ├── delete
│   ├── enable
│   ├── disable
│   ├── run
│   ├── tools
│   └── tools-set
├── team
│   ├── ls
│   ├── get
│   ├── create
│   ├── update
│   ├── delete
│   ├── run
│   ├── runs
│   └── run-events
├── skill
│   ├── ls
│   ├── get
│   ├── create
│   ├── update
│   ├── delete
│   ├── enable
│   ├── disable
│   ├── publish
│   ├── install <url>
│   ├── import <zip-path>
│   ├── import-status <job_id>
│   └── import-apply <job_id>
├── tool
│   ├── ls
│   ├── get
│   ├── enable
│   └── disable
├── plugin
│   ├── ls
│   ├── get
│   ├── enable
│   ├── disable
│   └── config-set
├── mcp
│   ├── ls
│   ├── get
│   ├── add
│   ├── update
│   ├── delete
│   └── test
├── cron
│   ├── ls
│   ├── get
│   ├── add
│   ├── update
│   ├── delete
│   ├── pause
│   ├── resume
│   └── trigger
├── channel
│   ├── ls
│   ├── get
│   ├── add
│   ├── update
│   ├── delete
│   ├── test
│   └── send
├── session
│   ├── ls
│   ├── get
│   └── send
├── monitor
│   ├── audit-logs
│   ├── events
│   └── traces
├── system
│   └── info
├── version
├── config
│   ├── get
│   ├── set
│   ├── edit
│   └── path
├── login
├── completion
└── chat
```

### 3.2 API Client

```go
// cmd/aranea/apiclient/client.go
type APIClient struct {
    baseURL    string
    httpClient *http.Client
    token      string
    output     string
}

func NewAPIClient(baseURL, token string) *APIClient

func (c *APIClient) Do(ctx context.Context, method, path string, body, result any) error

func (c *APIClient) ListAgents(ctx context.Context, req ListAgentsRequest) (*ListAgentsResponse, error)
func (c *APIClient) GetAgent(ctx context.Context, id string) (*Agent, error)
func (c *APIClient) CreateAgent(ctx context.Context, req CreateAgentRequest) (*Agent, error)
func (c *APIClient) UpdateAgent(ctx context.Context, id string, req UpdateAgentRequest) (*Agent, error)
func (c *APIClient) DeleteAgent(ctx context.Context, id string) error
func (c *APIClient) ListSkills(ctx context.Context, req ListSkillsRequest) (*ListSkillsResponse, error)
func (c *APIClient) ImportSkill(ctx context.Context, zipData []byte, metadata ImportMetadata) (*ImportResponse, error)
func (c *APIClient) GetImportStatus(ctx context.Context, jobID string) (*ImportStatus, error)
func (c *APIClient) ApplyImport(ctx context.Context, jobID string, decisions []Decision) error
```

API Client 的请求 / 响应结构体从 proto 定义映射，不直接 import proto 生成代码（避免 CLI 依赖 gRPC 栈），而是手动定义等价的 Go struct。

### 3.3 输出格式化

```go
// cmd/aranea/output/printer.go
type Printer interface {
    PrintList(items []any, total int)
    PrintDetail(item any)
    PrintError(code, message string)
    PrintSuccess(message string)
    PrintKeyValue(pairs ...string)
}

func NewPrinter(format string, colorMode string) Printer
```

| 格式 | 实现 | 说明 |
|------|------|------|
| `text` | `textPrinter` | 带色 + 对齐表格；无 TTY 时降级 |
| `json` | `jsonPrinter` | `json.MarshalIndent`，2 空格缩进 |
| `yaml` | `yamlPrinter` | YAML 序列化 |
| `table` | `tablePrinter` | 纯表格，无色 |

### 3.4 配置管理

```go
// cmd/aranea/config/config.go
type CLIConfig struct {
    Backend  BackendConfig  `toml:"backend"`
    UI       UIConfig       `toml:"ui"`
    Skill    SkillConfig    `toml:"skill"`
    Chat     ChatConfig     `toml:"chat"`
    Telemetry TelemetryConfig `toml:"telemetry"`
}

type BackendConfig struct {
    BaseURL     string `toml:"base_url"`
    Token       string `toml:"token"`
    WorkspaceID string `toml:"workspace_id"`
}

type UIConfig struct {
    Output string `toml:"output"`
    Color  string `toml:"color"`
}

type SkillConfig struct {
    DefaultDecision string `toml:"default_decision"`
    RefineProvider  string `toml:"refine_provider"`
    RefineModel     string `toml:"refine_model"`
    MaxZipMB        int    `toml:"max_zip_mb"`
    KeepTemp        bool   `toml:"keep_temp"`
}

type ChatConfig struct {
    DefaultAgent string `toml:"default_agent"`
    DefaultTeam  string `toml:"default_team"`
    AutoResume   bool   `toml:"auto_resume"`
}

type TelemetryConfig struct {
    Enabled bool `toml:"enabled"`
}

func Load() (*CLIConfig, error)
func (c *CLIConfig) Save() error
func ConfigPath() string
```

配置优先级：`--flag` > 环境变量 > `config.toml` > 默认值。

---

## 四、对话模式设计

### 4.1 REPL 核心循环

对话模式复用 `internal/agent/trpc_runtime.go` 的 `NewTRPCRunner` + `RunTRPCUserTurn`，与 Web Chat 共享同一运行时：

```text
REPL 主循环
  │
  ├─ 读取用户输入（os.Stdin）
  │   ├─ 斜杠命令 → slash.go 处理
  │   └─ 自然语言 → 调用 RunTRPCUserTurn
  │
  ├─ RunTRPCUserTurn 返回 event channel
  │   ├─ event.LLMResponse → 增量打印文本
  │   ├─ event.ToolCall    → 渲染折叠块 + spinner
  │   ├─ event.ToolResult  → 折叠块内追加结果
  │   ├─ event.ToolError   → 红色追加错误
  │   └─ event.Done        → 回到提示符
  │
  └─ 二次确认（高风险操作）
      ├─ BeforeTool Plugin 抛出 ErrAwaitConfirm
      └─ REPL 捕获 → 弹 prompt → 用户确认后 Resume
```

### 4.2 系统管家 Agent 构建

系统管家 Agent 使用 `BuildTRPCLLMAgent` 构建，与普通 Agent 一致，差异仅在 Tool 集和 Instruction：

```go
func BuildSystemAdminAgent(ctx context.Context, deps agent.TRPCBuilderDeps) (trpcagent.Agent, error) {
    adminAgent := biz.Agent{
        AgentKey:    "__system_admin__",
        DisplayName: "系统管家",
        Provider:    deps.Provider,
        Model:       deps.Model,
        SystemPrompt: systemAdminInstruction,
        ToolsProfile: "system_admin",
        ToolsAllowJSON: `["group:cli_admin","web_fetch","read_file","datetime"]`,
        ToolsDenyJSON:  `["shell_exec","write_file","create_image","tts"]`,
    }
    return agent.BuildTRPCLLMAgent(ctx, adminAgent, deps)
}
```

### 4.3 Agent Loader

`dbAgentLoader` 实现 `trpcagent.Agent` 的动态加载，从数据库读取 Agent 配置并构建：

```go
type dbAgentLoader struct {
    agentUC *biz.AgentUsecase
    catalog *biz.LlmProviderModelUsecase
    rt      *provider.RoundTrip
    cache   map[string]trpcagent.Agent
}

func (l *dbAgentLoader) Load(ctx context.Context, agentKey string) (trpcagent.Agent, error) {
    if a, ok := l.cache[agentKey]; ok {
        return a, nil
    }
    rec, err := l.agentUC.GetByKey(ctx, agentKey)
    if err != nil {
        return nil, err
    }
    deps := agent.TRPCBuilderDeps{
        Catalog: l.catalog,
        AgentUC: l.agentUC,
        RT:      l.rt,
        Provider: rec.Provider,
        Model:   rec.Model,
    }
    a, err := agent.BuildTRPCLLMAgent(ctx, rec, deps)
    if err != nil {
        return nil, err
    }
    l.cache[agentKey] = a
    return a, nil
}
```

### 4.4 Session 桥接

CLI 对话模式与 Web Chat 共用 `sessions` / `messages` 表。Session 通过 `internal/session/trpc/sqlite.go` 的 `SQLiteSessionService` 管理。

CLI 在创建 Session 时，在 `metadata_json` 中写入来源标识：

```json
{
  "origin": "cli",
  "cli_version": "1.0.0",
  "cli_terminal": "xterm-256color",
  "cli_os": "windows-amd64"
}
```

### 4.5 WS 流式渲染

对话模式应连接 `/v1/ws` 并发送 `user_message` 上行；非交互或后台模式可调用 `POST /v1/chat/messages`。事件类型沿用 `1 chat.md` 的 Envelope 约定：

| 事件 | 渲染 |
|------|------|
| `message.delta` | 增量打印模型文本 |
| `tool.call` | 折叠块标题 + spinner |
| `tool.result` | 在折叠块内追加结果摘要 |
| `tool.error` | 红色追加错误码 |
| `usage` | 不渲染，写入 `--debug` 输出 |
| `done` | 结束 spinner，回到提示符 |

---

## 五、cli_admin_* Tool 集设计

### 5.1 Tool 实现模式

每个 `cli_admin_*` Tool 实现为 trpc-agent-go 的 `tool.Tool` 接口，通过 `internal/tools/trpc/toolsets.go` 的 `BuildToolsets` 装配到系统管家 Agent。

```go
// internal/tools/cli_admin/skill_list.go
func NewSkillListTool(apiClient *apiclient.APIClient) tool.Tool {
    return tool.NewFunctionTool(
        "cli_admin_skill_list",
        "搜索/分页查询 Skill 列表",
        skillListSchema,
        func(ctx context.Context, input map[string]any) (string, error) {
            req := apiclient.ListSkillsRequest{
                Search:   input["search"].(string),
                Page:     int(input["page"].(float64)),
                PageSize: int(input["page_size"].(float64)),
            }
            resp, err := apiClient.ListSkills(ctx, req)
            if err != nil {
                return "", err
            }
            return formatToolResult(resp)
        },
    )
}
```

### 5.2 Tool 注册

```go
// internal/tools/cli_admin/registry.go
func RegisterAll(apiClient *apiclient.APIClient) []tool.Tool {
    return []tool.Tool{
        NewSkillListTool(apiClient),
        NewSkillInstallFromURLTool(apiClient),
        NewSkillInstallFromPathTool(apiClient),
        NewSkillImportStatusTool(apiClient),
        NewSkillImportApplyTool(apiClient),
        NewSkillRefineConflictTool(apiClient),
        NewSkillEnableTool(apiClient),
        NewSkillDisableTool(apiClient),
        NewSkillDeleteTool(apiClient),
        NewAgentListTool(apiClient),
        NewAgentGetTool(apiClient),
        NewAgentCreateTool(apiClient),
        NewAgentUpdateTool(apiClient),
        NewAgentDeleteTool(apiClient),
        NewAgentToolsGetTool(apiClient),
        NewAgentToolsSetTool(apiClient),
        NewTeamListTool(apiClient),
        NewTeamCreateTool(apiClient),
        NewTeamUpdateTool(apiClient),
        NewTeamDeleteTool(apiClient),
        NewTeamRunTool(apiClient),
        NewToolListTool(apiClient),
        NewToolEnableTool(apiClient),
        NewToolDisableTool(apiClient),
        NewToolConfigSetTool(apiClient),
        NewPluginListTool(apiClient),
        NewPluginEnableTool(apiClient),
        NewPluginDisableTool(apiClient),
        NewPluginOrderSetTool(apiClient),
        NewPluginConfigSetTool(apiClient),
        NewMCPListTool(apiClient),
        NewMCPAddTool(apiClient),
        NewMCPUpdateTool(apiClient),
        NewMCPDeleteTool(apiClient),
        NewMCPTestTool(apiClient),
        NewCronListTool(apiClient),
        NewCronAddTool(apiClient),
        NewCronUpdateTool(apiClient),
        NewCronDeleteTool(apiClient),
        NewCronPauseTool(apiClient),
        NewCronResumeTool(apiClient),
        NewCronTriggerTool(apiClient),
        NewChannelListTool(apiClient),
        NewChannelAddTool(apiClient),
        NewChannelUpdateTool(apiClient),
        NewChannelDeleteTool(apiClient),
        NewChannelTestTool(apiClient),
        NewChannelSendTool(apiClient),
        NewProviderListTool(apiClient),
        NewProviderAddTool(apiClient),
        NewProviderUpdateTool(apiClient),
        NewProviderDeleteTool(apiClient),
        NewProviderInspectTool(apiClient),
        NewSessionListTool(apiClient),
        NewSessionGetTool(apiClient),
        NewSessionSendTool(apiClient),
    }
}
```

### 5.3 二次确认机制

高风险 Tool 调用通过 ADK Plugin 的 `BeforeTool` 钩子拦截：

```go
// internal/plugin/trpc/confirm.go
type ConfirmPlugin struct {
    highRiskTools map[string]bool
}

func (p *ConfirmPlugin) BeforeTool(ctx context.Context, toolName string, args map[string]any) error {
    if !p.highRiskTools[toolName] {
        return nil
    }
    if autoYes, _ := ctx.Value(ctxKeyAutoYes).(bool); autoYes {
        return nil
    }
    return ErrAwaitConfirm
}
```

REPL 捕获 `ErrAwaitConfirm` 后弹出交互确认，用户输入 `y` 后通过 `runner.Resume` 继续。

### 5.4 Skill Install from URL 工具

`cli_admin_skill_install_from_url` 是最复杂的 Tool，编排整个 §4 流程：

```go
func NewSkillInstallFromURLTool(apiClient *apiclient.APIClient) tool.Tool {
    return tool.NewFunctionTool(
        "cli_admin_skill_install_from_url",
        "从 Git URL 安装 Skill",
        skillInstallSchema,
        func(ctx context.Context, input map[string]any) (string, error) {
            url := input["url"].(string)
            ref, _ := input["ref"].(string)
            subpath, _ := input["subpath"].(string)

            stepEmit := getStepEmitter(ctx)

            stepEmit("解析 URL", func() (string, error) {
                return parseGitURL(url)
            })

            stepEmit("下载仓库", func() (string, error) {
                return gitCloneShallow(url, ref, tmpDir)
            })

            stepEmit("定位 SKILL.md", func() (string, error) {
                return locateSkillRoot(tmpDir, subpath)
            })

            stepEmit("本地预校验", func() (string, error) {
                return validateSkillDir(skillDir)
            })

            stepEmit("打包 zip", func() (string, error) {
                return packZip(skillDir, zipPath)
            })

            stepEmit("上传到后端", func() (string, error) {
                return apiClient.ImportSkill(ctx, zipData, metadata)
            })

            stepEmit("轮询导入状态", func() (string, error) {
                return pollImportStatus(ctx, apiClient, jobID)
            })

            return formatInstallResult(result)
        },
    )
}
```

---

## 六、后端契约

CLI 默认完全复用现有 `/api/v1/*` REST API。仅以下接口需要新增或扩展：

### 6.1 系统信息（新增）

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

### 6.2 系统管家 Agent 种子数据

| 时机 | 行为 |
|------|------|
| 后端启动 | `SeedSystemAdminAgent()`：插入或更新 `agent` 表中 `agent_key=__system_admin__` 记录，`readonly=1`，`category=system`，`tools_profile=system_admin`，`tools_allow_json=["group:cli_admin","web_fetch","read_file","datetime"]` |
| 后端启动 | `SeedBuiltinTools()`：注册所有 `cli_admin_*` 工具，分类 `system`，`source=builtin` |
| 后端启动 | `SeedToolGroups()`：建立 `group:cli_admin` 工具组 → 包含全部 `cli_admin_*` |

### 6.3 Skill 导入扩展

`POST /api/v1/skills/import` 增加可选字段：

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

后端在 `tool_invocations.metadata_json` 与 `audit_logs` 中保留 `source_url` 与 `source_ref`。

### 6.4 错误格式

复用 `20 skill.md` / `23 tools.md` 的统一错误结构。CLI 在终端再封装一层：把 `error.code` 翻译成中文短句（可选），但 `--output json` 时原样透传。

---

## 七、数据模型变更

CLI 主要复用现有数据模型。新增 / 扩展项：

### 7.1 `agent` 表

| 字段 | 变更 | 说明 |
|------|------|------|
| `readonly` | 新增 `INTEGER NOT NULL DEFAULT 0` | 标记 `__system_admin__` 等内置不可改不可删的 Agent |
| `kind` | 新增 `TEXT NOT NULL DEFAULT 'user'` | 枚举 `user` / `system`；前端按 `system` 区分展示 |

### 7.2 `tool_invocations` 表

| 字段 | 变更 | 说明 |
|------|------|------|
| `source` | 枚举扩展：`adk` / `manual` / `system` / `cli` | CLI 调用标记为 `cli` |
| `metadata_json` | 增加约定字段 `cli.terminal`、`cli.version`、`cli.os` | 便于排障 |

### 7.3 `sessions` 表

不新建表。CLI 端会话通过 `metadata_json` 标记来源：

```json
{
  "origin": "cli",
  "cli_version": "1.0.0",
  "cli_terminal": "xterm-256color",
  "cli_os": "windows-amd64"
}
```

### 7.4 本地存储

CLI 本地文件结构：

```text
~/.aranea/
├── config.toml                # 配置文件
├── sessions/                  # 会话历史
│   └── <session_id>.jsonl     # 每个会话一行一条消息
├── logs/                      # 日志
│   └── cli-2026-05-19.log
└── tmp/                       # 临时文件（Skill 安装等）
    └── <job_id>/
```

---

## 八、Wire 注入

CLI 为独立二进制，不通过 Wire 注入。但 CLI 对话模式需要手动组装依赖：

```text
CLI main.go
  ├─ 直接命令模式
  │   └─ APIClient（HTTP 客户端，无需 Wire）
  │
  └─ 对话模式
      ├─ 打开 SQLite（复用 internal/data 的 NewData 逻辑）
      ├─ 初始化各 Usecase（手动 new，不走 Wire）
      ├─ 构建 SystemAdminAgent（BuildTRPCLLMAgent）
      ├─ 构建 Runner（NewTRPCRunner）
      └─ 启动 REPL
```

---

## 九、构建与发布

### 9.1 Makefile 扩展

```makefile
.PHONY: cli
cli:
	go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/aranea ./cmd/aranea/
```

### 9.2 跨平台构建

```makefile
.PHONY: cli-all
cli-all:
	GOOS=windows GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/aranea-windows-amd64.exe ./cmd/aranea/
	GOOS=darwin GOARCH=arm64 go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/aranea-darwin-arm64 ./cmd/aranea/
	GOOS=linux GOARCH=amd64 go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/aranea-linux-amd64 ./cmd/aranea/
```

---

*文档版本：2.0 — 对齐实际项目结构（module `aranea-agents`、框架 `trpc.group/trpc-go/trpc-agent-go`）；需求与设计分离。*
