# taxonomy-data-unify Specification

## Purpose
TBD - created by archiving change phase1-taxonomy-review-fixes. Update Purpose after archive.
## Requirements
### Requirement: taxonomy.yaml 丰富字段补全
taxonomy.yaml SHALL 包含设计规范 §5.1 要求的丰富字段（responsibilities/skills_required/seniority_level/variants）。

#### Scenario: 部门级 responsibilities 字段
- **WHEN** 查看 `internal/scenario/taxonomy.yaml` 中的部门定义
- **THEN** 每个部门 SHALL 包含 `responsibilities`（字符串数组）字段

#### Scenario: 岗位级丰富字段
- **WHEN** 查看 `internal/scenario/taxonomy.yaml` 中的岗位定义
- **THEN** 每个岗位 SHALL 包含 `seniority_level`、`skills_required`（字符串数组）、`responsibilities`（字符串数组）、`variants`（对象数组，含 key 和 name）字段

#### Scenario: variant key 命名规范
- **WHEN** 查看 taxonomy.yaml 中的 variant 定义
- **THEN** 每个 variant key SHALL 匹配 `^[a-z0-9_]+$`（仅小写字母、数字、下划线）

### Requirement: selfmedia 部门 key 对齐
selfmedia 行业的部门 key SHALL 按设计规范 §5.3 对齐。

#### Scenario: content_creation 拆分
- **WHEN** 查看 taxonomy.yaml 中 selfmedia 行业
- **THEN** `content_creation` 部门 SHALL 拆分为 `fiction_writing`（小说创作部）和 `content_graphic`（内容图文部）

#### Scenario: growth_monetization 重命名
- **WHEN** 查看 taxonomy.yaml 中 selfmedia 行业
- **THEN** `growth_monetization` SHALL 变更为 `distribution`（分发运营部）

### Requirement: finance/agents.yaml 缺失 variant 和 team 补全
finance/agents.yaml SHALL 补全设计规范 §6 要求的 variant 和 team。

#### Scenario: trading_coordinator/critic variant
- **WHEN** 查看 `internal/scenario/finance/agents.yaml`
- **THEN** trading_coordinator 岗位 SHALL 包含 `critic` variant（agent_key: `trading-coordinator-critic`）

#### Scenario: report_writer/chart variant
- **WHEN** 查看 `internal/scenario/finance/agents.yaml`
- **THEN** report_writer 岗位 SHALL 包含 `chart` variant（agent_key: `report-writer-chart`）

#### Scenario: team-research-pipeline 新增
- **WHEN** 查看 `internal/scenario/finance/agents.yaml`
- **THEN** SHALL 存在 `team-research-pipeline` 团队定义，成员引用 SHALL 使用 finance agent_key

#### Scenario: team-deep-dive-critic 新增
- **WHEN** 查看 `internal/scenario/finance/agents.yaml`
- **THEN** SHALL 存在 `team-deep-dive-critic` 团队定义，成员引用 SHALL 使用 finance agent_key

### Requirement: softwaredev/agents.yaml P1 Agent 补全
softwaredev/agents.yaml SHALL 补全 P1 批次 Agent 定义（backend/frontend/gamedev 岗位，共 ~27 个 Agent）。

#### Scenario: backend 岗位 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** backend 部门 SHALL 包含 Java 高级工程师(3 variant)、Python 高级工程师(2 variant)、Rust 工程师(2 variant)、C++ 后端工程师(2 variant)、DBA(2 variant) 的 Agent 定义

#### Scenario: frontend 岗位 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** frontend 部门 SHALL 包含 React 高级前端(3 variant)、TypeScript 专家(2 variant)、前端性能(2 variant)、UI/UX 还原(1 variant) 的 Agent 定义

#### Scenario: gamedev 岗位 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** gamedev 部门 SHALL 包含 UE 游戏逻辑(2 variant)、UE 图形渲染(2 variant)、游戏服务端(2 variant)、TA(1 variant)、策划(1 variant) 的 Agent 定义

#### Scenario: Agent 定义完整性
- **WHEN** 查看任意新增 Agent 定义
- **THEN** 该 Agent SHALL 包含 `position_key`、`agent_variant`、`model`（含 fast_model 和 strong_model）、`tools_profile`、`skills` 字段

### Requirement: softwaredev/agents.yaml P2+P3 Agent 补全
softwaredev/agents.yaml SHALL 补全 P2 和 P3 批次 Agent 定义（共 ~45 个 Agent）。

#### Scenario: P2 批次 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** devops(10)、architecture(7)、qa(6) 部门 SHALL 包含对应的 Agent 定义

#### Scenario: P3 批次 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** mobiledev(9)、dataeng(5)、security(4)、productpm(4) 部门 SHALL 包含对应的 Agent 定义

### Requirement: 全量编译验证
所有修改完成后 SHALL 通过全量编译和测试验证。

#### Scenario: 后端编译验证
- **WHEN** 运行 `make api && make wire && make build && make test`
- **THEN** 所有命令 SHALL 成功通过

#### Scenario: 前端编译验证
- **WHEN** 运行 `cd web && pnpm lint && pnpm build`
- **THEN** 所有命令 SHALL 成功通过

