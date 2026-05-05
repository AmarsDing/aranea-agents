# Legacy → Kratos 运维基线（Phase 0 对账）

本文与 [pkg-backend-to-kratos.md](pkg-backend-to-kratos.md) §6、§7 配套，供部署与迭代对账。**不取代** §6.3 明细。**Phase 状态总表**：[phase-status.md](phase-status.md)。

## 1. 环境变量（`cmd/admin`）

| 变量 | 行为摘要 |
|------|-----------|
| `LEGACY_REST_ORIGIN` | 指向实现 **`/api/v1/chat/*`** 的上游根 URL（无尾 `/`）。未设置：**`/v1/chat/*`** 返回 **503**；**cronrunner** **`postChat`** 失败（需显式报错）。 |
| `CRON_RUNNER_INTERVAL` | `internal/cronrunner` tick；非法或空 → **1m**。 |
| `CRON_RUNNER_DISABLED` | 为 **`1`** 时不启动 runner，仅保留 **`cron/v1` CRUD**。 |
| **`CRON_CHAT_DISPATCH_ORIGIN`**（推荐） | 若设置：**`internal/cronrunner`** **`postChat`** 向 **`POST {origin}/v1/chat/messages`**（**admin 网关 / kratos 入口**）。未设置时回退 **`LEGACY_REST_ORIGIN`** → 遗留 **`/api/v1/chat/messages`**。用于 Phase 4 去遗留；与 **`LEGACY_REST_ORIGIN`** 可指向同一 admin 主机，仅路径前缀不同。 |
| **`CHAT_RECORD_USAGE_INGRESS`** | 若为 **`1` / true / yes**： unary **`POST /v1/chat/messages`** 在 legacy 成功返回且 **`agent_message` 含 token 字段**时，额外调用 **`UsageUsecase.RecordTokenUsageEvent`** 写入 **`cmd/admin`** 侧 **`model_token_usage_events`**。**默认不设**，避免与 **`pkg/backend`** 同源双写；迁移「单一写入方」时：**关闭遗留侧同源写入后再开启**，或仅存一处。 |

## 2. 双进程与 SQLite

- 迁移期 **`cmd/admin`** 与 **`pkg/backend`（或其它上游）** 可能**同台**挂载同一 **`*.sqlite`**：须约定**单写入方**或使用**只读副本**，避免锁与损坏。
- **`cmd/admin`** 内 **`NewData`** 仅 **`ent.Open`** 一处 SQLite——禁止并联第二套 **`sql.Open` 同源 DSN**（见 §2 硬约束）。

## 3. 双 Cron 数据源

| 数据来源 | 进程 |
|-----------|------|
| Ent 表 **`cron_task`** | **`internal/cronrunner`（cmd/admin）** |
| **`platform_resources` 资源行 cron-tasks** | 遗留 **`pkg/backend CronRunner`**（若仍启用） |

二者**不等价**；同一业务任务不得在两侧重复配置。产品应明确：**仅 Ent** 或**仅遗留**派发。

## 4. 用量语义双写

- **`POST /v1/usage/token-events`** 与遗留 chat **同一轮对话**末尾写入：见 [pkg-backend-to-kratos.md](pkg-backend-to-kratos.md) **§6.3.2**。
- 目标：**单一写入方**；浏览器 ingest 需 feature flag / 明示关闭。

## 5. `pkg/backend` 路由骨架（`transport/handler.go` `registerRoutes`）

以下为旧前缀 **`/api/v1/...`** 分组示意，便于与 **`api/kratos/**`** 及网关 **`/v1/...`** 对账。**权威契约以 proto `google.api.http` 为准。**

| 分组 | 旧路径示例 |
|------|-------------|
| 健康 | `/healthz` |
| Agent / Team | `/api/v1/agents*`、`teams*`、`team-runs*`、`team-run-events` |
| 平台资源 | `/api/v1/agent-categories*`、`llm-provider-models*`、`avatar-assets`、`hooks`、`mcp-servers`、`cron-tasks*`、`cron-task-runs`、`plugins*` |
| 通道 / 技能 / 工具 | `/api/v1/channels*`、`/api/v1/skills*`、`skill-runs`、tools（cap adapter） |
| 会话 / 聊天 | `/api/v1/sessions*`、`/api/v1/chat/messages`、`/messages/stream`、`/options` |
| 记忆 / 进化 | memory 适配器注册路由、`evolution` 注册路由 |
| 用量 | `/api/v1/model-usage/*` |
| 监控 | `/api/v1/monitor/*`（含 `logs/stream`） |

多数管理面已由 **`cmd/admin`** **`/v1/...`** 承接；仍依赖上游的主要为 **聊天发送 / 流式 / options**（经 **`LEGACY_REST_ORIGIN`**）及 **Cron `postChat`** 直至原生 **`chat/v1`** 与派发路径收口。

---

*文档版本：与迁移计划 Phase 0 同步新增。*
