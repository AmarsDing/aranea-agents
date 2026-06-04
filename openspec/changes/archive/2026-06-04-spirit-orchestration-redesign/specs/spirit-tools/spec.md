# Spirit Tools (Modified)

## Overview
Spirit 工具集从 7 工具精简为 3 工具，旧工具双写过渡期保留。

## Requirements

### REQ-SKT-01: 新工具 plan_and_execute
- 替代 assess_complexity + assemble_team + list_butlers + query_butler_status
- 输入：task_prompt + mode(auto|direct|single|parallel|dag|coordinator)
- 输出：plan_id, strategy, complexity_level, subtask_count, sub_tasks[], orchestration_id, memory_hit
- 内部顺序调用 TaskPlanner.Plan → AgentAllocator.Allocate → TaskOrchestrator.Orchestrate
- mode=auto 时自动选择策略

### REQ-SKT-02: 新工具 check_progress
- 替代 check_team_progress
- 输入：orchestration_id
- 输出：[]TaskProgress（基于 Graph 事件的细粒度进度）

### REQ-SKT-03: 新工具 cancel_orchestration
- 替代 cancel_team
- 输入：orchestration_id
- 调用 TaskOrchestrator.Cancel()

### REQ-SKT-04: 保留 synthesize_results
- 输入：spirit_session_id
- 调用 TaskOrchestrator.Synthesize()

### REQ-SKT-05: 旧工具双写过渡
- 旧 7 工具保留 2 个版本，标记 deprecated
- 旧工具内部委托新工具实现
- builtin_tools_seed.go 同时注册新旧工具
- DECISION.md / CAPABILITIES.md prompt 更新工具名引用
- 2 个版本后移除旧工具

### REQ-SKT-06: ComplexityRuleEngine 更新
- 更新 complexAvailableTools / moderateAvailableTools 中的工具名
- assess_complexity 逻辑内嵌到 ChatOrchestrator 的 turn 入口
