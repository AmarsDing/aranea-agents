# 插件运行时与 SyncBuiltins（Phase 5 声明）

按计划 **不迁移 `SyncBuiltins` / ADK 运行时装配** 至 `cmd/admin`，而是**刻意保留在 `pkg/backend`**，直至下列条件成立之一后再评估收口或删除：

1. **`cmd/admin`** 内 **原生 `chat/v1`** 对话执行链完全接管 ADK/embed，且不再需要遗留进程侧的插件装载与内置种子。
2. 产品确认 **`PluginService.SyncBuiltins`**（见 `pkg/backend/internal/service/plugin_service.go`）所写入的 **`plugins`  catalog** 行可由 **仅靠 Ent + `internal/data` 运维脚本**（或一次性 Job）等价替代。

## 运行时边界

| 能力 | 位置 |
|------|------|
| `BuiltinPluginDefinitions` / ADK 内置插件装配 | **`pkg/backend/internal/conversation/adapters/adkruntime`** |
| `SyncBuiltins`（SQLite `plugins` 表 upsert） | **`pkg/backend/internal/service`** |
| Catalog CRUD、gRPC **`plugin/v1`** | **`cmd/admin`** |

运维上若仍以 admin 为主进程，需明确：**builtin 插件元数据是否与遗留库共用同一 SQLite**；若为双进程，遵循 [runbook-operational-baseline.md](runbook-operational-baseline.md) 单写方约定。

## 退役检查清单

- [ ] 原生 chat 不再需要遗留 HTTP `/api/v1/chat/*`。
- [ ] admin 插件表与运行时所需行的来源（manual / migrate job）文档化。
- [ ] CLI `aranea monitor` 等仍依赖遗留路径的命令已替换或归档。
