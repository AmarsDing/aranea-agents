# Changelog — A2A Code Review 修复（配置落地 + 质量优化）

**日期**：2026-05-20  
**模块**：A2A（26）、配置（conf）、前端 `/a2a`

## 问题清单与处理状态

| # | 级别 | 问题 | 处理 |
|---|------|------|------|
| 1 | P1 | 生产 `A2A_PUBLIC_BASE_URL` 未文档化/无告警 | ✅ conf + env + 启动 warn |
| 2 | P2 | `provideA2APublicBaseURL` 仅处理 `:port` | ✅ `ResolvePublicBaseURL` 支持 `0.0.0.0`/`::` |
| 3 | P2 | 无 scheme 配置 | ✅ env/conf 可填完整 HTTPS URL |
| 4 | P1 | `BuildGraphResumeMetadata` 与 trpc envelope 不兼容 | ✅ 改为 flattened metadata 根级字段 |
| 5 | P2 | `GetAgentCard` 无行返回 disabled 空卡 | ✅ 改为 `ErrNotFound` |
| 6 | P2 | 本地 disabled 仍 fallback 远程同 ID | ✅ `ResolveInvokeTarget` |
| 7 | P2 | 远程 Invoke 未传 capability | ✅ `metadata.aranea_capability` |
| 8 | P2 | Discover 吞掉 `ListRemoteAgents` 错误 | ✅ 向上返回 error |
| 9 | P3 | `RegisterRemoteAgent.enabled` 写死 true | ✅ proto `optional bool` + service 默认 true |
| 10 | P3 | `agentIDsFromCards` 重复 | ✅ `biz.AgentIDsFromCards` |
| 11 | P3 | Gateway health 无 timeout | ✅ 10s context |
| 12 | — | 配置无 UI 入口 | ✅ `GET /v1/a2a/config` + `A2ARuntimeConfigBanner` |
| 13 | — | 配置未进 conf.yaml | ✅ `server.a2a_public_base_url` |

## 配置优先级（落地）

```
A2A_PUBLIC_BASE_URL (env)  >  server.a2a_public_base_url (YAML)  >  推导 http://127.0.0.1:{port}/v1/a2a/public
```

- 文件：`configs/config.yaml` → `server.a2a_public_base_url`
- 环境变量：`A2A_PUBLIC_BASE_URL`
- **UI**：`/a2a` 页只读 Banner（`A2ARuntimeConfigBanner`），非 System Settings（部署项，非运行时 DB 配置）

## 代码

- `internal/a2a/public_base_url.go` · `callee_resolve.go` · `capability_metadata.go`
- `internal/conf/conf.proto` · `cmd/admin/wire.go`（warn + struct inject）
- `api/kratos/a2a/v1/a2a.proto`：`GetA2AConfig` · `optional enabled`
- `internal/data/a2a.go` · `internal/service/a2a.go` · `internal/biz/a2a.go`

## 验证

- `make config && make api && make wire`
- `go test ./internal/a2a/... ./internal/biz/... ./internal/service/...`
- `pnpm test && pnpm build`

## 仍待 Phase 4

- Admin Invoke 流式（低优先级）
- 网关健康 Cron + 指标
- API 速率限制
