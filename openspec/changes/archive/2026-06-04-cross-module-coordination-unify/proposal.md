## Why

当前 20 个 OpenSpec 归档变更中，7 个全部未实施、2 个部分完成，且 5 大冲突集群（管家体系、Session/数据层、Skill 进化、监控、Taxonomy）存在类型定义分裂、接口重叠、表结构假设不一致等兼容性风险。若各模块独立升级，将导致：
- 同一概念（如编排管家、SkillHealth）存在多套类型定义，运行时类型断言失败
- sessions 表拆分后，未实施变更仍基于旧表结构假设，写入路径冲突
- 工具注入路径竞争（system_builtin_tools.go 被多个管家同时修改）
- EventBus 事件类型私自定义，跨模块事件无法正确路由

需要一次跨模块统筹协调，在实施前统一类型、接口和事件契约，确保各模块升级后兼容一致。

## What Changes

- **统一管家工具体系**：将 `system-builtin-agents` 的 8 个编排管家工具和 `spirit-orchestration-redesign` 的 3 个编排工具合并为统一的三层架构（plan_and_execute / check_progress / cancel_orchestration），8 个细粒度工具降级为内部实现细节
- **统一 Skill 进化类型**：将 `skill-butler-tools`（已完成）、`skill-intelligence`（未实施）、`memory-skills-butler`（未实施）中共享的 `SkillHealth`、`ToolWeightReport`、`ExperienceReport` 等类型收敛到 `internal/biz/types/skill_health.go` 单一定义源
- **统一 Session 表结构假设**：确认 `data-architecture-overhaul` 三表拆分（sessions / session_metrics / session_runtime）为最终 Schema，所有未实施变更基于此结构适配
- **统一事件契约**：在 `internal/event/envelope.go` 中集中注册所有跨模块事件类型，禁止各变更私自定义 EnvelopeType
- **统一监控扩展策略**：`monitor-self-healing` 和 `monitor-selfcheck-repair` 共享 `RootCauseCondition` 扩展方式，统一为 proto oneof 模式
- **补全 team-graph-optimization M2~M5**：基于统一后的 CompiledTeam 架构完成剩余实施
- **补全 aranea-pack-import-export S6~S8**：基于统一后的 taxonomy_key 完成种子迁移和旧代码清理

## Capabilities

### New Capabilities
- `butler-unified-architecture`: 管家体系统一架构——合并 system-builtin-agents 和 spirit-orchestration-redesign 的管家工具体系，定义统一的工具注入路径和层级关系
- `skill-type-registry`: Skill 类型注册中心——收敛 SkillHealth / ToolWeightReport / ExperienceReport 等共享类型到 biz 层单一定义源，各模块通过 import 引用
- `event-contract-registry`: 事件契约注册中心——集中管理跨模块 EnvelopeType，确保事件路由和消费的一致性
- `monitor-condition-unify`: 监控条件统一——合并 monitor-self-healing 和 monitor-selfcheck-repair 的 RootCauseCondition 扩展策略

### Modified Capabilities
- `task-orchestrator`: 适配 butler-unified-architecture 的工具体系变更，plan_and_execute 内部编排逻辑调整
- `spirit-tools`: 工具粒度从 8 个细粒度工具收敛为 3 个粗粒度工具的内部实现
- `session-repo-interfaces`: 确认三表拆分为最终 Schema，补充 session_metrics / session_runtime 的读写接口
- `team-graph-optimization`: 基于 data-architecture-overhaul 的最终表结构适配 CompiledTeam 持久化

## Impact

**后端 biz 层**：
- 新增 `internal/biz/types/` 包（skill_health.go / monitor_condition.go / butler_types.go）
- 修改 `internal/event/envelope.go`（新增 5+ 个 EnvelopeType 常量）
- 修改 `internal/biz/task_orchestrator.go`（工具编排逻辑调整）
- 修改 `internal/tools/spirit/` 目录（工具收敛）

**后端 data 层**：
- `internal/data/session_metrics_repo.go` / `session_runtime_repo.go`（确认三表 Schema）
- `internal/data/compiled_team_repo.go`（适配新表结构）

**后端 service 层**：
- `internal/service/monitor.go`（RootCauseCondition proto 扩展）

**前端**：
- `web/src/stores/agents/`（管家工具展示适配）
- `web/src/components/monitor/`（监控条件统一展示）

**依赖关系**：
- 依赖 `spirit-orchestration-redesign`（已完成）的三阶段架构为基线
- 依赖 `data-architecture-overhaul`（实质完成）的三表拆分为最终 Schema
- 依赖 `skill-butler-tools`（已完成）的类型定义为收敛起点
- 依赖 `phase1-taxonomy-rename-and-unify`（已完成）的 taxonomy_key 体系

## Non-goals

- 不重新设计已完成变更的核心架构（spirit-orchestration-redesign 的三阶段架构保持不变）
- 不修改 trpc-agent-go 框架层
- 不修改 Proto API 对外接口（保持向后兼容）
- 不实施 learning-loop-frontend 和 chat-sidebar-pinned-collapse-drag（纯前端独立变更，无跨模块冲突）
- 不实施 self-iteration-engine 的扩展（CI/CD 工具链独立，无跨模块冲突）
- 不实施 modelregistry-refactor（独立模块，与管家/Skill/Monitor 体系无直接冲突）
