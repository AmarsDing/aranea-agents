# Aranea CLI 快速上手指南

> 10 分钟内跑通 5 个核心命令。

## 前置要求

- 后端服务已启动（默认 `http://127.0.0.1:8080`）
- 已有管理员账号（用户名 + 密码）

## 1. 安装 CLI

### 方式一：从源码编译

```bash
git clone https://github.com/your-org/aranea-agents.git
cd aranea-agents
go build -o ./bin/aranea ./cmd/aranea
# 或者（Makefile，POSIX shell）：
make cli
```

编译完成后 `./bin/aranea` 即为可执行文件。可选择将其拷贝至 `$PATH`：

```bash
cp ./bin/aranea /usr/local/bin/
```

### 方式二：Windows（PowerShell）

```powershell
go build -o .\bin\aranea.exe .\cmd\aranea
```

---

## 2. 配置后端地址

### 环境变量（临时）

```bash
export ARANEA_BASE_URL=http://your-backend:8080
```

### 配置文件（持久）

查看配置文件路径：

```bash
aranea config path
```

编辑或通过命令设置：

```bash
aranea config set backend.base_url http://your-backend:8080
```

---

## 3. 登录

```bash
aranea login
```

按提示输入用户名和密码。成功后 token 将被写入配置文件（权限 0600），
后续命令自动携带认证信息。

```
✓  登录成功  user=admin@example.com
```

---

## 4. 查看系统信息

```bash
aranea system info
```

输出示例：

```
backend_url    http://127.0.0.1:8080
version        v1.0.0
provider       openai
model          gpt-4o
```

---

## 5. 列出 Agent

```bash
aranea agent ls
```

输出示例（TTY）：

```
id                                    display_name                    status
----------------------------------------------------------------------------------
agent-001                             客服助手                         active
agent-002                             代码审查                         inactive

Total: 2
```

JSON 模式（可被 `jq` 解析）：

```bash
aranea agent ls --output json | jq '.items[0].id'
```

---

## 6. 常用命令速查

### Agent 管理

| 命令 | 说明 |
|------|------|
| `aranea agent ls` | 列出所有 Agent |
| `aranea agent get <id>` | 查看单个 Agent 详情 |
| `aranea agent create -f agent.json` | 从 JSON 文件创建 Agent |
| `aranea agent update <id> -f update.json` | 更新 Agent |
| `aranea agent enable <id>` | 启用 Agent |
| `aranea agent disable <id>` | 停用 Agent |
| `aranea agent delete <id> --yes` | 删除 Agent（需要 `--yes` 确认） |
| `aranea agent tools <id>` | 查看 Agent 可用工具 |
| `aranea agent tools-set <id> --allow browser,code` | 设置工具策略 |

### Skill 管理

| 命令 | 说明 |
|------|------|
| `aranea skill ls` | 列出所有 Skill |
| `aranea skill get <id>` | 查看 Skill 详情 |
| `aranea skill enable <id>` | 启用 Skill |
| `aranea skill disable <id>` | 停用 Skill |
| `aranea skill publish <id>` | 发布 Skill |
| `aranea skill delete <id> --yes` | 删除 Skill |

### Tool 管理

| 命令 | 说明 |
|------|------|
| `aranea tool ls` | 列出所有 Tool |
| `aranea tool get <id>` | 查看 Tool 详情 |
| `aranea tool enable <id>` | 启用 Tool（需二次确认） |
| `aranea tool disable <id>` | 停用 Tool（需二次确认） |

### Package 安装与聊天

| 命令 | 说明 |
|------|------|
| `aranea pkg install <git-url>` | 从 Git 仓库安装 `aranea-package.yaml` 描述的 MCP/Skill/Agent/Team/Graph |
| `aranea pkg install <git-url> --strict=false` | 保留安装汇总，但部分失败不让进程返回非 0 |
| `aranea pkg validate <git-url>` | 仅克隆并校验 manifest，不写入后端 |
| `aranea chat` | 进入 REPL，默认连接系统管家 `__system_admin__` |
| `aranea chat --agent <agent-key>` | 进入 REPL 并指定 Agent |

### 版本与认证

| 命令 | 说明 |
|------|------|
| `aranea version` | 显示本地版本与后端可达性 |
| `aranea login` | 登录并保存 token |
| `aranea config path` | 查看配置文件路径 |
| `aranea config get <key>` | 读取配置项（token 默认脱敏） |
| `aranea config set <key> <value>` | 写入配置项 |

---

## 7. 全局选项

所有命令均支持以下全局标志：

| 标志 | 说明 |
|------|------|
| `--output text\|json` | 输出格式，默认 `text` |
| `--quiet` / `-q` | 简洁输出（仅 ID） |
| `--yes` / `-y` | 跳过交互确认（用于脚本） |
| `--debug` | 在 stderr 输出 HTTP 请求/响应（token 已脱敏） |
| `--timeout` | 全局 HTTP 超时秒数，默认 60 |
| `--no-color` | 禁用 ANSI 颜色 |
| `--base-url` | 临时覆盖后端地址 |
| `--token` | 临时覆盖认证 token |
| `--config` | 指定配置文件路径 |

---

## 8. 环境变量

| 变量 | 说明 |
|------|------|
| `ARANEA_BASE_URL` | 后端地址（优先于配置文件） |
| `ARANEA_TOKEN` | 认证 token（优先于配置文件） |
| `ARANEA_CLI_ADMIN_BASE_URL` | 系统管家 `cli_admin_*` 工具回调后端 API 的地址 |
| `ARANEA_CLI_ADMIN_TOKEN` | 系统管家 `cli_admin_*` 工具回调后端 API 的 Bearer token |
| `ARANEA_OUTPUT` | 输出格式（`text` 或 `json`） |
| `NO_COLOR` | 设为任意非空值可禁用颜色 |

---

## 9. 常见问题

**Q: 登录后提示 `UNAUTHENTICATED`？**

运行 `aranea config get backend.token` 确认 token 是否已保存。若后端重启可能需要重新登录。

**Q: 输出乱码或无颜色？**

尝试 `--no-color` 标志，或设置 `NO_COLOR=1` 环境变量。

**Q: 后端不可达（exit 3）？**

1. 确认后端服务已启动
2. 检查 `ARANEA_BASE_URL` 或 `aranea config get backend.base_url`
3. 使用 `aranea version` 测试连通性

**Q: 想在 CI/脚本中使用？**

推荐组合：`--output json --quiet --yes`，并通过 `ARANEA_TOKEN` 环境变量注入 token，避免交互提示。

```bash
ARANEA_TOKEN=<token> aranea agent ls --output json --quiet | jq -r '.items[].id'
```

**Q: WebSocket token 会出现在 URL 里吗？**

不会。CLI 的聊天 WebSocket 只在 URL 中携带 `session_id`，认证 token 通过 `Authorization: Bearer` header 发送，错误信息不会回显 query 或 token。

---

## 10. 下一步

- 查看完整命令：`aranea --help`
- 查看子命令帮助：`aranea agent --help`
- [AI-DEVELOPMENT-SPECIFICATION.md](./AI-DEVELOPMENT-SPECIFICATION.md) — 后端开发规范
