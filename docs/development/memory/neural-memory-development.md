# 神经记忆系统 — 开发计划

> **版本**：2026-07-16 | **状态**：❌ 专用模块计划未启动（48 项任务仍以本表为准）
> **旁路落地说明**：编排升级 / Phase6 已部分实现重叠能力（L3 bi-temporal 列、A-MEM links、Ebbinghaus decay、sleep-time、spreading activation / Hebbian）。**勿双重记账为「Neural 已完成」**——下列任务包与 `memory_neural.go` 等专用落点仍未启动。
> **需求**：[`neural-memory.md`](./neural-memory.md) · **设计**：[`neural-memory.design.md`](./neural-memory.design.md)
> **前置开发计划**：[`memory-development.md`](./memory-development.md)
> **运行时边界**：[AGENT_RUNTIME_BOUNDARY.md](../../AGENT_RUNTIME_BOUNDARY.md)

---

## 1. 模块定位

神经记忆系统是 Memory L0-L4 的**增量能力层**，在现有五层架构之上叠加时间感知、关联链接、联动更新、仿生生命周期四个能力。不改变现有存储拓扑和双轨关系。

### 1.1 架构分层

| 层级 | 新增/修改包 |
|------|------------|
| `api/kratos/memory/v1` | Proto 新增 12 RPC + 扩展 5 RPC |
| `internal/service` | `memory_neural.go`（新增 RPC 实现） |
| `internal/biz` | `memory_temporal.go`、`memory_links.go`、`memory_linked_update.go`、`memory_consolidation.go`、`memory_bio_decay.go`、`memory_reconsolidation.go`、`memory_agent.go` |
| `internal/agent` | `memory_tools.go`（Agent Memory Tool 装配） |
| `internal/data/memory_shim_*.go` | temporal / links / evolution / consolidation 扩展（原计划路径 `sessionmemory` 已折叠） |
| `internal/cronrunner/jobs` | `memory_consolidation_worker.go`、`memory_bio_decay_worker.go` |
| `internal/runtime` | `memory_set.go` 扩展（增加 MemoryAgent 等） |

### 1.2 主从关系

- **从** `memory-development.md`：本计划是 memory 总开发计划的增量，不替代
- **依赖** memory Phase 1-3 已落地的基础设施（L3 facts、L4 graph、MemoryWorker、Cascade）
- **独立开关**：`neural_memory_enabled` 默认 false，不影响现有功能

### 1.3 全局代码锚点

| 能力 | 现有锚点 | 扩展方式 |
|------|----------|----------|
| 提取管道 | `internal/cronrunner/jobs/auto_memory.go` | extract() 末尾增加 temporal + volatility + links |
| L3 存储 | `internal/data/memory_shim_l3.go` / recall adapters | ALTER TABLE + 新查询方法 |
| L4 存储 | `internal/data/memory_l4.go` | ALTER TABLE + 双时态查询 |
| L3 召回 | `internal/biz/memory_l3_fused_recall.go` | 时间过滤 + 再巩固 |
| L4 召回 | `internal/biz/memory_l4_usecase.go` | valid_until 过滤 |
| 衰减 | `internal/cronrunner/jobs/memory_l3_decay.go` | 替换为 BioDecayWorker |
| 级联 | `internal/biz/memory_l4_cascade.go` | 复用 CascadeProposal |
| MemorySet | `internal/runtime/memory_set.go` | 增加 MemoryAgent |
| Agent 装配 | `internal/agent/trpc_build.go` | 条件装配 Memory Tools |

---

## 2. 全局现状

| 项 | 状态 | 说明 |
|----|------|------|
| L3 facts 基础 CRUD | ✅ | 已落地 |
| L4 实体/关系基础 | ✅ | 已落地 |
| MemoryWorker LLM 提取 | ✅ MVP | ChainConsolidator |
| CascadeProposal 审批 | ✅ | BFS + 审批闭环 |
| L3 向量双写 | ✅ | SQLite + pgvector |
| L3 语义时间线 | ❌ | 无 `semantic_time_start/end` 字段 |
| L3 关联链接 | ❌ | 无 `memory_fact_links` 表 |
| L3 波动性分类 | ❌ | 无 `volatility` 字段 |
| L4 双时态 | ❌ | 无 `valid_from/valid_until` 字段 |
| 联动更新引擎 | ❌ | 无 |
| 睡眠期巩固 | ❌ | 无 |
| 干扰性遗忘 | ❌ | 固定衰减因子 |
| 印迹成熟 | ❌ | 召回不刷新 importance |
| 提取再巩固 | ❌ | 召回是只读操作 |
| Memory-Agent 后台守护 | ❌ | 无统一调度 |
| Agent 自主记忆编辑 | ❌ | 无 Memory Tool |
| 标准化评测 | ❌ | 无 benchmark |
| evolution_log 审计 | ❌ | 无 |

---

## 3. 差距与优先级

| 优先级 | 项 | 需求编号 | 说明 |
|--------|-----|----------|------|
| **P0** | L3 语义时间线 | F-01 | 记忆"何时有效"的基础 |
| **P0** | L3 波动性分类 | F-02 | 动态衰减的前提 |
| **P0** | L4 双时态 | F-03 | 图谱时间感知的基础 |
| **P0** | L3 关联链接 | F-04 | 联动更新的前提 |
| **P0** | 标准化评测 | F-14 | 量化验证 |
| **P1** | 联动更新引擎 | F-05 | 核心业务能力 |
| **P1** | L3-L4 联合推理 | F-06 | 跨层推理 |
| **P1** | 睡眠期巩固 | F-07 | 记忆整理 |
| **P1** | 干扰性遗忘 | F-08 | 替代固定衰减 |
| **P1** | 印迹成熟 | F-09 | 重要记忆保护 |
| **P1** | 提取再巩固 | F-10 | 召回时检查 |
| **P2** | Memory-Agent 后台守护 | F-11 | 统一调度 |
| **P2** | Agent 自主记忆编辑 | F-12 | Agent 能力增强 |
| **P2** | 提取质量优化 | F-13 | 可选增强 |

---

## 4. 开发阶段

### Phase 1：时间感知 + 关联链接 — ❌

**目标**：让记忆"知道何时有效"，让 fact 之间"相互关联"。

| # | 任务 | 层 | 依赖 | 状态 |
|---|------|-----|------|------|
| NM-01 | `memory_facts` ALTER TABLE 新增 temporal/volatility/access 字段 | data | — | ❌ |
| NM-02 | `memory_relations` ALTER TABLE 新增双时态字段 | data | — | ❌ |
| NM-03 | `memory_fact_links` 建表 | data | — | ❌ |
| NM-04 | `memory_evolution_log` 建表 | data | — | ❌ |
| NM-05 | `VolatilityClassifier` 规则版本实现 | biz | NM-01 | ❌ |
| NM-06 | `TemporalExtractor` LLM 提取实现 | biz | NM-01 | ❌ |
| NM-07 | `FactLinkGenerator` 实现语义关联生成 | biz | NM-03 | ❌ |
| NM-08 | auto_memory.go extract() 集成 temporal + volatility + links | cron | NM-05,06,07 | ❌ |
| NM-09 | L4 WriteFromUserText 集成双时态 + 冲突自动失效 | biz | NM-02 | ❌ |
| NM-10 | L3 RecallFactsFused 增加时间过滤 | biz | NM-01 | ❌ |
| NM-11 | L4 GraphRecall 增加 valid_until 过滤 | biz | NM-02 | ❌ |
| NM-12 | Proto 新增 `ListFactLinks`/`CreateFactLink`/`DeleteFactLink` RPC | api+service | NM-03 | ❌ |
| NM-13 | Proto 扩展 `ListMemoryFacts`/`UpsertMemoryFact` 响应字段 | api+service | NM-01 | ❌ |
| NM-14 | 前端 Memory Center fact 详情展示时间线+波动性+关联 | web | NM-12,13 | ❌ |
| NM-15 | 接入 LongMemEval 评测 | test | NM-08,10 | ❌ |

**验收标准**：
- [ ] 用户说"我上周换了工作"后，旧工作 fact 的 `semantic_time_end` 被设置，新工作 fact 的 `semantic_time_start` 被设置
- [ ] L4 关系冲突时旧 relation 的 `valid_until` 自动设置
- [ ] 新 fact 写入时自动生成关联链接
- [ ] 静态事实（出生地等）的 `volatility = 'static'`，不衰减
- [ ] LongMemEval temporal reasoning 维度得分 > 基线 10pp

### Phase 2：联动更新 + 仿生生命周期 — ❌

**目标**：实现"记忆块联动更新"和"仿生记忆生命周期"。

| # | 任务 | 层 | 依赖 | 状态 |
|---|------|-----|------|------|
| NM-16 | `LinkedUpdateEngine` 实现 | biz | NM-03,04 | ❌ |
| NM-17 | 联动更新风险评估 (risk_level) | biz | NM-16 | ❌ |
| NM-18 | 联动更新与 CascadeProposal 集成 | biz | NM-16, 现有 Cascade | ❌ |
| NM-19 | L3-L4 联合推理 (CompositeRecall 扩展) | biz | NM-10,11 | ❌ |
| NM-20 | `memory_consolidation_state` 建表 | data | — | ❌ |
| NM-21 | `ConsolidationEngine` 去重合并实现 | biz | NM-20 | ❌ |
| NM-22 | `ConsolidationEngine` 持续性记忆构建 | biz | NM-20, NM-01 | ❌ |
| NM-23 | `ConsolidationEngine` Episode→Fact 二次提炼 | biz | NM-20 | ❌ |
| NM-24 | `ConsolidationEngine` 实体消歧 | biz | NM-20 | ❌ |
| NM-25 | `ConsolidationEngine` 链接补全 | biz | NM-20, NM-03 | ❌ |
| NM-26 | `MemoryConsolidationWorker` Cron 实现 | cron | NM-21~25 | ❌ |
| NM-27 | `BioDecayEngine` 实现 (替换固定衰减) | biz | NM-01 | ❌ |
| NM-28 | `BioDecayWorker` Cron 实现 (替换 L3/L4 DecayWorker) | cron | NM-27 | ❌ |
| NM-29 | 印迹成熟: 召回时 access_count++ + importance 微增 | biz | NM-01 | ❌ |
| NM-30 | `ReconsolidationEngine` 实现 | biz | NM-01 | ❌ |
| NM-31 | 再巩固集成到 L3 RecallFactsFused | biz | NM-30 | ❌ |
| NM-32 | Proto 新增 `GetLinkedUpdatePreview`/`ListEvolutionLog`/`GetConsolidationStatus`/`TriggerConsolidation` | api+service | NM-16,21,30 | ❌ |
| NM-33 | 前端 Memory Center 联动更新预览 + 巩固状态 + 审计日志 | web | NM-32 | ❌ |

**验收标准**：
- [ ] 工作地点变更后，与旧地点关联的通勤/餐厅/天气记忆被自动评估和更新
- [ ] 低风险联动更新自动执行，高风险更新生成 CascadeProposal
- [ ] 联动更新链路可在 evolution_log 中完整追溯
- [ ] 联动更新最大递归深度 3，不产生无限循环
- [ ] 巩固后 fact 存储量缩减 ≥ 30%，保留精度 ≥ 95%
- [ ] 被取代的旧事实衰减速度明显快于未被取代的事实
- [ ] 频繁被召回的 fact 的 importance 高于同等初始值但从未被召回的 fact
- [ ] 超过 30 天未验证的高波动性事实被标记为 pending_validation

### Phase 3：Memory-Agent + 自主编辑 — ❌

**目标**：统一记忆管理入口，Agent 可自主编辑记忆。

| # | 任务 | 层 | 依赖 | 状态 |
|---|------|-----|------|------|
| NM-34 | `MemoryAgent` 后台守护实现 (统一调度) | biz | NM-26,28,30 | ❌ |
| NM-35 | `MemoryAgentConfig` 配置管理 | biz | NM-34 | ❌ |
| NM-36 | 睡眠巩固调度 (多租户隔离 + 超时保护) | biz | NM-34,26 | ❌ |
| NM-37 | 质量审计 (抽样评估 + 审计报告) | biz | NM-34 | ❌ |
| NM-38 | Agent Memory Tool 定义与注册 | agent | — | ❌ |
| NM-39 | Agent Memory Tool 写入安全 (PII + 冲突 + 置信度) | biz+agent | NM-38 | ❌ |
| NM-40 | `memory_fact_add` Tool 实现 | agent | NM-38,39 | ❌ |
| NM-41 | `memory_fact_update` Tool 实现 | agent | NM-38,39 | ❌ |
| NM-42 | `memory_fact_search` Tool 实现 | agent | NM-38 | ❌ |
| NM-43 | `memory_entity_link` Tool 实现 | agent | NM-38 | ❌ |
| NM-44 | 提取质量优化: 对话摘要上下文 | biz | — | ❌ |
| NM-45 | 提取质量优化: 自定义提取 prompt 模板 | biz | — | ❌ |
| NM-46 | Proto 新增 `GetMemoryAgentStatus`/`UpdateMemoryAgentConfig`/`AgentMemoryFact*` | api+service | NM-34,38 | ❌ |
| NM-47 | 前端 Memory Center Agent 配置面板 + 审计报告 | web | NM-46 | ❌ |
| NM-48 | 接入 LoCoMo 评测 | test | NM-34 | ❌ |

**验收标准**：
- [ ] Memory-Agent 可通过配置开关启用/禁用
- [ ] 巩固任务按 tenant 隔离，互不影响
- [ ] 单次巩固超时后保存进度退出
- [ ] 质量审计报告可在 Memory Center 查看
- [ ] Agent 可通过 Tool 主动添加 fact，fact 出现在 Memory Center
- [ ] Agent 写入的低置信度 fact 被标记为"候选"
- [ ] Agent 写入的 PII 内容被自动脱敏
- [ ] LoCoMo 综合得分 > 85

---

## 5. 跨层任务清单

| # | 任务 | 层 | 依赖 | 状态 |
|---|------|-----|------|------|
| NM-01 | L3 fact 新增 temporal/volatility/access 字段 | data | — | ❌ |
| NM-02 | L4 relation 新增双时态字段 | data | — | ❌ |
| NM-03 | memory_fact_links 建表 | data | — | ❌ |
| NM-04 | memory_evolution_log 建表 | data | — | ❌ |
| NM-05 | VolatilityClassifier 规则版本 | biz | NM-01 | ❌ |
| NM-06 | TemporalExtractor LLM 实现 | biz | NM-01 | ❌ |
| NM-07 | FactLinkGenerator 实现 | biz | NM-03 | ❌ |
| NM-08 | auto_memory.go 集成 | cron | NM-05,06,07 | ❌ |
| NM-09 | L4 双时态 + 冲突自动失效 | biz | NM-02 | ❌ |
| NM-10 | L3 Recall 时间过滤 | biz | NM-01 | ❌ |
| NM-11 | L4 Recall 时间过滤 | biz | NM-02 | ❌ |
| NM-12 | Proto ListFactLinks 等 | api+service | NM-03 | ❌ |
| NM-13 | Proto 扩展 ListMemoryFacts 等 | api+service | NM-01 | ❌ |
| NM-14 | 前端 fact 时间线+关联展示 | web | NM-12,13 | ❌ |
| NM-15 | LongMemEval 接入 | test | NM-08,10 | ❌ |
| NM-16 | LinkedUpdateEngine | biz | NM-03,04 | ❌ |
| NM-17 | 联动更新风险评估 | biz | NM-16 | ❌ |
| NM-18 | 联动更新 + CascadeProposal 集成 | biz | NM-16 | ❌ |
| NM-19 | L3-L4 联合推理 | biz | NM-10,11 | ❌ |
| NM-20 | memory_consolidation_state 建表 | data | — | ❌ |
| NM-21 | ConsolidationEngine 去重 | biz | NM-20 | ❌ |
| NM-22 | ConsolidationEngine 持续性记忆 | biz | NM-20 | ❌ |
| NM-23 | ConsolidationEngine Episode→Fact | biz | NM-20 | ❌ |
| NM-24 | ConsolidationEngine 实体消歧 | biz | NM-20 | ❌ |
| NM-25 | ConsolidationEngine 链接补全 | biz | NM-20 | ❌ |
| NM-26 | MemoryConsolidationWorker | cron | NM-21~25 | ❌ |
| NM-27 | BioDecayEngine | biz | NM-01 | ❌ |
| NM-28 | BioDecayWorker | cron | NM-27 | ❌ |
| NM-29 | 印迹成熟 | biz | NM-01 | ❌ |
| NM-30 | ReconsolidationEngine | biz | NM-01 | ❌ |
| NM-31 | 再巩固集成 L3 Recall | biz | NM-30 | ❌ |
| NM-32 | Proto 联动/巩固/审计 RPC | api+service | NM-16,21,30 | ❌ |
| NM-33 | 前端联动预览+巩固+审计 | web | NM-32 | ❌ |
| NM-34 | MemoryAgent 后台守护 | biz | NM-26,28,30 | ❌ |
| NM-35 | MemoryAgentConfig | biz | NM-34 | ❌ |
| NM-36 | 睡眠巩固调度 | biz | NM-34,26 | ❌ |
| NM-37 | 质量审计 | biz | NM-34 | ❌ |
| NM-38 | Agent Memory Tool 定义 | agent | — | ❌ |
| NM-39 | Memory Tool 写入安全 | biz+agent | NM-38 | ❌ |
| NM-40 | memory_fact_add Tool | agent | NM-38,39 | ❌ |
| NM-41 | memory_fact_update Tool | agent | NM-38,39 | ❌ |
| NM-42 | memory_fact_search Tool | agent | NM-38 | ❌ |
| NM-43 | memory_entity_link Tool | agent | NM-38 | ❌ |
| NM-44 | 提取质量: 对话摘要上下文 | biz | — | ❌ |
| NM-45 | 提取质量: 自定义 prompt 模板 | biz | — | ❌ |
| NM-46 | Proto MemoryAgent + AgentTool RPC | api+service | NM-34,38 | ❌ |
| NM-47 | 前端 Agent 配置 + 审计报告 | web | NM-46 | ❌ |
| NM-48 | LoCoMo 接入 | test | NM-34 | ❌ |

---

## 6. 依赖与风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| 联动更新 LLM 调用成本 | 每次联动评估需 LLM 判断，token 消耗增加 | 低风险用规则引擎；仅高风险调 LLM；单次联动 LLM 调用上限 20 |
| 巩固与在线请求 DB 竞争 | 巩固可能占用 SQLite 写锁 | 低峰期执行；超时保护 5min；增量处理；只读分析不持锁 |
| 时间提取不准确 | LLM 可能错误解析模糊时间 | 保留 dialogue_time 兜底；temporal_confidence 字段；低置信度不设时间 |
| 联动更新误判 | LLM 可能错误判断关联 fact 需更新 | 高风险需人工审批；所有变更可回滚（evolution_log 有 before/after snapshot） |
| 递归联动爆炸 | A→B→C→D... 无限链式 | 最大深度 3；单次联动 LLM 调用上限 20；深度越大风险阈值越高 |
| ALTER TABLE 兼容性 | SQLite ALTER TABLE 限制 | 新增列均为 NOT NULL DEFAULT；不修改现有列类型 |
| BioDecayWorker 替换风险 | 替换现有 DecayWorker 可能影响现有衰减行为 | `neural_memory_enabled` 开关控制；关闭时走原 DecayWorker |

---

## 7. 与 memory-development.md 的关系

| 维度 | memory-development.md | 本文档 |
|------|----------------------|--------|
| 范围 | L0-L4 基础设施 + MemoryWorker + Cascade | 神经记忆系统增量能力 |
| 状态 | 🟢 大部分已落地 | ❌ 未启动 |
| 依赖 | 无 | 依赖 memory Phase 1-3 已落地 |
| 开关 | 无（默认启用） | `neural_memory_enabled`（默认关闭） |
| 任务编号 | T1-T18 | NM-01 至 NM-48 |

---

## 8. 验收标准（模块级）

### Phase 1 验收

- [ ] 用户说"我上周换了工作"后，旧工作 fact 被标记为已结束
- [ ] L4 关系冲突时旧 relation 自动失效
- [ ] 新 fact 写入时自动生成关联链接
- [ ] 静态事实不衰减
- [ ] LongMemEval temporal reasoning > 基线 10pp

### Phase 2 验收

- [ ] 工作地点变更后，关联记忆被自动评估和更新
- [ ] 联动更新链路可追溯
- [ ] 巩固后存储缩减 ≥ 30%，保留精度 ≥ 95%
- [ ] 被取代的旧事实衰减更快
- [ ] 频繁召回的 fact importance 更高

### Phase 3 验收

- [ ] Memory-Agent 可配置开关
- [ ] 巩固按 tenant 隔离
- [ ] Agent 可通过 Tool 主动添加/更新 fact
- [ ] LoCoMo 综合得分 > 85
