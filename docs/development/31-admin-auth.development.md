# Admin & Auth — 开发计划

> **版本**：2026-06-06 | **状态**：Phase 0/1 已落地；PAT 待 Phase 2
> **需求**：[admin-auth.md](./admin-auth.md) · **设计**：[admin-auth.design.md](./admin-auth.design.md)

---

## 1. 模块定位

管理端身份认证：管理员登录、会话 Cookie/JWT、HTTP/WS/gRPC 鉴权、开发 bypass、Webhook 与 Admin 凭证分离。

---

## 2. 代码锚点

| 层级 | 路径 |
|------|------|
| Proto 契约 | `api/kratos/admin/v1/admin.proto` |
| 认证库 | `pkg/auth/`（`middleware.go`、`token.go`、`config.go`、`cookie.go`、`request_token.go`、`health.go`、`features.go`、`webhook.go`、`grpc_middleware.go`、`auth.go`） |
| HTTP 注册 | `internal/server/http.go`（`auth.Middleware` + `RegisterWebhookPath`） |
| WS 鉴权 | `internal/server/ws.go`（`wsAuthenticate`） |
| 登录 RPC | `internal/service/admin.go` |
| 业务层 | `internal/biz/admin.go`（`AdminReader`/`AdminWriter`/`AdminUsecase`） |
| 数据访问 | `internal/data/admin.go` |
| 数据 Schema | `internal/data/ent/schema/admin.go` |
| 种子账号 | `internal/data/bootstrap_dev_admin.go`、`internal/data/bootstrap_initial_admin.go` |
| 前端会话 | `web/src/stores/auth.ts` |
| 登录页 | `web/src/pages/LoginPage.vue` |
| 登录错误 | `web/src/features/admin/loginErrors.ts` |
| WS URL | `web/src/config/runtime.ts`（`buildWsUrl`，`readAccessTokenCookie` 已 deprecated） |

---

## 3. 现状评估

| 项 | 状态 | 说明 |
|----|------|------|
| JWT + Cookie 登录 | ✅ | `POST /v1/admins/login` → HttpOnly `Set-Cookie`（SameSite=Lax） |
| HTTP Bearer 回退 | ✅ | `Authorization: Bearer` + Cookie + query `token`（三级提取） |
| WS 同源 Cookie | ✅ | 不再依赖 JS 读 token；跨源可显式 `?token=` |
| `/healthz` auth_mode | ✅ | `auth_mode` / `cookie_name` / `ws_path` / `deploy_env` |
| 登录错误分类 | ✅ | `loginErrors.ts` 区分网络 / 凭据 / 服务端错误 |
| 开发 bypass | ✅ | 登录页提示 +「进入系统（免登录）」 |
| Cookie HttpOnly | ✅ | `cookie.go` HttpOnly=true + SameSite=Lax + 可选 Secure |
| Admin CRUD | ✅ | List/Create/Update/Delete/Get（需 admin 权限） |
| 登录防枚举 | ✅ | `ErrInvalidCredentials` 统一错误 + `subtle.ConstantTimeCompare` |
| Token 测试覆盖 | ✅ | `token_test.go` 覆盖过期与坏签名路径 |
| gRPC 鉴权 | 🟡 | Bearer token 解析，无 token 仅 Info 日志不拒绝 |
| 密码哈希 | 🟡 | MD5（待升级 bcrypt，Phase 3） |
| PAT / API Key | ❌ | Phase 2 |
| RBAC | ❌ | 仅 JWT `access` 字段（二元：admin/非 admin） |
| SSO/OIDC | ❌ | Phase 3 |

---

## 4. 差距与优化

| 编号 | 差距 | 影响 | 优化方向 | 归属 Phase |
|------|------|------|----------|-----------|
| G-1 | gRPC 无 token 仅记日志不拒绝 | 内网外暴露则未认证可调用 | 生产强制 Bearer 或 mTLS | Phase 3 |
| G-2 | 密码 MD5 存储 | 可快速碰撞，弱安全 | 迁移 bcrypt，登录兼容一次升级 | Phase 3 |
| G-3 | Cookie 过期 7 天硬编码于 `service/admin.go` | 无法按部署调参 | 抽到配置项 | Phase 2+ |
| G-4 | Proto `phone` 字段未持久化 | 契约与 Schema 不一致 | 清理 Proto 字段或补 Schema | 待清理 |
| G-5 | Admin Schema 缺 `entsql.Annotation{Table}` | 违反 DB-N4，依赖默认复数化 | 补显式表名映射 | 待清理 |
| G-6 | PAT / RBAC 缺失 | 脚本集成与细粒度权限受限 | PAT 表 + Proto + Middleware；角色与权限点 | Phase 2/3 |
| G-7 | 跨域部署 WS 依赖 `?token=` | Cookie 无法跨域携带 | 统一域名或继续支持 query token | 持续 |

---

## 5. Phase 划分与任务清单

### Phase 0 — 文档与体验修补（已完成）

| # | 任务 | 状态 |
|---|------|------|
| 0.1 | 用户友好认证设计文档 | ✅ |
| 0.2 | README / 登录页：9001 与两种 dev 模式 | ✅ |
| 0.3 | dev 发送前 `checkBackendHealth`，避免误跳登录 | ✅ |
| 0.4 | WS bypass 与 HTTP 对齐 | ✅ |
| 0.5 | 登录失败区分网络 vs 凭据（解析 error envelope） | ✅ |

### Phase 1 — 浏览器会话加固（已完成）

| # | 任务 | 状态 |
|---|------|------|
| 1.1 | Cookie HttpOnly（WS 改纯 Cookie 转发，`readAccessTokenCookie` deprecated） | ✅ |
| 1.2 | `SameSite=Lax` / `Secure` 按环境配置（`KRATOS_AUTH_COOKIE_SECURE`） | ✅ |
| 1.3 | 统一 WS 鉴权：同源仅 Cookie，跨源回退 `?token=` | ✅ |
| 1.4 | `/healthz` 暴露 `auth_mode` | ✅ |
| 1.5 | 登录错误分类（`loginErrors.ts`） | ✅ |
| 1.6 | Token 三级提取（Cookie > Bearer > query） | ✅ |
| 1.7 | Admin CRUD（List/Create/Update/Delete/Get） | ✅ |
| 1.8 | 登录防枚举（统一错误 + 常量时间比较） | ✅ |

### Phase 2 — Personal Access Token（待实施）

| # | 任务 | 状态 |
|---|------|------|
| 2.1 | 表 `admin_access_tokens`（hash、name、expires、revoked_at） | ⏳ |
| 2.2 | Proto：`CreateToken` / `ListTokens` / `RevokeToken` | ⏳ |
| 2.3 | `auth.Middleware` 支持 `Bearer arn_pat_*` 与 Cookie JWT 并存 | ⏳ |
| 2.4 | CLI `aranea login` / `ARANEA_TOKEN` 文档 | ⏳ |

### Phase 3 — 企业能力（待实施）

| # | 任务 | 状态 |
|---|------|------|
| 3.1 | 密码哈希升级（bcrypt，登录兼容一次迁移） | ⏳ |
| 3.2 | gRPC 生产强制认证 | ⏳ |
| 3.3 | RBAC：角色与权限点 | ⏳ |
| 3.4 | OAuth2/OIDC SSO（回调仍落 Session Cookie） | ⏳ |

---

## 6. 验收标准

- [x] 设计文档已编写
- [x] Phase 0/1 实现（HttpOnly、healthz、登录 UX、WS Cookie）
- [x] Cookie HttpOnly + SameSite-Lax + 可选 Secure
- [x] Token 三级提取：Cookie > Bearer > query
- [x] 登录错误分类（网络 / 凭据 / 服务端）
- [x] `/healthz` 暴露 auth_mode 诊断信息
- [x] Admin CRUD 端点（List/Create/Update/Delete/Get）
- [x] 文档区分 JWT 签名密钥 / 会话 Cookie / Webhook secret
- [x] 登录防枚举（统一 `ErrInvalidCredentials` + 常量时间比较）
- [x] `make test` 覆盖 `ParseToken` 过期与坏签名路径（`token_test.go`）
- [ ] Phase 2 PAT 与 CLI `ARANEA_TOKEN`
- [ ] 密码哈希从 MD5 升级到 bcrypt
- [ ] RBAC 角色与权限点
- [ ] gRPC 生产强制认证

> 用户视角验收标准见 [需求文档 · 验收标准](./admin-auth.md#验收标准)。

---

## 7. 改动文件清单

模块涉及的代码文件（按层组织）：

**Proto**
- `api/kratos/admin/v1/admin.proto`

**认证库 `pkg/auth/`**
- `middleware.go`、`token.go`、`config.go`、`cookie.go`、`request_token.go`、`health.go`、`features.go`、`webhook.go`、`grpc_middleware.go`、`auth.go`
- 测试：`token_test.go`、`cookie_test.go`、`health_test.go`

**后端 service / biz / data**
- `internal/service/admin.go`
- `internal/biz/admin.go`
- `internal/data/admin.go`、`internal/data/ent/schema/admin.go`
- `internal/data/ent/admin/`（生成物，禁止手改）
- `internal/data/bootstrap_dev_admin.go`、`internal/data/bootstrap_initial_admin.go`

**传输层**
- `internal/server/http.go`（中间件与 webhook 路由注册）
- `internal/server/ws.go`（`wsAuthenticate`）

**前端 `web/src/`**
- `stores/auth.ts`、`pages/LoginPage.vue`
- `features/admin/loginErrors.ts`、`features/admin/api.ts`
- `config/runtime.ts`、`config/authHealth.ts`

---

## 8. 依赖与风险

- 跨域部署（`runtime-config.json` 设 `backendUrl`）需继续支持 WS `?token=` 或统一域名。
- PAT 需审计与吊销能力，仅存 hash。
- 密码算法升级需兼容旧 MD5 一次迁移（登录时检测并重写为 bcrypt）。
- gRPC 当前无 token 仅记日志，生产环境需内网隔离或强制认证。
- Admin Schema 缺 `entsql.Annotation{Table}`，补齐时需走 Ent 重新生成流程。
