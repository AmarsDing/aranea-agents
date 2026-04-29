# SQLite -> PostgreSQL/Redis/K8s 平滑升级路线

## 阶段A（当前）
- 数据层：SQLite 单文件。
- 存储抽象：通过 `repository.Store` 接口解耦业务服务。
- 部署：单机 docker-compose。

## 阶段B（数据库升级）
- 新增 `PostgresRepository`，实现同一 `repository.Store` 接口。
- 使用双写/回放工具把 SQLite 历史数据迁入 PostgreSQL。
- 在配置层增加 `DB_DRIVER=sqlite|postgres`，支持无代码切换。

## 阶段C（缓存与队列）
- 引入 Redis：
  - 会话热数据缓存。
  - 限流计数器共享。
  - 异步任务队列（技能构建、批量任务、定时任务执行）。

## 阶段D（集群化）
- 后端无状态化，数据库外置。
- 使用 Kubernetes：
  - `Deployment`（backend/frontend）
  - `Service` + `Ingress`
  - `ConfigMap` + `Secret`
  - HPA（按 CPU/QPS 扩缩容）

## 阶段E（生产治理）
- 蓝绿/金丝雀发布。
- 自动化回滚与数据库迁移守护。
- SLO: 响应时间、错误率、任务成功率。
