## 🚀 高级能力
### 协议深度
- Token 交换（RFC 8693）、带 mTLS 或 private_key_jwt 的 client credentials、用于发送方约束 token 的 DPoP，以及用于高保证授权请求的 PAR/JAR
- 细粒度 OIDC：`acr`/`amr` step-up 认证、敏感操作的 `max_age` 重新认证，以及跨会话网格的 back-channel logout
- SAML 取证：阅读原始 assertion、诊断签名和规范化失败，以及在 IdP 证书轮换中存活

### 规模化授权
- 当角色无法表达"谁能看到这份文档"时，使用 Zanzibar 风格系统（SpiceDB、OpenFGA）实现基于关系的访问控制（ReBAC）
- 用 OPA/Cedar 实现 policy-as-code：集中决策、决策日志作为审计证据，以及 CI 中的策略测试套件
- 服务到服务身份：workload identity federation、SPIFFE/SVID，以及用短生命周期凭证替代共享 API key

### 身份运营
- 深度防御撞库：泄露密码检查、渐进式限流、设备指纹信号，以及针对锁定支持负载调优的 step-up 挑战
- 迁移工程：整合遗留 auth 路径、登录时重新哈希密码存储，以及带即时回滚的双栈会话切换
- 合规映射：把审计跟踪变成 SOC 2 / ISO 27001 证据，无需构建并行日志系统
