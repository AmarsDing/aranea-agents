# 运维手册（Stage-1）

## 日常巡检
- 检查 `/healthz` 返回 `status=ok`。
- 检查审计接口 `/api/v1/monitor/audit` 是否持续有写入。
- 检查 SQLite 文件大小增长和磁盘空间。

## 备份策略
- 每日定时执行 `scripts/backup-sqlite.ps1`。
- 至少保留 7 天备份。
- 每周做一次恢复演练（`scripts/restore-sqlite.ps1`）。

## 故障排查
- 通过访问日志定位 `request_id`。
- 在审计日志定位资源变更记录。
- 出现高频 429 时，调优限流或拆分客户端流量。
