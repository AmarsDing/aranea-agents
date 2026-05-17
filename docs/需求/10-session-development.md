# Session — 开发计划

> **版本**：2026-05-17 | **状态**：✅ 端到端可用
> **需求**：[10 session.md](./10%20session.md) · **设计**：[10 session.design.md](./10%20session.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Session 管理：管理用户与 Agent/Team 的对话会话，包括会话创建、列表、删除、标题更新、上下文压缩等。

**代码锚点**：
- `api/kratos/session/v1/` — Session CRUD RPC
- `internal/service/session.go` — SessionService
- `internal/biz/session.go` — SessionUsecase
- `internal/data/session.go` — SessionRepo
- `internal/service/session_compress.go` — 上下文压缩

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| Session CRUD | ✅ | Create/Update/Delete/Get/List |
| 消息列表 | ✅ | `ListMessages` RPC |
| 上下文压缩 | ✅ | `SessionCompressor.AfterNativeTurn` |
| 标题生成 | ✅ | `LLMSessionTitleGenerator` |
| Session 类型 | ✅ | agent / team |

---

## 3. 差距与优化

1. **P2**：Session 无"置顶"功能，用户无法固定重要会话。
2. **P3**：Session 无"导出"功能，用户无法导出对话记录。
3. **P3**：Session 消息无"搜索"功能，用户无法在历史消息中搜索关键词。

---

## 4. 开发阶段

- **Phase 1**：Session 置顶功能（pinned_at 字段 + 排序）
- **Phase 2**：Session 导出功能（Markdown / JSON 格式）
- **Phase 3**：消息搜索功能（全文检索）

---

## 5. 任务清单

| # | 任务 | 优先级 | EP |
|---|------|--------|-----|
| 1 | Session 表增加 `pinned_at` 字段 + PinSession RPC | P2 | — |
| 2 | ExportSession RPC（Markdown/JSON） | P3 | — |
| 3 | SearchMessages RPC（全文检索） | P3 | — |

---

## 6. 验收标准

- [ ] 用户可置顶/取消置顶会话
- [ ] 可导出会话为 Markdown/JSON
- [ ] 可在历史消息中搜索关键词

---

## 7. 依赖与风险

- 全文检索需评估 SQLite FTS5 或引入搜索引擎
