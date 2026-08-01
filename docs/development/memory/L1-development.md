# L1 — 开发计划

> **需求**：[`L1.md`](./L1.md) · **设计**：[`L1.design.md`](./L1.design.md)
> **进度真相**：以本文为准；需求/设计正文不写修复记录。

---

## 现状（2026-06-06）

| 项 | 状态 | 证据 |
|----|------|------|
| `memory_l1_*` 表 | ✅ | `memory_l1_tasks` / `memory_l1_fields` / `memory_l1_field_history` |
| Admin ListL1Tasks/Fields | ✅ | `internal/biz/memory_admin_store.go` |
| working_memory 工具（6 个） | ✅ | `internal/tools/working_memory/tools.go`（read/list/write/patch/delete/complete） |
| L1 写入 Store | ✅ | `internal/data/memory_shim_l1.go`（StartTask/EndTask/UpsertField/DeleteField/PatchFields/ArchiveTask） |
| L1 Writer Biz | ✅ | `internal/biz/memory_admin_store.go`（L1Writer 接口 + DTO） |
| task→episode 归档 hook | ✅ | `internal/biz/memory_admin_usecase.go`（EndL1Task 异步归档 + 创建 L2 Episode，P1-2 起异步化） |
| L1 归档 Worker | ✅ | `internal/cronrunner/jobs/memory_l1_archive.go`（P1-2 起含 ended-unarchived 重试 + 死信告警） |
| Context 注入 | ✅ | `internal/agent/working_memory_inject.go`（BeforeToolHook 注入 L1Writer/L1Reader） |
| Prompt 注入 | ✅ | `internal/agent/l1_prompt.go`（L1MemoryCue） |
| 工具 Catalog 注册 | ✅ | `internal/data/builtin_tools_seed.go`（6 个 working_memory 工具种子） |
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

- `internal/data/memory_shim_l1.go` — L1 表读写（StartL1Task/EndL1Task/UpsertL1Field/DeleteL1Field/PatchL1Fields/ArchiveL1Task/ListIdleL1Tasks 双分支扫描）
- `internal/tools/working_memory/tools.go` — 6 个 working_memory 工具（complete 为 P1-2 任务结束触发源）
- `internal/biz/memory_admin_store.go` — L1Writer 接口
- `internal/biz/memory_admin_usecase.go` — EndL1Task 归档 hook（P1-2 起 safego 异步，失败由 Worker 重试分支兜底）
- `internal/cronrunner/jobs/memory_l1_archive.go` — 空闲任务归档 Worker（ended-unarchived 重试 + 连续失败死信告警 `system.memory_l1_archive.failed`）
- `internal/agent/l1_prompt.go` — L1MemoryCue prompt 注入
- `internal/agent/working_memory_inject.go` — BeforeToolHook 注入

---

## P1-2 归档原子化（2026-07-31 完成）

依据 `docs/reports/2026-07-29-review-memory-system-redesign.md` P1-2：

1. **任务结束触发**：新增 `working_memory_complete` 工具，LLM 显式声明任务完成 → `EndL1Task('completed')` → usecase 归档 hook（原子归档 + episode + Path A/B）。设计裁定：不采用 per-turn 自动归档（`L1MemoryCue` 只读 active/paused 任务，每轮归档会导致 pinned 工作记忆跨轮丢失）。
2. **失败可重试（留在扫描集合内）**：`ListIdleL1Tasks` 扫描集合扩展为双分支——idle active（60min cutoff，EndL1Task+归档）与 ended-unarchived（2min retry cutoff，仅重试归档 tx，不再 re-end）。任何路径下任务 ended 而归档失败都会在后续 tick 重试，不再静默丢失。
3. **死信告警**：Worker 内存计数每任务连续归档失败次数，阈值 3 次触发 flow-log 告警（`system.memory_l1_archive.failed`，Monitor Logs 流程日志 Tab 可见），之后每 10 次重发；成功归档即重置。任务始终留在扫描集合持续重试。
4. **归档 hook 异步化**：usecase `EndL1Task` 的归档 hook 改 `safego.Go` + `context.WithoutCancel`（内含 Path B LLM 调用，不阻塞工具响应），失败由 Worker 重试分支兜底。

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


