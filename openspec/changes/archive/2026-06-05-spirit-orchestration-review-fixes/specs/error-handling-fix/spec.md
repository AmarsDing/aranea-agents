# Error Handling Fix

## Overview
修复 task_orchestrator_impl.go、dag_graph_compiler.go、agent_as_tool.go 中业务错误使用 fmt.Errorf 而非 kerrors 的问题。

## ADDED Requirements

### Requirement: task_orchestrator_impl.go 错误修复
- TaskOrchestrator SHALL 将所有 fmt.Errorf 业务错误替换为 kerrors 类型化错误。
- 行 196: `fmt.Errorf("build agent-as-tool: %w", err)` → `kerrors.InternalServer("SPIRIT", "build agent-as-tool: "+err.Error())`
- 行 240: `fmt.Errorf("assemble team: %w", err)` → `kerrors.InternalServer("SPIRIT", "assemble team: "+err.Error())`
- 行 268: `fmt.Errorf("compile DAG to graph: %w", err)` → `kerrors.InternalServer("SPIRIT", "compile DAG to graph: "+err.Error())`
- 行 316: `fmt.Errorf("assemble DAG team: %w", err)` → `kerrors.InternalServer("SPIRIT", "assemble DAG team: "+err.Error())`
- 行 496: `fmt.Errorf("no checkpoint available...")` → `kerrors.NotFound("SPIRIT", "no checkpoint available for orchestration "+orchestrationID)`
- 行 520: `fmt.Errorf("failed to load checkpoint...")` → `kerrors.InternalServer("SPIRIT", "failed to load checkpoint for orchestration "+orchestrationID+": "+loadErr.Error())`
- 行 537: `fmt.Errorf("checkpoint %s not found...")` → `kerrors.NotFound("SPIRIT", fmt.Sprintf("checkpoint %s not found for orchestration %s", ...))`
- 行 576: `fmt.Errorf("list interrupted orchestrations: %w", err)` → `kerrors.InternalServer("SPIRIT", "list interrupted orchestrations: "+err.Error())`

#### Scenario: 业务错误返回正确的 HTTP 状态码
- Given TaskOrchestrator 处理编排请求时发生错误
- When 错误是内部错误（build/assemble/compile 失败）
- Then 返回 kerrors.InternalServer（HTTP 500）
- When 错误是资源未找到（checkpoint 不存在）
- Then 返回 kerrors.NotFound（HTTP 404）

### Requirement: dag_graph_compiler.go 错误修复
- DAGToGraphCompiler SHALL 将参数校验错误替换为 kerrors.BadRequest。
- 行 35: `fmt.Errorf("dag and allocPlan must not be nil")` → `kerrors.BadRequest("SPIRIT", "dag and allocPlan must not be nil")`

#### Scenario: 参数校验错误返回 400
- Given DAGToGraphCompiler.Compile 被调用时 dag 或 allocPlan 为 nil
- When 编译器检测到参数为空
- Then 返回 kerrors.BadRequest（HTTP 400）

### Requirement: agent_as_tool.go 错误修复
- BuildAgentAsTool SHALL 将 Agent 未找到错误替换为 kerrors.NotFound。
- 行 24: `fmt.Errorf("no matching agent found for: %s", taskDesc)` → `kerrors.NotFound("SPIRIT", "no matching agent found for: "+taskDesc)`

#### Scenario: Agent 未找到返回 404
- Given BuildAgentAsTool 被调用时没有匹配的 Agent
- When matcher 返回 nil match
- Then 返回 kerrors.NotFound（HTTP 404）
