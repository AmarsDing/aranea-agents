# Memory Legacy Backfill — 启动死锁修复与迁移架构

**日期**：2026-05-24

## 问题

`go run ./cmd/admin` 在遥测 noop 日志后长时间无输出，HTTP `:8000` 未监听；前端代理 `ECONNREFUSED`。

**根因**：`ensureAllSchemas` → `BackfillLegacyTRPCMemoryEntities` 在 SQLite **同一连接**上对 `memory_entities` 保持 `SELECT` 游标的同时 `UPDATE` 同表，触发 SQLite 锁等待（表现为 indefinite 阻塞）。

**影响范围**：含 `scope_type='trpc_memory'` 且未 `migrated` 的存量库；与 Legacy 旧 trpc Memory 写路径相关，**非** L3 目标态运行时逻辑。

## 修复与优化

| 优先级 | 项 | 状态 |
|--------|-----|------|
| P0 | SQLite 游标死锁修复（先读后写） | ✅ |
| P1 | backfill 单测 + `[startup]` step 日志 | ✅ |
| P2 | `schema_migrations` 表 · version `20260524` gate | ✅ |
| P3 | 拆分 `ensureSchemaDDL` / `runPendingDataMigrations` | ✅ `MemoryDataMigrationWorker` · kratos `AfterStart` |
| P4 | `cmd/memory-migrate legacy-trpc-facts` | ✅ |

**Review 跟进（同日）**：

| 项 | 状态 |
|----|------|
| 无效 legacy 行 `status=skipped` | ✅ |
| `schema_migrations` 进 Ent schema | ✅ |
| post-HTTP 数据迁移 worker | ✅ |
| invalid pending 回归测试 | ✅ |
| CLI 勿与 admin 同 DSN 文档 | ✅ |

**架构立场**：Legacy（`trpc_memory`、`memory_items`）属旧业务系统兼容层；数据迁移在 HTTP listen 后由 `MemoryDataMigrationWorker` 执行（可用 `MEMORY_DATA_MIGRATION_DISABLED=1` 关闭）。

## 涉及文件

- `internal/data/sessionmemory/store_legacy_backfill.go`
- `internal/data/sessionmemory/store_legacy_backfill_test.go`
- `internal/data/schema_migrations.go`
- `internal/data/memory_migrate.go`
- `internal/data/memory_migrate_test.go`
- `internal/data/data.go`
- `internal/data/ent/schema/schema_migration.go`
- `internal/cronrunner/jobs/memory_data_migration.go`
- `cmd/memory-migrate/main.go`

## 文档

- `docs/需求/memory/memory.design.md` §3.1、§十一
- `docs/需求/memory/L3.design.md` §3.8
- `docs/需求/memory/memory-development.md` §7
- `docs/需求/memory/README.md` §3

## 运维

**离线 CLI**：执行 `--apply` 前须停止 `go run ./cmd/admin`（或任何占用同一 SQLite 文件的进程），否则 Windows 上易出现锁等待。路径可用 `ARANEA_SQLITE_PATH` 覆盖。

```powershell
# 查看 pending（不写入）
go run ./cmd/memory-migrate legacy-trpc-facts --dry-run

# 离线应用（与启动时 gate 相同）
go run ./cmd/memory-migrate legacy-trpc-facts --apply
```

启动日志示例：

```text
[startup] initSQLite done in 120ms
[startup] ensureSchemaDDL done in 45ms
memory data migration worker started
memory data migration skipped (version 20260524 applied)
```
