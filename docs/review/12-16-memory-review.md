# 12–16 Memory L0–L4 Review

> **评分**：74 / 100 | **风险等级**：P1  
> **文档**：[12-16 memory.md](../需求/12-16%20memory.md) · [12-16 memory.design.md](../需求/12-16%20memory.design.md) · [12-16 memory-development.md](../需求/12-16%20memory-development.md)  
> **代码锚点**：`internal/biz/memory.go` · `internal/runtime/memory_set.go` · `internal/biz/memory_l4*.go` · `internal/biz/memory_worker.go` · `internal/memory/trpc/` · `internal/data/sessionmemory/`  
> **审查时间**：2026-05-21  
> **状态**：已于 2026-05-21 校正 — P0 边界迁至 `internal/runtime/memory_set.go`；双轨见 design §1.1；L4 衰减已实现。见 `docs/changelog/2026-05-21-Review-Optimization-Phase0.md`。

---

## 评分详情

| 维度 | 得分 | 满分 | 评述 |
|------|------|------|------|
| 需求符合度 | 15 | 20 | L0-L3 + L4 启发式注入 + MemoryWorker + AutoMemory 图写入 ✅；冲突检测/级联/衰减待补 |
| 架构一致性 | 18 | 25 | `memory.RuntimeSet` 端口统一 ✅；MemorySet 已迁至 `internal/runtime` ✅；双轨主从见 design §1.1 ✅ |
| 后端实现质量 | 16 | 20 | L4 图自动写入 ✅；冲突检测 ✅；L4 衰减 ✅（I2-MEM-01） |
| 前端实现质量 | 12 | 15 | 记忆中心 4+1 Tab ✅；图谱 Tab 默认隐藏（`VITE_MEMORY_GRAPH_TAB=1` 启用） |
| 测试与验证 | 7 | 10 | `memory_worker_test.go` ✅；l4_governance 测试 ✅ |
| 文档一致性 | 6 | 10 | 三件套 + UX 文档；`12-16 memory.md` UX 文档与产品文档边界清晰；文档更新于 DocSync changelog |

---

## Memory 五层模型

| 层 | 名称 | 存储 | 状态 |
|----|------|------|------|
| L0 | Sensory（感官）| trpc Session（上下文窗口） | ✅ |
| L1 | Working（工作记忆）| SQLite `sessionmemory` | ✅ |
| L2 | Episodic（情节记忆）| SQLite，按对话分组 | ✅ |
| L3 | Semantic（语义记忆）| pgvector Knowledge | ✅ 基础 |
| L4 | Persistent（持久记忆）| SQLite graph + 实体关系 | ✅ 基础；衰减 ✅；图谱 Tab feature flag |

---

## 架构边界 — P0 已闭合

`memory_runtime_set.go` 已删除；`internal/runtime/memory_set.go` 负责框架 MemoryService 组装，biz 层仅持端口类型。`make runtime-boundary` CI 通过。

## 双轨问题

**当前状态**：  
- `trpc-agent-go/memory.Service`：Runner 层跨会话记忆接口（framework 真相源）
- Aranea L0-L4：产品级 5 层记忆模型

**期望关系**：Aranea L0-L4 **实现** `trpc-agent-go/memory.Service` 接口（产品是框架的实现）

**现状**：两者并行存在，主从关系未在代码中明确。

---

## 已验收功能

| 功能 | 状态 |
|------|------|
| L0-L3 读/写 | ✅ |
| L4 prompt 注入 | ✅ |
| L4 图自动写入（`L4GraphWriter`）| ✅ M4 |
| MemoryWorker（异步处理）| ✅ |
| AutoMemory 图写入（cronrunner `auto_memory` job）| ✅ |
| `l4_governance.go` 冲突检测 | ✅ I2-MEM-01 |
| 级联更新 | 🟡 基础 |
| 衰减算法 | ❌ |
| Memory 管理页 5 Tab | ✅ |
| 图谱与进化 Tab | 🟡 占位 |
| `MemorySnapshotDrawer.vue` | ✅ |
| pgvector L3（Agent 级别） | ✅ 基础 |
| pgvector 多租户稳定性 | 🟡 |

---

## 主要风险

### P0

| ID | 问题 | 建议修复 |
|----|------|---------|
| MEM-P0-01 | `biz/memory_runtime_set.go` 疑似违反 biz 不 import trpc-agent-go 红线 | 立即运行 `make runtime-boundary`；选方案 A 或 B 修复 |

### P1

| ID | 问题 | 建议修复 |
|----|------|---------|
| MEM-P1-01 | L0-L4 产品层与框架 MemoryService 双轨，主从未定义 | 明确 L0-L4 是 MemoryService 的产品实现；在设计文档中更新 |
| MEM-P1-02 | `MemoryWorker` 无测试；异步处理失败无 dead letter | 补 worker 单测；加失败告警 |
| MEM-P1-03 | 记忆中心"图谱与进化"Tab 为占位，不应展示给用户 | 加 feature flag 隐藏或实现基础视图 |

### P2

| ID | 问题 | 建议修复 |
|----|------|---------|
| MEM-P2-01 | 衰减算法未实现 | 规划 L4 衰减策略 |
| MEM-P2-02 | pgvector 多租户（多 Agent/多用户隔离）稳定性待验证 | 补多租户集成测试 |

---

## 建议优化路径

1. **立即**：运行 `make runtime-boundary` 确认 `biz/memory_runtime_set.go` 边界。
2. **本迭代**：修复双轨问题，明确 L0-L4 是 MemoryService 的产品实现。
3. **下迭代**：补 MemoryWorker 单测；实现 L4 衰减算法。
4. **长期**：图谱与进化 Tab 完整实现。
