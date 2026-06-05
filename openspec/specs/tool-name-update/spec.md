# tool-name-update Specification

## Purpose
TBD - created by archiving change spirit-orchestration-review-fixes. Update Purpose after archive.
## Requirements
### Requirement: 更新 spirit profile 工具名引用
- Spirit profile SHALL 更新工具名引用，过渡期 MUST 同时保留新旧工具名。
- 将 spirit profile 的工具列表从旧工具名更新为包含新工具名
- 新工具名：plan_and_execute, check_progress, cancel_orchestration
- 旧工具名：assemble_team, list_butlers, query_butler_status, check_team_progress, cancel_team
- 过渡期同时保留新旧工具名，确保向后兼容

#### Scenario: Spirit agent 可调用新工具
- Given Spirit agent 使用 spirit 工具 profile
- When LLM 选择工具时
- Then 新工具名（plan_and_execute, check_progress, cancel_orchestration）可用
- And 旧工具名（assemble_team, check_team_progress, cancel_team）仍可用（过渡期）

