# A2A 公开地址迁入 System Settings（2026-05-20）

## 变更

- `system_settings.a2a_public_base_url`（Ent + SQLite patch）
- 优先级：`A2A_PUBLIC_BASE_URL` (env) > **系统设置 DB** > `server.a2a_public_base_url` (yaml) > 推导
- `PublicBaseURLStore` 热更新；保存系统设置后 `A2APublicBaseReloader` 立即生效并清空 Endpoint 缓存
- UI：`/settings` 可编辑；`/a2a` Banner 链接至系统设置

## 验证

- `make api && make generate && make wire`
- `go test ./internal/a2a/... ./internal/biz/... ./internal/service/... ./internal/data/...`
- `cd web && pnpm test && pnpm build`
