## ADDED Requirements

### Requirement: stockx Agent 合并到 finance/agents.yaml
stockx 的所有 Agent 定义 SHALL 合并到 `finance/agents.yaml`，使用 finance 的 taxonomy 岗位映射和 variant 命名。

#### Scenario: agent_coordinator 映射
- **WHEN** 合并 stockx `agent_coordinator`（主控调度员）
- **THEN** 该 Agent SHALL 映射到 finance `trading_coordinator` 岗位的 `premarket` variant，agent_key 格式为 `trading-coordinator-premarket`

#### Scenario: agent_critic 映射
- **WHEN** 合并 stockx `agent_critic`（评审员）
- **THEN** 该 Agent SHALL 映射到 finance `trading_coordinator` 岗位的 `critic` variant，agent_key 格式为 `trading-coordinator-critic`

#### Scenario: agent_data_collector 映射
- **WHEN** 合并 stockx `agent_data_collector`（数据采集员）
- **THEN** 该 Agent SHALL 映射到 finance `data_collector` 岗位的 `general` variant，agent_key 格式为 `data-collector-general`

#### Scenario: agent_technical_analyst 映射
- **WHEN** 合并 stockx `agent_technical_analyst`（技术分析师）
- **THEN** 该 Agent SHALL 映射到 finance `technical_analyst` 岗位的 `general` variant，agent_key 格式为 `technical-analyst-general`

#### Scenario: agent_fundamental_analyst 映射
- **WHEN** 合并 stockx `agent_fundamental_analyst`（基本面分析师）
- **THEN** 该 Agent SHALL 映射到 finance `fundamental_analyst` 岗位的 `general` variant，agent_key 格式为 `fundamental-analyst-general`

#### Scenario: agent_money_flow_analyst 映射
- **WHEN** 合并 stockx `agent_money_flow_analyst`（资金面分析师）
- **THEN** 该 Agent SHALL 映射到 finance `money_flow_analyst` 岗位的 `general` variant，agent_key 格式为 `money-flow-analyst-general`

#### Scenario: agent_news_analyst 映射
- **WHEN** 合并 stockx `agent_news_analyst`（消息面分析师）
- **THEN** 该 Agent SHALL 映射到 finance `news_analyst` 岗位的 `general` variant，agent_key 格式为 `news-analyst-general`

#### Scenario: agent_sentiment_analyst 映射
- **WHEN** 合并 stockx `agent_sentiment_analyst`（情绪面分析师）
- **THEN** 该 Agent SHALL 映射到 finance `sentiment_analyst` 岗位的 `general` variant，agent_key 格式为 `sentiment-analyst-general`

#### Scenario: agent_industry_analyst 映射
- **WHEN** 合并 stockx `agent_industry_analyst`（行业分析师）
- **THEN** 该 Agent SHALL 映射到 finance `industry_analyst` 岗位的 `general` variant，agent_key 格式为 `industry-analyst-general`

#### Scenario: agent_risk_assessor 映射
- **WHEN** 合并 stockx `agent_risk_assessor`（风险评估师）
- **THEN** 该 Agent SHALL 映射到 finance `risk_assessor` 岗位的 `general` variant，agent_key 格式为 `risk-assessor-general`

#### Scenario: agent_quant_factor 映射
- **WHEN** 合并 stockx `agent_quant_factor`（因子计算员）
- **THEN** 该 Agent SHALL 映射到 finance `quant_researcher` 岗位的 `factor` variant，agent_key 格式为 `quant-researcher-factor`

#### Scenario: agent_chart_builder 映射
- **WHEN** 合并 stockx `agent_chart_builder`（图表构建员）
- **THEN** 该 Agent SHALL 映射到 finance `report_writer` 岗位的 `chart` variant，agent_key 格式为 `report-writer-chart`

#### Scenario: agent_report_writer 映射
- **WHEN** 合并 stockx `agent_report_writer`（报告撰写员）
- **THEN** 该 Agent SHALL 映射到 finance `report_writer` 岗位的 `general` variant，agent_key 格式为 `report-writer-general`

#### Scenario: Agent key 唯一性
- **WHEN** 所有 stockx Agent 合并到 finance/agents.yaml 后
- **THEN** 所有 agent_key SHALL 在 finance/agents.yaml 中唯一，不与已有 finance Agent 冲突

### Requirement: stockx Team 合并到 finance/agents.yaml
stockx 的所有 Team 定义 SHALL 合并到 `finance/agents.yaml`，成员引用 SHALL 统一使用 finance 的 agent_key。

#### Scenario: 已有 Team 成员引用统一
- **WHEN** 合并 stockx 已有的 5 个 Team（`team-premarket-brief`、`team-stock-deep-dive`、`team-sector-rotation`、`team-portfolio-doctor`、`team-market-recap`）
- **THEN** 每个 Team 的成员引用 SHALL 使用 finance agent_key（如 `trading-coordinator-premarket`、`data-collector-general` 等），不使用 stockx 原始 agent_key

#### Scenario: team-research-pipeline 新增
- **WHEN** 合并 stockx Team 到 finance/agents.yaml
- **THEN** `team-research-pipeline`（研究管线团队）SHALL 被添加到 finance/agents.yaml，成员引用 SHALL 使用 finance agent_key

#### Scenario: team-deep-dive-critic 新增
- **WHEN** 合并 stockx Team 到 finance/agents.yaml
- **THEN** `team-deep-dive-critic`（深度分析评审团队）SHALL 被添加到 finance/agents.yaml，成员引用 SHALL 使用 finance agent_key

#### Scenario: Team 配置完整性
- **WHEN** 查看 finance/agents.yaml 中的 Team 定义
- **THEN** 每个 Team SHALL 包含完整的 `mode`（coordinator/parallel/sequential）、`members` 列表（含 agent_key 和 role）、`description`，Graph 定义（nodes/edges）SHALL 完整且可达

### Requirement: stockx Prompt 迁移
stockx 的 prompt 文件 SHALL 从 `cmd/seed-stockx-org/` 迁移到 `internal/scenario/finance/prompts/positions/` 目录。

#### Scenario: critic prompt 迁移
- **WHEN** 迁移 stockx `agent_critic` 的 prompt
- **THEN** prompt 文件 SHALL 存在于 `internal/scenario/finance/prompts/positions/trading_coordinator/critic.md`

#### Scenario: chart prompt 迁移
- **WHEN** 迁移 stockx `agent_chart_builder` 的 prompt
- **THEN** prompt 文件 SHALL 存在于 `internal/scenario/finance/prompts/positions/report_writer/chart.md`

#### Scenario: Prompt 内容适配
- **WHEN** 查看迁移后的 prompt 文件
- **THEN** prompt 内容 SHALL 适配 finance 场景上下文，保留核心分析逻辑，更新角色描述为 finance 体系下的命名

### Requirement: 删除 cmd/seed-stockx-org/
`cmd/seed-stockx-org/` 目录 SHALL 被完全删除。

#### Scenario: 目录删除
- **WHEN** 查看 `cmd/` 目录
- **THEN** `cmd/seed-stockx-org/` 目录 SHALL 不存在

#### Scenario: 编译验证
- **WHEN** 运行 `make build`
- **THEN** 编译 SHALL 成功，无对 `cmd/seed-stockx-org/` 的残留引用

#### Scenario: Wire 验证
- **WHEN** 运行 `make wire`
- **THEN** wire_gen.go SHALL 不包含任何 stockx 相关的依赖注入

### Requirement: stockx 独立分类树定义删除
stockx 相关的独立分类树定义 SHALL 被删除，统一使用 finance 的 taxonomy 岗位体系。

#### Scenario: 无独立 stockx 分类树
- **WHEN** 查看 `internal/scenario/` 目录
- **THEN** SHALL 不存在独立的 stockx 分类树定义文件，stockx 场景 SHALL 完全通过 finance 的 taxonomy 岗位体系表达

### Requirement: 合并后验证
stockx 合并完成后 SHALL 通过全量编译和种子数据验证。

#### Scenario: 编译验证
- **WHEN** 运行 `make api && make wire && make build && make test`
- **THEN** 所有命令 SHALL 成功通过

#### Scenario: 种子数据验证
- **WHEN** 运行 seed-industry-agents CLI 或应用启动自动 seed
- **THEN** finance 行业的 Agent 和 Team 数据 SHALL 正确写入，包含所有合并后的 stockx Agent variant 和 Team

#### Scenario: Agent key 无冲突
- **WHEN** 检查 finance/agents.yaml 中所有 agent_key
- **THEN** 所有 agent_key SHALL 唯一，stockx 合并后的 Agent 与原有 finance Agent 无 key 冲突
