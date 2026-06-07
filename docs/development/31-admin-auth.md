# Admin / Auth 需求

> **模块**：平台认证与授权
> **关联**：[admin-auth.design.md](./admin-auth.design.md) · [admin-auth.development.md](./admin-auth.development.md)

---

## 用户故事

1. 作为平台管理员，我希望使用 JWT 登录 Admin 控制台，以便安全访问运维与配置能力。
2. 作为开发者，我希望 HTTP/gRPC 请求携带 Bearer Token 时通过中间件校验，以便 API 不被未授权访问。
3. 作为开发者，我希望本地开发时可启用 bypass 模式免登录，以便快速迭代而不误用于生产。
4. 作为脚本/CLI 用户，我希望通过 Bearer Token 调用 API（Phase 2 PAT），以便自动化集成。
5. 作为多工作区部署者，我希望 Token claims 可携带 workspace 标识，以便数据隔离（Phase 3）。

---

## 功能规格

| 项 | 说明 | 状态 |
|----|------|------|
| 登录 | `POST /v1/admins/login` 校验凭据并签发 JWT，Set-Cookie（HttpOnly, SameSite=Lax） | ✅ |
| 登出 | `POST /v1/admins/logout` 清除 Cookie | ✅ |
| 当前用户 | `GET /v1/admins/current` 校验 Cookie 有效性并返回用户信息 | ✅ |
| HTTP 鉴权 | `pkg/auth` Middleware 解析 Cookie / `Authorization: Bearer` / query `token` | ✅ |
| gRPC 鉴权 | `GRPCMiddleware` 解析 Bearer Token（无 token 仅 warning 不拒绝，待收紧） | ✅ |
| WS 鉴权 | 同源 Cookie 自动携带；跨源 `?token=` 回退 | ✅ |
| 开发 bypass | `DEPLOY_ENV=dev` + `KRATOS_HTTP_AUTH_DISABLED=1` 跳过鉴权 | ✅ |
| Webhook 隔离 | `/webhooks/*` 使用渠道签名密钥，不走 Admin JWT | ✅ |
| 健康诊断 | `/healthz` 暴露 `auth_mode`/`cookie_name`/`deploy_env` | ✅ |
| 登录错误分类 | 前端区分网络错误 / 凭据错误 / 服务端错误 | ✅ |
| Token 过期 | 过期或签名错误返回 401 | ✅ |
| 密钥配置 | `KRATOS_AUTH_SECRET`；生产环境未配置则 panic | ✅ |
| Admin CRUD | 创建/更新/删除/查询管理员（需 admin 权限） | ✅ |
| PAT（访问令牌） | `Authorization: Bearer arn_pat_…` 长期凭证，可撤销 | ⏳ Phase 2 |
| RBAC | 角色与权限点，替代当前 `access: admin` 二元判断 | ⏳ Phase 3 |
| 密码哈希升级 | MD5 → bcrypt，登录时兼容一次迁移 | ⏳ Phase 3 |
| SSO/OIDC | 企业单点登录 | ⏳ Phase 3 |

---

## 验收标准

- [x] 有效 Token 可访问受保护 Admin API
- [x] 过期 / 篡改 Token 返回 401
- [x] Cookie 为 HttpOnly + SameSite=Lax
- [x] WS 同源时自动携带 Cookie，无需 JS 读取 token
- [x] bypass 模式仅 dev/test 环境生效，production 禁止
- [x] `/healthz` 暴露 auth_mode 供前端诊断
- [x] 登录错误区分网络 vs 凭据 vs 服务端
- [x] Webhook 与 Admin 凭证隔离
- [ ] PAT CRUD + Bearer 并存（Phase 2）
- [ ] 密码哈希从 MD5 升级到 bcrypt（Phase 3）
- [ ] RBAC 角色与权限点（Phase 3）
- [ ] `make test` 覆盖 `ParseToken` 过期与坏签名路径
- [x] 文档三件套（本文 + design + development）与实现对齐
