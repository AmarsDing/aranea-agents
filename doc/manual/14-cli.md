# 14 CLI 工具

## 功能

功能完整的命令行工具 `aranea`：无需打开浏览器即可管理 Agent、发起对话、监控运行状态。**Web 端能做的，CLI 都能做。**

## 原理

- **交互式 REPL**：裸 `aranea` 或 `aranea chat` 进入交互对话，支持 `/help`、`/session`、`/agent`、`/tools`、`/cancel` 等斜杠命令，类似 Claude Code 的交互体验；
- **WebSocket 实时流**：chat 命令通过 WS 接收流式响应，支持 thinking / tool / reply 事件；
- **多输出格式**：text / json 两种格式（全局旗标 `-o`），适配脚本与人工阅读；
- **配置管理**：平台配置目录下的 `aranea/config.toml`（Linux/macOS `~/.config/aranea/config.toml`，Windows `%APPDATA%\aranea\config.toml`）管理 endpoint、token、默认 agent，登录后文件权限 0600；
- **跨平台**：Windows / Linux / macOS，自动检测终端能力（颜色 / Unicode / TTY）；
- **脚本友好**：JSON 输出 + 退出码，便于 CI/CD 集成。

## 构建与登录

```bash
make cli                    # 构建到 ./bin/（Windows 为 aranea.exe）

./bin/aranea login --base-url http://localhost:8810 --user dev --password dev
```

`--base-url` 为全局持久旗标，登录后 token 存入本地 `config.toml`。

## 命令分类（26 个命令域、130+ 子命令）

| 类别 | 命令 |
|------|------|
| Agent 管理 | `agent ls / get / create / update / delete / enable / disable / tools / tools-set` |
| Team 编排 | `team ls / get / create / update / delete / run / runs` |
| Graph 编排 | `graph ls / get / create / update / delete / import / export` |
| Skill 管理 | `skill ls / get / create / update / delete / enable / disable / publish / files / file-get / file-put / file-delete / import` |
| 会话对话 | `chat`，`session ls / get / send / messages / archive / restore / pin / unpin / compact / export` |
| MCP / 工具 | `mcp ls / get / add / update / delete / test / validate`，`tool ls / get / enable / disable / test` |
| Channel | `channel ls / get / add / update / delete / test / toggle` |
| 定时任务 | `cron ls / get / add / update / delete / trigger / runs / pause / resume / reset-failures` |
| 系统监控 | `monitor audit-logs / events / traces` |
| 组织架构 | `org ls / tree / get / create / update / delete / reorder` |
| 业务分类 | `taxonomy ls / tree / get / create / update / delete / reorder` |
| 记忆中心 | `memory facts ls`、`memory proposals ls / approve / reject`、`memory search / recall-debug` |
| 知识库 | `knowledge collections ls / get / create / delete`，`knowledge documents ls / get / delete`，`knowledge search` |
| 模型目录 | `model-catalog ls / get / policy / policy-set / sync` |
| 评测 | `eval datasets ls / get / create`，`eval runs ls / get / create`，`eval results` |
| A2A 联邦 | `a2a discover / remote-agents / audit / config` |
| 插件 | `plugin ls / enable / disable / order-set / config-set` |
| 场景包 | `pack export / import / validate`（.arpack 一键迁移 Agent/Team/行业场景） |
| 行为包安装 | `pkg install / validate`（从 URL 安装 skill/org 行为包） |
| 组织导入 | `import org <spec-file>`（PGO 特性，可用 `PGO_CLI_IMPORT_ENABLED=0` 关闭） |
| 系统管理 | `login` / `version` / `system info` / `config path / get / set` |

## 常用示例

```bash
# 交互式对话（默认系统管家 __system_admin__，可用 --agent 切换）
./bin/aranea chat --agent __spirit__

# 触发团队编排并查看运行记录
./bin/aranea team run <team_id> --content "盘前简报"
./bin/aranea team runs <team_id>

# 审计日志 / 事件 / 调用链
./bin/aranea monitor audit-logs
./bin/aranea monitor traces

# 批量导出 Agent 配置迁移到另一环境
./bin/aranea pack export --kind agent --ref <agent_id> -o agent.arpack
./bin/aranea pack import agent.arpack --strategy skip
```

## 设计要点

- CLI 与 Web 共用同一套 HTTP/WS API，无特权接口；
- 写操作默认交互确认，`-y/--yes` 跳过，便于脚本化；
- 输出格式全局旗标 `-o text|json`，`-q/--quiet` 精简输出，`--debug` 打印 HTTP 请求明细到 stderr。

## 深入阅读

- [25 CLI 开发文档](../../docs/development/25-cli.development.md)
