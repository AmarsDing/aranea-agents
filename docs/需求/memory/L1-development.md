# L1 — 开发计划

> **需求**：[`L1.md`](./L1.md) · **设计**：[`L1.design.md`](./L1.design.md)

---

## 现状

| 项 | 状态 |
|----|------|
| `memory_l1_*` 表 | ✅ |
| Admin ListL1Tasks/Fields | ✅ |
| working_memory 工具 | 🟡 |
| task→episode 归档 | 🟡 |
| 前端工作记忆 Tab | 🟡 |

---

## 待办

| # | 任务 | 状态 |
|---|------|------|
| L1-1 | working_memory 工具端到端 | 🟡 |
| L1-2 | 自动 episode 归档 hook | 🟡 |
| L1-3 | field history UI | ❌ |
| L1-4 | Team shared_with_json | ❌ |

---

## 代码锚点

- `internal/data/sessionmemory/store*.go`
- `internal/service/memory.go`
- `internal/biz/memory_admin_*.go`

---

## 附录：原落地阶段 / 运行时（迁移自分层需求文）

## 11. 落地实施阶段

### Phase 1（最小可用，1～2 周）

- [ ] §3.1～§3.3 三表落库 + ensureLegacyColumns 兼容。
- [ ] `MemoryL1Service.{StartTask, EndTask, GetField, SetField, ListFieldsByTask}`。
- [ ] `working_memory.read/write/list` 工具注册。
- [ ] `ChatService` 在用户首条消息时自动 StartTask。
- [ ] L0 `RenderForPrompt` 接入。
- [ ] 单元测试：overflow / revision 冲突 / 渲染。

### Phase 2（Schema 与 Team，1 周）

- [ ] §3.4 schemas 表 + §6.2 schema 管理接口。
- [ ] JSON Schema 校验集成（`github.com/santhosh-tekuri/jsonschema/v5`）。
- [ ] `TeamRuntime` 多子 task 创建与 shared_with。
- [ ] 缺失字段提示注入 prompt。

### Phase 3（前端与历史，1 周）

- [ ] §8.2 工作记忆 Tab。
- [ ] 字段历史 + 回滚。
- [ ] §8.3 Schema 管理页。
- [ ] §8.4 Trace 详情 L1 节点。

### Phase 4（治理与扩展）

- [ ] TTL 过期定时任务（cron 1 分钟一次）。
- [ ] Idle 任务归档（cron 10 分钟一次）。
- [ ] 与 L2 episode 落库联调。
- [ ] L1 字段升档 L3 的策略（在 L3 文档定义）。

---


