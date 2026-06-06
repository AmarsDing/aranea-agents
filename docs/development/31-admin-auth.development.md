# Admin & Auth — 开发计划

> **版本**：2026-05-20 | **状态**：Phase 0/1 已落地；PAT 待 Phase 2  
> **设计**：[admin-auth.design.md](./admin-auth.design.md)  
> **进度真相**：[execution-plan.md](../guides/execution-plan.md)

---

## 1. 模块定位

管理端身份认证：管理员登录、会话 Cookie/JWT、HTTP/WS/gRPC 鉴权、开发 bypass、Webhook 与 Admin 凭证分离。

**代码锚点**：

| 层级 | 路径 |
|------|------|
| 认证库 | `pkg/auth/`（`middleware.go`、`token.go`、`cookie.go`、`request_token.go`、`health.go`） |
| HTTP 注册 | `internal/server/http.go` |
| WS | `internal/server/ws.go` |
| 登录 RPC | `internal/service/admin.go` |
| 前端 | `web/src/stores/auth.ts`、`web/src/pages/LoginPage.vue`、`web/src/features/admin/loginErrors.ts` |

---

## 2. 现状评估

| 项 | 状态 | 说明 |
|----|------|------|
| JWT + Cookie 登录 | ✅ | `POST /v1/admins/login` → HttpOnly `Set-Cookie` |
| HTTP Bearer 回退 | ✅ | `Authorization: Bearer` + Cookie |
| WS 同源 Cookie | ✅ | 不再依赖 JS 读 token；跨域可显式 `?token=` |
| `/healthz` auth_mode | ✅ | `auth_mode` / `cookie_name` / `deploy_env` |
| 登录错误分类 | ✅ | `loginErrors.ts` + Login 页 |
| 开发 bypass | ✅ | 登录页提示 +「进入系统（免登录）」 |
| PAT / API Key | ❌ | Phase 2 |
| RBAC | ❌ | 仅 JWT `access` 字段 |
| SSO/OIDC | ❌ | Phase 3 |
| 密码哈希 | 🟡 | MD5（待升级 bcrypt） |

---

## 3. 任务清单

| # | 任务 | 优先级 | 状态 |
|---|------|--------|------|
| 1 | 用户友好认证设计文档 | P0 | ✅ |
| 2 | README / 登录页：9001 与两种 dev 模式 | P0 | ✅ |
| 3 | 登录错误区分网络 vs 凭据 | P1 | ✅ |
| 4 | Cookie HttpOnly + WS 纯 Cookie 同源 | P1 | ✅ |
| 5 | `/healthz` 暴露 `auth_mode` | P2 | ✅ |
| 6 | Personal Access Token CRUD + Bearer | P2 | ⏳ |
| 7 | RBAC + 多租户 | P3 | ⏳ |

---

## 4. 验收标准

- [x] [admin-auth.design.md](./admin-auth.design.md) 已编写
- [x] Phase 0/1 实现（HttpOnly、healthz、登录 UX）
- [ ] Phase 2 PAT 与 CLI `ARANEA_TOKEN`
- [x] 文档区分 JWT 签名密钥 / 会话 Cookie / Webhook secret

---

## 5. 依赖与风险

- 跨域部署（`runtime-config.json` 设 `backendUrl`）需继续支持 WS `?token=` 或统一域名
- PAT 需审计与吊销
- 密码算法升级需兼容旧 MD5 一次迁移
