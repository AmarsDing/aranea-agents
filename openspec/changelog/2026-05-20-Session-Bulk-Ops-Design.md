# Session 历史 — 批量治理设计

> 日期：2026-05-20  
> 关联：SES-1b · [10 session.md](../需求/10%20session.md) · [10 session.design.md](../需求/10%20session.design.md)

## 摘要

会话历史列表增加：**行内永久删除**、**批量选择**（勾选归档/删除）、**按保留天数**批量归档与批量删除；勾选归档无二次确认并显示进度条，删除类操作需确认弹窗。

## 需求映射

| # | 需求 | 设计要点 |
|---|------|----------|
| 1 | 行内删除 + 确认 | `DeleteSession` + `SessionDeleteConfirmDialog` |
| 2 | 按天数删除（保留 N 天） | `BatchDeleteSessions` + `older_than_days` + preview |
| 3 | 按天数归档 + 确认弹窗 | `BatchArchiveSessions` + `SessionRetentionDialog` |
| 4 | 批量选择 | checkbox 列 + 勾选归档（无弹窗/有进度）+ 勾选删除（有弹窗） |

## 删除语义

- **用户**：永久不可恢复（列表/搜索不可见）
- **后端**：软删除（`deleted_at`），usage/audit 保留 — 与 [10 session.md §12 原则 6](../需求/10%20session.md) 一致

## 文档更新

- `10 session.md` §6.7、§7.2.1–7.2.3
- `10 session.design.md` §2.3、§8.5
- `10-session-development.md` Phase 1b
- `frontend-pages.md` 会话列表

## 实现顺序建议

1. 后端 Batch RPC + cutoff 单测  
2. 前端行删除（可独立上线）  
3. 批量选择 + 进度条（可先 fallback 逐条 RPC）  
4. 按天数 Dialog + preview  
