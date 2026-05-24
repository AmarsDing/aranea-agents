# Session Phase 2 — Code Review

> **依据**：[`docs/README.md`](../README.md) · [`docs/review/README.md`](./README.md) · [10-session-development.md §8](../需求/10-session-development.md#8-待优化清单全部)  
> **审查时间**：2026-05-24 · **范围**：Pin / Export / Timeline UNION / ListSessionRuns / ListSessionParticipants / 前端 Tab & 侧栏 / biz 拆分  
> **基线**：[10-session-review.md](./10-session-review.md)（2026-05-21，79/100）

---

## 综合评级

| 指标 | 结果 |
|------|------|
| **总分** | **83 / 100**（+4） |
| **风险等级** | **P1**（Participants 读时全量 Sync、Export/Chat Timeline 无界加载） |
| **迭代结论** | **可合并继续迭代**；P1 项应在 Phase 2 收尾前修复 |

### 六维得分

| 维度 | 权重 | 得分 | 说明 |
|------|------|------|------|
| 需求符合度 | 20 | 17 | F1/F2/F4 闭环；F6 仅读时聚合；F5/F7+ 仍缺 |
| 架构一致性 | 25 | 22 | biz 拆分、Timeline ref→hydrate 模式好；Runs 直调 repo |
| 后端实现质量 | 20 | 16 | SQL UNION 正确；Participants/Export 有性能隐患 |
| 前端实现质量 | 15 | 13 | 详情页分页/Tab 规范；Chat Timeline 仍全量拉取 |
| 测试与验证 | 10 | 5 | 新 SQL/Export/Participants 几乎无集成测 |
| 文档一致性 | 10 | 9 | development/design/session.md 已同步 §8 |

### 专项六维（代码 Review 关注点）

| 维度 | 评级 | 摘要 |
|------|------|------|
| 代码质量 | B+ | `internal/biz/session/*` 拆分清晰；`limits.go` dead code；stub 维护成本升 |
| 业务逻辑 | B | Pin/Export/Timeline 主路径正确；Participants 非增量、MCP 启发式分类 |
| 架构与设计模式 | A− | Ref 分页 + hydrate；Observability 分文件；Runs 缺 biz 门面 |
| 可读性与风格 | A− | 命名与分层符合项目惯例；raw SQL 缺 MCP 判定说明 |
| 错误处理与健壮性 | B− | 校验较统一；Export/Participants 缺 size 上限与并发保护 |
| 影响范围与回归风险 | B | Timeline 行为变更面大；Chat 对话框从 2000 cap 变为全量 UNION |

---

## 1. 改动概览

| 能力 | 后端锚点 | 前端 | 状态 |
|------|----------|------|------|
| 置顶 `pinned_at` | `pin.go` · `session_repo.PinSession` | `sessionStore` · 管理页 | ✅ |
| 导出 MD/JSON | `export.go` · `session_observability.go` | `downloadExport.ts` · 详情页 | ✅ |
| Timeline UNION 分页 | `session_timeline.go` · `timeline_hydrate.go` | `useSessionTimelinePanel` 100/页 | ✅ |
| ListSessionRuns | `session_run_repo.ListBySession` | `SessionRunsPanel` | ✅ |
| ListSessionParticipants | `session_participant_repo` 读时 Sync | `SessionParticipantsPanel` | 🟡 |
| Chat Timeline 对话框 | 同 API，`limit=0`→全量 | `useSessionTimelineInspector` | 🟡 回归风险 |

**代码锚点**：

- `internal/biz/session/` — timeline、export、pin、participants_list
- `internal/data/session_timeline.go` — SQL UNION ALL + COUNT + 分页
- `internal/service/session_observability.go` — Export / Runs / Participants
- `web/src/features/session/` — API、composable、详情 Tab

---

## 2. 亮点

### 2.1 Timeline：Ref 分页 + Hydrate

1. `ListTimelineEventRefsPaged` — SQL `UNION ALL` + `COUNT` + `LIMIT/OFFSET`
2. `hydrateTimelineRefs` — 按 kind 批量 `List*ByIDs`，保持 ref 顺序

移除 in-memory merge 与 2000 message cap，分页在 DB 完成，符合 design「明细独立表 + session_id 关联」。

### 2.2 biz 包拆分

`timeline.go` / `timeline_hydrate.go` / `timeline_items.go` / `export.go` / `pin.go` / `participants_list.go` 从巨型 usecase 拆出，便于 F5/F7 续扩展。

### 2.3 Pin 服务端化

Ent `pinned_at` + RPC + Audit；前端按 `pinned_at` 排序；Search 默认 `pinned_at DESC`。

### 2.4 Export 实现

消息分块拉取、文件名 sanitize、Markdown/JSON 双格式；观测 RPC 独立 `session_observability.go`。

### 2.5 前端分层

`features/session/*` composable + `components/sessions/*`；wire 映射集中在 `api.ts`。

---

## 3. 问题与风险

### P1 — 当前迭代应修

| ID | 问题 | 证据 | 计划 ID |
|----|------|------|---------|
| **SESS-R-P1-01** | Participants 每次 List 全量 Sync | `ListParticipants` → `listAllMessages` + `DELETE` + 全量 `INSERT` | **F6-a** |
| **SESS-R-P1-02** | Export 无界内存 / 响应体 | 全消息 + `Timeline(limit=0)` 一次进内存；proto inline `content` | **ARCH-03** |
| **SESS-R-P1-03** | Chat Timeline 全量拉取 | `getSessionTimeline(id)` 无 limit；`clampTimelinePageLimit(0,total)=total` | **FE-TL-01** |

**SESS-R-P1-03 回归说明**：去掉 2000 cap 后语义更正确，但 Chat `SessionTimelineDialog` 更易 OOM/超时；详情页已分页，主聊天入口更危险。

### P2 — 下一迭代

| ID | 问题 | 计划 ID |
|----|------|---------|
| SESS-R-P2-01 | `ListSessionRuns` 绕过 biz，service 直调 repo | 抽 biz 门面 |
| SESS-R-P2-02 | MCP 分类 `LIKE '%mcp%'` 启发式易误判 | 统一 `source='mcp'` |
| SESS-R-P2-03 | `limits.go` dead code（O3 旧 merge 路径） | 删除或改测 |
| SESS-R-P2-04 | `sessionTimelineSummary` 漏计 MCPCount | 修 Total 聚合 |
| SESS-R-P2-05 | `ListParticipants(ctx, id, repo)` repo 作参数 | 注入 usecase |
| SESS-R-P2-06 | 无 `session_timeline.go` UNION / Export 集成测 | 补单测 |
| SESS-R-P2-07 | Pin Update 不校验 affected rows | 与 Archive 对齐 |
| SESS-R-P2-08 | M55 runs ≠ design §4.3 编排 schema | **F4-schema** 定稿 |

### P3 — 优化

| ID | 项 |
|----|-----|
| SESS-R-P3-01 | hydrate 丢 ref 时静默跳过 — 可加 partial 标记 |
| SESS-R-P3-02 | `participantFromMessage` `_ = json.Unmarshal` |
| SESS-R-P3-03 | Export Markdown 未含 Runs/Participants |
| SESS-R-P3-04 | 收藏仍 localStorage（**FE-FAV**） |

---

## 4. 影响域与回归清单

| 变更 | 影响面 | 风险 |
|------|--------|------|
| Timeline UNION | 所有 `GetSessionTimeline` | **高** — COUNT 慢；依赖索引 |
| 移除 2000 cap | Chat Timeline 对话框 | **高** — 全量加载 |
| Pin RPC | Search、侧栏排序 | **低** |
| Export | 新 RPC | **低** — 大会话超时 |
| Participants Sync | 每次 List API | **中** — 写放大 |
| Wire 新依赖 | admin 构建 | **低** — stub 已补 |

**建议回归**：

1. 小会话（<100 msg）：Timeline 分页、Export、Pin 排序  
2. 大会话（>5k msg）：Export 超时、Participants Tab、Chat Timeline 对话框  
3. Team 多 agent：Participants 角色/handoff  
4. `kind_filter=tool|mcp|skill` 与混合 filter 计数一致  
5. `make api && make wire && go test ./internal/service/...`

---

## 5. 建议修复顺序

| 顺序 | 任务 | 理由 |
|------|------|------|
| 1 | **F6-a** / SESS-R-P1-01 | 消除 Participants O(n) 与写放大 |
| 2 | **FE-TL-01** / SESS-R-P1-03 | 降低 Timeline 最大回归面 |
| 3 | **ARCH-03** / SESS-R-P1-02 | Export 生产可用性 |
| 4 | 删 dead code + timeline SQL 单测 | 质量债 |
| 5 | Runs 经 biz 门面 + MCP 分类统一 | 架构一致 |

---

## 6. 与基线 Review 对照

| 基线 (2026-05-21) | 本次 |
|-------------------|------|
| SESS-P1-01 usecase 体量大 | 🟡 已拆 timeline/export/participant |
| SESS-P1-02 压缩单测 / minGap | 未动 |
| SESS-P2-02 Turns 缺导出 | ✅ F2 Export |
| 批量治理 UI | 已有 + pin/export 增强 |

---

## 7. 结论

本轮在 **功能闭环**（Pin、Export、Timeline 正确分页、Runs/Participants 可读）和 **结构拆分** 上进展扎实。主要短板是 **Participants 读时全量 Sync** 与 **Export/Chat Timeline 无界加载**——「能演示、大会话会痛」的 P1，非架构红线。

**评级：83/100，P1，建议合并后继续 Phase 2 收尾。**
