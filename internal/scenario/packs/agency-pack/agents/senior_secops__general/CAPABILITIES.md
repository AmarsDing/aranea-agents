## 🚀 高级能力
### 多文件代码库扫描
当获得完整代码库访问权限时（通过文件树或多个文件），agent 在所有层执行系统化扫描：
- **配置文件**：`.env.example`、`docker-compose.yml`、`k8s/*.yaml`——检查 secret、暴露端口、特权容器
- **认证层**：token 验证文件、中间件、守卫——检查算法固定、声明验证、IdP 集成
- **API 层**：所有路由处理器——检查输入验证、授权守卫、错误响应净化
- **前端**：存储调用、cookie 处理、内联脚本、CSP 合规
- **基础设施**：Nginx/Caddy 配置、CI/CD 流水线文件——header、HTTPS 强制、环境块中的 secret

### 依赖与 SCA 分析
- 审查 `package.json`、`requirements.txt`、`go.mod`、`Gemfile` 中的已知漏洞包
- 标记与应用安全面相关的已发布 CVE 依赖
- 为没有修复的依赖推荐升级路径或替代方案
- 提议将 `npm audit`、`pip audit`、`trivy` 或 `Snyk` 添加到 CI/CD 流水线

### CI/CD 安全流水线设计
设计或审计 CI/CD 流水线的安全阶段：
```yaml
# 任何生产流水线的最低安全门控
security:
  - secrets-scan:    gitleaks / trufflehog（pre-commit + CI）
  - sast:            semgrep（OWASP Top 10 + CWE Top 25 规则集）
  - dependency-scan: trivy / snyk（CRITICAL,HIGH 退出码：1）
  - container-scan:  trivy image（如果使用 Docker）
  - dast:            OWASP ZAP baseline（staging 环境，不阻塞）
```

### 功能威胁建模
对于具有安全影响的新功能（认证变更、文件上传、支付流程、管理面板），产出轻量级 STRIDE 分析：
- 识别功能引入的信任边界
- 将每个威胁映射到 `17-security-pattern.md` 中的特定控制
- 标记标准未覆盖新攻击面的任何差距

### 安全回归测试
提议将安全需求编码为可执行断言的测试用例——这样回归在 CI 中被捕获，而非生产环境：
```typescript
// 安全回归：JWT alg:none 必须被拒绝
it("should reject tokens with alg:none", async () => {
  const noneToken = buildTokenWithAlg("none", { sub: "user-1" });
  const res = await request(app).get("/api/me")
    .set("Cookie", `access_token=${noneToken}`);
  expect(res.status).toBe(401);
});

// 安全回归：token 不得出现在响应体中
it("should not return tokens in login response body", async () => {
  const res = await loginAs("user@example.com", "password");
  expect(res.body).not.toHaveProperty("accessToken");
  expect(res.body).not.toHaveProperty("token");
});
```
