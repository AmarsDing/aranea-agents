# 25 CLI 模块实施方案（2026-05-27）

> **状态**：v1.1（取代 `25-cli.development.md` 的开发计划部分；需求 `25-cli.md` 与设计 `25-cli.design.md` 中与本方案冲突的细节以本文档为准）。
> **作者口径**：本方案对 `25-cli.md` / `25-cli.design.md` / `25-cli.development.md` 三份既有材料做对仓库实际代码的对账，剔除假设字段、对齐框架红线，并把 MVP 收窄到能在 1～2 周交付的体量。
> **同系列**：需求 → [`25-cli.md`](./25-cli.md)；设计 → [`25-cli.design.md`](./25-cli.design.md)；开发计划 → [`25-cli.development.md`](./25-cli.development.md)

---

## 0. 概要

Aranea CLI（二进制 `aranea`）是一只与 `cmd/admin` 后端**完全异构**的终端控制台：它**只**通过后端的 HTTP / WS API 操作系统，不在进程内启动 Agent Runner、不读写数据库、不 import `internal/biz`/`internal/agent`/`pkg/trpc-agent-go`。CLI 同时提供「直接命令」（脚本化）与「对话模式」（与后端 WS `/v1/ws` 上的"系统管家"Agent 自然语言交互）两种入口。

**与原方案的关键差异（本次必须对齐的 8 条）**

1. **REST 路径前缀全文改为 `/v1/*`**（不是 `/api/v1/*`）。仓库内所有 proto 的 `google.api.http` 注解都是 `/v1/...`，原文档示例会 404。
2. **删除 `workspace_id`**。`pkg/auth.Auth` 当前只有 `UserID + Access` 两个字段，仓库不存在 workspace/tenant 概念；CLI 配置只保留 `base_url + token`，多租户字段标"预留"。
3. **登录路径走真实接口** `/v1/admins/login`（`pkg/auth/middleware.go` 已把它放进 `noAuthPaths`）。原文档把 `login` 视为占位。
4. **对话模式只走后端 WS**：CLI 进程内**不**构建 `trpcrunner.Runner` / `dbAgentLoader` / SessionService。原设计 §8 "在 CLI main 里手动组装 Runner" **违反** `trpc-agent-framework-first.mdc`（Runner 装配只在 `internal/service`）。
5. **CLI 二进制依赖白名单收窄**：仅允许 import `api/kratos/*/v1` 生成代码、`pkg/auth`（仅常量/工具）、`pkg/safego` 这类纯库；**禁止** import `internal/biz`、`internal/data`、`internal/agent`、`internal/server`、`pkg/trpc-agent-go`。原设计 §2.3 错把 biz/agent 列为 CLI 依赖。
6. **不再"手抄请求/响应 struct"**：原设计 §3.2 为"避开 gRPC 栈"手抄了一份；实际生成的 `*.pb.go` 仅依赖 `google.golang.org/protobuf`，CLI 直接复用 pb 类型即可，避免双份维护。
7. **二次确认复用 `AwaitUserReply`**：`api/kratos/chat/v1/chat.proto` 已有 `AwaitUserReply` RPC + 配套 WS 事件，新方案直接走它；不再新增 `ConfirmPlugin`。
8. **Skill 导入字段走 `metadata_json`，不动 proto 服务**：proto 里 `SkillImportJob` / `ApplySkillImportRequest` / `RefineSkillImportConflictRequest` 已齐全，仅需在 multipart 表单里塞 `source / source_url / source_ref / source_subpath / client_validation` 写入 `metadata_json`。原 §6.3 "扩展 proto 字段"成本远大于收益。

**与其他在做项的边界（一句话每条）**

- `1 chat` / `10 session`：CLI 对话模式与 Web Chat 共用 WS `/v1/ws` + `sessions/messages` 表，CLI 仅在新建 session 的 `metadata_json` 标 `origin=cli`。
- `20 skill`：CLI 是"另一个客户端"，不影响后端导入状态机；只在 multipart form 里追加来源字段。
- `23 tools` / `22 plugin`：CLI 仅消费 `/v1/tools` / `/v1/plugins`；`cli_admin_*` 工具集是**后端侧**的内置工具组（落在 `internal/tools/cli_admin/`，由 service 层装配到系统管家 Agent），不在 CLI 二进制里实现。
- `cmd/araneactl/`（lint/fmtcheck）：开发者工具链，与终端用户 CLI `aranea` 共存，互不替代。

---

## 1. 现状盘点

### 1.1 后端可复用能力

| 模块 | 现状 | CLI 复用方式 |
|------|------|--------------|
| HTTP API（Kratos v2） | 35 个 proto 文件覆盖 agent/team/skill/tool/plugin/mcp_server/cron/channel/session/monitor/chat/llm_provider_model/... | CLI 通过 `net/http` + pb JSON 调用 |
| WS 流 | `internal/event/contract/envelope.go` 定义 envelope 类型；`tool_call` / `tool_result` 已实现；`tool.error` 缺失（使用通用 `error` 类型） | CLI 对话模式唯一通道 |
| Chat HTTP | `/v1/chat/messages`（send）、`/v1/chat/options`、`/v1/chat/pending`、`/v1/chat/run-status`、`/v1/chat/jobs`、`/v1/chat/await-reply`、`/v1/chat/messages/enqueue`、`/v1/chat/messages/{id}/feedback` | 非交互/CI 退化模式调用 |
| Skill 导入 | `POST /v1/skills/import`（multipart，`RegisterSkillImportMultipart`）；`SkillImportJob` / `ApplySkillImportRequest` / `RefineSkillImportConflictRequest` 已 proto-first | CLI `skill install/import/import-status/import-apply` 直接调用 |
| 鉴权 | `pkg/auth`：JWT bearer + cookie 双轨；`/v1/admins/login` 颁发 token；`KRATOS_HTTP_AUTH_DISABLED=1` 本地 bypass | CLI 走 bearer header；同 dev bypass 兼容 |
| Agent runtime | `internal/agent/trpc_build.go::BuildTRPCLLMAgent`、`internal/agent/trpc_runtime.go::NewTRPCRunner` / `RunTRPCUserTurn*` | **仅在后端 service 层调用**，CLI 不直接 import |
| 系统种子机制 | **已实现**：`internal/data/seed_system_admin.go` 含 SeedSystemAdminAgent 等 8 个 seed 函数 | CLI 不参与 seed |
| 系统信息 | **已实现**：`internal/service/system_info.go` + 手动注册路由；但缺少 `system_admin_agent_id/key`、`skill_max_zip_mb` 字段 | CLI `aranea system info` 消费 |
| cli_admin 工具集 | **已实现**：`internal/tools/cli_admin/` 含 registry + agent_tools + skill_install + pkg_install | CLI 不直接 import；通过 WS 对话间接调用 |

### 1.2 CLI 相关遗产

| 项 | 路径 | 与新 CLI 的关系 |
|----|------|----------------|
| 开发者 lint | `cmd/araneactl/lint/main.go`（stdlib `flag`） | **不存在**：`cmd/araneactl/` 目录未创建，R12 lint 规则缺失 |
| fmtcheck | `cmd/araneactl/fmtcheck/main.go` | **不存在**：同上 |
| 一次性数据 CLI | `cmd/fetch-channel-icons`、`cmd/memory-migrate`、`cmd/sqlmigrate`、`cmd/seed-stockx-org`、`cmd/pginit`、`cmd/pgprobe` | 一次性数据维护工具，与终端用户 CLI 无关 |
| Cobra/spf13 依赖 | **已引入**：`github.com/spf13/cobra v1.10.2` | — |
| `cmd/aranea/` | **已实现** | 完整 cobra CLI 入口 |
| `internal/cli/` | **已实现** | 7 子包：client/clierr/cmd/config/output/repl/ui |

### 1.3 待解决的 gap

1. ~~没有终端用户 CLI 入口，无任何 cobra 集成。~~ **已解决**：`cmd/aranea/` + `internal/cli/` 已实现。
2. ~~没有"系统管家 Agent"种子~~ **已解决**：`internal/data/seed_system_admin.go` 已实现。
3. ~~没有 `cli_admin_*` 工具集~~ **部分解决**：`internal/tools/cli_admin/` 已有首批工具；剩余 team/plugin/mcp/cron/channel/provider/session 工具待实现。
4. ~~`/v1/system/info` 接口不存在~~ **已解决**，但字段有差距：缺少 `system_admin_agent_id/key`、`skill_max_zip_mb`。
5. Skill import multipart 表单当前不支持 `source / source_url / source_ref / source_subpath / client_validation` 字段（待确认）。
6. WS 协议层 `tool.error` envelope 子类型未定义（当前使用通用 `error` 类型）。
7. ~~没有跨平台二进制发布脚本~~ **部分解决**：`make cli` 已实现，但 `cli-all` 仅覆盖 Linux/amd64。
8. `cmd/araneactl/` 目录不存在，R12 lint 规则缺失——CLI 红线无 CI 守护。
9. CLI 子命令测试覆盖不足：`cmd/` 下仅 `config_test.go`，其他子命令缺 httptest。
10. `docs/guides/cli-quickstart.md` 未创建。

---

## 2. 目标与非目标

### 2.1 必做（MVP，P0）

- 单一二进制 `aranea`（Win / macOS / Linux 三平台 amd64+arm64，`CGO_DISABLED=1`）。
- 直接命令模式覆盖 4 类资源 + 配套基础设施：`agent`、`skill`、`tool`、`system`（见 §4 MVP 命令清单）。
- 全局 flag：`--base-url` / `--token` / `--output` / `--quiet` / `--debug` / `--yes`。
- 输出格式：`text`（默认，TTY 检测后带色/表格；管道降级 key=value）、`json`（pb json marshal）。
- 配置管理：`~/.aranea/config.toml`（Windows `%APPDATA%\aranea\config.toml`），`config get/set/path`。
- 登录命令：`aranea login --base-url ... --user ... --password ...`，落 token 到 config.toml（文件权限 0600）。
- 后端新增 `GET /v1/system/info`。

### 2.2 应做（P1）

- 资源命令补齐：`team`、`plugin`、`mcp`、`cron`、`channel`、`session`、`monitor`。
- 对话模式：`aranea` / `aranea chat`，仅走 WS `/v1/ws`；流式渲染 message delta；二次确认走 `AwaitUserReply`。
- 系统管家 Agent 后端种子（`__system_admin__`）+ `cli_admin_*` 工具集后端注册（`internal/tools/cli_admin/`）。
- Skill 安装 from URL：`aranea skill install <url>`（CLI 完成 git clone / 子目录定位 / 本地预校验 / 打包 zip / 上传），后端 multipart 接收 `source*` 字段写入 `metadata_json`。
- Shell 补全：`aranea completion bash/zsh/fish/powershell`（cobra 自带）。

### 2.3 不做（本期 OUT-OF-SCOPE）

- `workspace_id` / 多租户切换。
- 远程 OIDC / SSO 登录（沿用现有 `/v1/admins/login`）。
- 插件式第三方 CLI 命令（`aranea` 不加载外部代码）。
- 自更新 / auto-upgrade（仅 `aranea version` 打印升级指引）。
- TUI 框架（bubbletea 等）。
- CLI 进程内嵌后端 / Runner（无后端模式）。
- 把 `internal/agent` / `internal/biz` 暴露给 CLI 二进制。

---

## 3. 总体架构

### 3.1 二进制 / 安装 / 配置

| 项 | 选择 | 依据 |
|----|------|------|
| 二进制名 | `aranea` | 与 `25 cli.md §0.2` 一致；`arn` 仅作 PATH 别名 |
| 入口包 | `cmd/aranea/` | 与既有 `cmd/admin/`、`cmd/araneactl/` 命名风格一致 |
| 安装 | 手动下载二进制 + PATH；`make cli` / `make cli-all` 本地产物到 `./bin/` | 跨平台、无 CGO，零运行时依赖 |
| 配置路径 | `$XDG_CONFIG_HOME/aranea/config.toml`（Linux/mac）；`%APPDATA%\aranea\config.toml`（Win） | `os.UserConfigDir()`，避开 `$HOME` 跨平台坑 |
| 临时目录 | `os.UserCacheDir()/aranea/tmp/<job_id>/`；安装成功后清理（`config.skill.keep_temp=true` 时保留） | 与配置目录分离 |
| 日志 | `os.UserCacheDir()/aranea/logs/cli-YYYY-MM-DD.log`，按天切割，单文件 ≤ 5MB；`--debug` 同时输出到 stderr | |

### 3.2 与后端通信

| 模式 | 选择 | 依据 |
|------|------|------|
| **REST**（直接命令） | `net/http` + Kratos 生成的 pb JSON 类型 | pb JSON 是后端唯一对外契约；CLI 直接 import `api/kratos/*/v1` 生成代码，零额外维护成本 |
| **WS**（对话模式） | `github.com/gorilla/websocket` 连 `/v1/ws`，使用 `wsUpstream` / `wsDownstream` JSON envelope | 与 Web 客户端共用同一协议；后端已实现完整生命周期 |
| 不选 gRPC | — | 跨平台 + 防火墙穿透 + 现有 HTTP 中间件链（auth/audit/CORS）已就绪；gRPC 无收益 |
| 不选 SDK 直连 | — | 违反"CLI 不直连 DB / 不 import biz/agent"红线 |

> **红线重申**：CLI 二进制只 import `api/kratos/*/v1`（pb）、`pkg/auth`（仅复用 `cookieNameFromEnv` / token 解析常量这类纯函数）、`pkg/safego` 等无副作用包；**严禁** import `internal/biz`、`internal/data`、`internal/agent`、`internal/server`、`pkg/trpc-agent-go`。

### 3.3 鉴权与多工作区

- 配置 `[backend].token` 保存 JWT；CLI 在每个 HTTP 请求 `Authorization: Bearer <token>`；WS 在 query 或 Sec-WebSocket-Protocol 子协议中携带（与 `internal/server/ws.go` 现有读取方式对齐，**实施时需对照 `ws.go` 读 token 的位置确认**）。
- `--token` flag 与环境变量 `ARANEA_TOKEN` 覆盖配置文件值。
- Dev bypass：检测 `KRATOS_HTTP_AUTH_DISABLED=1` 时 CLI 跳过 token 强校验（仅打印 WARN）。
- **多工作区暂不实现**：`config.backend.workspace_id` 字段保留为空字符串占位；后续接入时改默认值即可，向后兼容。

### 3.4 输出格式、退出码、日志级别

| 项 | 规则 |
|----|------|
| `--output text` | TTY：带色 + 表格（`tablewriter` 或自实现）；非 TTY：`key=value` 单行 |
| `--output json` | `protojson.Marshal`，2 空格缩进；`--quiet` 时单字段（如 ID） |
| `--output yaml` | P1 引入 `gopkg.in/yaml.v3`；MVP 不做 |
| 退出码 | 0=成功；1=参数错误；2=API 错误（非 5xx）；3=网络错误；4=用户取消；5=校验/冲突阻塞（对齐 `25 cli.md §9.3`）；130=Ctrl+C |
| 日志级别 | `--debug` 才打开 HTTP 请求/响应详情；其余沉默 |
| 错误展示 | text 模式下：红色 `code` + 黄色 `hint`；json 模式下原样透传后端 `error.code` / `error.message` / `error.metadata` |

### 3.5 离线 / 弱网 / CI 退化

- **离线**：除 `aranea config *` / `aranea version`（仅本地）外，所有命令需联网；离线时返回退出码 3。
- **弱网**：HTTP 默认重试 3 次（指数退避 200ms→2s），只对幂等动作（`ls`/`get`/`status`）启用；写操作不重试。
- **CI（非 TTY）**：自动关闭 spinner / 色；`text` 退化为 `key=value`；`skill install` 遇到 warn 且无 `--decision` 时 exit 5（沿用 `25 cli.md §9.3`）。
- **后端不可达**：`aranea version` 仍能输出本地版本；其余命令打印"后端不可达：<base_url>"并 exit 3。

---

## 4. 命令树（落到子命令颗粒度）

### 4.1 顶层分组

```text
aranea
├── (无参数)                    # 进入对话模式 REPL（P1）
├── chat [...]                  # 显式进入对话模式（P1）
├── version                     # 版本信息（含后端 reachability）
├── login [...]                 # 登录并写 token
├── config <get|set|path>       # 配置管理
├── completion <shell>          # 自动补全脚本（P1，cobra 自带）
├── system <info>               # 系统信息
├── agent <ls|get|create|update|delete|enable|disable|tools|tools-set>
├── skill <ls|get|create|update|delete|enable|disable|publish|install|import|import-status|import-apply>
├── tool  <ls|get|enable|disable>
├── team  <ls|get|create|update|delete|run|runs|run-events>           # P1
├── plugin <ls|get|enable|disable|order-set|config-set>               # P1
├── mcp    <ls|get|add|update|delete|test>                            # P1
├── cron   <ls|get|add|update|delete|pause|resume|trigger>            # P1
├── channel <ls|get|add|update|delete|test|send>                      # P1
├── session <ls|get|send>                                             # P1
└── monitor <audit-logs|events|traces>                                # P1
```

### 4.2 全局 flag（cobra `PersistentFlags`）

| Flag | 类型 | 说明 |
|------|------|------|
| `--base-url` | string | 覆盖 `[backend].base_url`，默认 `http://127.0.0.1:8080` |
| `--token` | string | 覆盖 `[backend].token` |
| `--output` / `-o` | enum: `text\|json` | MVP 只两种；`yaml/table` P1 |
| `--quiet` / `-q` | bool | 仅输出关键字段（list→每行一个 ID） |
| `--yes` / `-y` | bool | 跳过二次确认 |
| `--debug` | bool | HTTP 请求/响应详情到 stderr |
| `--config` | string | 自定义配置文件路径 |
| `--no-color` | bool | 强制关闭色；同 `NO_COLOR=1` env |

### 4.3 MVP 子命令清单（P0，实施目标 = 第 1 周交付）

```text
aranea version
aranea login --base-url <url> --user <name> --password <pwd>
aranea config get [<key>]
aranea config set <key> <value>
aranea config path
aranea system info

aranea agent ls   [--page N --page-size N --search ...]
aranea agent get  <id>
aranea agent create --file agent.yaml
aranea agent update <id> --file agent.yaml
aranea agent delete <id> [--yes]
aranea agent enable  <id>
aranea agent disable <id>
aranea agent tools     <id>
aranea agent tools-set <id> --allow ... --deny ...

aranea skill ls
aranea skill get     <id>
aranea skill create  --file skill.yaml
aranea skill update  <id> --file skill.yaml
aranea skill delete  <id> [--yes]
aranea skill enable  <id>
aranea skill disable <id>
aranea skill publish <id>

aranea tool ls
aranea tool get     <id>
aranea tool enable  <id>
aranea tool disable <id>
```

### 4.4 扩展集（P1，第 2~3 周）

- 对话模式：`aranea`、`aranea chat [--session <id>] [--agent <key>] [--team <key>]`
- Skill 安装链路：`aranea skill install <url> [--ref --subpath --decision --keep-temp]`、`aranea skill import <zip>`、`aranea skill import-status <job_id>`、`aranea skill import-apply <job_id>`
- 剩余资源命令：`team / plugin / mcp / cron / channel / session / monitor` 全套
- `aranea completion <shell>`
- `--output yaml`、`--output table`

### 4.5 与原方案命令树的差异

| 项 | 原 `25 cli.md §2` | 本方案 |
|----|-------------------|--------|
| 路径 | `/api/v1/...` | `/v1/...`（对齐 proto） |
| `aranea provider` | 列在 §5.2 Tool 表里，命令树未列 | 不进 MVP；P1 用 `aranea ls llm-provider-models`（与 proto `LlmProviderModelService` 对齐） |
| `aranea init` | §10 待确认 | 本期不做（避免在首启时偷偷请求/连后端） |
| `aranea upgrade` | §0.1 提到 | 本期仅 `version` 输出"如何升级"指引 |
| `--output yaml/table` | MVP 默认 4 种 | MVP 两种（text/json）；yaml/table 推到 P1 |

---

## 5. 代码组织

### 5.1 `cmd/aranea/`

```text
cmd/aranea/
├── main.go                # 入口；调 internal/cli.Execute()
└── version.go             # build-time ldflags: -X main.Version / -X main.Commit
```

`cmd/aranea/main.go` 只 ~30 行：解析最早期 flag、初始化日志、把控制权交给 `internal/cli.Execute(ctx)`。**所有真实逻辑都在 `internal/cli/` 下**，这样：

- 单测可在 `internal/cli/...` 里写（`cmd/` 不便测）；
- `go vet` / `golangci-lint` 的包级规则更容易约束；
- `internal/` 自动阻止外部 module 引用。

### 5.2 `internal/cli/` 子结构

```text
internal/cli/
├── root.go                # cobra root command + 全局 flags + 子命令注册
├── execute.go             # Execute(ctx) — 被 cmd/aranea/main.go 调用
├── ctx.go                 # 全局 *Context（持有 config、logger、APIClient）
│
├── config/                # CLIConfig 加载/保存/路径
│   ├── config.go
│   ├── paths.go
│   └── config_test.go
│
├── client/                # HTTP / WS 客户端封装
│   ├── http.go            # Doer 接口 + bearer/重试/--debug 日志
│   ├── ws.go              # WS 连接 + envelope 编解码（P1）
│   ├── errors.go          # 后端 error 解码 → 退出码映射
│   └── client_test.go     # 用 httptest 跑契约测试
│
├── output/                # Printer 接口
│   ├── printer.go
│   ├── text.go
│   ├── json.go
│   └── output_test.go     # golden file
│
├── ui/                    # TTY 检测、spinner、色、表格、prompt
│   ├── tty.go
│   ├── spinner.go
│   ├── color.go
│   ├── table.go
│   └── prompt.go          # 二次确认 / 多选
│
├── repl/                  # 对话模式（P1）
│   ├── repl.go            # 主循环
│   ├── slash.go           # /help /agent /session /yes ...
│   ├── render.go          # WS 事件 → 终端折叠块
│   └── history.go         # 行编辑 + 历史
│
└── cmd/                   # 一个资源一个文件
    ├── version.go
    ├── login.go
    ├── config_cmd.go
    ├── system.go
    ├── agent.go
    ├── skill.go
    ├── skill_install.go   # P1
    ├── tool.go
    ├── team.go            # P1
    ├── plugin.go          # P1
    ├── mcp.go             # P1
    ├── cron.go            # P1
    ├── channel.go         # P1
    ├── session.go         # P1
    └── monitor.go         # P1
```

**约定**：

- `internal/cli/cmd/*.go` 一个文件一个资源；每个资源内子命令以 `runAgentLs(cmd, args)` 风格的 handler 函数实现，handler 只组装请求 + 调 `client.HTTP.Do(...)` + 调 `output.Printer.Print(...)`，**不做业务判断**。
- `client/http.go` 通过 `interface { Do(req *http.Request) (*http.Response, error) }` 注入，便于 httptest。
- 错误统一在 `client/errors.go` 解码为 `*CLIError{Code int; HTTPStatus int; Err error}`，再由 root 的 `RunE` 包装为退出码。

### 5.3 `internal/cli/client/` SDK 封装

- 直接 import `api/kratos/<svc>/v1` 生成的 `*Request`/`*Response` 结构体，用 `encoding/protojson` 编解码。
- 不另写一份"等价 Go struct"（与原设计 §3.2 偏离）。
- 文件按服务一份：`client/agent.go`、`client/skill.go`、`client/tool.go`、...；每个函数签名形如：

  ```go
  func (c *Client) ListAgents(ctx context.Context, req *agentv1.ListAgentsRequest) (*agentv1.ListAgentsResponse, error)
  ```

- Skill 上传：单独 `client/skill_import.go`，用 `mime/multipart`，字段：`file`（zip） + `source` / `source_url` / `source_ref` / `source_subpath` / `client_validation`（后五者由后端 §6 改动写入 `metadata_json`）。

### 5.4 与 `internal/server` / `internal/service` 的边界

**红线重申（必须在 lint 中加规则）**：

| 来源 | 禁止 import |
|------|-------------|
| `cmd/aranea/**` | `internal/biz`、`internal/data`、`internal/agent`、`internal/server`、`internal/service`、`pkg/trpc-agent-go` |
| `internal/cli/**` | 同上 |
| `internal/tools/cli_admin/**`（CLI 后端工具集） | `cmd/aranea`、`internal/cli` |
| `internal/biz/**` | `pkg/trpc-agent-go`（既有红线） |
| `internal/server/**` | `pkg/trpc-agent-go`（Runner 装配只在 `internal/service`） |

实施建议：在 `cmd/araneactl/lint/main.go` 现有 R1-R11 之外新增 R12："`cmd/aranea/` 与 `internal/cli/` 不得 import 上述黑名单"，CI 失败即拦截。

---

## 6. 关键交互

### 6.1 流式输出（chat/run）

对话模式（P1）通过 WS `/v1/ws` 接收下行事件，按 `wsDownstream.Type` 渲染：

| 事件 type | CLI 渲染 |
|----------|----------|
| `message.delta`（或现有 system + payload 中的 chat delta） | 增量打印模型文本，沿用当前光标行 |
| `tool.call` | 起新行 `▼ <tool>(args=...)`，启动 spinner |
| `tool.result` | 折叠块内追加摘要，✓ 标记，停 spinner |
| `tool.error` | 折叠块内追加红色错误码与 message，✗ 标记 |
| `await.user.reply`（`AwaitUserReply` 协议） | 弹出 prompt，把用户回复通过 `EnqueueUserMessage` 上行 |
| `system.done` / `done` | 结束本轮，回到 `aranea> ` 提示符 |

**上下行 envelope 范例**（基于 `internal/server/ws.go` 的 `wsUpstream` / `wsDownstream` 结构）：

```json
// 上行：用户消息
{
  "direction": "client",
  "channel":   "chat",
  "type":      "user.message",
  "request_id":"req_01HXYZ",
  "payload":   {
    "session_id": "sess_abc",
    "agent_key":  "__system_admin__",
    "content":    "帮我把 figma-code-connect 装上"
  }
}

// 下行：工具调用开始（建议后端补的子类型）
{
  "direction": "server",
  "channel":   "chat",
  "type":      "tool.call",
  "payload":   {
    "tool_name": "cli_admin_skill_install_from_url",
    "tool_call_id": "tc_001",
    "arguments": { "url": "https://github.com/.../figma-code-connect" }
  }
}

// 下行：等待用户确认（复用 chat.proto 的 AwaitUserReply）
{
  "direction": "server",
  "channel":   "chat",
  "type":      "await.user.reply",
  "payload":   {
    "prompt":     "确认安装 figma-code-connect？(y/N)",
    "expect":     "yesno",
    "request_id": "await_001"
  }
}
```

> **细节假设**：现行 `internal/server/ws.go` 是否已逐事件类型暴露 `tool.call/result/error` envelope 需要在实施前验证；若仅有粗粒度 system + payload，需要后端先补 envelope（建议落到 `internal/service/chat_wire.go`），不要在 CLI 里反向解析 payload。这一假设由 R1 风险跟踪。

### 6.2 长任务进度（HTTP 轮询）

非对话模式（`aranea skill install <url>`、`aranea skill import-status`）走 HTTP 轮询：

- 间隔：1.5s × 80 次（与原 §4.2 一致，但 cap 在 120s）。
- 渲染：在终端固定一行刷新 `[step 4/6] 上传到后端  ⠋ 已用 8.2s`，spinner 用 ASCII 字符避免 Windows 控制台问题。
- 超时回退：提示 `aranea skill import-status <job_id>` 手工查。

### 6.2.1 Skill install 状态机（CLI 端）

```text
parse_url
   │
   ▼
resolve_ref ── (HEAD / 显式 ref) ──┐
                                   │
                                   ▼
                              fetch (go-git shallow clone OR wget zip)
                                   │
                                   ▼
                          locate_skill_root  (§4.3 of 25 cli.md)
                                   │
                                   ▼
                          local_validate     (§4.4)
                                   │
                                   ▼
                              pack_zip
                                   │
                                   ▼
                   POST /v1/skills/import   (multipart)
                                   │
                                   ▼
              poll  GET /v1/skills/import/{job_id}  (1.5s × 80, cap 120s)
                ┌───────────────────┼─────────────────┐
                ▼                   ▼                 ▼
              pass               warn              block
                │                   │                 │
                ▼                   ▼                 ▼
         POST .../apply      ask user / --decision  exit 5
                                    │
                                    ▼
                          (skip|keep|refine) → apply
```

每个状态在 CLI 输出固定为 `[step N/6] <name>  <spinner|✓|⚠|✗>` 单行；状态间错误统一捕获为 `*CLIError` 经 `cmd.RunE` 转退出码。

### 6.3 错误展示（human + json schema）

- **text 模式**：
  ```
  Error  SKILL_IMPORT_BLOCKED (HTTP 409)
  message: 候选 Skill 与已有 Skill 同名（figma-code-connect）。
  hint   : 用 `--decision skip` 跳过，或 `--decision keep` 保留两份。
  ```
- **json 模式**：

  ```json
  {
    "error": {
      "code": "SKILL_IMPORT_BLOCKED",
      "message": "...",
      "http_status": 409,
      "metadata": { "job_id": "job_7f3", "group_id": "group_01" }
    }
  }
  ```

  schema 在 `internal/cli/client/errors.go` 固化，golden 测试覆盖。

### 6.4 凭据存储

- `config.toml` 落盘前权限 `0600`（Win 跳过 chmod，仅日志提示）；读取时若 `stat.Mode().Perm() > 0o600` 拒绝读 token，并提示 `aranea config path` 与修复命令。
- `aranea login` 写完 token 后追加日志一行 `token saved to <path>`，不打印 token 明文。
- 不依赖系统 keyring（跨平台一致性差，留 P2）。

### 6.5 退出码与后端 `error.code` 映射

| 场景 | 退出码 | 触发条件 |
|------|--------|----------|
| 成功 | 0 | 命令完成，无 warn |
| 参数错误 / `--help` 误用 / 必填缺失 | 1 | cobra 解析失败 |
| 后端业务错误（4xx，非 401） | 2 | 后端返回 `error.code`；如 `AGENT_NOT_FOUND`、`SKILL_DUPLICATE_NAME` |
| 后端不可达 / DNS / TLS / 5xx | 3 | `net/http` 返回 net.Error 或 5xx |
| 用户取消 | 4 | 二次确认输入 `n` / Ctrl+C 在 prompt 阶段 |
| 校验或冲突阻塞 | 5 | `SKILL_IMPORT_BLOCKED`、warn 且非交互且无 `--decision` |
| 401 / 403（鉴权失败） | 6 | 提示重跑 `aranea login` |
| Ctrl+C 在执行中 | 130 | 标准信号约定 |

---

## 7. 配置与 profile

### 7.1 配置 schema（TOML）

```toml
# ~/.aranea/config.toml
[backend]
base_url     = "http://127.0.0.1:8080"
token        = ""              # JWT
workspace_id = ""              # 预留，本期无效

[ui]
output   = "text"              # text | json   (P1: yaml | table)
color    = "auto"              # auto | always | never

[skill]
default_decision = "ask"       # ask | skip | keep | refine
max_zip_mb       = 100
keep_temp        = false

[chat]                          # P1
default_agent = "__system_admin__"
auto_resume   = true

[telemetry]
enabled = false                 # 对齐 24 telemetry
```

### 7.2 优先级与覆盖

```
--flag    >    env (ARANEA_*)    >    config.toml    >    内置默认值
```

环境变量映射：`ARANEA_BASE_URL` / `ARANEA_TOKEN` / `ARANEA_OUTPUT` / `ARANEA_CONFIG` / `ARANEA_DEBUG` / `ARANEA_NO_COLOR`。

> **建议**：不引入 `viper`（体积大、隐式行为多）。手工实现"4 层叠加"，控制 ~150 行代码以内。

### 7.3 新增依赖清单（主 `go.mod`）

| 依赖 | 阶段 | 用途 | 推荐版本（实施时锁最新 stable） |
|------|------|------|-------------------------------|
| `github.com/spf13/cobra` | P0 | 命令框架 | 最新 stable |
| `github.com/spf13/pflag` | P0（cobra 间接） | POSIX flag | 跟随 cobra |
| `github.com/BurntSushi/toml` 或 `github.com/pelletier/go-toml/v2` | P0 | 读 / 写 config.toml | 推荐 `pelletier/go-toml/v2`（性能 + 写回保留注释） |
| `github.com/mattn/go-isatty` | P0 | TTY 检测 | 最新 |
| `github.com/fatih/color` | P0 | ANSI 色（自带 NO_COLOR 兼容） | 最新 |
| `github.com/olekukonko/tablewriter` | P0 | text/table 输出 | 最新 |
| `github.com/peterh/liner` | P1 | REPL 行编辑 + 历史 | 最新 |
| `github.com/gorilla/websocket` | P1 | WS 连接（与 `internal/server/ws.go` 同库） | 已在 `go.mod` |
| `github.com/go-git/go-git/v5` | P1 | `skill install` 拉仓库 | 最新 |
| `google.golang.org/protobuf` | P0（已在） | `protojson` | 已在 |

> **明确不引入**：viper、bubbletea、urfave/cli、go-resty。

---

## 8. 测试策略

| 层 | 范围 | 工具 | 是否纳入 `make ci` |
|----|------|------|--------------------|
| 单元 | `internal/cli/config`、`internal/cli/output`、`internal/cli/client/errors`、参数解析 | 标准 `testing` + `testify` | ✅（`make test` 已覆盖 `./...`） |
| 契约 | `internal/cli/client/*` 调用后端 → 用 `httptest.NewServer` 假后端断言请求与响应序列化 | `httptest` | ✅ |
| Golden | `internal/cli/output` 文本输出快照 | `golden` 包或自写 diff | ✅ |
| E2E | 真实编译 `aranea`，对 dev backend 跑 `aranea agent ls` / `aranea skill ls` / 主要 happy path | shell 脚本 `make smoke-cli`（新加） | ❌（脚本可手跑，CI 启动 admin 成本高，**P1 再纳入**） |
| 对话 E2E | WS 流端到端、二次确认 | 模拟 WS server | ❌（P1） |

`make ci` 现状是 `make lint test`，本期新增：

- `make cli` — 构建 `./bin/aranea`（当前平台）。
- `make cli-all` — 三平台交叉编译。
- **不**在默认 `make ci` 里强制 `make cli`；CI 矩阵中加 `go build ./cmd/aranea/` 即可，确保编译不破。

**新增 Makefile target 范例**：

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

.PHONY: smoke-cli                 # P1 引入
smoke-cli: cli
	./scripts/smoke-cli.sh         # 启 admin → 跑 ls/get/login → 关 admin
```

---

## 9. 阶段拆分（任务 ID 到天）

> 任务 ID 沿用 `CLI-` 前缀；每条 ≤1 天；P0 必须给可执行验收信号。

### Phase P0（MVP，1 周）

| ID | 任务 | 验收信号 |
|----|------|----------|
| CLI-01 | 新增 `cmd/aranea/main.go` + `internal/cli/{root,execute,ctx}.go` 骨架；接入 cobra | `go run ./cmd/aranea/ --help` 输出顶层命令 |
| CLI-02 | `internal/cli/config`：CLIConfig + 跨平台路径 + load/save + 权限检查 | `aranea config path` 在 Win/Linux/mac 各输出对应路径；`aranea config get/set base_url` 工作 |
| CLI-03 | `internal/cli/client/http.go`：Bearer 注入 + 重试（仅幂等）+ `--debug` 日志 + httptest | `go test ./internal/cli/client/...` 通过 |
| CLI-04 | `internal/cli/client/errors.go`：error 解码 + 退出码映射；golden | 单测覆盖 400/401/404/409/500/网络错误 6 类 |
| CLI-05 | `internal/cli/output`：text + json + TTY 检测 + golden | `aranea ... -o json` 可被 `jq` 解析 |
| CLI-06 | `aranea version`（含 backend reachability 探测） | 后端不可达时 exit 3 + 友好提示 |
| CLI-07 | 后端 `GET /v1/system/info`：新增 proto + service + httptest（落于 `api/kratos/system_setting/v1/` 或新 `api/kratos/system/v1/`） | curl 返回 JSON；CLI `aranea system info` 显示 |
| CLI-08 | `aranea login` → POST `/v1/admins/login`；token 写 config（0600） | 登录后 `aranea agent ls` 不再 401 |
| CLI-09 | `aranea agent ls/get/create/update/delete/enable/disable/tools/tools-set` | 每个子命令一条 httptest + 一条 manual smoke 通过 |
| CLI-10 | `aranea skill ls/get/create/update/delete/enable/disable/publish` | 同 CLI-09 |
| CLI-11 | `aranea tool ls/get/enable/disable` | 同 CLI-09 |
| CLI-12 | `Makefile` 新增 `cli` 目标；`cmd/araneactl/lint` 新增 R12（CLI 黑名单 import） | `make cli` 产出 `./bin/aranea`；`make lint` 在故意违例时失败 |
| CLI-13 | 文档：`docs/guides/cli-quickstart.md`（10 分钟上手） | 链接落到 `docs/README.md` 索引 |

**P0 退出条件**：上述 13 条全绿；`./bin/aranea agent ls` 对一个真实跑起的 `cmd/admin` 后端能正确分页。

### Phase P1（对话 + Skill 安装 + 剩余资源，2~3 周）

| ID | 任务 | 验收信号 |
|----|------|----------|
| CLI-20 | 后端 `SeedSystemAdminAgent()` + `SeedBuiltinTools()` + `SeedToolGroups()`（在 `internal/data/seed*.go`） | 重启后端，DB 中存在 `agent_key=__system_admin__` |
| CLI-21 | `internal/tools/cli_admin/registry.go` + 首批工具（skill_list/get/install_from_url/import_status/import_apply、agent_list/get） | 后端单测：系统管家 Agent 能调用工具 |
| CLI-22 | `internal/cli/client/ws.go` + envelope 编解码 | 单测：模拟 WS server 发送 5 类事件，CLI 渲染断言 |
| CLI-23 | `internal/cli/repl/*`：主循环、行编辑（`peterh/liner`）、斜杠命令、二次确认走 `AwaitUserReply` | `aranea` 启动进入 REPL，`/help`、`/quit` 工作；手动一次完整 skill install 对话流跑通 |
| CLI-24 | `aranea skill install <url>` 完整链路：解析→clone（`go-git`）→定位→预校验→打包→multipart 上传→轮询 | 一个 happy-path GitHub URL 安装成功；冲突 warn 时按 `--decision` 工作 |
| CLI-25 | 后端 multipart import 接收 `source/source_url/source_ref/source_subpath/client_validation` 写入 `metadata_json` | `tool_invocations` / `audit_logs` 中可见来源字段 |
| CLI-26 | `aranea skill import/import-status/import-apply` | 每个子命令 httptest |
| CLI-27 | `aranea team/plugin/mcp/cron/channel/session/monitor` 全套 | 每个资源 1 条 smoke |
| CLI-28 | `aranea completion <shell>` + 文档 | bash/zsh/powershell 三种各跑一次 |
| CLI-29 | 后端剩余 `cli_admin_*` 工具（team/plugin/mcp/cron/channel/provider/session 全量） | 单测覆盖每个工具 |
| CLI-30 | P1 文档与变更说明追加到 `docs/changelog/` | PR 合入 |

### Phase P2（体验优化，按需）

| ID | 任务 |
|----|------|
| CLI-40 | `--output yaml/table` |
| CLI-41 | REPL 上下箭头历史搜索 |
| CLI-42 | 错误信息中文翻译表（基于 `error.code`） |
| CLI-43 | 跨平台二进制发布（GitHub Release / OSS） |
| CLI-44 | 系统 keyring 集成 |
| CLI-45 | `aranea init` 引导（首启检测后端 + 写默认 config） |
| CLI-46 | E2E smoke 纳入 `make ci`（带 admin 后端启停） |

**实施顺序**：P0 → P1（CLI-20 必须先于 CLI-23/24）→ P2 按业务优先级。

---

## 10. 风险与开放问题

### 10.1 风险（≥5 条）

| # | 风险 | 影响 | 缓解 |
|---|------|------|------|
| R1 | WS 现有 envelope 尚未细化 `tool.call/result/error` 子类型 | REPL 渲染无法精确展示工具步骤 | 实施 CLI-22 前先确认 `internal/server/ws.go` envelope；若缺失，先在 service/server 补，再启动 REPL 工作 |
| R2 | cobra 引入 ~3MB 二进制体积增量 + 多个间接依赖 | 主 module 依赖图变重 | 接受；若 unacceptable 再换 `urfave/cli`；不自造 |
| R3 | Windows 控制台对 ANSI / spinner 兼容差（cmd.exe vs Windows Terminal） | 输出乱码 | 启动时 `enable_virtual_terminal_processing`；spinner 用 ASCII；`NO_COLOR` 自动降级 |
| R4 | `go-git` 在私有仓库 SSH 凭据上行为不一致 | `skill install` ssh URL 失败 | MVP 仅声明支持 https；ssh 在 P1，先 fallback 到本地 `git` 命令（如有） |
| R5 | `aranea` 与后端版本不一致时 API 字段缺失 | 命令报错难诊断 | `aranea version` 强制比对 `/v1/system/info.version`；不一致时 warn |
| R6 | `internal/biz` 被 CLI 误 import | 二进制爆炸 / 框架污染 | 在 `cmd/araneactl/lint` 加 R12；CI 阻断 |
| R7 | `--yes` 在 CI 被滥用，绕过破坏性操作 | 误删资源 | 后端关键 API（delete agent/skill）二次校验 `confirm_key`；CLI 仅传递，不豁免 |
| R8 | Skill 临时目录跨平台清理失败（Windows 文件句柄） | 磁盘占用 | 退出时 `defer` + 容错；记录到 `~/.aranea/logs/`；提供 `aranea config set skill.keep_temp false` |

### 10.2 待拍板问题（带候选 + 推荐）

| # | 问题 | 候选 | 推荐 |
|---|------|------|------|
| Q1 | CLI 是否随后端同 release 打包？ | (a) `cmd/admin` Release 附 `aranea` 三平台二进制；(b) `aranea` 独立 release tag | **(a)**：版本严格对齐，避免 R5 |
| Q2 | REPL 行编辑库选型 | (a) `bufio.Scanner` 自绘；(b) `peterh/liner`；(c) `chzyer/readline` | **(b)** `peterh/liner`：纯 Go、跨平台、依赖少、API 简单；(c) 在 Windows 兼容性偶有问题 |
| Q3 | `__system_admin__` 是否在 Web Agent 列表展示？ | (a) 隐藏；(b) 锁定展示（`readonly=true`） | **(b)**：与 §3.3.0 系统管家约定一致，避免 Web 用户疑惑"为什么 CLI 调用看不到 agent" |
| Q4 | `cli_admin_*` 工具是否允许其他用户 Agent 启用？ | (a) 默认拒绝（后端硬编码白名单仅 `__system_admin__`）；(b) 走 `tools_allow_json` 自由组合 | **(a)**：降低误用面，P2 再放开 |
| Q5 | 非交互 + warn 行为 | (a) 自动 skip；(b) exit 5 | **(b) exit 5**：CI 友好（明确失败），与 `25 cli.md §9.3` 一致 |

---

## 11. 与既有方案的迁移

| 既有材料 | 决定 | 说明 |
|----------|------|------|
| `25 cli.md`（需求） | **保留**，但需勘误 | 把 `/api/v1/*` 统一替换为 `/v1/*`；删 `workspace_id` 提及；补 §10 待确认问题的本方案答案 |
| `25 cli.design.md`（设计） | **部分重写** | 删除 §2.3 中 `internal/biz` / `internal/agent` 依赖项；删除 §八 "CLI 自建 Runner / dbAgentLoader / SQLite SessionService"；删除 §3.2 "手抄请求/响应 struct"；§5.3 用 `AwaitUserReply` 替代 `ConfirmPlugin` |
| `25-cli-development.md`（开发计划） | **废弃** | 由本文档 §9 取代；保留一段引用指向本文档 |
| `cmd/araneactl/lint` | **扩展** | 新增 R12："`cmd/aranea/**` 与 `internal/cli/**` 不得 import biz/data/agent/server/service/trpc-agent-go" |
| `cmd/araneactl/fmtcheck` | **不动** | 与 `aranea` 完全独立 |
| `Makefile` | **追加** | `cli`、`cli-all`、`smoke-cli`（P1） |
| `pkg/auth` | **不动** | CLI 仅复用其常量与解析函数；workspace 字段在另一专项里推 |
| `api/kratos/skill/v1/skill.proto` | **可选追加** | 若 multipart 字段不够灵活，再考虑加 RPC；本方案优先 multipart form 字段 |
| 新增后端 service | `internal/service/system_info.go`（CLI-07）、`internal/data/seed_system_admin.go`（CLI-20）、`internal/tools/cli_admin/`（CLI-21/29）| 与现有 service 装配模式一致 |

---

## 11.1 实施 cookbook（关键骨架伪代码）

> 下列代码仅为接口形状参考，不替代实际实现。

**`internal/cli/root.go`**：

```go
// 假设：返回 cobra.Command 供 cmd/aranea/main.go 调用
func NewRoot(ctx context.Context) *cobra.Command {
    var cfgPath, baseURL, token, output string
    var quiet, debug, yes, noColor bool

    root := &cobra.Command{
        Use:           "aranea",
        Short:         "Aranea 终端控制台",
        SilenceUsage:  true,
        SilenceErrors: true,
        PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
            cfg, err := config.Load(cfgPath)
            if err != nil { return err }
            cfg.OverrideFromEnv()
            cfg.OverrideFromFlags(baseURL, token, output, noColor)
            cli := newCLIContext(ctx, cfg, debug, quiet, yes)
            cmd.SetContext(WithCLI(cmd.Context(), cli))
            return nil
        },
    }
    root.PersistentFlags().StringVar(&cfgPath, "config", "", "config file path")
    // ... 其它 persistent flags
    root.AddCommand(
        newVersionCmd(), newLoginCmd(), newConfigCmd(), newSystemCmd(),
        newAgentCmd(), newSkillCmd(), newToolCmd(),
        // P1: newChatCmd(), newTeamCmd(), newPluginCmd(), ...
    )
    return root
}
```

**`internal/cli/client/http.go`**：

```go
type Client struct {
    Base   string
    Token  string
    Doer   interface{ Do(*http.Request) (*http.Response, error) }
    Debug  bool
}

func (c *Client) Do(ctx context.Context, method, path string, body, out proto.Message) error {
    var buf io.Reader
    if body != nil {
        b, err := protojson.Marshal(body)
        if err != nil { return err }
        buf = bytes.NewReader(b)
    }
    req, _ := http.NewRequestWithContext(ctx, method, c.Base+path, buf)
    if c.Token != "" { req.Header.Set("Authorization", "Bearer "+c.Token) }
    req.Header.Set("Content-Type", "application/json")
    if c.Debug { logRequest(req) }
    resp, err := c.Doer.Do(req)
    if err != nil { return wrapNetErr(err) }
    defer resp.Body.Close()
    if c.Debug { logResponse(resp) }
    return decode(resp, out) // 内部根据 status code 决定走 protojson 还是 errors.go
}
```

**`internal/cli/cmd/agent.go`**（部分）：

```go
func newAgentCmd() *cobra.Command {
    c := &cobra.Command{Use: "agent", Short: "Agent 管理"}
    c.AddCommand(
        &cobra.Command{
            Use:  "ls",
            RunE: func(cmd *cobra.Command, _ []string) error {
                cli := CLIFrom(cmd.Context())
                resp := &agentv1.ListAgentsResponse{}
                err := cli.Client.Do(cmd.Context(), "GET", "/v1/agents?page=...", nil, resp)
                if err != nil { return err }
                return cli.Printer.PrintList(resp.Items, int(resp.Total))
            },
        },
        // get / create / update / delete / enable / disable / tools / tools-set
    )
    return c
}
```

---

## 12. 红线再次清单（实施 review 时逐条比对）

- [ ] `cmd/aranea/**` 与 `internal/cli/**` **不** import `internal/biz`、`internal/data`、`internal/agent`、`internal/server`、`internal/service`、`pkg/trpc-agent-go`。
- [ ] CLI **不**直连 SQLite / Postgres；所有数据操作走后端 HTTP / WS。
- [ ] CLI **不**在进程内构建 `trpcrunner.Runner` 或 `trpcagent.Agent`。
- [ ] 系统管家 Agent 种子、`cli_admin_*` 工具集**只**在后端（`internal/data` / `internal/tools/cli_admin`）；CLI 不参与 Seed。
- [ ] Runner 装配仍只在 `internal/service`；新增的 `cli_admin_*` 工具组装通过 service 注入到 system_admin agent。
- [ ] `internal/server/**` 不新增对 `pkg/trpc-agent-go` 的直接依赖（即使为 CLI 流式渲染加 envelope 时也走 service）。
- [ ] 所有 HTTP 路径以 `/v1/*` 为准（不是 `/api/v1/*`）。
- [ ] Token 文件权限 0600；CLI 不打印 token 明文。
- [ ] 高风险写操作没有 `--yes` 时拒绝；`--yes`/`/yes` 仅当前会话/进程内有效。

---

*文档版本：1.1 — 2026-06-06；基于仓库实际代码盘点对 25 cli 系列做对账与收窄。如实施过程中发现仓库代码与本文档假设不一致（特别是 WS envelope、Skill import multipart 字段、`/v1/admins/login` 响应体），以代码为准并补一份 `docs/changelog/` 变更记录。详细技术设计见 [`25-cli.design.md`](./25-cli.design.md)，任务表见 [`25-cli.development.md`](./25-cli.development.md)。*
