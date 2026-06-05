## ADDED Requirements

### Requirement: P1 批次 — backend 岗位 Agent 补全
softwaredev 行业 backend 部门 SHALL 补全 11 个 Agent（含 variant），覆盖 Java/Python/Rust/C++/DBA 等核心岗位。

#### Scenario: Java 高级工程师 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** backend 部门 SHALL 包含 Java 高级工程师的 3 个 variant Agent（如 `java-senior-{variant}`），每个 Agent SHALL 有 `position_key`、`agent_variant`、`model`、`tools_profile`、`skills` 定义

#### Scenario: Python 高级工程师 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** backend 部门 SHALL 包含 Python 高级工程师的 2 个 variant Agent

#### Scenario: Rust 工程师 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** backend 部门 SHALL 包含 Rust 工程师的 2 个 variant Agent

#### Scenario: C++ 后端工程师 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** backend 部门 SHALL 包含 C++ 后端工程师的 2 个 variant Agent

#### Scenario: DBA Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** backend 部门 SHALL 包含 DBA 的 2 个 variant Agent

### Requirement: P1 批次 — frontend 岗位 Agent 补全
softwaredev 行业 frontend 部门 SHALL 补全 8 个 Agent（含 variant），覆盖 React/TypeScript/性能/UI-UX 等核心岗位。

#### Scenario: React 高级前端 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** frontend 部门 SHALL 包含 React 高级前端的 3 个 variant Agent

#### Scenario: TypeScript 专家 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** frontend 部门 SHALL 包含 TypeScript 专家的 2 个 variant Agent

#### Scenario: 前端性能 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** frontend 部门 SHALL 包含前端性能的 2 个 variant Agent

#### Scenario: UI/UX 还原 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** frontend 部门 SHALL 包含 UI/UX 还原的 1 个 variant Agent

### Requirement: P1 批次 — gamedev 岗位 Agent 补全
softwaredev 行业 gamedev 部门 SHALL 补全 8 个 Agent（含 variant），覆盖 UE 游戏逻辑/图形渲染/服务端/TA/策划等核心岗位。

#### Scenario: UE 游戏逻辑 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** gamedev 部门 SHALL 包含 UE 游戏逻辑的 2 个 variant Agent

#### Scenario: UE 图形渲染 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** gamedev 部门 SHALL 包含 UE 图形渲染的 2 个 variant Agent

#### Scenario: 游戏服务端 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** gamedev 部门 SHALL 包含游戏服务端的 2 个 variant Agent

#### Scenario: TA Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** gamedev 部门 SHALL 包含 TA（技术美术）的 1 个 variant Agent

#### Scenario: 策划 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** gamedev 部门 SHALL 包含策划的 1 个 variant Agent

### Requirement: P2 批次 — devops 岗位 Agent 补全
softwaredev 行业 devops 部门 SHALL 补全 10 个 Agent（含 variant），覆盖 SRE/CI-CD/容器编排/监控/基础设施等岗位。

#### Scenario: SRE Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** devops 部门 SHALL 包含 SRE 的 3 个 variant Agent

#### Scenario: CI/CD Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** devops 部门 SHALL 包含 CI/CD 的 2 个 variant Agent

#### Scenario: 容器编排 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** devops 部门 SHALL 包含容器编排的 2 个 variant Agent

#### Scenario: 监控 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** devops 部门 SHALL 包含监控的 2 个 variant Agent

#### Scenario: 基础设施 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** devops 部门 SHALL 包含基础设施的 1 个 variant Agent

### Requirement: P2 批次 — architecture 岗位 Agent 补全
softwaredev 行业 architecture 部门 SHALL 补全 7 个 Agent（含 variant），覆盖系统架构师/技术负责人/解决方案架构师等岗位。

#### Scenario: 系统架构师 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** architecture 部门 SHALL 包含系统架构师的 3 个 variant Agent

#### Scenario: 技术负责人 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** architecture 部门 SHALL 包含技术负责人的 2 个 variant Agent

#### Scenario: 解决方案架构师 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** architecture 部门 SHALL 包含解决方案架构师的 2 个 variant Agent

### Requirement: P2 批次 — qa 岗位 Agent 补全
softwaredev 行业 qa 部门 SHALL 补全 6 个 Agent（含 variant），覆盖测试工程师/自动化测试/性能测试等岗位。

#### Scenario: 测试工程师 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** qa 部门 SHALL 包含测试工程师的 3 个 variant Agent

#### Scenario: 自动化测试 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** qa 部门 SHALL 包含自动化测试的 2 个 variant Agent

#### Scenario: 性能测试 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** qa 部门 SHALL 包含性能测试的 1 个 variant Agent

### Requirement: P3 批次 — mobiledev 岗位 Agent 补全
softwaredev 行业 mobiledev 部门 SHALL 补全 9 个 Agent（含 variant），覆盖 iOS/Android/Flutter/RN 等岗位。

#### Scenario: iOS Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** mobiledev 部门 SHALL 包含 iOS 的 3 个 variant Agent

#### Scenario: Android Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** mobiledev 部门 SHALL 包含 Android 的 3 个 variant Agent

#### Scenario: Flutter Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** mobiledev 部门 SHALL 包含 Flutter 的 2 个 variant Agent

#### Scenario: React Native Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** mobiledev 部门 SHALL 包含 React Native 的 1 个 variant Agent

### Requirement: P3 批次 — dataeng 岗位 Agent 补全
softwaredev 行业 dataeng 部门 SHALL 补全 5 个 Agent（含 variant），覆盖数据工程师/数据平台等岗位。

#### Scenario: 数据工程师 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** dataeng 部门 SHALL 包含数据工程师的 3 个 variant Agent

#### Scenario: 数据平台 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** dataeng 部门 SHALL 包含数据平台的 2 个 variant Agent

### Requirement: P3 批次 — security 岗位 Agent 补全
softwaredev 行业 security 部门 SHALL 补全 4 个 Agent（含 variant），覆盖安全工程师/渗透测试/安全审计等岗位。

#### Scenario: 安全工程师 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** security 部门 SHALL 包含安全工程师的 2 个 variant Agent

#### Scenario: 渗透测试 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** security 部门 SHALL 包含渗透测试的 1 个 variant Agent

#### Scenario: 安全审计 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** security 部门 SHALL 包含安全审计的 1 个 variant Agent

### Requirement: P3 批次 — productpm 岗位 Agent 补全
softwaredev 行业 productpm 部门 SHALL 补全 4 个 Agent（含 variant），覆盖产品经理/项目经理等岗位。

#### Scenario: 产品经理 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** productpm 部门 SHALL 包含产品经理的 2 个 variant Agent

#### Scenario: 项目经理 Agent
- **WHEN** 查看 `internal/scenario/softwaredev/agents.yaml`
- **THEN** productpm 部门 SHALL 包含项目经理的 2 个 variant Agent

### Requirement: Agent 定义完整性
每个 Agent SHALL 在 `agents.yaml` 中包含完整的定义字段。

#### Scenario: Agent 必需字段
- **WHEN** 查看 `agents.yaml` 中的任意 Agent 定义
- **THEN** 该 Agent SHALL 包含 `position_key`（归属岗位 key）、`agent_variant`（变体标识）、`model`（模型配置，含 fast_model 和 strong_model）、`tools_profile`（工具集配置）、`skills`（技能引用列表）

#### Scenario: Agent variant 命名规范
- **WHEN** 查看 `agents.yaml` 中的 Agent variant
- **THEN** `agent_variant` SHALL 使用语义化命名（如 `general`/`factor`/`backtest`），同一岗位不同 variant 之间的职责边界 SHALL 清晰

#### Scenario: Agent key 格式
- **WHEN** 查看 `agents.yaml` 中的 Agent 定义
- **THEN** `agent_key` SHALL 使用 `{position_key}-{variant}` 格式（如 `quant-researcher-factor`），agent_key SHALL 匹配正则 `^[a-z0-9_-]+$`

#### Scenario: Agent model 选择
- **WHEN** 查看 `agents.yaml` 中的 Agent 定义
- **THEN** `fast_model` 和 `strong_model` 的选择 SHALL 与岗位复杂度匹配，复杂推理岗位 SHALL 使用更强的模型

#### Scenario: Agent tools_profile 匹配
- **WHEN** 查看 `agents.yaml` 中的 Agent 定义
- **THEN** `tools_profile` SHALL 与岗位职责匹配（如 DBA Agent SHALL 包含数据库相关工具，前端 Agent SHALL 包含代码分析工具）

### Requirement: Prompt 文件交付
每个 Agent SHALL 有对应的 prompt 文件。

#### Scenario: Prompt 文件路径
- **WHEN** 查看 `internal/scenario/softwaredev/prompts/positions/` 目录
- **THEN** 每个岗位 SHALL 有对应的子目录（如 `backend/`、`frontend/`、`gamedev/`），每个 variant SHALL 有对应的 `.md` 文件（如 `backend/java_senior.md` 或 `backend/java_senior/general.md`）

#### Scenario: Prompt 内容质量
- **WHEN** 查看任意 prompt 文件
- **THEN** prompt 内容 SHALL 包含角色定义、职责描述、工作流程、输出格式要求，与 `taxonomy.yaml` 中的岗位描述一致

#### Scenario: Prompt 与 taxonomy.yaml 一致性
- **WHEN** 对比 prompt 文件和 taxonomy.yaml 中的岗位定义
- **THEN** prompt 中的角色描述 SHALL 与 taxonomy.yaml 中该岗位的 `responsibilities` 和 `skills_required` 一致

### Requirement: Skill 文件交付
需要 Skill 文件的 Agent SHALL 有对应的 Skill 定义文件。

#### Scenario: Skill 文件路径
- **WHEN** 查看 `internal/scenario/softwaredev/skills/` 目录
- **THEN** 每个 Skill SHALL 有对应的 `.md` 文件，Skill 命名 SHALL 语义化且与岗位职责匹配

#### Scenario: Skill 引用一致性
- **WHEN** 查看 `agents.yaml` 中 Agent 的 `skills` 字段
- **THEN** 引用的 Skill slug SHALL 与 `skills/` 目录下的文件名一致

### Requirement: Schema 文件交付（如需要）
需要 Schema 文件的 Agent SHALL 有对应的 Schema 定义文件。

#### Scenario: Schema 文件路径
- **WHEN** 查看 `internal/scenario/softwaredev/schemas/` 目录
- **THEN** 每个 Schema 文件 SHALL 定义结构化输出格式，供 Agent 在执行任务时使用

### Requirement: 批次进度目标
softwaredev Agent 补全 SHALL 按批次达到累计数量目标。

#### Scenario: P1 完成后数量
- **WHEN** P1 批次（backend + frontend + gamedev）完成
- **THEN** softwaredev Agent 总数 SHALL 从 10 增长到约 37 个

#### Scenario: P2 完成后数量
- **WHEN** P2 批次（devops + architecture + qa）完成
- **THEN** softwaredev Agent 总数 SHALL 从约 37 增长到约 60 个

#### Scenario: P3 完成后数量
- **WHEN** P3 批次（mobiledev + dataeng + security + productpm）完成
- **THEN** softwaredev Agent 总数 SHALL 从约 60 增长到约 82 个

### Requirement: 补全后验证
softwaredev Agent 补全完成后 SHALL 通过种子数据验证。

#### Scenario: 种子数据写入验证
- **WHEN** 运行 seed-industry-agents CLI 或应用启动自动 seed
- **THEN** softwaredev 行业的所有 Agent 数据 SHALL 正确写入数据库，agent_key 唯一，position_key 正确归属到对应岗位

#### Scenario: 编译验证
- **WHEN** 运行 `make build`
- **THEN** 编译 SHALL 成功，无对 softwaredev Agent 定义的残留错误

#### Scenario: Agent 命名/归属复查
- **WHEN** 对 softwaredev agents.yaml 执行 aranea-review 审查
- **THEN** 每个 Agent 的 `position_key` SHALL 正确归属到对应岗位，`agent_variant` 命名 SHALL 语义化且一致，同一岗位不同 variant 之间的职责边界 SHALL 清晰，model 选择 SHALL 与岗位复杂度匹配，tools_profile SHALL 与岗位职责匹配
