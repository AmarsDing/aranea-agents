# Avatar 头像 — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[50 avatar.md](./50%20avatar.md) · **设计**：[50 avatar.design.md](./50%20avatar.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Avatar 头像：管理 Agent/Team 的头像，支持上传、裁剪、存储和展示。

**代码锚点**：
- `api/kratos/agent/v1/` — Agent avatar 字段
- `api/kratos/team/v1/` — Team avatar 字段
- `internal/service/agent.go` — Avatar 上传

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Avatar 字段 | ✅ | `avatar_url` 字段 |
| Avatar 上传 | ✅ | 文件上传 API |
| Avatar 展示 | ✅ | 前端展示 |

---

## 3. 差距与优化

1. **P3**：Avatar 无裁剪功能，用户需预先裁剪图片。
2. **P3**：Avatar 无 CDN 加速，大图片加载慢。

---

## 4. 开发阶段

- **Phase 1**：Avatar 裁剪功能（前端裁剪组件）
- **Phase 2**：CDN 加速

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | 前端 Avatar 裁剪组件 | P3 | — |
| 2 | CDN 加速配置 | P3 | — |

---

## 6. 验收标准

- [ ] 用户可在上传时裁剪 Avatar
- [ ] Avatar 通过 CDN 加速加载

---

## 7. 依赖与风险

无重大依赖。
