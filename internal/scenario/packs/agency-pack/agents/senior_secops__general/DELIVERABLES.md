## 📋 你的技术交付物
### 快速失败 Secret 引导

```typescript
// TypeScript / Node.js — 启动时缺少 secret 则失败
function requireEnv(name: string): string {
  const value = process.env[name];
  if (!value) {
    console.error(`FATAL: Required environment variable "${name}" is not set.`);
    process.exit(1);
  }
  return value;
}

const config = {
  jwtSecret:    requireEnv("JWT_SECRET"),
  dbUrl:        requireEnv("DATABASE_URL"),
  idpJwksUri:   requireEnv("IDP_JWKS_URI"),
  allowedOrigins: requireEnv("ALLOWED_ORIGINS").split(","),
};
```

```python
# Python — 启动时缺少 secret 则失败
import os, sys

def require_env(name: str) -> str:
    value = os.environ.get(name)
    if not value:
        print(f"FATAL: Required environment variable '{name}' is not set.", file=sys.stderr)
        sys.exit(1)
    return value

config = {
    "jwt_secret":    require_env("JWT_SECRET"),
    "db_url":        require_env("DATABASE_URL"),
    "idp_jwks_uri":  require_env("IDP_JWKS_URI"),
}
```

### JWT 验证（Node.js — RS256 + JWKS）

```typescript
import jwksClient from "jwks-rsa";
import jwt from "jsonwebtoken";

const client = jwksClient({ jwksUri: config.idpJwksUri });

async function validateToken(token: string): Promise<jwt.JwtPayload> {
  const decoded = jwt.decode(token, { complete: true });
  if (!decoded || typeof decoded === "string") throw new Error("Invalid token format");

  const key = await client.getSigningKey(decoded.header.kid);
  const publicKey = key.getPublicKey();

  // 算法显式设置 — 绝不信任 token 自身的 alg 声明
  const payload = jwt.verify(token, publicKey, {
    algorithms: ["RS256"],        // 绝不使用 'none'，绝不来自 token header
    issuer: config.idpIssuer,
    audience: config.idpAudience,
  }) as jwt.JwtPayload;

  if (!payload.sub || !payload.exp || !payload.iat) {
    throw new Error("Missing required JWT claims");
  }

  return payload;
}
```

### 安全 Cookie 配置

```typescript
// Express — 生产就绪的 cookie 设置
const COOKIE_OPTIONS = {
  httpOnly: true,                            // 不可通过 JavaScript 访问
  secure: process.env.NODE_ENV === "production",  // 仅生产环境 HTTPS
  sameSite: "lax" as const,                 // CSRF 防护
  maxAge: 15 * 60 * 1000,                   // 15 分钟（access token）
  path: "/",
};

const REFRESH_COOKIE_OPTIONS = {
  ...COOKIE_OPTIONS,
  maxAge: 7 * 24 * 60 * 60 * 1000,          // 7 天（refresh token）
  path: "/api/auth/refresh",                  // 仅限刷新端点
};

// 设置 token — 生产环境绝不放在响应体中
res.cookie("access_token", accessToken, COOKIE_OPTIONS);
res.cookie("refresh_token", refreshToken, REFRESH_COOKIE_OPTIONS);
res.json({ message: "Authenticated" });     // 响应体中无 token
```

### HTTP 安全头（Nginx）

```nginx
server {
    # 强制 HTTPS（1年 + 子域名 + preload）
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains; preload" always;

    # 防止 MIME 嗅探
    add_header X-Content-Type-Options "nosniff" always;

    # 点击劫持防护
    add_header X-Frame-Options "DENY" always;

    # Referrer 策略
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    # 禁用不必要的浏览器功能
    add_header Permissions-Policy "camera=(), microphone=(), geolocation=(), payment=()" always;

    # CSP — 调整 script/style 来源以匹配你的 CDN
    add_header Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none';" always;

    # 认证路由不缓存
    location /api/auth/ {
        add_header Cache-Control "no-store" always;
    }

    # 移除服务器版本
    server_tokens off;
}
```

### CORS — 受限配置

```typescript
// Express + cors 包 — 显式允许列表
import cors from "cors";

const corsOptions: cors.CorsOptions = {
  origin: (origin, callback) => {
    // 允许无 origin 的请求（服务器间、curl、移动端）
    if (!origin) return callback(null, true);

    if (config.allowedOrigins.includes(origin)) {
      callback(null, true);
    } else {
      callback(new Error(`CORS: origin '${origin}' not allowed`));
    }
  },
  credentials: true,              // cookie 必需
  methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"],
  allowedHeaders: ["Content-Type", "Authorization"],
};

app.use(cors(corsOptions));
```

### 速率限制（Express）

```typescript
import rateLimit from "express-rate-limit";

// 认证路由 — 紧凑限制
export const authRateLimit = rateLimit({
  windowMs: 60 * 1000,             // 1 分钟
  max: 30,                          // 每 IP 30 个请求
  standardHeaders: true,            // X-RateLimit-* header
  legacyHeaders: false,
  message: { error: "Too many requests. Please try again later." },
  skipSuccessfulRequests: false,
});

// 密码重置 — 非常紧凑
export const passwordResetLimit = rateLimit({
  windowMs: 15 * 60 * 1000,        // 15 分钟
  max: 5,
  message: { error: "Too many password reset attempts." },
});

// 通用 API — 认证后按用户
export const apiRateLimit = rateLimit({
  windowMs: 60 * 1000,
  max: 100,
  keyGenerator: (req) => req.user?.id || req.ip,
});

// 应用
app.use("/api/auth/login",          authRateLimit);
app.use("/api/auth/register",       authRateLimit);
app.use("/api/auth/reset-password", passwordResetLimit);
app.use("/api/",                    apiRateLimit);
```

### 输入验证（Zod — TypeScript）

```typescript
import { z } from "zod";

// 严格 schema — 拒绝任何未显式允许的内容
const CreateUserSchema = z.object({
  username: z.string()
    .min(3).max(30)
    .regex(/^[a-zA-Z0-9_-]+$/, "Only alphanumeric, underscore, hyphen"),
  email: z.string().email().max(254),
  role: z.enum(["user", "moderator"]),   // 显式允许列表 — 绝不来自用户输入的 'admin'
});

// 中间件
export function validate<T>(schema: z.ZodSchema<T>) {
  return (req: Request, res: Response, next: NextFunction) => {
    const result = schema.safeParse(req.body);
    if (!result.success) {
      return res.status(400).json({
        error: "Validation failed",
        details: result.error.flatten().fieldErrors,
      });
    }
    req.body = result.data;  // 替换为已验证 + 类型化数据
    next();
  };
}

app.post("/api/users", validate(CreateUserSchema), createUserHandler);
```

### 安全日志模式

```typescript
// 应该记录什么
logger.info({
  event:    "user.login",
  userId:   user.id,              // 仅 ID，不是完整对象
  ip:       req.ip,
  userAgent: req.headers["user-agent"],
  timestamp: new Date().toISOString(),
  success:  true,
});

// 不应该记录什么 — 屏蔽敏感字段
function sanitizeForLog(obj: Record<string, unknown>) {
  const SENSITIVE = ["password", "token", "secret", "key", "authorization", "cookie", "cpf", "card"];
  return Object.fromEntries(
    Object.entries(obj).map(([k, v]) =>
      SENSITIVE.some(s => k.toLowerCase().includes(s)) ? [k, "[REDACTED]"] : [k, v]
    )
  );
}
```

---
