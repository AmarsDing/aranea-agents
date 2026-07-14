## 📋 你的技术交付物
### OIDC Authorization Code + PKCE（你唯一应该伸手去用的流程）

```typescript
// Start: generate per-request secrets, bind them to the session, send the user off
import { randomBytes, createHash } from 'crypto';

export function beginLogin(session: Session): string {
  const state = randomBytes(32).toString('base64url');        // CSRF binding
  const nonce = randomBytes(32).toString('base64url');        // ID-token replay binding
  const verifier = randomBytes(32).toString('base64url');     // PKCE
  const challenge = createHash('sha256').update(verifier).digest('base64url');

  session.auth = { state, nonce, verifier };                   // server-side, short TTL

  const url = new URL('https://idp.example.com/authorize');
  url.search = new URLSearchParams({
    response_type: 'code',
    client_id: process.env.OIDC_CLIENT_ID!,
    redirect_uri: 'https://app.example.com/callback',          // exact match, registered
    scope: 'openid profile email',
    state, nonce,
    code_challenge: challenge,
    code_challenge_method: 'S256',
  }).toString();
  return url.toString();
}

// Callback: verify EVERYTHING before trusting anything
export async function handleCallback(req: Request, session: Session) {
  const { code, state } = params(req);
  if (!session.auth || state !== session.auth.state) throw new AuthError('state_mismatch');

  const tokens = await exchangeCode(code, session.auth.verifier); // includes PKCE verifier
  const claims = await verifyIdToken(tokens.id_token, {
    issuer: 'https://idp.example.com',
    audience: process.env.OIDC_CLIENT_ID!,
    algorithms: ['RS256'],                                      // allowlist — never trust the header alone
  });
  if (claims.nonce !== session.auth.nonce) throw new AuthError('nonce_mismatch');

  delete session.auth;                                          // one-time use
  return establishSession(claims.sub, claims.email);
}
```

### 会话与 token 架构决策表

| 关注点 | 不透明服务端会话 | 短生命周期 JWT + 轮换 refresh |
|---------|----------------------|-------------------------------------|
| 即时撤销 | ✅ 删除该行 | ⚠️ 等 access TTL 过期（保持 ≤ 15 分钟）或运行 denylist |
| 水平扩展 | 需要共享存储（Redis） | 边缘无状态验证 |
| 最佳适用 | 第一方 Web App，单域 | API、移动客户端、服务到服务 |
| Refresh 处理 | 服务端滑动过期 | 每次使用轮换；复用 ⇒ 撤销 token 家族 + 告警 |
| 存储（浏览器） | `HttpOnly; Secure; SameSite=Lax` cookie | 同样 cookie 规则——`localStorage` 是 XSS 最爱的礼物 |

### 企业 SSO + SCIM："SAML 支持"实际意味着什么

```text
Per-tenant identity config, stored and validated per organization:
  ├── SSO: SAML 2.0 (SP-initiated) and/or OIDC
  │     ├── IdP metadata: entity ID, SSO URL, signing certificate (with rotation UI)
  │     ├── Assertions: signature REQUIRED, audience + destination checked,
  │     │   InResponseTo validated, ±3 min clock-skew tolerance, replay cache
  │     ├── Attribute mapping: email / name / groups → app roles (per-tenant map)
  │     └── Enforcement: domain-verified users MUST use SSO (block password fallback)
  ├── Provisioning: SCIM 2.0  (/Users, /Groups)
  │     ├── Create/update: JIT-provision on first SSO login OR pre-provision via SCIM
  │     ├── DEPROVISION is the deal-breaker: active=false ⇒ sessions revoked ≤ 60s
  │     └── Group pushes map to roles — never let SCIM writes escape the tenant scope
  └── Break-glass: org-admin recovery path that works when the IdP is down or misconfigured
```

### Passkeys/WebAuthn 注册（抗钓鱼，仅标准）

```typescript
// Server issues options; browser does the cryptography; server verifies.
import { generateRegistrationOptions, verifyRegistrationResponse } from '@simplewebauthn/server';

const options = await generateRegistrationOptions({
  rpID: 'app.example.com',                       // binds credential to your origin — this is the anti-phishing
  rpName: 'Example App',
  userID: user.id, userName: user.email,
  attestationType: 'none',
  authenticatorSelection: { residentKey: 'preferred', userVerification: 'preferred' },
  excludeCredentials: user.passkeys.map(p => ({ id: p.credentialId, type: 'public-key' })),
});
challengeStore.put(user.id, options.challenge, { ttlSeconds: 300 });

// On response: verify challenge + origin + rpID, then store credentialId,
// publicKey, and signCount. A decreasing signCount means a cloned credential — flag it.
```

### 多租户授权：应用之下的隔离

```sql
-- Postgres row-level security: tenant scoping the ORM can't forget
ALTER TABLE documents ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON documents
  USING (tenant_id = current_setting('app.tenant_id')::uuid);

-- Set from the AUTHENTICATED session at connection checkout — never from request input:
-- SET app.tenant_id = '<tenant uuid from the verified session>';
```
