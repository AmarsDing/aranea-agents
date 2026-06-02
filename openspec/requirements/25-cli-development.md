# CLI 命令行 — 开发计划

> ⚠️ **状态：SUPERSEDED（2026-05-27）**
> 本文档（v2.0）已被全面取代，请改读：
> - **新开发计划**：[`25-cli-development-plan-2026-05-27.md`](./25-cli-development-plan-2026-05-27.md)（v3.0，任务 ID 改为 `CLI-01..30`，每条 ≤1 天，含 DoD/依赖/AI 提示词）
> - 新需求：[`25-cli-PRD-2026-05-27.md`](./25-cli-PRD-2026-05-27.md)
> - 新设计：[`25-cli-design-2026-05-27.md`](./25-cli-design-2026-05-27.md)
>
> 与本文档的差异概要：
> - Phase 1 中"对话模式 dbAgentLoader / ConfirmPlugin / BuildSystemAdminAgent 在 CLI 进程组装"全部删除 —— CLI 仅作 WS 客户端
> - 任务粒度统一到 ≤1 天；BE / CLI 工种分开；R12 黑名单 lint 上线进入 P0 退出条件
> - 完整差异表见新开发计划 §9
>
> 本文档保留作为历史，**不要再据此实施**。

---

> **版本**：2.0 | **状态**：❌ 未实现（SUPERSEDED，2026-05-27）
> **需求**：[25 cli.md](./25%20cli.md)（SUPERSEDED） · **设计**：[25 cli.design.md](./25%20cli.design.md)（SUPERSEDED）
> **规范**：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md)

---

## 1. 模块定位

CLI 命令行工具：提供直接命令模式和交互式对话模式，管理 Agent / Team / Tool / Skill / Plugin / MCP / Cron / Channel 等资源。内置系统管家 Agent 支持自然语言驱动的跨模块操作。

**代码锚点**：
- 新增 `cmd/aranea/` — CLI 二进制入口
- 新增 `internal/tools/cli_admin/` — 系统管家 Tool 集
- 新增 `internal/plugin/trpc/confirm.go` — 二次确认 Plugin
- 复用 `internal/agent/trpc_build.go` — Agent 构建
- 复用 `internal/agent/trpc_runtime.go` — Agent 运行时
- 复用 `internal/service/*` — 后端 REST API

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| CLI 框架 | ❌ | 无 cobra 集成 |
| 直接命令模式 | ❌ | 无资源/动作子命令 |
| 对话交互模式 | ❌ | 无 REPL |
| 系统管家 Agent | ❌ | 无 `__system_admin__` Agent |
| cli_admin_* Tools | ❌ | 无系统管家工具集 |
| API Client | ❌ | 无 HTTP 客户端封装 |
| 输出格式化 | ❌ | 无 Printer 实现 |
| 配置管理 | ❌ | 无 config.toml |
| Skill URL 安装 | ❌ | 无 git clone + import 流程 |
| 二次确认 | ❌ | 无 ConfirmPlugin |
| 后端种子数据 | ❌ | 无系统管家 Agent 种子 |
| 现有 CLI 工具 | ✅ | `cmd/araneactl/` 有 lint / fmtcheck（共存不替代） |

---

## 3. 开发阶段

### Phase 1：骨架 + 直接命令模式（核心 CRUD）

**目标**：`aranea` 可安装、可登录、可通过直接命令管理核心资源。

**交付物**：
- `cmd/aranea/main.go` 入口 + Cobra root
- `cmd/aranea/apiclient/` HTTP 客户端
- `cmd/aranea/output/` 输出格式化
- `cmd/aranea/config/` 配置管理
- `cmd/aranea/login/` 登录命令
- `cmd/aranea/version/` 版本命令
- `cmd/aranea/agent/` Agent CRUD 子命令
- `cmd/aranea/skill/` Skill CRUD 子命令（不含 install from URL）
- `cmd/aranea/team/` Team CRUD 子命令
- `cmd/aranea/tool/` Tool 子命令
- `cmd/aranea/system/` system info 子命令
- `cmd/aranea/completion/` Shell 补全

**验收**：
- [ ] `aranea version` 输出版本号
- [ ] `aranea login` 完成认证并保存 token
- [ ] `aranea agent ls` 列出 Agent
- [ ] `aranea agent get <id>` 查看详情
- [ ] `aranea agent create --file agent.yaml` 创建 Agent
- [ ] `aranea agent update <id> --file agent.yaml` 更新 Agent
- [ ] `aranea agent delete <id>` 删除 Agent
- [ ] `aranea agent enable <id>` / `disable <id>` 启停 Agent
- [ ] `aranea skill ls` / `get` / `create` / `update` / `delete` / `enable` / `disable` 完整可用
- [ ] `aranea team ls` / `get` / `create` / `update` / `delete` 完整可用
- [ ] `aranea tool ls` / `enable` / `disable` 完整可用
- [ ] `aranea system info` 输出系统信息
- [ ] `--output json` / `--output table` / `--output yaml` 格式正确
- [ ] `aranea completion bash/zsh/fish` 生成补全脚本
- [ ] `go build ./cmd/aranea/` 编译通过

---

### Phase 2：对话模式 + 系统管家 Agent

**目标**：`aranea` / `aranea chat` 可进入 REPL，与系统管家 Agent 对话完成管理操作。

**交付物**：
- `cmd/aranea/repl/` REPL 主循环
- `cmd/aranea/chat/` chat 子命令
- `internal/tools/cli_admin/registry.go` Tool 注册
- `internal/tools/cli_admin/skill_*.go` Skill 管理 Tools
- `internal/tools/cli_admin/agent_*.go` Agent 管理 Tools
- `internal/tools/cli_admin/team_*.go` Team 管理 Tools
- `internal/tools/cli_admin/tool_*.go` Tool 管理 Tools
- `internal/plugin/trpc/confirm.go` 二次确认 Plugin
- 后端 `SeedSystemAdminAgent()` 种子数据
- 后端 `GET /api/v1/system/info` 接口

**验收**：
- [ ] `aranea` 无参数进入 REPL，显示欢迎信息
- [ ] `aranea chat` 进入 REPL
- [ ] `aranea chat --agent <key>` 与指定 Agent 对话
- [ ] `aranea chat --team <key>` 与指定 Team 对话
- [ ] `aranea chat --session <id>` 恢复已有会话
- [ ] REPL 中输入自然语言，系统管家 Agent 响应
- [ ] 系统管家调用 `cli_admin_skill_list` 查询 Skill
- [ ] 系统管家调用 `cli_admin_agent_create` 创建 Agent
- [ ] 高风险 Tool 触发二次确认，用户 `y` 后继续
- [ ] 高风险 Tool 用户 `n` 后取消，返回取消消息
- [ ] `/help` 显示斜杠命令列表
- [ ] `/exit` 退出 REPL
- [ ] `/clear` 清空当前会话
- [ ] `/agent <key>` 切换对话 Agent
- [ ] `/session <id>` 切换会话
- [ ] `Ctrl+C` 中断当前生成，回到提示符
- [ ] `Ctrl+D` 退出 REPL
- [ ] 流式输出增量显示，无明显卡顿

---

### Phase 3：Skill 安装 + 剩余资源命令

**目标**：Skill 从 URL 安装全流程可用；Plugin / MCP / Cron / Channel / Session / Monitor 子命令完整。

**交付物**：
- `cmd/aranea/skill/install.go` install from URL 子命令
- `cmd/aranea/skill/import.go` import / import-status / import-apply 子命令
- `internal/tools/cli_admin/skill_install.go` install from URL Tool
- `internal/tools/cli_admin/skill_import.go` import / status / apply Tools
- `internal/tools/cli_admin/skill_refine.go` 冲突精修 Tool
- `cmd/aranea/plugin/` Plugin 子命令
- `cmd/aranea/mcp/` MCP 子命令
- `cmd/aranea/cron/` Cron 子命令
- `cmd/aranea/channel/` Channel 子命令
- `cmd/aranea/session/` Session 子命令
- `cmd/aranea/monitor/` Monitor 子命令
- `internal/tools/cli_admin/plugin_*.go` Plugin Tools
- `internal/tools/cli_admin/mcp_*.go` MCP Tools
- `internal/tools/cli_admin/cron_*.go` Cron Tools
- `internal/tools/cli_admin/channel_*.go` Channel Tools
- `internal/tools/cli_admin/provider_*.go` Provider Tools
- `internal/tools/cli_admin/session_*.go` Session Tools
- 后端 `POST /api/v1/skills/import` 扩展字段

**验收**：
- [ ] `aranea skill install https://github.com/xxx/skill-repo` 完成安装
- [ ] `aranea skill install https://github.com/xxx/skill-repo --ref v1.0` 指定分支
- [ ] `aranea skill install https://github.com/xxx/mono-repo --subpath skills/xxx` 指定子目录
- [ ] 安装过程显示步骤进度（解析 → 下载 → 校验 → 打包 → 上传 → 轮询）
- [ ] 冲突时提示用户选择（覆盖 / 跳过 / 重命名）
- [ ] `aranea skill import ./skill.zip` 上传 zip
- [ ] `aranea skill import-status <job_id>` 查看导入状态
- [ ] `aranea skill import-apply <job_id>` 应用导入
- [ ] 对话模式中 "帮我安装 xxx skill" 触发 `cli_admin_skill_install_from_url`
- [ ] `aranea plugin ls` / `enable` / `disable` / `config-set` 完整可用
- [ ] `aranea mcp ls` / `add` / `update` / `delete` / `test` 完整可用
- [ ] `aranea cron ls` / `add` / `update` / `delete` / `pause` / `resume` / `trigger` 完整可用
- [ ] `aranea channel ls` / `add` / `update` / `delete` / `test` / `send` 完整可用
- [ ] `aranea session ls` / `get` / `send` 完整可用
- [ ] `aranea monitor audit-logs` / `events` / `traces` 完整可用

---

### Phase 4：体验优化 + 发布

**目标**：跨平台构建、文档、性能优化、发布流程。

**交付物**：
- Makefile `cli` / `cli-all` 目标
- 跨平台二进制（windows / darwin / linux）
- 配置文件 `config.toml` 完整支持
- REPL 历史搜索（上下箭头）
- 进度条 / spinner 优化
- 错误信息中文翻译
- `--debug` 模式
- 发布脚本

**验收**：
- [ ] `make cli` 编译成功
- [ ] `make cli-all` 生成三平台二进制
- [ ] `aranea config get` / `set` / `edit` / `path` 完整可用
- [ ] REPL 上下箭头可搜索历史
- [ ] `--debug` 输出 HTTP 请求/响应详情
- [ ] 错误信息在 text 模式下显示中文，json 模式下透传原始 code
- [ ] 无 TTY 时自动降级（无色、无 spinner）

---

## 4. 任务清单

### Phase 1 任务

| # | 任务 | 文件 | 依赖 | 优先级 |
|---|------|------|------|--------|
| 1.1 | 创建 CLI 入口 + Cobra root command | `cmd/aranea/main.go`, `cmd/aranea/root.go` | — | P2 |
| 1.2 | 全局 flags（--server, --token, --output, --color, --debug） | `cmd/aranea/root.go` | 1.1 | P2 |
| 1.3 | API Client 基础结构 | `cmd/aranea/apiclient/client.go` | 1.1 | P2 |
| 1.4 | API Client 认证/重试中间件 | `cmd/aranea/apiclient/transport.go` | 1.3 | P2 |
| 1.5 | Printer 接口 + text/json/yaml/table 实现 | `cmd/aranea/output/` | 1.1 | P2 |
| 1.6 | CLIConfig + 加载/保存 | `cmd/aranea/config/` | 1.1 | P2 |
| 1.7 | 跨平台路径解析 | `cmd/aranea/config/paths.go` | 1.6 | P2 |
| 1.8 | login 子命令 | `cmd/aranea/login/` | 1.3, 1.6 | P2 |
| 1.9 | version 子命令 | `cmd/aranea/version/` | 1.1 | P2 |
| 1.10 | completion 子命令 | `cmd/aranea/completion/` | 1.1 | P2 |
| 1.11 | 后端 `GET /api/v1/system/info` 接口 | `internal/service/system.go` | — | P2 |
| 1.12 | Agent 子命令（ls/get/create/update/delete/enable/disable/run/tools/tools-set） | `cmd/aranea/agent/` | 1.3, 1.5 | P2 |
| 1.13 | Skill 子命令（ls/get/create/update/delete/enable/disable/publish） | `cmd/aranea/skill/` | 1.3, 1.5 | P2 |
| 1.14 | Team 子命令（ls/get/create/update/delete/run/runs/run-events） | `cmd/aranea/team/` | 1.3, 1.5 | P2 |
| 1.15 | Tool 子命令（ls/get/enable/disable） | `cmd/aranea/tool/` | 1.3, 1.5 | P2 |
| 1.16 | system info 子命令 | `cmd/aranea/system/` | 1.11 | P2 |
| 1.17 | API Client 各资源方法（Agent/Skill/Team/Tool） | `cmd/aranea/apiclient/` | 1.3 | P2 |

### Phase 2 任务

| # | 任务 | 文件 | 依赖 | 优先级 |
|---|------|------|------|--------|
| 2.1 | REPL 主循环（输入/输出/事件处理） | `cmd/aranea/repl/repl.go` | 1.1 | P3 |
| 2.2 | 斜杠命令处理（/help, /exit, /clear, /agent, /session） | `cmd/aranea/repl/slash.go` | 2.1 | P3 |
| 2.3 | 二次确认交互 | `cmd/aranea/repl/confirm.go` | 2.1 | P3 |
| 2.4 | Spinner / 进度条 | `cmd/aranea/repl/spinner.go` | 2.1 | P3 |
| 2.5 | 会话历史管理 | `cmd/aranea/repl/history.go` | 2.1 | P3 |
| 2.6 | chat 子命令 | `cmd/aranea/chat/` | 2.1 | P3 |
| 2.7 | 后端 SeedSystemAdminAgent() 种子数据 | `internal/data/seed.go` | — | P3 |
| 2.8 | 后端 SeedBuiltinTools() + SeedToolGroups() | `internal/data/seed.go` | 2.7 | P3 |
| 2.9 | agent 表新增 readonly/kind 字段 | `internal/data/migration.go` | — | P3 |
| 2.10 | ConfirmPlugin 实现 | `internal/plugin/trpc/confirm.go` | — | P3 |
| 2.11 | cli_admin Tool 注册框架 | `internal/tools/cli_admin/registry.go` | 2.7 | P3 |
| 2.12 | cli_admin_skill_* Tools（list/install_from_url/install_from_path/import_status/import_apply/refine/enable/disable/delete） | `internal/tools/cli_admin/skill_*.go` | 2.11 | P3 |
| 2.13 | cli_admin_agent_* Tools（list/get/create/update/delete/tools_get/tools_set） | `internal/tools/cli_admin/agent_*.go` | 2.11 | P3 |
| 2.14 | cli_admin_team_* Tools（list/create/update/delete/run） | `internal/tools/cli_admin/team_*.go` | 2.11 | P3 |
| 2.15 | cli_admin_tool_* Tools（list/enable/disable/config_set） | `internal/tools/cli_admin/tool_*.go` | 2.11 | P3 |
| 2.16 | BuildSystemAdminAgent 函数 | `internal/agent/trpc_build.go` 扩展 | 2.7, 2.11 | P3 |
| 2.17 | dbAgentLoader 实现 | `cmd/aranea/repl/loader.go` | 2.16 | P3 |
| 2.18 | WS 流式渲染 | `cmd/aranea/repl/repl.go` 扩展 | 2.1 | P3 |

### Phase 3 任务

| # | 任务 | 文件 | 依赖 | 优先级 |
|---|------|------|------|--------|
| 3.1 | skill install from URL 子命令 | `cmd/aranea/skill/install.go` | 1.13 | P3 |
| 3.2 | skill import / import-status / import-apply 子命令 | `cmd/aranea/skill/import.go` | 1.13 | P3 |
| 3.3 | cli_admin_skill_install_from_url Tool（完整编排） | `internal/tools/cli_admin/skill_install.go` | 2.12 | P3 |
| 3.4 | cli_admin_skill_refine Tool（冲突精修） | `internal/tools/cli_admin/skill_refine.go` | 2.12 | P3 |
| 3.5 | 后端 POST /api/v1/skills/import 扩展字段 | `internal/service/skill.go` | — | P3 |
| 3.6 | Plugin 子命令（ls/get/enable/disable/config-set） | `cmd/aranea/plugin/` | 1.3, 1.5 | P3 |
| 3.7 | MCP 子命令（ls/get/add/update/delete/test） | `cmd/aranea/mcp/` | 1.3, 1.5 | P3 |
| 3.8 | Cron 子命令（ls/get/add/update/delete/pause/resume/trigger） | `cmd/aranea/cron/` | 1.3, 1.5 | P3 |
| 3.9 | Channel 子命令（ls/get/add/update/delete/test/send） | `cmd/aranea/channel/` | 1.3, 1.5 | P3 |
| 3.10 | Session 子命令（ls/get/send） | `cmd/aranea/session/` | 1.3, 1.5 | P3 |
| 3.11 | Monitor 子命令（audit-logs/events/traces） | `cmd/aranea/monitor/` | 1.3, 1.5 | P3 |
| 3.12 | cli_admin_plugin_* Tools | `internal/tools/cli_admin/plugin_*.go` | 2.11 | P3 |
| 3.13 | cli_admin_mcp_* Tools | `internal/tools/cli_admin/mcp_*.go` | 2.11 | P3 |
| 3.14 | cli_admin_cron_* Tools | `internal/tools/cli_admin/cron_*.go` | 2.11 | P3 |
| 3.15 | cli_admin_channel_* Tools | `internal/tools/cli_admin/channel_*.go` | 2.11 | P3 |
| 3.16 | cli_admin_provider_* Tools | `internal/tools/cli_admin/provider_*.go` | 2.11 | P3 |
| 3.17 | cli_admin_session_* Tools | `internal/tools/cli_admin/session_*.go` | 2.11 | P3 |
| 3.18 | API Client 各资源方法（Plugin/MCP/Cron/Channel/Session/Monitor） | `cmd/aranea/apiclient/` | 1.3 | P3 |

### Phase 4 任务

| # | 任务 | 文件 | 依赖 | 优先级 |
|---|------|------|------|--------|
| 4.1 | Makefile cli / cli-all 目标 | `Makefile` | 1.1 | P3 |
| 4.2 | config 子命令（get/set/edit/path） | `cmd/aranea/config_cmd/` | 1.6 | P3 |
| 4.3 | REPL 历史搜索（上下箭头） | `cmd/aranea/repl/history.go` 扩展 | 2.5 | P3 |
| 4.4 | 错误信息中文翻译 | `cmd/aranea/output/text.go` 扩展 | 1.5 | P3 |
| 4.5 | --debug 模式（HTTP 请求/响应日志） | `cmd/aranea/apiclient/transport.go` 扩展 | 1.4 | P3 |
| 4.6 | 无 TTY 降级（无色、无 spinner） | `cmd/aranea/output/`, `cmd/aranea/repl/` | 1.5, 2.4 | P3 |
| 4.7 | 跨平台构建验证 | — | 4.1 | P3 |

---

## 5. 依赖关系图

```text
Phase 1（骨架 + CRUD）
  1.1 root ──┬── 1.2 flags
             ├── 1.3 apiclient ──── 1.4 transport ──── 1.8 login
             ├── 1.5 output
             ├── 1.6 config ──── 1.7 paths
             ├── 1.9 version
             ├── 1.10 completion
             └── 1.11 system info (后端)
                    │
                    ├── 1.12 agent cmd ←── 1.17 apiclient methods
                    ├── 1.13 skill cmd ←── 1.17
                    ├── 1.14 team cmd  ←── 1.17
                    ├── 1.15 tool cmd  ←── 1.17
                    └── 1.16 system cmd

Phase 2（对话模式）  ←── Phase 1
  2.1 repl ────┬── 2.2 slash
               ├── 2.3 confirm
               ├── 2.4 spinner
               ├── 2.5 history
               └── 2.6 chat cmd
  2.7 seed (后端) ──── 2.8 seed tools
  2.9 migration (后端)
  2.10 ConfirmPlugin
  2.11 cli_admin registry ────┬── 2.12 skill tools
                               ├── 2.13 agent tools
                               ├── 2.14 team tools
                               └── 2.15 tool tools
  2.16 BuildSystemAdminAgent ←── 2.7, 2.11
  2.17 dbAgentLoader ←── 2.16
  2.18 WS 渲染 ←── 2.1

Phase 3（Skill 安装 + 剩余命令）  ←── Phase 2
  3.1 skill install cmd ←── 1.13
  3.2 skill import cmd  ←── 1.13
  3.3 skill install Tool ←── 2.12
  3.4 skill refine Tool  ←── 2.12
  3.5 后端 import 扩展
  3.6~3.11 各资源子命令 ←── 1.3, 1.5
  3.12~3.17 各资源 Tools ←── 2.11
  3.18 apiclient methods

Phase 4（体验优化）  ←── Phase 3
  4.1 Makefile
  4.2 config cmd
  4.3~4.7 优化项
```

---

## 6. 风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| trpc-agent-go Runner 接口变更 | 对话模式编译失败 | Phase 2 开始前确认 Runner API 稳定版 |
| 后端 API 与 proto 不一致 | 直接命令返回错误 | Phase 1 每个 cmd 对应写集成测试 |
| Skill URL 安装 git clone 失败 | 安装流程中断 | 支持 `--zip` 本地路径降级；超时重试 |
| 二次确认阻塞非交互模式 | CI/CD 场景卡死 | `--yes` flag 跳过确认 |
| Windows 路径兼容 | 配置/临时文件路径错误 | 使用 `os.UserConfigDir()` / `os.UserCacheDir()` |
| Cobra 子命令数量多 | 维护成本高 | 每个资源一个文件，模板化生成 |

---

## 7. 技术约束

- **Go 版本**：≥ 1.22（与项目 go.mod 一致）
- **不引入 gRPC 栈**：CLI HTTP 客户端手动定义请求/响应结构体，不 import proto 生成代码
- **不引入 TUI 框架**：REPL 使用标准库 `bufio.Scanner` + ANSI escape，不依赖 Bubbletea
- **与 araneactl 共存**：`cmd/araneactl/` 现有 lint / fmtcheck 不受影响
- **无 CGO 依赖**：确保跨平台编译零障碍

---

*文档版本：2.0 — 对齐需求文档 25 cli.md v2.0、设计文档 25 cli.design.md v2.0；任务可执行、边界清晰。*
