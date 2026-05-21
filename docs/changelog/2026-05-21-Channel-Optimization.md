# Channel 优化（2026-05-21）

## 摘要

完善 Channel 模块：修复凭据无法 runtime 解析的问题，增加连接状态定时检测与单元测试，同步开发计划文档。

## 变更

### 凭据加密（P2）

- `UpsertCredentials` 不再写入不可逆的 `local:` 哈希；改为 `enc:` + AES-GCM（依赖 `ARANEA_CREDENTIAL_KEY`）。
- `ResolveSecretRef` 支持 `enc:` 解密；`local:` 返回明确迁移提示。
- 新增 `internal/biz/channel_credential_crypto.go`。

### 连接状态检测（P3）

- `ChannelUsecase.RunHealthChecks` 对启用渠道执行 `EvaluateChannelTest` 并刷新 `status` / `metadata_json`。
- `internal/cronrunner/jobs/channel_health.go`（默认 10 分钟；`CHANNEL_HEALTH_DISABLED=1` 关闭）。
- Wire + `cmd/admin/main.go` 启动 scanner。

### 测试

- `internal/biz/channel_test.go`：`TestChannel*`（加密、Upsert、Evaluate、HealthChecks、normalize）。
- `internal/service/secret_ref_test.go`：`enc:` / `env:` / 废弃 `local:`。

### 文档

- 更新 `docs/需求/17-channel-development.md` 现状与任务清单，消除与 execution-plan 的矛盾描述。

## 验证

```bash
go test ./internal/biz/... -run TestChannel -count=1
go test ./internal/service/... -run TestResolveSecretRef -count=1
make wire && make build
```
