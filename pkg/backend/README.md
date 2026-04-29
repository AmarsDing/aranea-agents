# Backend

Aranea 控制平面只发一个二进制 `aranea`：

- 不带子命令进入 ADK 交互式控制台（`__system_admin__` Agent）。
- `aranea web` 启动 HTTP 后端（REST `/api/v1/*` + SSE 聊天流）。
- `aranea agent|skill|tool|plugin|mcp|cron|channel|monitor|session|system|config|version` 等子命令直接通过 REST 操作后端。

## 启动

```bash
go mod tidy
go run ./cmd/aranea web
```

> 等价的二进制流程：`go build -o aranea ./cmd/aranea && ./aranea web`。
> Windows 下 `go build -o aranea.exe ./cmd/aranea` 同理。

## 常见用法

```bash
# 1) 进入交互式控制台（默认 console 子启动器）
go run ./cmd/aranea

# 2) 在 :8080 启动后端
go run ./cmd/aranea web

# 3) 嵌入式 playground，绑定到随机端口、静音日志
go run ./cmd/aranea web --addr :0 --quiet

# 4) 远程运维（在另一台机器/终端连接同一后端）
ARANEA_BASE_URL=http://your-host:8080 go run ./cmd/aranea agent ls
ARANEA_BASE_URL=http://your-host:8080 go run ./cmd/aranea skill install https://github.com/owner/repo
```

## 环境变量

- `HTTP_ADDR`：监听地址，默认 `:8080`（被 `aranea web --addr` 覆盖）
- `DB_PATH`：SQLite 路径，默认 `data/arenea.db`（被 `aranea web --db` 覆盖）
- `SKILL_ROOT` / `SKILL_STORAGE_ROOT`：skill 目录覆盖（前者优先）
- `API_BASIC_USER` / `API_BASIC_PASS`：可选，开启 Basic Auth
- `ARANEA_BASE_URL` / `ARANEA_TOKEN`：CLI 子命令用于连接远程后端

## 数据存储

### SQLite（`DB_PATH`）

`internal/server/server.go` 解析顺序：`--db` flag → `DB_PATH` 环境变量 → 默认 `data/arenea.db`，**最后一项是相对于进程当前工作目录（CWD）的相对路径**，不会自动定位到仓库根目录。因此实际打开哪个文件取决于你在哪个目录里跑二进制：

| 启动方式（CWD） | 实际打开的文件 |
| --- | --- |
| `cd aranea/backend && go run ./cmd/aranea web` | `aranea/backend/data/arenea.db` ← **当前推荐** |
| `cd aranea/backend/cmd/server && ./server` | `aranea/backend/cmd/server/data/arenea.db`（历史目录，下文说明） |
| `DB_PATH=/abs/path/db.sqlite go run ./cmd/aranea web` | `/abs/path/db.sqlite` |

仓库里同时出现两份 `arenea.db` 是历史遗留：早期入口位于 `cmd/server/main.go`，从该目录启动会把 SQLite 写入相邻的 `data/`；当前入口已迁移到 `cmd/aranea/main.go`（见 `aranea/backend/cmd/aranea/main.go`），所以 `aranea/backend/cmd/server/` 下只剩一个空的 `data/` 目录留有旧 db 文件，**不再被读取**，可以安全删除或归档。启动时的日志（`server listening on …` 之前的 `init repository`）以及 `migrate` 行为都只会作用在 `data/arenea.db`（即 `aranea/backend/data/arenea.db`，假设你按推荐方式从 `aranea/backend/` 启动）。

### Skill 存储目录

由 `internal/util/storage_path.go::ResolveSkillStorageRoot` 决定，解析顺序：

1. `SKILL_ROOT` 环境变量（优先）
2. `SKILL_STORAGE_ROOT` 环境变量
3. 默认值，按平台落到 **用户配置目录** 下的 `Aranea/skills/`：

| OS | 默认路径 |
| --- | --- |
| Windows | `%APPDATA%\Aranea\skills`（即 `C:\Users\<user>\AppData\Roaming\Aranea\skills`） |
| macOS | `~/Library/Application Support/Aranea/skills` |
| Linux | `~/.config/aranea/skills` |
| 兜底（无 home 目录） | `data/skills`（CWD 相对） |

启动时会把解析后的绝对路径写到日志：`skill storage root: <path>`。`aranea skill install …` 把内容拉到该目录，`SkillService.StartDirectorySync` 也会周期性扫描它来同步数据库元数据。

## 核心接口

- `GET  /healthz`
- `GET/POST /api/v1/agents`
- `GET/POST /api/v1/sessions`
- `GET/POST /api/v1/chat/messages`
- 完整列表见 `docs/25 cli.md` §6.2。
