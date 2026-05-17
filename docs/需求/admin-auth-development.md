# Admin & Auth 管理 & 认证 — 开发计划

> **版本**：2026-05-17 | **状态**：🟡 JWT 认证可用；❌ RBAC/多租户未实现
> **需求**：管理 & 认证 · **设计**：管理 & 认证设计
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Admin & Auth 管理 & 认证：用户认证、权限管理、多租户隔离和系统管理。

**代码锚点**：
- `internal/middleware/` — JWT 认证中间件
- `internal/server/http.go` — 中间件注册
- `internal/biz/user.go` — UserUsecase

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| JWT 认证 | ✅ | 中间件验证 |
| 用户管理 | ✅ | User CRUD |
| RBAC 权限 | ❌ | 无角色/权限模型 |
| 多租户 | ❌ | 无 workspace_id 隔离 |
| SSO 集成 | ❌ | 无 OAuth2/OIDC |
| 审计日志 | ❌ | 无操作审计 |

---

## 3. 差距与优化

1. **P1**：无 RBAC 权限管理，所有用户权限相同。
2. **P1**：无多租户隔离，数据无 workspace_id 隔离。
3. **P2**：无 SSO 集成，无法对接企业身份系统。
4. **P3**：无审计日志，无法追溯操作历史。

---

## 4. 开发阶段

- **Phase 1**：RBAC 权限模型（角色/权限/分配）
- **Phase 2**：多租户隔离（workspace_id 注入）
- **Phase 3**：SSO 集成 + 审计日志

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | RBAC：role / permission / user_role 表 + API | P1 | — |
| 2 | 多租户：workspace 表 + workspace_id 注入 | P1 | — |
| 3 | SSO：OAuth2/OIDC 集成 | P2 | — |
| 4 | 审计日志：audit_log 表 + 查询 API | P3 | — |

---

## 6. 验收标准

- [ ] 不同角色有不同权限
- [ ] 数据按 workspace_id 隔离
- [ ] 可通过 SSO 登录

---

## 7. 依赖与风险

- RBAC 需修改所有 API 增加权限检查
- 多租户需修改所有数据表增加 workspace_id
