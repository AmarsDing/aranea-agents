# Artifact 产出物 — 开发计划

> **版本**：2026-05-17 | **状态**：🟡 基础存储可用；❌ 版本管理/预览未实现
> **需求**：[27 artifact.md](./27%20artifact.md) · **设计**：[27 artifact.design.md](./27%20artifact.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Artifact 产出物：管理 Agent 运行时产生的文件、图片、代码等产出物，支持存储、版本管理、预览和下载。

**代码锚点**：
- `api/kratos/artifact/v1/` — Artifact CRUD RPC
- `internal/service/artifact.go` — ArtifactService
- `internal/biz/artifact.go` — ArtifactUsecase
- `internal/data/artifact.go` — ArtifactRepo
- `internal/agent/codeexecutor/executor.go` — Docker Sandbox 产出物收集

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Artifact CRUD | ✅ | Create/Update/Delete/Get/List |
| 文件存储 | ✅ | 本地文件系统存储 |
| 产出物收集 | ✅ | CodeExecutor 自动收集 |
| 版本管理 | ❌ | 无版本历史 |
| 在线预览 | ❌ | 无预览功能 |
| 下载链接 | ❌ | 无签名下载 URL |

---

## 3. 差距与优化

1. **P2**：Artifact 无版本管理，更新后无法回滚。
2. **P2**：Artifact 无在线预览（图片/PDF/代码），用户需下载后查看。
3. **P3**：Artifact 无签名下载 URL，安全性不足。

---

## 4. 开发阶段

- **Phase 1**：Artifact 版本管理
- **Phase 2**：在线预览（图片/PDF/代码高亮）
- **Phase 3**：签名下载 URL

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | Artifact 版本表 + 版本历史查询 API | P2 | — |
| 2 | 在线预览：图片/PDF/代码高亮 | P2 | — |
| 3 | 签名下载 URL（HMAC-SHA256） | P3 | — |

---

## 6. 验收标准

- [ ] Artifact 可管理多个版本并回滚
- [ ] 图片/PDF/代码可在浏览器中预览
- [ ] 下载链接有时效性签名

---

## 7. 依赖与风险

- 预览功能需考虑 XSS 防护
- 大文件预览需流式处理
