# 商用能力补齐清单

## 已落地（当前版本）
- 基础认证：通过 `API_BASIC_USER` / `API_BASIC_PASS` 开启 Basic Auth。
- 基础限流：按来源地址做分钟级请求上限。
- 请求追踪：统一 `X-Request-Id` 透传。
- 访问日志：记录 method/path/status/latency/request_id。
- 审计日志：Agent/Session/Message 写操作自动写入 `audit_logs`。
- 备份恢复：`scripts/backup-sqlite.ps1` 与 `scripts/restore-sqlite.ps1`。

## 下一步（商用强化）
- 升级认证到 JWT/OIDC/SSO。
- RBAC 细粒度授权（角色、资源、动作）。
- 接入完整 OpenTelemetry（trace + metrics + log correlation）。
- 实现熔断、重试策略与任务队列背压。
- 敏感字段加密与日志脱敏策略。
