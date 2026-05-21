# 20 Skill Review

> **评分**：78 / 100 | **风险等级**：P1  
> **文档**：[20-skill-development.md](../需求/20-skill-development.md)  
> **代码锚点**：`internal/skill/` · `internal/service/skill.go` · `internal/service/skill_import.go` · `internal/server/skill_import_http.go` · `web/src/pages/SkillsPage.vue`  
> **审查时间**：2026-05-21

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 16 | 20 | 管理面 + Layer A/B 运行时已通；版本回滚/RBAC/Preview 原因缺失 |
| 架构一致性 | 21 | 25 | `internal/skill/trpc` 正确集成框架；**P1 问题**：`server/skill_import_http.go` 绕过 proto/service 层 |
| 后端实现质量 | 17 | 20 | ZIP 导入 + 任务轮询 + 统计条 + 运行时挂载 ✅；版本管理缺失 |
| 前端实现质量 | 12 | 15 | Skill 列表 + 启用 + 统计条 ✅；ZIP 导入进度轮询 ✅；版本回滚 UI 缺失 |
| 测试与验证 | 6 | 10 | 基础导入流程测试；版本/RBAC 路径无测试 |
| 文档一致性 | 6 | 10 | `20-skill-development.md` 与现状对齐；`20 skill struct design.md` 作为结构设计补充 |

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| Skill CRUD | ✅ |
| ZIP 导入（Layer A/B）| ✅ |
| 导入任务轮询 | ✅ |
| Skill 运行时工具挂载 | ✅ |
| `skill_call_count` session 计数 | ✅ |
| Skill 统计条 | ✅ |
| `/skills/runs` 调用记录 | ✅ |
| CodeExecutor 集成 | ✅ |
| 版本回滚 | ❌ |
| RBAC 权限控制 | ❌ |
| Preview 原因（为何触发）| ❌ |

---

## 架构问题 — P1

### `server/skill_import_http.go` 旁路

**问题**：Skill ZIP 导入使用了注册在 `registerCustomRoutes`（`internal/server/http.go`）中的自定义 HTTP 路由（实现在 `internal/service/skill_import_http.go`），绕过了 proto service 层的鉴权、观测、API 契约机制。

**影响**：
- 鉴权不一致（可能无 JWT/Workspace 中间件）
- 无 FlowLog / Prometheus 指标
- API 契约不在 proto 中管理

**修复方案**：
1. 在 `skill.proto` 中添加 `ImportSkillZip` RPC（multipart 或 streaming）
2. 将实现迁入 `SkillService.ImportSkillZip`
3. 删除 `server/skill_import_http.go`

---

## 主要风险

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| SKILL-P1-01 | `server/skill_import_http.go` 旁路 proto/service 层（见上方分析） | 迁入 `SkillService` + `skill.proto` |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| SKILL-P2-01 | 版本回滚未实现 | 规划 Skill 版本表 + 回滚 API |
| SKILL-P2-02 | RBAC 权限控制（谁可以导入/启用 Skill）未实现 | 规划 Skill RBAC |
| SKILL-P2-03 | Preview 原因（AgentKey/情境触发条件）未暴露给用户 | 规划 Preview 详情展示 |

---

## 建议优化路径

1. **设计例外**：`skill_import_http.go` 经 Service 鉴权；迁入 `SkillService` 为 P3 可选（见 [README-development §技术预览与 P3](../需求/README-development.md)）。
2. 规划 Skill 版本管理（P2）。
3. 规划 Skill RBAC（P3）。
