# Aranea 综合优化路线图（2026-05-26）

> **版本**：2026-05-26 · **状态**：📋 路线图草案 · **范围**：当前所有 5 份独立优化计划的统一执行序列
> **目的**：把分散在 5 个模块（Memory · Monitor · Tools/Plugin/Skill/MCP · Channel/Chat/Agent-Team · Team Graph）的 ~150 个优化项排成 **可顺序实施 / 可灰度回滚 / 可独立 ship** 的全局路线
> **真相源**：本文档是**单一执行序列**真相源；每个 Wave 内部细节回到对应需求/开发文档

---

## 1. 已纳入的优化计划

| # | 计划编号 | 主题 | 主文档 | 开发计划 | 体量 |
|---|---------|------|--------|----------|------|
| 1 | **M56 BLO** | Channel × Chat × Agent/Team 业务模型 5 主题 | [`56 business-logic-optimization.md`](./56%20business-logic-optimization.md) | [`56-business-logic-optimization-development.md`](./56-business-logic-optimization-development.md) | 5 主题 / ~40 任务 / 12 周 |
| 2 | **M57 TPM** | Tools / Plugin / Skill / MCP 子系统代码债 | [Review](../review/2026-05-26-Tools-Plugin-Skill-MCP-Code-Review.md) | [`57-tools-plugin-skill-mcp-optimization-development.md`](./57-tools-plugin-skill-mcp-optimization-development.md) | 12 P1 + 30 P2 + 13 P3 + 14 D / 4 Wave |
| 3 | **MEM-OPT** | Memory 业务逻辑（一致性 / 衰减 / 队列 / PII / 提取 / Cascade） | [`memory-optimization-2026-05-26.md`](./memory/memory-optimization-2026-05-26.md) | 内嵌 §8 排期 | 6 主题 / 5 Sprint / 11 周 |
| 4 | **MON-OPT** | Monitor 业务逻辑（Bus 分离 / 冷却 / 评估 / 反压 / Trace / DSL） | [`18 monitor-optimization-2026-05-26.md`](./18%20monitor-optimization-2026-05-26.md) | 内嵌 §8 排期 | 6 主题 / 6 Sprint / 13 周 |
| 5 | **TG-Q** | Team Graph 代码债（review 直接 backlog） | [Review §6](../review/2026-05-26-Team-Graph-Code-Review.md#6-问题清单按优先级) | 无独立 dev plan | 5 P1 + 6 P2 + 3 P3 / ~3 周 |

**合计**：~150 任务项 / 估算 30+ 人周。

---

## 2. 全局依赖图

```mermaid
flowchart TD
    classDef p0 fill:#fee,stroke:#c33,color:#900
    classDef p1 fill:#fef3c7,stroke:#d97706,color:#92400e
    classDef p2 fill:#dbeafe,stroke:#2563eb,color:#1e40af
    classDef p3 fill:#dcfce7,stroke:#16a34a,color:#166534

    M56_BLO5[M56 BLO-5 Unified BackgroundJob 基础设施]:::p0
    M56_BLO4[M56 BLO-4 Non-Blocking HITL]:::p1
    M56_BLO2[M56 BLO-2 Multi-Signal Escalation]:::p1
    M56_BLO1[M56 BLO-1 Intent-Aware Admission]:::p2
    M56_BLO3[M56 BLO-3 Channel Trigger Rules]:::p2

    M57_W1[M57 TPM Wave 1 P1×12]:::p0
    M57_W2[M57 TPM Wave 2 P2 + D-T1/S2/M2]:::p1
    M57_W3[M57 TPM Wave 3 D-P1/P2/P4/T2/S1/S3/S4]:::p2
    M57_W4[M57 TPM Wave 4 EventSourcing/OPA]:::p3

    MEM_A[MEM-OPT-01 双轨一致性 + MEM-OPT-03 队列优先级]:::p0
    MEM_B[MEM-OPT-02 L4 衰减 + Phase3 Reconciler]:::p1
    MEM_C[MEM-OPT-05 提取协议化 + MEM-OPT-04 PII Block]:::p1
    MEM_D[MEM-OPT-06 Cascade Saga + Dry-Run]:::p2
    MEM_E[MEM-OPT-04 PII Review 工作流]:::p2

    MON_A[MON-OPT-01 Bus 全分离 + MON-OPT-04 优先级队列]:::p0
    MON_B[MON-OPT-02 冷却持久化 + MON-OPT-03 RingBuffer 评估]:::p1
    MON_C[MON-OPT-05 Trace Projector]:::p1
    MON_D[MON-OPT-05 跨 trace 关联 + Lossless 模式]:::p2
    MON_E[MON-OPT-06 Registry + DSL]:::p2
    MON_F[MON-OPT-02 escalation + silence_windows]:::p2

    TG_P1[TG-Q-01..05 状态常量/GC 调度/拆函数/幽灵函数/单向依赖]:::p1
    TG_P2[TG-Q-06..11 critic 协议/watch 测试/resume 错误暴露]:::p2

    M56_BLO5 --> M56_BLO4
    M56_BLO5 --> M56_BLO2
    M56_BLO5 --> M56_BLO3
    M56_BLO4 --> M56_BLO1

    M57_W1 --> M57_W2
    M57_W2 --> M57_W3
    M57_W3 --> M57_W4

    MEM_A --> MEM_B
    MEM_A --> MEM_C
    MEM_C --> MEM_D
    MEM_D --> MEM_E
    MEM_C -.| MEM-OPT-05 用 schema | MON_C

    MON_A --> MON_B
    MON_B --> MON_C
    MON_C --> MON_D
    MON_B --> MON_F

    TG_P1 --> TG_P2

    M57_W1 -.| 同属 P1 速胜 | TG_P1
    M56_BLO5 -.| BackgroundJob 抽象可被复用 | MEM_A
    MON_A -.| Bus 分离影响 Memory Worker 事件 | MEM_A
```

**关键依赖判断**：
- **M56 BLO-5（BackgroundJob 抽象）解锁三个上层（BLO-1/2/3/4）** —— 必须最早完成。
- **MEM-OPT-01（一致性）与 MON-OPT-01（Bus 分离）互不依赖但都是 P0** —— 可并行。
- **M57 TPM Wave 1（12 项 P1）多为小修复（XS/S 体量）** —— 适合穿插在任意 Sprint 当"速胜"。
- **TG-Q-01..05** —— 5 项 P1 都是 1-2 天小改，可在 Sprint 间填空。

---

## 3. 全局执行序列（4 阶段）

按 **风险 + 依赖 + 业务可见度** 排序。每阶段产出 **可独立灰度上线 + 可回滚** 的能力集合。

### Phase 0 — 准备（0.5 周）

| 序号 | 任务 | 来源 | 工时 |
|------|------|------|------|
| P0-1 | 写 `56 business-logic-optimization.design.md`（BLO-PRE-01） | M56 | 1d |
| P0-2 | 注入 5 个 BLO Feature flag（BLO-PRE-02） | M56 | 0.5d |
| P0-3 | Datadog 看板雏形（BLO-PRE-03） | M56 | 0.5d |
| P0-4 | 本路线图归档到 `docs/需求/0-system-development.md §8.7`（路线图引用） | 本文 | 0.5d |

**Gate 0**：5 个 flag 默认 off，Datadog 看板可显示零数据。

---

### Phase 1 — 关键正确性 + 基础设施（4 周）

> **目标**：消除 P0/P1 业务正确性缺陷；为后续 Phase 提供基础设施。

#### Sprint 1.1（第 1-2 周）— 基础设施 + 静默失败收敛

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 1 | **M56 BLO-5 Sprint A1**（biz/data 层 BackgroundJob 抽象） | M56 | 1 周 | `internal/biz/backgroundjob/` + 表 |
| 2 | **MON-OPT-01 Phase 0/1**（Bus 路由表 + dual 灰度） | MON | 3d | `event.Infra.Publish` 路由 |
| 3 | **MEM-OPT-01 Phase 0/1**（fact `index_status` 列 + 写路径错误捕获） | MEM | 2d | DDL + sync 错误处理 |
| 4 | **TG-Q-01**（提取 `biz.TeamRunStatus*` 常量） | TG | 1d | `biz/team_run_status.go` |
| 5 | **TPM-P1-02 / P1-11 / P1-07**（XS 修复 3 项） | M57 | 1d | runtime alias / mcp probe / skill zipslip |

**Gate 1.1**：
- `BackgroundJob` 表上线，CRUD + `TryClaim` 单测通过
- Bus 路由表灰度 dual 模式下 flow_log 双发，监控页无可见变化
- Memory fact 写路径异常 100% 标 `index_status='stale'`，告警可见
- `failed` vs `error` 字面量全库统一为 `biz.TeamRunStatus*`

#### Sprint 1.2（第 3 周）— 业务可见的运维改善

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 6 | **MON-OPT-04 优先级 channel + drop metric** | MON | 3d | WS high/normal/low 三优先级 |
| 7 | **MON-OPT-02 firing 状态机 + DB 持久化冷却** | MON | 2d | `monitor_alert_rules` 加列 |
| 8 | **MEM-OPT-03 队列优先级 + Dead-Letter 表** | MEM | 3d | `MemoryJobQueue` 三档 + DL |
| 9 | **TG-Q-02**（`CleanupStaleSessions` 接入 cron） | TG | 0.5d | wire ticker |
| 10 | **TPM-P1-05 / P1-04**（plugin chain panic recover / output policy） | M57 | 2d | hook resilience |

**Gate 1.2**：
- WS 反压 metric 上 Datadog；告警优先级永不被丢
- 重启 / 多实例下告警 Webhook 重复率 0%
- Memory 高负载 session 不再静默失忆，dead-letter 可见
- Team Graph 长跑进程 Coordinator.sessions 不积累

#### Sprint 1.3（第 4 周）— Dispatcher + 评估批量化

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 11 | **M56 BLO-5 Sprint A2**（Dispatcher + 双 worker 池 + DAG） | M56 | 1 周 | runtime/backgroundjob/ |
| 12 | **MON-OPT-03 RingBuffer + EvalWorker** | MON | 3d | 评估 DB QPS -99% |
| 13 | **MEM-OPT-01 Phase 2/3**（读路径校验 + Reconciler cron） | MEM | 3d | pgvector 一致性收敛 |
| 14 | **TPM-P1-10 + P1-12**（MCP transport 归一化） | M57 | 2d | NormalizeTransport |
| 15 | **TPM-P1-01**（web_search alias 对齐） | M57 | 0.5d | runtime alias 表收敛 |

**Gate 1.3**：
- Dispatcher 100 Job 含 parent/child 全正确流转
- 1000 QPS completion 下评估 CPU < 5%
- Cascade Approve → pgvector 收敛时延 ≤ 15s

**Phase 1 总体 Gate**：
- ✅ MEM-OPT-01 / 03 完成（Memory 业务正确性 P0 消除）
- ✅ MON-OPT-01 / 02 / 03 / 04 完成（Monitor 4 项 P1 消除）
- ✅ M56 BLO-5 Sprint A1/A2 完成（BackgroundJob 基础就绪）
- ✅ 11 项 M57 P1 / TG-Q-01/02 速胜
- ✅ 所有改动有 Feature flag，可回滚

---

### Phase 2 — 异步任务 + 业务能力提升（4 周）

> **目标**：把 Phase 1 基础设施转化为业务能力（HITL 不阻塞 / 智能升级 / 反馈强化）。

#### Sprint 2.1（第 5 周）— BackgroundJob 接入 + Trace 写入

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 16 | **M56 BLO-5 Sprint A3**（迁移 Channel/Chat 异步至 BackgroundJob） | M56 | 1 周 | SessionRun durable + Channel async 接入 |
| 17 | **MON-OPT-05 MonitorTraceProjector** | MON | 3d | `monitor_traces` 100% 落行 |
| 18 | **TPM-P1-03 / P1-06**（cost_guard / skill summary） | M57 | 1.5d | double-block / slug 修复 |

**Gate 2.1**：
- `GET /v1/background-jobs` 统一返回 Channel + Session 任务
- Traces Tab 不再空，所有新 turn 落 trace 行

#### Sprint 2.2（第 6-7 周）— HITL 异步 + Escalation 智能化

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 19 | **M56 BLO-4**（Non-Blocking HITL 全套，3 Sprint 合并） | M56 | 2.5 周 | PendingTask 异步 |
| 20 | **M56 BLO-2**（Multi-Signal Escalation，并行） | M56 | 1 周 | EscalationPolicy |
| 21 | **MEM-OPT-02 L4 Decay Worker + 强化因子（无 UI）** | MEM | 1 周 | confidence 业务化公式 |
| 22 | **TG-Q-03 / TG-Q-05**（拆 620 行 + 移除 chatactivity 依赖） | TG | 3d | God function 收敛 |

**Gate 2.2**：
- `await_user_reply` 期间同 session 新 turn 可执行
- tool_calls=9 自动升 durable
- L4 entity 半年不活跃 confidence ≤ 0.4

#### Sprint 2.3（第 8 周）— 提取协议化 + Trace 关联

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 23 | **MEM-OPT-05**（function call schema 双轨提取） | MEM | 1 周 | extraction_quality 字段 |
| 24 | **MON-OPT-05 跨 trace 关联** | MON | 3d | parent_trace_id |
| 25 | **TG-Q-04**（删除幽灵函数 / 改写 E2E 直驱 watch） | TG | 1d | 真路径测试 |
| 26 | **TPM-P1-08 / P1-09**（skill Saga apply / mcp OAuth） | M57 | 1 周 | Wave 1 收口 |

**Gate 2.3**：
- LLM 提取解析成功率 ≥ 99.5%；heuristic 占比 < 5%
- Run 详情 Waterfall 跨 turn 可跳转

**Phase 2 总体 Gate**：
- ✅ M56 BLO-2/4/5 全完成
- ✅ MEM-OPT-02/05 完成
- ✅ MON-OPT-05 完成
- ✅ TG-Q-03/04/05 完成
- ✅ M57 Wave 1 全部 12 项 P1 完成

---

### Phase 3 — 体验与生态扩展（5 周）

> **目标**：用户/Admin 可见的产品能力扩展（Cascade Saga / PII 分级 / 群智能体 / Intent admission / 自定义告警 DSL）。

#### Sprint 3.1（第 9-10 周）— Cascade Saga + PII 升级

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 27 | **MEM-OPT-06**（Cascade Saga + Dry-Run + 前端 Tab 升级） | MEM | 3 周 | partial-fail 可恢复 |
| 28 | **MEM-OPT-04 PII Block 模式** | MEM | 1 周 | strict 合规可用 |
| 29 | **M56 BLO-1**（Intent-Aware Admission，3 Sprint） | M56 | 2.5 周 | classifier v0+v1 |
| 30 | **TG-Q-07**（critic_loop 协议化 → tool call） | TG | 2d | 字符串协议消除 |

#### Sprint 3.2（第 11-12 周）— Channel 触发器 + 告警 DSL + PII Review

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 31 | **M56 BLO-3**（Channel Trigger Rules，4 Sprint） | M56 | 4 周 | schedule/keyword/reaction/silent |
| 32 | **MON-OPT-06**（Registry + DSL 解析 + 旧规则迁移） | MON | 2 周 | 自定义 metric 不改代码 |
| 33 | **MEM-OPT-04 Review 工作流** | MEM | 2 周 | PII pending review Tab |
| 34 | **MON-OPT-02 escalation + silence_windows** | MON | 1 周 | 告警生命周期升级 |
| 35 | **TG-Q-06 / TG-Q-08 / TG-Q-09**（adaptive 裁剪可见 / watch 测试 / resume 错误暴露） | TG | 3d | P2 收口 |

#### Sprint 3.3（第 13 周）— MON Lossless + M57 Wave 2 部分

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 36 | **MON-OPT-04 Lossless 订阅模式** | MON | 3d | client 主动断重连 |
| 37 | **M57 Wave 2 安全 + 静默吞错组**（TPM-P2-08/11/12/27 + P2-01/02/13/21/25） | M57 | 2 周 | SerpAPI/PII/fail-open |

**Phase 3 总体 Gate**：
- ✅ M56 BLO-1/3 完成（5 主题全完成）
- ✅ MEM-OPT-04/06 完成（Memory 6 主题全完成）
- ✅ MON-OPT-06 完成（Monitor 6 主题全完成）
- ✅ TG-Q-06..09 完成（Team Graph P2 全部）
- ✅ M57 Wave 2 安全 + 静默吞错组完成

---

### Phase 4 — 架构升级与中长期愿景（按需启动，6-12 周）

> **目标**：进入"产品化 + 平台化"深水区。**Phase 1-3 完成后再评估是否启动**。

| # | 任务 ID | 来源 | 工时 | 产物 |
|---|---------|------|------|------|
| 38 | **M57 Wave 2 死配置/性能组**（TPM-P2-03/09/10/14/15/18/26/30） | M57 | 2 周 | 配置清理 + 缓存 |
| 39 | **M57 Wave 3 重设计**（D-P1/P2/P4/T2/S1/S3/S4 + Schema-as-Code） | M57 | 4-6 周 | Cost Reservation / Hook Isolation 等 |
| 40 | **TG-Q-10..14**（代码质量收口 + 魔法常量配置化） | TG | 1 周 | P3 收口 |
| 41 | **M56 收口 Sprint F1/F2**（灰度 soak + 旧路径下线） | M56 | 3 周 | BLO 全量上线 |
| 42 | **M57 Wave 4 中长期**（EventSourcing / OPA / MCP FSM / Plugin Scope） | M57 | 8-12 周 | 平台化基础 |

---

## 4. 跨计划资源共享与并行机会

| 共享项 | 主题方 | 复用方 |
|--------|--------|--------|
| **Feature flag 框架** | M56 BLO-PRE-02 | MEM / MON / TPM / TG 共用 |
| **Datadog 看板** | M56 BLO-PRE-03 | 加面板即可 |
| **BackgroundJob 抽象** | M56 BLO-5 | MEM-OPT-03 队列可考虑迁；MON-OPT-03 EvalWorker 可注册为 scheduled job |
| **Function call schema** | MEM-OPT-05 | TG-Q-07（critic_loop）；MON 告警 enrichment |
| **Bus 路由表** | MON-OPT-01 | 所有发 Envelope 的模块 |
| **PolicyVersion 常量** | MEM-OPT 衍生 | 所有 ActionLog 写入方 |

**可并行 lanes**：

```
Lane A（基础设施）: M56 BLO-5 → BLO-4/2 → BLO-1/3
Lane B（Memory）  : MEM-OPT-01 → 03 → 02/05 → 04/06
Lane C（Monitor） : MON-OPT-01 → 04 → 02/03 → 05 → 06
Lane D（Tools）   : M57 Wave 1（小修速胜）→ Wave 2 → Wave 3
Lane E（Team）    : TG-Q-01/02/05（P1 速胜）→ 04/03 → 07/08/09
```

**最佳并行度**：5 lane 同跑需 ≥ 3 后端工程师 + 1 前端 + 1 QA。  
**单工程师串行**：按 Phase 1.1 → 1.2 → 1.3 → 2.x → 3.x 顺序，每周 1 sprint。

---

## 5. 推荐执行序列（Top-30 优先级展开）

| 顺序 | 任务 ID | 模块 | 体量 | 业务价值（一句话） | 依赖 |
|------|---------|------|------|------------------|------|
| 1 | M56 BLO-PRE-01..03 | infra | XS | 设计文档 + flag + 看板 | 无 |
| 2 | TG-Q-01 | team | XS | `failed`/`error` 常量化，Monitor 不漏判 | 无 |
| 3 | TPM-P1-02 / P1-11 / P1-07 | tools/mcp/skill | XS | 3 个 XS 修复速胜 | 无 |
| 4 | M56 BLO-5-BIZ-01..02 / DATA-01..03 | M56 | M | BackgroundJob 抽象与表 | BLO-PRE |
| 5 | MEM-OPT-01 Phase 0/1 | memory | S | fact `index_status` + 写错误捕获 | 无 |
| 6 | MON-OPT-01 Phase 0/1 | monitor | M | Bus 路由表 dual 模式灰度 | 无 |
| 7 | TG-Q-02 | team | XS | 调度 `CleanupStaleSessions` cron | 无 |
| 8 | MON-OPT-04 优先级 channel | monitor | S | WS 反压可观测；alert 永不丢 | 无 |
| 9 | MON-OPT-02 firing 状态机 | monitor | M | 重启不重发 Webhook | 无 |
| 10 | MEM-OPT-03 队列优先级 + DL | memory | M | 高峰不静默失忆 | 无 |
| 11 | TPM-P1-05 / P1-04 | plugin | M | hook panic recover / output policy | 无 |
| 12 | M56 BLO-5 Sprint A2 Dispatcher | M56 | L | DAG + cascade cancel | 步骤 4 |
| 13 | MON-OPT-03 RingBuffer 评估 | monitor | M | 监控 DB QPS -99% | 步骤 6 |
| 14 | MEM-OPT-01 Phase 2/3 | memory | M | 读校验 + Reconciler | 步骤 5 |
| 15 | TPM-P1-10/12/01 | mcp/tools | M | transport / web_search alias | 无 |
| 16 | M56 BLO-5 Sprint A3 接入 | M56 | L | Channel + Chat 异步统一 | 步骤 12 |
| 17 | MON-OPT-05 TraceProjector | monitor | M | Traces Tab 不再空 | 步骤 6 |
| 18 | TPM-P1-03/06 | plugin/skill | M | cost / summary 修复 | 步骤 11 |
| 19 | M56 BLO-4 PendingTask | M56 | L | HITL 不阻塞 session | 步骤 16 |
| 20 | M56 BLO-2 EscalationPolicy | M56 | M | 智能升级（与 19 并行） | 步骤 16 |
| 21 | MEM-OPT-02 L4 Decay Worker | memory | M | 业务化置信度 | 无 |
| 22 | TG-Q-03 / TG-Q-05 | team | M | 拆 620 行 + 解耦 chatactivity | 无 |
| 23 | MEM-OPT-05 function call schema | memory | L | 提取 99.5% 成功率 | 无 |
| 24 | MON-OPT-05 跨 trace 关联 | monitor | M | parent_trace_id | 步骤 17 |
| 25 | TG-Q-04 | team | S | 删幽灵函数 + 真路径测试 | 步骤 22 |
| 26 | TPM-P1-08 / P1-09 | skill/mcp | L | Saga apply / OAuth（Wave 1 收口） | 无 |
| 27 | MEM-OPT-06 Cascade Saga + Dry-Run | memory | L | partial-fail 可恢复 | 步骤 14 |
| 28 | M56 BLO-1 Intent Admission | M56 | L | classifier v0+v1 | 步骤 19 |
| 29 | M56 BLO-3 Channel Trigger Rules | M56 | XL | schedule/keyword/reaction/silent | 步骤 16 |
| 30 | MON-OPT-06 Registry + DSL | monitor | L | 自定义 metric 不改代码 | 步骤 13 |

> Top-30 之后回到对应需求文档自有排期：Phase 3 Sprint 3.1–3.3 + Phase 4。

---

## 6. 执行原则（贯穿所有 Phase）

| 原则 | 含义 |
|------|------|
| **Feature flag 优先** | 任何业务行为变化必须有 env / DB flag 控制；灰度可独立切换 |
| **DDL 加列默认值** | 不改既有列；新增列默认值不影响存量 |
| **静默失败必可观测** | `_ = err` 全部改为 `slog.Warn` + metric 或 status 字段持久化 |
| **路由表先行** | 改业务行为前先改路由 / 注册表（OPT-06 / OPT-01 类的设计） |
| **dual 灰度路径** | 新旧双写 → 双读校验 → 切单 → 下线（如 MON-OPT-01 dual→split） |
| **每 PR 一个 ID** | PR 标题前缀 `[BLO-X-YYY]` / `[TPM-P1-XX]` / `[MEM-OPT-N]` / `[MON-OPT-N]` / `[TG-Q-NN]` |
| **不顺带 refactor 相邻模块** | 严格按 `AGENTS.md` 执行纪律 |

---

## 7. 风险与中止条件

| 风险 | 触发条件 | 缓解 |
|------|---------|------|
| Sprint 1.1 BackgroundJob 抽象有 bug | A1 单测失败率 > 20% | 推迟 BLO-4/2/3，先稳基础 |
| MEM-OPT-01 读校验导致 SearchMemories 延迟翻倍 | p95 > 200ms | flag 关闭读校验，依赖 Reconciler |
| MON-OPT-01 Bus 路由表迁移期间 flow_log 丢失 | dual 期间双 Bus 都缺日志 | flag 回到 `MONITOR_BUS_ROUTING=session`（旧行为） |
| M56 BLO-5 旧表迁移失败 | 双写期间数据漂移 | 写新表为主 + 旧表只读快照 |
| 资源不足跨 lane 并行 | 团队 < 3 后端 | 退化为 Lane B+C 串行（先 MEM 再 MON） |

**中止 / 回退红线**：
- Phase 1 任一 Sprint Gate 失败 → 暂停后续 Sprint，先修
- 灰度租户 P95 延迟 > 1.5× 基线 → 立即回退该 flag
- 全量 `make ci` 失败 → 不进 Phase 2

---

## 8. 立即可启动的 Phase 1 Sprint 1.1 任务包

> **本节用于执行阶段**：以下 5 个任务可在第 1 周并行启动，相互无阻塞。

| # | 任务 ID | 我可以立刻动手的文件 | 预估 |
|---|---------|--------------------|------|
| 1 | **TG-Q-01** 状态常量 | 新建 `internal/biz/team_run_status.go` + 全库替换 `"failed"` / `"error"` | 1d |
| 2 | **TPM-P1-02** aliasTool 返 error | `internal/tools/runtime_alias.go` 1 行 | 0.5h |
| 3 | **TPM-P1-11** mcp probe CheckRedirect | `internal/mcp/probe/eval.go` ~10 行 | 1h |
| 4 | **TPM-P1-07** skill zipslip 加固 | `internal/skill/importer/engine.go` ~10 行 | 1h |
| 5 | **MEM-OPT-01 Phase 0** index_status DDL | `internal/data/sql/memory_chain.sql` + ent schema | 0.5d |

**5 个任务合计 ≤ 2 天工时**，全部 P1 / 体量 XS-S，无相互依赖，是路线图的**首批可立即合并 PR**。

---

## 9. 关联文档

- M56 BLO 主文档：[`56 business-logic-optimization.md`](./56%20business-logic-optimization.md)
- M56 BLO 开发计划：[`56-business-logic-optimization-development.md`](./56-business-logic-optimization-development.md)
- M57 TPM 开发计划：[`57-tools-plugin-skill-mcp-optimization-development.md`](./57-tools-plugin-skill-mcp-optimization-development.md)
- Memory OPT：[`memory/memory-optimization-2026-05-26.md`](./memory/memory-optimization-2026-05-26.md)
- Monitor OPT：[`18 monitor-optimization-2026-05-26.md`](./18%20monitor-optimization-2026-05-26.md)
- Team Graph Review backlog：[`../review/2026-05-26-Team-Graph-Code-Review.md`](../review/2026-05-26-Team-Graph-Code-Review.md)
- 系统级路线图：[`0-system-development.md`](./0-system-development.md)
- 红线规则：[`.cursor/rules/trpc-agent-framework-first.mdc`](../../.cursor/rules/trpc-agent-framework-first.mdc)
- 执行纪律：[`AGENTS.md`](../../AGENTS.md)
