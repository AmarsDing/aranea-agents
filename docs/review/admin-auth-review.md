# Admin / Auth Review

> **评分**：75 / 100 | **风险等级**：P1  
> **文档**：[admin-auth.design.md](../需求/admin-auth.design.md) · [admin-auth-development.md](../需求/admin-auth-development.md)（注：缺少 `admin-auth.md` 需求文档）  
> **代码锚点**：`internal/service/admin.go`（或类似）· `web/src/pages/LoginPage.vue` · `web/src/stores/auth.ts` · `internal/server/` 中间件  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 15 | 20 | 登录/登出/鉴权路由守卫均已实现；RBAC/多用户/多工作区缺失 |
| 架构一致性 | 21 | 25 | Kratos 中间件（Auth/Workspace/Recovery/Tracing）位置正确；JWT 鉴权在传输层 ✅ |
| 后端实现质量 | 16 | 20 | 账号密码登录、JWT 生成、Workspace 中间件 ✅；多用户管理 API 缺失 |
| 前端实现质量 | 12 | 15 | 路由 `meta.requiresAuth` 守卫 ✅；`stores/auth.ts` 登录态管理 ✅；多用户 UI 缺失 |
| 测试与验证 | 5 | 10 | 鉴权中间件手动测试；无 auth 专项单测 |
| 文档一致性 | 6 | 10 | **缺少 `admin-auth.md` 需求文档**（仅有设计 + 开发计划两件套）；文档命名非标准编号格式 |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| 账号密码登录 | ✅ |
| JWT 生成 + 验证 | ✅ |
| 路由鉴权守卫（`meta.requiresAuth`）| ✅ |
| 登出 | ✅ |
| Workspace 中间件 | ✅ |
| Kratos Recovery 中间件 | ✅ |
| 多用户管理 | ❌ |
| RBAC 角色权限 | ❌ |
| 多工作区隔离 | 🟡 Workspace 中间件存在，多租户完整实现待定 |
| 后台健康检测（LoginPage）| ✅ |
| SSO / OAuth2 登录 | ❌ |

---

## 文档问题

| 问题 | 说明 |
|------|------|
| 缺少 `admin-auth.md` 需求文档 | 只有设计文档和开发计划，需补需求文档 |
| 文件命名无模块编号前缀 | `admin-auth.*` 不遵循 `NN <name>.*` 命名规范 |

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| AUTH-P1-01 | 缺少 `admin-auth.md` 需求文档 | 补充需求文档（用户故事、验收标准） |
| AUTH-P1-02 | 鉴权路径无专项单测（JWT 过期、无效 token、Workspace 隔离）| 补鉴权单测 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| AUTH-P2-01 | 多用户管理 API 缺失（当前只支持单 Admin 账号）| 规划多用户 + RBAC |
| AUTH-P2-02 | SSO/OAuth2 登录未规划 | 视需求优先级规划 |

---

## 建议优化路径

1. 补充 `admin-auth.md` 需求文档（P1）。
2. 补鉴权路径单测（P1）。
3. 规划多用户 + RBAC（P2）。
