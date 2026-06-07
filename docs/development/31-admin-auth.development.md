# Admin & Auth — 开发计划

> **版本**：2026-06-06 | **状态**：Phase 0/1 已落地；PAT 待 Phase 2
> **设计**：[admin-auth.design.md](./admin-auth.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)

---

## 1. 模块定位

管理端身份认证：管理员登录、会话 Cookie/JWT、HTTP/WS/gRPC 鉴权、开发 bypass、Webhook 与 Admin 凭证分离。

**代码锚点**：

| 层级 | 路径 |
|------|------|
| 认证库 | `pkg/auth/`（`middleware.go`、`token.go`、`cookie.go`、`request_token.go`、`health.go`、`features.go`、`webhook.go`、`grpc_middleware.go`） |
| HTTP 注册 | `internal/server/http.go` |
| WS | `internal/server/ws.go` |
| 登录 RPC | `internal/service/admin.go` |
| 前端 | `web/src/stores/auth.ts`、`web/src/pages/LoginPage.vue`、`web/src/features/admin/loginErrors.ts`、`web/src/config/runtime.ts` |

---

## 2. 现状评估

| 项 | 状态 | 说明 |
|----|------|------|
| JWT + Cookie 登录 | ✅ | `POST /v1/admins/login` → HttpOnly `Set-Cookie`（SameSite=Lax） |
| HTTP Bearer 回退 | ✅ | `Authorization: Bearer` + Cookie + query `token`（三级提取） |
| WS 同源 Cookie | ✅ | 不再依赖 JS 读 token；跨源可显式 `?token=` |
| `/healthz` auth_mode | ✅ | `auth_mode` / `cookie_name` / `ws_path` / `deploy_env` |
| 登录错误分类 | ✅ | `loginErrors.ts` 区分网络 / 凭据 / 服务端错误 |
| 开发 bypass | ✅ | 登录页提示 +「进入系统（免登录）」 |
| Cookie HttpOnly | ✅ | `cookie.go` HttpOnly=true + SameSite=Lax + 可选 Secure |
| Admin CRUD | ✅ | 创建/更新/删除/查询管理员（需 admin 权限） |
| gRPC 鉴权 | 🟡 | Bearer token 解析，无 token 仅 warning 不拒绝 |
| 密码哈希 | 🟡 | MD5（待升级 bcrypt，Phase 3） |
| PAT / API Key | ❌ | Phase 2 |
| RBAC | ❌ | 仅 JWT `access` 字段（二元：admin/非 admin） |
| SSO/OIDC | ❌ | Phase 3 |

---

## 3. 任务清单

| # | 任务 | 优先级 | 状态 |
|---|------|--------|------|
| 1 | 用户友好认证设计文档 | P0 | ✅ |
| 2 | README / 登录页：9001 与两种 dev 模式 | P0 | ✅ |
| 3 | 登录错误区分网络 vs 凭据 | P1 | ✅ |
| 4 | Cookie HttpOnly + WS 纯 Cookie 同源 | P1 | ✅ |
| 5 | `/healthz` 暴露 `auth_mode` | P2 | ✅ |
| 6 | SameSite-Lax + 可选 Secure 配置 | P2 | ✅ |
| 7 | Token 三级提取（Cookie > Bearer > query） | P2 | ✅ |
| 8 | Admin CRUD（创建/更新/删除/查询） | P2 | ✅ |
| 9 | Personal Access Token CRUD + Bearer `arn_pat_*` | P2 | ⏳ |
| 10 | gRPC 生产强制认证 | P3 | ⏳ |
| 11 | 密码哈希升级 MD5 → bcrypt | P3 | ⏳ |
| 12 | RBAC + 多租户 | P3 | ⏳ |
| 13 | SSO/OIDC | P3 | ⏳ |

---

## 4. 验收标准

- [x] [admin-auth.design.md](./admin-auth.design.md) 已编写
- [x] Phase 0/1 实现（HttpOnly、healthz、登录 UX、WS Cookie）
- [x] Cookie HttpOnly + SameSite-Lax + 可选 Secure
- [x] Token 三级提取：Cookie > Bearer > query
- [x] 登录错误分类（网络 / 凭据 / 服务端）
- [x] `/healthz` 暴露 auth_mode 诊断信息
- [x] Admin CRUD 端点
- [x] 文档区分 JWT 签名密钥 / 会话 Cookie / Webhook secret
- [ ] Phase 2 PAT 与 CLI `ARANEA_TOKEN`
- [ ] 密码哈希从 MD5 升级到 bcrypt
- [ ] RBAC 角色与权限点
- [ ] gRPC 生产强制认证

---

## 5. 依赖与风险

- 跨域部署（`runtime-config.json` 设 `backendUrl`）需继续支持 WS `?token=` 或统一域名
- PAT 需审计与吊销
- 密码算法升级需兼容旧 MD5 一次迁移
- gRPC 当前无 token 仅 warning，生产环境需内网隔离或强制认证
