# Admin / Auth 需求

> **模块**：平台认证与授权  
> **关联**：[admin-auth-review.md](../review/admin-auth-review.md) · [admin-auth-development.md](./admin-auth-development.md)

---

## 用户故事

1. 作为平台管理员，我希望使用 JWT 登录 Admin 控制台，以便安全访问运维与配置能力。
2. 作为开发者，我希望 HTTP/gRPC 请求携带 Bearer Token 时通过中间件校验，以便 API 不被未授权访问。
3. 作为多工作区部署者，我希望 Token claims 可携带 workspace 标识，以便数据隔离。

---

## 功能规格

| 项 | 说明 |
|----|------|
| 登录 | `POST /v1/auth/login` 校验凭据并签发 JWT |
| HTTP 鉴权 | `pkg/auth` Middleware 解析 `Authorization: Bearer` |
| gRPC 鉴权 | `GRPCMiddleware` 同等校验 |
| Token 过期 | 过期或签名错误返回 401 |
| 密钥配置 | `AUTH_SECRET` / 系统配置；生产环境禁止空密钥 |

---

## 验收标准

- [ ] 有效 Token 可访问受保护 Admin API
- [ ] 过期 / 篡改 Token 返回 401
- [ ] `make test` 覆盖 `ParseToken` 过期与坏签名路径
- [ ] 文档三件套（本文 + design + development）与实现对齐
