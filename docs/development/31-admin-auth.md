# Admin / Auth 需求

> **模块**：平台认证与授权
> **关联**：[admin-auth.design.md](./admin-auth.design.md) · [admin-auth.development.md](./admin-auth.development.md)

---

## 用户故事

1. 作为平台管理员，我希望使用 JWT 登录 Admin 控制台，以便安全访问运维与配置能力。
2. 作为开发者，我希望 HTTP/gRPC 请求携带 Bearer Token 时通过中间件校验，以便 API 不被未授权访问。
3. 作为开发者，我希望本地开发时可启用 bypass 模式免登录，以便快速迭代而不误用于生产。
4. 作为脚本/CLI 用户，我希望通过 Bearer Token 调用 API（PAT），以便自动化集成。
5. 作为多工作区部署者，我希望 Token claims 可携带 workspace 标识，以便数据隔离。

---

## 用户角色与期望

| 角色 | 期望 |
|------|------|
| 浏览器用户 | 登录一次即可用 Chat / 监控 / 配置；少碰环境变量；失败时有明确中文提示 |
| 本地开发者 | 一条命令启动；可选「免登录」但边界清楚，不误用于生产 |
| 脚本 / CLI | 不依赖 Cookie；可用长期 Token 或 `login` 后持久化凭据 |
| 外部 Webhook | 不走管理员账号；各 Channel 用自己的签名密钥 |
| 运维 / 安全 | 生产必须强密钥；dev bypass 不可在 `production` 生效；密钥不进仓库 |

---

## 功能需求

| 项 | 说明 |
|----|------|
| 登录 | `POST /v1/admins/login` 校验凭据并签发 JWT，Set-Cookie（HttpOnly, SameSite=Lax） |
| 登出 | `POST /v1/admins/logout` 清除 Cookie |
| 当前用户 | `GET /v1/admins/current` 校验 Cookie 有效性并返回用户信息 |
| Admin CRUD | 创建/查询/列表/更新/删除管理员（需 admin 权限） |
| HTTP 鉴权 | 中间件解析 Cookie / `Authorization: Bearer` / query `token` |
| gRPC 鉴权 | gRPC 中间件解析 Bearer Token（无 token 仅记录日志不拒绝，待收紧） |
| WS 鉴权 | 同源 Cookie 自动携带；跨源 `?token=` 回退 |
| 开发 bypass | `DEPLOY_ENV=dev` + `KRATOS_HTTP_AUTH_DISABLED=1` 跳过鉴权 |
| Webhook 隔离 | `/webhooks/*` 使用渠道签名密钥，不走 Admin JWT |
| 健康诊断 | `/healthz` 暴露 `auth_mode`/`cookie_name`/`deploy_env` |
| 登录错误分类 | 前端区分网络错误 / 凭据错误 / 服务端错误 |
| Token 过期 | 过期或签名错误返回 401 |
| 密钥配置 | `KRATOS_AUTH_SECRET`；生产环境未配置则启动 panic |
| PAT（访问令牌） | `Authorization: Bearer arn_pat_…` 长期凭证，可撤销 |
| RBAC | 角色与权限点，替代当前 `access: admin` 二元判断 |
| 密码哈希升级 | MD5 → bcrypt，登录时兼容一次迁移 |
| SSO/OIDC | 企业单点登录 |

> 各功能的实现进度与 Phase 划分见 [开发计划](./admin-auth.development.md)；技术设计、Proto 契约与数据模型见 [设计文档](./admin-auth.design.md)。

---

## 非功能需求

- **安全**：生产环境必须配置随机 ≥32 字节的 `KRATOS_AUTH_SECRET`；bypass 仅在 dev/development/test 或 CI 生效，`production` 必须为 false。
- **会话安全**：Cookie 必须 `HttpOnly; SameSite=Lax`；HTTPS 部署时通过 `KRATOS_AUTH_COOKIE_SECURE` 开启 `Secure`。
- **凭证隔离**：Webhook 签名密钥与 Admin JWT 签名密钥相互独立，文档与 UI 不得混称「秘钥」。
- **可诊断性**：`/healthz` 暴露 `auth_mode`，供前端与运维一键核对当前鉴权模式。
- **登录防枚举**：用户不存在与密码错误返回统一错误，避免泄露用户是否存在。

---

## 交互规格（用户视角）

### 登录流程

| 步骤 | 期望行为 |
|------|----------|
| 提交登录 | 按钮 loading；错误展示服务端 `message`（账号/密码错误 vs 网络错误） |
| 登录成功 | 写入会话状态；跳转 `?redirect=` 或默认页 |
| 冷启动 | 静默校验当前会话；失败则进登录页 |
| 401 拦截 | 带 `redirect` 回登录页；dev bypass 开启时登录页显示「进入系统（免登录）」 |
| 登录失败分类 | 区分网络 / 凭据 / 服务端错误，禁止只显示「登录失败」 |

### 会话保持与退出

| 事件 | 期望行为 |
|------|----------|
| JWT 未过期 | 正常使用 |
| JWT 过期 | API 401 → 登录页提示「登录已过期，请重新登录」 |
| 用户退出 | 清除 Cookie + 清除本地会话状态 |
| 多标签页 | 一端退出后，另一端下一次请求 401 后统一登出 |

### WebSocket 体验

- 浏览器优先使用同源 Cookie，无需手动配置 token 即可收流。
- 禁止要求用户「打开 DevTools 复制 token 才能聊天」。
- WS 401 时前端提示「会话已过期，请重新登录」，而非「WebSocket 失败」。

### 错误文案

| 状态 | 场景 | 建议文案 | 前端动作 |
|------|------|----------|----------|
| 401 | 无 Cookie / Token 无效 | 未登录或会话已失效 | 跳转登录 + redirect |
| 403 | 非 admin 调管理 API | 没有权限执行此操作 | Notify + 停留 |
| 502/网络 | 后端未启动 | 无法连接服务，请确认 admin 是否在运行 | dev：不踢登录；提示检查端口 |

---

## 验收标准

- 有效 Token 可访问受保护 Admin API
- 过期 / 篡改 Token 返回 401
- Cookie 为 HttpOnly + SameSite=Lax
- WS 同源时自动携带 Cookie，无需 JS 读取 token
- bypass 模式仅 dev/test 环境生效，production 禁止
- `/healthz` 暴露 auth_mode 供前端诊断
- 登录错误区分网络 vs 凭据 vs 服务端
- Webhook 与 Admin 凭证隔离，文档无混称
- 新同事仅读 README + 本文档，可在 10 分钟内完成登录并开始 Chat
- 浏览器登录后，Chat WS 无需手动配置 token 即可收流
- 生产未配置 `KRATOS_AUTH_SECRET` 时进程拒绝启动；bypass 在 production 无效
- 401/后端不可用时，文案能区分「重新登录」与「检查后端是否启动」
- PAT CRUD + Bearer 并存（演进目标）
- 密码哈希从 MD5 升级到 bcrypt（演进目标）
- RBAC 角色与权限点（演进目标）

> 实现进度与任务状态见 [开发计划 · 验收标准](./admin-auth.development.md)。
