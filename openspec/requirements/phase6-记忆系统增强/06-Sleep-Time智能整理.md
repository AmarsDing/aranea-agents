# 06 — Sleep-Time 智能整理

> **借鉴来源**：Letta（Sleep-Time Agent）
> **优先级**：中
> **影响层**：L2 情景记忆、L3 语义记忆、L4 知识图谱
> **最后更新**：2026-06-06

---

## 一、需求文档

### 1.1 背景

当前记忆整理通过 Cron Job 驱动（衰减、归档、对账），是规则驱动的：
- L2 衰减：按时间衰减因子降低权重
- L3 衰减：按 decay_factor 降低 importance/confidence
- L4 衰减：按实体类型半衰期降低置信度
- L1 归档：将 idle 的 task 标记为 archived

这些规则无法处理需要"理解"才能完成的整理任务：
- 识别重复/矛盾的 Episode 并整合
- 从多个 Episode 中提取新的 Fact
- 发现实体间的新关系
- 清理低质量记忆

### 1.2 Letta 的做法

Letta 引入 **Sleep-Time Agent**：一个独立的后台 Agent，在用户不活跃时运行，负责：
1. 回顾最近的对话，提取遗漏的记忆
2. 整合和压缩已有记忆
3. 更新 Core Memory Blocks
4. 为下次对话准备上下文

### 1.3 目标

1. 新增 Sleep-Time Worker：LLM 驱动的后台记忆整理
2. 与现有 Cron Job 协同：Cron 负责规则驱动的衰减，Sleep-Time 负责理解驱动的整理
3. Sleep-Time 可配置整理策略（整合/提取/清理/关系发现）

### 1.4 功能需求

#### P0 — 必须实现

| ID | 需求 | 说明 |
|----|------|------|
| ST-P0-1 | Sleep-Time Worker 接口 | 定义 SleepTimeWorker 接口和调度逻辑 |
| ST-P0-2 | Episode 整合 | LLM 判断重复/相关 Episode 并整合 |
| ST-P0-3 | Fact 提取 | 从未处理的 Episode 中提取新 Fact |
| ST-P0-4 | 整理结果 provenance | 所有整理操作写入 action_log |

#### P1 — 应该实现

| ID | 需求 | 说明 |
|----|------|------|
| ST-P1-1 | 关系发现 | 从 Fact 中推断实体间新关系 |
| ST-P1-2 | 低质量记忆清理 | LLM 判断无价值记忆并标记删除 |
| ST-P1-3 | 整理策略可配置 | MemoryRuntimePolicy 可配置 Sleep-Time 行为 |
| ST-P1-4 | 整理频率控制 | 限制 LLM 调用次数，控制成本 |

#### P2 — 可以实现

| ID | 需求 | 说明 |
|----|------|------|
| ST-P2-1 | 整理报告 | Sleep-Time 完成后生成整理摘要 |
| ST-P2-2 | 用户确认模式 | 高风险整理操作需用户确认 |

### 1.5 验收标准

1. Sleep-Time Worker 可在用户不活跃时自动运行
2. 重复 Episode 被正确整合
3. 新 Fact 从 Episode 中被正确提取
4. 整理操作有完整的 action_log 记录
5. LLM 调用次数在预算内（可配置）
6. `make wire && make build && make test` 全部通过

---

## 二、设计文档

### 2.1 核心接口

```go
// SleepTimeWorker 后台智能整理 Worker
type SleepTimeWorker interface {
    // Run 执行一次整理
    Run(ctx context.Context, agentID string) (*SleepTimeResult, error)
}

type SleepTimeResult struct {
    EpisodesConsolidated int      // 整合的 Episode 数
    FactsExtracted       int      // 提取的新 Fact 数
    RelationsDiscovered  int      // 发现的新关系数
    MemoriesCleaned      int      // 清理的记忆数
    LLMCallCount         int      // LLM 调用次数
    ActionLogIDs         []string // 操作日志 ID
}
```

### 2.2 与 Cron Job 的协同

| 整理类型 | 驱动方式 | 说明 |
|---------|---------|------|
| 衰减/归档 | Cron Job（规则驱动） | 确定性操作，无需 LLM |
| Episode 整合 | Sleep-Time（LLM 驱动） | 需要理解才能判断重复/相关 |
| Fact 提取 | Sleep-Time（LLM 驱动） | 需要理解才能提取新事实 |
| 关系发现 | Sleep-Time（LLM 驱动） | 需要推理才能发现新关系 |
| 索引对账 | Cron Job（规则驱动） | 确定性同步操作 |

### 2.3 实现位置

| 组件 | 文件路径 |
|------|----------|
| SleepTimeWorker 接口 | `internal/biz/memory_sleeptime.go` |
| Sleep-Time Cron Job | `internal/cronrunner/jobs/memory_sleeptime.go` |
| Episode 整合 prompt | 新增 |
| Fact 提取 prompt | 复用 Extractor |

---

## 三、开发计划

| # | 任务 | 涉及文件 | 优先级 |
|---|------|----------|--------|
| T1 | SleepTimeWorker 接口与实现 | `internal/biz/memory_sleeptime.go` | P0 |
| T2 | Episode 整合逻辑 | `internal/biz/memory_sleeptime.go` | P0 |
| T3 | Fact 提取逻辑 | `internal/biz/memory_sleeptime.go` | P0 |
| T4 | Cron Job 集成 | `internal/cronrunner/jobs/memory_sleeptime.go` | P0 |
| T5 | action_log 集成 | `internal/data/memory_shim_action_log.go` | P0 |
| T6 | 关系发现 | `internal/biz/memory_sleeptime.go` | P1 |
| T7 | 低质量记忆清理 | `internal/biz/memory_sleeptime.go` | P1 |
| T8 | RuntimePolicy 配置 | `internal/biz/agent_memory_runtime_policy.go` | P1 |
| T9 | 集成测试 | `internal/biz/memory_sleeptime_test.go` | P0 |
| T10 | 全量构建验证 | 全局 | P0 |
