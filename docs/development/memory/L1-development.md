# L1 — 开发计划

> **需求**：[`L1.md`](./L1.md) · **设计**：[`L1.design.md`](./L1.design.md)
> **进度真相**：以本文为准；需求/设计正文不写修复记录。

---

## 现状（2026-06-06）

| 项 | 状态 | 证据 |
|----|------|------|
| `memory_l1_*` 表 | ✅ | `memory_l1_tasks` / `memory_l1_fields` / `memory_l1_field_history` |
| Admin ListL1Tasks/Fields | ✅ | `internal/biz/memory_admin_store.go` |
| working_memory 工具（5 个） | ✅ | `internal/tools/working_memory/tools.go`（read/list/write/patch/delete） |
| L1 写入 Store | ✅ | `internal/data/sessionmemory/store_l1.go`（StartTask/EndTask/UpsertField/DeleteField/PatchFields/ArchiveTask） |
| L1 Writer Biz | ✅ | `internal/biz/memory_admin_store.go`（L1Writer 接口 + DTO） |
| task→episode 归档 hook | ✅ | `internal/biz/memory_admin_usecase.go`（EndL1Task 自动归档 + 创建 L2 Episode） |
| L1 归档 Worker | ✅ | `internal/cronrunner/jobs/memory_l1_archive.go` |
| Context 注入 | ✅ | `internal/agent/working_memory_inject.go`（BeforeToolHook 注入 L1Writer/L1Reader） |
| Prompt 注入 | ✅ | `internal/agent/l1_prompt.go`（L1MemoryCue） |
| 工具 Catalog 注册 | ✅ | `internal/data/builtin_tools_seed.go`（5 个 working_memory 工具种子） |
| shared_with 传输层 | ✅ | Schema + Store + Proto + Frontend 管道已打通 |
| Team shared_with 业务逻辑 | 🟡 | 传输层管道已打通，但无写入/权限验证/跨 agent 共享逻辑 |
| field history UI | ❌ | 字段版本历史前端展示，revision 字段存在但无 UI |

---

## 待办

| # | 任务 | 状态 | 优先级 |
|---|------|------|--------|
| L1-1 | Team shared_with 写入/权限验证/跨 agent 共享逻辑 | 🟡 | P2 |
| L1-2 | field history UI（字段版本历史展示 + 回滚） | ❌ | P3 |

---

## 代码锚点

- `internal/data/memory_shim_l1.go` — L1 表读写（StartL1Task/EndL1Task/UpsertL1Field/DeleteL1Field/PatchL1Fields/ArchiveL1Task）
- `internal/tools/working_memory/tools.go` — 5 个 working_memory 工具
- `internal/biz/memory_admin_store.go` — L1Writer 接口
- `internal/biz/memory_admin_usecase.go` — EndL1Task 归档 hook
- `internal/cronrunner/jobs/memory_l1_archive.go` — 空闲任务归档 Worker
- `internal/agent/l1_prompt.go` — L1MemoryCue prompt 注入
- `internal/agent/working_memory_inject.go` — BeforeToolHook 注入

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


