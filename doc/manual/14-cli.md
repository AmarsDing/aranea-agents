# 14 CLI 工具

## 功能

功能完整的命令行工具 `aranea`：无需打开浏览器即可管理 Agent、发起对话、监控运行状态。**Web 端能做的，CLI 都能做。**

## 原理

- **交互式 REPL**：`/agent`、`/session`、`/send` 等斜杠命令，类似 Claude Code 的交互体验；
- **WebSocket 实时流**：chat 命令通过 WS 接收流式响应，支持 thinking / tool / reply 事件；
- **多输出格式**：table / json / kv / text，适配脚本与人工阅读；
- **配置管理**：`~/.aranea/config.yaml` 管理 endpoint、token、默认 agent；
- **跨平台**：Windows / Linux / macOS，自动检测终端能力（颜色 / Unicode / TTY）；
- **脚本友好**：JSON 输出 + 退出码，便于 CI/CD 集成。

## 构建与登录

```bash
make cli                    # 构建到 ./bin/（Windows 为 aranea.exe）

./bin/aranea login --base-url http://localhost:8810 --user dev --password dev
```

`--base-url` 为全局持久旗标，登录后 token 存入本地配置。

## 命令分类（30+）

| 类别 | 命令 |
|------|------|
| Agent 管理 | `agent list` / `create` / `update` / `delete` / `batch-update` |
| Team 编排 | `team list` / `run` / `status` / `cancel` / `resume` |
| Graph 编排 | `graph list` / `run` / `status` / `cancel` / `resume` |
| Skill 管理 | `skill list` / `import` / `export` / `evolve` / `health` |
| 会话对话 | `chat` / `session list` / `messages` / `rename` / `batch-delete` |
| MCP / 工具 | `mcp list` / `test`，`tool list` / `grants` |
| Channel | `channel list` / `create` / `test` |
| 定时任务 | `cron list` / `create` / `trigger` |
| 系统监控 | `monitor dashboard` / `flow-log` / `trace` / `heal` |
| 系统管理 | `system info` / `version` / `login` / `logout` |
| 导入导出 | `pack export` / `pack import`（一键迁移 Agent 配置） |

## 常用示例

```bash
# 交互式对话精灵（进入 REPL 后直接输入消息）
./bin/aranea chat --agent __spirit__

# 触发团队编排并跟踪状态
./bin/aranea team run <team_key> --input "盘前简报"
./bin/aranea team status <run_id>

# 监控大盘 / 流日志
./bin/aranea monitor dashboard
./bin/aranea monitor flow-log --trace-id <id>

# 批量导出 Agent 配置迁移到另一环境
./bin/aranea pack export --agent ops_lead -o ops_lead.pack
./bin/aranea pack import ops_lead.pack
```

## 设计要点

- CLI 与 Web 共用同一套 HTTP/WS API，无特权接口；
- 批量操作（`batch-update` / `batch-delete`）服务端事务化；
- 输出格式全局旗标 `--output json|table|kv|text`。

## 深入阅读

- [25 CLI 开发文档](../../docs/development/25-cli.development.md)
