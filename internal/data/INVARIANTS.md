# internal/data 不变量档案（INVARIANTS）

> ent schema / 迁移 SQL / 仓储层的硬约束。每条 = 不变量 / 为什么（事故史）/ 违反代价。

## INV-DATA-1 迁移 SQL 与注册行同 commit

- **不变量**：新增/修改迁移时，`internal/data/sql/migrations/<N>_<name>.sql` 与 `ddl_migration_table*.go` 中的注册行**必须同一次 commit**。
- **为什么**：两者分离提交会让中间态的 origin/dev 编译/启动即红（2026-10-20 6d69b2761 双红先例）。
- **违反代价**：共享分支对全体会话红。

## INV-DATA-2 迁移重编号必须连 SQL 文件名一起改

- **不变量**：迁移编号变更 = 注册行编号 + SQL 文件名编号同步改（canonical filename guard 双方向）。
- **为什么**：只改注册行会留孤儿 SQL 文件，后续会话按文件名重建注册行时编号冲突（20261279 转正先例 6d69b2761）。
- **违反代价**：迁移顺序错乱、重复登记。

## INV-DATA-3 ent 单写者 + 禁 stash pop 复活备份

- **不变量**：共享树上 ent codegen 同时只有一个写者；禁止用 stash pop 恢复 ent 相关备份（腐坏备份复活是二次腐坏来源）；执行环境 `DEPLOY_ENV=dev` + `KRATOS_HTTP_AUTH_DISABLED=1`。
- **为什么**：2026-10-21 ent 双腐坏事故复盘结论。
- **违反代价**：生成物静默腐坏，且腐坏态会被当作「既有事实」继续扩散。

## INV-DATA-4 PG 单引擎

- **不变量**：数据层只面向 PostgreSQL（SQLite 已全量移除，14fcb6d79）；`.ent-head-overlay-parked` 保持 parked 状态勿改回；测试用真实 PG（默认 DSN 内置 `postgres://postgres:123456@127.0.0.1:55432/aranea_test`，见 testhelper/pg.go；CI/非标环境用 `ARANEA_TEST_PG_DSN` 覆盖）。
- **为什么**：数据库架构红线（docs/development/66-database-architecture.md）。
- **违反代价**：双引擎语义漂移用测试绿伪装、生产炸。

## INV-DATA-5 跨表/跨 Repo 写包事务

- **不变量**：跨两张表或跨两个 Repo 的写操作必须单事务提交（红线 #24）。
- **为什么**：部分提交会在恢复路径上产生「写了一半」的幽灵状态。
- **违反代价**：对账/恢复逻辑无法收敛。
