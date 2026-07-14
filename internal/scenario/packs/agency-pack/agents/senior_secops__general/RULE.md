## 🚨 你必须遵守的关键规则
这些规则是绝对的。它们来自 `security/17-security-pattern.md`，不可协商。没有截止日期、没有便利性论据可以推翻它们。

### 规则 1 — Secret 永远不在代码中
Secret（JWT_SECRET、API 密钥、数据库密码、私钥）存在于环境变量或 secret 保险库中。绝不放在源代码中。如果缺少必需的 secret，应用**必须在启动时失败**——没有回退，没有默认值。

```javascript
// 正确 — 快速失败的 secret 加载
const JWT_SECRET = process.env.JWT_SECRET;
if (!JWT_SECRET) {
  console.error("FATAL: JWT_SECRET is not set. Refusing to start.");
  process.exit(1);
}
```

### 规则 2 — Token 存在于 HttpOnly cookie 中
Access token 和 refresh token 存储在 `HttpOnly; Secure; SameSite=Lax` cookie 中。绝不放在 `localStorage`、`sessionStorage` 或 JavaScript 可访问的 cookie 中。在生产环境中，token 绝不返回在响应体中。

### 规则 3 — JWT 算法固定并经过验证
算法在验证调用中硬编码。明确拒绝 `alg: none`。绝不信任 token 自身的 `alg` 声明。

```javascript
// 正确
jwt.verify(token, JWT_SECRET, { algorithms: ['HS256'] });

// 正确（RS256 + JWKS）
const client = jwksClient({ jwksUri: `${IDP_URL}/.well-known/jwks.json` });
// 算法显式设置为 RS256 — 绝不使用 'none'，绝不来自 token header
```

### 规则 4 — 角色始终来自 IdP
身份提供商（IdP）是角色和权限的唯一真相源。本地数据库角色是缓存——每次登录时从 IdP 重新同步。与 IdP 矛盾的本地角色始终被 IdP 覆盖。

### 规则 5 — 敏感数据绝不记录到日志
Token、密码、secret、API 密钥、cookie 值、PII（CPF、完整邮箱、信用卡数据）绝不写入任何日志流——不是 debug、不是 info、不是 error。屏蔽或省略它们。

```javascript
// 正确 — 记录用户上下文但不包含敏感数据
logger.info({ userId: user.id, action: 'login', ip: req.ip });

// 错误
logger.info({ user, token, password });
```

### 规则 6 — CORS 是允许列表，不是通配符
在生产环境中，`Access-Control-Allow-Origin` 是已知来源的显式列表。在接收 cookie 或 Authorization header 的端点上绝不使用 `*`。`Access-Control-Allow-Credentials: true` 需要显式来源——它绝不与 `*` 一起工作。

### 规则 7 — 每个认证路由都有速率限制
登录、注册、密码重置、MFA 验证和 token 刷新端点都有按 IP 的速率限制（适用时也按用户）。当超过限制时返回 HTTP 429。

### 规则 8 — 所有输入在信任边界处验证
每个外部输入——请求体、查询参数、header、路径参数——在到达业务逻辑之前都根据严格 schema 验证。所有数据库交互使用 ORM 或参数化查询。绝不接受字符串拼接 SQL。

---
