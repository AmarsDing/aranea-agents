## Context

当前 20 个 OpenSpec 归档变更中，5 大冲突集群存在类型分裂、接口重叠和表结构假设不一致问题。已完成变更（spirit-orchestration-redesign、system-builtin-agents、skill-butler-tools、session-status-monitoring 等）建立了各自独立的类型定义和工具体系，未实施变更（memory-skills-butler、skill-intelligence、monitor-self-healing 等）若基于旧假设实施将导致运行时兼容性问题。

关键现状：
- `spirit-orchestration-redesign` 已实施 3 个编排工具（plan_and_execute / check_progress / cancel_orchestration）
- `system-builtin-agents` 已实施 8 个编排管家工具（classify_industry / assemble_team 等），两套工具并存
- `skill-butler-tools` 已实施 4 个技能管家工具，但类型定义散落在 tools 包内
- `data-architecture-overhaul` 三表拆分已完成，4 项延后任务涉及 DTO 解耦
- `team-graph-optimization` 仅 M1 完成，M2~M5 依赖统一的 CompiledTeam 架构

## Goals / Non-Goals

**Goals:**
- 统一管家工具体系，消除 system-builtin-agents 和 spirit-orchestration-redesign 的工具粒度冲突
- 收敛 Skill 相关共享类型到 biz 层单一定义源，消除类型分裂
- 统一跨模块事件契约，集中管理 EnvelopeType 注册
- 统一监控条件扩展策略，合并 RootCauseCondition 扩展方式
- 确认 Session 三表拆分为最终 Schema，补全延后任务
- 补全 team-graph-optimization M2~M5

**Non-Goals:**
- 不重新设计已完成变更的核心架构
- 不修改 trpc-agent-go 框架层
- 不修改 Proto API 对外接口
- 不实施纯前端独立变更（learning-loop-frontend、chat-sidebar-pinned-collapse-drag）
- 不实施 modelregistry-refactor（独立模块，无跨模块冲突）

## Decisions

### Decision 1: 管家工具体系采用"粗粒度入口 + 细粒度内部实现"模式

**选择**：以 `spirit-orchestration-redesign` 的 3 个粗粒度工具为对外接口，`system-builtin-agents` 的 8 个细粒度工具降级为内部实现函数。

**理由**：
- 3 个粗粒度工具已实施且通过验证，对外接口稳定
- 8 个细粒度工具的逻辑仍需保留，但作为 plan_and_execute 的内部编排步骤
- Agent 只需调用 plan_and_execute 即可完成全流程，降低使用复杂度

**替代方案**：
- 保留 8 个细粒度工具对外暴露 → Agent 需要理解复杂的多步调用序列，易出错
- 合并为 5 个中等粒度工具 → 粒度划分无清晰边界，维护成本高

**映射关系**：
```
plan_and_execute (对外)
  ├── classify_industry    (内部步骤)
  ├── search_positions     (内部步骤)
  ├── find_agents          (内部步骤)
  ├── instantiate_agent    (内部步骤)
  ├── estimate_task        (内部步骤)
  └── assemble_team        (内部步骤)

check_progress (对外)
  └── query_agent_status   (内部步骤)

cancel_orchestration (对外)
  └── report_task_result   (内部步骤，取消场景)
```

### Decision 2: 共享类型收敛到 `internal/biz/types/` 包

**选择**：新建 `internal/biz/types/` 包，按领域分文件收敛共享类型。

**文件布局**：
```
internal/biz/types/
├── skill_health.go        # SkillHealth, ToolWeightReport, ExperienceReport
├── monitor_condition.go   # RootCauseCondition 扩展类型, HealRecord, SelfCheckResult
├── butler_types.go        # ButlerTier, ButlerCapability, OrchestrationStep
└── session_types.go       # SessionStatus 枚举, StatusReason, SessionTreeNode
```

**理由**：
- biz 层是依赖方向的中心，data/service 层均可 import biz
- 避免循环依赖（tools 包不能被 biz import，但 biz/types 可以被 tools import）
- 单一真相源，消除同名不同义的类型分裂

**替代方案**：
- 放在 `internal/types/` 顶层包 → 违反分层规范，biz 层应有自己的类型定义权
- 各模块各自定义 + 类型转换函数 → 运行时断言风险，维护成本高

### Decision 3: 事件契约集中注册 + 分层命名

**选择**：所有跨模块 EnvelopeType 在 `internal/event/envelope.go` 集中注册，命名采用 `{domain}_{action}` 格式。

**新增事件类型**：
```
// 管家体系
EnvelopeTypeButlerOrchestrationStarted
EnvelopeTypeButlerOrchestrationCompleted
EnvelopeTypeButlerOrchestrationFailed

// Skill 进化
EnvelopeTypeSkillHealthChanged
EnvelopeTypeSkillEvolutionProposed

// 监控自愈
EnvelopeTypeMonitorAutoHealed
EnvelopeTypeMonitorSelfCheckCompleted
```

**理由**：
- 集中注册避免事件类型冲突（两个变更定义相同数值的常量）
- 分层命名便于事件路由和过滤
- 事件消费方通过 import event 包获取类型，无需私自定义

### Decision 4: RootCauseCondition 采用 proto oneof 扩展

**选择**：`RootCauseCondition` 的扩展维度通过 proto oneof 实现，各监控变更各自添加自己的 condition 类型。

```protobuf
message RootCauseCondition {
  oneof condition {
    AutoHealedCondition auto_healed = 10;
    HealAttemptsCondition heal_attempts = 11;
    SelfCheckStatusCondition self_check_status = 12;
  }
}
```

**理由**：
- oneof 天然互斥，避免条件冲突
- 各变更独立添加自己的 condition 类型，无需修改已有代码
- proto 生成代码自动处理序列化

**替代方案**：
- 各自扩展 proto message 字段 → 字段膨胀，语义不清
- 用 map<string, string> 传递 → 无类型安全，解析易出错

### Decision 5: Session 三表拆分为最终 Schema，延后任务补全策略

**选择**：确认 sessions / session_metrics / session_runtime 三表结构为最终 Schema，补全 4 项延后任务中的 2 项关键项（Task 2.7 DTO 解耦、Task 4.4 patch 迁移），其余 2 项继续延后。

**补全项**：
- Task 2.7：SessionMetricsDTO 解耦 — 将 `toProtoSession` 中的 metrics 字段映射改为通过 SessionMetricsRepo 独立查询
- Task 4.4：SessionRuntime patch 迁移 — 将 `UpdateSession` 中的 runtime 字段更新改为通过 SessionRuntimeRepo 独立更新

**继续延后项**：
- Task 2.8：SessionMetrics 批量查询 DTO — 当前无性能瓶颈，延后合理
- Task 8.3/8.4：Store 独立化 shim — 系统已正常工作，shim 阶段改方法参数风险高

### Decision 6: team-graph-optimization M2~M5 基于统一架构实施

**选择**：M2~M5 基于 data-architecture-overhaul 的最终表结构和 spirit-orchestration-redesign 的三阶段架构实施。

**关键约束**：
- M3 CompiledTeam 持久化必须使用 sessions 三表结构
- M4 Graph Independence 必须与 TaskOrchestrator 的 DAGToGraphCompiler 协调
- M5 Team Lifecycle 必须与 SessionStatusMachine 的状态转换对齐

## Risks / Trade-offs

| 风险 | 影响 | 缓解措施 |
|------|------|----------|
| 管家工具收敛可能破坏已有 Agent 的工具调用习惯 | 中 | 保留 8 个细粒度工具的内部函数签名，plan_and_execute 内部按需调用，Agent 无感知 |
| biz/types 包可能成为"类型垃圾桶" | 中 | 严格限制只放跨模块共享类型，模块内部类型留在各自包内 |
| 三表拆分延后任务补全可能引入回归 | 低 | 补全项有明确测试用例，逐项验证 |
| team-graph-optimization M2~M5 实施量大 | 高 | 按 M2→M3→M4→M5 顺序渐进，每阶段独立验证 |
| 事件类型集中注册可能增加 envelope.go 的编译依赖 | 低 | 事件类型只有常量定义，无逻辑依赖 |

## Migration Plan

1. **Phase 0：基础设施**（类型收敛 + 事件注册）— 无破坏性变更
2. **Phase 1：管家体系统一**（工具收敛 + 内部实现迁移）— 需要调整 system_builtin_tools.go 注入路径
3. **Phase 2：Session 延后补全**（DTO 解耦 + patch 迁移）— 需要修改 toProtoSession 和 UpdateSession
4. **Phase 3：team-graph-optimization M2~M5**（渐进实施）— 需要修改 Graph/Team 编译流程
5. **Phase 4：监控体系统一**（RootCauseCondition 扩展）— 需要修改 proto 和 service 层

每个 Phase 独立可回滚，Phase 间无强依赖（Phase 3 依赖 Phase 2 的表结构确认）。

## Open Questions

1. `memory-skills-butler` 的 ExperienceAnalyticsUsecase 是否应与 `skill-intelligence` 的 SkillIntelligenceUsecase 合并为一个共享 Usecase？还是保持两个独立 Usecase 共享 types 包？
2. `modelregistry-refactor` 的 CronRunner 路由与 `system-builtin-agents` 的管家 CronRunner 是否需要统一调度？还是各自独立调度？
3. `aranea-pack-import-export` S6 种子迁移是否应在本变更中一并完成？还是作为独立后续变更？
